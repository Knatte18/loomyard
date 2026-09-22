// seamchild_test.go covers NewSeedChild's full verdict table, keeping the two-source split visible
// -- recipe from the Board's own type, driver from prime's own seed -- and keeping the
// failed-commit and failed-push arms distinct, since collapsing either pairing is the likeliest
// regression.

package battenshed

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/shedengine"
)

// seedChildCalls records what each closure of a SeedChildDeps received, so a test can assert the
// two-source split (recipe from the Board, driver from ChildDriver) rather than trusting a single
// combined value.
type seedChildCalls struct {
	writeRecipe  string
	writeDriver  string
	writeCalled  bool
	commitCalled bool
	pushCalled   bool
}

func newSeedChildDeps(boardType string, boardErr error, driver string, driverErr error, writeErr error, commitErr error, pushErr error) (*seedChildCalls, SeedChildDeps) {
	calls := &seedChildCalls{}
	deps := SeedChildDeps{
		ReadBoardType: func(ctx context.Context) (string, error) {
			return boardType, boardErr
		},
		ChildDriver: func() (string, error) {
			return driver, driverErr
		},
		WriteSeed: func(ctx context.Context, recipe, drv string) error {
			calls.writeCalled = true
			calls.writeRecipe = recipe
			calls.writeDriver = drv
			return writeErr
		},
		CommitSeed: func(ctx context.Context) error {
			calls.commitCalled = true
			return commitErr
		},
		PushSeed: func(ctx context.Context) error {
			calls.pushCalled = true
			return pushErr
		},
	}
	return calls, deps
}

// TestSeedChild_TwoSourceSplit is the load-bearing assertion the two-source split depends on: the
// seed's recipe comes from the Board's own type, and its driver comes from the injected
// ChildDriver, never from one collapsed source.
func TestSeedChild_TwoSourceSplit(t *testing.T) {
	scratchDir := t.TempDir()
	calls, deps := newSeedChildDeps("batten", nil, "claude", nil, nil, nil, nil)

	producer := NewSeedChild("seedchild", "myslug", deps, scratchDir)
	outcome, _, err := producer.Call(context.Background())
	if err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}
	if outcome != shedengine.Done {
		t.Fatalf("Call() outcome = %v; want Done", outcome)
	}
	if calls.writeRecipe != "batten" {
		t.Errorf("WriteSeed recipe = %q; want %q (the Board's own type)", calls.writeRecipe, "batten")
	}
	if calls.writeDriver != "claude" {
		t.Errorf("WriteSeed driver = %q; want %q (the injected ChildDriver value)", calls.writeDriver, "claude")
	}
	if !calls.commitCalled {
		t.Error("CommitSeed was not called")
	}
	if !calls.pushCalled {
		t.Error("PushSeed was not called")
	}
}

// TestSeedChild_BoardTypeReadFreshAtCallTime proves the Board's own type is read at Call time, not
// captured at wiring time: a Board type that changes between wiring and this Call must still be
// honoured.
func TestSeedChild_BoardTypeReadFreshAtCallTime(t *testing.T) {
	scratchDir := t.TempDir()
	currentType := "loom"
	deps := SeedChildDeps{
		ReadBoardType: func(ctx context.Context) (string, error) {
			return currentType, nil
		},
		ChildDriver: func() (string, error) { return "claude", nil },
		WriteSeed: func(ctx context.Context, recipe, driver string) error {
			if recipe != "batten" {
				t.Errorf("WriteSeed recipe = %q; want %q, the type set after wiring", recipe, "batten")
			}
			return nil
		},
		CommitSeed: func(ctx context.Context) error { return nil },
		PushSeed:   func(ctx context.Context) error { return nil },
	}

	producer := NewSeedChild("seedchild", "myslug", deps, scratchDir)
	// Simulate the Board's type being corrected after prime was seeded but before this Call.
	currentType = "batten"

	outcome, _, err := producer.Call(context.Background())
	if err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}
	if outcome != shedengine.Done {
		t.Fatalf("Call() outcome = %v; want Done", outcome)
	}
}

