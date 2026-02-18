package cli

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/gridlhq/yeager/internal/monitor"
	"github.com/gridlhq/yeager/internal/output"
	"github.com/gridlhq/yeager/internal/provider"
	"github.com/gridlhq/yeager/internal/provision"
	"github.com/gridlhq/yeager/internal/state"

	fkexec "github.com/gridlhq/yeager/internal/exec"
	fkssh "github.com/gridlhq/yeager/internal/ssh"
	fkstorage "github.com/gridlhq/yeager/internal/storage"
	fksync "github.com/gridlhq/yeager/internal/sync"
	gossh "golang.org/x/crypto/ssh"
)

const remoteProjectDir = "/home/ubuntu/project"

// RunCommand executes a command on the remote VM.
// This is the core execution path: ensure VM → sync → execute → stream → upload.
// Returns the exit code from the remote command.
func RunCommand(ctx context.Context, cc *cmdContext, command string) (int, error) {
	w := cc.Output
	w.Infof("project: %s", cc.Project.DisplayName)

	// Step 0: Cancel any existing grace period monitor (new activity).
	// This is best-effort — if it fails, we still proceed with the command.
	cancelGracePeriodMonitor(cc)

	// Step 1: Ensure VM is running.
	vmInfo, freshVM, err := ensureVMRunning(ctx, cc)
	if err != nil {
		printError(w, err)
		return 1, displayed(err)
	}

	// Step 2: Sync files.
	w.StartSpinner("syncing files...")
	syncResult, err := cc.RunSync(ctx, cc, vmInfo)
	if err != nil {
		w.StopSpinner("sync failed", false)
		return 1, fmt.Errorf("syncing files: %w", err)
	}
	if syncResult != nil {
		w.StopSpinner(formatSyncResult(syncResult, freshVM), true)
	} else {
		w.StopSpinner("synced", true)
	}

	// Step 3: Establish SSH connection for command execution.
	if freshVM {
		w.StartSpinner("installing toolchain (first run)...")
	} else {
		w.StartSpinner("connecting...")
	}
	client, err := cc.ConnectSSH(ctx, vmInfo)
	if err != nil {
		w.StopSpinner("connection failed", false)
		if freshVM {
			w.Hint("the VM may still be provisioning — wait a minute and try again")
		}
		return 1, fmt.Errorf("SSH connection failed: %w", err)
	}
	w.StopSpinner("connected", true)
	if client != nil {
		defer client.Close()
	}

	// Step 4: Execute command.
	runID := fkexec.GenerateRunID()
	w.Infof("running: %s", command)
	w.Hint("Ctrl+C detaches — the command keeps running on the VM.")
	w.Separator()

	var stdoutBuf, stderrBuf bytes.Buffer
	stdoutWriter := newTeeWriter(w, &stdoutBuf)
	stderrWriter := newTeeWriter(w, &stderrBuf)

	startTime := time.Now().UTC()
	result, err := cc.RunExec(client, fkexec.RunOpts{
		Command: command,
		WorkDir: remoteProjectDir,
		RunID:   runID,
	}, stdoutWriter, stderrWriter)

	w.Separator()

	// Check for Ctrl+C (SIGINT) — detach from stream, don't kill remote process.
	if ctx.Err() != nil {
		elapsed := time.Since(startTime).Truncate(time.Second)
		w.Infof("detached (%s elapsed) — command still running on VM", formatDuration(elapsed))
		w.Hint("re-attach: yg logs    cancel: yg kill")
		// Save last run ID so yg logs can re-attach (best-effort).
		if err := cc.State.SaveLastRun(cc.Project.Hash, runID.String()); err != nil {
			slog.Debug("failed to save last run ID", "error", err)
		}
		return 0, nil
	}

	if err != nil {
		return 1, fmt.Errorf("running command: %w", err)
	}

	// Step 5: Report result and save last run ID.
	duration := result.Duration().Truncate(time.Second)
	if result.ExitCode == 0 {
		w.Success(fmt.Sprintf("done (exit 0) in %s", formatDuration(duration)))
	} else {
		w.Warn(fmt.Sprintf("done (exit %d) in %s", result.ExitCode, formatDuration(duration)), "")
	}

	// Save last run ID for `yg logs` (best-effort).
	if err := cc.State.SaveLastRun(cc.Project.Hash, runID.String()); err != nil {
		slog.Debug("failed to save last run ID", "error", err)
	}

	// Step 6: Save run history (best-effort).
	if err := cc.State.SaveRunHistory(cc.Project.Hash, state.RunHistoryEntry{
		RunID:     runID.String(),
		Command:   command,
		ExitCode:  result.ExitCode,
		StartTime: result.StartTime,
		Duration:  result.Duration(),
	}); err != nil {
		slog.Debug("failed to save run history", "error", err)
	}

	// Step 7: Upload output to S3 (best-effort).
	if err := uploadOutput(ctx, cc, runID, command, result, stdoutBuf.Bytes(), stderrBuf.Bytes()); err != nil {
		w.Warn(fmt.Sprintf("failed to upload output to S3: %s", err), "")
	} else {
		bucketName, err := cc.Provider.BucketName(ctx)
		if err != nil {
			w.Info("output uploaded to S3")
		} else {
			w.Infof("output saved: s3://%s/%s/%s/", bucketName, cc.Project.DisplayName, runID)
		}
	}

	// Step 7b: Upload artifacts to S3 (best-effort).
	if len(cc.Config.Artifacts.Paths) > 0 {
		uploaded := uploadArtifacts(ctx, cc, client, runID)
		if uploaded > 0 {
			w.Infof("uploaded %d artifact(s)", uploaded)
		}
	}

	// Step 8: Idle check — if no other commands are running, stop the VM.
	checkIdleAndStop(ctx, cc, vmInfo)

	return result.ExitCode, nil
}

