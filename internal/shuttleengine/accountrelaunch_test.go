// accountrelaunch_test.go covers how a run relaunches its session under another account after a usage limit.
// The rows drive Wait over a held, usage-limited turn end and a start over a usage-limited skill-load turn.
// Both run on fakeReed's hold ops and a fake clock.

package shuttleengine

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/testkit/permkit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// switchFakeEngine is a hookStateSkillEngine that also resumes sessions, reads the usage limit and switches accounts.
// A turn end whose events line is limitLine reads as the provider's usage limit.
// Prepare records every spec and launch it was given.
// A Prepare of a spec that resumes a session returns that session's id, and a later fresh one returns freshSessionID.
type switchFakeEngine struct {
	*hookStateSkillEngine

	// limitLine is the turn end line the engine reads as the usage limit.
	limitLine string
	// switchAnswers is what SwitchAccount answers, call by call, the last answer sticking; none answers false.
	switchAnswers []bool
	// onSwitch runs inside every SwitchAccount call.
	onSwitch func()
	// prepareFailAt is the Prepare call that fails, counted from one; zero fails none.
	prepareFailAt int
	// freshSessionID is the session id a fresh Prepare returns once the original launches are past.
	freshSessionID string
	// originalLaunches is how many Prepare calls are the run's own launch before any relaunch.
	// It is one for a start and zero for a Wait.
	originalLaunches int
	// checkErr is what CheckResume returns once checkErrAfter calls have succeeded.
	checkErr error
	// checkErrAfter is how many CheckResume calls succeed before checkErr applies; zero applies it to every call.
	checkErrAfter int
	// switchErr is what SwitchAccount fails with once its call is recorded.
	switchErr error
	// carried is what CarryTranscript reports: whether it copied the transcript.
	carried bool

	mu             sync.Mutex
	checks         int
	prepared       []Spec
	launches       []Launch
	switchRunDirs  []string
	switchTurnEnds []string
}

// newSwitchFakeEngine returns an engine whose first launch is prepareLaunch and whose startup reads ready.
// It reads "STOP:limit" as the usage limit and answers the account switch with true.
func newSwitchFakeEngine(prepareLaunch Launch) *switchFakeEngine {
	inner := &skillFakeEngine{
		fakeEngine: &fakeEngine{PrepareLaunch: prepareLaunch, StartupScript: []StartupState{StartupReady}},
		Timeout:    time.Minute,
		Reports:    []SkillLoadReport{{Verified: true, Loaded: []string{"a"}}},
	}
	return &switchFakeEngine{hookStateSkillEngine: &hookStateSkillEngine{skillFakeEngine: inner}, limitLine: "STOP:limit", switchAnswers: []bool{true}, freshSessionID: "session-fresh"}
}

func (e *switchFakeEngine) Prepare(runDir string, spec Spec, cfg Config) (Launch, error) {
	e.mu.Lock()
	e.prepared = append(e.prepared, spec)
	call := len(e.prepared)
	e.mu.Unlock()
	if call == e.prepareFailAt {
		return Launch{}, errors.New("switchFakeEngine: scripted Prepare failure")
	}
	launch, err := e.skillFakeEngine.Prepare(runDir, spec, cfg)
	if err != nil {
		return Launch{}, err
	}
	launch.ResumeCmd = fmt.Sprintf("resume-%d", call)
	switch {
	case spec.ResumeSessionID != "":
		launch.SessionID = spec.ResumeSessionID
	case call > e.originalLaunches:
		launch.SessionID = e.freshSessionID
	}
	if spec.SkipSkillLoadTurn {
		launch.Cmd, launch.PromptLine = fmt.Sprintf("cmd-%d", call), ""
	}
	e.mu.Lock()
	e.launches = append(e.launches, launch)
	e.mu.Unlock()
	return launch, nil
}

func (e *switchFakeEngine) CarryTranscript(string, string) (bool, error) { return e.carried, nil }

func (e *switchFakeEngine) CheckResume(string, string) (string, error) {
	e.mu.Lock()
	e.checks++
	call := e.checks
	e.mu.Unlock()
	if call <= e.checkErrAfter {
		return "", nil
	}
	return "", e.checkErr
}

func (e *switchFakeEngine) limited(turnEnd Event) bool {
	return e.limitLine != "" && strings.TrimSpace(string(turnEnd.Raw)) == e.limitLine
}

func (e *switchFakeEngine) TurnEndActivity(turnEnd Event) TurnEndActivity {
	if e.limited(turnEnd) {
		return TurnEndActivity{APIError: true, APIErrorText: "limit", UsageLimit: UsageLimit{Limited: true}}
	}
	return e.hookStateSkillEngine.TurnEndActivity(turnEnd)
}

func (e *switchFakeEngine) TurnEndUsageLimited(turnEnd Event) bool { return e.limited(turnEnd) }

func (e *switchFakeEngine) SwitchAccount(runDir string, turnEnd Event) (bool, error) {
	e.mu.Lock()
	e.switchRunDirs = append(e.switchRunDirs, runDir)
	e.switchTurnEnds = append(e.switchTurnEnds, string(turnEnd.Raw))
	call := len(e.switchRunDirs)
	onSwitch := e.onSwitch
	e.mu.Unlock()
	if onSwitch != nil {
		onSwitch()
	}
	if e.switchErr != nil {
		return false, e.switchErr
	}
	if len(e.switchAnswers) == 0 {
		return false, nil
	}
	return e.switchAnswers[min(call-1, len(e.switchAnswers)-1)], nil
}

var (
	_ AccountSwitcher     = (*switchFakeEngine)(nil)
	_ UsageLimitReader    = (*switchFakeEngine)(nil)
	_ SessionResumer      = (*switchFakeEngine)(nil)
	_ TranscriptCarrier   = (*switchFakeEngine)(nil)
	_ SessionSignalParser = (*switchFakeEngine)(nil)
)