func TestSeedChild_EmptyBoardTypeDefaultsToLoom(t *testing.T) {
	scratchDir := t.TempDir()
	calls, deps := newSeedChildDeps("", nil, "claude", nil, nil, nil, nil)

	producer := NewSeedChild("seedchild", "myslug", deps, scratchDir)
	outcome, _, err := producer.Call(context.Background())
	if err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}
	if outcome != shedengine.Done {
		t.Fatalf("Call() outcome = %v; want Done", outcome)
	}
	if calls.writeRecipe != defaultChildRecipe {
		t.Errorf("WriteSeed recipe = %q; want %q", calls.writeRecipe, defaultChildRecipe)
	}
}

func TestSeedChild_UnreadableBoardIsStuck(t *testing.T) {
	scratchDir := t.TempDir()
	boardErr := errors.New("board decode failed")
	_, deps := newSeedChildDeps("", boardErr, "claude", nil, nil, nil, nil)

	producer := NewSeedChild("seedchild", "myslug", deps, scratchDir)
	outcome, _, err := producer.Call(context.Background())
	if err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}
	if outcome != shedengine.Stuck {
		t.Fatalf("Call() outcome = %v; want Stuck", outcome)
	}
	reason := readStuckFile(t, scratchDir, "seedchild")
	if !strings.Contains(reason, "Board") {
		t.Errorf("stuck-reason file = %q; want it to name the Board read failure", reason)
	}
}

func TestSeedChild_UnknownRecipeNameIsStuck(t *testing.T) {
	scratchDir := t.TempDir()
	writeErr := fmt.Errorf("recipe %q: %w", "bogus", ErrUnknownRecipe)
	_, deps := newSeedChildDeps("bogus", nil, "claude", nil, writeErr, nil, nil)

	producer := NewSeedChild("seedchild", "myslug", deps, scratchDir)
	outcome, _, err := producer.Call(context.Background())
	if err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}
	if outcome != shedengine.Stuck {
		t.Fatalf("Call() outcome = %v; want Stuck", outcome)
	}
	reason := readStuckFile(t, scratchDir, "seedchild")
	if !strings.Contains(reason, "unknown recipe") {
		t.Errorf("stuck-reason file = %q; want it to name the unknown recipe", reason)
	}
}

// TestSeedChild_UnsupportedChildRecipeIsStuck pins the second refusal a WriteSeed seam may raise:
// a registered recipe the task worktree cannot bootstrap lands Stuck, naming the Board type, with
// no commit and no push attempted.
func TestSeedChild_UnsupportedChildRecipeIsStuck(t *testing.T) {
	scratchDir := t.TempDir()
	writeErr := fmt.Errorf("%w: only loom can", ErrUnsupportedChildRecipe)
	calls, deps := newSeedChildDeps("batten", nil, "go", nil, writeErr, nil, nil)

	producer := NewSeedChild("seedchild", "myslug", deps, scratchDir)
	outcome, _, err := producer.Call(context.Background())
	if err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}
	if outcome != shedengine.Stuck {
		t.Fatalf("Call() outcome = %v; want Stuck", outcome)
	}
	if calls.commitCalled || calls.pushCalled {
		t.Errorf("commitCalled=%v pushCalled=%v; want neither -- nothing was written to commit", calls.commitCalled, calls.pushCalled)
	}
	reason := readStuckFile(t, scratchDir, "seedchild")
	if !strings.Contains(reason, `Board task type "batten"`) {
		t.Errorf("stuck-reason file = %q; want it to name the Board task type", reason)
	}
}

