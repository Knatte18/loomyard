package orchengine

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/stencilstore"
	"github.com/Knatte18/loomyard/internal/testkit/logcapture"
)

type fakeClock struct{ now time.Time }

func (c *fakeClock) Now() time.Time          { return c.now }
func (c *fakeClock) advance(d time.Duration) { c.now = c.now.Add(d) }

// fakeSession scripts the Session seam; event offsets are indexes into events.
type fakeSession struct {
	alive    bool
	events   []shuttleengine.Event
	usage    map[string]int // Turn-end message to tokens; a missing message is unknown.
	idle     bool           // Answered by SessionIdle, which only the hub pause reads.
	idleSeq  []bool         // Consumed before idle.
	color    string         // Answered by StrandColor.
	colorErr error          // Returned by every StrandColor while set.
	// colors counts the color command injects the cycle sent, kept out of calls so a test of any other behavior ignores them.
	colors int
	// calls lists the steps the fake creel accepted in order, "send:<text>" for a post or a text inject, "clear", "reload-plugins", "skills:<list>" and "compact:<focus>", then every interrupt.
	calls        []string
	tokenAsks    []string
	onSend       func()
	onAlive      func()
	onReadEvents func()

	skillLoads  map[string]shuttleengine.SkillLoadReport    // Turn-end message to the report ClassifySkillLoad answers, else a report with every skill loaded.
	autoCompact map[string]shuttleengine.CompactionBoundary // Turn-end message to a main-chain compaction boundary CompactedSince finds through it.

	sessionState    shuttleengine.SessionState // Answered by SessionState while stateErr is nil.
	sessionStateErr error                      // Returned by every SessionState while set.
	sessionStartAt  time.Time                  // Answered by SessionState as the newest session-start time.
	sessionSinces   []time.Time                // Consumed one per SessionState call, which the watcher's per-tick session id read makes one more per tick, into the answered state's Since, the last one sticking, while it is not empty.
	sessionID       string                     // Answered by SessionState as the reading's session id.

	// interruptErrs is consumed in order by Interrupt, which answers nil once it is drained.
	interruptErrs []error
	// settle is the InterruptSettle answer.
	settle time.Duration
	// settleErr is returned by every InterruptSettle while set.
	settleErr error
	// strandNameErr is returned by every StrandName while set.
	strandNameErr error
}

func (f *fakeSession) SessionState(string) (shuttleengine.RunSessionState, error) {
	if f.sessionStateErr != nil {
		return shuttleengine.RunSessionState{}, f.sessionStateErr
	}
	state := f.sessionState
	if len(f.sessionSinces) > 0 {
		state.Since = f.sessionSinces[0]
		if len(f.sessionSinces) > 1 {
			f.sessionSinces = f.sessionSinces[1:]
		}
	}
	return shuttleengine.RunSessionState{State: state, SessionStartAt: f.sessionStartAt, SessionID: f.sessionID}, nil
}

// fakeOrchStrandName is the name the fake session answers for every strand.
const fakeOrchStrandName = "tst:orch"

func (f *fakeSession) StrandName(string) (string, error) {
	if f.strandNameErr != nil {
		return "", f.strandNameErr
	}
	return fakeOrchStrandName, nil
}

func (f *fakeSession) StrandColor(string) (string, error) {
	return f.color, f.colorErr
}

func (f *fakeSession) ClassifySkillLoad(guid string, turnEnd shuttleengine.Event, skills []string) (shuttleengine.SkillLoadReport, error) {
	if report, ok := f.skillLoads[turnEnd.Message]; ok {
		return report, nil
	}
	return shuttleengine.SkillLoadReport{Verified: true, Loaded: skills}, nil
}

func (f *fakeSession) CompactedSince(guid string, turnEnd shuttleengine.Event, since time.Time) (shuttleengine.CompactionBoundary, bool, error) {
	b, ok := f.autoCompact[turnEnd.Message]
	return b, ok && b.At.After(since), nil
}

func (f *fakeSession) StrandAlive(string) (bool, error) {
	if f.onAlive != nil {
		f.onAlive()
	}
	return f.alive, nil
}

func (f *fakeSession) ReadEvents(_ string, offset int64) ([]shuttleengine.Event, int64, error) {
	if f.onReadEvents != nil {
		f.onReadEvents()
	}
	if offset > int64(len(f.events)) {
		offset = int64(len(f.events))
	}
	return append([]shuttleengine.Event(nil), f.events[offset:]...), int64(len(f.events)), nil
}

func (f *fakeSession) ContextTokens(guid string, ev shuttleengine.Event) (shuttleengine.ContextReading, error) {
	f.tokenAsks = append(f.tokenAsks, ev.Message)
	n, ok := f.usage[ev.Message]
	return shuttleengine.ContextReading{Tokens: n, Known: ok}, nil
}

func (f *fakeSession) SessionIdle(string) (shuttleengine.IdleProbe, error) {
	idle := f.idle
	if len(f.idleSeq) > 0 {
		idle = f.idleSeq[0]
		f.idleSeq = f.idleSeq[1:]
	}
	return shuttleengine.IdleProbe{Idle: idle}, nil
}

func (f *fakeSession) Interrupt(string) error {
	f.calls = append(f.calls, interruptCall)
	if len(f.interruptErrs) == 0 {
		return nil
	}
	err := f.interruptErrs[0]
	f.interruptErrs = f.interruptErrs[1:]
	return err
}

func (f *fakeSession) InterruptSettle(guid string) (time.Duration, error) {
	if f.settleErr != nil {
		return 0, f.settleErr
	}
	return f.settle, nil
}

func (f *fakeSession) count(prefix string) int {
	n := 0
	for _, c := range f.calls {
		if strings.HasPrefix(c, prefix) {
			n++
		}
	}
	return n
}

var errBoom = errors.New("boom")

// reloadPluginsCall is the call the fake records for the plugins step.
const reloadPluginsCall = "reload-plugins"

// interruptCall is the call the fake records for an interrupt.
const interruptCall = "interrupt"

// callOf spells an inject the way fakeSession.calls records it.
func callOf(step CreelStep) string {
	switch step.Command {
	case "":
		return "send:" + step.Text
	case commandCompact:
		return "compact:" + step.Argument
	case commandLoadSkills:
		return "skills:" + strings.ReplaceAll(step.Argument, " ", ",")
	}
	return step.Command
}

type watchEnv struct {
	t     *testing.T
	w     *Watcher
	s     *fakeSession
	c     *fakeCreel
	clock *fakeClock
	paths Paths
	stDir string
	cfg   Config
	// pausePath is the pause record the watcher is told.
	pausePath string

	skills []string // The skill list the watcher reloads, set before newWatcher.

	// indexErr, when set, makes the watcher's index source fail; otherwise it serves testIndex.
	indexErr error
	// indexCalls counts the watcher's calls to its index source.
	indexCalls int
}

func stop(msg string) shuttleengine.Event {
	return shuttleengine.Event{Kind: shuttleengine.EventStop, Message: msg}
}

func ask(msg string) shuttleengine.Event {
	return shuttleengine.Event{Kind: shuttleengine.EventAsk, Message: msg}
}

// newWatchEnv builds an idle orch state on strand s1 with threshold 1000, grace 10s, timeout 100s.
// The fake creel lands every inject at once, sent at the clock's time, until a test scripts an outcome.
func newWatchEnv(t *testing.T) *watchEnv {
	t.Helper()
	e := &watchEnv{
		t:     t,
		paths: testPaths(t),
		stDir: seedStencils(t),
		s:     &fakeSession{alive: true, idle: true, usage: map[string]int{}, color: "colour208"},
		c:     &fakeCreel{autoLand: true},
		cfg:   Config{ThresholdTokens: 1000, IdleGraceS: 10, HandoffTimeoutS: 100, CycleMode: CycleClear},
		clock: &fakeClock{now: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)},
	}
	e.c.clock = func() time.Time { return e.clock.now }
	e.c.onSend = func(mail bool, step CreelStep, body string) {
		switch {
		case mail:
			e.s.calls = append(e.s.calls, "send:"+body)
		case step.Command == commandColor:
			e.s.colors++
		default:
			e.s.calls = append(e.s.calls, callOf(step))
		}
		if e.s.onSend != nil {
			e.s.onSend()
		}
	}
	e.pausePath = filepath.Join(t.TempDir(), "pause", pauserecord.RecordFileName)
	if err := SaveState(e.paths, State{Strand: "s1", Phase: PhaseIdle}); err != nil {
		t.Fatal(err)
	}
	e.w = e.newWatcher()
	return e
}

func (e *watchEnv) newWatcher() *Watcher {
	index := func() (string, error) {
		e.indexCalls++
		if e.indexErr != nil {
			return "", e.indexErr
		}
		return testIndex, nil
	}
	w := NewWatcher(e.s, e.c, e.cfg, e.paths, e.stDir, e.skills, index, e.clock)
	w.WatchPause(PauseWatch{RecordPath: e.pausePath, Alive: func(int, string) bool { return true }})
	return w
}

func (e *watchEnv) tick() {
	e.t.Helper()
	done, err := e.w.Tick()
	if err != nil || done {
		e.t.Fatalf("Tick = %v, %v", done, err)
	}
}

func (e *watchEnv) tickErr() error {
	e.t.Helper()
	_, err := e.w.Tick()
	if err == nil {
		e.t.Fatal("Tick: expected an error")
	}
	return err
}

func (e *watchEnv) state() State {
	e.t.Helper()
	st, err := LoadState(e.paths)
	if err != nil {
		e.t.Fatal(err)
	}
	return st
}

func (e *watchEnv) setState(mut func(*State)) {
	e.t.Helper()
	st := e.state()
	mut(&st)
	if err := SaveState(e.paths, st); err != nil {
		e.t.Fatal(err)
	}
}

// lastInjectID returns the id of the newest inject the fake creel accepted.
func (e *watchEnv) lastInjectID() int64 {
	e.t.Helper()
	if len(e.c.injects) == 0 {
		e.t.Fatal("no inject was accepted")
	}
	return e.c.injects[len(e.c.injects)-1].ID
}

// setOutcome scripts where inject id stands, ahead of the fake's own landing.
func (e *watchEnv) setOutcome(id int64, outcome CreelOutcome) {
	if e.c.outcomes == nil {
		e.c.outcomes = map[int64]CreelOutcome{}
	}
	e.c.outcomes[id] = outcome
}

// reply leaves the orch's reply to the pending note request in the watcher's mailbox.
func (e *watchEnv) reply(body string) {
	e.t.Helper()
	e.c.mailbox = append(e.c.mailbox, CreelMessage{ID: int64(100 + len(e.c.mailbox)), From: fakeOrchStrandName, ReplyTo: e.state().RequestID, Body: body})
}

// forgetLastSend rewinds the state to the instant after the newest post or inject was persisted and before creel answered it:
// the step is pending with no id.
// It returns the journal entry creel holds for that request.
func (e *watchEnv) forgetLastSend() CreelSent {
	e.t.Helper()
	var (
		lastID int64
		mail   bool
		step   CreelStep
		body   string
	)
	for _, post := range e.c.posts {
		if post.ID > lastID {
			lastID, mail, body = post.ID, true, post.Body
		}
	}
	for _, inject := range e.c.injects {
		if inject.ID > lastID {
			lastID, mail, step = inject.ID, false, inject.Step
		}
	}
	st := e.state()
	pending := PendingStep{Mail: mail, Command: step.Command, Argument: step.Argument, Text: step.Text, PersistedAt: e.clock.now}
	entry := CreelSent{ID: 900, At: e.clock.now, Mail: mail, Step: step, Subject: step.Text}
	if mail {
		pending.Text = body
		st.RequestID = 0
		entry.Subject, _, _ = strings.Cut(body, "\n")
	}
	st.Pending, st.PhaseInjected = &pending, false
	if err := SaveState(e.paths, st); err != nil {
		e.t.Fatal(err)
	}
	return entry
}

// injectHandoff makes one over-threshold turn end and ticks until the note request is posted.
func (e *watchEnv) injectHandoff() {
	e.t.Helper()
	e.s.usage["a"] = 2000
	e.s.events = append(e.s.events, stop("a"))
	e.tick()
	e.clock.advance(11 * time.Second)
	e.tick()
	if st := e.state(); st.Phase != PhaseHandoffRequested {
		e.t.Fatalf("phase after inject = %s", st.Phase)
	}
}

func (e *watchEnv) writeHandoff() string {
	e.t.Helper()
	path := e.state().PendingHandoff
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		e.t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("handoff"), 0o644); err != nil {
		e.t.Fatal(err)
	}
	return path
}

// reachClearing completes the note so the state is clearing with the clear sent.
func (e *watchEnv) reachClearing() {
	e.t.Helper()
	e.injectHandoff()
	e.finishNote()
	if st := e.state(); st.Phase != PhaseClearing {
		e.t.Fatalf("phase = %s, want clearing", st.Phase)
	}
}

// reachResuming continues from reachClearing until the resume pointer is sent: the color on one tick, the plugins reload on the next, the pointer on the one after.
func (e *watchEnv) reachResuming() {
	e.t.Helper()
	e.reachClearing()
	e.tick()
	e.tick()
	if st := e.state(); st.Phase != PhaseResuming {
		e.t.Fatalf("phase = %s, want resuming", st.Phase)
	}
	e.tick()
}

// reachReloadStep drives a clear cycle until the reload's step is queued and its inject typed.
// The skills and retry steps need the watcher told its skills first.
func (e *watchEnv) reachReloadStep(step int) {
	e.t.Helper()
	e.reachClearing()
	e.tick() // the clear typed: resuming, the color queued
	if step == ReloadStepColor {
		return
	}
	e.tick() // the color typed: the plugins reload queued
	if step == ReloadStepPlugins {
		return
	}
	e.tick() // the plugins reload typed: the skills load queued, or the pointer with no skills
	if step == ReloadStepSkills || len(e.skills) == 0 {
		return
	}
	if step == ReloadStepRetry {
		e.s.skillLoads = map[string]shuttleengine.SkillLoadReport{"t1": {Verified: true, Missing: []string{"ly:board"}}}
	}
	e.endTurn("t1")
}

func (e *watchEnv) assertNoCalls() {
	e.t.Helper()
	if len(e.s.calls) != 0 {
		e.t.Fatalf("unexpected calls: %v", e.s.calls)
	}
}

