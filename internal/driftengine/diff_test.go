// diff_test.go — tier 0 unit tests for driftengine's capture diff: CompileKeyPatterns, ListCaptures and DiffCaptures.
//
// Seeds capture directories under t.TempDir(); no hub, no git spawn.

package driftengine

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// defaultTestKeyPatterns are the `name = value` and `name: value` patterns the diff tests key lines with.
var defaultTestKeyPatterns = []string{
	`^\s*(?P<key>[^=:]+?)\s*=\s*(?P<value>.*?)\s*$`,
	`^\s*(?P<key>[^=:]+?)\s*:\s*(?P<value>.*?)\s*$`,
}

// writeCaptureTree writes each file of files under a fresh directory and returns it.
func writeCaptureTree(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
	}
	return dir
}

// TestDiffCaptures pins the diff of two capture directories: the cell kinds, identity pairing, line normalisation and the case leave-out,
// and the compile refusals and listing identities the diff rests on.
func TestDiffCaptures(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name     string
		baseline map[string]string
		current  map[string]string
		patterns []string
		leaveOut []string
		want     []Cell
	}{
		{
			name:     "AddedAndRemovedCases",
			baseline: map[string]string{"kept.txt": "x = 1\n", "gone.txt": "y = 1\n"},
			current:  map[string]string{"kept.txt": "x = 1\n", "new.txt": "z = 1\n"},
			patterns: defaultTestKeyPatterns,
			want: []Cell{
				{Case: "gone.txt", Kind: CellRemovedCase},
				{Case: "new.txt", Kind: CellAddedCase},
			},
		},
		{
			name:     "ChangedKeyedValue",
			baseline: map[string]string{"a.txt": "Re = 1\n"},
			current:  map[string]string{"a.txt": "Re = 2\n"},
			patterns: defaultTestKeyPatterns,
			want:     []Cell{{Case: "a.txt", Identity: Identity{Key: "Re", N: 1}, Kind: CellChanged, Old: "1", New: "2"}},
		},
		{
			name:     "InsertedKeyedLineMovesNoIdentity",
			baseline: map[string]string{"a.txt": "A = 1\nplain\nB = 2\n"},
			current:  map[string]string{"a.txt": "A = 1\nX = 9\nplain\nB = 2\n"},
			patterns: defaultTestKeyPatterns,
			want:     []Cell{{Case: "a.txt", Identity: Identity{Key: "X", N: 1}, Kind: CellAddedLine, New: "9"}},
		},
		{
			name:     "RepeatedKeyToldApartByOccurrence",
			baseline: map[string]string{"a.txt": "T = 1\nT = 2\n"},
			current:  map[string]string{"a.txt": "T = 1\nT = 3\n"},
			patterns: defaultTestKeyPatterns,
			want:     []Cell{{Case: "a.txt", Identity: Identity{Key: "T", N: 2}, Kind: CellChanged, Old: "2", New: "3"}},
		},
		{
			name:     "KeyedOnOneSideUnkeyedOnTheOther",
			baseline: map[string]string{"a.txt": "k = 1\n"},
			current:  map[string]string{"a.txt": "k\n"},
			patterns: defaultTestKeyPatterns,
			want: []Cell{
				{Case: "a.txt", Identity: Identity{Key: "k", N: 1}, Kind: CellRemovedLine, Old: "1"},
				{Case: "a.txt", Identity: Identity{Line: 1}, Kind: CellAddedLine, New: "k"},
			},
		},
		{
			name:     "EmptyPatternListMakesEveryLineUnkeyed",
			baseline: map[string]string{"a.txt": "a = 1\n"},
			current:  map[string]string{"a.txt": "a = 2\n"},
			patterns: []string{},
			want:     []Cell{{Case: "a.txt", Identity: Identity{Line: 1}, Kind: CellChanged, Old: "a = 1", New: "a = 2"}},
		},
		{
			name:     "EmptyKeyGroupMakesTheLineUnkeyed",
			baseline: map[string]string{"a.txt": "=1\n"},
			current:  map[string]string{"a.txt": "=2\n"},
			patterns: []string{`^(?P<key>\w*)=(?P<value>.*)$`},
			want:     []Cell{{Case: "a.txt", Identity: Identity{Line: 1}, Kind: CellChanged, Old: "=1", New: "=2"}},
		},
		{
			name:     "CRLFAndLFCopiesYieldNoCell",
			baseline: map[string]string{"a.txt": "A = 1\r\nplain\r\n"},
			current:  map[string]string{"a.txt": "A = 1\nplain\n"},
			patterns: defaultTestKeyPatterns,
		},
		{
			name:     "NonCaseFileReadAsCase",
			baseline: map[string]string{"sub/readme.md": "hello\n"},
			current:  map[string]string{"sub/readme.md": "hello world\n"},
			patterns: defaultTestKeyPatterns,
			want:     []Cell{{Case: "sub/readme.md", Identity: Identity{Line: 1}, Kind: CellChanged, Old: "hello", New: "hello world"}},
		},
		{
			name:     "RepeatedHashKeyKeepsKeyAndOccurrenceApart",
			baseline: map[string]string{"a.txt": "Test #3: ok\nTest #3: ok\n"},
			current:  map[string]string{"a.txt": "Test #3: ok\nTest #3: bad\n"},
			patterns: defaultTestKeyPatterns,
			want:     []Cell{{Case: "a.txt", Identity: Identity{Key: "Test #3", N: 2}, Kind: CellChanged, Old: "ok", New: "bad"}},
		},
		{
			name:     "LeaveOutCaseYieldsNoCell",
			baseline: map[string]string{"skip.txt": "a = 1\n", "other.txt": "a = 1\n"},
			current:  map[string]string{"skip.txt": "a = 2\n", "other.txt": "a = 2\n"},
			patterns: defaultTestKeyPatterns,
			leaveOut: []string{"skip.txt"},
			want:     []Cell{{Case: "other.txt", Identity: Identity{Key: "a", N: 1}, Kind: CellChanged, Old: "1", New: "2"}},
		},
		{
			name:     "ValueKeepsTheRestOfTheLine",
			baseline: map[string]string{"cs.txt": "Re = 127069, dP = 7925.267 Pa\n"},
			current:  map[string]string{"cs.txt": "Re = 127069, dP = 8000.000 Pa\n"},
			patterns: defaultTestKeyPatterns,
			want: []Cell{{
				Case: "cs.txt", Identity: Identity{Key: "Re", N: 1}, Kind: CellChanged,
				Old: "127069, dP = 7925.267 Pa", New: "127069, dP = 8000.000 Pa",
			}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			patterns, err := CompileKeyPatterns(tt.patterns)
			if err != nil {
				t.Fatalf("CompileKeyPatterns error = %v", err)
			}
			got, err := DiffCaptures(writeCaptureTree(t, tt.baseline), writeCaptureTree(t, tt.current), patterns, tt.leaveOut)
			if err != nil {
				t.Fatalf("DiffCaptures error = %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("DiffCaptures() = %+v; want %+v", got, tt.want)
			}
		})
	}

	t.Run("CompileKeyPatternsRefusals", func(t *testing.T) {
		t.Parallel()
		refusals := []struct {
			name    string
			pattern string
			want    string
		}{
			{"NoValueGroup", `^(?P<key>\w+)$`, `no named group "value"`},
			{"NoKeyGroup", `^(?P<value>\w+)$`, `no named group "key"`},
			{"InvalidExpression", `(?P<key>`, "does not compile"},
		}
		for _, tt := range refusals {
			_, err := CompileKeyPatterns([]string{defaultTestKeyPatterns[0], tt.pattern})
			if err == nil || !strings.Contains(err.Error(), fmt.Sprintf("key pattern 1 %q", tt.pattern)) || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("%s: CompileKeyPatterns error = %v; want one naming pattern 1, its text and %q", tt.name, err, tt.want)
			}
		}
	})

	t.Run("ListCapturesWritesIdentities", func(t *testing.T) {
		t.Parallel()
		patterns, err := CompileKeyPatterns(defaultTestKeyPatterns)
		if err != nil {
			t.Fatalf("CompileKeyPatterns error = %v", err)
		}
		dir := writeCaptureTree(t, map[string]string{"b/two.txt": "A = 1\nplain\nA = 2\nB: 3\n", "a.txt": "only\n"})

		got, err := ListCaptures(dir, patterns)
		if err != nil {
			t.Fatalf("ListCaptures error = %v", err)
		}
		want := []CaseListing{
			{Case: "a.txt", Lines: []ListedLine{{Identity: Identity{Line: 1}, Text: "only", Value: "only"}}},
			{Case: "b/two.txt", Lines: []ListedLine{
				{Identity: Identity{Key: "A", N: 1}, Text: "A = 1", Value: "1"},
				{Identity: Identity{Line: 1}, Text: "plain", Value: "plain"},
				{Identity: Identity{Key: "A", N: 2}, Text: "A = 2", Value: "2"},
				{Identity: Identity{Key: "B", N: 1}, Text: "B: 3", Value: "3"},
			}},
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("ListCaptures() = %+v; want %+v", got, want)
		}
	})
}

