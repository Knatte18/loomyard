// handle.go implements the `plan:` handle grammar: a handle has no reality to point at, unlike a
// glyph, so letting the planner invent its draft spelling is safe in a way that letting it invent
// a real glyph is not — quarry never sees a handle. This file holds the handle-specific parse
// helpers card 9 introduces (splitHandleDeclaration, handleUnit) and the reusable
// declared/referenced predicates card 10's checks share, so validate.go stays a check-dispatch
// file. classifyRef (classify.go) is the sole classifier that recognizes the "plan:" prefix as a
// shape; this file never re-implements that classification.

package planparser

import "strings"

// HandlePrefix is the literal prefix that marks a ref as a `plan:` handle. This package is the
// sole declarer of that literal.
const HandlePrefix = "plan:"

// splitHandleDeclaration matches a "**Create:**" sub-bullet's two-field declaration grammar,
// “ `plan:<draft-handle>` -> `<declaration head>` “, against payload's raw, unstripped text. It
// reuses moveLineRe -- the same “ `x` -> `y` “ shape parseRenameField already matches -- rather
// than declaring a second regex, and additionally requires the left-hand token to carry
// HandlePrefix, distinguishing a handle declaration from a plain Rename-style pair. ok is false
// for a payload that does not match moveLineRe at all, or that matches it but whose left-hand
// token does not carry HandlePrefix.
func splitHandleDeclaration(payload string) (handle, decl string, ok bool) {
	m := moveLineRe.FindStringSubmatch(payload)
	if m == nil {
		return "", "", false
	}
	if !strings.HasPrefix(m[1], HandlePrefix) {
		return "", "", false
	}
	return m[1], m[2], true
}

// handleUnit returns the substring of handle left of the first "#" following HandlePrefix, and
// whether one was found. It performs no glyph parsing and is not a glyph<->path conversion: it
// splits a handle, which is loomyard's own token, never a quarry answer.
func handleUnit(handle string) (string, bool) {
	rest := strings.TrimPrefix(handle, HandlePrefix)
	idx := strings.Index(rest, "#")
	if idx == -1 {
		return "", false
	}
	return rest[:idx], true
}

// declaredHandles maps every handle declared by some card's own Create group (via
// CardDeclaration) to every card ID that declares it, in card order. A handle declared twice by
// the same card appears once per declaration, matching handle-collision's own per-declaration
// counting.
func declaredHandles(plan *Plan) map[string][]string {
	declared := make(map[string][]string)
	for _, c := range plan.Cards {
		for _, d := range c.Declarations {
			declared[d.Handle] = append(declared[d.Handle], cardID(c))
		}
	}
	return declared
}

// referencedHandles maps every handle appearing in some card's Targets or Uses to every card ID
// referencing it, in card order. A Rename group's to-side handle is included, since it is
// projected into Targets exactly like any other ref.
func referencedHandles(plan *Plan) map[string][]string {
	referenced := make(map[string][]string)
	for _, c := range plan.Cards {
		for _, fields := range [][]string{c.Targets, c.Uses} {
			for _, r := range fields {
				if classifyRef(r) != refKindHandle {
					continue
				}
				referenced[r] = append(referenced[r], cardID(c))
			}
		}
	}
	return referenced
}
