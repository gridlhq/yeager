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

	// Step 1: Ensure VM is running.
	vmInfo, freshVM, err := ensureVMRunning(ctx, cc)
	if err != nil {
		printClassifiedError(w, err)
		return 1, displayed(err)
	}

	// Step 2: Sync files.
	w.Info("syncing files...")
	syncResult, err := cc.RunSync(ctx, cc, vmInfo)
	if err != nil {
		return 1, fmt.Errorf("syncing files: %w", err)
	}
	if syncResult != nil {
		w.Info(formatSyncResult(syncResult, freshVM))
	}

	// Step 3: Establish SSH connection for command execution.
	if freshVM {
		w.Info("installing toolchain (this runs once per VM)...")
	}
	w.Info("connecting...")
	client, err := cc.ConnectSSH(ctx, vmInfo)
	if err != nil {
		if freshVM {
			w.Info("hint: the VM may still be provisioning — wait a minute and try again")
		}
		return 1, fmt.Errorf("SSH connection failed: %w", err)
	}
	if client != nil {
		defer client.Close()
	}

	// Step 4: Execute command.
	runID := fkexec.GenerateRunID()
	w.Infof("running: %s", command)
	w.Info("")
	w.Info("Ctrl+C stops streaming. The command keeps running on the VM.")
	w.Info("Re-attach: yg logs    Status: yg status")
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
		w.Info("detached from output stream")
		w.Infof("command still running on VM (elapsed %s)", formatDuration(elapsed))
		w.Info("re-attach:  yg logs")
		w.Info("status:     yg status")
		w.Info("cancel:     yg kill")
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
	w.Infof("done (exit %d) in %s", result.ExitCode, formatDuration(duration))

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
		w.Infof("warning: failed to upload output to S3: %s", err)
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

// checkIdleAndStop does a single check after a command finishes: if no other
// commands are running on the VM, stop it immediately to save costs.
// This is a best-effort operation — failures are logged, not returned.
func checkIdleAndStop(ctx context.Context, cc *cmdContext, vmInfo *provider.VMInfo) {
	idleTimeout, err := cc.Config.Lifecycle.IdleStopDuration()
	if err != nil || idleTimeout <= 0 {
		return
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
		return
	}

	if monitor.CheckAndStop(ctx) {
		cc.Output.Info("VM stopped (no active commands)")
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
				currentHash := provision.SetupHash(cc.Config.Setup)
				if vmState.SetupHash != "" && vmState.SetupHash != currentHash {
					w.Info("setup changed — VM needs reprovisioning")
					w.Info("run: yg destroy && yg up")
				}
				w.Infof("VM running (%s)", info.InstanceID)
				return info, false, nil
			case "stopped":
				w.Infof("starting stopped VM %s...", info.InstanceID)
				if err := cc.Provider.StartVM(ctx, info.InstanceID); err != nil {
					return nil, false, err
				}
				w.Info("waiting for VM to be ready...")
				err := cc.Provider.WaitUntilRunningWithProgress(ctx, info.InstanceID, func(elapsed time.Duration) {
					w.Info(provider.FormatProgressMessage(elapsed))
				})
				if err != nil {
					return nil, false, err
				}
				// Re-query for updated IP and AZ.
				info, err = cc.Provider.FindVM(ctx, cc.Project.Hash)
				if err != nil {
					return nil, false, fmt.Errorf("querying VM after start: %w", err)
				}
				w.Infof("VM running (%s)", info.InstanceID)
				return info, false, nil
			case "pending":
				w.Infof("VM starting (%s)...", info.InstanceID)
				err := cc.Provider.WaitUntilRunningWithProgress(ctx, info.InstanceID, func(elapsed time.Duration) {
					w.Info(provider.FormatProgressMessage(elapsed))
				})
				if err != nil {
					return nil, false, err
				}
				info, err = cc.Provider.FindVM(ctx, cc.Project.Hash)
				if err != nil {
					return nil, false, fmt.Errorf("querying VM after wait: %w", err)
				}
				w.Infof("VM running (%s)", info.InstanceID)
				return info, false, nil
			default:
				w.Infof("VM %s is %s — creating a new one", info.InstanceID, info.State)
			}
		} else {
			w.Infof("VM %s no longer exists — creating a new one", vmState.InstanceID)
		}
	} else {
		w.Info("no VM found — creating one")
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

	w.Infof("launching instance... (%s)", cc.Provider.Region())

	info, err := cc.Provider.CreateVM(ctx, provider.CreateVMOpts{
		ProjectHash:     cc.Project.Hash,
		ProjectPath:     cc.Project.AbsPath,
		Size:            cc.Config.Compute.Size,
		SecurityGroupID: sgID,
		UserData:        userData,
	})
	if err != nil {
		return nil, err
	}

	w.Infof("instance %s launched — waiting for it to be ready...", info.InstanceID)
	err = cc.Provider.WaitUntilRunningWithProgress(ctx, info.InstanceID, func(elapsed time.Duration) {
		w.Info(provider.FormatProgressMessage(elapsed))
	})
	if err != nil {
		return nil, err
	}

	// Re-query for public IP and AZ.
	liveInfo, err := cc.Provider.FindVM(ctx, cc.Project.Hash)
	if err != nil {
		return nil, fmt.Errorf("querying new VM: %w", err)
	}
	if liveInfo == nil {
		return nil, fmt.Errorf("VM %s not found after creation", info.InstanceID)
	}

	setupHash := provision.SetupHash(cc.Config.Setup)
	if err := cc.State.SaveVM(cc.Project.Hash, state.VMState{
		InstanceID: liveInfo.InstanceID,
		Region:     liveInfo.Region,
		Created:    time.Now().UTC(),
		ProjectDir: cc.Project.AbsPath,
		SetupHash:  setupHash,
	}); err != nil {
		return nil, fmt.Errorf("saving VM state: %w", err)
	}

	w.Infof("VM running (%s)", liveInfo.InstanceID)
	return liveInfo, nil
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
		w.Infof("warning: failed to connect to S3 for artifacts: %s", err)
		return 0
	}

	uploaded := 0
	for _, artifactPath := range cc.Config.Artifacts.Paths {
		remotePath := remoteProjectDir + "/" + artifactPath
		data, err := cc.ReadRemoteFile(client, remotePath)
		if err != nil {
			w.Infof("warning: artifact %s not found on VM", artifactPath)
			slog.Debug("failed to read artifact", "path", artifactPath, "error", err)
			continue
		}

		if err := store.UploadArtifact(ctx, cc.Project.DisplayName, runID.String(), artifactPath, data); err != nil {
			w.Infof("warning: failed to upload artifact %s: %s", artifactPath, err)
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
