//go:build integration

// boltpullfirst_integration_test.go proves Bolt.PullThenCommitWritten and the gated seed-commit drop under it.
// The hub is a hubforge hub whose records upstream a second clone moves.
//
// Package fabricengine_test; shares the single TestMain in testmain_test.go.

package fabricengine_test

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/testkit/logcapture"
)

const boltTestLabel = "(0123456789ab production /bin/lyx)"

// boltFixture is a hub, its board as a Bolt and the board's branch.
type boltFixture struct {
	t      *testing.T
	hub    *hubforge.Hub
	board  string
	branch string
	bolt   *fabricengine.Bolt
}

func newBoltFixture(t *testing.T) *boltFixture {
	t.Helper()

	hub := hubforge.NewHub(t, ".")
	board := hub.BoardDir()
	return &boltFixture{t: t, hub: hub, board: board, branch: gitkit.CurrentBranch(t, board), bolt: fabricengine.NewBolt(board)}
}

// moveUpstream commits rel on the records upstream from a second clone and returns the new upstream tip.
func (f *boltFixture) moveUpstream(rel, content string) string {
	f.t.Helper()

	other := filepath.Join(f.t.TempDir(), "other")
	gitkit.Git(f.t, filepath.Dir(other), "clone", "--branch", f.branch, f.hub.RecordsBare, other)
	sha := gitkit.CommitFile(f.t, other, rel, content, "upstream: "+rel)
	gitkit.Git(f.t, other, "push", "origin", f.branch)
	return sha
}

// seedCommitLocal commits rel on the board under a labelled seed subject.
func (f *boltFixture) seedCommitLocal(rel, content string) string {
	f.t.Helper()
	return gitkit.CommitFile(f.t, f.board, rel, content, fabricengine.SeedCommitMessage("stencils", boltTestLabel))
}

func (f *boltFixture) head() string {
	f.t.Helper()
	return gitkit.RevParse(f.t, f.board, "HEAD")
}

// requireCleanBoard fails the test when the board has uncommitted changes or a rebase in progress.
func (f *boltFixture) requireCleanBoard() {
	f.t.Helper()

	if status := gitkit.GitStatusPorcelain(f.t, f.board); status != "" {
		f.t.Errorf("board status = %q, want a clean tree", status)
	}
	for _, state := range []string{"rebase-merge", "rebase-apply"} {
		path := gitkit.Git(f.t, f.board, "rev-parse", "--path-format=absolute", "--git-path", state)
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			f.t.Errorf("stat %s = %v, want no rebase in progress", path, err)
		}
	}
}

// writeOf returns a BoltWrite that writes rel under the board and counts its runs.
func (f *boltFixture) writeOf(rel, content, message string, runs *int) fabricengine.BoltWrite {
	return fabricengine.BoltWrite{
		Message: message,
		Write: func() ([]string, error) {
			*runs++
			path := filepath.Join(f.board, filepath.FromSlash(rel))
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				return nil, err
			}
			return []string{rel}, os.WriteFile(path, []byte(content), 0o644)
		},
	}
}

func kindsOf(rec *fabricengine.Mutations) []fabricengine.Kind {
	var kinds []fabricengine.Kind
	for _, m := range rec.Snapshot().Entries() {
		kinds = append(kinds, m.Kind)
	}
	return kinds
}

