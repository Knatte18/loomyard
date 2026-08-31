// attach_test.go pins AttachArgv's argv shape, its told-box source, and every one of its skip
// guards. Every guard exists to prevent the session-wipe hazard anyPlacedStrand and the len(live) < 2
// guard document in apply.go: a layout string enumerating zero panes is accepted by tmux (exit 0) and
// answered by destroying every pane in the session, so an attach that reaches select-layout on a
// suppressed precondition would wipe the very session it is attaching to.
//
// Every case drives AttachArgv entirely through TmuxCmd's execHook seam — no external process spawn,
// no live tmux server, no sleep — discriminating display-message responses on the format argument
// (the last element of args), never on call order, so these tests do not silently pass if the call
// sequence changes.

package reedengine

import (
	"errors"
	"testing"

	"github.com/Knatte18/loomyard/internal/reedengine/render"
)

// TestParseClientList mirrors TestParseWindowSize's shape for the sibling parser: a table of
// `list-clients` answer shapes, asserting the parsed attachedClient slice element by element.
func TestParseClientList(t *testing.T) {
	tests := []struct {
		name string
		out  string
		want []attachedClient
	}{
		{"Empty", "", []attachedClient{}},
		{"OneClient", "tty0 80 24", []attachedClient{{Name: "tty0", Width: 80, Height: 24}}},
		{
			"SeveralClients",
			"tty0 80 24\ntty1 100 40",
			[]attachedClient{{Name: "tty0", Width: 80, Height: 24}, {Name: "tty1", Width: 100, Height: 40}},
		},
		{
			"MalformedLineAmongWellFormed",
			"tty0 80 24\ngarbage\ntty1 100 40",
			[]attachedClient{{Name: "tty0", Width: 80, Height: 24}, {Name: "tty1", Width: 100, Height: 40}},
		},
		{"TrailingWhitespace", "tty0 80 24\n", []attachedClient{{Name: "tty0", Width: 80, Height: 24}}},
		{"ZeroSizeField", "tty0 0 24", []attachedClient{}},
		{"NegativeSizeField", "tty0 80 -1", []attachedClient{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseClientList(tt.out)
			if len(got) != len(tt.want) {
				t.Fatalf("parseClientList(%q) = %v, want %v", tt.out, got, tt.want)
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Errorf("parseClientList(%q)[%d] = %+v, want %+v", tt.out, i, got[i], tt.want[i])
				}
			}
		})
	}
}

// attachScript describes one scripted tmux round-trip set for AttachArgv: the has-session check, the
// two effective-value readbacks, and the pane list. A zero-value field means "answer success with an
// empty/zero value" except where a *Err field is set, which always takes priority for that call.
type attachScript struct {
	hasSessionErr error

	windowSize    string
	windowSizeErr error

	status    string
	statusErr error

	listPanes    string
	listPanesErr error
}

// attachRecorder captures every call AttachArgv's pre-flight makes through the execHook seam, so a
// test can assert both what was called and, where order matters (the pins vs. the status readback),
// the sequence it happened in.
type attachRecorder struct {
	sequence          []string
	setOptionCalls    [][]string
	mutationCalls     [][]string
	setHookCalls      [][]string
	windowSizeQueried bool
	liveBoxQueried    bool
}

// goodAttachScript is the fully-permissive script every degraded-path test starts from and mutates
// exactly one field of, so each test isolates the single guard it exists to pin.
func goodAttachScript() attachScript {
	return attachScript{
		windowSize: "latest",
		status:     "off",
		listPanes:  "%1 0 0 40 20 4321\n%2 0 20 40 20 4322\n",
	}
}

// goodAttachLive and goodAttachStrands are the pure-Go mirrors of goodAttachScript's listPanes string
// and the state this file's tests persist via SaveState, used to independently compute the expected
// planLayout output for comparison, rather than re-deriving it from the same code path under test.
func goodAttachLive() []LivePane {
	return []LivePane{
		{ID: "%1", Dead: false, Top: 0, Width: 40, Height: 20, PID: 4321},
		{ID: "%2", Dead: false, Top: 20, Width: 40, Height: 20, PID: 4322},
	}
}

func goodAttachStrands() []Strand {
	return []Strand{
		{GUID: "a", PaneID: "%1", Display: render.Display{Anchor: render.AnchorBelowParent}},
	}
}

