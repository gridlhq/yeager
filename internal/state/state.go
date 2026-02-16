package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	stateDir    = "yeager"
	stateFile   = "vm.json"
	historyFile = "history.json"
	maxHistory  = 20
)

// VMState represents the persisted state for a project's VM.
type VMState struct {
	InstanceID string    `json:"instance_id"`
	Region     string    `json:"region"`
	Created    time.Time `json:"created"`
	ProjectDir string    `json:"project_dir"`
	SetupHash  string    `json:"setup_hash,omitempty"`
}

// Store manages yeager state on the local filesystem.
// State lives at ~/.config/yeager/projects/<project-hash>/.
type Store struct {
	baseDir string
}

// NewStore creates a Store rooted at the given base directory.
// If baseDir is empty, uses the default (~/.config/yeager).
func NewStore(baseDir string) (*Store, error) {
	if baseDir == "" {
		configDir, err := os.UserConfigDir()
		if err != nil {
			return nil, fmt.Errorf("finding config directory: %w", err)
		}
		baseDir = filepath.Join(configDir, stateDir)
	}
	return &Store{baseDir: baseDir}, nil
}

// projectDir returns the directory for a specific project's state.
func (s *Store) projectDir(projectHash string) string {
	return filepath.Join(s.baseDir, "projects", projectHash)
}

// SaveVM persists VM state for a project. Uses atomic write (temp + rename).
func (s *Store) SaveVM(projectHash string, state VMState) error {
	dir := s.projectDir(projectHash)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating state directory: %w", err)
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling state: %w", err)
	}

	target := filepath.Join(dir, stateFile)

	// Atomic write: write to temp file, then rename.
	tmp := target + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("writing temp state file: %w", err)
	}
	if err := os.Rename(tmp, target); err != nil {
		os.Remove(tmp) //nolint:errcheck // best-effort cleanup of temp file
		return fmt.Errorf("renaming state file: %w", err)
	}

	return nil
}

// LoadVM reads VM state for a project. Returns os.ErrNotExist if no state exists.
func (s *Store) LoadVM(projectHash string) (VMState, error) {
	target := filepath.Join(s.projectDir(projectHash), stateFile)

	data, err := os.ReadFile(target)
	if err != nil {
		return VMState{}, fmt.Errorf("reading state file: %w", err)
	}

	var state VMState
	if err := json.Unmarshal(data, &state); err != nil {
		return VMState{}, fmt.Errorf("parsing state file: %w", err)
	}

	return state, nil
}

// DeleteVM removes VM state for a project.
func (s *Store) DeleteVM(projectHash string) error {
	target := filepath.Join(s.projectDir(projectHash), stateFile)
	if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("removing state file: %w", err)
	}
	return nil
}

// SaveLastRun records the most recent run ID for a project.
// Uses atomic write (temp + rename) to avoid corruption from concurrent writers.
func (s *Store) SaveLastRun(projectHash, runID string) error {
	dir := s.projectDir(projectHash)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating state directory: %w", err)
	}
	target := filepath.Join(dir, "last_run")
	tmp := target + ".tmp"
	if err := os.WriteFile(tmp, []byte(runID), 0o644); err != nil {
		return fmt.Errorf("writing temp last_run file: %w", err)
	}
	if err := os.Rename(tmp, target); err != nil {
		os.Remove(tmp) //nolint:errcheck // best-effort cleanup
		return fmt.Errorf("renaming last_run file: %w", err)
	}
	return nil
}

// LoadLastRun returns the most recent run ID for a project.
// Returns os.ErrNotExist if no runs have been recorded.
func (s *Store) LoadLastRun(projectHash string) (string, error) {
	target := filepath.Join(s.projectDir(projectHash), "last_run")
	data, err := os.ReadFile(target)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

// RunHistoryEntry records a completed run.
type RunHistoryEntry struct {
	RunID     string        `json:"run_id"`
	Command   string        `json:"command"`
	ExitCode  int           `json:"exit_code"`
	StartTime time.Time     `json:"start_time"`
	Duration  time.Duration `json:"duration"`
}

// SaveRunHistory appends a run to the project's history, capping at 20 entries.
// Uses atomic write (temp + rename).
func (s *Store) SaveRunHistory(projectHash string, entry RunHistoryEntry) error {
	dir := s.projectDir(projectHash)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating state directory: %w", err)
	}

	history, _ := s.LoadRunHistory(projectHash) // ignore error — start fresh if corrupt/missing
	history = append(history, entry)

	// Cap at maxHistory entries, keeping the most recent.
	if len(history) > maxHistory {
		history = history[len(history)-maxHistory:]
	}

	data, err := json.MarshalIndent(history, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling history: %w", err)
	}

	target := filepath.Join(dir, historyFile)
	tmp := target + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("writing temp history file: %w", err)
	}
	if err := os.Rename(tmp, target); err != nil {
		os.Remove(tmp) //nolint:errcheck // best-effort cleanup
		return fmt.Errorf("renaming history file: %w", err)
	}
	return nil
}

// LoadRunHistory reads the run history for a project.
// Returns nil, nil if no history exists.
func (s *Store) LoadRunHistory(projectHash string) ([]RunHistoryEntry, error) {
	target := filepath.Join(s.projectDir(projectHash), historyFile)
	data, err := os.ReadFile(target)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading history file: %w", err)
	}

	var history []RunHistoryEntry
	if err := json.Unmarshal(data, &history); err != nil {
		return nil, fmt.Errorf("parsing history file: %w", err)
	}
	return history, nil
}

// BaseDir returns the store's base directory.
func (s *Store) BaseDir() string {
	return s.baseDir
}
