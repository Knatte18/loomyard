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

// IsHandleRef reports whether raw is a "plan:" handle. It implements exactly classifyRef's rule 1
// (the "plan:" prefix test, classify.go) and is the one handle-vs-not predicate other packages
// consume — never re-implemented against classifyRef itself, since that classifier stays
// unexported.
func IsHandleRef(raw string) bool {
	return strings.HasPrefix(raw, HandlePrefix)
}

// HandleBody returns the substring of raw after HandlePrefix, and true when IsHandleRef(raw).
// It reports ("", false) for a raw that is not handle-shaped at all. This is the "plan:"-strip
// internal/planglyph's resolveKeyFor and BindHandles open-code today; both are migrated onto this
// accessor rather than re-deriving the strip.
func HandleBody(raw string) (string, bool) {
	if !IsHandleRef(raw) {
		return "", false
	}
	return strings.TrimPrefix(raw, HandlePrefix), true
}

// NewHandle builds a draft "plan:" handle over glyphID. It is the one sanctioned handle
// construction: every caller that needs to spell a handle from a glyph-shaped body uses this
// rather than concatenating HandlePrefix itself.
func NewHandle(glyphID string) string {
	return HandlePrefix + glyphID
}

// HandleMember returns the member-name portion of a plan: handle — the substring after its first
// "#" — and whether one was found. Local string work over loomyard's own plan: token, never glyph
// grammar, so the Glyph Conversion Chokepoint is untouched. Port of
// internal/planglyph's draftHandleMember.
func HandleMember(handle string) (string, bool) {
	idx := strings.Index(handle, "#")
	if idx == -1 {
		return "", false
	}
	return handle[idx+1:], true
}

// HandleIdentifier returns the bare declared identifier a plan: handle's member half names: its
// last dot-separated component. A member is "Name" for a free declaration and "Owner.Name" for a
// method, and only the Name half ever appears in a declaration head — a method's owner is carried
// by its receiver clause, not by its identifier. Substituting the qualified form into a signature
// produces text a declaration head can never legally carry. Reports false for a handle carrying no
// member at all, or one whose member (or final dot-segment) is empty. Like HandleMember this is
// local string work over loomyard's own plan: token, never glyph grammar. Port of
// internal/planglyph's draftHandleIdentifier.
func HandleIdentifier(handle string) (string, bool) {
	member, ok := HandleMember(handle)
	if !ok || member == "" {
		return "", false
	}
	if idx := strings.LastIndex(member, "."); idx != -1 {
		member = member[idx+1:]
	}
	if member == "" {
		return "", false
	}
	return member, true
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
			if _, disp := lookup(gateHandleClaims, p.New); disp != dispKeep {
				continue
			}
			claims[p.New] = append(claims[p.New], handleClaim{card: id, fromRename: true})
		}
	}
	return claims
}

// fileUnitRuleApplies reports whether checkHandleMalformed's file-unit rule (validate.go) binds a
// handle carrying claims.
//
// It does not bind a handle claimed ONLY as a Rename pair's to-side, because such a handle's unit
// half is never read: internal/planglyph's renameDeclSource takes the derived declaration's Unit
// from the RESOLVED old side, deliberately, so that a draft misspelling the unit is corrected by
// canonicalization rather than propagated. The rule's whole stated consequence — quarry's Name
// echoes a file-unit member ID its own Resolve will never answer — cannot arise for that handle,
// so firing on it refused a plan that would have canonicalized correctly, with a detail asserting
// something untrue of it (crucible round opus-high-r9, R9-5).
//
// A handle with NO claim at all keeps the rule: handle-dangling is already reporting it, nothing
// says which source was intended, and the extra diagnostic can only help.
func fileUnitRuleApplies(claims []handleClaim) bool {
	if len(claims) == 0 {
		return true
	}
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
				if _, disp := lookup(gateReferencedHandles, r); disp != dispKeep {
					continue
				}
				referenced[r] = append(referenced[r], cardID(c))
			}
		}
	}
	return referenced
}
