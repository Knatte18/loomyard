// seamtest.go holds the plan gate's seam-test-unlisted check: a card that touches a struct a `//lym:seam` test reflects over must cover that test.

package loomshed

import (
	"fmt"
	"slices"
	"strings"

	"github.com/Knatte18/loomyard/internal/impactset"
	"github.com/Knatte18/loomyard/internal/planindex"
	"github.com/Knatte18/loomyard/internal/planparser"
)

// seamTestUnlistedCheck names the finding seamTestFindings appends for a card that touches a marked struct without covering the marked test.
const seamTestUnlistedCheck = "seam-test-unlisted"

// seamTestWayForward is the way-forward clause every seam-test-unlisted finding carries.
const seamTestWayForward = "way forward: list it under Edit by its member glyph when the card changes it, under Uses when it stays as is and no later card targets it, leave it to the later card that edits it, else list it under Edit"

// SeamTestReader returns the reader a PlanContext's SeamTests field takes: called at check time, it parses the `//lym:seam` markers of every package directory under worktreeRoot and returns the marked tests in directory order.
// A marker the parser rejects, or a test file that does not parse, is returned as the error.
func SeamTestReader(worktreeRoot string) func() ([]impactset.SeamTest, error) {
	return func() ([]impactset.SeamTest, error) {
		dirs, err := goPackageDirs(worktreeRoot)
		if err != nil {
			return nil, err
		}
		found, err := impactset.FindSeamTests(worktreeRoot, dirs)
		if err != nil {
			return nil, err
		}
		var seams []impactset.SeamTest
		for _, dir := range dirs {
			seams = append(seams, found[dir]...)
		}
		return seams, nil
	}
}

// seamTestFindings returns one seam-test-unlisted finding per card of cards, marked struct it touches and marked test of that struct the card does not cover.
// A card touches a marked struct `u#T` when a ref of its Create, Edit, Extends, Delete or Rename groups, with a `plan:` prefix stripped, is `u#T`, a member `u#T.<x>`, the file self glyph of a file declaring `T`, or the package self glyph `u#`.
// It covers a test when its refs name the test's glyph or the glyph's package counterpart, the test's file, or the package self glyph of the test's unit or of its counterpart unit, or when a later card names the test glyph or its counterpart under Edit or Extends.
// Every marked struct is resolved in one index call, made only when cards and seams are both non-empty.
func seamTestFindings(plan *planparser.Plan, cards []planparser.Card, worktreeRoot string, index planindex.Index, seams []impactset.SeamTest) ([]planindex.Finding, error) {
	var glyphs []string
	for _, seam := range seams {
		for _, structGlyph := range seam.Structs {
			if !slices.Contains(glyphs, structGlyph) {
				glyphs = append(glyphs, structGlyph)
			}
		}
	}
	if len(cards) == 0 || len(glyphs) == 0 {
		return nil, nil
	}
	answers, err := index.ResolveGlyphs(worktreeRoot, glyphs)
	if err != nil {
		return nil, err
	}
	declaringFiles := make(map[string][]string, len(glyphs))
	for i, answer := range answers {
		if answer.Verdict == planindex.GlyphFound {
			declaringFiles[glyphs[i]] = answer.Files
		}
	}

	var findings []planindex.Finding
	for _, card := range cards {
		touchRefs := strippedRefs(card, touchGroupTypes...)
		for _, seam := range seams {
			for _, structGlyph := range seam.Structs {
				if !touchesStruct(touchRefs, structGlyph, declaringFiles[structGlyph]) || coversSeamTest(plan, card, seam) {
					continue
				}
				findings = append(findings, planindex.Finding{
					Check:    seamTestUnlistedCheck,
					Card:     card.ID(),
					Detail:   fmt.Sprintf("card %d touches %s, which the test %s#%s (%s) reflects over, and does not cover that test; %s", card.Number, structGlyph, seam.Unit, seam.Name, seam.File, seamTestWayForward),
					Severity: planindex.SeverityBlocking,
				})
			}
		}
	}
	return findings, nil
}

// touchGroupTypes are the card groups whose refs make a card touch a struct.
var touchGroupTypes = []planparser.CardType{
	planparser.CardTypeCreate, planparser.CardTypeEdit, planparser.CardTypeExtends, planparser.CardTypeDelete, planparser.CardTypeRename,
}

// strippedRefs returns the refs of card's groups of the given types, or of every group and Uses when types is empty, each with a `plan:` prefix stripped.
func strippedRefs(card planparser.Card, types ...planparser.CardType) []string {
	var refs []string
	for _, group := range card.TargetGroups {
		if len(types) == 0 || slices.Contains(types, group.Type) {
			refs = append(refs, group.Refs...)
		}
	}
	if len(types) == 0 {
		refs = append(refs, card.Uses...)
	}
	return stripHandles(refs)
}

// stripHandles returns refs with each `plan:` prefix stripped.
func stripHandles(refs []string) []string {
	stripped := make([]string, len(refs))
	for i, ref := range refs {
		stripped[i] = ref
		if body, handle := planparser.HandleBody(ref); handle {
			stripped[i] = body
		}
	}
	return stripped
}

// touchesStruct reports whether refs name the struct glyph, one of its members, a file declaring it or its package.
func touchesStruct(refs []string, structGlyph string, declaringFiles []string) bool {
	unit, _, _ := strings.Cut(structGlyph, "#")
	for _, ref := range refs {
		if ref == structGlyph || strings.HasPrefix(ref, structGlyph+".") || ref == unit+"#" {
			return true
		}
		if path, ok := planparser.PathEntryPath(ref); ok && slices.Contains(declaringFiles, path) {
			return true
		}
	}
	return false
}

// coversSeamTest reports whether card lists seam, or a later card of plan edits or extends it.
func coversSeamTest(plan *planparser.Plan, card planparser.Card, seam impactset.SeamTest) bool {
	testGlyphs := []string{seam.Unit + "#" + seam.Name}
	packageGlyphs := []string{seam.Unit + "#"}
	if counterpart, ok := planparser.GoPackageCounterpart(plan, testGlyphs[0]); ok {
		testGlyphs = append(testGlyphs, counterpart)
		counterpartUnit, _, _ := strings.Cut(counterpart, "#")
		packageGlyphs = append(packageGlyphs, counterpartUnit+"#")
	}
	for _, ref := range strippedRefs(card) {
		if slices.Contains(testGlyphs, ref) || slices.Contains(packageGlyphs, ref) {
			return true
		}
	}
	fileRefs := append(strippedRefs(card, planparser.CardTypeDelete, planparser.CardTypeRename, planparser.CardTypeMove), stripHandles(card.Uses)...)
	for _, ref := range fileRefs {
		if path, ok := planparser.PathEntryPath(ref); ok && path == seam.File {
			return true
		}
	}
	for _, later := range plan.Cards {
		if later.Number <= card.Number {
			continue
		}
		for _, ref := range strippedRefs(later, planparser.CardTypeEdit, planparser.CardTypeExtends) {
			if slices.Contains(testGlyphs, ref) {
				return true
			}
		}
	}
	return false
}
