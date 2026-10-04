// bouncerprompt_test.go covers focusSchemaMarkers' two variants against both shipped Bouncer
// stencils, and that BouncerConfig.ClusterExcludes reaches the seed and judge fills.

package shedadapters

import (
	"fmt"
	"maps"
	"os"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/contracts/stencils"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/stencil"
	"github.com/Knatte18/loomyard/internal/testkit/shedfake"
)

const (
	excludeLensesExampleLine = "exclude_lenses: []"
	excludeLensesRule        = "`exclude_lenses` is a list of strings, possibly empty."
)

// assertExcludeLensesText fails t unless prompt carries the exclude_lenses example line and rule
// exactly when clusterExcludes is true, and no exclude_lenses substring at all when it is false.
func assertExcludeLensesText(t *testing.T, prompt string, clusterExcludes bool) {
	t.Helper()

	if !clusterExcludes {
		if strings.Contains(prompt, "exclude_lenses") {
			t.Error("key-off prompt contains an exclude_lenses substring; want none")
		}
		return
	}
	if !strings.Contains(prompt, excludeLensesExampleLine) {
		t.Errorf("key-on prompt lacks the %q example line", excludeLensesExampleLine)
	}
	if !strings.Contains(prompt, excludeLensesRule) {
		t.Errorf("key-on prompt lacks the %q rule", excludeLensesRule)
	}
}

func TestFocusSchemaMarkers_BothStencilsBothModes(t *testing.T) {
	seedBase := map[string]string{
		"rubric":     "# Rubric\n\nBe thorough.\n",
		"artifacts":  "/abs/artifact.md",
		"round":      "1",
		"focus_path": "/abs/round-1-focus.md",
	}
	judgeBase := map[string]string{
		"rubric":          "# Rubric\n\nBe thorough.\n",
		"facts_path":      "/abs/round-1-facts.md",
		"round":           "1",
		"next_round":      "2",
		"report_path":     "/abs/round-1-report.md",
		"previous_ledger": "(none)",
		"verdict_path":    "/abs/round-1-bouncer-verdict.md",
		"ledger_path":     "/abs/round-1-bouncer-ledger.md",
		"focus_path":      "/abs/round-2-focus.md",
	}

	for _, clusterExcludes := range []bool{false, true} {
		for name, tc := range map[string]struct {
			template []byte
			base     map[string]string
		}{
			"Seed":  {stencils.BouncerTemplateSeed, seedBase},
			"Judge": {stencils.BouncerTemplateJudge, judgeBase},
		} {
			t.Run(fmt.Sprintf("%s_ClusterExcludes=%t", name, clusterExcludes), func(t *testing.T) {
				values := maps.Clone(tc.base)
				maps.Copy(values, focusSchemaMarkers(clusterExcludes))
				prompt, err := stencil.FillOptional(tc.template, values, []string{"pattern_directive"})
				if err != nil {
					t.Fatalf("stencil.Fill(%s, clusterExcludes=%v) error = %v; want nil", name, clusterExcludes, err)
				}
				assertExcludeLensesText(t, string(prompt), clusterExcludes)
			})
		}
	}
}

func TestBouncer_ClusterExcludesReachesSeedAndJudgePrompts(t *testing.T) {
	for _, clusterExcludes := range []bool{false, true} {
		t.Run(fmt.Sprintf("Seed_ClusterExcludes=%t", clusterExcludes), func(t *testing.T) {
			shuttle := &shedfake.Shuttle{Result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}}
			cfg := newBouncerFixture(t).Config
			cfg.Shuttle = shuttle
			cfg.ClusterExcludes = clusterExcludes
			b, err := NewBouncer(cfg)
			if err != nil {
				t.Fatalf("NewBouncer(...) error = %v; want nil", err)
			}

			shedfake.CallOK(t, b)
			if !shuttle.Called {
				t.Fatal("seed Call() did not invoke the shuttle seam")
			}
			assertExcludeLensesText(t, shuttle.GotSpec.Prompt, clusterExcludes)
		})

		t.Run(fmt.Sprintf("Judge_ClusterExcludes=%t", clusterExcludes), func(t *testing.T) {
			shuttle := &shedfake.Shuttle{Result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}}
			cfg := newBouncerFixture(t).Config
			cfg.Shuttle = shuttle
			cfg.ClusterExcludes = clusterExcludes
			b, err := NewBouncer(cfg)
			if err != nil {
				t.Fatalf("NewBouncer(...) error = %v; want nil", err)
			}
			report := cfg.RunDir + "/" + cfg.ReportName(1)
			if err := os.WriteFile(report, []byte("# Round 1 report\n\nA finding.\n"), 0o644); err != nil {
				t.Fatalf("WriteFile(%q) = %v; want nil", report, err)
			}

			shedfake.CallOK(t, b)
			if !shuttle.Called {
				t.Fatal("judge Call() did not invoke the shuttle seam")
			}
			if shuttle.GotSpec.Role != bouncerJudgeRole {
				t.Fatalf("recorded spec.Role = %q; want %q", shuttle.GotSpec.Role, bouncerJudgeRole)
			}
			assertExcludeLensesText(t, shuttle.GotSpec.Prompt, clusterExcludes)
		})
	}
}
