// driverstencil_test.go pins the shed driver stencil's recipe-blindness. It replaces the retired step-cap
// pin: the design's claim is that which recipe runs is a property of the seed alone, so the stencil
// must name no shipped recipe, no `.lyx/` path and no recipe-owned command.

package main

import (
	"regexp"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/contracts/stencils"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/shedrun"
)

// TestDriverStencil_IsRecipeBlind fails for every shipped recipe name appearing as a whole word
// (case-insensitively, so "loomyard" does not match) and for every forbidden literal, naming the
// offending token and its line number.
func TestDriverStencil_IsRecipeBlind(t *testing.T) {
	t.Parallel()

	type forbiddenToken struct {
		label   string
		matches func(line string) bool
	}
	var tokens []forbiddenToken
	for _, name := range shedrun.RecipeNames() {
		pattern := regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(name) + `\b`)
		tokens = append(tokens, forbiddenToken{label: "recipe name " + name, matches: pattern.MatchString})
	}
	// The stencil carries no phase knowledge: no loom row name appears as a whole word, and a hyphenated neighbour does not match.
	for row := range loomshed.InterruptPolicies {
		pattern := regexp.MustCompile(`(^|[^A-Za-z-])` + regexp.QuoteMeta(row) + `($|[^A-Za-z-])`)
		tokens = append(tokens, forbiddenToken{label: "loom row " + row, matches: pattern.MatchString})
	}
	for _, literal := range []string{".lyx/", "lyx loom", "lyx batten"} {
		tokens = append(tokens, forbiddenToken{
			label:   "literal " + literal,
			matches: func(line string) bool { return strings.Contains(line, literal) },
		})
	}

	for index, line := range strings.Split(string(stencils.ShedTemplateDriver), "\n") {
		for _, token := range tokens {
			if token.matches(line) {
				t.Errorf("shed-template-driver.md:%d names %s; the stencil must stay recipe-blind: %q", index+1, token.label, line)
			}
		}
	}
}
