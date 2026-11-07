// commitweftat_test.go covers commitWeftAt, the package-level commit that is not tied to a warp branch.
// Its fixture is newPlainWarpRepo from export_integration_test.go, a plain git repository.
// The tests rely on the package TestMain in testmain_test.go, which runs gitkit.HermeticGitEnv().

package fabricengine

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// gitStatusPorcelain returns the raw output of `git status --porcelain` for dir.
// A non-empty result means the worktree has uncommitted changes: staged, unstaged, or untracked.
func gitStatusPorcelain(t *testing.T, dir string) string {
	t.Helper()

	cmd := exec.Command("git", "status", "--porcelain")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git status --porcelain in %s: %v", dir, err)
	}
	return string(out)
}

// TestCommitWeftAt_CommitsDirtyWorktree asserts that commitWeftAt commits an untracked file.
// The message is passed through verbatim, and the commit carries no Warp-SHA trailer.
// There is no warp branch for the commit to trail against.
//
//testtiming:keep commitWeftAt committing an untracked file with the message passed through verbatim and no Warp-SHA trailer; coverage of its blocks by other tests does not show an assertion of this
func TestCommitWeftAt_CommitsDirtyWorktree(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	dir := newPlainWarpRepo(t)
	if err := os.WriteFile(filepath.Join(dir, "board-note.md"), []byte("a board note"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	sha, committed, err := commitWeftAt(dir, "board sync", SyncOptions{})
	if err != nil {
		t.Fatalf("commitWeftAt() error = %v; want nil", err)
	}
	if !committed {
		t.Fatalf("commitWeftAt() committed = false; want true")
	}
	if sha == "" {
		t.Fatalf("commitWeftAt() sha = %q; want a non-empty new HEAD SHA", sha)
	}

	rawMessage := commitMessageAt(t, dir, sha)
	if got := strings.TrimSpace(rawMessage); got != "board sync" {
		t.Errorf("commit message = %q; want exactly %q", got, "board sync")
	}
	if strings.Contains(rawMessage, WarpSHATrailerKey+":") {
		t.Errorf("commit message = %q; want no %s trailer (commitWeftAt is warp-untethered)", rawMessage, WarpSHATrailerKey)
	}
}

// TestCommitWeftAt_NoopOnCleanWorktree asserts that commitWeftAt is a no-op when nothing is new to stage.
// A second call with no changes in between reports committed=false, a nil error, and an empty sha.
//
//testtiming:keep commitWeftAt reporting committed=false with a nil error twice on a clean worktree; coverage of its blocks by other tests does not show an assertion of this
func TestCommitWeftAt_NoopOnCleanWorktree(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	dir := newPlainWarpRepo(t)

	if _, _, err := commitWeftAt(dir, "board sync 1", SyncOptions{}); err != nil {
		t.Fatalf("commitWeftAt() first call error = %v; want nil", err)
	}

	sha, committed, err := commitWeftAt(dir, "board sync 2", SyncOptions{})
	if err != nil {
		t.Fatalf("commitWeftAt() second call error = %v; want nil", err)
	}
	if committed {
		t.Fatalf("commitWeftAt() second call committed = true; want false (nothing changed since the first call)")
	}
	if sha != "" {
		t.Errorf("commitWeftAt() second call sha = %q; want empty", sha)
	}
}

// TestCommitWeftAt_SeedFailureFailsBeforeStaging asserts that commitWeftAt fails when seeding the excludes fails:
// a non-nil error, committed=false, an empty sha, and the dirty file left untracked.
// The seed read is made to fail by replacing the repo's info/exclude file with a directory.
//
//testtiming:keep commitWeftAt failing before staging when the exclude seed fails; coverage of its blocks by other tests does not show an assertion of this
func TestCommitWeftAt_SeedFailureFailsBeforeStaging(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	dir := newPlainWarpRepo(t)
	excludePath := filepath.Join(dir, ".git", "info", "exclude")
	if err := os.RemoveAll(excludePath); err != nil {
		t.Fatalf("RemoveAll(%s): %v", excludePath, err)
	}
	if err := os.MkdirAll(excludePath, 0o755); err != nil {
		t.Fatalf("MkdirAll(%s): %v", excludePath, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "board-note.md"), []byte("a board note"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	sha, committed, err := commitWeftAt(dir, "board sync", SyncOptions{})
	if err == nil || !strings.Contains(err.Error(), "seed weft artifact excludes") {
		t.Fatalf("commitWeftAt() error = %v; want a seed weft artifact excludes failure", err)
	}
	if committed {
		t.Errorf("commitWeftAt() committed = true; want false")
	}
	if sha != "" {
		t.Errorf("commitWeftAt() sha = %q; want empty", sha)
	}
	// git itself rejects a directory as the exclude file, so restore a plain file before reading the status.
	if err := os.RemoveAll(excludePath); err != nil {
		t.Fatalf("RemoveAll(%s): %v", excludePath, err)
	}
	if err := os.WriteFile(excludePath, nil, 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", excludePath, err)
	}
	if status := gitStatusPorcelain(t, dir); status != "?? board-note.md\n" {
		t.Errorf("git status --porcelain = %q; want only board-note.md, still untracked", status)
	}
}

// TestCommitWeftAt_SkipGitReturnsImmediately asserts that opts.SkipGit short circuits before any git is spawned:
// the return is exactly ("", false, nil), and a dirty untracked file is left untouched.
//
//testtiming:keep opts.SkipGit short-circuiting commitWeftAt to ("", false, nil) before git spawns, leaving a dirty file untouched; coverage of its blocks by other tests does not show an assertion of this
func TestCommitWeftAt_SkipGitReturnsImmediately(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	dir := newPlainWarpRepo(t)
	if err := os.WriteFile(filepath.Join(dir, "board-note.md"), []byte("a board note"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	sha, committed, err := commitWeftAt(dir, "msg", SyncOptions{SkipGit: true})
	if err != nil {
		t.Fatalf("commitWeftAt(SkipGit) error = %v; want nil", err)
	}
	if committed {
		t.Fatalf("commitWeftAt(SkipGit) committed = true; want false")
	}
	if sha != "" {
		t.Errorf("commitWeftAt(SkipGit) sha = %q; want empty", sha)
	}

	if status := gitStatusPorcelain(t, dir); status == "" {
		t.Errorf("git status --porcelain = %q; want non-empty (the untracked file must remain uncommitted)", status)
	}
}
