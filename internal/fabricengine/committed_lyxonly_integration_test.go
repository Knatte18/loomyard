//go:build integration

// committed_lyxonly_integration_test.go pins the value-identity argument from the discussion's
// commit-result-committed-method decision: a Fabric.Commit call whose file list is
// ScopedPathspec(AnchorRel, ["_lyx"])-scoped -- _lyx-only input, the same shape every one of the
// three CLI callers passes -- never produces a warp commit, so Committed() equals today's
// WeftCommitted for those callers. Package fabricengine (internal) to reuse
// commit_integration_test.go's fixture helpers (newCommitFixture, swapPushRecorder,
// writeWarpFile) and syncweft_integration_test.go's writeWeftConfigContent.

package fabricengine

import "testing"

// TestCommit_LyxOnlyPathspec_NeverProducesWarpCommit asserts that a Commit call scoped to
// ScopedPathspec(".", ["_lyx"]) -- the pathspec shape every CLI caller uses -- never lands a warp
// commit, so Committed() is value-identical to WeftCommitted for that call shape, even when an
// unrelated warp-side file is also dirty in the worktree.
func TestCommit_LyxOnlyPathspec_NeverProducesWarpCommit(t *testing.T) {
	f, warpPath, weftPath := newCommitFixture(t)
	swapPushRecorder(t)

	// Dirty an unrelated warp-side file to prove the _lyx-only pathspec, not
	// merely the absence of warp-side changes, is what keeps WarpCommitted
	// false.
	writeWarpFile(t, warpPath, "README", "warp change not passed to Commit")
	writeWeftConfigContent(t, weftPath, "lyx-only change")

	files := ScopedPathspec(".", []string{"_lyx"})
	result, err := f.Commit(files, "lyx-only commit", nil, SyncOptions{})
	if err != nil {
		t.Fatalf("Commit() error = %v", err)
	}

	if result.WarpCommitted {
		t.Fatalf("Commit(%v) = %+v; want WarpCommitted=false for a _lyx-only pathspec", files, result)
	}
	if !result.WeftCommitted {
		t.Fatalf("Commit(%v) = %+v; want WeftCommitted=true", files, result)
	}
	if result.Committed() != result.WeftCommitted {
		t.Errorf("Committed() = %v; want it value-identical to WeftCommitted (%v) for a _lyx-only pathspec", result.Committed(), result.WeftCommitted)
	}
}
