// testmap_test.go pins the prime dobby's coverage-map upkeep step over fake seams:
// the start rule over the exit record, HEAD and slots.
// It also pins the hold while a pass is registered or still the dobby's child.
// It pins the once-per-episode log of a failed exit, and a failed start or read.
// It pins that a failing step leaves the rest of the tick running.

package dobbyengine

import (
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// testMapHarness drives a daemon's upkeep step over fakes.
type testMapHarness struct {
	t *testing.T
	d *daemon
	// upkeep is the test_map_upkeep the config resolves; running is what TestMapRunning reads.
	upkeep, running bool
	// exit and exitFound are what TestMapExitRecord reads; head is the prime's HEAD and slots the gate slot count.
	exit      TestMapExit
	exitFound bool
	head      string
	slots     int
	// The errors, when set, are what the matching seams return.
	configErr, runningErr, exitErr, headErr, slotsErr, startErr error
	// children holds the exit channel of each pass started, in order.
	children []chan PassExit
	// logged holds the dobby log lines written, without their timestamps.
	logged []string
	now    time.Time
}

func newTestMapHarness(t *testing.T) *testMapHarness {
	t.Helper()
	h := &testMapHarness{t: t, upkeep: true, head: "h1", slots: 2, now: time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)}
	h.d = &daemon{
		geom: Geometry{WorktreeName: "prime", StatePath: filepath.Join(t.TempDir(), StateFileName)},
		deps: Deps{
			LoadConfig: func() (Config, error) { return Config{TestMapUpkeep: h.upkeep}, h.configErr },
			TestMapRunning: func() (bool, error) {
				return h.running, h.runningErr
			},
			TestMapExitRecord: func() (TestMapExit, bool, error) { return h.exit, h.exitFound, h.exitErr },
			PrimeHead:         func() (string, error) { return h.head, h.headErr },
			GateSlots:         func() (int, error) { return h.slots, h.slotsErr },
			StartTestMap: func() (<-chan PassExit, error) {
				if h.startErr != nil {
					return nil, h.startErr
				}
				child := make(chan PassExit, 1)
				h.children = append(h.children, child)
				return child, nil
			},
			Log: func(_, line string, _ bool) {
				_, text, _ := strings.Cut(line, " ")
				h.logged = append(h.logged, text)
			},
		},
	}
	return h
}

// mustStep runs the upkeep step at the harness's clock and fails the test on its error.
func (h *testMapHarness) mustStep() {
	h.t.Helper()
	if err := h.d.testMapStep(h.now); err != nil {
		h.t.Fatalf("testMapStep() = %v; want nil", err)
	}
}

// exitLast delivers exit as the exit of the last pass started.
func (h *testMapHarness) exitLast(exit PassExit) {
	h.t.Helper()
	h.children[len(h.children)-1] <- exit
}

