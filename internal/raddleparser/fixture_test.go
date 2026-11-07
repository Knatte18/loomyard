// fixture_test.go holds the valid proposal, board and decision record.
// The package's tests start from them and break one rule at a time.

package raddleparser

import (
	"slices"
	"strings"
)

// checkRecord is the decision record the tests check against: its decisions, risks, criteria and PATTERN changes.
// § stands for a backtick, which a raw string cannot hold.
var checkRecord = []byte(strings.NewReplacer("§", "`").Replace(`## Decisions

### D1 — First

The first decision.

### D2 — Second

The second decision.

## Open risks

- Risk one
- Risk two

## Acceptance criteria

- D1: first holds.
- D2: second holds.

## PATTERN changes

- §PATTERN-a§ — adds: Rule a.
- §PATTERN-b§ — adds: Rule b.
`))

// validProposal returns a proposal of parts that passes every check against the fixture decision record and board.
func validProposal() Proposal {
	return Proposal{
		Sketch:       "sketch-x",
		Shape:        ShapeTasks,
		BoardChecked: []BoardCheck{{Slug: "other-open", Revised: "2026-11-01T10:00:00Z"}},
		Parts: []Part{
			{
				Slug: "part-a", Title: "Part A", Type: "feature", Recipe: RecipeTapestry, Profile: "standard",
				Decisions: []string{"D1"}, Packages: []Package{{Path: "internal/a", Size: 10}},
				OpenRisks: []string{"Risk one"}, Criteria: []string{"D1: first holds."}, PatternChanges: []string{"PATTERN-a"},
				CostLine: "one tapestry part",
			},
			{
				Slug: "part-b", Title: "Part B", Type: "feature", Recipe: RecipeTapestry, Profile: "standard",
				Decisions: []string{"D2"}, Packages: []Package{{Path: "internal/b", Size: 10}}, DependsOn: []string{"part-a"},
				OpenRisks: []string{"Risk two"}, Criteria: []string{"D2: second holds."}, PatternChanges: []string{"PATTERN-b"},
				CostLine: "one tapestry part",
			},
		},
		Cost: Cost{Line: "two tapestry parts", NextBest: NextBest{Split: "one part", Line: "one tapestry part"}},
	}
}

// validBoard returns a board context in which the fixture proposal passes.
// The Sketch is the run's own entry, and the other open entry is weighed.
func validBoard() BoardContext {
	return BoardContext{
		Sketch:       "sketch-x",
		Entry:        "sketch-x",
		ModuleLabels: []string{"mod-a"},
		Entries: []BoardEntry{
			{Slug: "sketch-x", Kind: KindTask, Labels: []string{"mod-a", "sketch"}, Open: true, Revised: "2026-11-01T09:00:00Z"},
			{Slug: "other-open", Kind: KindTask, Labels: []string{"mod-z"}, Open: true, Revised: "2026-11-01T10:00:00Z"},
		},
		TypeLabels:  []string{"feature", "fix", "sketch"},
		Profiles:    map[string][]string{RecipeTapestry: {"standard"}, RecipeDarn: {"quick"}},
		LivePattern: func(id string) bool { return id == "PATTERN-live" },
	}
}

// checkNames returns the sorted check names of findings.
func checkNames(findings []Finding) []string {
	names := make([]string, len(findings))
	for i, finding := range findings {
		names[i] = finding.Check
	}
	slices.Sort(names)
	return names
}
