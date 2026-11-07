// mergestate_integration_test.go covers the on-disk merge-state record and the foreign-merge-state probes.
// It tests the save and load roundtrip, absence, and delete idempotence;
// foreignMergeStatePresent's true and false cases run against a real hubforge pair.

package fabricengine_test

import (
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
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// newMergeStateFixture builds a fresh hubforge pair and the *Fabric handle over its prime warp and weft worktrees.
// Every test in this file uses it to drive the merge-state record and foreign-probe helpers.
func newMergeStateFixture(t *testing.T) (f *fabricengine.Fabric, h *hubforge.Hub) {
	t.Helper()

	h = hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	f = fabricengine.NewFabricForTest(t, h.PrimeWorktree(), h.PrimeRecords())
	return f, h
}

// assertNoZeroFields fails the test for any field of v, a struct value, that holds its type's zero value.
// It keeps a whole-record roundtrip assertion honest as the record grows fields.
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

// TestMergeState_Record drives the on-disk merge-state record through one pair, in this order:
// with no record saved, loadMergeState reports not-found and mergeRecordExists reports false;
// the save->load roundtrip preserves every field;
// the record lands at <weft gitdir>/fabric-merge.json, and git status on both sides does not show it;
// and deleteMergeState removes a saved record and tolerates a second call against an absent one.
// The steps run serially on one fixture, each starting from the record state the step before left.
//
//lym:seam internal/fabricengine#MergeStateForTest
func TestMergeState_Record(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	f, h := newMergeStateFixture(t)

	t.Run("absent record", func(t *testing.T) {
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
	})

	t.Run("save and load roundtrip preserves every field", func(t *testing.T) {
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

		// "EveryField" is only true if want populates every field with a non-zero value.
		// A field added to the record and left out of want would roundtrip zero-to-zero and pass,
		// so the test would stay green while the property it names goes unchecked.
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
		assertMergeStateFileInvisibleToGit(t, "weft", h.PrimeRecords())
	})

	t.Run("delete removes the record and tolerates a second call", func(t *testing.T) {
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
	})
}

// assertMergeStateFileInvisibleToGit fails the test if `git status --porcelain` in dir mentions mergeStateFileName.
// That means saving the record produced a visible git-status entry.
// It does not require dir to be otherwise clean.
// A prime weft worktree can carry its own untracked scaffolding, such as the _lym junction target.
// That scaffolding is independent of the merge-state record.
func assertMergeStateFileInvisibleToGit(t *testing.T, label, dir string) {
	t.Helper()

	out := gitkit.GitStatusPorcelain(t, dir)
	if strings.Contains(out, "fabric-merge.json") {
		t.Errorf("git status --porcelain in %s worktree %s = %q; want no mention of fabric-merge.json (the merge-state record must be invisible to git)", label, dir, out)
	}
}

// TestMergeState_ForeignMergeStatePresent checks the true/false split.
// A plain-git conflicted merge staged directly in the warp checkout must report true.
// A clean pair must report false.
func TestMergeState_ForeignMergeStatePresent(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

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
	gitkit.CommitFile(t, warpPath, "conflict-target.txt", "branch content", "branch content commit")

	gitkit.MustRun(t, warpPath, "git", "checkout", "-q", "-")
	gitkit.CommitFile(t, warpPath, "conflict-target.txt", "main content", "main content commit")

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

	// The probe must leave the foreign state untouched, so MERGE_HEAD must still be present.
	statCmd := exec.Command("git", "rev-parse", "--verify", "--quiet", "MERGE_HEAD")
	statCmd.Dir = warpPath
	if err := statCmd.Run(); err != nil {
		t.Errorf("MERGE_HEAD missing after ForeignMergeStatePresentForTest() probe; want the foreign state left untouched (rev-parse error = %v)", err)
	}
}

// driveConflictedMergeStart builds a divergent "conflict-branch" in dir off the current HEAD.
// It then runs repo.MergeStart against it.
// The merge leaves dir with conflict markers present and the tracked worktree dirty.
// That is the state the reset under test exists to discard.
// It fails the test unless the merge is classified as conflicted.
// A non-conflicted outcome would make the reset assertions vacuous.
func driveConflictedMergeStart(t *testing.T, dir string, repo *gitrepo.Repo) {
	t.Helper()

	gitkit.MustRun(t, dir, "git", "checkout", "-q", "-b", "conflict-branch")
	gitkit.CommitFile(t, dir, "conflict-target.txt", "branch content", "branch content commit")

	gitkit.MustRun(t, dir, "git", "checkout", "-q", "-")
	gitkit.CommitFile(t, dir, "conflict-target.txt", "main content", "main content commit")

	outcome, err := repo.MergeStart("conflict-branch", false)
	if err != nil {
		t.Fatalf("MergeStart(conflict-branch) error = %v", err)
	}
	if outcome != gitrepo.MergeConflicted {
		t.Fatalf("MergeStart(conflict-branch) outcome = %v; want MergeConflicted", outcome)
	}
}

// wantWorktreeResetEntries asserts entries is exactly one KindWorktreeReset entry carrying the warp
// pre-merge SHA.
// The weft is never a reset target, so the warp reset is the record's only entry.
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

// TestMergeState_ResetMergeSides drives resetMergeSides on one fixture, running its steps serially.
// With the warp side left dirty by a real conflicted MergeStart, the reset restores warp HEAD.
// Warp HEAD returns to the captured pre-merge SHA, and the warp worktree ends clean.
// MergeHeadPresent is false on the warp, and the record holds exactly one KindWorktreeReset entry.
// The weft side is never dirtied in that case, so its unchanged HEAD check is trivially true.
// Against the hub's prime warp worktree with nothing to reset, the ownership gate must admit the reset.
// With the weft side left dirty by a real conflicted MergeStart, the warp-only reset restores warp HEAD alone.
// The weft's conflicted merge state stays exactly as it was:
// MERGE_HEAD still present, conflict markers still on disk.
// The weft step leaves its conflict in place, so it runs last.
func TestMergeState_ResetMergeSides(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	f, h := newMergeStateFixture(t)
	warpPath, weftPath := h.PrimeWorktree(), h.PrimeRecords()

	t.Run("warp side conflicted", func(t *testing.T) {
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
	})

	t.Run("prime warp worktree is admitted by the ownership gate", func(t *testing.T) {
		warpSHA := fabricengine.CurrentSHAForTest(t, warpPath)

		rec := fabricengine.NewMutations(h.Path)
		if err := fabricengine.ResetMergeSidesForTest(f, rec, warpSHA); err != nil {
			t.Fatalf("ResetMergeSidesForTest() against the prime pair error = %v; want the ownership gate to admit the prime warp worktree", err)
		}

		wantWorktreeResetEntries(t, rec.Snapshot().Entries(), warpSHA)
	})

	t.Run("weft side conflicted is left untouched", func(t *testing.T) {
		warpStartSHA := fabricengine.CurrentSHAForTest(t, warpPath)

		driveConflictedMergeStart(t, weftPath, fabricengine.WeftForTest(f))

		// Capture the weft HEAD after driveConflictedMergeStart, not before it.
		// That helper commits real content onto the checked-out branch before MergeStart,
		// so the weft HEAD has already moved off its pre-fixture SHA.
		// The reset must leave the weft where the conflict left it, not restore an earlier point.
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
	})
}
