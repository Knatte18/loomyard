// parse.go implements ParsePlan: it reads a plan directory's 00-overview.md (scalar frontmatter +
// task framing + Card Index) and, for each card the index lists, that card's own NN-<card-slug>.md
// file, producing the in-memory Plan the rest of webster drives from.
// Every distinct parse failure is a "planparser:"-prefixed wrapped error — loom-plan-spec.md's
// fail-loud discipline admits no silent-default reading of a malformed plan document structure.
// Per-card content defects (a missing field, a malformed Rename: bullet) are recorded leniently
// into the Card model instead, per the lenient-card-parse decision documented in doc.go.
// The card body grammar itself is format-4's type-label model
// (manifest/designs/plan-card-format.md): a card's own body carries one or more bold type labels
// (Create/Edit/Delete/Rename/Move/Prosa/Custom), each carrying its own target list and
// contributing its own TargetGroup, Uses:/Intent:/ImpactSummary: are the remaining recognized
// field labels, and the eight format-3 labels stay recognized but retired — they route into
// Card.RetiredLabels instead of being silently discarded or swallowed into Intent:'s prose.

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

// archiveDirPrefix is the fixed prefix ArchiveDirName builds its return value from, matching how
// PlanDirRel builds its own value from PlanDirName.
const archiveDirPrefix = "archive-"

// ArchiveDirName returns the plan directory's archive-subdirectory name for a rotation stamped
// stamp with same-second collision suffix suffix: archiveDirPrefix + stamp + suffix. The caller
// supplies both the already-formatted compact UTC stamp and the already-chosen same-second
// collision suffix -- ArchiveDirName performs no filesystem work whatsoever, joining no anchor
// path, formatting no time value, and stat-ing nothing. internal/loomshed is what performs the
// corresponding os.MkdirAll/os.Rename calls. The function stays in this package because the
// Planparser Sole-Parser Invariant makes planparser the sole declarer of the plan directory's
// path -- a subdirectory of that directory being part of the same path vocabulary.
func ArchiveDirName(stamp, suffix string) string {
	return archiveDirPrefix + stamp + suffix
}

// PlanDir returns the absolute path to the plan directory for the worktree anchored at anchorPath.
// The caller supplies the absolute directory lyx is anchored at, which in a lyx worktree is
// lyxcwd.Location.AnchorPath() and never WorktreePath() — planparser is the sole declarer of this
// path and never resolves cwd itself.
func PlanDir(anchorPath string) string {
	return filepath.Join(anchorPath, lyxdirs.LyxDirName, PlanDirName)
}

