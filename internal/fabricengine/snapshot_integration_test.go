//go:build integration

// snapshot_integration_test.go — integration coverage for
// Fabric.SnapshotWarpSHA: newest-tagged-commit-wins, tag isolation, the
// multi-tag-on-one-commit split, the absent-is-not-an-error miss path, the
// unborn-weft-HEAD tolerance, untagged/no-baseline commits, a Snapshot
// trailer with no Warp-SHA sibling, byte-exact tag matching, and per-branch
// scoping. Package fabricengine (internal), reusing
// index_integration_test.go's newPlainWarpRepo, currentSHA, commitWarp,
// commitWeftWithTrailer, and newFabric helpers rather than building a
// parallel harness — they share this package.

package fabricengine

import (
	"os"
	"path/filepath"
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
	weftSHA, committed, err := f.CommitWeft([]string{"_lyx"}, DefaultCommitMessage, SyncOptions{}, tags...)
	if err != nil {
		t.Fatalf("CommitWeft(tags=%v) error = %v", tags, err)
	}
	if !committed {
		t.Fatalf("CommitWeft(tags=%v) committed = false; want true", tags)
	}
	return warpSHA, weftSHA
}

// commitWeftSnapshotOnlyTrailer commits content into weftPath's tracked
// _lyx/config.yaml with a hand-crafted commit message carrying ONLY a
// "Snapshot: <tag>" trailer and no "Warp-SHA:" trailer at all — the shape
// SnapshotWarpSHA's reader must skip rather than mistake for an empty
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

// TestSnapshotWarpSHA_Miss is the TDD candidate for this card: a tag never
// recorded anywhere in history must resolve as absent — ("", nil), not an
// error — pinning the absent-is-not-an-error decision that lets a
// first-ever consumer run read "no baseline, generate everything" with no
// special-casing.
func TestSnapshotWarpSHA_Miss(t *testing.T) {
	t.Parallel()

	warpPath := newPlainWarpRepo(t)
	weftFixture := lyxtest.CopyWeft(t)
	f := newFabric(t, warpPath, weftFixture.WeftPath)

	commitWeftTagged(t, f, warpPath, weftFixture.WeftPath, "untagged change")

	got, err := f.SnapshotWarpSHA("never-recorded")
	if err != nil {
		t.Fatalf("SnapshotWarpSHA() error = %v; want nil", err)
	}
	if got != "" {
		t.Errorf("SnapshotWarpSHA() = %q; want \"\" (absent)", got)
	}
}

// TestSnapshotWarpSHA_NewestTaggedCommitWins covers three weft commits all
// tagged "raddle" at three different warp SHAs: the reader must return the
// newest one's Warp-SHA, not the first or the middle.
func TestSnapshotWarpSHA_NewestTaggedCommitWins(t *testing.T) {
	t.Parallel()

	warpPath := newPlainWarpRepo(t)
	weftFixture := lyxtest.CopyWeft(t)
	f := newFabric(t, warpPath, weftFixture.WeftPath)

	commitWeftTagged(t, f, warpPath, weftFixture.WeftPath, "raddle round 1", "raddle")
	commitWeftTagged(t, f, warpPath, weftFixture.WeftPath, "raddle round 2", "raddle")
	warpSHA3, _ := commitWeftTagged(t, f, warpPath, weftFixture.WeftPath, "raddle round 3", "raddle")

	got, err := f.SnapshotWarpSHA("raddle")
	if err != nil {
		t.Fatalf("SnapshotWarpSHA() error = %v", err)
	}
	if got != warpSHA3 {
		t.Errorf("SnapshotWarpSHA(\"raddle\") = %q; want the newest recorded %q", got, warpSHA3)
	}
}

