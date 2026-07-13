// plan.go implements the plan-format v2 parser: ParsePlan reads a plan
// directory's 00-overview.md (frontmatter + Batch Index + framing) and, for
// each batch the index lists, its own NN-<batch-slug>.md file, producing the
// in-memory Plan the rest of builderengine drives from. Every parse failure
// is a distinct, "builder:"-prefixed wrapped error — plan-format.md's
// fail-loud discipline admits no silent-default reading of a malformed
// plan. This file currently parses only 00-overview.md; per-batch file
// parsing is filled in by parseBatchFile (see plan.go's later revision).

package builderengine

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// overviewFileName is the fixed filename of a plan's overview file within
// its plan directory, per plan-format.md's on-disk layout.
const overviewFileName = "00-overview.md"

// Plan is the in-memory form of a parsed plan-format v2 plan: the
// overview's frontmatter and task framing, plus every batch the Batch Index
// lists, each itself parsed from its own NN-<batch-slug>.md file (see
// PlanBatch).
type Plan struct {
	// Dir is the plan directory ParsePlan was given: _lyx/plan in
	// production (resolved via hubgeometry.PlanDir by the caller), a plain
	// testdata directory in tests.
	Dir string

	// Format is the plan-format version the plan is written against, taken
	// as-is from 00-overview.md's frontmatter. ParsePlan does not reject an
	// unrecognized value — that is Validate's format-unrecognized check.
	Format int

	// Approved mirrors the overview frontmatter's approved: field.
	// ParsePlan does not refuse an unapproved plan itself — that is
	// Validate's plan-unapproved check.
	Approved bool

	// Framing is the short task-framing paragraph(s) between the overview's
	// title heading and its "## Batch Index" heading.
	Framing string

	// Batches is every batch the Batch Index lists, in index order, each
	// parsed from its own per-batch file.
	Batches []PlanBatch
}

// PlanBatch is one plan batch: the Batch Index entry's fields plus
// everything parsed from its own NN-<batch-slug>.md file.
type PlanBatch struct {
	// Number is the batch's NN ordering prefix, taken from the Batch Index
	// entry (and expected to agree with the batch filename's own NN
	// prefix — Validate's index-file-mismatch check verifies this).
	Number int

	// Slug is the batch's <batch-slug> segment, taken from the Batch Index
	// entry.
	Slug string

	// Intent is the Batch Index entry's third field — the batch's
	// stand-alone-unit one-line summary. It has exactly one source: the
	// index. The batch file's own "## Intent" section is prose for the
	// implementer and is never stored here.
	Intent string

	// File is the batch's filename (e.g. "01-json-flag.md"), relative to
	// Plan.Dir, derived from the Batch Index entry's number and slug.
	File string

	// Oversized mirrors the batch file's optional "oversized: true"
	// frontmatter key: the batch declares it needs a large-context
	// implementer.
	Oversized bool

	// VerifyDeferred mirrors the batch file's optional frontmatter
	// "verify: deferred" sentinel: this batch defers its verify: to its
	// chain's end (see ChainEnd). Mutually exclusive with a non-empty
	// VerifyCommand.
	VerifyDeferred bool

	// ChainEnd is the batch file's optional frontmatter "chain-end: NN" —
	// the number of the batch that runs the real verify: for this batch's
	// deferred-verify chain. Zero when the frontmatter key is absent.
	ChainEnd int

	// VerifyCommand is the batch file's "## verify:" body section's
	// command. Empty when the batch instead carries VerifyDeferred (or
	// when neither is present — Validate's verify-missing check flags
	// that).
	VerifyCommand string

	// Scope is the batch file's "## Scope" bullet list: plain paths
	// (prefix semantics, no globs) declaring the batch's file ownership.
	// Scope entries stay worktree-relative always — they are NOT
	// root-resolved (see Root below); a batch's declared ownership is
	// meant to read the same regardless of the root: shorthand its cards
	// happen to use.
	Scope []string

	// Root mirrors the batch file's optional frontmatter "root:" key: a
	// worktree-relative directory every card file-op path (all five
	// fields, both sides of a Moves: pair) is resolved against, unless the
	// path starts with "//" (always worktree-root-relative, root set or
	// not). Empty when the frontmatter key is absent, in which case card
	// paths are stored exactly as written (still worktree-relative). See
	// normalizeCardPath for the three-case resolution rule.
	Root string

	// Cards is every "### Card NN.C — <title>" card parsed from the batch
	// file's "## Cards" section, in document order.
	Cards []PlanCard

	// IndexCardCount is the Batch Index entry's mandatory "(C cards)"
	// segment for this batch — the Planner's own count, compared against
	// len(Cards) by Validate's card-count-mismatch check (not this
	// package's parse step).
	IndexCardCount int

	// HasRenameMechanic reports whether the batch file's body contains a
	// "## Rename mechanic" heading at all — presence only, never the
	// section's own prose (that text is for the implementer, not for
	// builderengine). Validate's move-mechanic-missing check flags a batch
	// with at least one parsed Moves: pair but HasRenameMechanic == false.
	HasRenameMechanic bool
}

