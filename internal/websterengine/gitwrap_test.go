//go:build integration

// gitwrap_test.go exercises headSHA, dirty, reconcileReportHead and refuseMidMerge against real scratch git repositories built fresh under t.TempDir() for each test,
// reusing the package's existing hermetic TestMain (testmain_test.go) so these git spawns never inherit the operator's global gitconfig.

package websterengine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/gitkit"
)

// gitwrapNewScratchRepo is the in-package scratch-repo initialiser; the external test package keeps its own, since the two packages cannot share a helper.
func gitwrapNewScratchRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	gitkit.Git(t, dir, "init")
	gitkit.Git(t, dir, "config", "user.name", "Test User")
	gitkit.Git(t, dir, "config", "user.email", "test@example.com")

	return dir
}

func TestHeadSHA_ReturnsHEAD(t *testing.T) {
	t.Parallel()

	dir := gitwrapNewScratchRepo(t)
	want := gitkit.CommitFile(t, dir, "a.txt", "one", "first")

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
	gitkit.CommitFile(t, dir, "a.txt", "one", "first")

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

// gitwrapParentBranch is the parent branch every reconcileReportHead test's run merges from.
const gitwrapParentBranch = "parent"

// gitwrapParent is the ParentBranchFunc naming gitwrapParentBranch.
func gitwrapParent() (string, error) { return gitwrapParentBranch, nil }

// gitwrapMergeSide checks out side (branching it off the current branch when it does not exist yet), commits one new file there,
// returns to the original branch and merges side with --no-ff, so a merge commit lands even without divergence.
// It returns the merge commit SHA and the side branch's own tip SHA.
func gitwrapMergeSide(t *testing.T, dir, side string) (mergeSHA, sideTip string) {
	t.Helper()
	base := strings.TrimSpace(gitkit.Git(t, dir, "rev-parse", "--abbrev-ref", "HEAD"))
	baseHead := strings.TrimSpace(gitkit.Git(t, dir, "rev-parse", "HEAD"))
	if _, _, exitCode, err := gitexec.RunGit([]string{"rev-parse", "--verify", "--quiet", "refs/heads/" + side}, dir); err == nil && exitCode == 0 {
		gitkit.Git(t, dir, "checkout", side)
	} else {
		gitkit.Git(t, dir, "checkout", "-b", side)
	}
	sideTip = gitkit.CommitFile(t, dir, side+"-"+baseHead[:12]+".txt", side, side+" commit")
	gitkit.Git(t, dir, "checkout", base)
	gitkit.Git(t, dir, "merge", "--no-ff", "-m", "merge "+side, side)
	return strings.TrimSpace(gitkit.Git(t, dir, "rev-parse", "HEAD")), sideTip
}

func TestReconcileReportHead_EqualIsFastPath(t *testing.T) {
	t.Parallel()
	dir := gitwrapNewScratchRepo(t)
	head := gitkit.CommitFile(t, dir, "a.txt", "one", "first")

	warning, err := reconcileReportHead(dir, head, "batch report x", gitwrapParent)
	if err != nil || warning != "" {
		t.Fatalf("reconcileReportHead() = (%q, %v); want (\"\", nil)", warning, err)
	}
}

func TestReconcileReportHead_MergesOnTopAccepted(t *testing.T) {
	t.Parallel()
	dir := gitwrapNewScratchRepo(t)
	report := gitkit.CommitFile(t, dir, "a.txt", "one", "first")
	merge1, _ := gitwrapMergeSide(t, dir, gitwrapParentBranch)

	warning, err := reconcileReportHead(dir, report, "batch report x", gitwrapParent)
	if err != nil {
		t.Fatalf("one merge: error = %v; want nil", err)
	}
	for _, want := range []string{"batch report x", report, merge1} {
		if !strings.Contains(warning, want) {
			t.Errorf("one merge: warning %q missing %q", warning, want)
		}
	}

	merge2, _ := gitwrapMergeSide(t, dir, gitwrapParentBranch)
	warning, err = reconcileReportHead(dir, report, "batch report x", gitwrapParent)
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
	gitkit.CommitFile(t, dir, "a.txt", "one", "first")
	report, _ := gitwrapMergeSide(t, dir, gitwrapParentBranch)
	merge2, _ := gitwrapMergeSide(t, dir, gitwrapParentBranch)

	warning, err := reconcileReportHead(dir, report, "batch report x", gitwrapParent)
	if err != nil {
		t.Fatalf("error = %v; want nil", err)
	}
	if !strings.Contains(warning, "("+merge2+")") {
		t.Errorf("warning %q; want the walked merges to be exactly (%s), without the report head", warning, merge2)
	}
}

func TestReconcileReportHead_Refusals(t *testing.T) {
	t.Parallel()

	t.Run("fast-forward onto side commits", func(t *testing.T) {
		t.Parallel()
		dir := gitwrapNewScratchRepo(t)
		report := gitkit.CommitFile(t, dir, "a.txt", "one", "first")
		base := strings.TrimSpace(gitkit.Git(t, dir, "rev-parse", "--abbrev-ref", "HEAD"))
		gitkit.Git(t, dir, "checkout", "-b", "side")
		gitkit.CommitFile(t, dir, "s.txt", "s", "side commit")
		gitkit.Git(t, dir, "checkout", base)
		gitkit.Git(t, dir, "merge", "--ff-only", "side")

		if _, err := reconcileReportHead(dir, report, "batch report x", gitwrapParent); err == nil {
			t.Fatal("error = nil; want refusal")
		}
	})

	t.Run("non-merge commit after report head", func(t *testing.T) {
		t.Parallel()
		dir := gitwrapNewScratchRepo(t)
		report := gitkit.CommitFile(t, dir, "a.txt", "one", "first")
		head := gitkit.CommitFile(t, dir, "b.txt", "two", "second")

		_, err := reconcileReportHead(dir, report, "batch report x", gitwrapParent)
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
		report := gitkit.CommitFile(t, dir, "a.txt", "one", "first")
		gitwrapMergeSide(t, dir, gitwrapParentBranch)
		gitkit.CommitFile(t, dir, "b.txt", "two", "second")
		head, _ := gitwrapMergeSide(t, dir, gitwrapParentBranch)

		_, err := reconcileReportHead(dir, report, "batch report x", gitwrapParent)
		if err == nil {
			t.Fatal("error = nil; want refusal")
		}
		for _, want := range []string{"does not match the worktree's actual HEAD", report, head, "only merge commits"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("error %q missing %q", err, want)
			}
		}
	})

	t.Run("report head only on merged-in side branch", func(t *testing.T) {
		t.Parallel()
		dir := gitwrapNewScratchRepo(t)
		gitkit.CommitFile(t, dir, "a.txt", "one", "first")
		_, sideTip := gitwrapMergeSide(t, dir, gitwrapParentBranch)

		if _, err := reconcileReportHead(dir, sideTip, "batch report x", gitwrapParent); err == nil {
			t.Fatal("error = nil; want refusal")
		}
	})

	t.Run("all-zero report head", func(t *testing.T) {
		t.Parallel()
		dir := gitwrapNewScratchRepo(t)
		gitkit.CommitFile(t, dir, "a.txt", "one", "first")
		gitwrapMergeSide(t, dir, gitwrapParentBranch)

		zero := strings.Repeat("0", 40)
		if _, err := reconcileReportHead(dir, zero, "batch report x", gitwrapParent); err == nil {
			t.Fatal("error = nil; want refusal")
		}
	})
}

