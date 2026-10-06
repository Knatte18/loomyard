// apply_test.go verifies planLayout produces the same layout string and focus target render.Rules
// would for an equivalent canonical strand table (reusing render's golden expectations),
// that planLayout is handed its box by the caller and issues no tmux query of its own (the
// told-box seam batch 2's AttachArgv relies on), and that applyLayoutLocked skips tmux entirely
// when fewer than two panes are live — all hermetic, no live tmux required.

package reedengine

import (
	"errors"
	"runtime"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/reedengine/render"
	"github.com/Knatte18/loomyard/internal/shell"
)

// planCollapsedRows and planMinFullRows are the layout parameters the planLayout tests pin on the engine and hand render.Rules as the expectation,
// so the two sides cannot drift apart silently.
const (
	planCollapsedRows = 2
	planMinFullRows   = 3
)

// liveRenderStrands maps strands to the render strands planLayout is expected to hand render.Rules, every one live.
func liveRenderStrands(strands []Strand) []render.Strand {
	out := make([]render.Strand, len(strands))
	for i, s := range strands {
		out[i] = render.Strand{GUID: s.GUID, Parent: s.Parent, PaneID: s.PaneID, Live: true, Display: s.Display}
	}
	return out
}

// TestPlanLayout_MatchesRenderRules pins that planLayout lays out against exactly the box its caller hands it,
// never the configured width and height, issues no tmux query of its own, and yields the layout and focus render.Rules gives for the same strands.
//
//testtiming:keep pins planLayout producing the layout and focus render.Rules gives for a canonical below-parent chain and for a hidden strand, laying out against the told box rather than the configured size and issuing no tmux query; its covering tests run this code without asserting it
func TestPlanLayout_MatchesRenderRules(t *testing.T) {
	below := render.Display{Anchor: render.AnchorBelowParent}
	cases := []struct {
		name        string
		cfgW, cfgH  int
		strands     []Strand
		live        []LivePane
		box         render.Box
		wantPaneIDs []string
	}{
		{
			// The same root->mid->active below-parent chain rules_test.go's belowParentChain fixture uses:
			// root stays full, mid collapses (blocked waiting on active), active is bottom/focused.
			name: "CanonicalBelowParentChain",
			strands: []Strand{
				{GUID: "root", PaneID: "%1", Display: below},
				{GUID: "mid", Parent: "root", PaneID: "%2", Display: below},
				{GUID: "active", Parent: "mid", PaneID: "%3", Display: below},
			},
			live: []LivePane{{ID: "%1"}, {ID: "%2"}, {ID: "%3"}},
			box:  render.Box{X: 0, Y: 0, W: 100, H: 21},
		},
		{
			name: "HiddenStrandExcludedFromPlacement",
			cfgW: 80, cfgH: 12,
			strands: []Strand{
				{GUID: "only", PaneID: "%7", Display: below},
				{GUID: "hid", PaneID: "%8", Display: render.Display{Anchor: render.AnchorHidden}},
			},
			live: []LivePane{{ID: "%7"}, {ID: "%8"}},
			box:  render.Box{X: 0, Y: 0, W: 80, H: 12},
		},
		{
			name: "ToldBoxWinsOverConfiguredSize",
			cfgW: 999, cfgH: 111,
			strands:     []Strand{{GUID: "only", PaneID: "%7", Display: below}},
			live:        []LivePane{{ID: "%7"}},
			box:         render.Box{X: 0, Y: 0, W: 80, H: 12},
			wantPaneIDs: []string{"%7"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := newTestEngine(t)
			if tc.cfgW != 0 {
				e.cfg.Width, e.cfg.Height = tc.cfgW, tc.cfgH
			}
			e.cfg.CollapsedRows, e.cfg.MinFullRows = planCollapsedRows, planMinFullRows
			fake := installFakeTmux(t, e)

			gotLayout, gotFocus, err := e.planLayout(&ReedState{Strands: tc.strands}, tc.live, tc.box)
			if err != nil {
				t.Fatalf("planLayout() unexpected error: %v", err)
			}
			wantLayout, wantFocus, err := render.Rules(liveRenderStrands(tc.strands), tc.box,
				render.Params{CollapsedRows: planCollapsedRows, MinFullRows: planMinFullRows}, tc.wantPaneIDs)
			if err != nil {
				t.Fatalf("render.Rules() unexpected error: %v", err)
			}
			if gotLayout != wantLayout || gotFocus != wantFocus {
				t.Errorf("planLayout() = (%q,%q), want (%q,%q)", gotLayout, gotFocus, wantLayout, wantFocus)
			}
			if calls := fake.Calls(); len(calls) != 0 {
				t.Errorf("planLayout() issued tmux calls %v; want zero tmux round trips", calls)
			}
		})
	}
}

