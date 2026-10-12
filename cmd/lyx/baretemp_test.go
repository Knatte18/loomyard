// baretemp_test.go enforces the Lyx Temp Root Invariant: no Go file outside internal/lyxtemp creates a temp file or directory directly in the bare system temp directory.
// A creation goes through lyxtemp.MkdirTemp or lyxtemp.CreateTemp, so it lands in the process run directory and is removed with it.
// Modelled on hermeticenv_test.go and tmuxisolation_test.go; see `PATTERN-lyx-temp-root`.

package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
)

// allowedBareTemp is the Lyx Temp Root Invariant allowlist, keyed `<file>:<call>`, for example `internal/x/y.go:os.MkdirTemp`.
// An entry admits one creation call in one file, and its reason names the obstacle to creating it through lyxtemp.
// An admitted site still removes what it creates on normal exit.
// What an entry lets through is that one site's directory left in the bare system temp directory after a kill.
var allowedBareTemp = []scankit.Entry{}

// lyxtempDir is the temp package's own directory, whose files create the run directories and so call the bare forms.
const lyxtempDir = "internal/lyxtemp/"

// osImportName returns the name the file binds the "os" import to, or "" when it does not import os under a usable name.
func osImportName(file *ast.File) string {
	for _, imp := range file.Imports {
		if strings.Trim(imp.Path.Value, `"`) != "os" {
			continue
		}
		if imp.Name == nil {
			return "os"
		}
		if imp.Name.Name == "_" || imp.Name.Name == "." {
			return ""
		}
		return imp.Name.Name
	}
	return ""
}

// isBareTempDirArg reports whether the expression is the empty string literal or a call of osName.TempDir().
func isBareTempDirArg(expr ast.Expr, osName string) bool {
	switch arg := expr.(type) {
	case *ast.BasicLit:
		return arg.Value == `""` || arg.Value == "``"
	case *ast.CallExpr:
		sel, ok := arg.Fun.(*ast.SelectorExpr)
		if !ok || len(arg.Args) != 0 || sel.Sel.Name != "TempDir" {
			return false
		}
		pkg, ok := sel.X.(*ast.Ident)
		return ok && pkg.Name == osName
	}
	return false
}

// bareTempFailures walks the Go files under opts and returns one failure line per osName.MkdirTemp or osName.CreateTemp call whose directory argument is the bare system temp directory, with the number of files scanned.
func bareTempFailures(t *testing.T, opts scankit.Options, allow *scankit.Allowlist) (failures []string, scanned int) {
	t.Helper()
	opts.Filter = scankit.All
	scanned = scankit.Walk(t, opts, func(f *scankit.File) {
		rel := filepath.ToSlash(f.Rel)
		if strings.HasPrefix(rel, lyxtempDir) {
			return
		}
		file := f.AST(t, parser.SkipObjectResolution)
		osName := osImportName(file)
		if osName == "" {
			return
		}
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) == 0 {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || (sel.Sel.Name != "MkdirTemp" && sel.Sel.Name != "CreateTemp") {
				return true
			}
			if pkg, ok := sel.X.(*ast.Ident); !ok || pkg.Name != osName || !isBareTempDirArg(call.Args[0], osName) {
				return true
			}
			name := "os." + sel.Sel.Name
			if allow.Allowed(rel + ":" + name) {
				return true
			}
			failures = append(failures, fmt.Sprintf("%s:%d: %s with the bare system temp directory; create it through lyxtemp.MkdirTemp or lyxtemp.CreateTemp", rel, f.FileSet().Position(call.Pos()).Line, name))
			return true
		})
	})
	sort.Strings(failures)
	return failures, scanned
}

// TestBareTemp_NoCreationInTheBareSystemTempDirectory fails for every os.MkdirTemp or os.CreateTemp call outside internal/lyxtemp whose directory is "" or os.TempDir(), unless allowlisted.
//
//lyx:guard
func TestBareTemp_NoCreationInTheBareSystemTempDirectory(t *testing.T) {
	allow := scankit.NewAllowlist(allowedBareTemp)
	failures, scanned := bareTempFailures(t, scankit.Options{}, allow)
	scankit.RequireFloor(t, scanned, 1, "bare temp scan")
	allow.RequireNoStale(t)
	if len(failures) > 0 {
		t.Errorf("`PATTERN-lyx-temp-root` violated:\n%s", strings.Join(failures, "\n"))
	}
}

func TestBareTemp_FixtureTrees(t *testing.T) {
	const header = "package p\n\nimport \"os\"\n\n"
	cases := []struct {
		name     string
		files    map[string]string
		wantFail int
	}{
		{"MkdirTemp with an empty dir", map[string]string{"p/a.go": header + "func f() { os.MkdirTemp(\"\", \"x\") }\n"}, 1},
		{"MkdirTemp with a raw empty dir", map[string]string{"p/a.go": header + "func f() { os.MkdirTemp(``, \"x\") }\n"}, 1},
		{"CreateTemp with an empty dir", map[string]string{"p/a.go": header + "func f() { os.CreateTemp(\"\", \"x\") }\n"}, 1},
		{"MkdirTemp with os.TempDir", map[string]string{"p/a.go": header + "func f() { os.MkdirTemp(os.TempDir(), \"x\") }\n"}, 1},
		{"CreateTemp with os.TempDir", map[string]string{"p/a.go": header + "func f() { os.CreateTemp(os.TempDir(), \"x\") }\n"}, 1},
		{"aliased os import", map[string]string{"p/a.go": "package p\n\nimport sys \"os\"\n\nfunc f() { sys.MkdirTemp(\"\", \"x\") }\n"}, 1},
		{"test file is scanned", map[string]string{"p/a_test.go": header + "func f() { os.MkdirTemp(\"\", \"x\") }\n"}, 1},
		{"mention in a comment", map[string]string{"p/a.go": header + "// os.MkdirTemp(\"\", \"x\") is bare.\nfunc f() { _ = os.Args }\n"}, 0},
		{"mention in a string", map[string]string{"p/a.go": header + "var s = `os.MkdirTemp(\"\", \"x\")`\n"}, 0},
		{"t.TempDir", map[string]string{"p/a_test.go": "package p\n\nimport (\n\t\"os\"\n\t\"testing\"\n)\n\nfunc TestX(t *testing.T) { os.MkdirTemp(t.TempDir(), \"x\") }\n"}, 0},
		{"named dir", map[string]string{"p/a.go": header + "func f(dir string) { os.MkdirTemp(dir, \"x\"); os.CreateTemp(os.Getenv(\"D\"), \"x\") }\n"}, 0},
		{"bare call inside lyxtemp", map[string]string{"internal/lyxtemp/a.go": header + "func f() { os.MkdirTemp(\"\", \"x\") }\n"}, 0},
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
			failures, _ := bareTempFailures(t, scankit.Options{Base: base}, scankit.NewAllowlist(nil))
			if len(failures) != tc.wantFail {
				t.Fatalf("failures = %v; want %d", failures, tc.wantFail)
			}
		})
	}
}