// MovePair is one well-formed "Moves:" sub-bullet: a card declaring that
// Old is renamed to New (both normalized worktree-relative paths, per
// normalizeCardPath). The repo's own rename convention — "git mv" first,
// then only surgical edits — is what a card's Moves: entry declares; see
// plan-format.md's Rename mechanic text.
type MovePair struct {
	Old, New string
}

// PlanCard is one "### Card NN.C — <title>" card parsed from a batch
// file's "## Cards" section. Card-level defects (a missing field, a
// malformed Moves: bullet) are never parse errors — plan-format.md's
// lenient-card-parse discipline records them leniently here so Validate can
// turn them into enumerable findings instead of a single fail-loud error.
type PlanCard struct {
	// BatchPrefix is the "NN" the card's own heading carries — the
	// batch's zero-padded number as written in "### Card NN.C — <title>".
	// It is stored as written, not derived from the batch's own Number,
	// so Validate's card-numbering check can compare the two and flag a
	// mismatch.
	BatchPrefix int

	// Number is the "C" part of the card's heading — the card's ordinal
	// within its batch, restarting at 1 per batch.
	Number int

	// Title is the card heading's trailing title text.
	Title string

	// ContextFiles, EditsFiles, CreatesFiles, and DeletesFiles are the
	// card's four non-Moves typed file-op fields, each a normalized
	// worktree-relative path list. nil means the field's bold label was
	// never seen at all (see HasContext etc.); a present field whose
	// label line carries the literal "none" parses to an empty non-nil
	// slice — the two are deliberately distinguishable.
	ContextFiles, EditsFiles, CreatesFiles, DeletesFiles []string

	// Moves is every well-formed "- `old` -> `new`" sub-bullet under the
	// card's "**Moves:**" field, both sides normalized. Present-but-none
	// parses to an empty non-nil slice, matching the other four fields.
	Moves []MovePair

	// MovesRaw is every "**Moves:**" sub-bullet that failed the pair
	// grammar, retained verbatim (not normalized) so Validate's
	// move-format check can name the exact offending line.
	MovesRaw []string

	// HasContext, HasEdits, HasCreates, HasDeletes, and HasMoves report
	// whether the card's body carried that field's bold label at all —
	// distinct from the field being present-but-none. Validate's
	// card-missing-field check flags a card missing any of the five.
	HasContext, HasEdits, HasCreates, HasDeletes, HasMoves bool

	// HasWhat reports whether the card carried a "**What:**" label. The
	// prose itself is never stored: v1 precedent is that the Batch
	// Index's Intent is the machine-read summary and a card's What: is
	// for the implementer only, not for builderengine.
	HasWhat bool

	// Commit is the card's optional "**Commit:**" field value, with its
	// surrounding backticks stripped. Empty when the field is absent; the
	// implementer then derives the commit subject from the "NN.C: <short
	// what>" convention itself. Validate's commit-subject-mismatch check
	// flags a present value that does not start with the card's own
	// "NN.C: " prefix.
	Commit string

	// VerifyCommand is the card's optional "**verify:**" field's value,
	// taken verbatim (v1 per-card verify semantics unchanged). Empty when
	// absent.
	VerifyCommand string
}

// overviewFrontmatter mirrors 00-overview.md's frontmatter shape 1:1.
// Fields are pointers so ParsePlan can distinguish "key present with its
// zero value" from "key absent entirely" — plan-format.md requires exactly
// these two keys, so a missing key is its own fail-loud error, never a
// silently-defaulted false/zero.
type overviewFrontmatter struct {
	Format   *int  `yaml:"format"`
	Approved *bool `yaml:"approved"`
}

// indexEntry is one parsed "## Batch Index" line: the machine-readable
// fields of "NN — <batch-slug> (C cards) — <one-line intent>" before its
// named batch file has been read.
type indexEntry struct {
	Number    int
	Slug      string
	CardCount int
	Intent    string
	File      string
}