// TestApplyLayoutLocked_GuardSkips pins that applyLayoutLocked and applyLayoutLockedOpts skip tmux entirely, set-hook clear included so a previously installed array survives,
// when fewer than two panes are live or no strand owns a present pane (tmux answers an empty-cell layout by destroying every pane in the session);
// the Opts form returns the zero applyResult.
//
//testtiming:keep pins both apply entry points issuing no tmux call at all when fewer than two panes are live or no strand owns a present pane, the Opts form returning the zero result; its covering tests run this code without asserting it
func TestApplyLayoutLocked_GuardSkips(t *testing.T) {
	below := render.Display{Anchor: render.AnchorBelowParent}
	cases := []struct {
		name string
		st   *ReedState
		live []LivePane
	}{
		{"ZeroLivePanes", &ReedState{Strands: []Strand{{GUID: "only", PaneID: "%1", Display: below}}}, nil},
		{"OneLivePane", &ReedState{Strands: []Strand{{GUID: "only", PaneID: "%1", Display: below}}}, []LivePane{{ID: "%1"}}},
		{"NoStrandsAtAll", &ReedState{}, []LivePane{{ID: "%1"}, {ID: "%2"}}},
		{
			"OnlyUnboundAndHiddenStrands",
			&ReedState{Strands: []Strand{
				{GUID: "cleared", PaneID: "", Display: below},
				{GUID: "hid", PaneID: "%1", Display: render.Display{Anchor: render.AnchorHidden}},
			}},
			[]LivePane{{ID: "%1"}, {ID: "%2"}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := newTestEngine(t)
			fake := installFakeTmux(t, e)

			if err := e.applyLayoutLocked(tc.st, tc.live); err != nil {
				t.Errorf("applyLayoutLocked() = %v, want nil", err)
			}
			got, err := e.applyLayoutLockedOpts(tc.st, tc.live, applyOpts{})
			if err != nil {
				t.Fatalf("applyLayoutLockedOpts() error = %v, want nil", err)
			}
			if got != (applyResult{}) {
				t.Errorf("applyLayoutLockedOpts() = %+v, want the zero applyResult", got)
			}
			if calls := fake.Calls(); len(calls) != 0 {
				t.Errorf("guard skip issued tmux calls %v, want none", calls)
			}
		})
	}
}

// TestPlanLayout_StaleSelvagePaneIDNeverEmittedAsLayoutCell pins planLayout's Selvage presence
// filter: a stale absent Selvage must render as if no Selvage existed.
//
//testtiming:keep pins a stale Selvage pane id rendering exactly the no-Selvage plan and a present dead Selvage still holding a layout cell; its covering tests run this code without asserting it
func TestPlanLayout_StaleSelvagePaneIDNeverEmittedAsLayoutCell(t *testing.T) {
	e := newTestEngine(t)
	e.cfg.CollapsedRows, e.cfg.MinFullRows = planCollapsedRows, planMinFullRows
	e.cfg.Selvage.HeightRows = 1

	strands := []Strand{
		{GUID: "a", PaneID: "%1", Display: render.Display{Anchor: render.AnchorBelowParent}},
		{GUID: "b", PaneID: "%2", Display: render.Display{Anchor: render.AnchorBelowParent}},
	}
	renderStrands := []render.Strand{
		{GUID: "a", PaneID: "%1", Live: true, Display: render.Display{Anchor: render.AnchorBelowParent}},
		{GUID: "b", PaneID: "%2", Live: true, Display: render.Display{Anchor: render.AnchorBelowParent}},
	}
	live := []LivePane{{ID: "%1", Top: 0}, {ID: "%2", Top: 11}}

	// Stale Selvage: %9 is nowhere in live, so the plan must equal the
	// no-Selvage plan bit for bit.
	st := &ReedState{Strands: strands, SelvagePaneID: "%9"}
	gotLayout, gotFocus, err := e.planLayout(st, live, render.Box{X: 0, Y: 0, W: 100, H: 21})
	if err != nil {
		t.Fatalf("planLayout() unexpected error: %v", err)
	}
	wantLayout, wantFocus, err := render.Rules(renderStrands,
		render.Box{X: 0, Y: 0, W: 100, H: 21},
		render.Params{CollapsedRows: planCollapsedRows, MinFullRows: planMinFullRows},
		[]string{"%1", "%2"})
	if err != nil {
		t.Fatalf("render.Rules() unexpected error: %v", err)
	}
	if gotLayout != wantLayout || gotFocus != wantFocus {
		t.Errorf("planLayout() with stale Selvage = (%q,%q), want the no-Selvage plan (%q,%q)", gotLayout, gotFocus, wantLayout, wantFocus)
	}

	// Present-but-dead Selvage corpse: the cell must still be emitted, same
	// as any dead-but-present pane the layout has to enumerate.
	liveWithCorpse := append([]LivePane{{ID: "%9", Dead: true, Top: 0}}, []LivePane{{ID: "%1", Top: 2}, {ID: "%2", Top: 12}}...)
	gotLayout, _, err = e.planLayout(st, liveWithCorpse, render.Box{X: 0, Y: 0, W: 100, H: 21})
	if err != nil {
		t.Fatalf("planLayout() with corpse Selvage unexpected error: %v", err)
	}
	wantLayout, _, err = render.Rules(renderStrands,
		render.Box{X: 0, Y: 0, W: 100, H: 21},
		render.Params{CollapsedRows: planCollapsedRows, MinFullRows: planMinFullRows, Selvage: render.Selvage{PaneID: "%9", HeightRows: 1}},
		[]string{"%9", "%1", "%2"})
	if err != nil {
		t.Fatalf("render.Rules() with Selvage unexpected error: %v", err)
	}
	if gotLayout != wantLayout {
		t.Errorf("planLayout() with corpse Selvage = %q, want the with-Selvage plan %q (a present corpse still occupies a layout slot)", gotLayout, wantLayout)
	}
}

