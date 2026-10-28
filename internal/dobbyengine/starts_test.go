//go:build tier0

// starts_test.go pins the start condition of a board task and the start step's cap, task-limit hold, refusal notices and backoff.

package dobbyengine

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/proc"
)

func TestReadyToStart(t *testing.T) {
	t.Parallel()
	draft := "draft"
	empty := ""
	ready := TaskView{Slug: "task", Kind: "task", Status: &empty, Parent: "prime"}

	tests := []struct {
		name    string
		mutate  func(*TaskView)
		covered bool
		want    bool
	}{
		{"EmptyStatusWithDoneDependenciesStarts", nil, false, true},
		{"AbsentStatusStarts", func(task *TaskView) { task.Status = nil }, false, true},
		{"DraftDoesNotStart", func(task *TaskView) { task.Status = &draft }, false, false},
		{"OpenDependencyDoesNotStart", func(task *TaskView) { task.PendingDependencies = []string{"dep"} }, false, false},
		{"CoveringPauseDoesNotStart", nil, true, false},
		{"OwnedElsewhereDoesNotStart", func(task *TaskView) { task.OwnedElsewhere = true }, false, false},
		{"AnotherBeamsTaskDoesNotStart", func(task *TaskView) { task.Parent = "other-beam" }, false, false},
		{"AProjectDoesNotStart", func(task *TaskView) { task.Kind = "project" }, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			task := ready
			if tt.mutate != nil {
				tt.mutate(&task)
			}
			if got := readyToStart(task, "prime", tt.covered); got != tt.want {
				t.Errorf("readyToStart() = %v; want %v", got, tt.want)
			}
		})
	}
}

func TestNextStartTry(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)
	probe := 10 * time.Second

	first := nextStartTry(StartState{}, now, probe)
	second := nextStartTry(first, now, probe)
	capped := nextStartTry(StartState{Interval: 40 * time.Minute}, now, probe)
	if first.Interval != probe || !first.NextTry.Equal(now.Add(probe)) {
		t.Errorf("first retry = %+v; want one probe interval", first)
	}
	if second.Interval != 2*probe {
		t.Errorf("second retry interval = %v; want twice the probe interval", second.Interval)
	}
	if capped.Interval != time.Hour {
		t.Errorf("capped retry interval = %v; want an hour", capped.Interval)
	}
}

// readyTask is a board entry that is ready to start under the prime's dobby.
func readyTask(slug string) TaskView {
	empty := ""
	return TaskView{Slug: slug, Kind: "task", Status: &empty, Parent: "prime", Fingerprint: slug + "-v1"}
}

