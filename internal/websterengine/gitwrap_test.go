//go:build integration

// gitwrap_test.go exercises headSHA, dirty, reconcileReportHead and refuseMidMerge against real
// scratch git repositories built fresh under t.TempDir() for each test,
// reusing the package's existing hermetic TestMain (testmain_test.go) so
// these git spawns never inherit the operator's global gitconfig.

package websterengine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitexec"
)

// gitwrapNewScratchRepo initializes a fresh git repo in a t.TempDir() and
// configures a throwaway committer identity, returning its path.
func gitwrapNewScratchRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	gitwrapMustGit(t, dir, "init")
	gitwrapMustGit(t, dir, "config", "user.name", "Test User")
	gitwrapMustGit(t, dir, "config", "user.email", "test@example.com")

	return dir
}

// gitwrapMustGit runs a git command in dir via gitexec.RunGit, failing the
// test on any spawn error or non-zero exit, and returns stdout.
func gitwrapMustGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	stdout, stderr, exitCode, err := gitexec.RunGit(args, dir)
	if err != nil {
		t.Fatalf("git %v in %s: %v", args, dir, err)
	}
	if exitCode != 0 {
		t.Fatalf("git %v in %s exited %d: %s", args, dir, exitCode, stderr)
	}
	return stdout
}

// gitwrapCommitFile writes name=content into dir and commits it with
// message, returning the resulting commit SHA.
func gitwrapCommitFile(t *testing.T, dir, name, content, message string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir for %s: %v", name, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	gitwrapMustGit(t, dir, "add", name)
	gitwrapMustGit(t, dir, "commit", "-m", message)
	return strings.TrimSpace(gitwrapMustGit(t, dir, "rev-parse", "HEAD"))
}

func TestHeadSHA_ReturnsHEAD(t *testing.T) {
	t.Parallel()

	dir := gitwrapNewScratchRepo(t)
	want := gitwrapCommitFile(t, dir, "a.txt", "one", "first")

	got, err := headSHA(dir)
	if err != nil {
		t.Fatalf("headSHA() error = %v; want nil", err)
	}
	if got != want {
		t.Errorf("headSHA() = %q; want %q", got, want)
	}
}

