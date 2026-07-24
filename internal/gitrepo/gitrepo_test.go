//go:build integration

// gitrepo_test.go covers the read/commit primitives (CurrentSHA,
// StageAndCommit, ChangedFilesSince, SHAExists) against real git repositories
// built fresh under t.TempDir() for each test. Every test spawns real git, so
// this file requires the hermetic TestMain in testmain_test.go.

package gitrepo_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/lyxtest"
)

// newRepo creates a fresh git repository on branch main under a temp
// directory and returns both the raw directory (for fixture setup via
// lyxtest.MustRun) and a gitrepo.Repo wrapping it (the type under test).
func newRepo(t *testing.T) (dir string, repo *gitrepo.Repo) {
	t.Helper()

	dir = t.TempDir()
	lyxtest.MustRun(t, dir, "git", "init", "-b", "main")
	return dir, gitrepo.New(dir)
}

// writeFile creates (or overwrites) name under dir with the given content.
func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()

	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

// commitAll stages every change in dir and commits it directly via git,
// bypassing the Repo under test — used only to build fixture history that a
// test's assertions do not themselves cover.
func commitAll(t *testing.T, dir, message string) {
	t.Helper()

	lyxtest.MustRun(t, dir, "git", "add", ".")
	lyxtest.MustRun(t, dir, "git", "commit", "-m", message)
}

// runGit runs a git subcommand in dir via gitexec.RunGit, failing the test
// on a spawn error. It exists so assertion helpers can inspect git's stdout
// directly (lyxtest.MustRun only fails-or-succeeds; it discards output).
func runGit(t *testing.T, dir string, args ...string) (stdout, stderr string, code int, err error) {
	t.Helper()

	return gitexec.RunGit(args, dir)
}

// runGitStatus returns the porcelain status output for dir, used to assert
// that a file StageAndCommit did not stage is still reported as dirty.
func runGitStatus(t *testing.T, dir string) (stdout, stderr string, code int, err error) {
	t.Helper()

	return runGit(t, dir, "status", "--porcelain")
}

func TestCurrentSHA_ReturnsHEAD(t *testing.T) {
	dir, repo := newRepo(t)
	writeFile(t, dir, "a.txt", "hello")
	commitAll(t, dir, "init")

	got, err := repo.CurrentSHA()
	if err != nil {
		t.Fatalf("CurrentSHA() error = %v; want nil", err)
	}
	if got == "" {
		t.Fatal("CurrentSHA() = \"\"; want a non-empty SHA")
	}
}

func TestCurrentSHA_EmptyRepo_ReturnsErrNoCommits(t *testing.T) {
	_, repo := newRepo(t)

	_, err := repo.CurrentSHA()
	if !errors.Is(err, gitrepo.ErrNoCommits) {
		t.Fatalf("CurrentSHA() error = %v; want errors.Is(err, ErrNoCommits)", err)
	}
}

func TestStageAndCommit_CommitsOnlyListedFiles(t *testing.T) {
	dir, repo := newRepo(t)
	writeFile(t, dir, "a.txt", "initial")
	commitAll(t, dir, "init")

	// Dirty two files; only a.txt is passed to StageAndCommit.
	writeFile(t, dir, "a.txt", "changed")
	writeFile(t, dir, "b.txt", "untracked")

	sha, committed, err := repo.StageAndCommit("commit a only", []string{"a.txt"})
	if err != nil {
		t.Fatalf("StageAndCommit() error = %v; want nil", err)
	}
	if !committed {
		t.Fatal("StageAndCommit() committed = false; want true")
	}
	if sha == "" {
		t.Fatal("StageAndCommit() sha = \"\"; want non-empty")
	}

	got, err := repo.CurrentSHA()
	if err != nil {
		t.Fatalf("CurrentSHA() error = %v", err)
	}
	if got != sha {
		t.Errorf("CurrentSHA() = %q; want %q (StageAndCommit's returned sha)", got, sha)
	}
}

