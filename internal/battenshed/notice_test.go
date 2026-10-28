//go:build tier0

// notice_test.go covers the notice step: the rendered line, each condition's one notice per episode, when a parked stop's notice may go, the awaiting relay rule, the arbiter hold, the quiet condition's guards, delivery and its retries, and a restart over the same scratch directory.
// The tests that capture log output replace the process-global logger output, so they do not run in parallel.

package battenshed

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/state"
	"github.com/Knatte18/loomyard/internal/testkit/logcapture"
)

// noticeHarness drives passes of the Run-Shed row whose status, driver liveness, stop report and orch strand a test sets between them.
// Every pass is a new producer over the harness's one scratch directory, so only the memory record carries state from one pass to the next.
type noticeHarness struct {
	t          *testing.T
	scratch    string
	statusPath string
	clock      *fakeClock
	status     shedengine.Status
	alive      bool
	// touched is the status file's modification time as the test last set it.
	touched time.Time

	notified     []string
	notifyTimes  []time.Time
	notifyErr    error
	notifyQueued bool
	strand       bool

	reportPath  string
	reportAt    time.Time
	reportFound bool
	decided     bool
	watched     bool

	// arbiter, arbiterFound and arbiterErr are what the fake ArbiterInFlight seam reports.
	arbiter      ChildArbiter
	arbiterFound bool
	arbiterErr   error

	// arbiterStop and arbiterStopErr are what the fake ArbiterStop seam reports; the zero reading is not eligible.
	arbiterStop    ChildArbiterStop
	arbiterStopErr error

	// driverLock is what the fake ChildDriverLockHeld seam reports; tackler and tacklerFound are what the fake TacklerInFlight seam reports.
	driverLock   bool
	tackler      ChildArbiter
	tacklerFound bool

	// runs, waitLive and activityErr are what the fake Activity seam reports; activityReads counts its calls.
	runs          []AgentActivity
	waitLive      bool
	activityErr   error
	activityReads int

	quiet time.Duration
	probe time.Duration
	poll  time.Duration

	// pause is what the fake Pause seam reports.
	pause pauserecord.Coverage

	// reviveAPI wires the fake ReviveAPIError seam, reviveAPIErr is what it reports, and revives counts its calls.
	reviveAPI    bool
	reviveAPIErr error
	revives      int

	// sleeps are the machine sleeps suspend has recorded, which the fake Sleeps seam reports.
	sleeps []pauserecord.Interval
}

func newNoticeHarness(t *testing.T) *noticeHarness {
	t.Helper()
	dir := t.TempDir()
	h := &noticeHarness{
		t:            t,
		scratch:      filepath.Join(dir, "scratch"),
		statusPath:   filepath.Join(dir, "status.json"),
		clock:        &fakeClock{t: t},
		status:       shedengine.Status{State: shedengine.StateRunning},
		alive:        true,
		notifyQueued: true,
		strand:       true,
		quiet:        45 * time.Minute,
		poll:         time.Second,
	}
	h.touch(h.clock.Now())
	return h
}

// suspend moves the fake wall clock forward by d and records the machine sleep of d it stands for, as the scheduling dobby would.
func (h *noticeHarness) suspend(d time.Duration) {
	start := h.clock.Now()
	h.clock.advance(d)
	h.sleeps = append(h.sleeps, pauserecord.Interval{Start: start, End: h.clock.Now()})
}

// memory reads the memory record the passes left in the scratch directory.
func (h *noticeHarness) memory() innerRunMemory {
	h.t.Helper()
	memory, found, err := state.ReadJSON[innerRunMemory](innerRunMemoryFile(h.scratch, "Run-Shed"), innerRunMemoryLockFile(h.scratch, "Run-Shed"))
	if err != nil || !found {
		h.t.Fatalf("memory record = %v found=%v; want it written", err, found)
	}
	return memory
}

// touch sets the status file's modification time, creating the file when absent.
func (h *noticeHarness) touch(at time.Time) {
	h.t.Helper()
	h.touched = at
	if err := os.WriteFile(h.statusPath, []byte("{}"), 0o644); err != nil {
		h.t.Fatal(err)
	}
	if err := os.Chtimes(h.statusPath, at, at); err != nil {
		h.t.Fatal(err)
	}
}

// setStatus makes the child read as status, with its status file written now.
func (h *noticeHarness) setStatus(status shedengine.Status) {
	h.t.Helper()
	h.status = status
	h.touch(h.clock.Now())
}

// coverFrom makes the fake pause record cover the slug from since, with one paused interval from since to until.
func (h *noticeHarness) coverFrom(since, until time.Time) {
	h.pause = pauserecord.Coverage{
		Covered:   true,
		Entry:     pauserecord.Entry{Scope: pauserecord.ScopeHub, PausedAt: since},
		Intervals: []pauserecord.Interval{{Start: since, End: until}},
	}
}

// resumeFrom lifts the cover, leaving the interval from since to until behind as a resumed pause.
func (h *noticeHarness) resumeFrom(since, until time.Time) {
	h.pause = pauserecord.Coverage{Intervals: []pauserecord.Interval{{Start: since, End: until}}}
}

// parked is a child halted in state, with a history of two entries.
func parked(state shedengine.State) shedengine.Status {
	return shedengine.Status{State: state, Error: "stuck", CurrentProducer: "Publish", History: make([]shedengine.HistoryEntry, 2)}
}

// writeReport makes the driver's park marker read as holding a stop report written at at.
func (h *noticeHarness) writeReport(at time.Time) {
	h.reportPath, h.reportAt, h.reportFound = "/wt/task/_lym/stop-report.md", at, true
}

// producer returns a producer whose every Call is one pass, run by a producer built anew over the harness's scratch directory.
func (h *noticeHarness) producer() shedengine.ShedProducer {
	return freshPerCall(h.newProducer)
}

// newProducer builds a producer over the harness's scratch directory.
func (h *noticeHarness) newProducer() shedengine.ShedProducer {
	deps := InnerRunDeps{
		Spawn:         func(context.Context) error { return nil },
		ResolveStatus: func() (string, string, error) { return h.statusPath, "", nil },
		ReadStatus: func(string, string) (shedengine.Status, bool, error) {
			return h.status, true, nil
		},
		Now:            h.clock.Now,
		Sleeps:         func() ([]pauserecord.Interval, error) { return h.sleeps, nil },
		PauseRequested: h.clock.pauseRequested,
		Pause:          func() (pauserecord.Coverage, error) { return h.pause, nil },
		NoticeProbe:    h.probe,
		ReadDecision: func() (ChildDecision, bool, error) {
			return ChildDecision{Kind: DecisionApprove, At: "2026-01-01T10:00:00Z"}, h.decided, nil
		},
		DriverAlive: func(context.Context) (bool, error) { return h.alive, nil },
		Notify: func(_ context.Context, line string) (bool, error) {
			h.notifyTimes = append(h.notifyTimes, h.clock.Now())
			if h.notifyErr != nil {
				return false, h.notifyErr
			}
			if h.notifyQueued {
				h.notified = append(h.notified, line)
			}
			return h.notifyQueued, nil
		},
		OrchStrandRecorded: func() (bool, error) { return h.strand, nil },
		StopReport: func() (string, time.Time, bool, error) {
			return h.reportPath, h.reportAt, h.reportFound, nil
		},
		MarkWatched: func(context.Context) (bool, error) { return h.watched, nil },
		ArbiterInFlight: func(context.Context) (ChildArbiter, bool, error) {
			return h.arbiter, h.arbiterFound, h.arbiterErr
		},
		ArbiterStop: func(context.Context, shedengine.Status) (ChildArbiterStop, error) {
			return h.arbiterStop, h.arbiterStopErr
		},
		ChildDriverLockHeld: func() (bool, error) { return h.driverLock, nil },
		TacklerInFlight: func(context.Context) (ChildArbiter, bool, error) {
			return h.tackler, h.tacklerFound, nil
		},
		NoticeQuiet: h.quiet,
		Activity: func(context.Context) ([]AgentActivity, bool, error) {
			h.activityReads++
			return h.runs, h.waitLive, h.activityErr
		},
		PairDir: func() (string, error) { return "/wt/task", nil },
	}
	if h.reviveAPI {
		deps.ReviveAPIError = func(context.Context) error {
			h.revives++
			return h.reviveAPIErr
		}
	}
	return NewInnerRun("Run-Shed", "task", deps, h.poll, h.scratch, testGrace)
}

