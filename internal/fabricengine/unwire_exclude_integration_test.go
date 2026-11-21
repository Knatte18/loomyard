// unwire_exclude_integration_test.go covers, over real hubs, the sibling census an unwire takes.
// The census comes before the unwire strips an exclude line.
// The unit test of the strip beside it feeds the census by hand and never runs the walk over the hub's worktrees.
//
// Package fabricengine_test; shares the single TestMain in testmain_test.go.

package fabricengine_test

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fslink"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/lymdirs"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestUnwireJunctions_ExcludeLineFollowsTheSiblingWalk covers which anchored exclude line an unwire keeps.
// A sibling worktree linking the name at the same anchored path keeps the line.
// A sibling holding the name at another anchor, or as a plain directory, leaves the line to be stripped.
func TestUnwireJunctions_ExcludeLineFollowsTheSiblingWalk(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()
	const mover = "mover"
	anchoredLine := "/" + lymdirs.LymDirName

	tests := []struct {
		name string
		// arrange shapes the prime, the sibling of the mover, before the mover unwires.
		arrange  func(t *testing.T, h *hubforge.Hub)
		wantKept bool
	}{
		{
			name:     "A sibling linking the name at the same anchor keeps the line",
			arrange:  func(t *testing.T, h *hubforge.Hub) {},
			wantKept: true,
		},
		{
			name: "A sibling holding the name at another anchor lets the line go",
			arrange: func(t *testing.T, h *hubforge.Hub) {
				hubforge.ReanchorPair(t, h, h.Location.WorktreeName, "sub")
			},
		},
		{
			name: "A sibling holding the name as a plain directory lets the line go",
			arrange: func(t *testing.T, h *hubforge.Hub) {
				link := filepath.Join(h.PrimeWorktree(), lymdirs.LymDirName)
				if err := fslink.Remove(link); err != nil {
					t.Fatalf("remove the prime's %s link: %v", lymdirs.LymDirName, err)
				}
				if err := os.MkdirAll(link, 0o755); err != nil {
					t.Fatalf("make a plain %s directory in the prime: %v", lymdirs.LymDirName, err)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := newAnchorHub(t)
			hubforge.AddPair(t, h, mover)
			tt.arrange(t, h)
			loc, err := lymcwd.NamedWorktree(h.Path, mover)
			if err != nil {
				t.Fatalf("NamedWorktree(%s): %v", mover, err)
			}
			names, err := fabricengine.RepoWiredNames(loc)
			if err != nil {
				t.Fatalf("RepoWiredNames: %v", err)
			}

			if _, err := fabricengine.UnwireJunctions(loc, mover, names); err != nil {
				t.Fatalf("UnwireJunctions: %v", err)
			}

			excludes := gitkit.ExcludeLines(t, h.PairCodeWorktree(mover))
			if kept := slices.Contains(excludes, anchoredLine); kept != tt.wantKept {
				t.Errorf("exclude lines = %v; want %q kept = %v", excludes, anchoredLine, tt.wantKept)
			}
		})
	}
}
