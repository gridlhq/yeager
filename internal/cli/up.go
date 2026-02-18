package cli

import (
	"context"
	"fmt"

	fkexec "github.com/gridlhq/yeager/internal/exec"
	"github.com/spf13/cobra"
)

func newUpCmd(f *flags) *cobra.Command {
	var keepAlive bool

	cmd := &cobra.Command{
		Use:   "up",
		Short: "Boot the VM and sync files without running a command",
		Long: `Creates or starts the VM and syncs your project files, but does not run
a command. Useful for warming up the VM before you need it.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cc, err := resolveCmdContext(cmd.Context(), f.outputMode())
			if err != nil {
				return err
			}
			return RunUp(cmd.Context(), cc, keepAlive)
		},
	}

	cmd.Flags().BoolVar(&keepAlive, "keep-alive", false, "keep VM running until stopped manually (Ctrl+C or yg stop)")

	return cmd
}

// RunUp boots the VM (creating or starting as needed).
// If keepAlive is true, keeps the VM running until Ctrl+C or manual stop.
func RunUp(ctx context.Context, cc *cmdContext, keepAlive bool) error {
	w := cc.Output
	w.Infof("project: %s", cc.Project.DisplayName)

	// Cancel any grace period monitor since user is explicitly starting the VM.
	cancelGracePeriodMonitorBestEffort(cc)

	vmInfo, _, err := ensureVMRunning(ctx, cc)
	if err != nil {
		return err
	}

	if !keepAlive {
		return nil
	}

	// Keep-alive mode: start idle monitor and wait until user stops.
	idleTimeout, err := cc.Config.Lifecycle.IdleStopDuration()
	if err != nil || idleTimeout <= 0 {
		w.Success("VM is up")
		w.Hint("press Ctrl+C to return (VM stays running)")
		<-ctx.Done()
		return nil
	}

	listRuns := cc.ListRuns
	if listRuns == nil {
		listRuns = fkexec.ListRuns
	}

	monitor := NewIdleMonitor(IdleMonitorOpts{
		IdleTimeout: idleTimeout,
		InstanceID:  vmInfo.InstanceID,
		Provider:    cc.Provider,
		ConnectSSH:  cc.ConnectSSH,
		ListRuns:    listRuns,
		VMInfo:      vmInfo,
	})

	if monitor == nil {
		w.Success("VM is up")
		w.Hint("press Ctrl+C to return (VM stays running)")
		<-ctx.Done()
		return nil
	}

	w.Success("VM is up and will run until stopped manually or idle")
	w.Hint(fmt.Sprintf("auto-stop after %s of inactivity (change: .yeager.toml lifecycle.idle_stop)", formatDuration(idleTimeout)))
	w.Hint("press Ctrl+C to return (VM keeps running)")

	done := monitor.Start(ctx)

	select {
	case <-done:
		w.Info("VM stopped (idle timeout)")
	case <-ctx.Done():
		w.Info("detached (VM still running)")
	}

	return nil
}
