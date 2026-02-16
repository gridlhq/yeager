package cli

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/gridlhq/yeager/internal/config"
	fkexec "github.com/gridlhq/yeager/internal/exec"
	"github.com/gridlhq/yeager/internal/output"
	"github.com/gridlhq/yeager/internal/project"
	"github.com/gridlhq/yeager/internal/provider"
	"github.com/gridlhq/yeager/internal/state"
	fkstorage "github.com/gridlhq/yeager/internal/storage"
	gossh "golang.org/x/crypto/ssh"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Mock provider ---

type mockProvider struct {
	accountIDFn          func(ctx context.Context) (string, error)
	ensureSecurityGroupFn func(ctx context.Context) (string, error)
	ensureBucketFn       func(ctx context.Context) error
	createVMFn           func(ctx context.Context, opts provider.CreateVMOpts) (provider.VMInfo, error)
	findVMFn             func(ctx context.Context, projectHash string) (*provider.VMInfo, error)
	startVMFn            func(ctx context.Context, instanceID string) error
	stopVMFn             func(ctx context.Context, instanceID string) error
	terminateVMFn        func(ctx context.Context, instanceID string) error
	waitUntilRunningFn   func(ctx context.Context, instanceID string) error
	regionVal            string
	bucketNameFn         func(ctx context.Context) (string, error)
}

func (m *mockProvider) AccountID(ctx context.Context) (string, error) {
	if m.accountIDFn != nil {
		return m.accountIDFn(ctx)
	}
	return "123456789012", nil
}
func (m *mockProvider) EnsureSecurityGroup(ctx context.Context) (string, error) {
	if m.ensureSecurityGroupFn != nil {
		return m.ensureSecurityGroupFn(ctx)
	}
	return "sg-test", nil
}
func (m *mockProvider) EnsureBucket(ctx context.Context) error {
	if m.ensureBucketFn != nil {
		return m.ensureBucketFn(ctx)
	}
	return nil
}
func (m *mockProvider) CreateVM(ctx context.Context, opts provider.CreateVMOpts) (provider.VMInfo, error) {
	if m.createVMFn != nil {
		return m.createVMFn(ctx, opts)
	}
	return provider.VMInfo{
		InstanceID: "i-new001",
		State:      "pending",
		Region:     "us-east-1",
	}, nil
}
func (m *mockProvider) FindVM(ctx context.Context, projectHash string) (*provider.VMInfo, error) {
	if m.findVMFn != nil {
		return m.findVMFn(ctx, projectHash)
	}
	return nil, nil
}
func (m *mockProvider) StartVM(ctx context.Context, instanceID string) error {
	if m.startVMFn != nil {
		return m.startVMFn(ctx, instanceID)
	}
	return nil
}
func (m *mockProvider) StopVM(ctx context.Context, instanceID string) error {
	if m.stopVMFn != nil {
		return m.stopVMFn(ctx, instanceID)
	}
	return nil
}
func (m *mockProvider) TerminateVM(ctx context.Context, instanceID string) error {
	if m.terminateVMFn != nil {
		return m.terminateVMFn(ctx, instanceID)
	}
	return nil
}
func (m *mockProvider) WaitUntilRunning(ctx context.Context, instanceID string) error {
	if m.waitUntilRunningFn != nil {
		return m.waitUntilRunningFn(ctx, instanceID)
	}
	return nil
}
func (m *mockProvider) WaitUntilRunningWithProgress(ctx context.Context, instanceID string, progressCallback provider.ProgressCallback) error {
	if m.waitUntilRunningFn != nil {
		return m.waitUntilRunningFn(ctx, instanceID)
	}
	return nil
}
func (m *mockProvider) Region() string {
	if m.regionVal != "" {
		return m.regionVal
	}
	return "us-east-1"
}
func (m *mockProvider) BucketName(ctx context.Context) (string, error) {
	if m.bucketNameFn != nil {
		return m.bucketNameFn(ctx)
	}
	return "yeager-123456789012", nil
}

// --- Test helpers ---

func testProject() project.Project {
	return project.Project{
		AbsPath:     "/home/user/myproject",
		Hash:        "abc123def456",
		DisplayName: "myproject",
	}
}

func testCmdContext(t *testing.T, prov *mockProvider) (*cmdContext, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	store, err := state.NewStore(t.TempDir())
	require.NoError(t, err)
	return &cmdContext{
		Project:  testProject(),
		Config:   config.Defaults(),
		Provider: prov,
		State:    store,
		Output:   output.NewWithWriters(&stdout, &stderr, output.ModeText),
	}, &stdout, &stderr
}

// saveTestVMState saves a VM state to the store for testing.
func saveTestVMState(t *testing.T, store *state.Store, hash string) {
	t.Helper()
	err := store.SaveVM(hash, state.VMState{
		InstanceID: "i-existing001",
		Region:     "us-east-1",
		Created:    time.Now().UTC(),
		ProjectDir: "/home/user/myproject",
	})
	require.NoError(t, err)
}

// --- Status tests ---

func TestRunStatus(t *testing.T) {
	t.Parallel()

	t.Run("no VM found shows helpful message", func(t *testing.T) {
		t.Parallel()
		cc, stdout, _ := testCmdContext(t, &mockProvider{})
		err := RunStatus(context.Background(), cc)
		require.NoError(t, err)
		assert.Contains(t, stdout.String(), "myproject")
		assert.Contains(t, stdout.String(), "no VM found")
	})

	t.Run("running VM shows instance ID and IP", func(t *testing.T) {
		t.Parallel()
		prov := &mockProvider{
			findVMFn: func(ctx context.Context, projectHash string) (*provider.VMInfo, error) {
				return &provider.VMInfo{
					InstanceID: "i-run001",
					State:      "running",
					PublicIP:   "1.2.3.4",
					Region:     "us-east-1",
				}, nil
			},
		}
		cc, stdout, _ := testCmdContext(t, prov)
		saveTestVMState(t, cc.State, cc.Project.Hash)

		err := RunStatus(context.Background(), cc)
		require.NoError(t, err)
		assert.Contains(t, stdout.String(), "i-run001")
		assert.Contains(t, stdout.String(), "running")
		assert.Contains(t, stdout.String(), "1.2.3.4")
		// Cost indicator should appear for running VMs.
		assert.Contains(t, stdout.String(), "~$0.034/hr")
	})

	t.Run("stopped VM suggests yg up", func(t *testing.T) {
		t.Parallel()
		prov := &mockProvider{
			findVMFn: func(ctx context.Context, projectHash string) (*provider.VMInfo, error) {
				return &provider.VMInfo{
					InstanceID: "i-stop001",
					State:      "stopped",
					Region:     "us-east-1",
				}, nil
			},
		}
		cc, stdout, _ := testCmdContext(t, prov)
		saveTestVMState(t, cc.State, cc.Project.Hash)

		err := RunStatus(context.Background(), cc)
		require.NoError(t, err)
		assert.Contains(t, stdout.String(), "stopped")
		assert.Contains(t, stdout.String(), "yg up")
	})

	t.Run("VM gone from AWS shows helpful message", func(t *testing.T) {
		t.Parallel()
		prov := &mockProvider{
			findVMFn: func(ctx context.Context, projectHash string) (*provider.VMInfo, error) {
				return nil, nil
			},
		}
		cc, stdout, _ := testCmdContext(t, prov)
		saveTestVMState(t, cc.State, cc.Project.Hash)

		err := RunStatus(context.Background(), cc)
		require.NoError(t, err)
		assert.Contains(t, stdout.String(), "no longer exists")
	})
}

