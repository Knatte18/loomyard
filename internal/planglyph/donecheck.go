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
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/quarry/quarry"
)

// scratchDirPrefix marks a Create target under the worktree root's git-ignored scratch directory.
// Such a target is a transient draft that never lands, which its card may remove once read, so no done-check judges it.
const scratchDirPrefix = ".scratch/"

// doneCheckEntry pairs one Create/Delete group ref with the card it belongs to, the check ID its
// group type calls for, and the resolve key actually looked up: a plan: handle's own resolve key
// is its member-glyph half — the string after HandlePrefix, which by canonicalization time is the
// card's own expected glyph — never the literal "plan:"-prefixed token quarry cannot parse.
// onDisk marks a key no glyph spells, which is judged by whether its worktree-relative path exists rather than resolved.
type doneCheckEntry struct {
	card    planparser.Card
	checkID string
	key     string
	display string
	onDisk  bool
}

// resolveKeyFor maps a Create/Delete group's own ref to the string DoneChecks actually resolves:
// a plan: handle strips to its expected-glyph half, and every other ref (a plain glyph, or a path
// naming a whole new/removed file) resolves as-is. planparser.HandleBody is the handle grammar's
// owner; this is a thin wrapper over it.
func resolveKeyFor(ref string) string {
	if body, ok := planparser.HandleBody(ref); ok {
		return body
	}
	return ref
}

// DoneChecks issues one batched resolve over cards' own Create, Delete, and Rename group targets
// against worktreeRoot's current tree, and applies three rules, all blocking because none is a
// judgment call: a Create target that still does not resolve found or multipart is check ID
// create-not-done; a Delete target whose answer is anything but not_found — found, multipart, or
// ambiguous, whose candidates are still-present declarations of that name — is check ID
// delete-not-done; and a Rename pair whose old side is still anything but not_found, or whose new
// side still does not resolve found or multipart, is check ID rename-not-done in either direction.
// A Create target under the worktree root's .scratch/ is a transient draft and is neither resolved nor judged.
// A target still path-shaped is a path canonicalization could not spell as a glyph, such as one whose directory holds a space:
// quarry rejects it before resolution, so its worktree-relative path is checked on disk instead, existing for the Create direction and absent for the Delete direction.
// A Create target or Rename new side quarry answers not_found also names, in its detail, the files declaring the looked-up unit
// and every declaration elsewhere in the worktree carrying the looked-up name, so a missing declaration reads apart from one written somewhere the card did not say.
// An answer outside quarry's four-value status vocabulary — a pre-resolution rejection, or a status
// this policy has not been taught — is the blocking finding glyph-rejected for that entry, failing
// closed exactly as statusFindings and createFindings do (see doneCheckVerdicts).
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
	if !plan.GlyphLanguage() {
		return nil, nil
	}

	var entries []doneCheckEntry
	seen := make(map[string]bool)
	var targets []string
	addEntry := func(c planparser.Card, checkID, key, display string) {
		onDisk := planparser.IsDiskCheckedTarget(key)
		entries = append(entries, doneCheckEntry{card: c, checkID: checkID, key: key, display: display, onDisk: onDisk})
		if !onDisk && !seen[key] {
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
					if strings.HasPrefix(key, scratchDirPrefix) {
						continue
					}
					addEntry(c, "create-not-done", key, ref)
				}
			case planparser.CardTypeDelete:
				for _, ref := range g.Refs {
					addEntry(c, "delete-not-done", resolveKeyFor(ref), ref)
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
					addEntry(c, "rename-not-done-old", resolveKeyFor(p.Old), p.Old)
					addEntry(c, "rename-not-done-new", resolveKeyFor(p.New), p.New)
				}
			}
		}
	}

	if len(entries) == 0 {
		return nil, nil
	}
	if len(targets) == 0 {
		return doneCheckVerdicts(plan, worktreeRoot, entries, nil, nil)
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

	var declarations []quarry.Symbol
	if anyMemberMissing(entries, index) {
		answer, err := repo.Glyphs(".")
		if err != nil {
			return nil, fmt.Errorf("%w: list the worktree's declarations: %v", ErrQuarryUnavailable, err)
		}
		declarations = answer.Symbols
	}

	return doneCheckVerdicts(plan, worktreeRoot, entries, index, declarations)
}

