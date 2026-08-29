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

func TestPlanLayout_MatchesRenderRulesForCanonicalStrandTable(t *testing.T) {
	e := newTestEngine(t)
	e.cfg.Width, e.cfg.Height = 100, 21
	e.cfg.CollapsedStripRows, e.cfg.MinFullRows = 2, 3

	// The same root->mid->active below-parent chain rules_test.go's
	// belowParentChain fixture uses: root stays full, mid collapses
	// (blocked waiting on active), active is bottom/focused.
	st := &ReedState{Strands: []Strand{
		{GUID: "root", PaneID: "%1", Display: render.Display{Anchor: render.AnchorBelowParent, ShrinkWhenWaitingOnChild: false}},
		{GUID: "mid", Parent: "root", PaneID: "%2", Display: render.Display{Anchor: render.AnchorBelowParent, ShrinkWhenWaitingOnChild: true}},
		{GUID: "active", Parent: "mid", PaneID: "%3", Display: render.Display{Anchor: render.AnchorBelowParent}},
	}}
	live := []LivePane{{ID: "%1"}, {ID: "%2"}, {ID: "%3"}}

	wantLayout, wantFocus, err := render.Rules([]render.Strand{
		{GUID: "root", PaneID: "%1", Live: true, Display: render.Display{Anchor: render.AnchorBelowParent, ShrinkWhenWaitingOnChild: false}},
		{GUID: "mid", Parent: "root", PaneID: "%2", Live: true, Display: render.Display{Anchor: render.AnchorBelowParent, ShrinkWhenWaitingOnChild: true}},
		{GUID: "active", Parent: "mid", PaneID: "%3", Live: true, Display: render.Display{Anchor: render.AnchorBelowParent}},
	}, render.Box{X: 0, Y: 0, W: 100, H: 21}, render.Params{CollapsedStripRows: 2, MinFullRows: 3}, nil)
	if err != nil {
		t.Fatalf("render.Rules() unexpected error: %v", err)
	}

	gotLayout, gotFocus, err := e.planLayout(st, live, render.Box{X: 0, Y: 0, W: 100, H: 21})
	if err != nil {
		t.Fatalf("planLayout() unexpected error: %v", err)
	}
	if gotLayout != wantLayout {
		t.Errorf("planLayout() layout = %q, want %q", gotLayout, wantLayout)
	}
	if gotFocus != wantFocus {
		t.Errorf("planLayout() focus = %q, want %q", gotFocus, wantFocus)
	}
}

func TestPlanLayout_HiddenStrandExcludedFromPlacement(t *testing.T) {
	e := newTestEngine(t)
	e.cfg.Width, e.cfg.Height = 80, 12
	e.cfg.CollapsedStripRows, e.cfg.MinFullRows = 2, 3

	st := &ReedState{Strands: []Strand{
		{GUID: "only", PaneID: "%7", Display: render.Display{Anchor: render.AnchorBelowParent}},
		{GUID: "hid", PaneID: "%8", Display: render.Display{Anchor: render.AnchorHidden}},
	}}
	live := []LivePane{{ID: "%7"}, {ID: "%8"}}

	gotLayout, gotFocus, err := e.planLayout(st, live, render.Box{X: 0, Y: 0, W: 80, H: 12})
	if err != nil {
		t.Fatalf("planLayout() unexpected error: %v", err)
	}
	wantLayout, wantFocus, err := render.Rules([]render.Strand{
		{GUID: "only", PaneID: "%7", Live: true, Display: render.Display{Anchor: render.AnchorBelowParent}},
		{GUID: "hid", PaneID: "%8", Live: true, Display: render.Display{Anchor: render.AnchorHidden}},
	}, render.Box{X: 0, Y: 0, W: 80, H: 12}, render.Params{CollapsedStripRows: 2, MinFullRows: 3}, nil)
	if err != nil {
		t.Fatalf("render.Rules() unexpected error: %v", err)
	}
	if gotLayout != wantLayout || gotFocus != wantFocus {
		t.Errorf("planLayout() = (%q,%q), want (%q,%q)", gotLayout, gotFocus, wantLayout, wantFocus)
	}
}

