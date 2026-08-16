// parse_test.go covers ParsePlan's overview-parsing behavior (frontmatter decoding, Card Index
// parsing, framing extraction), its per-card file-parsing behavior (the title heading, the typed
// per-card model, Depends-on, Commit, and verify:), the none-vs-nil field distinction, and a full
// round-trip over the contracts/specs/loom-plan-spec.md worked-example golden fixture
// (testdata/goodplan).

package planparser_test

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/planparser"
)

// writePlanFiles writes every entry of files (keyed by filename, e.g.
// "00-overview.md") into a fresh temp plan directory and returns that directory's
// path.
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

// minimalOverview is a syntactically complete overview with a single Card Index
// entry, used as the base fixture for tests that don't care about framing or
// plan-level sections.
const minimalOverview = `---
format: 3
approved: true
---

# Plan: minimal

Framing paragraph.

## Card Index

1 — only — the only card
`

// minimalCardFile is a syntactically complete card file body: all five typed
// file-op fields plus Depends-on carry "none" except a single Edits: bullet.
func minimalCardFile(number int, name, editsPath string) string {
	return fmt.Sprintf("# Card %d — %s\n\n", number, name) +
		"**What:** placeholder card.\n" +
		"**Context:** none\n" +
		"**Edits:**\n- `" + editsPath + "`\n" +
		"**Creates:** none\n" +
		"**Deletes:** none\n" +
		"**Moves:** none\n" +
		"**Depends-on:** none\n"
}

func TestParsePlan_Overview(t *testing.T) {
	t.Parallel()

	dir := writePlanFiles(t, map[string]string{
		"00-overview.md": minimalOverview,
		"01-only.md":     minimalCardFile(1, "only", "a.go"),
	})

	plan, err := planparser.ParsePlan(dir)
	if err != nil {
		t.Fatalf("ParsePlan(%q) error = %v; want nil", dir, err)
	}

	if plan.Dir != dir {
		t.Errorf("plan.Dir = %q; want %q", plan.Dir, dir)
	}
	if plan.Format != 3 {
		t.Errorf("plan.Format = %d; want 3", plan.Format)
	}
	if !plan.Approved {
		t.Errorf("plan.Approved = false; want true")
	}
	if plan.Root != "" {
		t.Errorf("plan.Root = %q; want empty (no root: key)", plan.Root)
	}
	wantFraming := "Framing paragraph."
	if plan.Framing != wantFraming {
		t.Errorf("plan.Framing = %q; want %q", plan.Framing, wantFraming)
	}
	if len(plan.Cards) != 1 {
		t.Fatalf("len(plan.Cards) = %d; want 1", len(plan.Cards))
	}
	if plan.Cards[0].Number != 1 || plan.Cards[0].Slug != "only" || plan.Cards[0].Intent != "the only card" {
		t.Errorf("plan.Cards[0] Number/Slug/Intent = %d/%q/%q; want 1/only/%q", plan.Cards[0].Number, plan.Cards[0].Slug, plan.Cards[0].Intent, "the only card")
	}
}