// anyMemberMissing reports whether a Create target or Rename new side among entries was answered not_found in index,
// the one answer whose finding names the worktree's declarations.
func anyMemberMissing(entries []doneCheckEntry, index map[string]quarry.ResolveResult) bool {
	for _, e := range entries {
		if e.onDisk || (e.checkID != "create-not-done" && e.checkID != "rename-not-done-new") {
			continue
		}
		if r, ok := index[e.key]; ok && r.Status == quarry.StatusNotFound {
			return true
		}
	}
	return false
}

// searchedDescription names, for a key quarry answered not_found, the files among declarations that declare the key's unit,
// and every declaration elsewhere whose own name is the key's member name, with its glyph and file.
// It returns "" when the key does not parse as a member glyph in the plan's alphabet, since then there is no name to look for.
func searchedDescription(plan *planparser.Plan, key string, declarations []quarry.Symbol) string {
	wanted, err := planparser.ParseRef(plan, key)
	if err != nil || wanted.IsSelf() {
		return ""
	}

	var unitFiles []string
	var elsewhere []string
	for _, s := range declarations {
		if s.Glyph.Unit == wanted.Unit && !slices.Contains(unitFiles, s.File) {
			unitFiles = append(unitFiles, s.File)
		}
		if s.Glyph.Name == wanted.Name && s.ID != key {
			elsewhere = append(elsewhere, fmt.Sprintf("%s in %s", s.ID, s.File))
		}
	}
	sort.Strings(unitFiles)

	var parts []string
	if len(unitFiles) > 0 {
		parts = append(parts, "quarry searched the unit's files "+strings.Join(unitFiles, ", "))
	}
	if len(elsewhere) > 0 {
		parts = append(parts, fmt.Sprintf("%s is declared as %s", wanted.Name, strings.Join(elsewhere, ", ")))
	} else {
		parts = append(parts, fmt.Sprintf("no declaration named %s exists anywhere in the worktree", wanted.Name))
	}
	return strings.Join(parts, "; ")
}

// diskLookup checks key, a worktree-relative path, on disk under worktreeRoot,
// and reports whether anything exists there together with the lookup description a finding detail carries.
// An error other than the path not existing is returned, so an unreadable tree never passes a done-check.
func diskLookup(worktreeRoot, key string) (bool, string, error) {
	_, err := os.Lstat(filepath.Join(worktreeRoot, filepath.FromSlash(key)))
	switch {
	case err == nil:
		return true, fmt.Sprintf("checked %q on disk, since no glyph spells it, and it exists", key), nil
	case errors.Is(err, fs.ErrNotExist):
		return false, fmt.Sprintf("checked %q on disk, since no glyph spells it, and nothing exists there", key), nil
	default:
		return false, "", fmt.Errorf("done-check: stat %q: %w", key, err)
	}
}

// lookupDescription renders what a done-check looked up and what quarry answered, for a finding detail.
// It names the resolve key and quarry's status, for not_found whether the unit exists, and for ambiguous each candidate's file and build constraint.
func lookupDescription(key string, status quarry.Status, unitFound bool, candidates []constrainedCandidate) string {
	description := fmt.Sprintf("looked up %q, quarry answered %s", key, status)
	switch status {
	case quarry.StatusNotFound:
		if unitFound {
			description += " (the unit exists and the member is missing)"
		} else {
			description += " (the unit is missing)"
		}
	case quarry.StatusAmbiguous:
		description += " among " + candidateList(candidates)
	}
	return description
}

