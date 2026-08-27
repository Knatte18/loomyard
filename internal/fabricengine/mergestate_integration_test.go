//go:build integration

// mergestate_integration_test.go — integration coverage for the on-disk merge-state record and the
// foreign-merge-state probes: save/load roundtrip, absence, delete idempotence, and
// foreignMergeStatePresent's true/false split against a real hubforge pair.

package fabricengine_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
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

// assertNoZeroFields fails the test if any exported field of v (a struct value, not a pointer) still
// carries its type's zero value — the guard that keeps a whole-record roundtrip assertion honest as
// the record grows fields.
func assertNoZeroFields(t *testing.T, label string, v any) {
	t.Helper()

	value := reflect.ValueOf(v)
	typ := value.Type()
	for i := 0; i < typ.NumField(); i++ {
		if value.Field(i).IsZero() {
			t.Errorf("%s.%s is the zero value; populate every field or the roundtrip assertion cannot detect that field failing to roundtrip", label, typ.Field(i).Name)
		}
	}
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
		WarpSource:    "warpsourcesha",
		WeftSource:    "weftsourcesha",
		WarpOutcome:   "staged",
		WeftOutcome:   "conflicted",
		WarpCommitted: "warpcommittedsha",
		WeftCommitted: "weftcommittedsha",
		StartedAt:     time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
	}

	// "EveryField" is only true if want actually populates every field with a non-zero value: a
	// field added to the record and left out of want here would roundtrip zero-to-zero and pass,
	// which is the test staying green while the property it names goes unchecked.
	assertNoZeroFields(t, "want", want)

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

// wantWorktreeResetEntries asserts entries is exactly one KindWorktreeReset entry carrying the warp
// pre-merge SHA — resetMergeSides no longer touches the weft, so there is nothing else for the
// record to carry.
func wantWorktreeResetEntries(t *testing.T, entries []fabricengine.Mutation, wantWarpSHA string) {
	t.Helper()

	if len(entries) != 1 {
		t.Fatalf("resetMergeSides record has %d entries; want exactly 1 (warp reset): %+v", len(entries), entries)
	}
	if entries[0].Kind != fabricengine.KindWorktreeReset {
		t.Errorf("entries[0].Kind = %q; want %q", entries[0].Kind, fabricengine.KindWorktreeReset)
	}
	if entries[0].Detail != wantWarpSHA {
		t.Errorf("entries[0] (warp) Detail = %q; want warp pre-merge SHA %q", entries[0].Detail, wantWarpSHA)
	}
}

