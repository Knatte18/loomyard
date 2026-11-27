// weftpaths_test.go covers the weft-sibling path accessors:
// RecordsWorktree, RecordsWorktreePath and WeftLymDirFor, with AnchorRel "." (root) and subpath cases.
// It pins the "-weft" sibling naming across container, base and subpath combinations.

package fabricengine

import (
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestWeftPathAccessors checks that the weft path accessors mirror AnchorRel.
//
//testtiming:keep RecordsWorktree, RecordsWorktreePath and WeftLymDirFor mirroring AnchorRel; coverage of its blocks by other tests does not show an assertion of this
func TestWeftPathAccessors(t *testing.T) {
	tiergate.Require(t, 0)
	tests := []struct {
		name    string
		hub     string
		slug    string
		relPath string

		wantWeftWorktree     string
		wantWeftWorktreePath string
		wantWeftLymDirFor    string
	}{
		{
			name:                 "/h /h/main feat . case",
			hub:                  "/h",
			slug:                 "x",
			relPath:              ".",
			wantWeftWorktree:     filepath.Join("/h", "feat-weft"),
			wantWeftWorktreePath: filepath.Join("/h", "x-weft"),
			wantWeftLymDirFor:    filepath.Join("/h", "x-weft", "_lym"),
		},
		{
			name:                 "/h /h/main feat sub case",
			hub:                  "/h",
			slug:                 "x",
			relPath:              "sub",
			wantWeftWorktree:     filepath.Join("/h", "feat-weft"),
			wantWeftWorktreePath: filepath.Join("/h", "x-weft"),
			wantWeftLymDirFor:    filepath.Join("/h", "x-weft", "sub", "_lym"),
		},
		{
			name:                 "/h /h/main feat sub/dir case",
			hub:                  "/h",
			slug:                 "y",
			relPath:              "sub/dir",
			wantWeftWorktree:     filepath.Join("/h", "feat-weft"),
			wantWeftWorktreePath: filepath.Join("/h", "y-weft"),
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

			if got := WeftLymDirFor(loc, tt.slug); got != tt.wantWeftLymDirFor {
				t.Errorf("WeftLymDirFor(l, %q) = %q; want %q", tt.slug, got, tt.wantWeftLymDirFor)
			}
		})
	}
}
