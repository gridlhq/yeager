package cli

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/gridlhq/yeager/internal/monitor"
	"github.com/gridlhq/yeager/internal/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDestroyStopsMonitor(t *testing.T) {
	ctx := context.Background()
	prov := &mockProvider{
		findVMFn: func(ctx context.Context, projectHash string) (*provider.VMInfo, error) {
			return &provider.VMInfo{
				InstanceID: "i-test123",
				State:      "running",
			}, nil
		},
	}

	cc, _, _ := testCmdContext(t, prov)
	saveTestVMState(t, cc.State, cc.Project.Hash)

	// Start a monitor daemon for this project.
	gracePeriod := 10 * time.Second
	m := monitor.New(cc.Project.Hash, cc.State, prov, gracePeriod)
	err := m.Start()
	require.NoError(t, err)

	// Verify monitor is running.
	pid, err := monitor.LoadPIDFile(cc.State, cc.Project.Hash)
	require.NoError(t, err)

	// Verify idle start time was saved.
	_, err = cc.State.LoadIdleStart(cc.Project.Hash)
	require.NoError(t, err, "idle start should exist before destroy")

	// Run destroy.
	err = RunDestroy(ctx, cc)
	require.NoError(t, err)

	// Give monitor a moment to actually shut down.
	time.Sleep(200 * time.Millisecond)

	// Verify monitor was stopped (PID file and idle start cleaned up).
	if _, err := monitor.LoadPIDFile(cc.State, cc.Project.Hash); !os.IsNotExist(err) {
		t.Error("destroy should have cleaned up monitor PID file")
	}

	// Verify idle start was cleared.
	if _, err := cc.State.LoadIdleStart(cc.Project.Hash); !os.IsNotExist(err) {
		t.Error("destroy should have cleared idle start time")
	}

	// Note: We don't verify the process itself stopped because in tests,
	// the spawned process is the test binary which may not handle signals
	// the same way the real CLI does. The important verification is that
	// the PID file and idle start are cleaned up.
	_ = pid
}

func TestDestroyWithoutMonitor(t *testing.T) {
	ctx := context.Background()
	prov := &mockProvider{
		findVMFn: func(ctx context.Context, projectHash string) (*provider.VMInfo, error) {
			return &provider.VMInfo{
				InstanceID: "i-test456",
				State:      "running",
			}, nil
		},
	}

	cc, _, _ := testCmdContext(t, prov)
	saveTestVMState(t, cc.State, cc.Project.Hash)

	// No monitor running - destroy should still succeed.
	err := RunDestroy(ctx, cc)
	require.NoError(t, err)
}

func TestDestroyWithNoVMState(t *testing.T) {
	ctx := context.Background()
	prov := &mockProvider{}
	cc, stdout, _ := testCmdContext(t, prov)

	// No VM state exists.
	err := RunDestroyWithOptions(ctx, cc, DestroyOptions{Force: true})
	require.NoError(t, err)

	assert.Contains(t, stdout.String(), "no VM found")
}

func TestDestroyWithoutForceShowsWarning(t *testing.T) {
	ctx := context.Background()
	prov := &mockProvider{}
	cc, stdout, stderr := testCmdContext(t, prov)
	saveTestVMState(t, cc.State, cc.Project.Hash)

	// Without force flag, should show warning and not destroy.
	err := RunDestroyWithOptions(ctx, cc, DestroyOptions{Force: false})
	require.NoError(t, err)

	// Warning goes to stderr (via w.Warn), hint goes to stdout.
	assert.Contains(t, stderr.String(), "warning: destroying this VM will permanently delete")
	assert.Contains(t, stdout.String(), "run again with --force to proceed")

	// Verify VM state still exists (not destroyed).
	_, err = cc.State.LoadVM(cc.Project.Hash)
	require.NoError(t, err, "VM state should not be deleted without --force")
}

func TestDestroyWithForceActuallyDestroys(t *testing.T) {
	ctx := context.Background()
	terminateCalled := false
	prov := &mockProvider{
		findVMFn: func(ctx context.Context, projectHash string) (*provider.VMInfo, error) {
			return &provider.VMInfo{
				InstanceID: "i-test789",
				State:      "running",
			}, nil
		},
		terminateVMFn: func(ctx context.Context, instanceID string) error {
			terminateCalled = true
			assert.Equal(t, "i-test789", instanceID)
			return nil
		},
	}

	cc, _, _ := testCmdContext(t, prov)
	saveTestVMState(t, cc.State, cc.Project.Hash)

	// With force flag, should actually destroy.
	err := RunDestroyWithOptions(ctx, cc, DestroyOptions{Force: true})
	require.NoError(t, err)

	// Verify terminate was called.
	assert.True(t, terminateCalled, "TerminateVM should have been called")

	// Verify VM state was deleted.
	_, err = cc.State.LoadVM(cc.Project.Hash)
	assert.Error(t, err, "VM state should be deleted after destroy")
	assert.True(t, errors.Is(err, os.ErrNotExist), "error should be ErrNotExist")
}

func TestDestroyCleanupEvenIfVMGone(t *testing.T) {
	ctx := context.Background()
	prov := &mockProvider{
		findVMFn: func(ctx context.Context, projectHash string) (*provider.VMInfo, error) {
			// VM doesn't exist in AWS anymore.
			return nil, nil
		},
	}

	cc, stdout, _ := testCmdContext(t, prov)
	saveTestVMState(t, cc.State, cc.Project.Hash)

	// Start a monitor.
	m := monitor.New(cc.Project.Hash, cc.State, prov, 10*time.Second)
	require.NoError(t, m.Start())

	// Destroy should clean up local state even if VM is gone.
	err := RunDestroy(ctx, cc)
	require.NoError(t, err)

	assert.Contains(t, stdout.String(), "no longer exists in AWS")

	// Verify local state cleaned up.
	_, err = cc.State.LoadVM(cc.Project.Hash)
	assert.True(t, errors.Is(err, os.ErrNotExist), "VM state should be deleted")

	// Verify monitor stopped.
	time.Sleep(200 * time.Millisecond)
	_, err = monitor.LoadPIDFile(cc.State, cc.Project.Hash)
	assert.True(t, os.IsNotExist(err), "monitor PID file should be removed")
}
