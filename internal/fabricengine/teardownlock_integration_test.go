//go:build tier1

// teardownlock_integration_test.go covers the per-slug teardown lock between Remove and the pair's detached pushes:
// Remove's refusal while a push of the pair holds the lock, the shared hold of a push child for its whole run, a child that meets the lock held or the worktrees gone, and the respawn of the pair's push after a Remove that fails and leaves a worktree behind.
//
// Package fabricengine_test; shares the single TestMain in testmain_test.go.

package fabricengine_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lock"
)

// teardownTestWait is the shortened teardown-lock wait of every row; no row sleeps, since each ordering is told by channels.
const teardownTestWait = 100 * time.Millisecond

// pairPushRequest is the detached push request a status push of slug's pair makes.
func pairPushRequest(h *hubforge.Hub, slug string) fabricengine.PushSpawn {
	return fabricengine.PushSpawn{
		WarpPath:     fabricengine.WorktreePath(h.Location, slug),
		WeftPath:     fabricengine.RecordsWorktreePath(h.Location, slug),
		PauseSlug:    slug,
		TeardownSlug: slug,
		WorkDir:      fabricengine.BoardDir(h.Path),
	}
}

// requireBranchesGoneFromOrigins fails unless neither bare remote lists slug's code or records branch.
func requireBranchesGoneFromOrigins(t *testing.T, h *hubforge.Hub, slug string) {
	t.Helper()

	for bare, branch := range map[string]string{h.CodeBare: slug, h.RecordsBare: fabricengine.RecordsBranchName(slug)} {
		out, err := gitexec.Run([]string{"ls-remote", "--heads", bare, branch}, h.Container)
		if err != nil {
			t.Fatalf("ls-remote %s: %v", bare, err)
		}
		if strings.TrimSpace(out) != "" {
			t.Errorf("origin %s still lists %s: %q", bare, branch, out)
		}
	}
}