// drive runs passes of p, one poll interval apart, until one ends with a Stuck, which it requires to be a Stuck whatever the notice step did.
// A change of state the child made before the first pass ends that pass at once, as the change back into the state the memory left; it counts as no check and the passes go on at the same time.
func (h *noticeHarness) drive(p shedengine.ShedProducer) {
	h.t.Helper()
	for first := true; ; first = false {
		outcome, ptr, err := p.Call(context.Background())
		switch {
		case err != nil:
			h.t.Fatalf("Call() error = %v", err)
		case outcome == shedengine.Yield:
			h.clock.Sleep(context.Background(), h.poll)
		case outcome == shedengine.Stuck && first && isStateChange(ptr.Path):
		case outcome == shedengine.Stuck:
			return
		default:
			h.t.Fatalf("Call() = %v %+v; want a Yield or a Stuck", outcome, ptr)
		}
	}
}

// isStateChange reports whether path names a change of the child's state.
func isStateChange(path string) bool {
	_, ok := ParseStateChange(path)
	return ok
}

// untilEnd runs passes of p, one poll interval apart, and returns the first that does not yield.
func (h *noticeHarness) untilEnd(p shedengine.ShedProducer) (shedengine.Outcome, shedengine.OutputPointer, error) {
	for {
		outcome, ptr, err := p.Call(context.Background())
		if outcome != shedengine.Yield || err != nil {
			return outcome, ptr, err
		}
		h.clock.Sleep(context.Background(), h.poll)
	}
}

// run drives passes that end after checks checks unless something else ends them first.
func (h *noticeHarness) run(p shedengine.ShedProducer, checks int) {
	h.t.Helper()
	h.clock.pauseAtSleep = h.clock.sleepCalls + checks
	h.drive(p)
}

// call drives passes that end at the first check after the first pass.
func (h *noticeHarness) call(p shedengine.ShedProducer) { h.t.Helper(); h.run(p, 1) }

// leaveRunning drives passes over a running child that turns into status at the first check, so the run ends on the state change before the new state's notice has had a pass of its own.
func (h *noticeHarness) leaveRunning(p shedengine.ShedProducer, status shedengine.Status) {
	h.t.Helper()
	h.setStatus(shedengine.Status{State: shedengine.StateRunning})
	h.clock.pauseAtSleep = 0
	h.clock.onSleep = func(int) { h.setStatus(status) }
	defer func() { h.clock.onSleep = nil }()
	for {
		outcome, ptr, err := h.untilEnd(p)
		if err != nil || outcome != shedengine.Stuck {
			h.t.Fatalf("Call() = %v %+v, %v; want the return for the change out of running", outcome, ptr, err)
		}
		if strings.HasPrefix(ptr.Path, "child running → ") {
			return
		}
		// The change into running from the state the memory left ends the first pass; the child leaves running at the next.
	}
}

// secondCheck runs fn at the second check of the passes the test drives next, after the first check has had its chance to send, and ends them after four checks.
// It returns a function reporting how many notices had gone when fn ran.
func (h *noticeHarness) secondCheck(fn func()) (sentBefore func() int) {
	h.t.Helper()
	base := h.clock.sleepCalls
	before := -1
	h.clock.onSleep = func(call int) {
		if call == base+2 {
			before = len(h.notified)
			fn()
		}
	}
	h.clock.pauseAtSleep = base + 4
	return func() int { return before }
}

func TestRenderNotice(t *testing.T) {
	t.Parallel()

	since := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	long := shedengine.Status{State: shedengine.StateFailed, CurrentProducer: "Publish", Error: "boom\nsecond\x1b[0m line " + strings.Repeat("x", 300), History: make([]shedengine.HistoryEntry, 7)}
	awaiting := shedengine.Status{State: shedengine.StateAwaiting, CurrentProducer: "PR-Gate", Error: "the pull request awaits review: /wt/task/_lym/review.md, beside /other/wt/task/x", History: make([]shedengine.HistoryEntry, 3)}
	done := shedengine.Status{State: shedengine.StateDone}
	tests := []struct {
		name string
		line noticeLine
		want []string
		deny []string
	}{
		{
			name: "AFailedChildLabelsItsReasonAnError",
			line: noticeLine{slug: "task", condition: haltText(long), status: long, since: since, report: reportClause("/wt/task/_lym/stop.md", true), pairDir: "/wt/task"},
			want: []string{"task: failed at Publish;", "error: boom second", "since 2026-01-02T03:04:05Z", "history 7", "report _lym/stop.md"},
			deny: []string{"/wt/task"},
		},
		{
			name: "NoReportYet",
			line: noticeLine{slug: "task", condition: haltText(parked(shedengine.StateBlocked)), status: parked(shedengine.StateBlocked), since: since, report: reportClause("", false), pairDir: "/wt/task"},
			want: []string{"task: blocked at Publish;", "error: stuck", "since 2026-01-02T03:04:05Z", "history 2", "report none yet"},
		},
		{
			name: "AnAwaitingChildIsWaitingNotAnError",
			line: noticeLine{slug: "task", condition: haltText(awaiting), status: awaiting, since: since, pairDir: "/wt/task"},
			want: []string{"task: awaiting at PR-Gate;", "waiting: the pull request awaits review: _lym/review.md, beside /other/wt/task/x;"},
			deny: []string{"error:", " /wt/task"},
		},
		{
			name: "ADoneNoticeCarriesNoReportAndNoReason",
			line: noticeLine{slug: "task", condition: haltText(done), status: done, since: since, pairDir: "/wt"},
			want: []string{"task: done;", "since 2026-01-02T03:04:05Z", "history 0"},
			deny: []string{"report", "error:", "waiting:"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			line := renderNotice(tt.line)
			if !strings.HasPrefix(line, noticePrefix) || strings.Contains(line, "reed attach") {
				t.Errorf("notice = %q; want the prefix and no attach command", line)
			}
			if strings.ContainsAny(line, "\n\r\x1b") {
				t.Errorf("notice = %q; want one line without control characters", line)
			}
			for _, want := range tt.want {
				if !strings.Contains(line, want) {
					t.Errorf("notice = %q; want it to contain %q", line, want)
				}
			}
			for _, deny := range tt.deny {
				if strings.Contains(line, deny) {
					t.Errorf("notice = %q; want it without %q", line, deny)
				}
			}
		})
	}

	// The child error is cut to its limit.
	const head = "boom\nsecond\x1b[0m line "
	kept := strings.Repeat("x", noticeErrorMax-len([]rune(head)))
	if line := renderNotice(tests[0].line); !strings.Contains(line, kept+";") {
		t.Errorf("notice = %q; want the error cut to %d characters", line, noticeErrorMax)
	}
}

func TestNotice_StateChangeNotifiesOncePerEpisode(t *testing.T) {
	h := newNoticeHarness(t)
	p := h.producer()

	h.call(p)
	if len(h.notified) != 0 {
		t.Fatalf("notified = %v; want none for a live running child", h.notified)
	}

	h.setStatus(parked(shedengine.StateBlocked))
	h.writeReport(h.clock.Now())
	h.call(p)
	h.call(p)
	if len(h.notified) != 1 || !strings.Contains(h.notified[0], "blocked") {
		t.Fatalf("notified = %v; want exactly one blocked notice across two Calls", h.notified)
	}

	t.Run("AHaltingChildStillNotifiesUnderAPause", func(t *testing.T) {
		h := newNoticeHarness(t)
		p := h.producer()
		h.coverFrom(h.clock.Now(), h.clock.Now().Add(time.Hour))
		h.setStatus(parked(shedengine.StateBlocked))
		h.writeReport(h.clock.Now())
		h.call(p)
		h.call(p)
		if len(h.notified) != 1 || !strings.Contains(h.notified[0], "blocked") {
			t.Fatalf("notified = %v; want the state-changed notice once although a pause covers the child", h.notified)
		}
	})
}

