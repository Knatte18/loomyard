// sequence.go derives a plan's execution order from card-level Targets/Uses ref matching rather than
// from a batchifier's own declared order: internal/batcher owns GROUPING (which cards land in the
// same batch), and this file owns SEQUENCING (the order those batches run in) — a separate policy,
// kept out of internal/batcher per the Batcher Registry+Config Invariant, and out of internal/webstercli
// because it is pure, deterministic logic with no I/O seam to justify living at the CLI layer.

package websterengine

import (
	"container/heap"
	"sort"
	"strings"

	"github.com/Knatte18/loomyard/internal/batcher"
)

// Cycle names one strongly-connected component SequenceBatches condensed into a single execution
// group because its member batches depend on each other in both directions.
// Batches holds the cycle's member batch numbers in ascending order.
type Cycle struct {
	Batches []int
}

// Warning returns the single operator-facing line a caller surfaces for this cycle: it names every
// member batch number, states that the members were condensed into one execution group kept in
// declared order, and that mutually-dependent cards are worth checking in the plan.
// Warning is the sole renderer of this sentence — Run calls it rather than formatting its own
// string — so the wording lives in exactly one place and is directly unit-testable.
// It never panics on a zero-value Cycle or on an empty Batches slice.
func (c Cycle) Warning() string {
	numbers := make([]string, len(c.Batches))
	for i, n := range c.Batches {
		numbers[i] = itoa(n)
	}
	return "batches " + strings.Join(numbers, ", ") + " form a dependency cycle and were condensed into one execution group, kept in declared order; check mutually-dependent cards in the plan"
}

// itoa converts a non-negative int to its decimal string form without pulling in strconv for a
// single call site — kept local because sequence.go's import list is deliberately minimal.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	pos := len(buf)
	for n > 0 {
		pos--
		buf[pos] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}

// sortKey is a vertex's ordering identity: a batch's declared number paired with its index in the
// input slice.
// The index half keeps every comparison total even when two batches carry the same number (a
// zero-card batch yields number 0) — every ordering decision in this file compares this pair, never
// a bare number and never a map iteration.
type sortKey struct {
	number int
	index  int
}

// less reports whether k sorts before other: primarily by number, then by index.
func (k sortKey) less(other sortKey) bool {
	if k.number != other.number {
		return k.number < other.number
	}
	return k.index < other.index
}

// SequenceBatches reorders batches into a dependency-respecting execution order derived from each
// card's Targets/Uses refs, and reports every dependency cycle it condensed along the way.
//
// Three properties every caller depends on:
//
//   - Purity and determinism: same input, same output, on every call in every process. Ordering
//     never consults Go map iteration order; every comparison keys on (batch number, input index).
//   - Length preservation: len(returned) == len(batches) always, and the returned slice holds
//     exactly the input's batch values reordered — never filtered, merged, or synthesized. A nil or
//     empty input returns an empty (or nil) slice and no cycles, never a panic.
//   - No-op on an already dependency-correct plan: a plan whose declared order already satisfies
//     every derived edge sequences to exactly that declared order.
//
// SequenceBatches never mutates the caller's input slice — it builds and returns a fresh one.
func SequenceBatches(batches []batcher.Batch) ([]batcher.Batch, []Cycle) {
	n := len(batches)
	if n == 0 {
		return nil, nil
	}

	keys := make([]sortKey, n)
	for i := range batches {
		number, _ := batchIdentity(batches[i])
		keys[i] = sortKey{number: number, index: i}
	}

	edges := deriveEdges(batches)
	components := stronglyConnected(edges)
	order := orderComponents(components, edges, keys)

	out := make([]batcher.Batch, 0, n)
	var cycles []Cycle
	for _, comp := range order {
		members := append([]int(nil), comp...)
		sort.Slice(members, func(a, b int) bool {
			return keys[members[a]].less(keys[members[b]])
		})
		for _, v := range members {
			out = append(out, batches[v])
		}
		if len(members) > 1 {
			numbers := make([]int, len(members))
			for i, v := range members {
				numbers[i], _ = batchIdentity(batches[v])
			}
			sort.Ints(numbers)
			cycles = append(cycles, Cycle{Batches: numbers})
		}
	}

	return out, cycles
}