func TestWatcher_BelowThresholdNoInjection(t *testing.T) {
	e := newWatchEnv(t)
	e.s.usage["a"] = 500
	e.s.events = []shuttleengine.Event{stop("a")}
	e.tick()
	e.clock.advance(11 * time.Second)
	e.tick()
	e.assertNoCalls()
	if st := e.state(); st.LastContextTokens != 500 || !st.LastContextKnown {
		t.Errorf("reading = %d known=%v", st.LastContextTokens, st.LastContextKnown)
	}
}

func TestWatcher_AboveThresholdInjectsHandoffPersistedFirst(t *testing.T) {
	e := newWatchEnv(t)
	var stateAtSend State
	e.s.onSend = func() { stateAtSend = e.state() }
	e.injectHandoff()
	if stateAtSend.Phase != PhaseHandoffRequested || stateAtSend.Pending == nil || !stateAtSend.Pending.Mail || stateAtSend.Pending.ID != 0 {
		t.Errorf("state at send = %+v, want handoff-requested with the request pending and no id", stateAtSend)
	}
	st := e.state()
	if len(e.s.calls) != 1 || !strings.Contains(e.s.calls[0], st.PendingHandoff) {
		t.Fatalf("calls = %v, pending %q", e.s.calls, st.PendingHandoff)
	}
	if !st.PhaseInjected || st.PhaseStrand != "s1" || st.PhaseEventsOffset != 1 || st.Pending != nil || st.RequestID != e.c.posts[0].ID {
		t.Errorf("state = %+v, want the request's id saved and nothing pending", st)
	}
	if e.c.posts[0].Receiver != fakeOrchStrandName {
		t.Errorf("request posted to %q, want %q", e.c.posts[0].Receiver, fakeOrchStrandName)
	}
}

func TestWatcher_AskEventBlocksInjection(t *testing.T) {
	e := newWatchEnv(t)
	e.s.usage["a"] = 2000
	e.s.events = []shuttleengine.Event{stop("a"), ask("q")}
	e.tick()
	e.clock.advance(11 * time.Second)
	e.tick()
	e.assertNoCalls()
}

func TestWatcher_NewerEventRestartsIdleGrace(t *testing.T) {
	e := newWatchEnv(t)
	e.s.usage["a"] = 2000
	e.s.events = []shuttleengine.Event{stop("a")}
	e.tick()
	e.clock.advance(8 * time.Second)
	e.s.events = append(e.s.events, stop("a"))
	e.tick()
	e.clock.advance(5 * time.Second)
	e.tick()
	e.assertNoCalls()
	e.clock.advance(6 * time.Second)
	e.tick()
	if e.s.count("send:") != 1 {
		t.Fatalf("calls = %v", e.s.calls)
	}
}

func TestWatcher_UnknownUsageNeverTriggers(t *testing.T) {
	e := newWatchEnv(t)
	e.s.events = []shuttleengine.Event{stop("unreadable")}
	e.tick()
	e.clock.advance(11 * time.Second)
	e.tick()
	e.assertNoCalls()
	if st := e.state(); st.LastContextKnown {
		t.Errorf("reading should be unknown: %+v", st)
	}
}

func TestWatcher_ManualRequestBelowThresholdCyclesWithoutGrace(t *testing.T) {
	e := newWatchEnv(t)
	e.s.usage["a"] = 10
	e.s.events = []shuttleengine.Event{stop("a")}
	if err := RequestCycle(e.paths, CycleClear, e.clock.now); err != nil {
		t.Fatal(err)
	}
	// The turn end is read on this same tick: a requested cycle waits no idle grace.
	e.tick()
	if e.state().Phase != PhaseHandoffRequested {
		t.Fatalf("phase = %s", e.state().Phase)
	}
	if _, requested, _ := CycleRequested(e.paths); requested {
		t.Error("cycle request should be cleared")
	}
}

func TestWatcher_HandoffTurnEndWithoutFileNoClear(t *testing.T) {
	e := newWatchEnv(t)
	e.injectHandoff()
	e.s.events = append(e.s.events, stop("handoff"))
	e.tick()
	if e.s.count("clear") != 0 || e.state().Phase != PhaseHandoffRequested {
		t.Fatalf("calls = %v phase = %s", e.s.calls, e.state().Phase)
	}
}

func TestWatcher_HandoffTimeoutAborts(t *testing.T) {
	e := newWatchEnv(t)
	e.injectHandoff()
	e.clock.advance(101 * time.Second)
	e.tick()
	st := e.state()
	if st.Phase != PhaseIdle || st.LastAbortReason == "" || e.s.count("clear") != 0 {
		t.Fatalf("state = %+v calls = %v", st, e.s.calls)
	}
}

func TestWatcher_AskDuringHandoffAborts(t *testing.T) {
	e := newWatchEnv(t)
	e.injectHandoff()
	e.s.events = append(e.s.events, ask("which?"))
	e.tick()
	st := e.state()
	if st.Phase != PhaseIdle || st.LastAbortReason == "" || e.s.count("clear") != 0 {
		t.Fatalf("state = %+v calls = %v", st, e.s.calls)
	}
}

func TestWatcher_HandoffCompleteClearsThenResumes(t *testing.T) {
	t.Parallel()

	e := newWatchEnv(t)
	e.reachClearing()
	handoff := e.state().PendingHandoff
	if e.s.count("clear") != 1 {
		t.Fatalf("calls = %v", e.s.calls)
	}
	if got := e.c.count("hold:"); got < 2 || e.c.count("release:") != 0 {
		t.Fatalf("holds = %d releases = %d, want the hold taken at the cycle start and renewed since", got, e.c.count("release:"))
	}
	holds := e.c.count("hold:")
	e.tick()
	st := e.state()
	if st.Phase != PhaseResuming || st.CycleCount != 1 || st.LastHandoff != handoff {
		t.Fatalf("state = %+v", st)
	}
	if e.s.colors != 2 || e.s.count(reloadPluginsCall) != 0 {
		t.Fatalf("colors = %d calls = %v, want the color queued and no plugins reload yet", e.s.colors, e.s.calls)
	}
	e.tick()
	if last := e.s.calls[len(e.s.calls)-1]; last != reloadPluginsCall {
		t.Fatalf("last call = %q, want the plugins reload after the color", last)
	}
	e.tick()
	last := e.s.calls[len(e.s.calls)-1]
	if !strings.HasPrefix(last, "send:") || !strings.Contains(last, handoff) {
		t.Errorf("resume prompt = %q, want one naming %s", last, handoff)
	}
	if got := e.c.count("hold:"); got != holds+3 || e.c.count("release:") != 0 {
		t.Errorf("holds = %d releases = %d, want the hold renewed on each of the three ticks and still kept", got, e.c.count("release:"))
	}
	e.endTurn("resumed")
	if st := e.state(); st.Phase != PhaseIdle {
		t.Fatalf("phase = %s, want idle", st.Phase)
	}
	if got := e.c.count("release:"); got != 1 {
		t.Errorf("releases = %d, want the hold released once when the pointer's turn end ended the phase", got)
	}
}

func TestWatcher_ConfirmedClearRemovesOnlyASatisfiedClearRequest(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		mode        string
		wantPending bool
	}{
		{"clear request made during the cycle is removed", CycleClear, false},
		{"compact request made during the cycle stays pending", CycleCompact, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			e := newWatchEnv(t)
			e.reachClearing()
			if err := RequestCycle(e.paths, c.mode, e.clock.Now()); err != nil {
				t.Fatal(err)
			}
			e.tick()
			if st := e.state(); st.Phase != PhaseResuming {
				t.Fatalf("phase = %s, want resuming", st.Phase)
			}
			if _, pending, _ := CycleRequested(e.paths); pending != c.wantPending {
				t.Errorf("request pending = %v, want %v", pending, c.wantPending)
			}
		})
	}
}

// TestWatcher_ClearingTimeoutNeverTypesWhileBusy pins that a clear creel has not typed is waited for however long that lasts.
// Its stall reason shows in State.Stuck, and it is queued again only after creel expired it.
func TestWatcher_ClearingTimeoutNeverTypesWhileBusy(t *testing.T) {
	t.Parallel()

	e := newWatchEnv(t)
	e.reachClearing()
	e.setOutcome(e.lastInjectID(), CreelOutcome{State: CreelQueued})
	e.tick()
	e.clock.advance(101 * time.Second)
	e.tick()
	e.tick()
	st := e.state()
	if st.Phase != PhaseClearing || e.s.count("clear") != 1 {
		t.Fatalf("phase = %s calls = %v, want one clear still waiting for the busy orch", st.Phase, e.s.calls)
	}

	e.setOutcome(e.lastInjectID(), CreelOutcome{State: CreelExpired, Stall: "the orch is busy"})
	e.tick()
	if st := e.state(); st.Phase != PhaseClearing || st.Stuck != "the orch is busy" || e.s.count("clear") != 2 {
		t.Fatalf("state = %+v calls = %v, want the clear queued again with the stall shown", st, e.s.calls)
	}
	e.tick()
	st = e.state()
	if st.Phase != PhaseResuming || st.CycleCount != 1 || st.Stuck != "" || e.s.count(reloadPluginsCall) != 0 {
		t.Fatalf("state = %+v calls = %v", st, e.s.calls)
	}
	e.tick()
	if e.s.count(reloadPluginsCall) != 1 {
		t.Fatalf("calls = %v", e.s.calls)
	}
	e.tick()
	if e.s.count("send:") != 2 {
		t.Fatalf("calls = %v", e.s.calls)
	}
}

func TestWatcher_ResumeTurnNeverEndingReturnsIdleUnknown(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// outcome scripts the pointer inject's outcome, none when it stays landed.
		outcome *CreelOutcome
	}{
		{"a landed pointer whose turn never ends", nil},
		{"an unconfirmed pointer keeps waiting for its turn end", &CreelOutcome{State: CreelUnconfirmed}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			e := newWatchEnv(t)
			e.reachResuming()
			if tt.outcome != nil {
				e.setOutcome(e.lastInjectID(), *tt.outcome)
			}
			e.clock.advance(50 * time.Second)
			e.tick()
			if st := e.state(); st.Phase != PhaseResuming {
				t.Fatalf("phase = %s, want resuming while the turn has not ended", st.Phase)
			}
			e.clock.advance(51 * time.Second)
			e.tick()
			st := e.state()
			if st.Phase != PhaseIdle || st.LastAbortReason != "resume timed out" || st.LastContextKnown {
				t.Fatalf("state = %+v", st)
			}
		})
	}
}

func TestWatcher_ResumeTurnEndTakesFirstPostCycleReading(t *testing.T) {
	e := newWatchEnv(t)
	e.reachResuming()
	e.s.usage["resumed"] = 300
	e.endTurn("resumed")
	st := e.state()
	if st.Phase != PhaseIdle || st.LastContextTokens != 300 || !st.LastContextKnown {
		t.Fatalf("state = %+v", st)
	}
	if st.LastInjectionOffset != int64(len(e.s.events)) {
		t.Errorf("LastInjectionOffset = %d, want %d", st.LastInjectionOffset, len(e.s.events))
	}
}

func TestWatcher_PreClearTurnEndNeverReadNorCyclesAgain(t *testing.T) {
	e := newWatchEnv(t)
	e.reachClearing()
	e.s.usage["pre"] = 5000
	// The stray turn end predates the clear's sending record.
	e.s.events = append(e.s.events, shuttleengine.Event{Kind: shuttleengine.EventStop, Message: "pre", At: e.clock.now.Add(-10 * time.Second)})
	e.tick() // clearing: the clear typed, moves to resuming past the stray turn end.
	if e.state().Phase != PhaseResuming {
		t.Fatalf("phase = %s", e.state().Phase)
	}
	e.clock.advance(101 * time.Second)
	e.tick() // the plugins step follows the color
	e.tick() // the pointer follows the plugins step
	sends := e.s.count("send:")
	for i := 0; i < 3; i++ {
		e.clock.advance(60 * time.Second)
		e.tick()
	}
	for _, m := range e.s.tokenAsks {
		if m == "pre" {
			t.Error("the pre-clear turn end must not yield a usage reading")
		}
	}
	if e.s.count("send:") != sends || e.state().Phase != PhaseIdle {
		t.Fatalf("second cycle started: calls = %v phase = %s", e.s.calls, e.state().Phase)
	}
	if st := e.state(); st.LastInjectionOffset != int64(len(e.s.events)) {
		t.Errorf("LastInjectionOffset = %d, want %d", st.LastInjectionOffset, len(e.s.events))
	}
}

func TestWatcher_AbortedHandoffNoRetryUntilLaterTurnEnd(t *testing.T) {
	e := newWatchEnv(t)
	e.injectHandoff()
	e.s.events = append(e.s.events, stop("handoff")) // no file was written
	e.tick()
	e.clock.advance(101 * time.Second)
	e.tick()
	if e.state().Phase != PhaseIdle {
		t.Fatalf("phase = %s", e.state().Phase)
	}
	sends := e.s.count("send:")
	for i := 0; i < 3; i++ {
		e.clock.advance(30 * time.Second)
		e.tick()
	}
	if e.s.count("send:") != sends {
		t.Fatalf("retried without a new turn end: %v", e.s.calls)
	}
	e.s.usage["b"] = 2000
	e.s.events = append(e.s.events, stop("b"))
	e.tick()
	e.clock.advance(11 * time.Second)
	e.tick()
	if e.s.count("send:") != sends+1 {
		t.Fatalf("a later turn end should retry: %v", e.s.calls)
	}
}

func TestWatcher_EveryAbortReturnPersistsCursor(t *testing.T) {
	tests := []struct {
		name  string
		abort func(e *watchEnv)
	}{
		{"ask during the handoff", func(e *watchEnv) {
			e.s.events = append(e.s.events, stop("chatter"), ask("which?"))
			e.tick()
		}},
		{"handoff timeout", func(e *watchEnv) {
			e.s.events = append(e.s.events, stop("handoff"))
			e.clock.advance(101 * time.Second)
			e.tick()
		}},
		{"resume render failure", func(e *watchEnv) {
			breakStencil(e.t, e.stDir, resumeStencilName)
			e.finishNote()
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newWatchEnv(t)
			e.injectHandoff()
			tt.abort(e)
			st := e.state()
			if st.Phase != PhaseIdle || st.LastAbortReason == "" {
				t.Fatalf("state = %+v", st)
			}
			if st.LastInjectionOffset != int64(len(e.s.events)) {
				t.Errorf("LastInjectionOffset = %d, want %d", st.LastInjectionOffset, len(e.s.events))
			}
		})
	}
}

