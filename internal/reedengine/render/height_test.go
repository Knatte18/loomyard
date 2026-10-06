// height_test.go exercises the derived height policy in height.go: the heights-fill-the-box
// invariant, the uniform collapsed_rows rule with the remainder going to the bottom-most pane, the
// too-short-window clamp order, and the band-vs-window height clamp (clampBandHeight).

package render

import "testing"

// stackOf builds n live below-parent strands with pane ids %1..%n, in insertion order.
func stackOf(n int) []Strand {
	names := []string{"a", "b", "c", "d", "e", "f"}
	stack := make([]Strand, n)
	for i := range stack {
		stack[i] = Strand{GUID: names[i], PaneID: "%" + string(rune('1'+i)), Live: true, Display: Display{Anchor: AnchorBelowParent}}
	}
	return stack
}

func TestStackHeights(t *testing.T) {
	tests := []struct {
		name          string
		strands       int
		boxH          int
		collapsedRows int
		wantHeights   []int
		// unfillable marks a window shorter than the pane count, where the heights cannot sum to the box.
		unfillable bool
	}{
		{name: "OneStrandGetsTheWholeBox", strands: 1, boxH: 20, collapsedRows: 3, wantHeights: []int{20}},
		// usable 20 - 1 divider = 19; 3 collapsed, 16 rest.
		{name: "TwoStrandsGetCollapsedPlusRest", strands: 2, boxH: 20, collapsedRows: 3, wantHeights: []int{3, 16}},
		// Adding a third keeps the first pane's height and collapses the previous bottom-most.
		{name: "ThreeStrandsKeepTheFirstPaneHeightOfTwo", strands: 3, boxH: 30, collapsedRows: 3, wantHeights: []int{3, 3, 22}},
		// Removing the bottom-most expands the one above: usable 30 - 1 divider = 29, minus the collapsed 3.
		{name: "TwoStrandsInTheSameBoxExpandTheOneAbove", strands: 2, boxH: 30, collapsedRows: 3, wantHeights: []int{3, 26}},
		{name: "CollapsedRowsOneFillsTheBox", strands: 3, boxH: 15, collapsedRows: 1, wantHeights: []int{1, 1, 11}},
		{name: "CollapsedRowsTwoFillsTheBox", strands: 3, boxH: 15, collapsedRows: 2, wantHeights: []int{2, 2, 9}},
		{name: "CollapsedRowsFourFillsTheBox", strands: 3, boxH: 20, collapsedRows: 4, wantHeights: []int{4, 4, 10}},
		{name: "CollapsedRowsSixFillsTheBox", strands: 3, boxH: 30, collapsedRows: 6, wantHeights: []int{6, 6, 16}},
		// Three collapsed placements each demanding 3 rows plus a bottom-most pane in 5 usable rows:
		// the natural split would drive the bottom-most pane negative, so the collapsed ones yield.
		{name: "TooShortWindowReclaimsFromTheCollapsedFirst", strands: 4, boxH: 8, collapsedRows: 3, wantHeights: []int{1, 1, 2, 1}},
		// usable 6 - 3 dividers = 3, less than the 4 panes: not exactly fillable, still never non-positive.
		{name: "ImpossibleWindowNeverYieldsANonPositiveHeight", strands: 4, boxH: 6, collapsedRows: 3, wantHeights: []int{1, 1, 1, 1}, unfillable: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			stack := stackOf(tt.strands)
			box := Box{X: 0, Y: 0, W: 100, H: tt.boxH}

			placements := stackHeights(stack, box, Params{CollapsedRows: tt.collapsedRows, MinFullRows: 3})
			if len(placements) != len(stack) {
				t.Fatalf("stackHeights returned %d placements, want %d", len(placements), len(stack))
			}

			sum := 0
			for i, pl := range placements {
				if pl.height != tt.wantHeights[i] {
					t.Errorf("placement[%d].height = %d, want %d (all heights %v)", i, pl.height, tt.wantHeights[i], tt.wantHeights)
				}
				if pl.height <= 0 {
					t.Errorf("placement %+v has non-positive height", pl)
				}
				sum += pl.height
			}
			if dividers := len(stack) - 1; !tt.unfillable && sum+dividers != box.H {
				t.Errorf("heights sum + dividers = %d, want box.H %d", sum+dividers, box.H)
			}
			if last := placements[len(placements)-1]; last.strip {
				t.Errorf("bottom-most placement %+v must not be collapsed", last)
			}
		})
	}
}

// TestClampBandHeight covers the window-split clamp: the band yields rows first so the
// strand-stack region never shrinks below MinFullRows (floored at 1) total rows, distinct from
// clampToFit's job of distributing rows AMONG strands inside an already-shrunk box.
func TestClampBandHeight(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		bandRows     int
		windowRows   int
		minStackRows int
		want         int
	}{
		{"WellWithinFloor_Unclamped", 3, 21, 3, 3},
		{"ExactlyAtFloor_Unclamped", 18, 21, 3, 18},
		{
			// An oversized configured height_rows must yield rows so the
			// stack region keeps its MinFullRows floor, even though that
			// means the band itself ends up shorter than configured.
			name: "Oversized_ClampedToPreserveFloor", bandRows: 25, windowRows: 21, minStackRows: 3, want: 18,
		},
		{
			// The window cannot fit both a band and the floor at all: the
			// band still keeps its 1-row minimum (real tmux/psmux does not
			// cleanly support a zero-height select-layout cell — see
			// height.go's doc comment) rather than going to zero, even
			// though that means the stack floor itself is violated instead.
			name: "WindowTooShortForBoth_BandFlooredAtOne", bandRows: 5, windowRows: 2, minStackRows: 3, want: 1,
		},
		{
			// bandRows <= 0 (including the negative-treated-as-zero case)
			// still floors to 1 once the window has any rows to give — the
			// Selvage band exists whenever this function is called, so it
			// can never legitimately request/receive a zero-height cell.
			name: "NegativeBandRows_FlooredAtOne", bandRows: -4, windowRows: 21, minStackRows: 3, want: 1,
		},
		{"NonPositiveMinStackRowsFlooredAtOne", 25, 21, 0, 20},
		{
			// windowRows itself has nothing to give: the result is 0, not a
			// floored 1, since there is no row available at all.
			name: "ZeroWindowRows_NothingToGive", bandRows: 5, windowRows: 0, minStackRows: 3, want: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := clampBandHeight(tt.bandRows, tt.windowRows, tt.minStackRows)
			if got != tt.want {
				t.Errorf("clampBandHeight(%d, %d, %d) = %d, want %d", tt.bandRows, tt.windowRows, tt.minStackRows, got, tt.want)
			}
			// Invariant every case must hold: the stack region resulting
			// from this clamp never shrinks below the floored MinFullRows.
			floor := tt.minStackRows
			if floor < 1 {
				floor = 1
			}
			if stackRows := tt.windowRows - got; stackRows < floor && tt.windowRows >= floor {
				t.Errorf("clampBandHeight(%d, %d, %d) left only %d stack rows, want >= floor %d", tt.bandRows, tt.windowRows, tt.minStackRows, stackRows, floor)
			}
		})
	}
}
