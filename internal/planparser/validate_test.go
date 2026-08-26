// validate_test.go covers all seventeen of Validate's format-4 checks, each with at least one
// triggering and one clean case, per manifest/designs/plan-card-format.md's seventeen distinct
// ValidationError.Check IDs — sixteen of which ValidateFormat also emits, everything but
// plan-unapproved.
// The golden happy-path test reuses the format-4 seven-card golden fixture (testdata/goodplan,
// already parsed by parse_test.go's TestParsePlan_GoldenFixture) and materializes exactly the
// seven distinct paths its checked entries name under a hermetic t.TempDir() worktree root,
// deliberately leaving absent the Custom card's own path-shaped target
// (internal/output/emit.go) and the Rename pair's post-rename side
// (internal/boardengine/rowsjson.go) — proving both exemptions positively rather than by omission
// — so the whole seventeen-check Validate run returns zero findings.

package planparser_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/planparser"
)

// validCard returns a fully well-formed format-4 Card at position number with slug — Type Edit,
// exactly one recognized type label, a single path-shaped Targets entry, a single TargetGroups
// entry mirroring that same Edit label and its own Refs, a present-but-empty Uses:, non-empty
// Intent:, a non-empty one-line ImpactSummary:, no retired labels, no malformed Rename bullets,
// and a correctly prefixed Commit:. Each subtest below starts from this baseline and mutates
// exactly the one field its own check cares about, so a finding it observes can only come from
// the check under test, never incidental noise from the other checks.
func validCard(number int, slug string) planparser.Card {
	targets := []string{fmt.Sprintf("pkg/card%d.go", number)}
	return planparser.Card{
		Number:           number,
		Slug:             slug,
		Title:            slug,
		Type:             planparser.CardTypeEdit,
		TypeLabelCount:   1,
		HasType:          true,
		Targets:          targets,
		TargetGroups:     []planparser.TargetGroup{{Type: planparser.CardTypeEdit, Refs: targets}},
		HasUses:          true,
		Uses:             []string{},
		HasIntent:        true,
		Intent:           "intent for " + slug,
		HasImpactSummary: true,
		ImpactSummary:    "impact for " + slug,
		Commit:           fmt.Sprintf("%d: %s", number, slug),
	}
}

// cardOfType returns a well-formed format-4 Card at position number with slug whose Type,
// Targets, and single-entry TargetGroups all agree on typ and refs — the only construction path
// this file's group-scoped subtests use for a fixture whose type differs from validCard's Edit
// baseline, since reassigning Type or Targets on an already-returned Card never reaches its
// separately-held TargetGroups entry.
func cardOfType(number int, slug string, typ planparser.CardType, refs []string) planparser.Card {
	card := validCard(number, slug)
	card.Type = typ
	card.Targets = refs
	card.TargetGroups = []planparser.TargetGroup{{Type: typ, Refs: refs}}
	return card
}

// countFor returns how many of findings carry the given Check name — every
// subtest below asserts on this count (and, for the golden fixture, the total)
// rather than exact Detail text, per the "assert on Check names and
// cardinality" instruction: Detail is for humans, Check is the stable contract.
func countFor(findings []planparser.ValidationError, check string) int {
	n := 0
	for _, f := range findings {
		if f.Check == check {
			n++
		}
	}
	return n
}

// materializeFiles writes an empty placeholder file at each of paths, joined
// under root, creating parent directories as needed — the on-disk half of
// path-missing's hermetic fixtures.
func materializeFiles(t *testing.T, root string, paths ...string) {
	t.Helper()
	for _, p := range paths {
		full := filepath.Join(root, p)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("materialize %s: mkdir: %v", p, err)
		}
		if err := os.WriteFile(full, []byte("placeholder\n"), 0o644); err != nil {
			t.Fatalf("materialize %s: write: %v", p, err)
		}
	}
}

// TestValidate_GoldenFixture_ZeroFindings round-trips the format-4 golden fixture
// (testdata/goodplan) through Validate with exactly the seven distinct paths its checked entries
// name materialized under a t.TempDir() worktreeRoot, but deliberately NOT the Custom card's own
// path-shaped target or the Rename pair's post-rename side — proving all seventeen checks pass
// simultaneously on the format-4 happy path.
func TestValidate_GoldenFixture_ZeroFindings(t *testing.T) {
	t.Parallel()

	plan, err := planparser.ParsePlan(goodPlanDir())
	if err != nil {
		t.Fatalf("ParsePlan(%q) error = %v; want nil", goodPlanDir(), err)
	}

	root := t.TempDir()
	materializeFiles(t, root,
		"internal/boardcli/list.go",          // card 2's own target, and card 3's Uses (dedup)
		"internal/output/envelope.go",        // card 2's Uses
		"internal/boardengine/legacyrows.go", // card 4's target
		"internal/boardengine/rows.go",       // card 5's Rename pair pre-rename (Old) side
		"cmd/lyx/helppins.go",                // card 6's target
		"internal/boardcli/doc.go",           // card 7's first target
		"docs/boardcli-json.md",              // card 7's second target
		// Deliberately absent: internal/output/emit.go (card 3's own Custom target — exempt) and
		// internal/boardengine/rowsjson.go (card 5's Rename New side — never checked).
	)

	findings := planparser.Validate(plan, root)
	if len(findings) != 0 {
		t.Errorf("Validate(goldenFixture, materializedRoot) = %+v; want zero findings", findings)
	}
}

// TestValidateFormat_NeverReportsApproval drives ValidateFormat and asserts format-unrecognized
// still fires on an unrecognized format: value while plan-unapproved never appears, regardless of
// whether the fixture's approved: is true, false, or (Go's zero value) absent.
func TestValidateFormat_NeverReportsApproval(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		format          int
		approved        bool
		wantFormatUnrec int
	}{
		{name: "clean, approved true", format: 4, approved: true, wantFormatUnrec: 0},
		{name: "clean, approved false", format: 4, approved: false, wantFormatUnrec: 0},
		{name: "unrecognized format, approved true", format: 3, approved: true, wantFormatUnrec: 1},
		{name: "unrecognized format, approved false", format: 3, approved: false, wantFormatUnrec: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			plan := &planparser.Plan{
				Format:   tt.format,
				Approved: tt.approved,
				Cards:    []planparser.Card{validCard(1, "only")},
			}
			findings := planparser.ValidateFormat(plan, t.TempDir())

			if got := countFor(findings, "format-unrecognized"); got != tt.wantFormatUnrec {
				t.Errorf("countFor(findings, format-unrecognized) = %d; want %d", got, tt.wantFormatUnrec)
			}
			if got := countFor(findings, "plan-unapproved"); got != 0 {
				t.Errorf("countFor(findings, plan-unapproved) = %d; want 0 (ValidateFormat never reports approval)", got)
			}
		})
	}
}

