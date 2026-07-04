// rules_test.go golden-tests the composed Rules entry point: top bands
// pinned above the stack, the below-parent stack ordered by parent chain,
// hidden-strand exclusion, mixed top+stack+hidden sets, empty/single-strand
// edges, the checksum-prefix invariant, and the own-window rejection error.

package render

import (
	"strings"
	"testing"
)

// belowParentChain returns a root->mid->active three-level parent chain,
// all AnchorBelowParent and live, with distinct pane ids. root stays full
// (shrink:false) even though it is mid's ancestor; mid collapses
// (shrink:true) since it is blocked waiting on active. This is the fixture
// the golden below-parent and mixed-set cases build on.
func belowParentChain() []Strand {
	return []Strand{
		{GUID: "root", Parent: "", PaneID: "%1", Live: true, Display: Display{Anchor: AnchorBelowParent, ShrinkWhenWaitingOnChild: false}},
		{GUID: "mid", Parent: "root", PaneID: "%2", Live: true, Display: Display{Anchor: AnchorBelowParent, ShrinkWhenWaitingOnChild: true}},
		{GUID: "active", Parent: "mid", PaneID: "%3", Live: true, Display: Display{Anchor: AnchorBelowParent}},
	}
}

func TestRulesGolden(t *testing.T) {
	params := Params{TopBandRows: 3, CollapsedStripRows: 2, MinFullRows: 3}

	tests := []struct {
		name      string
		strands   []Strand
		box       Box
		wantBody  string
		wantFocus string
	}{
		{
			// Regression for orch_04 review 04 finding #2: with >=2 top
			// strands and zero stack strands, nothing but the last top band
			// is left to fill the rest of box.H, so it must stretch — a
			// fixed-height last band here would leave 13 rows unaccounted
			// for (box.H 20 minus two 3-row bands minus one 1-row divider),
			// producing a window_layout string select-layout rejects.
			name: "TopPinnedAsFixedBandAboveStack",
			strands: []Strand{
				{GUID: "ta", PaneID: "%10", Live: true, Display: Display{Anchor: AnchorTop}},
				{GUID: "tb", PaneID: "%20", Live: true, Display: Display{Anchor: AnchorTop}},
			},
			box:       Box{X: 0, Y: 0, W: 100, H: 20},
			wantBody:  "100x20,0,0[100x3,0,0,10,100x16,0,4,20]",
			// With no below-parent stack, focus falls back to the LAST top
			// band (the stretched one): leaving it unset would let psmux
			// park the active pane on an arbitrary 1-row band after
			// select-layout.
			wantFocus: "%20",
		},
		{
			// A second, distinct (3 top, 0 stack) instance of the same
			// finding #2 combination, pinning that the stretch always lands
			// on the LAST top band specifically (not split across all of
			// them, and not the first).
			name: "ThreeTopBandsNoStackLastBandAbsorbsRemainder",
			strands: []Strand{
				{GUID: "ta", PaneID: "%10", Live: true, Display: Display{Anchor: AnchorTop}},
				{GUID: "tb", PaneID: "%20", Live: true, Display: Display{Anchor: AnchorTop}},
				{GUID: "tc", PaneID: "%30", Live: true, Display: Display{Anchor: AnchorTop}},
			},
			box:       Box{X: 0, Y: 0, W: 100, H: 15},
			wantBody:  "100x15,0,0[100x3,0,0,10,100x3,0,4,20,100x7,0,8,30]",
			wantFocus: "%30", // stack empty: the last (stretched) top band is the fallback focus
		},
		{
			name:      "BelowParentFormsBottomDominantStackOrderedByParentChain",
			strands:   belowParentChain(),
			box:       Box{X: 0, Y: 0, W: 100, H: 21}, // usable = 21 - 2 dividers = 19
			wantBody:  "100x21,0,0[100x8,0,0,1,100x2,0,9,2,100x9,0,12,3]",
			wantFocus: "%3", // bottom-most/active default
		},
		{
			name: "HiddenStrandsExcludedFromString",
			strands: append(belowParentChain(),
				Strand{GUID: "h", PaneID: "%99", Live: true, Display: Display{Anchor: AnchorHidden}},
			),
			box:       Box{X: 0, Y: 0, W: 100, H: 21},
			wantBody:  "100x21,0,0[100x8,0,0,1,100x2,0,9,2,100x9,0,12,3]", // identical to the no-hidden case
			wantFocus: "%3",
		},
		{
			name: "MixedTopStackHidden",
			strands: append(
				[]Strand{
					{GUID: "ta", PaneID: "%10", Live: true, Display: Display{Anchor: AnchorTop}},
					{GUID: "tb", PaneID: "%20", Live: true, Display: Display{Anchor: AnchorTop}},
					{GUID: "h", PaneID: "%99", Live: true, Display: Display{Anchor: AnchorHidden}},
				},
				belowParentChain()...,
			),
			box: Box{X: 0, Y: 0, W: 100, H: 29}, // 8 rows of top bands + dividers, then the same 21-row stack region
			wantBody: "100x29,0,0[" +
				"100x3,0,0,10,100x3,0,4,20," +
				"100x8,0,8,1,100x2,0,17,2,100x9,0,20,3]",
			wantFocus: "%3",
		},
		{
			name:      "EmptyStrandsProducesEmptyPaneGroup",
			strands:   nil,
			box:       Box{X: 0, Y: 0, W: 50, H: 10},
			wantBody:  "50x10,0,0[]",
			wantFocus: "",
		},
		{
			name: "SingleStrandFillsTheWholeBox",
			strands: []Strand{
				{GUID: "only", PaneID: "%7", Live: true, Display: Display{Anchor: AnchorBelowParent}},
			},
			box:       Box{X: 0, Y: 0, W: 80, H: 12},
			wantBody:  "80x12,0,0[80x12,0,0,7]",
			wantFocus: "%7",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			layout, focus, err := Rules(tt.strands, tt.box, params, nil)
			if err != nil {
				t.Fatalf("Rules() unexpected error: %v", err)
			}

			wantLayout := wrapLayout(tt.wantBody)
			if layout != wantLayout {
				t.Errorf("Rules() layout = %q, want %q", layout, wantLayout)
			}
			if focus != tt.wantFocus {
				t.Errorf("Rules() focus = %q, want %q", focus, tt.wantFocus)
			}

			// The checksum prefix must always equal layoutChecksum(body),
			// for every case, not just the golden ones above.
			csum, body := layout[:4], layout[5:]
			if want := layoutChecksum(body); csum != want {
				t.Errorf("checksum prefix = %q, want %q (body=%q)", csum, want, body)
			}

			// hidden strands (GUID "h") must never appear in the emitted
			// pane group.
			for _, s := range tt.strands {
				if s.Display.Anchor == AnchorHidden && strings.Contains(layout, s.PaneID) {
					t.Errorf("hidden strand pane id %q leaked into layout %q", s.PaneID, layout)
				}
			}
		})
	}
}

