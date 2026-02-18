package monitor

import (
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
)

var (
	// Cache the built binary path so we only build once per test run.
	builtBinaryPath string
	buildOnce       sync.Once
	buildErr        error
)

// BuildYeagerBinary builds the yeager CLI binary for integration testing.
// It caches the result so the binary is only built once per test run.
func BuildYeagerBinary(t *testing.T) string {
	t.Helper()

	buildOnce.Do(func() {
		// Create a temporary directory for the binary.
		tmpDir, err := os.MkdirTemp("", "yeager-test-binary-*")
		if err != nil {
			buildErr = err
			return
		}

		// Don't clean up tmpDir - we need it for the duration of all tests.
		// The OS will clean it up eventually.

		binaryPath := filepath.Join(tmpDir, "yeager")

		// Find the repo root by looking for go.mod.
		// We're in internal/monitor, so go up two levels.
		cwd, err := os.Getwd()
		if err != nil {
			buildErr = err
			return
		}
		repoRoot := filepath.Join(cwd, "..", "..")

		// Build the CLI binary.
		cmd := exec.Command("go", "build", "-o", binaryPath, "./cmd/yeager")
		cmd.Dir = repoRoot
		output, err := cmd.CombinedOutput()
		if err != nil {
			buildErr = err
			t.Logf("Build output: %s", output)
			return
		}

		builtBinaryPath = binaryPath
	})

	if buildErr != nil {
		t.Fatalf("failed to build yeager binary: %v", buildErr)
	}

	return builtBinaryPath
}
