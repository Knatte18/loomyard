//go:build integration

// diff_integration_test.go — integration coverage for the unified
// Fabric.Diff and Fabric.Status added in diff.go: the exact-correspondence
// merge, the nearest-older weft-anchor bridge, the no-correspondence case,
// and Status's live worktree merge (including that it genuinely honors the
// weft repo's git-excluded artifacts, not merely asserts on an untested
// assumption). Package fabricengine_test, reusing export_test.go's fixture
// shims (NewPlainWarpRepoForTest, CommitWarpForTest, NewFabricForTest,
// CurrentSHAForTest, WriteWeftConfigContentForTest, SeedFabricConfigForTest)
// rather than redefining them.

package fabricengine_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/gitrepo"
)

// changeEntryPaths returns the Path of every entry in entries whose Side
// matches side, for assertion convenience.
func changeEntryPaths(entries []fabricengine.ChangeEntry, side fabricengine.ChangeSide) []string {
	var paths []string
	for _, e := range entries {
		if e.Side == side {
			paths = append(paths, e.Path)
		}
	}
	return paths
}

// containsPath reports whether want is present in paths.
func containsPath(paths []string, want string) bool {
	for _, p := range paths {
		if p == want {
			return true
		}
	}
	return false
}

// TestDiff_MergesWarpAndWeftSides covers the exact-correspondence path: two Commit rounds record
// correspondence for both warp SHAs,
// and diffing since the first must report both sides' changes made in the second round, correctly
// side-labelled.
func TestDiff_MergesWarpAndWeftSides(t *testing.T) {
	warpPath := fabricengine.NewPlainWarpRepoForTest(t)
	weftFixture := gitkit.CopyWeft(t)
	fabricengine.SeedFabricConfigForTest(t, warpPath)
	f := fabricengine.NewFabricForTest(t, warpPath, weftFixture.WeftPath)

	warpSHA1 := fabricengine.CommitWarpForTest(t, warpPath, "warp change 1")
	fabricengine.WriteWeftConfigContentForTest(t, weftFixture.WeftPath, "weft change 1")
	res1, err := f.Commit([]string{"_lyx"}, fabricengine.DefaultCommitMessage, nil, fabricengine.SyncOptions{})
	if err != nil || !res1.WeftCommitted {
		t.Fatalf("first Commit: %+v, err=%v", res1, err)
	}

	fabricengine.CommitWarpForTest(t, warpPath, "warp change 2")
	fabricengine.WriteWeftConfigContentForTest(t, weftFixture.WeftPath, "weft change 2")
	res2, err := f.Commit([]string{"_lyx"}, fabricengine.DefaultCommitMessage, nil, fabricengine.SyncOptions{})
	if err != nil || !res2.WeftCommitted {
		t.Fatalf("second Commit: %+v, err=%v", res2, err)
	}

	got, err := f.Diff(warpSHA1)
	if err != nil {
		t.Fatalf("Diff(%q) error = %v", warpSHA1, err)
	}
	if got.NoWeftCorrespondence {
		t.Errorf("Diff(%q) NoWeftCorrespondence = true; want false (exact correspondence exists)", warpSHA1)
	}

	warpPaths := changeEntryPaths(got.Entries, fabricengine.SideWarp)
	if !containsPath(warpPaths, "README") {
		t.Errorf("Diff(%q) warp-side paths = %v; want it to contain README", warpSHA1, warpPaths)
	}
	weftPaths := changeEntryPaths(got.Entries, fabricengine.SideWeft)
	if !containsPath(weftPaths, filepath.Join("_lyx", "config.yaml")) {
		t.Errorf("Diff(%q) weft-side paths = %v; want it to contain _lyx/config.yaml", warpSHA1, weftPaths)
	}
}

