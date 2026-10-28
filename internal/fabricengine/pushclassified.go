// pushclassified.go holds the one push every fabric push path runs per side.
// It checks the branch against origin, pushes rebase-free, retries a records rejection by replay, and records and reports a failure with its class.

package fabricengine

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/weftname"
)

// PushFailureRewound means origin rewound the branch past commits this machine holds, so the push is held back instead of restoring them.
const PushFailureRewound PushFailureClass = "rewound"

// ErrBranchGone is wrapped by the error of a push that was not run because origin no longer has the branch.
var ErrBranchGone error = errors.New("fabricengine: branch gone on origin")

// errPushHeldBack is wrapped by the error of a push that was not run because origin rewound the branch.
var errPushHeldBack = errors.New("fabricengine: push held back")

// PushFailedError is the error of a push that failed and was recorded as a failed push.
// Record holds the failed-push record, its class included, and Err the push's own error.
type PushFailedError struct {
	Record FailedPush
	Err    error
}

// Error returns the push's own error followed by the way forward of the record's class.
func (e *PushFailedError) Error() string {
	return fmt.Sprintf("%v; way forward: %s", e.Err, FailedPushWayForward(e.Record))
}

// Unwrap returns the push's own error, so errors.Is sees through to gitrepo.ErrPushRejected and gitrepo.ErrPushAuthRefused.
func (e *PushFailedError) Unwrap() error {
	return e.Err
}

// HasUnclassifiedFailure reports whether some failure in err is not a *PushFailedError.
// It reads each member of a joined error, so a caller that appends a way forward of its own to a push error appends it only when a failure carries none.
// A nil error holds no failure.
func HasUnclassifiedFailure(err error) bool {
	if err == nil {
		return false
	}
	if _, classified := err.(*PushFailedError); classified {
		return false
	}
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		return slices.ContainsFunc(joined.Unwrap(), HasUnclassifiedFailure)
	}
	if inner := errors.Unwrap(err); inner != nil {
		return HasUnclassifiedFailure(inner)
	}
	return true
}

// pushJob is one side's push in flight.
type pushJob struct {
	rec     *Mutations
	hubPath string
	side    string
	path    string
	opts    SyncOptions
	repo    *gitrepo.Repo
	branch  string
	prime   bool
}

// pushClassified pushes the side at path rebase-free and classifies a failure.
// side is "code", "records" or "bolt", which the failed-push record and its way forward read.
//
// A weft or a task branch with an upstream is first looked up on origin:
// a branch gone from origin is not pushed, so a parked push never recreates a branch landed or deleted elsewhere, and an entry origin has ended writes no record.
// A task branch origin rewound is not pushed either, since a plain push would restore the commits the rewind dropped.
// A gone branch whose entry is not ended and an unreachable origin are retried by one backoff before they are recorded.
//
// A rejection is retried before it is recorded, over the push backoff, while each retry fails with a bare rejection or an unnamed error such as a failed fetch, so a push that lost a race with another push of the branch lands instead of reporting a divergence.
// A records retry runs retryRecords, which replays the local commits onto origin's and pushes again;
// a deferral it reports writes a deferred record and returns nil, leaving the commits to the next push of the branch.
// A code-side retry fetches and pushes again only while origin's tip is contained in the local tip, and reports the rejection at once when origin holds commits the branch lacks.
// Every other failure is recorded and returned as a *PushFailedError holding the push's own error, under the "other" class when it matches none.
// A push that lands removes the record of its side and branch.
func pushClassified(rec *Mutations, hubPath, side, path string, opts SyncOptions, retryRecords func() (deferred bool, err error)) error {
	job := &pushJob{rec: rec, hubPath: hubPath, side: side, path: path, opts: opts, repo: gitrepo.New(path)}
	branch, err := job.repo.CurrentBranch()
	if err != nil {
		return job.push()
	}
	job.branch = branch
	if marked, err := job.repo.BranchMarkedLocal(branch); err != nil || marked {
		return job.push()
	}
	if side == failedPushSideCode {
		job.prime = job.isPrimeWorktree()
	}

	if side != failedPushSideBolt {
		proceed, err := job.checkOrigin()
		if !proceed {
			return err
		}
	}

	hadUnpushed, hadUnpushedErr := job.repo.HasUnpushed()
	if side == failedPushSideBolt && hadUnpushedErr == nil && !hadUnpushed {
		return job.clear()
	}
	pushErr := job.push()
	if pushErr == nil {
		return job.landed(hadUnpushed, hadUnpushedErr)
	}
	if errors.Is(pushErr, gitrepo.ErrLocalBranch) {
		return pushErr
	}
	if errors.Is(pushErr, gitrepo.ErrPushRejected) {
		retry := retryRecords
		if retry == nil {
			retry = job.repushOverOriginContained
		}
		deferred, retryErr := job.settleRejection(pushErr, retry)
		switch {
		case deferred:
			return job.deferral()
		case retryErr == nil:
			return job.landed(hadUnpushed, hadUnpushedErr)
		}
		pushErr = retryErr
	}
	if failed, classified := job.classifyFailure(pushErr); classified {
		return failed
	}
	return job.fail(PushFailureOther, pushErr, nil, nil, "")
}

