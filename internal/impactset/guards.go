// guards.go finds the top-level tests marked `//lym:guard` by a static parse of the `_test.go` files of the module's package directories.

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
	"sort"
	"strconv"
	"strings"

	"github.com/Knatte18/loomyard/internal/logger"
)

const (
	guardMarker       = "//lym:guard"
	testtimingKeepTag = "//testtiming:keep"
)

// GuardScanKind classifies a GuardScanError by the way forward its author has.
type GuardScanKind string

const (
	// GuardMisplaced is a marker not directly above a top-level `func Test…` line.
	GuardMisplaced GuardScanKind = "misplaced"
	// GuardWrongTier is a marker in a `tier2` or `tier3` test file, whose tier a gate runs only whole or by hand.
	GuardWrongTier GuardScanKind = "wrong-tier"
	// GuardParse is a file the Go parser rejects, including a file that cannot be read.
	GuardParse GuardScanKind = "parse"
)

// GuardTest is a test marked `//lym:guard`: its name and the tier of the file declaring it.
type GuardTest struct {
	// Name is the top-level test function's name.
	Name string
	// Tier is the N of the `tierN` tag the file's build constraint names, 0 for a file naming none.
	Tier int
}

// GuardScanError is a failure of the guard or seam scan that sits in the content of a test file: a file that does not parse, a `//lym:guard` or `//lym:seam` marker not directly above a top-level test, a guard marker in a `tier2` or `tier3` file, or a seam marker whose argument is not a `<unit>#<Type>` glyph.
// The author of the change can fix each one, unlike a failure to read a directory or git.
type GuardScanError struct {
	// File is the worktree-relative, slash-separated path of the test file the error sits in.
	File string
	// Kind is which of the four failures this is.
	Kind GuardScanKind
	// Line is the marker's line for the marker kinds, and the parser's line for GuardParse when the parser reports a position; 0 otherwise.
	Line int

	message string
}

func (e *GuardScanError) Error() string { return e.message }

// IsGuardScanError reports whether err is, or wraps, a GuardScanError.
func IsGuardScanError(err error) bool {
	var scanErr *GuardScanError
	return errors.As(err, &scanErr)
}

// FindGuardTests returns, per package directory, the tests marked `//lym:guard` in its `_test.go` files, sorted by name, each with the tier of its file.
// The caller passes the package directories, worktree-relative and slash-separated.
// A marker not directly above a top-level `func Test…` line, or in a `tier2` or `tier3` file, is an error naming the file and line.
func FindGuardTests(worktree string, dirs []string) (map[string][]GuardTest, error) {
	guards := map[string][]GuardTest{}
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
			found, err := guardNamesInFile(filepath.Join(worktree, filepath.FromSlash(rel)), rel)
			if err != nil {
				return nil, err
			}
			guards[dir] = append(guards[dir], found...)
		}
		sort.Slice(guards[dir], func(i, j int) bool { return guards[dir][i].Name < guards[dir][j].Name })
	}
	return guards, nil
}

// moduleDirsFormat is the `go list -f` template ModuleGuardTests reads: one NUL-terminated record per entry, its directory then its error, tab-separated.
const moduleDirsFormat = `{{.Dir}}{{"\t"}}{{with .Error}}{{.Err}}{{end}}{{"\x00"}}`

// ModuleGuardTests returns, per package directory of the module at moduleDir, the tests marked `//lym:guard` with their file's tier, keyed by the module-relative, slash-separated directory.
// It lists the packages under the tier tags `tier0` through `tier3` with `go list -e`, so a package whose test file cannot be read stays listed and the scan reports that file as a GuardParse error.
// A failed `go list` run, or an entry with no directory such as an unreadable package directory, is a plain error naming moduleDir and the enumeration's own error, and so is a listed package directory outside moduleDir.
// A marker the scan rejects is a *GuardScanError.
func ModuleGuardTests(moduleDir string) (map[string][]GuardTest, error) {
	absolute, err := filepath.Abs(moduleDir)
	if err != nil {
		return nil, fmt.Errorf("impactset: resolve %s: %w", moduleDir, err)
	}
	cmd := exec.Command("go", "list", "-e", "-tags", "tier0,tier1,tier2,tier3", "-f", moduleDirsFormat, "./...")
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
			return nil, fmt.Errorf("impactset: list the packages of %s: package directory %s lies outside the module directory", absolute, dir)
		}
		dirs = append(dirs, filepath.ToSlash(relative))
	}
	return FindGuardTests(absolute, dirs)
}

// guardNamesInFile returns the tests marked in one file, each with the file's tier; rel names the file in an error.
func guardNamesInFile(path, rel string) ([]GuardTest, error) {
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

	tier := FileTier(file)
	var names []GuardTest
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
			names = append(names, GuardTest{Name: fn.Name.Name, Tier: tier})
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
	if len(names) > 0 && tier > 1 {
		return nil, &GuardScanError{File: rel, Kind: GuardWrongTier, Line: firstMarkerLine, message: fmt.Sprintf("impactset: %s:%d: %s marks a guard test in a tier%d file; a guard test lives in a tier0 or tier1 file", rel, firstMarkerLine, guardMarker, tier)}
	}
	return names, nil
}

// onlyKeepLinesAfter reports whether every comment is a `//testtiming:keep` or `//lym:seam` line, the only lines allowed between a marker and its func line.
func onlyKeepLinesAfter(comments []*ast.Comment) bool {
	for _, comment := range comments {
		if !strings.HasPrefix(comment.Text, testtimingKeepTag) && !isSeamLine(comment.Text) {
			return false
		}
	}
	return true
}

// FileTier returns the tier of a test file: the N of the `tierN` tag its `//go:build` constraint names, 0 when it names none.
// A negated tag does not count, and a constraint naming several tier tags reads as the lowest.
func FileTier(file *ast.File) int {
	expr := buildConstraint(file)
	if expr == nil {
		return 0
	}
	tier := -1
	for _, number := range positiveTierTags(expr) {
		if tier < 0 || number < tier {
			tier = number
		}
	}
	return max(tier, 0)
}

// positiveTierTags returns the N of every `tierN` tag the expression names outside a negation.
func positiveTierTags(expr constraint.Expr) []int {
	switch x := expr.(type) {
	case *constraint.TagExpr:
		number, err := strconv.Atoi(strings.TrimPrefix(x.Tag, "tier"))
		if err != nil || !strings.HasPrefix(x.Tag, "tier") {
			return nil
		}
		return []int{number}
	case *constraint.AndExpr:
		return append(positiveTierTags(x.X), positiveTierTags(x.Y)...)
	case *constraint.OrExpr:
		return append(positiveTierTags(x.X), positiveTierTags(x.Y)...)
	}
	return nil
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