func TestDirty_TrueAndFalse(t *testing.T) {
	t.Parallel()

	dir := gitwrapNewScratchRepo(t)
	gitwrapCommitFile(t, dir, "a.txt", "one", "first")

	clean, err := dirty(dir)
	if err != nil {
		t.Fatalf("dirty() error = %v; want nil", err)
	}
	if clean {
		t.Errorf("dirty() = true right after a commit with no other changes; want false")
	}

	if err := os.WriteFile(filepath.Join(dir, "untracked.txt"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write untracked file: %v", err)
	}

	isDirty, err := dirty(dir)
	if err != nil {
		t.Fatalf("dirty() error = %v; want nil", err)
	}
	if !isDirty {
		t.Errorf("dirty() = false with an untracked file present; want true")
	}
}

// gitwrapMergeSide branches off the current branch as side, commits one file there, returns to the
// original branch and merges side with --no-ff, so a merge commit lands even without divergence.
// It returns the merge commit SHA and the side branch's own tip SHA.
func gitwrapMergeSide(t *testing.T, dir, side string) (mergeSHA, sideTip string) {
	t.Helper()
	base := strings.TrimSpace(gitwrapMustGit(t, dir, "rev-parse", "--abbrev-ref", "HEAD"))
	gitwrapMustGit(t, dir, "checkout", "-b", side)
	sideTip = gitwrapCommitFile(t, dir, side+".txt", side, side+" commit")
	gitwrapMustGit(t, dir, "checkout", base)
	gitwrapMustGit(t, dir, "merge", "--no-ff", "-m", "merge "+side, side)
	return strings.TrimSpace(gitwrapMustGit(t, dir, "rev-parse", "HEAD")), sideTip
}

func TestReconcileReportHead_EqualIsFastPath(t *testing.T) {
	t.Parallel()
	dir := gitwrapNewScratchRepo(t)
	head := gitwrapCommitFile(t, dir, "a.txt", "one", "first")

	warning, err := reconcileReportHead(dir, head, "batch report x")
	if err != nil || warning != "" {
		t.Fatalf("reconcileReportHead() = (%q, %v); want (\"\", nil)", warning, err)
	}
}

func TestReconcileReportHead_MergesOnTopAccepted(t *testing.T) {
	t.Parallel()
	dir := gitwrapNewScratchRepo(t)
	report := gitwrapCommitFile(t, dir, "a.txt", "one", "first")
	merge1, _ := gitwrapMergeSide(t, dir, "side1")

	warning, err := reconcileReportHead(dir, report, "batch report x")
	if err != nil {
		t.Fatalf("one merge: error = %v; want nil", err)
	}
	for _, want := range []string{"batch report x", report, merge1} {
		if !strings.Contains(warning, want) {
			t.Errorf("one merge: warning %q missing %q", warning, want)
		}
	}

	merge2, _ := gitwrapMergeSide(t, dir, "side2")
	warning, err = reconcileReportHead(dir, report, "batch report x")
	if err != nil {
		t.Fatalf("two merges: error = %v; want nil", err)
	}
	i2, i1 := strings.Index(warning, merge2), strings.Index(warning, merge1)
	if i2 < 0 || i1 < 0 || i2 > i1 {
		t.Errorf("two merges: warning %q must name %s then %s", warning, merge2, merge1)
	}
}

func TestReconcileReportHead_ReportHeadIsMergeCommit(t *testing.T) {
	t.Parallel()
	dir := gitwrapNewScratchRepo(t)
	gitwrapCommitFile(t, dir, "a.txt", "one", "first")
	report, _ := gitwrapMergeSide(t, dir, "side1")
	merge2, _ := gitwrapMergeSide(t, dir, "side2")

	warning, err := reconcileReportHead(dir, report, "batch report x")
	if err != nil {
		t.Fatalf("error = %v; want nil", err)
	}
	if !strings.Contains(warning, merge2) {
		t.Errorf("warning %q missing second merge %s", warning, merge2)
	}
	if strings.Contains(warning, "("+report) || strings.Contains(warning, ", "+report+",") {
		t.Errorf("warning %q lists the report head as a walked merge", warning)
	}
}

func TestReconcileReportHead_Refusals(t *testing.T) {
	t.Parallel()

	t.Run("fast-forward onto side commits", func(t *testing.T) {
		t.Parallel()
		dir := gitwrapNewScratchRepo(t)
		report := gitwrapCommitFile(t, dir, "a.txt", "one", "first")
		base := strings.TrimSpace(gitwrapMustGit(t, dir, "rev-parse", "--abbrev-ref", "HEAD"))
		gitwrapMustGit(t, dir, "checkout", "-b", "side")
		gitwrapCommitFile(t, dir, "s.txt", "s", "side commit")
		gitwrapMustGit(t, dir, "checkout", base)
		gitwrapMustGit(t, dir, "merge", "--ff-only", "side")

		if _, err := reconcileReportHead(dir, report, "batch report x"); err == nil {
			t.Fatal("error = nil; want refusal")
		}
	})

	t.Run("non-merge commit after report head", func(t *testing.T) {
		t.Parallel()
		dir := gitwrapNewScratchRepo(t)
		report := gitwrapCommitFile(t, dir, "a.txt", "one", "first")
		head := gitwrapCommitFile(t, dir, "b.txt", "two", "second")

		_, err := reconcileReportHead(dir, report, "batch report x")
		if err == nil {
			t.Fatal("error = nil; want refusal")
		}
		for _, want := range []string{"does not match the worktree's actual HEAD", report, head, "merge"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("error %q missing %q", err, want)
			}
		}
	})

	t.Run("non-merge commit between merges", func(t *testing.T) {
		t.Parallel()
		dir := gitwrapNewScratchRepo(t)
		report := gitwrapCommitFile(t, dir, "a.txt", "one", "first")
		gitwrapMergeSide(t, dir, "side1")
		gitwrapCommitFile(t, dir, "b.txt", "two", "second")
		gitwrapMergeSide(t, dir, "side2")

		if _, err := reconcileReportHead(dir, report, "batch report x"); err == nil {
			t.Fatal("error = nil; want refusal")
		}
	})

	t.Run("report head only on merged-in side branch", func(t *testing.T) {
		t.Parallel()
		dir := gitwrapNewScratchRepo(t)
		gitwrapCommitFile(t, dir, "a.txt", "one", "first")
		_, sideTip := gitwrapMergeSide(t, dir, "side1")

		if _, err := reconcileReportHead(dir, sideTip, "batch report x"); err == nil {
			t.Fatal("error = nil; want refusal")
		}
	})

	t.Run("all-zero report head", func(t *testing.T) {
		t.Parallel()
		dir := gitwrapNewScratchRepo(t)
		gitwrapCommitFile(t, dir, "a.txt", "one", "first")
		gitwrapMergeSide(t, dir, "side1")

		zero := strings.Repeat("0", 40)
		if _, err := reconcileReportHead(dir, zero, "batch report x"); err == nil {
			t.Fatal("error = nil; want refusal")
		}
	})
}