// --- Enhanced status tests (active commands + history) ---

func TestRunStatus_ShowsActiveCommands(t *testing.T) {
	t.Parallel()

	prov := &mockProvider{
		findVMFn: func(ctx context.Context, projectHash string) (*provider.VMInfo, error) {
			return &provider.VMInfo{
				InstanceID:       "i-active001",
				State:            "running",
				PublicIP:         "1.2.3.4",
				Region:           "us-east-1",
				AvailabilityZone: "us-east-1a",
			}, nil
		},
	}
	cc, stdout, _ := testCmdContext(t, prov)
	saveTestVMState(t, cc.State, cc.Project.Hash)

	// Mock SSH + ListRuns.
	cc.ConnectSSH = func(ctx context.Context, vmInfo *provider.VMInfo) (*gossh.Client, error) {
		return nil, nil
	}
	cc.ListRuns = func(client *gossh.Client) ([]fkexec.ActiveRun, error) {
		return []fkexec.ActiveRun{
			{RunID: "aaa11111", Command: "cargo test", StartTime: time.Now().UTC().Add(-30 * time.Second)},
			{RunID: "bbb22222", Command: "npm run build"},
		}, nil
	}

	err := RunStatus(context.Background(), cc)
	require.NoError(t, err)
	out := stdout.String()
	assert.Contains(t, out, "active commands: 2")
	assert.Contains(t, out, "aaa11111")
	assert.Contains(t, out, "cargo test")
	assert.Contains(t, out, "bbb22222")
	assert.Contains(t, out, "npm run build")
}

func TestRunStatus_ShowsNoActiveCommands(t *testing.T) {
	t.Parallel()

	prov := &mockProvider{
		findVMFn: func(ctx context.Context, projectHash string) (*provider.VMInfo, error) {
			return &provider.VMInfo{
				InstanceID:       "i-idle001",
				State:            "running",
				PublicIP:         "1.2.3.4",
				Region:           "us-east-1",
				AvailabilityZone: "us-east-1a",
			}, nil
		},
	}
	cc, stdout, _ := testCmdContext(t, prov)
	saveTestVMState(t, cc.State, cc.Project.Hash)

	cc.ConnectSSH = func(ctx context.Context, vmInfo *provider.VMInfo) (*gossh.Client, error) {
		return nil, nil
	}
	cc.ListRuns = func(client *gossh.Client) ([]fkexec.ActiveRun, error) {
		return nil, nil
	}

	err := RunStatus(context.Background(), cc)
	require.NoError(t, err)
	assert.Contains(t, stdout.String(), "no active commands")
}

func TestRunStatus_SSHFailureIsBestEffort(t *testing.T) {
	t.Parallel()

	prov := &mockProvider{
		findVMFn: func(ctx context.Context, projectHash string) (*provider.VMInfo, error) {
			return &provider.VMInfo{
				InstanceID:       "i-sshfail",
				State:            "running",
				PublicIP:         "1.2.3.4",
				Region:           "us-east-1",
				AvailabilityZone: "us-east-1a",
			}, nil
		},
	}
	cc, stdout, _ := testCmdContext(t, prov)
	saveTestVMState(t, cc.State, cc.Project.Hash)

	cc.ConnectSSH = func(ctx context.Context, vmInfo *provider.VMInfo) (*gossh.Client, error) {
		return nil, fmt.Errorf("connection refused")
	}
	cc.ListRuns = func(client *gossh.Client) ([]fkexec.ActiveRun, error) {
		t.Fatal("ListRuns should not be called when SSH fails")
		return nil, nil
	}

	// Should succeed even though SSH failed — best-effort.
	err := RunStatus(context.Background(), cc)
	require.NoError(t, err)
	assert.Contains(t, stdout.String(), "running")
	assert.NotContains(t, stdout.String(), "active commands")
}

func TestRunStatus_ShowsRunHistory(t *testing.T) {
	t.Parallel()

	prov := &mockProvider{
		findVMFn: func(ctx context.Context, projectHash string) (*provider.VMInfo, error) {
			return &provider.VMInfo{
				InstanceID: "i-hist001",
				State:      "stopped",
				Region:     "us-east-1",
			}, nil
		},
	}
	cc, stdout, _ := testCmdContext(t, prov)
	saveTestVMState(t, cc.State, cc.Project.Hash)

	// Save some history entries.
	for i := 0; i < 3; i++ {
		err := cc.State.SaveRunHistory(cc.Project.Hash, state.RunHistoryEntry{
			RunID:    fmt.Sprintf("hist%04d", i),
			Command:  fmt.Sprintf("cmd%d", i),
			ExitCode: i,
			Duration: time.Duration(i+1) * time.Second,
		})
		require.NoError(t, err)
	}

	err := RunStatus(context.Background(), cc)
	require.NoError(t, err)
	out := stdout.String()
	assert.Contains(t, out, "recent runs:")
	assert.Contains(t, out, "hist0000")
	assert.Contains(t, out, "cmd0")
	assert.Contains(t, out, "hist0002")
	assert.Contains(t, out, "cmd2")
}

func TestRunStatus_HistoryCapsAtFive(t *testing.T) {
	t.Parallel()

	prov := &mockProvider{
		findVMFn: func(ctx context.Context, projectHash string) (*provider.VMInfo, error) {
			return &provider.VMInfo{
				InstanceID: "i-cap001",
				State:      "stopped",
				Region:     "us-east-1",
			}, nil
		},
	}
	cc, stdout, _ := testCmdContext(t, prov)
	saveTestVMState(t, cc.State, cc.Project.Hash)

	// Save 8 entries.
	for i := 0; i < 8; i++ {
		err := cc.State.SaveRunHistory(cc.Project.Hash, state.RunHistoryEntry{
			RunID:   fmt.Sprintf("run%02d", i),
			Command: fmt.Sprintf("cmd%d", i),
		})
		require.NoError(t, err)
	}

	err := RunStatus(context.Background(), cc)
	require.NoError(t, err)
	out := stdout.String()
	// Should show only the last 5.
	assert.NotContains(t, out, "run00")
	assert.NotContains(t, out, "run01")
	assert.NotContains(t, out, "run02")
	assert.Contains(t, out, "run03")
	assert.Contains(t, out, "run07")
}

// --- LoadVM error propagation (non-ErrNotExist errors) ---

func TestRunStatus_LoadVMCorruptError(t *testing.T) {
	t.Parallel()

	cc, _, _ := testCmdContext(t, &mockProvider{})
	// Write corrupt state file to trigger non-ErrNotExist error.
	err := cc.State.SaveVM(cc.Project.Hash, state.VMState{InstanceID: "i-test", Region: "us-east-1", Created: time.Now()})
	require.NoError(t, err)
	corruptFile := fmt.Sprintf("%s/projects/%s/vm.json", cc.State.BaseDir(), cc.Project.Hash)
	require.NoError(t, os.WriteFile(corruptFile, []byte("not json{{{"), 0o644))

	err = RunStatus(context.Background(), cc)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "loading VM state")
}

