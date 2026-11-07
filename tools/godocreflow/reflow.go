// reflow.go finds the doc-comment groups that reflow may change and splices their reflowed text into the source.
// It edits by byte offset and never goes through go/printer, so nothing outside the touched comment lines changes.

package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

var (
	generatedRE = regexp.MustCompile(`^// Code generated .* DO NOT EDIT\.?\s*$`)
	directiveRE = regexp.MustCompile(`^(go:|nolint\b)`)
	headingRE   = regexp.MustCompile(`^#(\s|$)`)
	listItemRE  = regexp.MustCompile(`^([-*]\s|[0-9]{1,3}[.)]\s)`)
)

// isGenerated reports whether src carries a "Code generated ... DO NOT EDIT" marker anywhere in the file.
// The marker is the convention documented at https://go.dev/s/generatedcode.
// Such files are left alone entirely.
func isGenerated(src string) bool {
	for _, line := range strings.Split(src, "\n") {
		if generatedRE.MatchString(strings.TrimRight(line, "\r")) {
			return true
		}
	}
	return false
}

// edit is a byte-offset replacement span into the original source.
type edit struct {
	start, end int
	line       int    // 1-based source line of the comment group, for -check display
	old        string // the text being replaced, for -check display
	text       string
	wrapped    int // number of semantic lines that needed the last-resort width fallback
}

// collectEdits parses src and returns every reflow edit it qualifies for.
// maxWidth is the last-resort word-wrap column budget, counting indentation, "// ", and content; 0 disables it.
// err is non-nil only on a parse failure; a generated file yields no edits and no error.
func collectEdits(fset *token.FileSet, filename, src string, maxWidth int) ([]edit, error) {
	if isGenerated(src) {
		return nil, nil
	}
	file, err := parser.ParseFile(fset, filename, src, parser.ParseComments)
	if err != nil {
		return nil, err
	}

	var edits []edit
	for _, cg := range collectGroups(file) {
		if e := planEdit(fset, src, cg, maxWidth); e != nil {
			edits = append(edits, *e)
		}
	}
	return edits, nil
}

// reflowSource returns src with its reflow edits applied, or src unchanged when nothing qualifies.
// err is non-nil only on a parse failure.
func reflowSource(fset *token.FileSet, filename, src string, maxWidth int) (string, error) {
	edits, err := collectEdits(fset, filename, src, maxWidth)
	if err != nil {
		return src, err
	}
	if len(edits) == 0 {
		return src, nil
	}
	return applyEdits(src, edits), nil
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

// planEdit builds the replacement edit for one comment group, or returns nil when the group stays as written.
// A group stays as written when it is a directive, an indented code example, a heading or list, or already reflowed.
// A one-line comment is reflowed only when it is wider than maxWidth.
// maxWidth (0 disables it) is the last-resort word-wrap fallback.
// It applies to a semantic line still too wide after sentence and clause splitting.
func planEdit(fset *token.FileSet, src string, cg *ast.CommentGroup, maxWidth int) *edit {
	// Reuse the indentation bytes that precede the comment, which may be tabs.
	// Spaces rebuilt from the column number would misindent every continuation line of a tab-indented block.
	firstPos := fset.Position(cg.List[0].Pos())
	lineStartOffset := fset.Position(fset.File(cg.List[0].Pos()).LineStart(firstPos.Line)).Offset
	indent := src[lineStartOffset:firstPos.Offset]
	prefixLen := utf8.RuneCountInString(indent) + len("// ")

	type line struct {
		raw     string // comment text with the leading "//" stripped
		content string // raw with the mandatory single gofmt space stripped too
	}
	lines := make([]line, len(cg.List))
	for i, c := range cg.List {
		if !strings.HasPrefix(c.Text, "//") {
			return nil // /* */ block comment -- leave alone, not the doc-comment convention here
		}
		raw := strings.TrimPrefix(c.Text, "//")
		if directiveRE.MatchString(strings.TrimSpace(raw)) {
			return nil // build/tool directive comment mixed into the group -- leave the whole block alone
		}
		content := raw
		indented := strings.HasPrefix(content, "\t")
		if strings.HasPrefix(content, " ") {
			content = content[1:]
			if strings.HasPrefix(content, " ") || strings.HasPrefix(content, "\t") {
				indented = true
			}
		}
		if indented {
			return nil // indented example code inside the doc comment -- must stay verbatim
		}
		if headingRE.MatchString(content) || listItemRE.MatchString(content) {
			return nil // Go doc comment heading/list structure -- not flat prose, leave alone
		}
		lines[i] = line{raw: raw, content: content}
	}

	if len(cg.List) < 2 {
		// A one-line comment has no hard wrap to reflow.
		// It goes through the multi-line pipeline only when it is wider than maxWidth:
		// sentence and clause splitting may bring it under, and the word wrap below catches what remains.
		if maxWidth <= 0 || prefixLen+utf8.RuneCountInString(lines[0].content) <= maxWidth {
			return nil
		}
	}

	// Split into blank-line-delimited paragraphs and reflow each one on its own.
	// The bare "//" lines between them stay, since godoc treats them as paragraph separators.
	var paragraphs [][]string
	var current []string
	for _, l := range lines {
		if l.content == "" {
			if current != nil {
				paragraphs = append(paragraphs, current)
				current = nil
			}
			paragraphs = append(paragraphs, nil) // nil marks a blank separator line
			continue
		}
		current = append(current, l.content)
	}
	if current != nil {
		paragraphs = append(paragraphs, current)
	}

	var newContentLines []string
	wrapped := 0
	for _, p := range paragraphs {
		if p == nil {
			newContentLines = append(newContentLines, "")
			continue
		}
		for _, semantic := range reflowText(strings.Join(p, " ")) {
			pieces := wrapLongLine(semantic, prefixLen, maxWidth)
			if len(pieces) > 1 {
				wrapped++
			}
			newContentLines = append(newContentLines, pieces...)
		}
	}

	origContentLines := make([]string, len(lines))
	for i, l := range lines {
		origContentLines[i] = l.content
	}
	if equalLines(origContentLines, newContentLines) {
		return nil // already correctly reflowed
	}

	build := func(contentLines []string) string {
		var b strings.Builder
		for i, c := range contentLines {
			if i > 0 {
				b.WriteString("\n")
				b.WriteString(indent)
			}
			b.WriteString("//")
			if c != "" {
				b.WriteString(" ")
				b.WriteString(c)
			}
		}
		return b.String()
	}

	return &edit{
		start:   fset.Position(cg.Pos()).Offset,
		end:     fset.Position(cg.End()).Offset,
		line:    fset.Position(cg.Pos()).Line,
		old:     build(origContentLines),
		text:    build(newContentLines),
		wrapped: wrapped,
	}
}

func equalLines(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// applyEdits splices non-overlapping edits into src, applied last-to-first so earlier offsets stay valid.
func applyEdits(src string, edits []edit) string {
	sort.Slice(edits, func(i, j int) bool { return edits[i].start > edits[j].start })
	for _, e := range edits {
		src = src[:e.start] + e.text + src[e.end:]
	}
	return src
}
