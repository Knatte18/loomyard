// weftpaths_test.go covers the weft-sibling path accessors relocated from internal/lyxcwd in this
// batch — WeftWorktree, WeftWorktreePath, WeftLyxDir and WeftLyxDirFor — with both AnchorRel "."
// (root) and subpath cases, pinning the "-weft" sibling naming across container/base/subpath
// combinations the illusion depends on.
// WeftRaddleDir had zero production callers and is deleted outright rather than relocated, so it
// has no coverage here.

package fabricengine

import (
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

// TestWeftPathAccessors covers WeftWorktree, WeftWorktreePath, WeftLyxDir and WeftLyxDirFor,
// verifying AnchorRel-mirroring and junction pairing against the host-side worktree.
func TestWeftPathAccessors(t *testing.T) {
	tests := []struct {
		name    string
		hub     string
		slug    string
		relPath string

		wantWeftWorktree     string
		wantWeftWorktreePath string
		wantWeftLyxDir       string
		wantWeftLyxDirFor    string
	}{
		{
			name:                 "/h /h/main feat . case",
			hub:                  "/h",
			slug:                 "x",
			relPath:              ".",
			wantWeftWorktree:     filepath.Join("/h", "feat-weft"),
			wantWeftWorktreePath: filepath.Join("/h", "x-weft"),
			wantWeftLyxDir:       filepath.Join("/h", "feat-weft", "_lyx"),
			wantWeftLyxDirFor:    filepath.Join("/h", "x-weft", "_lyx"),
		},
		{
			name:                 "/h /h/main feat sub case",
			hub:                  "/h",
			slug:                 "x",
			relPath:              "sub",
			wantWeftWorktree:     filepath.Join("/h", "feat-weft"),
			wantWeftWorktreePath: filepath.Join("/h", "x-weft"),
			wantWeftLyxDir:       filepath.Join("/h", "feat-weft", "sub", "_lyx"),
			wantWeftLyxDirFor:    filepath.Join("/h", "x-weft", "sub", "_lyx"),
		},
		{
			name:                 "/h /h/main feat sub/dir case",
			hub:                  "/h",
			slug:                 "y",
			relPath:              "sub/dir",
			wantWeftWorktree:     filepath.Join("/h", "feat-weft"),
			wantWeftWorktreePath: filepath.Join("/h", "y-weft"),
			wantWeftLyxDir:       filepath.Join("/h", "feat-weft", "sub/dir", "_lyx"),
			wantWeftLyxDirFor:    filepath.Join("/h", "y-weft", "sub/dir", "_lyx"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loc := &lyxcwd.Location{
				HubPath:      tt.hub,
				WorktreeName: "feat",
				AnchorRel:    tt.relPath,
			}

			if got := WeftWorktree(loc); got != tt.wantWeftWorktree {
				t.Errorf("WeftWorktree(l) = %q; want %q", got, tt.wantWeftWorktree)
			}

			if got := WeftWorktreePath(loc, tt.slug); got != tt.wantWeftWorktreePath {
				t.Errorf("WeftWorktreePath(l, %q) = %q; want %q", tt.slug, got, tt.wantWeftWorktreePath)
			}

			if got := WeftLyxDir(loc); got != tt.wantWeftLyxDir {
				t.Errorf("WeftLyxDir(l) = %q; want %q", got, tt.wantWeftLyxDir)
			}

			if got := WeftLyxDirFor(loc, tt.slug); got != tt.wantWeftLyxDirFor {
				t.Errorf("WeftLyxDirFor(l, %q) = %q; want %q", tt.slug, got, tt.wantWeftLyxDirFor)
			}

			// Verify junction pairing: the host and weft worktree bases differ
			// only by the -weft suffix on the worktree dir.
			hostWtName := filepath.Base(filepath.Join(loc.HubPath, tt.slug))
			weftWtName := filepath.Base(WeftWorktreePath(loc, tt.slug))

			if hostWtName != tt.slug {
				t.Errorf("WorktreePath(%q) base = %q; want %q", tt.slug, hostWtName, tt.slug)
			}
			if weftWtName != tt.slug+"-weft" {
				t.Errorf("WeftWorktreePath(%q) base = %q; want %q", tt.slug, weftWtName, tt.slug+"-weft")
			}
		})
	}
}

// TestWeftHostSlug covers WeftHostSlug's round-trip against a weft sibling directory name,
// and rejects names that do not carry the weftname.Suffix tail or whose stripped prefix is empty.
func TestWeftHostSlug(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantSlug string
		wantOK   bool
	}{
		{"plain_slug", "feat-weft", "feat", true},
		{"nested_prefix_slug", "hanf/foo-weft", "hanf/foo", true},
		{"no_suffix", "feat", "", false},
		{"suffix_only_empty_prefix", "-weft", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotSlug, gotOK := WeftHostSlug(tt.input)
			if gotOK != tt.wantOK {
				t.Errorf("WeftHostSlug(%q) ok = %v; want %v", tt.input, gotOK, tt.wantOK)
			}
			if gotSlug != tt.wantSlug {
				t.Errorf("WeftHostSlug(%q) = %q; want %q", tt.input, gotSlug, tt.wantSlug)
			}
		})
	}
}
