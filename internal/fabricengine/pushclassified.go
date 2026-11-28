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

	"github.com/Knatte18/loomyard/internal/beamsetting"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/fabriccore"
	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/weftname"
)

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
	Record fabriclayout.FailedPush
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
	// unmade is the commit on origin's branch that this machine did not make, which kept the branch from being reset.
	// unmadeAt is origin's tip at that read. Both are empty when no reset was refused.
	unmade   string
	unmadeAt string
	// actionErrs holds the error of each action run, in the order of the outcomes that are no wait.
	actionErrs []error
	// pushes and retries count the pushes and the rejection retries run so far, for the log of a wait.
	pushes  int
	retries int
	// originErr is the error of the last lookup on origin that failed in transit.
	originErr error
	// boardErr is the failed read an unseeded board's refusal names.
	boardErr error
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
// With opts.SingleAttempt neither the push nor the lookup of the branch is retried over a transport failure.
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
//
// The code side of a worktree whose detach marker stands is not pushed.
// It returns a *PushFailedError of class deferred that names the marker, so the push reads as owed.
func pushClassified(rec *Mutations, hubPath, side, path string, opts SyncOptions, retryRecords func() (deferred bool, err error)) error {
	job := &pushJob{rec: rec, hubPath: hubPath, side: side, path: path, opts: opts, repo: gitrepo.New(path)}
	if side == fabriclayout.RepoCode {
		l, marked, stands, err := markedWorktree(path)
		if err != nil {
			return err
		}
		if stands {
			job.branch = marked
			cause := fmt.Errorf("the code branch %s is not pushed while the detach marker %s stands; to end the detach by hand, %s",
				marked, DetachMarkerPath(l), detachWayForward(l))
			return job.fail(fabriclayout.PushFailureDeferred, cause, nil, nil, "")
		}
	}
	snapshot, err := job.run(retryRecords)
	if err != nil {
		var refusal *fabriccore.Refusal
		if errors.As(err, &refusal) && job.boardErr != nil {
			return &refusalOverRead{refusal: refusal, read: job.boardErr}
		}
		return err
	}

	switch verdict := fabriccore.JudgePush(snapshot); verdict.Kind {
	case fabriccore.PushVerdictDone:
		return nil
	case fabriccore.PushVerdictReturn:
		if verdict.Outcome >= 0 {
			return job.errOf(snapshot, verdict.Outcome)
		}
		return verdict.Err
	case fabriccore.PushVerdictLanded:
		return job.landed(snapshot.Push.Unpushed, snapshot.Push.UnpushedErr)
	case fabriccore.PushVerdictCleared:
		return job.clear()
	case fabriccore.PushVerdictDeferred:
		return job.deferral()
	default:
		return job.classifyFailure(snapshot, verdict)
	}
}

// refusalOverRead is a refusal that names a failed read, which errors.Is and errors.As still reach.
type refusalOverRead struct {
	refusal *fabriccore.Refusal
	read    error
}

// Error returns the refusal's text.
func (e *refusalOverRead) Error() string { return e.refusal.Error() }

// Unwrap returns the refusal and the failed read.
func (e *refusalOverRead) Unwrap() []error { return []error{e.refusal, e.read} }

// run drives the push's rule and returns the snapshot it read.
// retryRecords retries a rejected push; a nil one retries the code side's way.
func (j *pushJob) run(retryRecords func() (deferred bool, err error)) (fabriccore.Snapshot, error) {
	retry := retryRecords
	if retry == nil {
		retry = j.repushOverOriginContained
	}
	return runRule(fabriccore.PushSide, fabriccore.PushSide(fabriccore.Snapshot{}), j.read, nil, j.runners(retry), j.sleep)
}

// sleep waits one delay of the push backoff through the job's BackoffSleep.
func (j *pushJob) sleep(delay time.Duration) {
	// A wait before the first push is one for origin, and is not logged.
	switch {
	case j.retries > 0:
		logger.Warn("fabricengine: rejected push retried", "side", j.side, "branch", j.branch, "attempt", j.retries, "error", j.actionErrs[len(j.actionErrs)-1])
	case j.pushes > 0:
		logger.Warn("fabricengine: push failed in transit, retrying", "side", j.side, "branch", j.branch, "attempt", j.pushes, "error", j.actionErrs[len(j.actionErrs)-1])
	}
	if j.opts.BackoffSleep != nil {
		j.opts.BackoffSleep(delay)
		return
	}
	time.Sleep(delay)
}

