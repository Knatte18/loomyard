// reapply_test.go pins reapplyLayout's guard inheritance, focus suppression, deferral, box-equality
// guard, degraded-box handling, and the hook probe's exact-match contract and ordering, all driven
// through TmuxCmd's execHook seam — no live tmux server, matching apply_test.go's fixture style.

package reedengine

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
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
	e.cfg.Width, e.cfg.Height = 100, 21
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

// scriptedHook returns an execHook that records every subcommand's argv and answers has-session,
// list-panes, and display-message from the given fixtures, deferring everything else to a no-op
// success.
func scriptedHook(calls *[][]string, live []LivePane, boxAnswer string, boxErr error) func(bool, ...string) (string, error) {
	return func(capture bool, args ...string) (string, error) {
		*calls = append(*calls, append([]string{}, args...))
		switch args[0] {
		case "has-session":
			return "", nil
		case "list-panes":
			return encodeLivePanes(live), nil
		case "display-message":
			// Disambiguate by the trailing format string: the generation probe (generation.go) and
			// the window-size query (windowsize.go) both go through display-message, and only the
			// latter is under test here — a scripted generation stamp keeps adoptPaneGenerationLocked
			// quiet rather than warning on every call.
			if args[len(args)-1] == paneGenerationFormat {
				return "$0|1|1000", nil
			}
			return boxAnswer, boxErr
		default:
			return "", nil
		}
	}
}

// encodeLivePanes renders live back into list-panes' own six-field wire format, matching
// overlay.go's listPanes/parsePaneList round trip.
func encodeLivePanes(live []LivePane) string {
	out := ""
	for _, p := range live {
		dead := "0"
		if p.Dead {
			dead = "1"
		}
		out += p.ID + " " + dead + " " + strconv.Itoa(p.Top) + " " + strconv.Itoa(p.Width) + " " + strconv.Itoa(p.Height) + " " + strconv.Itoa(p.PID) + "\n"
	}
	return out
}

func hasArg(calls [][]string, subcommand string) bool {
	for _, c := range calls {
		if len(c) > 0 && c[0] == subcommand {
			return true
		}
	}
	return false
}

// TestReapplyLayout_GuardInheritance pins that reapplyLayout inherits applyLayoutLockedOpts' two
// session-survival guards: fewer than two live panes, and two panes with no strand owning a present
// pane, both issue no select-layout, return Applied: false, BoxIsLive: false, and nil.
func TestReapplyLayout_GuardInheritance(t *testing.T) {
	t.Run("FewerThanTwoLivePanes", func(t *testing.T) {
		e, _ := newReapplyTestEngine(t)
		var calls [][]string
		e.tmux.execHook = scriptedHook(&calls, []LivePane{{ID: "%1"}}, "100 21", nil)

		got, err := e.reapplyLayout(render.Box{}, false)
		if err != nil {
			t.Fatalf("reapplyLayout() error = %v, want nil", err)
		}
		if got.Applied || got.BoxIsLive {
			t.Errorf("reapplyLayout() = %+v, want Applied false and BoxIsLive false", got)
		}
		if hasArg(calls, "select-layout") {
			t.Errorf("calls = %v, want no select-layout", calls)
		}
	})

	t.Run("NoStrandOwnsAPresentPane", func(t *testing.T) {
		e := newTestEngine(t)
		e.cfg.Width, e.cfg.Height = 100, 21
		st := &ReedState{}
		if err := SaveState(e.stateDir(), st); err != nil {
			t.Fatalf("SaveState: %v", err)
		}
		var calls [][]string
		e.tmux.execHook = scriptedHook(&calls, []LivePane{{ID: "%1"}, {ID: "%2"}}, "100 21", nil)

		got, err := e.reapplyLayout(render.Box{}, false)
		if err != nil {
			t.Fatalf("reapplyLayout() error = %v, want nil", err)
		}
		if got.Applied || got.BoxIsLive {
			t.Errorf("reapplyLayout() = %+v, want Applied false and BoxIsLive false", got)
		}
		if hasArg(calls, "select-layout") {
			t.Errorf("calls = %v, want no select-layout", calls)
		}
	})
}

