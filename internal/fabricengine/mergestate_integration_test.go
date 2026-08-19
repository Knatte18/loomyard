//go:build integration

// mergestate_integration_test.go — integration coverage for the on-disk merge-state record and the
// foreign-merge-state probes: save/load roundtrip, absence, delete idempotence, and
// foreignMergeStatePresent's true/false split against a real hubforge pair.

package fabricengine_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/hubforge"
)

// newMergeStateFixture builds a fresh hubforge pair and the *Fabric handle over its prime
// warp/weft worktrees — the standard fixture every test in this file drives the merge-state record
// and foreign-probe helpers against.
func newMergeStateFixture(t *testing.T) (f *fabricengine.Fabric, h *hubforge.Hub) {
	t.Helper()

	h = hubforge.NewHub(t, ".")
	f = fabricengine.NewFabricForTest(t, h.PrimeWorktree(), h.PrimeWeft())
	return f, h
}

// TestMergeState_SaveLoadRoundtripPreservesEveryField covers the save->load roundtrip, asserting
// the record lands at <weft gitdir>/fabric-merge.json and is invisible to git status on both
// sides.
func TestMergeState_SaveLoadRoundtripPreservesEveryField(t *testing.T) {
	f, h := newMergeStateFixture(t)

	want := fabricengine.MergeStateForTest{
		Verb:          "merge-in",
		Source:        "some-branch",
		Squash:        true,
		Message:       "a merge message",
		WarpStart:     "warpstartsha",
		WeftStart:     "weftstartsha",
		WarpOutcome:   "staged",
		WeftOutcome:   "conflicted",
		WarpCommitted: "warpcommittedsha",
		WeftCommitted: "weftcommittedsha",
		StartedAt:     time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
	}

	if err := fabricengine.SaveMergeStateForTest(f, want); err != nil {
		t.Fatalf("SaveMergeStateForTest() error = %v", err)
	}

	got, found, err := fabricengine.LoadMergeStateForTest(f)
	if err != nil {
		t.Fatalf("LoadMergeStateForTest() error = %v", err)
	}
	if !found {
		t.Fatal("LoadMergeStateForTest() found = false; want true after save")
	}
	if !got.StartedAt.Equal(want.StartedAt) {
		t.Errorf("LoadMergeStateForTest().StartedAt = %v; want %v", got.StartedAt, want.StartedAt)
	}
	got.StartedAt = want.StartedAt // time.Time equality via == is unreliable across encode/decode; compared above.
	if got != want {
		t.Errorf("LoadMergeStateForTest() = %+v; want %+v", got, want)
	}

	path, err := fabricengine.MergeStatePathForTest(f)
	if err != nil {
		t.Fatalf("MergeStatePathForTest() error = %v", err)
	}
	weftGitDir, err := fabricengine.WeftGitDirForTest(f)
	if err != nil {
		t.Fatalf("WeftGitDirForTest() error = %v", err)
	}
	wantPath := filepath.Join(weftGitDir, "fabric-merge.json")
	if path != wantPath {
		t.Errorf("MergeStatePathForTest() = %q; want %q", path, wantPath)
	}

	assertMergeStateFileInvisibleToGit(t, "warp", h.PrimeWorktree())
	assertMergeStateFileInvisibleToGit(t, "weft", h.PrimeWeft())
}

// assertMergeStateFileInvisibleToGit fails the test if `git status --porcelain` in dir mentions
// mergeStateFileName. It does not require dir to be otherwise clean — a prime weft worktree
// legitimately carries its own untracked scaffolding (e.g. the _lyx junction target) independent of
// the merge-state record — only that saving the record itself produced no observable git-status
// entry.
func assertMergeStateFileInvisibleToGit(t *testing.T, label, dir string) {
	t.Helper()

	out := gitkit.GitStatusPorcelain(t, dir)
	if strings.Contains(out, "fabric-merge.json") {
		t.Errorf("git status --porcelain in %s worktree %s = %q; want no mention of fabric-merge.json (the merge-state record must be invisible to git)", label, dir, out)
	}
}

// TestMergeState_AbsentRecord covers the no-record case: loadMergeState reports not-found and
// mergeRecordExists reports false.
func TestMergeState_AbsentRecord(t *testing.T) {
	f, _ := newMergeStateFixture(t)

	_, found, err := fabricengine.LoadMergeStateForTest(f)
	if err != nil {
		t.Fatalf("LoadMergeStateForTest() error = %v", err)
	}
	if found {
		t.Error("LoadMergeStateForTest() found = true; want false with no record ever saved")
	}

	exists, err := fabricengine.MergeRecordExistsForTest(f)
	if err != nil {
		t.Fatalf("MergeRecordExistsForTest() error = %v", err)
	}
	if exists {
		t.Error("MergeRecordExistsForTest() = true; want false with no record ever saved")
	}
}

