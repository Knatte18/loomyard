//go:build integration

// gitrepo_test.go covers the read/commit primitives (CurrentSHA,
// StageAndCommit, StageAllAndCommit, ChangedFilesSince, SHAExists,
// CurrentBranch) against real git repositories built under t.TempDir(). Every
// test spawns real git, so this file requires the hermetic TestMain in
// testmain_test.go.

package gitrepo_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/gitrepo"
)

// newRepo creates a fresh git repository on branch main and returns both
// the raw directory and a gitrepo.Repo wrapping it.
func newRepo(t *testing.T) (dir string, repo *gitrepo.Repo) {
	t.Helper()

	dir = t.TempDir()
	gitkit.MustRun(t, dir, "git", "init", "-b", "main")
	return dir, gitrepo.New(dir)
}

// writeFile creates or overwrites a file with the given content.
func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()

	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

// commitAll stages and commits all changes in dir via git, bypassing the
// Repo under test — used only for fixture setup.
func commitAll(t *testing.T, dir, message string) {
	t.Helper()

	gitkit.Git(t, dir, "add", ".")
	gitkit.Git(t, dir, "commit", "-m", message)
}

// runGit runs a git subcommand in dir, returning stdout and stderr.
func runGit(t *testing.T, dir string, args ...string) (stdout, stderr string, code int, err error) {
	t.Helper()

	return gitexec.RunGit(args, dir)
}

// runGitStatus returns the porcelain status output for dir.
func runGitStatus(t *testing.T, dir string) (stdout, stderr string, code int, err error) {
	t.Helper()

	return runGit(t, dir, "status", "--porcelain")
}

// statusOf returns the porcelain status output for dir, failing the test on a git error.
func statusOf(t *testing.T, dir string) string {
	t.Helper()

	stdout, stderr, code, err := runGitStatus(t, dir)
	if err != nil {
		t.Fatalf("git status error = %v", err)
	}
	if code != 0 {
		t.Fatalf("git status exited %d: %s", code, stderr)
	}
	return stdout
}

// headFilesOf returns the file names the HEAD commit touches, one per line, failing the test on a
// git error.
func headFilesOf(t *testing.T, dir string) string {
	t.Helper()

	shown, stderr, code, err := runGit(t, dir, "show", "--name-only", "--format=", "HEAD")
	if err != nil {
		t.Fatalf("git show error = %v", err)
	}
	if code != 0 {
		t.Fatalf("git show exited %d: %s", code, stderr)
	}
	return strings.TrimSpace(shown)
}

// mergeHeadPresent reports whether a merge is pending in dir.
func mergeHeadPresent(t *testing.T, dir string) bool {
	t.Helper()

	_, _, code, _ := runGit(t, dir, "rev-parse", "--verify", "--quiet", "MERGE_HEAD")
	return code == 0
}

// requireCurrentSHA returns repo's HEAD, failing the test on an error.
func requireCurrentSHA(t *testing.T, repo *gitrepo.Repo) string {
	t.Helper()

	sha, err := repo.CurrentSHA()
	if err != nil {
		t.Fatalf("CurrentSHA() error = %v", err)
	}
	return sha
}

// requireSameFiles fails the test unless got lists exactly the files in want, in any order.
func requireSameFiles(t *testing.T, call string, got []string, want ...string) {
	t.Helper()

	wantSet := map[string]bool{}
	for _, f := range want {
		wantSet[f] = true
	}
	if len(got) != len(wantSet) {
		t.Fatalf("%s = %v; want exactly %v", call, got, want)
	}
	for _, f := range got {
		if !wantSet[f] {
			t.Errorf("%s contains unexpected file %q", call, f)
		}
	}
}

// TestCurrentSHA covers an unborn HEAD and a committed one in a single repository.
// The steps run serially against shared repository state: the second step relies on the commit it
// makes itself, after the first has seen the repository empty.
// The top-level test calls t.Parallel; no step does, because the steps share the repository.
func TestCurrentSHA(t *testing.T) {
	t.Parallel()

	dir, repo := newRepo(t)

	if !t.Run("empty repository returns ErrNoCommits", func(t *testing.T) {
		if _, err := repo.CurrentSHA(); !errors.Is(err, gitrepo.ErrNoCommits) {
			t.Fatalf("CurrentSHA() error = %v; want errors.Is(err, ErrNoCommits)", err)
		}
	}) {
		return
	}

	t.Run("returns HEAD", func(t *testing.T) {
		writeFile(t, dir, "a.txt", "hello")
		commitAll(t, dir, "init")

		if got := requireCurrentSHA(t, repo); got == "" {
			t.Fatal("CurrentSHA() = \"\"; want a non-empty SHA")
		}
	})
}

