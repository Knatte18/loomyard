// seams.go finds the top-level tests marked `//lym:seam` by a static parse of the `_test.go` files of the module's package directories.

package impactset

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/scanner"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const seamMarker = "//lym:seam"

// SeamMalformed is a `//lym:seam` line whose argument is not one `<unit>#<Type>` token.
const SeamMalformed GuardScanKind = "seam-malformed"

// SeamTest is a test marked `//lym:seam`: the struct types its reflection walks, named by glyph.
type SeamTest struct {
	// Name is the top-level test function's name.
	Name string
	// Unit is the test's glyph unit: the package directory, or `<dir>_test` for a file whose package clause ends `_test`.
	Unit string
	// File is the worktree-relative, slash-separated path of the test file declaring the test.
	File string
	// Structs holds each marker's glyph, in comment order.
	Structs []string
}

// FindSeamTests returns, per package directory, the tests carrying at least one `//lym:seam <unit>#<Type>` line in their doc comment, sorted by name.
// The caller passes the package directories, worktree-relative and slash-separated.
// A marker not in the comment block directly above a top-level `func Test…` line is a GuardMisplaced error, and a marker whose argument is not one `<unit>#<Type>` token is a SeamMalformed error, each naming the file and line.
// Whether a glyph names a struct is not checked.
func FindSeamTests(worktree string, dirs []string) (map[string][]SeamTest, error) {
	seams := map[string][]SeamTest{}
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
			found, err := seamTestsInFile(filepath.Join(worktree, filepath.FromSlash(rel)), dir, rel)
			if err != nil {
				return nil, err
			}
			seams[dir] = append(seams[dir], found...)
		}
		sort.Slice(seams[dir], func(i, j int) bool { return seams[dir][i].Name < seams[dir][j].Name })
	}
	return seams, nil
}

// seamTestsInFile returns the tests marked in one file; dir is the file's package directory and rel names the file in an error.
func seamTestsInFile(path, dir, rel string) ([]SeamTest, error) {
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

	unit := dir
	if strings.HasSuffix(file.Name.Name, "_test") {
		unit = dir + "_test"
	}
	var tests []SeamTest
	placed := map[*ast.Comment]bool{}
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Doc == nil {
			continue
		}
		var structs []string
		for i, comment := range fn.Doc.List {
			if !isSeamLine(comment.Text) {
				continue
			}
			if fn.Recv != nil || !strings.HasPrefix(fn.Name.Name, "Test") || !onlyDirectiveLinesAfter(fn.Doc.List[i+1:]) {
				continue
			}
			placed[comment] = true
			glyph, ok := seamGlyph(comment.Text)
			if !ok {
				line := fset.Position(comment.Pos()).Line
				return nil, &GuardScanError{File: rel, Kind: SeamMalformed, Line: line, message: fmt.Sprintf("impactset: %s:%d: %q is not one %s <unit>#<Type> token; way forward: write the marker as `%s <unit>#<Type>`, one struct per line", rel, line, comment.Text, seamMarker, seamMarker)}
			}
			structs = append(structs, glyph)
		}
		if len(structs) > 0 {
			tests = append(tests, SeamTest{Name: fn.Name.Name, Unit: unit, File: rel, Structs: structs})
		}
	}

	for _, group := range file.Comments {
		for _, comment := range group.List {
			if !isSeamLine(comment.Text) || placed[comment] {
				continue
			}
			line := fset.Position(comment.Pos()).Line
			return nil, &GuardScanError{File: rel, Kind: GuardMisplaced, Line: line, message: fmt.Sprintf("impactset: %s:%d: %s is not in the comment block directly above a top-level func Test line; way forward: move the `%s` line into the comment block directly above the `func Test…` line of the test that reflects", rel, line, seamMarker, seamMarker)}
		}
	}
	return tests, nil
}

// isSeamLine reports whether a comment is a `//lym:seam` marker line, with or without an argument.
func isSeamLine(text string) bool {
	rest, found := strings.CutPrefix(text, seamMarker)
	return found && (rest == "" || rest[0] == ' ' || rest[0] == '\t')
}

// seamGlyph returns the marker's argument when it is exactly one token holding one `#` with a non-empty unit and member.
func seamGlyph(text string) (string, bool) {
	fields := strings.Fields(strings.TrimPrefix(text, seamMarker))
	if len(fields) != 1 || strings.Count(fields[0], "#") != 1 {
		return "", false
	}
	unit, member, _ := strings.Cut(fields[0], "#")
	return fields[0], unit != "" && member != ""
}

// onlyDirectiveLinesAfter reports whether every comment is a `//lym:` or `//testtiming:keep` line, the only lines allowed between a seam marker and its func line.
func onlyDirectiveLinesAfter(comments []*ast.Comment) bool {
	for _, comment := range comments {
		if !strings.HasPrefix(comment.Text, "//lym:") && !strings.HasPrefix(comment.Text, testtimingKeepTag) {
			return false
		}
	}
	return true
}
