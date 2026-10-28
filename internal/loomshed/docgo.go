// docgo.go holds the plan gate's doc-go-unlisted check: a card that changes a package's non-test Go code must cover the package's doc.go.

package loomshed

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Knatte18/loomyard/internal/planindex"
	"github.com/Knatte18/loomyard/internal/planparser"
)

// docGoUnlistedCheck names the finding docGoFindings appends for a card that changes a package's non-test Go code without covering its doc.go.
const docGoUnlistedCheck = "doc-go-unlisted"

// docGoWayForward is the way-forward clause every doc-go-unlisted finding carries.
const docGoWayForward = "way forward: list it under Edit when the card changes what it says, under Extends when it only adds to it, under Uses when it stays as is and no later card targets it, leave it to the later card that edits it, else list it under Edit"

// testMemberPrefixes begin the name of a Go test, benchmark, fuzz target or example.
var testMemberPrefixes = []string{"Test", "Benchmark", "Fuzz", "Example"}

// docGoFindings returns one doc-go-unlisted finding per card of cards and package directory whose non-test Go code the card changes, when the directory holds a doc.go the card does not cover.
// A card changes a package through the refs of its Create, Edit, Extends, Delete, Rename and Move groups; a non-Go file and test code are not Go code of the package.
// A card covers the doc.go when any of its groups or its Uses names the file, when a ref is the package self glyph, or when a later card names the file under Edit or Extends.
// Every member ref of cards is resolved in one index call, made only when there is one.
func docGoFindings(plan *planparser.Plan, cards []planparser.Card, worktreeRoot string, index planindex.Index) ([]planindex.Finding, error) {
	var members []string
	for _, card := range cards {
		for _, ref := range docGoRefs(card) {
			if member, ok := planparser.RefMember(plan, ref); ok && !isTestUnitMember(plan, ref) && !slices.Contains(members, member) {
				members = append(members, member)
			}
		}
	}
	answers := map[string]planindex.GlyphAnswer{}
	if len(members) > 0 {
		resolved, err := index.ResolveGlyphs(worktreeRoot, members)
		if err != nil {
			return nil, err
		}
		for i, answer := range resolved {
			answers[members[i]] = answer
		}
	}

	var findings []planindex.Finding
	for _, card := range cards {
		for _, dir := range changedCodeDirs(plan, card, answers) {
			docPath := path.Join(dir, "doc.go")
			if _, err := os.Stat(filepath.Join(worktreeRoot, filepath.FromSlash(docPath))); err != nil {
				continue
			}
			if coversDocGo(plan, card, dir, docPath) {
				continue
			}
			findings = append(findings, planindex.Finding{
				Check:    docGoUnlistedCheck,
				Card:     card.ID(),
				Detail:   fmt.Sprintf("card %d changes non-test Go code in package %s and does not cover %s; %s", card.Number, dir, docPath, docGoWayForward),
				Severity: planindex.SeverityBlocking,
			})
		}
	}
	return findings, nil
}

// docGoRefs returns the refs of card's Create, Edit, Extends, Delete, Rename and Move groups; a Rename group contributes both sides of each pair.
func docGoRefs(card planparser.Card) []string {
	var refs []string
	for _, group := range card.TargetGroups {
		switch group.Type {
		case planparser.CardTypeCreate, planparser.CardTypeEdit, planparser.CardTypeExtends, planparser.CardTypeDelete, planparser.CardTypeMove:
			refs = append(refs, group.Refs...)
		case planparser.CardTypeRename:
			for _, pair := range group.Pairs {
				refs = append(refs, pair.Old, pair.New)
			}
		}
	}
	return refs
}

