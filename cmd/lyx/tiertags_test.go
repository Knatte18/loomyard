//go:build tier0 || integration

// tiertags_test.go enforces the tier-tag half of `PATTERN-test-speed`: every test file holding a test function carries exactly one `tierN` build tag, and the transitional `|| integration` term appears only on a `//lyx:guard` file.
// The scan parses each file, so a function inside a string literal is no function of the file.

package main

import (
	"bytes"
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

// oldTierSpellings are the tier tags a file may still carry before the tier tags replace them; each is false when the scan asks which tiers a constraint admits.
var oldTierSpellings = []string{"integration", "tmux", "llm"}

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
				case slices.Contains(tierTagNames, tag), slices.Contains(oldTierSpellings, tag):
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

// mentionsIntegrationTag reports whether the constraint queries the `integration` tag.
func mentionsIntegrationTag(expr constraint.Expr) bool {
	if expr == nil {
		return false
	}
	mentioned := false
	expr.Eval(func(tag string) bool {
		if tag == "integration" {
			mentioned = true
		}
		return false
	})
	return mentioned
}

// hasGuardMarker reports whether the source holds the `//lyx:guard` marker text.
// It reads the text rather than the comments, as the guard step does, so a file quoting the marker in a string literal counts.
func hasGuardMarker(data []byte) bool {
	return bytes.Contains(data, []byte("//lyx:guard"))
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
	switch {
	case holdsTest && len(tiers) == 0:
		return fmt.Sprintf("%s: holds a test function but its constraint names no tier tag; give it one `tierN` tag", rel)
	case holdsTest && len(tiers) > 1:
		return fmt.Sprintf("%s: holds a test function but its constraint admits %s; give it the one `tierN` tag of the tier that runs it", rel, strings.Join(tiers, " and "))
	case mentionsIntegrationTag(expr) && !hasGuardMarker(data):
		return fmt.Sprintf("%s: its constraint carries the transitional `integration` term but the file holds no `//lyx:guard` marker; give it one `tierN` tag", rel)
	}
	return ""
}

// TestTierTags_OneTagPerTestFile fails for a test file that holds a test function without exactly one tier tag, and for an unmarked file still carrying the `integration` term.
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
		t.Errorf("`PATTERN-tier-tags` violated:\n%s", strings.Join(failures, "\n"))
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
		{"GuardWithIntegrationTerm", "//go:build tier0 || integration\n\n" + guardFunc, false},
		{"UnmarkedWithIntegrationTerm", "//go:build tier0 || integration\n\n" + testFunc, true},
		{"MarkerQuotedInStringWithIntegrationTerm", "//go:build tier0 || integration\n\n" + testFunc + "\nconst quoted = \"//lyx:guard\"\n", false},
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
