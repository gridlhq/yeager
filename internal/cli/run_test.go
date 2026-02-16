package cli

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	fkexec "github.com/gridlhq/yeager/internal/exec"
	"github.com/gridlhq/yeager/internal/output"
	"github.com/gridlhq/yeager/internal/provider"
	fkstorage "github.com/gridlhq/yeager/internal/storage"
	fksync "github.com/gridlhq/yeager/internal/sync"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gossh "golang.org/x/crypto/ssh"
)

func TestFormatDuration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		duration time.Duration
		expected string
	}{
		{0, "0s"},
		{1 * time.Second, "1s"},
		{14 * time.Second, "14s"},
		{59 * time.Second, "59s"},
		{60 * time.Second, "1m 00s"},
		{83 * time.Second, "1m 23s"},
		{5*time.Minute + 30*time.Second, "5m 30s"},
		{10*time.Minute + 5*time.Second, "10m 05s"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.expected, formatDuration(tt.duration))
		})
	}
}

func TestTeeWriter(t *testing.T) {
	t.Parallel()

	var outBuf bytes.Buffer
	w := output.NewWithWriters(&outBuf, &bytes.Buffer{}, output.ModeText)
	var captureBuf bytes.Buffer
	tw := newTeeWriter(w, &captureBuf)

	n, err := tw.Write([]byte("hello world\n"))
	require.NoError(t, err)
	assert.Equal(t, 12, n)

	// Should be captured in buffer.
	assert.Equal(t, "hello world\n", captureBuf.String())
	// Should be streamed to output writer.
	assert.Equal(t, "hello world\n", outBuf.String())
}

func TestTeeWriter_MultipleWrites(t *testing.T) {
	t.Parallel()

	var outBuf bytes.Buffer
	w := output.NewWithWriters(&outBuf, &bytes.Buffer{}, output.ModeText)
	var captureBuf bytes.Buffer
	tw := newTeeWriter(w, &captureBuf)

	tw.Write([]byte("first\n"))
	tw.Write([]byte("second\n"))

	assert.Equal(t, "first\nsecond\n", captureBuf.String())
	assert.Equal(t, "first\nsecond\n", outBuf.String())
}

func TestEnsureVMRunning_RunningVM(t *testing.T) {
	t.Parallel()

	prov := &mockProvider{
		findVMFn: func(ctx context.Context, projectHash string) (*provider.VMInfo, error) {
			return &provider.VMInfo{
				InstanceID:       "i-run001",
				State:            "running",
				PublicIP:         "1.2.3.4",
				Region:           "us-east-1",
				AvailabilityZone: "us-east-1a",
			}, nil
		},
	}
	cc, stdout, _ := testCmdContext(t, prov)
	saveTestVMState(t, cc.State, cc.Project.Hash)

	info, freshVM, err := ensureVMRunning(context.Background(), cc)
	require.NoError(t, err)
	assert.False(t, freshVM, "existing running VM should not be fresh")
	assert.Equal(t, "i-run001", info.InstanceID)
	assert.Equal(t, "1.2.3.4", info.PublicIP)
	assert.Equal(t, "us-east-1a", info.AvailabilityZone)
	assert.Contains(t, stdout.String(), "VM running")
}

func TestEnsureVMRunning_CreatesNewVM(t *testing.T) {
	t.Parallel()

	prov := &mockProvider{
		createVMFn: func(ctx context.Context, opts provider.CreateVMOpts) (provider.VMInfo, error) {
			return provider.VMInfo{
				InstanceID: "i-new001",
				State:      "pending",
				Region:     "us-east-1",
			}, nil
		},
		findVMFn: func(ctx context.Context, projectHash string) (*provider.VMInfo, error) {
			// Called by createVMForRun after WaitUntilRunning to get live IP/AZ.
			return &provider.VMInfo{
				InstanceID:       "i-new001",
				State:            "running",
				PublicIP:         "5.6.7.8",
				Region:           "us-east-1",
				AvailabilityZone: "us-east-1b",
			}, nil
		},
	}
	cc, stdout, _ := testCmdContext(t, prov)

	info, freshVM, err := ensureVMRunning(context.Background(), cc)
	require.NoError(t, err)
	assert.True(t, freshVM, "newly created VM should be fresh")
	assert.Equal(t, "i-new001", info.InstanceID)
	assert.Equal(t, "5.6.7.8", info.PublicIP)
	assert.Contains(t, stdout.String(), "creating one")
	assert.Contains(t, stdout.String(), "VM running")

	// Verify cost indicator is shown during VM creation.
	assert.Contains(t, stdout.String(), "VM size: medium")
	assert.Contains(t, stdout.String(), "~$0.034/hr")
}

