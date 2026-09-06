// donecheck.go implements DoneChecks, the mechanical verdict a card's completion has never had
// before: a Create group's own target that still does not resolve, and a Delete group's own target
// that still does resolve, both block.
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

// DoneChecks issues one batched resolve over cards' own Create and Delete group targets against
// worktreeRoot's current tree, and applies two rules, both blocking because neither is a judgment
// call: a Create target that still does not resolve found or multipart is check ID
// create-not-done, and a Delete target that still resolves found or multipart is check ID
// delete-not-done.
//
// An infrastructure error from the resolve blocks the done-checks rather than passing them: a
// Create done-check that could not resolve is indistinguishable from a Create that never happened,
// and passing it would be a false success. The wrapped ErrQuarryUnavailable error is returned so
// the caller can tell the two apart.
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

	var findings []Finding
	for _, e := range entries {
		r, ok := index[e.key]
		if !ok {
			continue
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
		}
	}

	return findings, nil
}
