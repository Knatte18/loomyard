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

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/lyxtest"
)

// newSyncRepo returns an isolated working-tree and helpers that count commits on
// the remote (@{u}) and locally (HEAD). It uses lyxtest.CopyWeft so that fixture
// construction runs zero per-test git spawns; the fixture already has upstream
// tracking established via the template-once build.
func newSyncRepo(t *testing.T) (work string, remoteCommits, localCommits func() int) {
	t.Helper()

	fixture := lyxtest.CopyWeft(t)
	work = fixture.WeftPath

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
	t.Parallel()
	work, remoteCommits, _ := newSyncRepo(t)
	before := remoteCommits()

	dirty(t, work, `[{"id":0,"slug":"a","title":"A"}]`)
	cfg := boardengine.Config{Path: work, Home: "Home.md", Sidebar: "_Sidebar.md", ProposalPrefix: "proposal-"}
	if err := boardengine.New(cfg).Sync(); err != nil {
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
	t.Parallel()
	work, remoteCommits, _ := newSyncRepo(t)
	before := remoteCommits()

	// Several changes land before a single Sync — they collapse into one commit.
	for i := 0; i < 5; i++ {
		dirty(t, work, `[{"id":0,"slug":"a","title":"v`+strconv.Itoa(i)+`"}]`)
	}
	cfg := boardengine.Config{Path: work, Home: "Home.md", Sidebar: "_Sidebar.md", ProposalPrefix: "proposal-"}
	if err := boardengine.New(cfg).Sync(); err != nil {
		t.Fatalf("Sync: %v", err)
	}

	if got := remoteCommits(); got != before+1 {
		t.Fatalf("expected 1 coalesced commit, remote went %d -> %d", before, got)
	}
}

func TestSyncSkipPushCommitsLocallyOnly(t *testing.T) {
	t.Parallel()
	work, remoteCommits, localCommits := newSyncRepo(t)
	remoteBefore, localBefore := remoteCommits(), localCommits()

	dirty(t, work, `[{"id":0,"slug":"a","title":"A"}]`)
	cfg := boardengine.Config{Path: work, Home: "Home.md", Sidebar: "_Sidebar.md", ProposalPrefix: "proposal-"}
	cfg.SkipPush = true
	if err := boardengine.New(cfg).Sync(); err != nil {
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
	t.Parallel()
	work, remoteCommits, _ := newSyncRepo(t)
	cfg := boardengine.Config{Path: work, Home: "Home.md", Sidebar: "_Sidebar.md", ProposalPrefix: "proposal-"}
	w := boardengine.New(cfg)

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

// TestSyncNothingPendingSkipsPushEntirely locks in the conditional-push
// contract the pre-gitrepo pushUnpushed provided: a Sync that finds a clean
// tree and nothing ahead of upstream must not contact the remote at all. The
// unreachable-remote setup makes any push attempt fail loudly, so an
// unconditional per-iteration push (the regression this guards against) turns
// into a test failure instead of a silent wasted round-trip.
func TestSyncNothingPendingSkipsPushEntirely(t *testing.T) {
	t.Parallel()
	work, _, _ := newSyncRepo(t)
	cfg := boardengine.Config{Path: work, Home: "Home.md", Sidebar: "_Sidebar.md", ProposalPrefix: "proposal-"}
	w := boardengine.New(cfg)

	// First sync commits and pushes the .gitignore so the board is fully synced.
	if err := w.Sync(); err != nil {
		t.Fatalf("initial Sync: %v", err)
	}

	// Point origin at a path that does not exist: from here on, any git push
	// fails. A fully-synced Sync must still succeed because it has nothing to
	// push and therefore never runs one.
	bogus := filepath.Join(t.TempDir(), "gone.git")
	if out, err := exec.Command("git", "-C", work, "remote", "set-url", "origin", bogus).CombinedOutput(); err != nil {
		t.Fatalf("remote set-url: %v: %s", err, out)
	}
	if err := w.Sync(); err != nil {
		t.Fatalf("Sync with nothing pending must not touch the unreachable remote: %v", err)
	}

	// Control: once there IS something to push, the unreachable remote must
	// surface as a genuine error — the no-op above is a guard, not a swallow.
	dirty(t, work, `[{"id":0,"slug":"a","title":"offline"}]`)
	if err := w.Sync(); err == nil {
		t.Fatal("Sync with a pending commit and unreachable remote should fail")
	}
}

func TestSyncIgnoresLockfiles(t *testing.T) {
	t.Parallel()
	work, _, _ := newSyncRepo(t)

	dirty(t, work, `[{"id":0,"slug":"a","title":"A"}]`)
	cfg := boardengine.Config{Path: work, Home: "Home.md", Sidebar: "_Sidebar.md", ProposalPrefix: "proposal-"}
	if err := boardengine.New(cfg).Sync(); err != nil {
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

func TestSkipSeam(t *testing.T) {
	t.Parallel()

	t.Run("SkipPush=true commits locally but leaves unpushed", func(t *testing.T) {
		t.Parallel()
		work, remoteCommits, localCommits := newSyncRepo(t)
		remoteBefore, localBefore := remoteCommits(), localCommits()

		dirty(t, work, `[{"id":0,"slug":"a","title":"A"}]`)
		cfg := boardengine.Config{Path: work, Home: "Home.md", Sidebar: "_Sidebar.md", ProposalPrefix: "proposal-"}
		cfg.SkipPush = true
		if err := boardengine.New(cfg).Sync(); err != nil {
			t.Fatalf("Sync: %v", err)
		}

		// A commit should be made locally.
		if got := localCommits(); got != localBefore+1 {
			t.Fatalf("expected local commit with SkipPush=true, local went %d -> %d", localBefore, got)
		}
		// But nothing should be pushed.
		if got := remoteCommits(); got != remoteBefore {
			t.Fatalf("expected no push with SkipPush=true, remote went %d -> %d", remoteBefore, got)
		}

		// Verify @{u} is behind HEAD.
		cmd := exec.Command("git", "-C", work, "rev-list", "--count", "@{u}..HEAD")
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("rev-list @{u}..HEAD: %v", err)
		}
		unpushed, _ := strconv.Atoi(strings.TrimSpace(string(out)))
		if unpushed == 0 {
			t.Fatalf("expected unpushed commits with SkipPush=true, got 0")
		}
	})

	t.Run("SkipGit=true is a no-op", func(t *testing.T) {
		t.Parallel()
		work, remoteCommits, localCommits := newSyncRepo(t)
		remoteBefore, localBefore := remoteCommits(), localCommits()

		dirty(t, work, `[{"id":0,"slug":"a","title":"A"}]`)
		cfg := boardengine.Config{Path: work, Home: "Home.md", Sidebar: "_Sidebar.md", ProposalPrefix: "proposal-"}
		cfg.SkipGit = true
		if err := boardengine.New(cfg).Sync(); err != nil {
			t.Fatalf("Sync: %v", err)
		}

		// No commit should be made.
		if got := localCommits(); got != localBefore {
			t.Fatalf("expected no commit with SkipGit=true, local went %d -> %d", localBefore, got)
		}
		// No push should occur.
		if got := remoteCommits(); got != remoteBefore {
			t.Fatalf("expected no push with SkipGit=true, remote went %d -> %d", remoteBefore, got)
		}
	})
}
