//go:build livefire

// Package livefire contains Live Fire tests — real AWS, no mocks, no fakes.
// These are 3-tier BDD tests using godog (Cucumber for Go) that exercise every
// CLI interaction path against actual EC2 instances.
//
// Run with: go test -tags livefire -v -count=1 ./test/livefire/...
package livefire

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// lfShared holds state shared across all scenarios in the suite.
// Safe because Concurrency is 0 (sequential execution).
var lfShared struct {
	projectDir string // shared project directory for AWS tests
	binary     string // path to yg binary
}

// findBinary locates the yg binary.
func findBinary() string {
	if b := os.Getenv("YG_BINARY"); b != "" {
		return b
	}
	if b := os.Getenv("FK_BINARY"); b != "" {
		return b
	}
	// Prefer the freshly built binary at project root over a stale PATH install.
	abs, _ := filepath.Abs(filepath.Join("..", "..", "yg"))
	if _, err := os.Stat(abs); err == nil {
		return abs
	}
	// Fall back to PATH.
	if path, err := exec.LookPath("yg"); err == nil {
		return path
	}
	panic("yg binary not found — build with 'make build' or set YG_BINARY")
}

// runYG executes the yg binary with args in the given directory.
// Returns combined output, exit code, and any non-exit-code error (e.g. timeout).
func runYG(dir string, timeout time.Duration, env map[string]string, args ...string) (output string, exitCode int, err error) {
	bin := lfShared.binary

	cmd := exec.Command(bin, args...)
	cmd.Dir = dir

	// Build environment: inherit current + overrides + force non-interactive.
	cmdEnv := os.Environ()
	for k, v := range env {
		cmdEnv = append(cmdEnv, k+"="+v)
	}
	cmdEnv = append(cmdEnv, "TERM=dumb")
	cmd.Env = cmdEnv

	done := make(chan struct{})
	var out []byte
	var cmdErr error

	go func() {
		out, cmdErr = cmd.CombinedOutput()
		close(done)
	}()

	select {
	case <-done:
		if cmdErr != nil {
			if exitErr, ok := cmdErr.(*exec.ExitError); ok {
				return string(out), exitErr.ExitCode(), nil
			}
			return string(out), 1, cmdErr
		}
		return string(out), 0, nil
	case <-time.After(timeout):
		_ = cmd.Process.Kill()
		<-done
		return string(out), -1, fmt.Errorf("timed out after %v", timeout)
	}
}

// writeTestFile writes a file in the given directory, creating parents as needed.
func writeTestFile(dir, name, content string) error {
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

// setupGoProjectFiles creates a minimal Go project.
func setupGoProjectFiles(dir string) error {
	if err := writeTestFile(dir, "go.mod", "module livefire-test\n\ngo 1.21\n"); err != nil {
		return err
	}
	return writeTestFile(dir, "main.go", "package main\n\nfunc main() {}\n")
}

// setupNodeProjectFiles creates a minimal Node.js project.
func setupNodeProjectFiles(dir string) error {
	return writeTestFile(dir, "package.json", `{"name":"lf-test","version":"1.0.0","scripts":{"test":"echo ok"}}`+"\n")
}

// setupPythonProjectFiles creates a minimal Python project.
func setupPythonProjectFiles(dir string) error {
	return writeTestFile(dir, "pyproject.toml", "[project]\nname = \"lf-test\"\nversion = \"0.1.0\"\n")
}

// setupRustProjectFiles creates a minimal Rust project.
func setupRustProjectFiles(dir string) error {
	if err := writeTestFile(dir, "Cargo.toml", "[package]\nname = \"lf-test\"\nversion = \"0.1.0\"\nedition = \"2021\"\n"); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(dir, "src"), 0o755); err != nil {
		return err
	}
	return writeTestFile(dir, "src/lib.rs", "pub fn add(a: i32, b: i32) -> i32 { a + b }\n")
}

// setupSharedProject creates the shared project directory for AWS tests.
func setupSharedProject() error {
	dir, err := os.MkdirTemp("", "livefire-*")
	if err != nil {
		return err
	}
	lfShared.projectDir = dir

	if err := setupGoProjectFiles(dir); err != nil {
		return err
	}

	// Use smallest instance size; disable idle auto-stop during tests.
	return writeTestFile(dir, ".yeager.toml", "[compute]\nsize = \"small\"\n")
}

// destroySharedProject tears down the shared VM and cleans up the temp directory.
func destroySharedProject() {
	if lfShared.projectDir == "" {
		return
	}
	// Best-effort destroy — don't fail the suite if cleanup fails.
	out, _, err := runYG(lfShared.projectDir, 2*time.Minute, nil, "destroy", "--force")
	if err != nil {
		fmt.Fprintf(os.Stderr, "livefire cleanup: destroy failed: %v\n%s\n", err, out)
	}
	os.RemoveAll(lfShared.projectDir)
}

// parseCommand splits a "yg ..." command string into args for runYG.
// Strips the "yg" prefix since runYG prepends the binary path.
func parseCommand(command string) []string {
	args := strings.Fields(command)
	if len(args) > 0 && args[0] == "yg" {
		args = args[1:]
	}
	return args
}

// truncateOutput limits output for readable error messages.
func truncateOutput(s string) string {
	const maxLen = 1500
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + fmt.Sprintf("\n... (%d chars total, truncated)", len(s))
}
