//go:build integration

// mergestage_integration_test.go covers MergeStageResolved against real conflicted pairs: resolving
// on disk then staging via the verb (unblocking MergeContinue), a path neither side lists (error, not
// a silent skip), a delete/modify conflict resolved by deletion (staging succeeds), the empty-paths
// no-op, and the Mutation Record Invariant's record assertion.

package fabricengine_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
)

// TestMergeStageResolved_ForeignMergeStateRefusesWithoutStaging pins the foreign-state refusal on
// the one mutating merge verb that lacked it.
// doc.go states that foreign git merge state — a merge fabric did not start — is refused by EVERY
// mutating merge verb, and MergeStageResolved's own godoc justified having no guard at all with
// "with no merge in progress both sides' ConflictedFiles() are empty". Both claims were false in
// exactly one state, and it is a state CONSTRAINTS.md explicitly permits an operator to create: a
// plain-git conflicted merge in the warp checkout leaves MergeInProgress() false while
// ConflictedFiles() lists the conflict, so the partition routed that path straight to `git add -A`
// and staged into a merge fabric had no record of and no lock over.
func TestMergeStageResolved_ForeignMergeStateRefusesWithoutStaging(t *testing.T) {
	h, f, _, _, _, _ := newMergePairFixture(t, ".")

	setupConflictingDivergence(t, h.PrimeWorktree(), "other", "plain-conflict.txt")
	gitMergeAllowConflict(t, h.PrimeWorktree(), "other")

	// Preconditions, asserted rather than assumed: fabric has no merge of its own, yet the path IS
	// conflicted — the exact pair of facts the old justification claimed could not co-occur.
	if inProgress, err := f.MergeInProgress(); err != nil || inProgress {
		t.Fatalf("MergeInProgress() = (%v, %v); want (false, nil) — foreign state is not fabric's merge", inProgress, err)
	}
	statusBefore := gitkit.GitStatusPorcelain(t, h.PrimeWorktree())
	if !strings.Contains(statusBefore, "plain-conflict.txt") {
		t.Fatalf("git status = %q; the test setup must have produced a real conflict on plain-conflict.txt", statusBefore)
	}

	res, err := f.MergeStageResolved([]string{"plain-conflict.txt"})
	var foreignErr *fabricengine.ErrForeignMergeState
	if !errors.As(err, &foreignErr) {
		t.Fatalf("MergeStageResolved() over foreign merge state: error = %v (%T); want *fabricengine.ErrForeignMergeState", err, err)
	}
	if res.Mutated().Len() != 0 {
		t.Errorf("MergeStageResolved() mutations = %v; want none — a refusal stages nothing", res.Mutated().Entries())
	}
	if got := gitkit.GitStatusPorcelain(t, h.PrimeWorktree()); got != statusBefore {
		t.Errorf("git status changed: before = %q, after = %q; want the operator's own merge state untouched", statusBefore, got)
	}
}

// TestMergeStageResolved_ResolvedConflictsStageThenContinueSucceeds covers the primitive's whole
// point: after a conflicted MergeIn, editing the conflicted file alone leaves it unmerged in the
// index — MergeStageResolved with exactly the conflicted paths clears both sides' ConflictedFiles(),
// and a subsequent MergeContinue then succeeds.
func TestMergeStageResolved_ResolvedConflictsStageThenContinueSucceeds(t *testing.T) {
	h, f, _, _, _, _ := newMergePairFixture(t, ".")

	setupConflictingDivergence(t, h.PrimeWorktree(), "feature", "conflict.txt")
	branchAtCurrentHEAD(t, h.PrimeWeft(), "feature-weft")

	res, err := f.MergeIn("feature")
	if err != nil {
		t.Fatalf("MergeIn(feature) error = %v", err)
	}
	if len(res.Conflicts) == 0 {
		t.Fatal("MergeIn(feature) produced no conflicts; want the seeded warp conflict")
	}

	resolvedPath := filepath.Join(h.PrimeWorktree(), "conflict.txt")
	if err := os.WriteFile(resolvedPath, []byte("resolved content\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(conflict.txt): %v", err)
	}

	stageRes, err := f.MergeStageResolved(res.Conflicts)
	if err != nil {
		t.Fatalf("MergeStageResolved(%v) error = %v", res.Conflicts, err)
	}
	if stageRes.Mutated().Len() == 0 {
		t.Error("MergeStageResolved(...).Mutated().Len() = 0; want a real staging call to populate the record")
	}

	continued, err := f.MergeContinue("")
	if err != nil {
		t.Fatalf("MergeContinue(\"\") after MergeStageResolved error = %v; want the index guard to pass", err)
	}
	if !continued.Committed {
		t.Errorf("MergeContinue(\"\").Committed = false; want true")
	}
}