func TestEnsureVMRunning_StartsStoppedVM(t *testing.T) {
	t.Parallel()

	findCalls := 0
	prov := &mockProvider{
		findVMFn: func(ctx context.Context, projectHash string) (*provider.VMInfo, error) {
			findCalls++
			if findCalls == 1 {
				return &provider.VMInfo{
					InstanceID: "i-stopped",
					State:      "stopped",
					Region:     "us-east-1",
				}, nil
			}
			// After start, return running with IP.
			return &provider.VMInfo{
				InstanceID:       "i-stopped",
				State:            "running",
				PublicIP:         "9.8.7.6",
				Region:           "us-east-1",
				AvailabilityZone: "us-east-1c",
			}, nil
		},
	}
	cc, stdout, _ := testCmdContext(t, prov)
	saveTestVMState(t, cc.State, cc.Project.Hash)

	info, freshVM, err := ensureVMRunning(context.Background(), cc)
	require.NoError(t, err)
	assert.False(t, freshVM, "restarted VM should not be fresh")
	assert.Equal(t, "i-stopped", info.InstanceID)
	assert.Equal(t, "9.8.7.6", info.PublicIP)
	assert.Contains(t, stdout.String(), "starting stopped VM")
}

func TestEnsureVMRunning_FindVMError(t *testing.T) {
	t.Parallel()

	prov := &mockProvider{
		findVMFn: func(ctx context.Context, projectHash string) (*provider.VMInfo, error) {
			return nil, fmt.Errorf("access denied")
		},
	}
	cc, _, _ := testCmdContext(t, prov)
	saveTestVMState(t, cc.State, cc.Project.Hash)

	_, _, err := ensureVMRunning(context.Background(), cc)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "querying VM state")
}

func TestRunCommand_FullPipeline(t *testing.T) {
	t.Parallel()

	syncCalled := false
	storageCalled := false

	prov := &mockProvider{
		findVMFn: func(ctx context.Context, projectHash string) (*provider.VMInfo, error) {
			return &provider.VMInfo{
				InstanceID:       "i-test001",
				State:            "running",
				PublicIP:         "10.0.0.1",
				Region:           "us-east-1",
				AvailabilityZone: "us-east-1a",
			}, nil
		},
	}
	cc, stdout, _ := testCmdContext(t, prov)
	saveTestVMState(t, cc.State, cc.Project.Hash)

	// Mock sync — just succeed.
	cc.RunSync = func(ctx context.Context, cc *cmdContext, vmInfo *provider.VMInfo) (*fksync.SyncResult, error) {
		syncCalled = true
		assert.Equal(t, "10.0.0.1", vmInfo.PublicIP)
		return &fksync.SyncResult{TotalFiles: 10, FilesTransferred: 3, BytesTransferred: 1024}, nil
	}

	// Mock SSH connection — return error since we can't run real SSH in tests.
	// The test verifies the pipeline up to SSH connection.
	cc.ConnectSSH = func(ctx context.Context, vmInfo *provider.VMInfo) (*gossh.Client, error) {
		return nil, fmt.Errorf("test: SSH not available")
	}

	// Mock storage.
	cc.NewStorage = func(ctx context.Context) (*fkstorage.Store, error) {
		storageCalled = true
		return nil, fmt.Errorf("test: storage not available")
	}

	_, err := RunCommand(context.Background(), cc, "cargo test")
	// Will fail at SSH connection stage, which is expected in unit tests.
	require.Error(t, err)
	assert.Contains(t, err.Error(), "SSH not available")

	// Verify pipeline reached sync step.
	assert.True(t, syncCalled, "sync should have been called")
	// Storage shouldn't be called since we failed at SSH.
	assert.False(t, storageCalled)

	// Verify output includes expected messages.
	out := stdout.String()
	assert.Contains(t, out, "project: myproject")
	assert.Contains(t, out, "VM running")
	assert.Contains(t, out, "syncing files")
	assert.Contains(t, out, "connecting")
}

