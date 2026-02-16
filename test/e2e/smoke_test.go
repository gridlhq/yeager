//go:build e2e

package e2e

import (
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestE2E_BinaryVersionOutput verifies the built binary outputs a version string.
func TestE2E_BinaryVersionOutput(t *testing.T) {
	bin := fkBinary(t)

	cmd := exec.Command(bin, "--version")
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "yg --version failed: %s", output)

	out := strings.TrimSpace(string(output))
	// Cobra outputs "yg version <version>". The version is "dev" when
	// built from source or a semver like "0.5.0" from goreleaser.
	assert.Contains(t, out, "yg version",
		"expected 'yg version <ver>' format, got: %s", out)
}

// TestE2E_HelpOutput verifies --help shows all subcommands.
func TestE2E_HelpOutput(t *testing.T) {
	bin := fkBinary(t)

	cmd := exec.Command(bin, "--help")
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "yg --help failed: %s", output)

	out := string(output)
	requireContainsAll(t, out,
		"status",
		"logs",
		"stop",
		"destroy",
		"init",
		"up",
	)
}

// TestE2E_InitCreatesConfig verifies `yg init` creates a .yeager.toml file.
func TestE2E_InitCreatesConfig(t *testing.T) {
	dir := uniqueDir(t)

	out := runFKSuccess(t, dir, 10*time.Second, "init")
	assert.Contains(t, out, ".yeager.toml")
}

// TestE2E_SmokeInstallScript verifies the install script can be downloaded and
// produces a valid binary.
//
// This test downloads the install script from the GitHub releases and runs it
// in a temp directory. It validates the Phase 6 distribution pipeline.
//
// Prerequisites:
// - Internet access
// - curl available
// - Running on macOS or Linux
func TestE2E_SmokeInstallScript(t *testing.T) {
	t.Skip("install script smoke test requires published GitHub release")

	if runtime.GOOS == "windows" {
		t.Skip("install script not supported on Windows")
	}

	dir := t.TempDir()

	// Download and run the install script.
	installCmd := exec.Command("bash", "-c",
		"curl -fsSL https://yeager.dev/install | INSTALL_DIR="+dir+" sh")
	installCmd.Dir = dir

	installOutput, err := installCmd.CombinedOutput()
	require.NoError(t, err, "install script failed:\n%s", installOutput)

	// Verify the binary was installed and works.
	fkPath := dir + "/yg"
	versionCmd := exec.Command(fkPath, "--version")
	versionOutput, err := versionCmd.CombinedOutput()
	require.NoError(t, err, "installed yg --version failed:\n%s", versionOutput)

	out := string(versionOutput)
	assert.Contains(t, out, "yg", "expected version output from installed binary")
}
