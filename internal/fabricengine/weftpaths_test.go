// weftpaths_test.go covers the weft-sibling path accessors:
// RecordsWorktree, RecordsWorktreePath, WeftLymDir and WeftLymDirFor, with AnchorRel "." (root) and subpath cases.
// It pins the "-weft" sibling naming across container, base and subpath combinations.

package fabricengine

import (
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestWeftPathAccessors checks that the weft path accessors mirror AnchorRel.
// It also checks that each weft worktree pairs with its warp-side worktree.
//
//testtiming:keep RecordsWorktree, RecordsWorktreePath, WeftLymDir and WeftLymDirFor mirroring AnchorRel and pairing with the warp side; coverage of its blocks by other tests does not show an assertion of this
func TestWeftPathAccessors(t *testing.T) {
	tiergate.Require(t, 0)
	tests := []struct {
		name    string
		hub     string
		slug    string
		relPath string

		wantWeftWorktree     string
		wantWeftWorktreePath string
		wantWeftLymDir       string
		wantWeftLymDirFor    string
	}{
		{
			name:                 "/h /h/main feat . case",
			hub:                  "/h",
			slug:                 "x",
			relPath:              ".",
			wantWeftWorktree:     filepath.Join("/h", "feat-weft"),
			wantWeftWorktreePath: filepath.Join("/h", "x-weft"),
			wantWeftLymDir:       filepath.Join("/h", "feat-weft", "_lym"),
			wantWeftLymDirFor:    filepath.Join("/h", "x-weft", "_lym"),
		},
		{
			name:                 "/h /h/main feat sub case",
			hub:                  "/h",
			slug:                 "x",
			relPath:              "sub",
			wantWeftWorktree:     filepath.Join("/h", "feat-weft"),
			wantWeftWorktreePath: filepath.Join("/h", "x-weft"),
			wantWeftLymDir:       filepath.Join("/h", "feat-weft", "sub", "_lym"),
			wantWeftLymDirFor:    filepath.Join("/h", "x-weft", "sub", "_lym"),
		},
		{
			name:                 "/h /h/main feat sub/dir case",
			hub:                  "/h",
			slug:                 "y",
			relPath:              "sub/dir",
			wantWeftWorktree:     filepath.Join("/h", "feat-weft"),
			wantWeftWorktreePath: filepath.Join("/h", "y-weft"),
			wantWeftLymDir:       filepath.Join("/h", "feat-weft", "sub/dir", "_lym"),
			wantWeftLymDirFor:    filepath.Join("/h", "y-weft", "sub/dir", "_lym"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loc := &lymcwd.Location{
				HubPath:      tt.hub,
				WorktreeName: "feat",
				AnchorRel:    tt.relPath,
			}

			if got := RecordsWorktree(loc); got != tt.wantWeftWorktree {
				t.Errorf("RecordsWorktree(l) = %q; want %q", got, tt.wantWeftWorktree)
			}

			if got := RecordsWorktreePath(loc, tt.slug); got != tt.wantWeftWorktreePath {
				t.Errorf("RecordsWorktreePath(l, %q) = %q; want %q", tt.slug, got, tt.wantWeftWorktreePath)
			}

			if got := WeftLymDir(loc); got != tt.wantWeftLymDir {
				t.Errorf("WeftLymDir(l) = %q; want %q", got, tt.wantWeftLymDir)
			}

			if got := WeftLymDirFor(loc, tt.slug); got != tt.wantWeftLymDirFor {
				t.Errorf("WeftLymDirFor(l, %q) = %q; want %q", tt.slug, got, tt.wantWeftLymDirFor)
			}

			// Verify junction pairing: the warp and weft worktree bases differ
			// only by the -weft suffix on the worktree dir.
			warpWtName := filepath.Base(filepath.Join(loc.HubPath, tt.slug))
			weftWtName := filepath.Base(RecordsWorktreePath(loc, tt.slug))

			if warpWtName != tt.slug {
				t.Errorf("WorktreePath(%q) base = %q; want %q", tt.slug, warpWtName, tt.slug)
			}
			if weftWtName != tt.slug+"-weft" {
				t.Errorf("RecordsWorktreePath(%q) base = %q; want %q", tt.slug, weftWtName, tt.slug+"-weft")
			}
		})
	}
}

// TestWeftWarpSlug checks that WeftWarpSlug round-trips a weft sibling directory name,
// and that it rejects names without the weftname.Suffix tail or with an empty stripped prefix.
func TestWeftWarpSlug(t *testing.T) {
	tiergate.Require(t, 0)
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
			gotSlug, gotOK := WeftWarpSlug(tt.input)
			if gotOK != tt.wantOK {
				t.Errorf("WeftWarpSlug(%q) ok = %v; want %v", tt.input, gotOK, tt.wantOK)
			}
			if gotSlug != tt.wantSlug {
				t.Errorf("WeftWarpSlug(%q) = %q; want %q", tt.input, gotSlug, tt.wantSlug)
			}
		})
	}
}
