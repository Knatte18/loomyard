// junctionnames_test.go — unit tests for junctionNames and filterHubReserved.
//
// Package fabricengine (not fabricengine_test) so it can call the unexported
// junctionNames and filterHubReserved directly. Hermetic: no git spawn, so
// this stays a Tier-1 unit test even though the package as a whole also
// carries git-spawning integration tests elsewhere.

package fabricengine

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

// TestJunctionNames_NoFallbackOnLoadFailure asserts the no-fallback rule: a
// config-load failure at baseDir is surfaced as a non-nil error and a nil
// name slice, never silently defaulted to _lyx/_pattern.
func TestJunctionNames_NoFallbackOnLoadFailure(t *testing.T) {
	baseDir := t.TempDir() // no _lyx/, so LoadConfig cannot find fabric.yaml

	names, err := junctionNames(baseDir)
	if err == nil {
		t.Fatalf("junctionNames(%q) error = nil; want a config-load error", baseDir)
	}
	if names != nil {
		t.Errorf("junctionNames(%q) names = %v; want nil on error (no fallback default)", baseDir, names)
	}
}

// TestFilterHubReserved covers filterHubReserved's table of shapes: mixed
// reserved/non-reserved input, all-reserved input, and no-reserved input.
func TestFilterHubReserved(t *testing.T) {
	reserved := lyxcwd.HubReservedNames()

	tests := []struct {
		name  string
		input []string
		want  []string
	}{
		{
			name:  "MixedDropsReservedKeepsOrder",
			input: append(append([]string{"_lyx"}, reserved...), "_pattern", "_extra"),
			want:  []string{"_lyx", "_pattern", "_extra"},
		},
		{
			name:  "AllReservedYieldsEmpty",
			input: reserved,
			want:  []string{},
		},
		{
			name:  "NoneReservedReturnedUnchanged",
			input: []string{"_lyx", "_pattern", "_extra"},
			want:  []string{"_lyx", "_pattern", "_extra"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := filterHubReserved(tt.input)
			if len(got) != len(tt.want) {
				t.Fatalf("filterHubReserved(%v) = %v; want %v", tt.input, got, tt.want)
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Errorf("filterHubReserved(%v)[%d] = %q; want %q", tt.input, i, got[i], tt.want[i])
				}
			}
		})
	}

	// Sanity: HubReservedNames() itself must contain exactly the four
	// hub-structural tokens this test's "MixedDropsReservedKeepsOrder" case
	// relies on being dropped.
	for _, want := range []string{"_board", "_portals", "_launchers", "_raddle"} {
		found := false
		for _, r := range reserved {
			if r == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("lyxcwd.HubReservedNames() = %v; want it to contain %q", reserved, want)
		}
	}
}