func TestRunStop_LoadVMCorruptError(t *testing.T) {
	t.Parallel()

	cc, _, _ := testCmdContext(t, &mockProvider{})
	err := cc.State.SaveVM(cc.Project.Hash, state.VMState{InstanceID: "i-test", Region: "us-east-1", Created: time.Now()})
	require.NoError(t, err)
	corruptFile := fmt.Sprintf("%s/projects/%s/vm.json", cc.State.BaseDir(), cc.Project.Hash)
	require.NoError(t, os.WriteFile(corruptFile, []byte("not json{{{"), 0o644))

	err = RunStop(context.Background(), cc)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "loading VM state")
}

func TestRunDestroy_LoadVMCorruptError(t *testing.T) {
	t.Parallel()

	cc, _, _ := testCmdContext(t, &mockProvider{})
	err := cc.State.SaveVM(cc.Project.Hash, state.VMState{InstanceID: "i-test", Region: "us-east-1", Created: time.Now()})
	require.NoError(t, err)
	corruptFile := fmt.Sprintf("%s/projects/%s/vm.json", cc.State.BaseDir(), cc.Project.Hash)
	require.NoError(t, os.WriteFile(corruptFile, []byte("not json{{{"), 0o644))

	err = RunDestroy(context.Background(), cc)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "loading VM state")
}

func TestRunUp_LoadVMCorruptError(t *testing.T) {
	t.Parallel()

	cc, _, _ := testCmdContext(t, &mockProvider{})
	err := cc.State.SaveVM(cc.Project.Hash, state.VMState{InstanceID: "i-test", Region: "us-east-1", Created: time.Now()})
	require.NoError(t, err)
	corruptFile := fmt.Sprintf("%s/projects/%s/vm.json", cc.State.BaseDir(), cc.Project.Hash)
	require.NoError(t, os.WriteFile(corruptFile, []byte("not json{{{"), 0o644))

	err = RunUp(context.Background(), cc)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "loading VM state")
}

// --- Status error propagation ---

func TestRunStatus_FindVMError(t *testing.T) {
	t.Parallel()

	prov := &mockProvider{
		findVMFn: func(ctx context.Context, projectHash string) (*provider.VMInfo, error) {
			return nil, fmt.Errorf("access denied")
		},
	}
	cc, _, _ := testCmdContext(t, prov)
	saveTestVMState(t, cc.State, cc.Project.Hash)

	err := RunStatus(context.Background(), cc)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "querying VM state")
}

func TestRunStatus_PendingVM(t *testing.T) {
	t.Parallel()

	prov := &mockProvider{
		findVMFn: func(ctx context.Context, projectHash string) (*provider.VMInfo, error) {
			return &provider.VMInfo{
				InstanceID: "i-pend001",
				State:      "pending",
				Region:     "us-east-1",
			}, nil
		},
	}
	cc, stdout, _ := testCmdContext(t, prov)
	saveTestVMState(t, cc.State, cc.Project.Hash)

	err := RunStatus(context.Background(), cc)
	require.NoError(t, err)
	assert.Contains(t, stdout.String(), "starting")
	assert.Contains(t, stdout.String(), "i-pend001")
}

func TestRunStatus_RunningNoIP(t *testing.T) {
	t.Parallel()

	prov := &mockProvider{
		findVMFn: func(ctx context.Context, projectHash string) (*provider.VMInfo, error) {
			return &provider.VMInfo{
				InstanceID: "i-noip001",
				State:      "running",
				Region:     "us-east-1",
			}, nil
		},
	}
	cc, stdout, _ := testCmdContext(t, prov)
	saveTestVMState(t, cc.State, cc.Project.Hash)

	err := RunStatus(context.Background(), cc)
	require.NoError(t, err)
	assert.Contains(t, stdout.String(), "i-noip001")
	assert.Contains(t, stdout.String(), "running")
	assert.NotContains(t, stdout.String(), "1.2.3.4")
}

// --- Stop tests ---

func TestRunStop(t *testing.T) {
	t.Parallel()

	t.Run("no VM found", func(t *testing.T) {
		t.Parallel()
		cc, stdout, _ := testCmdContext(t, &mockProvider{})
		err := RunStop(context.Background(), cc)
		require.NoError(t, err)
		assert.Contains(t, stdout.String(), "nothing to stop")
	})

	t.Run("stops running VM", func(t *testing.T) {
		t.Parallel()
		stopCalled := false
		prov := &mockProvider{
			findVMFn: func(ctx context.Context, projectHash string) (*provider.VMInfo, error) {
				return &provider.VMInfo{
					InstanceID: "i-stop001",
					State:      "running",
					Region:     "us-east-1",
				}, nil
			},
			stopVMFn: func(ctx context.Context, instanceID string) error {
				assert.Equal(t, "i-stop001", instanceID)
				stopCalled = true
				return nil
			},
		}
		cc, stdout, _ := testCmdContext(t, prov)
		saveTestVMState(t, cc.State, cc.Project.Hash)

		err := RunStop(context.Background(), cc)
		require.NoError(t, err)
		assert.True(t, stopCalled)
		assert.Contains(t, stdout.String(), "VM stopped")
	})

	t.Run("already stopped VM is no-op", func(t *testing.T) {
		t.Parallel()
		prov := &mockProvider{
			findVMFn: func(ctx context.Context, projectHash string) (*provider.VMInfo, error) {
				return &provider.VMInfo{
					InstanceID: "i-already",
					State:      "stopped",
					Region:     "us-east-1",
				}, nil
			},
		}
		cc, stdout, _ := testCmdContext(t, prov)
		saveTestVMState(t, cc.State, cc.Project.Hash)

		err := RunStop(context.Background(), cc)
		require.NoError(t, err)
		assert.Contains(t, stdout.String(), "already stopped")
	})

	t.Run("VM in stopping state cannot be stopped", func(t *testing.T) {
		t.Parallel()
		prov := &mockProvider{
			findVMFn: func(ctx context.Context, projectHash string) (*provider.VMInfo, error) {
				return &provider.VMInfo{
					InstanceID: "i-stopping",
					State:      "stopping",
					Region:     "us-east-1",
				}, nil
			},
		}
		cc, stdout, _ := testCmdContext(t, prov)
		saveTestVMState(t, cc.State, cc.Project.Hash)

		err := RunStop(context.Background(), cc)
		require.NoError(t, err)
		assert.Contains(t, stdout.String(), "cannot stop")
	})

	t.Run("FindVM error propagates", func(t *testing.T) {
		t.Parallel()
		prov := &mockProvider{
			findVMFn: func(ctx context.Context, projectHash string) (*provider.VMInfo, error) {
				return nil, fmt.Errorf("network error")
			},
		}
		cc, _, _ := testCmdContext(t, prov)
		saveTestVMState(t, cc.State, cc.Project.Hash)

		err := RunStop(context.Background(), cc)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "querying VM state")
	})

	t.Run("VM gone from AWS", func(t *testing.T) {
		t.Parallel()
		prov := &mockProvider{
			findVMFn: func(ctx context.Context, projectHash string) (*provider.VMInfo, error) {
				return nil, nil
			},
		}
		cc, stdout, _ := testCmdContext(t, prov)
		saveTestVMState(t, cc.State, cc.Project.Hash)

		err := RunStop(context.Background(), cc)
		require.NoError(t, err)
		assert.Contains(t, stdout.String(), "no longer exists")
	})

	t.Run("propagates stop error", func(t *testing.T) {
		t.Parallel()
		prov := &mockProvider{
			findVMFn: func(ctx context.Context, projectHash string) (*provider.VMInfo, error) {
				return &provider.VMInfo{
					InstanceID: "i-err001",
					State:      "running",
					Region:     "us-east-1",
				}, nil
			},
			stopVMFn: func(ctx context.Context, instanceID string) error {
				return fmt.Errorf("API error")
			},
		}
		cc, _, _ := testCmdContext(t, prov)
		saveTestVMState(t, cc.State, cc.Project.Hash)

		err := RunStop(context.Background(), cc)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "API error")
	})
}