// push runs the side's rebase-free push.
func (j *pushJob) push() error {
	return j.repo.PushRebaseFree()
}

// errOriginAhead is returned by a code-side retry that found origin holding commits the local branch lacks, a divergence no retry settles.
var errOriginAhead = errors.New("fabricengine: origin holds commits the local branch lacks")

// settleRejection retries a rejected push before it is reported, by running retry until it lands, defers or fails for a reason no retry clears.
// Two pushes of one branch can race, and the loser's rejection, or a fetch its retry made while the winner held the tracking ref, names a divergence the winner's landing already removed.
// retry runs again, after one delay of the push backoff slept through the job's BackoffSleep, only while it fails with a bare rejection or an error no class names, such as a failed fetch;
// it is bounded by the backoff's length.
// It returns the deferral or nil of a retry that deferred or landed, and otherwise the error to classify:
// rejected when a code-side retry finds origin ahead, the retry's own error when a class names it,
// and an unnamed error joined to rejected so the record still reads as a rejection and names the retry's error.
func (j *pushJob) settleRejection(rejected error, retry func() (deferred bool, err error)) (deferred bool, err error) {
	delays := gitrepo.PushBackoffDelays()
	sleep := j.opts.BackoffSleep
	if sleep == nil {
		sleep = time.Sleep
	}
	for attempt := 0; ; attempt++ {
		deferred, retryErr := retry()
		if deferred || retryErr == nil {
			return deferred, nil
		}
		if errors.Is(retryErr, errOriginAhead) {
			return false, rejected
		}
		named, retryable := rejectionRetryKind(retryErr)
		if !named {
			retryErr = errors.Join(rejected, retryErr)
		}
		if !retryable || attempt == len(delays) {
			return false, retryErr
		}
		logger.Warn("fabricengine: rejected push retried", "side", j.side, "branch", j.branch, "attempt", attempt+1, "error", retryErr)
		sleep(delays[attempt])
	}
}

// rejectionRetryKind sorts the failure of a retried rejected push.
// named reports whether a failure class names err.
// retryable reports whether another attempt may still land the push: true for a bare rejection and for an error no class names, such as a failed fetch.
func rejectionRetryKind(err error) (named, retryable bool) {
	var anchorStop *AnchorStopError
	var refusal *destructiveRefusal
	switch {
	case errors.As(err, &anchorStop), errors.Is(err, gitrepo.ErrPushAuthRefused):
		return true, false
	case errors.Is(err, gitrepo.ErrPushRejected):
		return true, true
	case errors.Is(err, ErrReplayConflict), errors.As(err, &refusal):
		return true, false
	case errors.Is(err, gitrepo.ErrPushRetriesExhausted), gitexec.IsTransportFailure(err):
		return true, false
	default:
		return false, true
	}
}

// repushOverOriginContained is the code side's retry of a rejected push: it fetches and pushes again while origin's tip is contained in the local tip, which makes the rejection a race or a stale tracking ref rather than a divergence.
// It returns errOriginAhead, without pushing, when origin holds commits the local branch lacks, and never rebases, merges or forces.
func (j *pushJob) repushOverOriginContained() (deferred bool, err error) {
	if err := j.repo.Fetch(); err != nil {
		return false, err
	}
	unpulled, err := j.repo.HasUnpulled()
	if err != nil {
		return false, err
	}
	if unpulled {
		return false, errOriginAhead
	}
	return false, j.repo.PushRebaseFree()
}

// isPrimeWorktree reports whether the worktree at the job's path is the hub's prime.
// A worktree whose prime cannot be resolved reads as the prime, since the task-branch checks only ever hold a push back.
func (j *pushJob) isPrimeWorktree() bool {
	l, err := lymcwd.ResolveWorktree(j.path)
	if err != nil {
		logger.Warn("fabricengine: prime worktree not resolved, the push is judged as the prime's", "path", j.path, "error", err)
		return true
	}
	prime, err := IsPrimeWorktree(l)
	if err != nil {
		logger.Warn("fabricengine: prime worktree not resolved, the push is judged as the prime's", "path", j.path, "error", err)
		return true
	}
	return prime
}