// newAttachHook builds the execHook closure a test installs on e.tmux, answering every round trip
// AttachArgv's pre-flight can issue (has-session, the two geometry pins, both display-message
// readbacks, list-panes, and the generation probe loadOrInitStateLocked always runs) and recording
// every call into rec.
func newAttachHook(script attachScript, rec *attachRecorder) func(capture bool, args ...string) (string, error) {
	return func(capture bool, args ...string) (string, error) {
		switch args[0] {
		case "has-session":
			rec.sequence = append(rec.sequence, "has-session")
			return "", script.hasSessionErr
		case "set-option":
			call := append([]string{}, args...)
			rec.setOptionCalls = append(rec.setOptionCalls, call)
			option := call[len(call)-2]
			rec.sequence = append(rec.sequence, "set-option:"+option)
			return "", nil
		case "display-message":
			format := args[len(args)-1]
			switch format {
			case "#{window-size}":
				rec.windowSizeQueried = true
				rec.sequence = append(rec.sequence, "display-message:window-size")
				return script.windowSize, script.windowSizeErr
			case "#{status}":
				rec.sequence = append(rec.sequence, "display-message:status")
				return script.status, script.statusErr
			case "#{window_width} #{window_height}":
				rec.liveBoxQueried = true
				return "", errors.New("AttachArgv must never query the live window size")
			default:
				// The pane-generation probe (loadOrInitStateLocked ->
				// adoptPaneGenerationLocked) spends its own three-field format here.
				// Answering it well-formed keeps this hermetic fixture from
				// spuriously clearing pane bindings via the probe's fail-open path.
				return "$0|4321|1700000000", nil
			}
		case "list-panes":
			rec.sequence = append(rec.sequence, "list-panes")
			return script.listPanes, script.listPanesErr
		case "select-layout", "select-pane", "kill-pane", "split-window":
			rec.mutationCalls = append(rec.mutationCalls, append([]string{}, args...))
			return "", nil
		case "set-hook":
			call := append([]string{}, args...)
			rec.setHookCalls = append(rec.setHookCalls, call)
			rec.sequence = append(rec.sequence, "set-hook")
			return "", nil
		default:
			return "", nil
		}
	}
}

// newAttachTestEngine builds a fixture engine with strands persisted to disk (loadOrInitStateLocked
// reads reed.json from disk, not from an in-memory struct) and its execHook wired to script, returning
// the engine and the recorder the hook writes into.
func newAttachTestEngine(t *testing.T, script attachScript, strands []Strand) (*Engine, *attachRecorder) {
	t.Helper()
	e := newTestEngine(t)
	if err := SaveState(e.stateDir(), &ReedState{Strands: strands}); err != nil {
		t.Fatalf("SaveState: %v", err)
	}
	rec := &attachRecorder{}
	e.tmux.execHook = newAttachHook(script, rec)
	return e, rec
}

// wantBareAttachArgv builds the expected five-element degraded argv for e, asserted element by
// element rather than by length alone: this is where the two deleted CLI-side TestAttachArgv tests'
// pinned "-L <socket> attach-session -t =<session>" expectation lands, so it is not lost with them.
func wantBareAttachArgv(e *Engine) []string {
	return []string{"-L", e.Socket(), "attach-session", "-t", "=" + e.SessionName()}
}

func assertBareArgv(t *testing.T, e *Engine, got []string) {
	t.Helper()
	want := wantBareAttachArgv(e)
	if len(got) != len(want) {
		t.Fatalf("AttachArgv() = %v (len %d), want %v (len %d)", got, len(got), want, len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("AttachArgv()[%d] = %q, want %q (full: got=%v want=%v)", i, got[i], want[i], got, want)
		}
	}
}

// TestAttachArgv_ChainedShape pins the full ten-element argv shape on a known-good pre-flight: five
// bare elements, the one-character ";" separator (length-checked so "\\;" cannot pass), then
// select-layout/-t/target, then the layout string planLayout itself would produce for the same box.
func TestAttachArgv_ChainedShape(t *testing.T) {
	e, _ := newAttachTestEngine(t, goodAttachScript(), goodAttachStrands())
	const cols, rows = 80, 24

	got := e.AttachArgv(cols, rows)

	if len(got) != 10 {
		t.Fatalf("AttachArgv() = %v, want 10 elements", got)
	}
	bare := wantBareAttachArgv(e)
	for i := range bare {
		if got[i] != bare[i] {
			t.Errorf("AttachArgv()[%d] = %q, want bare element %q", i, got[i], bare[i])
		}
	}
	if len(got[5]) != 1 || got[5] != ";" {
		t.Errorf("AttachArgv()[5] = %q, want the literal one-character \";\" separator", got[5])
	}
	target := exactSessionWindowTarget(e.SessionName())
	wantTail := []string{"select-layout", "-t", target}
	for i, want := range wantTail {
		if got[6+i] != want {
			t.Errorf("AttachArgv()[%d] = %q, want %q", 6+i, got[6+i], want)
		}
	}

	wantLayout, _, err := e.planLayout(&ReedState{Strands: goodAttachStrands()}, goodAttachLive(), render.Box{X: 0, Y: 0, W: cols, H: rows})
	if err != nil {
		t.Fatalf("planLayout() unexpected error: %v", err)
	}
	if got[9] != wantLayout {
		t.Errorf("AttachArgv()[9] = %q, want the planned layout %q", got[9], wantLayout)
	}
}

