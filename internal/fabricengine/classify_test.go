//go:build tier0

// classify_test.go — Tier-1 (untagged, no git spawn) table test for classifyPaths, the pure
// warp-vs-weft-vs-never-committed path classifier.

package fabricengine

import "testing"

// TestClassifyPaths covers relPath "."
// and "sub" scoping, the _lymfoo/.lymfoo segment-boundary cases, empty input, all-warp and all-weft
// inputs, a routingNames set with more than two entries, and the never-committed bucket — including
// the case that distinguishes the correct implementation from the dangerous one: a never-committed
// path must land in neither weft nor warp, not merely "not weft".
//
//testtiming:keep classifyPaths' routing buckets, including segment-boundary names and the never-committed bucket landing in neither side; coverage of its blocks by other tests does not show an assertion of this
func TestClassifyPaths(t *testing.T) {
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
			routingNames: []string{"_lym", "_extra", "_board", "_launchers"},
			files:        []string{"_board/entry.md", "_launchers/run.sh", "cmd/lym/main.go", "_extra/notes.md"},
			wantWarp:     []string{"cmd/lym/main.go"},
			wantWeft:     []string{"_board/entry.md", "_launchers/run.sh", "_extra/notes.md"},
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
			gotWarp, gotWeft, gotNeverCommitted := classifyPaths(tt.relPath, tt.routingNames, tt.neverCommittedNames, tt.files)
			if !stringSlicesEqual(gotWarp, tt.wantWarp) {
				t.Errorf("classifyPaths(%q, %v, %v, %v) warp = %v; want %v", tt.relPath, tt.routingNames, tt.neverCommittedNames, tt.files, gotWarp, tt.wantWarp)
			}
			if !stringSlicesEqual(gotWeft, tt.wantWeft) {
				t.Errorf("classifyPaths(%q, %v, %v, %v) weft = %v; want %v", tt.relPath, tt.routingNames, tt.neverCommittedNames, tt.files, gotWeft, tt.wantWeft)
			}
			if !stringSlicesEqual(gotNeverCommitted, tt.wantNeverCommitted) {
				t.Errorf("classifyPaths(%q, %v, %v, %v) neverCommitted = %v; want %v", tt.relPath, tt.routingNames, tt.neverCommittedNames, tt.files, gotNeverCommitted, tt.wantNeverCommitted)
			}
		})
	}
}

// TestClassifyPaths_PartitionsInputWithNothingLostOrDuplicated asserts that for a mixed input,
// every original path appears in exactly one of the three output slices, in input order, with no
// path lost or duplicated.
//
//testtiming:keep every input path landing in exactly one output slice in input order; coverage of its blocks by other tests does not show an assertion of this
func TestClassifyPaths_PartitionsInputWithNothingLostOrDuplicated(t *testing.T) {
	relPath := "."
	routingNames := []string{"_lym", "_extra"}
	neverCommittedNames := []string{".lym"}
	files := []string{
		"cmd/lym/main.go",
		"_lym/module/state.json",
		"go.mod",
		"_extra/notes.md",
		"_lymfoo/notweft.json",
		".lym/webster/state.json.lock",
		".lymfoo/notnevercommitted.json",
	}

	warp, weft, neverCommitted := classifyPaths(relPath, routingNames, neverCommittedNames, files)

	// Reassemble warp/weft/neverCommitted back into input order and compare against files: this
	// proves both that nothing was lost or duplicated and that each slice preserves the relative
	// order of the paths it received.
	reconstructed := make([]string, 0, len(files))
	wi, fi, ni := 0, 0, 0
	for _, want := range files {
		switch {
		case wi < len(warp) && warp[wi] == want:
			reconstructed = append(reconstructed, warp[wi])
			wi++
		case fi < len(weft) && weft[fi] == want:
			reconstructed = append(reconstructed, weft[fi])
			fi++
		case ni < len(neverCommitted) && neverCommitted[ni] == want:
			reconstructed = append(reconstructed, neverCommitted[ni])
			ni++
		}
	}
	if !stringSlicesEqual(reconstructed, files) {
		t.Errorf("classifyPaths(%q, %v, %v, %v) warp=%v weft=%v neverCommitted=%v did not partition input in order; reconstructed=%v", relPath, routingNames, neverCommittedNames, files, warp, weft, neverCommitted, reconstructed)
	}
	if len(warp)+len(weft)+len(neverCommitted) != len(files) {
		t.Errorf("classifyPaths(%q, %v, %v, %v) len(warp)+len(weft)+len(neverCommitted) = %d; want %d (input length)", relPath, routingNames, neverCommittedNames, files, len(warp)+len(weft)+len(neverCommitted), len(files))
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
