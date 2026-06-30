// geometry_test.go covers the pure geometry constructors and the WeftHostSlug reverse
// parser added in the paths-foundation batch. It also asserts parity between the
// refactored weft Layout methods and their WeftSiblingPath equivalents.

package paths_test

import (
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/paths"
)

// TestWeftSiblingPath verifies that WeftSiblingPath joins hub and slug with WeftSuffix.
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
			want: filepath.Join("/h", "feat"+paths.WeftSuffix),
		},
		{
			name: "nested hub",
			hub:  "/repos/loomyard-HUB",
			slug: "main",
			want: filepath.Join("/repos/loomyard-HUB", "main"+paths.WeftSuffix),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := paths.WeftSiblingPath(tt.hub, tt.slug)
			if got != tt.want {
				t.Errorf("WeftSiblingPath(%q, %q) = %q; want %q", tt.hub, tt.slug, got, tt.want)
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
			want: filepath.Join("/h", paths.BoardDirName),
		},
		{
			name: "nested hub",
			hub:  "/repos/loomyard-HUB",
			want: filepath.Join("/repos/loomyard-HUB", paths.BoardDirName),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := paths.BoardDir(tt.hub)
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
			want:     filepath.Join("/repos", "loomyard"+paths.HubSuffix),
		},
		{
			name:     "nested parent",
			parent:   "/home/user/code",
			repoName: "myproject",
			want:     filepath.Join("/home/user/code", "myproject"+paths.HubSuffix),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := paths.HubPath(tt.parent, tt.repoName)
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
			gotSlug, gotOK := paths.WeftHostSlug(tt.input)
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

	layout := &paths.Layout{
		Cwd:          filepath.Join(hub, slug),
		WorktreeRoot: filepath.Join(hub, slug),
		Hub:          hub,
		RelPath:      ".",
		Prime:        prime,
	}

	// WeftWorktreePath(slug) must equal WeftSiblingPath(hub, slug).
	gotWorktreePath := layout.WeftWorktreePath(slug)
	wantWorktreePath := paths.WeftSiblingPath(hub, slug)
	if gotWorktreePath != wantWorktreePath {
		t.Errorf("WeftWorktreePath(%q) = %q; want WeftSiblingPath(%q, %q) = %q",
			slug, gotWorktreePath, hub, slug, wantWorktreePath)
	}

	// WeftRepoRoot() must equal WeftSiblingPath(hub, filepath.Base(prime)).
	gotRepoRoot := layout.WeftRepoRoot()
	wantRepoRoot := paths.WeftSiblingPath(hub, filepath.Base(prime))
	if gotRepoRoot != wantRepoRoot {
		t.Errorf("WeftRepoRoot() = %q; want WeftSiblingPath(%q, %q) = %q",
			gotRepoRoot, hub, filepath.Base(prime), wantRepoRoot)
	}

	// WeftWorktree() must equal WeftSiblingPath(hub, filepath.Base(WorktreeRoot)).
	gotWorktree := layout.WeftWorktree()
	wantWorktree := paths.WeftSiblingPath(hub, filepath.Base(layout.WorktreeRoot))
	if gotWorktree != wantWorktree {
		t.Errorf("WeftWorktree() = %q; want WeftSiblingPath(%q, %q) = %q",
			gotWorktree, hub, filepath.Base(layout.WorktreeRoot), wantWorktree)
	}
}
