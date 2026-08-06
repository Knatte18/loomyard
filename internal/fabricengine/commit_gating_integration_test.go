//go:build integration

// commit_gating_integration_test.go — integration coverage for
// Fabric.Commit's commit-side skip-git gating: under opts.SkipGit, the warp
// side still commits but the weft side no-ops entirely, with no weft lock
// acquired. Package fabricengine (internal) is not strictly required here
// (no unexported surface is touched beyond the .weft lock-dir name already
// exported as weftLockDirName's literal), but stays consistent with the
// batch's other commit_*_integration_test.go files. Reuses
// commit_integration_test.go's fixture and recorder helpers
// (newCommitFixture, swapPushRecorder, writeWarpFile) and
// syncweft_integration_test.go's writeWeftConfigContent.

package fabricengine

import (
	"os"
	"path/filepath"
	"testing"
)

// TestCommit_SkipGit_TwoSided asserts that under opts.SkipGit, a two-sided Fabric.Commit lands the warp side but no-ops the weft side entirely: no weft commit and unchanged weft HEAD.
// Per the combined-commit-lock Shared Decision, the combined write lock is taken whenever the call commits anything at all, warp-only included — since this call's warp side is non-empty ("README"), the lock dir IS created here even though the weft side is skipped;
// that is the correct, intended behavior, not a weft-scoped-lock regression.
func TestCommit_SkipGit_TwoSided(t *testing.T) {
	f, warpPath, weftPath := newCommitFixture(t)
	swapPushRecorder(t)

	writeWarpFile(t, warpPath, "README", "warp change")
	writeWeftConfigContent(t, weftPath, "weft change")

	preWeftSHA, err := f.Weft.CurrentSHA()
	if err != nil {
		t.Fatalf("Weft.CurrentSHA() error = %v", err)
	}

	lockDirPath := filepath.Join(weftPath, weftLockDirName)
	if _, err := os.Stat(lockDirPath); !os.IsNotExist(err) {
		t.Fatalf("weft lock dir %q already exists before Commit(); fixture assumption violated", lockDirPath)
	}

	result, err := f.Commit([]string{"README", "_lyx/config.yaml"}, "skip-git commit", nil, SyncOptions{SkipGit: true})
	if err != nil {
		t.Fatalf("Commit() error = %v", err)
	}

	if !result.WarpCommitted || result.WarpSHA == "" {
		t.Errorf("Commit() = %+v; want the warp commit to land under SkipGit", result)
	}
	if result.WeftCommitted || result.WeftSHA != "" {
		t.Errorf("Commit() = %+v; want the weft side to no-op entirely under SkipGit", result)
	}

	postWeftSHA, err := f.Weft.CurrentSHA()
	if err != nil {
		t.Fatalf("Weft.CurrentSHA() error = %v", err)
	}
	if postWeftSHA != preWeftSHA {
		t.Errorf("weft HEAD changed under SkipGit: %q -> %q; want unchanged", preWeftSHA, postWeftSHA)
	}

	if _, err := os.Stat(lockDirPath); os.IsNotExist(err) {
		t.Errorf("weft lock dir %q does not exist after Commit() under SkipGit; want it created (the warp side alone makes this a committing call under combined-commit-lock)", lockDirPath)
	}
}

// TestCommit_SkipGit_WarpOnly asserts that a warp-only input under opts.SkipGit still lands its warp commit — SkipGit narrows to the weft side only, never suppressing the warp commit.
func TestCommit_SkipGit_WarpOnly(t *testing.T) {
	f, warpPath, _ := newCommitFixture(t)
	swapPushRecorder(t)

	writeWarpFile(t, warpPath, "README", "warp only change")

	result, err := f.Commit([]string{"README"}, "skip-git warp only commit", nil, SyncOptions{SkipGit: true})
	if err != nil {
		t.Fatalf("Commit() error = %v", err)
	}
	if !result.WarpCommitted || result.WarpSHA == "" {
		t.Errorf("Commit() = %+v; want the warp commit to land under SkipGit", result)
	}
	if result.WeftCommitted {
		t.Errorf("Commit() = %+v; want WeftCommitted=false", result)
	}
}

// TestCommit_TwoSided_NormalOpts_ControlCase is the control case for the two SkipGit tests above: under normal (zero-value) opts, a two-sided Fabric.Commit lands both sides.
func TestCommit_TwoSided_NormalOpts_ControlCase(t *testing.T) {
	f, warpPath, weftPath := newCommitFixture(t)
	swapPushRecorder(t)

	writeWarpFile(t, warpPath, "README", "warp change")
	writeWeftConfigContent(t, weftPath, "weft change")

	result, err := f.Commit([]string{"README", "_lyx/config.yaml"}, "control commit", nil, SyncOptions{})
	if err != nil {
		t.Fatalf("Commit() error = %v", err)
	}
	if !result.WarpCommitted || result.WarpSHA == "" {
		t.Errorf("Commit() = %+v; want the warp commit to land", result)
	}
	if !result.WeftCommitted || result.WeftSHA == "" {
		t.Errorf("Commit() = %+v; want the weft commit to land", result)
	}
}
