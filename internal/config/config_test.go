package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaults(t *testing.T) {
	t.Parallel()

	cfg := Defaults()
	assert.Equal(t, "medium", cfg.Compute.Size)
	assert.Equal(t, "us-east-1", cfg.Compute.Region)
	assert.Equal(t, "2m", cfg.Lifecycle.GracePeriod)
	assert.Equal(t, "10m", cfg.Lifecycle.IdleStop)
	assert.Equal(t, "7d", cfg.Lifecycle.StoppedTerminate)
	assert.Equal(t, "30d", cfg.Lifecycle.TerminatedDeleteAMI)
}

func TestDefaultDurationsParse(t *testing.T) {
	t.Parallel()

	cfg := Defaults()

	d, err := cfg.Lifecycle.GracePeriodDuration()
	require.NoError(t, err)
	assert.Equal(t, 2*time.Minute, d)

	d, err = cfg.Lifecycle.IdleStopDuration()
	require.NoError(t, err)
	assert.Equal(t, 10*time.Minute, d)

	d, err = cfg.Lifecycle.StoppedTerminateDuration()
	require.NoError(t, err)
	assert.Equal(t, 7*24*time.Hour, d)

	d, err = cfg.Lifecycle.TerminatedDeleteAMIDuration()
	require.NoError(t, err)
	assert.Equal(t, 30*24*time.Hour, d)
}

func TestLoadDefaults(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	cfg, configPath, err := Load(dir)
	require.NoError(t, err)
	assert.Empty(t, configPath)
	assert.Equal(t, "medium", cfg.Compute.Size)
	assert.Equal(t, "us-east-1", cfg.Compute.Region)
	assert.Equal(t, "10m", cfg.Lifecycle.IdleStop)
}

func TestLoadFromFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	toml := `
[compute]
size = "large"
region = "eu-west-1"

[lifecycle]
idle_stop = "5m"
stopped_terminate = "3d"

[setup]
packages = ["libpq-dev", "redis-tools"]
run = ["cargo install cargo-nextest"]

[sync]
exclude = ["data/"]

[artifacts]
paths = ["coverage/"]
`
	err := os.WriteFile(filepath.Join(dir, FileName), []byte(toml), 0o644)
	require.NoError(t, err)

	cfg, configPath, err := Load(dir)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(dir, FileName), configPath)
	assert.Equal(t, "large", cfg.Compute.Size)
	assert.Equal(t, "eu-west-1", cfg.Compute.Region)
	assert.Equal(t, "5m", cfg.Lifecycle.IdleStop)
	assert.Equal(t, "3d", cfg.Lifecycle.StoppedTerminate)
	assert.Equal(t, []string{"libpq-dev", "redis-tools"}, cfg.Setup.Packages)
	assert.Equal(t, []string{"cargo install cargo-nextest"}, cfg.Setup.Run)
	assert.Equal(t, []string{"data/"}, cfg.Sync.Exclude)
	assert.Equal(t, []string{"coverage/"}, cfg.Artifacts.Paths)
}

func TestLoadPartialFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	toml := `
[compute]
size = "small"
`
	err := os.WriteFile(filepath.Join(dir, FileName), []byte(toml), 0o644)
	require.NoError(t, err)

	cfg, _, err := Load(dir)
	require.NoError(t, err)
	assert.Equal(t, "small", cfg.Compute.Size)
	assert.Equal(t, "us-east-1", cfg.Compute.Region) // default preserved
}

func TestLoadEnvOverride(t *testing.T) {
	dir := t.TempDir()
	toml := `
[compute]
size = "small"
`
	err := os.WriteFile(filepath.Join(dir, FileName), []byte(toml), 0o644)
	require.NoError(t, err)

	t.Setenv("YEAGER_COMPUTE_SIZE", "xlarge")

	cfg, _, err := Load(dir)
	require.NoError(t, err)
	assert.Equal(t, "xlarge", cfg.Compute.Size)
}

func TestLoadEnvWithoutFile(t *testing.T) {
	dir := t.TempDir()

	t.Setenv("YEAGER_COMPUTE_REGION", "ap-southeast-1")

	cfg, configPath, err := Load(dir)
	require.NoError(t, err)
	assert.Empty(t, configPath)
	assert.Equal(t, "ap-southeast-1", cfg.Compute.Region)
}

func TestFindConfigWalksUp(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	sub1 := filepath.Join(root, "sub1")
	sub2 := filepath.Join(sub1, "sub2")
	sub3 := filepath.Join(sub2, "sub3")
	require.NoError(t, os.MkdirAll(sub3, 0o755))

	configFile := filepath.Join(sub1, FileName)
	require.NoError(t, os.WriteFile(configFile, []byte("[compute]\nsize = \"large\"\n"), 0o644))

	found := FindConfig(sub3)
	assert.Equal(t, configFile, found)

	found = FindConfig(sub1)
	assert.Equal(t, configFile, found)
}

func TestFindConfigNotFound(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	found := FindConfig(dir)
	assert.Empty(t, found)
}

func TestFindConfigStopsAtNearestFile(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	inner := filepath.Join(root, "inner")
	require.NoError(t, os.MkdirAll(inner, 0o755))

	require.NoError(t, os.WriteFile(filepath.Join(root, FileName), []byte("[compute]\nsize = \"small\"\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(inner, FileName), []byte("[compute]\nsize = \"large\"\n"), 0o644))

	found := FindConfig(inner)
	assert.Equal(t, filepath.Join(inner, FileName), found)
}

func TestValidateInvalidSize(t *testing.T) {
	t.Parallel()

	cfg := Defaults()
	cfg.Compute.Size = "mega"
	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid compute.size")
}

func TestValidateValidSizes(t *testing.T) {
	t.Parallel()

	for _, size := range []string{"small", "medium", "large", "xlarge"} {
		t.Run(size, func(t *testing.T) {
			t.Parallel()
			cfg := Defaults()
			cfg.Compute.Size = size
			assert.NoError(t, cfg.Validate())
		})
	}
}

func TestValidateInvalidDuration(t *testing.T) {
	t.Parallel()

	cfg := Defaults()
	cfg.Lifecycle.IdleStop = "banana"
	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid lifecycle.idle_stop")
}

func TestParseDuration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		wantDur time.Duration
	}{
		{"minutes", "10m", 10 * time.Minute},
		{"hours", "2h", 2 * time.Hour},
		{"days", "7d", 7 * 24 * time.Hour},
		{"30 days", "30d", 30 * 24 * time.Hour},
		{"seconds", "30s", 30 * time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			d, err := ParseDuration(tt.input)
			require.NoError(t, err)
			assert.Equal(t, tt.wantDur, d)
		})
	}
}

func TestParseDurationInvalid(t *testing.T) {
	t.Parallel()

	_, err := ParseDuration("banana")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid duration")
}

func TestTemplateIsValidTOML(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	err := os.WriteFile(filepath.Join(dir, FileName), []byte(Template), 0o644)
	require.NoError(t, err)

	cfg, _, err := Load(dir)
	require.NoError(t, err)
	assert.Equal(t, "medium", cfg.Compute.Size)
}
