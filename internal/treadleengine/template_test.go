// template_test.go pins the three shipped-default judge/targeting prompt templates'
// load-bearing statements as substring assertions,
// and separately proves each template actually fills through stencil with its required markers —
// mirroring burlerengine's TestTemplate_StatesRoundDiscipline / TestTemplate_FillsWithAllMarkers
// style.

package treadleengine

import (
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/contracts/stencils"
	"github.com/Knatte18/loomyard/internal/stencil"
)

// requireHandoffMaintenanceRules asserts a judge template (circling or
// milestone) carries its three BLOCKING handoff-maintenance rules in prose:
// (a) the lossless carry-forward rule for the ledger, (b) the covers_rounds
// computation (previous handoff's own coverage plus every round read this
// call), and (d) the two-output-files rule (the verdict AND handoff files,
// written by the SAME call) — so an edit that silently weakens any of them
// fails this test rather than only a human review.
func requireHandoffMaintenanceRules(t *testing.T, text string) {
	t.Helper()
	requireContains(t, text, "previous_handoff")
	requireContains(t, text, "(none)")
	// (a) lossless carry-forward.
	requireContains(t, text, "lossless carry-forward rule")
	requireContains(t, text, "MUST reappear in this handoff's ledger")
	requireContains(t, text, "NEVER silently dropped")
	// (b) covers_rounds computation.
	requireContains(t, text, "covers_rounds")
	requireContains(t, text, "the previous handoff's own `covers_rounds`")
	requireContains(t, text, "PLUS the round number of every review file you actually read this call")
	// (d) exactly two output files, same call.
	requireContains(t, text, "Write EXACTLY TWO files this call")
	requireContains(t, text, "{{.handoff_path}}")
}

// requireQuotedRationaleRule asserts a template both SHOWS a double-quoted
// rationale in its example frontmatter and STATES the quoting rule in prose.
// This is load-bearing against a live-observed failure: a real judge writing
// an unquoted rationale containing ": " produces invalid YAML, the strict
// parser rejects the file, and the genuine verdict is discarded by the
// fail-safe — so the templates must actively steer agents to quote it.
func requireQuotedRationaleRule(t *testing.T, text string) {
	t.Helper()
	requireContains(t, text, `rationale: "`)
	requireContains(t, text, "double-quoted, single-line YAML string")
}

// requireContains fails the test, naming the missing needle, if text does
// not contain it. Shared across this package's template tests.
func requireContains(t *testing.T, text, needle string) {
	t.Helper()
	if !strings.Contains(text, needle) {
		t.Errorf("output does not contain %q", needle)
	}
}

// judgeCirclingMarkerValues and judgeMilestoneMarkerValues
// return a values map with every one of the
// corresponding template's required top-level markers set to a non-empty
// placeholder, so tests can delete one key at a time to prove stencil.Fill's
// per-marker error.
func judgeCirclingMarkerValues() map[string]string {
	return map[string]string{
		"round":            "3",
		"prior_reviews":    "/run/round-2-review.md",
		"verdict_path":     "/run/round-3-judge.md",
		"previous_handoff": "/run/round-1-handoff.md",
		"handoff_path":     "/run/round-3-handoff.md",
		"parent_directive": "the parent directive",
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
	}
}

// TestShippedTemplates table-drives the three shipped judge and targeting templates through the two properties each must hold.
// States load-bearing rules: the template carries its load-bearing phrases, so an edit that silently weakens one fails here rather than only in human review.
// The judge templates (circling and milestone) also carry the quoted-rationale rule and the handoff-maintenance rules; targeting produces no verdict, so it has no rationale-quoting rule to pin.
// Fills with all markers: stencil.Fill succeeds when every required marker is supplied and fails, naming the marker, when any single one is absent.
func TestShippedTemplates(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		template        []byte
		phrases         []string
		quotedRationale bool
		handoffRules    bool
		values          func() map[string]string
		requiredMarkers []string
	}{
		{
			name:     "judge circling",
			template: stencils.TreadleTemplateJudgeCircling,
			phrases: []string{
				"PROGRESSING", "CIRCLING", "UNCERTAIN", "clear, citable evidence", "when in doubt", "## Themes", "EXACTLY TWO",
			},
			quotedRationale: true,
			handoffRules:    true,
			values:          judgeCirclingMarkerValues,
			requiredMarkers: []string{"round", "prior_reviews", "verdict_path", "previous_handoff", "handoff_path", "parent_directive"},
		},
		{
			name:     "judge milestone",
			template: stencils.TreadleTemplateJudgeMilestone,
			phrases: []string{
				"CONTINUE", "STOP", "UNCERTAIN", "clear evidence of a stall or circularity", "when in doubt", "## Themes", "EXACTLY TWO",
			},
			quotedRationale: true,
			handoffRules:    true,
			values:          judgeMilestoneMarkerValues,
			requiredMarkers: []string{"round", "hard_cap", "prior_reviews", "verdict_path", "previous_handoff", "handoff_path", "parent_directive"},
		},
		{
			// The pre-round targeting template's read-the-handoff instruction, exactly-one-output-file rule and free-form (no frontmatter) output rule.
			name:            "targeting",
			template:        stencils.TreadleTemplateTargeting,
			phrases:         []string{"Read the previous handoff at", "EXACTLY ONE", "free-form prose", "NO `---`-delimited YAML frontmatter"},
			values:          targetingMarkerValues,
			requiredMarkers: []string{"round", "previous_handoff", "seed_path", "parent_directive"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			t.Run("states load-bearing rules", func(t *testing.T) {
				t.Parallel()
				text := string(tt.template)
				for _, phrase := range tt.phrases {
					requireContains(t, text, phrase)
				}
				if tt.quotedRationale {
					requireQuotedRationaleRule(t, text)
				}
				if tt.handoffRules {
					requireHandoffMaintenanceRules(t, text)
				}
			})

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
