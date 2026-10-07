// models.go declares the per-round model choices of a burler round's two halves: the reviewer's and the fixer's resolved model-specs, picked by round number.

package burlerengine

// ModelChoice is one resolved model-spec: the provider-side model string and its effort and version parameters, each empty when unset.
type ModelChoice struct {
	Model   string
	Effort  string
	Version string
}

// RoundModels holds the reviewer's and the fixer's model choices as per-round lists.
// A list of one applies to every round.
type RoundModels struct {
	Review []ModelChoice
	Fix    []ModelChoice
}

// Pick returns the review and fix choices for round, counting rounds from 1.
// Each list is resolved independently: entry round of the list, its last entry for a round past the list, and the first entry for a round below 1.
// An empty list yields the zero ModelChoice.
func (m RoundModels) Pick(round int) (review, fix ModelChoice) {
	return pickRound(m.Review, round), pickRound(m.Fix, round)
}

// pickRound returns the entry of choices for round, clamped to the list's bounds, or the zero ModelChoice for an empty list.
func pickRound(choices []ModelChoice, round int) ModelChoice {
	if len(choices) == 0 {
		return ModelChoice{}
	}
	index := min(max(round, 1), len(choices)) - 1
	return choices[index]
}
