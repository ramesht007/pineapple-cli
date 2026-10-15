package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/ramesht007/pineapple-cli/internal/config"
	"github.com/ramesht007/pineapple-cli/internal/daemon"
	"github.com/ramesht007/pineapple-cli/internal/paths"
	"github.com/ramesht007/pineapple-cli/internal/ui"
)

func doctorCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Diagnose the pine setup",
		Args:  cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			failed := false
			// mark: ok -> ✓, critical -> ✗, else –
			row := func(ok, critical bool, name, detail, fix string) {
				m := ui.Good("✓")
				if !ok {
					m = ui.Dim("–")
					if critical {
						m, failed = ui.Bad("✗"), true
					}
				}
				fmt.Printf("%s %-14s %s\n", m, name, detail)
				if !ok && fix != "" {
					fmt.Println(ui.Dim("    fix: " + fix))
				}
			}
			_, gitErr := exec.LookPath("git")
			row(gitErr == nil, true, "git", "on PATH", "install git (macOS: xcode-select --install)")

			cfg, cfgErr := config.Load()
			row(cfgErr == nil, true, "config", paths.ConfigFile(), "run `pine init`")

			if cfgErr == nil {
				_, err := os.Stat(filepath.Join(paths.RepoDir(), ".git"))
				if err != nil {
					row(false, true, "repo", paths.RepoDir()+" missing", "run `pine init`")
				} else {
					ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
					err := exec.CommandContext(ctx, "git", "-C", paths.RepoDir(), "ls-remote", "origin").Run()
					cancel()
					row(err == nil, true, "remote", cfg.Repo, "check network and git credentials (`gh auth login`) for "+cfg.Repo)
				}
			}

			_, ghErr := exec.LookPath("gh")
			if ghErr == nil {
				ghErr = exec.Command("gh", "auth", "status").Run()
			}
			row(ghErr == nil, false, "gh", "installed and authenticated (only needed for repo creation)", "run `pine init` (installs gh and logs in) or `gh auth login`")

			_, clErr := exec.LookPath("claude")
			row(clErr == nil, false, "claude", "on PATH (otherwise plugin installs wait for Claude Code launch)", "install Claude Code")

			keyPath := filepath.Join(paths.PineappleHome(), "age.key")
			if _, err := os.Stat(keyPath); err != nil {
				row(false, false, "age key", keyPath+" missing (secrets stay encrypted)", "copy age.key from your other machine")
			} else {
				row(true, false, "age key", keyPath, "")
			}

			b := daemon.Detect()
			row(daemon.Status(b), false, "daemon", string(b)+" schedule", "run `pine daemon install`")

			if cfgErr == nil {
				row(fresh(cfg), false, "freshness", "recent sync", "run `pine sync --now`; if it keeps lapsing check `pine daemon status` and daemon.log")
			}
			if failed {
				return errors.New(ui.Bad("critical checks failed"))
			}
			return nil
		},
	}
}

// fresh returns true when the last push and pull are within 2x their intervals.
func fresh(cfg config.Config) bool {
	st, _ := config.LoadState()
	if st.LastPush.IsZero() || time.Since(st.LastPush) > 2*time.Duration(cfg.PushIntervalHours)*time.Hour {
		return false
	}
	fi, err := os.Stat(paths.StateFile())
	return err == nil && time.Since(fi.ModTime()) <= 2*time.Duration(cfg.PullIntervalMinutes)*time.Minute
}