// TestMergeState_DeleteRemovesAndToleratesSecondCall covers deleteMergeState removing a saved
// record and tolerating a second call against an already-absent one.
func TestMergeState_DeleteRemovesAndToleratesSecondCall(t *testing.T) {
	f, _ := newMergeStateFixture(t)

	if err := fabricengine.SaveMergeStateForTest(f, fabricengine.MergeStateForTest{Verb: "merge", Source: "x"}); err != nil {
		t.Fatalf("SaveMergeStateForTest() error = %v", err)
	}

	if err := fabricengine.DeleteMergeStateForTest(f); err != nil {
		t.Fatalf("DeleteMergeStateForTest() first call error = %v", err)
	}
	if exists, err := fabricengine.MergeRecordExistsForTest(f); err != nil || exists {
		t.Fatalf("MergeRecordExistsForTest() after delete = (%v, %v); want (false, nil)", exists, err)
	}

	if err := fabricengine.DeleteMergeStateForTest(f); err != nil {
		t.Fatalf("DeleteMergeStateForTest() second call (already absent) error = %v; want nil (tolerates absence)", err)
	}
}

// TestMergeState_ForeignMergeStatePresent covers the true/false split: a plain-git conflicted merge
// staged directly in the warp checkout must report true and leave the foreign state untouched; a
// clean pair must report false.
func TestMergeState_ForeignMergeStatePresent(t *testing.T) {
	f, h := newMergeStateFixture(t)

	present, err := fabricengine.ForeignMergeStatePresentForTest(f)
	if err != nil {
		t.Fatalf("ForeignMergeStatePresentForTest() on clean pair error = %v", err)
	}
	if present {
		t.Error("ForeignMergeStatePresentForTest() on clean pair = true; want false")
	}

	warpPath := h.PrimeWorktree()
	gitkit.MustRun(t, warpPath, "git", "checkout", "-q", "-b", "conflict-branch")
	gitkit.MustRun(t, warpPath, "git", "commit", "-q", "--allow-empty", "-m", "branch commit")
	writeConflictFile(t, warpPath, "conflict-target.txt", "branch content")
	gitkit.MustRun(t, warpPath, "git", "add", "conflict-target.txt")
	gitkit.MustRun(t, warpPath, "git", "commit", "-q", "-m", "branch content commit")

	gitkit.MustRun(t, warpPath, "git", "checkout", "-q", "-")
	writeConflictFile(t, warpPath, "conflict-target.txt", "main content")
	gitkit.MustRun(t, warpPath, "git", "add", "conflict-target.txt")
	gitkit.MustRun(t, warpPath, "git", "commit", "-q", "-m", "main content commit")

	mergeCmd := exec.Command("git", "merge", "conflict-branch")
	mergeCmd.Dir = warpPath
	_, mergeErr := mergeCmd.CombinedOutput() // conflicted merge exits non-zero, intentionally ignored

	present, err = fabricengine.ForeignMergeStatePresentForTest(f)
	if err != nil {
		t.Fatalf("ForeignMergeStatePresentForTest() after foreign conflicted merge error = %v", err)
	}
	if !present {
		t.Fatalf("ForeignMergeStatePresentForTest() after foreign conflicted merge = false; want true (mergeErr from git merge = %v)", mergeErr)
	}

	// The probe must leave the foreign state untouched: MERGE_HEAD should still be present.
	statCmd := exec.Command("git", "rev-parse", "--verify", "--quiet", "MERGE_HEAD")
	statCmd.Dir = warpPath
	if err := statCmd.Run(); err != nil {
		t.Errorf("MERGE_HEAD missing after ForeignMergeStatePresentForTest() probe; want the foreign state left untouched (rev-parse error = %v)", err)
	}
}

// writeConflictFile overwrites name inside dir with content, for
// TestMergeState_ForeignMergeStatePresent's hand-built conflicting-branch fixture.
func writeConflictFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", name, err)
	}
}