// TestRemove_TeardownLockAgainstThePairsPush drives Remove against the pair's own detached pushes, one hub per row.
// It sets package-level hooks, so neither it nor its rows are parallel.
func TestRemove_TeardownLockAgainstThePairsPush(t *testing.T) {
	const slug = "teardown-lock"

	newPairHub := func(t *testing.T) *hubforge.Hub {
		t.Helper()
		h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
		hubforge.AddPairWith(t, h, slug, fabricengine.AddOptions{})
		fabricengine.SetTeardownWaitForTest(h.Topology, teardownTestWait)
		return h
	}

	t.Run("SharedHolderPastTheWaitRefusesBeforeAnyMutation", func(t *testing.T) {
		h := newPairHub(t)
		lockPath := fabricengine.TeardownLockPath(h.Path, slug)
		if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
			t.Fatalf("mkdir the teardown lock dir: %v", err)
		}
		held, ok, err := lock.TryAcquireReadLock(lockPath)
		if err != nil || !ok {
			t.Fatalf("hold the teardown lock shared = (%v, %v); want (lock, true, nil)", ok, err)
		}
		defer held.Release()

		_, err = h.Topology.Remove(h.Location, slug, false, false)
		if !errors.Is(err, fabricengine.ErrPairPushRunning) {
			t.Fatalf("Remove with a shared holder = %v; want an error wrapping ErrPairPushRunning", err)
		}
		if !strings.Contains(err.Error(), "lyx fabric remove") {
			t.Errorf("Remove error = %q; want it to name `lyx fabric remove`", err.Error())
		}
		for _, p := range []string{h.PairCodeWorktree(slug), h.PairRecordsSibling(slug)} {
			if _, statErr := os.Lstat(p); statErr != nil {
				t.Errorf("%s missing after a refused Remove: %v", p, statErr)
			}
		}
		if tags := archiveTagsAt(t, h.RecordsBare); len(tags) != 0 {
			t.Errorf("origin archive tags = %v; want none after a refused Remove", tags)
		}
	})

	t.Run("PushChildHoldsTheLockSharedForItsWholeRun", func(t *testing.T) {
		h := newPairHub(t)
		parked, release, done := make(chan struct{}), make(chan struct{}), make(chan error, 1)
		fabricengine.SetTeardownHooksForTest(t, func() {
			close(parked)
			<-release
		}, nil)

		go func() {
			_, err := fabricengine.DetachedPush(h.Path, pairPushRequest(h, slug))
			done <- err
		}()
		<-parked

		if _, err := h.Topology.Remove(h.Location, slug, false, false); !errors.Is(err, fabricengine.ErrPairPushRunning) {
			t.Fatalf("Remove beside a parked push child = %v; want an error wrapping ErrPairPushRunning", err)
		}

		close(release)
		if err := <-done; err != nil {
			t.Fatalf("DetachedPush error = %v; want nil", err)
		}
		if _, err := h.Topology.Remove(h.Location, slug, false, false); err != nil {
			t.Fatalf("Remove after the push child returned error = %v; want nil", err)
		}
	})

	t.Run("PushChildStartedDuringRemoveTakesNoLockAndPushesNothing", func(t *testing.T) {
		h := newPairHub(t)
		var childErr error
		var childMutations int
		fabricengine.SetTeardownHooksForTest(t, nil, func() {
			res, err := fabricengine.DetachedPush(h.Path, pairPushRequest(h, slug))
			childErr, childMutations = err, res.Mutated().Len()
		})

		if _, err := h.Topology.Remove(h.Location, slug, false, false); err != nil {
			t.Fatalf("Remove error = %v; want nil", err)
		}
		if childErr != nil {
			t.Errorf("DetachedPush during Remove error = %v; want nil", childErr)
		}
		if childMutations != 0 {
			t.Errorf("DetachedPush during Remove recorded %d mutations; want none", childMutations)
		}
	})

	t.Run("PushChildAfterRemoveRecreatesNothing", func(t *testing.T) {
		h := newPairHub(t)
		if _, err := h.Topology.Remove(h.Location, slug, false, true); err != nil {
			t.Fatalf("Remove(remote) error = %v; want nil", err)
		}

		if _, err := fabricengine.DetachedPush(h.Path, pairPushRequest(h, slug)); err != nil {
			t.Fatalf("DetachedPush after Remove error = %v; want nil", err)
		}
		for _, p := range []string{h.PairCodeWorktree(slug), h.PairRecordsSibling(slug)} {
			if _, err := os.Lstat(p); !os.IsNotExist(err) {
				t.Errorf("%s after a push child that found the pair removed: Lstat error = %v; want it still absent", p, err)
			}
		}
		requireBranchesGoneFromOrigins(t, h, slug)
		hubforge.AddPairWith(t, h, slug, fabricengine.AddOptions{})
	})

	t.Run("FailedRemoveRespawnsThePairsPushWithBothWorktrees", func(t *testing.T) {
		h := newPairHub(t)
		mustBreakOrigin(t, mustRecordsRepoRoot(t, h.Location))
		recorder := fabricengine.SwapPushRecorderForTest(t)

		if _, err := h.Topology.Remove(h.Location, slug, false, false); err == nil {
			t.Fatalf("Remove with an unreachable origin = nil error; want the archive failure")
		}

		calls := recorder.Calls()
		if len(calls) != 1 {
			t.Fatalf("push requests after a failed Remove = %d; want 1", len(calls))
		}
		want := pairPushRequest(h, slug)
		got := calls[0]
		if got.WarpPath != want.WarpPath || got.WeftPath != want.WeftPath || got.PauseSlug != slug || got.TeardownSlug != slug || got.WorkDir != want.WorkDir {
			t.Errorf("push request = %+v; want the pair's two worktrees, both slugs and the board dir from %+v", got, want)
		}
		if relock, ok, err := lock.TryAcquireWriteLock(fabricengine.TeardownLockPath(h.Path, slug)); err != nil || !ok {
			t.Errorf("teardown lock after a failed Remove = (%v, %v); want it released", ok, err)
		} else {
			relock.Release()
		}
	})

	t.Run("FailedRemoveAfterTheTaskTeardownRespawnsTheRecordsSideOnly", func(t *testing.T) {
		h := newPairHub(t)
		recordsWorktree := h.PairRecordsSibling(slug)
		gitkit.MustRun(t, mustRecordsRepoRoot(t, h.Location), "git", "worktree", "lock", recordsWorktree)
		recorder := fabricengine.SwapPushRecorderForTest(t)

		if _, err := h.Topology.Remove(h.Location, slug, false, false); err == nil {
			t.Fatalf("Remove with a locked records worktree = nil error; want the weft teardown failure")
		}

		calls := recorder.Calls()
		if len(calls) != 1 {
			t.Fatalf("push requests after a failed Remove = %d; want 1", len(calls))
		}
		got := calls[0]
		if got.WarpPath != "" || got.WeftPath != recordsWorktree || got.PauseSlug != slug || got.TeardownSlug != slug || got.WorkDir != fabricengine.BoardDir(h.Path) {
			t.Errorf("push request = %+v; want only the records worktree %s, both slugs and the board dir", got, recordsWorktree)
		}
	})
}