// TestPlanLayout_StaleHeaderPaneIDNeverEmittedAsLayoutCell pins planLayout's header presence
// filter: a stale absent header must render as if no header existed.
func TestPlanLayout_StaleHeaderPaneIDNeverEmittedAsLayoutCell(t *testing.T) {
	e := newTestEngine(t)
	e.cfg.Width, e.cfg.Height = 100, 21
	e.cfg.CollapsedStripRows, e.cfg.MinFullRows = 2, 3
	e.cfg.Header.HeightRows = 1

	strands := []Strand{
		{GUID: "a", PaneID: "%1", Display: render.Display{Anchor: render.AnchorBelowParent}},
		{GUID: "b", PaneID: "%2", Display: render.Display{Anchor: render.AnchorBelowParent}},
	}
	renderStrands := []render.Strand{
		{GUID: "a", PaneID: "%1", Live: true, Display: render.Display{Anchor: render.AnchorBelowParent}},
		{GUID: "b", PaneID: "%2", Live: true, Display: render.Display{Anchor: render.AnchorBelowParent}},
	}
	live := []LivePane{{ID: "%1", Top: 0}, {ID: "%2", Top: 11}}

	// Stale header: %9 is nowhere in live, so the plan must equal the
	// no-header plan bit for bit.
	st := &ReedState{Strands: strands, HeaderPaneID: "%9"}
	gotLayout, gotFocus, err := e.planLayout(st, live, render.Box{X: 0, Y: 0, W: 100, H: 21})
	if err != nil {
		t.Fatalf("planLayout() unexpected error: %v", err)
	}
	wantLayout, wantFocus, err := render.Rules(renderStrands,
		render.Box{X: 0, Y: 0, W: 100, H: 21},
		render.Params{CollapsedStripRows: 2, MinFullRows: 3},
		[]string{"%1", "%2"})
	if err != nil {
		t.Fatalf("render.Rules() unexpected error: %v", err)
	}
	if gotLayout != wantLayout || gotFocus != wantFocus {
		t.Errorf("planLayout() with stale header = (%q,%q), want the no-header plan (%q,%q)", gotLayout, gotFocus, wantLayout, wantFocus)
	}

	// Present-but-dead header corpse: the cell must still be emitted, same
	// as any dead-but-present pane the layout has to enumerate.
	liveWithCorpse := append([]LivePane{{ID: "%9", Dead: true, Top: 0}}, []LivePane{{ID: "%1", Top: 2}, {ID: "%2", Top: 12}}...)
	gotLayout, _, err = e.planLayout(st, liveWithCorpse, render.Box{X: 0, Y: 0, W: 100, H: 21})
	if err != nil {
		t.Fatalf("planLayout() with corpse header unexpected error: %v", err)
	}
	wantLayout, _, err = render.Rules(renderStrands,
		render.Box{X: 0, Y: 0, W: 100, H: 21},
		render.Params{CollapsedStripRows: 2, MinFullRows: 3, Header: render.Header{PaneID: "%9", HeightRows: 1}},
		[]string{"%9", "%1", "%2"})
	if err != nil {
		t.Fatalf("render.Rules() with header unexpected error: %v", err)
	}
	if gotLayout != wantLayout {
		t.Errorf("planLayout() with corpse header = %q, want the with-header plan %q (a present corpse still occupies a layout slot)", gotLayout, wantLayout)
	}
}

