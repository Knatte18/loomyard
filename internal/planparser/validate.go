// validate.go implements ValidateFormat and Validate, format-4 plan-format's machine check sets
// (manifest/designs/plan-card-format.md), run in this fixed order.
// ValidateFormat emits fifteen of the following distinct ValidationError.Check IDs, everything but
// plan-unapproved; Validate emits all sixteen: format-unrecognized (checkFormatRecognized),
// plan-unapproved (checkApproved), index-file-mismatch (checkIndexFileConsistency), card-type-missing
// (checkCardTypeMissing), card-retired-label (checkCardRetiredLabel), card-path-malformed
// (checkCardPathMalformed), rename-format (checkRenameFormat), rename-mechanic-missing
// (checkRenameMechanicMissing), card-missing-field (checkCardMissingField), card-field-empty
// (checkCardFieldEmpty), card-field-overlap (checkCardFieldOverlap), impact-summary-multiline
// (checkImpactSummaryMultiline), prosa-symbol-target (checkProsaSymbolTarget), card-numbering
// (checkCardNumbering), path-missing (checkPathMissing), and commit-subject-mismatch
// (checkCommitSubjectMismatch).
// Findings are keyed by card (flat `N-<slug>`), not batch: the format has no batch concept,
// and there is no ValidateCaps because there is no oversized-batch cap to configure.
// No scheduler, dependency graph, or topological sort belongs in this file — the dependency graph
// and topological order live in internal/websterengine's sequence.go, which derives them from the
// Targets/Uses refs this package parses, because scheduling is the executor's job and parsing is
// this package's, per the Planparser Sole-Parser Invariant.

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
const recognizedFormat = 4

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

// Validate runs every plan-format machine check against plan, including the plan-unapproved
// approval gate, and returns every finding in fixed order: all sixteen check IDs documented in
// this file's package comment, with plan-unapproved at position two.
func Validate(plan *Plan, worktreeRoot string) []ValidationError {
	return validate(plan, worktreeRoot, true)
}

// ValidateFormat runs every plan-format machine check against plan except the plan-unapproved
// approval gate, and returns every finding in fixed order: fifteen of the sixteen check IDs
// documented in this file's package comment, everything but plan-unapproved.
// Approval is deliberately not ValidateFormat's business: the approved: flag is written after the
// review segment settles, so a pre-review caller must not be told the plan is unapproved.
func ValidateFormat(plan *Plan, worktreeRoot string) []ValidationError {
	return validate(plan, worktreeRoot, false)
}

// validate is the shared dispatch list behind Validate and ValidateFormat.
// requireApproved selects whether checkApproved's plan-unapproved finding is included; it is an
// ordering detail of that split, not a second exported seam.
func validate(plan *Plan, worktreeRoot string, requireApproved bool) []ValidationError {
	var findings []ValidationError

	findings = append(findings, checkFormatRecognized(plan)...)
	if requireApproved {
		findings = append(findings, checkApproved(plan)...)
	}
	findings = append(findings, checkIndexFileConsistency(plan)...)
	findings = append(findings, checkCardTypeMissing(plan)...)
	findings = append(findings, checkCardRetiredLabel(plan)...)
	findings = append(findings, checkCardPathMalformed(plan)...)
	findings = append(findings, checkRenameFormat(plan)...)
	findings = append(findings, checkRenameMechanicMissing(plan)...)
	findings = append(findings, checkCardMissingField(plan)...)
	findings = append(findings, checkCardFieldEmpty(plan)...)
	findings = append(findings, checkCardFieldOverlap(plan)...)
	findings = append(findings, checkImpactSummaryMultiline(plan)...)
	findings = append(findings, checkProsaSymbolTarget(plan)...)
	findings = append(findings, checkCardNumbering(plan)...)
	findings = append(findings, checkPathMissing(plan, worktreeRoot)...)
	findings = append(findings, checkCommitSubjectMismatch(plan)...)

	return findings
}

