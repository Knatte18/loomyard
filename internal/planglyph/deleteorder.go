// deleteorder.go implements LaterDeleteReferences, the delete-before-reference check: a card that deletes a symbol while a later card's Edit code still references it.
// The references come from the computation caller-uncovered shares (memberReferences), so a member is found by identity and never by its bare word.

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
	"slices"
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

// editRegion is the code one card's Edit target covers: lines start..end of file, or the whole file when end is zero.
type editRegion struct {
	card  planparser.Card
	file  string
	start int
	end   int
}

// holds reports whether line of file lies in r's code.
func (r editRegion) holds(file string, line int) bool {
	return r.file == file && (r.end == 0 || (r.start <= line && line <= r.end))
}

// editTarget is one Edit-group ref of a card, a member glyph or a file, with its shape and disk path.
type editTarget struct {
	card  planparser.Card
	ref   string
	shape refShape
	path  string
}

// collectEditTargets returns, in card and body order, every Edit-group target of cards, limited to member glyphs and files.
func collectEditTargets(lang glyph.Language, cards []planparser.Card) []editTarget {
	var edits []editTarget
	for _, c := range cards {
		for _, group := range c.TargetGroups {
			if group.Type != planparser.CardTypeEdit {
				continue
			}
			for _, ref := range group.Refs {
				shape, _, refPath := classifyDeleteOrderRef(lang, ref)
				if shape == shapeMember || shape == shapeFile {
					edits = append(edits, editTarget{card: c, ref: ref, shape: shape, path: refPath})
				}
			}
		}
	}
	return edits
}

// buildEditRegions returns the code each of edits covers: the whole file for a file target, and each resolved span for a member glyph answered in index.
// A member whose answer cannot be read adds a glyph-rejected finding instead of a region.
func buildEditRegions(edits []editTarget, index map[string]quarry.ResolveResult, partitioned map[string]bool) ([]editRegion, []Finding) {
	var regions []editRegion
	var findings []Finding
	for _, e := range edits {
		if e.shape == shapeFile {
			regions = append(regions, editRegion{card: e.card, file: e.path})
			continue
		}
		answered, readable := answerSymbols(index[e.ref], partitioned[e.ref])
		if !readable {
			findings = append(findings, unreadableAnswerFinding(e.card, "Edit target", e.ref, index[e.ref]))
			continue
		}
		for _, s := range answered {
			regions = append(regions, editRegion{card: e.card, file: s.File, start: s.Start, end: s.End})
		}
	}
	return regions, findings
}

// LaterDeleteReferences reports every place a card in later numbered above a card in deleting still references a symbol that card deletes.
// The deleting card's Delete targets that are member glyphs or package self glyphs are looked up; a file-path Delete target, and a member target that no longer resolves found, are not checked.
// The searched code is each later card's Edit targets: a member glyph's resolved span, or the whole file for a file path or file self glyph; any other Edit target shape is not searched.
// A reference that an Edit target of the deleting card, or of a card in deleting or later numbered before it, also covers is not reported, since that card rewrites the code first.
// Only Go files outside the directories the go tool ignores are searched, so a comment, a string literal and a non-Go target never match.
// A reference is found the way a caller of the deleted member is: a package-level member by its bare identifier inside its own package and elsewhere through an import of its path, a method by its name after a dot.
// No type load runs, so a package-level reference is blocking and a method matched by name alone is informational, since its receiver is not resolved.
// A package self glyph Delete is referenced by a quoted import of its path, which is blocking.
// The fix every finding names, moving the delete to a card after the later one, is always available; code outside a later card's Edit targets is never searched.
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
	}
	var deletes []pendingDelete
	for _, c := range sortedCards(deleting) {
		for _, group := range c.TargetGroups {
			if group.Type != planparser.CardTypeDelete {
				continue
			}
			for _, ref := range group.Refs {
				shape, _, _ := classifyDeleteOrderRef(lang, ref)
				if shape == shapeMember || shape == shapePackage {
					deletes = append(deletes, pendingDelete{card: c, ref: ref, shape: shape})
				}
			}
		}
	}
	if len(deletes) == 0 || len(collectEditTargets(lang, later)) == 0 {
		return nil, nil
	}
	editing := slices.Clone(later)
	for _, c := range deleting {
		if !slices.ContainsFunc(editing, func(other planparser.Card) bool { return other.Number == c.Number }) {
			editing = append(editing, c)
		}
	}
	edits := collectEditTargets(lang, sortedCards(editing))

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
	partitioned := partitionedAnswers(worktreeRoot, index)

	var findings []Finding
	for _, d := range deletes {
		if d.shape != shapeMember {
			continue
		}
		if _, readable := answerSymbols(index[d.ref], partitioned[d.ref]); !readable {
			findings = append(findings, unreadableAnswerFinding(d.card, "Delete target", d.ref, index[d.ref]))
		}
	}

	allRegions, regionFindings := buildEditRegions(edits, index, partitioned)
	findings = append(findings, regionFindings...)
	// Cards added from deleting only supply exemption regions; a holder is reported only from a card in later.
	regions := slices.DeleteFunc(slices.Clone(allRegions), func(r editRegion) bool {
		return !slices.ContainsFunc(later, func(c planparser.Card) bool { return c.Number == r.card.Number })
	})

	deletingView := *plan
	deletingView.Cards = deleting
	allSubjects, err := coverageSubjects(&deletingView, lang, worktreeRoot, index, partitioned)
	if err != nil {
		return findings, err
	}
	subjects := slices.DeleteFunc(allSubjects, func(subject coverageSubject) bool { return !subject.deleted })
	if len(subjects) > 0 {
		resolved, nameOnly, err := memberReferences(plan, worktreeRoot, subjects, regionFiles(worktreeRoot, regions), nil)
		if err != nil {
			return findings, err
		}
		findings = append(findings, deleteOrderFindings(subjects, resolved, nameOnly, regions, allRegions)...)
	}

	importFindings, err := laterImportFindings(lang, deleting, regions, worktreeRoot)
	return append(findings, importFindings...), err
}

