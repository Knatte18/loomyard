package orchengine

import (
	"os"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/stencilstore"
)

// newCompactEnv builds a watchEnv in compact mode whose soft threshold sits above the hard cap, so only the hard cap and a request fire.
func newCompactEnv(t *testing.T) *watchEnv {
	t.Helper()
	e := newWatchEnv(t)
	e.cfg.CycleMode = CycleCompact
	e.cfg.SoftThresholdTokens = 5000
	e.cfg.SoftIdleS = 60
	e.s.boundary = map[string]time.Time{}
	e.w = e.newWatcher()
	return e
}

func (e *watchEnv) compactFocus() string {
	e.t.Helper()
	focus, err := RenderCompactFocus(e.stDir)
	if err != nil {
		e.t.Fatal(err)
	}
	return focus
}

// reachCompacting makes one over-threshold turn end and ticks until `/compact` is typed.
func (e *watchEnv) reachCompacting() {
	e.t.Helper()
	e.s.usage["a"] = 2000
	e.s.events = append(e.s.events, stop("a"))
	e.tick()
	e.clock.advance(11 * time.Second)
	e.tick()
	if st := e.state(); st.Phase != PhaseCompacting {
		e.t.Fatalf("phase = %s, want compacting", st.Phase)
	}
}

// landBoundary makes the transcript read through turn end "a" as a compaction boundary at, with tokens.
func (e *watchEnv) landBoundary(at time.Time, tokens int) {
	e.s.boundary["a"] = at
	e.s.usage["a"] = tokens
}

func (e *watchEnv) assertCompactCalls(n int) {
	e.t.Helper()
	if got := e.s.count("compact:"); got != n {
		e.t.Fatalf("compact calls = %d, want %d: %v", got, n, e.s.calls)
	}
}

func TestCompact_TriggerTypesCompactOnlyOnIdlePass(t *testing.T) {
	e := newCompactEnv(t)
	e.s.idle = false
	e.s.usage["a"] = 2000
	e.s.events = []shuttleengine.Event{stop("a")}
	e.tick()
	e.clock.advance(11 * time.Second)
	e.tick()
	e.assertNoCalls()
	if e.state().Phase != PhaseIdle {
		t.Fatalf("phase = %s, want idle", e.state().Phase)
	}

	e.s.idle = true
	e.tick()
	want := "compact:" + e.compactFocus()
	if len(e.s.calls) != 1 || e.s.calls[0] != want {
		t.Fatalf("calls = %v, want [%s]", e.s.calls, want)
	}
	st := e.state()
	if st.Phase != PhaseCompacting || !st.PhaseInjected || st.CycleTrigger != TriggerHard {
		t.Errorf("state = %+v", st)
	}
	if st.PendingHandoff != "" || st.LastHandoff != "" {
		t.Errorf("compact mode wrote handoff state: %+v", st)
	}
	if st.ReadingTurnEnd == nil || st.ReadingTurnEnd.Message != "a" {
		t.Errorf("ReadingTurnEnd = %+v, want the turn end a", st.ReadingTurnEnd)
	}
}

func TestCompact_PersistsCompactingBeforeTyping(t *testing.T) {
	e := newCompactEnv(t)
	var phaseAtCompact Phase
	e.s.usage["a"] = 2000
	e.s.events = []shuttleengine.Event{stop("a")}
	e.tick()
	e.clock.advance(11 * time.Second)
	e.w.session = &phaseProbeSession{fakeSession: e.s, probe: func() { phaseAtCompact = e.state().Phase }}
	e.tick()
	if phaseAtCompact != PhaseCompacting {
		t.Errorf("phase when /compact was typed = %s, want compacting", phaseAtCompact)
	}
}

// phaseProbeSession runs probe whenever CompactSession is called.
type phaseProbeSession struct {
	*fakeSession
	probe func()
}

func (p *phaseProbeSession) CompactSession(guid, focus string) error {
	p.probe()
	return p.fakeSession.CompactSession(guid, focus)
}

