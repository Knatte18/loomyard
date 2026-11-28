// bouncer_verify_test.go covers the check, at a spent budget, of the BLOCKING findings a round's fixer reported fixed.
// It pins which rounds spawn the verifier, the spec it spawns, and how its verification closes ledger entries.

package shedadapters

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/agentconfig"
	"github.com/Knatte18/loomyard/internal/burlerengine"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/shedfake"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// The fixer reports and verifications the rows of TestBouncer_FixVerification write.
const (
	fixedF1Report              = "---\ndispositions:\n  - id: F1\n    disposition: fixed\n---\nfixed in one commit\n"
	fixedDifferentlyF1Report   = "---\ndispositions:\n  - id: F1\n    disposition: fixed-differently\n---\n"
	fixedF1AndM1Report         = "---\ndispositions:\n  - id: F1\n    disposition: fixed\n  - id: M1\n    disposition: fixed\n---\n"
	fixedF1DisputedF2Report    = "---\ndispositions:\n  - id: F1\n    disposition: fixed\n  - id: F2\n    disposition: disputed\n---\n"
	disputedF1Report           = "---\ndispositions:\n  - id: F1\n    disposition: disputed\n---\n"
	spawnedFalseFixedF1Report  = "---\nspawned: false\ndispositions:\n  - id: F1\n    disposition: fixed\n---\n"
	closedHard                 = "---\nround: 2\nentries:\n  - key: hard\n    status: closed\n    finding: F1\n    reason: \"commit abc1234 guards the nil map\"\n---\n"
	closedHardAsF2             = "---\nround: 2\nentries:\n  - key: hard\n    status: closed\n    finding: F2\n    reason: \"commit abc1234 guards the nil map\"\n---\n"
	closedHardWithoutFinding   = "---\nround: 2\nentries:\n  - key: hard\n    status: closed\n    reason: \"commit abc1234 guards the nil map\"\n---\n"
	closedHardOfRoundOne       = "---\nround: 1\nentries:\n  - key: hard\n    status: closed\n    finding: F1\n    reason: \"commit abc1234 guards the nil map\"\n---\n"
	closedHardOpenOther        = "---\nround: 2\nentries:\n  - key: hard\n    status: closed\n    finding: F1\n    reason: \"commit abc1234 guards the nil map\"\n  - key: other\n    status: open\n    reason: \"the second site is untouched\"\n---\n"
	openHard                   = "---\nround: 2\nentries:\n  - key: hard\n    status: open\n    reason: \"the guard misses the second site\"\n---\n"
	maybeHard                  = "---\nround: 2\nentries:\n  - key: hard\n    status: maybe\n---\n"
	emptyKeyClosed             = "---\nround: 2\nentries:\n  - key: \"\"\n    status: closed\n    finding: F1\n---\n"
	duplicateHardClosedAndOpen = "---\nround: 2\nentries:\n  - key: hard\n    status: closed\n    finding: F1\n  - key: hard\n    status: open\n---\n"
)

