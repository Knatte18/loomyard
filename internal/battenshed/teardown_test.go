// teardown_test.go covers NewWorktreeTeardown over fake seams: the strict Shutdown-before-Remove
// ordering, which half a stuck reason names, the abandoned-session log path, and prime-lock
// dispositions.

package battenshed

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/testkit/shedfake"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// teardownCallRecorder records the order the teardown's seams were called in.
// Its Push returns pushErr, and its Level reads false levelFalseReads times, then true, unless levelErr fails it.
type teardownCallRecorder struct {
	calls           []string
	pushErr         error
	levelErr        error
	levelFalseReads int
}

func (r *teardownCallRecorder) deps(shutdownErr error, abandonedSession string, removeErr error) TeardownDeps {
	return TeardownDeps{
		Shutdown: func(ctx context.Context) (string, error) {
			r.calls = append(r.calls, "shutdown")
			return abandonedSession, shutdownErr
		},
		Push: func(ctx context.Context) error {
			r.calls = append(r.calls, "push")
			return r.pushErr
		},
		Level: func() (bool, error) {
			r.calls = append(r.calls, "level")
			if r.levelErr != nil {
				return false, r.levelErr
			}
			if r.levelFalseReads > 0 {
				r.levelFalseReads--
				return false, nil
			}
			return true, nil
		},
		Remove: func(ctx context.Context) error {
			r.calls = append(r.calls, "remove")
			return removeErr
		},
	}
}

// recordingPrimeLock builds a free PrimeLock that notes each take as "lock" and each release as "release" in rec's calls.
func recordingPrimeLock(rec *teardownCallRecorder) PrimeLock {
	return PrimeLock{
		Path: "/lock/path",
		Acquire: func() (func() error, bool, error) {
			rec.calls = append(rec.calls, "lock")
			return func() error {
				rec.calls = append(rec.calls, "release")
				return nil
			}, true, nil
		},
		Sleep: func(context.Context, time.Duration) {},
	}
}

// TestWorktreeTeardown_ShutdownFailureNeverCallsRemove covers each way the row stops before the removal.
// A failed shutdown, a refused push, a handed-over push and a failed level read are Stuck and never call Remove.
// A push that reports the entry ended removes the pair without reading Level.
func TestWorktreeTeardown_ShutdownFailureNeverCallsRemove(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	shutdownErr := errors.New("session would not end")
	pushErr := errors.New("push refused: way forward: fix the credentials")
	levelErr := errors.New("tracking ref unreadable")
	tests := []struct {
		name        string
		shutdownErr error
		pushErr     error
		levelErr    error
		wantOutcome shedengine.Outcome
		wantCalls   []string
		wantReason  []string
	}{
		{
			name: "a failed shutdown", shutdownErr: shutdownErr, wantOutcome: shedengine.Stuck,
			wantCalls:  []string{"shutdown"},
			wantReason: []string{"session shutdown", shutdownErr.Error()},
		},
		{
			name: "a refused push", pushErr: pushErr, wantOutcome: shedengine.Stuck,
			wantCalls:  []string{"shutdown", "push"},
			wantReason: []string{pushErr.Error(), "lym batten run myslug"},
		},
		{
			name: "a handed-over push", pushErr: fmt.Errorf("%w: machine oslo holds the run", ErrPushHandedOver), wantOutcome: shedengine.Stuck,
			wantCalls:  []string{"shutdown", "push"},
			wantReason: []string{"handed over", "machine oslo holds the run"},
		},
		{
			name: "a failed level read", levelErr: levelErr, wantOutcome: shedengine.Stuck,
			wantCalls:  []string{"shutdown", "push", "level"},
			wantReason: []string{levelErr.Error(), "lym batten run myslug"},
		},
		{
			name: "an ended entry removes without reading level", pushErr: fmt.Errorf("%w: branch gone", ErrPushEntryEnded), wantOutcome: shedengine.Done,
			wantCalls: []string{"shutdown", "push", "remove"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			scratchDir := t.TempDir()
			var released bool
			lock := fakePrimeLock("/lock/path", true, nil, nil, &released)
			rec := &teardownCallRecorder{pushErr: tt.pushErr, levelErr: tt.levelErr}
			producer := NewWorktreeTeardown("teardown", "myslug", rec.deps(tt.shutdownErr, "", nil), lock, scratchDir)

			ptr := shedfake.RequireOutcome(t, producer, tt.wantOutcome)

			if !slices.Equal(rec.calls, tt.wantCalls) {
				t.Errorf("call order = %v; want %v", rec.calls, tt.wantCalls)
			}
			if !released {
				t.Error("release was not invoked")
			}
			if tt.wantOutcome != shedengine.Stuck {
				return
			}
			reason := readStuckFile(t, scratchDir, "teardown", ptr)
			for _, want := range tt.wantReason {
				if !strings.Contains(reason, want) {
					t.Errorf("stuck-reason file = %q; want it to contain %q", reason, want)
				}
			}
		})
	}
}

