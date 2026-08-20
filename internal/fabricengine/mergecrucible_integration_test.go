//go:build integration

// mergecrucible_integration_test.go holds the regression scenarios the crucible review round
// opus-medium-r1 found against real git repositories — the ones the hermetic and pre-existing
// integration tiers both passed while the defect was live.
// Each test names the finding it pins in its own doc comment.
// Reuses newMergePairFixture and its sibling helpers from mergein_integration_test.go and
// openFreshFabric from mergein_recovery_integration_test.go, since all three files share
// package fabricengine_test.

package fabricengine_test

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

// assertSoleGuardReason fails unless err is a *fabricengine.MergeGuardError carrying exactly the one
// reason want.
func assertSoleGuardReason(t *testing.T, label string, err error, want string) {
	t.Helper()

	var guardErr *fabricengine.MergeGuardError
	if !errors.As(err, &guardErr) {
		t.Fatalf("%s error = %v (%T); want *fabricengine.MergeGuardError", label, err, err)
	}
	if len(guardErr.Reasons) != 1 || guardErr.Reasons[0] != want {
		t.Errorf("%s guard reasons = %v; want exactly [%q]", label, guardErr.Reasons, want)
	}
}

// TestMergeCrucible_DetachedHeadRefused pins finding F2: a merge verb must refuse while either
// checkout has HEAD pointing straight at a commit rather than at a branch.
// Without the guard, MergeIn reported full success on a detached warp HEAD, landed a warp merge
// commit no ref reaches, landed the weft merge commit permanently, and deleted its own record — so
// the warp half vanished at the next checkout with the weft half already final and no longer
// abortable.
// The table drives both sides, since the guard is aggregated and must fire whichever side is
// detached.
func TestMergeCrucible_DetachedHeadRefused(t *testing.T) {
	tests := []struct {
		name       string
		detachWeft bool
	}{
		{name: "WarpDetached", detachWeft: false},
		{name: "WeftDetached", detachWeft: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, f, commitOnWarpBranch, commitOnWeftBranch, _, _ := newMergePairFixture(t, ".")
			commitOnWarpBranch("feature", "feature.txt", "feature\n", "feature: warp")
			commitOnWeftBranch("feature-weft", "feature.txt", "feature\n", "feature: weft")

			detachDir := h.PrimeWorktree()
			if tt.detachWeft {
				detachDir = h.PrimeWeft()
			}
			gitkit.MustRun(t, detachDir, "git", "checkout", "-q", "--detach", "HEAD")

			warpBefore := fabricengine.CurrentSHAForTest(t, h.PrimeWorktree())
			weftBefore := fabricengine.CurrentSHAForTest(t, h.PrimeWeft())

			_, err := f.MergeIn("feature")
			assertSoleGuardReason(t, "MergeIn(feature)", err, "checkout is not on a branch")

			if got := fabricengine.CurrentSHAForTest(t, h.PrimeWorktree()); got != warpBefore {
				t.Errorf("warp HEAD = %q; want unchanged %q", got, warpBefore)
			}
			if got := fabricengine.CurrentSHAForTest(t, h.PrimeWeft()); got != weftBefore {
				t.Errorf("weft HEAD = %q; want unchanged %q", got, weftBefore)
			}

			inProgress, err := f.MergeInProgress()
			if err != nil {
				t.Fatalf("MergeInProgress: %v", err)
			}
			if inProgress {
				t.Error("MergeInProgress() = true after a refused merge; want false — a guard refusal must write no record")
			}
		})
	}
}