// indexLineRe matches a plan-format v2 Batch Index entry's four fields,
// accepting either the em dash "—" or one-or-two ASCII hyphens as either
// separator (plan-format.md's worked example uses "—"; hand-written plans
// may use ASCII). Both separators are required to be surrounded by
// whitespace so neither is ever confused with a hyphen inside the slug
// itself (e.g. "json-flag"). The "(C cards)" segment is mandatory —
// singular "(1 card)" is accepted — between the slug and the second
// separator; a line without it does not match at all, so it falls through
// to parseBatchIndex's fail-loud "unparseable batch index line" error, per
// the lenient-card-parse decision (this is document structure, not a
// card-level defect).
var indexLineRe = regexp.MustCompile(`^(\d+)\s+(?:—|-{1,2})\s+(\S+)\s+\((\d+)\s+cards?\)\s+(?:—|-{1,2})\s+(.+)$`)

// ParsePlan reads the plan directory planDir and returns the fully parsed
// Plan: 00-overview.md's frontmatter, framing, and Batch Index, plus every
// listed batch's own per-batch file. Every distinct failure mode — a
// missing overview file, missing or duplicate frontmatter keys, an
// unparseable Batch Index line, or a per-batch file failure — is returned
// as its own wrapped error, all prefixed "builder:" per the fail-loud,
// never-misread discipline plan-format.md pins for every machine-read plan
// artifact.
func ParsePlan(planDir string) (*Plan, error) {
	overviewPath := filepath.Join(planDir, overviewFileName)

	data, err := os.ReadFile(overviewPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("builder: plan overview not found: %s", overviewPath)
		}
		return nil, fmt.Errorf("builder: read plan overview %s: %w", overviewPath, err)
	}

	fm, body, err := parseOverviewFrontmatter(string(data), overviewPath)
	if err != nil {
		return nil, err
	}

	framing, indexLines, err := splitFraming(body)
	if err != nil {
		return nil, fmt.Errorf("builder: plan overview %s: %w", overviewPath, err)
	}

	entries, err := parseBatchIndex(indexLines)
	if err != nil {
		return nil, fmt.Errorf("builder: plan overview %s: batch index: %w", overviewPath, err)
	}

	batches := make([]PlanBatch, 0, len(entries))
	for _, entry := range entries {
		batch, err := parseBatchFile(planDir, entry)
		if err != nil {
			return nil, fmt.Errorf("builder: batch %s: %w", entry.File, err)
		}
		batches = append(batches, batch)
	}

	return &Plan{
		Dir:      planDir,
		Format:   *fm.Format,
		Approved: *fm.Approved,
		Framing:  framing,
		Batches:  batches,
	}, nil
}

// parseOverviewFrontmatter extracts and strict-decodes 00-overview.md's
// leading frontmatter block, enforcing that both format: and approved: are
// present (a missing key is a fail-loud error, not a zero-valued default).
// It returns the decoded frontmatter and the document body following the
// closing fence.
func parseOverviewFrontmatter(content, overviewPath string) (overviewFrontmatter, string, error) {
	fmBlock, body, found, err := splitFrontmatter(content)
	if err != nil {
		return overviewFrontmatter{}, "", fmt.Errorf("builder: plan overview %s: %w", overviewPath, err)
	}
	if !found {
		return overviewFrontmatter{}, "", fmt.Errorf("builder: plan overview %s: missing required frontmatter", overviewPath)
	}

	var fm overviewFrontmatter
	dec := yaml.NewDecoder(strings.NewReader(fmBlock))
	dec.KnownFields(true)
	if err := dec.Decode(&fm); err != nil {
		return overviewFrontmatter{}, "", fmt.Errorf("builder: plan overview %s: frontmatter: %w", overviewPath, err)
	}
	if fm.Format == nil {
		return overviewFrontmatter{}, "", fmt.Errorf("builder: plan overview %s: frontmatter missing required key %q", overviewPath, "format")
	}
	if fm.Approved == nil {
		return overviewFrontmatter{}, "", fmt.Errorf("builder: plan overview %s: frontmatter missing required key %q", overviewPath, "approved")
	}

	return fm, body, nil
}

