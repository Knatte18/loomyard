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
	"github.com/Knatte18/loomyard/internal/lyxdirs"
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
	// lockFilePattern and swapLockFilePattern cover every module's write- and swap-lock artifact
	// anywhere in the weft repo, not only fabric's own. They are load-bearing rather than tidy: a
	// lock file left at the board root by an ordinary board read makes `git status --porcelain`
	// non-empty there forever, and the once-per-hub warp-binding backfill refuses to run against a
	// dirty board (Bolt.Commit is stage-all), so a hub predating the binding would report
	// WarpBindingOutcomeDeferred on every reconcile and never record one.
	lockFilePattern     = "*.lock"
	swapLockFilePattern = "*.swaplock"
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

// seedWeftArtifactExcludes appends the weft repo's never-tracked operational artifacts — the
// .weft/ lock directory, gitrepo's push lock file, lyxdirs.DotLyxDirName, and every module's
// *.lock / *.swaplock write- and swap-lock file
// — to the weft repo's .git/info/exclude, line-exact idempotent (the same
// discipline as seedGitExclude). It is the sole owner of that file's
// content: one `.lyx/` pattern now keeps every module's machine-local
// scratch untracked in the weft repo, replacing the three deep wildcard
// patterns batch 6 deleted, and no .gitignore is ever committed in weft
// either — .git/info/exclude wins there because it needs no commit, no
// pathspec change, and no new file in the weft root. Without this seeding,
// every weft worktree that has ever run a weft-git verb reports fabric's own
// artifacts (and, now, .lyx scratch) as untracked dirt forever: Remove's
// no-force dirty gate then refuses with a "run lyx fabric sync" hint that a
// pathspec-scoped sync can never satisfy. The exclude file lives in the
// repo's common gitdir, so one seeding covers every linked weft worktree,
// and — because excludes are evaluated at status time — it also heals
// worktrees that already carry the artifacts as untracked (though not ones
// where a prior sync already committed them; see commitWeft's doc comment
// for that limit).
func seedWeftArtifactExcludes(weftPath string) error {
	_, _, err := mutateGitExclude(weftPath, func(content string) (string, error) {
		entries := []string{weftLockDirName + "/", gitrepo.PushLockFileName, lyxdirs.DotLyxDirName + "/", lockFilePattern, swapLockFilePattern}
		for _, entry := range entries {
			present := false
			for _, line := range strings.Split(content, "\n") {
				if strings.TrimSpace(line) == entry {
					present = true
					break
				}
			}
			if present {
				continue
			}
			if content != "" && !strings.HasSuffix(content, "\n") {
				content += "\n"
			}
			content += entry + "\n"
		}
		return content, nil
	})
	if err != nil {
		return fmt.Errorf("fabricengine: seed weft artifact excludes in %s: %w", weftPath, err)
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
// entryMatchesWeft's probe excludes ignored files (--exclude-standard),
// closing a latent bug where an entry matching only an ignored file was
// forwarded as if it matched real content: git add on that doomed entry
// then failed the whole invocation with exit 1, toppling a commit that had
// legitimate content in the same call. With the flag, such an entry is
// filtered out here, positive stays false for it, and the commit degrades
// to a clean no-op instead.
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
// --exclude-standard is load-bearing, not cosmetic: without it, `git
// ls-files --cached --others` also matches ignored files, so a stray
// ignored file matching entry would make this report a match that git add
// then refuses, failing the whole staging invocation (see
// weftPathspecFilter's doc comment for the full failure mode this closes).
func entryMatchesWeft(weftPath, entry string) (bool, error) {
	stdout, err := gitexec.Run([]string{"ls-files", "--cached", "--others", "--exclude-standard", "--", entry}, weftPath)
	if err != nil {
		return false, fmt.Errorf("fabricengine: match pathspec entry %s against the weft index and worktree in %s: %w", entry, weftPath, err)
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

// PushResult exists solely to carry the mutation record: push had no result type at all before this
// slice, which is why its envelope had nowhere to put one.
// EVERY push entry point in this package returns this same type, whichever side it pushes and
// whichever primitive it pushes through, so a composed push verb concatenates homogeneous records.
// That is stated as the invariant rather than as a roll-call of the entry points: the roll-call
// spelling ("all three push entry points — PushWeft, PushWarpAt, and CoalescePushBothAt") had
// already gone stale twice by the time anyone read it, first for PushWarpRebaseFreeAt and then for
// PushAnchored, because a list of call sites is maintenance a doc comment cannot win.
type PushResult struct {
	MutationRecord
}

// recordPushIfAdvanced records KindBranchPushed for repo's current branch when hasUnpushedBefore and
// hasUnpushedErr together prove a real true -> false transition across the push call already made:
// hasUnpushedErr must be nil, hasUnpushedBefore must be true (there was something to push), and
// repo.HasUnpushed() sampled now must report false (nothing is unpushed any more).
// Bare "the push call returned nil" is not sufficient evidence: PushCoalesced also returns nil when
// there was nothing to push, and pushRebaseFreeLogged maps a remote rejection to a warning and a nil
// return, so either would otherwise be recorded as a push that never actually happened — the
// commission-direction lie this slice exists to kill.
// If either HasUnpushed sample or CurrentBranch errors, nothing is recorded and the error is not
// propagated: a failure to observe is not a failure to push.
func recordPushIfAdvanced(rec *Mutations, repo *gitrepo.Repo, hasUnpushedBefore bool, hasUnpushedErr error) {
	if hasUnpushedErr != nil || !hasUnpushedBefore {
		return
	}
	stillUnpushed, err := repo.HasUnpushed()
	if err != nil || stillUnpushed {
		return
	}
	branch, err := repo.CurrentBranch()
	if err != nil {
		return
	}
	rec.AppendRef(KindBranchPushed, branch, "")
}

// PushWeft pushes unpushed weft commits via PushCoalesced, honoring SkipGit/SkipPush gating.
func (f *Fabric) PushWeft(opts SyncOptions) (res PushResult, err error) {
	rec := NewMutations(filepath.Dir(f.warpPath))
	defer func() { res.Mutations = rec.Snapshot() }()

	if opts.SkipGit || opts.SkipPush {
		return PushResult{}, nil
	}

	hadUnpushed, hadUnpushedErr := f.weft.HasUnpushed()
	if err := f.weft.PushCoalesced(); err != nil {
		return PushResult{}, err
	}
	recordPushIfAdvanced(rec, f.weft, hadUnpushed, hadUnpushedErr)

	return PushResult{}, nil
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