// runners returns the runners of the actions the push's rule lists; retry runs the retry of a rejection.
func (j *pushJob) runners(retry func() (deferred bool, err error)) map[fabriccore.ActionKind]actionRunner {
	keep := func(err error) error {
		j.actionErrs = append(j.actionErrs, err)
		return err
	}
	return map[fabriccore.ActionKind]actionRunner{
		fabriccore.ActionPush: func(fabriccore.Action) error {
			j.pushes++
			return keep(j.push())
		},
		fabriccore.ActionSeamCall: func(fabriccore.Action) error {
			j.retries++
			deferred, err := retry()
			if deferred {
				err = errRetryDeferred
			}
			return keep(err)
		},
		fabriccore.ActionRefDelete: func(fabriccore.Action) error {
			return keep(j.dropTrackingRefOfEndedBranch())
		},
	}
}

// errOf returns the error of the action whose outcome is at index in snapshot.
func (j *pushJob) errOf(snapshot fabriccore.Snapshot, index int) error {
	ran := 0
	for _, outcome := range snapshot.Outcomes[:index] {
		if outcome.Action.Kind != fabriccore.ActionWait {
			ran++
		}
	}
	return j.actionErrs[ran]
}

// read fills what the push's rule asks for.
func (j *pushJob) read(phase fabriccore.Phase, fields []fabriccore.Field, snapshot *fabriccore.Snapshot) error {
	for _, field := range fields {
		switch field {
		case fabriccore.FieldPushSide:
			switch {
			case !snapshot.Push.Read:
				j.readSide(snapshot)
			case phase == fabriccore.PhaseOrigin:
				if err := j.checkOrigin(snapshot); err != nil {
					return err
				}
			default:
				snapshot.Push.Unpushed, snapshot.Push.UnpushedErr = j.repo.HasUnpushed()
				snapshot.Push.UnpushedRead = true
			}
		case fabriccore.FieldPushJudgment:
			snapshot.Push.Judgment = j.failRejection()
		}
	}
	return nil
}

// readSide reads the push's single-attempt option, branch and repository.
// A branch that cannot be read ends the reading, since the push then goes out unchecked.
func (j *pushJob) readSide(snapshot *fabriccore.Snapshot) {
	snapshot.Backoff = gitrepo.PushBackoffDelays()
	facts := fabriccore.PushSideFacts{
		Read:          true,
		Side:          j.side,
		SingleAttempt: j.opts.SingleAttempt,
	}
	defer func() { snapshot.Push = facts }()

	branch, err := j.repo.CurrentBranch()
	if err != nil {
		return
	}
	j.branch, facts.Branch = branch, branch
	if j.side == fabriclayout.RepoCode {
		j.prime = j.isPrimeWorktree()
		facts.Prime = j.prime
	}
	if j.side == fabriclayout.RepoBolt {
		facts.Board, facts.NamespaceErr = j.refuseUnseededBoard()
		return
	}
	if _, facts.NamespaceErr = originNamespaceAt(j.path); facts.NamespaceErr != nil {
		return
	}
	_, err = j.repo.UpstreamSHA()
	switch {
	case err == nil:
		facts.Upstream = fabriccore.PushUpstreamTracked
	case errors.Is(err, gitrepo.ErrNoUpstream):
		facts.Upstream = fabriccore.PushUpstreamNone
	case errors.Is(err, gitrepo.ErrRefNotFound):
		facts.Upstream = fabriccore.PushUpstreamPruned
	default:
		facts.UpstreamErr = err
	}
}

// push runs the side's rebase-free push, under the namespace of the repository it pushes from.
// A job with a ceiling sends no commit past it.
func (j *pushJob) push() error {
	ns, err := originNamespaceAt(j.path)
	if err != nil {
		return err
	}
	if j.opts.ceiling != "" {
		// The empty remote makes it the default push, which gives a branch with no upstream its upstream.
		return j.repo.PushCommit("", j.opts.ceiling, j.branch, ns)
	}
	return j.repo.PushRebaseFree(ns)
}

