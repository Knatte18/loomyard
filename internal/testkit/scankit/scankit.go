// Package scankit is the shared harness for source-scan invariant tests.
// It locates the module root, walks source files under the shared skip set,
// matches findings against allowlists that report their own stale entries,
// guards a scan against running vacuously, and checks a package's imports.
// It imports the standard library only.
package scankit

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
)

// FileFilter selects which files a walk yields by their test-file suffix.
type FileFilter int

const (
	// Prod yields files that do not end in `_test.go`.
	Prod FileFilter = iota
	// Test yields only files that end in `_test.go`.
	Test
	// All yields every file with a matching extension.
	All
)

// skipDirNames are directories no walk enters.
var skipDirNames = map[string]bool{
	".git":     true,
	"_lyx":     true,
	".lyx":     true,
	"_mill":    true,
	".scratch": true,
	".wiki":    true,
	"_raddle":  true,
}

func skipDir(name string) bool {
	return skipDirNames[name] || strings.Contains(name, "testdata")
}

// Options configures a Walk.
type Options struct {
	// Roots are directories to scan, relative to Base.
	// Empty means Base itself.
	Roots []string
	// Base is the absolute directory Roots resolve against.
	// Empty means the module root.
	Base string
	// Filter selects prod, test or all files; the zero value is Prod.
	Filter FileFilter
	// Exts are the file extensions to yield; empty means `.go`.
	Exts []string
	// Shallow scans only the files directly inside each root.
	Shallow bool
}

// File is one scanned file.
type File struct {
	// Rel is the slash-separated path relative to the walk's base.
	Rel  string
	Abs  string
	Data []byte

	ast  *ast.File
	fset *token.FileSet
	err  error
	done bool
}

// AST parses the file on first use and caches the result.
// A parse error fails the test through t.
func (f *File) AST(t testing.TB, mode parser.Mode) *ast.File {
	t.Helper()
	if !f.done {
		f.fset = token.NewFileSet()
		f.ast, f.err = parser.ParseFile(f.fset, f.Abs, f.Data, mode)
		f.done = true
	}
	if f.err != nil {
		t.Fatalf("parse %s: %v", f.Rel, f.err)
	}
	return f.ast
}

// FileSet returns the file set the last AST call parsed into.
func (f *File) FileSet() *token.FileSet { return f.fset }

// Root returns the module root, found from the kit's own source location and the enclosing go.mod.
func Root(t testing.TB) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("scankit: could not determine its own source location")
	}
	root, err := findModuleRoot(filepath.Dir(file))
	if err != nil {
		t.Fatalf("scankit: %v", err)
	}
	return root
}

func findModuleRoot(dir string) (string, error) {
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("no go.mod above the kit's source directory")
		}
		dir = parent
	}
}

// Walk visits every file the options select and returns how many it visited.
func Walk(t testing.TB, opts Options, fn func(f *File)) int {
	t.Helper()
	files, err := collect(opts, rootOrBase(t, opts))
	if err != nil {
		t.Fatalf("scankit: %v", err)
	}
	for i := range files {
		fn(&files[i])
	}
	return len(files)
}

func rootOrBase(t testing.TB, opts Options) string {
	if opts.Base != "" {
		return opts.Base
	}
	return Root(t)
}