func TestTestMapStep(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	t.Run("the start rule", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name    string
			arrange func(h *testMapHarness)
			// wantLog is the one start line, empty when the tick starts none.
			wantLog string
		}{
			{"no exit record starts a pass", func(*testMapHarness) {}, "upkeep pass started: no exit record"},
			{
				"a HEAD past the planned one starts a pass on a one-slot hub",
				func(h *testMapHarness) {
					h.exitFound, h.exit, h.slots = true, TestMapExit{PlannedHead: "h0", Queued: 2}, 1
				},
				"upkeep pass started: HEAD moved past the last plan",
			},
			{
				"queued work at the planned HEAD starts a pass on a two-slot hub",
				func(h *testMapHarness) { h.exitFound, h.exit = true, TestMapExit{PlannedHead: "h1", Queued: 4} },
				"upkeep pass started: 4 queued",
			},
			{
				"queued work at the planned HEAD starts none on a one-slot hub",
				func(h *testMapHarness) {
					h.exitFound, h.exit, h.slots = true, TestMapExit{PlannedHead: "h1", Queued: 4}, 1
				},
				"",
			},
			{
				"no queued work at the planned HEAD starts none, reading no slots",
				func(h *testMapHarness) {
					h.exitFound, h.exit = true, TestMapExit{PlannedHead: "h1"}
					h.slotsErr = errors.New("must not be read")
				},
				"",
			},
			{"test_map_upkeep false starts none", func(h *testMapHarness) { h.upkeep = false }, ""},
			{"a registered pass, a hand-started one included, holds the start", func(h *testMapHarness) { h.running = true }, ""},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				h := newTestMapHarness(t)
				tt.arrange(h)
				h.mustStep()
				var wantLogged []string
				if tt.wantLog != "" {
					wantLogged = []string{tt.wantLog}
				}
				if len(h.children) != len(wantLogged) || !slices.Equal(h.logged, wantLogged) {
					t.Errorf("passes started, log lines = %d, %q; want %d, %q", len(h.children), h.logged, len(wantLogged), wantLogged)
				}
			})
		}
	})

	t.Run("the dobby's live child holds the start, registered or not, and a later tick after its exit starts the next", func(t *testing.T) {
		t.Parallel()
		h := newTestMapHarness(t)
		h.mustStep()
		h.mustStep()
		if len(h.children) != 1 {
			t.Fatalf("passes started = %d while the first is the dobby's live, unregistered child; want 1", len(h.children))
		}
		h.exitLast(PassExit{})
		h.mustStep()
		if len(h.children) != 1 {
			t.Fatalf("passes started = %d on the tick that read the exit; want 1", len(h.children))
		}
		h.mustStep()
		if len(h.children) != 2 {
			t.Errorf("passes started = %d on the tick after the exit; want 2", len(h.children))
		}
	})

	t.Run("a failed exit is logged once per episode, and a clean exit ends the episode", func(t *testing.T) {
		t.Parallel()
		h := newTestMapHarness(t)
		failed := PassExit{Code: 1, Error: "gate testmap: cannot register the upkeep pass\nso it plans nothing"}
		h.mustStep()
		h.exitLast(failed)
		h.mustStep()
		h.mustStep()
		h.exitLast(failed)
		h.mustStep()
		h.mustStep()
		h.exitLast(PassExit{})
		h.mustStep()
		started := "upkeep pass started: no exit record"
		want := []string{
			started, "upkeep pass failing: exit 1: gate testmap: cannot register the upkeep pass so it plans nothing",
			started, started, "upkeep pass exits cleanly again",
		}
		if !slices.Equal(h.logged, want) {
			t.Errorf("log lines = %q; want %q", h.logged, want)
		}
	})

	t.Run("a failing start or read returns its wrapped error and starts none", func(t *testing.T) {
		t.Parallel()
		samePlanQueued := func(h *testMapHarness) { h.exitFound, h.exit = true, TestMapExit{PlannedHead: "h1", Queued: 1} }
		tests := []struct {
			name    string
			arrange func(h *testMapHarness)
			wantErr string
		}{
			{"the config does not load", func(h *testMapHarness) { h.configErr = errors.New("unreadable") }, "load beam.yaml: unreadable"},
			{"the liveness read fails", func(h *testMapHarness) { h.runningErr = errors.New("unreadable") }, "read whether an upkeep pass runs: unreadable"},
			{"the exit record read fails", func(h *testMapHarness) { h.exitErr = errors.New("unreadable") }, "read the upkeep exit record: unreadable"},
			{"the HEAD read fails", func(h *testMapHarness) {
				samePlanQueued(h)
				h.headErr = errors.New("no repository")
			}, "read the prime's HEAD: no repository"},
			{"the slot read fails", func(h *testMapHarness) {
				samePlanQueued(h)
				h.slotsErr = errors.New("no gate config")
			}, "read the hub's gate slots: no gate config"},
			{"the start fails", func(h *testMapHarness) { h.startErr = errors.New("no binary") }, "start the upkeep pass: no binary"},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				h := newTestMapHarness(t)
				tt.arrange(h)
				err := h.d.testMapStep(h.now)
				if err == nil || err.Error() != tt.wantErr {
					t.Fatalf("testMapStep() = %v; want %q", err, tt.wantErr)
				}
				if len(h.children) != 0 || len(h.logged) != 0 {
					t.Errorf("passes started, log lines = %d, %q; want none", len(h.children), h.logged)
				}
			})
		}
	})
}

// TestRun_AFailingTestMapStartLeavesTheRestOfTheTick drives ticks whose upkeep start fails.
// The tick's later steps still run, shown by the Tick hook that ends it, and each tick tries the start again.
func TestRun_AFailingTestMapStartLeavesTheRestOfTheTick(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	h := newLoopHarness(t, 3)
	starts := 0
	h.deps.LoadConfig = func() (Config, error) { return Config{TestMapUpkeep: true}, nil }
	h.deps.TestMapRunning = func() (bool, error) { return false, nil }
	h.deps.TestMapExitRecord = func() (TestMapExit, bool, error) { return TestMapExit{}, false, nil }
	h.deps.StartTestMap = func() (<-chan PassExit, error) {
		starts++
		// Ends the run once the start was tried on three ticks, so a tick that returns early fails the test at once.
		if starts == 3 {
			h.cancel()
		}
		return nil, errors.New("no binary")
	}

	if err := h.run(t); err != nil {
		t.Fatalf("Run() = %v; want nil", err)
	}
	if len(h.ticks) != 3 {
		t.Errorf("ticks that reached the Tick hook = %d; want 3", len(h.ticks))
	}
	if starts != 3 {
		t.Errorf("starts tried = %d; want one on each tick", starts)
	}
}
