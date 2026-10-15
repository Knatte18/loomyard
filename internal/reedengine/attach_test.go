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
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/reedengine/render"
	"github.com/Knatte18/loomyard/internal/testkit/logcapture"
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

// goodAttachLive and goodAttachStrands are the pure-Go mirrors of goodAttachListPanes and the state this file's tests persist via SaveState, used to independently compute the expected planLayout output for comparison, rather than re-deriving it from the same code path under test.
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

// newAttachTestEngine builds a fixture engine with strands persisted to disk (loadOrInitStateLocked reads reed.json from disk, not from an in-memory struct) and a fakeTmux answering every round trip AttachArgv's pre-flight can issue with the fully-permissive script each degraded-path test starts from and re-scripts exactly one answer of,
// so each test isolates the single guard it exists to pin.
func newAttachTestEngine(t *testing.T, strands []Strand) (*Engine, *fakeTmux) {
	t.Helper()
	e := newTestEngine(t)
	if err := SaveState(e.stateDir(), &ReedState{Strands: strands}); err != nil {
		t.Fatalf("SaveState: %v", err)
	}
	fake := installFakeTmux(t, e)
	// The pane-generation probe (loadOrInitStateLocked -> adoptPaneGenerationLocked) spends its own three-field format on display-message.
	// Answering it well-formed keeps this hermetic fixture from spuriously clearing pane bindings via the probe's fail-open path.
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

// wantZoomBracketedChain builds the expected chained argv: the bare attach, the zoom record entry, then middle (the select-layout and the pins, each led by its ";"), then the zoom restore entry.
func wantZoomBracketedChain(e *Engine, middle ...string) []string {
	out := append(wantBareAttachArgv(e), ";")
	out = append(out, zoomRecordChainArgv(fakeStrandWindow)...)
	out = append(out, ";")
	out = append(out, middle...)
	out = append(out, ";")
	return append(out, zoomRestoreChainArgv(fakeStrandWindow)...)
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

// TestAttachArgv_ChainedArgv pins the chained argv on a known-good pre-flight, element by element:
// the five bare elements, the one-character ";" separator (compared exactly, so "\\;" cannot pass) and the zoom record entry.
// Then select-layout/-t/target, the layout planLayout itself would produce for the told box, and the zoom restore entry.
// The box comes from the client's told cols/rows, never from a live display-message query and never from the configured size;
// the #{status} readback is the reserved-row source (off reserves zero rows, on one, a non-negative integer that many),
// clamped to rows-1 so a multi-line status bar cannot drive the planned height to zero or below.
func TestAttachArgv_ChainedArgv(t *testing.T) {
	const cols, rows = 80, 24
	tests := []struct {
		name         string
		status       string // #{status} readback; empty keeps the fixture's "off"
		cfgW, cfgH   int    // configured size, deliberately distinct from the told cols/rows; zero keeps the fixture's
		wantReserved int
	}{
		{"StatusOff_ReservesZero", "", 0, 0, 0},
		{"StatusOn_ReservesOne", "on", 0, 0, 1},
		{"NumericTwo_ReservesTwo", "2", 0, 0, 2},
		{"HugeStatus_FlooredToRowsMinusOne", "30", 0, 0, rows - 1},
		{"ToldBoxWinsOverConfiguredSize", "", 999, 111, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e, fake := newAttachTestEngine(t, goodAttachStrands())
			if tt.status != "" {
				fake.answerFormat("#{status}", tt.status, nil)
			}
			if tt.cfgW != 0 {
				e.cfg.Width, e.cfg.Height = tt.cfgW, tt.cfgH
			}

			got := e.AttachArgv(cols, rows)

			wantLayout, _, err := e.planLayout(&ReedState{Strands: goodAttachStrands()}, goodAttachLive(), render.Box{X: 0, Y: 0, W: cols, H: rows - tt.wantReserved}, false)
			if err != nil {
				t.Fatalf("planLayout() unexpected error: %v", err)
			}
			want := wantZoomBracketedChain(e, "select-layout", "-t", fakeStrandWindow, wantLayout)
			if !slices.Equal(got, want) {
				t.Errorf("AttachArgv() = %v, want %v", got, want)
			}
			for _, argv := range fake.ArgvFor("display-message") {
				if argv[len(argv)-1] == liveBoxFormat {
					t.Fatal("AttachArgv() queried the live #{window_width} #{window_height} pair; want zero live-box round trips")
				}
			}
		})
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

			assertStrandOptionsReasserted(t, fake, 0)
			if tt.wantBare {
				assertBareArgv(t, e, got)
				return
			}
			if !slices.Contains(got, "select-layout") {
				t.Fatalf("AttachArgv() = %v, want the chained argv (this case must not suppress)", got)
			}
		})
	}
}