func TestRunCommand_SyncError(t *testing.T) {
	t.Parallel()

	prov := &mockProvider{
		findVMFn: func(ctx context.Context, projectHash string) (*provider.VMInfo, error) {
			return &provider.VMInfo{
				InstanceID:       "i-test001",
				State:            "running",
				PublicIP:         "10.0.0.1",
				Region:           "us-east-1",
				AvailabilityZone: "us-east-1a",
			}, nil
		},
	}
	cc, _, _ := testCmdContext(t, prov)
	saveTestVMState(t, cc.State, cc.Project.Hash)

	cc.RunSync = func(ctx context.Context, cc *cmdContext, vmInfo *provider.VMInfo) (*fksync.SyncResult, error) {
		return nil, fmt.Errorf("rsync failed: connection refused")
	}

	exitCode, err := RunCommand(context.Background(), cc, "cargo test")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "syncing files")
	assert.Equal(t, 1, exitCode)
}

func TestRunCommand_VMCreationError(t *testing.T) {
	t.Parallel()

	prov := &mockProvider{
		ensureSecurityGroupFn: func(ctx context.Context) (string, error) {
			return "", fmt.Errorf("insufficient permissions")
		},
	}
	cc, _, _ := testCmdContext(t, prov)

	exitCode, err := RunCommand(context.Background(), cc, "cargo test")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "insufficient permissions")
	assert.Equal(t, 1, exitCode)
}

func TestRunCommand_AccessDeniedShowsActionableError(t *testing.T) {
	t.Parallel()

	prov := &mockProvider{
		ensureSecurityGroupFn: func(ctx context.Context) (string, error) {
			return "", fmt.Errorf("UnauthorizedOperation: You are not authorized to perform ec2:CreateSecurityGroup")
		},
	}
	cc, _, stderr := testCmdContext(t, prov)

	exitCode, err := RunCommand(context.Background(), cc, "cargo test")
	require.Error(t, err)
	assert.Equal(t, 1, exitCode)

	// Verify actionable error message was printed.
	errOut := stderr.String()
	assert.Contains(t, errOut, "permissions denied")
	assert.Contains(t, errOut, "EC2, S3, STS")
}

func TestRunCommand_CapacityErrorShowsActionableError(t *testing.T) {
	t.Parallel()

	prov := &mockProvider{
		createVMFn: func(ctx context.Context, opts provider.CreateVMOpts) (provider.VMInfo, error) {
			return provider.VMInfo{}, fmt.Errorf("launching instance: InsufficientInstanceCapacity: no capacity")
		},
	}
	cc, _, stderr := testCmdContext(t, prov)

	exitCode, err := RunCommand(context.Background(), cc, "cargo test")
	require.Error(t, err)
	assert.Equal(t, 1, exitCode)

	errOut := stderr.String()
	assert.Contains(t, errOut, "capacity limit")
	assert.Contains(t, errOut, "different region")
}

func TestRunCommand_SSHFailOnFreshVMShowsHint(t *testing.T) {
	t.Parallel()

	prov := &mockProvider{
		createVMFn: func(ctx context.Context, opts provider.CreateVMOpts) (provider.VMInfo, error) {
			return provider.VMInfo{InstanceID: "i-new001", State: "pending", Region: "us-east-1"}, nil
		},
		findVMFn: func(ctx context.Context, projectHash string) (*provider.VMInfo, error) {
			return &provider.VMInfo{
				InstanceID: "i-new001", State: "running", PublicIP: "1.2.3.4",
				Region: "us-east-1", AvailabilityZone: "us-east-1a",
			}, nil
		},
	}
	cc, stdout, _ := testCmdContext(t, prov)

	cc.RunSync = func(ctx context.Context, cc *cmdContext, vmInfo *provider.VMInfo) (*fksync.SyncResult, error) {
		return &fksync.SyncResult{TotalFiles: 5, FilesTransferred: 5}, nil
	}
	cc.ConnectSSH = func(ctx context.Context, vmInfo *provider.VMInfo) (*gossh.Client, error) {
		return nil, fmt.Errorf("connection refused")
	}

	_, err := RunCommand(context.Background(), cc, "cargo test")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "SSH connection failed")

	// On fresh VM, should show provisioning hint.
	out := stdout.String()
	assert.Contains(t, out, "still be provisioning")
}

