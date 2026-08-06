// geometry_test.go covers the pure geometry constructors this module still owns —
// weftname.SiblingPath's join shape, BoardDir, HubPath and IsReservedHubName. The
// WeftHostSlug reverse parser and the weft Location-method parity coverage relocated
// to internal/fabricengine along with the methods themselves.

package lyxcwd_test

import (
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/weftname"
)

// TestWeftSiblingPath verifies that weftname.SiblingPath joins hub and slug with weftname.Suffix.
func TestWeftSiblingPath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		hub  string
		slug string
		want string
	}{
		{
			name: "simple slug",
			hub:  "/h",
			slug: "feat",
			want: filepath.Join("/h", "feat"+weftname.Suffix),
		},
		{
			name: "nested hub",
			hub:  "/repos/loomyard-HUB",
			slug: "main",
			want: filepath.Join("/repos/loomyard-HUB", "main"+weftname.Suffix),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := weftname.SiblingPath(tt.hub, tt.slug)
			if got != tt.want {
				t.Errorf("SiblingPath(%q, %q) = %q; want %q", tt.hub, tt.slug, got, tt.want)
			}
		})
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
			want: filepath.Join("/h", lyxcwd.BoardDirName),
		},
		{
			name: "nested hub",
			hub:  "/repos/loomyard-HUB",
			want: filepath.Join("/repos/loomyard-HUB", lyxcwd.BoardDirName),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := lyxcwd.BoardDir(tt.hub)
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
			want:     filepath.Join("/repos", "loomyard"+lyxcwd.HubSuffix),
		},
		{
			name:     "nested parent",
			parent:   "/home/user/code",
			repoName: "myproject",
			want:     filepath.Join("/home/user/code", "myproject"+lyxcwd.HubSuffix),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := lyxcwd.HubPath(tt.parent, tt.repoName)
			if got != tt.want {
				t.Errorf("HubPath(%q, %q) = %q; want %q", tt.parent, tt.repoName, got, tt.want)
			}
		})
	}
}

// TestIsReservedHubName verifies the reserved hub-entry name predicate slug
// validation (fabric's Add) gates on: every geometry-owned hub-level entry
// name is reserved (union of HubReservedNames() and the caller-supplied
// junctionNames), ordinary slugs and near-misses are not.
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
			if got := lyxcwd.IsReservedHubName(tt.input, junctionNames); got != tt.want {
				t.Errorf("IsReservedHubName(%q, %v) = %v; want %v", tt.input, junctionNames, got, tt.want)
			}
		})
	}

	// Regression (r1-review): the hub-structural tokens must stay reserved even
	// when junctionNames is empty — a worktree slug must never claim _board,
	// _portals, _launchers, or _raddle regardless of the caller's pathspec.
	t.Run("hub-structural tokens reserved for empty junctionNames", func(t *testing.T) {
		t.Parallel()

		for _, hubStructural := range []string{"_board", "_portals", "_launchers", "_raddle"} {
			if got := lyxcwd.IsReservedHubName(hubStructural, []string{}); !got {
				t.Errorf("IsReservedHubName(%q, []) = %v; want true", hubStructural, got)
			}
		}
	})

	// A name present only in the caller-supplied junctionNames (not in
	// HubReservedNames()) must be reported reserved — the injected portion of
	// the union.
	t.Run("junction-only name is reserved", func(t *testing.T) {
		t.Parallel()

		if got := lyxcwd.IsReservedHubName("_custom", []string{"_custom"}); !got {
			t.Errorf("IsReservedHubName(%q, %v) = %v; want true", "_custom", []string{"_custom"}, got)
		}
	})

	// A name absent from both HubReservedNames() and junctionNames must not be
	// reserved.
	t.Run("unrelated name is not reserved", func(t *testing.T) {
		t.Parallel()

		if got := lyxcwd.IsReservedHubName("_other", []string{"_custom"}); got {
			t.Errorf("IsReservedHubName(%q, %v) = %v; want false", "_other", []string{"_custom"}, got)
		}
	})

	// The union over the default pathspec's junctionNames reserves exactly the
	// six names today's hardcoded switch reserved: _lyx, _pattern, _board,
	// _portals, _launchers, _raddle — no more, no fewer.
	t.Run("default pathspec union reserves exactly six names", func(t *testing.T) {
		t.Parallel()

		wantReserved := []string{"_lyx", "_pattern", "_board", "_portals", "_launchers", "_raddle"}
		for _, name := range wantReserved {
			if got := lyxcwd.IsReservedHubName(name, junctionNames); !got {
				t.Errorf("IsReservedHubName(%q, %v) = %v; want true", name, junctionNames, got)
			}
		}
		if got := lyxcwd.IsReservedHubName("not-reserved", junctionNames); got {
			t.Errorf("IsReservedHubName(%q, %v) = %v; want false", "not-reserved", junctionNames, got)
		}
	})
}
