// changedtests.go finds the test functions a diff adds or changes, from the file contents of both sides of the diff.

package testreach

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"maps"
	"path"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

// testFuncPrefix opens the name of every function `go test` runs as a test.
const testFuncPrefix = "Test"

// testMainName is the one Test-prefixed function `go test` runs as the binary's main, never as a test.
const testMainName = "TestMain"

// Module names the Go module whose test files a diff is read in.
type Module struct {
	// Dir is the absolute root directory of the module.
	Dir string
	// Rel is the module's worktree-relative slash path, empty or "." for a module at the worktree root.
	Rel string
	// Path is the module's import path.
	Path string
}

// FileSet maps a worktree-relative slash path to the contents of the file at that path.
type FileSet map[string][]byte

// ChangedTest is one test function a diff adds or changes.
type ChangedTest struct {
	// Package is the import path of the directory that holds the test's file; an external test package shares it.
	Package string
	// File is the base name of the test's file.
	File string
	// Name is the test function's name.
	Name string
	// New is set when the base side holds no test the test matches.
	New bool
}

// declaredTest is one test function declaration of one side, with its declaration text.
type declaredTest struct {
	pkg, file, name string
	text            string
}

// ChangedTests returns the test functions of head that are new or whose declaration text differs from base.
// The result is sorted by package and name.
// A test function is a top-level function of a `_test.go` file of the module.
// Its name is `Test` and a suffix that opens with no lower-case letter, as `go test` names one; TestMain is none.
// Both sets hold the test files of every package whose tests the caller wants compared.
// A test is matched across the sets by package and name, whatever its file.
// A test moved verbatim to another file is therefore unchanged.
// A name that either side declares in more than one file of its package is matched by package, file and name instead.
// A test only base holds is no changed test.
// A file outside the module, or under testdata or a directory opening with "." or "_", is passed over;
// a file that does not parse is an error naming it.
func ChangedTests(module Module, base, head FileSet) ([]ChangedTest, error) {
	baseTests, err := declaredTests(module, base)
	if err != nil {
		return nil, err
	}
	headTests, err := declaredTests(module, head)
	if err != nil {
		return nil, err
	}
	ambiguous := ambiguousNames(baseTests, headTests)
	baseByID := map[string]declaredTest{}
	for _, fn := range baseTests {
		baseByID[matchID(fn, ambiguous)] = fn
	}
	var changed []ChangedTest
	for _, fn := range headTests {
		before, found := baseByID[matchID(fn, ambiguous)]
		if found && before.text == fn.text {
			continue
		}
		changed = append(changed, ChangedTest{Package: fn.pkg, File: fn.file, Name: fn.name, New: !found})
	}
	slices.SortStableFunc(changed, func(a, b ChangedTest) int {
		if c := strings.Compare(a.Package, b.Package); c != 0 {
			return c
		}
		return strings.Compare(a.Name, b.Name)
	})
	return changed, nil
}

// matchID returns the text that matches a test to the same test of the other side.
func matchID(fn declaredTest, ambiguous map[string]bool) string {
	nameID := fn.pkg + "|" + fn.name
	if ambiguous[nameID] {
		return fn.pkg + "|" + fn.file + "|" + fn.name
	}
	return nameID
}

// ambiguousNames returns the package-and-name texts that either side declares in more than one file.
func ambiguousNames(sides ...[]declaredTest) map[string]bool {
	ambiguous := map[string]bool{}
	for _, side := range sides {
		files := map[string]string{}
		for _, fn := range side {
			nameID := fn.pkg + "|" + fn.name
			if file, seen := files[nameID]; seen && file != fn.file {
				ambiguous[nameID] = true
			}
			files[nameID] = fn.file
		}
	}
	return ambiguous
}

// declaredTests returns the test functions of the test files of files that lie in the module, in path order.
func declaredTests(module Module, files FileSet) ([]declaredTest, error) {
	var tests []declaredTest
	for _, name := range slices.Sorted(maps.Keys(files)) {
		pkg, ok := testFileImportPath(module, name)
		if !ok {
			continue
		}
		fset := token.NewFileSet()
		parsed, err := parser.ParseFile(fset, name, files[name], parser.SkipObjectResolution)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", name, err)
		}
		file := fset.File(parsed.Pos())
		for _, decl := range parsed.Decls {
			fn, isFunc := decl.(*ast.FuncDecl)
			if !isFunc || !isTestFunc(fn) {
				continue
			}
			tests = append(tests, declaredTest{
				pkg:  pkg,
				file: path.Base(name),
				name: fn.Name.Name,
				text: string(files[name][file.Offset(fn.Pos()):file.Offset(fn.End())]),
			})
		}
	}
	return tests, nil
}

// testFileImportPath returns the import path of the directory that holds a test file.
// name is the file's worktree-relative slash path.
// It reports false for a file that is no test file of the module.
// It also reports false for a test file under testdata or a directory opening with "." or "_".
func testFileImportPath(module Module, name string) (string, bool) {
	if !strings.HasSuffix(name, "_test.go") {
		return "", false
	}
	rel := name
	if module.Rel != "" && module.Rel != "." {
		var inside bool
		if rel, inside = strings.CutPrefix(name, module.Rel+"/"); !inside {
			return "", false
		}
	}
	dir := path.Dir(rel)
	if dir == "." {
		return module.Path, true
	}
	for _, segment := range strings.Split(dir, "/") {
		if segment == "testdata" || strings.HasPrefix(segment, ".") || strings.HasPrefix(segment, "_") {
			return "", false
		}
	}
	return module.Path + "/" + dir, true
}

// isTestFunc reports whether fn is a function `go test` runs as a test: no receiver,
// and a name of testFuncPrefix and a suffix that opens with no lower-case letter, bar testMainName.
func isTestFunc(fn *ast.FuncDecl) bool {
	if fn.Recv != nil || fn.Name.Name == testMainName {
		return false
	}
	suffix, isTest := strings.CutPrefix(fn.Name.Name, testFuncPrefix)
	if !isTest {
		return false
	}
	first, _ := utf8.DecodeRuneInString(suffix)
	return suffix == "" || !unicode.IsLower(first)
}
