//go:build integration

// snapshot_integration_test.go — integration coverage for
// Fabric.snapshotWarpSHA: newest-tagged-commit-wins, tag isolation, the
// multi-tag-on-one-commit split, the absent-is-not-an-error miss path, the
// unborn-weft-HEAD tolerance, untagged/no-baseline commits, a Snapshot
// trailer with no Warp-SHA sibling, byte-exact tag matching, per-branch
// scoping, and (card 13) the one fixture that can actually discriminate
// card 10's --topo-order change from git log's date-ordered default: a
// back-dated merge commit, plus a RebuildIndex equivalence assertion on the
// same history. Package fabricengine (internal), reusing
// index_integration_test.go's newPlainWarpRepo, currentSHA, commitWarp,
// commitWeftWithTrailer, and newFabric helpers rather than building a
// parallel harness — they share this package.

package fabricengine

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/lyxtest"
)

// commitWeftTagged advances the warp repo by one commit, writes new content
// into the weft worktree's tracked _lyx/config.yaml, and lands it through
// f.CommitWeft carrying tags — the standard way this file's tests build a
// trailer history with Snapshot: entries. Returns the new warp and weft HEAD
// SHAs. Passing zero tags produces a plain, untagged weft commit.
func commitWeftTagged(t *testing.T, f *Fabric, warpPath, weftPath, content string, tags ...string) (warpSHA, weftSHA string) {
	t.Helper()

	warpSHA = commitWarp(t, warpPath, content)
	writeWeftConfigContent(t, weftPath, content)
	weftSHA, committed, err := f.commitWeft([]string{"_lyx"}, DefaultCommitMessage, SyncOptions{}, tags...)
	if err != nil {
		t.Fatalf("commitWeft(tags=%v) error = %v", tags, err)
	}
	if !committed {
		t.Fatalf("commitWeft(tags=%v) committed = false; want true", tags)
	}
	return warpSHA, weftSHA
}

// commitWeftSnapshotOnlyTrailer commits content into weftPath's tracked
// _lyx/config.yaml with a hand-crafted commit message carrying ONLY a
// "Snapshot: <tag>" trailer and no "Warp-SHA:" trailer at all — the shape
// snapshotWarpSHA's reader must skip rather than mistake for an empty
// baseline. Built directly via git rather than through CommitWeft, since
// CommitWeft only ever appends a Snapshot trailer alongside a Warp-SHA one
// (and drops tags entirely on an unborn warp HEAD — see commitWeftLocked),
// so there is no CommitWeft call that produces this shape on its own.
func commitWeftSnapshotOnlyTrailer(t *testing.T, weftPath, content, tag string) string {
	t.Helper()

	configPath := filepath.Join(weftPath, "_lyx", "config.yaml")
	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	lyxtest.MustRun(t, weftPath, "git", "add", ".")
	msg := "weft sync\n\n" + SnapshotTrailerKey + ": " + tag
	lyxtest.MustRun(t, weftPath, "git", "commit", "-q", "-m", msg)
	return currentSHA(t, weftPath)
}

// TestSnapshotWarpSHA_Miss is the TDD candidate for this card: a tag never recorded anywhere in
// history must resolve as absent — ("", nil), not an error — pinning the absent-is-not-an-error
// decision that lets a first-ever consumer run read "no baseline, generate everything" with no
// special-casing.
func TestSnapshotWarpSHA_Miss(t *testing.T) {
	t.Parallel()

	warpPath := newPlainWarpRepo(t)
	weftFixture := lyxtest.CopyWeft(t)
	f := newFabric(t, warpPath, weftFixture.WeftPath)

	commitWeftTagged(t, f, warpPath, weftFixture.WeftPath, "untagged change")

	got, err := f.snapshotWarpSHA("never-recorded")
	if err != nil {
		t.Fatalf("snapshotWarpSHA() error = %v; want nil", err)
	}
	if got != "" {
		t.Errorf("snapshotWarpSHA() = %q; want \"\" (absent)", got)
	}
}

