//go:build integration

// wiring_commitstatus_integration_test.go drives the per-transition status seam against a REAL
// fabric pair — real MergeStateActive, real CommitAnchoredPaths, real PushAnchored, real git — which
// wiring_commitstatus_test.go's Tier 1 stub closures by construction cannot.
// The distinction earned its keep: every stub test passes over a seam that stages the status file
// into a foreign merge's index and then kills the run, because a stub Commit has no index to stage
// into and no git to refuse it. Only the composed shape shows what the three dispositions actually
// do to a repository.

package loomcli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/shedrun"
)

// gitOut runs a git command in dir and returns its trimmed combined output alongside the error, so
// a caller can probe a command that is EXPECTED to fail (rev-parse --verify MERGE_HEAD) without
// failing the test. gitkit.MustRun cannot serve here: it discards output and fatals on any error.
func gitOut(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// mustGitOut is gitOut for a command that must succeed, returning its trimmed output.
func mustGitOut(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := gitOut(dir, args...)
	if err != nil {
		t.Fatalf("git %s in %s: %v; output: %s", strings.Join(args, " "), dir, err, out)
	}
	return out
}

// realSeamFixture builds a hub, adds one pair, seeds a status file at loom's own status path, and
// returns the seam wired to that pair through loomCommitStatusDeps — the same call wire() makes —
// plus the pair's weft sibling path for direct git inspection.
func realSeamFixture(t *testing.T) (seam func(producer, state string) error, location *lyxcwd.Location, weftSibling string) {
	t.Helper()

	hub := hubforge.NewHub(t, ".")
	const slug = "commitstatus"
	hubforge.AddPair(t, hub, slug)
	warpWorktree := hub.PairWarpWorktree(slug)

	location, err := lyxcwd.ResolveWorktree(warpWorktree)
	if err != nil {
		t.Fatalf("ResolveWorktree(%s) error = %v; want nil", warpWorktree, err)
	}
	writeStatusFile(t, location, `{"current_producer":"seed","state":"running"}`)

	return newCommitStatusSeam(loomCommitStatusDeps(location, shedrun.SelfRunID)), location, hub.PairWeftSibling(slug)
}

// writeStatusFile writes content at loom's own status path for location, creating the directory the
// first call needs. It goes through shedrun's own accessor rather than a hand-built join so the test
// commits exactly the path the seam's own pathspec names.
func writeStatusFile(t *testing.T, location *lyxcwd.Location, content string) {
	t.Helper()
	path := shedrun.StatusFile(location, shedrun.SelfRunID)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s) error = %v; want nil", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content+"\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(%s) error = %v; want nil", path, err)
	}
}

// startForeignMergeInWeft leaves a live MERGE_HEAD in the weft sibling by running plain git there —
// the operator behaviour the Fabric Git Invariant's own carve-out permits, and the only way weft
// merge state exists at all now that no fabric verb puts it there.
func startForeignMergeInWeft(t *testing.T, weftSibling string) {
	t.Helper()
	current := mustGitOut(t, weftSibling, "rev-parse", "--abbrev-ref", "HEAD")
	mustGitOut(t, weftSibling, "checkout", "-q", "-b", "foreign-side")
	if err := os.WriteFile(filepath.Join(weftSibling, "foreign.txt"), []byte("foreign\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(foreign.txt) error = %v; want nil", err)
	}
	mustGitOut(t, weftSibling, "add", "foreign.txt")
	mustGitOut(t, weftSibling, "commit", "-q", "-m", "foreign side commit")
	mustGitOut(t, weftSibling, "checkout", "-q", current)
	mustGitOut(t, weftSibling, "merge", "--no-commit", "--no-ff", "foreign-side")

	if !mergeHeadPresent(t, weftSibling) {
		t.Fatal("no MERGE_HEAD in the weft sibling after a plain-git merge --no-commit; the fixture proves nothing without one")
	}
}

