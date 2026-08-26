// cancellation_test.go carries the one test whose subject is this package's own constructors under
// an already-cancelled context, plus the reduced fixture it drives from. It stays in
// internal/loomshed rather than moving to internal/loomrecipe alongside the rest of the sequence
// suite because it calls neither New nor Run: it constructs NewDiscussionValidate, NewPlanValidate,
// NewBatchifier, NewWebsterProducer, and NewLoomPreflight directly and calls Call on each -- the
// same criterion keeping batchifier_test.go and planvalidate_test.go in this package.

package loomshed

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// cancellationFixture carries the six told values TestCancellation_RealProducersReturnErrorNotStuck
// reads. It is a plain struct rather than Deps: Deps is gone from the moved fixture's return and is
// deleted outright in a later batch, so it cannot be the carrier here.
type cancellationFixture struct {
	AnchorPath         string
	WorktreeRoot       string
	DecisionRecordPath string
	SupportLogPath     string
	StatusPath         string
	StatusLockPath     string
}

// buildCancellationFixture reproduces the moved whole-list fixture's on-disk seeding exactly --
// writeDiscussionFixture into a discussion subdirectory of one t.TempDir(), the plan-validate
// fixture seed, and the Seed(statusPath, statusLockPath, "fixture-slug", "fixture-parent") call --
// so this test's behaviour is unchanged by the reduction. It drops only the construction this test
// does not read: the whole-list Env/ShedPaths pair, the landing passthrough, and the row-1/Webster
// injection.
func buildCancellationFixture(t *testing.T) cancellationFixture {
	t.Helper()

	dir := t.TempDir()

	discussionDir := filepath.Join(dir, "discussion")
	if err := os.MkdirAll(discussionDir, 0o755); err != nil {
		t.Fatalf("mkdir discussion dir: %v", err)
	}
	decisionRecordPath, supportLogPath := writeDiscussionFixture(t, discussionDir, validDecisionRecord, "support log")

	seedPlanValidateFixture(t, dir, true)

	statusPath := filepath.Join(dir, "status.json")
	statusLockPath := filepath.Join(dir, "status.json.lock")
	if err := Seed(statusPath, statusLockPath, "fixture-slug", "fixture-parent"); err != nil {
		t.Fatalf("Seed(): %v", err)
	}

	return cancellationFixture{
		AnchorPath:         dir,
		WorktreeRoot:       dir,
		DecisionRecordPath: decisionRecordPath,
		SupportLogPath:     supportLogPath,
		StatusPath:         statusPath,
		StatusLockPath:     statusLockPath,
	}
}

// TestCancellation_RealProducersReturnErrorNotStuck asserts the one obligation shedengine cannot
// enforce for itself: every real producer this task builds -- the discussion validator, the plan
// validator, the batch gate, the Webster wrapper, and loom's own seed row -- returns a non-nil
// error rather than shedengine.Stuck when called under an already-cancelled context. A Stuck under
// a cancelled context is indistinguishable to Shed from a genuine verdict and would silently
// consume bounce budget for what was actually an operator stop.
func TestCancellation_RealProducersReturnErrorNotStuck(t *testing.T) {
	fx := buildCancellationFixture(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	producers := []struct {
		name string
		p    interface {
			Call(context.Context) (shedengine.Outcome, shedengine.OutputPointer, error)
		}
	}{
		{NameDiscussionValidate, NewDiscussionValidate(NameDiscussionValidate, fx.DecisionRecordPath, fx.SupportLogPath)},
		{NamePlanValidate, NewPlanValidate(NamePlanValidate, fx.AnchorPath, fx.WorktreeRoot, true)},
		{NameBatchifier, NewBatchifier(NameBatchifier, fx.AnchorPath)},
		{NameWebster, NewWebsterProducer(NameWebster, fx.AnchorPath, (&fakeWebsterRun{}).run, websterengine.RunDeps{})},
		{NameLoomPreflight, NewLoomPreflight(NameLoomPreflight, fx.StatusPath, fx.StatusLockPath)},
	}

	for _, tt := range producers {
		t.Run(tt.name, func(t *testing.T) {
			outcome, _, err := tt.p.Call(ctx)
			if err == nil {
				t.Fatalf("Call(cancelled) error = nil; want non-nil error")
			}
			if outcome == shedengine.Done || outcome == shedengine.Stuck {
				t.Errorf("Call(cancelled) outcome = %q; want no verdict alongside a cancellation error", outcome)
			}
		})
	}
}