// TestValidate_FormatAndApproval covers format-unrecognized and plan-unapproved together, since
// both stem from the same overview frontmatter and manifest/designs/plan-card-format.md checks
// them as a pair.
func TestValidate_FormatAndApproval(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		format     int
		approved   bool
		wantChecks []string
	}{
		{name: "clean", format: 4, approved: true},
		{name: "unrecognized format", format: 3, approved: true, wantChecks: []string{"format-unrecognized"}},
		{name: "unapproved", format: 4, approved: false, wantChecks: []string{"plan-unapproved"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			plan := &planparser.Plan{
				Format:   tt.format,
				Approved: tt.approved,
				Cards:    []planparser.Card{validCard(1, "only")},
			}
			findings := planparser.Validate(plan, t.TempDir())

			for _, check := range []string{"format-unrecognized", "plan-unapproved"} {
				want := 0
				for _, wc := range tt.wantChecks {
					if wc == check {
						want = 1
					}
				}
				if got := countFor(findings, check); got != want {
					t.Errorf("countFor(findings, %q) = %d; want %d", check, got, want)
				}
			}
		})
	}
}

// TestValidate_FormatAndApprovalOrder asserts Validate's finding order still matches
// contracts/specs/loom-plan-spec.md's fixed order when a plan trips both format-unrecognized and
// plan-unapproved at once: format-unrecognized first, plan-unapproved second, any remaining
// findings after them.
func TestValidate_FormatAndApprovalOrder(t *testing.T) {
	t.Parallel()

	plan := &planparser.Plan{
		Format:   3,
		Approved: false,
		// A card with no type label at all also trips card-type-missing, giving this test a third
		// finding to confirm lands after the first two rather than reordering them.
		Cards: []planparser.Card{{Number: 1, Slug: "only"}},
	}
	findings := planparser.Validate(plan, t.TempDir())

	if len(findings) < 3 {
		t.Fatalf("Validate(plan, tempDir) = %+v; want at least 3 findings", findings)
	}
	if got := findings[0].Check; got != "format-unrecognized" {
		t.Errorf("findings[0].Check = %q; want %q", got, "format-unrecognized")
	}
	if got := findings[1].Check; got != "plan-unapproved" {
		t.Errorf("findings[1].Check = %q; want %q", got, "plan-unapproved")
	}
	for _, f := range findings[2:] {
		if f.Check == "format-unrecognized" || f.Check == "plan-unapproved" {
			t.Errorf("findings after position two unexpectedly repeats %q", f.Check)
		}
	}
}

// TestValidate_IndexFileMismatch covers the Card Index numbering-sequence half of
// index-file-mismatch (the orphaned-on-disk-file half is exercised implicitly by every other
// test's clean plan.Dir == "" case, where os.ReadDir fails and that half is silently skipped).
func TestValidate_IndexFileMismatch(t *testing.T) {
	t.Parallel()

	t.Run("clean", func(t *testing.T) {
		t.Parallel()
		plan := &planparser.Plan{
			Format: 4, Approved: true,
			Cards: []planparser.Card{validCard(1, "a"), validCard(2, "b")},
		}
		findings := planparser.Validate(plan, t.TempDir())
		if got := countFor(findings, "index-file-mismatch"); got != 0 {
			t.Errorf("countFor(findings, index-file-mismatch) = %d; want 0", got)
		}
	})

	t.Run("numbering gap", func(t *testing.T) {
		t.Parallel()
		plan := &planparser.Plan{
			Format: 4, Approved: true,
			// Card Index entries 1, 3 — skipping 2.
			Cards: []planparser.Card{validCard(1, "a"), validCard(3, "b")},
		}
		findings := planparser.Validate(plan, t.TempDir())
		if got := countFor(findings, "index-file-mismatch"); got != 1 {
			t.Errorf("countFor(findings, index-file-mismatch) = %d; want 1", got)
		}
	})
}

// TestValidate_CardTypeMissing covers card-type-missing: zero type labels produces one finding,
// while one label or more than one label both produce none — carrying multiple labels is legal.
func TestValidate_CardTypeMissing(t *testing.T) {
	t.Parallel()

	t.Run("clean (exactly one type label)", func(t *testing.T) {
		t.Parallel()
		plan := &planparser.Plan{Format: 4, Approved: true, Cards: []planparser.Card{validCard(1, "a")}}
		findings := planparser.Validate(plan, t.TempDir())
		if got := countFor(findings, "card-type-missing"); got != 0 {
			t.Errorf("countFor(findings, card-type-missing) = %d; want 0", got)
		}
	})

	t.Run("zero type labels", func(t *testing.T) {
		t.Parallel()
		card := validCard(1, "a")
		card.HasType = false
		card.TypeLabelCount = 0
		card.Type = planparser.CardTypeUnknown
		card.TargetGroups = nil
		plan := &planparser.Plan{Format: 4, Approved: true, Cards: []planparser.Card{card}}
		findings := planparser.Validate(plan, t.TempDir())
		if got := countFor(findings, "card-type-missing"); got != 1 {
			t.Errorf("countFor(findings, card-type-missing) = %d; want 1", got)
		}
	})

	t.Run("two type labels", func(t *testing.T) {
		t.Parallel()
		card := validCard(1, "a")
		card.TypeLabelCount = 2
		plan := &planparser.Plan{Format: 4, Approved: true, Cards: []planparser.Card{card}}
		findings := planparser.Validate(plan, t.TempDir())
		if got := countFor(findings, "card-type-missing"); got != 0 {
			t.Errorf("countFor(findings, card-type-missing) = %d; want 0 (multiple labels are legal)", got)
		}
	})
}

