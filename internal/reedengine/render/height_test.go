// height_test.go exercises the derived height policy in height.go: the
// heights-fill-the-box invariant, the collapsed-strip height, the active
// pane's remainder rule, the too-short-window clamp order, and the
// header-vs-window height clamp (clampHeaderHeight). It also exercises
// layout.go's buildStackBody/wrapLayout and focus.go's isAncestor, since
// cards 5 and 7 ship no standalone test file.

package render

import "testing"

// chainStack builds a root->mid->active three-level parent chain, all
// AnchorBelowParent, live, with a pane id per strand, so tests can control
// each level's ShrinkWhenWaitingOnChild independently.
func chainStack(rootShrink, midShrink bool) []Strand {
	return []Strand{
		{GUID: "root", Parent: "", PaneID: "%1", Live: true, Display: Display{Anchor: AnchorBelowParent, ShrinkWhenWaitingOnChild: rootShrink}},
		{GUID: "mid", Parent: "root", PaneID: "%2", Live: true, Display: Display{Anchor: AnchorBelowParent, ShrinkWhenWaitingOnChild: midShrink}},
		{GUID: "active", Parent: "mid", PaneID: "%3", Live: true, Display: Display{Anchor: AnchorBelowParent}},
	}
}

func TestStackHeightsFillBoxAndCollapsedStripEqualsParam(t *testing.T) {
	tests := []struct {
		name               string
		collapsedStripRows int
		boxH               int
	}{
		{"strip1", 1, 15},
		{"strip2", 2, 15},
		{"strip4", 4, 20},
		{"strip6", 6, 30},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// A single-ancestor chain: root collapses (shrink:true), mid
			// stays full (shrink:false), active is always full.
			stack := chainStack(true, false)
			box := Box{X: 0, Y: 0, W: 100, H: tt.boxH}
			p := Params{CollapsedStripRows: tt.collapsedStripRows, MinFullRows: 3}

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

			// root is the only collapsed ancestor in this fixture.
			if placements[0].height != tt.collapsedStripRows {
				t.Errorf("collapsed ancestor height = %d, want CollapsedStripRows %d", placements[0].height, tt.collapsedStripRows)
			}
		})
	}
}

func TestStackHeightsActiveStrictlyTallestWithSingleAncestor(t *testing.T) {
	// root collapses (single shrink:true ancestor), mid also collapses so
	// the active pane is genuinely alone as the sole full pane.
	stack := []Strand{
		{GUID: "root", Parent: "", PaneID: "%1", Live: true, Display: Display{Anchor: AnchorBelowParent, ShrinkWhenWaitingOnChild: true}},
		{GUID: "active", Parent: "root", PaneID: "%2", Live: true, Display: Display{Anchor: AnchorBelowParent}},
	}
	box := Box{X: 0, Y: 0, W: 100, H: 15}
	p := Params{CollapsedStripRows: 2, MinFullRows: 3}

	placements := stackHeights(stack, box, p)
	if placements[1].height <= placements[0].height {
		t.Errorf("active height %d must be strictly greater than ancestor height %d", placements[1].height, placements[0].height)
	}
	if placements[0].height != p.CollapsedStripRows {
		t.Errorf("ancestor height = %d, want CollapsedStripRows %d", placements[0].height, p.CollapsedStripRows)
	}
}

func TestStackHeightsRemainderGoesToActivePane(t *testing.T) {
	// root stays full (shrink:false) even though it is mid's ancestor; mid
	// collapses; active is full. So there are two full panes (root,
	// active) splitting the remainder, and >=2-full-pane remainder
	// assignment must be deterministic: always to the active/bottom pane.
	stack := chainStack(false, true)
	box := Box{X: 0, Y: 0, W: 100, H: 21} // usable = 21 - 2 dividers = 19
	p := Params{CollapsedStripRows: 2, MinFullRows: 3}

	placements := stackHeights(stack, box, p)
	rootH, midH, activeH := placements[0].height, placements[1].height, placements[2].height

	if midH != p.CollapsedStripRows {
		t.Fatalf("mid (collapsed) height = %d, want %d", midH, p.CollapsedStripRows)
	}
	// usable=19, stripDemand=2, fullRemaining=17, fullBase=8, remainder=1.
	if rootH != 8 {
		t.Errorf("root (full, non-active) height = %d, want 8 (base, no remainder)", rootH)
	}
	if activeH != 9 {
		t.Errorf("active height = %d, want 9 (base + remainder)", activeH)
	}
	if activeH <= rootH {
		t.Errorf("active height %d must exceed the other full pane's height %d once the remainder is applied", activeH, rootH)
	}

	sum := rootH + midH + activeH
	if dividers := len(stack) - 1; sum+dividers != box.H {
		t.Errorf("heights sum + dividers = %d, want box.H %d", sum+dividers, box.H)
	}
}

