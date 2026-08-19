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
