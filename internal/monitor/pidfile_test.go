package monitor

import (
	"os"
	"testing"

	"github.com/gridlhq/yeager/internal/state"
)

func TestPIDFile(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "yeager-monitor-pid-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	st, err := state.NewStore(tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	projectHash := "test-project-123"

	t.Run("WriteAndLoad", func(t *testing.T) {
		pid := 12345

		// Write PID file.
		if err := WritePIDFile(st, projectHash, pid); err != nil {
			t.Fatalf("WritePIDFile failed: %v", err)
		}

		// Load and verify.
		loaded, err := LoadPIDFile(st, projectHash)
		if err != nil {
			t.Fatalf("LoadPIDFile failed: %v", err)
		}

		if loaded != pid {
			t.Errorf("loaded PID = %d, want %d", loaded, pid)
		}
	})

	t.Run("LoadNonExistent", func(t *testing.T) {
		_, err := LoadPIDFile(st, "nonexistent-project")
		if !os.IsNotExist(err) {
			t.Errorf("expected os.ErrNotExist, got: %v", err)
		}
	})

	t.Run("Remove", func(t *testing.T) {
		pid := 67890

		// Write PID file.
		if err := WritePIDFile(st, projectHash, pid); err != nil {
			t.Fatalf("WritePIDFile failed: %v", err)
		}

		// Verify it exists.
		if _, err := LoadPIDFile(st, projectHash); err != nil {
			t.Fatalf("LoadPIDFile failed before remove: %v", err)
		}

		// Remove it.
		if err := RemovePIDFile(st, projectHash); err != nil {
			t.Fatalf("RemovePIDFile failed: %v", err)
		}

		// Verify it's gone.
		_, err := LoadPIDFile(st, projectHash)
		if !os.IsNotExist(err) {
			t.Errorf("expected os.ErrNotExist after remove, got: %v", err)
		}
	})

	t.Run("RemoveNonExistent", func(t *testing.T) {
		// Removing non-existent PID file should not error.
		if err := RemovePIDFile(st, "nonexistent-project"); err != nil {
			t.Errorf("RemovePIDFile on nonexistent project should not error, got: %v", err)
		}
	})

	t.Run("Overwrite", func(t *testing.T) {
		pid1 := 11111
		pid2 := 22222

		// Write first PID.
		if err := WritePIDFile(st, projectHash, pid1); err != nil {
			t.Fatalf("WritePIDFile (first) failed: %v", err)
		}

		// Write second PID (overwrite).
		if err := WritePIDFile(st, projectHash, pid2); err != nil {
			t.Fatalf("WritePIDFile (second) failed: %v", err)
		}

		// Load and verify second PID.
		loaded, err := LoadPIDFile(st, projectHash)
		if err != nil {
			t.Fatalf("LoadPIDFile failed: %v", err)
		}

		if loaded != pid2 {
			t.Errorf("loaded PID = %d, want %d", loaded, pid2)
		}
	})

	t.Run("InvalidPID", func(t *testing.T) {
		// Write invalid PID manually.
		path := pidFilePath(st, projectHash)
		if err := os.MkdirAll(st.BaseDir()+"/projects/"+projectHash, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("not-a-number\n"), 0o644); err != nil {
			t.Fatal(err)
		}

		// Loading should fail with parse error.
		_, err := LoadPIDFile(st, projectHash)
		if err == nil {
			t.Error("expected error loading invalid PID, got nil")
		}
	})
}

func TestIsProcessRunning(t *testing.T) {
	t.Run("CurrentProcess", func(t *testing.T) {
		// Current process should be running.
		if !IsProcessRunning(os.Getpid()) {
			t.Error("current process should be detected as running")
		}
	})

	t.Run("NonExistentProcess", func(t *testing.T) {
		// PID 99999 is unlikely to exist.
		if IsProcessRunning(99999) {
			t.Error("nonexistent process should not be detected as running")
		}
	})

	t.Run("InitProcess", func(t *testing.T) {
		// PID 1 (init/launchd) should be running.
		// On some systems (macOS), we may not have permission to signal PID 1,
		// so just verify the function doesn't panic.
		_ = IsProcessRunning(1)
	})
}
