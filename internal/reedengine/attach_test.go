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
	"strings"
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

const (
	goodAttachListPanes = "%1 0 0 40 20 4321\n%2 0 20 40 20 4322\n"
	oneAttachListPane   = "%1 0 0 40 20 4321\n"
)

// goodAttachLive and goodAttachStrands are the pure-Go mirrors of goodAttachListPanes and the state
// this file's tests persist via SaveState, used to independently compute the expected planLayout
// output for comparison, rather than re-deriving it from the same code path under test.
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

// newAttachTestEngine builds a fixture engine with strands persisted to disk (loadOrInitStateLocked
// reads reed.json from disk, not from an in-memory struct) and a fakeTmux answering every round trip
// AttachArgv's pre-flight can issue with the fully-permissive script each degraded-path test starts
// from and re-scripts exactly one answer of, so each test isolates the single guard it exists to pin.
func newAttachTestEngine(t *testing.T, strands []Strand) (*Engine, *fakeTmux) {
	t.Helper()
	e := newTestEngine(t)
	if err := SaveState(e.stateDir(), &ReedState{Strands: strands}); err != nil {
		t.Fatalf("SaveState: %v", err)
	}
	fake := installFakeTmux(t, e)
	// The pane-generation probe (loadOrInitStateLocked -> adoptPaneGenerationLocked) spends its own
	// three-field format on display-message. Answering it well-formed keeps this hermetic fixture from
	// spuriously clearing pane bindings via the probe's fail-open path.
	fake.answer("display-message", "$0|4321|1700000000", nil)
	fake.answerFormat("#{window-size}", "latest", nil)
	fake.answerFormat("#{status}", "off", nil)
	fake.answerFormat(liveBoxFormat, "", errors.New("AttachArgv must never query the live window size"))
	fake.answer("list-panes", goodAttachListPanes, nil)
	return e, fake
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
	e, _ := newAttachTestEngine(t, goodAttachStrands())
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
	e, fake := newAttachTestEngine(t, goodAttachStrands())
	e.cfg.Width, e.cfg.Height = 999, 111 // deliberately distinct from the client size below
	const cols, rows = 80, 24

	got := e.AttachArgv(cols, rows)

	for _, argv := range fake.ArgvFor("display-message") {
		if argv[len(argv)-1] == liveBoxFormat {
			t.Fatal("AttachArgv() queried the live #{window_width} #{window_height} pair; want zero live-box round trips")
		}
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
			e, fake := newAttachTestEngine(t, goodAttachStrands())
			fake.answerFormat("#{status}", tt.status, nil)

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
	const status = "30"
	const cols, rows = 80, 24
	e, fake := newAttachTestEngine(t, goodAttachStrands())
	fake.answerFormat("#{status}", status, nil)

	got := e.AttachArgv(cols, rows)

	const wantReserved = rows - 1
	wantLayout, _, err := e.planLayout(&ReedState{Strands: goodAttachStrands()}, goodAttachLive(), render.Box{X: 0, Y: 0, W: cols, H: rows - wantReserved})
	if err != nil {
		t.Fatalf("planLayout() unexpected error: %v", err)
	}
	if len(got) != 10 || got[9] != wantLayout {
		t.Fatalf("AttachArgv() with #{status}=%q (rows=%d) = %v, want reserved floored to %d (layout %q)", status, rows, got, wantReserved, wantLayout)
	}
}

// TestAttachArgv_ChainGate pins readback-not-exit-status-gates-the-chain: only #{window-size} and an
// unrecognised #{status} suppress the chain; a recognised #{status} other than "off" is an input to
// the reserved-row count, never a gate.
func TestAttachArgv_ChainGate(t *testing.T) {
	tests := []struct {
		name     string
		mutate   func(*fakeTmux)
		wantBare bool
	}{
		{"WindowSize_Manual_Suppresses", func(f *fakeTmux) { f.answerFormat("#{window-size}", "manual", nil) }, true},
		{"WindowSize_Largest_Suppresses", func(f *fakeTmux) { f.answerFormat("#{window-size}", "largest", nil) }, true},
		{"WindowSize_Garbage_Suppresses", func(f *fakeTmux) { f.answerFormat("#{window-size}", "garbage", nil) }, true},
		{"WindowSize_Error_Suppresses", func(f *fakeTmux) { f.answerFormat("#{window-size}", "latest", errors.New("boom")) }, true},
		{"Status_Garbage_Suppresses", func(f *fakeTmux) { f.answerFormat("#{status}", "garbage", nil) }, true},
		{"Status_Error_Suppresses", func(f *fakeTmux) { f.answerFormat("#{status}", "off", errors.New("boom")) }, true},
		{"Status_On_DoesNotSuppress", func(f *fakeTmux) { f.answerFormat("#{status}", "on", nil) }, false},
	}
	const cols, rows = 80, 24
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e, fake := newAttachTestEngine(t, goodAttachStrands())
			tt.mutate(fake)

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
		e, _ := newAttachTestEngine(t, goodAttachStrands())
		assertBareArgv(t, e, e.AttachArgv(0, 24))
	})
	t.Run("NegativeCols", func(t *testing.T) {
		e, _ := newAttachTestEngine(t, goodAttachStrands())
		assertBareArgv(t, e, e.AttachArgv(-1, 24))
	})
	t.Run("ZeroRows", func(t *testing.T) {
		e, _ := newAttachTestEngine(t, goodAttachStrands())
		assertBareArgv(t, e, e.AttachArgv(80, 0))
	})
	t.Run("NegativeRows", func(t *testing.T) {
		e, _ := newAttachTestEngine(t, goodAttachStrands())
		assertBareArgv(t, e, e.AttachArgv(80, -1))
	})
	t.Run("HasSessionFails", func(t *testing.T) {
		e, fake := newAttachTestEngine(t, goodAttachStrands())
		fake.answer("has-session", "", errors.New("boom"))
		assertBareArgv(t, e, e.AttachArgv(80, 24))
	})
	t.Run("FewerThanTwoLivePanes", func(t *testing.T) {
		e, fake := newAttachTestEngine(t, goodAttachStrands())
		fake.answer("list-panes", oneAttachListPane, nil)
		assertBareArgv(t, e, e.AttachArgv(80, 24))
	})
	t.Run("NoStrandOwnsAPresentPane", func(t *testing.T) {
		e, _ := newAttachTestEngine(t, nil)
		assertBareArgv(t, e, e.AttachArgv(80, 24))
	})
	t.Run("ListPanesErrors", func(t *testing.T) {
		e, fake := newAttachTestEngine(t, goodAttachStrands())
		fake.answer("list-panes", "", errors.New("boom"))
		assertBareArgv(t, e, e.AttachArgv(80, 24))
	})
	t.Run("PlanError_DeferredAnchorRejected", func(t *testing.T) {
		strands := []Strand{{GUID: "a", PaneID: "%1", Display: render.Display{Anchor: render.AnchorOwnWindow}}}
		e, _ := newAttachTestEngine(t, strands)
		assertBareArgv(t, e, e.AttachArgv(80, 24))
	})
}