// TestApplyLayoutLockedOpts_SkipFocusSuppressesSelectPane pins the focus-preservation contract:
// SkipFocus issues select-layout and no select-pane, while the zero applyOpts on the same fixture
// issues both.
//
//testtiming:keep pins SkipFocus issuing select-layout without select-pane while the zero options and the applyLayoutLocked wrapper issue both; its covering tests run this code without asserting it
func TestApplyLayoutLockedOpts_SkipFocusSuppressesSelectPane(t *testing.T) {
	newFixture := func(t *testing.T) (*Engine, *ReedState, []LivePane, *fakeTmux) {
		e := newTestEngine(t)
		fake := installFakeTmux(t, e)
		fake.answer("display-message", "100 21", nil)
		st := &ReedState{Strands: []Strand{
			{GUID: "only", PaneID: "%1", Display: render.Display{Anchor: render.AnchorBelowParent, Focus: true}},
		}}
		live := []LivePane{{ID: "%1"}, {ID: "%2"}}
		return e, st, live, fake
	}

	t.Run("SkipFocusTrue", func(t *testing.T) {
		e, st, live, fake := newFixture(t)
		got, err := e.applyLayoutLockedOpts(st, live, applyOpts{SkipFocus: true})
		if err != nil {
			t.Fatalf("applyLayoutLockedOpts() error = %v, want nil", err)
		}
		if !got.Applied {
			t.Errorf("applyLayoutLockedOpts() Applied = false, want true")
		}
		if !containsArg(fake.Sequence(), "select-layout") {
			t.Errorf("calls = %v, want select-layout", fake.Sequence())
		}
		if containsArg(fake.Sequence(), "select-pane") {
			t.Errorf("calls = %v, want no select-pane", fake.Sequence())
		}
	})

	t.Run("ZeroOptsIssuesBoth", func(t *testing.T) {
		e, st, live, fake := newFixture(t)
		got, err := e.applyLayoutLockedOpts(st, live, applyOpts{})
		if err != nil {
			t.Fatalf("applyLayoutLockedOpts() error = %v, want nil", err)
		}
		if !got.Applied {
			t.Errorf("applyLayoutLockedOpts() Applied = false, want true")
		}
		if !containsArg(fake.Sequence(), "select-layout") {
			t.Errorf("calls = %v, want select-layout", fake.Sequence())
		}
		if !containsArg(fake.Sequence(), "select-pane") {
			t.Errorf("calls = %v, want select-pane", fake.Sequence())
		}
	})

	// applyLayoutLocked is the thin wrapper over the zero applyOpts: the full focus half, unabbreviated.
	t.Run("WrapperIssuesBoth", func(t *testing.T) {
		e, st, live, fake := newFixture(t)
		if err := e.applyLayoutLocked(st, live); err != nil {
			t.Fatalf("applyLayoutLocked() = %v, want nil", err)
		}
		if !containsArg(fake.Sequence(), "select-layout") || !containsArg(fake.Sequence(), "select-pane") {
			t.Errorf("applyLayoutLocked() calls = %v, want select-layout and select-pane", fake.Sequence())
		}
	})
}

