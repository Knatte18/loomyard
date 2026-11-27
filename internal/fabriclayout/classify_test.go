// classify_test.go table-tests ScopedPathspec and ClassifyPaths.
// ClassifyPaths sorts paths into the warp, the weft and the never-committed bucket.

package fabriclayout

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/Knatte18/loomyard/internal/lymdirs"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestScopedPathspec covers a root relPath, which joins nothing, and a nested relPath, which prefixes each entry.
//
//testtiming:keep ScopedPathspec returning the entries unchanged at the root and prefixed by a nested relPath; coverage of its blocks by other tests does not show an assertion of this
func TestScopedPathspec(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		name    string
		relPath string
		dirs    []string
		want    []string
	}{
		{"root", ".", []string{"_lym"}, []string{"_lym"}},
		{"nested", "sub", []string{"_lym"}, []string{filepath.Join("sub", "_lym")}},
		{"nested_multiple_dirs", "sub", []string{"_lym", "_extra"}, []string{filepath.Join("sub", "_lym"), filepath.Join("sub", "_extra")}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := ScopedPathspec(tt.relPath, tt.dirs); !slices.Equal(got, tt.want) {
				t.Errorf("ScopedPathspec(%q, %v) = %v; want %v", tt.relPath, tt.dirs, got, tt.want)
			}
		})
	}
}

