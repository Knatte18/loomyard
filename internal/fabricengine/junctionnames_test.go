// junctionnames_test.go — unit tests for junctionNames, filterHubReserved, and the hub-structural
// name constructors BoardDir, HubPath and IsReservedHubName relocated from internal/lyxcwd in this
// batch.
//
// Package fabricengine (not fabricengine_test) so it can call the unexported junctionNames and
// filterHubReserved directly.
// Hermetic: no git spawn, so this stays a Tier-1 unit test even though the package as a whole also
// carries git-spawning integration tests elsewhere.

package fabricengine

import (
	"path/filepath"
	"testing"
)

// TestJunctionNames_NoFallbackOnLoadFailure asserts the no-fallback rule: a config-load failure at
// baseDir is surfaced as a non-nil error and a nil name slice, never silently defaulted to `_lyx`.
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

// TestFilterHubReserved covers filterHubReserved's table of shapes: mixed reserved/non-reserved
// input, all-reserved input, and no-reserved input.
func TestFilterHubReserved(t *testing.T) {
	reserved := HubReservedNames()

	tests := []struct {
		name  string
		input []string
		want  []string
	}{
		{
			name:  "MixedDropsReservedKeepsOrder",
			input: append(append([]string{"_lyx"}, reserved...), "_other", "_extra"),
			want:  []string{"_lyx", "_other", "_extra"},
		},
		{
			name:  "AllReservedYieldsEmpty",
			input: reserved,
			want:  []string{},
		},
		{
			name:  "NoneReservedReturnedUnchanged",
			input: []string{"_lyx", "_other", "_extra"},
			want:  []string{"_lyx", "_other", "_extra"},
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

	// Sanity: HubReservedNames() itself must contain exactly the three
	// hub-structural tokens this test's "MixedDropsReservedKeepsOrder" case
	// relies on being dropped -- the junction-wiring block set, unchanged by
	// this batch's structural-directory work.
	for _, want := range []string{"_board", "_portals", "_launchers"} {
		found := false
		for _, r := range reserved {
			if r == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("HubReservedNames() = %v; want it to contain %q", reserved, want)
		}
	}

	// .lyx must NEVER be in HubReservedNames(): adding it there would make
	// filterHubReserved delete it from the wired names (so the per-worktree
	// junction is never created) and would make scanOnDiskJunctionNames skip
	// it (so Unwire's sweep and applyStaleRemoval could never see it).
	for _, r := range reserved {
		if r == ".lyx" {
			t.Errorf("HubReservedNames() = %v; want it to NEVER contain %q", reserved, ".lyx")
		}
	}
}

// TestBoardDir verifies that BoardDir joins hub with BoardDirName.
func TestBoardDir(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		hub  string
		want string
	}{
		{
			name: "simple hub",
			hub:  "/h",
			want: filepath.Join("/h", BoardDirName),
		},
		{
			name: "nested hub",
			hub:  "/repos/loomyard-HUB",
			want: filepath.Join("/repos/loomyard-HUB", BoardDirName),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BoardDir(tt.hub)
			if got != tt.want {
				t.Errorf("BoardDir(%q) = %q; want %q", tt.hub, got, tt.want)
			}
		})
	}
}

// TestHubPath verifies that HubPath joins parent and name with HubSuffix.
func TestHubPath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		parent   string
		repoName string
		want     string
	}{
		{
			name:     "simple repo name",
			parent:   "/repos",
			repoName: "loomyard",
			want:     filepath.Join("/repos", "loomyard"+HubSuffix),
		},
		{
			name:     "nested parent",
			parent:   "/home/user/code",
			repoName: "myproject",
			want:     filepath.Join("/home/user/code", "myproject"+HubSuffix),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := HubPath(tt.parent, tt.repoName)
			if got != tt.want {
				t.Errorf("HubPath(%q, %q) = %q; want %q", tt.parent, tt.repoName, got, tt.want)
			}
		})
	}
}

