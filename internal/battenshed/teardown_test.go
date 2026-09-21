// teardown_test.go covers NewWorktreeTeardown over fake seams: the strict Shutdown-before-Remove
// ordering, which half a stuck reason names, the abandoned-session log path, and prime-lock
// dispositions.

package battenshed

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/shedengine"
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

func TestWorktreeTeardown_HappyPathOrdering(t *testing.T) {
	scratchDir := t.TempDir()
	var released bool
	lock := fakePrimeLock("/lock/path", true, nil, nil, &released)

	rec := &teardownCallRecorder{}
	producer := NewWorktreeTeardown("teardown", "myslug", rec.deps(nil, "", nil), lock, scratchDir)

	outcome, _, err := producer.Call(context.Background())
	if err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}
	if outcome != shedengine.Done {
		t.Errorf("Call() outcome = %v; want Done", outcome)
	}
	if len(rec.calls) != 2 || rec.calls[0] != "shutdown" || rec.calls[1] != "remove" {
		t.Errorf("call order = %v; want [shutdown remove]", rec.calls)
	}
	if !released {
		t.Error("release was not invoked on the Done path")
	}
}

func TestWorktreeTeardown_ShutdownFailureNeverCallsRemove(t *testing.T) {
	scratchDir := t.TempDir()
	var released bool
	lock := fakePrimeLock("/lock/path", true, nil, nil, &released)

	shutdownErr := errors.New("session would not end")
	rec := &teardownCallRecorder{}
	producer := NewWorktreeTeardown("teardown", "myslug", rec.deps(shutdownErr, "", nil), lock, scratchDir)

	outcome, _, err := producer.Call(context.Background())
	if err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}
	if outcome != shedengine.Stuck {
		t.Errorf("Call() outcome = %v; want Stuck", outcome)
	}
	if len(rec.calls) != 1 || rec.calls[0] != "shutdown" {
		t.Errorf("call order = %v; want [shutdown] only, Remove must never be called", rec.calls)
	}
	if !released {
		t.Error("release was not invoked on the Shutdown-error Stuck path")
	}
	reason := readStuckFile(t, scratchDir, "teardown")
	if !strings.Contains(reason, "session shutdown") {
		t.Errorf("stuck-reason file = %q; want it to name session shutdown as the failed half", reason)
	}
	if !strings.Contains(reason, shutdownErr.Error()) {
		t.Errorf("stuck-reason file = %q; want it to contain the underlying error", reason)
	}
}

func TestWorktreeTeardown_RemoveFailureNamesRemovalHalf(t *testing.T) {
	scratchDir := t.TempDir()
	var released bool
	lock := fakePrimeLock("/lock/path", true, nil, nil, &released)

	removeErr := errors.New("worktree remove refused: merge in progress")
	rec := &teardownCallRecorder{}
	producer := NewWorktreeTeardown("teardown", "myslug", rec.deps(nil, "", removeErr), lock, scratchDir)

	outcome, _, err := producer.Call(context.Background())
	if err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}
	if outcome != shedengine.Stuck {
		t.Errorf("Call() outcome = %v; want Stuck", outcome)
	}
	if len(rec.calls) != 2 {
		t.Errorf("call count = %d; want 2 (both Shutdown and Remove called)", len(rec.calls))
	}
	reason := readStuckFile(t, scratchDir, "teardown")
	if !strings.Contains(reason, "worktree removal") {
		t.Errorf("stuck-reason file = %q; want it to name worktree removal as the failed half", reason)
	}
	if !strings.Contains(reason, "shutdown") || !strings.Contains(reason, "already succeeded") {
		t.Errorf("stuck-reason file = %q; want it to state that shutdown already succeeded", reason)
	}
}

func TestWorktreeTeardown_RemoveRefusalMergeInProgress(t *testing.T) {
	scratchDir := t.TempDir()
	var released bool
	lock := fakePrimeLock("/lock/path", true, nil, nil, &released)

	removeErr := errors.New("refuse to remove: merge in progress")
	rec := &teardownCallRecorder{}
	producer := NewWorktreeTeardown("teardown", "myslug", rec.deps(nil, "", removeErr), lock, scratchDir)

	outcome, _, err := producer.Call(context.Background())
	if err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}
	if outcome != shedengine.Stuck {
		t.Errorf("Call() outcome = %v; want Stuck", outcome)
	}
	reason := readStuckFile(t, scratchDir, "teardown")
	if !strings.Contains(reason, "merge in progress") {
		t.Errorf("stuck-reason file = %q; want the merge-in-progress refusal text preserved", reason)
	}
}

func TestWorktreeTeardown_AbandonedSessionOnOtherwiseDoneRow(t *testing.T) {
	scratchDir := t.TempDir()
	var released bool
	lock := fakePrimeLock("/lock/path", true, nil, nil, &released)

	rec := &teardownCallRecorder{}
	producer := NewWorktreeTeardown("teardown", "myslug", rec.deps(nil, "abandoned-session-1", nil), lock, scratchDir)

	outcome, _, err := producer.Call(context.Background())
	if err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}
	if outcome != shedengine.Done {
		t.Errorf("Call() outcome = %v; want Done even with a non-empty abandoned session", outcome)
	}
}

func TestWorktreeTeardown_PrimeLockDispositions(t *testing.T) {
	t.Run("contention", func(t *testing.T) {
		scratchDir := t.TempDir()
		var released bool
		lock := fakePrimeLock("/lock/contended/path", false, nil, nil, &released)
		rec := &teardownCallRecorder{}
		producer := NewWorktreeTeardown("teardown", "myslug", rec.deps(nil, "", nil), lock, scratchDir)

		outcome, _, err := producer.Call(context.Background())
		if err != nil {
			t.Fatalf("Call() error = %v; want nil", err)
		}
		if outcome != shedengine.Stuck {
			t.Errorf("Call() outcome = %v; want Stuck", outcome)
		}
		if len(rec.calls) != 0 {
			t.Errorf("call count = %d; want 0 under lock contention", len(rec.calls))
		}
		reason := readStuckFile(t, scratchDir, "teardown")
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

		if _, _, err := producer.Call(context.Background()); err != nil {
			t.Fatalf("Call() error = %v; want nil", err)
		}
		if !released {
			t.Error("release was not invoked on the Remove-error Stuck path")
		}
	})
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
// session was abandoned and cleared when a later teardown abandoned none.
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
}

// TestTeardown_DoneRunRecordsTheAbandonedSession asserts the record is written from the producer's
// own Done path, not only by the helper in isolation.
func TestTeardown_DoneRunRecordsTheAbandonedSession(t *testing.T) {
	scratchDir := t.TempDir()
	deps := TeardownDeps{
		Shutdown: func(ctx context.Context) (string, error) { return "lyx-abandoned", nil },
		Remove:   func(ctx context.Context) error { return nil },
	}
	var released bool
	producer := NewWorktreeTeardown("Worktree-Teardown", "some-slug", deps, fakePrimeLock("/lock/free/path", true, nil, nil, &released), scratchDir)

	outcome, _, err := producer.Call(context.Background())
	if err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}
	if outcome != shedengine.Done {
		t.Fatalf("Call() outcome = %v; want Done", outcome)
	}
	data, err := os.ReadFile(AbandonedSessionFile(scratchDir))
	if err != nil {
		t.Fatalf("read abandoned-session record after a Done teardown: %v", err)
	}
	if got := strings.TrimSpace(string(data)); got != "lyx-abandoned" {
		t.Errorf("abandoned-session record = %q; want %q", got, "lyx-abandoned")
	}
}
