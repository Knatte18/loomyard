// deleteorder.go implements LaterDeleteReferences, the delete-before-reference check: a card that deletes a symbol while a later card's Edit code still references it.

package planglyph

import (
	"errors"
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/quarry/glyph"
	"github.com/Knatte18/quarry/quarry"
)

// refShape is the kind of target a card ref names, as far as the delete-order check cares.
type refShape int

const (
	// shapeOther is a handle or an unparseable ref: never checked.
	shapeOther refShape = iota
	// shapeMember is a member glyph, such as sub#Foo or sub#Type.Method.
	shapeMember
	// shapeFile is a Go file, spelled as a plain path or a file self glyph.
	shapeFile
	// shapePackage is a package self glyph.
	shapePackage
)

// classifyDeleteOrderRef classifies ref and returns its parsed glyph for the member and package shapes, and its repository-relative path for the file and package shapes.
func classifyDeleteOrderRef(lang glyph.Language, ref string) (refShape, glyph.Glyph, string) {
	if planparser.IsHandleRef(ref) {
		return shapeOther, glyph.Glyph{}, ""
	}
	g, err := glyph.Parse(lang, ref)
	if err != nil {
		if !strings.Contains(ref, "#") && path.Ext(ref) == ".go" {
			return shapeFile, glyph.Glyph{}, ref
		}
		return shapeOther, glyph.Glyph{}, ""
	}
	if !g.IsSelf() {
		return shapeMember, g, ""
	}
	unitPath, ok := g.UnitPath()
	if !ok {
		return shapeOther, glyph.Glyph{}, ""
	}
	if path.Ext(unitPath) == ".go" {
		return shapeFile, g, unitPath
	}
	return shapePackage, g, unitPath
}

// deletedSymbol is one Delete target that a later card may still reference.
type deletedSymbol struct {
	card   planparser.Card
	target string
	// samePackage matches a reference to the symbol in a file of the package at dir.
	samePackage *regexp.Regexp
	// otherPackage matches a reference to the symbol in a file of any other package.
	otherPackage *regexp.Regexp
	// dir is the repository-relative directory of the deleted symbol's package.
	dir string
}

// editRegion is the code one later card's Edit target covers: lines start..end of file, or the whole file when end is zero.
type editRegion struct {
	card  planparser.Card
	file  string
	start int
	end   int
}