func TestNotice_StateChangeStartsNewEpisode(t *testing.T) {
	h := newNoticeHarness(t)
	p := h.producer()
	h.setStatus(parked(shedengine.StatePaused))
	h.writeReport(h.clock.Now())
	h.call(p)

	h.setStatus(parked(shedengine.StateFailed))
	h.writeReport(h.clock.Now())
	h.call(p)
	if len(h.notified) != 2 || !strings.Contains(h.notified[0], "paused") || !strings.Contains(h.notified[1], "failed") {
		t.Fatalf("notified = %v; want a notice for each state the child entered", h.notified)
	}
}

func TestNotice_ReturnToRunningStartsNewEpisode(t *testing.T) {
	h := newNoticeHarness(t)
	p := h.producer()
	h.setStatus(parked(shedengine.StateAwaiting))
	h.writeReport(h.clock.Now())
	h.call(p)

	h.setStatus(shedengine.Status{State: shedengine.StateRunning})
	h.call(p)
	h.setStatus(parked(shedengine.StateAwaiting))
	h.writeReport(h.clock.Now())
	h.call(p)
	if len(h.notified) != 2 {
		t.Fatalf("notified = %v; want one notice per awaiting episode", h.notified)
	}
}

func TestNotice_DeadDriverWhileRunning(t *testing.T) {
	h := newNoticeHarness(t)
	p := h.producer()
	h.alive = false
	h.setStatus(shedengine.Status{State: shedengine.StateRunning, CurrentProducer: "Plan-Write"})

	h.call(p)
	h.call(p)
	if len(h.notified) != 1 || !strings.Contains(h.notified[0], "running at Plan-Write; driver strand is dead") || !strings.Contains(h.notified[0], "since ") || !strings.Contains(h.notified[0], "history 0") {
		t.Fatalf("notified = %v; want exactly one dead-driver notice leading with the child's state and row, with its since and history", h.notified)
	}
	if strings.Contains(h.notified[0], "while the child is running") {
		t.Fatalf("notified = %v; want the state to say the child is running, not the condition", h.notified)
	}

	t.Run("AHeldDriverLockSendsNoDriverDeadNotice", func(t *testing.T) {
		h := newNoticeHarness(t)
		p := h.producer()
		h.alive = false
		h.driverLock = true
		h.setStatus(shedengine.Status{State: shedengine.StateRunning, CurrentProducer: "Plan-Write"})
		h.call(p)
		h.call(p)
		if len(h.notified) != 0 {
			t.Fatalf("notified = %v; want none while the driver lock is held", h.notified)
		}
	})

	t.Run("ACoveredChildSendsNoDriverDeadNotice", func(t *testing.T) {
		h := newNoticeHarness(t)
		p := h.producer()
		h.alive = false
		h.coverFrom(h.clock.Now(), h.clock.Now().Add(time.Hour))
		h.setStatus(shedengine.Status{State: shedengine.StateRunning, CurrentProducer: "Plan-Write"})
		h.call(p)
		h.call(p)
		if len(h.notified) != 0 {
			t.Fatalf("notified = %v; want none while a pause covers the child", h.notified)
		}
	})

	t.Run("AFailedReviveOfAHaltedChildSendsTheDriverDeadNoticeAfterTheHaltsOwn", func(t *testing.T) {
		h := newNoticeHarness(t)
		p := h.producer()
		h.setStatus(parked(shedengine.StateBlocked))
		h.writeReport(h.clock.Now())
		h.call(p)
		if err := os.WriteFile(revivedFile(h.scratch, "Run-Shed"), []byte("1234 failed\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		h.call(p)
		h.call(p)
		want := `blocked at Publish; driver strand could not be revived; run "lym loom start" in the task worktree to bring a driver back`
		if len(h.notified) != 2 || !strings.Contains(h.notified[0], "blocked at Publish; error: stuck") || !strings.Contains(h.notified[1], want) {
			t.Fatalf("notified = %q; want the halt's own notice, then one driver-dead notice reading %q", h.notified, want)
		}
	})
}

func TestNotice_QuietOnlyForRunningChildWithLiveDriver(t *testing.T) {
	t.Run("IdleAgentsGiveOneAfterTheWindow", func(t *testing.T) {
		h := newNoticeHarness(t)
		p := h.producer()
		h.setStatus(shedengine.Status{State: shedengine.StateRunning, CurrentProducer: "Plan-Write"})
		h.runs = []AgentActivity{{Producer: "t:task:impl", LastActivity: h.clock.Now()}, {Producer: "t:task:review", LastActivity: h.clock.Now().Add(-time.Hour)}}
		h.clock.advance(44 * time.Minute)
		h.call(p)
		if len(h.notified) != 0 {
			t.Fatalf("notified = %v; want none inside the quiet window", h.notified)
		}
		h.clock.advance(time.Minute)
		h.call(p)
		h.call(p)
		if len(h.notified) != 1 || !strings.Contains(h.notified[0], "running at Plan-Write; agents idle for 45m") || strings.Contains(h.notified[0], "while the child is running") {
			t.Fatalf("notified = %v; want exactly one quiet notice leading with the child's state and row, measured from the newest run", h.notified)
		}
	})

	t.Run("ACoveredChildSendsNoQuietNoticeHoweverLong", func(t *testing.T) {
		h := newNoticeHarness(t)
		p := h.producer()
		start := h.clock.Now()
		h.runs = []AgentActivity{{Producer: "t:task:impl", LastActivity: start}}
		h.coverFrom(start, start.Add(2*time.Hour))
		h.clock.advance(2 * time.Hour)
		h.run(p, 3)
		if len(h.notified) != 0 {
			t.Fatalf("notified = %v; want none while a pause covers the child", h.notified)
		}
	})

	t.Run("AfterTheResumeThePausedIntervalIsNotIdleTime", func(t *testing.T) {
		h := newNoticeHarness(t)
		p := h.producer()
		start := h.clock.Now()
		h.runs = []AgentActivity{{Producer: "t:task:impl", LastActivity: start}}
		h.coverFrom(start, start.Add(time.Hour))
		h.clock.advance(time.Hour)
		h.call(p)
		h.resumeFrom(start, start.Add(time.Hour))
		h.clock.advance(44 * time.Minute)
		h.call(p)
		if len(h.notified) != 0 {
			t.Fatalf("notified = %v; want none until 45 unpaused minutes have passed", h.notified)
		}
		h.clock.advance(time.Minute)
		h.call(p)
		if len(h.notified) != 1 || !strings.Contains(h.notified[0], "agents idle for 45m") || strings.Contains(h.notified[0], "1h") {
			t.Fatalf("notified = %v; want one quiet notice reporting 45m", h.notified)
		}
	})

	t.Run("AnAgentThatKeepsWorkingNeverGoesQuietWhateverTheStatusFileDoes", func(t *testing.T) {
		h := newNoticeHarness(t)
		p := h.producer()
		for range 4 {
			h.clock.advance(30 * time.Minute)
			h.runs = []AgentActivity{{Producer: "t:task:impl", LastActivity: h.clock.Now()}}
			h.call(p)
		}
		if len(h.notified) != 0 {
			t.Fatalf("notified = %v; want none while an agent keeps reporting activity", h.notified)
		}
	})

	t.Run("ALiveWaitMarkerSuppressesIt", func(t *testing.T) {
		h := newNoticeHarness(t)
		p := h.producer()
		h.runs = []AgentActivity{{Producer: "t:task:impl", LastActivity: h.clock.Now()}}
		h.waitLive = true
		h.clock.advance(2 * time.Hour)
		h.call(p)
		h.runs = nil
		h.call(p)
		if len(h.notified) != 0 {
			t.Fatalf("notified = %v; want none while a verify or shuttle wait is live, with or without a live run", h.notified)
		}
	})

	t.Run("NoLiveRunFallsBackToTheLaterOfTheNewestHistoryEntryAndSince", func(t *testing.T) {
		h := newNoticeHarness(t)
		p := h.producer()
		h.call(p)
		h.status.History = []shedengine.HistoryEntry{{At: h.clock.Now().Add(30 * time.Minute).Format(time.RFC3339)}}
		h.clock.advance(60 * time.Minute)
		h.call(p)
		if len(h.notified) != 0 {
			t.Fatalf("notified = %v; want none 30 minutes after the newest history entry", h.notified)
		}
		h.clock.advance(15 * time.Minute)
		h.call(p)
		h.call(p)
		if len(h.notified) != 1 || !strings.Contains(h.notified[0], "no agent activity readable for 45m") {
			t.Fatalf("notified = %v; want exactly one fallback quiet notice", h.notified)
		}
	})

	t.Run("AnUnreadableActivitySendsNothing", func(t *testing.T) {
		h := newNoticeHarness(t)
		p := h.producer()
		h.activityErr = errors.New("shuttle config unreadable")
		h.clock.advance(2 * time.Hour)
		h.call(p)
		if len(h.notified) != 0 {
			t.Fatalf("notified = %v; want none when the activity cannot be read", h.notified)
		}
	})

	t.Run("NotForADeadDriver", func(t *testing.T) {
		h := newNoticeHarness(t)
		p := h.producer()
		h.alive = false
		h.clock.advance(time.Hour)
		h.call(p)
		if len(h.notified) != 1 || strings.Contains(h.notified[0], "no agent activity") {
			t.Fatalf("notified = %v; want the dead-driver notice only", h.notified)
		}
	})

	t.Run("NotForAHaltedChild", func(t *testing.T) {
		h := newNoticeHarness(t)
		p := h.producer()
		h.setStatus(parked(shedengine.StatePaused))
		h.writeReport(h.clock.Now())
		h.clock.advance(time.Hour)
		h.call(p)
		if len(h.notified) != 1 || strings.Contains(h.notified[0], "no agent activity") {
			t.Fatalf("notified = %v; want the state-change notice only", h.notified)
		}
	})

	t.Run("ZeroWindowDisablesIt", func(t *testing.T) {
		h := newNoticeHarness(t)
		h.quiet = 0
		p := h.producer()
		h.clock.advance(24 * time.Hour)
		h.call(p)
		if len(h.notified) != 0 {
			t.Fatalf("notified = %v; want none with the quiet window off", h.notified)
		}
	})

	// The sleep subtests record the suspended time through the Sleeps seam, as the scheduling dobby does: onCheck runs fn at the nth check's sleep, before that check judges.
	onCheck := func(h *noticeHarness, fns map[int]func()) {
		h.clock.onSleep = func(call int) {
			if fn := fns[call]; fn != nil {
				fn()
			}
		}
	}

	t.Run("ASleepGapRaisesNoNoticeUntilTheAwakeIdleTimeReachesTheWindow", func(t *testing.T) {
		h := newNoticeHarness(t)
		p := h.producer()
		h.runs = []AgentActivity{{Producer: "t:task:impl", LastActivity: h.clock.Now()}}
		sentBefore := -1
		onCheck(h, map[int]func(){
			2: func() { h.suspend(3 * time.Hour) },
			3: func() { sentBefore = len(h.notified); h.clock.advance(h.quiet) },
		})
		h.run(p, 6)
		if sentBefore != 0 || len(h.notified) != 1 || !strings.Contains(h.notified[0], "agents idle for 45m") || strings.Contains(h.notified[0], "3h") {
			t.Fatalf("notified = %v, %d before the window; want one quiet notice reporting the awake idle time, none during the sleep", h.notified, sentBefore)
		}
	})

	t.Run("AnActivityWrittenJustBeforeTheSleepKeepsTheGap", func(t *testing.T) {
		h := newNoticeHarness(t)
		p := h.producer()
		h.runs = []AgentActivity{{Producer: "t:task:impl", LastActivity: h.clock.Now()}}
		onCheck(h, map[int]func(){
			2: func() {
				h.runs = []AgentActivity{{Producer: "t:task:impl", LastActivity: h.clock.Now()}}
				h.suspend(3 * time.Hour)
			},
			3: func() { h.clock.advance(h.quiet) },
		})
		h.run(p, 6)
		if len(h.notified) != 1 || !strings.Contains(h.notified[0], "agents idle for 45m") || strings.Contains(h.notified[0], "3h") {
			t.Fatalf("notified = %v; want one quiet notice reporting 45m of awake idle time, not the night's gap", h.notified)
		}
	})

	t.Run("AMovedNewestActivityDropsTheGapObservedWithIt", func(t *testing.T) {
		h := newNoticeHarness(t)
		p := h.producer()
		h.runs = []AgentActivity{{Producer: "t:task:impl", LastActivity: h.clock.Now()}}
		onCheck(h, map[int]func(){
			2: func() {
				h.suspend(3 * time.Hour)
				h.runs = []AgentActivity{{Producer: "t:task:impl", LastActivity: h.clock.Now()}}
			},
			3: func() { h.clock.advance(h.quiet) },
		})
		h.run(p, 6)
		if len(h.notified) != 1 || !strings.Contains(h.notified[0], "agents idle for 45m") {
			t.Fatalf("notified = %v; want one quiet notice at the window measured from the new activity", h.notified)
		}
	})

	t.Run("TheNoLiveRunFallbackDropsTheGapWhenItsStampMoves", func(t *testing.T) {
		h := newNoticeHarness(t)
		p := h.producer()
		onCheck(h, map[int]func(){
			2: func() {
				h.suspend(3 * time.Hour)
				h.status.History = []shedengine.HistoryEntry{{At: h.clock.Now().Format(time.RFC3339)}}
			},
			3: func() { h.clock.advance(h.quiet) },
		})
		h.run(p, 6)
		if len(h.notified) != 1 || !strings.Contains(h.notified[0], "no agent activity readable for 45m") {
			t.Fatalf("notified = %v; want one fallback notice at the window measured from the new history entry", h.notified)
		}
	})

	t.Run("AnUnreadableActivityKeepsTheGapForALaterReadableCheck", func(t *testing.T) {
		h := newNoticeHarness(t)
		p := h.producer()
		h.runs = []AgentActivity{{Producer: "t:task:impl", LastActivity: h.clock.Now()}}
		onCheck(h, map[int]func(){
			2: func() {
				h.activityErr = errors.New("shuttle config unreadable")
				h.suspend(3 * time.Hour)
			},
			3: func() {
				h.activityErr = nil
				h.clock.advance(30 * time.Minute)
			},
		})
		h.run(p, 6)
		if len(h.notified) != 0 {
			t.Fatalf("notified = %v; want none: the gap seen on the unreadable check is subtracted at the readable one", h.notified)
		}
	})
}

func TestNotice_APIErrorStall(t *testing.T) {
	h := newNoticeHarness(t)
	p := h.producer()
	h.setStatus(shedengine.Status{State: shedengine.StateRunning, CurrentProducer: "Plan-Write"})
	stall := func() {
		h.runs = []AgentActivity{
			{Producer: "t:task:impl", LastActivity: h.clock.Now(), APIError: true, APIErrorText: "overloaded\nplease retry"},
			{Producer: "t:task:review", LastActivity: h.clock.Now().Add(-time.Hour)},
		}
	}

	stall()
	h.clock.advance(noticeAPIErrorIdle - 5*time.Second)
	h.call(p)
	if len(h.notified) != 0 {
		t.Fatalf("notified = %v; want none before the error has stood for %s", h.notified, noticeAPIErrorIdle)
	}
	h.clock.advance(5 * time.Second)
	h.call(p)
	h.call(p)
	if len(h.notified) != 1 || !strings.Contains(h.notified[0], "running at Plan-Write; agent t:task:impl hit an API error: overloaded please retry") || strings.Contains(h.notified[0], "while the child is running") {
		t.Fatalf("notified = %v; want exactly one api-error notice leading with the child's state and row, naming the producer and the one-line error", h.notified)
	}

	// Activity ends the episode, and a later stall is a new notice that outranks quiet however long it stands.
	h.runs = []AgentActivity{{Producer: "t:task:impl", LastActivity: h.clock.Now()}}
	h.call(p)
	stall()
	h.clock.advance(noticeAPIErrorIdle)
	h.call(p)
	h.clock.advance(2 * h.quiet)
	h.call(p)
	if len(h.notified) != 2 || !strings.Contains(h.notified[1], "API error") {
		t.Fatalf("notified = %v; want a second api-error notice and no quiet one", h.notified)
	}

	t.Run("ACoveredChildSendsNoApiErrorNotice", func(t *testing.T) {
		h := newNoticeHarness(t)
		p := h.producer()
		start := h.clock.Now()
		h.runs = []AgentActivity{{Producer: "t:task:impl", LastActivity: start, APIError: true, APIErrorText: "overloaded"}}
		h.coverFrom(start, start.Add(time.Hour))
		h.clock.advance(time.Hour)
		h.run(p, 3)
		if len(h.notified) != 0 {
			t.Fatalf("notified = %v; want none while a pause covers the child", h.notified)
		}
	})

	// A sleep after the stall is not time the error has stood: the notice waits for the awake idle time.
	t.Run("ASleepGapRaisesNoNoticeUntilTheAwakeIdleTimeReachesTheIdle", func(t *testing.T) {
		h := newNoticeHarness(t)
		p := h.producer()
		h.runs = []AgentActivity{{Producer: "t:task:impl", LastActivity: h.clock.Now(), APIError: true, APIErrorText: "overloaded"}}
		sentBefore := -1
		h.clock.onSleep = func(call int) {
			switch call {
			case 2:
				h.suspend(3 * time.Hour)
			case 3:
				sentBefore = len(h.notified)
				h.clock.advance(noticeAPIErrorIdle)
			}
		}
		h.run(p, 6)
		if sentBefore != 0 || len(h.notified) != 1 || !strings.Contains(h.notified[0], "API error") {
			t.Fatalf("notified = %v, %d before the idle time; want one api-error notice, none during the sleep", h.notified, sentBefore)
		}
	})

	// With the revive seam wired, each pass below is one Call, and an episode is a stall at a new newest activity.
	t.Run("WithTheReviveWired", func(t *testing.T) {
		newRevivingHarness := func(t *testing.T) (*noticeHarness, shedengine.ShedProducer, func()) {
			h := newNoticeHarness(t)
			h.reviveAPI = true
			h.setStatus(shedengine.Status{State: shedengine.StateRunning, CurrentProducer: "Plan-Write"})
			p := h.producer()
			// newEpisode stalls an agent at the clock's time and lets the error stand long enough to be judged.
			newEpisode := func() {
				h.runs = []AgentActivity{{Producer: "t:task:impl", LastActivity: h.clock.Now(), APIError: true, APIErrorText: "overloaded"}}
				h.clock.advance(noticeAPIErrorIdle)
			}
			return h, p, newEpisode
		}
		pass := func(t *testing.T, h *noticeHarness, p shedengine.ShedProducer) {
			t.Helper()
			h.clock.advance(h.poll)
			if outcome, ptr, err := p.Call(context.Background()); outcome != shedengine.Yield || err != nil {
				t.Fatalf("Call() = %v %+v, %v; want a Yield", outcome, ptr, err)
			}
		}

		t.Run("TheFirstSightRevivesAndTheEpisodeStillStandingAtTheNextProbeNotices", func(t *testing.T) {
			h, p, newEpisode := newRevivingHarness(t)
			h.reviveAPIErr = errors.New("tender unavailable")
			newEpisode()

			pass(t, h, p)
			if h.revives != 1 || len(h.notified) != 0 {
				t.Fatalf("revives = %d, notified = %v; want one revive, errored or not, and no notice at the first sight", h.revives, h.notified)
			}
			pass(t, h, p)
			pass(t, h, p)
			if h.revives != 1 || len(h.notified) != 1 || !strings.Contains(h.notified[0], "API error") {
				t.Fatalf("revives = %d, notified = %v; want the one revive and one api-error notice", h.revives, h.notified)
			}
		})

		t.Run("AnEpisodeThatEndsAfterTheReviveNoticesNothing", func(t *testing.T) {
			h, p, newEpisode := newRevivingHarness(t)
			newEpisode()
			pass(t, h, p)
			h.runs = []AgentActivity{{Producer: "t:task:impl", LastActivity: h.clock.Now()}}
			pass(t, h, p)
			pass(t, h, p)
			if h.revives != 1 || len(h.notified) != 0 {
				t.Fatalf("revives = %d, notified = %v; want one revive and no notice once the session was active again", h.revives, h.notified)
			}
		})

		tests := []struct {
			name string
			// writeAfter is the number of revived episodes after which the child's status file is written.
			writeAfter  int
			wantNotices int
		}{
			{name: "ThreeRevivedEpisodesWithNoStatusWriteNoticeOnTheThird", wantNotices: 1},
			{name: "AStatusWriteBetweenThemResetsTheCount", writeAfter: 2, wantNotices: 0},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				h, p, newEpisode := newRevivingHarness(t)
				for episode := 1; episode <= apiErrorReviveStreak; episode++ {
					newEpisode()
					pass(t, h, p)
					if tt.writeAfter == episode {
						h.touch(h.clock.Now())
					}
					if sent := episode == apiErrorReviveStreak && tt.wantNotices > 0; sent != (len(h.notified) > 0) {
						t.Fatalf("after episode %d: notified = %v; want a notice only at the last episode of an unbroken streak", episode, h.notified)
					}
				}
				if h.revives != apiErrorReviveStreak || len(h.notified) != tt.wantNotices {
					t.Errorf("revives = %d, notified = %v; want %d revives and %d notices", h.revives, h.notified, apiErrorReviveStreak, tt.wantNotices)
				}
			})
		}
	})
}

