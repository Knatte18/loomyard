// gitquery.go implements builder's thin git query layer over internal/gitexec: HeadSHA (batch
// start-SHA capture), ChangedFiles (the drift computation's diff source), and Dirty (the
// half-done-work signal).
// Every helper takes an explicit worktree cwd;
// none resolves geometry itself.

package builderengine

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Knatte18/loomyard/internal/gitexec"
)

// HeadSHA returns worktree's current HEAD commit SHA via `git rev-parse HEAD`.
// A non-zero git exit wraps stderr into the returned error.
func HeadSHA(worktree string) (string, error) {
	stdout, stderr, exitCode, err := gitexec.RunGit([]string{"rev-parse", "HEAD"}, worktree)
	if err != nil {
		return "", fmt.Errorf("builder: git rev-parse HEAD in %s: %w", worktree, err)
	}
	if exitCode != 0 {
		return "", fmt.Errorf("builder: git rev-parse HEAD in %s failed: %s", worktree, strings.TrimSpace(stderr))
	}
	return strings.TrimSpace(stdout), nil
}

// ChangedFiles returns every file path that differs between sinceSHA and HEAD in worktree,
// slash-normalized and sorted lexically.
func ChangedFiles(worktree, sinceSHA string) ([]string, error) {
	rangeArg := sinceSHA + "..HEAD"
	stdout, stderr, exitCode, err := gitexec.RunGit([]string{"diff", "--name-only", rangeArg}, worktree)
	if err != nil {
		return nil, fmt.Errorf("builder: git diff --name-only %s in %s: %w", rangeArg, worktree, err)
	}
	if exitCode != 0 {
		return nil, fmt.Errorf("builder: git diff --name-only %s in %s failed: %s", rangeArg, worktree, strings.TrimSpace(stderr))
	}

	var files []string
	for _, line := range strings.Split(stdout, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		files = append(files, filepath.ToSlash(line))
	}
	sort.Strings(files)
	return files, nil
}

// Dirty reports whether worktree has any uncommitted or untracked changes, via a non-empty `git
// status --porcelain`.
func Dirty(worktree string) (bool, error) {
	stdout, stderr, exitCode, err := gitexec.RunGit([]string{"status", "--porcelain"}, worktree)
	if err != nil {
		return false, fmt.Errorf("builder: git status --porcelain in %s: %w", worktree, err)
	}
	if exitCode != 0 {
		return false, fmt.Errorf("builder: git status --porcelain in %s failed: %s", worktree, strings.TrimSpace(stderr))
	}
	return strings.TrimSpace(stdout) != "", nil
}
