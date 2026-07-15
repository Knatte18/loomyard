// apply_test.go verifies planLayout produces the same layout string and
// focus target render.Rules would for an equivalent canonical strand table
// (reusing render's golden expectations), and that applyLayoutLocked skips
// tmux entirely when fewer than two panes are live — both hermetic, no
// live tmux required.

package muxengine

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/muxengine/render"
)

func TestPlanLayout_MatchesRenderRulesForCanonicalStrandTable(t *testing.T) {
	e := newTestEngine(t)
	e.cfg.Width, e.cfg.Height = 100, 21
	e.cfg.CollapsedStripRows, e.cfg.MinFullRows = 2, 3

	// The same root->mid->active below-parent chain rules_test.go's
	// belowParentChain fixture uses: root stays full, mid collapses
	// (blocked waiting on active), active is bottom/focused.
	st := &MuxState{Strands: []Strand{
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

	gotLayout, gotFocus, err := e.planLayout(st, live)
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

	st := &MuxState{Strands: []Strand{
		{GUID: "only", PaneID: "%7", Display: render.Display{Anchor: render.AnchorBelowParent}},
		{GUID: "hid", PaneID: "%8", Display: render.Display{Anchor: render.AnchorHidden}},
	}}
	live := []LivePane{{ID: "%7"}, {ID: "%8"}}

	gotLayout, gotFocus, err := e.planLayout(st, live)
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

func TestApplyLayoutLocked_SkipsTmuxWhenFewerThanTwoLivePanes(t *testing.T) {
	// e's tmux points at a nonexistent binary (newTestEngine's fixture);
	// if applyLayoutLocked issued select-layout/select-pane here it would
	// fail loudly rather than silently passing.
	e := newTestEngine(t)

	st := &MuxState{Strands: []Strand{
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
		st := &MuxState{}
		if err := e.applyLayoutLocked(st, []LivePane{{ID: "%1"}, {ID: "%2"}}); err != nil {
			t.Errorf("applyLayoutLocked(no strands, 2 panes) = %v, want nil", err)
		}
	})

	t.Run("OnlyUnboundAndHiddenStrands", func(t *testing.T) {
		st := &MuxState{Strands: []Strand{
			{GUID: "cleared", PaneID: "", Display: render.Display{Anchor: render.AnchorBelowParent}},
			{GUID: "hid", PaneID: "%1", Display: render.Display{Anchor: render.AnchorHidden}},
		}}
		if err := e.applyLayoutLocked(st, []LivePane{{ID: "%1"}, {ID: "%2"}}); err != nil {
			t.Errorf("applyLayoutLocked(no placeable strand, 2 panes) = %v, want nil", err)
		}
	})
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