// TestSnapshotWarpSHA_NewestTaggedCommitWins covers three weft commits all tagged "raddle" at three
// different warp SHAs: the reader must return the newest one's Warp-SHA, not the first or the
// middle.
func TestSnapshotWarpSHA_NewestTaggedCommitWins(t *testing.T) {
	t.Parallel()

	warpPath := newPlainWarpRepo(t)
	weftFixture := lyxtest.CopyWeft(t)
	f := newFabric(t, warpPath, weftFixture.WeftPath)

	commitWeftTagged(t, f, warpPath, weftFixture.WeftPath, "raddle round 1", "raddle")
	commitWeftTagged(t, f, warpPath, weftFixture.WeftPath, "raddle round 2", "raddle")
	warpSHA3, _ := commitWeftTagged(t, f, warpPath, weftFixture.WeftPath, "raddle round 3", "raddle")

	got, err := f.snapshotWarpSHA("raddle")
	if err != nil {
		t.Fatalf("snapshotWarpSHA() error = %v", err)
	}
	if got != warpSHA3 {
		t.Errorf("snapshotWarpSHA(\"raddle\") = %q; want the newest recorded %q", got, warpSHA3)
	}
}

// TestSnapshotWarpSHA_TagIsolation interleaves "raddle" and "trace" tagged commits and asserts each
// tag resolves to its own newest commit, never the other tag's.
func TestSnapshotWarpSHA_TagIsolation(t *testing.T) {
	t.Parallel()

	warpPath := newPlainWarpRepo(t)
	weftFixture := lyxtest.CopyWeft(t)
	f := newFabric(t, warpPath, weftFixture.WeftPath)

	commitWeftTagged(t, f, warpPath, weftFixture.WeftPath, "raddle round 1", "raddle")
	commitWeftTagged(t, f, warpPath, weftFixture.WeftPath, "trace round 1", "trace")
	warpRaddle2, _ := commitWeftTagged(t, f, warpPath, weftFixture.WeftPath, "raddle round 2", "raddle")
	warpTrace2, _ := commitWeftTagged(t, f, warpPath, weftFixture.WeftPath, "trace round 2", "trace")

	gotRaddle, err := f.snapshotWarpSHA("raddle")
	if err != nil {
		t.Fatalf("snapshotWarpSHA(\"raddle\") error = %v", err)
	}
	if gotRaddle != warpRaddle2 {
		t.Errorf("snapshotWarpSHA(\"raddle\") = %q; want %q", gotRaddle, warpRaddle2)
	}

	gotTrace, err := f.snapshotWarpSHA("trace")
	if err != nil {
		t.Fatalf("snapshotWarpSHA(\"trace\") error = %v", err)
	}
	if gotTrace != warpTrace2 {
		t.Errorf("snapshotWarpSHA(\"trace\") = %q; want %q", gotTrace, warpTrace2)
	}
}

// TestSnapshotWarpSHA_MultipleTagsOnOneCommit is the integration-level witness for card 10's
// multi-line-value split: a single commit tagged both "raddle" and "trace" must resolve correctly
// for each tag.
func TestSnapshotWarpSHA_MultipleTagsOnOneCommit(t *testing.T) {
	t.Parallel()

	warpPath := newPlainWarpRepo(t)
	weftFixture := lyxtest.CopyWeft(t)
	f := newFabric(t, warpPath, weftFixture.WeftPath)

	warpSHA, _ := commitWeftTagged(t, f, warpPath, weftFixture.WeftPath, "multi-tag commit", "raddle", "trace")

	gotRaddle, err := f.snapshotWarpSHA("raddle")
	if err != nil {
		t.Fatalf("snapshotWarpSHA(\"raddle\") error = %v", err)
	}
	if gotRaddle != warpSHA {
		t.Errorf("snapshotWarpSHA(\"raddle\") = %q; want %q", gotRaddle, warpSHA)
	}

	gotTrace, err := f.snapshotWarpSHA("trace")
	if err != nil {
		t.Fatalf("snapshotWarpSHA(\"trace\") error = %v", err)
	}
	if gotTrace != warpSHA {
		t.Errorf("snapshotWarpSHA(\"trace\") = %q; want %q", gotTrace, warpSHA)
	}
}

