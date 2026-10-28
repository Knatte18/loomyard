//go:build tier0

// agentmessaging_test.go pins PATTERN-agent-messaging: no stencil, prompt fixture or Go message text names Claude Code's SendMessage tool,
// so a role that has a parent can run on an engine without that tool.

package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
)

// engineTool is the name of the engine-specific messaging tool no prompt may name.
const engineTool = "SendMessage"

// engineToolAllowed maps a repository-relative stencil path to the one line text admitted there.
// A row is made only for a stencil whose role's spec sets ForkSubagents, since such a role addresses a fork in-session;
// the scan cannot read the spec from a stencil, so review holds that half.
var engineToolAllowed = map[string]string{
	"contracts/stencils/webster/webster-template-master.md": "to that batch's fork by `SendMessage`, addressed by the agent id its completion notification carried",
}

// engineToolHits returns one "line N: SendMessage" entry per line of content naming the tool, skipping a line the allowlist admits for rel.
func engineToolHits(rel, content string) []string {
	admitted, hasAdmitted := engineToolAllowed[rel]
	var hits []string
	for i, line := range strings.Split(content, "\n") {
		if !strings.Contains(line, engineTool) {
			continue
		}
		if hasAdmitted && strings.Contains(line, admitted) {
			continue
		}
		hits = append(hits, fmt.Sprintf("line %d: %s", i+1, engineTool))
	}
	return hits
}

// goLiteralHits returns one "line N: SendMessage" entry per string literal of the Go source src that names the tool.
// A comment naming the tool is no hit.
func goLiteralHits(t *testing.T, rel string, src []byte) []string {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, rel, src, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", rel, err)
	}
	var hits []string
	ast.Inspect(file, func(n ast.Node) bool {
		lit, ok := n.(*ast.BasicLit)
		if ok && lit.Kind == token.STRING && strings.Contains(lit.Value, engineTool) {
			hits = append(hits, fmt.Sprintf("line %d: %s", fset.Position(lit.Pos()).Line, engineTool))
		}
		return true
	})
	return hits
}

// testdataPromptRoots lists, relative to root and slash-separated, every testdata directory under root's internal directory.
// scankit skips a testdata directory below a walk root, so each one is handed to a walk as a root of its own.
func testdataPromptRoots(t *testing.T, root string) []string {
	t.Helper()
	var roots []string
	err := filepath.WalkDir(filepath.Join(root, "internal"), func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && strings.Contains(d.Name(), "testdata") {
			rel, err := filepath.Rel(root, p)
			if err != nil {
				return err
			}
			roots = append(roots, filepath.ToSlash(rel))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("list testdata directories under %s: %v", root, err)
	}
	return roots
}

func TestAgentMessaging_NoEngineTool(t *testing.T) {
	t.Parallel()

	t.Run("FixtureRows", func(t *testing.T) {
		t.Parallel()
		const masterStencil = "contracts/stencils/webster/webster-template-master.md"
		admittedLine := "  Send the refusal's message verbatim to that batch's fork by `SendMessage`, addressed by the agent id its completion notification carried, and end your turn."

		engineTests := []struct {
			name    string
			rel     string
			content string
			want    []string
		}{
			{"PlantedStencilLine", "contracts/stencils/x.md", "ok\nmessage the chair with SendMessage\n", []string{"line 2: SendMessage"}},
			{"AllowlistedMasterLine", masterStencil, admittedLine, nil},
			{"SameLineInAnotherStencil", "contracts/stencils/other.md", admittedLine, []string{"line 1: SendMessage"}},
			{"AnotherLineInTheMasterStencil", masterStencil, "use SendMessage to reach the chair", []string{"line 1: SendMessage"}},
			{"CleanContent", "contracts/stencils/x.md", "post with lym creel post\n", nil},
		}
		for _, tt := range engineTests {
			t.Run("engineToolHits_"+tt.name, func(t *testing.T) {
				t.Parallel()
				got := engineToolHits(tt.rel, tt.content)
				if strings.Join(got, "|") != strings.Join(tt.want, "|") {
					t.Errorf("engineToolHits(%q) = %q; want %q", tt.rel, got, tt.want)
				}
			})
		}

		literalTests := []struct {
			name string
			src  string
			want []string
		}{
			{"LiteralNamesTheTool", "package p\n\nvar notice = \"ask via SendMessage\"\n", []string{"line 3: SendMessage"}},
			{"RawLiteralNamesTheTool", "package p\n\nvar notice = `ask via SendMessage`\n", []string{"line 3: SendMessage"}},
			{"CommentOnly", "package p\n\n// SendMessage is not used here.\nvar notice = \"ask via creel\"\n", nil},
		}
		for _, tt := range literalTests {
			t.Run("goLiteralHits_"+tt.name, func(t *testing.T) {
				t.Parallel()
				got := goLiteralHits(t, "p.go", []byte(tt.src))
				if strings.Join(got, "|") != strings.Join(tt.want, "|") {
					t.Errorf("goLiteralHits() = %q; want %q", got, tt.want)
				}
			})
		}

		t.Run("testdataPromptRoots_ListsAPlantedDirectory", func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			for _, dir := range []string{"internal/x/testdata", "internal/y/plain"} {
				if err := os.MkdirAll(filepath.Join(root, filepath.FromSlash(dir)), 0o755); err != nil {
					t.Fatalf("create %s: %v", dir, err)
				}
			}
			got := testdataPromptRoots(t, root)
			if strings.Join(got, "|") != "internal/x/testdata" {
				t.Errorf("testdataPromptRoots() = %q; want only internal/x/testdata", got)
			}
		})
	})

	t.Run("Tree", func(t *testing.T) {
		t.Parallel()
		root := scankit.Root(t)
		var failures []string

		// Exts holding the empty suffix selects every file; the callback leaves the Go files of contracts/stencils to the Go surface.
		scannedStencils := scankit.Walk(t, scankit.Options{
			Roots:  []string{"contracts/stencils"},
			Filter: scankit.All,
			Exts:   []string{""},
		}, func(f *scankit.File) {
			if strings.HasSuffix(f.Rel, ".go") {
				return
			}
			for _, hit := range engineToolHits(f.Rel, string(f.Data)) {
				failures = append(failures, fmt.Sprintf("%s: %s", f.Rel, hit))
			}
		})
		scankit.RequireFloor(t, scannedStencils, 20, "stencil files")

		scannedFixtures := scankit.Walk(t, scankit.Options{
			Roots:  testdataPromptRoots(t, root),
			Filter: scankit.All,
			Exts:   []string{".md"},
		}, func(f *scankit.File) {
			for _, hit := range engineToolHits(f.Rel, string(f.Data)) {
				failures = append(failures, fmt.Sprintf("%s: %s", f.Rel, hit))
			}
		})
		scankit.RequireFloor(t, scannedFixtures, 5, "testdata prompt fixtures")

		scannedGo := scankit.Walk(t, scankit.Options{}, func(f *scankit.File) {
			for _, hit := range goLiteralHits(t, f.Rel, f.Data) {
				failures = append(failures, fmt.Sprintf("%s: %s", f.Rel, hit))
			}
		})
		scankit.RequireFloor(t, scannedGo, 200, "production Go files")

		if len(failures) > 0 {
			t.Errorf("an engine's own messaging tool is named; message through lym creel, or admit a ForkSubagents stencil line in engineToolAllowed:\n%s", strings.Join(failures, "\n"))
		}
	})
}
