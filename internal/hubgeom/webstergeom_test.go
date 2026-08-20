// webstergeom_test.go pins WebsterGeometry against the websterengine accessors, planparser.PlanDir
// and fabricengine.StencilsDir themselves, so this test cannot drift from those packages' own joins.
// Following hubgeom_test.go's fixture discipline, hub, worktree root, and anchor path stay three
// distinct directories so a field mix-up inside WebsterGeometry surfaces instead of passing
// silently.

package hubgeom

import (
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

func TestWebsterGeometry(t *testing.T) {
	tests := []struct {
		name      string
		anchorRel string
	}{
		{"subpath-anchored fixture", filepath.Join("sub", "dir")},
		{"unanchored fixture", "."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			hub := filepath.Join(root, "some-hub-HUB")
			worktreeName := "some-worktree"
			worktreeRoot := filepath.Join(hub, worktreeName)
			anchorPath := filepath.Join(worktreeRoot, tt.anchorRel)

			l := &lyxcwd.Location{
				RepoName:     "distinct-repo-name",
				HubPath:      hub,
				WorktreeName: worktreeName,
				AnchorRel:    tt.anchorRel,
			}

			got := WebsterGeometry(l)

			if got.AnchorRoot != anchorPath {
				t.Errorf("WebsterGeometry(l).AnchorRoot = %q; want %q", got.AnchorRoot, anchorPath)
			}
			if got.WorktreeRoot != l.AnchorPath() {
				t.Errorf("WebsterGeometry(l).WorktreeRoot = %q; want %q (l.AnchorPath())", got.WorktreeRoot, l.AnchorPath())
			}
			if tt.anchorRel != "." && got.WorktreeRoot == l.WorktreePath() {
				// The subpath-anchored row must catch a later "consistency fix"
				// that converges webster's field on reed's WorktreePath: the two
				// only coincide when AnchorRel is ".", which this row deliberately
				// is not.
				t.Errorf("WebsterGeometry(l).WorktreeRoot = %q; want != l.WorktreePath() %q", got.WorktreeRoot, l.WorktreePath())
			}
			if want := websterengine.Dir(anchorPath); got.WebsterDir != want {
				t.Errorf("WebsterGeometry(l).WebsterDir = %q; want %q", got.WebsterDir, want)
			}
			if want := websterengine.ReportsDir(anchorPath); got.ReportsDir != want {
				t.Errorf("WebsterGeometry(l).ReportsDir = %q; want %q", got.ReportsDir, want)
			}
			if want := websterengine.ScratchDir(anchorPath); got.ScratchDir != want {
				t.Errorf("WebsterGeometry(l).ScratchDir = %q; want %q", got.ScratchDir, want)
			}
			if want := websterengine.PromptsDir(anchorPath); got.PromptsDir != want {
				t.Errorf("WebsterGeometry(l).PromptsDir = %q; want %q", got.PromptsDir, want)
			}
			if want := fabricengine.StencilsDir(hub); got.StencilsDir != want {
				t.Errorf("WebsterGeometry(l).StencilsDir = %q; want %q", got.StencilsDir, want)
			}
			if want := planparser.PlanDir(anchorPath); got.PlanDir != want {
				t.Errorf("WebsterGeometry(l).PlanDir = %q; want %q", got.PlanDir, want)
			}
		})
	}
}
