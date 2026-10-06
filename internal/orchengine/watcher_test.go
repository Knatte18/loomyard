package orchengine

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/stencilstore"
)

type fakeClock struct{ now time.Time }

func (c *fakeClock) Now() time.Time          { return c.now }
func (c *fakeClock) advance(d time.Duration) { c.now = c.now.Add(d) }

// fakeSession scripts the Session seam; event offsets are indexes into events.
type fakeSession struct {
	alive      bool
	events     []shuttleengine.Event
	usage      map[string]int       // Turn-end message to tokens; a missing message is unknown.
	boundary   map[string]time.Time // Turn-end message to a compaction boundary read through it; the boundary's tokens are usage's.
	idle       bool
	idleSeq    []bool // Consumed before idle.
	tooShort   bool   // Reported with every probe that is not idle.
	sendErrs   []error
	clearErr   error // Returned by every ClearSession while set.
	clearErrs  []error
	compactErr error    // Returned by every CompactSession while set.
	calls      []string // "send:<text>", "clear" and "compact:<focus>", in order.
	tokenAsks  []string
	onSend     func()
	onAlive    func()

	skillUnknown map[string]bool      // Skill to whether SkillUnknown reports it.
	autoCompact  map[string]time.Time // Turn-end message to a main-chain compaction boundary CompactedSince finds through it.
}

func (f *fakeSession) LoadSkill(_, skill string) error {
	f.calls = append(f.calls, "skill:"+skill)
	return nil
}

func (f *fakeSession) SkillUnknown(_, skill string) (bool, error) {
	return f.skillUnknown[skill], nil
}

func (f *fakeSession) CompactedSince(turnEnd shuttleengine.Event, since time.Time) (time.Time, bool, error) {
	at, ok := f.autoCompact[turnEnd.Message]
	return at, ok && at.After(since), nil
}

func (f *fakeSession) StrandAlive(string) (bool, error) {
	if f.onAlive != nil {
		f.onAlive()
	}
	return f.alive, nil
}

func (f *fakeSession) ReadEvents(_ string, offset int64) ([]shuttleengine.Event, int64, error) {
	if offset > int64(len(f.events)) {
		offset = int64(len(f.events))
	}
	return append([]shuttleengine.Event(nil), f.events[offset:]...), int64(len(f.events)), nil
}

func (f *fakeSession) ContextTokens(ev shuttleengine.Event) (shuttleengine.ContextReading, error) {
	f.tokenAsks = append(f.tokenAsks, ev.Message)
	n, ok := f.usage[ev.Message]
	at, compacted := f.boundary[ev.Message]
	return shuttleengine.ContextReading{Tokens: n, Known: ok, Compacted: compacted, BoundaryAt: at}, nil
}

func (f *fakeSession) SessionIdle(string) (shuttleengine.IdleProbe, error) {
	idle := f.idle
	if len(f.idleSeq) > 0 {
		idle = f.idleSeq[0]
		f.idleSeq = f.idleSeq[1:]
	}
	return shuttleengine.IdleProbe{Idle: idle, TooShort: !idle && f.tooShort}, nil
}

func (f *fakeSession) Send(_, text string) error {
	f.calls = append(f.calls, "send:"+text)
	if f.onSend != nil {
		f.onSend()
	}
	if len(f.sendErrs) > 0 {
		err := f.sendErrs[0]
		f.sendErrs = f.sendErrs[1:]
		return err
	}
	return nil
}

func (f *fakeSession) ClearSession(string) error {
	f.calls = append(f.calls, "clear")
	if f.clearErr != nil {
		return f.clearErr
	}
	if len(f.clearErrs) > 0 {
		err := f.clearErrs[0]
		f.clearErrs = f.clearErrs[1:]
		return err
	}
	return nil
}

func (f *fakeSession) CompactSession(_, focus string) error {
	f.calls = append(f.calls, "compact:"+focus)
	if f.compactErr != nil {
		return f.compactErr
	}
	return nil
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

type watchEnv struct {
	t     *testing.T
	w     *Watcher
	s     *fakeSession
	clock *fakeClock
	paths Paths
	stDir string
	cfg   Config

	skills []string // The skill list the watcher reloads, set before newWatcher.
}

func stop(msg string) shuttleengine.Event {
	return shuttleengine.Event{Kind: shuttleengine.EventStop, Message: msg}
}

func ask(msg string) shuttleengine.Event {
	return shuttleengine.Event{Kind: shuttleengine.EventAsk, Message: msg}
}

// newWatchEnv builds an idle orch state on strand s1 with threshold 1000, grace 10s, timeout 100s.
func newWatchEnv(t *testing.T) *watchEnv {
	t.Helper()
	e := &watchEnv{
		t:     t,
		paths: testPaths(t),
		stDir: seedStencils(t),
		s:     &fakeSession{alive: true, idle: true, usage: map[string]int{}},
		cfg:   Config{ThresholdTokens: 1000, IdleGraceS: 10, HandoffTimeoutS: 100, CycleMode: CycleClear},
		clock: &fakeClock{now: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)},
	}
	e.paths.NoticesDir = filepath.Join(e.paths.Dir, "notices")
	if err := SaveState(e.paths, State{Strand: "s1", Phase: PhaseIdle}); err != nil {
		t.Fatal(err)
	}
	e.w = e.newWatcher()
	return e
}

