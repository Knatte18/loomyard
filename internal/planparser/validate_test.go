// validate_test.go covers all seventeen of Validate's format-4 checks, each with at least one
// triggering and one clean case, per manifest/designs/plan-card-format.md's seventeen distinct
// ValidationError.Check IDs — sixteen of which ValidateFormat also emits, everything but
// plan-unapproved.
// The golden happy-path test reuses the format-4 seven-card golden fixture (testdata/goodplan,
// already parsed by parse_test.go's TestParsePlan_GoldenFixture) and materializes exactly the
// seven distinct paths its checked entries name under a hermetic t.TempDir() worktree root,
// deliberately leaving absent the Custom card's own path-shaped target
// (internal/output/emit.go), the Rename pair's post-rename side
// (internal/boardengine/rowsjson.go), and card 2's own Create-group target
// (internal/boardcli/list_json_test.go) — proving all three exemptions positively rather than by
// omission — so the whole seventeen-check Validate run returns zero findings.

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
// path-shaped target, the Rename pair's post-rename side, or card 2's own Create-group target —
// proving all seventeen checks pass simultaneously on the format-4 happy path, including card 2's
// multi-label Edit-plus-Create shape.
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
		// Deliberately absent: internal/output/emit.go (card 3's own Custom target — exempt),
		// internal/boardengine/rowsjson.go (card 5's Rename New side — never checked), and
		// internal/boardcli/list_json_test.go (card 2's own Create-group target — exempt).
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
		{name: "clean, approved true", format: 5, approved: true, wantFormatUnrec: 0},
		{name: "clean, approved false", format: 5, approved: false, wantFormatUnrec: 0},
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
		{name: "clean", format: 5, approved: true},
		{name: "unrecognized format", format: 3, approved: true, wantChecks: []string{"format-unrecognized"}},
		{name: "unapproved", format: 5, approved: false, wantChecks: []string{"plan-unapproved"}},
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

// TestValidate_UnrecognizedLanguageSilencesEveryAlphabetGatedCheck is R6-22's regression test:
// bare-symbol-target and directory-target gated on the literal "none" while every sibling
// alphabet-gated check gated on planLanguage, so under an unrecognized language: those two kept
// classifying refs ParsePlan never canonicalized. plan-language-unrecognized already blocks such a
// plan, so the extra findings were noise.
func TestValidate_UnrecognizedLanguageSilencesEveryAlphabetGatedCheck(t *testing.T) {
	t.Parallel()

	plan := &planparser.Plan{
		Format: 5, Approved: true, Language: "python",
		Cards: []planparser.Card{{
			Number: 1, Slug: "a",
			Targets:      []string{"boardcli.RowJSON", "internal/boardcli"},
			TargetGroups: []planparser.TargetGroup{{Type: planparser.CardTypeEdit, Refs: []string{"boardcli.RowJSON", "internal/boardcli"}}},
			Intent:       "one",
		}},
	}
	findings := planparser.Validate(plan, t.TempDir())
	for _, check := range []string{"bare-symbol-target", "directory-target"} {
		if got := countFor(findings, check); got != 0 {
			t.Errorf("countFor(findings, %s) = %d; want 0 under an unrecognized language:", check, got)
		}
	}
	if got := countFor(findings, "plan-language-unrecognized"); got != 1 {
		t.Errorf("countFor(findings, plan-language-unrecognized) = %d; want 1 — that is the finding an unrecognized language earns", got)
	}
}

// TestValidate_IndexFileMismatch covers the Card Index numbering-sequence half of
// index-file-mismatch. An empty plan.Dir -- the in-memory plan shape most cases here build -- means
// "no plan directory was told" and scans nothing; a Dir that IS told but cannot be listed is its own
// finding (see TestValidate_IndexFileMismatch_UnlistablePlanDirIsAFinding).
func TestValidate_IndexFileMismatch(t *testing.T) {
	t.Parallel()

	t.Run("clean", func(t *testing.T) {
		t.Parallel()
		plan := &planparser.Plan{
			Format: 5, Approved: true,
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
			Format: 5, Approved: true,
			// Card Index entries 1, 3 — skipping 2.
			Cards: []planparser.Card{validCard(1, "a"), validCard(3, "b")},
		}
		findings := planparser.Validate(plan, t.TempDir())
		if got := countFor(findings, "index-file-mismatch"); got != 1 {
			t.Errorf("countFor(findings, index-file-mismatch) = %d; want 1", got)
		}
	})

	// R6-10: a told plan directory that cannot be listed silently disabled the whole
	// orphaned-card-file half of this check, with no finding and no error, so the check reported
	// CLEAN against its own unconditional guarantee.
	t.Run("an unlistable plan directory is a finding, not silence", func(t *testing.T) {
		t.Parallel()
		plan := &planparser.Plan{
			Format: 5, Approved: true,
			Dir:   filepath.Join(t.TempDir(), "gone"),
			Cards: []planparser.Card{validCard(1, "a")},
		}
		findings := planparser.Validate(plan, t.TempDir())
		if got := countFor(findings, "index-file-mismatch"); got != 1 {
			t.Errorf("countFor(findings, index-file-mismatch) = %d; want 1 — a plan directory that cannot be listed is a defect at this gate", got)
		}
	})

	t.Run("a plan directory containing amendments.md produces no finding", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, planparser.AmendmentsFileName), []byte("# Amendments\n"), 0o644); err != nil {
			t.Fatalf("write amendments file: %v", err)
		}
		plan := &planparser.Plan{
			Dir: dir, Format: 5, Approved: true,
			Cards: []planparser.Card{validCard(1, "a")},
		}
		findings := planparser.Validate(plan, t.TempDir())
		if got := countFor(findings, "index-file-mismatch"); got != 0 {
			t.Errorf("countFor(findings, index-file-mismatch) = %d; want 0 (amendments.md is a known non-card file)", got)
		}
	})
}