// driveConflictedMergeStart builds a divergent "conflict-branch" in dir off the current HEAD and
// runs repo.MergeStart against it, leaving dir with conflict markers present and the tracked
// worktree dirty — the state resetMergeSides' abort exists to discard. It fails the test unless the
// merge genuinely classified as conflicted, since a non-conflicted outcome would make the resulting
// reset assertions vacuous.
func driveConflictedMergeStart(t *testing.T, dir string, repo *gitrepo.Repo) {
	t.Helper()

	gitkit.MustRun(t, dir, "git", "checkout", "-q", "-b", "conflict-branch")
	writeConflictFile(t, dir, "conflict-target.txt", "branch content")
	gitkit.MustRun(t, dir, "git", "add", "conflict-target.txt")
	gitkit.MustRun(t, dir, "git", "commit", "-q", "-m", "branch content commit")

	gitkit.MustRun(t, dir, "git", "checkout", "-q", "-")
	writeConflictFile(t, dir, "conflict-target.txt", "main content")
	gitkit.MustRun(t, dir, "git", "add", "conflict-target.txt")
	gitkit.MustRun(t, dir, "git", "commit", "-q", "-m", "main content commit")

	outcome, err := repo.MergeStart("conflict-branch", false)
	if err != nil {
		t.Fatalf("MergeStart(conflict-branch) error = %v", err)
	}
	if outcome != gitrepo.MergeConflicted {
		t.Fatalf("MergeStart(conflict-branch) outcome = %v; want MergeConflicted", outcome)
	}
}

// wantWorktreeResetEntries asserts entries is exactly two KindWorktreeReset entries in warp-then-
// weft target order, per the scenario-symmetric mutation recording rule abort resets follow
// unconditionally.
func wantWorktreeResetEntries(t *testing.T, entries []fabricengine.Mutation, wantWarpSHA, wantWeftSHA string) {
	t.Helper()

	if len(entries) != 2 {
		t.Fatalf("resetMergeSides record has %d entries; want exactly 2 (warp reset, weft reset): %+v", len(entries), entries)
	}
	for i, e := range entries {
		if e.Kind != fabricengine.KindWorktreeReset {
			t.Errorf("entries[%d].Kind = %q; want %q", i, e.Kind, fabricengine.KindWorktreeReset)
		}
	}
	if entries[0].Detail != wantWarpSHA {
		t.Errorf("entries[0] (warp) Detail = %q; want warp pre-merge SHA %q", entries[0].Detail, wantWarpSHA)
	}
	if entries[1].Detail != wantWeftSHA {
		t.Errorf("entries[1] (weft) Detail = %q; want weft pre-merge SHA %q", entries[1].Detail, wantWeftSHA)
	}
}

// TestMergeState_ResetMergeSides_WarpSideConflicted covers the abort/self-abort reset with the warp
// side left dirty by a real conflicted MergeStart: both HEADs restore to the captured pre-merge
// SHAs, both worktrees end clean, MergeHeadPresent is false on both sides, and the record carries
// exactly two KindWorktreeReset entries in warp-then-weft order.
func TestMergeState_ResetMergeSides_WarpSideConflicted(t *testing.T) {
	f, h := newMergeStateFixture(t)
	warpPath, weftPath := h.PrimeWorktree(), h.PrimeWeft()

	warpStartSHA := fabricengine.CurrentSHAForTest(t, warpPath)
	weftStartSHA := fabricengine.CurrentSHAForTest(t, weftPath)

	driveConflictedMergeStart(t, warpPath, fabricengine.WarpForTest(f))

	rec := fabricengine.NewMutations(h.Path)
	if err := fabricengine.ResetMergeSidesForTest(f, rec, warpStartSHA, weftStartSHA); err != nil {
		t.Fatalf("ResetMergeSidesForTest() error = %v", err)
	}

	if got := fabricengine.CurrentSHAForTest(t, warpPath); got != warpStartSHA {
		t.Errorf("warp HEAD after reset = %q; want restored pre-merge SHA %q", got, warpStartSHA)
	}
	if got := fabricengine.CurrentSHAForTest(t, weftPath); got != weftStartSHA {
		t.Errorf("weft HEAD after reset = %q; want restored pre-merge SHA %q", got, weftStartSHA)
	}
	if out := gitkit.GitStatusPorcelain(t, warpPath); out != "" {
		t.Errorf("warp git status --porcelain after reset = %q; want clean", out)
	}
	assertMergeStateFileInvisibleToGit(t, "weft", weftPath)

	if present, err := fabricengine.WarpForTest(f).MergeHeadPresent(); err != nil || present {
		t.Errorf("warp MergeHeadPresent() after reset = (%v, %v); want (false, nil)", present, err)
	}
	if present, err := fabricengine.WeftForTest(f).MergeHeadPresent(); err != nil || present {
		t.Errorf("weft MergeHeadPresent() after reset = (%v, %v); want (false, nil)", present, err)
	}

	wantWorktreeResetEntries(t, rec.Snapshot().Entries(), warpStartSHA, weftStartSHA)
}

