package sync

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestTranslateRsyncError tests that rsync exit codes are mapped to user-friendly messages.
func TestTranslateRsyncError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		exitCode   int
		stderr     string
		wantMsg    string
		wantStderr bool // whether stderr should be included
	}{
		{
			name:       "exit code 23 - partial transfer",
			exitCode:   23,
			stderr:     "rsync: some files/attrs were not transferred",
			wantMsg:    "rsync: partial transfer (some files could not be synced)",
			wantStderr: true,
		},
		{
			name:       "exit code 12 - protocol error",
			exitCode:   12,
			stderr:     "rsync: protocol incompatibility",
			wantMsg:    "rsync: protocol error (version mismatch or corruption)",
			wantStderr: true,
		},
		{
			name:       "exit code 5 - I/O error",
			exitCode:   5,
			stderr:     "rsync: read error",
			wantMsg:    "rsync: I/O error (disk space, permissions, or network issue)",
			wantStderr: true,
		},
		{
			name:       "exit code 255 - SSH error",
			exitCode:   255,
			stderr:     "ssh: connect to host failed",
			wantMsg:    "rsync: SSH connection failed (check network and SSH access)",
			wantStderr: true,
		},
		{
			name:       "exit code 1 - generic error",
			exitCode:   1,
			stderr:     "rsync: some error occurred",
			wantMsg:    "rsync failed",
			wantStderr: true,
		},
		{
			name:       "unknown exit code",
			exitCode:   99,
			stderr:     "something went wrong",
			wantMsg:    "rsync failed",
			wantStderr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := fmt.Errorf("exit status %d", tt.exitCode)
			translated := TranslateRsyncError(err, tt.stderr)

			assert.Contains(t, translated.Error(), tt.wantMsg)
			if tt.wantStderr {
				assert.Contains(t, translated.Error(), tt.stderr)
			}
		})
	}
}

// TestTranslateRsyncError_NilError verifies that nil errors pass through.
func TestTranslateRsyncError_NilError(t *testing.T) {
	t.Parallel()
	assert.Nil(t, TranslateRsyncError(nil, ""))
	assert.Nil(t, TranslateRsyncError(nil, "some stderr"))
}

// TestTranslateRsyncError_NonExitStatus verifies errors without exit codes are wrapped with stderr.
func TestTranslateRsyncError_NonExitStatus(t *testing.T) {
	t.Parallel()
	err := fmt.Errorf("connection refused")
	translated := TranslateRsyncError(err, "ssh: connect failed")
	assert.Contains(t, translated.Error(), "connection refused")
	assert.Contains(t, translated.Error(), "ssh: connect failed")
}

// TestExtractExitCode tests extracting exit codes from error messages.
func TestExtractExitCode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		err      error
		wantCode int
		wantOK   bool
	}{
		{
			name:     "exit status error",
			err:      fmt.Errorf("exit status 23"),
			wantCode: 23,
			wantOK:   true,
		},
		{
			name:     "exit status with context",
			err:      fmt.Errorf("command failed: exit status 12"),
			wantCode: 12,
			wantOK:   true,
		},
		{
			name:     "no exit code",
			err:      fmt.Errorf("some other error"),
			wantCode: 0,
			wantOK:   false,
		},
		{
			name:     "nil error",
			err:      nil,
			wantCode: 0,
			wantOK:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			code, ok := extractExitCode(tt.err)
			assert.Equal(t, tt.wantCode, code)
			assert.Equal(t, tt.wantOK, ok)
		})
	}
}

// TestRsyncErrorMessage tests the error message formatting.
func TestRsyncErrorMessage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		exitCode int
		want     string
	}{
		{exitCode: 23, want: "rsync: partial transfer (some files could not be synced)"},
		{exitCode: 12, want: "rsync: protocol error (version mismatch or corruption)"},
		{exitCode: 5, want: "rsync: I/O error (disk space, permissions, or network issue)"},
		{exitCode: 255, want: "rsync: SSH connection failed (check network and SSH access)"},
		{exitCode: 1, want: "rsync failed"},
		{exitCode: 99, want: "rsync failed"},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("code_%d", tt.exitCode), func(t *testing.T) {
			t.Parallel()

			got := rsyncErrorMessage(tt.exitCode)
			assert.Equal(t, tt.want, got)
		})
	}
}
