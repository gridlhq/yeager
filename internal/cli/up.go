package cli

import (
	"context"

	"github.com/spf13/cobra"
)

func newUpCmd(f *flags) *cobra.Command {
	return &cobra.Command{
		Use:   "up",
		Short: "Boot the VM and sync files without running a command",
		Long: `Creates or starts the VM and syncs your project files, but does not run
a command. Useful for warming up the VM before you need it.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cc, err := resolveCmdContext(cmd.Context(), f.outputMode())
			if err != nil {
				return err
			}
			return RunUp(cmd.Context(), cc)
		},
	}
}

// RunUp boots the VM (creating or starting as needed).
func RunUp(ctx context.Context, cc *cmdContext) error {
	cc.Output.Infof("project: %s", cc.Project.DisplayName)
	_, _, err := ensureVMRunning(ctx, cc)
	return err
}