// TestSeedChild_DisagreeingChildSeedIsStuck pins the third refusal a WriteSeed seam may raise: a
// pre-existing child seed that disagrees with the one being written lands Stuck, not a hard error --
// the same business-judgment treatment as the other two WriteSeed refusals, with no commit and no
// push attempted. Live-reproduced (batten review sonnet-xhigh-r5, finding F3): before this fix, this
// exact case surfaced as a hard StateFailed with no stuck_reason.
func TestSeedChild_DisagreeingChildSeedIsStuck(t *testing.T) {
	scratchDir := t.TempDir()
	writeErr := fmt.Errorf("%w: run \"self\" is already seeded with a different driver", ErrDisagreeingChildSeed)
	calls, deps := newSeedChildDeps("loom", nil, "go", nil, writeErr, nil, nil)

	producer := NewSeedChild("seedchild", "myslug", deps, scratchDir)
	outcome, _, err := producer.Call(context.Background())
	if err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}
	if outcome != shedengine.Stuck {
		t.Fatalf("Call() outcome = %v; want Stuck", outcome)
	}
	if calls.commitCalled || calls.pushCalled {
		t.Errorf("commitCalled=%v pushCalled=%v; want neither -- nothing was written to commit", calls.commitCalled, calls.pushCalled)
	}
	reason := readStuckFile(t, scratchDir, "seedchild")
	if !strings.Contains(reason, "already disagrees") {
		t.Errorf("stuck-reason file = %q; want it to name the disagreement", reason)
	}
}

func TestSeedChild_WriteSeedFailureNotUnknownRecipeIsReturnedError(t *testing.T) {
	scratchDir := t.TempDir()
	writeErr := errors.New("resolve seed path failed")
	_, deps := newSeedChildDeps("batten", nil, "claude", nil, writeErr, nil, nil)

	producer := NewSeedChild("seedchild", "myslug", deps, scratchDir)
	outcome, _, err := producer.Call(context.Background())
	if err == nil {
		t.Fatal("Call() error = nil; want a returned error")
	}
	if !errors.Is(err, writeErr) {
		t.Errorf("Call() error = %v; want it to wrap %v", err, writeErr)
	}
	if outcome == shedengine.Stuck {
		t.Error("Call() outcome = Stuck; want a path-resolution failure to hard-error, not Stuck")
	}
}

func TestSeedChild_FailedCommitIsStuck(t *testing.T) {
	scratchDir := t.TempDir()
	commitErr := errors.New("commit failed")
	calls, deps := newSeedChildDeps("batten", nil, "claude", nil, nil, commitErr, nil)

	producer := NewSeedChild("seedchild", "myslug", deps, scratchDir)
	outcome, _, err := producer.Call(context.Background())
	if err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}
	if outcome != shedengine.Stuck {
		t.Fatalf("Call() outcome = %v; want Stuck", outcome)
	}
	if calls.pushCalled {
		t.Error("PushSeed was called after a failed commit; want it skipped")
	}
	reason := readStuckFile(t, scratchDir, "seedchild")
	if !strings.Contains(reason, "commit") {
		t.Errorf("stuck-reason file = %q; want it to name the commit failure", reason)
	}
}

// TestSeedChild_FailedPushWarnsAndStillReturnsDone is the second load-bearing assertion: a failed
// push must not halt the run, since an offline machine must not block on it, and must be kept
// distinct from the failed-commit arm above rather than collapsed into it.
func TestSeedChild_FailedPushWarnsAndStillReturnsDone(t *testing.T) {
	scratchDir := t.TempDir()
	pushErr := errors.New("push failed: offline")
	calls, deps := newSeedChildDeps("batten", nil, "claude", nil, nil, nil, pushErr)

	producer := NewSeedChild("seedchild", "myslug", deps, scratchDir)
	outcome, _, err := producer.Call(context.Background())
	if err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}
	if outcome != shedengine.Done {
		t.Errorf("Call() outcome = %v; want Done even though push failed", outcome)
	}
	if !calls.commitCalled {
		t.Error("CommitSeed was not called")
	}
	if !calls.pushCalled {
		t.Error("PushSeed was not called")
	}
}