// TestAttachArgv_PinsMadeByBuilderBeforeStatusReadback pins that AttachArgv itself issues every
// geometry pin — not a second exported call the CLI has to remember — and that the status-line pin
// precedes the #{status} readback, the ordering the told box depends on (pinGeometryOptionsLocked's
// doc comment: the told box is only correct once the status-line pins have landed and been read back).
func TestAttachArgv_PinsMadeByBuilderBeforeStatusReadback(t *testing.T) {
	e, fake := newAttachTestEngine(t, goodAttachStrands())
	// newTestEngine's Geometry leaves WorktreeName unset; the default status-line template's
	// {{.worktree}} marker requires it, so this case sets it so StatusLineText() succeeds and all
	// eight set-option calls (not the six-call degraded shape) are issued.
	e.geom.WorktreeName = "test-worktree"

	got := e.AttachArgv(80, 24)
	if len(got) != 10 {
		t.Fatalf("AttachArgv() = %v, want the 10-element chained argv on this known-good script", got)
	}

	// The seven status-line options plus the pre-existing window-size pin.
	const wantSetOptionCalls = 8
	if setOptions := fake.ArgvFor("set-option"); len(setOptions) != wantSetOptionCalls {
		t.Fatalf("AttachArgv() issued %d set-option calls, want %d: %v", len(setOptions), wantSetOptionCalls, setOptions)
	}

	calls := fake.Calls()
	statusPinIdx, statusReadbackIdx := -1, -1
	for i, argv := range calls {
		if argv[0] == "set-option" && argv[len(argv)-2] == "status" && statusPinIdx == -1 {
			statusPinIdx = i
		}
		if argv[0] == "display-message" && argv[len(argv)-1] == "#{status}" && statusReadbackIdx == -1 {
			statusReadbackIdx = i
		}
	}
	if statusPinIdx == -1 || statusReadbackIdx == -1 {
		t.Fatalf("calls = %v, want both a status pin and a status readback", calls)
	}
	if statusPinIdx >= statusReadbackIdx {
		t.Errorf("calls = %v, want the status-off pin (index %d) before the #{status} readback (index %d)", calls, statusPinIdx, statusReadbackIdx)
	}
}

