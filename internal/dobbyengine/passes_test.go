//go:build tier0

// passes_test.go pins the pass planner and the scheduling of passes over fake runs, a fake pass spawner and a temp state record.

package dobbyengine

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPlanPass(t *testing.T) {
	t.Parallel()
	exited := time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)
	running := RunView{Slug: "task", State: runStateRunning, Producer: producerRunShed, StatusModified: exited}
	probe := passTrigger{ProbeTick: true}

	tests := []struct {
		name    string
		mutate  func(run *RunView, rs *RunState)
		trigger passTrigger
		covered bool
		want    passKind
	}{
		{"RunningAtRunShedGivesContinue", nil, probe, false, passContinue},
		{"OwedGivesFull", func(_ *RunView, rs *RunState) { rs.FullPassOwed = true }, probe, false, passFull},
		{"OriginOwedGivesFullWithNoReopen", func(run *RunView, _ *RunState) { run.OriginOwed = true }, probe, false, passFullNoReopen},
		{"AnotherProducerGivesFull", func(run *RunView, _ *RunState) { run.Producer = "Land" }, probe, false, passFull},
		{"HaltedGivesNone", func(run *RunView, _ *RunState) { run.State = "blocked" }, probe, false, passNone},
		{"CancelMarkedGivesNone", func(run *RunView, _ *RunState) { run.CancelMarker = true }, probe, false, passNone},
		{"InFlightGivesNone", nil, passTrigger{ProbeTick: true, InFlight: true}, false, passNone},
		{"HeldDriverLockReadsAsInFlight", func(run *RunView, _ *RunState) { run.DriverLockHeld = true }, probe, false, passNone},
		{"OwnedElsewhereUncoveredProbeGivesFull", func(_ *RunView, rs *RunState) { rs.OwnedElsewhere = true }, probe, false, passFull},
		{"OwnedElsewhereCoveredProbeGivesNone", func(_ *RunView, rs *RunState) { rs.OwnedElsewhere = true }, probe, true, passNone},
		{"OwnedElsewhereCoveredStartGivesNone", func(_ *RunView, rs *RunState) { rs.OwnedElsewhere = true }, passTrigger{Start: true}, true, passNone},
		{"OwnedElsewhereCoveredLiftGivesFull", func(_ *RunView, rs *RunState) { rs.OwnedElsewhere = true }, passTrigger{Lift: true}, true, passFull},
		{"NoTriggerGivesNone", nil, passTrigger{}, false, passNone},
		{"BattenWriteNoLaterThanTheLastExitGivesNone", func(_ *RunView, rs *RunState) { rs.LastPassExit = exited }, passTrigger{}, false, passNone},
		{"BattenWriteLaterThanTheLastExitGivesContinue", func(_ *RunView, rs *RunState) { rs.LastPassExit = exited.Add(-time.Second) }, passTrigger{}, false, passContinue},
		{"CoveredProbeGivesContinue", nil, probe, true, passContinue},
		{"CoveredOwedFullWaits", func(_ *RunView, rs *RunState) { rs.FullPassOwed = true }, probe, true, passNone},
		{"CoveredLiftGivesFull", func(_ *RunView, rs *RunState) { rs.FullPassOwed = true }, passTrigger{Lift: true}, true, passFull},
		{"LiftGivesFull", nil, passTrigger{Lift: true}, false, passFull},
		{"LiftWithOriginOwedGivesFullWithNoReopen", func(run *RunView, _ *RunState) { run.OriginOwed = true }, passTrigger{Lift: true}, false, passFullNoReopen},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			run, rs := running, RunState{LastPassExit: exited}
			if tt.mutate != nil {
				tt.mutate(&run, &rs)
			}
			if got := planPass(run, rs, tt.trigger, tt.covered); got != tt.want {
				t.Errorf("planPass() = %v; want %v", got, tt.want)
			}
		})
	}
}

// passFake is the runs listing, the pass spawner and the notice seam of one scheduling test.
type passFake struct {
	runs    []RunView
	spawned []spawnedPass
	exits   []PassExit
	// open holds the exit channel of a pass held in flight, by slug, when hold is set.
	open    map[string]chan PassExit
	hold    bool
	notices []string

	// pauses, releases and lifts count the owner pause seams' calls; pauseErr fails the pause, and a pause that stands covers the first run.
	pauses, releases, lifts int
	pauseErr                error
	// owner is what the local Bolt reads as the run's owner.
	owner string
	// subject is what the local Bolt reads as the run's adopt subject, and subjectErr fails that read.
	subject    string
	subjectErr error

	// tasks is the local board, maxRuns the beam.yaml cap, statuses the slugs a first pass left a status file for and covers the slugs a pause covers.
	tasks    []TaskView
	maxRuns  int
	statuses map[string]bool
	covers   map[string]bool
}

