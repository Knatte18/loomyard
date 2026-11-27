// redundancyreport_test.go covers when a redundancy report asks for a retry and when it never matches a tree.

package gatecli

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestRedundancyReportRetries(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	judged := []RedundancyPackage{{Package: "a", Judged: true}}
	tests := []struct {
		name   string
		report RedundancyReport
		want   bool
	}{
		{"AJudgedReportWithNoShortfall", RedundancyReport{Packages: judged, Tests: []RedundancyTest{{Name: "TestA", Verdict: RedundancyUnique}}}, false},
		{"ATestNotJudgedForAnotherReason", RedundancyReport{Packages: judged, Tests: []RedundancyTest{{Name: "TestA", Verdict: RedundancyNotJudged, Reason: "kept: slow"}}}, false},
		{"ANote", RedundancyReport{Note: noteNoParent}, true},
		{"ARedGate", RedundancyReport{RedGate: true}, true},
		{"AnErrorLineAlone", RedundancyReport{Errors: []string{"coverage build of a: failed"}}, true},
		{"APackageWithoutAUsableMap", RedundancyReport{Packages: []RedundancyPackage{{Package: "a", Reason: "it has no coverage map"}}}, true},
		{"ATestTheBudgetLeftUnrun", RedundancyReport{Packages: judged, Tests: []RedundancyTest{{Name: "TestA", Verdict: RedundancyNotJudged, Reason: reasonBudgetSpent}}}, true},
		{"ATestACancelledCallLeftUnrun", RedundancyReport{Packages: judged, Tests: []RedundancyTest{{Name: "TestA", Verdict: RedundancyNotJudged, Reason: reasonCancelled}}}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.report.retries(); got != tt.want {
				t.Errorf("retries() of %+v = %v; want %v", tt.report, got, tt.want)
			}
		})
	}
}

func TestRedundancyReportMatchesNothingWhenItJudgedNothing(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name   string
		report RedundancyReport
	}{
		{"ANote", RedundancyReport{Note: noteNoParent}},
		{"ARedGate", RedundancyReport{RedGate: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got, err := RedundancyReportMatches(nil, tt.report, "HEAD"); err != nil || got {
				t.Errorf("RedundancyReportMatches(%+v) = %v, %v; want false, nil", tt.report, got, err)
			}
		})
	}
}
