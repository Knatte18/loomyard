// roundcheck.go defines the round checks: what a review after the first round must hold against the round before it.
// A round context says what round N-1 found and what its fixer answered.
// CheckReviewRound judges a review against that context alone, and RoundCheckGateEntry lets the reviewer repair it.

package burlerengine

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// PriorStatus is what a review says became of one finding of the round before it.
type PriorStatus string

// The four legal PriorStatus values.
const (
	// PriorResolved: the finding no longer holds.
	PriorResolved PriorStatus = "resolved"
	// PriorOpen: the finding still holds and a finding of this review carries it.
	PriorOpen PriorStatus = "open"
	// PriorOutdated: the finding no longer applies because the code it named changed.
	PriorOutdated PriorStatus = "outdated"
	// PriorWithdrawn: the reviewer accepts the fixer's dispute of the finding.
	PriorWithdrawn PriorStatus = "withdrawn"
)

// PriorEntry is one `prior` entry of a review's frontmatter: the status of one finding of the round before it.
// Answer is the reviewer's reasoning, which a disputed finding that stays open and a held finding that is resolved need.
type PriorEntry struct {
	ID     string      `yaml:"id"`
	Status PriorStatus `yaml:"status"`
	Answer string      `yaml:"answer"`
}

// PriorFinding is one finding of the round before the one being judged, with what the fixer answered.
// Disposition is nil when the fixer gave none.
// Held marks a decision-class finding of a round that routed its decisions to the run's parent.
type PriorFinding struct {
	Finding     Finding
	Disposition *FindingDisposition
	Held        bool
}

// RoundContext is what a review after the first round is judged against.
// A nil context stands for a round with no earlier findings to answer.
type RoundContext struct {
	// Incremental marks a round that raises a new finding only at BLOCKING, and only on what the fixes changed.
	Incremental bool
	// Prior lists the findings of the round before, in the order its review listed them.
	Prior []PriorFinding
	// AddedTitles holds the title of every decision record entry added since that round, in order.
	// AddedTexts holds their texts in the same order.
	AddedTitles []string
	AddedTexts  []string
	// HasChanges is true when the round holds a changes file; Changed then lists its worktree-relative paths.
	HasChanges bool
	Changed    []string
	// AmendingTexts holds the full text, heading included, of every decision record entry pending as a plan amendment.
	// A plan card file such an entry names is exempt from the changed-file rule.
	AmendingTexts []string
	// HaltReason is the stuck reason of the plan executor call that sent the run back to review, empty when none.
	// A new BLOCKING `origin: webster` finding is exempt from the changed-file rule while it is set.
	HaltReason string
	// HardenerRound marks a hardener round, which drives behaviour that the round's changed hunks do not bound.
	// None of its findings, new or carried, is held to the BLOCKING or changed-file rules of an incremental round.
	HardenerRound bool
	// ParentReportPath is the report of the round's parent-review ask, empty when the round holds none.
	ParentReportPath string
	// PriorReviewPath, PriorFixerReportPath and ChangesPath name the files the prior-findings block points at.
	// ChangesPath is empty when the round holds no changes file.
	PriorReviewPath      string
	PriorFixerReportPath string
	ChangesPath          string
	// WorktreeRoot is the worktree a finding's absolute location is made relative to.
	WorktreeRoot string
}

// roundCheckGateName labels the round-check entry in a gate outcome.
const roundCheckGateName = "round"

// roundCheckGateAttempts is the re-prompt budget of the round-check gate entry.
const roundCheckGateAttempts = 3

// roundCheckWayForward opens every refusal's closing clause; each refusal then names what to change.
const roundCheckWayForward = "way forward: rewrite the review file"

// roundRefusal returns a round-check refusal: the problem, then the way forward that completes rewrite.
func roundRefusal(problem, rewrite string) error {
	return fmt.Errorf("burler: review file %s; %s %s", problem, roundCheckWayForward, rewrite)
}

