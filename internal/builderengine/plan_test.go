// plan_test.go covers ParsePlan's overview-parsing behavior (frontmatter
// decoding, Batch Index parsing including the mandatory "(C cards)"
// segment, framing extraction), its per-batch file-parsing behavior
// (frontmatter incl. root:, Scope, the typed per-card model, verify:
// sections, and the "one or the other, never both" verify rule), and a
// full round-trip over the hand-written testdata/plan-valid fixture.

package builderengine_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/builderengine"
)

// writePlanFiles writes every entry of files (keyed by filename, e.g.
// "00-overview.md") into a fresh temp plan directory and returns that
// directory's path.
func writePlanFiles(t *testing.T, files map[string]string) string {
	t.Helper()

	dir := t.TempDir()
	for name, content := range files {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write plan fixture %s: %v", name, err)
		}
	}
	return dir
}

// writeOverview writes a minimal 00-overview.md with the given content into
// a fresh temp plan directory and returns that directory's path. Used only
// by tests that never reach per-batch file parsing (overview-level errors).
func writeOverview(t *testing.T, content string) string {
	t.Helper()
	return writePlanFiles(t, map[string]string{"00-overview.md": content})
}

// minimalBatchFile is a syntactically complete v2 batch file body: a Scope
// entry, one "### Card 01.1" card carrying all five required file-op
// fields ("none" except a single Edits: bullet), and a verify: command —
// enough to satisfy parseBatchFile without exercising any of its optional
// paths.
func minimalBatchFile(scopePath, editsPath, verifyCommand string) string {
	return "# Batch\n\n## Scope\n\n- " + scopePath + "\n\n## Cards\n\n" +
		"### Card 01.1 — placeholder\n\n" +
		"**What:** placeholder card.\n" +
		"**Context:** none\n" +
		"**Edits:**\n- `" + editsPath + "`\n" +
		"**Creates:** none\n" +
		"**Deletes:** none\n" +
		"**Moves:** none\n\n" +
		"## verify:\n\n" + verifyCommand + "\n"
}

// validOverview is a worked-example-shaped overview with two Batch Index
// entries (each batch has exactly one card, per minimalBatchFile), used as
// the base fixture for the positive-path tests below.
const validOverview = `---
format: 2
approved: true
---

# Plan: add --json to lyx board list

Add a --json output mode to lyx board list, emitting one JSON object per row.

## Batch Index

- 01 — json-flag (1 card) — add the --json flag and envelope emission to boardcli list
- 02 — list-tests (1 card) — cover --json in boardcli list tests and update help-tree pins
`

// TestParsePlan_InlineFieldValueFailsLoud proves a card file-op label line
// carrying an inline value other than "none" (e.g. "**Edits:** `foo.go`")
// is a fail-loud parse error, never silently read as an empty field: an
// empty-but-present field passes card-missing-field while its paths vanish
// from every other check — exactly the silent degradation the none-sentinel
// grammar exists to prevent.
func TestParsePlan_InlineFieldValueFailsLoud(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		fieldLine string
	}{
		{name: "inline Edits path", fieldLine: "**Edits:** `list.go`"},
		{name: "inline Moves pair", fieldLine: "**Moves:** `a.go` -> `b.go`"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			batchBody := "# Batch\n\n## Scope\n\n- internal\n\n## Cards\n\n" +
				"### Card 01.1 — placeholder\n\n" +
				"**What:** placeholder card.\n" +
				"**Context:** none\n" +
				tt.fieldLine + "\n" +
				"**Creates:** none\n" +
				"**Deletes:** none\n\n" +
				"## verify:\n\ngo build ./...\n"
			// Complete the five-field set for the Moves case, which replaced
			// the Edits line above.
			if strings.HasPrefix(tt.fieldLine, "**Moves:**") {
				batchBody = strings.Replace(batchBody, "**Context:** none\n", "**Context:** none\n**Edits:** none\n", 1)
			} else {
				batchBody = strings.Replace(batchBody, "**Deletes:** none\n", "**Deletes:** none\n**Moves:** none\n", 1)
			}

			dir := writePlanFiles(t, map[string]string{
				"00-overview.md": "---\nformat: 2\napproved: true\n---\n\n# Plan\n\nFraming.\n\n## Batch Index\n\n- 01 — json-flag (1 card) — placeholder\n",
				"01-json-flag.md": batchBody,
			})

			_, err := builderengine.ParsePlan(dir)
			if err == nil {
				t.Fatalf("ParsePlan() error = nil; want a fail-loud inline-value error for %q", tt.fieldLine)
			}
			if !strings.Contains(err.Error(), "inline value") {
				t.Errorf("ParsePlan() error = %q; want it to name the inline value", err.Error())
			}
		})
	}
}

