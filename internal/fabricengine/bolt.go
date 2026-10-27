// bolt.go defines Bolt, the handle over the unpaired weft:main area (today consumed only by
// _board): it names that area as a self-contained unit through Commit/CommitWritten/Push/Sync,
// so a caller outside this package never has to spell "weft" to reach it.

package fabricengine

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lymcwd"
)

// Bolt is a handle over a single weft:main-backed repo path with no paired warp side — the
// exception to fabric's one-repo illusion.
// It never constructs any geometry token itself.
type Bolt struct {
	path string
}

// NewBolt returns a Bolt over the repo at repoPath.
func NewBolt(repoPath string) *Bolt {
	return &Bolt{path: repoPath}
}

// Commit stages and commits every change in the Bolt's repo under message, with no warp trailer and
// no correspondence recording.
func (b *Bolt) Commit(message string, opts SyncOptions) (sha string, committed bool, err error) {
	return commitWeftAt(b.path, message, opts)
}

// CommitWritten runs write under the board write lock and commits exactly the repo-relative paths write returns,
// so unrelated dirty files in the repo stay for the next sync.
// The lock is released only after the commit, so a concurrent whole-repo Commit never captures a half-written file.
//
// A write error is returned unwrapped and nothing is committed.
// An empty path list commits nothing and returns committed == false.
// A path that no longer exists stages its deletion, and one HEAD does not hold either is left out.
// With opts.SkipGit, write still runs under the lock but nothing is committed.
// It never pushes; callers push with Push afterwards, outside the write lock.
func (b *Bolt) CommitWritten(message string, write func() ([]string, error), opts SyncOptions) (sha string, committed bool, err error) {
	l, err := lock.AcquireWriteLock(filepath.Join(b.path, BoardWriteLockFile))
	if err != nil {
		return "", false, fmt.Errorf("fabricengine: acquire board write lock: %w", err)
	}
	defer func() { _ = l.Release() }()

	writtenPaths, err := write()
	if err != nil {
		return "", false, err
	}
	if opts.SkipGit {
		return "", false, nil
	}
	repo := gitrepo.New(b.path)
	stageable, err := stageablePaths(repo, b.path, writtenPaths)
	if err != nil {
		return "", false, err
	}
	return repo.StageAndCommit(message, ScopedPathspec(".", stageable))
}

// stageablePaths returns the paths, relative to the repo at root, that git can stage: each one on disk, and each missing one HEAD's tree holds, whose deletion it stages.
// git add refuses a path that matches nothing, so a missing path HEAD does not hold, such as a name a stale local manifest kept after a pull removed the file, is left out.
// An unborn HEAD holds no path.
// Paths are file paths: a path HEAD holds as a directory is not recognised as held.
func stageablePaths(repo *gitrepo.Repo, root string, paths []string) ([]string, error) {
	head, err := repo.CurrentSHA()
	if err != nil && !errors.Is(err, gitrepo.ErrNoCommits) {
		return nil, fmt.Errorf("fabricengine: resolve HEAD of %s: %w", root, err)
	}
	var stageable []string
	for _, path := range paths {
		if _, err := os.Lstat(filepath.Join(root, path)); !errors.Is(err, fs.ErrNotExist) {
			stageable = append(stageable, path)
			continue
		}
		if head == "" {
			continue
		}
		_, err := repo.FileAtRevision(head, filepath.ToSlash(path))
		switch {
		case err == nil:
			stageable = append(stageable, path)
		case errors.Is(err, gitrepo.ErrPathNotAtRevision):
		default:
			return nil, fmt.Errorf("fabricengine: read %s at %s: %w", path, head, err)
		}
	}
	return stageable, nil
}

// Push pushes any unpushed commits in the Bolt's repo, replaying them onto a moved origin as PushRecorded does.
// A seed-commit drop of that replay is logged at Info, since a caller without a mutation record has nowhere else to show it.
func (b *Bolt) Push(opts SyncOptions) error {
	rec := NewMutations(filepath.Dir(b.path))
	err := b.PushRecorded(opts, rec)
	for _, entry := range rec.Snapshot().Entries() {
		if entry.Kind == KindCommitsDropped {
			logger.Info("fabricengine: dropped seed commits from the board at push", "path", b.path, "commits", entry.Detail)
		}
	}
	return err
}

