// reflow.go finds the doc-comment groups that reflow may change and splices their reflowed text into the source.
// It edits by byte offset and never goes through go/printer, so nothing outside the touched comment lines changes.

package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"slices"
	"sort"
	"strings"

	"github.com/Knatte18/loomyard/internal/commentlint"
)

// edit is a byte-offset replacement span into the original source.
type edit struct {
	start, end int
	// line is the 1-based source line of the comment group.
	line int
	// old is the text being replaced, for -check display.
	old  string
	text string
}

// lineShift returns how many lines the edit adds to the file.
func (e edit) lineShift() int {
	return strings.Count(e.text, "\n") - strings.Count(e.old, "\n")
}

// groupLongLines holds the prose lines of one comment group that reach the length limit.
type groupLongLines struct {
	// line is the 1-based source line of the comment group.
	line int
	// indexes are the 0-based indexes, within the group's final text, of those lines.
	indexes []int
}

// collectEdits parses src and returns every reflow edit it qualifies for,
// and the line numbers, in the reflowed file, of the prose lines that still reach the length limit.
// Those lines are in comment groups the tool may reflow, whether or not the group needs an edit.
// limit is the comment line-length limit, in characters of the text after "//".
// err is non-nil only on a parse failure; a generated file yields no edits and no error.
func collectEdits(fset *token.FileSet, filename, src string, limit int) ([]edit, []int, error) {
	if commentlint.IsGenerated(src) {
		return nil, nil, nil
	}
	file, err := parser.ParseFile(fset, filename, src, parser.ParseComments)
	if err != nil {
		return nil, nil, err
	}

	var edits []edit
	var longGroups []groupLongLines
	for _, cg := range collectGroups(file) {
		e, long := planEdit(fset, src, cg, limit)
		if e != nil {
			edits = append(edits, *e)
		}
		if len(long.indexes) > 0 {
			longGroups = append(longGroups, long)
		}
	}

	var longLines []int
	for _, group := range longGroups {
		lineShift := 0
		for _, e := range edits {
			if e.line < group.line {
				lineShift += e.lineShift()
			}
		}
		for _, index := range group.indexes {
			longLines = append(longLines, group.line+lineShift+index)
		}
	}
	slices.Sort(longLines)
	return edits, longLines, nil
}

// collectGroups returns every comment group eligible for reflow consideration.
// The file-level header comment qualifies when a blank line separates it from the package clause.
// The package doc comment qualifies too.
// The doc comment before each exported top-level declaration qualifies.
// So does each per-spec doc comment in a grouped var, const or type block.
func collectGroups(file *ast.File) []*ast.CommentGroup {
	var groups []*ast.CommentGroup

	if len(file.Comments) > 0 {
		first := file.Comments[0]
		if first.End() < file.Package && first != file.Doc {
			groups = append(groups, first)
		}
	}
	if file.Doc != nil {
		groups = append(groups, file.Doc)
	}

	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *ast.GenDecl:
			if d.Tok == token.IMPORT {
				continue
			}
			groupExported := false
			for _, spec := range d.Specs {
				if specExported(spec) {
					groupExported = true
					break
				}
			}
			if d.Doc != nil && groupExported {
				groups = append(groups, d.Doc)
			}
			for _, spec := range d.Specs {
				switch s := spec.(type) {
				case *ast.TypeSpec:
					if s.Doc != nil && s.Name.IsExported() {
						groups = append(groups, s.Doc)
					}
				case *ast.ValueSpec:
					if s.Doc == nil {
						continue
					}
					for _, n := range s.Names {
						if n.IsExported() {
							groups = append(groups, s.Doc)
							break
						}
					}
				}
			}
		case *ast.FuncDecl:
			if d.Doc != nil && d.Name.IsExported() {
				groups = append(groups, d.Doc)
			}
		}
	}
	return groups
}

func specExported(spec ast.Spec) bool {
	switch s := spec.(type) {
	case *ast.TypeSpec:
		return s.Name.IsExported()
	case *ast.ValueSpec:
		for _, n := range s.Names {
			if n.IsExported() {
				return true
			}
		}
	}
	return false
}