func TestParsePlan_Overview(t *testing.T) {
	t.Parallel()

	dir := writePlanFiles(t, map[string]string{
		"00-overview.md": validOverview,
		"01-json-flag.md": minimalBatchFile(
			"internal/boardcli/list.go", "internal/boardcli/list.go", "go build ./...",
		),
		"02-list-tests.md": minimalBatchFile(
			"internal/boardcli/list_test.go", "internal/boardcli/list_test.go", "go test ./...",
		),
	})

	plan, err := builderengine.ParsePlan(dir)
	if err != nil {
		t.Fatalf("ParsePlan(%q) error = %v; want nil", dir, err)
	}

	if plan.Dir != dir {
		t.Errorf("plan.Dir = %q; want %q", plan.Dir, dir)
	}
	if plan.Format != 2 {
		t.Errorf("plan.Format = %d; want 2", plan.Format)
	}
	if !plan.Approved {
		t.Errorf("plan.Approved = false; want true")
	}
	wantFraming := "Add a --json output mode to lyx board list, emitting one JSON object per row."
	if plan.Framing != wantFraming {
		t.Errorf("plan.Framing = %q; want %q", plan.Framing, wantFraming)
	}

	if len(plan.Batches) != 2 {
		t.Fatalf("len(plan.Batches) = %d; want 2", len(plan.Batches))
	}

	want := []builderengine.PlanBatch{
		{Number: 1, Slug: "json-flag", Intent: "add the --json flag and envelope emission to boardcli list", File: "01-json-flag.md", IndexCardCount: 1},
		{Number: 2, Slug: "list-tests", Intent: "cover --json in boardcli list tests and update help-tree pins", File: "02-list-tests.md", IndexCardCount: 1},
	}
	for i, w := range want {
		got := plan.Batches[i]
		if got.Number != w.Number || got.Slug != w.Slug || got.Intent != w.Intent || got.File != w.File || got.IndexCardCount != w.IndexCardCount {
			t.Errorf("plan.Batches[%d] = %+v; want %+v", i, got, w)
		}
		if len(got.Cards) != 1 {
			t.Errorf("len(plan.Batches[%d].Cards) = %d; want 1", i, len(got.Cards))
		}
	}
}

func TestParsePlan_Overview_ASCIIDashSeparators(t *testing.T) {
	t.Parallel()

	const overview = `---
format: 2
approved: true
---

# Plan: ascii dash variant

Framing paragraph.

## Batch Index

- 01 - single-dash (1 card) - intent using a single ASCII hyphen
- 02 -- double-dash (1 card) -- intent using a double ASCII hyphen
`
	dir := writePlanFiles(t, map[string]string{
		"00-overview.md":    overview,
		"01-single-dash.md": minimalBatchFile("a.go", "a.go", "go build ./..."),
		"02-double-dash.md": minimalBatchFile("b.go", "b.go", "go build ./..."),
	})

	plan, err := builderengine.ParsePlan(dir)
	if err != nil {
		t.Fatalf("ParsePlan(%q) error = %v; want nil", dir, err)
	}
	if len(plan.Batches) != 2 {
		t.Fatalf("len(plan.Batches) = %d; want 2", len(plan.Batches))
	}
	if plan.Batches[0].Slug != "single-dash" {
		t.Errorf("plan.Batches[0].Slug = %q; want %q", plan.Batches[0].Slug, "single-dash")
	}
	if plan.Batches[1].Slug != "double-dash" {
		t.Errorf("plan.Batches[1].Slug = %q; want %q", plan.Batches[1].Slug, "double-dash")
	}
}

func TestParsePlan_Overview_Errors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		content    string
		noFile     bool
		wantSubstr string
	}{
		{
			name:       "missing overview file",
			noFile:     true,
			wantSubstr: "not found",
		},
		{
			name:       "missing frontmatter entirely",
			content:    "# Plan: no frontmatter\n\nFraming.\n\n## Batch Index\n\n- 01 — a (1 card) — b\n",
			wantSubstr: "missing required frontmatter",
		},
		{
			name:       "missing format key",
			content:    "---\napproved: true\n---\n\n# Plan\n\nFraming.\n\n## Batch Index\n\n- 01 — a (1 card) — b\n",
			wantSubstr: `missing required key "format"`,
		},
		{
			name:       "missing approved key",
			content:    "---\nformat: 2\n---\n\n# Plan\n\nFraming.\n\n## Batch Index\n\n- 01 — a (1 card) — b\n",
			wantSubstr: `missing required key "approved"`,
		},
		{
			name:       "unknown frontmatter key",
			content:    "---\nformat: 2\napproved: true\nextra: true\n---\n\n# Plan\n\nFraming.\n\n## Batch Index\n\n- 01 — a (1 card) — b\n",
			wantSubstr: "field extra not found",
		},
		{
			name:       "duplicate frontmatter key",
			content:    "---\nformat: 2\nformat: 2\napproved: true\n---\n\n# Plan\n\nFraming.\n\n## Batch Index\n\n- 01 — a (1 card) — b\n",
			wantSubstr: "already defined",
		},
		{
			name:       "unterminated frontmatter fence",
			content:    "---\nformat: 2\napproved: true\n\n# Plan\n\nFraming.\n\n## Batch Index\n\n- 01 — a (1 card) — b\n",
			wantSubstr: "unterminated frontmatter fence",
		},
		{
			name:       "missing batch index heading",
			content:    "---\nformat: 2\napproved: true\n---\n\n# Plan\n\nFraming.\n",
			wantSubstr: `missing "## Batch Index" heading`,
		},
		{
			name:       "unparseable batch index line",
			content:    "---\nformat: 2\napproved: true\n---\n\n# Plan\n\nFraming.\n\n## Batch Index\n\n- not a valid entry\n",
			wantSubstr: "unparseable batch index line",
		},
		{
			name:       "batch index line missing the (C cards) segment",
			content:    "---\nformat: 2\napproved: true\n---\n\n# Plan\n\nFraming.\n\n## Batch Index\n\n- 01 — a — b\n",
			wantSubstr: "unparseable batch index line",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var dir string
			if tt.noFile {
				dir = t.TempDir()
			} else {
				dir = writeOverview(t, tt.content)
			}

			_, err := builderengine.ParsePlan(dir)
			if err == nil {
				t.Fatalf("ParsePlan(%q) error = nil; want error containing %q", dir, tt.wantSubstr)
			}
			if !strings.Contains(err.Error(), tt.wantSubstr) {
				t.Errorf("ParsePlan(%q) error = %q; want substring %q", dir, err.Error(), tt.wantSubstr)
			}
			if !strings.HasPrefix(err.Error(), "builder:") {
				t.Errorf("ParsePlan(%q) error = %q; want \"builder:\" prefix", dir, err.Error())
			}
		})
	}
}

