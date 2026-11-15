// snapshot_integration_test.go covers the weft trailer scan behind RebuildIndex and the snapshot tag lookup.
// It runs against real repositories.
// A back-dated merge commit gives it a history where date order and topological order disagree.
// An unborn weft HEAD reads as an empty history.
// One warp SHA recorded by two weft commits covers the last-recorded-wins rule.
// The file is package fabricengine_test.
// It reuses the fixture shims in export_test.go and expireAndPruneUnreachable from syncweft_integration_test.go.

package fabricengine_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// commitWeftTagged advances the warp repo by one commit.
// It writes new content to the weft worktree's tracked _lym/config.yaml.
// It then lands the commit through f.CommitWeft with the tags.
// This file's tests build their Snapshot: trailer histories that way.
// It returns the new warp and weft HEAD SHAs.
// Passing zero tags makes a plain, untagged weft commit.
func commitWeftTagged(t *testing.T, f *fabricengine.Fabric, warpPath, weftPath, content string, tags ...string) (warpSHA, weftSHA string) {
	t.Helper()

	warpSHA = fabricengine.CommitWarpForTest(t, warpPath, content)
	fabricengine.WriteWeftConfigContentForTest(t, weftPath, content)
	weftSHA, committed, err := fabricengine.CommitWeftForTest(f, []string{"_lym"}, fabricengine.DefaultCommitMessage, fabricengine.SyncOptions{}, tags...)
	if err != nil {
		t.Fatalf("commitWeft(tags=%v) error = %v", tags, err)
	}
	if !committed {
		t.Fatalf("commitWeft(tags=%v) committed = false; want true", tags)
	}
	return warpSHA, weftSHA
}

// commitWeftTaggedWithDate commits content into weftPath's tracked _lym/<file> at a caller-chosen GIT_COMMITTER_DATE.
// The commit carries a Warp-SHA trailer for warp's current HEAD (advanced first) and one Snapshot trailer per tag.
// The file name is caller-chosen.
// A side-branch commit picks a path no mainline commit touches, so the merge is conflict-free.
// It calls f.RecordCorrespondence itself, rather than committing through CommitWeft and amending the date afterward,
// so no earlier correspondence entry is left to clean up before the equivalence assertion runs.
//
// The raw `git commit` call is a bare exec.Command.
// gitexec.RunGit and gitkit.MustRun accept no caller-supplied environment.
// cmd.Env is scoped to this one invocation: it is appended onto os.Environ(), never replacing it.
// A process-wide os.Setenv(GIT_COMMITTER_DATE, ...) would leak into other t.Parallel() tests committing at that moment.
// Appending onto os.Environ() keeps the hermetic git config from gitkit.HermeticGitEnv(),
// so this commit stays hermetic too.
func commitWeftTaggedWithDate(t *testing.T, f *fabricengine.Fabric, warpPath, weftPath, file, content, committerDate string, tags ...string) (warpSHA, weftSHA string) {
	t.Helper()

	warpSHA = fabricengine.CommitWarpForTest(t, warpPath, content)

	filePath := filepath.Join(weftPath, "_lym", file)
	if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	gitkit.Git(t, weftPath, "add", ".")

	msg := fabricengine.AppendWarpSHATrailerForTest(fabricengine.DefaultCommitMessage, warpSHA)
	msg, err := fabricengine.AppendSnapshotTrailersForTest(msg, tags)
	if err != nil {
		t.Fatalf("appendSnapshotTrailers() error = %v", err)
	}

	cmd := exec.Command("git", "commit", "-q", "-m", msg)
	cmd.Dir = weftPath
	cmd.Env = append(os.Environ(), "GIT_COMMITTER_DATE="+committerDate)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git commit (backdated, dir=%s): %v; output: %s", weftPath, err, output)
	}

	weftSHA = fabricengine.CurrentSHAForTest(t, weftPath)
	if err := f.RecordCorrespondence(warpSHA, weftSHA); err != nil {
		t.Fatalf("RecordCorrespondence() error = %v", err)
	}
	return warpSHA, weftSHA
}

// TestRebuildIndex_UnbornWeftHEAD covers a weft repo with zero commits.
// It checks that the unborn-HEAD tolerance in the trailer scan reads as an empty history, so the rebuild succeeds.
func TestRebuildIndex_UnbornWeftHEAD(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	warpPath := fabricengine.NewPlainWarpRepoForTest(t)
	weftPath := t.TempDir()
	gitkit.Git(t, weftPath, "init", "-q", "-b", "main")
	f := fabricengine.NewFabricForTest(t, warpPath, weftPath)

	if err := f.RebuildIndex(); err != nil {
		t.Fatalf("RebuildIndex() error = %v; want nil (unborn weft HEAD)", err)
	}
}