// TestValidate_CardTypeMissing covers card-type-missing: zero type labels produces one finding,
// while one label or more than one label both produce none — carrying multiple labels is legal.
func TestValidate_CardTypeMissing(t *testing.T) {
	t.Parallel()

	t.Run("clean (exactly one type label)", func(t *testing.T) {
		t.Parallel()
		plan := &planparser.Plan{Format: 5, Approved: true, Cards: []planparser.Card{validCard(1, "a")}}
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
		plan := &planparser.Plan{Format: 5, Approved: true, Cards: []planparser.Card{card}}
		findings := planparser.Validate(plan, t.TempDir())
		if got := countFor(findings, "card-type-missing"); got != 1 {
			t.Errorf("countFor(findings, card-type-missing) = %d; want 1", got)
		}
	})

	t.Run("two type labels", func(t *testing.T) {
		t.Parallel()
		card := validCard(1, "a")
		card.TypeLabelCount = 2
		plan := &planparser.Plan{Format: 5, Approved: true, Cards: []planparser.Card{card}}
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
		plan := &planparser.Plan{Format: 5, Approved: true, Cards: []planparser.Card{card}}
		findings := planparser.Validate(plan, t.TempDir())
		if got := countFor(findings, "card-custom-not-alone"); got != 1 {
			t.Errorf("countFor(findings, card-custom-not-alone) = %d; want 1", got)
		}
	})

	t.Run("Custom-only card yields none", func(t *testing.T) {
		t.Parallel()
		card := cardOfType(1, "a", planparser.CardTypeCustom, []string{"custom-target.go"})
		plan := &planparser.Plan{Format: 5, Approved: true, Cards: []planparser.Card{card}}
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
		plan := &planparser.Plan{Format: 5, Approved: true, Cards: []planparser.Card{card}}
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
		plan := &planparser.Plan{Format: 5, Approved: true, Cards: []planparser.Card{card}}
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
		plan := &planparser.Plan{Format: 5, Approved: true, Cards: []planparser.Card{card}}
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
		plan := &planparser.Plan{Format: 5, Approved: true, Cards: []planparser.Card{validCard(1, "a")}}
		findings := planparser.Validate(plan, t.TempDir())
		if got := countFor(findings, "card-retired-label"); got != 0 {
			t.Errorf("countFor(findings, card-retired-label) = %d; want 0", got)
		}
	})

	t.Run("two retired labels", func(t *testing.T) {
		t.Parallel()
		card := validCard(1, "a")
		card.RetiredLabels = []string{"**Context:**", "**verify:**"}
		plan := &planparser.Plan{Format: 5, Approved: true, Cards: []planparser.Card{card}}
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
		plan := &planparser.Plan{Format: 5, Approved: true, Cards: []planparser.Card{validCard(1, "a")}}
		findings := planparser.Validate(plan, t.TempDir())
		if got := countFor(findings, "card-path-malformed"); got != 0 {
			t.Errorf("countFor(findings, card-path-malformed) = %d; want 0", got)
		}
	})

	t.Run("malformed symbol-shaped entry produces no finding", func(t *testing.T) {
		t.Parallel()
		card := cardOfType(1, "a", planparser.CardTypeEdit, []string{"..BadSymbol"})
		// No "/" at all, so classifyRef reads this as a symbol despite the leading "..".
		plan := &planparser.Plan{Format: 5, Approved: true, Cards: []planparser.Card{card}}
		findings := planparser.Validate(plan, t.TempDir())
		if got := countFor(findings, "card-path-malformed"); got != 0 {
			t.Errorf("countFor(findings, card-path-malformed) = %d; want 0 (symbol-shaped entries are skipped)", got)
		}
	})

	t.Run("malformed path-shaped entry produces a finding", func(t *testing.T) {
		t.Parallel()
		card := cardOfType(1, "a", planparser.CardTypeEdit, []string{"/abs/path.go"})
		plan := &planparser.Plan{Format: 5, Approved: true, Cards: []planparser.Card{card}}
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
		plan := &planparser.Plan{Format: 5, Approved: true, Cards: []planparser.Card{validCard(1, "a")}}
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
		plan := &planparser.Plan{Format: 5, Approved: true, RenameMechanic: "mechanic", Cards: []planparser.Card{card}}
		findings := planparser.Validate(plan, t.TempDir())
		if got := countFor(findings, "rename-format"); got != 2 {
			t.Errorf("countFor(findings, rename-format) = %d; want 2", got)
		}
	})
}

// TestValidate_HandleConsistency covers handle-dangling, handle-collision, and
// handle-unreferenced, each firing exactly once on a minimal offending plan, and a well-formed
// plan with one declaration and one reference producing none of them.
func TestValidate_HandleConsistency(t *testing.T) {
	t.Parallel()

	t.Run("well-formed plan with one declaration and one reference produces no findings", func(t *testing.T) {
		t.Parallel()
		declarer := cardOfType(1, "declarer", planparser.CardTypeCreate, []string{"plan:internal/foo#NewThing"})
		declarer.Declarations = []planparser.CardDeclaration{{Handle: "plan:internal/foo#NewThing", Decl: "func NewThing() *Thing"}}
		declarer.TargetGroups[0].Declarations = declarer.Declarations
		referencer := validCard(2, "referencer")
		referencer.HasUses = true
		referencer.Uses = []string{"plan:internal/foo#NewThing"}
		plan := &planparser.Plan{Format: 5, Approved: true, Cards: []planparser.Card{declarer, referencer}}
		findings := planparser.Validate(plan, t.TempDir())
		for _, check := range []string{"handle-dangling", "handle-collision", "handle-unreferenced"} {
			if got := countFor(findings, check); got != 0 {
				t.Errorf("countFor(findings, %q) = %d; want 0", check, got)
			}
		}
	})

	t.Run("handle-dangling: a referenced handle with no declaration", func(t *testing.T) {
		t.Parallel()
		card := validCard(1, "a")
		card.HasUses = true
		card.Uses = []string{"plan:internal/foo#NewThing"}
		plan := &planparser.Plan{Format: 5, Approved: true, Cards: []planparser.Card{card}}
		findings := planparser.Validate(plan, t.TempDir())
		if got := countFor(findings, "handle-dangling"); got != 1 {
			t.Errorf("countFor(findings, handle-dangling) = %d; want 1", got)
		}
	})

	t.Run("handle-dangling: exempt when satisfied by a Rename to-side", func(t *testing.T) {
		t.Parallel()
		renamer := cardOfType(1, "renamer", planparser.CardTypeRename, nil)
		renamer.Pairs = []planparser.MovePair{{Old: "internal/foo#OldThing", New: "plan:internal/foo#NewThing"}}
		renamer.Targets = []string{"internal/foo#OldThing", "plan:internal/foo#NewThing"}
		renamer.TargetGroups[0].Pairs = renamer.Pairs
		renamer.TargetGroups[0].Refs = renamer.Targets
		card := validCard(2, "b")
		card.HasUses = true
		card.Uses = []string{"plan:internal/foo#NewThing"}
		plan := &planparser.Plan{Format: 5, Approved: true, RenameMechanic: "mechanic", Cards: []planparser.Card{renamer, card}}
		findings := planparser.Validate(plan, t.TempDir())
		if got := countFor(findings, "handle-dangling"); got != 0 {
			t.Errorf("countFor(findings, handle-dangling) = %d; want 0", got)
		}
	})

	t.Run("handle-collision: the same handle declared twice", func(t *testing.T) {
		t.Parallel()
		one := cardOfType(1, "one", planparser.CardTypeCreate, []string{"plan:internal/foo#NewThing"})
		one.Declarations = []planparser.CardDeclaration{{Handle: "plan:internal/foo#NewThing", Decl: "func NewThing() *Thing"}}
		one.TargetGroups[0].Declarations = one.Declarations
		two := cardOfType(2, "two", planparser.CardTypeCreate, []string{"plan:internal/foo#NewThing"})
		two.Declarations = []planparser.CardDeclaration{{Handle: "plan:internal/foo#NewThing", Decl: "func NewThing() *Thing"}}
		two.TargetGroups[0].Declarations = two.Declarations
		plan := &planparser.Plan{Format: 5, Approved: true, Cards: []planparser.Card{one, two}}
		findings := planparser.Validate(plan, t.TempDir())
		if got := countFor(findings, "handle-collision"); got != 1 {
			t.Errorf("countFor(findings, handle-collision) = %d; want 1", got)
		}
	})

	t.Run("handle-collision: one Create declaration and one Rename to-side claiming the same handle", func(t *testing.T) {
		t.Parallel()
		one := cardOfType(1, "one", planparser.CardTypeCreate, []string{"plan:internal/foo#NewThing"})
		one.Declarations = []planparser.CardDeclaration{{Handle: "plan:internal/foo#NewThing", Decl: "func NewThing() *Thing"}}
		one.TargetGroups[0].Declarations = one.Declarations
		two := renameCard(2, "two", "internal/bar#OldThing", "plan:internal/foo#NewThing")
		plan := &planparser.Plan{Format: 5, Approved: true, RenameMechanic: "mechanic", Cards: []planparser.Card{one, two}}
		findings := planparser.Validate(plan, t.TempDir())
		if got := countFor(findings, "handle-collision"); got != 1 {
			t.Errorf("countFor(findings, handle-collision) = %d; want 1 — two sources claiming one handle canonicalize to two different glyphs and one silently wins (R9-3)", got)
		}
	})

	t.Run("handle-collision: two Rename to-sides claiming the same handle", func(t *testing.T) {
		t.Parallel()
		one := renameCard(1, "one", "internal/bar#OldOne", "plan:internal/foo#NewThing")
		two := renameCard(2, "two", "internal/baz#OldTwo", "plan:internal/foo#NewThing")
		plan := &planparser.Plan{Format: 5, Approved: true, RenameMechanic: "mechanic", Cards: []planparser.Card{one, two}}
		findings := planparser.Validate(plan, t.TempDir())
		if got := countFor(findings, "handle-collision"); got != 1 {
			t.Errorf("countFor(findings, handle-collision) = %d; want 1", got)
		}
	})

	t.Run("a lone Rename to-side handle is neither a collision nor unreferenced", func(t *testing.T) {
		t.Parallel()
		card := renameCard(1, "one", "internal/bar#OldThing", "plan:internal/foo#NewThing")
		plan := &planparser.Plan{Format: 5, Approved: true, RenameMechanic: "mechanic", Cards: []planparser.Card{card}}
		findings := planparser.Validate(plan, t.TempDir())
		for _, check := range []string{"handle-dangling", "handle-collision", "handle-unreferenced"} {
			if got := countFor(findings, check); got != 0 {
				t.Errorf("countFor(findings, %q) = %d; want 0 — a rename destination nothing else references is the ordinary case", check, got)
			}
		}
	})

	t.Run("handle-unreferenced: a declared handle no other card references", func(t *testing.T) {
		t.Parallel()
		card := cardOfType(1, "a", planparser.CardTypeCreate, []string{"plan:internal/foo#NewThing"})
		card.Declarations = []planparser.CardDeclaration{{Handle: "plan:internal/foo#NewThing", Decl: "func NewThing() *Thing"}}
		card.TargetGroups[0].Declarations = card.Declarations
		plan := &planparser.Plan{Format: 5, Approved: true, Cards: []planparser.Card{card}}
		findings := planparser.Validate(plan, t.TempDir())
		if got := countFor(findings, "handle-unreferenced"); got != 1 {
			t.Errorf("countFor(findings, handle-unreferenced) = %d; want 1", got)
		}
	})

	t.Run("runs under language: none", func(t *testing.T) {
		t.Parallel()
		card := validCard(1, "a")
		card.HasUses = true
		card.Uses = []string{"plan:internal/foo#NewThing"}
		plan := &planparser.Plan{Format: 5, Approved: true, Language: "none", Cards: []planparser.Card{card}}
		findings := planparser.Validate(plan, t.TempDir())
		if got := countFor(findings, "handle-dangling"); got != 1 {
			t.Errorf("countFor(findings, handle-dangling) = %d; want 1 (handle checks run under language: none)", got)
		}
	})
}