// TestBouncer_FixVerification covers a BlockingEscalates round at a spent budget.
// Its ledger holds an open BLOCKING entry.
// When the fixer reported that entry's finding fixed, a verifier pass checks it first.
// A closed entry mapped to a finding the fixer reported fixed is resolved in the ledger.
// The round then settles without the parent.
// An entry left open, a closed entry mapped to another finding, or a check that fails, escalates with cause budget.
// Only the keys still open reach the parent.
// A round whose fixer did not report the finding fixed, or whose fix left the target unchanged, spawns no verifier.
// A re-call over the escalation spawns no verifier either.
//
//testtiming:keep pins the fix verification's spawn, its spec and how its ruling closes ledger entries, which no other test reaches
func TestBouncer_FixVerification(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		name string
		// fixerReport is round 2's fixer report; empty writes none.
		fixerReport string
		// verification is the file the verifier writes; empty writes nothing.
		verification string
		// onDisk is a verification already on disk before the first Call.
		onDisk string
		// unchangedTarget records round 2's target as the same before and after its fix.
		unchangedTarget bool
		// noTargetIDs writes round 2's snapshot record with no target id from before or after its fix.
		noTargetIDs bool
		// decision adds a decision-class finding to the review and sets EscalateDecisions.
		decision bool
		// extra adds findings to the review beside the BLOCKING F1.
		extra []reviewFixtureFinding
		// twoKeys adds a second open BLOCKING entry, `other`, to round 2's ledger.
		twoKeys bool
		// spawnError makes the verifier's run fail.
		spawnError bool
		// blockLedgerRewrite makes the ledger's temporary sibling unwritable.
		blockLedgerRewrite bool
		wantOutcome        shedengine.Outcome
		wantCause          EscalationCause
		wantDetail         string
		wantVerifierRuns   int
		wantResolved       bool
	}{
		{name: "AConfirmedFixClosesTheEntryAndTheRoundAccepts", fixerReport: fixedF1AndM1Report, verification: closedHard,
			extra:       []reviewFixtureFinding{{"M1", "MEDIUM", "design"}},
			wantOutcome: shedengine.Done, wantVerifierRuns: 1, wantResolved: true},
		{name: "AFixedDifferentlyFindingIsVerified", fixerReport: fixedDifferentlyF1Report, verification: closedHard,
			wantOutcome: shedengine.Done, wantVerifierRuns: 1, wantResolved: true},
		{name: "AFixTheVerifierKeepsOpenEscalatesNamingIt", fixerReport: fixedF1Report, verification: openHard,
			wantOutcome: shedengine.Awaiting, wantCause: EscalationBudget, wantDetail: "BLOCKING findings open (hard)", wantVerifierRuns: 1},
		{name: "APartialCloseEscalatesNamingOnlyTheKeyLeftOpen", fixerReport: fixedF1Report, verification: closedHardOpenOther, twoKeys: true,
			wantOutcome: shedengine.Awaiting, wantCause: EscalationBudget, wantDetail: "BLOCKING findings open (other)", wantVerifierRuns: 1, wantResolved: true},
		{name: "AKeyClosedForAFindingTheFixerDisputedStaysOpen", fixerReport: fixedF1DisputedF2Report, verification: closedHardAsF2,
			extra:       []reviewFixtureFinding{{"F2", "BLOCKING", "design"}},
			wantOutcome: shedengine.Awaiting, wantCause: EscalationBudget, wantDetail: "BLOCKING findings open (hard)", wantVerifierRuns: 1},
		{name: "AVerifierThatWritesNothingEscalatesNamingTheFailedCheck", fixerReport: fixedF1Report,
			wantOutcome: shedengine.Awaiting, wantCause: EscalationBudget, wantDetail: "the check of the findings the fixer reported fixed failed", wantVerifierRuns: 1},
		{name: "AVerifierThatFailsToRunEscalatesNamingTheFailedCheck", fixerReport: fixedF1Report, spawnError: true,
			wantOutcome: shedengine.Awaiting, wantCause: EscalationBudget, wantDetail: "the check of the findings the fixer reported fixed failed", wantVerifierRuns: 1},
		{name: "AVerificationOfAnUnknownStatusEscalatesNamingTheFailedCheck", fixerReport: fixedF1Report, verification: maybeHard,
			wantOutcome: shedengine.Awaiting, wantCause: EscalationBudget, wantDetail: `status "maybe"`, wantVerifierRuns: 1},
		{name: "AVerificationWithAnEmptyKeyEscalatesNamingTheFailedCheck", fixerReport: fixedF1Report, verification: emptyKeyClosed,
			wantOutcome: shedengine.Awaiting, wantCause: EscalationBudget, wantDetail: "an empty key", wantVerifierRuns: 1},
		{name: "AVerificationListingAKeyTwiceEscalatesNamingTheFailedCheck", fixerReport: fixedF1Report, verification: duplicateHardClosedAndOpen,
			wantOutcome: shedengine.Awaiting, wantCause: EscalationBudget, wantDetail: "more than once", wantVerifierRuns: 1},
		{name: "AClosedEntryNamingNoFindingEscalatesNamingTheFailedCheck", fixerReport: fixedF1Report, verification: closedHardWithoutFinding,
			wantOutcome: shedengine.Awaiting, wantCause: EscalationBudget, wantDetail: "names no finding", wantVerifierRuns: 1},
		{name: "ALedgerThatCannotBeRewrittenEscalatesWithTheKeyOpen", fixerReport: fixedF1Report, verification: closedHard, blockLedgerRewrite: true,
			wantOutcome: shedengine.Awaiting, wantCause: EscalationBudget, wantDetail: "the verified findings could not be closed in the ledger", wantVerifierRuns: 1},
		{name: "AVerificationOnDiskIsUsedWithoutAVerifier", fixerReport: fixedF1Report, onDisk: closedHard,
			wantOutcome: shedengine.Done, wantResolved: true},
		{name: "AVerificationOnDiskOfAnotherRoundIsReplacedByAVerifierRun", fixerReport: fixedF1Report, onDisk: closedHardOfRoundOne, verification: closedHard,
			wantOutcome: shedengine.Done, wantVerifierRuns: 1, wantResolved: true},
		{name: "ADisputedFindingIsNotVerified", fixerReport: disputedF1Report,
			wantOutcome: shedengine.Awaiting, wantCause: EscalationBudget, wantDetail: "BLOCKING findings open (hard)"},
		{name: "AReportTheEngineWroteForAFixerItNeverSpawnedIsNotVerified", fixerReport: spawnedFalseFixedF1Report,
			wantOutcome: shedengine.Awaiting, wantCause: EscalationBudget, wantDetail: "BLOCKING findings open (hard)"},
		{name: "ARoundWithNoFixerReportIsNotVerified",
			wantOutcome: shedengine.Awaiting, wantCause: EscalationBudget, wantDetail: "BLOCKING findings open (hard)"},
		{name: "AFixThatLeftTheTargetUnchangedIsNotVerified", fixerReport: fixedF1Report, verification: closedHard, unchangedTarget: true,
			wantOutcome: shedengine.Awaiting, wantCause: EscalationBudget, wantDetail: "BLOCKING findings open (hard)"},
		{name: "ARecordWithNoTargetIdsIsVerified", fixerReport: fixedF1Report, verification: closedHard, noTargetIDs: true,
			wantOutcome: shedengine.Done, wantVerifierRuns: 1, wantResolved: true},
		{name: "AConfirmedFixLeavesADecisionFindingToTheDecisionEscalation", fixerReport: fixedF1Report, verification: closedHard, decision: true,
			wantOutcome: shedengine.Awaiting, wantCause: EscalationDecision, wantVerifierRuns: 1, wantResolved: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			entries := []evidenceEntry{{"alpha", "resolved", "design", "MEDIUM"}, {"hard", "open", "scope", "BLOCKING"}}
			if tt.twoKeys {
				entries = append(entries, evidenceEntry{"other", "open", "scope", "BLOCKING"})
			}
			ledger := evidenceLedgerContent(t, 2, entries...)
			shuttle := &shedfake.Shuttle{}
			shuttle.RunFn = func(spec shuttleengine.Spec) (shuttleengine.Result, error) {
				switch {
				case len(spec.OutputFiles) == 3:
					_ = os.WriteFile(spec.OutputFiles[0], []byte(bouncerVerdictContent("CONTINUE")), 0o644)
					_ = os.WriteFile(spec.OutputFiles[1], []byte(ledger), 0o644)
					_ = os.WriteFile(spec.OutputFiles[2], []byte("---\nround: 3\nexclude_lenses: []\nfocus: []\n---\n"), 0o644)
				case spec.Role == bouncerVerifyRole && tt.spawnError:
					return shuttleengine.Result{}, errors.New("the verifier could not start")
				case tt.verification != "":
					_ = os.WriteFile(spec.OutputFiles[0], []byte(tt.verification), 0o644)
				}
				return shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}, nil
			}
			cfg := newBouncerFixture(t, withNestedRunDir(), withShuttle(shuttle), withEscalationInputs()).Config
			cfg.CirclingCheckpoint = 2
			cfg.Bounces = spentBounces
			cfg.AutoAccept = true
			cfg.BlockingEscalates = true
			cfg.EscalateDecisions = tt.decision
			seams := &circlingSeams{}
			carry := &carryOverRecorder{}
			seams.install(&cfg)
			carry.install(&cfg)
			findings := append([]reviewFixtureFinding{{"F1", "BLOCKING", "design"}}, tt.extra...)
			if tt.decision {
				findings = append(findings, reviewFixtureFinding{"D1", "LOW", "decision"})
			}
			layoutBouncerRun(t, cfg, []bouncerJudgeFixture{
				{round: 1, report: bouncerReport(1), verdict: bouncerVerdictContent("CONTINUE"), ledger: bouncerLedgerContent(1)},
				{round: 2, report: decisionReviewContent(findings...)},
			})
			writeEvidenceLedger(t, cfg.RunDir, 1, evidenceEntry{"alpha", "open", "design", "MEDIUM"})
			for path, content := range map[string]string{roundFixerReportPath(cfg.RunDir, 2): tt.fixerReport, verificationPath(cfg.RunDir, 2): tt.onDisk} {
				if content == "" {
					continue
				}
				if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
					t.Fatalf("WriteFile(%s) = %v; want nil", path, err)
				}
			}
			if tt.unchangedTarget {
				if err := updateRoundSnapshot(cfg.RunDir, 2, func(rec *roundSnapshot, _ bool) { rec.TargetID, rec.PostFixTargetID = "tree-1", "tree-1" }); err != nil {
					t.Fatalf("updateRoundSnapshot() = %v; want nil", err)
				}
			}
			if tt.noTargetIDs {
				if err := updateRoundSnapshot(cfg.RunDir, 2, func(*roundSnapshot, bool) {}); err != nil {
					t.Fatalf("updateRoundSnapshot() = %v; want nil", err)
				}
			}
			if tt.blockLedgerRewrite {
				if err := os.Mkdir(ledgerPath(cfg.RunDir, 2)+".tmp", 0o755); err != nil {
					t.Fatalf("Mkdir(ledger temporary) = %v; want nil", err)
				}
			}
			b, err := NewBouncer(cfg)
			if err != nil {
				t.Fatalf("NewBouncer(...) error = %v; want nil", err)
			}

			ptr := shedfake.RequireOutcome(t, b, tt.wantOutcome)

			verifierSpecs := verifierSpecsOf(shuttle)
			if len(verifierSpecs) != tt.wantVerifierRuns {
				t.Errorf("verifier runs = %d; want %d", len(verifierSpecs), tt.wantVerifierRuns)
			}
			if len(verifierSpecs) > 0 {
				requireVerifierSpec(t, verifierSpecs[0], cfg)
			}
			got, err := readLedger(cfg.RunDir, 2)
			if err != nil {
				t.Fatalf("readLedger(round 2) = %v; want nil", err)
			}
			hardStatus := "open"
			if tt.wantResolved {
				hardStatus = "resolved"
			}
			wantLedger := ledgerFile{Round: 2, Prose: "prose", Entries: []ledgerEntry{
				{Key: "alpha", Rounds: []int{2}, Status: "resolved", Class: burlerengine.ClassDesign, Severity: burlerengine.SeverityMedium},
				{Key: "hard", Rounds: []int{2}, Status: hardStatus, Class: burlerengine.ClassScope, Severity: burlerengine.SeverityBlocking},
			}}
			if tt.twoKeys {
				wantLedger.Entries = append(wantLedger.Entries,
					ledgerEntry{Key: "other", Rounds: []int{2}, Status: "open", Class: burlerengine.ClassScope, Severity: burlerengine.SeverityBlocking})
			}
			if !reflect.DeepEqual(got, wantLedger) {
				t.Errorf("round 2 ledger = %+v; want %+v", got, wantLedger)
			}
			if tt.wantOutcome == shedengine.Done {
				if len(carry.entries) != 1 || len(carry.entries[0].Findings) != 0 {
					t.Errorf("carry-over entries = %+v; want one with no finding, the verified entry closed", carry.entries)
				}
				return
			}
			requireEscalation(t, &cfg, tt.wantCause)
			if tt.wantDetail != "" {
				if brief, err := os.ReadFile(escalationPath(cfg.RunDir, 2)); err != nil || !strings.Contains(string(brief), tt.wantDetail) {
					t.Errorf("escalation brief = %q, %v; want it to carry %q", brief, err, tt.wantDetail)
				}
			}

			again := shedfake.RequireOutcome(t, b, shedengine.Awaiting)
			if !reflect.DeepEqual(again, ptr) {
				t.Errorf("second Call() pointer = %+v; want %+v", again, ptr)
			}
			if runs := len(verifierSpecsOf(shuttle)); runs != tt.wantVerifierRuns {
				t.Errorf("verifier runs after the re-call = %d; want %d", runs, tt.wantVerifierRuns)
			}
		})
	}
}

