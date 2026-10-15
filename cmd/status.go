package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/ramesht007/pineapple-cli/internal/manifest"
	"github.com/ramesht007/pineapple-cli/internal/paths"
	"github.com/ramesht007/pineapple-cli/internal/repo"
	"github.com/ramesht007/pineapple-cli/internal/syncer"
	"github.com/ramesht007/pineapple-cli/internal/ui"
)

func statusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show sync status",
		Args:  cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			cfg, st, err := load()
			if err != nil {
				return err
			}
			local, err := syncer.LocalHashes(paths.ClaudeSkills(), paths.ClaudeMods())
			if err != nil {
				return err
			}
			modified := 0
			for rel, h := range local {
				if st.Files[rel] != h {
					modified++
				}
			}
			for rel := range st.Files {
				if _, ok := local[rel]; !ok {
					modified++
				}
			}
			remote := ui.Dim("unknown")
			if ahead, err := repo.RemoteAhead(paths.RepoDir()); err == nil {
				remote = ui.Dim("no")
				if ahead {
					remote = ui.Warn("yes")
				}
			}
			mod := fmt.Sprintf("%d file(s)", modified)
			if modified > 0 {
				mod = ui.Warn(mod)
			}
			row := func(k, v string) { fmt.Printf("%s %s\n", ui.Dim(fmt.Sprintf("%-15s", k+":")), v) }
			row("machine", cfg.Machine)
			row("repo", cfg.Repo)
			row("last push", ui.RelTime(st.LastPush))
			row("last pull", ui.RelTime(st.LastPull))
			row("local modified", mod)
			row("remote ahead", remote)
			row("tracked", tracked(paths.RepoDir()))
			return nil
		},
	}
}

// tracked summarises what the local clone holds: skill/mod dirs and manifest plugins.
func tracked(repoDir string) string {
	count := func(sub string) int {
		es, _ := os.ReadDir(filepath.Join(repoDir, sub))
		n := 0
		for _, e := range es {
			if e.IsDir() && e.Name() != "synced" && e.Name()[0] != '.' {
				n++
			}
		}
		return n
	}
	var m manifest.Manifest
	if b, err := os.ReadFile(filepath.Join(repoDir, "manifest.json")); err == nil {
		_ = json.Unmarshal(b, &m)
	}
	return fmt.Sprintf("%d skills · %d mods · %d plugins", count("skills"), count("mods"), len(m.Plugins))
}
