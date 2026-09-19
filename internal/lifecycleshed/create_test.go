// create_test.go covers NewWorktreeCreate over fake seams: the happy path, the prime-lock
// dispositions, and createWorktree's error mappings, including fabric's own verbatim refusal text.

package lifecycleshed

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/shedengine"
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
	}
}

func readStuckFile(t *testing.T, scratchDir, producer string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(scratchDir, producer+stuckFileSuffix))
	if err != nil {
		t.Fatalf("read stuck file: %v", err)
	}
	return string(data)
}

func TestWorktreeCreate_HappyPath(t *testing.T) {
	scratchDir := t.TempDir()
	var released bool
	lock := fakePrimeLock("/lock/path", true, nil, nil, &released)

	called := false
	producer := NewWorktreeCreate("create", "myslug", func(ctx context.Context) error {
		called = true
		return nil
	}, lock, scratchDir)

	outcome, _, err := producer.Call(context.Background())
	if err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}
	if outcome != shedengine.Done {
		t.Errorf("Call() outcome = %v; want Done", outcome)
	}
	if !called {
		t.Error("createWorktree was not called")
	}
	if !released {
		t.Error("release was not invoked on the Done path")
	}
}

func TestWorktreeCreate_CreateWorktreeError(t *testing.T) {
	scratchDir := t.TempDir()
	var released bool
	lock := fakePrimeLock("/lock/path", true, nil, nil, &released)

	wantErr := errors.New("some underlying failure")
	producer := NewWorktreeCreate("create", "myslug", func(ctx context.Context) error {
		return wantErr
	}, lock, scratchDir)

	outcome, _, err := producer.Call(context.Background())
	if err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}
	if outcome != shedengine.Stuck {
		t.Errorf("Call() outcome = %v; want Stuck", outcome)
	}
	if !released {
		t.Error("release was not invoked on the createWorktree-error Stuck path")
	}
	reason := readStuckFile(t, scratchDir, "create")
	if !strings.Contains(reason, wantErr.Error()) {
		t.Errorf("stuck-reason file = %q; want it to contain %q verbatim", reason, wantErr.Error())
	}
}

func TestWorktreeCreate_PreExistingBranchRemedySurvivesVerbatim(t *testing.T) {
	scratchDir := t.TempDir()
	var released bool
	lock := fakePrimeLock("/lock/path", true, nil, nil, &released)

	fabricErr := errors.New(`branch "task-slug" already exists; switch a pair onto it with "lyx fabric checkout task-slug", or delete it first with "git branch -D task-slug" if it is a leftover from a removed pair`)
	producer := NewWorktreeCreate("create", "myslug", func(ctx context.Context) error {
		return fabricErr
	}, lock, scratchDir)

	outcome, _, err := producer.Call(context.Background())
	if err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}
	if outcome != shedengine.Stuck {
		t.Errorf("Call() outcome = %v; want Stuck", outcome)
	}
	reason := readStuckFile(t, scratchDir, "create")
	if !strings.Contains(reason, "switch a pair onto it with") {
		t.Errorf("stuck-reason file = %q; want fabric's own remedy wording preserved unreworded", reason)
	}
}

func TestWorktreeCreate_DirtyDrivingWorktreePassesThroughVerbatim(t *testing.T) {
	scratchDir := t.TempDir()
	var released bool
	lock := fakePrimeLock("/lock/path", true, nil, nil, &released)

	dirtyErr := errors.New("source worktree has uncommitted changes")
	producer := NewWorktreeCreate("create", "myslug", func(ctx context.Context) error {
		return dirtyErr
	}, lock, scratchDir)

	outcome, _, err := producer.Call(context.Background())
	if err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}
	if outcome != shedengine.Stuck {
		t.Errorf("Call() outcome = %v; want Stuck", outcome)
	}
	reason := readStuckFile(t, scratchDir, "create")
	if !strings.Contains(reason, "source worktree has uncommitted changes") {
		t.Errorf("stuck-reason file = %q; want the bare dirty-worktree string preserved verbatim", reason)
	}
}

func TestWorktreeCreate_PrimeLockUnavailable(t *testing.T) {
	scratchDir := t.TempDir()
	var released bool
	lock := fakePrimeLock("/lock/contended/path", false, nil, nil, &released)

	called := false
	producer := NewWorktreeCreate("create", "myslug", func(ctx context.Context) error {
		called = true
		return nil
	}, lock, scratchDir)

	outcome, _, err := producer.Call(context.Background())
	if err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}
	if outcome != shedengine.Stuck {
		t.Errorf("Call() outcome = %v; want Stuck", outcome)
	}
	if called {
		t.Error("createWorktree was called despite lock contention")
	}
	reason := readStuckFile(t, scratchDir, "create")
	if !strings.Contains(reason, "/lock/contended/path") {
		t.Errorf("stuck-reason file = %q; want it to name the prime lock path", reason)
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