// relaunchReed is a skillReed whose one strand, strand-1, is live until EndStrandProcess holds it, and gone once removed.
// RelaunchStrand brings it live again and plays the launch line's load turn, as a relaunched pane would.
// onRelaunch runs after that.
type relaunchReed struct {
	*skillReed

	// endLeavesDead makes a failing EndStrandProcess leave the strand not live, as a process that already exited.
	endLeavesDead bool
	// onRelaunch runs inside every successful RelaunchStrand.
	onRelaunch func()
	// beforeRelaunch runs at the start of every RelaunchStrand, before the fake records the call.
	beforeRelaunch func()

	mu                  sync.Mutex
	live, held, removed bool
}

func newRelaunchReed(skill *skillReed) *relaunchReed {
	return &relaunchReed{skillReed: skill, live: true}
}

func (r *relaunchReed) Status() (reedengine.StatusResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.removed {
		return reedengine.StatusResult{}, nil
	}
	return reedengine.StatusResult{Strands: []reedengine.StrandStatus{{GUID: "strand-1", PaneID: "%1", Live: r.live, Held: r.held}}}, nil
}

func (r *relaunchReed) RemoveStrand(guid string, recursive bool) (reedengine.Removed, error) {
	removed, err := r.fakeReed.RemoveStrand(guid, recursive)
	r.mu.Lock()
	r.removed = true
	r.mu.Unlock()
	return removed, err
}

func (r *relaunchReed) EndStrandProcess(guid string) error {
	err := r.fakeReed.EndStrandProcess(guid)
	r.mu.Lock()
	defer r.mu.Unlock()
	if err == nil || r.endLeavesDead {
		r.live, r.held = false, true
	}
	return err
}

func (r *relaunchReed) RelaunchStrand(guid string, spec reedengine.RelaunchSpec) error {
	if r.beforeRelaunch != nil {
		r.beforeRelaunch()
	}
	if err := r.fakeReed.RelaunchStrand(guid, spec); err != nil {
		return err
	}
	r.mu.Lock()
	r.live, r.held = true, false
	r.mu.Unlock()
	if list, ok := strings.CutPrefix(spec.Cmd, "LOAD:"); ok {
		if err := r.skillReed.playLoad(list); err != nil {
			return err
		}
	}
	if r.onRelaunch != nil {
		r.onRelaunch()
	}
	return nil
}

func (r *relaunchReed) stranded() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return !r.removed
}

var _ ReedOps = (*relaunchReed)(nil)

// relaunchSnapshot is what a run's record and events file held when reed was asked to relaunch the strand.
type relaunchSnapshot struct {
	taken      bool
	record     RunState
	found      bool
	err        error
	eventsSize int64
}

// recordAtRelaunch returns a relaunchReed.beforeRelaunch hook that fills into from the run directory runDir names.
func recordAtRelaunch(runDir func() string, into *relaunchSnapshot) func() {
	return func() {
		dir := runDir()
		into.taken = true
		into.record, into.found, into.err = loadRunState(dir)
		into.eventsSize = eventsSize(filepath.Join(dir, eventsFileName))
	}
}

// checkRecordedBeforeRelaunch fails unless the record already named the relaunched launch.
// The check reads the snapshot taken when reed was asked to relaunch the strand.
// That is the new session, its deferred prompt line, the cleared started flag, the relaunching mark,
// and the prompt and launch offsets at the events file's size.
func checkRecordedBeforeRelaunch(t *testing.T, snapshot relaunchSnapshot, wantSessionID, wantPromptLine string) {
	t.Helper()
	if !snapshot.taken {
		t.Fatal("reed was never asked to relaunch the strand")
	}
	if snapshot.err != nil || !snapshot.found {
		t.Fatalf("the record when reed was asked to relaunch: found %v, error %v", snapshot.found, snapshot.err)
	}
	rs := snapshot.record
	if rs.SessionID != wantSessionID || rs.PromptLine != wantPromptLine || rs.Started || rs.RelaunchingAt == "" ||
		rs.PromptOffset != snapshot.eventsSize || rs.LaunchOffset != snapshot.eventsSize {
		t.Errorf("record when reed was asked to relaunch = %+v; want session %q, prompt line %q, not started, marked relaunching, and both offsets at %d",
			rs, wantSessionID, wantPromptLine, snapshot.eventsSize)
	}
}

// callsOf returns the tags of the reed calls log holds that name op, in call order.
func callsOf(log []string, op string) int {
	return len(slices.DeleteFunc(slices.Clone(log), func(call string) bool { return call != op }))
}

// waitRelaunchRow is one case of TestWait_UsageLimitSwitchRelaunchesInTheRun that drives a Wait over a held turn end.
type waitRelaunchRow struct {
	name string
	// events is the events file the run starts over; it defaults to a started turn that ended on the usage limit.
	events string
	// startup is the engine's startup script, ready when empty.
	startup []StartupState
	// switchAnswers is the account switch's answers, true when nil.
	switchAnswers []bool
	// retries is Spec.APIErrorRetries.
	retries int
	// timeout is the run's timeout, an hour when zero.
	timeout time.Duration
	// finishes makes the relaunched pane finish the task: it appends a finished turn and writes the output file.
	finishes bool
	// seedStopped records a stop in the run's record before Wait begins.
	seedStopped bool
	// copied, launchOffset and promptOffset seed the record's TranscriptCopied, LaunchOffset and PromptOffset.
	// The latest launch then resumed a copied session.
	copied       bool
	launchOffset int64
	promptOffset int64
	// setup changes the harness before Wait runs.
	setup func(h *waitRelaunchHarness)
	// check judges the harness after Wait returned.
	check func(t *testing.T, h *waitRelaunchHarness)
}