// TestPlanLayout_UsesTheToldBoxAndIssuesNoQuery pins the told-box seam batch 2's AttachArgv relies
// on: planLayout must lay out against exactly the box its caller hands it, never the configured
// width/height, and must never touch tmux itself to get there.
func TestPlanLayout_UsesTheToldBoxAndIssuesNoQuery(t *testing.T) {
	e := newTestEngine(t)
	e.cfg.Width, e.cfg.Height = 999, 111
	e.cfg.CollapsedStripRows, e.cfg.MinFullRows = 2, 3

	hookCalled := false
	e.tmux.execHook = func(capture bool, args ...string) (string, error) {
		hookCalled = true
		return "", errors.New("planLayout must never touch tmux")
	}

	toldBox := render.Box{X: 0, Y: 0, W: 80, H: 12}
	st := &ReedState{Strands: []Strand{
		{GUID: "only", PaneID: "%7", Display: render.Display{Anchor: render.AnchorBelowParent}},
	}}
	live := []LivePane{{ID: "%7"}}

	gotLayout, _, err := e.planLayout(st, live, toldBox)
	if err != nil {
		t.Fatalf("planLayout() unexpected error: %v", err)
	}
	if hookCalled {
		t.Error("planLayout() invoked the tmux hook; want zero tmux round trips")
	}

	wantLayout, _, err := render.Rules([]render.Strand{
		{GUID: "only", PaneID: "%7", Live: true, Display: render.Display{Anchor: render.AnchorBelowParent}},
	}, toldBox, render.Params{CollapsedStripRows: 2, MinFullRows: 3}, []string{"%7"})
	if err != nil {
		t.Fatalf("render.Rules() unexpected error: %v", err)
	}
	if gotLayout != wantLayout {
		t.Errorf("planLayout() layout = %q, want %q (the told box's dimensions, not the configured %dx%d pair)", gotLayout, wantLayout, e.cfg.Width, e.cfg.Height)
	}
}

func TestApplyLayoutLocked_SkipsTmuxWhenFewerThanTwoLivePanes(t *testing.T) {
	// e's tmux points at a nonexistent binary (newTestEngine's fixture);
	// if applyLayoutLocked issued select-layout/select-pane here it would
	// fail loudly rather than silently passing.
	e := newTestEngine(t)

	st := &ReedState{Strands: []Strand{
		{GUID: "only", PaneID: "%1", Display: render.Display{Anchor: render.AnchorBelowParent}},
	}}

	t.Run("ZeroLivePanes", func(t *testing.T) {
		if err := e.applyLayoutLocked(st, nil); err != nil {
			t.Errorf("applyLayoutLocked(0 panes) = %v, want nil", err)
		}
	})

	t.Run("OneLivePane", func(t *testing.T) {
		if err := e.applyLayoutLocked(st, []LivePane{{ID: "%1"}}); err != nil {
			t.Errorf("applyLayoutLocked(1 pane) = %v, want nil", err)
		}
	})
}

func TestApplyLayoutLocked_SkipsTmuxWhenNoStrandOwnsAPresentPane(t *testing.T) {
	// e's tmux points at a nonexistent binary (newTestEngine's fixture);
	// if applyLayoutLocked issued select-layout here it would fail loudly.
	// Two live panes but no strand owning either: the rendered layout would
	// enumerate ZERO cells, and tmux answers an empty-cell layout by
	// destroying every pane in the session — so the apply must be skipped.
	e := newTestEngine(t)

	t.Run("NoStrandsAtAll", func(t *testing.T) {
		st := &ReedState{}
		if err := e.applyLayoutLocked(st, []LivePane{{ID: "%1"}, {ID: "%2"}}); err != nil {
			t.Errorf("applyLayoutLocked(no strands, 2 panes) = %v, want nil", err)
		}
	})

	t.Run("OnlyUnboundAndHiddenStrands", func(t *testing.T) {
		st := &ReedState{Strands: []Strand{
			{GUID: "cleared", PaneID: "", Display: render.Display{Anchor: render.AnchorBelowParent}},
			{GUID: "hid", PaneID: "%1", Display: render.Display{Anchor: render.AnchorHidden}},
		}}
		if err := e.applyLayoutLocked(st, []LivePane{{ID: "%1"}, {ID: "%2"}}); err != nil {
			t.Errorf("applyLayoutLocked(no placeable strand, 2 panes) = %v, want nil", err)
		}
	})
}