// TestVolatile covers which cells the volatile lists drop, and that the expect agent's listing leaves out what they name under diff lines alone.
func TestVolatile(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	t.Run("WithoutCells", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name     string
			volatile Volatile
			cell     Cell
			dropped  bool
		}{
			{"AFileNameAtTheTop", Volatile{Files: []string{"_source.txt"}}, Cell{Case: "_source.txt", Kind: CellChanged}, true},
			{"AFileNameAtAnyDepth", Volatile{Files: []string{"_source.txt"}}, Cell{Case: "deep/er/_source.txt", Kind: CellAddedCase}, true},
			{"AGlobOverTheFileName", Volatile{Files: []string{"*.stamp"}}, Cell{Case: "deep/a.stamp", Kind: CellRemovedCase}, true},
			{"APathPatternOverItsOwnDirectory", Volatile{Files: []string{"meta/*.txt"}}, Cell{Case: "meta/a.txt"}, true},
			{"APathPatternLeavesAnotherDirectory", Volatile{Files: []string{"meta/*.txt"}}, Cell{Case: "other/a.txt"}, false},
			{"AnotherFile", Volatile{Files: []string{"_source.txt"}}, Cell{Case: "a.txt"}, false},
			{"AKeyInAnyCase", Volatile{Keys: []string{"captured_at"}}, Cell{Case: "a.txt", Identity: Identity{Key: "captured_at", N: 1}}, true},
			{"AnotherKey", Volatile{Keys: []string{"captured_at"}}, Cell{Case: "a.txt", Identity: Identity{Key: "x", N: 1}}, false},
			{"AnUnkeyedLineUnderAKeyList", Volatile{Keys: []string{"captured_at"}}, Cell{Case: "a.txt", Identity: Identity{Line: 1}}, false},
			{"NoList", Volatile{}, Cell{Case: "_source.txt"}, false},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				got := tt.volatile.withoutCells([]Cell{tt.cell})
				if dropped := len(got) == 0; dropped != tt.dropped {
					t.Errorf("withoutCells(%+v) = %+v; want dropped %v", tt.cell, got, tt.dropped)
				}
			})
		}
	})

	t.Run("TheListingLeavesThemOutUnderLinesAlone", func(t *testing.T) {
		t.Parallel()
		patterns, err := CompileKeyPatterns(defaultTestKeyPatterns)
		if err != nil {
			t.Fatalf("CompileKeyPatterns error = %v", err)
		}
		baseline := writeCaptureTree(t, map[string]string{"_source.txt": "commit = abc\n", "a.txt": "x = 1\ncaptured_at = now\n"})
		volatile := Volatile{Files: []string{"_source.txt"}, Keys: []string{"captured_at"}}
		tests := []struct {
			mode        string
			wantSource  bool
			wantCapture bool
		}{
			{DiffLines, false, false},
			{DiffCommand, true, false},
		}
		for _, tt := range tests {
			listing := filepath.Join(t.TempDir(), "listing.yaml")
			if err := writeListing(listing, tt.mode, baseline, patterns, volatile); err != nil {
				t.Fatalf("writeListing(%s) = %v; want nil", tt.mode, err)
			}
			data, err := os.ReadFile(listing)
			if err != nil {
				t.Fatal(err)
			}
			text := string(data)
			if strings.Contains(text, "_source.txt") != tt.wantSource || strings.Contains(text, "captured_at") != tt.wantCapture || !strings.Contains(text, "a.txt") {
				t.Errorf("listing under %s = %q; want _source.txt listed %v, captured_at listed %v and a.txt listed", tt.mode, text, tt.wantSource, tt.wantCapture)
			}
		}
	})
}
