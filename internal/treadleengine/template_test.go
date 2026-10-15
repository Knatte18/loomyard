// template_test.go pins that each shipped judge template names the verdict words and frontmatter keys this package parses,
// and that each template fills through stencil with its required markers.

package treadleengine

import (
	"reflect"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/contracts/stencils"
	"github.com/Knatte18/loomyard/internal/stencil"
)

// requireContains fails the test, naming the missing needle, if text does
// not contain it. Shared across this package's template tests.
func requireContains(t *testing.T, text, needle string) {
	t.Helper()
	if !strings.Contains(text, needle) {
		t.Errorf("output does not contain %q", needle)
	}
}

// judgeCirclingMarkerValues and judgeMilestoneMarkerValues return a values map with every one of the corresponding template's required top-level markers set to a non-empty placeholder,
// so tests can delete one key at a time to prove stencil.Fill's
// per-marker error.
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

// targetingMarkerValues returns a values map with every one of the
// targeting template's required top-level markers set to a non-empty
// placeholder, mirroring the two judge marker-value helpers above.
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

// TestShippedTemplates table-drives the three shipped judge and targeting templates through the two properties each must hold.
// Names the values Go parses: a judge template names every verdict word and frontmatter key this package parses, read from their Go definitions.
// Targeting produces no verdict and parses no key, so it carries no list.
// Fills with all markers: stencil.Fill succeeds when every required marker is supplied and fails, naming the marker, when any single one is absent.
func TestShippedTemplates(t *testing.T) {
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