// TestSnapshotWarpSHA_TopologicalOrderBeatsCommitDate covers a history where date order and topological order disagree.
// A mainline commit carries the "raddle" tag at the current committer date.
// A "side" branch forks from that mainline commit, so the side commit is a topological descendant of it,
// and its own "raddle"-tagged commit is back-dated to the year 2000, as a skewed clock would produce.
// The side branch is merged back into mainline with --no-ff.
// The trailer scan must answer with the topologically newest baseline (the side one), not the date-newest.
// A date-ordered scan answers with the older mainline baseline, under-reporting staleness.
// RebuildIndex must also agree, entry by entry, with the incrementally built index over this history.
//
//testtiming:keep a back-dated side commit merged back resolving to the topologically newest baseline, with RebuildIndex agreeing with the incremental index; coverage of its blocks by other tests does not show an assertion of this
func TestSnapshotWarpSHA_TopologicalOrderBeatsCommitDate(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	warpPath := fabricengine.NewPlainWarpRepoForTest(t)
	weftFixture := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	f := fabricengine.NewFabricForTest(t, warpPath, weftFixture.PrimeRecords())

	// A real hub's weft primary checks out the suffixed branch (fabricengine.RecordsBranchName("main")), never bare "main".
	// Bare "main" is already checked out by the hub's Bolt checkout worktree.
	// Switching this worktree onto it would collide with that worktree instead of reaching the primary's own mainline.
	mainlineBranch := fabricengine.RecordsBranchName("main")
	warpSHAMainline, _ := commitWeftTagged(t, f, warpPath, weftFixture.PrimeRecords(), "mainline tagged", "raddle")

	gitkit.MustRun(t, weftFixture.PrimeRecords(), "git", "checkout", "-b", "side")
	warpSHASide, _ := commitWeftTaggedWithDate(t, f, warpPath, weftFixture.PrimeRecords(), "side-marker.txt", "side tagged (back-dated)", "2000-01-01T00:00:00+0000", "raddle")

	gitkit.MustRun(t, weftFixture.PrimeRecords(), "git", "checkout", mainlineBranch)
	gitkit.MustRun(t, weftFixture.PrimeRecords(), "git", "merge", "--no-ff", "-m", "merge side into main", "side")

	got, err := fabricengine.SnapshotWarpSHAForTest(f, "raddle")
	if err != nil {
		t.Fatalf("SnapshotWarpSHAForTest() error = %v", err)
	}
	if got != warpSHASide {
		t.Errorf("SnapshotWarpSHAForTest(\"raddle\") = %q; want the topologically-newest (side) baseline %q, not the date-newest mainline baseline %q", got, warpSHASide, warpSHAMainline)
	}

	path, err := fabricengine.CorrIndexPathForTest(f)
	if err != nil {
		t.Fatalf("corrIndexPath() error = %v", err)
	}
	incremental, err := fabricengine.LoadCorrIndexForTest(path)
	if err != nil {
		t.Fatalf("loadCorrIndex() error = %v", err)
	}
	wantEntries := fabricengine.CorrIndexEntriesForTest(incremental)
	if len(wantEntries) != 2 {
		t.Fatalf("incrementally-built index has %d entries; want 2", len(wantEntries))
	}

	if err := f.RebuildIndex(); err != nil {
		t.Fatalf("RebuildIndex() error = %v", err)
	}
	rebuilt, err := fabricengine.LoadCorrIndexForTest(path)
	if err != nil {
		t.Fatalf("loadCorrIndex() (post-rebuild) error = %v", err)
	}
	gotEntries := fabricengine.CorrIndexEntriesForTest(rebuilt)

	if len(gotEntries) != len(wantEntries) {
		t.Fatalf("RebuildIndex() entries = %d; want %d", len(gotEntries), len(wantEntries))
	}
	for i := range wantEntries {
		if gotEntries[i] != wantEntries[i] {
			t.Errorf("entries[%d]: RebuildIndex()=%+v incremental=%+v; want equal", i, gotEntries[i], wantEntries[i])
		}
	}
}

// treeSHA resolves rev's tree object SHA in repoPath via `git rev-parse <rev>^{tree}`.
// It lets two commits be compared by their trees, independent of their commit SHAs.
func treeSHA(t *testing.T, repoPath, rev string) string {
	t.Helper()

	cmd := exec.Command("git", "rev-parse", rev+"^{tree}")
	cmd.Dir = repoPath
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git rev-parse %s^{tree} in %s: %v", rev, repoPath, err)
	}
	return strings.TrimSpace(string(out))
}