// gitwrapParentCommit commits name=content on the parent branch (creating it off the current branch when absent) and returns to the current branch,
// returning the current branch's name.
func gitwrapParentCommit(t *testing.T, dir, name, content string) (base string) {
	t.Helper()
	base = strings.TrimSpace(gitkit.Git(t, dir, "rev-parse", "--abbrev-ref", "HEAD"))
	if _, _, exitCode, err := gitexec.RunGit([]string{"rev-parse", "--verify", "--quiet", "refs/heads/" + gitwrapParentBranch}, dir); err == nil && exitCode == 0 {
		gitkit.Git(t, dir, "checkout", gitwrapParentBranch)
	} else {
		gitkit.Git(t, dir, "checkout", "-b", gitwrapParentBranch)
	}
	gitkit.CommitFile(t, dir, name, content, "parent: "+name)
	gitkit.Git(t, dir, "checkout", base)
	return base
}

// TestReconcileReportHead_UncleanParentMergesRefused proves a merge commit is walked over only when it is a clean merge of the run's parent branch:
// every other two-or-more-parent shape is refused with the parent-merge refusal and its way forward.
func TestReconcileReportHead_UncleanParentMergesRefused(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		parent ParentBranchFunc
		// move builds the moved HEAD on top of the report head in dir.
		move       func(t *testing.T, dir string)
		wantReason string
	}{
		{
			name:   "evil merge carrying an extra edit",
			parent: gitwrapParent,
			move: func(t *testing.T, dir string) {
				gitwrapParentCommit(t, dir, "p.txt", "p")
				gitkit.Git(t, dir, "merge", "--no-ff", "--no-commit", gitwrapParentBranch)
				if err := os.WriteFile(filepath.Join(dir, "smuggled.txt"), []byte("unaudited"), 0o644); err != nil {
					t.Fatalf("write smuggled file: %v", err)
				}
				gitkit.Git(t, dir, "add", "smuggled.txt")
				gitkit.Git(t, dir, "commit", "--no-edit")
			},
			wantReason: "carries changes beyond a clean merge",
		},
		{
			name:   "merge of a non-parent branch",
			parent: gitwrapParent,
			move: func(t *testing.T, dir string) {
				gitwrapParentCommit(t, dir, "p.txt", "p")
				gitwrapMergeSide(t, dir, "other")
			},
			wantReason: "not on the run's parent branch",
		},
		{
			name:   "hand-made two-parent commit with an arbitrary tree",
			parent: gitwrapParent,
			move: func(t *testing.T, dir string) {
				gitwrapParentCommit(t, dir, "p.txt", "p")
				gitkit.CommitFile(t, dir, "scratch.txt", "arbitrary", "scratch")
				tree := strings.TrimSpace(gitkit.Git(t, dir, "rev-parse", "HEAD^{tree}"))
				gitkit.Git(t, dir, "reset", "--hard", "HEAD~1")
				forged := strings.TrimSpace(gitkit.Git(t, dir, "commit-tree", tree, "-p", "HEAD", "-p", gitwrapParentBranch, "-m", "forged merge"))
				gitkit.Git(t, dir, "reset", "--hard", forged)
			},
			wantReason: "carries changes beyond a clean merge",
		},
		{
			name:   "hand-resolved conflicting parent merge",
			parent: gitwrapParent,
			move: func(t *testing.T, dir string) {
				gitwrapParentCommit(t, dir, "a.txt", "parent side")
				gitkit.CommitFile(t, dir, "a.txt", "our side", "our side")
				if _, _, exitCode, err := gitexec.RunGit([]string{"merge", "--no-ff", gitwrapParentBranch}, dir); err != nil || exitCode == 0 {
					t.Fatalf("expected a conflicting merge; exit %d err %v", exitCode, err)
				}
				if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("resolved"), 0o644); err != nil {
					t.Fatalf("resolve conflict: %v", err)
				}
				gitkit.Git(t, dir, "add", "a.txt")
				gitkit.Git(t, dir, "commit", "--no-edit")
			},
			wantReason: "do not merge cleanly",
		},
		{
			name:   "octopus merge",
			parent: gitwrapParent,
			move: func(t *testing.T, dir string) {
				// Two independent lines, both merged into the parent branch, so each octopus head is reachable from its tip.
				base := strings.TrimSpace(gitkit.Git(t, dir, "rev-parse", "--abbrev-ref", "HEAD"))
				for _, line := range []string{"line-a", "line-b"} {
					gitkit.Git(t, dir, "checkout", "-b", line, base)
					gitkit.CommitFile(t, dir, line+".txt", line, line+" commit")
				}
				gitkit.Git(t, dir, "checkout", "-b", gitwrapParentBranch, "line-a")
				gitkit.Git(t, dir, "merge", "--no-ff", "-m", "parent takes line-b", "line-b")
				gitkit.Git(t, dir, "checkout", base)
				gitkit.Git(t, dir, "merge", "--no-ff", "-m", "octopus", "line-a", "line-b")
			},
			wantReason: "has 3 parents",
		},
		{
			name:   "no known parent branch",
			parent: nil,
			move: func(t *testing.T, dir string) {
				gitwrapMergeSide(t, dir, gitwrapParentBranch)
			},
			wantReason: "no known parent branch",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := gitwrapNewScratchRepo(t)
			report := gitkit.CommitFile(t, dir, "a.txt", "one", "first")
			tc.move(t, dir)
			head := strings.TrimSpace(gitkit.Git(t, dir, "rev-parse", "HEAD"))
			if head == report {
				t.Fatal("move left HEAD at the report head")
			}

			_, err := reconcileReportHead(dir, report, "batch report x", tc.parent)
			if err == nil {
				t.Fatal("error = nil; want refusal")
			}
			for _, want := range []string{"does not match the worktree's actual HEAD", report, head, "merge commit " + head + " does not qualify", tc.wantReason, "way forward: 1) run `git reset --keep " + report + "` to move HEAD back"} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q missing %q", err, want)
				}
			}
		})
	}
}

