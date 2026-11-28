// check_test.go covers Check: a valid proposal passes with no finding.
// Each row breaks one rule of the spec and asserts exactly the findings it brings.

package raddleparser

import (
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestCheck starts from one valid proposal and breaks one rule per row.
// A row whose break also trips a neighbouring rule lists that finding too.
func TestCheck(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	// mapped is a mapping of the first part's change onto the board's live entry.
	mapped := PatternCluster{SketchID: "PATTERN-a", Entry: "PATTERN-live", Text: "Live rule."}
	withText := func(text string) PatternCluster {
		cluster := mapped
		cluster.Text = text
		return cluster
	}

	tests := []struct {
		name   string
		mutate func(p *Proposal, b *BoardContext)
		want   []string
	}{
		{"ValidProposalPasses", func(*Proposal, *BoardContext) {}, nil},
		{"ChangeMappedOntoALiveEntryPasses", func(p *Proposal, _ *BoardContext) { p.Parts[0].PatternClusters = []PatternCluster{mapped} }, nil},
		{"ClusterOfAChangeThePartDoesNotCarry", func(p *Proposal, _ *BoardContext) { p.Parts[1].PatternClusters = []PatternCluster{mapped} }, []string{CheckPatternClusterInvalid}},
		{"ClusterWrittenTwice", func(p *Proposal, _ *BoardContext) { p.Parts[0].PatternClusters = []PatternCluster{mapped, mapped} }, []string{CheckPatternClusterInvalid}},
		{"ClusterWithNoText", func(p *Proposal, _ *BoardContext) { p.Parts[0].PatternClusters = []PatternCluster{withText(" ")} }, []string{CheckPatternClusterInvalid}},
		{"ClusterTextOfTwoLines", func(p *Proposal, _ *BoardContext) {
			p.Parts[0].PatternClusters = []PatternCluster{withText("One.\nTwo.")}
		}, []string{CheckPatternClusterInvalid}},
		{"ClusterTextOverTheCap", func(p *Proposal, _ *BoardContext) {
			p.Parts[0].PatternClusters = []PatternCluster{withText(strings.Repeat("x", 300))}
		}, []string{CheckPatternClusterInvalid}},
		{"ClusterOntoAnEntryPatternMdLacks", func(p *Proposal, _ *BoardContext) {
			p.Parts[0].PatternClusters = []PatternCluster{{SketchID: "PATTERN-a", Entry: "PATTERN-ghost", Text: "Rule."}}
		}, []string{CheckPatternClusterUnknown}},
		{"TwoPartsMapOntoOneEntry", func(p *Proposal, _ *BoardContext) {
			p.Parts[0].PatternClusters = []PatternCluster{mapped}
			p.Parts[1].PatternClusters = []PatternCluster{{SketchID: "PATTERN-b", Entry: "PATTERN-live", Text: "Rule."}}
		}, []string{CheckPatternClusterShared}},
		{"ClusterOntoAnotherDeclaredEntry", func(p *Proposal, b *BoardContext) {
			b.LivePattern = func(string) bool { return true }
			p.Parts[0].PatternClusters = []PatternCluster{{SketchID: "PATTERN-a", Entry: "PATTERN-b", Text: "Rule."}}
		}, []string{CheckPatternClusterShared}},
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
		{"DependsOnANoteOnTheBoardPasses", func(p *Proposal, b *BoardContext) {
			b.Entries = append(b.Entries, BoardEntry{Slug: "idea", Kind: KindNote, Open: true})
			p.Parts[1].DependsOn = append(p.Parts[1].DependsOn, "idea")
		}, nil},
		{"DependsOnPartThatIsAlsoANoteIsASlugClash", func(_ *Proposal, b *BoardContext) {
			b.Entries = append(b.Entries, BoardEntry{Slug: "part-a", Kind: KindNote, Open: true})
		}, []string{CheckSlugTaken}},
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
		{"TapestryPartIsRecipeInvalid", func(p *Proposal, _ *BoardContext) { p.Parts[0].Recipe = "tapestry" }, []string{CheckRecipeInvalid}},
		{"DarnPartPasses", func(p *Proposal, _ *BoardContext) {
			p.Parts[0].Recipe = RecipeDarn
			p.Parts[0].Profile = "quick"
		}, nil},
		{"ProfileUnknown", func(p *Proposal, _ *BoardContext) { p.Parts[0].Profile = "ghost" }, []string{CheckProfileUnknown}},
		{"PartTypedSketch", func(p *Proposal, _ *BoardContext) { p.Parts[0].Type = TypeSketch }, []string{CheckTypeInvalid}},
		{"PartWithNoModuleLabel", func(p *Proposal, _ *BoardContext) { p.Parts[0].Labels = nil }, []string{CheckLabelsMissing}},
		{"PartLabelOutsideTheBoardsVocabulary", func(p *Proposal, _ *BoardContext) { p.Parts[0].Labels = []string{"mod-a", "ghost"} }, []string{CheckLabelUnknown}},
		{"PartLabelListedTwice", func(p *Proposal, _ *BoardContext) { p.Parts[0].Labels = []string{"mod-a", "mod-a"} }, []string{CheckLabelDuplicate}},
		{"PartLabelThatIsATypeLabel", func(p *Proposal, _ *BoardContext) { p.Parts[0].Labels = []string{"feature"} }, []string{CheckLabelUnknown}},
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
			p.Parts = append(p.Parts, Part{Slug: "part-b", Type: "feature", Labels: []string{"mod-a"}, Recipe: RecipeJacquard, Profile: "standard", Packages: []Package{{Path: "internal/c", Size: 1}}})
		}, []string{CheckSlugDuplicate}},
		{"SlugTheBoardRefuses", func(p *Proposal, _ *BoardContext) { p.Parts[1].Slug = refusedSlug }, []string{CheckSlugInvalid}},
		{"ProjectSlugTheBoardRefuses", func(p *Proposal, _ *BoardContext) {
			p.Shape = ShapeProject
			p.Project = &Project{Slug: refusedSlug, Title: "Project", Type: "feature"}
		}, []string{CheckSlugInvalid}},
		{"NoSlugRuleChecksNothing", func(p *Proposal, b *BoardContext) {
			p.Parts[1].Slug = refusedSlug
			b.ValidateSlug = nil
		}, nil},
		{"TapestryWithTwoParts", func(p *Proposal, _ *BoardContext) { p.Shape = ShapeTapestry }, []string{CheckShapeInvalid}},
		{"ProjectShapeWithNoProject", func(p *Proposal, _ *BoardContext) { p.Shape = ShapeProject }, []string{CheckShapeInvalid}},
		{"TasksShapeWithAProject", func(p *Proposal, _ *BoardContext) { p.Project = &Project{Slug: "proj-new", Type: "feature"} }, []string{CheckShapeInvalid}},
		{"ShapeOutsideTheFour", func(p *Proposal, _ *BoardContext) { p.Shape = "epic" }, []string{CheckShapeInvalid}},
		{"PerchTapestryPasses", func(p *Proposal, b *BoardContext) {
			mergeIntoOnePart(p)
			p.Shape = ShapeTapestry
			b.Perch = true
		}, nil},
		{"PerchTasksShapeWithOnePartPasses", func(p *Proposal, b *BoardContext) {
			mergeIntoOnePart(p)
			b.Perch = true
		}, nil},
		{"PerchProjectUnderTheRunsEntryPasses", func(p *Proposal, b *BoardContext) {
			p.Shape = ShapeProject
			p.Project = &Project{Slug: "sketch-x", Title: "Project", Type: "feature"}
			b.Perch = true
		}, nil},
		{"ProjectUnderTheRunsEntryOutsidePerch", func(p *Proposal, _ *BoardContext) {
			p.Shape = ShapeProject
			p.Project = &Project{Slug: "sketch-x", Title: "Project", Type: "feature"}
		}, []string{CheckSlugSketch}},
		{"PerchTasksShapeWithTwoParts", func(_ *Proposal, b *BoardContext) { b.Perch = true }, []string{CheckPerchOutcome}},
		{"PerchBeam", func(p *Proposal, b *BoardContext) {
			p.Shape = ShapeBeam
			p.Project = &Project{Slug: "proj-new", Title: "Project", Type: "feature"}
			b.Perch = true
		}, []string{CheckPerchOutcome}},
		{"PerchProjectUnderAnotherSlug", func(p *Proposal, b *BoardContext) {
			p.Shape = ShapeProject
			p.Project = &Project{Slug: "proj-new", Title: "Project", Type: "feature"}
			b.Perch = true
		}, []string{CheckPerchOutcome}},
		{"PerchPartUnderTheSketchsSlug", func(p *Proposal, b *BoardContext) {
			p.Shape = ShapeProject
			p.Project = &Project{Slug: "sketch-x", Title: "Project", Type: "feature"}
			p.Parts[1].Slug = "sketch-x"
			b.Perch = true
		}, []string{CheckSlugDuplicate, CheckSlugSketch}},
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

// TestCheck_DetailNamesTheWayForward pins the text that tells the author how to proceed past a refusal.
//
//testtiming:keep pins each refusal's detail text, its reason and way forward, which TestCheck never asserts
func TestCheck_DetailNamesTheWayForward(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name   string
		check  string
		mutate func(p *Proposal, b *BoardContext)
		want   []string
	}{
		{"SlugInvalid", CheckSlugInvalid, func(p *Proposal, _ *BoardContext) { p.Parts[1].Slug = refusedSlug }, []string{
			`slug "refused-slug" is refused`, "pick a slug the board admits", "lym loom raddle-propose",
		}},
		{"TapestryPartNamesJacquard", CheckRecipeInvalid, func(p *Proposal, _ *BoardContext) { p.Parts[0].Recipe = "tapestry" }, []string{
			`recipe "tapestry" is neither jacquard nor darn`, "a part filed from a Sketch is filed with its decision record",
			"jacquard is the recipe that builds from it", "set the part's recipe to jacquard or darn", "lym loom raddle-propose",
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			proposal, board := validProposal(), validBoard()
			tt.mutate(&proposal, &board)

			var details []string
			for _, finding := range Check(proposal, checkRecord, board) {
				if finding.Check == tt.check {
					details = append(details, finding.Detail)
				}
			}

			if len(details) != 1 {
				t.Fatalf("Check() %s details = %q; want exactly one", tt.check, details)
			}
			for _, want := range tt.want {
				if !strings.Contains(details[0], want) {
					t.Errorf("%s detail = %q; want it to contain %q", tt.check, details[0], want)
				}
			}
		})
	}
}

// mergeIntoOnePart folds the second part into the first, leaving one part that builds every decision.
func mergeIntoOnePart(p *Proposal) {
	first, second := p.Parts[0], p.Parts[1]
	first.Decisions = append(first.Decisions, second.Decisions...)
	first.OpenRisks = append(first.OpenRisks, second.OpenRisks...)
	first.Criteria = append(first.Criteria, second.Criteria...)
	first.PatternChanges = append(first.PatternChanges, second.PatternChanges...)
	first.Packages = append(first.Packages, second.Packages...)
	first.Labels = append(first.Labels, second.Labels...)
	p.Parts = []Part{first}
}

// dependOnGoneEntry makes the second part depend on an entry the proposal weighed that is not on the board.
func dependOnGoneEntry(p *Proposal) {
	p.BoardChecked = append(p.BoardChecked, BoardCheck{Slug: "landed-thing", Revised: "2026-11-01T08:00:00Z"})
	p.Parts[1].DependsOn = append(p.Parts[1].DependsOn, "landed-thing")
}