// checkIdleAndStop checks if the VM should be stopped after a command finishes.
// If no other commands are running, starts a background monitor that will stop
// the VM after the grace period elapses. This keeps the VM "warm" for quick
// follow-up commands while automatically saving costs when idle.
// This is a best-effort operation — failures are logged, not returned.
func checkIdleAndStop(ctx context.Context, cc *cmdContext, vmInfo *provider.VMInfo) {
	gracePeriod, err := cc.Config.Lifecycle.GracePeriodDuration()
	if err != nil || gracePeriod <= 0 {
		// If no grace period configured, don't start monitor.
		return
	}

	// Check if there are active commands.
	listRuns := cc.ListRuns
	if listRuns == nil {
		listRuns = fkexec.ListRuns
	}

	client, err := cc.ConnectSSH(ctx, vmInfo)
	if err != nil {
		slog.Debug("checkIdleAndStop: SSH failed", "error", err)
		return
	}
	if client != nil {
		defer client.Close()
	}

	runs, err := listRuns(client)
	if err != nil {
		slog.Debug("checkIdleAndStop: list runs failed", "error", err)
		return
	}

	if len(runs) == 0 {
		// No active commands — start background monitor to stop VM after grace period.
		m := monitor.New(cc.Project.Hash, cc.State, cc.Provider, gracePeriod)
		if err := m.Start(); err != nil {
			slog.Warn("failed to start grace period monitor", "error", err)
			return
		}

		cc.Output.Infof("VM idle (auto-stopping in %s — run another command to cancel)", formatDuration(gracePeriod))
		cc.Output.Hint("change grace period: .yeager.toml lifecycle.grace_period")
	}
}

// cancelGracePeriodMonitor stops any running grace period monitor for this project.
// This should be called at the start of every command to cancel pending auto-stop.
// This is best-effort — failures are logged, not returned.
func cancelGracePeriodMonitor(cc *cmdContext) {
	gracePeriod, err := cc.Config.Lifecycle.GracePeriodDuration()
	if err != nil || gracePeriod <= 0 {
		return
	}

	m := monitor.New(cc.Project.Hash, cc.State, cc.Provider, gracePeriod)
	if err := m.Stop(); err != nil {
		slog.Debug("failed to stop grace period monitor", "error", err)
	}
}