// TestReapplyLayout_FocusIsNeverMoved pins that a successful re-apply issues select-layout and no
// select-pane, even though the persisted strand carries Display.Focus: true.
func TestReapplyLayout_FocusIsNeverMoved(t *testing.T) {
	e, _ := newReapplyTestEngine(t)
	var calls [][]string
	e.tmux.execHook = scriptedHook(&calls, []LivePane{{ID: "%1"}, {ID: "%2"}}, "80 24", nil)

	got, err := e.reapplyLayout(render.Box{X: 0, Y: 0, W: 100, H: 21}, false)
	if err != nil {
		t.Fatalf("reapplyLayout() error = %v, want nil", err)
	}
	if !got.Applied {
		t.Errorf("reapplyLayout() Applied = false, want true")
	}
	if !hasArg(calls, "select-layout") {
		t.Errorf("calls = %v, want select-layout", calls)
	}
	if hasArg(calls, "select-pane") {
		t.Errorf("calls = %v, want no select-pane", calls)
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

	var calls [][]string
	e.tmux.execHook = scriptedHook(&calls, []LivePane{{ID: "%1"}, {ID: "%2"}}, "100 21", nil)

	got, err := e.reapplyLayout(render.Box{}, true)
	if err != nil {
		t.Fatalf("reapplyLayout() error = %v, want nil", err)
	}
	if got != (ReapplyResult{Deferred: true}) {
		t.Errorf("reapplyLayout() = %+v, want ReapplyResult{Deferred: true}", got)
	}
	if len(calls) != 0 {
		t.Errorf("calls = %v, want zero tmux calls on a deferral", calls)
	}
}

// TestReapplyLayout_BoxEqualityGuard pins that a call whose scripted live box equals lastApplied
// issues no select-layout and returns Applied: false, BoxIsLive: true; a call whose box differs
// applies.
func TestReapplyLayout_BoxEqualityGuard(t *testing.T) {
	t.Run("EqualBoxSkips", func(t *testing.T) {
		e, _ := newReapplyTestEngine(t)
		var calls [][]string
		e.tmux.execHook = scriptedHook(&calls, []LivePane{{ID: "%1"}, {ID: "%2"}}, "100 21", nil)

		lastApplied := render.Box{X: 0, Y: 0, W: 100, H: 21}
		got, err := e.reapplyLayout(lastApplied, false)
		if err != nil {
			t.Fatalf("reapplyLayout() error = %v, want nil", err)
		}
		if got.Applied || !got.BoxIsLive {
			t.Errorf("reapplyLayout() = %+v, want Applied false and BoxIsLive true", got)
		}
		if hasArg(calls, "select-layout") {
			t.Errorf("calls = %v, want no select-layout", calls)
		}
	})

	t.Run("DifferingBoxApplies", func(t *testing.T) {
		e, _ := newReapplyTestEngine(t)
		var calls [][]string
		e.tmux.execHook = scriptedHook(&calls, []LivePane{{ID: "%1"}, {ID: "%2"}}, "80 24", nil)

		lastApplied := render.Box{X: 0, Y: 0, W: 100, H: 21}
		got, err := e.reapplyLayout(lastApplied, false)
		if err != nil {
			t.Fatalf("reapplyLayout() error = %v, want nil", err)
		}
		if !got.Applied || !got.BoxIsLive {
			t.Errorf("reapplyLayout() = %+v, want Applied true and BoxIsLive true", got)
		}
		if !hasArg(calls, "select-layout") {
			t.Errorf("calls = %v, want select-layout", calls)
		}
	})
}

// TestReapplyLayout_DegradedBox pins that with display-message scripted to error, reapplyLayout
// returns BoxIsLive: false whether or not the fallback box happens to equal lastApplied, and — in the
// happens-to-equal case — still issues select-layout.
func TestReapplyLayout_DegradedBox(t *testing.T) {
	t.Run("FallbackHappensToEqualLastApplied", func(t *testing.T) {
		e, _ := newReapplyTestEngine(t)
		var calls [][]string
		e.tmux.execHook = scriptedHook(&calls, []LivePane{{ID: "%1"}, {ID: "%2"}}, "", errors.New("boom"))

		lastApplied := render.Box{X: 0, Y: 0, W: e.cfg.Width, H: e.cfg.Height}
		got, err := e.reapplyLayout(lastApplied, false)
		if err != nil {
			t.Fatalf("reapplyLayout() error = %v, want nil", err)
		}
		if got.BoxIsLive {
			t.Errorf("reapplyLayout() BoxIsLive = true, want false (a fallback box is not an observation)")
		}
		if !hasArg(calls, "select-layout") {
			t.Errorf("calls = %v, want select-layout still issued", calls)
		}
	})

	t.Run("FallbackDoesNotEqualLastApplied", func(t *testing.T) {
		e, _ := newReapplyTestEngine(t)
		var calls [][]string
		e.tmux.execHook = scriptedHook(&calls, []LivePane{{ID: "%1"}, {ID: "%2"}}, "", errors.New("boom"))

		lastApplied := render.Box{X: 0, Y: 0, W: 5, H: 5}
		got, err := e.reapplyLayout(lastApplied, false)
		if err != nil {
			t.Fatalf("reapplyLayout() error = %v, want nil", err)
		}
		if got.BoxIsLive {
			t.Errorf("reapplyLayout() BoxIsLive = true, want false")
		}
		if !hasArg(calls, "select-layout") {
			t.Errorf("calls = %v, want select-layout still issued", calls)
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
			var calls [][]string
			hook := scriptedHook(&calls, []LivePane{{ID: "%1"}, {ID: "%2"}}, "100 21", nil)
			e.tmux.execHook = func(capture bool, args ...string) (string, error) {
				if args[0] == "show-options" {
					calls = append(calls, append([]string{}, args...))
					return tt.answer, tt.err
				}
				return hook(capture, args...)
			}

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
	var calls [][]string
	e.tmux.execHook = scriptedHook(&calls, []LivePane{{ID: "%1"}}, "100 21", nil)

	got, err := e.reapplyLayout(render.Box{}, true)
	if err != nil {
		t.Fatalf("reapplyLayout() error = %v, want nil", err)
	}
	if !got.HookKnown {
		t.Errorf("reapplyLayout() HookKnown = false, want true (the probe must run even when the apply guard skips)")
	}
	if !hasArg(calls, "show-options") {
		t.Errorf("calls = %v, want show-options", calls)
	}
}

// TestReapplyLayout_ProbeHookFalseAsksNothing pins that probeHook: false issues no show-options round
// trip at all and returns HookInstalled: false, HookKnown: false — "not asked", indistinguishable
// from a deferral's undecided shape and deliberately so.
func TestReapplyLayout_ProbeHookFalseAsksNothing(t *testing.T) {
	e, _ := newReapplyTestEngine(t)
	var calls [][]string
	e.tmux.execHook = scriptedHook(&calls, []LivePane{{ID: "%1"}}, "100 21", nil)

	got, err := e.reapplyLayout(render.Box{}, false)
	if err != nil {
		t.Fatalf("reapplyLayout() error = %v, want nil", err)
	}
	if hasArg(calls, "show-options") {
		t.Errorf("calls = %v, want no show-options round trip", calls)
	}
	if got.HookInstalled || got.HookKnown {
		t.Errorf("reapplyLayout() hook = (%v, %v), want (false, false)", got.HookInstalled, got.HookKnown)
	}

	// Exactly one show-options round trip on the current GOOS when probeHook is true, the mirror
	// assertion this GOOS-conditional test can make without a build-tagged Windows file.
	var probedCalls [][]string
	e.tmux.execHook = scriptedHook(&probedCalls, []LivePane{{ID: "%1"}}, "100 21", nil)
	if _, err := e.reapplyLayout(render.Box{}, true); err != nil {
		t.Fatalf("reapplyLayout() error = %v, want nil", err)
	}
	count := 0
	for _, c := range probedCalls {
		if c[0] == "show-options" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("show-options round trips = %d, want exactly 1", count)
	}
}

// TestReapplyLayout_PersistsNothing pins that a successful re-apply leaves reed.json's bytes
// byte-identical to what they were before the call.
func TestReapplyLayout_PersistsNothing(t *testing.T) {
	e, _ := newReapplyTestEngine(t)
	e.tmux.execHook = scriptedHook(new([][]string), []LivePane{{ID: "%1"}, {ID: "%2"}}, "80 24", nil)

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
