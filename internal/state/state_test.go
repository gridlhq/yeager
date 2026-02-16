package state

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSaveAndLoadVM(t *testing.T) {
	t.Parallel()

	store, err := NewStore(t.TempDir())
	require.NoError(t, err)

	created := time.Date(2026, 1, 15, 10, 30, 0, 0, time.UTC)
	want := VMState{
		InstanceID: "i-0a1b2c3d4e5f6",
		Region:     "us-east-1",
		Created:    created,
		ProjectDir: "/Users/dev/repos/my-app",
	}

	err = store.SaveVM("abc123", want)
	require.NoError(t, err)

	got, err := store.LoadVM("abc123")
	require.NoError(t, err)
	assert.Equal(t, want.InstanceID, got.InstanceID)
	assert.Equal(t, want.Region, got.Region)
	assert.True(t, want.Created.Equal(got.Created))
	assert.Equal(t, want.ProjectDir, got.ProjectDir)
}

func TestLoadVMNotFound(t *testing.T) {
	t.Parallel()

	store, err := NewStore(t.TempDir())
	require.NoError(t, err)

	_, err = store.LoadVM("nonexistent")
	require.Error(t, err)
	assert.True(t, errors.Is(err, os.ErrNotExist) || os.IsNotExist(errors.Unwrap(err)),
		"expected not-exist error, got: %v", err)
}

func TestDeleteVM(t *testing.T) {
	t.Parallel()

	store, err := NewStore(t.TempDir())
	require.NoError(t, err)

	state := VMState{
		InstanceID: "i-test",
		Region:     "us-east-1",
		Created:    time.Now(),
	}

	err = store.SaveVM("todelete", state)
	require.NoError(t, err)

	// Verify it exists.
	_, err = store.LoadVM("todelete")
	require.NoError(t, err)

	// Delete it.
	err = store.DeleteVM("todelete")
	require.NoError(t, err)

	// Verify it's gone.
	_, err = store.LoadVM("todelete")
	require.Error(t, err)
}

func TestDeleteVMIdempotent(t *testing.T) {
	t.Parallel()

	store, err := NewStore(t.TempDir())
	require.NoError(t, err)

	// Deleting a non-existent state should not error.
	err = store.DeleteVM("never-existed")
	require.NoError(t, err)
}

func TestSaveVMOverwrite(t *testing.T) {
	t.Parallel()

	store, err := NewStore(t.TempDir())
	require.NoError(t, err)

	hash := "overwrite-test"

	// Save original.
	err = store.SaveVM(hash, VMState{
		InstanceID: "i-original",
		Region:     "us-east-1",
		Created:    time.Now(),
	})
	require.NoError(t, err)

	// Overwrite with new value.
	err = store.SaveVM(hash, VMState{
		InstanceID: "i-replacement",
		Region:     "eu-west-1",
		Created:    time.Now(),
	})
	require.NoError(t, err)

	// Should get the new value.
	got, err := store.LoadVM(hash)
	require.NoError(t, err)
	assert.Equal(t, "i-replacement", got.InstanceID)
	assert.Equal(t, "eu-west-1", got.Region)
}

func TestNewStoreDefault(t *testing.T) {
	t.Parallel()

	store, err := NewStore("")
	require.NoError(t, err)
	assert.Contains(t, store.BaseDir(), "yeager")
}

func TestSaveAndLoadVMWithSetupHash(t *testing.T) {
	t.Parallel()

	store, err := NewStore(t.TempDir())
	require.NoError(t, err)

	want := VMState{
		InstanceID: "i-setup001",
		Region:     "us-east-1",
		Created:    time.Date(2026, 1, 15, 10, 30, 0, 0, time.UTC),
		ProjectDir: "/home/user/myproject",
		SetupHash:  "abc123def4567890",
	}

	err = store.SaveVM("withhash", want)
	require.NoError(t, err)

	got, err := store.LoadVM("withhash")
	require.NoError(t, err)
	assert.Equal(t, "abc123def4567890", got.SetupHash)
	assert.Equal(t, want.InstanceID, got.InstanceID)
}

func TestLoadVMCorruptFile(t *testing.T) {
	t.Parallel()

	store, err := NewStore(t.TempDir())
	require.NoError(t, err)

	// Save valid state first to create the directory structure.
	err = store.SaveVM("corrupt", VMState{
		InstanceID: "i-test",
		Region:     "us-east-1",
		Created:    time.Now(),
	})
	require.NoError(t, err)

	// Overwrite with invalid JSON.
	corruptFile := filepath.Join(store.BaseDir(), "projects", "corrupt", "vm.json")
	err = os.WriteFile(corruptFile, []byte("not valid json{{{"), 0o644)
	require.NoError(t, err)

	_, err = store.LoadVM("corrupt")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parsing state file")
}

func TestMultipleProjects(t *testing.T) {
	t.Parallel()

	store, err := NewStore(t.TempDir())
	require.NoError(t, err)

	// Two different projects stored independently.
	err = store.SaveVM("project-a", VMState{InstanceID: "i-aaa", Region: "us-east-1", Created: time.Now()})
	require.NoError(t, err)
	err = store.SaveVM("project-b", VMState{InstanceID: "i-bbb", Region: "eu-west-1", Created: time.Now()})
	require.NoError(t, err)

	gotA, err := store.LoadVM("project-a")
	require.NoError(t, err)
	assert.Equal(t, "i-aaa", gotA.InstanceID)

	gotB, err := store.LoadVM("project-b")
	require.NoError(t, err)
	assert.Equal(t, "i-bbb", gotB.InstanceID)
}

