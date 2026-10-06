// pins_test.go pins FixedHeightPins against the heights Rules places for the same inputs, so the two
// can never drift apart. Every case calls both entry points on the identical (strands, box, params)
// triple: the expected pin list is asserted directly, and each returned pin's height is additionally
// re-parsed out of Rules' own layout string rather than restated from the expectation — the second
// check is what makes this a drift guard rather than a second copy of the policy.

package render

import (
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// pinCellPattern matches one cell of a tmux window_layout body — "<w>x<h>,<x>,<y>,<id>" — capturing
// the height and the bare pane id (a GROUP header has no trailing id field and so never matches).
var pinCellPattern = regexp.MustCompile(`\d+x(\d+),\d+,\d+,([^,\]]+)`)

// paneHeightFromLayout returns the height of paneID's cell within layout, parsed directly out of the
// rendered window_layout string rather than recomputed from policy.
func paneHeightFromLayout(t *testing.T, layout, paneID string) int {
	t.Helper()
	want := strings.TrimPrefix(paneID, "%")
	for _, m := range pinCellPattern.FindAllStringSubmatch(layout, -1) {
		if m[2] != want {
			continue
		}
		height, err := strconv.Atoi(m[1])
		if err != nil {
			t.Fatalf("pane %q height %q did not parse as an integer: %v", paneID, m[1], err)
		}
		return height
	}
	t.Fatalf("pane %q not found as a cell in layout %q", paneID, layout)
	return 0
}

// twoSiblings returns two below-parent strands in insertion order — the first collapses and the
// second (bottom-most) takes the rest.
func twoSiblings() []Strand {
	return []Strand{
		{GUID: "a", PaneID: "%1", Live: true, Display: Display{Anchor: AnchorBelowParent}},
		{GUID: "b", PaneID: "%2", Live: true, Display: Display{Anchor: AnchorBelowParent}},
	}
}

func TestFixedHeightPinsMatchesRulesPlacedHeights(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		strands  []Strand
		box      Box
		params   Params
		wantPins []Pin
		wantErr  bool
	}{
		{
			name:     "SelvagePlusTwoStrandsSelvageThenTheCollapsedOne",
			strands:  twoSiblings(),
			box:      Box{X: 0, Y: 0, W: 100, H: 21},
			params:   Params{CollapsedRows: 2, MinFullRows: 3, Selvage: Selvage{PaneID: "%h", HeightRows: 2}},
			wantPins: []Pin{{PaneID: "%h", Height: 2}, {PaneID: "%1", Height: 2}},
		},
		{
			// Mirrors TestRulesGolden's SelvageBandEnumeratesEveryStrandCellPlusSelvage fixture:
			// band unclamped at 3, root and mid collapse to CollapsedRows (2).
			name:     "SelvagePlusChainSelvageThenEveryCollapsedPlacement",
			strands:  belowParentChain(),
			box:      Box{X: 0, Y: 0, W: 100, H: 21},
			params:   Params{CollapsedRows: 2, MinFullRows: 3, Selvage: Selvage{PaneID: "%h", HeightRows: 3}},
			wantPins: []Pin{{PaneID: "%h", Height: 3}, {PaneID: "%1", Height: 2}, {PaneID: "%2", Height: 2}},
		},
		{
			// Mirrors TestRulesGolden's BelowParentFormsBottomDominantStackOrderedByInsertion
			// fixture with no Selvage band configured: only the collapsed placements are pinned.
			name:     "NoSelvageConfiguredOnlyTheCollapsedPlacementsArePinned",
			strands:  belowParentChain(),
			box:      Box{X: 0, Y: 0, W: 100, H: 21},
			params:   Params{CollapsedRows: 2, MinFullRows: 3},
			wantPins: []Pin{{PaneID: "%1", Height: 2}, {PaneID: "%2", Height: 2}},
		},
		{
			name:     "NoSelvageAndOneStrandYieldsNoPins",
			strands:  []Strand{{GUID: "a", PaneID: "%1", Live: true, Display: Display{Anchor: AnchorBelowParent}}},
			box:      Box{X: 0, Y: 0, W: 100, H: 21},
			params:   Params{CollapsedRows: 2, MinFullRows: 3},
			wantPins: nil,
		},
		{
			name:     "OneStrandWithSelvageOnlyTheSelvageIsPinned",
			strands:  []Strand{{GUID: "a", PaneID: "%1", Live: true, Display: Display{Anchor: AnchorBelowParent}}},
			box:      Box{X: 0, Y: 0, W: 100, H: 21},
			params:   Params{CollapsedRows: 2, MinFullRows: 3, Selvage: Selvage{PaneID: "%h", HeightRows: 2}},
			wantPins: []Pin{{PaneID: "%h", Height: 2}},
		},
		{
			// heightRows=25 requested; clampBandHeight(25, box.H-1=20, MinFullRows=3) clamps to
			// windowRows-floor=17 to preserve the stack's floor — the pin must carry 17, never the
			// configured 25. The 3-row stack region then leaves 2 usable rows, so the collapsed
			// placement is itself reclaimed from 2 down to 1.
			name:     "OversizedSelvageHeightRowsPinCarriesTheClampedValueNotConfigured",
			strands:  twoSiblings(),
			box:      Box{X: 0, Y: 0, W: 100, H: 21},
			params:   Params{CollapsedRows: 2, MinFullRows: 3, Selvage: Selvage{PaneID: "%h", HeightRows: 25}},
			wantPins: []Pin{{PaneID: "%h", Height: 17}, {PaneID: "%1", Height: 1}},
		},
		{
			// Mirrors TestRulesGolden's SelvagePresentClampedRowNoCellEverNonPositive row: the window is too short for the collapsed placements' natural CollapsedRows (2), and clampToFit's priority-1 pass reclaims each down to 1 — the pins must carry 1, never CollapsedRows.
			name:     "TooShortWindowCollapsedPinsCarryTheReclaimedValueNotCollapsedRows",
			strands:  belowParentChain(),
			box:      Box{X: 0, Y: 0, W: 100, H: 8},
			params:   Params{CollapsedRows: 2, MinFullRows: 3, Selvage: Selvage{PaneID: "%h", HeightRows: 2}},
			wantPins: []Pin{{PaneID: "%h", Height: 2}, {PaneID: "%1", Height: 1}, {PaneID: "%2", Height: 1}},
		},
		{
			// The sole-band branch: a Selvage band configured with no strand placed claims the whole
			// box and has no absolute budget of its own — a stale one-row pin must never be emitted.
			name:     "SelvageConfiguredWithNoStrandPlacedYieldsNoPin",
			strands:  nil,
			box:      Box{X: 0, Y: 0, W: 100, H: 21},
			params:   Params{CollapsedRows: 2, MinFullRows: 3, Selvage: Selvage{PaneID: "%h", HeightRows: 3}},
			wantPins: nil,
		},
		{
			name: "AnchorOwnWindowYieldsNilNotAPanicMatchingRulesError",
			strands: []Strand{
				{GUID: "a", PaneID: "%1", Live: true, Display: Display{Anchor: AnchorOwnWindow}},
			},
			box:      Box{X: 0, Y: 0, W: 100, H: 21},
			params:   Params{CollapsedRows: 2, MinFullRows: 3},
			wantPins: nil,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			gotPins := FixedHeightPins(tt.strands, tt.box, tt.params)
			if diff := cmp.Diff(tt.wantPins, gotPins); diff != "" {
				t.Errorf("FixedHeightPins() mismatch (-want +got):\n%s", diff)
			}

			layout, focus, err := Rules(tt.strands, tt.box, tt.params, nil)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("Rules() with the same input: expected error, got nil")
				}
				if layout != "" || focus != "" {
					t.Errorf("Rules() on error = (%q, %q), want both empty", layout, focus)
				}
				return
			}
			if err != nil {
				t.Fatalf("Rules() unexpected error: %v", err)
			}

			// Assertion (b): every pin's height must equal the height that
			// pane's cell actually carries in Rules' own layout string,
			// parsed out of the string rather than restated from the
			// expectation above.
			for _, pin := range gotPins {
				if got := paneHeightFromLayout(t, layout, pin.PaneID); got != pin.Height {
					t.Errorf("pane %q: FixedHeightPins reported height %d, but Rules placed it at %d", pin.PaneID, pin.Height, got)
				}
			}
		})
	}
}
