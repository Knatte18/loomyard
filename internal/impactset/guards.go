// guards.go finds the top-level tests marked `//lyx:guard` by a static parse of the `_test.go` files of the module's package directories.

package impactset

import (
	"fmt"
	"go/ast"
	"go/build/constraint"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

const (
	guardMarker       = "//lyx:guard"
	testtimingKeepTag = "//testtiming:keep"
)

// findGuardTests returns, per package directory, the sorted names of the tests marked `//lyx:guard` in its `_test.go` files.
// A marker not directly above a top-level `func Test…` line, or in a file no `integration`-tier run compiles, is an error naming the file and line.
func findGuardTests(worktree string, dirs []string) (map[string][]string, error) {
	guards := map[string][]string{}
	for _, dir := range dirs {
		entries, err := os.ReadDir(filepath.Join(worktree, filepath.FromSlash(dir)))
		if err != nil {
			return nil, fmt.Errorf("impactset: read %s: %w", dir, err)
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), "_test.go") {
				continue
			}
			rel := dir + "/" + entry.Name()
			if dir == "." {
				rel = entry.Name()
			}
			names, err := guardNamesInFile(filepath.Join(worktree, filepath.FromSlash(rel)), rel)
			if err != nil {
				return nil, err
			}
			guards[dir] = append(guards[dir], names...)
		}
		sort.Strings(guards[dir])
	}
	return guards, nil
}

// guardNamesInFile returns the names of the tests marked in one file; rel names the file in an error.
func guardNamesInFile(path, rel string) ([]string, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("impactset: parse %s: %w", rel, err)
	}

	var names []string
	placed := map[*ast.Comment]bool{}
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Doc == nil {
			continue
		}
		for i, comment := range fn.Doc.List {
			if comment.Text != guardMarker {
				continue
			}
			if fn.Recv != nil || !strings.HasPrefix(fn.Name.Name, "Test") || !onlyKeepLinesAfter(fn.Doc.List[i+1:]) {
				continue
			}
			placed[comment] = true
			names = append(names, fn.Name.Name)
		}
	}

	for _, group := range file.Comments {
		for _, comment := range group.List {
			if comment.Text != guardMarker || placed[comment] {
				continue
			}
			return nil, fmt.Errorf("impactset: %s:%d: %s is not directly above a top-level func Test line", rel, fset.Position(comment.Pos()).Line, guardMarker)
		}
	}
	if len(names) > 0 && !compiledUnderIntegration(file) {
		return nil, fmt.Errorf("impactset: %s: marks a guard test in a file no integration-tier run compiles", rel)
	}
	return names, nil
}

// onlyKeepLinesAfter reports whether every comment is a `//testtiming:keep` line, the only line allowed between a marker and its func line.
func onlyKeepLinesAfter(comments []*ast.Comment) bool {
	for _, comment := range comments {
		if !strings.HasPrefix(comment.Text, testtimingKeepTag) {
			return false
		}
	}
	return true
}

// compiledUnderIntegration reports whether the file's build constraint holds with no tag or with the `integration` tag alone, on the host platform.
// A file with no constraint always holds.
func compiledUnderIntegration(file *ast.File) bool {
	expr := buildConstraint(file)
	if expr == nil {
		return true
	}
	for _, integration := range []bool{false, true} {
		if expr.Eval(func(tag string) bool {
			return tag == runtime.GOOS || tag == runtime.GOARCH || tag == "gc" || tag == "cgo" || (tag == "unix" && runtime.GOOS != "windows") || (tag == "integration" && integration)
		}) {
			return true
		}
	}
	return false
}

// buildConstraint returns the `//go:build` expression in the comments above the package clause, nil when there is none.
func buildConstraint(file *ast.File) constraint.Expr {
	for _, group := range file.Comments {
		if group.End() > file.Package {
			break
		}
		for _, comment := range group.List {
			if constraint.IsGoBuild(comment.Text) {
				expr, err := constraint.Parse(comment.Text)
				if err == nil {
					return expr
				}
			}
		}
	}
	return nil
}
