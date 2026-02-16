package exec

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateRunID(t *testing.T) {
	t.Parallel()

	id := GenerateRunID()
	assert.Len(t, string(id), 8, "run ID should be 8 hex chars")
	assert.NoError(t, ValidateRunID(id.String()), "generated ID should be valid")
}

func TestGenerateRunID_Uniqueness(t *testing.T) {
	t.Parallel()

	ids := make(map[RunID]bool)
	for i := 0; i < 100; i++ {
		id := GenerateRunID()
		assert.False(t, ids[id], "run ID collision: %s", id)
		ids[id] = true
	}
}

func TestRunID_String(t *testing.T) {
	t.Parallel()

	id := RunID("abc12345")
	assert.Equal(t, "abc12345", id.String())
}

func TestValidateRunID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		id      string
		wantErr bool
	}{
		{"valid hex", "abc12345", false},
		{"valid all digits", "00000000", false},
		{"valid all letters", "abcdefab", false},
		{"too short", "abc1234", true},
		{"too long", "abc123456", true},
		{"uppercase", "ABC12345", true},
		{"shell injection attempt", "abc'; rm -rf /; echo '", true},
		{"path traversal", "../../../", true},
		{"empty", "", true},
		{"spaces", "abc 1234", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateRunID(tt.id)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "invalid run ID")
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestRunResult_Duration(t *testing.T) {
	t.Parallel()

	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(47 * time.Second)

	result := &RunResult{
		StartTime: start,
		EndTime:   end,
	}
	assert.Equal(t, 47*time.Second, result.Duration())
}

func TestShellEscape(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "no special chars",
			input:    "cargo test",
			expected: "cargo test",
		},
		{
			name:     "single quote",
			input:    "echo 'hello'",
			expected: "echo '\\''hello'\\''",
		},
		{
			name:     "multiple single quotes",
			input:    "echo 'a' 'b'",
			expected: "echo '\\''a'\\'' '\\''b'\\''",
		},
		{
			name:     "double quotes unchanged",
			input:    `echo "hello world"`,
			expected: `echo "hello world"`,
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.expected, shellEscape(tt.input))
		})
	}
}

func TestBuildTmuxCommand(t *testing.T) {
	t.Parallel()

	cmd := buildTmuxCommand(RunOpts{
		Command: "cargo test",
		WorkDir: "/home/ubuntu/project",
		RunID:   "abc12345",
	})

	// Verify it starts a detached tmux session with the correct session name.
	assert.Contains(t, cmd, "tmux new-session -d -s yg-abc12345")

	// The inner script is shell-escaped inside the outer tmux single quotes,
	// so internal single quotes become '\'' sequences.
	assert.Contains(t, cmd, "cd /home/ubuntu/project")
	assert.Contains(t, cmd, "cargo test")
	assert.Contains(t, cmd, "/tmp/yg-run-abc12345")
	assert.Contains(t, cmd, "tee /tmp/yg-log-abc12345")
	assert.Contains(t, cmd, "/tmp/yg-exit-abc12345")
	assert.Contains(t, cmd, "rm -f /tmp/yg-run-abc12345")
}

func TestBuildTmuxCommand_SingleQuotes(t *testing.T) {
	t.Parallel()

	cmd := buildTmuxCommand(RunOpts{
		Command: "echo 'hello world'",
		WorkDir: "/home/ubuntu/project",
		RunID:   "def67890",
	})

	// Single quotes in the user command should be properly escaped.
	// The inner script escapes them once, then the outer tmux wrapper escapes again.
	assert.Contains(t, cmd, "hello world")
	assert.Contains(t, cmd, "tmux new-session -d -s yg-def67890")
}

func TestBuildTmuxCommand_UsesConstants(t *testing.T) {
	t.Parallel()

	cmd := buildTmuxCommand(RunOpts{
		Command: "ls",
		WorkDir: "/home/ubuntu/project",
		RunID:   "11223344",
	})

	// Verify marker files use the defined constants.
	assert.Contains(t, cmd, markerDir+"/"+markerPrefix+"11223344")
	assert.Contains(t, cmd, markerDir+"/"+exitPrefix+"11223344")
	assert.Contains(t, cmd, markerDir+"/"+logPrefix+"11223344")
	// Verify tmux session name.
	assert.Contains(t, cmd, tmuxPrefix+"11223344")
}

func TestBuildTmuxCommand_CapturesExitCode(t *testing.T) {
	t.Parallel()

	cmd := buildTmuxCommand(RunOpts{
		Command: "make test",
		WorkDir: "/home/ubuntu/project",
		RunID:   "aabbccdd",
	})

	// Uses PIPESTATUS to capture exit code through the tee pipe.
	assert.Contains(t, cmd, "PIPESTATUS[0]")
	assert.Contains(t, cmd, "echo $EC > /tmp/yg-exit-aabbccdd")
}

func TestLogPath(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "/tmp/yg-log-abc12345", LogPath(RunID("abc12345")))
}

func TestTmuxSession(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "yg-abc12345", TmuxSession(RunID("abc12345")))
}

