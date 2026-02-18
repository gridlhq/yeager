package dist_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/gridlhq/yeager/internal/dist"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// repoRoot returns the repository root (two levels up from internal/dist/).
func repoRoot(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	require.True(t, ok)
	return filepath.Join(filepath.Dir(filename), "..", "..")
}

func TestVersionFile_Exists(t *testing.T) {
	root := repoRoot(t)
	v, err := dist.ReadVersionFile(filepath.Join(root, "version.txt"))
	require.NoError(t, err)
	assert.Regexp(t, `^\d+\.\d+\.\d+`, v, "version.txt should contain semver")
}

func TestVersionFile_ConsistentWithNpmPackage(t *testing.T) {
	root := repoRoot(t)

	fileVersion, err := dist.ReadVersionFile(filepath.Join(root, "version.txt"))
	require.NoError(t, err)

	// Main npm package.
	data, err := os.ReadFile(filepath.Join(root, "npm", "yeager", "package.json"))
	require.NoError(t, err)

	var pkg struct {
		Version string `json:"version"`
	}
	require.NoError(t, json.Unmarshal(data, &pkg))
	assert.Equal(t, fileVersion, pkg.Version, "npm/yeager/package.json version should match version.txt")
}

func TestGoReleaserConfig_Valid(t *testing.T) {
	root := repoRoot(t)
	data, err := os.ReadFile(filepath.Join(root, ".goreleaser.yml"))
	require.NoError(t, err)

	var cfg map[string]interface{}
	require.NoError(t, yaml.Unmarshal(data, &cfg), ".goreleaser.yml should be valid YAML")

	// Verify version field.
	assert.Equal(t, 2, cfg["version"], "goreleaser config should use version 2")

	// Verify builds section.
	builds, ok := cfg["builds"].([]interface{})
	require.True(t, ok, "builds should be a list")
	require.Len(t, builds, 1)

	build := builds[0].(map[string]interface{})
	assert.Equal(t, "yg", build["binary"])
	assert.Equal(t, "./cmd/yeager/", build["main"])

	// Verify cross-compile targets.
	goos := build["goos"].([]interface{})
	goarch := build["goarch"].([]interface{})
	assert.ElementsMatch(t, []interface{}{"linux", "darwin", "windows"}, goos)
	assert.ElementsMatch(t, []interface{}{"amd64", "arm64"}, goarch)
}

func TestGoReleaserConfig_LdflagsInjectVersion(t *testing.T) {
	root := repoRoot(t)
	data, err := os.ReadFile(filepath.Join(root, ".goreleaser.yml"))
	require.NoError(t, err)

	var cfg struct {
		Builds []struct {
			Ldflags []string `yaml:"ldflags"`
		} `yaml:"builds"`
	}
	require.NoError(t, yaml.Unmarshal(data, &cfg))
	require.Len(t, cfg.Builds, 1)

	found := false
	for _, lf := range cfg.Builds[0].Ldflags {
		if strings.Contains(lf, "main.version") {
			found = true
			break
		}
	}
	assert.True(t, found, "ldflags should inject main.version")
}

func TestNpmMainPackage_HasRequiredFields(t *testing.T) {
	root := repoRoot(t)
	data, err := os.ReadFile(filepath.Join(root, "npm", "yeager", "package.json"))
	require.NoError(t, err)

	var pkg map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &pkg))

	assert.Equal(t, "@yeager.sh/cli", pkg["name"])
	assert.NotEmpty(t, pkg["version"])
	assert.NotEmpty(t, pkg["description"])
	assert.NotEmpty(t, pkg["bin"])
	assert.NotEmpty(t, pkg["optionalDependencies"])

	// Verify bin points to the JS shim.
	bin := pkg["bin"].(map[string]interface{})
	assert.Equal(t, "bin/yeager.js", bin["yg"])
}

