package cli

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	fkexec "github.com/gridlhq/yeager/internal/exec"
	"github.com/gridlhq/yeager/internal/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gossh "golang.org/x/crypto/ssh"
)

func TestNewIdleMonitor_ZeroTimeoutReturnsNil(t *testing.T) {
	t.Parallel()

	m := NewIdleMonitor(IdleMonitorOpts{
		IdleTimeout: 0,
	})
	assert.Nil(t, m)
}

func TestNewIdleMonitor_NegativeTimeoutReturnsNil(t *testing.T) {
	t.Parallel()

	m := NewIdleMonitor(IdleMonitorOpts{
		IdleTimeout: -1 * time.Minute,
	})
	assert.Nil(t, m)
}

func TestNewIdleMonitor_DefaultPollInterval(t *testing.T) {
	t.Parallel()

	m := NewIdleMonitor(IdleMonitorOpts{
		IdleTimeout: 5 * time.Minute,
		InstanceID:  "i-test",
	})
	require.NotNil(t, m)
	assert.Equal(t, defaultPollInterval, m.pollInterval)
}

func TestNewIdleMonitor_CustomPollInterval(t *testing.T) {
	t.Parallel()

	m := NewIdleMonitor(IdleMonitorOpts{
		IdleTimeout:  5 * time.Minute,
		PollInterval: 10 * time.Second,
		InstanceID:   "i-test",
	})
	require.NotNil(t, m)
	assert.Equal(t, 10*time.Second, m.pollInterval)
}

func TestIdleMonitor_StopsVMAfterIdleTimeout(t *testing.T) {
	t.Parallel()

	var stopCalled atomic.Bool

	prov := &mockProvider{
		stopVMFn: func(ctx context.Context, instanceID string) error {
			assert.Equal(t, "i-idle001", instanceID)
			stopCalled.Store(true)
			return nil
		},
	}

	m := NewIdleMonitor(IdleMonitorOpts{
		IdleTimeout:  100 * time.Millisecond,
		PollInterval: 10 * time.Millisecond,
		InstanceID:   "i-idle001",
		Provider:     prov,
		VMInfo: &provider.VMInfo{
			InstanceID:       "i-idle001",
			State:            "running",
			PublicIP:         "1.2.3.4",
			Region:           "us-east-1",
			AvailabilityZone: "us-east-1a",
		},
		ConnectSSH: func(ctx context.Context, vmInfo *provider.VMInfo) (*gossh.Client, error) {
			return nil, nil
		},
		ListRuns: func(client *gossh.Client) ([]fkexec.ActiveRun, error) {
			return nil, nil // no active runs
		},
	})
	require.NotNil(t, m)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	done := m.Start(ctx)

	// Wait for monitor to finish (VM stopped).
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("monitor did not stop VM within timeout")
	}

	assert.True(t, stopCalled.Load(), "StopVM should have been called after idle timeout")
}

func TestIdleMonitor_ResetsTimerWhenRunsActive(t *testing.T) {
	t.Parallel()

	var stopCalled atomic.Bool
	var pollCount atomic.Int32

	prov := &mockProvider{
		stopVMFn: func(ctx context.Context, instanceID string) error {
			stopCalled.Store(true)
			return nil
		},
	}

	m := NewIdleMonitor(IdleMonitorOpts{
		IdleTimeout:  50 * time.Millisecond,
		PollInterval: 10 * time.Millisecond,
		InstanceID:   "i-active001",
		Provider:     prov,
		VMInfo: &provider.VMInfo{
			InstanceID:       "i-active001",
			State:            "running",
			PublicIP:         "1.2.3.4",
			Region:           "us-east-1",
			AvailabilityZone: "us-east-1a",
		},
		ConnectSSH: func(ctx context.Context, vmInfo *provider.VMInfo) (*gossh.Client, error) {
			return nil, nil
		},
		ListRuns: func(client *gossh.Client) ([]fkexec.ActiveRun, error) {
			count := pollCount.Add(1)
			if count <= 5 {
				// Runs are active for the first 5 polls.
				return []fkexec.ActiveRun{{RunID: "aaa11111", Command: "test"}}, nil
			}
			// Then all runs finish.
			return nil, nil
		},
	})
	require.NotNil(t, m)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	done := m.Start(ctx)

	// Wait for monitor to finish (VM stopped after runs finish + idle timeout).
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("monitor did not stop VM within timeout")
	}

	assert.True(t, stopCalled.Load(), "StopVM should be called after runs finish and idle timeout elapses")

	// Verify we polled multiple times while runs were active.
	assert.GreaterOrEqual(t, pollCount.Load(), int32(5))
}