// cancelGracePeriodMonitorBestEffort cancels any grace period monitor.
// Unlike cancelGracePeriodMonitor, this doesn't require a grace period config.
// Useful for explicit stop/up commands where we want to cancel regardless.
func cancelGracePeriodMonitorBestEffort(cc *cmdContext) {
	// Try to get grace period from config, but proceed even if not configured.
	gracePeriod, err := cc.Config.Lifecycle.GracePeriodDuration()
	if err != nil || gracePeriod <= 0 {
		gracePeriod = 1 // dummy value for monitor creation
	}

	m := monitor.New(cc.Project.Hash, cc.State, cc.Provider, gracePeriod)
	if err := m.Stop(); err != nil {
		slog.Debug("failed to stop grace period monitor", "error", err)
	}
}

// ensureVMRunning makes sure the VM is running, creating or starting as needed.
// Returns the VM info and whether a fresh VM was just created.
func ensureVMRunning(ctx context.Context, cc *cmdContext) (*provider.VMInfo, bool, error) {
	w := cc.Output

	vmState, err := cc.State.LoadVM(cc.Project.Hash)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, false, fmt.Errorf("loading VM state: %w", err)
	}

	hasLocalState := err == nil

	if hasLocalState {
		info, err := cc.Provider.FindVM(ctx, cc.Project.Hash)
		if err != nil {
			return nil, false, fmt.Errorf("querying VM state: %w", err)
		}

		if info != nil {
			switch info.State {
			case "running":
				// Check for outdated cloud-init version.
				if vmState.CloudInitVersion != 0 && vmState.CloudInitVersion != provision.CloudInitVersion {
					return nil, false, fmt.Errorf("VM is outdated and needs to be recreated\n       run: yg destroy && yg <command>")
				}
				currentHash := provision.SetupHash(cc.Config.Setup)
				if vmState.SetupHash != "" && vmState.SetupHash != currentHash {
					w.Info("setup changed — VM needs reprovisioning")
					w.Info("run: yg destroy && yg up")
				}
				// Check if compute size has changed.
				expectedType, sizeErr := provider.InstanceTypeForSize(cc.Config.Compute.Size)
				if sizeErr == nil && info.InstanceType != "" && string(expectedType) != info.InstanceType {
					w.Infof("size changed (%s → %s) — recreating VM...", info.InstanceType, string(expectedType))
					if termErr := cc.Provider.TerminateVM(ctx, info.InstanceID); termErr != nil {
						return nil, false, fmt.Errorf("terminating VM for size change: %w", termErr)
					}
					_ = cc.State.DeleteVM(cc.Project.Hash)
					// Fall through to createVMForRun below.
				} else {
					w.Infof("VM running (%s)", info.InstanceID)
					return info, false, nil
				}
			case "stopped":
				// Check if compute size has changed before starting.
				expectedType, sizeErr := provider.InstanceTypeForSize(cc.Config.Compute.Size)
				if sizeErr == nil && info.InstanceType != "" && string(expectedType) != info.InstanceType {
					w.Infof("size changed (%s → %s) — recreating VM...", info.InstanceType, string(expectedType))
					if termErr := cc.Provider.TerminateVM(ctx, info.InstanceID); termErr != nil {
						return nil, false, fmt.Errorf("terminating VM for size change: %w", termErr)
					}
					_ = cc.State.DeleteVM(cc.Project.Hash)
					break // fall through to createVMForRun below
				}
				w.StartSpinner(fmt.Sprintf("starting stopped VM %s...", info.InstanceID))
				if err := cc.Provider.StartVM(ctx, info.InstanceID); err != nil {
					w.StopSpinner("failed to start VM", false)
					return nil, false, err
				}
				err := cc.Provider.WaitUntilRunningWithProgress(ctx, info.InstanceID, func(elapsed time.Duration) {
					w.UpdateSpinner(provider.FormatProgressMessage(elapsed))
				})
				if err != nil {
					w.StopSpinner("VM failed to start", false)
					return nil, false, err
				}
				// Re-query for updated IP and AZ.
				info, err = cc.Provider.FindVM(ctx, cc.Project.Hash)
				if err != nil {
					w.StopSpinner("VM started", true)
					return nil, false, fmt.Errorf("querying VM after start: %w", err)
				}
				w.StopSpinner(fmt.Sprintf("VM running (%s)", info.InstanceID), true)
				return info, false, nil
			case "pending":
				w.StartSpinner(fmt.Sprintf("VM starting (%s)...", info.InstanceID))
				err := cc.Provider.WaitUntilRunningWithProgress(ctx, info.InstanceID, func(elapsed time.Duration) {
					w.UpdateSpinner(provider.FormatProgressMessage(elapsed))
				})
				if err != nil {
					w.StopSpinner("VM failed to start", false)
					return nil, false, err
				}
				info, err = cc.Provider.FindVM(ctx, cc.Project.Hash)
				if err != nil {
					w.StopSpinner("VM started", true)
					return nil, false, fmt.Errorf("querying VM after wait: %w", err)
				}
				w.StopSpinner(fmt.Sprintf("VM running (%s)", info.InstanceID), true)
				return info, false, nil
			default:
				w.Infof("VM %s is %s — creating a new one", info.InstanceID, info.State)
			}
		} else {
			w.Infof("VM %s no longer exists — creating a new one", vmState.InstanceID)
		}
	} else {
		w.Infof("no VM found — creating one in %s", cc.Provider.Region())
	}

	info, err := createVMForRun(ctx, cc)
	return info, true, err
}

