// callercoverage.go implements caller-uncovered: a card that deletes or re-signs a member while some Go code still references it and no admissible card's target covers that code.
// The reference walk is name-based and tokenizes every Go file under the worktree root, so it reads the tree and belongs to the plan gates only.

package planglyph

import (
	"fmt"
	"go/scanner"
	"go/token"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/quarry/glyph"
	"github.com/Knatte18/quarry/quarry"
)

// coverageSubject is one member a card deletes or re-signs, with the declaration its answer holds.
type coverageSubject struct {
	card     planparser.Card
	ref      string
	name     string
	isMethod bool
	// deleted is false for a re-signed member.
	deleted bool
	symbols []quarry.Symbol
	// dir and packageClause locate the declaring package of a package-level subject.
	dir           string
	packageClause string
}

// callerCoverageFindings reports caller-uncovered for every deleted or re-signed member of plan, given answers, the plan gate's one batched resolve answer.
// A deleted member's reference inside a later card's Edit code is delete-before-reference's alone and is not reported here.
// The walk runs only when a subject exists.
func callerCoverageFindings(plan *planparser.Plan, lang glyph.Language, worktreeRoot string, answers map[string]quarry.ResolveResult) ([]Finding, error) {
	subjects, err := coverageSubjects(plan, lang, worktreeRoot, answers)
	if err != nil || len(subjects) == 0 {
		return nil, err
	}

	references, err := scanReferences(worktreeRoot, subjects)
	if err != nil {
		return nil, err
	}

	regions, _ := buildEditRegions(collectEditTargets(lang, plan.Cards), answers)

	var findings []Finding
	for i, subject := range subjects {
		files := make([]string, 0, len(references[i]))
		for file := range references[i] {
			files = append(files, file)
		}
		sort.Strings(files)

		for _, file := range files {
			var lines []int
			for _, line := range references[i][file] {
				if coveredByTarget(plan, lang, subject, file, line, answers) {
					continue
				}
				if subject.deleted && insideLaterEditRegion(regions, subject.card.Number, file, line) {
					continue
				}
				lines = append(lines, line)
			}
			if len(lines) == 0 {
				continue
			}
			findings = append(findings, uncoveredFinding(subject, file, lines))
		}
	}
	return findings, nil
}

// coverageSubjects returns, in card and body order, every member glyph a card deletes or re-signs whose answer holds its declaration.
func coverageSubjects(plan *planparser.Plan, lang glyph.Language, worktreeRoot string, answers map[string]quarry.ResolveResult) ([]coverageSubject, error) {
	var subjects []coverageSubject
	add := func(c planparser.Card, ref string, deleted bool) error {
		if planparser.IsHandleRef(ref) {
			return nil
		}
		g, err := glyph.Parse(lang, ref)
		if err != nil || g.IsSelf() {
			return nil
		}
		symbols, readable := answerSymbols(answers[ref])
		if !readable || len(symbols) == 0 {
			return nil
		}
		subject := coverageSubject{card: c, ref: ref, name: g.Name, isMethod: len(g.Owner) > 0, deleted: deleted, symbols: symbols}
		if !subject.isMethod {
			declaringFile := symbols[0].File
			packageClause, err := packageNameOfFile(filepath.Join(worktreeRoot, filepath.FromSlash(declaringFile)))
			if err != nil {
				return fmt.Errorf("%w: read package clause of %q: %v", ErrQuarryUnavailable, declaringFile, err)
			}
			subject.dir = path.Dir(declaringFile)
			subject.packageClause = packageClause
		}
		subjects = append(subjects, subject)
		return nil
	}

	for _, c := range sortedCards(plan.Cards) {
		for _, group := range c.TargetGroups {
			if group.Type != planparser.CardTypeDelete {
				continue
			}
			for _, ref := range group.Refs {
				if err := add(c, ref, true); err != nil {
					return nil, err
				}
			}
		}
		for _, r := range c.Resigns {
			if err := add(c, r.Target, false); err != nil {
				return nil, err
			}
		}
	}
	return subjects, nil
}

// scanReferences tokenizes every Go file under root and returns, per subject, the lines of each file that reference it.
// It skips directories named testdata or vendor and directories whose name starts with . or _, which the go tool ignores too, and every file that is not Go source.
// Comments and string literals never match, since the scanner drops the first and yields the second as one token.
// A package-level subject is referenced by its bare identifier inside its own package, and elsewhere as the declaring package clause, a dot and the identifier.
// A method subject is referenced by a dot followed by its identifier, anywhere.
// An occurrence inside one of the subject's own resolved spans is its declaration, not a reference.
func scanReferences(root string, subjects []coverageSubject) ([]map[string][]int, error) {
	references := make([]map[string][]int, len(subjects))
	for i := range references {
		references[i] = make(map[string][]int)
	}

	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := d.Name()
		if d.IsDir() {
			if p != root && (name == "testdata" || name == "vendor" || strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_")) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(name, ".go") {
			return nil
		}
		source, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		scanFileReferences(filepath.ToSlash(relative), source, subjects, references)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("%w: walk %q for callers: %v", ErrQuarryUnavailable, root, err)
	}

	for i := range references {
		for file, lines := range references[i] {
			slices.Sort(lines)
			references[i][file] = slices.Compact(lines)
		}
	}
	return references, nil
}

