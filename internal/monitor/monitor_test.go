package monitor

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/gridlhq/yeager/internal/provider"
	"github.com/gridlhq/yeager/internal/state"
)

// mockProvider implements provider.CloudProvider for testing.
type mockProvider struct {
	stopped bool
}

func (m *mockProvider) AccountID(context.Context) (string, error)          { return "123456789012", nil }
func (m *mockProvider) Region() string                                     { return "us-east-1" }
func (m *mockProvider) EnsureSecurityGroup(context.Context) (string, error) { return "", nil }
func (m *mockProvider) EnsureBucket(context.Context) error                 { return nil }
func (m *mockProvider) CreateVM(context.Context, provider.CreateVMOpts) (provider.VMInfo, error) {
	return provider.VMInfo{}, nil
}
func (m *mockProvider) FindVM(context.Context, string) (*provider.VMInfo, error) { return nil, nil }
func (m *mockProvider) StartVM(context.Context, string) error                    { return nil }
func (m *mockProvider) StopVM(context.Context, string) error {
	m.stopped = true
	return nil
}
func (m *mockProvider) TerminateVM(context.Context, string) error { return nil }
func (m *mockProvider) WaitUntilRunning(context.Context, string) error {
	return nil
}
func (m *mockProvider) WaitUntilRunningWithProgress(context.Context, string, provider.ProgressCallback) error {
	return nil
}
func (m *mockProvider) BucketName(context.Context) (string, error) { return "", nil }

func TestMonitorStartStop(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "yeager-monitor-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	st, err := state.NewStore(tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	prov := &mockProvider{}
	projectHash := "test-project-123"
	gracePeriod := 5 * time.Second

	m := New(projectHash, st, prov, gracePeriod)

	t.Run("Start", func(t *testing.T) {
		if err := m.Start(); err != nil {
			t.Fatalf("Start failed: %v", err)
		}

		// Verify PID file exists.
		pid, err := LoadPIDFile(st, projectHash)
		if err != nil {
			t.Fatalf("PID file not created: %v", err)
		}

		// Verify process is running.
		if !IsProcessRunning(pid) {
			t.Error("monitor process not running")
		}

		// Verify idle start time was saved.
		idleStart, err := st.LoadIdleStart(projectHash)
		if err != nil {
			t.Fatalf("idle start time not saved: %v", err)
		}

		// Should be recent.
		if time.Since(idleStart) > 2*time.Second {
			t.Errorf("idle start time too old: %v", idleStart)
		}
	})

	t.Run("Stop", func(t *testing.T) {
		// Give monitor a moment to fully start.
		time.Sleep(100 * time.Millisecond)

		if err := m.Stop(); err != nil {
			t.Fatalf("Stop failed: %v", err)
		}

		// Verify PID file removed.
		_, err := LoadPIDFile(st, projectHash)
		if !os.IsNotExist(err) {
			t.Errorf("PID file should be removed, got error: %v", err)
		}

		// Verify idle start time cleared.
		_, err = st.LoadIdleStart(projectHash)
		if !os.IsNotExist(err) {
			t.Errorf("idle start time should be cleared, got error: %v", err)
		}
	})

	t.Run("StartIdempotent", func(t *testing.T) {
		// Start monitor.
		if err := m.Start(); err != nil {
			t.Fatalf("first Start failed: %v", err)
		}

		pid1, _ := LoadPIDFile(st, projectHash)

		// Starting again should be a no-op.
		if err := m.Start(); err != nil {
			t.Fatalf("second Start failed: %v", err)
		}

		pid2, _ := LoadPIDFile(st, projectHash)

		// Should be the same PID (no new monitor spawned).
		if pid1 != pid2 {
			t.Errorf("second Start spawned new process: %d != %d", pid1, pid2)
		}

		// Clean up.
		_ = m.Stop()
	})

	t.Run("StopIdempotent", func(t *testing.T) {
		// Stopping when no monitor is running should not error.
		if err := m.Stop(); err != nil {
			t.Errorf("Stop on non-running monitor errored: %v", err)
		}

		// Second stop should also not error.
		if err := m.Stop(); err != nil {
			t.Errorf("second Stop errored: %v", err)
		}
	})
}

func TestCheckShouldStop(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "yeager-monitor-check-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	st, err := state.NewStore(tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	projectHash := "test-project-123"

	t.Run("NoIdleStart", func(t *testing.T) {
		shouldStop, err := checkShouldStop(st, projectHash, 5*time.Second)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if shouldStop {
			t.Error("should not stop when no idle start time")
		}
	})

	t.Run("GracePeriodNotElapsed", func(t *testing.T) {
		// Set idle start to 1 second ago.
		idleStart := time.Now().UTC().Add(-1 * time.Second)
		if err := st.SaveIdleStart(projectHash, idleStart); err != nil {
			t.Fatal(err)
		}

		shouldStop, err := checkShouldStop(st, projectHash, 5*time.Second)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if shouldStop {
			t.Error("should not stop before grace period elapsed")
		}
	})

	t.Run("GracePeriodElapsed", func(t *testing.T) {
		// Set idle start to 10 seconds ago.
		idleStart := time.Now().UTC().Add(-10 * time.Second)
		if err := st.SaveIdleStart(projectHash, idleStart); err != nil {
			t.Fatal(err)
		}

		shouldStop, err := checkShouldStop(st, projectHash, 5*time.Second)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if !shouldStop {
			t.Error("should stop after grace period elapsed")
		}
	})

	t.Run("GracePeriodExactly", func(t *testing.T) {
		// Set idle start to exactly 5 seconds ago.
		idleStart := time.Now().UTC().Add(-5 * time.Second)
		if err := st.SaveIdleStart(projectHash, idleStart); err != nil {
			t.Fatal(err)
		}

		shouldStop, err := checkShouldStop(st, projectHash, 5*time.Second)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if !shouldStop {
			t.Error("should stop when grace period exactly elapsed")
		}
	})
}
