// redundancynote_test.go covers how a code review's redundancy_report note renders the gate's redundancy report.

package loomcli

import (
	"errors"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/gatecli"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestRenderRedundancyReportNote(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	written := time.Date(2026, 11, 9, 20, 0, 0, 0, time.UTC)
	judged := gatecli.RedundancyReport{
		Written: written,
		GateKey: "darn-write",
		Tests: []gatecli.RedundancyTest{
			{Package: "alpha", Name: "TestAlphaNew", File: "alpha/alpha_test.go", Verdict: gatecli.RedundancyContained, Covering: []string{"TestAlphaBase", "TestAlphaOther"}},
			{Package: "alpha", Name: "TestAlphaOwn", File: "alpha/alpha_test.go", Verdict: gatecli.RedundancyUnique},
			{Package: "drops", Name: "TestDropsTier2", File: "drops/drops_test.go", Verdict: gatecli.RedundancyNotJudged, Reason: "a tier2 test, which no coverage map holds"},
		},
		Errors: []string{"beta: build failed"},
	}
	// unreached stands in for the HEAD match of a row whose report must be rendered without one.
	unreached := func() (bool, error) {
		t.Error("matchesHead was called; want no match for a missing report or one that judged nothing")
		return false, nil
	}
	tests := []struct {
		name        string
		report      gatecli.RedundancyReport
		found       bool
		matchesHead func() (bool, error)
		want        string
	}{
		{
			name:        "NoReport",
			matchesHead: unreached,
			want:        "No current redundancy report: no gate call of this worktree has written one.",
		},
		{
			name:        "RedGateNamesTheGateKey",
			report:      gatecli.RedundancyReport{Written: written, GateKey: "code-burler-fix", RedGate: true, Note: "a tier of the call was red"},
			found:       true,
			matchesHead: unreached,
			want:        "No current redundancy report: a tier of the `code-burler-fix` gate call of 2026-11-09T20:00:00Z was red, so its report judged nothing.",
		},
		{
			name:        "JudgedNothing",
			report:      gatecli.RedundancyReport{Written: written, GateKey: "darn-write", Note: "the worktree has no parent branch"},
			found:       true,
			matchesHead: unreached,
			want:        "No current redundancy report: the report of the `darn-write` gate call of 2026-11-09T20:00:00Z judged nothing: the worktree has no parent branch.",
		},
		{
			name:        "MatchFailed",
			report:      judged,
			found:       true,
			matchesHead: func() (bool, error) { return false, errors.New("no merge base") },
			want:        "No current redundancy report: the report of the `darn-write` gate call of 2026-11-09T20:00:00Z could not be matched against HEAD: no merge base.",
		},
		{
			name:        "OtherTree",
			report:      judged,
			found:       true,
			matchesHead: func() (bool, error) { return false, nil },
			want:        "No current redundancy report: the report of the `darn-write` gate call of 2026-11-09T20:00:00Z was written for a tree other than HEAD's.",
		},
		{
			name:        "MatchListsEveryTestAndError",
			report:      judged,
			found:       true,
			matchesHead: func() (bool, error) { return true, nil },
			want: "The redundancy report of the `darn-write` gate call of 2026-11-09T20:00:00Z matches HEAD's tree.\n" +
				"- `alpha.TestAlphaNew` in `alpha/alpha_test.go`: contained, covered by `TestAlphaBase`, `TestAlphaOther`\n" +
				"- `alpha.TestAlphaOwn` in `alpha/alpha_test.go`: unique\n" +
				"- `drops.TestDropsTier2` in `drops/drops_test.go`: not judged, a tier2 test, which no coverage map holds\n" +
				"- error: beta: build failed",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := renderRedundancyReportNote(tt.report, tt.found, tt.matchesHead); got != tt.want {
				t.Errorf("renderRedundancyReportNote() = %q; want %q", got, tt.want)
			}
		})
	}
}
