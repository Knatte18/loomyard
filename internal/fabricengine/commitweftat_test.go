//go:build integration

// commitweftat_test.go — integration coverage for CommitWeftAt, the
// package-level, warp-untethered commit primitive: a real commit on a dirty
// worktree, a no-op on a clean one, and the SkipGit early return. Package
// fabricengine (internal), reusing index_integration_test.go's
// newPlainWarpRepo(t) as a generic plain-git-repo fixture — it is not
// warp-specific despite its name, just `git init -b main` plus one commit,
// which is exactly the weft:main-shaped fixture these tests need. Relies on
// testmain_test.go's package-wide TestMain for HermeticGitEnv(); no new
// TestMain is added here.

package fabricengine

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// gitStatusPorcelain returns `git status --porcelain`'s raw output for dir —
// non-empty means the worktree has uncommitted changes (staged, unstaged, or
// untracked).
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

// TestCommitWeftAt_CommitsDirtyWorktree asserts that CommitWeftAt commits an
// untracked file via a real wildcard-stage commit, with the exact message
// passed through verbatim — no Warp-SHA trailer, unlike Fabric.CommitWeft,
// since CommitWeftAt's caller (_board's weft:main checkout) has no
// corresponding warp branch to trailer against.
func TestCommitWeftAt_CommitsDirtyWorktree(t *testing.T) {
	t.Parallel()

	dir := newPlainWarpRepo(t)
	if err := os.WriteFile(filepath.Join(dir, "board-note.md"), []byte("a board note"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	sha, committed, err := CommitWeftAt(dir, "board sync", SyncOptions{})
	if err != nil {
		t.Fatalf("CommitWeftAt() error = %v; want nil", err)
	}
	if !committed {
		t.Fatalf("CommitWeftAt() committed = false; want true")
	}
	if sha == "" {
		t.Fatalf("CommitWeftAt() sha = %q; want a non-empty new HEAD SHA", sha)
	}

	rawMessage := commitMessageAt(t, dir, sha)
	if got := strings.TrimSpace(rawMessage); got != "board sync" {
		t.Errorf("commit message = %q; want exactly %q", got, "board sync")
	}
	if strings.Contains(rawMessage, WarpSHATrailerKey+":") {
		t.Errorf("commit message = %q; want no %s trailer (CommitWeftAt is warp-untethered)", rawMessage, WarpSHATrailerKey)
	}
}

// TestCommitWeftAt_NoopOnCleanWorktree asserts that CommitWeftAt is a true
// no-op — committed=false, err=nil — when the worktree has nothing new to
// stage, called twice in a row with no changes in between.
func TestCommitWeftAt_NoopOnCleanWorktree(t *testing.T) {
	t.Parallel()

	dir := newPlainWarpRepo(t)

	if _, _, err := CommitWeftAt(dir, "board sync 1", SyncOptions{}); err != nil {
		t.Fatalf("CommitWeftAt() first call error = %v; want nil", err)
	}

	sha, committed, err := CommitWeftAt(dir, "board sync 2", SyncOptions{})
	if err != nil {
		t.Fatalf("CommitWeftAt() second call error = %v; want nil", err)
	}
	if committed {
		t.Fatalf("CommitWeftAt() second call committed = true; want false (nothing changed since the first call)")
	}
	if sha != "" {
		t.Errorf("CommitWeftAt() second call sha = %q; want empty", sha)
	}
}

// TestCommitWeftAt_SkipGitReturnsImmediately asserts that opts.SkipGit short
// circuits before any git is spawned: the return is exactly ("", false,
// nil), and a dirty untracked file is left completely untouched.
func TestCommitWeftAt_SkipGitReturnsImmediately(t *testing.T) {
	t.Parallel()

	dir := newPlainWarpRepo(t)
	if err := os.WriteFile(filepath.Join(dir, "board-note.md"), []byte("a board note"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	sha, committed, err := CommitWeftAt(dir, "msg", SyncOptions{SkipGit: true})
	if err != nil {
		t.Fatalf("CommitWeftAt(SkipGit) error = %v; want nil", err)
	}
	if committed {
		t.Fatalf("CommitWeftAt(SkipGit) committed = true; want false")
	}
	if sha != "" {
		t.Errorf("CommitWeftAt(SkipGit) sha = %q; want empty", sha)
	}

	if status := gitStatusPorcelain(t, dir); status == "" {
		t.Errorf("git status --porcelain = %q; want non-empty (the untracked file must remain uncommitted)", status)
	}
}
