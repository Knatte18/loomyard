// watches_test.go parses fixture test files written under t.TempDir and matches watched tests against changed paths.

package impactset

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestFindWatchedTests(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	const file = "dir/p/p_test.go"
	tests := []struct {
		name     string
		pkg      string
		want     []WatchedTest
		wantKind GuardScanKind
		wantLine int
	}{
		{
			name: "a marker with two patterns is found with its name, file and patterns",
			pkg:  "package p\n\n//lym:watch *.cs internal/*/x.go\nfunc TestA(t *testing.T) {}\n\nfunc TestUnmarked(t *testing.T) {}\n",
			want: []WatchedTest{{Name: "TestA", File: file, Patterns: []string{"*.cs", "internal/*/x.go"}}},
		},
		{
			name: "two watch lines on one test concatenate their patterns in order",
			pkg:  "package p\n\n// TestA watches two kinds.\n//\n//lym:watch *.cs\n//lym:watch *.sql *.go\nfunc TestA(t *testing.T) {}\n",
			want: []WatchedTest{{Name: "TestA", File: file, Patterns: []string{"*.cs", "*.sql", "*.go"}}},
		},
		{
			name: "a watch line stacked with a guard, a seam and a keep line in any order is found",
			pkg:  "package p\n\n//lym:watch *.cs\n//lym:guard\n//lym:seam p#T\n//testtiming:keep x\nfunc TestA(t *testing.T) {}\n\n//lym:guard\n//testtiming:keep x\n//lym:seam p#T\n//lym:watch *.sql\nfunc TestB(t *testing.T) {}\n",
			want: []WatchedTest{
				{Name: "TestA", File: file, Patterns: []string{"*.cs"}},
				{Name: "TestB", File: file, Patterns: []string{"*.sql"}},
			},
		},
		{
			name:     "a marker followed by prose before the func line is misplaced at its line",
			pkg:      "package p\n\n//lym:watch *.cs\n// a stray line\nfunc TestA(t *testing.T) {}\n",
			wantKind: GuardMisplaced,
			wantLine: 3,
		},
		{
			name:     "a marker inside a func body is misplaced at its line",
			pkg:      "package p\n\nfunc TestA(t *testing.T) {\n\t//lym:watch *.cs\n}\n",
			wantKind: GuardMisplaced,
			wantLine: 4,
		},
		{
			name:     "a marker above a func that is not a test is misplaced at its line",
			pkg:      "package p\n\n//lym:watch *.cs\nfunc helper() {}\n",
			wantKind: GuardMisplaced,
			wantLine: 3,
		},
		{
			name:     "a marker with no pattern is malformed at its line",
			pkg:      "package p\n\n//lym:watch\nfunc TestA(t *testing.T) {}\n",
			wantKind: WatchMalformed,
			wantLine: 3,
		},
		{
			name:     "a marker with a malformed pattern is malformed at its line",
			pkg:      "package p\n\n//lym:watch *.cs [\nfunc TestA(t *testing.T) {}\n",
			wantKind: WatchMalformed,
			wantLine: 3,
		},
		{
			name:     "a file that does not parse is a parse error",
			pkg:      "package p\n\nfunc TestA(t *testing.T) {\n",
			wantKind: GuardParse,
			wantLine: 3,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			writeTestFile(t, root, "dir/p", "p_test.go", tt.pkg)

			got, err := FindWatchedTests(root, []string{"dir/p"})
			if tt.wantKind != "" {
				var scanErr *GuardScanError
				if !errors.As(err, &scanErr) {
					t.Fatalf("FindWatchedTests() error = %#v; want a *GuardScanError", err)
				}
				if scanErr.File != file || scanErr.Kind != tt.wantKind || scanErr.Line != tt.wantLine {
					t.Errorf("FindWatchedTests() error file, kind, line = %q, %q, %d; want %q, %q, %d", scanErr.File, scanErr.Kind, scanErr.Line, file, tt.wantKind, tt.wantLine)
				}
				if tt.wantKind != GuardParse && !strings.Contains(err.Error(), fmt.Sprintf("p_test.go:%d", tt.wantLine)) {
					t.Errorf("FindWatchedTests() error = %v; want the file and line", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("FindWatchedTests() error = %v; want nil", err)
			}
			if diff := cmp.Diff(tt.want, got["dir/p"]); diff != "" {
				t.Errorf("FindWatchedTests()[dir/p] mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestUnwatchedTests(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	watched := map[string][]WatchedTest{
		"internal/x": {
			{Name: "TestB", File: "internal/x/x_test.go", Patterns: []string{"*.sql"}},
			{Name: "TestA", File: "internal/x/x_test.go", Patterns: []string{"internal/*/x.go"}},
		},
		"internal/y": {{Name: "TestC", File: "internal/y/y_test.go", Patterns: []string{"*_linux.go"}}},
	}
	tests := []struct {
		name    string
		watched map[string][]WatchedTest
		changed []string
		want    map[string][]string
	}{
		{
			name:    "no changed path leaves every watched test unwatched, sorted per directory",
			watched: watched,
			want:    map[string][]string{"internal/x": {"TestA", "TestB"}, "internal/y": {"TestC"}},
		},
		{
			name:    "a slash pattern matches the full path",
			watched: watched,
			changed: []string{"internal/z/x.go"},
			want:    map[string][]string{"internal/x": {"TestB"}, "internal/y": {"TestC"}},
		},
		{
			name:    "a slash pattern does not match a base name alone",
			watched: watched,
			changed: []string{"x.go"},
			want:    map[string][]string{"internal/x": {"TestA", "TestB"}, "internal/y": {"TestC"}},
		},
		{
			name:    "a slash pattern does not match a deeper path",
			watched: watched,
			changed: []string{"internal/z/w/x.go"},
			want:    map[string][]string{"internal/x": {"TestA", "TestB"}, "internal/y": {"TestC"}},
		},
		{
			name:    "a pattern without a slash matches the base name in any directory",
			watched: watched,
			changed: []string{"deep/er/q.sql"},
			want:    map[string][]string{"internal/x": {"TestA"}, "internal/y": {"TestC"}},
		},
		{
			name:    "a change to the test's own file counts as watching it",
			watched: watched,
			changed: []string{"internal/x/x_test.go"},
			want:    map[string][]string{"internal/y": {"TestC"}},
		},
		{
			name:    "a directory whose tests are all matched is left out",
			watched: watched,
			changed: []string{"a/b_linux.go", "internal/z/x.go", "c.sql"},
			want:    nil,
		},
		{
			name:    "no watched test gives nil",
			watched: nil,
			changed: []string{"a.go"},
			want:    nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := UnwatchedTests(tt.watched, tt.changed)
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("UnwatchedTests(%v) mismatch (-want +got):\n%s", tt.changed, diff)
			}
		})
	}
}