// queue queues line as a notice, failing the test when it is not queued.
func (e *watchEnv) queue(line string) {
	e.t.Helper()
	e.clock.advance(time.Millisecond)
	if queued, err := QueueNotice(e.paths, line, e.clock.now); err != nil || !queued {
		e.t.Fatalf("QueueNotice(%q) = %v, %v", line, queued, err)
	}
}

func (e *watchEnv) noticeLines() []string {
	e.t.Helper()
	ns, err := ListNotices(e.paths)
	if err != nil {
		e.t.Fatal(err)
	}
	var lines []string
	for _, n := range ns {
		lines = append(lines, n.Line)
	}
	return lines
}

func (e *watchEnv) newWatcher() *Watcher {
	return NewWatcher(e.s, e.cfg, e.paths, e.stDir, e.skills, e.clock)
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

// injectHandoff makes one over-threshold turn end and ticks until the handoff instruction is sent.
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

// reachClearing completes the handoff so the state is clearing with the clear sent.
func (e *watchEnv) reachClearing() {
	e.t.Helper()
	e.injectHandoff()
	e.writeHandoff()
	e.s.events = append(e.s.events, stop("handoff"))
	e.tick()
	if st := e.state(); st.Phase != PhaseClearing {
		e.t.Fatalf("phase = %s, want clearing", st.Phase)
	}
}

// reachResuming continues from reachClearing until the resume prompt is sent.
func (e *watchEnv) reachResuming() {
	e.t.Helper()
	e.reachClearing()
	e.tick()
	if st := e.state(); st.Phase != PhaseResuming {
		e.t.Fatalf("phase = %s, want resuming", st.Phase)
	}
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
	var phaseAtSend Phase
	e.s.onSend = func() { phaseAtSend = e.state().Phase }
	e.injectHandoff()
	if phaseAtSend != PhaseHandoffRequested {
		t.Errorf("phase at send = %s, want handoff-requested", phaseAtSend)
	}
	st := e.state()
	if len(e.s.calls) != 1 || !strings.Contains(e.s.calls[0], st.PendingHandoff) {
		t.Fatalf("calls = %v, pending %q", e.s.calls, st.PendingHandoff)
	}
	if !st.PhaseInjected || st.PhaseStrand != "s1" || st.PhaseEventsOffset != 1 {
		t.Errorf("state = %+v", st)
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

func TestWatcher_IdleProbeFalseThenPasses(t *testing.T) {
	e := newWatchEnv(t)
	e.s.idle = false
	e.s.usage["a"] = 2000
	e.s.events = []shuttleengine.Event{stop("a")}
	e.tick()
	e.clock.advance(11 * time.Second)
	e.tick()
	e.assertNoCalls()
	e.s.idle = true
	e.tick()
	if e.s.count("send:") != 1 {
		t.Fatalf("calls = %v", e.s.calls)
	}
}

func TestWatcher_TooShortProbeRecordsStuckInIdlePhaseAndPassingProbeClearsIt(t *testing.T) {
	e := newWatchEnv(t)
	e.s.idle, e.s.tooShort = false, true
	e.s.usage["a"] = 2000
	e.s.events = []shuttleengine.Event{stop("a")}
	e.tick()
	e.clock.advance(11 * time.Second)
	e.tick()
	e.assertNoCalls()
	st := e.state()
	if st.Phase != PhaseIdle || st.Stuck != paneTooShortReason {
		t.Fatalf("state = %+v, want idle stuck on the too-short reason", st)
	}

	e.s.tooShort = false
	e.tick()
	if st := e.state(); st.Stuck != "" {
		t.Errorf("Stuck = %q after a probe that is not too short, want cleared", st.Stuck)
	}
	e.assertNoCalls()

	e.s.idle = true
	e.tick()
	if e.s.count("send:") != 1 {
		t.Errorf("calls = %v, want the handoff request once the pane is idle", e.s.calls)
	}
}

func TestWatcher_PassingProbeLeavesOtherStuckReasonsAlone(t *testing.T) {
	e := newWatchEnv(t)
	if err := SaveState(e.paths, State{Strand: "s1", Phase: PhaseIdle, Stuck: "something else"}); err != nil {
		t.Fatal(err)
	}
	e.s.idle = false
	e.s.usage["a"] = 2000
	e.s.events = []shuttleengine.Event{stop("a")}
	e.tick()
	e.clock.advance(11 * time.Second)
	e.tick()
	if got := e.state().Stuck; got != "something else" {
		t.Errorf("Stuck = %q, want an unrelated reason untouched by the probe", got)
	}
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

func TestWatcher_ManualRequestBelowThresholdCycles(t *testing.T) {
	e := newWatchEnv(t)
	e.s.usage["a"] = 10
	e.s.events = []shuttleengine.Event{stop("a")}
	if err := RequestCycle(e.paths, CycleClear, e.clock.now); err != nil {
		t.Fatal(err)
	}
	e.tick()
	e.clock.advance(11 * time.Second)
	e.tick()
	if e.state().Phase != PhaseHandoffRequested {
		t.Fatalf("phase = %s", e.state().Phase)
	}
	if _, requested, _ := CycleRequested(e.paths); requested {
		t.Error("cycle request should be cleared")
	}
}

func TestWatcher_HandoffFileWithoutTurnEndNoClear(t *testing.T) {
	e := newWatchEnv(t)
	e.injectHandoff()
	e.writeHandoff()
	e.tick()
	if e.s.count("clear") != 0 || e.state().Phase != PhaseHandoffRequested {
		t.Fatalf("calls = %v phase = %s", e.s.calls, e.state().Phase)
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
	e := newWatchEnv(t)
	e.reachClearing()
	handoff := e.state().PendingHandoff
	if e.s.count("clear") != 1 {
		t.Fatalf("calls = %v", e.s.calls)
	}
	e.tick()
	st := e.state()
	if st.Phase != PhaseResuming || st.CycleCount != 1 || st.LastHandoff != handoff {
		t.Fatalf("state = %+v", st)
	}
	last := e.s.calls[len(e.s.calls)-1]
	if !strings.HasPrefix(last, "send:") || !strings.Contains(last, handoff) {
		t.Errorf("resume prompt = %q, want one naming %s", last, handoff)
	}
}

func TestWatcher_ClearingTimeoutNeverTypesWhileBusy(t *testing.T) {
	e := newWatchEnv(t)
	e.reachClearing()
	e.s.idle = false
	e.tick()
	if e.state().Phase != PhaseClearing {
		t.Fatalf("phase = %s", e.state().Phase)
	}
	e.clock.advance(101 * time.Second)
	e.tick()
	e.tick()
	st := e.state()
	if st.Phase != PhaseClearing || st.Stuck == "" || e.s.count("send:") != 1 || e.s.count("clear") != 1 {
		t.Fatalf("typed into a busy pane or stuck unrecorded: state = %+v calls = %v", st, e.s.calls)
	}
	e.s.idle = true
	e.tick()
	st = e.state()
	if st.Phase != PhaseResuming || st.CycleCount != 1 || st.Stuck != "" || e.s.count("send:") != 2 {
		t.Fatalf("state = %+v calls = %v", st, e.s.calls)
	}
}

func TestWatcher_HandoffGateWaitsForIdleBeforeClear(t *testing.T) {
	e := newWatchEnv(t)
	e.injectHandoff()
	e.writeHandoff()
	e.s.events = append(e.s.events, stop("handoff"))
	e.s.idle = false
	e.tick()
	e.tick()
	if e.s.count("clear") != 0 || e.state().Phase != PhaseHandoffRequested {
		t.Fatalf("cleared a busy pane: calls = %v phase = %s", e.s.calls, e.state().Phase)
	}
	e.s.idle = true
	e.tick()
	if e.s.count("clear") != 1 || e.state().Phase != PhaseClearing {
		t.Fatalf("calls = %v phase = %s", e.s.calls, e.state().Phase)
	}
}

func TestWatcher_TurnEndBeforeHandoffFileDoesNotOpenClearGate(t *testing.T) {
	e := newWatchEnv(t)
	e.injectHandoff()
	e.s.events = append(e.s.events, stop("before the file"))
	e.tick()
	e.writeHandoff()
	e.tick()
	e.tick()
	if e.s.count("clear") != 0 || e.state().Phase != PhaseHandoffRequested {
		t.Fatalf("an earlier turn end opened the gate: calls = %v phase = %s", e.s.calls, e.state().Phase)
	}
	e.s.events = append(e.s.events, stop("after the file"))
	e.tick()
	if e.s.count("clear") != 1 || e.state().Phase != PhaseClearing {
		t.Fatalf("calls = %v phase = %s", e.s.calls, e.state().Phase)
	}
}

func TestWatcher_RestartReplayedTurnEndDoesNotOpenClearGate(t *testing.T) {
	e := newWatchEnv(t)
	e.injectHandoff()
	e.writeHandoff()
	e.s.events = append(e.s.events, stop("handoff"))
	e.w = e.newWatcher()
	e.tick()
	e.tick()
	if e.s.count("clear") != 0 {
		t.Fatalf("a replayed turn end opened the gate: %v", e.s.calls)
	}
	e.s.events = append(e.s.events, stop("later"))
	e.tick()
	if e.s.count("clear") != 1 || e.state().Phase != PhaseClearing {
		t.Fatalf("calls = %v phase = %s", e.s.calls, e.state().Phase)
	}
}

func TestWatcher_ClearRetryWaitsForIdle(t *testing.T) {
	e := newWatchEnv(t)
	e.injectHandoff()
	e.writeHandoff()
	e.s.events = append(e.s.events, stop("handoff"))
	e.s.clearErrs = []error{errBoom}
	e.tickErr()
	e.s.idle = false
	e.tick()
	e.clock.advance(101 * time.Second)
	e.tick()
	if e.s.count("clear") != 1 || e.s.count("send:") != 1 {
		t.Fatalf("typed into a busy pane: %v", e.s.calls)
	}
	if e.state().Stuck == "" {
		t.Error("an overdue clearing phase should record Stuck")
	}
}

func TestWatcher_ResumeTurnNeverEndingReturnsIdleUnknown(t *testing.T) {
	e := newWatchEnv(t)
	e.reachResuming()
	e.clock.advance(101 * time.Second)
	e.tick()
	st := e.state()
	if st.Phase != PhaseIdle || st.LastAbortReason == "" || st.LastContextKnown {
		t.Fatalf("state = %+v", st)
	}
}

func TestWatcher_ResumeTurnEndTakesFirstPostCycleReading(t *testing.T) {
	e := newWatchEnv(t)
	e.reachResuming()
	e.s.usage["resumed"] = 300
	e.s.events = append(e.s.events, stop("resumed"))
	e.tick()
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
	e.s.events = append(e.s.events, stop("pre"))
	e.tick() // clearing: probe passes, moves to resuming past the stray turn end.
	if e.state().Phase != PhaseResuming {
		t.Fatalf("phase = %s", e.state().Phase)
	}
	e.clock.advance(101 * time.Second)
	e.tick() // resume timeout → idle
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
			e.writeHandoff()
			e.s.events = append(e.s.events, stop("handoff"))
			e.tick()
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

func TestWatcher_RestartLandedInjectionNoDuplicateSend(t *testing.T) {
	e := newWatchEnv(t)
	e.injectHandoff()
	sends := e.s.count("send:")
	e.s.events = append(e.s.events, stop("handoff")) // no file yet: the phase keeps waiting
	e.w = e.newWatcher()
	e.tick()
	if e.s.count("send:") != sends {
		t.Fatalf("duplicate send in handoff-requested: %v", e.s.calls)
	}
	if !e.state().PhaseInjected {
		t.Error("landed injection should be confirmed")
	}

	e2 := newWatchEnv(t)
	e2.reachResuming()
	sends = e2.s.count("send:")
	e2.s.events = append(e2.s.events, stop("resumed"))
	e2.w = e2.newWatcher()
	e2.tick()
	if e2.s.count("send:") != sends || e2.state().Phase != PhaseIdle {
		t.Fatalf("calls = %v phase = %s", e2.s.calls, e2.state().Phase)
	}
}

func TestWatcher_RestartUnlandedInjectionResendsOnce(t *testing.T) {
	e := newWatchEnv(t)
	e.injectHandoff()
	first := e.s.calls[0]
	e.w = e.newWatcher()
	e.tick()
	e.tick()
	if len(e.s.calls) != 2 || e.s.calls[1] != first {
		t.Fatalf("handoff calls = %v", e.s.calls)
	}

	e2 := newWatchEnv(t)
	e2.reachResuming()
	resume := e2.s.calls[len(e2.s.calls)-1]
	e2.w = e2.newWatcher()
	e2.tick()
	e2.tick()
	if e2.s.count("send:") != 3 || e2.s.calls[len(e2.s.calls)-1] != resume {
		t.Fatalf("resume calls = %v", e2.s.calls)
	}
}

func TestWatcher_RestartInClearingClearsOnceAndCountsOnce(t *testing.T) {
	e := newWatchEnv(t)
	e.reachClearing()
	e.w = e.newWatcher()
	e.tick() // unconditional re-clear
	if e.s.count("clear") != 2 {
		t.Fatalf("calls = %v", e.s.calls)
	}
	e.tick() // probe passes
	st := e.state()
	if st.Phase != PhaseResuming || st.CycleCount != 1 {
		t.Fatalf("state = %+v", st)
	}
	if e.s.count("clear") != 2 {
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
	breakStencil(t, e.stDir, handoffStencilName)
	e.s.usage["a"] = 2000
	e.s.events = []shuttleengine.Event{stop("a")}
	if err := RequestCycle(e.paths, CycleClear, e.clock.now); err != nil {
		t.Fatal(err)
	}
	e.tick()
	e.clock.advance(11 * time.Second)
	e.tickErr()
	e.assertNoCalls()
	if e.state().Phase != PhaseIdle {
		t.Errorf("phase = %s", e.state().Phase)
	}
	if _, requested, _ := CycleRequested(e.paths); !requested {
		t.Error("cycle request should be kept")
	}
}

func TestWatcher_NoteRequestRendersTemplateFileBeforeTyping(t *testing.T) {
	e := newWatchEnv(t)
	e.injectHandoff()
	if _, err := os.Stat(e.paths.NoteTemplatePath); err != nil {
		t.Fatalf("note template file not rendered before the request: %v", err)
	}
	var sent string
	for _, c := range e.s.calls {
		if strings.HasPrefix(c, "send:") {
			sent = c
		}
	}
	if !strings.Contains(sent, e.paths.NoteTemplatePath) || !strings.Contains(sent, e.state().PendingHandoff) {
		t.Errorf("note request %q must name the template and the note path", sent)
	}
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

// endTurn reads one turn end and ticks.
func (e *watchEnv) endTurn(msg string) {
	e.t.Helper()
	e.s.events = append(e.s.events, stop(msg))
	e.tick()
}

// assertReload checks the calls after prefix are every skill in order, then a pointer naming the role file and, when note is not empty, the note.
func (e *watchEnv) assertReload(prefix, note string) {
	e.t.Helper()
	got := e.callsAfter(prefix)
	if len(got) != len(reloadSkills)+1 {
		e.t.Fatalf("calls after %q = %v, want %d skills and a pointer", prefix, got, len(reloadSkills))
	}
	for i, skill := range reloadSkills {
		if got[i] != "skill:"+skill {
			e.t.Errorf("call %d = %q, want the skill %s", i, got[i], skill)
		}
	}
	pointer := got[len(got)-1]
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
	e := newWatchEnv(t)
	e.withSkills()
	e.reachClearing()
	e.tick() // clearing -> resuming, first skill typed
	if st := e.state(); st.Phase != PhaseResuming || st.ReloadStep != 0 || st.ReloadTypedAt.IsZero() {
		t.Fatalf("state = %+v", st)
	}
	e.endTurn("t1")
	e.endTurn("t2")
	if st := e.state(); st.Phase != PhaseResuming || st.ReloadStep != len(reloadSkills) {
		t.Fatalf("state = %+v, want the pointer step", st)
	}
	e.s.usage["resumed"] = 300
	e.endTurn("resumed")
	e.assertReload("clear", e.state().LastHandoff)
	if st := e.state(); st.Phase != PhaseIdle || st.LastContextTokens != 300 || st.ReloadStep != 0 {
		t.Errorf("state = %+v", st)
	}
}

func TestWatcher_SkillSkipCauses(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		unknown  map[string]bool
		advances []time.Duration
		endTurns []string
		reloaded bool // the reload completes after the turn ends, else only the second skill follows the clear
	}{
		{
			name:     "unknown skill is skipped and logged",
			unknown:  map[string]bool{"scribe:prose": true},
			endTurns: []string{"t2", "resumed"},
			reloaded: true,
		},
		{
			name:     "silent skill is skipped at the timeout",
			advances: []time.Duration{99 * time.Second, 2 * time.Second},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			e := newWatchEnv(t)
			e.withSkills()
			e.s.skillUnknown = tt.unknown
			e.reachClearing()
			e.tick() // first skill typed
			for i, d := range tt.advances {
				if i > 0 {
					if st := e.state(); st.ReloadStep != 0 {
						t.Fatalf("ReloadStep = %d before the skip", st.ReloadStep)
					}
				}
				e.clock.advance(d)
				e.tick()
			}
			if len(tt.advances) == 0 {
				e.tick() // the skip lands, the next skill typed on the same tick
			}
			if st := e.state(); st.ReloadStep != 1 {
				t.Fatalf("ReloadStep = %d, want the skill skipped", st.ReloadStep)
			}
			for _, turn := range tt.endTurns {
				e.endTurn(turn)
			}
			if tt.reloaded {
				e.assertReload("clear", e.state().LastHandoff)
			} else if got := e.callsAfter("clear"); len(got) != 2 || got[1] != "skill:ly:board" {
				t.Errorf("calls after clear = %v", got)
			}
		})
	}
}

func TestWatcher_ReloadRestartRetypesOnlyTheUnconfirmedStep(t *testing.T) {
	e := newWatchEnv(t)
	e.withSkills()
	e.reachClearing()
	e.tick()
	e.endTurn("t1") // first skill confirmed, second typed
	typedAt := e.state().ReloadTypedAt

	e.clock.advance(50 * time.Second)
	e.w = e.newWatcher()
	e.s.idle = false
	e.tick()
	if got := e.callsAfter("clear"); len(got) != 2 {
		t.Fatalf("typed behind a failing probe: %v", got)
	}
	e.s.idle = true
	e.tick()
	got := e.callsAfter("clear")
	if len(got) != 3 || got[2] != "skill:ly:board" {
		t.Fatalf("calls after clear = %v, want only the unconfirmed skill typed again", got)
	}
	if st := e.state(); !st.ReloadTypedAt.Equal(typedAt) || st.ReloadStep != 1 {
		t.Errorf("state = %+v, want the step and its first typing time kept", st)
	}
}

func TestWatcher_ReloadRestartKeepsSkippedSkillSkipped(t *testing.T) {
	e := newWatchEnv(t)
	e.withSkills()
	e.reachClearing()
	e.tick()
	e.clock.advance(101 * time.Second)
	e.tick() // first skill timed out, second typed
	e.w = e.newWatcher()
	e.tick()
	for _, c := range e.callsAfter("clear") {
		if c == "skill:scribe:prose" && e.s.count("skill:scribe:prose") > 1 {
			t.Fatalf("a skipped skill was typed again: %v", e.s.calls)
		}
	}
	if st := e.state(); st.ReloadStep != 1 {
		t.Errorf("ReloadStep = %d, want 1", st.ReloadStep)
	}
}

func TestWatcher_ReloadTypesNothingWhenIdleProbeFails(t *testing.T) {
	e := newWatchEnv(t)
	e.withSkills()
	e.reachClearing()
	e.s.idleSeq = []bool{true, false, false}
	e.tick() // clearing probe passes, first skill typed
	e.s.events = append(e.s.events, stop("t1"))
	e.tick() // confirmed, but the next probe fails
	e.tick()
	if got := e.callsAfter("clear"); len(got) != 1 {
		t.Fatalf("typed behind a failing probe: %v", got)
	}
	e.s.idle = true
	e.tick()
	if got := e.callsAfter("clear"); len(got) != 2 || got[1] != "skill:ly:board" {
		t.Fatalf("calls after clear = %v", got)
	}
}

func TestWatcher_AutoCompactionReloadsSkillsRoleAndPointer(t *testing.T) {
	e := newWatchEnv(t)
	e.withSkills()
	boundary := e.clock.now.Add(time.Second)
	e.s.autoCompact = map[string]time.Time{"a": boundary}
	e.s.usage["a"] = 100
	e.endTurn("a")
	if st := e.state(); st.Phase != PhaseResuming || !st.CompactionBaseline.Equal(boundary) {
		t.Fatalf("state = %+v, want resuming with the baseline at the boundary", st)
	}
	if _, err := os.Stat(e.paths.RolePath); err != nil {
		t.Errorf("role file not rendered: %v", err)
	}
	e.endTurn("t1")
	e.endTurn("t2")
	e.endTurn("resumed")
	if got := e.s.calls; len(got) != len(reloadSkills)+1 {
		t.Fatalf("calls = %v", got)
	}
	pointer := e.s.calls[len(e.s.calls)-1]
	if !strings.Contains(pointer, e.paths.RolePath) || strings.Contains(pointer, "note") {
		t.Errorf("pointer = %q, want the role file and no note", pointer)
	}
	if st := e.state(); st.Phase != PhaseIdle {
		t.Errorf("phase = %s, want idle", st.Phase)
	}
	e.endTurn("later") // the same boundary again: no second reload
	e.tick()
	if len(e.s.calls) != len(reloadSkills)+1 {
		t.Errorf("a handled boundary reloaded twice: %v", e.s.calls)
	}
}

func TestWatcher_AutoCompactionBoundaryAtOrBeforeBaselineTriggersNothing(t *testing.T) {
	e := newWatchEnv(t)
	e.withSkills()
	e.setState(func(st *State) { st.CompactionBaseline = e.clock.now })
	e.w = e.newWatcher()
	e.s.autoCompact = map[string]time.Time{"a": e.clock.now}
	e.s.usage["a"] = 100
	e.endTurn("a")
	e.tick()
	e.assertNoCalls()
	if st := e.state(); st.Phase != PhaseIdle {
		t.Errorf("phase = %s", st.Phase)
	}
}

func TestWatcher_AutoCompactionWaitsForIdleProbe(t *testing.T) {
	e := newWatchEnv(t)
	e.withSkills()
	e.s.autoCompact = map[string]time.Time{"a": e.clock.now.Add(time.Second)}
	e.s.usage["a"] = 100
	e.s.idle = false
	e.endTurn("a")
	e.assertNoCalls()
	if st := e.state(); st.Phase != PhaseIdle || !st.CompactionBaseline.IsZero() {
		t.Fatalf("state = %+v, want idle with the baseline unmoved", st)
	}
	e.s.idle = true
	e.tick()
	if e.s.count("skill:") != 1 {
		t.Fatalf("calls = %v", e.s.calls)
	}
}

func TestWatcher_ClearRendersRoleFileBeforeResume(t *testing.T) {
	e := newWatchEnv(t)
	e.injectHandoff()
	e.writeHandoff()
	e.s.events = append(e.s.events, stop("handoff"))
	e.tick()
	if e.s.count("clear") != 1 {
		t.Fatalf("calls = %v; want the clear sent", e.s.calls)
	}
	if _, err := os.Stat(e.paths.RolePath); err != nil {
		t.Errorf("role file not rendered before the resume pointer: %v", err)
	}
	if !strings.Contains(e.state().PendingResume, e.paths.RolePath) {
		t.Errorf("resume pointer %q must name the role file", e.state().PendingResume)
	}
}

func TestWatcher_FailingRenderAbortsWithoutClear(t *testing.T) {
	cases := []struct {
		name    string
		stencil string
		content string
	}{
		{"role", roleStencilName, "{{.nope}}"},
		{"resume", resumeStencilName, "line one\nline two\n"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			e := newWatchEnv(t)
			e.injectHandoff()
			if err := os.WriteFile(stencilstore.Path(e.stDir, c.stencil), []byte(c.content), 0o644); err != nil {
				t.Fatal(err)
			}
			e.writeHandoff()
			e.s.events = append(e.s.events, stop("handoff"))
			e.tick()
			st := e.state()
			if st.Phase != PhaseIdle || e.s.count("clear") != 0 || !strings.Contains(st.LastAbortReason, c.stencil) {
				t.Fatalf("state = %+v calls = %v", st, e.s.calls)
			}
		})
	}
}

func TestWatcher_HandoffSendErrorResendsSamePathOnce(t *testing.T) {
	for _, behindFailingProbe := range []bool{false, true} {
		e := newWatchEnv(t)
		e.s.sendErrs = []error{errBoom}
		e.s.usage["a"] = 2000
		e.s.events = []shuttleengine.Event{stop("a")}
		e.tick()
		e.clock.advance(11 * time.Second)
		e.tickErr()
		first := e.s.calls[0]
		if behindFailingProbe {
			e.s.idle = false
			e.tick()
			if len(e.s.calls) != 1 {
				t.Fatalf("re-sent behind a failing probe: %v", e.s.calls)
			}
			e.s.idle = true
		}
		e.tick()
		e.tick()
		if len(e.s.calls) != 2 || e.s.calls[1] != first {
			t.Fatalf("calls = %v", e.s.calls)
		}
	}
}

func TestWatcher_ResumeSendErrorResendsOnce(t *testing.T) {
	e := newWatchEnv(t)
	e.reachClearing()
	e.s.sendErrs = []error{errBoom}
	e.tickErr()
	resume := e.s.calls[len(e.s.calls)-1]
	e.tick()
	e.tick()
	if e.s.count("send:") != 3 || e.s.calls[len(e.s.calls)-1] != resume {
		t.Fatalf("calls = %v", e.s.calls)
	}
}

func TestWatcher_ResumeSendErrorButLandedNoResend(t *testing.T) {
	e := newWatchEnv(t)
	e.reachClearing()
	e.s.sendErrs = []error{errBoom}
	e.tickErr()
	sends := e.s.count("send:")
	e.s.usage["resumed"] = 200
	e.s.events = append(e.s.events, stop("resumed"))
	e.tick()
	if e.s.count("send:") != sends || e.state().Phase != PhaseIdle {
		t.Fatalf("calls = %v phase = %s", e.s.calls, e.state().Phase)
	}
}

func TestWatcher_ClearTimeoutSendErrorResendsOnlyOncePassing(t *testing.T) {
	e := newWatchEnv(t)
	e.reachClearing()
	e.s.idle = false
	e.clock.advance(101 * time.Second)
	e.tick()
	e.s.idle = true
	e.s.sendErrs = []error{errBoom}
	e.tickErr()
	e.s.idle = false
	resume := e.s.calls[len(e.s.calls)-1]
	sends := e.s.count("send:")
	e.tick()
	e.tick()
	if e.s.count("send:") != sends {
		t.Fatalf("re-sent while the probe failed: %v", e.s.calls)
	}
	e.s.idle = true
	e.tick()
	e.tick()
	if e.s.count("send:") != sends+1 || e.s.calls[len(e.s.calls)-1] != resume {
		t.Fatalf("calls = %v", e.s.calls)
	}
}

func TestWatcher_ResumeSendErrorWhileTurnRunningNoResend(t *testing.T) {
	e := newWatchEnv(t)
	e.reachClearing()
	e.s.sendErrs = []error{errBoom}
	e.tickErr()
	sends := e.s.count("send:")
	e.s.idle = false
	e.tick()
	e.s.events = append(e.s.events, stop("resumed"))
	e.tick()
	if e.s.count("send:") != sends || e.state().Phase != PhaseIdle {
		t.Fatalf("calls = %v phase = %s", e.s.calls, e.state().Phase)
	}
}

func TestWatcher_RestartResumingInjectedTrueFailingThenPassingResendsOnce(t *testing.T) {
	e := newWatchEnv(t)
	e.reachResuming()
	sends := e.s.count("send:")
	if !e.state().PhaseInjected {
		t.Fatal("precondition: injection persisted true")
	}
	e.w = e.newWatcher()
	e.s.idleSeq = []bool{false, true}
	e.tick()
	if e.s.count("send:") != sends {
		t.Fatalf("re-sent behind a failing probe: %v", e.s.calls)
	}
	e.tick()
	e.tick()
	if e.s.count("send:") != sends+1 {
		t.Fatalf("calls = %v", e.s.calls)
	}
}

func TestWatcher_ClearErrorThenSuccessMovesOnce(t *testing.T) {
	e := newWatchEnv(t)
	e.injectHandoff()
	e.writeHandoff()
	e.s.events = append(e.s.events, stop("handoff"))
	e.s.clearErrs = []error{errBoom}
	e.tickErr()
	if e.state().PhaseInjected {
		t.Fatal("a failed clear must stay unconfirmed")
	}
	e.tick() // re-clear succeeds
	e.tick() // probe passes
	st := e.state()
	if st.Phase != PhaseResuming || st.CycleCount != 1 || e.s.count("clear") != 2 {
		t.Fatalf("state = %+v calls = %v", st, e.s.calls)
	}
}

func TestWatcher_ClearErrorsUntilTimeoutResumesWithoutCounting(t *testing.T) {
	e := newWatchEnv(t)
	e.injectHandoff()
	e.writeHandoff()
	e.s.events = append(e.s.events, stop("handoff"))
	e.s.clearErr = errBoom
	e.tickErr()
	e.tickErr()
	e.clock.advance(101 * time.Second)
	e.tick()
	st := e.state()
	if st.Phase != PhaseResuming || st.CycleCount != 0 {
		t.Fatalf("state = %+v", st)
	}
	if !strings.HasPrefix(e.s.calls[len(e.s.calls)-1], "send:") {
		t.Errorf("resume prompt not sent: %v", e.s.calls)
	}
}

func TestWatcher_PhaseForDifferentStrandIsReset(t *testing.T) {
	e := newWatchEnv(t)
	e.setState(func(s *State) {
		s.Phase = PhaseResuming
		s.PhaseStrand = "old"
		s.PendingResume = "resume"
	})
	e.tick()
	e.tick()
	st := e.state()
	if st.Phase != PhaseIdle || !strings.Contains(st.LastAbortReason, "resuming") {
		t.Fatalf("state = %+v", st)
	}
	e.assertNoCalls()
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
			if len(e.s.calls) != 1 || !strings.Contains(e.s.calls[0], "DEFER") {
				t.Fatalf("calls = %v, want one soft request", e.s.calls)
			}
		})
	}
}

func TestWatcher_SoftDeferWithoutFileReturnsIdleAndBlocksNextAttempt(t *testing.T) {
	e := newSoftEnv(t)
	e.injectSoft(stop("a"))
	e.s.events = append(e.s.events, stop("DEFER"))
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
		t.Fatalf("soft attempt before soft_idle_s after the DEFER: %v", e.s.calls)
	}
	e.clock.advance(time.Second)
	e.tick()
	if e.s.count("send:") != sends+1 || e.state().CycleTrigger != TriggerSoft {
		t.Fatalf("calls = %v state = %+v", e.s.calls, e.state())
	}
}

// TestWatcher_SoftDeferTurnEndAloneNeverRequalifies pins that a DEFER needs a fresh turn end after it:
// the DEFER turn end itself never starts another soft cycle, however long the session stays quiet.
func TestWatcher_SoftDeferTurnEndAloneNeverRequalifies(t *testing.T) {
	e := newSoftEnv(t)
	e.injectSoft(stop("a"))
	e.s.events = append(e.s.events, stop("DEFER"))
	e.tick()
	if st := e.state(); st.Phase != PhaseIdle || st.LastAbortReason != "deferred" {
		t.Fatalf("state = %+v, want idle after the deferral", st)
	}
	sends := e.s.count("send:")
	e.clock.advance(time.Hour)
	e.tick()
	if e.s.count("send:") != sends || e.state().Phase != PhaseIdle {
		t.Fatalf("a soft request was re-sent with no turn end after the DEFER: calls = %v", e.s.calls)
	}
}

func TestWatcher_SoftDeferAmongOtherTextDoesNotDefer(t *testing.T) {
	e := newSoftEnv(t)
	e.injectSoft(stop("a"))
	e.s.events = append(e.s.events, stop("DEFER, but first a note"))
	e.tick()
	if st := e.state(); st.Phase != PhaseHandoffRequested || !st.LastDeferral.IsZero() {
		t.Fatalf("state = %+v", st)
	}
}

func TestWatcher_SoftDeferWithHandoffWrittenProceedsToClearing(t *testing.T) {
	e := newSoftEnv(t)
	e.injectSoft(stop("a"))
	e.writeHandoff()
	e.s.events = append(e.s.events, stop("DEFER"))
	e.tick()
	if st := e.state(); st.Phase != PhaseClearing || !st.LastDeferral.IsZero() {
		t.Fatalf("state = %+v", st)
	}
}

func TestWatcher_DeferIgnoredInHardAndRequestedCycles(t *testing.T) {
	t.Run("hard", func(t *testing.T) {
		e := newWatchEnv(t)
		e.injectHandoff()
		e.s.events = append(e.s.events, stop("DEFER"))
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
		e.s.events = append(e.s.events, stop("DEFER"))
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
	if len(e.s.calls) != 1 || strings.Contains(e.s.calls[0], "DEFER") {
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
	e.w = e.newWatcher()
	e.tick()
	e.tick()
	if len(e.s.calls) != 2 || e.s.calls[1] != first || !strings.Contains(e.s.calls[1], "DEFER") {
		t.Fatalf("calls = %v", e.s.calls)
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

func TestWatcher_NoticesDeliveredOldestFirstOnePerTick(t *testing.T) {
	e := newWatchEnv(t)
	e.queue("first")
	e.queue("second")

	e.tick()
	if got := strings.Join(e.s.calls, "|"); got != "send:first" {
		t.Fatalf("calls after tick 1 = %q, want send:first", got)
	}
	if got := e.noticeLines(); len(got) != 1 || got[0] != "second" {
		t.Errorf("queue after tick 1 = %v, want [second]", got)
	}

	e.tick()
	if got := strings.Join(e.s.calls, "|"); got != "send:first|send:second" {
		t.Fatalf("calls after tick 2 = %q", got)
	}
	if got := e.noticeLines(); len(got) != 0 {
		t.Errorf("queue after tick 2 = %v, want empty", got)
	}
}

func TestWatcher_NoticeWaitsOnANonIdleProbe(t *testing.T) {
	e := newWatchEnv(t)
	e.s.idle = false
	e.queue("hold")
	e.tick()
	e.assertNoCalls()
	if got := e.noticeLines(); len(got) != 1 {
		t.Errorf("queue = %v, want the notice kept", got)
	}

	e.s.idle = true
	e.tick()
	if got := strings.Join(e.s.calls, "|"); got != "send:hold" {
		t.Errorf("calls = %q, want send:hold", got)
	}
}

func TestWatcher_NoticeWaitsOutANonIdlePhase(t *testing.T) {
	e := newWatchEnv(t)
	e.injectHandoff()
	e.s.calls = nil
	e.queue("later")
	e.tick()
	if n := e.s.count("send:later"); n != 0 {
		t.Fatalf("notice typed during a cycle: %v", e.s.calls)
	}
	if got := e.noticeLines(); len(got) != 1 {
		t.Errorf("queue = %v, want the notice kept", got)
	}
}

func TestWatcher_TickThatStartsACycleDeliversNoNotice(t *testing.T) {
	e := newWatchEnv(t)
	e.s.usage["a"] = 2000
	e.s.events = []shuttleengine.Event{stop("a")}
	e.tick()
	e.queue("not now")
	e.clock.advance(11 * time.Second)
	e.tick()
	if st := e.state(); st.Phase != PhaseHandoffRequested {
		t.Fatalf("phase = %s, want handoff-requested", st.Phase)
	}
	if n := e.s.count("send:not now"); n != 0 {
		t.Errorf("notice typed on the cycle's own tick: %v", e.s.calls)
	}
	if got := e.noticeLines(); len(got) != 1 {
		t.Errorf("queue = %v, want the notice kept", got)
	}
}
