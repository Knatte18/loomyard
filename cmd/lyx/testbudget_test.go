// testbudget_test.go enforces the per-package test budget: the number of top-level `func Test…` functions in a package's `_test.go` files, under every build tag, never exceeds the row cmd/lyx/testdata/test-budget.yaml pins for it.
// A budget raise is a one-line edit in the same commit as the tests that need it.
// See `PATTERN-test-speed`.

package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path"
	"sort"
	"strings"
	"testing"
	"unicode"

	"gopkg.in/yaml.v3"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
)

// testBudgetFile is the budget's module-relative path from this package's directory.
const testBudgetFile = "testdata/test-budget.yaml"

// testBudgetScanFloor is the fewest test files the repository scan must visit, so a broken walk cannot pass vacuously.
const testBudgetScanFloor = 100

// isTopLevelTest reports whether decl is a package-level function `go test` runs as a test: a name starting with `Test` whose next rune is not lowercase, other than `TestMain`.
func isTopLevelTest(decl ast.Decl) bool {
	fn, ok := decl.(*ast.FuncDecl)
	if !ok || fn.Recv != nil || fn.Name.Name == "TestMain" {
		return false
	}
	rest, found := strings.CutPrefix(fn.Name.Name, "Test")
	if !found {
		return false
	}
	for _, r := range rest {
		return !unicode.IsLower(r)
	}
	return true
}

// countTestsByPackage parses every source, keyed by its slash-separated module-relative path, and returns the top-level test count of each package directory holding at least one.
// A file counts under every build tag, since the parse ignores build constraints.
func countTestsByPackage(sources map[string]string) (map[string]int, error) {
	counts := map[string]int{}
	for file, source := range sources {
		parsed, err := parser.ParseFile(token.NewFileSet(), file, source, parser.SkipObjectResolution)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", file, err)
		}
		for _, decl := range parsed.Decls {
			if isTopLevelTest(decl) {
				counts[path.Dir(file)]++
			}
		}
	}
	return counts, nil
}

// budgetViolations lists, sorted by package, each package whose count exceeds its budget row or has tests and no row.
// A count below its budget passes, and so does a row whose package has no tests left.
func budgetViolations(counts, budget map[string]int) []string {
	var violations []string
	for pkg, count := range counts {
		allowed, hasRow := budget[pkg]
		switch {
		case !hasRow:
			violations = append(violations, fmt.Sprintf("%s: %d top-level tests and no budget row", pkg, count))
		case count > allowed:
			violations = append(violations, fmt.Sprintf("%s: %d top-level tests, over its budget of %d", pkg, count, allowed))
		}
	}
	sort.Strings(violations)
	return violations
}

// TestBudgetViolations pins the comparison over in-memory sources: a package over budget fails with its count and budget, a package with tests and no row fails, a package under budget or a row without tests passes, and a test in a `tmux` or `llm` file counts.
func TestBudgetViolations(t *testing.T) {
	t.Parallel()

	const tests = "package p\n\nimport \"testing\"\n\nfunc TestA(t *testing.T) {}\n\nfunc TestB(t *testing.T) {}\n\nfunc TestMain(m *testing.M) {}\n\nfunc Testable() {}\n\nfunc (s S) TestMethod() {}\n"
	cases := []struct {
		name    string
		sources map[string]string
		budget  map[string]int
		want    []string
	}{
		{"over budget", map[string]string{"a/a_test.go": tests}, map[string]int{"a": 1}, []string{"a: 2 top-level tests, over its budget of 1"}},
		{"no row", map[string]string{"a/a_test.go": tests}, map[string]int{}, []string{"a: 2 top-level tests and no budget row"}},
		{"at budget", map[string]string{"a/a_test.go": tests}, map[string]int{"a": 2}, nil},
		{"under budget", map[string]string{"a/a_test.go": tests}, map[string]int{"a": 5}, nil},
		{"row without tests", map[string]string{"a/a.go": "package a\n"}, map[string]int{"gone": 3}, nil},
		{
			"tagged files and files of one package add up",
			map[string]string{
				"a/a_test.go":      "package a\n\nfunc TestA(t *testing.T) {}\n",
				"a/b_tmux_test.go": "//go:build tmux\n\npackage a\n\nfunc TestB(t *testing.T) {}\n",
				"a/c_llm_test.go":  "//go:build llm\n\npackage a\n\nfunc TestC(t *testing.T) {}\n",
			},
			map[string]int{"a": 2},
			[]string{"a: 3 top-level tests, over its budget of 2"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			counts, err := countTestsByPackage(tc.sources)
			if err != nil {
				t.Fatalf("countTestsByPackage() error = %v; want nil", err)
			}
			got := budgetViolations(counts, tc.budget)
			if strings.Join(got, "\n") != strings.Join(tc.want, "\n") {
				t.Errorf("budgetViolations() = %q; want %q", got, tc.want)
			}
		})
	}
}

// TestTestBudget_PackagesStayWithinBudget fails when a package's top-level test count exceeds its row in the budget file, or when a package with tests has no row.
// Raise a row in the commit that adds the tests that need it.
//
//lyx:guard
func TestTestBudget_PackagesStayWithinBudget(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile(testBudgetFile)
	if err != nil {
		t.Fatalf("read %s: %v", testBudgetFile, err)
	}
	var budget map[string]int
	if err := yaml.Unmarshal(data, &budget); err != nil {
		t.Fatalf("parse %s: %v", testBudgetFile, err)
	}

	sources := map[string]string{}
	scanned := scankit.Walk(t, scankit.Options{Filter: scankit.Test}, func(f *scankit.File) {
		sources[f.Rel] = string(f.Data)
	})
	scankit.RequireFloor(t, scanned, testBudgetScanFloor, "test files")

	counts, err := countTestsByPackage(sources)
	if err != nil {
		t.Fatal(err)
	}
	for _, violation := range budgetViolations(counts, budget) {
		t.Errorf("%s; raise its row in cmd/lyx/%s in the commit that adds the tests", violation, testBudgetFile)
	}
}
