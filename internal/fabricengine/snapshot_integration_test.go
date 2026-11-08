// snapshot_integration_test.go covers Fabric.snapshotWarpSHA against real weft and warp repositories.
// A lookup that matches nothing reads as absent, not as an error.
// Per-branch scoping and a dangling Warp-SHA are covered too.
// A back-dated merge commit tells the --topo-order walk from git log's default date order,
// and a RebuildIndex assertion covers the same history.
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

// TestSnapshotWarpSHA_UnbornWeftHEAD covers a weft repo with zero commits.
// It checks that the "does not have any commits yet" tolerance in scanWarpSHATrailers reads as absent, not as an error.
func TestSnapshotWarpSHA_UnbornWeftHEAD(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	warpPath := fabricengine.NewPlainWarpRepoForTest(t)
	weftPath := t.TempDir()
	gitkit.Git(t, weftPath, "init", "-q", "-b", "main")
	f := fabricengine.NewFabricForTest(t, warpPath, weftPath)

	got, err := fabricengine.SnapshotWarpSHAForTest(f, "raddle")
	if err != nil {
		t.Fatalf("snapshotWarpSHA() error = %v; want nil (unborn weft HEAD)", err)
	}
	if got != "" {
		t.Errorf("snapshotWarpSHA() = %q; want \"\" (unborn weft HEAD)", got)
	}
}

// TestSnapshotWarpSHA_PerBranchScoping records a tag on a side branch.
// It then switches the weft worktree with a plain `git checkout -b` to a branch forked before the tagged commit landed.
// It asserts that snapshotWarpSHA reads the tag as absent, since it scans only the weft worktree's current branch.
//
// Topology.Checkout switches warp and weft together, the production path to this state.
// This test needs the weft-side switch alone.
//
//testtiming:keep snapshotWarpSHA scanning only the weft worktree's current branch, so a tag on another branch reads absent; coverage of its blocks by other tests does not show an assertion of this
func TestSnapshotWarpSHA_PerBranchScoping(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	warpPath := fabricengine.NewPlainWarpRepoForTest(t)
	weftFixture := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	f := fabricengine.NewFabricForTest(t, warpPath, weftFixture.PrimeRecords())

	// The weft worktree's original branch is fabricengine.RecordsBranchName("main") ("main-weft"), never bare "main".
	// A real hub's own weft:main is a separate worktree (the Bolt checkout), so this file's own branch stays suffixed.
	// Fork "tagged" off it and record the Snapshot tag there, so the mainline
	// branch itself never advances past the fixture's initial commit.
	mainlineBranch := fabricengine.RecordsBranchName("main")
	gitkit.MustRun(t, weftFixture.PrimeRecords(), "git", "checkout", "-b", "tagged")
	commitWeftTagged(t, f, warpPath, weftFixture.PrimeRecords(), "tagged change", "raddle")

	// Fork "other" off the mainline branch, not off "tagged", so its history does not contain the tagged commit at all.
	// Then switch the weft worktree onto "other": it is the branch snapshotWarpSHA must now scan.
	gitkit.MustRun(t, weftFixture.PrimeRecords(), "git", "checkout", "-b", "other", mainlineBranch)

	got, err := fabricengine.SnapshotWarpSHAForTest(f, "raddle")
	if err != nil {
		t.Fatalf("snapshotWarpSHA() error = %v", err)
	}
	if got != "" {
		t.Errorf("snapshotWarpSHA() = %q; want \"\" (tag recorded on another branch must read as absent)", got)
	}
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

// TestSnapshotWarpSHA_TopologicalOrderBeatsCommitDate covers a history where date order and topological order disagree.
// Every other case in this file has a linear history, where date order and topological order coincide.
// Such a case passes with or without --topo-order, so it proves nothing about the flag.
//
// The fixture: a mainline commit carries the "raddle" tag at the current committer date.
// A "side" branch forks from that mainline commit, so the side commit is a topological descendant of it,
// and its own "raddle"-tagged commit is back-dated to the year 2000, as a skewed clock would produce.
// The side branch is merged back into mainline with --no-ff.
// Whatever its clock claims, the side commit is a descendant of the mainline commit.
// So --topo-order never lists it before that ancestor.
// A date-ordered scan gets this backwards and answers with the older mainline baseline, under-reporting staleness.
// The two assertions below run against this same history.
// The first checks that snapshotWarpSHA picks the topologically newest commit (the side one), not the date-newest.
// The second checks that RebuildIndex agrees with the incrementally built index over the same history.
// That shows the ordering change did not desynchronize the two.
//
// Two snapshot commits for the same tag can sit on concurrent branches, neither an ancestor of the other.
// So "newest" is a topological choice between incomparable commits, and either may be returned.
// Both are legitimate baselines.
// ChangedFilesSince against either reports a superset or an equal set of the truly changed files.
// Over-reporting is the safe direction, so no tie-break between concurrent branches is defined.
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
		t.Fatalf("snapshotWarpSHA() error = %v", err)
	}
	if got != warpSHASide {
		t.Errorf("snapshotWarpSHA(\"raddle\") = %q; want the topologically-newest (side) baseline %q, not the date-newest mainline baseline %q", got, warpSHASide, warpSHAMainline)
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

// TestSnapshotWarpSHA_DanglingWarpSHA_ReturnsRawWithSHAExistsFalse pins the reader's validate-at-use posture.
// A recorded Warp-SHA whose warp commit is later rewritten away is returned raw by snapshotWarpSHA, with a nil error.
// It is not collapsed to absent, and it is not resolved to an older baseline.
// f.code.SHAExists on the returned SHA reports false.
// This runs the read-then-check idiom that the doc comment of snapshotWarpSHA describes.
//
//testtiming:keep a recorded Warp-SHA whose warp commit was rewritten away being returned raw with SHAExists false; coverage of its blocks by other tests does not show an assertion of this
func TestSnapshotWarpSHA_DanglingWarpSHA_ReturnsRawWithSHAExistsFalse(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	warpPath := fabricengine.NewPlainWarpRepoForTest(t)
	weftFixture := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	f := fabricengine.NewFabricForTest(t, warpPath, weftFixture.PrimeRecords())

	baseWarpSHA := fabricengine.CurrentSHAForTest(t, warpPath)
	danglingWarpSHA, _ := commitWeftTagged(t, f, warpPath, weftFixture.PrimeRecords(), "will be rewritten away", "raddle")

	// Reset warp back to the base commit, so danglingWarpSHA no longer resolves.
	// A plain reset leaves the orphaned object resolvable through the reflog for a while.
	// Git must also forget it: expireAndPruneUnreachable in syncweft_integration_test.go does that.
	gitkit.MustRun(t, warpPath, "git", "reset", "--hard", baseWarpSHA)
	expireAndPruneUnreachable(t, warpPath)

	got, err := fabricengine.SnapshotWarpSHAForTest(f, "raddle")
	if err != nil {
		t.Fatalf("snapshotWarpSHA() error = %v; want nil (a dangling Warp-SHA is returned raw, not an error)", err)
	}
	if got != danglingWarpSHA {
		t.Errorf("snapshotWarpSHA() = %q; want the dangling SHA %q returned raw", got, danglingWarpSHA)
	}
	if fabricengine.WarpForTest(f).SHAExists(got) {
		t.Errorf("Warp.SHAExists(%q) = true; want false (the warp commit was rewritten away)", got)
	}
}
