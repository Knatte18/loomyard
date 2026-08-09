// gitexclude.go owns every read-modify-write of a git repository's `.git/info/exclude`, on both the
// warp and the weft side.
// It exists because that file is repo-wide rather than per-worktree — git resolves `info/exclude` to
// the COMMON gitdir — so two `lyx fabric` verbs running in two worktrees of one hub edit the same
// bytes, and an unsynchronised read-modify-write between them loses whichever update lands first.
// Worse than losing an update: `os.WriteFile` truncates before it writes, so a concurrent reader can
// observe an EMPTY file and then write back that emptiness plus its own additions, destroying the
// operator's own exclude patterns along with fabric's junction exclusions.

package fabricengine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/lock"
)

// gitExcludeLockFileName is the flock file guarding `.git/info/exclude`.
// It sits beside the file it guards, inside the common gitdir, so it is repo-wide by the same
// mechanism the exclude file is and is never a candidate for tracking.
const gitExcludeLockFileName = "exclude.lyx.lock"

// resolveGitExcludePath returns the absolute path of the `.git/info/exclude` belonging to the
// repository checked out at repoDir.
// It asks git rather than composing the path, so a linked worktree resolves to its repo's COMMON
// gitdir instead of its own private one.
func resolveGitExcludePath(repoDir string) (string, error) {
	stdout, stderr, exitCode, err := gitexec.RunGit(
		[]string{"rev-parse", "--git-path", "info/exclude"},
		repoDir,
	)
	if err != nil {
		return "", fmt.Errorf("resolve git exclude path for %q: %w", repoDir, err)
	}
	if exitCode != 0 {
		return "", fmt.Errorf("resolve git exclude path for %q failed (git exit %d): %s",
			repoDir, exitCode, strings.TrimSpace(stderr))
	}

	excludePath := strings.TrimSpace(stdout)
	if !filepath.IsAbs(excludePath) {
		excludePath = filepath.Join(repoDir, excludePath)
	}
	return excludePath, nil
}

// mutateGitExclude applies rewrite to the `.git/info/exclude` of the repository checked out at
// repoDir, serialised against every other fabric process editing the same repo and replaced
// atomically.
// It reports whether the file's content actually changed;
// a rewrite that returns its input unchanged writes nothing.
//
// rewrite is called with the file's current content ("" when the file does not exist yet) while the
// lock is held, so any decision it makes about the repo's state — a sibling-worktree census, say —
// is made under the same serialisation as the write it feeds.
func mutateGitExclude(repoDir string, rewrite func(content string) (string, error)) (bool, error) {
	excludePath, err := resolveGitExcludePath(repoDir)
	if err != nil {
		return false, err
	}

	excludeDir := filepath.Dir(excludePath)
	if err := os.MkdirAll(excludeDir, 0o755); err != nil {
		return false, fmt.Errorf("mkdir for exclude file: %w", err)
	}

	excludeLock, err := lock.AcquireWriteLock(filepath.Join(excludeDir, gitExcludeLockFileName))
	if err != nil {
		return false, fmt.Errorf("lock exclude file: %w", err)
	}
	defer func() { _ = excludeLock.Release() }()

	existing, err := os.ReadFile(excludePath)
	if err != nil && !os.IsNotExist(err) {
		return false, fmt.Errorf("read exclude file: %w", err)
	}

	updated, err := rewrite(string(existing))
	if err != nil {
		return false, err
	}
	if updated == string(existing) {
		return false, nil
	}

	if err := writeFileAtomically(excludePath, []byte(updated)); err != nil {
		return false, err
	}
	return true, nil
}

// writeFileAtomically replaces path with content via a same-directory temp file and a rename, so a
// concurrent reader observes either the whole old file or the whole new one and never the empty
// window os.WriteFile's truncate-then-write opens.
func writeFileAtomically(path string, content []byte) error {
	temp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".lyx-*")
	if err != nil {
		return fmt.Errorf("create temp file beside %s: %w", path, err)
	}
	tempPath := temp.Name()

	if _, err := temp.Write(content); err != nil {
		_ = temp.Close()
		_ = os.Remove(tempPath)
		return fmt.Errorf("write temp file %s: %w", tempPath, err)
	}
	if err := temp.Close(); err != nil {
		_ = os.Remove(tempPath)
		return fmt.Errorf("close temp file %s: %w", tempPath, err)
	}
	if err := os.Chmod(tempPath, 0o644); err != nil {
		_ = os.Remove(tempPath)
		return fmt.Errorf("chmod temp file %s: %w", tempPath, err)
	}
	if err := os.Rename(tempPath, path); err != nil {
		_ = os.Remove(tempPath)
		return fmt.Errorf("replace %s: %w", path, err)
	}
	return nil
}