// mergeHeadPresent reports whether the repo at dir currently has a live MERGE_HEAD.
func mergeHeadPresent(t *testing.T, dir string) bool {
	t.Helper()
	_, err := gitOut(dir, "rev-parse", "-q", "--verify", "MERGE_HEAD")
	return err == nil
}

// headSHA returns dir's current HEAD SHA.
func headSHA(t *testing.T, dir string) string {
	t.Helper()
	return mustGitOut(t, dir, "rev-parse", "HEAD")
}

// TestCommitStatusSeam_Real_OrdinaryPathCommitsAndPushes asserts the ordinary disposition against a
// real pair: the seam lands a real weft commit carrying the status file under the transition's own
// message, and leaves nothing unpushed.
func TestCommitStatusSeam_Real_OrdinaryPathCommitsAndPushes(t *testing.T) {
	seam, location, weftSibling := realSeamFixture(t)
	before := headSHA(t, weftSibling)

	if err := seam("Discussion-Write", "running"); err != nil {
		t.Fatalf("seam(Discussion-Write, running) error = %v; want nil", err)
	}

	after := headSHA(t, weftSibling)
	if after == before {
		t.Fatalf("weft HEAD = %q; want it moved off %q — the seam must land a real commit", after, before)
	}
	if got := mustGitOut(t, weftSibling, "log", "-1", "--format=%s"); got != "loom: Discussion-Write -> running" {
		t.Errorf("weft HEAD subject = %q; want %q", got, "loom: Discussion-Write -> running")
	}
	if got := mustGitOut(t, weftSibling, "show", "--name-only", "--format=", "HEAD"); !strings.Contains(got, shedrun.StatusRel(location, shedrun.SelfRunID)) {
		t.Errorf("weft HEAD touched %q; want it to include %q", got, shedrun.StatusRel(location, shedrun.SelfRunID))
	}
	if got := mustGitOut(t, weftSibling, "log", "--oneline", "@{u}..HEAD"); got != "" {
		t.Errorf("unpushed weft commits after the seam = %q; want none — the seam pushes synchronously", got)
	}
}

// TestCommitStatusSeam_Real_MidMergeSkipsWithoutTouchingTheMerge asserts the skip disposition
// against a real foreign merge: nothing is committed, nothing is staged into the operator's index,
// and their MERGE_HEAD survives untouched.
func TestCommitStatusSeam_Real_MidMergeSkipsWithoutTouchingTheMerge(t *testing.T) {
	seam, _, weftSibling := realSeamFixture(t)
	startForeignMergeInWeft(t, weftSibling)
	before := headSHA(t, weftSibling)
	stagedBefore := mustGitOut(t, weftSibling, "diff", "--cached", "--name-only")

	if err := seam("Discussion-Write", "running"); err != nil {
		t.Fatalf("seam(...) error = %v; want nil — a mid-merge weft skips, it does not halt the run", err)
	}

	if got := headSHA(t, weftSibling); got != before {
		t.Errorf("weft HEAD = %q; want unchanged %q — the skip must commit nothing", got, before)
	}
	if !mergeHeadPresent(t, weftSibling) {
		t.Error("MERGE_HEAD is gone after the seam ran; want the operator's merge left exactly as it was")
	}
	if got := mustGitOut(t, weftSibling, "diff", "--cached", "--name-only"); got != stagedBefore {
		t.Errorf("staged paths = %q; want unchanged %q — the skip must not stage the status file into the operator's merge index", got, stagedBefore)
	}
}

