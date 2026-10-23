// render.go writes a card graph as the markdown report and as Graphviz DOT.

package main

import (
	"fmt"
	"strings"
)

// renderMarkdown returns the report of g for the plan at planDir:
// a summary line, the levels table of cards that could run in parallel, each card's direct dependencies and the card by ref matrix.
// A non-nil guessed, the same plan's graph under the additive guess, adds one line after the summary comparing the two critical paths and best speedups.
func renderMarkdown(g cardGraph, guessed *cardGraph, planDir string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Card DAG: %s\n\n", planDir)

	levels := map[int][]string{}
	criticalPath, directEdges := 0, 0
	for i, card := range g.cards {
		levels[g.depth[i]] = append(levels[g.depth[i]], card.ID())
		criticalPath = max(criticalPath, g.depth[i])
		for j := i + 1; j < len(g.cards); j++ {
			if g.direct[i][j] != edgeNone {
				directEdges++
			}
		}
	}
	widest := 0
	for _, ids := range levels {
		widest = max(widest, len(ids))
	}
	fmt.Fprintf(&b, "%d cards, %d direct dependencies, critical path %d cards, widest level %d cards.\n\n", len(g.cards), directEdges, criticalPath, widest)
	if guessed != nil {
		plainPath, guessedPath := criticalPathLength(g), criticalPathLength(*guessed)
		fmt.Fprintf(&b, "Critical path %d cards, best speedup %.2fx; with the additive guess %d cards, best speedup %.2fx.\n\n",
			plainPath, bestSpeedup(g, plainPath), guessedPath, bestSpeedup(*guessed, guessedPath))
	}

	b.WriteString("## Levels\n\n")
	b.WriteString("A card's level is the number of cards on the longest chain ending at it; cards of one level have no path between them.\n\n")
	b.WriteString("| Level | Cards |\n|---|---|\n")
	for level := 1; level <= criticalPath; level++ {
		fmt.Fprintf(&b, "| %d | %s |\n", level, strings.Join(levels[level], ", "))
	}

	b.WriteString("\n## Dependencies\n\n")
	b.WriteString("Each card's direct dependencies, `C` where both cards change an overlapping ref, `U` where one only uses it.\n\n")
	b.WriteString("| Card | After |\n|---|---|\n")
	for j, card := range g.cards {
		var after []string
		for i := range j {
			if kind := g.direct[i][j]; kind != edgeNone {
				after = append(after, fmt.Sprintf("%s (%s)", g.cards[i].ID(), kindMark(kind)))
			}
		}
		fmt.Fprintf(&b, "| %s | %s |\n", card.ID(), strings.Join(after, ", "))
	}

	b.WriteString("\n## Matrix\n\n")
	b.WriteString("`C` the card changes the ref, `U` it only uses it.\n\n")
	b.WriteString("| Card |")
	for _, column := range g.columns {
		fmt.Fprintf(&b, " `%s` |", column)
	}
	b.WriteString("\n|---|")
	b.WriteString(strings.Repeat("---|", len(g.columns)))
	b.WriteString("\n")
	for i, card := range g.cards {
		fmt.Fprintf(&b, "| %s |", card.ID())
		for _, column := range g.columns {
			cell := ""
			if m := g.cells[i][column]; m != markNone {
				cell = string(m)
			}
			fmt.Fprintf(&b, " %s |", cell)
		}
		b.WriteString("\n")
	}
	return b.String()
}

// criticalPathLength returns the number of cards on g's longest dependency chain.
func criticalPathLength(g cardGraph) int {
	longest := 0
	for _, depth := range g.depth {
		longest = max(longest, depth)
	}
	return longest
}

// bestSpeedup returns g's card count divided by its critical path, the speedup of running every level in parallel.
func bestSpeedup(g cardGraph, criticalPath int) float64 {
	if criticalPath == 0 {
		return 0
	}
	return float64(len(g.cards)) / float64(criticalPath)
}

// kindMark returns the matrix letter naming an edge kind.
func kindMark(kind edgeKind) string {
	if kind == edgeChange {
		return string(markChange)
	}
	return string(markUse)
}

// renderDOT returns g's transitively reduced graph as Graphviz DOT, one node per card labelled with its ID and its summary as tooltip.
// An edge where both cards change an overlapping ref is solid, and one where a card only uses the ref is dashed.
func renderDOT(g cardGraph) string {
	var b strings.Builder
	b.WriteString("digraph plan {\n")
	b.WriteString("\tnode [shape=box];\n")
	for _, card := range g.cards {
		fmt.Fprintf(&b, "\t%d [label=%s, tooltip=%s];\n", card.Number, dotString(card.ID()), dotString(card.Summary))
	}
	for i, from := range g.cards {
		for j := i + 1; j < len(g.cards); j++ {
			if !g.reduced[i][j] {
				continue
			}
			style := ""
			if g.direct[i][j] == edgeUse {
				style = " [style=dashed]"
			}
			fmt.Fprintf(&b, "\t%d -> %d%s;\n", from.Number, g.cards[j].Number, style)
		}
	}
	b.WriteString("}\n")
	return b.String()
}

// dotString quotes s as a DOT string literal.
func dotString(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) + `"`
}