// TestApplyLayoutLockedOpts_GuardSkipsReturnZeroResult pins that applyLayoutLockedOpts' two inherited
// guard-skip paths return a zero applyResult (Applied false, BoxIsLive false), issue no
// select-layout, and return nil — exactly like applyLayoutLocked did before this batch.
func TestApplyLayoutLockedOpts_GuardSkipsReturnZeroResult(t *testing.T) {
	e := newTestEngine(t)
	hookCalled := false
	e.tmux.execHook = func(capture bool, args ...string) (string, error) {
		hookCalled = true
		return "", errors.New("must not be called")
	}

	t.Run("FewerThanTwoLivePanes", func(t *testing.T) {
		hookCalled = false
		st := &ReedState{Strands: []Strand{
			{GUID: "only", PaneID: "%1", Display: render.Display{Anchor: render.AnchorBelowParent}},
		}}
		got, err := e.applyLayoutLockedOpts(st, []LivePane{{ID: "%1"}}, applyOpts{})
		if err != nil {
			t.Fatalf("applyLayoutLockedOpts() error = %v, want nil", err)
		}
		if got != (applyResult{}) {
			t.Errorf("applyLayoutLockedOpts() = %+v, want the zero applyResult", got)
		}
		if hookCalled {
			t.Error("applyLayoutLockedOpts() issued a tmux call, want none")
		}
	})

	t.Run("NoStrandOwnsAPresentPane", func(t *testing.T) {
		hookCalled = false
		st := &ReedState{}
		got, err := e.applyLayoutLockedOpts(st, []LivePane{{ID: "%1"}, {ID: "%2"}}, applyOpts{})
		if err != nil {
			t.Fatalf("applyLayoutLockedOpts() error = %v, want nil", err)
		}
		if got != (applyResult{}) {
			t.Errorf("applyLayoutLockedOpts() = %+v, want the zero applyResult", got)
		}
		if hookCalled {
			t.Error("applyLayoutLockedOpts() issued a tmux call, want none")
		}
	})
}

// TestApplyLayoutLockedOpts_SkipFocusSuppressesSelectPane pins the focus-preservation contract:
// SkipFocus issues select-layout and no select-pane, while the zero applyOpts on the same fixture
// issues both.
func TestApplyLayoutLockedOpts_SkipFocusSuppressesSelectPane(t *testing.T) {
	newFixture := func(t *testing.T) (*Engine, *ReedState, []LivePane, *[]string) {
		e := newTestEngine(t)
		e.cfg.Width, e.cfg.Height = 100, 21
		var calls []string
		e.tmux.execHook = func(capture bool, args ...string) (string, error) {
			calls = append(calls, args[0])
			if args[0] == "display-message" {
				return "100 21", nil
			}
			return "", nil
		}
		st := &ReedState{Strands: []Strand{
			{GUID: "only", PaneID: "%1", Display: render.Display{Anchor: render.AnchorBelowParent, Focus: true}},
		}}
		live := []LivePane{{ID: "%1"}, {ID: "%2"}}
		return e, st, live, &calls
	}

	t.Run("SkipFocusTrue", func(t *testing.T) {
		e, st, live, calls := newFixture(t)
		got, err := e.applyLayoutLockedOpts(st, live, applyOpts{SkipFocus: true})
		if err != nil {
			t.Fatalf("applyLayoutLockedOpts() error = %v, want nil", err)
		}
		if !got.Applied {
			t.Errorf("applyLayoutLockedOpts() Applied = false, want true")
		}
		if !containsArg(*calls, "select-layout") {
			t.Errorf("calls = %v, want select-layout", *calls)
		}
		if containsArg(*calls, "select-pane") {
			t.Errorf("calls = %v, want no select-pane", *calls)
		}
	})

	t.Run("ZeroOptsIssuesBoth", func(t *testing.T) {
		e, st, live, calls := newFixture(t)
		got, err := e.applyLayoutLockedOpts(st, live, applyOpts{})
		if err != nil {
			t.Fatalf("applyLayoutLockedOpts() error = %v, want nil", err)
		}
		if !got.Applied {
			t.Errorf("applyLayoutLockedOpts() Applied = false, want true")
		}
		if !containsArg(*calls, "select-layout") {
			t.Errorf("calls = %v, want select-layout", *calls)
		}
		if !containsArg(*calls, "select-pane") {
			t.Errorf("calls = %v, want select-pane", *calls)
		}
	})
}

