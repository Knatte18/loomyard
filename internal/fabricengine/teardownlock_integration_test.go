// teardownlock_integration_test.go covers the per-slug teardown lock between Remove and the pair's detached pushes:
// Remove's refusal while a push of the pair holds the lock;
// the shared hold of a push child for its whole run;
// a child that meets the lock held or the worktrees gone;
// and a Remove that fails and leaves a worktree behind, which spawns no push.
// It also covers Add, the task-branch discard and the synchronous pair push meeting an exclusive hold of the lock.
//
// Package fabricengine_test; shares the single TestMain in testmain_test.go.

package fabricengine_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// holdTeardownLock takes slug's teardown lock exclusive, as a removal, an Add or a discard would.
// It returns the release, and the test's cleanup releases it too.
func holdTeardownLock(t *testing.T, h *hubforge.Hub, slug string) (release func()) {
	t.Helper()

	lockPath := fabriclayout.TeardownLockPath(h.Path, slug)
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
		t.Fatalf("mkdir the teardown lock dir: %v", err)
	}
	held, err := lock.AcquireWriteLock(lockPath)
	if err != nil {
		t.Fatalf("hold the teardown lock exclusive: %v", err)
	}
	var once sync.Once
	release = func() { once.Do(func() { _ = held.Release() }) }
	t.Cleanup(release)
	return release
}

// teardownTestWait is the shortened teardown-lock wait of every row;
// no row sleeps, since each ordering is told by channels.
const teardownTestWait = 100 * time.Millisecond

// pairPushRequest is the detached push request a status push of slug's pair makes.
func pairPushRequest(h *hubforge.Hub, slug string) fabriclayout.PushSpawn {
	return fabriclayout.PushSpawn{
		WarpPath:     fabricengine.WorktreePath(h.Location, slug),
		WeftPath:     fabricengine.RecordsWorktreePath(h.Location, slug),
		PauseSlug:    slug,
		TeardownSlug: slug,
		WorkDir:      fabriclayout.BoardDir(h.Path),
	}
}

