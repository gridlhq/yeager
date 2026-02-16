package cli

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/gridlhq/yeager/internal/provider"
	"github.com/spf13/cobra"
)

func newStatusCmd(f *flags) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show VM state, active commands, and recent history",
		Long: `Shows the current state of the project's VM, any commands actively
running on it, and a summary of recent completed runs.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cc, err := resolveCmdContext(cmd.Context(), f.outputMode())
			if err != nil {
				return err
			}
			return RunStatus(cmd.Context(), cc)
		},
	}
}

// RunStatus shows the VM state for the current project.
func RunStatus(ctx context.Context, cc *cmdContext) error {
	w := cc.Output
	w.Infof("project: %s", cc.Project.DisplayName)

	// Check local state first.
	vmState, err := cc.State.LoadVM(cc.Project.Hash)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			w.Info("no VM found")
			w.Info("run a command to create one: yg <command>")
			return nil
		}
		return fmt.Errorf("loading VM state: %w", err)
	}

	// Query AWS for live state.
	info, err := cc.Provider.FindVM(ctx, cc.Project.Hash)
	if err != nil {
		return fmt.Errorf("querying VM state: %w", err)
	}

	if info == nil {
		w.Infof("VM %s no longer exists in AWS", vmState.InstanceID)
		w.Info("run a command to create a new one: yg <command>")
		return nil
	}

	// Display live state.
	switch info.State {
	case "running":
		if info.PublicIP != "" {
			w.Infof("VM: %s (running, %s, %s)", info.InstanceID, info.Region, info.PublicIP)
		} else {
			w.Infof("VM: %s (running, %s)", info.InstanceID, info.Region)
		}

		// Show active commands (best-effort via SSH).
		showActiveCommands(ctx, cc, info)

	case "stopped":
		w.Infof("VM: %s (stopped, %s)", info.InstanceID, info.Region)
		w.Info("start it with: yg up")
	case "pending":
		w.Infof("VM: %s (starting, %s)", info.InstanceID, info.Region)
	case "stopping":
		w.Infof("VM: %s (stopping, %s)", info.InstanceID, info.Region)
	default:
		w.Infof("VM: %s (%s, %s)", info.InstanceID, info.State, info.Region)
	}

	// Show recent run history from local state.
	showRunHistory(cc)

	return nil
}

// showActiveCommands SSHs into the VM to list active runs.
// Best-effort — failures are logged but not returned.
func showActiveCommands(ctx context.Context, cc *cmdContext, vmInfo *provider.VMInfo) {
	w := cc.Output

	if cc.ConnectSSH == nil || cc.ListRuns == nil {
		return
	}

	client, err := cc.ConnectSSH(ctx, vmInfo)
	if err != nil {
		slog.Debug("status: SSH connection failed (best-effort)", "error", err)
		return
	}
	if client != nil {
		defer client.Close()
	}

	runs, err := cc.ListRuns(client)
	if err != nil {
		slog.Debug("status: listing runs failed (best-effort)", "error", err)
		return
	}

	if len(runs) == 0 {
		w.Info("no active commands")
		return
	}

	now := time.Now().UTC()
	w.Info("")
	w.Infof("active commands: %d", len(runs))
	for _, run := range runs {
		elapsed := ""
		if !run.StartTime.IsZero() {
			elapsed = fmt.Sprintf(" (%s)", formatDuration(now.Sub(run.StartTime).Truncate(time.Second)))
		}
		cmd := run.Command
		if cmd == "" {
			cmd = "(unknown)"
		}
		w.Infof("  %s  %s%s", run.RunID, cmd, elapsed)
	}
}

// showRunHistory displays recent completed runs from local history.
func showRunHistory(cc *cmdContext) {
	w := cc.Output

	history, err := cc.State.LoadRunHistory(cc.Project.Hash)
	if err != nil {
		slog.Debug("status: loading history failed", "error", err)
		return
	}

	if len(history) == 0 {
		return
	}

	w.Info("")
	// Show last 5 entries.
	start := 0
	if len(history) > 5 {
		start = len(history) - 5
	}
	w.Info("recent runs:")
	for _, entry := range history[start:] {
		dur := formatDuration(entry.Duration.Truncate(time.Second))
		w.Infof("  %s  exit %d  %s  %s", entry.RunID, entry.ExitCode, dur, entry.Command)
	}
}
