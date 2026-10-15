package cmd

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/ramesht007/pineapple-cli/internal/config"
	"github.com/ramesht007/pineapple-cli/internal/lockfile"
	"github.com/ramesht007/pineapple-cli/internal/manifest"
	"github.com/ramesht007/pineapple-cli/internal/paths"
	"github.com/ramesht007/pineapple-cli/internal/repo"
	"github.com/ramesht007/pineapple-cli/internal/secrets"
	"github.com/ramesht007/pineapple-cli/internal/syncer"
	"github.com/ramesht007/pineapple-cli/internal/ui"
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
			release, err := lockfile.Acquire()
			if err != nil {
				return err
			}
			defer release()
			return pullFlow(cfg, &st)
		},
	}
}

func pullFlow(cfg config.Config, st *config.State) error {
	dir := paths.RepoDir()
	stop := ui.Spin("checking " + cfg.Repo)
	err := repo.Fetch(dir)
	stop(err == nil)
	if err != nil {
		return fmt.Errorf("check remote: %w", err)
	}
	sha, err := repo.OriginSHA(dir)
	if err != nil {
		return fmt.Errorf("check remote: %w", err)
	}
	if sha != "" && sha == st.AppliedSHA {
		// Secrets re-apply even when up to date: the age key may have arrived after the last pull.
		applySecrets(dir)
		progress("%s", ui.Dim("up to date"))
		return nil
	}
	progress("pulling from %s", cfg.Repo)
	if err := repo.ResetToOriginHead(dir); err != nil {
		return err
	}
	applied, conflicts, err := syncer.Apply(dir, paths.ClaudeSkills(), paths.ClaudeMods(), st.Files)
	for _, c := range conflicts {
		fmt.Fprintln(os.Stderr, ui.Warn(fmt.Sprintf("CONFLICT: %s (remote version applied; your local copy was backed up)", c)))
	}
	if err != nil {
		return fmt.Errorf("apply: %w", err)
	}
	if err := manifest.Apply(dir); err != nil {
		return fmt.Errorf("manifest: %w", err)
	}
	applySecrets(dir)
	files, err := syncer.Rehash(dir)
	if err != nil {
		return err
	}
	// State is saved only after everything applied, so a failure re-applies on the next run.
	st.Files, st.AppliedSHA, st.LastPull = files, sha, time.Now()
	progress("%s pulled: %d file(s) changed, %d conflict(s)", ui.Good("✓"), len(applied), len(conflicts))
	return config.SaveState(*st)
}

// applySecrets is best-effort: a secrets failure must never break an unattended pull.
func applySecrets(dir string) {
	if err := secrets.Apply(dir); err != nil {
		fmt.Fprintln(os.Stderr, ui.Warn("secrets: "+err.Error()))
	}
}
