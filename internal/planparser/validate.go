// validate.go implements Validate, plan-format v3's complete machine check set
// (docs/reference/plan-format-v3.md, "Validation checks"), run in this fixed
// order: format/approval (format-unrecognized, plan-unapproved), Card Index <->
// card-file consistency (index-file-mismatch), card path well-formedness and the
// Moves: grammar/redundancy/mechanic checks (card-path-malformed, move-format,
// move-redundant, move-source-missing, move-target-collision,
// move-mechanic-missing), the per-card structural checks (card-missing-field,
// card-field-overlap), the card-numbering heading cross-check, the
// existence-dependent cross-referencing checks (path-missing,
// commit-subject-mismatch), and the depends-on-order gate. Unlike the frozen v2
// validator (internal/builderengine/validate.go), findings are keyed by card
// (flat `N-<slug>`), not batch — v3 has no batch concept — and there is no
// ValidateCaps: the oversized-batch cap dies with batch itself.
//
// This file is added across three cards (see docs/reference/plan-format-v3.md's
// worked spec): format/structure checks land first, then the card-path/Moves
// grammar checks, then the existence-dependent and depends-on checks — each
// addition also extends Validate's call sequence in place, in the spec's fixed
// numbering, so every intermediate commit still compiles and runs a strict
// subset of the final 14 checks.

package planparser

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// recognizedFormat is the only plan-format version Validate currently
// understands; a plan declaring any other format: value fails the
// format-unrecognized check. v3 supersedes v2 for webster's own consumption
// (see plan-format-v3.md's coexistence note) — there is no dual-version support
// inside this package.
const recognizedFormat = 3

// ValidationError is one finding from Validate: which check tripped (Check, a
// stable kebab-case name matching plan-format-v3.md's "Validation checks" list),
// which card it concerns (Card, empty for a plan-level finding — v3's flat card
// list has no wider batch unit to key on instead), and a human-readable Detail
// naming the specific problem.
type ValidationError struct {
	Check  string
	Card   string
	Detail string
}

// Error implements the error interface so a ValidationError can be used
// anywhere a single error is expected (e.g. in a test's error-substring
// assertion), formatted as "check[/card]: detail".
func (v ValidationError) Error() string {
	if v.Card == "" {
		return fmt.Sprintf("%s: %s", v.Check, v.Detail)
	}
	return fmt.Sprintf("%s/%s: %s", v.Check, v.Card, v.Detail)
}

// cardID returns the stable "N-<slug>" identifier Validate uses to name a card
// in a ValidationError, mirroring the frozen v2 validator's batchID shape but
// keyed on the card's own flat number since batches no longer exist.
func cardID(c Card) string {
	return fmt.Sprintf("%d-%s", c.Number, c.Slug)
}

// Validate runs every plan-format v3 machine check against plan and returns
// every finding, in the fixed order docs/reference/plan-format-v3.md's
// "Validation checks" section pins. worktreeRoot is the sole base the
// existence-dependent checks (move-source-missing, move-target-collision,
// path-missing) resolve a card path against to test on-disk presence; every
// other check operates purely on the parsed Plan model (or, for
// index-file-mismatch and card-numbering, on plan.Dir — the plan directory
// ParsePlan itself already read, not the caller-supplied worktreeRoot). A
// nil/empty return means the plan passes every check.
func Validate(plan *Plan, worktreeRoot string) []ValidationError {
	var findings []ValidationError

	findings = append(findings, checkFormatAndApproval(plan)...)
	findings = append(findings, checkIndexFileConsistency(plan)...)
	findings = append(findings, checkCardPathMalformed(plan)...)
	findings = append(findings, checkMoveFormat(plan)...)
	findings = append(findings, checkMoveRedundant(plan)...)
	findings = append(findings, checkMoveSourceMissing(plan, worktreeRoot)...)
	findings = append(findings, checkMoveTargetCollision(plan, worktreeRoot)...)
	findings = append(findings, checkMoveMechanicMissing(plan)...)
	findings = append(findings, checkCardMissingField(plan)...)
	findings = append(findings, checkCardFieldOverlap(plan)...)
	findings = append(findings, checkCardNumbering(plan)...)
	findings = append(findings, checkPathMissing(plan, worktreeRoot)...)
	findings = append(findings, checkCommitSubjectMismatch(plan)...)
	findings = append(findings, checkDependsOnOrder(plan)...)

	return findings
}

