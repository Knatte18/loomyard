// childrecipe.go declares the child-recipe record: the recipe a batten run seeded its task worktree's run with.
// The record sits in the run's durable directory, so it outlives the task worktree.

package shedrun

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/lymdirs"
)

// childRecipeFileName is the filename of the child-recipe record, whose path ChildRecipeFile returns.
const childRecipeFileName = "child-recipe"

// ChildRecipeFile returns the path of the durable child-recipe record of runID, under RunDir(l, runID).
func ChildRecipeFile(l *lymcwd.Location, runID string) string {
	return filepath.Join(RunDir(l, runID), childRecipeFileName)
}

// ChildRecipeRel returns the worktree-anchor-relative form of ChildRecipeFile's path, for a records commit's pathspec.
func ChildRecipeRel(l *lymcwd.Location, runID string) string {
	return filepath.Join(lymdirs.LymDirName, shedDirName, runSegment(l, runID), childRecipeFileName)
}

// WriteChildRecipe records recipe as the recipe runID seeded its task worktree's run with, replacing any recorded before.
func WriteChildRecipe(l *lymcwd.Location, runID, recipe string) error {
	path := ChildRecipeFile(l, runID)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(recipe+"\n"), 0o644)
}

// ReadChildRecipe returns the recipe recorded for runID's task worktree run, empty when none is recorded.
func ReadChildRecipe(l *lymcwd.Location, runID string) (string, error) {
	data, err := os.ReadFile(ChildRecipeFile(l, runID))
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}
