// attach_test.go pins AttachArgv's argv shape, its pre-flight and its window selection.
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
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/reedengine/render"
	"github.com/Knatte18/loomyard/internal/testkit/logcapture"
)

// TestParseClientList is a table of `list-clients` answer shapes, asserting the parsed attachedClient slice element by element.
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

// goodAttachSelvagePane is the console pane newAttachTestEngine records when the strand table is not empty; it is the first pane of goodAttachListPanes and sits in the shell window.
const goodAttachSelvagePane = "%1"

// goodAttachStrands is the strand table this file's tests persist via SaveState; its one strand is bound to the second pane of goodAttachListPanes.
func goodAttachStrands() []Strand {
	return []Strand{
		{GUID: "a", PaneID: "%2", Display: render.Display{Anchor: render.AnchorOwnWindow}},
	}
}

// newAttachTestEngine builds a fixture engine with strands persisted to disk (loadOrInitStateLocked reads reed.json from disk, not from an in-memory struct) and a fakeTmux answering every round trip AttachArgv's pre-flight can issue with the fully-permissive script each test starts from and re-scripts exactly one answer of,
// so each test isolates the single step it exists to pin.
func newAttachTestEngine(t *testing.T, strands []Strand) (*Engine, *fakeTmux) {
	t.Helper()
	e := newTestEngine(t)
	persisted := &ReedState{Strands: strands}
	if len(strands) > 0 {
		persisted.SelvagePaneID = goodAttachSelvagePane
	}
	if err := SaveState(e.stateDir(), persisted); err != nil {
		t.Fatalf("SaveState: %v", err)
	}
	fake := installFakeTmux(t, e)
	// The pane-generation probe (loadOrInitStateLocked -> adoptPaneGenerationLocked) spends its own three-field format on display-message.
	// Answering it well-formed keeps this hermetic fixture from spuriously clearing pane bindings via the probe's fail-open path.
	fake.answer("display-message", "$0|4321|1700000000", nil)
	fake.answerFormat(liveBoxFormat, "", errors.New("AttachArgv must never query the live window size"))
	fake.answer("list-panes", goodAttachListPanes, nil)
	return e, fake
}

// wantBareAttachArgv builds the expected five-element argv for e, asserted element by
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

// assertResizedUnpinned fails unless fake saw exactly the two window-resized unpins on target and no entry that installs one.
func assertResizedUnpinned(t *testing.T, fake *fakeTmux, target string) {
	t.Helper()
	var want [][]string
	if resizedUnpinCount() > 0 {
		want = wantResizedUnpins(target)
	}
	if got := fake.SetHookCalls(windowResizedHookName); !slices.EqualFunc(got, want, slices.Equal[[]string]) {
		t.Errorf("set-hook calls naming %s = %v, want exactly the unpins %v", windowResizedHookName, got, want)
	}
}

// TestAttachArgv_EveryOtherDegradedPathYieldsBareArgv covers the pre-flight's remaining paths:
// no client size, has-session failing, a pane listing that errors and a state with no strand owning a present pane.
// Every one must yield exactly the bare argv, asserted element by element.
// A path that gets past the session check still pins the geometry options, so it also unpins the window-resized hook and pins the unseen-clear hook.
// The unpins land on the strand window where one resolves and on the session's current window where none does.
// has-session failing returns before any pin.
func TestAttachArgv_EveryOtherDegradedPathYieldsBareArgv(t *testing.T) {
	tests := []struct {
		name          string
		strands       []Strand
		cols, rows    int
		mutate        func(*fakeTmux)
		wantPreflight bool
		// sessionTarget marks a path whose strand window does not resolve, so the pins land on the session's current window.
		sessionTarget bool
	}{
		{"UnknownClientSize", goodAttachStrands(), 0, 0, nil, true, false},
		{"HasSessionFails", goodAttachStrands(), 80, 24, func(f *fakeTmux) { f.answer("has-session", "", errors.New("boom")) }, false, false},
		{"NoStrandOwnsAPresentPane", nil, 80, 24, nil, true, true},
		{"ListPanesErrors", goodAttachStrands(), 80, 24, func(f *fakeTmux) { f.answer("list-panes", "", errors.New("boom")) }, true, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e, fake := newAttachTestEngine(t, tt.strands)
			if tt.mutate != nil {
				tt.mutate(fake)
			}

			assertBareArgv(t, e, e.AttachArgv(tt.cols, tt.rows))

			if !tt.wantPreflight {
				if hooks := fake.ArgvFor("set-hook"); len(hooks) != 0 {
					t.Errorf("set-hook calls = %v, want none before the session check passes", hooks)
				}
				return
			}
			target := fakeStrandWindow
			if tt.sessionTarget {
				target = exactSessionWindowTarget(e.SessionName())
			}
			assertResizedUnpinned(t, fake, target)
			assertUnseenClearHookPinned(t, e, fake)
		})
	}
}

