// models.go declares the per-round lists of shuttle model choices of a burler round's two halves: the reviewer's and the fixer's, picked by round number.

package burlerengine

import "github.com/Knatte18/loomyard/internal/shuttleengine"

// RoundModels holds the reviewer's and the fixer's model choices as per-round lists.
// Each entry carries its own engine, so one round can run on a different engine from the next.
// A list of one applies to every round.
type RoundModels struct {
	Review []shuttleengine.ModelChoice
	Fix    []shuttleengine.ModelChoice
}

// Pick returns the review and fix choices for round, counting rounds from 1.
// Each list is resolved independently: entry round of the list, its last entry for a round past the list, and the first entry for a round below 1.
// An empty list yields the zero ModelChoice.
func (m RoundModels) Pick(round int) (review, fix shuttleengine.ModelChoice) {
	return pickRound(m.Review, round), pickRound(m.Fix, round)
}

// pickRound returns the entry of choices for round, clamped to the list's bounds, or the zero ModelChoice for an empty list.
func pickRound(choices []shuttleengine.ModelChoice, round int) shuttleengine.ModelChoice {
	if len(choices) == 0 {
		return shuttleengine.ModelChoice{}
	}
	index := min(max(round, 1), len(choices)) - 1
	return choices[index]
}
