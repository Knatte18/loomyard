// donecheck.go implements DoneChecks, the mechanical verdict a card's completion has never had
// before: a Create group's own target that still does not resolve, a Delete group's own target
// that still does resolve, and a Rename pair whose old side still resolves or whose new side still
// does not, all block.
//
// The Delete gate would ideally also want quarry's parked assert-no-callers check, which is not
// available and is not in this task's scope, so a Delete whose target was in fact renamed rather
// than genuinely removed is caught by card 36's drift detector instead of by this check.

package planglyph

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/quarry/quarry"
)

// doneCheckEntry pairs one Create/Delete group ref with the card it belongs to, the check ID its
// group type calls for, and the resolve key actually looked up: a plan: handle's own resolve key
// is its member-glyph half — the string after HandlePrefix, which by canonicalization time is the
// card's own expected glyph — never the literal "plan:"-prefixed token quarry cannot parse.
type doneCheckEntry struct {
	card    planparser.Card
	checkID string
	key     string
	display string
}

// resolveKeyFor maps a Create/Delete group's own ref to the string DoneChecks actually resolves:
// a plan: handle strips to its expected-glyph half, and every other ref (a plain glyph, or a path
// naming a whole new/removed file) resolves as-is.
func resolveKeyFor(ref string) string {
	if strings.HasPrefix(ref, planparser.HandlePrefix) {
		return strings.TrimPrefix(ref, planparser.HandlePrefix)
	}
	return ref
}

// DoneChecks issues one batched resolve over cards' own Create, Delete, and Rename group targets
// against worktreeRoot's current tree, and applies three rules, all blocking because none is a
// judgment call: a Create target that still does not resolve found or multipart is check ID
// create-not-done; a Delete target that still resolves found or multipart is check ID
// delete-not-done; and a Rename pair whose old side still resolves, or whose new side still does
// not, is check ID rename-not-done in either direction.
//
// An infrastructure error from the resolve blocks the done-checks rather than passing them: a
// Create done-check that could not resolve is indistinguishable from a Create that never happened,
// and passing it would be a false success. The wrapped ErrQuarryUnavailable error is returned so
// the caller can tell the two apart.
//
// That rule binds a resolve that ANSWERED INCOMPLETELY exactly as it binds one that failed
// outright. Every key looked up below was put into the target list by this function itself, so a
// missing answer is quarry's positional contract not holding rather than a target this function
// never asked about — and it too returns a wrapped ErrQuarryUnavailable rather than skipping the
// entry, which would silently pass whichever blocking check that target carried.
func DoneChecks(plan *planparser.Plan, cards []planparser.Card, worktreeRoot string) ([]Finding, error) {
	if _, ok := resolveLanguage(plan); !ok {
		return nil, nil
	}

	var entries []doneCheckEntry
	seen := make(map[string]bool)
	var targets []string
	addTarget := func(key string) {
		if !seen[key] {
			seen[key] = true
			targets = append(targets, key)
		}
	}

	for _, c := range cards {
		for _, g := range c.TargetGroups {
			switch g.Type {
			case planparser.CardTypeCreate:
				for _, ref := range g.Refs {
					key := resolveKeyFor(ref)
					entries = append(entries, doneCheckEntry{card: c, checkID: "create-not-done", key: key, display: ref})
					addTarget(key)
				}
			case planparser.CardTypeDelete:
				for _, ref := range g.Refs {
					key := resolveKeyFor(ref)
					entries = append(entries, doneCheckEntry{card: c, checkID: "delete-not-done", key: key, display: ref})
					addTarget(key)
				}
			case planparser.CardTypeRename:
				// A Rename's mechanical done verdict is the Create and Delete verdicts composed:
				// the Old side must have stopped resolving (its Delete half) and the New side —
				// resolveKeyFor strips a symbol rename's canonical plan: handle to its expected
				// glyph, and passes a file rename's destination self glyph through — must resolve
				// now (its Create half). Both report under one check ID, rename-not-done, with the
				// detail naming which half failed. Without this a fork that skipped its Rename card
				// entirely recorded clean: DoneChecks saw no Create/Delete group, BindHandles no
				// declarations, and DetectDrift only inspects what the delta says DID change.
				for _, p := range g.Pairs {
					oldKey := resolveKeyFor(p.Old)
					entries = append(entries, doneCheckEntry{card: c, checkID: "rename-not-done-old", key: oldKey, display: p.Old})
					addTarget(oldKey)
					newKey := resolveKeyFor(p.New)
					entries = append(entries, doneCheckEntry{card: c, checkID: "rename-not-done-new", key: newKey, display: p.New})
					addTarget(newKey)
				}
			}
		}
	}

	if len(entries) == 0 {
		return nil, nil
	}
	sort.Strings(targets)

	repo, err := openRepo(worktreeRoot)
	if err != nil {
		return nil, err
	}
	results, err := resolveTargets(repo, targets)
	if err != nil {
		return nil, err
	}
	index := resultByTarget(results)

	return doneCheckVerdicts(entries, index)
}

