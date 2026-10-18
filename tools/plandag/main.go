// Command plandag reads a plan directory and draws its cards as a dependency graph, showing which cards the plan could run in parallel.
//
//	go run ./tools/plandag <plan-dir>
//	go run ./tools/plandag -out .scratch/dag ../some-pair/_lyx/plan
//
// The two launchers beside this file, plandag.sh and plandag.cmd, are the intended entry points.
// Each takes the plan directory, makes it absolute and runs from the repository root, writing into .scratch/plandag with fixed file names overwritten per run.
//
// The plan is read through internal/planparser.
// The tool builds a matrix with one row per card and one column per ref any card names, a glyph, `plan:` handle, file or directory.
// A member ref, a glyph or `plan:` handle, is keyed by its canonical glyph, so a handle a card creates and the glyph a later card names share a column.
// A cell is C when the card changes the ref, naming it under a type label, and U when it names the ref only under Uses.
//
// A later card depends on an earlier one when a column of each overlaps and at least one of the two cells is C;
// two uses never conflict.
// Two columns overlap when equal, or when both are member glyphs of one unit and one names a member of the other, as `pkg#T` and `pkg#T.M` do.
// Edges run in plan order, earlier card to later, so the graph is acyclic.
// Cards with no path between them are independent: the report's levels table groups cards by the longest chain ending at each, and one level's cards could run in parallel.
//
// -out receives three files:
// plandag.md, the report holding the levels table, each card's direct dependencies and the matrix;
// plandag.dot, the transitively reduced graph, an edge drawn dashed when one of its cards only uses the shared ref;
// and plandag.svg, rendered with `dot -Tsvg` when dot is on PATH.
// Without dot on PATH the tool says so and names the command that renders the DOT file.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/planparser"
)

func main() {
	out := flag.String("out", ".scratch/plandag", "directory receiving plandag.md, plandag.dot and plandag.svg")
	flag.Usage = func() {
		fmt.Fprintln(flag.CommandLine.Output(), "usage: plandag [-out dir] <plan-dir>")
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(2)
	}

	dotBinary, err := exec.LookPath("dot")
	if err != nil {
		dotBinary = ""
	}
	if err := run(flag.Arg(0), *out, dotBinary, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "plandag:", err)
		os.Exit(1)
	}
}

// run parses the plan at planDir and writes its report and DOT graph into outDir, printing each written path to stdout.
// It renders the SVG through dotBinary, and with dotBinary empty prints that no SVG was rendered and how to render it.
func run(planDir, outDir, dotBinary string, stdout io.Writer) error {
	plan, err := planparser.ParsePlan(planDir)
	if err != nil {
		return err
	}
	graph := buildCardGraph(plan)

	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}
	reportPath := filepath.Join(outDir, "plandag.md")
	if err := os.WriteFile(reportPath, []byte(renderMarkdown(graph, planDir)), 0o644); err != nil {
		return err
	}
	fmt.Fprintln(stdout, reportPath)
	dotPath := filepath.Join(outDir, "plandag.dot")
	if err := os.WriteFile(dotPath, []byte(renderDOT(graph)), 0o644); err != nil {
		return err
	}
	fmt.Fprintln(stdout, dotPath)

	svgPath := filepath.Join(outDir, "plandag.svg")
	if dotBinary == "" {
		fmt.Fprintf(stdout, "dot is not on PATH, so no SVG was rendered; render it with: dot -Tsvg -o %s %s\n", svgPath, dotPath)
		return nil
	}
	render := exec.Command(dotBinary, "-Tsvg", "-o", svgPath, dotPath)
	if output, err := render.CombinedOutput(); err != nil {
		return fmt.Errorf("dot -Tsvg: %w: %s", err, output)
	}
	fmt.Fprintln(stdout, svgPath)
	return nil
}