// splitFrontmatter separates a leading "---"-fenced YAML block (skipping
// any blank lines before the opening fence) from the rest of a markdown
// document. found is false when the document has no frontmatter at all
// (the first non-blank line is not "---"); err is non-nil when an opening
// fence is present but never closed, which is always malformed regardless
// of whether frontmatter is optional for the caller.
func splitFrontmatter(content string) (frontmatter, body string, found bool, err error) {
	lines := strings.Split(content, "\n")

	i := 0
	for i < len(lines) && strings.TrimSpace(lines[i]) == "" {
		i++
	}
	if i >= len(lines) || strings.TrimSpace(lines[i]) != "---" {
		return "", content, false, nil
	}

	for j := i + 1; j < len(lines); j++ {
		if strings.TrimSpace(lines[j]) == "---" {
			return strings.Join(lines[i+1:j], "\n"), strings.Join(lines[j+1:], "\n"), true, nil
		}
	}
	return "", "", false, fmt.Errorf("unterminated frontmatter fence")
}

// splitFraming locates the overview body's "## Batch Index" heading and
// splits the body into the task-framing prose above it (with the document's
// H1 title line dropped, since the title is not part of the framing prose)
// and the raw index lines below it, up to the next "## " heading or EOF.
func splitFraming(body string) (framing string, indexLines []string, err error) {
	lines := strings.Split(body, "\n")

	headingIdx := -1
	for i, l := range lines {
		if strings.TrimSpace(l) == "## Batch Index" {
			headingIdx = i
			break
		}
	}
	if headingIdx == -1 {
		return "", nil, fmt.Errorf(`missing "## Batch Index" heading`)
	}

	var framingLines []string
	for _, l := range lines[:headingIdx] {
		if strings.HasPrefix(strings.TrimSpace(l), "# ") {
			// Drop the H1 title line: it identifies the plan, it is not
			// itself framing prose.
			continue
		}
		framingLines = append(framingLines, l)
	}
	framing = strings.TrimSpace(strings.Join(framingLines, "\n"))

	end := len(lines)
	for i := headingIdx + 1; i < len(lines); i++ {
		if strings.HasPrefix(strings.TrimSpace(lines[i]), "## ") {
			end = i
			break
		}
	}
	return framing, lines[headingIdx+1 : end], nil
}

// parseBatchIndex parses every non-blank Batch Index line into an
// indexEntry. Each line is expected as a markdown bullet ("- NN — slug —
// intent"); the leading bullet marker is stripped before indexLineRe is
// applied. A line that does not match the expected shape is a fail-loud
// error naming the offending line, never a silently-skipped entry.
func parseBatchIndex(lines []string) ([]indexEntry, error) {
	var entries []indexEntry
	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		line = strings.TrimSpace(strings.TrimPrefix(line, "- "))

		m := indexLineRe.FindStringSubmatch(line)
		if m == nil {
			return nil, fmt.Errorf("unparseable batch index line %q", raw)
		}

		number, err := strconv.Atoi(m[1])
		if err != nil {
			return nil, fmt.Errorf("unparseable batch index line %q: %w", raw, err)
		}
		slug := m[2]
		cardCount, err := strconv.Atoi(m[3])
		if err != nil {
			return nil, fmt.Errorf("unparseable batch index line %q: %w", raw, err)
		}

		entries = append(entries, indexEntry{
			Number:    number,
			Slug:      slug,
			CardCount: cardCount,
			Intent:    normalizeWhitespace(m[4]),
			File:      fmt.Sprintf("%02d-%s.md", number, slug),
		})
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("no batch index entries found")
	}
	return entries, nil
}

// normalizeWhitespace collapses any run of whitespace in s to a single
// space and trims the result, so a Batch Index intent copied with
// inconsistent internal spacing compares equal to its canonical form.
func normalizeWhitespace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// batchFrontmatter mirrors a batch file's optional frontmatter shape 1:1.
// Fields are pointers so an absent key is distinguishable from its
// zero/empty value: oversized: defaults to false, chain-end: defaults to
// zero, and verify: (when present) must equal the literal sentinel
// "deferred" — any other value is a fail-loud error, never silently
// ignored.
type batchFrontmatter struct {
	Oversized *bool   `yaml:"oversized"`
	Verify    *string `yaml:"verify"`
	ChainEnd  *int    `yaml:"chain-end"`
	Root      *string `yaml:"root"`
}

// verifyDeferredSentinel is the only value plan-format.md permits for a
// batch file's frontmatter verify: key.
const verifyDeferredSentinel = "deferred"

// cardsHeading, scopeHeading, and renameMechanicHeading are the exact "## "
// headings plan-format.md pins for a batch file's Scope, Cards, and Rename
// mechanic sections.
const (
	scopeHeading          = "## Scope"
	cardsHeading          = "## Cards"
	renameMechanicHeading = "## Rename mechanic"
)

