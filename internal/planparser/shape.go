// shape.go implements the Ref-Shape Registry:
// the single ledger of kind policies that every planparser dispatch site gating on a refKind consults.
// An undeclared disposition panics at lookup, so a new refKind cannot slip past a gate unnoticed.
//
// diskPathForRef and refKindName also live here as switch-form ledger entries:
// each is an exhaustive kind-to-behavior switch, fail-closed through its default arm or fallthrough.
//
// Per PATTERN-sole-parsers, only this file and classify.go may compare refKind or call classifyRef outside _test.go.
// The rule spans internal/planparser and internal/planglyph;
// every other production dispatch site routes through lookup or through the exported handle vocabulary.
//
// shape.go stays tier1-pure like classify.go:
// string analysis only, stdlib imports only, no disk access, no glyph.Parse.

package planparser

import "fmt"

// disposition is the action a refGate takes for one refKind.
// The zero value is unnamed and means "undeclared".
// A policy map missing an entry for some refKind, or a gate missing from ledger, yields it at lookup.
// lookup panics on it rather than treating it as one of the named dispositions.
type disposition int

const (
	// dispKeep means the gate acts on this kind.
	dispKeep disposition = iota + 1
	// dispSkip means the gate ignores this kind; the ref passes through untouched.
	dispSkip
	// dispFinding means the gate raises its own finding for this kind.
	dispFinding
)

// refGate names one kind-gate: an independently declared policy over refKind.
// It is keyed by the consuming function, or by one arm of a multi-gate function.
type refGate string

const (
	// gateBareSymbolTarget is checkBareSymbolTarget's (validate.go) policy:
	// a bare package-qualified symbol is its own finding; every other kind passes through untouched.
	gateBareSymbolTarget refGate = "bare-symbol-target"
	// gateDirectoryTarget is checkDirectoryTarget's (validate.go) policy:
	// only a path-shaped ref is a candidate for the directory-target finding; every other kind is skipped.
	gateDirectoryTarget refGate = "directory-target"
	// gateGlyphMalformed is checkGlyphMalformed's (validate.go) policy:
	// only a glyph-shaped ref is checked against glyph.Parse; every other kind is skipped.
	gateGlyphMalformed refGate = "glyph-malformed"
	// gateHandleMalformed is checkHandleMalformed's (validate.go) policy:
	// only a handle-shaped ref is checked against the handle grammar; every other kind is skipped.
	gateHandleMalformed refGate = "handle-malformed"
	// gateHandleClaims is handleClaims' (handle.go) policy:
	// only a handle-shaped Rename New side is a claim on that handle; every other kind is skipped.
	gateHandleClaims refGate = "handle-claims"
	// gateReferencedHandles is referencedHandles' (handle.go) policy:
	// only a handle-shaped ref counts as a reference to that handle; every other kind is skipped.
	gateReferencedHandles refGate = "referenced-handles"
	// gateFileRenamePair is isFileRenamePair's (validate.go) policy, applied identically to both sides of a Rename pair:
	// only a glyph-shaped side is a candidate for a file-rename pair;
	// every other kind is skipped, meaning "not a file-rename pair".
	gateFileRenamePair refGate = "file-rename-pair"
	// gateRenameTo is checkRenamePairShape's (validate.go) to-side policy:
	// a handle-shaped new side is kept;
	// a path-, symbol-, or glyph-shaped new side is the rename-to-not-handle finding.
	gateRenameTo refGate = "rename-to"
	// gateRenameFrom is checkRenamePairShape's (validate.go) from-side policy:
	// a glyph-shaped old side is kept;
	// a path-, symbol-, or handle-shaped old side is the rename-from-not-glyph finding.
	gateRenameFrom refGate = "rename-from"
	// gateNormalizePath is normalizeRefIfPath's (normalize.go) policy:
	// only a path-shaped ref is root:-joined; every other kind passes through untouched.
	gateNormalizePath refGate = "normalize-path"
	// gateCanonicalizePath is canonicalizeCard's (normalize.go) canon closure policy:
	// only a path-shaped ref is a candidate for glyph canonicalization; every other kind passes through untouched.
	gateCanonicalizePath refGate = "canonicalize-path"
	// gateProsaPathOnly is checkProsaSymbolTarget's (validate.go) language:none branch policy:
	// a path-shaped ref is kept; a symbol-, glyph-, or handle-shaped ref is the prosa-symbol-target finding.
	gateProsaPathOnly refGate = "prosa-path-only"
	// gateRedundantTarget is redundancyOwnerOf's (redundancy.go) policy:
	// a glyph- or handle-shaped target can belong to a package; a path- or symbol-shaped one is skipped.
	gateRedundantTarget refGate = "redundant-target"
	// gateResignTarget is checkResignNotMember's (validate.go) policy:
	// a glyph-shaped re-sign target is kept for a member check;
	// a path-, symbol- or handle-shaped one is the resign-not-member finding:
	// it names no member with a prior signature.
	gateResignTarget refGate = "resign-target"
	// gateEditTestFile is CheckEditTestFile's (edittestfile.go) policy:
	// only a glyph-shaped ref can be a `_test.go` file self glyph under Edit; every other kind is skipped.
	gateEditTestFile refGate = "edit-test-file"
	// gateUntypedFileRenamePair is SymbolRenamePairs' policy for a plan whose language has no glyph alphabet.
	// Its rule applies identically to both sides of a Rename pair:
	// only a path-shaped side is a candidate for a file-rename pair;
	// every other kind is skipped, meaning "not a file-rename pair".
	gateUntypedFileRenamePair refGate = "untyped-file-rename-pair"
	// gateDiskCheckedTarget is IsDiskCheckedTarget's (glyphref.go) policy:
	// a path-shaped done-check key, one canonicalization could not spell as a glyph, is judged on disk;
	// every other kind is resolved and skipped here.
	gateDiskCheckedTarget refGate = "disk-checked-target"
)

