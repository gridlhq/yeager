package monitor

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gridlhq/yeager/internal/state"
)

// WritePIDFile writes the monitor's PID to a file in the state directory.
// Uses atomic write (temp + rename) to avoid corruption.
func WritePIDFile(st *state.Store, projectHash string, pid int) error {
	target := pidFilePath(st, projectHash)
	dir := filepath.Dir(target)

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating state directory: %w", err)
	}

	tmp := target + ".tmp"
	if err := os.WriteFile(tmp, []byte(fmt.Sprintf("%d\n", pid)), 0o644); err != nil {
		return fmt.Errorf("writing temp PID file: %w", err)
	}

	if err := os.Rename(tmp, target); err != nil {
		_ = os.Remove(tmp) // best-effort cleanup
		return fmt.Errorf("renaming PID file: %w", err)
	}

	return nil
}

// LoadPIDFile reads the monitor's PID from the state directory.
// Returns os.ErrNotExist if no PID file exists.
func LoadPIDFile(st *state.Store, projectHash string) (int, error) {
	target := pidFilePath(st, projectHash)
	data, err := os.ReadFile(target)
	if err != nil {
		return 0, err
	}

	pidStr := strings.TrimSpace(string(data))
	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		return 0, fmt.Errorf("parsing PID: %w", err)
	}

	return pid, nil
}

// RemovePIDFile removes the monitor's PID file.
func RemovePIDFile(st *state.Store, projectHash string) error {
	target := pidFilePath(st, projectHash)
	if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("removing PID file: %w", err)
	}
	return nil
}

// pidFilePath returns the path to the PID file for a project.
func pidFilePath(st *state.Store, projectHash string) string {
	return filepath.Join(st.BaseDir(), "projects", projectHash, pidFileName)
}
