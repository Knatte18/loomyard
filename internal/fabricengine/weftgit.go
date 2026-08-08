// weftgit.go — the weft-git content-sync verbs on Fabric: commitWeft, PushWeft, PullWeft, plus the
// package-level pushWeftAt and commitWeftAt for the detached-push child and board's warp-untethered
// weft:main commit (via Bolt).
// commitWeft's commit carries a Warp-SHA trailer and records the correspondence immediately —
// except on an unborn warp HEAD (see warpHeadSHA), where both are skipped for that one commit.

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

const (
	// weftLockDirName and weftWriteLockFile name fabric's own write-lock
	// location, so every concurrent commitWeft caller contends on the
	// identical file rather than racing past each other.
	weftLockDirName   = ".weft"
	weftWriteLockFile = "weft.write.lock"
	// weftPushLockFile names the absorbing push lock the coalescing loop
	// (coalescePush, in coalesce.go) holds for the whole loop-until-clean push
	// window. It lives inside .weft/ alongside weftWriteLockFile — already
	// git-excluded by seedWeftArtifactExcludes' whole-directory
	// weftLockDirName + "/" entry, so no separate exclude entry is needed for
	// it (Shared Decision lock-artifact-under-weft).
	weftPushLockFile = "fabric.push.lock"
)

// ensureWeftLockDir creates the .weft lock directory and seeds git-exclude
// entries for fabric's lock artifacts.
func (f *Fabric) ensureWeftLockDir() (string, error) {
	return ensureWeftLockDirAt(f.weftPath)
}

// ensureWeftLockDirAt is the no-Fabric form of ensureWeftLockDir for callers
// without a Fabric instance.
func ensureWeftLockDirAt(weftPath string) (string, error) {
	dir := filepath.Join(weftPath, weftLockDirName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("fabricengine: mkdir weft lock dir: %w", err)
	}
	if err := seedWeftArtifactExcludes(weftPath); err != nil {
		return "", err
	}
	return dir, nil
}

// seedWeftArtifactExcludes appends fabric's own operational artifacts — the
// .weft/ lock directory and gitrepo's push lock file — to the weft repo's
// .git/info/exclude, line-exact idempotent (the same discipline as
// seedGitExclude). It no longer carries any module's machine-local patterns:
// every module transient now lives under .lyx (see the Durable-vs-Ephemeral
// State Invariant in CONSTRAINTS.md) and never enters a weft worktree, so
// there is nothing cross-module left to exclude here. Without this seeding,
// every weft worktree that has ever run a weft-git verb reports fabric's own
// artifacts as untracked dirt forever: Remove's no-force dirty gate then
// refuses with a "run lyx fabric sync" hint that a pathspec-scoped sync can
// never satisfy. The exclude file lives in the repo's common gitdir, so one
// seeding covers every linked weft worktree, and — because excludes are
// evaluated at status time — it also heals worktrees that already carry the
// artifacts as untracked (though not ones where a prior sync already
// committed them; see commitWeft's doc comment for that limit).
func seedWeftArtifactExcludes(weftPath string) error {
	stdout, stderr, exitCode, err := gitexec.RunGit(
		[]string{"rev-parse", "--git-path", "info/exclude"},
		weftPath,
	)
	if err != nil {
		return fmt.Errorf("fabricengine: resolve weft git exclude path: %w", err)
	}
	if exitCode != 0 {
		return fmt.Errorf("fabricengine: git rev-parse --git-path info/exclude in %s: %s", weftPath, stderr)
	}

	excludePath := strings.TrimSpace(stdout)
	if !filepath.IsAbs(excludePath) {
		excludePath = filepath.Join(weftPath, excludePath)
	}
	if err := os.MkdirAll(filepath.Dir(excludePath), 0o755); err != nil {
		return fmt.Errorf("fabricengine: mkdir weft exclude dir: %w", err)
	}

	content, err := os.ReadFile(excludePath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("fabricengine: read weft exclude file: %w", err)
	}
	contentStr := string(content)

	entries := []string{weftLockDirName + "/", gitrepo.PushLockFileName}
	for _, entry := range entries {
		present := false
		for _, line := range strings.Split(contentStr, "\n") {
			if strings.TrimSpace(line) == entry {
				present = true
				break
			}
		}
		if present {
			continue
		}
		if contentStr != "" && !strings.HasSuffix(contentStr, "\n") {
			contentStr += "\n"
		}
		contentStr += entry + "\n"
	}

	if string(content) == contentStr {
		return nil
	}
	if err := os.WriteFile(excludePath, []byte(contentStr), 0o644); err != nil {
		return fmt.Errorf("fabricengine: write weft exclude file: %w", err)
	}
	return nil
}

// warpHeadSHA returns the warp repo's HEAD SHA, or reports unborn=true for an
// unborn HEAD (zero commits), preventing regression on first-run paths.
func (f *Fabric) warpHeadSHA() (sha string, unborn bool, err error) {
	sha, err = f.warp.CurrentSHA()
	if err == nil {
		return sha, false, nil
	}
	if errors.Is(err, gitrepo.ErrNoCommits) {
		return "", true, nil
	}
	return "", false, err
}

// weftPathspecFilter filters pathspec entries before staging: keeps git
// pathspec magic (entries starting with ":") untouched and evaluates plain
// paths for matches in the weft worktree or index. Returns filtered entries
// and whether at least one non-magic entry survived. It prevents stale
// entries from reaching git add, which fails its entire invocation on a
// non-matching entry.
func weftPathspecFilter(weftPath string, pathspec []string) (filtered []string, positive bool, err error) {
	for _, entry := range pathspec {
		if strings.HasPrefix(entry, ":") {
			filtered = append(filtered, entry)
			continue
		}
		matches, err := entryMatchesWeft(weftPath, entry)
		if err != nil {
			return nil, false, err
		}
		if matches {
			filtered = append(filtered, entry)
			positive = true
		}
	}
	return filtered, positive, nil
}

