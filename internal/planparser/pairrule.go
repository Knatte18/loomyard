// pairrule.go implements the one rule that decides whether two cards' overlapping refs make a dependency edge, shared by Webster's batch-order check and the plandag measurement tool.
// Overlap detection stays each caller's; this file only judges a pair already found overlapping.

package planparser

import "strings"

// RefKind is what one card does with one ref.
// The zero value means the card does not name the ref.
type RefKind int

const (
	// RefUse marks a ref a card only names under Uses.
	RefUse RefKind = 1
	// RefChange marks a ref a card changes: it sits under a type label other than Extends.
	RefChange RefKind = 2
	// RefExtends marks a ref a card only adds to: it sits under Extends groups alone.
	RefExtends RefKind = 3
)

// CardRefKind reports what card does with ref, compared by exact string after trimming.
// A group of any type label but Extends holding ref makes it RefChange, as does card.Targets holding it when no group does, which is how a hand-built card with no groups names its targets.
// A ref held by Extends groups alone is RefExtends, one held by Uses alone is RefUse, and a ref the card does not name is 0.
func CardRefKind(card Card, ref string) RefKind {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return 0
	}
	inExtends := false
	for _, group := range card.TargetGroups {
		if !holdsRef(group.Refs, ref) {
			continue
		}
		if group.Type != CardTypeExtends {
			return RefChange
		}
		inExtends = true
	}
	switch {
	case inExtends:
		return RefExtends
	case holdsRef(card.Targets, ref):
		return RefChange
	case holdsRef(card.Uses, ref):
		return RefUse
	}
	return 0
}

// holdsRef reports whether refs holds ref once each entry is trimmed.
func holdsRef(refs []string, ref string) bool {
	for _, candidate := range refs {
		if strings.TrimSpace(candidate) == ref {
			return true
		}
	}
	return false
}

// DependencyEdge reports whether the pair of cards makes a dependency edge, for a ref both cards name that the caller already found overlapping.
// It is false when either kind is 0 or both are RefUse, and for two RefExtends exactly when both cards fence a verbatim PATTERN.md line for one entry ID.
// Every other pair makes an edge: an Extends against any other label, a Uses against an Extends target, and each pair the rule made an edge before Extends existed.
func DependencyEdge(earlier Card, earlierKind RefKind, later Card, laterKind RefKind) bool {
	switch {
	case earlierKind == 0 || laterKind == 0:
		return false
	case earlierKind == RefUse && laterKind == RefUse:
		return false
	case earlierKind == RefExtends && laterKind == RefExtends:
		return fenceSharedPatternEntry(earlier, later)
	}
	return true
}

// fenceSharedPatternEntry reports whether the two cards fence a verbatim PATTERN.md line for one entry ID.
func fenceSharedPatternEntry(left, right Card) bool {
	rightIDs := map[string]bool{}
	for _, entry := range CardPatternEntries(right) {
		rightIDs[entry.ID] = true
	}
	for _, entry := range CardPatternEntries(left) {
		if rightIDs[entry.ID] {
			return true
		}
	}
	return false
}