func TestStartReady_RefusalsAndBackoff(t *testing.T) {
	t.Parallel()
	refused := func(text string) PassExit { return PassExit{Code: 1, Error: text} }

	t.Run("a fourth ready task past max_runs waits", func(t *testing.T) {
		t.Parallel()
		h := newPassHarness(t)
		h.fake.tasks = []TaskView{readyTask("a"), readyTask("b"), readyTask("c"), readyTask("d")}
		h.tick(true)
		if got := len(h.fake.spawned); got != 3 || len(h.fake.spawnsOf("d")) != 0 {
			t.Errorf("first passes = %+v; want the first three tasks only", h.fake.spawned)
		}
	})

	t.Run("max_runs 0 starts nothing", func(t *testing.T) {
		t.Parallel()
		h := newPassHarness(t)
		h.fake.maxRuns = 0
		h.fake.tasks = []TaskView{readyTask("a")}
		h.tick(true)
		if len(h.fake.spawned) != 0 {
			t.Errorf("first passes = %+v; want none", h.fake.spawned)
		}
	})

	t.Run("a refused first pass notices once and retries after the probe interval, then after twice it, and at once after an entry change", func(t *testing.T) {
		t.Parallel()
		h := newPassHarness(t)
		h.fake.tasks = []TaskView{readyTask("a")}
		h.fake.exits = []PassExit{refused("pre-run refused"), refused("pre-run refused"), refused("pre-run refused")}

		h.tick(true)
		h.tick(true)
		if len(h.fake.spawned) != 1 || len(h.fake.notices) != 1 || h.fake.notices[0] != "a: dobby could not start the task: pre-run refused" {
			t.Fatalf("passes, notices after the first refusal = %d, %q; want 1 pass and the one notice", len(h.fake.spawned), h.fake.notices)
		}
		if got := h.stateRecord().Starts["a"].Refusal; got != "pre-run refused" {
			t.Errorf("start record refusal = %q; want the refusal text", got)
		}
		h.tick(true)
		if len(h.fake.spawned) != 2 {
			t.Fatalf("passes one probe interval after the refusal = %d; want the retry, 2", len(h.fake.spawned))
		}
		h.tick(true)
		h.tick(true)
		if len(h.fake.spawned) != 2 {
			t.Fatalf("passes inside the doubled wait = %d; want still 2", len(h.fake.spawned))
		}
		h.tick(true)
		if len(h.fake.spawned) != 3 {
			t.Fatalf("passes after the doubled wait = %d; want the third", len(h.fake.spawned))
		}
		if len(h.fake.notices) != 1 {
			t.Errorf("notices = %d; want the one text noticed once", len(h.fake.notices))
		}

		h.tick(true)
		h.fake.tasks[0].Fingerprint = "a-v2"
		h.tick(true)
		if len(h.fake.spawned) != 4 {
			t.Errorf("passes after the entry changed = %d; want one at once, 4", len(h.fake.spawned))
		}
	})

	t.Run("a retry whose pass leaves a status file clears the refusal text", func(t *testing.T) {
		t.Parallel()
		h := newPassHarness(t)
		h.fake.tasks = []TaskView{readyTask("a")}
		h.fake.exits = []PassExit{refused("pre-run refused"), {}}
		h.tick(true)
		h.tick(true)
		if got := h.stateRecord().Starts["a"].Refusal; got != "pre-run refused" {
			t.Fatalf("start record refusal = %q; want the refusal text before the retry", got)
		}
		h.fake.statuses = map[string]bool{"a": true}
		h.tick(true)
		h.tick(true)
		if got, found := h.stateRecord().Starts["a"]; found {
			t.Errorf("start record after the claiming pass = %+v; want none", got)
		}
	})

	t.Run("a covered first pass notices nothing and sets no backoff", func(t *testing.T) {
		t.Parallel()
		h := newPassHarness(t)
		h.fake.tasks = []TaskView{readyTask("a")}
		h.fake.exits = []PassExit{{Code: PassExitCovered}}
		h.tick(true)
		h.tick(true)
		if len(h.fake.notices) != 0 || len(h.fake.spawned) != 2 {
			t.Errorf("notices, passes = %d, %d; want no notice and an immediate second start", len(h.fake.notices), len(h.fake.spawned))
		}
	})

	t.Run("an owned-elsewhere first pass with no status file is a refused start with one notice and no pause", func(t *testing.T) {
		t.Parallel()
		h := newPassHarness(t)
		h.fake.tasks = []TaskView{readyTask("a")}
		h.fake.exits = []PassExit{{Code: PassExitOwnedElsewhere, Error: "owned by m2"}}
		h.tick(true)
		h.tick(true)
		if len(h.fake.notices) != 1 || h.fake.pauses != 0 {
			t.Errorf("notices, pauses = %d, %d; want 1, 0", len(h.fake.notices), h.fake.pauses)
		}
	})

	t.Run("a run marked owned elsewhere leaves a slot under max_runs free", func(t *testing.T) {
		t.Parallel()
		h := newPassHarness(t, runningRun("held"))
		h.fake.maxRuns = 1
		h.fake.tasks = []TaskView{readyTask("a")}
		h.tick(false)
		if len(h.fake.spawnsOf("a")) != 0 {
			t.Fatalf("a task started past max_runs with one run in flight")
		}

		err := UpdateState(h.d.geom.StatePath, func(s *State) error {
			s.Runs = map[string]RunState{"held": {OwnedElsewhere: true}}
			return nil
		})
		if err != nil {
			t.Fatalf("UpdateState() = %v; want nil", err)
		}
		h.tick(false)
		if len(h.fake.spawnsOf("a")) != 1 {
			t.Errorf("passes of the ready task = %d; want it started in the freed slot", len(h.fake.spawnsOf("a")))
		}
	})

	t.Run("a pawl run takes no slot until its landing hold stands", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name      string
			noSlot    bool
			wantStart int
		}{
			{"WithoutItsHoldItLeavesTheSlotFree", true, 1},
			{"WithItsHoldItFillsTheSlot", false, 0},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				pawl := runningRun("pawl")
				pawl.NoSlot = tt.noSlot
				h := newPassHarness(t, pawl)
				h.fake.maxRuns = 1
				h.fake.tasks = []TaskView{readyTask("a")}
				h.tick(false)
				if got := len(h.fake.spawnsOf("a")); got != tt.wantStart {
					t.Errorf("passes of the ready task = %d; want %d", got, tt.wantStart)
				}
			})
		}
	})

	t.Run("done runs take no slot, get no pass and leave the state record", func(t *testing.T) {
		t.Parallel()
		done := func(slug string) RunView {
			run := runningRun(slug)
			run.State = runStateDone
			return run
		}
		h := newPassHarness(t, done("x"), done("y"))
		h.fake.maxRuns = 1
		h.fake.tasks = []TaskView{readyTask("a")}
		h.d.owesAll = true
		err := UpdateState(h.d.geom.StatePath, func(s *State) error {
			s.Runs = map[string]RunState{"x": {FullPassOwed: true}}
			return nil
		})
		if err != nil {
			t.Fatalf("UpdateState() = %v; want nil", err)
		}
		h.tick(true)
		if got := h.fake.spawned; len(got) != 1 || got[0].slug != "a" {
			t.Errorf("passes = %+v; want only the first pass of the ready task", got)
		}
		if runs := h.stateRecord().Runs; len(runs) != 0 {
			t.Errorf("state runs = %+v; want no entry for a done run", runs)
		}
	})

	t.Run("a done run keeps its slot until its pass in flight exits", func(t *testing.T) {
		t.Parallel()
		h := newPassHarness(t, runningRun("r"))
		h.fake.maxRuns = 1
		h.fake.hold = true
		h.tick(true)
		h.fake.runs[0].State = runStateDone
		h.fake.tasks = []TaskView{readyTask("a")}
		h.tick(true)
		if len(h.fake.spawnsOf("a")) != 0 {
			t.Fatalf("a task started in the slot of a done run whose pass is in flight")
		}
		h.fake.open["r"] <- PassExit{}
		h.tick(true)
		if len(h.fake.spawnsOf("a")) != 1 {
			t.Errorf("passes of the ready task = %d; want it started once the done run's pass exited", len(h.fake.spawnsOf("a")))
		}
		if _, kept := h.stateRecord().Runs["r"]; kept {
			t.Errorf("the done run's state entry is kept after its pass exited; want it dropped")
		}
	})

	t.Run("two refusals with different texts send two notices, and a claim clears the noticed texts", func(t *testing.T) {
		t.Parallel()
		h := newPassHarness(t)
		h.fake.tasks = []TaskView{readyTask("a")}
		h.fake.exits = []PassExit{refused("first text"), refused("second text"), {}, refused("first text")}
		later := func() { h.clock.sleep(context.Background(), time.Hour) }

		h.tick(true)
		h.tick(true)
		later()
		h.tick(true)
		h.tick(true)
		if len(h.fake.notices) != 2 {
			t.Fatalf("notices after two different texts = %d; want 2", len(h.fake.notices))
		}

		later()
		h.fake.statuses["a"] = true
		h.tick(true)
		h.tick(true)
		h.fake.statuses["a"] = false
		h.tick(true)
		if len(h.fake.notices) != 3 {
			t.Errorf("notices after the claim and the same text again = %d; want it noticed again, 3", len(h.fake.notices))
		}
	})
}

