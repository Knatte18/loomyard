// diff_integration_test.go holds the integration tests for Fabric.Diff and Fabric.Status, which diff.go defines.
// It covers the exact-correspondence merge and the nearest-older weft-anchor bridge.
// It also covers Status's live worktree merge, which honours the weft repo's git-excluded artifacts.

package fabricengine_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// changeEntryPaths returns the Path of every entry in entries whose Side equals side, for test assertions.
func changeEntryPaths(entries []fabricengine.ChangeEntry, side fabricengine.ChangeSide) []string {
	var paths []string
	for _, e := range entries {
		if e.Side == side {
			paths = append(paths, e.Path)
		}
	}
	return paths
}

// TestDiff_MergesWarpAndWeftSides covers the exact-correspondence path.
// Two Commit rounds record correspondence for both warp SHAs.
// Diff since the first round must report both sides' changes made in the second round, each side-labelled.
func TestDiff_MergesWarpAndWeftSides(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	warpPath := fabricengine.NewPlainWarpRepoForTest(t)
	weftFixture := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	fabricengine.SeedFabricConfigForTest(t, warpPath)
	f := fabricengine.NewFabricForTest(t, warpPath, weftFixture.PrimeRecords())

	warpSHA1 := fabricengine.CommitWarpForTest(t, warpPath, "warp change 1")
	fabricengine.WriteWeftConfigContentForTest(t, weftFixture.PrimeRecords(), "weft change 1")
	res1, err := f.Commit([]string{"_lym"}, fabricengine.DefaultCommitMessage, nil, fabricengine.SyncOptions{})
	if err != nil || !res1.WeftCommitted {
		t.Fatalf("first Commit: %+v, err=%v", res1, err)
	}

	fabricengine.CommitWarpForTest(t, warpPath, "warp change 2")
	fabricengine.WriteWeftConfigContentForTest(t, weftFixture.PrimeRecords(), "weft change 2")
	res2, err := f.Commit([]string{"_lym"}, fabricengine.DefaultCommitMessage, nil, fabricengine.SyncOptions{})
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
	if !slices.Contains(warpPaths, "README") {
		t.Errorf("Diff(%q) warp-side paths = %v; want it to contain README", warpSHA1, warpPaths)
	}
	weftPaths := changeEntryPaths(got.Entries, fabricengine.SideWeft)
	if !slices.Contains(weftPaths, filepath.Join("_lym", "config.yaml")) {
		t.Errorf("Diff(%q) weft-side paths = %v; want it to contain _lym/config.yaml", warpSHA1, weftPaths)
	}
}