// TestMergeCrucible_ContinueRefusesAttemptThatNeverReachedBothSides pins finding F1: a record whose
// attempt never reached one side must refuse MergeContinue outright, before anything lands.
// The reconstructed state is byte-for-byte what a kill between the two MergeStart calls leaves —
// merge.go persists WarpOutcome only after the warp MergeStart returns, so WeftOutcome is still
// empty at that instant. Without the guard, MergeContinue committed the warp side, then failed
// concluding a weft side that was never started, returned "run MergeContinue again" (an instruction
// that could never succeed), and left the pair out of correspondence.
// MergeAbort must still recover the same record — that is the whole point of refusing.
func TestMergeCrucible_ContinueRefusesAttemptThatNeverReachedBothSides(t *testing.T) {
	h, f, commitOnWarpBranch, commitOnWeftBranch, commitOnWarpCurrent, commitOnWeftCurrent := newMergePairFixture(t, ".")
	commitOnWarpBranch("feature", "feature.txt", "feature\n", "feature: warp")
	commitOnWeftBranch("feature-weft", "feature.txt", "feature\n", "feature: weft")
	// Diverge the target branches so the reconstructed attempt STAGES rather than fast-forwards:
	// a fast-forward moves HEAD, and the crash window this test reconstructs is the staged one.
	commitOnWarpCurrent("target.txt", "target\n", "target: warp")
	commitOnWeftCurrent("target.txt", "target\n", "target: weft")

	warpStart := fabricengine.CurrentSHAForTest(t, h.PrimeWorktree())
	weftStart := fabricengine.CurrentSHAForTest(t, h.PrimeWeft())

	// The warp side of the attempt really ran; the weft side never did.
	gitkit.MustRun(t, h.PrimeWorktree(), "git", "merge", "--no-commit", "feature")
	if err := fabricengine.SaveMergeStateForTest(f, fabricengine.MergeStateForTest{
		Verb:        "merge-in",
		Source:      "feature",
		WarpStart:   warpStart,
		WeftStart:   weftStart,
		WarpOutcome: "staged",
		StartedAt:   time.Now(),
	}); err != nil {
		t.Fatalf("SaveMergeStateForTest: %v", err)
	}

	resumed := openFreshFabric(t, h.PrimeWorktree())
	_, err := resumed.MergeContinue("")
	assertSoleGuardReason(t, "MergeContinue on an attempt that never reached both sides", err,
		"merge attempt did not reach both sides")

	if got := fabricengine.CurrentSHAForTest(t, h.PrimeWorktree()); got != warpStart {
		t.Errorf("warp HEAD = %q after the refused MergeContinue; want unchanged %q — the refusal must land nothing", got, warpStart)
	}
	if got := fabricengine.CurrentSHAForTest(t, h.PrimeWeft()); got != weftStart {
		t.Errorf("weft HEAD = %q after the refused MergeContinue; want unchanged %q", got, weftStart)
	}

	// The record survives the refusal, and MergeAbort is still the working recovery.
	if _, err := openFreshFabric(t, h.PrimeWorktree()).MergeAbort(); err != nil {
		t.Fatalf("MergeAbort after a refused MergeContinue: %v", err)
	}
	if got := fabricengine.CurrentSHAForTest(t, h.PrimeWorktree()); got != warpStart {
		t.Errorf("warp HEAD = %q after MergeAbort; want %q", got, warpStart)
	}
	if got := fabricengine.CurrentSHAForTest(t, h.PrimeWeft()); got != weftStart {
		t.Errorf("weft HEAD = %q after MergeAbort; want %q", got, weftStart)
	}
	inProgress, err := openFreshFabric(t, h.PrimeWorktree()).MergeInProgress()
	if err != nil {
		t.Fatalf("MergeInProgress: %v", err)
	}
	if inProgress {
		t.Error("MergeInProgress() = true after MergeAbort; want false")
	}
}

