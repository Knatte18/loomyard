// validate.go implements Validate, plan-format's complete machine check set
// (contracts/specs/loom-plan-spec.md, "Validation checks"), run in this fixed order: format/approval
// (format-unrecognized, plan-unapproved), Card Index <-> card-file consistency
// (index-file-mismatch), card path well-formedness and the Moves: grammar/redundancy/mechanic
// checks (card-path-malformed, move-format, move-redundant, move-source-missing,
// move-target-collision, move-mechanic-missing), the per-card structural checks
// (card-missing-field, card-field-overlap), the card-numbering heading cross-check, the
// existence-dependent cross-referencing checks (path-missing, commit-subject-mismatch), and the
// depends-on-order gate.
// Findings are keyed by card (flat `N-<slug>`), not batch: the format has no batch concept,
// and there is no ValidateCaps because there is no oversized-batch cap to configure.
//
// This file is added across three cards (see contracts/specs/loom-plan-spec.md's worked spec):
// format/structure checks land first, then the card-path/Moves grammar checks, then the
// existence-dependent and depends-on checks — each addition also extends Validate's call sequence
// in place, in the spec's fixed numbering, so every intermediate commit still compiles and runs a
// strict subset of the final 14 checks.

package planparser

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// recognizedFormat is the only plan-format version Validate currently understands.
const recognizedFormat = 3

// ValidationError is one finding from Validate: which check tripped, which card it concerns, and a
// human-readable detail.
type ValidationError struct {
	Check  string
	Card   string
	Detail string
}

// Error implements the error interface, formatted as "check[/card]: detail".
func (v ValidationError) Error() string {
	if v.Card == "" {
		return fmt.Sprintf("%s: %s", v.Check, v.Detail)
	}
	return fmt.Sprintf("%s/%s: %s", v.Check, v.Card, v.Detail)
}

// cardID returns the stable "N-<slug>" identifier Validate uses to name a card.
func cardID(c Card) string {
	return fmt.Sprintf("%d-%s", c.Number, c.Slug)
}

// Validate runs every plan-format machine check against plan and returns every finding in fixed
// order.
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

// checkFormatAndApproval implements format-unrecognized/plan-unapproved checks.
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

// checkIndexFileConsistency implements index-file-mismatch: every *.md file on disk must be named by some parsed card, and card numbers must run 1..M with no gaps or duplicates.
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

// cardPathMalformedReason reports why p is not a well-formed plan-format card path, or "" when well-formed.
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
	if cleaned := cleanPosixPath(posix); cleaned != posix {
		return fmt.Sprintf("not a clean path (cleans to %q)", cleaned)
	}

	return ""
}

// checkCardPathMalformed implements card-path-malformed: every card path must be non-empty, relative, clean, and free of ".." escapes.
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

// checkMoveFormat implements move-format: every card's non-well-formed "Moves:" sub-bullet yields one finding.
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

// checkMoveRedundant implements move-redundant: a path that is both a Moves: endpoint and in Creates:/Deletes: is a conflicting instruction.
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

// createsUnion returns the union, across every card in plan, of every CreatesFiles entry.
func createsUnion(plan *Plan) map[string]bool {
	union := make(map[string]bool)
	for _, c := range plan.Cards {
		for _, p := range c.CreatesFiles {
			union[p] = true
		}
	}
	return union
}

// movesTargetsUnion returns the union, across every card in plan, of every MovePair.New.
func movesTargetsUnion(plan *Plan) map[string]bool {
	union := make(map[string]bool)
	for _, c := range plan.Cards {
		for _, mv := range c.Moves {
			union[mv.New] = true
		}
	}
	return union
}

// pathExistsOnDisk reports whether worktreeRoot-joined p exists on disk.
func pathExistsOnDisk(worktreeRoot, p string) bool {
	_, err := os.Stat(filepath.Join(worktreeRoot, p))
	return err == nil
}

// checkMoveSourceMissing implements move-source-missing: a Moves: source that doesn't exist on disk and isn't created/relocated by another card.
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

// checkMoveTargetCollision implements move-target-collision: a target that already exists, is targeted by multiple cards, or collides with a different card's Creates:.
func checkMoveTargetCollision(plan *Plan, worktreeRoot string) []ValidationError {
	var findings []ValidationError

	targetCards := make(map[string]map[string]bool)
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

// checkMoveMechanicMissing implements move-mechanic-missing: a plan with at least one Moves: pair but an empty RenameMechanic section.
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

// cardFieldLabel pairs a card field's Has-presence bool with its bold label.
type cardFieldLabel struct {
	present bool
	label   string
}

// checkCardMissingField implements card-missing-field: every card must carry all seven required fields.
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

// checkCardFieldOverlap implements card-field-overlap: a path appearing in more than one field within a single card.
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

// checkCardNumbering implements card-numbering: a card file's heading number must equal the Card Index number.
func checkCardNumbering(plan *Plan) []ValidationError {
	var findings []ValidationError

	for _, c := range plan.Cards {
		headingNumber, ok := cardHeadingNumber(plan.Dir, c)
		if !ok {
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

// cardHeadingNumber re-reads c's own card file and extracts its heading number.
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

// checkPathMissing implements path-missing: every card path in ContextFiles, EditsFiles, and DeletesFiles must exist on disk or be satisfied by Creates: or Moves: targets.
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

// checkCommitSubjectMismatch implements commit-subject-mismatch: a card's Commit value must start with the "N: " prefix.
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

// checkDependsOnOrder implements depends-on-order: a card's Depends-on: must name only cards strictly earlier in the Card Index.
func checkDependsOnOrder(plan *Plan) []ValidationError {
	var findings []ValidationError

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
