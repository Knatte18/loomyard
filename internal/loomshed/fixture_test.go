// fixture_test.go holds the shared fixtures for the decision record and the plan:
// validDecisionRecord, writeDiscussionFixture, and the seed helpers that write a plan under an anchor path.

package loomshed

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/lymdirs"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/testkit/plankit"
)

// validDecisionRecord is a complete decision record: the seven required sections in order, plus the optional eighth.
// It is a literal here rather than an export of internal/discussionparser:
// no caller outside that package needs the section list.
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

// seedPlanFormatFixture writes a syntactically complete, one-card plan under <anchorPath>/_lym/plan/.
// The plan is approved when approved is true.
// The sole card carries a Create group, so the path-missing check never fires whatever the worktree holds:
// a Create group's targets are exempt from on-disk existence checking.
func seedPlanFormatFixture(t *testing.T, anchorPath string, approved bool) {
	t.Helper()
	plankit.Write(t, filepath.Join(anchorPath, lymdirs.LymDirName, "plan"), firstCardPlan(approved, "none", "internal/firstcard/new.go", ""))
}

// firstCardPlan describes a one-card plan whose sole Create group targets createTarget.
// When useTarget is non-empty, the card also uses it.
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

// seedPlanContextFixture writes a one-card plan under anchorPath.
// The card renames by renamePair, a complete "`old` -> `new`" line, or edits a file when renamePair is empty.
// planSection is the body of the overview's `## PATTERN changes` section, omitted when empty.
// It writes a decision record next to the plan, with recordSection as its `## PATTERN changes` body.
// When recordSection is empty, the record has no such section; when it is the literal "absent", no record is written.
// cardFence is fenced text written into the card's Intent, omitted when empty.
// The returned PlanContext names the record path and lists no rename tool.
func seedPlanContextFixture(t *testing.T, anchorPath, renamePair, planSection, recordSection, cardFence string) PlanContext {
	t.Helper()
	plan := firstCardPlan(true, "none", "internal/firstcard/new.go", "")
	if cardFence != "" {
		plan.Cards[0].Intent += "\n\n" + cardFence
	}
	if renamePair != "" {
		plan.Cards[0].Groups = []plankit.Group{{Label: "Rename", Targets: []string{renamePair}}}
		plan.Sections = append(plan.Sections, plankit.Section{Heading: "Rename mechanic", Body: "1. Run the rename tool."})
	}
	if planSection != "" {
		plan.Sections = append(plan.Sections, plankit.Section{Heading: "PATTERN changes", Body: planSection})
	}
	plankit.Write(t, filepath.Join(anchorPath, lymdirs.LymDirName, "plan"), plan)

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

// overviewOf returns a PATTERN.md of exactly total bytes: a heading, one filler line, then entries.
// The filler sits above the entries, so a replaced bullet never swallows it as a continuation line.
func overviewOf(total int, entries ...string) string {
	const heading = "# PATTERN\n"
	tail := strings.Join(entries, "\n") + "\n"
	return heading + strings.Repeat("x", total-len(heading)-len("\n\n")-len(tail)) + "\n\n" + tail
}

// fenceOf returns lines as one fenced code block, the form a card carries a verbatim PATTERN.md entry line in.
func fenceOf(lines ...string) string {
	return "```\n" + strings.Join(lines, "\n") + "\n```"
}

// seedFormatInvalidPlanFixture writes a plan whose overview declares an unrecognized format.
// The format trips format-unrecognized whatever requireApproved is.
// The mode dimension must not change a format-invalid plan's Stuck disposition, in either mode.
func seedFormatInvalidPlanFixture(t *testing.T, anchorPath string) {
	t.Helper()

	files := plankit.Render(firstCardPlan(true, "none", "internal/firstcard/new.go", ""))
	recognized := fmt.Sprintf("format: %d\n", planparser.RecognizedFormat)
	overview := strings.Replace(string(files["00-overview.md"]), recognized, "format: 99\n", 1)
	if overview == string(files["00-overview.md"]) {
		t.Fatalf("overview does not carry %q", recognized)
	}
	files["00-overview.md"] = []byte(overview)

	planDir := filepath.Join(anchorPath, lymdirs.LymDirName, "plan")
	if err := os.MkdirAll(planDir, 0o755); err != nil {
		t.Fatalf("mkdir plan dir: %v", err)
	}
	for name, data := range files {
		if err := os.WriteFile(filepath.Join(planDir, name), data, 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
}

// seedGlyphPlanFixture writes a syntactically complete, one-card go plan under <anchorPath>/_lym/plan/.
// createTarget names the card's sole Create group entry.
// A non-empty useTarget adds a Uses: entry naming it,
// so a test can add a second glyph target resolved outside the Create inversion.
func seedGlyphPlanFixture(t *testing.T, anchorPath string, approved bool, createTarget, useTarget string) {
	t.Helper()
	plankit.Write(t, filepath.Join(anchorPath, lymdirs.LymDirName, "plan"), firstCardPlan(approved, "go", createTarget, useTarget))
}
