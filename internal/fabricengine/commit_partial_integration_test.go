// commit_partial_integration_test.go covers the partial-failure outcomes of Fabric.Commit:
// the warp commit lands and the weft commit fails; the warp commit fails and nothing lands;
// or the weft commit lands and RecordCorrespondence fails, leaving the commit recoverable by RebuildIndex.
// It is in package fabricengine, not fabricengine_test, because it asserts on unexported state.

package fabricengine

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestCommit_PartialFailure_WarpLandsWeftCommitFails covers a landed warp commit with a failed weft commit.
// It forces the weft commit to fail by pre-creating the weft gitdir's index.lock.
// The warp commit must stay.
// The returned error must be a *PartialCommitError naming the warp SHA with weftCommitted=false.
// The durable warp commit must still be pushed.
func TestCommit_PartialFailure_WarpLandsWeftCommitFails(t *testing.T) {
	tiergate.Require(t, 1)
	// Serial: swapPushRecorder sets the package-level spawnDetachedPushFn.
	f, warpPath, weftPath := newCommitFixture(t)
	recorder := swapPushRecorder(t)

	writeWarpFile(t, warpPath, "README", "warp change")
	writeWeftConfigContent(t, weftPath, "weft change")

	gitDir, err := f.weftGitDir()
	if err != nil {
		t.Fatalf("weftGitDir() error = %v", err)
	}
	lockPath := filepath.Join(gitDir, "index.lock")
	if err := os.WriteFile(lockPath, nil, 0o644); err != nil {
		t.Fatalf("WriteFile(index.lock): %v", err)
	}
	t.Cleanup(func() { os.Remove(lockPath) })

	result, err := f.Commit([]string{"README", "_lym/config.yaml"}, "two-sided commit", nil, SyncOptions{})
	if err == nil {
		t.Fatal("Commit() error = nil; want an error (weft commit should fail)")
	}

	var partialErr *PartialCommitError
	if !errors.As(err, &partialErr) {
		t.Fatalf("Commit() error = %v; want *PartialCommitError via errors.As", err)
	}
	if partialErr.weftCommitted {
		t.Errorf("PartialCommitError.weftCommitted = true; want false (the weft commit itself failed)")
	}
	if partialErr.warpSHA != result.WarpSHA {
		t.Errorf("PartialCommitError.warpSHA = %q; want %q", partialErr.warpSHA, result.WarpSHA)
	}

	if !result.WarpCommitted || result.WarpSHA == "" {
		t.Errorf("Commit() = %+v; want the warp commit to have landed", result)
	}
	if result.WeftCommitted {
		t.Errorf("Commit() = %+v; want WeftCommitted=false", result)
	}

	if len(recorder.Calls()) != 1 {
		t.Fatalf("push recorder invocation count = %d; want 1 (the durable warp commit should still be pushed)", len(recorder.Calls()))
	}
}

// TestCommit_PartialFailure_WarpCommitFails covers a failed warp commit.
// It forces the warp commit to fail by pre-creating the warp gitdir's index.lock.
// Nothing must land on weft, an error must be returned, and the push recorder must not be called.
// Fabric.Commit returns before the push step when the warp commit fails.
func TestCommit_PartialFailure_WarpCommitFails(t *testing.T) {
	tiergate.Require(t, 1)
	// Serial: swapPushRecorder sets the package-level spawnDetachedPushFn.
	f, warpPath, weftPath := newCommitFixture(t)
	recorder := swapPushRecorder(t)

	writeWarpFile(t, warpPath, "README", "warp change")
	writeWeftConfigContent(t, weftPath, "weft change")

	preWeftSHA, err := f.records.CurrentSHA()
	if err != nil {
		t.Fatalf("Weft.CurrentSHA() error = %v", err)
	}

	lockPath := filepath.Join(warpPath, ".git", "index.lock")
	if err := os.WriteFile(lockPath, nil, 0o644); err != nil {
		t.Fatalf("WriteFile(index.lock): %v", err)
	}
	t.Cleanup(func() { os.Remove(lockPath) })

	result, err := f.Commit([]string{"README", "_lym/config.yaml"}, "two-sided commit", nil, SyncOptions{})
	if err == nil {
		t.Fatal("Commit() error = nil; want an error (warp commit should fail)")
	}
	if result.WarpCommitted || result.WeftCommitted {
		t.Errorf("Commit() = %+v; want a zero CommitResult (nothing should have landed)", result)
	}

	postWeftSHA, err := f.records.CurrentSHA()
	if err != nil {
		t.Fatalf("Weft.CurrentSHA() error = %v", err)
	}
	if postWeftSHA != preWeftSHA {
		t.Errorf("weft HEAD changed after a failed warp commit: %q -> %q; want unchanged", preWeftSHA, postWeftSHA)
	}

	if len(recorder.Calls()) != 0 {
		t.Errorf("push recorder invocation count = %d; want 0 (Commit returns before the push step on a warp-commit failure)", len(recorder.Calls()))
	}
}