// landed records a push that landed and removes the record of its side and branch.
func (j *pushJob) landed(hadUnpushed bool, hadUnpushedErr error) error {
	if j.side != failedPushSideBolt {
		recordPushIfAdvanced(j.rec, j.repo, j.side, j.path, hadUnpushed, hadUnpushedErr)
	}
	return j.clear()
}

// clear removes the record of the job's side and branch.
func (j *pushJob) clear() error {
	if err := ClearPushFailure(j.hubPath, j.side, j.branch); err != nil {
		logger.Warn("fabricengine: failed-push record not cleared", "side", j.side, "branch", j.branch, "error", err)
	}
	return nil
}

// record writes a failed-push record of the job's side and branch and returns it.
func (j *pushJob) record(class PushFailureClass, cause error, paths, commits []string, originHead string) FailedPush {
	f := FailedPush{
		Repo:       j.side,
		RepoPath:   j.path,
		Branch:     j.branch,
		Prime:      j.side == failedPushSideCode && j.prime,
		Class:      class,
		Error:      cause.Error(),
		Paths:      paths,
		Commits:    commits,
		OriginHead: originHead,
	}
	if j.side == failedPushSideRecords {
		f.CodeWorktree = pairedCodeWorktree(j.hubPath, j.path)
	}
	var anchorStop *AnchorStopError
	if errors.As(cause, &anchorStop) {
		f.WayForward = anchorStop.WayForward
	}
	if err := RecordPushFailure(j.hubPath, f, time.Now()); err != nil {
		logger.Warn("fabricengine: failed-push record not written", "side", j.side, "branch", j.branch, "error", err)
	}
	return f
}

// fail records a failed push and returns it as a *PushFailedError.
func (j *pushJob) fail(class PushFailureClass, cause error, paths, commits []string, originHead string) error {
	return &PushFailedError{Record: j.record(class, cause, paths, commits, originHead), Err: cause}
}

// deferral records that the push was held back for a lock or an uncommitted change, which the branch's next push retries.
func (j *pushJob) deferral() error {
	j.record(PushFailureDeferred, errors.New("the push was deferred to the branch's next push"), nil, nil, "")
	return nil
}

// classifyFailure records the failure of a push that was run and returns it as a *PushFailedError.
// An *AnchorStopError is recorded under its own class ahead of the rejection classes, with the way forward it names, so the record never points at a merge that would carry the other marker in by hand.
// An error that matches no named class is returned unchanged and unrecorded, with classified false.
func (j *pushJob) classifyFailure(err error) (failed error, classified bool) {
	var conflict *replayConflict
	var refusal *destructiveRefusal
	var anchorStop *AnchorStopError
	switch {
	case errors.As(err, &anchorStop):
		return j.fail(PushFailureAnchorChange, err, nil, nil, ""), true
	case errors.Is(err, gitrepo.ErrPushAuthRefused):
		return j.fail(PushFailureAuth, err, nil, nil, ""), true
	case errors.Is(err, gitrepo.ErrPushRejected):
		return j.fail(PushFailureNotFastForward, err, nil, j.unpushedCommits(), ""), true
	case errors.Is(err, ErrReplayConflict), errors.As(err, &refusal):
		var paths []string
		if errors.As(err, &conflict) {
			paths = conflict.paths
		}
		return j.fail(PushFailureNotFastForward, fmt.Errorf("%w: %w", gitrepo.ErrPushRejected, err), paths, j.unpushedCommits(), ""), true
	case errors.Is(err, gitrepo.ErrPushRetriesExhausted), gitexec.IsTransportFailure(err):
		return j.fail(PushFailureTransport, err, nil, nil, ""), true
	default:
		return err, false
	}
}

// unpushedCommits returns the local commits the upstream, as last fetched, lacks, newest first.
func (j *pushJob) unpushedCommits() []string {
	upstream, err := j.repo.UpstreamSHA()
	if err != nil {
		return nil
	}
	head, err := j.repo.CurrentSHA()
	if err != nil {
		return nil
	}
	commits, err := j.repo.CommitsNotIn(head, upstream)
	if err != nil {
		return nil
	}
	return commits
}

