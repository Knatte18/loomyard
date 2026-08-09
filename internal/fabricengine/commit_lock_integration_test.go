//go:build integration

// commit_lock_integration_test.go — integration coverage for the
// combined-commit-lock and commit-lock-scoped-to-commit-only Shared
// Decisions: two concurrent warp-only Fabric.Commit calls serialize on
// .weft/weft.write.lock rather than racing unlocked (the gap this batch
// closes), a warp-only/weft-only/two-sided Commit call all contend on that
// same lock file, the lock is released before spawnDetachedPushFn fires, and
// the push seam still fires on a *PartialCommitError where the warp side
// landed. Package fabricengine (internal) because it swaps the unexported
// spawnDetachedPushFn seam and reaches the unexported weftWriteLockFile
// constant and ensureWeftLockDir/weftGitDir methods. Reuses
// commit_integration_test.go's fixture and recorder helpers
// (newCommitFixture, swapPushRecorder, writeWarpFile) and
// syncweft_integration_test.go's writeWeftConfigContent — no
// t.Parallel() anywhere in this file, per the push-invocation-seam-for-tests
// discipline those helpers already follow.

package fabricengine

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/lock"
)

// gitCommitCount runs `git rev-list --count <args...>` in dir and parses the
// result, failing the test on any git or parse failure. args typically
// supplies the flags/revision list to count (e.g. "HEAD" or "--merges",
// "HEAD").
func gitCommitCount(t *testing.T, dir string, args ...string) int {
	t.Helper()

	cmd := exec.Command("git", append([]string{"rev-list", "--count"}, args...)...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git rev-list --count %v in %s: %v", args, dir, err)
	}
	count, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil {
		t.Fatalf("parse git rev-list --count output %q: %v", string(out), err)
	}
	return count
}

// weftWriteLockPath returns the path of f's combined commit write lock file,
// creating the .weft lock directory (idempotently) if it does not already
// exist — the same path every committing Fabric.Commit call contends on.
func weftWriteLockPath(t *testing.T, f *Fabric) string {
	t.Helper()

	lockDir, err := f.ensureWeftLockDir()
	if err != nil {
		t.Fatalf("ensureWeftLockDir() error = %v", err)
	}
	return filepath.Join(lockDir, weftWriteLockFile)
}

// TestCommitLock_WarpOnlySerializesConcurrentCommits asserts that two concurrent warp-only
// Fabric.Commit calls (weft-empty inputs) against one warp+weft pair serialize on the combined
// write lock: both commits land, neither corrupts the other's index (no git failure from a stray
// .git/index.lock collision), and the resulting warp history is linear (no merge commit, exactly
// two new commits on top of the fixture's initial one).
// This is the scenario that fails against a lock scoped to the weft side only, since both calls
// here have zero weft-side files.
func TestCommitLock_WarpOnlySerializesConcurrentCommits(t *testing.T) {
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
			// os.WriteFile (not the t.Fatalf-based writeWarpFile helper) is used
			// here deliberately: t.Fatal/FailNow may only be called from the
			// goroutine running the test itself, never from a spawned goroutine
			// like this one.
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

	totalCommits := gitCommitCount(t, warpPath, "HEAD")
	if totalCommits != 3 {
		t.Errorf("warp commit count = %d; want 3 (the fixture's initial commit plus the two concurrent ones)", totalCommits)
	}
	mergeCommits := gitCommitCount(t, warpPath, "--merges", "HEAD")
	if mergeCommits != 0 {
		t.Errorf("warp merge commit count = %d; want 0 (history should be linear)", mergeCommits)
	}
}

// TestCommitLock_ContendsAcrossSides asserts that a warp-only, a weft-only, and a two-sided
// Fabric.Commit call all contend on the same combined write lock file: each subtest externally
// acquires the lock first, starts the Commit call, confirms it is still blocked while the external
// lock is held, releases the external lock, and confirms the call then completes.
func TestCommitLock_ContendsAcrossSides(t *testing.T) {
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

		assertCommitBlocksOnHeldLock(t, f, []string{"_lyx/config.yaml"}, "weft-only under external lock")
	})

	t.Run("TwoSided", func(t *testing.T) {
		f, warpPath, weftPath := newCommitFixture(t)
		swapPushRecorder(t)
		writeWarpFile(t, warpPath, "README", "two-sided under external lock")
		writeWeftConfigContent(t, weftPath, "two-sided under external lock weft")

		assertCommitBlocksOnHeldLock(t, f, []string{"README", "_lyx/config.yaml"}, "two-sided under external lock")
	})
}

// assertCommitBlocksOnHeldLock externally acquires f's combined write lock,
// starts f.Commit(files, msg, nil, SyncOptions{}) in a goroutine, asserts it
// has not completed after a short wait (the lock is still held), releases
// the external lock, and then asserts the call completes successfully within
// a generous timeout — proving the Commit call was genuinely blocked on, not
// merely slower than, the external lock holder.
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

// TestCommitLock_ReleasedBeforePush asserts that the combined write lock is NOT held at the moment
// spawnDetachedPushFn fires: the swapped seam attempts a non-blocking TryAcquireWriteLock on the
// same lock path Fabric.Commit uses, which must succeed because Commit has already released its own
// hold by the time it calls the seam (the commit-lock-scoped-to-commit-only Shared Decision).
func TestCommitLock_ReleasedBeforePush(t *testing.T) {
	f, warpPath, weftPath := newCommitFixture(t)
	lockPath := weftWriteLockPath(t, f)

	var lockWasFreeDuringPush bool
	original := spawnDetachedPushFn
	spawnDetachedPushFn = func(gotWarpPath, gotWeftPath string) error {
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

	if _, err := f.Commit([]string{"README", "_lyx/config.yaml"}, "release before push commit", nil, SyncOptions{}); err != nil {
		t.Fatalf("Commit() error = %v", err)
	}

	if !lockWasFreeDuringPush {
		t.Error("combined write lock was still held when spawnDetachedPushFn fired; want it released beforehand")
	}
}

// TestCommitLock_PushFiresOnPartialFailure asserts that Commit's WarpCommitted || WeftCommitted
// push gate still fires the push seam on a *PartialCommitError where the warp side landed but the
// weft commit itself failed — reusing commit_partial_integration_test.go's failure-injection
// approach (pre-creating the weft gitdir's index.lock) to force that outcome.
func TestCommitLock_PushFiresOnPartialFailure(t *testing.T) {
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

	result, err := f.Commit([]string{"README", "_lyx/config.yaml"}, "partial failure commit", nil, SyncOptions{})
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
