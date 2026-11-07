// recordsgit.go holds the weft-git content-sync verbs on Fabric: commitWeft, PushWeft and PullWeft.
// It also holds commitWeftAt, which makes board's commit on weft:main without a warp tether, via Bolt.
// A commitWeft commit carries a Warp-SHA trailer and records the correspondence at once.
// On an unborn warp HEAD, both are skipped for that one commit.

package fabricengine

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/lymdirs"
)

const (
	// weftLockDirName and weftWriteLockFile name the write lock's location,
	// so every concurrent committer contends on the same file rather than racing past the others.
	weftLockDirName   = ".weft"
	weftWriteLockFile = "weft.write.lock"
	// weftPushLockFile names the push lock that coalescePush holds for the whole loop-until-clean push window.
	// It sits in .weft/ with weftWriteLockFile, so the .weft/ git exclude already covers it.
	weftPushLockFile = "fabric.push.lock"
	// retiredLockFilePattern and retiredSwapLockFilePattern are the broad excludes seedWeftArtifactExcludes removes.
	// Keeping them would hide a lock file that strays into a tracked tree from git status.
	retiredLockFilePattern     = "*.lock"
	retiredSwapLockFilePattern = "*.swaplock"
)

// legacyLockFileNames are lock files that earlier versions wrote beside tracked files in the weft checkouts:
// in the Bolt root, and beside the review records of a run directory.
// No current code writes them.
// They stay excluded by name, so a leftover one never enters a stage-all board commit or a records commit.
var legacyLockFileNames = []string{".gitrepo-push.lock", "board.lock", "board.push.lock", "board.json.swaplock", "tasks.json.swaplock", "notes.json.swaplock", "generation-record.json.lock", "settle-record.json.lock", "settle-record.patch.lock"}

// ensureWeftLockDir creates the .weft lock directory and seeds git-exclude
// entries for fabric's lock artifacts.
func (f *Fabric) ensureWeftLockDir() (string, error) {
	return ensureWeftLockDirAt(f.weftPath)
}

// RecordsLockDirPath returns the lock directory path inside the records worktree at recordsWorktreePath.
// It builds the path only and does not create the directory.
func RecordsLockDirPath(recordsWorktreePath string) string {
	return filepath.Join(recordsWorktreePath, weftLockDirName)
}

// ensureWeftLockDirAt creates the .weft lock directory under weftPath.
// It also seeds the git-exclude entries for fabric's lock artifacts, so callers need no Fabric instance.
func ensureWeftLockDirAt(weftPath string) (string, error) {
	dir := RecordsLockDirPath(weftPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("fabricengine: mkdir weft lock dir: %w", err)
	}
	if err := seedWeftArtifactExcludes(weftPath); err != nil {
		return "", err
	}
	return dir, nil
}

// HoldRecordsWriteLock takes the combined records write lock of the records worktree at recordsWorktreePath exclusively.
// It waits at most wait for the lock.
// Committers take the same lock, so a holder keeps a records commit from running while it works.
// On success it returns the release function and held true.
// When wait runs out first, it returns a nil release and held false.
// A wait of zero reports held false without trying.
// An error reports a failed lock directory creation or lock call.
func HoldRecordsWriteLock(recordsWorktreePath string, wait time.Duration) (release func() error, held bool, err error) {
	lockDir, err := ensureWeftLockDirAt(recordsWorktreePath)
	if err != nil {
		return nil, false, err
	}
	l, acquired, err := lock.AcquireWriteLockWithin(filepath.Join(lockDir, weftWriteLockFile), wait)
	if err != nil {
		return nil, false, fmt.Errorf("fabricengine: hold records write lock in %s: %w", recordsWorktreePath, err)
	}
	if !acquired {
		return nil, false, nil
	}
	return l.Release, true, nil
}

