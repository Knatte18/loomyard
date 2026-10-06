// teardown_test.go covers NewWorktreeTeardown over fake seams: the strict Shutdown-before-Remove
// ordering, which half a stuck reason names, the abandoned-session log path, and prime-lock
// dispositions.

package battenshed

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/testkit/shedfake"
)

// teardownCallRecorder records the order Shutdown and Remove were called in.
type teardownCallRecorder struct {
	calls []string
}

func (r *teardownCallRecorder) deps(shutdownErr error, abandonedSession string, removeErr error) TeardownDeps {
	return TeardownDeps{
		Shutdown: func(ctx context.Context) (string, error) {
			r.calls = append(r.calls, "shutdown")
			return abandonedSession, shutdownErr
		},
		Remove: func(ctx context.Context) error {
			r.calls = append(r.calls, "remove")
			return removeErr
		},
	}
}

func TestWorktreeTeardown_ShutdownFailureNeverCallsRemove(t *testing.T) {
	scratchDir := t.TempDir()
	var released bool
	lock := fakePrimeLock("/lock/path", true, nil, nil, &released)

	shutdownErr := errors.New("session would not end")
	rec := &teardownCallRecorder{}
	producer := NewWorktreeTeardown("teardown", "myslug", rec.deps(shutdownErr, "", nil), lock, scratchDir)

	ptr := shedfake.RequireOutcome(t, producer, shedengine.Stuck)
	if len(rec.calls) != 1 || rec.calls[0] != "shutdown" {
		t.Errorf("call order = %v; want [shutdown] only, Remove must never be called", rec.calls)
	}
	if !released {
		t.Error("release was not invoked on the Shutdown-error Stuck path")
	}
	reason := readStuckFile(t, scratchDir, "teardown", ptr)
	if !strings.Contains(reason, "session shutdown") {
		t.Errorf("stuck-reason file = %q; want it to name session shutdown as the failed half", reason)
	}
	if !strings.Contains(reason, shutdownErr.Error()) {
		t.Errorf("stuck-reason file = %q; want it to contain the underlying error", reason)
	}
}

//testtiming:keep pins the removal half and the shutdown-already-succeeded statement in the stuck reason, which the lock-disposition test never asserts
func TestWorktreeTeardown_RemoveFailureNamesRemovalHalf(t *testing.T) {
	scratchDir := t.TempDir()
	var released bool
	lock := fakePrimeLock("/lock/path", true, nil, nil, &released)

	removeErr := errors.New("worktree remove refused: merge in progress")
	rec := &teardownCallRecorder{}
	producer := NewWorktreeTeardown("teardown", "myslug", rec.deps(nil, "", removeErr), lock, scratchDir)

	ptr := shedfake.RequireOutcome(t, producer, shedengine.Stuck)
	if len(rec.calls) != 2 {
		t.Errorf("call count = %d; want 2 (both Shutdown and Remove called)", len(rec.calls))
	}
	reason := readStuckFile(t, scratchDir, "teardown", ptr)
	if !strings.Contains(reason, "worktree removal") {
		t.Errorf("stuck-reason file = %q; want it to name worktree removal as the failed half", reason)
	}
	if !strings.Contains(reason, "shutdown") || !strings.Contains(reason, "already succeeded") {
		t.Errorf("stuck-reason file = %q; want it to state that shutdown already succeeded", reason)
	}
	if !strings.Contains(reason, "merge in progress") {
		t.Errorf("stuck-reason file = %q; want the merge-in-progress refusal text preserved", reason)
	}
}

