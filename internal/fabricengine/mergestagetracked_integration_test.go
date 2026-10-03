//go:build integration

// mergestagetracked_integration_test.go covers MergeStageTracked and MergeUntrackedFiles against a real conflicted pair: a tracked, non-conflicted edit staged by the verb lands in the merge commit MergeContinue writes, a file created mid-merge in the warp is listed as untracked, an untracked weft file is not, and the verb refuses with no merge in progress.

package fabricengine_test

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
)

// TestMergeStageTracked_EditLandsInMergeCommitAndUntrackedIsListed drives a conflicted MergeIn, edits a tracked file the merge did not conflict on and creates a new file,
// then asserts the verb stages only the tracked edit and MergeContinue's commit carries it.
func TestMergeStageTracked_EditLandsInMergeCommitAndUntrackedIsListed(t *testing.T) {
	h, f, _, _, _, _ := newMergePairFixture(t, ".")
	warpDir := h.PrimeWorktree()

	gitkit.CommitFile(t, warpDir, "tracked.txt", "base\n", "seed tracked.txt")
	setupConflictingDivergence(t, warpDir, "feature", "conflict.txt")
	branchAtCurrentHEAD(t, h.PrimeWeft(), "feature-weft")

	res, err := f.MergeIn("feature")
	if err != nil {
		t.Fatalf("MergeIn(feature) error = %v", err)
	}
	if len(res.Conflicts) == 0 {
		t.Fatal("MergeIn(feature) produced no conflicts; want the seeded warp conflict")
	}

	if err := os.WriteFile(filepath.Join(warpDir, "conflict.txt"), []byte("resolved\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(conflict.txt): %v", err)
	}
	if err := os.WriteFile(filepath.Join(warpDir, "tracked.txt"), []byte("edited mid-merge\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(tracked.txt): %v", err)
	}
	if err := os.WriteFile(filepath.Join(warpDir, "created.txt"), []byte("new\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(created.txt): %v", err)
	}
	// The weft is no merge participant, so its untracked file is neither listed nor a reason to fail.
	if err := os.WriteFile(filepath.Join(h.PrimeWeft(), "weft-state.txt"), []byte("lyx state\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(weft-state.txt): %v", err)
	}

	untracked, err := f.MergeUntrackedFiles()
	if err != nil {
		t.Fatalf("MergeUntrackedFiles() error = %v", err)
	}
	if want := []string{"created.txt"}; !slices.Equal(untracked, want) {
		t.Errorf("MergeUntrackedFiles() = %v; want %v", untracked, want)
	}

	if _, err := f.MergeStageResolved(res.Conflicts); err != nil {
		t.Fatalf("MergeStageResolved(%v) error = %v", res.Conflicts, err)
	}
	staged, err := f.MergeStageTracked()
	if err != nil {
		t.Fatalf("MergeStageTracked() error = %v", err)
	}
	if staged.Mutated().Len() == 0 {
		t.Error("MergeStageTracked().Mutated().Len() = 0; want the staging call recorded")
	}

	if _, err := f.MergeContinue(""); err != nil {
		t.Fatalf("MergeContinue(\"\") error = %v", err)
	}

	changed := gitkit.Git(t, warpDir, "diff", "--name-only", "HEAD~1", "HEAD")
	if !strings.Contains(changed, "tracked.txt") {
		t.Errorf("merge commit diff against its first parent = %q; want tracked.txt carried", changed)
	}
	if strings.Contains(changed, "created.txt") {
		t.Errorf("merge commit diff = %q; want the untracked created.txt left out", changed)
	}
}

// TestMergeStageTracked_NoMergeInProgressRefuses asserts the verb refuses, staging nothing, when no fabric merge record exists.
func TestMergeStageTracked_NoMergeInProgressRefuses(t *testing.T) {
	_, f, _, _, _, _ := newMergePairFixture(t, ".")

	res, err := f.MergeStageTracked()
	var noMerge *fabricengine.ErrNoMergeInProgress
	if !errors.As(err, &noMerge) {
		t.Fatalf("MergeStageTracked() with no merge: error = %v (%T); want *fabricengine.ErrNoMergeInProgress", err, err)
	}
	if res.Mutated().Len() != 0 {
		t.Errorf("MergeStageTracked() mutations = %v; want none", res.Mutated().Entries())
	}
}

// TestMergeStageTracked_ForeignMergeStateRefuses asserts the verb refuses a plain-git conflicted merge fabric did not start.
func TestMergeStageTracked_ForeignMergeStateRefuses(t *testing.T) {
	h, f, _, _, _, _ := newMergePairFixture(t, ".")

	setupConflictingDivergence(t, h.PrimeWorktree(), "other", "plain-conflict.txt")
	gitMergeAllowConflict(t, h.PrimeWorktree(), "other")

	res, err := f.MergeStageTracked()
	var foreign *fabricengine.ErrForeignMergeState
	if !errors.As(err, &foreign) {
		t.Fatalf("MergeStageTracked() over foreign state: error = %v (%T); want *fabricengine.ErrForeignMergeState", err, err)
	}
	if res.Mutated().Len() != 0 {
		t.Errorf("MergeStageTracked() mutations = %v; want none", res.Mutated().Entries())
	}
}