// TestAttachArgv_EveryOtherDegradedPathYieldsBareArgv covers every remaining refusal/skip path:
// a non-positive client size, has-session failing, fewer than two live panes, no strand owning a
// present pane, a list-panes error, and a plan error. Every one must yield exactly the bare argv,
// asserted element by element, and install no resize-pin hook (the guard-skip disposition
// install-points-are-two-named-statements-no-guard-moves documents).
// The unseen-clear hook is a geometry pin, so these paths past the pre-flight pins still issue it.
func TestAttachArgv_EveryOtherDegradedPathYieldsBareArgv(t *testing.T) {
	tests := []struct {
		name       string
		strands    []Strand
		cols, rows int
		mutate     func(*fakeTmux)
	}{
		{"ZeroCols", goodAttachStrands(), 0, 24, nil},
		{"NegativeCols", goodAttachStrands(), -1, 24, nil},
		{"ZeroRows", goodAttachStrands(), 80, 0, nil},
		{"NegativeRows", goodAttachStrands(), 80, -1, nil},
		{"HasSessionFails", goodAttachStrands(), 80, 24, func(f *fakeTmux) { f.answer("has-session", "", errors.New("boom")) }},
		{"FewerThanTwoLivePanes", goodAttachStrands(), 80, 24, func(f *fakeTmux) { f.answer("list-panes", oneAttachListPane, nil) }},
		{"NoStrandOwnsAPresentPane", nil, 80, 24, nil},
		{"ListPanesErrors", goodAttachStrands(), 80, 24, func(f *fakeTmux) { f.answer("list-panes", "", errors.New("boom")) }},
		{
			"PlanError_TopAnchorWithoutContentSizeRejected",
			[]Strand{{GUID: "a", PaneID: "%1", Display: render.Display{Anchor: render.AnchorTop, Size: render.Size{Kind: render.SizePositional}}}},
			80, 24, nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e, fake := newAttachTestEngine(t, tt.strands)
			if tt.mutate != nil {
				tt.mutate(fake)
			}

			assertBareArgv(t, e, e.AttachArgv(tt.cols, tt.rows))
			if resizeHooks := fake.SetHookCalls(windowResizedHookName); len(resizeHooks) != 0 {
				t.Errorf("set-hook calls naming %s = %v, want none on a degraded path", windowResizedHookName, resizeHooks)
			}
		})
	}
}

