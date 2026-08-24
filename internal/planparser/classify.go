// classify.go implements the format-4 card model's shape classifier: classifyRef decides, by
// string shape alone, whether a card ref (a Targets/Uses entry, or one side of a Rename pair) is
// a file path or a package-qualified symbol. Per the shape-classification-at-validation decision,
// this is the package's sole classifier — normalizeCard (normalize.go) and the path-shaped
// validator checks (validate.go) both gate on it, and it is never called at parse time. The
// function performs string analysis only: it never stats the disk and never spawns a process, so
// this file stays a tier1-pure leaf per the Test Tier Purity Invariant.

package planparser

import "strings"

// refKind is the shape classifyRef assigns to one card ref.
type refKind int

const (
	// refKindPath marks a ref classified as a file path.
	refKindPath refKind = iota
	// refKindSymbol marks a ref classified as a package-qualified symbol.
	refKindSymbol
)

// classifyRef classifies raw by shape alone, applying exactly three rules in order:
//  1. raw contains a "/" -> refKindPath (this also covers the "//" worktree-root escape, which
//     always contains a slash).
//  2. otherwise, if raw contains a "." and the segment after the final "." is non-empty and
//     consists entirely of lowercase ASCII letters and ASCII digits -> refKindPath (a bare
//     filename with a lowercase extension, e.g. "list.go").
//  3. otherwise -> refKindSymbol. This is the explicit default for two distinct cases: an entry
//     with no "." at all never reaches rule 2's test (e.g. "Lookup", "Makefile"), and an entry
//     whose final dot-segment is not all-lowercase-alphanumeric falls through from rule 2 (e.g.
//     "shedrecipe.Lookup"). "shedrecipe.lookup" is a documented misclassification: it reaches
//     refKindPath because its final segment happens to be all-lowercase, exactly as rule 2 requires.
func classifyRef(raw string) refKind {
	if strings.Contains(raw, "/") {
		return refKindPath
	}

	if dot := strings.LastIndex(raw, "."); dot != -1 {
		segment := raw[dot+1:]
		if segment != "" && isLowerAlphanumeric(segment) {
			return refKindPath
		}
	}

	return refKindSymbol
}

// isLowerAlphanumeric reports whether s consists entirely of lowercase ASCII letters and ASCII
// digits, checked byte-by-byte so a non-ASCII rune can never be treated as lowercase.
func isLowerAlphanumeric(s string) bool {
	for i := 0; i < len(s); i++ {
		b := s[i]
		if (b >= 'a' && b <= 'z') || (b >= '0' && b <= '9') {
			continue
		}
		return false
	}
	return true
}

// isPathRef is a convenience wrapper reporting whether raw classifies as a path.
func isPathRef(raw string) bool {
	return classifyRef(raw) == refKindPath
}
