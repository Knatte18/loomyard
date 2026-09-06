// glyphref.go holds the glyph-side helpers classify.go and the rest of the package delegate to,
// so classify.go itself stays a shape-only file. Per the glyph-conversion-chokepoint Shared
// Decision, parseGlyph delegates to glyph.Parse with no local grammar of its own: no
// strings.TrimSuffix(raw, "#"), no reading Glyph.Unit as a disk path, and no regex over a glyph
// string anywhere in this package.

package planparser

import "github.com/Knatte18/quarry/glyph"

// parseGlyph parses raw against lang's alphabet, delegating entirely to glyph.Parse.
func parseGlyph(lang glyph.Language, raw string) (glyph.Glyph, error) {
	return glyph.Parse(lang, raw)
}

// planLanguage maps plan.Language to the glyph.Language the glyph package's alphabet uses. "go"
// maps to glyph.Go; "" (the Go zero value, meaning the language: key was absent from a plan built
// outside ParsePlan) is treated identically to "go", matching Plan.Language's own documented
// "absent defaults to go" rule. "none" and any other, unrecognized value (already reported by
// checkLanguageRecognized) report the not-ok second return: no glyph-side check may run against a
// plan that has opted out of the alphabet, or whose language: value is not one this package
// understands.
func planLanguage(plan *Plan) (glyph.Language, bool) {
	switch plan.Language {
	case "", "go":
		return glyph.Go, true
	default:
		return glyph.Language(""), false
	}
}