// refuseUnseededBoard reads what the refusal of an unseeded board rests on:
// a board with no upstream in a one-repository records store.
// A seeded board takes its upstream at the seed push, and a joined board adopts origin's board branch.
// A clone killed mid-seed can leave origin holding the whole seed, so origin's board is read too.
// It is fetched first, so the comparison holds even for a commit another clone pushed after the seed.
// The reading stops at the first fact that decides the refusal.
// nsErr is a failed read of the records store's layout.
func (j *pushJob) refuseUnseededBoard() (board fabriccore.PushBoardFacts, nsErr error) {
	ns, err := originNamespaceAt(j.path)
	if err != nil {
		return board, err
	}
	if ns == (gitrepo.RefNamespace{}) {
		return board, nil
	}
	if _, upstreamErr := j.repo.UpstreamSHA(); !errors.Is(upstreamErr, gitrepo.ErrNoUpstream) {
		return board, nil
	}
	board = fabriccore.PushBoardFacts{Unseeded: true, Path: j.path, HubPath: j.hubPath, Remote: originRemoteName, DiscardWorkFlag: DiscardWorkFlag}
	board.OriginTip, board.OriginFound, board.OriginReadErr = originBranchHead(j.path, j.branch)
	if board.OriginReadErr != nil {
		j.boardErr = board.OriginReadErr
		return board, nil
	}
	if !board.OriginFound {
		return board, nil
	}
	if board.FetchErr = j.repo.Fetch(); board.FetchErr != nil {
		j.boardErr = board.FetchErr
		return board, nil
	}
	board.LocalTip, err = j.repo.CurrentSHA()
	switch {
	case errors.Is(err, gitrepo.ErrNoCommits):
		board.NoCommits = true
		return board, nil
	case err != nil:
		board.CompareErr, j.boardErr = err, err
		return board, nil
	}
	board.Shared, board.CompareErr = sharesHistory(j.path, board.OriginTip, board.LocalTip)
	if board.CompareErr != nil {
		j.boardErr = board.CompareErr
		return board, nil
	}
	if !board.Shared {
		return board, nil
	}
	recordsBranch, err := gitexec.Run([]string{"config", "--get", fabriclayout.PrimeRecordsBranchKey}, j.path)
	if err != nil {
		board.RecordsBranchErr, j.boardErr = err, err
		return board, nil
	}
	for _, branch := range []string{j.branch, strings.TrimSpace(recordsBranch)} {
		board.Upstreams = append(board.Upstreams, fabriccore.PushUpstreamSetting{Branch: branch, MergeRef: ns.Branch(branch)})
	}
	return board, nil
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

// repushOverOriginContained is the code side's retry of a rejected push.
// It fetches and pushes again while origin's tip is contained in the local tip;
// that containment makes the rejection a race or a stale tracking ref rather than a divergence.
// It returns errOriginAhead, without pushing, when origin holds commits the local branch lacks.
// A task pair's code side sent with SyncOptions.AfterRecords first resets a tip no transition of the run names.
// It pushes again when it did.
// A push with a ceiling resets nothing, since the named tip may lie past the ceiling.
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
		if j.side != fabriclayout.RepoCode || j.prime || !j.opts.AfterRecords || j.opts.ceiling != "" {
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
// It reports false and changes nothing in five cases.
// A commit past the named tip is one this machine did not make, which the job then names for its refusal.
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
	unmade, err := unmadeOriginCommit(j.path, named, originTip, j.branch)
	if err != nil {
		return false, err
	}
	if unmade != "" {
		j.unmade, j.unmadeAt = unmade, originTip
		return false, nil
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

// unmadeOriginCommit returns the newest commit originTip holds past named that this machine did not make, or "".
// A commit counts as made here when a local branch, or an entry of the reflog of the local branch, reaches it.
// Remote-tracking refs do not count, since a fetch fills them with origin's commits.
// The answer relies on git keeping reflog entries until its reflog expiry.
func unmadeOriginCommit(path, named, originTip, branch string) (string, error) {
	listing, err := gitexec.Run([]string{"reflog", "show", "--format=%H", "refs/heads/" + branch}, path)
	if err != nil {
		return "", fmt.Errorf("read the reflog of %s in %s: %w", branch, path, err)
	}
	entries := strings.Fields(listing)
	slices.Sort(entries)
	// The revisions go on stdin, since a long-lived branch's reflog exceeds the Windows command-line limit.
	revisions := []string{originTip, "^" + named}
	for _, entry := range slices.Compact(entries) {
		revisions = append(revisions, "^"+entry)
	}
	out, err := gitexec.RunStdin([]string{"rev-list", "--stdin", "--not", "--branches"}, path, strings.Join(revisions, "\n")+"\n")
	if err != nil {
		return "", fmt.Errorf("list the commits of %s past %s that this machine did not make in %s: %w", originTip, named, path, err)
	}
	commits := strings.Fields(out)
	if len(commits) == 0 {
		return "", nil
	}
	return commits[0], nil
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
	switch {
	case j.side == fabriclayout.RepoBolt:
	case j.opts.ceiling != "":
		// A capped push runs only while origin lacks its ceiling, and leaves the branch ahead of origin.
		appendBranchPushed(j.rec, j.repo, j.side, j.path)
	default:
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
func (j *pushJob) record(class fabriclayout.PushFailureClass, cause error, paths, commits []string, originHead string) fabriclayout.FailedPush {
	f := fabriclayout.FailedPush{
		Repo:       j.side,
		RepoPath:   j.path,
		Branch:     j.branch,
		Prime:      j.side == fabriclayout.RepoCode && j.prime,
		Class:      class,
		Error:      cause.Error(),
		Paths:      paths,
		Commits:    commits,
		OriginHead: originHead,
	}
	if j.side == fabriclayout.RepoRecords {
		f.CodeWorktree = pairedCodeWorktree(j.hubPath, j.path)
	}
	if j.opts.RunSlug != "" {
		f.Runs = []string{j.opts.RunSlug}
	}
	var anchorStop *AnchorStopError
	if errors.As(cause, &anchorStop) {
		f.WayForward = anchorStop.WayForward
	}
	// A refused reset keeps its own record, which names the commit this machine did not make.
	if len(j.opts.landings) > 0 && j.unmade == "" {
		switch {
		case class == fabriclayout.PushFailureNotFastForward:
			f.Commits = j.opts.landings
			f.WayForward = fmt.Sprintf("origin lacks the landing commits %s; %s",
				strings.Join(j.opts.landings, " "), codeDivergedWayForward(j.path, j.branch, j.prime))
		case class == fabriclayout.PushFailureOther && ProtectedBranchRefusal(cause):
			f.WayForward = protectedParentWayForward(j.path, j.branch)
		}
	}
	if j.opts.OwedUnrecorded && isOwedClass(class) {
		return f
	}
	if err := recordPushFailure(j.hubPath, f, time.Now(), j.opts.notifier()); err != nil {
		logger.Warn("fabricengine: failed-push record not written", "side", j.side, "branch", j.branch, "error", err)
	}
	return f
}

// protectedParentWayForward is the way forward of a landing push that origin refused as protected.
// branch is the parent branch, and worktree is the path of its worktree.
// A plain push is refused every time, so the landed work goes through a pull request by hand;
// the setting it names only spares later landings.
func protectedParentWayForward(worktree, branch string) string {
	return fmt.Sprintf("origin protects %s, so a push of it is refused every time; "+
		"merge each landed task branch into %s through a pull request by hand on GitHub, "+
		"then run `git fetch origin` and `git reset --keep origin/%s` in %s; "+
		"so that later landings go through a pull request lym squash-merges at once, set %s to trail, "+
		"with `lym config beam --set pull_request=trail` in the parent's worktree, "+
		"`lym config beam --layer default --set pull_request=trail` from the prime, or the task's run_settings",
		branch, branch, branch, worktree, configengine.KeyName(beamsetting.Module, beamsetting.PullRequestKey))
}

// fail records a failed push and returns it as a *PushFailedError.
// A failure of an owed class is not recorded, and posts no notice, when the job's options set OwedUnrecorded.
func (j *pushJob) fail(class fabriclayout.PushFailureClass, cause error, paths, commits []string, originHead string) error {
	return &PushFailedError{Record: j.record(class, cause, paths, commits, originHead), Err: cause}
}

// deferral records that the push was held back for a lock or an uncommitted change.
// The branch's next push retries it.
// A records side whose worktree holds uncommitted tracked changes records them as the cause.
// Origin's commits cannot be replayed onto the worktree until those changes are committed or discarded.
// It returns nil, or with the job's OwedUnrecorded set the deferred class's unrecorded *PushFailedError.
func (j *pushJob) deferral() error {
	cause := errors.New("the push was deferred to the branch's next push")
	if j.side == fabriclayout.RepoRecords && j.hasUncommittedChanges() {
		cause = fmt.Errorf("origin's commits cannot be replayed onto %s until its uncommitted changes are committed or discarded", j.path)
	}
	record := j.record(fabriclayout.PushFailureDeferred, cause, nil, nil, "")
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

// classifyFailure records the failed push verdict names and returns it as a *PushFailedError.
// The record's error is built from the error the verdict rests on.
func (j *pushJob) classifyFailure(snapshot fabriccore.Snapshot, verdict fabriccore.PushVerdict) error {
	var cause error
	var paths []string
	commits := verdict.Commits
	switch {
	case verdict.Outcome >= 0:
		cause = j.errOf(snapshot, verdict.Outcome)
		if verdict.Rejection >= 0 {
			cause = errors.Join(j.errOf(snapshot, verdict.Rejection), cause)
		}
		switch snapshot.Outcomes[verdict.Outcome].Class {
		case fabriccore.OutcomeReplayConflict, fabriccore.OutcomeDestructiveRefusal:
			var conflict *replayConflict
			if errors.As(cause, &conflict) {
				paths = conflict.paths
			}
			cause = fmt.Errorf("%w: %w", gitrepo.ErrPushRejected, cause)
		}
		switch {
		case verdict.Err != nil:
			cause = errors.Join(cause, verdict.Err)
		case verdict.NothingLacking:
			cause = fmt.Errorf("%w; a fetch found origin holding nothing this branch lacks", cause)
		}
	case verdict.Class == fabriclayout.PushFailureGone:
		cause = fmt.Errorf("%w: %s", ErrBranchGone, j.branch)
	case verdict.Class == fabriclayout.PushFailureRewound:
		cause = fmt.Errorf("%w: origin rewound %s to %s, dropping %s", errPushHeldBack, j.branch, verdict.OriginHead, joinOrNone(commits))
	case verdict.Class == fabriclayout.PushFailureTransport:
		cause = j.originErr
	default:
		cause = verdict.Err
	}
	if verdict.LocalCommits {
		commits = j.unpushedCommits()
	}
	return j.fail(verdict.Class, cause, paths, commits, verdict.OriginHead)
}

// failRejection reads what a rejection the retries did not settle is judged by: what a fresh fetch shows origin holding.
// Origin's tip is the branch's upstream, or origin's branch of the same name for a branch never given one.
// A failed fetch, or a tip that cannot be read because origin has no such branch, leaves origin unknown.
// A reset refused over a commit this machine did not make already names what origin holds, and fetches nothing.
func (j *pushJob) failRejection() fabriccore.PushJudgmentFacts {
	judgment := fabriccore.PushJudgmentFacts{Read: true, Unmade: j.unmade, UnmadeAt: j.unmadeAt}
	if j.unmade != "" {
		return judgment
	}
	if judgment.Err = j.repo.Fetch(); judgment.Err != nil {
		return judgment
	}
	originTip, err := j.repo.UpstreamSHA()
	if errors.Is(err, gitrepo.ErrNoUpstream) {
		originTip, err = j.repo.ResolveSHA("refs/remotes/" + originRemoteName + "/" + j.branch)
	}
	if err != nil {
		judgment.Err = err
		return judgment
	}
	head, err := j.repo.CurrentSHA()
	if err != nil {
		judgment.Err = err
		return judgment
	}
	originOnly, err := j.repo.CommitsNotIn(originTip, head)
	judgment.Err = err
	judgment.OriginHoldsMore = len(originOnly) > 0
	return judgment
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
	slug, ok := fabriclayout.WeftWarpSlug(filepath.Base(weftPath))
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

// checkOrigin looks the job's branch up on origin and fills snapshot's view of it.
// A gone branch's entry is judged ended or not, and a task branch origin still holds is checked for a rewind.
// That check reads the history of the tracking ref, so a pruned ref skips it.
// A lookup that fails for no reason of reaching origin says nothing of the branch, so the view reads unchecked.
// A lookup or judgment that fails in transit leaves the view unread and returns a *fieldReadError.
func (j *pushJob) checkOrigin(snapshot *fabriccore.Snapshot) error {
	snapshot.Push.Origin = fabriccore.PushOriginView{}
	view, err := j.observeOrigin()
	if err != nil && !gitexec.IsTransportFailure(err) {
		logger.Warn("fabricengine: origin's branch not looked up, the push goes on unchecked", "path", j.path, "branch", j.branch, "error", err)
		view, err = originView{unchecked: true}, nil
	}
	if err == nil && view.gone {
		view.ended, err = goneEntryEnded(j.hubPath, j.branch)
		if err != nil && !gitexec.IsTransportFailure(err) {
			snapshot.Push.Origin = fabriccore.PushOriginView{Read: true, Gone: true, JudgeErr: err}
			return nil
		}
	}
	if err != nil {
		j.originErr = err
		return &fieldReadError{Field: fabriccore.FieldPushSide, Store: fabriccore.Store(j.side), Err: err}
	}

	found := fabriccore.PushOriginView{Read: true, Head: view.head, Gone: view.gone, Ended: view.ended, Unchecked: view.unchecked}
	if j.side == fabriclayout.RepoCode && !j.prime && snapshot.Push.Upstream != fabriccore.PushUpstreamPruned && !view.gone && !view.unchecked {
		found.Rewound, found.Dropped, err = j.holdBackRewound(view.head)
		if err != nil {
			return err
		}
	}
	snapshot.Push.Origin = found
	return nil
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

// observeOrigin asks origin for the job's branch.
// A code side keeps origin's head, which the rewind check reads.
func (j *pushJob) observeOrigin() (originView, error) {
	if j.side == fabriclayout.RepoCode {
		head, found, err := originBranchHead(j.path, j.branch)
		return originView{head: head, gone: !found}, err
	}
	gone, err := branchGoneOnOrigin(j.path)
	return originView{gone: gone}, err
}

// holdBackRewound reports whether origin rewound a task branch to originHead, and the commits the rewind dropped.
// A branch whose origin head is a commit this machine lacks is not rewound.
func (j *pushJob) holdBackRewound(originHead string) (rewound bool, dropped []string, err error) {
	if _, resolveErr := j.repo.ResolveSHA(originHead + "^{commit}"); resolveErr != nil {
		return false, nil, nil
	}
	localTip, err := j.repo.CurrentSHA()
	if err != nil {
		return false, nil, err
	}
	trackingRef, err := gitexec.Run([]string{"rev-parse", "--symbolic-full-name", "@{u}"}, j.path)
	if err != nil {
		return false, nil, err
	}
	earlier, err := trackingReflog(j.path, strings.TrimSpace(trackingRef))
	if err != nil {
		return false, nil, err
	}
	kind, dropped, err := classifyRewrite(j.repo, localTip, originHead, earlier)
	if err != nil || kind != rewriteRewound {
		return false, nil, err
	}
	return true, dropped, nil
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
	boardDir := fabriclayout.BoltDir(hubPath)
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
// A non-empty ceiling pushes only up to it and replays nothing, since a replay would rewrite the held commit.
// An origin holding commits the branch lacks then returns errOriginAhead.
func retryWeftPush(rec *Mutations, hubPath, weftPath, runSlug string, here fabriclayout.MachineNames, ceiling string) (deferred bool, err error) {
	if stop := emptyMarkerStop(weftPath); stop != nil {
		return false, stop
	}
	repo := gitrepo.New(weftPath)
	if err := repo.Fetch(); err != nil {
		return false, fmt.Errorf("fabricengine: fetch the weft branch in %s: %w", weftPath, err)
	}
	var handedOver []string
	if !here.IsZero() {
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
	if ceiling != "" {
		behind, err := repo.HasUnpulled()
		if err != nil {
			return false, err
		}
		if behind {
			return false, errOriginAhead
		}
		job := &pushJob{path: weftPath, repo: repo, opts: SyncOptions{ceiling: ceiling}}
		if job.branch, err = repo.CurrentBranch(); err != nil {
			return false, err
		}
		return false, job.push()
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
		slug, ok := fabriclayout.WeftWarpSlug(filepath.Base(weftPath))
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
	boardDir := fabriclayout.BoltDir(hubPath)
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
	writeLockPath, err := BoltLockPath(boardDir, fabriclayout.BoardWriteLockFile)
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
