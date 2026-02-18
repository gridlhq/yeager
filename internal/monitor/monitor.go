package monitor

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/gridlhq/yeager/internal/provider"
	"github.com/gridlhq/yeager/internal/state"
)

const (
	// defaultCheckInterval is how often the monitor checks if grace period has elapsed.
	defaultCheckInterval = 5 * time.Second

	// pidFileName is the name of the PID file in the state directory.
	pidFileName = "monitor.pid"

	// logFileName is the name of the log file for the monitor daemon.
	logFileName = "monitor.log"
)

// getCheckInterval returns the check interval, respecting YEAGER_CHECK_INTERVAL env var.
// This allows tests to use faster intervals.
func getCheckInterval() time.Duration {
	if intervalStr := os.Getenv("YEAGER_CHECK_INTERVAL"); intervalStr != "" {
		if d, err := time.ParseDuration(intervalStr); err == nil && d > 0 {
			return d
		}
	}
	return defaultCheckInterval
}

// Monitor manages the background process that stops VMs after grace period.
type Monitor struct {
	projectHash    string
	state          *state.Store
	provider       provider.CloudProvider
	gracePeriod    time.Duration
	executablePath string // Optional: path to yeager binary (defaults to os.Args[0])
}

// New creates a new Monitor instance.
func New(projectHash string, st *state.Store, prov provider.CloudProvider, gracePeriod time.Duration) *Monitor {
	return &Monitor{
		projectHash: projectHash,
		state:       st,
		provider:    prov,
		gracePeriod: gracePeriod,
	}
}

// SetExecutablePath sets a custom path to the yeager binary.
// This is primarily used for testing to point to a built binary.
func (m *Monitor) SetExecutablePath(path string) {
	m.executablePath = path
}

// Start spawns a detached background monitor process.
// The monitor will periodically check if the grace period has elapsed and stop the VM.
// Returns immediately after spawning the background process.
func (m *Monitor) Start() error {
	// Acquire exclusive lock to prevent race conditions.
	lock, err := AcquireLock(m.state, m.projectHash)
	if err != nil {
		return fmt.Errorf("acquiring monitor lock: %w", err)
	}
	if lock == nil {
		// Another process is spawning/managing the monitor.
		slog.Debug("monitor lock held by another process, skipping spawn")
		return nil
	}
	defer lock.Release()

	// Check if a monitor is already running (under lock).
	if pid, err := LoadPIDFile(m.state, m.projectHash); err == nil {
		if IsProcessRunning(pid) {
			// Monitor already running, nothing to do.
			return nil
		}
		// Stale PID file, clean it up.
		_ = RemovePIDFile(m.state, m.projectHash)
	}

	// Record the idle start time in state.
	if err := m.state.SaveIdleStart(m.projectHash, time.Now().UTC()); err != nil {
		return fmt.Errorf("saving idle start time: %w", err)
	}

	// Create log file for monitor daemon.
	logPath := filepath.Join(m.state.BaseDir(), "projects", m.projectHash, logFileName)
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("creating log file: %w", err)
	}

	// Spawn detached monitor process.
	args := []string{
		"monitor-daemon",
		"--project-hash", m.projectHash,
		"--state-dir", m.state.BaseDir(),
		"--grace-period", m.gracePeriod.String(),
	}

	// Use custom executable path if set (for testing), otherwise use current binary.
	execPath := m.executablePath
	if execPath == "" {
		execPath = os.Args[0]
	}
	cmd := exec.Command(execPath, args...)

	// Inherit environment from parent (including test mode flags).
	cmd.Env = os.Environ()

	// Detach from parent process.
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}

	// Redirect stderr to log file for debugging.
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = logFile

	if err := cmd.Start(); err != nil {
		logFile.Close()
		return fmt.Errorf("spawning monitor daemon: %w", err)
	}

	// Close the log file in parent process - child has its own copy.
	// Must close after cmd.Start() so child inherits the open fd.
	logFile.Close()

	// Write PID file for coordination.
	if err := WritePIDFile(m.state, m.projectHash, cmd.Process.Pid); err != nil {
		// Best-effort: log but don't fail if we can't write PID file.
		slog.Warn("failed to write monitor PID file", "error", err)
	}

	// Detach from the child process.
	if err := cmd.Process.Release(); err != nil {
		slog.Warn("failed to release monitor process", "error", err)
	}

	slog.Debug("started monitor daemon", "pid", cmd.Process.Pid, "grace_period", m.gracePeriod, "log", logPath)
	return nil
}