// TestApplyLayoutLockedOpts_SkipWhenBoxEquals pins the box-equality guard: an equal, live-observed
// box suppresses select-layout and reports Applied: false with the observed box; a differing box
// still applies.
func TestApplyLayoutLockedOpts_SkipWhenBoxEquals(t *testing.T) {
	newFixture := func(t *testing.T, answer string) (*Engine, *ReedState, []LivePane, *[]string) {
		e := newTestEngine(t)
		e.cfg.Width, e.cfg.Height = 999, 111
		var calls []string
		e.tmux.execHook = func(capture bool, args ...string) (string, error) {
			calls = append(calls, args[0])
			if args[0] == "display-message" {
				return answer, nil
			}
			return "", nil
		}
		st := &ReedState{Strands: []Strand{
			{GUID: "only", PaneID: "%1", Display: render.Display{Anchor: render.AnchorBelowParent}},
		}}
		live := []LivePane{{ID: "%1"}, {ID: "%2"}}
		return e, st, live, &calls
	}

	t.Run("EqualBoxSkips", func(t *testing.T) {
		e, st, live, calls := newFixture(t, "100 21")
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
		if containsArg(*calls, "select-layout") {
			t.Errorf("calls = %v, want no select-layout", *calls)
		}
	})

	t.Run("DifferingBoxApplies", func(t *testing.T) {
		e, st, live, calls := newFixture(t, "80 24")
		box := render.Box{X: 0, Y: 0, W: 100, H: 21}
		got, err := e.applyLayoutLockedOpts(st, live, applyOpts{SkipWhenBoxEquals: &box})
		if err != nil {
			t.Fatalf("applyLayoutLockedOpts() error = %v, want nil", err)
		}
		if !got.Applied || !got.BoxIsLive {
			t.Errorf("applyLayoutLockedOpts() = %+v, want Applied true and BoxIsLive true", got)
		}
		if !containsArg(*calls, "select-layout") {
			t.Errorf("calls = %v, want select-layout", *calls)
		}
	})

	// The degraded case: a fallback box is not an observation and must never satisfy the guard, even
	// when it happens to equal SkipWhenBoxEquals.
	t.Run("DegradedFallbackBoxNeverSatisfiesGuard", func(t *testing.T) {
		e := newTestEngine(t)
		e.cfg.Width, e.cfg.Height = 100, 21
		var calls []string
		e.tmux.execHook = func(capture bool, args ...string) (string, error) {
			calls = append(calls, args[0])
			if args[0] == "display-message" {
				return "", errors.New("boom")
			}
			return "", nil
		}
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
		if !containsArg(calls, "select-layout") {
			t.Errorf("calls = %v, want select-layout still issued (the guard must not fire on a degraded box)", calls)
		}
	})
}