// --- Destroy tests ---

func TestRunDestroy(t *testing.T) {
	t.Parallel()

	t.Run("no VM found", func(t *testing.T) {
		t.Parallel()
		cc, stdout, _ := testCmdContext(t, &mockProvider{})
		err := RunDestroy(context.Background(), cc)
		require.NoError(t, err)
		assert.Contains(t, stdout.String(), "nothing to destroy")
	})

	t.Run("terminates running VM and cleans state", func(t *testing.T) {
		t.Parallel()
		terminateCalled := false
		prov := &mockProvider{
			findVMFn: func(ctx context.Context, projectHash string) (*provider.VMInfo, error) {
				return &provider.VMInfo{
					InstanceID: "i-term001",
					State:      "running",
					Region:     "us-east-1",
				}, nil
			},
			terminateVMFn: func(ctx context.Context, instanceID string) error {
				assert.Equal(t, "i-term001", instanceID)
				terminateCalled = true
				return nil
			},
		}
		cc, stdout, _ := testCmdContext(t, prov)
		saveTestVMState(t, cc.State, cc.Project.Hash)

		err := RunDestroy(context.Background(), cc)
		require.NoError(t, err)
		assert.True(t, terminateCalled)
		assert.Contains(t, stdout.String(), "destroyed")

		// Local state should be cleaned up.
		_, loadErr := cc.State.LoadVM(cc.Project.Hash)
		require.Error(t, loadErr, "state file should be deleted after destroy")
	})

	t.Run("cleans state when VM already gone from AWS", func(t *testing.T) {
		t.Parallel()
		prov := &mockProvider{
			findVMFn: func(ctx context.Context, projectHash string) (*provider.VMInfo, error) {
				return nil, nil
			},
		}
		cc, stdout, _ := testCmdContext(t, prov)
		saveTestVMState(t, cc.State, cc.Project.Hash)

		err := RunDestroy(context.Background(), cc)
		require.NoError(t, err)
		assert.Contains(t, stdout.String(), "no longer exists")
		assert.Contains(t, stdout.String(), "destroyed")
	})

	t.Run("propagates FindVM error", func(t *testing.T) {
		t.Parallel()
		prov := &mockProvider{
			findVMFn: func(ctx context.Context, projectHash string) (*provider.VMInfo, error) {
				return nil, fmt.Errorf("describe failed")
			},
		}
		cc, _, _ := testCmdContext(t, prov)
		saveTestVMState(t, cc.State, cc.Project.Hash)

		err := RunDestroy(context.Background(), cc)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "querying VM state")
	})

	t.Run("propagates TerminateVM error", func(t *testing.T) {
		t.Parallel()
		prov := &mockProvider{
			findVMFn: func(ctx context.Context, projectHash string) (*provider.VMInfo, error) {
				return &provider.VMInfo{
					InstanceID: "i-termerr",
					State:      "running",
					Region:     "us-east-1",
				}, nil
			},
			terminateVMFn: func(ctx context.Context, instanceID string) error {
				return fmt.Errorf("terminate denied")
			},
		}
		cc, _, _ := testCmdContext(t, prov)
		saveTestVMState(t, cc.State, cc.Project.Hash)

		err := RunDestroy(context.Background(), cc)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "terminate denied")
	})

	t.Run("shows warning about data loss without --force", func(t *testing.T) {
		t.Parallel()
		prov := &mockProvider{
			findVMFn: func(ctx context.Context, projectHash string) (*provider.VMInfo, error) {
				return &provider.VMInfo{
					InstanceID: "i-warn001",
					State:      "running",
					Region:     "us-east-1",
				}, nil
			},
		}
		cc, stdout, _ := testCmdContext(t, prov)
		saveTestVMState(t, cc.State, cc.Project.Hash)

		// Without --force, should show warning and exit without destroying
		err := RunDestroyWithOptions(context.Background(), cc, DestroyOptions{Force: false})
		require.NoError(t, err, "should exit cleanly with warning, not error")
		output := stdout.String()
		assert.Contains(t, output, "warning:", "should show warning")
		assert.Contains(t, output, "cached build artifacts", "should mention cached artifacts")
		assert.Contains(t, output, "installed packages", "should mention installed packages")
		assert.Contains(t, output, "accumulated state", "should mention accumulated state")
		assert.Contains(t, output, "--force", "should mention --force flag")

		// VM should still exist (not destroyed)
		_, loadErr := cc.State.LoadVM(cc.Project.Hash)
		require.NoError(t, loadErr, "state file should still exist when destroy is cancelled")
	})

	t.Run("proceeds with --force flag", func(t *testing.T) {
		t.Parallel()
		terminateCalled := false
		prov := &mockProvider{
			findVMFn: func(ctx context.Context, projectHash string) (*provider.VMInfo, error) {
				return &provider.VMInfo{
					InstanceID: "i-force001",
					State:      "running",
					Region:     "us-east-1",
				}, nil
			},
			terminateVMFn: func(ctx context.Context, instanceID string) error {
				terminateCalled = true
				return nil
			},
		}
		cc, stdout, _ := testCmdContext(t, prov)
		saveTestVMState(t, cc.State, cc.Project.Hash)

		// With --force, should proceed without warning
		err := RunDestroyWithOptions(context.Background(), cc, DestroyOptions{Force: true})
		require.NoError(t, err)
		assert.True(t, terminateCalled, "VM should be terminated with --force")
		assert.Contains(t, stdout.String(), "destroyed")

		// VM state should be cleaned up
		_, loadErr := cc.State.LoadVM(cc.Project.Hash)
		require.Error(t, loadErr, "state file should be deleted after destroy")
	})
}

// --- Up tests ---

