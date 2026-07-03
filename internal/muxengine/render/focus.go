// focus.go resolves which pane receives psmux input focus, and detects
// whether a strand has a descendant present in the ordered stack — the
// isAncestor test the height policy (height.go) uses to decide whether a
// shrink:true strand collapses to a compact strip.

package render

// focusTarget returns the pane id of the strand that should receive psmux
// focus, exactly one per session. If one or more strands in ordered declare
// Display.Focus, the bottom-most such strand wins (ties resolve to
// bottom-most). Otherwise the bottom-most strand in ordered is the default
// focus target — ordered places the deepest/active descendant last, so the
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

// isAncestor reports whether s has a descendant present in ordered — some
// other entry whose Parent chain passes through s. The height policy
// collapses a shrink:true strand only when it is an ancestor of a present
// descendant; a strand with no descendant in ordered stays full height
// regardless of its ShrinkWhenWaitingOnChild setting, since there is
// nothing for it to be "waiting on".
func isAncestor(s Strand, ordered []Strand) bool {
	byGUID := make(map[string]Strand, len(ordered))
	for _, o := range ordered {
		byGUID[o.GUID] = o
	}
	for _, o := range ordered {
		if o.GUID == s.GUID {
			continue
		}
		cur := o.Parent
		for cur != "" {
			if cur == s.GUID {
				return true
			}
			parent, ok := byGUID[cur]
			if !ok {
				break
			}
			cur = parent.Parent
		}
	}
	return false
}
