// judgement_test.go — tier 0 tests for the judgement parser.

package driftengine

import (
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestParseJudgement covers a valid file in each mode and each refusal, naming what the refusal must say.
func TestParseJudgement(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	lines := JudgementWant{
		Mode:       DiffLines,
		SpawnID:    "spawn-1",
		CellIDs:    []string{"c1", "c2"},
		OpenRanges: []OpenRangeNote{{Case: "a.txt", Range: "5-"}},
	}
	command := JudgementWant{Mode: DiffCommand, SpawnID: "spawn-1", CompareFailed: true}
	validLines := "spawn_id: spawn-1\ncells:\n  - {id: c1, ruling: matches, reason: fits}\n  - {id: c2, ruling: does not match, reason: wrong value}\nopen_ranges:\n  - {case: a.txt, range: 5-, checked: every later line}\n"

	tests := []struct {
		name    string
		want    JudgementWant
		data    string
		parsed  Judgement
		wantErr string
	}{
		{
			name: "ValidLines", want: lines, data: validLines,
			parsed: Judgement{
				SpawnID: "spawn-1",
				Cells: []CellRuling{
					{ID: "c1", Ruling: RulingMatches, Reason: "fits"},
					{ID: "c2", Ruling: RulingNoMatch, Reason: "wrong value"},
				},
				OpenRanges: []OpenRangeNote{{Case: "a.txt", Range: "5-", Checked: "every later line"}},
			},
		},
		{
			name: "ValidCommand", want: command,
			data: "spawn_id: spawn-1\ncases:\n  - {case: a, ruling: unexpected, reason: new failure}\n  - {case: b, ruling: matches, reason: expected}\n",
			parsed: Judgement{SpawnID: "spawn-1", Cases: []CaseRuling{
				{Case: "a", Ruling: RulingUnexpected, Reason: "new failure"},
				{Case: "b", Ruling: RulingMatches, Reason: "expected"},
			}},
		},
		{name: "ValidCommandWithPassingCompareAndNoCases", want: JudgementWant{Mode: DiffCommand, SpawnID: "spawn-1"}, data: "spawn_id: spawn-1\n", parsed: Judgement{SpawnID: "spawn-1"}},
		{name: "MissingCellID", want: lines, data: "spawn_id: spawn-1\ncells:\n  - {id: c1, ruling: matches, reason: fits}\nopen_ranges:\n  - {case: a.txt, range: 5-, checked: x}\n", wantErr: `cell "c2" has no ruling`},
		{name: "DuplicateCellID", want: lines, data: strings.Replace(validLines, "  - {id: c2, ruling: does not match, reason: wrong value}\n", "  - {id: c1, ruling: matches, reason: again}\n  - {id: c2, ruling: matches, reason: ok}\n", 1), wantErr: `cell "c1" is ruled on more than once`},
		{name: "UnknownCellID", want: lines, data: strings.Replace(validLines, "id: c2", "id: c9", 1), wantErr: `cell "c9" is not a cell of this spawn`},
		{name: "MissingOpenRange", want: lines, data: "spawn_id: spawn-1\ncells:\n  - {id: c1, ruling: matches, reason: fits}\n  - {id: c2, ruling: matches, reason: fits}\n", wantErr: `open range 5- of case "a.txt" covers a judged cell but open_ranges does not list it`},
		{name: "RulingOutsideLinesSet", want: lines, data: strings.Replace(validLines, "ruling: matches", "ruling: unexpected", 1), wantErr: `cell "c1" has ruling "unexpected"`},
		{name: "EmptyReasonUnderLines", want: lines, data: strings.Replace(validLines, "reason: fits", "reason: ''", 1), wantErr: `cell "c1" has an empty reason`},
		{name: "CasesListUnderLines", want: lines, data: validLines + "cases:\n  - {case: a, ruling: matches, reason: x}\n", wantErr: "a cases list belongs to diff"},
		{name: "UnknownField", want: lines, data: validLines + "extra: 1\n", wantErr: "extra"},
		{name: "OtherSpawnID", want: lines, data: strings.Replace(validLines, "spawn-1", "spawn-0", 1), wantErr: `spawn_id "spawn-0" is not this spawn's "spawn-1"`},
		{name: "EmptyListAfterFailingCompare", want: command, data: "spawn_id: spawn-1\ncases: []\n", wantErr: "the compare failed, so cases must rule on at least one case"},
		{name: "RulingOutsideCommandSet", want: command, data: "spawn_id: spawn-1\ncases:\n  - {case: a, ruling: maybe, reason: x}\n", wantErr: `case "a" has ruling "maybe"`},
		{name: "EmptyReasonUnderCommand", want: command, data: "spawn_id: spawn-1\ncases:\n  - {case: a, ruling: matches, reason: ' '}\n", wantErr: `case "a" has an empty reason`},
		{name: "RepeatedCaseUnderCommand", want: command, data: "spawn_id: spawn-1\ncases:\n  - {case: a, ruling: matches, reason: x}\n  - {case: a, ruling: unexpected, reason: y}\n", wantErr: `case "a" is ruled on more than once`},
		{name: "CellsListUnderCommand", want: command, data: "spawn_id: spawn-1\ncells:\n  - {id: c1, ruling: matches, reason: x}\ncases:\n  - {case: a, ruling: matches, reason: x}\n", wantErr: "a cells or open_ranges list belongs to diff"},
		{name: "UnknownMode", want: JudgementWant{Mode: "other"}, data: "", wantErr: `mode "other"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := ParseJudgement([]byte(tt.data), tt.want)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("ParseJudgement() error = %v; want it to contain %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseJudgement() error = %v; want nil", err)
			}
			if diff := cmp.Diff(tt.parsed, got); diff != "" {
				t.Errorf("ParseJudgement() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
