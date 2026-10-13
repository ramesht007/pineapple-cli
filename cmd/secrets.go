package cmd

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/ramesht007/pineapple-cli/internal/config"
	"github.com/ramesht007/pineapple-cli/internal/paths"
	"github.com/ramesht007/pineapple-cli/internal/repo"
	"github.com/ramesht007/pineapple-cli/internal/secrets"
)

var keyRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

var errNoChanges = errors.New("no changes")

func secretsCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "secrets",
		Short: "Manage age-encrypted secrets synced into ~/.claude/.env",
	}
	c.AddCommand(secretsInitCmd(), secretsSetCmd(), secretsRmCmd(), secretsListCmd(), secretsEditCmd())
	return c
}

// printAgeKey creates the age key if needed and prints the public half plus copy instructions.
func printAgeKey() error {
	pub, created, err := secrets.EnsureKey()
	if err != nil {
		return err
	}
	if created {
		fmt.Println("generated age key at", paths.AgeKeyFile())
	}
	fmt.Println("age public key:", pub)
	fmt.Printf("copy %s to your other machines (keep it 0600; pine never uploads it)\n", paths.AgeKeyFile())
	return nil
}

func secretsInitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Create the age key for this machine (or show the existing public key)",
		Args:  cobra.NoArgs,
		RunE:  func(*cobra.Command, []string) error { return printAgeKey() },
	}
}

// mutate pulls the latest secrets, lets fn change them, then pushes and applies locally.
func mutate(fn func(map[string]string) error) error {
	cfg, _, err := load()
	if err != nil {
		return err
	}
	dir := paths.RepoDir()
	if err := repo.ResetToOriginHead(dir); err != nil {
		return fmt.Errorf("pull: %w", err)
	}
	m, err := secrets.Load(dir)
	if err != nil {
		return err
	}
	if err := fn(m); errors.Is(err, errNoChanges) {
		progress("no changes")
		return nil
	} else if err != nil {
		return err
	}
	if err := secrets.Save(dir, m); err != nil {
		return err
	}
	progress("pushing secrets to %s", cfg.Repo)
	if _, err := commitPush(dir, "secrets: update"); err != nil {
		return err
	}
	return secrets.Apply(dir)
}

func secretsSetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "set KEY=VALUE [KEY=VALUE...]",
		Short: "Add or update secrets, push, and apply locally",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return mutate(func(m map[string]string) error {
				for _, a := range args {
					k, v, ok := strings.Cut(a, "=")
					if !ok || !keyRe.MatchString(k) {
						return fmt.Errorf("expected KEY=VALUE with KEY matching %s", keyRe)
					}
					if strings.ContainsAny(v, "\r\n") {
						return fmt.Errorf("%s: value must be a single line", k)
					}
					m[k] = v
				}
				return nil
			})
		},
	}
}

func secretsRmCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "rm KEY [KEY...]",
		Short: "Delete secrets, push, and apply locally",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return mutate(func(m map[string]string) error {
				for _, k := range args {
					if _, ok := m[k]; !ok {
						return fmt.Errorf("no such secret: %s", k)
					}
					delete(m, k)
				}
				return nil
			})
		},
	}
}

func secretsListCmd() *cobra.Command {
	var values bool
	c := &cobra.Command{
		Use:   "list",
		Short: "List secret keys from the local clone (run `pine pull` to refresh)",
		Args:  cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			if _, err := config.Load(); err != nil {
				return err
			}
			m, err := secrets.Load(paths.RepoDir())
			if err != nil {
				return err
			}
			if values {
				fmt.Print(secrets.Render(m))
				return nil
			}
			keys := make([]string, 0, len(m))
			for k := range m {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				fmt.Println(k)
			}
			return nil
		},
	}
	c.Flags().BoolVar(&values, "values", false, "show values too")
	return c
}

func secretsEditCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "edit",
		Short: "Edit all secrets in $EDITOR, then push and apply locally",
		Args:  cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			return mutate(func(m map[string]string) error {
				tmp, err := os.CreateTemp("", "pine-secrets-*.env")
				if err != nil {
					return err
				}
				defer os.Remove(tmp.Name())
				before := secrets.Render(m)
				if err := tmp.Chmod(0o600); err != nil {
					return err
				}
				if _, err := tmp.WriteString(before); err != nil {
					return err
				}
				if err := tmp.Close(); err != nil {
					return err
				}
				editor := strings.Fields(os.Getenv("EDITOR"))
				if len(editor) == 0 {
					editor = []string{"vi"}
				}
				ed := exec.Command(editor[0], append(editor[1:], tmp.Name())...)
				ed.Stdin, ed.Stdout, ed.Stderr = os.Stdin, os.Stdout, os.Stderr
				if err := ed.Run(); err != nil {
					return fmt.Errorf("%s: %w", editor[0], err)
				}
				b, err := os.ReadFile(tmp.Name())
				if err != nil {
					return err
				}
				after, err := secrets.Parse(string(b))
				if err != nil {
					return err
				}
				for k := range after {
					if !keyRe.MatchString(k) {
						return fmt.Errorf("invalid key %q (must match %s)", k, keyRe)
					}
				}
				if secrets.Render(after) == before {
					return errNoChanges
				}
				for k := range m {
					delete(m, k)
				}
				for k, v := range after {
					m[k] = v
				}
				return nil
			})
		},
	}
}
