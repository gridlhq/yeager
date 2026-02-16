//go:build e2e

package e2e

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// fkBinary returns the path to the built yeager binary.
// Set FK_BINARY env var to override (default: looks for "yg" in PATH).
func fkBinary(t *testing.T) string {
	t.Helper()
	if b := os.Getenv("FK_BINARY"); b != "" {
		return b
	}
	path, err := exec.LookPath("yg")
	require.NoError(t, err, "yg binary not found in PATH; build with 'make build' or set FK_BINARY")
	return path
}

// runFK executes yg in the given directory with the given args.
// Returns combined stdout+stderr and any error.
// Safe to call from any goroutine (does not call t.Fatalf).
func runFK(t *testing.T, dir string, timeout time.Duration, args ...string) (string, error) {
	t.Helper()
	bin := fkBinary(t)

	cmd := exec.Command(bin, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		// Ensure non-interactive mode.
		"TERM=dumb",
	)

	done := make(chan struct{})
	var output []byte
	var cmdErr error

	go func() {
		output, cmdErr = cmd.CombinedOutput()
		close(done)
	}()

	select {
	case <-done:
		return string(output), cmdErr
	case <-time.After(timeout):
		_ = cmd.Process.Kill()
		<-done // wait for goroutine to exit after kill
		return string(output), fmt.Errorf("yg %s timed out after %v", strings.Join(args, " "), timeout)
	}
}

// runFKSuccess calls runFK and requires no error.
func runFKSuccess(t *testing.T, dir string, timeout time.Duration, args ...string) string {
	t.Helper()
	out, err := runFK(t, dir, timeout, args...)
	require.NoError(t, err, "yg %s failed:\n%s", strings.Join(args, " "), out)
	return out
}

// destroyProject cleans up the VM for a project directory.
// Best-effort — failures are logged but don't fail the test.
func destroyProject(t *testing.T, dir string) {
	t.Helper()
	out, err := runFK(t, dir, 2*time.Minute, "destroy")
	if err != nil {
		t.Logf("cleanup: yg destroy failed (best-effort): %s\n%s", err, out)
	}
}

// setupRustProject creates a minimal Rust project in the given directory.
func setupRustProject(t *testing.T, dir string) {
	t.Helper()

	// Cargo.toml
	writeFile(t, filepath.Join(dir, "Cargo.toml"), `[package]
name = "yg-e2e-test"
version = "0.1.0"
edition = "2021"
`)

	// src/lib.rs with a passing test
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "src"), 0o755))
	writeFile(t, filepath.Join(dir, "src", "lib.rs"), `pub fn add(a: i32, b: i32) -> i32 {
    a + b
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_add() {
        assert_eq!(add(2, 3), 5);
    }
}
`)
}

// setupNodeProject creates a minimal Node.js project in the given directory.
func setupNodeProject(t *testing.T, dir string) {
	t.Helper()

	writeFile(t, filepath.Join(dir, "package.json"), `{
  "name": "yg-e2e-test",
  "version": "1.0.0",
  "scripts": {
    "test": "node test.js"
  }
}
`)

	writeFile(t, filepath.Join(dir, "test.js"), `const assert = require('assert');

function add(a, b) { return a + b; }

assert.strictEqual(add(2, 3), 5);
console.log('all tests passed');
process.exit(0);
`)
}

// setupPythonProject creates a minimal Python project in the given directory.
func setupPythonProject(t *testing.T, dir string) {
	t.Helper()

	writeFile(t, filepath.Join(dir, "pyproject.toml"), `[project]
name = "yg-e2e-test"
version = "0.1.0"
requires-python = ">=3.8"

[build-system]
requires = ["setuptools"]
build-backend = "setuptools.backends._legacy:_Backend"
`)

	writeFile(t, filepath.Join(dir, "test_main.py"), `def add(a, b):
    return a + b

def test_add():
    assert add(2, 3) == 5

if __name__ == "__main__":
    test_add()
    print("all tests passed")
`)
}

// setupGoProject creates a minimal Go project in the given directory.
func setupGoProject(t *testing.T, dir string) {
	t.Helper()

	writeFile(t, filepath.Join(dir, "go.mod"), `module yg-e2e-test

go 1.21
`)

	writeFile(t, filepath.Join(dir, "main.go"), `package main

func Add(a, b int) int {
	return a + b
}

func main() {}
`)

	writeFile(t, filepath.Join(dir, "main_test.go"), `package main

import "testing"

func TestAdd(t *testing.T) {
	if Add(2, 3) != 5 {
		t.Fatal("expected 5")
	}
}
`)
}

// setupProjectWithArtifacts creates a project that produces artifacts.
func setupProjectWithArtifacts(t *testing.T, dir string) {
	t.Helper()

	// Minimal Go project that writes an artifact file.
	writeFile(t, filepath.Join(dir, "go.mod"), `module yg-e2e-test

go 1.21
`)

	writeFile(t, filepath.Join(dir, "main.go"), `package main

import "os"

func main() {
	os.MkdirAll("output", 0o755)
	os.WriteFile("output/result.txt", []byte("build complete\n"), 0o644)
}
`)

	writeFile(t, filepath.Join(dir, ".yeager.toml"), `[artifacts]
paths = ["output/result.txt"]
`)
}

// setupLongRunningProject creates a project with a command that takes a while.
// Useful for testing disconnect resilience and concurrent commands.
func setupLongRunningProject(t *testing.T, dir string) {
	t.Helper()

	writeFile(t, filepath.Join(dir, "go.mod"), `module yg-e2e-test

go 1.21
`)

	writeFile(t, filepath.Join(dir, "main.go"), `package main

import (
	"fmt"
	"time"
)

func main() {
	for i := 1; i <= 10; i++ {
		fmt.Printf("tick %d\n", i)
		time.Sleep(1 * time.Second)
	}
	fmt.Println("done")
}
`)
}

// writeFile writes content to a file, creating parent directories as needed.
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

// waitForStatus polls `yg status` until the output contains the expected substring
// or the timeout expires.
func waitForStatus(t *testing.T, dir, want string, timeout time.Duration) string {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		out, _ := runFK(t, dir, 30*time.Second, "status")
		if strings.Contains(out, want) {
			return out
		}
		time.Sleep(5 * time.Second)
	}
	t.Fatalf("status did not contain %q within %v", want, timeout)
	return ""
}

// uniqueDir creates a uniquely named temp directory for a test project.
// The name includes the test name to help with debugging.
func uniqueDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	// t.TempDir() already creates a unique directory with test name.
	return dir
}

// requireContainsAll checks that output contains all of the expected substrings.
func requireContainsAll(t *testing.T, output string, substrings ...string) {
	t.Helper()
	for _, s := range substrings {
		if !strings.Contains(output, s) {
			t.Errorf("output missing %q:\n%s", s, truncate(output, 500))
		}
	}
}

// truncate limits a string to maxLen chars, appending "..." if truncated.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + fmt.Sprintf("... (%d chars total)", len(s))
}