func TestNpmMainPackage_HasAllPlatformDeps(t *testing.T) {
	root := repoRoot(t)
	data, err := os.ReadFile(filepath.Join(root, "npm", "yeager", "package.json"))
	require.NoError(t, err)

	var pkg struct {
		OptionalDependencies map[string]string `json:"optionalDependencies"`
	}
	require.NoError(t, json.Unmarshal(data, &pkg))

	expectedPlatforms := []string{
		"@yeager.sh/darwin-arm64",
		"@yeager.sh/darwin-x64",
		"@yeager.sh/linux-arm64",
		"@yeager.sh/linux-x64",
		"@yeager.sh/win32-arm64",
		"@yeager.sh/win32-x64",
	}

	for _, platform := range expectedPlatforms {
		_, ok := pkg.OptionalDependencies[platform]
		assert.True(t, ok, "missing optionalDependency: %s", platform)
	}
}

func TestNpmPlatformPackages_Valid(t *testing.T) {
	root := repoRoot(t)

	platforms := []struct {
		dir  string
		os   string
		cpu  string
	}{
		{"npm/@yeager.sh/darwin-arm64", "darwin", "arm64"},
		{"npm/@yeager.sh/darwin-x64", "darwin", "x64"},
		{"npm/@yeager.sh/linux-arm64", "linux", "arm64"},
		{"npm/@yeager.sh/linux-x64", "linux", "x64"},
		{"npm/@yeager.sh/win32-arm64", "win32", "arm64"},
		{"npm/@yeager.sh/win32-x64", "win32", "x64"},
	}

	for _, p := range platforms {
		t.Run(p.dir, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join(root, p.dir, "package.json"))
			require.NoError(t, err)

			var pkg struct {
				Name    string   `json:"name"`
				Version string   `json:"version"`
				OS      []string `json:"os"`
				CPU     []string `json:"cpu"`
			}
			require.NoError(t, json.Unmarshal(data, &pkg))

			assert.NotEmpty(t, pkg.Name)
			assert.NotEmpty(t, pkg.Version)
			assert.Contains(t, pkg.OS, p.os, "os field should contain %s", p.os)
			assert.Contains(t, pkg.CPU, p.cpu, "cpu field should contain %s", p.cpu)
		})
	}
}

func TestNpmJsShim_Exists(t *testing.T) {
	root := repoRoot(t)
	shimPath := filepath.Join(root, "npm", "yeager", "bin", "yeager.js")

	data, err := os.ReadFile(shimPath)
	require.NoError(t, err)

	content := string(data)
	assert.Contains(t, content, "#!/usr/bin/env node", "shim should have node shebang")
	assert.Contains(t, content, "PLATFORM_PACKAGES", "shim should reference platform packages")
	assert.Contains(t, content, "execFileSync", "shim should exec the binary")
}

func TestInstallScript_Exists(t *testing.T) {
	root := repoRoot(t)
	data, err := os.ReadFile(filepath.Join(root, "scripts", "install.sh"))
	require.NoError(t, err)

	content := string(data)
	assert.Contains(t, content, "#!/bin/sh", "install script should have sh shebang")
	assert.Contains(t, content, "set -e", "install script should use set -e")
	assert.Contains(t, content, "detect_os", "install script should detect OS")
	assert.Contains(t, content, "detect_arch", "install script should detect arch")
	assert.Contains(t, content, "sha256", "install script should verify checksums")
	assert.Contains(t, content, "GITHUB_REPO=", "install script should reference the repo")
}

func TestInstallScript_HandlesAllPlatforms(t *testing.T) {
	root := repoRoot(t)
	data, err := os.ReadFile(filepath.Join(root, "scripts", "install.sh"))
	require.NoError(t, err)

	content := string(data)
	// Verify OS detection covers linux and darwin.
	assert.Contains(t, content, `Linux*)  echo "linux"`)
	assert.Contains(t, content, `Darwin*) echo "darwin"`)

	// Verify arch detection covers amd64 and arm64.
	assert.Contains(t, content, `echo "amd64"`)
	assert.Contains(t, content, `echo "arm64"`)
}

