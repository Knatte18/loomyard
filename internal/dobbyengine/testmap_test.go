// testmap_test.go pins the prime dobby's test-map refresh step over fake seams and a temp state record:
// the landing count, the start every few landings, the hold while a refresh is live, a failed start or read,
// and that a failing step leaves the rest of the tick running.

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

// testMapBatch is the test_map_batch the harness's config resolves.
const testMapBatch = 3

// testMapHarness drives a daemon's landing count and test-map step over fakes.
type testMapHarness struct {
	t      *testing.T
	d      *daemon
	landed []string
	every  int
	// running is what TestMapRunning reads; startErr is what StartTestMap returns.
	running  bool
	startErr error
	// configErr and runningErr, when set, are what LoadConfig and TestMapRunning return.
	configErr  error
	runningErr error
	// started holds the --max of each refresh started.
	started []int
	// logged holds the dobby log lines written, without their timestamps.
	logged []string
	now    time.Time
}

func newTestMapHarness(t *testing.T, every int) *testMapHarness {
	t.Helper()
	h := &testMapHarness{t: t, every: every, now: time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)}
	h.d = h.daemonOver(filepath.Join(t.TempDir(), StateFileName))
	return h
}

// daemonOver builds a daemon over a state path, as a restarted dobby would build one over the same record.
func (h *testMapHarness) daemonOver(statePath string) *daemon {
	return &daemon{
		geom: Geometry{WorktreeName: "prime", StatePath: statePath},
		deps: Deps{
			LoadConfig: func() (Config, error) {
				if h.configErr != nil {
					return Config{}, h.configErr
				}
				return Config{TestMapEvery: h.every, TestMapBatch: testMapBatch}, nil
			},
			Landings:       func() ([]string, error) { return slices.Clone(h.landed), nil },
			TestMapRunning: func() (bool, error) { return h.running, h.runningErr },
			StartTestMap: func(maxPackages int) error {
				if h.startErr != nil {
					return h.startErr
				}
				h.started = append(h.started, maxPackages)
				return nil
			},
			Log: func(_, line string, _ bool) {
				_, text, _ := strings.Cut(line, " ")
				h.logged = append(h.logged, text)
			},
		},
	}
}

// step runs the landing count and the test-map step at the harness's clock and returns the step's error.
func (h *testMapHarness) step() error {
	h.t.Helper()
	if err := h.d.countLandings(h.now); err != nil {
		h.t.Fatalf("countLandings() = %v; want nil", err)
	}
	return h.d.testMapStep(h.now)
}

// mustStep runs step and fails the test on its error.
func (h *testMapHarness) mustStep() {
	h.t.Helper()
	if err := h.step(); err != nil {
		h.t.Fatalf("testMapStep() = %v; want nil", err)
	}
}

// setTidy records a board-tidy agent in flight in the state record, or clears it when tidy is nil.
func (h *testMapHarness) setTidy(tidy *TidyState) {
	h.t.Helper()
	err := UpdateState(h.d.geom.StatePath, func(s *State) error {
		s.Tidy = tidy
		return nil
	})
	if err != nil {
		h.t.Fatalf("UpdateState() = %v; want nil", err)
	}
}

func (h *testMapHarness) count() int {
	h.t.Helper()
	current, _, err := ReadState(h.d.geom.StatePath)
	if err != nil {
		h.t.Fatalf("ReadState() = %v; want nil", err)
	}
	return current.LandingsSinceTestMap
}

