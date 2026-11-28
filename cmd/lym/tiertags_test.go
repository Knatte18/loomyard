// tiertags_test.go enforces the tier half of `PATTERN-test-speed`.
// Each test function opens with `tiergate.Require` naming its tier, and the test functions of one file name one tier.
// No test file carries a tier build tag or a retired tier spelling.
// The scan parses each file, so a function inside a string literal is not a function of the file.

package main

import (
	"fmt"
	"go/ast"
	"go/build/constraint"
	"go/parser"
	"go/token"
	"slices"
	"sort"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

	"github.com/Knatte18/loomyard/internal/impactset"
	"github.com/Knatte18/loomyard/internal/testkit/scankit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// tierTagNames are the build tags a test file must not name in its constraint.
// `tiergate.Require` selects the tier at run time, so no file names one.
var tierTagNames = []string{"tier0", "tier1", "tier2", "tier3"}

// retiredTierSpellings maps each old build tag, superseded by the `tierN` tags, to the tier it named.
var retiredTierSpellings = map[string]string{"integration": "tier1", "tmux": "tier2", "llm": "tier3"}

// holdsTestFunction reports whether the name is a `Test`, `Benchmark`, `Fuzz` or `Example` function that go test runs;
// `TestMain` is not one.
func holdsTestFunction(name string) bool {
	if name == "TestMain" {
		return false
	}
	for _, prefix := range []string{"Test", "Benchmark", "Fuzz", "Example"} {
		rest, ok := strings.CutPrefix(name, prefix)
		if !ok {
			continue
		}
		r, _ := utf8.DecodeRuneInString(rest)
		return rest == "" || !unicode.IsLower(r)
	}
	return false
}

// testFunctions returns the file's top-level functions that go test runs.
func testFunctions(file *ast.File) []*ast.FuncDecl {
	var tests []*ast.FuncDecl
	for _, decl := range file.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Recv == nil && holdsTestFunction(fn.Name.Name) {
			tests = append(tests, fn)
		}
	}
	return tests
}

// firstTierTerm returns the first tier tag or retired tier spelling the constraint names, left to right.
// A negated term counts too; it returns "" when the constraint names none.
// It walks the expression tree because Eval short-circuits, which would skip an operand to the right of a false term.
func firstTierTerm(expr constraint.Expr) string {
	switch e := expr.(type) {
	case *constraint.TagExpr:
		if _, retired := retiredTierSpellings[e.Tag]; retired || slices.Contains(tierTagNames, e.Tag) {
			return e.Tag
		}
	case *constraint.NotExpr:
		return firstTierTerm(e.X)
	case *constraint.AndExpr:
		if found := firstTierTerm(e.X); found != "" {
			return found
		}
		return firstTierTerm(e.Y)
	case *constraint.OrExpr:
		if found := firstTierTerm(e.X); found != "" {
			return found
		}
		return firstTierTerm(e.Y)
	}
	return ""
}

// tierFailure returns the failure line for a test file that breaks the tier rule, or "" when the file passes.
func tierFailure(rel string, data []byte) string {
	file, err := parser.ParseFile(token.NewFileSet(), rel, data, parser.ParseComments|parser.SkipObjectResolution)
	if err != nil {
		return fmt.Sprintf("%s: does not parse: %v", rel, err)
	}
	if term := firstTierTerm(buildConstraint(file)); term != "" {
		if tier, retired := retiredTierSpellings[term]; retired {
			return fmt.Sprintf("%s: its constraint names the retired tier tag `%s`; drop the term and open each test function with tiergate.Require for %s", rel, term, tier)
		}
		return fmt.Sprintf("%s: its constraint names the tier tag `%s`, which splits the package's test binary by tier; drop the term and open each test function with tiergate.Require", rel, term)
	}
	tiers := map[int]bool{}
	for _, fn := range testFunctions(file) {
		if strings.HasPrefix(fn.Name.Name, "Example") {
			return fmt.Sprintf("%s: %s is an Example function, which cannot select its tier; write it as a Test function", rel, fn.Name.Name)
		}
		tier, ok := impactset.RequiredTier(file, fn)
		if !ok {
			return fmt.Sprintf("%s: %s does not open with tiergate.Require(t, N) naming its tier", rel, fn.Name.Name)
		}
		tiers[tier] = true
	}
	if len(tiers) > 1 {
		var named []string
		for tier := range tiers {
			named = append(named, fmt.Sprintf("tier%d", tier))
		}
		sort.Strings(named)
		return fmt.Sprintf("%s: its test functions name %s; keep one tier per file and move the others to a file of their own", rel, strings.Join(named, " and "))
	}
	return ""
}