// TestValidate_HandleMalformed covers handle-malformed: one finding per CreateRaw entry, and one
// finding for a handle whose text after HandlePrefix carries no "#".
func TestValidate_HandleMalformed(t *testing.T) {
	t.Parallel()

	t.Run("clean", func(t *testing.T) {
		t.Parallel()
		plan := &planparser.Plan{Format: 5, Approved: true, Cards: []planparser.Card{validCard(1, "a")}}
		findings := planparser.Validate(plan, t.TempDir())
		if got := countFor(findings, "handle-malformed"); got != 0 {
			t.Errorf("countFor(findings, handle-malformed) = %d; want 0", got)
		}
	})

	t.Run("malformed Create arrow bullet", func(t *testing.T) {
		t.Parallel()
		card := cardOfType(1, "a", planparser.CardTypeCreate, nil)
		card.CreateRaw = []string{"this bullet has -> an arrow but no backticks"}
		card.TargetGroups[0].CreateRaw = card.CreateRaw
		plan := &planparser.Plan{Format: 5, Approved: true, Cards: []planparser.Card{card}}
		findings := planparser.Validate(plan, t.TempDir())
		if got := countFor(findings, "handle-malformed"); got != 1 {
			t.Errorf("countFor(findings, handle-malformed) = %d; want 1", got)
		}
	})

	t.Run("handle with no # names no unit", func(t *testing.T) {
		t.Parallel()
		card := validCard(1, "a")
		card.HasUses = true
		card.Uses = []string{"plan:noUnitAtAll"}
		plan := &planparser.Plan{Format: 5, Approved: true, Cards: []planparser.Card{card}}
		findings := planparser.Validate(plan, t.TempDir())
		if got := countFor(findings, "handle-malformed"); got != 1 {
			t.Errorf("countFor(findings, handle-malformed) = %d; want 1", got)
		}
	})
}

// renameCard returns a well-formed Rename Card at position number with slug carrying exactly one
// Pairs entry (old -> new), for checkRenamePairShape's subtests.
func renameCard(number int, slug, oldSide, newSide string) planparser.Card {
	pairs := []planparser.MovePair{{Old: oldSide, New: newSide}}
	refs := []string{oldSide, newSide}
	card := cardOfType(number, slug, planparser.CardTypeRename, refs)
	card.Pairs = pairs
	card.TargetGroups[0].Pairs = pairs
	return card
}

