// languagemismatch.go implements languageMismatchFindings, the resolve-backed half of plan-language-mismatch:
// a go plan whose targets resolve only into C# files is refused with the way forward the pure half names.

package planglyph

import (
	"fmt"
	"path"
	"strings"

	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/quarry/quarry"
)

// languageMismatchFindings reports plan-language-mismatch for a go plan whose targets name C# files and no Go file, judged over results, the batched resolve answers.
// The finding is blocking and attributed to no card.
// Go evidence is a path target naming a .go file, a found or multipart answer with a symbol in a .go file, or a found self glyph whose listing holds a .go file.
// C# evidence is the same three with .cs, and a .cs path target counts only beside a target that is not a file path, since a plan of file paths alone is the pure half's to judge.
// It never fires when a non-path target has no found or multipart answer, since that glyph's own finding stands.
// It never fires when a card creates a declaration whose head begins with a Go keyword, since the batched resolve never sees a handle and such a handle names new Go code.
// The caller runs it only when no card is done, so it judges the whole plan.
func languageMismatchFindings(plan *planparser.Plan, results []quarry.ResolveResult) []Finding {
	for _, c := range plan.Cards {
		for _, declaration := range c.Declarations {
			if opensGoDeclaration(declaration.Decl) {
				return nil
			}
		}
	}

	var goEvidence, csharpEvidence, csharpPath, nonPathTarget bool
	nonPathGlyphs := map[string]bool{}
	for _, c := range plan.Cards {
		for _, target := range c.Targets {
			file, isPath := planparser.PathEntryPath(target)
			if planparser.IsHandleRef(target) {
				nonPathTarget = true
				continue
			}
			if !isPath || path.Ext(file) == "" {
				nonPathGlyphs[target] = true
				continue
			}
			switch path.Ext(file) {
			case ".go":
				goEvidence = true
			case ".cs":
				csharpPath = true
			}
		}
	}

	// Symbols and Listing are set only on a found or multipart answer.
	// A non-path target with any other answer is that glyph's own finding, and it hides what language the target belongs to, so no mismatch is judged.
	for _, r := range results {
		if nonPathGlyphs[r.Target] {
			if len(r.Symbols) == 0 && r.Listing == nil && r.Listings == nil {
				return nil
			}
			nonPathTarget = true
		}
		for _, s := range r.Symbols {
			noteFileEvidence(s.File, &goEvidence, &csharpEvidence)
		}
		if r.Listing != nil {
			noteListingEvidence(*r.Listing, &goEvidence, &csharpEvidence)
		}
	}

	if goEvidence || !(csharpEvidence || (csharpPath && nonPathTarget)) {
		return nil
	}
	return []Finding{{
		Check: "plan-language-mismatch",
		Detail: fmt.Sprintf(
			"the plan's language is %q but its targets resolve into C# (.cs) files only; set `language:` to csharp, the language of the files, or to `language: none` for a plan that changes both",
			plan.Language,
		),
		Severity: SeverityBlocking,
	}}
}

// opensGoDeclaration reports whether head begins with the Go keyword func, type, var or const.
func opensGoDeclaration(head string) bool {
	fields := strings.Fields(head)
	if len(fields) == 0 {
		return false
	}
	switch fields[0] {
	case "func", "type", "var", "const":
		return true
	}
	return false
}

// noteFileEvidence sets goEvidence when file ends .go and csharpEvidence when it ends .cs.
func noteFileEvidence(file string, goEvidence, csharpEvidence *bool) {
	switch path.Ext(file) {
	case ".go":
		*goEvidence = true
	case ".cs":
		*csharpEvidence = true
	}
}

// noteListingEvidence applies noteFileEvidence to every file of listing and of its subdirectories.
func noteListingEvidence(listing quarry.DirAnswer, goEvidence, csharpEvidence *bool) {
	for _, file := range listing.Files {
		noteFileEvidence(file.Name, goEvidence, csharpEvidence)
	}
	for _, dir := range listing.Dirs {
		noteListingEvidence(dir, goEvidence, csharpEvidence)
	}
}