// checkFormatRecognized implements format-unrecognized: plan.Format must equal recognizedFormat.
func checkFormatRecognized(plan *Plan) []ValidationError {
	var findings []ValidationError

	if plan.Format != recognizedFormat {
		findings = append(findings, ValidationError{
			Check:  "format-unrecognized",
			Detail: fmt.Sprintf("format %d is not recognized; only format %d is known", plan.Format, recognizedFormat),
		})
	}

	return findings
}

// checkApproved implements plan-unapproved: plan.Approved must be true.
func checkApproved(plan *Plan) []ValidationError {
	var findings []ValidationError

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

// checkCardTypeMissing implements card-type-missing: every card must carry exactly one recognized type label.
func checkCardTypeMissing(plan *Plan) []ValidationError {
	var findings []ValidationError

	for _, c := range plan.Cards {
		switch {
		case c.TypeLabelCount == 0:
			findings = append(findings, ValidationError{
				Check:  "card-type-missing",
				Card:   cardID(c),
				Detail: fmt.Sprintf("card %d carries no recognized type label (Create/Edit/Delete/Rename/Move/Prosa/Custom)", c.Number),
			})
		case c.TypeLabelCount > 1:
			findings = append(findings, ValidationError{
				Check:  "card-type-missing",
				Card:   cardID(c),
				Detail: fmt.Sprintf("card %d carries %d type labels; exactly one is required", c.Number, c.TypeLabelCount),
			})
		}
	}

	return findings
}

// retiredLabelMapping names, for each format-3 label, how format-4 replaces it.
var retiredLabelMapping = map[string]string{
	whatLabel:         "became **Intent:**",
	contextLabel:      "became **Uses:**",
	editsLabel:        "was absorbed into the card's own type-label target list",
	createsLabel:      "was absorbed into the card's own type-label target list",
	deletesLabel:      "was absorbed into the card's own type-label target list",
	movesLabel:        "was absorbed into the card's own type-label target list",
	dependsOnLabel:    "was dropped because dependency edges are derived rather than authored",
	legacyVerifyLabel: "became **Verify:**",
}

// checkCardRetiredLabel implements card-retired-label: every retired format-3 label occurrence on a card is a finding.
func checkCardRetiredLabel(plan *Plan) []ValidationError {
	var findings []ValidationError

	for _, c := range plan.Cards {
		for _, label := range c.RetiredLabels {
			findings = append(findings, ValidationError{
				Check:  "card-retired-label",
				Card:   cardID(c),
				Detail: fmt.Sprintf("card %d carries the retired label %s; it %s", c.Number, label, retiredLabelMapping[label]),
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

// checkCardPathMalformed implements card-path-malformed: every path-shaped Targets/Uses entry must be non-empty, relative, clean, and free of ".." escapes. Symbol-shaped entries are skipped, and Pairs is not iterated separately because both endpoints of every pair are already projected into Targets.
func checkCardPathMalformed(plan *Plan) []ValidationError {
	var findings []ValidationError

	for _, c := range plan.Cards {
		for _, fields := range [][]string{c.Targets, c.Uses} {
			for _, p := range fields {
				if !isPathRef(p) {
					continue
				}
				if reason := cardPathMalformedReason(p); reason != "" {
					findings = append(findings, ValidationError{
						Check:  "card-path-malformed",
						Card:   cardID(c),
						Detail: fmt.Sprintf("card %d path %q is malformed: %s", c.Number, p, reason),
					})
				}
			}
		}
	}

	return findings
}

// checkRenameFormat implements rename-format: every card's non-well-formed "Rename:" sub-bullet yields one finding.
func checkRenameFormat(plan *Plan) []ValidationError {
	var findings []ValidationError

	for _, c := range plan.Cards {
		for _, raw := range c.RenameRaw {
			findings = append(findings, ValidationError{
				Check: "rename-format",
				Card:  cardID(c),
				Detail: fmt.Sprintf(
					"card %d Rename: entry %q does not match the required `old` -> `new` grammar",
					c.Number, raw,
				),
			})
		}
	}

	return findings
}

// checkRenameMechanicMissing implements rename-mechanic-missing: a plan with at least one Rename card but an empty RenameMechanic section.
func checkRenameMechanicMissing(plan *Plan) []ValidationError {
	var findings []ValidationError

	hasRename := false
	for _, c := range plan.Cards {
		if c.Type == CardTypeRename {
			hasRename = true
			break
		}
	}
	if hasRename && plan.RenameMechanic == "" {
		findings = append(findings, ValidationError{
			Check:  "rename-mechanic-missing",
			Detail: `plan has at least one Rename card but no "## Rename mechanic" section`,
		})
	}

	return findings
}

// cardFieldLabel pairs a card field's Has-presence bool with its bold label.
type cardFieldLabel struct {
	present bool
	label   string
}

// checkCardMissingField implements card-missing-field: every card must carry Intent:, and a card of type Edit or Delete must also carry ImpactSummary:.
func checkCardMissingField(plan *Plan) []ValidationError {
	var findings []ValidationError

	for _, c := range plan.Cards {
		fields := []cardFieldLabel{
			{c.HasIntent, "Intent:"},
		}
		if c.Type == CardTypeEdit || c.Type == CardTypeDelete {
			fields = append(fields, cardFieldLabel{c.HasImpactSummary, "ImpactSummary:"})
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

// checkCardFieldEmpty implements card-field-empty: a label present with no content is distinct from an absent label.
func checkCardFieldEmpty(plan *Plan) []ValidationError {
	var findings []ValidationError

	for _, c := range plan.Cards {
		if c.HasType && len(c.Targets) == 0 {
			findings = append(findings, ValidationError{
				Check:  "card-field-empty",
				Card:   cardID(c),
				Detail: fmt.Sprintf("card %d's type label carries no targets", c.Number),
			})
		}
		if c.HasUses && len(c.Uses) == 0 {
			findings = append(findings, ValidationError{
				Check:  "card-field-empty",
				Card:   cardID(c),
				Detail: fmt.Sprintf("card %d's Uses: field carries no entries", c.Number),
			})
		}
		if c.HasIntent && c.Intent == "" {
			findings = append(findings, ValidationError{
				Check:  "card-field-empty",
				Card:   cardID(c),
				Detail: fmt.Sprintf("card %d's Intent: field carries no prose", c.Number),
			})
		}
		if c.HasImpactSummary && c.ImpactSummary == "" {
			findings = append(findings, ValidationError{
				Check:  "card-field-empty",
				Card:   cardID(c),
				Detail: fmt.Sprintf("card %d's ImpactSummary: field carries no value", c.Number),
			})
		}
	}

	return findings
}

// checkCardFieldOverlap implements card-field-overlap: an entry appearing in both a card's Targets and its own Uses.
func checkCardFieldOverlap(plan *Plan) []ValidationError {
	var findings []ValidationError

	for _, c := range plan.Cards {
		targets := make(map[string]bool, len(c.Targets))
		for _, t := range c.Targets {
			targets[t] = true
		}

		var duplicated []string
		seen := make(map[string]bool)
		for _, u := range c.Uses {
			if targets[u] && !seen[u] {
				duplicated = append(duplicated, u)
				seen[u] = true
			}
		}
		sort.Strings(duplicated)

		for _, p := range duplicated {
			findings = append(findings, ValidationError{
				Check: "card-field-overlap",
				Card:  cardID(c),
				Detail: fmt.Sprintf(
					"card %d entry %q appears in both its own target list and its Uses: field",
					c.Number, p,
				),
			})
		}
	}

	return findings
}

// checkImpactSummaryMultiline implements impact-summary-multiline: an ImpactSummary: field followed by trailing lines is a defect, since ImpactSummary is required to stay a single line.
func checkImpactSummaryMultiline(plan *Plan) []ValidationError {
	var findings []ValidationError

	for _, c := range plan.Cards {
		if len(c.ImpactSummaryTrailing) == 0 {
			continue
		}
		findings = append(findings, ValidationError{
			Check: "impact-summary-multiline",
			Card:  cardID(c),
			Detail: fmt.Sprintf(
				"card %d's ImpactSummary: field carries %d trailing line(s); ImpactSummary must stay a single line",
				c.Number, len(c.ImpactSummaryTrailing),
			),
		})
	}

	return findings
}

// checkProsaSymbolTarget implements prosa-symbol-target: a Prosa card's target list must hold only file(s), never a symbol.
func checkProsaSymbolTarget(plan *Plan) []ValidationError {
	var findings []ValidationError

	for _, c := range plan.Cards {
		if c.Type != CardTypeProsa {
			continue
		}
		for _, t := range c.Targets {
			if isPathRef(t) {
				continue
			}
			findings = append(findings, ValidationError{
				Check: "prosa-symbol-target",
				Card:  cardID(c),
				Detail: fmt.Sprintf(
					"card %d is a Prosa card but targets the symbol %q; Prosa cards may only target files",
					c.Number, t,
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

// pathExistsOnDisk reports whether worktreeRoot-joined p exists on disk.
func pathExistsOnDisk(worktreeRoot, p string) bool {
	_, err := os.Stat(filepath.Join(worktreeRoot, p))
	return err == nil
}

// createTargetsUnion returns the union, across every card in plan, of every CardTypeCreate card's path-shaped Targets entries.
func createTargetsUnion(plan *Plan) map[string]bool {
	union := make(map[string]bool)
	for _, c := range plan.Cards {
		if c.Type != CardTypeCreate {
			continue
		}
		for _, t := range c.Targets {
			if isPathRef(t) {
				union[t] = true
			}
		}
	}
	return union
}

// renameTargetsUnion returns the union, across every card in plan, of every Pairs entry's New side that is path-shaped.
func renameTargetsUnion(plan *Plan) map[string]bool {
	union := make(map[string]bool)
	for _, c := range plan.Cards {
		for _, p := range c.Pairs {
			if isPathRef(p.New) {
				union[p.New] = true
			}
		}
	}
	return union
}

// checkPathMissing implements path-missing: type-conditional, existence-dependent path checking.
// Every card's path-shaped Uses entries are checked, including a Custom card's; this is a
// card-level check, run once per card, not once per group. Within each card, its own
// TargetGroups are then walked one at a time: a group's path-shaped Refs are checked only when
// its own Type is Edit, Delete, Move, or Prosa. A Rename group's path-shaped Pairs.Old entries
// are checked, read from that group's own Pairs, and its Refs are skipped entirely (so a
// Rename's New side is never checked). Create and Custom groups' Refs are skipped. A path
// otherwise reported missing is satisfied by existing on disk, by createTargetsUnion membership,
// or by renameTargetsUnion membership.
func checkPathMissing(plan *Plan, worktreeRoot string) []ValidationError {
	var findings []ValidationError

	creates := createTargetsUnion(plan)
	renames := renameTargetsUnion(plan)

	satisfied := func(p string) bool {
		return pathExistsOnDisk(worktreeRoot, p) || creates[p] || renames[p]
	}

	report := func(c Card, p string) {
		findings = append(findings, ValidationError{
			Check: "path-missing",
			Card:  cardID(c),
			Detail: fmt.Sprintf(
				"card %d path %q does not exist on disk and is not a Create target or Rename destination of any card",
				c.Number, p,
			),
		})
	}

	for _, c := range plan.Cards {
		for _, u := range c.Uses {
			if !isPathRef(u) || satisfied(u) {
				continue
			}
			report(c, u)
		}

		for _, g := range c.TargetGroups {
			switch g.Type {
			case CardTypeEdit, CardTypeDelete, CardTypeMove, CardTypeProsa:
				for _, t := range g.Refs {
					if !isPathRef(t) || satisfied(t) {
						continue
					}
					report(c, t)
				}
			case CardTypeRename:
				for _, p := range g.Pairs {
					if !isPathRef(p.Old) || satisfied(p.Old) {
						continue
					}
					report(c, p.Old)
				}
			case CardTypeCreate, CardTypeCustom:
				// Create's refs are new by definition, and Custom is an explicit escape hatch
				// exempt from path-missing on its own refs.
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