// TestAttachArgv_PreflightOnAKnownGoodSession pins the pre-flight of one known-good AttachArgv call:
// AttachArgv itself issues every geometry pin (not a second exported call the CLI has to remember),
// the status-line pin precedes the #{status} readback the told box depends on, and the resize-pin install
// (the set-hook clear and pin rebuild) comes after the state and pane list are read.
// It issues no pane-set mutation and leaves reed.json untouched; it does mutate a window option, the resize-pin hook,
// so "never mutates" is scoped to the pane set.
// A set-hook error then neither suppresses the chain nor changes a single element of the chained argv.
//
//testtiming:keep pins one known-good AttachArgv call issuing every geometry pin itself, the status-line pin before the #{status} readback and the set-hook clear after list-panes, no pane-set mutation, reed.json left untouched, and a failing set-hook leaving the chained argv unchanged; its covering tests run this code without asserting it
func TestAttachArgv_PreflightOnAKnownGoodSession(t *testing.T) {
	e, fake := newAttachTestEngine(t, goodAttachStrands())
	fake.mustNotCall("select-layout", "select-pane", "kill-pane", "split-window")
	// Where the shell resolves, the pre-flight also pins the session's two shell options.
	wantShellPins := 0
	if runtime.GOOS != "windows" {
		e.cfg.Shell = "sh"
		wantShellPins = len(shellOptionArgvs("", "sh"))
	}

	stateBefore, err := LoadState(e.stateDir())
	if err != nil {
		t.Fatalf("LoadState before AttachArgv: %v", err)
	}

	want := e.AttachArgv(80, 24)
	if !slices.Contains(want, "select-layout") {
		t.Fatalf("AttachArgv() = %v, want the chained argv on this known-good script", want)
	}

	// The seven bar and border pins plus the pre-existing window-size pin, the window marker and the strand pane's two options.
	wantSetOptionCalls := 11 + wantShellPins
	if setOptions := fake.ArgvFor("set-option"); len(setOptions) != wantSetOptionCalls {
		t.Fatalf("AttachArgv() issued %d set-option calls, want %d: %v", len(setOptions), wantSetOptionCalls, setOptions)
	}

	assertStrandOptionsReasserted(t, fake, wantShellPins)

	calls := fake.Calls()
	statusPinIdx, statusReadbackIdx, listPanesIdx, firstSetHookIdx := -1, -1, -1, -1
	for i, argv := range calls {
		switch {
		case argv[0] == "set-option" && argv[len(argv)-2] == "status" && statusPinIdx == -1:
			statusPinIdx = i
		case argv[0] == "display-message" && argv[len(argv)-1] == "#{status}" && statusReadbackIdx == -1:
			statusReadbackIdx = i
		case callVerb(argv) == "list-panes" && listPanesIdx == -1:
			listPanesIdx = i
		case argv[0] == "set-hook" && containsArg(argv, windowResizedHookName) && firstSetHookIdx == -1:
			firstSetHookIdx = i
		}
	}
	if statusPinIdx == -1 || statusReadbackIdx == -1 || listPanesIdx == -1 || firstSetHookIdx == -1 {
		t.Fatalf("calls = %v, want a status pin, a status readback, a list-panes and a set-hook call", calls)
	}
	if statusPinIdx >= statusReadbackIdx {
		t.Errorf("calls = %v, want the status-off pin (index %d) before the #{status} readback (index %d)", calls, statusPinIdx, statusReadbackIdx)
	}
	if firstSetHookIdx <= listPanesIdx {
		t.Errorf("calls = %v, want the first set-hook call (index %d) after list-panes (index %d)", calls, firstSetHookIdx, listPanesIdx)
	}
	if setHooks := fake.SetHookCalls(windowResizedHookName); !containsArg(setHooks[0], "-u") {
		t.Errorf("first resize set-hook argv = %v, want the -u clear", setHooks[0])
	}
	assertUnseenClearHookPinned(t, e, fake)

	stateAfter, err := LoadState(e.stateDir())
	if err != nil {
		t.Fatalf("LoadState after AttachArgv: %v", err)
	}
	if len(stateAfter.Strands) != len(stateBefore.Strands) || stateAfter.SelvagePaneID != stateBefore.SelvagePaneID {
		t.Errorf("reed.json changed across AttachArgv: before=%+v after=%+v", stateBefore, stateAfter)
	}

	// Hook failure is non-fatal on the AttachArgv path.
	hooksBefore := len(fake.ArgvFor("set-hook"))
	fake.answer("set-hook", "", errors.New("boom"))
	got := e.AttachArgv(80, 24)
	if !slices.Equal(got, want) {
		t.Errorf("AttachArgv() with failing set-hook = %v, want %v (a failing set-hook must not change the chained argv)", got, want)
	}
	if len(fake.ArgvFor("set-hook")) == hooksBefore {
		t.Error("no set-hook call recorded despite the failing hook, want the install statement still attempted")
	}
}

// assertStrandOptionsReasserted asserts the pre-flight marked the strand window and set the pane options on the live bound pane %1, and none on the unbound pane %2.
// It also asserts the pre-flight issued exactly wantShellPins session shell pins, none where the configured shell does not resolve.
func assertStrandOptionsReasserted(t *testing.T, fake *fakeTmux, wantShellPins int) {
	t.Helper()
	var marked, labeled bool
	shellPins := 0
	for _, argv := range fake.ArgvFor("set-option") {
		switch {
		case containsArg(argv, "default-shell"), containsArg(argv, "default-command"):
			shellPins++
		case containsArg(argv, "@lyx_strands"):
			marked = true
		case containsArg(argv, "@strand"):
			labeled = true
			if !containsArg(argv, "%1") {
				t.Errorf("@strand set with %v, want it on the bound pane %%1 only", argv)
			}
		}
	}
	if !marked || !labeled {
		t.Errorf("set-option calls = %v, want the window marked (%v) and the bound pane labeled (%v)", fake.ArgvFor("set-option"), marked, labeled)
	}
	if shellPins != wantShellPins {
		t.Errorf("set-option calls = %v, want %d session shell pins, got %d", fake.ArgvFor("set-option"), wantShellPins, shellPins)
	}
}