// TestMergeCrucible_ResultFlagsDescribeWhatHappened pins finding F3: MergeResult.Committed and
// MergeResult.AlreadyUpToDate must describe what the call did to the pair, not which return
// statement it reached.
// Both verbs used to hardcode Committed true on the both-sides-clean path, so a merge that
// fast-forwarded both sides reported committed with no merge_committed entry anywhere in the record,
// and the loser of two concurrent MergeIn calls reported
// {already_up_to_date:false, committed:true, mutations:[]} for a call that did nothing — where a
// strictly sequential run of the same two calls honestly reports {already_up_to_date:true,
// committed:false}. The second subtest is that sequential control, which is what the interleaved
// loser now also reports.
func TestMergeCrucible_ResultFlagsDescribeWhatHappened(t *testing.T) {
	t.Run("FastForwardBothSidesFabricatesNoCommit", func(t *testing.T) {
		h, f, commitOnWarpBranch, commitOnWeftBranch, _, _ := newMergePairFixture(t, ".")
		commitOnWarpBranch("feature", "feature.txt", "feature\n", "feature: warp")
		commitOnWeftBranch("feature-weft", "feature.txt", "feature\n", "feature: weft")

		res, err := f.MergeIn("feature")
		if err != nil {
			t.Fatalf("MergeIn(feature) error = %v", err)
		}
		if res.Committed {
			t.Error("MergeIn(feature).Committed = true; want false — both sides fast-forwarded, so no conclude-commit exists")
		}
		if res.AlreadyUpToDate {
			t.Error("MergeIn(feature).AlreadyUpToDate = true; want false — both sides advanced")
		}
		// The pair really did advance, so the flags are reporting "no commit", not "no merge".
		if !fileExistsInWorktree(t, h.PrimeWorktree(), "feature.txt") {
			t.Error("feature.txt missing from the warp worktree; want the fast-forward to have landed")
		}
	})

	t.Run("SecondCallReportsAlreadyUpToDateNotCommitted", func(t *testing.T) {
		_, f, commitOnWarpBranch, commitOnWeftBranch, commitOnWarpCurrent, commitOnWeftCurrent := newMergePairFixture(t, ".")
		commitOnWarpBranch("feature", "feature.txt", "feature\n", "feature: warp")
		commitOnWeftBranch("feature-weft", "feature.txt", "feature\n", "feature: weft")
		commitOnWarpCurrent("target.txt", "target\n", "target: warp")
		commitOnWeftCurrent("target.txt", "target\n", "target: weft")

		first, err := f.MergeIn("feature")
		if err != nil {
			t.Fatalf("first MergeIn(feature) error = %v", err)
		}
		if !first.Committed {
			t.Error("first MergeIn(feature).Committed = false; want true — both sides needed a conclude-commit")
		}

		second, err := f.MergeIn("feature")
		if err != nil {
			t.Fatalf("second MergeIn(feature) error = %v", err)
		}
		if !second.AlreadyUpToDate {
			t.Error("second MergeIn(feature).AlreadyUpToDate = false; want true")
		}
		if second.Committed {
			t.Error("second MergeIn(feature).Committed = true; want false — nothing was committed")
		}
	})
}

// fileExistsInWorktree reports whether name exists at the root of the worktree at dir.
func fileExistsInWorktree(t *testing.T, dir, name string) bool {
	t.Helper()

	_, err := os.Stat(filepath.Join(dir, name))
	if err == nil {
		return true
	}
	if os.IsNotExist(err) {
		return false
	}
	t.Fatalf("stat %s in %s: %v", name, dir, err)
	return false
}

