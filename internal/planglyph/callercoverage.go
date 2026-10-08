// callercoverage.go implements caller-uncovered: a card that deletes or re-signs a member while some Go code still references it and no admissible card's target covers that code.
// The reference walk tokenizes every Go file under the worktree root, nested modules included, and resolves a package-level reference by import path, so it reads the tree and belongs to the plan gates only.
// A method reference is matched by name alone and reported informationally, since the receiver is not resolved.

package planglyph

import (
	"errors"
	"fmt"
	"go/parser"
	"go/scanner"
	"go/token"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/quarry/glyph"
	"github.com/Knatte18/quarry/quarry"
	"golang.org/x/mod/modfile"
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
	// dir, packageClause and importPath locate the declaring package of a package-level subject; importPath is empty when no go.mod covers dir.
	dir           string
	packageClause string
	importPath    string
}

// referenceSite is one identifier of a Go file that is named like a subject, with the context that decides whether it references the subject.
type referenceSite struct {
	// dir and packageClause are the file's directory, worktree-relative, and package clause.
	dir           string
	packageClause string
	// afterPeriod reports a "." directly before the identifier, and qualifier the identifier left of that "." or "" when there is none.
	afterPeriod bool
	qualifier   string
	// imports maps the name each import is bound to onto its path; dotImports holds the paths imported with ".".
	imports    map[string]string
	dotImports map[string]bool
}

// majorVersionSuffix matches the "/vN" major-version element a module path ends in.
var majorVersionSuffix = regexp.MustCompile(`/v[0-9]+$`)

// importPathOf returns the import path of the package at dir: the module path of the go.mod of ModuleOf(modules, dir), joined with dir relative to that module.
// It returns "" when that go.mod does not exist, and an error only when one exists but cannot be read.
func importPathOf(worktreeRoot string, modules []string, dir string) (string, error) {
	module := planparser.ModuleOf(modules, dir)
	source, err := os.ReadFile(filepath.Join(worktreeRoot, filepath.FromSlash(module), "go.mod"))
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	modulePath := modfile.ModulePath(source)
	if modulePath == "" {
		return "", nil
	}
	if module == "." {
		return path.Join(modulePath, dir), nil
	}
	return path.Join(modulePath, strings.TrimPrefix(dir, module)), nil
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

		unresolved := make(map[string][]int)
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
			if subject.isMethod {
				unresolved[file] = lines
				continue
			}
			findings = append(findings, uncoveredFinding(subject, file, lines))
		}
		if len(unresolved) > 0 {
			findings = append(findings, unresolvedMethodFinding(subject, unresolved))
		}
	}
	return findings, nil
}

