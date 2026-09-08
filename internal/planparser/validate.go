// validate.go implements ValidateFormat and Validate, format-5 plan-format's machine check sets
// (manifest/designs/plan-card-format.md), run in this fixed order.
// ValidateFormat emits twenty-seven of the following distinct ValidationError.Check IDs, everything
// but plan-unapproved; Validate emits all twenty-eight: format-unrecognized (checkFormatRecognized),
// plan-language-unrecognized (checkLanguageRecognized), plan-unapproved (checkApproved),
// index-file-mismatch (checkIndexFileConsistency), card-type-missing (checkCardTypeMissing),
// card-custom-not-alone (checkCustomNotAlone), card-retired-label (checkCardRetiredLabel),
// card-path-malformed (checkCardPathMalformed), bare-symbol-target (checkBareSymbolTarget),
// directory-target (checkDirectoryTarget), glyph-malformed (checkGlyphMalformed),
// rename-format (checkRenameFormat), handle-dangling,
// handle-collision, handle-unreferenced (all three checkHandleConsistency), handle-malformed
// (checkHandleMalformed), rename-to-not-handle, rename-from-not-glyph (both
// checkRenamePairShape), rename-mechanic-missing (checkRenameMechanicMissing),
// card-missing-field (checkCardMissingField), card-field-empty (checkCardFieldEmpty),
// card-field-overlap (checkCardFieldOverlap), containment-unit-overlap
// (syntacticContainment, containment.go), impact-summary-multiline
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
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/Knatte18/quarry/glyph"
)

// recognizedFormat is the only plan-format version Validate currently understands.
const recognizedFormat = 5

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
// approval gate, and returns every finding in fixed order: all twenty-eight check IDs documented
// in this file's package comment, with plan-unapproved at position three.
func Validate(plan *Plan, worktreeRoot string) []ValidationError {
	return validate(plan, worktreeRoot, true)
}

// ValidateFormat runs every plan-format machine check against plan except the plan-unapproved
// approval gate, and returns every finding in fixed order: twenty-seven of the twenty-eight check
// IDs documented in this file's package comment, everything but plan-unapproved.
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
	findings = append(findings, checkLanguageRecognized(plan)...)
	if requireApproved {
		findings = append(findings, checkApproved(plan)...)
	}
	findings = append(findings, checkIndexFileConsistency(plan)...)
	findings = append(findings, checkCardTypeMissing(plan)...)
	findings = append(findings, checkCustomNotAlone(plan)...)
	findings = append(findings, checkCardRetiredLabel(plan)...)
	findings = append(findings, checkCardPathMalformed(plan)...)
	findings = append(findings, checkBareSymbolTarget(plan)...)
	findings = append(findings, checkDirectoryTarget(plan)...)
	findings = append(findings, checkGlyphMalformed(plan)...)
	findings = append(findings, checkRenameFormat(plan)...)
	findings = append(findings, checkHandleConsistency(plan)...)
	findings = append(findings, checkHandleMalformed(plan)...)
	findings = append(findings, checkRenamePairShape(plan)...)
	findings = append(findings, checkRenameMechanicMissing(plan)...)
	findings = append(findings, checkCardMissingField(plan)...)
	findings = append(findings, checkCardFieldEmpty(plan)...)
	findings = append(findings, checkCardFieldOverlap(plan)...)
	if lang, ok := planLanguage(plan); ok {
		findings = append(findings, syntacticContainment(plan, lang)...)
	}
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

