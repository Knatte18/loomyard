//go:build tier0

// guards_test.go parses fixture test files written under t.TempDir.

package impactset

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// writeTestFile writes one fixture file into a package directory of root.
func writeTestFile(t *testing.T, root, dir, name, content string) {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(dir))
	if err := os.MkdirAll(full, 0o755); err != nil {
		t.Fatalf("MkdirAll(%s): %v", full, err)
	}
	if err := os.WriteFile(filepath.Join(full, name), []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", name, err)
	}
}

func TestFindGuardTests(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		file     string
		want     []GuardTest
		wantErr  string
		wantKind GuardScanKind
		wantLine int
	}{
		{
			name: "a marked top-level test is found",
			file: "package p\n\n// TestA scans the tree.\n//\n//lym:guard\nfunc TestA(t *testing.T) {}\n\nfunc TestUnmarked(t *testing.T) {}\n",
			want: []GuardTest{{Name: "TestA", Tier: 0}},
		},
		{
			name: "a marker stacked with a testtiming keep line is found",
			file: "package p\n\n//lym:guard\n//testtiming:keep pins the scan\nfunc TestA(t *testing.T) {}\n",
			want: []GuardTest{{Name: "TestA", Tier: 0}},
		},
		{
			name: "a guard then a seam line is found",
			file: "package p\n\n//lym:guard\n//lym:seam p#T\nfunc TestA(t *testing.T) {}\n",
			want: []GuardTest{{Name: "TestA", Tier: 0}},
		},
		{
			name: "a seam line then a guard is found",
			file: "package p\n\n//lym:seam p#T\n//lym:guard\nfunc TestA(t *testing.T) {}\n",
			want: []GuardTest{{Name: "TestA", Tier: 0}},
		},
		{
			name: "a guard in a tier0 file is found as tier 0",
			file: "//go:build tier0\n\npackage p\n\n//lym:guard\nfunc TestA(t *testing.T) {}\n",
			want: []GuardTest{{Name: "TestA", Tier: 0}},
		},
		{
			name: "a guard in a tier1 file is found as tier 1",
			file: "//go:build tier1\n\npackage p\n\n//lym:guard\nfunc TestA(t *testing.T) {}\n",
			want: []GuardTest{{Name: "TestA", Tier: 1}},
		},
		{
			name:     "a file that does not parse fails naming the file",
			file:     "package p\n\nfunc TestA(t *testing.T) {\n",
			wantErr:  "impactset: parse dir/p/p_test.go",
			wantKind: GuardParse,
			wantLine: 3,
		},
		{
			name:     "a marker with another comment line between it and the func fails with its line",
			file:     "package p\n\n//lym:guard\n// a stray line\nfunc TestA(t *testing.T) {}\n",
			wantErr:  "p_test.go:3",
			wantKind: GuardMisplaced,
			wantLine: 3,
		},
		{
			name:     "a marker above a func that is not a test fails with its line",
			file:     "package p\n\n//lym:guard\nfunc helper() {}\n",
			wantErr:  "p_test.go:3",
			wantKind: GuardMisplaced,
			wantLine: 3,
		},
		{
			name:     "a marker above a method fails with its line",
			file:     "package p\n\n//lym:guard\nfunc (s S) TestA() {}\n",
			wantErr:  "p_test.go:3",
			wantKind: GuardMisplaced,
			wantLine: 3,
		},
		{
			name:     "a marker floating in a function body fails with its line",
			file:     "package p\n\nfunc TestA(t *testing.T) {\n\t//lym:guard\n}\n",
			wantErr:  "p_test.go:4",
			wantKind: GuardMisplaced,
			wantLine: 4,
		},
		{
			name:     "a marker in a tier2 file fails with its line",
			file:     "//go:build tier2\n\npackage p\n\n//lym:guard\nfunc TestA(t *testing.T) {}\n",
			wantErr:  "p_test.go:5",
			wantKind: GuardWrongTier,
			wantLine: 5,
		},
		{
			name:     "a marker in a tier3 file with a platform term fails with its line",
			file:     "//go:build tier3 && linux\n\npackage p\n\n//lym:guard\nfunc TestA(t *testing.T) {}\n",
			wantErr:  "p_test.go:5",
			wantKind: GuardWrongTier,
			wantLine: 5,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			writeTestFile(t, root, "dir/p", "p_test.go", tt.file)

			got, err := FindGuardTests(root, []string{"dir/p"})
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("FindGuardTests() error = %v; want it to contain %q", err, tt.wantErr)
				}
				var scanErr *GuardScanError
				if !errors.As(err, &scanErr) || scanErr.File != "dir/p/p_test.go" {
					t.Fatalf("FindGuardTests() error = %#v; want a *GuardScanError with File %q", err, "dir/p/p_test.go")
				}
				if scanErr.Kind != tt.wantKind || scanErr.Line != tt.wantLine {
					t.Errorf("FindGuardTests() error kind, line = %q, %d; want %q, %d", scanErr.Kind, scanErr.Line, tt.wantKind, tt.wantLine)
				}
				return
			}
			if err != nil {
				t.Fatalf("FindGuardTests() error = %v; want nil", err)
			}
			if !slices.Equal(got["dir/p"], tt.want) {
				t.Errorf("FindGuardTests()[dir/p] = %v; want %v", got["dir/p"], tt.want)
			}
		})
	}
}

