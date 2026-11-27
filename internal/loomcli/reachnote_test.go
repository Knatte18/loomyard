// reachnote_test.go covers how a code review's reach_report note renders the gate's reach report.

package loomcli

import (
	"errors"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/gatecli"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestRenderReachReportNote(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	written := time.Date(2026, 11, 9, 20, 0, 0, 0, time.UTC)
	judged := gatecli.ReachReport{
		Written: written,
		GateKey: "darn-write",
		Functions: []gatecli.ReachFunction{
			{Package: "alpha", File: "alpha/alpha.go", Func: "One", Tests: []gatecli.ReachTest{{Package: "alpha", Test: "TestAlphaOne"}}},
		},
	}
	// unreached stands in for the HEAD match of a row whose report must be rendered without one.
	unreached := func() (bool, error) {
		t.Error("matchesHead was called; want no match for a missing report, one that judged nothing or one written in the prime")
		return false, nil
	}
	tests := []struct {
		name        string
		report      gatecli.ReachReport
		found       bool
		matchesHead func() (bool, error)
		want        string
	}{
		{
			name:        "NoReport",
			matchesHead: unreached,
			want:        "No current reach report: no gate call of this worktree has written one.",
		},
		{
			name:        "RedGateNamesTheGateKey",
			report:      gatecli.ReachReport{Written: written, GateKey: "code-burler-fix", RedGate: true, Note: "a tier of the call was red"},
			found:       true,
			matchesHead: unreached,
			want:        "No current reach report: a tier of the `code-burler-fix` gate call of 2026-11-09T20:00:00Z was red, so its report judged nothing.",
		},
		{
			name:        "JudgedNothing",
			report:      gatecli.ReachReport{Written: written, GateKey: "darn-write", Note: "the worktree has no parent branch"},
			found:       true,
			matchesHead: unreached,
			want:        "No current reach report: the report of the `darn-write` gate call of 2026-11-09T20:00:00Z judged nothing: the worktree has no parent branch.",
		},
		{
			name:        "PrimeReportRendersThePrimeReasonWithNoMatch",
			report:      gatecli.ReachReport{Written: written, GateKey: "darn-write", Prime: true},
			found:       true,
			matchesHead: unreached,
			want:        "No current reach report: the report of the `darn-write` gate call of 2026-11-09T20:00:00Z was written in the prime, which has no parent branch, so it matches no HEAD.",
		},
		{
			name:        "MatchFailed",
			report:      judged,
			found:       true,
			matchesHead: func() (bool, error) { return false, errors.New("no merge base") },
			want:        "No current reach report: the report of the `darn-write` gate call of 2026-11-09T20:00:00Z could not be matched against HEAD: no merge base.",
		},
		{
			name:        "OtherTree",
			report:      judged,
			found:       true,
			matchesHead: func() (bool, error) { return false, nil },
			want:        "No current reach report: the report of the `darn-write` gate call of 2026-11-09T20:00:00Z was written for a tree other than HEAD's.",
		},
		{
			name:        "MatchRendersTheNoteLines",
			report:      judged,
			found:       true,
			matchesHead: func() (bool, error) { return true, nil },
			want: "The reach report of the `darn-write` gate call of 2026-11-09T20:00:00Z matches HEAD's tree.\n" +
				"gate test: reach: alpha.One: TestAlphaOne",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := renderReachReportNote(tt.report, tt.found, tt.matchesHead); got != tt.want {
				t.Errorf("renderReachReportNote() = %q; want %q", got, tt.want)
			}
		})
	}
}
