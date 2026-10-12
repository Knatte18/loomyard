// index_test.go covers the checks the plan index appends beside planparser's entry points.

package planglyph

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/planparser"
)

// noFabricReference is a FabricReferenceMatcher that never matches.
type noFabricReference struct{}

func (noFabricReference) MatchPath(string) (string, bool)     { return "", false }
func (noFabricReference) MatchSpelling(string) (string, bool) { return "", false }

// TestIndex_AppendsFabricVocabularyName asserts the index reports a declared host-geometry name through its format, rework and dispatch validations, and drops it at dispatch once its card is completed.
func TestIndex_AppendsFabricVocabularyName(t *testing.T) {
	t.Parallel()

	declaring := func(handle, decl string) planparser.Card {
		return planparser.Card{
			Number:       1,
			Slug:         "one",
			Declarations: []planparser.CardDeclaration{{Handle: handle, Decl: decl}},
		}
	}
	countVocabularyFindings := func(t *testing.T, findings []Finding) int {
		t.Helper()
		count := 0
		for _, finding := range findings {
			if finding.Check == "fabric-vocabulary-name" {
				count++
			}
		}
		return count
	}

	offending := declaring("plan:internal/x#hostBranch", "func hostBranch()")
	plain := declaring("plan:internal/x#plainName", "func plainName()")
	idx := NewIndex(noFabricReference{})

	tests := []struct {
		name string
		run  func(plan *planparser.Plan) ([]Finding, error)
		card planparser.Card
		want int
	}{
		{
			name: "format with no done card",
			run: func(plan *planparser.Plan) ([]Finding, error) {
				return idx.ValidateFormat(plan, t.TempDir(), nil)
			},
			card: offending,
			want: 1,
		},
		{
			name: "rework told the plan's first card",
			run: func(plan *planparser.Plan) ([]Finding, error) {
				return idx.ValidateRework(plan, t.TempDir(), plan.FirstCard)
			},
			card: offending,
			want: 1,
		},
		{
			name: "dispatch with the card forthcoming",
			run: func(plan *planparser.Plan) ([]Finding, error) {
				return idx.ValidateDispatch(plan, t.TempDir(), nil, plan.Cards)
			},
			card: offending,
			want: 1,
		},
		{
			name: "dispatch with the card completed",
			run: func(plan *planparser.Plan) ([]Finding, error) {
				return idx.ValidateDispatch(plan, t.TempDir(), plan.Cards, nil)
			},
			card: offending,
			want: 0,
		},
		{
			name: "format over a plain name",
			run: func(plan *planparser.Plan) ([]Finding, error) {
				return idx.ValidateFormat(plan, t.TempDir(), nil)
			},
			card: plain,
			want: 0,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			plan := minimalPlan(t, t.TempDir())
			plan.FirstCard = 1
			plan.Cards = []planparser.Card{tc.card}

			findings, _ := tc.run(plan)

			if got := countVocabularyFindings(t, findings); got != tc.want {
				t.Errorf("fabric-vocabulary-name findings = %d, want %d (all: %v)", got, tc.want, findings)
			}
		})
	}
}
