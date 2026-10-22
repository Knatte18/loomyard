// graph.go builds a plan's card by ref matrix and derives the card dependency graph from it: direct edges, their transitive reduction and each card's depth.

package main

import (
	"slices"
	"sort"

	"github.com/Knatte18/loomyard/internal/planparser"
)

// mark is one matrix cell: what a card does with a ref.
type mark byte

const (
	markNone   mark = 0
	markUse    mark = 'U'
	markChange mark = 'C'
)

// edgeKind is how strongly a later card depends on an earlier one.
type edgeKind int

const (
	edgeNone edgeKind = iota
	// edgeUse is a dependency where every overlapping pair of refs has a use on one side.
	edgeUse
	// edgeChange is a dependency where both cards change an overlapping ref.
	edgeChange
)

// cardGraph is a plan's card by ref matrix and the dependency graph derived from it, cards indexed in plan order.
type cardGraph struct {
	cards []planparser.Card
	// columns is every ref any card names, keyed canonically and sorted.
	columns []string
	// cells holds one map per card, from column to that card's mark.
	cells []map[string]mark
	// direct[i][j], for i < j, is the kind of card j's dependency on card i.
	direct [][]edgeKind
	// reduced[i][j] reports whether the edge i -> j survives transitive reduction.
	reduced [][]bool
	// depth[i] is the number of cards on the longest chain ending at card i, itself included.
	depth []int
}

// buildCardGraph derives the matrix and dependency graph of plan.
// A card changes every ref under its type labels and uses every ref under its Uses label alone,
// and a later card depends on an earlier one when an overlapping pair of their refs has a change on at least one side.
func buildCardGraph(plan *planparser.Plan) cardGraph {
	g := cardGraph{cards: plan.Cards}
	seen := map[string]bool{}
	for _, card := range plan.Cards {
		cells := map[string]mark{}
		for _, ref := range card.Uses {
			cells[columnKey(plan, ref)] = markUse
		}
		for _, ref := range card.Targets {
			cells[columnKey(plan, ref)] = markChange
		}
		for column := range cells {
			if !seen[column] {
				seen[column] = true
				g.columns = append(g.columns, column)
			}
		}
		g.cells = append(g.cells, cells)
	}
	sort.Strings(g.columns)

	n := len(plan.Cards)
	g.direct = make([][]edgeKind, n)
	for i := range n {
		g.direct[i] = make([]edgeKind, n)
		for j := i + 1; j < n; j++ {
			g.direct[i][j] = dependencyKind(g.cells[i], g.cells[j], plan)
		}
	}
	g.reduced = transitiveReduction(g.direct)
	g.depth = longestChainDepths(g.direct)
	return g
}

// columnKey returns the matrix column a card ref falls in.
// A ref naming a member, a glyph or a `plan:` handle, is keyed by its canonical glyph string, so a handle and the glyph it becomes share a column;
// any other ref is its own key.
func columnKey(plan *planparser.Plan, ref string) string {
	if member, ok := planparser.RefMember(plan, ref); ok {
		return member
	}
	return ref
}

// dependencyKind returns the strongest dependency between two cards' cells:
// edgeChange when both change an overlapping pair of refs, edgeUse when one changes and the other only uses it, and edgeNone otherwise.
func dependencyKind(earlier, later map[string]mark, plan *planparser.Plan) edgeKind {
	kind := edgeNone
	for earlierColumn, earlierMark := range earlier {
		for laterColumn, laterMark := range later {
			if earlierMark != markChange && laterMark != markChange {
				continue
			}
			if !columnsOverlap(earlierColumn, laterColumn, plan) {
				continue
			}
			if earlierMark == markChange && laterMark == markChange {
				return edgeChange
			}
			kind = edgeUse
		}
	}
	return kind
}

// columnsOverlap reports whether two columns name the same code:
// they are equal, or both are member glyphs of one unit and one names a member of the other, as `pkg#T` and `pkg#T.M` do.
func columnsOverlap(a, b string, plan *planparser.Plan) bool {
	if a == b {
		return true
	}
	if !plan.GlyphLanguage() {
		return false
	}
	ga, errA := planparser.ParseRef(plan, a)
	gb, errB := planparser.ParseRef(plan, b)
	if errA != nil || errB != nil || ga.IsSelf() || gb.IsSelf() || ga.Unit != gb.Unit {
		return false
	}
	chainA := append(slices.Clone(ga.Owner), ga.Name)
	chainB := append(slices.Clone(gb.Owner), gb.Name)
	if len(chainA) > len(chainB) {
		chainA, chainB = chainB, chainA
	}
	return slices.Equal(chainA, chainB[:len(chainA)])
}

// transitiveReduction keeps each edge i -> j of direct that no longer path from i to j implies.
// direct's edges all run from a lower to a higher index, so the graph is acyclic and index order is a topological order.
func transitiveReduction(direct [][]edgeKind) [][]bool {
	n := len(direct)
	reach := make([][]bool, n)
	for i := n - 1; i >= 0; i-- {
		reach[i] = make([]bool, n)
		for k := i + 1; k < n; k++ {
			if direct[i][k] == edgeNone {
				continue
			}
			reach[i][k] = true
			for j := k + 1; j < n; j++ {
				reach[i][j] = reach[i][j] || reach[k][j]
			}
		}
	}

	reduced := make([][]bool, n)
	for i := range n {
		reduced[i] = make([]bool, n)
		for j := i + 1; j < n; j++ {
			if direct[i][j] == edgeNone {
				continue
			}
			implied := false
			for k := i + 1; k < j && !implied; k++ {
				implied = direct[i][k] != edgeNone && reach[k][j]
			}
			reduced[i][j] = !implied
		}
	}
	return reduced
}

// longestChainDepths returns, per card, the number of cards on the longest dependency chain ending at it.
// Cards of equal depth have no path between them, so they could run in parallel.
func longestChainDepths(direct [][]edgeKind) []int {
	depth := make([]int, len(direct))
	for j := range direct {
		depth[j] = 1
		for i := range j {
			if direct[i][j] != edgeNone && depth[i]+1 > depth[j] {
				depth[j] = depth[i] + 1
			}
		}
	}
	return depth
}