// TestAttachArgv_NeverMutatesTheSessionOrPersistsState pins that AttachArgv issues no pane-set
// mutation: no select-layout, select-pane, kill-pane, or split-window is ever issued (the chain
// carries select-layout only inside the returned ARGV, never applies it), and reed.json is neither
// created nor modified by the call. AttachArgv deliberately does mutate a window OPTION now — the
// resize-pin hook, alongside the two geometry pins it already set — so "never mutates" is scoped to
// the pane set, not to every tmux call this builder makes.
func TestAttachArgv_NeverMutatesTheSessionOrPersistsState(t *testing.T) {
	e, fake := newAttachTestEngine(t, goodAttachStrands())
	fake.mustNotCall("select-layout", "select-pane", "kill-pane", "split-window")

	before, err := LoadState(e.stateDir())
	if err != nil {
		t.Fatalf("LoadState before AttachArgv: %v", err)
	}

	if got := e.AttachArgv(80, 24); len(got) != 10 {
		t.Fatalf("AttachArgv() = %v, want the 10-element chained argv on this known-good script", got)
	}

	after, err := LoadState(e.stateDir())
	if err != nil {
		t.Fatalf("LoadState after AttachArgv: %v", err)
	}
	if len(after.Strands) != len(before.Strands) || after.SelvagePaneID != before.SelvagePaneID {
		t.Errorf("reed.json changed across AttachArgv: before=%+v after=%+v", before, after)
	}
}

// TestAttachArgv_InstallsResizePinsAfterStateAndPanesRead pins the install statement's position in
// AttachArgv's pre-flight: a known-good pre-flight issues the set-hook clear (and pin rebuild) after
// the state and pane list are read, and before the argv is returned.
func TestAttachArgv_InstallsResizePinsAfterStateAndPanesRead(t *testing.T) {
	e, fake := newAttachTestEngine(t, goodAttachStrands())

	got := e.AttachArgv(80, 24)
	if len(got) != 10 {
		t.Fatalf("AttachArgv() = %v, want the 10-element chained argv on this known-good script", got)
	}

	sequence := fake.Sequence("list-panes", "set-hook")
	listPanesIdx, firstSetHookIdx := -1, -1
	for i, step := range sequence {
		if step == "list-panes" && listPanesIdx == -1 {
			listPanesIdx = i
		}
		if step == "set-hook" && firstSetHookIdx == -1 {
			firstSetHookIdx = i
		}
	}
	if listPanesIdx == -1 {
		t.Fatalf("sequence = %v, want a list-panes call", sequence)
	}
	if firstSetHookIdx == -1 {
		t.Fatalf("sequence = %v, want at least one set-hook call", sequence)
	}
	if firstSetHookIdx <= listPanesIdx {
		t.Errorf("sequence = %v, want the first set-hook call (index %d) after list-panes (index %d)", sequence, firstSetHookIdx, listPanesIdx)
	}
	setHooks := fake.ArgvFor("set-hook")
	if len(setHooks) == 0 {
		t.Fatal("no set-hook calls recorded, want at least the clear")
	}
	if !containsArg(setHooks[0], "-u") {
		t.Errorf("first set-hook argv = %v, want the -u clear", setHooks[0])
	}
}

