// rules_test.go golden-tests the composed Rules entry point: the below-parent stack ordered by
// parent chain, hidden-strand exclusion, empty/single-strand/parent-child edges, the
// checksum-prefix invariant, the own-window rejection error, pane-order resequencing to physical
// pane position, and the Selvage bottom-band enumeration (Params.Selvage).
// It also pins the two layout regimes a real (as opposed to config-pinned) terminal box makes
// reachable: a budget-satisfying box where height.go's clamps never fire, and a too-short box where
// they must — the latter with a companion assertion that no clamped cell height is ever non-positive.

package render

import (
	"regexp"
	"strconv"
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
	params := Params{CollapsedStripRows: 2, MinFullRows: 3}

	tests := []struct {
		name      string
		strands   []Strand
		box       Box
		selvage   Selvage
		wantBody  string
		wantFocus string
	}{
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
		{
			// The loom shape endorsed by discussion decision
			// childless-full-height-is-acceptable's counterpart: a
			// below-parent root parent with a single below-parent child
			// collapses the parent to CollapsedStripRows once the child is
			// present, and the child takes the remainder. The height-layer
			// form of this is height_test.go's
			// TestStackHeightsActiveStrictlyTallestWithSingleAncestor; this
			// case only proves the same shape survives through Rules.
			name: "BelowParentRootChildCollapsesRootToStripChildTakesRemainder",
			strands: []Strand{
				{GUID: "parent", PaneID: "%1", Live: true, Display: Display{Anchor: AnchorBelowParent, ShrinkWhenWaitingOnChild: true}},
				{GUID: "child", Parent: "parent", PaneID: "%2", Live: true, Display: Display{Anchor: AnchorBelowParent}},
			},
			box:       Box{X: 0, Y: 0, W: 100, H: 15},
			wantBody:  "100x15,0,0[100x2,0,0,1,100x12,0,3,2]",
			wantFocus: "%2",
		},
		{
			// A live terminal is routinely 24 or 30 rows, unlike the 220x50
			// box a pinned config used to always hand Rules — a box this
			// short means clampBandHeight/clampToFit now govern the common
			// case rather than almost never firing. This row's box has room
			// for the Selvage band, its one-row divider, the collapsed strip
			// at CollapsedStripRows, and both full panes above MinFullRows,
			// so no clamp fires: band=2 (unclamped: floor=3, maxBand=
			// box.H-1-3=20, 2<=20), stack region {Y:0,H:21}, usable=21-2
			// dividers=19, stripDemand=2 (mid collapses), fullRemaining=17
			// split 8/9 between root and active (remainder to active); the
			// band cell lands last, at Y=box.H-2=22.
			name:      "SelvagePresentBudgetSatisfyingPreservesConfiguredSelvageAndStripHeights",
			strands:   belowParentChain(),
			box:       Box{X: 0, Y: 0, W: 100, H: 24},
			selvage:   Selvage{PaneID: "%h", HeightRows: 2},
			wantBody:  "100x24,0,0[100x8,0,0,1,100x2,0,9,2,100x9,0,12,3,100x2,0,22,h]",
			wantFocus: "%3",
		},
		{
			// The same strand fixture against a box too short for those
			// budgets: band=2 stays unclamped (floor=3, maxBand=
			// box.H-1-3=4, 2<=4), stack region {Y:0,H:5}, usable=5-2
			// dividers=3, stripDemand=2 (mid), fullRemaining=1 split 0/1
			// between root and active (remainder to active) — root's natural
			// 0 borrows 1 row via clampToFit's priority-1 reclaim, which the
			// strip (mid) repays by shrinking from its natural 2 down to 1,
			// leaving every stack cell at exactly 1 row; the band cell lands
			// last, at Y=box.H-2=6.
			name:      "SelvagePresentClampedRowNoCellEverNonPositive",
			strands:   belowParentChain(),
			box:       Box{X: 0, Y: 0, W: 100, H: 8},
			selvage:   Selvage{PaneID: "%h", HeightRows: 2},
			wantBody:  "100x8,0,0[100x1,0,0,1,100x1,0,2,2,100x1,0,4,3,100x2,0,6,h]",
			wantFocus: "%3",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := params
			if tt.selvage != (Selvage{}) {
				p.Selvage = tt.selvage
			}
			layout, focus, err := Rules(tt.strands, tt.box, p, nil)
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

// cellHeightPattern matches one PANE or GROUP cell's leading "<w>x<h>," dimension field of a tmux
// window_layout string, capturing the height.
var cellHeightPattern = regexp.MustCompile(`\d+x(\d+),`)

// TestRulesClampedRowNeverEmitsANonPositiveCellHeight is the companion assertion
// TestRulesGolden's table shape cannot express: every cell height in the
// SelvagePresentClampedRowNoCellEverNonPositive golden row must be at least 1, no matter how far the
// clamp had to reach. clampToFit's own documented last-resort branch (the active pane absorbing
// whatever the earlier priority passes could not reclaim) is deliberately permitted to leave the
// emitted cell heights summing to MORE than box.H when the window is shorter than the pane count —
// this test does not exercise that branch (this fixture's clamp resolves in priority 1), and a
// future reader should not read an over-sum in some OTHER fixture as a defect this test would have
// caught.
func TestRulesClampedRowNeverEmitsANonPositiveCellHeight(t *testing.T) {
	params := Params{CollapsedStripRows: 2, MinFullRows: 3, Selvage: Selvage{PaneID: "%h", HeightRows: 2}}
	box := Box{X: 0, Y: 0, W: 100, H: 8}

	layout, _, err := Rules(belowParentChain(), box, params, nil)
	if err != nil {
		t.Fatalf("Rules() unexpected error: %v", err)
	}

	for _, match := range cellHeightPattern.FindAllStringSubmatch(layout, -1) {
		height, err := strconv.Atoi(match[1])
		if err != nil {
			t.Fatalf("cell height %q did not parse as an integer: %v", match[1], err)
		}
		if height < 1 {
			t.Errorf("Rules() clamped layout %q contains a non-positive cell height %d", layout, height)
		}
	}
}

func TestRulesOwnWindowReturnsError(t *testing.T) {
	strands := []Strand{
		{GUID: "a", PaneID: "%1", Live: true, Display: Display{Anchor: AnchorOwnWindow}},
	}
	box := Box{X: 0, Y: 0, W: 100, H: 20}
	p := Params{CollapsedStripRows: 2, MinFullRows: 3}

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
	p := Params{CollapsedStripRows: 2, MinFullRows: 3}

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
	p := Params{CollapsedStripRows: 2, MinFullRows: 3}

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
		{GUID: "root", PaneID: "%10", Live: true, Display: Display{Anchor: AnchorBelowParent}},
		{GUID: "child", Parent: "root", PaneID: "%20", Live: true, Display: Display{Anchor: AnchorBelowParent}},
	}
	box := Box{X: 0, Y: 0, W: 100, H: 20}
	p := Params{CollapsedStripRows: 2, MinFullRows: 3}

	// Physical order inverted vs table order: %20 (child) sits on top.
	layout, focus, err := Rules(strands, box, p, []string{"%20", "%10"})
	if err != nil {
		t.Fatalf("Rules() unexpected error: %v", err)
	}
	// child keeps its intended remainder-bearing height (10) but is emitted
	// first (at y=0); root keeps its intended base height (9) but lands at
	// the bottom.
	wantBody := "100x20,0,0[100x10,0,0,20,100x9,0,11,10]"
	if want := layoutChecksum(wantBody) + "," + wantBody; layout != want {
		t.Errorf("Rules() layout = %q, want %q", layout, want)
	}
	// Focus stays id-based: the active/bottom strand, regardless of where it
	// physically sits.
	if want := "%20"; focus != want {
		t.Errorf("Rules() focus = %q, want %q", focus, want)
	}
}

// TestRulesSelvageBandEnumeratesEveryStrandCellPlusSelvage asserts the Selvage bottom-band shape: the
// below-parent stack laid out in the shrunk region at the top, followed by a fixed-height Selvage
// cell at the bottom — the emitted window_layout must enumerate every strand cell plus the Selvage
// cell so the live-pane count the caller's select-layout applies against matches tmux's actual pane
// set.
func TestRulesSelvageBandEnumeratesEveryStrandCellPlusSelvage(t *testing.T) {
	params := Params{
		CollapsedStripRows: 2,
		MinFullRows:        3,
		Selvage:            Selvage{PaneID: "%h", HeightRows: 3},
	}
	box := Box{X: 0, Y: 0, W: 100, H: 21}

	layout, focus, err := Rules(belowParentChain(), box, params, nil)
	if err != nil {
		t.Fatalf("Rules() unexpected error: %v", err)
	}

	// bandHeight=3 (unclamped: with the Selvage band's own one-row divider
	// budget subtracted first (box.H-1=20), MinFullRows=3 leaves 17 rows for
	// the stack, well above the natural split's needs). The stack region is
	// {X:0,Y:0,W:100,H:17}: usable=17-2 dividers=15, stripDemand=2 (mid
	// collapses to CollapsedStripRows), fullRemaining=13 split 6/7 between
	// root and active (remainder to active). The Selvage cell lands last, at
	// Y=box.H-3=18.
	wantBody := "100x21,0,0[100x6,0,0,1,100x2,0,7,2,100x7,0,10,3,100x3,0,18,h]"
	if want := wrapLayout(wantBody); layout != want {
		t.Errorf("Rules() with Selvage layout = %q, want %q", layout, want)
	}
	if want := "%3"; focus != want {
		t.Errorf("Rules() with Selvage focus = %q, want %q (Selvage never affects focus)", focus, want)
	}
}

// TestRulesSelvageWithNoPlacedStrandClaimsWholeBoxAsSoleCell pins the empty-stack Selvage shape: with
// a Selvage pane and ZERO placed strands, Rules must emit the Selvage band as a bracket-less
// single-cell body claiming the whole box — the same shape tmux reports for a one-pane window — never
// a zero-height Selvage cell inside a group (the fable-header-r1 finding: bandHeight stayed 0 on
// this path and bandSelvage emitted a literal "Wx0" cell, exactly the shape
// TestHeaderNeverGetsZeroHeightLayoutCell exists to forbid, while the doc comment claimed the
// band "may claim the whole box").
// Unreachable through applyLayoutLocked today (anyPlacedStrand gates the apply),
// but Rules is a pure function whose contract must hold for any caller.
func TestRulesSelvageWithNoPlacedStrandClaimsWholeBoxAsSoleCell(t *testing.T) {
	box := Box{X: 0, Y: 0, W: 100, H: 21}
	p := Params{CollapsedStripRows: 2, MinFullRows: 3, Selvage: Selvage{PaneID: "%h", HeightRows: 3}}

	// nil strands and an all-filtered stack (a hidden strand) must both
	// produce the sole-band shape.
	for name, strands := range map[string][]Strand{
		"NilStrands":       nil,
		"OnlyHiddenStrand": {{GUID: "hid", PaneID: "%9", Live: true, Display: Display{Anchor: AnchorHidden}}},
	} {
		layout, focus, err := Rules(strands, box, p, nil)
		if err != nil {
			t.Fatalf("%s: Rules() unexpected error: %v", name, err)
		}
		if want := wrapLayout("100x21,0,0,h"); layout != want {
			t.Errorf("%s: Rules() layout = %q, want the sole-band body %q", name, layout, want)
		}
		if focus != "" {
			t.Errorf("%s: Rules() focus = %q, want \"\" (no placed strand to focus)", name, focus)
		}
	}
}

// TestRulesNoSelvagePreservesPreSelvageBehavior asserts a zero-value Params.Selvage (empty PaneID)
// produces byte-identical output to omitting Selvage entirely — every pre-Selvage caller must be
// unaffected.
func TestRulesNoSelvagePreservesPreSelvageBehavior(t *testing.T) {
	strands := belowParentChain()
	box := Box{X: 0, Y: 0, W: 100, H: 21}

	withZeroSelvage, focus1, err1 := Rules(strands, box, Params{CollapsedStripRows: 2, MinFullRows: 3, Selvage: Selvage{}}, nil)
	without, focus2, err2 := Rules(strands, box, Params{CollapsedStripRows: 2, MinFullRows: 3}, nil)
	if err1 != nil || err2 != nil {
		t.Fatalf("Rules() unexpected errors: %v, %v", err1, err2)
	}
	if withZeroSelvage != without || focus1 != focus2 {
		t.Errorf("Rules() with zero-value Selvage = (%q,%q), want identical to omitting Selvage entirely (%q,%q)", withZeroSelvage, focus1, without, focus2)
	}
}

func TestRulesPaneOrderUnknownIDsKeepIntendedTailOrder(t *testing.T) {
	strands := []Strand{
		{GUID: "root", PaneID: "%10", Live: true, Display: Display{Anchor: AnchorBelowParent}},
		{GUID: "child", Parent: "root", PaneID: "%20", Live: true, Display: Display{Anchor: AnchorBelowParent}},
	}
	box := Box{X: 0, Y: 0, W: 100, H: 20}
	p := Params{CollapsedStripRows: 2, MinFullRows: 3}

	// paneOrder naming only a pane render never placed: the intended order
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

// paneCellPattern matches one PANE cell of a tmux window_layout string, "<w>x<h>,<x>,<y>,<paneNum>",
// capturing the pane number. A GROUP header has the same leading "<w>x<h>,<x>,<y>" but is followed
// by '[' rather than a fourth field, so it never matches — which is exactly the distinction that
// makes this count cells rather than coordinates.
var paneCellPattern = regexp.MustCompile(`\d+x\d+,\d+,\d+,(\d+)`)

// paneNumberCounts counts how often each bare pane number appears as a layout cell in layout,
// keyed by the number tmux reads (the pane id minus its leading '%').
func paneNumberCounts(layout string) map[string]int {
	counts := make(map[string]int)
	for _, match := range paneCellPattern.FindAllStringSubmatch(layout, -1) {
		counts[match[1]]++
	}
	return counts
}

// TestRules_NeverEmitsOnePaneNumberTwice is the regression guard for the R5 review's R5-F3.
// tmux does not REJECT a window_layout string naming one pane twice: it accepts it with exit 0,
// assigns cells positionally, and destroys every pane the short cell list no longer covers
// (reproduced live, tmux 3.6 — one `lyx reed up` reduced a two-pane session to one, reported
// ok:true, and then reported the strand live against the Selvage pane).
// Rules is documented as pure and TOTAL, so it must be structurally incapable of producing that
// string no matter how corrupt the strand table it is handed.
func TestRules_NeverEmitsOnePaneNumberTwice(t *testing.T) {
	box := Box{X: 0, Y: 0, W: 100, H: 40}

	tests := []struct {
		name          string
		strands       []Strand
		selvagePaneID string
	}{
		{
			name:          "a strand bound to the Selvage band's own pane",
			strands:       []Strand{{GUID: "a", PaneID: "%1", Live: true, Display: Display{Anchor: AnchorBelowParent}}},
			selvagePaneID: "%1",
		},
		{
			name: "a strand bound to the Selvage band's pane beside a healthy strand",
			strands: []Strand{
				{GUID: "a", PaneID: "%1", Live: true, Display: Display{Anchor: AnchorBelowParent}},
				{GUID: "b", PaneID: "%2", Live: true, Display: Display{Anchor: AnchorBelowParent}},
			},
			selvagePaneID: "%1",
		},
		{
			name: "two strands bound to one pane",
			strands: []Strand{
				{GUID: "a", PaneID: "%2", Live: true, Display: Display{Anchor: AnchorBelowParent}},
				{GUID: "b", PaneID: "%2", Live: true, Display: Display{Anchor: AnchorBelowParent}},
			},
			selvagePaneID: "%1",
		},
		{
			name: "two strands bound to one pane with no Selvage band at all",
			strands: []Strand{
				{GUID: "a", PaneID: "%2", Live: true, Display: Display{Anchor: AnchorBelowParent}},
				{GUID: "b", PaneID: "%2", Live: true, Display: Display{Anchor: AnchorBelowParent}},
			},
			selvagePaneID: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params := Params{CollapsedStripRows: 2, MinFullRows: 3, Selvage: Selvage{PaneID: tt.selvagePaneID, HeightRows: 1}}
			layout, _, err := Rules(tt.strands, box, params, nil)
			if err != nil {
				t.Fatalf("Rules() error = %v; want nil", err)
			}
			for paneNumber, count := range paneNumberCounts(layout) {
				if count > 1 {
					t.Errorf("Rules() = %q; pane number %s appears %d times, want at most 1", layout, paneNumber, count)
				}
			}
		})
	}
}

// TestRules_KeepsTheFirstOwnerWhenPaneCellsCollide pins WHICH strand survives a collision, so the
// repair stays deterministic rather than merely non-destructive: the Selvage band always keeps its
// own pane, and among strands the earlier table entry wins.
func TestRules_KeepsTheFirstOwnerWhenPaneCellsCollide(t *testing.T) {
	box := Box{X: 0, Y: 0, W: 100, H: 40}
	params := Params{CollapsedStripRows: 2, MinFullRows: 3, Selvage: Selvage{PaneID: "%1", HeightRows: 1}}

	strands := []Strand{
		{GUID: "first", PaneID: "%2", Live: true, Display: Display{Anchor: AnchorBelowParent}},
		{GUID: "second", PaneID: "%2", Live: true, Display: Display{Anchor: AnchorBelowParent}},
	}
	_, focus, err := Rules(strands, box, params, nil)
	if err != nil {
		t.Fatalf("Rules() error = %v; want nil", err)
	}
	// Both duplicates name %2, so focus proves the count rather than the identity: exactly one
	// entry survived, and focusTarget resolved against a single-entry stack.
	if focus != "%2" {
		t.Errorf("Rules() focus = %q; want %q", focus, "%2")
	}

	kept := removeDuplicatePaneCells(partitionByAnchor(strands), "%1")
	if len(kept) != 1 || kept[0].GUID != "first" {
		t.Errorf("removeDuplicatePaneCells kept %+v; want exactly the first owner (GUID %q)", kept, "first")
	}
}
