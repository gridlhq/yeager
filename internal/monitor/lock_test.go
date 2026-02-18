package monitor

import (
	"os"
	"sync"
	"testing"
	"time"

	"github.com/gridlhq/yeager/internal/state"
)

func TestLockAcquisition(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "yeager-monitor-lock-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	st, err := state.NewStore(tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	projectHash := "test-project-123"

	t.Run("AcquireAndRelease", func(t *testing.T) {
		lock, err := AcquireLock(st, projectHash)
		if err != nil {
			t.Fatalf("AcquireLock failed: %v", err)
		}
		if lock == nil {
			t.Fatal("expected lock, got nil")
		}

		// Release the lock.
		if err := lock.Release(); err != nil {
			t.Errorf("Release failed: %v", err)
		}
	})

	t.Run("ExclusiveLock", func(t *testing.T) {
		// Acquire first lock.
		lock1, err := AcquireLock(st, projectHash)
		if err != nil {
			t.Fatalf("first AcquireLock failed: %v", err)
		}
		if lock1 == nil {
			t.Fatal("expected first lock, got nil")
		}
		defer lock1.Release()

		// Try to acquire second lock - should return nil (lock held).
		lock2, err := AcquireLock(st, projectHash)
		if err != nil {
			t.Fatalf("second AcquireLock returned error: %v", err)
		}
		if lock2 != nil {
			t.Error("second lock should be nil (already held)")
			lock2.Release()
		}
	})

	t.Run("ReleaseAllowsReacquisition", func(t *testing.T) {
		// Acquire lock.
		lock1, err := AcquireLock(st, projectHash)
		if err != nil {
			t.Fatalf("first AcquireLock failed: %v", err)
		}
		if lock1 == nil {
			t.Fatal("expected first lock, got nil")
		}

		// Release it.
		if err := lock1.Release(); err != nil {
			t.Fatalf("Release failed: %v", err)
		}

		// Should be able to acquire again.
		lock2, err := AcquireLock(st, projectHash)
		if err != nil {
			t.Fatalf("second AcquireLock failed: %v", err)
		}
		if lock2 == nil {
			t.Error("expected second lock after release, got nil")
		}
		defer lock2.Release()
	})

	t.Run("ConcurrentAcquisition", func(t *testing.T) {
		const goroutines = 10
		var wg sync.WaitGroup
		acquiredCount := 0
		var mu sync.Mutex

		wg.Add(goroutines)
		for i := 0; i < goroutines; i++ {
			go func() {
				defer wg.Done()
				lock, err := AcquireLock(st, projectHash)
				if err != nil {
					t.Errorf("AcquireLock failed: %v", err)
					return
				}
				if lock != nil {
					mu.Lock()
					acquiredCount++
					mu.Unlock()
					time.Sleep(10 * time.Millisecond)
					lock.Release()
				}
			}()
		}

		wg.Wait()

		// Only one goroutine should have acquired the lock at a time.
		// But with releases, multiple goroutines may have acquired it sequentially.
		if acquiredCount == 0 {
			t.Error("no goroutine acquired lock")
		}
		// At least one should have been blocked initially.
		if acquiredCount == goroutines {
			t.Logf("warning: all %d goroutines acquired lock (may indicate no contention)", goroutines)
		}
	})
}
