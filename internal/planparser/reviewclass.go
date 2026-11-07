// reviewclass.go holds ReviewExempt, which decides if a plan may skip Plan-Review, and the source and test classifiers.
// It reads card type labels and target paths alone, never a label the plan writer chooses for itself,
// so a risky card cannot talk its way out of review.

package planparser

import (
	"path"
	"strings"
)

// sourceExtensions maps each plan language to the file extensions that mark a source file.
// A language absent from the map, including "none", declares none.
var sourceExtensions = map[string][]string{
	"go":     {".go"},
	"csharp": {".cs"},
}

// ReviewExempt reports whether plan is exempt from Plan-Review:
// it has at least one card, every card's every TargetGroups entry is Prosa, and no Prosa target is a source file.
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

// isSourceTarget reports whether a Prosa target is a source file.
// It also reports true for a whole package or directory, and for a target it cannot tell apart from a source file.
func isSourceTarget(plan *Plan, target string) bool {
	if _, ok := planLanguage(plan); !ok {
		return !hasFileExtension(target)
	}
	gl, err := ParseRef(plan, target)
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

// PlanLanguage returns plan's language, with an absent key read as "go".
func PlanLanguage(plan *Plan) string {
	return effectiveLanguage(plan)
}

// testFileMatchers maps each language to its test-file predicate over a slash-separated, repository-relative path.
// A language absent from the map, including "none", has no matcher.
// IsTestFile reads every path of such a language as a test file.
var testFileMatchers = map[string]func(path string) bool{
	"go": func(p string) bool {
		return strings.HasSuffix(p, "_test.go")
	},
	"csharp": func(p string) bool {
		if path.Ext(p) != ".cs" {
			return false
		}
		if strings.HasSuffix(p, "Tests.cs") || strings.HasSuffix(p, "Test.cs") {
			return true
		}
		for _, dir := range strings.Split(path.Dir(p), "/") {
			if strings.HasSuffix(dir, "Tests") {
				return true
			}
		}
		return false
	},
}

// IsTestFile reports whether p, a slash-separated repository-relative path, is a test file of language.
// A language with no declared matcher, "none" included, answers true for every path,
// so a cap built on it over-covers rather than lets a test edit through.
func IsTestFile(language, p string) bool {
	match, ok := testFileMatchers[language]
	if !ok {
		return true
	}
	return match(p)
}

// IsAnyLanguageTestFile reports whether p is a test file of any language with a declared matcher.
func IsAnyLanguageTestFile(p string) bool {
	for _, match := range testFileMatchers {
		if match(p) {
			return true
		}
	}
	return false
}
