package cli

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"

	fkexec "github.com/gridlhq/yeager/internal/exec"
	"github.com/gridlhq/yeager/internal/output"
	"github.com/spf13/cobra"
)

func newLogsCmd(f *flags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "logs [run-id]",
		Short: "Replay and stream output from the last (or specified) run",
		Long: `Replays all output from a run, then streams if still active (like docker logs -f).
If the run is still active on the VM, streams live from the tmux session.
If the run is complete, replays from S3.`,
		Example: `  yg logs              # replay the most recent run
  yg logs 007          # replay run 007
  yg logs --tail 50    # last 50 lines, then stream if active`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cc, err := resolveCmdContext(cmd.Context(), f.outputMode())
			if err != nil {
				return err
			}
			tail, _ := cmd.Flags().GetInt("tail")
			var runID string
			if len(args) > 0 {
				if err := fkexec.ValidateRunID(args[0]); err != nil {
					return err
				}
				runID = args[0]
			}
			return RunLogs(cmd.Context(), cc, runID, tail)
		},
	}

	cmd.Flags().Int("tail", 0, "show last N lines then stream")

	return cmd
}

// RunLogs replays and streams output from a run.
// If the run is still active (tmux session alive), streams live from the VM.
// If the run is complete, replays from S3.
func RunLogs(ctx context.Context, cc *cmdContext, runID string, tail int) error {
	w := cc.Output
	w.Infof("project: %s", cc.Project.DisplayName)

	// Resolve run ID.
	if runID == "" {
		lastRun, err := cc.State.LoadLastRun(cc.Project.Hash)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				w.Info("no previous runs found")
				w.Info("run a command first: yg <command>")
				return nil
			}
			return fmt.Errorf("loading last run: %w", err)
		}
		runID = lastRun
	}

	// Try live streaming from the VM if the run is still active.
	if streamed := tryLiveStream(ctx, cc, runID); streamed {
		return nil
	}

	// Fall back to S3 replay.
	return replayFromS3(ctx, cc, runID, tail)
}

// tryLiveStream attempts to stream output from an active tmux session on the VM.
// Returns true if live streaming was performed (regardless of whether it completed cleanly).
func tryLiveStream(ctx context.Context, cc *cmdContext, runID string) bool {
	w := cc.Output

	// Check if we have a VM.
	vmState, err := cc.State.LoadVM(cc.Project.Hash)
	if err != nil {
		slog.Debug("logs: no VM state, falling back to S3", "error", err)
		return false
	}

	info, err := cc.Provider.FindVM(ctx, cc.Project.Hash)
	if err != nil || info == nil || info.State != "running" {
		slog.Debug("logs: VM not running, falling back to S3",
			"instance", vmState.InstanceID, "err", err)
		return false
	}

	// Connect via SSH.
	client, err := cc.ConnectSSH(ctx, info)
	if err != nil {
		slog.Debug("logs: SSH connection failed, falling back to S3", "error", err)
		return false
	}
	if client != nil {
		defer client.Close()
	}

	// Check if the tmux session is still active.
	isRunActive := cc.IsRunActive
	if isRunActive == nil {
		isRunActive = fkexec.IsRunActive
	}

	active, err := isRunActive(client, fkexec.RunID(runID))
	if err != nil || !active {
		slog.Debug("logs: run not active, falling back to S3", "run_id", runID, "err", err)
		return false
	}

	// Stream live from the tmux log.
	w.Infof("streaming run %s (live)", runID)
	w.Separator()

	tailLog := cc.TailLog
	if tailLog == nil {
		tailLog = fkexec.TailLog
	}

	if err := tailLog(client, fkexec.RunID(runID), newStreamWriter(w)); err != nil {
		slog.Debug("logs: live stream ended", "error", err)
	}

	w.Separator()
	w.Info("stream ended")
	return true
}

// replayFromS3 downloads and replays output from S3.
func replayFromS3(ctx context.Context, cc *cmdContext, runID string, tail int) error {
	w := cc.Output
	w.Infof("replaying run %s", runID)

	store, err := cc.NewStorage(ctx)
	if err != nil {
		return fmt.Errorf("connecting to storage: %w", err)
	}

	data, err := store.DownloadOutput(ctx, cc.Project.DisplayName, runID)
	if err != nil {
		return fmt.Errorf("downloading output: %w", err)
	}

	// Apply --tail if specified.
	content := string(data)
	if tail > 0 {
		trimmed := strings.TrimRight(content, "\n")
		lines := strings.Split(trimmed, "\n")
		if len(lines) > tail {
			lines = lines[len(lines)-tail:]
		}
		content = strings.Join(lines, "\n")
	}

	w.Separator()
	w.Stream([]byte(content))
	w.Separator()

	// Show metadata (best-effort — may not exist for interrupted runs).
	meta, err := store.DownloadMeta(ctx, cc.Project.DisplayName, runID)
	if err == nil {
		w.Infof("command: %s", meta.Command)
		w.Infof("exit %d in %s", meta.ExitCode, meta.Duration)
	} else {
		slog.Debug("meta.json not available", "run_id", runID, "error", err)
	}

	return nil
}

// streamWriter wraps output.Writer to implement io.Writer for live streaming.
type streamWriter struct {
	w *output.Writer
}

func newStreamWriter(w *output.Writer) *streamWriter {
	return &streamWriter{w: w}
}

func (s *streamWriter) Write(p []byte) (n int, err error) {
	s.w.Stream(p)
	return len(p), nil
}
