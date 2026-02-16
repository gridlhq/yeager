//go:build e2e

package e2e

import (
	"strings"
	"testing"
	"time"
)

// TestE2E_ResumeStoppedVM verifies the stopped VM resume flow:
// 1. Run a command (creates VM)
// 2. Stop the VM
// 3. Run another command (auto-starts the stopped VM)
// 4. Verify the command succeeds without re-provisioning.
func TestE2E_ResumeStoppedVM(t *testing.T) {
	dir := uniqueDir(t)
	setupGoProject(t, dir)
	t.Cleanup(func() { destroyProject(t, dir) })

	// Step 1: Cold start — creates VM.
	out := runFKSuccess(t, dir, 10*time.Minute, "go", "test", "./...")
	requireContainsAll(t, out, "PASS")

	// Step 2: Stop the VM.
	stopOut := runFKSuccess(t, dir, 2*time.Minute, "stop")
	requireContainsAll(t, stopOut, "VM stopped")

	// Step 3: Verify status shows stopped.
	statusOut := waitForStatus(t, dir, "stopped", 2*time.Minute)
	requireContainsAll(t, statusOut, "stopped")

	// Step 4: Run again — should auto-start the stopped VM.
	out2 := runFKSuccess(t, dir, 5*time.Minute, "go", "test", "./...")
	requireContainsAll(t, out2,
		"starting stopped VM",
		"PASS",
	)

	// Should NOT say "creating" — the VM should be reused.
	if strings.Contains(out2, "creating one") || strings.Contains(out2, "creating a new") {
		t.Error("expected VM restart, not re-creation")
	}
}

// TestE2E_StatusCommand verifies `yg status` shows the expected information.
func TestE2E_StatusCommand(t *testing.T) {
	dir := uniqueDir(t)
	setupGoProject(t, dir)
	t.Cleanup(func() { destroyProject(t, dir) })

	// Before any command: no VM.
	out, _ := runFK(t, dir, 30*time.Second, "status")
	requireContainsAll(t, out, "no VM found")

	// Create VM.
	runFKSuccess(t, dir, 10*time.Minute, "go", "test", "./...")

	// After command: VM running.
	statusOut := runFKSuccess(t, dir, 30*time.Second, "status")
	requireContainsAll(t, statusOut, "running")

	// Should show recent run history.
	requireContainsAll(t, statusOut, "recent runs:", "go test")
}

// TestE2E_DestroyCommand verifies `yg destroy` terminates the VM and cleans up.
func TestE2E_DestroyCommand(t *testing.T) {
	dir := uniqueDir(t)
	setupGoProject(t, dir)
	// No t.Cleanup(destroyProject) — we're testing destroy itself.

	// Create VM.
	runFKSuccess(t, dir, 10*time.Minute, "go", "test", "./...")

	// Destroy it.
	destroyOut := runFKSuccess(t, dir, 2*time.Minute, "destroy")
	requireContainsAll(t, destroyOut, "destroyed", "cleaned up")

	// Status should show no VM.
	statusOut, _ := runFK(t, dir, 30*time.Second, "status")
	requireContainsAll(t, statusOut, "no VM found")
}

// TestE2E_UpCommand verifies `yg up` creates/starts a VM without running a command.
func TestE2E_UpCommand(t *testing.T) {
	dir := uniqueDir(t)
	setupGoProject(t, dir)
	t.Cleanup(func() { destroyProject(t, dir) })

	// yg up — creates VM.
	out := runFKSuccess(t, dir, 10*time.Minute, "up")
	requireContainsAll(t, out, "VM running")

	// Subsequent command should be faster (warm VM).
	out2 := runFKSuccess(t, dir, 3*time.Minute, "go", "test", "./...")
	requireContainsAll(t, out2,
		"VM running",
		"PASS",
	)
}
