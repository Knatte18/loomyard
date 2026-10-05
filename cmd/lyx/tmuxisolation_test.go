// tmuxisolation_test.go enforces the Tmux Test Isolation Invariant: every test package with an `integration`- or `smoke`-tagged test file runs its tests through tmuxkit.Main, under every tag set and on every platform that compile any of its test files.
// "Starts tmux" has no static shape, so the rule keys on "has a tagged test file" instead, at the cost of one temp directory per tagged package.
// Modelled on hermeticenv_test.go; see `PATTERN-test-isolation`.

package main

import (
	"fmt"
	"go/ast"
	"go/build/constraint"
	"go/parser"
	"os"
	"path/filepath"
	"slices"
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

// isolationPlatform is one platform a package's test files are compiled for, with the build tags it satisfies.
type isolationPlatform struct {
	name string
	tags map[string]bool
}

// isolationPlatforms are the platforms each tag set is checked on.
var isolationPlatforms = []isolationPlatform{
	{"linux", map[string]bool{"linux": true, "unix": true, "amd64": true, "cgo": true, "gc": true}},
	{"windows", map[string]bool{"windows": true, "amd64": true, "cgo": true, "gc": true}},
	{"darwin", map[string]bool{"darwin": true, "unix": true, "arm64": true, "cgo": true, "gc": true}},
}

// knownOS and knownArch are the GOOS and GOARCH values a file-name suffix can carry.
var (
	knownOS = map[string]bool{
		"aix": true, "android": true, "darwin": true, "dragonfly": true, "freebsd": true, "hurd": true, "illumos": true, "ios": true,
		"js": true, "linux": true, "nacl": true, "netbsd": true, "openbsd": true, "plan9": true, "solaris": true, "wasip1": true, "windows": true, "zos": true,
	}
	knownArch = map[string]bool{
		"386": true, "amd64": true, "arm": true, "arm64": true, "loong64": true, "mips": true, "mipsle": true, "mips64": true, "mips64le": true,
		"ppc64": true, "ppc64le": true, "riscv64": true, "s390x": true, "sparc64": true, "wasm": true,
	}
)

// fileNamePlatform returns the GOOS and GOARCH a file name's `_GOOS`, `_GOARCH` or `_GOOS_GOARCH` suffix confines it to, following go/build's rule.
func fileNamePlatform(name string) (goos, goarch string) {
	name, _, _ = strings.Cut(name, ".")
	i := strings.Index(name, "_")
	if i < 0 {
		return "", ""
	}
	parts := strings.Split(name[i:], "_")
	if n := len(parts); n > 0 && parts[n-1] == "test" {
		parts = parts[:n-1]
	}
	n := len(parts)
	if n >= 2 && knownOS[parts[n-2]] && knownArch[parts[n-1]] {
		return parts[n-2], parts[n-1]
	}
	if n >= 1 && knownOS[parts[n-1]] {
		return parts[n-1], ""
	}
	if n >= 1 && knownArch[parts[n-1]] {
		return "", parts[n-1]
	}
	return "", ""
}

// isolationFile is the evidence collected for one test file.
type isolationFile struct {
	rel          string
	expr         constraint.Expr
	goos, goarch string
	tagged       bool
	testMain     bool
	callsKitMain bool
}

// compilesUnder reports whether the file compiles under the tag set on the platform.
func (f isolationFile) compilesUnder(tags []string, platform isolationPlatform) bool {
	if (f.goos != "" && !platform.tags[f.goos]) || (f.goarch != "" && !platform.tags[f.goarch]) {
		return false
	}
	if f.expr == nil {
		return true
	}
	return f.expr.Eval(func(tag string) bool {
		return platform.tags[tag] || slices.Contains(tags, tag)
	})
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

const (
	tmuxkitImportPath = "github.com/Knatte18/loomyard/internal/testkit/tmuxkit"
	// tmuxkitDir is the kit's own directory, whose tests cannot import the kit and so call the entry point unqualified.
	tmuxkitDir = "internal/testkit/tmuxkit"
)

// kitImportName returns the name the file binds the kit's import path to, or "" when it does not import the kit.
func kitImportName(file *ast.File) string {
	for _, imp := range file.Imports {
		if strings.Trim(imp.Path.Value, `"`) != tmuxkitImportPath {
			continue
		}
		if imp.Name != nil {
			return imp.Name.Name
		}
		return "tmuxkit"
	}
	return ""
}

// declaresTestMain reports whether the file, in package directory dir, declares TestMain, and whether that function calls tmuxkit.Main.
func declaresTestMain(file *ast.File, dir string) (declared, callsKitMain bool) {
	kitName := kitImportName(file)
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
				if pkg, ok := fun.X.(*ast.Ident); ok && kitName != "" && pkg.Name == kitName && fun.Sel.Name == "Main" {
					callsKitMain = true
				}
			case *ast.Ident:
				if dir == tmuxkitDir && fun.Name == "Main" {
					callsKitMain = true
				}
			}
			return true
		})
	}
	return declared, callsKitMain
}