func TestCompact_RequestedCycleClearsRequestAndCompacts(t *testing.T) {
	e := newCompactEnv(t)
	e.s.usage["a"] = 10
	e.s.events = []shuttleengine.Event{stop("a")}
	if err := RequestCycle(e.paths); err != nil {
		t.Fatal(err)
	}
	e.tick()
	e.clock.advance(11 * time.Second)
	e.tick()
	e.assertCompactCalls(1)
	if st := e.state(); st.CycleTrigger != TriggerRequested {
		t.Errorf("trigger = %q", st.CycleTrigger)
	}
	if requested, _ := CycleRequested(e.paths); requested {
		t.Error("cycle request was not cleared")
	}
}

func TestCompact_RenderFailureChangesNothing(t *testing.T) {
	e := newCompactEnv(t)
	if err := os.WriteFile(stencilstore.Path(e.stDir, compactStencilName), []byte("one\ntwo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := RequestCycle(e.paths); err != nil {
		t.Fatal(err)
	}
	e.s.usage["a"] = 10
	e.s.events = []shuttleengine.Event{stop("a")}
	e.tick()
	e.clock.advance(11 * time.Second)
	e.tickErr()
	e.assertNoCalls()
	if st := e.state(); st.Phase != PhaseIdle {
		t.Errorf("phase = %s, want idle", st.Phase)
	}
	if requested, _ := CycleRequested(e.paths); !requested {
		t.Error("cycle request was cleared by a failed render")
	}
}

func TestCompact_CompletesOnBoundaryAfterEntryAndIdle(t *testing.T) {
	e := newCompactEnv(t)
	e.reachCompacting()
	entered := e.state().PhaseEnteredAt

	e.landBoundary(entered.Add(-time.Second), 3000)
	e.clock.advance(5 * time.Second)
	e.tick()
	if st := e.state(); st.Phase != PhaseCompacting || st.CycleCount != 0 {
		t.Fatalf("a boundary before entry completed the phase: %+v", st)
	}

	e.landBoundary(entered.Add(2*time.Second), 150)
	e.s.idle = false
	e.tick()
	if st := e.state(); st.Phase != PhaseCompacting {
		t.Fatalf("phase = %s, want compacting while the pane is not idle", st.Phase)
	}

	e.s.idle = true
	e.tick()
	st := e.state()
	if st.Phase != PhaseIdle || st.CycleCount != 1 || st.LastContextTokens != 150 || !st.LastContextKnown {
		t.Errorf("state = %+v", st)
	}
	if st.LastAbortReason != "" || st.LastHandoff != "" {
		t.Errorf("completion recorded an abort or a handoff: %+v", st)
	}
	e.assertCompactCalls(1)
}

func TestCompact_BoundaryAtEntryCompletes(t *testing.T) {
	e := newCompactEnv(t)
	e.reachCompacting()
	e.landBoundary(e.state().PhaseEnteredAt, 150)
	e.tick()
	if st := e.state(); st.Phase != PhaseIdle || st.CycleCount != 1 {
		t.Errorf("state = %+v", st)
	}
}

func TestCompact_TimeoutReturnsToIdleAndHoldsAutomaticTriggers(t *testing.T) {
	e := newCompactEnv(t)
	e.reachCompacting()
	e.s.usage["a"] = 1500
	e.clock.advance(100 * time.Second)
	e.tick()
	st := e.state()
	if st.Phase != PhaseIdle || st.LastAbortReason != "compaction timed out" {
		t.Fatalf("state = %+v", st)
	}
	if !st.LastDeferral.Equal(e.clock.Now()) {
		t.Errorf("LastDeferral = %v, want now %v", st.LastDeferral, e.clock.Now())
	}
	if st.LastContextTokens != 1500 || !st.LastContextKnown {
		t.Errorf("reading was not re-read: %d known=%v", st.LastContextTokens, st.LastContextKnown)
	}
	if st.CycleCount != 0 {
		t.Errorf("a timed-out compaction counted as a cycle: %d", st.CycleCount)
	}

	// A hard trigger is held for soft_idle_s (60s) after the failure.
	e.s.usage["b"] = 2000
	e.s.events = append(e.s.events, stop("b"))
	e.tick()
	e.clock.advance(11 * time.Second)
	e.tick()
	e.assertCompactCalls(1)
	e.clock.advance(50 * time.Second)
	e.tick()
	e.assertCompactCalls(2)
}

func TestCompact_TimeoutHoldsSoftTrigger(t *testing.T) {
	e := newCompactEnv(t)
	e.cfg.SoftThresholdTokens = 500
	e.cfg.ThresholdTokens = 5000
	e.w = e.newWatcher()
	e.s.usage["a"] = 600
	e.s.events = []shuttleengine.Event{stop("a")}
	e.tick()
	e.clock.advance(61 * time.Second)
	e.tick()
	e.assertCompactCalls(1)

	e.clock.advance(100 * time.Second)
	e.tick()
	if st := e.state(); st.Phase != PhaseIdle || st.LastAbortReason != "compaction timed out" {
		t.Fatalf("state = %+v", st)
	}

	e.s.usage["b"] = 600
	e.s.events = append(e.s.events, stop("b"))
	e.tick()
	e.clock.advance(30 * time.Second)
	e.tick()
	e.assertCompactCalls(1)
	e.clock.advance(31 * time.Second)
	e.tick()
	e.assertCompactCalls(2)
}

func TestCompact_TimeoutDoesNotHoldRequestedCycle(t *testing.T) {
	e := newCompactEnv(t)
	e.reachCompacting()
	e.clock.advance(100 * time.Second)
	e.tick()
	if e.state().Phase != PhaseIdle {
		t.Fatalf("phase = %s, want idle", e.state().Phase)
	}

	e.s.usage["b"] = 10
	e.s.events = append(e.s.events, stop("b"))
	if err := RequestCycle(e.paths); err != nil {
		t.Fatal(err)
	}
	e.tick()
	e.clock.advance(11 * time.Second)
	e.tick()
	e.assertCompactCalls(2)
}

func TestCompact_RestartRetypesOnlyOnIdleWithNoBoundary(t *testing.T) {
	e := newCompactEnv(t)
	e.reachCompacting()

	e.w = e.newWatcher()
	e.s.idle = false
	e.tick()
	e.assertCompactCalls(1)

	e.s.idle = true
	e.tick()
	e.assertCompactCalls(2)
	if st := e.state(); st.Phase != PhaseCompacting || !st.PhaseInjected {
		t.Errorf("state = %+v", st)
	}
}

func TestCompact_RestartCompletesWithoutRetypingWhenBoundaryRead(t *testing.T) {
	e := newCompactEnv(t)
	e.reachCompacting()
	e.landBoundary(e.state().PhaseEnteredAt.Add(time.Second), 150)

	e.w = e.newWatcher()
	e.tick()
	e.assertCompactCalls(1)
	if st := e.state(); st.Phase != PhaseIdle || st.CycleCount != 1 || st.LastContextTokens != 150 {
		t.Errorf("state = %+v", st)
	}
}

func TestCompact_RestartWithBoundaryButBusyPaneTypesNothing(t *testing.T) {
	e := newCompactEnv(t)
	e.reachCompacting()
	e.landBoundary(e.state().PhaseEnteredAt.Add(time.Second), 150)

	e.w = e.newWatcher()
	e.s.idle = false
	e.tick()
	e.assertCompactCalls(1)
	if st := e.state(); st.Phase != PhaseCompacting {
		t.Errorf("phase = %s, want compacting", st.Phase)
	}
}

func TestCompact_DeferTurnEndChangesNothing(t *testing.T) {
	e := newCompactEnv(t)
	e.s.usage["DEFER"] = 10
	e.s.events = []shuttleengine.Event{stop("DEFER")}
	e.tick()
	e.clock.advance(11 * time.Second)
	e.tick()
	e.assertNoCalls()
	if st := e.state(); st.Phase != PhaseIdle || !st.LastDeferral.IsZero() || st.LastAbortReason != "" {
		t.Errorf("idle DEFER changed state: %+v", st)
	}
}

func TestCompact_DeferTurnEndWhileCompactingChangesNothing(t *testing.T) {
	e := newCompactEnv(t)
	e.reachCompacting()
	e.s.events = append(e.s.events, stop("DEFER"))
	e.clock.advance(time.Second)
	e.tick()
	st := e.state()
	if st.Phase != PhaseCompacting || !st.LastDeferral.IsZero() || st.LastAbortReason != "" {
		t.Errorf("DEFER in compacting changed state: %+v", st)
	}
	e.assertCompactCalls(1)
}
