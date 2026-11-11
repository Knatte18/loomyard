// ended_test.go pins which runs dobby counts as stranded past their ended board entry,
// and the teardown of their pairs it starts, reads and retries.

package dobbyengine

import (
	"errors"
	"slices"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestStrandedEnded(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name     string
		mutate   func(run *RunView, rs *RunState)
		inFlight bool
		want     bool
	}{
		{"FailedWithItsEntryEndedAndItsPairOnDisk", nil, false, true},
		{"Halted", func(run *RunView, _ *RunState) { run.State = "blocked" }, false, true},
		{"PausedShortOfItsParent", func(run *RunView, _ *RunState) { run.State = runStatePaused }, false, true},
		{"Running", func(run *RunView, _ *RunState) { run.State = runStateRunning }, false, false},
		{"PausedWithItsWorkOnItsParent", func(run *RunView, _ *RunState) { run.State, run.ReachedParent = runStatePaused, true }, false, false},
		{"Done", func(run *RunView, _ *RunState) { run.State = runStateDone }, false, false},
		{"EntryOpen", func(run *RunView, _ *RunState) { run.EntryEnded = false }, false, false},
		{"PairGone", func(run *RunView, _ *RunState) { run.PairPresent = false }, false, false},
		{"PassInFlight", nil, true, false},
		{"DriverLockHeld", func(run *RunView, _ *RunState) { run.DriverLockHeld = true }, false, false},
		{"CancelMarked", func(run *RunView, _ *RunState) { run.CancelMarker = true }, false, false},
		{"CoveredByAPause", func(run *RunView, _ *RunState) { run.Covered = true }, false, false},
		{"OwnedElsewhere", func(_ *RunView, rs *RunState) { rs.OwnedElsewhere = true }, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			run, rs := strandedRun("x"), RunState{}
			if tt.mutate != nil {
				tt.mutate(&run, &rs)
			}
			if got := strandedEnded(run, rs, tt.inFlight); got != tt.want {
				t.Errorf("strandedEnded(%+v, %+v, %v) = %v; want %v", run, rs, tt.inFlight, got, tt.want)
			}
		})
	}
}

