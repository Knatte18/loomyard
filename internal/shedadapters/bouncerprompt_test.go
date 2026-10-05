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
	"github.com/Knatte18/loomyard/internal/parentdirective"
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

		parentdirective.MarkerName: "PARENT DIRECTIVE",
	}
	judgeBase := map[string]string{
		"rubric":          "# Rubric\n\nBe thorough.\n",
		"facts_path":      "/abs/round-1-facts.md",
		"round":           "1",
		"next_round":      "2",
		"decision_rule":   decisionRuleMarker(1, 3),
		"report_path":     "/abs/round-1-report.md",
		"previous_ledger": "(none)",
		"verdict_path":    "/abs/round-1-bouncer-verdict.md",
		"ledger_path":     "/abs/round-1-bouncer-ledger.md",
		"focus_path":      "/abs/round-2-focus.md",

		parentdirective.MarkerName: "PARENT DIRECTIVE",
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

// decisionRuleSection returns the part of a rendered judge prompt between its Decision rule and Output files headings.
func decisionRuleSection(t *testing.T, prompt string) string {
	t.Helper()

	_, rest, ok := strings.Cut(prompt, "## Decision rule")
	if !ok {
		t.Fatal("judge prompt has no Decision rule heading")
	}
	section, _, ok := strings.Cut(rest, "## Output files")
	if !ok {
		t.Fatal("judge prompt has no Output files heading after the Decision rule")
	}
	return section
}

func TestDecisionRuleMarker_CirclingOnlyFromTheCheckpoint(t *testing.T) {
	for _, tt := range []struct {
		name         string
		round        int
		checkpoint   int
		wantCircling bool
	}{
		{"below the checkpoint", 1, 2, false},
		{"at the checkpoint", 2, 2, true},
		{"above the checkpoint", 5, 2, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := decisionRuleMarker(tt.round, tt.checkpoint)
			if has := strings.Contains(got, "CIRCLING"); has != tt.wantCircling {
				t.Errorf("decisionRuleMarker(%d, %d) names CIRCLING = %v; want %v", tt.round, tt.checkpoint, has, tt.wantCircling)
			}
			for _, verdict := range []string{"`CONVERGED`", "`CONTINUE`"} {
				if !strings.Contains(got, verdict) {
					t.Errorf("decisionRuleMarker(%d, %d) lacks %s; want both non-circling verdicts always offered", tt.round, tt.checkpoint, verdict)
				}
			}
			if tt.wantCircling && !strings.Contains(got, "never circling") {
				t.Errorf("decisionRuleMarker(%d, %d) lacks the rising-count caveat", tt.round, tt.checkpoint)
			}
		})
	}
}

func TestBouncer_JudgePromptOffersCirclingOnlyFromTheCheckpoint(t *testing.T) {
	for _, tt := range []struct {
		name         string
		checkpoint   int
		wantCircling bool
	}{
		{"below the checkpoint", 2, false},
		{"at the checkpoint", 1, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			shuttle := &shedfake.Shuttle{Result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}}
			cfg := newBouncerFixture(t).Config
			cfg.Shuttle = shuttle
			cfg.CirclingCheckpoint = tt.checkpoint
			b, err := NewBouncer(cfg)
			if err != nil {
				t.Fatalf("NewBouncer(...) error = %v; want nil", err)
			}
			report := cfg.RunDir + "/" + cfg.ReportName(1)
			if err := os.WriteFile(report, []byte("# Round 1 report\n\nA finding.\n"), 0o644); err != nil {
				t.Fatalf("WriteFile(%q) = %v; want nil", report, err)
			}

			shedfake.CallOK(t, b)
			section := decisionRuleSection(t, shuttle.GotSpec.Prompt)
			if has := strings.Contains(section, "CIRCLING"); has != tt.wantCircling {
				t.Errorf("rendered decision rule names CIRCLING = %v; want %v\n%s", has, tt.wantCircling, section)
			}
		})
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
