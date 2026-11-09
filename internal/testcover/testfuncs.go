// testfuncs.go reads the top-level Test functions of a package directory's test files.
// Each comes with the tier its opening tiergate.Require names and the hash of its source text.

package testcover

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/Knatte18/loomyard/internal/impactset"
)

// TestFunc is one top-level Test function of a package's test files.
type TestFunc struct {
	Name string
	// File is the base name of the test file that declares the function.
	File string
	// Tier is the tier the function's opening tiergate.Require names; Tiered is false, and Tier 0, when it opens with none.
	Tier   int
	Tiered bool
	// Hash is the ContentHash of the function's source text, from `func` to its closing brace.
	Hash string
}

// ScanTestFuncs returns the top-level Test functions of the `_test.go` files directly in dir, sorted by name.
// A Test function is one `go test` runs as a test; TestMain and methods are not.
// Build constraints are not read, so a file only another platform compiles still counts.
// A test file that cannot be read or parsed is an error naming it.
func ScanTestFuncs(dir string) ([]TestFunc, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var funcs []TestFunc
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		filename := filepath.Join(dir, entry.Name())
		src, err := os.ReadFile(filename)
		if err != nil {
			return nil, err
		}
		found, err := ParseTestFuncs(filename, src)
		if err != nil {
			return nil, err
		}
		funcs = append(funcs, found...)
	}
	sort.Slice(funcs, func(i, j int) bool { return funcs[i].Name < funcs[j].Name })
	return funcs, nil
}

// ParseTestFuncs returns the top-level Test functions of the test file source src, in source order.
// It reads source from any tree, such as a file as a commit stores it.
// filename names the file in a parse error, and its base name fills each function's File.
func ParseTestFuncs(filename string, src []byte) ([]TestFunc, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filename, src, parser.SkipObjectResolution)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", filename, err)
	}
	base := path.Base(filepath.ToSlash(filename))
	var funcs []TestFunc
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Recv != nil || !isTestFuncName(fn.Name.Name) {
			continue
		}
		tier, tiered := impactset.RequiredTier(file, fn)
		start, end := fset.Position(fn.Pos()).Offset, fset.Position(fn.End()).Offset
		funcs = append(funcs, TestFunc{Name: fn.Name.Name, File: base, Tier: tier, Tiered: tiered, Hash: ContentHash(src[start:end])})
	}
	return funcs, nil
}

// isTestFuncName reports whether name is that of a function `go test` runs as a test:
// Test followed by nothing or by no lowercase letter, TestMain apart.
func isTestFuncName(name string) bool {
	rest, found := strings.CutPrefix(name, "Test")
	if !found || name == "TestMain" {
		return false
	}
	first, _ := utf8.DecodeRuneInString(rest)
	return rest == "" || !unicode.IsLower(first)
}
