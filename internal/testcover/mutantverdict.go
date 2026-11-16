// mutantverdict.go confirms coverage redundancy candidates by the mutants each one kills.
// A candidate is redundant only when its covering tests kill every mutant it kills.

package testcover

import (
	"cmp"
	"slices"
	"strings"
)

// Reasons a mutation check does not judge a candidate.
const (
	ReasonNoMutant       = "its blocks hold no mutant"
	ReasonKillsNone      = "kills no mutant of its blocks"
	ReasonFailsUnmutated = "does not pass with no mutant on"
)

// MutantCandidate is one coverage redundancy candidate as JudgeMutants sees it.
type MutantCandidate struct {
	Name string
	// Elapsed is the test's standalone seconds; the greedy pass removes slower tests first.
	Elapsed float64
	// Covering names the tests that cover the candidate's blocks.
	Covering []string
	// Mutants are the IDs of the mutants in the blocks the candidate covers.
	Mutants []int
	// FailsUnmutated is set when the candidate did not pass with no mutant on.
	FailsUnmutated bool
}

// Kills holds, for each mutant by ID, the tests that failed while it was on.
type Kills map[int]map[string]bool

// MutantVerdict is the mutation check's judgement of one candidate.
type MutantVerdict struct {
	Name string
	// Reason is non-empty for a candidate the check cannot judge, and names why.
	Reason string
	// Killed are the IDs of the mutants the candidate killed, ascending.
	Killed []int
	// Unique are the IDs of the mutants it killed that none of its covering tests killed, ascending.
	Unique []int
	// Redundant is set when it killed a mutant and a covering test killed each one it killed.
	Redundant bool
	// Removable is set for a redundant candidate the greedy pass drops:
	// a test it keeps killed each mutant the candidate killed.
	Removable bool
}

// JudgeMutants returns the verdict of each of candidates, in the order given, from kills.
// A candidate that did not pass with no mutant on, whose blocks hold no mutant or that killed none is not judged.
// The removable set is a greedy pass over the redundant candidates, slowest first, then by name.
// It drops a candidate while some other test still kept killed each mutant the candidate killed.
func JudgeMutants(candidates []MutantCandidate, kills Kills) []MutantVerdict {
	out := make([]MutantVerdict, len(candidates))
	var redundant []int
	for i, c := range candidates {
		verdict := MutantVerdict{Name: c.Name}
		switch {
		case c.FailsUnmutated:
			verdict.Reason = ReasonFailsUnmutated
		case len(c.Mutants) == 0:
			verdict.Reason = ReasonNoMutant
		}
		if verdict.Reason == "" {
			for _, id := range slices.Sorted(slices.Values(c.Mutants)) {
				if !kills[id][c.Name] {
					continue
				}
				verdict.Killed = append(verdict.Killed, id)
				if !slices.ContainsFunc(c.Covering, func(name string) bool { return kills[id][name] }) {
					verdict.Unique = append(verdict.Unique, id)
				}
			}
			switch {
			case len(verdict.Killed) == 0:
				verdict.Reason = ReasonKillsNone
			case len(verdict.Unique) == 0:
				verdict.Redundant = true
				redundant = append(redundant, i)
			}
		}
		out[i] = verdict
	}

	slices.SortStableFunc(redundant, func(a, b int) int {
		return cmp.Or(cmp.Compare(candidates[b].Elapsed, candidates[a].Elapsed), strings.Compare(candidates[a].Name, candidates[b].Name))
	})
	removed := map[string]bool{}
	for _, i := range redundant {
		name := candidates[i].Name
		killedElsewhere := func(id int) bool {
			for killer := range kills[id] {
				if killer != name && !removed[killer] {
					return true
				}
			}
			return false
		}
		if !slices.ContainsFunc(out[i].Killed, func(id int) bool { return !killedElsewhere(id) }) {
			out[i].Removable = true
			removed[name] = true
		}
	}
	return out
}
