// classify.go implements the format-5 card model's shape classifier: classifyRef decides, by
// string shape alone, whether a card ref (a Targets/Uses entry, or one side of a Rename pair) is
// a plan: handle, a glyph, a file path, or a package-qualified symbol. Per the
// shape-classification-at-validation decision, this is the package's sole classifier —
// normalizeCard (normalize.go), canonicalizeCard (normalize.go), and the shape-gated validator
// checks (validate.go) all gate on it, and it is never called at parse time to decide anything
// other than shape. The function performs string analysis only: it never stats the disk and never
// spawns a process, so this file stays a tier1-pure leaf per the Test Tier Purity Invariant. It
// never calls glyph.Parse either — existence and grammar validation of a glyph-shaped ref is
// glyphref.go's business, and (for a glyph's disk existence) internal/planglyph's.

package planparser

import "strings"

// refKind is the shape classifyRef assigns to one card ref.
type refKind int

const (
	// refKindPath marks a ref classified as a file path.
	refKindPath refKind = iota
	// refKindSymbol marks a ref classified as a bare package-qualified symbol.
	refKindSymbol
	// refKindGlyph marks a ref classified as a glyph (contains "#").
	refKindGlyph
	// refKindHandle marks a ref classified as a "plan:"-prefixed handle.
	refKindHandle
)

// classifyRef classifies raw by shape alone, applying exactly five rules in order:
//  1. raw begins with the literal prefix "plan:" -> refKindHandle.
//  2. otherwise, raw contains "#" -> refKindGlyph. This rule must precede the path rules below,
//     because every Go glyph contains a "/" in its unit half, so a "/"-based path rule reached
//     first would send every Go glyph to refKindPath.
//  3. otherwise, raw contains a "/", or raw contains a "." and the segment after the final "." is
//     non-empty and consists entirely of lowercase ASCII letters and ASCII digits -> refKindPath (a
//     nested path, or a bare filename with a lowercase extension, e.g. "list.go").
//  4. otherwise, if raw contains no "." and no "/" -> refKindPath as well: an extensionless
//     repository-root filename such as "Makefile", "LICENSE" or "Dockerfile". This rule is required
//     rather than tidy — without it, such a filename would fall to rule 5's refKindSymbol with no
//     legal spelling left, since the "//" worktree-root escape does not rescue it (normalizeCardPath
//     strips the prefix and hands back the identical bare token). canonicalizablePath (normalize.go)
//     then carries the rule the rest of the way: a slash-free ref is canonicalized to its self glyph
//     even without an extension, so the spelling this rule admits is one every glyph-backed layer
//     downstream can actually act on.
//  5. otherwise -> refKindSymbol. This is the explicit default for an entry whose final dot-segment
//     is not all-lowercase-alphanumeric (e.g. "shedrecipe.Lookup"). "shedrecipe.lookup" is a
//     documented misclassification: it reaches refKindPath at rule 3 because its final segment
//     happens to be all-lowercase, exactly as that rule requires.
//
// Rule 4's consequence is worth stating plainly rather than leaving it to be discovered: a bare
// extensionless filename is a REPOSITORY-ROOT spelling only. Under a non-"." root:, a bare
// "Makefile" goes through normalizeRefIfPath to "<root>/Makefile" — a slashed extensionless path,
// which canonicalizablePath (normalize.go) declines and checkDirectoryTarget (validate.go) then
// refuses as a blocking finding, because a lexical classifier cannot tell that spelling from a
// directory. The legal spelling for a root:-scoped extensionless file is its repository-root-
// relative file self glyph ("<root>/Makefile#"), which quarry resolves found; the directory-target
// finding's own detail names that remedy (crucible round fable-high-r10, F7).
func classifyRef(raw string) refKind {
	if strings.HasPrefix(raw, HandlePrefix) {
		return refKindHandle
	}

	if strings.Contains(raw, "#") {
		return refKindGlyph
	}

	if strings.Contains(raw, "/") {
		return refKindPath
	}

	if dot := strings.LastIndex(raw, "."); dot != -1 {
		segment := raw[dot+1:]
		if segment != "" && isLowerAlphanumeric(segment) {
			return refKindPath
		}
	} else {
		// No "." and (per the rule above) no "/": an extensionless repository-root filename.
		return refKindPath
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

// isGlyphRef is a convenience wrapper reporting whether raw classifies as a glyph.
func isGlyphRef(raw string) bool {
	return classifyRef(raw) == refKindGlyph
}

// isHandleRef is a convenience wrapper reporting whether raw classifies as a plan: handle.
func isHandleRef(raw string) bool {
	return classifyRef(raw) == refKindHandle
}