// checkFormatAndApproval implements format-unrecognized/plan-unapproved: the
// overview's format: must be the one version this package understands, and
// approved: must be true — the two are checked together (as plan-format-v3.md's
// own numbering does) since both are prerequisites for treating the plan as
// runnable at all.
func checkFormatAndApproval(plan *Plan) []ValidationError {
	var findings []ValidationError

	if plan.Format != recognizedFormat {
		findings = append(findings, ValidationError{
			Check:  "format-unrecognized",
			Detail: fmt.Sprintf("format %d is not recognized; only format %d is known", plan.Format, recognizedFormat),
		})
	}
	if !plan.Approved {
		findings = append(findings, ValidationError{
			Check:  "plan-unapproved",
			Detail: "plan frontmatter approved: is not true",
		})
	}

	return findings
}

// checkIndexFileConsistency implements index-file-mismatch: every *.md file on
// disk in plan.Dir (other than the overview itself) must be named by some
// parsed card — an unreferenced file is silently orphaned work a Planner forgot
// to wire into the Card Index — and the Card Index's own card numbers must run
// 1..M with no gaps or duplicates. The reverse direction (an index entry naming
// a file that does not exist) is not checked here: ParsePlan already fails loud
// on that case (a missing card file is document structure, not a lenient
// card-level defect), so a successfully parsed Plan can never reach Validate
// with that half of the mismatch. This check absorbs v2's dropped
// card-count-mismatch — v3's Card Index has no separate "(C cards)" segment to
// cross-check; the index itself is the card list.
func checkIndexFileConsistency(plan *Plan) []ValidationError {
	var findings []ValidationError

	indexed := make(map[string]bool, len(plan.Cards))
	for _, c := range plan.Cards {
		indexed[cardFileName(c.Number, c.Slug)] = true
	}

	entries, err := os.ReadDir(plan.Dir)
	if err == nil {
		var onDisk []string
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") || e.Name() == overviewFileName {
				continue
			}
			if !indexed[e.Name()] {
				onDisk = append(onDisk, e.Name())
			}
		}
		sort.Strings(onDisk)
		for _, name := range onDisk {
			findings = append(findings, ValidationError{
				Check:  "index-file-mismatch",
				Detail: fmt.Sprintf("file %s exists in %s but is not referenced by the Card Index", name, plan.Dir),
			})
		}
	}

	for i, c := range plan.Cards {
		want := i + 1
		if c.Number != want {
			findings = append(findings, ValidationError{
				Check: "index-file-mismatch",
				Card:  cardID(c),
				Detail: fmt.Sprintf(
					"Card Index numbering has a gap or duplicate: expected card number %d at index position %d, got %d",
					want, i+1, c.Number,
				),
			})
		}
	}

	return findings
}

// cardPathMalformedReason reports why p is not a well-formed plan-format-v3
// card path, or "" when p is well-formed. p is already normalized (root:///
// resolution applied by normalizeCard at parse time), so a still-absolute
// path or a surviving ".." segment here is a genuine escape past the worktree
// root, not an artifact of an unresolved root:. p is treated as a POSIX-style
// path (plan files are authored with forward slashes) so the check behaves
// the same on every platform Validate runs on.
func cardPathMalformedReason(p string) string {
	if p == "" {
		return "empty entry"
	}

	posix := filepath.ToSlash(p)
	if strings.HasPrefix(posix, "/") {
		return "absolute path"
	}
	for _, seg := range strings.Split(posix, "/") {
		if seg == ".." {
			return `contains a ".." escape`
		}
	}
	// Reuse normalize.go's own cleanPosixPath rather than re-implementing
	// path.Clean's segment-collapsing rules a second time in this package.
	if cleaned := cleanPosixPath(posix); cleaned != posix {
		return fmt.Sprintf("not a clean path (cleans to %q)", cleaned)
	}

	return ""
}

