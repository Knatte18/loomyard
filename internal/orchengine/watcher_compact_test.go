package orchengine

import (
	"os"
	"strings"
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
	e.s.autoCompact = map[string]shuttleengine.CompactionBoundary{}
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

// finishNote writes the pending note and answers the request with `written <path>`, then ticks once: the note gate passes and the next step is sent.
func (e *watchEnv) finishNote() {
	e.t.Helper()
	path := e.writeHandoff()
	e.reply("written " + path)
	e.tick()
}

// reachCompacting makes one over-threshold turn end and ticks until `/compact` is sent, through the note gate.
func (e *watchEnv) reachCompacting() {
	e.t.Helper()
	e.injectHandoff()
	e.finishNote()
	if st := e.state(); st.Phase != PhaseCompacting {
		e.t.Fatalf("phase = %s, want compacting", st.Phase)
	}
}

// landBoundary makes the transcript of turn end "a" hold a compaction boundary at, with no turn end after it, and reads tokens through "a".
func (e *watchEnv) landBoundary(at time.Time, tokens int) {
	e.landBoundaryFollowedBy(at, tokens, 0)
}

// landBoundaryFollowedBy is landBoundary with turnEndsAfter turn ends after the boundary, "a" being the newest.
func (e *watchEnv) landBoundaryFollowedBy(at time.Time, tokens, turnEndsAfter int) {
	e.s.autoCompact["a"] = shuttleengine.CompactionBoundary{At: at, TurnEndsAfter: turnEndsAfter, ReadTurnEndAfter: turnEndsAfter > 0}
	e.s.usage["a"] = tokens
}

func (e *watchEnv) assertCompactCalls(n int) {
	e.t.Helper()
	if got := e.s.count("compact:"); got != n {
		e.t.Fatalf("compact calls = %d, want %d: %v", got, n, e.s.calls)
	}
}

func TestCompact_PersistsCompactingBeforeTyping(t *testing.T) {
	e := newCompactEnv(t)
	var stateAtCompact State
	e.injectHandoff()
	e.s.onSend = func() {
		if strings.HasPrefix(e.s.calls[len(e.s.calls)-1], "compact:") {
			stateAtCompact = e.state()
		}
	}
	e.finishNote()
	if stateAtCompact.Phase != PhaseCompacting || stateAtCompact.Pending == nil || stateAtCompact.Pending.Command != commandCompact || stateAtCompact.Pending.ID != 0 {
		t.Errorf("state when /compact was sent = %+v, want compacting with the compact pending and no id", stateAtCompact)
	}
	want := "compact:" + e.compactFocus()
	if got := e.s.calls[len(e.s.calls)-1]; got != want {
		t.Fatalf("last call = %q, want %q", got, want)
	}
	st := e.state()
	if st.Phase != PhaseCompacting || st.Pending == nil || st.Pending.ID == 0 || st.CycleTrigger != TriggerHard || st.LastHandoff == "" {
		t.Errorf("state = %+v", st)
	}
	if st.ReadingTurnEnd == nil || st.ReadingTurnEnd.Message != "a" {
		t.Errorf("ReadingTurnEnd = %+v, want the turn end a", st.ReadingTurnEnd)
	}
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
	path := e.writeHandoff()
	e.reply("written " + path)
	e.tickErr()
	e.assertCompactCalls(0)
	if st := e.state(); st.Phase != PhaseHandoffRequested || st.LastHandoff != "" {
		t.Errorf("state = %+v, want the note phase kept and no LastHandoff", st)
	}
	if len(e.c.mailbox) != 1 {
		t.Errorf("mailbox = %+v, want the reply left unread", e.c.mailbox)
	}
}

func TestCompact_CompletesOnBoundaryAfterEntryAndIdle(t *testing.T) {
	t.Parallel()

	for _, c := range []struct {
		name          string
		turnEndsAfter int
		// unconfirmed scripts the /compact inject as typed but not seen to be taken, which keeps the search going.
		unconfirmed bool
	}{
		{"no turn end after the boundary", 0, false},
		{"a turn end answered after the boundary", 2, false},
		{"an unconfirmed compact", 0, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			e := newCompactEnv(t)
			e.reachCompacting()
			if c.unconfirmed {
				e.setOutcome(e.lastInjectID(), CreelOutcome{State: CreelUnconfirmed})
			}
			entered := e.state().PhaseEnteredAt

			for _, at := range []time.Time{entered.Add(-time.Second), entered} {
				e.landBoundaryFollowedBy(at, 3000, c.turnEndsAfter)
				e.clock.advance(5 * time.Second)
				e.tick()
				if st := e.state(); st.Phase != PhaseCompacting || st.CycleCount != 0 {
					t.Fatalf("a boundary at %v (entry %v) completed the phase: %+v", at, entered, st)
				}
			}

			e.landBoundaryFollowedBy(entered.Add(2*time.Second), 150, c.turnEndsAfter)
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
		})
	}
}