// requireBranchesGoneFromOrigins fails unless neither bare remote lists slug's code or records branch.
func requireBranchesGoneFromOrigins(t *testing.T, h *hubforge.Hub, slug string) {
	t.Helper()

	for bare, branch := range map[string]string{h.CodeBare: slug, h.RecordsBare: fabriclayout.RecordsBranchName(slug)} {
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
	tiergate.Require(t, 1)
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
		lockPath := fabriclayout.TeardownLockPath(h.Path, slug)
		if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
			t.Fatalf("mkdir the teardown lock dir: %v", err)
		}
		held, ok, err := lock.TryAcquireReadLock(lockPath)
		if err != nil || !ok {
			t.Fatalf("hold the teardown lock shared = (%v, %v); want (lock, true, nil)", ok, err)
		}
		defer held.Release()

		_, err = h.Topology.Remove(h.Location, slug, false, false, "")
		if !errors.Is(err, fabricengine.ErrPairPushRunning) {
			t.Fatalf("Remove with a shared holder = %v; want an error wrapping ErrPairPushRunning", err)
		}
		if !strings.Contains(err.Error(), "lym fabric remove") {
			t.Errorf("Remove error = %q; want it to name `lym fabric remove`", err.Error())
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

	t.Run("RemoveWhoseWaitRunsOutNamesTheGivenRerun", func(t *testing.T) {
		const rerun = "lym batten cancel " + slug + " --remove"
		h := newPairHub(t)
		holdTeardownLock(t, h, slug)

		_, err := h.Topology.Remove(h.Location, slug, false, false, rerun)
		if !errors.Is(err, fabricengine.ErrPairPushRunning) {
			t.Fatalf("Remove beside an exclusive holder = %v; want an error wrapping ErrPairPushRunning", err)
		}
		if want := "re-run `" + rerun + "`"; !strings.Contains(err.Error(), want) {
			t.Errorf("Remove error = %q; want it to name %q", err.Error(), want)
		}
		if strings.Contains(err.Error(), "lym fabric remove") {
			t.Errorf("Remove error = %q; want it not to name `lym fabric remove`", err.Error())
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
			_, err := fabricengine.DetachedPush(h.Path, pairPushRequest(h, slug), fabriclayout.MachineNames{})
			done <- err
		}()
		<-parked

		if _, err := h.Topology.Remove(h.Location, slug, false, false, ""); !errors.Is(err, fabricengine.ErrPairPushRunning) {
			t.Fatalf("Remove beside a parked push child = %v; want an error wrapping ErrPairPushRunning", err)
		}

		close(release)
		if err := <-done; err != nil {
			t.Fatalf("DetachedPush error = %v; want nil", err)
		}
		if _, err := h.Topology.Remove(h.Location, slug, false, false, ""); err != nil {
			t.Fatalf("Remove after the push child returned error = %v; want nil", err)
		}
	})

	t.Run("PushChildStartedDuringRemoveTakesNoLockAndPushesNothing", func(t *testing.T) {
		h := newPairHub(t)
		var childErr error
		var childMutations int
		fabricengine.SetTeardownHooksForTest(t, nil, func() {
			res, err := fabricengine.DetachedPush(h.Path, pairPushRequest(h, slug), fabriclayout.MachineNames{})
			childErr, childMutations = err, res.Mutated().Len()
		})

		if _, err := h.Topology.Remove(h.Location, slug, false, false, ""); err != nil {
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
		if _, err := h.Topology.Remove(h.Location, slug, false, true, ""); err != nil {
			t.Fatalf("Remove(remote) error = %v; want nil", err)
		}

		if _, err := fabricengine.DetachedPush(h.Path, pairPushRequest(h, slug), fabriclayout.MachineNames{}); err != nil {
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

	t.Run("FailedRemoveSpawnsNoPushAndReleasesTheLock", func(t *testing.T) {
		h := newPairHub(t)
		t.Cleanup(fabricengine.SetRemoveAfterLockHookForTest(func() error { return errors.New("injected failure after the teardown lock") }))
		recorder := fabricengine.SwapPushRecorderForTest(t)

		if _, err := h.Topology.Remove(h.Location, slug, false, false, ""); err == nil {
			t.Fatalf("Remove with a failure after the lock = nil error; want the injected failure")
		}

		if calls := recorder.Calls(); len(calls) != 0 {
			t.Errorf("push requests after a failed Remove = %+v; want none", calls)
		}
		if relock, ok, err := lock.TryAcquireWriteLock(fabriclayout.TeardownLockPath(h.Path, slug)); err != nil || !ok {
			t.Errorf("teardown lock after a failed Remove = (%v, %v); want it released", ok, err)
		} else {
			relock.Release()
		}
	})

	t.Run("FailedRemoveAfterTheTaskTeardownSpawnsNoPush", func(t *testing.T) {
		h := newPairHub(t)
		gitkit.MustRun(t, mustRecordsRepoRoot(t, h.Location), "git", "worktree", "lock", h.PairRecordsSibling(slug))
		recorder := fabricengine.SwapPushRecorderForTest(t)

		if _, err := h.Topology.Remove(h.Location, slug, false, false, ""); err == nil {
			t.Fatalf("Remove with a locked records worktree = nil error; want the weft teardown failure")
		}

		if calls := recorder.Calls(); len(calls) != 0 {
			t.Errorf("push requests after a failed Remove = %+v; want none", calls)
		}
	})
}

// TestPushPairAnchored_HeldByAPauseWithoutTheTeardownLock covers a pair push a pause holds.
// At every wait the pause makes, the pair's teardown lock is free for an exclusive taker such as a removal.
// The push then lands the pair's records commit.
// Serial: it installs the process's network gate.
func TestPushPairAnchored_HeldByAPauseWithoutTheTeardownLock(t *testing.T) {
	tiergate.Require(t, 1)
	const slug = "lock-pair-pause"
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	hubforge.AddPairWith(t, h, slug, fabricengine.AddOptions{})
	pair, err := lymcwd.ResolveWorktree(h.PairCodeWorktree(slug))
	if err != nil {
		t.Fatalf("resolve the pair's location: %v", err)
	}
	if _, err := fabricengine.PushPairAnchored(pair, fabricengine.SyncOptions{}, fabriclayout.PairPush{LockWait: fabriclayout.LockWaitUnbounded}); err != nil {
		t.Fatalf("first PushPairAnchored error = %v; want nil", err)
	}
	gitkit.CommitFile(t, h.PairRecordsSibling(slug), "paused.txt", "paused\n", "records work under a pause")
	teardownLockPath := fabriclayout.TeardownLockPath(h.Path, slug)
	waits := fabricengine.PassingPauseGateForTest(t, func() {
		if !fabricengine.LockFreeForTest(t, teardownLockPath) {
			t.Errorf("the teardown lock was held while the pause held the push; want it free")
		}
	})

	if _, err := fabricengine.PushPairAnchored(pair, fabricengine.SyncOptions{}, fabriclayout.PairPush{LockWait: fabriclayout.LockWaitUnbounded}); err != nil {
		t.Fatalf("PushPairAnchored under a pause error = %v; want nil", err)
	}

	if *waits == 0 {
		t.Errorf("the pause held no call of the push; want at least one")
	}
	weftBranch := fabriclayout.RecordsBranchName(slug)
	if got, want := gitkit.RevParse(t, h.RecordsBare, weftBranch), gitkit.RevParse(t, h.PairRecordsSibling(slug), "HEAD"); got != want {
		t.Errorf("origin weft tip = %s; want the pushed %s", got, want)
	}
}

// TestTeardownLock_SerialisesAddDiscardAndPairPush holds the slug's teardown lock exclusive, one hub per row.
// It meets Add, the task-branch discard and the synchronous prime push with that hold.
// Each row sets its wait on its own Topology, and the test releases the holder,
// so no row sleeps and no process-global state is touched.
func TestTeardownLock_SerialisesAddDiscardAndPairPush(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	t.Run("AddWaitsForTheHolderThenMakesThePair", func(t *testing.T) {
		t.Parallel()
		const slug = "lock-add-waits"
		h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
		release := holdTeardownLock(t, h, slug)

		done := make(chan error, 1)
		go func() {
			_, err := h.Topology.Add(h.Location, slug, fabricengine.AddOptions{})
			done <- err
		}()
		release()

		if err := <-done; err != nil {
			t.Fatalf("Add after the holder released error = %v; want nil", err)
		}
		if _, err := os.Lstat(h.PairCodeWorktree(slug)); err != nil {
			t.Errorf("the pair's code worktree is missing after Add: %v", err)
		}
	})

	t.Run("AddWhoseWaitRunsOutRefusesBeforeAnyMutation", func(t *testing.T) {
		t.Parallel()
		const slug = "lock-add-refused"
		h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
		fabricengine.SetTeardownWaitForTest(h.Topology, teardownTestWait)
		holdTeardownLock(t, h, slug)

		_, err := h.Topology.Add(h.Location, slug, fabricengine.AddOptions{})
		if !errors.Is(err, fabricengine.ErrPairPushRunning) {
			t.Fatalf("Add beside an exclusive holder = %v; want an error wrapping ErrPairPushRunning", err)
		}
		if !strings.Contains(err.Error(), "re-run the command that made the pair") {
			t.Errorf("Add error = %q; want it to name re-running the command that made the pair", err.Error())
		}
		requireNothingCreated(t, h, slug)
	})

	t.Run("DiscardWaitsForTheHolderThenArchivesAndDeletes", func(t *testing.T) {
		t.Parallel()
		const slug = "lock-discard-waits"
		h, _ := discardHub(t, slug)
		release := holdTeardownLock(t, h, slug)

		type outcome struct {
			res fabriclayout.DiscardResult
			err error
		}
		done := make(chan outcome, 1)
		go func() {
			res, err := h.Topology.DiscardTaskBranch(h.Location, slug, "")
			done <- outcome{res, err}
		}()
		release()

		got := <-done
		if got.err != nil {
			t.Fatalf("DiscardTaskBranch after the holder released error = %v; want nil", got.err)
		}
		if !got.res.LocalDeleted || !got.res.OriginDeleted {
			t.Errorf("LocalDeleted = %v, OriginDeleted = %v; want both true", got.res.LocalDeleted, got.res.OriginDeleted)
		}
	})

	t.Run("DiscardWhoseWaitRunsOutKeepsTheTaskBranch", func(t *testing.T) {
		t.Parallel()
		const slug = "lock-discard-refused"
		h, _ := discardHub(t, slug)
		fabricengine.SetTeardownWaitForTest(h.Topology, teardownTestWait)
		holdTeardownLock(t, h, slug)

		_, err := h.Topology.DiscardTaskBranch(h.Location, slug, "")
		if !errors.Is(err, fabricengine.ErrPairPushRunning) {
			t.Fatalf("DiscardTaskBranch beside an exclusive holder = %v; want an error wrapping ErrPairPushRunning", err)
		}
		if !strings.Contains(err.Error(), "re-run the command that discarded the task branch") {
			t.Errorf("DiscardTaskBranch error = %q; want it to name re-running the command that discarded the task branch", err.Error())
		}
		if !gitkit.BranchExists(t, h.PrimeWorktree(), slug) {
			t.Errorf("task branch %q gone locally; want it kept", slug)
		}
		if !gitkit.BranchExists(t, h.CodeBare, slug) {
			t.Errorf("task branch %q gone from origin; want it kept", slug)
		}
		if tags := archiveTagsOnOrigin(t, h, slug); len(tags) != 0 {
			t.Errorf("archive tags on origin = %v; want none after a refused discard", tags)
		}
	})

	t.Run("DiscardWhoseWaitRunsOutNamesTheGivenRerun", func(t *testing.T) {
		t.Parallel()
		const slug = "lock-discard-given-rerun"
		const rerun = "lym batten cancel lock-discard-given-rerun --remove"
		h, _ := discardHub(t, slug)
		fabricengine.SetTeardownWaitForTest(h.Topology, teardownTestWait)
		holdTeardownLock(t, h, slug)

		_, err := h.Topology.DiscardTaskBranch(h.Location, slug, rerun)
		if !errors.Is(err, fabricengine.ErrPairPushRunning) {
			t.Fatalf("DiscardTaskBranch beside an exclusive holder = %v; want an error wrapping ErrPairPushRunning", err)
		}
		if want := "re-run `" + rerun + "`"; !strings.Contains(err.Error(), want) {
			t.Errorf("DiscardTaskBranch error = %q; want it to name %q", err.Error(), want)
		}
	})

	t.Run("PrimePushIgnoresAPairsExclusiveHold", func(t *testing.T) {
		t.Parallel()
		const slug = "lock-prime-push"
		h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
		hubforge.AddPairWith(t, h, slug, fabricengine.AddOptions{})
		holdTeardownLock(t, h, slug)
		gitkit.CommitFile(t, h.PrimeRecords(), "prime-held.txt", "prime\n", "prime records work")

		if _, err := fabricengine.PushPairAnchored(h.Location, fabricengine.SyncOptions{}, fabriclayout.PairPush{LockWait: fabriclayout.LockWaitUnbounded}); err != nil {
			t.Fatalf("prime PushPairAnchored error = %v; want nil", err)
		}
		branch := gitkit.Git(t, h.PrimeRecords(), "rev-parse", "--abbrev-ref", "HEAD")
		if got, want := gitkit.RevParse(t, h.RecordsBare, branch), gitkit.RevParse(t, h.PrimeRecords(), "HEAD"); got != want {
			t.Errorf("origin prime records tip = %s; want the pushed %s", got, want)
		}
	})
}
