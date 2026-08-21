// check.go implements Check, the authoring-time structural analysis of an assembled
// shedengine.ProducerDef graph: it walks OnDone/OnStuck routing the same way shedengine.Run would,
// and reports every structural defect it finds instead of stopping at the first one.

package shedcheck

import (
	"fmt"
	"sort"

	"github.com/Knatte18/loomyard/internal/shedengine"
)

// The three states a node passes through while check walks the done-edge subgraph looking for
// cycles: unvisited, currently on the walk's own path (visiting), or fully resolved (finished, no
// cycle through it remains to be found).
const (
	stateUnvisited = iota
	stateVisiting
	stateFinished
)

// Check inspects an assembled OnDone/OnStuck graph and reports every structural defect it finds,
// in the fixed order documented on the Kind constants. It reads only Name, OnDone, and OnStuck off
// each shedengine.ProducerDef -- it never dereferences Producer, so a ProducerDef with a nil
// Producer is valid input, and it never reads Segment or MaxBounces. Check never panics and never
// returns an error; a clean graph returns nil, not an empty non-nil slice.
func Check(producers []shedengine.ProducerDef, entry string, terminals []string) []Finding {
	indexByName := buildIndexByName(producers)

	// Endpoint findings short-circuit every check below: reachability and terminality are
	// meaningless without told endpoints, so an endpoint defect is reported alone rather than
	// alongside an avalanche of derived noise.
	var endpointFindings []Finding
	if entry == "" {
		endpointFindings = append(endpointFindings, Finding{Kind: KindBadEntry, Producer: entry, Target: "", Message: "entry is empty"})
	} else if _, ok := indexByName[entry]; !ok {
		endpointFindings = append(endpointFindings, Finding{Kind: KindBadEntry, Producer: entry, Target: "", Message: fmt.Sprintf("entry %q names no producer in the list", entry)})
	}
	if len(terminals) == 0 {
		endpointFindings = append(endpointFindings, Finding{Kind: KindNoTerminals, Producer: "", Target: "", Message: "terminals is empty"})
	}
	for _, term := range terminals {
		if term == "" {
			endpointFindings = append(endpointFindings, Finding{Kind: KindBadTerminal, Producer: term, Target: "", Message: "terminal name is empty"})
			continue
		}
		if _, ok := indexByName[term]; !ok {
			endpointFindings = append(endpointFindings, Finding{Kind: KindBadTerminal, Producer: term, Target: "", Message: fmt.Sprintf("terminal %q names no producer in the list", term)})
		}
	}
	if len(endpointFindings) > 0 {
		return endpointFindings
	}

	// liveIdx holds every live row's list index, in list order: a row is live when its own index
	// equals indexByName's recorded first occurrence of its Name, and shadowed otherwise. A
	// shadowed row contributes no edges to the graph -- it is skipped by every check below except
	// unreachable, which always reports it, because shedengine.findProducer's linear first-match
	// scan means Run would never reach or walk it either.
	liveIdx := make([]int, 0, len(producers))
	for i, p := range producers {
		if indexByName[p.Name] == i {
			liveIdx = append(liveIdx, i)
		}
	}

	// The field-value vs. graph-structure rule, resolved once here: a check keyed on a raw field
	// value (unexpectedTerminal's raw OnDone == "") reads the field as supplied, while a check
	// keyed on graph structure (reachability, doneCycle, blindGate) reads the post-drop graph
	// built below, where a dangling edge has already been removed. Its two pinned consequences: a
	// row with a dangling (therefore non-empty) OnDone is never also reported as
	// unexpectedTerminal, and a row whose stuck edge is dangling has no stuck edge at all in the
	// post-drop graph and therefore can never be a blindGate.
	onDoneIdx := make(map[int]int, len(liveIdx))
	onStuckIdx := make(map[int]int, len(liveIdx))
	var danglingFindings []Finding
	for _, i := range liveIdx {
		p := producers[i]
		if p.OnDone != "" {
			if target, ok := indexByName[p.OnDone]; ok {
				onDoneIdx[i] = target
			} else {
				danglingFindings = append(danglingFindings, Finding{Kind: KindDanglingTarget, Producer: p.Name, Target: p.OnDone, Message: fmt.Sprintf("OnDone %q names no producer in the list", p.OnDone)})
			}
		}
		if p.OnStuck != "" {
			if target, ok := indexByName[p.OnStuck]; ok {
				onStuckIdx[i] = target
			} else {
				danglingFindings = append(danglingFindings, Finding{Kind: KindDanglingTarget, Producer: p.Name, Target: p.OnStuck, Message: fmt.Sprintf("OnStuck %q names no producer in the list", p.OnStuck)})
			}
		}
	}

	reachable := reachableIndices(indexByName[entry], onDoneIdx, onStuckIdx)

	var unreachableFindings []Finding
	for i, p := range producers {
		if !reachable[i] {
			unreachableFindings = append(unreachableFindings, Finding{Kind: KindUnreachable, Producer: p.Name, Target: "", Message: "not reachable from entry"})
		}
	}

	terminalSet := make(map[string]bool, len(terminals))
	for _, t := range terminals {
		terminalSet[t] = true
	}
	var unexpectedTerminalFindings []Finding
	for _, i := range liveIdx {
		if !reachable[i] {
			continue
		}
		p := producers[i]
		if p.OnDone == "" && !terminalSet[p.Name] {
			unexpectedTerminalFindings = append(unexpectedTerminalFindings, Finding{Kind: KindUnexpectedTerminal, Producer: p.Name, Target: "", Message: "reaches a silent Done terminal that was never told as one"})
		}
	}

	doneCycleFindings := findDoneCycles(producers, liveIdx, reachable, onDoneIdx)

	var blindGateFindings []Finding
	for _, i := range liveIdx {
		if !reachable[i] {
			continue
		}
		p := producers[i]
		target, ok := onStuckIdx[i]
		if !ok || target == i {
			// No stuck edge at all (the escalate-to-human sentinel), or a self-bounce, which is
			// trivially not blind and is legal and budgeted.
			continue
		}
		fromTarget := reachableIndices(target, onDoneIdx, onStuckIdx)
		if !fromTarget[i] {
			blindGateFindings = append(blindGateFindings, Finding{Kind: KindBlindGate, Producer: p.Name, Target: p.OnStuck, Message: "stuck target has no route back to this gate"})
		}
	}

	var result []Finding
	result = append(result, danglingFindings...)
	result = append(result, unreachableFindings...)
	result = append(result, unexpectedTerminalFindings...)
	result = append(result, doneCycleFindings...)
	result = append(result, blindGateFindings...)
	if len(result) == 0 {
		return nil
	}
	return result
}

