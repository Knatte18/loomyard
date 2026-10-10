// dotgit.go holds the `.git` reader: a directory's Geometry, the repository check over it and the walk up to a worktree root.

package dotgit

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Geometry is where a directory's git data lives: GitDir is the directory holding HEAD and the worktree's own state, CommonDir the directory holding objects and refs.
// Both are absolute and equal for a plain clone; they differ for a linked worktree.
type Geometry struct {
	GitDir    string
	CommonDir string
}

// ErrNotRepository is returned, wrapped with the offending path, when a directory has no usable `.git` entry.
var ErrNotRepository = errors.New("dotgit: not a git repository")

// Read reads dir's `.git` entry into a Geometry; dir must be absolute.
// A `.git` directory is the git dir; a `.git` file is a gitfile whose `gitdir:` line, relative to dir when not absolute, names it.
// The git dir's `commondir` file, relative to the git dir when not absolute, names the common dir; without the file the git dir is the common dir.
// A missing `.git` entry, a gitfile without a `gitdir:` line, or a `gitdir:` or `commondir` target that does not exist is ErrNotRepository.
func Read(dir string) (Geometry, error) {
	entry := filepath.Join(dir, ".git")
	info, err := os.Stat(entry)
	if err != nil {
		return Geometry{}, fmt.Errorf("%w: %s: %v", ErrNotRepository, entry, err)
	}

	gitDir := entry
	if !info.IsDir() {
		target, err := readGitFile(entry)
		if err != nil {
			return Geometry{}, err
		}
		gitDir = resolveAgainst(dir, target)
		if err := requireDir(gitDir); err != nil {
			return Geometry{}, fmt.Errorf("%w: %s: gitdir target: %v", ErrNotRepository, entry, err)
		}
	}

	commonDir, err := readCommonDir(gitDir)
	if err != nil {
		return Geometry{}, err
	}
	return Geometry{GitDir: gitDir, CommonDir: commonDir}, nil
}

// IsRepository reports whether g is a usable repository: HEAD present in the git dir, and the objects and refs directories present in the common dir.
func IsRepository(g Geometry) bool {
	if g.GitDir == "" || g.CommonDir == "" {
		return false
	}
	head, err := os.Stat(filepath.Join(g.GitDir, "HEAD"))
	if err != nil || head.IsDir() {
		return false
	}
	return requireDir(filepath.Join(g.CommonDir, "objects")) == nil && requireDir(filepath.Join(g.CommonDir, "refs")) == nil
}

// FindRoot walks up from start (absolute) to the nearest directory that is a worktree root, as `git rev-parse --show-toplevel` does, and returns it with its Geometry.
// A directory whose `.git` is a directory ends the walk only when IsRepository holds; otherwise the walk skips it, as git does.
// A directory whose `.git` is a file ends the walk too, but one whose target is missing or fails IsRepository is ErrNotRepository and never resolves to an enclosing repository.
// Reaching the filesystem root without a repository is ErrNotRepository.
func FindRoot(start string) (root string, g Geometry, err error) {
	dir := filepath.Clean(start)
	for {
		entry := filepath.Join(dir, ".git")
		info, statErr := os.Stat(entry)
		switch {
		case statErr == nil && info.IsDir():
			if found, readErr := Read(dir); readErr == nil && IsRepository(found) {
				return dir, found, nil
			}
		case statErr == nil:
			found, readErr := Read(dir)
			if readErr != nil {
				return "", Geometry{}, readErr
			}
			if !IsRepository(found) {
				return "", Geometry{}, fmt.Errorf("%w: %s: gitfile target is not a repository", ErrNotRepository, entry)
			}
			return dir, found, nil
		case !errors.Is(statErr, os.ErrNotExist):
			return "", Geometry{}, fmt.Errorf("%w: %s: %v", ErrNotRepository, entry, statErr)
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return "", Geometry{}, fmt.Errorf("%w: no .git above %s", ErrNotRepository, start)
		}
		dir = parent
	}
}

// readGitFile returns the path on the `gitdir:` line of the gitfile at path, as written.
func readGitFile(path string) (string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("%w: %s: %v", ErrNotRepository, path, err)
	}
	for _, line := range strings.Split(string(content), "\n") {
		if target, ok := strings.CutPrefix(strings.TrimSpace(line), "gitdir:"); ok {
			return strings.TrimSpace(target), nil
		}
	}
	return "", fmt.Errorf("%w: %s: no gitdir line", ErrNotRepository, path)
}

// readCommonDir returns the common dir of gitDir: the directory its `commondir` file names, or gitDir itself when it has none.
func readCommonDir(gitDir string) (string, error) {
	content, err := os.ReadFile(filepath.Join(gitDir, "commondir"))
	if errors.Is(err, os.ErrNotExist) {
		return gitDir, nil
	}
	if err != nil {
		return "", fmt.Errorf("%w: %s: %v", ErrNotRepository, gitDir, err)
	}

	commonDir := resolveAgainst(gitDir, strings.TrimSpace(string(content)))
	if err := requireDir(commonDir); err != nil {
		return "", fmt.Errorf("%w: %s: commondir target: %v", ErrNotRepository, gitDir, err)
	}
	return commonDir, nil
}

// resolveAgainst returns target as an absolute clean path, joined onto base when target is relative.
func resolveAgainst(base, target string) string {
	target = filepath.FromSlash(target)
	if filepath.IsAbs(target) {
		return filepath.Clean(target)
	}
	return filepath.Join(base, target)
}

func requireDir(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return errors.New("not a directory")
	}
	return nil
}
