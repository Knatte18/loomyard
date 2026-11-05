// partsplice_test.go covers SectionBullets and SplicePart over small fixture records.

package discussionparser

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// spliceRecord is a decision record with three decisions, one added entry, a carry-over block and two PATTERN changes; § stands for a backtick.
var spliceRecord = strings.NewReplacer("§", "`").Replace(`# Decision record

## Goal

Build the thing.

## Scope

The thing.

## Decisions

### D1 — First call

The first decision.

#### Detail

Kept with D1.

### D2 — Second call

The second decision.

### D3 — Third call

The third decision.

### Added after Discussion (parent, 2026-11-05): A late call

Decision: late.

## Constraints

- Keep it small.

## Auto-mode assumptions

- None.

## Open risks

- Risk one
  continues here.
- Risk two

<!-- lym:carry-over code -->
- Code-Review, round 1: converged.
<!-- /lym:carry-over code -->

## Acceptance criteria

- D1: first holds.
- D2: second holds.
- D3: third holds.

## PATTERN changes

- §PATTERN-one§ — adds: One rule.
- §PATTERN-two§ — changes: Two rule.
`)

// TestSectionBullets covers plain bullets, continuation lines, a fenced bullet, a carry-over block and an absent heading.
func TestSectionBullets(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name    string
		record  string
		heading string
		want    []string
	}{
		{"PlainBullets", "## Open risks\n\n- a\n- b\n\n## Acceptance criteria\n\n- c\n", "Open risks", []string{"a", "b"}},
		{"ContinuationLines", "## Open risks\n\n- a\n  more\n  - nested\n- b\n", "Open risks", []string{"a\nmore\n- nested", "b"}},
		{"FencedBulletSkipped", "## Open risks\n\n```\n- fenced\n```\n- real\n", "Open risks", []string{"real"}},
		{"CarryOverBlockSkipped", "## Open risks\n\n- own\n\n<!-- lym:carry-over code -->\n- carried\n<!-- /lym:carry-over code -->\n", "Open risks", []string{"own"}},
		{"AbsentHeading", "## Goal\n\n- a\n", "Open risks", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := SectionBullets([]byte(tt.record), tt.heading); !slices.Equal(got, tt.want) {
				t.Errorf("SectionBullets(%q) = %q; want %q", tt.heading, got, tt.want)
			}
		})
	}
}

// TestSplicePart covers the spliced record's content, that it passes the decision-id and heading checks, and the refusal of an id, bullet or PATTERN entry the record lacks.
func TestSplicePart(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	part := PartSplice{
		Goal:           "Build the first part.",
		Scope:          "Only the first part.",
		Decisions:      []string{"D1", "D3"},
		OpenRisks:      []string{"Risk one\ncontinues here."},
		Criteria:       []string{"D1: first holds.", "D3: third holds."},
		PatternChanges: []string{"PATTERN-two"},
	}

	t.Run("SplicedRecord", func(t *testing.T) {
		t.Parallel()
		spliced, err := SplicePart([]byte(spliceRecord), part)
		if err != nil {
			t.Fatalf("SplicePart() error = %v; want nil", err)
		}
		text := string(spliced)

		if got := DecisionIDs(spliced); !slices.Equal(got, []string{"D1", "D3"}) {
			t.Errorf("DecisionIDs() = %v; want [D1 D3]", got)
		}
		added, err := ParseAddedDecisions(spliced)
		if err != nil || len(added) != 1 || added[0].Title != "A late call" {
			t.Errorf("ParseAddedDecisions() = %+v, %v; want the one entry titled %q", added, err, "A late call")
		}
		for _, kept := range []string{"Build the first part.", "Only the first part.", "Kept with D1.", "- Keep it small.", "- None."} {
			if !strings.Contains(text, kept) {
				t.Errorf("spliced record lacks %q:\n%s", kept, text)
			}
		}
		for _, dropped := range []string{"The second decision.", "Risk two", "D2: second holds.", "PATTERN-one", "lym:carry-over"} {
			if strings.Contains(text, dropped) {
				t.Errorf("spliced record holds %q, which the part does not carry:\n%s", dropped, text)
			}
		}
		if got := SectionBullets(spliced, "Open risks"); !slices.Equal(got, part.OpenRisks) {
			t.Errorf("SectionBullets(Open risks) = %q; want %q", got, part.OpenRisks)
		}
		if got := SectionBullets(spliced, "Acceptance criteria"); !slices.Equal(got, part.Criteria) {
			t.Errorf("SectionBullets(Acceptance criteria) = %q; want %q", got, part.Criteria)
		}
		if changes, findings := ParsePatternChanges(spliced); len(findings) != 0 || len(changes) != 1 || changes[0].ID != "PATTERN-two" {
			t.Errorf("ParsePatternChanges() = %+v, %v; want only PATTERN-two", changes, findings)
		}
		if findings := CheckDecisionIDs(spliced); len(findings) != 0 {
			t.Errorf("CheckDecisionIDs() = %v; want no finding", findings)
		}

		dir := t.TempDir()
		recordPath, logPath := filepath.Join(dir, "decision-record.md"), filepath.Join(dir, "support-log.md")
		if err := os.WriteFile(recordPath, spliced, 0o644); err != nil {
			t.Fatalf("write spliced record: %v", err)
		}
		if err := os.WriteFile(logPath, nil, 0o644); err != nil {
			t.Fatalf("write support log: %v", err)
		}
		if findings, err := Validate(recordPath, logPath); err != nil || len(findings) != 0 {
			t.Errorf("Validate() = %v, %v; want no finding and no error", findings, err)
		}
	})

	t.Run("NoPatternSectionWithoutAChange", func(t *testing.T) {
		t.Parallel()
		bare := part
		bare.PatternChanges = nil
		spliced, err := SplicePart([]byte(spliceRecord), bare)
		if err != nil {
			t.Fatalf("SplicePart() error = %v; want nil", err)
		}
		if strings.Contains(string(spliced), patternChangesHeading) {
			t.Errorf("spliced record holds %q for a part with no PATTERN change:\n%s", patternChangesHeading, spliced)
		}
	})

	refusals := []struct {
		name   string
		mutate func(*PartSplice)
		want   string
	}{
		{"UnknownDecision", func(p *PartSplice) { p.Decisions = []string{"D9"} }, "D9"},
		{"UnknownRisk", func(p *PartSplice) { p.OpenRisks = []string{"Risk nine"} }, "Risk nine"},
		{"UnknownCriterion", func(p *PartSplice) { p.Criteria = []string{"D9: nothing."} }, "D9: nothing."},
		{"UnknownPatternEntry", func(p *PartSplice) { p.PatternChanges = []string{"PATTERN-nine"} }, "PATTERN-nine"},
	}
	for _, tt := range refusals {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			broken := part
			tt.mutate(&broken)
			if _, err := SplicePart([]byte(spliceRecord), broken); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("SplicePart() error = %v; want one naming %q", err, tt.want)
			}
		})
	}
}