// singleBatchOverview is a one-batch overview used by the per-batch-file
// tests below, which only care about batch "01-only.md"'s own content.
const singleBatchOverview = `---
format: 2
approved: true
---

# Plan: single batch

Framing.

## Batch Index

- 01 — only (1 card) — the only batch
`

// parseSingleBatch writes singleBatchOverview plus one "01-only.md" batch
// file with the given body and returns the parsed PlanBatch (t.Fatal on any
// ParsePlan error).
func parseSingleBatch(t *testing.T, batchBody string) builderengine.PlanBatch {
	t.Helper()

	dir := writePlanFiles(t, map[string]string{
		"00-overview.md": singleBatchOverview,
		"01-only.md":     batchBody,
	})
	plan, err := builderengine.ParsePlan(dir)
	if err != nil {
		t.Fatalf("ParsePlan(%q) error = %v; want nil", dir, err)
	}
	if len(plan.Batches) != 1 {
		t.Fatalf("len(plan.Batches) = %d; want 1", len(plan.Batches))
	}
	return plan.Batches[0]
}

func TestParsePlan_BatchFile_ScopeCardsVerify(t *testing.T) {
	t.Parallel()

	body := "# 01 — only\n\n## Intent\n\nProse for the implementer, never stored on PlanBatch.\n\n" +
		"## Scope\n\n- internal/boardcli/list.go\n- internal/boardengine/rows.go\n\n" +
		"## Cards\n\n### Card 01.1 — flag + row struct\n\n**What:** add a flag.\n" +
		"**Context:** none\n" +
		"**Edits:**\n- `internal/boardcli/list.go`\n- `internal/boardengine/rows.go`\n" +
		"**Creates:** none\n**Deletes:** none\n**Moves:** none\n\n" +
		"### Card 01.2 — emission path\n\n**What:** wire it up.\n" +
		"**Context:** none\n**Edits:**\n- `internal/boardcli/list.go`\n" +
		"**Creates:** none\n**Deletes:** none\n**Moves:** none\n\n" +
		"## verify:\n\ngo test ./internal/boardcli/... ./internal/boardengine/...\n"

	batch := parseSingleBatch(t, body)

	wantScope := []string{"internal/boardcli/list.go", "internal/boardengine/rows.go"}
	if !slices.Equal(batch.Scope, wantScope) {
		t.Errorf("batch.Scope = %v; want %v", batch.Scope, wantScope)
	}

	if len(batch.Cards) != 2 {
		t.Fatalf("len(batch.Cards) = %d; want 2", len(batch.Cards))
	}

	card1 := batch.Cards[0]
	wantEdits1 := []string{"internal/boardcli/list.go", "internal/boardengine/rows.go"}
	if !slices.Equal(card1.EditsFiles, wantEdits1) {
		t.Errorf("batch.Cards[0].EditsFiles = %v; want %v", card1.EditsFiles, wantEdits1)
	}
	if card1.BatchPrefix != 1 || card1.Number != 1 || card1.Title != "flag + row struct" {
		t.Errorf("batch.Cards[0] BatchPrefix/Number/Title = %d/%d/%q; want 1/1/%q", card1.BatchPrefix, card1.Number, card1.Title, "flag + row struct")
	}

	card2 := batch.Cards[1]
	wantEdits2 := []string{"internal/boardcli/list.go"}
	if !slices.Equal(card2.EditsFiles, wantEdits2) {
		t.Errorf("batch.Cards[1].EditsFiles = %v; want %v", card2.EditsFiles, wantEdits2)
	}
	if card2.Number != 2 || card2.Title != "emission path" {
		t.Errorf("batch.Cards[1] Number/Title = %d/%q; want 2/%q", card2.Number, card2.Title, "emission path")
	}

	wantVerify := "go test ./internal/boardcli/... ./internal/boardengine/..."
	if batch.VerifyCommand != wantVerify {
		t.Errorf("batch.VerifyCommand = %q; want %q", batch.VerifyCommand, wantVerify)
	}
	if batch.VerifyDeferred {
		t.Errorf("batch.VerifyDeferred = true; want false")
	}
	if batch.Intent != "the only batch" {
		t.Errorf("batch.Intent = %q; want %q (index-sourced, not the body's \"## Intent\" section)", batch.Intent, "the only batch")
	}
}