// TestIsReservedHubName verifies the reserved hub-entry name predicate slug validation (fabric's
// Add) gates on: every geometry-owned hub-level entry name is reserved (union of HubReservedNames()
// and the caller-supplied junctionNames), ordinary slugs and near-misses are not.
func TestIsReservedHubName(t *testing.T) {
	t.Parallel()

	junctionNames := []string{}

	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"lyx dir", "_lyx", true},
		{"raddle dir", "_raddle", false},
		{"board dir", "_board", true},
		{"portals dir", "_portals", true},
		{"launchers dir", "_launchers", true},
		{"ordinary slug", "my-task", false},
		{"underscore-prefixed but unreserved", "_mytask", false},
		{"compound near-miss", "_boardroom", false},
		{"empty", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsReservedHubName(tt.input, junctionNames); got != tt.want {
				t.Errorf("IsReservedHubName(%q, %v) = %v; want %v", tt.input, junctionNames, got, tt.want)
			}
		})
	}

	// Regression (r1-review): the hub-structural tokens must stay reserved even
	// when junctionNames is empty -- a worktree slug must never claim _board,
	// _portals, or _launchers regardless of the caller's pathspec.
	// This is now the sole coverage of that invariant for an empty junctionNames
	// (exactly the configuration this batch's empty pathspec default creates),
	// making it more load-bearing than before -- _raddle dropped out of the set
	// entirely in card 19 and is no longer reserved at all.
	t.Run("hub-structural tokens reserved for empty junctionNames", func(t *testing.T) {
		t.Parallel()

		for _, hubStructural := range []string{"_board", "_portals", "_launchers"} {
			if got := IsReservedHubName(hubStructural, []string{}); !got {
				t.Errorf("IsReservedHubName(%q, []) = %v; want true", hubStructural, got)
			}
		}
	})

	// A name present only in the caller-supplied junctionNames (not in
	// HubReservedNames()) must be reported reserved -- the injected portion of
	// the union.
	t.Run("junction-only name is reserved", func(t *testing.T) {
		t.Parallel()

		if got := IsReservedHubName("_custom", []string{"_custom"}); !got {
			t.Errorf("IsReservedHubName(%q, %v) = %v; want true", "_custom", []string{"_custom"}, got)
		}
	})

	// A name absent from both HubReservedNames() and junctionNames must not be
	// reserved.
	t.Run("unrelated name is not reserved", func(t *testing.T) {
		t.Parallel()

		if got := IsReservedHubName("_other", []string{"_custom"}); got {
			t.Errorf("IsReservedHubName(%q, %v) = %v; want false", "_other", []string{"_custom"}, got)
		}
	})

	// The union over the default (now empty) pathspec's junctionNames reserves
	// exactly four names -- down from the six a hardcoded switch once reserved.
	// Two names left the set in this batch: _raddle, because HubReservedNames()
	// drops it (card 19); and _pattern, because an empty pathspec removes it
	// from the junctionNames union entirely (there is no longer any junction
	// name for a default-configured repo to inject).
	// With junctionNames now empty, the four survivors no longer share one
	// source: _lyx is reserved via structuralCommittedDirs (never the config
	// arm), while _board, _portals, and _launchers are reserved via
	// hubSlugReservedNames() (through HubReservedNames()).
	// `.lyx` is reserved too, via structuralNeverCommittedDirs and
	// hubSlugReservedNames(), but was never a member of this list and still is
	// not -- this is a positive "these are reserved" set, not an exhaustiveness
	// assertion over every reserved name.
	t.Run("default pathspec union reserves exactly four names", func(t *testing.T) {
		t.Parallel()

		wantReserved := []string{"_lyx", "_board", "_portals", "_launchers"}
		for _, name := range wantReserved {
			if got := IsReservedHubName(name, junctionNames); !got {
				t.Errorf("IsReservedHubName(%q, %v) = %v; want true", name, junctionNames, got)
			}
		}
		if got := IsReservedHubName("not-reserved", junctionNames); got {
			t.Errorf("IsReservedHubName(%q, %v) = %v; want false", "not-reserved", junctionNames, got)
		}
	})

	// _raddle has converged on an anchor-level `_lyx/raddle/` design with no
	// hub-level presence, so a worktree slug named _raddle is now accepted by
	// IsReservedHubName -- the observable behaviour change this batch delivers.
	t.Run("raddle slug is no longer reserved", func(t *testing.T) {
		t.Parallel()

		if got := IsReservedHubName("_raddle", junctionNames); got {
			t.Errorf("IsReservedHubName(%q, %v) = %v; want false", "_raddle", junctionNames, got)
		}
	})
}
