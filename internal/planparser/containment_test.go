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

func TestSyntacticContainment(t *testing.T) {
	t.Parallel()

	t.Run("a member glyph and a self glyph naming the same unit on two cards produces one finding", func(t *testing.T) {
		t.Parallel()
		plan := &Plan{
			Cards: []Card{
				{Number: 1, Slug: "member", Targets: []string{"internal/foo#Bar"}},
				{Number: 2, Slug: "self", Targets: []string{"internal/foo#"}},
			},
		}
		findings := syntacticContainment(plan, glyph.Go)
		if got := len(findings); got != 1 {
			t.Fatalf("len(syntacticContainment()) = %d; want 1", got)
		}
		if findings[0].Check != "containment-unit-overlap" {
			t.Errorf("findings[0].Check = %q; want %q", findings[0].Check, "containment-unit-overlap")
		}
	})

	t.Run("the same two refs on one card produces none", func(t *testing.T) {
		t.Parallel()
		plan := &Plan{
			Cards: []Card{
				{Number: 1, Slug: "both", Targets: []string{"internal/foo#Bar", "internal/foo#"}},
			},
		}
		findings := syntacticContainment(plan, glyph.Go)
		if got := len(findings); got != 0 {
			t.Errorf("len(syntacticContainment()) = %d; want 0 (a card cannot conflict with itself)", got)
		}
	})

	t.Run("two member glyphs in one unit produces none", func(t *testing.T) {
		t.Parallel()
		plan := &Plan{
			Cards: []Card{
				{Number: 1, Slug: "one", Targets: []string{"internal/foo#Bar"}},
				{Number: 2, Slug: "two", Targets: []string{"internal/foo#Baz"}},
			},
		}
		findings := syntacticContainment(plan, glyph.Go)
		if got := len(findings); got != 0 {
			t.Errorf("len(syntacticContainment()) = %d; want 0 (symbol granularity is not flagged)", got)
		}
	})

	// The self-vs-self cross-granularity pairing (crucible round fable-high-r10, F4): a file self
	// glyph and the self glyph of the directory that file sits directly in involve no member glyph,
	// so neither the member pairing above nor planglyph's resolve-backed member-vs-file tier ever
	// compared them.
	t.Run("a file self glyph and its directory's self glyph on two cards produces one finding", func(t *testing.T) {
		t.Parallel()
		plan := &Plan{
			Cards: []Card{
				{Number: 1, Slug: "file", Targets: []string{"internal/foo/bar.go#"}},
				{Number: 2, Slug: "unit", Targets: []string{"internal/foo#"}},
			},
		}
		findings := syntacticContainment(plan, glyph.Go)
		if got := len(findings); got != 1 {
			t.Fatalf("len(syntacticContainment()) = %d; want 1 — a whole-unit target and one of its own files dispatch blind in parallel otherwise", got)
		}
		if findings[0].Check != "containment-unit-overlap" {
			t.Errorf("findings[0].Check = %q; want %q", findings[0].Check, "containment-unit-overlap")
		}
		if findings[0].Card != "1-file" {
			t.Errorf("findings[0].Card = %q; want %q (attributed to the finer-grained file card)", findings[0].Card, "1-file")
		}
	})

	t.Run("a file self glyph and its directory's self glyph on ONE card produces none", func(t *testing.T) {
		t.Parallel()
		plan := &Plan{
			Cards: []Card{
				{Number: 1, Slug: "both", Targets: []string{"internal/foo/bar.go#", "internal/foo#"}},
			},
		}
		findings := syntacticContainment(plan, glyph.Go)
		if got := len(findings); got != 0 {
			t.Errorf("len(syntacticContainment()) = %d; want 0 (a card cannot conflict with itself)", got)
		}
	})

	t.Run("a file self glyph against an unrelated directory self glyph produces none", func(t *testing.T) {
		t.Parallel()
		plan := &Plan{
			Cards: []Card{
				{Number: 1, Slug: "file", Targets: []string{"internal/foo/bar.go#"}},
				{Number: 2, Slug: "unit", Targets: []string{"internal/other#"}},
				{Number: 3, Slug: "grandparent", Targets: []string{"internal#"}},
			},
		}
		findings := syntacticContainment(plan, glyph.Go)
		if got := len(findings); got != 0 {
			t.Errorf("len(syntacticContainment()) = %d; want 0 — only the file's own direct directory overlaps it", got)
		}
	})

	t.Run("two file self glyphs in one directory produce none", func(t *testing.T) {
		t.Parallel()
		plan := &Plan{
			Cards: []Card{
				{Number: 1, Slug: "one", Targets: []string{"internal/foo/a.go#"}},
				{Number: 2, Slug: "two", Targets: []string{"internal/foo/b.go#"}},
			},
		}
		findings := syntacticContainment(plan, glyph.Go)
		if got := len(findings); got != 0 {
			t.Errorf("len(syntacticContainment()) = %d; want 0 (distinct files never overlap)", got)
		}
	})
}

// TestValidate_ContainmentUnitOverlap_LanguageNone proves language: none skips the check entirely
// via Validate's own dispatch, since syntacticContainment is never called for a not-ok
// planLanguage.
func TestValidate_ContainmentUnitOverlap_LanguageNone(t *testing.T) {
	t.Parallel()

	plan := &Plan{
		Format: recognizedFormat, Approved: true, Language: "none",
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
