// notice_test.go covers the notice step: the rendered line, each condition's one notice per episode, when a parked stop's notice may go, the awaiting relay rule, the quiet condition's guards, delivery and its retries, and a restart over the same scratch directory.
// The tests that capture log output replace the process-global logger output, so they do not run in parallel.

package battenshed

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/shedengine"
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

	quiet time.Duration
	probe time.Duration
	poll  time.Duration
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
		AttachDir:   func() (string, error) { return "/wt/task", nil },
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
	t.Run("FiresAfterTheWindow", func(t *testing.T) {
		h := newNoticeHarness(t)
		p := h.producer()
		h.clock.advance(44 * time.Minute)
		h.call(p)
		if len(h.notified) != 0 {
			t.Fatalf("notified = %v; want none inside the quiet window", h.notified)
		}
		h.clock.advance(time.Minute)
		h.call(p)
		h.call(p)
		if len(h.notified) != 1 || !strings.Contains(h.notified[0], "unchanged for 45m0s") {
			t.Fatalf("notified = %v; want exactly one quiet notice", h.notified)
		}
	})

	t.Run("NotForADeadDriver", func(t *testing.T) {
		h := newNoticeHarness(t)
		p := h.producer()
		h.alive = false
		h.clock.advance(time.Hour)
		h.call(p)
		if len(h.notified) != 1 || strings.Contains(h.notified[0], "unchanged") {
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
		if len(h.notified) != 1 || strings.Contains(h.notified[0], "unchanged") {
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
