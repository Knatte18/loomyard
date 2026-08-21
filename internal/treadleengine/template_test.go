// template_test.go pins the four shipped-default judge/triage/targeting prompt templates'
// load-bearing statements as substring assertions,
// and separately proves each template actually fills through stencil with its required markers —
// mirroring burlerengine's TestTemplate_StatesRoundDiscipline / TestTemplate_FillsWithAllMarkers
// style. It also proves runTriage composes its prompt from an on-disk edit rather than the shipped
// default, per the runtime-read-not-embed Shared Decision.

package treadleengine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/contracts/stencils"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/stencil"
)

// TestJudgeCirclingTemplate_StatesLoadBearingRules asserts the template's load-bearing phrases are
// present.
func TestJudgeCirclingTemplate_StatesLoadBearingRules(t *testing.T) {
	text := string(stencils.TreadleTemplateJudgeCircling)

	requireContains(t, text, "PROGRESSING")
	requireContains(t, text, "CIRCLING")
	requireContains(t, text, "UNCERTAIN")
	requireContains(t, text, "clear, citable evidence")
	requireContains(t, text, "when in doubt")
	requireContains(t, text, "## Themes")
	requireContains(t, text, "EXACTLY TWO")
	requireQuotedRationaleRule(t, text)
	requireHandoffMaintenanceRules(t, text)
}

// TestJudgeMilestoneTemplate_StatesLoadBearingRules is the milestone continuation gate's analogue
// of the circling-check test above.
func TestJudgeMilestoneTemplate_StatesLoadBearingRules(t *testing.T) {
	text := string(stencils.TreadleTemplateJudgeMilestone)

	requireContains(t, text, "CONTINUE")
	requireContains(t, text, "STOP")
	requireContains(t, text, "UNCERTAIN")
	requireContains(t, text, "clear evidence of a stall or circularity")
	requireContains(t, text, "when in doubt")
	requireContains(t, text, "## Themes")
	requireContains(t, text, "EXACTLY TWO")
	requireQuotedRationaleRule(t, text)
	requireHandoffMaintenanceRules(t, text)
}

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

// TestTriageTemplate_StatesLoadBearingRules is the asking-triage template's analogue: its
// vocabulary, the one-line-restate-the-blocker rule, and the single-output-file instruction.
func TestTriageTemplate_StatesLoadBearingRules(t *testing.T) {
	text := string(stencils.TreadleTemplateTriage)

	requireContains(t, text, "RETRY")
	requireContains(t, text, "GIVE_UP")
	requireContains(t, text, "restate")
	requireContains(t, text, "EXACTLY ONE")
	requireQuotedRationaleRule(t, text)
}

// TestTargetingTemplate_StatesLoadBearingRules is the pre-round targeting judge template's analogue
// of the other templates' load-bearing-statement pins: the read-the-handoff instruction, the
// exactly-one-output-file rule, and the free-form (no frontmatter) output rule — unlike every other
// template in this package, targeting produces no verdict and so has no rationale-quoting rule to
// pin.
func TestTargetingTemplate_StatesLoadBearingRules(t *testing.T) {
	text := string(stencils.TreadleTemplateTargeting)

	requireContains(t, text, "Read the previous handoff at")
	requireContains(t, text, "EXACTLY ONE")
	requireContains(t, text, "free-form prose")
	requireContains(t, text, "NO `---`-delimited YAML frontmatter")
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

// judgeCirclingMarkerValues, judgeMilestoneMarkerValues, and
// triageMarkerValues return a values map with every one of the
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
	}
}

func triageMarkerValues() map[string]string {
	return map[string]string{
		"round":        "2",
		"question":     "should I proceed without the fasit file?",
		"verdict_path": "/run/round-2-triage.md",
	}
}

// targetingMarkerValues returns a values map with every one of the
// targeting template's required top-level markers set to a non-empty
// placeholder, mirroring the three judge/triage marker-value helpers above.
func targetingMarkerValues() map[string]string {
	return map[string]string{
		"round":            "3",
		"previous_handoff": "/run/round-2-handoff.md",
		"seed_path":        "/run/round-3-seed.md",
	}
}

// TestJudgeCirclingTemplate_FillsWithAllMarkers asserts stencil.Fill succeeds when every required
// marker is supplied,
// and fails — naming the marker — when any single one is absent.
func TestJudgeCirclingTemplate_FillsWithAllMarkers(t *testing.T) {
	t.Run("all markers supplied", func(t *testing.T) {
		if _, err := stencil.Fill(stencils.TreadleTemplateJudgeCircling, judgeCirclingMarkerValues()); err != nil {
			t.Fatalf("stencil.Fill() = %v; want nil", err)
		}
	})

	for _, marker := range []string{"round", "prior_reviews", "verdict_path", "previous_handoff", "handoff_path"} {
		t.Run("missing "+marker, func(t *testing.T) {
			values := judgeCirclingMarkerValues()
			delete(values, marker)
			_, err := stencil.Fill(stencils.TreadleTemplateJudgeCircling, values)
			if err == nil {
				t.Fatalf("stencil.Fill() with %q missing = nil error; want error naming the marker", marker)
			}
			if !strings.Contains(err.Error(), marker) {
				t.Errorf("stencil.Fill() error = %q; want it to name marker %q", err.Error(), marker)
			}
		})
	}
}