// Stop terminates any running monitor for this project.
func (m *Monitor) Stop() error {
	pid, err := LoadPIDFile(m.state, m.projectHash)
	if err != nil {
		// No monitor running.
		return nil
	}

	if !IsProcessRunning(pid) {
		// Stale PID file, clean it up.
		_ = RemovePIDFile(m.state, m.projectHash)
		return nil
	}

	// Send SIGTERM to monitor process.
	process, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("finding monitor process: %w", err)
	}

	if err := process.Signal(syscall.SIGTERM); err != nil {
		// Process may have already exited, ignore error.
		slog.Debug("failed to signal monitor process", "pid", pid, "error", err)
	}

	// Clean up PID file.
	_ = RemovePIDFile(m.state, m.projectHash)

	// Clear idle start time.
	_ = m.state.ClearIdleStart(m.projectHash)

	slog.Debug("stopped monitor daemon", "pid", pid)
	return nil
}

// RunDaemon is the main loop for the background monitor process.
// This should only be called from the spawned child process.
func RunDaemon(projectHash, stateDir string, gracePeriod time.Duration) error {
	// Set up logging for daemon.
	logOpts := &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, logOpts))
	slog.SetDefault(logger)

	slog.Info("monitor daemon started", "project_hash", projectHash, "grace_period", gracePeriod)

	// Set up signal handling for graceful shutdown.
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGTERM, syscall.SIGINT)

	// Create state store.
	st, err := state.NewStore(stateDir)
	if err != nil {
		return fmt.Errorf("creating state store: %w", err)
	}

	// Load VM state to get instance ID and region.
	vmState, err := st.LoadVM(projectHash)
	if err != nil {
		slog.Error("failed to load VM state", "error", err)
		return fmt.Errorf("loading VM state: %w", err)
	}

	// Create provider for stopping VM.
	// Check if we're in test mode and should use a fake provider.
	ctx := context.Background()
	var prov provider.CloudProvider
	if os.Getenv("YEAGER_TEST_MODE") == "1" {
		slog.Warn("⚠️  RUNNING IN TEST MODE - DO NOT USE IN PRODUCTION ⚠️")
		slog.Debug("using fake provider for testing")
		prov = newFakeProvider(stateDir)
	} else {
		p, err := provider.NewAWSProvider(ctx, vmState.Region)
		if err != nil {
			return fmt.Errorf("creating provider: %w", err)
		}
		prov = p
	}

	checkInterval := getCheckInterval()
	slog.Debug("monitor check interval", "interval", checkInterval)
	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-sigChan:
			slog.Info("monitor daemon received signal, exiting")
			return nil

		case <-ticker.C:
			// Check if grace period has elapsed.
			shouldStop, err := checkShouldStop(st, projectHash, gracePeriod)
			if err != nil {
				slog.Error("check failed", "error", err)
				continue
			}

			slog.Debug("grace period check", "should_stop", shouldStop)

			if shouldStop {
				slog.Info("grace period elapsed, stopping VM", "instance_id", vmState.InstanceID)

				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()

				// Stop the VM.
				if err := prov.StopVM(ctx, vmState.InstanceID); err != nil {
					slog.Error("failed to stop VM", "error", err)
					// Don't exit on error, keep trying.
					continue
				}

				slog.Info("VM stopped successfully, monitor exiting")

				// Clean up PID file and idle start time.
				_ = RemovePIDFile(st, projectHash)
				_ = st.ClearIdleStart(projectHash)

				return nil
			}
		}
	}
}

// checkShouldStop determines if the VM should be stopped based on grace period.
func checkShouldStop(st *state.Store, projectHash string, gracePeriod time.Duration) (bool, error) {
	// Load idle start time.
	idleStart, err := st.LoadIdleStart(projectHash)
	if err != nil {
		// If no idle start time, the monitor was likely cancelled.
		slog.Debug("no idle start time found", "error", err)
		return false, nil
	}

	// Check if grace period has elapsed.
	elapsed := time.Since(idleStart)
	slog.Debug("checking grace period", "idle_start", idleStart, "elapsed", elapsed, "grace_period", gracePeriod)
	return elapsed >= gracePeriod, nil
}

// IsProcessRunning checks if a process with the given PID exists.
func IsProcessRunning(pid int) bool {
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}

	// On Unix, FindProcess always succeeds, so we need to send signal 0.
	err = process.Signal(syscall.Signal(0))
	return err == nil
}
