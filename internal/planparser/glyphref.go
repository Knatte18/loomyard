// glyphref.go holds the glyph-side helpers classify.go and the rest of the package delegate to, so classify.go itself stays a shape-only file.
// Per the glyph-conversion-chokepoint Shared Decision, ParseRef is the one alphabet choice for a plan ref and delegates to glyph.Parse with no local grammar of its own:
// no strings.TrimSuffix(raw, "#"), no reading Glyph.Unit as a disk path, and no regex over a glyph string anywhere in this package.

package planparser

import (
	"fmt"

	"github.com/Knatte18/quarry/glyph"
)

// ParseRef parses a `#`-shaped plan ref, or a `plan:` handle's body the caller already stripped, against the alphabet plan's language selects.
// Under a language with no glyph alphabet, such as none, it returns an error and parses nothing.
// Under go it is the Go alphabet's parse.
// Under csharp a ref the C# alphabet accepts is a C# glyph, and a ref only the Go alphabet accepts is returned only when it is a self glyph, since quarry spells a path in the Go unit alphabet;
// any other ref returns the C# alphabet's parse error.
func ParseRef(plan *Plan, ref string) (glyph.Glyph, error) {
	lang, ok := planLanguage(plan)
	if !ok {
		return glyph.Glyph{}, fmt.Errorf("plan language %q has no glyph alphabet to parse %q against", plan.Language, ref)
	}
	parsed, err := glyph.Parse(lang, ref)
	if err == nil || lang != glyph.CSharp {
		return parsed, err
	}
	if goGlyph, goErr := glyph.Parse(glyph.Go, ref); goErr == nil && goGlyph.IsSelf() {
		return goGlyph, nil
	}
	return glyph.Glyph{}, err
}

// planLanguage maps plan.Language to the glyph.Language the glyph package's alphabet uses.
// "go" maps to glyph.Go and "csharp" to glyph.CSharp;
// "" (the Go zero value, meaning the language: key was absent from a plan built outside ParsePlan) is treated identically to "go", matching Plan.Language's own documented "absent defaults to go" rule.
// "none" and any other, unrecognized value (already reported by checkLanguageRecognized) report the not-ok second return:
// no glyph-side check may run against a plan that has opted out of the alphabet, or whose language: value is not one this package understands.
func planLanguage(plan *Plan) (glyph.Language, bool) {
	switch plan.Language {
	case "", "go":
		return glyph.Go, true
	case "csharp":
		return glyph.CSharp, true
	default:
		return glyph.Language(""), false
	}
}

// SymbolRenamePairs returns, in card order and pair order, every pair of every cards entry's Pairs that is a symbol pair: every pair but a file pair.
// Under a language planLanguage accepts, a file pair is one isFileRenamePair reports;
// under a language it rejects, such as none, a file pair is one whose two sides both classify as file paths.
// It returns nil when no card holds a symbol pair and never reads the disk.
// plan supplies only the language, so a caller passes the cards it checks: the whole plan's for the plan gate, a batch's for webster.
func SymbolRenamePairs(plan *Plan, cards []Card) []MovePair {
	_, typed := planLanguage(plan)
	var symbolPairs []MovePair
	for _, card := range cards {
		for _, pair := range card.Pairs {
			if typed && isFileRenamePair(plan, pair) {
				continue
			}
			if !typed && isUntypedFileRenamePair(pair) {
				continue
			}
			symbolPairs = append(symbolPairs, pair)
		}
	}
	return symbolPairs
}

// isUntypedFileRenamePair reports whether both sides of p classify as file paths, the only file-pair test a plan with no glyph alphabet has.
func isUntypedFileRenamePair(p MovePair) bool {
	if _, disp := lookup(gateUntypedFileRenamePair, p.Old); disp != dispKeep {
		return false
	}
	_, disp := lookup(gateUntypedFileRenamePair, p.New)
	return disp == dispKeep
}

// GlyphLanguage reports whether the plan's language selects a glyph alphabet, so glyph-side checks run.
// It is false under none and under an unrecognized language.
// The alphabet itself is never returned: a caller parses a ref through ParseRef.
func (p *Plan) GlyphLanguage() bool {
	_, ok := planLanguage(p)
	return ok
}