func TestNotice_UsageLimitNotifiesOncePerLimitWithoutARevive(t *testing.T) {
	const limitText = "You've hit your weekly limit · resets 2pm (Europe/Oslo)"
	resetsAt := time.Date(2026, 10, 28, 13, 0, 0, 0, time.UTC)
	tests := []struct {
		name     string
		resetsAt time.Time
		wantText string
	}{
		{"AReadableResetTime", resetsAt, "running at Plan-Write; agent t:task:impl hit a usage limit, resets 2026-10-28T13:00:00Z: " + limitText},
		{"AnUnknownResetTime", time.Time{}, "running at Plan-Write; agent t:task:impl hit a usage limit, reset time unknown: " + limitText},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newNoticeHarness(t)
			h.reviveAPI = true
			h.setStatus(shedengine.Status{State: shedengine.StateRunning, CurrentProducer: "Plan-Write"})
			p := h.producer()
			limited := func(resetsAt time.Time) {
				h.runs = []AgentActivity{{Producer: "t:task:impl", LastActivity: h.clock.Now(), APIError: true, APIErrorText: limitText, UsageLimited: true, UsageLimitResetsAt: resetsAt}}
			}

			// The session's activity moves under a standing limit, as a revive typed into it would move it.
			for range 3 {
				limited(tt.resetsAt)
				h.clock.advance(noticeAPIErrorIdle)
				h.call(p)
				h.clock.advance(2 * h.quiet)
				h.call(p)
			}
			if h.revives != 0 || len(h.notified) != 1 || !strings.Contains(h.notified[0], tt.wantText) {
				t.Fatalf("revives = %d, notified = %v; want no revive and one usage-limit notice containing %q", h.revives, h.notified, tt.wantText)
			}

			// The limit ends, and a later one is a new notice.
			h.runs = []AgentActivity{{Producer: "t:task:impl", LastActivity: h.clock.Now()}}
			h.call(p)
			limited(tt.resetsAt.Add(24 * time.Hour))
			h.clock.advance(noticeAPIErrorIdle)
			h.call(p)
			if len(h.notified) != 2 || !strings.Contains(h.notified[1], "hit a usage limit") {
				t.Fatalf("notified = %v; want a second usage-limit notice for the later limit", h.notified)
			}
		})
	}
}

