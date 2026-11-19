// mutationnote_test.go covers how a code review's mutation_report note renders the gate's mutation report.

package loomcli

import (
	"errors"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/gatecli"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestRenderMutationReportNote(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	written := time.Date(2026, 11, 9, 20, 0, 0, 0, time.UTC)
	judged := gatecli.MutationReport{
		Written: written,
		GateKey: "darn-write",
		Mutants: gatecli.MutationCounts{Run: 2, Killed: 1, Survived: 1},
		Survivors: []gatecli.MutationSurvivor{{
			MutationMutant:  gatecli.MutationMutant{File: "alpha/alpha.go", Line: 7, Operator: "negate condition", Function: "One"},
			PossibleKillers: []string{},
		}},
	}
	// unmatched stands in for the HEAD match of a row whose report must be rendered without one.
	unmatched := func() (bool, error) {
		t.Error("matchesHead was called; want no match for a missing report, a red-gate one or one written in the prime")
		return false, nil
	}
	tests := []struct {
		name        string
		report      gatecli.MutationReport
		found       bool
		matchesHead func() (bool, error)
		want        string
	}{
		{
			name:        "NoReport",
			matchesHead: unmatched,
			want:        "No current mutation report: no gate call of this worktree has written one.",
		},
		{
			name:        "RedGateNamesTheGateKey",
			report:      gatecli.MutationReport{Written: written, GateKey: "code-burler-fix", RedGate: true, Note: "a tier of the call was red"},
			found:       true,
			matchesHead: unmatched,
			want:        "No current mutation report: a tier of the `code-burler-fix` gate call of 2026-11-09T20:00:00Z was red, so its report judged nothing.",
		},
		{
			name:        "PrimeReportRendersThePrimeReasonWithNoMatch",
			report:      gatecli.MutationReport{Written: written, GateKey: "darn-write", Prime: true},
			found:       true,
			matchesHead: unmatched,
			want:        "No current mutation report: the report of the `darn-write` gate call of 2026-11-09T20:00:00Z was written in the prime, which has no parent branch, so it matches no HEAD.",
		},
		{
			name:        "ANoteOtherThanARedGateRendersTheJudgedNothingLine",
			report:      gatecli.MutationReport{Written: written, GateKey: "darn-write", Note: "the worktree has no parent branch"},
			found:       true,
			matchesHead: unmatched,
			want:        "No current mutation report: the report of the `darn-write` gate call of 2026-11-09T20:00:00Z judged nothing: the worktree has no parent branch.",
		},
		{
			name:        "AHeadMatchErrorIsNamed",
			report:      judged,
			found:       true,
			matchesHead: func() (bool, error) { return false, errors.New("read HEAD: gone") },
			want:        "No current mutation report: the report of the `darn-write` gate call of 2026-11-09T20:00:00Z could not be matched against HEAD: read HEAD: gone.",
		},
		{
			name:        "OtherTree",
			report:      judged,
			found:       true,
			matchesHead: func() (bool, error) { return false, nil },
			want:        "No current mutation report: the report of the `darn-write` gate call of 2026-11-09T20:00:00Z was written for a tree other than HEAD's.",
		},
		{
			name:        "MatchRendersTheCountsAndSurvivors",
			report:      judged,
			found:       true,
			matchesHead: func() (bool, error) { return true, nil },
			want: "The mutation report of the `darn-write` gate call of 2026-11-09T20:00:00Z matches HEAD's tree.\n" +
				"mutation: 2 mutants run, 1 killed, 1 survived, 0 unjudged, 0 uncovered changed blocks, 0 packages not judged\n" +
				"mutation: survivor alpha/alpha.go:7, negate condition in One; no test of another package reaches its function",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := renderMutationReportNote(tt.report, tt.found, tt.matchesHead); got != tt.want {
				t.Errorf("renderMutationReportNote() = %q; want %q", got, tt.want)
			}
		})
	}
}