// TestParsePlan_Card_FiveFieldsNoneSentinel covers the three-way
// distinction plan-format v2 pins for each of the five typed file-op
// fields: absent entirely (nil slice, HasX == false), present with the
// literal "none" (empty non-nil slice, HasX == true), and present with
// bullets (populated non-nil slice, HasX == true).
func TestParsePlan_Card_FiveFieldsNoneSentinel(t *testing.T) {
	t.Parallel()

	t.Run("all five none", func(t *testing.T) {
		t.Parallel()

		body := "# Batch\n\n## Cards\n\n### Card 01.1 — none everywhere\n\n" +
			"**What:** nothing.\n**Context:** none\n**Edits:** none\n**Creates:** none\n" +
			"**Deletes:** none\n**Moves:** none\n"
		batch := parseSingleBatch(t, body)
		card := batch.Cards[0]

		for name, got := range map[string][]string{
			"ContextFiles": card.ContextFiles,
			"EditsFiles":   card.EditsFiles,
			"CreatesFiles": card.CreatesFiles,
			"DeletesFiles": card.DeletesFiles,
		} {
			if got == nil {
				t.Errorf("card.%s = nil; want empty non-nil slice for a present \"none\" field", name)
			}
			if len(got) != 0 {
				t.Errorf("card.%s = %v; want empty", name, got)
			}
		}
		if card.Moves == nil || len(card.Moves) != 0 {
			t.Errorf("card.Moves = %v; want empty non-nil slice", card.Moves)
		}
		if !card.HasContext || !card.HasEdits || !card.HasCreates || !card.HasDeletes || !card.HasMoves || !card.HasWhat {
			t.Errorf("card Has* = %+v; want all true (every field's label was present)", card)
		}
	})

	t.Run("field absent entirely", func(t *testing.T) {
		t.Parallel()

		// Edits: is entirely missing (no label line at all) — this is a
		// card-level defect (Validate's card-missing-field territory), not
		// a parse error, per the lenient-card-parse decision.
		body := "# Batch\n\n## Cards\n\n### Card 01.1 — missing edits\n\n" +
			"**Context:** none\n**Creates:** none\n**Deletes:** none\n**Moves:** none\n"
		batch := parseSingleBatch(t, body)
		card := batch.Cards[0]

		if card.EditsFiles != nil {
			t.Errorf("card.EditsFiles = %v; want nil (label never present)", card.EditsFiles)
		}
		if card.HasEdits {
			t.Errorf("card.HasEdits = true; want false")
		}
		if card.HasWhat {
			t.Errorf("card.HasWhat = true; want false (no **What:** label present)")
		}
	})

	t.Run("populated bullets", func(t *testing.T) {
		t.Parallel()

		body := "# Batch\n\n## Cards\n\n### Card 01.1 — populated\n\n" +
			"**Context:**\n- `a.go`\n- `b.go`\n" +
			"**Edits:**\n- `c.go`\n" +
			"**Creates:**\n- `d.go`\n" +
			"**Deletes:**\n- `e.go`\n" +
			"**Moves:** none\n"
		batch := parseSingleBatch(t, body)
		card := batch.Cards[0]

		if want := []string{"a.go", "b.go"}; !slices.Equal(card.ContextFiles, want) {
			t.Errorf("card.ContextFiles = %v; want %v", card.ContextFiles, want)
		}
		if want := []string{"c.go"}; !slices.Equal(card.EditsFiles, want) {
			t.Errorf("card.EditsFiles = %v; want %v", card.EditsFiles, want)
		}
		if want := []string{"d.go"}; !slices.Equal(card.CreatesFiles, want) {
			t.Errorf("card.CreatesFiles = %v; want %v", card.CreatesFiles, want)
		}
		if want := []string{"e.go"}; !slices.Equal(card.DeletesFiles, want) {
			t.Errorf("card.DeletesFiles = %v; want %v", card.DeletesFiles, want)
		}
	})

	t.Run("bullet payload not backtick-wrapped is retained as-is", func(t *testing.T) {
		t.Parallel()

		body := "# Batch\n\n## Cards\n\n### Card 01.1 — unwrapped\n\n" +
			"**Context:** none\n**Edits:**\n- not-backtick-wrapped.go\n" +
			"**Creates:** none\n**Deletes:** none\n**Moves:** none\n"
		batch := parseSingleBatch(t, body)
		card := batch.Cards[0]

		if want := []string{"not-backtick-wrapped.go"}; !slices.Equal(card.EditsFiles, want) {
			t.Errorf("card.EditsFiles = %v; want %v", card.EditsFiles, want)
		}
	})
}

// TestParsePlan_Card_MovesGrammar covers Moves: bullets: well-formed pairs
// land in Moves (normalized), and a bullet that fails the pair grammar is
// retained verbatim in MovesRaw rather than becoming a parse error
// (lenient-card-parse decision).
func TestParsePlan_Card_MovesGrammar(t *testing.T) {
	t.Parallel()

	body := "# Batch\n\n## Cards\n\n### Card 01.1 — moves\n\n" +
		"**Context:** none\n**Edits:** none\n**Creates:** none\n**Deletes:** none\n" +
		"**Moves:**\n- `old/path.go` -> `new/path.go`\n- this bullet has no arrow at all\n"
	batch := parseSingleBatch(t, body)
	card := batch.Cards[0]

	wantPairs := []builderengine.MovePair{{Old: "old/path.go", New: "new/path.go"}}
	if !slices.Equal(card.Moves, wantPairs) {
		t.Errorf("card.Moves = %+v; want %+v", card.Moves, wantPairs)
	}
	wantRaw := []string{"this bullet has no arrow at all"}
	if !slices.Equal(card.MovesRaw, wantRaw) {
		t.Errorf("card.MovesRaw = %v; want %v", card.MovesRaw, wantRaw)
	}
}