func TestStartReady_TaskLimitHold(t *testing.T) {
	t.Parallel()
	near := proc.TaskLimit{Current: 2997, Max: 3000, Cgroup: "/user.slice/user-1000.slice"}
	free := proc.TaskLimit{Current: 1200, Max: 3000, Cgroup: "/user.slice/user-1000.slice"}

	t.Run("a start waits while the headroom is below task_headroom, logged once, and goes once it returns", func(t *testing.T) {
		t.Parallel()
		h := newPassHarness(t)
		lines := h.recordLog()
		h.fake.taskHeadroom = 500
		h.fake.limit, h.fake.limitFound = near, true
		h.fake.tasks = []TaskView{readyTask("a"), readyTask("b")}

		h.tick(true)
		h.tick(true)
		if len(h.fake.spawned) != 0 {
			t.Fatalf("first passes under the hold = %+v; want none", h.fake.spawned)
		}
		if starts := h.stateRecord().Starts; len(starts) != 0 {
			t.Errorf("start records under the hold = %+v; want none", starts)
		}
		want := []string{"hold starts: 2997 of 3000 tasks in /user.slice/user-1000.slice, below task_headroom 500"}
		if !slices.Equal(*lines, want) {
			t.Fatalf("log lines under the hold = %q; want %q", *lines, want)
		}

		h.fake.limit = free
		h.tick(true)
		if got := h.fake.spawned; len(got) != 2 {
			t.Errorf("first passes once the headroom returned = %+v; want both tasks", got)
		}
		want = append(want, "starts resume: 1200 of 3000 tasks in /user.slice/user-1000.slice", "start a", "start b")
		if !slices.Equal(*lines, want) {
			t.Errorf("log lines = %q; want %q", *lines, want)
		}
	})

	t.Run("a hold ended by task_headroom 0 logs the resume without a reading", func(t *testing.T) {
		t.Parallel()
		h := newPassHarness(t)
		lines := h.recordLog()
		h.fake.taskHeadroom = 500
		h.fake.limit, h.fake.limitFound = near, true
		h.fake.tasks = []TaskView{readyTask("a")}
		h.tick(true)
		h.fake.taskHeadroom = 0
		h.tick(true)
		want := []string{"hold starts: 2997 of 3000 tasks in /user.slice/user-1000.slice, below task_headroom 500", "starts resume: no task limit applies", "start a"}
		if !slices.Equal(*lines, want) {
			t.Errorf("log lines = %q; want %q", *lines, want)
		}
	})

	t.Run("the limit is read right before each start, so a reading that turns near holds the next task", func(t *testing.T) {
		t.Parallel()
		h := newPassHarness(t)
		lines := h.recordLog()
		h.fake.taskHeadroom = 500
		h.fake.tasks = []TaskView{readyTask("a"), readyTask("b")}
		reads := 0
		h.d.deps.TaskLimit = func() (proc.TaskLimit, bool) {
			reads++
			if reads == 1 {
				return free, true
			}
			return near, true
		}

		h.tick(true)
		if got := h.fake.spawned; len(got) != 1 || got[0].slug != "a" {
			t.Errorf("first passes = %+v; want only the first task", got)
		}
		want := []string{"start a", "hold starts: 2997 of 3000 tasks in /user.slice/user-1000.slice, below task_headroom 500"}
		if !slices.Equal(*lines, want) {
			t.Errorf("log lines = %q; want %q", *lines, want)
		}
	})

	tests := []struct {
		name         string
		taskHeadroom int
		limitFound   bool
		nilSeam      bool
	}{
		{"TaskHeadroomZeroHoldsNothing", 0, true, false},
		{"AnUnreadLimitHoldsNothing", 500, false, false},
		{"ANilSeamHoldsNothing", 500, true, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := newPassHarness(t)
			h.fake.taskHeadroom = tt.taskHeadroom
			h.fake.limit, h.fake.limitFound = near, tt.limitFound
			if tt.nilSeam {
				h.d.deps.TaskLimit = nil
			}
			h.fake.tasks = []TaskView{readyTask("a")}
			h.tick(true)
			if len(h.fake.spawnsOf("a")) != 1 {
				t.Errorf("first passes = %+v; want the task started", h.fake.spawned)
			}
		})
	}
}
