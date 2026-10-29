//go:build tier0

// fslinkonly_test.go enforces `PATTERN-fs-links`: no production Go file outside internal/fslink calls `os.Symlink` or `os.Link`.
// A link is created through fslink.CreateDirLink.

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

// allowedLinkCalls is the `PATTERN-fs-links` allowlist, keyed `<file>:<call>`, for example `internal/x/y.go:os.Symlink`.
// An entry admits one link call in one file, and its reason names the obstacle to creating the link through fslink.
var allowedLinkCalls = []scankit.Entry{
	{Key: "tools/deploy/main.go:os.Symlink", Why: "copies a plugin-tree symlink verbatim, and it may point at a file, which fslink has no primitive for"},
}

// fslinkDir is the link package's own directory, whose files make the raw link calls.
const fslinkDir = "internal/fslink/"

// linkCallFailures walks the production Go files under opts and returns one failure line per osName.Symlink or osName.Link call outside internal/fslink, with the number of files scanned.
func linkCallFailures(t *testing.T, opts scankit.Options, allow *scankit.Allowlist) (failures []string, scanned int) {
	t.Helper()
	scanned = scankit.Walk(t, opts, func(f *scankit.File) {
		rel := filepath.ToSlash(f.Rel)
		if strings.HasPrefix(rel, fslinkDir) {
			return
		}
		file := f.AST(t, parser.SkipObjectResolution)
		for _, imp := range file.Imports {
			if imp.Name != nil && imp.Name.Name == "." && strings.Trim(imp.Path.Value, `"`) == "os" {
				failures = append(failures, fmt.Sprintf("%s:%d: dot import of os hides link calls; import os by name", rel, f.FileSet().Position(imp.Pos()).Line))
			}
		}
		osName := osImportName(file)
		if osName == "" {
			return
		}
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || (sel.Sel.Name != "Symlink" && sel.Sel.Name != "Link") {
				return true
			}
			if pkg, ok := sel.X.(*ast.Ident); !ok || pkg.Name != osName {
				return true
			}
			name := "os." + sel.Sel.Name
			if allow.Allowed(rel + ":" + name) {
				return true
			}
			failures = append(failures, fmt.Sprintf("%s:%d: %s outside internal/fslink; create the link through fslink.CreateDirLink", rel, f.FileSet().Position(call.Pos()).Line, name))
			return true
		})
	})
	sort.Strings(failures)
	return failures, scanned
}

// TestFslinkOnly_NoRawLinkCalls fails for every production os.Symlink or os.Link call outside internal/fslink, unless allowlisted.
//
//lym:guard
func TestFslinkOnly_NoRawLinkCalls(t *testing.T) {
	t.Parallel()
	allow := scankit.NewAllowlist(allowedLinkCalls)
	failures, scanned := linkCallFailures(t, scankit.Options{}, allow)
	scankit.RequireFloor(t, scanned, 20, "fslink-only scan")
	allow.RequireNoStale(t)
	if len(failures) > 0 {
		t.Errorf("`PATTERN-fs-links` violated:\n%s", strings.Join(failures, "\n"))
	}
}

func TestFslinkOnly_FixtureTrees(t *testing.T) {
	t.Parallel()
	const header = "package p\n\nimport \"os\"\n\n"
	tests := []struct {
		name     string
		files    map[string]string
		wantFail int
	}{
		{"Symlink", map[string]string{"p/a.go": header + "func f() { os.Symlink(\"t\", \"l\") }\n"}, 1},
		{"Link", map[string]string{"p/a.go": header + "func f() { os.Link(\"t\", \"l\") }\n"}, 1},
		{"aliased os import", map[string]string{"p/a.go": "package p\n\nimport sys \"os\"\n\nfunc f() { sys.Symlink(\"t\", \"l\") }\n"}, 1},
		{"dot import of os", map[string]string{"p/a.go": "package p\n\nimport . \"os\"\n\nfunc f() { Symlink(\"t\", \"l\") }\n"}, 1},
		{"blank import of os", map[string]string{"p/a.go": "package p\n\nimport _ \"os\"\n"}, 0},
		{"call inside fslink", map[string]string{"internal/fslink/a.go": header + "func f() { os.Symlink(\"t\", \"l\") }\n"}, 0},
		{"mention in a comment", map[string]string{"p/a.go": header + "// os.Symlink(\"t\", \"l\") is raw.\nfunc f() { _ = os.Args }\n"}, 0},
		{"Readlink", map[string]string{"p/a.go": header + "func f() { os.Readlink(\"l\") }\n"}, 0},
		{"test file is not scanned", map[string]string{"p/a_test.go": header + "func f() { os.Symlink(\"t\", \"l\") }\n"}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			base := t.TempDir()
			for rel, body := range tt.files {
				path := filepath.Join(base, filepath.FromSlash(rel))
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			failures, _ := linkCallFailures(t, scankit.Options{Base: base}, scankit.NewAllowlist(nil))
			if len(failures) != tt.wantFail {
				t.Errorf("linkCallFailures() = %v; want %d failures", failures, tt.wantFail)
			}
		})
	}
}