func TestNotice_UsageLimitOfOneRunOutranksTheOthersAndStandsWhileItReadsLimited(t *testing.T) {
	const limitText = "You've hit your weekly limit · resets 2pm (Europe/Oslo)"
	resetsAt := time.Date(2026, 10, 28, 13, 0, 0, 0, time.UTC)
	tests := []struct {
		name string
		// otherRun builds the child's second run at now.
		otherRun func(now time.Time) AgentActivity
		// limitedFirst puts the limited run ahead of the other run in the reading.
		limitedFirst bool
	}{
		{"AnotherRunWithAPlainAPIError", func(now time.Time) AgentActivity {
			return AgentActivity{Producer: "t:task:review", LastActivity: now, APIError: true, APIErrorText: "API Error: 529 overloaded"}
		}, false},
		{"AnotherRunNotLimited", func(now time.Time) AgentActivity {
			return AgentActivity{Producer: "t:task:review", LastActivity: now}
		}, false},
		{"AnotherRunNotLimitedAfterTheLimitedRun", func(now time.Time) AgentActivity {
			return AgentActivity{Producer: "t:task:review", LastActivity: now}
		}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newNoticeHarness(t)
			h.reviveAPI = true
			h.setStatus(shedengine.Status{State: shedengine.StateRunning, CurrentProducer: "Plan-Write"})
			p := h.producer()

			for range 3 {
				now := h.clock.Now()
				limited := AgentActivity{Producer: "t:task:impl", LastActivity: now, APIError: true, APIErrorText: limitText, UsageLimited: true, UsageLimitResetsAt: resetsAt}
				h.runs = []AgentActivity{tt.otherRun(now), limited}
				if tt.limitedFirst {
					h.runs = []AgentActivity{limited, tt.otherRun(now)}
				}
				h.clock.advance(noticeAPIErrorIdle)
				h.call(p)
				h.clock.advance(2 * h.quiet)
				h.call(p)
			}
			if len(h.notified) != 1 || !strings.Contains(h.notified[0], "hit a usage limit") || strings.Contains(h.notified[0], "API error") {
				t.Fatalf("notified = %v; want exactly one usage-limit notice and no API-error notice", h.notified)
			}
		})
	}
}

