// decisionswitches_test.go pins which shipped recipe rows carry the decision-escalation switches:
// escalate_decisions on a Bouncer row and decisions_escalated on a BurlerFix row.
//
// The two are one switch in two rows.
// The Bouncer sends a round's decision-class findings to the run's parent,
// and the fix row of the same segment keeps them out of the fixer and posts nothing.
// It parses the real embedded recipes and reads no worktree, staying offline.

package loomrecipe

import (
	"reflect"
	"sort"
	"testing"

	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestDecisionSwitches_SetTogetherOnLoomsDiscussionRowsOnly asserts both switches are true on exactly Discussion-Bouncer and Discussion-Burler-Fix in the tapestry recipe and on no row of the darn recipe.
// The Plan-Review and Code-Review rows therefore keep the fix row's own parent notice.
func TestDecisionSwitches_SetTogetherOnLoomsDiscussionRowsOnly(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	for _, name := range []string{shedrun.RecipeTapestry, shedrun.RecipeDarn} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			recipe, err := parseEmbeddedRecipe(name)
			if err != nil {
				t.Fatalf("parseEmbeddedRecipe(%s) error = %v; want nil", name, err)
			}
			var gotEscalate, gotEscalated []string
			for _, row := range recipe.Producers {
				if set, _ := row.Config["escalate_decisions"].(bool); set {
					gotEscalate = append(gotEscalate, row.Name)
				}
				if set, _ := row.Config["decisions_escalated"].(bool); set {
					gotEscalated = append(gotEscalated, row.Name)
				}
			}
			sort.Strings(gotEscalate)
			sort.Strings(gotEscalated)
			var wantEscalate, wantEscalated []string
			if name == shedrun.RecipeTapestry {
				wantEscalate = []string{"Discussion-Bouncer"}
				wantEscalated = []string{"Discussion-Burler-Fix"}
			}
			if !reflect.DeepEqual(gotEscalate, wantEscalate) {
				t.Errorf("rows with escalate_decisions = %v; want %v", gotEscalate, wantEscalate)
			}
			if !reflect.DeepEqual(gotEscalated, wantEscalated) {
				t.Errorf("rows with decisions_escalated = %v; want %v", gotEscalated, wantEscalated)
			}
		})
	}
}