// wantChainedAttachArgv builds the exact chained argv TestAttachArgv_ChainedArgv already pins for
// goodAttachStrands at cols/rows, so the multi-client warning tests below can assert their argv is
// byte-identical to what the same script produces today without re-deriving the expectation.
func wantChainedAttachArgv(t *testing.T, e *Engine, cols, rows int) []string {
	t.Helper()
	layout, _, err := e.planLayout(&ReedState{Strands: goodAttachStrands()}, goodAttachLive(), render.Box{X: 0, Y: 0, W: cols, H: rows}, false)
	if err != nil {
		t.Fatalf("planLayout() unexpected error: %v", err)
	}
	return wantZoomBracketedChain(e, "select-layout", "-t", fakeStrandWindow, layout)
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

// relevantWindowFixture returns the state and the scripted window listings of a session with the shell window @0, the overview @3 at index 0 and two agent windows, alpha @1 and beta @2 in start order.
// alphaDead and betaDead mark that agent's pane dead.
// The strands also hold the window 0 strand of goodAttachStrands.
func relevantWindowFixture(alphaDead, betaDead bool) (strands []Strand, windows, panes string) {
	flag := func(dead bool) string {
		if dead {
			return "1"
		}
		return "0"
	}
	own := render.Display{Anchor: render.AnchorOwnWindow}
	strands = append(goodAttachStrands(),
		Strand{GUID: "alpha", Name: "alpha", PaneID: "%10", Display: own},
		Strand{GUID: "beta", Name: "beta", PaneID: "%11", Display: own},
		Strand{GUID: "status", Name: "status", PaneID: "%12", Display: render.Display{Anchor: render.AnchorOverview}},
	)
	windows = "@3 0 0 0 overview\n@1 1 0 0 alpha\n@2 2 0 0 beta\n@0 3 0 0 shell\n"
	panes = "@0 0 - %1 0 4321 a\n" +
		"@1 1 - %10 " + flag(alphaDead) + " 5001 alpha\n" +
		"@2 1 - %11 " + flag(betaDead) + " 5002 beta\n" +
		"@3 1 - %12 0 5003 status\n"
	return strands, windows, panes
}

// TestAttachArgv_SelectsTheRelevantWindow pins the window selection AttachArgv appends to every argv it returns:
// `; select-window -t <window>` for the newest live agent window, else the overview, else nothing.
// Beside another attached client, or a failed client listing, the selection is kept only while the follow state reads on, an unset option included, and dropped while it reads off or its read fails.
// Attach reads the follow state only then and never writes it.
func TestAttachArgv_SelectsTheRelevantWindow(t *testing.T) {
	const cols, rows = 80, 24
	tests := []struct {
		name       string
		alphaDead  bool
		betaDead   bool
		noOwn      bool
		clients    string
		clientsErr error
		follow     string
		followErr  error
		cols, rows int
		script     func(f *fakeTmux)
		wantChain  bool
		wantWindow string
	}{
		{name: "NewestLiveAgent", cols: cols, rows: rows, wantChain: true, wantWindow: "@2"},
		{name: "NewestAgentDeadFallsBackToTheOlderLiveAgent", betaDead: true, cols: cols, rows: rows, wantChain: true, wantWindow: "@1"},
		{name: "NoLiveAgentFallsBackToTheOverview", alphaDead: true, betaDead: true, cols: cols, rows: rows, wantChain: true, wantWindow: "@3"},
		{name: "NeitherAgentNorOverviewAppendsNothing", noOwn: true, cols: cols, rows: rows, wantChain: true},
		{name: "AnotherClientWithFollowOffAppendsNothing", clients: "tty0 80 24", follow: "off", cols: cols, rows: rows, wantChain: true},
		{name: "AnotherClientWithFollowUnsetSelectsTheRelevantWindow", clients: "tty0 80 24", follow: "", cols: cols, rows: rows, wantChain: true, wantWindow: "@2"},
		{name: "AnotherClientWithFollowOnSelectsTheRelevantWindow", clients: "tty0 80 24", follow: "on", cols: cols, rows: rows, wantChain: true, wantWindow: "@2"},
		{name: "AnotherClientWithAFailedFollowReadAppendsNothing", clients: "tty0 80 24", followErr: errors.New("boom"), cols: cols, rows: rows, wantChain: true},
		{name: "FailedClientListingWithFollowOnSelectsTheRelevantWindow", clientsErr: errors.New("boom"), follow: "on", cols: cols, rows: rows, wantChain: true, wantWindow: "@2"},
		{name: "UnknownClientSizeGivesTheBareArgvPlusTheSelection", wantWindow: "@2"},
		{
			name: "OnlySelvageInTheStrandsWindowGivesTheBareArgvPlusTheSelection", cols: cols, rows: rows, wantWindow: "@2",
			script: func(f *fakeTmux) { f.answer("list-panes", oneAttachListPane, nil) },
		},
		{
			name: "WindowSizeOtherThanLatestGivesTheBareArgvPlusTheSelection", cols: cols, rows: rows, wantWindow: "@2",
			script: func(f *fakeTmux) { f.answerFormat("#{window-size}", "manual", nil) },
		},
		{
			name: "UnrecognizedStatusGivesTheBareArgvPlusTheSelection", cols: cols, rows: rows, wantWindow: "@2",
			script: func(f *fakeTmux) { f.answerFormat("#{status}", "bogus", nil) },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			strands, windows, panes := relevantWindowFixture(tt.alphaDead, tt.betaDead)
			if tt.noOwn {
				strands, windows, panes = goodAttachStrands(), "@0 0 0 0 shell\n", "@0 0 - %1 0 4321 a\n"
			}
			e, fake := newAttachTestEngine(t, strands)
			fake.answer("list-windows", windows, nil)
			fake.answerWindowPanes(panes, nil)
			fake.answer("list-clients", tt.clients, tt.clientsErr)
			otherClient := tt.clients != "" || tt.clientsErr != nil
			if otherClient {
				fake.answerFormat("#{@lyx_follow}", tt.follow, tt.followErr)
			}
			if tt.script != nil {
				tt.script(fake)
			}

			got := e.AttachArgv(tt.cols, tt.rows)

			followReads := 0
			for _, call := range fake.Calls() {
				if !slices.ContainsFunc(call, func(arg string) bool { return strings.Contains(arg, "@lyx_follow") }) {
					continue
				}
				if call[0] != "display-message" || call[len(call)-1] != "#{@lyx_follow}" {
					t.Errorf("attach issued %v, want no call carrying @lyx_follow but the display-message read", call)
					continue
				}
				followReads++
			}
			if wantRead := followReads == 1; wantRead != otherClient || followReads > 1 {
				t.Errorf("follow reads = %d, want one read exactly when another client is attached or the listing failed (%v)", followReads, otherClient)
			}

			want := wantBareAttachArgv(e)
			if tt.wantChain {
				want = wantChainedAttachArgv(t, e, tt.cols, tt.rows)
			}
			if tt.wantWindow != "" {
				want = append(want, ";", "select-window", "-t", tt.wantWindow)
			}
			if !slices.Equal(got, want) {
				t.Errorf("AttachArgv(%d, %d) = %v, want %v", tt.cols, tt.rows, got, want)
			}
		})
	}
}

