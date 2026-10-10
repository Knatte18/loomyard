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
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/lock"
)

// gitExcludeLockFileName is the flock file guarding `.git/info/exclude`.
// It sits beside the file it guards, inside the common gitdir, so it is repo-wide by the same
// mechanism the exclude file is and is never a candidate for tracking.
const gitExcludeLockFileName = "exclude.lyx.lock"

// resolveGitExcludePath returns the absolute path of the `.git/info/exclude` belonging to the
// repository checked out at repoDir.
// It joins onto the repository's COMMON gitdir, so a linked worktree resolves to its repo's exclude file instead of a private one.
func resolveGitExcludePath(repoDir string) (string, error) {
	commonDir, err := gitrepo.New(repoDir).CommonDir()
	if err != nil {
		return "", fmt.Errorf("resolve git exclude path for %q: %w", repoDir, err)
	}
	return filepath.Join(commonDir, "info", "exclude"), nil
}

// mutateGitExclude applies rewrite to the `.git/info/exclude` of the repository checked out at
// repoDir, serialised against every other fabric process editing the same repo and replaced
// atomically.
// It returns the exclude file's own resolved path on every return, including the error ones where it
// is known, and reports whether the file's content actually changed;
// a rewrite that returns its input unchanged writes nothing.
// The resolved path is what a caller needs to record a KindFileWritten mutation entry against the
// file actually written, since resolveGitExcludePath's result never otherwise leaves this function.
//
// rewrite is called with the file's current content ("" when the file does not exist yet) while the
// lock is held, so any decision it makes about the repo's state — a sibling-worktree census, say —
// is made under the same serialisation as the write it feeds.
func mutateGitExclude(repoDir string, rewrite func(content string) (string, error)) (excludePath string, changed bool, err error) {
	excludePath, err = resolveGitExcludePath(repoDir)
	if err != nil {
		return "", false, err
	}

	excludeDir := filepath.Dir(excludePath)
	if err := os.MkdirAll(excludeDir, 0o755); err != nil {
		return excludePath, false, fmt.Errorf("mkdir for exclude file: %w", err)
	}

	excludeLock, err := lock.AcquireWriteLock(filepath.Join(excludeDir, gitExcludeLockFileName))
	if err != nil {
		return excludePath, false, fmt.Errorf("lock exclude file: %w", err)
	}
	defer func() { _ = excludeLock.Release() }()

	existing, err := os.ReadFile(excludePath)
	if err != nil && !os.IsNotExist(err) {
		return excludePath, false, fmt.Errorf("read exclude file: %w", err)
	}

	updated, err := rewrite(string(existing))
	if err != nil {
		return excludePath, false, err
	}
	if updated == string(existing) {
		return excludePath, false, nil
	}

	if err := writeFileAtomically(excludePath, []byte(updated)); err != nil {
		return excludePath, false, err
	}
	return excludePath, true, nil
}

// ExcludeAnchoredDir makes git ignore the directory dirName at the anchor subpath anchorRel of the repository checked out at worktreeDir,
// and returns the exclude file's resolved path and whether it wrote anything.
// It first asks `git check-ignore` whether the directory is already ignored — by a tracked `.gitignore`, the operator's global excludes or any other source — and writes nothing when it is.
// Otherwise it appends `/<anchorRel>/<dirName>/` to the shared `info/exclude` through mutateGitExclude, so the write holds the same lock and atomic replace as every `lyx fabric` verb.
// The leading slash anchors the entry to this anchor's own directory, where a bare name would match at any depth;
// the trailing slash is kept because dirName is a real directory, not a junction.
// git reports a directory holding tracked content as not ignored even when a rule covers it,
// so the caller must not reach this function in that case.
// It takes no *Mutations recorder: it is not a fabric CLI verb.
func ExcludeAnchoredDir(worktreeDir, anchorRel, dirName string) (excludePath string, changed bool, err error) {
	relDir := dirName
	if slashed := filepath.ToSlash(anchorRel); slashed != "." && slashed != "" {
		relDir = slashed + "/" + dirName
	}

	_, err = gitexec.Run([]string{"check-ignore", "-q", "--", relDir + "/"}, worktreeDir)
	if err == nil {
		return "", false, nil
	}
	var gitErr *gitexec.GitError
	if !errors.As(err, &gitErr) || gitErr.ExitCode != 1 {
		return "", false, fmt.Errorf("check-ignore %q in %q: %w", relDir, worktreeDir, err)
	}

	entry := "/" + relDir + "/"
	return mutateGitExclude(worktreeDir, func(content string) (string, error) {
		for _, line := range strings.Split(content, "\n") {
			if strings.TrimSpace(line) == entry {
				return content, nil
			}
		}
		if content != "" && !strings.HasSuffix(content, "\n") {
			content += "\n"
		}
		return content + entry + "\n", nil
	})
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
