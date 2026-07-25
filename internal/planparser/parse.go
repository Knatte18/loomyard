// parse.go implements ParsePlan: it reads a plan directory's 00-overview.md (scalar
// frontmatter + task framing + Card Index) and, for each card the index lists, that
// card's own NN-<card-slug>.md file, producing the in-memory Plan the rest of webster
// drives from. Every distinct parse failure is a "planparser:"-prefixed wrapped
// error — plan-format-v3.md's fail-loud discipline admits no silent-default reading
// of a malformed plan document structure. Per-card content defects (a missing field,
// a malformed Moves: bullet) are recorded leniently into the Card model instead,
// per the lenient-card-parse decision documented in doc.go.

package planparser

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// overviewFileName is the fixed filename of a plan's overview file within its plan
// directory, per plan-format-v3.md's on-disk layout.
const overviewFileName = "00-overview.md"

// cardIndexHeading is the exact "## " heading plan-format-v3.md pins for the
// overview's Card Index section.
const cardIndexHeading = "## Card Index"

// overviewFrontmatter mirrors 00-overview.md's frontmatter shape 1:1. Fields are
// pointers so ParsePlan can distinguish "key present with its zero value" from "key
// absent entirely": plan-format-v3.md's format-unrecognized/plan-unapproved checks
// (Validate's job, not ParsePlan's) need that distinction to tell an explicit
// `format: 0` apart from a plan that never set format: at all, and both fold to the
// same zero value once dereferenced onto Plan.
type overviewFrontmatter struct {
	Format   *int    `yaml:"format"`
	Approved *bool   `yaml:"approved"`
	Root     *string `yaml:"root"`
}

// cardIndexEntry is one parsed "## Card Index" line: the machine-readable fields of
// "N — <card-slug> — <one-line intent>" before its own card file has been read.
type cardIndexEntry struct {
	Number int
	Slug   string
	Intent string
}

// cardIndexLineRe matches a plan-format-v3 Card Index entry's three fields,
// accepting either the em dash "—" or one-or-two ASCII hyphens as either separator
// (plan-format-v3.md's worked example uses "—"; hand-written plans may use ASCII).
// Both separators are required to be surrounded by whitespace so neither is ever
// confused with a hyphen inside the slug itself (e.g. "json-flag"). A line that does
// not match this shape at all is document structure, not a card-level defect — it
// fails loud rather than being silently skipped, per the lenient-card-parse
// decision (that leniency applies to per-card body content, not to the index).
var cardIndexLineRe = regexp.MustCompile(`^(\d+)\s+(?:—|-{1,2})\s+(\S+)\s+(?:—|-{1,2})\s+(.+)$`)

// ParsePlan reads the plan directory planDir and returns the fully parsed Plan:
// 00-overview.md's frontmatter, framing, Card Index, and plan-level body sections,
// plus every listed card's own per-card file. planDir is an explicit argument;
// ParsePlan never constructs the "_lyx"/"plan" path tokens itself — the caller
// resolves that (via hubgeometry.PlanDir), per the Hub Geometry Invariant. Every
// distinct failure mode — a missing overview file, undecodable frontmatter, an
// unparseable Card Index line, a missing or malformed card file — is returned as its
// own wrapped error, all prefixed "planparser:" per the fail-loud, never-misread
// discipline plan-format-v3.md pins for every machine-read plan artifact.
func ParsePlan(planDir string) (*Plan, error) {
	overviewPath := filepath.Join(planDir, overviewFileName)

	data, err := os.ReadFile(overviewPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("planparser: plan overview not found: %s", overviewPath)
		}
		return nil, fmt.Errorf("planparser: read plan overview %s: %w", overviewPath, err)
	}

	fm, body, err := parseOverviewFrontmatter(string(data), overviewPath)
	if err != nil {
		return nil, err
	}

	framing, indexLines, err := splitFraming(body)
	if err != nil {
		return nil, fmt.Errorf("planparser: plan overview %s: %w", overviewPath, err)
	}

	entries, err := parseCardIndex(indexLines)
	if err != nil {
		return nil, fmt.Errorf("planparser: plan overview %s: card index: %w", overviewPath, err)
	}

	root := ""
	if fm.Root != nil {
		root = *fm.Root
	}

	cards := make([]Card, 0, len(entries))
	for _, entry := range entries {
		card, err := parseCardFile(planDir, entry)
		if err != nil {
			return nil, err
		}
		// Resolve every card path's root:/// shorthand exactly once, here, so
		// every downstream consumer (Validate included) only ever sees plain,
		// normalized, worktree-relative paths.
		normalizeCard(&card, root)
		cards = append(cards, card)
	}

	plan := &Plan{
		Dir:     planDir,
		Framing: framing,
		Cards:   cards,
		Root:    root,
	}
	if fm.Format != nil {
		plan.Format = *fm.Format
	}
	if fm.Approved != nil {
		plan.Approved = *fm.Approved
	}

	return plan, nil
}