// TestAttachArgv_PreflightOnAKnownGoodSession pins the pre-flight of one known-good AttachArgv call:
// AttachArgv itself issues every geometry pin (not a second exported call the CLI has to remember),
// and the window-resized unpin comes after the state and pane list are read.
// It issues no pane-set mutation and leaves reed.json untouched; it does mutate window options and the hook pins, so "never mutates" is scoped to the pane set.
// A set-hook error then does not change a single element of the returned argv.
//
//testtiming:keep pins one known-good AttachArgv call issuing every geometry pin itself, the window-resized unpin in both scopes after list-panes, no pane-set mutation, reed.json left untouched, and a failing set-hook leaving the argv unchanged; its covering tests run this code without asserting it
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
	assertBareArgv(t, e, want)

	// The five bar pins plus the window-size pin, the one follow-command pin (set, or unset when the jump cannot be composed), the build-stamp unset, the window marker, the strand pane's two options and the console marker on the recorded Selvage pane.
	wantSetOptionCalls := 12 + wantShellPins
	if setOptions := fake.ArgvFor("set-option"); len(setOptions) != wantSetOptionCalls {
		t.Fatalf("AttachArgv() issued %d set-option calls, want %d: %v", len(setOptions), wantSetOptionCalls, setOptions)
	}

	assertStrandOptionsReasserted(t, fake, wantShellPins)

	calls := fake.Calls()
	listPanesIdx, firstSetHookIdx := -1, -1
	for i, argv := range calls {
		switch {
		case callVerb(argv) == "list-panes" && listPanesIdx == -1:
			listPanesIdx = i
		case argv[0] == "set-hook" && containsArg(argv, windowResizedHookName) && firstSetHookIdx == -1:
			firstSetHookIdx = i
		}
	}
	if resizedUnpinCount() > 0 {
		if listPanesIdx == -1 || firstSetHookIdx == -1 {
			t.Fatalf("calls = %v, want a list-panes and a window-resized set-hook call", calls)
		}
		if firstSetHookIdx <= listPanesIdx {
			t.Errorf("calls = %v, want the first set-hook call (index %d) after list-panes (index %d)", calls, firstSetHookIdx, listPanesIdx)
		}
	}
	assertResizedUnpinned(t, fake, fakeStrandWindow)
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
		t.Errorf("AttachArgv() with failing set-hook = %v, want %v (a failing set-hook must not change the argv)", got, want)
	}
	if len(fake.ArgvFor("set-hook")) == hooksBefore {
		t.Error("no set-hook call recorded despite the failing hook, want the unpin statements still attempted")
	}
}

// assertStrandOptionsReasserted asserts the pre-flight marked the strand window and set the pane options on the live bound pane %2, and none on the console pane %1.
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
			if !containsArg(argv, "%2") {
				t.Errorf("@strand set with %v, want it on the bound pane %%2 only", argv)
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

// relevantWindowFixture returns the state and the scripted window listings of a session with the shell window @0, the overview @3 at index 0 and two agent windows, alpha @1 and beta @2 in start order.
// alphaDead and betaDead mark that agent's pane dead.
// The strands also hold the strand of goodAttachStrands, whose pane is not listed.
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

// TestAttachArgv_SelectsTheRelevantWindow pins the window selection AttachArgv appends to the bare argv:
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
		wantWindow string
	}{
		{name: "NewestLiveAgent", cols: cols, rows: rows, wantWindow: "@2"},
		{name: "NewestAgentDeadFallsBackToTheOlderLiveAgent", betaDead: true, cols: cols, rows: rows, wantWindow: "@1"},
		{name: "NoLiveAgentFallsBackToTheOverview", alphaDead: true, betaDead: true, cols: cols, rows: rows, wantWindow: "@3"},
		{name: "NeitherAgentNorOverviewAppendsNothing", noOwn: true, cols: cols, rows: rows},
		{name: "AnotherClientWithFollowOffAppendsNothing", clients: "tty0 80 24", follow: "off", cols: cols, rows: rows},
		{name: "AnotherClientWithFollowUnsetSelectsTheRelevantWindow", clients: "tty0 80 24", follow: "", cols: cols, rows: rows, wantWindow: "@2"},
		{name: "AnotherClientWithFollowOnSelectsTheRelevantWindow", clients: "tty0 80 24", follow: "on", cols: cols, rows: rows, wantWindow: "@2"},
		{name: "AnotherClientWithAFailedFollowReadAppendsNothing", clients: "tty0 80 24", followErr: errors.New("boom"), cols: cols, rows: rows},
		{name: "FailedClientListingWithFollowOnSelectsTheRelevantWindow", clientsErr: errors.New("boom"), follow: "on", cols: cols, rows: rows, wantWindow: "@2"},
		{name: "UnknownClientSizeStillSelectsTheRelevantWindow", wantWindow: "@2"},
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

			got := e.AttachArgv(tt.cols, tt.rows)

			followReads := 0
			for _, call := range fake.Calls() {
				if call[0] == "set-option" && slices.Contains(call, "@lyx_follow") {
					t.Errorf("attach issued %v, want it never to write the follow state", call)
				}
				if call[0] == "display-message" && call[len(call)-1] == "#{@lyx_follow}" {
					followReads++
				}
			}
			if wantRead := followReads == 1; wantRead != otherClient || followReads > 1 {
				t.Errorf("follow reads = %d, want one read exactly when another client is attached or the listing failed (%v)", followReads, otherClient)
			}

			want := wantBareAttachArgv(e)
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

		assertBareArgv(t, e, got)
		if strings.Contains(buf.String(), "reed: another client is attached") {
			t.Errorf("log output = %q, want no multi-client warning for a same-size client", buf.String())
		}
	})

	t.Run("DifferentSizeClient_OneWarningLine", func(t *testing.T) {
		e, fake := newAttachTestEngine(t, goodAttachStrands())
		fake.answer("list-clients", "tty0 100 40", nil)
		buf := logcapture.CaptureVerbose(t)

		got := e.AttachArgv(cols, rows)

		assertBareArgv(t, e, got)
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

		assertBareArgv(t, e, got)
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

		assertBareArgv(t, e, got)
		if !strings.Contains(buf.String(), "reed: failed to list attached clients") {
			t.Errorf("log output = %q, want the list-clients round-trip failure logged", buf.String())
		}
	})
}