func TestRunUp(t *testing.T) {
	t.Parallel()

	t.Run("creates new VM when no state exists", func(t *testing.T) {
		t.Parallel()
		createCalled := false
		waitCalled := false
		prov := &mockProvider{
			createVMFn: func(ctx context.Context, opts provider.CreateVMOpts) (provider.VMInfo, error) {
				assert.Equal(t, "abc123def456", opts.ProjectHash)
				assert.Equal(t, "/home/user/myproject", opts.ProjectPath)
				assert.Equal(t, "medium", opts.Size)
				assert.Equal(t, "sg-test", opts.SecurityGroupID)
				assert.NotEmpty(t, opts.UserData, "UserData must be set (cloud-init)")
				createCalled = true
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
			waitUntilRunningFn: func(ctx context.Context, instanceID string) error {
				assert.Equal(t, "i-new001", instanceID)
				waitCalled = true
				return nil
			},
		}
		cc, stdout, _ := testCmdContext(t, prov)

		err := RunUp(context.Background(), cc)
		require.NoError(t, err)
		assert.True(t, createCalled)
		assert.True(t, waitCalled)
		assert.Contains(t, stdout.String(), "creating one")
		assert.Contains(t, stdout.String(), "VM running")

		// Should have saved state with setup hash.
		vmState, err := cc.State.LoadVM(cc.Project.Hash)
		require.NoError(t, err)
		assert.Equal(t, "i-new001", vmState.InstanceID)
		assert.NotEmpty(t, vmState.SetupHash, "setup hash must be saved")
	})

	t.Run("noop when VM already running", func(t *testing.T) {
		t.Parallel()
		prov := &mockProvider{
			findVMFn: func(ctx context.Context, projectHash string) (*provider.VMInfo, error) {
				return &provider.VMInfo{
					InstanceID: "i-running",
					State:      "running",
					Region:     "us-east-1",
				}, nil
			},
		}
		cc, stdout, _ := testCmdContext(t, prov)
		saveTestVMState(t, cc.State, cc.Project.Hash)

		err := RunUp(context.Background(), cc)
		require.NoError(t, err)
		assert.Contains(t, stdout.String(), "VM running")
		assert.Contains(t, stdout.String(), "i-running")
	})

	t.Run("starts stopped VM", func(t *testing.T) {
		t.Parallel()
		startCalled := false
		prov := &mockProvider{
			findVMFn: func(ctx context.Context, projectHash string) (*provider.VMInfo, error) {
				return &provider.VMInfo{
					InstanceID: "i-stopped",
					State:      "stopped",
					Region:     "us-east-1",
				}, nil
			},
			startVMFn: func(ctx context.Context, instanceID string) error {
				assert.Equal(t, "i-stopped", instanceID)
				startCalled = true
				return nil
			},
			waitUntilRunningFn: func(ctx context.Context, instanceID string) error {
				return nil
			},
		}
		cc, stdout, _ := testCmdContext(t, prov)
		saveTestVMState(t, cc.State, cc.Project.Hash)

		err := RunUp(context.Background(), cc)
		require.NoError(t, err)
		assert.True(t, startCalled)
		assert.Contains(t, stdout.String(), "starting stopped VM")
		assert.Contains(t, stdout.String(), "VM running")
	})

	t.Run("creates new VM when previous VM gone from AWS", func(t *testing.T) {
		t.Parallel()
		createCalled := false
		findCalls := 0
		prov := &mockProvider{
			findVMFn: func(ctx context.Context, projectHash string) (*provider.VMInfo, error) {
				findCalls++
				if findCalls == 1 {
					// First call: ensureVMRunning checks existing VM — gone.
					return nil, nil
				}
				// Second call: createVMForRun re-queries after creation.
				return &provider.VMInfo{
					InstanceID:       "i-replacement",
					State:            "running",
					PublicIP:         "10.0.0.1",
					Region:           "us-east-1",
					AvailabilityZone: "us-east-1a",
				}, nil
			},
			createVMFn: func(ctx context.Context, opts provider.CreateVMOpts) (provider.VMInfo, error) {
				createCalled = true
				return provider.VMInfo{
					InstanceID: "i-replacement",
					State:      "pending",
					Region:     "us-east-1",
				}, nil
			},
		}
		cc, stdout, _ := testCmdContext(t, prov)
		saveTestVMState(t, cc.State, cc.Project.Hash)

		err := RunUp(context.Background(), cc)
		require.NoError(t, err)
		assert.True(t, createCalled)
		assert.Contains(t, stdout.String(), "no longer exists")
		assert.Contains(t, stdout.String(), "creating a new one")
	})

	t.Run("waits for pending VM", func(t *testing.T) {
		t.Parallel()
		waitCalled := false
		prov := &mockProvider{
			findVMFn: func(ctx context.Context, projectHash string) (*provider.VMInfo, error) {
				return &provider.VMInfo{
					InstanceID: "i-pending",
					State:      "pending",
					Region:     "us-east-1",
				}, nil
			},
			waitUntilRunningFn: func(ctx context.Context, instanceID string) error {
				assert.Equal(t, "i-pending", instanceID)
				waitCalled = true
				return nil
			},
		}
		cc, stdout, _ := testCmdContext(t, prov)
		saveTestVMState(t, cc.State, cc.Project.Hash)

		err := RunUp(context.Background(), cc)
		require.NoError(t, err)
		assert.True(t, waitCalled)
		assert.Contains(t, stdout.String(), "VM running")
	})

	t.Run("detects setup change on running VM", func(t *testing.T) {
		t.Parallel()
		prov := &mockProvider{
			findVMFn: func(ctx context.Context, projectHash string) (*provider.VMInfo, error) {
				return &provider.VMInfo{
					InstanceID: "i-running",
					State:      "running",
					Region:     "us-east-1",
				}, nil
			},
		}
		cc, stdout, _ := testCmdContext(t, prov)
		// Save state with a different setup hash to simulate config change.
		err := cc.State.SaveVM(cc.Project.Hash, state.VMState{
			InstanceID: "i-running",
			Region:     "us-east-1",
			Created:    time.Now().UTC(),
			ProjectDir: "/home/user/myproject",
			SetupHash:  "oldhash12345678",
		})
		require.NoError(t, err)

		err = RunUp(context.Background(), cc)
		require.NoError(t, err)
		assert.Contains(t, stdout.String(), "setup changed")
		assert.Contains(t, stdout.String(), "yg destroy")
	})

	t.Run("propagates create error", func(t *testing.T) {
		t.Parallel()
		prov := &mockProvider{
			createVMFn: func(ctx context.Context, opts provider.CreateVMOpts) (provider.VMInfo, error) {
				return provider.VMInfo{}, fmt.Errorf("insufficient capacity")
			},
		}
		cc, _, _ := testCmdContext(t, prov)

		err := RunUp(context.Background(), cc)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "insufficient capacity")
	})

	t.Run("propagates EnsureSecurityGroup error", func(t *testing.T) {
		t.Parallel()
		prov := &mockProvider{
			ensureSecurityGroupFn: func(ctx context.Context) (string, error) {
				return "", fmt.Errorf("sg creation failed")
			},
		}
		cc, _, _ := testCmdContext(t, prov)

		err := RunUp(context.Background(), cc)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "sg creation failed")
	})

	t.Run("propagates EnsureBucket error", func(t *testing.T) {
		t.Parallel()
		prov := &mockProvider{
			ensureBucketFn: func(ctx context.Context) error {
				return fmt.Errorf("bucket creation failed")
			},
		}
		cc, _, _ := testCmdContext(t, prov)

		err := RunUp(context.Background(), cc)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "bucket creation failed")
	})

	t.Run("propagates FindVM error", func(t *testing.T) {
		t.Parallel()
		prov := &mockProvider{
			findVMFn: func(ctx context.Context, projectHash string) (*provider.VMInfo, error) {
				return nil, fmt.Errorf("describe instances failed")
			},
		}
		cc, _, _ := testCmdContext(t, prov)
		saveTestVMState(t, cc.State, cc.Project.Hash)

		err := RunUp(context.Background(), cc)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "querying VM state")
	})

	t.Run("propagates WaitUntilRunning error on new VM", func(t *testing.T) {
		t.Parallel()
		prov := &mockProvider{
			createVMFn: func(ctx context.Context, opts provider.CreateVMOpts) (provider.VMInfo, error) {
				return provider.VMInfo{
					InstanceID: "i-wait-err",
					State:      "pending",
					Region:     "us-east-1",
				}, nil
			},
			waitUntilRunningFn: func(ctx context.Context, instanceID string) error {
				return fmt.Errorf("exceeded max wait time")
			},
		}
		cc, _, _ := testCmdContext(t, prov)

		err := RunUp(context.Background(), cc)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "exceeded max wait time")
	})

	t.Run("propagates StartVM error on stopped VM", func(t *testing.T) {
		t.Parallel()
		prov := &mockProvider{
			findVMFn: func(ctx context.Context, projectHash string) (*provider.VMInfo, error) {
				return &provider.VMInfo{
					InstanceID: "i-starterr",
					State:      "stopped",
					Region:     "us-east-1",
				}, nil
			},
			startVMFn: func(ctx context.Context, instanceID string) error {
				return fmt.Errorf("cannot start instance")
			},
		}
		cc, _, _ := testCmdContext(t, prov)
		saveTestVMState(t, cc.State, cc.Project.Hash)

		err := RunUp(context.Background(), cc)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot start instance")
	})

	t.Run("propagates WaitUntilRunning error on started VM", func(t *testing.T) {
		t.Parallel()
		prov := &mockProvider{
			findVMFn: func(ctx context.Context, projectHash string) (*provider.VMInfo, error) {
				return &provider.VMInfo{
					InstanceID: "i-waiterr2",
					State:      "stopped",
					Region:     "us-east-1",
				}, nil
			},
			startVMFn: func(ctx context.Context, instanceID string) error {
				return nil
			},
			waitUntilRunningFn: func(ctx context.Context, instanceID string) error {
				return fmt.Errorf("wait timed out")
			},
		}
		cc, _, _ := testCmdContext(t, prov)
		saveTestVMState(t, cc.State, cc.Project.Hash)

		err := RunUp(context.Background(), cc)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "wait timed out")
	})

	t.Run("propagates WaitUntilRunning error on pending VM", func(t *testing.T) {
		t.Parallel()
		prov := &mockProvider{
			findVMFn: func(ctx context.Context, projectHash string) (*provider.VMInfo, error) {
				return &provider.VMInfo{
					InstanceID: "i-penderr",
					State:      "pending",
					Region:     "us-east-1",
				}, nil
			},
			waitUntilRunningFn: func(ctx context.Context, instanceID string) error {
				return fmt.Errorf("pending wait failed")
			},
		}
		cc, _, _ := testCmdContext(t, prov)
		saveTestVMState(t, cc.State, cc.Project.Hash)

		err := RunUp(context.Background(), cc)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "pending wait failed")
	})

	t.Run("creates new VM when existing VM in terminal state", func(t *testing.T) {
		t.Parallel()
		createCalled := false
		findCalls := 0
		prov := &mockProvider{
			findVMFn: func(ctx context.Context, projectHash string) (*provider.VMInfo, error) {
				findCalls++
				if findCalls == 1 {
					// First call: ensureVMRunning sees terminal state.
					return &provider.VMInfo{
						InstanceID: "i-shutting",
						State:      "shutting-down",
						Region:     "us-east-1",
					}, nil
				}
				// Second call: createVMForRun re-queries after creation.
				return &provider.VMInfo{
					InstanceID:       "i-new999",
					State:            "running",
					PublicIP:         "10.0.0.2",
					Region:           "us-east-1",
					AvailabilityZone: "us-east-1a",
				}, nil
			},
			createVMFn: func(ctx context.Context, opts provider.CreateVMOpts) (provider.VMInfo, error) {
				createCalled = true
				return provider.VMInfo{
					InstanceID: "i-new999",
					State:      "pending",
					Region:     "us-east-1",
				}, nil
			},
		}
		cc, stdout, _ := testCmdContext(t, prov)
		saveTestVMState(t, cc.State, cc.Project.Hash)

		err := RunUp(context.Background(), cc)
		require.NoError(t, err)
		assert.True(t, createCalled)
		assert.Contains(t, stdout.String(), "creating a new")
	})
}

