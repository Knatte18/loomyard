// pushanchored.go holds PushPairAnchored, the synchronous push that takes the weft-side push lock.
// It lets a caller outside the Fabric Vocabulary Invariant's owner set push without naming a weft path.
// In the hub's prime it pushes only the records side and reports the code branch as PushResult.CodePushSkipped.

package fabricengine

import (
	"errors"
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"time"

	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/pushledger"
)

// ErrPushLockBusy is wrapped by the error a bounded push returns when the push lock is still held once its wait ran out.
// Nothing was pushed.
// The error names the lock's holder, its pid and how long it has held the lock, when the holder recorded them.
var ErrPushLockBusy = errors.New("fabricengine: push lock busy")

// ErrEntryEnded is wrapped by the error of a records push that was skipped.
// The push was skipped because its branch is gone from origin and its entry is ended there.
// Such a branch is left to its teardown; the push is no failure and no owed push.
var ErrEntryEnded error = errors.New("fabricengine: the entry of the branch is ended on origin")

// entryEndedError is an error with its own text that satisfies errors.Is(err, ErrEntryEnded).
type entryEndedError struct {
	text string
}

// Error returns the error's own text.
func (e entryEndedError) Error() string { return e.text }

// Is reports whether target is ErrEntryEnded.
func (e entryEndedError) Is(target error) bool { return target == ErrEntryEnded }

// isOwedClass reports whether class is one a caller retries until the push lands: transport, deferred or teardown_lock.
func isOwedClass(class fabriclayout.PushFailureClass) bool {
	switch class {
	case fabriclayout.PushFailureTransport, fabriclayout.PushFailureDeferred, fabriclayout.PushFailureTeardownLock:
		return true
	}
	return false
}

// IsPushRejected reports whether err carries a remote's non-fast-forward rejection of a push.
func IsPushRejected(err error) bool {
	return errors.Is(err, gitrepo.ErrPushRejected)
}

// IsPushOwed reports whether err is a push that is owed, one a caller retries until it lands.
// A push is owed when the push lock stayed busy past the wait.
// It is also owed when every recorded failure it carries is of class transport, deferred or teardown_lock.
// A takeover read of the prime's records push that failed is owed too, and its error wraps ErrTakeoverPassOwed.
// A bare ErrPairPushRunning is not owed: it comes only from a push with no teardown wait, which records no failure.
// A rejection, a takeover, a *TakeoversError and a hand-over are not owed.
func IsPushOwed(err error) bool {
	if errors.Is(err, ErrPushLockBusy) || errors.Is(err, ErrTakeoverPassOwed) {
		return true
	}
	failures := pushFailuresOf(err)
	return len(failures) > 0 && !slices.ContainsFunc(failures, func(failed *PushFailedError) bool {
		return !isOwedClass(failed.Record.Class)
	})
}

// pushFailuresOf returns every *PushFailedError err carries, reading each member of a joined error.
func pushFailuresOf(err error) []*PushFailedError {
	if err == nil {
		return nil
	}
	if failed, ok := err.(*PushFailedError); ok {
		return []*PushFailedError{failed}
	}
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		var failures []*PushFailedError
		for _, inner := range joined.Unwrap() {
			failures = append(failures, pushFailuresOf(inner)...)
		}
		return failures
	}
	return pushFailuresOf(errors.Unwrap(err))
}

// recordsPushCeiling returns the records commit of the run slug that no push may yet carry.
// The push ledger answers first; without a hold there, the run's held-records ref and then its landing commit do,
// so a hold outlives a lost ledger.
// held is false when there is none.
// An empty slug, the prime's, holds none: its records are shared by every batten run.
func recordsPushCeiling(hubPath, slug string) (hold string, held bool, err error) {
	if slug == "" {
		return "", false, nil
	}
	hold, held, err = pushledger.HeldRecordsCommit(hubPath, slug)
	if err != nil {
		return "", false, fmt.Errorf("fabricengine: read the held records of %q: %w", slug, err)
	}
	if held {
		return hold, true, nil
	}
	return heldByRefOrLanding(hubPath, slug)
}

