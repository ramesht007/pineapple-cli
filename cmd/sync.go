package cmd

import (
	"time"

	"github.com/spf13/cobra"
)

func syncCmd() *cobra.Command {
	var now bool
	c := &cobra.Command{
		Use:   "sync",
		Short: "Pull, then push if the push interval has elapsed",
		Args:  cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			cfg, st, err := load()
			if err != nil {
				return err
			}
			if err := pullFlow(cfg, &st); err != nil {
				return err
			}
			due := time.Duration(cfg.PushIntervalHours) * time.Hour
			if now || st.LastPush.IsZero() || time.Since(st.LastPush) >= due {
				return pushFlow(cfg, &st, false)
			}
			return nil
		},
	}
	c.Flags().BoolVar(&now, "now", false, "push even if the push interval has not elapsed")
	return c
}
