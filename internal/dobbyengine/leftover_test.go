// leftover_test.go pins which pairs dobby counts as left over and the teardown it starts, reads and retries.
// It also pins the notice of the kept pairs and the orphan pairs dobby notices and tears down after their grace.

package dobbyengine

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/runfsm"
	"github.com/Knatte18/loomyard/internal/state"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// ReadOrphanSights reads the first-sight record at path; an absent record reads empty.
func ReadOrphanSights(path string) (OrphanSights, error) {
	sights, _, err := state.ReadJSON[OrphanSights](path, lockPath(path))
	return sights, err
}

func TestLeftoverKind(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name     string
		run      RunView
		mutate   func(run *RunView, rs *RunState)
		inFlight bool
		want     LeftoverKind
	}{
		{"FailedWithItsEntryEndedAndItsPairOnDisk", strandedRun("x"), nil, false, LeftoverEnded},
		{"Halted", strandedRun("x"), func(run *RunView, _ *RunState) { run.State = "blocked" }, false, LeftoverEnded},
		{"PausedShortOfItsParent", strandedRun("x"), func(run *RunView, _ *RunState) { run.State = runStatePaused }, false, LeftoverEnded},
		{"Running", strandedRun("x"), func(run *RunView, _ *RunState) { run.State = runStateRunning }, false, ""},
		{"PausedWithItsWorkOnItsParent", strandedRun("x"), func(run *RunView, _ *RunState) { run.State, run.ReachedParent = runStatePaused, true }, false, ""},
		{"DoneWithItsEntryEnded", strandedRun("x"), func(run *RunView, _ *RunState) { run.State = runStateDone }, false, ""},
		{"EntryOpen", strandedRun("x"), func(run *RunView, _ *RunState) { run.EntryEnded = false }, false, ""},
		{"PairGone", strandedRun("x"), func(run *RunView, _ *RunState) { run.PairPresent = false }, false, ""},
		{"PassInFlight", strandedRun("x"), nil, true, ""},
		{"DriverLockHeld", strandedRun("x"), func(run *RunView, _ *RunState) { run.DriverLockHeld = true }, false, ""},
		{"CancelMarked", strandedRun("x"), func(run *RunView, _ *RunState) { run.CancelMarker = true }, false, ""},
		{"CoveredByAPause", strandedRun("x"), func(run *RunView, _ *RunState) { run.Covered = true }, false, ""},
		{"MarkedOwnedElsewhere", strandedRun("x"), func(_ *RunView, rs *RunState) { rs.OwnedElsewhere = true }, false, ""},
		{"Landed", landedRun("x"), nil, false, LeftoverLanded},
		{"LandedWithItsEntryOpen", landedRun("x"), func(run *RunView, _ *RunState) { run.EntryEnded = false }, false, LeftoverLanded},
		{"LandedUnderABoltHold", landedRun("x"), func(run *RunView, _ *RunState) { run.BoltHeld = true }, false, ""},
		{"LandedOwnedElsewhere", landedRun("x"), func(run *RunView, _ *RunState) { run.OwnedElsewhere = true }, false, ""},
		{"LandedCoveredByAPause", landedRun("x"), func(run *RunView, _ *RunState) { run.Covered = true }, false, ""},
		{"CancelledWithoutKeep", cancelledRun("x", false), nil, false, LeftoverCancelled},
		{"CancelledWithKeep", cancelledRun("x", true), nil, false, ""},
		{"CancelledWithKeepAndItsEntryRemoved", cancelledRun("x", true), func(run *RunView, _ *RunState) { run.EntryEnded = true }, false, ""},
		{"HandedOverAndOwnedElsewhere", handedOverRun("x"), nil, false, LeftoverHandedOver},
		{"HandedOverUnderABoltHoldAndAPause", handedOverRun("x"), func(run *RunView, _ *RunState) { run.BoltHeld, run.Covered = true, true }, false, LeftoverHandedOver},
		{"HandedOverThenAdoptedBack", handedOverRun("x"), func(run *RunView, _ *RunState) { run.OwnedElsewhere = false }, false, ""},
		{"HandedOverWithAPassInFlight", handedOverRun("x"), nil, true, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			run, rs := tt.run, RunState{}
			if tt.mutate != nil {
				tt.mutate(&run, &rs)
			}
			got, left := leftoverKind(run, rs, tt.inFlight)
			if got != tt.want || left != (tt.want != "") {
				t.Errorf("leftoverKind(%+v, %+v, %v) = %q, %v; want %q, %v", run, rs, tt.inFlight, got, left, tt.want, tt.want != "")
			}
		})
	}
}

