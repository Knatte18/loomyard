package loomshed

import (
	"context"
	"testing"

	"github.com/Knatte18/loomyard/internal/batcher"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// fakeWebsterRun is a shedadapters.WebsterRunner fake recording the RunDeps it was called with and
// returning a fixed done outcome.
type fakeWebsterRun struct {
	receivedDeps []websterengine.RunDeps
}

func (f *fakeWebsterRun) run(deps websterengine.RunDeps, _ websterengine.RunOptions) (websterengine.RunResult, error) {
	f.receivedDeps = append(f.receivedDeps, deps)
	return websterengine.RunResult{Outcome: "done"}, nil
}

func TestWebsterProducer_Call(t *testing.T) {
	t.Run("BatcherActiveErrorMapsToStuckNotError", func(t *testing.T) {
		anchorPath := t.TempDir()
		writeBatcherConfig(t, anchorPath, "active: [not valid yaml\n")

		fake := &fakeWebsterRun{}
		p := newWebsterProducer("Webster", anchorPath, fake.run, websterengine.RunDeps{})
		outcome, _, err := p.Call(context.Background())
		if err != nil {
			t.Fatalf("Call() error = %v; want nil", err)
		}
		if outcome != shedengine.Stuck {
			t.Errorf("Call() outcome = %q; want %q", outcome, shedengine.Stuck)
		}
		if len(fake.receivedDeps) != 0 {
			t.Errorf("fake runner was called %d time(s); want 0 -- a batcher.Active error must never reach run", len(fake.receivedDeps))
		}
	})

	t.Run("SuccessfulResolutionReachesRunnerWithNonNilBatcher", func(t *testing.T) {
		anchorPath := t.TempDir()
		writeBatcherConfig(t, anchorPath, `active: "identity"`+"\n")

		fake := &fakeWebsterRun{}
		p := newWebsterProducer("Webster", anchorPath, fake.run, websterengine.RunDeps{})
		outcome, _, err := p.Call(context.Background())
		if err != nil {
			t.Fatalf("Call() error = %v; want nil", err)
		}
		if outcome != shedengine.Done {
			t.Errorf("Call() outcome = %q; want %q", outcome, shedengine.Done)
		}
		if len(fake.receivedDeps) != 1 {
			t.Fatalf("fake runner was called %d time(s); want 1", len(fake.receivedDeps))
		}
		if fake.receivedDeps[0].Batcher == nil {
			t.Fatal("run() received a nil Batcher; want the resolved active batchifier")
		}
		if got, want := fake.receivedDeps[0].Batcher.Name(), batcher.DefaultName; got != want {
			t.Errorf("run() received Batcher.Name() = %q; want %q", got, want)
		}
	})

	t.Run("NoCachedValue_SecondCallObservesMutatedConfig", func(t *testing.T) {
		anchorPath := t.TempDir()
		writeBatcherConfig(t, anchorPath, `active: "identity"`+"\n")

		fake := &fakeWebsterRun{}
		p := newWebsterProducer("Webster", anchorPath, fake.run, websterengine.RunDeps{})

		outcome1, _, err := p.Call(context.Background())
		if err != nil {
			t.Fatalf("first Call() error = %v; want nil", err)
		}
		if outcome1 != shedengine.Done {
			t.Fatalf("first Call() outcome = %q; want %q", outcome1, shedengine.Done)
		}

		writeBatcherConfig(t, anchorPath, `active: "no-such-batcher"`+"\n")

		outcome2, _, err := p.Call(context.Background())
		if err != nil {
			t.Fatalf("second Call() error = %v; want nil", err)
		}
		if outcome2 != shedengine.Stuck {
			t.Errorf("second Call() outcome = %q; want %q -- the wrapper must re-resolve on every Call rather than reuse a value cached at construction or from the first Call", outcome2, shedengine.Stuck)
		}
	})

	t.Run("CancelledContextReturnsErrorNotVerdict", func(t *testing.T) {
		anchorPath := t.TempDir()
		writeBatcherConfig(t, anchorPath, `active: "identity"`+"\n")

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		fake := &fakeWebsterRun{}
		p := newWebsterProducer("Webster", anchorPath, fake.run, websterengine.RunDeps{})
		outcome, _, err := p.Call(ctx)
		if err == nil {
			t.Fatalf("Call(cancelled) error = nil; want non-nil error")
		}
		if outcome == shedengine.Done || outcome == shedengine.Stuck {
			t.Errorf("Call(cancelled) outcome = %q; want no verdict alongside a cancellation error", outcome)
		}
	})
}