func TestWorktreeTeardown_PrimeLockDispositions(t *testing.T) {
	t.Run("contention", func(t *testing.T) {
		scratchDir := t.TempDir()
		var released bool
		lock := fakePrimeLock("/lock/contended/path", false, nil, nil, &released)
		rec := &teardownCallRecorder{}
		producer := NewWorktreeTeardown("teardown", "myslug", rec.deps(nil, "", nil), lock, scratchDir)

		ptr := shedfake.RequireOutcome(t, producer, shedengine.Stuck)
		if len(rec.calls) != 0 {
			t.Errorf("call count = %d; want 0 under lock contention", len(rec.calls))
		}
		reason := readStuckFile(t, scratchDir, "teardown", ptr)
		if !strings.Contains(reason, "/lock/contended/path") {
			t.Errorf("stuck-reason file = %q; want it to name the prime lock path", reason)
		}
	})

	t.Run("acquire error", func(t *testing.T) {
		scratchDir := t.TempDir()
		var released bool
		acquireErr := errors.New("flock: device error")
		lock := fakePrimeLock("/lock/path", false, acquireErr, nil, &released)
		rec := &teardownCallRecorder{}
		producer := NewWorktreeTeardown("teardown", "myslug", rec.deps(nil, "", nil), lock, scratchDir)

		_, _, err := producer.Call(context.Background())
		if err == nil {
			t.Fatal("Call() error = nil; want a returned error")
		}
		if !errors.Is(err, acquireErr) {
			t.Errorf("Call() error = %v; want it to wrap %v", err, acquireErr)
		}
	})

	t.Run("release on every exit path", func(t *testing.T) {
		scratchDir := t.TempDir()
		var released bool
		lock := fakePrimeLock("/lock/path", true, nil, nil, &released)
		removeErr := errors.New("removal failed")
		rec := &teardownCallRecorder{}
		producer := NewWorktreeTeardown("teardown", "myslug", rec.deps(nil, "", removeErr), lock, scratchDir)

		shedfake.CallOK(t, producer)
		if !released {
			t.Error("release was not invoked on the Remove-error Stuck path")
		}
	})
}

// TestWorktreeTeardown_CancelledDuringAcquireError mirrors
// TestWorktreeCreate_CancelledDuringAcquireError (create_test.go): the Acquire-error hard-error
// path must consult cancelErr before returning, per F1 (crucible round sonnet-xhigh-r3).
func TestWorktreeTeardown_CancelledDuringAcquireError(t *testing.T) {
	scratchDir := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	acquireErr := errors.New("flock: device error")
	lock := PrimeLock{
		Path: "/lock/path",
		Acquire: func() (func() error, bool, error) {
			cancel()
			return nil, false, acquireErr
		},
	}

	rec := &teardownCallRecorder{}
	producer := NewWorktreeTeardown("teardown", "myslug", rec.deps(nil, "", nil), lock, scratchDir)

	_, _, err := producer.Call(ctx)
	if err == nil {
		t.Fatal("Call() error = nil; want the cancelled-context diagnosis")
	}
	if !strings.Contains(err.Error(), "context cancelled during run") {
		t.Errorf("Call() error = %q; want it to carry the cancelled-context diagnosis, not the raw Acquire error", err.Error())
	}
}

func TestWorktreeTeardown_CancelledContext(t *testing.T) {
	scratchDir := t.TempDir()
	var released bool
	lock := fakePrimeLock("/lock/path", true, nil, nil, &released)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	rec := &teardownCallRecorder{}
	producer := NewWorktreeTeardown("teardown", "myslug", rec.deps(nil, "", nil), lock, scratchDir)

	outcome, _, err := producer.Call(ctx)
	if err == nil {
		t.Fatal("Call() error = nil; want a non-nil error for a cancelled context")
	}
	if outcome == shedengine.Stuck {
		t.Error("Call() outcome = Stuck; want a cancelled context to never surface as Stuck")
	}
}

// TestRecordAbandonedSession asserts the teardown row's abandoned-session record is written when a
// session was abandoned and cleared when a later teardown abandoned none, and that the producer's
// own Done path writes it, not only the helper in isolation.
//
//testtiming:keep pins the record's write, its clearing of a stale record and its write from the Done path, which the covering teardown tests never read
func TestRecordAbandonedSession(t *testing.T) {
	scratchDir := t.TempDir()
	path := AbandonedSessionFile(scratchDir)

	recordAbandonedSession("Worktree-Teardown", "some-slug", "lyx-some-slug", scratchDir)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read abandoned-session record: %v", err)
	}
	if got := strings.TrimSpace(string(data)); got != "lyx-some-slug" {
		t.Errorf("abandoned-session record = %q; want %q", got, "lyx-some-slug")
	}

	recordAbandonedSession("Worktree-Teardown", "some-slug", "", scratchDir)
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("os.Stat(%s) after a teardown that abandoned nothing = %v; want the stale record cleared", path, err)
	}

	// Clearing an already-absent record is a no-op, never a reported failure.
	recordAbandonedSession("Worktree-Teardown", "some-slug", "", scratchDir)

	// The producer's own Done path writes the record.
	producerScratchDir := t.TempDir()
	deps := TeardownDeps{
		Shutdown: func(ctx context.Context) (string, error) { return "lyx-abandoned", nil },
		Remove:   func(ctx context.Context) error { return nil },
	}
	var released bool
	producer := NewWorktreeTeardown("Worktree-Teardown", "some-slug", deps, fakePrimeLock("/lock/free/path", true, nil, nil, &released), producerScratchDir)

	shedfake.RequireOutcome(t, producer, shedengine.Done)
	data, err = os.ReadFile(AbandonedSessionFile(producerScratchDir))
	if err != nil {
		t.Fatalf("read abandoned-session record after a Done teardown: %v", err)
	}
	if got := strings.TrimSpace(string(data)); got != "lyx-abandoned" {
		t.Errorf("abandoned-session record = %q; want %q", got, "lyx-abandoned")
	}
}

