// recipes.go writes the Recipes section, which compares the counted runs by recipe.
// It gives the cost and span per run, and the cost per changed line of the commits they landed.

package main

import (
	"fmt"
	"io"
	"maps"
	"slices"
	"strings"
	"time"
)

// noRecipe is the recipe a run is listed under when no archived seed names one, such as a run in flight.
const noRecipe = "unknown"

// recipeRow sums the counted runs of one recipe.
type recipeRow struct {
	runs int
	cost Cost
	span time.Duration
	// landed counts the runs with a landed commit; lines counts changed lines of those commits; landedCost sums their cost.
	landed     int
	lines      int
	landedCost Cost
}

// writeRecipes writes one row per recipe of the runs, by name, then names each run without a landed commit and why.
// perRun holds each run's segments; a run's cost is their total, its orch charge included.
// The cost per changed line divides the cost of the runs that landed a commit by the lines those commits changed.
func writeRecipes(w io.Writer, runs []RunTally, perRun [][]Segment) {
	rows := map[string]*recipeRow{}
	var missing []string
	for i, run := range runs {
		row := rows[run.Recipe()]
		if row == nil {
			row = &recipeRow{}
			rows[run.Recipe()] = row
		}
		cost := totalCost(perRun[i])
		row.runs++
		row.cost.add(cost)
		row.span += run.AwakeSpan()
		switch {
		case run.Landing == nil:
			missing = append(missing, run.Slug+" ("+noShedRecord+")")
		case run.Landing.SHA == "":
			missing = append(missing, run.Slug+" ("+run.Landing.Missing+")")
		default:
			row.landed++
			row.lines += run.Landing.Lines
			row.landedCost.add(cost)
		}
	}
	fmt.Fprintf(w, "## Recipes\n\n")
	fmt.Fprintln(w, "| recipe | runs | cost per run | span per run | landed runs | changed lines per run | cost per changed line |")
	fmt.Fprintln(w, "|---|---|---|---|---|---|---|")
	for _, name := range slices.Sorted(maps.Keys(rows)) {
		row := rows[name]
		linesPerRun, perLine := "n/a", "n/a"
		if row.landed > 0 {
			linesPerRun = fmt.Sprintf("%.0f", float64(row.lines)/float64(row.landed))
		}
		if row.lines > 0 {
			perLine = row.landedCost.share(1 / float64(row.lines)).precise()
		}
		span := (row.span / time.Duration(row.runs)).Round(time.Second)
		fmt.Fprintf(w, "| %s | %d | %s | %s | %d | %s | %s |\n", name, row.runs, row.cost.share(1/float64(row.runs)), span, row.landed, linesPerRun, perLine)
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "A run's landed commit is the newest commit titled after the change description archived at its tag and committed within its span; its changed lines are the lines it added and deleted.")
	if len(missing) > 0 {
		fmt.Fprintf(w, "No landed commit: %s.\n", strings.Join(missing, ", "))
	}
	fmt.Fprintln(w)
}