// recordsCeilingAt returns the newest commit of the records worktree at weftPath that a push of slug's pair may send.
// It returns "" when nothing is held.
func recordsCeilingAt(hubPath, weftPath, slug string) (string, error) {
	hold, held, err := recordsPushCeiling(hubPath, slug)
	if err != nil || !held {
		return "", err
	}
	return heldCeiling(weftPath, hold)
}

// unpushedUnder reports whether a push of the worktree at path that may send nothing past ceiling still has work.
// With no ceiling that is any commit past the upstream.
// With one, only a tracking ref holding the ceiling reads done;
// a branch with no upstream or a pruned tracking ref is left to the push to judge.
func unpushedUnder(path, ceiling string) (bool, error) {
	if ceiling == "" {
		return gitrepo.New(path).HasUnpushed()
	}
	branch, err := readLocalBranch(path)
	if err != nil || branch.state != upstreamTracked {
		return true, err
	}
	return branch.lacks(ceiling)
}

// acquirePushLock returns a function that takes the push lock at the path it is given.
// LockWaitUnbounded blocks until the lock is free.
// A positive lockWait gives up once it expires, reporting the lock as not acquired.
func acquirePushLock(lockWait time.Duration) func(lockPath string) (*lock.FileLock, bool, error) {
	return func(lockPath string) (*lock.FileLock, bool, error) {
		if lockWait == fabriclayout.LockWaitUnbounded {
			held, err := lock.AcquireWriteLock(lockPath)
			if err != nil {
				return nil, false, fmt.Errorf("fabricengine: acquire push lock: %w", err)
			}
			return held, true, nil
		}

		held, acquired, err := lock.AcquireWriteLockWithin(lockPath, lockWait)
		if err != nil {
			return nil, false, fmt.Errorf("fabricengine: acquire push lock: %w", err)
		}
		return held, acquired, nil
	}
}

// HoldPairPushLock takes the push lock of the pair at pair and returns its release.
// It waits without bound while another push holds the lock.
// No push of the pair runs until release is called, so a caller can commit records and hold them back first.
func HoldPairPushLock(pair *lymcwd.Location) (release func(), err error) {
	lockDir, err := ensureWeftLockDirAt(RecordsWorktree(pair))
	if err != nil {
		return nil, err
	}
	leaveGate := pauserecord.Admit(pushLockGateOp)
	held, _, err := acquirePushLock(fabriclayout.LockWaitUnbounded)(filepath.Join(lockDir, weftPushLockFile))
	if err != nil {
		leaveGate()
		return nil, err
	}
	releaseHeld := holdPushLock(held, lockDir, time.Now())
	return func() {
		if err := releaseHeld(); err != nil {
			logger.Warn("fabricengine: push lock not released", "path", lockDir, "error", err)
		}
		leaveGate()
	}, nil
}

// pushLockBusyError returns the error for a synchronous push whose wait for the push lock under lockDir ran out.
// It names the lock's holder as its holder record gives it.
func pushLockBusyError(lockDir string, lockWait time.Duration) error {
	holder := describePushLockHolder(lockDir, time.Now(), proc.IsAlive)
	return fmt.Errorf("%w: %s still held after %s, %s; neither side was pushed", ErrPushLockBusy, filepath.Join(lockDir, weftPushLockFile), lockWait, holder)
}