// TestSnapshotWarpSHA_TagIsolation interleaves "raddle" and "trace" tagged
// commits and asserts each tag resolves to its own newest commit, never the
// other tag's.
func TestSnapshotWarpSHA_TagIsolation(t *testing.T) {
	t.Parallel()

	warpPath := newPlainWarpRepo(t)
	weftFixture := lyxtest.CopyWeft(t)
	f := newFabric(t, warpPath, weftFixture.WeftPath)

	commitWeftTagged(t, f, warpPath, weftFixture.WeftPath, "raddle round 1", "raddle")
	commitWeftTagged(t, f, warpPath, weftFixture.WeftPath, "trace round 1", "trace")
	warpRaddle2, _ := commitWeftTagged(t, f, warpPath, weftFixture.WeftPath, "raddle round 2", "raddle")
	warpTrace2, _ := commitWeftTagged(t, f, warpPath, weftFixture.WeftPath, "trace round 2", "trace")

	gotRaddle, err := f.SnapshotWarpSHA("raddle")
	if err != nil {
		t.Fatalf("SnapshotWarpSHA(\"raddle\") error = %v", err)
	}
	if gotRaddle != warpRaddle2 {
		t.Errorf("SnapshotWarpSHA(\"raddle\") = %q; want %q", gotRaddle, warpRaddle2)
	}

	gotTrace, err := f.SnapshotWarpSHA("trace")
	if err != nil {
		t.Fatalf("SnapshotWarpSHA(\"trace\") error = %v", err)
	}
	if gotTrace != warpTrace2 {
		t.Errorf("SnapshotWarpSHA(\"trace\") = %q; want %q", gotTrace, warpTrace2)
	}
}

// TestSnapshotWarpSHA_MultipleTagsOnOneCommit is the integration-level
// witness for card 10's multi-line-value split: a single commit tagged both
// "raddle" and "trace" must resolve correctly for each tag.
func TestSnapshotWarpSHA_MultipleTagsOnOneCommit(t *testing.T) {
	t.Parallel()

	warpPath := newPlainWarpRepo(t)
	weftFixture := lyxtest.CopyWeft(t)
	f := newFabric(t, warpPath, weftFixture.WeftPath)

	warpSHA, _ := commitWeftTagged(t, f, warpPath, weftFixture.WeftPath, "multi-tag commit", "raddle", "trace")

	gotRaddle, err := f.SnapshotWarpSHA("raddle")
	if err != nil {
		t.Fatalf("SnapshotWarpSHA(\"raddle\") error = %v", err)
	}
	if gotRaddle != warpSHA {
		t.Errorf("SnapshotWarpSHA(\"raddle\") = %q; want %q", gotRaddle, warpSHA)
	}

	gotTrace, err := f.SnapshotWarpSHA("trace")
	if err != nil {
		t.Fatalf("SnapshotWarpSHA(\"trace\") error = %v", err)
	}
	if gotTrace != warpSHA {
		t.Errorf("SnapshotWarpSHA(\"trace\") = %q; want %q", gotTrace, warpSHA)
	}
}

// TestSnapshotWarpSHA_UnbornWeftHEAD covers a weft repo with zero commits: it
// must exercise the "does not have any commits yet" tolerance
// scanWarpSHATrailers already carries and resolve as absent, not an error.
func TestSnapshotWarpSHA_UnbornWeftHEAD(t *testing.T) {
	t.Parallel()

	warpPath := newPlainWarpRepo(t)
	weftPath := t.TempDir()
	lyxtest.MustRun(t, weftPath, "git", "init", "-q", "-b", "main")
	f := newFabric(t, warpPath, weftPath)

	got, err := f.SnapshotWarpSHA("raddle")
	if err != nil {
		t.Fatalf("SnapshotWarpSHA() error = %v; want nil (unborn weft HEAD)", err)
	}
	if got != "" {
		t.Errorf("SnapshotWarpSHA() = %q; want \"\" (unborn weft HEAD)", got)
	}
}

