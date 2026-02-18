package cli

import (
	"fmt"

	"github.com/gridlhq/yeager/internal/config"
	"github.com/gridlhq/yeager/internal/monitor"
	"github.com/spf13/cobra"
)

// newMonitorDaemonCmd creates the hidden monitor-daemon command.
// This is invoked by spawned background processes, not by users.
func newMonitorDaemonCmd() *cobra.Command {
	var (
		projectHash string
		stateDir    string
		gracePeriod string
	)

	cmd := &cobra.Command{
		Use:    "monitor-daemon",
		Hidden: true,
		Short:  "Background daemon for grace period monitoring (internal use only)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if projectHash == "" {
				return fmt.Errorf("--project-hash is required")
			}
			if stateDir == "" {
				return fmt.Errorf("--state-dir is required")
			}
			if gracePeriod == "" {
				return fmt.Errorf("--grace-period is required")
			}

			// Parse grace period duration.
			duration, err := config.ParseDuration(gracePeriod)
			if err != nil {
				return fmt.Errorf("invalid grace period: %w", err)
			}

			// Run the daemon.
			return monitor.RunDaemon(projectHash, stateDir, duration)
		},
	}

	cmd.Flags().StringVar(&projectHash, "project-hash", "", "Project hash")
	cmd.Flags().StringVar(&stateDir, "state-dir", "", "State directory")
	cmd.Flags().StringVar(&gracePeriod, "grace-period", "", "Grace period duration")

	return cmd
}
