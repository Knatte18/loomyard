// junctionnames_test.go — unit tests for junctionNames, filterHubReserved, and the hub-structural name constructors BoardDir, HubPath and IsReservedHubName relocated from internal/lyxcwd in this batch.
//
// Package fabricengine (not fabricengine_test) so it can call the unexported junctionNames and filterHubReserved directly.
// Hermetic: no git spawn, so this stays a Tier-1 unit test even though the package as a whole also carries git-spawning integration tests elsewhere.

package fabricengine

import (
	"path/filepath"
	"testing"
)

// TestJunctionNames_NoFallbackOnLoadFailure asserts the no-fallback rule: a config-load failure at baseDir is surfaced as a non-nil error and a nil name slice, never silently defaulted to _lyx/_pattern.
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

// TestFilterHubReserved covers filterHubReserved's table of shapes: mixed reserved/non-reserved input, all-reserved input, and no-reserved input.
func TestFilterHubReserved(t *testing.T) {
	reserved := HubReservedNames()

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
			t.Errorf("HubReservedNames() = %v; want it to contain %q", reserved, want)
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

// TestIsReservedHubName verifies the reserved hub-entry name predicate slug validation (fabric's Add) gates on: every geometry-owned hub-level entry name is reserved (union of HubReservedNames() and the caller-supplied junctionNames), ordinary slugs and near-misses are not.
func TestIsReservedHubName(t *testing.T) {
	t.Parallel()

	junctionNames := []string{"_lyx", "_pattern"}

	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"lyx dir", "_lyx", true},
		{"raddle dir", "_raddle", true},
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
	// _portals, _launchers, or _raddle regardless of the caller's pathspec.
	t.Run("hub-structural tokens reserved for empty junctionNames", func(t *testing.T) {
		t.Parallel()

		for _, hubStructural := range []string{"_board", "_portals", "_launchers", "_raddle"} {
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

	// The union over the default pathspec's junctionNames reserves exactly the
	// six names today's hardcoded switch reserved: _lyx, _pattern, _board,
	// _portals, _launchers, _raddle -- no more, no fewer.
	t.Run("default pathspec union reserves exactly six names", func(t *testing.T) {
		t.Parallel()

		wantReserved := []string{"_lyx", "_pattern", "_board", "_portals", "_launchers", "_raddle"}
		for _, name := range wantReserved {
			if got := IsReservedHubName(name, junctionNames); !got {
				t.Errorf("IsReservedHubName(%q, %v) = %v; want true", name, junctionNames, got)
			}
		}
		if got := IsReservedHubName("not-reserved", junctionNames); got {
			t.Errorf("IsReservedHubName(%q, %v) = %v; want false", "not-reserved", junctionNames, got)
		}
	})
}
