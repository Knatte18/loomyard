// focus.go resolves which pane receives tmux input focus.

package render

// focusTarget returns the pane id of the strand that should receive tmux
// focus, exactly one per session. If one or more strands in ordered declare
// Display.Focus, the bottom-most such strand wins (ties resolve to
// bottom-most). Otherwise the bottom-most strand in ordered is the default
// focus target — ordered places the most recently inserted strand last, so the
// bottom pane is always the one currently in use. Returns "" when ordered is
// empty.
func focusTarget(ordered []Strand) string {
	for i := len(ordered) - 1; i >= 0; i-- {
		if ordered[i].Display.Focus {
			return ordered[i].PaneID
		}
	}
	if len(ordered) == 0 {
		return ""
	}
	return ordered[len(ordered)-1].PaneID
}