// TestValidate_CustomNotAlone covers card-custom-not-alone: a Custom group coexisting with a
// differently-typed group on the same card is a defect, but repeating Custom is not.
func TestValidate_CustomNotAlone(t *testing.T) {
	t.Parallel()

	t.Run("Custom group plus Edit group yields exactly one finding", func(t *testing.T) {
		t.Parallel()
		card := validCard(1, "a")
		card.Type = planparser.CardTypeCustom
		card.Targets = []string{"custom-target.go", "edit-target.go"}
		card.TargetGroups = []planparser.TargetGroup{
			{Type: planparser.CardTypeCustom, Refs: []string{"custom-target.go"}},
			{Type: planparser.CardTypeEdit, Refs: []string{"edit-target.go"}},
		}
		plan := &planparser.Plan{Format: 4, Approved: true, Cards: []planparser.Card{card}}
		findings := planparser.Validate(plan, t.TempDir())
		if got := countFor(findings, "card-custom-not-alone"); got != 1 {
			t.Errorf("countFor(findings, card-custom-not-alone) = %d; want 1", got)
		}
	})

	t.Run("Custom-only card yields none", func(t *testing.T) {
		t.Parallel()
		card := cardOfType(1, "a", planparser.CardTypeCustom, []string{"custom-target.go"})
		plan := &planparser.Plan{Format: 4, Approved: true, Cards: []planparser.Card{card}}
		findings := planparser.Validate(plan, t.TempDir())
		if got := countFor(findings, "card-custom-not-alone"); got != 0 {
			t.Errorf("countFor(findings, card-custom-not-alone) = %d; want 0", got)
		}
	})

	t.Run("two Custom groups and nothing else yields none", func(t *testing.T) {
		t.Parallel()
		card := validCard(1, "a")
		card.Type = planparser.CardTypeCustom
		card.TypeLabelCount = 2
		card.Targets = []string{"first-custom.go", "second-custom.go"}
		card.TargetGroups = []planparser.TargetGroup{
			{Type: planparser.CardTypeCustom, Refs: []string{"first-custom.go"}},
			{Type: planparser.CardTypeCustom, Refs: []string{"second-custom.go"}},
		}
		plan := &planparser.Plan{Format: 4, Approved: true, Cards: []planparser.Card{card}}
		findings := planparser.Validate(plan, t.TempDir())
		if got := countFor(findings, "card-custom-not-alone"); got != 0 {
			t.Errorf("countFor(findings, card-custom-not-alone) = %d; want 0", got)
		}
	})

	t.Run("two Custom groups plus one Edit group yields exactly one finding, not two", func(t *testing.T) {
		t.Parallel()
		card := validCard(1, "a")
		card.Type = planparser.CardTypeCustom
		card.TypeLabelCount = 3
		card.Targets = []string{"first-custom.go", "second-custom.go", "edit-target.go"}
		card.TargetGroups = []planparser.TargetGroup{
			{Type: planparser.CardTypeCustom, Refs: []string{"first-custom.go"}},
			{Type: planparser.CardTypeCustom, Refs: []string{"second-custom.go"}},
			{Type: planparser.CardTypeEdit, Refs: []string{"edit-target.go"}},
		}
		plan := &planparser.Plan{Format: 4, Approved: true, Cards: []planparser.Card{card}}
		findings := planparser.Validate(plan, t.TempDir())
		if got := countFor(findings, "card-custom-not-alone"); got != 1 {
			t.Errorf("countFor(findings, card-custom-not-alone) = %d; want 1", got)
		}
	})

	t.Run("multi-label card with no Custom group yields none", func(t *testing.T) {
		t.Parallel()
		card := validCard(1, "a")
		card.Type = planparser.CardTypeCreate
		card.TypeLabelCount = 2
		card.Targets = []string{"new-file.go", "edit-target.go"}
		card.TargetGroups = []planparser.TargetGroup{
			{Type: planparser.CardTypeCreate, Refs: []string{"new-file.go"}},
			{Type: planparser.CardTypeEdit, Refs: []string{"edit-target.go"}},
		}
		plan := &planparser.Plan{Format: 4, Approved: true, Cards: []planparser.Card{card}}
		findings := planparser.Validate(plan, t.TempDir())
		if got := countFor(findings, "card-custom-not-alone"); got != 0 {
			t.Errorf("countFor(findings, card-custom-not-alone) = %d; want 0", got)
		}
	})
}

// TestValidate_CardRetiredLabel covers card-retired-label: one finding per RetiredLabels entry.
func TestValidate_CardRetiredLabel(t *testing.T) {
	t.Parallel()

	t.Run("clean", func(t *testing.T) {
		t.Parallel()
		plan := &planparser.Plan{Format: 4, Approved: true, Cards: []planparser.Card{validCard(1, "a")}}
		findings := planparser.Validate(plan, t.TempDir())
		if got := countFor(findings, "card-retired-label"); got != 0 {
			t.Errorf("countFor(findings, card-retired-label) = %d; want 0", got)
		}
	})

	t.Run("two retired labels", func(t *testing.T) {
		t.Parallel()
		card := validCard(1, "a")
		card.RetiredLabels = []string{"**Context:**", "**verify:**"}
		plan := &planparser.Plan{Format: 4, Approved: true, Cards: []planparser.Card{card}}
		findings := planparser.Validate(plan, t.TempDir())
		if got := countFor(findings, "card-retired-label"); got != 2 {
			t.Errorf("countFor(findings, card-retired-label) = %d; want 2", got)
		}
	})
}

// TestValidate_CardPathMalformed covers card-path-malformed: the check applies to path-shaped
// entries only — a malformed symbol-shaped entry produces no finding, while a malformed
// path-shaped entry in the same list does.
func TestValidate_CardPathMalformed(t *testing.T) {
	t.Parallel()

	t.Run("clean", func(t *testing.T) {
		t.Parallel()
		plan := &planparser.Plan{Format: 4, Approved: true, Cards: []planparser.Card{validCard(1, "a")}}
		findings := planparser.Validate(plan, t.TempDir())
		if got := countFor(findings, "card-path-malformed"); got != 0 {
			t.Errorf("countFor(findings, card-path-malformed) = %d; want 0", got)
		}
	})

	t.Run("malformed symbol-shaped entry produces no finding", func(t *testing.T) {
		t.Parallel()
		card := cardOfType(1, "a", planparser.CardTypeEdit, []string{"..BadSymbol"})
		// No "/" at all, so classifyRef reads this as a symbol despite the leading "..".
		plan := &planparser.Plan{Format: 4, Approved: true, Cards: []planparser.Card{card}}
		findings := planparser.Validate(plan, t.TempDir())
		if got := countFor(findings, "card-path-malformed"); got != 0 {
			t.Errorf("countFor(findings, card-path-malformed) = %d; want 0 (symbol-shaped entries are skipped)", got)
		}
	})

	t.Run("malformed path-shaped entry produces a finding", func(t *testing.T) {
		t.Parallel()
		card := cardOfType(1, "a", planparser.CardTypeEdit, []string{"/abs/path.go"})
		plan := &planparser.Plan{Format: 4, Approved: true, Cards: []planparser.Card{card}}
		findings := planparser.Validate(plan, t.TempDir())
		if got := countFor(findings, "card-path-malformed"); got != 1 {
			t.Errorf("countFor(findings, card-path-malformed) = %d; want 1", got)
		}
	})
}

