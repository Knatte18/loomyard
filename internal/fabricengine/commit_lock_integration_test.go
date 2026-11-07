// commit_lock_integration_test.go tests the combined write lock on .weft/weft.write.lock, which Fabric.Commit takes.
// Its tests check that commits contend on that lock and that the lock is released before the push seam fires.
// They also check that the push seam fires on a *PartialCommitError where the warp side landed.
// The file is in package fabricengine because it swaps the unexported spawnDetachedPushFn seam.

package fabricengine

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// weftWriteLockPath returns the path of f's combined commit write lock file.
// It creates the .weft lock directory if it does not exist.
// Every commit on f contends on this file.
func weftWriteLockPath(t *testing.T, f *Fabric) string {
	t.Helper()

	lockDir, err := f.ensureWeftLockDir()
	if err != nil {
		t.Fatalf("ensureWeftLockDir() error = %v", err)
	}
	return filepath.Join(lockDir, weftWriteLockFile)
}

// WeftWriteLockPathForTest returns the path of f's combined commit write lock file.
// It creates the .weft lock directory if needed, so the package's external tests can hold the lock.
func WeftWriteLockPathForTest(t *testing.T, f *Fabric) string {
	return weftWriteLockPath(t, f)
}

// TestCommitLock_WarpOnlySerializesConcurrentCommits asserts that two concurrent warp-only commits both land:
// neither commit fails, and the warp history gains exactly two commits with no merge commit.
// Both calls have no weft-side files, so the test fails against a lock scoped to the weft side only.
func TestCommitLock_WarpOnlySerializesConcurrentCommits(t *testing.T) {
	tiergate.Require(t, 1)
	// Serial: swapPushRecorder sets the package-level spawnDetachedPushFn.
	f, warpPath, _ := newCommitFixture(t)
	swapPushRecorder(t)

	const goroutineCount = 2
	names := [goroutineCount]string{"fileA", "fileB"}

	type outcome struct {
		result CommitResult
		err    error
	}
	outcomes := make(chan outcome, goroutineCount)

	for i := 0; i < goroutineCount; i++ {
		name := names[i]
		go func() {
			// os.WriteFile is used here instead of the t.Fatalf-based writeWarpFile helper:
			// t.Fatal and FailNow may only be called from the goroutine that runs the test.
			// This goroutine is spawned, so it must not call them.
			if err := os.WriteFile(filepath.Join(warpPath, name), []byte("concurrent "+name), 0o644); err != nil {
				outcomes <- outcome{err: err}
				return
			}
			result, err := f.Commit([]string{name}, "concurrent warp commit "+name, nil, SyncOptions{})
			outcomes <- outcome{result: result, err: err}
		}()
	}

	landedCount := 0
	for i := 0; i < goroutineCount; i++ {
		got := <-outcomes
		if got.err != nil {
			t.Fatalf("Commit() error = %v", got.err)
		}
		if got.result.WarpCommitted {
			landedCount++
		}
	}
	if landedCount != goroutineCount {
		t.Errorf("landed commit count = %d; want %d (both concurrent warp-only commits should land)", landedCount, goroutineCount)
	}

	totalCommits := gitkit.RevListCount(t, warpPath, "HEAD")
	if totalCommits != 3 {
		t.Errorf("warp commit count = %d; want 3 (the fixture's initial commit plus the two concurrent ones)", totalCommits)
	}
	mergeCommits := gitkit.RevListCount(t, warpPath, "--merges", "HEAD")
	if mergeCommits != 0 {
		t.Errorf("warp merge commit count = %d; want 0 (history should be linear)", mergeCommits)
	}
}

// TestCommitLock_ContendsAcrossSides asserts that Fabric.Commit calls contend on the combined write lock file:
// a warp-only, a weft-only, and a two-sided call each block while another holder has the lock.
func TestCommitLock_ContendsAcrossSides(t *testing.T) {
	tiergate.Require(t, 1)
	// Serial: swapPushRecorder sets the package-level spawnDetachedPushFn.
	t.Run("WarpOnly", func(t *testing.T) {
		f, warpPath, _ := newCommitFixture(t)
		swapPushRecorder(t)
		writeWarpFile(t, warpPath, "README", "warp-only under external lock")

		assertCommitBlocksOnHeldLock(t, f, []string{"README"}, "warp-only under external lock")
	})

	t.Run("WeftOnly", func(t *testing.T) {
		f, _, weftPath := newCommitFixture(t)
		swapPushRecorder(t)
		writeWeftConfigContent(t, weftPath, "weft-only under external lock")

		assertCommitBlocksOnHeldLock(t, f, []string{"_lym/config.yaml"}, "weft-only under external lock")
	})

	t.Run("TwoSided", func(t *testing.T) {
		f, warpPath, weftPath := newCommitFixture(t)
		swapPushRecorder(t)
		writeWarpFile(t, warpPath, "README", "two-sided under external lock")
		writeWeftConfigContent(t, weftPath, "two-sided under external lock weft")

		assertCommitBlocksOnHeldLock(t, f, []string{"README", "_lym/config.yaml"}, "two-sided under external lock")
	})
}