// pairedCodeWorktree returns the code worktree paired with the records worktree at weftPath, or "" when weftPath is no weft sibling.
func pairedCodeWorktree(hubPath, weftPath string) string {
	slug, ok := WeftWarpSlug(filepath.Base(weftPath))
	if !ok {
		return ""
	}
	return filepath.Join(hubPath, slug)
}

// originView is what the check of a branch against origin found.
// head is origin's commit of the branch for a code side, gone says origin lacks the branch, and ended says its board entry is ended on origin.
// unchecked says the lookup failed for no reason of reaching origin, so nothing is known of the branch.
type originView struct {
	head      string
	gone      bool
	ended     bool
	unchecked bool
}

// checkOrigin judges the job's branch against origin before it is pushed, for a branch with an upstream.
// proceed is false when the push is not to run.
// err then holds the *PushFailedError of a push held back and recorded, the error of a judgment that failed, or nil for a branch whose entry origin has ended.
// A branch without an upstream was never pushed and has nothing to compare with.
func (j *pushJob) checkOrigin() (proceed bool, err error) {
	if _, upstreamErr := j.repo.UpstreamSHA(); upstreamErr != nil {
		if errors.Is(upstreamErr, gitrepo.ErrNoUpstream) {
			return true, nil
		}
		return false, upstreamErr
	}

	view, err := j.awaitOrigin()
	switch {
	case err != nil && gitexec.IsTransportFailure(err):
		return false, j.fail(PushFailureTransport, err, nil, nil, "")
	case err != nil:
		return false, err
	case view.unchecked:
		return true, nil
	case view.ended:
		return false, nil
	case view.gone:
		return false, j.fail(PushFailureGone, fmt.Errorf("%w: %s", ErrBranchGone, j.branch), nil, nil, "")
	}

	if j.side == failedPushSideCode && !j.prime {
		return j.holdBackRewound(view.head)
	}
	return true, nil
}

// awaitOrigin looks the branch up on origin, and retries a gone branch whose entry is not ended and an unreachable origin by one backoff.
// Each retry re-runs the lookup, the fetch of origin's Bolt branch and the judgment.
// The error is a transport failure still standing after the backoff, or a failed judgment of a gone branch's entry.
// A lookup that fails for no reason of reaching origin says nothing of the branch, so it returns an unchecked view and the push itself is left to judge.
func (j *pushJob) awaitOrigin() (originView, error) {
	delays := gitrepo.PushBackoffDelays()
	sleep := j.opts.BackoffSleep
	if sleep == nil {
		sleep = time.Sleep
	}
	for attempt := 0; ; attempt++ {
		view, err := j.observeOrigin()
		if err != nil && !gitexec.IsTransportFailure(err) {
			logger.Warn("fabricengine: origin's branch not looked up, the push goes on unchecked", "path", j.path, "branch", j.branch, "error", err)
			return originView{unchecked: true}, nil
		}
		if err == nil && view.gone {
			view.ended, err = goneEntryEnded(j.hubPath, j.branch)
			if err != nil && !gitexec.IsTransportFailure(err) {
				return originView{}, err
			}
		}
		if err == nil && (!view.gone || view.ended) {
			return view, nil
		}
		if attempt == len(delays) {
			return view, err
		}
		sleep(delays[attempt])
	}
}

// observeOrigin asks origin for the job's branch.
// A code side keeps origin's head, which the rewind check reads.
func (j *pushJob) observeOrigin() (originView, error) {
	if j.side == failedPushSideCode {
		head, found, err := originBranchHead(j.path, j.branch)
		return originView{head: head, gone: !found}, err
	}
	gone, err := branchGoneOnOrigin(j.path)
	return originView{gone: gone}, err
}

// holdBackRewound holds a task branch's push back when origin rewound it to originHead, and writes the rewound record.
// It proceeds when the branch is not rewound, including when origin's head is a commit this machine lacks.
func (j *pushJob) holdBackRewound(originHead string) (proceed bool, err error) {
	if _, resolveErr := j.repo.ResolveSHA(originHead + "^{commit}"); resolveErr != nil {
		return true, nil
	}
	localTip, err := j.repo.CurrentSHA()
	if err != nil {
		return false, err
	}
	trackingRef, err := gitexec.Run([]string{"rev-parse", "--symbolic-full-name", "@{u}"}, j.path)
	if err != nil {
		return false, err
	}
	earlier, err := trackingReflog(j.path, strings.TrimSpace(trackingRef))
	if err != nil {
		return false, err
	}
	kind, dropped, err := classifyRewrite(j.repo, localTip, originHead, earlier)
	if err != nil {
		return false, err
	}
	if kind != rewriteRewound {
		return true, nil
	}
	cause := fmt.Errorf("%w: origin rewound %s to %s, dropping %s", errPushHeldBack, j.branch, originHead, joinOrNone(dropped))
	return false, j.fail(PushFailureRewound, cause, nil, dropped, originHead)
}

