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
	"testing"

	"github.com/Knatte18/loomyard/internal/lyxdirs"
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

	planDir := filepath.Join(anchorPath, lyxdirs.LyxDirName, "plan")
	if err := os.MkdirAll(planDir, 0o755); err != nil {
		t.Fatalf("mkdir plan dir: %v", err)
	}

	cardBody := "# Card 1 — first-card\n\n**Create:**\n- `internal/firstcard/new.go`\n\n" +
		"**Intent:** placeholder card.\n"
	if err := os.WriteFile(filepath.Join(planDir, "01-first-card.md"), []byte(cardBody), 0o644); err != nil {
		t.Fatalf("write card file: %v", err)
	}

	overview := fmt.Sprintf(
		"---\nformat: 5\napproved: %t\nlanguage: none\n---\n\n# Plan\n\nFraming.\n\n## Card Index\n\n1 — first-card — placeholder card 1\n",
		approved,
	)
	if err := os.WriteFile(filepath.Join(planDir, "00-overview.md"), []byte(overview), 0o644); err != nil {
		t.Fatalf("write overview file: %v", err)
	}
}

// seedFormatInvalidPlanFixture writes a plan whose overview declares an unrecognized format,
// tripping format-unrecognized regardless of requireApproved — the mode dimension must not change a
// format-invalid plan's Stuck disposition either way.
func seedFormatInvalidPlanFixture(t *testing.T, anchorPath string) {
	t.Helper()

	planDir := filepath.Join(anchorPath, lyxdirs.LyxDirName, "plan")
	if err := os.MkdirAll(planDir, 0o755); err != nil {
		t.Fatalf("mkdir plan dir: %v", err)
	}

	cardBody := "# Card 1 — first-card\n\n**Create:**\n- `internal/firstcard/new.go`\n\n" +
		"**Intent:** placeholder card.\n"
	if err := os.WriteFile(filepath.Join(planDir, "01-first-card.md"), []byte(cardBody), 0o644); err != nil {
		t.Fatalf("write card file: %v", err)
	}

	overview := "---\nformat: 99\napproved: true\nlanguage: none\n---\n\n# Plan\n\nFraming.\n\n## Card Index\n\n1 — first-card — placeholder card 1\n"
	if err := os.WriteFile(filepath.Join(planDir, "00-overview.md"), []byte(overview), 0o644); err != nil {
		t.Fatalf("write overview file: %v", err)
	}
}

// writeGlyphRepoFixture writes files (keyed by repository-relative path) under a fresh t.TempDir()
// and returns that directory's absolute path, ready to hand to NewPlanGate as worktreeRoot --
// duplicated from internal/planglyph/repo_test.go's writeFixtureRepo per the
// duplicate-test-helpers-rather-than-share-them Shared Decision.
func writeGlyphRepoFixture(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for rel, content := range files {
		full := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("MkdirAll(%q) failed: %v", filepath.Dir(full), err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatalf("WriteFile(%q) failed: %v", full, err)
		}
	}
	return root
}

// seedGlyphPlanFixture writes a syntactically complete, one-card language: go plan under
// <anchorPath>/_lyx/plan/. createTarget names the card's sole Create group entry; when useTarget is
// non-empty it also carries a Uses: entry naming useTarget, so a test can add a second glyph target
// resolved outside the Create inversion.
func seedGlyphPlanFixture(t *testing.T, anchorPath string, approved bool, createTarget, useTarget string) {
	t.Helper()

	planDir := filepath.Join(anchorPath, lyxdirs.LyxDirName, "plan")
	if err := os.MkdirAll(planDir, 0o755); err != nil {
		t.Fatalf("mkdir plan dir: %v", err)
	}

	usesBlock := ""
	if useTarget != "" {
		usesBlock = fmt.Sprintf("\n**Uses:**\n- `%s`\n", useTarget)
	}
	cardBody := fmt.Sprintf(
		"# Card 1 — first-card\n\n**Create:**\n- `%s`\n%s\n**Intent:** placeholder card.\n",
		createTarget, usesBlock,
	)
	if err := os.WriteFile(filepath.Join(planDir, "01-first-card.md"), []byte(cardBody), 0o644); err != nil {
		t.Fatalf("write card file: %v", err)
	}

	overview := fmt.Sprintf(
		"---\nformat: 5\napproved: %t\nlanguage: go\n---\n\n# Plan\n\nFraming.\n\n## Card Index\n\n1 — first-card — placeholder card 1\n",
		approved,
	)
	if err := os.WriteFile(filepath.Join(planDir, "00-overview.md"), []byte(overview), 0o644); err != nil {
		t.Fatalf("write overview file: %v", err)
	}
}