func TestWatcher_RestartInIdleAfterResumeTimeoutStartsNoCycle(t *testing.T) {
	e := newWatchEnv(t)
	e.reachResuming()
	e.s.usage["pre"] = 5000
	e.clock.advance(101 * time.Second)
	e.tick()
	sends := e.s.count("send:")
	e.w = e.newWatcher()
	for i := 0; i < 3; i++ {
		e.clock.advance(60 * time.Second)
		e.tick()
	}
	if e.s.count("send:") != sends || e.state().Phase != PhaseIdle {
		t.Fatalf("calls = %v phase = %s", e.s.calls, e.state().Phase)
	}
}

// TestWatcher_RestartLandedInjectionNoDuplicateSend pins that a restart in any non-idle phase, over a pending step whose id is saved, takes the hold again before anything else.
// The restarted watcher reads the step's outcome and posts or injects nothing again.
func TestWatcher_RestartLandedInjectionNoDuplicateSend(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// env builds the env and drives it into the phase.
		env func(t *testing.T) *watchEnv
		// after runs the restarted watcher's measured tick and returns the phase wanted afterwards.
		after func(e *watchEnv) Phase
	}{
		{"handoff-requested", func(t *testing.T) *watchEnv {
			e := newWatchEnv(t)
			e.injectHandoff()
			return e
		}, func(e *watchEnv) Phase {
			e.s.events = append(e.s.events, stop("handoff")) // no reply yet: the phase keeps waiting
			e.tick()
			return PhaseHandoffRequested
		}},
		{"clearing", func(t *testing.T) *watchEnv {
			e := newWatchEnv(t)
			e.reachClearing()
			return e
		}, func(e *watchEnv) Phase {
			e.tick()
			return PhaseResuming
		}},
		{"resuming", func(t *testing.T) *watchEnv {
			e := newWatchEnv(t)
			e.reachResuming()
			return e
		}, func(e *watchEnv) Phase {
			e.endTurn("resumed")
			return PhaseIdle
		}},
		{"compacting", func(t *testing.T) *watchEnv {
			e := newCompactEnv(t)
			e.reachCompacting()
			return e
		}, func(e *watchEnv) Phase {
			e.tick()
			return PhaseCompacting
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			e := tt.env(t)
			sends, colors, creelCalls := len(e.s.calls), e.s.colors, len(e.c.calls)
			e.w = e.newWatcher()
			wantPhase := tt.after(e)
			if len(e.s.calls) != sends {
				t.Fatalf("calls = %v, want nothing posted or injected again", e.s.calls[sends:])
			}
			if wantPhase != PhaseResuming && e.s.colors != colors {
				t.Errorf("colors = %d, want %d", e.s.colors, colors)
			}
			if got := e.state().Phase; got != wantPhase {
				t.Errorf("phase = %s, want %s", got, wantPhase)
			}
			if first := e.c.calls[creelCalls]; first != "hold:"+fakeOrchStrandName {
				t.Errorf("first creel call of the restarted watcher = %q, want the hold taken again", first)
			}
		})
	}
}

// TestWatcher_RestartUnlandedInjectionResendsOnce pins that a restart over a pending step with no saved id adopts the step when creel journaled it.
// It sends the step once when creel journaled nothing.
func TestWatcher_RestartUnlandedInjectionResendsOnce(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		env  func(t *testing.T) *watchEnv
	}{
		{"note request", func(t *testing.T) *watchEnv {
			e := newWatchEnv(t)
			e.injectHandoff()
			return e
		}},
		{"clear", func(t *testing.T) *watchEnv {
			e := newWatchEnv(t)
			e.reachClearing()
			return e
		}},
		{"resume pointer", func(t *testing.T) *watchEnv {
			e := newWatchEnv(t)
			e.reachResuming()
			return e
		}},
		{"compact", func(t *testing.T) *watchEnv {
			e := newCompactEnv(t)
			e.reachCompacting()
			return e
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name+" absent from the journal is sent once", func(t *testing.T) {
			t.Parallel()
			e := tt.env(t)
			want := e.s.calls[len(e.s.calls)-1]
			sends := len(e.s.calls)
			e.forgetLastSend()
			e.w = e.newWatcher()
			e.tick()
			if len(e.s.calls) != sends+1 || e.s.calls[sends] != want {
				t.Fatalf("calls = %v, want %q sent once more", e.s.calls[sends:], want)
			}
		})
		t.Run(tt.name+" in the journal is adopted", func(t *testing.T) {
			t.Parallel()
			e := tt.env(t)
			sends := len(e.s.calls)
			entry := e.forgetLastSend()
			e.c.sent = []CreelSent{entry}
			e.w = e.newWatcher()
			e.tick()
			if len(e.s.calls) != sends {
				t.Fatalf("calls = %v, want the journaled step adopted and nothing sent", e.s.calls[sends:])
			}
			st := e.state()
			if entry.Mail && (st.RequestID != entry.ID || st.Pending != nil) {
				t.Errorf("state = %+v, want the adopted post's id as the request id", st)
			}
			if !entry.Mail && (st.Pending == nil || st.Pending.ID != entry.ID) && st.Phase != PhaseResuming {
				t.Errorf("pending = %+v, want the adopted inject's id", st.Pending)
			}
		})
	}
}

func TestWatcher_RestartInClearingClearsOnceAndCountsOnce(t *testing.T) {
	e := newWatchEnv(t)
	e.reachClearing()
	e.w = e.newWatcher()
	e.tick() // the saved clear is read as typed
	st := e.state()
	if st.Phase != PhaseResuming || st.CycleCount != 1 {
		t.Fatalf("state = %+v", st)
	}
	if e.s.count("clear") != 1 {
		t.Errorf("clear repeated: %v", e.s.calls)
	}
}

func breakStencil(t *testing.T, dir, name string) {
	t.Helper()
	if err := os.WriteFile(stencilstore.Path(dir, name), []byte("line one\nline two\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestWatcher_FailingHandoffRenderChangesNothing(t *testing.T) {
	e := newWatchEnv(t)
	if err := os.WriteFile(stencilstore.Path(e.stDir, handoffStencilName), []byte("{{.nope}}"), 0o644); err != nil {
		t.Fatal(err)
	}
	e.s.usage["a"] = 2000
	e.s.events = []shuttleengine.Event{stop("a")}
	if err := RequestCycle(e.paths, CycleClear, e.clock.now); err != nil {
		t.Fatal(err)
	}
	e.tick()
	e.clock.advance(11 * time.Second)
	e.tickErr()
	e.assertNoCalls()
	if e.state().Phase != PhaseIdle || e.c.count("hold:") != 0 {
		t.Errorf("phase = %s holds = %d, want idle and no hold taken", e.state().Phase, e.c.count("hold:"))
	}
	if _, requested, _ := CycleRequested(e.paths); !requested {
		t.Error("cycle request should be kept")
	}
}

// TestWatcher_NoteRequestRendersTemplateFileBeforeTyping pins that the note template file is rendered before the request is posted,
// and that the request body names the note, the template and the answers the orch gives.
func TestWatcher_NoteRequestRendersTemplateFileBeforeTyping(t *testing.T) {
	t.Run("hard request", func(t *testing.T) {
		e := newWatchEnv(t)
		e.injectHandoff()
		if _, err := os.Stat(e.paths.NoteTemplatePath); err != nil {
			t.Fatalf("note template file not rendered before the request: %v", err)
		}
		body := e.c.posts[0].Body
		for _, want := range []string{e.paths.NoteTemplatePath, e.state().PendingHandoff, "lyx creel post --reply", `"written ` + e.state().PendingHandoff + `"`, "lyx creel read"} {
			if !strings.Contains(body, want) {
				t.Errorf("note request %q must name %q", body, want)
			}
		}
		if strings.Contains(body, "defer") {
			t.Errorf("a hard request %q must not offer to defer", body)
		}
	})
	t.Run("soft request", func(t *testing.T) {
		e := newSoftEnv(t)
		e.injectSoft(stop("a"))
		body := e.c.posts[0].Body
		for _, want := range []string{e.paths.NoteTemplatePath, e.state().PendingHandoff, `"written ` + e.state().PendingHandoff + `"`, "lyx creel post --reply <id> defer"} {
			if !strings.Contains(body, want) {
				t.Errorf("soft note request %q must name %q", body, want)
			}
		}
	})
}

var reloadSkills = []string{"scribe:prose", "ly:board"}

// withSkills makes the watcher reload reloadSkills.
func (e *watchEnv) withSkills() {
	e.skills = reloadSkills
	e.w = e.newWatcher()
}

// callsAfter returns the calls made after the first call with prefix.
func (e *watchEnv) callsAfter(prefix string) []string {
	e.t.Helper()
	for i, c := range e.s.calls {
		if strings.HasPrefix(c, prefix) {
			return e.s.calls[i+1:]
		}
	}
	e.t.Fatalf("no call with prefix %q in %v", prefix, e.s.calls)
	return nil
}

// endTurn reads one turn end stamped two seconds past the clock, after any step sent so far, and ticks.
func (e *watchEnv) endTurn(msg string) {
	e.t.Helper()
	e.s.events = append(e.s.events, shuttleengine.Event{Kind: shuttleengine.EventStop, Message: msg, At: e.clock.now.Add(2 * time.Second)})
	e.tick()
}

// reloadSkillsCall is the call that loads every one of reloadSkills in one turn.
var reloadSkillsCall = "skills:" + strings.Join(reloadSkills, ",")

// assertReload checks the calls after prefix are the plugins reload, one skills load when afterClear, then a pointer naming the role file and, when note is not empty, the note.
func (e *watchEnv) assertReload(prefix, note string, afterClear bool) {
	e.t.Helper()
	want := []string{reloadPluginsCall}
	if afterClear {
		want = append(want, reloadSkillsCall)
	}
	got := e.callsAfter(prefix)
	if len(got) != len(want)+1 || !slices.Equal(got[:len(want)], want) {
		e.t.Fatalf("calls after %q = %v, want %v and a pointer", prefix, got, want)
	}
	pointer := got[len(want)]
	if !strings.HasPrefix(pointer, "send:") || !strings.Contains(pointer, e.paths.RolePath) {
		e.t.Errorf("pointer = %q, want it to name the role file", pointer)
	}
	if note != "" && !strings.Contains(pointer, note) {
		e.t.Errorf("pointer = %q, want it to name the note %s", pointer, note)
	}
	if note == "" && strings.Contains(pointer, "note") {
		e.t.Errorf("pointer = %q, want no note", pointer)
	}
}

func TestWatcher_ClearCycleReloadsSkillsThenPointer(t *testing.T) {
	t.Parallel()

	e := newWatchEnv(t)
	e.withSkills()
	e.reachClearing()
	colorsBefore := e.s.colors
	e.tick() // the clear typed: resuming, the color queued and nothing else
	if st := e.state(); st.Phase != PhaseResuming || st.ReloadStep != ReloadStepColor || st.ReloadTypedAt.IsZero() {
		t.Fatalf("state = %+v, want the color step queued", st)
	}
	if got := e.callsAfter("clear"); len(got) != 0 || e.s.colors != colorsBefore+1 {
		t.Fatalf("calls after clear = %v colors = %d, want only the color queued", got, e.s.colors-colorsBefore)
	}
	e.tick() // the color typed: the plugins reload queued
	if st := e.state(); st.Phase != PhaseResuming || st.ReloadStep != ReloadStepPlugins || st.ReloadTypedAt.IsZero() || st.ReloadSkipsSkills {
		t.Fatalf("state = %+v, want the plugins step queued", st)
	}
	if got := e.callsAfter("clear"); !slices.Equal(got, []string{reloadPluginsCall}) {
		t.Fatalf("calls after clear = %v, want only the plugins reload", got)
	}
	e.tick() // the plugins reload typed: the skills load queued
	if st := e.state(); st.Phase != PhaseResuming || st.ReloadStep != ReloadStepSkills || st.ReloadTypedAt.IsZero() {
		t.Fatalf("state = %+v", st)
	}
	holds := e.c.count("hold:")
	e.endTurn("t1")
	if st := e.state(); st.Phase != PhaseResuming || st.ReloadStep != ReloadStepPointer {
		t.Fatalf("state = %+v, want the pointer step", st)
	}
	e.s.usage["resumed"] = 300
	e.endTurn("resumed")
	e.assertReload("clear", e.state().LastHandoff, true)
	if st := e.state(); st.Phase != PhaseIdle || st.LastContextTokens != 300 || st.ReloadStep != ReloadStepSkills {
		t.Errorf("state = %+v", st)
	}
	if got := e.c.count("hold:"); got != holds+2 || e.c.count("release:") != 1 {
		t.Errorf("holds = %d releases = %d, want the hold renewed on each tick and released once at the end", got-holds, e.c.count("release:"))
	}
}

// TestWatcher_SkillSkipCauses does not call t.Parallel: it asserts on the logger output,
// which is process-global.
func TestWatcher_SkillSkipCauses(t *testing.T) {
	verified := func(unknown, missing []string) shuttleengine.SkillLoadReport {
		return shuttleengine.SkillLoadReport{Verified: true, Unknown: unknown, Missing: missing}
	}
	tests := []struct {
		name      string
		loads     map[string]shuttleengine.SkillLoadReport
		timeout   bool     // the last load turn queued never ends and passes its timeout
		endTurns  []string // turn ends read before the timeout and the pointer's
		wantSkill []string // the skills calls after the plugins reload
		wantSkips int      // the skill skipped warnings
		wantLogs  []string // fragments the log must hold
	}{
		{
			name:      "unknown skill is skipped with no retry",
			loads:     map[string]shuttleengine.SkillLoadReport{"t1": verified([]string{"scribe:prose"}, nil)},
			endTurns:  []string{"t1"},
			wantSkill: []string{reloadSkillsCall},
			wantSkips: 1,
			wantLogs:  []string{"skill=scribe:prose", "cause=unknown"},
		},
		{
			name:      "missing skill is retried once, naming only it",
			loads:     map[string]shuttleengine.SkillLoadReport{"t1": verified(nil, []string{"ly:board"})},
			endTurns:  []string{"t1", "t2"},
			wantSkill: []string{reloadSkillsCall, "skills:ly:board"},
		},
		{
			name: "skill still missing after the retry is skipped",
			loads: map[string]shuttleengine.SkillLoadReport{
				"t1": verified(nil, []string{"ly:board"}),
				"t2": verified(nil, []string{"ly:board"}),
			},
			endTurns:  []string{"t1", "t2"},
			wantSkill: []string{reloadSkillsCall, "skills:ly:board"},
			wantSkips: 1,
			wantLogs:  []string{"skill=ly:board", "cause=\"not loaded\""},
		},
		{
			name:      "unverified turn goes straight to the pointer",
			loads:     map[string]shuttleengine.SkillLoadReport{"t1": {}},
			endTurns:  []string{"t1"},
			wantSkill: []string{reloadSkillsCall},
			wantLogs:  []string{"skill load unverified"},
		},
		{
			name:      "silent skills turn is skipped at the timeout with no retry",
			timeout:   true,
			wantSkill: []string{reloadSkillsCall},
			wantSkips: len(reloadSkills),
			wantLogs:  []string{"cause=timeout"},
		},
		{
			name:      "silent retry turn skips only the retry skills with no second retry",
			loads:     map[string]shuttleengine.SkillLoadReport{"t1": verified(nil, []string{"ly:board"})},
			endTurns:  []string{"t1"},
			timeout:   true,
			wantSkill: []string{reloadSkillsCall, "skills:ly:board"},
			wantSkips: 1,
			wantLogs:  []string{"skill=ly:board", "cause=timeout"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := logcapture.Capture(t)
			e := newWatchEnv(t)
			e.withSkills()
			e.s.skillLoads = tt.loads
			e.reachClearing()
			e.tick() // clear typed, color queued
			e.tick() // color typed, plugins queued
			e.tick() // plugins typed, skills queued
			for _, turn := range tt.endTurns {
				e.endTurn(turn)
			}
			if tt.timeout {
				e.clock.advance(101 * time.Second)
				e.tick() // the silent turn's skills are skipped, the pointer queued on the same tick
			}
			if st := e.state(); st.Phase != PhaseResuming || st.ReloadStep != ReloadStepPointer || len(st.ReloadRetry) != 0 {
				t.Fatalf("state = %+v, want the pointer step with nothing left to retry", st)
			}
			e.endTurn("resumed")
			got := e.callsAfter(reloadPluginsCall)
			if len(got) != len(tt.wantSkill)+1 {
				t.Fatalf("calls after the plugins reload = %v, want %v and a pointer", got, tt.wantSkill)
			}
			for i, want := range tt.wantSkill {
				if got[i] != want {
					t.Errorf("call %d = %q, want %q", i, got[i], want)
				}
			}
			if pointer := got[len(got)-1]; !strings.HasPrefix(pointer, "send:") {
				t.Errorf("last call = %q, want the pointer", pointer)
			}
			if st := e.state(); st.Phase != PhaseIdle {
				t.Errorf("phase = %s, want idle", st.Phase)
			}
			if got := strings.Count(buf.String(), "skill skipped"); got != tt.wantSkips {
				t.Errorf("skill skipped warnings = %d, want %d; log:\n%s", got, tt.wantSkips, buf.String())
			}
			for _, want := range tt.wantLogs {
				if !strings.Contains(buf.String(), want) {
					t.Errorf("log lacks %q; log:\n%s", want, buf.String())
				}
			}
		})
	}
}

// TestWatcher_ReloadRestartRetypesOnlyTheUnconfirmedStep pins that a restart at each reload step, with the step pending and no id saved, adopts that step's own journal entry.
// It never adopts an earlier or later step's.
func TestWatcher_ReloadRestartRetypesOnlyTheUnconfirmedStep(t *testing.T) {
	t.Parallel()

	steps := []int{ReloadStepColor, ReloadStepPlugins, ReloadStepSkills, ReloadStepRetry, ReloadStepPointer}
	for _, step := range steps {
		t.Run(reloadStepName(step), func(t *testing.T) {
			t.Parallel()
			e := newWatchEnv(t)
			e.withSkills()
			e.reachReloadStep(step)
			pointer := e.state().PendingResume
			catalog := map[int]CreelSent{
				ReloadStepColor:   {Step: CreelStep{Command: commandColor, Argument: "colour208"}},
				ReloadStepPlugins: {Step: CreelStep{Command: commandReloadPlugins}},
				ReloadStepSkills:  {Step: CreelStep{Command: commandLoadSkills, Argument: strings.Join(reloadSkills, " ")}},
				ReloadStepRetry:   {Step: CreelStep{Command: commandLoadSkills, Argument: "ly:board"}},
				ReloadStepPointer: {Subject: pointer},
			}
			own := e.forgetLastSend()
			sends, injects := len(e.s.calls), len(e.c.injects)
			// Every other step's entry sits in the journal at the same time, and the own entry comes last.
			e.c.sent = nil
			for _, other := range steps {
				if other != step {
					entry := catalog[other]
					entry.ID, entry.At = int64(10+other+3), e.clock.now
					e.c.sent = append(e.c.sent, entry)
				}
			}
			e.c.sent = append(e.c.sent, own)
			e.w = e.newWatcher()
			e.tick()
			st := e.state()
			if st.Pending == nil || st.Pending.ID != own.ID {
				t.Fatalf("pending = %+v, want the step's own journal entry %d adopted", st.Pending, own.ID)
			}
			if len(e.s.calls) != sends || len(e.c.injects) != injects {
				t.Errorf("calls = %v, want nothing sent", e.s.calls[sends:])
			}
			if e.reloadStepOf(st) != step {
				t.Errorf("reload step = %d, want it kept at %d", e.reloadStepOf(st), step)
			}
		})
	}
}

// reloadStepOf returns the normalised reload step st is in.
func (e *watchEnv) reloadStepOf(st State) int {
	return e.w.reloadStep(st)
}

func TestWatcher_ReloadRestartAroundTheRetryStep(t *testing.T) {
	t.Parallel()
	e := newWatchEnv(t)
	e.withSkills()
	e.s.skillLoads = map[string]shuttleengine.SkillLoadReport{
		"t1": {Verified: true, Missing: []string{"ly:board"}},
		"t2": {Verified: true, Missing: []string{"ly:board"}},
	}
	e.reachClearing()
	e.tick()        // clear typed, color queued
	e.tick()        // color typed, plugins queued
	e.tick()        // plugins typed, skills queued
	e.endTurn("t1") // moves to the retry step, queued on the same tick
	if st := e.state(); st.ReloadStep != ReloadStepRetry || len(st.ReloadRetry) != 1 || st.ReloadRetry[0] != "ly:board" {
		t.Fatalf("state = %+v, want the retry step naming ly:board", st)
	}

	// A restart in the retry step reads the saved retry's outcome and sends nothing, and never retries a second time.
	e.w = e.newWatcher()
	e.tick()
	got := e.callsAfter("clear")
	if len(got) != 3 || got[2] != "skills:ly:board" {
		t.Fatalf("calls after clear = %v, want the retry sent once", got)
	}
	e.endTurn("t2")
	if st := e.state(); st.ReloadStep != ReloadStepPointer || len(st.ReloadRetry) != 0 {
		t.Fatalf("state = %+v, want the pointer step", st)
	}

	// A restart in the pointer step starts no retry.
	e.w = e.newWatcher()
	e.tick()
	if e.s.count("skills:ly:board") != 1 {
		t.Fatalf("a second retry was sent: %v", e.s.calls)
	}
}

func TestWatcher_ReloadReadsAnUnreadableStepAsThePointer(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		step  int
		retry []string
	}{
		{name: "old per-skill index past the skills step", step: 1},
		{name: "retry step with an empty retry list", step: ReloadStepRetry},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			e := newWatchEnv(t)
			e.withSkills()
			e.reachClearing()
			e.tick() // clear typed, color queued
			e.tick() // color typed, plugins queued
			e.setState(func(st *State) {
				st.ReloadStep, st.ReloadRetry, st.ReloadTypedAt, st.Pending = tt.step, tt.retry, time.Time{}, nil
			})
			e.w = e.newWatcher()
			e.tick()
			got := e.callsAfter("clear")
			if len(got) != 2 || !strings.HasPrefix(got[1], "send:") {
				t.Fatalf("calls after clear = %v, want the pointer sent after the persisted step", got)
			}
		})
	}
}

