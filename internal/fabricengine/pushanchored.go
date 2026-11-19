// pushanchored.go holds PushPairAnchored, the synchronous push that takes the weft-side push lock.
// It lets a caller outside the Fabric Vocabulary Invariant's owner set push without naming a weft path.
// In the hub's prime it pushes only the records side and reports the code branch as PushResult.CodePushSkipped.

package fabricengine

import (
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"time"

	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/proc"
)

// StatusPushLockWait is the bounded push-lock wait that `commit-records` and a run's transition push pass.
// It is also the default for a pair's teardown lock.
// It is long enough to ride out a detached push child, and short enough that a stuck holder never stalls a run.
const StatusPushLockWait = 30 * time.Second

// LockWaitUnbounded is the lockWait value that blocks until the push lock is free.
const LockWaitUnbounded time.Duration = 0

// ErrPushLockBusy is wrapped by the error a bounded push returns when the push lock is still held once its wait ran out.
// Nothing was pushed.
// The error names the lock's holder, its pid and how long it has held the lock, when the holder recorded them.
var ErrPushLockBusy = errors.New("fabricengine: push lock busy")

// IsPushRejected reports whether err carries a remote's non-fast-forward rejection of a push.
func IsPushRejected(err error) bool {
	return errors.Is(err, gitrepo.ErrPushRejected)
}