// checkCardPathMalformed implements card-path-malformed (v2's scope-malformed,
// renamed because Scope is gone in v3): every card path — all four non-Moves
// typed fields and both sides of every Moves: pair — must be non-empty,
// relative, clean, and free of ".." escapes, once normalized.
func checkCardPathMalformed(plan *Plan) []ValidationError {
	var findings []ValidationError

	for _, c := range plan.Cards {
		for _, fields := range [][]string{c.ContextFiles, c.EditsFiles, c.CreatesFiles, c.DeletesFiles} {
			for _, p := range fields {
				if reason := cardPathMalformedReason(p); reason != "" {
					findings = append(findings, ValidationError{
						Check:  "card-path-malformed",
						Card:   cardID(c),
						Detail: fmt.Sprintf("card %d path %q is malformed: %s", c.Number, p, reason),
					})
				}
			}
		}
		for _, mv := range c.Moves {
			for _, p := range []string{mv.Old, mv.New} {
				if reason := cardPathMalformedReason(p); reason != "" {
					findings = append(findings, ValidationError{
						Check:  "card-path-malformed",
						Card:   cardID(c),
						Detail: fmt.Sprintf("card %d Moves: endpoint %q is malformed: %s", c.Number, p, reason),
					})
				}
			}
		}
	}

	return findings
}

// checkMoveFormat implements move-format: every card's non-well-formed
// "Moves:" sub-bullet (retained verbatim in Card.MovesRaw by the
// lenient-card-parse parser) yields one finding, quoting the raw bullet and
// naming the offending card so a Planner can find and fix it.
func checkMoveFormat(plan *Plan) []ValidationError {
	var findings []ValidationError

	for _, c := range plan.Cards {
		for _, raw := range c.MovesRaw {
			findings = append(findings, ValidationError{
				Check: "move-format",
				Card:  cardID(c),
				Detail: fmt.Sprintf(
					"card %d Moves: entry %q does not match the required `src` -> `dst` grammar",
					c.Number, raw,
				),
			})
		}
	}

	return findings
}

// checkMoveRedundant implements move-redundant: a plan-wide check (v3 has no
// batch to scope it to) — a path that is both a Moves: endpoint (either side
// of any card's pair) and named in the whole plan's Creates:/Deletes: (across
// any card) is a conflicting instruction: the Planner must pick one
// mechanism, not both. Findings are sorted by path for determinism.
func checkMoveRedundant(plan *Plan) []ValidationError {
	var findings []ValidationError

	endpoints := make(map[string]bool)
	createsDeletes := make(map[string]bool)
	for _, c := range plan.Cards {
		for _, mv := range c.Moves {
			endpoints[mv.Old] = true
			endpoints[mv.New] = true
		}
		for _, p := range c.CreatesFiles {
			createsDeletes[p] = true
		}
		for _, p := range c.DeletesFiles {
			createsDeletes[p] = true
		}
	}

	var conflicts []string
	for p := range endpoints {
		if createsDeletes[p] {
			conflicts = append(conflicts, p)
		}
	}
	sort.Strings(conflicts)

	for _, p := range conflicts {
		findings = append(findings, ValidationError{
			Check: "move-redundant",
			Detail: fmt.Sprintf(
				"%q is both a Moves: endpoint and in Creates:/Deletes: somewhere in the plan; use Moves: or Creates:/Deletes:, not both",
				p,
			),
		})
	}

	return findings
}

// createsUnion returns the union, across every card in plan, of every
// CreatesFiles entry: plan-wide suppression semantics — order-independent, so
// a Moves: source or target satisfied by ANY card's Creates: (earlier or
// later in the Card Index) is not flagged. Consulted by checkMoveSourceMissing
// and checkPathMissing.
func createsUnion(plan *Plan) map[string]bool {
	union := make(map[string]bool)
	for _, c := range plan.Cards {
		for _, p := range c.CreatesFiles {
			union[p] = true
		}
	}
	return union
}

// movesTargetsUnion returns the union, across every card in plan, of every
// MovePair.New: the second plan-wide suppression set, letting a chained
// rename (card A: X -> Y, card B: Y -> Z) pass move-source-missing regardless
// of Card Index order.
func movesTargetsUnion(plan *Plan) map[string]bool {
	union := make(map[string]bool)
	for _, c := range plan.Cards {
		for _, mv := range c.Moves {
			union[mv.New] = true
		}
	}
	return union
}

// pathExistsOnDisk reports whether worktreeRoot-joined p exists on disk —
// shared by every existence-dependent check (move-source-missing,
// move-target-collision, path-missing). These three checks are the sole
// place Validate's worktreeRoot parameter is consulted, matching the frozen
// v2 validator's design.
func pathExistsOnDisk(worktreeRoot, p string) bool {
	_, err := os.Stat(filepath.Join(worktreeRoot, p))
	return err == nil
}

