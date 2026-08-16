// parse.go implements ParsePlan: it reads a plan directory's 00-overview.md (scalar frontmatter +
// task framing + Card Index) and, for each card the index lists, that card's own NN-<card-slug>.md
// file, producing the in-memory Plan the rest of webster drives from.
// Every distinct parse failure is a "planparser:"-prefixed wrapped error — loom-plan-spec.md's
// fail-loud discipline admits no silent-default reading of a malformed plan document structure.
// Per-card content defects (a missing field, a malformed Moves: bullet) are recorded leniently into
// the Card model instead, per the lenient-card-parse decision documented in doc.go.

package planparser

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/Knatte18/loomyard/internal/lyxdirs"
	"gopkg.in/yaml.v3"
)

// overviewFileName is the fixed filename of a plan's overview file, per loom-plan-spec.md's on-disk layout.
const overviewFileName = "00-overview.md"

// PlanDirName is the relative-path segment planparser joins onto lyxdirs.LyxDirName to form
// the plan directory's worktree-relative token.
// planparser is this segment's sole declarer, per the module-owned-constructors per-segment join
// rule.
const PlanDirName = "plan"

// PlanDirRel returns the worktree-relative plan-directory token, `_lyx/plan`.
// Callers use this for relative plan-file pointers (e.g.
// this file's own Card.SourcePath stamping below).
// It uses the stdlib path package so the token is always forward-slash, never OS-dependent.
func PlanDirRel() string {
	return path.Join(lyxdirs.LyxDirName, PlanDirName)
}

// cardIndexHeading is the exact "## " heading loom-plan-spec.md pins for the overview's Card Index section.
const cardIndexHeading = "## Card Index"

// overviewFrontmatter mirrors 00-overview.md's frontmatter shape 1:1 with pointer fields to distinguish absent vs zero-value keys.
type overviewFrontmatter struct {
	Format   *int    `yaml:"format"`
	Approved *bool   `yaml:"approved"`
	Root     *string `yaml:"root"`
}

// cardIndexEntry is one parsed "## Card Index" line's machine-readable fields before the card file is read.
type cardIndexEntry struct {
	Number int
	Slug   string
	Intent string
}

// cardIndexLineRe matches a plan-format Card Index entry's three fields, accepting either the em dash "—" or ASCII hyphens as separators.
var cardIndexLineRe = regexp.MustCompile(`^(\d+)\s+(?:—|-{1,2})\s+(\S+)\s+(?:—|-{1,2})\s+(.+)$`)

// ParsePlan reads the plan directory and returns the fully parsed Plan.
// It returns wrapped errors prefixed "planparser:" for every distinct failure mode.
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
		// Resolve every card path's root:/// shorthand exactly once so every downstream consumer sees normalized paths.
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
	extractPlanSections(plan, body)

	return plan, nil
}

// parseOverviewFrontmatter extracts and strict-decodes 00-overview.md's leading frontmatter block. It returns the decoded frontmatter and the document body following the closing fence.
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

// splitFrontmatter separates a leading "---"-fenced YAML block from the rest of a markdown document.
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

// splitFraming locates the "## Card Index" heading and splits the body into framing prose above it and index lines below it.
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

// parseCardIndex parses every non-blank Card Index line into a cardIndexEntry, accepting optional leading bullet markers.
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

// normalizeWhitespace collapses any run of whitespace in s to a single space.
func normalizeWhitespace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// cardFileName returns the on-disk filename a Card Index entry's card file must carry.
func cardFileName(number int, slug string) string {
	return fmt.Sprintf("%02d-%s.md", number, slug)
}

// cardHeadingRe matches a card file's "# Card N — <name>" title heading, accepting em dash or ASCII hyphens.
var cardHeadingRe = regexp.MustCompile(`^#\s+Card\s+(\d+)\s*(?:—|-{1,2})\s*(.*)$`)