// --- Mock S3 for logs/kill tests ---

type testS3 struct {
	getObjectFn func(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error)
	putObjectFn func(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error)
}

func (m *testS3) PutObject(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
	if m.putObjectFn != nil {
		return m.putObjectFn(ctx, params, optFns...)
	}
	return &s3.PutObjectOutput{}, nil
}

func (m *testS3) GetObject(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	if m.getObjectFn != nil {
		return m.getObjectFn(ctx, params, optFns...)
	}
	return &s3.GetObjectOutput{
		Body: io.NopCloser(bytes.NewReader([]byte("{}"))),
	}, nil
}

// mockStorageFactory returns a StorageFactory using the given mock S3.
func mockStorageFactory(s3api fkstorage.S3API) StorageFactory {
	return func(ctx context.Context) (*fkstorage.Store, error) {
		return fkstorage.NewStore(s3api, "test-bucket"), nil
	}
}

// --- Logs tests ---

func TestRunLogs(t *testing.T) {
	t.Parallel()

	t.Run("no previous runs shows helpful message", func(t *testing.T) {
		t.Parallel()
		cc, stdout, _ := testCmdContext(t, &mockProvider{})

		err := RunLogs(context.Background(), cc, "", 0)
		require.NoError(t, err)
		assert.Contains(t, stdout.String(), "no previous runs")
		assert.Contains(t, stdout.String(), "yg <command>")
	})

	t.Run("with explicit run ID downloads from S3", func(t *testing.T) {
		t.Parallel()
		s3mock := &testS3{
			getObjectFn: func(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
				key := *params.Key
				if key == "myproject/run123/stdout.log" {
					return &s3.GetObjectOutput{
						Body: io.NopCloser(bytes.NewReader([]byte("test output line 1\ntest output line 2\n"))),
					}, nil
				}
				if key == "myproject/run123/meta.json" {
					meta := `{"run_id":"run123","command":"cargo test","exit_code":0,"duration":"5s"}`
					return &s3.GetObjectOutput{
						Body: io.NopCloser(bytes.NewReader([]byte(meta))),
					}, nil
				}
				return &s3.GetObjectOutput{
					Body: io.NopCloser(bytes.NewReader([]byte("{}"))),
				}, nil
			},
		}
		cc, stdout, _ := testCmdContext(t, &mockProvider{})
		cc.NewStorage = mockStorageFactory(s3mock)

		err := RunLogs(context.Background(), cc, "run123", 0)
		require.NoError(t, err)
		out := stdout.String()
		assert.Contains(t, out, "replaying run run123")
		assert.Contains(t, out, "test output line 1")
		assert.Contains(t, out, "test output line 2")
		assert.Contains(t, out, "cargo test")
		assert.Contains(t, out, "exit 0")
	})

	t.Run("uses last run ID when none specified", func(t *testing.T) {
		t.Parallel()
		s3mock := &testS3{
			getObjectFn: func(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
				return &s3.GetObjectOutput{
					Body: io.NopCloser(bytes.NewReader([]byte("last run output"))),
				}, nil
			},
		}
		cc, stdout, _ := testCmdContext(t, &mockProvider{})
		cc.NewStorage = mockStorageFactory(s3mock)

		// Save a last run.
		err := cc.State.SaveLastRun(cc.Project.Hash, "lastrun99")
		require.NoError(t, err)

		err = RunLogs(context.Background(), cc, "", 0)
		require.NoError(t, err)
		assert.Contains(t, stdout.String(), "replaying run lastrun99")
	})

	t.Run("tail option limits output lines", func(t *testing.T) {
		t.Parallel()
		s3mock := &testS3{
			getObjectFn: func(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
				key := *params.Key
				if key == "myproject/tail-run/stdout.log" {
					return &s3.GetObjectOutput{
						Body: io.NopCloser(bytes.NewReader([]byte("line1\nline2\nline3\nline4\nline5"))),
					}, nil
				}
				return &s3.GetObjectOutput{
					Body: io.NopCloser(bytes.NewReader([]byte("{}"))),
				}, nil
			},
		}
		cc, stdout, _ := testCmdContext(t, &mockProvider{})
		cc.NewStorage = mockStorageFactory(s3mock)

		err := RunLogs(context.Background(), cc, "tail-run", 2)
		require.NoError(t, err)
		out := stdout.String()
		assert.Contains(t, out, "line4")
		assert.Contains(t, out, "line5")
		assert.NotContains(t, out, "line1")
		assert.NotContains(t, out, "line2")
		assert.NotContains(t, out, "line3")
	})

	t.Run("tail handles trailing newlines correctly", func(t *testing.T) {
		t.Parallel()
		s3mock := &testS3{
			getObjectFn: func(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
				key := *params.Key
				if key == "myproject/tail-nl/stdout.log" {
					// Output with trailing newline — should not count as an extra line.
					return &s3.GetObjectOutput{
						Body: io.NopCloser(bytes.NewReader([]byte("line1\nline2\nline3\nline4\nline5\n"))),
					}, nil
				}
				return &s3.GetObjectOutput{
					Body: io.NopCloser(bytes.NewReader([]byte("{}"))),
				}, nil
			},
		}
		cc, stdout, _ := testCmdContext(t, &mockProvider{})
		cc.NewStorage = mockStorageFactory(s3mock)

		err := RunLogs(context.Background(), cc, "tail-nl", 2)
		require.NoError(t, err)
		out := stdout.String()
		assert.Contains(t, out, "line4")
		assert.Contains(t, out, "line5")
		assert.NotContains(t, out, "line3")
	})

	t.Run("storage connection error propagates", func(t *testing.T) {
		t.Parallel()
		cc, _, _ := testCmdContext(t, &mockProvider{})
		cc.NewStorage = func(ctx context.Context) (*fkstorage.Store, error) {
			return nil, fmt.Errorf("no credentials")
		}

		err := RunLogs(context.Background(), cc, "some-run", 0)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "connecting to storage")
	})

	t.Run("download error propagates", func(t *testing.T) {
		t.Parallel()
		s3mock := &testS3{
			getObjectFn: func(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
				return nil, fmt.Errorf("NoSuchKey")
			},
		}
		cc, _, _ := testCmdContext(t, &mockProvider{})
		cc.NewStorage = mockStorageFactory(s3mock)

		err := RunLogs(context.Background(), cc, "missing-run", 0)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "downloading output")
	})

	t.Run("live streams from active tmux session", func(t *testing.T) {
		t.Parallel()
		prov := &mockProvider{
			findVMFn: func(ctx context.Context, projectHash string) (*provider.VMInfo, error) {
				return &provider.VMInfo{
					InstanceID:       "i-live001",
					State:            "running",
					PublicIP:         "10.0.0.1",
					Region:           "us-east-1",
					AvailabilityZone: "us-east-1a",
				}, nil
			},
		}
		cc, stdout, _ := testCmdContext(t, prov)
		saveTestVMState(t, cc.State, cc.Project.Hash)

		cc.ConnectSSH = func(ctx context.Context, vmInfo *provider.VMInfo) (*gossh.Client, error) {
			return nil, nil
		}
		cc.IsRunActive = func(client *gossh.Client, runID fkexec.RunID) (bool, error) {
			assert.Equal(t, fkexec.RunID("liverun1"), runID)
			return true, nil
		}
		cc.TailLog = func(client *gossh.Client, runID fkexec.RunID, stdout io.Writer) error {
			assert.Equal(t, fkexec.RunID("liverun1"), runID)
			stdout.Write([]byte("live output line 1\nlive output line 2\n"))
			return nil
		}

		err := RunLogs(context.Background(), cc, "liverun1", 0)
		require.NoError(t, err)
		out := stdout.String()
		assert.Contains(t, out, "streaming run liverun1 (live)")
		assert.Contains(t, out, "live output line 1")
		assert.Contains(t, out, "live output line 2")
		assert.Contains(t, out, "stream ended")
		assert.NotContains(t, out, "replaying") // should NOT fall back to S3
	})

	t.Run("falls back to S3 when run is not active", func(t *testing.T) {
		t.Parallel()
		prov := &mockProvider{
			findVMFn: func(ctx context.Context, projectHash string) (*provider.VMInfo, error) {
				return &provider.VMInfo{
					InstanceID:       "i-done001",
					State:            "running",
					PublicIP:         "10.0.0.1",
					Region:           "us-east-1",
					AvailabilityZone: "us-east-1a",
				}, nil
			},
		}
		s3mock := &testS3{
			getObjectFn: func(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
				return &s3.GetObjectOutput{
					Body: io.NopCloser(bytes.NewReader([]byte("s3 output"))),
				}, nil
			},
		}
		cc, stdout, _ := testCmdContext(t, prov)
		saveTestVMState(t, cc.State, cc.Project.Hash)
		cc.NewStorage = mockStorageFactory(s3mock)

		cc.ConnectSSH = func(ctx context.Context, vmInfo *provider.VMInfo) (*gossh.Client, error) {
			return nil, nil
		}
		cc.IsRunActive = func(client *gossh.Client, runID fkexec.RunID) (bool, error) {
			return false, nil // run completed
		}

		err := RunLogs(context.Background(), cc, "donerun1", 0)
		require.NoError(t, err)
		out := stdout.String()
		assert.Contains(t, out, "replaying run donerun1")
		assert.Contains(t, out, "s3 output")
		assert.NotContains(t, out, "streaming") // should NOT show live stream
	})

	t.Run("falls back to S3 when SSH fails", func(t *testing.T) {
		t.Parallel()
		prov := &mockProvider{
			findVMFn: func(ctx context.Context, projectHash string) (*provider.VMInfo, error) {
				return &provider.VMInfo{
					InstanceID:       "i-sshfail",
					State:            "running",
					PublicIP:         "10.0.0.1",
					Region:           "us-east-1",
					AvailabilityZone: "us-east-1a",
				}, nil
			},
		}
		s3mock := &testS3{
			getObjectFn: func(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
				return &s3.GetObjectOutput{
					Body: io.NopCloser(bytes.NewReader([]byte("s3 fallback"))),
				}, nil
			},
		}
		cc, stdout, _ := testCmdContext(t, prov)
		saveTestVMState(t, cc.State, cc.Project.Hash)
		cc.NewStorage = mockStorageFactory(s3mock)

		cc.ConnectSSH = func(ctx context.Context, vmInfo *provider.VMInfo) (*gossh.Client, error) {
			return nil, fmt.Errorf("SSH connection failed")
		}

		err := RunLogs(context.Background(), cc, "sshfail1", 0)
		require.NoError(t, err)
		out := stdout.String()
		assert.Contains(t, out, "replaying run sshfail1")
		assert.Contains(t, out, "s3 fallback")
	})

	t.Run("falls back to S3 when VM is stopped", func(t *testing.T) {
		t.Parallel()
		prov := &mockProvider{
			findVMFn: func(ctx context.Context, projectHash string) (*provider.VMInfo, error) {
				return &provider.VMInfo{
					InstanceID: "i-stopped",
					State:      "stopped",
					Region:     "us-east-1",
				}, nil
			},
		}
		s3mock := &testS3{
			getObjectFn: func(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
				return &s3.GetObjectOutput{
					Body: io.NopCloser(bytes.NewReader([]byte("stopped vm output"))),
				}, nil
			},
		}
		cc, stdout, _ := testCmdContext(t, prov)
		saveTestVMState(t, cc.State, cc.Project.Hash)
		cc.NewStorage = mockStorageFactory(s3mock)

		err := RunLogs(context.Background(), cc, "stop1234", 0)
		require.NoError(t, err)
		out := stdout.String()
		assert.Contains(t, out, "replaying run stop1234")
		assert.Contains(t, out, "stopped vm output")
	})

	t.Run("falls back to S3 when no VM state exists", func(t *testing.T) {
		t.Parallel()
		s3mock := &testS3{
			getObjectFn: func(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
				return &s3.GetObjectOutput{
					Body: io.NopCloser(bytes.NewReader([]byte("no-vm output"))),
				}, nil
			},
		}
		cc, stdout, _ := testCmdContext(t, &mockProvider{})
		cc.NewStorage = mockStorageFactory(s3mock)

		err := RunLogs(context.Background(), cc, "novm1234", 0)
		require.NoError(t, err)
		out := stdout.String()
		assert.Contains(t, out, "replaying run novm1234")
		assert.Contains(t, out, "no-vm output")
	})
}