// TestSnapshotWarpSHA_UntaggedCommitsAreSkipped covers plain, untagged weft
// commits (CommitWeft called with zero tags): they must be skipped without
// error, and a lookup for a tag that was never attached to any of them
// resolves as absent.
func TestSnapshotWarpSHA_UntaggedCommitsAreSkipped(t *testing.T) {
	t.Parallel()

	warpPath := newPlainWarpRepo(t)
	weftFixture := lyxtest.CopyWeft(t)
	f := newFabric(t, warpPath, weftFixture.WeftPath)

	commitWeftTagged(t, f, warpPath, weftFixture.WeftPath, "plain change 1")
	commitWeftTagged(t, f, warpPath, weftFixture.WeftPath, "plain change 2")

	got, err := f.SnapshotWarpSHA("raddle")
	if err != nil {
		t.Fatalf("SnapshotWarpSHA() error = %v", err)
	}
	if got != "" {
		t.Errorf("SnapshotWarpSHA() = %q; want \"\" (no commit carries this tag)", got)
	}
}

// TestSnapshotWarpSHA_SnapshotWithNoWarpSHAIsSkipped covers a commit
// carrying a Snapshot trailer but no Warp-SHA trailer: it must be skipped
// entirely, never surfaced as a match with an empty baseline.
func TestSnapshotWarpSHA_SnapshotWithNoWarpSHAIsSkipped(t *testing.T) {
	t.Parallel()

	warpPath := newPlainWarpRepo(t)
	weftFixture := lyxtest.CopyWeft(t)
	f := newFabric(t, warpPath, weftFixture.WeftPath)

	commitWeftSnapshotOnlyTrailer(t, weftFixture.WeftPath, "no warp trailer", "raddle")

	got, err := f.SnapshotWarpSHA("raddle")
	if err != nil {
		t.Fatalf("SnapshotWarpSHA() error = %v", err)
	}
	if got != "" {
		t.Errorf("SnapshotWarpSHA() = %q; want \"\" (Snapshot trailer with no Warp-SHA sibling is unusable)", got)
	}
}

// TestSnapshotWarpSHA_ByteExactMatching covers the no-fuzzy-matching
// decision: a tag recorded as "raddle" must not be resolved by "Raddle" (case
// difference) or "raddle " (trailing space) — both read as absent, neither
// errors.
func TestSnapshotWarpSHA_ByteExactMatching(t *testing.T) {
	t.Parallel()

	warpPath := newPlainWarpRepo(t)
	weftFixture := lyxtest.CopyWeft(t)
	f := newFabric(t, warpPath, weftFixture.WeftPath)

	commitWeftTagged(t, f, warpPath, weftFixture.WeftPath, "exact tag", "raddle")

	for _, tag := range []string{"Raddle", "raddle "} {
		got, err := f.SnapshotWarpSHA(tag)
		if err != nil {
			t.Fatalf("SnapshotWarpSHA(%q) error = %v", tag, err)
		}
		if got != "" {
			t.Errorf("SnapshotWarpSHA(%q) = %q; want \"\" (byte-exact match only)", tag, got)
		}
	}
}

// TestSnapshotWarpSHA_PerBranchScoping records a tag on a side branch, then
// switches the weft worktree to a different branch (forked from the weft
// worktree's ORIGINAL branch, before the tagged commit landed) via a plain
// `git checkout -b`, and asserts SnapshotWarpSHA reads the tag as absent
// rather than answering cross-branch — the reader's per-branch contract.
//
// Topology.Checkout is deliberately not used here: it needs a full
// *hubgeometry.Layout, and the only fixture in this package building one
// lives in the external fabricengine_test package, unreachable from this
// internal-package file. It would also test the wrong thing — SnapshotWarpSHA
// scans the weft worktree's CURRENT branch and nothing else, so a weft-side
// branch switch by itself is the whole mechanism under test; the coordinated
// host+weft checkout is only how that state arises in production.
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
	// it — the branch SnapshotWarpSHA must now scan.
	lyxtest.MustRun(t, weftFixture.WeftPath, "git", "checkout", "-b", "other", "main")

	got, err := f.SnapshotWarpSHA("raddle")
	if err != nil {
		t.Fatalf("SnapshotWarpSHA() error = %v", err)
	}
	if got != "" {
		t.Errorf("SnapshotWarpSHA() = %q; want \"\" (tag recorded on another branch must read as absent)", got)
	}
}
