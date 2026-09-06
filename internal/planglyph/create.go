// create.go implements createFindings, the per-group inversion a Create group's own targets
// require, plus createTargetSet, the exclusion set resolvePass uses to keep statusFindings
// (resolve.go) from also reporting on the same targets — otherwise the same target would produce
// both a glyph-not-found finding and a pass, a contradiction in one report.
//
// This check also discharges the plan spec's own card-types table obligation for Create — "none —
// check nothing equivalent exists first" — which had no mechanical implementation before.

package planglyph

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/quarry/quarry"
)

// createTargetSet returns the set of every Create group's own Refs across plan, whether
// handle-shaped or glyph-shaped: resolvePass excludes these from statusFindings, and this file's
// own createFindings handles them instead.
func createTargetSet(plan *planparser.Plan) map[string]bool {
	set := make(map[string]bool)
	for _, c := range plan.Cards {
		for _, g := range c.TargetGroups {
			if g.Type != planparser.CardTypeCreate {
				continue
			}
			for _, t := range g.Refs {
				set[t] = true
			}
		}
	}
	return set
}

// resultByTarget indexes results by their own Target, for a group-scoped lookup by both
// createFindings and CanonicalizeHandles (handle.go).
func resultByTarget(results []quarry.ResolveResult) map[string]quarry.ResolveResult {
	index := make(map[string]quarry.ResolveResult, len(results))
	for _, r := range results {
		index[r.Target] = r
	}
	return index
}

// createHandleResults resolves the expected glyph standing behind every Create group's plan:
// handle, in one batched call, and returns the answers keyed by the HANDLE each one is for — the
// spelling the card itself carries, so createFindings can look a handle up exactly as it looks a
// glyph up.
//
// It is a second batched Resolve, and deliberately so: a handle's expected glyph is only knowable
// after CanonicalizeHandles has computed it, and canonicalization itself consumes the first batched
// Resolve's answers. The one-batched-call discipline is per pass, not per validation; the
// alternative is leaving the Create inversion inert for handles, which is exactly the defect this
// exists to close — and the inversion is the check that keeps a misspelled unit from silently
// creating a package nobody intended, in the one shape (a handle) the plan format prescribes for
// creating something genuinely new.
//
// It must run against an already-canonicalized plan; resolvePass is its only caller and does.
func createHandleResults(repo *quarry.Repo, plan *planparser.Plan) (map[string]quarry.ResolveResult, error) {
	expected := make(map[string]string) // handle -> the glyph it stands for
	var targets []string
	for _, c := range plan.Cards {
		for _, g := range c.TargetGroups {
			if g.Type != planparser.CardTypeCreate {
				continue
			}
			for _, ref := range g.Refs {
				if !strings.HasPrefix(ref, planparser.HandlePrefix) {
					continue
				}
				if _, seen := expected[ref]; seen {
					continue
				}
				key := resolveKeyFor(ref)
				expected[ref] = key
				targets = append(targets, key)
			}
		}
	}
	if len(targets) == 0 {
		return nil, nil
	}
	sort.Strings(targets)

	results, err := resolveTargets(repo, targets)
	if err != nil {
		return nil, err
	}

	byGlyph := resultByTarget(results)
	byHandle := make(map[string]quarry.ResolveResult, len(expected))
	for handle, key := range expected {
		if r, ok := byGlyph[key]; ok {
			byHandle[handle] = r
		}
	}
	return byHandle, nil
}

// createFindings inverts the resolve verdict for every Create group's own targets, glyph-shaped and
// handle-shaped alike — index is keyed by the ref as the card spells it, so a handle resolves
// through the answer createHandleResults collected for the glyph it stands for. A found or
// multipart result is the blocking finding create-already-exists, naming the target and the status
// that contradicts it. A not_found result with unit: found passes with no finding: creating a
// package is creating its first symbol, so demanding unit: found is incoherent for a Create target
// — the package does not exist apart from its files. A not_found result with unit: not_found
// produces the informational finding create-new-unit, naming the new unit explicitly, so a
// misspelled unit cannot silently create a package nobody intended.
func createFindings(plan *planparser.Plan, index map[string]quarry.ResolveResult) []Finding {
	var findings []Finding

	for _, c := range plan.Cards {
		for _, g := range c.TargetGroups {
			if g.Type != planparser.CardTypeCreate {
				continue
			}
			for _, t := range g.Refs {
				r, ok := index[t]
				if !ok {
					continue // a path, a bare symbol, or any ref collectGlyphTargets excluded.
				}

				switch r.Status {
				case quarry.StatusFound, quarry.StatusMultipart:
					findings = append(findings, Finding{
						Check:    "create-already-exists",
						Card:     cardIDOf(c),
						Detail:   fmt.Sprintf("Create target %q already resolves %s", t, r.Status),
						Severity: SeverityBlocking,
					})
				case quarry.StatusNotFound:
					if r.Unit == quarry.StatusNotFound {
						findings = append(findings, Finding{
							Check:    "create-new-unit",
							Card:     cardIDOf(c),
							Detail:   fmt.Sprintf("Create target %q introduces a new unit", t),
							Severity: SeverityInformational,
						})
					}
					// unit: found passes with no finding.
				}
			}
		}
	}

	return findings
}