//testtiming:keep pins the removal half and the shutdown-already-succeeded statement in the stuck reason, which the lock-disposition test never asserts
func TestWorktreeTeardown_RemoveFailureNamesRemovalHalf(t *testing.T) {
	tiergate.Require(t, 0)
	scratchDir := t.TempDir()
	var released bool
	lock := fakePrimeLock("/lock/path", true, nil, nil, &released)

	removeErr := errors.New("worktree remove refused: merge in progress")
	rec := &teardownCallRecorder{}
	producer := NewWorktreeTeardown("teardown", "myslug", rec.deps(nil, "", removeErr), lock, scratchDir)

	ptr := shedfake.RequireOutcome(t, producer, shedengine.Stuck)
	if want := []string{"shutdown", "push", "level", "remove"}; !slices.Equal(rec.calls, want) {
		t.Errorf("call order = %v; want %v", rec.calls, want)
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

// TestWorktreeTeardown_LevelThatNeverReadsTrueIsStuckAfterTheBound asserts the row stops pushing and keeps the pair.
// A Level that reads false after every push yields Stuck after three pushes, naming the resume, and never calls Remove.
func TestWorktreeTeardown_LevelThatNeverReadsTrueIsStuckAfterTheBound(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	scratchDir := t.TempDir()
	rec := &teardownCallRecorder{levelFalseReads: 100}
	producer := NewWorktreeTeardown("teardown", "myslug", rec.deps(nil, "", nil), recordingPrimeLock(rec), scratchDir)

	ptr := shedfake.RequireOutcome(t, producer, shedengine.Stuck)

	pushes := 0
	for _, call := range rec.calls {
		switch call {
		case "push":
			pushes++
		case "remove":
			t.Errorf("call order = %v; want no remove while origin lacks a tip", rec.calls)
		}
	}
	if pushes != 3 {
		t.Errorf("pushes = %d; want 3", pushes)
	}
	reason := readStuckFile(t, scratchDir, "teardown", ptr)
	for _, want := range []string{"origin still lacks a tip", "lym batten run myslug"} {
		if !strings.Contains(reason, want) {
			t.Errorf("stuck-reason file = %q; want it to contain %q", reason, want)
		}
	}
}

func TestWorktreeTeardown_PrimeLockDispositions(t *testing.T) {
	tiergate.Require(t, 0)
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

	t.Run("a stuck push released the first hold and takes no second", func(t *testing.T) {
		scratchDir := t.TempDir()
		rec := &teardownCallRecorder{pushErr: errors.New("push refused")}
		producer := NewWorktreeTeardown("teardown", "myslug", rec.deps(nil, "", nil), recordingPrimeLock(rec), scratchDir)

		shedfake.RequireOutcome(t, producer, shedengine.Stuck)
		if want := []string{"lock", "shutdown", "release", "push"}; !slices.Equal(rec.calls, want) {
			t.Errorf("call order = %v; want %v", rec.calls, want)
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

// TestWorktreeTeardown_CancelledDuringAcquireError covers the Acquire-error hard-error path.
// That path must consult cancelErr before it returns.
func TestWorktreeTeardown_CancelledDuringAcquireError(t *testing.T) {
	tiergate.Require(t, 0)
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
	tiergate.Require(t, 0)
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

// TestRecordAbandonedSession asserts the abandoned-session record is written when a session was abandoned.
// It also asserts that a teardown abandoning none clears the record, and that the Done path writes it.
//
//testtiming:keep pins the record's write, its clearing of a stale record and its write from the Done path, which the covering teardown tests never read
func TestRecordAbandonedSession(t *testing.T) {
	tiergate.Require(t, 0)
	scratchDir := t.TempDir()
	path := AbandonedSessionFile(scratchDir)

	recordAbandonedSession("Worktree-Teardown", "some-slug", "lym-some-slug", scratchDir)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read abandoned-session record: %v", err)
	}
	if got := strings.TrimSpace(string(data)); got != "lym-some-slug" {
		t.Errorf("abandoned-session record = %q; want %q", got, "lym-some-slug")
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
		Shutdown: func(ctx context.Context) (string, error) { return "lym-abandoned", nil },
		Remove:   func(ctx context.Context) error { return nil },
	}
	var released bool
	producer := NewWorktreeTeardown("Worktree-Teardown", "some-slug", deps, fakePrimeLock("/lock/free/path", true, nil, nil, &released), producerScratchDir)

	shedfake.RequireOutcome(t, producer, shedengine.Done)
	data, err = os.ReadFile(AbandonedSessionFile(producerScratchDir))
	if err != nil {
		t.Fatalf("read abandoned-session record after a Done teardown: %v", err)
	}
	if got := strings.TrimSpace(string(data)); got != "lym-abandoned" {
		t.Errorf("abandoned-session record = %q; want %q", got, "lym-abandoned")
	}
}

// TestWorktreeTeardown_TearsDownInOrderOnceTheLockIsFree asserts the row's order over its two prime-lock holds:
// lock, shutdown, release, push, lock, level, remove, release.
// A Level that reads false once releases the second hold and pushes again, so the row records two pushes and one remove.
// It also asserts that the row reports Done whether the prime lock is free at once or contended for a few polls,
// and that it sleeps once per contended poll and releases the lock afterwards.
func TestWorktreeTeardown_TearsDownInOrderOnceTheLockIsFree(t *testing.T) {
	tiergate.Require(t, 0)
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
			if want := []string{"shutdown", "push", "level", "remove"}; !slices.Equal(rec.calls, want) {
				t.Errorf("call order = %v; want %v", rec.calls, want)
			}
			if sleeps != contendedPolls {
				t.Errorf("sleeps = %d; want %d", sleeps, contendedPolls)
			}
			if !released {
				t.Error("release was not invoked on the Done path")
			}
		})
	}

	holds := []struct {
		name            string
		levelFalseReads int
		wantCalls       []string
	}{
		{"LevelOnTheFirstRead", 0, []string{"lock", "shutdown", "release", "push", "lock", "level", "remove", "release"}},
		{"LevelFalseOnce", 1, []string{
			"lock", "shutdown", "release",
			"push", "lock", "level", "release",
			"push", "lock", "level", "remove", "release",
		}},
	}
	for _, tt := range holds {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rec := &teardownCallRecorder{levelFalseReads: tt.levelFalseReads}
			producer := NewWorktreeTeardown("teardown", "myslug", rec.deps(nil, "", nil), recordingPrimeLock(rec), t.TempDir())

			shedfake.RequireOutcome(t, producer, shedengine.Done)
			if !slices.Equal(rec.calls, tt.wantCalls) {
				t.Errorf("call order = %v; want %v", rec.calls, tt.wantCalls)
			}
		})
	}
}

func TestWorktreeTeardown_LockStillHeldPastBoundIsStuck(t *testing.T) {
	tiergate.Require(t, 0)
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
	tiergate.Require(t, 0)
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