// TestAttachArgv_ToldBoxAndNoLiveQuery pins the told-box seam: the box AttachArgv plans against comes
// from the client's told cols/rows, never from a live display-message query, even when the configured
// e.cfg.Width/Height is a different pair.
func TestAttachArgv_ToldBoxAndNoLiveQuery(t *testing.T) {
	e, rec := newAttachTestEngine(t, goodAttachScript(), goodAttachStrands())
	e.cfg.Width, e.cfg.Height = 999, 111 // deliberately distinct from the client size below
	const cols, rows = 80, 24

	got := e.AttachArgv(cols, rows)

	if rec.liveBoxQueried {
		t.Fatal("AttachArgv() queried the live #{window_width} #{window_height} pair; want zero live-box round trips")
	}

	wantLayout, _, err := e.planLayout(&ReedState{Strands: goodAttachStrands()}, goodAttachLive(), render.Box{X: 0, Y: 0, W: cols, H: rows})
	if err != nil {
		t.Fatalf("planLayout() unexpected error: %v", err)
	}
	if len(got) != 10 || got[9] != wantLayout {
		t.Fatalf("AttachArgv() = %v, want a chained argv whose layout is %q (the client box, not the configured %dx%d)", got, wantLayout, e.cfg.Width, e.cfg.Height)
	}
}

// TestAttachArgv_ReservedRows pins the #{status} readback as the reserved-row source: off reserves
// zero rows, on reserves one, and a non-negative integer string reserves exactly that many.
func TestAttachArgv_ReservedRows(t *testing.T) {
	tests := []struct {
		name     string
		status   string
		reserved int
	}{
		{"Off_ReservesZero", "off", 0},
		{"On_ReservesOne", "on", 1},
		{"NumericTwo_ReservesTwo", "2", 2},
	}
	const cols, rows = 80, 24
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			script := goodAttachScript()
			script.status = tt.status
			e, _ := newAttachTestEngine(t, script, goodAttachStrands())

			got := e.AttachArgv(cols, rows)

			wantLayout, _, err := e.planLayout(&ReedState{Strands: goodAttachStrands()}, goodAttachLive(), render.Box{X: 0, Y: 0, W: cols, H: rows - tt.reserved})
			if err != nil {
				t.Fatalf("planLayout() unexpected error: %v", err)
			}
			if len(got) != 10 || got[9] != wantLayout {
				t.Fatalf("AttachArgv() with #{status}=%q = %v, want the chain planned for %d reserved rows (layout %q)", tt.status, got, tt.reserved, wantLayout)
			}
		})
	}
}

// TestAttachArgv_ReservedRowsFloor pins the reserved-row floor: a #{status} readback large enough
// relative to rows (e.g. a multi-line status bar) must not drive the planned box height to zero or
// negative. reserved is clamped to rows-1 before the box is built, so the chain still plans a
// one-row-remaining box rather than handing planLayout/render.Rules a non-positive height.
func TestAttachArgv_ReservedRowsFloor(t *testing.T) {
	script := goodAttachScript()
	script.status = "30"
	const cols, rows = 80, 24
	e, _ := newAttachTestEngine(t, script, goodAttachStrands())

	got := e.AttachArgv(cols, rows)

	const wantReserved = rows - 1
	wantLayout, _, err := e.planLayout(&ReedState{Strands: goodAttachStrands()}, goodAttachLive(), render.Box{X: 0, Y: 0, W: cols, H: rows - wantReserved})
	if err != nil {
		t.Fatalf("planLayout() unexpected error: %v", err)
	}
	if len(got) != 10 || got[9] != wantLayout {
		t.Fatalf("AttachArgv() with #{status}=%q (rows=%d) = %v, want reserved floored to %d (layout %q)", script.status, rows, got, wantReserved, wantLayout)
	}
}

