package cli

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func newDestroyCmd(f *flags) *cobra.Command {
	return &cobra.Command{
		Use:   "destroy",
		Short: "Terminate the VM and clean up all resources",
		Long: `Terminates the VM, deletes the EBS volume, and removes local state.
The next command will create a fresh VM from scratch. S3 output history
is not affected.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cc, err := resolveCmdContext(cmd.Context(), f.outputMode())
			if err != nil {
				return err
			}
			return RunDestroy(cmd.Context(), cc)
		},
	}
}

// RunDestroy terminates the VM and deletes local state.
func RunDestroy(ctx context.Context, cc *cmdContext) error {
	w := cc.Output
	w.Infof("project: %s", cc.Project.DisplayName)

	vmState, err := cc.State.LoadVM(cc.Project.Hash)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			w.Info("no VM found — nothing to destroy")
			return nil
		}
		return fmt.Errorf("loading VM state: %w", err)
	}

	// Try to terminate in AWS (best-effort — might already be gone).
	info, err := cc.Provider.FindVM(ctx, cc.Project.Hash)
	if err != nil {
		return fmt.Errorf("querying VM state: %w", err)
	}

	if info != nil {
		w.Infof("terminating VM %s...", info.InstanceID)
		if err := cc.Provider.TerminateVM(ctx, info.InstanceID); err != nil {
			return err
		}
	} else {
		w.Infof("VM %s no longer exists in AWS", vmState.InstanceID)
	}

	// Clean up local state.
	if err := cc.State.DeleteVM(cc.Project.Hash); err != nil {
		return fmt.Errorf("cleaning up local state: %w", err)
	}

	w.Info("VM destroyed and local state cleaned up")
	return nil
}