// TestValidate_RenamePairShape covers rename-to-not-handle and rename-from-not-glyph: a symbol
// rename's old side must classify as a glyph and its new side must classify as a plan: handle,
// with a file-rename pair (both endpoints self glyphs) exempt from both, and neither check running
// under plan.Language "none".
//
// Both checks are the negation of the one admitted shape, never an enumeration of the forbidden
// ones — the sub-tests naming a path-shaped side are R9-2's regression, since the enumerated form
// let refKindPath through both halves.
func TestValidate_RenamePairShape(t *testing.T) {
	t.Parallel()

	t.Run("path-shaped new side is rename-to-not-handle", func(t *testing.T) {
		t.Parallel()
		card := renameCard(1, "a", "internal/foo#OldThing", "internal/nested/dir")
		plan := &planparser.Plan{Format: 5, Approved: true, RenameMechanic: "mechanic", Cards: []planparser.Card{card}}
		findings := planparser.Validate(plan, t.TempDir())
		if got := countFor(findings, "rename-to-not-handle"); got != 1 {
			t.Errorf("countFor(findings, rename-to-not-handle) = %d; want 1", got)
		}
	})

	t.Run("path-shaped old side is rename-from-not-glyph", func(t *testing.T) {
		t.Parallel()
		card := renameCard(1, "a", "internal/nested/dir", "plan:internal/foo#NewThing")
		plan := &planparser.Plan{Format: 5, Approved: true, RenameMechanic: "mechanic", Cards: []planparser.Card{card}}
		findings := planparser.Validate(plan, t.TempDir())
		if got := countFor(findings, "rename-from-not-glyph"); got != 1 {
			t.Errorf("countFor(findings, rename-from-not-glyph) = %d; want 1", got)
		}
	})

	t.Run("symbol rename with a glyph old side and a handle new side passes", func(t *testing.T) {
		t.Parallel()
		card := renameCard(1, "a", "internal/foo#OldThing", "plan:internal/foo#NewThing")
		plan := &planparser.Plan{Format: 5, Approved: true, RenameMechanic: "mechanic", Cards: []planparser.Card{card}}
		findings := planparser.Validate(plan, t.TempDir())
		if got := countFor(findings, "rename-to-not-handle"); got != 0 {
			t.Errorf("countFor(findings, rename-to-not-handle) = %d; want 0", got)
		}
		if got := countFor(findings, "rename-from-not-glyph"); got != 0 {
			t.Errorf("countFor(findings, rename-from-not-glyph) = %d; want 0", got)
		}
	})

	t.Run("symbol rename whose new side is a glyph produces rename-to-not-handle", func(t *testing.T) {
		t.Parallel()
		card := renameCard(1, "a", "internal/foo#OldThing", "internal/foo#NewThing")
		plan := &planparser.Plan{Format: 5, Approved: true, RenameMechanic: "mechanic", Cards: []planparser.Card{card}}
		findings := planparser.Validate(plan, t.TempDir())
		if got := countFor(findings, "rename-to-not-handle"); got != 1 {
			t.Errorf("countFor(findings, rename-to-not-handle) = %d; want 1", got)
		}
	})

	t.Run("symbol rename whose old side is a bare symbol produces rename-from-not-glyph", func(t *testing.T) {
		t.Parallel()
		card := renameCard(1, "a", "package.OldThing", "plan:internal/foo#NewThing")
		plan := &planparser.Plan{Format: 5, Approved: true, RenameMechanic: "mechanic", Cards: []planparser.Card{card}}
		findings := planparser.Validate(plan, t.TempDir())
		if got := countFor(findings, "rename-from-not-glyph"); got != 1 {
			t.Errorf("countFor(findings, rename-from-not-glyph) = %d; want 1", got)
		}
	})

	t.Run("symbol rename whose old side is a plan: handle produces rename-from-not-glyph", func(t *testing.T) {
		t.Parallel()
		card := renameCard(1, "a", "plan:internal/foo#OldThing", "plan:internal/foo#NewThing")
		plan := &planparser.Plan{Format: 5, Approved: true, RenameMechanic: "mechanic", Cards: []planparser.Card{card}}
		findings := planparser.Validate(plan, t.TempDir())
		if got := countFor(findings, "rename-from-not-glyph"); got != 1 {
			t.Errorf("countFor(findings, rename-from-not-glyph) = %d; want 1", got)
		}
	})

	t.Run("self-glyph old side paired with a handle new side produces rename-from-not-glyph", func(t *testing.T) {
		// The one shape the two negation checks both let through (crucible round fable-high-r10,
		// F5): the old side IS a glyph and the new side IS a handle, but the old side names a
		// file/unit where a symbol rename must name the symbol being renamed.
		t.Parallel()
		card := renameCard(1, "a", "internal/foo/old.go#", "plan:internal/foo#NewThing")
		plan := &planparser.Plan{Format: 5, Approved: true, RenameMechanic: "mechanic", Cards: []planparser.Card{card}}
		findings := planparser.Validate(plan, t.TempDir())
		if got := countFor(findings, "rename-from-not-glyph"); got != 1 {
			t.Errorf("countFor(findings, rename-from-not-glyph) = %d; want 1", got)
		}
		if got := countFor(findings, "rename-to-not-handle"); got != 0 {
			t.Errorf("countFor(findings, rename-to-not-handle) = %d; want 0", got)
		}
	})

	t.Run("file self-glyph pair on both sides produces neither", func(t *testing.T) {
		t.Parallel()
		card := renameCard(1, "a", "internal/foo/old.go#", "internal/foo/new.go#")
		plan := &planparser.Plan{Format: 5, Approved: true, RenameMechanic: "mechanic", Cards: []planparser.Card{card}}
		findings := planparser.Validate(plan, t.TempDir())
		if got := countFor(findings, "rename-to-not-handle"); got != 0 {
			t.Errorf("countFor(findings, rename-to-not-handle) = %d; want 0", got)
		}
		if got := countFor(findings, "rename-from-not-glyph"); got != 0 {
			t.Errorf("countFor(findings, rename-from-not-glyph) = %d; want 0", got)
		}
	})

	t.Run("language: none produces neither", func(t *testing.T) {
		t.Parallel()
		card := renameCard(1, "a", "package.OldThing", "internal/foo#NewThing")
		plan := &planparser.Plan{Format: 5, Approved: true, Language: "none", RenameMechanic: "mechanic", Cards: []planparser.Card{card}}
		findings := planparser.Validate(plan, t.TempDir())
		if got := countFor(findings, "rename-to-not-handle"); got != 0 {
			t.Errorf("countFor(findings, rename-to-not-handle) = %d; want 0", got)
		}
		if got := countFor(findings, "rename-from-not-glyph"); got != 0 {
			t.Errorf("countFor(findings, rename-from-not-glyph) = %d; want 0", got)
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
		plan := &planparser.Plan{Format: 5, Approved: true, Cards: []planparser.Card{validCard(1, "a")}}
		findings := planparser.Validate(plan, t.TempDir())
		if got := countFor(findings, "rename-mechanic-missing"); got != 0 {
			t.Errorf("countFor(findings, rename-mechanic-missing) = %d; want 0", got)
		}
	})

	t.Run("Rename card, mechanic present", func(t *testing.T) {
		t.Parallel()
		card := cardOfType(1, "a", planparser.CardTypeRename, nil)
		plan := &planparser.Plan{Format: 5, Approved: true, RenameMechanic: "1. git mv old new first.", Cards: []planparser.Card{card}}
		findings := planparser.Validate(plan, t.TempDir())
		if got := countFor(findings, "rename-mechanic-missing"); got != 0 {
			t.Errorf("countFor(findings, rename-mechanic-missing) = %d; want 0", got)
		}
	})

	t.Run("Rename card, mechanic absent", func(t *testing.T) {
		t.Parallel()
		card := cardOfType(1, "a", planparser.CardTypeRename, nil)
		plan := &planparser.Plan{Format: 5, Approved: true, Cards: []planparser.Card{card}}
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
		plan := &planparser.Plan{Format: 5, Approved: true, Cards: []planparser.Card{card}}
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
		plan := &planparser.Plan{Format: 5, Approved: true, Cards: []planparser.Card{validCard(1, "a")}}
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
		plan := &planparser.Plan{Format: 5, Approved: true, Cards: []planparser.Card{card}}
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
		plan := &planparser.Plan{Format: 5, Approved: true, Cards: []planparser.Card{card}}
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
		plan := &planparser.Plan{Format: 5, Approved: true, Cards: []planparser.Card{card}}
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
			plan := &planparser.Plan{Format: 5, Approved: true, Cards: []planparser.Card{card}}
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
		plan := &planparser.Plan{Format: 5, Approved: true, Cards: []planparser.Card{card}}
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
		plan := &planparser.Plan{Format: 5, Approved: true, Cards: []planparser.Card{card}}
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
		plan := &planparser.Plan{Format: 5, Approved: true, Cards: []planparser.Card{card}}
		findings := planparser.Validate(plan, t.TempDir())
		if got := countFor(findings, "card-field-empty"); got != 0 {
			t.Errorf("countFor(findings, card-field-empty) = %d; want 0", got)
		}
	})

	t.Run("type label present with zero Targets", func(t *testing.T) {
		t.Parallel()
		card := cardOfType(1, "a", planparser.CardTypeEdit, []string{})
		card.Uses = []string{"pkg/dep.go"}
		plan := &planparser.Plan{Format: 5, Approved: true, Cards: []planparser.Card{card}}
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
		plan := &planparser.Plan{Format: 5, Approved: true, Cards: []planparser.Card{card}}
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
		plan := &planparser.Plan{Format: 5, Approved: true, Cards: []planparser.Card{card}}
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
		plan := &planparser.Plan{Format: 5, Approved: true, Cards: []planparser.Card{card}}
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
		plan := &planparser.Plan{Format: 5, Approved: true, Cards: []planparser.Card{card}}
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
		plan := &planparser.Plan{Format: 5, Approved: true, Cards: []planparser.Card{validCard(1, "a")}}
		findings := planparser.Validate(plan, t.TempDir())
		if got := countFor(findings, "card-field-overlap"); got != 0 {
			t.Errorf("countFor(findings, card-field-overlap) = %d; want 0", got)
		}
	})

	t.Run("entry in both Targets and Uses", func(t *testing.T) {
		t.Parallel()
		card := validCard(1, "a")
		card.Uses = []string{card.Targets[0]}
		plan := &planparser.Plan{Format: 5, Approved: true, Cards: []planparser.Card{card}}
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
		plan := &planparser.Plan{Format: 5, Approved: true, Cards: []planparser.Card{validCard(1, "a")}}
		findings := planparser.Validate(plan, t.TempDir())
		if got := countFor(findings, "impact-summary-multiline"); got != 0 {
			t.Errorf("countFor(findings, impact-summary-multiline) = %d; want 0", got)
		}
	})

	t.Run("trailing lines", func(t *testing.T) {
		t.Parallel()
		card := validCard(1, "a")
		card.ImpactSummaryTrailing = []string{"an unwanted second line"}
		plan := &planparser.Plan{Format: 5, Approved: true, Cards: []planparser.Card{card}}
		findings := planparser.Validate(plan, t.TempDir())
		if got := countFor(findings, "impact-summary-multiline"); got != 1 {
			t.Errorf("countFor(findings, impact-summary-multiline) = %d; want 1", got)
		}
	})
}

// TestValidate_ProsaSymbolTarget covers prosa-symbol-target: under the default glyph-enabled
// plan.Language, a Prosa card's target list must hold only a self glyph (file or unit), never a
// member glyph or anything that fails to parse as a glyph at all; under "none" it keeps its
// pre-glyph path-vs-symbol behavior exactly.
func TestValidate_ProsaSymbolTarget(t *testing.T) {
	t.Parallel()

	t.Run("clean (file self glyph)", func(t *testing.T) {
		t.Parallel()
		card := cardOfType(1, "a", planparser.CardTypeProsa, []string{"doc.go#"})
		plan := &planparser.Plan{Format: 5, Approved: true, Cards: []planparser.Card{card}}
		findings := planparser.Validate(plan, t.TempDir())
		if got := countFor(findings, "prosa-symbol-target"); got != 0 {
			t.Errorf("countFor(findings, prosa-symbol-target) = %d; want 0", got)
		}
	})

	t.Run("clean (unit self glyph)", func(t *testing.T) {
		t.Parallel()
		card := cardOfType(1, "a", planparser.CardTypeProsa, []string{"internal/foo#"})
		plan := &planparser.Plan{Format: 5, Approved: true, Cards: []planparser.Card{card}}
		findings := planparser.Validate(plan, t.TempDir())
		if got := countFor(findings, "prosa-symbol-target"); got != 0 {
			t.Errorf("countFor(findings, prosa-symbol-target) = %d; want 0", got)
		}
	})

	t.Run("member glyph on a Prosa card is the finding", func(t *testing.T) {
		t.Parallel()
		card := cardOfType(1, "a", planparser.CardTypeProsa, []string{"internal/foo#Bar"})
		plan := &planparser.Plan{Format: 5, Approved: true, Cards: []planparser.Card{card}}
		findings := planparser.Validate(plan, t.TempDir())
		if got := countFor(findings, "prosa-symbol-target"); got != 1 {
			t.Errorf("countFor(findings, prosa-symbol-target) = %d; want 1", got)
		}
	})

	t.Run("bare symbol on a Prosa card is the finding", func(t *testing.T) {
		t.Parallel()
		card := cardOfType(1, "a", planparser.CardTypeProsa, []string{"pkg.Symbol"})
		plan := &planparser.Plan{Format: 5, Approved: true, Cards: []planparser.Card{card}}
		findings := planparser.Validate(plan, t.TempDir())
		if got := countFor(findings, "prosa-symbol-target"); got != 1 {
			t.Errorf("countFor(findings, prosa-symbol-target) = %d; want 1", got)
		}
	})

	t.Run("Edit group symbol plus Prosa group member glyph: only the Prosa group's is flagged", func(t *testing.T) {
		t.Parallel()
		card := validCard(1, "a")
		card.Type = planparser.CardTypeEdit
		card.Targets = []string{"pkg.EditSymbol", "internal/foo#ProsaMember"}
		card.TargetGroups = []planparser.TargetGroup{
			{Type: planparser.CardTypeEdit, Refs: []string{"pkg.EditSymbol"}},
			{Type: planparser.CardTypeProsa, Refs: []string{"internal/foo#ProsaMember"}},
		}
		plan := &planparser.Plan{Format: 5, Approved: true, Cards: []planparser.Card{card}}
		findings := planparser.Validate(plan, t.TempDir())
		if got := countFor(findings, "prosa-symbol-target"); got != 1 {
			t.Errorf("countFor(findings, prosa-symbol-target) = %d; want 1", got)
		}
	})

	t.Run("symbol lives only in the Edit group: no finding", func(t *testing.T) {
		t.Parallel()
		card := cardOfType(1, "a", planparser.CardTypeEdit, []string{"pkg.EditSymbol"})
		plan := &planparser.Plan{Format: 5, Approved: true, Cards: []planparser.Card{card}}
		findings := planparser.Validate(plan, t.TempDir())
		if got := countFor(findings, "prosa-symbol-target"); got != 0 {
			t.Errorf("countFor(findings, prosa-symbol-target) = %d; want 0", got)
		}
	})

	t.Run("language none: path-only Prosa is clean", func(t *testing.T) {
		t.Parallel()
		card := cardOfType(1, "a", planparser.CardTypeProsa, []string{"doc.go"})
		plan := &planparser.Plan{Format: 5, Approved: true, Language: "none", Cards: []planparser.Card{card}}
		findings := planparser.Validate(plan, t.TempDir())
		if got := countFor(findings, "prosa-symbol-target"); got != 0 {
			t.Errorf("countFor(findings, prosa-symbol-target) = %d; want 0", got)
		}
	})

	t.Run("language none: symbol target on a Prosa card is still the finding", func(t *testing.T) {
		t.Parallel()
		card := cardOfType(1, "a", planparser.CardTypeProsa, []string{"pkg.Symbol"})
		plan := &planparser.Plan{Format: 5, Approved: true, Language: "none", Cards: []planparser.Card{card}}
		findings := planparser.Validate(plan, t.TempDir())
		if got := countFor(findings, "prosa-symbol-target"); got != 1 {
			t.Errorf("countFor(findings, prosa-symbol-target) = %d; want 1", got)
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
				plan := &planparser.Plan{Format: 5, Approved: true, Cards: []planparser.Card{card}}
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
		plan := &planparser.Plan{Format: 5, Approved: true, Cards: []planparser.Card{card}}
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
		plan := &planparser.Plan{Format: 5, Approved: true, RenameMechanic: "mechanic", Cards: []planparser.Card{card}}
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
		plan := &planparser.Plan{Format: 5, Approved: true, RenameMechanic: "mechanic", Cards: []planparser.Card{card}}
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
		plan := &planparser.Plan{Format: 5, Approved: true, Cards: []planparser.Card{card}}
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
		plan := &planparser.Plan{Format: 5, Approved: true, Cards: []planparser.Card{card}}
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
		plan := &planparser.Plan{Format: 5, Approved: true, Cards: []planparser.Card{card}}
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
			Format: 5, Approved: true, RenameMechanic: "mechanic",
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
			Format: 5, Approved: true,
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
		plan := &planparser.Plan{Format: 5, Approved: true, Cards: []planparser.Card{validCard(1, "a")}}
		findings := planparser.Validate(plan, t.TempDir())
		if got := countFor(findings, "commit-subject-mismatch"); got != 0 {
			t.Errorf("countFor(findings, commit-subject-mismatch) = %d; want 0", got)
		}
	})

	t.Run("wrong prefix", func(t *testing.T) {
		t.Parallel()
		card := validCard(1, "a")
		card.Commit = "2: wrong prefix"
		plan := &planparser.Plan{Format: 5, Approved: true, Cards: []planparser.Card{card}}
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

	plan := &planparser.Plan{Format: 5, Approved: true, Cards: []planparser.Card{card}}
	findings := planparser.Validate(plan, t.TempDir())

	for _, check := range []string{"card-path-malformed", "card-missing-field", "card-field-overlap", "commit-subject-mismatch"} {
		if got := countFor(findings, check); got != 1 {
			t.Errorf("countFor(findings, %q) = %d; want 1", check, got)
		}
	}
}

// TestValidate_LanguageRecognized covers plan-language-unrecognized: "go" and "none" are accepted,
// as is "" (the zero value, meaning absent — matching Plan.Language's own documented "absent
// defaults to go" rule), and any other value is exactly one finding.
func TestValidate_LanguageRecognized(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		language string
		want     int
	}{
		{name: "go accepted", language: "go", want: 0},
		{name: "none accepted", language: "none", want: 0},
		{name: "absent defaulting to go", language: "", want: 0},
		{name: "unknown value", language: "python", want: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			plan := &planparser.Plan{Format: 5, Approved: true, Language: tt.language, Cards: []planparser.Card{validCard(1, "a")}}
			findings := planparser.Validate(plan, t.TempDir())
			if got := countFor(findings, "plan-language-unrecognized"); got != tt.want {
				t.Errorf("countFor(findings, plan-language-unrecognized) = %d; want %d", got, tt.want)
			}
		})
	}
}

// TestValidate_BareSymbolTarget covers bare-symbol-target: any Targets/Uses entry classifying as
// a bare package-qualified symbol is a hard finding under a glyph-enabled plan.Language, and is
// skipped entirely under "none".
func TestValidate_BareSymbolTarget(t *testing.T) {
	t.Parallel()

	t.Run("clean (glyph target)", func(t *testing.T) {
		t.Parallel()
		card := cardOfType(1, "a", planparser.CardTypeEdit, []string{"internal/foo#Bar"})
		plan := &planparser.Plan{Format: 5, Approved: true, Cards: []planparser.Card{card}}
		findings := planparser.Validate(plan, t.TempDir())
		if got := countFor(findings, "bare-symbol-target"); got != 0 {
			t.Errorf("countFor(findings, bare-symbol-target) = %d; want 0", got)
		}
	})

	t.Run("bare symbol target", func(t *testing.T) {
		t.Parallel()
		card := cardOfType(1, "a", planparser.CardTypeEdit, []string{"pkg.Symbol"})
		plan := &planparser.Plan{Format: 5, Approved: true, Cards: []planparser.Card{card}}
		findings := planparser.Validate(plan, t.TempDir())
		if got := countFor(findings, "bare-symbol-target"); got != 1 {
			t.Errorf("countFor(findings, bare-symbol-target) = %d; want 1", got)
		}
	})

	t.Run("bare symbol in Uses", func(t *testing.T) {
		t.Parallel()
		card := validCard(1, "a")
		card.HasUses = true
		card.Uses = []string{"pkg.Symbol"}
		plan := &planparser.Plan{Format: 5, Approved: true, Cards: []planparser.Card{card}}
		findings := planparser.Validate(plan, t.TempDir())
		if got := countFor(findings, "bare-symbol-target"); got != 1 {
			t.Errorf("countFor(findings, bare-symbol-target) = %d; want 1", got)
		}
	})

	t.Run("language none skips the check entirely", func(t *testing.T) {
		t.Parallel()
		card := cardOfType(1, "a", planparser.CardTypeEdit, []string{"pkg.Symbol"})
		plan := &planparser.Plan{Format: 5, Approved: true, Language: "none", Cards: []planparser.Card{card}}
		findings := planparser.Validate(plan, t.TempDir())
		if got := countFor(findings, "bare-symbol-target"); got != 0 {
			t.Errorf("countFor(findings, bare-symbol-target) = %d; want 0", got)
		}
	})
}

// TestValidate_DirectoryTarget covers directory-target: a path-shaped entry with a "/" and no
// file extension names a directory rather than a file, and is skipped entirely under "none". A
// slash-free extensionless entry (e.g. "Makefile") is out of scope for this check by design.
func TestValidate_DirectoryTarget(t *testing.T) {
	t.Parallel()

	t.Run("clean (file with extension)", func(t *testing.T) {
		t.Parallel()
		card := cardOfType(1, "a", planparser.CardTypeEdit, []string{"internal/foo/list.go"})
		plan := &planparser.Plan{Format: 5, Approved: true, Cards: []planparser.Card{card}}
		findings := planparser.Validate(plan, t.TempDir())
		if got := countFor(findings, "directory-target"); got != 0 {
			t.Errorf("countFor(findings, directory-target) = %d; want 0", got)
		}
	})

	t.Run("directory target", func(t *testing.T) {
		t.Parallel()
		card := cardOfType(1, "a", planparser.CardTypeEdit, []string{"internal/foo"})
		plan := &planparser.Plan{Format: 5, Approved: true, Cards: []planparser.Card{card}}
		findings := planparser.Validate(plan, t.TempDir())
		if got := countFor(findings, "directory-target"); got != 1 {
			t.Errorf("countFor(findings, directory-target) = %d; want 1", got)
		}
	})

	t.Run("slash-free extensionless filename is out of scope", func(t *testing.T) {
		t.Parallel()
		card := cardOfType(1, "a", planparser.CardTypeEdit, []string{"Makefile"})
		plan := &planparser.Plan{Format: 5, Approved: true, Cards: []planparser.Card{card}}
		findings := planparser.Validate(plan, t.TempDir())
		if got := countFor(findings, "directory-target"); got != 0 {
			t.Errorf("countFor(findings, directory-target) = %d; want 0 (slash-free extensionless names are not this check's business)", got)
		}
	})

	t.Run("detail names the file self glyph remedy for the root:-scoped extensionless-file case", func(t *testing.T) {
		// A bare extensionless filename under a non-"." root: joins to a slashed extensionless path
		// no lexical rule can tell from a directory, so it lands here — the detail must offer the
		// file-self-glyph spelling too, not only the package remedy (crucible round fable-high-r10,
		// F7).
		t.Parallel()
		card := cardOfType(1, "a", planparser.CardTypeEdit, []string{"internal/foo/Makefile"})
		plan := &planparser.Plan{Format: 5, Approved: true, Cards: []planparser.Card{card}}
		findings := planparser.Validate(plan, t.TempDir())
		var detail string
		for _, f := range findings {
			if f.Check == "directory-target" {
				detail = f.Detail
			}
		}
		if detail == "" {
			t.Fatalf("Validate() produced no directory-target finding for %q", "internal/foo/Makefile")
		}
		if !strings.Contains(detail, `"internal/foo/Makefile#"`) || !strings.Contains(detail, "file self glyph") {
			t.Errorf("directory-target detail = %q; want it to name the file self glyph remedy %q", detail, "internal/foo/Makefile#")
		}
	})

	t.Run("language none skips the check entirely", func(t *testing.T) {
		t.Parallel()
		card := cardOfType(1, "a", planparser.CardTypeEdit, []string{"internal/foo"})
		plan := &planparser.Plan{Format: 5, Approved: true, Language: "none", Cards: []planparser.Card{card}}
		findings := planparser.Validate(plan, t.TempDir())
		if got := countFor(findings, "directory-target"); got != 0 {
			t.Errorf("countFor(findings, directory-target) = %d; want 0", got)
		}
	})
}

// TestValidate_GlyphMalformed covers glyph-malformed (crucible round sonnet-xhigh-r8, PG-1): a
// "#"-containing entry that fails glyph.Parse is a hard finding, card-generic over Targets/Uses,
// and skipped entirely under "none" -- the exact shape bare-symbol-target and directory-target
// already follow. Before this check existed, every one of the malformed cases below validated
// completely clean.
func TestValidate_GlyphMalformed(t *testing.T) {
	t.Parallel()

	t.Run("clean (well-formed glyph target)", func(t *testing.T) {
		t.Parallel()
		card := cardOfType(1, "a", planparser.CardTypeEdit, []string{"internal/foo#Bar"})
		plan := &planparser.Plan{Format: 5, Approved: true, Cards: []planparser.Card{card}}
		findings := planparser.Validate(plan, t.TempDir())
		if got := countFor(findings, "glyph-malformed"); got != 0 {
			t.Errorf("countFor(findings, glyph-malformed) = %d; want 0", got)
		}
	})

	t.Run("doubled hash is malformed", func(t *testing.T) {
		t.Parallel()
		card := cardOfType(1, "a", planparser.CardTypeEdit, []string{"internal/foo#Bar#extra"})
		plan := &planparser.Plan{Format: 5, Approved: true, Cards: []planparser.Card{card}}
		findings := planparser.Validate(plan, t.TempDir())
		if got := countFor(findings, "glyph-malformed"); got != 1 {
			t.Errorf("countFor(findings, glyph-malformed) = %d; want 1", got)
		}
		// None of the checks a malformed-but-"#"-shaped entry used to sail past silently through
		// fire either -- this is PG-1's whole point: before this check existed nothing flagged it.
		for _, check := range []string{"bare-symbol-target", "directory-target", "card-path-malformed", "path-missing"} {
			if got := countFor(findings, check); got != 0 {
				t.Errorf("countFor(findings, %s) = %d; want 0 (wrong shape for this check, not evidence the entry is fine)", check, got)
			}
		}
	})

	t.Run("malformed glyph in Uses", func(t *testing.T) {
		t.Parallel()
		card := validCard(1, "a")
		card.HasUses = true
		card.Uses = []string{"internal/foo#Bar#extra"}
		plan := &planparser.Plan{Format: 5, Approved: true, Cards: []planparser.Card{card}}
		findings := planparser.Validate(plan, t.TempDir())
		if got := countFor(findings, "glyph-malformed"); got != 1 {
			t.Errorf("countFor(findings, glyph-malformed) = %d; want 1", got)
		}
	})

	t.Run("malformed glyph in a Prosa group is card-generic, not group-scoped", func(t *testing.T) {
		t.Parallel()
		// This deliberately does NOT exempt Prosa: bare-symbol-target/directory-target don't either,
		// and prosa-symbol-target already separately flags the same entry as "not a self glyph" --
		// two checks naming the same defect from two angles, exactly as they already do for a
		// Prosa group's bare-symbol target.
		card := cardOfType(1, "a", planparser.CardTypeProsa, []string{"internal/foo#Bar#extra"})
		plan := &planparser.Plan{Format: 5, Approved: true, Cards: []planparser.Card{card}}
		findings := planparser.Validate(plan, t.TempDir())
		if got := countFor(findings, "glyph-malformed"); got != 1 {
			t.Errorf("countFor(findings, glyph-malformed) = %d; want 1", got)
		}
		if got := countFor(findings, "prosa-symbol-target"); got != 1 {
			t.Errorf("countFor(findings, prosa-symbol-target) = %d; want 1", got)
		}
	})

	t.Run("language none skips the check entirely", func(t *testing.T) {
		t.Parallel()
		card := cardOfType(1, "a", planparser.CardTypeEdit, []string{"internal/foo#Bar#extra"})
		plan := &planparser.Plan{Format: 5, Approved: true, Language: "none", Cards: []planparser.Card{card}}
		findings := planparser.Validate(plan, t.TempDir())
		if got := countFor(findings, "glyph-malformed"); got != 0 {
			t.Errorf("countFor(findings, glyph-malformed) = %d; want 0", got)
		}
	})
}

// TestValidate_PathMissing_Glyphs covers checkPathMissing's card-6 rework over glyphs: a file self
// glyph whose file exists passes, one whose file does not exist and is not a Create target fails,
// a member glyph resolving to its unit's directory is skipped rather than reported, a unit self
// glyph for a package that exists passes, and the "none"-language path is byte-for-byte its
// pre-glyph behavior.
func TestValidate_PathMissing_Glyphs(t *testing.T) {
	t.Parallel()

	t.Run("file self glyph, file exists: passes", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		materializeFiles(t, root, "internal/foo/list.go")
		card := cardOfType(1, "a", planparser.CardTypeEdit, []string{"internal/foo/list.go#"})
		plan := &planparser.Plan{Format: 5, Approved: true, Cards: []planparser.Card{card}}
		findings := planparser.Validate(plan, root)
		if got := countFor(findings, "path-missing"); got != 0 {
			t.Errorf("countFor(findings, path-missing) = %d; want 0", got)
		}
	})

	t.Run("file self glyph, file absent and not a Create target: fails", func(t *testing.T) {
		t.Parallel()
		card := cardOfType(1, "a", planparser.CardTypeEdit, []string{"internal/foo/missing.go#"})
		plan := &planparser.Plan{Format: 5, Approved: true, Cards: []planparser.Card{card}}
		findings := planparser.Validate(plan, t.TempDir())
		if got := countFor(findings, "path-missing"); got != 1 {
			t.Errorf("countFor(findings, path-missing) = %d; want 1", got)
		}
	})

	t.Run("member glyph: skipped rather than reported, even when its unit directory is absent", func(t *testing.T) {
		t.Parallel()
		card := cardOfType(1, "a", planparser.CardTypeEdit, []string{"internal/nonexistent#Bar"})
		plan := &planparser.Plan{Format: 5, Approved: true, Cards: []planparser.Card{card}}
		findings := planparser.Validate(plan, t.TempDir())
		if got := countFor(findings, "path-missing"); got != 0 {
			t.Errorf("countFor(findings, path-missing) = %d; want 0 (member glyphs are skipped, not resolved, by this package)", got)
		}
	})

	t.Run("unit self glyph, package exists: passes", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		materializeFiles(t, root, "internal/foo/list.go")
		card := cardOfType(1, "a", planparser.CardTypeEdit, []string{"internal/foo#"})
		plan := &planparser.Plan{Format: 5, Approved: true, Cards: []planparser.Card{card}}
		findings := planparser.Validate(plan, root)
		if got := countFor(findings, "path-missing"); got != 0 {
			t.Errorf("countFor(findings, path-missing) = %d; want 0", got)
		}
	})

	t.Run("language none: glyph-shaped entry is skipped, matching pre-glyph isPathRef-only behavior", func(t *testing.T) {
		t.Parallel()
		card := cardOfType(1, "a", planparser.CardTypeEdit, []string{"internal/foo/missing.go#"})
		plan := &planparser.Plan{Format: 5, Approved: true, Language: "none", Cards: []planparser.Card{card}}
		findings := planparser.Validate(plan, t.TempDir())
		if got := countFor(findings, "path-missing"); got != 0 {
			t.Errorf("countFor(findings, path-missing) = %d; want 0", got)
		}
	})

	t.Run("glyph Create target in one card satisfies a glyph Uses reference in another", func(t *testing.T) {
		t.Parallel()
		create := cardOfType(1, "create", planparser.CardTypeCreate, []string{"internal/foo/new.go#"})
		usesCreateTarget := cardOfType(2, "uses-create-target", planparser.CardTypeCustom, []string{"pkg/card2.go"})
		usesCreateTarget.HasUses = true
		usesCreateTarget.Uses = []string{"internal/foo/new.go#"}
		plan := &planparser.Plan{Format: 5, Approved: true, Cards: []planparser.Card{create, usesCreateTarget}}
		findings := planparser.Validate(plan, t.TempDir())
		if got := countFor(findings, "path-missing"); got != 0 {
			t.Errorf("countFor(findings, path-missing) = %d; want 0", got)
		}
	})
}