func TestTestMapStep(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	t.Run("every test_map_every landings start one refresh capped at test_map_batch", func(t *testing.T) {
		t.Parallel()
		h := newTestMapHarness(t, 2)
		h.landed = []string{"old"}
		h.mustStep()
		h.landed = []string{"old", "a"}
		h.mustStep()
		if len(h.started) != 0 || h.count() != 1 {
			t.Fatalf("started, count = %v, %d after one landing; want none started and a count of 1", h.started, h.count())
		}
		h.landed = []string{"old", "a", "b"}
		h.mustStep()
		if !slices.Equal(h.started, []int{testMapBatch}) || h.count() != 0 {
			t.Fatalf("started, count = %v, %d after two landings; want [%d] and the count reset", h.started, h.count(), testMapBatch)
		}
		if want := "test-map refresh started after 2 landings, at most 3 packages"; !slices.Equal(h.logged, []string{want}) {
			t.Errorf("log lines = %q; want the one %q", h.logged, want)
		}
		h.landed = []string{"old", "a", "b", "c", "d"}
		h.mustStep()
		if !slices.Equal(h.started, []int{testMapBatch, testMapBatch}) {
			t.Errorf("started = %v after two more landings; want a second refresh", h.started)
		}
	})

	t.Run("test_map_every 0 starts none and keeps no count, so enabling it later starts none on old landings", func(t *testing.T) {
		t.Parallel()
		h := newTestMapHarness(t, 0)
		h.mustStep()
		h.landed = []string{"a", "b", "c"}
		h.mustStep()
		if len(h.started) != 0 || h.count() != 0 {
			t.Fatalf("started, count = %v, %d with the key at 0; want none started and a count of 0", h.started, h.count())
		}
		h.every = 2
		h.mustStep()
		if len(h.started) != 0 {
			t.Errorf("started = %v after the key was enabled; want none on the landings counted while it was off", h.started)
		}
	})

	t.Run("a live refresh holds the start across a restart, and the next tick after it ended starts one", func(t *testing.T) {
		t.Parallel()
		h := newTestMapHarness(t, 2)
		h.running = true
		h.mustStep()
		h.landed = []string{"a", "b"}
		h.mustStep()
		h.d = h.daemonOver(h.d.geom.StatePath)
		h.mustStep()
		if len(h.started) != 0 || h.count() != 2 {
			t.Fatalf("started, count = %v, %d while a refresh is live; want none started and the count kept", h.started, h.count())
		}
		h.running = false
		h.mustStep()
		if !slices.Equal(h.started, []int{testMapBatch}) || h.count() != 0 {
			t.Errorf("started, count = %v, %d once the refresh ended; want [%d] and the count reset", h.started, h.count(), testMapBatch)
		}
	})

	t.Run("a start that fails keeps the count and the next tick starts it", func(t *testing.T) {
		t.Parallel()
		h := newTestMapHarness(t, 1)
		h.mustStep()
		h.landed = []string{"a"}
		h.startErr = errors.New("no binary")
		if err := h.step(); err == nil || !strings.Contains(err.Error(), "start the test-map refresh: no binary") {
			t.Fatalf("testMapStep() = %v; want the start's error", err)
		}
		if h.count() != 1 || len(h.logged) != 0 {
			t.Fatalf("count, log lines = %d, %q after a failed start; want the count kept and no line", h.count(), h.logged)
		}
		h.startErr = nil
		h.mustStep()
		if !slices.Equal(h.started, []int{testMapBatch}) {
			t.Errorf("started = %v; want the retried refresh", h.started)
		}
	})

	t.Run("a failing read returns its wrapped error, keeps the count and starts none", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name    string
			arrange func(h *testMapHarness)
			wantErr string
		}{
			{"the config does not load", func(h *testMapHarness) { h.configErr = errors.New("unreadable") }, "load beam.yaml: unreadable"},
			{"the liveness read fails", func(h *testMapHarness) { h.runningErr = errors.New("unreadable") }, "read whether a test-map refresh runs: unreadable"},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				h := newTestMapHarness(t, 1)
				h.mustStep()
				h.landed = []string{"a"}
				tt.arrange(h)
				if err := h.step(); err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("testMapStep() = %v; want an error holding %q", err, tt.wantErr)
				}
				if len(h.started) != 0 || h.count() != 1 {
					t.Errorf("started, count = %v, %d after the failed read; want none started and the count kept", h.started, h.count())
				}
			})
		}
	})

	t.Run("a board-tidy agent in flight counts no landing for the refresh, and the landing counts once it ended", func(t *testing.T) {
		t.Parallel()
		h := newTestMapHarness(t, 5)
		h.mustStep()
		h.setTidy(&TidyState{StrandGUID: "agent"})
		h.landed = []string{"a"}
		h.mustStep()
		if h.count() != 0 {
			t.Fatalf("count = %d while a tidy agent is in flight; want 0", h.count())
		}
		h.setTidy(nil)
		h.mustStep()
		if h.count() != 1 {
			t.Errorf("count = %d once the tidy agent ended; want the landing counted", h.count())
		}
	})

	t.Run("a dobby without the refresh seams counts no landing for it and starts none", func(t *testing.T) {
		t.Parallel()
		h := newTestMapHarness(t, 1)
		h.d.deps.StartTestMap, h.d.deps.TestMapRunning = nil, nil
		h.mustStep()
		h.landed = []string{"a", "b"}
		h.mustStep()
		if h.count() != 0 {
			t.Errorf("count = %d; want 0 with the refresh off", h.count())
		}
	})
}

// TestRun_AFailingTestMapStartLeavesTheRestOfTheTick drives ticks whose test-map start fails.
// The tick's later steps still run, shown by the Tick hook that ends it, and each tick tries the start again.
func TestRun_AFailingTestMapStartLeavesTheRestOfTheTick(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	h := newLoopHarness(t, 3)
	landingReads, starts := 0, 0
	h.deps.LoadConfig = func() (Config, error) { return Config{TestMapEvery: 1, TestMapBatch: testMapBatch}, nil }
	// The first read baselines the landings, so the one landing shows from the second tick on.
	h.deps.Landings = func() ([]string, error) {
		landingReads++
		if landingReads == 1 {
			return nil, nil
		}
		return []string{"landed"}, nil
	}
	h.deps.TestMapRunning = func() (bool, error) { return false, nil }
	h.deps.StartTestMap = func(int) error {
		starts++
		// Ends the run once the start was tried twice, so a tick that returns early fails the test at once.
		if starts == 2 {
			h.cancel()
		}
		return errors.New("no binary")
	}

	if err := h.run(t); err != nil {
		t.Fatalf("Run() = %v; want nil", err)
	}
	if len(h.ticks) != 3 {
		t.Errorf("ticks that reached the Tick hook = %d; want 3", len(h.ticks))
	}
	if starts != 2 {
		t.Errorf("starts tried = %d; want one on each tick after the landing was counted", starts)
	}
}
