// template_test.go checks the shipped judge templates.
// Each one names the verdict words and frontmatter keys this package parses.
// Each one also fills through stencil with its required markers.

package treadleengine

import (
	"reflect"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/contracts/stencils"
	"github.com/Knatte18/loomyard/internal/stencil"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// requireContains fails the test, naming the missing needle, if text does not contain it.
// It is shared across this package's template tests.
func requireContains(t *testing.T, text, needle string) {
	t.Helper()
	if !strings.Contains(text, needle) {
		t.Errorf("output does not contain %q", needle)
	}
}

// judgeCirclingMarkerValues and judgeMilestoneMarkerValues return a values map for their judge template.
// Every required top-level marker is set to a non-empty placeholder.
// A test drops one key at a time to check that filling fails and names that marker.
func judgeCirclingMarkerValues() map[string]string {
	return map[string]string{
		"round":            "3",
		"prior_reviews":    "/run/round-2-review.md",
		"verdict_path":     "/run/round-3-judge.md",
		"previous_handoff": "/run/round-1-handoff.md",
		"handoff_path":     "/run/round-3-handoff.md",
		"parent_directive": "the parent directive",
		"edit_directive":   "the edit directive",
	}
}

func judgeMilestoneMarkerValues() map[string]string {
	return map[string]string{
		"round":            "5",
		"hard_cap":         "10",
		"prior_reviews":    "/run/round-4-review.md",
		"verdict_path":     "/run/round-5-judge.md",
		"previous_handoff": "/run/round-3-handoff.md",
		"handoff_path":     "/run/round-5-handoff.md",
		"parent_directive": "the parent directive",
		"edit_directive":   "the edit directive",
	}
}

// targetingMarkerValues returns a values map that sets every required top-level marker of the targeting template.
// Each marker gets a non-empty placeholder.
func targetingMarkerValues() map[string]string {
	return map[string]string{
		"round":            "3",
		"previous_handoff": "/run/round-2-handoff.md",
		"seed_path":        "/run/round-3-seed.md",
		"parent_directive": "the parent directive",
		"edit_directive":   "the edit directive",
	}
}

// yamlTagOf returns the yaml tag of the named field of the struct type of v.
func yamlTagOf(t *testing.T, v any, field string) string {
	t.Helper()
	f, ok := reflect.TypeOf(v).FieldByName(field)
	if !ok {
		t.Fatalf("%T has no field %q", v, field)
	}
	return f.Tag.Get("yaml")
}

// TestShippedTemplates table-drives the shipped judge and targeting templates through the two properties each must hold.
// A judge template names every verdict word and frontmatter key the package parses, read from their Go definitions.
// The targeting template produces no verdict and parses no key, so it carries no list.
// Filling succeeds when every required marker is supplied,
// and it fails, naming the marker, when any one is absent.
func TestShippedTemplates(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	frontmatterKeys := []string{
		yamlTagOf(t, judgeHeader{}, "Rationale"),
		yamlTagOf(t, handoffHeader{}, "CoversRounds"),
		yamlTagOf(t, handoffHeader{}, "Ledger"),
	}

	tests := []struct {
		name            string
		template        []byte
		parsedValues    []string
		values          func() map[string]string
		requiredMarkers []string
	}{
		{
			name:     "judge circling",
			template: stencils.TreadleTemplateJudgeCircling,
			parsedValues: append([]string{
				string(JudgeProgressing), string(JudgeCircling), string(JudgeUncertain),
			}, frontmatterKeys...),
			values:          judgeCirclingMarkerValues,
			requiredMarkers: []string{"round", "prior_reviews", "verdict_path", "previous_handoff", "handoff_path", "parent_directive"},
		},
		{
			name:     "judge milestone",
			template: stencils.TreadleTemplateJudgeMilestone,
			parsedValues: append([]string{
				string(JudgeContinue), string(JudgeStop), string(JudgeUncertain),
			}, frontmatterKeys...),
			values:          judgeMilestoneMarkerValues,
			requiredMarkers: []string{"round", "hard_cap", "prior_reviews", "verdict_path", "previous_handoff", "handoff_path", "parent_directive"},
		},
		{
			name:            "targeting",
			template:        stencils.TreadleTemplateTargeting,
			values:          targetingMarkerValues,
			requiredMarkers: []string{"round", "previous_handoff", "seed_path", "parent_directive"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if len(tt.parsedValues) > 0 {
				t.Run("names the values Go parses", func(t *testing.T) {
					t.Parallel()
					text := string(tt.template)
					for _, value := range tt.parsedValues {
						requireContains(t, text, value)
					}
				})
			}

			t.Run("fills with all markers", func(t *testing.T) {
				t.Parallel()
				if _, err := stencil.Fill(tt.template, tt.values()); err != nil {
					t.Fatalf("stencil.Fill() = %v; want nil", err)
				}
				for _, marker := range tt.requiredMarkers {
					values := tt.values()
					delete(values, marker)
					_, err := stencil.Fill(tt.template, values)
					if err == nil {
						t.Fatalf("stencil.Fill() with %q missing = nil error; want error naming the marker", marker)
					}
					if !strings.Contains(err.Error(), marker) {
						t.Errorf("stencil.Fill() error = %q; want it to name marker %q", err.Error(), marker)
					}
				}
			})
		})
	}
}