func TestTearDownLeftovers(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	t.Run("a removed pair logs once and gets no further teardown once it is gone", func(t *testing.T) {
		t.Parallel()
		h := newPassHarness(t, strandedRun("x"))
		lines := h.recordLog()
		fake := &teardownFake{results: []map[string]error{{"x": nil}}}
		h.d.deps.TearDownLeftovers = fake.tearDown

		h.tick(false)
		h.tick(false)
		h.fake.runs[0].PairPresent = false
		h.tick(false)
		h.tick(false)

		if want := [][]LeftoverPair{{{Slug: "x", Kind: LeftoverEnded}}}; !slices.EqualFunc(fake.calls, want, slices.Equal[[]LeftoverPair]) {
			t.Errorf("teardown calls = %v; want %v", fake.calls, want)
		}
		if retry, held := h.d.leftoverRetries["x"]; held {
			t.Errorf("leftoverRetries[x] = %+v; want no wait for a pair that is gone", retry)
		}
		if want := []string{"run x: halted at Run-Shed", "teardown x: pair removed"}; !slices.Equal(*lines, want) {
			t.Errorf("log lines = %q; want %q", *lines, want)
		}
	})

	t.Run("a landed run's pair is torn down though batten reads done and the run takes no pass", func(t *testing.T) {
		t.Parallel()
		h := newPassHarness(t, landedRun("x"), cancelledRun("y", false), cancelledRun("z", true))
		fake := &teardownFake{results: []map[string]error{{"x": nil, "y": nil}}}
		h.d.deps.TearDownLeftovers = fake.tearDown

		h.tick(false)

		want := [][]LeftoverPair{{{Slug: "x", Kind: LeftoverLanded}, {Slug: "y", Kind: LeftoverCancelled}}}
		if !slices.EqualFunc(fake.calls, want, slices.Equal[[]LeftoverPair]) {
			t.Errorf("teardown calls = %v; want %v", fake.calls, want)
		}
	})

	t.Run("a failing teardown logs once and retries after one probe interval, then after twice it", func(t *testing.T) {
		t.Parallel()
		h := newPassHarness(t, strandedRun("x"))
		lines := h.recordLog()
		busy := map[string]error{"x": errors.New("its pair is busy")}
		fake := &teardownFake{results: slices.Repeat([]map[string]error{busy}, 12)}
		h.d.deps.TearDownLeftovers = fake.tearDown

		for _, want := range []int{1, 1, 2, 2, 2, 3} {
			h.tick(false)
			if got := len(fake.calls); got != want {
				t.Fatalf("teardown calls = %d; want %d", got, want)
			}
		}
		// The wait doubles from 10s, so the 10th failure reaches the ceiling and the 12th, the last one read, must not pass it.
		for len(fake.calls) < 12 {
			h.advance(h.d.leftoverRetries["x"].next.Sub(h.clock.now()))
			h.tick(false)
		}
		// This tick reads the 12th result and, with the wait it sets, starts no 13th teardown.
		h.tick(false)
		retry := h.d.leftoverRetries["x"]
		if next := retry.next.Sub(h.clock.now()); retry.interval != leftoverTeardownCeiling || next != leftoverTeardownCeiling {
			t.Errorf("wait, time to next attempt after 12 failures read = %v, %v; want %v, %v",
				retry.interval, next, leftoverTeardownCeiling, leftoverTeardownCeiling)
		}
		if want := []string{"run x: halted at Run-Shed", "teardown x failing: its pair is busy"}; !slices.Equal(*lines, want) {
			t.Errorf("log lines = %q; want %q", *lines, want)
		}
	})

	t.Run("no teardown seam starts none and leaves the loop idle", func(t *testing.T) {
		t.Parallel()
		h := newPassHarness(t, strandedRun("x"))

		h.tick(false)

		if h.d.teardown != nil || h.d.inFlight || len(h.d.leftoverRetries) != 0 {
			t.Errorf("teardown, in flight, waits = %v, %v, %v; want nil, false, none", h.d.teardown, h.d.inFlight, h.d.leftoverRetries)
		}
	})

	t.Run("a teardown in flight holds the binary check", func(t *testing.T) {
		t.Parallel()
		h := newLoopHarness(t, 3)
		h.deps.Runs = func() ([]RunView, error) { return []RunView{strandedRun("x")}, nil }
		h.deps.TearDownLeftovers = func([]LeftoverPair) <-chan map[string]error { return make(chan map[string]error) }
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
		h.d.deps.TearDownLeftovers = func([]LeftoverPair) <-chan map[string]error {
			calls++
			return make(chan map[string]error)
		}

		h.tick(false)
		h.tick(false)

		if calls != 1 || !h.d.inFlight {
			t.Errorf("teardown calls, in flight = %d, %v; want 1, true", calls, h.d.inFlight)
		}
	})

	t.Run("a result that leaves the pair out fails it as unreported", func(t *testing.T) {
		t.Parallel()
		h := newPassHarness(t, strandedRun("x"))
		lines := h.recordLog()
		fake := &teardownFake{results: []map[string]error{{}}}
		h.d.deps.TearDownLeftovers = fake.tearDown

		h.tick(false)
		h.tick(false)

		want := []string{"run x: halted at Run-Shed", "teardown x failing: " + errTeardownUnreported.Error()}
		if !slices.Equal(*lines, want) {
			t.Errorf("log lines = %q; want %q", *lines, want)
		}
	})
}