// TestValidate_CardPathMalformed_Glyphs covers checkCardPathMalformed's card-6 rework: a
// malformed disk path reached through a self glyph is reported exactly as a malformed plain path
// would be, and a member glyph is skipped.
func TestValidate_CardPathMalformed_Glyphs(t *testing.T) {
	t.Parallel()

	t.Run("member glyph is skipped even though its unit half is well-formed", func(t *testing.T) {
		t.Parallel()
		card := cardOfType(1, "a", planparser.CardTypeEdit, []string{"internal/foo#Bar"})
		plan := &planparser.Plan{Format: 5, Approved: true, Cards: []planparser.Card{card}}
		findings := planparser.Validate(plan, t.TempDir())
		if got := countFor(findings, "card-path-malformed"); got != 0 {
			t.Errorf("countFor(findings, card-path-malformed) = %d; want 0", got)
		}
	})

	t.Run("clean file self glyph", func(t *testing.T) {
		t.Parallel()
		card := cardOfType(1, "a", planparser.CardTypeEdit, []string{"internal/foo/list.go#"})
		plan := &planparser.Plan{Format: 5, Approved: true, Cards: []planparser.Card{card}}
		findings := planparser.Validate(plan, t.TempDir())
		if got := countFor(findings, "card-path-malformed"); got != 0 {
			t.Errorf("countFor(findings, card-path-malformed) = %d; want 0", got)
		}
	})
}