func TestHomebrewFormula_Valid(t *testing.T) {
	root := repoRoot(t)
	data, err := os.ReadFile(filepath.Join(root, "homebrew-tap", "yeager.rb"))
	require.NoError(t, err)

	content := string(data)
	assert.Contains(t, content, "class Yeager < Formula")
	assert.Contains(t, content, `desc "Remote execution for local AI coding agents"`)
	assert.Contains(t, content, `bin.install "yg"`)
	assert.Contains(t, content, "darwin_arm64")
	assert.Contains(t, content, "darwin_amd64")
	assert.Contains(t, content, "linux_arm64")
	assert.Contains(t, content, "linux_amd64")
}

func TestCIWorkflow_TestExists(t *testing.T) {
	root := repoRoot(t)
	data, err := os.ReadFile(filepath.Join(root, ".github", "workflows", "test.yml"))
	require.NoError(t, err)

	var cfg map[string]interface{}
	require.NoError(t, yaml.Unmarshal(data, &cfg))

	assert.Equal(t, "Test", cfg["name"])

	// Verify jobs exist.
	jobs := cfg["jobs"].(map[string]interface{})
	assert.Contains(t, jobs, "test")
	assert.Contains(t, jobs, "lint")
	assert.Contains(t, jobs, "build")
}

func TestCIWorkflow_ReleaseExists(t *testing.T) {
	root := repoRoot(t)
	data, err := os.ReadFile(filepath.Join(root, ".github", "workflows", "release.yml"))
	require.NoError(t, err)

	var cfg map[string]interface{}
	require.NoError(t, yaml.Unmarshal(data, &cfg))

	assert.Equal(t, "Release", cfg["name"])

	// Verify triggers on tag push.
	on := cfg["on"].(map[string]interface{})
	push := on["push"].(map[string]interface{})
	tags := push["tags"].([]interface{})
	assert.Contains(t, tags, "v*")

	// Verify jobs exist.
	jobs := cfg["jobs"].(map[string]interface{})
	assert.Contains(t, jobs, "goreleaser")
	assert.Contains(t, jobs, "npm-publish")
	assert.Contains(t, jobs, "update-homebrew")
}

func TestVersions_AllConsistent(t *testing.T) {
	root := repoRoot(t)

	// Read canonical version from version.txt.
	canonical, err := dist.ReadVersionFile(filepath.Join(root, "version.txt"))
	require.NoError(t, err)

	// Check main npm package.
	npmData, err := os.ReadFile(filepath.Join(root, "npm", "yeager", "package.json"))
	require.NoError(t, err)
	var npmPkg struct{ Version string }
	require.NoError(t, json.Unmarshal(npmData, &npmPkg))
	assert.Equal(t, canonical, npmPkg.Version, "npm/yeager version mismatch")

	// Check all platform packages.
	platformDirs := []string{
		"npm/@yeager.sh/darwin-arm64",
		"npm/@yeager.sh/darwin-x64",
		"npm/@yeager.sh/linux-arm64",
		"npm/@yeager.sh/linux-x64",
		"npm/@yeager.sh/win32-arm64",
		"npm/@yeager.sh/win32-x64",
	}

	for _, dir := range platformDirs {
		data, err := os.ReadFile(filepath.Join(root, dir, "package.json"))
		require.NoError(t, err)
		var pkg struct{ Version string }
		require.NoError(t, json.Unmarshal(data, &pkg))
		assert.Equal(t, canonical, pkg.Version, "%s version mismatch", dir)
	}

	// Check Homebrew formula.
	brewData, err := os.ReadFile(filepath.Join(root, "homebrew-tap", "yeager.rb"))
	require.NoError(t, err)
	brewRe := regexp.MustCompile(`version "([^"]+)"`)
	matches := brewRe.FindSubmatch(brewData)
	require.Len(t, matches, 2, "homebrew formula should contain version")
	assert.Equal(t, canonical, string(matches[1]), "homebrew formula version mismatch")
}
