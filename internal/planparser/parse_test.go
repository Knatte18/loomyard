// parse_test.go covers ParsePlan's overview-parsing behavior (frontmatter decoding, Card Index
// parsing, framing extraction), its per-card file-parsing behavior (the title heading, the
// format-4 one-or-more type-label model and its per-label TargetGroups, Uses:/Intent:/
// ImpactSummary:, retired-label routing, and Commit:/Verify:), the label-present-vs-absent field
// distinction, and a full round-trip over the format-4 golden fixture (testdata/goodplan).

package planparser_test

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
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
format: 5
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

// TestParsePlan_Overview covers the overview's frontmatter, framing and Card Index parsing: a complete overview, ASCII single and double hyphen Card Index separators, and a missing format:/approved: key -- not a ParsePlan failure, since format-unrecognized and plan-unapproved are Validate's checks, not the parser's, so the plan simply parses with the zero value.
//
//testtiming:keep pins the overview's frontmatter, framing and Card Index fields, which TestParsePlan_CardFields does not assert
func TestParsePlan_Overview(t *testing.T) {
	t.Parallel()

	type wantCard struct {
		number  int
		slug    string
		summary string
	}
	tests := []struct {
		name         string
		overview     string
		cards        map[string]string
		wantFormat   int
		wantApproved bool
		wantFraming  string
		wantCards    []wantCard
	}{
		{
			name:         "complete overview",
			overview:     minimalOverview,
			cards:        map[string]string{"01-only.md": minimalCardFile(1, "only", "a.go")},
			wantFormat:   5,
			wantApproved: true,
			wantFraming:  "Framing paragraph.",
			wantCards:    []wantCard{{1, "only", "the only card"}},
		},
		{
			name: "ASCII dash separators",
			overview: "---\nformat: 5\napproved: true\n---\n\n# Plan: ascii dash variant\n\nFraming paragraph.\n\n## Card Index\n\n" +
				"1 - single-dash - intent using a single ASCII hyphen\n" +
				"2 -- double-dash -- intent using a double ASCII hyphen\n",
			cards: map[string]string{
				"01-single-dash.md": minimalCardFile(1, "single-dash", "a.go"),
				"02-double-dash.md": minimalCardFile(2, "double-dash", "b.go"),
			},
			wantFormat:   5,
			wantApproved: true,
			wantFraming:  "Framing paragraph.",
			wantCards: []wantCard{
				{1, "single-dash", "intent using a single ASCII hyphen"},
				{2, "double-dash", "intent using a double ASCII hyphen"},
			},
		},
		{
			name:        "absent format and approved parse as zero values",
			overview:    "---\n{}\n---\n\n# Plan\n\nFraming.\n\n## Card Index\n\n1 — only — the only card\n",
			cards:       map[string]string{"01-only.md": minimalCardFile(1, "only", "a.go")},
			wantFraming: "Framing.",
			wantCards:   []wantCard{{1, "only", "the only card"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			files := map[string]string{"00-overview.md": tt.overview}
			for name, content := range tt.cards {
				files[name] = content
			}
			dir := writePlanFiles(t, files)

			plan, err := planparser.ParsePlan(dir)
			if err != nil {
				t.Fatalf("ParsePlan(%q) error = %v; want nil", dir, err)
			}

			if plan.Dir != dir {
				t.Errorf("plan.Dir = %q; want %q", plan.Dir, dir)
			}
			if plan.Format != tt.wantFormat {
				t.Errorf("plan.Format = %d; want %d", plan.Format, tt.wantFormat)
			}
			if plan.Approved != tt.wantApproved {
				t.Errorf("plan.Approved = %v; want %v", plan.Approved, tt.wantApproved)
			}
			if plan.Root != "" {
				t.Errorf("plan.Root = %q; want empty (no root: key)", plan.Root)
			}
			if plan.Framing != tt.wantFraming {
				t.Errorf("plan.Framing = %q; want %q", plan.Framing, tt.wantFraming)
			}
			if len(plan.Cards) != len(tt.wantCards) {
				t.Fatalf("len(plan.Cards) = %d; want %d", len(plan.Cards), len(tt.wantCards))
			}
			for i, want := range tt.wantCards {
				got := plan.Cards[i]
				if got.Number != want.number || got.Slug != want.slug || got.Summary != want.summary {
					t.Errorf("plan.Cards[%d] Number/Slug/Summary = %d/%q/%q; want %d/%q/%q", i, got.Number, got.Slug, got.Summary, want.number, want.slug, want.summary)
				}
			}
		})
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
			content:    "---\nformat: 5\napproved: true\nextra: true\n---\n\n# Plan\n\nFraming.\n\n## Card Index\n\n1 — a — b\n",
			wantSubstr: "field extra not found",
		},
		{
			name:       "duplicate frontmatter key",
			content:    "---\nformat: 5\nformat: 5\napproved: true\n---\n\n# Plan\n\nFraming.\n\n## Card Index\n\n1 — a — b\n",
			wantSubstr: "already defined",
		},
		{
			name:       "unterminated frontmatter fence",
			content:    "---\nformat: 5\napproved: true\n\n# Plan\n\nFraming.\n\n## Card Index\n\n1 — a — b\n",
			wantSubstr: "unterminated frontmatter fence",
		},
		{
			name:       "missing card index heading",
			content:    "---\nformat: 5\napproved: true\n---\n\n# Plan\n\nFraming.\n",
			wantSubstr: `missing "## Card Index" heading`,
		},
		{
			name:       "card file absent",
			content:    minimalOverview,
			wantSubstr: "card file not found",
		},
		{
			name:       "unparseable card index line",
			content:    "---\nformat: 5\napproved: true\n---\n\n# Plan\n\nFraming.\n\n## Card Index\n\nnot a valid entry\n",
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

// mapReader returns a ParsePlanFrom reader over an in-memory file map, wrapping fs.ErrNotExist for absent names.
func mapReader(files map[string]string) func(string) ([]byte, error) {
	return func(name string) ([]byte, error) {
		content, ok := files[name]
		if !ok {
			return nil, fmt.Errorf("read %s: %w", name, fs.ErrNotExist)
		}
		return []byte(content), nil
	}
}

//testtiming:keep pins ParsePlanFrom's equality with ParsePlan and its card-file-not-found error, which its covering tests do not assert
func TestParsePlanFrom(t *testing.T) {
	t.Parallel()

	t.Run("matches ParsePlan over the same files", func(t *testing.T) {
		t.Parallel()

		files := map[string]string{
			"00-overview.md": minimalOverview,
			"01-only.md":     minimalCardFile(1, "only", "a.go"),
		}
		dir := writePlanFiles(t, files)

		want, err := planparser.ParsePlan(dir)
		if err != nil {
			t.Fatalf("ParsePlan(%q) error = %v; want nil", dir, err)
		}
		got, err := planparser.ParsePlanFrom(dir, mapReader(files))
		if err != nil {
			t.Fatalf("ParsePlanFrom() error = %v; want nil", err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("ParsePlanFrom() = %+v; want %+v", got, want)
		}
	})

	t.Run("absent card file", func(t *testing.T) {
		t.Parallel()

		_, err := planparser.ParsePlanFrom("plan", mapReader(map[string]string{"00-overview.md": minimalOverview}))
		if err == nil {
			t.Fatal("ParsePlanFrom() error = nil; want card-file-not-found error")
		}
		if !strings.Contains(err.Error(), "card file not found") {
			t.Errorf("ParsePlanFrom() error = %q; want card-file-not-found substring", err.Error())
		}
	})
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

// TestParsePlan_CardFields covers how one card file's body parses into its Card, each row pinning one field family:
// the one-or-more type-label model and its per-label TargetGroups (two recognized labels on a card, even the same label twice, is the supported shape; zero labels stays a defect card-type-missing catches); a "**Uses:**" label present with no bullets (a non-nil zero-length slice, distinct from an absent label); the ImpactSummary inline remainder plus trailing lines; the retired labels routed to RetiredLabels while terminating the preceding Intent prose; the Rename and Create grammars, where a malformed bullet reaches RenameRaw or CreateRaw rather than becoming a parse error and the arrow form reaches its matcher with its backticks intact; a handle-shaped target never picking up a root: prefix; and the optional Commit and Verify fields.
func TestParsePlan_CardFields(t *testing.T) {
	t.Parallel()

	const rootedOverview = `---
format: 5
approved: true
root: internal/boardcli
---

# Plan: rooted

Framing paragraph.

## Card Index

1 — only — the only card
`
	tests := []struct {
		name string
		// overview defaults to minimalOverview.
		overview string
		body     string
		check    func(t *testing.T, card planparser.Card)
	}{
		{
			name: "two type labels",
			body: "# Card 1 — dual\n\n**Edit:**\n- `a.go`\n**Delete:**\n- `b.go`\n**Intent:** placeholder.\n",
			check: func(t *testing.T, card planparser.Card) {
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
				// Canonicalized to their file self glyph form: both carry a file extension, so
				// ParsePlan's canonicalizeCard rewrites them under the default language: go.
				if want := []string{"a.go#"}; !slices.Equal(card.TargetGroups[0].Refs, want) {
					t.Errorf("card.TargetGroups[0].Refs = %v; want %v", card.TargetGroups[0].Refs, want)
				}
				if want := []string{"b.go#"}; !slices.Equal(card.TargetGroups[1].Refs, want) {
					t.Errorf("card.TargetGroups[1].Refs = %v; want %v", card.TargetGroups[1].Refs, want)
				}
				if want := []string{"a.go#", "b.go#"}; !slices.Equal(card.Targets, want) {
					t.Errorf("card.Targets = %v; want %v (concatenation of both groups' Refs, body order)", card.Targets, want)
				}
			},
		},
		{
			name: "no type label",
			body: "# Card 1 — typeless\n\n**Intent:** placeholder.\n",
			check: func(t *testing.T, card planparser.Card) {
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
			},
		},
		{
			name: "single-label card produces exactly one group",
			body: minimalCardFile(1, "only", "a.go"),
			check: func(t *testing.T, card planparser.Card) {
				if len(card.TargetGroups) != 1 {
					t.Fatalf("len(card.TargetGroups) = %d; want 1", len(card.TargetGroups))
				}
				if card.TargetGroups[0].Type != planparser.CardTypeEdit {
					t.Errorf("card.TargetGroups[0].Type = %q; want %q", card.TargetGroups[0].Type, planparser.CardTypeEdit)
				}
			},
		},
		{
			name: "repeated label produces two groups whose union equals one merged group's refs",
			body: "# Card 1 — repeated label\n\n**Edit:**\n- `a.go`\n**Edit:**\n- `b.go`\n**Intent:** placeholder.\n",
			check: func(t *testing.T, card planparser.Card) {
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
				if want := []string{"a.go#", "b.go#"}; !slices.Equal(union, want) {
					t.Errorf("union of both groups' Refs = %v; want %v (equal to one merged group's refs)", union, want)
				}
			},
		},
		{
			name: "two Rename labels give each group its own Pairs",
			body: "# Card 1 — two renames\n\n" +
				"**Rename:**\n- `old1.Symbol` -> `new1.Symbol`\n" +
				"**Rename:**\n- `old2.Symbol` -> `new2.Symbol`\n" +
				"**Intent:** placeholder.\n",
			check: func(t *testing.T, card planparser.Card) {
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
			},
		},
		{
			name: "Uses present with no bullets is a non-nil empty slice",
			body: "# Card 1 — empty uses\n\n**Edit:**\n- `a.go`\n**Uses:**\n**Intent:** placeholder.\n",
			check: func(t *testing.T, card planparser.Card) {
				if !card.HasUses {
					t.Errorf("card.HasUses = false; want true")
				}
				if card.Uses == nil {
					t.Errorf("card.Uses = nil; want a non-nil, zero-length slice")
				}
				if len(card.Uses) != 0 {
					t.Errorf("card.Uses = %v; want empty", card.Uses)
				}
			},
		},
		{
			// The label line's own remainder lands in ImpactSummary and every following non-label line in ImpactSummaryTrailing -- captured rather than discarded so impact-summary-multiline has something to report.
			name: "ImpactSummary inline remainder plus trailing lines",
			body: "# Card 1 — multiline impact\n\n**Edit:**\n- `a.go`\n" +
				"**ImpactSummary:** first line.\nsecond line.\nthird line.\n**Intent:** placeholder.\n",
			check: func(t *testing.T, card planparser.Card) {
				if card.ImpactSummary != "first line." {
					t.Errorf("card.ImpactSummary = %q; want %q", card.ImpactSummary, "first line.")
				}
				if want := []string{"second line.", "third line."}; !slices.Equal(card.ImpactSummaryTrailing, want) {
					t.Errorf("card.ImpactSummaryTrailing = %v; want %v", card.ImpactSummaryTrailing, want)
				}
			},
		},
		{
			name: "retired Context label terminates Intent",
			body: "# Card 1 — half-migrated\n\n**Intent:** prose before context.\n**Context:**\n- `a.go`\n",
			check: func(t *testing.T, card planparser.Card) {
				if card.Intent != "prose before context." {
					t.Errorf("card.Intent = %q; want %q (Context: must terminate collection, not be swallowed)", card.Intent, "prose before context.")
				}
				if want := []string{"**Context:**"}; !slices.Equal(card.RetiredLabels, want) {
					t.Errorf("card.RetiredLabels = %v; want %v", card.RetiredLabels, want)
				}
			},
		},
		{
			// The case-sensitive match routes format-3's lowercase label to RetiredLabels rather than mistaking it for format-4's "**Verify:**" field.
			name: "retired lowercase verify label terminates Intent",
			body: "# Card 1 — half-migrated\n\n**Intent:** prose before verify.\n**verify:** go test ./...\n",
			check: func(t *testing.T, card planparser.Card) {
				if card.Intent != "prose before verify." {
					t.Errorf("card.Intent = %q; want %q (verify: must terminate collection, not be swallowed)", card.Intent, "prose before verify.")
				}
				if want := []string{"**verify:**"}; !slices.Equal(card.RetiredLabels, want) {
					t.Errorf("card.RetiredLabels = %v; want %v", card.RetiredLabels, want)
				}
				if card.HasVerify {
					t.Errorf("card.HasVerify = true; want false (lowercase verify: is not the format-4 Verify: field)")
				}
				if card.Verify != "" {
					t.Errorf("card.Verify = %q; want empty", card.Verify)
				}
			},
		},
		{
			// Both endpoints of every pair are projected into Targets in pair order, Old before New.
			name: "Rename grammar: well-formed pair and malformed bullet",
			body: "# Card 1 — rename\n\n**Rename:**\n- `old.Symbol` -> `new.Symbol`\n- this bullet has no arrow at all\n" +
				"**Intent:** placeholder.\n",
			check: func(t *testing.T, card planparser.Card) {
				if want := []planparser.MovePair{{Old: "old.Symbol", New: "new.Symbol"}}; !slices.Equal(card.Pairs, want) {
					t.Errorf("card.Pairs = %+v; want %+v", card.Pairs, want)
				}
				if want := []string{"this bullet has no arrow at all"}; !slices.Equal(card.RenameRaw, want) {
					t.Errorf("card.RenameRaw = %v; want %v", card.RenameRaw, want)
				}
				if want := []string{"old.Symbol", "new.Symbol"}; !slices.Equal(card.Targets, want) {
					t.Errorf("card.Targets = %v; want %v (Old before New, pair order)", card.Targets, want)
				}
			},
		},
		{
			// parseRefField strips backticks before returning a payload, so routing the arrow form through it would never match the two-backticked-token shape: a declaration in Declarations, not CreateRaw, proves the backticks arrived intact.
			name: "Create grammar: declaration, plain ref and malformed arrow bullet",
			body: "# Card 1 — create handles\n\n**Create:**\n" +
				"- `plan:internal/foo#NewThing` -> `func NewThing() *Thing`\n" +
				"- `internal/bar.go`\n" +
				"- this bullet has -> an arrow but no backticks\n" +
				"**Intent:** placeholder.\n",
			check: func(t *testing.T, card planparser.Card) {
				wantDecls := []planparser.CardDeclaration{{Handle: "plan:internal/foo#NewThing", Decl: "func NewThing() *Thing"}}
				if !slices.Equal(card.Declarations, wantDecls) {
					t.Errorf("card.Declarations = %+v; want %+v", card.Declarations, wantDecls)
				}
				if want := []string{"plan:internal/foo#NewThing", "internal/bar.go#"}; !slices.Equal(card.Targets, want) {
					t.Errorf("card.Targets = %v; want %v", card.Targets, want)
				}
				if want := []string{"this bullet has -> an arrow but no backticks"}; !slices.Equal(card.CreateRaw, want) {
					t.Errorf("card.CreateRaw = %v; want %v", card.CreateRaw, want)
				}
				if len(card.TargetGroups) != 1 {
					t.Fatalf("len(card.TargetGroups) = %d; want 1", len(card.TargetGroups))
				}
				if !slices.Equal(card.TargetGroups[0].Declarations, wantDecls) {
					t.Errorf("card.TargetGroups[0].Declarations = %+v; want %+v", card.TargetGroups[0].Declarations, wantDecls)
				}
			},
		},
		{
			name:     "handle-shaped Create target never picks up a root: prefix",
			overview: rootedOverview,
			body: "# Card 1 — create handle\n\n**Create:**\n" +
				"- `plan:internal/foo#NewThing` -> `func NewThing() *Thing`\n" +
				"**Intent:** placeholder.\n",
			check: func(t *testing.T, card planparser.Card) {
				if want := "plan:internal/foo#NewThing"; card.Targets[0] != want {
					t.Errorf("card.Targets[0] = %q; want %q (a handle must never pick up a root: prefix)", card.Targets[0], want)
				}
			},
		},
		{
			name: "Commit and Verify fields",
			body: "# Card 1 — flag\n\n**Edit:**\n- `a.go`\n**Intent:** placeholder.\n" +
				"**Commit:** `1: add the --json flag`\n**Verify:** go build ./...\n",
			check: func(t *testing.T, card planparser.Card) {
				if card.Commit != "1: add the --json flag" {
					t.Errorf("card.Commit = %q; want %q", card.Commit, "1: add the --json flag")
				}
				if !card.HasVerify {
					t.Errorf("card.HasVerify = false; want true")
				}
				if card.Verify != "go build ./..." {
					t.Errorf("card.Verify = %q; want %q", card.Verify, "go build ./...")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			overview := tt.overview
			if overview == "" {
				overview = minimalOverview
			}
			dir := writePlanFiles(t, map[string]string{"00-overview.md": overview, "01-only.md": tt.body})
			plan, err := planparser.ParsePlan(dir)
			if err != nil {
				t.Fatalf("ParsePlan() error = %v; want nil", err)
			}
			tt.check(t, plan.Cards[0])
		})
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

// TestParsePlan_Card_SourcePath proves each parsed card's SourcePath is the bare worktree-relative `_lyx/plan/NN-<slug>.md` token, never prefixed by the (t.TempDir()) absolute Plan.Dir the fixture is parsed from.
//
//testtiming:keep pins SourcePath as the bare worktree-relative token without the absolute plan directory, which TestParsePlan_CardFields does not assert
func TestParsePlan_Card_SourcePath(t *testing.T) {
	t.Parallel()

	const overview = `---
format: 5
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
		if strings.Contains(c.SourcePath, dir) || strings.Contains(c.SourcePath, os.TempDir()) {
			t.Errorf("card %d SourcePath = %q; leaks the absolute Plan.Dir %q", c.Number, c.SourcePath, dir)
		}
	}
}

// goodPlanDir is this package's format-5 golden happy-path fixture, exercising all seven card
// types.
func goodPlanDir() string {
	return filepath.Join("testdata", "goodplan")
}

// TestParsePlan_GoldenFixture round-trips testdata/goodplan exactly: the overview's frontmatter, framing, and every field of every one of the seven cards must match the fixture's own canonicalized content, including the root: internal/boardcli resolution (for a plain path) and the // worktree-root escape, both followed by glyph canonicalization under the fixture's language: go.
// It also pins this migration's sharpest possible regression: a glyph copied verbatim from the fixture must survive canonicalization byte-identical even though the fixture's root: is non-empty — a glyph is never root:-joined.
// Card 2 additionally round-trips a multi-label card: an **Edit:** group followed by a **Create:** group.
//
//testtiming:keep pins the full field-by-field round-trip of the golden fixture, which TestValidate_CardNumbering does not assert
func TestParsePlan_GoldenFixture(t *testing.T) {
	t.Parallel()

	plan, err := planparser.ParsePlan(goodPlanDir())
	if err != nil {
		t.Fatalf("ParsePlan(%q) error = %v; want nil", goodPlanDir(), err)
	}

	if plan.Format != 5 {
		t.Errorf("plan.Format = %d; want 5", plan.Format)
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

	// wantGroup pins one expected TargetGroup: its own Type and its own Refs.
	type wantGroup struct {
		typ  planparser.CardType
		refs []string
	}

	type wantCard struct {
		number           int
		slug             string
		summary          string
		typ              planparser.CardType
		typeLabelCount   int
		targets          []string
		groups           []wantGroup
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
			targets: []string{"internal/boardcli#RowJSON"},
			groups: []wantGroup{
				{typ: planparser.CardTypeCreate, refs: []string{"internal/boardcli#RowJSON"}},
			},
			intent: "Define the `RowJSON` struct carrying the list command's existing table columns as JSON-taggable fields.",
			commit: "1: json-row-type", verify: "go build ./...", hasVerify: true,
		},
		{
			number: 2, slug: "json-flag", summary: "add the --json bool flag and wire list.go",
			typ: planparser.CardTypeEdit, typeLabelCount: 2,
			targets: []string{"internal/boardcli#newListCmd", "internal/boardcli/list_json_test.go#"},
			groups: []wantGroup{
				{typ: planparser.CardTypeEdit, refs: []string{"internal/boardcli#newListCmd"}},
				{typ: planparser.CardTypeCreate, refs: []string{"internal/boardcli/list_json_test.go#"}},
			},
			uses: []string{"internal/output/envelope.go#"}, hasUses: true,
			intent:           "Add the `--json` bool flag to `newListCmd` and branch its row output between the table writer and the JSON path.",
			impactSummary:    "Adds a --json flag to the list command and branches its row-emission path on it.",
			hasImpactSummary: true,
		},
		{
			number: 3, slug: "json-emission", summary: "marshal each row through output.Ok when --json is set",
			typ: planparser.CardTypeCustom, typeLabelCount: 1,
			targets: []string{"internal/output#emitJSON", "internal/output/emit.go#"},
			groups: []wantGroup{
				{typ: planparser.CardTypeCustom, refs: []string{"internal/output#emitJSON", "internal/output/emit.go#"}},
			},
			uses: []string{"internal/boardcli/list.go#"}, hasUses: true,
			intent: "Introduce `emitJSON`, a new helper in a new file, marshaling each row through `output.Ok` when `--json` is set.",
		},
		{
			number: 4, slug: "legacy-rows-delete", summary: "remove the superseded legacy row-conversion file",
			typ: planparser.CardTypeDelete, typeLabelCount: 1,
			targets: []string{"internal/boardengine/legacyrows.go#"},
			groups: []wantGroup{
				{typ: planparser.CardTypeDelete, refs: []string{"internal/boardengine/legacyrows.go#"}},
			},
			intent:           "Remove the legacy per-row conversion helper now that `boardengine.MapRowJSON` (card 5) supersedes it.",
			impactSummary:    "Deletes the legacy row-conversion file; no remaining callers reference it.",
			hasImpactSummary: true,
		},
		{
			number: 5, slug: "rowmapper-rename", summary: "rename the row mapper ahead of a later extraction",
			typ: planparser.CardTypeRename, typeLabelCount: 1,
			targets: []string{"internal/boardengine#MapRow", "plan:internal/boardengine#MapRowJSON", "internal/boardengine/rows.go#", "internal/boardengine/rowsjson.go#"},
			groups: []wantGroup{
				{typ: planparser.CardTypeRename, refs: []string{"internal/boardengine#MapRow", "plan:internal/boardengine#MapRowJSON", "internal/boardengine/rows.go#", "internal/boardengine/rowsjson.go#"}},
			},
			pairs: []planparser.MovePair{
				{Old: "internal/boardengine#MapRow", New: "plan:internal/boardengine#MapRowJSON"},
				{Old: "internal/boardengine/rows.go#", New: "internal/boardengine/rowsjson.go#"},
			},
			intent: "Rename the row mapper and its file to make the JSON-oriented behavior explicit ahead of a later extraction.",
		},
		{
			number: 6, slug: "helppins-move", summary: "relocate the pinned help-tree fixture",
			typ: planparser.CardTypeMove, typeLabelCount: 1,
			targets: []string{"cmd/lyx/helppins.go#"},
			groups: []wantGroup{
				{typ: planparser.CardTypeMove, refs: []string{"cmd/lyx/helppins.go#"}},
			},
			intent: "Relocate the pinned help-tree fixture to `//cmd/lyx/helptree/helppins.go` ahead of the CLI help-tree split, with no behavior change in this card.",
		},
		{
			number: 7, slug: "json-docs", summary: "update the package doc comment and the standalone docs page",
			typ: planparser.CardTypeProsa, typeLabelCount: 1,
			targets: []string{"internal/boardcli/doc.go#", "docs/boardcli-json.md#"},
			groups: []wantGroup{
				{typ: planparser.CardTypeProsa, refs: []string{"internal/boardcli/doc.go#", "docs/boardcli-json.md#"}},
			},
			intent: "Update the package doc comment and the standalone docs page describing `--json` output.",
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
		if len(c.TargetGroups) != len(w.groups) {
			t.Fatalf("card %d len(TargetGroups) = %d; want %d", w.number, len(c.TargetGroups), len(w.groups))
		}
		for gi, wg := range w.groups {
			g := c.TargetGroups[gi]
			if g.Type != wg.typ {
				t.Errorf("card %d TargetGroups[%d].Type = %q; want %q", w.number, gi, g.Type, wg.typ)
			}
			if !slices.Equal(g.Refs, wg.refs) {
				t.Errorf("card %d TargetGroups[%d].Refs = %v; want %v", w.number, gi, g.Refs, wg.refs)
			}
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

	// The sharpest regression this migration can introduce: a glyph copied verbatim from the
	// fixture must pass through both normalization and canonicalization unmodified, even though
	// the fixture's root: is non-empty — a glyph is never root:-joined.
	if got := plan.Cards[0].Targets[0]; got != "internal/boardcli#RowJSON" {
		t.Errorf("card 1 glyph target = %q; want %q unmodified despite non-empty root:", got, "internal/boardcli#RowJSON")
	}

	// SurfaceRefs records the pre-canonicalization surface lexeme for every canonicalized
	// path-shaped ref, keyed by the owning card's own identity.
	wantSurface := map[string]string{
		"internal/boardcli/list_json_test.go#": "internal/boardcli/list_json_test.go",
		"internal/output/envelope.go#":         "internal/output/envelope.go",
	}
	gotSurface := plan.SurfaceRefs["2-json-flag"]
	for canonical, wantRaw := range wantSurface {
		if got := gotSurface[canonical]; len(got) != 1 || got[0] != wantRaw {
			t.Errorf("plan.SurfaceRefs[%q][%q] = %v; want exactly [%q]", "2-json-flag", canonical, got, wantRaw)
		}
	}
	// A glyph copied verbatim from the fixture is never rewritten, so it never gains a
	// SurfaceRefs entry of its own.
	if _, ok := plan.SurfaceRefs["1-json-row-type"]; ok {
		t.Errorf("plan.SurfaceRefs[%q] = %v; want no entry (card 1's own target is already a glyph)", "1-json-row-type", plan.SurfaceRefs["1-json-row-type"])
	}
}

// TestParsePlan_Language covers the language: frontmatter key's effect on Plan.Language and on canonicalization: absent defaults to "go" (and canonicalizes), an explicit "go" behaves identically, and "none" leaves every ref byte-identical with an empty SurfaceRefs.
//
//testtiming:keep pins the language: key's effect on Plan.Language, canonicalization and SurfaceRefs, which its covering tests do not assert
func TestParsePlan_Language(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		languageLine string
		wantLanguage string
		wantTarget   string
		// wantNoSurface requires SurfaceRefs to be empty, as under language none.
		wantNoSurface bool
	}{
		{name: "absent defaults to go and canonicalizes", languageLine: "", wantLanguage: "go", wantTarget: "a.go#"},
		{name: "explicit go behaves identically to absent", languageLine: "language: go\n", wantLanguage: "go", wantTarget: "a.go#"},
		{name: "none leaves every ref byte-identical", languageLine: "language: none\n", wantLanguage: "none", wantTarget: "a.go", wantNoSurface: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := writePlanFiles(t, map[string]string{
				"00-overview.md": "---\nformat: 5\napproved: true\n" + tt.languageLine + "---\n\n# Plan\n\nFraming.\n\n## Card Index\n\n1 — only — the only card\n",
				"01-only.md":     minimalCardFile(1, "only", "a.go"),
			})
			plan, err := planparser.ParsePlan(dir)
			if err != nil {
				t.Fatalf("ParsePlan() error = %v; want nil", err)
			}
			if plan.Language != tt.wantLanguage {
				t.Errorf("plan.Language = %q; want %q", plan.Language, tt.wantLanguage)
			}
			if plan.Cards[0].Targets[0] != tt.wantTarget {
				t.Errorf("plan.Cards[0].Targets[0] = %q; want %q", plan.Cards[0].Targets[0], tt.wantTarget)
			}
			if tt.wantNoSurface && len(plan.SurfaceRefs) != 0 {
				t.Errorf("len(plan.SurfaceRefs) = %d; want 0", len(plan.SurfaceRefs))
			}
		})
	}
}

// TestParsePlan_FirstCard pins how the optional first_card overview key parses, including that a non-integer value reaches validation instead of failing the strict decode.
func TestParsePlan_FirstCard(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		line        string
		wantFirst   int
		wantInvalid string
	}{
		{name: "absent", line: "", wantFirst: 1},
		{name: "seven", line: "first_card: 7\n", wantFirst: 7},
		{name: "zero", line: "first_card: 0\n", wantFirst: 1, wantInvalid: "0"},
		{name: "negative", line: "first_card: -2\n", wantFirst: 1, wantInvalid: "-2"},
		{name: "non-integer", line: "first_card: seven\n", wantFirst: 1, wantInvalid: "seven"},
		{name: "empty", line: "first_card:\n", wantFirst: 1, wantInvalid: "<empty>"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := writePlanFiles(t, map[string]string{
				"00-overview.md": "---\nformat: 5\napproved: true\n" + tt.line + "---\n\n# Plan: p\n\nFraming.\n\n## Card Index\n\n1 — only — the only card\n",
				"01-only.md":     "# Card 1 — only\n\n**Edit:**\n- `a.go`\n\n**Intent:** placeholder.\n",
			})
			plan, err := planparser.ParsePlan(dir)
			if err != nil {
				t.Fatalf("ParsePlan error = %v; want nil", err)
			}
			if plan.FirstCard != tt.wantFirst || plan.FirstCardInvalid != tt.wantInvalid {
				t.Errorf("FirstCard, FirstCardInvalid = %d, %q; want %d, %q", plan.FirstCard, plan.FirstCardInvalid, tt.wantFirst, tt.wantInvalid)
			}
		})
	}
}