func TestRulesOwnWindowReturnsError(t *testing.T) {
	strands := []Strand{
		{GUID: "a", PaneID: "%1", Live: true, Display: Display{Anchor: AnchorOwnWindow}},
	}
	box := Box{X: 0, Y: 0, W: 100, H: 20}
	p := Params{TopBandRows: 3, CollapsedStripRows: 2, MinFullRows: 3}

	layout, focus, err := Rules(strands, box, p, nil)
	if err == nil {
		t.Fatal("Rules() with an own-window strand: expected error, got nil")
	}
	if layout != "" || focus != "" {
		t.Errorf("Rules() on error = (%q, %q), want both empty", layout, focus)
	}
}

func TestRulesFocusPrefersDeclaredFocusStrandOverDefault(t *testing.T) {
	// root declares Focus explicitly; without that flag the default would
	// pick the bottom-most/active strand instead.
	strands := belowParentChain()
	strands[0].Display.Focus = true

	box := Box{X: 0, Y: 0, W: 100, H: 21}
	p := Params{TopBandRows: 3, CollapsedStripRows: 2, MinFullRows: 3}

	_, focus, err := Rules(strands, box, p, nil)
	if err != nil {
		t.Fatalf("Rules() unexpected error: %v", err)
	}
	if want := "%1"; focus != want {
		t.Errorf("Rules() focus = %q, want %q (the strand that declared Focus)", focus, want)
	}
}