// PushPairAnchored pushes the unpushed commits of l's code worktree and its paired records sibling.
// It pushes rebase-free, under the weft-side absorbing push lock.
// When l is the hub's prime it pushes the records side only, since its code branch is the operator's parent branch.
// A code branch with unpushed commits is reported in PushResult.CodePushSkipped:
// the note names the branch and says it was left for the operator to push.
// Otherwise the records side is pushed first, and the code side only once the records side landed.
// The records side landed when its push returned nil and its worktree then holds nothing unpushed.
// A records side with unpushed commits is pushed again over the push backoff while origin holds its branch.
// Commits made during the push, or a tracking ref the push left unmoved, keep it from reading as landed.
// A side still unpushed once that backoff is spent records a failed push of class PushFailureDeferred,
// and a failed lookup of the branch on origin that cannot reach origin records one of class PushFailureTransport;
// each returns its *PushFailedError, which marks the push as owed.
// Only a records branch gone from origin reads as one whose entry ended.
// A records side that fails, is rejected, is deferred, or was left to its entry's teardown sends no code side:
// the error is the records side's, joined with a line naming the withheld code worktree,
// and the code commits go with the next pair push.
// A deferral returns the *PushFailedError of the deferred record it wrote.
// A side whose HEAD is unborn is skipped.
// Each side retries a rejection over the push backoff, after a fetch, while the remote tip is contained in local HEAD.
// A failing side's error names the side, code or records, in `fabricengine: push <side> side at <path>: %w`.
// errors.Is(err, gitrepo.ErrPushRejected) holds for a rejected side;
// push.LockWait bounds only the wait for the push lock:
// a positive value gives up with an error wrapping ErrPushLockBusy and neither side pushed;
// LockWaitUnbounded blocks.
// push.RunSlug names the run whose records a rejected prime push reads.
// A pair's failed-push records name the pair's own run.
// A rejected records push reads that run's record on origin before it replays anything.
// When the record holds another machine's takeover entry that push.Here does not hold, the push pushes neither side.
// It returns a *TakeoverError, or an error wrapping ErrHandedOver after this machine handed the run over.
// Each side it pushes is marked pending before the lock is taken and re-read after it is released,
// so a code commit made while another holder pushes is pushed by whichever holder releases last.
// It passes the process's pause gate before it takes any lock and stays admitted until it returns,
// so a pause never holds it while it holds the pair's teardown lock or the push lock.
// For a pair, not the prime, it first takes the pair's teardown lock shared and holds it through the push.
// While any holder has that lock exclusive, it pushes nothing.
// push.TeardownWait zero makes one try, and a positive value polls the lock for at most that long.
// It returns an empty PushResult and an error wrapping ErrPairPushRunning that names the pair;
// the commits stay local for the pair's next push.
// A positive wait that runs out also records a failed push of class PushFailureTeardownLock for the records side,
// and the error is that record's *PushFailedError, which marks the push as owed.
// With push.OwedUnrecorded an owed failure is returned as its *PushFailedError and written to no record, with no notice:
// a deferred side is told from the returned error, and the caller retries the push itself.
// Every other failure is recorded as without it.
// With push.SingleAttempt a side that cannot reach origin fails after one transport attempt.
// While a records commit of a pair's run is held, the records side is pushed only up to that commit's first parent.
// The records side then reads as landed once origin holds that parent.
// A non-empty push.CodeCeiling likewise caps the code side at that commit.
// A capped side whose origin already holds its ceiling is not pushed, and a rejected one is never replayed or reset.
// The prime's records push takes no teardown lock.
// The code side is sent with SyncOptions.AfterRecords set,
// so a rejection by an origin tip that no transition of the run names resets that tip.
// It does not reset past a commit this machine did not make: the push is then refused, naming that commit.
//
// Every side that observably advanced is recorded in the returned PushResult.
// Returns (PushResult{}, nil) immediately, with no lock taken, when opts.SkipGit or opts.SkipPush is true.
// A failure to resolve whether l is the prime returns that error before the lock is taken and nothing is pushed.
func PushPairAnchored(l *lymcwd.Location, opts SyncOptions, push fabriclayout.PairPush) (res fabriclayout.PushResult, err error) {
	lockWait := push.LockWait
	rec := NewMutations(l.HubPath)
	defer func() { res.Mutations = rec.Snapshot() }()

	if opts.SkipGit || opts.SkipPush {
		return fabriclayout.PushResult{}, nil
	}
	leaveGate := pauserecord.Admit(pushLockGateOp)
	defer leaveGate()

	isPrime, err := IsPrimeWorktree(l)
	if err != nil {
		return fabriclayout.PushResult{}, err
	}
	warpPath := l.WorktreePath()
	weftPath := RecordsWorktree(l)

	// A pair's push is held and teardown-guarded under its own slug; the prime's records push is neither.
	slug := ""
	if !isPrime {
		slug = l.WorktreeName
	}
	opts.RunSlug = push.RunSlug
	opts.OwedUnrecorded = push.OwedUnrecorded
	opts.SingleAttempt = push.SingleAttempt
	if slug != "" {
		opts.RunSlug = slug
		releaseTeardown, acquired, err := acquireTeardownShared(l.HubPath, slug, push.TeardownWait)
		if err != nil {
			return fabriclayout.PushResult{}, err
		}
		if !acquired {
			return fabriclayout.PushResult{}, teardownLockOwedError(l.HubPath, weftPath, slug, push.TeardownWait, opts)
		}
		defer releaseTeardown()
	}
	lockDir, err := ensureWeftLockDirAt(weftPath)
	if err != nil {
		return fabriclayout.PushResult{}, err
	}
	owed := map[string]fabriclayout.PushSpawn{pushSideRecords: {WeftPath: weftPath, PauseSlug: slug, TeardownSlug: slug}}
	if !isPrime {
		owed[pushSideCode] = fabriclayout.PushSpawn{WarpPath: warpPath, PauseSlug: slug, TeardownSlug: slug}
	}

	var codePushSkipped string
	pushSide := func(side, path string) error {
		head, err := headOrEmpty(path)
		if err != nil {
			return fmt.Errorf("fabricengine: push %s side at %s: %w", side, path, err)
		}
		if head == "" {
			return nil
		}
		sideOpts := opts
		sideOpts.AfterRecords = side == pushSideCode
		if side == pushSideRecords {
			if sideOpts.ceiling, err = recordsCeilingAt(l.HubPath, path, slug); err != nil {
				return fmt.Errorf("fabricengine: push %s side at %s: %w", side, path, err)
			}
		} else {
			sideOpts.ceiling = push.CodeCeiling
			// A parent branch's refusal names the landings it carries, whoever sent the push.
			landings, err := LandingCommits(l)
			if err != nil {
				return fmt.Errorf("fabricengine: push %s side at %s: %w", side, path, err)
			}
			sideOpts.landings = slices.Sorted(maps.Values(landings))
		}
		if sideOpts.ceiling != "" {
			unpushed, err := unpushedUnder(path, sideOpts.ceiling)
			if err != nil {
				return fmt.Errorf("fabricengine: push %s side at %s: %w", side, path, err)
			}
			if !unpushed {
				return nil
			}
		}
		var retryRecords func() (bool, error)
		if side == pushSideRecords {
			retryRecords = func() (bool, error) {
				return retryWeftPush(rec, l.HubPath, path, opts.RunSlug, push.Here, sideOpts.ceiling)
			}
		}
		if err := pushClassified(rec, l.HubPath, side, path, sideOpts, retryRecords); err != nil {
			return fmt.Errorf("fabricengine: push %s side at %s: %w", side, path, err)
		}
		return nil
	}

	pushRecordsSide := func() error {
		if head, err := headOrEmpty(weftPath); err == nil && head == "" {
			return nil
		}
		delays := gitrepo.PushBackoffDelays()
		sleep := opts.BackoffSleep
		if sleep == nil {
			sleep = time.Sleep
		}
		for attempt := 0; ; attempt++ {
			started := time.Now()
			if err := pushSide(pushSideRecords, weftPath); err != nil {
				return err
			}
			ceiling, err := recordsCeilingAt(l.HubPath, weftPath, slug)
			if err != nil {
				return fmt.Errorf("fabricengine: push records side at %s: %w", weftPath, err)
			}
			unpushed, err := unpushedUnder(weftPath, ceiling)
			if err != nil {
				return fmt.Errorf("fabricengine: push records side at %s: %w", weftPath, err)
			}
			if !unpushed {
				return nil
			}
			notLanded := recordsSideNotLanded(l.HubPath, weftPath, started, opts)
			if !errors.Is(notLanded, errRecordsUnpushedPastPush) {
				return fmt.Errorf("fabricengine: push records side at %s: %w", weftPath, notLanded)
			}
			if attempt == len(delays) {
				owed := recordOwedRecordsPush(l.HubPath, weftPath, fabriclayout.PushFailureDeferred, notLanded, opts)
				return fmt.Errorf("fabricengine: push records side at %s: %w", weftPath, owed)
			}
			logger.Warn("fabricengine: records side still unpushed after its push, pushed again", "path", weftPath, "attempt", attempt+1, "error", notLanded)
			sleep(delays[attempt])
		}
	}

	step := func() (progressed bool, err error) {
		codePushSkipped = ""
		if isPrime {
			codePushSkipped = unpushedCodeBranchNote(warpPath)
			return false, pushRecordsSide()
		}
		if err := pushRecordsSide(); err != nil {
			return false, fmt.Errorf("the code side at %s was withheld until the records side lands, and goes with the next pair push: %w", warpPath, err)
		}
		return false, pushSide(pushSideCode, warpPath)
	}

	acquired, err := coalesceFlagged(lockDir, owed, acquirePushLock(lockWait), step)
	if err == nil && !acquired {
		return fabriclayout.PushResult{}, pushLockBusyError(lockDir, lockWait)
	}
	return fabriclayout.PushResult{CodePushSkipped: codePushSkipped}, err
}