func TestKeptPairs(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	since := time.Date(2026, 11, 10, 8, 0, 0, 0, time.UTC)
	keptWithSince := func(slug string) RunView {
		run := cancelledRun(slug, true)
		run.KeptSince = since
		return run
	}

	t.Run("a fresh start posts one notice naming each kept pair and the verb that removes it", func(t *testing.T) {
		t.Parallel()
		h := newPassHarness(t, keptWithSince("k"), runningRun("r"), cancelledRun("gone", true))
		h.fake.runs[2].PairPresent = false
		h.d.keptNoticeOwed = true

		h.tick(false)
		h.tick(false)

		want := []string{"prime: kept pairs stand: k (kept since 2026-11-10T08:00:00Z); way forward: `lym fabric remove <slug>` removes each pair no longer needed"}
		if !slices.Equal(h.fake.notices, want) {
			t.Errorf("notices = %q; want %q", h.fake.notices, want)
		}
	})

	t.Run("a re-exec posts no notice", func(t *testing.T) {
		t.Parallel()
		h := newPassHarness(t, keptWithSince("k"))

		h.tick(false)

		if len(h.fake.notices) != 0 {
			t.Errorf("notices = %q; want none", h.fake.notices)
		}
	})

	t.Run("the view lists each kept pair that stands with when its cancel finished", func(t *testing.T) {
		t.Parallel()
		gone := cancelledRun("gone", true)
		gone.PairPresent = false
		view := BuildView(ViewInput{
			Runs:   []RunView{keptWithSince("k"), cancelledRun("removed", false), gone},
			Covers: func(string) bool { return false },
		})
		if want := []KeptPair{{Slug: "k", Since: since}}; !slices.Equal(view.Kept, want) {
			t.Errorf("Kept = %+v; want %+v", view.Kept, want)
		}
	})
}

