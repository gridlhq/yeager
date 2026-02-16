package sync

import (
	"fmt"
	"regexp"
	"strconv"
)

// TranslateRsyncError translates rsync exit codes into user-friendly error messages.
// It wraps the original error with additional context based on the exit code.
func TranslateRsyncError(err error, stderr string) error {
	if err == nil {
		return nil
	}

	code, ok := extractExitCode(err)
	if !ok {
		// Not an exit status error, return as-is
		return fmt.Errorf("%w\n%s", err, stderr)
	}

	msg := rsyncErrorMessage(code)
	return fmt.Errorf("%s: %w\n%s", msg, err, stderr)
}

// rsyncErrorMessage returns a user-friendly message for common rsync exit codes.
func rsyncErrorMessage(exitCode int) string {
	switch exitCode {
	case 23:
		return "rsync: partial transfer (some files could not be synced)"
	case 12:
		return "rsync: protocol error (version mismatch or corruption)"
	case 5:
		return "rsync: I/O error (disk space, permissions, or network issue)"
	case 255:
		return "rsync: SSH connection failed (check network and SSH access)"
	default:
		return "rsync failed"
	}
}

var exitCodeRegex = regexp.MustCompile(`exit status (\d+)`)

// extractExitCode extracts the exit code from an error message.
// Returns (code, true) if found, (0, false) otherwise.
func extractExitCode(err error) (int, bool) {
	if err == nil {
		return 0, false
	}

	msg := err.Error()
	matches := exitCodeRegex.FindStringSubmatch(msg)
	if len(matches) < 2 {
		return 0, false
	}

	code, parseErr := strconv.Atoi(matches[1])
	if parseErr != nil {
		return 0, false
	}

	return code, true
}