// TestWeftSHAForWarpSHA_CorrespondenceOverwrite_EmptyCommitWins covers one warp SHA recorded by two weft commits:
// a content commit under "raddle", then a tags-only commit at the same unchanged warp HEAD under the same tag.
// Three things are asserted.
// First, WeftSHAForWarpSHA resolves to the newer, empty commit, so the last recorded entry wins.
// Second, RebuildIndex's full trailer rescan agrees with the incrementally built index over the same history.
// The comparison is an entry-by-entry check of the two loaded indexes.
// Third, the empty commit's tree is byte-identical to the content commit's.
// So resolving through the overwritten entry restores the same weft state either commit would have.
// That makes the overwrite benign, not merely tolerated.
//
//testtiming:keep a warp SHA recorded by a content commit and a tags-only commit resolving to the newer empty commit with identical trees and an agreeing rebuild; coverage of its blocks by other tests does not show an assertion of this
func TestWeftSHAForWarpSHA_CorrespondenceOverwrite_EmptyCommitWins(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	warpPath := fabricengine.NewPlainWarpRepoForTest(t)
	weftFixture := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	f := fabricengine.NewFabricForTest(t, warpPath, weftFixture.PrimeRecords())

	warpSHA, contentWeftSHA := commitWeftTagged(t, f, warpPath, weftFixture.PrimeRecords(), "content commit", "raddle")

	// Make a tags-only call (nil pathspec) at the same warp HEAD, without advancing warp.
	// The empty-commit rule takes its `!positive` fall-through.
	// RecordCorrespondence then upserts over the entry the content commit wrote for warpSHA.
	emptyWeftSHA, committed, err := fabricengine.CommitWeftForTest(f, nil, fabricengine.DefaultCommitMessage, fabricengine.SyncOptions{}, "raddle")
	if err != nil {
		t.Fatalf("commitWeft() (tags-only, same warp HEAD) error = %v", err)
	}
	if !committed || emptyWeftSHA == "" {
		t.Fatalf("commitWeft() = (%q, %v); want an empty commit to have landed", emptyWeftSHA, committed)
	}
	if emptyWeftSHA == contentWeftSHA {
		t.Fatalf("commitWeft() landed the same SHA as the content commit; want a distinct empty commit")
	}

	got, err := f.WeftSHAForWarpSHA(warpSHA)
	if err != nil {
		t.Fatalf("WeftSHAForWarpSHA() error = %v", err)
	}
	if got != emptyWeftSHA {
		t.Errorf("WeftSHAForWarpSHA(%q) = %q; want the newer empty commit %q (last recorded wins)", warpSHA, got, emptyWeftSHA)
	}

	path, err := fabricengine.CorrIndexPathForTest(f)
	if err != nil {
		t.Fatalf("corrIndexPath() error = %v", err)
	}
	incremental, err := fabricengine.LoadCorrIndexForTest(path)
	if err != nil {
		t.Fatalf("loadCorrIndex() error = %v", err)
	}
	wantEntries := fabricengine.CorrIndexEntriesForTest(incremental)

	if err := f.RebuildIndex(); err != nil {
		t.Fatalf("RebuildIndex() error = %v", err)
	}
	rebuilt, err := fabricengine.LoadCorrIndexForTest(path)
	if err != nil {
		t.Fatalf("loadCorrIndex() (post-rebuild) error = %v", err)
	}
	gotEntries := fabricengine.CorrIndexEntriesForTest(rebuilt)

	if len(gotEntries) != len(wantEntries) {
		t.Fatalf("RebuildIndex() entries = %d; want %d", len(gotEntries), len(wantEntries))
	}
	for i := range wantEntries {
		if gotEntries[i] != wantEntries[i] {
			t.Errorf("entries[%d]: RebuildIndex()=%+v incremental=%+v; want equal", i, gotEntries[i], wantEntries[i])
		}
	}

	gotTree := treeSHA(t, weftFixture.PrimeRecords(), emptyWeftSHA)
	wantTree := treeSHA(t, weftFixture.PrimeRecords(), contentWeftSHA)
	if gotTree != wantTree {
		t.Errorf("empty commit tree = %q; want it identical to the content commit's tree %q (the overwrite is benign because both resolve to the same weft state)", gotTree, wantTree)
	}
}