// IsPushOwed reports whether err is a push that is owed, one a caller retries until it lands.
// A push is owed when the push lock stayed busy past the wait.
// It is also owed when every recorded failure it carries is of class transport, deferred or teardown_lock.
// A bare ErrPairPushRunning is not owed: it comes only from a push with no teardown wait, which records no failure.
// A rejection, a takeover and a hand-over are not owed.
func IsPushOwed(err error) bool {
	if errors.Is(err, ErrPushLockBusy) {
		return true
	}
	failures := pushFailuresOf(err)
	return len(failures) > 0 && !slices.ContainsFunc(failures, func(failed *PushFailedError) bool {
		switch failed.Record.Class {
		case PushFailureTransport, PushFailureDeferred, PushFailureTeardownLock:
			return false
		}
		return true
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

// acquirePushLock returns a function that takes the push lock at the path it is given.
// LockWaitUnbounded blocks until the lock is free.
// A positive lockWait gives up once it expires, reporting the lock as not acquired.
func acquirePushLock(lockWait time.Duration) func(lockPath string) (*lock.FileLock, bool, error) {
	return func(lockPath string) (*lock.FileLock, bool, error) {
		if lockWait == LockWaitUnbounded {
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
// The prime's records push takes no teardown lock.
// The code side is sent with SyncOptions.AfterRecords set,
// so a rejection by an origin tip that no transition of the run names resets that tip.
//
// Every side that observably advanced is recorded in the returned PushResult.
// Returns (PushResult{}, nil) immediately, with no lock taken, when opts.SkipGit or opts.SkipPush is true.
// A failure to resolve whether l is the prime returns that error before the lock is taken and nothing is pushed.
func PushPairAnchored(l *lymcwd.Location, opts SyncOptions, push PairPush) (res PushResult, err error) {
	lockWait := push.LockWait
	rec := NewMutations(l.HubPath)
	defer func() { res.Mutations = rec.Snapshot() }()

	if opts.SkipGit || opts.SkipPush {
		return PushResult{}, nil
	}
	leaveGate := pauserecord.Admit(pushLockGateOp)
	defer leaveGate()

	isPrime, err := IsPrimeWorktree(l)
	if err != nil {
		return PushResult{}, err
	}
	warpPath := l.WorktreePath()
	weftPath := RecordsWorktree(l)

	request, err := statusPushRequest(l, "")
	if err != nil {
		return PushResult{}, err
	}
	opts.RunSlug = push.RunSlug
	if request.TeardownSlug != "" {
		opts.RunSlug = request.TeardownSlug
		releaseTeardown, acquired, err := acquireTeardownShared(l.HubPath, request.TeardownSlug, push.TeardownWait)
		if err != nil {
			return PushResult{}, err
		}
		if !acquired {
			return PushResult{}, teardownLockOwedError(l.HubPath, weftPath, request.TeardownSlug, push.TeardownWait, opts)
		}
		defer releaseTeardown()
	}
	lockDir, err := ensureWeftLockDirAt(weftPath)
	if err != nil {
		return PushResult{}, err
	}
	owed := map[string]PushSpawn{pushSideRecords: {WeftPath: weftPath, PauseSlug: request.PauseSlug, TeardownSlug: request.TeardownSlug}}
	if !isPrime {
		owed[pushSideCode] = PushSpawn{WarpPath: warpPath, PauseSlug: request.PauseSlug, TeardownSlug: request.TeardownSlug}
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
		var retryRecords func() (bool, error)
		if side == pushSideRecords {
			retryRecords = func() (bool, error) { return retryWeftPush(rec, l.HubPath, path, opts.RunSlug, push.Here) }
		}
		sideOpts := opts
		sideOpts.AfterRecords = side == pushSideCode
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
			unpushed, err := gitrepo.New(weftPath).HasUnpushed()
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
				owed := recordOwedRecordsPush(l.HubPath, weftPath, PushFailureDeferred, notLanded, opts)
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
		return PushResult{}, pushLockBusyError(lockDir, lockWait)
	}
	return PushResult{CodePushSkipped: codePushSkipped}, err
}

// MachineNames carries the names that read as this machine.
// Current is the machine's present name and Former the names it carried before.
// The caller fills it, since fabric reads no board.
// The zero value turns the takeover read off.
type MachineNames struct {
	Current string
	Former  []string
}

// PairPush carries what a pair push takes beyond the sync options.
// LockWait bounds the wait for the push lock, and LockWaitUnbounded blocks.
// TeardownWait bounds the wait for the pair's teardown lock: zero makes one try, and a longer wait owes the push.
// RunSlug names the run whose records a rejected prime push reads.
// Here names this machine, for a takeover read; its zero value turns that read off.
type PairPush struct {
	LockWait     time.Duration
	TeardownWait time.Duration
	RunSlug      string
	Here         MachineNames
}

// teardownLockOwedError is the error of a pair push that found slug's teardown lock held.
// A zero wait returns the plain refusal wrapping ErrPairPushRunning, and records nothing.
// A positive wait that ran out records a failed push of class PushFailureTeardownLock.
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
	record := FailedPush{
		Repo:         failedPushSideRecords,
		RepoPath:     weftPath,
		CodeWorktree: pairedCodeWorktree(hubPath, weftPath),
		Branch:       branch,
		Class:        PushFailureTeardownLock,
		Error:        refusal.Error(),
		Runs:         []string{slug},
	}
	if err := recordPushFailure(hubPath, record, time.Now(), opts.notifier()); err != nil {
		logger.Warn("fabricengine: failed-push record not written", "side", failedPushSideRecords, "branch", branch, "error", err)
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
// and its error says the branch is left to its teardown.
// A lookup of the branch on origin that fails to reach origin records a failed push of class PushFailureTransport,
// and the error is that record's *PushFailedError, which marks the push as owed.
// Any other failed lookup returns its error unclassified.
func recordsSideNotLanded(hubPath, weftPath string, started time.Time, opts SyncOptions) error {
	branch, err := gitrepo.New(weftPath).CurrentBranch()
	if err != nil {
		return err
	}
	record, found, err := readFailedPush(hubPath, failedPushSideRecords, branch)
	if err != nil {
		return err
	}
	if found && record.Class == PushFailureDeferred && !record.LastSeen.Before(started) {
		return &PushFailedError{Record: record, Err: errors.New(record.Error)}
	}
	gone, err := branchGoneOnOrigin(weftPath)
	if err != nil {
		lookupErr := fmt.Errorf("look up branch %s on origin after its push: %w", branch, err)
		if gitexec.IsTransportFailure(err) {
			return recordOwedRecordsPush(hubPath, weftPath, PushFailureTransport, lookupErr, opts)
		}
		return lookupErr
	}
	if !gone {
		return fmt.Errorf("%w: branch %s is still on origin", errRecordsUnpushedPastPush, branch)
	}
	return fmt.Errorf("the entry of branch %s is ended on origin, so the branch was not pushed and is left to its teardown", branch)
}

// recordOwedRecordsPush records a failed push of class for the records branch at weftPath.
// It returns the record as a *PushFailedError wrapping cause.
// The classes it is given read as owed pushes, so the caller retries the push.
// A records branch that cannot be read returns cause with nothing recorded.
func recordOwedRecordsPush(hubPath, weftPath string, class PushFailureClass, cause error, opts SyncOptions) error {
	branch, err := gitrepo.New(weftPath).CurrentBranch()
	if err != nil {
		return cause
	}
	record := FailedPush{
		Repo:         failedPushSideRecords,
		RepoPath:     weftPath,
		CodeWorktree: pairedCodeWorktree(hubPath, weftPath),
		Branch:       branch,
		Class:        class,
		Error:        cause.Error(),
	}
	if opts.RunSlug != "" {
		record.Runs = []string{opts.RunSlug}
	}
	if err := recordPushFailure(hubPath, record, time.Now(), opts.notifier()); err != nil {
		logger.Warn("fabricengine: failed-push record not written", "side", failedPushSideRecords, "branch", branch, "error", err)
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
