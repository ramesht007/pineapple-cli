// Package cmd wires the pine cobra commands.
package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/ramesht007/pineapple-cli/internal/config"
)

// version is overridden at build time: -ldflags "-X github.com/ramesht007/pineapple-cli/cmd.version=1.2.3".
var version = "dev"

var quiet bool

func Execute() {
	root := &cobra.Command{
		Use:           "pine",
		Short:         "Sync Claude Code user config across machines via a GitHub repo",
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.PersistentFlags().BoolVarP(&quiet, "quiet", "q", false, "only print errors and conflicts")
	root.AddCommand(initCmd(), pushCmd(), pullCmd(), syncCmd(), statusCmd())
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "pine:", err)
		os.Exit(1)
	}
}

// progress prints a one-line progress message to stderr unless --quiet.
func progress(format string, a ...any) {
	if !quiet {
		fmt.Fprintf(os.Stderr, format+"\n", a...)
	}
}

func load() (config.Config, config.State, error) {
	cfg, err := config.Load()
	if err != nil {
		return cfg, config.State{}, err
	}
	st, err := config.LoadState()
	return cfg, st, err
}