// TestMergeState_ResetMergeSides_WarpSideConflicted covers the abort/self-abort reset with the warp
// side left dirty by a real conflicted MergeStart: warp HEAD restores to the captured pre-merge SHA,
// the warp worktree ends clean, MergeHeadPresent is false on the warp, and the record carries
// exactly one KindWorktreeReset entry. The weft side was never dirtied by this fixture, so it stays
// exactly as it started — trivially true whether or not a weft reset ever ran, but stated because
// the weft is no longer a reset target regardless.
func TestMergeState_ResetMergeSides_WarpSideConflicted(t *testing.T) {
	f, h := newMergeStateFixture(t)
	warpPath, weftPath := h.PrimeWorktree(), h.PrimeWeft()

	warpStartSHA := fabricengine.CurrentSHAForTest(t, warpPath)
	weftStartSHA := fabricengine.CurrentSHAForTest(t, weftPath)

	driveConflictedMergeStart(t, warpPath, fabricengine.WarpForTest(f))

	rec := fabricengine.NewMutations(h.Path)
	if err := fabricengine.ResetMergeSidesForTest(f, rec, warpStartSHA); err != nil {
		t.Fatalf("ResetMergeSidesForTest() error = %v", err)
	}

	if got := fabricengine.CurrentSHAForTest(t, warpPath); got != warpStartSHA {
		t.Errorf("warp HEAD after reset = %q; want restored pre-merge SHA %q", got, warpStartSHA)
	}
	if got := fabricengine.CurrentSHAForTest(t, weftPath); got != weftStartSHA {
		t.Errorf("weft HEAD after reset = %q; want unchanged %q — the weft was never dirtied and is never a reset target", got, weftStartSHA)
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

	wantWorktreeResetEntries(t, rec.Snapshot().Entries(), warpStartSHA)
}

// TestMergeState_ResetMergeSides_WeftSideConflicted covers the same call when the weft side, rather
// than the warp side, is the one left dirty by a real conflicted MergeStart: the warp-only reset
// restores the warp HEAD alone, and the weft's conflicted merge state is left exactly as it was —
// MERGE_HEAD still present, conflict markers still on disk — since the weft is not a reset target,
// per abort-does-not-reset-weft.
func TestMergeState_ResetMergeSides_WeftSideConflicted(t *testing.T) {
	f, h := newMergeStateFixture(t)
	warpPath, weftPath := h.PrimeWorktree(), h.PrimeWeft()

	warpStartSHA := fabricengine.CurrentSHAForTest(t, warpPath)

	driveConflictedMergeStart(t, weftPath, fabricengine.WeftForTest(f))

	// Captured AFTER driveConflictedMergeStart, not before: that helper commits real content onto
	// the checked-out branch before running MergeStart, so the weft's pre-reset HEAD has already
	// moved off its pre-fixture SHA — the reset's job is to leave it exactly where the conflict
	// left it, not to restore some earlier point.
	weftBeforeReset := fabricengine.CurrentSHAForTest(t, weftPath)

	rec := fabricengine.NewMutations(h.Path)
	if err := fabricengine.ResetMergeSidesForTest(f, rec, warpStartSHA); err != nil {
		t.Fatalf("ResetMergeSidesForTest() error = %v", err)
	}

	if got := fabricengine.CurrentSHAForTest(t, warpPath); got != warpStartSHA {
		t.Errorf("warp HEAD after reset = %q; want restored pre-merge SHA %q", got, warpStartSHA)
	}
	if got := fabricengine.CurrentSHAForTest(t, weftPath); got != weftBeforeReset {
		t.Errorf("weft HEAD after reset = %q; want unchanged %q — the weft is not a reset target", got, weftBeforeReset)
	}
	// The reset must leave the weft's conflict exactly as driveConflictedMergeStart left it.
	if out := gitkit.GitStatusPorcelain(t, weftPath); !strings.Contains(out, "conflict-target.txt") {
		t.Errorf("weft git status --porcelain after reset = %q; want it to still mention conflict-target.txt — the weft is not a reset target", out)
	}

	if present, err := fabricengine.WarpForTest(f).MergeHeadPresent(); err != nil || present {
		t.Errorf("warp MergeHeadPresent() after reset = (%v, %v); want (false, nil)", present, err)
	}
	if present, err := fabricengine.WeftForTest(f).MergeHeadPresent(); err != nil || !present {
		t.Errorf("weft MergeHeadPresent() after reset = (%v, %v); want (true, nil) — the weft's conflicted merge state is not a reset target", present, err)
	}

	wantWorktreeResetEntries(t, rec.Snapshot().Entries(), warpStartSHA)
}

// TestMergeState_ResetMergeSides_WarpOnly drives resetMergeSides against the hub's prime warp
// worktree, asserting the ownership gate admits it — the sole worktree resetMergeSides now ever
// resets. It was TestMergeState_ResetMergeSides_PrimePairBothSidesAdmitted, which exercised
// ownedWeftCheckout's admission of the weft primary (hubforge.NewHub's clone, a main worktree of its
// own repo, unlike an AddPair linked worktree); with the weft dropped as a reset target entirely,
// ownedWeftCheckout is gone and there is no weft-side admission left to pin.
func TestMergeState_ResetMergeSides_WarpOnly(t *testing.T) {
	f, h := newMergeStateFixture(t)
	warpPath := h.PrimeWorktree()

	warpSHA := fabricengine.CurrentSHAForTest(t, warpPath)

	rec := fabricengine.NewMutations(h.Path)
	if err := fabricengine.ResetMergeSidesForTest(f, rec, warpSHA); err != nil {
		t.Fatalf("ResetMergeSidesForTest() against the prime pair error = %v; want the ownership gate to admit the prime warp worktree", err)
	}

	wantWorktreeResetEntries(t, rec.Snapshot().Entries(), warpSHA)
}
