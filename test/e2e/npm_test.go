//go:build e2e

package e2e

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestE2E_NPM_PackageStructure verifies the npm package structure
// is correct and all required files are present.
func TestE2E_NPM_PackageStructure(t *testing.T) {
	repoRoot := filepath.Join(t.TempDir(), "..", "..", "..")
	npmDir := filepath.Join(repoRoot, "npm", "yeager")

	// Verify package.json exists.
	packagePath := filepath.Join(npmDir, "package.json")
	require.FileExists(t, packagePath, "package.json should exist")

	// Read and parse package.json.
	data, err := os.ReadFile(packagePath)
	require.NoError(t, err)

	var pkg map[string]interface{}
	err = json.Unmarshal(data, &pkg)
	require.NoError(t, err)

	// Verify package name is @yeager.sh/cli (not @yeager/cli).
	name, ok := pkg["name"].(string)
	require.True(t, ok, "name field should be string")
	assert.Equal(t, "@yeager.sh/cli", name, "package name should be @yeager.sh/cli")

	// Verify bin entry exists.
	bin, ok := pkg["bin"].(map[string]interface{})
	require.True(t, ok, "bin field should be object")
	assert.Contains(t, bin, "yg", "should have yg binary")

	// Verify optionalDependencies use @yeager.sh scope.
	optDeps, ok := pkg["optionalDependencies"].(map[string]interface{})
	require.True(t, ok, "optionalDependencies should be object")

	for dep := range optDeps {
		assert.True(t, strings.HasPrefix(dep, "@yeager.sh/"),
			"optional dependency %s should use @yeager.sh scope", dep)
	}

	// Verify wrapper script exists.
	wrapperPath := filepath.Join(npmDir, "bin", "yeager.js")
	require.FileExists(t, wrapperPath, "wrapper script should exist")

	// Verify wrapper uses correct package names.
	wrapperContent, err := os.ReadFile(wrapperPath)
	require.NoError(t, err)
	assert.Contains(t, string(wrapperContent), "@yeager.sh/",
		"wrapper should reference @yeager.sh packages")
	assert.NotContains(t, string(wrapperContent), "@yeager/",
		"wrapper should not reference old @yeager scope")
}

// TestE2E_NPM_BinaryResolution verifies the platform-specific binary
// resolution logic in the npm wrapper works correctly.
func TestE2E_NPM_BinaryResolution(t *testing.T) {
	repoRoot := filepath.Join(t.TempDir(), "..", "..", "..")
	wrapperPath := filepath.Join(repoRoot, "npm", "yeager", "bin", "yeager.js")

	// Read wrapper script.
	wrapperContent, err := os.ReadFile(wrapperPath)
	require.NoError(t, err)
	wrapper := string(wrapperContent)

	// Verify platform mapping exists for current platform.
	currentPlatform := runtime.GOOS
	currentArch := runtime.GOARCH

	platformKey := currentPlatform + "-" + currentArch
	assert.Contains(t, wrapper, platformKey,
		"wrapper should support current platform %s", platformKey)

	// Verify all expected platforms are mapped.
	expectedPlatforms := []string{
		"darwin-arm64",
		"darwin-x64",
		"linux-arm64",
		"linux-x64",
		"win32-arm64",
		"win32-x64",
	}

	for _, platform := range expectedPlatforms {
		assert.Contains(t, wrapper, platform,
			"wrapper should support platform %s", platform)
	}

	// Verify error handling for unsupported platforms.
	assert.Contains(t, wrapper, "unsupported platform",
		"wrapper should have error message for unsupported platforms")
}