func TestOrphans(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	// orphanHarness is a dobby with no runs whose orphan seam answers listing and whose teardowns always succeed.
	orphanHarness := func(t *testing.T, record string, listing *[]OrphanView) (*passHarness, *teardownFake) {
		t.Helper()
		h := newPassHarness(t)
		h.d.deps.Orphans = func() ([]OrphanView, error) { return slices.Clone(*listing), nil }
		h.d.deps.OrphanRecord = record
		fake := &teardownFake{results: slices.Repeat([]map[string]error{{"o": nil}}, 4)}
		h.d.deps.TearDownLeftovers = fake.tearDown
		return h, fake
	}

	t.Run("an idle orphan is noticed once and torn down no earlier than the grace after the notice", func(t *testing.T) {
		t.Parallel()
		record := filepath.Join(t.TempDir(), "orphan", "first-sight.json")
		listing := []OrphanView{{Slug: "o"}}
		h, fake := orphanHarness(t, record, &listing)
		first := h.clock.now().Add(10 * time.Second)

		h.tick(true)
		h.advance(OrphanGrace - 20*time.Second)
		h.tick(true)
		callsInGrace := len(fake.calls)
		h.advance(10 * time.Second)
		h.tick(true)

		if len(h.fake.notices) != 1 || !strings.Contains(h.fake.notices[0], "o: orphan pair") ||
			!strings.Contains(h.fake.notices[0], first.Add(OrphanGrace).Format(time.RFC3339)) {
			t.Errorf("notices = %q; want one orphan notice naming %s", h.fake.notices, first.Add(OrphanGrace).Format(time.RFC3339))
		}
		if want := [][]LeftoverPair{{{Slug: "o", Kind: LeftoverOrphan}}}; callsInGrace != 0 || !slices.EqualFunc(fake.calls, want, slices.Equal[[]LeftoverPair]) {
			t.Errorf("teardown calls inside the grace, after = %d, %v; want 0, %v", callsInGrace, fake.calls, want)
		}
	})

	t.Run("an orphan with a commit inside the grace or a live process stays", func(t *testing.T) {
		t.Parallel()
		for _, orphan := range []func(now time.Time) OrphanView{
			func(now time.Time) OrphanView { return OrphanView{Slug: "o", LastCommit: now.Add(-time.Hour)} },
			func(time.Time) OrphanView { return OrphanView{Slug: "o", ProcessLive: true} },
		} {
			record := filepath.Join(t.TempDir(), "orphan", "first-sight.json")
			var listing []OrphanView
			h, fake := orphanHarness(t, record, &listing)
			listing = []OrphanView{orphan(h.clock.now())}

			h.tick(true)
			h.advance(OrphanGrace + time.Minute)
			listing = []OrphanView{orphan(h.clock.now())}
			h.tick(true)

			if len(fake.calls) != 0 {
				t.Errorf("teardown calls = %v; want none for %+v", fake.calls, listing[0])
			}
		}
	})

	t.Run("a restarted dobby keeps the first sight and posts no second notice", func(t *testing.T) {
		t.Parallel()
		record := filepath.Join(t.TempDir(), "orphan", "first-sight.json")
		listing := []OrphanView{{Slug: "o"}}
		h, fake := orphanHarness(t, record, &listing)
		h.tick(true)
		first := h.clock.now()
		h.advance(OrphanGrace)

		restarted, restartedFake := orphanHarness(t, record, &listing)
		restarted.clock = h.clock
		restarted.d.deps.Now, restarted.d.deps.Awake, restarted.d.deps.Sleep = h.clock.now, h.clock.awake, h.clock.sleep
		restarted.tick(true)

		sights, err := ReadOrphanSights(record)
		if err != nil || !sights.Seen["o"].Equal(first) {
			t.Errorf("ReadOrphanSights() = %v, %v; want o first seen at %v", sights, err, first)
		}
		if len(restarted.fake.notices) != 0 || len(fake.calls) != 0 || len(restartedFake.calls) != 1 {
			t.Errorf("restarted notices, first and restarted teardowns = %q, %d, %d; want none, 0, 1",
				restarted.fake.notices, len(fake.calls), len(restartedFake.calls))
		}
	})

	t.Run("a pair that stops being an orphan or is torn down leaves the record", func(t *testing.T) {
		t.Parallel()
		record := filepath.Join(t.TempDir(), "orphan", "first-sight.json")
		listing := []OrphanView{{Slug: "o"}, {Slug: "p", ProcessLive: true}}
		h, _ := orphanHarness(t, record, &listing)
		h.tick(true)
		h.advance(OrphanGrace)
		listing = []OrphanView{{Slug: "o"}}
		h.tick(true)
		h.tick(false)

		sights, err := ReadOrphanSights(record)
		if err != nil || len(sights.Seen) != 0 {
			t.Errorf("ReadOrphanSights() = %v, %v; want an empty record", sights, err)
		}
	})

	t.Run("an orphan whose notice fails is not recorded and is noticed again before any teardown", func(t *testing.T) {
		t.Parallel()
		record := filepath.Join(t.TempDir(), "orphan", "first-sight.json")
		listing := []OrphanView{{Slug: "o"}}
		h, fake := orphanHarness(t, record, &listing)
		posts := 0
		h.d.deps.Notice = func(context.Context, string) error {
			posts++
			if posts == 1 {
				return errors.New("no orch strand is recorded to receive the notice")
			}
			return nil
		}

		h.tick(true)
		afterFailure, err := ReadOrphanSights(record)
		if err != nil || len(afterFailure.Seen) != 0 {
			t.Errorf("ReadOrphanSights() after the failed notice = %v, %v; want an empty record", afterFailure, err)
		}
		h.advance(OrphanGrace)
		h.tick(true)
		callsAtRetry := len(fake.calls)
		h.advance(OrphanGrace)
		h.tick(true)

		if posts != 2 {
			t.Errorf("notice posts = %d; want 2, the failed one and its retry", posts)
		}
		if want := [][]LeftoverPair{{{Slug: "o", Kind: LeftoverOrphan}}}; callsAtRetry != 0 || !slices.EqualFunc(fake.calls, want, slices.Equal[[]LeftoverPair]) {
			t.Errorf("teardown calls at the retry, after the grace from it = %d, %v; want 0, %v", callsAtRetry, fake.calls, want)
		}
	})

	t.Run("a failing orphan reading keeps the last due list until a later reading recovers", func(t *testing.T) {
		t.Parallel()
		record := filepath.Join(t.TempDir(), "orphan", "first-sight.json")
		listing := []OrphanView{{Slug: "o"}}
		h, _ := orphanHarness(t, record, &listing)
		busy := &teardownFake{results: slices.Repeat([]map[string]error{{"o": errors.New("its pair is busy")}}, 4)}
		h.d.deps.TearDownLeftovers = busy.tearDown
		failing := false
		h.d.deps.Orphans = func() ([]OrphanView, error) {
			if failing {
				return nil, errors.New("the worktree list failed")
			}
			return slices.Clone(listing), nil
		}

		h.tick(true)
		h.advance(OrphanGrace)
		h.tick(true)
		if !slices.Equal(h.d.orphansDue, []string{"o"}) {
			t.Fatalf("due orphans after the good reading = %v; want [o]", h.d.orphansDue)
		}
		failing = true
		h.tick(true)
		if !slices.Equal(h.d.orphansDue, []string{"o"}) {
			t.Errorf("due orphans after the failed reading = %v; want the last reading's [o]", h.d.orphansDue)
		}
		failing, listing = false, nil
		h.tick(true)
		if len(h.d.orphansDue) != 0 {
			t.Errorf("due orphans after the recovered reading = %v; want none", h.d.orphansDue)
		}
	})
}