// TestAttachArgv_MultiClientWarning covers warnMismatchedClientsLocked's cardinality: exactly one
// logger.Warn line per client whose listed size differs from the size this attach was told, none for
// a matching client, and — in every case — an argv byte-identical to what the same script produces
// with no attached clients at all, since the warning is a side effect that must never perturb it.
// That holds while the follow state reads off or no window is relevant, which is when the other client's presence drops the window selection.
func TestAttachArgv_MultiClientWarning(t *testing.T) {
	const cols, rows = 80, 24

	t.Run("SameSizeClient_NoWarning", func(t *testing.T) {
		e, fake := newAttachTestEngine(t, goodAttachStrands())
		fake.answer("list-clients", "tty0 80 24", nil)
		buf := logcapture.CaptureVerbose(t)

		got := e.AttachArgv(cols, rows)

		assertChainedArgv(t, e, cols, rows, got)
		if strings.Contains(buf.String(), "reed: another client is attached") {
			t.Errorf("log output = %q, want no multi-client warning for a same-size client", buf.String())
		}
	})

	t.Run("DifferentSizeClient_OneWarningLine", func(t *testing.T) {
		e, fake := newAttachTestEngine(t, goodAttachStrands())
		fake.answer("list-clients", "tty0 100 40", nil)
		buf := logcapture.CaptureVerbose(t)

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
		buf := logcapture.CaptureVerbose(t)

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

	t.Run("ListClientsError_WarnsAndAppendsNoSelection", func(t *testing.T) {
		strands, windows, panes := relevantWindowFixture(false, false)
		e, fake := newAttachTestEngine(t, strands)
		fake.answer("list-windows", windows, nil)
		fake.answerWindowPanes(panes, nil)
		fake.answer("list-clients", "", errors.New("boom"))
		// A failed listing counts as another client, so follow decides the selection; off drops it.
		fake.answerFormat("#{@lyx_follow}", "off", nil)
		buf := logcapture.CaptureVerbose(t)

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
		buf := logcapture.CaptureVerbose(t)

		got := e.AttachArgv(cols, rows)

		assertBareArgv(t, e, got)
		out := buf.String()
		if n := strings.Count(out, "reed: another client is attached"); n != 1 {
			t.Fatalf("log output = %q, want exactly 1 multi-client warning line even though the chain is suppressed, got %d", out, n)
		}
	})
}

// TestAttachArgv_ChainCarriesTheAdjustedPinsAfterSelectLayout pins the chain's pin tail and the hook it installs:
// `attach-session ; <zoom record> ; select-layout ... ; resize-pane -t <pane> -y <n>` per pin, then the zoom restore, the row-0 pane taken from the physical pane order and its pin one row shorter under a title row.
func TestAttachArgv_ChainCarriesTheAdjustedPinsAfterSelectLayout(t *testing.T) {
	const cols, rows = 80, 24
	strands := []Strand{
		{GUID: "root", PaneID: "%1", Display: render.Display{Anchor: render.AnchorBelowParent}},
		{GUID: "child", Parent: "root", PaneID: "%2", Display: render.Display{Anchor: render.AnchorBelowParent}},
	}
	tests := []struct {
		name         string
		borderStatus string
		listPanes    string
		wantHeight   int
	}{
		{"title row with the pinned pane at row 0", "top", "%1 0 0 40 20 4321\n%2 0 20 40 20 4322\n", planCollapsedRows - 1},
		{"title row with an unpinned pane at row 0, whatever the pin order", "top", "%2 0 0 40 20 4322\n%1 0 20 40 20 4321\n", planCollapsedRows},
		{"no title row", "off", "%1 0 0 40 20 4321\n%2 0 20 40 20 4322\n", planCollapsedRows},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e, fake := newAttachTestEngine(t, strands)
			fake.answerFormat("#{pane-border-status}", tt.borderStatus, nil)
			fake.answer("list-panes", tt.listPanes, nil)

			got := e.AttachArgv(cols, rows)

			layoutAt := slices.Index(got, "select-layout")
			if layoutAt == -1 {
				t.Fatalf("AttachArgv() = %v, want a chained select-layout", got)
			}
			want := wantZoomBracketedChain(e, "select-layout", "-t", fakeStrandWindow, got[layoutAt+3], ";", "resize-pane", "-t", "%1", "-y", strconv.Itoa(tt.wantHeight))
			if !slices.Equal(got, want) {
				t.Fatalf("AttachArgv() = %v, want %v", got, want)
			}
			wantBody := "resize-pane -t %1 -y " + strconv.Itoa(tt.wantHeight)
			var bodies []string
			for _, argv := range fake.ArgvFor("set-hook") {
				bodies = append(bodies, argv[len(argv)-1])
			}
			if !slices.Contains(bodies, wantBody) {
				t.Errorf("hook entries = %v, want %q among them", bodies, wantBody)
			}
		})
	}
}