// TestCommitStatusSeam_Real_MergeGoesLiveAfterProbeSkipsInsteadOfHalting drives the lost race the
// unlocked probe leaves open, deterministically: MergeActive answers false once — as it would if the
// operator started their merge a millisecond later — while every other dep, including the re-probe,
// is the real one.
// Before the re-probe existed this returned git's "cannot do a partial commit during a merge" and
// halted the whole run. It must now resolve as the skip, with the operator's merge intact.
func TestCommitStatusSeam_Real_MergeGoesLiveAfterProbeSkipsInsteadOfHalting(t *testing.T) {
	_, location, weftSibling := realSeamFixture(t)
	startForeignMergeInWeft(t, weftSibling)
	before := headSHA(t, weftSibling)

	deps := loomCommitStatusDeps(location, shedrun.SelfRunID)
	realMergeActive := deps.MergeActive
	probes := 0
	deps.MergeActive = func() (bool, error) {
		probes++
		if probes == 1 {
			return false, nil
		}
		return realMergeActive()
	}

	if err := newCommitStatusSeam(deps)("Discussion-Write", "running"); err != nil {
		t.Fatalf("seam(...) error = %v; want nil — a commit failure the re-probe explains as a live merge takes the skip", err)
	}
	if probes != 2 {
		t.Errorf("MergeActive called %d time(s); want exactly 2 — the pre-commit probe and the re-probe that explains its failure", probes)
	}
	if got := headSHA(t, weftSibling); got != before {
		t.Errorf("weft HEAD = %q; want unchanged %q", got, before)
	}
	if !mergeHeadPresent(t, weftSibling) {
		t.Error("MERGE_HEAD is gone after the lost race; want the operator's merge left exactly as it was")
	}
}

// TestCommitStatusSeam_Real_RejectedPushWarnsAndTheCommitStays asserts the push-warns disposition
// against a genuinely diverged weft remote: the seam returns nil, the local commit stays, and the
// branch is left behind its origin for the next transition to catch up.
func TestCommitStatusSeam_Real_RejectedPushWarnsAndTheCommitStays(t *testing.T) {
	seam, _, weftSibling := realSeamFixture(t)

	// Advance the weft remote out from under the local sibling, so the seam's push is rejected for
	// the ordinary reason: another machine got there first.
	clone := t.TempDir()
	branch := mustGitOut(t, weftSibling, "rev-parse", "--abbrev-ref", "HEAD")
	origin := mustGitOut(t, weftSibling, "remote", "get-url", "origin")
	mustGitOut(t, clone, "clone", "-q", "-b", branch, origin, ".")
	mustGitOut(t, clone, "config", "user.email", "rogue@example.test")
	mustGitOut(t, clone, "config", "user.name", "rogue")
	if err := os.WriteFile(filepath.Join(clone, "rogue.txt"), []byte("rogue\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(rogue.txt) error = %v; want nil", err)
	}
	mustGitOut(t, clone, "add", "rogue.txt")
	mustGitOut(t, clone, "commit", "-q", "-m", "rogue advance")
	mustGitOut(t, clone, "push", "-q", "origin", branch)

	before := headSHA(t, weftSibling)
	if err := seam("Discussion-Write", "running"); err != nil {
		t.Fatalf("seam(...) error = %v; want nil — a rejected push warns and continues", err)
	}
	if got := headSHA(t, weftSibling); got == before {
		t.Errorf("weft HEAD = %q; want it moved off %q — the commit lands even though the push is rejected", got, before)
	}
	if got := mustGitOut(t, weftSibling, "log", "--oneline", "@{u}..HEAD"); got == "" {
		t.Error("unpushed weft commits after a rejected push = none; want the local commit still unpushed, waiting for the next transition")
	}
}

// TestCommitStatusSeam_Real_UnreachableRemoteWarnsToo asserts the push-warns disposition covers push
// failures that are NOT gitrepo.ErrPushRejected — an unreachable remote is the offline case the
// disposition exists for, and it must not halt the run either.
func TestCommitStatusSeam_Real_UnreachableRemoteWarnsToo(t *testing.T) {
	seam, _, weftSibling := realSeamFixture(t)
	mustGitOut(t, weftSibling, "remote", "set-url", "origin", filepath.Join(t.TempDir(), "does-not-exist.git"))

	before := headSHA(t, weftSibling)
	if err := seam("Discussion-Write", "running"); err != nil {
		t.Fatalf("seam(...) error = %v; want nil — an unreachable remote warns and continues, exactly as a rejection does", err)
	}
	if got := headSHA(t, weftSibling); got == before {
		t.Errorf("weft HEAD = %q; want it moved off %q — the commit lands even though the push failed", got, before)
	}
}

