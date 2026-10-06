// rules_test.go golden-tests the composed Rules entry point: the below-parent stack ordered by insertion, hidden-strand exclusion, empty/single-strand/parent-child edges, the checksum-prefix invariant, the declared-focus override, purity across repeated calls, pane-order resequencing to physical pane position, the Selvage bottom-band enumeration (Params.Selvage) and the empty-stack Selvage sole cell.
// It also pins the two layout regimes a real (as opposed to config-pinned) terminal box makes reachable: a budget-satisfying box where height.go's clamps never fire, and a too-short box where they must — the latter with exact cell heights, so no clamped cell is ever non-positive.

package render

import (
	"regexp"
	"strings"
	"testing"
)

// belowParentChain returns a root->mid->active three-level parent chain,
// all AnchorBelowParent and live, with distinct pane ids, in insertion order.
// The two above the bottom-most collapse to collapsed_rows and active takes
// the rest. This is the fixture the golden below-parent and mixed-set cases
// build on.
func belowParentChain() []Strand {
	return []Strand{
		{GUID: "root", Parent: "", PaneID: "%1", Live: true, Display: Display{Anchor: AnchorBelowParent}},
		{GUID: "mid", Parent: "root", PaneID: "%2", Live: true, Display: Display{Anchor: AnchorBelowParent}},
		{GUID: "active", Parent: "mid", PaneID: "%3", Live: true, Display: Display{Anchor: AnchorBelowParent}},
	}
}

// belowParentChainWithRootFocus is belowParentChain with the root declaring Focus explicitly;
// without that flag the default would pick the bottom-most/active strand instead.
func belowParentChainWithRootFocus() []Strand {
	strands := belowParentChain()
	strands[0].Display.Focus = true
	return strands
}