func TestParsePlan_Overview_ASCIIDashSeparators(t *testing.T) {
	t.Parallel()

	const overview = `---
format: 3
approved: true
---

# Plan: ascii dash variant

Framing paragraph.

## Card Index

1 - single-dash - intent using a single ASCII hyphen
2 -- double-dash -- intent using a double ASCII hyphen
`
	dir := writePlanFiles(t, map[string]string{
		"00-overview.md":    overview,
		"01-single-dash.md": minimalCardFile(1, "single-dash", "a.go"),
		"02-double-dash.md": minimalCardFile(2, "double-dash", "b.go"),
	})

	plan, err := planparser.ParsePlan(dir)
	if err != nil {
		t.Fatalf("ParsePlan(%q) error = %v; want nil", dir, err)
	}
	if len(plan.Cards) != 2 {
		t.Fatalf("len(plan.Cards) = %d; want 2", len(plan.Cards))
	}
	if plan.Cards[0].Slug != "single-dash" {
		t.Errorf("plan.Cards[0].Slug = %q; want %q", plan.Cards[0].Slug, "single-dash")
	}
	if plan.Cards[1].Slug != "double-dash" {
		t.Errorf("plan.Cards[1].Slug = %q; want %q", plan.Cards[1].Slug, "double-dash")
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
			content:    "# Plan: no frontmatter\n\nFraming.\n\n## Card Index\n\n1 — a — b\n",
			wantSubstr: "missing required frontmatter",
		},
		{
			name:       "unknown frontmatter key",
			content:    "---\nformat: 3\napproved: true\nextra: true\n---\n\n# Plan\n\nFraming.\n\n## Card Index\n\n1 — a — b\n",
			wantSubstr: "field extra not found",
		},
		{
			name:       "duplicate frontmatter key",
			content:    "---\nformat: 3\nformat: 3\napproved: true\n---\n\n# Plan\n\nFraming.\n\n## Card Index\n\n1 — a — b\n",
			wantSubstr: "already defined",
		},
		{
			name:       "unterminated frontmatter fence",
			content:    "---\nformat: 3\napproved: true\n\n# Plan\n\nFraming.\n\n## Card Index\n\n1 — a — b\n",
			wantSubstr: "unterminated frontmatter fence",
		},
		{
			name:       "missing card index heading",
			content:    "---\nformat: 3\napproved: true\n---\n\n# Plan\n\nFraming.\n",
			wantSubstr: `missing "## Card Index" heading`,
		},
		{
			name:       "unparseable card index line",
			content:    "---\nformat: 3\napproved: true\n---\n\n# Plan\n\nFraming.\n\n## Card Index\n\nnot a valid entry\n",
			wantSubstr: "unparseable card index line",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var dir string
			if tt.noFile {
				dir = t.TempDir()
			} else {
				dir = writePlanFiles(t, map[string]string{"00-overview.md": tt.content})
			}

			_, err := planparser.ParsePlan(dir)
			if err == nil {
				t.Fatalf("ParsePlan(%q) error = nil; want error containing %q", dir, tt.wantSubstr)
			}
			if !strings.Contains(err.Error(), tt.wantSubstr) {
				t.Errorf("ParsePlan(%q) error = %q; want substring %q", dir, err.Error(), tt.wantSubstr)
			}
			if !strings.HasPrefix(err.Error(), "planparser:") {
				t.Errorf("ParsePlan(%q) error = %q; want \"planparser:\" prefix", dir, err.Error())
			}
		})
	}
}

func TestParsePlan_Overview_MissingFormatOrApprovedIsNotFailLoud(t *testing.T) {
	t.Parallel()

	// A missing format:/approved: key is not a ParsePlan failure —
	// format-unrecognized/plan-unapproved are Validate's checks, not the parser's; a plan
	// simply parses with the zero value.
	dir := writePlanFiles(t, map[string]string{
		"00-overview.md": "---\n{}\n---\n\n# Plan\n\nFraming.\n\n## Card Index\n\n1 — only — the only card\n",
		"01-only.md":     minimalCardFile(1, "only", "a.go"),
	})

	plan, err := planparser.ParsePlan(dir)
	if err != nil {
		t.Fatalf("ParsePlan(%q) error = %v; want nil", dir, err)
	}
	if plan.Format != 0 {
		t.Errorf("plan.Format = %d; want 0 (absent)", plan.Format)
	}
	if plan.Approved {
		t.Errorf("plan.Approved = true; want false (absent)")
	}
}

func TestParsePlan_CardFile_NotFound(t *testing.T) {
	t.Parallel()

	dir := writePlanFiles(t, map[string]string{"00-overview.md": minimalOverview})
	_, err := planparser.ParsePlan(dir)
	if err == nil {
		t.Fatal("ParsePlan() error = nil; want card-file-not-found error")
	}
	if !strings.Contains(err.Error(), "card file not found") {
		t.Errorf("ParsePlan() error = %q; want card-file-not-found substring", err.Error())
	}
}