// TestCommitAndReadPrimitives drives StageAndCommit, StageAllAndCommit, ChangedFilesSince,
// SHAExists, CurrentBranch and the mid-merge refusal through one repository.
// The steps run serially in one order and share the repository's state: every step starts from a
// clean tree on main with the files a.txt and b.txt tracked, except where a comment names the dirt
// an earlier step leaves for the next, and the mid-merge step runs last because it leaves a merge
// pending.
// The top-level test calls t.Parallel; no step does, because the steps share the repository.
func TestCommitAndReadPrimitives(t *testing.T) {
	t.Parallel()

	dir, repo := newRepo(t)
	writeFile(t, dir, "a.txt", "initial")
	writeFile(t, dir, "b.txt", "initial")
	commitAll(t, dir, "init")

	steps := []struct {
		name string
		run  func(t *testing.T)
	}{
		{"StageAndCommit with unchanged files reports nothing to commit", func(t *testing.T) {
			sha, committed, err := repo.StageAndCommit("no-op", []string{"a.txt"})
			if err != nil {
				t.Fatalf("StageAndCommit() error = %v; want nil", err)
			}
			if committed || sha != "" {
				t.Errorf("StageAndCommit() = (%q, %v); want (\"\", false) (nothing-to-commit signal)", sha, committed)
			}
		}},
		// Leaves wip.txt staged for the next step: an index entry staged outside the call is a
		// human's half-staged WIP in the shared worktree.
		{"StageAndCommit with an empty list never commits a pre-staged entry", func(t *testing.T) {
			writeFile(t, dir, "wip.txt", "half-staged WIP")
			gitkit.MustRun(t, dir, "git", "add", "wip.txt")
			headBefore := requireCurrentSHA(t, repo)

			for _, files := range [][]string{nil, {}} {
				sha, committed, err := repo.StageAndCommit("must not happen", files)
				if err != nil {
					t.Fatalf("StageAndCommit(%v) error = %v; want nil", files, err)
				}
				if committed || sha != "" {
					t.Errorf("StageAndCommit(%v) = (%q, %v); want (\"\", false)", files, sha, committed)
				}
			}

			if headAfter := requireCurrentSHA(t, repo); headAfter != headBefore {
				t.Errorf("HEAD after empty-list StageAndCommit = %q; want unchanged %q", headAfter, headBefore)
			}
		}},
		// Relies on wip.txt staged by the previous step; leaves wip.txt, b.txt and c.txt dirty for
		// the StageAllAndCommit step.
		{"StageAndCommit commits only the listed files", func(t *testing.T) {
			base := requireCurrentSHA(t, repo)
			writeFile(t, dir, "a.txt", "changed")
			writeFile(t, dir, "b.txt", "also changed")
			writeFile(t, dir, "c.txt", "untracked")

			sha, committed, err := repo.StageAndCommit("commit a only", []string{"a.txt"})
			if err != nil {
				t.Fatalf("StageAndCommit() error = %v; want nil", err)
			}
			if !committed || sha == "" {
				t.Fatalf("StageAndCommit() = (%q, %v); want a real commit of a.txt", sha, committed)
			}
			if got := requireCurrentSHA(t, repo); got != sha {
				t.Errorf("CurrentSHA() = %q; want %q (StageAndCommit's returned sha)", got, sha)
			}

			// The new commit holds a.txt only: never the modified b.txt, the untracked c.txt or the
			// pre-staged wip.txt.
			if got := headFilesOf(t, dir); got != "a.txt" {
				t.Errorf("git show --name-only HEAD = %q; want only a.txt", got)
			}
			changed, err := repo.ChangedFilesSince(base)
			if err != nil {
				t.Fatalf("ChangedFilesSince() error = %v", err)
			}
			requireSameFiles(t, "ChangedFilesSince()", changed, "a.txt")

			status := statusOf(t, dir)
			for _, dirty := range []string{"b.txt", "c.txt", "wip.txt"} {
				if !strings.Contains(status, dirty) {
					t.Errorf("git status --porcelain = %q; want it to still list %s as dirty", status, dirty)
				}
			}
		}},
		// Relies on the dirt the previous step left; a.txt is modified again so the sweep also
		// covers a tracked modification. Leaves a clean tree.
		{"StageAllAndCommit commits every dirty file, including what an explicit list left", func(t *testing.T) {
			writeFile(t, dir, "a.txt", "changed again")

			sha, committed, err := repo.StageAllAndCommit("stage everything")
			if err != nil {
				t.Fatalf("StageAllAndCommit() error = %v; want nil", err)
			}
			if !committed || sha == "" {
				t.Fatalf("StageAllAndCommit() = (%q, %v); want a real commit", sha, committed)
			}
			if got := requireCurrentSHA(t, repo); got != sha {
				t.Errorf("CurrentSHA() = %q; want %q (StageAllAndCommit's returned sha)", got, sha)
			}
			if status := statusOf(t, dir); status != "" {
				t.Errorf("git status --porcelain = %q; want empty (working tree clean)", status)
			}
			shown := headFilesOf(t, dir)
			for _, captured := range []string{"a.txt", "b.txt", "c.txt", "wip.txt"} {
				if !strings.Contains(shown, captured) {
					t.Errorf("git show --name-only HEAD = %q; want %s captured by StageAllAndCommit", shown, captured)
				}
			}
		}},
		{"StageAllAndCommit with a clean tree reports nothing to commit", func(t *testing.T) {
			headBefore := requireCurrentSHA(t, repo)

			sha, committed, err := repo.StageAllAndCommit("must not happen")
			if err != nil {
				t.Fatalf("StageAllAndCommit() error = %v; want nil", err)
			}
			if committed || sha != "" {
				t.Errorf("StageAllAndCommit() = (%q, %v); want (\"\", false)", sha, committed)
			}
			if headAfter := requireCurrentSHA(t, repo); headAfter != headBefore {
				t.Errorf("HEAD after clean-tree StageAllAndCommit = %q; want unchanged %q", headAfter, headBefore)
			}
		}},
		{"ChangedFilesSince returns the files changed since a commit", func(t *testing.T) {
			base := requireCurrentSHA(t, repo)
			writeFile(t, dir, "a.txt", "changed")
			writeFile(t, dir, "new.txt", "new")
			commitAll(t, dir, "second commit")

			got, err := repo.ChangedFilesSince(base)
			if err != nil {
				t.Fatalf("ChangedFilesSince() error = %v; want nil", err)
			}
			requireSameFiles(t, "ChangedFilesSince()", got, "a.txt", "new.txt")
		}},
		{"ChangedFilesSince is empty when the sha equals HEAD", func(t *testing.T) {
			got, err := repo.ChangedFilesSince(requireCurrentSHA(t, repo))
			if err != nil {
				t.Fatalf("ChangedFilesSince() error = %v; want nil", err)
			}
			if len(got) != 0 {
				t.Errorf("ChangedFilesSince(HEAD) = %v; want empty", got)
			}
		}},
		{"ChangedFilesSince excludes an uncommitted edit", func(t *testing.T) {
			base := requireCurrentSHA(t, repo)
			writeFile(t, dir, "a.txt", "uncommitted edit")

			got, err := repo.ChangedFilesSince(base)
			if err != nil {
				t.Fatalf("ChangedFilesSince() error = %v; want nil", err)
			}
			if len(got) != 0 {
				t.Errorf("ChangedFilesSince() = %v; want empty (uncommitted edits are excluded)", got)
			}
			gitkit.MustRun(t, dir, "git", "checkout", "--", "a.txt")
		}},
		// A filename outside ASCII must come back as the literal on-disk path, not
		// core.quotePath's C-quoted escape form ("\"bl\\303\\245b\\303\\246r.txt\"") that matches
		// nothing on disk.
		{"ChangedFilesSince returns a non-ASCII path verbatim", func(t *testing.T) {
			base := requireCurrentSHA(t, repo)
			const name = "blåbær.txt"
			writeFile(t, dir, name, "berries")
			commitAll(t, dir, "add non-ascii filename")

			got, err := repo.ChangedFilesSince(base)
			if err != nil {
				t.Fatalf("ChangedFilesSince() error = %v; want nil", err)
			}
			if len(got) != 1 || got[0] != name {
				t.Errorf("ChangedFilesSince() = %q; want [%q] verbatim", got, name)
			}
		}},
		// A rename must list both the old path (which no longer exists at HEAD) and the new one;
		// git's default rename detection would report only the destination, leaving a consumer's
		// per-file state for the old path stale forever.
		{"ChangedFilesSince reports both paths of a rename", func(t *testing.T) {
			writeFile(t, dir, "old.txt", "content that stays identical")
			commitAll(t, dir, "add old.txt")
			base := requireCurrentSHA(t, repo)

			// A pure rename (identical content) is the case rename detection folds.
			gitkit.MustRun(t, dir, "git", "mv", "old.txt", "renamed.txt")
			gitkit.MustRun(t, dir, "git", "commit", "-m", "rename")

			got, err := repo.ChangedFilesSince(base)
			if err != nil {
				t.Fatalf("ChangedFilesSince() error = %v; want nil", err)
			}
			requireSameFiles(t, "ChangedFilesSince()", got, "old.txt", "renamed.txt")
		}},
		{"ChangedFilesSince errors on a fabricated sha", func(t *testing.T) {
			if _, err := repo.ChangedFilesSince("0123456789abcdef0123456789abcdef01234567"); err == nil {
				t.Fatal("ChangedFilesSince(fabricated sha) error = nil; want an error")
			}
		}},
		{"SHAExists", func(t *testing.T) {
			tests := []struct {
				name string
				sha  string
				want bool
			}{
				{"RealSHA", requireCurrentSHA(t, repo), true},
				{"FabricatedSHA", "0123456789abcdef0123456789abcdef01234567", false},
				{"GarbageInput", "not-a-sha at all!!", false},
			}
			for _, tt := range tests {
				if got := repo.SHAExists(tt.sha); got != tt.want {
					t.Errorf("SHAExists(%s %q) = %v; want %v", tt.name, tt.sha, got, tt.want)
				}
			}
		}},
		// CurrentBranch must surface an error rather than an empty string on a detached HEAD, so a
		// caller can never mistake "no branch captured" for a legitimate empty branch name.
		// Reattaches main afterwards.
		{"CurrentBranch errors on a detached HEAD", func(t *testing.T) {
			gitkit.MustRun(t, dir, "git", "checkout", "--detach", requireCurrentSHA(t, repo))
			defer gitkit.MustRun(t, dir, "git", "checkout", "main")

			if _, err := repo.CurrentBranch(); err == nil {
				t.Fatal("CurrentBranch() on detached HEAD error = nil; want non-nil")
			}
		}},
		// While a merge is in progress (MERGE_HEAD present), a pathspec-scoped StageAndCommit of an
		// unrelated file is refused by git ("cannot do a partial commit during a merge") rather than
		// silently finalizing the human's half-done merge under the automated message.
		// The merge is clean and resolved (git merge --no-commit of a non-conflicting branch) on
		// purpose: an unresolved conflict would block any commit and mask the distinction.
		// It guards the pathspec scoping of the commit: `commit -- <files>` refuses a partial
		// commit mid-merge, whereas an unscoped `git commit` would complete the merge, so the merge
		// must still be pending afterward.
		// Runs last: it leaves the merge pending.
		{"StageAndCommit mid-merge refuses a partial commit", func(t *testing.T) {
			// A feature branch adds feat.txt while main edits a different file, so the merge is
			// non-conflicting and leaves a clean, fully-resolved index.
			gitkit.MustRun(t, dir, "git", "checkout", "-b", "feature")
			writeFile(t, dir, "feat.txt", "feature\n")
			commitAll(t, dir, "feature edit")
			gitkit.MustRun(t, dir, "git", "checkout", "main")
			writeFile(t, dir, "b.txt", "main edit\n")
			commitAll(t, dir, "main edit")

			// --no-commit stops after merging into the index/worktree, so MERGE_HEAD is set with a
			// clean index: the mid-merge state a commit could finalize.
			gitkit.MustRun(t, dir, "git", "merge", "--no-commit", "feature")
			if !mergeHeadPresent(t, dir) {
				t.Fatal("MERGE_HEAD not present after --no-commit merge; test needs a mid-merge state")
			}

			writeFile(t, dir, "other.txt", "written by caller\n")
			sha, committed, err := repo.StageAndCommit("automated commit during merge", []string{"other.txt"})
			if err == nil {
				t.Fatal("StageAndCommit() mid-merge error = nil; want git's partial-commit refusal")
			}
			if committed || sha != "" {
				t.Errorf("StageAndCommit() mid-merge = (%q, %v); want (\"\", false) -- no commit", sha, committed)
			}
			if !mergeHeadPresent(t, dir) {
				t.Error("MERGE_HEAD absent after refused StageAndCommit; the merge must not have been completed")
			}
		}},
	}
	for _, step := range steps {
		if !t.Run(step.name, step.run) {
			return
		}
	}
}
