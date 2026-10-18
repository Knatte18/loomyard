package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writePlan writes a plan directory holding the overview and the cards, each card a file name and its body.
func writePlan(t *testing.T, cardIndex string, cards map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	overview := "---\nformat: 5\napproved: true\nlanguage: go\n---\n\n# Plan: fixture\n\nFixture plan.\n\n## Card Index\n\n" + cardIndex
	if err := os.WriteFile(filepath.Join(dir, "00-overview.md"), []byte(overview), 0o644); err != nil {
		t.Fatal(err)
	}
	for name, body := range cards {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestRun_WritesMatrixLevelsAndReducedGraph(t *testing.T) {
	t.Parallel()
	planDir := writePlan(t, `1 — type — create type T
2 — method — edit T.M
3 — type-user — edit F, using T
4 — type-reader — edit G, using T
5 — reader-first — edit H, using K
6 — changes-read — edit K
`, map[string]string{
		"01-type.md":         "# Card 1 — type\n\n**Create:**\n- `plan:internal/a#T`\n\n**Intent:** x\n",
		"02-method.md":       "# Card 2 — method\n\n**Edit:**\n- `internal/a#T.M`\n\n**Intent:** x\n",
		"03-type-user.md":    "# Card 3 — type-user\n\n**Edit:**\n- `internal/b#F`\n\n**Uses:**\n- `internal/a#T`\n\n**Intent:** x\n",
		"04-type-reader.md":  "# Card 4 — type-reader\n\n**Edit:**\n- `internal/c#G`\n\n**Uses:**\n- `internal/a#T`\n\n**Intent:** x\n",
		"05-reader-first.md": "# Card 5 — reader-first\n\n**Edit:**\n- `internal/d#H`\n\n**Uses:**\n- `internal/e#K`\n\n**Intent:** x\n",
		"06-changes-read.md": "# Card 6 — changes-read\n\n**Edit:**\n- `internal/e#K`\n\n**Intent:** x\n",
	})
	outDir := filepath.Join(t.TempDir(), "out")

	var stdout strings.Builder
	if err := run(planDir, outDir, "", &stdout); err != nil {
		t.Fatalf("run() = %v; want nil", err)
	}

	reportPath := filepath.Join(outDir, "plandag.md")
	dotPath := filepath.Join(outDir, "plandag.dot")
	svgPath := filepath.Join(outDir, "plandag.svg")
	wantStdout := reportPath + "\n" + dotPath + "\n" +
		"dot is not on PATH, so no SVG was rendered; render it with: dot -Tsvg -o " + svgPath + " " + dotPath + "\n"
	if got := stdout.String(); got != wantStdout {
		t.Errorf("run() stdout = %q; want %q", got, wantStdout)
	}
	if _, err := os.Stat(svgPath); !os.IsNotExist(err) {
		t.Errorf("os.Stat(%s) = %v; want not exist", svgPath, err)
	}

	// The edges 1 -> 3 and 1 -> 4 are implied through card 2 and dropped, and the two readers of T, cards 3 and 4, get no edge.
	wantDOT := `digraph plan {
	node [shape=box];
	1 [label="1-type", tooltip="create type T"];
	2 [label="2-method", tooltip="edit T.M"];
	3 [label="3-type-user", tooltip="edit F, using T"];
	4 [label="4-type-reader", tooltip="edit G, using T"];
	5 [label="5-reader-first", tooltip="edit H, using K"];
	6 [label="6-changes-read", tooltip="edit K"];
	1 -> 2;
	2 -> 3 [style=dashed];
	2 -> 4 [style=dashed];
	5 -> 6 [style=dashed];
}
`
	if got := readFile(t, dotPath); got != wantDOT {
		t.Errorf("plandag.dot = %q; want %q", got, wantDOT)
	}

	wantReport := "# Card DAG: " + planDir + `

6 cards, 6 direct dependencies, critical path 3 cards, widest level 2 cards.

## Levels

A card's level is the number of cards on the longest chain ending at it; cards of one level have no path between them.

| Level | Cards |
|---|---|
| 1 | 1-type, 5-reader-first |
| 2 | 2-method, 6-changes-read |
| 3 | 3-type-user, 4-type-reader |

## Dependencies

Each card's direct dependencies, ` + "`C`" + ` where both cards change an overlapping ref, ` + "`U`" + ` where one only uses it.

| Card | After |
|---|---|
| 1-type |  |
| 2-method | 1-type (C) |
| 3-type-user | 1-type (U), 2-method (U) |
| 4-type-reader | 1-type (U), 2-method (U) |
| 5-reader-first |  |
| 6-changes-read | 5-reader-first (U) |

## Matrix

` + "`C` the card changes the ref, `U` it only uses it." + `

| Card | ` + "`internal/a#T` | `internal/a#T.M` | `internal/b#F` | `internal/c#G` | `internal/d#H` | `internal/e#K`" + ` |
|---|---|---|---|---|---|---|
| 1-type | C |  |  |  |  |  |
| 2-method |  | C |  |  |  |  |
| 3-type-user | U |  | C |  |  |  |
| 4-type-reader | U |  |  | C |  |  |
| 5-reader-first |  |  |  |  | C | U |
| 6-changes-read |  |  |  |  |  | C |
`
	if got := readFile(t, reportPath); got != wantReport {
		t.Errorf("plandag.md = %q; want %q", got, wantReport)
	}
}

func TestRun_UnparseablePlanFailsWithoutOutput(t *testing.T) {
	t.Parallel()
	outDir := filepath.Join(t.TempDir(), "out")

	var stdout strings.Builder
	if err := run(t.TempDir(), outDir, "", &stdout); err == nil {
		t.Error("run(empty dir) = nil; want an error naming the missing overview")
	}
	if _, err := os.Stat(outDir); !os.IsNotExist(err) {
		t.Errorf("os.Stat(%s) = %v; want not exist", outDir, err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