// TestApplyLayoutLocked_WrapperStillIssuesBothSelectLayoutAndSelectPane pins that applyLayoutLocked,
// now a thin wrapper over applyLayoutLockedOpts, keeps its exact pre-batch-2 behaviour: the full
// focus half, unabbreviated.
func TestApplyLayoutLocked_WrapperStillIssuesBothSelectLayoutAndSelectPane(t *testing.T) {
	e := newTestEngine(t)
	e.cfg.Width, e.cfg.Height = 100, 21

	var calls []string
	e.tmux.execHook = func(capture bool, args ...string) (string, error) {
		calls = append(calls, args[0])
		if args[0] == "display-message" {
			return "100 21", nil
		}
		return "", nil
	}

	st := &ReedState{Strands: []Strand{
		{GUID: "only", PaneID: "%1", Display: render.Display{Anchor: render.AnchorBelowParent, Focus: true}},
	}}
	live := []LivePane{{ID: "%1"}, {ID: "%2"}}

	if err := e.applyLayoutLocked(st, live); err != nil {
		t.Fatalf("applyLayoutLocked() = %v, want nil", err)
	}

	if !containsArg(calls, "select-layout") {
		t.Errorf("applyLayoutLocked() calls = %v, want select-layout", calls)
	}
	if !containsArg(calls, "select-pane") {
		t.Errorf("applyLayoutLocked() calls = %v, want select-pane", calls)
	}
}

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

// applyHookRecorder captures every call applyLayoutLocked issues through the execHook seam, in order,
// so a test can discriminate on the recorded call sequence rather than on call count alone. setHookArgvs
// holds the full argv of every set-hook call, in call order, alongside sequence's "set-hook" entries.
type applyHookRecorder struct {
	sequence     []string
	setHookArgvs [][]string
}

// newApplyRecordingHook builds the execHook closure a test installs on e.tmux, recording every call
// into rec and answering select-layout/select-pane/set-hook with success — the fixture apply_test.go
// lacked before this card, built from scratch rather than extending an existing single-purpose
// closure.
func newApplyRecordingHook(rec *applyHookRecorder) func(capture bool, args ...string) (string, error) {
	return func(capture bool, args ...string) (string, error) {
		switch args[0] {
		case "select-layout":
			rec.sequence = append(rec.sequence, "select-layout")
			return "", nil
		case "select-pane":
			rec.sequence = append(rec.sequence, "select-pane")
			return "", nil
		case "set-hook":
			rec.sequence = append(rec.sequence, "set-hook")
			rec.setHookArgvs = append(rec.setHookArgvs, append([]string{}, args...))
			return "", nil
		default:
			return "", nil
		}
	}
}

// TestApplyLayoutLocked_InstallsResizePinsAfterSelectLayout pins the install statement's position: a
// successful apply issues the set-hook clear and pin rebuild after select-layout and before
// select-pane, discriminated on the recorded call sequence.
func TestApplyLayoutLocked_InstallsResizePinsAfterSelectLayout(t *testing.T) {
	e := newTestEngine(t)
	e.cfg.Width, e.cfg.Height = 100, 21
	e.cfg.CollapsedStripRows, e.cfg.MinFullRows = 2, 3
	e.cfg.Header.HeightRows = 1

	rec := &applyHookRecorder{}
	e.tmux.execHook = newApplyRecordingHook(rec)

	st := &ReedState{
		HeaderPaneID: "%9",
		Strands: []Strand{
			{GUID: "root", PaneID: "%1", Display: render.Display{Anchor: render.AnchorBelowParent, ShrinkWhenWaitingOnChild: true}},
			{GUID: "child", Parent: "root", PaneID: "%2", Display: render.Display{Anchor: render.AnchorBelowParent, Focus: true}},
		},
	}
	live := []LivePane{{ID: "%9", Top: 0}, {ID: "%1", Top: 2}, {ID: "%2", Top: 4}}

	if err := e.applyLayoutLocked(st, live); err != nil {
		t.Fatalf("applyLayoutLocked() unexpected error: %v", err)
	}

	wantMinLen := 3 // select-layout, at least the set-hook clear, select-pane
	if len(rec.sequence) < wantMinLen {
		t.Fatalf("sequence = %v, want at least %d entries", rec.sequence, wantMinLen)
	}
	if rec.sequence[0] != "select-layout" {
		t.Fatalf("sequence[0] = %q, want select-layout", rec.sequence[0])
	}
	if rec.sequence[1] != "set-hook" {
		t.Fatalf("sequence[1] = %q, want set-hook (the install statement right after select-layout)", rec.sequence[1])
	}
	if rec.sequence[len(rec.sequence)-1] != "select-pane" {
		t.Fatalf("sequence tail = %q, want select-pane after every set-hook call", rec.sequence[len(rec.sequence)-1])
	}
	for _, step := range rec.sequence[1 : len(rec.sequence)-1] {
		if step != "set-hook" {
			t.Errorf("sequence = %v, want only set-hook calls between select-layout and select-pane", rec.sequence)
		}
	}

	if len(rec.setHookArgvs) == 0 {
		t.Fatal("no set-hook calls recorded, want at least the clear")
	}
	clear := rec.setHookArgvs[0]
	if containsArg(clear, "-u") == false {
		t.Errorf("first set-hook argv = %v, want the -u clear", clear)
	}
}

