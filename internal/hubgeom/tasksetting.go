// tasksetting.go tells dobbyengine's per-task lookup the chain of worktrees a task's beam setting is read through.

package hubgeom

import (
	"errors"
	"fmt"
	"io/fs"
	"os"

	"github.com/Knatte18/loomyard/internal/dobbyengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lymcwd"
)

// TaskSettingChain returns the chain the per-task beam settings of the worktree slug are looked up through.
// It lists the anchor of slug's worktree, then each parent's that ParentOf names, ending with the prime's;
// an empty slug names the prime alone.
// A worktree with no directory in the hub on this machine is left out, so its parent's value applies.
// A cycle among the parents ends the walk at the prime.
// The board is the hub's Bolt checkout, whose default layer applies when no worktree sets a key.
// A parent that does not read, or a worktree whose anchor does not resolve, is an error naming it.
func TaskSettingChain(prime *lymcwd.Location, slug string) (dobbyengine.TaskSettingChain, error) {
	var chain dobbyengine.TaskSettingChain
	seen := map[string]bool{}
	for slug != "" && !seen[slug] {
		seen[slug] = true
		root := fabricengine.WorktreePath(prime, slug)
		_, err := os.Stat(root)
		switch {
		case err == nil:
			worktree, err := lymcwd.NamedWorktree(prime.HubPath, slug)
			if err != nil {
				return dobbyengine.TaskSettingChain{}, fmt.Errorf("hubgeom: resolve the worktree of %q for its beam settings: %w", slug, err)
			}
			chain.Anchors = append(chain.Anchors, worktree.AnchorPath())
		case !errors.Is(err, fs.ErrNotExist):
			return dobbyengine.TaskSettingChain{}, fmt.Errorf("hubgeom: stat the worktree of %q for its beam settings: %w", slug, err)
		}
		if slug, err = ParentOf(prime, slug); err != nil {
			return dobbyengine.TaskSettingChain{}, err
		}
	}
	chain.Anchors = append(chain.Anchors, prime.AnchorPath())
	chain.Board = fabricengine.BoltDir(prime.HubPath)
	return chain, nil
}
