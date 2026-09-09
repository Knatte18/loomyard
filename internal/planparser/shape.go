// shape.go implements the Ref-Shape Registry: the single kind-policy ledger every planparser
// dispatch site that gates on classifyRef's four-way refKind consults, instead of each site
// hand-picking its own kind subset. The ledger's unit is one kind-gate (a named refGate constant),
// not one function -- a function with several independent gates registers one named policy per
// gate. lookup is the fail-closed entry point: an undeclared disposition (the zero value) panics
// rather than silently skipping the ref, so a fifth refKind added to classify.go's enum cannot
// slip past any gate unnoticed -- the enum<->allRefKinds sync meta-test and the ledger
// completeness meta-test in shape_test.go make that guarantee mechanical rather than aspirational.
//
// This file also holds diskPathForRef and refKindName, relocated verbatim from validate.go: both
// are exhaustive kind->behavior switches, the registry's own nucleus, and are the file's two
// sanctioned switch-form ledger entries -- diskPathForRef's default arm and refKindName's
// "an unrecognized shape" fallthrough are how each stays fail-closed without going through the
// map-based ledger.
//
// Per the Ref-Shape Registry Invariant (CONSTRAINTS.md), this file and classify.go are the only
// two files in internal/planparser (and the only files at all, across internal/planparser and
// internal/planglyph) where a refKind comparison or a classifyRef call is legal outside a
// _test.go file; every other production dispatch site routes through lookup or through the
// exported handle vocabulary.
//
// shape.go stays tier1-pure like classify.go: string analysis only, stdlib imports only, no disk
// access, no glyph.Parse.

package planparser

import "fmt"

// disposition is the action a refGate takes for one refKind. The zero value is deliberately
// unnamed and means "undeclared" -- a gate's policy map missing an entry for some refKind, or a
// gate missing from ledger entirely, both yield this zero value at lookup, and lookup panics
// rather than treating it as any of the three named dispositions.
type disposition int

const (
	// dispKeep means the gate acts on this kind.
	dispKeep disposition = iota + 1
	// dispSkip means the gate deliberately ignores this kind -- the ref passes through untouched.
	dispSkip
	// dispFinding means the gate raises its own finding for this kind.
	dispFinding
)

// refGate names one kind-gate: one independently-declared policy over refKind, keyed by the
// consuming function (or, for a multi-gate function, one arm of it) that owns it.
type refGate string

const (
	// gateBareSymbolTarget is checkBareSymbolTarget's (validate.go) policy: a bare
	// package-qualified symbol is its own finding; every other kind passes through untouched.
	gateBareSymbolTarget refGate = "bare-symbol-target"
	// gateDirectoryTarget is checkDirectoryTarget's (validate.go) policy: only a path-shaped ref
	// is a candidate for the directory-target finding; every other kind is skipped.
	gateDirectoryTarget refGate = "directory-target"
	// gateGlyphMalformed is checkGlyphMalformed's (validate.go) policy: only a glyph-shaped ref is
	// checked against glyph.Parse; every other kind is skipped.
	gateGlyphMalformed refGate = "glyph-malformed"
	// gateHandleMalformed is checkHandleMalformed's (validate.go) policy: only a handle-shaped ref
	// is checked against the handle grammar; every other kind is skipped.
	gateHandleMalformed refGate = "handle-malformed"
	// gateSyntacticContainment is syntacticContainment's (containment.go) policy: only a
	// glyph-shaped ref is a candidate for cross-granularity containment; every other kind is
	// skipped.
	gateSyntacticContainment refGate = "syntactic-containment"
	// gateHandleClaims is handleClaims' (handle.go) policy: only a handle-shaped Rename New side
	// is a claim on that handle; every other kind is skipped.
	gateHandleClaims refGate = "handle-claims"
	// gateReferencedHandles is referencedHandles' (handle.go) policy: only a handle-shaped ref
	// counts as a reference to that handle; every other kind is skipped.
	gateReferencedHandles refGate = "referenced-handles"
	// gateFileRenamePair is isFileRenamePair's (validate.go) policy, applied identically to both
	// sides of a Rename pair: only a glyph-shaped side is a candidate for a file-rename pair;
	// every other kind is skipped, meaning "not a file-rename pair".
	gateFileRenamePair refGate = "file-rename-pair"
	// gateRenameTo is checkRenamePairShape's (validate.go) to-side policy: a handle-shaped new
	// side is kept; a path-, symbol-, or glyph-shaped new side is the rename-to-not-handle
	// finding.
	gateRenameTo refGate = "rename-to"
	// gateRenameFrom is checkRenamePairShape's (validate.go) from-side policy: a glyph-shaped old
	// side is kept; a path-, symbol-, or handle-shaped old side is the rename-from-not-glyph
	// finding.
	gateRenameFrom refGate = "rename-from"
	// gateNormalizePath is normalizeRefIfPath's (normalize.go) policy: only a path-shaped ref is
	// root:-joined; every other kind passes through untouched. Its own doc comment calls this
	// "the single sharpest regression this migration can introduce".
	gateNormalizePath refGate = "normalize-path"
	// gateCanonicalizePath is canonicalizeCard's (normalize.go) canon closure policy: only a
	// path-shaped ref is a candidate for glyph canonicalization; every other kind passes through
	// untouched.
	gateCanonicalizePath refGate = "canonicalize-path"
	// gateProsaPathOnly is checkProsaSymbolTarget's (validate.go) language:none branch policy: a
	// path-shaped ref is kept; a symbol-, glyph-, or handle-shaped ref is the prosa-symbol-target
	// finding.
	gateProsaPathOnly refGate = "prosa-path-only"
)