func TestStageAndCommit_LeavesUnlistedDirtyFileUncommitted(t *testing.T) {
	dir, repo := newRepo(t)
	writeFile(t, dir, "a.txt", "initial")
	writeFile(t, dir, "b.txt", "initial")
	commitAll(t, dir, "init")

	writeFile(t, dir, "a.txt", "changed")
	writeFile(t, dir, "b.txt", "also changed")

	if _, _, err := repo.StageAndCommit("commit a only", []string{"a.txt"}); err != nil {
		t.Fatalf("StageAndCommit() error = %v", err)
	}

	// b.txt must still show as a modified, uncommitted file.
	stdout, stderr, code, err := runGitStatus(t, dir)
	if err != nil {
		t.Fatalf("git status error = %v", err)
	}
	if code != 0 {
		t.Fatalf("git status exited %d: %s", code, stderr)
	}
	if !strings.Contains(stdout, "b.txt") {
		t.Errorf("git status --porcelain = %q; want it to still list b.txt as dirty", stdout)
	}
}

func TestStageAndCommit_NothingToCommit_WhenFilesUnchanged(t *testing.T) {
	dir, repo := newRepo(t)
	writeFile(t, dir, "a.txt", "initial")
	commitAll(t, dir, "init")

	// a.txt is unchanged since the last commit; nothing to stage.
	sha, committed, err := repo.StageAndCommit("no-op", []string{"a.txt"})
	if err != nil {
		t.Fatalf("StageAndCommit() error = %v; want nil", err)
	}
	if committed {
		t.Fatal("StageAndCommit() committed = true; want false (nothing-to-commit signal)")
	}
	if sha != "" {
		t.Errorf("StageAndCommit() sha = %q; want \"\"", sha)
	}
}

func TestStageAndCommit_NeverStagesUnlistedFile(t *testing.T) {
	dir, repo := newRepo(t)
	writeFile(t, dir, "a.txt", "initial")
	commitAll(t, dir, "init")

	writeFile(t, dir, "a.txt", "changed")
	writeFile(t, dir, "b.txt", "new and untracked")

	if _, _, err := repo.StageAndCommit("commit a only", []string{"a.txt"}); err != nil {
		t.Fatalf("StageAndCommit() error = %v", err)
	}

	changed, err := repo.ChangedFilesSince(firstCommitSHA(t, dir))
	if err != nil {
		t.Fatalf("ChangedFilesSince() error = %v", err)
	}
	for _, f := range changed {
		if f == "b.txt" {
			t.Fatalf("ChangedFilesSince() = %v; b.txt must never have been committed", changed)
		}
	}
}

// TestStageAndCommit_PreStagedUnlistedEntry_NotCommitted asserts that an
// index entry staged outside the call (a human's half-staged WIP in the
// shared worktree) is not swept into the automated commit: only the listed
// file is committed, and the pre-staged entry stays staged and uncommitted.
func TestStageAndCommit_PreStagedUnlistedEntry_NotCommitted(t *testing.T) {
	dir, repo := newRepo(t)
	writeFile(t, dir, "a.txt", "initial")
	commitAll(t, dir, "init")

	// Someone else stages wip.txt but never commits it; the caller then
	// commits an unrelated change to a.txt.
	writeFile(t, dir, "wip.txt", "half-staged WIP")
	lyxtest.MustRun(t, dir, "git", "add", "wip.txt")
	writeFile(t, dir, "a.txt", "changed")

	sha, committed, err := repo.StageAndCommit("commit a only", []string{"a.txt"})
	if err != nil {
		t.Fatalf("StageAndCommit() error = %v; want nil", err)
	}
	if !committed || sha == "" {
		t.Fatalf("StageAndCommit() = (%q, %v); want a real commit of a.txt", sha, committed)
	}

	// The new commit must contain a.txt only — never the pre-staged wip.txt.
	shown, stderr, code, err := runGit(t, dir, "show", "--name-only", "--format=", "HEAD")
	if err != nil {
		t.Fatalf("git show error = %v", err)
	}
	if code != 0 {
		t.Fatalf("git show exited %d: %s", code, stderr)
	}
	if strings.Contains(shown, "wip.txt") {
		t.Errorf("git show --name-only HEAD = %q; pre-staged wip.txt must not be committed", shown)
	}

	// wip.txt must still be staged (status "A ") awaiting its own commit.
	stdout, stderr, code, err := runGitStatus(t, dir)
	if err != nil {
		t.Fatalf("git status error = %v", err)
	}
	if code != 0 {
		t.Fatalf("git status exited %d: %s", code, stderr)
	}
	if !strings.Contains(stdout, "wip.txt") {
		t.Errorf("git status --porcelain = %q; want wip.txt still staged and uncommitted", stdout)
	}
}