// TestSnapshotWarpSHA_UnbornWeftHEAD covers a weft repo with zero commits: it must exercise the
// "does not have any commits yet" tolerance scanWarpSHATrailers already carries and resolve as
// absent, not an error.
func TestSnapshotWarpSHA_UnbornWeftHEAD(t *testing.T) {
	t.Parallel()

	warpPath := newPlainWarpRepo(t)
	weftPath := t.TempDir()
	lyxtest.MustRun(t, weftPath, "git", "init", "-q", "-b", "main")
	f := newFabric(t, warpPath, weftPath)

	got, err := f.snapshotWarpSHA("raddle")
	if err != nil {
		t.Fatalf("snapshotWarpSHA() error = %v; want nil (unborn weft HEAD)", err)
	}
	if got != "" {
		t.Errorf("snapshotWarpSHA() = %q; want \"\" (unborn weft HEAD)", got)
	}
}

// TestSnapshotWarpSHA_UntaggedCommitsAreSkipped covers plain, untagged weft commits (CommitWeft
// called with zero tags): they must be skipped without error, and a lookup for a tag that was never
// attached to any of them resolves as absent.
func TestSnapshotWarpSHA_UntaggedCommitsAreSkipped(t *testing.T) {
	t.Parallel()

	warpPath := newPlainWarpRepo(t)
	weftFixture := lyxtest.CopyWeft(t)
	f := newFabric(t, warpPath, weftFixture.WeftPath)

	commitWeftTagged(t, f, warpPath, weftFixture.WeftPath, "plain change 1")
	commitWeftTagged(t, f, warpPath, weftFixture.WeftPath, "plain change 2")

	got, err := f.snapshotWarpSHA("raddle")
	if err != nil {
		t.Fatalf("snapshotWarpSHA() error = %v", err)
	}
	if got != "" {
		t.Errorf("snapshotWarpSHA() = %q; want \"\" (no commit carries this tag)", got)
	}
}

// TestSnapshotWarpSHA_SnapshotWithNoWarpSHAIsSkipped covers a commit carrying a Snapshot trailer
// but no Warp-SHA trailer: it must be skipped entirely, never surfaced as a match with an empty
// baseline.
func TestSnapshotWarpSHA_SnapshotWithNoWarpSHAIsSkipped(t *testing.T) {
	t.Parallel()

	warpPath := newPlainWarpRepo(t)
	weftFixture := lyxtest.CopyWeft(t)
	f := newFabric(t, warpPath, weftFixture.WeftPath)

	commitWeftSnapshotOnlyTrailer(t, weftFixture.WeftPath, "no warp trailer", "raddle")

	got, err := f.snapshotWarpSHA("raddle")
	if err != nil {
		t.Fatalf("snapshotWarpSHA() error = %v", err)
	}
	if got != "" {
		t.Errorf("snapshotWarpSHA() = %q; want \"\" (Snapshot trailer with no Warp-SHA sibling is unusable)", got)
	}
}