// TestApplyLayoutLockedOpts_SkipWhenBoxEquals pins the box-equality guard: an equal, live-observed
// box suppresses select-layout and reports Applied: false with the observed box; a differing box
// still applies.
//
//testtiming:keep pins the box-equality guard: an equal live-observed box suppresses select-layout and reports the observed box, a differing box applies, and a degraded fallback box never satisfies the guard; its covering tests run this code without asserting it
func TestApplyLayoutLockedOpts_SkipWhenBoxEquals(t *testing.T) {
	newFixture := func(t *testing.T, answer string) (*Engine, *ReedState, []LivePane, *fakeTmux) {
		e := newTestEngine(t)
		e.cfg.Width, e.cfg.Height = 999, 111
		fake := installFakeTmux(t, e)
		fake.answer("display-message", answer, nil)
		st := &ReedState{Strands: []Strand{
			{GUID: "only", PaneID: "%1", Display: render.Display{Anchor: render.AnchorBelowParent}},
		}}
		live := []LivePane{{ID: "%1"}, {ID: "%2"}}
		return e, st, live, fake
	}

	t.Run("EqualBoxSkips", func(t *testing.T) {
		e, st, live, fake := newFixture(t, "100 21")
		box := render.Box{X: 0, Y: 0, W: 100, H: 21}
		got, err := e.applyLayoutLockedOpts(st, live, applyOpts{SkipWhenBoxEquals: &box})
		if err != nil {
			t.Fatalf("applyLayoutLockedOpts() error = %v, want nil", err)
		}
		if got.Applied {
			t.Errorf("applyLayoutLockedOpts() Applied = true, want false")
		}
		if !got.BoxIsLive || got.Box != box {
			t.Errorf("applyLayoutLockedOpts() = %+v, want BoxIsLive true and Box %+v", got, box)
		}
		if containsArg(fake.Sequence(), "select-layout") {
			t.Errorf("calls = %v, want no select-layout", fake.Sequence())
		}
	})

	t.Run("DifferingBoxApplies", func(t *testing.T) {
		e, st, live, fake := newFixture(t, "80 24")
		box := render.Box{X: 0, Y: 0, W: 100, H: 21}
		got, err := e.applyLayoutLockedOpts(st, live, applyOpts{SkipWhenBoxEquals: &box})
		if err != nil {
			t.Fatalf("applyLayoutLockedOpts() error = %v, want nil", err)
		}
		if !got.Applied || !got.BoxIsLive {
			t.Errorf("applyLayoutLockedOpts() = %+v, want Applied true and BoxIsLive true", got)
		}
		if !containsArg(fake.Sequence(), "select-layout") {
			t.Errorf("calls = %v, want select-layout", fake.Sequence())
		}
	})

	// The degraded case: a fallback box is not an observation and must never satisfy the guard, even
	// when it happens to equal SkipWhenBoxEquals.
	t.Run("DegradedFallbackBoxNeverSatisfiesGuard", func(t *testing.T) {
		e := newTestEngine(t)
		fake := installFakeTmux(t, e)
		fake.answer("display-message", "", errors.New("boom"))
		st := &ReedState{Strands: []Strand{
			{GUID: "only", PaneID: "%1", Display: render.Display{Anchor: render.AnchorBelowParent}},
		}}
		live := []LivePane{{ID: "%1"}, {ID: "%2"}}
		box := render.Box{X: 0, Y: 0, W: 100, H: 21}

		got, err := e.applyLayoutLockedOpts(st, live, applyOpts{SkipWhenBoxEquals: &box})
		if err != nil {
			t.Fatalf("applyLayoutLockedOpts() error = %v, want nil", err)
		}
		if got.BoxIsLive {
			t.Errorf("applyLayoutLockedOpts() BoxIsLive = true, want false (a fallback box is not an observation)")
		}
		if !containsArg(fake.Sequence(), "select-layout") {
			t.Errorf("calls = %v, want select-layout still issued (the guard must not fire on a degraded box)", fake.Sequence())
		}
	})
}