func TestNotice_RestartDoesNotRenotify(t *testing.T) {
	h := newNoticeHarness(t)
	h.setStatus(parked(shedengine.StateFailed))
	h.writeReport(h.clock.Now())
	h.call(h.producer())
	h.call(h.producer())
	if len(h.notified) != 1 {
		t.Fatalf("notified = %v; want one notice across a restart", h.notified)
	}
}

func TestNotice_NilNotifyRunsNoStep(t *testing.T) {
	h := newNoticeHarness(t)
	h.status = parked(shedengine.StateBlocked)
	deps := InnerRunDeps{
		Spawn:          func(context.Context) error { return nil },
		ResolveStatus:  func() (string, string, error) { return h.statusPath, "", nil },
		ReadStatus:     func(string, string) (shedengine.Status, bool, error) { return h.status, true, nil },
		Now:            h.clock.Now,
		PauseRequested: h.clock.pauseRequested,
		ReadDecision:   func() (ChildDecision, bool, error) { return ChildDecision{}, false, nil },
		DriverAlive:    func(context.Context) (bool, error) { return true, nil },
	}
	p := NewInnerRun("Run-Shed", "task", deps, time.Second, h.scratch, testGrace)
	h.call(p)
	if _, err := os.Stat(noticeEpisodeFile(h.scratch, "Run-Shed")); err == nil {
		t.Errorf("episode marker exists; want no notice bookkeeping without a Notify")
	}
}

// TestNotice_ParkedStopWaitsForTheStopReport covers when the state-changed notice of a parked stop goes:
// once the driver's report exists, once the driver strand has ended or three minutes passed since the stop without one, never carrying a report older than the stop,
// and always sent by the pass that begins in the new state, never by the pass that saw the change.
func TestNotice_ParkedStopWaitsForTheStopReport(t *testing.T) {
	tests := []struct {
		name  string
		state shedengine.State
		// before runs between the two Calls, and at runs at the second Call's second check, each with the stop's since.
		before     func(h *noticeHarness, since time.Time)
		at         func(h *noticeHarness, since time.Time)
		wantReport string
	}{
		{
			name:       "AReportWrittenAfterTheStopGoesWithItsPath",
			state:      shedengine.StateBlocked,
			at:         func(h *noticeHarness, since time.Time) { h.writeReport(since.Add(10 * time.Second)) },
			wantReport: "report _lym/stop-report.md",
		},
		{
			name:       "NoReportGoesAfterThreeMinutesAsNoneYet",
			state:      shedengine.StatePaused,
			at:         func(h *noticeHarness, since time.Time) { h.clock.advance(noticeReportWait) },
			wantReport: "report none yet",
		},
		{
			name:       "ThreeMinutesCountFromTheStopNotFromTheCall",
			state:      shedengine.StateFailed,
			before:     func(h *noticeHarness, since time.Time) { h.clock.advance(noticeReportWait - 2*time.Second) },
			wantReport: "report none yet",
		},
		{
			name:  "AnOlderReportNeverQualifies",
			state: shedengine.StateBlocked,
			at: func(h *noticeHarness, since time.Time) {
				h.writeReport(since.Add(-time.Minute))
				h.clock.advance(noticeReportWait)
			},
			wantReport: "report none yet",
		},
		{
			name:       "AnEndedDriverGoesWithoutWaiting",
			state:      shedengine.StateBlocked,
			at:         func(h *noticeHarness, since time.Time) { h.alive = false },
			wantReport: "report none yet",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newNoticeHarness(t)
			p := h.producer()
			h.leaveRunning(p, parked(tt.state))
			since := h.touched
			if len(h.notified) != 0 {
				t.Fatalf("notified = %v after the Call that saw the change; want the notice left to the next Call's wait", h.notified)
			}
			if tt.before != nil {
				tt.before(h, since)
			}

			sentBefore := h.secondCheck(func() {
				if tt.at != nil {
					tt.at(h, since)
				}
			})
			h.drive(p)
			if sentBefore() != 0 || len(h.notified) != 1 {
				t.Fatalf("notices before the second check = %d, in all = %v; want none, then exactly one", sentBefore(), h.notified)
			}
			line := h.notified[0]
			for _, want := range []string{"task: " + string(tt.state) + " at Publish;", "since " + since.UTC().Format(time.RFC3339), "history 2", tt.wantReport} {
				if !strings.Contains(line, want) {
					t.Errorf("notice = %q; want it to contain %q", line, want)
				}
			}
			if tt.wantReport == "report none yet" && strings.Contains(line, "stop-report.md") {
				t.Errorf("notice = %q; want no report path", line)
			}
		})
	}
}

// TestNotice_SameStateRewriteMovesNeitherSinceNorTheKey asserts a rewrite of the status file in the same state, between two Calls, starts no new episode and sends nothing more.
func TestNotice_SameStateRewriteMovesNeitherSinceNorTheKey(t *testing.T) {
	h := newNoticeHarness(t)
	h.setStatus(parked(shedengine.StateBlocked))
	since := h.touched
	h.writeReport(since)
	p := h.producer()
	h.call(p)
	if len(h.notified) != 1 {
		t.Fatalf("notified = %v; want the one notice", h.notified)
	}

	h.touch(since.Add(10 * time.Minute))
	h.call(p)
	if len(h.notified) != 1 {
		t.Errorf("notified = %v after a same-state rewrite; want nothing more", h.notified)
	}
	if got := h.memory().Episode.Since; !got.Equal(since) {
		t.Errorf("since = %s after the rewrite; want it left at %s", got, since)
	}
}

