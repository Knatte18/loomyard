// watches.go finds the top-level tests marked `//lym:watch` by a static parse of `_test.go` files.
// It also names those of them that no changed path concerns.

package impactset

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/scanner"
	"go/token"
	"os"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strings"
)

const watchMarker = "//lym:watch"

// WatchMalformed is a `//lym:watch` line that names no pattern, or a pattern `path.Match` rejects.
const WatchMalformed GuardScanKind = "watch-malformed"

// WatchedTest is a test marked `//lym:watch`: the file patterns whose change makes a narrowed gate run it.
type WatchedTest struct {
	// Name is the top-level test function's name.
	Name string
	// File is the worktree-relative, slash-separated path of the test file declaring the test.
	File string
	// Patterns are the marker's `path.Match` patterns, in comment order.
	// A pattern holding a `/` matches a worktree-relative path, and any other matches a path's base name.
	Patterns []string
}

// FindWatchedTests returns, per package directory, the tests with a `//lym:watch <pattern>...` line in their doc comment.
// Each directory's tests are sorted by name.
// The caller passes the package directories, worktree-relative and slash-separated.
// A marker not in the comment block directly above a top-level `func Test…` line is a GuardMisplaced error.
// A marker naming no pattern, or a pattern `path.Match` rejects, is a WatchMalformed error.
// Each error names the file and line.
func FindWatchedTests(worktree string, dirs []string) (map[string][]WatchedTest, error) {
	watched := map[string][]WatchedTest{}
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
			found, err := watchedTestsInFile(filepath.Join(worktree, filepath.FromSlash(rel)), rel)
			if err != nil {
				return nil, err
			}
			watched[dir] = append(watched[dir], found...)
		}
		sort.Slice(watched[dir], func(i, j int) bool { return watched[dir][i].Name < watched[dir][j].Name })
	}
	return watched, nil
}

// UnwatchedTests returns, per package directory of watched, the sorted names of its tests that changedPaths leave alone.
// A test is left alone when no changed path matches one of its patterns and its own file did not change.
// A directory with no such test is left out, and an empty result is nil.
func UnwatchedTests(watched map[string][]WatchedTest, changedPaths []string) map[string][]string {
	var unwatched map[string][]string
	for dir, tests := range watched {
		for _, test := range tests {
			if slices.Contains(changedPaths, test.File) || slices.ContainsFunc(changedPaths, func(changed string) bool { return test.watches(changed) }) {
				continue
			}
			if unwatched == nil {
				unwatched = map[string][]string{}
			}
			unwatched[dir] = append(unwatched[dir], test.Name)
		}
		if names := unwatched[dir]; len(names) > 0 {
			sort.Strings(names)
			unwatched[dir] = slices.Compact(names)
		}
	}
	return unwatched
}

// watches reports whether one of the test's patterns matches the worktree-relative, slash-separated changed path.
func (w WatchedTest) watches(changed string) bool {
	for _, pattern := range w.Patterns {
		subject := path.Base(changed)
		if strings.Contains(pattern, "/") {
			subject = changed
		}
		if matched, _ := path.Match(pattern, subject); matched {
			return true
		}
	}
	return false
}

// watchedTestsInFile returns the tests marked in one file; rel names the file in a test and an error.
func watchedTestsInFile(filePath, rel string) ([]WatchedTest, error) {
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

	var tests []WatchedTest
	placed := map[*ast.Comment]bool{}
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Doc == nil {
			continue
		}
		var patterns []string
		for i, comment := range fn.Doc.List {
			if !isWatchLine(comment.Text) {
				continue
			}
			if fn.Recv != nil || !strings.HasPrefix(fn.Name.Name, "Test") || !onlyDirectiveLinesAfter(fn.Doc.List[i+1:]) {
				continue
			}
			placed[comment] = true
			fields := strings.Fields(strings.TrimPrefix(comment.Text, watchMarker))
			if bad := malformedWatch(fields); bad != "" {
				line := fset.Position(comment.Pos()).Line
				return nil, &GuardScanError{File: rel, Kind: WatchMalformed, Line: line, message: fmt.Sprintf("impactset: %s:%d: %s; way forward: write the marker as `%s <pattern>...` with `path.Match` patterns", rel, line, bad, watchMarker)}
			}
			patterns = append(patterns, fields...)
		}
		if len(patterns) > 0 {
			tests = append(tests, WatchedTest{Name: fn.Name.Name, File: rel, Patterns: patterns})
		}
	}

	for _, group := range file.Comments {
		for _, comment := range group.List {
			if !isWatchLine(comment.Text) || placed[comment] {
				continue
			}
			line := fset.Position(comment.Pos()).Line
			return nil, &GuardScanError{File: rel, Kind: GuardMisplaced, Line: line, message: fmt.Sprintf("impactset: %s:%d: %s is not in the comment block directly above a top-level func Test line; way forward: move the `%s` line into the comment block directly above the `func Test…` line of the test it scopes", rel, line, watchMarker, watchMarker)}
		}
	}
	return tests, nil
}

// malformedWatch returns what is wrong with a marker's patterns, or "" when there is at least one and each is valid.
func malformedWatch(patterns []string) string {
	if len(patterns) == 0 {
		return watchMarker + " names no pattern"
	}
	for _, pattern := range patterns {
		if _, err := path.Match(pattern, ""); err != nil {
			return fmt.Sprintf("%s pattern %q is malformed: %v", watchMarker, pattern, err)
		}
	}
	return ""
}

// isWatchLine reports whether a comment is a `//lym:watch` marker line, with or without patterns.
func isWatchLine(text string) bool {
	rest, found := strings.CutPrefix(text, watchMarker)
	return found && (rest == "" || rest[0] == ' ' || rest[0] == '\t')
}
