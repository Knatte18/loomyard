// mergepaths_test.go is table-driven, pure-logic (Tier 1) coverage for unifyConflictPaths' mapping
// rule: warp pass-through, weft identity mapping at "." and a subpath anchor, the unmappable
// classes (outside-wired-set and both-sides-collision), sort order, and the empty-never-nil result.

package fabricengine

import "testing"

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