func TestBolt_PullThenCommitWritten(t *testing.T) {
	t.Parallel()

	stencilPath := fabricengine.StencilsSubtreeRel() + "/loom/s.md"

	t.Run("behind only fast-forwards then commits", func(t *testing.T) {
		t.Parallel()
		f := newBoltFixture(t)
		upstream := f.moveUpstream("notes/up.md", "up\n")
		var runs int
		rec := fabricengine.NewMutations(f.hub.Path)

		res, err := f.bolt.PullThenCommitWritten([]fabricengine.BoltWrite{f.writeOf("notes/a.md", "a\n", "write a", &runs)}, rec)
		if err != nil {
			t.Fatalf("PullThenCommitWritten: %v", err)
		}
		if res.Skipped != "" || !res.Committed || len(res.SHAs) != 1 || res.SHAs[0] != f.head() {
			t.Errorf("result = %+v, head %s; want one landed commit at head and no skip", res, f.head())
		}
		if !gitkit.IsAncestor(t, f.board, upstream, "HEAD") {
			t.Errorf("upstream %s is not under the board's HEAD", upstream)
		}
		want := []fabricengine.Kind{fabricengine.KindRepoAdvanced, fabricengine.KindFileWritten, fabricengine.KindCommitCreated}
		if got := kindsOf(rec); !slices.Equal(got, want) {
			t.Errorf("record kinds = %v, want %v", got, want)
		}
		if got := res.Mutations.Len(); got != len(want) {
			t.Errorf("result record holds %d entries, want %d", got, len(want))
		}
	})

	t.Run("two writes land as two commits behind one pull", func(t *testing.T) {
		t.Parallel()
		f := newBoltFixture(t)
		f.moveUpstream("notes/up.md", "up\n")
		var runs int
		writes := []fabricengine.BoltWrite{
			f.writeOf("notes/a.md", "a\n", "write a", &runs),
			f.writeOf("notes/b.md", "b\n", "write b", &runs),
		}

		res, err := f.bolt.PullThenCommitWritten(writes, fabricengine.NewMutations(f.hub.Path))
		if err != nil {
			t.Fatalf("PullThenCommitWritten: %v", err)
		}
		if len(res.SHAs) != 2 || runs != 2 {
			t.Fatalf("landed %d commits over %d writes, want 2 and 2", len(res.SHAs), runs)
		}
		if got := gitkit.Git(t, f.board, "log", "--format=%s", "-2"); strings.TrimSpace(got) != "write b\nwrite a" {
			t.Errorf("commit subjects = %q, want write b over write a", got)
		}
	})

	t.Run("seed commits ahead are dropped and the write reruns on the pulled copy", func(t *testing.T) {
		t.Parallel()
		f := newBoltFixture(t)
		oldSeed := f.seedCommitLocal(stencilPath, "old\n")
		upstream := f.moveUpstream("notes/up.md", "up\n")
		var runs int
		rec := fabricengine.NewMutations(f.hub.Path)

		res, err := f.bolt.PullThenCommitWritten([]fabricengine.BoltWrite{f.writeOf(stencilPath, "new\n", fabricengine.SeedCommitMessage("stencils", boltTestLabel), &runs)}, rec)
		if err != nil {
			t.Fatalf("PullThenCommitWritten: %v", err)
		}
		if res.Skipped != "" || len(res.SHAs) != 1 || runs != 1 {
			t.Fatalf("result = %+v after %d write runs, want one landed commit and no skip", res, runs)
		}
		if gitkit.IsAncestor(t, f.board, oldSeed, "HEAD") {
			t.Errorf("the old seed commit %s survived the drop", oldSeed)
		}
		if got := gitkit.RevListCount(t, f.board, upstream+"..HEAD"); got != 1 {
			t.Errorf("board is %d commits past upstream, want only the rewritten seed commit", got)
		}
		var dropped []string
		for _, m := range rec.Snapshot().Entries() {
			if m.Kind == fabricengine.KindCommitsDropped {
				dropped = append(dropped, m.Detail)
			}
		}
		if !slices.Equal(dropped, []string{oldSeed}) {
			t.Errorf("commits_dropped details = %v, want [%s]", dropped, oldSeed)
		}
	})

	t.Run("non-seed commits over a moved upstream skip diverged and warn", func(t *testing.T) {
		t.Parallel()
		f := newBoltFixture(t)
		tip := gitkit.CommitFile(t, f.board, "notes/local.md", "local\n", "local work")
		f.moveUpstream("notes/up.md", "up\n")
		var runs int
		logs := logcapture.Capture(t)

		res, err := f.bolt.PullThenCommitWritten([]fabricengine.BoltWrite{f.writeOf("notes/a.md", "a\n", "write a", &runs)}, fabricengine.NewMutations(f.hub.Path))
		if err != nil {
			t.Fatalf("PullThenCommitWritten: %v", err)
		}
		if res.Skipped != fabricengine.BoltSkipDiverged || runs != 0 || f.head() != tip {
			t.Errorf("skipped %q, %d writes run, head %s; want diverged, none run, head %s", res.Skipped, runs, f.head(), tip)
		}
		for _, want := range []string{f.board, "git pull --rebase", "lyx board sync"} {
			if !strings.Contains(res.SkipDetail, want) {
				t.Errorf("detail %q does not name %q", res.SkipDetail, want)
			}
		}
		// Other parallel tests write to the same sink, so the count is of this board's own detail.
		if got := strings.Count(logs.String(), res.SkipDetail); got != 1 {
			t.Errorf("detail logged %d times, want once", got)
		}
	})

	t.Run("fetch failure skips with nothing written", func(t *testing.T) {
		t.Parallel()
		f := newBoltFixture(t)
		gitkit.Git(t, f.board, "remote", "set-url", "origin", filepath.Join(t.TempDir(), "gone"))
		var runs int

		res, err := f.bolt.PullThenCommitWritten([]fabricengine.BoltWrite{f.writeOf("notes/a.md", "a\n", "write a", &runs)}, fabricengine.NewMutations(f.hub.Path))
		if err != nil {
			t.Fatalf("PullThenCommitWritten: %v", err)
		}
		if res.Skipped != fabricengine.BoltSkipFetchFailed || runs != 0 || !strings.Contains(res.SkipDetail, "origin") {
			t.Errorf("skipped %q, %d writes run, detail %q; want fetch_failed naming origin and no write", res.Skipped, runs, res.SkipDetail)
		}
	})

	t.Run("a board with no upstream writes and commits", func(t *testing.T) {
		t.Parallel()
		for _, tc := range []struct {
			name       string
			fetchFails bool
		}{
			{name: "fetch succeeds"},
			{name: "fetch fails", fetchFails: true},
		} {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				f := newBoltFixture(t)
				gitkit.Git(t, f.board, "branch", "--unset-upstream")
				if tc.fetchFails {
					gitkit.Git(t, f.board, "remote", "set-url", "origin", filepath.Join(t.TempDir(), "gone"))
				}
				var runs int
				rec := fabricengine.NewMutations(f.hub.Path)

				res, err := f.bolt.PullThenCommitWritten([]fabricengine.BoltWrite{f.writeOf("notes/a.md", "a\n", "write a", &runs)}, rec)
				if err != nil {
					t.Fatalf("PullThenCommitWritten: %v", err)
				}
				if res.Skipped != "" || runs != 1 || len(res.SHAs) != 1 || res.SHAs[0] != f.head() {
					t.Errorf("result = %+v after %d write runs, head %s; want one landed commit at head and no skip", res, runs, f.head())
				}
				want := []fabricengine.Kind{fabricengine.KindFileWritten, fabricengine.KindCommitCreated}
				if got := kindsOf(rec); !slices.Equal(got, want) {
					t.Errorf("record kinds = %v, want %v", got, want)
				}
			})
		}
	})

	t.Run("a replay conflict restores the original tip", func(t *testing.T) {
		t.Parallel()
		f := newBoltFixture(t)
		f.seedCommitLocal(stencilPath, "seed\n")
		tip := gitkit.CommitFile(t, f.board, "notes/c.md", "local\n", "local work")
		f.moveUpstream("notes/c.md", "remote\n")
		var runs int
		rec := fabricengine.NewMutations(f.hub.Path)

		res, err := f.bolt.PullThenCommitWritten([]fabricengine.BoltWrite{f.writeOf("notes/a.md", "a\n", "write a", &runs)}, rec)
		if err != nil {
			t.Fatalf("PullThenCommitWritten: %v", err)
		}
		if res.Skipped != fabricengine.BoltSkipDiverged || runs != 0 {
			t.Errorf("skipped %q after %d write runs, want diverged and none", res.Skipped, runs)
		}
		if f.head() != tip {
			t.Errorf("head = %s, want the original tip %s", f.head(), tip)
		}
		if status := gitkit.GitStatusPorcelain(t, f.board); strings.TrimSpace(status) != "" {
			t.Errorf("board is left dirty after the restore: %q", status)
		}
		if rec.Snapshot().Len() != 0 {
			t.Errorf("a restored drop recorded %v", kindsOf(rec))
		}
	})

	t.Run("a dirty file outside the touched paths is carried across the drop", func(t *testing.T) {
		t.Parallel()
		f := newBoltFixture(t)
		f.moveUpstream("notes/d.md", "v1\n")
		gitkit.Git(t, f.board, "pull", "--ff-only")
		f.seedCommitLocal(stencilPath, "seed\n")
		f.moveUpstream("notes/up.md", "up\n")
		dirtyPath := filepath.Join(f.board, "notes", "d.md")
		if err := os.WriteFile(dirtyPath, []byte("dirty\n"), 0o644); err != nil {
			t.Fatalf("dirty the file: %v", err)
		}
		var runs int

		res, err := f.bolt.PullThenCommitWritten([]fabricengine.BoltWrite{f.writeOf("notes/a.md", "a\n", "write a", &runs)}, fabricengine.NewMutations(f.hub.Path))
		if err != nil {
			t.Fatalf("PullThenCommitWritten: %v", err)
		}
		if res.Skipped != "" || runs != 1 {
			t.Fatalf("skipped %q after %d write runs, want a run", res.Skipped, runs)
		}
		if got, _ := os.ReadFile(dirtyPath); string(got) != "dirty\n" {
			t.Errorf("dirty file = %q, want it carried across the drop", got)
		}
	})

	t.Run("a dirty file on a touched path skips dirty with Bolt unmoved", func(t *testing.T) {
		t.Parallel()
		f := newBoltFixture(t)
		f.seedCommitLocal(stencilPath, "seed\n")
		tip := f.head()
		f.moveUpstream("notes/up.md", "up\n")
		if err := os.WriteFile(filepath.Join(f.board, filepath.FromSlash(stencilPath)), []byte("dirty\n"), 0o644); err != nil {
			t.Fatalf("dirty the file: %v", err)
		}
		var runs int

		res, err := f.bolt.PullThenCommitWritten([]fabricengine.BoltWrite{f.writeOf("notes/a.md", "a\n", "write a", &runs)}, fabricengine.NewMutations(f.hub.Path))
		if err != nil {
			t.Fatalf("PullThenCommitWritten: %v", err)
		}
		if res.Skipped != fabricengine.BoltSkipDirty || runs != 0 || f.head() != tip {
			t.Errorf("skipped %q, %d writes run, head %s; want dirty, none run, head %s", res.Skipped, runs, f.head(), tip)
		}
		if !strings.Contains(res.SkipDetail, "lyx board sync") {
			t.Errorf("detail %q does not name lyx board sync", res.SkipDetail)
		}
	})

	t.Run("a labelled seed message touching a path outside the subtrees is kept and replayed", func(t *testing.T) {
		t.Parallel()
		f := newBoltFixture(t)
		oldSeed := f.seedCommitLocal(stencilPath, "old\n")
		gitkit.CommitFile(t, f.board, "notes/x.md", "x\n", fabricengine.SeedCommitMessage("stencils", boltTestLabel))
		upstream := f.moveUpstream("notes/up.md", "up\n")
		var runs int

		res, err := f.bolt.PullThenCommitWritten([]fabricengine.BoltWrite{f.writeOf("notes/a.md", "a\n", "write a", &runs)}, fabricengine.NewMutations(f.hub.Path))
		if err != nil {
			t.Fatalf("PullThenCommitWritten: %v", err)
		}
		if res.Skipped != "" {
			t.Fatalf("skipped %q, want the genuine seed commit dropped and the write to run", res.Skipped)
		}
		if gitkit.IsAncestor(t, f.board, oldSeed, "HEAD") {
			t.Errorf("the genuine seed commit %s survived", oldSeed)
		}
		if got := gitkit.Git(t, f.board, "log", "--format=%s", upstream+"..HEAD"); !strings.Contains(got, fabricengine.SeedCommitMessage("stencils", boltTestLabel)) {
			t.Errorf("commits past upstream = %q, want the out-of-subtree commit replayed under its message", got)
		}
		if got, _ := os.ReadFile(filepath.Join(f.board, "notes", "x.md")); string(got) != "x\n" {
			t.Errorf("notes/x.md = %q, want the replayed commit's content", got)
		}
	})

	t.Run("a held push lock delays the drop and the write", func(t *testing.T) {
		t.Parallel()
		f := newBoltFixture(t)
		f.seedCommitLocal(stencilPath, "old\n")
		tip := f.head()
		upstream := f.moveUpstream("notes/up.md", "up\n")
		pushLock, err := lock.AcquireWriteLock(filepath.Join(f.board, gitrepo.PushLockFileName))
		if err != nil {
			t.Fatalf("take the push lock: %v", err)
		}
		var runs int
		done := make(chan error, 1)
		go func() {
			_, err := f.bolt.PullThenCommitWritten([]fabricengine.BoltWrite{f.writeOf("notes/a.md", "a\n", "write a", &runs)}, fabricengine.NewMutations(f.hub.Path))
			done <- err
		}()

		// The goroutine cannot signal that it is blocked on the lock, so the check waits a short while; a call that ignored the lock would already have moved HEAD.
		time.Sleep(300 * time.Millisecond)
		if f.head() != tip {
			t.Errorf("head moved to %s while the push lock was held, want %s", f.head(), tip)
		}
		if err := pushLock.Release(); err != nil {
			t.Fatalf("release the push lock: %v", err)
		}
		if err := <-done; err != nil {
			t.Fatalf("PullThenCommitWritten: %v", err)
		}
		if !gitkit.IsAncestor(t, f.board, upstream, "HEAD") || runs != 1 {
			t.Errorf("after release: upstream under HEAD = false or %d write runs, want the drop and one write", runs)
		}
	})
}

