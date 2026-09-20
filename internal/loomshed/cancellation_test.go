// cancellation_test.go carries the one test whose subject is this package's own constructors under
// an already-cancelled context, plus the reduced fixture it drives from. It stays in
// internal/loomshed rather than moving to internal/loomrecipe alongside the rest of the sequence
// suite because it calls neither New nor Run: it constructs NewBatchifier, NewWebsterProducer, and
// NewLoomPreflight directly and calls Call on each -- the same criterion keeping batchifier_test.go
// in this package.
//
// Only three of this test's original five producers survive the row-removal batch: the two validate
// producers this test used to cover, discussionValidate and planValidate, are deleted along with
// their own rows, and this is the only test anywhere proving the three survivors' real Call wiring
// returns an error rather than a verdict under an already-cancelled context -- the shared
// cancellation helpers' own tests exercise those helpers in isolation and never through a producer.

package loomshed

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// cancellationFixture carries the three told values TestCancellation_RealProducersReturnErrorNotStuck
// reads. It is a plain struct rather than Deps: Deps is gone from the moved fixture's return and is
// deleted outright in a later batch, so it cannot be the carrier here.
type cancellationFixture struct {
	AnchorPath     string
	StatusPath     string
	StatusLockPath string
}

// buildCancellationFixture seeds only what the three surviving producers this test drives actually
// read: NewBatchifier and NewWebsterProducer both read AnchorPath, and NewLoomPreflight reads the
// status seed the Seed(statusPath, statusLockPath, "fixture-slug", "fixture-parent") call below
// produces. The discussion and plan-format fixtures the two removed validate producers used to read
// are dropped with them.
func buildCancellationFixture(t *testing.T) cancellationFixture {
	t.Helper()

	dir := t.TempDir()

	statusPath := filepath.Join(dir, "status.json")
	statusLockPath := filepath.Join(dir, "status.json.lock")
	if err := Seed(statusPath, statusLockPath, "fixture-slug", "fixture-parent"); err != nil {
		t.Fatalf("Seed(): %v", err)
	}

	return cancellationFixture{
		AnchorPath:     dir,
		StatusPath:     statusPath,
		StatusLockPath: statusLockPath,
	}
}

// TestCancellation_RealProducersReturnErrorNotStuck asserts the one obligation shedengine cannot
// enforce for itself: every real producer this test drives -- the batch gate, the Webster wrapper,
// and loom's own seed row -- returns a non-nil error rather than shedengine.Stuck when called under
// an already-cancelled context. A Stuck under a cancelled context is indistinguishable to Shed from
// a genuine verdict and would silently consume bounce budget for what was actually an operator stop.
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