// TestAttachArgv_DegradedPathsInstallNoResizePinHook pins that every degraded path yielding the bare
// argv issues no set-hook call at all — the guard-skip disposition
// install-points-are-two-named-statements-no-guard-moves documents.
func TestAttachArgv_DegradedPathsInstallNoResizePinHook(t *testing.T) {
	t.Run("ZeroCols", func(t *testing.T) {
		e, fake := newAttachTestEngine(t, goodAttachStrands())
		fake.mustNotCall("set-hook")
		assertBareArgv(t, e, e.AttachArgv(0, 24))
	})
	t.Run("HasSessionFails", func(t *testing.T) {
		e, fake := newAttachTestEngine(t, goodAttachStrands())
		fake.answer("has-session", "", errors.New("boom"))
		fake.mustNotCall("set-hook")
		assertBareArgv(t, e, e.AttachArgv(80, 24))
	})
	t.Run("FewerThanTwoLivePanes", func(t *testing.T) {
		e, fake := newAttachTestEngine(t, goodAttachStrands())
		fake.answer("list-panes", oneAttachListPane, nil)
		fake.mustNotCall("set-hook")
		assertBareArgv(t, e, e.AttachArgv(80, 24))
	})
	t.Run("NoStrandOwnsAPresentPane", func(t *testing.T) {
		e, fake := newAttachTestEngine(t, nil)
		fake.mustNotCall("set-hook")
		assertBareArgv(t, e, e.AttachArgv(80, 24))
	})
	t.Run("PlanError_DeferredAnchorRejected", func(t *testing.T) {
		strands := []Strand{{GUID: "a", PaneID: "%1", Display: render.Display{Anchor: render.AnchorOwnWindow}}}
		e, fake := newAttachTestEngine(t, strands)
		fake.mustNotCall("set-hook")
		assertBareArgv(t, e, e.AttachArgv(80, 24))
	})
}

// TestAttachArgv_SetHookErrorDoesNotChangeTheChainedArgv pins hook-failure-is-non-fatal-everywhere on
// the AttachArgv path: a set-hook returning an error neither suppresses the chain nor changes a
// single element of the ten-element chained argv, compared element by element against the same argv
// built with a non-failing hook.
func TestAttachArgv_SetHookErrorDoesNotChangeTheChainedArgv(t *testing.T) {
	e, fake := newAttachTestEngine(t, goodAttachStrands())

	want := e.AttachArgv(80, 24)
	if len(want) != 10 {
		t.Fatalf("baseline AttachArgv() = %v, want the 10-element chained argv", want)
	}

	fake.answer("set-hook", "", errors.New("boom"))

	got := e.AttachArgv(80, 24)
	if len(got) != len(want) {
		t.Fatalf("AttachArgv() with failing set-hook = %v (len %d), want %v (len %d)", got, len(got), want, len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("AttachArgv()[%d] = %q, want %q (a failing set-hook must not change the chained argv)", i, got[i], want[i])
		}
	}
	if len(fake.ArgvFor("set-hook")) == 0 {
		t.Fatal("no set-hook calls recorded despite the failing hook, want the install statement still attempted")
	}
}

// wantChainedAttachArgv builds the exact chained argv TestAttachArgv_ChainedShape already pins for
// goodAttachStrands at cols/rows, so the multi-client warning tests below can assert their argv is
// byte-identical to what the same script produces today without re-deriving the expectation.
func wantChainedAttachArgv(t *testing.T, e *Engine, cols, rows int) []string {
	t.Helper()
	layout, _, err := e.planLayout(&ReedState{Strands: goodAttachStrands()}, goodAttachLive(), render.Box{X: 0, Y: 0, W: cols, H: rows})
	if err != nil {
		t.Fatalf("planLayout() unexpected error: %v", err)
	}
	bare := wantBareAttachArgv(e)
	out := append([]string{}, bare...)
	target := exactSessionWindowTarget(e.SessionName())
	out = append(out, ";", "select-layout", "-t", target, layout)
	return out
}

