package monitor

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"

	"github.com/gridlhq/yeager/internal/state"
)

const lockFileName = "monitor.lock"

// Lock represents an exclusive file lock for monitor coordination.
type Lock struct {
	file *os.File
	path string
}

// AcquireLock attempts to acquire an exclusive lock for the monitor.
// Returns a Lock that must be released with Release().
// If another process holds the lock, returns nil without error.
func AcquireLock(st *state.Store, projectHash string) (*Lock, error) {
	lockPath := filepath.Join(st.BaseDir(), "projects", projectHash, lockFileName)
	dir := filepath.Dir(lockPath)

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("creating lock directory: %w", err)
	}

	// Open or create lock file.
	file, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, fmt.Errorf("opening lock file: %w", err)
	}

	// Try to acquire exclusive lock (non-blocking).
	err = syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if err != nil {
		file.Close()
		if err == syscall.EWOULDBLOCK {
			// Another process holds the lock - this is normal.
			return nil, nil
		}
		return nil, fmt.Errorf("acquiring lock: %w", err)
	}

	return &Lock{
		file: file,
		path: lockPath,
	}, nil
}

// Release releases the lock and cleans up the lock file.
func (l *Lock) Release() error {
	if l == nil || l.file == nil {
		return nil
	}

	// Unlock.
	_ = syscall.Flock(int(l.file.Fd()), syscall.LOCK_UN)

	// Close file.
	if err := l.file.Close(); err != nil {
		return fmt.Errorf("closing lock file: %w", err)
	}

	// Clean up lock file (best-effort).
	_ = os.Remove(l.path)

	return nil
}
