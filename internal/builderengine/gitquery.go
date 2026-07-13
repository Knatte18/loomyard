// gitquery.go implements builder's thin git query layer over
// internal/gitexec: HeadSHA (batch start-SHA capture), ChangedFiles (the
// drift computation's diff source), Dirty (the half-done-work signal), and
// ResetHard (the chain-rollback act — consumed only by chain.go's
// RestartChain, per the discussion's correctness-by-tool-design decision
// that no other caller ever performs a destructive reset). Every helper
// takes an explicit worktree cwd; none resolves geometry itself.

package builderengine

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Knatte18/loomyard/internal/gitexec"
)

// HeadSHA returns worktree's current HEAD commit SHA via `git rev-parse
// HEAD`. A non-zero git exit wraps stderr into the returned error.
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

// ChangedFiles returns every file path that differs between sinceSHA and
// HEAD in worktree, via `git diff --name-only <sinceSHA>..HEAD`. Each path
// is slash-normalized (filepath.ToSlash) and the result is sorted
// lexically, so callers get a platform-independent, deterministic list.
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

// Dirty reports whether worktree has any uncommitted or untracked changes,
// via a non-empty `git status --porcelain`.
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

// ResetHard resets worktree's host repo to sha via `git reset --hard <sha>`.
// It is consumed only by chain.go's RestartChain: the recorded chain-start
// SHA is the ONLY reset target builder ever uses, never a caller-supplied
// SHA string typed elsewhere.
func ResetHard(worktree, sha string) error {
	_, stderr, exitCode, err := gitexec.RunGit([]string{"reset", "--hard", sha}, worktree)
	if err != nil {
		return fmt.Errorf("builder: git reset --hard %s in %s: %w", sha, worktree, err)
	}
	if exitCode != 0 {
		return fmt.Errorf("builder: git reset --hard %s in %s failed: %s", sha, worktree, strings.TrimSpace(stderr))
	}
	return nil
}
