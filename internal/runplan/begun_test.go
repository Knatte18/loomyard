// begun_test.go pins LastBegunCard, the bound the card verbs read from a run's batch records.
// Tier 0: states built in memory.

package runplan

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestLastBegunCard(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name string
		st   *State
		want int
	}{
		{"nil state", nil, 0},
		{"no records", &State{Batches: map[int]*BatchState{}}, 0},
		{"record with cards", &State{Batches: map[int]*BatchState{
			1: {Cards: []string{"01-a", "02-b", "03-c"}},
		}}, 3},
		{"record without cards falls back to its key", &State{Batches: map[int]*BatchState{
			4: {Slug: "old"},
		}}, 4},
		{"done record beside a later non-terminal one", &State{Batches: map[int]*BatchState{
			1: {Cards: []string{"01-a"}, Terminal: true, Status: "done"},
			2: {Cards: []string{"02-b"}},
		}}, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := LastBegunCard(tt.st); got != tt.want {
				t.Errorf("LastBegunCard() = %d; want %d", got, tt.want)
			}
		})
	}
}