// TestDiff_NearestOlderAnchor_ResolvesToNearestOlderSyncedWeftBaseline covers the nearest-older
// bridge: a warp SHA advanced past the last synced round has no correspondence of its own, so Diff
// must anchor the weft side to the nearest older synced weft SHA — proven by a manual, unrecorded
// weft commit made ahead of that baseline showing up in the weft-side result, rather than the weft
// side coming back empty (which is what a strictly exact-only anchor would produce).
func TestDiff_NearestOlderAnchor_ResolvesToNearestOlderSyncedWeftBaseline(t *testing.T) {
	warpPath := fabricengine.NewPlainWarpRepoForTest(t)
	weftFixture := gitkit.CopyWeft(t)
	fabricengine.SeedFabricConfigForTest(t, warpPath)
	f := fabricengine.NewFabricForTest(t, warpPath, weftFixture.WeftPath)

	fabricengine.WriteWeftConfigContentForTest(t, weftFixture.WeftPath, "weft change 1")
	res1, err := f.Commit([]string{"_lyx"}, fabricengine.DefaultCommitMessage, nil, fabricengine.SyncOptions{})
	if err != nil || !res1.WeftCommitted {
		t.Fatalf("Commit: %+v, err=%v", res1, err)
	}

	// A further warp commit with no corresponding weft sync at all.
	warpSHA2 := fabricengine.CommitWarpForTest(t, warpPath, "warp change 2, never synced")

	// A manual weft commit made after the last synced baseline but never
	// recorded in the correspondence index — proof that the anchor genuinely
	// resolved to the nearest older synced weft SHA (res1.WeftSHA), not an
	// empty/absent one: only a real anchor makes ChangedFilesSince surface
	// this file.
	manualPath := filepath.Join(weftFixture.WeftPath, "manual-ahead.txt")
	if err := os.WriteFile(manualPath, []byte("weft change 2, manual, not recorded"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	gitkit.MustRun(t, weftFixture.WeftPath, "git", "add", ".")
	gitkit.MustRun(t, weftFixture.WeftPath, "git", "commit", "-q", "-m", "manual weft commit ahead of last sync")

	got, err := f.Diff(warpSHA2)
	if err != nil {
		t.Fatalf("Diff(%q) error = %v", warpSHA2, err)
	}
	if got.NoWeftCorrespondence {
		t.Errorf("Diff(%q) NoWeftCorrespondence = true; want false (nearest-older correspondence exists)", warpSHA2)
	}

	weftPaths := changeEntryPaths(got.Entries, fabricengine.SideWeft)
	if !containsPath(weftPaths, "manual-ahead.txt") {
		t.Errorf("Diff(%q) weft-side paths = %v; want it to contain manual-ahead.txt (the nearest-older baseline resolved, not empty)", warpSHA2, weftPaths)
	}
}

// TestDiff_NoWeftCorrespondence_BeforeFirstSync covers the no-correspondence case: a sinceWarpSHA
// older than the first weft commit — no entry exists at or before it — must not be an error;
// it must report warp entries, empty weft entries, and NoWeftCorrespondence = true.
func TestDiff_NoWeftCorrespondence_BeforeFirstSync(t *testing.T) {
	warpPath := fabricengine.NewPlainWarpRepoForTest(t)
	initialWarpSHA := fabricengine.CurrentSHAForTest(t, warpPath)
	weftFixture := gitkit.CopyWeft(t)
	fabricengine.SeedFabricConfigForTest(t, warpPath)
	f := fabricengine.NewFabricForTest(t, warpPath, weftFixture.WeftPath)

	// Advance warp and synchronize weft — this records correspondence for
	// the NEW warp commit, never for initialWarpSHA, which predates any
	// recorded correspondence entirely.
	fabricengine.CommitWarpForTest(t, warpPath, "warp change")
	fabricengine.WriteWeftConfigContentForTest(t, weftFixture.WeftPath, "weft change")
	res, err := f.Commit([]string{"_lyx"}, fabricengine.DefaultCommitMessage, nil, fabricengine.SyncOptions{})
	if err != nil || !res.WeftCommitted {
		t.Fatalf("Commit: %+v, err=%v", res, err)
	}

	got, err := f.Diff(initialWarpSHA)
	if err != nil {
		t.Fatalf("Diff(%q) error = %v; want nil (no correspondence is not a failure)", initialWarpSHA, err)
	}
	if !got.NoWeftCorrespondence {
		t.Errorf("Diff(%q) NoWeftCorrespondence = false; want true", initialWarpSHA)
	}
	if len(changeEntryPaths(got.Entries, fabricengine.SideWeft)) != 0 {
		t.Errorf("Diff(%q) weft-side entries = %v; want empty", initialWarpSHA, changeEntryPaths(got.Entries, fabricengine.SideWeft))
	}
	warpPaths := changeEntryPaths(got.Entries, fabricengine.SideWarp)
	if !containsPath(warpPaths, "README") {
		t.Errorf("Diff(%q) warp-side paths = %v; want it to contain README", initialWarpSHA, warpPaths)
	}
}

// TestStatus_MergesUncommittedChangesBothSides_ExcludesWeftArtifacts covers Fabric.Status's live
// worktree merge: uncommitted changes on both sides must be reported with correct side labels,
// and the weft side must never surface fabric's own git-excluded artifacts (the .weft/ lock dir and
// the gitrepo push lock file) — a regression guard over the verified go-git exclude behavior
// WorktreeChangedFiles' doc comment documents, not an unexercised assumption.
// The push lock file is created explicitly here because CommitWeft never writes it — only
// PushCoalesced does — so leaving it out would make the exclude assertion vacuous.
func TestStatus_MergesUncommittedChangesBothSides_ExcludesWeftArtifacts(t *testing.T) {
	warpPath := fabricengine.NewPlainWarpRepoForTest(t)
	weftFixture := gitkit.CopyWeft(t)
	f := fabricengine.NewFabricForTest(t, warpPath, weftFixture.WeftPath)

	// One CommitWeft round first, so ensureWeftLockDir has already seeded
	// the weft repo's .git/info/exclude with both artifact patterns before
	// Status is asked to filter them out.
	fabricengine.CommitWarpForTest(t, warpPath, "warp change")
	fabricengine.WriteWeftConfigContentForTest(t, weftFixture.WeftPath, "weft change")
	if _, committed, err := fabricengine.CommitWeftForTest(f, []string{"_lyx"}, fabricengine.DefaultCommitMessage, fabricengine.SyncOptions{}); err != nil || !committed {
		t.Fatalf("commitWeft() committed=%v err=%v; want committed=true, err=nil", committed, err)
	}

	// Dirty both worktrees with genuinely uncommitted changes.
	warpDirtyPath := filepath.Join(warpPath, "uncommitted.txt")
	if err := os.WriteFile(warpDirtyPath, []byte("warp dirty"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	fabricengine.WriteWeftConfigContentForTest(t, weftFixture.WeftPath, "weft dirty, uncommitted")

	// CommitWeft never creates the push lock file — only PushCoalesced
	// does — so it must be created explicitly to genuinely exercise the
	// exclude mechanism rather than assert on an absent file.
	pushLockPath := filepath.Join(weftFixture.WeftPath, gitrepo.PushLockFileName)
	if err := os.WriteFile(pushLockPath, nil, 0o644); err != nil {
		t.Fatalf("WriteFile(push lock): %v", err)
	}

	entries, err := f.Status()
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}

	warpPaths := changeEntryPaths(entries, fabricengine.SideWarp)
	weftPaths := changeEntryPaths(entries, fabricengine.SideWeft)

	if !containsPath(warpPaths, "uncommitted.txt") {
		t.Errorf("Status() warp-side paths = %v; want it to contain uncommitted.txt", warpPaths)
	}
	if !containsPath(weftPaths, filepath.Join("_lyx", "config.yaml")) {
		t.Errorf("Status() weft-side paths = %v; want it to contain _lyx/config.yaml", weftPaths)
	}

	// Neither the lock dir nor the push lock file (both git-excluded) may
	// appear on the weft side. Scoped to weft only, per this batch's
	// documented known limitation: the warp/warp side deliberately does not
	// seed these excludes.
	for _, p := range weftPaths {
		if strings.HasPrefix(p, fabricengine.WeftLockDirNameForTest+"/") || p == gitrepo.PushLockFileName {
			t.Errorf("Status() weft-side paths = %v; want neither %s/ nor %s (both git-excluded)", weftPaths, fabricengine.WeftLockDirNameForTest, gitrepo.PushLockFileName)
		}
	}
}