//testtiming:keep pins which strand shapes count as placed: bound and present yes, absent pane, unbound and hidden no; its covering tests run this code without asserting it
func TestAnyPlacedStrand(t *testing.T) {
	present := map[string]bool{"%1": true, "%2": true}
	cases := []struct {
		name    string
		strands []Strand
		want    bool
	}{
		{"NoStrands", nil, false},
		{"BoundPresentVisible", []Strand{{GUID: "a", PaneID: "%1", Display: render.Display{Anchor: render.AnchorBelowParent}}}, true},
		{"BoundAbsentPane", []Strand{{GUID: "a", PaneID: "%9", Display: render.Display{Anchor: render.AnchorBelowParent}}}, false},
		{"UnboundStrand", []Strand{{GUID: "a", PaneID: "", Display: render.Display{Anchor: render.AnchorBelowParent}}}, false},
		{"HiddenStrandNeverPlaced", []Strand{{GUID: "a", PaneID: "%1", Display: render.Display{Anchor: render.AnchorHidden}}}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := anyPlacedStrand(tc.strands, present); got != tc.want {
				t.Errorf("anyPlacedStrand(%s) = %v, want %v", tc.name, got, tc.want)
			}
		})
	}
}

// TestApplyLayoutLocked_InstallsResizePinsAfterSelectLayout pins the install statement's position: a
// successful apply issues the set-hook clear and pin rebuild after select-layout and before
// select-pane, discriminated on the recorded call sequence.
//
//testtiming:keep pins the order of one apply's calls: select-layout, then only set-hook calls starting with the -u clear, then select-pane; its covering tests run this code without asserting it
func TestApplyLayoutLocked_InstallsResizePinsAfterSelectLayout(t *testing.T) {
	e := newTestEngine(t)
	e.cfg.Selvage.HeightRows = 1

	fake := installFakeTmux(t, e)

	st := &ReedState{
		SelvagePaneID: "%9",
		Strands: []Strand{
			{GUID: "root", PaneID: "%1", Display: render.Display{Anchor: render.AnchorBelowParent}},
			{GUID: "child", Parent: "root", PaneID: "%2", Display: render.Display{Anchor: render.AnchorBelowParent, Focus: true}},
		},
	}
	live := []LivePane{{ID: "%9", Top: 0}, {ID: "%1", Top: 2}, {ID: "%2", Top: 4}}

	if err := e.applyLayoutLocked(st, live); err != nil {
		t.Fatalf("applyLayoutLocked() unexpected error: %v", err)
	}

	sequence := fake.Sequence("select-layout", "select-pane", "set-hook")
	wantMinLen := 3 // select-layout, at least the set-hook clear, select-pane
	if len(sequence) < wantMinLen {
		t.Fatalf("sequence = %v, want at least %d entries", sequence, wantMinLen)
	}
	if sequence[0] != "select-layout" {
		t.Fatalf("sequence[0] = %q, want select-layout", sequence[0])
	}
	if sequence[1] != "set-hook" {
		t.Fatalf("sequence[1] = %q, want set-hook (the install statement right after select-layout)", sequence[1])
	}
	if sequence[len(sequence)-1] != "select-pane" {
		t.Fatalf("sequence tail = %q, want select-pane after every set-hook call", sequence[len(sequence)-1])
	}
	for _, step := range sequence[1 : len(sequence)-1] {
		if step != "set-hook" {
			t.Errorf("sequence = %v, want only set-hook calls between select-layout and select-pane", sequence)
		}
	}

	setHooks := fake.ArgvFor("set-hook")
	if len(setHooks) == 0 {
		t.Fatal("no set-hook calls recorded, want at least the clear")
	}
	if !containsArg(setHooks[0], "-u") {
		t.Errorf("first set-hook argv = %v, want the -u clear", setHooks[0])
	}
}