// TestStageAndCommit_EmptyFiles_WithPreStagedEntry_NoCommit asserts the
// documented empty-list contract holds even when the index already has a
// staged entry: ("", false, nil) with HEAD unmoved — an empty list must not
// become a commit of someone else's staged change.
func TestStageAndCommit_EmptyFiles_WithPreStagedEntry_NoCommit(t *testing.T) {
	dir, repo := newRepo(t)
	writeFile(t, dir, "a.txt", "initial")
	commitAll(t, dir, "init")

	headBefore, err := repo.CurrentSHA()
	if err != nil {
		t.Fatalf("CurrentSHA() error = %v", err)
	}

	writeFile(t, dir, "wip.txt", "half-staged WIP")
	lyxtest.MustRun(t, dir, "git", "add", "wip.txt")

	for _, files := range [][]string{nil, {}} {
		sha, committed, err := repo.StageAndCommit("must not happen", files)
		if err != nil {
			t.Fatalf("StageAndCommit(%v) error = %v; want nil", files, err)
		}
		if committed || sha != "" {
			t.Errorf("StageAndCommit(%v) = (%q, %v); want (\"\", false)", files, sha, committed)
		}
	}

	headAfter, err := repo.CurrentSHA()
	if err != nil {
		t.Fatalf("CurrentSHA() error = %v", err)
	}
	if headAfter != headBefore {
		t.Errorf("HEAD after empty-list StageAndCommit = %q; want unchanged %q", headAfter, headBefore)
	}
}

func TestChangedFilesSince_ReturnsCorrectSet(t *testing.T) {
	dir, repo := newRepo(t)
	writeFile(t, dir, "a.txt", "initial")
	commitAll(t, dir, "init")

	base, err := repo.CurrentSHA()
	if err != nil {
		t.Fatalf("CurrentSHA() error = %v", err)
	}

	writeFile(t, dir, "a.txt", "changed")
	writeFile(t, dir, "b.txt", "new")
	commitAll(t, dir, "second commit")

	got, err := repo.ChangedFilesSince(base)
	if err != nil {
		t.Fatalf("ChangedFilesSince() error = %v; want nil", err)
	}

	want := map[string]bool{"a.txt": true, "b.txt": true}
	if len(got) != len(want) {
		t.Fatalf("ChangedFilesSince() = %v; want exactly %v", got, want)
	}
	for _, f := range got {
		if !want[f] {
			t.Errorf("ChangedFilesSince() contains unexpected file %q", f)
		}
	}
}

func TestChangedFilesSince_EmptyWhenShaEqualsHEAD(t *testing.T) {
	dir, repo := newRepo(t)
	writeFile(t, dir, "a.txt", "initial")
	commitAll(t, dir, "init")

	head, err := repo.CurrentSHA()
	if err != nil {
		t.Fatalf("CurrentSHA() error = %v", err)
	}

	got, err := repo.ChangedFilesSince(head)
	if err != nil {
		t.Fatalf("ChangedFilesSince() error = %v; want nil", err)
	}
	if len(got) != 0 {
		t.Errorf("ChangedFilesSince(HEAD) = %v; want empty", got)
	}
}

func TestChangedFilesSince_ExcludesUncommittedEdit(t *testing.T) {
	dir, repo := newRepo(t)
	writeFile(t, dir, "a.txt", "initial")
	commitAll(t, dir, "init")

	base, err := repo.CurrentSHA()
	if err != nil {
		t.Fatalf("CurrentSHA() error = %v", err)
	}

	// Dirty the working tree without committing.
	writeFile(t, dir, "a.txt", "uncommitted edit")

	got, err := repo.ChangedFilesSince(base)
	if err != nil {
		t.Fatalf("ChangedFilesSince() error = %v; want nil", err)
	}
	if len(got) != 0 {
		t.Errorf("ChangedFilesSince() = %v; want empty (uncommitted edits are excluded)", got)
	}
}

