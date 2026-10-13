package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/ramesht007/pineapple-cli/internal/config"
	"github.com/ramesht007/pineapple-cli/internal/daemon"
	"github.com/ramesht007/pineapple-cli/internal/paths"
)

func daemonLog() string { return filepath.Join(paths.PineappleHome(), "daemon.log") }

func daemonCmd() *cobra.Command {
	c := &cobra.Command{Use: "daemon", Short: "Run pine sync automatically in the background"}

	c.AddCommand(&cobra.Command{
		Use:   "install",
		Short: "Register an OS schedule that runs `pine sync --quiet`",
		Args:  cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			if !config.Exists() {
				return fmt.Errorf("run pine init first")
			}
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			exe, err := daemon.Exe()
			if err != nil {
				return err
			}
			b := daemon.Detect()
			if err := daemon.Install(b, exe, cfg.PullIntervalMinutes, daemonLog()); err != nil {
				return fmt.Errorf("install %s schedule: %w", b, err)
			}
			fmt.Printf("Installed %s schedule: `%s sync --quiet` every %d minutes (push happens every %d hours).\nLog: %s\n",
				b, exe, cfg.PullIntervalMinutes, cfg.PushIntervalHours, daemonLog())
			return nil
		},
	})

	c.AddCommand(&cobra.Command{
		Use:   "uninstall",
		Short: "Remove the OS schedule",
		Args:  cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			b := daemon.Detect()
			if err := daemon.Uninstall(b); err != nil {
				return fmt.Errorf("uninstall %s schedule: %w", b, err)
			}
			fmt.Printf("Removed %s schedule.\n", b)
			return nil
		},
	})

	c.AddCommand(&cobra.Command{
		Use:   "status",
		Short: "Show whether the schedule is active",
		Args:  cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			b := daemon.Detect()
			if daemon.Status(b) {
				fmt.Printf("active (%s)\n", b)
			} else {
				fmt.Printf("not installed (%s); run `pine daemon install`\n", b)
			}
			return nil
		},
	})

	c.AddCommand(&cobra.Command{
		Use:   "run",
		Short: "Run the sync loop in the foreground (debugging / systems without a scheduler)",
		Args:  cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			exe, err := daemon.Exe()
			if err != nil {
				return err
			}
			sig := make(chan os.Signal, 1)
			signal.Notify(sig, os.Interrupt)
			tick := time.NewTicker(time.Duration(cfg.PullIntervalMinutes) * time.Minute)
			defer tick.Stop()
			for {
				out, err := execSync(exe)
				if err != nil {
					fmt.Printf("%s sync failed: %v %s\n", time.Now().Format(time.RFC3339), err, out)
				} else {
					fmt.Printf("%s sync ok\n", time.Now().Format(time.RFC3339))
				}
				select {
				case <-sig:
					return nil
				case <-tick.C:
				}
			}
		},
	})
	return c
}

// execSync runs a child `pine sync --quiet` so one failed cycle cannot kill the loop.
func execSync(exe string) (string, error) {
	out, err := exec.Command(exe, "sync", "--quiet").CombinedOutput()
	return strings.TrimSpace(string(out)), err
}
