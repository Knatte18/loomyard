// fixture_test.go carries the package-level test fixture helpers built for the two removed validate
// producers' own test files (batch 3) that gates_test.go and cancellation_test.go still depend on --
// validDecisionRecord and writeDiscussionFixture from discussionvalidate_test.go, and
// seedPlanValidateFixture (renamed to seedPlanFormatFixture), seedFormatInvalidPlanValidateFixture
// (renamed to seedFormatInvalidPlanFixture), writeGlyphRepoFixture, and seedGlyphPlanFixture, all
// from planvalidate_test.go. Both source files are deleted by the same commit that adds this one;
// this file is what keeps their surviving callers compiling.

package loomshed

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/lyxdirs"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/testkit/plankit"
)

// validDecisionRecord carries all seven required sections, in order, plus the optional eighth. It
// stays a hardcoded literal here rather than an export of internal/discussionparser: no caller
// outside that package needs the list itself now that the section-iteration case has moved there.
const validDecisionRecord = `# Decision record

## Goal

Goal text.

## Scope

Scope text.

## Decisions

Decisions text.

## Constraints

Constraints text.

## Auto-mode assumptions

Assumptions text.

## Open risks

Risks text.

## Acceptance criteria

Criteria text.

## Notes for the plan writer

Notes text.
`

func writeDiscussionFixture(t *testing.T, dir, decisionRecord, supportLog string) (decisionRecordPath, supportLogPath string) {
	t.Helper()
	decisionRecordPath = filepath.Join(dir, "decision-record.md")
	supportLogPath = filepath.Join(dir, "support-log.md")
	if decisionRecord != "" {
		if err := os.WriteFile(decisionRecordPath, []byte(decisionRecord), 0o644); err != nil {
			t.Fatalf("write decision record: %v", err)
		}
	}
	if supportLog != "" {
		if err := os.WriteFile(supportLogPath, []byte(supportLog), 0o644); err != nil {
			t.Fatalf("write support log: %v", err)
		}
	}
	return decisionRecordPath, supportLogPath
}

// seedPlanFormatFixture writes a syntactically complete, one-card plan-format plan under
// <anchorPath>/_lyx/plan/, approved or not per approved. The sole card carries a Create group so
// path-missing never fires regardless of worktreeRoot's contents — a Create group's targets stay
// exempt from on-disk existence checking.
func seedPlanFormatFixture(t *testing.T, anchorPath string, approved bool) {
	t.Helper()
	plankit.Write(t, filepath.Join(anchorPath, lyxdirs.LyxDirName, "plan"), firstCardPlan(approved, "none", "internal/firstcard/new.go", ""))
}

// firstCardPlan describes a one-card plan whose sole Create group targets createTarget and, when useTarget is non-empty, whose card also uses useTarget.
func firstCardPlan(approved bool, language, createTarget, useTarget string) plankit.Plan {
	card := plankit.Card{
		Number:  1,
		Slug:    "first-card",
		Summary: "placeholder card 1",
		Groups:  []plankit.Group{{Label: "Create", Targets: []string{createTarget}}},
		Intent:  "placeholder card.",
	}
	if useTarget != "" {
		card.Uses = []string{useTarget}
	}
	return plankit.Plan{Approved: approved, Language: language, Framing: "Framing.", Cards: []plankit.Card{card}}
}

// seedPlanContextFixture writes a one-card plan under anchorPath whose card renames by renamePair, a complete "`old` -> `new`" line, or edits a file when renamePair is empty.
// planSection is the body of the overview's `## PATTERN changes` section, omitted when empty.
// It writes a decision record next to the plan holding recordSection as its `## PATTERN changes` body, a record without the section when recordSection is empty, and no record when it is the literal "absent".
// The returned PlanContext names the record path and lists no rename tool.
func seedPlanContextFixture(t *testing.T, anchorPath, renamePair, planSection, recordSection string) PlanContext {
	t.Helper()
	plan := firstCardPlan(true, "none", "internal/firstcard/new.go", "")
	if renamePair != "" {
		plan.Cards[0].Groups = []plankit.Group{{Label: "Rename", Targets: []string{renamePair}}}
		plan.Sections = append(plan.Sections, plankit.Section{Heading: "Rename mechanic", Body: "1. Run the rename tool."})
	}
	if planSection != "" {
		plan.Sections = append(plan.Sections, plankit.Section{Heading: "PATTERN changes", Body: planSection})
	}
	plankit.Write(t, filepath.Join(anchorPath, lyxdirs.LyxDirName, "plan"), plan)

	recordPath := filepath.Join(anchorPath, "decision-record.md")
	if recordSection != "absent" {
		record := validDecisionRecord
		if recordSection != "" {
			record += "\n## PATTERN changes\n\n" + recordSection + "\n"
		}
		if err := os.WriteFile(recordPath, []byte(record), 0o644); err != nil {
			t.Fatalf("write decision record: %v", err)
		}
	}
	return PlanContext{DecisionRecordPath: recordPath}
}

// seedFormatInvalidPlanFixture writes a plan whose overview declares an unrecognized format,
// tripping format-unrecognized regardless of requireApproved — the mode dimension must not change a
// format-invalid plan's Stuck disposition either way.
func seedFormatInvalidPlanFixture(t *testing.T, anchorPath string) {
	t.Helper()

	files := plankit.Render(firstCardPlan(true, "none", "internal/firstcard/new.go", ""))
	recognized := fmt.Sprintf("format: %d\n", planparser.RecognizedFormat)
	overview := strings.Replace(string(files["00-overview.md"]), recognized, "format: 99\n", 1)
	if overview == string(files["00-overview.md"]) {
		t.Fatalf("overview does not carry %q", recognized)
	}
	files["00-overview.md"] = []byte(overview)

	planDir := filepath.Join(anchorPath, lyxdirs.LyxDirName, "plan")
	if err := os.MkdirAll(planDir, 0o755); err != nil {
		t.Fatalf("mkdir plan dir: %v", err)
	}
	for name, data := range files {
		if err := os.WriteFile(filepath.Join(planDir, name), data, 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
}

// seedGlyphPlanFixture writes a syntactically complete, one-card language: go plan under
// <anchorPath>/_lyx/plan/. createTarget names the card's sole Create group entry; when useTarget is
// non-empty it also carries a Uses: entry naming useTarget, so a test can add a second glyph target
// resolved outside the Create inversion.
func seedGlyphPlanFixture(t *testing.T, anchorPath string, approved bool, createTarget, useTarget string) {
	t.Helper()
	plankit.Write(t, filepath.Join(anchorPath, lyxdirs.LyxDirName, "plan"), firstCardPlan(approved, "go", createTarget, useTarget))
}