// TestApplyLayoutLocked_ZeroPinsStillIssuesTheClear pins the-clear-is-unconditional-including-zero-pins:
// an apply whose plan yields zero pins — a HeaderPaneID absent from the live set, no strip strand
// present — still issues the clear, and issues no resize-pane entry behind it.
// The two subtests separate the two opinions a zero-pin rebuild carries: "nothing is pinned" is
// unconditional, while the watchdog's touch entry rides watchdog on/off, so a watchdog: on session
// with nothing to pin still gets told about a resize.
func TestApplyLayoutLocked_ZeroPinsStillIssuesTheClear(t *testing.T) {
	newZeroPinApply := func(t *testing.T, watchdog string) (*Engine, *applyHookRecorder) {
		t.Helper()
		e := newTestEngine(t)
		e.cfg.Width, e.cfg.Height = 100, 21
		e.cfg.CollapsedStripRows, e.cfg.MinFullRows = 2, 3
		e.cfg.Header.HeightRows = 1
		e.cfg.Watchdog = watchdog

		rec := &applyHookRecorder{}
		e.tmux.execHook = newApplyRecordingHook(rec)

		st := &ReedState{
			HeaderPaneID: "%9", // absent from live below, so the mapping blanks it
			Strands: []Strand{
				{GUID: "root", PaneID: "%1", Display: render.Display{Anchor: render.AnchorBelowParent}},
				{GUID: "child", Parent: "root", PaneID: "%2", Display: render.Display{Anchor: render.AnchorBelowParent}},
			},
		}
		live := []LivePane{{ID: "%1", Top: 0}, {ID: "%2", Top: 11}}

		if err := e.applyLayoutLocked(st, live); err != nil {
			t.Fatalf("applyLayoutLocked() unexpected error: %v", err)
		}
		return e, rec
	}

	assertClearFirstAndNoPin := func(t *testing.T, rec *applyHookRecorder) {
		t.Helper()
		if len(rec.setHookArgvs) == 0 {
			t.Fatal("no set-hook calls recorded, want at least the unconditional clear")
		}
		if !containsArg(rec.setHookArgvs[0], "-u") {
			t.Errorf("first set-hook argv = %v, want the -u clear", rec.setHookArgvs[0])
		}
		for i, argv := range rec.setHookArgvs {
			if strings.HasPrefix(argv[len(argv)-1], "resize-pane ") {
				t.Errorf("set-hook argv[%d] = %v, want no resize-pane entry on a zero-pin plan", i, argv)
			}
		}
	}

	t.Run("WatchdogOffIsTheClearAlone", func(t *testing.T) {
		_, rec := newZeroPinApply(t, "off")
		assertClearFirstAndNoPin(t, rec)
		if len(rec.setHookArgvs) != 1 {
			t.Fatalf("recorded %d set-hook calls, want exactly 1 (the unconditional clear): %v", len(rec.setHookArgvs), rec.setHookArgvs)
		}
	})

	t.Run("WatchdogOnAlsoInstallsTheSignalEntry", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("the hook is never installed on Windows")
		}
		e, rec := newZeroPinApply(t, "on")
		assertClearFirstAndNoPin(t, rec)
		if len(rec.setHookArgvs) != 2 {
			t.Fatalf("recorded %d set-hook calls, want exactly 2 (the clear plus the resize-signal entry): %v", len(rec.setHookArgvs), rec.setHookArgvs)
		}
		signal := rec.setHookArgvs[1]
		want := resizeHookCommand(shell.ForGOOS(), e.resizeSignalPath())
		if signal[len(signal)-1] != want {
			t.Errorf("second set-hook body = %q, want reed's own touch command %q", signal[len(signal)-1], want)
		}
	})
}

