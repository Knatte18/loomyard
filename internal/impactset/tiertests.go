// tiertests.go reads what the test files of a package directory hold, without `go list`:
// which tiers' tests, from the `tiergate.Require` call opening each test, and whether they use the `lym` binary.

package impactset

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// TestsUseLymBinary reports whether the tests of the package directly in dir may run the `lym` binary.
// Only `internal/testkit/lymbin` hands a test that binary.
// So they may when dir is that package's own directory, or when a `_test.go` file directly in dir imports it.
// Build constraints are not read, so a file only another platform compiles still counts.
// A directory or test file that cannot be read or parsed counts as using it, so a run that needs the binary keeps it.
func TestsUseLymBinary(dir string) bool {
	if strings.HasSuffix(filepath.ToSlash(filepath.Clean(dir)), "/"+lymbinDir) {
		return true
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return true
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), filepath.Join(dir, entry.Name()), nil, parser.ImportsOnly)
		if err != nil {
			return true
		}
		for _, spec := range file.Imports {
			path, err := strconv.Unquote(spec.Path.Value)
			if err == nil && (path == lymbinDir || strings.HasSuffix(path, "/"+lymbinDir)) {
				return true
			}
		}
	}
	return false
}

// testFunctionPrefixes are the name prefixes of the top-level functions `go test` runs, benchmarks, fuzzes or checks.
var testFunctionPrefixes = []string{"Test", "Benchmark", "Fuzz", "Example"}

// HoldsTierTest reports whether a `_test.go` file directly in dir declares a test function that tiers' runs execute.
// A test function is a top-level Test, Benchmark, Fuzz or Example function, TestMain apart.
// One opening with `tiergate.Require(t, N)` runs in tier N alone, and any other runs in every tier.
// Build constraints are not read, so a file only another platform compiles still counts.
// A directory or test file that cannot be read or parsed counts as holding one, so the test run reports it.
func HoldsTierTest(dir string, tiers []int) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return true
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), filepath.Join(dir, entry.Name()), nil, parser.SkipObjectResolution)
		if err != nil {
			return true
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv != nil || !isTestFunctionName(fn.Name.Name) {
				continue
			}
			tier, required := RequiredTier(file, fn)
			if !required || slices.Contains(tiers, tier) {
				return true
			}
		}
	}
	return false
}

// isTestFunctionName reports whether name is that of a function `go test` runs, benchmarks, fuzzes or checks:
// one of testFunctionPrefixes followed by nothing or by no lowercase letter, with `TestMain` apart.
func isTestFunctionName(name string) bool {
	if name == "TestMain" {
		return false
	}
	for _, prefix := range testFunctionPrefixes {
		if rest, found := strings.CutPrefix(name, prefix); found {
			first, _ := utf8.DecodeRuneInString(rest)
			return rest == "" || !unicode.IsLower(first)
		}
	}
	return false
}
