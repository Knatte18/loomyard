// guards.go finds the top-level tests marked `//lyx:guard` by a static parse of the `_test.go` files of the module's package directories.

package impactset

import (
	"errors"
	"fmt"
	"go/ast"
	"go/build/constraint"
	"go/parser"
	"go/scanner"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/Knatte18/loomyard/internal/logger"
)

const (
	guardMarker       = "//lyx:guard"
	testtimingKeepTag = "//testtiming:keep"
)

// GuardScanKind classifies a GuardScanError by the way forward its author has.
type GuardScanKind string

const (
	// GuardMisplaced is a marker not directly above a top-level `func Test…` line.
	GuardMisplaced GuardScanKind = "misplaced"
	// GuardNotIntegration is a marker in a file no `integration`-tier run compiles.
	GuardNotIntegration GuardScanKind = "not-integration"
	// GuardParse is a file the Go parser rejects, including a file that cannot be read.
	GuardParse GuardScanKind = "parse"
)

// GuardScanError is a failure of the guard scan that sits in the content of a test file: a file that does not parse, a marker not directly above a top-level test, or a marker in a file no `integration`-tier run compiles.
// The author of the change can fix each one, unlike a failure to read a directory or git.
type GuardScanError struct {
	// File is the worktree-relative, slash-separated path of the test file the error sits in.
	File string
	// Kind is which of the three failures this is.
	Kind GuardScanKind
	// Line is the marker's line for the two marker kinds, and the parser's line for GuardParse when the parser reports a position; 0 otherwise.
	Line int

	message string
}

func (e *GuardScanError) Error() string { return e.message }

// IsGuardScanError reports whether err is, or wraps, a GuardScanError.
func IsGuardScanError(err error) bool {
	var scanErr *GuardScanError
	return errors.As(err, &scanErr)
}

// FindGuardTests returns, per package directory, the sorted names of the tests marked `//lyx:guard` in its `_test.go` files.
// The caller passes the package directories, worktree-relative and slash-separated.
// A marker not directly above a top-level `func Test…` line, or in a file no `integration`-tier run compiles, is an error naming the file and line.
func FindGuardTests(worktree string, dirs []string) (map[string][]string, error) {
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

// moduleDirsFormat is the `go list -f` template ModuleGuardTests reads: one NUL-terminated record per entry, its directory then its error, tab-separated.
const moduleDirsFormat = `{{.Dir}}{{"\t"}}{{with .Error}}{{.Err}}{{end}}{{"\x00"}}`

// ModuleGuardTests returns, per package directory of the module at moduleDir, the sorted names of the tests marked `//lyx:guard`, keyed by the module-relative, slash-separated directory.
// It lists the packages under the `integration`, `tmux` and `llm` tags with `go list -e`, so a package whose test file cannot be read stays listed and the scan reports that file as a GuardParse error.
// A failed `go list` run, or an entry with no directory such as an unreadable package directory, is a plain error naming moduleDir and the enumeration's own error.
// A marker the scan rejects is a *GuardScanError.
func ModuleGuardTests(moduleDir string) (map[string][]string, error) {
	absolute, err := filepath.Abs(moduleDir)
	if err != nil {
		return nil, fmt.Errorf("impactset: resolve %s: %w", moduleDir, err)
	}
	cmd := exec.Command("go", "list", "-e", "-tags", "integration,tmux,llm", "-f", moduleDirsFormat, "./...")
	cmd.Dir = moduleDir
	logger.Info("impactset: spawning go list", "dir", moduleDir)
	output, err := cmd.Output()
	logger.Info("impactset: go list exited", "dir", moduleDir, "err", err)
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return nil, fmt.Errorf("impactset: list the packages of %s: %w: %s", moduleDir, err, strings.TrimSpace(string(exitErr.Stderr)))
		}
		return nil, fmt.Errorf("impactset: list the packages of %s: %w", moduleDir, err)
	}

	var dirs []string
	for _, record := range strings.Split(string(output), "\x00") {
		record = strings.TrimPrefix(record, "\n")
		if record == "" {
			continue
		}
		dir, listError, _ := strings.Cut(record, "\t")
		if dir == "" {
			return nil, fmt.Errorf("impactset: list the packages of %s: %s", moduleDir, strings.TrimSpace(listError))
		}
		relative, err := filepath.Rel(absolute, dir)
		if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			continue
		}
		dirs = append(dirs, filepath.ToSlash(relative))
	}
	return FindGuardTests(absolute, dirs)
}

// guardNamesInFile returns the names of the tests marked in one file; rel names the file in an error.
func guardNamesInFile(path, rel string) ([]string, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if err != nil {
		parseLine := 0
		var positions scanner.ErrorList
		if errors.As(err, &positions) && len(positions) > 0 {
			parseLine = positions[0].Pos.Line
		}
		return nil, &GuardScanError{File: rel, Kind: GuardParse, Line: parseLine, message: fmt.Sprintf("impactset: parse %s: %v", rel, err)}
	}

	var names []string
	firstMarkerLine := 0
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
			if firstMarkerLine == 0 {
				firstMarkerLine = fset.Position(comment.Pos()).Line
			}
			names = append(names, fn.Name.Name)
		}
	}

	for _, group := range file.Comments {
		for _, comment := range group.List {
			if comment.Text != guardMarker || placed[comment] {
				continue
			}
			markerLine := fset.Position(comment.Pos()).Line
			return nil, &GuardScanError{File: rel, Kind: GuardMisplaced, Line: markerLine, message: fmt.Sprintf("impactset: %s:%d: %s is not directly above a top-level func Test line", rel, markerLine, guardMarker)}
		}
	}
	if len(names) > 0 && !compiledUnderIntegration(file) {
		return nil, &GuardScanError{File: rel, Kind: GuardNotIntegration, Line: firstMarkerLine, message: fmt.Sprintf("impactset: %s:%d: %s marks a guard test in a file no integration-tier run compiles", rel, firstMarkerLine, guardMarker)}
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
