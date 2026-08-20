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
