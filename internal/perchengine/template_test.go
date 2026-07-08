// template_test.go pins the three embedded judge/triage prompt templates'
// load-bearing statements as substring assertions, and separately proves
// each template actually fills through stencil with its required markers —
// mirroring burlerengine's TestTemplate_StatesRoundDiscipline /
// TestTemplate_FillsWithAllMarkers style.

package perchengine

import (
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/stencil"
)

// TestJudgeCirclingTemplate_StatesLoadBearingRules asserts the circling-check
// template's bytes carry its load-bearing phrases in prose, so an edit that
// silently waters down the vocabulary, the clear-evidence requirement, the
// fail-safe direction, the Themes section, or the single-output-file
// instruction fails this test rather than only a human review.
func TestJudgeCirclingTemplate_StatesLoadBearingRules(t *testing.T) {
	text := string(judgeCirclingTemplate)

	requireContains(t, text, "PROGRESSING")
	requireContains(t, text, "CIRCLING")
	requireContains(t, text, "UNCERTAIN")
	requireContains(t, text, "clear, citable evidence")
	requireContains(t, text, "when in doubt")
	requireContains(t, text, "## Themes")
	requireContains(t, text, "EXACTLY ONE")
}

// TestJudgeMilestoneTemplate_StatesLoadBearingRules is the milestone
// continuation gate's analogue of the circling-check test above.
func TestJudgeMilestoneTemplate_StatesLoadBearingRules(t *testing.T) {
	text := string(judgeMilestoneTemplate)

	requireContains(t, text, "CONTINUE")
	requireContains(t, text, "STOP")
	requireContains(t, text, "UNCERTAIN")
	requireContains(t, text, "clear evidence of a stall or circularity")
	requireContains(t, text, "when in doubt")
	requireContains(t, text, "## Themes")
	requireContains(t, text, "EXACTLY ONE")
}

// TestTriageTemplate_StatesLoadBearingRules is the asking-triage template's
// analogue: its vocabulary, the one-line-restate-the-blocker rule, and the
// single-output-file instruction.
func TestTriageTemplate_StatesLoadBearingRules(t *testing.T) {
	text := string(triageTemplate)

	requireContains(t, text, "RETRY")
	requireContains(t, text, "GIVE_UP")
	requireContains(t, text, "restate")
	requireContains(t, text, "EXACTLY ONE")
}

// requireContains fails the test, naming the missing needle, if text does
// not contain it. Shared across this package's template tests.
func requireContains(t *testing.T, text, needle string) {
	t.Helper()
	if !strings.Contains(text, needle) {
		t.Errorf("output does not contain %q", needle)
	}
}

// judgeCirclingMarkerValues, judgeMilestoneMarkerValues, and
// triageMarkerValues return a values map with every one of the
// corresponding template's required top-level markers set to a non-empty
// placeholder, so tests can delete one key at a time to prove stencil.Fill's
// per-marker error.
func judgeCirclingMarkerValues() map[string]string {
	return map[string]string{
		"round":         "3",
		"prior_reviews": "/run/round-1-review.md\n/run/round-2-review.md",
		"verdict_path":  "/run/round-3-judge.md",
	}
}

func judgeMilestoneMarkerValues() map[string]string {
	return map[string]string{
		"round":         "5",
		"hard_cap":      "10",
		"prior_reviews": "/run/round-1-review.md\n/run/round-2-review.md",
		"verdict_path":  "/run/round-5-judge.md",
	}
}

func triageMarkerValues() map[string]string {
	return map[string]string{
		"round":        "2",
		"question":     "should I proceed without the fasit file?",
		"verdict_path": "/run/round-2-triage.md",
	}
}

// TestJudgeCirclingTemplate_FillsWithAllMarkers asserts stencil.Fill succeeds
// when every required marker is supplied, and fails — naming the marker —
// when any single one is absent.
func TestJudgeCirclingTemplate_FillsWithAllMarkers(t *testing.T) {
	t.Run("all markers supplied", func(t *testing.T) {
		if _, err := stencil.Fill(judgeCirclingTemplate, judgeCirclingMarkerValues()); err != nil {
			t.Fatalf("stencil.Fill() = %v; want nil", err)
		}
	})

	for _, marker := range []string{"round", "prior_reviews", "verdict_path"} {
		t.Run("missing "+marker, func(t *testing.T) {
			values := judgeCirclingMarkerValues()
			delete(values, marker)
			_, err := stencil.Fill(judgeCirclingTemplate, values)
			if err == nil {
				t.Fatalf("stencil.Fill() with %q missing = nil error; want error naming the marker", marker)
			}
			if !strings.Contains(err.Error(), marker) {
				t.Errorf("stencil.Fill() error = %q; want it to name marker %q", err.Error(), marker)
			}
		})
	}
}

// TestJudgeMilestoneTemplate_FillsWithAllMarkers is the milestone template's
// analogue of the circling-check fill test above.
func TestJudgeMilestoneTemplate_FillsWithAllMarkers(t *testing.T) {
	t.Run("all markers supplied", func(t *testing.T) {
		if _, err := stencil.Fill(judgeMilestoneTemplate, judgeMilestoneMarkerValues()); err != nil {
			t.Fatalf("stencil.Fill() = %v; want nil", err)
		}
	})

	for _, marker := range []string{"round", "hard_cap", "prior_reviews", "verdict_path"} {
		t.Run("missing "+marker, func(t *testing.T) {
			values := judgeMilestoneMarkerValues()
			delete(values, marker)
			_, err := stencil.Fill(judgeMilestoneTemplate, values)
			if err == nil {
				t.Fatalf("stencil.Fill() with %q missing = nil error; want error naming the marker", marker)
			}
			if !strings.Contains(err.Error(), marker) {
				t.Errorf("stencil.Fill() error = %q; want it to name marker %q", err.Error(), marker)
			}
		})
	}
}

// TestTriageTemplate_FillsWithAllMarkers is the triage template's analogue of
// the two judge fill tests above.
func TestTriageTemplate_FillsWithAllMarkers(t *testing.T) {
	t.Run("all markers supplied", func(t *testing.T) {
		if _, err := stencil.Fill(triageTemplate, triageMarkerValues()); err != nil {
			t.Fatalf("stencil.Fill() = %v; want nil", err)
		}
	})

	for _, marker := range []string{"round", "question", "verdict_path"} {
		t.Run("missing "+marker, func(t *testing.T) {
			values := triageMarkerValues()
			delete(values, marker)
			_, err := stencil.Fill(triageTemplate, values)
			if err == nil {
				t.Fatalf("stencil.Fill() with %q missing = nil error; want error naming the marker", marker)
			}
			if !strings.Contains(err.Error(), marker) {
				t.Errorf("stencil.Fill() error = %q; want it to name marker %q", err.Error(), marker)
			}
		})
	}
}
