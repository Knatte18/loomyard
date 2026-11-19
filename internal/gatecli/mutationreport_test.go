// mutationreport_test.go covers the summary line the mutation report step prints and the lines a review reads.

package gatecli

import (
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestMutationReport_SummaryLine(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	const path = "/gate/mutation.json"
	tests := []struct {
		name   string
		report MutationReport
		want   string
	}{
		{
			name: "AJudgedReportCountsItsMutantsBlocksAndPackages",
			report: MutationReport{
				Mutants:   MutationCounts{Run: 5, Killed: 3, Survived: 1, Unjudged: 2, NotRun: 4},
				Uncovered: []MutationUncoveredBlock{{Package: "a", File: "a.go", Function: "Spare"}, {Package: "a", File: "a.go", Function: "Other"}},
				NotJudged: []MutationNotJudged{{Package: "c", Reason: reasonNoMapToMutate}},
			},
			want: "gate test: mutation report: 5 mutants run, 3 killed, 1 survived, 2 unjudged, 2 uncovered changed blocks, 1 packages not judged; written to " + path,
		},
		{
			name:   "ErrorLinesAreCountedLast",
			report: MutationReport{Errors: []string{"read HEAD: gone"}},
			want:   "gate test: mutation report: 0 mutants run, 0 killed, 0 survived, 0 unjudged, 0 uncovered changed blocks, 0 packages not judged, 1 errors; written to " + path,
		},
		{
			name:   "AReportThatJudgedNothingPrintsItsNote",
			report: MutationReport{RedGate: true, Note: noteRedGate},
			want:   "gate test: mutation report: " + noteRedGate + "; written to " + path,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.report.SummaryLine(path); got != tt.want {
				t.Errorf("SummaryLine(%q) = %q; want %q", path, got, tt.want)
			}
		})
	}
}

// TestMutationReportMatches_RefusesAReportThatJudgedNothing pins the refusal that comes before any git read.
func TestMutationReportMatches_RefusesAReportThatJudgedNothing(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name   string
		report MutationReport
	}{
		{name: "ANote", report: MutationReport{Note: noteNoParent}},
		{name: "ARedGate", report: MutationReport{RedGate: true}},
		{name: "APrime", report: MutationReport{Prime: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if matches, err := MutationReportMatches(nil, tt.report, "HEAD"); err != nil || matches {
				t.Errorf("MutationReportMatches() = %v, %v; want false and no error", matches, err)
			}
		})
	}
}

func TestMutationReport_ReviewLines(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	const path = "/gate/mutation.json"
	survivor := func(line int, function string, killers ...string) MutationSurvivor {
		return MutationSurvivor{
			MutationMutant:  MutationMutant{File: "a/a.go", Line: line, Operator: "negate condition", Function: function},
			PossibleKillers: append([]string{}, killers...),
		}
	}
	reached := survivor(12, "Run", "b.TestUses")
	reached.MorePossibleKillers = 2
	var many []MutationSurvivor
	manyLines := []string{"mutation: 0 mutants run, 0 killed, 0 survived, 0 unjudged, 0 uncovered changed blocks, 0 packages not judged"}
	for i := range mutationReviewLineCap + 5 {
		many = append(many, survivor(i+1, "Run"))
		if i < mutationReviewLineCap-2 {
			manyLines = append(manyLines, fmt.Sprintf("mutation: survivor a/a.go:%d, negate condition in Run; no test of another package reaches its function", i+1))
		}
	}
	manyLines = append(manyLines, "mutation: list cut; the rest is in "+path)

	tests := []struct {
		name   string
		report MutationReport
		want   []string
	}{
		{
			name: "TheCountsThenEachSurvivorWithItsPossibleKillers",
			report: MutationReport{
				Mutants:   MutationCounts{Run: 3, Killed: 1, Survived: 2},
				Survivors: []MutationSurvivor{reached, survivor(20, "Stop")},
			},
			want: []string{
				"mutation: 3 mutants run, 1 killed, 2 survived, 0 unjudged, 0 uncovered changed blocks, 0 packages not judged",
				"mutation: survivor a/a.go:12, negate condition in Run; possible killers: b.TestUses, and 2 more",
				"mutation: survivor a/a.go:20, negate condition in Stop; no test of another package reaches its function",
			},
		},
		{
			name:   "AListOverTheCapEndsNamingTheReport",
			report: MutationReport{Survivors: many},
			want:   manyLines,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := tt.report.ReviewLines(path)
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("ReviewLines(%q) mismatch (-want +got):\n%s", path, diff)
			}
			if len(got) > mutationReviewLineCap {
				t.Errorf("ReviewLines(%q) holds %d lines; want at most %d", path, len(got), mutationReviewLineCap)
			}
		})
	}
}
