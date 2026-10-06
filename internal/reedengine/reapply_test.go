// reapply_test.go pins reapplyLayout's guard inheritance, focus suppression, deferral, box-equality
// guard, degraded-box handling, and the hook probe's exact-match contract and ordering, all driven
// through TmuxCmd's execHook seam — no live tmux server, matching apply_test.go's fixture style.

package reedengine

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/reedengine/render"
	"github.com/Knatte18/loomyard/internal/shell"
)

// newReapplyTestEngine builds an Engine and a persisted ReedState the way apply_test.go's fixtures
// do, ready for reapplyLayout: one strand bound to "%1", live panes "%1" and "%2".
func newReapplyTestEngine(t *testing.T) (*Engine, *ReedState) {
	t.Helper()
	e := newTestEngine(t)
	st := &ReedState{
		Strands: []Strand{
			{GUID: "only", PaneID: "%1", Display: render.Display{Anchor: render.AnchorBelowParent, Focus: true}},
		},
	}
	if err := SaveState(e.stateDir(), st); err != nil {
		t.Fatalf("SaveState: %v", err)
	}
	return e, st
}

// TestReapplyLayout pins reapplyLayout's outcome per scenario, driven through the fake tmux:
//   - it inherits applyLayoutLockedOpts' two session-survival guards (fewer than two live panes, two panes with no strand owning a present pane):
//     no select-layout, Applied false, BoxIsLive false;
//   - a box differing from lastApplied applies with select-layout, never moving focus (no select-pane) even though the strand carries Display.Focus: true;
//   - a live box equal to lastApplied issues no select-layout and reports Applied false, BoxIsLive true;
//   - with display-message erroring, BoxIsLive is false whether or not the fallback box happens to equal lastApplied, and select-layout is still issued;
//   - probeHook: true runs exactly one show-options even when the apply guards skip, probeHook: false asks nothing and reports HookKnown false;
//
// and in every scenario reed.json is left byte-identical.
//
//testtiming:keep pins reapplyLayout's outcome per scenario: both session-survival guards issuing no select-layout, a differing box applying without moving focus, an equal live box skipping, a degraded box never counting as live, the hook probe running once even when the guards skip, and reed.json left byte-identical; its covering tests run this code without asserting it
func TestReapplyLayout(t *testing.T) {
	tests := []struct {
		name             string
		noStrands        bool
		live             []LivePane
		box              string
		boxErr           error
		lastApplied      func(e *Engine) render.Box
		probeHook        bool
		wantApplied      bool
		wantBoxIsLive    bool
		wantHookKnown    bool
		wantSelectLayout bool
		wantShowOptions  int
	}{
		{
			name: "GuardFewerThanTwoLivePanes",
			live: []LivePane{{ID: "%1"}},
			box:  "100 21",
		},
		{
			name:      "GuardNoStrandOwnsAPresentPane",
			noStrands: true,
			live:      []LivePane{{ID: "%1"}, {ID: "%2"}},
			box:       "100 21",
		},
		{
			name:             "DifferingBoxAppliesAndNeverMovesFocus",
			live:             []LivePane{{ID: "%1"}, {ID: "%2"}},
			box:              "80 24",
			lastApplied:      func(*Engine) render.Box { return render.Box{X: 0, Y: 0, W: 100, H: 21} },
			wantApplied:      true,
			wantBoxIsLive:    true,
			wantSelectLayout: true,
		},
		{
			name:          "EqualBoxSkips",
			live:          []LivePane{{ID: "%1"}, {ID: "%2"}},
			box:           "100 21",
			lastApplied:   func(*Engine) render.Box { return render.Box{X: 0, Y: 0, W: 100, H: 21} },
			wantBoxIsLive: true,
		},
		{
			name:   "DegradedBoxFallbackHappensToEqualLastApplied",
			live:   []LivePane{{ID: "%1"}, {ID: "%2"}},
			boxErr: errors.New("boom"),
			lastApplied: func(e *Engine) render.Box {
				return render.Box{X: 0, Y: 0, W: e.cfg.Width, H: e.cfg.Height}
			},
			wantApplied:      true,
			wantSelectLayout: true,
		},
		{
			name:             "DegradedBoxFallbackDiffersFromLastApplied",
			live:             []LivePane{{ID: "%1"}, {ID: "%2"}},
			boxErr:           errors.New("boom"),
			lastApplied:      func(*Engine) render.Box { return render.Box{X: 0, Y: 0, W: 5, H: 5} },
			wantApplied:      true,
			wantSelectLayout: true,
		},
		{
			// The probe must run even when the apply guard skips.
			name:            "ProbeRunsOnceEvenWhenTheApplyGuardSkips",
			live:            []LivePane{{ID: "%1"}},
			box:             "100 21",
			probeHook:       true,
			wantHookKnown:   true,
			wantShowOptions: 1,
		},
		{
			// "Not asked", indistinguishable from a deferral's undecided shape and deliberately so.
			name: "ProbeHookFalseAsksNothing",
			live: []LivePane{{ID: "%1"}},
			box:  "100 21",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e, _ := newReapplyTestEngine(t)
			if tt.noStrands {
				if err := SaveState(e.stateDir(), &ReedState{}); err != nil {
					t.Fatalf("SaveState: %v", err)
				}
			}
			fake := installFakeTmux(t, e)
			fake.answerSession(tt.live, tt.box, tt.boxErr)
			fake.mustNotCall("select-pane")
			var lastApplied render.Box
			if tt.lastApplied != nil {
				lastApplied = tt.lastApplied(e)
			}
			path := filepath.Join(e.stateDir(), reedStateFileName)
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("ReadFile before: %v", err)
			}

			got, err := e.reapplyLayout(lastApplied, tt.probeHook)
			if err != nil {
				t.Fatalf("reapplyLayout() error = %v, want nil", err)
			}

			if got.Applied != tt.wantApplied || got.BoxIsLive != tt.wantBoxIsLive {
				t.Errorf("reapplyLayout() = %+v, want Applied %v and BoxIsLive %v", got, tt.wantApplied, tt.wantBoxIsLive)
			}
			if got.HookKnown != tt.wantHookKnown || got.HookInstalled {
				t.Errorf("reapplyLayout() hook = (%v, %v), want (false, %v)", got.HookInstalled, got.HookKnown, tt.wantHookKnown)
			}
			if issued := fake.Count("select-layout") > 0; issued != tt.wantSelectLayout {
				t.Errorf("select-layout issued = %v, want %v: %v", issued, tt.wantSelectLayout, fake.Sequence())
			}
			if count := fake.Count("show-options"); count != tt.wantShowOptions {
				t.Errorf("show-options round trips = %d, want %d", count, tt.wantShowOptions)
			}
			after, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("ReadFile after: %v", err)
			}
			if string(before) != string(after) {
				t.Errorf("reed.json changed across reapplyLayout: before=%q after=%q", before, after)
			}
		})
	}
}