// teardownLockOwedError is the error of a pair push that found slug's teardown lock held.
// A zero wait returns the plain refusal wrapping ErrPairPushRunning, and records nothing.
// A positive wait that ran out records a failed push of class PushFailureTeardownLock, unless opts.OwedUnrecorded is set.
// The record is against the records branch at weftPath.
// The error returned is a *PushFailedError wrapping ErrPairPushRunning, which reads as an owed push.
func teardownLockOwedError(hubPath, weftPath, slug string, wait time.Duration, opts SyncOptions) error {
	refusal := fmt.Errorf("the pair %q is being made, discarded or removed, so nothing was pushed and its commits stay local for the pair's next push: %w", slug, ErrPairPushRunning)
	if wait <= 0 {
		return refusal
	}
	branch, err := gitrepo.New(weftPath).CurrentBranch()
	if err != nil {
		return refusal
	}
	refusal = fmt.Errorf("the teardown lock of the pair %q was still held after %s, so nothing was pushed and its commits stay local for the pair's next push: %w", slug, wait, ErrPairPushRunning)
	record := fabriclayout.FailedPush{
		Repo:         fabriclayout.RepoRecords,
		RepoPath:     weftPath,
		CodeWorktree: pairedCodeWorktree(hubPath, weftPath),
		Branch:       branch,
		Class:        fabriclayout.PushFailureTeardownLock,
		Error:        refusal.Error(),
		Runs:         []string{slug},
	}
	if !opts.OwedUnrecorded {
		if err := recordPushFailure(hubPath, record, time.Now(), opts.notifier()); err != nil {
			logger.Warn("fabricengine: failed-push record not written", "side", fabriclayout.RepoRecords, "branch", branch, "error", err)
		}
	}
	return &PushFailedError{Record: record, Err: refusal}
}