// PlanOverview returns the absolute path to the plan's overview file for the worktree anchored at
// anchorPath. The caller supplies the absolute directory lyx is anchored at, which in a lyx worktree
// is lyxcwd.Location.AnchorPath() and never WorktreePath() — planparser is the sole declarer of this
// path and never resolves cwd itself.
func PlanOverview(anchorPath string) string {
	return filepath.Join(PlanDir(anchorPath), overviewFileName)
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
	Number  int
	Slug    string
	Summary string
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
			Number:  number,
			Slug:    m[2],
			Summary: normalizeWhitespace(m[3]),
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
		Number:  entry.Number,
		Slug:    entry.Slug,
		Summary: entry.Summary,
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

// Bold-label prefixes for the seven type labels format-4 recognizes as a card's own target-list
// key (manifest/designs/plan-card-format.md's "Card fields"). The type name is the key — there is
// no separate "Type:" label. A card body carries one or more of these labels, each contributing
// its own TargetGroup.
const (
	createLabel = "**Create:**"
	editLabel   = "**Edit:**"
	deleteLabel = "**Delete:**"
	renameLabel = "**Rename:**"
	moveLabel   = "**Move:**"
	prosaLabel  = "**Prosa:**"
	customLabel = "**Custom:**"
)

// typeLabels maps each of the seven type labels to the CardType it declares. parseCardBody's
// switch and card-type resolution share this one table so the two never drift apart. A card body
// carrying more than one of these labels — even the same label twice — is the supported
// one-or-more grammar, not a defect: each occurrence contributes its own TargetGroup.
var typeLabels = map[string]CardType{
	createLabel: CardTypeCreate,
	editLabel:   CardTypeEdit,
	deleteLabel: CardTypeDelete,
	renameLabel: CardTypeRename,
	moveLabel:   CardTypeMove,
	prosaLabel:  CardTypeProsa,
	customLabel: CardTypeCustom,
}

// Bold-label prefixes for format-4's five remaining recognized field labels.
const (
	usesLabel          = "**Uses:**"
	intentLabel        = "**Intent:**"
	impactSummaryLabel = "**ImpactSummary:**"
	commitLabel        = "**Commit:**"
	cardVerifyLabel    = "**Verify:**"
)

// Bold-label prefixes retired by format-4. Each stays in cardLabels — deleting one would let it be
// silently discarded or swallowed into **Intent:**'s prose (see the retired-labels-stay-recognized
// decision) — but parseCardBody records only that the label occurred, in Card.RetiredLabels, and
// keeps none of its content.
const (
	whatLabel      = "**What:**"
	contextLabel   = "**Context:**"
	editsLabel     = "**Edits:**"
	createsLabel   = "**Creates:**"
	deletesLabel   = "**Deletes:**"
	movesLabel     = "**Moves:**"
	dependsOnLabel = "**Depends-on:**"
	// legacyVerifyLabel is format-3's lowercase spelling, matched case-sensitively and therefore
	// distinct from cardVerifyLabel ("**Verify:**").
	legacyVerifyLabel = "**verify:**"
)

// cardLabels lists every bold-label prefix parseCardBody recognizes: the seven type labels, the
// five field labels, and the eight retired labels.
var cardLabels = []string{
	createLabel, editLabel, deleteLabel, renameLabel, moveLabel, prosaLabel, customLabel,
	usesLabel, intentLabel, impactSummaryLabel, commitLabel, cardVerifyLabel,
	whatLabel, contextLabel, editsLabel, createsLabel, deletesLabel, movesLabel, dependsOnLabel, legacyVerifyLabel,
}

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

// moveLineRe matches a "Rename:" sub-bullet's well-formed two-symbol pair grammar after the leading "- " bullet marker.
var moveLineRe = regexp.MustCompile("^`([^`]+)` -> `([^`]+)`$")

// parseCardBody parses lines after the card title into card's remaining fields.
// No label in cardLabels is a prefix of another (**Edit:**/**Edits:**, **Create:**/**Creates:**,
// **Delete:**/**Deletes:**, and **Move:**/**Moves:** each differ at their seventh byte), so this
// switch's case order carries no semantics — do not reorder it under the assumption that an
// earlier case can shadow a later one.
func parseCardBody(card *Card, lines []string) error {
	i := 0
	for i < len(lines) {
		trimmed := strings.TrimSpace(lines[i])
		var fieldErr error
		switch {
		case trimmed == "":
			i++
		case strings.HasPrefix(trimmed, createLabel):
			i, fieldErr = parseTypeLabelCase(card, trimmed, createLabel, lines, i+1)
		case strings.HasPrefix(trimmed, editLabel):
			i, fieldErr = parseTypeLabelCase(card, trimmed, editLabel, lines, i+1)
		case strings.HasPrefix(trimmed, deleteLabel):
			i, fieldErr = parseTypeLabelCase(card, trimmed, deleteLabel, lines, i+1)
		case strings.HasPrefix(trimmed, renameLabel):
			i, fieldErr = parseTypeLabelCase(card, trimmed, renameLabel, lines, i+1)
		case strings.HasPrefix(trimmed, moveLabel):
			i, fieldErr = parseTypeLabelCase(card, trimmed, moveLabel, lines, i+1)
		case strings.HasPrefix(trimmed, prosaLabel):
			i, fieldErr = parseTypeLabelCase(card, trimmed, prosaLabel, lines, i+1)
		case strings.HasPrefix(trimmed, customLabel):
			i, fieldErr = parseTypeLabelCase(card, trimmed, customLabel, lines, i+1)
		case strings.HasPrefix(trimmed, usesLabel):
			card.HasUses = true
			card.Uses, i, fieldErr = parseRefField(trimmed, usesLabel, lines, i+1)
		case strings.HasPrefix(trimmed, intentLabel):
			card.HasIntent = true
			// Collect the prose: the label line's own remainder plus every following line up to the next card label line.
			proseLines := []string{strings.TrimSpace(strings.TrimPrefix(trimmed, intentLabel))}
			i++
			for i < len(lines) && !isCardLabelLine(lines[i]) {
				proseLines = append(proseLines, strings.TrimSpace(lines[i]))
				i++
			}
			card.Intent = strings.TrimSpace(strings.Join(proseLines, "\n"))
		case strings.HasPrefix(trimmed, impactSummaryLabel):
			card.HasImpactSummary = true
			card.ImpactSummary = strings.TrimSpace(strings.TrimPrefix(trimmed, impactSummaryLabel))
			i++
			for i < len(lines) && !isCardLabelLine(lines[i]) {
				if t := strings.TrimSpace(lines[i]); t != "" {
					card.ImpactSummaryTrailing = append(card.ImpactSummaryTrailing, t)
				}
				i++
			}
		case strings.HasPrefix(trimmed, commitLabel):
			card.Commit = stripBackticks(strings.TrimSpace(strings.TrimPrefix(trimmed, commitLabel)))
			i++
		case strings.HasPrefix(trimmed, cardVerifyLabel):
			card.HasVerify = true
			card.Verify = strings.TrimSpace(strings.TrimPrefix(trimmed, cardVerifyLabel))
			i++
		case strings.HasPrefix(trimmed, whatLabel):
			card.RetiredLabels = append(card.RetiredLabels, whatLabel)
			i = consumeRetiredLabel(lines, i+1)
		case strings.HasPrefix(trimmed, contextLabel):
			card.RetiredLabels = append(card.RetiredLabels, contextLabel)
			i = consumeRetiredLabel(lines, i+1)
		case strings.HasPrefix(trimmed, editsLabel):
			card.RetiredLabels = append(card.RetiredLabels, editsLabel)
			i = consumeRetiredLabel(lines, i+1)
		case strings.HasPrefix(trimmed, createsLabel):
			card.RetiredLabels = append(card.RetiredLabels, createsLabel)
			i = consumeRetiredLabel(lines, i+1)
		case strings.HasPrefix(trimmed, deletesLabel):
			card.RetiredLabels = append(card.RetiredLabels, deletesLabel)
			i = consumeRetiredLabel(lines, i+1)
		case strings.HasPrefix(trimmed, movesLabel):
			card.RetiredLabels = append(card.RetiredLabels, movesLabel)
			i = consumeRetiredLabel(lines, i+1)
		case strings.HasPrefix(trimmed, dependsOnLabel):
			card.RetiredLabels = append(card.RetiredLabels, dependsOnLabel)
			i = consumeRetiredLabel(lines, i+1)
		case strings.HasPrefix(trimmed, legacyVerifyLabel):
			card.RetiredLabels = append(card.RetiredLabels, legacyVerifyLabel)
			i = consumeRetiredLabel(lines, i+1)
		default:
			i++
		}
		if fieldErr != nil {
			return fieldErr
		}
	}
	return nil
}

// parseTypeLabelCase handles one type-label card-body case: it records the card's type-label
// bookkeeping (TypeLabelCount, HasType, and, on the first type label seen, Type) and then collects
// the label's bullets — via parseRenameField for renameLabel, appending both endpoints of every
// pair to card.Targets in pair order, or via parseRefField for every other type label — appending
// exactly one new TargetGroup to card.TargetGroups for this call's own occurrence of label.
func parseTypeLabelCase(card *Card, labelLine, label string, lines []string, start int) (next int, err error) {
	card.TypeLabelCount++
	card.HasType = true
	if card.Type == CardTypeUnknown {
		card.Type = typeLabels[label]
	}

	group := TargetGroup{Type: typeLabels[label]}

	if label == renameLabel {
		var pairs []MovePair
		var raw []string
		pairs, raw, next, err = parseRenameField(labelLine, lines, start)
		card.Pairs = append(card.Pairs, pairs...)
		card.RenameRaw = append(card.RenameRaw, raw...)
		group.Pairs = pairs
		group.RenameRaw = raw
		// Refs starts as a non-nil empty slice so a **Rename:** label with zero well-formed
		// pairs still yields a non-nil empty Refs, matching every other type label's own
		// zero-bullets behavior.
		group.Refs = []string{}
		for _, p := range pairs {
			card.Targets = append(card.Targets, p.Old, p.New)
			group.Refs = append(group.Refs, p.Old, p.New)
		}
		card.TargetGroups = append(card.TargetGroups, group)
		return next, err
	}

	var refs []string
	refs, next, err = parseRefField(labelLine, label, lines, start)
	card.Targets = append(card.Targets, refs...)
	group.Refs = refs
	card.TargetGroups = append(card.TargetGroups, group)
	return next, err
}

// consumeRetiredLabel advances past every line following a retired label's own line, up to the
// next card label line, storing none of that content.
func consumeRetiredLabel(lines []string, start int) int {
	i := start
	for i < len(lines) && !isCardLabelLine(lines[i]) {
		i++
	}
	return i
}

// parseRefField parses one of a card's ref-list fields (a type label's own target list, or
// **Uses:**), returning the field's raw ref list and the index of the first line not consumed.
// A label present with zero bullets under it returns a non-nil empty slice, distinguishing it
// from an absent label.
func parseRefField(labelLine, label string, lines []string, start int) ([]string, int, error) {
	rest := strings.TrimSpace(strings.TrimPrefix(labelLine, label))
	if rest != "" {
		return nil, start, fmt.Errorf("card field %s carries an inline value %q; plan-format admits only \"- `ref`\" sub-bullets on the following lines", label, rest)
	}

	refs := []string{}
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
		refs = append(refs, payload)
		i++
	}
	return refs, i, nil
}

// parseRenameField parses a Rename card's "**Rename:**" field, matching each bullet against moveLineRe.
func parseRenameField(labelLine string, lines []string, start int) (pairs []MovePair, raw []string, next int, err error) {
	rest := strings.TrimSpace(strings.TrimPrefix(labelLine, renameLabel))
	if rest != "" {
		return nil, nil, start, fmt.Errorf("card field %s carries an inline value %q; plan-format admits only \"- `old` -> `new`\" sub-bullets on the following lines", renameLabel, rest)
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
