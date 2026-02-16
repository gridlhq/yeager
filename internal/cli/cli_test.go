package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/gridlhq/yeager/internal/config"
	"github.com/gridlhq/yeager/internal/output"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRootHelpContainsAllSubcommands(t *testing.T) {
	t.Parallel()

	root := newRootCmd("test")
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetArgs([]string{"--help"})
	err := root.Execute()
	require.NoError(t, err)

	helpOutput := buf.String()
	for _, cmd := range []string{"status", "logs", "kill", "stop", "destroy", "init", "up"} {
		assert.Contains(t, helpOutput, cmd, "help should mention %s subcommand", cmd)
	}
}

func TestVersionFlag(t *testing.T) {
	t.Parallel()

	root := newRootCmd("1.2.3")
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetArgs([]string{"--version"})
	err := root.Execute()
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "1.2.3")
}

func TestSubcommandsExist(t *testing.T) {
	t.Parallel()

	root := newRootCmd("test")
	subcommands := map[string]bool{}
	for _, cmd := range root.Commands() {
		subcommands[cmd.Name()] = true
	}

	for _, name := range []string{"status", "logs", "kill", "stop", "destroy", "init", "up"} {
		assert.True(t, subcommands[name], "subcommand %s should exist", name)
	}
}

func TestRunWithNoArgs(t *testing.T) {
	t.Parallel()

	root := newRootCmd("test")
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetArgs([]string{})
	err := root.Execute()
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "remote Linux VM")
}

func TestRootHelpContainsExamples(t *testing.T) {
	t.Parallel()

	root := newRootCmd("test")
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetArgs([]string{"--help"})
	err := root.Execute()
	require.NoError(t, err)

	helpOutput := buf.String()
	assert.Contains(t, helpOutput, "cargo test")
	assert.Contains(t, helpOutput, "npm run build")
	assert.Contains(t, helpOutput, "go test")
}

func TestSubcommandHelpTextQuality(t *testing.T) {
	t.Parallel()

	root := newRootCmd("test")
	cmds := map[string]struct {
		wantLong    string
		wantExample string
	}{
		"status":  {wantLong: "current state"},
		"logs":    {wantLong: "Replays all output", wantExample: "yg logs"},
		"kill":    {wantLong: "Cancels a running command", wantExample: "yg kill"},
		"stop":    {wantLong: "Stops the VM"},
		"destroy": {wantLong: "Terminates the VM"},
		"up":      {wantLong: "Creates or starts"},
		"init":    {wantLong: "Creates a .yeager.toml"},
	}

	for _, cmd := range root.Commands() {
		expected, ok := cmds[cmd.Name()]
		if !ok {
			continue
		}
		t.Run(cmd.Name(), func(t *testing.T) {
			t.Parallel()
			assert.NotEmpty(t, cmd.Long, "%s should have a Long description", cmd.Name())
			assert.Contains(t, cmd.Long, expected.wantLong, "%s Long description", cmd.Name())
			if expected.wantExample != "" {
				assert.NotEmpty(t, cmd.Example, "%s should have examples", cmd.Name())
				assert.Contains(t, cmd.Example, expected.wantExample, "%s Example", cmd.Name())
			}
		})
	}
}

func TestLogsAcceptsTailFlag(t *testing.T) {
	t.Parallel()

	root := newRootCmd("test")
	root.SetArgs([]string{"logs", "--tail", "50"})
	err := root.Execute()
	require.NoError(t, err)
}

func TestLogsAcceptsRunID(t *testing.T) {
	t.Parallel()

	// Verify the logs command accepts a run ID argument (cobra validation only).
	root := newRootCmd("test")
	logsCmd, _, err := root.Find([]string{"logs"})
	require.NoError(t, err)
	require.NoError(t, logsCmd.Args(logsCmd, []string{"007"}))
}

func TestKillAcceptsRunID(t *testing.T) {
	t.Parallel()

	// Verify the kill command accepts a run ID argument (cobra validation only).
	root := newRootCmd("test")
	killCmd, _, err := root.Find([]string{"kill"})
	require.NoError(t, err)
	require.NoError(t, killCmd.Args(killCmd, []string{"007"}))
}

// --- Init tests (using RunInit directly, no os.Chdir needed) ---

func TestInitCreatesConfigFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	err := RunInit(dir, false, output.ModeText)
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(dir, config.FileName))
	require.NoError(t, err)
	assert.Equal(t, config.Template, string(data))
}

func TestInitRefusesOverwrite(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, config.FileName), []byte("existing"), 0o644))

	err := RunInit(dir, false, output.ModeText)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")

	// Original content should be preserved.
	data, err := os.ReadFile(filepath.Join(dir, config.FileName))
	require.NoError(t, err)
	assert.Equal(t, "existing", string(data))
}

func TestInitForceOverwrites(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, config.FileName), []byte("old content"), 0o644))

	err := RunInit(dir, true, output.ModeText)
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(dir, config.FileName))
	require.NoError(t, err)
	assert.Equal(t, config.Template, string(data))
}

func TestInitGeneratedFileIsValidConfig(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	require.NoError(t, RunInit(dir, false, output.ModeText))

	cfg, configPath, err := config.Load(dir)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(dir, config.FileName), configPath)
	assert.Equal(t, "medium", cfg.Compute.Size)
	assert.Equal(t, "us-east-1", cfg.Compute.Region)
}
