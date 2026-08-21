// mergepaths_test.go is table-driven, pure-logic (Tier 1) coverage for unifyConflictPaths' mapping
// rule: warp pass-through, weft identity mapping at "." and a subpath anchor, the unmappable
// classes (outside-wired-set and both-sides-collision), sort order, and the empty-never-nil result.

package fabricengine

import (
	"os"
	"strings"
	"testing"
)

func TestMergePaths_UnifyConflictPaths(t *testing.T) {
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
			wiredNames:    []string{"_lyx"},
			wantUnified:   []string{"sub/warp-other.txt", "warp-file.txt"},
		},
		{
			name:          "weft identity mapping at anchorRel dot",
			warpConflicts: nil,
			weftConflicts: []string{"_lyx/pattern/foo.md"},
			anchorRel:     ".",
			wiredNames:    []string{"_lyx"},
			wantUnified:   []string{"_lyx/pattern/foo.md"},
		},
		{
			name:          "weft identity mapping at subpath anchor",
			warpConflicts: nil,
			weftConflicts: []string{"backend/_lyx/pattern/foo.md"},
			anchorRel:     "backend",
			wiredNames:    []string{"_lyx"},
			wantUnified:   []string{"backend/_lyx/pattern/foo.md"},
		},
		{
			// A multi-segment anchor is reachable (`lyx fabric clone --subpath apps/backend`) and is
			// the shape the separator bug bites on Windows, where ValidateAnchorRel hands this
			// function `apps\backend`. On Linux the value is already slash-form, so this row guards
			// the mapping itself rather than the conversion.
			name:          "weft identity mapping at a multi-segment subpath anchor",
			warpConflicts: nil,
			weftConflicts: []string{"apps/backend/_lyx/pattern/foo.md"},
			anchorRel:     "apps/backend",
			wiredNames:    []string{"_lyx"},
			wantUnified:   []string{"apps/backend/_lyx/pattern/foo.md"},
		},
		{
			// A backslash is an ordinary filename character on Linux, so an anchor directory really
			// can be named `weird\name` there and git reports it verbatim. This row is what pins the
			// conversion as filepath.ToSlash (identity when the OS separator is already '/') rather
			// than a blanket strings.ReplaceAll, which would rewrite the name into a component
			// boundary that does not exist and make every conflict under it unmappable.
			name:          "single anchor segment containing a backslash is not split",
			warpConflicts: nil,
			weftConflicts: []string{`weird\name/_lyx/foo.md`},
			anchorRel:     `weird\name`,
			wiredNames:    []string{"_lyx"},
			wantUnified:   []string{`weird\name/_lyx/foo.md`},
		},
		{
			name:           "weft path outside the wired set is unmappable",
			warpConflicts:  nil,
			weftConflicts:  []string{"README.md"},
			anchorRel:      ".",
			wiredNames:     []string{"_lyx"},
			wantUnified:    []string{},
			wantUnmappable: true,
		},
		{
			name:           "weft repo-root file (warp-binding record name) is unmappable",
			warpConflicts:  nil,
			weftConflicts:  []string{"warp-binding.yaml"},
			anchorRel:      ".",
			wiredNames:     []string{"_lyx"},
			wantUnified:    []string{},
			wantUnmappable: true,
		},
		{
			name:           "both-sides collision on one unified path is unmappable",
			warpConflicts:  []string{"_lyx/config.yaml"},
			weftConflicts:  []string{"_lyx/config.yaml"},
			anchorRel:      ".",
			wiredNames:     []string{"_lyx"},
			wantUnified:    []string{"_lyx/config.yaml"},
			wantUnmappable: true,
		},
		{
			name:          "merged list lexically sorted with per-side ordering destroyed",
			warpConflicts: []string{"z-warp.txt", "a-warp.txt"},
			weftConflicts: []string{"_lyx/z-weft.txt", "_lyx/a-weft.txt"},
			anchorRel:     ".",
			wiredNames:    []string{"_lyx"},
			wantUnified:   []string{"_lyx/a-weft.txt", "_lyx/z-weft.txt", "a-warp.txt", "z-warp.txt"},
		},
		{
			name:          "empty inputs yield empty non-nil result",
			warpConflicts: nil,
			weftConflicts: nil,
			anchorRel:     ".",
			wiredNames:    []string{"_lyx"},
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

// TestMergePaths_WeftPathVisibleAcrossSeparators drives weftPathVisible's separator conversion with
// an EXPLICIT separator, which is the only way this host can exercise it at all.
//
// The conversion is a no-op whenever the OS separator already is '/', so on Linux and macOS a test
// that goes through weftPathVisible cannot tell the conversion from its absence: deleting it left
// the entire hermetic and integration suite green, and the campaign has no Windows host to run the
// real thing on, two instalments and seven rounds in. Driving weftPathVisibleWithSeparator directly
// with separator '\\' reproduces the Windows spelling of anchorRel here, so both wrong
// implementations fail on this host:
//   - dropping the conversion → the WindowsMultiSegmentAnchor rows stop matching,
//   - blanket-replacing every backslash regardless of the OS separator → the PosixBackslashInName
//     rows start matching a component boundary that does not exist.
func TestMergePaths_WeftPathVisibleAcrossSeparators(t *testing.T) {
	wired := []string{"_lyx", ".lyx"}

	tests := []struct {
		name      string
		weftPath  string
		anchorRel string
		separator rune
		want      bool
	}{
		{
			// The Windows shape round 6's fix is about: ValidateAnchorRel hands back `apps\backend`
			// while git reports `apps/backend/_lyx/…`. Without the conversion the prefix is
			// `apps\backend/_lyx/`, nothing matches, and MergeIn self-aborts the whole merge with
			// *ErrUnmergeableState for a conflict squarely inside the fabric-managed tree.
			name:      "WindowsMultiSegmentAnchor_MapsUnderWiredName",
			weftPath:  "apps/backend/_lyx/pattern/foo.md",
			anchorRel: `apps\backend`,
			separator: '\\',
			want:      true,
		},
		{
			name:      "WindowsMultiSegmentAnchor_MapsUnderTheOtherWiredName",
			weftPath:  "apps/backend/.lyx/scratch/foo.md",
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
			weftPath:  "backend/_lyx/foo.md",
			anchorRel: "backend",
			separator: '\\',
			want:      true,
		},
		{
			// The mirror case that a blanket strings.ReplaceAll(anchorRel, "\\", "/") would break: on
			// a platform whose separator is '/', a backslash is an ordinary filename character and a
			// directory really can be named `weird\name`. Rewriting it into a component boundary
			// makes every conflict under that anchor unmappable.
			name:      "PosixBackslashInName_IsNotSplitIntoComponents",
			weftPath:  `weird\name/_lyx/foo.md`,
			anchorRel: `weird\name`,
			separator: '/',
			want:      true,
		},
		{
			name:      "PosixBackslashInName_SlashSpellingDoesNotMatch",
			weftPath:  "weird/name/_lyx/foo.md",
			anchorRel: `weird\name`,
			separator: '/',
			want:      false,
		},
		{
			name:      "DotAnchor_IsSeparatorIndependent",
			weftPath:  "_lyx/foo.md",
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

// TestMergePaths_WeftPathVisibleUsesTheOSSeparator pins the wiring between weftPathVisible and its
// separator-explicit form by SOURCE INSPECTION, not by calling it, and the reason is the same
// identity problem that made the original defect invisible: on this host os.PathSeparator IS '/', so
// a weftPathVisible that hardcoded '/' behaves identically to the correct one and no runtime
// assertion can separate them. Verified by sabotage rather than assumed — replacing the argument
// with a literal '/' leaves every runtime test in this package green.
//
// The conversion LOGIC, which is where the defect actually lived, is fully exercised at runtime by
// TestMergePaths_WeftPathVisibleAcrossSeparators. What is left over is one argument, and a source
// scan is the proportionate guard for it — the same posture cmd/lyx's destructive- and
// uncontained-write guards take for facts no runtime assertion on this host can reach.
func TestMergePaths_WeftPathVisibleUsesTheOSSeparator(t *testing.T) {
	source, err := os.ReadFile("mergepaths.go")
	if err != nil {
		t.Fatalf("ReadFile(mergepaths.go) error = %v", err)
	}

	const wiring = "return weftPathVisibleWithSeparator(weftPath, anchorRel, wiredNames, os.PathSeparator)"
	if !strings.Contains(string(source), wiring) {
		t.Errorf("mergepaths.go does not contain %q; weftPathVisible must pass os.PathSeparator, never a hardcoded separator — a hardcoded '/' is indistinguishable from the correct wiring at runtime on this host", wiring)
	}
}