func TestWatcher_HookDeliveredPointerIsSkippedAndAnythingElseIsTyped(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// mark returns the mark written before the reload starts, and false for none.
		mark      func(boundary time.Time, pointer string) (ResumeMark, bool)
		wantTyped bool
	}{
		{"a mark after the boundary holding the pointer skips it", func(boundary time.Time, pointer string) (ResumeMark, bool) {
			return ResumeMark{At: boundary.Add(time.Second), Text: pointer}, true
		}, false},
		{"no mark types the pointer", func(time.Time, string) (ResumeMark, bool) { return ResumeMark{}, false }, true},
		{"a mark older than the boundary types the pointer", func(boundary time.Time, pointer string) (ResumeMark, bool) {
			return ResumeMark{At: boundary.Add(-time.Second), Text: pointer}, true
		}, true},
		{"a mark with other text types the pointer", func(boundary time.Time, _ string) (ResumeMark, bool) {
			return ResumeMark{At: boundary.Add(time.Second), Text: "something else"}, true
		}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			e := newWatchEnv(t)
			e.withSkills()
			boundary := e.clock.now.Add(time.Second)
			e.s.autoCompact = map[string]shuttleengine.CompactionBoundary{"a": freshBoundary(boundary)}
			e.s.usage["a"] = 100
			pointer, err := RenderReloadPrompt(e.stDir, e.paths.RolePath)
			if err != nil {
				t.Fatal(err)
			}
			if mark, ok := tt.mark(boundary, pointer); ok {
				if err := WriteResumeMark(e.paths, mark); err != nil {
					t.Fatal(err)
				}
			}
			e.endTurn("a") // the boundary is read and the reload starts: the color queued
			e.tick()       // the color typed, the plugins queued
			e.tick()       // the plugins typed, the pointer step

			wantCalls := []string{reloadPluginsCall}
			if tt.wantTyped {
				wantCalls = append(wantCalls, "send:"+pointer)
				if st := e.state(); st.Phase != PhaseResuming {
					t.Fatalf("phase = %s, want resuming until a turn end", st.Phase)
				}
				e.endTurn("resumed")
			}
			if !slices.Equal(e.s.calls, wantCalls) {
				t.Errorf("calls = %q, want %q", e.s.calls, wantCalls)
			}
			if st := e.state(); st.Phase != PhaseIdle || st.LastAbortReason != "" {
				t.Errorf("state = %+v, want idle with no abort reason", st)
			}
			if _, found, err := ReadResumeMark(e.paths); err != nil || found {
				t.Errorf("mark found = %v, %v after the reload ended; want none", found, err)
			}
		})
	}
}

func TestWatcher_AutoCompactionReloadsPluginsThenPointer(t *testing.T) {
	t.Parallel()

	e := newWatchEnv(t)
	e.withSkills()
	boundary := e.clock.now.Add(time.Second)
	e.s.autoCompact = map[string]shuttleengine.CompactionBoundary{"a": freshBoundary(boundary)}
	e.s.usage["a"] = 100
	e.endTurn("a")
	if st := e.state(); st.Phase != PhaseResuming || !st.CompactionBaseline.Equal(boundary) || !st.ReloadSkipsSkills || st.ReloadStep != ReloadStepColor {
		t.Fatalf("state = %+v, want resuming at the color step with the baseline at the boundary", st)
	}
	if got := e.c.count("hold:"); got != 1 {
		t.Fatalf("holds = %d, want the hold taken when the reload started", got)
	}
	if role, err := os.ReadFile(e.paths.RolePath); err != nil || !strings.Contains(string(role), testIndex) {
		t.Errorf("role file = %q, %v; want it rendered with the index", role, err)
	}
	e.tick() // the color typed, the plugins reload queued
	e.tick() // the plugins typed, the pointer queued with no skills step in between
	if got := e.c.count("hold:"); got != 3 || e.c.count("release:") != 0 {
		t.Errorf("holds = %d releases = %d, want the hold renewed through the steps and kept", got, e.c.count("release:"))
	}
	e.endTurn("resumed")
	if got := e.s.calls; len(got) != 2 || got[0] != reloadPluginsCall || e.s.count("skills:") != 0 {
		t.Fatalf("calls = %v, want the plugins reload then the pointer and no skills load", got)
	}
	pointer := e.s.calls[len(e.s.calls)-1]
	if !strings.Contains(pointer, e.paths.RolePath) || strings.Contains(pointer, "note") {
		t.Errorf("pointer = %q, want the role file and no note", pointer)
	}
	if st := e.state(); st.Phase != PhaseIdle {
		t.Errorf("phase = %s, want idle", st.Phase)
	}
	if got := e.c.count("release:"); got != 1 {
		t.Errorf("releases = %d, want the hold released when the reload ended", got)
	}
	e.endTurn("later") // the same boundary again: no second reload
	e.tick()
	if len(e.s.calls) != 2 {
		t.Errorf("a handled boundary reloaded twice: %v", e.s.calls)
	}
}

func TestWatcher_AutoCompactionBoundaryAtOrBeforeBaselineTriggersNothing(t *testing.T) {
	e := newWatchEnv(t)
	e.withSkills()
	e.setState(func(st *State) { st.CompactionBaseline = e.clock.now })
	e.w = e.newWatcher()
	e.s.autoCompact = map[string]shuttleengine.CompactionBoundary{"a": freshBoundary(e.clock.now)}
	e.s.usage["a"] = 100
	e.endTurn("a")
	e.tick()
	e.assertNoCalls()
	if st := e.state(); st.Phase != PhaseIdle {
		t.Errorf("phase = %s", st.Phase)
	}
}

// freshBoundary is a compaction boundary with exactly one turn end after it, the one being read.
func freshBoundary(at time.Time) shuttleengine.CompactionBoundary {
	return shuttleengine.CompactionBoundary{At: at, TurnEndsAfter: 1, ReadTurnEndAfter: true}
}