func TestRulesGolden(t *testing.T) {
	t.Parallel()
	params := Params{CollapsedRows: 2, MinFullRows: 3}

	tests := []struct {
		name      string
		strands   []Strand
		box       Box
		selvage   Selvage
		wantBody  string
		wantFocus string
	}{
		{
			name:      "BelowParentFormsBottomDominantStackOrderedByInsertion",
			strands:   belowParentChain(),
			box:       Box{X: 0, Y: 0, W: 100, H: 21}, // usable = 21 - 2 dividers = 19
			wantBody:  "100x21,0,0[100x2,0,0,1,100x2,0,3,2,100x15,0,6,3]",
			wantFocus: "%3", // bottom-most/active default
		},
		{
			// A persisted Display.Focus flag wins regardless of depth: focusTarget scans for the bottom-most Display.Focus strand before falling back to bottom-most-overall.
			// This is the outcome an implementer is most likely to read as a bug and "fix", and it is not: in a worktree opened through the VS Code chain the operator's own working pane already exists and is the agent session the chain focused.
			// This pins an assumption internal/loomcli relies on rather than declares, which is why the test lives here beside the code that owns it.
			name:      "DeclaredFocusStrandWinsOverBottomMost",
			strands:   belowParentChainWithRootFocus(),
			box:       Box{X: 0, Y: 0, W: 100, H: 21},
			wantBody:  "100x21,0,0[100x2,0,0,1,100x2,0,3,2,100x15,0,6,3]",
			wantFocus: "%1",
		},
		{
			name: "HiddenStrandsExcludedFromString",
			strands: append(belowParentChain(),
				Strand{GUID: "h", PaneID: "%99", Live: true, Display: Display{Anchor: AnchorHidden}},
			),
			box:       Box{X: 0, Y: 0, W: 100, H: 21},
			wantBody:  "100x21,0,0[100x2,0,0,1,100x2,0,3,2,100x15,0,6,3]", // identical to the no-hidden case
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
			// The loom shape endorsed by discussion decision childless-full-height-is-acceptable's counterpart: a below-parent root parent with a single below-parent child collapses the parent to CollapsedRows once the child is present, and the child takes the remainder.
			// The height-layer form of this is height_test.go's TestStackHeights; this case only proves the same shape survives through Rules.
			name: "BelowParentRootChildCollapsesRootToStripChildTakesRemainder",
			strands: []Strand{
				{GUID: "parent", PaneID: "%1", Live: true, Display: Display{Anchor: AnchorBelowParent}},
				{GUID: "child", Parent: "parent", PaneID: "%2", Live: true, Display: Display{Anchor: AnchorBelowParent}},
			},
			box:       Box{X: 0, Y: 0, W: 100, H: 15},
			wantBody:  "100x15,0,0[100x2,0,0,1,100x12,0,3,2]",
			wantFocus: "%2",
		},
		{
			// The strand-table order alone decides which strand is bottom-most: a child listed before its parent collapses, and the parent takes the remainder and the focus.
			name: "OrdersTheStackByInsertionNotParentChain",
			strands: []Strand{
				{GUID: "child", Parent: "parent", PaneID: "%2", Live: true, Display: Display{Anchor: AnchorBelowParent}},
				{GUID: "parent", PaneID: "%1", Live: true, Display: Display{Anchor: AnchorBelowParent}},
			},
			box:       Box{X: 0, Y: 0, W: 100, H: 15},
			wantBody:  "100x15,0,0[100x2,0,0,2,100x12,0,3,1]",
			wantFocus: "%1",
		},
		{
			// A live terminal is routinely 24 or 30 rows, unlike the 220x50
			// box a pinned config used to always hand Rules — a box this
			// short means clampBandHeight/clampToFit now govern the common
			// case rather than almost never firing. This row's box has room
			// for the Selvage band, its one-row divider, and the two collapsed
			// placements at CollapsedRows, so no clamp fires: band=2
			// (unclamped: floor=3, maxBand=box.H-1-3=20, 2<=20), stack region
			// {Y:0,H:21}, usable=21-2 dividers=19, root and mid collapse to 2
			// and active takes the remaining 15; the band cell lands last, at
			// Y=box.H-2=22.
			name:      "SelvagePresentBudgetSatisfyingPreservesConfiguredSelvageAndStripHeights",
			strands:   belowParentChain(),
			box:       Box{X: 0, Y: 0, W: 100, H: 24},
			selvage:   Selvage{PaneID: "%h", HeightRows: 2},
			wantBody:  "100x24,0,0[100x2,0,0,1,100x2,0,3,2,100x15,0,6,3,100x2,0,22,h]",
			wantFocus: "%3",
		},
		{
			// The below-parent stack laid out in the shrunk region at the top, followed by a fixed-height Selvage cell at the bottom — the emitted window_layout must enumerate every strand cell plus the Selvage cell so the live-pane count the caller's select-layout applies against matches tmux's actual pane set. bandHeight=3 (unclamped: with the Selvage band's own one-row divider budget subtracted first (box.H-1=20), MinFullRows=3 leaves 17 rows for the stack).
			// The stack region is {X:0,Y:0,W:100,H:17}: usable=17-2 dividers=15, root and mid collapse to CollapsedRows (2 each) and active takes the remaining 11.
			// The strand cells end on row 16 (box.H-B-2 with a one-row divider before the band), the Selvage cell occupies rows 18..20 (box.H-B..box.H-1), and the band never affects focus.
			name:      "SelvageBandEnumeratesEveryStrandCellPlusSelvage",
			strands:   belowParentChain(),
			box:       Box{X: 0, Y: 0, W: 100, H: 21},
			selvage:   Selvage{PaneID: "%h", HeightRows: 3},
			wantBody:  "100x21,0,0[100x2,0,0,1,100x2,0,3,2,100x11,0,6,3,100x3,0,18,h]",
			wantFocus: "%3",
		},
		{
			// The same strand fixture against a box too short for those
			// budgets: band=2 stays unclamped (floor=3, maxBand=
			// box.H-1-3=4, 2<=4), stack region {Y:0,H:5}, usable=5-2
			// dividers=3, root and mid demand 2 each so active's natural
			// -1 borrows 2 rows via clampToFit's priority-1 reclaim, which
			// the collapsed placements repay by shrinking from 2 down to 1,
			// leaving every stack cell at exactly 1 row; the band cell lands
			// last, at Y=box.H-2=6.
			name:      "SelvagePresentClampedRowNoCellEverNonPositive",
			strands:   belowParentChain(),
			box:       Box{X: 0, Y: 0, W: 100, H: 8},
			selvage:   Selvage{PaneID: "%h", HeightRows: 2},
			wantBody:  "100x8,0,0[100x1,0,0,1,100x1,0,2,2,100x1,0,4,3,100x2,0,6,h]",
			wantFocus: "%3",
		},
		{
			// With a Selvage pane and ZERO placed strands, Rules emits the Selvage band as a bracket-less single-cell body claiming the whole box — the same shape tmux reports for a one-pane window — never a zero-height Selvage cell inside a group.
			// Unreachable through applyLayoutLocked today (anyPlacedStrand gates the apply), but Rules is a pure function whose contract must hold for any caller.
			name:      "SelvageWithNoStrandsClaimsWholeBoxAsSoleCell",
			strands:   nil,
			box:       Box{X: 0, Y: 0, W: 100, H: 21},
			selvage:   Selvage{PaneID: "%h", HeightRows: 3},
			wantBody:  "100x21,0,0,h",
			wantFocus: "",
		},
		{
			// An all-filtered stack (only a hidden strand) takes the same sole-band shape.
			name:      "SelvageWithOnlyAHiddenStrandClaimsWholeBoxAsSoleCell",
			strands:   []Strand{{GUID: "hid", PaneID: "%9", Live: true, Display: Display{Anchor: AnchorHidden}}},
			box:       Box{X: 0, Y: 0, W: 100, H: 21},
			selvage:   Selvage{PaneID: "%h", HeightRows: 3},
			wantBody:  "100x21,0,0,h",
			wantFocus: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
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

			// Rules is pure: a repeated call on the same input matches.
			layoutAgain, focusAgain, err := Rules(tt.strands, tt.box, p, nil)
			if err != nil {
				t.Fatalf("Rules() unexpected error on the repeated call: %v", err)
			}
			if layoutAgain != layout || focusAgain != focus {
				t.Errorf("Rules() is not pure: (%q,%q) != (%q,%q)", layout, focus, layoutAgain, focusAgain)
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

func TestRulesPaneOrder(t *testing.T) {
	t.Parallel()
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
	p := Params{CollapsedRows: 2, MinFullRows: 3}

	tests := []struct {
		name      string
		paneOrder []string
		wantBody  string
		wantFocus string
	}{
		{
			// Physical order inverted vs table order: %20 (child) sits on top.
			// Child keeps its intended bottom-most height (17) but is emitted first (at y=0); root keeps its collapsed height (2) but lands at the bottom.
			// Focus stays id-based: the active/bottom strand, regardless of where it physically sits.
			name:      "InvertedPhysicalOrderEmitsCellsAtPhysicalPositions",
			paneOrder: []string{"%20", "%10"},
			wantBody:  "100x20,0,0[100x17,0,0,20,100x2,0,18,10]",
			wantFocus: "%20",
		},
		{
			// paneOrder naming only a pane render never placed: the intended order survives at the tail, identical to the nil-paneOrder shape.
			name:      "UnknownIDsKeepIntendedTailOrder",
			paneOrder: []string{"%99"},
			wantBody:  "100x20,0,0[100x2,0,0,10,100x17,0,3,20]",
			wantFocus: "%20",
		},
		{
			name:      "NilPaneOrderKeepsIntendedOrder",
			paneOrder: nil,
			wantBody:  "100x20,0,0[100x2,0,0,10,100x17,0,3,20]",
			wantFocus: "%20",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			layout, focus, err := Rules(strands, box, p, tt.paneOrder)
			if err != nil {
				t.Fatalf("Rules() unexpected error: %v", err)
			}
			if want := layoutChecksum(tt.wantBody) + "," + tt.wantBody; layout != want {
				t.Errorf("Rules() layout = %q, want %q", layout, want)
			}
			if focus != tt.wantFocus {
				t.Errorf("Rules() focus = %q, want %q", focus, tt.wantFocus)
			}
		})
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
// tmux does not REJECT a window_layout string naming one pane twice: it accepts it with exit 0, assigns cells positionally, and destroys every pane the short cell list no longer covers (reproduced live, tmux 3.6 — one `lyx reed up` reduced a two-pane session to one, reported ok:true, and then reported the strand live against the Selvage pane).
// Rules is documented as pure and TOTAL, so it must be structurally incapable of producing that string no matter how corrupt the strand table it is handed.
//
//testtiming:keep pins that no pane number appears twice in the emitted layout, which its covering tests never count
func TestRules_NeverEmitsOnePaneNumberTwice(t *testing.T) {
	t.Parallel()
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
			t.Parallel()
			params := Params{CollapsedRows: 2, MinFullRows: 3, Selvage: Selvage{PaneID: tt.selvagePaneID, HeightRows: 1}}
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

// TestRules_KeepsTheFirstOwnerWhenPaneCellsCollide pins WHICH strand survives a collision, so the repair stays deterministic rather than merely non-destructive: the Selvage band always keeps its own pane, and among strands the earlier table entry wins.
//
//testtiming:keep pins that the earlier table entry survives a pane-cell collision, which the layout-only assertions of its covering tests cannot see
func TestRules_KeepsTheFirstOwnerWhenPaneCellsCollide(t *testing.T) {
	t.Parallel()
	box := Box{X: 0, Y: 0, W: 100, H: 40}
	params := Params{CollapsedRows: 2, MinFullRows: 3, Selvage: Selvage{PaneID: "%1", HeightRows: 1}}

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
