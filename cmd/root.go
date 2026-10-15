// Package cmd wires the pine cobra commands.
package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/ramesht007/pineapple-cli/internal/config"
	"github.com/ramesht007/pineapple-cli/internal/ui"
)

// version is overridden at build time: -ldflags "-X github.com/ramesht007/pineapple-cli/cmd.version=1.2.3".
var version = "dev"

var quiet bool

func Execute() {
	root := &cobra.Command{
		Use:              "pine",
		Short:            "Sync Claude Code user config across machines via a GitHub repo",
		SilenceUsage:     true,
		SilenceErrors:    true,
		PersistentPreRun: func(*cobra.Command, []string) { ui.Quiet = quiet },
		Run: func(c *cobra.Command, _ []string) {
			ui.Quiet = quiet
			if v, _ := c.Flags().GetBool("version"); v {
				printVersion()
				return
			}
			ui.ShowBanner(version)
			_ = c.Help()
		},
	}
	root.Flags().Bool("version", false, "print version")
	root.PersistentFlags().BoolVarP(&quiet, "quiet", "q", false, "only print errors and conflicts")
	root.AddCommand(initCmd(), pushCmd(), pullCmd(), syncCmd(), statusCmd(), daemonCmd(), doctorCmd(), secretsCmd(), versionCmd())
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "pine:", err)
		os.Exit(1)
	}
}

func versionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the pine version",
		Args:  cobra.NoArgs,
		Run: func(*cobra.Command, []string) {
			ui.Quiet = quiet
			printVersion()
		},
	}
}

// printVersion shows the banner on a TTY, else a plain version line.
func printVersion() {
	if ui.IsStdoutTTY() && !quiet {
		ui.ShowBanner(version)
		return
	}
	fmt.Println("pine version", version)
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