// TestReconcileReportHead_ParentOnlyOnOrigin proves a merge of the parent branch's origin remote-tracking tip is accepted when no local parent branch exists.
func TestReconcileReportHead_ParentOnlyOnOrigin(t *testing.T) {
	t.Parallel()
	dir := gitwrapNewScratchRepo(t)
	report := gitkit.CommitFile(t, dir, "a.txt", "one", "first")
	merge, sideTip := gitwrapMergeSide(t, dir, gitwrapParentBranch)
	gitkit.Git(t, dir, "update-ref", "refs/remotes/origin/"+gitwrapParentBranch, sideTip)
	gitkit.Git(t, dir, "branch", "-D", gitwrapParentBranch)

	warning, err := reconcileReportHead(dir, report, "batch report x", gitwrapParent)
	if err != nil {
		t.Fatalf("error = %v; want nil", err)
	}
	if !strings.Contains(warning, "("+merge+")") {
		t.Errorf("warning %q; want the walked merge (%s)", warning, merge)
	}
}

// gitwrapConflictingMerge sets up a conflict on file c.txt between the current branch and a side branch, leaving the merge in progress in dir.
func gitwrapConflictingMerge(t *testing.T, dir string) {
	t.Helper()
	base := strings.TrimSpace(gitkit.Git(t, dir, "rev-parse", "--abbrev-ref", "HEAD"))
	gitkit.CommitFile(t, dir, "c.txt", "base", "add c")
	gitkit.Git(t, dir, "checkout", "-b", "conflict-side")
	gitkit.CommitFile(t, dir, "c.txt", "side", "side c")
	gitkit.Git(t, dir, "checkout", base)
	gitkit.CommitFile(t, dir, "c.txt", "main", "main c")
	if _, _, exitCode, err := gitexec.RunGit([]string{"merge", "conflict-side"}, dir); err != nil || exitCode == 0 {
		t.Fatalf("expected a conflicting merge; exit %d err %v", exitCode, err)
	}
}