func collect(opts Options, base string) ([]File, error) {
	exts := opts.Exts
	if len(exts) == 0 {
		exts = []string{".go"}
	}
	roots := opts.Roots
	if len(roots) == 0 {
		roots = []string{"."}
	}
	var files []File
	for _, r := range roots {
		start := filepath.Join(base, filepath.FromSlash(r))
		err := filepath.WalkDir(start, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if p == start {
					return nil
				}
				if opts.Shallow || skipDir(d.Name()) {
					return filepath.SkipDir
				}
				return nil
			}
			if !matches(d.Name(), exts, opts.Filter) {
				return nil
			}
			data, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			rel, err := filepath.Rel(base, p)
			if err != nil {
				return err
			}
			files = append(files, File{Rel: filepath.ToSlash(rel), Abs: p, Data: data})
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return files, nil
}

func matches(name string, exts []string, filter FileFilter) bool {
	ok := false
	for _, e := range exts {
		if strings.HasSuffix(name, e) {
			ok = true
			break
		}
	}
	if !ok {
		return false
	}
	isTest := strings.HasSuffix(name, "_test.go")
	switch filter {
	case Prod:
		return !isTest
	case Test:
		return isTest
	}
	return true
}

// Entry is one allowlist row: a key and the reason it is allowed.
type Entry struct {
	Key string
	Why string
}

// Allowlist matches keys against its entries and records which entries matched.
// An entry key ending in "/" matches every key under that path prefix; any other entry matches exactly.
type Allowlist struct {
	entries []Entry
	hit     map[string]bool
}

// NewAllowlist builds an allowlist from entries.
func NewAllowlist(entries []Entry) *Allowlist {
	return &Allowlist{entries: entries, hit: map[string]bool{}}
}

// Allowed reports whether key matches an entry, recording the match.
func (a *Allowlist) Allowed(key string) bool {
	found := false
	for _, e := range a.entries {
		if e.Key == key || (strings.HasSuffix(e.Key, "/") && strings.HasPrefix(key, e.Key)) {
			a.hit[e.Key] = true
			found = true
		}
	}
	return found
}

// Stale returns, sorted, the keys of entries that matched nothing.
func (a *Allowlist) Stale() []string {
	var stale []string
	for _, e := range a.entries {
		if !a.hit[e.Key] {
			stale = append(stale, e.Key)
		}
	}
	sort.Strings(stale)
	return stale
}

// RequireNoStale fails the test for every entry that matched nothing.
func (a *Allowlist) RequireNoStale(t testing.TB) {
	t.Helper()
	for _, k := range a.Stale() {
		t.Errorf("stale allowlist entry %q matched nothing; remove it", k)
	}
}

func floorError(scanned, min int, what string) error {
	if scanned < min {
		return fmt.Errorf("%s: scanned %d file(s), want at least %d; the scan is vacuous", what, scanned, min)
	}
	return nil
}

// RequireFloor fails the test when a scan visited fewer than min files.
func RequireFloor(t testing.TB, scanned, min int, what string) {
	t.Helper()
	if err := floorError(scanned, min, what); err != nil {
		t.Error(err)
	}
}

// importViolations returns the imports of the package's production files that are neither
// standard library nor in allowed, as `file: import` lines, and the number of files parsed.
func importViolations(dir string, allowed []string) ([]string, int, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, 0, err
	}
	ok := map[string]bool{}
	for _, a := range allowed {
		ok[a] = true
	}
	var violations []string
	parsed := 0
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(token.NewFileSet(), filepath.Join(dir, name), nil, parser.ImportsOnly)
		if err != nil {
			return nil, 0, err
		}
		parsed++
		for _, imp := range f.Imports {
			path := strings.Trim(imp.Path.Value, `"`)
			if isStdlib(path) || ok[path] {
				continue
			}
			violations = append(violations, name+": "+path)
		}
	}
	return violations, parsed, nil
}

func isStdlib(path string) bool {
	first, _, _ := strings.Cut(path, "/")
	return !strings.Contains(first, ".")
}

// AssertImportAllowlist fails the test when a production file in pkgDir imports anything beyond
// the standard library and the allowed import paths.
// pkgDir is a slash path relative to the module root, or absolute.
func AssertImportAllowlist(t testing.TB, pkgDir string, allowed ...string) {
	t.Helper()
	dir := pkgDir
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(Root(t), filepath.FromSlash(pkgDir))
	}
	violations, parsed, err := importViolations(dir, allowed)
	if err != nil {
		t.Fatalf("scankit: %v", err)
	}
	RequireFloor(t, parsed, 1, "import scan of "+pkgDir)
	for _, v := range violations {
		t.Errorf("%s imports outside its allowlist: %s", pkgDir, v)
	}
}
