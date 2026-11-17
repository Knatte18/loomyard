// worktreetree.go — the read-only tree-id reads a caller uses to tell whether the code checkout changed:
// the tree a `git add -A` would stage, and the paths that differ between two trees.

package fabricengine

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/lymtemp"
)

// WorktreeTreeID returns the id of the git tree that `git add -A` would stage from the code checkout's working tree.
// Uncommitted edits count, so a committed change and the same bytes left uncommitted give the same id.
// The checkout's exclude and ignore files apply to untracked paths, and every tracked file stays in the tree.
// The tree is built in a throwaway index, so the real index, HEAD and every ref are left untouched.
func (f *Fabric) WorktreeTreeID() (string, error) {
	dir, err := lymtemp.MkdirTemp("worktree-tree-index-*")
	if err != nil {
		return "", fmt.Errorf("fabricengine: worktree tree id: create the temporary index: %w", err)
	}
	defer func() { _ = os.RemoveAll(dir) }()

	env := []string{"GIT_INDEX_FILE=" + filepath.Join(dir, "index"), "GIT_LITERAL_PATHSPECS=1"}
	for _, args := range [][]string{{"read-tree", "HEAD"}, {"add", "-A"}} {
		if _, err := gitexec.RunWithEnv(args, f.warpPath, env); err != nil {
			return "", fmt.Errorf("fabricengine: worktree tree id: git %s: %w", args[0], err)
		}
	}
	out, err := gitexec.RunWithEnv([]string{"write-tree"}, f.warpPath, env)
	if err != nil {
		return "", fmt.Errorf("fabricengine: worktree tree id: git write-tree: %w", err)
	}
	return strings.TrimSpace(out), nil
}

// ChangedPathsBetween returns the paths that differ between the git trees fromTree and toTree.
// The paths are checkout-relative with forward slashes, sorted, and a rename counts as a delete and an add.
// A tree id git no longer holds is an error.
func (f *Fabric) ChangedPathsBetween(fromTree, toTree string) ([]string, error) {
	out, err := gitexec.Run([]string{"diff-tree", "-r", "-z", "--name-only", "--no-renames", fromTree, toTree}, f.warpPath)
	if err != nil {
		return nil, fmt.Errorf("fabricengine: changed paths between %s and %s: %w", fromTree, toTree, err)
	}
	var paths []string
	for _, path := range strings.Split(out, "\x00") {
		if path != "" {
			paths = append(paths, path)
		}
	}
	slices.Sort(paths)
	return paths, nil
}