// TestChangedFilesSince_NonASCIIPathReturnedVerbatim asserts that a filename
// outside ASCII comes back as the literal on-disk path, not core.quotePath's
// C-quoted escape form ("\"bl\\303\\245b\\303\\246r.txt\"") that matches
// nothing on disk.
func TestChangedFilesSince_NonASCIIPathReturnedVerbatim(t *testing.T) {
	dir, repo := newRepo(t)
	writeFile(t, dir, "a.txt", "initial")
	commitAll(t, dir, "init")

	base, err := repo.CurrentSHA()
	if err != nil {
		t.Fatalf("CurrentSHA() error = %v", err)
	}

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
}

// TestChangedFilesSince_RenameReportsBothPaths asserts that a rename lists
// both the old path (which no longer exists at HEAD) and the new one; git's
// default rename detection would report only the destination, leaving a
// consumer's per-file state for the old path stale forever.
func TestChangedFilesSince_RenameReportsBothPaths(t *testing.T) {
	dir, repo := newRepo(t)
	writeFile(t, dir, "old.txt", "content that stays identical")
	commitAll(t, dir, "init")

	base, err := repo.CurrentSHA()
	if err != nil {
		t.Fatalf("CurrentSHA() error = %v", err)
	}

	// A pure rename (identical content) is the case rename detection folds.
	lyxtest.MustRun(t, dir, "git", "mv", "old.txt", "new.txt")
	lyxtest.MustRun(t, dir, "git", "commit", "-m", "rename")

	got, err := repo.ChangedFilesSince(base)
	if err != nil {
		t.Fatalf("ChangedFilesSince() error = %v; want nil", err)
	}
	want := map[string]bool{"old.txt": true, "new.txt": true}
	if len(got) != len(want) {
		t.Fatalf("ChangedFilesSince() = %v; want exactly both sides of the rename %v", got, want)
	}
	for _, f := range got {
		if !want[f] {
			t.Errorf("ChangedFilesSince() contains unexpected file %q", f)
		}
	}
}

func TestChangedFilesSince_ErrorsOnFabricatedSHA(t *testing.T) {
	dir, repo := newRepo(t)
	writeFile(t, dir, "a.txt", "initial")
	commitAll(t, dir, "init")

	_, err := repo.ChangedFilesSince("0123456789abcdef0123456789abcdef01234567")
	if err == nil {
		t.Fatal("ChangedFilesSince(fabricated sha) error = nil; want an error")
	}
}

func TestSHAExists(t *testing.T) {
	dir, repo := newRepo(t)
	writeFile(t, dir, "a.txt", "initial")
	commitAll(t, dir, "init")

	real, err := repo.CurrentSHA()
	if err != nil {
		t.Fatalf("CurrentSHA() error = %v", err)
	}

	tests := []struct {
		name string
		sha  string
		want bool
	}{
		{"RealSHA", real, true},
		{"FabricatedSHA", "0123456789abcdef0123456789abcdef01234567", false},
		{"GarbageInput", "not-a-sha at all!!", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := repo.SHAExists(tt.sha); got != tt.want {
				t.Errorf("SHAExists(%q) = %v; want %v", tt.sha, got, tt.want)
			}
		})
	}
}

// firstCommitSHA returns the SHA of the repository's first (root) commit,
// used as a fixed base point for ChangedFilesSince assertions.
func firstCommitSHA(t *testing.T, dir string) string {
	t.Helper()

	stdout, stderr, code, err := runGit(t, dir, "rev-list", "--max-parents=0", "HEAD")
	if err != nil {
		t.Fatalf("git rev-list error = %v", err)
	}
	if code != 0 {
		t.Fatalf("git rev-list exited %d: %s", code, stderr)
	}
	return strings.TrimSpace(stdout)
}