func TestParsePlan_CardHeading(t *testing.T) {
	t.Parallel()

	t.Run("em dash separator", func(t *testing.T) {
		t.Parallel()

		dir := writePlanFiles(t, map[string]string{
			"00-overview.md": minimalOverview,
			"01-only.md":     "# Card 1 — flag + row struct\n\n**Context:** none\n**Edits:** none\n**Creates:** none\n**Deletes:** none\n**Moves:** none\n**Depends-on:** none\n",
		})
		plan, err := planparser.ParsePlan(dir)
		if err != nil {
			t.Fatalf("ParsePlan() error = %v; want nil", err)
		}
		if plan.Cards[0].Title != "flag + row struct" {
			t.Errorf("plan.Cards[0].Title = %q; want %q", plan.Cards[0].Title, "flag + row struct")
		}
	})

	t.Run("ASCII hyphen separator", func(t *testing.T) {
		t.Parallel()

		dir := writePlanFiles(t, map[string]string{
			"00-overview.md": minimalOverview,
			"01-only.md":     "# Card 1 -- flag + row struct\n\n**Context:** none\n**Edits:** none\n**Creates:** none\n**Deletes:** none\n**Moves:** none\n**Depends-on:** none\n",
		})
		plan, err := planparser.ParsePlan(dir)
		if err != nil {
			t.Fatalf("ParsePlan() error = %v; want nil", err)
		}
		if plan.Cards[0].Title != "flag + row struct" {
			t.Errorf("plan.Cards[0].Title = %q; want %q", plan.Cards[0].Title, "flag + row struct")
		}
	})

	t.Run("unrecognized heading is a parse error", func(t *testing.T) {
		t.Parallel()

		dir := writePlanFiles(t, map[string]string{
			"00-overview.md": minimalOverview,
			"01-only.md":     "not a card heading at all\n",
		})
		_, err := planparser.ParsePlan(dir)
		if err == nil {
			t.Fatal("ParsePlan() error = nil; want a parse error for the unrecognized heading")
		}
		if !strings.Contains(err.Error(), "unrecognized card heading") {
			t.Errorf("ParsePlan() error = %q; want unrecognized-card-heading substring", err.Error())
		}
	})
}

