package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/ramesht007/pineapple-cli/internal/paths"
	"github.com/ramesht007/pineapple-cli/internal/repo"
	"github.com/ramesht007/pineapple-cli/internal/syncer"
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
			remote := "unknown"
			if ahead, err := repo.RemoteAhead(paths.RepoDir()); err == nil {
				remote = fmt.Sprint(ahead)
			}
			last := "never"
			if !st.LastPush.IsZero() {
				last = st.LastPush.Format("2006-01-02 15:04:05 MST")
			}
			fmt.Printf("machine:        %s\nrepo:           %s\nlast push:      %s\nlocal modified: %d file(s)\nremote ahead:   %s\n",
				cfg.Machine, cfg.Repo, last, modified, remote)
			return nil
		},
	}
}
