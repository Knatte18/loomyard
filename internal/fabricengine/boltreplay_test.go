// boltreplay_test.go pins that the Bolt replay, ended-set and owner-read code names no board file.
// It also pins that no declaration in that code is board-named.
//
// Package fabricengine; shares the single TestMain in testmain_test.go.

package fabricengine

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// boardNameFindings parses src, named filename, and returns one finding per board file name among its string literals.
// With `only` empty, it scans the whole file.
// It also reports each top-level declaration whose name holds "board" in any case.
// With `only` set, it scans just the bodies of the named functions, written `Type.method` or `function`.
// It reports each named function that the file does not hold.
func boardNameFindings(filename string, src any, only []string) []string {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filename, src, 0)
	if err != nil {
		return []string{fmt.Sprintf("%s: %v", filename, err)}
	}
	var findings []string
	scanLiterals := func(node ast.Node) {
		ast.Inspect(node, func(n ast.Node) bool {
			literal, ok := n.(*ast.BasicLit)
			if !ok || literal.Kind != token.STRING {
				return true
			}
			value, err := strconv.Unquote(literal.Value)
			if err != nil {
				return true
			}
			if value == "board.json" || value == "README.md" || strings.HasPrefix(value, "project-") || strings.HasPrefix(value, "design-") {
				findings = append(findings, fmt.Sprintf("%s: board file name %s", fset.Position(literal.Pos()), literal.Value))
			}
			return true
		})
	}

	if len(only) == 0 {
		scanLiterals(file)
		for _, decl := range file.Decls {
			for _, name := range declaredNames(decl) {
				if strings.Contains(strings.ToLower(name.Name), "board") {
					findings = append(findings, fmt.Sprintf("%s: declaration %s", fset.Position(name.Pos()), name.Name))
				}
			}
		}
		return findings
	}

	found := map[string]bool{}
	for _, decl := range file.Decls {
		function, ok := decl.(*ast.FuncDecl)
		if !ok || function.Body == nil {
			continue
		}
		key := functionKey(function)
		if !slices.Contains(only, key) {
			continue
		}
		found[key] = true
		scanLiterals(function.Body)
	}
	for _, key := range only {
		if !found[key] {
			findings = append(findings, fmt.Sprintf("%s: function %s not found", filename, key))
		}
	}
	return findings
}

// declaredNames returns the identifiers a top-level declaration declares.
func declaredNames(decl ast.Decl) []*ast.Ident {
	switch decl := decl.(type) {
	case *ast.FuncDecl:
		return []*ast.Ident{decl.Name}
	case *ast.GenDecl:
		var names []*ast.Ident
		for _, spec := range decl.Specs {
			switch spec := spec.(type) {
			case *ast.TypeSpec:
				names = append(names, spec.Name)
			case *ast.ValueSpec:
				names = append(names, spec.Names...)
			}
		}
		return names
	}
	return nil
}

// functionKey names function as `Type.method` for a method and by its bare name otherwise.
func functionKey(function *ast.FuncDecl) string {
	if function.Recv == nil || len(function.Recv.List) == 0 {
		return function.Name.Name
	}
	receiver := function.Recv.List[0].Type
	if star, ok := receiver.(*ast.StarExpr); ok {
		receiver = star.X
	}
	if ident, ok := receiver.(*ast.Ident); ok {
		return ident.Name + "." + function.Name.Name
	}
	return function.Name.Name
}

func TestBoltReplay_NamesNoBoardFile(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	t.Run("The matcher finds each board file name and board-named declaration and nothing else", func(t *testing.T) {
		t.Parallel()
		const planted = `package p

type BoardEntry struct{}

var storeFile = "board.json"

func overview() []string { return []string{"README.md", "project-x", "design-x", "other.json"} }
`
		const clean = `package p

type Entry struct{}

func store() string { return "heddle.json" }
`
		const scoped = `package p

type T struct{}

func (t *T) listed() string { return "board.json" }

func unlisted() string { return "board.json" }
`
		if got := boardNameFindings("planted.go", planted, nil); len(got) != 5 {
			t.Errorf("planted whole-file findings = %v; want 5, one per file name and declaration", got)
		}
		if got := boardNameFindings("clean.go", clean, nil); len(got) != 0 {
			t.Errorf("clean whole-file findings = %v; want none", got)
		}
		if got := boardNameFindings("scoped.go", scoped, []string{"T.listed"}); len(got) != 1 {
			t.Errorf("scoped findings = %v; want only the listed function's literal", got)
		}
		if got := boardNameFindings("scoped.go", scoped, []string{"T.missing"}); len(got) != 1 || !strings.Contains(got[0], "not found") {
			t.Errorf("scoped findings for a missing function = %v; want one not-found finding", got)
		}
	})

	t.Run("The replay and registry files name no board file and declare no board-named symbol", func(t *testing.T) {
		t.Parallel()
		for _, filename := range []string{"replay.go", "boltreplay.go"} {
			src, err := os.ReadFile(filename)
			if err != nil {
				t.Fatalf("ReadFile(%q) = %v; want nil", filename, err)
			}
			for _, finding := range boardNameFindings(filename, src, nil) {
				t.Error(finding)
			}
		}
	})

	t.Run("The ended-set and owner readers and the replay log name no board file", func(t *testing.T) {
		t.Parallel()
		readers := map[string][]string{
			"pull.go":                 {"Fabric.weftEndedByOrigin"},
			"pushclassified.go":       {"goneEntryEnded"},
			"originreconcilemoves.go": {"addLocalCreatedStamps"},
			"originreconcile.go":      {"readOriginOwners"},
			"bolt.go":                 {"Bolt.endedByOrigin", "Bolt.logReplay"},
		}
		for filename, functions := range readers {
			src, err := os.ReadFile(filename)
			if err != nil {
				t.Fatalf("ReadFile(%q) = %v; want nil", filename, err)
			}
			for _, finding := range boardNameFindings(filename, src, functions) {
				t.Error(finding)
			}
		}
	})
}