// TestMergeCrucible_RemoveRefusesAPairSomeOtherMergeIsConsuming pins finding F5: Topology.Remove
// must refuse a pair whose branches some OTHER pair's merge is currently resolving against.
// The pre-existing guard asks only "is the pair being removed itself mid-merge", which is a
// different subject: with the prime pair mid-merge on merge-in <slug>, removing <slug> succeeded and
// deleted branch <slug>-weft out from under the live merge, leaving the source work reachable only
// from the remote if the operator then aborted. Once the merge is aborted the same Remove must
// succeed again, so the guard closes a window rather than blocking the pair forever.
func TestMergeCrucible_RemoveRefusesAPairSomeOtherMergeIsConsuming(t *testing.T) {
	h := hubforge.NewHub(t, ".")
	const slug = "merge-crucible-source"
	hubforge.AddPair(t, h, slug)

	sourceWarpDir := h.PairWarpWorktree(slug)
	sourceWeftDir := h.PairWeftSibling(slug)
	sourceBranch, err := readBranchForTest(t, sourceWarpDir)
	if err != nil {
		t.Fatalf("readBranchForTest(%s): %v", sourceWarpDir, err)
	}

	// Conflicting divergence on the warp side only — a weft-root conflict would be unmappable and
	// self-abort the whole attempt — so MergeIn on the prime leaves a live record naming sourceBranch
	// rather than concluding immediately.
	commitOnCurrentBranch(t, sourceWarpDir, "conflict.txt", "source side\n", "source: warp conflict")
	commitOnCurrentBranch(t, sourceWeftDir, "source-only.txt", "source weft\n", "source: weft advance")
	commitOnCurrentBranch(t, h.PrimeWorktree(), "conflict.txt", "prime side\n", "prime: warp conflict")
	commitOnCurrentBranch(t, h.PrimeWeft(), "prime-only.txt", "prime weft\n", "prime: weft advance")

	primeLocation, err := lyxcwd.ResolveWorktree(h.PrimeWorktree())
	if err != nil {
		t.Fatalf("lyxcwd.ResolveWorktree(%s): %v", h.PrimeWorktree(), err)
	}
	prime, err := fabricengine.Open(primeLocation)
	if err != nil {
		t.Fatalf("fabricengine.Open(prime): %v", err)
	}

	res, err := prime.MergeIn(sourceBranch)
	if err != nil {
		t.Fatalf("MergeIn(%s) on the prime pair error = %v; want a conflict result", sourceBranch, err)
	}
	if len(res.Conflicts) == 0 {
		t.Fatalf("MergeIn(%s).Conflicts is empty; the fixture must leave a live merge record", sourceBranch)
	}

	_, err = h.Topology.Remove(primeLocation, slug, false)
	var refused *fabricengine.ErrMergeInProgress
	if !errors.As(err, &refused) {
		t.Fatalf("Remove(%s) while the prime pair is mid-merge on its branches: error = %v (%T); want *ErrMergeInProgress", slug, err, err)
	}
	if !fileExistsInWorktree(t, sourceWarpDir, "conflict.txt") {
		t.Errorf("source warp worktree %s was torn down by the refused Remove", sourceWarpDir)
	}
	if !branchExistsLocally(t, h.PrimeWeft(), fabricengine.WeftBranchName(sourceBranch)) {
		t.Errorf("weft branch %q was deleted by the refused Remove; want it intact", fabricengine.WeftBranchName(sourceBranch))
	}

	// force answers dirtiness only, never a live merge record.
	if _, err := h.Topology.Remove(primeLocation, slug, true); !errors.As(err, &refused) {
		t.Fatalf("Remove(%s, force=true): error = %v (%T); want *ErrMergeInProgress even with force", slug, err, err)
	}

	// Once the merge is aborted the window is closed and the same Remove must succeed.
	if _, err := prime.MergeAbort(); err != nil {
		t.Fatalf("MergeAbort: %v", err)
	}
	if _, err := h.Topology.Remove(primeLocation, slug, true); err != nil {
		t.Fatalf("Remove(%s) after MergeAbort: %v; want success — the guard must close a window, not block the pair forever", slug, err)
	}
}

// TestMergeCrucible_ConflictsIsEmptyNeverNil pins finding F7: MergeResult.Conflicts is documented as
// empty-never-nil so a caller's JSON never sees "conflicts": null, and merge.go declares the
// mergeNoConflicts sentinel for exactly that — but MergeContinue and MergeAbort both returned a bare
// MergeResult, leaving the field nil on their success paths.
// The check is on the marshalled JSON, since null-vs-[] is the property that actually matters to a
// consumer.
func TestMergeCrucible_ConflictsIsEmptyNeverNil(t *testing.T) {
	assertConflictsMarshalsAsArray := func(t *testing.T, label string, res fabricengine.MergeResult) {
		t.Helper()
		if res.Conflicts == nil {
			t.Errorf("%s.Conflicts is nil; want an empty non-nil slice", label)
		}
		data, err := json.Marshal(res)
		if err != nil {
			t.Fatalf("json.Marshal(%s): %v", label, err)
		}
		if !strings.Contains(string(data), `"conflicts":[]`) {
			t.Errorf("%s marshalled to %s; want it to carry \"conflicts\":[] rather than null", label, data)
		}
	}

	t.Run("MergeContinue", func(t *testing.T) {
		h, f := mergeCrucibleWarpConflictFixture(t)
		resolveWarpConflict(t, h.PrimeWorktree(), "conflict.txt")
		res, err := f.MergeContinue("")
		if err != nil {
			t.Fatalf("MergeContinue: %v", err)
		}
		assertConflictsMarshalsAsArray(t, "MergeContinue", res)
	})

	t.Run("MergeAbort", func(t *testing.T) {
		_, f := mergeCrucibleWarpConflictFixture(t)
		res, err := f.MergeAbort()
		if err != nil {
			t.Fatalf("MergeAbort: %v", err)
		}
		assertConflictsMarshalsAsArray(t, "MergeAbort", res)
	})
}