// TestAttachArgv_ChainGate pins readback-not-exit-status-gates-the-chain: only #{window-size} and an
// unrecognised #{status} suppress the chain; a recognised #{status} other than "off" is an input to
// the reserved-row count, never a gate.
func TestAttachArgv_ChainGate(t *testing.T) {
	tests := []struct {
		name     string
		mutate   func(*attachScript)
		wantBare bool
	}{
		{"WindowSize_Manual_Suppresses", func(s *attachScript) { s.windowSize = "manual" }, true},
		{"WindowSize_Largest_Suppresses", func(s *attachScript) { s.windowSize = "largest" }, true},
		{"WindowSize_Garbage_Suppresses", func(s *attachScript) { s.windowSize = "garbage" }, true},
		{"WindowSize_Error_Suppresses", func(s *attachScript) { s.windowSizeErr = errors.New("boom") }, true},
		{"Status_Garbage_Suppresses", func(s *attachScript) { s.status = "garbage" }, true},
		{"Status_Error_Suppresses", func(s *attachScript) { s.statusErr = errors.New("boom") }, true},
		{"Status_On_DoesNotSuppress", func(s *attachScript) { s.status = "on" }, false},
	}
	const cols, rows = 80, 24
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			script := goodAttachScript()
			tt.mutate(&script)
			e, _ := newAttachTestEngine(t, script, goodAttachStrands())

			got := e.AttachArgv(cols, rows)

			if tt.wantBare {
				assertBareArgv(t, e, got)
				return
			}
			if len(got) != 10 {
				t.Fatalf("AttachArgv() = %v, want the 10-element chained argv (this case must not suppress)", got)
			}
		})
	}
}

// TestAttachArgv_EveryOtherDegradedPathYieldsBareArgv covers every remaining refusal/skip path:
// a non-positive client size, has-session failing, fewer than two live panes, no strand owning a
// present pane, a list-panes error, and a plan error. Every one must yield exactly the bare argv,
// asserted element by element.
func TestAttachArgv_EveryOtherDegradedPathYieldsBareArgv(t *testing.T) {
	t.Run("ZeroCols", func(t *testing.T) {
		e, _ := newAttachTestEngine(t, goodAttachScript(), goodAttachStrands())
		assertBareArgv(t, e, e.AttachArgv(0, 24))
	})
	t.Run("NegativeCols", func(t *testing.T) {
		e, _ := newAttachTestEngine(t, goodAttachScript(), goodAttachStrands())
		assertBareArgv(t, e, e.AttachArgv(-1, 24))
	})
	t.Run("ZeroRows", func(t *testing.T) {
		e, _ := newAttachTestEngine(t, goodAttachScript(), goodAttachStrands())
		assertBareArgv(t, e, e.AttachArgv(80, 0))
	})
	t.Run("NegativeRows", func(t *testing.T) {
		e, _ := newAttachTestEngine(t, goodAttachScript(), goodAttachStrands())
		assertBareArgv(t, e, e.AttachArgv(80, -1))
	})
	t.Run("HasSessionFails", func(t *testing.T) {
		script := goodAttachScript()
		script.hasSessionErr = errors.New("boom")
		e, _ := newAttachTestEngine(t, script, goodAttachStrands())
		assertBareArgv(t, e, e.AttachArgv(80, 24))
	})
	t.Run("FewerThanTwoLivePanes", func(t *testing.T) {
		script := goodAttachScript()
		script.listPanes = "%1 0 0 40 20 4321\n"
		e, _ := newAttachTestEngine(t, script, goodAttachStrands())
		assertBareArgv(t, e, e.AttachArgv(80, 24))
	})
	t.Run("NoStrandOwnsAPresentPane", func(t *testing.T) {
		e, _ := newAttachTestEngine(t, goodAttachScript(), nil)
		assertBareArgv(t, e, e.AttachArgv(80, 24))
	})
	t.Run("ListPanesErrors", func(t *testing.T) {
		script := goodAttachScript()
		script.listPanesErr = errors.New("boom")
		e, _ := newAttachTestEngine(t, script, goodAttachStrands())
		assertBareArgv(t, e, e.AttachArgv(80, 24))
	})
	t.Run("PlanError_DeferredAnchorRejected", func(t *testing.T) {
		strands := []Strand{{GUID: "a", PaneID: "%1", Display: render.Display{Anchor: render.AnchorOwnWindow}}}
		e, _ := newAttachTestEngine(t, goodAttachScript(), strands)
		assertBareArgv(t, e, e.AttachArgv(80, 24))
	})
}