// scanFileReferences adds to references the lines of the Go source file at relative that reference each subject.
func scanFileReferences(relative string, source []byte, subjects []coverageSubject, references []map[string][]int) {
	fileSet := token.NewFileSet()
	file := fileSet.AddFile(relative, fileSet.Base(), len(source))
	var s scanner.Scanner
	s.Init(file, source, nil, 0)

	type scanned struct {
		tok token.Token
		lit string
	}
	var previous, beforePrevious scanned
	packageClause := ""
	for {
		pos, tok, lit := s.Scan()
		if tok == token.EOF {
			return
		}

		if previous.tok == token.PACKAGE && tok == token.IDENT && packageClause == "" {
			packageClause = lit
		} else if tok == token.IDENT {
			for i, subject := range subjects {
				if lit != subject.name || !isReference(subject, relative, packageClause, previous.tok == token.PERIOD, beforePrevious.tok == token.IDENT && beforePrevious.lit == subject.packageClause) {
					continue
				}
				line := fileSet.Position(pos).Line
				if !insideOwnSpan(subject, relative, line) {
					references[i][relative] = append(references[i][relative], line)
				}
			}
		}
		beforePrevious, previous = previous, scanned{tok, lit}
	}
}

// isReference reports whether an identifier named like subject, found in the file at relative whose package clause is packageClause, is a reference to it.
// afterPeriod reports a preceding dot, and afterPackageDot a preceding declaring-package identifier and dot.
func isReference(subject coverageSubject, relative, packageClause string, afterPeriod, afterPackageDot bool) bool {
	if subject.isMethod {
		return afterPeriod
	}
	if path.Dir(relative) == subject.dir && packageClause == subject.packageClause {
		return !afterPeriod
	}
	return afterPeriod && afterPackageDot
}

// insideOwnSpan reports whether line of file lies in one of subject's resolved declaration spans.
func insideOwnSpan(subject coverageSubject, file string, line int) bool {
	return slices.ContainsFunc(subject.symbols, func(s quarry.Symbol) bool {
		return s.File == file && s.Start <= line && line <= s.End
	})
}

// coveredByTarget reports whether a target of an admissible card covers line of file.
// A re-signed member admits only its own card, since no earlier card can call a signature that does not exist yet; a deleted member admits its own card and every earlier one.
// A target covers the line when it is the file's self glyph, the self glyph of the file's directory, or a member glyph whose resolved span holds the line.
func coveredByTarget(plan *planparser.Plan, lang glyph.Language, subject coverageSubject, file string, line int, answers map[string]quarry.ResolveResult) bool {
	for _, c := range plan.Cards {
		admissible := c.Number == subject.card.Number || (subject.deleted && c.Number < subject.card.Number)
		if !admissible {
			continue
		}
		for _, target := range c.Targets {
			if planparser.IsHandleRef(target) {
				continue
			}
			g, err := glyph.Parse(lang, target)
			if err != nil {
				continue
			}
			if g.IsSelf() {
				unitPath, ok := g.UnitPath()
				if ok && (unitPath == file || unitPath == path.Dir(file)) {
					return true
				}
				continue
			}
			symbols, _ := answerSymbols(answers[target])
			if slices.ContainsFunc(symbols, func(s quarry.Symbol) bool { return s.File == file && s.Start <= line && line <= s.End }) {
				return true
			}
		}
	}
	return false
}

// insideLaterEditRegion reports whether line of file lies in the Edit code of a card numbered above cardNumber.
func insideLaterEditRegion(regions []editRegion, cardNumber int, file string, line int) bool {
	return slices.ContainsFunc(regions, func(r editRegion) bool {
		return r.card.Number > cardNumber && r.file == file && (r.end == 0 || (r.start <= line && line <= r.end))
	})
}

// uncoveredFinding is the caller-uncovered finding for subject's references at lines of file.
// It is blocking for a package-level member and informational for a method, since a name-based scan cannot tell receivers apart.
func uncoveredFinding(subject coverageSubject, file string, lines []int) Finding {
	verb := "re-signs"
	way := "list the file, or the member glyph whose body holds the reference, on that card"
	if subject.deleted {
		verb = "deletes"
		way = "list the file, or the member glyph whose body holds the reference, on that card or an earlier one"
	}
	numbers := make([]string, len(lines))
	for i, line := range lines {
		numbers[i] = strconv.Itoa(line)
	}
	severity := SeverityBlocking
	if subject.isMethod {
		severity = SeverityInformational
	}
	return Finding{
		Check: "caller-uncovered",
		Card:  subject.card.ID(),
		Detail: fmt.Sprintf(
			"card %d %s %q, but %s references it at line %s and no admissible card's target covers that code; %s",
			subject.card.Number, verb, subject.ref, file, strings.Join(numbers, ", "), way,
		),
		Severity: severity,
		Ref:      subject.ref,
	}
}
