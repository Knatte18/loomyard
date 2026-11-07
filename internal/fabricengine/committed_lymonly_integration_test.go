// committed_lymonly_integration_test.go checks that a _lym-only Fabric.Commit never makes a warp commit.
// It reuses fixture helpers from commit_integration_test.go and syncweft_integration_test.go.

package fabricengine

import "testing"

import "github.com/Knatte18/loomyard/internal/testkit/tiergate"

// TestCommit_LymOnlyPathspec_NeverProducesWarpCommit asserts that a Commit scoped to _lym never lands a warp commit.
// Committed() is therefore value-identical to WeftCommitted, even when an unrelated warp-side file is also dirty.
func TestCommit_LymOnlyPathspec_NeverProducesWarpCommit(t *testing.T) {
	tiergate.Require(t, 1)
	// Serial: swapPushRecorder sets the package-level spawnDetachedPushFn.
	f, warpPath, weftPath := newCommitFixture(t)
	swapPushRecorder(t)

	// Dirty an unrelated warp-side file, since a clean warp side keeps WarpCommitted false even without the pathspec.
	writeWarpFile(t, warpPath, "README", "warp change not passed to Commit")
	writeWeftConfigContent(t, weftPath, "lym-only change")

	files := ScopedPathspec(".", []string{"_lym"})
	result, err := f.Commit(files, "lym-only commit", nil, SyncOptions{})
	if err != nil {
		t.Fatalf("Commit() error = %v", err)
	}

	if result.WarpCommitted {
		t.Fatalf("Commit(%v) = %+v; want WarpCommitted=false for a _lym-only pathspec", files, result)
	}
	if !result.WeftCommitted {
		t.Fatalf("Commit(%v) = %+v; want WeftCommitted=true", files, result)
	}
	if result.Committed() != result.WeftCommitted {
		t.Errorf("Committed() = %v; want it value-identical to WeftCommitted (%v) for a _lym-only pathspec", result.Committed(), result.WeftCommitted)
	}
}