// TestClassifyPaths covers relPath "." and "sub" scoping and the _lymfoo/.lymfoo segment-boundary cases.
// It also covers empty input, all-warp and all-weft inputs, and a routingNames set with more than two entries;
// the never-committed bucket must place a path in neither weft nor warp.
//
//testtiming:keep ClassifyPaths' routing buckets, including segment-boundary names and the never-committed bucket landing in neither side; coverage of its blocks by other tests does not show an assertion of this
func TestClassifyPaths(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		name                string
		relPath             string
		routingNames        []string
		neverCommittedNames []string
		files               []string
		wantWarp            []string
		wantWeft            []string
		wantNeverCommitted  []string
	}{
		{
			name:         "under_lym_is_weft",
			relPath:      ".",
			routingNames: []string{"_lym", "_extra"},
			files:        []string{"_lym/module/state.json"},
			wantWarp:     nil,
			wantWeft:     []string{"_lym/module/state.json"},
		},
		{
			name:         "under_extra_is_weft",
			relPath:      ".",
			routingNames: []string{"_lym", "_extra"},
			files:        []string{"_extra/notes.md"},
			wantWarp:     nil,
			wantWeft:     []string{"_extra/notes.md"},
		},
		{
			name:         "warp_source_path_is_warp",
			relPath:      ".",
			routingNames: []string{"_lym", "_extra"},
			files:        []string{"cmd/lym/main.go"},
			wantWarp:     []string{"cmd/lym/main.go"},
			wantWeft:     nil,
		},
		{
			name:         "relpath_dot_scopes_prefix_at_root",
			relPath:      ".",
			routingNames: []string{"_lym"},
			files:        []string{"_lym/state.json", "sub/_lym/state.json"},
			wantWarp:     []string{"sub/_lym/state.json"},
			wantWeft:     []string{"_lym/state.json"},
		},
		{
			name:         "relpath_sub_scopes_prefix_under_sub",
			relPath:      "sub",
			routingNames: []string{"_lym"},
			files:        []string{"sub/_lym/state.json", "_lym/state.json"},
			wantWarp:     []string{"_lym/state.json"},
			wantWeft:     []string{"sub/_lym/state.json"},
		},
		{
			name:         "lymfoo_is_a_segment_boundary_miss_so_warp",
			relPath:      ".",
			routingNames: []string{"_lym"},
			files:        []string{"_lymfoo/state.json"},
			wantWarp:     []string{"_lymfoo/state.json"},
			wantWeft:     nil,
		},
		{
			name:                "empty_file_list_yields_three_empty_lists",
			relPath:             ".",
			routingNames:        []string{"_lym"},
			neverCommittedNames: []string{".lym"},
			files:               nil,
			wantWarp:            nil,
			wantWeft:            nil,
			wantNeverCommitted:  nil,
		},
		{
			name:         "all_warp_inputs",
			relPath:      ".",
			routingNames: []string{"_lym"},
			files:        []string{"cmd/lym/main.go", "go.mod", "README.md"},
			wantWarp:     []string{"cmd/lym/main.go", "go.mod", "README.md"},
			wantWeft:     nil,
		},
		{
			name:         "all_weft_inputs",
			relPath:      ".",
			routingNames: []string{"_lym", "_extra"},
			files:        []string{"_lym/a.json", "_extra/notes.md"},
			wantWarp:     nil,
			wantWeft:     []string{"_lym/a.json", "_extra/notes.md"},
		},
		{
			name:         "routingNames_with_more_than_two_entries",
			relPath:      ".",
			routingNames: []string{"_lym", "_extra", lymdirs.BoltDirName, "_launchers"},
			files:        []string{lymdirs.BoltDirName + "/entry.md", "_launchers/run.sh", "cmd/lym/main.go", "_extra/notes.md"},
			wantWarp:     []string{"cmd/lym/main.go"},
			wantWeft:     []string{lymdirs.BoltDirName + "/entry.md", "_launchers/run.sh", "_extra/notes.md"},
		},
		{
			name:                "dotlym_path_lands_in_neither_weft_nor_warp",
			relPath:             ".",
			routingNames:        []string{"_lym", "_extra"},
			neverCommittedNames: []string{".lym"},
			files:               []string{".lym/webster/state.json.lock"},
			wantWarp:            nil,
			wantWeft:            nil,
			wantNeverCommitted:  []string{".lym/webster/state.json.lock"},
		},
		{
			name:                "dotlymfoo_is_a_segment_boundary_miss_so_warp",
			relPath:             ".",
			routingNames:        []string{"_lym"},
			neverCommittedNames: []string{".lym"},
			files:               []string{".lymfoo/state.json"},
			wantWarp:            []string{".lymfoo/state.json"},
			wantWeft:            nil,
			wantNeverCommitted:  nil,
		},
		{
			name:                "dotlym_prefix_wins_over_weft_when_names_would_overlap",
			relPath:             "sub",
			routingNames:        []string{"_lym"},
			neverCommittedNames: []string{".lym"},
			files:               []string{"sub/.lym/state.json.lock", "sub/_lym/state.json"},
			wantWarp:            nil,
			wantWeft:            []string{"sub/_lym/state.json"},
			wantNeverCommitted:  []string{"sub/.lym/state.json.lock"},
		},
		{
			name:                "mixed_bucket_with_relpath_not_dot",
			relPath:             "sub",
			routingNames:        []string{"_lym", "_extra"},
			neverCommittedNames: []string{".lym"},
			files: []string{
				"sub/_lym/module/state.json",
				"sub/.lym/module/state.json.lock",
				"warp/main.go",
			},
			wantWarp:           []string{"warp/main.go"},
			wantWeft:           []string{"sub/_lym/module/state.json"},
			wantNeverCommitted: []string{"sub/.lym/module/state.json.lock"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			gotWarp, gotWeft, gotNeverCommitted := ClassifyPaths(tt.relPath, tt.routingNames, tt.neverCommittedNames, tt.files)
			if !slices.Equal(gotWarp, tt.wantWarp) {
				t.Errorf("ClassifyPaths(%q, %v, %v, %v) warp = %v; want %v", tt.relPath, tt.routingNames, tt.neverCommittedNames, tt.files, gotWarp, tt.wantWarp)
			}
			if !slices.Equal(gotWeft, tt.wantWeft) {
				t.Errorf("ClassifyPaths(%q, %v, %v, %v) weft = %v; want %v", tt.relPath, tt.routingNames, tt.neverCommittedNames, tt.files, gotWeft, tt.wantWeft)
			}
			if !slices.Equal(gotNeverCommitted, tt.wantNeverCommitted) {
				t.Errorf("ClassifyPaths(%q, %v, %v, %v) neverCommitted = %v; want %v", tt.relPath, tt.routingNames, tt.neverCommittedNames, tt.files, gotNeverCommitted, tt.wantNeverCommitted)
			}
		})
	}
}