// TestSnapshotWarpSHA_ByteExactMatching covers the no-fuzzy-matching decision: a tag recorded as
// "raddle" must not be resolved by "Raddle" (case difference) or "raddle " (trailing space) — both
// read as absent, neither errors.
func TestSnapshotWarpSHA_ByteExactMatching(t *testing.T) {
	t.Parallel()

	warpPath := newPlainWarpRepo(t)
	weftFixture := lyxtest.CopyWeft(t)
	f := newFabric(t, warpPath, weftFixture.WeftPath)

	commitWeftTagged(t, f, warpPath, weftFixture.WeftPath, "exact tag", "raddle")

	for _, tag := range []string{"Raddle", "raddle "} {
		got, err := f.snapshotWarpSHA(tag)
		if err != nil {
			t.Fatalf("snapshotWarpSHA(%q) error = %v", tag, err)
		}
		if got != "" {
			t.Errorf("snapshotWarpSHA(%q) = %q; want \"\" (byte-exact match only)", tag, got)
		}
	}
}

// TestSnapshotWarpSHA_PerBranchScoping records a tag on a side branch, then switches the weft
// worktree to a different branch (forked from the weft worktree's ORIGINAL branch, before the
// tagged commit landed) via a plain `git checkout -b`, and asserts snapshotWarpSHA reads the tag as
// absent rather than answering cross-branch — the reader's per-branch contract.
//
// Topology.Checkout is deliberately not used here: it needs a full *lyxcwd.Location,
// and the only fixture in this package building one lives in the external fabricengine_test
// package, unreachable from this internal-package file.
// It would also test the wrong thing — snapshotWarpSHA scans the weft worktree's CURRENT branch and
// nothing else, so a weft-side branch switch by itself is the whole mechanism under test;
// the coordinated host+weft checkout is only how that state arises in production.
func TestSnapshotWarpSHA_PerBranchScoping(t *testing.T) {
	t.Parallel()

	warpPath := newPlainWarpRepo(t)
	weftFixture := lyxtest.CopyWeft(t)
	f := newFabric(t, warpPath, weftFixture.WeftPath)

	// "main" is the weft worktree's original branch (from CopyWeft). Fork
	// "tagged" off it and record the Snapshot tag there, so "main" itself
	// never advances past the fixture's initial commit.
	lyxtest.MustRun(t, weftFixture.WeftPath, "git", "checkout", "-b", "tagged")
	commitWeftTagged(t, f, warpPath, weftFixture.WeftPath, "tagged change", "raddle")

	// Fork "other" off "main" (NOT off "tagged"), so its history does not
	// contain the tagged commit at all, and switch the weft worktree onto
	// it — the branch snapshotWarpSHA must now scan.
	lyxtest.MustRun(t, weftFixture.WeftPath, "git", "checkout", "-b", "other", "main")

	got, err := f.snapshotWarpSHA("raddle")
	if err != nil {
		t.Fatalf("snapshotWarpSHA() error = %v", err)
	}
	if got != "" {
		t.Errorf("snapshotWarpSHA() = %q; want \"\" (tag recorded on another branch must read as absent)", got)
	}
}

