package cli

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/gridlhq/yeager/internal/output"
	"github.com/gridlhq/yeager/internal/provider"
	"github.com/spf13/cobra"
)

// statusJSON is the structured output for `yg status --json`.
type statusJSON struct {
	State            string `json:"state"`
	InstanceID       string `json:"instance_id,omitempty"`
	ID               string `json:"id,omitempty"`
	Region           string `json:"region,omitempty"`
	InstanceType     string `json:"instance_type,omitempty"`
	AvailabilityZone string `json:"availability_zone,omitempty"`
	PublicIP         string `json:"public_ip,omitempty"`
	Project          string `json:"project,omitempty"`
}

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
	if cc.Output.Mode() == output.ModeJSON {
		return runStatusJSON(ctx, cc)
	}

	w := cc.Output
	w.Infof("project: %s", cc.Project.DisplayName)

	// Show AWS credential status (best-effort).
	showAWSCredentialStatus(ctx, cc)

	// Check local state first.
	vmState, err := cc.State.LoadVM(cc.Project.Hash)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			w.Info("no VM found")
			w.Hint("run a command to create one: yg <command>")
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
		w.Hint("run a command to create a new one: yg <command>")
		return nil
	}

	// Display live state with cost information.
	vmSize := cc.Config.Compute.Size
	cost := provider.CostPerHour(vmSize)
	costStr := ""
	if cost > 0.0 {
		costStr = fmt.Sprintf(", %s", provider.FormatCost(cost))
	}

	switch info.State {
	case "running":
		stateStr := stateIndicator("running", w.ColorOut())
		if info.PublicIP != "" {
			w.Infof("VM: %s %s  %s  %s  %s%s", info.InstanceID, stateStr, info.Region, vmSize, info.PublicIP, costStr)
		} else {
			w.Infof("VM: %s %s  %s  %s%s", info.InstanceID, stateStr, info.Region, vmSize, costStr)
		}

		// Show active commands (best-effort via SSH).
		showActiveCommands(ctx, cc, info)

	case "stopped":
		w.Infof("VM: %s %s  %s", info.InstanceID, stateIndicator("stopped", w.ColorOut()), info.Region)
		w.Hint("start it with: yg up")
	case "pending":
		w.Infof("VM: %s %s  %s", info.InstanceID, stateIndicator("pending", w.ColorOut()), info.Region)
	case "stopping":
		w.Infof("VM: %s %s  %s", info.InstanceID, stateIndicator("stopping", w.ColorOut()), info.Region)
	default:
		w.Infof("VM: %s %s  %s", info.InstanceID, stateIndicator(info.State, w.ColorOut()), info.Region)
	}

	// Show recent run history from local state.
	showRunHistory(cc)

	return nil
}

// runStatusJSON outputs structured JSON for `yg status --json`.
func runStatusJSON(ctx context.Context, cc *cmdContext) error {
	s := statusJSON{
		Project: cc.Project.DisplayName,
	}

	// Check local state.
	vmState, err := cc.State.LoadVM(cc.Project.Hash)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			s.State = "none"
			return cc.Output.WriteJSON(s)
		}
		return fmt.Errorf("loading VM state: %w", err)
	}

	// Query AWS for live state.
	info, err := cc.Provider.FindVM(ctx, cc.Project.Hash)
	if err != nil {
		return fmt.Errorf("querying VM state: %w", err)
	}

	if info == nil {
		s.State = "terminated"
		s.InstanceID = vmState.InstanceID
		s.ID = vmState.InstanceID
		s.Region = vmState.Region
		return cc.Output.WriteJSON(s)
	}

	s.State = info.State
	s.InstanceID = info.InstanceID
	s.ID = info.InstanceID
	s.Region = info.Region
	s.InstanceType = info.InstanceType
	s.AvailabilityZone = info.AvailabilityZone
	s.PublicIP = info.PublicIP

	return cc.Output.WriteJSON(s)
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
		showIdleStatus(cc)
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

// showAWSCredentialStatus displays AWS credential status (best-effort).
func showAWSCredentialStatus(ctx context.Context, cc *cmdContext) {
	if cc.CheckAWSCredStatus == nil {
		return
	}

	checkCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	accountID, err := cc.CheckAWSCredStatus(checkCtx)
	if err != nil {
		slog.Debug("status: AWS credential check failed", "error", err)
		cc.Output.Info("AWS: not verified")
		return
	}
	cc.Output.Infof("AWS: ok (account %s)", accountID)
}

// showIdleStatus displays information about auto-stop timers and how to configure them.
func showIdleStatus(cc *cmdContext) {
	w := cc.Output

	gracePeriod, err := cc.Config.Lifecycle.GracePeriodDuration()
	if err == nil && gracePeriod > 0 {
		w.Infof("  auto-stop: %s after command completes (change: lifecycle.grace_period in .yeager.toml)", formatDuration(gracePeriod))
	}

	idleStop, err := cc.Config.Lifecycle.IdleStopDuration()
	if err == nil && idleStop > 0 {
		w.Infof("  idle timeout: %s (change: lifecycle.idle_stop in .yeager.toml)", formatDuration(idleStop))
	}

	if gracePeriod <= 0 && idleStop <= 0 {
		w.Info("  no auto-stop configured (VM will run until manually stopped)")
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
		exitStr := fmt.Sprintf("exit %d", entry.ExitCode)
		if w.ColorOut() {
			if entry.ExitCode == 0 {
				exitStr = output.SuccessStyle.Render(exitStr)
			} else {
				exitStr = output.ErrorStyle.Render(exitStr)
			}
		}
		w.Infof("  %s  %s  %s  %s", entry.RunID, exitStr, dur, entry.Command)
	}
}

// stateIndicator returns a colored state indicator string.
func stateIndicator(state string, color bool) string {
	// Map AWS state names to human-friendly labels.
	label := state
	switch state {
	case "pending":
		label = "starting"
	case "stopping":
		label = "stopping"
	}

	if !color {
		return fmt.Sprintf("(%s)", label)
	}
	switch state {
	case "running":
		return output.SuccessStyle.Render("● running")
	case "stopped":
		return output.DimStyle.Render("○ stopped")
	case "pending":
		return output.YellowStyle.Render("◐ starting")
	case "stopping":
		return output.YellowStyle.Render("◑ stopping")
	default:
		return output.DimStyle.Render("○ " + state)
	}
}
