// validate.go implements Validate and ValidateFormat, which run the plan-format machine checks in a fixed order.
// The checks are specified in contracts/specs/loom-plan-checks-spec.md.
// Scheduling is not this file's job: the dependency graph lives in internal/websterengine/sequence.go.

package planparser

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/Knatte18/quarry/glyph"
)

// RecognizedFormat is the only plan-format version this package understands.
const RecognizedFormat = 5

// ValidationError is one finding of the plan-format checks:
// the check that tripped, the card it concerns, and a human-readable detail.
// Ref is the raw plan ref the finding reports, set by the checks that report one ref per finding and empty otherwise.
type ValidationError struct {
	Check  string
	Card   string
	Detail string
	Ref    string
}

// Error implements the error interface, formatted as "check[/card]: detail".
func (v ValidationError) Error() string {
	if v.Card == "" {
		return fmt.Sprintf("%s: %s", v.Check, v.Detail)
	}
	return fmt.Sprintf("%s/%s: %s", v.Check, v.Card, v.Detail)
}

// cardID returns the stable "N-<slug>" identifier that names a card in a finding.
// It is the form other packages consume for the same card.
func cardID(c Card) string {
	return fmt.Sprintf("%d-%s", c.Number, c.Slug)
}

// Validate runs every plan-format machine check against plan, including the plan-unapproved approval gate.
// It returns every finding in a fixed order.
func Validate(plan *Plan, worktreeRoot string) []ValidationError {
	return validate(plan, worktreeRoot, true)
}

// ValidateFormat runs every plan-format machine check against plan except the plan-unapproved approval gate.
// It returns every finding in a fixed order.
// Approval is not ValidateFormat's business: the approved: flag is written after the review segment settles,
// so a pre-review caller must not be told the plan is unapproved.
func ValidateFormat(plan *Plan, worktreeRoot string) []ValidationError {
	return validate(plan, worktreeRoot, false)
}

// validate runs the plan-format checks in a fixed order and returns every finding.
// requireApproved selects whether the plan-unapproved finding from `checkApproved` is included.
// It is a detail of how the two entry points split the list, not a second exported seam.
func validate(plan *Plan, worktreeRoot string, requireApproved bool) []ValidationError {
	var findings []ValidationError

	findings = append(findings, checkFormatRecognized(plan)...)
	findings = append(findings, checkLanguageRecognized(plan)...)
	findings = append(findings, checkLanguageMismatch(plan)...)
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
	findings = append(findings, checkResignNotMember(plan)...)
	findings = append(findings, checkRenamePairShape(plan)...)
	findings = append(findings, checkRenameMechanicMissing(plan)...)
	findings = append(findings, checkRenameLeavesTest(plan)...)
	findings = append(findings, checkCardMissingField(plan)...)
	findings = append(findings, checkCardFieldEmpty(plan)...)
	findings = append(findings, checkCardFieldOverlap(plan)...)
	findings = append(findings, checkExtendsOverlap(plan)...)
	findings = append(findings, checkExtendsResign(plan)...)
	findings = append(findings, checkUsesLaterTarget(plan)...)
	findings = append(findings, checkRedundantPackageTarget(plan)...)
	findings = append(findings, checkImpactSummaryMultiline(plan)...)
	findings = append(findings, checkProsaSymbolTarget(plan)...)
	findings = append(findings, checkCardNumbering(plan)...)
	findings = append(findings, checkPathMissing(plan, worktreeRoot)...)
	if plan.Part != nil {
		findings = append(findings, checkPartSpotTarget(plan)...)
	}
	findings = append(findings, checkRenameNamespacePair(plan, worktreeRoot)...)
	findings = append(findings, checkCommitSubjectMismatch(plan)...)
	findings = append(findings, checkVerifyNestedModule(plan, worktreeRoot)...)
	findings = append(findings, checkVerifyModuleWide(plan)...)
	findings = append(findings, checkVerifyPackageMissing(plan, worktreeRoot)...)
	findings = append(findings, checkPatternEntryLineCap(plan)...)

	return findings
}

// checkFormatRecognized implements format-unrecognized: plan.Format must equal RecognizedFormat.
func checkFormatRecognized(plan *Plan) []ValidationError {
	var findings []ValidationError

	if plan.Format != RecognizedFormat {
		findings = append(findings, ValidationError{
			Check:  "format-unrecognized",
			Detail: fmt.Sprintf("format %d is not recognized; only format %d is known", plan.Format, RecognizedFormat),
		})
	}

	return findings
}