// verifierSpecsOf returns the specs of the verifier passes shuttle ran, in order.
func verifierSpecsOf(shuttle *shedfake.Shuttle) []shuttleengine.Spec {
	var specs []shuttleengine.Spec
	for _, spec := range shuttle.Specs {
		if spec.Role == bouncerVerifyRole {
			specs = append(specs, spec)
		}
	}
	return specs
}

// requireVerifierSpec fails the test unless spec is round 2's verifier pass on the judge's model and config role.
// Its prompt must name the fixed finding, the open ledger key, the fixer report and the output file.
// It must not list a finding the fixer did not report fixed at severity BLOCKING.
func requireVerifierSpec(t *testing.T, spec shuttleengine.Spec, cfg BouncerConfig) {
	t.Helper()
	if want := []string{verificationPath(cfg.RunDir, 2)}; !slices.Equal(spec.OutputFiles, want) {
		t.Errorf("verifier OutputFiles = %v; want %v", spec.OutputFiles, want)
	}
	if spec.ModelChoice != cfg.Models.Judge || spec.ConfigRole != agentconfig.RoleBouncerJudge || spec.Round != "2" {
		t.Errorf("verifier spec = model %+v, config role %q, round %q; want the judge's model, %q and round 2", spec.ModelChoice, spec.ConfigRole, spec.Round, agentconfig.RoleBouncerJudge)
	}
	for _, want := range []string{"`F1` (BLOCKING)", "- `hard`", roundFixerReportPath(cfg.RunDir, 2), filepath.Base(verificationPath(cfg.RunDir, 2))} {
		if !strings.Contains(spec.Prompt, want) {
			t.Errorf("verifier prompt lacks %q", want)
		}
	}
	for _, unwanted := range []string{"`M1`", "`F2`"} {
		if strings.Contains(spec.Prompt, unwanted) {
			t.Errorf("verifier prompt carries %q; want only the BLOCKING findings the fixer reported fixed", unwanted)
		}
	}
}