func TestWatcher_AutoCompactionReloadGoesIdleWhenTheIndexSourceFails(t *testing.T) {
	t.Parallel()

	e := newWatchEnv(t)
	e.indexErr = errBoom
	e.s.autoCompact = map[string]shuttleengine.CompactionBoundary{"a": freshBoundary(e.clock.now.Add(time.Second))}
	e.s.usage["a"] = 100
	e.endTurn("a")
	st := e.state()
	if want := "command index unavailable: " + errBoom.Error(); st.Phase != PhaseIdle || st.LastAbortReason != want {
		t.Fatalf("state = %+v, want idle with the reason %q", st, want)
	}
	e.tick()
	if e.indexCalls != 1 {
		t.Errorf("index source called %d times, want once: a later tick without a new turn end must not retry the reload", e.indexCalls)
	}
	e.assertNoCalls()
}

func TestWatcher_AutoCompactionReloadsOnlyAFreshBoundary(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		run  func(t *testing.T, e *watchEnv, boundary time.Time)
	}{
		{"stale boundary on an old cursor moves the baseline and types nothing", func(t *testing.T, e *watchEnv, boundary time.Time) {
			e.s.events = []shuttleengine.Event{stop("a"), stop("b")}
			e.s.autoCompact = map[string]shuttleengine.CompactionBoundary{"b": {At: boundary, TurnEndsAfter: 2, ReadTurnEndAfter: true}}
			e.w = e.newWatcher()
			e.tick()
			e.assertNoCalls()
			if st := e.state(); st.Phase != PhaseIdle || !st.CompactionBaseline.Equal(boundary) {
				t.Fatalf("state = %+v, want idle with the baseline at the boundary", st)
			}
		}},
		{"unread later turn end waits, then the tick that reads it reloads", func(t *testing.T, e *watchEnv, boundary time.Time) {
			e.s.autoCompact = map[string]shuttleengine.CompactionBoundary{
				"a": {At: boundary, TurnEndsAfter: 1},
				"b": freshBoundary(boundary),
			}
			e.endTurn("a")
			e.assertNoCalls()
			if st := e.state(); !st.CompactionBaseline.IsZero() {
				t.Fatalf("baseline = %v, want it left where it was", st.CompactionBaseline)
			}
			e.endTurn("b")
			if st := e.state(); st.Phase != PhaseResuming || st.ReloadStep != ReloadStepColor {
				t.Fatalf("state = %+v, want the reload entered at its color step", st)
			}
		}},
		{"no later turn end types nothing and keeps the baseline", func(t *testing.T, e *watchEnv, boundary time.Time) {
			e.s.autoCompact = map[string]shuttleengine.CompactionBoundary{"a": {At: boundary}}
			e.endTurn("a")
			e.assertNoCalls()
			if st := e.state(); !st.CompactionBaseline.IsZero() {
				t.Fatalf("baseline = %v, want it left where it was", st.CompactionBaseline)
			}
		}},
		{"boundary held by a refused hold found stale by the next turn end types nothing", func(t *testing.T, e *watchEnv, boundary time.Time) {
			e.s.autoCompact = map[string]shuttleengine.CompactionBoundary{
				"a": freshBoundary(boundary),
				"b": {At: boundary, TurnEndsAfter: 2, ReadTurnEndAfter: true},
			}
			e.c.failNext("hold", errBoom)
			e.endTurn("a")
			e.assertNoCalls()
			if st := e.state(); st.Phase != PhaseIdle || !st.StuckByCreel {
				t.Fatalf("state = %+v, want idle with the refused hold recorded", st)
			}
			e.endTurn("b")
			e.tick()
			e.assertNoCalls()
			if st := e.state(); !st.CompactionBaseline.Equal(boundary) {
				t.Fatalf("baseline = %v, want the boundary", st.CompactionBaseline)
			}
		}},
		{"boundary held for the old strand never reloads the new one", func(t *testing.T, e *watchEnv, boundary time.Time) {
			e.s.autoCompact = map[string]shuttleengine.CompactionBoundary{"a": freshBoundary(boundary)}
			e.c.failNext("hold", errBoom)
			e.endTurn("a")
			e.setState(func(st *State) {
				st.Strand = "s2"
				st.LastInjectionOffset = int64(len(e.s.events))
			})
			e.tick()
			e.assertNoCalls()
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			e := newWatchEnv(t)
			e.withSkills()
			e.s.usage["a"], e.s.usage["b"] = 100, 100
			c.run(t, e, e.clock.now.Add(time.Second))
		})
	}
}

func TestWatcher_ClearRendersRoleFileBeforeResume(t *testing.T) {
	e := newWatchEnv(t)
	e.injectHandoff()
	e.finishNote()
	if e.s.count("clear") != 1 {
		t.Fatalf("calls = %v; want the clear sent", e.s.calls)
	}
	if role, err := os.ReadFile(e.paths.RolePath); err != nil || !strings.Contains(string(role), testIndex) {
		t.Errorf("role file = %q, %v; want it rendered with the index before the resume pointer", role, err)
	}
	if !strings.Contains(e.state().PendingResume, e.paths.RolePath) {
		t.Errorf("resume pointer %q must name the role file", e.state().PendingResume)
	}
}

func TestWatcher_FailingRenderAbortsWithoutClear(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		stencil string
		content string
		// indexErr makes the index source fail instead of the stencil, which stays as shipped.
		indexErr error
		// rulesDir makes the repository rules unreadable by creating a directory at their path, and wants the reason to name that path.
		rulesDir bool
		// wantReason is the abort reason's prefix, which names the failing source.
		wantReason string
	}{
		{name: "role", stencil: roleStencilName, content: "{{.nope}}", wantReason: "role file failed to render: orch: " + roleStencilName},
		{name: "resume", stencil: resumeStencilName, content: "line one\nline two\n", wantReason: "resume stencil " + resumeStencilName},
		{name: "index source", stencil: roleStencilName, indexErr: errBoom, wantReason: "command index unavailable: " + errBoom.Error()},
		{name: "repository rules", rulesDir: true, wantReason: "role file failed to render: "},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			e := newWatchEnv(t)
			e.injectHandoff()
			if c.indexErr != nil {
				e.indexErr = c.indexErr
			} else if c.rulesDir {
				if err := os.MkdirAll(e.paths.RepoRulesPath, 0o755); err != nil {
					t.Fatal(err)
				}
			} else if err := os.WriteFile(stencilstore.Path(e.stDir, c.stencil), []byte(c.content), 0o644); err != nil {
				t.Fatal(err)
			}
			e.finishNote()
			st := e.state()
			if st.Phase != PhaseIdle || e.s.count("clear") != 0 || !strings.HasPrefix(st.LastAbortReason, c.wantReason) {
				t.Fatalf("state = %+v calls = %v", st, e.s.calls)
			}
			if c.rulesDir && !strings.Contains(st.LastAbortReason, e.paths.RepoRulesPath) {
				t.Errorf("abort reason %q does not name %s", st.LastAbortReason, e.paths.RepoRulesPath)
			}
		})
	}
}

func TestWatcher_HandoffSendErrorResendsSamePathOnce(t *testing.T) {
	e := newWatchEnv(t)
	e.c.failNext("post", errBoom)
	e.s.usage["a"] = 2000
	e.s.events = []shuttleengine.Event{stop("a")}
	e.tick()
	e.clock.advance(11 * time.Second)
	e.tick()
	st := e.state()
	if st.Phase != PhaseHandoffRequested || st.Pending == nil || st.Pending.ID != 0 || st.Stuck != errBoom.Error() || !st.StuckByCreel {
		t.Fatalf("state = %+v, want the request pending with no id and the failure recorded", st)
	}
	if len(e.s.calls) != 0 {
		t.Fatalf("calls = %v, want nothing accepted by the failed call", e.s.calls)
	}
	e.tick()
	e.tick()
	if len(e.s.calls) != 1 || !strings.Contains(e.s.calls[0], st.PendingHandoff) {
		t.Fatalf("calls = %v, want the same request posted once, naming %s", e.s.calls, st.PendingHandoff)
	}
	if st := e.state(); st.Pending != nil || st.RequestID == 0 || st.Stuck != "" {
		t.Errorf("state = %+v, want the request's id saved and the failure cleared", st)
	}
}

func TestWatcher_ResumeSendErrorResendsOnce(t *testing.T) {
	e := newWatchEnv(t)
	e.reachClearing()
	e.tick() // clear typed, color queued
	e.tick() // color typed, plugins queued
	e.c.failNext("inject", errBoom)
	e.tick() // plugins typed, the pointer's inject fails
	if st := e.state(); st.Pending == nil || st.Pending.ID != 0 || e.s.count("send:") != 1 {
		t.Fatalf("state = %+v calls = %v, want the pointer pending with no id", st, e.s.calls)
	}
	e.tick()
	e.tick()
	if e.s.count("send:") != 2 || e.s.calls[len(e.s.calls)-1] != "send:"+e.state().PendingResume {
		t.Fatalf("calls = %v, want the pointer sent once", e.s.calls)
	}
}

// TestWatcher_PhaseForDifferentStrandIsReset pins the reset of a phase that belongs to another strand:
// it withdraws, releases the old hold and clears the noticed mark of an entry whose resume line it withdrew,
// and a reset whose withdraw fails saves no reset and runs again on the next tick.
func TestWatcher_PhaseForDifferentStrandIsReset(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		run  func(t *testing.T, e *watchEnv)
	}{
		{"the reset withdraws, releases the old hold and clears the mark of an entry whose resume line was withdrawn", func(t *testing.T, e *watchEnv) {
			e.noticedResumedEntry()
			e.c.withdrawn = []CreelWithdrawn{{ID: 5, Subject: pauserecord.ResumeLine}}
			e.tick()
			st := e.state()
			if st.Phase != PhaseIdle || !strings.Contains(st.LastAbortReason, "resuming") || st.Pending != nil || st.RequestID != 0 {
				t.Fatalf("state = %+v", st)
			}
			if e.c.count("withdraw:") != 1 || e.c.count("release:") != 1 {
				t.Errorf("creel calls = %v, want one withdraw and one release", e.c.calls)
			}
			// The cleared mark has the watcher post the withdrawn line again on the same tick, and mark it noticed once more.
			if bodies := e.c.postBodies(fakeOrchStrandName); !slices.Equal(bodies, []string{pauserecord.ResumeLine}) {
				t.Errorf("posts to the orch = %v, want the resume line posted again", bodies)
			}
			if sessions := e.orchSessions(); len(sessions) != 1 || !sessions[0].ResumeNoticed {
				t.Errorf("orch sessions = %+v, want the line noticed again", sessions)
			}
		}},
		{"an entry is left alone when the withdrawn message is not the resume line", func(t *testing.T, e *watchEnv) {
			e.noticedResumedEntry()
			e.c.withdrawn = []CreelWithdrawn{{ID: 5, Subject: "some other mail"}}
			e.tick()
			if st := e.state(); st.Phase != PhaseIdle {
				t.Fatalf("state = %+v", st)
			}
			if sessions := e.orchSessions(); len(sessions) != 1 || !sessions[0].ResumeNoticed {
				t.Errorf("orch sessions = %+v, want the mark kept", sessions)
			}
			e.assertNoCalls()
		}},
		{"a reset whose mark clear fails after the withdraw removed the resume line still clears the mark on the next tick", func(t *testing.T, e *watchEnv) {
			e.noticedResumedEntry()
			e.c.withdrawn = []CreelWithdrawn{{ID: 5, Subject: pauserecord.ResumeLine}}
			lock := e.pausePath + ".lock"
			if err := os.Remove(lock); err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(lock, 0o755); err != nil {
				t.Fatal(err)
			}
			e.tickErr()
			if st := e.state(); st.Phase != PhaseResuming || !st.ResumeLineWithdrawn {
				t.Fatalf("state = %+v, want the phase kept and the withdrawn line recorded", st)
			}
			if err := os.Remove(lock); err != nil {
				t.Fatal(err)
			}
			e.c.withdrawn = nil
			e.tick()
			if st := e.state(); st.Phase != PhaseIdle || st.ResumeLineWithdrawn {
				t.Fatalf("state = %+v, want the reset done and the record cleared", st)
			}
			if bodies := e.c.postBodies(fakeOrchStrandName); !slices.Equal(bodies, []string{pauserecord.ResumeLine}) {
				t.Errorf("posts to the orch = %v, want the resume line posted again", bodies)
			}
		}},
		{"a reset whose withdraw fails saves no reset and runs again on the next tick", func(t *testing.T, e *watchEnv) {
			e.c.failNext("withdraw", errBoom)
			e.tick()
			st := e.state()
			if st.Phase != PhaseResuming || st.PhaseStrand != "old" || st.Stuck != errBoom.Error() || !st.StuckByCreel {
				t.Fatalf("state = %+v, want the phase kept with the failure recorded", st)
			}
			e.tick()
			if st := e.state(); st.Phase != PhaseIdle || st.Stuck != "" || e.c.count("withdraw:") != 2 {
				t.Fatalf("state = %+v creel calls = %v, want the reset run again and the failure cleared", st, e.c.calls)
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			e := newWatchEnv(t)
			e.setState(func(s *State) {
				s.Phase = PhaseResuming
				s.PhaseStrand = "old"
				s.PendingResume = "resume"
				s.Pending = &PendingStep{Command: "clear", ID: 9}
				s.RequestID = 4
			})
			tt.run(t, e)
		})
	}
}

// noticedResumedEntry records a resumed hub entry that owns the orch s1 and has noticed its resume line.
func (e *watchEnv) noticedResumedEntry() {
	e.t.Helper()
	entry := pauserecord.Entry{
		Scope: pauserecord.ScopeHub, PausedAt: e.clock.now.Add(-time.Minute), ResumedAt: e.clock.now,
		Sessions: []pauserecord.Session{{GUID: "s1", Orch: true, ResumeNoticed: true}},
	}
	if _, err := pauserecord.Update(e.pausePath, e.clock.now, func(r *pauserecord.Record) error {
		r.Resumed = append(r.Resumed, entry)
		return nil
	}); err != nil {
		e.t.Fatal(err)
	}
}

func TestWatcher_StrandGoneIsDone(t *testing.T) {
	e := newWatchEnv(t)
	e.s.alive = false
	done, err := e.w.Tick()
	if err != nil || !done {
		t.Fatalf("Tick = %v, %v", done, err)
	}
	if e.state().WatcherExit == "" {
		t.Error("WatcherExit should be recorded")
	}
}