// buildIndexByName records the first list index at which each Name occurs, in a single forward
// pass -- matching findProducer's linear first-match scan in internal/shedengine/run.go. The
// returned map is used for lookups only and must never be range-iterated: Go map iteration order
// is randomised, and Check's whole output order is a pinned contract.
func buildIndexByName(producers []shedengine.ProducerDef) map[string]int {
	indexByName := make(map[string]int, len(producers))
	for i, p := range producers {
		if _, exists := indexByName[p.Name]; !exists {
			indexByName[p.Name] = i
		}
	}
	return indexByName
}

// reachableIndices walks both done and stuck edges (post-drop) from start, over list indices, and
// returns every index the walk reaches, including start itself.
func reachableIndices(start int, onDoneIdx, onStuckIdx map[int]int) map[int]bool {
	visited := map[int]bool{start: true}
	queue := []int{start}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, edges := range [2]map[int]int{onDoneIdx, onStuckIdx} {
			if target, ok := edges[cur]; ok && !visited[target] {
				visited[target] = true
				queue = append(queue, target)
			}
		}
	}
	return visited
}

// findDoneCycles reports one KindDoneCycle finding per cycle member, over live, reachable rows and
// done edges only. The done-edge subgraph is functional -- every row has at most one outgoing done
// edge -- so a single walk with visited marks per node finds every cycle; no general
// strongly-connected-components algorithm is needed.
func findDoneCycles(producers []shedengine.ProducerDef, liveIdx []int, reachable map[int]bool, onDoneIdx map[int]int) []Finding {
	state := make([]int, len(producers))
	var cycles [][]int
	for _, i := range liveIdx {
		if !reachable[i] || state[i] != stateUnvisited {
			continue
		}
		if cycle := walkForCycle(i, onDoneIdx, state); cycle != nil {
			cycles = append(cycles, cycle)
		}
	}

	// Discovery order is not the required order: a walk starting at a row outside a cycle can
	// reach a high-index cycle before a lower-index one is ever visited, so the collected cycles
	// are sorted explicitly by their own lowest member index before emitting.
	sort.Slice(cycles, func(a, b int) bool {
		return minInt(cycles[a]) < minInt(cycles[b])
	})

	var findings []Finding
	for _, cycle := range cycles {
		node := minInt(cycle)
		for range cycle {
			p := producers[node]
			findings = append(findings, Finding{Kind: KindDoneCycle, Producer: p.Name, Target: p.OnDone, Message: "member of a done-edge cycle"})
			node = onDoneIdx[node]
		}
	}
	return findings
}

// walkForCycle follows done edges from start until it either closes a cycle back onto its own
// path or runs off the path's end, mutating state as it goes so no node is walked twice across
// calls. It returns the cycle's member indices (unordered) when one is found, and nil otherwise.
func walkForCycle(start int, onDoneIdx map[int]int, state []int) []int {
	var path []int
	cur := start
	for {
		switch state[cur] {
		case stateVisiting:
			pos := indexOfInt(path, cur)
			cycle := append([]int(nil), path[pos:]...)
			markFinished(path, state)
			return cycle
		case stateFinished:
			markFinished(path, state)
			return nil
		}
		state[cur] = stateVisiting
		path = append(path, cur)
		next, ok := onDoneIdx[cur]
		if !ok {
			markFinished(path, state)
			return nil
		}
		cur = next
	}
}

// markFinished marks every node on path as fully resolved: no cycle through any of them remains
// to be found by a later walk.
func markFinished(path []int, state []int) {
	for _, node := range path {
		state[node] = stateFinished
	}
}

// indexOfInt returns the first position of v in s, or -1 if s does not contain v.
func indexOfInt(s []int, v int) int {
	for i, x := range s {
		if x == v {
			return i
		}
	}
	return -1
}

// minInt returns the smallest value in a non-empty slice.
func minInt(values []int) int {
	m := values[0]
	for _, v := range values[1:] {
		if v < m {
			m = v
		}
	}
	return m
}
