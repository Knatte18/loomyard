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

// twoFullSiblings returns two co-equal, non-shrinking below-parent strands with no strip anywhere in
// the stack — the fixture the no-strip cases build on.
func twoFullSiblings() []Strand {
	return []Strand{
		{GUID: "a", PaneID: "%1", Live: true, Display: Display{Anchor: AnchorBelowParent}},
		{GUID: "b", PaneID: "%2", Live: true, Display: Display{Anchor: AnchorBelowParent}},
	}
}

// twoDistinctStrips returns a root->mid->active chain where BOTH root and mid collapse to a strip —
// each is an ancestor of the present active descendant — leaving active as the sole full pane. This
// is the fixture the ordering test uses to prove two strip pins both follow the header pin.
func twoDistinctStrips() []Strand {
	return []Strand{
		{GUID: "root", Parent: "", PaneID: "%1", Live: true, Display: Display{Anchor: AnchorBelowParent, ShrinkWhenWaitingOnChild: true}},
		{GUID: "mid", Parent: "root", PaneID: "%2", Live: true, Display: Display{Anchor: AnchorBelowParent, ShrinkWhenWaitingOnChild: true}},
		{GUID: "active", Parent: "mid", PaneID: "%3", Live: true, Display: Display{Anchor: AnchorBelowParent}},
	}
}