func TestCompact_CompletionRemovesOnlyASatisfiedCompactRequest(t *testing.T) {
	t.Parallel()

	const (
		duringHandoff    = "during handoff"
		duringCompacting = "during compacting"
		afterBoundary    = "after boundary"
	)
	cases := []struct {
		name        string
		mode        string
		when        string
		wantPending bool
	}{
		{"compact request during the handoff phase is removed", CycleCompact, duringHandoff, false},
		{"compact request during compacting before the boundary is removed", CycleCompact, duringCompacting, false},
		{"compact request after the boundary stays pending", CycleCompact, afterBoundary, true},
		{"clear request during the cycle stays pending", CycleClear, duringCompacting, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			e := newCompactEnv(t)
			e.injectHandoff()
			request := func(at time.Time) {
				if err := RequestCycle(e.paths, c.mode, at); err != nil {
					t.Fatal(err)
				}
			}
			if c.when == duringHandoff {
				request(e.clock.Now())
			}
			e.finishNote()
			entered := e.state().PhaseEnteredAt
			boundaryAt := entered.Add(2 * time.Second)
			switch c.when {
			case duringCompacting:
				request(entered.Add(time.Second))
			case afterBoundary:
				request(boundaryAt.Add(time.Second))
			}
			e.landBoundary(boundaryAt, 150)
			e.tick()
			if st := e.state(); st.Phase != PhaseResuming {
				t.Fatalf("phase = %s, want the compaction completed", st.Phase)
			}
			if _, pending, _ := CycleRequested(e.paths); pending != c.wantPending {
				t.Errorf("request pending = %v, want %v", pending, c.wantPending)
			}
		})
	}
}

func TestCompact_CycleReloadsPluginsThenPointerNamingTheNote(t *testing.T) {
	t.Parallel()

	e := newCompactEnv(t)
	e.withSkills()
	e.reachCompacting()
	e.landBoundary(e.state().PhaseEnteredAt.Add(time.Second), 150)
	e.tick() // boundary read: the color queued
	if st := e.state(); st.Phase != PhaseResuming || st.ReloadStep != ReloadStepColor || !st.ReloadSkipsSkills {
		t.Fatalf("state = %+v", st)
	}
	e.tick() // the color typed, the plugins queued
	e.tick() // the plugins typed, the pointer queued
	e.endTurn("resumed")
	e.assertReload("compact:", e.state().LastHandoff, false)
	if role, err := os.ReadFile(e.paths.RolePath); err != nil || !strings.Contains(string(role), testIndex) {
		t.Errorf("role file = %q, %v; want it rendered with the index", role, err)
	}
	if st := e.state(); st.Phase != PhaseIdle {
		t.Errorf("phase = %s, want idle", st.Phase)
	}
	e.assertCompactCalls(1)
	e.tick()
	if e.s.count(reloadPluginsCall) != 1 {
		t.Errorf("the compaction's own boundary reloaded again: %v", e.s.calls)
	}
}

func TestCompact_TimeoutReturnsToIdleAndHoldsAutomaticTriggers(t *testing.T) {
	t.Parallel()

	for _, c := range []struct {
		name        string
		unconfirmed bool
	}{
		{"a compact that was typed", false},
		{"an unconfirmed compact", true},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			e := newCompactEnv(t)
			e.reachCompacting()
			if c.unconfirmed {
				e.setOutcome(e.lastInjectID(), CreelOutcome{State: CreelUnconfirmed})
			}
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
		})
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

// TestCompact_RestartRetypesOnlyOnIdleWithNoBoundary pins that a restart in compacting reads the saved compact's outcome and sends nothing again.
// The compact is sent once more only when its id was never saved and creel journaled none.
func TestCompact_RestartRetypesOnlyOnIdleWithNoBoundary(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		// arrange leaves the compact pending as the row says and scripts the journal.
		arrange      func(e *watchEnv)
		wantCompacts int
	}{
		{"a saved id is read and not sent again", func(e *watchEnv) {}, 1},
		{"an id never saved and none journaled sends it once more", func(e *watchEnv) { e.forgetLastSend() }, 2},
		{"an id never saved and one journaled adopts it", func(e *watchEnv) { e.c.sent = []CreelSent{e.forgetLastSend()} }, 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			e := newCompactEnv(t)
			e.reachCompacting()
			c.arrange(e)
			e.w = e.newWatcher()
			e.tick()
			e.tick()
			e.assertCompactCalls(c.wantCompacts)
			if st := e.state(); st.Phase != PhaseCompacting || !st.PhaseInjected && st.Pending == nil {
				t.Errorf("state = %+v", st)
			}
		})
	}
}

func TestCompact_RestartWithBoundaryReadTypesNothing(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		// turnEndsAfter is the number of turn ends after the boundary in the transcript.
		turnEndsAfter int
	}{
		{"completes without retyping", 0},
		{"completes on a boundary followed by a turn end", 2},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			e := newCompactEnv(t)
			e.reachCompacting()
			e.landBoundaryFollowedBy(e.state().PhaseEnteredAt.Add(time.Second), 150, c.turnEndsAfter)

			e.w = e.newWatcher()
			e.tick()
			e.assertCompactCalls(1)
			st := e.state()
			if st.Phase != PhaseResuming || st.CycleCount != 1 || st.LastContextTokens != 150 {
				t.Errorf("state = %+v, want the completed compaction", st)
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
	e.reply("defer")
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
	t.Run("hold never granted", func(t *testing.T) {
		e := newCompactEnv(t)
		e.c.failNext("hold", errBoom)
		e.c.failNext("hold", errBoom)
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
