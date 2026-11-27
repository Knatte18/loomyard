// only_test.go parses fixture test files written under t.TempDir for their `//lym:only` markers.

package impactset

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestFindOnlyTests(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	const file = "dir/p/p_test.go"
	const tierImport = "import \"github.com/Knatte18/loomyard/internal/testkit/tiergate\"\n\n"
	tests := []struct {
		name     string
		pkg      string
		want     []OnlyTest
		wantKind GuardScanKind
		wantLine int
	}{
		{
			name: "a marker's keys are found sorted with the test's name and its file's tier",
			pkg:  "package p\n\n" + tierImport + "//lym:only upkeep finalize\nfunc TestA(t *testing.T) {\n\ttiergate.Require(t, 1)\n}\n\nfunc TestUnmarked(t *testing.T) {\n\ttiergate.Require(t, 1)\n}\n",
			want: []OnlyTest{{Name: "TestA", Tier: 1, Keys: []string{"finalize", "upkeep"}}},
		},
		{
			name: "two only lines on one test join their keys without repeats",
			pkg:  "package p\n\n// TestA runs at two keys.\n//\n//lym:only finalize\n//lym:only parent-align finalize\nfunc TestA(t *testing.T) {}\n",
			want: []OnlyTest{{Name: "TestA", Tier: 0, Keys: []string{"finalize", "parent-align"}}},
		},
		{
			name: "an only line stacked with guard, watch and keep lines is found",
			pkg:  "package p\n\n//lym:guard\n//lym:only upkeep\n//testtiming:keep x\n//lym:watch *.go\nfunc TestA(t *testing.T) {}\n",
			want: []OnlyTest{{Name: "TestA", Tier: 0, Keys: []string{"upkeep"}}},
		},
		{
			name:     "a marker followed by prose before the func line is misplaced at its line",
			pkg:      "package p\n\n//lym:only upkeep\n// a stray line\nfunc TestA(t *testing.T) {}\n",
			wantKind: GuardMisplaced,
			wantLine: 3,
		},
		{
			name:     "a marker above a func that is not a test is misplaced at its line",
			pkg:      "package p\n\n//lym:only upkeep\nfunc helper() {}\n",
			wantKind: GuardMisplaced,
			wantLine: 3,
		},
		{
			name:     "a marker naming no key is malformed at its line",
			pkg:      "package p\n\n//lym:only\nfunc TestA(t *testing.T) {}\n",
			wantKind: OnlyMalformed,
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

			got, err := FindOnlyTests(root, []string{"dir/p"})
			if tt.wantKind != "" {
				var scanErr *GuardScanError
				if !errors.As(err, &scanErr) {
					t.Fatalf("FindOnlyTests() error = %#v; want a *GuardScanError", err)
				}
				if scanErr.File != file || scanErr.Kind != tt.wantKind || scanErr.Line != tt.wantLine {
					t.Errorf("FindOnlyTests() error file, kind, line = %q, %q, %d; want %q, %q, %d", scanErr.File, scanErr.Kind, scanErr.Line, file, tt.wantKind, tt.wantLine)
				}
				if tt.wantKind != GuardParse && !strings.Contains(err.Error(), fmt.Sprintf("p_test.go:%d", tt.wantLine)) {
					t.Errorf("FindOnlyTests() error = %v; want the file and line", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("FindOnlyTests() error = %v; want nil", err)
			}
			if diff := cmp.Diff(tt.want, got["dir/p"]); diff != "" {
				t.Errorf("FindOnlyTests()[dir/p] mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
