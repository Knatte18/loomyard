// reviewclass.go holds ReviewExempt, the one classification of whether a plan may skip Plan-Review.
// It reads card type labels and target paths alone, never a label the plan writer chooses for itself, so a risky card cannot talk its way out of review.

package planparser

import "path"

// sourceExtensions is the per-language set of file extensions that mark a source file, keyed by the plan's language: value.
// A language absent from the map, including "none", declares none.
var sourceExtensions = map[string][]string{
	"go": {".go"},
}

// ReviewExempt reports whether plan is exempt from Plan-Review: it has at least one card, every card's every TargetGroups entry is Prosa, and no Prosa target is a source file.
// Anything that fails to parse counts as a source file, so the function over-covers rather than under-covers.
func ReviewExempt(plan *Plan) bool {
	if len(plan.Cards) == 0 {
		return false
	}
	for _, c := range plan.Cards {
		for _, g := range c.TargetGroups {
			if g.Type != CardTypeProsa {
				return false
			}
			for _, t := range g.Refs {
				if isSourceTarget(plan, t) {
					return false
				}
			}
		}
	}
	return true
}

// isSourceTarget reports whether a Prosa target is a source file, or a whole package or directory that holds source files, or cannot be told apart from one.
func isSourceTarget(plan *Plan, target string) bool {
	lang, ok := planLanguage(plan)
	if !ok {
		return !hasFileExtension(target)
	}
	gl, err := parseGlyph(lang, target)
	if err != nil || !gl.IsSelf() {
		return true
	}
	unitPath, ok := gl.UnitPath()
	if !ok || !hasFileExtension(unitPath) {
		return true
	}
	ext := path.Ext(unitPath)
	for _, src := range sourceExtensions[effectiveLanguage(plan)] {
		if ext == src {
			return true
		}
	}
	return false
}

// effectiveLanguage returns plan.Language with the absent value read as "go".
func effectiveLanguage(plan *Plan) string {
	if plan.Language == "" {
		return "go"
	}
	return plan.Language
}