// assertCommitBlocksOnHeldLock acquires f's combined write lock externally and starts f.Commit in a goroutine.
// It asserts that the call has not completed after a short wait, while the lock is still held.
// It then releases the lock and asserts that the call completes within a generous timeout.
// That sequence shows the call was blocked on the lock, not merely slower than its holder.
func assertCommitBlocksOnHeldLock(t *testing.T, f *Fabric, files []string, msg string) {
	t.Helper()

	lockPath := weftWriteLockPath(t, f)
	externalLock, err := lock.AcquireWriteLock(lockPath)
	if err != nil {
		t.Fatalf("AcquireWriteLock(%q) error = %v", lockPath, err)
	}

	type outcome struct {
		result CommitResult
		err    error
	}
	done := make(chan outcome, 1)
	go func() {
		result, err := f.Commit(files, msg, nil, SyncOptions{})
		done <- outcome{result: result, err: err}
	}()

	select {
	case <-done:
		_ = externalLock.Release()
		t.Fatal("Commit() completed while the combined write lock was externally held; want it to block")
	case <-time.After(150 * time.Millisecond):
		// Still blocked, as expected: fall through to release the external lock.
	}

	if err := externalLock.Release(); err != nil {
		t.Fatalf("Release() error = %v", err)
	}

	select {
	case got := <-done:
		if got.err != nil {
			t.Fatalf("Commit() error = %v", got.err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Commit() did not complete after the external lock was released")
	}
}

// TestCommitLock_ReleasedBeforePush asserts that the combined write lock is not held when spawnDetachedPushFn fires.
// Commit releases its own hold before it calls the seam.
// The swapped seam tries a non-blocking TryAcquireWriteLock on the same lock path and must find it free.
func TestCommitLock_ReleasedBeforePush(t *testing.T) {
	tiergate.Require(t, 1)
	// Serial: the test assigns the package-level spawnDetachedPushFn.
	f, warpPath, weftPath := newCommitFixture(t)
	lockPath := weftWriteLockPath(t, f)

	var lockWasFreeDuringPush bool
	original := spawnDetachedPushFn
	spawnDetachedPushFn = func(PushSpawn) error {
		probe, acquired, err := lock.TryAcquireWriteLock(lockPath)
		if err != nil {
			t.Errorf("TryAcquireWriteLock(%q) error = %v", lockPath, err)
			return nil
		}
		lockWasFreeDuringPush = acquired
		if acquired {
			_ = probe.Release()
		}
		return nil
	}
	t.Cleanup(func() { spawnDetachedPushFn = original })

	writeWarpFile(t, warpPath, "README", "release before push")
	writeWeftConfigContent(t, weftPath, "release before push weft")

	if _, err := f.Commit([]string{"README", "_lym/config.yaml"}, "release before push commit", nil, SyncOptions{}); err != nil {
		t.Fatalf("Commit() error = %v", err)
	}

	if !lockWasFreeDuringPush {
		t.Error("combined write lock was still held when spawnDetachedPushFn fired; want it released beforehand")
	}
}

// TestCommitLock_PushFiresOnPartialFailure asserts that Commit fires the push seam on a partial failure:
// the warp side landed, the weft commit failed, and the returned error is a *PartialCommitError.
// The push gate is WarpCommitted || WeftCommitted.
// The test forces the weft commit to fail by pre-creating the weft gitdir's index.lock.
func TestCommitLock_PushFiresOnPartialFailure(t *testing.T) {
	tiergate.Require(t, 1)
	// Serial: swapPushRecorder sets the package-level spawnDetachedPushFn.
	f, warpPath, weftPath := newCommitFixture(t)
	recorder := swapPushRecorder(t)

	writeWarpFile(t, warpPath, "README", "partial failure warp change")
	writeWeftConfigContent(t, weftPath, "partial failure weft change")

	gitDir, err := f.weftGitDir()
	if err != nil {
		t.Fatalf("weftGitDir() error = %v", err)
	}
	indexLockPath := filepath.Join(gitDir, "index.lock")
	if err := os.WriteFile(indexLockPath, nil, 0o644); err != nil {
		t.Fatalf("WriteFile(index.lock): %v", err)
	}
	t.Cleanup(func() { os.Remove(indexLockPath) })

	result, err := f.Commit([]string{"README", "_lym/config.yaml"}, "partial failure commit", nil, SyncOptions{})
	if err == nil {
		t.Fatal("Commit() error = nil; want an error (the weft commit should fail)")
	}
	var partialErr *PartialCommitError
	if !errors.As(err, &partialErr) {
		t.Fatalf("Commit() error = %v; want *PartialCommitError via errors.As", err)
	}
	if !result.WarpCommitted {
		t.Errorf("Commit() = %+v; want the warp commit to have landed", result)
	}

	if len(recorder.Calls()) != 1 {
		t.Fatalf("push recorder invocation count = %d; want 1 (the WarpCommitted||WeftCommitted gate should still fire the push seam)", len(recorder.Calls()))
	}
}