func TestTearDownEnded(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	t.Run("a removed pair logs once and its run gets no further teardown once its pair is gone", func(t *testing.T) {
		t.Parallel()
		h := newPassHarness(t, strandedRun("x"))
		lines := h.recordLog()
		fake := &teardownFake{results: []map[string]error{{"x": nil}}}
		h.d.deps.TearDownEnded = fake.tearDown

		h.tick(false)
		h.tick(false)
		h.fake.runs[0].PairPresent = false
		h.tick(false)
		h.tick(false)

		if want := [][]string{{"x"}}; !slices.EqualFunc(fake.calls, want, slices.Equal[[]string]) {
			t.Errorf("teardown calls = %q; want %q", fake.calls, want)
		}
		if retry, held := h.d.endedRetries["x"]; held {
			t.Errorf("endedRetries[x] = %+v; want no wait for a run whose pair is gone", retry)
		}
		if want := []string{"run x: failed at Run-Shed", "teardown x: pair removed"}; !slices.Equal(*lines, want) {
			t.Errorf("log lines = %q; want %q", *lines, want)
		}
	})

	t.Run("a failing teardown logs once and retries after one probe interval, then after twice it", func(t *testing.T) {
		t.Parallel()
		h := newPassHarness(t, strandedRun("x"))
		lines := h.recordLog()
		busy := map[string]error{"x": errors.New("its pair is busy")}
		fake := &teardownFake{results: slices.Repeat([]map[string]error{busy}, 12)}
		h.d.deps.TearDownEnded = fake.tearDown

		for _, want := range []int{1, 1, 2, 2, 2, 3} {
			h.tick(false)
			if got := len(fake.calls); got != want {
				t.Fatalf("teardown calls = %d; want %d", got, want)
			}
		}
		// The wait doubles from 10s, so the 10th failure reaches the ceiling and the 12th, the last one read, must not pass it.
		for len(fake.calls) < 12 {
			h.advance(h.d.endedRetries["x"].next.Sub(h.clock.now()))
			h.tick(false)
		}
		// This tick reads the 12th result and, with the wait it sets, starts no 13th teardown.
		h.tick(false)
		retry := h.d.endedRetries["x"]
		if next := retry.next.Sub(h.clock.now()); retry.interval != endedTeardownCeiling || next != endedTeardownCeiling {
			t.Errorf("wait, time to next attempt after 12 failures read = %v, %v; want %v, %v",
				retry.interval, next, endedTeardownCeiling, endedTeardownCeiling)
		}
		if want := []string{"run x: failed at Run-Shed", "teardown x failing: its pair is busy"}; !slices.Equal(*lines, want) {
			t.Errorf("log lines = %q; want %q", *lines, want)
		}
	})

	t.Run("no teardown seam starts none and leaves the loop idle", func(t *testing.T) {
		t.Parallel()
		h := newPassHarness(t, strandedRun("x"))

		h.tick(false)

		if h.d.teardown != nil || h.d.inFlight || len(h.d.endedRetries) != 0 {
			t.Errorf("teardown, in flight, waits = %v, %v, %v; want nil, false, none", h.d.teardown, h.d.inFlight, h.d.endedRetries)
		}
	})

	t.Run("a teardown in flight holds the binary check", func(t *testing.T) {
		t.Parallel()
		h := newLoopHarness(t, 3)
		h.deps.Runs = func() ([]RunView, error) { return []RunView{strandedRun("x")}, nil }
		h.deps.TearDownEnded = func([]string) <-chan map[string]error { return make(chan map[string]error) }
		checks := 0
		h.checkBinary = func(func(), func(error)) { checks++ }

		if err := h.run(t); err != nil {
			t.Fatalf("Run() = %v; want nil", err)
		}

		if checks != 0 {
			t.Errorf("CheckBinary calls = %d; want 0 while a teardown is in flight", checks)
		}
	})

	t.Run("a teardown in flight is not doubled and keeps the loop busy", func(t *testing.T) {
		t.Parallel()
		h := newPassHarness(t, strandedRun("x"))
		calls := 0
		h.d.deps.TearDownEnded = func([]string) <-chan map[string]error {
			calls++
			return make(chan map[string]error)
		}

		h.tick(false)
		h.tick(false)

		if calls != 1 || !h.d.inFlight {
			t.Errorf("teardown calls, in flight = %d, %v; want 1, true", calls, h.d.inFlight)
		}
	})

	t.Run("a result that leaves the run out fails it as unreported", func(t *testing.T) {
		t.Parallel()
		h := newPassHarness(t, strandedRun("x"))
		lines := h.recordLog()
		fake := &teardownFake{results: []map[string]error{{}}}
		h.d.deps.TearDownEnded = fake.tearDown

		h.tick(false)
		h.tick(false)

		want := []string{"run x: failed at Run-Shed", "teardown x failing: " + errTeardownUnreported.Error()}
		if !slices.Equal(*lines, want) {
			t.Errorf("log lines = %q; want %q", *lines, want)
		}
	})
}

// strandedRun is a run batten left failed at Run-Shed whose board entry ended and whose pair is still on disk.
func strandedRun(slug string) RunView {
	return RunView{Slug: slug, State: "failed", Producer: producerRunShed, EntryEnded: true, PairPresent: true}
}

// teardownFake answers the TearDownEnded seam with its next result, delivered at once, and records each call's slugs.
type teardownFake struct {
	calls   [][]string
	results []map[string]error
}

func (f *teardownFake) tearDown(slugs []string) <-chan map[string]error {
	f.calls = append(f.calls, slices.Clone(slugs))
	result := make(chan map[string]error, 1)
	next, rest := f.results[0], f.results[1:]
	f.results = rest
	result <- next
	return result
}