type spawnedPass struct {
	slug         string
	continuePass bool
	noReopen     bool
}

func (f *passFake) spawn(slug string, continuePass, noReopen bool) (<-chan PassExit, error) {
	f.spawned = append(f.spawned, spawnedPass{slug, continuePass, noReopen})
	exit := make(chan PassExit, 1)
	if f.hold {
		f.open[slug] = exit
		return exit, nil
	}
	next := PassExit{}
	if len(f.exits) > 0 {
		next, f.exits = f.exits[0], f.exits[1:]
	}
	exit <- next
	return exit, nil
}

// spawnsOf returns the passes spawned for slug.
func (f *passFake) spawnsOf(slug string) []spawnedPass {
	var out []spawnedPass
	for _, s := range f.spawned {
		if s.slug == slug {
			out = append(out, s)
		}
	}
	return out
}

// passHarness drives a daemon's ticks over a passFake.
type passHarness struct {
	t     *testing.T
	d     *daemon
	clock *loopClock
	fake  *passFake
}

func newPassHarness(t *testing.T, runs ...RunView) *passHarness {
	t.Helper()
	fake := &passFake{runs: runs, open: map[string]chan PassExit{}, maxRuns: 3, statuses: map[string]bool{}, covers: map[string]bool{}}
	clock := newLoopClock()
	h := &passHarness{t: t, clock: clock, fake: fake}
	h.d = &daemon{
		geom: Geometry{StatePath: filepath.Join(t.TempDir(), StateFileName), WorktreeName: "prime"},
		deps: Deps{
			Now: clock.now, Awake: clock.awake, Sleep: clock.sleep,
			Runs:      func() ([]RunView, error) { return append([]RunView(nil), fake.runs...), nil },
			SpawnPass: fake.spawn,
			Notice: func(_ context.Context, line string) error {
				fake.notices = append(fake.notices, line)
				return nil
			},
			Machine:      func() (string, error) { return "m1", nil },
			Tasks:        func() ([]TaskView, error) { return append([]TaskView(nil), fake.tasks...), nil },
			LoadConfig:   func() (Config, error) { return Config{MaxRuns: fake.maxRuns}, nil },
			StatusExists: func(slug string) (bool, error) { return fake.statuses[slug], nil },
			PauseCovers:  func(slug string) (bool, error) { return fake.covers[slug], nil },
			PauseOwned: func(context.Context, string) error {
				fake.pauses++
				if fake.pauseErr == nil {
					fake.runs[0].Covered = true
				}
				return fake.pauseErr
			},
			ReleaseParentLock: func(context.Context, string) error { fake.releases++; return nil },
			SlugOwner:         func(string) (string, error) { return fake.owner, nil },
			AdoptSubject:      func(string) (string, error) { return fake.subject, fake.subjectErr },
			LiftOwnerPause: func(context.Context, string) error {
				fake.lifts++
				fake.runs[0].Covered = false
				return nil
			},
		},
		probe:   10 * time.Second,
		flights: map[string]flight{}, childSeen: map[string]time.Time{}, wasCovered: map[string]bool{}, liftPending: map[string]bool{},
	}
	// A record with a pid is a dobby already started, so a tick finds no start to owe passes for.
	if err := UpdateState(h.d.geom.StatePath, func(s *State) error { s.PID = 1; return nil }); err != nil {
		t.Fatalf("UpdateState() = %v; want nil", err)
	}
	return h
}

// tick advances the clock one probe interval and runs one tick, a probe tick when probe is set.
func (h *passHarness) tick(probe bool) {
	h.t.Helper()
	h.clock.sleep(context.Background(), 10*time.Second)
	h.d.probeDue = probe
	if err := h.d.tick(context.Background(), h.clock.now()); err != nil {
		h.t.Fatalf("tick() = %v; want nil", err)
	}
}

func (h *passHarness) state() RunState {
	h.t.Helper()
	return h.stateOf("task")
}

func (h *passHarness) stateOf(slug string) RunState {
	h.t.Helper()
	return h.stateRecord().Runs[slug]
}

func (h *passHarness) stateRecord() State {
	h.t.Helper()
	record, _, err := ReadState(h.d.geom.StatePath)
	if err != nil {
		h.t.Fatalf("ReadState() = %v; want nil", err)
	}
	return record
}

