// tmuxisolation_test.go enforces the Tmux Test Isolation Invariant: every test package with an
// `integration`- or `smoke`-tagged test file runs its tests through tmuxkit.Main, under every tag set
// that compiles any of its test files.
// "Starts tmux" has no static shape, so the rule keys on "has a tagged test file" instead,
// at the cost of one temp directory per tagged package.
// Modelled on hermeticenv_test.go; see CONSTRAINTS.md's Tmux Test Isolation Invariant.

package main

import (
	"fmt"
	"go/ast"
	"go/build/constraint"
	"go/parser"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
)

// allowedNoTmuxMain is the Tmux Test Isolation Invariant allowlist, keyed by package directory with a trailing "/".
// An entry is admitted only for a package whose TestMain cannot call tmuxkit.Main, and its reason names that obstacle.
// "Does not start tmux" is not a reason.
var allowedNoTmuxMain = []scankit.Entry{}

// isolationTags are the build tags that make a test file tagged.
var isolationTags = []string{"integration", "smoke"}

// isolationTagSets are the tag sets a package's test files are compiled under: untagged, then each isolation tag.
var isolationTagSets = [][]string{nil, {"integration"}, {"smoke"}}

// isolationPlatforms are the platform-tag profiles a constraint is tried under; a file compiles under a tag set when any profile satisfies it.
var isolationPlatforms = []map[string]bool{
	{"linux": true, "unix": true, "amd64": true, "cgo": true, "gc": true},
	{"windows": true, "amd64": true, "cgo": true, "gc": true},
	{"darwin": true, "unix": true, "arm64": true, "cgo": true, "gc": true},
}

// isolationFile is the evidence collected for one test file.
type isolationFile struct {
	rel          string
	expr         constraint.Expr
	tagged       bool
	testMain     bool
	callsKitMain bool
}

// compilesUnder reports whether the file compiles under the tag set on at least one platform profile.
func (f isolationFile) compilesUnder(tags []string) bool {
	if f.expr == nil {
		return true
	}
	for _, platform := range isolationPlatforms {
		if f.expr.Eval(func(tag string) bool {
			if platform[tag] {
				return true
			}
			for _, t := range tags {
				if t == tag {
					return true
				}
			}
			return false
		}) {
			return true
		}
	}
	return false
}

// buildConstraint returns the `//go:build` expression in the file's header comments, or nil when it has none.
func buildConstraint(file *ast.File) constraint.Expr {
	for _, group := range file.Comments {
		if group.Pos() >= file.Package {
			break
		}
		for _, c := range group.List {
			if constraint.IsGoBuild(c.Text) {
				expr, err := constraint.Parse(c.Text)
				if err != nil {
					return nil
				}
				return expr
			}
		}
	}
	return nil
}

// mentionsIsolationTag reports whether the expression queries any isolation tag.
func mentionsIsolationTag(expr constraint.Expr) bool {
	if expr == nil {
		return false
	}
	mentioned := false
	expr.Eval(func(tag string) bool {
		for _, t := range isolationTags {
			if t == tag {
				mentioned = true
			}
		}
		return false
	})
	return mentioned
}

// declaresTestMain reports whether the file declares TestMain, and whether that function calls tmuxkit.Main.
func declaresTestMain(file *ast.File) (declared, callsKitMain bool) {
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Recv != nil || fn.Name.Name != "TestMain" || fn.Body == nil {
			continue
		}
		declared = true
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			switch fun := call.Fun.(type) {
			case *ast.SelectorExpr:
				if pkg, ok := fun.X.(*ast.Ident); ok && pkg.Name == "tmuxkit" && fun.Sel.Name == "Main" {
					callsKitMain = true
				}
			case *ast.Ident:
				// The kit's own tests cannot import it, so they call the entry point unqualified.
				if file.Name.Name == "tmuxkit" && fun.Name == "Main" {
					callsKitMain = true
				}
			}
			return true
		})
	}
	return declared, callsKitMain
}