func TestWatcher_StrandReboundMidTickIsNotOverwritten(t *testing.T) {
	e := newWatchEnv(t)
	e.s.alive = false
	e.s.onAlive = func() {
		e.setState(func(s *State) { s.Strand = "s2" })
	}
	done, err := e.w.Tick()
	if err != nil || done {
		t.Fatalf("Tick = %v, %v; want false, nil", done, err)
	}
	if st := e.state(); st.Strand != "s2" || st.WatcherExit != "" {
		t.Fatalf("the rebound state was overwritten: %+v", st)
	}

	e.s.alive, e.s.onAlive = true, nil
	e.tick()
	if e.w.strand != "s2" {
		t.Errorf("watcher strand = %q; want s2", e.w.strand)
	}

	t.Run("a session id read for a rebound strand is not written", func(t *testing.T) {
		e := newWatchEnv(t)
		e.setState(func(s *State) { s.SessionID = "launch-id" })
		e.s.sessionID = "other-id"
		e.s.onAlive = func() {
			e.setState(func(s *State) { s.Strand, s.SessionID = "s2", "" })
		}
		done, err := e.w.Tick()
		if err != nil || done {
			t.Fatalf("Tick = %v, %v; want false, nil", done, err)
		}
		if st := e.state(); st.Strand != "s2" || st.SessionID != "" {
			t.Errorf("the rebound state was overwritten: strand %q, session id %q; want s2 and empty", st.Strand, st.SessionID)
		}
	})
}

func TestWatcher_RecordsTheSessionIDItReads(t *testing.T) {
	tests := []struct {
		name      string
		recorded  string
		read      string
		readErr   error
		want      string
		wantTouch bool
	}{
		{name: "a reading naming a new id records it", recorded: "launch-id", read: "post-clear-id", want: "post-clear-id", wantTouch: true},
		{name: "an unchanged id writes nothing", recorded: "same-id", read: "same-id", want: "same-id"},
		{name: "a reading naming no id keeps the recorded one", recorded: "post-clear-id", want: "post-clear-id"},
		{name: "an unreadable reading keeps the id and the tick goes on", recorded: "launch-id", readErr: errBoom, want: "launch-id"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newWatchEnv(t)
			// The first two ticks queue the start color and record it, so the measured tick has nothing else to save.
			e.tick()
			e.tick()
			e.setState(func(s *State) { s.SessionID = tt.recorded })
			past := e.clock.now.Add(-time.Hour)
			if err := os.Chtimes(e.paths.StatePath, past, past); err != nil {
				t.Fatal(err)
			}
			e.s.sessionID, e.s.sessionStateErr = tt.read, tt.readErr

			done, err := e.w.Tick()
			if err != nil || done {
				t.Fatalf("Tick = %v, %v; want false, nil", done, err)
			}
			if got := e.state().SessionID; got != tt.want {
				t.Errorf("SessionID = %q; want %q", got, tt.want)
			}
			info, err := os.Stat(e.paths.StatePath)
			if err != nil {
				t.Fatal(err)
			}
			if touched := !info.ModTime().Equal(past); touched != tt.wantTouch {
				t.Errorf("state file rewritten = %v; want %v", touched, tt.wantTouch)
			}
		})
	}
}

// newSoftEnv is a watch env with soft threshold 500 and soft idle 20s under the hard cap 1000 and grace 10s.
func newSoftEnv(t *testing.T) *watchEnv {
	t.Helper()
	e := newWatchEnv(t)
	e.cfg.SoftThresholdTokens, e.cfg.SoftIdleS = 500, 20
	e.w = e.newWatcher()
	return e
}

func waiting(msg string) shuttleengine.Event {
	return shuttleengine.Event{Kind: shuttleengine.EventWaiting, Message: msg}
}

// injectSoft reads one turn end ev at a soft-range reading and ticks past soft_idle_s, requiring a soft handoff request.
func (e *watchEnv) injectSoft(ev shuttleengine.Event) {
	e.t.Helper()
	e.s.usage[ev.Message] = 600
	e.s.events = append(e.s.events, ev)
	e.tick()
	e.clock.advance(19 * time.Second)
	e.tick()
	e.assertNoCalls()
	e.clock.advance(time.Second)
	e.tick()
	st := e.state()
	if st.Phase != PhaseHandoffRequested || st.CycleTrigger != TriggerSoft {
		e.t.Fatalf("state = %+v, want a soft handoff request", st)
	}
}

func TestWatcher_SoftTriggerFiresOnStopAndWaitingAfterSoftIdle(t *testing.T) {
	for name, ev := range map[string]shuttleengine.Event{"stop": stop("a"), "waiting": waiting("a")} {
		t.Run(name, func(t *testing.T) {
			e := newSoftEnv(t)
			e.injectSoft(ev)
			if len(e.s.calls) != 1 || !strings.Contains(e.s.calls[0], "defer") {
				t.Fatalf("calls = %v, want one soft request", e.s.calls)
			}
		})
	}
}

func TestWatcher_SoftDeferWithoutFileReturnsIdleAndBlocksNextAttempt(t *testing.T) {
	e := newSoftEnv(t)
	e.injectSoft(stop("a"))
	e.reply("defer")
	e.tick()
	read := e.clock.Now()
	st := e.state()
	if st.Phase != PhaseIdle || st.LastAbortReason != "deferred" || !st.LastDeferral.Equal(read) {
		t.Fatalf("state = %+v", st)
	}
	sends := e.s.count("send:")
	e.s.usage["b"] = 600
	e.s.events = append(e.s.events, stop("b"))
	e.tick()
	e.clock.advance(19 * time.Second)
	e.tick()
	if e.s.count("send:") != sends {
		t.Fatalf("soft attempt before soft_idle_s after the deferral: %v", e.s.calls)
	}
	e.clock.advance(time.Second)
	e.tick()
	if e.s.count("send:") != sends+1 || e.state().CycleTrigger != TriggerSoft {
		t.Fatalf("calls = %v state = %+v", e.s.calls, e.state())
	}
}

// TestWatcher_SoftDeferTurnEndAloneNeverRequalifies pins that a deferral needs a fresh turn end after it:
// the deferral itself never starts another soft cycle, however long the session stays quiet.
func TestWatcher_SoftDeferTurnEndAloneNeverRequalifies(t *testing.T) {
	e := newSoftEnv(t)
	e.injectSoft(stop("a"))
	e.reply("defer")
	e.tick()
	if st := e.state(); st.Phase != PhaseIdle || st.LastAbortReason != "deferred" {
		t.Fatalf("state = %+v, want idle after the deferral", st)
	}
	sends := e.s.count("send:")
	e.clock.advance(time.Hour)
	e.tick()
	if e.s.count("send:") != sends || e.state().Phase != PhaseIdle {
		t.Fatalf("a soft request was re-sent with no turn end after the deferral: calls = %v", e.s.calls)
	}
}

func TestWatcher_SoftDeferAmongOtherTextDoesNotDefer(t *testing.T) {
	e := newSoftEnv(t)
	e.injectSoft(stop("a"))
	e.reply("defer, but first a note")
	e.tick()
	if st := e.state(); st.Phase != PhaseHandoffRequested || !st.LastDeferral.IsZero() {
		t.Fatalf("state = %+v", st)
	}
}

func TestWatcher_SoftDeferWithHandoffWrittenProceedsToClearing(t *testing.T) {
	e := newSoftEnv(t)
	e.injectSoft(stop("a"))
	path := e.writeHandoff()
	e.reply("defer")
	e.tick()
	if st := e.state(); st.Phase != PhaseHandoffRequested || !st.LastDeferral.IsZero() {
		t.Fatalf("state = %+v, want a defer ignored once the note is written", st)
	}
	e.reply("written " + path)
	e.tick()
	if st := e.state(); st.Phase != PhaseClearing || !st.LastDeferral.IsZero() {
		t.Fatalf("state = %+v", st)
	}
}

func TestWatcher_DeferIgnoredInHardAndRequestedCycles(t *testing.T) {
	t.Run("hard", func(t *testing.T) {
		e := newWatchEnv(t)
		e.injectHandoff()
		e.reply("defer")
		e.tick()
		if st := e.state(); st.Phase != PhaseHandoffRequested || st.CycleTrigger != TriggerHard {
			t.Fatalf("state = %+v", st)
		}
		e.clock.advance(101 * time.Second)
		e.tick()
		if st := e.state(); st.Phase != PhaseIdle || st.LastAbortReason != "handoff timed out" || !st.LastDeferral.IsZero() {
			t.Fatalf("state = %+v", st)
		}
	})
	t.Run("requested", func(t *testing.T) {
		e := newWatchEnv(t)
		e.s.usage["a"] = 10
		e.s.events = []shuttleengine.Event{stop("a")}
		if err := RequestCycle(e.paths, CycleClear, e.clock.now); err != nil {
			t.Fatal(err)
		}
		e.tick()
		e.clock.advance(11 * time.Second)
		e.tick()
		e.reply("defer")
		e.tick()
		if st := e.state(); st.Phase != PhaseHandoffRequested || st.CycleTrigger != TriggerRequested {
			t.Fatalf("state = %+v", st)
		}
		e.clock.advance(101 * time.Second)
		e.tick()
		if st := e.state(); st.Phase != PhaseIdle || st.LastAbortReason != "handoff timed out" || !st.LastDeferral.IsZero() {
			t.Fatalf("state = %+v", st)
		}
	})
}

func TestWatcher_HardTriggerFiresOnWaitingAfterIdleGrace(t *testing.T) {
	e := newSoftEnv(t)
	e.s.usage["a"] = 2000
	e.s.events = []shuttleengine.Event{waiting("a")}
	e.tick()
	e.clock.advance(11 * time.Second)
	e.tick()
	st := e.state()
	if st.Phase != PhaseHandoffRequested || st.CycleTrigger != TriggerHard {
		t.Fatalf("state = %+v", st)
	}
	if len(e.s.calls) != 1 || strings.Contains(e.s.calls[0], "defer") {
		t.Fatalf("calls = %v, want the plain handoff request", e.s.calls)
	}
}

func TestWatcher_HardTriggerKeepsIdleGraceWhenLongerThanSoftIdle(t *testing.T) {
	e := newSoftEnv(t)
	e.cfg.IdleGraceS = 30
	e.w = e.newWatcher()
	e.s.usage["a"] = 1000
	e.s.events = []shuttleengine.Event{stop("a")}
	e.tick()
	e.clock.advance(25 * time.Second)
	e.tick()
	e.assertNoCalls()
	e.clock.advance(6 * time.Second)
	e.tick()
	if st := e.state(); st.Phase != PhaseHandoffRequested || st.CycleTrigger != TriggerHard {
		t.Fatalf("state = %+v", st)
	}
}

func TestWatcher_SoftThresholdAtOrAboveHardNeverFires(t *testing.T) {
	for _, soft := range []int{1000, 1500} {
		e := newWatchEnv(t)
		e.cfg.SoftThresholdTokens = soft
		e.w = e.newWatcher()
		e.s.usage["a"] = 900
		e.s.events = []shuttleengine.Event{stop("a")}
		e.tick()
		e.clock.advance(time.Hour)
		e.tick()
		e.assertNoCalls()
	}
}

func TestWatcher_RequestedTriggerRecorded(t *testing.T) {
	e := newSoftEnv(t)
	e.s.usage["a"] = 10
	e.s.events = []shuttleengine.Event{stop("a")}
	if err := RequestCycle(e.paths, CycleClear, e.clock.now); err != nil {
		t.Fatal(err)
	}
	e.tick()
	e.clock.advance(11 * time.Second)
	e.tick()
	if st := e.state(); st.CycleTrigger != TriggerRequested {
		t.Fatalf("CycleTrigger = %q", st.CycleTrigger)
	}
}

func TestWatcher_RestartInSoftHandoffResendsSoftStencil(t *testing.T) {
	e := newSoftEnv(t)
	e.injectSoft(stop("a"))
	first := e.s.calls[0]
	e.forgetLastSend()
	e.w = e.newWatcher()
	e.tick()
	e.tick()
	if len(e.s.calls) != 2 || e.s.calls[1] != first || !strings.Contains(e.s.calls[1], "defer") {
		t.Fatalf("calls = %v, want the soft stencil posted again", e.s.calls)
	}
}

// TestWatcher_StaleReadingIsRereadBeforeFiring replays the 2026-10-04 incident:
// the transcript is compacted without a turn end, so the reading saved at the last turn end is stale.
func TestWatcher_StaleReadingIsRereadBeforeFiring(t *testing.T) {
	e := newSoftEnv(t)
	e.cfg.ThresholdTokens, e.cfg.SoftThresholdTokens = 400000, 300000
	e.w = e.newWatcher()
	e.s.usage["a"] = 354815
	e.s.events = []shuttleengine.Event{stop("a")}
	e.tick()
	if st := e.state(); st.LastContextTokens != 354815 {
		t.Fatalf("saved reading = %d, want 354815", st.LastContextTokens)
	}
	e.s.usage["a"] = 13673
	e.clock.advance(21 * time.Second)
	e.tick()
	e.assertNoCalls()
	if st := e.state(); st.Phase != PhaseIdle || st.LastContextTokens != 13673 || !st.LastContextKnown {
		t.Errorf("state = %+v, want idle with the re-read 13673", st)
	}
}

func TestWatcher_HardTriggerRereadBelowCapDoesNotFire(t *testing.T) {
	e := newSoftEnv(t)
	e.s.usage["a"] = 2000
	e.s.events = []shuttleengine.Event{stop("a")}
	e.tick()
	// Below the hard cap but at the soft threshold: no soft firing on the same tick.
	e.s.usage["a"] = 600
	e.clock.advance(21 * time.Second)
	e.tick()
	e.assertNoCalls()
	if st := e.state(); st.Phase != PhaseIdle || st.LastContextTokens != 600 {
		t.Errorf("state = %+v, want idle with reading 600", st)
	}
	// The next tick re-evaluates from the saved reading and fires soft.
	e.tick()
	if st := e.state(); st.Phase != PhaseHandoffRequested || st.CycleTrigger != TriggerSoft {
		t.Errorf("state = %+v, want a soft handoff request on the next tick", st)
	}
}