// errRecordsUnpushedPastPush is wrapped by the not-landed error of a records branch origin still holds.
// The worktree gained commits after its push read HEAD, or the push left its tracking ref unmoved,
// so another push of the branch carries them.
var errRecordsUnpushedPastPush = errors.New("fabricengine: the records branch holds commits its push did not carry")

// recordsSideNotLanded is the error of a records push that returned nil.
// The worktree at weftPath still holds unpushed commits.
// A push deferred since started returns its *PushFailedError, read from the deferred record the push wrote.
// A branch origin still holds returns an error wrapping errRecordsUnpushedPastPush.
// A branch gone from origin had its entry ended there, since the push skips only such a branch,
// and its error wraps ErrEntryEnded and says the branch is left to its teardown.
// A lookup of the branch on origin that fails to reach origin records a failed push of class PushFailureTransport,
// and the error is that record's *PushFailedError, which marks the push as owed.
// Any other failed lookup returns its error unclassified.
// Outside opts.OwedUnrecorded a deferral is read back from the deferred record the push wrote;
// with it the deferral came back as the push's own error, so no record is read.
func recordsSideNotLanded(hubPath, weftPath string, started time.Time, opts SyncOptions) error {
	branch, err := gitrepo.New(weftPath).CurrentBranch()
	if err != nil {
		return err
	}
	if !opts.OwedUnrecorded {
		record, found, err := readFailedPush(hubPath, fabriclayout.RepoRecords, branch)
		if err != nil {
			return err
		}
		if found && record.Class == fabriclayout.PushFailureDeferred && !record.LastSeen.Before(started) {
			return &PushFailedError{Record: record, Err: errors.New(record.Error)}
		}
	}
	gone, err := branchGoneOnOrigin(weftPath)
	if err != nil {
		lookupErr := fmt.Errorf("look up branch %s on origin after its push: %w", branch, err)
		if gitexec.IsTransportFailure(err) {
			return recordOwedRecordsPush(hubPath, weftPath, fabriclayout.PushFailureTransport, lookupErr, opts)
		}
		return lookupErr
	}
	if !gone {
		return fmt.Errorf("%w: branch %s is still on origin", errRecordsUnpushedPastPush, branch)
	}
	return entryEndedError{text: fmt.Sprintf("the entry of branch %s is ended on origin, so the branch was not pushed and is left to its teardown", branch)}
}