// gitwrapConflictingMerge sets up a conflict on file c.txt between the current branch and a side
// branch, leaving the merge in progress in dir.
func gitwrapConflictingMerge(t *testing.T, dir string) {
	t.Helper()
	base := strings.TrimSpace(gitwrapMustGit(t, dir, "rev-parse", "--abbrev-ref", "HEAD"))
	gitwrapCommitFile(t, dir, "c.txt", "base", "add c")
	gitwrapMustGit(t, dir, "checkout", "-b", "conflict-side")
	gitwrapCommitFile(t, dir, "c.txt", "side", "side c")
	gitwrapMustGit(t, dir, "checkout", base)
	gitwrapCommitFile(t, dir, "c.txt", "main", "main c")
	if _, _, exitCode, err := gitexec.RunGit([]string{"merge", "conflict-side"}, dir); err != nil || exitCode == 0 {
		t.Fatalf("expected a conflicting merge; exit %d err %v", exitCode, err)
	}
}

func TestRefuseMidMerge(t *testing.T) {
	t.Parallel()
	dir := gitwrapNewScratchRepo(t)
	gitwrapCommitFile(t, dir, "a.txt", "one", "first")

	if err := refuseMidMerge(dir); err != nil {
		t.Fatalf("clean repo: error = %v; want nil", err)
	}

	gitwrapConflictingMerge(t, dir)
	err := refuseMidMerge(dir)
	if err == nil {
		t.Fatal("mid-merge: error = nil; want refusal")
	}
	for _, want := range []string{"lyx fabric merge --continue", "lyx fabric merge --abort", "git merge --continue", "git merge --abort"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q missing %q", err, want)
		}
	}

	gitwrapMustGit(t, dir, "merge", "--abort")
	if err := refuseMidMerge(dir); err != nil {
		t.Fatalf("after abort: error = %v; want nil", err)
	}
}

func TestRefuseMidMerge_LinkedWorktree(t *testing.T) {
	t.Parallel()
	dir := gitwrapNewScratchRepo(t)
	gitwrapCommitFile(t, dir, "a.txt", "one", "first")
	linked := filepath.Join(t.TempDir(), "linked")
	gitwrapMustGit(t, dir, "worktree", "add", "-b", "linked-branch", linked)
	gitwrapMustGit(t, linked, "config", "user.name", "Test User")
	gitwrapMustGit(t, linked, "config", "user.email", "test@example.com")

	gitwrapConflictingMerge(t, linked)
	err := refuseMidMerge(linked)
	if err == nil || !strings.Contains(err.Error(), "merge in progress") {
		t.Fatalf("linked worktree: error = %v; want merge-in-progress refusal", err)
	}
}