// TestJudgeMilestoneTemplate_FillsWithAllMarkers is the milestone template's analogue of the
// circling-check fill test above.
func TestJudgeMilestoneTemplate_FillsWithAllMarkers(t *testing.T) {
	t.Run("all markers supplied", func(t *testing.T) {
		if _, err := stencil.Fill(stencils.TreadleTemplateJudgeMilestone, judgeMilestoneMarkerValues()); err != nil {
			t.Fatalf("stencil.Fill() = %v; want nil", err)
		}
	})

	for _, marker := range []string{"round", "hard_cap", "prior_reviews", "verdict_path", "previous_handoff", "handoff_path"} {
		t.Run("missing "+marker, func(t *testing.T) {
			values := judgeMilestoneMarkerValues()
			delete(values, marker)
			_, err := stencil.Fill(stencils.TreadleTemplateJudgeMilestone, values)
			if err == nil {
				t.Fatalf("stencil.Fill() with %q missing = nil error; want error naming the marker", marker)
			}
			if !strings.Contains(err.Error(), marker) {
				t.Errorf("stencil.Fill() error = %q; want it to name marker %q", err.Error(), marker)
			}
		})
	}
}

// TestTriageTemplate_FillsWithAllMarkers is the triage template's analogue of the two judge fill
// tests above.
func TestTriageTemplate_FillsWithAllMarkers(t *testing.T) {
	t.Run("all markers supplied", func(t *testing.T) {
		if _, err := stencil.Fill(stencils.TreadleTemplateTriage, triageMarkerValues()); err != nil {
			t.Fatalf("stencil.Fill() = %v; want nil", err)
		}
	})

	for _, marker := range []string{"round", "question", "verdict_path"} {
		t.Run("missing "+marker, func(t *testing.T) {
			values := triageMarkerValues()
			delete(values, marker)
			_, err := stencil.Fill(stencils.TreadleTemplateTriage, values)
			if err == nil {
				t.Fatalf("stencil.Fill() with %q missing = nil error; want error naming the marker", marker)
			}
			if !strings.Contains(err.Error(), marker) {
				t.Errorf("stencil.Fill() error = %q; want it to name marker %q", err.Error(), marker)
			}
		})
	}
}

// TestTargetingTemplate_FillsWithAllMarkers is the pre-round targeting template's analogue of the
// three fill tests above.
func TestTargetingTemplate_FillsWithAllMarkers(t *testing.T) {
	t.Run("all markers supplied", func(t *testing.T) {
		if _, err := stencil.Fill(stencils.TreadleTemplateTargeting, targetingMarkerValues()); err != nil {
			t.Fatalf("stencil.Fill() = %v; want nil", err)
		}
	})

	for _, marker := range []string{"round", "previous_handoff", "seed_path"} {
		t.Run("missing "+marker, func(t *testing.T) {
			values := targetingMarkerValues()
			delete(values, marker)
			_, err := stencil.Fill(stencils.TreadleTemplateTargeting, values)
			if err == nil {
				t.Fatalf("stencil.Fill() with %q missing = nil error; want error naming the marker", marker)
			}
			if !strings.Contains(err.Error(), marker) {
				t.Errorf("stencil.Fill() error = %q; want it to name marker %q", err.Error(), marker)
			}
		})
	}
}

// TestRunTriage_ReadsFromDiskAtCallTime proves runTriage composes its prompt from the on-disk
// triage stencil at call time, not the embedded shipped default — the runtime-read-not-embed Shared
// Decision's whole point. It overwrites the seeded stencils directory's triage file with a modified
// body (markers intact) and asserts the shuttle spec's Prompt carries the on-disk sentinel text.
func TestRunTriage_ReadsFromDiskAtCallTime(t *testing.T) {
	dir := newTestStencilsDir(t)
	sentinel := "SENTINEL-ON-DISK-EDIT-NOT-THE-SHIPPED-DEFAULT"
	modified := strings.Replace(string(stencils.TreadleTemplateTriage), "# Treadle asking-triage", "# Treadle asking-triage — "+sentinel, 1)
	triagePath := filepath.Join(dir, "treadle", "treadle-template-triage.md")
	if err := os.WriteFile(triagePath, []byte(modified), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) = %v; want nil", triagePath, err)
	}

	verdictContent := `---
verdict: RETRY
rationale: "fine"
---
`
	sh := &fakeJudgeShuttle{
		verdictContent: verdictContent,
		result:         shuttleengine.Result{Outcome: shuttleengine.OutcomeDone},
	}

	runTriage(dir, sh, "gate", 1, "a question", filepath.Join(t.TempDir(), "v.md"), "haiku", "low")

	if !sh.called {
		t.Fatal("runTriage() never called the shuttle")
	}
	if !strings.Contains(sh.spec.Prompt, sentinel) {
		t.Error("runTriage() prompt does not contain the on-disk edit; it composed from the embedded shipped default instead")
	}
	if strings.Contains(string(stencils.TreadleTemplateTriage), sentinel) {
		t.Fatal("the embedded shipped default unexpectedly already contains the sentinel; the test setup is broken")
	}
}
