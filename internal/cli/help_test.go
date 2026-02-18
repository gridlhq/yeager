package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHelpOutputContainsSections(t *testing.T) {
	t.Parallel()

	root := newRootCmd("test")
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetArgs([]string{"--help"})
	err := root.Execute()
	require.NoError(t, err)

	out := buf.String()

	// Header / tagline.
	assert.Contains(t, out, "yeager")
	assert.Contains(t, out, "remote execution")

	// Sections.
	assert.Contains(t, out, "Usage:")
	assert.Contains(t, out, "Commands:")
	assert.Contains(t, out, "Setup:")
	assert.Contains(t, out, "Examples:")
	assert.Contains(t, out, "Flags:")
}

func TestHelpCommandsHaveYGPrefix(t *testing.T) {
	t.Parallel()

	root := newRootCmd("test")
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetArgs([]string{"--help"})
	err := root.Execute()
	require.NoError(t, err)

	out := buf.String()

	// Each subcommand should appear with "yg " prefix.
	for _, cmd := range []string{"configure", "status", "logs", "kill", "stop", "destroy", "init", "up"} {
		assert.Contains(t, out, "yg "+cmd, "help should show yg %s", cmd)
	}
}

func TestHelpContainsExamples(t *testing.T) {
	t.Parallel()

	root := newRootCmd("test")
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetArgs([]string{"--help"})
	err := root.Execute()
	require.NoError(t, err)

	out := buf.String()
	assert.Contains(t, out, "cargo test")
	assert.Contains(t, out, "npm run build")
	assert.Contains(t, out, "go test")
}

func TestHelpContainsFlags(t *testing.T) {
	t.Parallel()

	root := newRootCmd("test")
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetArgs([]string{"--help"})
	err := root.Execute()
	require.NoError(t, err)

	out := buf.String()
	assert.Contains(t, out, "--json")
	assert.Contains(t, out, "--quiet")
	assert.Contains(t, out, "--verbose")
	assert.Contains(t, out, "--help")
	// Shorthand flags.
	assert.Contains(t, out, "-j,")
	assert.Contains(t, out, "-q,")
	assert.Contains(t, out, "-v,")
}

func TestHelpNoANSIInNonTTY(t *testing.T) {
	t.Parallel()

	root := newRootCmd("test")
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetArgs([]string{"--help"})
	err := root.Execute()
	require.NoError(t, err)

	out := buf.String()
	assert.False(t, strings.Contains(out, "\033["), "help output to non-TTY should not contain ANSI codes")
}

func TestHelpFooter(t *testing.T) {
	t.Parallel()

	root := newRootCmd("test")
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetArgs([]string{"--help"})
	err := root.Execute()
	require.NoError(t, err)

	out := buf.String()
	assert.Contains(t, out, "yg <command> --help")
	assert.Contains(t, out, "Getting started?")
	assert.Contains(t, out, "yg configure")
}

func TestHelpCommandGrouping(t *testing.T) {
	t.Parallel()

	root := newRootCmd("test")
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetArgs([]string{"--help"})
	err := root.Execute()
	require.NoError(t, err)

	out := buf.String()

	// Daily-use commands should be under "Commands:".
	cmdIdx := strings.Index(out, "Commands:")
	setupIdx := strings.Index(out, "Setup:")
	require.Greater(t, setupIdx, cmdIdx, "Setup section should come after Commands section")

	// Verify daily-use commands appear before setup section.
	for _, cmd := range []string{"yg status", "yg logs", "yg kill", "yg stop", "yg up", "yg destroy"} {
		idx := strings.Index(out, cmd)
		assert.Greater(t, idx, cmdIdx, "%s should appear in Commands section", cmd)
		assert.Less(t, idx, setupIdx, "%s should appear before Setup section", cmd)
	}

	// Verify setup commands appear after setup section header.
	for _, cmd := range []string{"yg configure", "yg init"} {
		idx := strings.Index(out, cmd)
		assert.Greater(t, idx, setupIdx, "%s should appear in Setup section", cmd)
	}
}

func TestHelpContainsHelloWorldExample(t *testing.T) {
	t.Parallel()

	root := newRootCmd("test")
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetArgs([]string{"--help"})
	err := root.Execute()
	require.NoError(t, err)

	assert.Contains(t, buf.String(), "echo 'hello world'")
}