// writeReviewsDirFile writes content at rel under the reviews directory, creating its directories.
func writeReviewsDirFile(t *testing.T, location *lyxcwd.Location, rel, content string) {
	t.Helper()
	path := filepath.Join(loomengine.LoomReviewsDir(location), rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s) error = %v; want nil", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%s) error = %v; want nil", path, err)
	}
}

// TestCommitStatusSeam_Real_CommitsTheRoundRecord asserts a round's files written before a transition land in the weft HEAD commit and leave the reviews directory clean.
func TestCommitStatusSeam_Real_CommitsTheRoundRecord(t *testing.T) {
	seam, location, weftSibling := realSeamFixture(t)
	files := []string{"plan/round-1-review.md", "plan/round-1-fixer-report.md", "plan/round-1-focus.md"}
	for _, f := range files {
		writeReviewsDirFile(t, location, f, "record\n")
	}

	if err := seam("Plan-Review", "running"); err != nil {
		t.Fatalf("seam error = %v; want nil", err)
	}

	got := mustGitOut(t, weftSibling, "show", "--name-only", "--format=", "HEAD")
	for _, f := range files {
		want := filepath.ToSlash(filepath.Join(loomengine.LoomReviewsDirRel(), f))
		if !strings.Contains(got, want) {
			t.Errorf("weft HEAD touched %q; want it to include %q", got, want)
		}
	}
	if st := mustGitOut(t, weftSibling, "status", "--porcelain", "--", filepath.ToSlash(loomengine.LoomReviewsDirRel())); st != "" {
		t.Errorf("weft status under the reviews directory = %q; want clean", st)
	}
}

// TestCommitStatusSeam_Real_NoReviewsDirTouchesOnlyStatus asserts a run with no reviews directory commits the status file alone.
func TestCommitStatusSeam_Real_NoReviewsDirTouchesOnlyStatus(t *testing.T) {
	seam, location, weftSibling := realSeamFixture(t)

	if err := seam("Discussion-Write", "running"); err != nil {
		t.Fatalf("seam error = %v; want nil", err)
	}
	if got := mustGitOut(t, weftSibling, "show", "--name-only", "--format=", "HEAD"); got != filepath.ToSlash(shedrun.StatusRel(location, shedrun.SelfRunID)) {
		t.Errorf("weft HEAD touched %q; want only the status file", got)
	}
}

