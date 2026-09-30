// height_test.go exercises the derived height policy in height.go: the heights-fill-the-box
// invariant, the uniform collapsed_rows rule with the remainder going to the bottom-most pane, the
// too-short-window clamp order, and the band-vs-window height clamp (clampBandHeight).
// It also exercises layout.go's buildStackBody/wrapLayout over the resulting placements.

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

func heightsOf(placements []placement) []int {
	out := make([]int, len(placements))
	for i, pl := range placements {
		out[i] = pl.height
	}
	return out
}

func TestStackHeightsFillBoxAndCollapsedEqualsParam(t *testing.T) {
	tests := []struct {
		name          string
		collapsedRows int
		boxH          int
	}{
		{"rows1", 1, 15},
		{"rows2", 2, 15},
		{"rows4", 4, 20},
		{"rows6", 6, 30},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stack := stackOf(3)
			box := Box{X: 0, Y: 0, W: 100, H: tt.boxH}
			p := Params{CollapsedRows: tt.collapsedRows, MinFullRows: 3}

			placements := stackHeights(stack, box, p)
			if len(placements) != len(stack) {
				t.Fatalf("stackHeights returned %d placements, want %d", len(placements), len(stack))
			}

			sum := 0
			for _, pl := range placements {
				if pl.height <= 0 {
					t.Errorf("placement %+v has non-positive height", pl)
				}
				sum += pl.height
			}
			dividers := len(stack) - 1
			if sum+dividers != box.H {
				t.Errorf("heights sum + dividers = %d, want box.H %d", sum+dividers, box.H)
			}

			for i := 0; i < len(stack)-1; i++ {
				if placements[i].height != tt.collapsedRows || !placements[i].strip {
					t.Errorf("placement[%d] = %+v, want collapsed at %d", i, placements[i], tt.collapsedRows)
				}
			}
			if last := placements[len(placements)-1]; last.strip {
				t.Errorf("bottom-most placement %+v must not be collapsed", last)
			}
		})
	}
}

func TestStackHeightsOneStrandGetsTheWholeBox(t *testing.T) {
	got := heightsOf(stackHeights(stackOf(1), Box{W: 100, H: 20}, Params{CollapsedRows: 3, MinFullRows: 3}))
	if len(got) != 1 || got[0] != 20 {
		t.Errorf("heights = %v, want [20]", got)
	}
}

func TestStackHeightsTwoStrandsGetCollapsedPlusRest(t *testing.T) {
	got := heightsOf(stackHeights(stackOf(2), Box{W: 100, H: 20}, Params{CollapsedRows: 3, MinFullRows: 3}))
	want := []int{3, 16} // usable 20 - 1 divider = 19; 3 collapsed, 16 rest
	if got[0] != want[0] || got[1] != want[1] {
		t.Errorf("heights = %v, want %v", got, want)
	}
}

func TestStackHeightsAddingAThirdKeepsTheFirstPaneHeight(t *testing.T) {
	p := Params{CollapsedRows: 3, MinFullRows: 3}
	box := Box{W: 100, H: 30}
	two := heightsOf(stackHeights(stackOf(2), box, p))
	three := heightsOf(stackHeights(stackOf(3), box, p))

	if two[0] != three[0] {
		t.Errorf("first pane height changed from %d to %d when a third strand was added", two[0], three[0])
	}
	if three[1] != 3 {
		t.Errorf("previous bottom-most height = %d, want it collapsed to 3", three[1])
	}
	want := 30 - 2 - 3 - 3 // usable minus the two collapsed
	if three[2] != want {
		t.Errorf("bottom-most height = %d, want %d", three[2], want)
	}
}

func TestStackHeightsRemovingTheBottomMostExpandsTheOneAbove(t *testing.T) {
	p := Params{CollapsedRows: 3, MinFullRows: 3}
	box := Box{W: 100, H: 30}
	three := heightsOf(stackHeights(stackOf(3), box, p))
	two := heightsOf(stackHeights(stackOf(2), box, p))

	if two[1] <= three[1] {
		t.Errorf("strand above the removed bottom-most did not expand: %d -> %d", three[1], two[1])
	}
	if want := 30 - 1 - 3; two[1] != want {
		t.Errorf("new bottom-most height = %d, want %d", two[1], want)
	}
}

