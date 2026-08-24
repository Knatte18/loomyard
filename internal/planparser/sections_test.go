// sections_test.go covers the three plan-level body sections extracted from 00-overview.md: they
// are exposed verbatim from the format-4 golden fixture,
// and each is empty when its heading is absent from the overview.

package planparser_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/planparser"
)

func TestParsePlan_GoldenFixture_PlanLevelSections(t *testing.T) {
	t.Parallel()

	plan, err := planparser.ParsePlan(goodPlanDir())
	if err != nil {
		t.Fatalf("ParsePlan(%q) error = %v; want nil", goodPlanDir(), err)
	}

	wantSharedDecisions := "### Decision: json-envelope-reuse\n\n" +
		"- **Decision:** `--json` marshals each row through the existing `internal/output.Ok` envelope —\n" +
		"  no new envelope type is introduced.\n" +
		"- **Rationale:** one JSON emission path for the whole CLI; a second envelope shape would fork\n" +
		"  behavior for no gain.\n" +
		"- **Applies to:** all cards"
	if plan.SharedDecisions != wantSharedDecisions {
		t.Errorf("plan.SharedDecisions = %q; want %q", plan.SharedDecisions, wantSharedDecisions)
	}

	wantRenameMechanic := "1. Run `git mv <old> <new>` FIRST, before any other change to the moved file.\n" +
		"2. Then make ONLY surgical edits (package declaration, imports, identifier\n" +
		"   retargeting) — no unrelated rewrites.\n" +
		"3. A genuinely new file with no predecessor belongs in a separate `Create` card, never folded\n" +
		"   into the `Rename` pair.\n" +
		"4. Never write the relocated file from scratch and delete the original — that loses\n" +
		"   git history exactly as an unstructured create+delete pair would."
	if plan.RenameMechanic != wantRenameMechanic {
		t.Errorf("plan.RenameMechanic = %q; want %q", plan.RenameMechanic, wantRenameMechanic)
	}

	wantVerify := "go test ./internal/boardcli/... ./internal/boardengine/... ./cmd/lyx/..."
	if plan.Verify != wantVerify {
		t.Errorf("plan.Verify = %q; want %q", plan.Verify, wantVerify)
	}
}

// TestParsePlan_PlanLevelSections_AbsentAreEmpty proves all three plan-level sections default to ""
// when their headings are absent.
func TestParsePlan_PlanLevelSections_AbsentAreEmpty(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	overview := "---\nformat: 4\napproved: true\n---\n\n# Plan\n\nFraming.\n\n## Card Index\n\n1 — only — a card\n"
	card := "# Card 1 — only\n\n**Edit:**\n- `a.go`\n**Intent:** placeholder.\n"
	if err := os.WriteFile(filepath.Join(dir, "00-overview.md"), []byte(overview), 0o644); err != nil {
		t.Fatalf("write overview fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "01-only.md"), []byte(card), 0o644); err != nil {
		t.Fatalf("write card fixture: %v", err)
	}

	plan, err := planparser.ParsePlan(dir)
	if err != nil {
		t.Fatalf("ParsePlan(%q) error = %v; want nil", dir, err)
	}
	if plan.SharedDecisions != "" {
		t.Errorf("plan.SharedDecisions = %q; want empty (section absent)", plan.SharedDecisions)
	}
	if plan.RenameMechanic != "" {
		t.Errorf("plan.RenameMechanic = %q; want empty (section absent)", plan.RenameMechanic)
	}
	if plan.Verify != "" {
		t.Errorf("plan.Verify = %q; want empty (section absent)", plan.Verify)
	}
}