// PushRecorded pushes any unpushed commits in the Bolt's repo rebase-free and records what a replay of them does in rec.
// A push origin rejects is retried once after the board's local commits are replayed onto origin's, under the board write lock and then the push lock;
// the replay drops the seed commits, and an entry only one side changed never conflicts.
// A held board write lock or an uncommitted change on a path the replay touches defers the push to the next one and returns nil.
// A failed push is recorded as a failed push and returned as a *PushFailedError.
// A push honoring SkipGit or SkipPush does nothing.
func (b *Bolt) PushRecorded(opts SyncOptions, rec *Mutations) error {
	if opts.SkipGit || opts.SkipPush {
		return nil
	}
	retry := func() (bool, error) { return retryBoltPush(rec, b.path) }
	return pushClassified(rec, filepath.Dir(b.path), failedPushSideBolt, b.path, opts, retry)
}

// Sync drives step to completion under an absorbing push lock, looping while step reports progress.
func (b *Bolt) Sync(step func() (progressed bool, err error)) error {
	return coalescePush(filepath.Join(b.path, "board.push.lock"), step)
}

// HasUpstream reports whether the Bolt's current branch has an upstream configured, read from local config without contacting the remote.
func (b *Bolt) HasUpstream() (bool, error) {
	_, err := gitrepo.New(b.path).UpstreamSHA()
	if errors.Is(err, gitrepo.ErrNoUpstream) {
		return false, nil
	}
	return err == nil, err
}

// HasUnpushed reports whether the Bolt's current branch is ahead of its upstream as last fetched, read from local state without contacting the remote.
// A branch with no upstream counts as ahead.
func (b *Bolt) HasUnpushed() (bool, error) {
	return gitrepo.New(b.path).HasUnpushed()
}

// HasUncommitted reports whether the Bolt's worktree holds a file with an uncommitted change, read from local state.
func (b *Bolt) HasUncommitted() (bool, error) {
	changed, err := gitrepo.New(b.path).WorktreeChangedFiles()
	return len(changed) > 0, err
}

// BoltSkip names why a pull-first write did not run: Bolt could not be brought up to date.
// The empty value means Bolt is up to date and the writes ran.
type BoltSkip string

const (
	// BoltSkipFetchFailed means the fetch from the remote failed.
	BoltSkipFetchFailed BoltSkip = "fetch_failed"
	// BoltSkipDiverged means Bolt holds commits the moved upstream lacks and they could not be replayed onto it.
	BoltSkipDiverged BoltSkip = "diverged"
	// BoltSkipDirty means an uncommitted change in Bolt blocked bringing it up to date.
	BoltSkipDirty BoltSkip = "dirty"
)

// BoltWrite is one write PullThenCommitWritten runs: Write returns the board-relative paths it wrote or deleted, committed under Message.
type BoltWrite struct {
	Message string
	Write   func() ([]string, error)
}

// BoltWriteResult reports what PullThenCommitWritten did.
// SHAs lists the landed commits in order, Committed is true when any landed, and a non-empty Skipped means no write ran, with SkipDetail naming the way forward.
type BoltWriteResult struct {
	MutationRecord
	SHAs       []string
	Committed  bool
	Skipped    BoltSkip
	SkipDetail string
}

