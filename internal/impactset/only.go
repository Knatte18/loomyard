// only.go finds the top-level tests marked `//lym:only <key>...` by a static parse of `_test.go` files.

package impactset

import (
	"context"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/scanner"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
)

const onlyMarker = "//lym:only"

// OnlyMalformed is a `//lym:only` line that names no gate key.
const OnlyMalformed GuardScanKind = "only-malformed"

// OnlyTest is a test marked `//lym:only`: the gate keys whose calls run it, and the tier of the file declaring it.
type OnlyTest struct {
	// Name is the top-level test function's name.
	Name string
	// Tier is the tier of the file declaring the test, from its `tiergate.Require` calls or else its build constraint.
	Tier int
	// Keys are the gate keys the marker names, sorted and without repeats.
	Keys []string
}

// FindOnlyTests returns, per package directory, the tests with a `//lym:only <key>...` line in their doc comment.
// Each directory's tests are sorted by name, and each test carries the tier of its file.
// The caller passes the package directories, worktree-relative and slash-separated.
// A marker not in the comment block directly above a top-level `func Test…` line is a GuardMisplaced error.
// A marker naming no key is an OnlyMalformed error.
// Each error names the file and line.
func FindOnlyTests(worktree string, dirs []string) (map[string][]OnlyTest, error) {
	only := map[string][]OnlyTest{}
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
			found, err := onlyTestsInFile(filepath.Join(worktree, filepath.FromSlash(rel)), rel)
			if err != nil {
				return nil, err
			}
			if len(found) > 0 {
				only[dir] = append(only[dir], found...)
			}
		}
		sort.Slice(only[dir], func(i, j int) bool { return only[dir][i].Name < only[dir][j].Name })
	}
	return only, nil
}

// ModuleOnlyTests returns, per package directory of the module at moduleDir, the tests marked `//lym:only`.
// The map is keyed by the module-relative, slash-separated directory.
// It lists the packages as ModuleGuardTests does and fails the same ways;
// a marker the scan rejects is a *GuardScanError.
// ctx bounds the listing, so a stalled `go list` is killed when it ends.
func ModuleOnlyTests(ctx context.Context, moduleDir string) (map[string][]OnlyTest, error) {
	absolute, dirs, err := modulePackageDirs(ctx, moduleDir)
	if err != nil {
		return nil, err
	}
	return FindOnlyTests(absolute, dirs)
}

// onlyTestsInFile returns the tests marked in one file, each with the file's tier; rel names the file in an error.
func onlyTestsInFile(filePath, rel string) ([]OnlyTest, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filePath, nil, parser.ParseComments)
	if err != nil {
		parseLine := 0
		var positions scanner.ErrorList
		if errors.As(err, &positions) && len(positions) > 0 {
			parseLine = positions[0].Pos.Line
		}
		return nil, &GuardScanError{File: rel, Kind: GuardParse, Line: parseLine, message: fmt.Sprintf("impactset: parse %s: %v", rel, err)}
	}

	tier := FileTier(file)
	var tests []OnlyTest
	placed := map[*ast.Comment]bool{}
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Doc == nil {
			continue
		}
		var keys []string
		for i, comment := range fn.Doc.List {
			if !isOnlyLine(comment.Text) {
				continue
			}
			if fn.Recv != nil || !strings.HasPrefix(fn.Name.Name, "Test") || !onlyDirectiveLinesAfter(fn.Doc.List[i+1:]) {
				continue
			}
			placed[comment] = true
			fields := strings.Fields(strings.TrimPrefix(comment.Text, onlyMarker))
			if len(fields) == 0 {
				line := fset.Position(comment.Pos()).Line
				return nil, &GuardScanError{File: rel, Kind: OnlyMalformed, Line: line, message: fmt.Sprintf("impactset: %s:%d: %s names no gate key; way forward: write the marker as `%s <key>...` with the gate keys that run the test", rel, line, onlyMarker, onlyMarker)}
			}
			keys = append(keys, fields...)
		}
		if len(keys) > 0 {
			slices.Sort(keys)
			tests = append(tests, OnlyTest{Name: fn.Name.Name, Tier: tier, Keys: slices.Compact(keys)})
		}
	}

	for _, group := range file.Comments {
		for _, comment := range group.List {
			if !isOnlyLine(comment.Text) || placed[comment] {
				continue
			}
			line := fset.Position(comment.Pos()).Line
			return nil, &GuardScanError{File: rel, Kind: GuardMisplaced, Line: line, message: fmt.Sprintf("impactset: %s:%d: %s is not in the comment block directly above a top-level func Test line; way forward: move the `%s` line into the comment block directly above the `func Test…` line of the test it holds back", rel, line, onlyMarker, onlyMarker)}
		}
	}
	return tests, nil
}

// isOnlyLine reports whether a comment is a `//lym:only` marker line, with or without keys.
func isOnlyLine(text string) bool {
	rest, found := strings.CutPrefix(text, onlyMarker)
	return found && (rest == "" || rest[0] == ' ' || rest[0] == '\t')
}