func TestRunCommand_CtrlCDetach(t *testing.T) {
	t.Parallel()

	prov := &mockProvider{
		findVMFn: func(ctx context.Context, projectHash string) (*provider.VMInfo, error) {
			return &provider.VMInfo{
				InstanceID:       "i-test001",
				State:            "running",
				PublicIP:         "10.0.0.1",
				Region:           "us-east-1",
				AvailabilityZone: "us-east-1a",
			}, nil
		},
	}
	cc, stdout, _ := testCmdContext(t, prov)
	saveTestVMState(t, cc.State, cc.Project.Hash)

	ctx, cancel := context.WithCancel(context.Background())

	cc.RunSync = func(ctx context.Context, cc *cmdContext, vmInfo *provider.VMInfo) (*fksync.SyncResult, error) {
		return &fksync.SyncResult{TotalFiles: 5, FilesTransferred: 1}, nil
	}

	// Mock SSH connection — returns nil client (RunExec is also mocked so client is unused).
	cc.ConnectSSH = func(ctx context.Context, vmInfo *provider.VMInfo) (*gossh.Client, error) {
		return nil, nil
	}

	// Mock exec — cancel context during execution to simulate Ctrl+C, then return error.
	cc.RunExec = func(client *gossh.Client, opts fkexec.RunOpts, stdoutW, stderrW io.Writer) (*fkexec.RunResult, error) {
		cancel()
		return nil, fmt.Errorf("session closed")
	}

	exitCode, err := RunCommand(ctx, cc, "cargo test")
	require.NoError(t, err)
	assert.Equal(t, 0, exitCode, "Ctrl+C detach should return exit code 0")

	// Verify detach message is printed.
	out := stdout.String()
	assert.Contains(t, out, "detached from output stream")
	assert.Contains(t, out, "command still running on VM")
	assert.Contains(t, out, "re-attach:  yg logs")
	assert.Contains(t, out, "status:     yg status")
	assert.Contains(t, out, "cancel:     yg kill")

	// Verify last run ID was saved for re-attach.
	lastRun, err := cc.State.LoadLastRun(cc.Project.Hash)
	require.NoError(t, err)
	assert.NotEmpty(t, lastRun, "last run ID should be saved on detach")
}

func TestRunCommand_CtrlCDoesNotUploadToS3(t *testing.T) {
	t.Parallel()

	prov := &mockProvider{
		findVMFn: func(ctx context.Context, projectHash string) (*provider.VMInfo, error) {
			return &provider.VMInfo{
				InstanceID:       "i-test001",
				State:            "running",
				PublicIP:         "10.0.0.1",
				Region:           "us-east-1",
				AvailabilityZone: "us-east-1a",
			}, nil
		},
	}
	cc, _, _ := testCmdContext(t, prov)
	saveTestVMState(t, cc.State, cc.Project.Hash)

	ctx, cancel := context.WithCancel(context.Background())
	storageCalled := false

	cc.RunSync = func(ctx context.Context, cc *cmdContext, vmInfo *provider.VMInfo) (*fksync.SyncResult, error) {
		return &fksync.SyncResult{TotalFiles: 5, FilesTransferred: 1}, nil
	}
	cc.ConnectSSH = func(ctx context.Context, vmInfo *provider.VMInfo) (*gossh.Client, error) {
		return nil, nil
	}
	cc.RunExec = func(client *gossh.Client, opts fkexec.RunOpts, stdoutW, stderrW io.Writer) (*fkexec.RunResult, error) {
		cancel()
		return nil, fmt.Errorf("session closed")
	}
	cc.NewStorage = func(ctx context.Context) (*fkstorage.Store, error) {
		storageCalled = true
		return nil, fmt.Errorf("should not be called")
	}

	RunCommand(ctx, cc, "cargo test")
	assert.False(t, storageCalled, "S3 upload should be skipped on Ctrl+C detach")
}

