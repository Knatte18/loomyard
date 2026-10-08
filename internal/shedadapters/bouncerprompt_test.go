// bouncerprompt_test.go covers that BouncerConfig.ClusterExcludes reaches the seed and judge fills
// and that the judge prompt offers CIRCLING only from the checkpoint round.

package shedadapters

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/shuttleengine"
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
	for _, rule := range []string{"An exclusion is permanent for the segment generation", "the latest round ran it", "never restate them"} {
		if !strings.Contains(prompt, rule) {
			t.Errorf("key-on prompt lacks the %q exclusion rule", rule)
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

//testtiming:keep pins the decision rule's text: both non-circling verdicts are always offered, the rising-count caveat comes with CIRCLING, and round 2 on renders the lighter rule with its earlier-open list and round-1 fallback
func TestDecisionRuleMarker_CirclingOnlyFromTheCheckpoint(t *testing.T) {
	for _, tt := range []struct {
		name         string
		round        int
		checkpoint   int
		wantCircling bool
		wantLighter  bool
	}{
		{"round 1 below the checkpoint", 1, 2, false, false},
		{"round 2 below the checkpoint", 2, 3, false, true},
		{"at the checkpoint", 2, 2, true, true},
		{"above the checkpoint", 5, 2, true, true},
		{"round 1 at the checkpoint", 1, 1, true, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := decisionRuleMarker(tt.round, tt.checkpoint)
			lighterPhrases := []string{"keys open in an earlier round", "`## Open risks`", "rule by round 1's rule instead: `CONTINUE` while the latest round carries a gating-class finding at MEDIUM or worse, or any BLOCKING finding"}
			for _, phrase := range lighterPhrases {
				if has := strings.Contains(got, phrase); has != tt.wantLighter {
					t.Errorf("decisionRuleMarker(%d, %d) contains %q = %v; want %v", tt.round, tt.checkpoint, phrase, has, tt.wantLighter)
				}
			}
			if !tt.wantLighter && !strings.Contains(got, "- `CONTINUE` while the latest round carries a gating-class finding at MEDIUM or worse, or any BLOCKING finding.\n") {
				t.Errorf("decisionRuleMarker(%d, %d) lacks round 1's CONTINUE bullet", tt.round, tt.checkpoint)
			}
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

//testtiming:keep pins that the checkpoint round configured on the Bouncer reaches the judge prompt's decision rule
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

//testtiming:keep pins that the exclude_lenses example line and rule reach the seed and judge prompts exactly when ClusterExcludes is set
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
			assertExcludeLensesText(t, shuttle.GotSpec.Prompt, false)
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