// originBranchHead returns the commit origin holds for branch, asked by one ls-remote run in path, and whether origin has the branch.
func originBranchHead(path, branch string) (head string, found bool, err error) {
	ref := "refs/heads/" + branch
	out, err := gitexec.Run([]string{"ls-remote", "--heads", originRemoteName, ref}, path)
	if err != nil {
		return "", false, fmt.Errorf("fabricengine: ls-remote of %s in %s: %w", branch, path, err)
	}
	for _, line := range strings.Split(out, "\n") {
		sha, name, cut := strings.Cut(strings.TrimSpace(line), "\t")
		if cut && name == ref {
			return sha, true, nil
		}
	}
	return "", false, nil
}

// branchGoneOnOrigin reports whether origin no longer has the branch checked out at path, asked by one ls-remote of that branch.
func branchGoneOnOrigin(path string) (gone bool, err error) {
	branch, err := gitrepo.New(path).CurrentBranch()
	if err != nil {
		return false, err
	}
	_, found, err := originBranchHead(path, branch)
	return !found, err
}

// goneEntryEnded reports whether the board entry of a branch origin no longer has is ended on origin, done or removed.
// It fetches origin's Bolt branch once, and reads the ended set from that tip.
// The entry's slug is the branch's last path segment, without the records suffix.
// A hub with no board, or a board with no upstream, ends no entry.
func goneEntryEnded(hubPath, branch string) (bool, error) {
	boardDir := BoardDir(hubPath)
	if _, err := os.Stat(boardDir); err != nil {
		return false, nil
	}
	board := gitrepo.New(boardDir)
	if err := board.Fetch(); err != nil {
		return false, err
	}
	upstream, err := board.UpstreamSHA()
	if errors.Is(err, gitrepo.ErrNoUpstream) {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	name := strings.TrimSuffix(branch, weftname.Suffix)
	slug := name[strings.LastIndex(name, "/")+1:]
	replay, err := InstalledBoardReplay()
	if err != nil {
		return false, err
	}
	head, err := board.CurrentSHA()
	if err != nil {
		return false, err
	}
	data, err := blobAt(board, head, boardFileName)
	if err != nil {
		return false, err
	}
	entries, err := replay.Entries(data)
	if err != nil {
		return false, fmt.Errorf("%s at %s: %w", boardFileName, head, err)
	}
	created := map[string]string{}
	for _, entry := range entries {
		if entry.Slug == slug && !entry.Created.IsZero() {
			created[slug] = entry.Created.UTC().Format(time.RFC3339)
		}
	}
	ended, err := readEndedSet(boardDir, upstream, []string{slug}, created)
	if err != nil {
		return false, err
	}
	return len(ended) > 0, nil
}

// retryWeftPush replays the weft worktree's local commits onto origin's and pushes again, after a rejected push.
// It fetches the weft branch and origin's Bolt branch, then try-acquires the weft write lock while the caller holds the push lock.
// A held write lock and an uncommitted change on a path the replay touches report a deferral, with nothing changed.
// A replay that stops returns ErrReplayConflict with the conflicting paths, and a second rejection returns gitrepo.ErrPushRejected.
// A weft whose own anchor marker is empty, one whose upstream carries commits and another anchor marker, and one that carries a move record while its upstream has commits it lacks, return an *AnchorStopError before anything is replayed or changed.
func retryWeftPush(rec *Mutations, hubPath, weftPath string) (deferred bool, err error) {
	if stop := emptyMarkerStop(weftPath); stop != nil {
		return false, stop
	}
	repo := gitrepo.New(weftPath)
	if err := repo.Fetch(); err != nil {
		return false, fmt.Errorf("fabricengine: fetch the weft branch in %s: %w", weftPath, err)
	}
	if err := fetchBoltBranch(hubPath); err != nil {
		return false, err
	}

	lockDir, err := ensureWeftLockDirAt(weftPath)
	if err != nil {
		return false, err
	}
	writeLock, held, err := lock.TryAcquireWriteLock(filepath.Join(lockDir, weftWriteLockFile))
	if err != nil {
		return false, fmt.Errorf("fabricengine: acquire weft write lock: %w", err)
	}
	if !held {
		return true, nil
	}
	defer func() { _ = writeLock.Release() }()

	upstream, hasUpstream, err := upstreamSHAAt(weftPath)
	if err != nil {
		return false, err
	}
	if hasUpstream {
		head, err := repo.CurrentSHA()
		if err != nil {
			return false, err
		}
		slug, ok := WeftWarpSlug(filepath.Base(weftPath))
		if !ok {
			return false, fmt.Errorf("fabricengine: %s is no weft sibling worktree", weftPath)
		}
		// A replay rewrites the weft's commits, which a standing move record forbids; a push that needs no replay is no write to the weft.
		behind, err := repo.CommitsNotIn(upstream, head)
		if err != nil {
			return false, err
		}
		if len(behind) > 0 {
			moveStop, err := moveRecordStop(weftPath)
			if err != nil {
				return false, err
			}
			if moveStop != nil {
				return false, moveStop
			}
		}
		stop, err := upstreamAnchorStop(weftPath, filepath.Join(hubPath, slug), upstream)
		if err != nil {
			return false, err
		}
		if stop != nil {
			return false, stop
		}
		ahead, err := repo.CommitsNotIn(head, upstream)
		if err != nil {
			return false, err
		}
		l, err := lymcwd.ResolveWorktree(filepath.Join(filepath.Dir(weftPath), slug))
		if err != nil {
			return false, err
		}
		ended, err := (&Fabric{records: repo}).weftEndedByOrigin(l, ahead)
		if err != nil {
			return false, err
		}
		outcome, err := replayWeft(rec, l, weftPath, upstream, ended)
		if errors.Is(err, ErrReplayDirty) {
			return true, nil
		}
		if err != nil {
			return false, err
		}
		logWeftReplay(weftPath, outcome)
	}
	return false, repo.PushRebaseFree()
}

// fetchBoltBranch fetches origin's Bolt branch into the hub's board checkout, and does nothing for a hub with no board or a board with no upstream.
func fetchBoltBranch(hubPath string) error {
	boardDir := BoardDir(hubPath)
	if _, err := os.Stat(boardDir); err != nil {
		return nil
	}
	board := gitrepo.New(boardDir)
	if _, err := board.UpstreamSHA(); errors.Is(err, gitrepo.ErrNoUpstream) {
		return nil
	}
	if err := board.Fetch(); err != nil {
		return fmt.Errorf("fabricengine: fetch the board branch in %s: %w", boardDir, err)
	}
	return nil
}

// retryBoltPush replays the board's local commits onto origin's and pushes again, after a rejected push.
// It try-acquires the board write lock, then takes the push lock, in pullFirst's order, and holds both across the replay and the push.
// A held write lock and an uncommitted change on a path the replay touches report a deferral, with nothing changed.
// A replay that stops returns ErrReplayConflict, and a second rejection returns gitrepo.ErrPushRejected.
func retryBoltPush(rec *Mutations, boardDir string) (deferred bool, err error) {
	writeLock, held, err := lock.TryAcquireWriteLock(filepath.Join(boardDir, BoardWriteLockFile))
	if err != nil {
		return false, fmt.Errorf("fabricengine: acquire board write lock: %w", err)
	}
	if !held {
		return true, nil
	}
	defer func() { _ = writeLock.Release() }()
	pushLock, err := lock.AcquireWriteLock(filepath.Join(boardDir, gitrepo.PushLockFileName))
	if err != nil {
		return false, fmt.Errorf("fabricengine: acquire push lock: %w", err)
	}
	defer func() { _ = pushLock.Release() }()

	repo := gitrepo.New(boardDir)
	if err := repo.Fetch(); err != nil {
		return false, fmt.Errorf("fabricengine: fetch the board in %s: %w", boardDir, err)
	}
	upstream, err := repo.UpstreamSHA()
	if err != nil {
		return false, err
	}
	head, err := repo.CurrentSHA()
	if err != nil {
		return false, err
	}
	ahead, err := repo.CommitsNotIn(head, upstream)
	if err != nil {
		return false, err
	}

	bolt := NewBolt(boardDir)
	location, ended, err := bolt.endedByOrigin(repo, head, upstream, ahead)
	if err != nil {
		return false, err
	}
	outcome, err := replayBolt(rec, location, boardDir, upstream, ended)
	if errors.Is(err, ErrReplayDirty) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	bolt.logReplay(outcome)
	return false, repo.PushRebaseFree()
}