func TestWatcher_RequestedCycleFiresOnSmallReadingWithoutReread(t *testing.T) {
	e := newWatchEnv(t)
	e.s.usage["a"] = 10
	e.s.events = []shuttleengine.Event{stop("a")}
	e.tick()
	asks := len(e.s.tokenAsks)
	if err := RequestCycle(e.paths, CycleClear, e.clock.now); err != nil {
		t.Fatal(err)
	}
	e.clock.advance(11 * time.Second)
	e.tick()
	if st := e.state(); st.Phase != PhaseHandoffRequested || st.CycleTrigger != TriggerRequested {
		t.Fatalf("state = %+v, want a requested handoff", st)
	}
	if len(e.s.tokenAsks) != asks {
		t.Errorf("ContextTokens asked %d more times, want none", len(e.s.tokenAsks)-asks)
	}
}

// colorInjects counts the color command injects the fake creel accepted.
func (e *watchEnv) colorInjects() int {
	n := 0
	for _, inject := range e.c.injects {
		if inject.Step.Command == commandColor {
			n++
		}
	}
	return n
}

func TestWatcher_StartTypesTheColorOnTheFirstIdleTickOnly(t *testing.T) {
	t.Parallel()

	t.Run("queued once per launch and recorded", func(t *testing.T) {
		t.Parallel()
		e := newWatchEnv(t)
		e.tick()
		if e.s.colors != 1 || e.c.injects[0].Step.Argument != "colour208" || e.c.injects[0].Receiver != fakeOrchStrandName {
			t.Fatalf("colors = %d injects = %+v, want the strand's color queued for the orch at the first tick", e.s.colors, e.c.injects)
		}
		e.tick()
		e.tick()
		if e.s.colors != 1 {
			t.Fatalf("colors = %d, want the color queued once", e.s.colors)
		}
		e.assertNoCalls()

		// A watcher restarted on the same session, as a re-exec onto a new binary is, queues nothing.
		e.w = e.newWatcher()
		e.tick()
		if e.s.colors != 1 {
			t.Fatalf("colors = %d, want the restarted watcher to queue nothing", e.s.colors)
		}

		// A new launch on the strand queues it once again.
		e.setState(func(st *State) { st.LaunchedAt = st.LaunchedAt.Add(time.Minute) })
		e.w = e.newWatcher()
		e.tick()
		e.tick()
		e.tick()
		if e.s.colors != 2 {
			t.Fatalf("colors = %d, want the color queued once for the new launch", e.s.colors)
		}
	})
	t.Run("a strand with no color queues no color inject and counts as done", func(t *testing.T) {
		t.Parallel()
		e := newWatchEnv(t)
		e.s.color = ""
		e.tick()
		e.tick()
		if e.colorInjects() != 0 {
			t.Fatalf("color injects = %d, want none", e.colorInjects())
		}
		if st := e.state(); !colorTyped(st) {
			t.Errorf("state = %+v, want the color recorded as typed", st)
		}
	})
	t.Run("an expired bind color is logged and not queued again", func(t *testing.T) {
		t.Parallel()
		e := newWatchEnv(t)
		e.c.autoLand = false
		e.tick()
		e.setOutcome(e.lastInjectID(), CreelOutcome{State: CreelExpired, Stall: "the orch is busy"})
		e.tick()
		e.tick()
		e.w = e.newWatcher()
		if e.colorInjects() != 1 {
			t.Fatalf("color injects = %d, want the expired color dropped, not queued again", e.colorInjects())
		}
		if st := e.state(); colorTyped(st) {
			t.Errorf("state = %+v, want an expired color not recorded as typed", st)
		}
	})
	t.Run("a refused bind color is logged and not queued again", func(t *testing.T) {
		t.Parallel()
		e := newWatchEnv(t)
		e.c.failNext("inject", errBoom)
		e.tick()
		e.tick()
		e.tick()
		if e.colorInjects() != 0 || e.state().Stuck != "" {
			t.Fatalf("color injects = %d stuck = %q, want a display-only color dropped silently", e.colorInjects(), e.state().Stuck)
		}
	})
	t.Run("an unconfirmed bind color counts as typed", func(t *testing.T) {
		t.Parallel()
		e := newWatchEnv(t)
		e.c.autoLand = false
		e.tick()
		e.setOutcome(e.lastInjectID(), CreelOutcome{State: CreelUnconfirmed})
		e.tick()
		if st := e.state(); !colorTyped(st) {
			t.Errorf("state = %+v, want the unconfirmed color recorded as typed", st)
		}
	})
}

func TestWatcher_ReloadTypesTheColorFirstAndAFailingColorNeverWedgesIt(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// arrange makes the strand's color unavailable.
		arrange func(e *watchEnv)
		// wantColor is whether the reload queues a color inject.
		wantColor bool
	}{
		{"a strand with a color queues it first", func(*watchEnv) {}, true},
		{"a strand with no color moves to the plugins step with no color inject", func(e *watchEnv) { e.s.color = "" }, false},
		{"a failing color read is logged and moves to the plugins step", func(e *watchEnv) { e.s.colorErr = errBoom }, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			e := newWatchEnv(t)
			e.reachClearing()
			colors := e.colorInjects()
			tt.arrange(e)
			e.tick() // the clear typed: resuming
			if st := e.state(); st.Phase != PhaseResuming {
				t.Fatalf("state = %+v, want resuming", st)
			}
			if got := e.colorInjects() - colors; (got == 1) != tt.wantColor || got > 1 {
				t.Fatalf("color injects = %d, want a color step: %v", got, tt.wantColor)
			}
			got := e.callsAfter("clear")
			if tt.wantColor {
				if len(got) != 0 {
					t.Fatalf("calls after clear = %v, want the color alone on this tick", got)
				}
				e.tick()
				got = e.callsAfter("clear")
			}
			if !slices.Equal(got, []string{reloadPluginsCall}) {
				t.Fatalf("calls after clear = %v, want the plugins reload", got)
			}
			if st := e.state(); st.ReloadStep != ReloadStepPlugins {
				t.Errorf("state = %+v, want the plugins step", st)
			}
		})
	}
}

func TestWatcher_RestartMidColorStepTypesItOnceWhenIdle(t *testing.T) {
	t.Parallel()

	e := newWatchEnv(t)
	e.reachClearing()
	e.tick() // the clear typed, the color queued
	e.setState(func(st *State) {
		st.ReloadStep, st.ReloadTypedAt, st.ColorStrand, st.Pending = ReloadStepColor, time.Time{}, "", nil
	})
	before := e.s.colors
	e.w = e.newWatcher()
	e.tick()
	if e.s.colors != before+1 {
		t.Fatalf("colors queued = %d, want the color step queued once", e.s.colors-before)
	}
	e.tick()
	if st := e.state(); st.ReloadStep != ReloadStepPlugins {
		t.Errorf("state = %+v, want the move to the plugins step", st)
	}

	// The color step's typing counts as typed for the session, so a re-exec queues no start color.
	e.w = e.newWatcher()
	e.tick()
	if e.s.colors != before+1 {
		t.Errorf("colors queued = %d, want no start color after the color step", e.s.colors-before)
	}
}

func TestWatcher_NoteReplyGate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// arrange leaves the reply, and the note, in the state the row gates.
		arrange func(e *watchEnv, path string)
	}{
		{"a note written with no reply posted", func(e *watchEnv, path string) { e.writeHandoff() }},
		{"a reply naming another path", func(e *watchEnv, path string) {
			e.writeHandoff()
			e.reply("written /somewhere/else.md")
		}},
		{"a reply carrying other text", func(e *watchEnv, path string) {
			e.writeHandoff()
			e.reply("done with " + path)
		}},
		{"a reply with no note file", func(e *watchEnv, path string) { e.reply("written " + path) }},
		{"a reply with an empty note file", func(e *watchEnv, path string) {
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				e.t.Fatal(err)
			}
			if err := os.WriteFile(path, nil, 0o644); err != nil {
				e.t.Fatal(err)
			}
			e.reply("written " + path)
		}},
		{"a reply to an earlier request id", func(e *watchEnv, path string) {
			e.writeHandoff()
			e.c.mailbox = append(e.c.mailbox, CreelMessage{ID: 50, From: fakeOrchStrandName, ReplyTo: e.state().RequestID - 1, Body: "written " + path})
		}},
		{"a reply from another sender", func(e *watchEnv, path string) {
			e.writeHandoff()
			e.c.mailbox = append(e.c.mailbox, CreelMessage{ID: 51, From: "tst:task:driver", ReplyTo: e.state().RequestID, Body: "written " + path})
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			e := newWatchEnv(t)
			e.injectHandoff()
			tt.arrange(e, e.state().PendingHandoff)
			e.tick()
			if st := e.state(); st.Phase != PhaseHandoffRequested || e.s.count("clear") != 0 {
				t.Fatalf("phase = %s calls = %v, want the gate closed", st.Phase, e.s.calls)
			}
			if len(e.c.mailbox) != 0 {
				t.Errorf("mailbox = %+v, want every message read", e.c.mailbox)
			}
		})
	}

	t.Run("a reply that passes the gate moves on to clearing", func(t *testing.T) {
		t.Parallel()
		e := newWatchEnv(t)
		e.injectHandoff()
		e.finishNote()
		if st := e.state(); st.Phase != PhaseClearing || e.s.count("clear") != 1 {
			t.Fatalf("phase = %s calls = %v", st.Phase, e.s.calls)
		}
	})

	t.Run("a save that fails after the reply is read leaves it unread, and a new watcher gates on it", func(t *testing.T) {
		t.Parallel()
		if runtime.GOOS == "windows" || os.Geteuid() == 0 {
			t.Skip("a read-only directory stops no write here, so a failing save cannot be arranged")
		}
		e := newWatchEnv(t)
		e.injectHandoff()
		path := e.writeHandoff()
		e.reply("written " + path)
		// The directory turns unwritable as the handler is handed the reply, so its save fails.
		e.c.onReadMailbox = func() {
			if err := os.Chmod(e.paths.Dir, 0o500); err != nil {
				t.Error(err)
			}
		}
		t.Cleanup(func() { _ = os.Chmod(e.paths.Dir, 0o755) })
		e.tickErr()
		if len(e.c.mailbox) != 1 {
			t.Fatalf("mailbox = %+v, want the reply left unread", e.c.mailbox)
		}
		if err := os.Chmod(e.paths.Dir, 0o755); err != nil {
			t.Fatal(err)
		}
		e.c.onReadMailbox = nil
		e.w = e.newWatcher()
		e.tick()
		if st := e.state(); st.Phase != PhaseClearing {
			t.Fatalf("phase = %s, want the new watcher to gate on the unread reply", st.Phase)
		}
	})
}

func TestWatcher_StepOutcomes(t *testing.T) {
	t.Parallel()

	t.Run("an expired step inject is queued again and its stall shows until the step advances", func(t *testing.T) {
		t.Parallel()
		e := newWatchEnv(t)
		e.reachClearing()
		e.setOutcome(e.lastInjectID(), CreelOutcome{State: CreelExpired, Stall: "the input box holds a draft"})
		e.tick()
		if st := e.state(); st.Phase != PhaseClearing || st.Stuck != "the input box holds a draft" || st.StuckByCreel || e.s.count("clear") != 2 {
			t.Fatalf("state = %+v calls = %v, want the clear queued again with the stall shown", st, e.s.calls)
		}
		e.tick()
		if st := e.state(); st.Phase != PhaseResuming || st.Stuck != "" {
			t.Fatalf("state = %+v, want the step advanced with the stall cleared", st)
		}
	})

	t.Run("an unconfirmed color or plugins inject advances the reload", func(t *testing.T) {
		t.Parallel()
		e := newWatchEnv(t)
		e.reachClearing()
		e.tick() // the clear typed, the color queued
		e.setOutcome(e.lastInjectID(), CreelOutcome{State: CreelUnconfirmed})
		e.tick()
		if st := e.state(); st.ReloadStep != ReloadStepPlugins {
			t.Fatalf("state = %+v, want the plugins step after an unconfirmed color", st)
		}
		e.setOutcome(e.lastInjectID(), CreelOutcome{State: CreelUnconfirmed})
		e.tick()
		if st := e.state(); st.Phase != PhaseResuming || e.reloadStepOf(st) != ReloadStepPointer {
			t.Fatalf("state = %+v, want the pointer step after an unconfirmed plugins reload", st)
		}
	})

	t.Run("an unconfirmed clear moves to resuming", func(t *testing.T) {
		t.Parallel()
		e := newWatchEnv(t)
		e.reachClearing()
		e.setOutcome(e.lastInjectID(), CreelOutcome{State: CreelUnconfirmed})
		e.tick()
		if st := e.state(); st.Phase != PhaseResuming || st.CycleCount != 1 {
			t.Fatalf("state = %+v", st)
		}
	})

	t.Run("a plugins inject that keeps expiring is skipped at the timeout from its first queueing", func(t *testing.T) {
		t.Parallel()
		e := newWatchEnv(t)
		e.reachClearing()
		e.tick() // the clear typed, the color queued
		e.tick() // the color typed, the plugins queued
		if st := e.state(); st.ReloadStep != ReloadStepPlugins {
			t.Fatalf("state = %+v, want the plugins step", st)
		}
		e.c.autoLand = false
		for range 3 {
			e.setOutcome(e.lastInjectID(), CreelOutcome{State: CreelExpired, Stall: "the orch is busy"})
			e.clock.advance(30 * time.Second)
			e.tick()
			if st := e.state(); st.ReloadStep != ReloadStepPlugins || st.Stuck != "the orch is busy" {
				t.Fatalf("state = %+v, want the plugins step still queued with the stall shown", st)
			}
		}
		e.clock.advance(11 * time.Second)
		e.tick()
		if st := e.state(); st.Phase != PhaseResuming || e.reloadStepOf(st) != ReloadStepPointer || st.Stuck != "" {
			t.Fatalf("state = %+v, want the plugins step skipped at its timeout with the stall cleared", st)
		}
	})

	t.Run("a withdrawn inject returns to idle and releases the hold", func(t *testing.T) {
		t.Parallel()
		e := newWatchEnv(t)
		e.reachClearing()
		e.setOutcome(e.lastInjectID(), CreelOutcome{State: CreelWithdrawnState})
		injects := len(e.c.injects)
		e.tick()
		if st := e.state(); st.Phase != PhaseIdle || e.c.count("release:") != 1 || len(e.c.injects) != injects {
			t.Fatalf("state = %+v creel calls = %v, want idle, one release and nothing queued again", st, e.c.calls)
		}
	})

	t.Run("a stale reason left in an idle state file is cleared by the first tick", func(t *testing.T) {
		t.Parallel()
		e := newWatchEnv(t)
		raw, err := json.Marshal(State{Strand: "s1", Phase: PhaseIdle, Stuck: "a turn is running"})
		if err != nil {
			t.Fatal(err)
		}
		var fields map[string]any
		if err := json.Unmarshal(raw, &fields); err != nil {
			t.Fatal(err)
		}
		fields["stuck_by_hold"] = true
		raw, err = json.Marshal(fields)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(e.paths.StatePath, raw, 0o644); err != nil {
			t.Fatal(err)
		}
		e.tick()
		if st := e.state(); st.Stuck != "" || st.StuckByCreel {
			t.Fatalf("state = %+v, want the stale reason cleared", st)
		}
	})
}