// deriveEdges builds the vertex adjacency list — one entry per input batch index — by comparing
// every ordered pair of cards drawn from different batches.
// Each vertex's successor list is deduplicated and sorted ascending, which is what makes
// stronglyConnected's traversal deterministic.
//
// An edge i -> j is added when:
//   - batch j's card b has a Uses entry matching batch i's card a's Targets entry (producer before
//     consumer); or
//   - batch i's card a and batch j's card b share a Targets entry AND a.Number < b.Number (declared
//     card order settles two writers of the same ref).
//
// A shared entry between two cards' Uses never produces an edge — a read creates no ordering
// against another read. Refs that are empty or whitespace-only after strings.TrimSpace are ignored
// on both sides of every comparison. Ref classification (symbol-shaped vs. path-shaped) is not
// consulted: both kinds participate identically.
func deriveEdges(batches []batcher.Batch) [][]int {
	n := len(batches)
	adj := make([][]int, n)
	seen := make([]map[int]bool, n)
	for i := range seen {
		seen[i] = map[int]bool{}
	}

	addEdge := func(from, to int) {
		if from == to {
			return
		}
		if seen[from][to] {
			return
		}
		seen[from][to] = true
		adj[from] = append(adj[from], to)
	}

	for i := range batches {
		for _, a := range batches[i].Cards {
			for j := range batches {
				if i == j {
					continue
				}
				for _, b := range batches[j].Cards {
					if refsIntersect(b.Uses, a.Targets) {
						addEdge(i, j)
					}
					if a.Number < b.Number && refsIntersect(a.Targets, b.Targets) {
						addEdge(i, j)
					}
				}
			}
		}
	}

	for i := range adj {
		sort.Ints(adj[i])
	}
	return adj
}

// refsIntersect reports whether left and right share at least one entry, compared by exact string
// equality after trimming whitespace; an entry that trims to empty is ignored on both sides.
func refsIntersect(left, right []string) bool {
	set := make(map[string]bool, len(left))
	for _, ref := range left {
		ref = strings.TrimSpace(ref)
		if ref == "" {
			continue
		}
		set[ref] = true
	}
	if len(set) == 0 {
		return false
	}
	for _, ref := range right {
		ref = strings.TrimSpace(ref)
		if ref == "" {
			continue
		}
		if set[ref] {
			return true
		}
	}
	return false
}

// stronglyConnected runs an iterative Tarjan's algorithm over adj, returning one member-index list
// per strongly-connected component.
// Because adj's successor lists are sorted and the outer sweep runs over vertex indices in
// ascending order, the traversal — and so the component membership and discovery order — is
// deterministic.
func stronglyConnected(adj [][]int) [][]int {
	n := len(adj)
	index := make([]int, n)
	lowlink := make([]int, n)
	onStack := make([]bool, n)
	for i := range index {
		index[i] = -1
	}

	var stack []int
	var components [][]int
	nextIndex := 0

	// frame is one call activation of the recursive Tarjan walk, reified so the traversal can run
	// iteratively rather than recursively — a plan with enough batches could otherwise overflow the
	// goroutine stack on a deeply chained dependency graph.
	type frame struct {
		v       int
		childAt int
	}

	for start := 0; start < n; start++ {
		if index[start] != -1 {
			continue
		}

		callStack := []frame{{v: start, childAt: 0}}
		index[start] = nextIndex
		lowlink[start] = nextIndex
		nextIndex++
		stack = append(stack, start)
		onStack[start] = true

		for len(callStack) > 0 {
			top := &callStack[len(callStack)-1]
			v := top.v

			if top.childAt < len(adj[v]) {
				w := adj[v][top.childAt]
				top.childAt++

				switch {
				case index[w] == -1:
					index[w] = nextIndex
					lowlink[w] = nextIndex
					nextIndex++
					stack = append(stack, w)
					onStack[w] = true
					callStack = append(callStack, frame{v: w, childAt: 0})
				case onStack[w]:
					if index[w] < lowlink[v] {
						lowlink[v] = index[w]
					}
				}
				continue
			}

			// Every successor of v has been explored; pop v's frame and propagate its lowlink to its
			// parent, then close v's component if it is its own root.
			callStack = callStack[:len(callStack)-1]
			if len(callStack) > 0 {
				parent := &callStack[len(callStack)-1]
				if lowlink[v] < lowlink[parent.v] {
					lowlink[parent.v] = lowlink[v]
				}
			}

			if lowlink[v] == index[v] {
				var comp []int
				for {
					w := stack[len(stack)-1]
					stack = stack[:len(stack)-1]
					onStack[w] = false
					comp = append(comp, w)
					if w == v {
						break
					}
				}
				components = append(components, comp)
			}
		}
	}

	return components
}