// TestValidate_RenameFormat covers rename-format: one finding per RenameRaw entry.
func TestValidate_RenameFormat(t *testing.T) {
	t.Parallel()

	t.Run("clean", func(t *testing.T) {
		t.Parallel()
		plan := &planparser.Plan{Format: 4, Approved: true, Cards: []planparser.Card{validCard(1, "a")}}
		findings := planparser.Validate(plan, t.TempDir())
		if got := countFor(findings, "rename-format"); got != 0 {
			t.Errorf("countFor(findings, rename-format) = %d; want 0", got)
		}
	})

	t.Run("two malformed Rename bullets", func(t *testing.T) {
		t.Parallel()
		card := cardOfType(1, "a", planparser.CardTypeRename, nil)
		card.RenameRaw = []string{"this bullet has no arrow", "neither does this one"}
		card.TargetGroups[0].RenameRaw = card.RenameRaw
		plan := &planparser.Plan{Format: 4, Approved: true, RenameMechanic: "mechanic", Cards: []planparser.Card{card}}
		findings := planparser.Validate(plan, t.TempDir())
		if got := countFor(findings, "rename-format"); got != 2 {
			t.Errorf("countFor(findings, rename-format) = %d; want 2", got)
		}
	})
}

// TestValidate_RenameMechanicMissing covers rename-mechanic-missing: a Rename card with an empty
// Plan.RenameMechanic produces one plan-level finding, and a plan whose only cards are other
// types produces none even with an empty section.
func TestValidate_RenameMechanicMissing(t *testing.T) {
	t.Parallel()

	t.Run("no Rename card, mechanic absent", func(t *testing.T) {
		t.Parallel()
		plan := &planparser.Plan{Format: 4, Approved: true, Cards: []planparser.Card{validCard(1, "a")}}
		findings := planparser.Validate(plan, t.TempDir())
		if got := countFor(findings, "rename-mechanic-missing"); got != 0 {
			t.Errorf("countFor(findings, rename-mechanic-missing) = %d; want 0", got)
		}
	})

	t.Run("Rename card, mechanic present", func(t *testing.T) {
		t.Parallel()
		card := cardOfType(1, "a", planparser.CardTypeRename, nil)
		plan := &planparser.Plan{Format: 4, Approved: true, RenameMechanic: "1. git mv old new first.", Cards: []planparser.Card{card}}
		findings := planparser.Validate(plan, t.TempDir())
		if got := countFor(findings, "rename-mechanic-missing"); got != 0 {
			t.Errorf("countFor(findings, rename-mechanic-missing) = %d; want 0", got)
		}
	})

	t.Run("Rename card, mechanic absent", func(t *testing.T) {
		t.Parallel()
		card := cardOfType(1, "a", planparser.CardTypeRename, nil)
		plan := &planparser.Plan{Format: 4, Approved: true, Cards: []planparser.Card{card}}
		findings := planparser.Validate(plan, t.TempDir())
		if got := countFor(findings, "rename-mechanic-missing"); got != 1 {
			t.Errorf("countFor(findings, rename-mechanic-missing) = %d; want 1", got)
		}
	})

	t.Run("Rename group on a multi-label card, mechanic absent", func(t *testing.T) {
		t.Parallel()
		card := validCard(1, "a")
		card.Type = planparser.CardTypeEdit
		renamePairs := []planparser.MovePair{{Old: "old.go", New: "new.go"}}
		card.Pairs = renamePairs
		card.Targets = []string{"pkg/card1.go", "old.go", "new.go"}
		card.TargetGroups = []planparser.TargetGroup{
			{Type: planparser.CardTypeEdit, Refs: []string{"pkg/card1.go"}},
			{Type: planparser.CardTypeRename, Refs: []string{"old.go", "new.go"}, Pairs: renamePairs},
		}
		plan := &planparser.Plan{Format: 4, Approved: true, Cards: []planparser.Card{card}}
		findings := planparser.Validate(plan, t.TempDir())
		if got := countFor(findings, "rename-mechanic-missing"); got != 1 {
			t.Errorf("countFor(findings, rename-mechanic-missing) = %d; want 1", got)
		}
	})
}