// TestNotice_DoneChildSendsNoNotice asserts a done child's checks and its Done return call Notify zero times and write no episode key, also when an earlier blocked notice of the same run was sent.
func TestNotice_DoneChildSendsNoNotice(t *testing.T) {
	for _, tt := range []struct {
		name         string
		priorBlocked bool
	}{
		{name: "OnFirstSight"},
		{name: "AfterAnEarlierBlockedNotice", priorBlocked: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			h := newNoticeHarness(t)
			wantNotified, wantKeys := 0, 0
			if tt.priorBlocked {
				h.setStatus(parked(shedengine.StateBlocked))
				h.writeReport(h.touched)
				h.call(h.producer())
				wantNotified = len(h.notified)
				wantKeys = len(readNoticeKeys(noticeEpisodeFile(h.scratch, "Run-Shed")))
				if wantNotified != 1 || wantKeys != 1 {
					t.Fatalf("blocked notices = %d with %d keys; want one of each", wantNotified, wantKeys)
				}
			}

			h.setStatus(shedengine.Status{State: shedengine.StateDone, History: make([]shedengine.HistoryEntry, 5)})
			h.probe = 2 * time.Second
			h.clock.pauseAtSleep = 0
			h.clock.onSleep = func(call int) {
				if call == 2 {
					h.alive = false
				}
			}
			p := h.producer()
			if tt.priorBlocked {
				// The pass that sees the child done ends on the change of state, with no notice of its own.
				if outcome, _, err := p.Call(context.Background()); err != nil || outcome != shedengine.Stuck {
					t.Fatalf("first Call() = %v, %v; want the Stuck of the change to done", outcome, err)
				}
			}
			outcome, _, err := h.untilEnd(p)
			if err != nil || outcome != shedengine.Done {
				t.Fatalf("Call() = %v, %v; want Done once the driver ended", outcome, err)
			}
			if len(h.notified) != wantNotified || len(h.notifyTimes) != wantNotified {
				t.Errorf("notified = %v after %d Notify calls; want only the %d earlier notices", h.notified, len(h.notifyTimes), wantNotified)
			}
			if keys := readNoticeKeys(noticeEpisodeFile(h.scratch, "Run-Shed")); len(keys) > wantKeys {
				t.Errorf("episode keys = %v; want no key for the done child", keys)
			}
		})
	}
}

// TestNotice_AwaitingLeavesTheRelayToTheDriver covers the awaiting child whose driver handed the parent a notice:
// while this batten holds the watched marker its notice waits for the relay, and goes when the driver ends, after three minutes with no report, or ten minutes after the report with no decision.
func TestNotice_AwaitingLeavesTheRelayToTheDriver(t *testing.T) {
	tests := []struct {
		name         string
		parentNotice string
		watched      bool
		// at runs at the second Call's second check, with the stop's since.
		at         func(h *noticeHarness, since time.Time)
		wantNotice bool
	}{
		{name: "NoParentNoticeGoesLikeAnyStop", watched: true, at: func(h *noticeHarness, since time.Time) { h.writeReport(since) }, wantNotice: true},
		{name: "WithoutTheMarkerItGoesAtOnce", parentNotice: "pull request ready", at: func(h *noticeHarness, since time.Time) { h.writeReport(since) }, wantNotice: true},
		{name: "ALiveDriverWithAReportHoldsIt", parentNotice: "pull request ready", watched: true, at: func(h *noticeHarness, since time.Time) {
			h.writeReport(since)
			h.clock.advance(noticeRelayWait - time.Minute)
		}},
		{name: "TheDriverEndingReleasesIt", parentNotice: "pull request ready", watched: true, at: func(h *noticeHarness, since time.Time) {
			h.writeReport(since)
			h.alive = false
		}, wantNotice: true},
		{name: "NoReportAfterThreeMinutesReleasesIt", parentNotice: "pull request ready", watched: true, at: func(h *noticeHarness, since time.Time) { h.clock.advance(noticeReportWait) }, wantNotice: true},
		{name: "TenMinutesAfterTheReportUndecidedReleasesIt", parentNotice: "pull request ready", watched: true, at: func(h *noticeHarness, since time.Time) {
			h.writeReport(since)
			h.clock.advance(noticeRelayWait)
		}, wantNotice: true},
		{name: "ADecisionRecordKeepsItHeld", parentNotice: "pull request ready", watched: true, at: func(h *noticeHarness, since time.Time) {
			h.writeReport(since)
			h.decided = true
			h.clock.advance(noticeRelayWait)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newNoticeHarness(t)
			h.watched = tt.watched
			p := h.producer()
			awaiting := parked(shedengine.StateAwaiting)
			awaiting.ParentNotice = tt.parentNotice
			h.leaveRunning(p, awaiting)
			since := h.touched

			h.secondCheck(func() { tt.at(h, since) })
			h.drive(p)
			if got := len(h.notified) == 1; got != tt.wantNotice {
				t.Errorf("notified = %v; want a notice = %v", h.notified, tt.wantNotice)
			}
		})
	}
}

// TestNotice_LiveArbiterHoldsTheStateChange asserts the notice of a halted or awaiting child, which an ended driver would release at once, is held while a live arbiter or tackler marker is within its deadline plus the report wait.
// It goes once that bound passes, and as today for a dead strand, a missing marker or an unreadable one.
// It is never sent when the arbiter's resume moves the child back to running.
func TestNotice_LiveArbiterHoldsTheStateChange(t *testing.T) {
	liveFor := func(h *noticeHarness, d time.Duration) {
		h.arbiter = ChildArbiter{Deadline: h.clock.Now().Add(d), Alive: true}
		h.arbiterFound = true
	}
	tests := []struct {
		name string
		// at runs at the second check of the Call the test makes, with the driver strand already ended.
		at         func(h *noticeHarness)
		wantNotice bool
	}{
		{name: "ALiveMarkerWithinItsDeadlineHolds", at: func(h *noticeHarness) { liveFor(h, 30*time.Minute) }},
		{name: "ALiveMarkerPastItsDeadlineWithinTheReportWaitHolds", at: func(h *noticeHarness) {
			liveFor(h, 0)
			h.clock.advance(noticeReportWait - 10*time.Second)
		}},
		{name: "ALiveMarkerPastItsDeadlinePlusTheReportWaitSendsTheNotice", at: func(h *noticeHarness) {
			liveFor(h, 0)
			h.clock.advance(noticeReportWait)
		}, wantNotice: true},
		{name: "ALiveTacklerWithinItsDeadlineHolds", at: func(h *noticeHarness) {
			h.tackler = ChildArbiter{Deadline: h.clock.Now().Add(30 * time.Minute), Alive: true}
			h.tacklerFound = true
		}},
		{name: "ALiveTacklerPastItsDeadlinePlusTheReportWaitSendsTheNotice", at: func(h *noticeHarness) {
			h.tackler = ChildArbiter{Deadline: h.clock.Now(), Alive: true}
			h.tacklerFound = true
			h.clock.advance(noticeReportWait)
		}, wantNotice: true},
		{name: "ADeadStrandHoldsNothing", at: func(h *noticeHarness) {
			liveFor(h, 30*time.Minute)
			h.arbiter.Alive = false
		}, wantNotice: true},
		{name: "AMissingMarkerHoldsNothing", at: func(h *noticeHarness) {}, wantNotice: true},
		{name: "AnUnreadableMarkerHoldsNothing", at: func(h *noticeHarness) {
			liveFor(h, 30*time.Minute)
			h.arbiterErr = errors.New("marker boom")
		}, wantNotice: true},
		{name: "AResumeToRunningSendsNothing", at: func(h *noticeHarness) {
			liveFor(h, 30*time.Minute)
			h.arbiterFound = false
			h.setStatus(shedengine.Status{State: shedengine.StateRunning})
		}},
	}
	for _, state := range []shedengine.State{shedengine.StateBlocked, shedengine.StateAwaiting} {
		for _, tt := range tests {
			t.Run(string(state)+"/"+tt.name, func(t *testing.T) {
				h := newNoticeHarness(t)
				p := h.producer()
				h.leaveRunning(p, parked(state))

				h.secondCheck(func() {
					h.alive = false
					tt.at(h)
				})
				h.drive(p)
				if got := len(h.notified); got != btoi(tt.wantNotice) {
					t.Errorf("notified = %v; want %d notice", h.notified, btoi(tt.wantNotice))
				}
			})
		}
	}
}