// createVMForRun handles VM creation and returns the live VMInfo.
func createVMForRun(ctx context.Context, cc *cmdContext) (*provider.VMInfo, error) {
	w := cc.Output

	langs := provision.DetectLanguages(cc.Project.AbsPath)
	for _, lang := range langs {
		w.Infof("detected %s", lang.DisplayName)
	}

	ci := provision.GenerateCloudInit(langs, cc.Config.Setup)
	userData := base64.StdEncoding.EncodeToString([]byte(ci.Render()))

	sgID, err := cc.Provider.EnsureSecurityGroup(ctx)
	if err != nil {
		return nil, err
	}

	if err := cc.Provider.EnsureBucket(ctx); err != nil {
		return nil, err
	}

	// Display VM size with cost and specs.
	cost := provider.CostPerHour(cc.Config.Compute.Size)
	instanceType, _ := provider.InstanceTypeForSize(cc.Config.Compute.Size)
	vcpu, mem := provider.InstanceSpecs(instanceType)

	if cost > 0.0 && vcpu != "" {
		w.Infof("VM size: %s (%s, %s) %s", cc.Config.Compute.Size, vcpu, mem, provider.FormatCost(cost))
	} else {
		w.Infof("VM size: %s", cc.Config.Compute.Size)
	}

	if instanceType != "" {
		w.StartSpinner(fmt.Sprintf("launching %s in %s...", instanceType, cc.Provider.Region()))
	} else {
		w.StartSpinner(fmt.Sprintf("launching instance in %s...", cc.Provider.Region()))
	}

	info, err := cc.Provider.CreateVM(ctx, provider.CreateVMOpts{
		ProjectHash:     cc.Project.Hash,
		ProjectPath:     cc.Project.AbsPath,
		Size:            cc.Config.Compute.Size,
		SecurityGroupID: sgID,
		UserData:        userData,
	})
	if err != nil {
		w.StopSpinner("failed to launch VM", false)
		return nil, err
	}

	w.UpdateSpinner(fmt.Sprintf("instance %s launched — waiting for it to be ready...", info.InstanceID))
	err = cc.Provider.WaitUntilRunningWithProgress(ctx, info.InstanceID, func(elapsed time.Duration) {
		w.UpdateSpinner(provider.FormatProgressMessage(elapsed))
	})
	if err != nil {
		w.StopSpinner("VM failed to start", false)
		return nil, err
	}

	// Re-query for public IP and AZ.
	liveInfo, err := cc.Provider.FindVM(ctx, cc.Project.Hash)
	if err != nil {
		w.StopSpinner("VM launched", true)
		return nil, fmt.Errorf("querying new VM: %w", err)
	}
	if liveInfo == nil {
		w.StopSpinner("VM launched", true)
		return nil, fmt.Errorf("VM %s not found after creation", info.InstanceID)
	}

	setupHash := provision.SetupHash(cc.Config.Setup)
	if err := cc.State.SaveVM(cc.Project.Hash, state.VMState{
		InstanceID:       liveInfo.InstanceID,
		Region:           liveInfo.Region,
		Created:          time.Now().UTC(),
		ProjectDir:       cc.Project.AbsPath,
		SetupHash:        setupHash,
		CloudInitVersion: provision.CloudInitVersion,
	}); err != nil {
		w.StopSpinner("VM launched", true)
		return nil, fmt.Errorf("saving VM state: %w", err)
	}

	w.StopSpinner(fmt.Sprintf("VM ready (%s)", liveInfo.InstanceID), true)

	// Wait for SSH to become available (cloud-init needs ~30s to start sshd).
	// The instance is "running" but SSH may not be ready yet.
	// Poll with exponential backoff instead of sleeping.
	w.StartSpinner("connecting via SSH...")
	if err := waitForSSH(ctx, cc, liveInfo, w); err != nil {
		w.StopSpinner("SSH connection failed", false)
		return nil, fmt.Errorf("waiting for SSH: %w", err)
	}
	w.StopSpinner("SSH connected", true)

	return liveInfo, nil
}

