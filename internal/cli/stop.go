package cli

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func newStopCmd(f *flags) *cobra.Command {
	return &cobra.Command{
		Use:   "stop",
		Short: "Stop the VM (keeps it for fast restart later)",
		Long: `Stops the VM but keeps the EBS volume so it can restart quickly.
No compute charges while stopped. Use "yg up" or run any command to restart.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cc, err := resolveCmdContext(cmd.Context(), f.outputMode())
			if err != nil {
				return err
			}
			return RunStop(cmd.Context(), cc)
		},
	}
}

// RunStop stops the VM for the current project.
func RunStop(ctx context.Context, cc *cmdContext) error {
	w := cc.Output
	w.Infof("project: %s", cc.Project.DisplayName)

	vmState, err := cc.State.LoadVM(cc.Project.Hash)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			w.Info("no VM found — nothing to stop")
			return nil
		}
		return fmt.Errorf("loading VM state: %w", err)
	}

	info, err := cc.Provider.FindVM(ctx, cc.Project.Hash)
	if err != nil {
		return fmt.Errorf("querying VM state: %w", err)
	}

	if info == nil {
		w.Infof("VM %s no longer exists in AWS", vmState.InstanceID)
		return nil
	}

	if info.State == "stopped" {
		w.Infof("VM %s is already stopped", info.InstanceID)
		return nil
	}

	if info.State != "running" && info.State != "pending" {
		w.Infof("VM %s is in state %s — cannot stop", info.InstanceID, info.State)
		return nil
	}

	w.Infof("stopping VM %s...", info.InstanceID)
	if err := cc.Provider.StopVM(ctx, info.InstanceID); err != nil {
		return err
	}

	w.Info("VM stopped — restart with: yg up")
	return nil
}