// cardHeadingRe matches a "### Card NN.C — <title>" card heading inside a
// batch file's "## Cards" section, capturing the batch prefix "NN", the
// card number "C", and the trailing title. ASCII "-"/"--" is accepted
// wherever the em dash "—" is, mirroring indexLineRe's tolerance. A
// "### " line inside "## Cards" that does not match this shape is
// document structure, not a card-level defect — parseCardsSection fails
// loud rather than silently skipping it (lenient-card-parse decision).
var cardHeadingRe = regexp.MustCompile(`^###\s+Card\s+(\d{2})\.(\d+)\s*(?:—|-{1,2})\s*(.*)$`)

// moveLineRe matches a "Moves:" sub-bullet's well-formed two-path grammar,
// after its leading "- " bullet marker has already been stripped:
// "`old/path` -> `new/path`" (backtick-wrapped paths, ASCII " -> " arrow).
// A bullet that does not match is retained verbatim in PlanCard.MovesRaw
// for Validate's move-format check to flag, per lenient-card-parse.
var moveLineRe = regexp.MustCompile("^`([^`]+)` -> `([^`]+)`$")

// Bold-label prefixes for the fields plan-format v2 recognizes inside a
// card, in the field order plan-format.md pins. Matched by exact string
// prefix, mirroring v1's whereLinePrefix precedent.
const (
	whatLabel       = "**What:**"
	contextLabel    = "**Context:**"
	editsLabel      = "**Edits:**"
	createsLabel    = "**Creates:**"
	deletesLabel    = "**Deletes:**"
	movesLabel      = "**Moves:**"
	commitLabel     = "**Commit:**"
	cardVerifyLabel = "**verify:**"
)

// cardLabels lists every bold-label prefix parseCardBody recognizes, used
// by isCardLabelLine to detect where a "**What:**" prose block or a
// file-op field's bullet list ends: at the next label line (or the end of
// the card's own line range, which already stops at the next "### "
// heading).
var cardLabels = []string{
	whatLabel, contextLabel, editsLabel, createsLabel, deletesLabel,
	movesLabel, commitLabel, cardVerifyLabel,
}

// noneSentinel is the literal case-insensitive value a file-op field's
// label line carries when the field is empty: an inline "none" (rather
// than any "- `path`" bullets) yields the field's empty non-nil slice —
// present-but-empty, distinct from the field being absent altogether
// (nil slice, HasX == false).
const noneSentinel = "none"

// isCardLabelLine reports whether line (as found in a card's raw body,
// pre-trim) begins one of the card's recognized bold-label fields.
func isCardLabelLine(line string) bool {
	trimmed := strings.TrimSpace(line)
	for _, label := range cardLabels {
		if strings.HasPrefix(trimmed, label) {
			return true
		}
	}
	return false
}

// stripBackticks removes a single pair of surrounding backticks from s, if
// present, or returns s unchanged otherwise — a bullet whose payload is not
// backtick-wrapped is retained as-is, well-formedness being validator
// territory (lenient-card-parse decision).
func stripBackticks(s string) string {
	if len(s) >= 2 && strings.HasPrefix(s, "`") && strings.HasSuffix(s, "`") {
		return s[1 : len(s)-1]
	}
	return s
}

// normalizeCardPath resolves one card file-op path per the three-case rule
// (per-batch-root-path-shorthand decision): a "//"-prefixed path always
// strips exactly that prefix and is worktree-root-relative, whether or not
// root is set; otherwise, a non-empty root joins as "<root>/<raw>", except
// the degenerate "root: ." (the worktree root itself), which joins to raw
// unchanged rather than the unclean "./<raw>" a literal string join would
// produce. This is parse-time normalization of a well-formed degenerate
// case, not well-formedness rejection: an actually malformed root or raw
// (an absolute path, a ".." segment) still reaches Validate's
// scope-malformed check unchanged (normalize-at-parse decision).
func normalizeCardPath(root, raw string) string {
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, "//") {
		return strings.TrimPrefix(raw, "//")
	}
	if root != "" && root != "." {
		return root + "/" + raw
	}
	return raw
}