// allRefKinds is the canonical, complete list of every refKind classify.go's enum declares.
// The registry's meta-tests key on it:
// TestRefKindEnumMatchesAllRefKinds (shape_test.go) asserts it stays in exact sync with classify.go's const block.
// TestLedgerCompleteness asserts every ledger policy's domain equals it exactly.
var allRefKinds = []refKind{refKindPath, refKindSymbol, refKindGlyph, refKindHandle}

// ledger is the package-level registry of every gate's policy:
// one map[refKind]disposition per refGate, covering exactly the refGate constants declared in this file.
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
	gateRedundantTarget: {
		refKindPath:   dispSkip,
		refKindSymbol: dispSkip,
		refKindGlyph:  dispKeep,
		refKindHandle: dispKeep,
	},
	gateResignTarget: {
		refKindPath:   dispFinding,
		refKindSymbol: dispFinding,
		refKindGlyph:  dispKeep,
		refKindHandle: dispFinding,
	},
	gateEditTestFile: {
		refKindPath:   dispSkip,
		refKindSymbol: dispSkip,
		refKindGlyph:  dispKeep,
		refKindHandle: dispSkip,
	},
	gateUntypedFileRenamePair: {
		refKindPath:   dispKeep,
		refKindSymbol: dispSkip,
		refKindGlyph:  dispSkip,
		refKindHandle: dispSkip,
	},
	gateDiskCheckedTarget: {
		refKindPath:   dispKeep,
		refKindSymbol: dispSkip,
		refKindGlyph:  dispSkip,
		refKindHandle: dispSkip,
	},
}

// lookupIn classifies ref via classifyRef and reads the entry for its kind from policy, split out for testability.
// It panics with a message naming g and refKindName(k) when that entry is the zero value:
// an undeclared kind is a programming error in the registry itself, never a silent skip.
// A nil policy, such as one for a gate missing from the ledger, yields the same zero value and the same panic.
func lookupIn(g refGate, policy map[refKind]disposition, ref string) (refKind, disposition) {
	k := classifyRef(ref)
	d := policy[k]
	if d == 0 {
		panic(fmt.Sprintf("planparser: gate %q has no declared disposition for kind %s", g, refKindName(k)))
	}
	return k, d
}

// lookup classifies ref and returns its refKind with the disposition that gate g declares for that kind.
// It panics when g declares no disposition for the kind.
// Dispatch sites read a ref's disposition through lookup rather than classifying the ref themselves.
func lookup(g refGate, ref string) (refKind, disposition) {
	return lookupIn(g, ledger[g], ref)
}

// diskPathForRef maps raw to the disk-relative path it names, and reports false when raw names no path.
// A path-shaped raw maps to itself.
// A glyph-shaped raw maps to its unit's path only for a self glyph in a plan with a glyph alphabet.
// A member glyph names a symbol within its unit, not the unit itself.
// Checking the unit's directory would answer a different question.
// Member existence is internal/planglyph's to check.
// Every other glyph, and any plan: handle or bare symbol, reports false.
func diskPathForRef(plan *Plan, raw string) (string, bool) {
	switch classifyRef(raw) {
	case refKindPath:
		return raw, true
	case refKindGlyph:
		if _, ok := planLanguage(plan); !ok {
			return "", false
		}
		g, err := ParseRef(plan, raw)
		if err != nil || !g.IsSelf() {
			return "", false
		}
		return g.UnitPath()
	default:
		return "", false
	}
}

// refKindName returns the prose name of k that findings print.
// It lets a finding say what a ref is, not only that it is the wrong kind.
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
	// Unreachable while classifyRef returns only the kinds named above.
	// It reports rather than panics: this package is lenient at card level,
	// and a shape it cannot name must still be reported, not crash the whole validation pass.
	return "an unrecognized shape"
}
