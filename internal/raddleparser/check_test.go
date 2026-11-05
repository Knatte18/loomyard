// check_test.go covers Check: a valid proposal passes with no finding, and each row breaks one rule of the spec and asserts exactly the findings it brings.

package raddleparser

import (
	"slices"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestCheck starts from one valid proposal and breaks one rule per row.
// A row whose break also trips a neighbouring rule lists that finding too.
func TestCheck(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(p *Proposal, b *BoardContext)
		want   []string
	}{
		{"ValidProposalPasses", func(*Proposal, *BoardContext) {}, nil},
		{"DecisionUnbuilt", func(p *Proposal, _ *BoardContext) { p.Parts[1].Decisions = nil }, []string{CheckCarriedDecisionUnbuilt, CheckDecisionUnbuilt}},
		{"DecisionUnknown", func(p *Proposal, _ *BoardContext) { p.Parts[0].Decisions = append(p.Parts[0].Decisions, "D9") }, []string{CheckDecisionUnknown}},
		{"CriterionUncarried", func(p *Proposal, _ *BoardContext) { p.Parts[1].Criteria = nil }, []string{CheckCriteriaMissDecision, CheckCriterionUncarried}},
		{"CriterionUnknown", func(p *Proposal, _ *BoardContext) { p.Parts[0].Criteria = append(p.Parts[0].Criteria, "D1: invented.") }, []string{CheckCriterionUnknown}},
		{"RiskUncarried", func(p *Proposal, _ *BoardContext) { p.Parts[1].OpenRisks = nil }, []string{CheckRiskUncarried}},
		{"RiskUnknown", func(p *Proposal, _ *BoardContext) { p.Parts[0].OpenRisks = append(p.Parts[0].OpenRisks, "Risk nine") }, []string{CheckRiskUnknown}},
		{"PatternUncarried", func(p *Proposal, _ *BoardContext) { p.Parts[1].PatternChanges = nil }, []string{CheckPatternUncarried}},
		{"PatternSharedByTwoParts", func(p *Proposal, _ *BoardContext) {
			p.Parts[1].PatternChanges = append(p.Parts[1].PatternChanges, "PATTERN-a")
		}, []string{CheckPatternShared}},
		{"PatternUnknown", func(p *Proposal, _ *BoardContext) {
			p.Parts[0].PatternChanges = append(p.Parts[0].PatternChanges, "PATTERN-z")
		}, []string{CheckPatternUnknown}},
		{"CriteriaMissADecisionThePartBuilds", func(p *Proposal, _ *BoardContext) { p.Parts[0].Decisions = []string{"D1", "D2"} }, []string{CheckCriteriaMissDecision}},
		{"CriterionNamesADecisionOffThePath", func(p *Proposal, _ *BoardContext) {
			p.Parts[0].Criteria = append(p.Parts[0].Criteria, "D2: second holds.")
		}, []string{CheckCarriedDecisionUnbuilt}},
		{"OpenEntryOfTheSketchsModuleUnchecked", func(_ *Proposal, b *BoardContext) {
			b.Entries = append(b.Entries, BoardEntry{Slug: "new-task", Kind: KindTask, Labels: []string{"mod-a"}, Open: true})
		}, []string{CheckBoardUnchecked}},
		{"DependsOnCycle", func(p *Proposal, _ *BoardContext) { p.Parts[0].DependsOn = []string{"part-b"} }, []string{CheckDependsCycle}},
		{"DependsOnUnknown", func(p *Proposal, _ *BoardContext) { p.Parts[1].DependsOn = append(p.Parts[1].DependsOn, "ghost") }, []string{CheckDependsUnknown}},
		{"DependsOnCheckedEntryGoneFromTheBoardPasses", func(p *Proposal, _ *BoardContext) { dependOnGoneEntry(p) }, nil},
		{"DependsOnCheckedEntryGoneFromTheBoardPassesWhileFiling", func(p *Proposal, b *BoardContext) {
			dependOnGoneEntry(p)
			b.Filing = true
		}, nil},
		{"PackagesSharedWithNoPathBetween", func(p *Proposal, _ *BoardContext) {
			p.Parts[1].DependsOn = nil
			p.Parts[1].Packages = p.Parts[0].Packages
		}, []string{CheckPackagesShared}},
		{"RecipeInvalid", func(p *Proposal, _ *BoardContext) { p.Parts[0].Recipe = "batten" }, []string{CheckRecipeInvalid}},
		{"ProfileUnknown", func(p *Proposal, _ *BoardContext) { p.Parts[0].Profile = "ghost" }, []string{CheckProfileUnknown}},
		{"PartTypedSketch", func(p *Proposal, _ *BoardContext) { p.Parts[0].Type = TypeSketch }, []string{CheckTypeInvalid}},
		{"ProjectWithNoValidType", func(p *Proposal, _ *BoardContext) {
			p.Shape = ShapeProject
			p.Project = &Project{Slug: "proj-new", Title: "Project"}
		}, []string{CheckTypeInvalid}},
		{"SlugOnTheBoard", func(p *Proposal, _ *BoardContext) { p.Parts[1].Slug = "other-open" }, []string{CheckSlugTaken}},
		{"SlugInTheStore", func(p *Proposal, b *BoardContext) {
			p.Parts[1].Slug = "stored"
			b.Store = []string{"stored"}
		}, []string{CheckSlugTaken}},
		{"SlugEqualToTheSketchs", func(p *Proposal, _ *BoardContext) { p.Parts[1].Slug = "sketch-x" }, []string{CheckSlugSketch}},
		{"SlugFiledTwice", func(p *Proposal, _ *BoardContext) {
			p.Parts = append(p.Parts, Part{Slug: "part-b", Type: "feature", Recipe: RecipeTapestry, Profile: "standard", Packages: []Package{{Path: "internal/c", Size: 1}}})
		}, []string{CheckSlugDuplicate}},
		{"TapestryWithTwoParts", func(p *Proposal, _ *BoardContext) { p.Shape = ShapeTapestry }, []string{CheckShapeInvalid}},
		{"ProjectShapeWithNoProject", func(p *Proposal, _ *BoardContext) { p.Shape = ShapeProject }, []string{CheckShapeInvalid}},
		{"TasksShapeWithAProject", func(p *Proposal, _ *BoardContext) { p.Project = &Project{Slug: "proj-new", Type: "feature"} }, []string{CheckShapeInvalid}},
		{"ShapeOutsideTheFour", func(p *Proposal, _ *BoardContext) { p.Shape = "epic" }, []string{CheckShapeInvalid}},
		{"ProposalCostLineMissing", func(p *Proposal, _ *BoardContext) { p.Cost.Line = "" }, []string{CheckCostMissing}},
		{"NextBestCostLineMissing", func(p *Proposal, _ *BoardContext) { p.Cost.NextBest.Line = "" }, []string{CheckCostMissing}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			proposal, board := validProposal(), validBoard()
			tt.mutate(&proposal, &board)

			got := checkNames(Check(proposal, checkRecord, board))

			if !slices.Equal(got, tt.want) {
				t.Errorf("Check() findings = %v; want %v", got, tt.want)
			}
		})
	}
}

// dependOnGoneEntry makes the second part depend on an entry the proposal weighed that is no longer on the board.
func dependOnGoneEntry(p *Proposal) {
	p.BoardChecked = append(p.BoardChecked, BoardCheck{Slug: "landed-thing", Revised: "2026-11-01T08:00:00Z"})
	p.Parts[1].DependsOn = append(p.Parts[1].DependsOn, "landed-thing")
}