// parseBatchFile reads planDir's entry.File and parses it into a complete
// PlanBatch, seeded with the Batch Index fields ParsePlan already knows
// (Number, Slug, Intent, File, IndexCardCount). It decodes the file's
// optional frontmatter (including root:, needed before card paths can be
// normalized), then its "## Scope" bullet list, its "## Cards" section's
// typed cards, and finally its "## verify:" section's command — enforcing
// plan-format.md's "one or the other, never both" rule between frontmatter
// verify: deferred and a "## verify:" body section.
func parseBatchFile(planDir string, entry indexEntry) (PlanBatch, error) {
	batch := PlanBatch{
		Number:         entry.Number,
		Slug:           entry.Slug,
		Intent:         entry.Intent,
		File:           entry.File,
		IndexCardCount: entry.CardCount,
	}

	path := filepath.Join(planDir, entry.File)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return PlanBatch{}, fmt.Errorf("batch file not found: %s", path)
		}
		return PlanBatch{}, fmt.Errorf("read batch file %s: %w", path, err)
	}
	content := string(data)

	fmBlock, body, found, err := splitFrontmatter(content)
	if err != nil {
		return PlanBatch{}, fmt.Errorf("%s: %w", path, err)
	}
	if !found {
		// A batch file's frontmatter is entirely optional; splitFrontmatter
		// returns the whole document as body when there is none.
		body = content
	} else {
		if err := decodeBatchFrontmatter(fmBlock, path, &batch); err != nil {
			return PlanBatch{}, err
		}
	}

	scope, err := parseScopeSection(body)
	if err != nil {
		return PlanBatch{}, fmt.Errorf("%s: %w", path, err)
	}
	batch.Scope = scope

	batch.HasRenameMechanic = hasHeading(body, renameMechanicHeading)

	cards, err := parseCardsSection(body, batch.Root)
	if err != nil {
		return PlanBatch{}, fmt.Errorf("%s: %w", path, err)
	}
	batch.Cards = cards

	verifyCommand, hasVerifySection := parseVerifySection(body)
	if batch.VerifyDeferred && hasVerifySection {
		return PlanBatch{}, fmt.Errorf(`%s: batch has both frontmatter "verify: deferred" and a "## verify:" section; plan-format.md allows only one`, path)
	}
	batch.VerifyCommand = verifyCommand

	return batch, nil
}

// decodeBatchFrontmatter strict-decodes a batch file's frontmatter block
// into batch's frontmatter-sourced fields (Oversized, VerifyDeferred,
// ChainEnd, Root), rejecting any verify: value other than the "deferred"
// sentinel.
func decodeBatchFrontmatter(fmBlock, path string, batch *PlanBatch) error {
	var fm batchFrontmatter
	dec := yaml.NewDecoder(strings.NewReader(fmBlock))
	dec.KnownFields(true)
	if err := dec.Decode(&fm); err != nil {
		return fmt.Errorf("%s: frontmatter: %w", path, err)
	}

	if fm.Oversized != nil {
		batch.Oversized = *fm.Oversized
	}
	if fm.Verify != nil {
		if *fm.Verify != verifyDeferredSentinel {
			return fmt.Errorf("%s: frontmatter verify: %q is not a recognized sentinel (only %q)", path, *fm.Verify, verifyDeferredSentinel)
		}
		batch.VerifyDeferred = true
	}
	if fm.ChainEnd != nil {
		batch.ChainEnd = *fm.ChainEnd
	}
	if fm.Root != nil {
		batch.Root = *fm.Root
	}
	return nil
}

// extractSection returns the lines strictly between an exact heading match
// (trimmed equality) and the next "## " heading or EOF, or nil when heading
// is not present at all. Because the match requires a two-hash prefix, a
// "### Card N" sub-heading inside "## Cards" never terminates that section.
func extractSection(body, heading string) []string {
	lines := strings.Split(body, "\n")

	start := -1
	for i, l := range lines {
		if strings.TrimSpace(l) == heading {
			start = i + 1
			break
		}
	}
	if start == -1 {
		return nil
	}

	end := len(lines)
	for i := start; i < len(lines); i++ {
		if strings.HasPrefix(strings.TrimSpace(lines[i]), "## ") {
			end = i
			break
		}
	}
	return lines[start:end]
}

// hasHeading reports whether body contains a line matching heading by the
// same trimmed-line equality extractSection uses to find a heading's start —
// consulted where only a heading's presence matters, not its section body
// (PlanBatch.HasRenameMechanic).
func hasHeading(body, heading string) bool {
	for _, l := range strings.Split(body, "\n") {
		if strings.TrimSpace(l) == heading {
			return true
		}
	}
	return false
}

