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
	alive     bool
	events    []shuttleengine.Event
	usage     map[string]int // Turn-end message to tokens; a missing message is unknown.
	idle      bool
	idleSeq   []bool // Consumed before idle.
	sendErrs  []error
	clearErr  error // Returned by every ClearSession while set.
	clearErrs []error
	calls     []string // "send:<text>" and "clear", in order.
	tokenAsks []string
	onSend    func()
}

func (f *fakeSession) StrandAlive(string) (bool, error) { return f.alive, nil }

func (f *fakeSession) ReadEvents(_ string, offset int64) ([]shuttleengine.Event, int64, error) {
	if offset > int64(len(f.events)) {
		offset = int64(len(f.events))
	}
	return append([]shuttleengine.Event(nil), f.events[offset:]...), int64(len(f.events)), nil
}

func (f *fakeSession) ContextTokens(ev shuttleengine.Event) (int, bool, error) {
	f.tokenAsks = append(f.tokenAsks, ev.Message)
	n, ok := f.usage[ev.Message]
	return n, ok, nil
}

func (f *fakeSession) SessionIdle(string) (bool, error) {
	if len(f.idleSeq) > 0 {
		v := f.idleSeq[0]
		f.idleSeq = f.idleSeq[1:]
		return v, nil
	}
	return f.idle, nil
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
		cfg:   Config{ThresholdTokens: 1000, IdleGraceS: 10, HandoffTimeoutS: 100},
		clock: &fakeClock{now: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)},
	}
	if err := SaveState(e.paths, State{Strand: "s1", Phase: PhaseIdle}); err != nil {
		t.Fatal(err)
	}
	e.w = e.newWatcher()
	return e
}

func (e *watchEnv) newWatcher() *Watcher {
	return NewWatcher(e.s, e.cfg, e.paths, e.stDir, e.clock)
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
	if err := RequestCycle(e.paths); err != nil {
		t.Fatal(err)
	}
	e.tick()
	e.clock.advance(11 * time.Second)
	e.tick()
	if e.state().Phase != PhaseHandoffRequested {
		t.Fatalf("phase = %s", e.state().Phase)
	}
	if requested, _ := CycleRequested(e.paths); requested {
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

func TestWatcher_ProbeNeverPassingResumesAfterTimeout(t *testing.T) {
	e := newWatchEnv(t)
	e.reachClearing()
	e.s.idle = false
	e.tick()
	if e.state().Phase != PhaseClearing {
		t.Fatalf("phase = %s", e.state().Phase)
	}
	e.clock.advance(101 * time.Second)
	e.tick()
	st := e.state()
	if st.Phase != PhaseResuming || st.CycleCount != 1 || e.s.count("send:") != 2 {
		t.Fatalf("state = %+v calls = %v", st, e.s.calls)
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
	if err := RequestCycle(e.paths); err != nil {
		t.Fatal(err)
	}
	e.tick()
	e.clock.advance(11 * time.Second)
	e.tickErr()
	e.assertNoCalls()
	if e.state().Phase != PhaseIdle {
		t.Errorf("phase = %s", e.state().Phase)
	}
	if requested, _ := CycleRequested(e.paths); !requested {
		t.Error("cycle request should be kept")
	}
}

func TestWatcher_FailingResumeRenderAbortsWithoutClear(t *testing.T) {
	e := newWatchEnv(t)
	e.injectHandoff()
	breakStencil(t, e.stDir, resumeStencilName)
	e.writeHandoff()
	e.s.events = append(e.s.events, stop("handoff"))
	e.tick()
	st := e.state()
	if st.Phase != PhaseIdle || e.s.count("clear") != 0 || !strings.Contains(st.LastAbortReason, resumeStencilName) {
		t.Fatalf("state = %+v calls = %v", st, e.s.calls)
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
	e.s.sendErrs = []error{errBoom}
	e.tickErr()
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
