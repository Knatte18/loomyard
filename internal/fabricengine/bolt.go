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
// A path that no longer exists stages its deletion.
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
	return gitrepo.New(b.path).StageAndCommit(message, ScopedPathspec(".", writtenPaths))
}

// Push pushes any unpushed commits in the Bolt's repo, dropping seed commits that conflict at the push as PushRecorded does.
// A drop is logged at Info, since a caller without a mutation record has nowhere else to show it.
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

// PushRecorded pushes any unpushed commits in the Bolt's repo and records a seed-commit drop in rec.
// A push whose recovery rebase conflicted is retried once after the board's seed commits ahead of its upstream are dropped and its other commits replayed, under the board write lock and then the push lock.
// With no seed commits ahead, or when the drop refuses or its replay conflicts, the original push error is returned.
// A second failure is returned and never retried.
func (b *Bolt) PushRecorded(opts SyncOptions, rec *Mutations) error {
	err := pushWeftAt(b.path, opts)
	if !errors.Is(err, gitrepo.ErrPullRebaseFailed) {
		return err
	}
	if !b.dropSeedCommitsAfterConflict(rec) {
		return err
	}
	return pushWeftAt(b.path, opts)
}

// dropSeedCommitsAfterConflict drops the seed commits the board holds over its upstream and reports whether it did.
// The locks are taken in pullFirst's order, so the two cannot deadlock, and are released before the caller retries the push.
// A refused drop, a conflicting replay or a failed read leaves the board as it was and reports false.
func (b *Bolt) dropSeedCommitsAfterConflict(rec *Mutations) bool {
	writeLock, err := lock.AcquireWriteLock(filepath.Join(b.path, BoardWriteLockFile))
	if err != nil {
		return false
	}
	defer func() { _ = writeLock.Release() }()
	pushLock, err := lock.AcquireWriteLock(filepath.Join(b.path, gitrepo.PushLockFileName))
	if err != nil {
		return false
	}
	defer func() { _ = pushLock.Release() }()

	repo := gitrepo.New(b.path)
	upstream, err := repo.UpstreamSHA()
	if err != nil {
		return false
	}
	head, err := repo.CurrentSHA()
	if err != nil {
		return false
	}
	ahead, err := repo.CommitsNotIn(head, upstream)
	if err != nil {
		return false
	}
	seed, _, touched, err := seedCommitsAhead(repo, ahead)
	if err != nil || len(seed) == 0 {
		return false
	}
	return dropSeedCommits(rec, boardDropRequest(b.path, touched), repo, upstream, ahead) == nil
}

// Sync drives step to completion under an absorbing push lock, looping while step reports progress.
func (b *Bolt) Sync(step func() (progressed bool, err error)) error {
	return coalescePush(filepath.Join(b.path, "board.push.lock"), step)
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
// A path that no longer exists stages its deletion and is not recorded as a written file.
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
		if len(paths) == 0 {
			continue
		}
		sha, committed, err := repo.StageAndCommit(write.Message, ScopedPathspec(".", paths))
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
// Seed commits ahead of a moved upstream are dropped and the remaining commits replayed;
// a Bolt only behind is fast-forwarded.
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

	seed, _, touched, err := seedCommitsAhead(repo, ahead)
	if err != nil {
		return "", "", err
	}
	if len(seed) == 0 {
		return b.skip(BoltSkipDiverged, b.divergedDetail())
	}
	err = dropSeedCommits(rec, boardDropRequest(b.path, touched), repo, upstream, ahead)
	var refusal *destructiveRefusal
	switch {
	case err == nil:
		logger.Info("fabricengine: dropped seed commits from the board", "path", b.path, "commits", strings.Join(seed, " "))
		return "", "", nil
	case errors.As(err, &refusal) && refusal.Check == CheckDirtiness, errors.Is(err, errDropResetRefused):
		return b.skip(BoltSkipDirty, b.dirtyDetail(err))
	case errors.As(err, &refusal), errors.Is(err, errDropReplayFailed):
		return b.skip(BoltSkipDiverged, b.divergedDetail())
	default:
		return "", "", err
	}
}

// skip logs a pull-first skip at Warn and returns it.
func (b *Bolt) skip(skip BoltSkip, detail string) (BoltSkip, string, error) {
	logger.Warn("fabricengine: board not brought up to date, nothing written", "skip", string(skip), "detail", detail)
	return skip, detail, nil
}

// dirtyDetail words the way forward for a board an uncommitted change keeps from being brought up to date.
func (b *Bolt) dirtyDetail(cause error) string {
	return fmt.Sprintf("an uncommitted change in the board at %s blocks bringing it up to date (%v); run `lyx board sync` to commit it, then re-run the command", b.path, cause)
}

// divergedDetail words the way forward for a board whose own commits cannot be replayed onto its moved upstream.
func (b *Bolt) divergedDetail() string {
	return fmt.Sprintf("the board at %s holds commits its upstream lacks and the upstream has moved; run `git pull --rebase` in %s and `lyx board sync`, then re-run the command", b.path, b.path)
}

// seedCommitsAhead splits ahead, a list of commits, into the ones IsSeedCommit admits and the rest, each in ahead's order,
// and returns the sorted union of the paths every commit in ahead changes.
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