func TestLog_OneLinePerStartAndPass(t *testing.T) {
	t.Parallel()
	owed := runningRun("o")
	owed.OriginOwed = true
	h := newPassHarness(t, runningRun("r"), runningRun("c"), owed)
	h.fake.maxRuns = 4
	var lines []string
	h.d.deps.Log = func(line string) { lines = append(lines, line) }
	h.fake.tasks = []TaskView{readyTask("a")}
	if err := UpdateState(h.d.geom.StatePath, func(s *State) error {
		s.Runs = map[string]RunState{"r": {FullPassOwed: true}, "o": {FullPassOwed: true}}
		return nil
	}); err != nil {
		t.Fatalf("UpdateState() = %v; want nil", err)
	}

	h.tick(true)

	want := []string{
		"2026-10-25T12:00:10Z pass r full",
		"2026-10-25T12:00:10Z pass c continue",
		"2026-10-25T12:00:10Z pass o full-no-reopen",
		"2026-10-25T12:00:10Z start a",
	}
	if strings.Join(lines, "\n") != strings.Join(want, "\n") {
		t.Errorf("log lines = %q; want %q", lines, want)
	}
}

func runningRun(slug string) RunView {
	return RunView{Slug: slug, State: runStateRunning, Producer: producerRunShed}
}