func TestWatcher_CycleHold(t *testing.T) {
	t.Parallel()

	t.Run("a cycle spanning a hub pause keeps renewing its hold", func(t *testing.T) {
		t.Parallel()
		e := newWatchEnv(t)
		e.injectHandoff()
		e.pauseHub("EXTERNAL", pauseGrace)
		holds, calls := e.c.count("hold:"), len(e.s.calls)
		for range 3 {
			e.tick()
		}
		if got := e.c.count("hold:"); got != holds+3 {
			t.Fatalf("holds = %d, want the hold renewed on each of the three held ticks (%d)", got, holds+3)
		}
		if len(e.s.calls) != calls || e.c.count("release:") != 0 {
			t.Errorf("calls = %v releases = %d, want the cycle held without a step or a release", e.s.calls, e.c.count("release:"))
		}
	})

	t.Run("a refused hold starts no cycle and is tried again", func(t *testing.T) {
		t.Parallel()
		e := newWatchEnv(t)
		e.c.failNext("hold", errBoom)
		e.s.usage["a"] = 2000
		e.s.events = []shuttleengine.Event{stop("a")}
		e.tick()
		e.clock.advance(11 * time.Second)
		e.tick()
		if st := e.state(); st.Phase != PhaseIdle || len(e.c.posts) != 0 || st.Stuck != errBoom.Error() {
			t.Fatalf("state = %+v posts = %v, want no cycle started and the refusal shown", st, e.c.posts)
		}
		e.tick()
		if st := e.state(); st.Phase != PhaseHandoffRequested || len(e.c.posts) != 1 || st.Stuck != "" {
			t.Fatalf("state = %+v posts = %v, want the cycle started on the retry", st, e.c.posts)
		}
	})

	t.Run("a state reset to idle on another strand before the step is queued queues nothing", func(t *testing.T) {
		t.Parallel()
		e := newWatchEnv(t)
		e.reachClearing()
		e.setState(func(st *State) { st.Pending = nil })
		injects := len(e.c.injects)
		e.s.onReadEvents = func() {
			if err := SaveState(e.paths, State{Strand: "s2", Phase: PhaseIdle}); err != nil {
				t.Error(err)
			}
		}
		e.tick()
		if len(e.c.injects) != injects || len(e.c.posts) != 1 {
			t.Errorf("injects = %v posts = %v, want nothing queued", e.c.injects, e.c.posts)
		}
		if st := e.state(); st.Strand != "s2" || st.Phase != PhaseIdle || st.Pending != nil {
			t.Errorf("state = %+v, want the reset kept as it was written", st)
		}
	})
}

// TestWatcher_CreelFailuresHoldTheCycle does not call t.Parallel: it asserts on the logger output,
// which is process-global.
func TestWatcher_CreelFailuresHoldTheCycle(t *testing.T) {
	t.Run("a failing call holds the cycle, logs once per change and never counts toward the tick-error cap", func(t *testing.T) {
		buf := logcapture.Capture(t)
		e := newWatchEnv(t)
		e.injectHandoff()
		for range maxConsecutiveTickErrors + 5 {
			e.c.failNext("hold", errBoom)
			e.tick()
		}
		if st := e.state(); st.Phase != PhaseHandoffRequested || st.Stuck != errBoom.Error() || !st.StuckByCreel {
			t.Fatalf("state = %+v, want the cycle held with the failure shown", st)
		}
		if got := strings.Count(buf.String(), "a creel call failed"); got != 1 {
			t.Errorf("failure logged %d times for one error, want once; log:\n%s", got, buf.String())
		}
		e.c.failNext("hold", errors.New("creel: the daemon is gone"))
		e.tick()
		if got := strings.Count(buf.String(), "a creel call failed"); got != 2 {
			t.Errorf("failure logged %d times after the error changed, want 2", got)
		}
	})

	t.Run("a sender-resolution refusal of the hold shows in Stuck", func(t *testing.T) {
		e := newWatchEnv(t)
		e.injectHandoff()
		refusal := errors.New("creel: the sender cannot be resolved; run it from the orch's strand")
		e.c.failNext("hold", refusal)
		e.tick()
		if st := e.state(); st.Stuck != refusal.Error() || !st.StuckByCreel {
			t.Fatalf("state = %+v, want the refusal shown", st)
		}
	})

	t.Run("a release that fails at the return to idle still returns to idle, and the next good tick clears it", func(t *testing.T) {
		e := newWatchEnv(t)
		e.injectHandoff()
		e.clock.advance(101 * time.Second)
		e.c.failNext("release", errBoom)
		e.tick()
		if st := e.state(); st.Phase != PhaseIdle || st.Stuck != errBoom.Error() || !st.StuckByCreel {
			t.Fatalf("state = %+v, want idle with the failed release recorded", st)
		}
		e.tick()
		if st := e.state(); st.Stuck != "" || st.StuckByCreel {
			t.Fatalf("state = %+v, want the failure cleared by a tick whose creel calls succeeded", st)
		}
	})

	t.Run("a recorded fresh-launch failure is retried on every tick, holding back every post, inject and hold until both calls succeed", func(t *testing.T) {
		e := newWatchEnv(t)
		e.setState(func(s *State) { s.FreshLaunchFailure = "withdraw after the fresh launch: boom" })
		releaseErr := errors.New("creel: release refused")

		e.c.failNext("withdraw", errBoom)
		e.tick()
		st := e.state()
		if st.FreshLaunchFailure == "" || st.Stuck != errBoom.Error() || st.StuckByCreel {
			t.Fatalf("state = %+v, want the failure kept and shown in Stuck without StuckByCreel", st)
		}
		e.c.failNext("release", releaseErr)
		e.tick()
		st = e.state()
		if st.FreshLaunchFailure == "" || st.Stuck != releaseErr.Error() || st.StuckByCreel {
			t.Fatalf("state = %+v, want the failure kept and rewritten with the release error", st)
		}
		if len(e.c.injects) != 0 || len(e.c.posts) != 0 || e.c.count("hold:") != 0 {
			t.Fatalf("injects = %+v posts = %+v calls = %v, want nothing posted, queued or held while the failure stands", e.c.injects, e.c.posts, e.c.calls)
		}

		e.tick()
		st = e.state()
		if st.FreshLaunchFailure != "" || st.Stuck != "" || st.StuckByCreel {
			t.Fatalf("state = %+v, want the failure and Stuck cleared once both calls succeeded", st)
		}
		if e.c.count("withdraw:") != 3 || e.c.count("release:") != 3 {
			t.Errorf("creel calls = %v, want a withdraw and a release on each of three ticks", e.c.calls)
		}
		if got := e.colorInjects(); got != 1 {
			t.Errorf("color injects = %d, want the bind color queued once the retry succeeded", got)
		}
		lastWithdraw := -1
		for i, call := range e.c.calls {
			if call == "withdraw:"+fakeOrchStrandName {
				lastWithdraw = i
			}
		}
		firstInject := slices.IndexFunc(e.c.calls, func(call string) bool { return strings.HasPrefix(call, "inject:") })
		if firstInject != -1 && firstInject < lastWithdraw {
			t.Errorf("creel calls = %v, want no inject before the retry's withdraw succeeded", e.c.calls)
		}
	})

	t.Run("a retry whose withdraw removes a resume line clears that entry's noticed mark", func(t *testing.T) {
		e := newWatchEnv(t)
		e.noticedResumedEntry()
		e.setState(func(s *State) { s.FreshLaunchFailure = "release after the fresh launch: boom" })
		e.c.withdrawn = []CreelWithdrawn{{ID: 5, Subject: pauserecord.ResumeLine}}
		e.tick()
		if st := e.state(); st.FreshLaunchFailure != "" {
			t.Fatalf("state = %+v, want the failure cleared", st)
		}
		if bodies := e.c.postBodies(fakeOrchStrandName); !slices.Equal(bodies, []string{pauserecord.ResumeLine}) {
			t.Errorf("posts to the orch = %v, want the withdrawn resume line posted again", bodies)
		}
	})

	t.Run("a read acknowledgement that fails after the handler moved the phase keeps the phase and acts on the reply once", func(t *testing.T) {
		e := newWatchEnv(t)
		e.injectHandoff()
		path := e.writeHandoff()
		e.reply("written " + path)
		e.c.failNext("acknowledge", errBoom)
		e.tick()
		st := e.state()
		if st.Phase != PhaseClearing || st.Stuck != errBoom.Error() || !st.StuckByCreel {
			t.Fatalf("state = %+v, want the phase the handler moved to kept with the failure shown", st)
		}
		e.tick()
		e.tick()
		clears := 0
		for _, inject := range e.c.injects {
			if inject.Step.Command == commandClear {
				clears++
			}
		}
		if clears != 1 {
			t.Errorf("injects = %+v, want the clear injected once", e.c.injects)
		}
		if st := e.state(); st.Stuck != "" || st.StuckByCreel {
			t.Errorf("state = %+v, want the failure cleared once the read was acknowledged", st)
		}
	})

	t.Run("a withdraw that removes a resume line whose mark clear fails is retried until the mark clears", func(t *testing.T) {
		e := newWatchEnv(t)
		e.noticedResumedEntry()
		e.setState(func(s *State) { s.FreshLaunchFailure = "release after the fresh launch: boom" })
		e.c.withdrawn = []CreelWithdrawn{{ID: 5, Subject: pauserecord.ResumeLine}}
		lock := e.pausePath + ".lock"
		if err := os.Remove(lock); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(lock, 0o755); err != nil {
			t.Fatal(err)
		}
		e.tick()
		st := e.state()
		if st.FreshLaunchFailure == "" || st.Stuck == "" || st.StuckByCreel || !st.ResumeLineWithdrawn {
			t.Fatalf("state = %+v, want the failure kept, shown in Stuck and the withdrawn line recorded", st)
		}
		if err := os.Remove(lock); err != nil {
			t.Fatal(err)
		}
		e.c.withdrawn = nil
		e.tick()
		if st := e.state(); st.FreshLaunchFailure != "" || st.ResumeLineWithdrawn {
			t.Fatalf("state = %+v, want the failure cleared once the mark clear succeeded", st)
		}
		if bodies := e.c.postBodies(fakeOrchStrandName); !slices.Equal(bodies, []string{pauserecord.ResumeLine}) {
			t.Errorf("posts to the orch = %v, want the withdrawn resume line posted again", bodies)
		}
	})

	t.Run("an inject creel journaled but answered with an error is adopted on the next tick", func(t *testing.T) {
		e := newWatchEnv(t)
		e.injectHandoff()
		path := e.writeHandoff()
		e.reply("written " + path)
		e.c.journalFails = []error{errBoom}
		e.tick()
		st := e.state()
		if st.Phase != PhaseClearing || st.Pending == nil || st.Pending.ID != 0 || st.Stuck != errBoom.Error() {
			t.Fatalf("state = %+v, want the clear pending with no id", st)
		}
		if len(e.c.sent) != 1 {
			t.Fatalf("sent = %+v, want the journaled clear", e.c.sent)
		}
		e.tick()
		st = e.state()
		if st.Pending == nil || st.Pending.ID != e.c.sent[0].ID || st.Stuck != "" {
			t.Fatalf("state = %+v, want the journaled clear adopted", st)
		}
		if len(e.c.injects) != 1 || e.s.count("clear") != 0 {
			t.Errorf("injects = %+v calls = %v, want nothing injected beside the bind color", e.c.injects, e.s.calls)
		}
	})
}

func TestWatcher_TurnEndCountsOnlyAfterSending(t *testing.T) {
	t.Parallel()

	steps := []int{ReloadStepSkills, ReloadStepRetry, ReloadStepPointer}
	for _, step := range steps {
		t.Run(reloadStepName(step), func(t *testing.T) {
			t.Parallel()
			e := newWatchEnv(t)
			e.withSkills()
			e.reachReloadStep(step)
			id := e.lastInjectID()
			sending := e.clock.now.Add(10 * time.Second)
			// Not yet sent: an operator turn ending between the queueing and the sending confirms nothing.
			e.setOutcome(id, CreelOutcome{State: CreelQueued})
			e.s.events = append(e.s.events, shuttleengine.Event{Kind: shuttleengine.EventStop, Message: "operator", At: e.clock.now.Add(5 * time.Second)})
			e.tick()
			e.setOutcome(id, CreelOutcome{State: CreelInjected, SendingAt: sending})
			e.tick()
			// A turn end stamped in the second of the sending record, and one with no time, confirm nothing.
			e.s.events = append(e.s.events,
				shuttleengine.Event{Kind: shuttleengine.EventStop, Message: "same second", At: sending.Add(500 * time.Millisecond)},
				shuttleengine.Event{Kind: shuttleengine.EventStop, Message: "no time"})
			e.tick()
			st := e.state()
			if st.Phase != PhaseResuming || e.reloadStepOf(st) != step {
				t.Fatalf("state = %+v, want the %s step still waiting", st, reloadStepName(step))
			}
			// A turn end a second later confirms it.
			e.s.events = append(e.s.events, shuttleengine.Event{Kind: shuttleengine.EventStop, Message: "answer", At: sending.Add(time.Second)})
			e.tick()
			st = e.state()
			if step == ReloadStepPointer {
				if st.Phase != PhaseIdle {
					t.Fatalf("state = %+v, want idle after the pointer's turn end", st)
				}
				return
			}
			if e.reloadStepOf(st) == step && st.Pending != nil && st.Pending.ID == id {
				t.Fatalf("state = %+v, want the %s step settled by the turn end after its sending", st, reloadStepName(step))
			}
		})
	}
}