// TestValidate_CardMissingField covers card-missing-field: every card must carry Intent:, and a
// card of type Edit or Delete must also carry ImpactSummary: — a Create, Rename, Move, Prosa, or
// Custom card without ImpactSummary produces no finding.
func TestValidate_CardMissingField(t *testing.T) {
	t.Parallel()

	t.Run("clean", func(t *testing.T) {
		t.Parallel()
		plan := &planparser.Plan{Format: 4, Approved: true, Cards: []planparser.Card{validCard(1, "a")}}
		findings := planparser.Validate(plan, t.TempDir())
		if got := countFor(findings, "card-missing-field"); got != 0 {
			t.Errorf("countFor(findings, card-missing-field) = %d; want 0", got)
		}
	})

	t.Run("missing Intent", func(t *testing.T) {
		t.Parallel()
		card := validCard(1, "a")
		card.HasIntent = false
		card.Intent = ""
		plan := &planparser.Plan{Format: 4, Approved: true, Cards: []planparser.Card{card}}
		findings := planparser.Validate(plan, t.TempDir())
		if got := countFor(findings, "card-missing-field"); got != 1 {
			t.Errorf("countFor(findings, card-missing-field) = %d; want 1", got)
		}
	})

	t.Run("Edit missing ImpactSummary", func(t *testing.T) {
		t.Parallel()
		card := validCard(1, "a")
		card.HasImpactSummary = false
		card.ImpactSummary = ""
		plan := &planparser.Plan{Format: 4, Approved: true, Cards: []planparser.Card{card}}
		findings := planparser.Validate(plan, t.TempDir())
		if got := countFor(findings, "card-missing-field"); got != 1 {
			t.Errorf("countFor(findings, card-missing-field) = %d; want 1", got)
		}
	})

	t.Run("Delete missing ImpactSummary", func(t *testing.T) {
		t.Parallel()
		card := cardOfType(1, "a", planparser.CardTypeDelete, []string{"pkg/card1.go"})
		card.HasImpactSummary = false
		card.ImpactSummary = ""
		plan := &planparser.Plan{Format: 4, Approved: true, Cards: []planparser.Card{card}}
		findings := planparser.Validate(plan, t.TempDir())
		if got := countFor(findings, "card-missing-field"); got != 1 {
			t.Errorf("countFor(findings, card-missing-field) = %d; want 1", got)
		}
	})

	otherTypes := []planparser.CardType{
		planparser.CardTypeCreate, planparser.CardTypeRename, planparser.CardTypeMove,
		planparser.CardTypeProsa, planparser.CardTypeCustom,
	}
	for _, typ := range otherTypes {
		t.Run(string(typ)+" missing ImpactSummary produces none", func(t *testing.T) {
			t.Parallel()
			card := cardOfType(1, "a", typ, []string{"pkg/card1.go"})
			card.HasImpactSummary = false
			card.ImpactSummary = ""
			plan := &planparser.Plan{Format: 4, Approved: true, Cards: []planparser.Card{card}}
			findings := planparser.Validate(plan, t.TempDir())
			if got := countFor(findings, "card-missing-field"); got != 0 {
				t.Errorf("countFor(findings, card-missing-field) = %d; want 0", got)
			}
		})
	}

	t.Run("Create-plus-Edit card with no ImpactSummary produces a finding", func(t *testing.T) {
		t.Parallel()
		card := validCard(1, "a")
		card.Type = planparser.CardTypeCreate
		card.Targets = []string{"new-file.go", "edit-file.go"}
		card.TargetGroups = []planparser.TargetGroup{
			{Type: planparser.CardTypeCreate, Refs: []string{"new-file.go"}},
			{Type: planparser.CardTypeEdit, Refs: []string{"edit-file.go"}},
		}
		card.HasImpactSummary = false
		card.ImpactSummary = ""
		plan := &planparser.Plan{Format: 4, Approved: true, Cards: []planparser.Card{card}}
		findings := planparser.Validate(plan, t.TempDir())
		if got := countFor(findings, "card-missing-field"); got != 1 {
			t.Errorf("countFor(findings, card-missing-field) = %d; want 1", got)
		}
	})

	t.Run("Create-plus-Prosa card with no ImpactSummary produces none", func(t *testing.T) {
		t.Parallel()
		card := validCard(1, "a")
		card.Type = planparser.CardTypeCreate
		card.Targets = []string{"new-file.go", "doc.go"}
		card.TargetGroups = []planparser.TargetGroup{
			{Type: planparser.CardTypeCreate, Refs: []string{"new-file.go"}},
			{Type: planparser.CardTypeProsa, Refs: []string{"doc.go"}},
		}
		card.HasImpactSummary = false
		card.ImpactSummary = ""
		plan := &planparser.Plan{Format: 4, Approved: true, Cards: []planparser.Card{card}}
		findings := planparser.Validate(plan, t.TempDir())
		if got := countFor(findings, "card-missing-field"); got != 0 {
			t.Errorf("countFor(findings, card-missing-field) = %d; want 0", got)
		}
	})
}

// TestValidate_CardFieldEmpty covers card-field-empty: a present label with zero-length content
// is distinct from an absent label, checked on each of the four applicable fields.
func TestValidate_CardFieldEmpty(t *testing.T) {
	t.Parallel()

	t.Run("clean", func(t *testing.T) {
		t.Parallel()
		card := validCard(1, "a")
		// validCard's baseline Uses: is present-but-empty by design (covers the HasUses
		// clean-parse case) — give it content here so this "clean" case has no field-empty
		// findings of its own.
		card.Uses = []string{"pkg/dep.go"}
		plan := &planparser.Plan{Format: 4, Approved: true, Cards: []planparser.Card{card}}
		findings := planparser.Validate(plan, t.TempDir())
		if got := countFor(findings, "card-field-empty"); got != 0 {
			t.Errorf("countFor(findings, card-field-empty) = %d; want 0", got)
		}
	})

	t.Run("type label present with zero Targets", func(t *testing.T) {
		t.Parallel()
		card := cardOfType(1, "a", planparser.CardTypeEdit, []string{})
		card.Uses = []string{"pkg/dep.go"}
		plan := &planparser.Plan{Format: 4, Approved: true, Cards: []planparser.Card{card}}
		findings := planparser.Validate(plan, t.TempDir())
		if got := countFor(findings, "card-field-empty"); got != 1 {
			t.Errorf("countFor(findings, card-field-empty) = %d; want 1", got)
		}
	})

	t.Run("populated Edit group plus empty Create group: one finding naming Create", func(t *testing.T) {
		t.Parallel()
		card := validCard(1, "a")
		card.Uses = []string{"pkg/dep.go"}
		card.Type = planparser.CardTypeEdit
		card.TargetGroups = []planparser.TargetGroup{
			{Type: planparser.CardTypeEdit, Refs: card.Targets},
			{Type: planparser.CardTypeCreate, Refs: nil},
		}
		plan := &planparser.Plan{Format: 4, Approved: true, Cards: []planparser.Card{card}}
		findings := planparser.Validate(plan, t.TempDir())
		if got := countFor(findings, "card-field-empty"); got != 1 {
			t.Errorf("countFor(findings, card-field-empty) = %d; want 1", got)
		}
		found := false
		for _, f := range findings {
			if f.Check == "card-field-empty" && strings.Contains(f.Detail, "**Create:**") {
				found = true
			}
		}
		if !found {
			t.Errorf("card-field-empty finding = %+v; want one naming the Create label", findings)
		}
	})

	t.Run("Uses: present with zero entries", func(t *testing.T) {
		t.Parallel()
		// validCard's baseline already carries HasUses true with an empty Uses.
		card := validCard(1, "a")
		plan := &planparser.Plan{Format: 4, Approved: true, Cards: []planparser.Card{card}}
		findings := planparser.Validate(plan, t.TempDir())
		if got := countFor(findings, "card-field-empty"); got != 1 {
			t.Errorf("countFor(findings, card-field-empty) = %d; want 1", got)
		}
	})

	t.Run("Intent: present but empty", func(t *testing.T) {
		t.Parallel()
		card := validCard(1, "a")
		card.Uses = []string{"pkg/dep.go"}
		card.Intent = ""
		plan := &planparser.Plan{Format: 4, Approved: true, Cards: []planparser.Card{card}}
		findings := planparser.Validate(plan, t.TempDir())
		if got := countFor(findings, "card-field-empty"); got != 1 {
			t.Errorf("countFor(findings, card-field-empty) = %d; want 1", got)
		}
	})

	t.Run("ImpactSummary: present but empty", func(t *testing.T) {
		t.Parallel()
		card := validCard(1, "a")
		card.Uses = []string{"pkg/dep.go"}
		card.ImpactSummary = ""
		plan := &planparser.Plan{Format: 4, Approved: true, Cards: []planparser.Card{card}}
		findings := planparser.Validate(plan, t.TempDir())
		if got := countFor(findings, "card-field-empty"); got != 1 {
			t.Errorf("countFor(findings, card-field-empty) = %d; want 1", got)
		}
	})
}

