// geometry_test.go covers the pure geometry constructors and the WeftHostSlug reverse
// parser added in the paths-foundation batch. It also asserts parity between the
// refactored weft Layout methods and their weftname.SiblingPath equivalents.

package hubgeometry_test

import (
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/hubgeometry"
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
			want: filepath.Join("/h", hubgeometry.BoardDirName),
		},
		{
			name: "nested hub",
			hub:  "/repos/loomyard-HUB",
			want: filepath.Join("/repos/loomyard-HUB", hubgeometry.BoardDirName),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := hubgeometry.BoardDir(tt.hub)
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
			want:     filepath.Join("/repos", "loomyard"+hubgeometry.HubSuffix),
		},
		{
			name:     "nested parent",
			parent:   "/home/user/code",
			repoName: "myproject",
			want:     filepath.Join("/home/user/code", "myproject"+hubgeometry.HubSuffix),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := hubgeometry.HubPath(tt.parent, tt.repoName)
			if got != tt.want {
				t.Errorf("HubPath(%q, %q) = %q; want %q", tt.parent, tt.repoName, got, tt.want)
			}
		})
	}
}

// TestWeftHostSlug verifies the reverse parser for weft sibling directory names.
func TestWeftHostSlug(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		wantSlug string
		wantOK   bool
	}{
		{
			name:     "valid weft name",
			input:    "feat-weft",
			wantSlug: "feat",
			wantOK:   true,
		},
		{
			name:     "name without weft suffix",
			input:    "feat",
			wantSlug: "",
			wantOK:   false,
		},
		{
			name:     "bare suffix only (empty-slug guard)",
			input:    "-weft",
			wantSlug: "",
			wantOK:   false,
		},
		{
			name:     "multi-segment slug",
			input:    "my-feature-weft",
			wantSlug: "my-feature",
			wantOK:   true,
		},
		{
			name:     "empty string",
			input:    "",
			wantSlug: "",
			wantOK:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotSlug, gotOK := hubgeometry.WeftHostSlug(tt.input)
			if gotSlug != tt.wantSlug || gotOK != tt.wantOK {
				t.Errorf("WeftHostSlug(%q) = (%q, %v); want (%q, %v)",
					tt.input, gotSlug, gotOK, tt.wantSlug, tt.wantOK)
			}
		})
	}
}

// TestWeftLayoutMethodParity asserts that the refactored weft Layout methods produce
// byte-identical results to the direct WeftSiblingPath form.
func TestWeftLayoutMethodParity(t *testing.T) {
	t.Parallel()

	hub := "/h"
	prime := filepath.Join(hub, "main")
	slug := "feat"

	layout := &hubgeometry.Layout{
		Cwd:          filepath.Join(hub, slug),
		WorktreeRoot: filepath.Join(hub, slug),
		Hub:          hub,
		RelPath:      ".",
		Prime:        prime,
	}

	// WeftWorktreePath(slug) must equal weftname.SiblingPath(hub, slug).
	gotWorktreePath := layout.WeftWorktreePath(slug)
	wantWorktreePath := weftname.SiblingPath(hub, slug)
	if gotWorktreePath != wantWorktreePath {
		t.Errorf("WeftWorktreePath(%q) = %q; want SiblingPath(%q, %q) = %q",
			slug, gotWorktreePath, hub, slug, wantWorktreePath)
	}

	// WeftRepoRoot() must equal weftname.SiblingPath(hub, filepath.Base(prime)).
	gotRepoRoot := layout.WeftRepoRoot()
	wantRepoRoot := weftname.SiblingPath(hub, filepath.Base(prime))
	if gotRepoRoot != wantRepoRoot {
		t.Errorf("WeftRepoRoot() = %q; want SiblingPath(%q, %q) = %q",
			gotRepoRoot, hub, filepath.Base(prime), wantRepoRoot)
	}

	// WeftWorktree() must equal weftname.SiblingPath(hub, filepath.Base(WorktreeRoot)).
	gotWorktree := layout.WeftWorktree()
	wantWorktree := weftname.SiblingPath(hub, filepath.Base(layout.WorktreeRoot))
	if gotWorktree != wantWorktree {
		t.Errorf("WeftWorktree() = %q; want SiblingPath(%q, %q) = %q",
			gotWorktree, hub, filepath.Base(layout.WorktreeRoot), wantWorktree)
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
			if got := hubgeometry.IsReservedHubName(tt.input, junctionNames); got != tt.want {
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
			if got := hubgeometry.IsReservedHubName(hubStructural, []string{}); !got {
				t.Errorf("IsReservedHubName(%q, []) = %v; want true", hubStructural, got)
			}
		}
	})

	// A name present only in the caller-supplied junctionNames (not in
	// HubReservedNames()) must be reported reserved — the injected portion of
	// the union.
	t.Run("junction-only name is reserved", func(t *testing.T) {
		t.Parallel()

		if got := hubgeometry.IsReservedHubName("_custom", []string{"_custom"}); !got {
			t.Errorf("IsReservedHubName(%q, %v) = %v; want true", "_custom", []string{"_custom"}, got)
		}
	})

	// A name absent from both HubReservedNames() and junctionNames must not be
	// reserved.
	t.Run("unrelated name is not reserved", func(t *testing.T) {
		t.Parallel()

		if got := hubgeometry.IsReservedHubName("_other", []string{"_custom"}); got {
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
			if got := hubgeometry.IsReservedHubName(name, junctionNames); !got {
				t.Errorf("IsReservedHubName(%q, %v) = %v; want true", name, junctionNames, got)
			}
		}
		if got := hubgeometry.IsReservedHubName("not-reserved", junctionNames); got {
			t.Errorf("IsReservedHubName(%q, %v) = %v; want false", "not-reserved", junctionNames, got)
		}
	})
}