// mergeCrucibleWarpConflictFixture builds a pair left mid-merge by a real MergeIn that conflicted on
// the warp side only — the weft side diverges cleanly, since a weft-root conflict would be
// unmappable and self-abort the whole attempt before any record survives.
func mergeCrucibleWarpConflictFixture(t *testing.T) (*hubforge.Hub, *fabricengine.Fabric) {
	t.Helper()

	hub, fabric, onWarpBranch, onWeftBranch, onWarpCurrent, onWeftCurrent := newMergePairFixture(t, ".")
	onWarpBranch("feature", "conflict.txt", "feature side\n", "feature: warp conflict")
	onWeftBranch("feature-weft", "weft-only.txt", "weft feature\n", "feature: weft advance")
	onWarpCurrent("conflict.txt", "target side\n", "target: warp conflict")
	onWeftCurrent("target-only.txt", "weft target\n", "target: weft advance")

	res, err := fabric.MergeIn("feature")
	if err != nil {
		t.Fatalf("MergeIn(feature): %v", err)
	}
	if len(res.Conflicts) == 0 {
		t.Fatal("MergeIn(feature).Conflicts is empty; the fixture must leave the pair mid-merge")
	}
	return hub, fabric
}

// resolveWarpConflict writes a resolution for name in the warp worktree at dir and stages it, so
// MergeContinue's unresolved-conflicts guard passes.
func resolveWarpConflict(t *testing.T, dir, name string) {
	t.Helper()

	if err := os.WriteFile(filepath.Join(dir, name), []byte("resolved\n"), 0o644); err != nil {
		t.Fatalf("write resolution for %s in %s: %v", name, dir, err)
	}
	gitkit.MustRun(t, dir, "git", "add", name)
}

// mergeHeadPresentInCheckout reports whether dir's checkout holds a live MERGE_HEAD, probed with
// plain git rather than through the engine, so a test can assert what git believes independently of
// what fabric's own record says.
func mergeHeadPresentInCheckout(t *testing.T, dir string) bool {
	t.Helper()

	cmd := exec.Command("git", "rev-parse", "--verify", "--quiet", "MERGE_HEAD")
	cmd.Dir = dir
	return cmd.Run() == nil
}