// CheckReviewRound judges a review against the round context ctx, after the shape checks of ParseReview.
// With a nil ctx it refuses any `prior` entry and any `carries` key.
// Otherwise it refuses, with the way forward:
// a `prior` list that misses, repeats or invents an id of the round before, or holds an unknown status;
// a withdrawn entry whose finding the fixer did not dispute, or an open one that gives a disputed finding no answer;
// a held finding that is neither resolved nor open, or resolved without quoting a title of an added entry;
// `carries` links that do not match the open entries one to one;
// a carried finding that changes class, raises its severity, or moves to a file the round did not change;
// and a new finding in an incremental round that is not BLOCKING, is outside the changed files, or is not covered.
// A new `origin: parent` finding is exempt from the BLOCKING and changed-file rules.
// It is exempt only when its parent_ids are all ids of the round's parent report.
// A carried or new finding located in a file that an entry of ctx.AmendingTexts names passes the changed-file rule.
// A new BLOCKING finding with `origin: webster` passes it too when ctx.HaltReason is set.
// With ctx.HardenerRound set, a new finding passes at any severity and in any file,
// and a carried one may move to any file.
func CheckReviewRound(content []byte, ctx *RoundContext) error {
	_, findings, err := ParseReview(content)
	if err != nil {
		return err
	}
	header, _, err := splitFrontmatter(content)
	if err != nil {
		return err
	}
	var parsed reviewHeader
	if err := yaml.Unmarshal([]byte(header), &parsed); err != nil {
		return fmt.Errorf("burler: review file frontmatter is not valid YAML: %w", err)
	}
	if ctx == nil {
		return checkNoRoundContext(parsed.Prior, findings)
	}

	check := roundCheck{ctx: ctx, earlier: make(map[string]PriorFinding, len(ctx.Prior))}
	for _, prior := range ctx.Prior {
		check.earlier[prior.Finding.ID] = prior
	}
	if err := check.priorEntries(parsed.Prior); err != nil {
		return err
	}
	if err := check.carries(findings); err != nil {
		return err
	}
	return check.newFindings(findings)
}

// checkNoRoundContext refuses a review that answers earlier findings in a round that has none.
func checkNoRoundContext(prior []PriorEntry, findings []Finding) error {
	if len(prior) > 0 {
		return roundRefusal("holds a `prior` list, but this round has no earlier findings to answer", "without the `prior` key")
	}
	for _, f := range findings {
		if f.Carries != "" {
			return roundRefusal(fmt.Sprintf("finding %q carries %q, but this round has no earlier findings to carry", f.ID, f.Carries),
				fmt.Sprintf("without the `carries` key on %q", f.ID))
		}
	}
	return nil
}

// roundCheck holds one review's judgment against its round context.
type roundCheck struct {
	ctx *RoundContext
	// earlier maps each finding id of the round before to that finding.
	earlier map[string]PriorFinding
	// entries maps each id of the review's prior list to its entry, set by priorEntries.
	entries map[string]PriorEntry
}

// priorEntries checks the review's prior list against the round before it and records the entries by id.
func (c *roundCheck) priorEntries(prior []PriorEntry) error {
	c.entries = make(map[string]PriorEntry, len(prior))
	for _, entry := range prior {
		earlier, known := c.earlier[entry.ID]
		if !known {
			return roundRefusal(fmt.Sprintf("has a `prior` entry for %q, which the round before does not list", entry.ID),
				fmt.Sprintf("without the `prior` entry for %q", entry.ID))
		}
		if _, repeated := c.entries[entry.ID]; repeated {
			return roundRefusal(fmt.Sprintf("lists %q twice in its `prior` list", entry.ID), fmt.Sprintf("with one `prior` entry for %q", entry.ID))
		}
		c.entries[entry.ID] = entry
		if err := c.priorEntry(entry, earlier); err != nil {
			return err
		}
	}
	for _, earlier := range c.ctx.Prior {
		if _, listed := c.entries[earlier.Finding.ID]; !listed {
			return roundRefusal(fmt.Sprintf("has no `prior` entry for finding %q", earlier.Finding.ID),
				fmt.Sprintf("with a `prior` entry for %q", earlier.Finding.ID))
		}
	}
	return nil
}