// checkLanguageRecognized implements plan-language-unrecognized: plan.Language must be "go",
// "csharp" or "none".
// An empty language, the zero value of a hand-built *Plan, is treated as "go".
// The parser always writes "go" explicitly when the frontmatter key is absent.
// So only a hand-built plan carries an empty language.
// This is a pure string check: it calls no Resolve, stats no disk, and keeps the package tier1-safe.
func checkLanguageRecognized(plan *Plan) []ValidationError {
	var findings []ValidationError

	switch plan.Language {
	case "", "go", "csharp", "none":
		return findings
	}

	findings = append(findings, ValidationError{
		Check:  "plan-language-unrecognized",
		Detail: fmt.Sprintf(`language %q is not recognized; only "go", "csharp" and "none" are known`, plan.Language),
	})

	return findings
}

// checkLanguageMismatch implements the pure half of plan-language-mismatch:
// the plan's language must fit the extensions of the files its cards' Targets name.
// Targets are read as paths.
// Under go it fires when they name a .cs file and no .go file.
// It stays silent when a Target is a member glyph, a `plan:` handle or a self glyph with no file extension.
// Such a ref may name .go code, which the resolve-backed half of the check judges.
// Under csharp it fires when they name a .go file and no .cs file.
// It stays silent when a Target is a member glyph or a `plan:` handle, since under csharp such a ref names C# code.
// It never fires under none or an unrecognized language, and reads no disk.
// The one finding is plan-level, and its way forward is to set `language:` to the files' language or to none.
func checkLanguageMismatch(plan *Plan) []ValidationError {
	lang, ok := planLanguage(plan)
	if !ok {
		return nil
	}

	var goFiles, csharpFiles int
	for _, c := range plan.Cards {
		for _, target := range c.Targets {
			if IsHandleRef(target) {
				return nil
			}
			parsed, err := ParseRef(plan, target)
			if err != nil {
				continue
			}
			if !parsed.IsSelf() {
				return nil
			}
			file, isPath := PathEntryPath(target)
			if !isPath {
				continue
			}
			switch path.Ext(file) {
			case ".go":
				goFiles++
			case ".cs":
				csharpFiles++
			case "":
				if lang == glyph.Go {
					return nil
				}
			}
		}
	}

	var filesLanguage, wantLanguage string
	switch {
	case lang == glyph.Go && csharpFiles > 0 && goFiles == 0:
		filesLanguage, wantLanguage = "C# (.cs)", "csharp"
	case lang == glyph.CSharp && goFiles > 0 && csharpFiles == 0:
		filesLanguage, wantLanguage = "Go (.go)", "go"
	default:
		return nil
	}

	return []ValidationError{{
		Check: "plan-language-mismatch",
		Detail: fmt.Sprintf(
			"the plan's language is %q but the files its cards target are %s only; set `language:` to %s, the language of the files, or to `language: none` for a plan that changes both",
			plan.Language, filesLanguage, wantLanguage,
		),
	}}
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

// knownNonCardFiles lists the plan-directory filenames that the `index-file-mismatch` check never reports.
// Each one is a plan-level file with its own dedicated owner, not a card.
// Lens report files are skipped by name pattern instead, since each name carries its lens.
var knownNonCardFiles = map[string]bool{
	overviewFileName:   true,
	AmendmentsFileName: true,
	LensReviewFileName: true,
}

// checkIndexFileConsistency implements index-file-mismatch.
// Every *.md file on disk must be named by a parsed card, except the plan-level and lens report files it exempts.
// Card numbers must run consecutively from first_card, which defaults to 1, with no gaps or duplicates.
// A non-positive or non-integer first_card is a finding of its own, and the numbering half then checks against 1.
func checkIndexFileConsistency(plan *Plan) []ValidationError {
	var findings []ValidationError

	indexed := make(map[string]bool, len(plan.Cards))
	for _, c := range plan.Cards {
		indexed[cardFileName(c.Number, c.Slug)] = true
	}

	// An empty Dir means no plan directory was given, not a fault.
	// ParsePlan always sets Dir, so an empty Dir is the in-memory plan shape that tests build.
	// There is nothing on disk to scan and nothing to report.
	// The guard runs before the ReadDir it guards, because ReadDir("") always fails.
	var entries []os.DirEntry
	var err error
	if plan.Dir != "" {
		entries, err = os.ReadDir(plan.Dir)
	}
	switch {
	case plan.Dir == "":
	case err != nil:
		// A plan directory that cannot be listed is a finding, not silence.
		// Without a listing, the half that reports orphaned card files cannot run, and the check must not report clean.
		// The numbering half below still runs.
		findings = append(findings, ValidationError{
			Check:  "index-file-mismatch",
			Detail: fmt.Sprintf("plan directory %s cannot be listed (%v), so no file on disk could be checked against the Card Index", plan.Dir, err),
		})
	default:
		var onDisk []string
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") || knownNonCardFiles[e.Name()] || IsLensReportFileName(e.Name()) {
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

	first := plan.FirstCard
	if plan.FirstCardInvalid != "" {
		findings = append(findings, ValidationError{
			Check:  "index-file-mismatch",
			Detail: fmt.Sprintf("first_card %q is not a positive integer; numbering is checked against 1", plan.FirstCardInvalid),
		})
		first = 1
	}
	if first < 1 {
		// A hand-built Plan leaves FirstCard at its zero value, which means the default.
		first = 1
	}

	for i, c := range plan.Cards {
		want := first + i
		if c.Number != want {
			findings = append(findings, ValidationError{
				Check: "index-file-mismatch",
				Card:  cardID(c),
				Detail: fmt.Sprintf(
					"Card Index numbering has a gap, duplicate or card below first_card (%d): expected card number %d at index position %d, got %d",
					first, want, i+1, c.Number,
				),
			})
		}
	}

	return findings
}

// checkCardTypeMissing implements card-type-missing: every card must carry at least one recognized type label.
// Carrying more than one, or repeating the same label, is legal.
func checkCardTypeMissing(plan *Plan) []ValidationError {
	var findings []ValidationError

	for _, c := range plan.Cards {
		if c.TypeLabelCount == 0 {
			findings = append(findings, ValidationError{
				Check:  "card-type-missing",
				Card:   cardID(c),
				Detail: fmt.Sprintf("card %d carries no recognized type label (Create/Edit/Extends/Delete/Rename/Move/Prosa/Custom)", c.Number),
			})
		}
	}

	return findings
}

// checkCustomNotAlone implements card-custom-not-alone.
// A card carrying a Custom TargetGroup alongside a group of a different Type is a defect.
// Custom is a last-resort escape hatch, not a way to bundle an untyped target list onto an ordinary card.
// Two Custom groups on one card are not a finding.
// At most one finding is emitted per offending card, never one per offending group.
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

// checkCardPathMalformed implements card-path-malformed.
// Each path- or self-glyph-shaped Targets or Uses entry must map to a disk path.
// That path must be non-empty, relative, clean, and free of ".." escapes.
// Symbol-, handle-, and member-glyph-shaped entries are skipped.
// Pairs is not iterated separately, because both endpoints of every pair are already projected into Targets.
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

// checkBareSymbolTarget implements bare-symbol-target.
// A Targets or Uses entry that classifies as a symbol ref is a hard finding.
// A bare package-qualified symbol cannot have come verbatim from a quarry answer.
// The check is skipped when the plan's language does not enable the glyph alphabet, unrecognized languages included.
// An unrecognized language is not canonicalized by ParsePlan, so its refs lack the form this check assumes.
// plan-language-unrecognized already blocks an unrecognized-language plan, so nothing is lost by staying silent.
func checkBareSymbolTarget(plan *Plan) []ValidationError {
	var findings []ValidationError

	if _, ok := planLanguage(plan); !ok {
		return findings
	}

	for _, c := range plan.Cards {
		for _, fields := range [][]string{c.Targets, c.Uses} {
			for _, t := range fields {
				if _, disp := lookup(gateBareSymbolTarget, t); disp != dispFinding {
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

// checkDirectoryTarget implements directory-target.
// A refKindPath entry that contains a "/" and has no file extension names a directory, not a file.
// It should be spelled as a unit glyph if it is a package, or its files listed instead if it is not code.
// A slash-free extensionless entry at the repository root is not caught here.
// It falls to path-missing, or on a Prosa group to prosa-symbol-target, and that narrower coverage is accepted.
// An extensionless file under a root other than "." is caught here too.
// No lexical rule can tell it from a directory.
// For that case, the detail names the file self glyph as the fix.
// The check is skipped when the plan's language does not enable the glyph alphabet, unrecognized languages included.
func checkDirectoryTarget(plan *Plan) []ValidationError {
	var findings []ValidationError

	if _, ok := planLanguage(plan); !ok {
		return findings
	}

	for _, c := range plan.Cards {
		for _, fields := range [][]string{c.Targets, c.Uses} {
			for _, t := range fields {
				if _, disp := lookup(gateDirectoryTarget, t); disp != dispKeep {
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

// checkGlyphMalformed implements glyph-malformed.
// A Targets or Uses entry that contains "#" but fails to parse as a glyph is a hard finding.
// Without this check, the plan would validate clean while carrying a target that no execution engine can act on,
// and the defect would surface only deep in a batch's done check, not at the plan gate.
// Under a go plan, an entry that the C# alphabet accepts gets a hint to set `language: csharp`.
// The check is skipped when the plan's language does not enable the glyph alphabet, unrecognized languages included.
func checkGlyphMalformed(plan *Plan) []ValidationError {
	var findings []ValidationError

	if _, ok := planLanguage(plan); !ok {
		return findings
	}

	for _, c := range plan.Cards {
		for _, fields := range [][]string{c.Targets, c.Uses} {
			for _, t := range fields {
				if _, disp := lookup(gateGlyphMalformed, t); disp != dispKeep {
					continue
				}
				if _, err := ParseRef(plan, t); err != nil {
					detail := fmt.Sprintf(
						"card %d entry %q looks like a glyph (contains \"#\") but fails to parse: %v",
						c.Number, t, err,
					)
					if _, csharpErr := glyph.Parse(glyph.CSharp, t); csharpErr == nil && plan.GoToolchain() {
						detail += "; it is a C# glyph, so set the plan's `language: csharp`"
					}
					findings = append(findings, ValidationError{
						Check:  "glyph-malformed",
						Card:   cardID(c),
						Detail: detail,
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

// checkHandleConsistency implements handle-dangling and handle-collision.
// These checks run under every plan.Language, including "none": a handle is loomyard grammar, not glyph grammar.
// Its consistency is checkable without any alphabet.
func checkHandleConsistency(plan *Plan) []ValidationError {
	var findings []ValidationError

	claims := handleClaims(plan)
	referenced := referencedHandles(plan)

	// handle-dangling: a referenced handle with no matching Create declaration and no matching Rename to-side.
	handles := make([]string, 0, len(referenced))
	for h := range referenced {
		handles = append(handles, h)
	}
	sort.Strings(handles)
	for _, handle := range handles {
		if len(claims[handle]) > 0 || partHoldsHandle(plan, handle) {
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

	// handle-collision: the same handle is claimed more than once across the plan.
	// The claims come from two Create sub-bullets, two Rename to-sides, or one of each.
	// One finding is emitted per colliding handle, not one per claiming card.
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

	return findings
}

// checkHandleMalformed implements handle-malformed.
// It reports each CreateRaw entry that fails the two-field declaration grammar.
// It also reports each handle-shaped Targets or Uses entry whose text after the handle prefix has no "#".
// Such an entry names no unit.
// The first two rules run under every language, because a handle is loomyard grammar, not glyph grammar.
// Under a go plan, it also reports a handle whose unit half names a ".go" file, not a package directory.
// The file-unit rule exists because quarry's Name accepts a file unit and echoes it as the canonical ID.
// Resolve answers members under their package unit only, so the canonical handle never resolves.
// The plan validates clean, since the Create inversion reads not_found as the expected pre-create answer.
// The run then wedges at the creating card's record-batch done check, with no earlier diagnostic.
// The file-unit rule applies only to a Create declaration's handle, the one whose unit half is actually read.
func checkHandleMalformed(plan *Plan) []ValidationError {
	var findings []ValidationError

	claims := handleClaims(plan)
	goToolchain := plan.GoToolchain()

	for _, c := range plan.Cards {
		for _, raw := range c.CreateRaw {
			findings = append(findings, ValidationError{
				Check: handleMalformedCheck,
				Card:  cardID(c),
				Detail: fmt.Sprintf(
					"card %d Create: entry %q does not match the required `plan:<handle>` -> `<declaration head>` grammar",
					c.Number, raw,
				),
			})
		}

		for _, fields := range [][]string{c.Targets, c.Uses} {
			for _, r := range fields {
				if _, disp := lookup(gateHandleMalformed, r); disp != dispKeep {
					continue
				}
				unit, ok := handleUnit(r)
				if !ok {
					findings = append(findings, ValidationError{
						Check: handleMalformedCheck,
						Card:  cardID(c),
						Detail: fmt.Sprintf(
							"card %d handle %q carries no \"#\" after %q and therefore names no unit",
							c.Number, r, HandlePrefix,
						),
					})
					continue
				}
				if goToolchain && strings.HasSuffix(unit, ".go") && fileUnitRuleApplies(claims[r]) {
					findings = append(findings, ValidationError{
						Check: handleMalformedCheck,
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

// checkResignNotMember implements resign-not-member: an Edit re-sign arrow whose target is not a member glyph.
// A path, a bare symbol or a plan: handle is the finding.
// A file or package self glyph is a finding too, since none of these shapes has a prior signature to re-sign.
// A glyph that glyph.Parse rejects is left to glyph-malformed.
// The check is skipped when plan.Language does not enable the glyph alphabet.
func checkResignNotMember(plan *Plan) []ValidationError {
	var findings []ValidationError

	if _, ok := planLanguage(plan); !ok {
		return findings
	}

	for _, c := range plan.Cards {
		for _, r := range c.Resigns {
			shape, bad := resignTargetShape(plan, r.Target)
			if !bad {
				continue
			}
			findings = append(findings, ValidationError{
				Check: "resign-not-member",
				Card:  cardID(c),
				Detail: fmt.Sprintf(
					"card %d re-sign arrow target %q is %s, not a member glyph; put the arrow on the member glyph whose signature changes, since a file, a package or a handle has no prior signature to re-sign",
					c.Number, r.Target, shape,
				),
				Ref: r.Target,
			})
		}
	}

	return findings
}

// resignTargetShape names the shape of a re-sign arrow's target and reports whether that shape is a finding.
func resignTargetShape(plan *Plan, target string) (shape string, bad bool) {
	kind, disp := lookup(gateResignTarget, target)
	if disp == dispFinding {
		return refKindName(kind), true
	}
	g, err := ParseRef(plan, target)
	if err != nil || !g.IsSelf() {
		return "", false
	}
	unitPath, ok := g.UnitPath()
	if !ok {
		return "a namespace self glyph", true
	}
	if !hasFileExtension(unitPath) {
		return "a package self glyph", true
	}
	return "a file self glyph", true
}

// isFileRenamePair reports whether p is a file-rename pair under plan's language.
// Both p.Old and p.New must classify as refKindGlyph and parse to a glyph whose IsSelf() reports true.
// Such a pair has no declaration head to name, so the Rename pair shape checks skip it.
func isFileRenamePair(plan *Plan, p MovePair) bool {
	if _, disp := lookup(gateFileRenamePair, p.Old); disp != dispKeep {
		return false
	}
	if _, disp := lookup(gateFileRenamePair, p.New); disp != dispKeep {
		return false
	}
	oldGlyph, err := ParseRef(plan, p.Old)
	if err != nil || !oldGlyph.IsSelf() {
		return false
	}
	newGlyph, err := ParseRef(plan, p.New)
	if err != nil || !newGlyph.IsSelf() {
		return false
	}
	return true
}

// checkRenamePairShape implements rename-to-not-handle and rename-from-not-glyph.
// On a symbol rename the old side must be a glyph, since it names something that exists and will be resolved.
// The new side must be a plan: handle, whose declaration is derived from the resolved old side.
// The planner's draft spelling is overwritten at the validation boundary, not trusted.
// A Rename card therefore needs no declaration head of its own.
// File-rename pairs are exempt from both checks.
// Neither check runs when planLanguage reports not-ok (e.g. plan.Language "none").
//
// Each check rejects every shape but the one the format admits, rather than listing the shapes it forbids.
// A list would miss a kind nobody thought to name.
// Anything that is not the admitted shape is the finding, and refKindName names what it actually was.
// A self glyph on the old side with a handle on the new side passes both checks.
// That pair names a file or unit where a symbol rename must name a symbol, so rename-from-not-glyph also flags it.
func checkRenamePairShape(plan *Plan) []ValidationError {
	var findings []ValidationError

	if _, langOK := planLanguage(plan); !langOK {
		return findings
	}

	for _, c := range plan.Cards {
		for _, p := range c.Pairs {
			if isFileRenamePair(plan, p) {
				continue
			}
			if k, disp := lookup(gateRenameTo, p.New); disp == dispFinding {
				findings = append(findings, ValidationError{
					Check: "rename-to-not-handle",
					Card:  cardID(c),
					Detail: fmt.Sprintf(
						"card %d Rename pair %q -> %q has a new side that is %s, not a plan: handle",
						c.Number, p.Old, p.New, refKindName(k),
					),
				})
			}
			if k, disp := lookup(gateRenameFrom, p.Old); disp == dispFinding {
				findings = append(findings, ValidationError{
					Check: "rename-from-not-glyph",
					Card:  cardID(c),
					Detail: fmt.Sprintf(
						"card %d Rename pair %q -> %q has an old side that is %s, not a glyph",
						c.Number, p.Old, p.New, refKindName(k),
					),
				})
			} else if _, disp := lookup(gateRenameTo, p.New); disp == dispKeep {
				// A symbol rename's old side must name a symbol, a member glyph.
				// The to-side declaration is derived from the resolved old symbol.
				// A self glyph on the old side with a handle on the new side fits neither admitted shape.
				// Left to the resolve pass, the pair is refused as an unresolved old side, which misstates the mistake.
				if g, err := ParseRef(plan, p.Old); err == nil && g.IsSelf() {
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

// checkRenameMechanicMissing implements rename-mechanic-missing:
// a plan with at least one Rename group but an empty RenameMechanic section.
// A Rename group counts even on a card with other labels, since the requirement is keyed on the group's own type.
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

// checkRenameLeavesTest implements rename-leaves-test, under go only, since it reads Go's test naming.
// It flags a symbol Rename pair of `unit#Name` or `unit#T.M` when a card lists, under Edit, a test named after it:
// `Test<base>` or `Test<base>_<case>`, where base is Name or T_M, in the same unit or its external test package,
// and no Rename pair in the plan renames that test.
// An implementer renames such a test along with its symbol,
// so a later card still naming the old test refuses at its begin-batch.
// The finding is attributed to the renaming card, where the test's own Rename pair belongs.
func checkRenameLeavesTest(plan *Plan) []ValidationError {
	var findings []ValidationError
	if !plan.GoToolchain() {
		return findings
	}

	renamed := make(map[string]bool)
	for _, c := range plan.Cards {
		for _, p := range c.Pairs {
			renamed[p.Old] = true
		}
	}

	for _, c := range plan.Cards {
		for _, p := range c.Pairs {
			if isFileRenamePair(plan, p) {
				continue
			}
			old, err := ParseRef(plan, p.Old)
			if err != nil || old.IsSelf() || len(old.Owner) > 1 {
				continue
			}
			base := old.Name
			if len(old.Owner) == 1 {
				base = old.Owner[0] + "_" + old.Name
			}
			for _, editor := range plan.Cards {
				for _, g := range editor.TargetGroups {
					if g.Type != CardTypeEdit {
						continue
					}
					for _, ref := range g.Refs {
						if renamed[ref] {
							continue
						}
						test, err := ParseRef(plan, ref)
						if err != nil || goPackageDir(test.Unit) != goPackageDir(old.Unit) || len(test.Owner) != 0 || !namesTestOf(test.Name, base) {
							continue
						}
						findings = append(findings, ValidationError{
							Check: "rename-leaves-test",
							Card:  cardID(c),
							Ref:   ref,
							Detail: fmt.Sprintf(
								"card %d renames %q, but card %d lists its test %q under Edit and no Rename pair renames the test; an implementer renames the test with its symbol, so add the test's rename as a Rename pair on card %d",
								c.Number, p.Old, editor.Number, ref, c.Number,
							),
						})
					}
				}
			}
		}
	}

	return findings
}

// namesTestOf reports whether name is a Go test function named after base: `Test<base>`, or `Test<base>_<case>`.
func namesTestOf(name, base string) bool {
	rest, ok := strings.CutPrefix(name, "Test"+base)
	return ok && (rest == "" || strings.HasPrefix(rest, "_"))
}

// cardFieldLabel pairs a card field's Has-presence bool with its bold label.
type cardFieldLabel struct {
	present bool
	label   string
}

// checkCardMissingField implements card-missing-field: every card must carry the Intent: field.
// A card carrying any Edit, Extends or Delete TargetGroup must also carry the ImpactSummary: field.
// The requirement is a union over the card's own groups.
func checkCardMissingField(plan *Plan) []ValidationError {
	var findings []ValidationError

	for _, c := range plan.Cards {
		fields := []cardFieldLabel{
			{c.HasIntent, "Intent:"},
		}
		needsImpactSummary := false
		for _, g := range c.TargetGroups {
			if g.Type == CardTypeEdit || g.Type == CardTypeExtends || g.Type == CardTypeDelete {
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

// checkCardFieldEmpty implements card-field-empty: a label present with no content is distinct from an absent label.
// Each of a card's own TargetGroups is checked independently,
// so a card with a populated group and an empty one still gets a finding for the empty group.
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

// checkUsesLaterTarget implements uses-later-target:
// a card's Uses: entry that a card with a higher number lists among its targets.
// A plan runs in card-number order, so such an entry is read before the card that produces it.
// Entries match by exact string equality after trimming whitespace; an entry that trims to empty is ignored.
func checkUsesLaterTarget(plan *Plan) []ValidationError {
	var findings []ValidationError

	for _, consumer := range plan.Cards {
		for _, producer := range plan.Cards {
			if producer.Number <= consumer.Number {
				continue
			}

			targets := make(map[string]bool, len(producer.Targets))
			for _, t := range producer.Targets {
				targets[strings.TrimSpace(t)] = true
			}

			seen := make(map[string]bool)
			for _, u := range consumer.Uses {
				entry := strings.TrimSpace(u)
				if entry == "" || !targets[entry] || seen[entry] {
					continue
				}
				seen[entry] = true
				findings = append(findings, ValidationError{
					Check: "uses-later-target",
					Card:  cardID(consumer),
					Detail: fmt.Sprintf(
						"card %d Uses %q, which card %d targets; a plan runs in card-number order, so move that work or reorder the cards so card %d's change comes before card %d",
						consumer.Number, entry, producer.Number, producer.Number, consumer.Number,
					),
				})
			}
		}
	}

	return findings
}

// checkImpactSummaryMultiline implements impact-summary-multiline:
// an ImpactSummary: field followed by trailing lines is a defect.
// ImpactSummary is required to stay a single line.
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

// checkProsaSymbolTarget implements prosa-symbol-target:
// a Prosa group's own target list must hold only file(s)/package(s), never an individual symbol.
// Only the Prosa group's own refs are checked; a symbol in another group of the same card is not flagged.
// Under a glyph-enabled plan.Language, a ref passes when it parses as a glyph and that glyph's IsSelf() reports true.
// The file and unit self glyphs both pass, so a whole package such as "internal/foo#" is a valid target.
// Any other ref is the finding:
// a member glyph, a plain path, a bare symbol, or anything that fails to parse as a glyph.
// Under "none", a path-shaped ref passes, and a symbol-shaped ref is the finding.
func checkProsaSymbolTarget(plan *Plan) []ValidationError {
	var findings []ValidationError

	_, langOK := planLanguage(plan)

	for _, c := range plan.Cards {
		for _, g := range c.TargetGroups {
			if g.Type != CardTypeProsa {
				continue
			}
			for _, t := range g.Refs {
				if langOK {
					if gl, err := ParseRef(plan, t); err == nil && gl.IsSelf() {
						continue
					}
				} else if _, disp := lookup(gateProsaPathOnly, t); disp == dispKeep {
					continue
				}
				// The detail states what the entry must be rather than calling it a symbol.
				// A bare extensionless directory, for one, is not a symbol.
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
		headingNumber, ok := cardHeadingNumber(plan, c)
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

// cardHeadingNumber returns the card number declared by the heading on the first line of c's own card file.
// It reports false when the file cannot be read or that line does not parse as a card heading.
func cardHeadingNumber(plan *Plan, c Card) (int, bool) {
	data, err := plan.ReadFile(cardFileName(c.Number, c.Slug))
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

// pathExistsOnDisk reports whether p, joined to worktreeRoot, exists on disk.
func pathExistsOnDisk(worktreeRoot, p string) bool {
	_, err := os.Stat(filepath.Join(worktreeRoot, p))
	return err == nil
}

// createTargetsUnion returns the set of disk paths that the refs of every Create target group in plan resolve to.
// Only a card's Create groups count, never its other groups, and a ref that resolves to no disk path is left out.
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

// renameTargetsUnion returns the set of disk paths that the New side of every pair in plan resolves to.
// A pair whose New side resolves to no disk path is left out.
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

// checkPathMissing reports each path that a card names and that is missing on disk.
// Every card's Uses entries are checked, as are the refs of its Edit, Extends, Delete, Move and Prosa groups.
// The Old side of each pair in a Rename group is checked too; Create and Custom refs are not.
// A path is also satisfied by a Create target or Rename destination of any card, or by one the part scope lists.
// A ref that resolves to no disk path is skipped.
func checkPathMissing(plan *Plan, worktreeRoot string) []ValidationError {
	var findings []ValidationError

	creates := createTargetsUnion(plan)
	renames := renameTargetsUnion(plan)

	satisfied := func(p string) bool {
		return pathExistsOnDisk(worktreeRoot, p) || creates[p] || renames[p] || (plan.Part != nil && slices.Contains(plan.Part.Paths, p))
	}

	checkRef := func(c Card, raw string) {
		mapped, ok := diskPathForRef(plan, raw)
		if !ok || satisfied(mapped) {
			return
		}
		findings = append(findings, ValidationError{
			Check: "path-missing",
			Card:  cardID(c),
			Detail: fmt.Sprintf(
				"card %d path %q does not exist on disk and is not a Create target or Rename destination of any card%s",
				c.Number, raw, rootRelativeGlyphRemedy(plan, raw, mapped, satisfied),
			),
			Ref: raw,
		})
	}

	for _, c := range plan.Cards {
		for _, u := range c.Uses {
			checkRef(c, u)
		}

		for _, g := range c.TargetGroups {
			switch g.Type {
			case CardTypeEdit, CardTypeExtends, CardTypeDelete, CardTypeMove, CardTypeProsa:
				for _, t := range g.Refs {
					checkRef(c, t)
				}
			case CardTypeRename:
				for _, p := range g.Pairs {
					checkRef(c, p.Old)
				}
			case CardTypeCreate, CardTypeCustom:
				// A Create ref names a file that the card itself introduces.
				// A Custom group is an explicit escape hatch from path-missing.
			}
		}
	}

	return findings
}

// rootRelativeGlyphRemedy returns the remedy that path-missing appends when a self glyph's disk path is missing.
// A glyph is never joined to root:, so `README.md#` under `root: docs` names a file at the worktree root.
// The remedy is given only when the path joined under root: also passes path-missing's test.
// Its fixes: name the self glyph of that joined path, or drop the `//` escape or trailing `#` so root: joins it.
// It returns "" when raw does not parse as a glyph, root: is absent or ".", or the joined path fails the test.
func rootRelativeGlyphRemedy(plan *Plan, raw, mapped string, satisfied func(string) bool) string {
	root := cleanPosixPath(plan.Root)
	if root == "" || root == "." {
		return ""
	}
	parsed, err := ParseRef(plan, raw)
	if err != nil {
		return ""
	}
	joined := path.Join(root, mapped)
	if !satisfied(joined) {
		return ""
	}
	remedy, err := glyph.Self(parsed.Lang, joined)
	if err != nil {
		return ""
	}
	return fmt.Sprintf(
		"; root: is never joined to a glyph or a //-escaped path, so this names the worktree-root file %q; to name the file root: joins instead, spell it as %q, or drop the // escape or the trailing \"#\" so root: joins the path %q",
		mapped, remedy.String(), mapped,
	)
}

// partHoldsHandle reports whether the plan's part scope lists handle.
// The part scope's handles are the ones that other parts and shared spots export to this part.
func partHoldsHandle(plan *Plan, handle string) bool {
	return plan.Part != nil && slices.Contains(plan.Part.Handles, handle)
}

// checkPartSpotTarget reports each target ref that resolves to a shared spot's path under a part scope.
// It reads every target group's refs and both sides of its pairs, never a card's Uses entries.
// The chair writes a shared spot's cards.
// A card that targets one drops the target and says in its **Intent:** what the card needs from the spot.
func checkPartSpotTarget(plan *Plan) []ValidationError {
	var findings []ValidationError
	for _, c := range plan.Cards {
		reported := map[string]bool{}
		check := func(raw string) {
			mapped, ok := diskPathForRef(plan, raw)
			if !ok || reported[raw] || !slices.Contains(plan.Part.SpotPaths, mapped) {
				return
			}
			reported[raw] = true
			findings = append(findings, ValidationError{
				Check: "part-spot-target",
				Card:  cardID(c),
				Detail: fmt.Sprintf(
					"card %d targets %q, which is the shared spot %q; the chair writes a shared spot's cards, so drop the target and say in the card's **Intent:** what the card needs from the spot",
					c.Number, raw, mapped,
				),
				Ref: raw,
			})
		}
		for _, g := range c.TargetGroups {
			for _, t := range g.Refs {
				check(t)
			}
			for _, p := range g.Pairs {
				check(p.Old)
				check(p.New)
			}
		}
	}
	return findings
}

// checkRenameNamespacePair implements rename-namespace-pair.
// Under csharp, a Rename pair of two self glyphs is refused when its old side is slash-free and names nothing on disk.
// Under csharp such a pair is a file pair that runs `git mv`,
// so a namespace pair such as `Glyphchain.Sharp# -> Glyphchain.Core#` would move a path that does not exist.
// A slash-free self glyph maps to no disk path under csharp, so the old side is read as a path entry instead.
// The check asks the disk rather than the file extension, since a dotted namespace also has an extension.
// A path that an earlier card creates or renames into is not reported.
// Dispatch validation drops a finding against a done card, so a pair whose `git mv` has landed is not rechecked.
// Under go, path-missing covers the same pair, so this check stays silent there.
func checkRenameNamespacePair(plan *Plan, worktreeRoot string) []ValidationError {
	var findings []ValidationError
	if lang, ok := planLanguage(plan); !ok || lang != glyph.CSharp {
		return findings
	}

	earlier := make(map[string]bool)
	for _, c := range plan.Cards {
		for _, pair := range c.Pairs {
			if !isFileRenamePair(plan, pair) {
				continue
			}
			oldPath, ok := PathEntryPath(pair.Old)
			if !ok || strings.Contains(oldPath, "/") || earlier[oldPath] || pathExistsOnDisk(worktreeRoot, oldPath) {
				continue
			}
			findings = append(findings, ValidationError{
				Check: "rename-namespace-pair",
				Card:  cardID(c),
				Ref:   pair.Old,
				Detail: fmt.Sprintf(
					"card %d Rename pair %q -> %q names no path on disk, and under csharp a pair of two self glyphs is a file pair that runs `git mv`, so a namespace cannot be renamed this way; rename the namespace through Edit cards on the files that declare it and the files that refer to it; when the old side is meant as a root file, correct it to a file that exists; or, when a later card creates the old side, move the pair to a card after it",
					c.Number, pair.Old, pair.New,
				),
			})
		}

		for _, g := range c.TargetGroups {
			if g.Type != CardTypeCreate {
				continue
			}
			for _, ref := range g.Refs {
				if created, ok := PathEntryPath(ref); ok {
					earlier[created] = true
				}
			}
		}
		for _, pair := range c.Pairs {
			if destination, ok := PathEntryPath(pair.New); ok {
				earlier[destination] = true
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