func TestSaveAndLoadLastRun(t *testing.T) {
	t.Parallel()

	store, err := NewStore(t.TempDir())
	require.NoError(t, err)

	// Save a last run.
	err = store.SaveLastRun("project-x", "abc12345")
	require.NoError(t, err)

	// Load it back.
	got, err := store.LoadLastRun("project-x")
	require.NoError(t, err)
	assert.Equal(t, "abc12345", got)
}

func TestLoadLastRun_NotExist(t *testing.T) {
	t.Parallel()

	store, err := NewStore(t.TempDir())
	require.NoError(t, err)

	_, err = store.LoadLastRun("nonexistent")
	require.Error(t, err)
	assert.True(t, errors.Is(err, os.ErrNotExist))
}

func TestSaveLastRun_Overwrite(t *testing.T) {
	t.Parallel()

	store, err := NewStore(t.TempDir())
	require.NoError(t, err)

	err = store.SaveLastRun("project-y", "first-run")
	require.NoError(t, err)

	err = store.SaveLastRun("project-y", "second-run")
	require.NoError(t, err)

	got, err := store.LoadLastRun("project-y")
	require.NoError(t, err)
	assert.Equal(t, "second-run", got)
}

func TestSaveAndLoadRunHistory(t *testing.T) {
	t.Parallel()

	store, err := NewStore(t.TempDir())
	require.NoError(t, err)

	entry := RunHistoryEntry{
		RunID:     "abc12345",
		Command:   "cargo test",
		ExitCode:  0,
		StartTime: time.Date(2026, 2, 15, 10, 0, 0, 0, time.UTC),
		Duration:  47 * time.Second,
	}

	err = store.SaveRunHistory("project-h", entry)
	require.NoError(t, err)

	history, err := store.LoadRunHistory("project-h")
	require.NoError(t, err)
	require.Len(t, history, 1)
	assert.Equal(t, "abc12345", history[0].RunID)
	assert.Equal(t, "cargo test", history[0].Command)
	assert.Equal(t, 0, history[0].ExitCode)
	assert.True(t, entry.StartTime.Equal(history[0].StartTime))
	assert.Equal(t, 47*time.Second, history[0].Duration)
}

func TestLoadRunHistory_NotExist(t *testing.T) {
	t.Parallel()

	store, err := NewStore(t.TempDir())
	require.NoError(t, err)

	history, err := store.LoadRunHistory("nonexistent")
	require.NoError(t, err)
	assert.Nil(t, history)
}

func TestSaveRunHistory_Appends(t *testing.T) {
	t.Parallel()

	store, err := NewStore(t.TempDir())
	require.NoError(t, err)

	for i := 0; i < 3; i++ {
		err = store.SaveRunHistory("project-a", RunHistoryEntry{
			RunID:   fmt.Sprintf("run%d", i),
			Command: fmt.Sprintf("cmd%d", i),
		})
		require.NoError(t, err)
	}

	history, err := store.LoadRunHistory("project-a")
	require.NoError(t, err)
	require.Len(t, history, 3)
	assert.Equal(t, "run0", history[0].RunID)
	assert.Equal(t, "run1", history[1].RunID)
	assert.Equal(t, "run2", history[2].RunID)
}

func TestSaveRunHistory_CapsAt20(t *testing.T) {
	t.Parallel()

	store, err := NewStore(t.TempDir())
	require.NoError(t, err)

	// Write 25 entries.
	for i := 0; i < 25; i++ {
		err = store.SaveRunHistory("project-cap", RunHistoryEntry{
			RunID:   fmt.Sprintf("run%02d", i),
			Command: "test",
		})
		require.NoError(t, err)
	}

	history, err := store.LoadRunHistory("project-cap")
	require.NoError(t, err)
	require.Len(t, history, 20)

	// Should keep the 20 most recent (entries 5-24).
	assert.Equal(t, "run05", history[0].RunID)
	assert.Equal(t, "run24", history[19].RunID)
}

func TestSaveRunHistory_CorruptFileStartsFresh(t *testing.T) {
	t.Parallel()

	store, err := NewStore(t.TempDir())
	require.NoError(t, err)

	// Write a valid entry first to create directory.
	err = store.SaveRunHistory("project-c", RunHistoryEntry{RunID: "first"})
	require.NoError(t, err)

	// Corrupt the file.
	corruptFile := filepath.Join(store.BaseDir(), "projects", "project-c", "history.json")
	err = os.WriteFile(corruptFile, []byte("not json{{{"), 0o644)
	require.NoError(t, err)

	// Saving should succeed — starts fresh on corrupt file.
	err = store.SaveRunHistory("project-c", RunHistoryEntry{RunID: "second"})
	require.NoError(t, err)

	history, err := store.LoadRunHistory("project-c")
	require.NoError(t, err)
	require.Len(t, history, 1)
	assert.Equal(t, "second", history[0].RunID)
}

func TestLoadRunHistory_CorruptFile(t *testing.T) {
	t.Parallel()

	store, err := NewStore(t.TempDir())
	require.NoError(t, err)

	// Create directory and write corrupt file.
	err = store.SaveRunHistory("project-lc", RunHistoryEntry{RunID: "ok"})
	require.NoError(t, err)
	corruptFile := filepath.Join(store.BaseDir(), "projects", "project-lc", "history.json")
	err = os.WriteFile(corruptFile, []byte("garbage"), 0o644)
	require.NoError(t, err)

	_, err = store.LoadRunHistory("project-lc")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parsing history file")
}