// checkMoveSourceMissing implements move-source-missing: a Moves: source that
// neither exists on disk nor is created or relocated by another card
// (plan-wide createsUnion/movesTargetsUnion suppression) is a dangling
// rename instruction.
func checkMoveSourceMissing(plan *Plan, worktreeRoot string) []ValidationError {
	var findings []ValidationError

	creates := createsUnion(plan)
	movesTargets := movesTargetsUnion(plan)

	for _, c := range plan.Cards {
		for _, mv := range c.Moves {
			if pathExistsOnDisk(worktreeRoot, mv.Old) {
				continue
			}
			if creates[mv.Old] || movesTargets[mv.Old] {
				continue
			}
			findings = append(findings, ValidationError{
				Check: "move-source-missing",
				Card:  cardID(c),
				Detail: fmt.Sprintf(
					"Moves: source %q does not exist on disk and is not a Creates: target or Moves: destination of any card",
					mv.Old,
				),
			})
		}
	}

	return findings
}

// checkMoveTargetCollision implements move-target-collision: three OR'd
// conditions per Moves: target, first match wins per occurrence: the target
// already exists on disk; more than one card names it as a Moves: target; or
// it collides with a DIFFERENT card's Creates: entry (same-card overlap is
// card-field-overlap's job, so it is deliberately skipped here).
func checkMoveTargetCollision(plan *Plan, worktreeRoot string) []ValidationError {
	var findings []ValidationError

	// targetCards counts, per target path, the distinct cards that name it
	// as a Moves: target — a size over 1 is condition (2).
	targetCards := make(map[string]map[string]bool)
	// targetCreatesCards records, per target path, the distinct cards whose
	// Creates: field names it — used by condition (3) to detect a DIFFERENT
	// card's Creates: collision.
	targetCreatesCards := make(map[string]map[string]bool)
	for _, c := range plan.Cards {
		id := cardID(c)
		for _, mv := range c.Moves {
			if targetCards[mv.New] == nil {
				targetCards[mv.New] = make(map[string]bool)
			}
			targetCards[mv.New][id] = true
		}
		for _, p := range c.CreatesFiles {
			if targetCreatesCards[p] == nil {
				targetCreatesCards[p] = make(map[string]bool)
			}
			targetCreatesCards[p][id] = true
		}
	}

	for _, c := range plan.Cards {
		id := cardID(c)
		for _, mv := range c.Moves {
			target := mv.New

			var detail string
			switch {
			case pathExistsOnDisk(worktreeRoot, target):
				detail = fmt.Sprintf("Moves: target %q already exists on disk", target)
			case len(targetCards[target]) > 1:
				detail = fmt.Sprintf("Moves: target %q is targeted by more than one card", target)
			default:
				for owner := range targetCreatesCards[target] {
					if owner != id {
						detail = fmt.Sprintf("Moves: target %q collides with another card's Creates: entry", target)
						break
					}
				}
			}

			if detail != "" {
				findings = append(findings, ValidationError{
					Check:  "move-target-collision",
					Card:   id,
					Detail: detail,
				})
			}
		}
	}

	return findings
}

// checkMoveMechanicMissing implements move-mechanic-missing: now a plan-level
// check (v2's was per-batch) — a plan with at least one parsed Moves: pair
// (across any card) but an empty Plan.RenameMechanic (the overview's "##
// Rename mechanic" section batch 1's sections.go already extracts) is missing
// the mechanical instruction that pins how the rename must be carried out. A
// plan whose every Moves: field is "none" (zero pairs) is skipped — a
// MovesRaw-only defect is move-format's finding, and requiring the section
// too would double-report the same underlying mistake.
func checkMoveMechanicMissing(plan *Plan) []ValidationError {
	var findings []ValidationError

	hasPair := false
	for _, c := range plan.Cards {
		if len(c.Moves) > 0 {
			hasPair = true
			break
		}
	}
	if hasPair && plan.RenameMechanic == "" {
		findings = append(findings, ValidationError{
			Check:  "move-mechanic-missing",
			Detail: `plan declares at least one Moves: pair but has no "## Rename mechanic" section`,
		})
	}

	return findings
}

// cardFieldLabel pairs a card field's Has-presence bool with the bold label
// plan-format-v3.md pins for it, in field order — checkCardMissingField's only
// data shape, kept next to the check so the field order stays visibly tied to
// the check that walks it.
type cardFieldLabel struct {
	present bool
	label   string
}