// priorEntry checks one prior entry against the finding it answers.
func (c *roundCheck) priorEntry(entry PriorEntry, earlier PriorFinding) error {
	switch entry.Status {
	case PriorResolved, PriorOpen, PriorOutdated, PriorWithdrawn:
	default:
		return roundRefusal(fmt.Sprintf("gives %q the status %q, which is none of resolved, open, outdated and withdrawn", entry.ID, entry.Status),
			fmt.Sprintf("with one of those four statuses for %q", entry.ID))
	}
	disputed := earlier.Disposition != nil && earlier.Disposition.Disposition == DispositionDisputed
	if entry.Status == PriorWithdrawn && !disputed {
		return roundRefusal(fmt.Sprintf("withdraws %q, whose fixer disposition is not `disputed`", entry.ID),
			fmt.Sprintf("with another status for %q, since only an accepted dispute is withdrawn", entry.ID))
	}
	if entry.Status == PriorOpen && disputed && strings.TrimSpace(entry.Answer) == "" {
		return roundRefusal(fmt.Sprintf("keeps %q open without answering the fixer's dispute", entry.ID),
			fmt.Sprintf("with an `answer` on the `prior` entry for %q that answers the rebuttal", entry.ID))
	}
	if !earlier.Held {
		return nil
	}
	if entry.Status != PriorResolved && entry.Status != PriorOpen {
		return roundRefusal(fmt.Sprintf("gives the held finding %q the status %q, but only the parent's call settles it", entry.ID, entry.Status),
			fmt.Sprintf("with the status resolved or open for %q", entry.ID))
	}
	if entry.Status == PriorResolved && !quotesAddedTitle(entry.Answer, c.ctx.AddedTitles) {
		return roundRefusal(fmt.Sprintf("resolves the held finding %q without quoting the title of an entry added to the decision record", entry.ID),
			fmt.Sprintf("with the `answer` for %q quoting the title of the added entry that settles it, or with the status open", entry.ID))
	}
	return nil
}

// quotesAddedTitle reports whether answer contains a title of titles.
func quotesAddedTitle(answer string, titles []string) bool {
	for _, title := range titles {
		if title != "" && strings.Contains(answer, title) {
			return true
		}
	}
	return false
}

// carries checks that the review's `carries` links match its open prior entries one to one,
// and that each carried finding keeps its class, does not raise its severity and stays in the changed files.
func (c *roundCheck) carries(findings []Finding) error {
	carried := make(map[string]bool, len(findings))
	for _, f := range findings {
		if f.Carries == "" {
			continue
		}
		if entry, listed := c.entries[f.Carries]; !listed || entry.Status != PriorOpen {
			return roundRefusal(fmt.Sprintf("finding %q carries %q, which is not an open `prior` entry", f.ID, f.Carries),
				fmt.Sprintf("with `carries` on %q naming an open entry, or without it", f.ID))
		}
		if carried[f.Carries] {
			return roundRefusal(fmt.Sprintf("carries %q in more than one finding", f.Carries), fmt.Sprintf("with one finding carrying %q", f.Carries))
		}
		carried[f.Carries] = true
		if err := c.carriedFinding(f, c.earlier[f.Carries].Finding); err != nil {
			return err
		}
	}
	for _, earlier := range c.ctx.Prior {
		if c.entries[earlier.Finding.ID].Status == PriorOpen && !carried[earlier.Finding.ID] {
			return roundRefusal(fmt.Sprintf("leaves %q open in its `prior` list but no finding carries it", earlier.Finding.ID),
				fmt.Sprintf("with a finding whose `carries` is %q, or with %q resolved, outdated or withdrawn", earlier.Finding.ID, earlier.Finding.ID))
		}
	}
	return nil
}