// parseOverviewFrontmatter extracts and strict-decodes 00-overview.md's leading
// frontmatter block. Unlike the frozen v2 parser, a missing format:/approved:/root:
// key is not itself a fail-loud error here — format-unrecognized and
// plan-unapproved are Validate's checks, not ParsePlan's; only a missing
// frontmatter block entirely, or one that fails to decode (unknown key, duplicate
// key, unterminated fence), is document structure. It returns the decoded
// frontmatter and the document body following the closing fence.
func parseOverviewFrontmatter(content, overviewPath string) (overviewFrontmatter, string, error) {
	fmBlock, body, found, err := splitFrontmatter(content)
	if err != nil {
		return overviewFrontmatter{}, "", fmt.Errorf("planparser: plan overview %s: %w", overviewPath, err)
	}
	if !found {
		return overviewFrontmatter{}, "", fmt.Errorf("planparser: plan overview %s: missing required frontmatter", overviewPath)
	}

	var fm overviewFrontmatter
	dec := yaml.NewDecoder(strings.NewReader(fmBlock))
	dec.KnownFields(true)
	if err := dec.Decode(&fm); err != nil {
		return overviewFrontmatter{}, "", fmt.Errorf("planparser: plan overview %s: frontmatter: %w", overviewPath, err)
	}

	return fm, body, nil
}

// splitFrontmatter separates a leading "---"-fenced YAML block (skipping any blank
// lines before the opening fence) from the rest of a markdown document. found is
// false when the document has no frontmatter at all (the first non-blank line is
// not "---"); err is non-nil when an opening fence is present but never closed,
// which is always malformed regardless of whether frontmatter is optional for the
// caller.
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

// splitFraming locates the overview body's "## Card Index" heading and splits the
// body into the task-framing prose above it (with the document's H1 title line
// dropped, since the title is not part of the framing prose) and the raw index
// lines below it, up to the next "## " heading or EOF.
func splitFraming(body string) (framing string, indexLines []string, err error) {
	lines := strings.Split(body, "\n")

	headingIdx := -1
	for i, l := range lines {
		if strings.TrimSpace(l) == cardIndexHeading {
			headingIdx = i
			break
		}
	}
	if headingIdx == -1 {
		return "", nil, fmt.Errorf(`missing %q heading`, cardIndexHeading)
	}

	var framingLines []string
	for _, l := range lines[:headingIdx] {
		if strings.HasPrefix(strings.TrimSpace(l), "# ") {
			// Drop the H1 title line: it identifies the plan, it is not itself
			// framing prose.
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

// parseCardIndex parses every non-blank Card Index line into a cardIndexEntry. Each
// line may carry an optional leading markdown bullet marker ("- N — slug — intent"),
// stripped before cardIndexLineRe is applied; plan-format-v3.md's own worked example
// carries no bullet marker at all, so both forms are accepted. A line that does not
// match the expected shape is a fail-loud error naming the offending line, never a
// silently-skipped entry.
func parseCardIndex(lines []string) ([]cardIndexEntry, error) {
	var entries []cardIndexEntry
	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		line = strings.TrimSpace(strings.TrimPrefix(line, "- "))

		m := cardIndexLineRe.FindStringSubmatch(line)
		if m == nil {
			return nil, fmt.Errorf("unparseable card index line %q", raw)
		}

		number, err := strconv.Atoi(m[1])
		if err != nil {
			return nil, fmt.Errorf("unparseable card index line %q: %w", raw, err)
		}

		entries = append(entries, cardIndexEntry{
			Number: number,
			Slug:   m[2],
			Intent: normalizeWhitespace(m[3]),
		})
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("no card index entries found")
	}
	return entries, nil
}

// normalizeWhitespace collapses any run of whitespace in s to a single space and
// trims the result, so a Card Index intent copied with inconsistent internal
// spacing compares equal to its canonical form.
func normalizeWhitespace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// cardFileName returns the on-disk filename a Card Index entry's card file must
// carry: its zero-padded number and slug, per plan-format-v3.md's on-disk layout.
func cardFileName(number int, slug string) string {
	return fmt.Sprintf("%02d-%s.md", number, slug)
}

// cardHeadingRe matches a card file's own "# Card N — <name>" title heading,
// capturing the card's own number and its trailing name. ASCII "-"/"--" is
// accepted wherever the em dash "—" is, mirroring cardIndexLineRe's tolerance. A
// first line that does not match this shape at all is document structure, not a
// card-level defect — parseCardFile fails loud rather than silently guessing a
// title (lenient-card-parse decision, which applies to a card's typed fields, not
// to its own heading).
var cardHeadingRe = regexp.MustCompile(`^#\s+Card\s+(\d+)\s*(?:—|-{1,2})\s*(.*)$`)

// parseCardFile reads planDir's card file for entry (per cardFileName) and parses
// its title heading, seeding the returned Card with the Card Index fields ParsePlan
// already knows (Number, Slug, Intent). The remainder of the card body (What:, the
// five typed file-op fields, Depends-on, Commit, verify:) is parsed by
// parseCardBody, added in a later revision of this file.
func parseCardFile(planDir string, entry cardIndexEntry) (Card, error) {
	card := Card{
		Number: entry.Number,
		Slug:   entry.Slug,
		Intent: entry.Intent,
	}

	fileName := cardFileName(entry.Number, entry.Slug)
	path := filepath.Join(planDir, fileName)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Card{}, fmt.Errorf("planparser: card file not found: %s", path)
		}
		return Card{}, fmt.Errorf("planparser: read card file %s: %w", path, err)
	}

	lines := strings.Split(string(data), "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) == "" {
		return Card{}, fmt.Errorf("planparser: card file %s: missing card heading", path)
	}

	headingLine := strings.TrimSpace(lines[0])
	m := cardHeadingRe.FindStringSubmatch(headingLine)
	if m == nil {
		return Card{}, fmt.Errorf("planparser: card file %s: unrecognized card heading %q", path, headingLine)
	}
	// The heading's own number is cross-checked against the Card Index's number
	// only by Validate's card-numbering check — a mismatch here is a card-level
	// defect, never a parse failure, per the lenient-card-parse decision.
	card.Title = strings.TrimSpace(m[2])

	if err := parseCardBody(&card, lines[1:]); err != nil {
		return Card{}, fmt.Errorf("planparser: card file %s: %w", path, err)
	}

	return card, nil
}

