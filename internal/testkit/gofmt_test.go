// gofmt_test.go enforces that the module's Go files are gofmt-formatted, whatever their build tags.
// Fixtures under testdata directories are exempt.
// It compares each source with go/format.Source's output for that source, so no gofmt binary is needed.
// It lives in this small package, not in cmd/lym, so that its test binary stays cheap enough for every task's tier0.

package testkit

import (
	"bytes"
	"fmt"
	"go/format"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// gofmtScanFloor is the fewest Go files the walk may see; fewer means the walk is broken, not that the module shrank.
const gofmtScanFloor = 500

// gofmtFinding describes how src, the content of the module-relative file, differs from its gofmt form.
// It returns false for a formatted source.
// Otherwise the finding names the first differing line, or the line where only the file's ending differs.
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
	// A missing final newline or extra trailing blank lines leave every compared line equal.
	// The formatted form ends in one newline, so its split length is the line after its last one.
	line, difference := len(want), "the file's ending differs from its formatted form"
	for i := 0; i < len(current) || i < len(want); i++ {
		var have, wanted string
		if i < len(current) {
			have = current[i]
		}
		if i < len(want) {
			wanted = want[i]
		}
		if have != wanted {
			line, difference = i+1, fmt.Sprintf("current %q, formatted %q", have, wanted)
			break
		}
	}
	return fmt.Sprintf("%s:%d: not gofmt-formatted; %s; run \"lym loom fmt %s\" to rewrite the file to its formatted form", file, line, difference, file), true
}

// TestGofmt_ModuleFilesAreFormatted fails for every Go file of the module that differs from its gofmt form.
// Fixtures under testdata directories are exempt.
//
//lym:guard
func TestGofmt_ModuleFilesAreFormatted(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	rows := []struct {
		name string
		src  string
		want []string
	}{
		{"unformatted source names file, line, current and formatted text", "package a\n\nfunc f() {\nx:=1\n_ = x\n}\n", []string{"a.go:4:", `"x:=1"`, `"\tx := 1"`, `"lym loom fmt a.go"`}},
		{"unparseable source names the file", "package a\nfunc (", []string{"a.go", "does not parse"}},
		{"formatted source passes", "package a\n\nfunc f() {}\n", nil},
		{"missing final newline fails", "package a", []string{"a.go:2:", "ending differs", `"lym loom fmt a.go"`}},
		{"extra trailing blank lines fail", "package a\n\n\n", []string{"a.go:2:", "ending differs", `"lym loom fmt a.go"`}},
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
