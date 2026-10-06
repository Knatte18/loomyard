// create_test.go covers NewWorktreeCreate over fake seams: the happy path, the prime-lock
// dispositions, and createWorktree's error mappings, including fabric's own verbatim refusal text.

package battenshed

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/testkit/shedfake"
)

// fakePrimeLock builds a PrimeLock whose Acquire reports the told disposition and records whether
// its release closure was invoked.
func fakePrimeLock(path string, ok bool, acquireErr error, releaseErr error, released *bool) PrimeLock {
	return PrimeLock{
		Path: path,
		Acquire: func() (func() error, bool, error) {
			if acquireErr != nil {
				return nil, false, acquireErr
			}
			if !ok {
				return nil, false, nil
			}
			return func() error {
				*released = true
				return releaseErr
			}, true, nil
		},
		Sleep: func(context.Context, time.Duration) {},
	}
}

// waitingPrimeLock builds a PrimeLock that reports contention for the first contendedPolls Acquire calls (forever when contendedPolls is negative), then acquires.
// laterErr, when non-nil, is returned as the Acquire error on the attempt after the first contended one.
// sleeps counts Sleep calls, and onSleep, when non-nil, runs inside each of them.
func waitingPrimeLock(path string, contendedPolls int, laterErr error, released *bool, sleeps *int, onSleep func()) PrimeLock {
	attempts := 0
	return PrimeLock{
		Path: path,
		Acquire: func() (func() error, bool, error) {
			attempts++
			if laterErr != nil && attempts > 1 {
				return nil, false, laterErr
			}
			if contendedPolls < 0 || attempts <= contendedPolls {
				return nil, false, nil
			}
			return func() error {
				*released = true
				return nil
			}, true, nil
		},
		Sleep: func(context.Context, time.Duration) {
			*sleeps++
			if onSleep != nil {
				onSleep()
			}
		},
	}
}

func readStuckFile(t *testing.T, scratchDir, producer string, ptr shedengine.OutputPointer) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(scratchDir, producer+stuckFileSuffix))
	if err != nil {
		t.Fatalf("read stuck file: %v", err)
	}
	if ptr.Reason == "" || ptr.Reason+"\n" != string(data) {
		t.Errorf("returned Reason = %q; want the reason file's line %q", ptr.Reason, strings.TrimSuffix(string(data), "\n"))
	}
	return string(data)
}

// TestWorktreeCreate_CreateErrorsStickWithTheirTextVerbatim asserts every createWorktree error parks the row Stuck with the error's own text in the reason file, unreworded -- fabric's pre-existing-branch remedy wording and its bare dirty-worktree string included -- and releases the prime lock.
func TestWorktreeCreate_CreateErrorsStickWithTheirTextVerbatim(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		err        error
		wantReason string
	}{
		{
			name:       "UnderlyingFailure",
			err:        errors.New("some underlying failure"),
			wantReason: "some underlying failure",
		},
		{
			name:       "PreExistingBranchRemedySurvivesVerbatim",
			err:        errors.New(`branch "task-slug" already exists; switch a pair onto it with "lyx fabric checkout task-slug", or delete it first with "git branch -D task-slug" if it is a leftover from a removed pair`),
			wantReason: "switch a pair onto it with",
		},
		{
			name:       "DirtyDrivingWorktreePassesThrough",
			err:        errors.New("source worktree has uncommitted changes"),
			wantReason: "source worktree has uncommitted changes",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			scratchDir := t.TempDir()
			var released bool
			lock := fakePrimeLock("/lock/path", true, nil, nil, &released)

			producer := NewWorktreeCreate("create", "myslug", func(ctx context.Context) error {
				return tt.err
			}, lock, scratchDir)

			ptr := shedfake.RequireOutcome(t, producer, shedengine.Stuck)
			if !released {
				t.Error("release was not invoked on the createWorktree-error Stuck path")
			}
			reason := readStuckFile(t, scratchDir, "create", ptr)
			if !strings.Contains(reason, tt.wantReason) {
				t.Errorf("stuck-reason file = %q; want it to contain %q verbatim", reason, tt.wantReason)
			}
		})
	}
}

func TestWorktreeCreate_AcquireError(t *testing.T) {
	scratchDir := t.TempDir()
	var released bool
	acquireErr := errors.New("flock: device error")
	lock := fakePrimeLock("/lock/path", false, acquireErr, nil, &released)

	producer := NewWorktreeCreate("create", "myslug", func(ctx context.Context) error {
		return nil
	}, lock, scratchDir)

	_, _, err := producer.Call(context.Background())
	if err == nil {
		t.Fatal("Call() error = nil; want a returned error")
	}
	if !errors.Is(err, acquireErr) {
		t.Errorf("Call() error = %v; want it to wrap %v", err, acquireErr)
	}
}

func TestWorktreeCreate_CancelledContext(t *testing.T) {
	scratchDir := t.TempDir()
	var released bool
	lock := fakePrimeLock("/lock/path", true, nil, nil, &released)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	producer := NewWorktreeCreate("create", "myslug", func(ctx context.Context) error {
		return nil
	}, lock, scratchDir)

	outcome, _, err := producer.Call(ctx)
	if err == nil {
		t.Fatal("Call() error = nil; want a non-nil error for a cancelled context")
	}
	if outcome == shedengine.Stuck {
		t.Error("Call() outcome = Stuck; want a cancelled context to never surface as Stuck")
	}
}