// TestMergeCrucible_EmptyResultMergeIsConcludedNotAbandoned pins crucible round opus-medium-r2's
// finding R1 at the pair level.
// Fixture: on both sides the source branch and the current branch reach the same content
// independently, so neither source is an ancestor of its side's HEAD, yet merging it stages nothing
// and moves no HEAD. Before the fix, gitrepo.MergeStart classified that as MergeAlreadyUpToDate, so
// MergeIn reported {ok, already_up_to_date: true, committed: false}, skipped the conclude on both
// sides, recorded the pre-merge SHAs as the pair's post-merge correspondence, and deleted its own
// merge-state record -- while git had a live MERGE_HEAD in both checkouts. The merge was silently
// lost, MergeInProgress() disagreed with git, and every merge verb (including MergeAbort, the
// recovery verb) then refused the pair with *ErrForeignMergeState, leaving only plain git as a way
// out.
// The assertion that catches the regression is the MERGE_HEAD pair: a verb that returns without
// error must never leave git-level merge state behind on either side.
func TestMergeCrucible_EmptyResultMergeIsConcludedNotAbandoned(t *testing.T) {
	h, f, commitOnWarpBranch, commitOnWeftBranch, commitOnWarpCurrent, commitOnWeftCurrent := newMergePairFixture(t, ".")

	// The same content reaches the branch and the trunk independently, on both sides.
	commitOnWarpBranch("feature", "shared.txt", "same change\n", "feature: warp same change")
	commitOnWarpCurrent("shared.txt", "same change\n", "target: warp same change, reached independently")
	commitOnWeftBranch("feature-weft", "shared-weft.txt", "same change\n", "feature: weft same change")
	commitOnWeftCurrent("shared-weft.txt", "same change\n", "target: weft same change, reached independently")

	warpBefore := fabricengine.CurrentSHAForTest(t, h.PrimeWorktree())
	weftBefore := fabricengine.CurrentSHAForTest(t, h.PrimeWeft())

	res, err := f.MergeIn("feature")
	if err != nil {
		t.Fatalf("MergeIn(feature): %v", err)
	}

	if mergeHeadPresentInCheckout(t, h.PrimeWorktree()) {
		t.Error("MERGE_HEAD is live in the warp checkout after MergeIn returned without error; fabric abandoned a merge it started")
	}
	if mergeHeadPresentInCheckout(t, h.PrimeWeft()) {
		t.Error("MERGE_HEAD is live in the weft checkout after MergeIn returned without error; fabric abandoned a merge it started")
	}

	if res.AlreadyUpToDate {
		t.Error("MergeIn(feature).AlreadyUpToDate = true; neither source is an ancestor of its side's HEAD, so this is a real merge")
	}
	if !res.Committed {
		t.Error("MergeIn(feature).Committed = false; a real merge on both sides must land its conclude-commit")
	}
	if got := fabricengine.CurrentSHAForTest(t, h.PrimeWorktree()); got == warpBefore {
		t.Errorf("warp HEAD = %q, unchanged; want the conclude-commit to have landed", got)
	}
	if got := fabricengine.CurrentSHAForTest(t, h.PrimeWeft()); got == weftBefore {
		t.Errorf("weft HEAD = %q, unchanged; want the conclude-commit to have landed", got)
	}

	inProgress, err := f.MergeInProgress()
	if err != nil {
		t.Fatalf("MergeInProgress: %v", err)
	}
	if inProgress {
		t.Error("MergeInProgress() = true after a completed MergeIn; want false")
	}

	// The pair must still be usable: before the fix, the abandoned MERGE_HEAD made every subsequent
	// merge verb refuse with *ErrForeignMergeState.
	if _, err := f.MergeAbort(); !errors.As(err, new(*fabricengine.ErrNoMergeInProgress)) {
		t.Errorf("MergeAbort() after a completed MergeIn error = %v (%T); want *fabricengine.ErrNoMergeInProgress — a *ErrForeignMergeState here means fabric left state it will not clean up", err, err)
	}
}

// installRefusingPreCommitHook writes a pre-commit hook in dir's checkout that always exits 1, so
// the next `git commit` there fails the way a policy hook, a missing gpg key, or a full disk would.
// It returns a function that removes the hook again.
func installRefusingPreCommitHook(t *testing.T, dir string) (remove func()) {
	t.Helper()

	hookDir := strings.TrimSpace(gitOutput(t, dir, "rev-parse", "--absolute-git-dir")) + "/hooks"
	if err := os.MkdirAll(hookDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%s): %v", hookDir, err)
	}
	hook := filepath.Join(hookDir, "pre-commit")
	if err := os.WriteFile(hook, []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatalf("write %s: %v", hook, err)
	}
	return func() {
		if err := os.Remove(hook); err != nil {
			t.Fatalf("remove %s: %v", hook, err)
		}
	}
}