func TestDrivePasses_ExitHandling(t *testing.T) {
	t.Parallel()

	t.Run("a dobby start owes every listed run a full pass and a re-exec owes none", func(t *testing.T) {
		t.Parallel()
		started := newPassHarness(t, runningRun("a"), runningRun("b"))
		started.d.owesAll = true
		started.tick(false)
		if got := started.fake.spawned; len(got) != 2 || got[0].continuePass || got[1].continuePass {
			t.Errorf("passes after a start = %+v; want a full pass for each of the two runs", got)
		}

		reexec := newPassHarness(t, runningRun("a"), runningRun("b"))
		reexec.tick(true)
		if got := reexec.fake.spawned; len(got) != 2 || !got[0].continuePass || !got[1].continuePass {
			t.Errorf("passes after a re-exec = %+v; want a continue pass for each of the two runs", got)
		}
		if reexec.stateOf("a").FullPassOwed {
			t.Errorf("a re-exec owes a full pass; want none")
		}
	})

	t.Run("a recorded sleep owes every run a full pass", func(t *testing.T) {
		t.Parallel()
		h := newPassHarness(t, runningRun("task"))
		h.d.owesAll = true
		h.tick(true)
		h.tick(true)
		h.clock.suspend(5 * time.Minute)
		h.tick(true)
		h.tick(true)
		spawned := h.fake.spawnsOf("task")
		if len(spawned) < 3 || spawned[0].continuePass || !spawned[1].continuePass || spawned[2].continuePass {
			t.Errorf("passes = %+v; want a full pass at the start, a continue pass, and a full pass once the sleep is seen", spawned)
		}
	})

	t.Run("a refused continue pass owes a full pass and counts no failure", func(t *testing.T) {
		t.Parallel()
		h := newPassHarness(t, runningRun("task"))
		h.fake.exits = []PassExit{{Code: PassExitContinueRefused, Error: "not a run dobby may continue"}}
		h.tick(true)
		h.tick(true)
		spawned := h.fake.spawnsOf("task")
		if len(spawned) != 2 || !spawned[0].continuePass || spawned[1].continuePass {
			t.Errorf("passes = %+v; want a continue pass and then a full pass", spawned)
		}
		if rs := h.state(); rs.FailingPassCount != 0 || len(h.fake.notices) != 0 {
			t.Errorf("failures, notices = %d, %d; want none", rs.FailingPassCount, len(h.fake.notices))
		}
	})

	t.Run("a full pass that exits 0 while the origin-owed marker stands stays owed", func(t *testing.T) {
		t.Parallel()
		run := runningRun("task")
		run.OriginOwed = true
		h := newPassHarness(t, run)
		h.d.owesAll = true
		h.tick(true)
		h.tick(true)
		if !h.state().FullPassOwed {
			t.Errorf("FullPassOwed = false after a full pass with the marker standing; want true")
		}
		if first := h.fake.spawnsOf("task")[0]; first.continuePass || !first.noReopen {
			t.Errorf("first pass = %+v; want a full pass with no reopen", first)
		}

		h.fake.runs[0].OriginOwed = false
		h.tick(true)
		h.tick(true)
		if h.state().FullPassOwed {
			t.Errorf("FullPassOwed = true after a full pass exited 0 with no marker; want it cleared")
		}
	})

	t.Run("a failed full pass is retried as a full pass on the next probe tick", func(t *testing.T) {
		t.Parallel()
		h := newPassHarness(t, runningRun("task"))
		h.d.owesAll = true
		h.fake.exits = []PassExit{{Code: 1, Error: "boom"}}
		h.tick(false)
		h.tick(true)
		spawned := h.fake.spawnsOf("task")
		if len(spawned) != 2 || spawned[1].continuePass {
			t.Errorf("passes = %+v; want the retry to be a full pass", spawned)
		}
	})

	t.Run("a covered exit counts no failure and sends no notice", func(t *testing.T) {
		t.Parallel()
		h := newPassHarness(t, runningRun("task"))
		h.fake.exits = []PassExit{{Code: PassExitCovered}, {Code: PassExitCovered}, {Code: PassExitCovered}, {Code: PassExitCovered}}
		for range 5 {
			h.tick(true)
		}
		if rs := h.state(); rs.FailingPassCount != 0 || len(h.fake.notices) != 0 {
			t.Errorf("failures, notices = %d, %d; want none", rs.FailingPassCount, len(h.fake.notices))
		}
	})

	t.Run("three failures with one text send one notice, a fourth none, and an exit 0 clears the count and the noticed text", func(t *testing.T) {
		t.Parallel()
		h := newPassHarness(t, runningRun("task"))
		failing := PassExit{Code: 1, Error: "pre-run refused"}
		h.fake.exits = []PassExit{failing, failing, failing, failing, failing}
		for range 3 {
			h.tick(true)
		}
		if len(h.fake.notices) != 0 {
			t.Fatalf("notices after two failures = %d; want 0", len(h.fake.notices))
		}
		h.tick(true)
		if len(h.fake.notices) != 1 || !strings.Contains(h.fake.notices[0], "task: batten pass keeps failing: pre-run refused; way forward:") {
			t.Fatalf("notices after three failures = %q; want one naming the run and the text", h.fake.notices)
		}
		h.tick(true)
		h.tick(true)
		if len(h.fake.notices) != 1 {
			t.Errorf("notices after a fourth failure = %d; want still 1", len(h.fake.notices))
		}

		h.fake.exits = nil
		h.tick(true)
		h.tick(true)
		if rs := h.state(); rs.FailingPassCount != 0 || rs.FailingPassError != "" || len(rs.Noticed) != 0 {
			t.Errorf("state after an exit 0 = %+v; want the count and the noticed texts cleared", rs)
		}
	})

	t.Run("a run dropped from the listing is no longer passed", func(t *testing.T) {
		t.Parallel()
		h := newPassHarness(t, runningRun("kept"), runningRun("dropped"))
		h.d.owesAll = true
		h.tick(true)
		h.fake.runs = h.fake.runs[:1]
		h.tick(true)
		h.tick(true)
		if got := len(h.fake.spawnsOf("dropped")); got != 1 {
			t.Errorf("passes of the dropped run = %d; want only the one before it was dropped", got)
		}
		if record, _, err := ReadState(h.d.geom.StatePath); err != nil {
			t.Fatalf("ReadState() = %v; want nil", err)
		} else if _, found := record.Runs["dropped"]; found {
			t.Errorf("state still holds the dropped run: %+v", record.Runs)
		}
	})

	t.Run("a dobby whose runs are all halted is idle for the backoff and one with a passable run is busy", func(t *testing.T) {
		t.Parallel()
		halted := runningRun("task")
		halted.State = "blocked"
		idle := newPassHarness(t, halted)
		idle.tick(true)
		if idle.d.inFlight {
			t.Errorf("inFlight = true with every run halted; want false")
		}

		busy := newPassHarness(t, runningRun("task"))
		busy.fake.hold = true
		busy.tick(true)
		if !busy.d.inFlight {
			t.Errorf("inFlight = false with a pass in flight; want true")
		}
	})

	t.Run("a lift while a pass is in flight gives the run its pass once that pass exits", func(t *testing.T) {
		t.Parallel()
		h := newPassHarness(t, runningRun("task"))
		h.fake.hold = true
		h.tick(true)
		h.fake.runs[0].Covered = true
		h.tick(false)
		h.fake.runs[0].Covered = false
		h.tick(false)
		if got := len(h.fake.spawned); got != 1 {
			t.Fatalf("passes with one in flight across a lift = %d; want 1", got)
		}

		h.fake.open["task"] <- PassExit{}
		h.fake.hold = false
		h.tick(false)
		if got := len(h.fake.spawned); got != 2 {
			t.Errorf("passes after the in-flight pass exited = %d; want the lift's pass, 2", got)
		}
		if got := h.fake.spawned[1]; got.continuePass {
			t.Errorf("the lift's pass = %+v; want a full pass", got)
		}
	})

	t.Run("a lift seen while the run is not running gives no pass once it runs again", func(t *testing.T) {
		t.Parallel()
		h := newPassHarness(t, runningRun("task"))
		h.tick(true)
		h.fake.runs[0].Covered = true
		h.tick(false)
		h.fake.runs[0].Covered = false
		h.fake.runs[0].State = "blocked"
		h.tick(false)
		h.fake.runs[0].State = runStateRunning
		h.tick(true)
		spawned := h.fake.spawnsOf("task")
		if len(spawned) != 2 || !spawned[1].continuePass {
			t.Errorf("passes = %+v; want a continue pass at the first probe tick and a continue pass after the run runs again", spawned)
		}
	})
}