// allRefKinds is the canonical, complete list of every refKind classify.go's enum declares. The
// registry's meta-tests key on it: TestRefKindEnumMatchesAllRefKinds (shape_test.go) asserts it
// stays in exact sync with classify.go's const block, and TestLedgerCompleteness asserts every
// ledger policy's domain equals it exactly.
var allRefKinds = []refKind{refKindPath, refKindSymbol, refKindGlyph, refKindHandle}

// ledger is the package-level registry of every gate's policy: one map[refKind]disposition per
// refGate, covering exactly the thirteen policies declared in this file's refGate constants. A
// refGate absent from ledger, or a refKind absent from one of ledger's policy maps, both yield the
// disposition zero value at lookup -- undeclared, and therefore fail-closed.
var ledger = map[refGate]map[refKind]disposition{
	gateBareSymbolTarget: {
		refKindPath:   dispSkip,
		refKindSymbol: dispFinding,
		refKindGlyph:  dispSkip,
		refKindHandle: dispSkip,
	},
	gateDirectoryTarget: {
		refKindPath:   dispKeep,
		refKindSymbol: dispSkip,
		refKindGlyph:  dispSkip,
		refKindHandle: dispSkip,
	},
	gateGlyphMalformed: {
		refKindPath:   dispSkip,
		refKindSymbol: dispSkip,
		refKindGlyph:  dispKeep,
		refKindHandle: dispSkip,
	},
	gateHandleMalformed: {
		refKindPath:   dispSkip,
		refKindSymbol: dispSkip,
		refKindGlyph:  dispSkip,
		refKindHandle: dispKeep,
	},
	gateSyntacticContainment: {
		refKindPath:   dispSkip,
		refKindSymbol: dispSkip,
		refKindGlyph:  dispKeep,
		refKindHandle: dispSkip,
	},
	gateHandleClaims: {
		refKindPath:   dispSkip,
		refKindSymbol: dispSkip,
		refKindGlyph:  dispSkip,
		refKindHandle: dispKeep,
	},
	gateReferencedHandles: {
		refKindPath:   dispSkip,
		refKindSymbol: dispSkip,
		refKindGlyph:  dispSkip,
		refKindHandle: dispKeep,
	},
	gateFileRenamePair: {
		refKindPath:   dispSkip,
		refKindSymbol: dispSkip,
		refKindGlyph:  dispKeep,
		refKindHandle: dispSkip,
	},
	gateRenameTo: {
		refKindPath:   dispFinding,
		refKindSymbol: dispFinding,
		refKindGlyph:  dispFinding,
		refKindHandle: dispKeep,
	},
	gateRenameFrom: {
		refKindPath:   dispFinding,
		refKindSymbol: dispFinding,
		refKindGlyph:  dispKeep,
		refKindHandle: dispFinding,
	},
	gateNormalizePath: {
		refKindPath:   dispKeep,
		refKindSymbol: dispSkip,
		refKindGlyph:  dispSkip,
		refKindHandle: dispSkip,
	},
	gateCanonicalizePath: {
		refKindPath:   dispKeep,
		refKindSymbol: dispSkip,
		refKindGlyph:  dispSkip,
		refKindHandle: dispSkip,
	},
	gateProsaPathOnly: {
		refKindPath:   dispKeep,
		refKindSymbol: dispFinding,
		refKindGlyph:  dispFinding,
		refKindHandle: dispFinding,
	},
}

