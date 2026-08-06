//go:build integration

// commit_partial_integration_test.go — integration coverage for
// Fabric.Commit's three partial-failure outcomes (see the
// partial-failure-report-three-outcomes Shared Decision): a landed warp
// commit with a failed weft commit, a failed warp commit (nothing lands),
// and a landed weft commit whose RecordCorrespondence call fails
// (committed-but-unrecorded, recoverable via RebuildIndex). Package
// fabricengine (internal) because it asserts on f.weftGitDir and
// f.corrIndexPath, both unexported. Reuses commit_integration_test.go's
// fixture and recorder helpers (newCommitFixture, swapPushRecorder,
// writeWarpFile) and syncweft_integration_test.go's writeWeftConfigContent.

package fabricengine

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// TestCommit_PartialFailure_WarpLandsWeftCommitFails covers outcome (1): the warp commit lands,
// but the weft commit itself fails (forced here by pre-creating the weft gitdir's index.lock, the same way a `git reset --hard` can be forced to fail).
// The warp commit must stay, the returned error must be a *PartialCommitError naming the warp SHA with WeftCommitted=false, and the durable warp commit must still be pushed.
func TestCommit_PartialFailure_WarpLandsWeftCommitFails(t *testing.T) {
	f, warpPath, weftPath := newCommitFixture(t)
	calls := swapPushRecorder(t)

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

	result, err := f.Commit([]string{"README", "_lyx/config.yaml"}, "two-sided commit", nil, SyncOptions{})
	if err == nil {
		t.Fatal("Commit() error = nil; want an error (weft commit should fail)")
	}

	var partialErr *PartialCommitError
	if !errors.As(err, &partialErr) {
		t.Fatalf("Commit() error = %v; want *PartialCommitError via errors.As", err)
	}
	if partialErr.WeftCommitted {
		t.Errorf("PartialCommitError.WeftCommitted = true; want false (the weft commit itself failed)")
	}
	if partialErr.WarpSHA != result.WarpSHA {
		t.Errorf("PartialCommitError.WarpSHA = %q; want %q", partialErr.WarpSHA, result.WarpSHA)
	}

	if !result.WarpCommitted || result.WarpSHA == "" {
		t.Errorf("Commit() = %+v; want the warp commit to have landed", result)
	}
	if result.WeftCommitted {
		t.Errorf("Commit() = %+v; want WeftCommitted=false", result)
	}

	if len(*calls) != 1 {
		t.Fatalf("push recorder invocation count = %d; want 1 (the durable warp commit should still be pushed)", len(*calls))
	}
}

// TestCommit_PartialFailure_WarpCommitFails covers outcome (2): the warp commit itself fails (forced here by pre-creating the warp gitdir's index.lock).
// Nothing must land on weft, an error must be returned, and the push recorder must not be called — Fabric.Commit returns before the push step on a warp-commit failure.
func TestCommit_PartialFailure_WarpCommitFails(t *testing.T) {
	f, warpPath, weftPath := newCommitFixture(t)
	calls := swapPushRecorder(t)

	writeWarpFile(t, warpPath, "README", "warp change")
	writeWeftConfigContent(t, weftPath, "weft change")

	preWeftSHA, err := f.Weft.CurrentSHA()
	if err != nil {
		t.Fatalf("Weft.CurrentSHA() error = %v", err)
	}

	lockPath := filepath.Join(warpPath, ".git", "index.lock")
	if err := os.WriteFile(lockPath, nil, 0o644); err != nil {
		t.Fatalf("WriteFile(index.lock): %v", err)
	}
	t.Cleanup(func() { os.Remove(lockPath) })

	result, err := f.Commit([]string{"README", "_lyx/config.yaml"}, "two-sided commit", nil, SyncOptions{})
	if err == nil {
		t.Fatal("Commit() error = nil; want an error (warp commit should fail)")
	}
	if result.WarpCommitted || result.WeftCommitted {
		t.Errorf("Commit() = %+v; want a zero CommitResult (nothing should have landed)", result)
	}

	postWeftSHA, err := f.Weft.CurrentSHA()
	if err != nil {
		t.Fatalf("Weft.CurrentSHA() error = %v", err)
	}
	if postWeftSHA != preWeftSHA {
		t.Errorf("weft HEAD changed after a failed warp commit: %q -> %q; want unchanged", preWeftSHA, postWeftSHA)
	}

	if len(*calls) != 0 {
		t.Errorf("push recorder invocation count = %d; want 0 (Commit returns before the push step on a warp-commit failure)", len(*calls))
	}
}

// TestCommit_PartialFailure_CommittedButUnrecorded covers outcome (3): the weft commit lands,
// but RecordCorrespondence fails to persist an index entry (forced here by pre-creating a directory at f.corrIndexPath() so its JSON write fails).
// CommitResult.WeftCommitted must be true with WeftSHA set, the error must be a *PartialCommitError with WeftCommitted=true, the push recorder must still be called, and — after clearing the block — an explicit RebuildIndex followed by WeftSHAForWarpSHA must resolve to the landed weft SHA (no data lost).
// This does NOT rely on WeftSHAForWarpSHA's own self-heal: a never-recorded entry is an index MISS (ErrNoCorrespondence), whose one-shot rebuild only fires on a stale HIT — see WeftSHAForWarpSHA's doc comment.
func TestCommit_PartialFailure_CommittedButUnrecorded(t *testing.T) {
	f, warpPath, weftPath := newCommitFixture(t)
	calls := swapPushRecorder(t)

	writeWarpFile(t, warpPath, "README", "warp change")
	writeWeftConfigContent(t, weftPath, "weft change")

	corrPath, err := f.corrIndexPath()
	if err != nil {
		t.Fatalf("corrIndexPath() error = %v", err)
	}
	if err := os.MkdirAll(corrPath, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q): %v", corrPath, err)
	}

	result, err := f.Commit([]string{"README", "_lyx/config.yaml"}, "two-sided commit", nil, SyncOptions{})
	if err == nil {
		t.Fatal("Commit() error = nil; want an error (RecordCorrespondence should fail)")
	}

	var partialErr *PartialCommitError
	if !errors.As(err, &partialErr) {
		t.Fatalf("Commit() error = %v; want *PartialCommitError via errors.As", err)
	}
	if !partialErr.WeftCommitted {
		t.Errorf("PartialCommitError.WeftCommitted = false; want true (the weft commit landed; only recording failed)")
	}

	if !result.WeftCommitted || result.WeftSHA == "" {
		t.Errorf("Commit() = %+v; want WeftCommitted=true and a populated WeftSHA", result)
	}

	if len(*calls) != 1 {
		t.Fatalf("push recorder invocation count = %d; want 1 (the landed commits should still be pushed)", len(*calls))
	}

	// Clear the block, rebuild the index from the landed commit's own
	// Warp-SHA trailer, and confirm no data was lost.
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