// TestDiff_NearestOlderAnchor_ResolvesToNearestOlderSyncedWeftBaseline covers the nearest-older anchor.
// A warp SHA advanced past the last synced round has no correspondence of its own.
// So Diff anchors the weft side to the nearest older synced weft SHA.
// The test makes a manual, unrecorded weft commit ahead of that baseline,
// and the weft side of the result must show it rather than come back empty.
func TestDiff_NearestOlderAnchor_ResolvesToNearestOlderSyncedWeftBaseline(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	warpPath := fabricengine.NewPlainWarpRepoForTest(t)
	weftFixture := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	fabricengine.SeedFabricConfigForTest(t, warpPath)
	f := fabricengine.NewFabricForTest(t, warpPath, weftFixture.PrimeRecords())

	fabricengine.WriteWeftConfigContentForTest(t, weftFixture.PrimeRecords(), "weft change 1")
	res1, err := f.Commit([]string{"_lym"}, fabricengine.DefaultCommitMessage, nil, fabricengine.SyncOptions{})
	if err != nil || !res1.WeftCommitted {
		t.Fatalf("Commit: %+v, err=%v", res1, err)
	}

	// This warp commit is never synced to weft, so no correspondence entry records it.
	warpSHA2 := fabricengine.CommitWarpForTest(t, warpPath, "warp change 2, never synced")

	// This manual weft commit lands after the last synced baseline and is never recorded in the correspondence index.
	// The file reaches the diff only when the anchor resolves to the nearest older synced weft SHA, `res1.WeftSHA`.
	manualPath := filepath.Join(weftFixture.PrimeRecords(), "manual-ahead.txt")
	if err := os.WriteFile(manualPath, []byte("weft change 2, manual, not recorded"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	gitkit.MustRun(t, weftFixture.PrimeRecords(), "git", "add", ".")
	gitkit.MustRun(t, weftFixture.PrimeRecords(), "git", "commit", "-q", "-m", "manual weft commit ahead of last sync")

	got, err := f.Diff(warpSHA2)
	if err != nil {
		t.Fatalf("Diff(%q) error = %v", warpSHA2, err)
	}
	if got.NoWeftCorrespondence {
		t.Errorf("Diff(%q) NoWeftCorrespondence = true; want false (nearest-older correspondence exists)", warpSHA2)
	}

	weftPaths := changeEntryPaths(got.Entries, fabricengine.SideWeft)
	if !slices.Contains(weftPaths, "manual-ahead.txt") {
		t.Errorf("Diff(%q) weft-side paths = %v; want it to contain manual-ahead.txt (the nearest-older baseline resolved, not empty)", warpSHA2, weftPaths)
	}
}

// TestStatus_MergesUncommittedChangesBothSides_ExcludesWeftArtifacts covers Fabric.Status's live worktree merge.
// Uncommitted changes on both sides must be reported with the correct side labels.
// The weft side must never surface fabric's git-excluded artifacts:
// the .weft/ lock dir and the gitrepo push lock file.
// Only `PushCoalesced` writes the push lock file, so the test creates it explicitly;
// leaving it out would make the exclusion assertion vacuous.
// It also covers Fabric.DirtyPaths: both sides' modified tracked paths come back absolute under their checkouts,
// and an untracked file does not.
// Fabric.CodeDirtyPaths returns the code checkout and only its tracked path, relative to it.
func TestStatus_MergesUncommittedChangesBothSides_ExcludesWeftArtifacts(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	warpPath := fabricengine.NewPlainWarpRepoForTest(t)
	weftFixture := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	f := fabricengine.NewFabricForTest(t, warpPath, weftFixture.PrimeRecords())

	// One weft commit round runs first, so `ensureWeftLockDir` has already seeded both artifact patterns;
	// they sit in the weft repo's `.git/info/exclude` before `Status` filters them out.
	fabricengine.CommitWarpForTest(t, warpPath, "warp change")
	fabricengine.WriteWeftConfigContentForTest(t, weftFixture.PrimeRecords(), "weft change")
	if _, committed, err := fabricengine.CommitWeftForTest(f, []string{"_lym"}, fabricengine.DefaultCommitMessage, fabricengine.SyncOptions{}); err != nil || !committed {
		t.Fatalf("commitWeft() committed=%v err=%v; want committed=true, err=nil", committed, err)
	}

	// Dirty both worktrees with uncommitted changes.
	warpDirtyPath := filepath.Join(warpPath, "uncommitted.txt")
	if err := os.WriteFile(warpDirtyPath, []byte("warp dirty"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	fabricengine.WriteWeftConfigContentForTest(t, weftFixture.PrimeRecords(), "weft dirty, uncommitted")

	// `commitWeft` never creates the push lock file; only `PushCoalesced` does.
	// The file lives in the records git dir, so the worktree listing must not show it.
	pushLockPath, err := gitrepo.New(weftFixture.PrimeRecords()).PushLockPath()
	if err != nil {
		t.Fatalf("PushLockPath() error = %v", err)
	}
	if err := os.WriteFile(pushLockPath, nil, 0o644); err != nil {
		t.Fatalf("WriteFile(push lock): %v", err)
	}

	// A tracked file modified on the warp side, so DirtyPaths has a tracked path there beside the untracked one.
	warpTrackedPath := filepath.Join(warpPath, "README")
	if err := os.WriteFile(warpTrackedPath, []byte("warp tracked, modified"), 0o644); err != nil {
		t.Fatalf("WriteFile(README): %v", err)
	}

	dirty, err := f.DirtyPaths()
	if err != nil {
		t.Fatalf("DirtyPaths() error = %v", err)
	}
	weftTrackedPath := filepath.Join(weftFixture.PrimeRecords(), "_lym", "config.yaml")
	if want := []string{warpTrackedPath, weftTrackedPath}; !slices.Equal(dirty, want) {
		t.Errorf("DirtyPaths() = %v; want exactly the modified tracked paths %v, with the untracked %s and %s omitted", dirty, want, warpDirtyPath, pushLockPath)
	}

	checkout, codePaths, err := f.CodeDirtyPaths()
	if err != nil {
		t.Fatalf("CodeDirtyPaths() error = %v", err)
	}
	gotCheckout, err := filepath.EvalSymlinks(checkout)
	if err != nil {
		t.Fatalf("EvalSymlinks(%q): %v", checkout, err)
	}
	wantCheckout, err := filepath.EvalSymlinks(warpPath)
	if err != nil {
		t.Fatalf("EvalSymlinks(%q): %v", warpPath, err)
	}
	if gotCheckout != wantCheckout {
		t.Errorf("CodeDirtyPaths() checkout = %q; want the code checkout %q", gotCheckout, wantCheckout)
	}
	if want := []string{"README"}; !slices.Equal(codePaths, want) {
		t.Errorf("CodeDirtyPaths() paths = %v; want exactly the code side's tracked path %v", codePaths, want)
	}

	entries, err := f.Status()
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}

	warpPaths := changeEntryPaths(entries, fabricengine.SideWarp)
	weftPaths := changeEntryPaths(entries, fabricengine.SideWeft)

	if !slices.Contains(warpPaths, "uncommitted.txt") {
		t.Errorf("Status() warp-side paths = %v; want it to contain uncommitted.txt", warpPaths)
	}

	codeChanges, err := f.CodeWorktreeChanges()
	if err != nil {
		t.Fatalf("CodeWorktreeChanges() error = %v", err)
	}
	if !slices.Equal(slices.Sorted(slices.Values(codeChanges)), slices.Sorted(slices.Values(warpPaths))) {
		t.Errorf("CodeWorktreeChanges() = %v; want exactly the code side's paths of Status(), %v", codeChanges, warpPaths)
	}
	if !slices.Contains(weftPaths, filepath.Join("_lym", "config.yaml")) {
		t.Errorf("Status() weft-side paths = %v; want it to contain _lym/config.yaml", weftPaths)
	}

	// Neither the git-excluded lock dir nor the push lock in the git dir may appear in the weft listing.
	// The warp side does not seed these excludes, so the check covers only the weft side.
	for _, p := range weftPaths {
		if strings.HasPrefix(p, fabricengine.WeftLockDirNameForTest+"/") || filepath.Base(p) == gitrepo.PushLockFileName {
			t.Errorf("Status() weft-side paths = %v; want neither %s/ nor any %s entry", weftPaths, fabricengine.WeftLockDirNameForTest, gitrepo.PushLockFileName)
		}
	}
}
