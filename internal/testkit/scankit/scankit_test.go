package scankit

import (
	"go/parser"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
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
	if _, err := os.Stat(filepath.Join(Root(t), "go.mod")); err != nil {
		t.Fatal(err)
	}
}

func TestWalk_Filters(t *testing.T) {
	base := writeTree(t, map[string]string{
		"a.go":              "package a",
		"a_test.go":         "package a",
		"sub/b.go":          "package b",
		"sub/deep/c.go":     "package c",
		"sub/notes.md":      "x",
		"_lyx/skip.go":      "package s",
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
	if err := floorError(1, 1, "scan"); err != nil {
		t.Fatalf("at floor: %v", err)
	}
	err := floorError(0, 1, "my scan")
	if err == nil || !strings.Contains(err.Error(), "my scan") {
		t.Fatalf("below floor: %v", err)
	}
}

func TestImportViolations(t *testing.T) {
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
	got, parsed, err := importViolations(dir, []string{"github.com/x/ok"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"b.go: github.com/x/bad", "d.go: example.com/other"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("violations = %v, want %v", got, want)
	}
	if parsed != 4 {
		t.Fatalf("parsed = %d, want 4", parsed)
	}
}

func TestImportViolations_Clean(t *testing.T) {
	dir := writeTree(t, map[string]string{"a.go": "package p\n\nimport \"os\"\n"})
	got, parsed, err := importViolations(dir, nil)
	if err != nil || len(got) != 0 || parsed != 1 {
		t.Fatalf("got %v, %d, %v", got, parsed, err)
	}
}

func TestImportViolations_Errors(t *testing.T) {
	if _, _, err := importViolations(filepath.Join(t.TempDir(), "missing"), nil); err == nil {
		t.Error("missing dir: want error")
	}
	dir := writeTree(t, map[string]string{"a.go": "not go at all"})
	if _, _, err := importViolations(dir, nil); err == nil {
		t.Error("unparsable file: want error")
	}
}

func TestAssertImportAllowlist_ThisPackage(t *testing.T) {
	AssertImportAllowlist(t, "internal/testkit/scankit")
}