// PullThenCommitWritten brings Bolt up to date with its upstream, then runs each write in order and commits each one's paths as its own commit.
// It is CommitWritten's pull-first sibling for callers that must not write over a stale copy: when Bolt cannot be brought up to date, nothing is written and the result's Skipped says why.
// Bolt without an upstream writes as CommitWritten does.
//
// The board write lock is held across the pull and every write.
// A write or commit error stops the sequence and is returned with the commits already landed kept in the result.
// An empty path list commits nothing.
// A path that no longer exists stages its deletion and is not recorded as a written file, and one HEAD does not hold either is left out of the commit.
// It never pushes.
func (b *Bolt) PullThenCommitWritten(writes []BoltWrite, rec *Mutations) (res BoltWriteResult, err error) {
	l, err := lock.AcquireWriteLock(filepath.Join(b.path, BoardWriteLockFile))
	if err != nil {
		return BoltWriteResult{}, fmt.Errorf("fabricengine: acquire board write lock: %w", err)
	}
	defer func() { _ = l.Release() }()

	defer func() { res.Mutations = rec.Snapshot() }()

	skip, detail, err := b.pullFirst(rec)
	if err != nil {
		return BoltWriteResult{}, err
	}
	if skip != "" {
		return BoltWriteResult{Skipped: skip, SkipDetail: detail}, nil
	}

	repo := gitrepo.New(b.path)
	for _, write := range writes {
		paths, err := write.Write()
		if err != nil {
			return res, err
		}
		for _, path := range paths {
			abs := filepath.Join(b.path, path)
			if _, err := os.Lstat(abs); errors.Is(err, fs.ErrNotExist) {
				continue
			}
			rec.Append(KindFileWritten, abs, "")
		}
		stageable, err := stageablePaths(repo, b.path, paths)
		if err != nil {
			return res, err
		}
		if len(stageable) == 0 {
			continue
		}
		sha, committed, err := repo.StageAndCommit(write.Message, ScopedPathspec(".", stageable))
		if err != nil {
			return res, err
		}
		if committed {
			rec.Append(KindCommitCreated, b.path, sha)
			res.SHAs = append(res.SHAs, sha)
			res.Committed = true
		}
	}
	return res, nil
}

// pullFirst brings Bolt up to date with its upstream and reports why it could not, as a skip with a detail naming the way forward.
// The caller holds the board write lock.
// pullFirst also holds the push lock in the board dir for its whole run, taken after the board write lock.
// A coalesced push rebases the board under that lock and never takes the write lock, so without it a push could rebase the board mid-drop.
//
// A Bolt with no upstream is left alone.
// Commits ahead of a moved upstream are replayed onto it by entry, the commits of an entry origin has ended set aside and seed commits dropped;
// a Bolt only behind is fast-forwarded.
// A replay that stops on a conflict skips as diverged, and one an uncommitted change blocks skips as dirty.
// Each skip is logged at Warn here, once, so no caller repeats the text.
func (b *Bolt) pullFirst(rec *Mutations) (BoltSkip, string, error) {
	pushLock, err := lock.AcquireWriteLock(filepath.Join(b.path, gitrepo.PushLockFileName))
	if err != nil {
		return "", "", fmt.Errorf("fabricengine: acquire push lock: %w", err)
	}
	defer func() { _ = pushLock.Release() }()

	repo := gitrepo.New(b.path)
	if err := repo.Fetch(); err != nil {
		if _, upstreamErr := repo.UpstreamSHA(); errors.Is(upstreamErr, gitrepo.ErrNoUpstream) {
			return "", "", nil
		}
		return b.skip(BoltSkipFetchFailed, fmt.Sprintf("fetching %s for the board at %s failed (%v); re-run the command once the remote is reachable", originRemoteName, b.path, err))
	}
	upstream, err := repo.UpstreamSHA()
	if errors.Is(err, gitrepo.ErrNoUpstream) {
		return "", "", nil
	}
	if err != nil {
		return "", "", err
	}
	head, err := repo.CurrentSHA()
	if err != nil {
		return "", "", err
	}
	ahead, err := repo.CommitsNotIn(head, upstream)
	if err != nil {
		return "", "", err
	}
	behind, err := repo.CommitsNotIn(upstream, head)
	if err != nil {
		return "", "", err
	}
	if len(behind) == 0 {
		return "", "", nil
	}

	if len(ahead) == 0 {
		if err := repo.MergeFFOnly(upstream); err != nil {
			return b.skip(BoltSkipDirty, b.dirtyDetail(err))
		}
		rec.Append(KindRepoAdvanced, b.path, upstream)
		return "", "", nil
	}

	location, ended, err := b.endedByOrigin(repo, head, upstream, ahead)
	if err != nil {
		return "", "", err
	}
	outcome, err := replayBolt(rec, location, b.path, upstream, ended)
	var refusal *destructiveRefusal
	switch {
	case err == nil:
		b.logReplay(outcome)
		return "", "", nil
	case errors.Is(err, ErrReplayDirty):
		return b.skip(BoltSkipDirty, b.dirtyDetail(err))
	case errors.Is(err, ErrReplayConflict), errors.As(err, &refusal):
		return b.skip(BoltSkipDiverged, b.divergedDetail(repo, outcome.Conflicts, ahead))
	default:
		return "", "", err
	}
}

