package testcover_test

import (
	"reflect"
	"testing"

	"github.com/Knatte18/loomyard/internal/testcover"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// killedBy returns a set of test names.
func killedBy(names ...string) map[string]bool {
	set := map[string]bool{}
	for _, name := range names {
		set[name] = true
	}
	return set
}

func TestJudgeMutants(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name       string
		candidates []testcover.MutantCandidate
		kills      testcover.Kills
		want       []testcover.MutantVerdict
	}{
		{
			name:       "a covering test that kills every mutant the candidate kills makes it redundant and removable",
			candidates: []testcover.MutantCandidate{{Name: "A", Elapsed: 1, Covering: []string{"B"}, Mutants: []int{2, 1, 3}}},
			kills:      testcover.Kills{1: killedBy("A", "B"), 2: killedBy("A", "B"), 3: killedBy("B")},
			want:       []testcover.MutantVerdict{{Name: "A", Killed: []int{1, 2}, Redundant: true, Removable: true}},
		},
		{
			name:       "a kill no covering test shares refutes the candidate, though a test outside its covers shares it",
			candidates: []testcover.MutantCandidate{{Name: "A", Elapsed: 1, Covering: []string{"B"}, Mutants: []int{1, 2}}},
			kills:      testcover.Kills{1: killedBy("A", "B"), 2: killedBy("A", "X")},
			want:       []testcover.MutantVerdict{{Name: "A", Killed: []int{1, 2}, Unique: []int{2}}},
		},
		{
			name: "a candidate that fails unmutated, holds no mutant or kills none is not judged",
			candidates: []testcover.MutantCandidate{
				{Name: "Fails", Covering: []string{"B"}, Mutants: []int{1}, FailsUnmutated: true},
				{Name: "Empty", Covering: []string{"B"}},
				{Name: "Silent", Covering: []string{"B"}, Mutants: []int{1}},
			},
			kills: testcover.Kills{1: killedBy("Fails", "B")},
			want: []testcover.MutantVerdict{
				{Name: "Fails", Reason: testcover.ReasonFailsUnmutated},
				{Name: "Empty", Reason: testcover.ReasonNoMutant},
				{Name: "Silent", Reason: testcover.ReasonKillsNone},
			},
		},
		{
			name: "two candidates killing only each other's mutants: the slower is removable and the other kept",
			candidates: []testcover.MutantCandidate{
				{Name: "D", Elapsed: 1, Covering: []string{"C"}, Mutants: []int{1}},
				{Name: "C", Elapsed: 2, Covering: []string{"D"}, Mutants: []int{1}},
			},
			kills: testcover.Kills{1: killedBy("C", "D")},
			want: []testcover.MutantVerdict{
				{Name: "D", Killed: []int{1}, Redundant: true},
				{Name: "C", Killed: []int{1}, Redundant: true, Removable: true},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := testcover.JudgeMutants(tt.candidates, tt.kills); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("JudgeMutants = %+v\nwant           %+v", got, tt.want)
			}
		})
	}
}
