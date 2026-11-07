// mergepaths_test.go holds table-driven, pure-logic (Tier 1) tests for unifyConflictPaths' mapping rule.
// The rows cover warp pass-through, weft identity mapping, unmappable classes, sort order, and the non-nil empty result.

package fabricengine

import (
	"os"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestMergePaths_UnifyConflictPaths(t *testing.T) {
	tiergate.Require(t, 0)
	tests := []struct {
		name           string
		warpConflicts  []string
		weftConflicts  []string
		anchorRel      string
		wiredNames     []string
		wantUnified    []string
		wantUnmappable bool
	}{
		{
			name:          "warp pass-through",
			warpConflicts: []string{"warp-file.txt", "sub/warp-other.txt"},
			weftConflicts: nil,
			anchorRel:     ".",
			wiredNames:    []string{"_lym"},
			wantUnified:   []string{"sub/warp-other.txt", "warp-file.txt"},
		},
		{
			name:          "weft identity mapping at anchorRel dot",
			warpConflicts: nil,
			weftConflicts: []string{"_lym/pattern/foo.md"},
			anchorRel:     ".",
			wiredNames:    []string{"_lym"},
			wantUnified:   []string{"_lym/pattern/foo.md"},
		},
		{
			name:          "weft identity mapping at subpath anchor",
			warpConflicts: nil,
			weftConflicts: []string{"backend/_lym/pattern/foo.md"},
			anchorRel:     "backend",
			wiredNames:    []string{"_lym"},
			wantUnified:   []string{"backend/_lym/pattern/foo.md"},
		},
		{
			// A multi-segment anchor is reachable (`lym fabric clone --subpath apps/backend`).
			// On Windows, ValidateAnchorRel hands this function `apps\backend`, so the separator conversion maps it.
			// On Linux the value is already slash-form, so this row guards the mapping itself rather than the conversion.
			name:          "weft identity mapping at a multi-segment subpath anchor",
			warpConflicts: nil,
			weftConflicts: []string{"apps/backend/_lym/pattern/foo.md"},
			anchorRel:     "apps/backend",
			wiredNames:    []string{"_lym"},
			wantUnified:   []string{"apps/backend/_lym/pattern/foo.md"},
		},
		{
			// A backslash is an ordinary filename character on Linux.
			// An anchor directory can be named `weird\name` there, and git reports it verbatim.
			// This row pins the conversion as filepath.ToSlash, which is the identity where the OS separator is '/'.
			// A blanket strings.ReplaceAll would split the name at a component boundary that does not exist.
			// That would leave every conflict under the anchor unmappable.
			name:          "single anchor segment containing a backslash is not split",
			warpConflicts: nil,
			weftConflicts: []string{`weird\name/_lym/foo.md`},
			anchorRel:     `weird\name`,
			wiredNames:    []string{"_lym"},
			wantUnified:   []string{`weird\name/_lym/foo.md`},
		},
		{
			name:           "weft path outside the wired set is unmappable",
			warpConflicts:  nil,
			weftConflicts:  []string{"README.md"},
			anchorRel:      ".",
			wiredNames:     []string{"_lym"},
			wantUnified:    []string{},
			wantUnmappable: true,
		},
		{
			name:           "weft repo-root file (warp-binding record name) is unmappable",
			warpConflicts:  nil,
			weftConflicts:  []string{"warp-binding.yaml"},
			anchorRel:      ".",
			wiredNames:     []string{"_lym"},
			wantUnified:    []string{},
			wantUnmappable: true,
		},
		{
			name:           "both-sides collision on one unified path is unmappable",
			warpConflicts:  []string{"_lym/config.yaml"},
			weftConflicts:  []string{"_lym/config.yaml"},
			anchorRel:      ".",
			wiredNames:     []string{"_lym"},
			wantUnified:    []string{"_lym/config.yaml"},
			wantUnmappable: true,
		},
		{
			name:          "merged list lexically sorted with per-side ordering destroyed",
			warpConflicts: []string{"z-warp.txt", "a-warp.txt"},
			weftConflicts: []string{"_lym/z-weft.txt", "_lym/a-weft.txt"},
			anchorRel:     ".",
			wiredNames:    []string{"_lym"},
			wantUnified:   []string{"_lym/a-weft.txt", "_lym/z-weft.txt", "a-warp.txt", "z-warp.txt"},
		},
		{
			name:          "empty inputs yield empty non-nil result",
			warpConflicts: nil,
			weftConflicts: nil,
			anchorRel:     ".",
			wiredNames:    []string{"_lym"},
			wantUnified:   []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotUnified, gotUnmappable := unifyConflictPaths(tt.warpConflicts, tt.weftConflicts, tt.anchorRel, tt.wiredNames)

			if gotUnified == nil {
				t.Fatal("unifyConflictPaths() unified = nil; want non-nil (empty slices allowed)")
			}
			if len(gotUnified) != len(tt.wantUnified) {
				t.Fatalf("unifyConflictPaths() unified = %v; want %v", gotUnified, tt.wantUnified)
			}
			for i := range tt.wantUnified {
				if gotUnified[i] != tt.wantUnified[i] {
					t.Errorf("unifyConflictPaths() unified = %v; want %v", gotUnified, tt.wantUnified)
					break
				}
			}
			if gotUnmappable != tt.wantUnmappable {
				t.Errorf("unifyConflictPaths() unmappable = %v; want %v", gotUnmappable, tt.wantUnmappable)
			}
		})
	}
}