// endedByOrigin reads which of the entries the local commits ahead of upstream change origin has ended, and the hub location the replay reads this machine's landing state through.
// It returns no location and an empty set when none is ended, and when the hub's location cannot be resolved, since a doubt never sets aside this machine's own commits.
func (b *Bolt) endedByOrigin(repo *gitrepo.Repo, head, upstream string, ahead []string) (*lymcwd.Location, EndedSet, error) {
	slugs, created, err := localBoardEntries(repo, head, ahead)
	if err != nil || len(slugs) == 0 {
		return nil, nil, err
	}
	ended, err := readEndedSet(b.path, upstream, slugs, created)
	if err != nil || len(ended) == 0 {
		return nil, nil, err
	}
	location, err := hubLocationOfBoard(b.path)
	if err != nil {
		logger.Warn("fabricengine: the hub of the board could not be resolved, so no entry origin ended is set aside", "path", b.path, "error", err)
		return nil, nil, nil
	}
	return location, ended, nil
}

// logReplay logs what a replay dropped, set aside or lost, naming each set-aside tag, so no caller repeats the text.
func (b *Bolt) logReplay(outcome ReplayOutcome) {
	if len(outcome.Dropped) > 0 {
		logger.Info("fabricengine: dropped commits from the board", "path", b.path, "commits", strings.Join(outcome.Dropped, " "))
	}
	for _, aside := range outcome.SetAside {
		logger.Warn("fabricengine: a local board commit was set aside under a tag", "path", b.path, "commit", aside.Commit, "tag", aside.Tag)
	}
	for _, lost := range outcome.Lost {
		logger.Warn("fabricengine: a local board value lost to origin's", "path", b.path, "entry", lost.Entry, "field", lost.Field, "value", lost.Local, "reason", lost.Reason)
	}
	for _, refused := range outcome.Refused {
		logger.Warn("fabricengine: a local board change was refused", "path", b.path, "entry", refused.Entry, "field", refused.Field, "reason", refused.Reason)
	}
}

// skip logs a pull-first skip at Warn and returns it.
func (b *Bolt) skip(skip BoltSkip, detail string) (BoltSkip, string, error) {
	logger.Warn("fabricengine: board not brought up to date, nothing written", "skip", string(skip), "detail", detail)
	return skip, detail, nil
}

// dirtyDetail words the way forward for a board an uncommitted change keeps from being brought up to date.
func (b *Bolt) dirtyDetail(cause error) string {
	return fmt.Sprintf("an uncommitted change in the board at %s blocks bringing it up to date (%v); run `lym board sync` to commit it, then re-run the command", b.path, cause)
}

// divergedDetail words the way forward for a board whose own commits cannot be replayed onto its moved upstream:
// the conflicting paths, and the two ways to settle them, each followed by `lym board sync`.
func (b *Bolt) divergedDetail(repo *gitrepo.Repo, conflicts, localCommits []string) string {
	branch, err := repo.CurrentBranch()
	if err != nil {
		branch = "<branch>"
	}
	return fmt.Sprintf("the board at %s holds commits its upstream lacks and could not replay them onto the moved upstream: %s; re-run the command afterwards",
		b.path, recordsConflictWayForward(b.path, branch, conflicts, localCommits, pushStep(FailedPush{Repo: failedPushSideBolt})))
}

// seedCommitsAhead splits ahead, a list of commits, into the ones IsSeedCommit admits and the rest, each in ahead's order, and returns the sorted union of the paths every commit in ahead changes.
func seedCommitsAhead(repo *gitrepo.Repo, ahead []string) (seed, other, touched []string, err error) {
	for _, sha := range ahead {
		detail, err := repo.CommitDetail(sha)
		if err != nil {
			return nil, nil, nil, err
		}
		if IsSeedCommit(detail.Message, detail.Paths) {
			seed = append(seed, sha)
		} else {
			other = append(other, sha)
		}
		touched = append(touched, detail.Paths...)
	}
	slices.Sort(touched)
	return seed, other, slices.Compact(touched), nil
}