// TestApplyLayoutLocked_ZeroPinsStillIssuesTheClear pins the-clear-is-unconditional-including-zero-pins:
// an apply whose plan yields zero pins — a SelvagePaneID absent from the live set, a lone strand
// with nothing collapsed — still issues the clear, and issues no resize-pane entry behind it.
// The two subtests separate the two opinions a zero-pin rebuild carries: "nothing is pinned" is
// unconditional, while the watchdog's touch entry rides watchdog on/off, so a watchdog: on session
// with nothing to pin still gets told about a resize.
//
//testtiming:keep pins the clear being unconditional on a zero-pin plan and the resize-signal entry riding the watchdog setting; its covering tests run this code without asserting it
func TestApplyLayoutLocked_ZeroPinsStillIssuesTheClear(t *testing.T) {
	newZeroPinApply := func(t *testing.T, watchdog string) (*Engine, *fakeTmux) {
		t.Helper()
		e := newTestEngine(t)
		e.cfg.Selvage.HeightRows = 1
		e.cfg.Watchdog = watchdog

		fake := installFakeTmux(t, e)

		st := &ReedState{
			SelvagePaneID: "%9", // absent from live below, so the mapping blanks it
			Strands: []Strand{
				{GUID: "root", PaneID: "%1", Display: render.Display{Anchor: render.AnchorBelowParent}},
			},
		}
		// %2 is a foreign pane no strand owns: a lone strand is the bottom-most, so it
		// collapses nothing and the plan yields zero pins.
		live := []LivePane{{ID: "%1", Top: 0}, {ID: "%2", Top: 11}}

		if err := e.applyLayoutLocked(st, live); err != nil {
			t.Fatalf("applyLayoutLocked() unexpected error: %v", err)
		}
		return e, fake
	}

	assertClearFirstAndNoPin := func(t *testing.T, fake *fakeTmux) {
		t.Helper()
		setHooks := fake.ArgvFor("set-hook")
		if len(setHooks) == 0 {
			t.Fatal("no set-hook calls recorded, want at least the unconditional clear")
		}
		if !containsArg(setHooks[0], "-u") {
			t.Errorf("first set-hook argv = %v, want the -u clear", setHooks[0])
		}
		for i, argv := range setHooks {
			if strings.HasPrefix(argv[len(argv)-1], "resize-pane ") {
				t.Errorf("set-hook argv[%d] = %v, want no resize-pane entry on a zero-pin plan", i, argv)
			}
		}
	}

	t.Run("WatchdogOffIsTheClearAlone", func(t *testing.T) {
		_, fake := newZeroPinApply(t, "off")
		assertClearFirstAndNoPin(t, fake)
		if setHooks := fake.ArgvFor("set-hook"); len(setHooks) != 1 {
			t.Fatalf("recorded %d set-hook calls, want exactly 1 (the unconditional clear): %v", len(setHooks), setHooks)
		}
	})

	t.Run("WatchdogOnAlsoInstallsTheSignalEntry", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("the hook is never installed on Windows")
		}
		e, fake := newZeroPinApply(t, "on")
		assertClearFirstAndNoPin(t, fake)
		setHooks := fake.ArgvFor("set-hook")
		if len(setHooks) != 2 {
			t.Fatalf("recorded %d set-hook calls, want exactly 2 (the clear plus the resize-signal entry): %v", len(setHooks), setHooks)
		}
		signal := setHooks[1]
		want := resizeHookCommand(shell.ForGOOS(), e.resizeSignalPath())
		if signal[len(signal)-1] != want {
			t.Errorf("second set-hook body = %q, want reed's own touch command %q", signal[len(signal)-1], want)
		}
	})
}

//testtiming:keep pins a failing set-hook leaving the apply successful; its covering tests run this code without asserting it
func TestApplyLayoutLocked_SetHookErrorDoesNotFailApply(t *testing.T) {
	e := newTestEngine(t)
	installFakeTmux(t, e).answer("set-hook", "", errors.New("boom"))

	st := &ReedState{Strands: []Strand{
		{GUID: "a", PaneID: "%1", Display: render.Display{Anchor: render.AnchorBelowParent}},
		{GUID: "b", PaneID: "%2", Display: render.Display{Anchor: render.AnchorBelowParent}},
	}}
	live := []LivePane{{ID: "%1"}, {ID: "%2"}}

	if err := e.applyLayoutLocked(st, live); err != nil {
		t.Fatalf("applyLayoutLocked() = %v, want nil even when set-hook fails", err)
	}
}

//testtiming:keep pins the pane ids coming back ordered by vertical position from an unordered input; its covering tests run this code without asserting it
func TestPaneIDsByTop_SortsByVerticalPosition(t *testing.T) {
	live := []LivePane{
		{ID: "%3", Top: 32},
		{ID: "%1", Top: 0},
		{ID: "%4", Top: 16},
	}
	got := paneIDsByTop(live)
	want := []string{"%1", "%4", "%3"}
	if len(got) != len(want) {
		t.Fatalf("paneIDsByTop = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("paneIDsByTop[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}