// TestParsePlan_RootNormalization covers the per-batch root: frontmatter
// shorthand and the "//" worktree-root-relative escape, across every case
// the per-batch-root-path-shorthand decision pins: root set, root absent,
// a "//"-escaped path under a set root, and a Moves: pair whose two sides
// cross the root boundary (one root-relative, one "//"-escaped).
func TestParsePlan_RootNormalization(t *testing.T) {
	t.Parallel()

	t.Run("root set joins root/path", func(t *testing.T) {
		t.Parallel()

		body := "---\nroot: internal/boardcli\n---\n\n# Batch\n\n## Cards\n\n### Card 01.1 — rooted\n\n" +
			"**Context:** none\n**Edits:**\n- `list.go`\n**Creates:** none\n**Deletes:** none\n**Moves:** none\n"
		batch := parseSingleBatch(t, body)
		if batch.Root != "internal/boardcli" {
			t.Errorf("batch.Root = %q; want %q", batch.Root, "internal/boardcli")
		}
		if want := []string{"internal/boardcli/list.go"}; !slices.Equal(batch.Cards[0].EditsFiles, want) {
			t.Errorf("card.EditsFiles = %v; want %v", batch.Cards[0].EditsFiles, want)
		}
	})

	t.Run("root absent stores the path unchanged", func(t *testing.T) {
		t.Parallel()

		body := "# Batch\n\n## Cards\n\n### Card 01.1 — rootless\n\n" +
			"**Context:** none\n**Edits:**\n- `internal/boardcli/list.go`\n**Creates:** none\n**Deletes:** none\n**Moves:** none\n"
		batch := parseSingleBatch(t, body)
		if batch.Root != "" {
			t.Errorf("batch.Root = %q; want empty", batch.Root)
		}
		if want := []string{"internal/boardcli/list.go"}; !slices.Equal(batch.Cards[0].EditsFiles, want) {
			t.Errorf("card.EditsFiles = %v; want %v", batch.Cards[0].EditsFiles, want)
		}
	})

	t.Run("// escapes the root, worktree-root-relative", func(t *testing.T) {
		t.Parallel()

		body := "---\nroot: internal/boardcli\n---\n\n# Batch\n\n## Cards\n\n### Card 01.1 — escaped\n\n" +
			"**Context:**\n- `//cmd/lyx/main.go`\n**Edits:**\n- `list.go`\n**Creates:** none\n**Deletes:** none\n**Moves:** none\n"
		batch := parseSingleBatch(t, body)
		if want := []string{"cmd/lyx/main.go"}; !slices.Equal(batch.Cards[0].ContextFiles, want) {
			t.Errorf("card.ContextFiles = %v; want %v (// always worktree-root-relative)", batch.Cards[0].ContextFiles, want)
		}
		if want := []string{"internal/boardcli/list.go"}; !slices.Equal(batch.Cards[0].EditsFiles, want) {
			t.Errorf("card.EditsFiles = %v; want %v", batch.Cards[0].EditsFiles, want)
		}
	})

	t.Run("Moves: pair crossing the root boundary", func(t *testing.T) {
		t.Parallel()

		body := "---\nroot: internal/boardcli\n---\n\n# Batch\n\n## Cards\n\n### Card 01.1 — crossing\n\n" +
			"**Context:** none\n**Edits:** none\n**Creates:** none\n**Deletes:** none\n" +
			"**Moves:**\n- `old.go` -> `//cmd/lyx/new.go`\n"
		batch := parseSingleBatch(t, body)
		want := []builderengine.MovePair{{Old: "internal/boardcli/old.go", New: "cmd/lyx/new.go"}}
		if !slices.Equal(batch.Cards[0].Moves, want) {
			t.Errorf("card.Moves = %+v; want %+v", batch.Cards[0].Moves, want)
		}
	})

	t.Run(`root: "." joins to raw unchanged, not an unclean "./raw"`, func(t *testing.T) {
		t.Parallel()

		body := "---\nroot: .\n---\n\n# Batch\n\n## Cards\n\n### Card 01.1 — dot root\n\n" +
			"**Context:** none\n**Edits:**\n- `list.go`\n**Creates:** none\n**Deletes:** none\n**Moves:** none\n"
		batch := parseSingleBatch(t, body)
		if want := []string{"list.go"}; !slices.Equal(batch.Cards[0].EditsFiles, want) {
			t.Errorf("card.EditsFiles = %v; want %v (root: \".\" must not produce an unclean \"./\" prefix)", batch.Cards[0].EditsFiles, want)
		}
	})

	t.Run("## Scope stays worktree-relative regardless of root:", func(t *testing.T) {
		t.Parallel()

		body := "---\nroot: internal/boardcli\n---\n\n# Batch\n\n## Scope\n\n- internal/boardcli/list.go\n\n" +
			"## Cards\n\n### Card 01.1 — scope check\n\n" +
			"**Context:** none\n**Edits:** none\n**Creates:** none\n**Deletes:** none\n**Moves:** none\n"
		batch := parseSingleBatch(t, body)
		if want := []string{"internal/boardcli/list.go"}; !slices.Equal(batch.Scope, want) {
			t.Errorf("batch.Scope = %v; want %v (root: never resolves Scope)", batch.Scope, want)
		}
	})
}

// TestParsePlan_HasRenameMechanic covers PlanBatch.HasRenameMechanic: it is
// true exactly when the batch body contains a "## Rename mechanic" heading
// at all (presence only, never the section's own prose), and the
// testdata/plan-valid fixture's own Moves batch (02-list-tests.md) has it
// set, since it is hand-written with the section already present.
func TestParsePlan_HasRenameMechanic(t *testing.T) {
	t.Parallel()

	t.Run("batch with the section sets the flag", func(t *testing.T) {
		t.Parallel()

		body := "# Batch\n\n## Rename mechanic\n\nRun `git mv old.go new.go` first.\n\n" +
			"## Cards\n\n### Card 01.1 — placeholder\n\n" +
			"**Context:** none\n**Edits:** none\n**Creates:** none\n**Deletes:** none\n" +
			"**Moves:**\n- `old.go` -> `new.go`\n"
		batch := parseSingleBatch(t, body)
		if !batch.HasRenameMechanic {
			t.Errorf("batch.HasRenameMechanic = false; want true")
		}
	})

	t.Run("batch without the section leaves the flag false", func(t *testing.T) {
		t.Parallel()

		body := "# Batch\n\n## Cards\n\n### Card 01.1 — placeholder\n\n" +
			"**Context:** none\n**Edits:** none\n**Creates:** none\n**Deletes:** none\n**Moves:** none\n"
		batch := parseSingleBatch(t, body)
		if batch.HasRenameMechanic {
			t.Errorf("batch.HasRenameMechanic = true; want false")
		}
	})

	t.Run("plan-valid fixture's Moves batch has it set", func(t *testing.T) {
		t.Parallel()

		plan, err := builderengine.ParsePlan(filepath.Join("testdata", "plan-valid"))
		if err != nil {
			t.Fatalf("ParsePlan(testdata/plan-valid) error = %v; want nil", err)
		}
		if !plan.Batches[1].HasRenameMechanic {
			t.Errorf("plan.Batches[1] (02-list-tests) HasRenameMechanic = false; want true")
		}
	})
}