// TestValidate_CardFieldOverlap covers card-field-overlap: an entry present in both a card's own
// Targets and its own Uses.
func TestValidate_CardFieldOverlap(t *testing.T) {
	t.Parallel()

	t.Run("clean", func(t *testing.T) {
		t.Parallel()
		plan := &planparser.Plan{Format: 4, Approved: true, Cards: []planparser.Card{validCard(1, "a")}}
		findings := planparser.Validate(plan, t.TempDir())
		if got := countFor(findings, "card-field-overlap"); got != 0 {
			t.Errorf("countFor(findings, card-field-overlap) = %d; want 0", got)
		}
	})

	t.Run("entry in both Targets and Uses", func(t *testing.T) {
		t.Parallel()
		card := validCard(1, "a")
		card.Uses = []string{card.Targets[0]}
		plan := &planparser.Plan{Format: 4, Approved: true, Cards: []planparser.Card{card}}
		findings := planparser.Validate(plan, t.TempDir())
		if got := countFor(findings, "card-field-overlap"); got != 1 {
			t.Errorf("countFor(findings, card-field-overlap) = %d; want 1", got)
		}
	})
}

// TestValidate_ImpactSummaryMultiline covers impact-summary-multiline: a non-empty
// ImpactSummaryTrailing is a defect, since ImpactSummary is required to stay a single line.
func TestValidate_ImpactSummaryMultiline(t *testing.T) {
	t.Parallel()

	t.Run("clean", func(t *testing.T) {
		t.Parallel()
		plan := &planparser.Plan{Format: 4, Approved: true, Cards: []planparser.Card{validCard(1, "a")}}
		findings := planparser.Validate(plan, t.TempDir())
		if got := countFor(findings, "impact-summary-multiline"); got != 0 {
			t.Errorf("countFor(findings, impact-summary-multiline) = %d; want 0", got)
		}
	})

	t.Run("trailing lines", func(t *testing.T) {
		t.Parallel()
		card := validCard(1, "a")
		card.ImpactSummaryTrailing = []string{"an unwanted second line"}
		plan := &planparser.Plan{Format: 4, Approved: true, Cards: []planparser.Card{card}}
		findings := planparser.Validate(plan, t.TempDir())
		if got := countFor(findings, "impact-summary-multiline"); got != 1 {
			t.Errorf("countFor(findings, impact-summary-multiline) = %d; want 1", got)
		}
	})
}

// TestValidate_ProsaSymbolTarget covers prosa-symbol-target: a Prosa card's target list must hold
// only file(s), never a symbol.
func TestValidate_ProsaSymbolTarget(t *testing.T) {
	t.Parallel()

	t.Run("clean (path-only Prosa)", func(t *testing.T) {
		t.Parallel()
		card := cardOfType(1, "a", planparser.CardTypeProsa, []string{"doc.go"})
		plan := &planparser.Plan{Format: 4, Approved: true, Cards: []planparser.Card{card}}
		findings := planparser.Validate(plan, t.TempDir())
		if got := countFor(findings, "prosa-symbol-target"); got != 0 {
			t.Errorf("countFor(findings, prosa-symbol-target) = %d; want 0", got)
		}
	})

	t.Run("symbol target on a Prosa card", func(t *testing.T) {
		t.Parallel()
		card := cardOfType(1, "a", planparser.CardTypeProsa, []string{"pkg.Symbol"})
		plan := &planparser.Plan{Format: 4, Approved: true, Cards: []planparser.Card{card}}
		findings := planparser.Validate(plan, t.TempDir())
		if got := countFor(findings, "prosa-symbol-target"); got != 1 {
			t.Errorf("countFor(findings, prosa-symbol-target) = %d; want 1", got)
		}
	})

	t.Run("Edit group symbol plus Prosa group symbol: only the Prosa group's is flagged", func(t *testing.T) {
		t.Parallel()
		card := validCard(1, "a")
		card.Type = planparser.CardTypeEdit
		card.Targets = []string{"pkg.EditSymbol", "pkg.ProsaSymbol"}
		card.TargetGroups = []planparser.TargetGroup{
			{Type: planparser.CardTypeEdit, Refs: []string{"pkg.EditSymbol"}},
			{Type: planparser.CardTypeProsa, Refs: []string{"pkg.ProsaSymbol"}},
		}
		plan := &planparser.Plan{Format: 4, Approved: true, Cards: []planparser.Card{card}}
		findings := planparser.Validate(plan, t.TempDir())
		if got := countFor(findings, "prosa-symbol-target"); got != 1 {
			t.Errorf("countFor(findings, prosa-symbol-target) = %d; want 1", got)
		}
	})

	t.Run("symbol lives only in the Edit group: no finding", func(t *testing.T) {
		t.Parallel()
		card := cardOfType(1, "a", planparser.CardTypeEdit, []string{"pkg.EditSymbol"})
		plan := &planparser.Plan{Format: 4, Approved: true, Cards: []planparser.Card{card}}
		findings := planparser.Validate(plan, t.TempDir())
		if got := countFor(findings, "prosa-symbol-target"); got != 0 {
			t.Errorf("countFor(findings, prosa-symbol-target) = %d; want 0", got)
		}
	})
}

