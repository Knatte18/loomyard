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

// TestIndex_AppendsPlanChecks asserts the index reports a declared host-geometry name (fabric-vocabulary-name) and a test file under Edit (edit-test-file) through its format, rework and dispatch validations.
// A completed card is dropped at dispatch for the first check, and at format validation for the second, which dispatch never runs.
func TestIndex_AppendsPlanChecks(t *testing.T) {
	t.Parallel()

	declaring := func(handle, decl string) planparser.Card {
		return planparser.Card{
			Number:       1,
			Slug:         "one",
			Declarations: []planparser.CardDeclaration{{Handle: handle, Decl: decl}},
		}
	}
	editingTestFile := planparser.Card{
		Number:       1,
		Slug:         "one",
		TargetGroups: []planparser.TargetGroup{{Type: planparser.CardTypeEdit, Refs: []string{"internal/x/x_test.go#"}}},
	}
	countFindings := func(t *testing.T, findings []Finding, check string) int {
		t.Helper()
		count := 0
		for _, finding := range findings {
			if finding.Check == check {
				count++
			}
		}
		return count
	}

	offending := declaring("plan:internal/x#hostBranch", "func hostBranch()")
	plain := declaring("plan:internal/x#plainName", "func plainName()")
	idx := NewIndex(noFabricReference{})

	tests := []struct {
		name     string
		run      func(plan *planparser.Plan) ([]Finding, error)
		check    string
		language string
		card     planparser.Card
		want     int
	}{
		{
			name: "format with no done card",
			run: func(plan *planparser.Plan) ([]Finding, error) {
				return idx.ValidateFormat(plan, t.TempDir(), nil)
			},
			check:    "fabric-vocabulary-name",
			language: "none",
			card:     offending,
			want:     1,
		},
		{
			name: "rework told the plan's first card",
			run: func(plan *planparser.Plan) ([]Finding, error) {
				return idx.ValidateRework(plan, t.TempDir(), plan.FirstCard)
			},
			check:    "fabric-vocabulary-name",
			language: "none",
			card:     offending,
			want:     1,
		},
		{
			name: "dispatch with the card forthcoming",
			run: func(plan *planparser.Plan) ([]Finding, error) {
				return idx.ValidateDispatch(plan, t.TempDir(), nil, plan.Cards)
			},
			check:    "fabric-vocabulary-name",
			language: "none",
			card:     offending,
			want:     1,
		},
		{
			name: "dispatch with the card completed",
			run: func(plan *planparser.Plan) ([]Finding, error) {
				return idx.ValidateDispatch(plan, t.TempDir(), plan.Cards, nil)
			},
			check:    "fabric-vocabulary-name",
			language: "none",
			card:     offending,
			want:     0,
		},
		{
			name: "format over a plain name",
			run: func(plan *planparser.Plan) ([]Finding, error) {
				return idx.ValidateFormat(plan, t.TempDir(), nil)
			},
			check:    "fabric-vocabulary-name",
			language: "none",
			card:     plain,
			want:     0,
		},
		{
			name: "format over a test file under Edit",
			run: func(plan *planparser.Plan) ([]Finding, error) {
				return idx.ValidateFormat(plan, t.TempDir(), nil)
			},
			check:    "edit-test-file",
			language: "go",
			card:     editingTestFile,
			want:     1,
		},
		{
			name: "rework over a test file under Edit",
			run: func(plan *planparser.Plan) ([]Finding, error) {
				return idx.ValidateRework(plan, t.TempDir(), plan.FirstCard)
			},
			check:    "edit-test-file",
			language: "go",
			card:     editingTestFile,
			want:     1,
		},
		{
			name: "dispatch over a test file under Edit",
			run: func(plan *planparser.Plan) ([]Finding, error) {
				return idx.ValidateDispatch(plan, t.TempDir(), nil, plan.Cards)
			},
			check:    "edit-test-file",
			language: "go",
			card:     editingTestFile,
			want:     0,
		},
		{
			name: "format with the test-file card done",
			run: func(plan *planparser.Plan) ([]Finding, error) {
				return idx.ValidateFormat(plan, t.TempDir(), plan.Cards)
			},
			check:    "edit-test-file",
			language: "go",
			card:     editingTestFile,
			want:     0,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			plan := minimalPlan(t, t.TempDir())
			plan.Language = tc.language
			plan.FirstCard = 1
			plan.Cards = []planparser.Card{tc.card}

			findings, _ := tc.run(plan)

			if got := countFindings(t, findings, tc.check); got != tc.want {
				t.Errorf("%s findings = %d, want %d (all: %v)", tc.check, got, tc.want, findings)
			}
		})
	}
}
