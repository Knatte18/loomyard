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

// HandleUnit is handleUnit's exported form: internal/planglyph's CanonicalizeHandles needs a
// Create declaration's own unit half to build the quarry.Declaration it hands to quarry.Name, and
// this is the one accessor for that split, so planglyph never re-implements handle.go's own
// grammar.
func HandleUnit(handle string) (string, bool) {
	return handleUnit(handle)
}

// declaredHandles maps every handle declared by some card's own Create group (via
// CardDeclaration) to every card ID that declares it, in card order. A handle declared twice by
// the same card appears once per declaration.
// It is the Create-declaration half of handleClaims, kept separate because handle-unreferenced and
// checkHandleMalformed's file-unit rule bind to that half alone (validate.go).
func declaredHandles(plan *Plan) map[string][]string {
	declared := make(map[string][]string)
	for _, c := range plan.Cards {
		for _, d := range c.Declarations {
			declared[d.Handle] = append(declared[d.Handle], cardID(c))
		}
	}
	return declared
}

// handleClaim is one card's claim on one handle: which card makes it, and which of the plan
// format's two handle-declaring sources it came from.
type handleClaim struct {
	// card is the "N-<slug>" identity of the claiming card.
	card string
	// fromRename reports whether the claim is a Rename pair's own handle-shaped New side rather
	// than a Create sub-bullet's declaration.
	fromRename bool
}

// handleClaims maps every handle some card claims to every claim on it, in card order,
// declarations before Rename pairs within one card, one entry per claim rather than per card.
//
// The plan format has TWO sources that bring a handle into existence — a Create sub-bullet's
// declaration and a Rename pair's own to-side — and both are equally a claim on that handle's
// spelling. Keeping them in one index with the source recorded is what lets handle-dangling accept
// either, handle-collision count both, and checkHandleMalformed's file-unit rule apply to only the
// one whose unit half is actually read (validate.go).
//
// Counting only Create declarations, as handle-collision did before, let a handle claimed by both
// sources — or by two Rename to-sides — pass validation clean and then be resolved by silent map
// overwrite inside internal/planglyph's CanonicalizeHandles, which derives a Create declaration's
// unit from the handle itself but a Rename to-side's unit from the RESOLVED old side: two
// different canonical forms, one substitution key, the later one winning and rewriting the Create
// card's own declaration bullet to point at the rename's destination (crucible round opus-high-r9,
// R9-3).
func handleClaims(plan *Plan) map[string][]handleClaim {
	claims := make(map[string][]handleClaim)
	for _, c := range plan.Cards {
		id := cardID(c)
		for _, d := range c.Declarations {
			claims[d.Handle] = append(claims[d.Handle], handleClaim{card: id})
		}
		for _, p := range c.Pairs {
			if classifyRef(p.New) != refKindHandle {
				continue
			}
			claims[p.New] = append(claims[p.New], handleClaim{card: id, fromRename: true})
		}
	}
	return claims
}

// claimedFromRename reports whether any of claims is a Rename pair's own to-side.
func claimedFromRename(claims []handleClaim) bool {
	for _, cl := range claims {
		if cl.fromRename {
			return true
		}
	}
	return false
}

// claimedFromDeclaration reports whether any of claims is a Create sub-bullet's own declaration.
func claimedFromDeclaration(claims []handleClaim) bool {
	for _, cl := range claims {
		if !cl.fromRename {
			return true
		}
	}
	return false
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