// TestParsePlan_Card_FiveFieldsNoneSentinel covers the three-way distinction plan-format pins
// for each of the five typed file-op fields (and Depends-on): absent entirely (nil slice, HasX ==
// false), present with the literal "none" (empty non-nil slice, HasX == true), and present with
// entries (populated non-nil slice, HasX == true).
func TestParsePlan_Card_FiveFieldsNoneSentinel(t *testing.T) {
	t.Parallel()

	t.Run("all none", func(t *testing.T) {
		t.Parallel()

		body := "# Card 1 — none everywhere\n\n**What:** nothing.\n**Context:** none\n**Edits:** none\n" +
			"**Creates:** none\n**Deletes:** none\n**Moves:** none\n**Depends-on:** none\n"
		dir := writePlanFiles(t, map[string]string{"00-overview.md": minimalOverview, "01-only.md": body})
		plan, err := planparser.ParsePlan(dir)
		if err != nil {
			t.Fatalf("ParsePlan() error = %v; want nil", err)
		}
		card := plan.Cards[0]

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
		if card.DependsOn == nil || len(card.DependsOn) != 0 {
			t.Errorf("card.DependsOn = %v; want empty non-nil slice", card.DependsOn)
		}
		if !card.HasContext || !card.HasEdits || !card.HasCreates || !card.HasDeletes || !card.HasMoves || !card.HasDependsOn || !card.HasWhat {
			t.Errorf("card Has* = %+v; want all true (every field's label was present)", card)
		}
	})

	t.Run("What prose is captured verbatim", func(t *testing.T) {
		t.Parallel()

		// The prose is the implementer's concrete instruction; the fork prompt
		// renders it verbatim, so the parser must store it — not just record
		// its presence (found in crucible round fable-r3: the prompt rendered
		// the index one-liner and the prose was silently dropped).
		body := "# Card 1 — prose\n\n**What:** First line of the instruction,\n" +
			"and a second line with detail.\n**Context:** none\n**Edits:** none\n" +
			"**Creates:** none\n**Deletes:** none\n**Moves:** none\n**Depends-on:** none\n"
		dir := writePlanFiles(t, map[string]string{"00-overview.md": minimalOverview, "01-only.md": body})
		plan, err := planparser.ParsePlan(dir)
		if err != nil {
			t.Fatalf("ParsePlan() error = %v; want nil", err)
		}
		card := plan.Cards[0]

		want := "First line of the instruction,\nand a second line with detail."
		if card.What != want {
			t.Errorf("card.What = %q; want %q", card.What, want)
		}
	})

	t.Run("field absent entirely", func(t *testing.T) {
		t.Parallel()

		// Edits: and Depends-on: are entirely missing (no label line at all) —
		// this is a card-level defect (Validate's card-missing-field
		// territory), not a parse error, per the lenient-card-parse decision.
		body := "# Card 1 — missing fields\n\n**Context:** none\n**Creates:** none\n**Deletes:** none\n**Moves:** none\n"
		dir := writePlanFiles(t, map[string]string{"00-overview.md": minimalOverview, "01-only.md": body})
		plan, err := planparser.ParsePlan(dir)
		if err != nil {
			t.Fatalf("ParsePlan() error = %v; want nil", err)
		}
		card := plan.Cards[0]

		if card.EditsFiles != nil {
			t.Errorf("card.EditsFiles = %v; want nil (label never present)", card.EditsFiles)
		}
		if card.HasEdits {
			t.Errorf("card.HasEdits = true; want false")
		}
		if card.DependsOn != nil {
			t.Errorf("card.DependsOn = %v; want nil (label never present)", card.DependsOn)
		}
		if card.HasDependsOn {
			t.Errorf("card.HasDependsOn = true; want false")
		}
		if card.HasWhat {
			t.Errorf("card.HasWhat = true; want false (no **What:** label present)")
		}
	})

	t.Run("populated bullets and Depends-on ids", func(t *testing.T) {
		t.Parallel()

		body := "# Card 1 — populated\n\n" +
			"**Context:**\n- `a.go`\n- `b.go`\n" +
			"**Edits:**\n- `c.go`\n" +
			"**Creates:**\n- `d.go`\n" +
			"**Deletes:**\n- `e.go`\n" +
			"**Moves:** none\n" +
			"**Depends-on:** 2, 3\n"
		dir := writePlanFiles(t, map[string]string{
			"00-overview.md": "---\nformat: 3\napproved: true\n---\n\n# Plan\n\nFraming.\n\n## Card Index\n\n1 — only — a\n",
			"01-only.md":     body,
		})
		plan, err := planparser.ParsePlan(dir)
		if err != nil {
			t.Fatalf("ParsePlan() error = %v; want nil", err)
		}
		card := plan.Cards[0]

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
		if want := []int{2, 3}; !slices.Equal(card.DependsOn, want) {
			t.Errorf("card.DependsOn = %v; want %v", card.DependsOn, want)
		}
	})
}

// TestParsePlan_Card_MovesGrammar covers Moves: bullets: well-formed pairs land in Moves,
// and a bullet that fails the pair grammar is retained verbatim in MovesRaw rather than becoming a
// parse error (lenient-card-parse decision).
func TestParsePlan_Card_MovesGrammar(t *testing.T) {
	t.Parallel()

	body := "# Card 1 — moves\n\n**Context:** none\n**Edits:** none\n**Creates:** none\n**Deletes:** none\n" +
		"**Moves:**\n- `old/path.go` -> `new/path.go`\n- this bullet has no arrow at all\n**Depends-on:** none\n"
	dir := writePlanFiles(t, map[string]string{"00-overview.md": minimalOverview, "01-only.md": body})
	plan, err := planparser.ParsePlan(dir)
	if err != nil {
		t.Fatalf("ParsePlan() error = %v; want nil", err)
	}
	card := plan.Cards[0]

	wantPairs := []planparser.MovePair{{Old: "old/path.go", New: "new/path.go"}}
	if !slices.Equal(card.Moves, wantPairs) {
		t.Errorf("card.Moves = %+v; want %+v", card.Moves, wantPairs)
	}
	wantRaw := []string{"this bullet has no arrow at all"}
	if !slices.Equal(card.MovesRaw, wantRaw) {
		t.Errorf("card.MovesRaw = %v; want %v", card.MovesRaw, wantRaw)
	}
}

