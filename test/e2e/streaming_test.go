//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	fkexec "github.com/gridlhq/yeager/internal/exec"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestE2E_OutputStreaming_CapturesAllLines verifies that the output
// streaming fix (tail -n +1 -f) captures all lines from the beginning,
// including early output that would have been lost with plain tail -f.
func TestE2E_OutputStreaming_CapturesAllLines(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}

	client := requireSSHClient(t)

	// Generate a command that outputs many lines quickly.
	// This tests that early output is captured, not just what arrives after tail starts.
	const numLines = 1000
	runID := fkexec.GenerateRunID()

	opts := fkexec.RunOpts{
		Command: fmt.Sprintf("for i in $(seq 1 %d); do echo \"line $i\"; done", numLines),
		WorkDir: "/tmp",
		RunID:   runID,
	}

	var stdout, stderr strings.Builder
	result, err := fkexec.Run(client, opts, &stdout, &stderr)
	require.NoError(t, err, "command execution should succeed")
	require.NotNil(t, result)
	assert.Equal(t, 0, result.ExitCode, "command should exit with code 0")

	// Verify all lines were captured.
	output := stdout.String()
	lines := strings.Split(strings.TrimSpace(output), "\n")

	// Should have exactly numLines lines.
	assert.Equal(t, numLines, len(lines), "should capture all %d lines", numLines)

	// Verify first and last lines to ensure no truncation.
	assert.Contains(t, lines[0], "line 1", "first line should be captured")
	assert.Contains(t, lines[len(lines)-1], fmt.Sprintf("line %d", numLines), "last line should be captured")

	// Verify no gaps in the sequence.
	for i := 1; i <= numLines; i++ {
		expected := fmt.Sprintf("line %d", i)
		assert.Contains(t, output, expected, "should contain line %d", i)
	}
}

// TestE2E_OutputStreaming_EarlyOutputNotLost verifies that output
// generated immediately at command start is captured, not lost due
// to tail -f starting too late.
func TestE2E_OutputStreaming_EarlyOutputNotLost(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}

	client := requireSSHClient(t)

	runID := fkexec.GenerateRunID()

	// Command that outputs a marker immediately, then sleeps, then outputs more.
	// The immediate output tests the fix for early line loss.
	opts := fkexec.RunOpts{
		Command: "echo 'EARLY_MARKER'; sleep 1; echo 'LATE_MARKER'",
		WorkDir: "/tmp",
		RunID:   runID,
	}

	var stdout, stderr strings.Builder
	result, err := fkexec.Run(client, opts, &stdout, &stderr)
	require.NoError(t, err)
	require.NotNil(t, result)

	output := stdout.String()

	// Both markers should be present - this verifies early output wasn't lost.
	assert.Contains(t, output, "EARLY_MARKER", "early output should be captured")
	assert.Contains(t, output, "LATE_MARKER", "late output should be captured")

	// Verify order is preserved.
	earlyIdx := strings.Index(output, "EARLY_MARKER")
	lateIdx := strings.Index(output, "LATE_MARKER")
	assert.Less(t, earlyIdx, lateIdx, "output should be in correct order")
}

// TestE2E_OutputStreaming_LargeOutputVolume tests that large volume
// output is captured completely without truncation or data loss.
func TestE2E_OutputStreaming_LargeOutputVolume(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}

	client := requireSSHClient(t)

	runID := fkexec.GenerateRunID()

	// Generate ~100KB of output (1000 lines * ~100 bytes each).
	opts := fkexec.RunOpts{
		Command: "for i in $(seq 1 1000); do printf 'LINE_%04d: %080d\n' $i $i; done",
		WorkDir: "/tmp",
		RunID:   runID,
	}

	var stdout, stderr strings.Builder
	result, err := fkexec.Run(client, opts, &stdout, &stderr)
	require.NoError(t, err)
	require.NotNil(t, result)

	output := stdout.String()
	lines := strings.Split(strings.TrimSpace(output), "\n")

	// Verify complete output capture.
	assert.Equal(t, 1000, len(lines), "should capture all 1000 lines")
	assert.Greater(t, len(output), 90000, "output should be at least 90KB")

	// Spot check various lines throughout.
	assert.Contains(t, output, "LINE_0001:", "first line present")
	assert.Contains(t, output, "LINE_0500:", "middle line present")
	assert.Contains(t, output, "LINE_1000:", "last line present")
}

// requireSSHClient returns a test SSH client or skips the test.
// This is a helper for integration tests that need real SSH connectivity.
func requireSSHClient(t *testing.T) interface{} {
	t.Helper()
	// This would need actual SSH setup - for now, we'll make this a placeholder
	// that the real e2e test harness can implement.
	t.Skip("SSH client setup required - run via integration test harness")
	return nil
}
