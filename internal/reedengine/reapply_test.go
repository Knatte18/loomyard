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

// TestReapplyLayout_GuardInheritance pins that reapplyLayout inherits applyLayoutLockedOpts' two
// session-survival guards: fewer than two live panes, and two panes with no strand owning a present
// pane, both issue no select-layout, return Applied: false, BoxIsLive: false, and nil.
func TestReapplyLayout_GuardInheritance(t *testing.T) {
	t.Run("FewerThanTwoLivePanes", func(t *testing.T) {
		e, _ := newReapplyTestEngine(t)
		fake := installFakeTmux(t, e)
		fake.answerSession([]LivePane{{ID: "%1"}}, "100 21", nil)
		fake.mustNotCall("select-layout")

		got, err := e.reapplyLayout(render.Box{}, false)
		if err != nil {
			t.Fatalf("reapplyLayout() error = %v, want nil", err)
		}
		if got.Applied || got.BoxIsLive {
			t.Errorf("reapplyLayout() = %+v, want Applied false and BoxIsLive false", got)
		}
	})

	t.Run("NoStrandOwnsAPresentPane", func(t *testing.T) {
		e := newTestEngine(t)
		st := &ReedState{}
		if err := SaveState(e.stateDir(), st); err != nil {
			t.Fatalf("SaveState: %v", err)
		}
		fake := installFakeTmux(t, e)
		fake.answerSession([]LivePane{{ID: "%1"}, {ID: "%2"}}, "100 21", nil)
		fake.mustNotCall("select-layout")

		got, err := e.reapplyLayout(render.Box{}, false)
		if err != nil {
			t.Fatalf("reapplyLayout() error = %v, want nil", err)
		}
		if got.Applied || got.BoxIsLive {
			t.Errorf("reapplyLayout() = %+v, want Applied false and BoxIsLive false", got)
		}
	})
}

// TestReapplyLayout_FocusIsNeverMoved pins that a successful re-apply issues select-layout and no
// select-pane, even though the persisted strand carries Display.Focus: true.
func TestReapplyLayout_FocusIsNeverMoved(t *testing.T) {
	e, _ := newReapplyTestEngine(t)
	fake := installFakeTmux(t, e)
	fake.answerSession([]LivePane{{ID: "%1"}, {ID: "%2"}}, "80 24", nil)
	fake.mustNotCall("select-pane")

	got, err := e.reapplyLayout(render.Box{X: 0, Y: 0, W: 100, H: 21}, false)
	if err != nil {
		t.Fatalf("reapplyLayout() error = %v, want nil", err)
	}
	if !got.Applied {
		t.Errorf("reapplyLayout() Applied = false, want true")
	}
	if fake.Count("select-layout") == 0 {
		t.Errorf("calls = %v, want select-layout", fake.Sequence())
	}
}

// TestReapplyLayout_Deferral pins that with reed.lock already held, reapplyLayout returns
// ReapplyResult{Deferred: true} and nil, issues no tmux call at all, and reports HookKnown: false.
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

// TestReapplyLayout_BoxEqualityGuard pins that a call whose scripted live box equals lastApplied
// issues no select-layout and returns Applied: false, BoxIsLive: true; a call whose box differs
// applies.
func TestReapplyLayout_BoxEqualityGuard(t *testing.T) {
	t.Run("EqualBoxSkips", func(t *testing.T) {
		e, _ := newReapplyTestEngine(t)
		fake := installFakeTmux(t, e)
		fake.answerSession([]LivePane{{ID: "%1"}, {ID: "%2"}}, "100 21", nil)
		fake.mustNotCall("select-layout")

		lastApplied := render.Box{X: 0, Y: 0, W: 100, H: 21}
		got, err := e.reapplyLayout(lastApplied, false)
		if err != nil {
			t.Fatalf("reapplyLayout() error = %v, want nil", err)
		}
		if got.Applied || !got.BoxIsLive {
			t.Errorf("reapplyLayout() = %+v, want Applied false and BoxIsLive true", got)
		}
	})

	t.Run("DifferingBoxApplies", func(t *testing.T) {
		e, _ := newReapplyTestEngine(t)
		fake := installFakeTmux(t, e)
		fake.answerSession([]LivePane{{ID: "%1"}, {ID: "%2"}}, "80 24", nil)

		lastApplied := render.Box{X: 0, Y: 0, W: 100, H: 21}
		got, err := e.reapplyLayout(lastApplied, false)
		if err != nil {
			t.Fatalf("reapplyLayout() error = %v, want nil", err)
		}
		if !got.Applied || !got.BoxIsLive {
			t.Errorf("reapplyLayout() = %+v, want Applied true and BoxIsLive true", got)
		}
		if fake.Count("select-layout") == 0 {
			t.Errorf("calls = %v, want select-layout", fake.Sequence())
		}
	})
}