func TestStackHeightsClampYieldsOnlyPositiveHeightsInTooShortWindow(t *testing.T) {
	// Three collapsed placements each demanding CollapsedRows=3 plus one bottom-most pane, but the
	// window only has 5 usable rows for 4 panes — the natural split would drive the bottom-most
	// pane negative. clampToFit must reclaim rows from the collapsed placements first and still
	// land on an exact, all-positive split.
	stack := stackOf(4)
	dividers := len(stack) - 1
	usable := 5
	box := Box{X: 0, Y: 0, W: 100, H: usable + dividers}
	p := Params{CollapsedRows: 3, MinFullRows: 3}

	placements := stackHeights(stack, box, p)
	sum := 0
	for _, pl := range placements {
		if pl.height <= 0 {
			t.Errorf("placement %+v has non-positive height under clamp", pl)
		}
		sum += pl.height
	}
	if sum+dividers != box.H {
		t.Errorf("heights sum + dividers = %d, want box.H %d", sum+dividers, box.H)
	}
}

func TestStackHeightsExtremelyShortWindowNeverNonPositive(t *testing.T) {
	// A window shorter than the pane count cannot be filled exactly (each
	// pane needs at least 1 row), but stackHeights must still never return
	// a non-positive height even in that impossible-to-satisfy case.
	box := Box{X: 0, Y: 0, W: 100, H: 6} // usable = 6 - 3 dividers = 3, less than 4 panes
	p := Params{CollapsedRows: 3, MinFullRows: 3}

	for _, pl := range stackHeights(stackOf(4), box, p) {
		if pl.height <= 0 {
			t.Errorf("placement %+v has non-positive height in an impossible-to-fit window", pl)
		}
	}
}

// TestClampBandHeight covers the window-split clamp: the band yields rows first so the
// strand-stack region never shrinks below MinFullRows (floored at 1) total rows, distinct from
// clampToFit's job of distributing rows AMONG strands inside an already-shrunk box.
func TestClampBandHeight(t *testing.T) {
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
			if got := clampBandHeight(tt.bandRows, tt.windowRows, tt.minStackRows); got != tt.want {
				t.Errorf("clampBandHeight(%d, %d, %d) = %d, want %d", tt.bandRows, tt.windowRows, tt.minStackRows, got, tt.want)
			}
			// Invariant every case must hold: the stack region resulting
			// from this clamp never shrinks below the floored MinFullRows.
			floor := tt.minStackRows
			if floor < 1 {
				floor = 1
			}
			got := clampBandHeight(tt.bandRows, tt.windowRows, tt.minStackRows)
			if stackRows := tt.windowRows - got; stackRows < floor && tt.windowRows >= floor {
				t.Errorf("clampBandHeight(%d, %d, %d) left only %d stack rows, want >= floor %d", tt.bandRows, tt.windowRows, tt.minStackRows, stackRows, floor)
			}
		})
	}
}

func TestStackHeightsAndBuildStackBodyIntegration(t *testing.T) {
	// Exercises stackHeights together with buildStackBody/wrapLayout turning
	// the resulting placements into a checksum-prefixed layout string.
	stack := stackOf(3)
	box := Box{X: 0, Y: 0, W: 100, H: 15}
	p := Params{CollapsedRows: 2, MinFullRows: 3}
	placements := stackHeights(stack, box, p)

	body := buildStackBody(box, placements)
	full := wrapLayout(body)
	if got, want := full[:4], layoutChecksum(body); got != want {
		t.Errorf("layout checksum prefix = %q, want %q", got, want)
	}
	if full[4] != ',' {
		t.Errorf("layout string = %q, want checksum then comma then body", full)
	}
}