func TestRulesIsPureRepeatedCallsMatch(t *testing.T) {
	strands := belowParentChain()
	box := Box{X: 0, Y: 0, W: 100, H: 21}
	p := Params{TopBandRows: 3, CollapsedStripRows: 2, MinFullRows: 3}

	layout1, focus1, err1 := Rules(strands, box, p, nil)
	layout2, focus2, err2 := Rules(strands, box, p, nil)
	if err1 != nil || err2 != nil {
		t.Fatalf("Rules() unexpected errors: %v, %v", err1, err2)
	}
	if layout1 != layout2 || focus1 != focus2 {
		t.Errorf("Rules() is not pure: (%q,%q) != (%q,%q)", layout1, focus1, layout2, focus2)
	}
}

func TestRulesPaneOrderResequencesCellsToPhysicalOrder(t *testing.T) {
	// psmux applies layout cells positionally to the window's current pane
	// order and ignores the pane numbers in the string; panes cannot be
	// physically reordered (swap-pane/move-pane are silently non-functional
	// on psmux 3.3.4). So when the physical order diverges from the intended
	// table order — e.g. a resumed strand's fresh pane split in at the
	// bottom — Rules must emit each pane's cell at that pane's physical
	// position, with the pane keeping its own intended height.
	strands := []Strand{
		{GUID: "ta", PaneID: "%10", Live: true, Display: Display{Anchor: AnchorTop}},
		{GUID: "tb", PaneID: "%20", Live: true, Display: Display{Anchor: AnchorTop}},
	}
	box := Box{X: 0, Y: 0, W: 100, H: 20}
	p := Params{TopBandRows: 3, CollapsedStripRows: 2, MinFullRows: 3}

	// Physical order inverted vs table order: %20 sits on top.
	layout, focus, err := Rules(strands, box, p, []string{"%20", "%10"})
	if err != nil {
		t.Fatalf("Rules() unexpected error: %v", err)
	}
	// %20 keeps its intended stretched height (16) but is emitted first (at
	// y=0); %10 keeps its intended 3-row band but lands at the bottom.
	wantBody := "100x20,0,0[100x16,0,0,20,100x3,0,17,10]"
	if want := layoutChecksum(wantBody) + "," + wantBody; layout != want {
		t.Errorf("Rules() layout = %q, want %q", layout, want)
	}
	// Focus stays id-based: the last (stretched) top band, regardless of
	// where it physically sits.
	if want := "%20"; focus != want {
		t.Errorf("Rules() focus = %q, want %q", focus, want)
	}
}

func TestRulesPaneOrderUnknownIDsKeepIntendedTailOrder(t *testing.T) {
	strands := []Strand{
		{GUID: "ta", PaneID: "%10", Live: true, Display: Display{Anchor: AnchorTop}},
		{GUID: "tb", PaneID: "%20", Live: true, Display: Display{Anchor: AnchorTop}},
	}
	box := Box{X: 0, Y: 0, W: 100, H: 20}
	p := Params{TopBandRows: 3, CollapsedStripRows: 2, MinFullRows: 3}

	// paneOrder naming only panes render never placed: the intended order
	// survives at the tail, identical to the nil-paneOrder shape.
	withUnknown, _, err1 := Rules(strands, box, p, []string{"%99"})
	intended, _, err2 := Rules(strands, box, p, nil)
	if err1 != nil || err2 != nil {
		t.Fatalf("Rules() unexpected errors: %v, %v", err1, err2)
	}
	if withUnknown != intended {
		t.Errorf("Rules() with unknown-only paneOrder = %q, want intended order %q", withUnknown, intended)
	}
}
