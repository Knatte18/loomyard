// roundcheck_test.go tables CheckReviewRound over every refusal of the round checks and the reviews they accept.

package burlerengine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// marshalRoundReview renders a review whose frontmatter holds prior and findings.
// Its verdict is the one the finding severities call for.
// The body holds no prose blocks, since the round checks never read them.
func marshalRoundReview(t *testing.T, prior []PriorEntry, findings []Finding) []byte {
	t.Helper()
	header := reviewHeader{Verdict: string(VerdictApproved), Prior: prior, Findings: findings}
	for _, f := range findings {
		if f.Severity == SeverityBlocking {
			header.Verdict = string(VerdictBlocking)
		}
	}
	data, err := yaml.Marshal(header)
	if err != nil {
		t.Fatalf("yaml.Marshal(review header) = %v; want nil", err)
	}
	return []byte("---\n" + string(data) + "---\n")
}

func TestCheckReviewRound(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	const root = "/work"
	reportPath := filepath.Join(t.TempDir(), "ask-1.report.yaml")
	report, err := yaml.Marshal(ParentReport{Ask: 1, Findings: []ParentFinding{{ID: "P1", Change: "change it"}}})
	if err != nil {
		t.Fatalf("yaml.Marshal(parent report) = %v; want nil", err)
	}
	if err := os.WriteFile(reportPath, report, 0o644); err != nil {
		t.Fatalf("WriteFile(parent report) = %v; want nil", err)
	}

	finding := func(id string, severity Severity, location string) Finding {
		return Finding{ID: id, Severity: severity, Class: ClassScope, Location: location, Summary: "s", Fix: "f"}
	}
	carrying := func(f Finding, earlier string) Finding {
		f.Carries = earlier
		return f
	}
	fromParent := func(f Finding, parentIDs ...string) Finding {
		f.Origin, f.ParentIDs = ParentOrigin, parentIDs
		return f
	}
	entry := func(id string, status PriorStatus) PriorEntry { return PriorEntry{ID: id, Status: status} }
	// round builds an incremental context whose round before held one MEDIUM finding E1 at a.go:10 with no disposition.
	round := func(tweak func(*RoundContext)) *RoundContext {
		ctx := &RoundContext{Incremental: true, WorktreeRoot: root, Prior: []PriorFinding{{Finding: finding("E1", SeverityMedium, "a.go:10")}}}
		if tweak != nil {
			tweak(ctx)
		}
		return ctx
	}
	changed := func(paths ...string) func(*RoundContext) {
		return func(ctx *RoundContext) { ctx.HasChanges, ctx.Changed = true, paths }
	}
	withParentReport := func(tweak func(*RoundContext)) func(*RoundContext) {
		return func(ctx *RoundContext) {
			ctx.ParentReportPath = reportPath
			tweak(ctx)
		}
	}
	disposed := func(disposition Disposition) func(*RoundContext) {
		return func(ctx *RoundContext) {
			ctx.Prior[0].Disposition = &FindingDisposition{ID: "E1", Disposition: disposition}
		}
	}
	amending := func(texts ...string) func(*RoundContext) {
		return func(ctx *RoundContext) { ctx.AmendingTexts = texts }
	}
	halted := func(reason string) func(*RoundContext) {
		return func(ctx *RoundContext) { ctx.HaltReason = reason }
	}
	changedWith := func(tweaks ...func(*RoundContext)) func(*RoundContext) {
		return func(ctx *RoundContext) {
			changed("c.go")(ctx)
			for _, tweak := range tweaks {
				tweak(ctx)
			}
		}
	}
	fromWebster := func(f Finding) Finding {
		f.Origin = WebsterOrigin
		return f
	}
	focusing := func(ctx *RoundContext) { ctx.FocusFindings = true }
	fromFocus := func(f Finding, class Class) Finding {
		f.Origin, f.Class = FocusOrigin, class
		return f
	}
	const planCard = "_lym/plan/14-card-slug.md"
	const amendingEntry = "### Rework the slug card\n\nAmend plan card 14-card-slug to name the retry."
	held := func(titles ...string) func(*RoundContext) {
		return func(ctx *RoundContext) {
			ctx.Prior[0].Held, ctx.AddedTitles = true, titles
		}
	}

	tests := []struct {
		name     string
		ctx      *RoundContext
		prior    []PriorEntry
		findings []Finding
		// wantErr is the substring the refusal holds; empty means the review passes.
		wantErr string
	}{
		{name: "NoContext_PriorEntryRefused", prior: []PriorEntry{entry("E1", PriorOpen)}, wantErr: "holds a `prior` list"},
		{name: "NoContext_CarriesRefused", findings: []Finding{carrying(finding("F1", SeverityMedium, "a.go:1"), "E1")}, wantErr: "no earlier findings to carry"},
		{name: "NoContext_PlainReviewPasses", findings: []Finding{finding("F1", SeverityBlocking, "z.go:1")}},

		{name: "PriorMissingAnId", ctx: round(nil), wantErr: "no `prior` entry for finding \"E1\""},
		{name: "PriorRepeatsAnId", ctx: round(nil), prior: []PriorEntry{entry("E1", PriorResolved), entry("E1", PriorResolved)}, wantErr: "twice"},
		{name: "PriorInventsAnId", ctx: round(nil), prior: []PriorEntry{entry("E1", PriorResolved), entry("E9", PriorResolved)}, wantErr: "does not list"},
		{name: "PriorStatusOutsideTheFour", ctx: round(nil), prior: []PriorEntry{entry("E1", "done")}, wantErr: "none of resolved, open, outdated and withdrawn"},
		{name: "WithdrawnOnAnUndisputedFinding", ctx: round(disposed(DispositionFixed)), prior: []PriorEntry{entry("E1", PriorWithdrawn)}, wantErr: "not `disputed`"},
		{
			name: "OpenWithoutAnswerOnADispute", ctx: round(disposed(DispositionDisputed)), prior: []PriorEntry{entry("E1", PriorOpen)},
			findings: []Finding{carrying(finding("F1", SeverityMedium, "a.go:10"), "E1")}, wantErr: "without answering",
		},
		{name: "HeldFindingOutdated", ctx: round(held("Use X")), prior: []PriorEntry{entry("E1", PriorOutdated)}, wantErr: "held finding"},
		{
			name: "HeldFindingResolvedWithoutAQuotedTitle", ctx: round(held("Use X")),
			prior: []PriorEntry{{ID: "E1", Status: PriorResolved, Answer: "done"}}, wantErr: "without quoting the title",
		},

		{name: "CarriesANonOpenEntry", ctx: round(nil), prior: []PriorEntry{entry("E1", PriorResolved)}, findings: []Finding{carrying(finding("F1", SeverityMedium, "a.go:10"), "E1")}, wantErr: "not an open `prior` entry"},
		{name: "CarriesAnUnknownId", ctx: round(nil), prior: []PriorEntry{entry("E1", PriorResolved)}, findings: []Finding{carrying(finding("F1", SeverityMedium, "a.go:10"), "E9")}, wantErr: "not an open `prior` entry"},
		{
			name: "OpenIdCarriedTwice", ctx: round(nil), prior: []PriorEntry{entry("E1", PriorOpen)},
			findings: []Finding{carrying(finding("F1", SeverityMedium, "a.go:10"), "E1"), carrying(finding("F2", SeverityMedium, "a.go:10"), "E1")}, wantErr: "more than one finding",
		},
		{name: "OpenIdNotCarried", ctx: round(nil), prior: []PriorEntry{entry("E1", PriorOpen)}, wantErr: "no finding carries it"},
		{
			name: "CarriedClassDiffers", ctx: round(nil), prior: []PriorEntry{entry("E1", PriorOpen)},
			findings: []Finding{carrying(Finding{ID: "F1", Severity: SeverityMedium, Class: ClassDesign, Location: "a.go:10", Summary: "s", Fix: "f"}, "E1")}, wantErr: "not the earlier class",
		},
		{name: "CarriedSeverityRaised", ctx: round(nil), prior: []PriorEntry{entry("E1", PriorOpen)}, findings: []Finding{carrying(finding("F1", SeverityBlocking, "a.go:10"), "E1")}, wantErr: "above its earlier"},
		{name: "CarriedPathMovedWithNoChangesFile", ctx: round(nil), prior: []PriorEntry{entry("E1", PriorOpen)}, findings: []Finding{carrying(finding("F1", SeverityMedium, "b.go:3"), "E1")}, wantErr: "no changes file"},
		{name: "CarriedPathMovedToAnUntouchedFile", ctx: round(changed("c.go")), prior: []PriorEntry{entry("E1", PriorOpen)}, findings: []Finding{carrying(finding("F1", SeverityMedium, "b.go:3"), "E1")}, wantErr: "does not list"},

		{name: "NewMediumInAnIncrementalRound", ctx: round(nil), prior: []PriorEntry{entry("E1", PriorResolved)}, findings: []Finding{finding("F1", SeverityMedium, "a.go:3")}, wantErr: "only at BLOCKING"},
		{name: "NewBlockingOnAnUntouchedFile", ctx: round(changed("c.go")), prior: []PriorEntry{entry("E1", PriorResolved)}, findings: []Finding{finding("F1", SeverityBlocking, "a.go:3")}, wantErr: "does not list"},
		{name: "NewBlockingCutAtASpaceOnAnUntouchedFile", ctx: round(changed("c.go")), prior: []PriorEntry{entry("E1", PriorResolved)}, findings: []Finding{finding("F1", SeverityBlocking, "a.go some prose")}, wantErr: "\"a.go\", which the changes file does not list"},
		{name: "NewParentFindingWithNoAsk", ctx: round(nil), prior: []PriorEntry{entry("E1", PriorResolved)}, findings: []Finding{fromParent(finding("F1", SeverityBlocking, "a.go:3"), "P1")}, wantErr: "origin: parent"},
		{name: "NewParentFindingWithNoParentIds", ctx: round(withParentReport(func(*RoundContext) {})), prior: []PriorEntry{entry("E1", PriorResolved)}, findings: []Finding{fromParent(finding("F1", SeverityBlocking, "a.go:3"))}, wantErr: "origin: parent"},
		{name: "NewParentFindingWithAnIdTheReportLacks", ctx: round(withParentReport(func(*RoundContext) {})), prior: []PriorEntry{entry("E1", PriorResolved)}, findings: []Finding{fromParent(finding("F1", SeverityBlocking, "a.go:3"), "P1", "P9")}, wantErr: "origin: parent"},

		{name: "CarriedMovedToATouchedFile", ctx: round(changed("c.go")), prior: []PriorEntry{entry("E1", PriorOpen)}, findings: []Finding{carrying(finding("F1", SeverityMedium, "c.go:4"), "E1")}},
		{name: "CarriedKeepsItsPathWithNoChangesFile", ctx: round(nil), prior: []PriorEntry{entry("E1", PriorOpen)}, findings: []Finding{carrying(finding("F1", SeverityLow, "a.go:99"), "E1")}},
		{name: "NewBlockingOnATouchedFileByLine", ctx: round(changed("c.go")), prior: []PriorEntry{entry("E1", PriorResolved)}, findings: []Finding{finding("F1", SeverityBlocking, "c.go:12")}},
		{name: "NewBlockingOnATouchedFileBySymbol", ctx: round(changed("c.go")), prior: []PriorEntry{entry("E1", PriorResolved)}, findings: []Finding{finding("F1", SeverityBlocking, "c.go#Sym")}},
		{name: "NewBlockingOnATouchedFileByAbsolutePath", ctx: round(changed("c.go")), prior: []PriorEntry{entry("E1", PriorResolved)}, findings: []Finding{finding("F1", SeverityBlocking, root+"/c.go:7")}},
		{name: "NewBlockingOnATouchedFileCutAtASpace", ctx: round(changed("c.go")), prior: []PriorEntry{entry("E1", PriorResolved)}, findings: []Finding{finding("F1", SeverityBlocking, "c.go some prose")}},
		{name: "NewBlockingOnAnyFileWithNoChangesFile", ctx: round(nil), prior: []PriorEntry{entry("E1", PriorResolved)}, findings: []Finding{finding("F1", SeverityBlocking, "z.go:1")}},
		{name: "NewMediumInAFullRound", ctx: round(func(ctx *RoundContext) { ctx.Incremental = false }), prior: []PriorEntry{entry("E1", PriorResolved)}, findings: []Finding{finding("F1", SeverityMedium, "z.go:1")}},
		{
			name: "ExemptParentFindingOnAnUntouchedFileAtLow",
			ctx:  round(withParentReport(changed("c.go"))), prior: []PriorEntry{entry("E1", PriorResolved)},
			findings: []Finding{fromParent(finding("F1", SeverityLow, "z.go:1"), "P1")},
		},
		{
			name: "CarriedParentFindingNeedsNoAsk", ctx: round(nil), prior: []PriorEntry{entry("E1", PriorOpen)},
			findings: []Finding{carrying(fromParent(finding("F1", SeverityMedium, "a.go:10"), "P1"), "E1")},
		},
		{name: "NewBlockingOnAPlanCardOutsideTheChangedFiles", ctx: round(changed("c.go")), prior: []PriorEntry{entry("E1", PriorResolved)}, findings: []Finding{finding("F1", SeverityBlocking, planCard+":3")}, wantErr: "does not list"},
		{name: "NewBlockingOnAnAmendedCardPasses", ctx: round(changedWith(amending(amendingEntry))), prior: []PriorEntry{entry("E1", PriorResolved)}, findings: []Finding{finding("F1", SeverityBlocking, planCard+":3")}},
		{name: "NewBlockingOnAnAmendedCardNamedBySlugPasses", ctx: round(changedWith(amending("Change the card-slug card"))), prior: []PriorEntry{entry("E1", PriorResolved)}, findings: []Finding{finding("F1", SeverityBlocking, planCard+":3")}},
		{name: "NewMediumOnAnAmendedCardIsStillRefused", ctx: round(changedWith(amending(amendingEntry))), prior: []PriorEntry{entry("E1", PriorResolved)}, findings: []Finding{finding("F1", SeverityMedium, planCard+":3")}, wantErr: "only at BLOCKING"},
		{name: "NewBlockingOnACardNoAmendmentNamesIsRefused", ctx: round(changedWith(amending("Amend plan card 15-other-card."))), prior: []PriorEntry{entry("E1", PriorResolved)}, findings: []Finding{finding("F1", SeverityBlocking, planCard+":3")}, wantErr: "does not list"},
		{name: "NewBlockingOnACardWhoseSlugIsInsideALongerNamedSlugIsRefused", ctx: round(changedWith(amending("Amend plan card 14-card-slug-extra."))), prior: []PriorEntry{entry("E1", PriorResolved)}, findings: []Finding{finding("F1", SeverityBlocking, "_lym/plan/15-card-slug.md:3")}, wantErr: "does not list"},
		{name: "NewBlockingOnANonCardFileSharingACardStemIsRefused", ctx: round(changedWith(amending(amendingEntry))), prior: []PriorEntry{entry("E1", PriorResolved)}, findings: []Finding{finding("F1", SeverityBlocking, "internal/card-slug.go:3")}, wantErr: "does not list"},
		{name: "NewBlockingOnThePlanOverviewNamedByAnAmendmentIsRefused", ctx: round(changedWith(amending(amendingEntry + " Revise the overview too."))), prior: []PriorEntry{entry("E1", PriorResolved)}, findings: []Finding{finding("F1", SeverityBlocking, "_lym/plan/00-overview.md:3")}, wantErr: "does not list"},
		{name: "NewBlockingOnThePlanAmendmentLogNamedByAnAmendmentIsRefused", ctx: round(changedWith(amending(amendingEntry + " Record it in amendments."))), prior: []PriorEntry{entry("E1", PriorResolved)}, findings: []Finding{finding("F1", SeverityBlocking, "_lym/plan/amendments.md:3")}, wantErr: "does not list"},
		{name: "NewBlockingOnThePlanLensReviewNamedByAnAmendmentIsRefused", ctx: round(changedWith(amending(amendingEntry + " Update lens-review."))), prior: []PriorEntry{entry("E1", PriorResolved)}, findings: []Finding{finding("F1", SeverityBlocking, "_lym/plan/lens-review.md:3")}, wantErr: "does not list"},
		{name: "SecondNewWebsterFindingUnderAHaltReasonIsRefused", ctx: round(changedWith(halted("card 14 misses the retry"))), prior: []PriorEntry{entry("E1", PriorResolved)}, findings: []Finding{fromWebster(finding("F1", SeverityBlocking, planCard+":3")), fromWebster(finding("F2", SeverityBlocking, "_lym/plan/15-other.md:3"))}, wantErr: "halt reason admits one"},
		{name: "NewBlockingOnAnAmendedCardSpelledThroughTheLinkPasses", ctx: round(changedWith(amending(amendingEntry))), prior: []PriorEntry{entry("E1", PriorResolved)}, findings: []Finding{finding("F1", SeverityBlocking, "/elsewhere/"+planCard+":3")}},
		{name: "WebsterFindingsOneOutsideAndOneInsideTheChangedFilesPass", ctx: round(changedWith(halted("card 14 misses the retry"))), prior: []PriorEntry{entry("E1", PriorResolved)}, findings: []Finding{fromWebster(finding("F1", SeverityBlocking, planCard+":3")), fromWebster(finding("F2", SeverityBlocking, "c.go:4"))}},
		{name: "NewBlockingOnAShortStemIsNotMatched", ctx: round(changedWith(amending("Rework plan card 1-a before the run."))), prior: []PriorEntry{entry("E1", PriorResolved)}, findings: []Finding{finding("F1", SeverityBlocking, "_lym/plan/1-a.md:3")}, wantErr: "does not list"},
		{name: "CarriedMovedToAnAmendedCardPasses", ctx: round(changedWith(amending(amendingEntry))), prior: []PriorEntry{entry("E1", PriorOpen)}, findings: []Finding{carrying(finding("F1", SeverityMedium, planCard+":3"), "E1")}},
		{name: "CarriedMovedToACardNoAmendmentNamesIsRefused", ctx: round(changedWith()), prior: []PriorEntry{entry("E1", PriorOpen)}, findings: []Finding{carrying(finding("F1", SeverityMedium, planCard+":3"), "E1")}, wantErr: "does not list"},
		{name: "NewWebsterFindingWithNoHaltReasonIsRefused", ctx: round(changed("c.go")), prior: []PriorEntry{entry("E1", PriorResolved)}, findings: []Finding{fromWebster(finding("F1", SeverityBlocking, planCard+":3"))}, wantErr: "does not list"},
		{name: "NewWebsterFindingWithAHaltReasonPasses", ctx: round(changedWith(halted("card 14 misses the retry"))), prior: []PriorEntry{entry("E1", PriorResolved)}, findings: []Finding{fromWebster(finding("F1", SeverityBlocking, planCard+":3"))}},
		{name: "NewWebsterFindingBelowBlockingIsRefused", ctx: round(changedWith(halted("card 14 misses the retry"))), prior: []PriorEntry{entry("E1", PriorResolved)}, findings: []Finding{fromWebster(finding("F1", SeverityMedium, planCard+":3"))}, wantErr: "only at BLOCKING"},
		{name: "NewFocusDecisionOnAnUntouchedFilePasses", ctx: round(changedWith(focusing)), prior: []PriorEntry{entry("E1", PriorResolved)}, findings: []Finding{fromFocus(finding("F1", SeverityBlocking, "z.go:1"), ClassDecision), fromFocus(finding("F2", SeverityBlocking, "y.go:1"), ClassDecision)}},
		{name: "NewFocusDecisionOnAnUntouchedFileOutsideAHardenerRoundIsRefused", ctx: round(changed("c.go")), prior: []PriorEntry{entry("E1", PriorResolved)}, findings: []Finding{fromFocus(finding("F1", SeverityBlocking, "z.go:1"), ClassDecision)}, wantErr: "does not list"},
		{name: "NewFocusFindingOfAnotherClassOnAnUntouchedFileIsRefused", ctx: round(changedWith(focusing)), prior: []PriorEntry{entry("E1", PriorResolved)}, findings: []Finding{fromFocus(finding("F1", SeverityBlocking, "z.go:1"), ClassScope)}, wantErr: "does not list"},
		{name: "NewFocusDecisionBelowBlockingIsRefused", ctx: round(changedWith(focusing)), prior: []PriorEntry{entry("E1", PriorResolved)}, findings: []Finding{fromFocus(finding("F1", SeverityMedium, "z.go:1"), ClassDecision)}, wantErr: "only at BLOCKING"},
		{
			name: "HeldPairOneResolvedAndOneCarried",
			ctx: round(func(ctx *RoundContext) {
				held("Use X")(ctx)
				ctx.Prior = append(ctx.Prior, PriorFinding{Finding: finding("E2", SeverityMedium, "b.go:5"), Held: true})
			}),
			prior:    []PriorEntry{{ID: "E1", Status: PriorResolved, Answer: "settled by Use X"}, entry("E2", PriorOpen)},
			findings: []Finding{carrying(finding("F1", SeverityMedium, "b.go:5"), "E2")},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := CheckReviewRound(marshalRoundReview(t, tt.prior, tt.findings), tt.ctx)
			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("CheckReviewRound() = %v; want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("CheckReviewRound() = %v; want an error containing %q", err, tt.wantErr)
			}
			if !strings.Contains(err.Error(), "way forward: rewrite the review file") {
				t.Errorf("CheckReviewRound() = %q; want it to end with the way forward", err)
			}
		})
	}
}
