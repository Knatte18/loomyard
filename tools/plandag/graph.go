// graph.go builds a plan's card by ref matrix and derives the card dependency graph from it: direct edges, their transitive reduction and each card's depth.

package main

import (
	pathpkg "path"
	"slices"
	"sort"
	"strings"

	"github.com/Knatte18/loomyard/internal/planparser"
)

// mark is one matrix cell: what a card does with a ref.
type mark byte

const (
	markNone   mark = 0
	markUse    mark = 'U'
	markChange mark = 'C'
	// markExtends is a ref the card only adds to, named under an Extends label.
	markExtends mark = 'E'
)

// refKind returns the planparser.RefKind m stands for, which is 0 for markNone.
func (m mark) refKind() planparser.RefKind {
	switch m {
	case markUse:
		return planparser.RefUse
	case markChange:
		return planparser.RefChange
	case markExtends:
		return planparser.RefExtends
	}
	return 0
}

// markOf returns the mark a planparser.RefKind is drawn as.
func markOf(kind planparser.RefKind) mark {
	switch kind {
	case planparser.RefUse:
		return markUse
	case planparser.RefChange:
		return markChange
	case planparser.RefExtends:
		return markExtends
	}
	return markNone
}

// strength orders marks where two refs of one card fall in one column: a change outranks an Extends, which outranks a use.
func (m mark) strength() int {
	switch m {
	case markChange:
		return 3
	case markExtends:
		return 2
	case markUse:
		return 1
	}
	return 0
}

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
// Each cell is the planparser.CardRefKind of the card's ref: a change under a type label, an Extends under that label alone, or a use under Uses alone.
// A later card depends on an earlier one when planparser.DependencyEdge holds for an overlapping pair of their refs.
// With additiveGuess set, a pair with no use on either side makes no edge when either column is guessed additive, unless some card labels that column Extends.
func buildCardGraph(plan *planparser.Plan, additiveGuess bool) cardGraph {
	g := cardGraph{cards: plan.Cards}
	seen := map[string]bool{}
	labelled := map[string]bool{}
	for _, card := range plan.Cards {
		cells := map[string]mark{}
		for _, ref := range slices.Concat(card.Uses, card.Targets) {
			column := columnKey(plan, ref)
			if m := markOf(planparser.CardRefKind(card, ref)); m.strength() > cells[column].strength() {
				cells[column] = m
			}
		}
		for column, m := range cells {
			if m == markExtends {
				labelled[column] = true
			}
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
			g.direct[i][j] = dependencyKind(plan.Cards[i], plan.Cards[j], g.cells[i], g.cells[j], labelled, plan, additiveGuess)
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

// dependencyKind returns the strongest dependency between two cards' cells.
// Each overlapping pair of columns makes an edge when planparser.DependencyEdge holds for its marks:
// edgeChange when neither side is a use, edgeUse otherwise, and the pairs that make none leave edgeNone.
// With guessAdditive set, a pair with no use on either side is skipped when either column is guessed additive, a column in labelled excepted so a labelled ref keeps its label.
func dependencyKind(earlier, later planparser.Card, earlierCells, laterCells map[string]mark, labelled map[string]bool, plan *planparser.Plan, guessAdditive bool) edgeKind {
	guessed := func(column string) bool {
		return guessAdditive && !labelled[column] && additiveGuess(column)
	}
	kind := edgeNone
	for earlierColumn, earlierMark := range earlierCells {
		for laterColumn, laterMark := range laterCells {
			if !columnsOverlap(earlierColumn, laterColumn, plan) {
				continue
			}
			if !planparser.DependencyEdge(earlier, earlierMark.refKind(), later, laterMark.refKind()) {
				continue
			}
			if earlierMark == markUse || laterMark == markUse {
				kind = edgeUse
				continue
			}
			if guessed(earlierColumn) || guessed(laterColumn) {
				continue
			}
			return edgeChange
		}
	}
	return kind
}

// additiveGuess reports whether a matrix column is one that cards usually only add to, so two cards changing it need no order:
// a path or self glyph whose file is a doc.go, anything under docs/, PATTERN.md or anything under contracts/specs/,
// and a member glyph whose last dotted name segment starts with wire or is addWeftVerbs.
// It is a heuristic for the report's measurement and never a statement that the change is safe to reorder.
func additiveGuess(column string) bool {
	path, isPath := planparser.PathEntryPath(column)
	if !isPath {
		member, err := planparser.ParseRef(&planparser.Plan{}, column)
		if err != nil {
			path, isPath = column, !strings.Contains(column, "#")
		} else if !member.IsSelf() {
			return strings.HasPrefix(member.Name, "wire") || member.Name == "addWeftVerbs"
		}
	}
	if !isPath {
		return false
	}
	return pathpkg.Base(path) == "doc.go" ||
		strings.HasPrefix(path, "docs/") ||
		path == "PATTERN.md" ||
		strings.HasPrefix(path, "contracts/specs/")
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
