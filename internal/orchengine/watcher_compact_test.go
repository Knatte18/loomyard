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

// finishNote writes the pending note and reads a turn end after it, then ticks once: the note gate passes and `/compact` is typed.
func (e *watchEnv) finishNote() {
	e.t.Helper()
	e.writeHandoff()
	e.s.events = append(e.s.events, stop("note"))
	e.tick()
}

// reachCompacting makes one over-threshold turn end and ticks until `/compact` is typed, through the note gate.
func (e *watchEnv) reachCompacting() {
	e.t.Helper()
	e.injectHandoff()
	e.finishNote()
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
	if st := e.state(); st.Phase != PhaseHandoffRequested || st.CycleMode != CycleCompact || st.PendingHandoff == "" {
		t.Fatalf("state = %+v, want a compact-mode note request", st)
	}
	e.assertCompactCalls(0)
	note := e.writeHandoff()
	e.s.events = append(e.s.events, stop("note"))
	e.tick()
	want := "compact:" + e.compactFocus()
	if got := e.s.calls[len(e.s.calls)-1]; got != want {
		t.Fatalf("last call = %q, want %q", got, want)
	}
	e.assertCompactCalls(1)
	st := e.state()
	if st.Phase != PhaseCompacting || !st.PhaseInjected || st.CycleTrigger != TriggerHard {
		t.Errorf("state = %+v", st)
	}
	if st.LastHandoff != note {
		t.Errorf("LastHandoff = %q, want the note %q", st.LastHandoff, note)
	}
	if st.ReadingTurnEnd == nil || st.ReadingTurnEnd.Message != "a" {
		t.Errorf("ReadingTurnEnd = %+v, want the turn end a", st.ReadingTurnEnd)
	}
}