// checkCardMissingField implements card-missing-field: every card must carry
// all seven of What:/Context:/Edits:/Creates:/Deletes:/Moves:/Depends-on: — a
// missing label yields one finding per absent field, Detail naming the card and
// the missing label. Commit: and verify: are optional and never flagged. v3
// adds Depends-on: to the six fields v2 required.
func checkCardMissingField(plan *Plan) []ValidationError {
	var findings []ValidationError

	for _, c := range plan.Cards {
		fields := []cardFieldLabel{
			{c.HasWhat, "What:"},
			{c.HasContext, "Context:"},
			{c.HasEdits, "Edits:"},
			{c.HasCreates, "Creates:"},
			{c.HasDeletes, "Deletes:"},
			{c.HasMoves, "Moves:"},
			{c.HasDependsOn, "Depends-on:"},
		}
		for _, f := range fields {
			if f.present {
				continue
			}
			findings = append(findings, ValidationError{
				Check:  "card-missing-field",
				Card:   cardID(c),
				Detail: fmt.Sprintf("card %d is missing its %s field", c.Number, f.label),
			})
		}
	}

	return findings
}

// checkCardFieldOverlap implements card-field-overlap: within a single card, a
// path appearing in more than one of ContextFiles/EditsFiles/CreatesFiles/
// DeletesFiles, or as either side of a Moves: pair, is a conflicting
// instruction — one finding per duplicated path, Detail naming the card and
// every field the path appears in. Overlap is deliberately per-card only, per
// plan-format-v3.md: the same path in one card's Creates: and a later card's
// Edits: is legitimate cross-card sequencing, not a defect.
func checkCardFieldOverlap(plan *Plan) []ValidationError {
	var findings []ValidationError

	for _, c := range plan.Cards {
		fieldsOf := make(map[string][]string)
		add := func(p, field string) {
			for _, seen := range fieldsOf[p] {
				if seen == field {
					return
				}
			}
			fieldsOf[p] = append(fieldsOf[p], field)
		}

		for _, p := range c.ContextFiles {
			add(p, "Context:")
		}
		for _, p := range c.EditsFiles {
			add(p, "Edits:")
		}
		for _, p := range c.CreatesFiles {
			add(p, "Creates:")
		}
		for _, p := range c.DeletesFiles {
			add(p, "Deletes:")
		}
		for _, mv := range c.Moves {
			add(mv.Old, "Moves:")
			add(mv.New, "Moves:")
		}

		var duplicated []string
		for p, fields := range fieldsOf {
			if len(fields) > 1 {
				duplicated = append(duplicated, p)
			}
		}
		sort.Strings(duplicated)

		for _, p := range duplicated {
			findings = append(findings, ValidationError{
				Check: "card-field-overlap",
				Card:  cardID(c),
				Detail: fmt.Sprintf(
					"card %d path %q appears in more than one field: %s",
					c.Number, p, strings.Join(fieldsOf[p], ", "),
				),
			})
		}
	}

	return findings
}

// checkCardNumbering implements card-numbering: a card file's own "# Card N —
// <name>" heading number must equal the number the Card Index assigned it.
// Unlike v2 (where this check also verified a per-batch 1..M card sequence),
// v3's cards ARE the Card Index — that sequencing is index-file-mismatch's job
// now (checkIndexFileConsistency above), since there is no narrower per-batch
// unit left to sequence separately. This check's own job — the heading-vs-index
// cross-check — is genuinely unique work: ParsePlan deliberately discards the
// heading's own captured number once the title text is extracted (see
// parseCardFile's comment), so this check re-reads the card file directly
// against plan.Dir (not the caller-supplied worktreeRoot) to recover it.
func checkCardNumbering(plan *Plan) []ValidationError {
	var findings []ValidationError

	for _, c := range plan.Cards {
		headingNumber, ok := cardHeadingNumber(plan.Dir, c)
		if !ok {
			// The card file was unreadable or its heading unrecognized — both
			// are document-structure failures ParsePlan would already have
			// caught fail-loud for a plan that reached Validate at all, so
			// this is unreachable in practice; skip rather than panic.
			continue
		}
		if headingNumber != c.Number {
			findings = append(findings, ValidationError{
				Check: "card-numbering",
				Card:  cardID(c),
				Detail: fmt.Sprintf(
					"card file %s heading declares card number %d, but the Card Index assigns it number %d",
					cardFileName(c.Number, c.Slug), headingNumber, c.Number,
				),
			})
		}
	}

	return findings
}

