package state

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestIdleStartTracking(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "yeager-state-idle-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	st, err := NewStore(tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	projectHash := "test-project-123"

	t.Run("SaveAndLoad", func(t *testing.T) {
		now := time.Now().UTC()

		// Save idle start time.
		if err := st.SaveIdleStart(projectHash, now); err != nil {
			t.Fatalf("SaveIdleStart failed: %v", err)
		}

		// Load and verify.
		loaded, err := st.LoadIdleStart(projectHash)
		if err != nil {
			t.Fatalf("LoadIdleStart failed: %v", err)
		}

		// Compare timestamps (allow small precision difference).
		diff := loaded.Sub(now).Abs()
		if diff > time.Millisecond {
			t.Errorf("loaded time differs: got %v, want %v (diff: %v)", loaded, now, diff)
		}
	})

	t.Run("LoadNonExistent", func(t *testing.T) {
		_, err := st.LoadIdleStart("nonexistent-project")
		if !os.IsNotExist(err) {
			t.Errorf("expected os.ErrNotExist, got: %v", err)
		}
	})

	t.Run("Clear", func(t *testing.T) {
		now := time.Now().UTC()

		// Save idle start time.
		if err := st.SaveIdleStart(projectHash, now); err != nil {
			t.Fatalf("SaveIdleStart failed: %v", err)
		}

		// Verify it exists.
		if _, err := st.LoadIdleStart(projectHash); err != nil {
			t.Fatalf("LoadIdleStart failed before clear: %v", err)
		}

		// Clear it.
		if err := st.ClearIdleStart(projectHash); err != nil {
			t.Fatalf("ClearIdleStart failed: %v", err)
		}

		// Verify it's gone.
		_, err := st.LoadIdleStart(projectHash)
		if !os.IsNotExist(err) {
			t.Errorf("expected os.ErrNotExist after clear, got: %v", err)
		}
	})

	t.Run("ClearNonExistent", func(t *testing.T) {
		// Clearing non-existent idle start should not error.
		if err := st.ClearIdleStart("nonexistent-project"); err != nil {
			t.Errorf("ClearIdleStart on nonexistent project should not error, got: %v", err)
		}
	})

	t.Run("AtomicWrite", func(t *testing.T) {
		now := time.Now().UTC()

		// Save idle start time.
		if err := st.SaveIdleStart(projectHash, now); err != nil {
			t.Fatalf("SaveIdleStart failed: %v", err)
		}

		// Verify no temp files left behind.
		dir := st.projectDir(projectHash)
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatalf("ReadDir failed: %v", err)
		}

		for _, entry := range entries {
			if filepath.Ext(entry.Name()) == ".tmp" {
				t.Errorf("found temp file left behind: %s", entry.Name())
			}
		}
	})

	t.Run("Precision", func(t *testing.T) {
		// Test that we preserve nanosecond precision.
		now := time.Now().UTC()

		if err := st.SaveIdleStart(projectHash, now); err != nil {
			t.Fatalf("SaveIdleStart failed: %v", err)
		}

		loaded, err := st.LoadIdleStart(projectHash)
		if err != nil {
			t.Fatalf("LoadIdleStart failed: %v", err)
		}

		// RFC3339Nano should preserve nanoseconds.
		if !loaded.Equal(now) {
			t.Errorf("time precision lost: got %v, want %v", loaded, now)
		}
	})
}