// Bold-label prefixes for the fields plan-format-v3 recognizes inside a card, in
// the field order plan-format-v3.md pins.
const (
	whatLabel       = "**What:**"
	contextLabel    = "**Context:**"
	editsLabel      = "**Edits:**"
	createsLabel    = "**Creates:**"
	deletesLabel    = "**Deletes:**"
	movesLabel      = "**Moves:**"
	dependsOnLabel  = "**Depends-on:**"
	commitLabel     = "**Commit:**"
	cardVerifyLabel = "**verify:**"
)

// cardLabels lists every bold-label prefix parseCardBody recognizes, used by
// isCardLabelLine to detect where a "**What:**" prose block or a file-op field's
// bullet list ends: at the next label line, or the end of the card file.
var cardLabels = []string{
	whatLabel, contextLabel, editsLabel, createsLabel, deletesLabel,
	movesLabel, dependsOnLabel, commitLabel, cardVerifyLabel,
}

// noneSentinel is the literal case-insensitive value a field's label line carries
// when the field is empty: an inline "none" (rather than any "- `path`" bullets, or
// any Depends-on ids) yields the field's empty non-nil slice — present-but-empty,
// distinct from the field being absent altogether (nil slice, HasX false).
const noneSentinel = "none"

// isCardLabelLine reports whether line (as found in a card's raw body, pre-trim)
// begins one of the card's recognized bold-label fields.
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
// backtick-wrapped is retained as-is, well-formedness being validator territory
// (lenient-card-parse decision).
func stripBackticks(s string) string {
	if len(s) >= 2 && strings.HasPrefix(s, "`") && strings.HasSuffix(s, "`") {
		return s[1 : len(s)-1]
	}
	return s
}

// dependsOnSplitRe splits a "**Depends-on:**" inline value into its individual
// card-id tokens: a run of one or more commas and/or whitespace, so both
// "1, 3" and "1 3" style lists parse identically.
var dependsOnSplitRe = regexp.MustCompile(`[,\s]+`)

// moveLineRe matches a "Moves:" sub-bullet's well-formed two-path grammar, after
// its leading "- " bullet marker has already been stripped: "`old/path` ->
// `new/path`" (backtick-wrapped paths, ASCII " -> " arrow). A bullet that does not
// match is retained verbatim in Card.MovesRaw for Validate's move-format check to
// flag, per the lenient-card-parse decision.
var moveLineRe = regexp.MustCompile("^`([^`]+)` -> `([^`]+)`$")

