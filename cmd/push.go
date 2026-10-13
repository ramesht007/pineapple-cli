package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/ramesht007/pineapple-cli/internal/config"
	"github.com/ramesht007/pineapple-cli/internal/manifest"
	"github.com/ramesht007/pineapple-cli/internal/paths"
	"github.com/ramesht007/pineapple-cli/internal/repo"
	"github.com/ramesht007/pineapple-cli/internal/scan"
	"github.com/ramesht007/pineapple-cli/internal/syncer"
)

var errFindings = errors.New("possible secrets found; nothing was pushed. Remove them from ~/.claude (or move them to secrets) and retry")

func pushCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "push",
		Short: "Push local skills/mods to the sync repo",
		Args:  cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			cfg, st, err := load()
			if err != nil {
				return err
			}
			return pushFlow(cfg, &st, false)
		},
	}
}

// stage mirrors local config into the repo clone (no heartbeat, no git).
func stage(dir string) error {
	if err := syncer.Snapshot(paths.ClaudeSkills(), paths.ClaudeMods(), dir); err != nil {
		return err
	}
	return manifest.Push(dir)
}

func writeHeartbeat(cfg config.Config, st config.State, dir string) error {
	now := time.Now().UTC()
	pull := st.LastPull
	if pull.IsZero() {
		pull = now
	}
	hb := map[string]string{
		"os":       runtime.GOOS,
		"lastPush": now.Format(time.RFC3339),
		"lastPull": pull.UTC().Format(time.RFC3339),
	}
	name := strings.NewReplacer("/", "_", string(os.PathSeparator), "_").Replace(cfg.Machine)
	return config.WriteJSON(filepath.Join(dir, "machines", name+".json"), hb, 0o644)
}

// commitPush scans staged additions for secrets, then commits and pushes.
func commitPush(dir, msg string) (bool, error) {
	lines, err := repo.DiffCachedAddedLines(dir)
	if err != nil {
		return false, err
	}
	if fs := scan.Scan(lines); len(fs) > 0 {
		for _, f := range fs {
			fmt.Fprintf(os.Stderr, "SECRET? %s: %s [%s]\n", f.File, strings.TrimSpace(f.Line), f.Rule)
		}
		if err := repo.Reset(dir); err != nil {
			return false, err
		}
		return false, errFindings
	}
	return repo.AddAllCommitPush(dir, msg)
}

func pushFlow(cfg config.Config, st *config.State, retried bool) error {
	dir := paths.RepoDir()
	if err := stage(dir); err != nil {
		return err
	}
	dirty, err := repo.Dirty(dir)
	if err != nil {
		return err
	}
	if !dirty {
		progress("nothing to push")
		st.LastPush = time.Now()
		return config.SaveState(*st)
	}
	if err := writeHeartbeat(cfg, *st, dir); err != nil {
		return err
	}
	progress("pushing to %s", cfg.Repo)
	_, err = commitPush(dir, fmt.Sprintf("sync from %s at %s", cfg.Machine, time.Now().Format(time.RFC3339)))
	if errors.Is(err, repo.ErrNonFastForward) && !retried {
		progress("remote has newer commits; pulling then retrying")
		if err := pullFlow(cfg, st); err != nil {
			return err
		}
		return pushFlow(cfg, st, true)
	}
	if err != nil {
		return err
	}
	if st.Files, err = syncer.Rehash(dir); err != nil {
		return err
	}
	st.LastPush = time.Now()
	progress("pushed")
	return config.SaveState(*st)
}