// TestTierTags_OneTierPerTestFile fails for a test function that does not open with `tiergate.Require`.
// It also fails for a test file whose functions name two tiers.
// It also fails for a test file whose constraint names a tier tag or a retired tier spelling.
//
//lym:guard
func TestTierTags_OneTierPerTestFile(t *testing.T) {
	tiergate.Require(t, 0)
	var failures []string
	scanned := scankit.Walk(t, scankit.Options{Filter: scankit.Test}, func(f *scankit.File) {
		if failure := tierFailure(f.Rel, f.Data); failure != "" {
			failures = append(failures, failure)
		}
	})
	scankit.RequireFloor(t, scanned, 1000, "tier guard")
	sort.Strings(failures)
	if len(failures) > 0 {
		t.Errorf("`PATTERN-test-speed` violated:\n%s", strings.Join(failures, "\n"))
	}
}

func TestTierFailure_FixtureSources(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	const header = "package p\n\nimport (\n\t\"testing\"\n\n\t\"github.com/Knatte18/loomyard/internal/testkit/tiergate\"\n)\n\n"
	const tier0Func = header + "func TestX(t *testing.T) {\n\ttiergate.Require(t, 0)\n}\n"
	const tier1Func = header + "func TestY(t *testing.T) {\n\ttiergate.Require(t, 1)\n}\n"
	const mainOnly = "package p\n\nimport (\n\t\"os\"\n\t\"testing\"\n)\n\nfunc TestMain(m *testing.M) { os.Exit(m.Run()) }\n\nfunc helper() {}\n"
	const literalOnly = "package p\n\nconst src = \"func TestX(t *testing.T) {}\"\n"

	tests := []struct {
		name     string
		source   string
		wantFail bool
	}{
		{"RequiredTestFunction", tier0Func, false},
		{"TestFunctionWithoutRequire", "package p\n\nimport \"testing\"\n\nfunc TestX(t *testing.T) {}\n", true},
		{"RequireNotFirst", header + "func TestX(t *testing.T) {\n\tt.Parallel()\n\ttiergate.Require(t, 0)\n}\n", true},
		{"BenchmarkWithRequire", header + "func BenchmarkX(b *testing.B) {\n\ttiergate.Require(b, 0)\n}\n", false},
		{"ExampleFunction", "package p\n\nfunc ExampleX() {}\n", true},
		{"TwoTiersInOneFile", tier0Func + "\nfunc TestY(t *testing.T) {\n\ttiergate.Require(t, 1)\n}\n", true},
		{"TierTag", "//go:build tier1\n\n" + tier1Func, true},
		{"TierTagBesidePlatformTerm", "//go:build tier2 && !windows\n\n" + tier1Func, true},
		{"NegatedTierTagOnSupport", "//go:build !tier1\n\n" + mainOnly, true},
		{"RetiredSpelling", "//go:build integration\n\n" + tier1Func, true},
		{"PlatformTermOnly", "//go:build !windows\n\n" + tier1Func, false},
		{"MainAndHelpers", mainOnly, false},
		{"FunctionInsideStringLiteral", literalOnly, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := tierFailure("p/a_test.go", []byte(tt.source))
			if (got != "") != tt.wantFail {
				t.Errorf("tierFailure(%q) = %q; want failure %v", tt.name, got, tt.wantFail)
			}
		})
	}
}