// commitWeftTaggedWithDate commits content into weftPath's tracked
// _lyx/<file> (a caller-chosen filename, so a side-branch commit can avoid
// touching the same path a mainline commit touches and merge without
// conflict) at a caller-chosen GIT_COMMITTER_DATE, carrying a Warp-SHA
// trailer for warp's current HEAD (advanced first) and one Snapshot trailer
// per tag. It then calls f.RecordCorrespondence itself, exactly as
// commitWeftLocked would have — building the back-dated commit directly,
// rather than committing normally through CommitWeft and amending the date
// afterward, means there is never an earlier, now-superseded correspondence
// entry to clean up before the equivalence assertion below runs.
//
// The raw `git commit` call is a bare exec.Command, not
// gitexec.RunGit/lyxtest.MustRun: neither of those helpers accepts a
// caller-supplied environment, and this is the only commit in this package
// that needs one. cmd.Env is scoped to this ONE invocation — appended onto
// os.Environ(), never replacing it — rather than a process-wide
// os.Setenv(GIT_COMMITTER_DATE, ...): several tests in this package run
// under t.Parallel(), so a process-wide override would leak into whichever
// of them happens to be committing at the same moment and produce an
// intermittent failure blamed on the wrong test. Appending onto
// os.Environ() (rather than replacing it) preserves the
// GIT_CONFIG_GLOBAL/GIT_CONFIG_NOSYSTEM pair this package's TestMain sets
// via lyxtest.HermeticGitEnv(), so this one raw commit stays hermetic too.
func commitWeftTaggedWithDate(t *testing.T, f *Fabric, warpPath, weftPath, file, content, committerDate string, tags ...string) (warpSHA, weftSHA string) {
	t.Helper()

	warpSHA = commitWarp(t, warpPath, content)

	filePath := filepath.Join(weftPath, "_lyx", file)
	if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	lyxtest.MustRun(t, weftPath, "git", "add", ".")

	msg := appendWarpSHATrailer(DefaultCommitMessage, warpSHA)
	msg, err := appendSnapshotTrailers(msg, tags)
	if err != nil {
		t.Fatalf("appendSnapshotTrailers() error = %v", err)
	}

	cmd := exec.Command("git", "commit", "-q", "-m", msg)
	cmd.Dir = weftPath
	cmd.Env = append(os.Environ(), "GIT_COMMITTER_DATE="+committerDate)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git commit (backdated, dir=%s): %v; output: %s", weftPath, err, output)
	}

	weftSHA = currentSHA(t, weftPath)
	if err := f.RecordCorrespondence(warpSHA, weftSHA); err != nil {
		t.Fatalf("RecordCorrespondence() error = %v", err)
	}
	return warpSHA, weftSHA
}

// TestSnapshotWarpSHA_TopologicalOrderBeatsCommitDate is the one fixture that can actually witness
// card 10's --topo-order change: every other case in this file (and
// TestRebuildIndex_EqualsIncrementallyBuiltIndex in syncweft_integration_test.go) is a linear
// history where date order and topological order coincide, so they would pass identically with or
// without the flag and prove nothing about it.
//
// The fixture: a mainline commit carries the "raddle" tag at a normal (real, current) committer
// date.
// A "side" branch is then forked FROM that mainline commit — so the side commit is a genuine
// topological descendant of it — and its own "raddle"-tagged commit is given a committer date
// back-dated to the year 2000, simulating a skewed clock on whatever machine produced it.
// The side branch is merged back into mainline with --no-ff.
// Because the side commit really is a descendant of the mainline one (regardless of what its clock
// claims), --topo-order must never list it before its own ancestor;
// a plain date-ordered scan would get this backwards and answer with the OLDER mainline baseline,
// under-reporting staleness.
// Both assertions below run against this same history: first, that snapshotWarpSHA resolves to the
// topologically-newest (side) commit's warp SHA, not the date-newest (mainline) one;
// second, reusing TestRebuildIndex_EqualsIncrementallyBuiltIndex's own comparison, that
// RebuildIndex agrees with the incrementally-built index over the same history — proving the
// ordering change did not desynchronize the two.
//
// A residual ambiguity is accepted here rather than solved: when two snapshot commits for the same
// tag sit on genuinely CONCURRENT branches — neither an ancestor of the other, as opposed to this
// fixture's genuine ancestor relationship — "newest" is a topological choice between incomparable
// commits, and either could legitimately be returned.
// Both are legitimate baselines in that case;
// whichever is chosen, the consumer's ChangedFilesSince against it reports a superset or equal set
// of the truly-changed files, and over-reporting is the safe direction, so no attempt is made to
// define a tie-break between concurrent branches here.
func TestSnapshotWarpSHA_TopologicalOrderBeatsCommitDate(t *testing.T) {
	t.Parallel()

	warpPath := newPlainWarpRepo(t)
	weftFixture := lyxtest.CopyWeft(t)
	f := newFabric(t, warpPath, weftFixture.WeftPath)

	warpSHAMainline, _ := commitWeftTagged(t, f, warpPath, weftFixture.WeftPath, "mainline tagged", "raddle")

	lyxtest.MustRun(t, weftFixture.WeftPath, "git", "checkout", "-b", "side")
	warpSHASide, _ := commitWeftTaggedWithDate(t, f, warpPath, weftFixture.WeftPath, "side-marker.txt", "side tagged (back-dated)", "2000-01-01T00:00:00+0000", "raddle")

	lyxtest.MustRun(t, weftFixture.WeftPath, "git", "checkout", "main")
	lyxtest.MustRun(t, weftFixture.WeftPath, "git", "merge", "--no-ff", "-m", "merge side into main", "side")

	got, err := f.snapshotWarpSHA("raddle")
	if err != nil {
		t.Fatalf("snapshotWarpSHA() error = %v", err)
	}
	if got != warpSHASide {
		t.Errorf("snapshotWarpSHA(\"raddle\") = %q; want the topologically-newest (side) baseline %q, not the date-newest mainline baseline %q", got, warpSHASide, warpSHAMainline)
	}

	path, err := f.corrIndexPath()
	if err != nil {
		t.Fatalf("corrIndexPath() error = %v", err)
	}
	incremental, err := loadCorrIndex(path)
	if err != nil {
		t.Fatalf("loadCorrIndex() error = %v", err)
	}
	wantEntries := incremental.entries()
	if len(wantEntries) != 2 {
		t.Fatalf("incrementally-built index has %d entries; want 2", len(wantEntries))
	}

	if err := f.RebuildIndex(); err != nil {
		t.Fatalf("RebuildIndex() error = %v", err)
	}
	rebuilt, err := loadCorrIndex(path)
	if err != nil {
		t.Fatalf("loadCorrIndex() (post-rebuild) error = %v", err)
	}
	gotEntries := rebuilt.entries()

	if len(gotEntries) != len(wantEntries) {
		t.Fatalf("RebuildIndex() entries = %d; want %d", len(gotEntries), len(wantEntries))
	}
	for i := range wantEntries {
		if gotEntries[i] != wantEntries[i] {
			t.Errorf("entries[%d]: RebuildIndex()=%+v incremental=%+v; want equal", i, gotEntries[i], wantEntries[i])
		}
	}
}