// deleteOrderFindings returns, for each deleted subject, the delete-before-reference findings for its references that lie inside a later card's Edit region.
// A line that an Edit region of the deleting card or an earlier card also holds is skipped, since that card rewrites it before any later card runs.
// A resolved line gets one blocking finding, naming the first later card whose region holds it.
// A method's name-only lines get one informational finding per subject, naming every file with its lines and each later card whose region holds one of them.
// resolved and nameOnly are memberReferences' answers, one map per subject; a re-signed subject is skipped, since its references are caller-uncovered's.
// regions are the later cards' Edit regions, the only ones reported; allRegions adds the deleting cards' regions and serves only the rewritten-line exemption.
func deleteOrderFindings(subjects []coverageSubject, resolved, nameOnly []map[string][]int, regions, allRegions []editRegion) []Finding {
	var findings []Finding
	for i, subject := range subjects {
		if !subject.deleted {
			continue
		}
		for _, file := range sortedFiles(resolved[i]) {
			for _, line := range resolved[i][file] {
				holders := unrewrittenLaterEditRegions(regions, allRegions, subject.card.Number, file, line)
				if len(holders) == 0 {
					continue
				}
				findings = append(findings, Finding{
					Check: "delete-before-reference",
					Card:  subject.card.ID(),
					Detail: fmt.Sprintf(
						"card %d deletes %q, but card %s still references it at %s:%d in its Edit code — move the delete to a card after card %d",
						subject.card.Number, subject.ref, holders[0].card.ID(), file, line, holders[0].card.Number,
					),
					Severity: SeverityBlocking,
				})
			}
		}

		var sites, holderIDs []string
		lastHolder := 0
		for _, file := range sortedFiles(nameOnly[i]) {
			var inside []int
			for _, line := range nameOnly[i][file] {
				holders := unrewrittenLaterEditRegions(regions, allRegions, subject.card.Number, file, line)
				if len(holders) == 0 {
					continue
				}
				inside = append(inside, line)
				for _, holder := range holders {
					if !slices.Contains(holderIDs, holder.card.ID()) {
						holderIDs = append(holderIDs, holder.card.ID())
					}
					lastHolder = max(lastHolder, holder.card.Number)
				}
			}
			if len(inside) > 0 {
				sites = append(sites, fmt.Sprintf("%s at line %s", file, lineList(inside)))
			}
		}
		if len(sites) > 0 {
			findings = append(findings, Finding{
				Check: "delete-before-reference",
				Card:  subject.card.ID(),
				Detail: fmt.Sprintf(
					"card %d deletes %q, but its name follows a dot in %s, in the Edit code of card %s, and the receiver could not be resolved — if one is a call, move the delete to a card after card %d",
					subject.card.Number, subject.ref, strings.Join(sites, "; "), strings.Join(holderIDs, ", "), lastHolder,
				),
				Severity: SeverityInformational,
			})
		}
	}
	return findings
}

// unrewrittenLaterEditRegions returns, in order, the regions of cards numbered above cardNumber whose code holds line of file, or none when a region of allRegions of a card numbered at or below cardNumber also holds it.
func unrewrittenLaterEditRegions(regions, allRegions []editRegion, cardNumber int, file string, line int) []editRegion {
	rewritten := slices.ContainsFunc(allRegions, func(r editRegion) bool {
		return r.card.Number <= cardNumber && r.holds(file, line)
	})
	if rewritten {
		return nil
	}
	return laterEditRegions(regions, cardNumber, file, line)
}

