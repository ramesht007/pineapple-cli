package cmd

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/ramesht007/pineapple-cli/internal/config"
	"github.com/ramesht007/pineapple-cli/internal/manifest"
	"github.com/ramesht007/pineapple-cli/internal/paths"
	"github.com/ramesht007/pineapple-cli/internal/repo"
	"github.com/ramesht007/pineapple-cli/internal/secrets"
	"github.com/ramesht007/pineapple-cli/internal/syncer"
)

func pullCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "pull",
		Short: "Apply remote changes to ~/.claude",
		Args:  cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			cfg, st, err := load()
			if err != nil {
				return err
			}
			return pullFlow(cfg, &st)
		},
	}
}

func pullFlow(cfg config.Config, st *config.State) error {
	dir := paths.RepoDir()
	ahead, err := repo.RemoteAhead(dir)
	if err != nil {
		return fmt.Errorf("check remote: %w", err)
	}
	if !ahead && len(st.Files) > 0 {
		progress("up to date")
		return nil
	}
	progress("pulling from %s", cfg.Repo)
	if err := repo.ResetToOriginHead(dir); err != nil {
		return err
	}
	applied, conflicts, err := syncer.Apply(dir, paths.ClaudeSkills(), paths.ClaudeMods(), st.Files)
	for _, c := range conflicts {
		fmt.Fprintf(os.Stderr, "CONFLICT: %s (remote version applied; your local copy was backed up)\n", c)
	}
	if err != nil {
		return fmt.Errorf("apply: %w", err)
	}
	if err := manifest.Apply(dir); err != nil {
		return fmt.Errorf("manifest: %w", err)
	}
	if err := secrets.Apply(dir); err != nil {
		return fmt.Errorf("secrets: %w", err)
	}
	if st.Files, err = syncer.Rehash(dir); err != nil {
		return err
	}
	st.LastPull = time.Now()
	progress("pulled: %d file(s) changed, %d conflict(s)", len(applied), len(conflicts))
	return config.SaveState(*st)
}