// TestParsePlan_CardHeading covers the "### Card NN.C — <title>" heading
// grammar: both em-dash and ASCII separators populate BatchPrefix/Number/
// Title identically, and a "### " heading that does not match the shape at
// all is a fail-loud parse error (document structure, not a card-level
// defect).
func TestParsePlan_CardHeading(t *testing.T) {
	t.Parallel()

	t.Run("em dash separator", func(t *testing.T) {
		t.Parallel()

		body := "# Batch\n\n## Cards\n\n### Card 03.2 — emission path\n\n" +
			"**Context:** none\n**Edits:** none\n**Creates:** none\n**Deletes:** none\n**Moves:** none\n"
		batch := parseSingleBatch(t, body)
		card := batch.Cards[0]
		if card.BatchPrefix != 3 || card.Number != 2 || card.Title != "emission path" {
			t.Errorf("card BatchPrefix/Number/Title = %d/%d/%q; want 3/2/%q", card.BatchPrefix, card.Number, card.Title, "emission path")
		}
	})

	t.Run("ASCII hyphen separator", func(t *testing.T) {
		t.Parallel()

		body := "# Batch\n\n## Cards\n\n### Card 03.2 -- emission path\n\n" +
			"**Context:** none\n**Edits:** none\n**Creates:** none\n**Deletes:** none\n**Moves:** none\n"
		batch := parseSingleBatch(t, body)
		card := batch.Cards[0]
		if card.BatchPrefix != 3 || card.Number != 2 || card.Title != "emission path" {
			t.Errorf("card BatchPrefix/Number/Title = %d/%d/%q; want 3/2/%q", card.BatchPrefix, card.Number, card.Title, "emission path")
		}
	})

	t.Run("non-card ### heading inside Cards is a parse error", func(t *testing.T) {
		t.Parallel()

		body := "# Batch\n\n## Cards\n\n### Not A Card Heading\n\nsome prose\n"
		dir := writePlanFiles(t, map[string]string{
			"00-overview.md": singleBatchOverview,
			"01-only.md":     body,
		})
		_, err := builderengine.ParsePlan(dir)
		if err == nil {
			t.Fatal("ParsePlan() error = nil; want a parse error for the unrecognized ### heading")
		}
		if !strings.Contains(err.Error(), "Not A Card Heading") {
			t.Errorf("ParsePlan() error = %q; want it to name the offending line", err.Error())
		}
	})
}

// TestParsePlan_CardCommitAndVerify covers the optional per-card
// "**Commit:**" field (backtick-stripped) and the optional per-card
// "**verify:**" field (taken verbatim, v1 semantics unchanged).
func TestParsePlan_CardCommitAndVerify(t *testing.T) {
	t.Parallel()

	body := "# Batch\n\n## Cards\n\n### Card 01.1 — flag\n\n" +
		"**Context:** none\n**Edits:** none\n**Creates:** none\n**Deletes:** none\n**Moves:** none\n" +
		"**Commit:** `01.1: add the --json flag`\n" +
		"**verify:** go build ./...\n"
	batch := parseSingleBatch(t, body)
	card := batch.Cards[0]

	if card.Commit != "01.1: add the --json flag" {
		t.Errorf("card.Commit = %q; want %q", card.Commit, "01.1: add the --json flag")
	}
	if card.VerifyCommand != "go build ./..." {
		t.Errorf("card.VerifyCommand = %q; want %q", card.VerifyCommand, "go build ./...")
	}
}

// TestParsePlan_IndexCardCount covers the Batch Index's mandatory
// "(C cards)" segment: it parses into IndexCardCount (singular "(1 card)"
// accepted), and a missing segment is the pre-existing "unparseable batch
// index line" fail-loud error (batch-index-card-counts decision).
func TestParsePlan_IndexCardCount(t *testing.T) {
	t.Parallel()

	t.Run("plural cards", func(t *testing.T) {
		t.Parallel()

		const overview = `---
format: 2
approved: true
---

# Plan

Framing.

## Batch Index

- 01 — multi (3 cards) — a batch with three cards
`
		dir := writePlanFiles(t, map[string]string{
			"00-overview.md": overview,
			"01-multi.md": "# Batch\n\n## Cards\n\n" +
				"### Card 01.1 — a\n\n**Context:** none\n**Edits:** none\n**Creates:** none\n**Deletes:** none\n**Moves:** none\n\n" +
				"### Card 01.2 — b\n\n**Context:** none\n**Edits:** none\n**Creates:** none\n**Deletes:** none\n**Moves:** none\n\n" +
				"### Card 01.3 — c\n\n**Context:** none\n**Edits:** none\n**Creates:** none\n**Deletes:** none\n**Moves:** none\n",
		})
		plan, err := builderengine.ParsePlan(dir)
		if err != nil {
			t.Fatalf("ParsePlan(%q) error = %v; want nil", dir, err)
		}
		if got := plan.Batches[0].IndexCardCount; got != 3 {
			t.Errorf("plan.Batches[0].IndexCardCount = %d; want 3", got)
		}
	})

	t.Run("singular card accepted", func(t *testing.T) {
		t.Parallel()

		if got := parseSingleBatch(t, minimalBatchFile("a.go", "a.go", "go build ./...")); got.IndexCardCount != 1 {
			t.Errorf("IndexCardCount = %d; want 1", got.IndexCardCount)
		}
	})

	t.Run("missing (C cards) segment is unparseable", func(t *testing.T) {
		t.Parallel()

		content := "---\nformat: 2\napproved: true\n---\n\n# Plan\n\nFraming.\n\n## Batch Index\n\n- 01 — only — the only batch\n"
		dir := writeOverview(t, content)
		_, err := builderengine.ParsePlan(dir)
		if err == nil {
			t.Fatal("ParsePlan() error = nil; want unparseable batch index line error")
		}
		if !strings.Contains(err.Error(), "unparseable batch index line") {
			t.Errorf("ParsePlan() error = %q; want unparseable-line substring", err.Error())
		}
	})
}