func TestFormatSyncResult(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		result  *fksync.SyncResult
		freshVM bool
		want    string
	}{
		{
			name:    "first run with bytes",
			result:  &fksync.SyncResult{TotalFiles: 847, FilesTransferred: 847, BytesTransferred: 12_582_912},
			freshVM: true,
			want:    "synced 847 files (12.0 MB)",
		},
		{
			name:    "first run without bytes",
			result:  &fksync.SyncResult{TotalFiles: 5, FilesTransferred: 5, BytesTransferred: 0},
			freshVM: true,
			want:    "synced 5 files",
		},
		{
			name:    "warm run with changes",
			result:  &fksync.SyncResult{TotalFiles: 847, FilesTransferred: 3, BytesTransferred: 4096},
			freshVM: false,
			want:    "synced 3 changed files",
		},
		{
			name:    "warm run single file",
			result:  &fksync.SyncResult{TotalFiles: 100, FilesTransferred: 1, BytesTransferred: 512},
			freshVM: false,
			want:    "synced 1 changed file",
		},
		{
			name:    "warm run no changes",
			result:  &fksync.SyncResult{TotalFiles: 200, FilesTransferred: 0, BytesTransferred: 0},
			freshVM: false,
			want:    "no files changed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := formatSyncResult(tt.result, tt.freshVM)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestRunCommand_SyncProgressOutput_WarmRun(t *testing.T) {
	t.Parallel()

	prov := &mockProvider{
		findVMFn: func(ctx context.Context, projectHash string) (*provider.VMInfo, error) {
			return &provider.VMInfo{
				InstanceID:       "i-test001",
				State:            "running",
				PublicIP:         "10.0.0.1",
				Region:           "us-east-1",
				AvailabilityZone: "us-east-1a",
			}, nil
		},
	}
	cc, stdout, _ := testCmdContext(t, prov)
	saveTestVMState(t, cc.State, cc.Project.Hash)

	cc.RunSync = func(ctx context.Context, cc *cmdContext, vmInfo *provider.VMInfo) (*fksync.SyncResult, error) {
		return &fksync.SyncResult{TotalFiles: 100, FilesTransferred: 3, BytesTransferred: 4096}, nil
	}
	cc.ConnectSSH = func(ctx context.Context, vmInfo *provider.VMInfo) (*gossh.Client, error) {
		return nil, fmt.Errorf("test: SSH not available")
	}

	RunCommand(context.Background(), cc, "cargo test")
	out := stdout.String()
	assert.Contains(t, out, "synced 3 changed files")
}

func TestRunCommand_SyncProgressOutput_NoChanges(t *testing.T) {
	t.Parallel()

	prov := &mockProvider{
		findVMFn: func(ctx context.Context, projectHash string) (*provider.VMInfo, error) {
			return &provider.VMInfo{
				InstanceID:       "i-test001",
				State:            "running",
				PublicIP:         "10.0.0.1",
				Region:           "us-east-1",
				AvailabilityZone: "us-east-1a",
			}, nil
		},
	}
	cc, stdout, _ := testCmdContext(t, prov)
	saveTestVMState(t, cc.State, cc.Project.Hash)

	cc.RunSync = func(ctx context.Context, cc *cmdContext, vmInfo *provider.VMInfo) (*fksync.SyncResult, error) {
		return &fksync.SyncResult{TotalFiles: 200, FilesTransferred: 0, BytesTransferred: 0}, nil
	}
	cc.ConnectSSH = func(ctx context.Context, vmInfo *provider.VMInfo) (*gossh.Client, error) {
		return nil, fmt.Errorf("test: SSH not available")
	}

	RunCommand(context.Background(), cc, "cargo test")
	out := stdout.String()
	assert.Contains(t, out, "no files changed")
}

func TestRunCommand_SavesRunHistory(t *testing.T) {
	t.Parallel()

	prov := &mockProvider{
		findVMFn: func(ctx context.Context, projectHash string) (*provider.VMInfo, error) {
			return &provider.VMInfo{
				InstanceID:       "i-hist001",
				State:            "running",
				PublicIP:         "10.0.0.1",
				Region:           "us-east-1",
				AvailabilityZone: "us-east-1a",
			}, nil
		},
	}
	cc, _, _ := testCmdContext(t, prov)
	saveTestVMState(t, cc.State, cc.Project.Hash)

	cc.RunSync = func(ctx context.Context, cc *cmdContext, vmInfo *provider.VMInfo) (*fksync.SyncResult, error) {
		return &fksync.SyncResult{}, nil
	}
	cc.ConnectSSH = func(ctx context.Context, vmInfo *provider.VMInfo) (*gossh.Client, error) {
		return nil, nil
	}
	cc.RunExec = func(client *gossh.Client, opts fkexec.RunOpts, stdout, stderr io.Writer) (*fkexec.RunResult, error) {
		return &fkexec.RunResult{
			RunID:     opts.RunID,
			Command:   opts.Command,
			ExitCode:  0,
			StartTime: time.Now().UTC(),
			EndTime:   time.Now().UTC().Add(5 * time.Second),
		}, nil
	}
	cc.NewStorage = func(ctx context.Context) (*fkstorage.Store, error) {
		return nil, fmt.Errorf("test: no S3")
	}

	exitCode, err := RunCommand(context.Background(), cc, "cargo test")
	require.NoError(t, err)
	assert.Equal(t, 0, exitCode)

	// Verify run history was saved.
	history, err := cc.State.LoadRunHistory(cc.Project.Hash)
	require.NoError(t, err)
	require.Len(t, history, 1)
	assert.Equal(t, "cargo test", history[0].Command)
	assert.Equal(t, 0, history[0].ExitCode)
	assert.True(t, history[0].Duration > 0, "duration should be positive")
}

func TestRunCommand_SavesHistoryOnNonZeroExit(t *testing.T) {
	t.Parallel()

	prov := &mockProvider{
		findVMFn: func(ctx context.Context, projectHash string) (*provider.VMInfo, error) {
			return &provider.VMInfo{
				InstanceID:       "i-fail001",
				State:            "running",
				PublicIP:         "10.0.0.1",
				Region:           "us-east-1",
				AvailabilityZone: "us-east-1a",
			}, nil
		},
	}
	cc, _, _ := testCmdContext(t, prov)
	saveTestVMState(t, cc.State, cc.Project.Hash)

	cc.RunSync = func(ctx context.Context, cc *cmdContext, vmInfo *provider.VMInfo) (*fksync.SyncResult, error) {
		return &fksync.SyncResult{}, nil
	}
	cc.ConnectSSH = func(ctx context.Context, vmInfo *provider.VMInfo) (*gossh.Client, error) {
		return nil, nil
	}
	cc.RunExec = func(client *gossh.Client, opts fkexec.RunOpts, stdout, stderr io.Writer) (*fkexec.RunResult, error) {
		return &fkexec.RunResult{
			RunID:     opts.RunID,
			Command:   opts.Command,
			ExitCode:  1,
			StartTime: time.Now().UTC(),
			EndTime:   time.Now().UTC().Add(3 * time.Second),
		}, nil
	}
	cc.NewStorage = func(ctx context.Context) (*fkstorage.Store, error) {
		return nil, fmt.Errorf("test: no S3")
	}

	exitCode, err := RunCommand(context.Background(), cc, "cargo test")
	require.NoError(t, err)
	assert.Equal(t, 1, exitCode)

	// Should still save history.
	history, err := cc.State.LoadRunHistory(cc.Project.Hash)
	require.NoError(t, err)
	require.Len(t, history, 1)
	assert.Equal(t, 1, history[0].ExitCode)
}

func TestRunCommand_UploadsArtifacts(t *testing.T) {
	t.Parallel()

	var uploadedKeys []string
	s3mock := &testS3{
		putObjectFn: func(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
			uploadedKeys = append(uploadedKeys, *params.Key)
			return &s3.PutObjectOutput{}, nil
		},
	}

	prov := &mockProvider{
		findVMFn: func(ctx context.Context, projectHash string) (*provider.VMInfo, error) {
			return &provider.VMInfo{
				InstanceID:       "i-art001",
				State:            "running",
				PublicIP:         "10.0.0.1",
				Region:           "us-east-1",
				AvailabilityZone: "us-east-1a",
			}, nil
		},
	}
	cc, stdout, _ := testCmdContext(t, prov)
	saveTestVMState(t, cc.State, cc.Project.Hash)

	// Configure artifacts.
	cc.Config.Artifacts.Paths = []string{"output/result.txt", "coverage/report.html"}

	cc.RunSync = func(ctx context.Context, cc *cmdContext, vmInfo *provider.VMInfo) (*fksync.SyncResult, error) {
		return &fksync.SyncResult{}, nil
	}
	cc.ConnectSSH = func(ctx context.Context, vmInfo *provider.VMInfo) (*gossh.Client, error) {
		return nil, nil
	}
	cc.RunExec = func(client *gossh.Client, opts fkexec.RunOpts, stdout, stderr io.Writer) (*fkexec.RunResult, error) {
		return &fkexec.RunResult{
			RunID:     opts.RunID,
			Command:   opts.Command,
			ExitCode:  0,
			StartTime: time.Now().UTC(),
			EndTime:   time.Now().UTC().Add(5 * time.Second),
		}, nil
	}
	cc.NewStorage = mockStorageFactory(s3mock)
	cc.ReadRemoteFile = func(client *gossh.Client, remotePath string) ([]byte, error) {
		switch remotePath {
		case "/home/ubuntu/project/output/result.txt":
			return []byte("build complete\n"), nil
		case "/home/ubuntu/project/coverage/report.html":
			return []byte("<html>coverage</html>"), nil
		default:
			return nil, fmt.Errorf("file not found: %s", remotePath)
		}
	}

	exitCode, err := RunCommand(context.Background(), cc, "go run main.go")
	require.NoError(t, err)
	assert.Equal(t, 0, exitCode)

	// Verify artifacts were uploaded.
	out := stdout.String()
	assert.Contains(t, out, "uploaded 2 artifact(s)")

	// Verify S3 keys include artifact paths.
	var artifactKeys []string
	for _, key := range uploadedKeys {
		if strings.Contains(key, "/artifacts/") {
			artifactKeys = append(artifactKeys, key)
		}
	}
	assert.Len(t, artifactKeys, 2)
	assert.Contains(t, artifactKeys[0], "artifacts/output/result.txt")
	assert.Contains(t, artifactKeys[1], "artifacts/coverage/report.html")
}

func TestRunCommand_ArtifactMissing_ContinuesUpload(t *testing.T) {
	t.Parallel()

	var uploadedKeys []string
	s3mock := &testS3{
		putObjectFn: func(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
			uploadedKeys = append(uploadedKeys, *params.Key)
			return &s3.PutObjectOutput{}, nil
		},
	}

	prov := &mockProvider{
		findVMFn: func(ctx context.Context, projectHash string) (*provider.VMInfo, error) {
			return &provider.VMInfo{
				InstanceID:       "i-art002",
				State:            "running",
				PublicIP:         "10.0.0.1",
				Region:           "us-east-1",
				AvailabilityZone: "us-east-1a",
			}, nil
		},
	}
	cc, stdout, _ := testCmdContext(t, prov)
	saveTestVMState(t, cc.State, cc.Project.Hash)

	// Two artifacts: first exists, second does not.
	cc.Config.Artifacts.Paths = []string{"output/result.txt", "missing/file.txt"}

	cc.RunSync = func(ctx context.Context, cc *cmdContext, vmInfo *provider.VMInfo) (*fksync.SyncResult, error) {
		return &fksync.SyncResult{}, nil
	}
	cc.ConnectSSH = func(ctx context.Context, vmInfo *provider.VMInfo) (*gossh.Client, error) {
		return nil, nil
	}
	cc.RunExec = func(client *gossh.Client, opts fkexec.RunOpts, stdout, stderr io.Writer) (*fkexec.RunResult, error) {
		return &fkexec.RunResult{
			RunID:     opts.RunID,
			Command:   opts.Command,
			ExitCode:  0,
			StartTime: time.Now().UTC(),
			EndTime:   time.Now().UTC().Add(2 * time.Second),
		}, nil
	}
	cc.NewStorage = mockStorageFactory(s3mock)
	cc.ReadRemoteFile = func(client *gossh.Client, remotePath string) ([]byte, error) {
		if remotePath == "/home/ubuntu/project/output/result.txt" {
			return []byte("data"), nil
		}
		return nil, fmt.Errorf("file not found: %s", remotePath)
	}

	exitCode, err := RunCommand(context.Background(), cc, "make build")
	require.NoError(t, err)
	assert.Equal(t, 0, exitCode)

	out := stdout.String()
	// Should warn about missing file.
	assert.Contains(t, out, "artifact missing/file.txt not found")
	// Should still upload the one that exists.
	assert.Contains(t, out, "uploaded 1 artifact(s)")
}

func TestRunCommand_NoArtifactsConfigured_SkipsUpload(t *testing.T) {
	t.Parallel()

	readFileCalled := false

	prov := &mockProvider{
		findVMFn: func(ctx context.Context, projectHash string) (*provider.VMInfo, error) {
			return &provider.VMInfo{
				InstanceID:       "i-noart001",
				State:            "running",
				PublicIP:         "10.0.0.1",
				Region:           "us-east-1",
				AvailabilityZone: "us-east-1a",
			}, nil
		},
	}
	cc, stdout, _ := testCmdContext(t, prov)
	saveTestVMState(t, cc.State, cc.Project.Hash)

	// No artifacts configured (default config).
	cc.RunSync = func(ctx context.Context, cc *cmdContext, vmInfo *provider.VMInfo) (*fksync.SyncResult, error) {
		return &fksync.SyncResult{}, nil
	}
	cc.ConnectSSH = func(ctx context.Context, vmInfo *provider.VMInfo) (*gossh.Client, error) {
		return nil, nil
	}
	cc.RunExec = func(client *gossh.Client, opts fkexec.RunOpts, stdout, stderr io.Writer) (*fkexec.RunResult, error) {
		return &fkexec.RunResult{
			RunID:     opts.RunID,
			Command:   opts.Command,
			ExitCode:  0,
			StartTime: time.Now().UTC(),
			EndTime:   time.Now().UTC().Add(1 * time.Second),
		}, nil
	}
	cc.NewStorage = func(ctx context.Context) (*fkstorage.Store, error) {
		return nil, fmt.Errorf("test: no S3")
	}
	cc.ReadRemoteFile = func(client *gossh.Client, remotePath string) ([]byte, error) {
		readFileCalled = true
		return nil, fmt.Errorf("should not be called")
	}

	exitCode, err := RunCommand(context.Background(), cc, "cargo test")
	require.NoError(t, err)
	assert.Equal(t, 0, exitCode)

	// ReadRemoteFile should never be called.
	assert.False(t, readFileCalled, "ReadRemoteFile should not be called when no artifacts configured")
	// Should not mention artifacts in output.
	assert.NotContains(t, stdout.String(), "artifact")
}

func TestExitCodeError(t *testing.T) {
	t.Parallel()

	err := &exitCodeError{code: 42}
	assert.Equal(t, "", err.Error())

	code := Execute("test", []string{"--help"})
	assert.Equal(t, 0, code)
}

func TestExitCodePropagation(t *testing.T) {
	t.Parallel()

	// Verify that exitCodeError is detected in Execute.
	ece := &exitCodeError{code: 5}
	assert.Equal(t, "", ece.Error())
	assert.Equal(t, 5, ece.code)
}