// --- Kill tests ---

func TestRunKill(t *testing.T) {
	t.Parallel()

	t.Run("no previous runs shows message", func(t *testing.T) {
		t.Parallel()
		cc, stdout, _ := testCmdContext(t, &mockProvider{})

		err := RunKill(context.Background(), cc, "")
		require.NoError(t, err)
		assert.Contains(t, stdout.String(), "no active commands")
	})

	t.Run("no VM state shows message", func(t *testing.T) {
		t.Parallel()
		cc, stdout, _ := testCmdContext(t, &mockProvider{})

		err := RunKill(context.Background(), cc, "aa112233")
		require.NoError(t, err)
		assert.Contains(t, stdout.String(), "no VM found")
	})

	t.Run("VM no longer exists shows message", func(t *testing.T) {
		t.Parallel()
		prov := &mockProvider{
			findVMFn: func(ctx context.Context, projectHash string) (*provider.VMInfo, error) {
				return nil, nil
			},
		}
		cc, stdout, _ := testCmdContext(t, prov)
		saveTestVMState(t, cc.State, cc.Project.Hash)

		err := RunKill(context.Background(), cc, "aa112233")
		require.NoError(t, err)
		assert.Contains(t, stdout.String(), "no longer exists")
	})

	t.Run("VM not running shows message", func(t *testing.T) {
		t.Parallel()
		prov := &mockProvider{
			findVMFn: func(ctx context.Context, projectHash string) (*provider.VMInfo, error) {
				return &provider.VMInfo{
					InstanceID: "i-stopped",
					State:      "stopped",
					Region:     "us-east-1",
				}, nil
			},
		}
		cc, stdout, _ := testCmdContext(t, prov)
		saveTestVMState(t, cc.State, cc.Project.Hash)

		err := RunKill(context.Background(), cc, "aa112233")
		require.NoError(t, err)
		assert.Contains(t, stdout.String(), "cannot kill")
	})

	t.Run("FindVM error propagates", func(t *testing.T) {
		t.Parallel()
		prov := &mockProvider{
			findVMFn: func(ctx context.Context, projectHash string) (*provider.VMInfo, error) {
				return nil, fmt.Errorf("network timeout")
			},
		}
		cc, _, _ := testCmdContext(t, prov)
		saveTestVMState(t, cc.State, cc.Project.Hash)

		err := RunKill(context.Background(), cc, "aa112233")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "querying VM state")
	})

	t.Run("SSH connection error propagates", func(t *testing.T) {
		t.Parallel()
		prov := &mockProvider{
			findVMFn: func(ctx context.Context, projectHash string) (*provider.VMInfo, error) {
				return &provider.VMInfo{
					InstanceID:       "i-kill001",
					State:            "running",
					PublicIP:         "10.0.0.1",
					Region:           "us-east-1",
					AvailabilityZone: "us-east-1a",
				}, nil
			},
		}
		cc, stdout, _ := testCmdContext(t, prov)
		saveTestVMState(t, cc.State, cc.Project.Hash)

		cc.ConnectSSH = func(ctx context.Context, vmInfo *provider.VMInfo) (*gossh.Client, error) {
			return nil, fmt.Errorf("test: SSH not available")
		}

		err := RunKill(context.Background(), cc, "a1b2c3d4")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "SSH not available")
		assert.Contains(t, stdout.String(), "cancelling run a1b2c3d4")
	})

	t.Run("resolves last run when no run ID given", func(t *testing.T) {
		t.Parallel()
		prov := &mockProvider{
			findVMFn: func(ctx context.Context, projectHash string) (*provider.VMInfo, error) {
				return &provider.VMInfo{
					InstanceID:       "i-kill002",
					State:            "running",
					PublicIP:         "10.0.0.2",
					Region:           "us-east-1",
					AvailabilityZone: "us-east-1a",
				}, nil
			},
		}
		cc, stdout, _ := testCmdContext(t, prov)
		saveTestVMState(t, cc.State, cc.Project.Hash)

		// Save a last run ID.
		err := cc.State.SaveLastRun(cc.Project.Hash, "autorun55")
		require.NoError(t, err)

		cc.ConnectSSH = func(ctx context.Context, vmInfo *provider.VMInfo) (*gossh.Client, error) {
			return nil, fmt.Errorf("test: SSH not available")
		}

		err = RunKill(context.Background(), cc, "")
		require.Error(t, err)
		// Verify it resolved the last run ID.
		assert.Contains(t, stdout.String(), "cancelling run autorun55")
	})

	t.Run("corrupt VM state error propagates", func(t *testing.T) {
		t.Parallel()
		cc, _, _ := testCmdContext(t, &mockProvider{})
		// Save then corrupt state.
		err := cc.State.SaveVM(cc.Project.Hash, state.VMState{
			InstanceID: "i-test", Region: "us-east-1", Created: time.Now(),
		})
		require.NoError(t, err)
		corruptFile := fmt.Sprintf("%s/projects/%s/vm.json", cc.State.BaseDir(), cc.Project.Hash)
		require.NoError(t, os.WriteFile(corruptFile, []byte("not json{{{"), 0o644))

		err = RunKill(context.Background(), cc, "some-run")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "loading VM state")
	})
}
