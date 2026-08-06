// classify_test.go — Tier-1 (untagged, no git spawn) table test for classifyPaths, the pure warp-vs-weft path classifier.

package fabricengine

import "testing"

// TestClassifyPaths covers relPath "."
// and "sub" scoping, the _lyxfoo segment-boundary case, empty input, all-warp and all-weft inputs, and a wiredNames set with more than two entries.
func TestClassifyPaths(t *testing.T) {
	tests := []struct {
		name       string
		relPath    string
		wiredNames []string
		files      []string
		wantWarp   []string
		wantWeft   []string
	}{
		{
			name:       "under_lyx_is_weft",
			relPath:    ".",
			wiredNames: []string{"_lyx", "_pattern"},
			files:      []string{"_lyx/module/state.json"},
			wantWarp:   nil,
			wantWeft:   []string{"_lyx/module/state.json"},
		},
		{
			name:       "under_pattern_is_weft",
			relPath:    ".",
			wiredNames: []string{"_lyx", "_pattern"},
			files:      []string{"_pattern/PATTERN.md"},
			wantWarp:   nil,
			wantWeft:   []string{"_pattern/PATTERN.md"},
		},
		{
			name:       "host_source_path_is_warp",
			relPath:    ".",
			wiredNames: []string{"_lyx", "_pattern"},
			files:      []string{"cmd/lyx/main.go"},
			wantWarp:   []string{"cmd/lyx/main.go"},
			wantWeft:   nil,
		},
		{
			name:       "relpath_dot_scopes_prefix_at_root",
			relPath:    ".",
			wiredNames: []string{"_lyx"},
			files:      []string{"_lyx/state.json", "sub/_lyx/state.json"},
			wantWarp:   []string{"sub/_lyx/state.json"},
			wantWeft:   []string{"_lyx/state.json"},
		},
		{
			name:       "relpath_sub_scopes_prefix_under_sub",
			relPath:    "sub",
			wiredNames: []string{"_lyx"},
			files:      []string{"sub/_lyx/state.json", "_lyx/state.json"},
			wantWarp:   []string{"_lyx/state.json"},
			wantWeft:   []string{"sub/_lyx/state.json"},
		},
		{
			name:       "lyxfoo_is_a_segment_boundary_miss_so_warp",
			relPath:    ".",
			wiredNames: []string{"_lyx"},
			files:      []string{"_lyxfoo/state.json"},
			wantWarp:   []string{"_lyxfoo/state.json"},
			wantWeft:   nil,
		},
		{
			name:       "empty_file_list_yields_two_empty_lists",
			relPath:    ".",
			wiredNames: []string{"_lyx"},
			files:      nil,
			wantWarp:   nil,
			wantWeft:   nil,
		},
		{
			name:       "all_warp_inputs",
			relPath:    ".",
			wiredNames: []string{"_lyx"},
			files:      []string{"cmd/lyx/main.go", "go.mod", "README.md"},
			wantWarp:   []string{"cmd/lyx/main.go", "go.mod", "README.md"},
			wantWeft:   nil,
		},
		{
			name:       "all_weft_inputs",
			relPath:    ".",
			wiredNames: []string{"_lyx", "_pattern"},
			files:      []string{"_lyx/a.json", "_pattern/PATTERN.md"},
			wantWarp:   nil,
			wantWeft:   []string{"_lyx/a.json", "_pattern/PATTERN.md"},
		},
		{
			name:       "wiredNames_with_more_than_two_entries",
			relPath:    ".",
			wiredNames: []string{"_lyx", "_pattern", "_board", "_launchers"},
			files:      []string{"_board/entry.md", "_launchers/run.sh", "cmd/lyx/main.go", "_pattern/PATTERN.md"},
			wantWarp:   []string{"cmd/lyx/main.go"},
			wantWeft:   []string{"_board/entry.md", "_launchers/run.sh", "_pattern/PATTERN.md"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotWarp, gotWeft := classifyPaths(tt.relPath, tt.wiredNames, tt.files)
			if !stringSlicesEqual(gotWarp, tt.wantWarp) {
				t.Errorf("classifyPaths(%q, %v, %v) warp = %v; want %v", tt.relPath, tt.wiredNames, tt.files, gotWarp, tt.wantWarp)
			}
			if !stringSlicesEqual(gotWeft, tt.wantWeft) {
				t.Errorf("classifyPaths(%q, %v, %v) weft = %v; want %v", tt.relPath, tt.wiredNames, tt.files, gotWeft, tt.wantWeft)
			}
		})
	}
}

// TestClassifyPaths_PartitionsInputWithNothingLostOrDuplicated asserts that for a mixed input, every original path appears in exactly one of the two output slices, in input order, with no path lost or duplicated.
func TestClassifyPaths_PartitionsInputWithNothingLostOrDuplicated(t *testing.T) {
	relPath := "."
	wiredNames := []string{"_lyx", "_pattern"}
	files := []string{
		"cmd/lyx/main.go",
		"_lyx/module/state.json",
		"go.mod",
		"_pattern/PATTERN.md",
		"_lyxfoo/notweft.json",
	}

	warp, weft := classifyPaths(relPath, wiredNames, files)

	// Reassemble warp/weft back into input order and compare against files:
	// this proves both that nothing was lost or duplicated and that each
	// slice preserves the relative order of the paths it received.
	reconstructed := make([]string, 0, len(files))
	wi, fi := 0, 0
	for _, want := range files {
		switch {
		case wi < len(warp) && warp[wi] == want:
			reconstructed = append(reconstructed, warp[wi])
			wi++
		case fi < len(weft) && weft[fi] == want:
			reconstructed = append(reconstructed, weft[fi])
			fi++
		}
	}
	if !stringSlicesEqual(reconstructed, files) {
		t.Errorf("classifyPaths(%q, %v, %v) warp=%v weft=%v did not partition input in order; reconstructed=%v", relPath, wiredNames, files, warp, weft, reconstructed)
	}
	if len(warp)+len(weft) != len(files) {
		t.Errorf("classifyPaths(%q, %v, %v) len(warp)+len(weft) = %d; want %d (input length)", relPath, wiredNames, files, len(warp)+len(weft), len(files))
	}
}

// stringSlicesEqual reports whether a and b contain the same strings in the
// same order. A nil slice and an empty slice compare equal.
func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