// TestMergeStageResolved_PathNotConflictedOnEitherSide covers the caller-bug guard: a path listed by
// neither side's ConflictedFiles() is an error naming that path, never a silent skip.
func TestMergeStageResolved_PathNotConflictedOnEitherSide(t *testing.T) {
	h, f, _, _, _, _ := newMergePairFixture(t, ".")

	setupConflictingDivergence(t, h.PrimeWorktree(), "feature", "conflict.txt")
	branchAtCurrentHEAD(t, h.PrimeWeft(), "feature-weft")

	if _, err := f.MergeIn("feature"); err != nil {
		t.Fatalf("MergeIn(feature) error = %v", err)
	}

	const bogusPath = "never-conflicted.txt"
	_, err := f.MergeStageResolved([]string{bogusPath})
	if err == nil {
		t.Fatal("MergeStageResolved([bogus path]) error = nil; want an error naming the unconflicted path")
	}
	if got := err.Error(); !strings.Contains(got, bogusPath) {
		t.Errorf("MergeStageResolved([bogus path]) error = %q; want it to name %q", got, bogusPath)
	}
}

// TestMergeStageResolved_DeleteModifyConflictResolvedByDeletion covers that a delete/modify conflict
// resolved by removing the file stages successfully rather than erroring on the missing pathspec.
func TestMergeStageResolved_DeleteModifyConflictResolvedByDeletion(t *testing.T) {
	h, f, _, _, _, _ := newMergePairFixture(t, ".")

	const filename = "delete-modify.txt"
	warpDir := h.PrimeWorktree()

	// Seed the file, branch off, delete it on the branch, then modify it on current — a classic
	// delete/modify conflict when the branch merges into current.
	commitOnCurrentBranch(t, warpDir, filename, "seed content\n", "seed "+filename)
	commitOnBranchDeleting(t, warpDir, "feature", filename)
	commitOnCurrentBranch(t, warpDir, filename, "current content\n", "modify "+filename+" on current")

	branchAtCurrentHEAD(t, h.PrimeWeft(), "feature-weft")

	res, err := f.MergeIn("feature")
	if err != nil {
		t.Fatalf("MergeIn(feature) error = %v", err)
	}
	if len(res.Conflicts) == 0 {
		t.Fatal("MergeIn(feature) produced no conflicts; want the delete/modify conflict")
	}

	// Resolve by deleting the file — the file being gone is a legitimate resolution.
	if err := os.Remove(filepath.Join(warpDir, filename)); err != nil {
		t.Fatalf("Remove(%s): %v", filename, err)
	}

	if _, err := f.MergeStageResolved(res.Conflicts); err != nil {
		t.Fatalf("MergeStageResolved(%v) error = %v; want deletion to stage rather than error on the missing pathspec", res.Conflicts, err)
	}

	continued, err := f.MergeContinue("")
	if err != nil {
		t.Fatalf("MergeContinue(\"\") after deletion-staged MergeStageResolved error = %v", err)
	}
	if !continued.Committed {
		t.Errorf("MergeContinue(\"\").Committed = false; want true")
	}
}

// commitOnBranchDeleting checks out branch in dir off whatever is currently checked out — creating it
// if absent — removes filename and commits the removal, then switches back to whatever branch was
// checked out before.
func commitOnBranchDeleting(t *testing.T, dir, branch, filename string) {
	t.Helper()

	current := currentBranchName(t, dir)
	if branchExistsLocally(t, dir, branch) {
		gitkit.MustRun(t, dir, "git", "checkout", "-q", branch)
	} else {
		gitkit.MustRun(t, dir, "git", "checkout", "-q", "-b", branch)
	}

	gitkit.MustRun(t, dir, "git", "rm", "-q", filename)
	gitkit.MustRun(t, dir, "git", "commit", "-q", "-m", "delete "+filename+" on "+branch)

	gitkit.MustRun(t, dir, "git", "checkout", "-q", current)
}

// TestMergeStageResolved_EmptyPathsIsNoOp covers the empty-slice call: a nil error and an empty
// mutation record, with nothing staged.
func TestMergeStageResolved_EmptyPathsIsNoOp(t *testing.T) {
	_, f, _, _, _, _ := newMergePairFixture(t, ".")

	res, err := f.MergeStageResolved(nil)
	if err != nil {
		t.Fatalf("MergeStageResolved(nil) error = %v; want nil", err)
	}
	if res.Mutated().Len() != 0 {
		t.Errorf("MergeStageResolved(nil).Mutated().Len() = %d; want 0 (empty mutation record)", res.Mutated().Len())
	}

	res, err = f.MergeStageResolved([]string{})
	if err != nil {
		t.Fatalf("MergeStageResolved([]) error = %v; want nil", err)
	}
	if res.Mutated().Len() != 0 {
		t.Errorf("MergeStageResolved([]).Mutated().Len() = %d; want 0 (empty mutation record)", res.Mutated().Len())
	}
}