// assertChainedArgv asserts got is byte-identical to wantChainedAttachArgv(e, cols, rows).
func assertChainedArgv(t *testing.T, e *Engine, cols, rows int, got []string) {
	t.Helper()
	want := wantChainedAttachArgv(t, e, cols, rows)
	if len(got) != len(want) {
		t.Fatalf("AttachArgv() = %v (len %d), want %v (len %d)", got, len(got), want, len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("AttachArgv()[%d] = %q, want %q (full: got=%v want=%v)", i, got[i], want[i], got, want)
		}
	}
}

// TestAttachArgv_MultiClientWarning covers warnMismatchedClientsLocked's cardinality: exactly one
// logger.Warn line per client whose listed size differs from the size this attach was told, none for
// a matching client, and — in every case — an argv byte-identical to what the same script produces
// with no attached clients at all, since the warning is a side effect that must never perturb it.
func TestAttachArgv_MultiClientWarning(t *testing.T) {
	const cols, rows = 80, 24

	t.Run("SameSizeClient_NoWarning", func(t *testing.T) {
		e, fake := newAttachTestEngine(t, goodAttachStrands())
		fake.answer("list-clients", "tty0 80 24", nil)
		buf := captureLogOutput(t)

		got := e.AttachArgv(cols, rows)

		assertChainedArgv(t, e, cols, rows, got)
		if strings.Contains(buf.String(), "reed: another client is attached") {
			t.Errorf("log output = %q, want no multi-client warning for a same-size client", buf.String())
		}
	})

	t.Run("DifferentSizeClient_OneWarningLine", func(t *testing.T) {
		e, fake := newAttachTestEngine(t, goodAttachStrands())
		fake.answer("list-clients", "tty0 100 40", nil)
		buf := captureLogOutput(t)

		got := e.AttachArgv(cols, rows)

		assertChainedArgv(t, e, cols, rows, got)
		out := buf.String()
		if n := strings.Count(out, "reed: another client is attached"); n != 1 {
			t.Fatalf("log output = %q, want exactly 1 multi-client warning line, got %d", out, n)
		}
		for _, want := range []string{"tty0", "100", "40", "80", "24"} {
			if !strings.Contains(out, want) {
				t.Errorf("log output = %q, want it to mention %q", out, want)
			}
		}
	})

	t.Run("ThreeClientsTwoDiffer_TwoWarningLines", func(t *testing.T) {
		e, fake := newAttachTestEngine(t, goodAttachStrands())
		fake.answer("list-clients", "tty0 80 24\ntty1 100 40\ntty2 90 30", nil)
		buf := captureLogOutput(t)

		got := e.AttachArgv(cols, rows)

		assertChainedArgv(t, e, cols, rows, got)
		out := buf.String()
		if n := strings.Count(out, "reed: another client is attached"); n != 2 {
			t.Fatalf("log output = %q, want exactly 2 multi-client warning lines, got %d", out, n)
		}
		if strings.Contains(out, "client=tty0") {
			t.Errorf("log output = %q, want no warning naming the matching client tty0", out)
		}
		for _, want := range []string{"tty1", "tty2"} {
			if !strings.Contains(out, want) {
				t.Errorf("log output = %q, want it to mention the differing client %q", out, want)
			}
		}
	})

	t.Run("ListClientsError_WarnsAndDoesNotChangeBehaviour", func(t *testing.T) {
		e, fake := newAttachTestEngine(t, goodAttachStrands())
		fake.answer("list-clients", "", errors.New("boom"))
		buf := captureLogOutput(t)

		got := e.AttachArgv(cols, rows)

		assertChainedArgv(t, e, cols, rows, got)
		if !strings.Contains(buf.String(), "reed: failed to list attached clients") {
			t.Errorf("log output = %q, want the list-clients round-trip failure logged", buf.String())
		}
	})

	t.Run("SuppressedChainStillWarns", func(t *testing.T) {
		e, fake := newAttachTestEngine(t, goodAttachStrands())
		fake.answerFormat("#{window-size}", "manual", nil)
		fake.answer("list-clients", "tty0 999 999", nil)
		buf := captureLogOutput(t)

		got := e.AttachArgv(cols, rows)

		assertBareArgv(t, e, got)
		out := buf.String()
		if n := strings.Count(out, "reed: another client is attached"); n != 1 {
			t.Fatalf("log output = %q, want exactly 1 multi-client warning line even though the chain is suppressed, got %d", out, n)
		}
	})
}
