// reachreport_test.go covers the note the reach report step prints from its report.

package gatecli

import (
	"fmt"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestReachReport_NoteLines(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	const path = "/gate/reach.json"
	written := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	var manyFunctions []ReachFunction
	var manyLines []string
	for i := range reachNoteLineCap + 5 {
		name := fmt.Sprintf("F%02d", i)
		manyFunctions = append(manyFunctions, ReachFunction{Package: "p", Func: name, Tests: []ReachTest{{Package: "p", Test: "TestP"}}})
		if i < reachNoteLineCap-1 {
			manyLines = append(manyLines, "gate test: reach: p."+name+": TestP")
		}
	}
	manyLines = append(manyLines, "gate test: reach: note cut; the rest is in "+path)

	tests := []struct {
		name   string
		report ReachReport
		want   []string
	}{
		{
			name: "TestsOfTheOwnPackageThenOthersWithTheCountOfTheRestAndEachMapsCommitAndAge",
			report: ReachReport{
				Written: written,
				Functions: []ReachFunction{{
					Package: "a", Func: "Run",
					Tests:     []ReachTest{{Package: "a", Test: "TestOne"}, {Package: "a", Test: "TestTwo"}, {Package: "b", Test: "TestUses"}},
					MoreTests: 2,
				}},
				Maps: []ReachMap{
					{Package: "a", Commit: "c0ffee", BuiltAt: written.Add(-2 * time.Hour)},
					{Package: "b", Commit: "beef", BuiltAt: written.Add(-30 * time.Minute), Worktree: true},
				},
			},
			want: []string{
				"gate test: reach: a.Run: TestOne, TestTwo, b.TestUses, and 2 more",
				"gate test: reach: map of a from commit c0ffee, 2h0m0s old",
				"gate test: reach: map of b from commit beef, 30m0s old (the worktree's own)",
			},
		},
		{
			name: "AnUnreachedFunctionWithItsCaveatsAndAnUnrefreshedPackage",
			report: ReachReport{
				Written: written,
				Functions: []ReachFunction{
					{Package: "a", Func: "Lone", New: true, Caveats: []string{"a tier2 test may run it unseen", "the tests of b may reach it"}},
				},
				Unrefreshed: []string{"a: not refreshed, past reach_budget_min"},
				Errors:      []string{"scan the tests of a: gone"},
			},
			want: []string{
				"gate test: reach: no mapped test reaches a.Lone (new)",
				"gate test: reach:   a tier2 test may run it unseen",
				"gate test: reach:   the tests of b may reach it",
				"gate test: reach: a: not refreshed, past reach_budget_min",
				"gate test: reach: error: scan the tests of a: gone",
			},
		},
		{
			name:   "AReportThatJudgedNothingRendersItsNoteAlone",
			report: ReachReport{Note: noteNoParent, Functions: []ReachFunction{{Package: "a", Func: "Run"}}, Unrefreshed: []string{"a: not refreshed"}},
			want:   []string{"gate test: reach report: " + noteNoParent + "; written to " + path},
		},
		{
			name:   "ADiffChangingNoFunction",
			report: ReachReport{},
			want:   []string{"gate test: reach: the diff changes no function of the module"},
		},
		{
			name:   "ANoteOverTheCapEndsNamingTheReport",
			report: ReachReport{Functions: manyFunctions},
			want:   manyLines,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := tt.report.NoteLines(path)
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("NoteLines(%q) mismatch (-want +got):\n%s", path, diff)
			}
			if len(got) > reachNoteLineCap {
				t.Errorf("NoteLines(%q) holds %d lines; want at most %d", path, len(got), reachNoteLineCap)
			}
		})
	}
}