func TestFindSeamTests(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		pkg      string
		file     string
		want     []SeamTest
		wantKind GuardScanKind
		wantLine int
	}{
		{
			name: "one marker above a test is found with its unit, file and struct",
			pkg:  "package p\n\n//lym:seam p#T\nfunc TestA(t *testing.T) {}\n\nfunc TestUnmarked(t *testing.T) {}\n",
			want: []SeamTest{{Name: "TestA", Unit: "dir/p", File: "dir/p/p_test.go", Structs: []string{"p#T"}}},
		},
		{
			name: "two markers on one test come back in order",
			pkg:  "package p\n\n// TestA walks two structs.\n//\n//lym:seam q#B\n//lym:seam p#A\nfunc TestA(t *testing.T) {}\n",
			want: []SeamTest{{Name: "TestA", Unit: "dir/p", File: "dir/p/p_test.go", Structs: []string{"q#B", "p#A"}}},
		},
		{
			name: "a marker in an external test package has the _test unit",
			pkg:  "package p_test\n\n//lym:seam time#Time\nfunc TestA(t *testing.T) {}\n",
			want: []SeamTest{{Name: "TestA", Unit: "dir/p_test", File: "dir/p/p_test.go", Structs: []string{"time#Time"}}},
		},
		{
			name: "a seam line stacked with a guard and a keep line on either side is found",
			pkg:  "package p\n\n//lym:guard\n//lym:seam p#T\n//testtiming:keep pins it\nfunc TestA(t *testing.T) {}\n\n//lym:seam p#U\n//testtiming:keep pins it\n//lym:guard\nfunc TestB(t *testing.T) {}\n",
			want: []SeamTest{
				{Name: "TestA", Unit: "dir/p", File: "dir/p/p_test.go", Structs: []string{"p#T"}},
				{Name: "TestB", Unit: "dir/p", File: "dir/p/p_test.go", Structs: []string{"p#U"}},
			},
		},
		{
			name:     "a marker above a helper func is misplaced at its line",
			pkg:      "package p\n\n//lym:seam p#T\nfunc helper() {}\n",
			wantKind: GuardMisplaced,
			wantLine: 3,
		},
		{
			name:     "a marker followed by prose before the func line is misplaced at its line",
			pkg:      "package p\n\n//lym:seam p#T\n// a stray line\nfunc TestA(t *testing.T) {}\n",
			wantKind: GuardMisplaced,
			wantLine: 3,
		},
		{
			name:     "a marker with no argument is malformed",
			pkg:      "package p\n\n//lym:seam\nfunc TestA(t *testing.T) {}\n",
			wantKind: SeamMalformed,
			wantLine: 3,
		},
		{
			name:     "a marker without a hash is malformed",
			pkg:      "package p\n\n//lym:seam Env\nfunc TestA(t *testing.T) {}\n",
			wantKind: SeamMalformed,
			wantLine: 3,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			writeTestFile(t, root, "dir/p", "p_test.go", tt.pkg)

			got, err := FindSeamTests(root, []string{"dir/p"})
			if tt.wantKind != "" {
				var scanErr *GuardScanError
				if !errors.As(err, &scanErr) {
					t.Fatalf("FindSeamTests() error = %#v; want a *GuardScanError", err)
				}
				if scanErr.File != "dir/p/p_test.go" || scanErr.Kind != tt.wantKind || scanErr.Line != tt.wantLine {
					t.Errorf("FindSeamTests() error file, kind, line = %q, %q, %d; want %q, %q, %d", scanErr.File, scanErr.Kind, scanErr.Line, "dir/p/p_test.go", tt.wantKind, tt.wantLine)
				}
				if !strings.Contains(err.Error(), "way forward:") || !strings.Contains(err.Error(), fmt.Sprintf("p_test.go:%d", tt.wantLine)) {
					t.Errorf("FindSeamTests() error = %v; want the file, line and a way forward", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("FindSeamTests() error = %v; want nil", err)
			}
			if diff := cmp.Diff(tt.want, got["dir/p"]); diff != "" {
				t.Errorf("FindSeamTests()[dir/p] mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