// seedWeftArtifactExcludes adds the weft repo's never-tracked operational artifacts to its .git/info/exclude.
// They are the .weft/ lock directory, the lymdirs.DotLymDirName directory, and the legacy lock file names.
// Each missing entry is appended line-exact and idempotently;
// the retired *.lock and *.swaplock lines are removed.
// Exclude is used instead of a committed .gitignore: it needs no commit and no new file in the weft root.
// Without these excludes, a weft worktree reports the artifacts as untracked dirt, which makes it look dirty.
// The exclude file lives in the repo's common gitdir, so one seeding covers every linked weft worktree.
// Excludes are evaluated at status time, so the seeding also clears artifacts already shown as untracked.
// It cannot untrack files that a prior sync already committed.
func seedWeftArtifactExcludes(weftPath string) error {
	_, _, err := mutateGitExclude(weftPath, func(content string) (string, error) {
		present := map[string]bool{}
		var kept []string
		for _, line := range strings.Split(content, "\n") {
			trimmed := strings.TrimSpace(line)
			if trimmed == retiredLockFilePattern || trimmed == retiredSwapLockFilePattern {
				continue
			}
			present[trimmed] = true
			kept = append(kept, line)
		}
		content = strings.Join(kept, "\n")
		entries := append([]string{weftLockDirName + "/", lymdirs.DotLymDirName + "/"}, legacyLockFileNames...)
		for _, entry := range entries {
			if present[entry] {
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

// warpHeadSHA returns the warp repo's HEAD SHA.
// On an unborn HEAD with zero commits, it reports unborn as true and returns no error.
func (f *Fabric) warpHeadSHA() (sha string, unborn bool, err error) {
	sha, err = f.code.CurrentSHA()
	if err == nil {
		return sha, false, nil
	}
	if errors.Is(err, gitrepo.ErrNoCommits) {
		return "", true, nil
	}
	return "", false, err
}

// weftPathspecFilter keeps the pathspec entries that git add can stage in the weft worktree.
// Magic entries, which start with ":", pass through unchanged.
// A plain entry is kept only when it matches a path in the weft index or worktree, with ignored files not counted.
// Dropping the others matters because git add fails its whole invocation on one non-matching entry.
// It returns the filtered entries and whether at least one plain entry survived.
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

// entryMatchesWeft reports whether pathspec entry matches a path in the weft repo's index or worktree.
// It asks git ls-files, the same matching git add uses.
// --exclude-standard is required.
// Without it, an ignored file matching the entry counts as a match.
// git add then refuses the entry and fails the whole staging invocation.
func entryMatchesWeft(weftPath, entry string) (bool, error) {
	stdout, err := gitexec.Run([]string{"ls-files", "--cached", "--others", "--exclude-standard", "--", entry}, weftPath)
	if err != nil {
		return false, fmt.Errorf("fabricengine: match pathspec entry %s against the weft index and worktree in %s: %w", entry, weftPath, err)
	}
	return strings.TrimSpace(stdout) != "", nil
}

// commitEmptySnapshot lands an empty weft commit with commitMessage and records its correspondence to warpSHA.
func (f *Fabric) commitEmptySnapshot(commitMessage, warpSHA string) (sha string, committed bool, err error) {
	sha, err = f.records.CommitEmpty(commitMessage)
	if err != nil {
		return "", false, err
	}
	if err := f.RecordCorrespondence(warpSHA, sha); err != nil {
		return sha, true, err
	}
	return sha, true, nil
}

// commitWeftLocked stages and commits pathspec-scoped changes in the weft worktree.
// The weft write lock must already be held.
// When warp has a HEAD, the commit carries a Warp-SHA trailer and Snapshot trailers.
// On an unborn warp, with zero commits, the tags are dropped.
// With snapshotTags set and warp past its first commit, a no-op commit still records the tags.
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

	sha, committed, err = f.records.StageAndCommit(commitMessage, filteredPathspec)
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

// commitWeft stages the pathspec and commits it while holding the weft write lock.
// Returns ("", false, nil) when opts.SkipGit is true.
// Snapshot tags force an empty commit when nothing is staged, so the tags are still recorded.
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

// PushResult carries the mutation record of a push.
// Every push entry point in this package returns this type, whatever side it pushes and whatever primitive it uses.
// That lets a composed push verb concatenate homogeneous records.
//
// CodePushSkipped is non-empty only when the push ran in the hub's prime,
// and it left the code side's unpushed commits alone.
// It names the branch and says it was left for the operator to push.
type PushResult struct {
	MutationRecord
	CodePushSkipped string `json:"code_push_skipped,omitempty"`
}

// recordPushIfAdvanced records KindBranchPushed for repo's current branch.
// It does so only when the unpushed state proves a real true -> false transition across the push call already made.
// hasUnpushedErr must be nil, hasUnpushedBefore must be true, and repo.HasUnpushed() sampled now must report false.
// A nil return from the push is not enough evidence: a push with nothing to push also returns nil,
// and a detached push can leave a rejection unreported to its caller.
// The entry's detail names side, repoPath and the branch's configured remote.
// If either HasUnpushed sample or CurrentBranch errors, nothing is recorded and the error is not
// propagated: a failure to observe is not a failure to push.
func recordPushIfAdvanced(rec *Mutations, repo *gitrepo.Repo, side, repoPath string, hasUnpushedBefore bool, hasUnpushedErr error) {
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
	// The configured remote is only trace evidence.
	// When it cannot be read, the remote stays empty and the record is still written.
	remote := ""
	if out, err := gitexec.Run([]string{"config", "--get", "branch." + branch + ".remote"}, repoPath); err == nil {
		remote = strings.TrimSpace(out)
	}
	rec.AppendRef(KindBranchPushed, branch, refDetail(side, repoPath, remote))
}

// PushWeft pushes unpushed weft commits and records the push when it verifiably advanced the branch.
// It does nothing when opts.SkipGit or opts.SkipPush is set.
func (f *Fabric) PushWeft(opts SyncOptions) (res PushResult, err error) {
	rec := NewMutations(filepath.Dir(f.warpPath))
	defer func() { res.Mutations = rec.Snapshot() }()

	if opts.SkipGit || opts.SkipPush {
		return PushResult{}, nil
	}

	hadUnpushed, hadUnpushedErr := f.records.HasUnpushed()
	if err := f.records.PushCoalesced(); err != nil {
		return PushResult{}, err
	}
	recordPushIfAdvanced(rec, f.records, "records", f.weftPath, hadUnpushed, hadUnpushedErr)

	return PushResult{}, nil
}

// PullWeft fast-forwards the weft worktree from its upstream.
// It does nothing when opts.SkipGit is set.
func (f *Fabric) PullWeft(opts SyncOptions) error {
	if opts.SkipGit {
		return nil
	}
	return f.records.Pull()
}

// commitWeftAt stages everything at weftPath and commits it, with no warp branch or Warp-SHA trailer.
// Returns ("", false, nil) when opts.SkipGit is true.
// It does not acquire the weft write lock; the caller is responsible for synchronization.
// It seeds the weft artifact excludes before staging,
// so the stage-all never sweeps up machine-local scratch, even in a repo no other verb has seeded.
func commitWeftAt(weftPath, message string, opts SyncOptions) (sha string, committed bool, err error) {
	if opts.SkipGit {
		return "", false, nil
	}
	if err := seedWeftArtifactExcludes(weftPath); err != nil {
		return "", false, err
	}
	return gitrepo.New(weftPath).StageAllAndCommit(message)
}
