//go:build e2e

package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestE2E_InstallScriptPostInstallGuidance verifies the install script
// prints post-install next steps after successful installation.
func TestE2E_InstallScriptPostInstallGuidance(t *testing.T) {
	scriptPath := filepath.Join(repoRoot(t), "scripts", "install.sh")

	// Verify the script source contains the expected post-install guidance.
	scriptContent, err := os.ReadFile(scriptPath)
	require.NoError(t, err)

	scriptText := string(scriptContent)

	// Verify post-install guidance is present in the script.
	assert.Contains(t, scriptText, "next steps:", "install script should include next steps")
	assert.Contains(t, scriptText, "aws configure", "install script should mention aws configure")
	assert.Contains(t, scriptText, "yg cargo test", "install script should show example command")

	// Verify rsync check is present.
	assert.Contains(t, scriptText, "rsync not found", "install script should check for rsync")
	assert.Contains(t, scriptText, "warning:", "install script should warn about missing rsync")

	// Verify the rsync warning is non-blocking (doesn't call err()).
	rsyncCheckStart := strings.Index(scriptText, "if ! has_cmd rsync")
	if rsyncCheckStart != -1 {
		rsyncCheckEnd := strings.Index(scriptText[rsyncCheckStart:], "fi")
		rsyncCheckBlock := scriptText[rsyncCheckStart : rsyncCheckStart+rsyncCheckEnd]
		assert.NotContains(t, rsyncCheckBlock, "err(", "rsync check should not call err() - should be non-blocking")
	}
}

// TestE2E_InstallScriptStructure verifies the install script has all
// expected sections and follows best practices.
func TestE2E_InstallScriptStructure(t *testing.T) {
	scriptPath := filepath.Join(repoRoot(t), "scripts", "install.sh")
	scriptContent, err := os.ReadFile(scriptPath)
	require.NoError(t, err)

	scriptText := string(scriptContent)

	// Verify script header and shebang.
	assert.True(t, strings.HasPrefix(scriptText, "#!/bin/sh"), "script should have sh shebang")
	assert.Contains(t, scriptText, "set -e", "script should have set -e for error handling")

	// Verify essential functions exist.
	assert.Contains(t, scriptText, "log()", "script should have log() function")
	assert.Contains(t, scriptText, "err()", "script should have err() function")
	assert.Contains(t, scriptText, "detect_os()", "script should have detect_os() function")
	assert.Contains(t, scriptText, "detect_arch()", "script should have detect_arch() function")
	assert.Contains(t, scriptText, "verify_checksum()", "script should have verify_checksum() function")

	// Verify platform detection.
	assert.Contains(t, scriptText, "Linux", "script should detect Linux")
	assert.Contains(t, scriptText, "Darwin", "script should detect macOS")
	assert.Contains(t, scriptText, "x86_64", "script should detect x86_64")
	assert.Contains(t, scriptText, "arm64", "script should detect arm64")

	// Verify download tool detection.
	assert.Contains(t, scriptText, "curl", "script should support curl")
	assert.Contains(t, scriptText, "wget", "script should support wget")

	// Verify GitHub release URL construction.
	assert.Contains(t, scriptText, "github.com", "script should use GitHub releases")
	assert.Contains(t, scriptText, "gridlhq/yeager", "script should use correct repo")
}

// TestE2E_InstallScriptRsyncWarning verifies the install script warns
// about missing rsync without blocking installation.
func TestE2E_InstallScriptRsyncWarning(t *testing.T) {
	scriptPath := filepath.Join(repoRoot(t), "scripts", "install.sh")
	scriptContent, err := os.ReadFile(scriptPath)
	require.NoError(t, err)

	scriptText := string(scriptContent)

	// Find the rsync check block.
	assert.Contains(t, scriptText, "if ! has_cmd rsync", "script should check for rsync")

	// Verify the warning message content.
	assert.Contains(t, scriptText, "warning: rsync not found", "script should warn about missing rsync")
	assert.Contains(t, scriptText, "yeager requires rsync", "script should explain why rsync is needed")

	// Verify platform-specific install instructions.
	assert.Contains(t, scriptText, "sudo apt install rsync", "script should show apt install command")
	assert.Contains(t, scriptText, "sudo dnf install rsync", "script should show dnf install command")
	assert.Contains(t, scriptText, "macOS: rsync is pre-installed", "script should note macOS has rsync")
}

// repoRoot returns the absolute path to the repository root.
func repoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	require.NoError(t, err)
	// From test/e2e/, go up two levels.
	return filepath.Join(wd, "..", "..")
}