// TestMergePaths_WeftPathVisibleAcrossSeparators checks weftPathVisible's separator conversion.
// It passes an explicit separator, because a '/' host never runs the conversion.
//
// The conversion is a no-op when the OS separator is '/'.
// On Linux and macOS, a test through weftPathVisible therefore cannot tell the conversion from its absence.
// Driving weftPathVisibleWithSeparator directly with separator '\\' reproduces the Windows spelling of anchorRel.
// Both wrong implementations then fail on this host:
//   - dropping the conversion → the WindowsMultiSegmentAnchor rows stop matching,
//   - blanket-replacing every backslash regardless of the OS separator → the PosixBackslashInName
//     rows start matching a component boundary that does not exist.
func TestMergePaths_WeftPathVisibleAcrossSeparators(t *testing.T) {
	tiergate.Require(t, 0)
	wired := []string{"_lym", ".lym"}

	tests := []struct {
		name      string
		weftPath  string
		anchorRel string
		separator rune
		want      bool
	}{
		{
			// On Windows, ValidateAnchorRel hands back `apps\backend`, while git reports `apps/backend/_lym/…`.
			// Without the conversion the prefix is `apps\backend/_lym/` and nothing matches.
			// MergeIn would then self-abort the whole merge with *ErrUnmergeableState.
			// The conflict lies inside the fabric-managed tree.
			name:      "WindowsMultiSegmentAnchor_MapsUnderWiredName",
			weftPath:  "apps/backend/_lym/pattern/foo.md",
			anchorRel: `apps\backend`,
			separator: '\\',
			want:      true,
		},
		{
			name:      "WindowsMultiSegmentAnchor_MapsUnderTheOtherWiredName",
			weftPath:  "apps/backend/.lym/scratch/foo.md",
			anchorRel: `apps\backend`,
			separator: '\\',
			want:      true,
		},
		{
			name:      "WindowsMultiSegmentAnchor_OutsideWiredSetStaysUnmapped",
			weftPath:  "apps/backend/README.md",
			anchorRel: `apps\backend`,
			separator: '\\',
			want:      false,
		},
		{
			name:      "WindowsSingleSegmentAnchor_CarriesNoSeparatorAndMapsEitherWay",
			weftPath:  "backend/_lym/foo.md",
			anchorRel: "backend",
			separator: '\\',
			want:      true,
		},
		{
			// The mirror case: a blanket strings.ReplaceAll would break this row.
			// On a platform whose separator is '/', a backslash is an ordinary filename character.
			// A directory can really be named `weird\name` there.
			// Rewriting it into a component boundary makes every conflict under that anchor unmappable.
			name:      "PosixBackslashInName_IsNotSplitIntoComponents",
			weftPath:  `weird\name/_lym/foo.md`,
			anchorRel: `weird\name`,
			separator: '/',
			want:      true,
		},
		{
			name:      "PosixBackslashInName_SlashSpellingDoesNotMatch",
			weftPath:  "weird/name/_lym/foo.md",
			anchorRel: `weird\name`,
			separator: '/',
			want:      false,
		},
		{
			name:      "DotAnchor_IsSeparatorIndependent",
			weftPath:  "_lym/foo.md",
			anchorRel: ".",
			separator: '\\',
			want:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := weftPathVisibleWithSeparator(tt.weftPath, tt.anchorRel, wired, tt.separator)
			if got != tt.want {
				t.Errorf("weftPathVisibleWithSeparator(%q, %q, %v, %q) = %v; want %v",
					tt.weftPath, tt.anchorRel, wired, string(tt.separator), got, tt.want)
			}
		})
	}
}

// TestMergePaths_WeftPathVisibleUsesTheOSSeparator checks by source inspection that the OS separator is passed.
// On this host os.PathSeparator is '/', so a runtime test cannot tell a hardcoded '/' from the OS separator.
//
// The conversion logic itself is covered at runtime; only this argument needs a source scan.
//
//testtiming:keep weftPathVisible being wired to its separator-explicit form, by source inspection; coverage of its blocks by other tests does not show an assertion of this
func TestMergePaths_WeftPathVisibleUsesTheOSSeparator(t *testing.T) {
	tiergate.Require(t, 0)
	source, err := os.ReadFile("mergepaths.go")
	if err != nil {
		t.Fatalf("ReadFile(mergepaths.go) error = %v", err)
	}

	const wiring = "return weftPathVisibleWithSeparator(weftPath, anchorRel, wiredNames, os.PathSeparator)"
	if !strings.Contains(string(source), wiring) {
		t.Errorf("mergepaths.go does not contain %q; weftPathVisible must pass os.PathSeparator, never a hardcoded separator — a hardcoded '/' is indistinguishable from the correct wiring at runtime on this host", wiring)
	}
}