func TestStackHeightsClampYieldsOnlyPositiveHeightsInTooShortWindow(t *testing.T) {
	// Three ancestors (all shrink:true, each demanding CollapsedStripRows=3)
	// plus one active pane, but the window only has 5 usable rows for 4
	// panes — the natural split would drive the active pane negative.
	// clampToFit must reclaim rows from the strips (priority 1) first and
	// still land on an exact, all-positive split.
	stack := []Strand{
		{GUID: "r1", Parent: "", PaneID: "%1", Live: true, Display: Display{Anchor: AnchorBelowParent, ShrinkWhenWaitingOnChild: true}},
		{GUID: "r2", Parent: "r1", PaneID: "%2", Live: true, Display: Display{Anchor: AnchorBelowParent, ShrinkWhenWaitingOnChild: true}},
		{GUID: "r3", Parent: "r2", PaneID: "%3", Live: true, Display: Display{Anchor: AnchorBelowParent, ShrinkWhenWaitingOnChild: true}},
		{GUID: "active", Parent: "r3", PaneID: "%4", Live: true, Display: Display{Anchor: AnchorBelowParent}},
	}
	dividers := len(stack) - 1
	usable := 5
	box := Box{X: 0, Y: 0, W: 100, H: usable + dividers}
	p := Params{CollapsedStripRows: 3, MinFullRows: 3}

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
	stack := []Strand{
		{GUID: "r1", Parent: "", PaneID: "%1", Live: true, Display: Display{Anchor: AnchorBelowParent, ShrinkWhenWaitingOnChild: true}},
		{GUID: "r2", Parent: "r1", PaneID: "%2", Live: true, Display: Display{Anchor: AnchorBelowParent, ShrinkWhenWaitingOnChild: true}},
		{GUID: "r3", Parent: "r2", PaneID: "%3", Live: true, Display: Display{Anchor: AnchorBelowParent, ShrinkWhenWaitingOnChild: true}},
		{GUID: "active", Parent: "r3", PaneID: "%4", Live: true, Display: Display{Anchor: AnchorBelowParent}},
	}
	box := Box{X: 0, Y: 0, W: 100, H: 6} // usable = 6 - 3 dividers = 3, less than 4 panes
	p := Params{CollapsedStripRows: 3, MinFullRows: 3}

	placements := stackHeights(stack, box, p)
	for _, pl := range placements {
		if pl.height <= 0 {
			t.Errorf("placement %+v has non-positive height in an impossible-to-fit window", pl)
		}
	}
}

// TestClampHeaderHeight covers the window-split clamp: the header yields
// rows first so the strand-stack region never shrinks below MinFullRows
// (floored at 1) total rows, distinct from clampToFit's job of distributing
// rows AMONG strands inside an already-shrunk box.
func TestClampHeaderHeight(t *testing.T) {
	tests := []struct {
		name         string
		headerRows   int
		windowRows   int
		minStackRows int
		want         int
	}{
		{"WellWithinFloor_Unclamped", 3, 21, 3, 3},
		{"ExactlyAtFloor_Unclamped", 18, 21, 3, 18},
		{
			// An oversized configured height_rows must yield rows so the
			// stack region keeps its MinFullRows floor, even though that
			// means the header itself ends up shorter than configured.
			name: "Oversized_ClampedToPreserveFloor", headerRows: 25, windowRows: 21, minStackRows: 3, want: 18,
		},
		{
			// The window cannot fit both a header and the floor at all: the
			// header still keeps its 1-row minimum (real tmux/psmux does not
			// cleanly support a zero-height select-layout cell — see
			// height.go's doc comment) rather than going to zero, even
			// though that means the stack floor itself is violated instead.
			name: "WindowTooShortForBoth_HeaderFlooredAtOne", headerRows: 5, windowRows: 2, minStackRows: 3, want: 1,
		},
		{
			// headerRows <= 0 (including the negative-treated-as-zero case)
			// still floors to 1 once the window has any rows to give — a
			// header pane exists whenever this function is called, so it
			// can never legitimately request/receive a zero-height cell.
			name: "NegativeHeaderRows_FlooredAtOne", headerRows: -4, windowRows: 21, minStackRows: 3, want: 1,
		},
		{"NonPositiveMinStackRowsFlooredAtOne", 25, 21, 0, 20},
		{
			// windowRows itself has nothing to give: the result is 0, not a
			// floored 1, since there is no row available at all.
			name: "ZeroWindowRows_NothingToGive", headerRows: 5, windowRows: 0, minStackRows: 3, want: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := clampHeaderHeight(tt.headerRows, tt.windowRows, tt.minStackRows); got != tt.want {
				t.Errorf("clampHeaderHeight(%d, %d, %d) = %d, want %d", tt.headerRows, tt.windowRows, tt.minStackRows, got, tt.want)
			}
			// Invariant every case must hold: the stack region resulting
			// from this clamp never shrinks below the floored MinFullRows.
			floor := tt.minStackRows
			if floor < 1 {
				floor = 1
			}
			got := clampHeaderHeight(tt.headerRows, tt.windowRows, tt.minStackRows)
			if stackRows := tt.windowRows - got; stackRows < floor && tt.windowRows >= floor {
				t.Errorf("clampHeaderHeight(%d, %d, %d) left only %d stack rows, want >= floor %d", tt.headerRows, tt.windowRows, tt.minStackRows, stackRows, floor)
			}
		})
	}
}

func TestIsAncestorAndBuildStackBodyIntegration(t *testing.T) {
	// Cards 5 (layout.go) and 7 (focus.go) ship no standalone _test.go
	// files; this exercises isAncestor's role in stackHeights' collapse
	// decision together with buildStackBody/wrapLayout turning the
	// resulting placements into a checksum-prefixed layout string.
	stack := chainStack(true, false)
	if !isAncestor(stack[0], stack) {
		t.Errorf("root must be an ancestor of mid and active")
	}
	if isAncestor(stack[2], stack) {
		t.Errorf("active (leaf) must not be an ancestor of anything")
	}

	box := Box{X: 0, Y: 0, W: 100, H: 15}
	p := Params{CollapsedStripRows: 2, MinFullRows: 3}
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