func TestParseTmuxOutput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		output   string
		expected []ActiveRun
	}{
		{
			name:     "empty output",
			output:   "",
			expected: nil,
		},
		{
			name: "single run with full metadata",
			output: "===TMUX:abc12345\n" +
				"cargo test\n" +
				"2026-02-15T10:30:00Z\n",
			expected: []ActiveRun{
				{
					RunID:     "abc12345",
					Command:   "cargo test",
					StartTime: time.Date(2026, 2, 15, 10, 30, 0, 0, time.UTC),
				},
			},
		},
		{
			name: "multiple runs",
			output: "===TMUX:aaa11111\n" +
				"npm test\n" +
				"2026-02-15T09:00:00Z\n" +
				"===TMUX:bbb22222\n" +
				"go test ./...\n" +
				"2026-02-15T09:05:00Z\n",
			expected: []ActiveRun{
				{
					RunID:     "aaa11111",
					Command:   "npm test",
					StartTime: time.Date(2026, 2, 15, 9, 0, 0, 0, time.UTC),
				},
				{
					RunID:     "bbb22222",
					Command:   "go test ./...",
					StartTime: time.Date(2026, 2, 15, 9, 5, 0, 0, time.UTC),
				},
			},
		},
		{
			name: "marker file with only command (no timestamp)",
			output: "===TMUX:ccc33333\n" +
				"pytest\n",
			expected: []ActiveRun{
				{
					RunID:   "ccc33333",
					Command: "pytest",
				},
			},
		},
		{
			name:   "tmux session with no marker file content",
			output: "===TMUX:ddd44444\n",
			expected: []ActiveRun{
				{
					RunID: "ddd44444",
				},
			},
		},
		{
			name: "invalid timestamp is ignored gracefully",
			output: "===TMUX:eee55555\n" +
				"make build\n" +
				"not-a-timestamp\n",
			expected: []ActiveRun{
				{
					RunID:   "eee55555",
					Command: "make build",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := parseTmuxOutput(tt.output)
			if tt.expected == nil {
				assert.Nil(t, result)
			} else {
				require.Equal(t, len(tt.expected), len(result))
				for i, want := range tt.expected {
					assert.Equal(t, want.RunID, result[i].RunID)
					assert.Equal(t, want.Command, result[i].Command)
					if !want.StartTime.IsZero() {
						assert.True(t, want.StartTime.Equal(result[i].StartTime),
							"expected %v, got %v", want.StartTime, result[i].StartTime)
					}
				}
			}
		})
	}
}

func TestBuildActiveRun(t *testing.T) {
	t.Parallel()

	t.Run("full content", func(t *testing.T) {
		t.Parallel()
		run := buildActiveRun("abc12345", []string{"cargo test", "2026-02-15T10:30:00Z"})
		assert.Equal(t, "abc12345", run.RunID)
		assert.Equal(t, "cargo test", run.Command)
		assert.True(t, run.StartTime.Equal(time.Date(2026, 2, 15, 10, 30, 0, 0, time.UTC)))
	})

	t.Run("empty content", func(t *testing.T) {
		t.Parallel()
		run := buildActiveRun("def67890", nil)
		assert.Equal(t, "def67890", run.RunID)
		assert.Empty(t, run.Command)
		assert.True(t, run.StartTime.IsZero())
	})

	t.Run("command only", func(t *testing.T) {
		t.Parallel()
		run := buildActiveRun("fff99999", []string{"make"})
		assert.Equal(t, "fff99999", run.RunID)
		assert.Equal(t, "make", run.Command)
		assert.True(t, run.StartTime.IsZero())
	})
}

func TestListRuns_NilClient(t *testing.T) {
	t.Parallel()

	_, err := ListRuns(nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "SSH client is nil")
}

func TestKill_InvalidRunID(t *testing.T) {
	t.Parallel()

	// Kill validates run ID before attempting SSH. Invalid ID should fail immediately.
	err := Kill(nil, RunID("bad!data"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid run ID")
}

func TestIsRunActive_InvalidRunID(t *testing.T) {
	t.Parallel()

	_, err := IsRunActive(nil, RunID("../hack!"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid run ID")
}

func TestTailLog_InvalidRunID(t *testing.T) {
	t.Parallel()

	err := TailLog(nil, RunID("bad;rm -rf /"), nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid run ID")
}

func TestConstants(t *testing.T) {
	t.Parallel()

	// Verify the constants that form the contract for tmux sessions and marker files.
	// These must not change accidentally or remote execution will break.
	assert.Equal(t, "yg-", tmuxPrefix)
	assert.Equal(t, "yg-run-", markerPrefix)
	assert.Equal(t, "yg-exit-", exitPrefix)
	assert.Equal(t, "yg-log-", logPrefix)
	assert.Equal(t, "/tmp", markerDir)
}

func TestSplitLines(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "empty",
			input:    "",
			expected: nil,
		},
		{
			name:     "single line no newline",
			input:    "hello",
			expected: []string{"hello"},
		},
		{
			name:     "single line with newline",
			input:    "hello\n",
			expected: []string{"hello"},
		},
		{
			name:     "multiple lines",
			input:    "a\nb\nc\n",
			expected: []string{"a", "b", "c"},
		},
		{
			name:     "lines without trailing newline",
			input:    "a\nb",
			expected: []string{"a", "b"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := splitLines(tt.input)
			if tt.expected == nil {
				assert.Nil(t, result)
			} else {
				require.Equal(t, tt.expected, result)
			}
		})
	}
}