func TestParsePlan_BatchFile_Frontmatter(t *testing.T) {
	t.Parallel()

	t.Run("oversized", func(t *testing.T) {
		t.Parallel()

		body := "---\noversized: true\n---\n\n" + minimalBatchFile("a.go", "a.go", "go build ./...")
		batch := parseSingleBatch(t, body)
		if !batch.Oversized {
			t.Errorf("batch.Oversized = false; want true")
		}
	})

	t.Run("verify deferred with chain-end", func(t *testing.T) {
		t.Parallel()

		body := "---\nverify: deferred\nchain-end: 4\n---\n\n# Batch\n\n## Scope\n\n- a.go\n\n" +
			"## Cards\n\n### Card 01.1 — placeholder\n\n" +
			"**Context:** none\n**Edits:**\n- `a.go`\n**Creates:** none\n**Deletes:** none\n**Moves:** none\n"
		batch := parseSingleBatch(t, body)
		if !batch.VerifyDeferred {
			t.Errorf("batch.VerifyDeferred = false; want true")
		}
		if batch.ChainEnd != 4 {
			t.Errorf("batch.ChainEnd = %d; want 4", batch.ChainEnd)
		}
		if batch.VerifyCommand != "" {
			t.Errorf("batch.VerifyCommand = %q; want empty", batch.VerifyCommand)
		}
	})

	t.Run("unrecognized verify sentinel", func(t *testing.T) {
		t.Parallel()

		dir := writePlanFiles(t, map[string]string{
			"00-overview.md": singleBatchOverview,
			"01-only.md":     "---\nverify: sometimes\n---\n\n" + minimalBatchFile("a.go", "a.go", "go build ./..."),
		})
		_, err := builderengine.ParsePlan(dir)
		if err == nil {
			t.Fatal("ParsePlan() error = nil; want error naming the unrecognized verify: sentinel")
		}
		if !strings.Contains(err.Error(), `"sometimes"`) {
			t.Errorf("ParsePlan() error = %q; want it to name the unrecognized value", err.Error())
		}
	})

	t.Run("both deferred and verify section is an error", func(t *testing.T) {
		t.Parallel()

		body := "---\nverify: deferred\nchain-end: 2\n---\n\n" + minimalBatchFile("a.go", "a.go", "go build ./...")
		dir := writePlanFiles(t, map[string]string{
			"00-overview.md": singleBatchOverview,
			"01-only.md":     body,
		})
		_, err := builderengine.ParsePlan(dir)
		if err == nil {
			t.Fatal("ParsePlan() error = nil; want error for both verify: deferred and a \"## verify:\" section")
		}
		if !strings.Contains(err.Error(), "allows only one") {
			t.Errorf("ParsePlan() error = %q; want it to name the one-or-the-other rule", err.Error())
		}
	})

	t.Run("unknown frontmatter key", func(t *testing.T) {
		t.Parallel()

		dir := writePlanFiles(t, map[string]string{
			"00-overview.md": singleBatchOverview,
			"01-only.md":     "---\nextra: true\n---\n\n" + minimalBatchFile("a.go", "a.go", "go build ./..."),
		})
		_, err := builderengine.ParsePlan(dir)
		if err == nil {
			t.Fatal("ParsePlan() error = nil; want unknown-key rejection")
		}
		if !strings.Contains(err.Error(), "field extra not found") {
			t.Errorf("ParsePlan() error = %q; want field-not-found substring", err.Error())
		}
	})
}

func TestParsePlan_BatchFile_ScopeGlobRejected(t *testing.T) {
	t.Parallel()

	body := "# Batch\n\n## Scope\n\n- internal/**/list.go\n\n## Cards\n\n### Card 01.1 — placeholder\n\n" +
		"**Context:** none\n**Edits:**\n- `a.go`\n**Creates:** none\n**Deletes:** none\n**Moves:** none\n\n" +
		"## verify:\n\ngo build ./...\n"
	dir := writePlanFiles(t, map[string]string{
		"00-overview.md": singleBatchOverview,
		"01-only.md":     body,
	})
	_, err := builderengine.ParsePlan(dir)
	if err == nil {
		t.Fatal("ParsePlan() error = nil; want glob-in-scope rejection")
	}
	if !strings.Contains(err.Error(), "glob") {
		t.Errorf("ParsePlan() error = %q; want it to name the glob rejection", err.Error())
	}
}

