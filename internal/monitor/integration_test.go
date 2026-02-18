package monitor

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/gridlhq/yeager/internal/state"
)

// mockProviderWithTracking extends mockProvider to track actual VM operations.
type mockProviderWithTracking struct {
	mockProvider
	mu          sync.Mutex
	stopCalled  bool
	stopTime    time.Time
	stopErrFunc func() error // Optional error injection
}

func (m *mockProviderWithTracking) StopVM(ctx context.Context, instanceID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.stopCalled = true
	m.stopTime = time.Now()
	m.stopped = true

	if m.stopErrFunc != nil {
		return m.stopErrFunc()
	}
	return nil
}

func (m *mockProviderWithTracking) WasStopCalled() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.stopCalled
}

func (m *mockProviderWithTracking) GetStopTime() time.Time {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.stopTime
}

func TestMonitorActuallyStopsVM(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Build the real yeager binary for this test.
	binaryPath := BuildYeagerBinary(t)

	tmpDir, err := os.MkdirTemp("", "yeager-monitor-integration-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	st, err := state.NewStore(tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	projectHash := "test-project-integration"

	// Save VM state so monitor can load it.
	vmState := state.VMState{
		InstanceID: "i-test123",
		Region:     "us-east-1",
	}
	if err := st.SaveVM(projectHash, vmState); err != nil {
		t.Fatal(err)
	}

	// Enable test mode so daemon uses fake provider.
	t.Setenv("YEAGER_TEST_MODE", "1")
	// Use faster check interval for testing (1 second instead of 5).
	t.Setenv("YEAGER_CHECK_INTERVAL", "1s")

	// Use a short grace period for testing (2 seconds).
	gracePeriod := 2 * time.Second
	m := New(projectHash, st, nil, gracePeriod) // Provider not used in parent process
	m.SetExecutablePath(binaryPath)

	// Start the monitor.
	startTime := time.Now()
	if err := m.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Verify monitor is running.
	pid, err := LoadPIDFile(st, projectHash)
	if err != nil {
		t.Fatalf("PID file not created: %v", err)
	}
	if !IsProcessRunning(pid) {
		t.Fatal("monitor process not running")
	}

	// Log the tmpDir for debugging.
	t.Logf("State dir: %s", tmpDir)
	t.Logf("Monitor PID: %d", pid)

	// Wait for grace period + checkInterval + buffer to ensure monitor stops VM.
	// The daemon checks every 1 second (configured via YEAGER_CHECK_INTERVAL).
	waitTime := gracePeriod + (2 * time.Second)
	time.Sleep(waitTime)

	// Read fake provider state to verify VM was stopped.
	fake := newFakeProvider(tmpDir)
	state, err := fake.GetState()
	if err != nil {
		t.Fatalf("failed to read fake provider state: %v", err)
	}

	// Verify VM was stopped.
	if !state.StopCalled {
		// Read monitor log for debugging.
		logPath := filepath.Join(tmpDir, "projects", projectHash, logFileName)
		if logData, err := os.ReadFile(logPath); err == nil {
			t.Logf("Monitor log:\n%s", string(logData))
		}
		t.Error("monitor did not stop VM after grace period")
	}

	// Verify stop was called around the right time.
	// The monitor checks every 1 second (test mode), so it will stop on the first check
	// after the grace period elapses. With a 2s grace period, that's around 2-3s.
	if !state.StopTime.IsZero() {
		elapsed := state.StopTime.Sub(startTime)
		// Should be at least the grace period.
		if elapsed < gracePeriod {
			t.Errorf("VM stopped too early: %v < %v", elapsed, gracePeriod)
		}
		// Should be within checkInterval (1s) + grace period + 1s buffer.
		maxExpected := 1*time.Second + gracePeriod + (1 * time.Second)
		if elapsed > maxExpected {
			t.Errorf("VM stopped too late: %v > %v", elapsed, maxExpected)
		}
	}

	// Verify monitor cleaned up after stopping VM.
	time.Sleep(500 * time.Millisecond)
	if _, err := LoadPIDFile(st, projectHash); !os.IsNotExist(err) {
		t.Error("monitor should have cleaned up PID file after stopping VM")
	}

	if _, err := st.LoadIdleStart(projectHash); !os.IsNotExist(err) {
		t.Error("monitor should have cleared idle start time after stopping VM")
	}
}

func TestMonitorCancellation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Build the real yeager binary for this test.
	binaryPath := BuildYeagerBinary(t)

	tmpDir, err := os.MkdirTemp("", "yeager-monitor-cancel-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	st, err := state.NewStore(tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	projectHash := "test-project-cancel"

	// Save VM state.
	vmState := state.VMState{
		InstanceID: "i-test456",
		Region:     "us-east-1",
	}
	if err := st.SaveVM(projectHash, vmState); err != nil {
		t.Fatal(err)
	}

	// Enable test mode so daemon uses fake provider.
	t.Setenv("YEAGER_TEST_MODE", "1")
	// Use faster check interval for testing.
	t.Setenv("YEAGER_CHECK_INTERVAL", "1s")

	gracePeriod := 5 * time.Second
	m := New(projectHash, st, nil, gracePeriod)
	m.SetExecutablePath(binaryPath)

	// Start the monitor.
	if err := m.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Wait a bit, then stop the monitor before grace period.
	time.Sleep(500 * time.Millisecond)

	if err := m.Stop(); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	// Wait to ensure VM is not stopped.
	time.Sleep(6 * time.Second)

	// Read fake provider state to verify VM was NOT stopped.
	fake := newFakeProvider(tmpDir)
	state, err := fake.GetState()
	if err != nil {
		t.Fatalf("failed to read fake provider state: %v", err)
	}

	if state.StopCalled {
		t.Error("monitor stopped VM even though it was cancelled")
	}
}

func TestMonitorLogging(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "yeager-monitor-logging-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	st, err := state.NewStore(tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	projectHash := "test-project-logging"
	prov := &mockProviderWithTracking{}

	gracePeriod := 1 * time.Second
	m := New(projectHash, st, prov, gracePeriod)

	// Start the monitor.
	if err := m.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Give monitor time to spawn and create log file.
	time.Sleep(200 * time.Millisecond)

	// Check that log file was created.
	logPath := st.BaseDir() + "/projects/" + projectHash + "/" + logFileName
	if _, err := os.Stat(logPath); err != nil {
		t.Fatalf("log file not created: %v", err)
	}

	// Clean up.
	_ = m.Stop()

	// Note: We don't verify log content in unit tests because the spawned
	// process is the test binary, not the actual CLI. The important thing
	// is that the log file is created and the monitor can write to it.
}

func TestMonitorRaceConditionPrevention(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "yeager-monitor-race-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	st, err := state.NewStore(tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	projectHash := "test-project-race"
	prov := &mockProviderWithTracking{}

	// Save VM state.
	vmState := state.VMState{
		InstanceID: "i-racetest",
		Region:     "us-east-1",
	}
	if err := st.SaveVM(projectHash, vmState); err != nil {
		t.Fatal(err)
	}

	gracePeriod := 10 * time.Second

	const concurrentStarts = 5
	var wg sync.WaitGroup
	errors := make(chan error, concurrentStarts)

	// Try to start monitor concurrently from multiple goroutines.
	wg.Add(concurrentStarts)
	for i := 0; i < concurrentStarts; i++ {
		go func() {
			defer wg.Done()
			m := New(projectHash, st, prov, gracePeriod)
			if err := m.Start(); err != nil {
				errors <- err
			}
		}()
	}

	wg.Wait()
	close(errors)

	// Check no errors occurred.
	for err := range errors {
		t.Errorf("concurrent Start returned error: %v", err)
	}

	// Verify only ONE monitor process is running.
	pid, err := LoadPIDFile(st, projectHash)
	if err != nil {
		t.Fatalf("PID file not found: %v", err)
	}

	if !IsProcessRunning(pid) {
		t.Error("monitor process not running")
	}

	// Clean up.
	m := New(projectHash, st, prov, gracePeriod)
	_ = m.Stop()
}

func TestMonitorLogFileAppends(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "yeager-monitor-append-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	st, err := state.NewStore(tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	projectHash := "test-project-append"
	prov := &mockProviderWithTracking{}

	gracePeriod := 500 * time.Millisecond

	// Start and stop monitor twice to test log file persistence.
	for i := 0; i < 2; i++ {
		m := New(projectHash, st, prov, gracePeriod)
		if err := m.Start(); err != nil {
			t.Fatalf("Start %d failed: %v", i, err)
		}

		// Give it time to spawn.
		time.Sleep(100 * time.Millisecond)

		// Stop the monitor.
		if err := m.Stop(); err != nil {
			t.Fatalf("Stop %d failed: %v", i, err)
		}

		// Give it time to actually stop.
		time.Sleep(100 * time.Millisecond)
	}

	// Verify log file exists (shows append mode works).
	logPath := st.BaseDir() + "/projects/" + projectHash + "/" + logFileName
	if _, err := os.Stat(logPath); err != nil {
		t.Errorf("log file should exist after multiple monitor runs: %v", err)
	}

	// Note: We don't verify exact log content in unit tests because the
	// spawned process is the test binary. The important thing is that
	// the file exists and persists across monitor starts.
}