// parseScopeSection parses a batch file's "## Scope" bullet list into a
// plain path list, per plan-format.md's "prefix semantics, no globs" rule:
// any entry containing "*" is a fail-loud error rather than a silently
// accepted glob the rest of builder cannot mechanically check. A batch
// file with no "## Scope" section yields a nil slice, not an error — scope
// well-formedness (empty, absolute, or ".."-escaping entries) is
// Validate's scope-malformed check, not a parse-time rejection.
func parseScopeSection(body string) ([]string, error) {
	section := extractSection(body, scopeHeading)
	if section == nil {
		return nil, nil
	}

	var scope []string
	for _, raw := range section {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		line = strings.TrimSpace(strings.TrimPrefix(line, "- "))
		if line == "" {
			continue
		}
		if strings.Contains(line, "*") {
			return nil, fmt.Errorf("scope entry %q contains a glob character; plan-format scope is a plain path list", line)
		}
		scope = append(scope, line)
	}
	return scope, nil
}

// parseCardsSection splits a batch file's "## Cards" section at "### Card
// NN.C — <title>" headings and parses each into a PlanCard, resolving
// every card file-op path against root (batch.Root, empty when the batch
// carries no root: frontmatter). A batch file with no "## Cards" section
// yields a nil slice, not an error. A "### " line inside the section that
// does not match cardHeadingRe's shape is document structure, not a
// card-level defect, and fails loud naming the offending line
// (lenient-card-parse decision).
func parseCardsSection(body, root string) ([]PlanCard, error) {
	section := extractSection(body, cardsHeading)
	if section == nil {
		return nil, nil
	}

	var cards []PlanCard
	i := 0
	for i < len(section) {
		trimmed := strings.TrimSpace(section[i])
		if !strings.HasPrefix(trimmed, "### ") {
			// Blank lines and any other prose between cards (or before the
			// first one) are not structurally significant.
			i++
			continue
		}

		m := cardHeadingRe.FindStringSubmatch(trimmed)
		if m == nil {
			return nil, fmt.Errorf("unrecognized heading inside %q: %q", cardsHeading, trimmed)
		}

		end := i + 1
		for end < len(section) && !strings.HasPrefix(strings.TrimSpace(section[end]), "### ") {
			end++
		}

		card, err := parseCardBody(m, section[i+1:end], root)
		if err != nil {
			return nil, err
		}
		cards = append(cards, card)
		i = end
	}
	return cards, nil
}

// parseCardBody parses one card's body lines (the lines strictly between
// its own "### Card NN.C — <title>" heading and the next card heading or
// the end of the "## Cards" section) into a PlanCard. headingMatch is
// cardHeadingRe's submatch against the card's heading line. Card-level
// defects (a missing field, a malformed Moves: bullet) are recorded
// leniently, never returned as an error — only Validate turns them into
// findings (lenient-card-parse decision).
func parseCardBody(headingMatch []string, lines []string, root string) (PlanCard, error) {
	batchPrefix, err := strconv.Atoi(headingMatch[1])
	if err != nil {
		return PlanCard{}, fmt.Errorf("card heading %q: %w", headingMatch[0], err)
	}
	number, err := strconv.Atoi(headingMatch[2])
	if err != nil {
		return PlanCard{}, fmt.Errorf("card heading %q: %w", headingMatch[0], err)
	}

	card := PlanCard{
		BatchPrefix: batchPrefix,
		Number:      number,
		Title:       strings.TrimSpace(headingMatch[3]),
	}

	i := 0
	for i < len(lines) {
		trimmed := strings.TrimSpace(lines[i])
		var fieldErr error
		switch {
		case trimmed == "":
			i++
		case strings.HasPrefix(trimmed, whatLabel):
			card.HasWhat = true
			i++
			for i < len(lines) && !isCardLabelLine(lines[i]) {
				i++
			}
		case strings.HasPrefix(trimmed, contextLabel):
			card.HasContext = true
			card.ContextFiles, i, fieldErr = parseFileOpField(trimmed, contextLabel, root, lines, i+1)
		case strings.HasPrefix(trimmed, editsLabel):
			card.HasEdits = true
			card.EditsFiles, i, fieldErr = parseFileOpField(trimmed, editsLabel, root, lines, i+1)
		case strings.HasPrefix(trimmed, createsLabel):
			card.HasCreates = true
			card.CreatesFiles, i, fieldErr = parseFileOpField(trimmed, createsLabel, root, lines, i+1)
		case strings.HasPrefix(trimmed, deletesLabel):
			card.HasDeletes = true
			card.DeletesFiles, i, fieldErr = parseFileOpField(trimmed, deletesLabel, root, lines, i+1)
		case strings.HasPrefix(trimmed, movesLabel):
			card.HasMoves = true
			card.Moves, card.MovesRaw, i, fieldErr = parseMovesField(trimmed, root, lines, i+1)
		case strings.HasPrefix(trimmed, commitLabel):
			card.Commit = stripBackticks(strings.TrimSpace(strings.TrimPrefix(trimmed, commitLabel)))
			i++
		case strings.HasPrefix(trimmed, cardVerifyLabel):
			card.VerifyCommand = strings.TrimSpace(strings.TrimPrefix(trimmed, cardVerifyLabel))
			i++
		default:
			// Any other line (stray prose outside a recognized field) is
			// not structurally significant — card-level content beyond the
			// pinned grammar is not this parser's concern.
			i++
		}
		if fieldErr != nil {
			return PlanCard{}, fmt.Errorf("card %02d.%d: %w", card.BatchPrefix, card.Number, fieldErr)
		}
	}

	return card, nil
}