// entryMatchesWeft reports whether pathspec entry matches a path in the
// weft repo's index or worktree via git ls-files, the same anchor used by
// git add.
func entryMatchesWeft(weftPath, entry string) (bool, error) {
	stdout, stderr, code, err := gitexec.RunGit([]string{"ls-files", "--cached", "--others", "--", entry}, weftPath)
	if err != nil {
		return false, fmt.Errorf("fabricengine: git ls-files --cached --others -- %s: %w", entry, err)
	}
	if code != 0 {
		return false, fmt.Errorf("fabricengine: git ls-files --cached --others -- %s in %s: %s", entry, weftPath, stderr)
	}
	return strings.TrimSpace(stdout) != "", nil
}

// commitEmptySnapshot lands an empty weft commit with the given commitMessage
// and records the correspondence via RecordCorrespondence.
func (f *Fabric) commitEmptySnapshot(commitMessage, warpSHA string) (sha string, committed bool, err error) {
	sha, err = f.weft.CommitEmpty(commitMessage)
	if err != nil {
		return "", false, err
	}
	if err := f.RecordCorrespondence(warpSHA, sha); err != nil {
		return sha, true, err
	}
	return sha, true, nil
}

// commitWeftLocked stages and commits pathspec-scoped changes in the weft
// worktree. The weft write lock must already be held. When warp has a HEAD,
// the commit carries a Warp-SHA trailer and Snapshot trailers. On unborn
// warp (zero commits), tags are dropped. When snapshotTags is non-empty and
// warp is not unborn, falls through to commitEmptySnapshot for no-op cases
// that still need snapshot tags recorded.
func (f *Fabric) commitWeftLocked(pathspec []string, message string, opts SyncOptions, snapshotTags ...string) (sha string, committed bool, err error) {
	warpSHA, unborn, err := f.warpHeadSHA()
	if err != nil {
		return "", false, fmt.Errorf("fabricengine: warp CurrentSHA: %w", err)
	}

	commitMessage := message
	if !unborn {
		commitMessage = appendWarpSHATrailer(message, warpSHA)
		commitMessage, err = appendSnapshotTrailers(commitMessage, snapshotTags)
		if err != nil {
			return "", false, err
		}
	}

	forceEmptyCommit := len(snapshotTags) > 0 && !unborn

	filteredPathspec, positive, err := weftPathspecFilter(f.weftPath, pathspec)
	if err != nil {
		return "", false, err
	}
	if !positive {
		if forceEmptyCommit {
			return f.commitEmptySnapshot(commitMessage, warpSHA)
		}
		return "", false, nil
	}

	sha, committed, err = f.weft.StageAndCommit(commitMessage, filteredPathspec)
	if err != nil {
		if strings.Contains(err.Error(), "did not match any files") {
			if forceEmptyCommit {
				return f.commitEmptySnapshot(commitMessage, warpSHA)
			}
			return "", false, nil
		}
		return "", false, err
	}
	if !committed {
		if forceEmptyCommit {
			return f.commitEmptySnapshot(commitMessage, warpSHA)
		}
		return "", false, nil
	}
	if unborn {
		return sha, true, nil
	}

	if err := f.RecordCorrespondence(warpSHA, sha); err != nil {
		return sha, true, err
	}

	return sha, true, nil
}

// commitWeft acquires the weft write lock and delegates to commitWeftLocked.
// Returns ("", false, nil) when opts.SkipGit is true. Optional snapshotTags
// force an empty commit to record them.
func (f *Fabric) commitWeft(pathspec []string, message string, opts SyncOptions, snapshotTags ...string) (sha string, committed bool, err error) {
	if opts.SkipGit {
		return "", false, nil
	}

	lockDir, err := f.ensureWeftLockDir()
	if err != nil {
		return "", false, err
	}
	l, err := lock.AcquireWriteLock(filepath.Join(lockDir, weftWriteLockFile))
	if err != nil {
		return "", false, fmt.Errorf("fabricengine: acquire weft write lock: %w", err)
	}
	defer func() { _ = l.Release() }()

	return f.commitWeftLocked(pathspec, message, opts, snapshotTags...)
}

// PushWeft pushes unpushed weft commits via PushCoalesced, honoring SkipGit/SkipPush gating.
func (f *Fabric) PushWeft(opts SyncOptions) error {
	if opts.SkipGit || opts.SkipPush {
		return nil
	}
	return f.weft.PushCoalesced()
}

// PullWeft fast-forwards the weft worktree from its upstream, honoring SkipGit gating.
func (f *Fabric) PullWeft(opts SyncOptions) error {
	if opts.SkipGit {
		return nil
	}
	return f.weft.Pull()
}

// pushWeftAt pushes unpushed commits at weftPath with no Fabric instance,
// used by the detached-push child.
func pushWeftAt(weftPath string, opts SyncOptions) error {
	if opts.SkipGit || opts.SkipPush {
		return nil
	}
	return gitrepo.New(weftPath).PushCoalesced()
}

// commitWeftAt commits wildcard-staged content at weftPath for _board's
// weft:main checkout, with no warp branch or Warp-SHA trailer. Returns
// ("", false, nil) when opts.SkipGit is true. Does not acquire the weft write
// lock; the caller is responsible for synchronization.
func commitWeftAt(weftPath, message string, opts SyncOptions) (sha string, committed bool, err error) {
	if opts.SkipGit {
		return "", false, nil
	}
	return gitrepo.New(weftPath).StageAllAndCommit(message)
}