// laterImportFindings returns a blocking delete-before-reference finding for each quoted import of a package that a card in deleting deletes, found in the code of a later card's region.
// A package is deleted by a package self glyph in a Delete group.
// Only regions in Go files outside the directories the go tool ignores are searched, and a region file absent from disk is skipped.
func laterImportFindings(lang glyph.Language, deleting []planparser.Card, regions []editRegion, worktreeRoot string) ([]Finding, error) {
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

	var findings []Finding
	reported := make(map[string]bool)
	for _, c := range sortedCards(deleting) {
		for _, group := range c.TargetGroups {
			if group.Type != planparser.CardTypeDelete {
				continue
			}
			for _, ref := range group.Refs {
				shape, _, unitPath := classifyDeleteOrderRef(lang, ref)
				if shape != shapePackage {
					continue
				}
				quoted := regexp.MustCompile(`"(?:[^"]*/)?` + regexp.QuoteMeta(unitPath) + `"`)
				for _, region := range regions {
					if region.card.Number <= c.Number || !isGoToolSource(region.file) {
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
					for n := first; n <= last; n++ {
						if !quoted.MatchString(lines[n-1]) {
							continue
						}
						key := fmt.Sprintf("%s|%s|%s:%d", c.ID(), ref, region.file, n)
						if reported[key] {
							continue
						}
						reported[key] = true
						findings = append(findings, Finding{
							Check: "delete-before-reference",
							Card:  c.ID(),
							Detail: fmt.Sprintf(
								"card %d deletes %q, but card %s still references it at %s:%d in its Edit code — move the delete to a card after card %d",
								c.Number, ref, region.card.ID(), region.file, n, region.card.Number,
							),
							Severity: SeverityBlocking,
						})
					}
				}
			}
		}
	}
	return findings, nil
}

// regionFiles returns, sorted, the Go files of regions that the go tool reads and that exist under worktreeRoot.
func regionFiles(worktreeRoot string, regions []editRegion) []string {
	seen := make(map[string]bool)
	var files []string
	for _, region := range regions {
		if seen[region.file] || !isGoToolSource(region.file) {
			continue
		}
		seen[region.file] = true
		if _, err := os.Stat(filepath.Join(worktreeRoot, filepath.FromSlash(region.file))); errors.Is(err, os.ErrNotExist) {
			continue
		}
		files = append(files, region.file)
	}
	sort.Strings(files)
	return files
}

// ignoredByGoTool reports whether the go tool skips a directory named dirName: testdata, vendor, and a name starting with . or _.
func ignoredByGoTool(dirName string) bool {
	return dirName == "testdata" || dirName == "vendor" || strings.HasPrefix(dirName, ".") || strings.HasPrefix(dirName, "_")
}

// isGoToolSource reports whether file, slash-separated and relative to the worktree root, is Go source in a directory the go tool reads.
func isGoToolSource(file string) bool {
	if path.Ext(file) != ".go" {
		return false
	}
	dir := path.Dir(file)
	if dir == "." {
		return true
	}
	return !slices.ContainsFunc(strings.Split(dir, "/"), ignoredByGoTool)
}

// partitionedAnswers classifies each ambiguous answer of answers once, reading its candidate files under worktreeRoot, and returns the targets whose candidates no single build environment selects together.
// A pass that consults one answer many times builds the map once, so no candidate file is read again per consultation.
// It tests the candidates rather than the status: only an ambiguous answer carries two or more.
func partitionedAnswers(worktreeRoot string, answers map[string]quarry.ResolveResult) map[string]bool {
	partitioned := make(map[string]bool)
	for target, answer := range answers {
		if len(answer.Candidates) >= 2 && classifyAmbiguity(worktreeRoot, answer.Candidates).Partitioned {
			partitioned[target] = true
		}
	}
	return partitioned
}

// answerSymbols returns the declarations a resolve answer names: the symbols of a found or multipart answer, the candidates of an ambiguous one that partitioned reports partitioned by build constraints, and none for a not_found or any other ambiguous one.
// readable is false for an answer outside quarry's four-value vocabulary, a pre-resolution rejection included, so the caller fails closed.
func answerSymbols(r quarry.ResolveResult, partitioned bool) (symbols []quarry.Symbol, readable bool) {
	switch r.Status {
	case quarry.StatusFound, quarry.StatusMultipart:
		return r.Symbols, true
	case quarry.StatusAmbiguous:
		if partitioned {
			return r.Candidates, true
		}
		return nil, true
	case quarry.StatusNotFound:
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
