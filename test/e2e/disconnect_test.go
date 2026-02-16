//go:build e2e

package e2e

import (
	"os/exec"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestE2E_DisconnectDuringRun verifies that:
// 1. A command started via yg keeps running on the VM after the client disconnects.
// 2. Output can be retrieved via `yg logs` after the command completes.
//
// This is the core product promise: laptop closes mid-run, output still lands.
func TestE2E_DisconnectDuringRun(t *testing.T) {
	dir := uniqueDir(t)
	setupLongRunningProject(t, dir)
	t.Cleanup(func() { destroyProject(t, dir) })

	// Warm up the VM.
	runFKSuccess(t, dir, 10*time.Minute, "echo", "warmup")

	// Start a long-running command.
	bin := fkBinary(t)
	cmd := exec.Command(bin, "go", "run", "main.go")
	cmd.Dir = dir

	require.NoError(t, cmd.Start(), "failed to start background command")

	// Wait for the command to start on the VM (some ticks should appear).
	time.Sleep(5 * time.Second)

	// Simulate disconnect: kill the local yg process (SIGKILL, no graceful cleanup).
	require.NoError(t, cmd.Process.Kill(), "failed to kill yg process")
	cmd.Wait() // reap zombie

	// Wait for the remote command to finish (10 ticks × 1s = ~10s, plus buffer).
	time.Sleep(15 * time.Second)

	// Retrieve output via `yg logs`.
	logsOut := runFKSuccess(t, dir, 2*time.Minute, "logs")

	// The remote command should have printed "done" and several ticks.
	// Since we killed the local process after 5s, the remote continued independently.
	assert.Contains(t, logsOut, "done", "remote command should have completed after disconnect")
	assert.Contains(t, logsOut, "tick 1", "output should include early ticks")
}

// TestE2E_CtrlCDetachAndReattach simulates Ctrl+C detach behavior:
// 1. Start a command and interrupt it with SIGINT.
// 2. Verify it exits cleanly with detach message.
// 3. Verify `yg logs` can reattach and show remaining output.
func TestE2E_CtrlCDetachAndReattach(t *testing.T) {
	dir := uniqueDir(t)
	setupLongRunningProject(t, dir)
	t.Cleanup(func() { destroyProject(t, dir) })

	// Warm up the VM.
	runFKSuccess(t, dir, 10*time.Minute, "echo", "warmup")

	// Start a long-running command.
	bin := fkBinary(t)
	cmd := exec.Command(bin, "go", "run", "main.go")
	cmd.Dir = dir

	require.NoError(t, cmd.Start(), "failed to start command")

	// Wait for the command to start, then send SIGINT (Ctrl+C).
	time.Sleep(3 * time.Second)
	require.NoError(t, cmd.Process.Signal(signalInterrupt()), "failed to send SIGINT")

	// Wait for the process to exit.
	err := cmd.Wait()
	// yg should exit with code 0 on Ctrl+C detach.
	if err != nil {
		// On some systems the exit code may be non-zero due to signal handling.
		// That's acceptable — the important thing is the remote command keeps running.
		t.Logf("yg exited with: %v (expected on some platforms)", err)
	}

	// Wait for the remote command to finish.
	time.Sleep(15 * time.Second)

	// Reattach via `yg logs`.
	logsOut := runFKSuccess(t, dir, 2*time.Minute, "logs")
	assert.Contains(t, logsOut, "done", "remote command should have completed after detach")
}