// TestMergeState_ResetMergeSides_WeftSideConflicted covers the same call succeeding when the weft
// side, rather than the warp side, is the one left dirty by a real conflicted MergeStart.
func TestMergeState_ResetMergeSides_WeftSideConflicted(t *testing.T) {
	f, h := newMergeStateFixture(t)
	warpPath, weftPath := h.PrimeWorktree(), h.PrimeWeft()

	warpStartSHA := fabricengine.CurrentSHAForTest(t, warpPath)
	weftStartSHA := fabricengine.CurrentSHAForTest(t, weftPath)

	driveConflictedMergeStart(t, weftPath, fabricengine.WeftForTest(f))

	rec := fabricengine.NewMutations(h.Path)
	if err := fabricengine.ResetMergeSidesForTest(f, rec, warpStartSHA, weftStartSHA); err != nil {
		t.Fatalf("ResetMergeSidesForTest() error = %v", err)
	}

	if got := fabricengine.CurrentSHAForTest(t, warpPath); got != warpStartSHA {
		t.Errorf("warp HEAD after reset = %q; want restored pre-merge SHA %q", got, warpStartSHA)
	}
	if got := fabricengine.CurrentSHAForTest(t, weftPath); got != weftStartSHA {
		t.Errorf("weft HEAD after reset = %q; want restored pre-merge SHA %q", got, weftStartSHA)
	}
	// The weft primary's own untracked scaffolding (e.g. the _lyx junction target) is legitimately
	// present regardless of the reset, so the assertion is narrowed to the conflict itself rather
	// than requiring an otherwise-clean status — see assertMergeStateFileInvisibleToGit's own note.
	if out := gitkit.GitStatusPorcelain(t, weftPath); strings.Contains(out, "conflict-target.txt") {
		t.Errorf("weft git status --porcelain after reset = %q; want no mention of conflict-target.txt (reset must discard it)", out)
	}

	if present, err := fabricengine.WarpForTest(f).MergeHeadPresent(); err != nil || present {
		t.Errorf("warp MergeHeadPresent() after reset = (%v, %v); want (false, nil)", present, err)
	}
	if present, err := fabricengine.WeftForTest(f).MergeHeadPresent(); err != nil || present {
		t.Errorf("weft MergeHeadPresent() after reset = (%v, %v); want (false, nil)", present, err)
	}

	wantWorktreeResetEntries(t, rec.Snapshot().Entries(), warpStartSHA, weftStartSHA)
}

// TestMergeState_ResetMergeSides_PrimePairBothSidesAdmitted drives resetMergeSides against the hub's
// prime warp worktree and the weft primary hubforge.NewHub clones — never an AddPair linked
// worktree — asserting the ownership gate admits both sides. This is the one fixture shape that can
// exercise ownedWeftCheckout's reason for existing at all: the weft primary is the sole weft
// checkout that is a main worktree of its repo, which ownedRegisteredLinkedWorktree's own godoc
// pins itself to excluding, and an AddPair-only fixture would only ever produce linked weft
// worktrees that pass the OLD (wrong) ownership kind too, masking the bug this decision exists to
// avoid.
func TestMergeState_ResetMergeSides_PrimePairBothSidesAdmitted(t *testing.T) {
	f, h := newMergeStateFixture(t)
	warpPath, weftPath := h.PrimeWorktree(), h.PrimeWeft()

	warpSHA := fabricengine.CurrentSHAForTest(t, warpPath)
	weftSHA := fabricengine.CurrentSHAForTest(t, weftPath)

	rec := fabricengine.NewMutations(h.Path)
	if err := fabricengine.ResetMergeSidesForTest(f, rec, warpSHA, weftSHA); err != nil {
		t.Fatalf("ResetMergeSidesForTest() against the prime pair error = %v; want the ownership gate to admit both the prime warp worktree and the weft primary", err)
	}

	wantWorktreeResetEntries(t, rec.Snapshot().Entries(), warpSHA, weftSHA)
}