// tmuxIsolationFailures walks the test files under opts and returns the violating packages' failure lines, the number of tagged packages found and the number of files scanned.
func tmuxIsolationFailures(t *testing.T, opts scankit.Options, allow *scankit.Allowlist) (failures []string, tagged, scanned int) {
	t.Helper()
	opts.Filter = scankit.Test
	packages := map[string][]isolationFile{}
	scanned = scankit.Walk(t, opts, func(f *scankit.File) {
		file := f.AST(t, parser.ParseComments|parser.SkipObjectResolution)
		expr := buildConstraint(file)
		dir := filepath.ToSlash(filepath.Dir(f.Rel))
		declared, calls := declaresTestMain(file, dir)
		goos, goarch := fileNamePlatform(filepath.Base(f.Rel))
		packages[dir] = append(packages[dir], isolationFile{
			rel:          f.Rel,
			expr:         expr,
			goos:         goos,
			goarch:       goarch,
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
			var missing []string
			for _, platform := range isolationPlatforms {
				compiled, hasTestMain := 0, false
				for _, f := range files {
					if f.compilesUnder(tags, platform) {
						compiled++
						hasTestMain = hasTestMain || f.testMain
					}
				}
				if compiled > 0 && !hasTestMain {
					missing = append(missing, platform.name)
				}
			}
			if len(missing) > 0 {
				failures = append(failures, fmt.Sprintf("%s: no TestMain compiles under tags [%s] on %s", dir, strings.Join(tags, ","), strings.Join(missing, ", ")))
			}
		}
	}
	sort.Strings(failures)
	return failures, tagged, scanned
}

// TestTmuxIsolation_TaggedPackagesRunThroughTmuxkitMain fails for every package with an integration- or smoke-tagged test file unless a TestMain compiles under each tag set and on each platform that compile any of its test files and every TestMain in the package calls tmuxkit.Main.
func TestTmuxIsolation_TaggedPackagesRunThroughTmuxkitMain(t *testing.T) {
	allow := scankit.NewAllowlist(allowedNoTmuxMain)
	failures, tagged, scanned := tmuxIsolationFailures(t, scankit.Options{}, allow)
	scankit.RequireFloor(t, scanned, 1, "tmux isolation scan")
	scankit.RequireFloor(t, tagged, 1, "tmux isolation scan tagged packages")
	allow.RequireNoStale(t)
	if len(failures) > 0 {
		t.Errorf("`PATTERN-test-isolation` violated:\n%s\nadd a TestMain calling os.Exit(tmuxkit.Main(m)) that compiles under every tag set the package's tests compile under", strings.Join(failures, "\n"))
	}
}

func TestFileNamePlatform(t *testing.T) {
	tests := []struct {
		name, goos, goarch string
	}{
		{"a_test.go", "", ""},
		{"linux_test.go", "", ""},
		{"proc_linux_test.go", "linux", ""},
		{"x_windows_amd64_test.go", "windows", "amd64"},
		{"x_arm64_test.go", "", "arm64"},
		{"smoke_procalive_windows_test.go", "windows", ""},
	}
	for _, tt := range tests {
		goos, goarch := fileNamePlatform(tt.name)
		if goos != tt.goos || goarch != tt.goarch {
			t.Errorf("fileNamePlatform(%q) = %q, %q; want %q, %q", tt.name, goos, goarch, tt.goos, tt.goarch)
		}
	}
}

func TestTmuxIsolation_FixtureTrees(t *testing.T) {
	const withMain = "package p\n\nimport (\n\t\"os\"\n\t\"testing\"\n\n\t\"github.com/Knatte18/loomyard/internal/testkit/tmuxkit\"\n)\n\nfunc TestMain(m *testing.M) { os.Exit(tmuxkit.Main(m)) }\n"
	const bareMain = "package p\n\nimport (\n\t\"os\"\n\t\"testing\"\n)\n\nfunc TestMain(m *testing.M) { os.Exit(m.Run()) }\n"
	const taggedTest = "//go:build integration\n\npackage p\n"
	const untaggedTest = "package p\n"
	const aliasedMain = "package p\n\nimport (\n\t\"os\"\n\t\"testing\"\n\n\tkit \"github.com/Knatte18/loomyard/internal/testkit/tmuxkit\"\n)\n\nfunc TestMain(m *testing.M) { os.Exit(kit.Main(m)) }\n"
	const unrelatedMain = "package p\n\nimport (\n\t\"os\"\n\t\"testing\"\n\n\t\"example.com/tmuxkit\"\n)\n\nfunc TestMain(m *testing.M) { os.Exit(tmuxkit.Main(m)) }\n"
	const unqualifiedMain = "package tmuxkit\n\nimport (\n\t\"os\"\n\t\"testing\"\n)\n\nfunc TestMain(m *testing.M) { os.Exit(Main(m)) }\n"
	const kitTaggedTest = "//go:build integration\n\npackage tmuxkit\n"

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
		{"main confined by constraint to another platform", map[string]string{"p/main_test.go": "//go:build windows\n\n" + withMain, "p/i_test.go": "//go:build integration && linux\n\npackage p\n"}, 1, 1},
		{"main confined by file name to another platform", map[string]string{"p/main_windows_test.go": withMain, "p/i_test.go": "//go:build integration && linux\n\npackage p\n"}, 1, 1},
		{"main on every platform its tests compile on", map[string]string{"p/main_linux_test.go": withMain, "p/i_linux_test.go": taggedTest}, 1, 0},
		{"aliased kit import", map[string]string{"p/main_test.go": aliasedMain, "p/i_test.go": taggedTest}, 1, 0},
		{"another package imported as tmuxkit", map[string]string{"p/main_test.go": unrelatedMain, "p/i_test.go": taggedTest}, 1, 1},
		{"kit's own unqualified call", map[string]string{"internal/testkit/tmuxkit/main_test.go": unqualifiedMain, "internal/testkit/tmuxkit/i_test.go": kitTaggedTest}, 1, 0},
		{"unqualified call outside the kit", map[string]string{"p/main_test.go": unqualifiedMain, "p/i_test.go": kitTaggedTest}, 1, 1},
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