func TestParsePlan_BatchFile_NoScopeOrCards(t *testing.T) {
	t.Parallel()

	// A batch file with neither section present is not itself a parse
	// error — Validate's own checks (verify-missing, scope-malformed) are
	// what flag an under-specified batch; ParsePlan only ever fails loud on
	// what it can mechanically read.
	body := "# Batch\n\n## verify:\n\ngo build ./...\n"
	batch := parseSingleBatch(t, body)
	if batch.Scope != nil {
		t.Errorf("batch.Scope = %v; want nil", batch.Scope)
	}
	if batch.Cards != nil {
		t.Errorf("batch.Cards = %v; want nil", batch.Cards)
	}
}

func TestParsePlan_BatchFile_NotFound(t *testing.T) {
	t.Parallel()

	dir := writeOverview(t, singleBatchOverview)
	_, err := builderengine.ParsePlan(dir)
	if err == nil {
		t.Fatal("ParsePlan() error = nil; want batch-file-not-found error")
	}
	if !strings.Contains(err.Error(), "batch file not found") {
		t.Errorf("ParsePlan() error = %q; want batch-file-not-found substring", err.Error())
	}
}

// TestParsePlan_PlanValidFixture round-trips the hand-written
// testdata/plan-valid fixture exactly: every batch's number, slug, flags,
// chain-end, root, index card count, scope list, and every card's typed
// fields must match the fixture's own byte-consistent content.
func TestParsePlan_PlanValidFixture(t *testing.T) {
	t.Parallel()

	plan, err := builderengine.ParsePlan(filepath.Join("testdata", "plan-valid"))
	if err != nil {
		t.Fatalf("ParsePlan(testdata/plan-valid) error = %v; want nil", err)
	}

	if plan.Format != 2 {
		t.Errorf("plan.Format = %d; want 2", plan.Format)
	}
	if !plan.Approved {
		t.Errorf("plan.Approved = false; want true")
	}
	if len(plan.Batches) != 5 {
		t.Fatalf("len(plan.Batches) = %d; want 5", len(plan.Batches))
	}

	b1 := plan.Batches[0]
	if b1.Number != 1 || b1.Slug != "json-flag" || b1.File != "01-json-flag.md" || b1.IndexCardCount != 2 {
		t.Errorf("batch 1 = %+v; want Number=1 Slug=json-flag File=01-json-flag.md IndexCardCount=2", b1)
	}
	if b1.Root != "." {
		t.Errorf("batch 1 Root = %q; want %q", b1.Root, ".")
	}
	if len(b1.Cards) != 2 {
		t.Fatalf("len(batch 1 Cards) = %d; want 2", len(b1.Cards))
	}
	if want := []string{"01-json-flag.md"}; !slices.Equal(b1.Cards[0].EditsFiles, want) {
		t.Errorf("batch 1 card 1 EditsFiles = %v; want %v (root: \".\" must not produce an unclean \"./\" prefix)", b1.Cards[0].EditsFiles, want)
	}
	if b1.Cards[0].Commit != "01.1: add the --json flag and row struct" {
		t.Errorf("batch 1 card 1 Commit = %q; want the NN.C-prefixed commit message", b1.Cards[0].Commit)
	}
	if want := []string{"02-list-tests.md"}; !slices.Equal(b1.Cards[1].ContextFiles, want) {
		t.Errorf("batch 1 card 2 ContextFiles = %v; want %v (// escape)", b1.Cards[1].ContextFiles, want)
	}

	b2 := plan.Batches[1]
	if b2.Number != 2 || b2.IndexCardCount != 2 {
		t.Errorf("batch 2 = %+v; want Number=2 IndexCardCount=2", b2)
	}
	if len(b2.Cards) != 2 {
		t.Fatalf("len(batch 2 Cards) = %d; want 2", len(b2.Cards))
	}
	wantMoves := []builderengine.MovePair{{Old: "03-refactor-a.md", New: "03-refactor-a-renamed.md"}}
	if !slices.Equal(b2.Cards[1].Moves, wantMoves) {
		t.Errorf("batch 2 card 2 Moves = %+v; want %+v", b2.Cards[1].Moves, wantMoves)
	}

	b3 := plan.Batches[2]
	if b3.Number != 3 || !b3.VerifyDeferred || b3.ChainEnd != 4 || b3.IndexCardCount != 1 {
		t.Errorf("batch 3 = %+v; want Number=3 VerifyDeferred=true ChainEnd=4 IndexCardCount=1", b3)
	}

	b4 := plan.Batches[3]
	if b4.Number != 4 || b4.VerifyCommand != "go build ./..." || b4.IndexCardCount != 1 {
		t.Errorf("batch 4 = %+v; want Number=4 VerifyCommand=\"go build ./...\" IndexCardCount=1", b4)
	}

	b5 := plan.Batches[4]
	if b5.Number != 5 || !b5.Oversized || b5.IndexCardCount != 1 {
		t.Errorf("batch 5 = %+v; want Number=5 Oversized=true IndexCardCount=1", b5)
	}
}

// TestParsePlan_OtherFixturesParseCleanly confirms the plan-unapproved and
// plan-broken-chain fixtures are each well-formed at the ParsePlan level —
// they are designed to trip Validate's checks (a design-level gate), never
// ParsePlan's fail-loud parse errors.
func TestParsePlan_OtherFixturesParseCleanly(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"plan-unapproved", "plan-broken-chain"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			_, err := builderengine.ParsePlan(filepath.Join("testdata", name))
			if err != nil {
				t.Fatalf("ParsePlan(testdata/%s) error = %v; want nil", name, err)
			}
		})
	}
}
