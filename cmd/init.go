package cmd

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/ramesht007/pineapple-cli/internal/config"
	"github.com/ramesht007/pineapple-cli/internal/paths"
	"github.com/ramesht007/pineapple-cli/internal/repo"
	"github.com/ramesht007/pineapple-cli/internal/syncer"
	"github.com/ramesht007/pineapple-cli/internal/ui"
)

func initCmd() *cobra.Command {
	var from, name string
	var public, force, yes bool
	c := &cobra.Command{
		Use:   "init",
		Short: "Create a new sync repo, or join an existing one with --from",
		Args:  cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			if config.Exists() && !force {
				return fmt.Errorf("already initialised (%s); use --force to start over", paths.ConfigFile())
			}
			dir := paths.RepoDir()
			if err := os.RemoveAll(dir); err != nil { // the clone is only a cache
				return err
			}
			var url string
			if from != "" {
				url = expandRepo(from)
				if strings.HasPrefix(url, "https://") {
					if err := repo.EnsureGH(yes); err != nil {
						return err
					}
				}
			} else {
				if err := repo.EnsureGH(yes); err != nil {
					return err
				}
				owner, err := repo.Owner()
				if err != nil {
					return err
				}
				if full := owner + "/" + name; repo.RepoExists(full) {
					fmt.Fprintf(os.Stderr, "repo %s already exists on your account\n", full)
					if !confirmDefaultYes("join it instead?", yes) {
						return fmt.Errorf("use --repo-name to create a different repo, or pine init --from %s", full)
					}
					from, url = full, "https://github.com/"+full+".git"
				} else {
					progress("creating GitHub repo %s", name)
					if url, err = repo.CreateRepo(name, !public); err != nil {
						return err
					}
				}
			}
			stop := ui.Spin("cloning " + url)
			err := repo.Clone(url, dir)
			stop(err == nil)
			if err != nil {
				return err
			}
			cfg, st := config.New(url), config.State{Files: map[string]string{}}
			if from != "" && repo.OriginRef(dir) != "" {
				if err := config.Save(cfg); err != nil {
					return err
				}
				if err := pullFlow(cfg, &st); err != nil {
					return err
				}
				if _, err := os.Stat(paths.AgeKeyFile()); err != nil {
					progress("secrets stay encrypted until you copy age.key from another machine to %s", paths.AgeKeyFile())
				}
				nextSteps(true)
				return nil
			}
			if err := seed(cfg, &st, dir); err != nil {
				return err
			}
			if err := printAgeKey(); err != nil {
				return err
			}
			nextSteps(false)
			return nil
		},
	}
	c.Flags().StringVar(&from, "from", "", "join an existing repo (owner/name, git URL or local path)")
	c.Flags().StringVar(&name, "repo-name", "pineapple-config", "name of the GitHub repo to create")
	c.Flags().BoolVar(&public, "public", false, "create the GitHub repo as public (default private)")
	c.Flags().BoolVar(&force, "force", false, "overwrite an existing pine config")
	c.Flags().BoolVarP(&yes, "yes", "y", false, "auto-approve installing missing dependencies (git, gh)")
	return c
}

// expandRepo turns owner/name into an https URL; URLs and existing paths pass through.
func expandRepo(x string) string {
	if strings.Contains(x, "://") || strings.Contains(x, "@") {
		return x
	}
	if abs, err := filepath.Abs(x); err == nil {
		if _, err := os.Stat(abs); err == nil {
			return abs
		}
	}
	return "https://github.com/" + x + ".git"
}

// confirmDefaultYes asks a Y/n question; enter, EOF or anything but n/no means yes.
func confirmDefaultYes(q string, assumeYes bool) bool {
	if assumeYes {
		return true
	}
	fmt.Fprintf(os.Stderr, "%s [Y/n] ", q)
	line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	a := strings.ToLower(strings.TrimSpace(line))
	return a != "n" && a != "no"
}

// seed fills an empty remote with this machine's config.
func seed(cfg config.Config, st *config.State, dir string) error {
	if err := repo.CheckoutMain(dir); err != nil {
		return err
	}
	meta := filepath.Join(dir, ".pineapple", "meta.json")
	if err := config.WriteJSON(meta, map[string]int{"schema": 1}, 0o644); err != nil {
		return err
	}
	if err := stage(dir); err != nil {
		return err
	}
	if err := writeHeartbeat(cfg, *st, dir); err != nil {
		return err
	}
	progress("seeding remote")
	if _, err := commitPush(dir, "pineapple init from "+cfg.Machine); err != nil {
		return err
	}
	var err error
	if st.Files, err = syncer.Rehash(dir); err != nil {
		return err
	}
	st.LastPush = time.Now()
	if err := config.Save(cfg); err != nil {
		return err
	}
	progress("initialised")
	return config.SaveState(*st)
}

// nextSteps prints the post-init box (stderr, like other progress output).
func nextSteps(join bool) {
	if quiet {
		return
	}
	lines := []string{
		"pine daemon install    automate sync on this machine",
		"copy " + paths.AgeKeyFile() + " to your other machines (secrets)",
	}
	if !join {
		lines = append(lines, "on other machines: pine init --from <owner/repo>")
	}
	fmt.Fprint(os.Stderr, "\n"+ui.Box("next steps", lines...))
}