// TestAttachArgv_PinsMadeByBuilderBeforeStatusReadback pins that AttachArgv itself issues both
// geometry pins — not a second exported call the CLI has to remember — and that the status-off pin
// precedes the #{status} readback, the ordering the told box depends on (pinGeometryOptionsLocked's
// doc comment: the told box is only correct once status off has landed).
func TestAttachArgv_PinsMadeByBuilderBeforeStatusReadback(t *testing.T) {
	e, rec := newAttachTestEngine(t, goodAttachScript(), goodAttachStrands())

	got := e.AttachArgv(80, 24)
	if len(got) != 10 {
		t.Fatalf("AttachArgv() = %v, want the 10-element chained argv on this known-good script", got)
	}

	if len(rec.setOptionCalls) != 2 {
		t.Fatalf("AttachArgv() issued %d set-option calls, want 2: %v", len(rec.setOptionCalls), rec.setOptionCalls)
	}

	statusPinIdx, statusReadbackIdx := -1, -1
	for i, step := range rec.sequence {
		if step == "set-option:status" && statusPinIdx == -1 {
			statusPinIdx = i
		}
		if step == "display-message:status" && statusReadbackIdx == -1 {
			statusReadbackIdx = i
		}
	}
	if statusPinIdx == -1 || statusReadbackIdx == -1 {
		t.Fatalf("sequence = %v, want both a status pin and a status readback", rec.sequence)
	}
	if statusPinIdx >= statusReadbackIdx {
		t.Errorf("sequence = %v, want the status-off pin (index %d) before the #{status} readback (index %d)", rec.sequence, statusPinIdx, statusReadbackIdx)
	}
}

// TestAttachArgv_NeverMutatesTheSessionOrPersistsState pins that AttachArgv issues no pane-set
// mutation: no select-layout, select-pane, kill-pane, or split-window is ever issued (the chain
// carries select-layout only inside the returned ARGV, never applies it), and reed.json is neither
// created nor modified by the call. AttachArgv deliberately does mutate a window OPTION now — the
// resize-pin hook, alongside the two geometry pins it already set — so "never mutates" is scoped to
// the pane set, not to every tmux call this builder makes.
func TestAttachArgv_NeverMutatesTheSessionOrPersistsState(t *testing.T) {
	e, rec := newAttachTestEngine(t, goodAttachScript(), goodAttachStrands())

	before, err := LoadState(e.stateDir())
	if err != nil {
		t.Fatalf("LoadState before AttachArgv: %v", err)
	}

	if got := e.AttachArgv(80, 24); len(got) != 10 {
		t.Fatalf("AttachArgv() = %v, want the 10-element chained argv on this known-good script", got)
	}

	if len(rec.mutationCalls) != 0 {
		t.Errorf("AttachArgv() issued mutating tmux calls %v, want none", rec.mutationCalls)
	}

	after, err := LoadState(e.stateDir())
	if err != nil {
		t.Fatalf("LoadState after AttachArgv: %v", err)
	}
	if len(after.Strands) != len(before.Strands) || after.HeaderPaneID != before.HeaderPaneID {
		t.Errorf("reed.json changed across AttachArgv: before=%+v after=%+v", before, after)
	}
}

// TestAttachArgv_InstallsResizePinsAfterStateAndPanesRead pins the install statement's position in
// AttachArgv's pre-flight: a known-good pre-flight issues the set-hook clear (and pin rebuild) after
// the state and pane list are read, and before the argv is returned.
func TestAttachArgv_InstallsResizePinsAfterStateAndPanesRead(t *testing.T) {
	e, rec := newAttachTestEngine(t, goodAttachScript(), goodAttachStrands())

	got := e.AttachArgv(80, 24)
	if len(got) != 10 {
		t.Fatalf("AttachArgv() = %v, want the 10-element chained argv on this known-good script", got)
	}

	listPanesIdx, firstSetHookIdx := -1, -1
	for i, step := range rec.sequence {
		if step == "list-panes" && listPanesIdx == -1 {
			listPanesIdx = i
		}
		if step == "set-hook" && firstSetHookIdx == -1 {
			firstSetHookIdx = i
		}
	}
	if listPanesIdx == -1 {
		t.Fatalf("sequence = %v, want a list-panes call", rec.sequence)
	}
	if firstSetHookIdx == -1 {
		t.Fatalf("sequence = %v, want at least one set-hook call", rec.sequence)
	}
	if firstSetHookIdx <= listPanesIdx {
		t.Errorf("sequence = %v, want the first set-hook call (index %d) after list-panes (index %d)", rec.sequence, firstSetHookIdx, listPanesIdx)
	}
	if len(rec.setHookCalls) == 0 {
		t.Fatal("no set-hook calls recorded, want at least the clear")
	}
	if !containsArg(rec.setHookCalls[0], "-u") {
		t.Errorf("first set-hook argv = %v, want the -u clear", rec.setHookCalls[0])
	}
}