// btoi is 1 for true and 0 for false.
func btoi(b bool) int {
	if b {
		return 1
	}
	return 0
}

// TestNotice_DeliveryIsRecordedOnlyWhenQueuedAndRetriedWithinItsCaps covers the delivery seams:
// a queued notice is recorded sent, a missing orch strand makes no Notify call and warns once, a failing or unqueued Notify is retried at most once per notice probe and three times, and a restarted batten sends what was not sent.
func TestNotice_DeliveryIsRecordedOnlyWhenQueuedAndRetriedWithinItsCaps(t *testing.T) {
	const probe = 30 * time.Second
	// ready is a harness whose parked child's notice may go at once, checked every ten seconds and probed every thirty.
	ready := func(t *testing.T) *noticeHarness {
		t.Helper()
		h := newNoticeHarness(t)
		h.probe, h.poll = probe, 10*time.Second
		h.setStatus(parked(shedengine.StateBlocked))
		h.writeReport(h.clock.Now())
		return h
	}

	t.Run("AQueuedNoticeIsRecordedSent", func(t *testing.T) {
		h := ready(t)
		h.run(h.producer(), 7)
		if len(h.notified) != 1 {
			t.Errorf("notified = %v; want one notice over seven checks", h.notified)
		}
		if keys := readNoticeKeys(noticeEpisodeFile(h.scratch, "Run-Shed")); len(keys) != 1 || !strings.HasPrefix(keys[0], noticeStateChanged+"|blocked|") {
			t.Errorf("episode keys = %v; want the sent notice's key", keys)
		}
	})

	for _, tt := range []struct {
		name  string
		queue bool
		err   error
	}{
		{name: "AnUnqueuedNoticeIsRetriedOncePerProbeThreeTimes", queue: false},
		{name: "AFailingNotifyIsRetriedOncePerProbeThreeTimes", err: errors.New("queue unavailable")},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			logger.SetOutput(&buf)
			t.Cleanup(func() { logger.SetOutput(logger.DefaultOutput()) })

			h := ready(t)
			h.notifyQueued, h.notifyErr = tt.queue, tt.err
			h.run(h.producer(), 15)
			if len(h.notifyTimes) != noticeMaxAttempts {
				t.Fatalf("Notify calls at %v; want %d, then the notice dropped", h.notifyTimes, noticeMaxAttempts)
			}
			for i := 1; i < len(h.notifyTimes); i++ {
				if gap := h.notifyTimes[i].Sub(h.notifyTimes[i-1]); gap < probe {
					t.Errorf("Notify calls %d and %d are %s apart; want at least the notice probe %s", i-1, i, gap, probe)
				}
			}
			if !strings.Contains(buf.String(), "notice dropped") {
				t.Errorf("log lacks the drop warning: %s", buf.String())
			}
			if keys := readNoticeKeys(noticeEpisodeFile(h.scratch, "Run-Shed")); len(keys) != 0 {
				t.Errorf("episode keys = %v; want none for a notice that was never queued", keys)
			}
		})
	}

	t.Run("NoStrandMakesNoNotifyCallAndWarnsOncePerEpisode", func(t *testing.T) {
		var buf bytes.Buffer
		logger.SetOutput(&buf)
		t.Cleanup(func() { logger.SetOutput(logger.DefaultOutput()) })

		h := ready(t)
		h.strand = false
		p := h.producer()
		h.run(p, 7)
		if len(h.notifyTimes) != 0 {
			t.Errorf("Notify calls at %v; want none with no orch strand recorded", h.notifyTimes)
		}
		if got := strings.Count(buf.String(), "no orch strand is recorded"); got != 1 {
			t.Errorf("no-strand warnings = %d; want 1\nlog: %s", got, buf.String())
		}

		h.strand = true
		h.run(p, 4)
		if len(h.notified) != 1 {
			t.Errorf("notified = %v; want the notice once a strand is recorded", h.notified)
		}
	})

	t.Run("ARestartedBattenSendsWhatWasNotSent", func(t *testing.T) {
		h := ready(t)
		h.strand = false
		h.run(h.producer(), 3)
		h.strand = true
		// The retry spacing outlives the producer: the unsent notice goes at the first pass a notice probe after the last try.
		h.run(h.producer(), 3)
		if len(h.notified) != 1 {
			t.Errorf("notified = %v; want the unsent notice sent by a later pass", h.notified)
		}
	})
}

// TestNotice_LogsEachRunsSessionStateAtAQuietOrAPIErrorFinding drives a quiet and an API-error finding over two live runs,
// and again with the runs' session state fields empty:
// both runs log the finding's condition with the run's state, cause and since, a run with no finding logs none, and the notices sent are the same either way.
// The lines log at Debug, so the capture raises the verbosity to Debug.
// The log is captured process-wide, so the test does not run in parallel.
func TestNotice_LogsEachRunsSessionStateAtAQuietOrAPIErrorFinding(t *testing.T) {
	tests := []struct {
		name          string
		advance       time.Duration
		apiError      bool
		wantCondition string
	}{
		{name: "a quiet finding", advance: 2 * 45 * time.Minute, wantCondition: noticeQuiet},
		{name: "an API-error finding", advance: noticeAPIErrorIdle, apiError: true, wantCondition: noticeAPIError},
		{name: "no finding", advance: time.Second},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			drive := func(withState bool) (notified []string, log string) {
				buf := logcapture.CaptureVerbose(t)
				logger.SetVerbosity(2)
				h := newNoticeHarness(t)
				p := h.producer()
				runs := []AgentActivity{
					{Producer: "t:task:impl", LastActivity: h.clock.Now(), APIError: tt.apiError, APIErrorText: "overloaded"},
					{Producer: "t:task:review", LastActivity: h.clock.Now()},
				}
				if withState {
					runs[0].SessionState, runs[0].SessionCause, runs[0].SessionSince = "idle-stalled", "api-error", "2026-01-01T10:00:00Z"
					runs[1].SessionState, runs[1].SessionCause, runs[1].SessionSince = "busy", "turn", "2026-01-01T09:00:00Z"
				}
				h.runs = runs
				h.clock.advance(tt.advance)
				h.call(p)
				return h.notified, buf.String()
			}

			withState, withStateLog := drive(true)
			withoutState, withoutStateLog := drive(false)
			if !slices.Equal(withState, withoutState) {
				t.Errorf("notices with the state fields = %q, without = %q", withState, withoutState)
			}
			if tt.wantCondition == "" {
				if strings.Contains(withStateLog, "session state at notice") {
					t.Errorf("a call with no finding logged a session state: %q", withStateLog)
				}
				return
			}
			for _, want := range []string{
				"producer=t:task:impl", "state=idle-stalled cause=api-error since=2026-01-01T10:00:00Z",
				"producer=t:task:review", "state=busy cause=turn since=2026-01-01T09:00:00Z",
				"condition=" + tt.wantCondition,
			} {
				if !strings.Contains(withStateLog, want) {
					t.Errorf("log lacks %q: %q", want, withStateLog)
				}
			}
			if got, want := strings.Count(withoutStateLog, "session state at notice"), strings.Count(withStateLog, "session state at notice"); got != want || got == 0 {
				t.Errorf("session state lines without the state fields = %d, with = %d, want equal and nonzero", got, want)
			}
			for _, line := range strings.Split(withStateLog, "\n") {
				if strings.Contains(line, "level=INFO") && strings.Contains(line, "session state at notice") {
					t.Errorf("session state logged at INFO, want Debug: %s", line)
				}
			}
		})
	}
}