// TestWorktreeTeardown_TearsDownInOrderOnceTheLockIsFree asserts the row runs Shutdown strictly
// before Remove and reports Done whether the prime lock is free at once or contended for a few
// polls first, sleeping once per contended poll, and releases the lock afterwards.
func TestWorktreeTeardown_TearsDownInOrderOnceTheLockIsFree(t *testing.T) {
	t.Parallel()

	for _, contendedPolls := range []int{0, 2} {
		t.Run(fmt.Sprintf("contended_polls=%d", contendedPolls), func(t *testing.T) {
			t.Parallel()

			scratchDir := t.TempDir()
			var released bool
			var sleeps int
			lock := waitingPrimeLock("/lock/path", contendedPolls, nil, &released, &sleeps, nil)
			rec := &teardownCallRecorder{}
			producer := NewWorktreeTeardown("teardown", "myslug", rec.deps(nil, "", nil), lock, scratchDir)

			shedfake.RequireOutcome(t, producer, shedengine.Done)
			if len(rec.calls) != 2 || rec.calls[0] != "shutdown" || rec.calls[1] != "remove" {
				t.Errorf("call order = %v; want [shutdown remove]", rec.calls)
			}
			if sleeps != contendedPolls {
				t.Errorf("sleeps = %d; want %d", sleeps, contendedPolls)
			}
			if !released {
				t.Error("release was not invoked on the Done path")
			}
		})
	}
}

func TestWorktreeTeardown_LockStillHeldPastBoundIsStuck(t *testing.T) {
	scratchDir := t.TempDir()
	var released bool
	var sleeps int
	lock := waitingPrimeLock("/lock/contended/path", -1, nil, &released, &sleeps, nil)
	rec := &teardownCallRecorder{}
	producer := NewWorktreeTeardown("teardown", "myslug", rec.deps(nil, "", nil), lock, scratchDir)

	ptr := shedfake.RequireOutcome(t, producer, shedengine.Stuck)
	if len(rec.calls) != 0 {
		t.Errorf("calls = %v; want neither Shutdown nor Remove", rec.calls)
	}
	reason := readStuckFile(t, scratchDir, "teardown", ptr)
	if !strings.Contains(reason, "/lock/contended/path") || !strings.Contains(reason, "after waiting 10m0s") {
		t.Errorf("stuck-reason file = %q; want it to name the lock path and the wait", reason)
	}
}

func TestWorktreeTeardown_CancelledDuringLockWait(t *testing.T) {
	scratchDir := t.TempDir()
	var released bool
	var sleeps int
	ctx, cancel := context.WithCancel(context.Background())
	lock := waitingPrimeLock("/lock/path", -1, nil, &released, &sleeps, cancel)
	rec := &teardownCallRecorder{}
	producer := NewWorktreeTeardown("teardown", "myslug", rec.deps(nil, "", nil), lock, scratchDir)

	outcome, _, err := producer.Call(ctx)
	if err == nil {
		t.Fatal("Call() error = nil; want the cancelled-during-run error")
	}
	if outcome == shedengine.Stuck {
		t.Error("Call() outcome = Stuck; want a cancelled wait to never surface as Stuck")
	}
	if !errors.Is(err, context.Canceled) || !strings.Contains(err.Error(), "context cancelled during run") {
		t.Errorf("Call() error = %v; want the cancelled-during-run diagnosis", err)
	}
	if len(rec.calls) != 0 {
		t.Errorf("calls = %v; want none after cancellation", rec.calls)
	}
}
