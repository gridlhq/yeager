package cli

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/gridlhq/yeager/internal/monitor"
	"github.com/spf13/cobra"
)

func newDestroyCmd(f *flags) *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "destroy",
		Short: "Terminate the VM and clean up all resources",
		Long: `Terminates the VM, deletes the EBS volume, and removes local state.
The next command will create a fresh VM from scratch. S3 output history
is not affected.

Use --force to skip the confirmation warning.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cc, err := resolveCmdContext(cmd.Context(), f.outputMode())
			if err != nil {
				return err
			}
			return RunDestroyWithOptions(cmd.Context(), cc, DestroyOptions{Force: force})
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "Skip confirmation warning")
	return cmd
}

// DestroyOptions controls destroy behavior.
type DestroyOptions struct {
	Force bool // Skip confirmation warning
}

// RunDestroy terminates the VM and deletes local state (backward-compatible, always forces).
func RunDestroy(ctx context.Context, cc *cmdContext) error {
	return RunDestroyWithOptions(ctx, cc, DestroyOptions{Force: true})
}

// RunDestroyWithOptions terminates the VM and deletes local state.
// Without Force, shows a warning about data loss and exits without destroying.
func RunDestroyWithOptions(ctx context.Context, cc *cmdContext, opts DestroyOptions) error {
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

	if !opts.Force {
		w.Warn("destroying this VM will permanently delete:", "")
		w.Info("  - cached build artifacts")
		w.Info("  - installed packages and toolchains")
		w.Info("  - accumulated state from previous runs")
		w.Info("")
		w.Info("your run history and logs in S3 are not affected.")
		w.Hint("run again with --force to proceed")
		return nil
	}

	// Stop any running monitor daemon before destroying.
	m := monitor.New(cc.Project.Hash, cc.State, cc.Provider, 0) // grace period doesn't matter for Stop
	if err := m.Stop(); err != nil {
		// Log but don't fail - we still want to destroy even if monitor cleanup fails.
		w.Warn(fmt.Sprintf("failed to stop monitor daemon: %v", err), "")
	}

	// Try to terminate in AWS (best-effort — might already be gone).
	info, err := cc.Provider.FindVM(ctx, cc.Project.Hash)
	if err != nil {
		return fmt.Errorf("querying VM state: %w", err)
	}

	if info != nil {
		w.StartSpinner(fmt.Sprintf("terminating VM %s...", info.InstanceID))
		if err := cc.Provider.TerminateVM(ctx, info.InstanceID); err != nil {
			w.StopSpinner("failed to terminate VM", false)
			return err
		}
		w.StopSpinner(fmt.Sprintf("terminated VM %s", info.InstanceID), true)
	} else {
		w.Infof("VM %s no longer exists in AWS", vmState.InstanceID)
	}

	// Clean up local state.
	if err := cc.State.DeleteVM(cc.Project.Hash); err != nil {
		return fmt.Errorf("cleaning up local state: %w", err)
	}

	w.Success("VM destroyed and local state cleaned up")
	return nil
}