// LaterDeleteReferences reports, as blocking delete-before-reference findings, every place a card in later numbered above a card in deleting still references a symbol that card deletes.
// The deleting card's Delete targets that are member glyphs or package self glyphs are looked up; a file-path Delete target, and a member target that no longer resolves found, are not checked.
// The searched code is each later card's Edit targets: a member glyph's resolved span, or the whole file for a file path or file self glyph; any other Edit target shape is not searched.
// A reference is the deleted identifier as a whole word, bare or as a selector inside the deleted symbol's own package and package-qualified outside it; a method or field is reached as a selector, and a package as an import of its path.
// The match is textual, so a same-named unrelated symbol inside a later card's span is also flagged.
// That is acceptable because the fix every finding names, moving the delete to a card after the later one, is always available; code outside a later card's Edit targets is never searched.
// Under a non-glyph language it returns no findings and opens no repository.
// An infrastructure error is wrapped in ErrQuarryUnavailable.
func LaterDeleteReferences(plan *planparser.Plan, deleting, later []planparser.Card, worktreeRoot string) ([]Finding, error) {
	lang, ok := plan.GlyphLanguage()
	if !ok {
		return nil, nil
	}

	type pendingDelete struct {
		card  planparser.Card
		ref   string
		shape refShape
		g     glyph.Glyph
		path  string
	}
	type pendingEdit struct {
		card  planparser.Card
		ref   string
		shape refShape
		path  string
	}

	var deletes []pendingDelete
	for _, c := range sortedCards(deleting) {
		for _, group := range c.TargetGroups {
			if group.Type != planparser.CardTypeDelete {
				continue
			}
			for _, ref := range group.Refs {
				shape, g, refPath := classifyDeleteOrderRef(lang, ref)
				if shape == shapeMember || shape == shapePackage {
					deletes = append(deletes, pendingDelete{card: c, ref: ref, shape: shape, g: g, path: refPath})
				}
			}
		}
	}
	var edits []pendingEdit
	for _, c := range sortedCards(later) {
		for _, group := range c.TargetGroups {
			if group.Type != planparser.CardTypeEdit {
				continue
			}
			for _, ref := range group.Refs {
				shape, _, refPath := classifyDeleteOrderRef(lang, ref)
				if shape == shapeMember || shape == shapeFile {
					edits = append(edits, pendingEdit{card: c, ref: ref, shape: shape, path: refPath})
				}
			}
		}
	}
	if len(deletes) == 0 || len(edits) == 0 {
		return nil, nil
	}

	seenTarget := make(map[string]bool)
	var targets []string
	addTarget := func(ref string) {
		if !seenTarget[ref] {
			seenTarget[ref] = true
			targets = append(targets, ref)
		}
	}
	for _, d := range deletes {
		if d.shape == shapeMember {
			addTarget(d.ref)
		}
	}
	for _, e := range edits {
		if e.shape == shapeMember {
			addTarget(e.ref)
		}
	}
	index := make(map[string]quarry.ResolveResult)
	if len(targets) > 0 {
		sort.Strings(targets)
		repo, err := openRepo(worktreeRoot)
		if err != nil {
			return nil, err
		}
		results, err := resolveTargets(repo, targets)
		if err != nil {
			return nil, err
		}
		index = resultByTarget(results)
	}

	var findings []Finding
	var symbols []deletedSymbol
	for _, d := range deletes {
		if d.shape == shapePackage {
			quoted := regexp.MustCompile(`"(?:[^"]*/)?` + regexp.QuoteMeta(d.path) + `"`)
			symbols = append(symbols, deletedSymbol{card: d.card, target: d.ref, samePackage: quoted, otherPackage: quoted, dir: d.path})
			continue
		}
		answered, readable := answerSymbols(index[d.ref])
		if !readable {
			findings = append(findings, unreadableAnswerFinding(d.card, "Delete target", d.ref, index[d.ref]))
			continue
		}
		if len(answered) == 0 {
			continue
		}
		declaringFile := answered[0].File
		name := regexp.QuoteMeta(d.g.Name)
		if len(d.g.Owner) > 0 {
			selector := regexp.MustCompile(`\.` + name + `\b`)
			symbols = append(symbols, deletedSymbol{card: d.card, target: d.ref, samePackage: selector, otherPackage: selector, dir: path.Dir(declaringFile)})
			continue
		}
		packageName, err := packageNameOfFile(filepath.Join(worktreeRoot, filepath.FromSlash(declaringFile)))
		if err != nil {
			return nil, fmt.Errorf("%w: read package clause of %q: %v", ErrQuarryUnavailable, declaringFile, err)
		}
		symbols = append(symbols, deletedSymbol{
			card:         d.card,
			target:       d.ref,
			samePackage:  regexp.MustCompile(`\b` + name + `\b`),
			otherPackage: regexp.MustCompile(`\b` + regexp.QuoteMeta(packageName) + `\.` + name + `\b`),
			dir:          path.Dir(declaringFile),
		})
	}

	var regions []editRegion
	for _, e := range edits {
		if e.shape == shapeFile {
			regions = append(regions, editRegion{card: e.card, file: e.path})
			continue
		}
		answered, readable := answerSymbols(index[e.ref])
		if !readable {
			findings = append(findings, unreadableAnswerFinding(e.card, "Edit target", e.ref, index[e.ref]))
			continue
		}
		for _, s := range answered {
			regions = append(regions, editRegion{card: e.card, file: s.File, start: s.Start, end: s.End})
		}
	}

	linesOf := make(map[string][]string)
	readLines := func(file string) ([]string, error) {
		if lines, ok := linesOf[file]; ok {
			return lines, nil
		}
		data, err := os.ReadFile(filepath.Join(worktreeRoot, filepath.FromSlash(file)))
		if errors.Is(err, os.ErrNotExist) {
			linesOf[file] = nil
			return nil, nil
		}
		if err != nil {
			return nil, fmt.Errorf("%w: read %q: %v", ErrQuarryUnavailable, file, err)
		}
		lines := strings.Split(string(data), "\n")
		linesOf[file] = lines
		return lines, nil
	}

	reported := make(map[string]bool)
	for _, sym := range symbols {
		for _, region := range regions {
			if region.card.Number <= sym.card.Number {
				continue
			}
			lines, err := readLines(region.file)
			if err != nil {
				return findings, err
			}
			first, last := 1, len(lines)
			if region.end != 0 {
				first, last = region.start, min(region.end, len(lines))
			}
			pattern := sym.otherPackage
			if path.Dir(region.file) == sym.dir {
				pattern = sym.samePackage
			}
			for n := first; n <= last; n++ {
				if !pattern.MatchString(lines[n-1]) {
					continue
				}
				key := fmt.Sprintf("%s|%s|%s:%d", sym.card.ID(), sym.target, region.file, n)
				if reported[key] {
					continue
				}
				reported[key] = true
				findings = append(findings, Finding{
					Check: "delete-before-reference",
					Card:  sym.card.ID(),
					Detail: fmt.Sprintf(
						"card %d deletes %q, but card %s still references it at %s:%d in its Edit code — move the delete to a card after card %d",
						sym.card.Number, sym.target, region.card.ID(), region.file, n, region.card.Number,
					),
					Severity: SeverityBlocking,
				})
			}
		}
	}
	return findings, nil
}

// answerSymbols returns the declarations a resolve answer names: the symbols of a found or multipart answer, none for a not_found or ambiguous one.
// readable is false for an answer outside quarry's four-value vocabulary, a pre-resolution rejection included, so the caller fails closed.
func answerSymbols(r quarry.ResolveResult) (symbols []quarry.Symbol, readable bool) {
	switch r.Status {
	case quarry.StatusFound, quarry.StatusMultipart:
		return r.Symbols, true
	case quarry.StatusNotFound, quarry.StatusAmbiguous:
		return nil, true
	default:
		return nil, false
	}
}

// unreadableAnswerFinding is the blocking glyph-rejected finding for a target of card whose answer answerSymbols could not read; noun names the target's group.
func unreadableAnswerFinding(card planparser.Card, noun, target string, r quarry.ResolveResult) Finding {
	return Finding{
		Check:    "glyph-rejected",
		Card:     card.ID(),
		Detail:   unreadableStatusDetail(noun, target, r),
		Severity: SeverityBlocking,
	}
}

// packageNameOfFile returns the package name declared by the Go source file at filePath.
func packageNameOfFile(filePath string) (string, error) {
	file, err := parser.ParseFile(token.NewFileSet(), filePath, nil, parser.PackageClauseOnly)
	if err != nil {
		return "", err
	}
	return file.Name.Name, nil
}
