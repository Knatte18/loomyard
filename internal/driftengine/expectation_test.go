// expectation_test.go — tier 0 tests for the expectation parser and for which cells an expectation covers.

package driftengine

import (
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// expectationBaseline is a baseline with a keyed case holding `Test #3` twice and an unkeyed case.
func expectationBaseline() []CaseListing {
	return []CaseListing{
		{Case: "keyed.txt", Lines: []ListedLine{
			{Identity: Identity{Key: "Test #3", N: 1}},
			{Identity: Identity{Key: "Test #3", N: 2}},
			{Identity: Identity{Key: "other", N: 1}},
		}},
		{Case: "plain.txt", Lines: []ListedLine{{Identity: Identity{Line: 1}}, {Identity: Identity{Line: 2}}}},
		{Case: "gone.txt"},
	}
}

// TestParseExpectation covers the shapes both modes accept and each refusal, naming the rule it must state.
func TestParseExpectation(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name    string
		mode    string
		data    string
		want    Expectation
		wantErr string
	}{
		{name: "EmptyFile", mode: DiffLines, data: "", want: Expectation{}},
		{name: "EmptyDocument", mode: DiffLines, data: "{}\n", want: Expectation{}},
		{
			name: "KeyWithAndWithoutN",
			mode: DiffLines,
			data: "cases:\n  keyed.txt:\n    keys:\n      - {key: 'Test #3', why: all moved}\n      - {key: 'Test #3', n: 2, why: second only}\n",
			want: Expectation{Cases: map[string]ExpectedCase{"keyed.txt": {Keys: []KeyEntry{
				{Key: "Test #3", Why: "all moved"},
				{Key: "Test #3", N: 2, Why: "second only"},
			}}}},
		},
		{
			name: "ClosedAndOpenRanges",
			mode: DiffLines,
			data: "cases:\n  plain.txt:\n    lines:\n      - {range: '1', why: a}\n      - {range: 1-2, why: b}\n      - {range: 2-, why: c}\n",
			want: Expectation{Cases: map[string]ExpectedCase{"plain.txt": {Lines: []LineEntry{
				{Range: "1", Why: "a"}, {Range: "1-2", Why: "b"}, {Range: "2-", Why: "c"},
			}}}},
		},
		{
			name: "AddedKeyAddedCaseAndRemovedCase",
			mode: DiffLines,
			data: "added:\n  - {case: new.txt, why: new file}\nremoved:\n  - {case: gone.txt, why: dropped}\ncases:\n  new.txt:\n    keys:\n      - {key: fresh, added: true, why: new key}\n  keyed.txt:\n    keys:\n      - {key: extra, added: true, why: new key}\n",
			want: Expectation{
				Added:   []CaseEntry{{Case: "new.txt", Why: "new file"}},
				Removed: []CaseEntry{{Case: "gone.txt", Why: "dropped"}},
				Cases: map[string]ExpectedCase{
					"new.txt":   {Keys: []KeyEntry{{Key: "fresh", Added: true, Why: "new key"}}},
					"keyed.txt": {Keys: []KeyEntry{{Key: "extra", Added: true, Why: "new key"}}},
				},
			},
		},
		{name: "UnknownCase", mode: DiffLines, data: "cases:\n  nope.txt:\n    lines:\n      - {range: 1, why: a}\n", wantErr: `case "nope.txt" is neither a case of the baseline nor listed under added`},
		{name: "RemovedCaseMissingFromBaseline", mode: DiffLines, data: "removed:\n  - {case: nope.txt, why: a}\n", wantErr: `removed case "nope.txt" is not a case of the baseline`},
		{name: "UnknownKeyNotMarkedAdded", mode: DiffLines, data: "cases:\n  keyed.txt:\n    keys:\n      - {key: missing, why: a}\n", wantErr: `keys entry "missing" is not a key of the baseline's case`},
		{name: "OccurrenceBeyondBaseline", mode: DiffLines, data: "cases:\n  keyed.txt:\n    keys:\n      - {key: other, n: 2, why: a}\n", wantErr: `names occurrence 2, but the baseline holds 1`},
		{name: "MalformedRange", mode: DiffLines, data: "cases:\n  plain.txt:\n    lines:\n      - {range: 3-1, why: a}\n", wantErr: `range "3-1"`},
		{name: "RangeBelowOne", mode: DiffLines, data: "cases:\n  plain.txt:\n    lines:\n      - {range: 0-2, why: a}\n", wantErr: `range "0-2"`},
		{name: "EmptyWhy", mode: DiffLines, data: "cases:\n  plain.txt:\n    lines:\n      - {range: 1, why: '  '}\n", wantErr: "has an empty why"},
		{name: "OccurrenceBelowOne", mode: DiffLines, data: "cases:\n  keyed.txt:\n    keys:\n      - {key: other, n: 0, why: a}\n", wantErr: "is below 1"},
		{name: "CaseLevelWhyUnderLines", mode: DiffLines, data: "cases:\n  plain.txt:\n    why: whole file\n", wantErr: "case-level why"},
		{name: "UnknownField", mode: DiffLines, data: "cases:\n  plain.txt:\n    lines:\n      - {range: 1, why: a, extra: b}\n", wantErr: "extra"},
		{name: "UnknownTopLevelField", mode: DiffLines, data: "extra: 1\n", wantErr: "extra"},
		{name: "CommandCaseAbsentFromBaseline", mode: DiffCommand, data: "cases:\n  anything:\n    why: compare differs\n", want: Expectation{Cases: map[string]ExpectedCase{"anything": {Why: "compare differs"}}}},
		{name: "CommandKeysEntry", mode: DiffCommand, data: "cases:\n  anything:\n    keys:\n      - {key: k, why: a}\n", wantErr: "holds keys or lines"},
		{name: "CommandLinesEntry", mode: DiffCommand, data: "cases:\n  anything:\n    lines:\n      - {range: 1, why: a}\n", wantErr: "holds keys or lines"},
		{name: "CommandEmptyWhy", mode: DiffCommand, data: "cases:\n  anything:\n    why: ''\n", wantErr: "has an empty why"},
		{name: "UnknownMode", mode: "other", data: "", wantErr: `mode "other"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := ParseExpectation([]byte(tt.data), tt.mode, expectationBaseline())
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("ParseExpectation() error = %v; want it to contain %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseExpectation() error = %v; want nil", err)
			}
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("ParseExpectation() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// TestExpectationCover covers which entry covers each kind of cell.
// It also covers the merge of an amendment and the entries that cover no cell.
func TestExpectationCover(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	expectation := Expectation{
		Added:   []CaseEntry{{Case: "new.txt", Why: "new file"}},
		Removed: []CaseEntry{{Case: "gone.txt", Why: "dropped"}},
		Cases: map[string]ExpectedCase{
			"keyed.txt": {Keys: []KeyEntry{
				{Key: "all", Why: "every occurrence"},
				{Key: "second", N: 2, Why: "second only"},
			}},
			"plain.txt": {Lines: []LineEntry{
				{Range: "2-3", Why: "closed"},
				{Range: "10-", Why: "open"},
			}},
		},
	}
	keyed := func(key string, n int) Cell {
		return Cell{Case: "keyed.txt", Identity: Identity{Key: key, N: n}, Kind: CellChanged}
	}
	unkeyed := func(line int) Cell {
		return Cell{Case: "plain.txt", Identity: Identity{Line: line}, Kind: CellChanged}
	}

	tests := []struct {
		name string
		cell Cell
		want Coverage
		ok   bool
	}{
		{"KeyWithoutNCoversEveryOccurrence", keyed("all", 3), Coverage{Why: "every occurrence"}, true},
		{"KeyWithNCoversThatOccurrence", keyed("second", 2), Coverage{Why: "second only"}, true},
		{"KeyWithNSkipsOtherOccurrence", keyed("second", 1), Coverage{}, false},
		{"LineInClosedRange", unkeyed(3), Coverage{Why: "closed"}, true},
		{"LineOutsideRange", unkeyed(4), Coverage{}, false},
		{"LineInOpenRange", unkeyed(500), Coverage{Why: "open", Open: true}, true},
		{"LineEntryNeverCoversKeyedCell", Cell{Case: "plain.txt", Identity: Identity{Key: "k", N: 1}, Kind: CellChanged}, Coverage{}, false},
		{"AddedCase", Cell{Case: "new.txt", Kind: CellAddedCase}, Coverage{Why: "new file"}, true},
		{"RemovedCase", Cell{Case: "gone.txt", Kind: CellRemovedCase}, Coverage{Why: "dropped"}, true},
		{"UnlistedAddedCase", Cell{Case: "other.txt", Kind: CellAddedCase}, Coverage{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, ok := expectation.Cover(tt.cell)
			if ok != tt.ok || got != tt.want {
				t.Errorf("Cover(%+v) = %+v, %v; want %+v, %v", tt.cell, got, ok, tt.want, tt.ok)
			}
		})
	}

	t.Run("MergeLetsAnAmendmentCover", func(t *testing.T) {
		t.Parallel()
		cell := keyed("amended", 1)
		if _, ok := expectation.Cover(cell); ok {
			t.Fatalf("Cover(%+v) before the merge = true; want false", cell)
		}
		amendment := Expectation{Cases: map[string]ExpectedCase{"keyed.txt": {Keys: []KeyEntry{{Key: "amended", Why: "later"}}}}}
		merged := expectation.Merge(amendment)
		if got, ok := merged.Cover(cell); !ok || got.Why != "later" {
			t.Errorf("merged Cover(%+v) = %+v, %v; want the amendment's entry", cell, got, ok)
		}
		if got, ok := merged.Cover(keyed("all", 1)); !ok || got.Why != "every occurrence" {
			t.Errorf("merged Cover() lost the sealed entry: %+v, %v", got, ok)
		}
		if _, ok := expectation.Cover(cell); ok {
			t.Errorf("Merge changed its receiver")
		}
	})

	t.Run("UncoveringListsEntriesWithNoCell", func(t *testing.T) {
		t.Parallel()
		cells := []Cell{keyed("all", 1), unkeyed(2), {Case: "new.txt", Kind: CellAddedCase}}
		want := []string{
			`removed case "gone.txt"`,
			`key "second" (n 2) of case "keyed.txt"`,
			`lines 10- of case "plain.txt"`,
		}
		if diff := cmp.Diff(want, expectation.Uncovering(cells)); diff != "" {
			t.Errorf("Uncovering() mismatch (-want +got):\n%s", diff)
		}
	})
}