// carriedFinding checks a finding that carries earlier: its class, its severity and its location path.
func (c *roundCheck) carriedFinding(f, earlier Finding) error {
	if f.Class != earlier.Class {
		return roundRefusal(fmt.Sprintf("finding %q carries %q with the class %q, not the earlier class %q", f.ID, earlier.ID, f.Class, earlier.Class),
			fmt.Sprintf("with the class of %q kept as %q", f.ID, earlier.Class))
	}
	if severityRank(f.Severity) > severityRank(earlier.Severity) {
		return roundRefusal(fmt.Sprintf("finding %q carries %q at the severity %s, above its earlier %s", f.ID, earlier.ID, f.Severity, earlier.Severity),
			fmt.Sprintf("with the severity of %q at %s or lower", f.ID, earlier.Severity))
	}
	path := locationPath(f.Location, c.ctx.WorktreeRoot)
	if path == locationPath(earlier.Location, c.ctx.WorktreeRoot) {
		return nil
	}
	if c.ctx.HardenerRound || c.namedByAmendment(path) {
		return nil
	}
	if !c.ctx.HasChanges {
		return roundRefusal(fmt.Sprintf("finding %q carries %q but moves its location from the earlier path, and the round has no changes file", f.ID, earlier.ID),
			fmt.Sprintf("with the location of %q keeping the path of %q", f.ID, earlier.ID))
	}
	if !slices.Contains(c.ctx.Changed, path) {
		return roundRefusal(fmt.Sprintf("finding %q carries %q at %q, which the changes file does not list", f.ID, earlier.ID, path),
			fmt.Sprintf("with the location of %q keeping its path or naming a file the changes file lists", f.ID))
	}
	return nil
}

// newFindings checks, in an incremental round, every finding that carries no earlier one.
func (c *roundCheck) newFindings(findings []Finding) error {
	if !c.ctx.Incremental {
		return nil
	}
	websterOutside := 0
	for _, f := range findings {
		if f.Carries != "" {
			continue
		}
		if c.parentCovered(f) {
			continue
		}
		if f.Origin == ParentOrigin {
			return roundRefusal(fmt.Sprintf("has the new finding %q with `origin: parent`, but its `parent_ids` are not all ids of this round's parent report", f.ID),
				fmt.Sprintf("with `parent_ids` on %q listing ids of the parent report, or without the finding", f.ID))
		}
		// A fault a hardener round drives sits where it lives, on a changed hunk or not, at the severity it has.
		if c.ctx.HardenerRound {
			continue
		}
		if f.Severity != SeverityBlocking {
			return roundRefusal(fmt.Sprintf("has the new finding %q at %s, but an incremental round raises a new finding only at BLOCKING", f.ID, f.Severity),
				fmt.Sprintf("without %q, or with it at BLOCKING on a changed hunk", f.ID))
		}
		path := locationPath(f.Location, c.ctx.WorktreeRoot)
		outsideChanges := c.ctx.HasChanges && !slices.Contains(c.ctx.Changed, path) && !c.namedByAmendment(path)
		if outsideChanges && f.Origin == WebsterOrigin && c.ctx.HaltReason != "" {
			// The halt reason is one finding, so only one finding outside the changed files takes its exemption.
			if websterOutside++; websterOutside == 1 {
				continue
			}
			return roundRefusal(fmt.Sprintf("has a second new finding %q with `origin: webster` at %q, which the changes file does not list, but the plan executor's halt reason admits one", f.ID, path),
				fmt.Sprintf("with %q located at a file the changes file lists, or without it", f.ID))
		}
		if outsideChanges {
			return roundRefusal(fmt.Sprintf("has the new finding %q at %q, which the changes file does not list", f.ID, path),
				fmt.Sprintf("with %q located at the changed line that causes it, or without it", f.ID))
		}
	}
	return nil
}

// minAmendmentStemLength is the shortest file stem that namedByAmendment matches.
// A shorter stem would match some text by accident.
const minAmendmentStemLength = 4

// namedByAmendment reports whether path is a plan card file named by a decision record entry pending as a plan amendment.
// Two spellings of the card count.
// One is its base name without extension; the other is that stem without a leading run of digits and one `-`.
// Either appears in one of the round's AmendingTexts as a whole token, with no identifier character beside it.
// Plan cards are files like `14-card-slug.md`, which an entry may name by `14-card-slug` or `card-slug`.
func (c *roundCheck) namedByAmendment(path string) bool {
	// The plan directory is `_lym/plan`.
	// `_lym` is a link out of the worktree, so a location may spell it under another root.
	dir := filepath.ToSlash(filepath.Dir(path))
	if len(c.ctx.AmendingTexts) == 0 || dir != planparser.PlanDirRel() && !strings.HasSuffix(dir, "/"+planparser.PlanDirRel()) || filepath.Ext(path) != ".md" {
		return false
	}
	base := filepath.Base(path)
	if planparser.IsNonCardFileName(base) {
		return false
	}
	stem := strings.TrimSuffix(base, filepath.Ext(base))
	names := []string{stem}
	if rest := strings.TrimLeft(stem, "0123456789"); rest != stem {
		if slug, found := strings.CutPrefix(rest, "-"); found {
			names = append(names, slug)
		}
	}
	for _, name := range names {
		if len(name) < minAmendmentStemLength {
			continue
		}
		for _, text := range c.ctx.AmendingTexts {
			if containsToken(text, name) {
				return true
			}
		}
	}
	return false
}