// doneCheckVerdicts applies the three done rules to entries against index, the batched resolve's
// answers keyed by target.
//
// It is split out from DoneChecks so the coverage guard below is reachable from a unit test: every
// other path into it runs a real quarry.Repo, and the one condition worth pinning -- an answer set
// that does not cover a target the caller asked about -- cannot be produced through one.
func doneCheckVerdicts(entries []doneCheckEntry, index map[string]quarry.ResolveResult) ([]Finding, error) {
	var findings []Finding
	for _, e := range entries {
		r, ok := index[e.key]
		if !ok {
			// Every key here was put into targets by this function itself and handed to
			// resolveTargets, so an absent answer means the batched resolve did not cover a target
			// it was asked about. Skipping the entry would let a blocking done-check PASS, which is
			// the exact disposition this function's own contract rejects: a done-check that could
			// not resolve is indistinguishable from work that never happened. Report it as
			// infrastructure, the way CanonicalizeHandles guards quarry.Name's positional contract
			// (handle.go), so nobody reads "quarry did not answer for this target" as a verdict on
			// the plan (crucible round opus-medium-r5, R5-6).
			return findings, fmt.Errorf("%w: resolve returned no answer for done-check target %q (card %s)", ErrQuarryUnavailable, e.key, cardIDOf(e.card))
		}
		resolved := r.Status == quarry.StatusFound || r.Status == quarry.StatusMultipart

		switch e.checkID {
		case "create-not-done":
			if !resolved {
				findings = append(findings, Finding{
					Check:    "create-not-done",
					Card:     cardIDOf(e.card),
					Detail:   fmt.Sprintf("Create target %q still does not resolve", e.display),
					Severity: SeverityBlocking,
				})
			}
		case "delete-not-done":
			if resolved {
				findings = append(findings, Finding{
					Check:    "delete-not-done",
					Card:     cardIDOf(e.card),
					Detail:   fmt.Sprintf("Delete target %q still resolves %s", e.display, r.Status),
					Severity: SeverityBlocking,
				})
			}
		case "rename-not-done-old":
			if resolved {
				findings = append(findings, Finding{
					Check:    "rename-not-done",
					Card:     cardIDOf(e.card),
					Detail:   fmt.Sprintf("Rename pair's old side %q still resolves %s — the rename did not happen", e.display, r.Status),
					Severity: SeverityBlocking,
				})
			}
		case "rename-not-done-new":
			if !resolved {
				findings = append(findings, Finding{
					Check:    "rename-not-done",
					Card:     cardIDOf(e.card),
					Detail:   fmt.Sprintf("Rename pair's new side %q still does not resolve — the rename did not happen", e.display),
					Severity: SeverityBlocking,
				})
			}
		}
	}

	return findings, nil
}