// TestE2E_NPM_PlatformPackagesExist verifies that platform-specific
// package directories exist with correct structure.
func TestE2E_NPM_PlatformPackagesExist(t *testing.T) {
	repoRoot := filepath.Join(t.TempDir(), "..", "..", "..")
	platformsDir := filepath.Join(repoRoot, "npm", "@yeager.sh")

	require.DirExists(t, platformsDir, "@yeager.sh directory should exist")

	expectedPlatforms := []string{
		"darwin-arm64",
		"darwin-x64",
		"linux-arm64",
		"linux-x64",
		"win32-arm64",
		"win32-x64",
	}

	for _, platform := range expectedPlatforms {
		platformDir := filepath.Join(platformsDir, platform)
		require.DirExists(t, platformDir, "platform directory %s should exist", platform)

		// Verify package.json exists.
		packagePath := filepath.Join(platformDir, "package.json")
		require.FileExists(t, packagePath, "package.json should exist for %s", platform)

		// Read and verify package.json.
		data, err := os.ReadFile(packagePath)
		require.NoError(t, err)

		var pkg map[string]interface{}
		err = json.Unmarshal(data, &pkg)
		require.NoError(t, err)

		// Verify package name uses @yeager.sh scope.
		name, ok := pkg["name"].(string)
		require.True(t, ok)
		assert.Equal(t, "@yeager.sh/"+platform, name,
			"package name should be @yeager.sh/%s", platform)

		// Verify bin directory exists.
		binDir := filepath.Join(platformDir, "bin")
		require.DirExists(t, binDir, "bin directory should exist for %s", platform)

		// Verify binary exists (platform-specific name).
		var binaryName string
		if strings.HasPrefix(platform, "win32") {
			binaryName = "yg.exe"
		} else {
			binaryName = "yg"
		}
		binaryPath := filepath.Join(binDir, binaryName)
		require.FileExists(t, binaryPath, "binary %s should exist for %s", binaryName, platform)
	}
}

// TestE2E_NPM_NoOldPackages verifies that old @yeager/* packages
// have been removed and don't exist in the repo.
func TestE2E_NPM_NoOldPackages(t *testing.T) {
	repoRoot := filepath.Join(t.TempDir(), "..", "..", "..")

	// Old @yeager directory should not exist.
	oldDir := filepath.Join(repoRoot, "npm", "@yeager")
	assert.NoDirExists(t, oldDir, "old @yeager directory should not exist")

	// Verify main package.json doesn't reference old packages.
	packagePath := filepath.Join(repoRoot, "npm", "yeager", "package.json")
	data, err := os.ReadFile(packagePath)
	require.NoError(t, err)

	content := string(data)
	assert.NotContains(t, content, "\"@yeager/darwin",
		"package.json should not reference old @yeager/* packages")
	assert.NotContains(t, content, "\"@yeager/linux",
		"package.json should not reference old @yeager/* packages")
	assert.NotContains(t, content, "\"@yeager/win32",
		"package.json should not reference old @yeager/* packages")
}

// TestE2E_NPM_InstallScript verifies npm install succeeds and
// creates proper node_modules structure.
func TestE2E_NPM_InstallScript(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping npm install test in short mode")
	}

	// Check if npm is available.
	if _, err := exec.LookPath("npm"); err != nil {
		t.Skip("npm not available, skipping install test")
	}

	repoRoot := filepath.Join(t.TempDir(), "..", "..", "..")
	npmDir := filepath.Join(repoRoot, "npm", "yeager")

	// Create temp directory for test install.
	testDir := t.TempDir()

	// Copy package.json to test directory.
	packageContent, err := os.ReadFile(filepath.Join(npmDir, "package.json"))
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(testDir, "package.json"), packageContent, 0644)
	require.NoError(t, err)

	// Copy bin directory.
	binSrc := filepath.Join(npmDir, "bin")
	binDst := filepath.Join(testDir, "bin")
	err = exec.Command("cp", "-r", binSrc, binDst).Run()
	require.NoError(t, err)

	// Run npm install.
	cmd := exec.Command("npm", "install", "--no-save")
	cmd.Dir = testDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Logf("npm install output:\n%s", output)
	}
	require.NoError(t, err, "npm install should succeed")

	// Verify node_modules was created.
	nodeModules := filepath.Join(testDir, "node_modules")
	require.DirExists(t, nodeModules, "node_modules should be created")

	// Verify platform-specific package was installed (optional dependency).
	// Note: Only the current platform's package will be installed.
	currentPlatform := runtime.GOOS
	currentArch := runtime.GOARCH
	platformPkg := currentPlatform + "-" + currentArch

	// Map Go platform names to npm platform names.
	if currentPlatform == "darwin" && currentArch == "amd64" {
		platformPkg = "darwin-x64"
	} else if currentArch == "amd64" {
		platformPkg = currentPlatform + "-x64"
	}

	platformDir := filepath.Join(nodeModules, "@yeager.sh", platformPkg)
	// Platform package might not be installed if optional deps are skipped,
	// so we just log if it's missing rather than failing.
	if _, err := os.Stat(platformDir); err != nil {
		t.Logf("Platform package not installed (expected for optional dependencies): %s", platformPkg)
	}
}