// TestWorktreeCreate_CancelledDuringAcquireError asserts the Acquire-error hard-error path
// consults cancelErr before returning: when ctx is cancelled by the time Acquire itself returns
// its own error, Call must report the "context cancelled during run" diagnosis, not the raw
// Acquire error text -- the property F1 (crucible round sonnet-xhigh-r3) found missing here.
func TestWorktreeCreate_CancelledDuringAcquireError(t *testing.T) {
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

	producer := NewWorktreeCreate("create", "myslug", func(ctx context.Context) error {
		return nil
	}, lock, scratchDir)

	_, _, err := producer.Call(ctx)
	if err == nil {
		t.Fatal("Call() error = nil; want the cancelled-context diagnosis")
	}
	if !strings.Contains(err.Error(), "context cancelled during run") {
		t.Errorf("Call() error = %q; want it to carry the cancelled-context diagnosis, not the raw Acquire error", err.Error())
	}
}

func TestWorktreeCreate_CancelledAfterSuccessfulCreate(t *testing.T) {
	scratchDir := t.TempDir()
	var released bool
	lock := fakePrimeLock("/lock/path", true, nil, nil, &released)

	ctx, cancel := context.WithCancel(context.Background())
	producer := NewWorktreeCreate("create", "myslug", func(ctx context.Context) error {
		cancel()
		return nil
	}, lock, scratchDir)

	outcome, _, err := producer.Call(ctx)
	if err == nil {
		t.Fatal("Call() error = nil; want a non-nil error once ctx is cancelled after a successful create")
	}
	if outcome == shedengine.Stuck {
		t.Error("Call() outcome = Stuck; want a cancelled context to never surface as Stuck")
	}
	if !released {
		t.Error("release was not invoked on the cancelled-context path")
	}
}

// TestWorktreeCreate_CreatesOnceTheLockIsFree asserts the row creates the worktree and reports Done whether the prime lock is free at once or contended for a few polls first, sleeping once per contended poll, and releases the lock afterwards.
func TestWorktreeCreate_CreatesOnceTheLockIsFree(t *testing.T) {
	t.Parallel()

	for _, contendedPolls := range []int{0, 3} {
		t.Run(fmt.Sprintf("contended_polls=%d", contendedPolls), func(t *testing.T) {
			t.Parallel()

			scratchDir := t.TempDir()
			var released bool
			var sleeps int
			lock := waitingPrimeLock("/lock/path", contendedPolls, nil, &released, &sleeps, nil)

			called := false
			producer := NewWorktreeCreate("create", "myslug", func(ctx context.Context) error {
				called = true
				return nil
			}, lock, scratchDir)

			shedfake.RequireOutcome(t, producer, shedengine.Done)
			if !called {
				t.Error("createWorktree was not called after the lock freed")
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

func TestWorktreeCreate_LockStillHeldPastBoundIsStuck(t *testing.T) {
	scratchDir := t.TempDir()
	var released bool
	var sleeps int
	lock := waitingPrimeLock("/lock/contended/path", -1, nil, &released, &sleeps, nil)

	called := false
	producer := NewWorktreeCreate("create", "myslug", func(ctx context.Context) error {
		called = true
		return nil
	}, lock, scratchDir)

	ptr := shedfake.RequireOutcome(t, producer, shedengine.Stuck)
	if called {
		t.Error("createWorktree was called although the lock was never acquired")
	}
	if want := int(primeLockWaitBound / primeLockPollInterval); sleeps != want {
		t.Errorf("sleeps = %d; want %d", sleeps, want)
	}
	reason := readStuckFile(t, scratchDir, "create", ptr)
	if !strings.Contains(reason, "/lock/contended/path") || !strings.Contains(reason, "after waiting 10m0s") {
		t.Errorf("stuck-reason file = %q; want it to name the lock path and the wait", reason)
	}
}

func TestWorktreeCreate_CancelledDuringLockWait(t *testing.T) {
	scratchDir := t.TempDir()
	var released bool
	var sleeps int
	ctx, cancel := context.WithCancel(context.Background())
	lock := waitingPrimeLock("/lock/path", -1, nil, &released, &sleeps, cancel)

	called := false
	producer := NewWorktreeCreate("create", "myslug", func(ctx context.Context) error {
		called = true
		return nil
	}, lock, scratchDir)

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
	if called {
		t.Error("createWorktree was called after cancellation")
	}
}

func TestWorktreeCreate_AcquireErrorOnLaterAttempt(t *testing.T) {
	scratchDir := t.TempDir()
	var released bool
	var sleeps int
	acquireErr := errors.New("flock: device error")
	lock := waitingPrimeLock("/lock/path", -1, acquireErr, &released, &sleeps, nil)

	producer := NewWorktreeCreate("create", "myslug", func(ctx context.Context) error {
		return nil
	}, lock, scratchDir)

	outcome, _, err := producer.Call(context.Background())
	if err == nil || !errors.Is(err, acquireErr) {
		t.Fatalf("Call() error = %v; want it to wrap %v", err, acquireErr)
	}
	if outcome == shedengine.Stuck {
		t.Error("Call() outcome = Stuck; want a hard error")
	}
	if sleeps != 1 {
		t.Errorf("sleeps = %d; want 1", sleeps)
	}
}
