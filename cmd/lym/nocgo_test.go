//go:build tier0

// nocgo_test.go enforces that no Go file of the module imports "C".
// Under `-trimpath` the Go build cache shares a cgo package's generated files between worktrees, and those files name the source by the absolute path of the checkout that first built it,
// so `go vet` in another worktree opens that path and fails once the first worktree is torn down.
// A cgo dependency in the module cache sits at the same path in every worktree and stays allowed; see `PATTERN-gate-slot`.

package main

import (
	"fmt"
	"go/parser"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
)

// cgoImportFailures walks the Go files under opts and returns one failure line per file that imports "C", with the number of files scanned.
func cgoImportFailures(t *testing.T, opts scankit.Options) (failures []string, scanned int) {
	t.Helper()
	opts.Filter = scankit.All
	scanned = scankit.Walk(t, opts, func(f *scankit.File) {
		file := f.AST(t, parser.ImportsOnly)
		for _, imp := range file.Imports {
			if imp.Path.Value != `"C"` {
				continue
			}
			rel := filepath.ToSlash(f.Rel)
			failures = append(failures, fmt.Sprintf("%s:%d: imports \"C\"; reach the C code through a dependency in the module cache, since a cgo package in the module makes `go vet` in one worktree open another worktree's files", rel, f.FileSet().Position(imp.Pos()).Line))
		}
	})
	sort.Strings(failures)
	return failures, scanned
}

// TestNoCgo_NoFileOfTheModuleImportsC fails for every Go file of the module that imports "C".
//
//lym:guard
func TestNoCgo_NoFileOfTheModuleImportsC(t *testing.T) {
	t.Parallel()
	failures, scanned := cgoImportFailures(t, scankit.Options{})
	scankit.RequireFloor(t, scanned, 1, "cgo import scan")
	if len(failures) > 0 {
		t.Errorf("`PATTERN-gate-slot` violated:\n%s", strings.Join(failures, "\n"))
	}
}

func TestNoCgo_FixtureTrees(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		files    map[string]string
		wantFail int
	}{
		{"CgoImport", map[string]string{"p/a.go": "package p\n\n// int f(void) { return 1; }\nimport \"C\"\n"}, 1},
		{"CgoImportInGroup", map[string]string{"p/a.go": "package p\n\nimport (\n\t\"C\"\n\t\"os\"\n)\n\nvar _ = os.Args\n"}, 1},
		{"CgoImportInTestFile", map[string]string{"p/a_test.go": "package p\n\nimport \"C\"\n"}, 1},
		{"MentionInComment", map[string]string{"p/a.go": "package p\n\n// import \"C\" is refused.\nimport \"os\"\n\nvar _ = os.Args\n"}, 0},
		{"MentionInString", map[string]string{"p/a.go": "package p\n\nvar s = `import \"C\"`\n"}, 0},
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
			failures, _ := cgoImportFailures(t, scankit.Options{Base: base})
			if len(failures) != tt.wantFail {
				t.Errorf("cgoImportFailures() = %v; want %d failures", failures, tt.wantFail)
			}
		})
	}
}