func TestCompact_PersistsCompactingBeforeTyping(t *testing.T) {
	e := newCompactEnv(t)
	var phaseAtCompact Phase
	e.injectHandoff()
	e.w.session = &phaseProbeSession{fakeSession: e.s, probe: func() { phaseAtCompact = e.state().Phase }}
	e.finishNote()
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

func TestCompact_RequestedCycleClearsRequestAndCompactsAfterNote(t *testing.T) {
	e := newCompactEnv(t)
	e.s.usage["a"] = 10
	e.s.events = []shuttleengine.Event{stop("a")}
	requestedAt := e.clock.now
	if err := RequestCycle(e.paths, CycleCompact, requestedAt); err != nil {
		t.Fatal(err)
	}
	e.tick()
	e.clock.advance(11 * time.Second)
	e.tick()
	e.assertCompactCalls(0)
	if st := e.state(); st.Phase != PhaseHandoffRequested || st.CycleTrigger != TriggerRequested || st.CycleMode != CycleCompact || !st.CycleRequestedAt.Equal(requestedAt) {
		t.Fatalf("state = %+v, want a requested compact note request", st)
	}
	if _, requested, _ := CycleRequested(e.paths); requested {
		t.Error("cycle request was not cleared")
	}
	e.finishNote()
	e.assertCompactCalls(1)
}

func TestCompact_RenderFailureAfterNoteChangesNothing(t *testing.T) {
	e := newCompactEnv(t)
	e.injectHandoff()
	if err := os.WriteFile(stencilstore.Path(e.stDir, compactStencilName), []byte("one\ntwo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	e.writeHandoff()
	e.s.events = append(e.s.events, stop("note"))
	e.tickErr()
	e.assertCompactCalls(0)
	if st := e.state(); st.Phase != PhaseHandoffRequested || st.LastHandoff != "" {
		t.Errorf("state = %+v, want the note phase kept and no LastHandoff", st)
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
	if st.Phase != PhaseResuming || st.CycleCount != 1 || st.LastContextTokens != 150 || !st.LastContextKnown {
		t.Errorf("state = %+v", st)
	}
	if !st.CompactionBaseline.Equal(entered.Add(2 * time.Second)) {
		t.Errorf("CompactionBaseline = %v, want the handled boundary", st.CompactionBaseline)
	}
	if st.LastAbortReason != "" || st.LastHandoff == "" {
		t.Errorf("completion recorded an abort or lost the note: %+v", st)
	}
	e.assertCompactCalls(1)
}

func TestCompact_CycleReloadsPluginsThenPointerNamingTheNote(t *testing.T) {
	t.Parallel()

	e := newCompactEnv(t)
	e.withSkills()
	e.reachCompacting()
	e.landBoundary(e.state().PhaseEnteredAt.Add(time.Second), 150)
	e.tick() // boundary read and the pane idle: the plugins reloaded
	if st := e.state(); st.Phase != PhaseResuming || st.ReloadStep != ReloadStepPointer || !st.ReloadSkipsSkills {
		t.Fatalf("state = %+v", st)
	}
	e.tick() // the pointer
	e.endTurn("resumed")
	e.assertReload("compact:", e.state().LastHandoff, false)
	if st := e.state(); st.Phase != PhaseIdle {
		t.Errorf("phase = %s, want idle", st.Phase)
	}
	e.assertCompactCalls(1)
	e.tick()
	if e.s.count(reloadPluginsCall) != 1 {
		t.Errorf("the compaction's own boundary reloaded again: %v", e.s.calls)
	}
}

func TestCompact_BoundaryAtEntryCompletes(t *testing.T) {
	e := newCompactEnv(t)
	e.reachCompacting()
	e.landBoundary(e.state().PhaseEnteredAt, 150)
	e.tick()
	if st := e.state(); st.Phase != PhaseResuming || st.CycleCount != 1 {
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
	if st := e.state(); st.Phase != PhaseIdle {
		t.Fatalf("phase = %s, want the hard trigger held in idle", st.Phase)
	}
	e.clock.advance(50 * time.Second)
	e.tick()
	if st := e.state(); st.Phase != PhaseHandoffRequested {
		t.Fatalf("phase = %s, want a note request once the hold ended", st.Phase)
	}
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
	e.finishNote()
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
	if st := e.state(); st.Phase != PhaseIdle {
		t.Fatalf("phase = %s, want the soft trigger held in idle", st.Phase)
	}
	e.clock.advance(31 * time.Second)
	e.tick()
	if st := e.state(); st.Phase != PhaseHandoffRequested {
		t.Fatalf("phase = %s, want a note request once the hold ended", st.Phase)
	}
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
	if err := RequestCycle(e.paths, CycleCompact, e.clock.now); err != nil {
		t.Fatal(err)
	}
	e.tick()
	e.clock.advance(11 * time.Second)
	e.tick()
	if st := e.state(); st.Phase != PhaseHandoffRequested {
		t.Fatalf("phase = %s, want the request to start a note request despite the hold", st.Phase)
	}
	e.finishNote()
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

func TestCompact_RestartWithBoundaryReadTypesNothing(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		idle      bool
		wantPhase Phase
		// wantCycles is the cycle count the restarted tick must have recorded.
		wantCycles int
	}{
		{"idle pane completes without retyping", true, PhaseResuming, 1},
		{"busy pane keeps compacting", false, PhaseCompacting, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			e := newCompactEnv(t)
			e.reachCompacting()
			e.landBoundary(e.state().PhaseEnteredAt.Add(time.Second), 150)

			e.w = e.newWatcher()
			e.s.idle = c.idle
			e.tick()
			e.assertCompactCalls(1)
			st := e.state()
			if st.Phase != c.wantPhase || st.CycleCount != c.wantCycles {
				t.Errorf("state = %+v, want phase %s after %d cycles", st, c.wantPhase, c.wantCycles)
			}
			if c.idle && st.LastContextTokens != 150 {
				t.Errorf("LastContextTokens = %d, want 150", st.LastContextTokens)
			}
		})
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

func TestCycleRequest_ModeFollowsRequestNotConfig(t *testing.T) {
	t.Run("clear request in compact config clears", func(t *testing.T) {
		e := newCompactEnv(t)
		e.s.usage["a"] = 10
		e.s.events = []shuttleengine.Event{stop("a")}
		if err := RequestCycle(e.paths, CycleClear, e.clock.now); err != nil {
			t.Fatal(err)
		}
		e.tick()
		e.clock.advance(11 * time.Second)
		e.tick()
		e.finishNote()
		if got := e.s.count("clear"); got != 1 {
			t.Fatalf("clear calls = %d, want 1: %v", got, e.s.calls)
		}
		e.assertCompactCalls(0)
		if st := e.state(); st.Phase != PhaseClearing || st.CycleMode != CycleClear {
			t.Errorf("state = %+v", st)
		}
	})
	t.Run("compact request in clear config compacts", func(t *testing.T) {
		e := newWatchEnv(t)
		e.s.boundary = map[string]time.Time{}
		e.s.usage["a"] = 10
		e.s.events = []shuttleengine.Event{stop("a")}
		if err := RequestCycle(e.paths, CycleCompact, e.clock.now); err != nil {
			t.Fatal(err)
		}
		e.tick()
		e.clock.advance(11 * time.Second)
		e.tick()
		e.finishNote()
		e.assertCompactCalls(1)
		if got := e.s.count("clear"); got != 0 {
			t.Errorf("clear calls = %d, want 0", got)
		}
	})
	t.Run("automatic trigger follows cycle_mode", func(t *testing.T) {
		e := newWatchEnv(t)
		e.injectHandoff()
		if st := e.state(); st.CycleMode != CycleClear || !st.CycleRequestedAt.IsZero() {
			t.Errorf("state = %+v, want clear mode and no request time", st)
		}
	})
}

func TestCompact_SoftCycleHonoursDefer(t *testing.T) {
	e := newCompactEnv(t)
	e.cfg.SoftThresholdTokens, e.cfg.ThresholdTokens = 500, 5000
	e.w = e.newWatcher()
	e.s.usage["a"] = 600
	e.s.events = []shuttleengine.Event{stop("a")}
	e.tick()
	e.clock.advance(61 * time.Second)
	e.tick()
	if st := e.state(); st.Phase != PhaseHandoffRequested || st.CycleTrigger != TriggerSoft {
		t.Fatalf("state = %+v, want a soft note request", st)
	}
	e.s.events = append(e.s.events, stop("DEFER"))
	e.tick()
	if st := e.state(); st.Phase != PhaseIdle || st.LastAbortReason != "deferred" || st.LastDeferral.IsZero() {
		t.Errorf("state = %+v, want idle after the deferral", st)
	}
	e.assertCompactCalls(0)
}

func TestCycleRequest_AbandonedWithinTimeoutOfRequest(t *testing.T) {
	t.Run("note never written", func(t *testing.T) {
		e := newCompactEnv(t)
		e.s.usage["a"] = 10
		e.s.events = []shuttleengine.Event{stop("a")}
		if err := RequestCycle(e.paths, CycleCompact, e.clock.now); err != nil {
			t.Fatal(err)
		}
		e.tick()
		e.clock.advance(11 * time.Second)
		e.tick()
		if e.state().Phase != PhaseHandoffRequested {
			t.Fatalf("phase = %s", e.state().Phase)
		}
		e.clock.advance(90 * time.Second)
		e.tick()
		if st := e.state(); st.Phase != PhaseIdle || st.LastAbortReason != "handoff timed out" {
			t.Errorf("state = %+v, want idle after the timeout measured from the request", st)
		}
		e.assertCompactCalls(0)
	})
	t.Run("idle probe never passes", func(t *testing.T) {
		e := newCompactEnv(t)
		e.s.idle = false
		e.s.usage["a"] = 10
		e.s.events = []shuttleengine.Event{stop("a")}
		if err := RequestCycle(e.paths, CycleClear, e.clock.now); err != nil {
			t.Fatal(err)
		}
		e.tick()
		e.clock.advance(11 * time.Second)
		e.tick()
		e.clock.advance(100 * time.Second)
		e.tick()
		if _, pending, _ := CycleRequested(e.paths); pending {
			t.Error("request still pending past the timeout")
		}
		e.s.idle = true
		e.clock.advance(11 * time.Second)
		e.tick()
		e.assertNoCalls()
		if st := e.state(); st.Phase != PhaseIdle {
			t.Errorf("phase = %s, want idle", st.Phase)
		}
	})
}

func TestCycleRequest_StaleMarkerRemovedAtFirstTick(t *testing.T) {
	for name, write := range map[string]func(*watchEnv){
		"old request": func(e *watchEnv) {
			if err := RequestCycle(e.paths, CycleClear, e.clock.now.Add(-101*time.Second)); err != nil {
				e.t.Fatal(err)
			}
		},
		"unparseable": func(e *watchEnv) {
			if err := os.WriteFile(e.paths.CycleRequestPath, []byte("cycle\n"), 0o644); err != nil {
				e.t.Fatal(err)
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			e := newWatchEnv(t)
			write(e)
			e.s.usage["a"] = 10
			e.s.events = []shuttleengine.Event{stop("a")}
			e.tick()
			e.clock.advance(11 * time.Second)
			e.tick()
			if _, pending, _ := CycleRequested(e.paths); pending {
				t.Error("stale marker was not removed")
			}
			e.assertNoCalls()
			if st := e.state(); st.Phase != PhaseIdle {
				t.Errorf("phase = %s, want idle", st.Phase)
			}
		})
	}
}

func TestCycleRequest_RestartedWatcherMeasuresFromRequestTime(t *testing.T) {
	e := newCompactEnv(t)
	e.s.usage["a"] = 10
	e.s.events = []shuttleengine.Event{stop("a")}
	if err := RequestCycle(e.paths, CycleCompact, e.clock.now); err != nil {
		t.Fatal(err)
	}
	e.tick()
	e.clock.advance(11 * time.Second)
	e.tick()
	if e.state().Phase != PhaseHandoffRequested {
		t.Fatalf("phase = %s", e.state().Phase)
	}
	e.w = e.newWatcher()
	e.clock.advance(90 * time.Second)
	e.tick()
	if st := e.state(); st.Phase != PhaseIdle || st.LastAbortReason != "handoff timed out" {
		t.Errorf("state = %+v, want the restarted watcher to time out from the request time", st)
	}
}