// TestValidate_CardNumbering covers card-numbering: the card file's own heading number must match
// the number the Card Index assigned it.
// Unlike most other checks, this one re-reads the card file from plan.Dir, so both cases need a
// real on-disk plan directory rather than a hand-built Plan struct.
func TestValidate_CardNumbering(t *testing.T) {
	t.Parallel()

	t.Run("clean (golden fixture)", func(t *testing.T) {
		t.Parallel()
		plan, err := planparser.ParsePlan(goodPlanDir())
		if err != nil {
			t.Fatalf("ParsePlan(%q) error = %v; want nil", goodPlanDir(), err)
		}
		findings := planparser.Validate(plan, t.TempDir())
		if got := countFor(findings, "card-numbering"); got != 0 {
			t.Errorf("countFor(findings, card-numbering) = %d; want 0", got)
		}
	})

	t.Run("heading number mismatch", func(t *testing.T) {
		t.Parallel()
		dir := writePlanFiles(t, map[string]string{
			"00-overview.md": minimalOverview,
			// The Card Index assigns this file (01-only.md) card number 1,
			// but its own heading declares "# Card 2" — the exact mismatch
			// checkCardNumbering exists to catch.
			"01-only.md": "# Card 2 — only\n\n**Edit:**\n- `a.go`\n**Intent:** placeholder.\n",
		})
		plan, err := planparser.ParsePlan(dir)
		if err != nil {
			t.Fatalf("ParsePlan(%q) error = %v; want nil", dir, err)
		}
		findings := planparser.Validate(plan, t.TempDir())
		if got := countFor(findings, "card-numbering"); got != 1 {
			t.Errorf("countFor(findings, card-numbering) = %d; want 1", got)
		}
	})
}

// TestValidate_PathMissing exhaustively pins path-missing's type-conditional rework, using a
// hermetic t.TempDir() worktree root for every case.
func TestValidate_PathMissing(t *testing.T) {
	t.Parallel()

	t.Run("Edit/Delete/Move/Prosa absent target produces a finding", func(t *testing.T) {
		t.Parallel()
		for _, typ := range []planparser.CardType{
			planparser.CardTypeEdit, planparser.CardTypeDelete, planparser.CardTypeMove, planparser.CardTypeProsa,
		} {
			t.Run(string(typ), func(t *testing.T) {
				t.Parallel()
				card := cardOfType(1, "a", typ, []string{"missing.go"})
				plan := &planparser.Plan{Format: 4, Approved: true, Cards: []planparser.Card{card}}
				findings := planparser.Validate(plan, t.TempDir())
				if got := countFor(findings, "path-missing"); got != 1 {
					t.Errorf("countFor(findings, path-missing) = %d; want 1", got)
				}
			})
		}
	})

	t.Run("Create card absent target produces none", func(t *testing.T) {
		t.Parallel()
		card := cardOfType(1, "a", planparser.CardTypeCreate, []string{"missing.go"})
		plan := &planparser.Plan{Format: 4, Approved: true, Cards: []planparser.Card{card}}
		findings := planparser.Validate(plan, t.TempDir())
		if got := countFor(findings, "path-missing"); got != 0 {
			t.Errorf("countFor(findings, path-missing) = %d; want 0", got)
		}
	})

	t.Run("Rename pair: absent Old side finds, absent New side does not", func(t *testing.T) {
		t.Parallel()
		card := cardOfType(1, "a", planparser.CardTypeRename, []string{"missing-old.go", "missing-new.go"})
		pairs := []planparser.MovePair{{Old: "missing-old.go", New: "missing-new.go"}}
		card.Pairs = pairs
		card.TargetGroups[0].Pairs = pairs
		plan := &planparser.Plan{Format: 4, Approved: true, RenameMechanic: "mechanic", Cards: []planparser.Card{card}}
		findings := planparser.Validate(plan, t.TempDir())
		if got := countFor(findings, "path-missing"); got != 1 {
			t.Errorf("countFor(findings, path-missing) = %d; want 1 (only the Old side is checked)", got)
		}
	})

	t.Run("two Rename groups: one absent Old side yields exactly one finding", func(t *testing.T) {
		t.Parallel()
		card := validCard(1, "a")
		firstPairs := []planparser.MovePair{{Old: "missing-old.go", New: "missing-new.go"}}
		secondPairs := []planparser.MovePair{{Old: "present-old.go", New: "present-new.go"}}
		card.Type = planparser.CardTypeRename
		card.Pairs = append(append([]planparser.MovePair{}, firstPairs...), secondPairs...)
		card.Targets = []string{"missing-old.go", "missing-new.go", "present-old.go", "present-new.go"}
		card.TargetGroups = []planparser.TargetGroup{
			{Type: planparser.CardTypeRename, Refs: []string{"missing-old.go", "missing-new.go"}, Pairs: firstPairs},
			{Type: planparser.CardTypeRename, Refs: []string{"present-old.go", "present-new.go"}, Pairs: secondPairs},
		}
		root := t.TempDir()
		materializeFiles(t, root, "present-old.go")
		plan := &planparser.Plan{Format: 4, Approved: true, RenameMechanic: "mechanic", Cards: []planparser.Card{card}}
		findings := planparser.Validate(plan, root)
		if got := countFor(findings, "path-missing"); got != 1 {
			t.Errorf("countFor(findings, path-missing) = %d; want 1 (one finding per group, not per card)", got)
		}
	})

	t.Run("Custom card: absent own target does not find, absent Uses path does", func(t *testing.T) {
		t.Parallel()
		card := cardOfType(1, "a", planparser.CardTypeCustom, []string{"missing-target.go"})
		card.HasUses = true
		card.Uses = []string{"missing-uses.go"}
		plan := &planparser.Plan{Format: 4, Approved: true, Cards: []planparser.Card{card}}
		findings := planparser.Validate(plan, t.TempDir())
		if got := countFor(findings, "path-missing"); got != 1 {
			t.Errorf("countFor(findings, path-missing) = %d; want 1 (from Uses only; Custom targets are exempt)", got)
		}
	})

	t.Run("Edit group absent, Create group on the same card absent: only the Edit path is reported", func(t *testing.T) {
		t.Parallel()
		card := validCard(1, "a")
		card.Type = planparser.CardTypeEdit
		card.Targets = []string{"missing-edit.go", "missing-create.go"}
		card.TargetGroups = []planparser.TargetGroup{
			{Type: planparser.CardTypeEdit, Refs: []string{"missing-edit.go"}},
			{Type: planparser.CardTypeCreate, Refs: []string{"missing-create.go"}},
		}
		plan := &planparser.Plan{Format: 4, Approved: true, Cards: []planparser.Card{card}}
		findings := planparser.Validate(plan, t.TempDir())
		if got := countFor(findings, "path-missing"); got != 1 {
			t.Errorf("countFor(findings, path-missing) = %d; want 1", got)
		}
		for _, f := range findings {
			if f.Check == "path-missing" && !strings.Contains(f.Detail, "missing-edit.go") {
				t.Errorf("path-missing finding %+v; want it to name missing-edit.go", f)
			}
		}
	})

	t.Run("first group Create, second group Edit: the Edit group's absent path is still reported", func(t *testing.T) {
		t.Parallel()
		card := validCard(1, "a")
		card.Type = planparser.CardTypeCreate
		card.Targets = []string{"new-file.go", "missing-edit.go"}
		card.TargetGroups = []planparser.TargetGroup{
			{Type: planparser.CardTypeCreate, Refs: []string{"new-file.go"}},
			{Type: planparser.CardTypeEdit, Refs: []string{"missing-edit.go"}},
		}
		plan := &planparser.Plan{Format: 4, Approved: true, Cards: []planparser.Card{card}}
		findings := planparser.Validate(plan, t.TempDir())
		if got := countFor(findings, "path-missing"); got != 1 {
			t.Errorf("countFor(findings, path-missing) = %d; want 1 (first-label-wins is gone)", got)
		}
	})

	t.Run("otherwise-missing path satisfied by the Create or Rename-New union", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()

		create := cardOfType(1, "create", planparser.CardTypeCreate, []string{"new-file.go"})

		// Custom so this card's own (irrelevant) Targets entry is exempt from path-missing,
		// isolating the assertion to its Uses: satisfaction by the Create union.
		usesCreateTarget := cardOfType(2, "uses-create-target", planparser.CardTypeCustom, []string{"pkg/card2.go"})
		usesCreateTarget.Uses = []string{"new-file.go"}

		rename := cardOfType(3, "rename", planparser.CardTypeRename, []string{"orig.go", "renamed.go"})
		renamePairs := []planparser.MovePair{{Old: "orig.go", New: "renamed.go"}}
		rename.Pairs = renamePairs
		rename.TargetGroups[0].Pairs = renamePairs
		materializeFiles(t, root, "orig.go")

		usesRenameTarget := cardOfType(4, "uses-rename-target", planparser.CardTypeCustom, []string{"pkg/card4.go"})
		usesRenameTarget.Uses = []string{"renamed.go"}

		plan := &planparser.Plan{
			Format: 4, Approved: true, RenameMechanic: "mechanic",
			Cards: []planparser.Card{create, usesCreateTarget, rename, usesRenameTarget},
		}
		findings := planparser.Validate(plan, root)
		if got := countFor(findings, "path-missing"); got != 0 {
			t.Errorf("countFor(findings, path-missing) = %d; want 0", got)
		}
	})

	t.Run("Create group on an otherwise-Edit card satisfies a later card's Edit target on the same path", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()

		createAndEdit := validCard(1, "create-and-edit")
		createAndEdit.Type = planparser.CardTypeCreate
		createAndEdit.Targets = []string{"shared-new.go", "own-edit.go"}
		createAndEdit.TargetGroups = []planparser.TargetGroup{
			{Type: planparser.CardTypeCreate, Refs: []string{"shared-new.go"}},
			{Type: planparser.CardTypeEdit, Refs: []string{"own-edit.go"}},
		}
		materializeFiles(t, root, "own-edit.go")

		editSharedNew := cardOfType(2, "edit-shared-new", planparser.CardTypeEdit, []string{"shared-new.go"})

		plan := &planparser.Plan{
			Format: 4, Approved: true,
			Cards: []planparser.Card{createAndEdit, editSharedNew},
		}
		findings := planparser.Validate(plan, root)
		if got := countFor(findings, "path-missing"); got != 0 {
			t.Errorf("countFor(findings, path-missing) = %d; want 0 (legitimate cross-card create-then-edit sequencing)", got)
		}
	})
}

