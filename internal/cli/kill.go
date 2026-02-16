package cli

import (
	"context"
	"errors"
	"fmt"
	"os"

	fkexec "github.com/gridlhq/yeager/internal/exec"
	"github.com/spf13/cobra"
)

func newKillCmd(f *flags) *cobra.Command {
	return &cobra.Command{
		Use:   "kill [run-id]",
		Short: "Cancel the most recent (or specified) command on the VM",
		Long: `Cancels a running command by killing its tmux session on the VM.
Without a run ID, kills the most recent command. Use "yg status" to see
active run IDs.`,
		Example: `  yg kill              # cancel the most recent command
  yg kill 007          # cancel run 007`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cc, err := resolveCmdContext(cmd.Context(), f.outputMode())
			if err != nil {
				return err
			}
			var runID string
			if len(args) > 0 {
				if err := fkexec.ValidateRunID(args[0]); err != nil {
					return err
				}
				runID = args[0]
			}
			return RunKill(cmd.Context(), cc, runID)
		},
	}
}

// RunKill cancels a running command on the VM.
func RunKill(ctx context.Context, cc *cmdContext, runID string) error {
	w := cc.Output
	w.Infof("project: %s", cc.Project.DisplayName)

	// Resolve run ID.
	if runID == "" {
		lastRun, err := cc.State.LoadLastRun(cc.Project.Hash)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				w.Info("no active commands to cancel")
				return nil
			}
			return fmt.Errorf("loading last run: %w", err)
		}
		runID = lastRun
	}

	// Ensure VM is running and get connection info.
	vmState, err := cc.State.LoadVM(cc.Project.Hash)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			w.Info("no VM found — nothing to kill")
			return nil
		}
		return fmt.Errorf("loading VM state: %w", err)
	}

	info, err := cc.Provider.FindVM(ctx, cc.Project.Hash)
	if err != nil {
		return fmt.Errorf("querying VM state: %w", err)
	}
	if info == nil {
		w.Infof("VM %s no longer exists", vmState.InstanceID)
		return nil
	}
	if info.State != "running" {
		w.Infof("VM %s is %s — cannot kill commands", info.InstanceID, info.State)
		return nil
	}

	// Connect via SSH and kill the process.
	w.Infof("cancelling run %s...", runID)

	client, err := cc.ConnectSSH(ctx, info)
	if err != nil {
		return fmt.Errorf("SSH connection failed: %w", err)
	}
	if client != nil {
		defer client.Close()
	}

	if err := fkexec.Kill(client, fkexec.RunID(runID)); err != nil {
		return fmt.Errorf("killing command: %w", err)
	}

	w.Infof("run %s cancelled", runID)
	return nil
}