// TestAttachArgv_DegradedPathsInstallNoResizePinHook pins that every degraded path yielding the bare
// argv issues no set-hook call at all — the guard-skip disposition
// install-points-are-two-named-statements-no-guard-moves documents.
func TestAttachArgv_DegradedPathsInstallNoResizePinHook(t *testing.T) {
	t.Run("ZeroCols", func(t *testing.T) {
		e, rec := newAttachTestEngine(t, goodAttachScript(), goodAttachStrands())
		assertBareArgv(t, e, e.AttachArgv(0, 24))
		if len(rec.setHookCalls) != 0 {
			t.Errorf("set-hook calls = %v, want none", rec.setHookCalls)
		}
	})
	t.Run("HasSessionFails", func(t *testing.T) {
		script := goodAttachScript()
		script.hasSessionErr = errors.New("boom")
		e, rec := newAttachTestEngine(t, script, goodAttachStrands())
		assertBareArgv(t, e, e.AttachArgv(80, 24))
		if len(rec.setHookCalls) != 0 {
			t.Errorf("set-hook calls = %v, want none", rec.setHookCalls)
		}
	})
	t.Run("FewerThanTwoLivePanes", func(t *testing.T) {
		script := goodAttachScript()
		script.listPanes = "%1 0 0 40 20 4321\n"
		e, rec := newAttachTestEngine(t, script, goodAttachStrands())
		assertBareArgv(t, e, e.AttachArgv(80, 24))
		if len(rec.setHookCalls) != 0 {
			t.Errorf("set-hook calls = %v, want none", rec.setHookCalls)
		}
	})
	t.Run("NoStrandOwnsAPresentPane", func(t *testing.T) {
		e, rec := newAttachTestEngine(t, goodAttachScript(), nil)
		assertBareArgv(t, e, e.AttachArgv(80, 24))
		if len(rec.setHookCalls) != 0 {
			t.Errorf("set-hook calls = %v, want none", rec.setHookCalls)
		}
	})
	t.Run("PlanError_DeferredAnchorRejected", func(t *testing.T) {
		strands := []Strand{{GUID: "a", PaneID: "%1", Display: render.Display{Anchor: render.AnchorOwnWindow}}}
		e, rec := newAttachTestEngine(t, goodAttachScript(), strands)
		assertBareArgv(t, e, e.AttachArgv(80, 24))
		if len(rec.setHookCalls) != 0 {
			t.Errorf("set-hook calls = %v, want none", rec.setHookCalls)
		}
	})
}

// TestAttachArgv_SetHookErrorDoesNotChangeTheChainedArgv pins hook-failure-is-non-fatal-everywhere on
// the AttachArgv path: a set-hook returning an error neither suppresses the chain nor changes a
// single element of the ten-element chained argv, compared element by element against the same argv
// built with a non-failing hook.
func TestAttachArgv_SetHookErrorDoesNotChangeTheChainedArgv(t *testing.T) {
	e, rec := newAttachTestEngine(t, goodAttachScript(), goodAttachStrands())
	baseHook := e.tmux.execHook

	want := e.AttachArgv(80, 24)
	if len(want) != 10 {
		t.Fatalf("baseline AttachArgv() = %v, want the 10-element chained argv", want)
	}

	e.tmux.execHook = func(capture bool, args ...string) (string, error) {
		if args[0] == "set-hook" {
			out, _ := baseHook(capture, args...)
			return out, errors.New("boom")
		}
		return baseHook(capture, args...)
	}

	got := e.AttachArgv(80, 24)
	if len(got) != len(want) {
		t.Fatalf("AttachArgv() with failing set-hook = %v (len %d), want %v (len %d)", got, len(got), want, len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("AttachArgv()[%d] = %q, want %q (a failing set-hook must not change the chained argv)", i, got[i], want[i])
		}
	}
	if len(rec.setHookCalls) == 0 {
		t.Fatal("no set-hook calls recorded despite the failing hook, want the install statement still attempted")
	}
}