func TestRefuseMidMerge(t *testing.T) {
	t.Parallel()
	dir := gitwrapNewScratchRepo(t)
	gitkit.CommitFile(t, dir, "a.txt", "one", "first")

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

	gitkit.Git(t, dir, "merge", "--abort")
	if err := refuseMidMerge(dir); err != nil {
		t.Fatalf("after abort: error = %v; want nil", err)
	}
}

func TestRefuseMidMerge_LinkedWorktree(t *testing.T) {
	t.Parallel()
	dir := gitwrapNewScratchRepo(t)
	gitkit.CommitFile(t, dir, "a.txt", "one", "first")
	linked := filepath.Join(t.TempDir(), "linked")
	gitkit.Git(t, dir, "worktree", "add", "-b", "linked-branch", linked)
	gitkit.Git(t, linked, "config", "user.name", "Test User")
	gitkit.Git(t, linked, "config", "user.email", "test@example.com")

	gitwrapConflictingMerge(t, linked)
	err := refuseMidMerge(linked)
	if err == nil || !strings.Contains(err.Error(), "merge in progress") {
		t.Fatalf("linked worktree: error = %v; want merge-in-progress refusal", err)
	}
}

func TestOtherWorktrees(t *testing.T) {
	main := gitwrapNewScratchRepo(t)
	gitkit.CommitFile(t, main, "a.txt", "x", "add a")
	added := filepath.Join(t.TempDir(), "added")
	gitkit.Git(t, main, "worktree", "add", added)

	mainCanon, err := canonicalPath(main)
	if err != nil {
		t.Fatal(err)
	}
	addedCanon, err := canonicalPath(added)
	if err != nil {
		t.Fatal(err)
	}

	got, err := otherWorktrees(main)
	if err != nil {
		t.Fatalf("otherWorktrees(main): %v", err)
	}
	if len(got) != 1 || got[0] != addedCanon {
		t.Errorf("otherWorktrees(main) = %v; want [%s]", got, addedCanon)
	}
	got, err = otherWorktrees(added)
	if err != nil {
		t.Fatalf("otherWorktrees(added): %v", err)
	}
	if len(got) != 1 || got[0] != mainCanon {
		t.Errorf("otherWorktrees(added) = %v; want [%s]", got, mainCanon)
	}
}