// TestValidate_RootFilenameCanonicalizesEndToEnd is R9-1's regression: a plan spelling a
// repository-root extensionless filename — classifyRef rule 4's own case, the rule that exists
// precisely so such a filename HAS a legal spelling — must reach the validator (and, past it,
// every glyph-backed layer in internal/planglyph) as a self glyph, not as a bare token quarry
// rejects before resolution.
//
// Left uncanonicalized, "LICENSE" validated 100% clean here while producing a false
// prosa-symbol-target on a Prosa group, and then made the batch that created it permanently
// unrecordable: DoneChecks handed quarry the bare token, quarry answered a pre-resolution
// rejection, and doneCheckVerdicts read the rejection as "did not resolve" — a blocking
// create-not-done against a card that had done its job.
func TestValidate_RootFilenameCanonicalizesEndToEnd(t *testing.T) {
	t.Parallel()

	dir := writePlanFiles(t, map[string]string{
		"00-overview.md": `---
format: 5
approved: true
---

# Plan: root filenames

Framing paragraph.

## Card Index

1 — prose — document the licence
2 — build — add a makefile
`,
		"01-prose.md": "# Card 1 — prose\n\n" +
			"**Prosa:**\n- `LICENSE`\n" +
			"**Intent:** rewrite the licence header.\n",
		"02-build.md": "# Card 2 — build\n\n" +
			"**Create:**\n- `Makefile`\n" +
			"**Intent:** add a makefile.\n",
	})

	plan, err := planparser.ParsePlan(dir)
	if err != nil {
		t.Fatalf("ParsePlan(%q) error = %v; want nil", dir, err)
	}

	if got, want := plan.Cards[0].Targets[0], "LICENSE#"; got != want {
		t.Errorf("Prosa target = %q; want %q", got, want)
	}
	if got, want := plan.Cards[1].Targets[0], "Makefile#"; got != want {
		t.Errorf("Create target = %q; want %q", got, want)
	}

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "LICENSE"), []byte("licence text\n"), 0o644); err != nil {
		t.Fatalf("write LICENSE fixture: %v", err)
	}

	findings := planparser.Validate(plan, root)
	if got := countFor(findings, "prosa-symbol-target"); got != 0 {
		t.Errorf("countFor(findings, prosa-symbol-target) = %d; want 0 (LICENSE is a file self glyph)", got)
	}
	if len(findings) != 0 {
		t.Errorf("findings = %v; want none", findings)
	}
}

// TestCardID asserts Card.ID renders the same "N-<slug>" identity Validate's own findings key
// their Card field on. This file is package planparser_test, an external test package with no
// access to validate.go's unexported cardID, so the equivalence is expressed against the same
// "N-<slug>" format cardID itself produces, rather than by calling it directly.
func TestCardID(t *testing.T) {
	t.Parallel()

	c := planparser.Card{Number: 3, Slug: "exported-surface"}
	if got, want := c.ID(), "3-exported-surface"; got != want {
		t.Errorf("Card.ID() = %q; want %q", got, want)
	}
}