// TestMergeCrucible_AbortRefusesAnAttemptWhoseConcludeLanded pins crucible round opus-medium-r2's
// finding R2: MergeAbort restores both sides from the recorded pre-merge SHAs, so an abort issued
// against a half-concluded attempt discarded a conclude-commit that had really landed -- in this
// flow, one carrying the operator's own hand-written conflict resolutions, reset away under
// force: true with an "ok" result and no warning.
// Two arms, because the record is not always honest about what landed:
//   - Recorded: the weft conclude fails on a refusing hook after the warp conclude landed and was
//     written into the record, which is the shape the ErrMergeIncomplete path documents as
//     deliberate retention.
//   - Invisible: the operator concludes the warp side by hand with plain git, so warp HEAD has moved
//     past its recorded start while warp_committed is still empty. concludeMergeSides leaves exactly
//     this shape whenever CurrentSHA or the record re-save fails after `git commit` succeeded, and a
//     guard keyed only on the recorded SHA would sail straight through it.
//
// Both must refuse, and the landed commit must survive; MergeContinue must then still finish.
func TestMergeCrucible_AbortRefusesAnAttemptWhoseConcludeLanded(t *testing.T) {
	tests := []struct {
		name string
		// landWarpConclude drives the warp side's conclude-commit into place and reports the SHA it
		// landed, leaving the pair mid-merge with the record still live.
		landWarpConclude func(t *testing.T, h *hubforge.Hub, f *fabricengine.Fabric) string
	}{
		{
			name: "RecordedConcludeSHA",
			landWarpConclude: func(t *testing.T, h *hubforge.Hub, f *fabricengine.Fabric) string {
				t.Helper()
				removeHook := installRefusingPreCommitHook(t, h.PrimeWeft())
				t.Cleanup(removeHook)
				if _, err := f.MergeContinue(""); !errors.As(err, new(*fabricengine.ErrMergeIncomplete)) {
					t.Fatalf("MergeContinue(\"\") error = %v (%T); want *fabricengine.ErrMergeIncomplete — the fixture needs the weft conclude to fail after the warp conclude landed", err, err)
				}
				return fabricengine.CurrentSHAForTest(t, h.PrimeWorktree())
			},
		},
		{
			name: "InvisibleConcludeTheRecordNeverLearnedAbout",
			landWarpConclude: func(t *testing.T, h *hubforge.Hub, f *fabricengine.Fabric) string {
				t.Helper()
				gitkit.MustRun(t, h.PrimeWorktree(), "git", "commit", "--no-edit")
				return fabricengine.CurrentSHAForTest(t, h.PrimeWorktree())
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, f := mergeCrucibleWarpConflictFixture(t)
			resolveWarpConflict(t, h.PrimeWorktree(), "conflict.txt")

			warpStart := readMergeRecordWarpStart(t, h)
			landedSHA := tt.landWarpConclude(t, h, f)
			if landedSHA == warpStart {
				t.Fatalf("fixture broken: warp HEAD = %q is still its recorded pre-merge SHA; no conclude landed", landedSHA)
			}

			res, err := f.MergeAbort()
			assertSoleGuardReason(t, "MergeAbort()", err, "merge conclude already landed")
			if res.Mutated().Len() != 0 {
				t.Errorf("MergeAbort() mutations = %v; want none — a refusal must not touch either checkout", res.Mutated().Entries())
			}
			if got := fabricengine.CurrentSHAForTest(t, h.PrimeWorktree()); got != landedSHA {
				t.Errorf("warp HEAD = %q after the refused abort; want the landed conclude-commit %q still in place", got, landedSHA)
			}
			inProgress, err := f.MergeInProgress()
			if err != nil {
				t.Fatalf("MergeInProgress: %v", err)
			}
			if !inProgress {
				t.Error("MergeInProgress() = false after a refused abort; the record must survive so MergeContinue can still finish")
			}
		})
	}
}

// readMergeRecordWarpStart reads the pre-merge SHA the live merge-state record holds for the warp
// side, straight off disk -- the value MergeAbort would reset that side to.
func readMergeRecordWarpStart(t *testing.T, h *hubforge.Hub) string {
	t.Helper()

	path := filepath.Join(strings.TrimSpace(gitOutput(t, h.PrimeWeft(), "rev-parse", "--absolute-git-dir")), "fabric-merge.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read merge record %s: %v", path, err)
	}
	var record struct {
		WarpStart string `json:"warp_start"`
	}
	if err := json.Unmarshal(raw, &record); err != nil {
		t.Fatalf("unmarshal merge record %s: %v", path, err)
	}
	if record.WarpStart == "" {
		t.Fatalf("merge record %s has an empty warp_start", path)
	}
	return record.WarpStart
}

// isAncestorInCheckout reports whether ref is an ancestor of descendant in dir's checkout, probed
// with plain git so a test can state the precondition the engine's own pre-lock probe reads.
func isAncestorInCheckout(t *testing.T, dir, ref, descendant string) bool {
	t.Helper()

	cmd := exec.Command("git", "merge-base", "--is-ancestor", ref, descendant)
	cmd.Dir = dir
	return cmd.Run() == nil
}