func TestBolt_PushRecorded_DropsSeedCommitsOnRebaseConflict(t *testing.T) {
	t.Parallel()

	stencilPath := fabricengine.StencilsSubtreeRel() + "/loom/s.md"

	t.Run("a conflicting seed commit is dropped and the board commit lands", func(t *testing.T) {
		t.Parallel()
		f := newBoltFixture(t)
		seed := f.seedCommitLocal(stencilPath, "local\n")
		gitkit.CommitFile(t, f.board, "notes/a.md", "a\n", "board change")
		upstream := f.moveUpstream(stencilPath, "upstream\n")
		rec := fabricengine.NewMutations(f.hub.Path)

		if err := f.bolt.PushRecorded(fabricengine.SyncOptions{}, rec); err != nil {
			t.Fatalf("PushRecorded: %v", err)
		}

		if gitkit.IsAncestor(t, f.board, seed, "HEAD") || !gitkit.IsAncestor(t, f.board, upstream, "HEAD") {
			t.Errorf("seed %s under HEAD or upstream %s missing from it; want the seed dropped and the board commit on top of upstream", seed, upstream)
		}
		if got, want := gitkit.RevParse(t, f.hub.RecordsBare, "refs/heads/"+f.branch), f.head(); got != want {
			t.Errorf("origin tip = %s, want the pushed board tip %s", got, want)
		}
		entries := rec.Snapshot().Entries()
		if len(entries) != 1 || entries[0].Kind != fabricengine.KindCommitsDropped || !strings.Contains(entries[0].Detail, seed) {
			t.Errorf("record = %+v, want one commits_dropped entry naming %s", entries, seed)
		}
	})

	t.Run("a conflicting board commit leaves the board at its tip with the error and no record", func(t *testing.T) {
		t.Parallel()
		f := newBoltFixture(t)
		f.seedCommitLocal(stencilPath, "local\n")
		gitkit.CommitFile(t, f.board, "notes/a.md", "mine\n", "board change")
		tip := f.head()
		f.moveUpstream(stencilPath, "upstream\n")
		other := filepath.Join(t.TempDir(), "other")
		gitkit.Git(t, filepath.Dir(other), "clone", "--branch", f.branch, f.hub.RecordsBare, other)
		gitkit.CommitFile(t, other, "notes/a.md", "theirs\n", "upstream: notes/a.md")
		gitkit.Git(t, other, "push", "origin", f.branch)
		rec := fabricengine.NewMutations(f.hub.Path)

		err := f.bolt.PushRecorded(fabricengine.SyncOptions{}, rec)

		if !errors.Is(err, gitrepo.ErrPullRebaseFailed) {
			t.Errorf("PushRecorded error = %v, want one satisfying ErrPullRebaseFailed", err)
		}
		if f.head() != tip {
			t.Errorf("head = %s, want the original tip %s", f.head(), tip)
		}
		f.requireCleanBoard()
		if n := rec.Snapshot().Len(); n != 0 {
			t.Errorf("record holds %d entries, want none", n)
		}
	})

	t.Run("a conflict with no seed commits ahead returns the error without a drop", func(t *testing.T) {
		t.Parallel()
		f := newBoltFixture(t)
		gitkit.CommitFile(t, f.board, "notes/a.md", "mine\n", "board change")
		tip := f.head()
		f.moveUpstream("notes/a.md", "theirs\n")
		rec := fabricengine.NewMutations(f.hub.Path)

		err := f.bolt.PushRecorded(fabricengine.SyncOptions{}, rec)

		if !errors.Is(err, gitrepo.ErrPullRebaseFailed) {
			t.Errorf("PushRecorded error = %v, want one satisfying ErrPullRebaseFailed", err)
		}
		if f.head() != tip {
			t.Errorf("head = %s, want the original tip %s", f.head(), tip)
		}
		f.requireCleanBoard()
		if n := rec.Snapshot().Len(); n != 0 {
			t.Errorf("record holds %d entries, want none", n)
		}
	})

	t.Run("a retry that also fails returns its error and leaves the dropped board clean", func(t *testing.T) {
		t.Parallel()
		f := newBoltFixture(t)
		seed := f.seedCommitLocal(stencilPath, "local\n")
		gitkit.CommitFile(t, f.board, "notes/a.md", "a\n", "board change")
		upstream := f.moveUpstream(stencilPath, "upstream\n")
		rejectPushes := filepath.Join(f.hub.RecordsBare, "hooks", "pre-receive")
		if err := os.WriteFile(rejectPushes, []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
			t.Fatalf("write pre-receive hook: %v", err)
		}
		rec := fabricengine.NewMutations(f.hub.Path)

		err := f.bolt.PushRecorded(fabricengine.SyncOptions{}, rec)

		if err == nil || errors.Is(err, gitrepo.ErrPullRebaseFailed) {
			t.Errorf("PushRecorded error = %v, want the retry's own push failure", err)
		}
		if gitkit.IsAncestor(t, f.board, seed, "HEAD") || !gitkit.IsAncestor(t, f.board, upstream, "HEAD") {
			t.Errorf("seed %s under HEAD or upstream %s missing from it; want the drop kept", seed, upstream)
		}
		f.requireCleanBoard()
		entries := rec.Snapshot().Entries()
		if len(entries) != 1 || entries[0].Kind != fabricengine.KindCommitsDropped || !strings.Contains(entries[0].Detail, seed) {
			t.Errorf("record = %+v, want one commits_dropped entry naming %s", entries, seed)
		}
	})
}

func TestBoardDropRequest_RefusesForeignBoard(t *testing.T) {
	t.Parallel()

	f := newBoltFixture(t)
	foreign := filepath.Join(t.TempDir(), "elsewhere", fabricengine.BoardDirName)
	if err := os.MkdirAll(filepath.Dir(foreign), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	gitkit.MustRun(t, f.hub.PrimeRecords(), "git", "worktree", "add", "-b", "foreign-board", foreign)
	rec := fabricengine.NewMutations("")
	tip := gitkit.RevParse(t, foreign, "HEAD")

	err := fabricengine.DropSeedCommitsForTest(rec, fabricengine.BoardDropRequestForTest(foreign, nil), gitrepo.New(foreign), tip, nil)

	refusal, ok := fabricengine.RefusalOf(err)
	if !ok || refusal.Check != fabricengine.CheckOwnership {
		t.Fatalf("err = %v, want an ownership refusal", err)
	}
	if rec.Snapshot().Len() != 0 {
		t.Errorf("a refusal recorded %v", kindsOf(rec))
	}
}