// treeSHA resolves rev's tree object SHA in repoPath, via `git rev-parse
// <rev>^{tree}` — used to compare two commits' trees directly, independent
// of their own commit SHAs.
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

// TestWeftSHAForWarpSHA_CorrespondenceOverwrite_EmptyCommitWins pins the case card 17 documents but
// does not itself pin: a warp SHA recorded by TWO weft commits — a content commit under "raddle",
// then a tags-only commit at the SAME (unchanged) warp HEAD under the same tag.
// Three things are asserted.
// First, WeftSHAForWarpSHA resolves to the newer, EMPTY commit — last recorded wins, exactly as
// corrIndex.record already documents.
// Second, RebuildIndex's full trailer rescan agrees with the incrementally-built index over this
// same history, reusing the entries()-comparison TestRebuildIndex_EqualsIncrementallyBuiltIndex
// (syncweft_integration_test.go) already establishes as the equivalence check.
// Third — the assertion that makes the overwrite BENIGN rather than merely tolerated — the empty
// commit's tree is byte-identical to the content commit's, so resolving through the overwritten
// entry restores exactly the same weft state either commit would have.
func TestWeftSHAForWarpSHA_CorrespondenceOverwrite_EmptyCommitWins(t *testing.T) {
	warpPath := newPlainWarpRepo(t)
	weftFixture := lyxtest.CopyWeft(t)
	f := newFabric(t, warpPath, weftFixture.WeftPath)

	warpSHA, contentWeftSHA := commitWeftTagged(t, f, warpPath, weftFixture.WeftPath, "content commit", "raddle")

	// A genuine tags-only call (nil pathspec) at the same warp HEAD — warp is
	// not advanced again here — so the empty-commit rule's `!positive`
	// fall-through fires and RecordCorrespondence upserts over the entry the
	// content commit just wrote for warpSHA.
	emptyWeftSHA, committed, err := f.commitWeft(nil, DefaultCommitMessage, SyncOptions{}, "raddle")
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

	path, err := f.corrIndexPath()
	if err != nil {
		t.Fatalf("corrIndexPath() error = %v", err)
	}
	incremental, err := loadCorrIndex(path)
	if err != nil {
		t.Fatalf("loadCorrIndex() error = %v", err)
	}
	wantEntries := incremental.entries()

	if err := f.RebuildIndex(); err != nil {
		t.Fatalf("RebuildIndex() error = %v", err)
	}
	rebuilt, err := loadCorrIndex(path)
	if err != nil {
		t.Fatalf("loadCorrIndex() (post-rebuild) error = %v", err)
	}
	gotEntries := rebuilt.entries()

	if len(gotEntries) != len(wantEntries) {
		t.Fatalf("RebuildIndex() entries = %d; want %d", len(gotEntries), len(wantEntries))
	}
	for i := range wantEntries {
		if gotEntries[i] != wantEntries[i] {
			t.Errorf("entries[%d]: RebuildIndex()=%+v incremental=%+v; want equal", i, gotEntries[i], wantEntries[i])
		}
	}

	gotTree := treeSHA(t, weftFixture.WeftPath, emptyWeftSHA)
	wantTree := treeSHA(t, weftFixture.WeftPath, contentWeftSHA)
	if gotTree != wantTree {
		t.Errorf("empty commit tree = %q; want it identical to the content commit's tree %q (the overwrite is benign because both resolve to the same weft state)", gotTree, wantTree)
	}
}