// recordOwedRecordsPush records a failed push of class for the records branch at weftPath.
// It returns the record as a *PushFailedError wrapping cause.
// The classes it is given read as owed pushes, so the caller retries the push.
// With opts.OwedUnrecorded it writes no record and posts no notice.
// A records branch that cannot be read returns cause with nothing recorded.
func recordOwedRecordsPush(hubPath, weftPath string, class fabriclayout.PushFailureClass, cause error, opts SyncOptions) error {
	branch, err := gitrepo.New(weftPath).CurrentBranch()
	if err != nil {
		return cause
	}
	record := fabriclayout.FailedPush{
		Repo:         fabriclayout.RepoRecords,
		RepoPath:     weftPath,
		CodeWorktree: pairedCodeWorktree(hubPath, weftPath),
		Branch:       branch,
		Class:        class,
		Error:        cause.Error(),
	}
	if opts.RunSlug != "" {
		record.Runs = []string{opts.RunSlug}
	}
	if !opts.OwedUnrecorded {
		if err := recordPushFailure(hubPath, record, time.Now(), opts.notifier()); err != nil {
			logger.Warn("fabricengine: failed-push record not written", "side", fabriclayout.RepoRecords, "branch", branch, "error", err)
		}
	}
	return &PushFailedError{Record: record, Err: cause}
}

// unpushedCodeBranchNote returns the CodePushSkipped text for the code worktree at path.
// It returns "" when the branch has nothing unpushed or its state cannot be read.
func unpushedCodeBranchNote(path string) string {
	head, err := headOrEmpty(path)
	if err != nil || head == "" {
		return ""
	}
	repo := gitrepo.New(path)
	unpushed, err := repo.HasUnpushed()
	if err != nil || !unpushed {
		return ""
	}
	branch, err := repo.CurrentBranch()
	if err != nil {
		return ""
	}
	return fmt.Sprintf("branch %s has unpushed commits; the prime's code side is left for the operator to push", branch)
}
