// focus_test.go pins orderStack composed with focusTarget across the three focus outcomes batch
// 2's design accepts. Case 3 — a persisted Display.Focus flag winning regardless of depth — is the
// one an implementer is most likely to read as a bug and "fix", and it is not: in a worktree
// opened through the VS Code chain the operator's own working pane already exists and is the agent
// session the chain focused. This pins an assumption internal/loomcli relies on rather than
// declares, which is why the test lives here beside the code that owns it.

package render

import "testing"

// TestFocusTarget_ColdBootstrapFallsThroughToBottomMost pins case 1: two parentless strands in
// insertion order — a status strand then an operator strand, neither carrying Display.Focus —
// resolve to the operator strand's pane. orderStack sorts by chain depth with sort.SliceStable and
// equal-depth strands keep insertion order, leaving the operator strand last, and focusTarget falls
// through to bottom-most.
func TestFocusTarget_ColdBootstrapFallsThroughToBottomMost(t *testing.T) {
	strands := []Strand{
		{GUID: "status", PaneID: "%1", Live: true, Display: Display{Anchor: AnchorBelowParent}},
		{GUID: "operator", PaneID: "%2", Live: true, Display: Display{Anchor: AnchorBelowParent}},
	}
	ordered := orderStack(strands)
	got := focusTarget(ordered)
	if want := "%2"; got != want {
		t.Errorf("focusTarget(cold bootstrap) = %q; want %q (the operator strand)", got, want)
	}
}

// TestFocusTarget_ReentrantBootstrapFocusesTheDeepestStrand pins case 2: the same two strands plus
// a depth-1 strand parented onto one of them resolve to the depth-1 strand's pane. A parentless
// strand is no longer bottom-most once anything deeper exists.
func TestFocusTarget_ReentrantBootstrapFocusesTheDeepestStrand(t *testing.T) {
	strands := []Strand{
		{GUID: "status", PaneID: "%1", Live: true, Display: Display{Anchor: AnchorBelowParent}},
		{GUID: "operator", PaneID: "%2", Live: true, Display: Display{Anchor: AnchorBelowParent}},
		{GUID: "child", Parent: "operator", PaneID: "%3", Live: true, Display: Display{Anchor: AnchorBelowParent}},
	}
	ordered := orderStack(strands)
	got := focusTarget(ordered)
	if want := "%3"; got != want {
		t.Errorf("focusTarget(re-entrant bootstrap) = %q; want %q (the depth-1 strand)", got, want)
	}
}

// TestFocusTarget_PersistedFocusFlagWinsRegardlessOfDepth pins case 3: the same three strands plus
// a parentless strand carrying Display.Focus: true resolve to that strand's pane. focusTarget scans
// for the bottom-most Display.Focus strand before falling back to bottom-most-overall, so a
// persisted focus flag wins even though it is shallower than "child".
func TestFocusTarget_PersistedFocusFlagWinsRegardlessOfDepth(t *testing.T) {
	strands := []Strand{
		{GUID: "status", PaneID: "%1", Live: true, Display: Display{Anchor: AnchorBelowParent}},
		{GUID: "operator", PaneID: "%2", Live: true, Display: Display{Anchor: AnchorBelowParent}},
		{GUID: "child", Parent: "operator", PaneID: "%3", Live: true, Display: Display{Anchor: AnchorBelowParent}},
		{GUID: "focused", PaneID: "%4", Live: true, Display: Display{Anchor: AnchorBelowParent, Focus: true}},
	}
	ordered := orderStack(strands)
	got := focusTarget(ordered)
	if want := "%4"; got != want {
		t.Errorf("focusTarget(persisted focus flag) = %q; want %q (the strand carrying Display.Focus: true)", got, want)
	}
}