// planEdit builds the replacement edit for one comment group, or returns a nil edit when the group stays as written.
// A group holding a /* */ comment or a directive stays as written.
// So does a one-line group whose text is under limit.
// Every other group has its prose paragraphs laid out anew.
// The second result names the prose lines of the group's final text that reach limit;
// it is empty for a group the tool does not consider.
func planEdit(fset *token.FileSet, src string, cg *ast.CommentGroup, limit int) (*edit, groupLongLines) {
	// Reuse the indentation bytes that precede the comment, which may be tabs.
	// Spaces rebuilt from the column number would misindent every continuation line of a tab-indented block.
	firstPos := fset.Position(cg.List[0].Pos())
	lineStartOffset := fset.Position(fset.File(cg.List[0].Pos()).LineStart(firstPos.Line)).Offset
	indent := src[lineStartOffset:firstPos.Offset]

	raws := make([]string, len(cg.List))
	for i, c := range cg.List {
		raw, isLineComment := strings.CutPrefix(c.Text, "//")
		if !isLineComment || commentlint.IsDirective(raw) {
			return nil, groupLongLines{}
		}
		raws[i] = raw
	}
	if len(raws) == 1 && !commentlint.TextReachesLimit(raws[0], limit) {
		// A one-line comment has no hard wrap to reflow, so it is reflowed only to bring it under the limit.
		return nil, groupLongLines{}
	}

	line := fset.Position(cg.Pos()).Line
	newRaws := reflowGroupLines(raws, limit)
	var longIndexes []int
	for i, raw := range newRaws {
		if commentlint.IsProseLine(raw) && commentlint.TextReachesLimit(raw, limit) {
			longIndexes = append(longIndexes, i)
		}
	}
	long := groupLongLines{line: line, indexes: longIndexes}
	if slices.Equal(raws, newRaws) {
		return nil, long
	}
	return &edit{
		start: fset.Position(cg.Pos()).Offset,
		end:   fset.Position(cg.End()).Offset,
		line:  line,
		old:   joinCommentLines(raws, indent),
		text:  joinCommentLines(newRaws, indent),
	}, long
}

// reflowGroupLines reflows the lines of one comment group, each given as its text after "//".
// Blank lines split the group into paragraphs and stay as written.
// A paragraph holding indented code, a heading or a list item stays as written too;
// every other paragraph is laid out anew.
func reflowGroupLines(raws []string, limit int) []string {
	var result, paragraph []string
	flushParagraph := func() {
		if len(paragraph) == 0 {
			return
		}
		result = append(result, reflowParagraph(paragraph, limit)...)
		paragraph = nil
	}
	for _, raw := range raws {
		if strings.TrimSpace(raw) == "" {
			flushParagraph()
			result = append(result, raw)
			continue
		}
		paragraph = append(paragraph, raw)
	}
	flushParagraph()
	return result
}

// reflowParagraph lays one paragraph out as semantic lines, each given as its text after "//".
// It returns the paragraph as written when any of its lines is not prose.
func reflowParagraph(raws []string, limit int) []string {
	for _, raw := range raws {
		if !commentlint.IsProseLine(raw) {
			return raws
		}
	}
	lines := layoutParagraph(raws, limit)
	if lines == nil {
		return raws
	}
	for i, line := range lines {
		lines[i] = " " + line
	}
	return lines
}

// joinCommentLines renders comment lines, each given as its text after "//", as source text.
// Every line after the first starts with indent.
func joinCommentLines(raws []string, indent string) string {
	var b strings.Builder
	for i, raw := range raws {
		if i > 0 {
			b.WriteString("\n")
			b.WriteString(indent)
		}
		b.WriteString("//")
		b.WriteString(raw)
	}
	return b.String()
}

// applyEdits splices non-overlapping edits into src, applied last-to-first so earlier offsets stay valid.
func applyEdits(src string, edits []edit) string {
	sort.Slice(edits, func(i, j int) bool { return edits[i].start > edits[j].start })
	for _, e := range edits {
		src = src[:e.start] + e.text + src[e.end:]
	}
	return src
}
