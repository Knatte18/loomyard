// gitexclude.go owns every read-modify-write of a repository's `.git/info/exclude`, on the warp and the weft side.
// That file is repo-wide, not per-worktree: git resolves `info/exclude` to the common gitdir.
// Two `lym fabric` verbs running in two worktrees of one hub therefore edit the same bytes,
// and an unsynchronised read-modify-write loses whichever update lands first.
// `os.WriteFile` truncates before it writes.
// A concurrent reader can therefore observe an empty file and write back that emptiness plus its own additions.
// Doing so destroys the operator's own exclude patterns along with fabric's junction exclusions.

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
const gitExcludeLockFileName = "exclude.lym.lock"

// resolveGitExcludePath returns the absolute path of the `.git/info/exclude` belonging to the
// repository checked out at repoDir.
// It joins onto the repository's common gitdir,
// so a linked worktree resolves to the repository's exclude file rather than a private one.
func resolveGitExcludePath(repoDir string) (string, error) {
	commonDir, err := gitrepo.New(repoDir).CommonDir()
	if err != nil {
		return "", fmt.Errorf("resolve git exclude path for %q: %w", repoDir, err)
	}
	return filepath.Join(commonDir, "info", "exclude"), nil
}

// mutateGitExclude applies rewrite to the `.git/info/exclude` of the repository checked out at
// repoDir, serialised against every other fabric process editing the same repo and replaced atomically.
// It returns the exclude file's resolved path on every return where it is known, error returns included.
// It reports whether the file's content changed: a rewrite that returns its input unchanged writes nothing.
// The returned path lets a caller record a KindFileWritten mutation entry against the file written.
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

// ExcludeAnchoredDir makes git ignore the directory dirName at the anchor subpath anchorRel of the
// repository checked out at worktreeDir.
// It returns the exclude file's resolved path and whether it wrote anything.
// It first asks `git check-ignore` whether the directory is already ignored, from any source.
// Such sources include a tracked `.gitignore` and the global excludes; when one covers it, nothing is written.
// Otherwise it appends `/<anchorRel>/<dirName>/` to the shared `info/exclude`.
// The write holds the same lock and atomic replace as every `lym fabric` verb.
// The leading slash anchors the entry to this anchor's own directory, where a bare name would match at any depth;
// the trailing slash is kept because dirName is a real directory, not a junction.
// git reports a directory holding tracked content as not ignored even when a rule covers it,
// so the caller must not reach this function in that case.
// It takes no mutation recorder, since it is not a fabric CLI verb.
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

// scratchDirName is the directory name agents keep scratch files under, at a warp worktree's anchor.
// The entry is a fabric-wired link into the weft-side `.lym` that agents write through.
// It is agent scratch, not lym state, so it is declared here and not in `internal/lymdirs`.
const scratchDirName = ".scratch"

// writeFileAtomically replaces path with content through a same-directory temp file and a rename.
// A concurrent reader observes either the whole old file or the whole new one.
// It never sees the empty window that os.WriteFile's truncate-then-write opens.
func writeFileAtomically(path string, content []byte) error {
	temp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".lym-*")
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