// TestParsePlan_Card_DependsOnMalformed covers a Depends-on token that fails to parse as a plain
// card number: document structure, so it fails loud rather than silently degrading (Validate's
// depends-on-order check never even sees a non-numeric token).
func TestParsePlan_Card_DependsOnMalformed(t *testing.T) {
	t.Parallel()

	body := "# Card 1 — bad deps\n\n**Context:** none\n**Edits:** none\n**Creates:** none\n" +
		"**Deletes:** none\n**Moves:** none\n**Depends-on:** first-card\n"
	dir := writePlanFiles(t, map[string]string{"00-overview.md": minimalOverview, "01-only.md": body})
	_, err := planparser.ParsePlan(dir)
	if err == nil {
		t.Fatal("ParsePlan() error = nil; want error for the non-numeric Depends-on token")
	}
	if !strings.Contains(err.Error(), "not a plain card number") {
		t.Errorf("ParsePlan() error = %q; want it to name the malformed token", err.Error())
	}
}

// TestParsePlan_InlineFieldValueFailsLoud proves a card file-op label line carrying an inline value
// other than "none" (e.g. "**Edits:** `foo.go`") is a fail-loud parse error, never silently read as
// an empty field.
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
			body := "# Card 1 — placeholder\n\n**Context:** none\n" + tt.fieldLine + "\n" +
				"**Creates:** none\n**Deletes:** none\n**Depends-on:** none\n"
			if strings.HasPrefix(tt.fieldLine, "**Moves:**") {
				body = strings.Replace(body, "**Context:** none\n", "**Context:** none\n**Edits:** none\n", 1)
			} else {
				body = strings.Replace(body, "**Deletes:** none\n", "**Deletes:** none\n**Moves:** none\n", 1)
			}

			dir := writePlanFiles(t, map[string]string{"00-overview.md": minimalOverview, "01-only.md": body})
			_, err := planparser.ParsePlan(dir)
			if err == nil {
				t.Fatalf("ParsePlan() error = nil; want a fail-loud inline-value error for %q", tt.fieldLine)
			}
			if !strings.Contains(err.Error(), "inline value") {
				t.Errorf("ParsePlan() error = %q; want it to name the inline value", err.Error())
			}
		})
	}
}

// TestParsePlan_Card_SourcePath proves each parsed card's SourcePath is the bare worktree-relative
// `_lyx/plan/NN-<slug>.md` token — never prefixed by the (t.TempDir()) absolute Plan.Dir the
// fixture is parsed from — for both a single-card and a multi-card plan.
func TestParsePlan_Card_SourcePath(t *testing.T) {
	t.Parallel()

	t.Run("single-card plan", func(t *testing.T) {
		t.Parallel()

		dir := writePlanFiles(t, map[string]string{
			"00-overview.md": minimalOverview,
			"01-only.md":     minimalCardFile(1, "only", "a.go"),
		})
		plan, err := planparser.ParsePlan(dir)
		if err != nil {
			t.Fatalf("ParsePlan(%q) error = %v; want nil", dir, err)
		}
		if len(plan.Cards) != 1 {
			t.Fatalf("len(plan.Cards) = %d; want 1", len(plan.Cards))
		}

		want := "_lyx/plan/01-only.md"
		got := plan.Cards[0].SourcePath
		if got != want {
			t.Errorf("plan.Cards[0].SourcePath = %q; want %q", got, want)
		}
		if strings.Contains(got, dir) {
			t.Errorf("plan.Cards[0].SourcePath = %q; leaks the absolute Plan.Dir %q", got, dir)
		}
		if strings.Contains(got, os.TempDir()) {
			t.Errorf("plan.Cards[0].SourcePath = %q; leaks the t.TempDir() temp path", got)
		}
	})

	t.Run("multi-card plan", func(t *testing.T) {
		t.Parallel()

		const overview = `---
format: 3
approved: true
---

# Plan: multi

Framing paragraph.

## Card Index

1 — first — the first card
2 — second — the second card
`
		dir := writePlanFiles(t, map[string]string{
			"00-overview.md": overview,
			"01-first.md":    minimalCardFile(1, "first", "a.go"),
			"02-second.md":   minimalCardFile(2, "second", "b.go"),
		})
		plan, err := planparser.ParsePlan(dir)
		if err != nil {
			t.Fatalf("ParsePlan(%q) error = %v; want nil", dir, err)
		}
		if len(plan.Cards) != 2 {
			t.Fatalf("len(plan.Cards) = %d; want 2", len(plan.Cards))
		}

		if want := "_lyx/plan/01-first.md"; plan.Cards[0].SourcePath != want {
			t.Errorf("plan.Cards[0].SourcePath = %q; want %q", plan.Cards[0].SourcePath, want)
		}
		if want := "_lyx/plan/02-second.md"; plan.Cards[1].SourcePath != want {
			t.Errorf("plan.Cards[1].SourcePath = %q; want %q", plan.Cards[1].SourcePath, want)
		}
		for _, c := range plan.Cards {
			if strings.Contains(c.SourcePath, dir) {
				t.Errorf("card %d SourcePath = %q; leaks the absolute Plan.Dir %q", c.Number, c.SourcePath, dir)
			}
		}
	})
}

