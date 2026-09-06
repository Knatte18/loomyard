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

// createFindings inverts the resolve verdict for every Create group's own glyph-shaped targets —
// a plan: handle target is never looked up here at all, since quarry never sees a handle and no
// result exists for it. A found or multipart result is the blocking finding
// create-already-exists, naming the target and the status that contradicts it. A not_found result
// with unit: found passes with no finding: creating a package is creating its first symbol, so
// demanding unit: found is incoherent for a Create target — the package does not exist apart from
// its files. A not_found result with unit: not_found produces the informational finding
// create-new-unit, naming the new unit explicitly, so a misspelled unit cannot silently create a
// package nobody intended.
func createFindings(plan *planparser.Plan, results []quarry.ResolveResult) []Finding {
	var findings []Finding
	index := resultByTarget(results)

	for _, c := range plan.Cards {
		for _, g := range c.TargetGroups {
			if g.Type != planparser.CardTypeCreate {
				continue
			}
			for _, t := range g.Refs {
				r, ok := index[t]
				if !ok {
					continue // a plan: handle, or any ref collectGlyphTargets excluded.
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