// TestMergeCrucible_DerivedAlreadyUpToDateIsReadFromTheRecord closes crucible round opus-medium-r2's
// residual 1: mergeState.bothSidesAlreadyUpToDate, which MergeResult.AlreadyUpToDate is derived
// from, had no test coverage at all. Hardwiring it to false left the entire suite green.
// The pre-existing subtest that looks like it covers the field does not: a plain second merge call is
// caught by the pre-lock already-up-to-date probe, which returns a hardcoded AlreadyUpToDate: true
// from a different return site, so the derived field is never read.
// Reaching the derived field needs both post-lock MergeStart outcomes to come back up_to_date while
// the pre-lock probe said otherwise. A real race between two processes gets there, but the route
// this test uses is deterministic, single-process and needs no seam: a squash merge whose source is
// NOT an ancestor of HEAD on either side -- so IsAncestor is false and the pre-lock probe cannot
// early-return -- but whose squash result tree equals HEAD's own tree, so `git merge --squash`
// stages nothing, moves no HEAD, writes no MERGE_HEAD, and classifies up_to_date on both sides.
// Reporting AlreadyUpToDate there is the honest answer, and it can only have come from the record.
func TestMergeCrucible_DerivedAlreadyUpToDateIsReadFromTheRecord(t *testing.T) {
	h, f, commitOnWarpBranch, commitOnWeftBranch, commitOnWarpCurrent, commitOnWeftCurrent := newMergePairFixture(t, ".")

	// The same content reaches the branch and the trunk independently, on both sides, so each
	// source is a real non-ancestor whose merge result is nevertheless empty.
	commitOnWarpBranch("feature", "shared.txt", "same change\n", "feature: warp same change")
	commitOnWarpCurrent("shared.txt", "same change\n", "target: warp same change, reached independently")
	commitOnWeftBranch("feature-weft", "shared-weft.txt", "same change\n", "feature: weft same change")
	commitOnWeftCurrent("shared-weft.txt", "same change\n", "target: weft same change, reached independently")

	// This is the whole point of the fixture: an ancestor source would be caught by the pre-lock
	// probe's hardcoded return and the derived field would never be read.
	if isAncestorInCheckout(t, h.PrimeWorktree(), "feature", "HEAD") {
		t.Fatal("fixture broken: feature is an ancestor of the warp HEAD, so the pre-lock probe would short-circuit before the derived field is read")
	}
	if isAncestorInCheckout(t, h.PrimeWeft(), "feature-weft", "HEAD") {
		t.Fatal("fixture broken: feature-weft is an ancestor of the weft HEAD, so the pre-lock probe would short-circuit before the derived field is read")
	}

	warpBefore := fabricengine.CurrentSHAForTest(t, h.PrimeWorktree())
	weftBefore := fabricengine.CurrentSHAForTest(t, h.PrimeWeft())

	res, err := f.Merge("feature", fabricengine.MergeOptions{Squash: true})
	if err != nil {
		t.Fatalf("Merge(feature, squash): %v", err)
	}

	if !res.AlreadyUpToDate {
		t.Error("Merge(feature, squash).AlreadyUpToDate = false; want true — both sides' post-lock MergeStart outcomes are up_to_date, which is what the derived field reads")
	}
	if res.Committed {
		t.Error("Merge(feature, squash).Committed = true; want false — a squash with an empty result has nothing to commit")
	}
	if got := fabricengine.CurrentSHAForTest(t, h.PrimeWorktree()); got != warpBefore {
		t.Errorf("warp HEAD = %q; want unchanged %q", got, warpBefore)
	}
	if got := fabricengine.CurrentSHAForTest(t, h.PrimeWeft()); got != weftBefore {
		t.Errorf("weft HEAD = %q; want unchanged %q", got, weftBefore)
	}
	if mergeHeadPresentInCheckout(t, h.PrimeWorktree()) || mergeHeadPresentInCheckout(t, h.PrimeWeft()) {
		t.Error("MERGE_HEAD is live after a squash merge; squash never writes one, so the up_to_date classification must not have left git mid-merge")
	}

	inProgress, err := f.MergeInProgress()
	if err != nil {
		t.Fatalf("MergeInProgress: %v", err)
	}
	if inProgress {
		t.Error("MergeInProgress() = true after the merge returned; want false")
	}
}
