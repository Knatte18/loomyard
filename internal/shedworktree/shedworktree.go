// Package shedworktree reads a task's code worktree through fabric's read-only reads.
// It serves the arbiter's worktree seam, where the arbiter may make one fix of its own,
// and the snapshot of the worktree a tackler must leave unchanged.
package shedworktree

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/shedarbiter"
	"github.com/Knatte18/loomyard/internal/sheddriver"
)

// ArbiterSeam returns the arbiter's seam over the task's code worktree at location.
func ArbiterSeam(location *lymcwd.Location) *shedarbiter.WorktreeSeam {
	return &shedarbiter.WorktreeSeam{
		Snapshot: func() (sheddriver.WorktreeSnapshot, error) { return Snapshot(location) },
		IsAncestor: func(sha, ref string) (bool, error) {
			fab, err := fabricengine.Open(location)
			if err != nil {
				return false, err
			}
			return fab.IsAncestor(sha, ref)
		},
		Changes: func() (map[string]string, error) {
			fab, err := fabricengine.Open(location)
			if err != nil {
				return nil, err
			}
			paths, err := fab.CodeWorktreeChanges()
			if err != nil {
				return nil, err
			}
			changes := make(map[string]string, len(paths))
			for _, path := range paths {
				hash, err := hashFileOrDeleted(filepath.Join(location.WorktreePath(), filepath.FromSlash(path)))
				if err != nil {
					return nil, fmt.Errorf("hash uncommitted path %s: %w", path, err)
				}
				changes[path] = hash
			}
			return changes, nil
		},
		CommitLog: func(from, to string) ([]string, error) {
			fab, err := fabricengine.Open(location)
			if err != nil {
				return nil, err
			}
			return fab.CommitLog(from, to)
		},
	}
}

// Snapshot reads the state a session must leave unchanged in the task's code worktree:
// HEAD, branch and the SHA-256 of each tracked dirty path.
// The records side is left out, since the run's own records change under every step.
func Snapshot(location *lymcwd.Location) (sheddriver.WorktreeSnapshot, error) {
	fab, err := fabricengine.Open(location)
	if err != nil {
		return sheddriver.WorktreeSnapshot{}, err
	}
	head, err := fab.HeadSHA()
	if err != nil {
		return sheddriver.WorktreeSnapshot{}, err
	}
	branch, err := fab.CurrentBranch()
	if err != nil {
		return sheddriver.WorktreeSnapshot{}, err
	}
	paths, err := fab.DirtyPaths()
	if err != nil {
		return sheddriver.WorktreeSnapshot{}, err
	}
	codeRoot := location.WorktreePath() + string(filepath.Separator)
	dirty := map[string]string{}
	for _, path := range paths {
		if !strings.HasPrefix(path, codeRoot) {
			continue
		}
		hash, err := hashFileOrDeleted(path)
		if err != nil {
			return sheddriver.WorktreeSnapshot{}, fmt.Errorf("hash dirty path %s: %w", path, err)
		}
		dirty[path] = hash
	}
	return sheddriver.WorktreeSnapshot{Head: head, Branch: branch, Dirty: dirty}, nil
}

// hashFileOrDeleted returns the hex SHA-256 of the file at path, or "deleted" when the file does not exist.
func hashFileOrDeleted(path string) (string, error) {
	content, err := os.ReadFile(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return "deleted", nil
	case err != nil:
		return "", err
	}
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:]), nil
}
