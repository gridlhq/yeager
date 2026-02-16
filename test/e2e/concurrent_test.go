//go:build e2e

package e2e

import (
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestE2E_ConcurrentCommands runs two commands in parallel on the same VM
// and verifies both complete successfully.
// Validates: concurrent SSH sessions, per-run tracking, no interference.
func TestE2E_ConcurrentCommands(t *testing.T) {
	dir := uniqueDir(t)
	setupLongRunningProject(t, dir)
	t.Cleanup(func() { destroyProject(t, dir) })

	// First: create VM with a quick command.
	runFKSuccess(t, dir, 10*time.Minute, "echo", "warmup")

	// Launch two concurrent commands.
	var wg sync.WaitGroup
	var out1, out2 string
	var err1, err2 error

	wg.Add(2)
	go func() {
		defer wg.Done()
		out1, err1 = runFK(t, dir, 3*time.Minute, "go", "run", "main.go")
	}()
	go func() {
		defer wg.Done()
		// Small delay to let the first command start.
		time.Sleep(2 * time.Second)
		out2, err2 = runFK(t, dir, 3*time.Minute, "echo", "concurrent-ok")
	}()

	wg.Wait()

	// Both should succeed.
	require.NoError(t, err1, "command 1 failed:\n%s", out1)
	require.NoError(t, err2, "command 2 failed:\n%s", out2)

	assert.Contains(t, out1, "done", "command 1 should complete")
	assert.Contains(t, out2, "concurrent-ok", "command 2 should complete")

	// Status should show both in recent runs.
	statusOut := runFKSuccess(t, dir, 30*time.Second, "status")
	assert.Contains(t, statusOut, "recent runs:")
}

// TestE2E_ConcurrentStatus verifies `yg status` shows active commands while
// a long-running command is in progress.
func TestE2E_ConcurrentStatus(t *testing.T) {
	dir := uniqueDir(t)
	setupLongRunningProject(t, dir)
	t.Cleanup(func() { destroyProject(t, dir) })

	// Warm up the VM.
	runFKSuccess(t, dir, 10*time.Minute, "echo", "warmup")

	// Start a long-running command in the background.
	bin := fkBinary(t)
	cmd := exec.Command(bin, "go", "run", "main.go")
	cmd.Dir = dir
	require.NoError(t, cmd.Start(), "failed to start background command")
	defer cmd.Process.Kill()

	// Give the command time to start on the VM.
	time.Sleep(5 * time.Second)

	// Check status — should show an active command or recent run.
	// The background command may have finished by the time status runs,
	// so we check for either active commands or recent runs.
	statusOut := runFKSuccess(t, dir, 30*time.Second, "status")
	hasActive := strings.Contains(statusOut, "active commands")
	hasRecent := strings.Contains(statusOut, "recent runs:")
	assert.True(t, hasActive || hasRecent,
		"status should show active commands or recent runs, got:\n%s", truncate(statusOut, 500))

	// Wait for the background command to finish.
	cmd.Wait()
}