func TestSeedChild_ChildDriverFailureIsReturnedError(t *testing.T) {
	scratchDir := t.TempDir()
	driverErr := errors.New("driver read failed")
	_, deps := newSeedChildDeps("batten", nil, "", driverErr, nil, nil, nil)

	producer := NewSeedChild("seedchild", "myslug", deps, scratchDir)
	outcome, _, err := producer.Call(context.Background())
	if err == nil {
		t.Fatal("Call() error = nil; want a returned error")
	}
	if !errors.Is(err, driverErr) {
		t.Errorf("Call() error = %v; want it to wrap %v", err, driverErr)
	}
	if outcome == shedengine.Stuck {
		t.Error("Call() outcome = Stuck; want a driver-read failure to hard-error, not Stuck")
	}
}

// TestSeedChild_CancelledDuringChildDriverError and TestSeedChild_CancelledDuringWriteSeedError
// assert the two seamchild.go hard-error paths F1 (crucible round sonnet-xhigh-r3) found missing
// their cancelErr check both now carry the cancelled-context diagnosis rather than the raw
// underlying error, when ctx is cancelled by the time the failing seam call itself returns.
func TestSeedChild_CancelledDuringChildDriverError(t *testing.T) {
	scratchDir := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	driverErr := errors.New("driver read failed")
	deps := SeedChildDeps{
		ReadBoardType: func(ctx context.Context) (string, error) { return "batten", nil },
		ChildDriver: func() (string, error) {
			cancel()
			return "", driverErr
		},
		WriteSeed:  func(ctx context.Context, recipe, driver string) error { return nil },
		CommitSeed: func(ctx context.Context) error { return nil },
		PushSeed:   func(ctx context.Context) error { return nil },
	}

	producer := NewSeedChild("seedchild", "myslug", deps, scratchDir)
	_, _, err := producer.Call(ctx)
	if err == nil {
		t.Fatal("Call() error = nil; want the cancelled-context diagnosis")
	}
	if !strings.Contains(err.Error(), "context cancelled during run") {
		t.Errorf("Call() error = %q; want it to carry the cancelled-context diagnosis, not the raw ChildDriver error", err.Error())
	}
}

func TestSeedChild_CancelledDuringWriteSeedError(t *testing.T) {
	scratchDir := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	writeErr := errors.New("resolve seed path failed")
	deps := SeedChildDeps{
		ReadBoardType: func(ctx context.Context) (string, error) { return "batten", nil },
		ChildDriver:   func() (string, error) { return "claude", nil },
		WriteSeed: func(ctx context.Context, recipe, driver string) error {
			cancel()
			return writeErr
		},
		CommitSeed: func(ctx context.Context) error { return nil },
		PushSeed:   func(ctx context.Context) error { return nil },
	}

	producer := NewSeedChild("seedchild", "myslug", deps, scratchDir)
	_, _, err := producer.Call(ctx)
	if err == nil {
		t.Fatal("Call() error = nil; want the cancelled-context diagnosis")
	}
	if !strings.Contains(err.Error(), "context cancelled during run") {
		t.Errorf("Call() error = %q; want it to carry the cancelled-context diagnosis, not the raw WriteSeed error", err.Error())
	}
}

func TestSeedChild_CancelledContext(t *testing.T) {
	scratchDir := t.TempDir()
	_, deps := newSeedChildDeps("batten", nil, "claude", nil, nil, nil, nil)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	producer := NewSeedChild("seedchild", "myslug", deps, scratchDir)
	outcome, _, err := producer.Call(ctx)
	if err == nil {
		t.Fatal("Call() error = nil; want a non-nil error for a cancelled context")
	}
	if outcome == shedengine.Stuck {
		t.Error("Call() outcome = Stuck; want a cancelled context to never surface as Stuck")
	}
}