// TestCommitStatusSeam_Real_EmptyReviewsSegmentTouchesOnlyStatus asserts the recipe-build state — the reviews root holding only an empty segment directory — neither errors nor widens the commit.
func TestCommitStatusSeam_Real_EmptyReviewsSegmentTouchesOnlyStatus(t *testing.T) {
	seam, location, weftSibling := realSeamFixture(t)
	if err := os.MkdirAll(filepath.Join(loomengine.LoomReviewsDir(location), "plan"), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := seam("Discussion-Write", "running"); err != nil {
		t.Fatalf("seam error = %v; want nil", err)
	}
	if got := mustGitOut(t, weftSibling, "show", "--name-only", "--format=", "HEAD"); got != filepath.ToSlash(shedrun.StatusRel(location, shedrun.SelfRunID)) {
		t.Errorf("weft HEAD touched %q; want only the status file", got)
	}
}

// TestCommitStatusSeam_Real_ArchiveRenameCommitsAdditionAndDeletion asserts a committed round file renamed to a timestamped sibling is recorded as both an addition and a deletion by the next commit.
func TestCommitStatusSeam_Real_ArchiveRenameCommitsAdditionAndDeletion(t *testing.T) {
	seam, location, weftSibling := realSeamFixture(t)
	writeReviewsDirFile(t, location, "plan/round-1-review.md", "record\n")
	if err := seam("Plan-Review", "running"); err != nil {
		t.Fatalf("first seam error = %v; want nil", err)
	}

	dir := filepath.Join(loomengine.LoomReviewsDir(location), "plan")
	if err := os.Rename(filepath.Join(dir, "round-1-review.md"), filepath.Join(dir, "round-1-review-20260101T000000.md")); err != nil {
		t.Fatal(err)
	}
	if err := seam("Plan-Review", "done"); err != nil {
		t.Fatalf("second seam error = %v; want nil", err)
	}

	got := mustGitOut(t, weftSibling, "show", "--no-renames", "--name-status", "--format=", "HEAD")
	prefix := filepath.ToSlash(filepath.Join(loomengine.LoomReviewsDirRel(), "plan")) + "/"
	if !strings.Contains(got, "A\t"+prefix+"round-1-review-20260101T000000.md") {
		t.Errorf("weft HEAD = %q; want the timestamped sibling added", got)
	}
	if !strings.Contains(got, "D\t"+prefix+"round-1-review.md") {
		t.Errorf("weft HEAD = %q; want the original deleted", got)
	}
}

// writeRecordFile writes content at path, creating its directory.
func writeRecordFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestCommitStatusSeam_Real_CommitsFrictionAndDriveReports asserts a friction note and a drive report written before a transition land in the weft HEAD commit and leave both directories clean.
func TestCommitStatusSeam_Real_CommitsFrictionAndDriveReports(t *testing.T) {
	seam, location, weftSibling := realSeamFixture(t)
	writeRecordFile(t, filepath.Join(loomengine.LoomFrictionDir(location), "note.md"), "friction\n")
	writeRecordFile(t, filepath.Join(shedrun.DriveReportsDir(location, shedrun.SelfRunID), "report.md"), "report\n")

	if err := seam("Plan-Write", "running"); err != nil {
		t.Fatalf("seam error = %v; want nil", err)
	}

	got := mustGitOut(t, weftSibling, "show", "--name-only", "--format=", "HEAD")
	for _, want := range []string{
		filepath.ToSlash(filepath.Join(loomengine.LoomDurableDirRel(), "friction", "note.md")),
		filepath.ToSlash(filepath.Join(shedrun.DriveReportsRel(location, shedrun.SelfRunID), "report.md")),
	} {
		if !strings.Contains(got, want) {
			t.Errorf("weft HEAD touched %q; want it to include %q", got, want)
		}
	}
	for _, dir := range []string{loomengine.LoomDurableDirRel(), shedrun.DriveReportsRel(location, shedrun.SelfRunID)} {
		if st := mustGitOut(t, weftSibling, "status", "--porcelain", "--", filepath.ToSlash(dir)); st != "" {
			t.Errorf("weft status under %s = %q; want clean", dir, st)
		}
	}
}

// TestCommitStatusSeam_Real_FrictionArchiveRenameCommitsAdditionAndDeletion asserts a committed friction directory renamed to a timestamped sibling is recorded as both an addition and a deletion, leaving the loom durable directory clean.
func TestCommitStatusSeam_Real_FrictionArchiveRenameCommitsAdditionAndDeletion(t *testing.T) {
	seam, location, weftSibling := realSeamFixture(t)
	writeRecordFile(t, filepath.Join(loomengine.LoomFrictionDir(location), "note.md"), "friction\n")
	if err := seam("Plan-Write", "running"); err != nil {
		t.Fatalf("first seam error = %v; want nil", err)
	}

	archive := filepath.Join(loomengine.LoomDurableDir(location), "friction-20260101T000000")
	if err := os.Rename(loomengine.LoomFrictionDir(location), archive); err != nil {
		t.Fatal(err)
	}
	if err := seam("Plan-Write", "done"); err != nil {
		t.Fatalf("second seam error = %v; want nil", err)
	}

	got := mustGitOut(t, weftSibling, "show", "--no-renames", "--name-status", "--format=", "HEAD")
	prefix := filepath.ToSlash(loomengine.LoomDurableDirRel()) + "/"
	if !strings.Contains(got, "A\t"+prefix+"friction-20260101T000000/note.md") {
		t.Errorf("weft HEAD = %q; want the archive sibling added", got)
	}
	if !strings.Contains(got, "D\t"+prefix+"friction/note.md") {
		t.Errorf("weft HEAD = %q; want the old friction path deleted", got)
	}
	if st := mustGitOut(t, weftSibling, "status", "--porcelain", "--", filepath.ToSlash(loomengine.LoomDurableDirRel())); st != "" {
		t.Errorf("weft status under the loom durable directory = %q; want clean", st)
	}
}

// TestCommitStatusSeam_Real_PendingRejectionHoldsTheRound asserts that, with a rejection pending, a status commit lands neither the review run directory's deletion nor any round-<N>/ file, and that the next status commit after the rejection clears records both.
func TestCommitStatusSeam_Real_PendingRejectionHoldsTheRound(t *testing.T) {
	seam, location, weftSibling := realSeamFixture(t)
	reviewRel := "plan/round-1-review.md"
	writeReviewsDirFile(t, location, reviewRel, "record\n")
	// A file in another run directory keeps the reviews root non-empty after the move, so the released commit still names it.
	writeReviewsDirFile(t, location, "webster/round-1-review.md", "record\n")
	if err := seam("Plan-Review", "running"); err != nil {
		t.Fatalf("first seam error = %v; want nil", err)
	}

	reviewRunDir := filepath.Join(loomengine.LoomReviewsDir(location), "plan")
	roundDir := filepath.Join(loomengine.LoomReworkDir(location), "round-1")
	archived := filepath.Join(roundDir, "prior-generation", "reviews", "plan")
	if err := os.MkdirAll(filepath.Dir(archived), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(reviewRunDir, archived); err != nil {
		t.Fatal(err)
	}
	writeRecordFile(t, filepath.Join(roundDir, "record.json"), `{"first_card":3,"class":"exempt"}`+"\n")
	writeRecordFile(t, loomengine.LoomRejectionPath(location), "{}\n")

	if err := seam("Plan-Review", "paused"); err != nil {
		t.Fatalf("held seam error = %v; want nil", err)
	}
	reviewPath := filepath.ToSlash(filepath.Join(loomengine.LoomReviewsDirRel(), reviewRel))
	if got := mustGitOut(t, weftSibling, "ls-tree", "-r", "--name-only", "HEAD"); !strings.Contains(got, reviewPath) {
		t.Errorf("HEAD tree lacks %q; want the review run directory still held while the rejection is pending", reviewPath)
	} else if strings.Contains(got, filepath.ToSlash(loomengine.LoomReworkDirRel())+"/round-1/") {
		t.Errorf("HEAD tree holds a round-1 file while the rejection is pending:\n%s", got)
	}

	if err := os.Remove(loomengine.LoomRejectionPath(location)); err != nil {
		t.Fatal(err)
	}
	if err := seam("Plan-Review", "done"); err != nil {
		t.Fatalf("released seam error = %v; want nil", err)
	}
	got := mustGitOut(t, weftSibling, "show", "--no-renames", "--name-status", "--format=", "HEAD")
	if !strings.Contains(got, "D\t"+reviewPath) {
		t.Errorf("weft HEAD = %q; want the review run directory's deletion recorded once the rejection cleared", got)
	}
	if !strings.Contains(got, "A\t"+filepath.ToSlash(loomengine.LoomReworkDirRel())+"/round-1/record.json") {
		t.Errorf("weft HEAD = %q; want round-1/record.json recorded once the rejection cleared", got)
	}
}