// TestAttachArgv_PreflightListsPanesOnTheUnzoomedWindow pins the pre-flight's bracket: a zoomed strand window is unzoomed before the pane list the layout is planned from, and zoomed again after it, the same pane both times.
func TestAttachArgv_PreflightListsPanesOnTheUnzoomedWindow(t *testing.T) {
	e, fake := newAttachTestEngine(t, goodAttachStrands())
	fake.answerFormat(zoomStateFormat, "1 %2", nil)

	got := e.AttachArgv(80, 24)

	if !slices.Contains(got, "select-layout") {
		t.Fatalf("AttachArgv() = %v, want the chained argv", got)
	}
	var steps []string
	for _, argv := range fake.Calls() {
		switch {
		case slices.Equal(argv, []string{"resize-pane", "-Z", "-t", "%2"}):
			steps = append(steps, "zoom-toggle")
		case callVerb(argv) == "list-panes":
			steps = append(steps, "list-panes")
		}
	}
	if len(steps) < 3 || steps[0] != "zoom-toggle" || steps[len(steps)-1] != "zoom-toggle" || slices.Index(steps, "list-panes") < 1 {
		t.Errorf("pre-flight steps = %v, want the unzoom toggle first, the pane lists after it and the re-zoom toggle last", steps)
	}
}