// changedCodeDirs returns, in first-seen order, the package directories in which card changes non-test Go code.
func changedCodeDirs(plan *planparser.Plan, card planparser.Card, answers map[string]planindex.GlyphAnswer) []string {
	var dirs []string
	add := func(ref string, isTest bool) {
		dir, ok := codeDir(plan, ref, isTest)
		if ok && !slices.Contains(dirs, dir) {
			dirs = append(dirs, dir)
		}
	}
	for _, group := range card.TargetGroups {
		switch group.Type {
		case planparser.CardTypeCreate, planparser.CardTypeEdit, planparser.CardTypeExtends, planparser.CardTypeDelete, planparser.CardTypeMove:
			for _, ref := range group.Refs {
				add(ref, isTestCode(plan, ref, answers))
			}
		case planparser.CardTypeRename:
			for _, pair := range group.Pairs {
				oldIsTest := isTestCode(plan, pair.Old, answers)
				add(pair.Old, oldIsTest)
				// A new side that does not resolve yet is test code exactly when its old side is.
				add(pair.New, oldIsTest || isTestCode(plan, pair.New, answers))
			}
		}
	}
	return dirs
}

// codeDir returns the package directory ref changes non-test Go code in, and false when ref is test code, a non-Go file or names no unit.
func codeDir(plan *planparser.Plan, ref string, isTest bool) (string, bool) {
	if isTest {
		return "", false
	}
	scope, isFile, ok := planparser.RefScope(plan, ref)
	if !ok {
		return "", false
	}
	if isFile {
		if !strings.HasSuffix(scope, ".go") {
			return "", false
		}
		return path.Dir(scope), true
	}
	return scope, true
}

// isTestCode reports whether ref names test code: a file ending _test.go, a member of a _test pseudo-unit, a member declared only in _test.go files, or an unresolved member named like a test.
// An ambiguous answer, and any other ref, is non-test code.
func isTestCode(plan *planparser.Plan, ref string, answers map[string]planindex.GlyphAnswer) bool {
	if scope, isFile, ok := planparser.RefScope(plan, ref); ok && isFile {
		return strings.HasSuffix(scope, "_test.go")
	}
	member, ok := planparser.RefMember(plan, ref)
	if !ok {
		return false
	}
	if isTestUnitMember(plan, ref) {
		return true
	}
	switch answer := answers[member]; answer.Verdict {
	case planindex.GlyphFound:
		return len(answer.Files) > 0 && !slices.ContainsFunc(answer.Files, func(file string) bool { return !strings.HasSuffix(file, "_test.go") })
	case planindex.GlyphAmbiguous:
		return false
	}
	parsed, err := planparser.ParseRef(plan, strings.TrimPrefix(ref, planparser.HandlePrefix))
	if err != nil {
		return false
	}
	return slices.ContainsFunc(testMemberPrefixes, func(prefix string) bool { return strings.HasPrefix(parsed.Name, prefix) })
}

// isTestUnitMember reports whether ref is a member glyph whose unit is an external test package's `<dir>_test` pseudo-unit.
func isTestUnitMember(plan *planparser.Plan, ref string) bool {
	if _, ok := planparser.RefMember(plan, ref); !ok {
		return false
	}
	parsed, err := planparser.ParseRef(plan, strings.TrimPrefix(ref, planparser.HandlePrefix))
	if err != nil {
		return false
	}
	unit, ok := parsed.UnitPath()
	return ok && strings.HasSuffix(unit, "_test") && path.Ext(unit) == ""
}

// coversDocGo reports whether card names docPath in any group or Uses, targets the package self glyph of dir, or has a later card of plan editing or extending docPath.
func coversDocGo(plan *planparser.Plan, card planparser.Card, dir, docPath string) bool {
	refs := slices.Clone(card.Uses)
	for _, group := range card.TargetGroups {
		refs = append(refs, group.Refs...)
	}
	for _, ref := range refs {
		body := strings.TrimPrefix(ref, planparser.HandlePrefix)
		if body == dir+"#" {
			return true
		}
		if file, ok := planparser.RefFile(plan, ref); ok && file == docPath {
			return true
		}
	}
	for _, later := range plan.Cards {
		if later.Number <= card.Number {
			continue
		}
		for _, group := range later.TargetGroups {
			if group.Type != planparser.CardTypeEdit && group.Type != planparser.CardTypeExtends {
				continue
			}
			for _, ref := range group.Refs {
				if file, ok := planparser.RefFile(plan, ref); ok && file == docPath {
					return true
				}
			}
		}
	}
	return false
}
