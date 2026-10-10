// notice_test.go covers the notice step: the rendered line, each condition's one notice per episode, when a parked stop's notice may go, the awaiting relay rule, the quiet condition's guards, delivery and its retries, and a restart over the same scratch directory.
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
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/testkit/logcapture"
)

// noticeHarness drives producers whose status, driver liveness, stop report and orch strand a test sets between and during Calls.
// Every producer it builds shares the scratch directory, so a second one models a batten restart.
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

	// runs, waitLive and activityErr are what the fake Activity seam reports; activityReads counts its calls.
	runs          []AgentActivity
	waitLive      bool
	activityErr   error
	activityReads int

	quiet time.Duration
	probe time.Duration
	poll  time.Duration

	// started is the fake clock's time when the harness was built, and asleep the time suspend has taken out of the awake clock since.
	started time.Time
	asleep  time.Duration
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
	h.started = h.clock.Now()
	h.touch(h.started)
	return h
}

// suspend moves the fake wall clock forward by d without moving the awake clock, as a machine sleeping for d does.
func (h *noticeHarness) suspend(d time.Duration) {
	h.clock.advance(d)
	h.asleep += d
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

// parked is a child halted in state, with a history of two entries.
func parked(state shedengine.State) shedengine.Status {
	return shedengine.Status{State: state, Error: "stuck", CurrentProducer: "Publish", History: make([]shedengine.HistoryEntry, 2)}
}

// writeReport makes the driver's park marker read as holding a stop report written at at.
func (h *noticeHarness) writeReport(at time.Time) {
	h.reportPath, h.reportAt, h.reportFound = "/wt/task/_lyx/stop-report.md", at, true
}

// producer builds a fresh producer over the harness's scratch directory, so a second one models a restart.
func (h *noticeHarness) producer() shedengine.ShedProducer {
	deps := InnerRunDeps{
		Spawn:         func(context.Context) error { return nil },
		ResolveStatus: func() (string, string, error) { return h.statusPath, "", nil },
		ReadStatus: func(string, string) (shedengine.Status, bool, error) {
			return h.status, true, nil
		},
		Sleep:          h.clock.Sleep,
		Now:            h.clock.Now,
		Awake:          func() time.Duration { return h.clock.Now().Sub(h.started) - h.asleep },
		PauseRequested: h.clock.pauseRequested,
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
		NoticeQuiet: h.quiet,
		Activity: func(context.Context) ([]AgentActivity, bool, error) {
			h.activityReads++
			return h.runs, h.waitLive, h.activityErr
		},
		AttachDir: func() (string, error) { return "/wt/task", nil },
	}
	return NewInnerRun("Run-Shed", "task", deps, h.poll, h.scratch, testGrace)
}

// run makes one Call whose wait ends after checks checks unless something else ends it first, and requires it to end as a Stuck.
func (h *noticeHarness) run(p shedengine.ShedProducer, checks int) {
	h.t.Helper()
	h.clock.pauseAtSleep = h.clock.sleepCalls + checks
	outcome, _, err := p.Call(context.Background())
	if err != nil || outcome != shedengine.Stuck {
		h.t.Fatalf("Call() = %v, %v; want a Stuck whatever the notice step did", outcome, err)
	}
}

// call makes one Call whose wait ends at its first check.
func (h *noticeHarness) call(p shedengine.ShedProducer) { h.t.Helper(); h.run(p, 1) }

// leaveRunning drives one Call over a running child that turns into status at its first check, so the Call returns on the state change before the new state's notice has had a wait of its own.
func (h *noticeHarness) leaveRunning(p shedengine.ShedProducer, status shedengine.Status) {
	h.t.Helper()
	h.setStatus(shedengine.Status{State: shedengine.StateRunning})
	h.clock.pauseAtSleep = 0
	h.clock.onSleep = func(int) { h.setStatus(status) }
	defer func() { h.clock.onSleep = nil }()
	_, ptr, err := p.Call(context.Background())
	if err != nil || !strings.HasPrefix(ptr.Path, "child running → ") {
		h.t.Fatalf("Call() = %+v, %v; want the return for the change out of running", ptr, err)
	}
}

// secondCheck runs fn at the second check of the Call the test makes next, after the first check has had its chance to send, and ends that Call after four checks.
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
	long := shedengine.Status{State: shedengine.StateFailed, Error: "boom\nsecond\x1b[0m line " + strings.Repeat("x", 300), History: make([]shedengine.HistoryEntry, 7)}
	tests := []struct {
		name string
		line noticeLine
		want []string
		deny []string
	}{
		{
			name: "SinceHistoryAndAReportPath",
			line: noticeLine{slug: "task", condition: "child left running", status: long, since: since, report: reportClause("/wt/task/stop.md", true), attachDir: "/wt/task"},
			want: []string{"task", "child left running", "failed", "boom second", "since 2026-01-02T03:04:05Z", "history 7", "report /wt/task/stop.md"},
		},
		{
			name: "NoReportYet",
			line: noticeLine{slug: "task", condition: "child left running", status: parked(shedengine.StateBlocked), since: since, report: reportClause("", false), attachDir: "/wt/task"},
			want: []string{"since 2026-01-02T03:04:05Z", "history 2", "report none yet"},
		},
		{
			name: "ADoneNoticeCarriesNoReportAndNoError",
			line: noticeLine{slug: "task", condition: "child left running", status: shedengine.Status{State: shedengine.StateDone}, since: since, attachDir: "/wt"},
			want: []string{"child state done", "since 2026-01-02T03:04:05Z", "history 0"},
			deny: []string{"report", "error:"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			line := renderNotice(tt.line)
			if !strings.HasPrefix(line, noticePrefix) || !strings.HasSuffix(line, "cd "+tt.line.attachDir+" && lyx reed attach") {
				t.Errorf("notice = %q; want the prefix and the attach command at the end", line)
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

	h.call(p)
	h.call(p)
	if len(h.notified) != 1 || !strings.Contains(h.notified[0], "driver strand is dead") || !strings.Contains(h.notified[0], "since ") || !strings.Contains(h.notified[0], "history 0") {
		t.Fatalf("notified = %v; want exactly one dead-driver notice with its since and history", h.notified)
	}
}

func TestNotice_QuietOnlyForRunningChildWithLiveDriver(t *testing.T) {
	t.Run("IdleAgentsGiveOneAfterTheWindow", func(t *testing.T) {
		h := newNoticeHarness(t)
		p := h.producer()
		h.runs = []AgentActivity{{Producer: "t:task:impl", LastActivity: h.clock.Now()}, {Producer: "t:task:review", LastActivity: h.clock.Now().Add(-time.Hour)}}
		h.clock.advance(44 * time.Minute)
		h.call(p)
		if len(h.notified) != 0 {
			t.Fatalf("notified = %v; want none inside the quiet window", h.notified)
		}
		h.clock.advance(time.Minute)
		h.call(p)
		h.call(p)
		if len(h.notified) != 1 || !strings.Contains(h.notified[0], "agents idle for 45m") {
			t.Fatalf("notified = %v; want exactly one quiet notice measured from the newest run", h.notified)
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

	// The sleep subtests run one Call, since the suspended time is kept per Call: onCheck runs fn at the nth check's sleep, before that check judges.
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

func TestSleepGap(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		prevWall, wall   time.Duration
		prevAwake, awake time.Duration
		want             time.Duration
	}{
		{name: "HoursOfWallAgainstSecondsAwake", prevWall: 0, wall: 3 * time.Hour, prevAwake: 0, awake: 5 * time.Second, want: 3*time.Hour - 5*time.Second},
		{name: "ExcessAtTheToleranceIsZero", prevWall: 0, wall: time.Hour, prevAwake: 0, awake: time.Hour - sleepGapTolerance, want: 0},
		{name: "ExcessJustOverTheToleranceCounts", prevWall: 0, wall: time.Hour, prevAwake: 0, awake: time.Hour - sleepGapTolerance - time.Second, want: sleepGapTolerance + time.Second},
		{name: "EqualDeltasAreZero", prevWall: time.Hour, wall: 2 * time.Hour, prevAwake: 10 * time.Second, awake: time.Hour + 10*time.Second, want: 0},
		{name: "WallBehindAwakeIsZero", prevWall: 0, wall: time.Minute, prevAwake: 0, awake: time.Hour, want: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := sleepGap(int64(tt.prevWall), int64(tt.wall), tt.prevAwake, tt.awake); got != tt.want {
				t.Errorf("sleepGap = %s; want %s", got, tt.want)
			}
		})
	}
}

func TestNotice_APIErrorStall(t *testing.T) {
	h := newNoticeHarness(t)
	p := h.producer()
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
	if len(h.notified) != 1 || !strings.Contains(h.notified[0], "agent t:task:impl hit an API error: overloaded please retry") {
		t.Fatalf("notified = %v; want exactly one api-error notice naming the producer and the one-line error", h.notified)
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

	// A sleep after the stall is not time the error has stood: the notice waits for the awake idle time, within one Call.
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
		Sleep:          h.clock.Sleep,
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
// and always sent by the wait of the Call that begins in the new state, never by the Call that saw the change.
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
			wantReport: "report /wt/task/_lyx/stop-report.md",
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
			if _, _, err := p.Call(context.Background()); err != nil {
				t.Fatalf("Call() error = %v", err)
			}
			if sentBefore() != 0 || len(h.notified) != 1 {
				t.Fatalf("notices before the second check = %d, in all = %v; want none, then exactly one", sentBefore(), h.notified)
			}
			line := h.notified[0]
			for _, want := range []string{"child state " + string(tt.state), "since " + since.UTC().Format(time.RFC3339), "history 2", tt.wantReport} {
				if !strings.Contains(line, want) {
					t.Errorf("notice = %q; want it to contain %q", line, want)
				}
			}
			if tt.wantReport == "report none yet" && strings.Contains(line, "report /") {
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
	if got := p.(*innerRunProducer).episode.since; !got.Equal(since) {
		t.Errorf("since = %s after the rewrite; want it left at %s", got, since)
	}
}

// TestNotice_DoneChildNotifiesOnFirstSightAndTheDoneReturnAttemptsAPendingOne asserts a done child's notice goes at once with no report, and that the Done return makes one last attempt of a notice that could not be sent before.
func TestNotice_DoneChildNotifiesOnFirstSightAndTheDoneReturnAttemptsAPendingOne(t *testing.T) {
	t.Run("OnFirstSightWithNoReport", func(t *testing.T) {
		h := newNoticeHarness(t)
		h.status = shedengine.Status{State: shedengine.StateDone, History: make([]shedengine.HistoryEntry, 5)}
		h.alive = false
		h.writeReport(h.clock.Now())
		outcome, _, err := h.producer().Call(context.Background())
		if err != nil || outcome != shedengine.Done {
			t.Fatalf("Call() = %v, %v; want Done", outcome, err)
		}
		if len(h.notified) != 1 || strings.Contains(h.notified[0], "report") || !strings.Contains(h.notified[0], "child state done") || !strings.Contains(h.notified[0], "history 5") {
			t.Errorf("notified = %v; want one done notice with no report", h.notified)
		}
	})

	for _, tt := range []struct {
		name         string
		strandAtDone bool
		wantNotified int
		wantWarn     string
	}{
		{name: "ALastAttemptReachesAnOrchThatRecordedAStrand", strandAtDone: true, wantNotified: 1},
		{name: "ALastAttemptWithNoStrandIsLogged", strandAtDone: false, wantNotified: 0, wantWarn: "notice lost"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			logger.SetOutput(&buf)
			t.Cleanup(func() { logger.SetOutput(os.Stderr) })

			h := newNoticeHarness(t)
			h.status = shedengine.Status{State: shedengine.StateDone}
			h.strand = false
			h.probe = 2 * time.Second
			h.clock.pauseAtSleep = 0
			h.clock.onSleep = func(call int) {
				if call == 2 {
					h.alive = false
					h.strand = tt.strandAtDone
				}
			}
			outcome, _, err := h.producer().Call(context.Background())
			if err != nil || outcome != shedengine.Done {
				t.Fatalf("Call() = %v, %v; want Done once the driver ended", outcome, err)
			}
			if len(h.notified) != tt.wantNotified {
				t.Errorf("notified = %v; want %d notices", h.notified, tt.wantNotified)
			}
			if tt.wantWarn != "" && !strings.Contains(buf.String(), tt.wantWarn) {
				t.Errorf("log = %q; want %q", buf.String(), tt.wantWarn)
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
			if _, _, err := p.Call(context.Background()); err != nil {
				t.Fatalf("Call() error = %v", err)
			}
			if got := len(h.notified) == 1; got != tt.wantNotice {
				t.Errorf("notified = %v; want a notice = %v", h.notified, tt.wantNotice)
			}
		})
	}
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
			t.Cleanup(func() { logger.SetOutput(os.Stderr) })

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
		t.Cleanup(func() { logger.SetOutput(os.Stderr) })

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
		h.run(h.producer(), 1)
		if len(h.notified) != 1 {
			t.Errorf("notified = %v; want the unsent notice sent by the restarted producer", h.notified)
		}
	})
}

// TestNotice_LogsEachRunsSessionStateAtAQuietOrAPIErrorFinding drives a quiet and an API-error finding over two live runs,
// and again with the runs' session state fields empty:
// both runs log the finding's condition with the run's state, cause and since, a run with no finding logs none, and the notices sent are the same either way.
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
		})
	}
}