// parseCardFile reads planDir's card file for entry and parses its title heading, seeding the returned Card with Card Index fields.
func parseCardFile(planDir string, entry cardIndexEntry) (Card, error) {
	fileName := cardFileName(entry.Number, entry.Slug)

	card := Card{
		Number: entry.Number,
		Slug:   entry.Slug,
		Intent: entry.Intent,
		// SourcePath is built from this package's own PlanDirRel (the `_lyx/plan` segment) joined with fileName.
		SourcePath: path.Join(PlanDirRel(), fileName),
	}

	filePath := filepath.Join(planDir, fileName)
	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return Card{}, fmt.Errorf("planparser: card file not found: %s", filePath)
		}
		return Card{}, fmt.Errorf("planparser: read card file %s: %w", filePath, err)
	}

	lines := strings.Split(string(data), "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) == "" {
		return Card{}, fmt.Errorf("planparser: card file %s: missing card heading", filePath)
	}

	headingLine := strings.TrimSpace(lines[0])
	m := cardHeadingRe.FindStringSubmatch(headingLine)
	if m == nil {
		return Card{}, fmt.Errorf("planparser: card file %s: unrecognized card heading %q", filePath, headingLine)
	}
	card.Title = strings.TrimSpace(m[2])

	if err := parseCardBody(&card, lines[1:]); err != nil {
		return Card{}, fmt.Errorf("planparser: card file %s: %w", filePath, err)
	}

	return card, nil
}

// Bold-label prefixes for the fields plan-format recognizes inside a card.
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

// cardLabels lists every bold-label prefix parseCardBody recognizes.
var cardLabels = []string{
	whatLabel, contextLabel, editsLabel, createsLabel, deletesLabel,
	movesLabel, dependsOnLabel, commitLabel, cardVerifyLabel,
}

// noneSentinel is the literal case-insensitive value a field's label line carries when the field is empty.
const noneSentinel = "none"

// isCardLabelLine reports whether line begins one of the card's recognized bold-label fields.
func isCardLabelLine(line string) bool {
	trimmed := strings.TrimSpace(line)
	for _, label := range cardLabels {
		if strings.HasPrefix(trimmed, label) {
			return true
		}
	}
	return false
}

// stripBackticks removes a single pair of surrounding backticks from s, if present, or returns s unchanged otherwise.
func stripBackticks(s string) string {
	if len(s) >= 2 && strings.HasPrefix(s, "`") && strings.HasSuffix(s, "`") {
		return s[1 : len(s)-1]
	}
	return s
}

// dependsOnSplitRe splits a "**Depends-on:**" inline value into card-id tokens.
var dependsOnSplitRe = regexp.MustCompile(`[,\s]+`)

// moveLineRe matches a "Moves:" sub-bullet's well-formed two-path grammar after the leading "- " bullet marker.
var moveLineRe = regexp.MustCompile("^`([^`]+)` -> `([^`]+)`$")

// parseCardBody parses lines after the card title into card's remaining fields.
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
			// Collect the prose: the label line's own remainder plus every following line up to the next field label.
			proseLines := []string{strings.TrimSpace(strings.TrimPrefix(trimmed, whatLabel))}
			i++
			for i < len(lines) && !isCardLabelLine(lines[i]) {
				proseLines = append(proseLines, strings.TrimSpace(lines[i]))
				i++
			}
			card.What = strings.TrimSpace(strings.Join(proseLines, "\n"))
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
			i++
		}
		if fieldErr != nil {
			return fieldErr
		}
	}
	return nil
}

// parseFileOpField parses one of a card's four non-Moves file-op fields, returning the field's raw path list and the index of the first line not consumed.
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

// parseMovesField parses a card's "**Moves:**" field, matching each bullet against moveLineRe.
func parseMovesField(labelLine string, lines []string, start int) (pairs []MovePair, raw []string, next int, err error) {
	rest := strings.TrimSpace(strings.TrimPrefix(labelLine, movesLabel))
	if strings.EqualFold(rest, noneSentinel) {
		return []MovePair{}, nil, start, nil
	}
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

// parseDependsOnField parses a card's inline "**Depends-on:**" value into card numbers.
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