// tmuxIsolationFailures walks the test files under opts and returns one failure line per violating package,
// the number of tagged packages found and the number of files scanned.
func tmuxIsolationFailures(t *testing.T, opts scankit.Options, allow *scankit.Allowlist) (failures []string, tagged, scanned int) {
	t.Helper()
	opts.Filter = scankit.Test
	packages := map[string][]isolationFile{}
	scanned = scankit.Walk(t, opts, func(f *scankit.File) {
		file := f.AST(t, parser.ParseComments|parser.SkipObjectResolution)
		expr := buildConstraint(file)
		declared, calls := declaresTestMain(file)
		dir := filepath.ToSlash(filepath.Dir(f.Rel))
		packages[dir] = append(packages[dir], isolationFile{
			rel:          f.Rel,
			expr:         expr,
			tagged:       mentionsIsolationTag(expr),
			testMain:     declared,
			callsKitMain: calls,
		})
	})

	for dir, files := range packages {
		isTagged := false
		for _, f := range files {
			isTagged = isTagged || f.tagged
		}
		if !isTagged {
			continue
		}
		tagged++
		if allow.Allowed(dir + "/") {
			continue
		}
		for _, f := range files {
			if f.testMain && !f.callsKitMain {
				failures = append(failures, fmt.Sprintf("%s: TestMain in %s does not call tmuxkit.Main", dir, f.rel))
			}
		}
		for _, tags := range isolationTagSets {
			compiled, hasTestMain := 0, false
			for _, f := range files {
				if f.compilesUnder(tags) {
					compiled++
					hasTestMain = hasTestMain || f.testMain
				}
			}
			if compiled > 0 && !hasTestMain {
				failures = append(failures, fmt.Sprintf("%s: no TestMain compiles under tags [%s]", dir, strings.Join(tags, ",")))
			}
		}
	}
	sort.Strings(failures)
	return failures, tagged, scanned
}

// TestTmuxIsolation_TaggedPackagesRunThroughTmuxkitMain fails for every package with an integration- or
// smoke-tagged test file unless, under each tag set that compiles any of its test files, a TestMain compiles,
// and every TestMain in the package calls tmuxkit.Main.
func TestTmuxIsolation_TaggedPackagesRunThroughTmuxkitMain(t *testing.T) {
	allow := scankit.NewAllowlist(allowedNoTmuxMain)
	failures, tagged, scanned := tmuxIsolationFailures(t, scankit.Options{}, allow)
	scankit.RequireFloor(t, scanned, 1, "tmux isolation scan")
	scankit.RequireFloor(t, tagged, 1, "tmux isolation scan tagged packages")
	allow.RequireNoStale(t)
	if len(failures) > 0 {
		t.Errorf("Tmux Test Isolation Invariant violated (see CONSTRAINTS.md):\n%s\nadd a TestMain calling os.Exit(tmuxkit.Main(m)) that compiles under every tag set the package's tests compile under", strings.Join(failures, "\n"))
	}
}

func TestTmuxIsolation_FixtureTrees(t *testing.T) {
	const withMain = "package p\n\nimport (\n\t\"os\"\n\t\"testing\"\n\n\t\"github.com/Knatte18/loomyard/internal/testkit/tmuxkit\"\n)\n\nfunc TestMain(m *testing.M) { os.Exit(tmuxkit.Main(m)) }\n"
	const bareMain = "package p\n\nimport (\n\t\"os\"\n\t\"testing\"\n)\n\nfunc TestMain(m *testing.M) { os.Exit(m.Run()) }\n"
	const taggedTest = "//go:build integration\n\npackage p\n"
	const untaggedTest = "package p\n"

	cases := []struct {
		name       string
		files      map[string]string
		wantTagged int
		wantFail   int
	}{
		{"main in every set", map[string]string{"p/main_test.go": withMain, "p/i_test.go": taggedTest}, 1, 0},
		{"missing kit call", map[string]string{"p/main_test.go": bareMain, "p/i_test.go": taggedTest}, 1, 1},
		{"no TestMain at all", map[string]string{"p/a_test.go": untaggedTest, "p/i_test.go": taggedTest}, 1, 3},
		{"integration-only main", map[string]string{"p/main_test.go": "//go:build integration\n\n" + withMain, "p/a_test.go": untaggedTest}, 1, 2},
		{"untagged package", map[string]string{"p/a_test.go": untaggedTest}, 0, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			base := t.TempDir()
			for rel, body := range tc.files {
				path := filepath.Join(base, filepath.FromSlash(rel))
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			failures, tagged, _ := tmuxIsolationFailures(t, scankit.Options{Base: base}, scankit.NewAllowlist(nil))
			if tagged != tc.wantTagged || len(failures) != tc.wantFail {
				t.Fatalf("tagged = %d, failures = %v; want tagged %d, %d failure(s)", tagged, failures, tc.wantTagged, tc.wantFail)
			}
		})
	}
}