// waitRelaunchHarness is a run over a fake reed and engine, and what Wait left behind.
type waitRelaunchHarness struct {
	t      *testing.T
	fx     *fixture
	engine *switchFakeEngine
	reed   *relaunchReed
	run    *Run
	output string
	runDir string
	events string

	// atRelaunch is what the record held when reed was asked to relaunch the strand.
	atRelaunch relaunchSnapshot

	result  Result
	err     error
	record  RunState
	notices []string
	ends    []SessionEnd
}

const defaultLimitedEvents = "START\nSTOP:limit\n"

// newWaitRelaunchHarness builds the harness for row; the run is not waited on yet.
func newWaitRelaunchHarness(t *testing.T, row waitRelaunchRow) *waitRelaunchHarness {
	t.Helper()
	h := &waitRelaunchHarness{t: t}
	h.engine = newSwitchFakeEngine(Launch{Cmd: "cmd", SessionID: "session-1"})
	if row.switchAnswers != nil {
		h.engine.switchAnswers = row.switchAnswers
	}
	if len(row.startup) > 0 {
		h.engine.StartupScript = row.startup
	}
	h.reed = newRelaunchReed(&skillReed{fakeReed: &fakeReed{AddStrandResult: reedengine.Strand{GUID: "strand-1"}}})
	h.reed.beforeRelaunch = recordAtRelaunch(func() string { return h.runDir }, &h.atRelaunch)
	clock := newFakeClock(time.Now())
	h.fx = newFixture(t, h.reed, h.engine, withConfig(fastConfig), withClock(clock), withSeparateRunDir())
	h.fx.Runner.SetNotifier(func(line string) error {
		h.notices = append(h.notices, line)
		return nil
	})
	h.fx.Runner.SetSessionEnd(func(end SessionEnd) { h.ends = append(h.ends, end) })

	h.runDir = filepath.Join(h.fx.RunRoot, "run-1")
	if err := os.MkdirAll(h.runDir, 0o755); err != nil {
		t.Fatalf("create the run directory: %v", err)
	}
	h.events = filepath.Join(h.runDir, eventsFileName)
	events := row.events
	if events == "" {
		events = defaultLimitedEvents
	}
	if err := os.WriteFile(h.events, []byte(events), 0o644); err != nil {
		t.Fatalf("seed the events file: %v", err)
	}
	h.output = filepath.Join(t.TempDir(), "out.md")
	state := RunState{
		RunID: "run-1", StrandGUID: "strand-1", SessionID: "session-1", EventsPath: h.events,
		OutputFiles: []string{h.output}, Outcome: runOutcomeRunning, Started: true,
		TranscriptCopied: row.copied, LaunchOffset: row.launchOffset, PromptOffset: row.promptOffset,
	}
	if row.seedStopped {
		stopped := state
		stopped.Outcome = runOutcomeStopped
		if err := saveRunState(h.runDir, stopped); err != nil {
			t.Fatalf("seed the stopped record: %v", err)
		}
	}
	timeout := row.timeout
	if timeout == 0 {
		timeout = time.Hour
	}
	h.run = h.fx.newRun(Spec{Prompt: "do the task", OutputFiles: []string{h.output}, Timeout: timeout, APIErrorRetries: row.retries, KeepPane: true},
		withRunState(state), withRunDir(h.runDir), withRunClock(clock, clock.Now().Add(timeout)))
	if row.finishes {
		h.reed.onRelaunch = func() {
			if err := os.WriteFile(h.output, []byte("done"), 0o644); err != nil {
				t.Errorf("write the output file: %v", err)
			}
			for _, line := range []string{"SESSIONSTART:startup", "START", "STOP:done"} {
				appendEventsLine(t, h.events, line)
			}
		}
	}
	if row.setup != nil {
		row.setup(h)
	}
	return h
}

// wait runs the harness's Wait and reads the run record it left.
func (h *waitRelaunchHarness) wait() {
	h.t.Helper()
	h.result, h.err = h.run.Wait()
	record, found, err := loadRunState(h.runDir)
	if err != nil || !found {
		h.t.Fatalf("read the run record: found %v, err %v", found, err)
	}
	h.record = record
}

// checkRelaunched fails unless the run relaunched once inside itself and ended done with one session end.
func (h *waitRelaunchHarness) checkRelaunched(t *testing.T) {
	t.Helper()
	if h.err != nil || h.result.Outcome != OutcomeDone {
		t.Fatalf("Wait() = outcome %q, error %v; want done", h.result.Outcome, h.err)
	}
	if got := callsOf(h.reed.CallLog, "RelaunchStrand"); got != 1 || len(h.reed.RelaunchStrandCalls) != 1 {
		t.Errorf("RelaunchStrand calls = %d; want 1", got)
	}
	if len(h.ends) != 1 || h.ends[0].Outcome != OutcomeDone {
		t.Errorf("session ends = %+v; want one, done", h.ends)
	}
}

