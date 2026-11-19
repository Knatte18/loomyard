// mutatecheck_test.go covers how the mutation check takes mutation_max_mutants of the planned mutants.

package gatecli

import (
	"fmt"
	"maps"
	"slices"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/Knatte18/loomyard/internal/testcover"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// plannedMutants returns the planned package pkg whose functions, in order, hold the counts of mutants.
// Its mutant IDs count from 1 in function order.
func plannedMutants(pkg string, counts ...int) *mutatedPackage {
	var mutants []testcover.Mutant
	for function, count := range counts {
		for range count {
			key := fmt.Sprintf("f.go|F%d|hash|0", function)
			mutants = append(mutants, testcover.Mutant{ID: len(mutants) + 1, File: "f.go", Blocks: []string{key}})
		}
	}
	return &mutatedPackage{pkg: pkg, plan: &testcover.MutantPlan{Mutants: mutants}}
}

func TestDiffMutation_SelectWithinCap(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	// Package a has four functions of two mutants each, and package b four functions of one each.
	tests := []struct {
		name       string
		maxMutants int
		// want holds the IDs the cap takes of each package.
		want       map[string][]int
		wantNotRun int
	}{
		{
			name: "ACapBelowTheFunctionCountTakesFunctionsSpreadEvenlyOverBothPackages", maxMutants: 4,
			want: map[string][]int{"a": {1, 5}, "b": {1, 3}}, wantNotRun: 8,
		},
		{
			name: "ACapAboveTheFunctionCountTakesOneOfEachThenSpreadsTheNextRound", maxMutants: 10,
			want: map[string][]int{"a": {1, 2, 3, 5, 6, 7}, "b": {1, 2, 3, 4}}, wantNotRun: 2,
		},
		{
			name: "ACapAboveEveryMutantTakesThemAll", maxMutants: 20,
			want: map[string][]int{"a": {1, 2, 3, 4, 5, 6, 7, 8}, "b": {1, 2, 3, 4}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			planned := []*mutatedPackage{plannedMutants("a", 2, 2, 2, 2), plannedMutants("b", 1, 1, 1, 1)}
			check := &diffMutation{maxMutants: tt.maxMutants, outcome: newMutateOutcome()}
			check.selectWithinCap(planned)
			got := map[string][]int{}
			for _, judged := range planned {
				got[judged.pkg] = slices.Sorted(maps.Keys(judged.selected))
			}
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("selectWithinCap() under a cap of %d mismatch (-want +got):\n%s", tt.maxMutants, diff)
			}
			if got := check.outcome.counts.NotRun; got != tt.wantNotRun {
				t.Errorf("selectWithinCap() under a cap of %d counts %d not run; want %d", tt.maxMutants, got, tt.wantNotRun)
			}
		})
	}
}