// strandedRun is a run batten left failed at Run-Shed whose board entry ended and whose pair is still on disk.
func strandedRun(slug string) RunView {
	return RunView{Slug: slug, State: "failed", Run: runfsm.Halted, Producer: producerRunShed, EntryEnded: true, PairPresent: true}
}

// landedRun is a run batten finished as done, landed in the FSM, whose pair is still on disk.
func landedRun(slug string) RunView {
	return RunView{Slug: slug, State: runStateDone, Run: runfsm.Landed, Producer: "Worktree-Teardown", PairPresent: true}
}

// cancelledRun is a run whose cancel finished, keeping its pair or not, with its pair still on disk.
func cancelledRun(slug string, keep bool) RunView {
	return RunView{Slug: slug, State: "cancelled", Run: runfsm.Cancelled, Producer: producerRunShed, PairKept: keep, PairPresent: true}
}

// handedOverRun is a run this machine handed over, which another machine owns, with its local pair still on disk.
func handedOverRun(slug string) RunView {
	return RunView{Slug: slug, State: runStateRunning, Run: runfsm.Running, Producer: producerRunShed, PairPresent: true, OwnedElsewhere: true, HandOverMarked: true}
}

// teardownFake answers the TearDownLeftovers seam with its next result, delivered at once, and records each call's pairs.
type teardownFake struct {
	calls   [][]LeftoverPair
	results []map[string]error
}

func (f *teardownFake) tearDown(pairs []LeftoverPair) <-chan map[string]error {
	f.calls = append(f.calls, slices.Clone(pairs))
	result := make(chan map[string]error, 1)
	next, rest := f.results[0], f.results[1:]
	f.results = rest
	result <- next
	return result
}
