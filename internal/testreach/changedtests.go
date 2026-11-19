// changedtests.go finds the test functions a diff adds or changes, from the file contents of both sides of the diff.

package testreach

import (
	"go/ast"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

// testFuncPrefix opens the name of every function `go test` runs as a test.
const testFuncPrefix = "Test"

// testMainName is the one Test-prefixed function `go test` runs as the binary's main, never as a test.
const testMainName = "TestMain"

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
	baseTests, err := declaredFuncsWhere(module, base, testFileImportPath, isTestFunc)
	if err != nil {
		return nil, err
	}
	headTests, err := declaredFuncsWhere(module, head, testFileImportPath, isTestFunc)
	if err != nil {
		return nil, err
	}
	ambiguous := ambiguousNames(baseTests, headTests)
	baseByID := map[string]declaredFunc{}
	for _, fn := range baseTests {
		baseByID[matchID(fn, ambiguous)] = fn
	}
	var changed []ChangedTest
	for _, fn := range headTests {
		before, found := baseByID[matchID(fn, ambiguous)]
		if found && before.hash == fn.hash {
			continue
		}
		changed = append(changed, ChangedTest{Package: fn.key.Package, File: fn.key.File, Name: fn.key.Func, New: !found})
	}
	slices.SortStableFunc(changed, func(a, b ChangedTest) int {
		if c := strings.Compare(a.Package, b.Package); c != 0 {
			return c
		}
		return strings.Compare(a.Name, b.Name)
	})
	return changed, nil
}

// testFileImportPath returns the import path of the directory that holds a test file.
// name is the file's worktree-relative slash path.
// It reports false for a file that is no test file of the module.
func testFileImportPath(module Module, name string) (string, bool) {
	if !strings.HasSuffix(name, "_test.go") {
		return "", false
	}
	return directoryImportPath(module, name)
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
