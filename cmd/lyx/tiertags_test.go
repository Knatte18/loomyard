//go:build tier0

// tiertags_test.go enforces the tier-tag half of `PATTERN-test-speed`: every test file holding a test function carries exactly one `tierN` build tag, and no test file names a retired tier spelling.
// The scan parses each file, so a function inside a string literal is no function of the file.

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

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
)

// tierTagNames are the build tags that name a test tier, one per tier.
var tierTagNames = []string{"tier0", "tier1", "tier2", "tier3"}

// retiredTierSpellings are the tags that named tiers before the `tierN` tags, each paired with the tier tag that replaced it.
// No tier command sets them, so a constraint naming one selects its file for no tier.
var retiredTierSpellings = map[string]string{"integration": "tier1", "tmux": "tier2", "llm": "tier3"}

// holdsTestFunction reports whether the name is a `Test`, `Benchmark`, `Fuzz` or `Example` function that go test runs, which `TestMain` is not.
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

// admittedTiers returns the tier tags the constraint can be true under when only that one tier tag is set, a platform term set either way.
func admittedTiers(expr constraint.Expr) []string {
	if expr == nil {
		return nil
	}
	var admitted []string
	for _, tier := range tierTagNames {
		for _, platformTerms := range []bool{false, true} {
			compiles := expr.Eval(func(tag string) bool {
				switch {
				case tag == tier:
					return true
				case slices.Contains(tierTagNames, tag):
					return false
				}
				return platformTerms
			})
			if compiles {
				admitted = append(admitted, tier)
				break
			}
		}
	}
	return admitted
}

// firstRetiredTierSpelling returns the first retired tier spelling the constraint names, left to right, or "" when it names none.
// It walks the expression tree because Eval short-circuits, which would skip an operand to the right of a false term.
func firstRetiredTierSpelling(expr constraint.Expr) string {
	switch e := expr.(type) {
	case *constraint.TagExpr:
		if _, retired := retiredTierSpellings[e.Tag]; retired {
			return e.Tag
		}
	case *constraint.NotExpr:
		return firstRetiredTierSpelling(e.X)
	case *constraint.AndExpr:
		if found := firstRetiredTierSpelling(e.X); found != "" {
			return found
		}
		return firstRetiredTierSpelling(e.Y)
	case *constraint.OrExpr:
		if found := firstRetiredTierSpelling(e.X); found != "" {
			return found
		}
		return firstRetiredTierSpelling(e.Y)
	}
	return ""
}

// tierTagFailure returns the failure line for a test file whose tags break the one-tier-tag rule, or "" when the file passes.
func tierTagFailure(rel string, data []byte) string {
	file, err := parser.ParseFile(token.NewFileSet(), rel, data, parser.ParseComments|parser.SkipObjectResolution)
	if err != nil {
		return fmt.Sprintf("%s: does not parse: %v", rel, err)
	}
	expr := buildConstraint(file)
	tiers := admittedTiers(expr)
	holdsTest := false
	for _, decl := range file.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Recv == nil && holdsTestFunction(fn.Name.Name) {
			holdsTest = true
			break
		}
	}
	retired := firstRetiredTierSpelling(expr)
	switch {
	case retired != "":
		return fmt.Sprintf("%s: its constraint names the retired tier tag `%s`, which no tier command sets; use `%s`", rel, retired, retiredTierSpellings[retired])
	case holdsTest && len(tiers) == 0:
		return fmt.Sprintf("%s: holds a test function but its constraint names no tier tag; give it one `tierN` tag", rel)
	case holdsTest && len(tiers) > 1:
		return fmt.Sprintf("%s: holds a test function but its constraint admits %s; give it the one `tierN` tag of the tier that runs it", rel, strings.Join(tiers, " and "))
	}
	return ""
}

// TestTierTags_OneTagPerTestFile fails for a test file that holds a test function without exactly one tier tag, and for a test file naming a retired tier spelling.
//
//lyx:guard
func TestTierTags_OneTagPerTestFile(t *testing.T) {
	var failures []string
	scanned := scankit.Walk(t, scankit.Options{Filter: scankit.Test}, func(f *scankit.File) {
		if failure := tierTagFailure(f.Rel, f.Data); failure != "" {
			failures = append(failures, failure)
		}
	})
	scankit.RequireFloor(t, scanned, 1000, "tier tag guard")
	sort.Strings(failures)
	if len(failures) > 0 {
		t.Errorf("`PATTERN-test-speed` violated:\n%s", strings.Join(failures, "\n"))
	}
}

func TestTierTagFailure_FixtureSources(t *testing.T) {
	t.Parallel()
	const testFunc = "package p\n\nimport \"testing\"\n\nfunc TestX(t *testing.T) {}\n"
	const guardFunc = "package p\n\nimport \"testing\"\n\n//lyx:guard\nfunc TestX(t *testing.T) {}\n"
	const mainOnly = "package p\n\nimport (\n\t\"os\"\n\t\"testing\"\n)\n\nfunc TestMain(m *testing.M) { os.Exit(m.Run()) }\n\nfunc helper() {}\n"
	const literalOnly = "package p\n\nconst src = \"func TestX(t *testing.T) {}\"\n"

	tests := []struct {
		name     string
		source   string
		wantFail bool
	}{
		{"UntaggedTestFunction", testFunc, true},
		{"Tier0TestFunction", "//go:build tier0\n\n" + testFunc, false},
		{"TwoTierTestFunction", "//go:build tier2 || tier3\n\n" + testFunc, true},
		{"OldSpellingOnlyTestFunction", "//go:build tmux && !windows\n\n" + testFunc, true},
		{"IntegrationOnlyTestFunction", "//go:build integration\n\n" + testFunc, true},
		{"RetiredSpellingRightOfTierTerm", "//go:build tier1 && integration\n\n" + testFunc, true},
		{"GuardWithIntegrationTerm", "//go:build tier0 || integration\n\n" + guardFunc, true},
		{"NegatedRetiredSpellingWithoutTest", "//go:build !tmux\n\n" + mainOnly, true},
		{"UntaggedMainAndHelpers", mainOnly, false},
		{"NegatedTierCompanion", "//go:build !tier1\n\n" + mainOnly, false},
		{"Tier2WithPlatformTerm", "//go:build tier2 && !windows\n\n" + testFunc, false},
		{"Tier0WithPlatformTerm", "//go:build tier0 && windows\n\n" + testFunc, false},
		{"FunctionInsideStringLiteral", literalOnly, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := tierTagFailure("p/a_test.go", []byte(tt.source))
			if (got != "") != tt.wantFail {
				t.Errorf("tierTagFailure(%q) = %q; want failure %v", tt.name, got, tt.wantFail)
			}
		})
	}
}
