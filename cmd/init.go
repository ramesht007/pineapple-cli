package cmd

import (
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
				progress("creating GitHub repo %s", name)
				var err error
				if url, err = repo.CreateRepo(name, !public); err != nil {
					return err
				}
			}
			progress("cloning %s", url)
			if err := repo.Clone(url, dir); err != nil {
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
				return nil
			}
			if err := seed(cfg, &st, dir); err != nil {
				return err
			}
			return printAgeKey()
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