// TestReapplyLayout_Deferral pins that with reed.lock already held, reapplyLayout returns
// ReapplyResult{Deferred: true} and nil, issues no tmux call at all, and reports HookKnown: false.
//
//testtiming:keep pins reapplyLayout returning exactly ReapplyResult{Deferred: true} with no tmux call while reed.lock is held; its covering tests run this code without asserting it
func TestReapplyLayout_Deferral(t *testing.T) {
	e, _ := newReapplyTestEngine(t)

	dotLyx := e.stateDir()
	if err := os.MkdirAll(dotLyx, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	held, err := lock.AcquireWriteLock(filepath.Join(dotLyx, reedLockFileName))
	if err != nil {
		t.Fatalf("AcquireWriteLock: %v", err)
	}
	defer held.Release()

	fake := installFakeTmux(t, e)
	fake.answerSession([]LivePane{{ID: "%1"}, {ID: "%2"}}, "100 21", nil)

	got, err := e.reapplyLayout(render.Box{}, true)
	if err != nil {
		t.Fatalf("reapplyLayout() error = %v, want nil", err)
	}
	if got != (ReapplyResult{Deferred: true}) {
		t.Errorf("reapplyLayout() = %+v, want ReapplyResult{Deferred: true}", got)
	}
	if calls := fake.Calls(); len(calls) != 0 {
		t.Errorf("calls = %v, want zero tmux calls on a deferral", calls)
	}
}

// TestReapplyLayout_HookProbeExactMatchOnly is the table for probeHook: true, asserting
// (HookInstalled, HookKnown) for every scripted show-options -v answer shape.
// "Some window-resized hook exists" is the wrong test and is what an obvious implementation writes —
// every case here pins the exact-match requirement instead.
//
// The multi-line cases are the ones the probe originally got wrong: show-options -v prints a hook
// ARRAY as one line per entry (live-verified, tmux 3.6), and reed's own array normally carries a
// resize-pane pin per fixed-height pane ahead of the touch — so an answer that merely CONTAINS reed's
// command among other entries is the healthy shape, not the degenerate one, while an answer whose
// line merely embeds that command as a substring is still a miss.
func TestReapplyLayout_HookProbeExactMatchOnly(t *testing.T) {
	e, _ := newReapplyTestEngine(t)
	ownCommand := resizeHookCommand(shell.ForGOOS(), e.resizeSignalPath())
	const pinEntry = `resize-pane -t "%1" -y 1`
	const secondPinEntry = `resize-pane -t "%2" -y 2`

	tests := []struct {
		name          string
		answer        string
		err           error
		wantInstalled bool
		wantKnown     bool
	}{
		{"ExactOwnCommand", ownCommand, nil, true, true},
		{"EmptyNoHookSet", "", nil, false, true},
		{"ForeignWindowResizedHook", "run-shell -b 'echo something-else'", nil, false, true},
		{"OwnShapeDifferentWorktree", "run-shell -b \"sh -c 'touch \\\"/some/other/worktree/.lyx/reed-resize.signal\\\"'\"", nil, false, true},
		{"RoundTripError", "", errors.New("boom"), false, false},
		{"PinsThenOwnCommandLast", pinEntry + "\n" + secondPinEntry + "\n" + ownCommand, nil, true, true},
		{"TrailingNewlineAfterOwnCommand", pinEntry + "\n" + ownCommand + "\n", nil, true, true},
		{"OwnCommandAheadOfAForeignEntry", ownCommand + "\nrun-shell -b 'echo something-else'", nil, true, true},
		{"PinsOnlyNoOwnCommand", pinEntry + "\n" + secondPinEntry, nil, false, true},
		{"PinsAndAnotherWorktreesTouch", pinEntry + "\nrun-shell -b \"sh -c 'touch \\\"/some/other/worktree/.lyx/reed-resize.signal\\\"'\"", nil, false, true},
		{"OwnCommandEmbeddedInALongerEntry", pinEntry + "\nif-shell true '" + ownCommand + "'", nil, false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := installFakeTmux(t, e)
			fake.answerSession([]LivePane{{ID: "%1"}, {ID: "%2"}}, "100 21", nil)
			fake.answer("show-options", tt.answer, tt.err)

			got, err := e.reapplyLayout(render.Box{X: 0, Y: 0, W: 100, H: 21}, true)
			if err != nil {
				t.Fatalf("reapplyLayout() error = %v, want nil", err)
			}
			if got.HookInstalled != tt.wantInstalled || got.HookKnown != tt.wantKnown {
				t.Errorf("reapplyLayout() hook = (%v, %v), want (%v, %v)", got.HookInstalled, got.HookKnown, tt.wantInstalled, tt.wantKnown)
			}
		})
	}
}