func TestFixedHeightPinsMatchesRulesPlacedHeights(t *testing.T) {
	tests := []struct {
		name     string
		strands  []Strand
		box      Box
		params   Params
		wantPins []Pin
		wantErr  bool
	}{
		{
			name:     "SelvagePlusTwoFullStrandsOnlyTheSelvageIsPinned",
			strands:  twoFullSiblings(),
			box:      Box{X: 0, Y: 0, W: 100, H: 21},
			params:   Params{CollapsedStripRows: 2, MinFullRows: 3, Selvage: Selvage{PaneID: "%h", HeightRows: 2}},
			wantPins: []Pin{{PaneID: "%h", Height: 2}},
		},
		{
			// Mirrors rules_test.go's TestRulesSelvageBandEnumeratesEveryStrandCellPlusSelvage fixture:
			// band unclamped at 3, mid collapses to CollapsedStripRows (2).
			name:     "SelvagePlusShrinkAncestorWithPresentDescendantSelvageThenStrip",
			strands:  belowParentChain(),
			box:      Box{X: 0, Y: 0, W: 100, H: 21},
			params:   Params{CollapsedStripRows: 2, MinFullRows: 3, Selvage: Selvage{PaneID: "%h", HeightRows: 3}},
			wantPins: []Pin{{PaneID: "%h", Height: 3}, {PaneID: "%2", Height: 2}},
		},
		{
			// Mirrors TestRulesGolden's BelowParentFormsBottomDominantStackOrderedByParentChain
			// fixture with no Selvage band configured: only the strip (mid) is pinned.
			name:     "NoSelvageConfiguredWithStripPresentOnlyTheStripIsPinned",
			strands:  belowParentChain(),
			box:      Box{X: 0, Y: 0, W: 100, H: 21},
			params:   Params{CollapsedStripRows: 2, MinFullRows: 3},
			wantPins: []Pin{{PaneID: "%2", Height: 2}},
		},
		{
			name:     "NoSelvageAndNoStripYieldsNoPins",
			strands:  twoFullSiblings(),
			box:      Box{X: 0, Y: 0, W: 100, H: 21},
			params:   Params{CollapsedStripRows: 2, MinFullRows: 3},
			wantPins: nil,
		},
		{
			// heightRows=25 requested; clampBandHeight(25, box.H-1=20, MinFullRows=3) clamps to
			// windowRows-floor=17 to preserve the stack's floor — the pin must carry 17, never the
			// configured 25.
			name:     "OversizedSelvageHeightRowsPinCarriesTheClampedValueNotConfigured",
			strands:  twoFullSiblings(),
			box:      Box{X: 0, Y: 0, W: 100, H: 21},
			params:   Params{CollapsedStripRows: 2, MinFullRows: 3, Selvage: Selvage{PaneID: "%h", HeightRows: 25}},
			wantPins: []Pin{{PaneID: "%h", Height: 17}},
		},
		{
			// Mirrors rules_test.go's SelvagePresentClampedRowNoCellEverNonPositive golden row: the
			// window is too short for the strip's natural CollapsedStripRows (2), and clampToFit's
			// priority-1 pass reclaims it down to 1 — the pin must carry 1, never CollapsedStripRows.
			name:     "TooShortWindowStripPinCarriesTheReclaimedValueNotCollapsedStripRows",
			strands:  belowParentChain(),
			box:      Box{X: 0, Y: 0, W: 100, H: 8},
			params:   Params{CollapsedStripRows: 2, MinFullRows: 3, Selvage: Selvage{PaneID: "%h", HeightRows: 2}},
			wantPins: []Pin{{PaneID: "%h", Height: 2}, {PaneID: "%2", Height: 1}},
		},
		{
			// The sole-band branch: a Selvage band configured with no strand placed claims the whole
			// box and has no absolute budget of its own — a stale one-row pin must never be emitted.
			name:     "SelvageConfiguredWithNoStrandPlacedYieldsNoPin",
			strands:  nil,
			box:      Box{X: 0, Y: 0, W: 100, H: 21},
			params:   Params{CollapsedStripRows: 2, MinFullRows: 3, Selvage: Selvage{PaneID: "%h", HeightRows: 3}},
			wantPins: nil,
		},
		{
			name: "AnchorOwnWindowYieldsNilNotAPanicMatchingRulesError",
			strands: []Strand{
				{GUID: "a", PaneID: "%1", Live: true, Display: Display{Anchor: AnchorOwnWindow}},
			},
			box:      Box{X: 0, Y: 0, W: 100, H: 21},
			params:   Params{CollapsedStripRows: 2, MinFullRows: 3},
			wantPins: nil,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotPins := FixedHeightPins(tt.strands, tt.box, tt.params)
			if diff := cmp.Diff(tt.wantPins, gotPins); diff != "" {
				t.Errorf("FixedHeightPins() mismatch (-want +got):\n%s", diff)
			}

			layout, _, err := Rules(tt.strands, tt.box, tt.params, nil)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("Rules() with the same input: expected error, got nil")
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

// TestFixedHeightPinsOrdersTheSelvagePinFirstThenEveryStripPin asserts pin ordering directly: with
// the Selvage band and two distinct strips present, the Selvage pin is index 0 and both strip pins
// follow — hook-array fire order, not screen position, since the band itself renders at the bottom.
func TestFixedHeightPinsOrdersTheSelvagePinFirstThenEveryStripPin(t *testing.T) {
	params := Params{CollapsedStripRows: 2, MinFullRows: 3, Selvage: Selvage{PaneID: "%h", HeightRows: 2}}
	box := Box{X: 0, Y: 0, W: 100, H: 21}

	pins := FixedHeightPins(twoDistinctStrips(), box, params)
	if len(pins) != 3 {
		t.Fatalf("FixedHeightPins() returned %d pins, want 3 (Selvage + two strips): %+v", len(pins), pins)
	}
	if pins[0].PaneID != "%h" {
		t.Errorf("pins[0].PaneID = %q, want the Selvage pane %q first", pins[0].PaneID, "%h")
	}
	gotStrips := map[string]bool{pins[1].PaneID: true, pins[2].PaneID: true}
	wantStrips := map[string]bool{"%1": true, "%2": true}
	if diff := cmp.Diff(wantStrips, gotStrips); diff != "" {
		t.Errorf("strip pins after the Selvage pin (-want +got):\n%s", diff)
	}
}

// TestSelvageBandCellOffsetsSumToTheBoxWithDivider asserts the bottom-band geometry directly: for a
// box of height H with a Selvage band of height B and n placed strands, the strand cells occupy rows
// box.Y .. box.Y+H-B-2 and the Selvage cell occupies rows box.Y+H-B .. box.Y+H-1 — the single row at
// box.Y+H-B-1 is the divider between the stack and the band, never claimed by either.
func TestSelvageBandCellOffsetsSumToTheBoxWithDivider(t *testing.T) {
	params := Params{CollapsedStripRows: 2, MinFullRows: 3, Selvage: Selvage{PaneID: "%h", HeightRows: 3}}
	box := Box{X: 0, Y: 0, W: 100, H: 21}

	layout, _, err := Rules(belowParentChain(), box, params, nil)
	if err != nil {
		t.Fatalf("Rules() unexpected error: %v", err)
	}

	plan, err := planCells(belowParentChain(), box, params)
	if err != nil {
		t.Fatalf("planCells() unexpected error: %v", err)
	}
	bandHeight := plan.bandHeight

	wantStackLastRow := box.Y + box.H - bandHeight - 2
	wantBandFirstRow := box.Y + box.H - bandHeight
	wantBandLastRow := box.Y + box.H - 1

	stackLastRow := paneHeightFromLayoutY(t, layout, "%3")
	if stackLastRow > wantStackLastRow {
		t.Errorf("strand cell %q occupies rows up to %d, want at most %d (box.Y+H-B-2)", "%3", stackLastRow, wantStackLastRow)
	}
	bandFirstRow := bandFirstRowFromLayout(t, layout, "h")
	if bandFirstRow != wantBandFirstRow {
		t.Errorf("Selvage cell first row = %d, want %d (box.Y+H-B)", bandFirstRow, wantBandFirstRow)
	}
	bandLastRow := bandFirstRow + bandHeight - 1
	if bandLastRow != wantBandLastRow {
		t.Errorf("Selvage cell last row = %d, want %d (box.Y+H-1)", bandLastRow, wantBandLastRow)
	}
}

// bandFirstRowFromLayout returns the y offset of paneID's cell within layout.
func bandFirstRowFromLayout(t *testing.T, layout, paneID string) int {
	t.Helper()
	pattern := regexp.MustCompile(`\d+x\d+,\d+,(\d+),` + regexp.QuoteMeta(paneID))
	m := pattern.FindStringSubmatch(layout)
	if m == nil {
		t.Fatalf("pane %q not found as a cell in layout %q", paneID, layout)
	}
	y, err := strconv.Atoi(m[1])
	if err != nil {
		t.Fatalf("pane %q y offset %q did not parse as an integer: %v", paneID, m[1], err)
	}
	return y
}

// paneHeightFromLayoutY returns the last row paneID's cell occupies within layout (its y offset plus
// its height minus one).
func paneHeightFromLayoutY(t *testing.T, layout, paneID string) int {
	t.Helper()
	want := strings.TrimPrefix(paneID, "%")
	pattern := regexp.MustCompile(`\d+x(\d+),\d+,(\d+),` + regexp.QuoteMeta(want))
	m := pattern.FindStringSubmatch(layout)
	if m == nil {
		t.Fatalf("pane %q not found as a cell in layout %q", paneID, layout)
	}
	height, err := strconv.Atoi(m[1])
	if err != nil {
		t.Fatalf("pane %q height %q did not parse as an integer: %v", paneID, m[1], err)
	}
	y, err := strconv.Atoi(m[2])
	if err != nil {
		t.Fatalf("pane %q y offset %q did not parse as an integer: %v", paneID, m[2], err)
	}
	return y + height - 1
}