// parseFileOpField parses one of a card's four non-Moves file-op fields
// (Context/Edits/Creates/Deletes): labelLine is the field's own
// "**Label:** ..." line, label is its exact bold-label prefix, and lines
// starting at start are the remaining card body lines to scan for the
// field's "- `path`" bullets. Returns the field's normalized path list
// (empty non-nil for an inline "none", nil if no bullets followed a
// non-none label) and the index of the first line not consumed. A non-empty
// label-line value other than the "none" sentinel (e.g. an inline path) is
// a fail-loud error, not a card-level finding: silently reading it as an
// empty field would be exactly the silent degradation the none-sentinel
// grammar exists to prevent — the field would look present to every check
// while its paths vanished from validation and the context estimate. This
// is document structure, the same class as parseScopeSection's glob
// rejection, so it fails at parse time rather than waiting for Validate.
func parseFileOpField(labelLine, label, root string, lines []string, start int) ([]string, int, error) {
	rest := strings.TrimSpace(strings.TrimPrefix(labelLine, label))
	if strings.EqualFold(rest, noneSentinel) {
		return []string{}, start, nil
	}
	if rest != "" {
		return nil, start, fmt.Errorf("card field %s carries an inline value %q; plan-format admits only the literal \"none\" or \"- `path`\" sub-bullets on the following lines", label, rest)
	}

	var files []string
	i := start
	for i < len(lines) {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed == "" {
			i++
			continue
		}
		if isCardLabelLine(lines[i]) || !strings.HasPrefix(trimmed, "- ") {
			break
		}
		payload := stripBackticks(strings.TrimSpace(strings.TrimPrefix(trimmed, "- ")))
		files = append(files, normalizeCardPath(root, payload))
		i++
	}
	return files, i, nil
}

// parseMovesField parses a card's "**Moves:**" field the same way
// parseFileOpField parses the other four, except each bullet is matched
// against moveLineRe: a well-formed "`old` -> `new`" bullet becomes a
// normalized MovePair, and any other bullet is retained verbatim (not
// normalized) in raw for Validate's move-format check.
func parseMovesField(labelLine, root string, lines []string, start int) (pairs []MovePair, raw []string, next int, err error) {
	rest := strings.TrimSpace(strings.TrimPrefix(labelLine, movesLabel))
	if strings.EqualFold(rest, noneSentinel) {
		return []MovePair{}, nil, start, nil
	}
	// Same inline-value rejection as parseFileOpField: an inline pair would
	// silently vanish from move validation and the rename mechanics.
	if rest != "" {
		return nil, nil, start, fmt.Errorf("card field %s carries an inline value %q; plan-format admits only the literal \"none\" or \"- `src` -> `dst`\" sub-bullets on the following lines", movesLabel, rest)
	}

	i := start
	for i < len(lines) {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed == "" {
			i++
			continue
		}
		if isCardLabelLine(lines[i]) || !strings.HasPrefix(trimmed, "- ") {
			break
		}
		payload := strings.TrimSpace(strings.TrimPrefix(trimmed, "- "))
		if m := moveLineRe.FindStringSubmatch(payload); m != nil {
			pairs = append(pairs, MovePair{Old: normalizeCardPath(root, m[1]), New: normalizeCardPath(root, m[2])})
		} else {
			raw = append(raw, payload)
		}
		i++
	}
	return pairs, raw, i, nil
}

// parseVerifySection parses a batch file's "## verify:" section, returning
// its first non-empty line as the command. hasSection reports whether the
// heading itself was present at all, distinct from an empty command — that
// distinction is what lets parseBatchFile enforce plan-format.md's "one or
// the other, never both" rule against VerifyDeferred.
func parseVerifySection(body string) (command string, hasSection bool) {
	section := extractSection(body, "## verify:")
	if section == nil {
		return "", false
	}
	for _, raw := range section {
		line := strings.TrimSpace(raw)
		if line != "" {
			return line, true
		}
	}
	return "", true
}
