package scankit

import (
	"fmt"
	"go/parser"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func writeTree(t *testing.T, files map[string]string) string {
	t.Helper()
	base := t.TempDir()
	for rel, body := range files {
		p := filepath.Join(base, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return base
}

func rels(t *testing.T, opts Options) []string {
	t.Helper()
	var got []string
	n := Walk(t, opts, func(f *File) { got = append(got, f.Rel) })
	if n != len(got) {
		t.Fatalf("Walk returned %d, visited %d", n, len(got))
	}
	return got
}

func TestRoot_HasGoMod(t *testing.T) {
	tiergate.Require(t, 0)
	if _, err := os.Stat(filepath.Join(Root(t), "go.mod")); err != nil {
		t.Fatal(err)
	}
}

func TestWalk_Filters(t *testing.T) {
	tiergate.Require(t, 0)
	base := writeTree(t, map[string]string{
		"a.go":              "package a",
		"a_test.go":         "package a",
		"sub/b.go":          "package b",
		"sub/deep/c.go":     "package c",
		"sub/notes.md":      "x",
		"_lym/skip.go":      "package s",
		".git/skip.go":      "package s",
		"pkg/testdata/s.go": "package s",
		"my_testdata/s.go":  "package s",
	})
	tests := []struct {
		name string
		opts Options
		want []string
	}{
		{"prod", Options{Base: base}, []string{"a.go", "sub/b.go", "sub/deep/c.go"}},
		{"test", Options{Base: base, Filter: Test}, []string{"a_test.go"}},
		{"all", Options{Base: base, Filter: All}, []string{"a.go", "a_test.go", "sub/b.go", "sub/deep/c.go"}},
		{"ext", Options{Base: base, Exts: []string{".md"}}, []string{"sub/notes.md"}},
		{"shallow", Options{Base: base, Shallow: true}, []string{"a.go"}},
		{"shallow root", Options{Base: base, Roots: []string{"sub"}, Shallow: true}, []string{"sub/b.go"}},
		{"roots", Options{Base: base, Roots: []string{"sub/deep"}}, []string{"sub/deep/c.go"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := rels(t, tt.opts); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestWalk_FileContents(t *testing.T) {
	tiergate.Require(t, 0)
	base := writeTree(t, map[string]string{"a.go": "package a\n\nfunc F() {}\n"})
	Walk(t, Options{Base: base}, func(f *File) {
		if string(f.Data) != "package a\n\nfunc F() {}\n" {
			t.Errorf("Data = %q", f.Data)
		}
		if f.AST(t, parser.ParseComments).Name.Name != "a" {
			t.Error("AST package name")
		}
		if f.AST(t, 0) != f.AST(t, 0) {
			t.Error("AST not cached")
		}
	})
}

func TestAllowlist_Matching(t *testing.T) {
	tiergate.Require(t, 0)
	a := NewAllowlist([]Entry{
		{Key: "x/exact.go", Why: "r"},
		{Key: "dir/", Why: "r"},
	})
	for key, want := range map[string]bool{
		"x/exact.go":      true,
		"x/exact.go.more": false,
		"dir/a/b.go":      true,
		"other/a.go":      false,
	} {
		if got := a.Allowed(key); got != want {
			t.Errorf("Allowed(%q) = %v, want %v", key, got, want)
		}
	}
}

func TestAllowlist_Stale(t *testing.T) {
	tiergate.Require(t, 0)
	a := NewAllowlist([]Entry{{Key: "used"}, {Key: "unused/"}, {Key: "also-unused"}})
	a.Allowed("used")
	want := []string{"also-unused", "unused/"}
	if got := a.Stale(); !reflect.DeepEqual(got, want) {
		t.Fatalf("Stale = %v, want %v", got, want)
	}
	a.Allowed("unused/x")
	a.Allowed("also-unused")
	if got := a.Stale(); len(got) != 0 {
		t.Fatalf("Stale = %v, want none", got)
	}
}

func TestFloorError(t *testing.T) {
	tiergate.Require(t, 0)
	if err := floorError(1, 1, "scan"); err != nil {
		t.Fatalf("at floor: %v", err)
	}
	err := floorError(0, 1, "my scan")
	if err == nil || !strings.Contains(err.Error(), "my scan") {
		t.Fatalf("below floor: %v", err)
	}
}

func TestImportViolations(t *testing.T) {
	tiergate.Require(t, 0)
	dir := writeTree(t, map[string]string{
		"a.go": "package p\n\nimport (\n\t\"fmt\"\n\t\"github.com/x/ok\"\n)\n",
		"b.go": "package p\n\nimport \"github.com/x/bad\"\n",
		// Test files and subdirectories are not part of the package's production imports.
		"a_test.go":  "package p\n\nimport \"github.com/x/testonly\"\n",
		"sub/c.go":   "package c\n\nimport \"github.com/x/sub\"\n",
		"c.go":       "package p\n\nimport \"net/http\"\n",
		"d.go":       "package p\n\nimport \"example.com/other\"\n",
		"README.txt": "not go",
	})
	got, err := importViolations(dir, []string{"github.com/x/ok", "github.com/x/testonly", "github.com/x/sub"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"b.go: github.com/x/bad", "d.go: example.com/other"}
	if !reflect.DeepEqual(got.violations, want) {
		t.Fatalf("violations = %v, want %v", got.violations, want)
	}
	// An allowed path only a test file or a subdirectory imports is unused by the package.
	wantUnused := []string{"github.com/x/sub", "github.com/x/testonly"}
	if !reflect.DeepEqual(got.unused, wantUnused) {
		t.Fatalf("unused = %v, want %v", got.unused, wantUnused)
	}
	if got.parsed != 4 {
		t.Fatalf("parsed = %d, want 4", got.parsed)
	}
}

func TestImportViolations_Clean(t *testing.T) {
	tiergate.Require(t, 0)
	dir := writeTree(t, map[string]string{"a.go": "package p\n\nimport (\n\t\"os\"\n\t\"github.com/x/ok\"\n)\n"})
	got, err := importViolations(dir, []string{"github.com/x/ok"})
	if err != nil || len(got.violations) != 0 || len(got.unused) != 0 || got.parsed != 1 {
		t.Fatalf("got %+v, %v", got, err)
	}
}

func TestImportViolations_Errors(t *testing.T) {
	tiergate.Require(t, 0)
	if _, err := importViolations(filepath.Join(t.TempDir(), "missing"), nil); err == nil {
		t.Error("missing dir: want error")
	}
	dir := writeTree(t, map[string]string{"a.go": "not go at all"})
	if _, err := importViolations(dir, nil); err == nil {
		t.Error("unparsable file: want error")
	}
}

// recordingTB records the failures an assertion reports instead of failing the test.
// Its Fatalf returns to the caller, and any other testing.TB method panics on the nil embedded value.
type recordingTB struct {
	testing.TB
	errors []string
	fatals []string
}

func (r *recordingTB) Helper() {}

func (r *recordingTB) Error(args ...any) { r.errors = append(r.errors, fmt.Sprint(args...)) }

func (r *recordingTB) Errorf(format string, args ...any) {
	r.errors = append(r.errors, fmt.Sprintf(format, args...))
}

func (r *recordingTB) Fatalf(format string, args ...any) {
	r.fatals = append(r.fatals, fmt.Sprintf(format, args...))
}

func TestBannedImports(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tree := writeTree(t, map[string]string{
		"p/a.go":       "package p\n\nimport (\n\t\"os/exec\"\n\t\"os\"\n\t\"github.com/x/bad\"\n\t\"fmt\"\n)\n",
		"p/a_test.go":  "package p\n\nimport \"net\"\n",
		"p/sub/b.go":   "package sub\n\nimport \"net\"\n",
		"clean/a.go":   "package p\n\nimport \"fmt\"\n",
		"broken/a.go":  "not go at all",
		"empty/README": "no go files",
	})
	scans := []struct {
		name       string
		dir        string
		banned     []string
		want       []string
		wantParsed int
		wantErr    bool
	}{
		{name: "Exact", dir: "p", banned: []string{"os/exec", "os"}, want: []string{"a.go: os/exec", "a.go: os"}, wantParsed: 1},
		{name: "Subtree", dir: "p", banned: []string{"os/..."}, want: []string{"a.go: os/exec"}, wantParsed: 1},
		{name: "NonStdlib", dir: "p", banned: []string{"github.com/x/bad"}, want: []string{"a.go: github.com/x/bad"}, wantParsed: 1},
		{name: "TestFileAndSubdirectoryNotScanned", dir: "p", banned: []string{"net"}, wantParsed: 1},
		{name: "Clean", dir: "clean", banned: []string{"os"}, wantParsed: 1},
		{name: "MissingDir", dir: "missing", wantErr: true},
		{name: "Unparsable", dir: "broken", wantErr: true},
	}
	for _, tt := range scans {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, parsed, err := bannedImports(filepath.Join(tree, tt.dir), tt.banned)
			if (err != nil) != tt.wantErr {
				t.Fatalf("bannedImports() error = %v; want error %v", err, tt.wantErr)
			}
			if !reflect.DeepEqual(got, tt.want) || parsed != tt.wantParsed {
				t.Errorf("bannedImports() = %v, %d; want %v, %d", got, parsed, tt.want, tt.wantParsed)
			}
		})
	}

	// Each want list holds one fragment list per recorded message, in order.
	asserts := []struct {
		name       string
		dir        string
		wantFatals [][]string
		wantErrors [][]string
	}{
		{name: "AssertMissingDir", dir: "missing", wantFatals: [][]string{{"scankit:"}}},
		{name: "AssertEmptyDir", dir: "empty", wantErrors: [][]string{{"vacuous"}}},
		{name: "AssertBannedImport", dir: "p", wantErrors: [][]string{{filepath.Join(tree, "p"), "a.go: os/exec"}}},
	}
	for _, tt := range asserts {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			rec := &recordingTB{}
			AssertNoImports(rec, filepath.Join(tree, tt.dir), "os/exec")
			for _, got := range []struct {
				kind     string
				messages []string
				want     [][]string
			}{{"fatal", rec.fatals, tt.wantFatals}, {"error", rec.errors, tt.wantErrors}} {
				if len(got.messages) != len(got.want) {
					t.Fatalf("AssertNoImports() %s messages = %q; want %d", got.kind, got.messages, len(got.want))
				}
				for i, fragments := range got.want {
					for _, fragment := range fragments {
						if !strings.Contains(got.messages[i], fragment) {
							t.Errorf("AssertNoImports() %s = %q; want it to contain %q", got.kind, got.messages[i], fragment)
						}
					}
				}
			}
		})
	}
}

func TestAssertImportAllowlist_ThisPackage(t *testing.T) {
	tiergate.Require(t, 0)
	AssertImportAllowlist(t, "internal/testkit/scankit")
}