// TestSnapshotWarpSHA_DanglingWarpSHA_ReturnsRawWithSHAExistsFalse pins the reader's
// validate-at-use posture: a recorded Warp-SHA whose warp commit is later rewritten away
// (rebase/amend/reset+prune) is returned RAW by snapshotWarpSHA, with a nil error — not collapsed
// to absent and not resolved to an older baseline — and f.Warp.SHAExists on the returned SHA
// reports false, demonstrating the "read, then check SHAExists" consumer idiom snapshotWarpSHA's
// own doc comment describes, in executable form.
func TestSnapshotWarpSHA_DanglingWarpSHA_ReturnsRawWithSHAExistsFalse(t *testing.T) {
	warpPath := newPlainWarpRepo(t)
	weftFixture := lyxtest.CopyWeft(t)
	f := newFabric(t, warpPath, weftFixture.WeftPath)

	baseWarpSHA := currentSHA(t, warpPath)
	danglingWarpSHA, _ := commitWeftTagged(t, f, warpPath, weftFixture.WeftPath, "will be rewritten away", "raddle")

	// Rewrite warp history so danglingWarpSHA no longer resolves: reset back
	// to the base commit, then force git to genuinely forget the orphaned
	// object — a plain reset alone leaves it resolvable via the reflog for a
	// while (expireAndPruneUnreachable, syncweft_integration_test.go).
	lyxtest.MustRun(t, warpPath, "git", "reset", "--hard", baseWarpSHA)
	expireAndPruneUnreachable(t, warpPath)

	got, err := f.snapshotWarpSHA("raddle")
	if err != nil {
		t.Fatalf("snapshotWarpSHA() error = %v; want nil (a dangling Warp-SHA is returned raw, not an error)", err)
	}
	if got != danglingWarpSHA {
		t.Errorf("snapshotWarpSHA() = %q; want the dangling SHA %q returned raw", got, danglingWarpSHA)
	}
	if f.Warp.SHAExists(got) {
		t.Errorf("Warp.SHAExists(%q) = true; want false (the warp commit was rewritten away)", got)
	}
}