// TestWait_UsageLimitSwitchRelaunchesInTheRun pins the relaunch of a run's session under the switched account.
func TestWait_UsageLimitSwitchRelaunchesInTheRun(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []waitRelaunchRow{
		{
			name: "ASwitchRelaunchesTheSessionInsideTheRun", finishes: true,
			check: func(t *testing.T, h *waitRelaunchHarness) {
				h.checkRelaunched(t)
				checkRecordedBeforeRelaunch(t, h.atRelaunch, "session-1", "")
				calls := h.reed.CallLog
				if end, relaunch := slices.Index(calls, "EndStrandProcess"), slices.Index(calls, "RelaunchStrand"); end < 0 || relaunch < end {
					t.Errorf("reed calls = %v; want EndStrandProcess, then RelaunchStrand", calls)
				}
				second := h.engine.launches[0]
				want := reedengine.RelaunchSpec{Cmd: second.Cmd, ResumeCmd: second.ResumeCmd, SessionID: "session-1"}
				if got := h.reed.RelaunchStrandCalls[0]; got.GUID != "strand-1" || got.Spec != want {
					t.Errorf("RelaunchStrand = %+v; want strand-1 relaunched with %+v", got, want)
				}
				if h.record.StrandGUID != "strand-1" || h.record.SessionID != "session-1" || h.record.PromptLine != "" || h.record.RelaunchingAt != "" {
					t.Errorf("record = %+v; want strand-1 and session-1 kept, no prompt line and no relaunching mark", h.record)
				}
				if spec := h.engine.prepared[0]; spec.ResumeSessionID != "session-1" || spec.Prompt != accountSwitchContinueLine || !spec.SkipSkillLoadTurn {
					t.Errorf("the relaunch's Prepare spec = %+v; want session-1 resumed with the continue line and no skill-load turn", spec)
				}
				if len(h.notices) != 0 {
					t.Errorf("notices = %q; want no hold notice", h.notices)
				}
			},
		},
		{
			name: "ATrustDialogOverTheOldSessionsLinesIsDismissedAndTheRunStartsOnAReadyCapture", finishes: true,
			events:  "SESSIONSTART:startup\nSTART\nSTOP:limit\n",
			startup: []StartupState{StartupTrustPrompt, StartupReady},
			check: func(t *testing.T, h *waitRelaunchHarness) {
				h.checkRelaunched(t)
				if !slices.ContainsFunc(h.reed.SendKeyCalls, func(call struct{ GUID, Key string }) bool { return call.Key == "Enter" }) {
					t.Errorf("keys sent = %v; want the trust dialog dismissed", h.reed.SendKeyCalls)
				}
				if !h.record.Started {
					t.Error("the record reads not started; want it started once the pane showed ready")
				}
			},
		},
		{
			name: "APrepareThatFailsEndsTheRunDiedAndRemovesTheStrand", setup: func(h *waitRelaunchHarness) { h.engine.prepareFailAt = 1 },
			check: func(t *testing.T, h *waitRelaunchHarness) {
				if h.err != nil || h.result.Outcome != OutcomeDied || h.record.Outcome != string(OutcomeDied) {
					t.Errorf("Wait() = outcome %q, error %v, record %q; want died", h.result.Outcome, h.err, h.record.Outcome)
				}
				if h.reed.stranded() || len(h.reed.RelaunchStrandCalls) != 0 || len(h.ends) != 1 {
					t.Errorf("strand left up %v, relaunches %d, session ends %d; want it removed, none relaunched, one end", h.reed.stranded(), len(h.reed.RelaunchStrandCalls), len(h.ends))
				}
			},
		},
		{
			name: "ACheckRefusalOtherThanNoTranscriptEndsTheRunDiedAndRemovesTheStrand", setup: func(h *waitRelaunchHarness) { h.engine.checkErr = errors.New("session is gone") },
			check: func(t *testing.T, h *waitRelaunchHarness) {
				if h.err != nil || h.result.Outcome != OutcomeDied || h.reed.stranded() || len(h.reed.RelaunchStrandCalls) != 0 {
					t.Errorf("Wait() = outcome %q, error %v, strand left up %v; want died and removed with nothing relaunched", h.result.Outcome, h.err, h.reed.stranded())
				}
			},
		},
		{
			name: "APaneThatNeverReachesReadyEndsTheRunDied", startup: []StartupState{StartupPending},
			check: func(t *testing.T, h *waitRelaunchHarness) {
				if h.err != nil || h.result.Outcome != OutcomeDied || h.reed.stranded() || len(h.ends) != 1 {
					t.Errorf("Wait() = outcome %q, error %v, strand left up %v, session ends %d; want died, removed, one end", h.result.Outcome, h.err, h.reed.stranded(), len(h.ends))
				}
			},
		},
		{
			name: "APaneThatNeverReachesReadyBeforeTheRunsDeadlineEndsTheRunTimeout", startup: []StartupState{StartupPending}, timeout: 10 * time.Second,
			check: func(t *testing.T, h *waitRelaunchHarness) {
				if h.err != nil || h.result.Outcome != OutcomeTimeout || h.reed.stranded() {
					t.Errorf("Wait() = outcome %q, error %v, strand left up %v; want timeout and removed", h.result.Outcome, h.err, h.reed.stranded())
				}
			},
		},
		{
			name: "ACheckRefusedWithNoTranscriptRelaunchesFresh", finishes: true,
			setup: func(h *waitRelaunchHarness) { h.engine.checkErr = fmt.Errorf("check: %w", ErrNoTranscript) },
			check: func(t *testing.T, h *waitRelaunchHarness) {
				h.checkRelaunched(t)
				checkRecordedBeforeRelaunch(t, h.atRelaunch, "session-fresh", "")
				if spec := h.engine.prepared[0]; spec.ResumeSessionID != "" || spec.Prompt != "do the task" || spec.SkipSkillLoadTurn {
					t.Errorf("the relaunch's Prepare spec = %+v; want a fresh launch of the run's own prompt", spec)
				}
				if got := h.reed.RelaunchStrandCalls[0].Spec.SessionID; got != "session-fresh" || h.record.SessionID != "session-fresh" {
					t.Errorf("relaunched session %q, record %q; want session-fresh in both", got, h.record.SessionID)
				}
			},
		},
		{
			name: "AStopStrandBetweenTheMarkAndTheRecordWriteEndsTheRunWithItsIdentityAndOutcomeDied",
			setup: func(h *waitRelaunchHarness) {
				h.reed.OnEndStrandProcess = func() {
					if err := h.fx.Runner.StopStrand("strand-1"); err != nil {
						h.t.Errorf("StopStrand() error: %v", err)
					}
				}
			},
			check: func(t *testing.T, h *waitRelaunchHarness) {
				if h.err != nil || h.result.Outcome != OutcomeDied || h.result.StrandGUID != "strand-1" || h.result.SessionID != "session-1" {
					t.Errorf("Wait() = %+v, error %v; want the run's identity with outcome died", h.result, h.err)
				}
				if h.record.Outcome != runOutcomeStopped || h.reed.stranded() || len(h.reed.RelaunchStrandCalls) != 0 {
					t.Errorf("record %q, strand left up %v, relaunches %d; want stopped, removed, none relaunched", h.record.Outcome, h.reed.stranded(), len(h.reed.RelaunchStrandCalls))
				}
				if len(h.ends) != 1 || h.ends[0].Outcome != Outcome(runOutcomeStopped) {
					t.Errorf("session ends = %+v; want the stop's alone", h.ends)
				}
			},
		},
		{
			name: "ARunStopBetweenTheMarkAndTheRecordWriteEndsTheRunFinalizedDied",
			setup: func(h *waitRelaunchHarness) {
				h.reed.OnEndStrandProcess = func() {
					if err := h.run.Stop(); err != nil {
						h.t.Errorf("Run.Stop() error: %v", err)
					}
				}
			},
			check: func(t *testing.T, h *waitRelaunchHarness) {
				if h.err != nil || h.result.Outcome != OutcomeDied || h.result.EndedAt.IsZero() {
					t.Errorf("Wait() = %+v, error %v; want a finalized result with outcome died", h.result, h.err)
				}
				if h.record.Outcome != runOutcomeStopped || h.reed.stranded() || len(h.reed.RelaunchStrandCalls) != 0 {
					t.Errorf("record %q, strand left up %v, relaunches %d; want stopped, removed, none relaunched", h.record.Outcome, h.reed.stranded(), len(h.reed.RelaunchStrandCalls))
				}
				if len(h.ends) != 1 || h.ends[0].Outcome != OutcomeDied {
					t.Errorf("session ends = %+v; want the run's own alone", h.ends)
				}
			},
		},
		{
			name: "ARecordAStopStrandMadeTerminalBeforeTheMarkEndsTheRelaunchWithNothingEnded", seedStopped: true,
			check: func(t *testing.T, h *waitRelaunchHarness) {
				if h.err != nil || h.result.Outcome != OutcomeDied || h.result.StrandGUID != "strand-1" {
					t.Errorf("Wait() = %+v, error %v; want the run's identity with outcome died", h.result, h.err)
				}
				if h.record.Outcome != runOutcomeStopped || h.reed.stranded() {
					t.Errorf("record %q, strand left up %v; want stopped and removed", h.record.Outcome, h.reed.stranded())
				}
				if callsOf(h.reed.CallLog, "EndStrandProcess")+callsOf(h.reed.CallLog, "RelaunchStrand") != 0 {
					t.Errorf("reed calls = %v; want no EndStrandProcess and no RelaunchStrand", h.reed.CallLog)
				}
			},
		},
		{
			name: "AnEndOfTheProcessThatFailsOverAStrandNoLongerLiveGoesOnWithTheRelaunch", finishes: true,
			setup: func(h *waitRelaunchHarness) {
				h.reed.EndStrandProcessErr = errors.New("pane already gone")
				h.reed.endLeavesDead = true
			},
			check: func(t *testing.T, h *waitRelaunchHarness) { h.checkRelaunched(t) },
		},
		{
			name: "AnEndOfTheProcessThatFailsReleasesTheHoldAndClearsTheMark", timeout: 2 * time.Minute,
			setup: func(h *waitRelaunchHarness) { h.reed.EndStrandProcessErr = errors.New("reed down") },
			check: func(t *testing.T, h *waitRelaunchHarness) {
				if h.err != nil || h.result.Outcome != OutcomeTimeout {
					t.Errorf("Wait() = outcome %q, error %v; want the run left as it was, timing out", h.result.Outcome, h.err)
				}
				if callsOf(h.reed.CallLog, "ReleaseStrandHold") != 1 || len(h.reed.RelaunchStrandCalls) != 0 || h.record.RelaunchingAt != "" {
					t.Errorf("reed calls = %v, mark %q; want the hold released once, no relaunch and no mark", h.reed.CallLog, h.record.RelaunchingAt)
				}
			},
		},
		{
			name: "ASwitchAnsweringFalseSendsNoReedOpAndLeavesTheRunAsItWas", switchAnswers: []bool{false}, timeout: 2 * time.Minute,
			check: func(t *testing.T, h *waitRelaunchHarness) {
				if h.err != nil || h.result.Outcome != OutcomeTimeout {
					t.Errorf("Wait() = outcome %q, error %v; want the run left as it was, timing out", h.result.Outcome, h.err)
				}
				if got := callsOf(h.reed.CallLog, "EndStrandProcess") + callsOf(h.reed.CallLog, "ReleaseStrandHold") + callsOf(h.reed.CallLog, "RelaunchStrand"); got != 0 {
					t.Errorf("reed calls = %v; want no hold op", h.reed.CallLog)
				}
				if len(h.engine.switchRunDirs) != 1 || h.engine.switchRunDirs[0] != h.runDir || h.engine.switchTurnEnds[0] != "STOP:limit" {
					t.Errorf("switch asked with %q and %q; want once, with the run directory and the turn end", h.engine.switchRunDirs, h.engine.switchTurnEnds)
				}
			},
		},
		{
			name: "AFailedSwitchSendsNoReedOpAndLeavesTheRunAsItWas", timeout: 2 * time.Minute,
			setup: func(h *waitRelaunchHarness) { h.engine.switchErr = errors.New("account file unreadable") },
			check: func(t *testing.T, h *waitRelaunchHarness) {
				if h.err != nil || h.result.Outcome != OutcomeTimeout {
					t.Errorf("Wait() = outcome %q, error %v; want the run left as it was, timing out", h.result.Outcome, h.err)
				}
				if got := callsOf(h.reed.CallLog, "EndStrandProcess") + callsOf(h.reed.CallLog, "ReleaseStrandHold") + callsOf(h.reed.CallLog, "RelaunchStrand"); got != 0 {
					t.Errorf("reed calls = %v; want no hold op", h.reed.CallLog)
				}
				if len(h.engine.switchRunDirs) != 1 {
					t.Errorf("switch asked %d times; want once", len(h.engine.switchRunDirs))
				}
			},
		},
		{
			name: "ARunWithAPIErrorRetriesRecordsNoReSendForTheLimitedTurnEnd", retries: 2, finishes: true,
			check: func(t *testing.T, h *waitRelaunchHarness) {
				h.checkRelaunched(t)
				if slices.ContainsFunc(h.reed.SendTextCalls, func(call struct {
					GUID   string
					Text   string
					Submit bool
				}) bool {
					return call.Text == apiErrorRetryLine
				}) {
					t.Errorf("texts typed = %v; want no API-error re-send", h.reed.SendTextCalls)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newWaitRelaunchHarness(t, tt)
			h.wait()
			tt.check(t, h)
		})
	}
}

// startRelaunchRow is one case of TestStart_UsageLimitedLoadTurnRepeatsTheLaunch.
// It drives a start over a limited load turn.
type startRelaunchRow struct {
	name string
	// switchAnswers is the account switch's answers, true when nil.
	switchAnswers []bool
	// prepareFailAt is the Prepare call that fails.
	prepareFailAt int
	// switchErr is what the account switch fails with.
	switchErr error
	// checkErr is what CheckResume fails with once checkErrAfter calls have succeeded.
	checkErr      error
	checkErrAfter int
	// outputsWritten makes the account switch write the run's output file, as the session would have meanwhile.
	outputsWritten bool
	// stopsTheStrand has EndStrandProcess run StopStrand, and drives the start seam with start retries set.
	stopsTheStrand bool
	// resumes makes the start resume a session whose transcript the engine copies.
	// turnEnd is the line that ends the load turn.
	resumes bool
	turnEnd string
	check   func(t *testing.T, h *startRelaunchHarness)
}

// startRelaunchHarness is a start over a skill-load turn that ends on the usage limit.
type startRelaunchHarness struct {
	t       *testing.T
	fx      *fixture
	engine  *switchFakeEngine
	reed    *relaunchReed
	output  string
	runDir  string
	run     *Run
	result  Result
	err     error
	ends    []SessionEnd
	started bool

	// atRelaunch is what the record held when reed was asked to relaunch the strand.
	atRelaunch relaunchSnapshot
}

// startRelaunch runs row's start.
func startRelaunch(t *testing.T, row startRelaunchRow) *startRelaunchHarness {
	t.Helper()
	h := &startRelaunchHarness{t: t}
	h.engine = newSwitchFakeEngine(Launch{Cmd: "cmd", SessionID: "session-1", PromptLine: "do the task"})
	if row.switchAnswers != nil {
		h.engine.switchAnswers = row.switchAnswers
	}
	h.engine.prepareFailAt = row.prepareFailAt
	h.engine.switchErr, h.engine.checkErr, h.engine.checkErrAfter = row.switchErr, row.checkErr, row.checkErrAfter
	h.engine.originalLaunches = 1
	h.engine.PrepareHook = func(runDir string) {
		f, err := os.OpenFile(filepath.Join(runDir, eventsFileName), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			t.Errorf("open the events file: %v", err)
			return
		}
		defer f.Close()
		if _, err := f.WriteString("SESSIONSTART:startup\n"); err != nil {
			t.Errorf("seed the events file: %v", err)
		}
	}
	base := &fakeReed{AddStrandResult: reedengine.Strand{GUID: "strand-1"}}
	skill := &skillReed{fakeReed: base, Hangs: map[int]bool{}, Dies: map[int]bool{}, TurnEnds: map[int]string{0: "STOP:limit"}, OpenTurns: true, Screen: "the ready screen"}
	h.reed = newRelaunchReed(skill)
	cfg := fastConfig
	if row.stopsTheStrand {
		cfg.StartRetries = 1
	}
	h.fx = newFixture(t, h.reed, h.engine, withConfig(cfg), withClock(newFakeClock(time.Now())))
	skill.EventsPath = func() string { return filepath.Join(soleStartupRunDir(t, cfg, h.fx.Anchor), eventsFileName) }
	h.reed.beforeRelaunch = recordAtRelaunch(func() string { return soleStartupRunDir(t, cfg, h.fx.Anchor) }, &h.atRelaunch)
	h.fx.Runner.SetSessionEnd(func(end SessionEnd) { h.ends = append(h.ends, end) })
	h.output = filepath.Join(t.TempDir(), "out.md")
	if row.outputsWritten {
		h.engine.onSwitch = func() {
			if err := os.WriteFile(h.output, []byte("done"), 0o644); err != nil {
				t.Errorf("write the output file: %v", err)
			}
		}
	}
	if row.stopsTheStrand {
		base.OnEndStrandProcess = func() {
			if err := h.fx.Runner.StopStrand("strand-1"); err != nil {
				t.Errorf("StopStrand() error: %v", err)
			}
		}
	}
	spec := Spec{Prompt: "x", OutputFiles: []string{h.output}, Skills: []string{"a"}, Timeout: time.Hour}
	if row.resumes {
		spec.ResumeSessionID = "11111111-2222-3333-4444-555555555555"
		h.engine.carried = true
	}
	if row.turnEnd != "" {
		skill.TurnEnds[0] = row.turnEnd
	}
	if row.stopsTheStrand {
		h.run, h.result, h.err = h.fx.Runner.start(spec, GateSpec{})
	} else {
		h.run, h.err = h.fx.Runner.Start(spec)
	}
	h.runDir = soleStartupRunDir(t, cfg, h.fx.Anchor)
	return h
}

// TestStart_UsageLimitedLoadTurnRepeatsTheLaunch pins the launch repeat of a start.
// The start's skill-load turn ended on a usage limit.
func TestStart_UsageLimitedLoadTurnRepeatsTheLaunch(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []startRelaunchRow{
		{
			name: "ASwitchRepeatsTheLaunchUnderTheNewAccountAndTypesThePromptOnce",
			check: func(t *testing.T, h *startRelaunchHarness) {
				if h.err != nil || h.run == nil {
					t.Fatalf("Start() = run %v, error %v; want the relaunched run", h.run, h.err)
				}
				checkRecordedBeforeRelaunch(t, h.atRelaunch, "session-fresh", "do the task")
				if len(h.engine.switchRunDirs) != 1 || h.engine.switchRunDirs[0] != h.runDir || h.engine.switchTurnEnds[0] != "STOP:limit" {
					t.Errorf("switch asked with %q and %q; want once, with the run directory and the limited turn end", h.engine.switchRunDirs, h.engine.switchTurnEnds)
				}
				if len(h.engine.prepared) != 2 || !reflect.DeepEqual(h.engine.prepared[0], h.engine.prepared[1]) {
					t.Errorf("Prepare specs = %+v; want two equal ones", h.engine.prepared)
				}
				record, _, err := loadRunState(h.runDir)
				if err != nil || record.PromptLine != "do the task" || record.RelaunchingAt != "" {
					t.Errorf("record = %+v, error %v; want the deferred prompt line and no relaunching mark", record, err)
				}
				if got := strings.Join(typedTexts(h.reed.skillReed), "|"); got != "do the task" {
					t.Errorf("typed = %q; want the prompt once and no load typed before the relaunch", got)
				}
				if len(h.reed.AddStrandCalls) != 1 || len(h.reed.RelaunchStrandCalls) != 1 || h.reed.RelaunchStrandCalls[0].Spec.Cmd != "LOAD:a" {
					t.Errorf("AddStrand calls %d, relaunches %+v; want one launch and one relaunch carrying the load", len(h.reed.AddStrandCalls), h.reed.RelaunchStrandCalls)
				}
			},
		},
		{
			name: "ASwitchAnsweringFalseLeavesTheLoadHoldToRetypeTheLoad", switchAnswers: []bool{false},
			check: func(t *testing.T, h *startRelaunchHarness) {
				if h.err != nil || h.run == nil {
					t.Fatalf("Start() = run %v, error %v; want the run", h.run, h.err)
				}
				if got := strings.Join(typedTexts(h.reed.skillReed), "|"); got != "LOAD:a|do the task" {
					t.Errorf("typed = %q; want the load typed again, then the prompt", got)
				}
				if len(h.engine.switchRunDirs) != 1 || callsOf(h.reed.CallLog, "EndStrandProcess") != 0 {
					t.Errorf("switch asked %d times, reed calls %v; want one ask and no hold op", len(h.engine.switchRunDirs), h.reed.CallLog)
				}
			},
		},
		{
			name: "AFailedSwitchLeavesTheLoadHoldToRetypeTheLoad", switchErr: errors.New("account file unreadable"),
			check: func(t *testing.T, h *startRelaunchHarness) {
				if h.err != nil || h.run == nil {
					t.Fatalf("Start() = run %v, error %v; want the run", h.run, h.err)
				}
				if got := strings.Join(typedTexts(h.reed.skillReed), "|"); got != "LOAD:a|do the task" {
					t.Errorf("typed = %q; want the load typed again, then the prompt", got)
				}
				if len(h.engine.switchRunDirs) != 1 || callsOf(h.reed.CallLog, "EndStrandProcess") != 0 {
					t.Errorf("switch asked %d times, reed calls %v; want one ask and no hold op", len(h.engine.switchRunDirs), h.reed.CallLog)
				}
			},
		},
		{
			name: "AResumedLaunchIsRepeatedWithTheSameSessionAndItsCarriedTranscript", resumes: true,
			check: func(t *testing.T, h *startRelaunchHarness) {
				if h.err != nil || h.run == nil {
					t.Fatalf("Start() = run %v, error %v; want the relaunched run", h.run, h.err)
				}
				const resumed = "11111111-2222-3333-4444-555555555555"
				if len(h.engine.prepared) != 2 || h.engine.prepared[1].ResumeSessionID != resumed {
					t.Errorf("Prepare specs = %+v; want a second one resuming %s", h.engine.prepared, resumed)
				}
				record, _, err := loadRunState(h.runDir)
				if err != nil || record.SessionID != resumed || !record.TranscriptCopied {
					t.Errorf("record = %+v, error %v; want session %s with its transcript copied", record, err, resumed)
				}
			},
		},
		{
			name: "ARepeatWhoseResumedTranscriptIsInNoDirectoryStartsFresh", resumes: true, checkErr: fmt.Errorf("check: %w", ErrNoTranscript), checkErrAfter: 1,
			check: func(t *testing.T, h *startRelaunchHarness) {
				if h.err != nil || h.run == nil {
					t.Fatalf("Start() = run %v, error %v; want the relaunched run", h.run, h.err)
				}
				if len(h.engine.prepared) != 2 || h.engine.prepared[1].ResumeSessionID != "" {
					t.Errorf("Prepare specs = %+v; want a second, fresh one", h.engine.prepared)
				}
				record, _, err := loadRunState(h.runDir)
				if err != nil || record.SessionID != "session-fresh" || record.TranscriptCopied {
					t.Errorf("record = %+v, error %v; want session-fresh and no copy", record, err)
				}
			},
		},
		{
			name: "ASecondPrepareThatFailsEndsTheStartNotStartedAndFinalizesTheRunOnce", prepareFailAt: 2,
			check: func(t *testing.T, h *startRelaunchHarness) {
				if h.run != nil || !errors.Is(h.err, ErrNotStarted) {
					t.Fatalf("Start() = run %v, error %v; want no run and an error wrapping ErrNotStarted", h.run, h.err)
				}
				record, _, err := loadRunState(h.runDir)
				if err != nil || record.Outcome != string(OutcomeDied) || h.reed.stranded() || len(h.ends) != 1 {
					t.Errorf("record %q, error %v, strand left up %v, session ends %d; want died, removed, one end", record.Outcome, err, h.reed.stranded(), len(h.ends))
				}
			},
		},
		{
			name: "ASecondPrepareThatFailsWithEveryOutputFilePresentReturnsTheRunWhichEndsDoneOnce", prepareFailAt: 2, outputsWritten: true,
			check: func(t *testing.T, h *startRelaunchHarness) {
				if h.err != nil || h.run == nil {
					t.Fatalf("Start() = run %v, error %v; want the run handed back", h.run, h.err)
				}
				result, err := h.run.Wait()
				if err != nil || result.Outcome != OutcomeDone || len(h.ends) != 1 {
					t.Errorf("Wait() = outcome %q, error %v, session ends %d; want done with one end", result.Outcome, err, len(h.ends))
				}
			},
		},
		{
			name: "AStopStrandDuringTheRelaunchReturnsNoRunAndLaunchesNothingAgain", stopsTheStrand: true,
			check: func(t *testing.T, h *startRelaunchHarness) {
				if h.run != nil || !errors.Is(h.err, ErrNotStarted) || h.result.Outcome != OutcomeDied || h.result.StrandGUID != "strand-1" {
					t.Fatalf("start() = run %v, result %+v, error %v; want no run, the run's identity with outcome died and ErrNotStarted", h.run, h.result, h.err)
				}
				record, _, err := loadRunState(h.runDir)
				if err != nil || record.Outcome != runOutcomeStopped || h.reed.stranded() {
					t.Errorf("record %q, error %v, strand left up %v; want stopped and removed", record.Outcome, err, h.reed.stranded())
				}
				if len(h.ends) != 1 || h.ends[0].Outcome != Outcome(runOutcomeStopped) || len(h.reed.AddStrandCalls) != 1 {
					t.Errorf("session ends %+v, launches %d; want the stop's one end and no second launch", h.ends, len(h.reed.AddStrandCalls))
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tt.check(t, startRelaunch(t, tt))
		})
	}
}

// TestRun_StopForRelaunch_ReturnsAFailedHoldReleaseBesideTheEndFailure pins what a relaunch stop reports.
// Its process end and its hold release both fail over a live strand.
func TestRun_StopForRelaunch_ReturnsAFailedHoldReleaseBesideTheEndFailure(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	h := newWaitRelaunchHarness(t, waitRelaunchRow{setup: func(h *waitRelaunchHarness) {
		h.reed.EndStrandProcessErr = errors.New("reed down")
		h.reed.ReleaseStrandHoldErr = errors.New("release refused")
	}})

	stopped, err := h.run.stopForRelaunch()

	if stopped || err == nil || !strings.Contains(err.Error(), "reed down") || !strings.Contains(err.Error(), "release refused") {
		t.Errorf("stopForRelaunch() = %v, %v; want not stopped and an error naming the end failure and the release failure", stopped, err)
	}
	if h.run.state.RelaunchingAt != "" {
		t.Errorf("relaunching mark = %q; want it cleared", h.run.state.RelaunchingAt)
	}
}

// TestRun_SwitchAccountAtLimit_ADeclinedRelaunchHandlesTheLimitAsWithoutASwitch pins a relaunch that never began.
// Its relaunching mark cannot be written, so nothing is ended and the handled offset is restored.
func TestRun_SwitchAccountAtLimit_ADeclinedRelaunchHandlesTheLimitAsWithoutASwitch(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	h := newWaitRelaunchHarness(t, waitRelaunchRow{})
	const priorOffset = 5
	h.run.apiErrors.handledOffset = priorOffset
	permkit.Chmod(t, h.runDir, 0o555)
	probe := filepath.Join(h.runDir, "probe")
	if err := os.WriteFile(probe, nil, 0o644); err == nil {
		_ = os.Remove(probe)
		t.Skip("the run directory stays writable under this user, so the record write cannot be made to fail")
	}

	_, finished, handled, err := h.run.switchAccountAtLimit(&heldTurnEnd{raw: []byte("STOP:limit"), offset: 17})

	if err != nil || finished || handled {
		t.Errorf("switchAccountAtLimit() = finished %v, handled %v, error %v; want none of them", finished, handled, err)
	}
	if h.run.apiErrors.handledOffset != priorOffset {
		t.Errorf("handledOffset = %d; want it restored to %d", h.run.apiErrors.handledOffset, priorOffset)
	}
	if len(h.engine.switchRunDirs) != 1 || callsOf(h.reed.CallLog, "EndStrandProcess") != 0 {
		t.Errorf("switch asked %d times, reed calls %v; want one ask and nothing ended", len(h.engine.switchRunDirs), h.reed.CallLog)
	}
}