// TestCommit_PartialFailure_CommittedButUnrecorded covers a landed weft commit whose RecordCorrespondence fails.
// It forces that failure by pre-creating a directory at f.corrIndexPath(), so the JSON write fails.
// CommitResult.WeftCommitted must be true with WeftSHA set.
// The error must be a *PartialCommitError with weftCommitted=true, and the push recorder must still be called.
// After clearing the block, an explicit RebuildIndex followed by WeftSHAForWarpSHA must resolve to the landed weft SHA.
// The test does not rely on the self-heal in WeftSHAForWarpSHA.
// A never-recorded warp SHA is an index miss, which returns ErrNoCorrespondence without a rebuild.
// The one-shot rebuild runs only on a hit whose weft SHA no longer exists.
func TestCommit_PartialFailure_CommittedButUnrecorded(t *testing.T) {
	tiergate.Require(t, 1)
	// Serial: swapPushRecorder sets the package-level spawnDetachedPushFn.
	f, warpPath, weftPath := newCommitFixture(t)
	recorder := swapPushRecorder(t)

	writeWarpFile(t, warpPath, "README", "warp change")
	writeWeftConfigContent(t, weftPath, "weft change")

	corrPath, err := f.corrIndexPath()
	if err != nil {
		t.Fatalf("corrIndexPath() error = %v", err)
	}
	if err := os.MkdirAll(corrPath, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q): %v", corrPath, err)
	}

	result, err := f.Commit([]string{"README", "_lym/config.yaml"}, "two-sided commit", nil, SyncOptions{})
	if err == nil {
		t.Fatal("Commit() error = nil; want an error (RecordCorrespondence should fail)")
	}

	var partialErr *PartialCommitError
	if !errors.As(err, &partialErr) {
		t.Fatalf("Commit() error = %v; want *PartialCommitError via errors.As", err)
	}
	if !partialErr.weftCommitted {
		t.Errorf("PartialCommitError.weftCommitted = false; want true (the weft commit landed; only recording failed)")
	}

	if !result.WeftCommitted || result.WeftSHA == "" {
		t.Errorf("Commit() = %+v; want WeftCommitted=true and a populated WeftSHA", result)
	}

	if len(recorder.Calls()) != 1 {
		t.Fatalf("push recorder invocation count = %d; want 1 (the landed commits should still be pushed)", len(recorder.Calls()))
	}

	// Clear the block, rebuild the index from the landed commit's Warp-SHA trailer, and confirm no data was lost.
	if err := os.Remove(corrPath); err != nil {
		t.Fatalf("Remove(%q): %v", corrPath, err)
	}
	if err := f.RebuildIndex(); err != nil {
		t.Fatalf("RebuildIndex() error = %v", err)
	}
	got, err := f.WeftSHAForWarpSHA(result.WarpSHA)
	if err != nil {
		t.Fatalf("WeftSHAForWarpSHA() error = %v", err)
	}
	if got != result.WeftSHA {
		t.Errorf("WeftSHAForWarpSHA(%q) = %q; want %q", result.WarpSHA, got, result.WeftSHA)
	}
}
