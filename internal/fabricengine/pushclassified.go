// pushclassified.go holds the push that every fabric push path runs for each side.
// It checks the branch against origin and pushes rebase-free.
// It retries a rejection, then records and reports a failure with its class.

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
	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/weftname"
)

// PushFailureRewound means origin rewound the branch past commits this machine holds, so the push is held back instead of restoring them.
const PushFailureRewound PushFailureClass = "rewound"

// protectedBranchMarkers are the lower-case texts of GitHub's push rejection for a protected branch.
// Such a branch takes changes only through a pull request,
// and the markers cover classic branch protection and repository rules.
var protectedBranchMarkers = []string{"protected branch", "repository rule violations"}

// ProtectedBranchRefusal reports whether err is a push the remote refused because the branch is protected.
func ProtectedBranchRefusal(err error) bool {
	text := strings.ToLower(err.Error())
	for _, marker := range protectedBranchMarkers {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}

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
// It reads each member of a joined error, so a caller adds its own way forward only when no failure carries one.
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
// a branch gone from origin is not pushed, so a parked push never recreates a branch landed or deleted elsewhere,
// and an entry origin has ended writes no record.
// A task branch origin rewound is not pushed either, since a plain push would restore the commits the rewind dropped.
// A gone branch whose entry is not ended and an unreachable origin are retried by one backoff before they are recorded.
//
// A push that fails in transit is pushed again once per delay of the push backoff before it is recorded.
//
// A rejection is retried before it is recorded, over the push backoff.
// Retries continue while each fails with a bare rejection or an unnamed error such as a failed fetch,
// so a push that lost a race with another push of the branch lands instead of reporting a divergence.
// A records retry runs retryRecords, which replays the local commits onto origin's and pushes again;
// a deferral it reports writes a deferred record and returns nil, leaving the commits to the next push.
// A takeover it reports is returned as it is, with no record and no notice.
// That is a *TakeoverError or an error wrapping ErrHandedOver.
// A code-side retry fetches and pushes again only while origin's tip is contained in the local tip;
// it reports the rejection at once when origin holds commits the branch lacks.
// An unsettled rejection records as not_fast_forward when a fresh fetch shows origin holding commits the branch lacks;
// otherwise it records as other.
// Every other failure is recorded and returned as a *PushFailedError holding the push's own error;
// under the "other" class when it matches none.
// A code or records side whose origin layout cannot be read is recorded under the other class before origin is read.
// The layout's error names the config that restores it.
// A push that lands removes the record of its side and branch.
func pushClassified(rec *Mutations, hubPath, side, path string, opts SyncOptions, retryRecords func() (deferred bool, err error)) error {
	job := &pushJob{rec: rec, hubPath: hubPath, side: side, path: path, opts: opts, repo: gitrepo.New(path)}
	branch, err := job.repo.CurrentBranch()
	if err != nil {
		return job.push()
	}
	job.branch = branch
	if side == failedPushSideCode {
		job.prime = job.isPrimeWorktree()
	}
	if side == failedPushSideBolt {
		if err := job.refuseUnseededBoard(); err != nil {
			return err
		}
	}

	if side != failedPushSideBolt {
		if _, err := originNamespaceAt(path); err != nil {
			return job.fail(PushFailureOther, err, nil, nil, "")
		}
		proceed, err := job.checkOrigin()
		if !proceed {
			return err
		}
	}

	hadUnpushed, hadUnpushedErr := job.repo.HasUnpushed()
	if side == failedPushSideBolt && hadUnpushedErr == nil && !hadUnpushed {
		return job.clear()
	}
	pushErr := job.retryTransportPush(job.push())
	if pushErr == nil {
		return job.landed(hadUnpushed, hadUnpushedErr)
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
		if isTakeoverStop(pushErr) {
			return pushErr
		}
	}
	if failed, classified := job.classifyFailure(pushErr); classified {
		return failed
	}
	return job.fail(PushFailureOther, pushErr, nil, nil, "")
}

// push runs the side's rebase-free push, under the namespace of the repository it pushes from.
func (j *pushJob) push() error {
	ns, err := originNamespaceAt(j.path)
	if err != nil {
		return err
	}
	return j.repo.PushRebaseFree(ns)
}

// retryTransportPush pushes again after each delay of the push backoff while pushErr is a transport failure.
// It returns the first push that did not fail in transit, or the last transport failure once the backoff is spent.
// A connection that drops mid-push leaves no refusal to classify, so only another push of the branch settles it.
func (j *pushJob) retryTransportPush(pushErr error) error {
	sleep := j.opts.BackoffSleep
	if sleep == nil {
		sleep = time.Sleep
	}
	for attempt, delay := range gitrepo.PushBackoffDelays() {
		if !gitexec.IsTransportFailure(pushErr) {
			return pushErr
		}
		logger.Warn("fabricengine: push failed in transit, retrying", "side", j.side, "branch", j.branch, "attempt", attempt+1, "error", pushErr)
		sleep(delay)
		pushErr = j.push()
	}
	return pushErr
}

// refuseUnseededBoard refuses a board push for a board with no upstream in a one-repository records store.
// Such a board is a --single-repo seed whose clone stopped before it set the upstreams.
// A seeded board takes its upstream at the seed push, and a joined board adopts origin's board branch.
// A clone killed mid-seed can leave origin holding the whole seed,
// so the refusal reads origin's board to pick its way forward.
// It fetches origin's board first, so the comparison holds even for a commit another clone pushed after the seed.
// Origin holding no board names a --reset clone.
// Origin holding a board that shares history with this board is this hub's own seed,
// and it names the upstream settings the clone never wrote.
// Origin holding a board with no history in common with this one names a join of the hub that seeded it.
// So does origin holding a board while this one holds no commits.
// The refusal is returned as it is, with no failed-push record, so no later push or reconcile retries it;
// a push here would put an unseeded board onto origin unleased.
func (j *pushJob) refuseUnseededBoard() error {
	ns, err := originNamespaceAt(j.path)
	if err != nil {
		return err
	}
	if ns == (gitrepo.RefNamespace{}) {
		return nil
	}
	if _, upstreamErr := j.repo.UpstreamSHA(); !errors.Is(upstreamErr, gitrepo.ErrNoUpstream) {
		return nil
	}
	refused := fmt.Sprintf("fabricengine: the board at %s has no upstream in a one-repository hub, so it is not pushed", j.path)
	originTip, found, err := originBranchHead(j.path, j.branch)
	if err != nil {
		return fmt.Errorf("%s, and origin's board could not be read to tell whether its seed landed: %w; "+
			"way forward: run the same command again once origin answers", refused, err)
	}
	if !found {
		return fmt.Errorf("%s, and origin holds no board under %s, so it was never seeded onto origin; "+
			"way forward: run the same `lym fabric clone --single-repo` again with `--reset`", refused, RecordsNamespaceRoot)
	}
	cannotCompare := func(err error) error {
		return fmt.Errorf("%s, and origin's board could not be compared with this board: %w; "+
			"way forward: run the same command again once the records store at %s reads", refused, err, j.path)
	}
	if err := j.repo.Fetch(); err != nil {
		return fmt.Errorf("%s, and origin's board could not be fetched to compare it with this board: %w; "+
			"way forward: run the same command again once origin answers", refused, err)
	}
	join := fmt.Sprintf("way forward: remove the hub at %s and run `lym fabric clone <url>` without `--single-repo` to join that hub", j.hubPath)
	localTip, err := j.repo.CurrentSHA()
	switch {
	case errors.Is(err, gitrepo.ErrNoCommits):
		return fmt.Errorf("%s, and origin holds another board at %s while this board holds no commits, so another clone seeded origin; %s",
			refused, originTip, join)
	case err != nil:
		return cannotCompare(err)
	}
	shared, err := sharesHistory(j.path, originTip, localTip)
	if err != nil {
		return cannotCompare(err)
	}
	if !shared {
		return fmt.Errorf("%s, and origin holds another board at %s, which shares no history with this board at %s, so another clone seeded origin; %s",
			refused, originTip, localTip, join)
	}
	recordsBranch, err := gitexec.Run([]string{"config", "--get", PrimeRecordsBranchKey}, j.path)
	if err != nil {
		return fmt.Errorf("%s, and %s could not be read from the records store at %s to name the upstream settings: %w; "+
			"way forward: set %s there to the prime's records branch, then run the same command again",
			refused, PrimeRecordsBranchKey, j.path, err, PrimeRecordsBranchKey)
	}
	var settings []string
	for _, branch := range []string{j.branch, strings.TrimSpace(recordsBranch)} {
		settings = append(settings,
			fmt.Sprintf("`git -C %s config branch.%s.remote %s`", j.path, branch, originRemoteName),
			fmt.Sprintf("`git -C %s config branch.%s.merge %s`", j.path, branch, ns.Branch(branch)))
	}
	return fmt.Errorf("%s, but origin holds this hub's board at %s, so the seed landed and its clone stopped before it set the upstreams; "+
		"way forward: set them with %s, then run the same command again", refused, originTip, strings.Join(settings, ", "))
}

// sharesHistory reports whether the commits a and b have a common ancestor in the repository at dir.
// Both commits must exist in that repository.
func sharesHistory(dir, a, b string) (bool, error) {
	_, err := gitexec.Run([]string{"merge-base", a, b}, dir)
	var gitErr *gitexec.GitError
	switch {
	case err == nil:
		return true, nil
	case errors.As(err, &gitErr) && gitErr.ExitCode == 1:
		return false, nil
	default:
		return false, err
	}
}

// errOriginAhead is returned by a code-side retry that found origin holding commits the local branch lacks, a divergence no retry settles.
var errOriginAhead = errors.New("fabricengine: origin holds commits the local branch lacks")

// settleRejection retries a rejected push before it is reported.
// It runs retry until the push lands, defers, or fails for a reason no retry clears.
// Two pushes of one branch can race, and the loser's rejection names a divergence the winner's landing already removed.
// A fetch its retry made while the winner held the tracking ref can name one too.
// A retry runs again only while it fails with a bare rejection or an error no class names, such as a failed fetch.
// Each retry waits one delay of the push backoff, slept through the job's BackoffSleep,
// and the retries are bounded by the backoff's length.
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
// A refusal as a protected branch is not retryable, since no retry lifts the protection.
func rejectionRetryKind(err error) (named, retryable bool) {
	var anchorStop *AnchorStopError
	var refusal *destructiveRefusal
	switch {
	case errors.As(err, &anchorStop), errors.Is(err, gitrepo.ErrPushAuthRefused), isTakeoverStop(err), ProtectedBranchRefusal(err):
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

// repushOverOriginContained is the code side's retry of a rejected push.
// It fetches and pushes again while origin's tip is contained in the local tip;
// that containment makes the rejection a race or a stale tracking ref rather than a divergence.
// It returns errOriginAhead, without pushing, when origin holds commits the local branch lacks.
// A task pair's code side sent with SyncOptions.AfterRecords first resets a tip no transition of the run names.
// It pushes again when it did.
// It never rebases or merges, and forces only that reset.
func (j *pushJob) repushOverOriginContained() (deferred bool, err error) {
	if err := j.repo.Fetch(); err != nil {
		return false, err
	}
	unpulled, err := j.repo.HasUnpulled()
	if err != nil {
		return false, err
	}
	if unpulled {
		if j.side != failedPushSideCode || j.prime || !j.opts.AfterRecords {
			return false, errOriginAhead
		}
		reset, err := j.resetUnnamedTip()
		if err != nil {
			return false, err
		}
		if !reset {
			return false, errOriginAhead
		}
		if err := j.repo.Fetch(); err != nil {
			return false, err
		}
	}
	return false, j.push()
}

// resetUnnamedTip resets origin's code branch to the code tip the run's last transition on origin names.
// It does so when origin's tip is neither that tip nor an ancestor of it, which drops every commit past the named tip.
// The reset is leased at the origin tip read, and it fetches the records branch first to read the named tip.
// It reports false and changes nothing in four cases.
// The run has no transition on origin.
// This repository lacks the named tip.
// Origin's tip is the named tip or behind it.
// The lease is lost to a push that moved the branch meanwhile.
func (j *pushJob) resetUnnamedTip() (reset bool, err error) {
	slug := filepath.Base(j.path)
	weftPath := weftname.SiblingPath(j.hubPath, slug)
	if err := gitrepo.New(weftPath).Fetch(); err != nil {
		return false, fmt.Errorf("fetch the records branch of %q: %w", slug, err)
	}
	upstream, hasUpstream, err := upstreamSHAAt(weftPath)
	if err != nil || !hasUpstream {
		return false, err
	}
	named, err := lastTransitionCodeTip(weftPath, upstream, slug)
	if err != nil || named == "" {
		return false, err
	}
	held, err := holdsCommit(j.path, named)
	if err != nil || !held {
		return false, err
	}
	originTip, err := j.repo.UpstreamSHA()
	if err != nil || originTip == named {
		return false, err
	}
	originLags, err := isAncestorOrEqual(j.path, originTip, named)
	if err != nil || originLags {
		return false, err
	}
	err = updateRemoteBranch(j.rec, remoteBranchUpdateRequest{
		pathReq: pathRequest{
			what:      "reset the code branch of the run's pair on origin to the tip its last transition names",
			container: filepath.Dir(j.path),
			target:    j.path,
			ownership: ownedPairWarpCheckout(j.path, weftPath, ""),
			dirtiness: dirtinessNA("a move of origin's branch touches no checkout file"),
		},
		repo:     j.repo,
		remote:   originRemoteName,
		branch:   j.branch,
		sha:      named,
		leaseSHA: originTip,
	})
	if errors.Is(err, gitrepo.ErrLeaseRejected) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("reset origin's code branch %s to the tip %s its last transition names: %w", j.branch, named, err)
	}
	return true, nil
}

// isPrimeWorktree reports whether the worktree at the job's path is the hub's prime.
// A worktree whose prime cannot be resolved counts as the prime, since the task-branch checks only hold a push back.
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

// record builds the failed-push record of the job's side and branch, writes it and returns it.
// It writes nothing for an owed class when the job's options set OwedUnrecorded.
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
	if j.opts.RunSlug != "" {
		f.Runs = []string{j.opts.RunSlug}
	}
	var anchorStop *AnchorStopError
	if errors.As(cause, &anchorStop) {
		f.WayForward = anchorStop.WayForward
	}
	if j.opts.OwedUnrecorded && isOwedClass(class) {
		return f
	}
	if err := recordPushFailure(j.hubPath, f, time.Now(), j.opts.notifier()); err != nil {
		logger.Warn("fabricengine: failed-push record not written", "side", j.side, "branch", j.branch, "error", err)
	}
	return f
}

// fail records a failed push and returns it as a *PushFailedError.
// A failure of an owed class is not recorded, and posts no notice, when the job's options set OwedUnrecorded.
func (j *pushJob) fail(class PushFailureClass, cause error, paths, commits []string, originHead string) error {
	return &PushFailedError{Record: j.record(class, cause, paths, commits, originHead), Err: cause}
}

// deferral records that the push was held back for a lock or an uncommitted change.
// The branch's next push retries it.
// A records side whose worktree holds uncommitted tracked changes records them as the cause.
// Origin's commits cannot be replayed onto the worktree until those changes are committed or discarded.
// It returns nil, or with the job's OwedUnrecorded set the deferred class's unrecorded *PushFailedError.
func (j *pushJob) deferral() error {
	cause := errors.New("the push was deferred to the branch's next push")
	if j.side == failedPushSideRecords && j.hasUncommittedChanges() {
		cause = fmt.Errorf("origin's commits cannot be replayed onto %s until its uncommitted changes are committed or discarded", j.path)
	}
	record := j.record(PushFailureDeferred, cause, nil, nil, "")
	if j.opts.OwedUnrecorded {
		return &PushFailedError{Record: record, Err: cause}
	}
	return nil
}

// hasUncommittedChanges reports whether the job's worktree holds an uncommitted change to a tracked file.
// A worktree whose status cannot be read reports none.
func (j *pushJob) hasUncommittedChanges() bool {
	out, err := gitexec.Run([]string{"status", "--porcelain", "--untracked-files=no"}, j.path)
	return err == nil && strings.TrimSpace(out) != ""
}

// classifyFailure records the failure of a push that was run and returns it as a *PushFailedError.
// An *AnchorStopError is recorded under its own class ahead of the rejection classes, with the way forward it names.
// That keeps the record from pointing at a merge that would carry the other marker in by hand.
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
		return j.failRejection(err), true
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

// failRejection records a rejection the retry did not settle, classed by what a fresh fetch shows origin holding.
// Only origin holding commits the branch lacks is a divergence, recorded as not_fast_forward with the local commits.
// Origin holding nothing the branch lacks is recorded as other.
// That is the case after a push that overlapped another, or under a declining hook.
// Its notice then names a retry and no merge.
// Origin's tip is the branch's upstream, or origin's branch of the same name for a branch never given one.
// A failed fetch, or a tip that cannot be read because origin has no such branch, leaves origin unknown.
// The failure is recorded as other with that error joined.
func (j *pushJob) failRejection(rejected error) error {
	if err := j.repo.Fetch(); err != nil {
		return j.fail(PushFailureOther, errors.Join(rejected, err), nil, nil, "")
	}
	originTip, err := j.repo.UpstreamSHA()
	if errors.Is(err, gitrepo.ErrNoUpstream) {
		originTip, err = j.repo.ResolveSHA("refs/remotes/" + originRemoteName + "/" + j.branch)
	}
	if err != nil {
		return j.fail(PushFailureOther, errors.Join(rejected, err), nil, nil, "")
	}
	head, err := j.repo.CurrentSHA()
	if err != nil {
		return j.fail(PushFailureOther, errors.Join(rejected, err), nil, nil, "")
	}
	originOnly, err := j.repo.CommitsNotIn(originTip, head)
	switch {
	case err != nil:
		return j.fail(PushFailureOther, errors.Join(rejected, err), nil, nil, "")
	case len(originOnly) > 0:
		return j.fail(PushFailureNotFastForward, rejected, nil, j.unpushedCommits(), "")
	default:
		return j.fail(PushFailureOther, fmt.Errorf("%w; a fetch found origin holding nothing this branch lacks", rejected), nil, nil, "")
	}
}

// unpushedCommits returns the local commits origin's tip, as last fetched, lacks, newest first.
// Origin's tip is the branch's upstream, or origin's branch of the same name for a branch never given one.
func (j *pushJob) unpushedCommits() []string {
	upstream, err := j.repo.UpstreamSHA()
	if errors.Is(err, gitrepo.ErrNoUpstream) {
		upstream, err = j.repo.ResolveSHA("refs/remotes/" + originRemoteName + "/" + j.branch)
	}
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

// pairedCodeWorktree returns the code worktree paired with the records worktree at weftPath.
// It returns "" when weftPath is no weft sibling.
func pairedCodeWorktree(hubPath, weftPath string) string {
	slug, ok := WeftWarpSlug(filepath.Base(weftPath))
	if !ok {
		return ""
	}
	return filepath.Join(hubPath, slug)
}

// originView is what the check of a branch against origin found.
// head is origin's commit of the branch for a code side.
// gone says origin lacks the branch.
// ended says its board entry is ended on origin.
// unchecked says the lookup failed for no reason of reaching origin, so nothing is known of the branch.
type originView struct {
	head      string
	gone      bool
	ended     bool
	unchecked bool
}

// checkOrigin judges the job's branch against origin before it is pushed, for a branch with an upstream.
// proceed is false when the push is not to run.
// err then holds the *PushFailedError of a push held back and recorded, or the error of a judgment that failed.
// It is nil for a branch whose entry origin has ended.
// That branch's local tracking ref is deleted first, with its upstream setting kept,
// so a local read of the branch sees an upstream with no ref and never a stale ref that lacks the local tip.
// A branch without an upstream was never pushed and has nothing to compare with.
// An upstream whose tracking ref is absent, as after a fetch pruned it, is looked up on origin like any other.
// A branch origin still holds is then pushed; the rewind check, which reads the ref's history, is skipped.
func (j *pushJob) checkOrigin() (proceed bool, err error) {
	refPruned := false
	if _, upstreamErr := j.repo.UpstreamSHA(); upstreamErr != nil {
		switch {
		case errors.Is(upstreamErr, gitrepo.ErrNoUpstream):
			return true, nil
		case errors.Is(upstreamErr, gitrepo.ErrRefNotFound):
			refPruned = true
		default:
			return false, upstreamErr
		}
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
		return false, j.dropTrackingRefOfEndedBranch()
	case view.gone:
		return false, j.fail(PushFailureGone, fmt.Errorf("%w: %s", ErrBranchGone, j.branch), nil, nil, "")
	}

	if j.side == failedPushSideCode && !j.prime && !refPruned {
		return j.holdBackRewound(view.head)
	}
	return true, nil
}

// dropTrackingRefOfEndedBranch deletes the local tracking ref of the job's branch, which origin no longer has.
// The branch keeps its upstream setting.
func (j *pushJob) dropTrackingRefOfEndedBranch() error {
	ns, err := originNamespaceAt(j.path)
	if err != nil {
		return err
	}
	if err := j.repo.DeleteRemoteTrackingRef(originRemoteName, j.branch, ns); err != nil {
		return fmt.Errorf("fabricengine: delete the tracking ref of the branch %s, whose entry origin ended: %w", j.branch, err)
	}
	return nil
}

// awaitOrigin looks the branch up on origin.
// It retries a gone branch whose entry is not ended, and an unreachable origin, once per backoff delay.
// Each retry re-runs the lookup, the fetch of origin's Bolt branch and the judgment.
// The error is a transport failure still standing after the last backoff delay.
// Otherwise it is a failed judgment of a gone branch's entry.
// A lookup that fails for no reason of reaching origin says nothing of the branch,
// so it returns an unchecked view and the push itself is left to judge.
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

// originBranchHead returns the commit origin holds for branch, and whether origin has the branch.
// It asks with one ls-remote run in path.
func originBranchHead(path, branch string) (head string, found bool, err error) {
	ns, err := originNamespaceAt(path)
	if err != nil {
		return "", false, err
	}
	ref := ns.Branch(branch)
	out, err := gitexec.Run([]string{"ls-remote", originRemoteName, ref}, path)
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

// branchGoneOnOrigin reports whether origin no longer has path's current branch, asked by one ls-remote of that branch.
func branchGoneOnOrigin(path string) (gone bool, err error) {
	branch, err := gitrepo.New(path).CurrentBranch()
	if err != nil {
		return false, err
	}
	_, found, err := originBranchHead(path, branch)
	return !found, err
}

// goneEntryEnded reports whether the run of a branch origin no longer has is ended on origin, done or removed.
// It fetches origin's Bolt branch once, and reads the ended set from that tip.
// The run's slug is the branch's last path segment, without the records suffix.
// A hub with no Bolt checkout, or one with no upstream, ends no run.
func goneEntryEnded(hubPath, branch string) (bool, error) {
	boardDir := BoltDir(hubPath)
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
	ledger, _, err := runLedger()
	if err != nil {
		return false, err
	}
	head, err := board.CurrentSHA()
	if err != nil {
		return false, err
	}
	runs, err := ledgerRunsAt(board, ledger, head)
	if err != nil {
		return false, fmt.Errorf("%s at %s: %w", ledger.File(), head, err)
	}
	created := map[string]string{}
	for _, run := range runs {
		if run.Slug == slug && !run.Created.IsZero() {
			created[slug] = run.Created.UTC().Format(time.RFC3339)
		}
	}
	ended, err := readEndedSet(boardDir, upstream, []string{slug}, created)
	if err != nil {
		return false, err
	}
	return len(ended) > 0, nil
}

// retryWeftPush replays the weft worktree's local commits onto origin's and pushes again, after a rejected push.
// It fetches the weft branch and then reads origin's record of the run runSlug through takeoverStop:
// a run taken over by a machine here does not hold returns an error before anything is replayed or changed.
// The error is a *TakeoverError, or wraps ErrHandedOver.
// A zero here skips that read, and so does an empty runSlug for a pair's records worktree.
// On the prime's records worktree the read covers every batten run a local commit origin lacks names, and runSlug too.
// It returns a *TakeoversError holding each pending takeover, with nothing replayed or pushed.
// A failed read returns an error wrapping ErrTakeoverPassOwed.
// The replay sets aside the commits of a run this machine already handed over, as it does an ended run's.
// It then fetches origin's Bolt branch, and try-acquires the weft write lock, while the caller holds the push lock.
// A held write lock and an uncommitted change on a path the replay touches report a deferral, with nothing changed.
// A replay that stops returns ErrReplayConflict with the conflicting paths,
// and a second rejection returns gitrepo.ErrPushRejected.
// A weft whose own anchor marker is empty returns an *AnchorStopError before anything is replayed or changed.
// So does a weft whose upstream carries commits and another anchor marker.
// So does a weft that carries a move record while its upstream has commits it lacks.
func retryWeftPush(rec *Mutations, hubPath, weftPath, runSlug string, here MachineNames) (deferred bool, err error) {
	if stop := emptyMarkerStop(weftPath); stop != nil {
		return false, stop
	}
	repo := gitrepo.New(weftPath)
	if err := repo.Fetch(); err != nil {
		return false, fmt.Errorf("fabricengine: fetch the weft branch in %s: %w", weftPath, err)
	}
	var handedOver []string
	if !here.isZero() {
		warpLocation, err := warpLocationOfRecords(weftPath)
		if err != nil {
			return false, err
		}
		prime, err := IsPrimeWorktree(warpLocation)
		if err != nil {
			return false, err
		}
		switch {
		case prime:
			if handedOver, err = primePushTakeovers(warpLocation, weftPath, runSlug, here); err != nil {
				return false, err
			}
		case runSlug != "":
			if err := takeoverStop(warpLocation, weftPath, runSlug, here); err != nil {
				return false, err
			}
		}
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
		// A replay rewrites the weft's commits, which a standing move record forbids;
		// a push that needs no replay is no write to the weft.
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
		if len(handedOver) > 0 && ended == nil {
			ended = EndedSet{}
		}
		for _, slug := range handedOver {
			ended[slug] = EndedHandedOver
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
	ns, err := originNamespaceAt(weftPath)
	if err != nil {
		return false, err
	}
	return false, repo.PushRebaseFree(ns)
}

// fetchBoltBranch fetches origin's Bolt branch into the hub's board checkout.
// It does nothing for a hub with no board or a board with no upstream.
func fetchBoltBranch(hubPath string) error {
	boardDir := BoltDir(hubPath)
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
// It try-acquires the board write lock, then takes the push lock, in the order the pull path uses.
// It holds both locks across the replay and the push.
// It passes the process's pause gate before it tries the write lock and stays admitted until both are released.
// A held write lock and an uncommitted change on a path the replay touches report a deferral, with nothing changed.
// A replay that stops returns ErrReplayConflict, and a second rejection returns gitrepo.ErrPushRejected.
func retryBoltPush(rec *Mutations, boardDir string) (deferred bool, err error) {
	writeLockPath, err := BoltLockPath(boardDir, BoardWriteLockFile)
	if err != nil {
		return false, err
	}
	leaveGate := pauserecord.Admit(pushLockGateOp)
	defer leaveGate()
	writeLock, held, err := lock.TryAcquireWriteLock(writeLockPath)
	if err != nil {
		return false, fmt.Errorf("fabricengine: acquire board write lock: %w", err)
	}
	if !held {
		return true, nil
	}
	defer func() { _ = writeLock.Release() }()
	repo := gitrepo.New(boardDir)
	pushLockPath, err := repo.PushLockPath()
	if err != nil {
		return false, err
	}
	pushLock, err := lock.AcquireWriteLock(pushLockPath)
	if err != nil {
		return false, fmt.Errorf("fabricengine: acquire push lock: %w", err)
	}
	defer func() { _ = pushLock.Release() }()

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
	ns, err := originNamespaceAt(boardDir)
	if err != nil {
		return false, err
	}
	return false, repo.PushRebaseFree(ns)
}
