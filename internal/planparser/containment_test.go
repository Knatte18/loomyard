// containment_test.go covers syntacticContainment: a member glyph and a unit self glyph naming
// the same unit on two cards producing exactly one finding; the same two refs on one card
// producing none, since a card cannot conflict with itself; two member glyphs in one unit
// producing none, which is the whole point of symbol granularity; and language: none producing
// none.

package planparser

import (
	"testing"

	"github.com/Knatte18/quarry/glyph"
)

// TestSyntacticContainment asserts which Targets pairings across cards overlap on one unit:
// a member glyph or a file self glyph against the self glyph of its own direct directory on
// another card produces exactly one containment-unit-overlap finding, attributed to the
// finer-grained card; the same refs on one card (a card cannot conflict with itself), two member
// glyphs in one unit (the whole point of symbol granularity), two files in one directory and a file
// against an unrelated or ancestor directory produce none.
// The file-vs-directory pairing is crucible round fable-high-r10, F4: it involves no member glyph,
// so neither the member pairing nor planglyph's resolve-backed member-vs-file tier ever compared
// them, and a whole-unit target and one of its own files dispatched blind in parallel.
func TestSyntacticContainment(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		cards []Card
		// wantCard is the card a single expected finding is attributed to; empty for none.
		wantCard string
	}{
		{
			name: "member glyph and self glyph of the same unit on two cards",
			cards: []Card{
				{Number: 1, Slug: "member", Targets: []string{"internal/foo#Bar"}},
				{Number: 2, Slug: "self", Targets: []string{"internal/foo#"}},
			},
			wantCard: "1-member",
		},
		{
			name:  "member glyph and self glyph on one card",
			cards: []Card{{Number: 1, Slug: "both", Targets: []string{"internal/foo#Bar", "internal/foo#"}}},
		},
		{
			name: "two member glyphs in one unit",
			cards: []Card{
				{Number: 1, Slug: "one", Targets: []string{"internal/foo#Bar"}},
				{Number: 2, Slug: "two", Targets: []string{"internal/foo#Baz"}},
			},
		},
		{
			name: "file self glyph and its directory's self glyph on two cards",
			cards: []Card{
				{Number: 1, Slug: "file", Targets: []string{"internal/foo/bar.go#"}},
				{Number: 2, Slug: "unit", Targets: []string{"internal/foo#"}},
			},
			wantCard: "1-file",
		},
		{
			name:  "file self glyph and its directory's self glyph on one card",
			cards: []Card{{Number: 1, Slug: "both", Targets: []string{"internal/foo/bar.go#", "internal/foo#"}}},
		},
		{
			name: "file self glyph against an unrelated and an ancestor directory",
			cards: []Card{
				{Number: 1, Slug: "file", Targets: []string{"internal/foo/bar.go#"}},
				{Number: 2, Slug: "unit", Targets: []string{"internal/other#"}},
				{Number: 3, Slug: "grandparent", Targets: []string{"internal#"}},
			},
		},
		{
			name: "two file self glyphs in one directory",
			cards: []Card{
				{Number: 1, Slug: "one", Targets: []string{"internal/foo/a.go#"}},
				{Number: 2, Slug: "two", Targets: []string{"internal/foo/b.go#"}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			findings := syntacticContainment(&Plan{Cards: tt.cards}, glyph.Go)
			if tt.wantCard == "" {
				if len(findings) != 0 {
					t.Fatalf("syntacticContainment() = %+v; want none", findings)
				}
				return
			}
			if len(findings) != 1 {
				t.Fatalf("len(syntacticContainment()) = %d; want 1", len(findings))
			}
			if findings[0].Check != "containment-unit-overlap" {
				t.Errorf("findings[0].Check = %q; want %q", findings[0].Check, "containment-unit-overlap")
			}
			if findings[0].Card != tt.wantCard {
				t.Errorf("findings[0].Card = %q; want %q", findings[0].Card, tt.wantCard)
			}
		})
	}
}

// TestValidate_ContainmentUnitOverlap_LanguageNone proves language: none skips the check entirely
// via Validate's own dispatch, since syntacticContainment is never called for a not-ok
// planLanguage.
func TestValidate_ContainmentUnitOverlap_LanguageNone(t *testing.T) {
	t.Parallel()

	plan := &Plan{
		Format: RecognizedFormat, Approved: true, Language: "none",
		Cards: []Card{
			{Number: 1, Slug: "member", Type: CardTypeEdit, TypeLabelCount: 1,
				TargetGroups: []TargetGroup{{Type: CardTypeEdit, Refs: []string{"internal/foo#Bar"}}},
				Targets:      []string{"internal/foo#Bar"}, HasIntent: true, Intent: "x", HasUses: true, Uses: []string{},
				HasImpactSummary: true, ImpactSummary: "x"},
			{Number: 2, Slug: "self", Type: CardTypeEdit, TypeLabelCount: 1,
				TargetGroups: []TargetGroup{{Type: CardTypeEdit, Refs: []string{"internal/foo#"}}},
				Targets:      []string{"internal/foo#"}, HasIntent: true, Intent: "x", HasUses: true, Uses: []string{},
				HasImpactSummary: true, ImpactSummary: "x"},
		},
	}
	findings := Validate(plan, t.TempDir())
	for _, f := range findings {
		if f.Check == "containment-unit-overlap" {
			t.Errorf("Validate() unexpectedly emits containment-unit-overlap under language: none: %+v", f)
		}
	}
}