// parseCardBody parses lines — everything in a card file after its own title
// heading — into card's remaining fields: What: presence, the five typed file-op
// fields (raw, un-normalized paths; normalizeCard applies root:/// resolution in a
// later pass), Depends-on, and the optional Commit and verify: fields. Card-level
// defects (a missing field, a malformed Moves: bullet, a Depends-on id that isn't
// an integer) are recorded leniently — via the HasX bits, MovesRaw, or simply left
// for Validate to enumerate — never returned as an error; parseCardBody fails loud
// only on document structure it cannot mechanically read at all: an inline value on
// a field that admits only "none" or a bullet/id list.
func parseCardBody(card *Card, lines []string) error {
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
			card.ContextFiles, i, fieldErr = parseFileOpField(trimmed, contextLabel, lines, i+1)
		case strings.HasPrefix(trimmed, editsLabel):
			card.HasEdits = true
			card.EditsFiles, i, fieldErr = parseFileOpField(trimmed, editsLabel, lines, i+1)
		case strings.HasPrefix(trimmed, createsLabel):
			card.HasCreates = true
			card.CreatesFiles, i, fieldErr = parseFileOpField(trimmed, createsLabel, lines, i+1)
		case strings.HasPrefix(trimmed, deletesLabel):
			card.HasDeletes = true
			card.DeletesFiles, i, fieldErr = parseFileOpField(trimmed, deletesLabel, lines, i+1)
		case strings.HasPrefix(trimmed, movesLabel):
			card.HasMoves = true
			card.Moves, card.MovesRaw, i, fieldErr = parseMovesField(trimmed, lines, i+1)
		case strings.HasPrefix(trimmed, dependsOnLabel):
			card.HasDependsOn = true
			card.DependsOn, fieldErr = parseDependsOnField(trimmed)
			i++
		case strings.HasPrefix(trimmed, commitLabel):
			card.Commit = stripBackticks(strings.TrimSpace(strings.TrimPrefix(trimmed, commitLabel)))
			i++
		case strings.HasPrefix(trimmed, cardVerifyLabel):
			card.Verify = strings.TrimSpace(strings.TrimPrefix(trimmed, cardVerifyLabel))
			i++
		default:
			// Any other line (stray prose outside a recognized field) is not
			// structurally significant — card-level content beyond the pinned
			// grammar is not this parser's concern.
			i++
		}
		if fieldErr != nil {
			return fieldErr
		}
	}
	return nil
}

// parseFileOpField parses one of a card's four non-Moves file-op fields
// (Context/Edits/Creates/Deletes): labelLine is the field's own "**Label:** ..."
// line, label is its exact bold-label prefix, and lines starting at start are the
// remaining card body lines to scan for the field's "- `path`" bullets. Returns the
// field's raw path list (empty non-nil for an inline "none", nil if no bullets
// followed a non-none label) and the index of the first line not consumed. A
// non-empty label-line value other than the "none" sentinel (e.g. an inline path)
// is a fail-loud error, not a card-level finding: silently reading it as an empty
// field would be exactly the silent degradation the none-sentinel grammar exists to
// prevent — the field would look present to every check while its paths vanished
// from validation. This is document structure, so it fails at parse time rather
// than waiting for Validate.
func parseFileOpField(labelLine, label string, lines []string, start int) ([]string, int, error) {
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
		files = append(files, payload)
		i++
	}
	return files, i, nil
}

// parseMovesField parses a card's "**Moves:**" field the same way parseFileOpField
// parses the other four, except each bullet is matched against moveLineRe: a
// well-formed "`old` -> `new`" bullet becomes a raw MovePair (un-normalized;
// normalizeCard resolves both sides in a later pass), and any other bullet is
// retained verbatim in raw for Validate's move-format check.
func parseMovesField(labelLine string, lines []string, start int) (pairs []MovePair, raw []string, next int, err error) {
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
			pairs = append(pairs, MovePair{Old: m[1], New: m[2]})
		} else {
			raw = append(raw, payload)
		}
		i++
	}
	return pairs, raw, i, nil
}

// parseDependsOnField parses a card's inline "**Depends-on:**" value: the literal
// "none" (case-insensitive) yields an empty non-nil slice, and otherwise the value
// is split into comma-and/or-whitespace-separated tokens, each parsed as a plain
// card number. A token that fails to parse as an integer is document structure —
// the field's grammar admits nothing else — so it is a fail-loud error rather than
// a card-level finding; whether a well-formed id actually names an existing,
// earlier card is Validate's depends-on-order check, not this function's concern.
func parseDependsOnField(labelLine string) ([]int, error) {
	rest := strings.TrimSpace(strings.TrimPrefix(labelLine, dependsOnLabel))
	if strings.EqualFold(rest, noneSentinel) {
		return []int{}, nil
	}
	if rest == "" {
		return nil, fmt.Errorf("card field %s carries no value; plan-format admits only the literal \"none\" or a list of card numbers", dependsOnLabel)
	}

	var ids []int
	for _, tok := range dependsOnSplitRe.Split(rest, -1) {
		if tok == "" {
			continue
		}
		id, err := strconv.Atoi(tok)
		if err != nil {
			return nil, fmt.Errorf("card field %s: %q is not a plain card number: %w", dependsOnLabel, tok, err)
		}
		ids = append(ids, id)
	}
	if ids == nil {
		ids = []int{}
	}
	return ids, nil
}
