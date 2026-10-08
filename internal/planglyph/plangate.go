// plangate.go holds planGatePass, the resolve-backed checks that run at the plan gates only: ValidateFormat and Validate call it, ValidateDispatch never does.
// A check whose verdict depends on state the run itself changes belongs here, because dispatch re-validates a plan against a tree the run has already changed.

package planglyph

import (
	"fmt"
	"path"
	"slices"
	"sort"
	"strings"

	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/quarry/glyph"
	"github.com/Knatte18/quarry/quarry"
)

// planGatePass reports redundant-file-target: a card listing a file self glyph and a member glyph that resolves into that file.
// One finding per file and member, attributed to the card, with Ref the member.
// It also reports resign-interface-method: a re-sign arrow whose member resolves to an interface method.
// A member that resolves not_found, ambiguous or unreadably is skipped, since the status policy already reports it.
// Under a non-glyph language it returns nothing and opens no repository.
// An infrastructure error is wrapped in ErrQuarryUnavailable.
func planGatePass(plan *planparser.Plan, worktreeRoot string) ([]Finding, error) {
	lang, ok := plan.GlyphLanguage()
	if !ok {
		return nil, nil
	}

	repo, err := openRepo(worktreeRoot)
	if err != nil {
		return nil, err
	}

	results, err := resolveTargets(repo, collectMemberTargets(plan, lang))
	if err != nil {
		return nil, err
	}
	answers := resultByTarget(results)

	var findings []Finding
	for _, c := range plan.Cards {
		var files, members []string
		for _, t := range c.Targets {
			if planparser.IsHandleRef(t) {
				continue
			}
			g, err := glyph.Parse(lang, t)
			if err != nil {
				continue
			}
			if !g.IsSelf() {
				members = append(members, t)
				continue
			}
			if unitPath, ok := g.UnitPath(); ok && path.Ext(unitPath) != "" {
				files = append(files, unitPath)
			}
		}

		for _, file := range files {
			for _, member := range members {
				symbols, readable := answerSymbols(answers[member])
				if !readable || !slices.ContainsFunc(symbols, func(s quarry.Symbol) bool { return s.File == file }) {
					continue
				}
				findings = append(findings, Finding{
					Check: "redundant-file-target",
					Card:  c.ID(),
					Detail: fmt.Sprintf(
						"card %d lists the file %q beside %q, which resolves into that file; keep the member glyphs and drop the file, or keep the file when the card changes the whole file",
						c.Number, file, member,
					),
					Severity: SeverityBlocking,
					Ref:      member,
				})
			}
		}

		for _, r := range c.Resigns {
			symbols, readable := answerSymbols(answers[r.Target])
			if !readable || !slices.ContainsFunc(symbols, isInterfaceMethod) {
				continue
			}
			findings = append(findings, Finding{
				Check: "resign-interface-method",
				Card:  c.ID(),
				Detail: fmt.Sprintf(
					"card %d re-sign arrow on %q: it is an interface method, whose own spec is no declaration a head can re-sign; drop the arrow and state the signature change in the card's Intent",
					c.Number, r.Target,
				),
				Severity: SeverityBlocking,
				Ref:      r.Target,
			})
		}
	}
	return findings, nil
}

// isInterfaceMethod reports whether s is a method whose signature does not open with func, quarry's answer for an interface method.
func isInterfaceMethod(s quarry.Symbol) bool {
	return s.Kind == quarry.KindMethod && !strings.HasPrefix(s.Signature, "func")
}

// collectMemberTargets returns every distinct member glyph plan's cards list among their own Targets, sorted for determinism.
// A handle and a self glyph are excluded.
func collectMemberTargets(plan *planparser.Plan, lang glyph.Language) []string {
	seen := make(map[string]bool)
	for _, c := range plan.Cards {
		for _, t := range c.Targets {
			if planparser.IsHandleRef(t) {
				continue
			}
			if g, err := glyph.Parse(lang, t); err == nil && !g.IsSelf() {
				seen[t] = true
			}
		}
	}

	targets := make([]string, 0, len(seen))
	for t := range seen {
		targets = append(targets, t)
	}
	sort.Strings(targets)
	return targets
}
