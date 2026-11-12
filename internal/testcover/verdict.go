// verdict.go judges the per-test block sets of one run.
// It decides which tests are contained in the others and which may go together.

package testcover

import (
	"slices"
	"sort"
)

// Reasons a test is not judged.
const (
	ReasonSkipped      = "skipped"
	ReasonOutOfProcess = "out of process"
	ReasonUnresolved   = "unclassifiable call"
	ReasonNoBlocks     = "no blocks"
	ReasonFailed       = "failed"
	ReasonKept         = "kept"
)

// Run is one top-level test as Judge sees it.
type Run struct {
	Name string
	// Elapsed is the test's standalone seconds; the greedy pass removes slower tests first.
	Elapsed float64
	// Action is ActionPass, ActionFail or ActionSkip.
	Action string
	// Blocks are the blocks the test covered.
	Blocks BlockSet
	// Spawn is the static spawn scan's verdict on the test.
	Spawn SpawnVerdict
}

// Verdict is the judgement of one run.
type Verdict struct {
	Name string
	// Reason is non-empty for a test that is not judged, and names why.
	Reason string
	// Contained is set when other judged tests cover every block the test covers.
	Contained bool
	// Removable is set when the greedy pass drops the test, so it may be deleted together with every other removable test.
	Removable bool
	// Covering names, for a contained test, the tests that together cover its blocks, largest overlap first.
	// For a removable test they come from the tests the pass keeps.
	Covering []string
}

// Judge returns the verdict of every run, in the order given.
// A test is judged when it passed and covered a block.
// It must also be one the spawn scan neither saw run module code in another process nor failed to resolve a call.
// Only a judged test's blocks count as covered.
// The removable set is a greedy pass over the contained tests, slowest first, then by name.
// It re-checks each contained test against the tests still kept.
func Judge(runs []Run) []Verdict {
	return JudgeAgainst(runs, nil)
}

// JudgeAgainst returns the verdict of every run, in the order given.
// It counts the runs of kept as covering tests that are never judged or removed themselves.
// A kept run covers its blocks for every run,
// and the greedy pass drops a contained run only while the runs left keep its blocks covered.
// A caller judging new tests against tests it already holds passes those as kept.
func JudgeAgainst(runs, kept []Run) []Verdict {
	all := slices.Concat(runs, kept)
	counts := map[string]int{}
	for _, r := range all {
		if notJudgedReason(r) == "" {
			for b := range r.Blocks {
				counts[b]++
			}
		}
	}
	covered := func(r Run) bool {
		for b := range r.Blocks {
			if counts[b] < 2 {
				return false
			}
		}
		return true
	}

	out := make([]Verdict, len(all))
	var contained []int
	for i, r := range all {
		out[i].Name = r.Name
		out[i].Reason = notJudgedReason(r)
		if i < len(runs) && out[i].Reason == "" && covered(r) {
			out[i].Contained = true
			contained = append(contained, i)
		}
	}

	sort.SliceStable(contained, func(a, b int) bool {
		ra, rb := all[contained[a]], all[contained[b]]
		if ra.Elapsed != rb.Elapsed {
			return ra.Elapsed > rb.Elapsed
		}
		return ra.Name < rb.Name
	})
	for _, i := range contained {
		if !covered(all[i]) {
			continue
		}
		out[i].Removable = true
		for b := range all[i].Blocks {
			counts[b]--
		}
	}

	for _, i := range contained {
		inPool := func(j int) bool { return out[j].Reason == "" && !out[j].Removable }
		if !out[i].Removable {
			inPool = func(j int) bool { return out[j].Reason == "" }
		}
		out[i].Covering = coveringTests(i, all, inPool)
	}
	return out[:len(runs)]
}

// JudgeMap returns the verdict of every test of the coverage map m, in the map's order.
// It judges by the blocks of the package's own files, each entry's seconds ordering the greedy pass.
// A test that keeps names, one marked `//testtiming:keep`, is neither judged nor counted as a cover;
// its reason is ReasonKept.
// An entry that records a TestOutcome is not judged either, and the outcome is its reason.
// Nor is an entry whose spawn verdict marks the test.
// Every block index of m must lie inside its block table.
func JudgeMap(m Map, keeps map[string]string) []Verdict {
	out := make([]Verdict, len(m.Tests))
	var runs []Run
	var at []int
	for i, test := range m.Tests {
		out[i].Name = test.Name
		if _, kept := keeps[test.Name]; kept {
			out[i].Reason = ReasonKept
			continue
		}
		if test.Outcome != "" {
			out[i].Reason = string(test.Outcome)
			continue
		}
		blocks := BlockSet{}
		for _, index := range test.Blocks {
			blocks[m.Blocks[index].String()] = struct{}{}
		}
		runs = append(runs, Run{Name: test.Name, Elapsed: test.Seconds, Action: ActionPass, Blocks: blocks, Spawn: test.Spawn})
		at = append(at, i)
	}
	for j, verdict := range Judge(runs) {
		out[at[j]] = verdict
	}
	return out
}

// notJudgedReason returns why a run is not judged, or "" for a run that is.
func notJudgedReason(r Run) string {
	switch {
	case r.Action == ActionSkip:
		return ReasonSkipped
	case r.Action != ActionPass:
		return ReasonFailed
	case r.Spawn.OutOfProcess:
		return ReasonOutOfProcess
	case r.Spawn.Unresolved:
		return ReasonUnresolved
	case len(r.Blocks) == 0:
		return ReasonNoBlocks
	}
	return ""
}

// coveringTests names the tests of the pool that together cover runs[self]'s blocks.
// It takes the largest overlap first and skips a test that adds no block still uncovered.
func coveringTests(self int, runs []Run, inPool func(int) bool) []string {
	type overlap struct {
		idx int
		n   int
	}
	var overlaps []overlap
	for j, r := range runs {
		if j == self || !inPool(j) {
			continue
		}
		n := 0
		for b := range runs[self].Blocks {
			if _, ok := r.Blocks[b]; ok {
				n++
			}
		}
		if n > 0 {
			overlaps = append(overlaps, overlap{j, n})
		}
	}
	sort.Slice(overlaps, func(a, b int) bool {
		if overlaps[a].n != overlaps[b].n {
			return overlaps[a].n > overlaps[b].n
		}
		return runs[overlaps[a].idx].Name < runs[overlaps[b].idx].Name
	})

	remaining := map[string]struct{}{}
	for b := range runs[self].Blocks {
		remaining[b] = struct{}{}
	}
	var names []string
	for _, o := range overlaps {
		if len(remaining) == 0 {
			break
		}
		hit := false
		for b := range remaining {
			if _, ok := runs[o.idx].Blocks[b]; ok {
				delete(remaining, b)
				hit = true
			}
		}
		if hit {
			names = append(names, runs[o.idx].Name)
		}
	}
	return names
}