// checkLanguageRecognized implements plan-language-unrecognized: plan.Language must be "go" or
// "none". "" (the zero value, meaning a hand-built *Plan carries no explicit language: at all) is
// treated identically to "go", matching Plan.Language's own documented "absent defaults to go"
// rule — ParsePlan itself never actually produces "", always writing "go" explicitly when the
// frontmatter key is absent. This is a pure string check: it calls no Resolve, stats no disk, and
// keeps the package tier1-safe.
func checkLanguageRecognized(plan *Plan) []ValidationError {
	var findings []ValidationError

	switch plan.Language {
	case "", "go", "none":
		return findings
	}

	findings = append(findings, ValidationError{
		Check:  "plan-language-unrecognized",
		Detail: fmt.Sprintf(`language %q is not recognized; only "go" and "none" are known`, plan.Language),
	})

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

// knownNonCardFiles is the explicit allowlist of on-disk plan-directory filenames
// checkIndexFileConsistency's scan never treats as an orphaned card file, because each one is a
// plan-level file with its own dedicated owner: overviewFileName (ParsePlan/SetApproved) and
// AmendmentsFileName (AppendAmendment). Named as an allowlist entry per file, not a heuristic over
// the filename's shape, so a future non-card file only needs one line added here.
var knownNonCardFiles = map[string]bool{
	overviewFileName:   true,
	AmendmentsFileName: true,
}

// checkIndexFileConsistency implements index-file-mismatch: every *.md file on disk must be named by some parsed card, and card numbers must run 1..M with no gaps or duplicates.
func checkIndexFileConsistency(plan *Plan) []ValidationError {
	var findings []ValidationError

	indexed := make(map[string]bool, len(plan.Cards))
	for _, c := range plan.Cards {
		indexed[cardFileName(c.Number, c.Slug)] = true
	}

	// An empty Dir is "no plan directory was told", not a fault: ParsePlan always sets Dir, so this is
	// the in-memory plan shape tests build. There is nothing on disk to scan and nothing to report.
	// The guard runs BEFORE the ReadDir it guards — reading first and testing afterwards issued a
	// guaranteed-failing ReadDir("") for every such plan and read as if the empty-Dir case were an
	// error branch, which is the opposite of what it means.
	var entries []os.DirEntry
	var err error
	if plan.Dir != "" {
		entries, err = os.ReadDir(plan.Dir)
	}
	switch {
	case plan.Dir == "":
	case err != nil:
		// A plan directory that cannot be listed is a finding, not silence. Swallowing the error
		// disabled the whole orphaned-card-file half of this check with nothing reported anywhere —
		// a permission fault, or a plan directory that stopped resolving mid-run, made the check
		// report CLEAN against its own unconditional guarantee (crucible round opus-medium-r6,
		// R6-10). The numbering half below still runs, so the failure was invisible.
		findings = append(findings, ValidationError{
			Check:  "index-file-mismatch",
			Detail: fmt.Sprintf("plan directory %s cannot be listed (%v), so no file on disk could be checked against the Card Index", plan.Dir, err),
		})
	default:
		var onDisk []string
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") || knownNonCardFiles[e.Name()] {
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

// checkCardTypeMissing implements card-type-missing: every card must carry at least one recognized
// type label. Carrying more than one, or repeating the same label, is legal.
func checkCardTypeMissing(plan *Plan) []ValidationError {
	var findings []ValidationError

	for _, c := range plan.Cards {
		if c.TypeLabelCount == 0 {
			findings = append(findings, ValidationError{
				Check:  "card-type-missing",
				Card:   cardID(c),
				Detail: fmt.Sprintf("card %d carries no recognized type label (Create/Edit/Delete/Rename/Move/Prosa/Custom)", c.Number),
			})
		}
	}

	return findings
}

// checkCustomNotAlone implements card-custom-not-alone: a card carrying a Custom TargetGroup
// alongside a TargetGroup whose Type differs from CardTypeCustom is a defect, because Custom is
// meant as a last-resort escape hatch, not a way to bundle an untyped target list onto an
// otherwise ordinary card. Repetition of a label is legal for all seven labels including Custom,
// so the predicate is "a Custom group coexists with a group whose Type differs", not "a Custom
// group coexists with any other group" — a card carrying two Custom groups and nothing else is
// unaffected. At most one finding is emitted per offending card, never one per offending group.
func checkCustomNotAlone(plan *Plan) []ValidationError {
	var findings []ValidationError

	for _, c := range plan.Cards {
		hasCustom := false
		hasOther := false
		for _, g := range c.TargetGroups {
			if g.Type == CardTypeCustom {
				hasCustom = true
			} else {
				hasOther = true
			}
		}
		if hasCustom && hasOther {
			findings = append(findings, ValidationError{
				Check:  "card-custom-not-alone",
				Card:   cardID(c),
				Detail: fmt.Sprintf("card %d carries a Custom group alongside a differently-typed group; Custom must stand alone", c.Number),
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

// diskPathForRef maps raw to the disk-relative path checkCardPathMalformed and checkPathMissing
// (and their two union builders) key on: a path-shaped raw maps to itself; a glyph-shaped raw maps
// through parseGlyph followed by Glyph.UnitPath() -- the one glyph->path call, per the
// glyph-conversion-chokepoint Shared Decision -- but only for a self glyph (IsSelf() true). A
// member glyph names a symbol within its unit, not the unit itself, so mapping it to its unit's
// directory and existence-checking that directory would answer a question these two checks were
// never meant to answer (member existence is internal/planglyph's resolve-backed business); a
// member glyph is therefore skipped (ok false) exactly like a not-ok UnitPath or a failed
// parseGlyph, rather than reported. Any other shape (a plan: handle, a bare symbol) is also
// skipped. ok is also false whenever plan.Language reports not-ok (e.g. "none"): a glyph-shaped
// entry is never resolved when the plan has opted out of the alphabet.
func diskPathForRef(plan *Plan, raw string) (string, bool) {
	switch classifyRef(raw) {
	case refKindPath:
		return raw, true
	case refKindGlyph:
		lang, ok := planLanguage(plan)
		if !ok {
			return "", false
		}
		g, err := parseGlyph(lang, raw)
		if err != nil || !g.IsSelf() {
			return "", false
		}
		return g.UnitPath()
	default:
		return "", false
	}
}

// checkCardPathMalformed implements card-path-malformed: every path- or self-glyph-shaped
// Targets/Uses entry must map (via diskPathForRef) to a disk path that is non-empty, relative,
// clean, and free of ".." escapes. Symbol-, handle-, and member-glyph-shaped entries are skipped,
// and Pairs is not iterated separately because both endpoints of every pair are already projected
// into Targets.
func checkCardPathMalformed(plan *Plan) []ValidationError {
	var findings []ValidationError

	for _, c := range plan.Cards {
		for _, fields := range [][]string{c.Targets, c.Uses} {
			for _, p := range fields {
				mapped, ok := diskPathForRef(plan, p)
				if !ok {
					continue
				}
				if reason := cardPathMalformedReason(mapped); reason != "" {
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

// checkBareSymbolTarget implements bare-symbol-target: any Targets/Uses entry classifying as
// refKindSymbol is a hard finding, because a bare package-qualified symbol is the one spelling that
// cannot have come verbatim from a quarry answer -- not because the form is uglier. Skipped
// entirely when plan.Language does not enable the glyph alphabet, where a symbol-shaped ref keeps its
// pre-glyph behavior. Gated on planLanguage, not on the literal "none", so an UNRECOGNIZED language
// silences this check too: ParsePlan performed no canonicalization for such a plan, so the refs this
// check classifies are raw paths rather than the canonical forms it assumes, and every other
// alphabet-gated check already goes quiet there (crucible round opus-medium-r6, R6-22).
// plan-language-unrecognized already blocks such a plan, so nothing is lost by staying silent.
func checkBareSymbolTarget(plan *Plan) []ValidationError {
	var findings []ValidationError

	if _, ok := planLanguage(plan); !ok {
		return findings
	}

	for _, c := range plan.Cards {
		for _, fields := range [][]string{c.Targets, c.Uses} {
			for _, t := range fields {
				if classifyRef(t) != refKindSymbol {
					continue
				}
				findings = append(findings, ValidationError{
					Check: "bare-symbol-target",
					Card:  cardID(c),
					Detail: fmt.Sprintf(
						"card %d entry %q is a bare package-qualified symbol, which cannot have come verbatim from a quarry answer; spell it as a glyph (unit#member) instead",
						c.Number, t,
					),
				})
			}
		}
	}

	return findings
}

// checkDirectoryTarget implements directory-target: a refKindPath entry that contains a "/" and
// carries no file extension names a directory rather than a file, and should be spelled as a unit
// glyph if it is a package, or its files listed instead if it is not code. The "/" condition is
// what keeps the extensionless repository-root filenames classify.go's rule 4 admits (e.g.
// "Makefile") out of this finding; a slash-free extensionless directory at the repository root is
// therefore not caught here -- it falls to path-missing and, on a Prosa group, to
// prosa-symbol-target, narrower coverage than the slashed case and accepted rather than papered
// over. An extensionless FILE under a non-"." root: does land here (root:-joining makes it slashed,
// and no lexical rule can tell it from a directory), so the finding's detail also names the file
// self glyph as the remedy for that case -- appending "#" is the legal spelling either way
// (crucible round fable-high-r10, F7). Skipped entirely when plan.Language does not enable the
// glyph alphabet -- gated on planLanguage rather than the literal "none", for the reason
// checkBareSymbolTarget states.
func checkDirectoryTarget(plan *Plan) []ValidationError {
	var findings []ValidationError

	if _, ok := planLanguage(plan); !ok {
		return findings
	}

	for _, c := range plan.Cards {
		for _, fields := range [][]string{c.Targets, c.Uses} {
			for _, t := range fields {
				if classifyRef(t) != refKindPath {
					continue
				}
				if !strings.Contains(t, "/") || hasFileExtension(t) {
					continue
				}
				findings = append(findings, ValidationError{
					Check: "directory-target",
					Card:  cardID(c),
					Detail: fmt.Sprintf(
						"card %d entry %q names a directory with no file extension; spell it as its self glyph %q — the unit glyph if it is a package, the file self glyph if it is an extensionless file — or list the files instead if it is not code",
						c.Number, t, t+"#",
					),
				})
			}
		}
	}

	return findings
}

// checkGlyphMalformed implements glyph-malformed: a refKindGlyph entry (classified on shape alone,
// by classifyRef rule 2 -- any "#"-containing entry, regardless of whether it actually parses) that
// fails glyph.Parse is a hard finding. Card-generic over Targets and Uses, exactly like
// checkBareSymbolTarget and checkDirectoryTarget -- including a Prosa group's own targets, which
// prosa-symbol-target ALSO separately flags as "not a self glyph" for the same malformed entry; the
// two checks answering the same defect from two angles (shape-invalid vs not-a-self-glyph) mirrors
// how bare-symbol-target and prosa-symbol-target already both fire on a Prosa group's bare-symbol
// target today. Skipped entirely when plan.Language does not enable the glyph alphabet, for the same
// reason checkBareSymbolTarget is.
//
// Without this check a malformed-but-"#"-shaped entry (a doubled "#", an empty unit, a member
// carrying a paren or a keyword) is invisible end to end outside a Prosa group: classifyRef sends it
// to refKindGlyph on shape alone and never calls glyph.Parse itself (by design -- see classify.go's
// own doc comment), bare-symbol-target/directory-target skip it (wrong shape),
// card-path-malformed/path-missing skip it (diskPathForRef returns not-ok on a parse error),
// containment-unit-overlap skips it the same way, and internal/planglyph's collectGlyphTargets
// silently drops it before it ever enters the batched Resolve call -- so it never even reaches a
// glyph-not-found/glyph-ambiguous/glyph-rejected verdict either. The plan would validate 100% clean
// while carrying a target no execution engine can ever act on, discovered only deep into a batch's
// own done-check, not at Plan-Validate up front where every other malformed-entry class is caught
// (crucible round sonnet-xhigh-r8, PG-1).
func checkGlyphMalformed(plan *Plan) []ValidationError {
	var findings []ValidationError

	lang, ok := planLanguage(plan)
	if !ok {
		return findings
	}

	for _, c := range plan.Cards {
		for _, fields := range [][]string{c.Targets, c.Uses} {
			for _, t := range fields {
				if classifyRef(t) != refKindGlyph {
					continue
				}
				if _, err := parseGlyph(lang, t); err != nil {
					findings = append(findings, ValidationError{
						Check: "glyph-malformed",
						Card:  cardID(c),
						Detail: fmt.Sprintf(
							"card %d entry %q looks like a glyph (contains \"#\") but fails to parse: %v",
							c.Number, t, err,
						),
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

// checkHandleConsistency implements handle-dangling, handle-collision, and handle-unreferenced,
// all pure string work over the parsed model via handleClaims/declaredHandles/referencedHandles
// (handle.go).
// These checks run under every plan.Language, including "none": a handle is loomyard grammar, not
// glyph grammar, and its consistency is checkable without any alphabet.
//
// handle-dangling and handle-collision both key on handleClaims, the union of the format's two
// handle-declaring sources; handle-unreferenced keys on declaredHandles alone, and deliberately so
// — a Rename card's destination that no OTHER card references is the ordinary case, not a defect,
// so folding Rename to-sides into that half would fire a false finding on essentially every Rename
// card in every plan.
func checkHandleConsistency(plan *Plan) []ValidationError {
	var findings []ValidationError

	claims := handleClaims(plan)
	declared := declaredHandles(plan)
	referenced := referencedHandles(plan)

	// handle-dangling: a referenced handle with no matching Create declaration and no matching
	// Rename to-side.
	handles := make([]string, 0, len(referenced))
	for h := range referenced {
		handles = append(handles, h)
	}
	sort.Strings(handles)
	for _, handle := range handles {
		if len(claims[handle]) > 0 {
			continue
		}
		for _, cid := range referenced[handle] {
			findings = append(findings, ValidationError{
				Check: "handle-dangling",
				Card:  cid,
				Detail: fmt.Sprintf(
					"handle %q is referenced with no matching Create declaration and no matching Rename to-side",
					handle,
				),
			})
		}
	}

	// handle-collision: the same handle claimed more than once across the plan — by two Create
	// sub-bullets, by two Rename to-sides, or by one of each — one finding per colliding handle
	// rather than one per claiming card.
	claimedHandleNames := make([]string, 0, len(claims))
	for h := range claims {
		claimedHandleNames = append(claimedHandleNames, h)
	}
	sort.Strings(claimedHandleNames)
	for _, handle := range claimedHandleNames {
		handleCards := claims[handle]
		if len(handleCards) <= 1 {
			continue
		}
		cards := make([]string, 0, len(handleCards))
		for _, cl := range handleCards {
			cards = append(cards, cl.card)
		}
		findings = append(findings, ValidationError{
			Check: "handle-collision",
			Detail: fmt.Sprintf(
				"handle %q is claimed by more than one Create sub-bullet or Rename to-side, on cards %s",
				handle, strings.Join(cards, ", "),
			),
		})
	}

	// handle-unreferenced: a declared handle no card other than its own declaring card(s)
	// references. A declaring card's own Create bullet contributes the handle to its own Targets
	// too, so that self-reference must not count.
	declaredHandleNames := make([]string, 0, len(declared))
	for h := range declared {
		declaredHandleNames = append(declaredHandleNames, h)
	}
	sort.Strings(declaredHandleNames)
	for _, handle := range declaredHandleNames {
		decCards := declared[handle]
		externallyReferenced := false
		for _, rc := range referenced[handle] {
			if !slices.Contains(decCards, rc) {
				externallyReferenced = true
				break
			}
		}
		if externallyReferenced {
			continue
		}
		for _, dc := range decCards {
			findings = append(findings, ValidationError{
				Check:  "handle-unreferenced",
				Card:   dc,
				Detail: fmt.Sprintf("card declares handle %q that no other card references", handle),
			})
		}
	}

	return findings
}

// checkHandleMalformed implements handle-malformed: every entry of a card's CreateRaw (a
// "**Create:**" arrow bullet that failed the two-field declaration grammar), every handle-shaped
// Targets/Uses entry whose text after HandlePrefix carries no "#" and therefore names no unit,
// and — under a glyph-enabled plan.Language only — every handle whose unit half names a ".go"
// FILE rather than a package directory. The first two rules run under every plan.Language, for
// the same reason checkHandleConsistency does; the file-unit rule is alphabet knowledge and so is
// language-gated.
//
// The file-unit rule exists because quarry's Name and Resolve disagree over that spelling: Name
// accepts a file unit and echoes `pkg/file.go#Symbol` as the canonical ID, but Resolve answers
// members under their PACKAGE unit only, so the canonicalized handle can never resolve — the plan
// validates clean (the Create inversion reads not_found as the expected pre-create answer) and
// the run then wedges at the creating card's own record-batch done-check, with no earlier
// diagnostic naming the actual mistake. Proven live in crucible round fable5-high-r3's standalone
// E2E (F-B8).
// That rationale binds a handle whose unit half is actually READ — a Create declaration's — and
// only that one, so the rule is additionally gated on fileUnitRuleApplies (handle.go).
func checkHandleMalformed(plan *Plan) []ValidationError {
	var findings []ValidationError

	claims := handleClaims(plan)
	_, langOK := planLanguage(plan)

	for _, c := range plan.Cards {
		for _, raw := range c.CreateRaw {
			findings = append(findings, ValidationError{
				Check: "handle-malformed",
				Card:  cardID(c),
				Detail: fmt.Sprintf(
					"card %d Create: entry %q does not match the required `plan:<handle>` -> `<declaration head>` grammar",
					c.Number, raw,
				),
			})
		}

		for _, fields := range [][]string{c.Targets, c.Uses} {
			for _, r := range fields {
				if classifyRef(r) != refKindHandle {
					continue
				}
				unit, ok := handleUnit(r)
				if !ok {
					findings = append(findings, ValidationError{
						Check: "handle-malformed",
						Card:  cardID(c),
						Detail: fmt.Sprintf(
							"card %d handle %q carries no \"#\" after %q and therefore names no unit",
							c.Number, r, HandlePrefix,
						),
					})
					continue
				}
				if langOK && strings.HasSuffix(unit, ".go") && fileUnitRuleApplies(claims[r]) {
					findings = append(findings, ValidationError{
						Check: "handle-malformed",
						Card:  cardID(c),
						Detail: fmt.Sprintf(
							"card %d handle %q names the file %q as its unit; a symbol's unit is its package directory (e.g. %q) — a file-unit member spelling can never resolve",
							c.Number, r, unit, filepath.ToSlash(filepath.Dir(unit)),
						),
					})
				}
			}
		}
	}

	return findings
}

// isFileRenamePair reports whether p is a file-rename pair under lang: both p.Old and p.New
// classify as refKindGlyph and both parse to a glyph whose IsSelf() reports true. A file-rename
// pair has no declaration head to name and is exempt from both of checkRenamePairShape's checks —
// it belongs in the same group as a plain path pair.
func isFileRenamePair(lang glyph.Language, p MovePair) bool {
	if classifyRef(p.Old) != refKindGlyph || classifyRef(p.New) != refKindGlyph {
		return false
	}
	oldGlyph, err := parseGlyph(lang, p.Old)
	if err != nil || !oldGlyph.IsSelf() {
		return false
	}
	newGlyph, err := parseGlyph(lang, p.New)
	if err != nil || !newGlyph.IsSelf() {
		return false
	}
	return true
}

// checkRenamePairShape implements rename-to-not-handle and rename-from-not-glyph. On a symbol
// rename the old side must be a glyph — it names something that exists and will be resolved — and
// the new side must be a plan: handle, whose content batch 4's binding step computes and overwrites
// at the validation boundary rather than trusting the planner's draft spelling. A Rename card
// therefore needs no declaration head of its own, unlike a Create card, because the declaration is
// derived from the resolved old side. isFileRenamePair exempts a file-rename pair from both checks.
// Neither check runs when planLanguage reports not-ok (e.g. plan.Language "none").
//
// Both checks are written as the NEGATION of the one shape the format admits, never as an
// enumeration of the shapes it forbids. classifyRef returns four kinds, and the enumerated form —
// "new side is a glyph or a bare symbol", "old side is a bare symbol or a plan: handle" — silently
// let the fourth kind, refKindPath, through BOTH halves: a pair such as
// `internal/a#Old` -> `LICENSE` drew no finding from either check, none from directory-target (no
// "/"), none from bare-symbol-target (wrong shape), and path-missing never checks a Rename pair's
// New side by design, so the pair was entirely unvalidated and surfaced only as a rename-not-done
// at the record-batch boundary (crucible round opus-high-r9, R9-2). Fail closed: anything that is
// not the admitted shape is the finding, and refKindName names what it actually was.
// One shape slips both negations — a SELF glyph old side paired with a handle new side, which is a
// glyph on the left and a handle on the right yet names a file/unit where a symbol rename must
// name a symbol — so a third arm flags exactly that pair under rename-from-not-glyph (crucible
// round fable-high-r10, F5).
func checkRenamePairShape(plan *Plan) []ValidationError {
	var findings []ValidationError

	lang, langOK := planLanguage(plan)
	if !langOK {
		return findings
	}

	for _, c := range plan.Cards {
		for _, p := range c.Pairs {
			if isFileRenamePair(lang, p) {
				continue
			}
			if k := classifyRef(p.New); k != refKindHandle {
				findings = append(findings, ValidationError{
					Check: "rename-to-not-handle",
					Card:  cardID(c),
					Detail: fmt.Sprintf(
						"card %d Rename pair %q -> %q has a new side that is %s, not a plan: handle",
						c.Number, p.Old, p.New, refKindName(k),
					),
				})
			}
			if k := classifyRef(p.Old); k != refKindGlyph {
				findings = append(findings, ValidationError{
					Check: "rename-from-not-glyph",
					Card:  cardID(c),
					Detail: fmt.Sprintf(
						"card %d Rename pair %q -> %q has an old side that is %s, not a glyph",
						c.Number, p.Old, p.New, refKindName(k),
					),
				})
			} else if classifyRef(p.New) == refKindHandle {
				// A symbol rename's old side must name a SYMBOL — a member glyph — because the
				// to-side declaration is derived from the resolved old symbol. A SELF glyph old side
				// paired with a handle new side is neither admitted shape (not a symbol rename, not
				// a file-rename pair), yet passed both negation checks above: the old side IS a
				// glyph and the new side IS a handle. It surfaced only inside the resolve pass,
				// where a found self glyph carries a Listing and no Symbols, so renameDeclSource
				// refused it with a detail claiming the old side "did not resolve found" — a
				// resolution story for what is a shape mistake (crucible round fable-high-r10, F5).
				if g, err := parseGlyph(lang, p.Old); err == nil && g.IsSelf() {
					findings = append(findings, ValidationError{
						Check: "rename-from-not-glyph",
						Card:  cardID(c),
						Detail: fmt.Sprintf(
							"card %d Rename pair %q -> %q has an old side that is a file or unit self glyph, not a member glyph naming a symbol; a symbol rename's old side must name the symbol being renamed",
							c.Number, p.Old, p.New,
						),
					})
				}
			}
		}
	}

	return findings
}

// refKindName names a refKind in the prose form checkRenamePairShape's own findings use, so a
// finding says what the offending side actually is rather than only what it failed to be.
func refKindName(k refKind) string {
	switch k {
	case refKindPath:
		return "a file path"
	case refKindSymbol:
		return "a bare symbol"
	case refKindGlyph:
		return "a glyph"
	case refKindHandle:
		return "a plan: handle"
	}
	// Unreachable while classifyRef returns only the four kinds above, and deliberately not a
	// panic: this package is lenient at card level, and a shape it cannot name is still a shape it
	// must report rather than crash the whole validation pass over.
	return "an unrecognized shape"
}

// checkRenameMechanicMissing implements rename-mechanic-missing: a plan with at least one Rename
// group but an empty RenameMechanic section. A Rename group on an otherwise multi-label card still
// counts — the requirement is keyed on the group's own presence, not the card's first-label Type.
func checkRenameMechanicMissing(plan *Plan) []ValidationError {
	var findings []ValidationError

	hasRename := false
	for _, c := range plan.Cards {
		for _, g := range c.TargetGroups {
			if g.Type == CardTypeRename {
				hasRename = true
			}
		}
	}
	if hasRename && plan.RenameMechanic == "" {
		findings = append(findings, ValidationError{
			Check:  "rename-mechanic-missing",
			Detail: `plan has at least one Rename group but no "## Rename mechanic" section`,
		})
	}

	return findings
}

// cardFieldLabel pairs a card field's Has-presence bool with its bold label.
type cardFieldLabel struct {
	present bool
	label   string
}

// checkCardMissingField implements card-missing-field: every card must carry Intent:, and a card
// carrying any Edit or Delete TargetGroup must also carry ImpactSummary: — the requirement is a
// union over the card's own groups, not the card's first-label Type.
func checkCardMissingField(plan *Plan) []ValidationError {
	var findings []ValidationError

	for _, c := range plan.Cards {
		fields := []cardFieldLabel{
			{c.HasIntent, "Intent:"},
		}
		needsImpactSummary := false
		for _, g := range c.TargetGroups {
			if g.Type == CardTypeEdit || g.Type == CardTypeDelete {
				needsImpactSummary = true
				break
			}
		}
		if needsImpactSummary {
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

// groupBoldLabel returns a TargetGroup's own type label in its card-body bold form, e.g. "**Edit:**".
func groupBoldLabel(typ CardType) string {
	return fmt.Sprintf("**%s:**", typ)
}

// checkCardFieldEmpty implements card-field-empty: a label present with no content is distinct
// from an absent label. Each of a card's own TargetGroups is checked independently, so a card
// carrying a populated group alongside an empty one still gets a finding for the empty group.
func checkCardFieldEmpty(plan *Plan) []ValidationError {
	var findings []ValidationError

	for _, c := range plan.Cards {
		for _, g := range c.TargetGroups {
			if len(g.Refs) > 0 {
				continue
			}
			findings = append(findings, ValidationError{
				Check:  "card-field-empty",
				Card:   cardID(c),
				Detail: fmt.Sprintf("card %d's %s label carries no targets", c.Number, groupBoldLabel(g.Type)),
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

// checkProsaSymbolTarget implements prosa-symbol-target: a Prosa group's own target list must
// hold only file(s)/package(s), never an individual symbol. A symbol in the same card's non-Prosa
// group is not flagged — the rule is scoped to the Prosa group's own Refs, not the card's flat
// Targets union.
// Under a glyph-enabled plan.Language, a ref passes when parseGlyph succeeds and the resulting
// glyph's IsSelf() reports true — admitting both the file self glyph and the unit self glyph, the
// latter required by the package-spelling rule so a Prosa card can target a whole package (e.g.
// "internal/foo#") — and is the finding when IsSelf() reports false (a member glyph, or anything
// that fails to parse as a glyph at all, including a plain path or a bare symbol). Under "none",
// the check keeps its pre-glyph behavior exactly: a path-shaped ref passes, a symbol-shaped ref is
// the finding.
func checkProsaSymbolTarget(plan *Plan) []ValidationError {
	var findings []ValidationError

	lang, langOK := planLanguage(plan)

	for _, c := range plan.Cards {
		for _, g := range c.TargetGroups {
			if g.Type != CardTypeProsa {
				continue
			}
			for _, t := range g.Refs {
				if langOK {
					if gl, err := parseGlyph(lang, t); err == nil && gl.IsSelf() {
						continue
					}
				} else if isPathRef(t) {
					continue
				}
				// The detail names what the entry FAILED to be rather than asserting it is a
				// symbol: under a glyph-enabled language the rejected shapes are a member glyph,
				// a plain path, and anything that does not parse as a glyph at all, and calling a
				// bare extensionless directory "the symbol" sent readers hunting for a symbol
				// that was never there.
				findings = append(findings, ValidationError{
					Check: "prosa-symbol-target",
					Card:  cardID(c),
					Detail: fmt.Sprintf(
						"card %d's Prosa group entry %q is not a file or whole-package self glyph; a Prosa group may only target files or whole packages",
						c.Number, t,
					),
				})
			}
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

// createTargetsUnion returns the union, across every card in plan, of the mapped disk path (via
// diskPathForRef) of every CardTypeCreate TargetGroup's own path- or self-glyph-shaped Refs entries
// — a card carrying a Create group alongside a differently-typed group contributes only the Create
// group's own refs, never its other groups'.
func createTargetsUnion(plan *Plan) map[string]bool {
	union := make(map[string]bool)
	for _, c := range plan.Cards {
		for _, g := range c.TargetGroups {
			if g.Type != CardTypeCreate {
				continue
			}
			for _, t := range g.Refs {
				if mapped, ok := diskPathForRef(plan, t); ok {
					union[mapped] = true
				}
			}
		}
	}
	return union
}

// renameTargetsUnion returns the union, across every card in plan, of the mapped disk path (via
// diskPathForRef) of every Pairs entry's New side that is path- or self-glyph-shaped.
func renameTargetsUnion(plan *Plan) map[string]bool {
	union := make(map[string]bool)
	for _, c := range plan.Cards {
		for _, p := range c.Pairs {
			if mapped, ok := diskPathForRef(plan, p.New); ok {
				union[mapped] = true
			}
		}
	}
	return union
}

// checkPathMissing implements path-missing: type-conditional, existence-dependent path checking.
// Every card's path- or self-glyph-shaped Uses entries are checked, including a Custom card's;
// this is a card-level check, run once per card, not once per group. Within each card, its own
// TargetGroups are then walked one at a time: a group's path- or self-glyph-shaped Refs are
// checked only when its own Type is Edit, Delete, Move, or Prosa. A Rename group's path- or
// self-glyph-shaped Pairs.Old entries are checked, read from that group's own Pairs, and its Refs
// are skipped entirely (so a Rename's New side is never checked). Create and Custom groups' Refs
// are skipped. A path otherwise reported missing is satisfied by existing on disk, by
// createTargetsUnion membership, or by renameTargetsUnion membership. A ref diskPathForRef cannot
// map (a member glyph, a plan: handle, a bare symbol, or any glyph under a not-ok plan.Language) is
// skipped rather than reported by every one of these paths.
func checkPathMissing(plan *Plan, worktreeRoot string) []ValidationError {
	var findings []ValidationError

	creates := createTargetsUnion(plan)
	renames := renameTargetsUnion(plan)

	satisfied := func(p string) bool {
		return pathExistsOnDisk(worktreeRoot, p) || creates[p] || renames[p]
	}

	report := func(c Card, raw string) {
		findings = append(findings, ValidationError{
			Check: "path-missing",
			Card:  cardID(c),
			Detail: fmt.Sprintf(
				"card %d path %q does not exist on disk and is not a Create target or Rename destination of any card",
				c.Number, raw,
			),
		})
	}

	checkRef := func(c Card, raw string) {
		mapped, ok := diskPathForRef(plan, raw)
		if !ok || satisfied(mapped) {
			return
		}
		report(c, raw)
	}

	for _, c := range plan.Cards {
		for _, u := range c.Uses {
			checkRef(c, u)
		}

		for _, g := range c.TargetGroups {
			switch g.Type {
			case CardTypeEdit, CardTypeDelete, CardTypeMove, CardTypeProsa:
				for _, t := range g.Refs {
					checkRef(c, t)
				}
			case CardTypeRename:
				for _, p := range g.Pairs {
					checkRef(c, p.Old)
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