// doneCheckVerdicts applies the three done rules to entries against index, the batched resolve's
// answers keyed by target, reading ambiguous candidates' files and onDisk entries' paths under worktreeRoot.
// Each finding's detail names the lookup: the resolve key, quarry's status, the unit status of a
// missing member, and each candidate's file and build constraint for an ambiguous answer.
// A Create or Rename new side answered not_found also names, from declarations, the worktree's listed declarations, its unit's files and every declaration of its name.
//
// It is split out from DoneChecks so the coverage guard below is reachable from a unit test: every
// other path into it runs a real quarry.Repo, and the one condition worth pinning -- an answer set
// that does not cover a target the caller asked about -- cannot be produced through one.
func doneCheckVerdicts(plan *planparser.Plan, worktreeRoot string, entries []doneCheckEntry, index map[string]quarry.ResolveResult, declarations []quarry.Symbol) ([]Finding, error) {
	var findings []Finding
	for _, e := range entries {
		if e.onDisk {
			exists, lookup, err := diskLookup(worktreeRoot, e.key)
			if err != nil {
				return findings, err
			}
			if finding, failed := doneCheckFinding(e, exists, exists, lookup); failed {
				findings = append(findings, finding)
			}
			continue
		}

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
			return findings, fmt.Errorf("%w: resolve returned no answer for done-check target %q (card %s)", ErrQuarryUnavailable, e.key, e.card.ID())
		}

		// Fail closed on an answer outside quarry's vocabulary — quarry.Status.Known(), the
		// predicate quarry.Statuses now owns — BEFORE reading it into either boolean below. The
		// old single boolean ("resolved = found || multipart") folded a pre-resolution rejection
		// (Status "", Error/Reason set) and any future unrecognized status into "the target is
		// gone", which fails OPEN for delete-not-done and rename-not-done-old: an unreadable
		// answer counted as a successful deletion. Same disposition statusFindings and
		// createFindings adopted in crucible round opus-high-r9's R9-6, and the same glyph-rejected
		// check ID, so the vocabulary can only ever widen deliberately (crucible round
		// fable-high-r10, F1).
		if !r.Status.Known() {
			findings = append(findings, Finding{
				Check:    "glyph-rejected",
				Card:     e.card.ID(),
				Detail:   unreadableStatusDetail("done-check target", e.display, r),
				Severity: SeverityBlocking,
			})
			continue
		}

		// The two directions read different questions off the four readable statuses. resolved is
		// the Create direction's bar: the work landed only when the target resolves cleanly (found
		// or multipart). stillExists is the Delete direction's bar: anything but not_found means a
		// declaration with that name is still present — INCLUDING ambiguous, whose candidates are
		// exactly such declarations, so an ambiguous answer must block a deletion verdict rather
		// than pass it (the old boolean passed it; crucible round fable-high-r10, F1).
		//
		// An ambiguous answer whose candidates sit in files no single build environment selects together is one member declared per constraint set, so it counts as resolved.
		// The Delete direction still blocks on it, since each of those declarations is still present.
		var classified ambiguity
		if r.Status == quarry.StatusAmbiguous {
			classified = classifyAmbiguity(worktreeRoot, r.Candidates)
		}
		resolved := r.Status == quarry.StatusFound || r.Status == quarry.StatusMultipart || classified.Partitioned
		stillExists := r.Status != quarry.StatusNotFound
		lookup := lookupDescription(e.key, r.Status, r.Unit == quarry.StatusFound, classified.Candidates)
		if crossDetail, crossAlphabet := crossAlphabetDetail(r); crossAlphabet {
			lookup = fmt.Sprintf("looked up %q, quarry answered ambiguous: %s", e.key, crossDetail)
		}
		if r.Status == quarry.StatusNotFound && (e.checkID == "create-not-done" || e.checkID == "rename-not-done-new") {
			if searched := searchedDescription(plan, e.key, declarations); searched != "" {
				lookup += "; " + searched
			}
		}

		if finding, failed := doneCheckFinding(e, resolved, stillExists, lookup); failed {
			findings = append(findings, finding)
		}
	}

	return findings, nil
}

// doneCheckFinding applies e's done rule: the Create direction fails when the target is not resolved, the Delete direction when it still exists.
// It returns the blocking finding, its detail ending in lookup, and whether the rule failed.
func doneCheckFinding(e doneCheckEntry, resolved, stillExists bool, lookup string) (Finding, bool) {
	var check, detail string
	switch e.checkID {
	case "create-not-done":
		if resolved {
			return Finding{}, false
		}
		check, detail = "create-not-done", fmt.Sprintf("Create target %q still does not resolve: %s", e.display, lookup)
	case "delete-not-done":
		if !stillExists {
			return Finding{}, false
		}
		check, detail = "delete-not-done", fmt.Sprintf("Delete target %q still resolves: %s", e.display, lookup)
	case "rename-not-done-old":
		if !stillExists {
			return Finding{}, false
		}
		check, detail = "rename-not-done", fmt.Sprintf("Rename pair's old side %q still resolves — the rename did not happen: %s", e.display, lookup)
	case "rename-not-done-new":
		if resolved {
			return Finding{}, false
		}
		check, detail = "rename-not-done", fmt.Sprintf("Rename pair's new side %q still does not resolve — the rename did not happen: %s", e.display, lookup)
	default:
		panic(fmt.Sprintf("planglyph: done-check entry with unknown check ID %q", e.checkID))
	}
	return Finding{Check: check, Card: e.card.ID(), Detail: detail, Severity: SeverityBlocking}, true
}
