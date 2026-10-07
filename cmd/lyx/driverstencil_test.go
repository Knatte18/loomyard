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

// TestDriverStencil_ShowsOneBareStepCall pins the invocation shape: a fenced block whose only line is the bare step call, no cd-subshell form for `lyx` calls, and the two envelope keys a driver reads named.
//
//testtiming:keep pins the stencil's bare step-call shape and the two envelope keys it names, which the recipe-blindness scan never reads
func TestDriverStencil_ShowsOneBareStepCall(t *testing.T) {
	t.Parallel()

	stencil := string(stencils.ShedTemplateDriver)
	if !strings.Contains(stencil, "```\nlyx shed step {{.run_id}}\n```") {
		t.Errorf("shed-template-driver.md has no fenced block whose only line is `lyx shed step {{.run_id}}`")
	}
	if strings.Contains(stencil, "(cd <drive-dir> && lyx") {
		t.Errorf("shed-template-driver.md still carries the `(cd <drive-dir> && lyx` subshell form")
	}
	for _, key := range []string{"trace_file", "envelope_path"} {
		if !strings.Contains(stencil, "`"+key+"`") {
			t.Errorf("shed-template-driver.md does not name `%s`", key)
		}
	}
}

// TestDriverStencil_IsRecipeBlind scans the driver stencil and the two parent-notification stencils filled into it, and fails for every shipped recipe name appearing as a whole word
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

	for _, file := range []struct {
		name string
		body []byte
	}{
		{"shed-template-driver.md", stencils.ShedTemplateDriver},
		{"shed-template-driver-notify.md", stencils.ShedTemplateDriverNotify},
		{"shed-template-driver-notify-watched.md", stencils.ShedTemplateDriverNotifyWatched},
	} {
		for index, line := range strings.Split(string(file.body), "\n") {
			for _, token := range tokens {
				if token.matches(line) {
					t.Errorf("%s:%d names %s; the stencil must stay recipe-blind: %q", file.name, index+1, token.label, line)
				}
			}
		}
	}
}