func TestIdleMonitor_CancellationStopsMonitor(t *testing.T) {
	t.Parallel()

	var stopCalled atomic.Bool

	prov := &mockProvider{
		stopVMFn: func(ctx context.Context, instanceID string) error {
			stopCalled.Store(true)
			return nil
		},
	}

	m := NewIdleMonitor(IdleMonitorOpts{
		IdleTimeout:  1 * time.Hour, // very long — should not trigger
		PollInterval: 10 * time.Millisecond,
		InstanceID:   "i-cancel001",
		Provider:     prov,
		VMInfo: &provider.VMInfo{
			InstanceID:       "i-cancel001",
			State:            "running",
			PublicIP:         "1.2.3.4",
			Region:           "us-east-1",
			AvailabilityZone: "us-east-1a",
		},
		ConnectSSH: func(ctx context.Context, vmInfo *provider.VMInfo) (*gossh.Client, error) {
			return nil, nil
		},
		ListRuns: func(client *gossh.Client) ([]fkexec.ActiveRun, error) {
			return nil, nil
		},
	})
	require.NotNil(t, m)

	ctx, cancel := context.WithCancel(context.Background())
	done := m.Start(ctx)

	// Cancel immediately.
	cancel()

	// Wait for the monitor goroutine to exit.
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("monitor did not exit after context cancellation")
	}

	// StopVM should NOT have been called — context was cancelled before idle timeout.
	assert.False(t, stopCalled.Load(), "StopVM should not be called when context is cancelled")
}

func TestIdleMonitor_SSHErrorResetsIdleTimer(t *testing.T) {
	t.Parallel()

	var pollCount atomic.Int32
	var stopCalled atomic.Bool

	prov := &mockProvider{
		stopVMFn: func(ctx context.Context, instanceID string) error {
			stopCalled.Store(true)
			return nil
		},
	}

	// SSH fails for the first 3 polls, then succeeds with no active runs.
	// The idle timeout should only start counting from the first successful
	// check — SSH errors must not accumulate as idle time.
	m := NewIdleMonitor(IdleMonitorOpts{
		IdleTimeout:  50 * time.Millisecond,
		PollInterval: 10 * time.Millisecond,
		InstanceID:   "i-ssherr001",
		Provider:     prov,
		VMInfo: &provider.VMInfo{
			InstanceID:       "i-ssherr001",
			State:            "running",
			PublicIP:         "1.2.3.4",
			Region:           "us-east-1",
			AvailabilityZone: "us-east-1a",
		},
		ConnectSSH: func(ctx context.Context, vmInfo *provider.VMInfo) (*gossh.Client, error) {
			count := pollCount.Add(1)
			if count <= 3 {
				return nil, assert.AnError // SSH fails first 3 times
			}
			return nil, nil
		},
		ListRuns: func(client *gossh.Client) ([]fkexec.ActiveRun, error) {
			return nil, nil // no active runs
		},
	})
	require.NotNil(t, m)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	done := m.Start(ctx)

	// Should eventually recover and stop the VM.
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("monitor did not stop VM within timeout")
	}

	assert.True(t, stopCalled.Load(), "StopVM should be called after recovery")

	// Verify it retried after SSH errors and needed additional idle polls.
	// 3 SSH errors + at least 5 successful idle polls (50ms / 10ms) = at least 8 total.
	assert.GreaterOrEqual(t, pollCount.Load(), int32(8),
		"SSH errors should reset idle timer, requiring additional polls after recovery")
}

func TestIdleMonitor_SSHErrorPreventsPrematureStop(t *testing.T) {
	t.Parallel()

	// Verify that continuous SSH errors don't cause the VM to be stopped.
	// The idle timer is reset on every error, so the timeout never elapses.
	var stopCalled atomic.Bool

	prov := &mockProvider{
		stopVMFn: func(ctx context.Context, instanceID string) error {
			stopCalled.Store(true)
			return nil
		},
	}

	m := NewIdleMonitor(IdleMonitorOpts{
		IdleTimeout:  30 * time.Millisecond,
		PollInterval: 10 * time.Millisecond,
		InstanceID:   "i-ssherr002",
		Provider:     prov,
		VMInfo: &provider.VMInfo{
			InstanceID:       "i-ssherr002",
			State:            "running",
			PublicIP:         "1.2.3.4",
			Region:           "us-east-1",
			AvailabilityZone: "us-east-1a",
		},
		ConnectSSH: func(ctx context.Context, vmInfo *provider.VMInfo) (*gossh.Client, error) {
			return nil, assert.AnError // SSH always fails
		},
		ListRuns: func(client *gossh.Client) ([]fkexec.ActiveRun, error) {
			t.Fatal("ListRuns should not be called when SSH fails")
			return nil, nil
		},
	})
	require.NotNil(t, m)

	// Run for 100ms — much longer than the 30ms idle timeout.
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	done := m.Start(ctx)
	<-done

	// StopVM should NOT have been called — SSH errors prevent idle detection.
	assert.False(t, stopCalled.Load(),
		"StopVM should not be called when SSH is failing — cannot confirm VM is idle")
}
