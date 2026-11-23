// decisionswitches_test.go pins which shipped recipe rows carry a switch that hands a review round to the run's parent.
// escalate_decisions sits on a Bouncer row.
// decisions_escalated sits on a BurlerFix or HardenFix row.
// blocking_escalates sits on a Bouncer row.
//
// The first two are one switch in two rows.
// The Bouncer sends a round's decision-class findings to the run's parent,
// and the fix row of the same segment keeps them out of the fixer and posts nothing.
// blocking_escalates sends a spent budget with an open BLOCKING finding to the parent instead of an accept.
// It parses the real embedded recipes and reads no worktree, staying offline.

package loomrecipe

import (
	"reflect"
	"sort"
	"testing"

	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestDecisionSwitches_SetOnTheRowsThatAskTheParent asserts which rows carry each switch.
// The decision switches sit on the Discussion-Review, Code-Review, split and Harden-Review perches,
// so the Plan-Review rows keep the fix row's own parent notice.
// blocking_escalates sits on every Code-Bouncer row.
//
//testtiming:keep pins which shipped recipe rows set escalate_decisions, decisions_escalated and blocking_escalates
func TestDecisionSwitches_SetOnTheRowsThatAskTheParent(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		recipe                                    string
		wantEscalate, wantEscalated, wantBlocking []string
	}{
		{recipe: shedrun.RecipeTapestry,
			wantEscalate:  []string{"Code-Bouncer", "Discussion-Bouncer"},
			wantEscalated: []string{"Code-Burler-Fix", "Discussion-Burler-Fix"},
			wantBlocking:  []string{"Code-Bouncer"}},
		{recipe: shedrun.RecipeDarn, wantEscalate: []string{"Code-Bouncer"}, wantEscalated: []string{"Code-Burler-Fix"}, wantBlocking: []string{"Code-Bouncer"}},
		{recipe: shedrun.RecipeSampler, wantEscalate: []string{"Code-Bouncer"}, wantEscalated: []string{"Code-Burler-Fix"}, wantBlocking: []string{"Code-Bouncer"}},
		{recipe: shedrun.RecipeArras,
			wantEscalate:  []string{"Discussion-Bouncer", "Raddle-Bouncer"},
			wantEscalated: []string{"Discussion-Burler-Fix", "Raddle-Burler-Fix"}},
		{recipe: shedrun.RecipeHardener, wantEscalate: []string{"Harden-Bouncer"}, wantEscalated: []string{"Harden-Burler-Fix"}},
	}
	for _, tt := range tests {
		t.Run(tt.recipe, func(t *testing.T) {
			t.Parallel()
			recipe, err := parseEmbeddedRecipe(tt.recipe)
			if err != nil {
				t.Fatalf("parseEmbeddedRecipe(%s) error = %v; want nil", tt.recipe, err)
			}
			var gotEscalate, gotEscalated, gotBlocking []string
			for _, row := range recipe.Producers {
				if set, _ := row.Config["escalate_decisions"].(bool); set {
					gotEscalate = append(gotEscalate, row.Name)
				}
				if set, _ := row.Config["decisions_escalated"].(bool); set {
					gotEscalated = append(gotEscalated, row.Name)
				}
				if set, _ := row.Config["blocking_escalates"].(bool); set {
					gotBlocking = append(gotBlocking, row.Name)
				}
			}
			sort.Strings(gotEscalate)
			sort.Strings(gotEscalated)
			sort.Strings(gotBlocking)
			if !reflect.DeepEqual(gotEscalate, tt.wantEscalate) {
				t.Errorf("rows with escalate_decisions = %v; want %v", gotEscalate, tt.wantEscalate)
			}
			if !reflect.DeepEqual(gotEscalated, tt.wantEscalated) {
				t.Errorf("rows with decisions_escalated = %v; want %v", gotEscalated, tt.wantEscalated)
			}
			if !reflect.DeepEqual(gotBlocking, tt.wantBlocking) {
				t.Errorf("rows with blocking_escalates = %v; want %v", gotBlocking, tt.wantBlocking)
			}
		})
	}
}