// TestReapplyLayout_DegradedBox pins that with display-message scripted to error, reapplyLayout
// returns BoxIsLive: false whether or not the fallback box happens to equal lastApplied, and — in the
// happens-to-equal case — still issues select-layout.
func TestReapplyLayout_DegradedBox(t *testing.T) {
	t.Run("FallbackHappensToEqualLastApplied", func(t *testing.T) {
		e, _ := newReapplyTestEngine(t)
		fake := installFakeTmux(t, e)
		fake.answerSession([]LivePane{{ID: "%1"}, {ID: "%2"}}, "", errors.New("boom"))

		lastApplied := render.Box{X: 0, Y: 0, W: e.cfg.Width, H: e.cfg.Height}
		got, err := e.reapplyLayout(lastApplied, false)
		if err != nil {
			t.Fatalf("reapplyLayout() error = %v, want nil", err)
		}
		if got.BoxIsLive {
			t.Errorf("reapplyLayout() BoxIsLive = true, want false (a fallback box is not an observation)")
		}
		if fake.Count("select-layout") == 0 {
			t.Errorf("calls = %v, want select-layout still issued", fake.Sequence())
		}
	})

	t.Run("FallbackDoesNotEqualLastApplied", func(t *testing.T) {
		e, _ := newReapplyTestEngine(t)
		fake := installFakeTmux(t, e)
		fake.answerSession([]LivePane{{ID: "%1"}, {ID: "%2"}}, "", errors.New("boom"))

		lastApplied := render.Box{X: 0, Y: 0, W: 5, H: 5}
		got, err := e.reapplyLayout(lastApplied, false)
		if err != nil {
			t.Fatalf("reapplyLayout() error = %v, want nil", err)
		}
		if got.BoxIsLive {
			t.Errorf("reapplyLayout() BoxIsLive = true, want false")
		}
		if fake.Count("select-layout") == 0 {
			t.Errorf("calls = %v, want select-layout still issued", fake.Sequence())
		}
	})
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

// TestReapplyLayout_ProbeOrdering pins that with probeHook: true on a session the apply guards skip
// (fewer than two panes), the probe still ran: HookKnown is true and show-options appears in the
// recorded argv.
func TestReapplyLayout_ProbeOrdering(t *testing.T) {
	e, _ := newReapplyTestEngine(t)
	fake := installFakeTmux(t, e)
	fake.answerSession([]LivePane{{ID: "%1"}}, "100 21", nil)

	got, err := e.reapplyLayout(render.Box{}, true)
	if err != nil {
		t.Fatalf("reapplyLayout() error = %v, want nil", err)
	}
	if !got.HookKnown {
		t.Errorf("reapplyLayout() HookKnown = false, want true (the probe must run even when the apply guard skips)")
	}
	if fake.Count("show-options") == 0 {
		t.Errorf("calls = %v, want show-options", fake.Sequence())
	}
}

// TestReapplyLayout_ProbeHookFalseAsksNothing pins that probeHook: false issues no show-options round
// trip at all and returns HookInstalled: false, HookKnown: false — "not asked", indistinguishable
// from a deferral's undecided shape and deliberately so.
func TestReapplyLayout_ProbeHookFalseAsksNothing(t *testing.T) {
	e, _ := newReapplyTestEngine(t)
	fake := installFakeTmux(t, e)
	fake.answerSession([]LivePane{{ID: "%1"}}, "100 21", nil)
	fake.mustNotCall("show-options")

	got, err := e.reapplyLayout(render.Box{}, false)
	if err != nil {
		t.Fatalf("reapplyLayout() error = %v, want nil", err)
	}
	if got.HookInstalled || got.HookKnown {
		t.Errorf("reapplyLayout() hook = (%v, %v), want (false, false)", got.HookInstalled, got.HookKnown)
	}

	// Exactly one show-options round trip on the current GOOS when probeHook is true, the mirror
	// assertion this GOOS-conditional test can make without a build-tagged Windows file.
	probed := installFakeTmux(t, e)
	probed.answerSession([]LivePane{{ID: "%1"}}, "100 21", nil)
	if _, err := e.reapplyLayout(render.Box{}, true); err != nil {
		t.Fatalf("reapplyLayout() error = %v, want nil", err)
	}
	if count := probed.Count("show-options"); count != 1 {
		t.Errorf("show-options round trips = %d, want exactly 1", count)
	}
}

func TestReapplyLayout_PersistsNothing(t *testing.T) {
	e, _ := newReapplyTestEngine(t)
	installFakeTmux(t, e).answerSession([]LivePane{{ID: "%1"}, {ID: "%2"}}, "80 24", nil)

	path := filepath.Join(e.stateDir(), reedStateFileName)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile before: %v", err)
	}

	if _, err := e.reapplyLayout(render.Box{X: 0, Y: 0, W: 100, H: 21}, false); err != nil {
		t.Fatalf("reapplyLayout() error = %v, want nil", err)
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile after: %v", err)
	}
	if string(before) != string(after) {
		t.Errorf("reed.json changed across reapplyLayout: before=%q after=%q", before, after)
	}
}