// waitForSSH polls the VM until SSH is available, with exponential backoff.
// Returns an error if SSH doesn't become available within the timeout.
func waitForSSH(ctx context.Context, cc *cmdContext, vmInfo *provider.VMInfo, w *output.Writer) error {
	const (
		maxAttempts = 12           // Max retry attempts
		initialWait = 2 * time.Second
		maxWait     = 8 * time.Second
	)

	wait := initialWait
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		// Try to establish SSH connection
		client, err := cc.ConnectSSH(ctx, vmInfo)
		if err == nil {
			// SSH is available (err == nil means connection succeeded).
			// Close the client if it's non-nil (in tests it may be nil).
			if client != nil {
				client.Close()
			}
			return nil // SSH is ready — caller handles the success message
		}

		// Connection failed - check if we should retry
		if attempt == maxAttempts {
			w.Hint("the VM may still be provisioning — wait a minute and try again")
			return fmt.Errorf("SSH not available after %d attempts: %w", maxAttempts, err)
		}

		// Update spinner with attempt count (every attempt for TTY).
		w.UpdateSpinner(fmt.Sprintf("connecting via SSH (attempt %d/%d)...", attempt, maxAttempts))
		// Also log progress every 3 attempts for non-TTY environments (CI, pipes).
		if attempt%3 == 0 {
			w.Infof("still waiting for SSH (attempt %d/%d)", attempt, maxAttempts)
		}

		// Wait before retry with exponential backoff
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(wait):
			// Exponential backoff with cap
			wait *= 2
			if wait > maxWait {
				wait = maxWait
			}
		}
	}

	return fmt.Errorf("SSH not available after %d attempts", maxAttempts)
}

// defaultSyncFunc runs rsync to sync project files to the VM.
func defaultSyncFunc(ctx context.Context, cc *cmdContext, vmInfo *provider.VMInfo) (*fksync.SyncResult, error) {
	// Generate ephemeral key for rsync.
	authorizedKey, privKey, err := fkssh.GenerateEphemeralKeyForSync()
	if err != nil {
		return nil, fmt.Errorf("generating sync key: %w", err)
	}

	// Push key to instance via EC2 Instance Connect.
	connector, err := cc.NewSSHConnector(ctx, vmInfo.Region, vmInfo.AvailabilityZone)
	if err != nil {
		return nil, err
	}
	if err := connector.PushKeyDirect(ctx, vmInfo.InstanceID, authorizedKey); err != nil {
		return nil, err
	}

	// Write private key to temp file for rsync -i flag.
	keyFile, err := os.CreateTemp("", "yg-sync-key-*")
	if err != nil {
		return nil, fmt.Errorf("creating temp key file: %w", err)
	}
	defer os.Remove(keyFile.Name())

	pemData, err := fkssh.MarshalPrivateKey(privKey)
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(keyFile.Name(), pemData, 0o600); err != nil {
		return nil, fmt.Errorf("writing temp key: %w", err)
	}
	keyFile.Close()

	// Build rsync args.
	langs := provision.DetectLanguages(cc.Project.AbsPath)
	var langNames []provision.LanguageName
	for _, l := range langs {
		langNames = append(langNames, l.Name)
	}

	sourceDir := cc.Project.AbsPath
	if !strings.HasSuffix(sourceDir, "/") {
		sourceDir += "/"
	}

	syncOpts := fksync.Options{
		SourceDir:  sourceDir,
		RemoteDir:  remoteProjectDir + "/",
		Host:       vmInfo.PublicIP,
		User:       "ubuntu",
		SSHPort:    22,
		SSHKeyPath: keyFile.Name(),
		SyncConfig: cc.Config.Sync,
		Languages:  langNames,
	}

	args := fksync.BuildArgs(syncOpts)
	cmd := exec.CommandContext(ctx, "rsync", args...)

	var rsyncOut, rsyncErr bytes.Buffer
	cmd.Stdout = &rsyncOut
	cmd.Stderr = &rsyncErr

	if err := cmd.Run(); err != nil {
		// Translate rsync error to user-friendly message
		translated := fksync.TranslateRsyncError(err, rsyncErr.String())
		return nil, translated
	}

	result := fksync.ParseStats(rsyncOut.String())
	return &result, nil
}

