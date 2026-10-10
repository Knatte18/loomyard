//go:build integration

// sync_test.go — unit tests for the background pusher (sync.go).
//
// Exercises Sync against a LOCAL bare repo (no network, no dummy remote): a commit + push, a burst coalescing into one commit, BOARD_SKIP_PUSH committing without pushing, BOARD_SKIP_GIT doing nothing, lock files staying untracked, and the clean-tree and nothing-pending no-ops.

package boardtest

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
)

// newSyncRepo returns an isolated working-tree and helpers that count commits on
// the remote (@{u}) and locally (HEAD). It builds a real hub via hubforge.NewHub, whose records
// worktree is cloned from h.RecordsBare by CloneHub -- but unlike the old records-only fixture's
// pre-established upstream tracking, that tracking is NOT set up here: hubforge's own records-bare
// template must stay genuinely empty and unpushed (pushing to it would trip CloneHub's records-sibling bootstrap guard), so a
// fresh hub's records primary carries a local-only "initialise records primary branch" commit with no
// @{u} yet. This establishes tracking by pushing that commit against this hub's own per-test copy
// of the bare (never the cached template) -- exactly what boardengine's own first real Sync would
// otherwise do via push.autoSetupRemote, done once up front so "before" reflects a board that has
// already been synced once, matching the old fixture's pre-established state.
func newSyncRepo(t *testing.T) (work string, remoteCommits, localCommits func() int) {
	t.Helper()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	work = h.PrimeRecords()

	if out, err := exec.Command("git", "-C", work, "push", "-u", "origin", "HEAD").CombinedOutput(); err != nil {
		t.Fatalf("establish upstream tracking: %v: %s", err, out)
	}

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

// syncFixture is the one hub work clone the steps of TestSync share, with counters for the remote's and the local commits.
type syncFixture struct {
	work                        string
	remoteCommits, localCommits func() int
}

// syncConfig returns a board config over the fixture's work clone.
func (f *syncFixture) syncConfig() boardengine.Config {
	return boardengine.Config{Path: f.work, Readme: "Home.md", DesignPrefix: "proposal-"}
}

// TestSync runs Sync's contract against one hub, in the order below.
// It calls t.Parallel and no step does: the steps share the one work clone and its remote, so they run serially.
// Each step measures its own before and after commit counts, so only two orderings matter:
// the first step runs against the fresh hub, and the last step points origin at an unreachable path.
// The SkipGit step leaves a dirty tree and the SkipPush step leaves an unpushed commit;
// every later step's Sync commits and pushes them along with its own change.
func TestSync(t *testing.T) {
	t.Parallel()
	work, remoteCommits, localCommits := newSyncRepo(t)
	fixture := &syncFixture{work: work, remoteCommits: remoteCommits, localCommits: localCommits}

	steps := []struct {
		name string
		run  func(t *testing.T, f *syncFixture)
	}{
		{"CommitsAndPushes", stepSyncCommitsAndPushes},
		{"CoalescesBurstIntoOneCommit", stepSyncCoalescesBurstIntoOneCommit},
		{"CleanTreeIsNoOp", stepSyncCleanTreeIsNoOp},
		{"IgnoresLockfiles", stepSyncIgnoresLockfiles},
		{"SkipGitIsNoOp", stepSyncSkipGitIsNoOp},
		{"SkipPushCommitsLocallyOnly", stepSyncSkipPushCommitsLocallyOnly},
		{"NothingPendingSkipsPushEntirely", stepSyncNothingPendingSkipsPushEntirely},
	}
	for _, step := range steps {
		if !t.Run(step.name, func(t *testing.T) { step.run(t, fixture) }) {
			return
		}
	}
}

func stepSyncCommitsAndPushes(t *testing.T, f *syncFixture) {
	before := f.remoteCommits()

	dirty(t, f.work, `[{"id":0,"slug":"a","title":"A"}]`)
	if err := boardengine.New(f.syncConfig()).Sync(); err != nil {
		t.Fatalf("Sync: %v", err)
	}

	if got := f.remoteCommits(); got != before+1 {
		t.Fatalf("expected remote to gain 1 commit, got %d -> %d", before, got)
	}
	out, _ := exec.Command("git", "-C", f.work, "status", "--porcelain").Output()
	if strings.TrimSpace(string(out)) != "" {
		t.Fatalf("working tree not clean after sync: %q", out)
	}
}

func stepSyncCoalescesBurstIntoOneCommit(t *testing.T, f *syncFixture) {
	before := f.remoteCommits()

	// Several changes land before a single Sync — they collapse into one commit.
	for i := 0; i < 5; i++ {
		dirty(t, f.work, `[{"id":0,"slug":"a","title":"v`+strconv.Itoa(i)+`"}]`)
	}
	if err := boardengine.New(f.syncConfig()).Sync(); err != nil {
		t.Fatalf("Sync: %v", err)
	}

	if got := f.remoteCommits(); got != before+1 {
		t.Fatalf("expected 1 coalesced commit, remote went %d -> %d", before, got)
	}
}

func stepSyncCleanTreeIsNoOp(t *testing.T, f *syncFixture) {
	w := boardengine.New(f.syncConfig())

	// Settle the tree first: after that a clean tree is a no-op.
	if err := w.Sync(); err != nil {
		t.Fatalf("initial Sync: %v", err)
	}
	before := f.remoteCommits()

	if err := w.Sync(); err != nil {
		t.Fatalf("Sync on clean tree: %v", err)
	}
	if got := f.remoteCommits(); got != before {
		t.Fatalf("clean-tree sync changed remote: %d -> %d", before, got)
	}
}

func stepSyncIgnoresLockfiles(t *testing.T, f *syncFixture) {
	cfg := f.syncConfig()

	dirty(t, f.work, `[{"id":0,"slug":"a","title":"A"}]`)
	if err := boardengine.New(cfg).Sync(); err != nil {
		t.Fatalf("Sync: %v", err)
	}

	out, err := exec.Command("git", "-C", f.work, "ls-files").Output()
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

	// Repeated syncs must not re-append patterns: seeding is idempotent and,
	// running under the push lock, serialized across concurrent sync processes.
	dirty(t, f.work, `[{"id":0,"slug":"a","title":"B"}]`)
	if err := boardengine.New(cfg).Sync(); err != nil {
		t.Fatalf("second Sync: %v", err)
	}
	gitignore, err := os.ReadFile(filepath.Join(f.work, ".gitignore"))
	if err != nil {
		t.Fatalf("read .gitignore: %v", err)
	}
	for _, pat := range []string{"*.lock", "*.swaplock"} {
		if got := strings.Count(string(gitignore), pat+"\n"); got != 1 {
			t.Errorf(".gitignore contains %q %d times; want exactly 1:\n%s", pat, got, gitignore)
		}
	}
}

func stepSyncSkipGitIsNoOp(t *testing.T, f *syncFixture) {
	remoteBefore, localBefore := f.remoteCommits(), f.localCommits()

	dirty(t, f.work, `[{"id":0,"slug":"a","title":"A"}]`)
	cfg := f.syncConfig()
	cfg.SkipGit = true
	if err := boardengine.New(cfg).Sync(); err != nil {
		t.Fatalf("Sync: %v", err)
	}

	if got := f.localCommits(); got != localBefore {
		t.Fatalf("expected no commit with SkipGit=true, local went %d -> %d", localBefore, got)
	}
	if got := f.remoteCommits(); got != remoteBefore {
		t.Fatalf("expected no push with SkipGit=true, remote went %d -> %d", remoteBefore, got)
	}
}

func stepSyncSkipPushCommitsLocallyOnly(t *testing.T, f *syncFixture) {
	remoteBefore, localBefore := f.remoteCommits(), f.localCommits()

	dirty(t, f.work, `[{"id":0,"slug":"a","title":"skip push"}]`)
	cfg := f.syncConfig()
	cfg.SkipPush = true
	if err := boardengine.New(cfg).Sync(); err != nil {
		t.Fatalf("Sync: %v", err)
	}

	if got := f.localCommits(); got != localBefore+1 {
		t.Fatalf("expected a local commit, local went %d -> %d", localBefore, got)
	}
	if got := f.remoteCommits(); got != remoteBefore {
		t.Fatalf("expected no push, remote went %d -> %d", remoteBefore, got)
	}

	// @{u} is behind HEAD.
	out, err := exec.Command("git", "-C", f.work, "rev-list", "--count", "@{u}..HEAD").Output()
	if err != nil {
		t.Fatalf("rev-list @{u}..HEAD: %v", err)
	}
	if unpushed, _ := strconv.Atoi(strings.TrimSpace(string(out))); unpushed == 0 {
		t.Fatalf("expected unpushed commits with SkipPush=true, got 0")
	}
}

// stepSyncNothingPendingSkipsPushEntirely locks in the conditional-push contract the pre-gitrepo pushUnpushed provided: a Sync that finds a clean tree and nothing ahead of upstream must not contact the remote at all.
// The unreachable-remote setup makes any push attempt fail loudly, so an unconditional per-iteration push (the regression this guards against) turns into a test failure instead of a silent wasted round-trip.
// It runs last: it leaves origin pointing at a path that does not exist.
func stepSyncNothingPendingSkipsPushEntirely(t *testing.T, f *syncFixture) {
	w := boardengine.New(f.syncConfig())

	// First sync commits and pushes whatever is left, so the board is fully synced.
	if err := w.Sync(); err != nil {
		t.Fatalf("initial Sync: %v", err)
	}

	// Point origin at a path that does not exist: from here on, any git push
	// fails. A fully-synced Sync must still succeed because it has nothing to
	// push and therefore never runs one.
	bogus := filepath.Join(t.TempDir(), "gone.git")
	if out, err := exec.Command("git", "-C", f.work, "remote", "set-url", "origin", bogus).CombinedOutput(); err != nil {
		t.Fatalf("remote set-url: %v: %s", err, out)
	}
	if err := w.Sync(); err != nil {
		t.Fatalf("Sync with nothing pending must not touch the unreachable remote: %v", err)
	}

	// Control: once there IS something to push, the unreachable remote must
	// surface as a genuine error — the no-op above is a guard, not a swallow.
	dirty(t, f.work, `[{"id":0,"slug":"a","title":"offline"}]`)
	if err := w.Sync(); err == nil {
		t.Fatal("Sync with a pending commit and unreachable remote should fail")
	}
}