// orderComponents runs Kahn's algorithm over the condensed DAG components/adj describe, returning
// the components in a deterministic topological order.
// The condensed edge set keeps only edges whose endpoints land in different components,
// deduplicated. The ready set is a min-heap keyed on each component's own lowest member sort key
// (keys, from SequenceBatches' step 1): the lowest-keyed ready component is always popped next, so
// an already dependency-correct plan sequences to exactly its declared order.
func orderComponents(components [][]int, adj [][]int, keys []sortKey) [][]int {
	compOf := make([]int, len(keys))
	for c, members := range components {
		for _, v := range members {
			compOf[v] = c
		}
	}

	compKey := make([]sortKey, len(components))
	for c, members := range components {
		best := keys[members[0]]
		for _, v := range members[1:] {
			if keys[v].less(best) {
				best = keys[v]
			}
		}
		compKey[c] = best
	}

	condensedAdj := make([]map[int]bool, len(components))
	for c := range condensedAdj {
		condensedAdj[c] = map[int]bool{}
	}
	indegree := make([]int, len(components))
	for v, succs := range adj {
		cv := compOf[v]
		for _, w := range succs {
			cw := compOf[w]
			if cv == cw {
				continue
			}
			if !condensedAdj[cv][cw] {
				condensedAdj[cv][cw] = true
				indegree[cw]++
			}
		}
	}

	condensedSucc := make([][]int, len(components))
	for c, succs := range condensedAdj {
		for w := range succs {
			condensedSucc[c] = append(condensedSucc[c], w)
		}
		sort.Ints(condensedSucc[c])
	}

	pq := &componentHeap{}
	for c := range components {
		if indegree[c] == 0 {
			heap.Push(pq, heapItem{comp: c, key: compKey[c]})
		}
	}

	order := make([][]int, 0, len(components))
	for pq.Len() > 0 {
		item := heap.Pop(pq).(heapItem)
		order = append(order, components[item.comp])
		for _, w := range condensedSucc[item.comp] {
			indegree[w]--
			if indegree[w] == 0 {
				heap.Push(pq, heapItem{comp: w, key: compKey[w]})
			}
		}
	}

	return order
}

// heapItem is one ready component waiting in orderComponents' min-heap, carrying the sort key its
// ordering decision is made on.
type heapItem struct {
	comp int
	key  sortKey
}

// componentHeap is a container/heap.Interface min-heap of heapItem, ordered by heapItem.key.
type componentHeap []heapItem

func (h componentHeap) Len() int { return len(h) }

func (h componentHeap) Less(i, j int) bool { return h[i].key.less(h[j].key) }

func (h componentHeap) Swap(i, j int) { h[i], h[j] = h[j], h[i] }

// Push implements heap.Interface by appending x, which must be a heapItem.
func (h *componentHeap) Push(x any) {
	*h = append(*h, x.(heapItem))
}

// Pop implements heap.Interface by removing and returning the last element in the underlying slice
// — container/heap has already moved the minimum element there before calling Pop.
func (h *componentHeap) Pop() any {
	old := *h
	n := len(old)
	item := old[n-1]
	*h = old[:n-1]
	return item
}