// containsToken reports whether text holds token with no letter, digit, `-` or `_` directly before or after it.
func containsToken(text, token string) bool {
	isIdentifier := func(r byte) bool {
		return r == '-' || r == '_' || r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z'
	}
	for offset := 0; ; {
		found := strings.Index(text[offset:], token)
		if found < 0 {
			return false
		}
		start := offset + found
		end := start + len(token)
		if (start == 0 || !isIdentifier(text[start-1])) && (end == len(text) || !isIdentifier(text[end])) {
			return true
		}
		offset = start + 1
	}
}

// parentCovered reports whether f is a finding kept from the round's parent report:
// its origin is parent, the round holds a parent report, and its parent_ids are non-empty and all ids of that report.
// A report that cannot be read has no ids.
func (c *roundCheck) parentCovered(f Finding) bool {
	if f.Origin != ParentOrigin || c.ctx.ParentReportPath == "" || len(f.ParentIDs) == 0 {
		return false
	}
	report, err := ReadParentReport(c.ctx.ParentReportPath)
	if err != nil {
		return false
	}
	ids := make(map[string]bool, len(report.Findings))
	for _, reported := range report.Findings {
		ids[reported.ID] = true
	}
	for _, id := range f.ParentIDs {
		if !ids[id] {
			return false
		}
	}
	return true
}

// severityRank orders severities from NIT, the lowest, to BLOCKING, the highest.
func severityRank(severity Severity) int {
	switch severity {
	case SeverityBlocking:
		return 3
	case SeverityMedium:
		return 2
	case SeverityLow:
		return 1
	default:
		return 0
	}
}

// locationPath returns the file path a finding's location names:
// the location's text up to the first `:`, `#` or space.
// An absolute path is made relative to worktreeRoot, and the result is cleaned to forward slashes.
func locationPath(location, worktreeRoot string) string {
	text := strings.TrimSpace(location)
	if cut := strings.IndexAny(text, ":# "); cut >= 0 {
		text = text[:cut]
	}
	if text == "" {
		return ""
	}
	if filepath.IsAbs(text) && worktreeRoot != "" {
		if relative, err := filepath.Rel(worktreeRoot, text); err == nil {
			text = relative
		}
	}
	return filepath.ToSlash(filepath.Clean(text))
}

// RoundCheckGateEntry returns the gate entry that re-prompts a reviewer to repair its review at reviewPath against ctx.
// It lets the run through once its re-prompt budget is spent,
// so a review that stays outside the round checks keeps its findings and the fixer works them.
func RoundCheckGateEntry(reviewPath string, ctx *RoundContext) shuttleengine.GateEntry {
	return shuttleengine.GateEntry{
		Name:      roundCheckGateName,
		Gate:      roundCheckGate(reviewPath, ctx),
		Attempts:  roundCheckGateAttempts,
		PassOnCap: true,
	}
}

// roundCheckGate returns the closure of the round-check entry; see RoundCheckGateEntry.
// A missing or refused review is a failed result, never a gate error.
func roundCheckGate(reviewPath string, ctx *RoundContext) shuttleengine.Gate {
	return func() (shuttleengine.GateResult, error) {
		content, err := os.ReadFile(reviewPath)
		if err != nil {
			err = fmt.Errorf("burler: read review file %q: %w", reviewPath, err)
		} else {
			err = CheckReviewRound(content, ctx)
		}
		if err != nil {
			return shuttleengine.GateResult{Findings: fmt.Sprintf("%s\n\nRewrite the review file (%s) so it passes the round checks.", err, reviewPath)}, nil
		}
		return shuttleengine.GateResult{Passed: true}, nil
	}
}
