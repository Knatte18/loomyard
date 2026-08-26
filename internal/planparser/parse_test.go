// parse_test.go covers ParsePlan's overview-parsing behavior (frontmatter decoding, Card Index
// parsing, framing extraction), its per-card file-parsing behavior (the title heading, the
// format-4 one-or-more type-label model and its per-label TargetGroups, Uses:/Intent:/
// ImpactSummary:, retired-label routing, and Commit:/Verify:), the label-present-vs-absent field
// distinction, and a full round-trip over the format-4 golden fixture (testdata/goodplan).

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

// minimalOverview is a syntactically complete format-4 overview with a single Card Index
// entry, used as the base fixture for tests that don't care about framing or
// plan-level sections.
const minimalOverview = `---
format: 4
approved: true
---

# Plan: minimal

Framing paragraph.

## Card Index

1 — only — the only card
`

// minimalCardFile is a syntactically complete format-4 card file body: one type label carrying a
// single bullet, plus an **Intent:** line — no "none" sentinel anywhere, since format 4 has none.
func minimalCardFile(number int, name, editPath string) string {
	return fmt.Sprintf("# Card %d — %s\n\n", number, name) +
		"**Edit:**\n- `" + editPath + "`\n" +
		"**Intent:** placeholder card.\n"
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
	if plan.Format != 4 {
		t.Errorf("plan.Format = %d; want 4", plan.Format)
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
	if plan.Cards[0].Number != 1 || plan.Cards[0].Slug != "only" || plan.Cards[0].Summary != "the only card" {
		t.Errorf("plan.Cards[0] Number/Slug/Summary = %d/%q/%q; want 1/only/%q", plan.Cards[0].Number, plan.Cards[0].Slug, plan.Cards[0].Summary, "the only card")
	}
}

func TestParsePlan_Overview_ASCIIDashSeparators(t *testing.T) {
	t.Parallel()

	const overview = `---
format: 4
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
			content:    "---\nformat: 4\napproved: true\nextra: true\n---\n\n# Plan\n\nFraming.\n\n## Card Index\n\n1 — a — b\n",
			wantSubstr: "field extra not found",
		},
		{
			name:       "duplicate frontmatter key",
			content:    "---\nformat: 4\nformat: 4\napproved: true\n---\n\n# Plan\n\nFraming.\n\n## Card Index\n\n1 — a — b\n",
			wantSubstr: "already defined",
		},
		{
			name:       "unterminated frontmatter fence",
			content:    "---\nformat: 4\napproved: true\n\n# Plan\n\nFraming.\n\n## Card Index\n\n1 — a — b\n",
			wantSubstr: "unterminated frontmatter fence",
		},
		{
			name:       "missing card index heading",
			content:    "---\nformat: 4\napproved: true\n---\n\n# Plan\n\nFraming.\n",
			wantSubstr: `missing "## Card Index" heading`,
		},
		{
			name:       "unparseable card index line",
			content:    "---\nformat: 4\napproved: true\n---\n\n# Plan\n\nFraming.\n\n## Card Index\n\nnot a valid entry\n",
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
			"01-only.md":     "# Card 1 — flag + row struct\n\n**Edit:**\n- `a.go`\n**Intent:** placeholder.\n",
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
			"01-only.md":     "# Card 1 -- flag + row struct\n\n**Edit:**\n- `a.go`\n**Intent:** placeholder.\n",
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

// TestParsePlan_Card_TypeLabelCount covers TypeLabelCount/Type/HasType/TargetGroups bookkeeping.
// Two recognized type labels on one card — even the same label twice — is the supported
// one-or-more shape, not a defect; the zero-labels shape stays a defect card-type-missing catches.
func TestParsePlan_Card_TypeLabelCount(t *testing.T) {
	t.Parallel()

	t.Run("two type labels", func(t *testing.T) {
		t.Parallel()

		body := "# Card 1 — dual\n\n**Edit:**\n- `a.go`\n**Delete:**\n- `b.go`\n**Intent:** placeholder.\n"
		dir := writePlanFiles(t, map[string]string{"00-overview.md": minimalOverview, "01-only.md": body})
		plan, err := planparser.ParsePlan(dir)
		if err != nil {
			t.Fatalf("ParsePlan() error = %v; want nil", err)
		}
		card := plan.Cards[0]

		if card.TypeLabelCount != 2 {
			t.Errorf("card.TypeLabelCount = %d; want 2", card.TypeLabelCount)
		}
		if !card.HasType {
			t.Errorf("card.HasType = false; want true")
		}
		if card.Type != planparser.CardTypeEdit {
			t.Errorf("card.Type = %q; want %q (the first type label seen)", card.Type, planparser.CardTypeEdit)
		}
		if len(card.TargetGroups) != 2 {
			t.Fatalf("len(card.TargetGroups) = %d; want 2", len(card.TargetGroups))
		}
		if card.TargetGroups[0].Type != planparser.CardTypeEdit {
			t.Errorf("card.TargetGroups[0].Type = %q; want %q", card.TargetGroups[0].Type, planparser.CardTypeEdit)
		}
		if card.TargetGroups[1].Type != planparser.CardTypeDelete {
			t.Errorf("card.TargetGroups[1].Type = %q; want %q", card.TargetGroups[1].Type, planparser.CardTypeDelete)
		}
		wantRefs0 := []string{"a.go"}
		if !slices.Equal(card.TargetGroups[0].Refs, wantRefs0) {
			t.Errorf("card.TargetGroups[0].Refs = %v; want %v", card.TargetGroups[0].Refs, wantRefs0)
		}
		wantRefs1 := []string{"b.go"}
		if !slices.Equal(card.TargetGroups[1].Refs, wantRefs1) {
			t.Errorf("card.TargetGroups[1].Refs = %v; want %v", card.TargetGroups[1].Refs, wantRefs1)
		}
		wantTargets := []string{"a.go", "b.go"}
		if !slices.Equal(card.Targets, wantTargets) {
			t.Errorf("card.Targets = %v; want %v (concatenation of both groups' Refs, body order)", card.Targets, wantTargets)
		}
	})

	t.Run("no type label", func(t *testing.T) {
		t.Parallel()

		body := "# Card 1 — typeless\n\n**Intent:** placeholder.\n"
		dir := writePlanFiles(t, map[string]string{"00-overview.md": minimalOverview, "01-only.md": body})
		plan, err := planparser.ParsePlan(dir)
		if err != nil {
			t.Fatalf("ParsePlan() error = %v; want nil", err)
		}
		card := plan.Cards[0]

		if card.TypeLabelCount != 0 {
			t.Errorf("card.TypeLabelCount = %d; want 0", card.TypeLabelCount)
		}
		if card.HasType {
			t.Errorf("card.HasType = true; want false")
		}
		if card.Type != planparser.CardTypeUnknown {
			t.Errorf("card.Type = %q; want %q", card.Type, planparser.CardTypeUnknown)
		}
		if len(card.TargetGroups) != 0 {
			t.Errorf("len(card.TargetGroups) = %d; want 0", len(card.TargetGroups))
		}
	})

	t.Run("single-label card produces exactly one group", func(t *testing.T) {
		t.Parallel()

		dir := writePlanFiles(t, map[string]string{
			"00-overview.md": minimalOverview,
			"01-only.md":     minimalCardFile(1, "only", "a.go"),
		})
		plan, err := planparser.ParsePlan(dir)
		if err != nil {
			t.Fatalf("ParsePlan() error = %v; want nil", err)
		}
		card := plan.Cards[0]

		if len(card.TargetGroups) != 1 {
			t.Fatalf("len(card.TargetGroups) = %d; want 1", len(card.TargetGroups))
		}
		if card.TargetGroups[0].Type != planparser.CardTypeEdit {
			t.Errorf("card.TargetGroups[0].Type = %q; want %q", card.TargetGroups[0].Type, planparser.CardTypeEdit)
		}
	})

	t.Run("repeated label produces two groups whose union equals one merged group's refs", func(t *testing.T) {
		t.Parallel()

		body := "# Card 1 — repeated label\n\n**Edit:**\n- `a.go`\n**Edit:**\n- `b.go`\n**Intent:** placeholder.\n"
		dir := writePlanFiles(t, map[string]string{"00-overview.md": minimalOverview, "01-only.md": body})
		plan, err := planparser.ParsePlan(dir)
		if err != nil {
			t.Fatalf("ParsePlan() error = %v; want nil", err)
		}
		card := plan.Cards[0]

		if len(card.TargetGroups) != 2 {
			t.Fatalf("len(card.TargetGroups) = %d; want 2", len(card.TargetGroups))
		}
		var union []string
		for _, g := range card.TargetGroups {
			if g.Type != planparser.CardTypeEdit {
				t.Errorf("group.Type = %q; want %q", g.Type, planparser.CardTypeEdit)
			}
			union = append(union, g.Refs...)
		}
		wantUnion := []string{"a.go", "b.go"}
		if !slices.Equal(union, wantUnion) {
			t.Errorf("union of both groups' Refs = %v; want %v (equal to one merged group's refs)", union, wantUnion)
		}
	})

	t.Run("two Rename labels give each group its own Pairs", func(t *testing.T) {
		t.Parallel()

		body := "# Card 1 — two renames\n\n" +
			"**Rename:**\n- `old1.Symbol` -> `new1.Symbol`\n" +
			"**Rename:**\n- `old2.Symbol` -> `new2.Symbol`\n" +
			"**Intent:** placeholder.\n"
		dir := writePlanFiles(t, map[string]string{"00-overview.md": minimalOverview, "01-only.md": body})
		plan, err := planparser.ParsePlan(dir)
		if err != nil {
			t.Fatalf("ParsePlan() error = %v; want nil", err)
		}
		card := plan.Cards[0]

		if len(card.TargetGroups) != 2 {
			t.Fatalf("len(card.TargetGroups) = %d; want 2", len(card.TargetGroups))
		}
		wantPairs0 := []planparser.MovePair{{Old: "old1.Symbol", New: "new1.Symbol"}}
		if !slices.Equal(card.TargetGroups[0].Pairs, wantPairs0) {
			t.Errorf("card.TargetGroups[0].Pairs = %+v; want %+v", card.TargetGroups[0].Pairs, wantPairs0)
		}
		wantPairs1 := []planparser.MovePair{{Old: "old2.Symbol", New: "new2.Symbol"}}
		if !slices.Equal(card.TargetGroups[1].Pairs, wantPairs1) {
			t.Errorf("card.TargetGroups[1].Pairs = %+v; want %+v", card.TargetGroups[1].Pairs, wantPairs1)
		}
		wantCardPairs := append(append([]planparser.MovePair{}, wantPairs0...), wantPairs1...)
		if !slices.Equal(card.Pairs, wantCardPairs) {
			t.Errorf("card.Pairs = %+v; want %+v (concatenation of both groups' Pairs, body order)", card.Pairs, wantCardPairs)
		}
	})
}

// TestParsePlan_Card_UsesPresentNoBullets proves a "**Uses:**" label present with zero bullets
// under it parses to a non-nil zero-length slice, distinguishing it from an absent label (nil,
// HasUses false).
func TestParsePlan_Card_UsesPresentNoBullets(t *testing.T) {
	t.Parallel()

	body := "# Card 1 — empty uses\n\n**Edit:**\n- `a.go`\n**Uses:**\n**Intent:** placeholder.\n"
	dir := writePlanFiles(t, map[string]string{"00-overview.md": minimalOverview, "01-only.md": body})
	plan, err := planparser.ParsePlan(dir)
	if err != nil {
		t.Fatalf("ParsePlan() error = %v; want nil", err)
	}
	card := plan.Cards[0]

	if !card.HasUses {
		t.Errorf("card.HasUses = false; want true")
	}
	if card.Uses == nil {
		t.Errorf("card.Uses = nil; want a non-nil, zero-length slice")
	}
	if len(card.Uses) != 0 {
		t.Errorf("card.Uses = %v; want empty", card.Uses)
	}
}

// TestParsePlan_Card_ImpactSummaryMultiline covers "**ImpactSummary:**"'s inline-remainder-plus-
// trailing-lines capture: the label line's own remainder lands in ImpactSummary, and every
// following non-label line lands in ImpactSummaryTrailing — captured rather than discarded so
// impact-summary-multiline has something to report.
func TestParsePlan_Card_ImpactSummaryMultiline(t *testing.T) {
	t.Parallel()

	body := "# Card 1 — multiline impact\n\n**Edit:**\n- `a.go`\n" +
		"**ImpactSummary:** first line.\nsecond line.\nthird line.\n**Intent:** placeholder.\n"
	dir := writePlanFiles(t, map[string]string{"00-overview.md": minimalOverview, "01-only.md": body})
	plan, err := planparser.ParsePlan(dir)
	if err != nil {
		t.Fatalf("ParsePlan() error = %v; want nil", err)
	}
	card := plan.Cards[0]

	if card.ImpactSummary != "first line." {
		t.Errorf("card.ImpactSummary = %q; want %q", card.ImpactSummary, "first line.")
	}
	wantTrailing := []string{"second line.", "third line."}
	if !slices.Equal(card.ImpactSummaryTrailing, wantTrailing) {
		t.Errorf("card.ImpactSummaryTrailing = %v; want %v", card.ImpactSummaryTrailing, wantTrailing)
	}
}

// TestParsePlan_Card_RetiredLabel_Context covers a half-migrated card carrying the retired
// "**Context:**" label: its literal text lands in RetiredLabels, and its presence terminates the
// preceding "**Intent:**" prose collection rather than being swallowed into it.
func TestParsePlan_Card_RetiredLabel_Context(t *testing.T) {
	t.Parallel()

	body := "# Card 1 — half-migrated\n\n**Intent:** prose before context.\n**Context:**\n- `a.go`\n"
	dir := writePlanFiles(t, map[string]string{"00-overview.md": minimalOverview, "01-only.md": body})
	plan, err := planparser.ParsePlan(dir)
	if err != nil {
		t.Fatalf("ParsePlan() error = %v; want nil", err)
	}
	card := plan.Cards[0]

	if card.Intent != "prose before context." {
		t.Errorf("card.Intent = %q; want %q (Context: must terminate collection, not be swallowed)", card.Intent, "prose before context.")
	}
	wantRetired := []string{"**Context:**"}
	if !slices.Equal(card.RetiredLabels, wantRetired) {
		t.Errorf("card.RetiredLabels = %v; want %v", card.RetiredLabels, wantRetired)
	}
}

// TestParsePlan_Card_RetiredLabel_LowercaseVerify covers a half-migrated card carrying format-3's
// lowercase "**verify:**" label: the case-sensitive match routes it to RetiredLabels rather than
// falling through as unrecognized text or being mistaken for format-4's "**Verify:**" field, and
// its presence terminates the preceding "**Intent:**" prose collection.
func TestParsePlan_Card_RetiredLabel_LowercaseVerify(t *testing.T) {
	t.Parallel()

	body := "# Card 1 — half-migrated\n\n**Intent:** prose before verify.\n**verify:** go test ./...\n"
	dir := writePlanFiles(t, map[string]string{"00-overview.md": minimalOverview, "01-only.md": body})
	plan, err := planparser.ParsePlan(dir)
	if err != nil {
		t.Fatalf("ParsePlan() error = %v; want nil", err)
	}
	card := plan.Cards[0]

	if card.Intent != "prose before verify." {
		t.Errorf("card.Intent = %q; want %q (verify: must terminate collection, not be swallowed)", card.Intent, "prose before verify.")
	}
	wantRetired := []string{"**verify:**"}
	if !slices.Equal(card.RetiredLabels, wantRetired) {
		t.Errorf("card.RetiredLabels = %v; want %v", card.RetiredLabels, wantRetired)
	}
	if card.HasVerify {
		t.Errorf("card.HasVerify = true; want false (lowercase verify: is not the format-4 Verify: field)")
	}
	if card.Verify != "" {
		t.Errorf("card.Verify = %q; want empty", card.Verify)
	}
}

// TestParsePlan_Card_RenameGrammar covers a Rename card's "**Rename:**" field: a well-formed
// "`old` -> `new`" sub-bullet reaches Pairs, a malformed sub-bullet reaches RenameRaw rather than
// becoming a parse error (lenient-card-parse decision), and both endpoints of every pair are
// projected into Targets in pair order, Old before New.
func TestParsePlan_Card_RenameGrammar(t *testing.T) {
	t.Parallel()

	body := "# Card 1 — rename\n\n**Rename:**\n- `old.Symbol` -> `new.Symbol`\n- this bullet has no arrow at all\n" +
		"**Intent:** placeholder.\n"
	dir := writePlanFiles(t, map[string]string{"00-overview.md": minimalOverview, "01-only.md": body})
	plan, err := planparser.ParsePlan(dir)
	if err != nil {
		t.Fatalf("ParsePlan() error = %v; want nil", err)
	}
	card := plan.Cards[0]

	wantPairs := []planparser.MovePair{{Old: "old.Symbol", New: "new.Symbol"}}
	if !slices.Equal(card.Pairs, wantPairs) {
		t.Errorf("card.Pairs = %+v; want %+v", card.Pairs, wantPairs)
	}
	wantRaw := []string{"this bullet has no arrow at all"}
	if !slices.Equal(card.RenameRaw, wantRaw) {
		t.Errorf("card.RenameRaw = %v; want %v", card.RenameRaw, wantRaw)
	}
	wantTargets := []string{"old.Symbol", "new.Symbol"}
	if !slices.Equal(card.Targets, wantTargets) {
		t.Errorf("card.Targets = %v; want %v (Old before New, pair order)", card.Targets, wantTargets)
	}
}

// TestParsePlan_InlineFieldValueFailsLoud proves a bullet-only field carrying an inline value
// (e.g. "**Edit:** `foo.go`") is a fail-loud ParsePlan error, never silently read as an empty
// field — for both a type label and "**Uses:**".
func TestParsePlan_InlineFieldValueFailsLoud(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		body string
	}{
		{
			name: "inline value on a type label",
			body: "# Card 1 — placeholder\n\n**Edit:** `list.go`\n**Intent:** placeholder.\n",
		},
		{
			name: "inline value on Uses:",
			body: "# Card 1 — placeholder\n\n**Edit:**\n- `list.go`\n**Uses:** `list.go`\n**Intent:** placeholder.\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := writePlanFiles(t, map[string]string{"00-overview.md": minimalOverview, "01-only.md": tt.body})
			_, err := planparser.ParsePlan(dir)
			if err == nil {
				t.Fatalf("ParsePlan() error = nil; want a fail-loud inline-value error")
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
format: 4
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

// TestParsePlan_CardCommitAndVerify covers a card's optional "**Commit:**" and recapitalized
// "**Verify:**" fields, and that HasVerify reports the label's presence.
func TestParsePlan_CardCommitAndVerify(t *testing.T) {
	t.Parallel()

	body := "# Card 1 — flag\n\n**Edit:**\n- `a.go`\n**Intent:** placeholder.\n" +
		"**Commit:** `1: add the --json flag`\n**Verify:** go build ./...\n"
	dir := writePlanFiles(t, map[string]string{"00-overview.md": minimalOverview, "01-only.md": body})
	plan, err := planparser.ParsePlan(dir)
	if err != nil {
		t.Fatalf("ParsePlan() error = %v; want nil", err)
	}
	card := plan.Cards[0]

	if card.Commit != "1: add the --json flag" {
		t.Errorf("card.Commit = %q; want %q", card.Commit, "1: add the --json flag")
	}
	if !card.HasVerify {
		t.Errorf("card.HasVerify = false; want true")
	}
	if card.Verify != "go build ./..." {
		t.Errorf("card.Verify = %q; want %q", card.Verify, "go build ./...")
	}
}

// goodPlanDir is this package's format-4 golden happy-path fixture, exercising all seven card
// types.
func goodPlanDir() string {
	return filepath.Join("testdata", "goodplan")
}

// TestParsePlan_GoldenFixture round-trips testdata/goodplan exactly: the overview's frontmatter,
// framing, and every field of every one of the seven cards must match the fixture's own
// byte-consistent content, including the root: internal/boardcli resolution and the // worktree-
// root escape. It also pins this migration's sharpest possible regression: a symbol target must
// survive normalization unmodified under the fixture's non-empty root:.
func TestParsePlan_GoldenFixture(t *testing.T) {
	t.Parallel()

	plan, err := planparser.ParsePlan(goodPlanDir())
	if err != nil {
		t.Fatalf("ParsePlan(%q) error = %v; want nil", goodPlanDir(), err)
	}

	if plan.Format != 4 {
		t.Errorf("plan.Format = %d; want 4", plan.Format)
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

	if len(plan.Cards) != 7 {
		t.Fatalf("len(plan.Cards) = %d; want 7", len(plan.Cards))
	}

	type wantCard struct {
		number           int
		slug             string
		summary          string
		typ              planparser.CardType
		typeLabelCount   int
		targets          []string
		pairs            []planparser.MovePair
		uses             []string
		hasUses          bool
		intent           string
		impactSummary    string
		hasImpactSummary bool
		commit           string
		verify           string
		hasVerify        bool
	}

	wants := []wantCard{
		{
			number: 1, slug: "json-row-type", summary: "define the RowJSON struct",
			typ: planparser.CardTypeCreate, typeLabelCount: 1,
			targets: []string{"boardcli.RowJSON"},
			intent:  "Define the `RowJSON` struct carrying the list command's existing table columns as JSON-taggable fields.",
			commit:  "1: json-row-type", verify: "go build ./...", hasVerify: true,
		},
		{
			number: 2, slug: "json-flag", summary: "add the --json bool flag and wire list.go",
			typ: planparser.CardTypeEdit, typeLabelCount: 1,
			targets: []string{"boardcli.newListCmd", "internal/boardcli/list.go"},
			uses:    []string{"internal/output/envelope.go"}, hasUses: true,
			intent:           "Add the `--json` bool flag to `newListCmd` and branch its row output between the table writer and the JSON path.",
			impactSummary:    "Adds a --json flag to the list command and branches its row-emission path on it.",
			hasImpactSummary: true,
		},
		{
			number: 3, slug: "json-emission", summary: "marshal each row through output.Ok when --json is set",
			typ: planparser.CardTypeCustom, typeLabelCount: 1,
			targets: []string{"boardcli.emitJSON", "internal/output/emit.go"},
			uses:    []string{"internal/boardcli/list.go"}, hasUses: true,
			intent: "Introduce `emitJSON`, a new helper in a new file, marshaling each row through `output.Ok` when `--json` is set.",
		},
		{
			number: 4, slug: "legacy-rows-delete", summary: "remove the superseded legacy row-conversion file",
			typ: planparser.CardTypeDelete, typeLabelCount: 1,
			targets:          []string{"internal/boardengine/legacyrows.go"},
			intent:           "Remove the legacy per-row conversion helper now that `boardengine.MapRowJSON` (card 5) supersedes it.",
			impactSummary:    "Deletes the legacy row-conversion file; no remaining callers reference it.",
			hasImpactSummary: true,
		},
		{
			number: 5, slug: "rowmapper-rename", summary: "rename the row mapper ahead of a later extraction",
			typ: planparser.CardTypeRename, typeLabelCount: 1,
			targets: []string{"boardengine.MapRow", "boardengine.MapRowJSON", "internal/boardengine/rows.go", "internal/boardengine/rowsjson.go"},
			pairs: []planparser.MovePair{
				{Old: "boardengine.MapRow", New: "boardengine.MapRowJSON"},
				{Old: "internal/boardengine/rows.go", New: "internal/boardengine/rowsjson.go"},
			},
			intent: "Rename the row mapper and its file to make the JSON-oriented behavior explicit ahead of a later extraction.",
		},
		{
			number: 6, slug: "helppins-move", summary: "relocate the pinned help-tree fixture",
			typ: planparser.CardTypeMove, typeLabelCount: 1,
			targets: []string{"cmd/lyx/helppins.go"},
			intent:  "Relocate the pinned help-tree fixture to `//cmd/lyx/helptree/helppins.go` ahead of the CLI help-tree split, with no behavior change in this card.",
		},
		{
			number: 7, slug: "json-docs", summary: "update the package doc comment and the standalone docs page",
			typ: planparser.CardTypeProsa, typeLabelCount: 1,
			targets: []string{"internal/boardcli/doc.go", "docs/boardcli-json.md"},
			intent:  "Update the package doc comment and the standalone docs page describing `--json` output.",
		},
	}

	for i, w := range wants {
		c := plan.Cards[i]
		if c.Number != w.number || c.Slug != w.slug || c.Title != w.slug {
			t.Errorf("card %d Number/Slug/Title = %d/%q/%q; want %d/%q/%q", w.number, c.Number, c.Slug, c.Title, w.number, w.slug, w.slug)
		}
		if c.Summary != w.summary {
			t.Errorf("card %d Summary = %q; want %q", w.number, c.Summary, w.summary)
		}
		if c.Type != w.typ {
			t.Errorf("card %d Type = %q; want %q", w.number, c.Type, w.typ)
		}
		if c.TypeLabelCount != w.typeLabelCount {
			t.Errorf("card %d TypeLabelCount = %d; want %d", w.number, c.TypeLabelCount, w.typeLabelCount)
		}
		if !slices.Equal(c.Targets, w.targets) {
			t.Errorf("card %d Targets = %v; want %v", w.number, c.Targets, w.targets)
		}
		if !slices.Equal(c.Pairs, w.pairs) {
			t.Errorf("card %d Pairs = %+v; want %+v", w.number, c.Pairs, w.pairs)
		}
		if w.hasUses {
			if !c.HasUses {
				t.Errorf("card %d HasUses = false; want true", w.number)
			}
			if !slices.Equal(c.Uses, w.uses) {
				t.Errorf("card %d Uses = %v; want %v", w.number, c.Uses, w.uses)
			}
		} else {
			if c.HasUses {
				t.Errorf("card %d HasUses = true; want false", w.number)
			}
			if c.Uses != nil {
				t.Errorf("card %d Uses = %v; want nil", w.number, c.Uses)
			}
		}
		if c.Intent != w.intent {
			t.Errorf("card %d Intent = %q; want %q", w.number, c.Intent, w.intent)
		}
		if w.hasImpactSummary {
			if !c.HasImpactSummary {
				t.Errorf("card %d HasImpactSummary = false; want true", w.number)
			}
			if c.ImpactSummary != w.impactSummary {
				t.Errorf("card %d ImpactSummary = %q; want %q", w.number, c.ImpactSummary, w.impactSummary)
			}
		} else if c.HasImpactSummary {
			t.Errorf("card %d HasImpactSummary = true; want false", w.number)
		}
		if c.Commit != w.commit {
			t.Errorf("card %d Commit = %q; want %q", w.number, c.Commit, w.commit)
		}
		if c.HasVerify != w.hasVerify {
			t.Errorf("card %d HasVerify = %v; want %v", w.number, c.HasVerify, w.hasVerify)
		}
		if c.Verify != w.verify {
			t.Errorf("card %d Verify = %q; want %q", w.number, c.Verify, w.verify)
		}
	}

	// The sharpest regression this migration can introduce: a symbol target must pass through
	// normalization unmodified even though the fixture's root: is non-empty.
	if got := plan.Cards[0].Targets[0]; got != "boardcli.RowJSON" {
		t.Errorf("card 1 symbol target = %q; want %q unmodified despite non-empty root:", got, "boardcli.RowJSON")
	}
}