// lookupIn is lookup's pure core, split out for testability: it classifies ref via classifyRef,
// reads policy[k], and panics with a message naming g and refKindName(k) when the resulting
// disposition is the zero value -- an undeclared kind is a programming error in the registry
// itself, never a silent skip. A nil policy (e.g. a gate missing from ledger entirely) yields the
// same zero-value disposition and therefore the same fail-closed panic.
func lookupIn(g refGate, policy map[refKind]disposition, ref string) (refKind, disposition) {
	k := classifyRef(ref)
	d := policy[k]
	if d == 0 {
		panic(fmt.Sprintf("planparser: gate %q has no declared disposition for kind %s", g, refKindName(k)))
	}
	return k, d
}

// lookup classifies ref and returns its refKind alongside the disposition g's registered ledger
// policy declares for that kind, panicking (via lookupIn) when the disposition is undeclared. This
// is the registry's sole read path: every migrated dispatch site calls lookup (or lookupIn
// directly, for a test) rather than comparing classifyRef's return value itself.
func lookup(g refGate, ref string) (refKind, disposition) {
	return lookupIn(g, ledger[g], ref)
}

// diskPathForRef maps raw to the disk-relative path checkCardPathMalformed and checkPathMissing
// (and their two union builders) key on: a path-shaped raw maps to itself; a glyph-shaped raw maps
// through parseGlyph followed by Glyph.UnitPath() -- the one glyph->path call, per the
// glyph-conversion-chokepoint Shared Decision -- but only for a self glyph (IsSelf() true). A
// member glyph names a symbol within its unit, not the unit itself, so mapping it to its unit's
// directory and existence-checking that directory would answer a question these two checks were
// never meant to answer (member existence is internal/planglyph's resolve-backed business); a
// member glyph is therefore skipped (ok false) exactly like a not-ok UnitPath or a failed
// parseGlyph, rather than reported. Any other shape (a plan: handle, a bare symbol) is also
// skipped. ok is also false whenever plan.Language reports not-ok (e.g. "none"): a glyph-shaped
// entry is never resolved when the plan has opted out of the alphabet.
func diskPathForRef(plan *Plan, raw string) (string, bool) {
	switch classifyRef(raw) {
	case refKindPath:
		return raw, true
	case refKindGlyph:
		lang, ok := planLanguage(plan)
		if !ok {
			return "", false
		}
		g, err := parseGlyph(lang, raw)
		if err != nil || !g.IsSelf() {
			return "", false
		}
		return g.UnitPath()
	default:
		return "", false
	}
}

// refKindName names a refKind in the prose form checkRenamePairShape's own findings use, so a
// finding says what the offending side actually is rather than only what it failed to be.
func refKindName(k refKind) string {
	switch k {
	case refKindPath:
		return "a file path"
	case refKindSymbol:
		return "a bare symbol"
	case refKindGlyph:
		return "a glyph"
	case refKindHandle:
		return "a plan: handle"
	}
	// Unreachable while classifyRef returns only the four kinds above, and deliberately not a
	// panic: this package is lenient at card level, and a shape it cannot name is still a shape it
	// must report rather than crash the whole validation pass over.
	return "an unrecognized shape"
}