// TestValidate_CommitSubjectMismatch covers commit-subject-mismatch: a present Commit: must start
// with the card's own "N: " prefix.
func TestValidate_CommitSubjectMismatch(t *testing.T) {
	t.Parallel()

	t.Run("clean", func(t *testing.T) {
		t.Parallel()
		plan := &planparser.Plan{Format: 4, Approved: true, Cards: []planparser.Card{validCard(1, "a")}}
		findings := planparser.Validate(plan, t.TempDir())
		if got := countFor(findings, "commit-subject-mismatch"); got != 0 {
			t.Errorf("countFor(findings, commit-subject-mismatch) = %d; want 0", got)
		}
	})

	t.Run("wrong prefix", func(t *testing.T) {
		t.Parallel()
		card := validCard(1, "a")
		card.Commit = "2: wrong prefix"
		plan := &planparser.Plan{Format: 4, Approved: true, Cards: []planparser.Card{card}}
		findings := planparser.Validate(plan, t.TempDir())
		if got := countFor(findings, "commit-subject-mismatch"); got != 1 {
			t.Errorf("countFor(findings, commit-subject-mismatch) = %d; want 1", got)
		}
	})
}

// TestValidate_CustomCardBoundByGenericChecks proves a Custom card remains bound by the
// card-generic checks despite being validate.go's explicit escape hatch on the type-conditional
// checks (path-missing's own-target exemption, card-missing-field's ImpactSummary exemption): a
// malformed path-shaped target, a missing Intent:, an entry duplicated across Targets and Uses,
// and a badly prefixed Commit: each still fire, so a blanket-skip regression would fail this test.
func TestValidate_CustomCardBoundByGenericChecks(t *testing.T) {
	t.Parallel()

	card := cardOfType(1, "a", planparser.CardTypeCustom, []string{"/abs/malformed.go", "shared.go"})
	card.HasIntent = false
	card.Intent = ""
	card.HasUses = true
	card.Uses = []string{"shared.go"}
	card.Commit = "9: wrong prefix"

	plan := &planparser.Plan{Format: 4, Approved: true, Cards: []planparser.Card{card}}
	findings := planparser.Validate(plan, t.TempDir())

	for _, check := range []string{"card-path-malformed", "card-missing-field", "card-field-overlap", "commit-subject-mismatch"} {
		if got := countFor(findings, check); got != 1 {
			t.Errorf("countFor(findings, %q) = %d; want 1", check, got)
		}
	}
}