// formatSyncResult formats a sync result for display.
// On first run (freshVM=true): "synced 847 files (12.0 MB)"
// On warm run (freshVM=false): "synced 3 changed files" or "no files changed"
func formatSyncResult(r *fksync.SyncResult, freshVM bool) string {
	if freshVM {
		if r.BytesTransferred > 0 {
			return fmt.Sprintf("synced %d files (%s)", r.TotalFiles, fksync.FormatBytes(r.BytesTransferred))
		}
		return fmt.Sprintf("synced %d files", r.TotalFiles)
	}
	if r.FilesTransferred == 0 {
		return "no files changed"
	}
	if r.FilesTransferred == 1 {
		return "synced 1 changed file"
	}
	return fmt.Sprintf("synced %d changed files", r.FilesTransferred)
}

// uploadArtifacts reads artifact files from the VM and uploads them to S3.
// Returns the number of artifacts successfully uploaded.
// This is best-effort — missing files are warned about, not fatal.
func uploadArtifacts(ctx context.Context, cc *cmdContext, client *gossh.Client, runID fkexec.RunID) int {
	w := cc.Output

	store, err := cc.NewStorage(ctx)
	if err != nil {
		w.Warn(fmt.Sprintf("failed to connect to S3 for artifacts: %s", err), "")
		return 0
	}

	uploaded := 0
	for _, artifactPath := range cc.Config.Artifacts.Paths {
		remotePath := remoteProjectDir + "/" + artifactPath
		data, err := cc.ReadRemoteFile(client, remotePath)
		if err != nil {
			w.Warn(fmt.Sprintf("artifact %s not found on VM", artifactPath), "")
			slog.Debug("failed to read artifact", "path", artifactPath, "error", err)
			continue
		}

		if err := store.UploadArtifact(ctx, cc.Project.DisplayName, runID.String(), artifactPath, data); err != nil {
			w.Warn(fmt.Sprintf("failed to upload artifact %s: %s", artifactPath, err), "")
			continue
		}
		uploaded++
	}

	return uploaded
}

// uploadOutput uploads run output to S3.
func uploadOutput(ctx context.Context, cc *cmdContext, runID fkexec.RunID, command string, result *fkexec.RunResult, stdout, stderr []byte) error {
	store, err := cc.NewStorage(ctx)
	if err != nil {
		return err
	}

	meta := fkstorage.RunMeta{
		RunID:     runID.String(),
		Command:   command,
		Project:   cc.Project.DisplayName,
		ExitCode:  result.ExitCode,
		StartTime: result.StartTime,
		EndTime:   result.EndTime,
		Duration:  formatDuration(result.Duration().Truncate(time.Second)),
	}

	return store.UploadOutput(ctx, cc.Project.DisplayName, runID.String(), stdout, stderr, meta)
}

// teeWriter streams output to the terminal via output.Writer and captures it in a buffer.
type teeWriter struct {
	w   *output.Writer
	buf *bytes.Buffer
}

func newTeeWriter(w *output.Writer, buf *bytes.Buffer) *teeWriter {
	return &teeWriter{w: w, buf: buf}
}

func (t *teeWriter) Write(p []byte) (n int, err error) {
	t.buf.Write(p)
	t.w.Stream(p)
	return len(p), nil
}

// formatDuration formats a duration into a human-readable string like "1m 23s".
func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	minutes := int(d.Minutes())
	seconds := int(d.Seconds()) - minutes*60
	return fmt.Sprintf("%dm %02ds", minutes, seconds)
}
