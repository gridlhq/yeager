//go:build e2e

package e2e

import (
	"testing"
	"time"
)

// TestE2E_RustProject runs `yg cargo test` on a minimal Rust project from cold start.
// Validates: VM creation, rustup provisioning, cargo fetch, rsync, execution, S3 upload.
func TestE2E_RustProject(t *testing.T) {
	dir := uniqueDir(t)
	setupRustProject(t, dir)
	t.Cleanup(func() { destroyProject(t, dir) })

	// Cold start: first run creates a new VM and provisions Rust.
	out := runFKSuccess(t, dir, 10*time.Minute, "cargo", "test")

	requireContainsAll(t, out,
		"project:",
		"syncing files",
		"running: cargo test",
		"test result: ok",
	)

	// Warm start: second run should be faster (VM already running, rust installed).
	out2 := runFKSuccess(t, dir, 3*time.Minute, "cargo", "test")
	requireContainsAll(t, out2,
		"VM running",
		"running: cargo test",
		"test result: ok",
	)

	// Verify logs work.
	logsOut := runFKSuccess(t, dir, 1*time.Minute, "logs")
	requireContainsAll(t, logsOut,
		"test result: ok",
	)
}

// TestE2E_NodeProject runs `yg npm test` on a minimal Node.js project.
// Validates: nvm provisioning, npm execution, output streaming.
func TestE2E_NodeProject(t *testing.T) {
	dir := uniqueDir(t)
	setupNodeProject(t, dir)
	t.Cleanup(func() { destroyProject(t, dir) })

	out := runFKSuccess(t, dir, 10*time.Minute, "npm", "test")

	requireContainsAll(t, out,
		"project:",
		"syncing files",
		"running: npm test",
		"all tests passed",
	)
}

// TestE2E_PythonProject runs `yg python test_main.py` on a minimal Python project.
// Validates: pyenv provisioning, python execution.
func TestE2E_PythonProject(t *testing.T) {
	dir := uniqueDir(t)
	setupPythonProject(t, dir)
	t.Cleanup(func() { destroyProject(t, dir) })

	out := runFKSuccess(t, dir, 10*time.Minute, "python", "test_main.py")

	requireContainsAll(t, out,
		"project:",
		"syncing files",
		"running: python test_main.py",
		"all tests passed",
	)
}

// TestE2E_GoProject runs `yg go test ./...` on a minimal Go project.
// Validates: Go provisioning, go mod download, test execution.
func TestE2E_GoProject(t *testing.T) {
	dir := uniqueDir(t)
	setupGoProject(t, dir)
	t.Cleanup(func() { destroyProject(t, dir) })

	out := runFKSuccess(t, dir, 10*time.Minute, "go", "test", "./...")

	requireContainsAll(t, out,
		"project:",
		"syncing files",
		"running: go test ./...",
		"PASS",
	)
}
