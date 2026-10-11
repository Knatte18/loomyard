// gofmt_test.go enforces that every Go file in the module is gofmt-formatted, whatever its build tags.
// It compares each source with go/format.Source of itself, so no gofmt binary is needed.

package main

import (
	"bytes"
	"fmt"
	"go/format"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
)

// gofmtScanFloor is the fewest Go files the walk may see; fewer means the walk is broken, not that the module shrank.
const gofmtScanFloor = 500

// gofmtFinding reports how src, the content of the module-relative file, differs from its gofmt form.
// It answers false for a formatted source.
// The finding names the first differing line, so the fix is an Edit.
func gofmtFinding(file string, src []byte) (string, bool) {
	formatted, err := format.Source(src)
	if err != nil {
		return fmt.Sprintf("%s: does not parse: %v", file, err), true
	}
	if bytes.Equal(src, formatted) {
		return "", false
	}
	current := strings.Split(string(src), "\n")
	want := strings.Split(string(formatted), "\n")
	for i := 0; i < len(current) || i < len(want); i++ {
		var have, wanted string
		if i < len(current) {
			have = current[i]
		}
		if i < len(want) {
			wanted = want[i]
		}
		if have != wanted {
			return fmt.Sprintf("%s:%d: not gofmt-formatted; current %q, formatted %q; edit the file to match the formatted line", file, i+1, have, wanted), true
		}
	}
	return "", false
}

// TestGofmt_ModuleFilesAreFormatted fails for every Go file in the module that differs from its gofmt form.
//
//lyx:guard
func TestGofmt_ModuleFilesAreFormatted(t *testing.T) {
	t.Parallel()

	rows := []struct {
		name string
		src  string
		want []string
	}{
		{"unformatted source names file, line, current and formatted text", "package a\n\nfunc f() {\nx:=1\n_ = x\n}\n", []string{"a.go:4:", `"x:=1"`, `"\tx := 1"`}},
		{"unparseable source names the file", "package a\nfunc (", []string{"a.go", "does not parse"}},
		{"formatted source passes", "package a\n\nfunc f() {}\n", nil},
		{"import order alone still fails with that line", "package a\n\nimport (\n\t\"os\"\n\t\"fmt\"\n)\n\nvar _ = fmt.Sprint\nvar _ = os.Args\n", []string{"a.go:4:", `"\t\"os\""`, `"\t\"fmt\""`}},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			got, found := gofmtFinding("a.go", []byte(row.src))
			if found != (row.want != nil) {
				t.Fatalf("gofmtFinding found = %v (%q), want %v", found, got, row.want != nil)
			}
			for _, part := range row.want {
				if !strings.Contains(got, part) {
					t.Errorf("finding %q lacks %q", got, part)
				}
			}
		})
	}

	scanned := scankit.Walk(t, scankit.Options{Filter: scankit.All}, func(f *scankit.File) {
		if finding, found := gofmtFinding(f.Rel, f.Data); found {
			t.Errorf("%s", finding)
		}
	})
	scankit.RequireFloor(t, scanned, gofmtScanFloor, "gofmt scan")
}
