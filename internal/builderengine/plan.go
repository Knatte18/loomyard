// plan.go implements the plan-format v1 parser: ParsePlan reads a plan
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

// Plan is the in-memory form of a parsed plan-format v1 plan: the
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
	Scope []string

	// WhereFiles accumulates every "## Cards" card's "**Where:**" line's
	// comma-separated paths, across all cards, in file order.
	WhereFiles []string

	// CardCount is the number of "### Card N" headings under the batch
	// file's "## Cards" section.
	CardCount int
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
// fields of "NN — <batch-slug> — <one-line intent>" before its named batch
// file has been read.
type indexEntry struct {
	Number int
	Slug   string
	Intent string
	File   string
}

// indexLineRe matches a Batch Index entry's three fields, accepting either
// the em dash "—" or one-or-two ASCII hyphens as the separator between them
// (plan-format.md's worked example uses "—"; hand-written plans may use
// ASCII). The separator is required to be surrounded by whitespace so it is
// never confused with a hyphen inside the slug itself (e.g. "json-flag").
var indexLineRe = regexp.MustCompile(`^(\d+)\s+(?:—|-{1,2})\s+(\S+)\s+(?:—|-{1,2})\s+(.+)$`)

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

		entries = append(entries, indexEntry{
			Number: number,
			Slug:   slug,
			Intent: normalizeWhitespace(m[3]),
			File:   fmt.Sprintf("%02d-%s.md", number, slug),
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

// parseBatchFile seeds a PlanBatch from its Batch Index entry. This is a
// stub: per-batch file parsing (frontmatter, Scope, Cards, verify:) is not
// implemented yet, so the batch file named by entry.File is never opened
// here. ParsePlan already calls this exact signature; per-batch parsing
// extends this function's body in place rather than restructuring
// ParsePlan's call site.
func parseBatchFile(planDir string, entry indexEntry) (PlanBatch, error) {
	return PlanBatch{
		Number: entry.Number,
		Slug:   entry.Slug,
		Intent: entry.Intent,
		File:   entry.File,
	}, nil
}
