//go:build integration

// sync_test.go — unit tests for the background pusher (sync.go).
//
// Exercises Sync against a LOCAL bare repo (no network, no dummy remote): a
// commit + push, a burst coalescing into one commit, BOARD_SKIP_PUSH committing
// without pushing, and the clean-tree no-op.

package boardtest

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/board"
)

// newSyncRepo creates a bare "remote" and a working clone with an upstream, seeds
// an initial commit, and returns the working-copy path plus helpers to count
// commits on the remote and locally.
func newSyncRepo(t *testing.T) (work string, remoteCommits, localCommits func() int) {
	t.Helper()
	t.Setenv("BOARD_SKIP_GIT", "") // Sync must not be disabled for these tests

	dir := t.TempDir()
	bare := filepath.Join(dir, "remote.git")
	work = filepath.Join(dir, "work")

	run := func(args ...string) {
		t.Helper()
		if out, err := exec.Command(args[0], args[1:]...).CombinedOutput(); err != nil {
			t.Fatalf("%v failed: %s", args, out)
		}
	}
	run("git", "init", "--bare", bare)
	run("git", "clone", bare, work)
	run("git", "-C", work, "config", "user.email", "t@loomyard.dev")
	run("git", "-C", work, "config", "user.name", "t")

	if err := os.WriteFile(filepath.Join(work, "tasks.json"), []byte("[]\n"), 0o644); err != nil {
		t.Fatalf("seed write: %v", err)
	}
	run("git", "-C", work, "add", "-A")
	run("git", "-C", work, "commit", "-m", "init")
	run("git", "-C", work, "push", "-u", "origin", "HEAD")

	// Count from the work clone: HEAD is local commits, @{u} (the upstream
	// remote-tracking ref, advanced on push) is what landed on the remote. This
	// avoids the bare repo's HEAD symref pointing at a different default branch.
	count := func(rev string) int {
		out, err := exec.Command("git", "-C", work, "rev-list", "--count", rev).Output()
		if err != nil {
			t.Fatalf("rev-list %s: %v", rev, err)
		}
		n, _ := strconv.Atoi(strings.TrimSpace(string(out)))
		return n
	}
	return work, func() int { return count("@{u}") }, func() int { return count("HEAD") }
}

// dirty overwrites tasks.json so the working tree has a change to commit.
func dirty(t *testing.T, work, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(work, "tasks.json"), []byte(content), 0o644); err != nil {
		t.Fatalf("dirty write: %v", err)
	}
}

func TestSyncCommitsAndPushes(t *testing.T) {
	work, remoteCommits, _ := newSyncRepo(t)
	before := remoteCommits()

	dirty(t, work, `[{"id":0,"slug":"a","title":"A"}]`)
	cfg := board.DefaultConfig()
	cfg.Path = work
	if err := board.New(cfg).Sync(); err != nil {
		t.Fatalf("Sync: %v", err)
	}

	if got := remoteCommits(); got != before+1 {
		t.Fatalf("expected remote to gain 1 commit, got %d -> %d", before, got)
	}
	out, _ := exec.Command("git", "-C", work, "status", "--porcelain").Output()
	if strings.TrimSpace(string(out)) != "" {
		t.Fatalf("working tree not clean after sync: %q", out)
	}
}

func TestSyncCoalescesBurstIntoOneCommit(t *testing.T) {
	work, remoteCommits, _ := newSyncRepo(t)
	before := remoteCommits()

	// Several changes land before a single Sync — they collapse into one commit.
	for i := 0; i < 5; i++ {
		dirty(t, work, `[{"id":0,"slug":"a","title":"v`+strconv.Itoa(i)+`"}]`)
	}
	cfg := board.DefaultConfig()
	cfg.Path = work
	if err := board.New(cfg).Sync(); err != nil {
		t.Fatalf("Sync: %v", err)
	}

	if got := remoteCommits(); got != before+1 {
		t.Fatalf("expected 1 coalesced commit, remote went %d -> %d", before, got)
	}
}

func TestSyncSkipPushCommitsLocallyOnly(t *testing.T) {
	t.Setenv("BOARD_SKIP_PUSH", "1")
	work, remoteCommits, localCommits := newSyncRepo(t)
	remoteBefore, localBefore := remoteCommits(), localCommits()

	dirty(t, work, `[{"id":0,"slug":"a","title":"A"}]`)
	cfg := board.DefaultConfig()
	cfg.Path = work
	if err := board.New(cfg).Sync(); err != nil {
		t.Fatalf("Sync: %v", err)
	}

	if got := localCommits(); got != localBefore+1 {
		t.Fatalf("expected a local commit, local went %d -> %d", localBefore, got)
	}
	if got := remoteCommits(); got != remoteBefore {
		t.Fatalf("expected no push, remote went %d -> %d", remoteBefore, got)
	}
}

func TestSyncCleanTreeIsNoOp(t *testing.T) {
	work, remoteCommits, _ := newSyncRepo(t)
	cfg := board.DefaultConfig()
	cfg.Path = work
	w := board.New(cfg)

	// The first sync commits the .gitignore; after that a clean tree is a no-op.
	if err := w.Sync(); err != nil {
		t.Fatalf("initial Sync: %v", err)
	}
	before := remoteCommits()

	if err := w.Sync(); err != nil {
		t.Fatalf("Sync on clean tree: %v", err)
	}
	if got := remoteCommits(); got != before {
		t.Fatalf("clean-tree sync changed remote: %d -> %d", before, got)
	}
}

func TestSyncIgnoresLockfiles(t *testing.T) {
	work, _, _ := newSyncRepo(t)

	dirty(t, work, `[{"id":0,"slug":"a","title":"A"}]`)
	cfg := board.DefaultConfig()
	cfg.Path = work
	if err := board.New(cfg).Sync(); err != nil {
		t.Fatalf("Sync: %v", err)
	}

	out, err := exec.Command("git", "-C", work, "ls-files").Output()
	if err != nil {
		t.Fatalf("ls-files: %v", err)
	}
	tracked := string(out)
	if !strings.Contains(tracked, ".gitignore") {
		t.Fatalf(".gitignore was not committed; tracked:\n%s", tracked)
	}
	if strings.Contains(tracked, ".lock") || strings.Contains(tracked, ".swaplock") {
		t.Fatalf("lock files were committed; tracked:\n%s", tracked)
	}
}