// coverageSubjects returns, in card and body order, every member glyph a card deletes or re-signs whose answer holds its declaration.
// A member listed more than once on one card is one subject.
func coverageSubjects(plan *planparser.Plan, lang glyph.Language, worktreeRoot string, answers map[string]quarry.ResolveResult) ([]coverageSubject, error) {
	// modules is read on the first package-level subject, since the walk costs a tree read.
	var modules []string
	modulesRead := false
	type cardRef struct {
		card int
		ref  string
	}
	var subjects []coverageSubject
	seen := make(map[cardRef]bool)
	add := func(c planparser.Card, ref string, deleted bool) error {
		if seen[cardRef{c.Number, ref}] || planparser.IsHandleRef(ref) {
			return nil
		}
		seen[cardRef{c.Number, ref}] = true
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
			if !modulesRead {
				modules, modulesRead = planparser.NestedModules(plan, 0, worktreeRoot), true
			}
			subject.importPath, err = importPathOf(worktreeRoot, modules, subject.dir)
			if err != nil {
				return fmt.Errorf("%w: read module path for %q: %v", ErrQuarryUnavailable, subject.dir, err)
			}
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
// A package-level subject is referenced by its bare identifier inside its own package, and elsewhere by an import of its import path under that import's name, a dot and the identifier, or by the bare identifier through a dot import.
// A method subject is referenced by a dot followed by its identifier, anywhere, unless the identifier left of the dot is a name the file imports.
// That exclusion also drops a method call on a local variable that shadows an imported package's name.
// An occurrence inside one of the subject's own resolved spans is its declaration, not a reference.
func scanReferences(root string, subjects []coverageSubject) ([]map[string][]int, error) {
	references := make([]map[string][]int, len(subjects))
	for i := range references {
		references[i] = make(map[string][]int)
	}
	subjectClauses := make(map[string]string)
	for _, subject := range subjects {
		if subject.importPath != "" {
			subjectClauses[subject.importPath] = subject.packageClause
		}
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
		scanFileReferences(filepath.ToSlash(relative), source, subjects, subjectClauses, references)
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
// subjectClauses maps the import path of each subject's package onto its package clause, the name an unaliased import of it binds.
func scanFileReferences(relative string, source []byte, subjects []coverageSubject, subjectClauses map[string]string, references []map[string][]int) {
	imports, dotImports := importsOf(relative, source, subjectClauses)
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
			site := referenceSite{
				dir:           path.Dir(relative),
				packageClause: packageClause,
				afterPeriod:   previous.tok == token.PERIOD,
				imports:       imports,
				dotImports:    dotImports,
			}
			if site.afterPeriod && beforePrevious.tok == token.IDENT {
				site.qualifier = beforePrevious.lit
			}
			for i, subject := range subjects {
				if lit != subject.name || !referencesSubject(subject, site) {
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

// importsOf returns the names the imports of the Go source file at relative are bound to, mapped onto their paths, and the set of paths imported with ".".
// An import's name is its alias, else the package clause of the subject it is the import path of, else the last element of its path without a major-version suffix.
// A file that does not parse past its imports yields what was read.
func importsOf(relative string, source []byte, subjectClauses map[string]string) (map[string]string, map[string]bool) {
	imports := make(map[string]string)
	dotImports := make(map[string]bool)
	file, _ := parser.ParseFile(token.NewFileSet(), relative, source, parser.ImportsOnly)
	if file == nil {
		return imports, dotImports
	}
	for _, spec := range file.Imports {
		importPath, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			continue
		}
		name := ""
		if spec.Name != nil {
			name = spec.Name.Name
		} else if clause, ok := subjectClauses[importPath]; ok {
			name = clause
		} else {
			name = path.Base(majorVersionSuffix.ReplaceAllString(importPath, ""))
		}
		switch name {
		case "_":
		case ".":
			dotImports[importPath] = true
		default:
			imports[name] = importPath
		}
	}
	return imports, dotImports
}

// referencesSubject reports whether the identifier at site, named like subject, is a reference to it.
// A method is referenced by any identifier after a dot whose left side is not an imported package name.
// A package-level member is referenced by its bare identifier inside its declaring package, and elsewhere through an import of its import path; with no import path known it is referenced as the declaring package clause, a dot and the identifier.
func referencesSubject(subject coverageSubject, site referenceSite) bool {
	if subject.isMethod {
		_, imported := site.imports[site.qualifier]
		return site.afterPeriod && !imported
	}
	if site.dir == subject.dir && site.packageClause == subject.packageClause {
		return !site.afterPeriod
	}
	if subject.importPath == "" {
		return site.afterPeriod && site.qualifier == subject.packageClause
	}
	if site.afterPeriod {
		return site.qualifier != "" && site.imports[site.qualifier] == subject.importPath
	}
	return site.dotImports[subject.importPath]
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
// It is always blocking, since every reference it reports is resolved to the subject.
func uncoveredFinding(subject coverageSubject, file string, lines []int) Finding {
	verb, way := coverageVerbAndWay(subject)
	return Finding{
		Check: "caller-uncovered",
		Card:  subject.card.ID(),
		Detail: fmt.Sprintf(
			"card %d %s %q, but %s references it at line %s and no admissible card's target covers that code; %s",
			subject.card.Number, verb, subject.ref, file, lineList(lines), way,
		),
		Severity: SeverityBlocking,
		Ref:      subject.ref,
	}
}

// unresolvedMethodFinding is the informational caller-uncovered finding for a method subject, one per subject: references maps each file onto the lines where its name follows a dot.
// The receiver of those matches is not resolved, so none is known to call the method.
func unresolvedMethodFinding(subject coverageSubject, references map[string][]int) Finding {
	verb, way := coverageVerbAndWay(subject)
	files := make([]string, 0, len(references))
	for file := range references {
		files = append(files, file)
	}
	sort.Strings(files)
	sites := make([]string, len(files))
	for i, file := range files {
		sites[i] = fmt.Sprintf("%s at line %s", file, lineList(references[file]))
	}
	return Finding{
		Check: "caller-uncovered",
		Card:  subject.card.ID(),
		Detail: fmt.Sprintf(
			"card %d %s %q, but its name follows a dot in %s and the receiver could not be resolved, and no admissible card's target covers that code; %s",
			subject.card.Number, verb, subject.ref, strings.Join(sites, "; "), way,
		),
		Severity: SeverityInformational,
		Ref:      subject.ref,
	}
}

// coverageVerbAndWay returns what subject's card does to the member and how a finding about it is cleared.
func coverageVerbAndWay(subject coverageSubject) (verb, way string) {
	if subject.deleted {
		return "deletes", "list the file, or the member glyph whose body holds the reference, on that card or an earlier one"
	}
	return "re-signs", "list the file, or the member glyph whose body holds the reference, on that card"
}

// lineList renders lines as a comma-separated list.
func lineList(lines []int) string {
	numbers := make([]string, len(lines))
	for i, line := range lines {
		numbers[i] = strconv.Itoa(line)
	}
	return strings.Join(numbers, ", ")
}