// cardHeadingNumber re-reads c's own card file under planDir and extracts the
// number its "# Card N — <name>" title heading carries, reusing parse.go's own
// cardFileName/cardHeadingRe (same package, so no edit to parse.go is needed)
// rather than duplicating that grammar. Returns ok == false when the file
// cannot be read or its first line does not match the heading grammar.
func cardHeadingNumber(planDir string, c Card) (int, bool) {
	path := filepath.Join(planDir, cardFileName(c.Number, c.Slug))
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, false
	}

	firstLine := strings.SplitN(string(data), "\n", 2)[0]
	m := cardHeadingRe.FindStringSubmatch(strings.TrimSpace(firstLine))
	if m == nil {
		return 0, false
	}

	n, err := strconv.Atoi(m[1])
	if err != nil {
		return 0, false
	}
	return n, true
}

// checkPathMissing implements path-missing: every card path in ContextFiles,
// EditsFiles, and DeletesFiles must exist on disk under worktreeRoot, unless
// it is satisfied by some card's Creates: or some Moves: pair's target (the
// same plan-wide createsUnion/movesTargetsUnion suppression sets
// move-source-missing consults). CreatesFiles is deliberately excluded —
// those paths are new by definition — and a card's own Moves: sources are
// move-source-missing's job, not this check's.
func checkPathMissing(plan *Plan, worktreeRoot string) []ValidationError {
	var findings []ValidationError

	creates := createsUnion(plan)
	movesTargets := movesTargetsUnion(plan)

	for _, c := range plan.Cards {
		for _, fields := range [][]string{c.ContextFiles, c.EditsFiles, c.DeletesFiles} {
			for _, p := range fields {
				if pathExistsOnDisk(worktreeRoot, p) {
					continue
				}
				if creates[p] || movesTargets[p] {
					continue
				}
				findings = append(findings, ValidationError{
					Check: "path-missing",
					Card:  cardID(c),
					Detail: fmt.Sprintf(
						"card %d path %q does not exist on disk and is not a Creates: target or Moves: destination of any card",
						c.Number, p,
					),
				})
			}
		}
	}

	return findings
}

// checkCommitSubjectMismatch implements commit-subject-mismatch: a card's
// non-empty Commit value must start with the exact "N: " prefix its own flat
// card number pins — v3's numbering-and-commit-subject discipline, the
// resume-trail invariant a pinned message that breaks the "N:" shape would
// corrupt.
func checkCommitSubjectMismatch(plan *Plan) []ValidationError {
	var findings []ValidationError

	for _, c := range plan.Cards {
		if c.Commit == "" {
			continue
		}
		prefix := fmt.Sprintf("%d: ", c.Number)
		if !strings.HasPrefix(c.Commit, prefix) {
			findings = append(findings, ValidationError{
				Check: "commit-subject-mismatch",
				Card:  cardID(c),
				Detail: fmt.Sprintf(
					"card %d Commit: %q does not start with the expected prefix %q",
					c.Number, c.Commit, prefix,
				),
			})
		}
	}

	return findings
}

// checkDependsOnOrder implements depends-on-order: a card's Depends-on: must
// name only cards strictly earlier in the Card Index — an id naming the
// card's own position, a later card, or a number that references no existing
// card at all is flagged before any LLM-based review runs, at zero LLM cost
// (plan-format-v3.md's "Depends-on" rationale).
func checkDependsOnOrder(plan *Plan) []ValidationError {
	var findings []ValidationError

	// positionOf maps each card's own Number to its index in plan.Cards, so
	// "at or after its own position" can be checked by comparing index
	// positions rather than raw card numbers (which need not be contiguous
	// once index-file-mismatch already has its own finding for that).
	positionOf := make(map[int]int, len(plan.Cards))
	for i, c := range plan.Cards {
		positionOf[c.Number] = i
	}

	for i, c := range plan.Cards {
		for _, dep := range c.DependsOn {
			pos, ok := positionOf[dep]
			switch {
			case !ok:
				findings = append(findings, ValidationError{
					Check: "depends-on-order",
					Card:  cardID(c),
					Detail: fmt.Sprintf(
						"card %d Depends-on: %d references a card number that does not exist",
						c.Number, dep,
					),
				})
			case pos >= i:
				findings = append(findings, ValidationError{
					Check: "depends-on-order",
					Card:  cardID(c),
					Detail: fmt.Sprintf(
						"card %d Depends-on: %d names a card at or after its own position",
						c.Number, dep,
					),
				})
			}
		}
	}

	return findings
}