// TestApplyLayoutLocked_GuardSkipIssuesNoSetHookCall pins guard-skip-leaves-a-stale-array-deliberately:
// neither of applyLayoutLocked's two guards reaches any set-hook call at all, INCLUDING no clear, so a
// previously installed array survives a guard-skip.
func TestApplyLayoutLocked_GuardSkipIssuesNoSetHookCall(t *testing.T) {
	e := newTestEngine(t)

	t.Run("FewerThanTwoLivePanes", func(t *testing.T) {
		rec := &applyHookRecorder{}
		e.tmux.execHook = newApplyRecordingHook(rec)
		st := &ReedState{Strands: []Strand{{GUID: "only", PaneID: "%1", Display: render.Display{Anchor: render.AnchorBelowParent}}}}

		if err := e.applyLayoutLocked(st, []LivePane{{ID: "%1"}}); err != nil {
			t.Fatalf("applyLayoutLocked() unexpected error: %v", err)
		}
		if len(rec.sequence) != 0 {
			t.Errorf("sequence = %v, want zero calls (including no clear)", rec.sequence)
		}
	})

	t.Run("NoStrandOwnsAPresentPane", func(t *testing.T) {
		rec := &applyHookRecorder{}
		e.tmux.execHook = newApplyRecordingHook(rec)
		st := &ReedState{}

		if err := e.applyLayoutLocked(st, []LivePane{{ID: "%1"}, {ID: "%2"}}); err != nil {
			t.Fatalf("applyLayoutLocked() unexpected error: %v", err)
		}
		if len(rec.sequence) != 0 {
			t.Errorf("sequence = %v, want zero calls (including no clear)", rec.sequence)
		}
	})
}

// TestApplyLayoutLocked_SetHookErrorDoesNotFailApply pins hook-failure-is-non-fatal-everywhere: a
// set-hook returning an error does not make applyLayoutLocked return an error.
func TestApplyLayoutLocked_SetHookErrorDoesNotFailApply(t *testing.T) {
	e := newTestEngine(t)
	e.cfg.Width, e.cfg.Height = 100, 21
	e.cfg.CollapsedStripRows, e.cfg.MinFullRows = 2, 3

	e.tmux.execHook = func(capture bool, args ...string) (string, error) {
		if args[0] == "set-hook" {
			return "", errors.New("boom")
		}
		return "", nil
	}

	st := &ReedState{Strands: []Strand{
		{GUID: "a", PaneID: "%1", Display: render.Display{Anchor: render.AnchorBelowParent}},
		{GUID: "b", PaneID: "%2", Display: render.Display{Anchor: render.AnchorBelowParent}},
	}}
	live := []LivePane{{ID: "%1"}, {ID: "%2"}}

	if err := e.applyLayoutLocked(st, live); err != nil {
		t.Fatalf("applyLayoutLocked() = %v, want nil even when set-hook fails", err)
	}
}

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
