// conditionedsend_test.go covers Runner.SendConditioned through a fake pane and a session-signal fake engine, with a fake clock so no row waits.

package shuttleengine

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

// framedReed is a tuiReed whose capture is the fixed frame while one is set.
type framedReed struct {
	*tuiReed

	frame string
}

func (r *framedReed) CapturePane(guid string) (string, error) {
	if r.frame != "" {
		return r.frame, nil
	}
	return r.tuiReed.CapturePane(guid)
}

var _ ReedOps = (*framedReed)(nil)

// TestRunner_SendConditioned pins the conditioned send over a session stalled on an API error:
// a clean send lands and reports its turn start;
// every refusal types nothing and waits on nothing;
// a send that does not land leaves its text in the box;
// and a pending-send record is not consulted.
func TestRunner_SendConditioned(t *testing.T) {
	const line = "run the suite"
	want := SendCondition{State: SessionIdleStalled, Cause: SessionCauseAPIError}
	cfg := Config{StartupTimeoutS: 30, RunTimeoutMin: 5, SubmitSettleMS: 100, SubmitConfirmTimeoutS: 30, SendReadyTimeoutS: 60}
	stalled := "START\nAPIERR\n"

	tests := []struct {
		name   string
		events string
		box    string
		frame  string
		active bool
		// pasteEngine runs the row over the engine that takes pastes, with a pending-send record of the line from an earlier send.
		pasteEngine bool
		// swallowEnter keeps the pane from ever submitting the box.
		swallowEnter bool
		// startsTurn appends a turn start to the events file once the box reads submitted.
		startsTurn      bool
		wantIs          error
		wantBusyError   bool
		wantDraft       bool
		wantTurnStarted bool
		// wantTyped is whether the line reached the pane.
		wantTyped bool
		wantBox   string
		// wantSubmitKeys is whether the pane was sent its Enter; every row asserts no clear key.
		wantSubmitKeys bool
	}{
		{name: "clean", events: stalled, startsTurn: true, wantTurnStarted: true, wantTyped: true, wantSubmitKeys: true},
		{name: "landed with no turn start", events: stalled, wantTyped: true, wantSubmitKeys: true},
		{name: "busy", events: "START\n", wantIs: ErrSendConditionUnmet},
		{name: "unmatched turn start after the stall", events: stalled + "START\n", wantIs: ErrSendConditionUnmet},
		{name: "draft", events: stalled, box: "half a thought", wantBusyError: true, wantDraft: true, wantIs: ErrSessionBusy, wantBox: "half a thought"},
		{name: "lone stray character", events: stalled, box: "x", wantBusyError: true, wantDraft: true, wantIs: ErrSessionBusy, wantBox: "x"},
		{name: "operator active", events: stalled, active: true, wantBusyError: true, wantIs: ErrSessionBusy},
		{name: "no input box", events: stalled, frame: "IDLE\nsome menu", wantBusyError: true, wantIs: ErrSessionBusy},
		{name: "rewind picker", events: stalled, frame: "REWIND-PICKER\n1. an entry", wantIs: ErrRewindPickerOpen},
		{name: "not landed", events: stalled, swallowEnter: true, wantIs: ErrSubmissionNotLanded, wantTyped: true, wantBox: line, wantSubmitKeys: true},
		{name: "pending record of an earlier send", events: stalled, pasteEngine: true, startsTurn: true, wantTurnStarted: true, wantTyped: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			clock := &hookClock{fakeClock: newFakeClock(time.Date(2026, 10, 8, 8, 0, 0, 0, time.UTC))}
			start := clock.Now()
			base := &fakeReed{StatusQueue: liveStrandStatus(true), ActiveQueue: []bool{tt.active}}
			tui := &tuiReed{fakeReed: base, clock: clock, box: tt.box}
			if tt.swallowEnter {
				tui.swallowEnterUntil = start.Add(time.Hour)
			}
			reed := &framedReed{tuiReed: tui, frame: tt.frame}
			var engine Engine = &pickerEngine{readinessEngine: readinessEngine{idleEngine: idleEngine{sessionFakeEngine: sessionFakeEngine{liveness: LivenessAlive}}}}
			if tt.pasteEngine {
				engine = &strayPasteEngine{pasteEngine: &pasteEngine{tuiEngine: newTUIEngine(), pasteSubmits: true}, alive: true}
			}
			fx := newFixture(t, reed, engine, withConfig(cfg), withClock(clock), withStrand("strand-1"))
			runDir := filepath.Join(fx.RunRoot, "run-1")
			if err := saveRunState(runDir, RunState{RunID: "run-1", StrandGUID: "strand-1", SessionID: "session-1", EventsPath: fx.EventsPath}); err != nil {
				t.Fatalf("saveRunState: %v", err)
			}
			if err := os.WriteFile(fx.EventsPath, []byte(tt.events), 0o644); err != nil {
				t.Fatalf("seed events: %v", err)
			}
			if tt.pasteEngine {
				if err := savePendingSend(runDir, pendingSend{Text: line}); err != nil {
					t.Fatalf("savePendingSend: %v", err)
				}
			}
			if tt.startsTurn {
				appended := false
				clock.onSleep = func(time.Time) {
					if appended || len(tui.history) == 0 {
						return
					}
					appended = true
					f, err := os.OpenFile(fx.EventsPath, os.O_APPEND|os.O_WRONLY, 0o644)
					if err != nil {
						t.Errorf("open events: %v", err)
						return
					}
					defer f.Close()
					if _, err := f.WriteString("START\n"); err != nil {
						t.Errorf("append turn start: %v", err)
					}
				}
			}

			turnStarted, err := fx.Runner.SendConditioned("strand-1", line, want)

			if tt.wantIs == nil && err != nil {
				t.Fatalf("SendConditioned() error = %v, want nil", err)
			}
			if tt.wantIs != nil && !errors.Is(err, tt.wantIs) {
				t.Fatalf("SendConditioned() error = %v, want it to wrap %v", err, tt.wantIs)
			}
			var busy *SessionBusyError
			if got := errors.As(err, &busy); got != tt.wantBusyError || (got && busy.Draft != tt.wantDraft) {
				t.Errorf("SendConditioned() error = %v, want a *SessionBusyError = %v with Draft %v", err, tt.wantBusyError, tt.wantDraft)
			}
			if turnStarted != tt.wantTurnStarted {
				t.Errorf("turnStarted = %v, want %v", turnStarted, tt.wantTurnStarted)
			}
			typed := len(base.SendTextCalls) + len(base.PasteSubmitCalls)
			if (typed > 0) != tt.wantTyped {
				t.Errorf("typed %d times (%d typed, %d pasted), want typed = %v", typed, len(base.SendTextCalls), len(base.PasteSubmitCalls), tt.wantTyped)
			}
			if tt.pasteEngine && (len(base.PasteSubmitCalls) != 1 || len(base.SendTextCalls) != 0) {
				t.Errorf("paste calls = %+v, typed = %+v; want one paste and no typing", base.PasteSubmitCalls, base.SendTextCalls)
			}
			var keys []string
			for _, call := range base.SendKeyCalls {
				keys = append(keys, call.Key)
			}
			if slices.ContainsFunc(keys, func(key string) bool { return key != "Enter" }) {
				t.Errorf("keys = %v, want nothing but Enter, so no clear and no picker close", keys)
			}
			if (len(keys) > 0) != tt.wantSubmitKeys && !tt.pasteEngine {
				t.Errorf("keys = %v, want Enter sent = %v", keys, tt.wantSubmitKeys)
			}
			if tui.box != tt.wantBox {
				t.Errorf("input box = %q, want %q", tui.box, tt.wantBox)
			}
			if !tt.wantTyped && !clock.Now().Equal(start) {
				t.Errorf("clock advanced %v, want a refusal that waits on nothing", clock.Now().Sub(start))
			}
		})
	}
}