func TestParsePlan_CardCommitAndVerify(t *testing.T) {
	t.Parallel()

	body := "# Card 1 — flag\n\n**Context:** none\n**Edits:** none\n**Creates:** none\n**Deletes:** none\n" +
		"**Moves:** none\n**Depends-on:** none\n**Commit:** `1: add the --json flag`\n**verify:** go build ./...\n"
	dir := writePlanFiles(t, map[string]string{"00-overview.md": minimalOverview, "01-only.md": body})
	plan, err := planparser.ParsePlan(dir)
	if err != nil {
		t.Fatalf("ParsePlan() error = %v; want nil", err)
	}
	card := plan.Cards[0]

	if card.Commit != "1: add the --json flag" {
		t.Errorf("card.Commit = %q; want %q", card.Commit, "1: add the --json flag")
	}
	if card.Verify != "go build ./..." {
		t.Errorf("card.Verify = %q; want %q", card.Verify, "go build ./...")
	}
}

// goodPlanDir is the contracts/specs/loom-plan-spec.md worked example, materialized
// verbatim as this package's golden happy-path fixture.
func goodPlanDir() string {
	return filepath.Join("testdata", "goodplan")
}

// TestParsePlan_GoldenFixture round-trips testdata/goodplan (the pinned spec's own worked example)
// exactly: the overview's frontmatter, framing, and every card's typed fields must match the
// fixture's own byte-consistent content, including the root: internal/boardcli resolution and the
// // worktree-root escape.
func TestParsePlan_GoldenFixture(t *testing.T) {
	t.Parallel()

	plan, err := planparser.ParsePlan(goodPlanDir())
	if err != nil {
		t.Fatalf("ParsePlan(%q) error = %v; want nil", goodPlanDir(), err)
	}

	if plan.Format != 3 {
		t.Errorf("plan.Format = %d; want 3", plan.Format)
	}
	if !plan.Approved {
		t.Errorf("plan.Approved = false; want true")
	}
	if plan.Root != "internal/boardcli" {
		t.Errorf("plan.Root = %q; want %q", plan.Root, "internal/boardcli")
	}
	wantFraming := "Add a `--json` output mode to `lyx board list`, emitting one JSON object per row via the\n" +
		"`internal/output` envelope, with tests and help text updated, and the row mapper relocated ahead\n" +
		"of a later extraction."
	if plan.Framing != wantFraming {
		t.Errorf("plan.Framing = %q; want %q", plan.Framing, wantFraming)
	}

	if len(plan.Cards) != 4 {
		t.Fatalf("len(plan.Cards) = %d; want 4", len(plan.Cards))
	}

	c1 := plan.Cards[0]
	if c1.Number != 1 || c1.Slug != "json-flag" || c1.Title != "json-flag" {
		t.Errorf("card 1 Number/Slug/Title = %d/%q/%q; want 1/json-flag/json-flag", c1.Number, c1.Slug, c1.Title)
	}
	if c1.Intent != "add the `--json` bool flag and RowJSON struct" {
		t.Errorf("card 1 Intent = %q; want the Card Index one-liner", c1.Intent)
	}
	if !c1.HasWhat {
		t.Errorf("card 1 HasWhat = false; want true")
	}
	wantWhat := "Add a `--json` bool flag to the list command; define `RowJSON` with the existing\n" +
		"table's columns as fields."
	if c1.What != wantWhat {
		t.Errorf("card 1 What = %q; want the card file's own multi-line prose %q", c1.What, wantWhat)
	}
	if want := []string{"internal/boardcli/list.go", "internal/boardengine/rows.go"}; !slices.Equal(c1.EditsFiles, want) {
		t.Errorf("card 1 EditsFiles = %v; want %v (root-joined + // escape)", c1.EditsFiles, want)
	}
	if c1.ContextFiles == nil || len(c1.ContextFiles) != 0 {
		t.Errorf("card 1 ContextFiles = %v; want empty non-nil (none)", c1.ContextFiles)
	}
	if c1.Moves == nil || len(c1.Moves) != 0 {
		t.Errorf("card 1 Moves = %v; want empty non-nil (none)", c1.Moves)
	}
	if c1.DependsOn == nil || len(c1.DependsOn) != 0 {
		t.Errorf("card 1 DependsOn = %v; want empty non-nil (none)", c1.DependsOn)
	}
	if c1.Commit != "1: json-flag" {
		t.Errorf("card 1 Commit = %q; want %q", c1.Commit, "1: json-flag")
	}
	if c1.Verify != "go build ./..." {
		t.Errorf("card 1 Verify = %q; want %q", c1.Verify, "go build ./...")
	}

	c2 := plan.Cards[1]
	if want := []string{"internal/output/envelope.go"}; !slices.Equal(c2.ContextFiles, want) {
		t.Errorf("card 2 ContextFiles = %v; want %v (// escape)", c2.ContextFiles, want)
	}
	if want := []string{"internal/boardcli/list.go"}; !slices.Equal(c2.EditsFiles, want) {
		t.Errorf("card 2 EditsFiles = %v; want %v", c2.EditsFiles, want)
	}
	if want := []int{1}; !slices.Equal(c2.DependsOn, want) {
		t.Errorf("card 2 DependsOn = %v; want %v", c2.DependsOn, want)
	}
	if c2.Commit != "" {
		t.Errorf("card 2 Commit = %q; want empty (field absent)", c2.Commit)
	}
	if c2.Verify != "" {
		t.Errorf("card 2 Verify = %q; want empty (field absent)", c2.Verify)
	}

	c3 := plan.Cards[2]
	if want := []string{"internal/boardcli/list_test.go"}; !slices.Equal(c3.EditsFiles, want) {
		t.Errorf("card 3 EditsFiles = %v; want %v", c3.EditsFiles, want)
	}
	if want := []int{2}; !slices.Equal(c3.DependsOn, want) {
		t.Errorf("card 3 DependsOn = %v; want %v", c3.DependsOn, want)
	}

	c4 := plan.Cards[3]
	if want := []string{"cmd/lyx/helptree_test.go"}; !slices.Equal(c4.EditsFiles, want) {
		t.Errorf("card 4 EditsFiles = %v; want %v (// escape)", c4.EditsFiles, want)
	}
	wantMoves := []planparser.MovePair{{Old: "internal/boardengine/rows.go", New: "internal/boardengine/rowsjson.go"}}
	if !slices.Equal(c4.Moves, wantMoves) {
		t.Errorf("card 4 Moves = %+v; want %+v (both sides // escaped)", c4.Moves, wantMoves)
	}
	if want := []int{1}; !slices.Equal(c4.DependsOn, want) {
		t.Errorf("card 4 DependsOn = %v; want %v", c4.DependsOn, want)
	}
}
