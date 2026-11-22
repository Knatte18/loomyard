// finalize_pr.go holds the landing's handling of the run's pull request.
// It has the decision made under the parent lock and the merge on GitHub before the parent-side merge.
// Its halt texts for a failed beam.yaml read, push or GitHub call are shared with PR-Review.

package landingshed

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/githubclient"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/pullrequest"
	"github.com/Knatte18/loomyard/internal/summaryparser"
)

// prMergeRetryWindow is how long retries are scheduled under the lock.
// It bounds the retries of a pending refusal.
// It also bounds those of a refusal whose merge-state reading says the pull request can merge.
const prMergeRetryWindow = pullrequest.MergeRetryWindow

// prLanding is what the decision about the run's pull request hands the landing.
// The zero value is a landing with no GitHub merge, which lands through the parent-side merge alone.
type prLanding struct {
	// merge marks a landing that merges the run's pull request on GitHub before the parent-side merge.
	// The pull request is opened first when `open` is set.
	merge bool
	// open marks a landing that finds or opens the pull request, records it, and merges it.
	open bool
	// atOnce marks a parent that takes its pull request with no review, which always squash-merges.
	atOnce bool
	// trueMerge marks a landing whose parent-side merge keeps the task branch's history.
	// The squash setting in landing.yaml does not change that.
	trueMerge bool
	// reopen marks a landing whose recorded pull request is closed, and is reopened just before it merges.
	reopen bool
	// dropBeamRecord marks the landing of a beam whose recorded pull request is spent.
	// The pull request is dropped from the beam's landing record once the parent-side landing completes.
	dropBeamRecord bool
	// api is the GitHub client for the repository the origin names, set when merge is.
	api pullrequest.API
	// record is the run's pull request record, set when merge is and open is not.
	record pullrequest.Record
}

// beamModeUnreadableReason is the Stuck reason of a failed lookup of beam.yaml's pull_request for a landing.
// err is the lookup's error.
func beamModeUnreadableReason(err error) string {
	return fmt.Sprintf("beam.yaml's pull_request for this landing could not be read: %v; way forward: fix the beam.yaml file the error names, with lym config beam --set pull_request=<on|off|merge|merge-in> run in that worktree, or for the hub's default file with lym config beam --layer default --set pull_request=<on|off|merge|merge-in> from the prime, then run lym loom resume; a run_settings change applies from the next run", err)
}

// pushHaltReason is the Stuck reason of a push of the task branch that failed for a reason other than a transient one.
func pushHaltReason(worktreeRoot string, err error) string {
	return fmt.Sprintf("pushing the task branch failed: %v; way forward: run lym fabric push in the task worktree %s, then run lym loom resume", err, worktreeRoot)
}

// tokenFailure reports whether err is a GitHub token that could not be resolved or that GitHub rejected.
// The HTTP client wraps both in an error that reads as transient.
// Callers therefore test for a token failure before they test for a transient one.
func tokenFailure(err error) bool {
	return errors.Is(err, githubclient.ErrTokenUnresolvable) || errors.Is(err, githubclient.ErrTokenRejected)
}

// githubHaltReason is the Stuck reason of a failed GitHub call named by action that is not transient.
// An unresolvable token names gh auth login;
// a rejected token, or a 401 or 403 response, names the token and repository access.
// A 422 says GitHub refused the request's content, and any other failure names GitHub's message.
func githubHaltReason(action string, err error) string {
	if errors.Is(err, githubclient.ErrTokenUnresolvable) {
		return fmt.Sprintf("no GitHub token is available to %s: %v; way forward: the operator runs gh auth login --hostname github.com, then lym loom resume", action, err)
	}
	status := githubclient.StatusCode(err)
	switch {
	case errors.Is(err, githubclient.ErrTokenRejected), status == http.StatusUnauthorized, status == http.StatusForbidden:
		return fmt.Sprintf("GitHub refused the token for the call to %s: %v; way forward: make sure the gh user's token (GH_TOKEN, GITHUB_TOKEN, or gh auth login --hostname github.com) is valid and can reach the repository, then run lym loom resume", action, err)
	case status == http.StatusUnprocessableEntity:
		return fmt.Sprintf("GitHub refused the call to %s as invalid (422): %v; a 422 means GitHub accepted the token but not the request, such as a pull request between branches with no commits between them; way forward: fix what GitHub's message names in the task or parent branch, then run lym loom resume", action, err)
	}
	return fmt.Sprintf("the GitHub call to %s failed: %v; way forward: fix what GitHub's message names, then run lym loom resume", action, err)
}

// noDriftVerdict stands in a prompt for the regression check when none ran for the task.
const noDriftVerdict = "No regression check ran for this task."

// driftVerdict returns the task's regression check section, read through the DriftVerdict seam.
// It is the empty string when none ran or when the seam is nil.
func (d Deps) driftVerdict() (string, error) {
	if d.DriftVerdict == nil {
		return "", nil
	}
	verdict, err := d.DriftVerdict()
	if err != nil {
		return "", fmt.Errorf("read the regression check: %w", err)
	}
	return verdict, nil
}

// withSection appends section to body after a blank line, with body's trailing whitespace trimmed.
// It returns section alone when body is blank, and body unchanged when section is empty.
func withSection(body, section string) string {
	if section == "" {
		return body
	}
	body = strings.TrimRight(body, " \t\r\n")
	if strings.TrimSpace(body) == "" {
		return section
	}
	return body + "\n\n" + section
}

// bodyWithCredit composes the text of a pull request's body and of its merge's commit message alike.
// It is the trimmed body, then a blank line and the credit block when the block is not empty;
// with an empty body, it is the credit block alone.
// The title is passed separately.
func bodyWithCredit(body, credit string) string {
	body = strings.TrimSpace(body)
	if credit == "" {
		return body
	}
	if body == "" {
		return credit
	}
	return body + "\n\n" + credit
}

// completeSummary appends the regression check to the body of summary, a parsed change description.
// It returns the credit block composed from the engines the task's sessions ran under.
// Together they make a landing's pull request text and its landing commit's message.
func (d Deps) completeSummary(summary *summaryparser.Summary) (string, error) {
	var enginesRun []string
	if d.EnginesRun != nil {
		var err error
		if enginesRun, err = d.EnginesRun(); err != nil {
			return "", fmt.Errorf("read the engines the task ran under: %w", err)
		}
	}
	verdict, err := d.driftVerdict()
	if err != nil {
		return "", err
	}
	summary.Body = withSection(summary.Body, verdict)
	return d.Config.CreditBlock(enginesRun), nil
}

// githubFailure judges producer's failed GitHub call named by action, and returns how the call ends on it.
// A token failure halts first, since the HTTP client wraps it in an error that reads as transient;
// a transient failure is returned as an error for the driver to re-step, and any other failure halts.
// Each halt reason opens with haltPrefix.
func githubFailure(ctx context.Context, producer, haltPrefix, action string, err error) *ending {
	if !tokenFailure(err) {
		if terr := transientFailure(ctx, producer, action, err); terr != nil {
			return &ending{err: terr}
		}
	}
	return halt(ctx, producer, haltPrefix+githubHaltReason(action, err))
}

// githubFailure judges Finalize's failed GitHub call named by action; each halt opens with LandingStopPrefix.
func (fz *Finalize) githubFailure(ctx context.Context, action string, err error) *ending {
	return githubFailure(ctx, finalizeName, LandingStopPrefix, action, err)
}

// pushTask pushes the task branch for producer, and returns nil once the push has finished.
// A transient failure is an error the driver re-steps;
// any other failure halts with haltPrefix and the push's way forward.
func pushTask(ctx context.Context, deps Deps, producer, haltPrefix string) *ending {
	err := deps.PullRequest.PushTask()
	if err == nil {
		return nil
	}
	if terr := transientFailure(ctx, producer, "push the task branch", err); terr != nil {
		return &ending{err: terr}
	}
	return halt(ctx, producer, haltPrefix+pushHaltReason(deps.WorktreeRoot, err))
}

// pullRequestStatus opens the GitHub client for originURL through pr and reads the status of pull request number.
// A failure returns the failed call's action beside its error.
func pullRequestStatus(ctx context.Context, pr PullRequestDeps, originURL string, number int) (pullrequest.API, pullrequest.Status, string, error) {
	api, err := pr.API(originURL)
	if err != nil {
		return nil, pullrequest.Status{}, "open the GitHub client", err
	}
	status, err := api.Status(ctx, number)
	if err != nil {
		return nil, pullrequest.Status{}, "read the pull request's status", err
	}
	return api, status, "", nil
}

// closeIfOpen closes pull request number, with reason in its closing comment, when GitHub reports it open.
// A failure returns the failed call's action beside its error.
func closeIfOpen(ctx context.Context, pr PullRequestDeps, originURL string, number int, reason string) (string, error) {
	api, status, action, err := pullRequestStatus(ctx, pr, originURL, number)
	if err != nil || status.State != pullrequest.StateOpen {
		return action, err
	}
	if err := api.Close(ctx, number, "lym closed this pull request: "+reason); err != nil {
		return "close the pull request", err
	}
	return "", nil
}

// findOrOpenPullRequest finds the open pull request from task into parent, or opens one carrying text.
// It reports whether it found one; a failure returns the failed call's action beside its error.
func findOrOpenPullRequest(ctx context.Context, api pullrequest.API, task, parent string, text prText) (pullrequest.Ref, bool, string, error) {
	ref, found, err := api.FindOpen(ctx, task, parent)
	if err != nil {
		return pullrequest.Ref{}, false, "look for an open pull request", err
	}
	if found {
		return ref, true, "", nil
	}
	if ref, err = api.Create(ctx, task, parent, text.title, text.body); err != nil {
		return pullrequest.Ref{}, false, "open the pull request", err
	}
	return ref, false, "", nil
}

// commitPRRecord commits and pushes the pull request record's last change through pr's told seam, for producer.
func commitPRRecord(pr PullRequestDeps, producer string) error {
	if err := pr.CommitRecord(); err != nil {
		return fmt.Errorf("landingshed: %s: commit the pull request record: %w", producer, err)
	}
	return nil
}

// removePRRecord removes the run's pull request record and commits the removal, for producer.
func removePRRecord(pr PullRequestDeps, producer string) error {
	if err := pullrequest.RemoveRecord(pr.RecordPath, pr.RecordLockPath); err != nil {
		return fmt.Errorf("landingshed: %s: %w", producer, err)
	}
	return commitPRRecord(pr, producer)
}

// pullRequestLanding decides, under the parent lock, how the landing relates to the run's pull request.
// The decision reads beam.yaml's pull_request for the landing and GitHub's rules for the parent branch.
// It never reads the record alone.
// A no closes a recorded pull request that GitHub reports open, removes the record, and lands locally.
// A yes halts without the recorded pull request, or with a closed one the operator did not end the review of.
// A yes marked MergeAtOnce has no review to end.
// With no record, it marks the landing to open the pull request.
// With a closed one, it marks the landing to reopen it.
// So nothing is opened or reopened on GitHub before the owner checks.
// A merged pull request lands through the parent-side merge's already-landed path;
// an open one is marked to merge on GitHub;
// a closed one whose review the operator ended is reopened and marked to merge, since GitHub merges only open ones.
// Without the Mode seam, the step is skipped and the landing is local.
// A beam's landing reads the beam's landing record beside the run's record.
// In a beam with no pull request recorded, an open pull request in the run's record is merged as it stands;
// the parent rule is not consulted for it, and a closed or merged one is decided as any other record is.
// A non-nil ending ends the call at once.
func (fz *Finalize) pullRequestLanding(ctx context.Context) (prLanding, *ending) {
	pr := fz.deps.PullRequest
	if pr.Mode == nil {
		return prLanding{}, nil
	}
	failed := func(what string, err error) (prLanding, *ending) {
		return prLanding{}, fail(ctx, finalizeName, fmt.Errorf("landingshed: %s: %s: %w", finalizeName, what, err))
	}

	mode, err := pr.Mode()
	if err != nil {
		return prLanding{}, halt(ctx, finalizeName, LandingStopPrefix+beamModeUnreadableReason(err))
	}
	origin, err := pr.Origin()
	if err != nil {
		return failed("read the origin", err)
	}
	hasCommits, err := pr.TaskHasCommits()
	if err != nil {
		return failed("read whether the task branch has commits beyond its parent", err)
	}
	rec, found, err := pullrequest.ReadRecord(pr.RecordPath, pr.RecordLockPath)
	if err != nil {
		return failed("read the pull request record", err)
	}
	beam := NotABeam
	var beamPullRequest BeamPullRequest
	if pr.BeamRecord != nil {
		recorded := false
		if beamPullRequest, recorded, err = pr.BeamRecord(); err != nil {
			return failed("read the beam landing record", err)
		}
		beam = BeamWithoutPullRequest
		if recorded {
			beam = BeamWithPullRequest
		}
	}

	if beam == BeamWithoutPullRequest && found {
		// In a beam with no pull request recorded, an open pull request in the run's record is merged as it stands;
		// it was opened before a halt, or left by PR-Review, and the parent rule is not consulted for it.
		// A closed or merged one is decided below as any other record is.
		api, status, action, err := pullRequestStatus(ctx, pr, origin.URL, rec.Number)
		if err != nil {
			return prLanding{}, fz.githubFailure(ctx, action, err)
		}
		if status.State == pullrequest.StateOpen {
			return prLanding{merge: true, atOnce: true, api: api, record: rec}, nil
		}
	}
	decision, err := resolveWithParentRule(ctx, finalizeName, fz.deps, mode, beam, origin, hasCommits, found)
	if err != nil {
		return prLanding{}, fail(ctx, finalizeName, err)
	}
	if decision.Halt != "" {
		return prLanding{}, halt(ctx, finalizeName, LandingStopPrefix+decision.Halt)
	}
	if beam == BeamWithPullRequest {
		return fz.recordedBeamLanding(ctx, mode, beamPullRequest, origin, hasCommits, decision, rec, found)
	}
	landing, end := fz.landingForDecision(ctx, decision, origin, rec, found)
	landing.trueMerge = decision.TrueMerge
	return landing, end
}

// landingForDecision turns the parent rule's yes or no into the landing, against the pull request record rec.
// found reports whether that record exists.
// A no closes a recorded pull request GitHub reports open and lands locally.
// A yes needs the recorded pull request, unless it is marked MergeAtOnce;
// a MergeAtOnce yes has the pull request opened, or reopened if closed, when it merges.
// A non-nil ending ends the call at once.
func (fz *Finalize) landingForDecision(ctx context.Context, decision PullRequestDecision, origin Origin, rec pullrequest.Record, found bool) (prLanding, *ending) {
	pr := fz.deps.PullRequest
	if !decision.Open {
		if !found {
			return prLanding{}, nil
		}
		return prLanding{}, fz.dropRecordedPullRequest(ctx, origin, rec, decision.Reason)
	}
	if !found && !decision.MergeAtOnce {
		return prLanding{}, halt(ctx, finalizeName, LandingStopPrefix+fmt.Sprintf("this landing goes through a pull request (%s) but the run has no pull request recorded; way forward: lym loom goto PR-Review, which opens it", decision.Reason))
	}

	api, err := pr.API(origin.URL)
	if err != nil {
		return prLanding{}, fz.githubFailure(ctx, "open the GitHub client", err)
	}
	if !found {
		return prLanding{merge: true, open: true, atOnce: true, api: api}, nil
	}
	status, err := api.Status(ctx, rec.Number)
	if err != nil {
		return prLanding{}, fz.githubFailure(ctx, "read the pull request's status", err)
	}
	switch status.State {
	case pullrequest.StateOpen:
		return prLanding{merge: true, atOnce: decision.MergeAtOnce, api: api, record: rec}, nil
	case pullrequest.StateClosed:
		if !rec.ReviewEnded && !decision.MergeAtOnce {
			return prLanding{}, halt(ctx, finalizeName, LandingStopPrefix+fmt.Sprintf("the run's pull request %s was closed without a merge; way forward: reopen it on GitHub, then lym loom goto PR-Review, or lym batten cancel %s", rec.URL, fz.deps.Slug))
		}
		if decision.MergeAtOnce {
			return prLanding{merge: true, atOnce: true, reopen: true, api: api, record: rec}, nil
		}
		if err := api.Reopen(ctx, rec.Number); err != nil {
			return prLanding{}, fz.githubFailure(ctx, "reopen the pull request", err)
		}
		return prLanding{merge: true, api: api, record: rec}, nil
	}
	// A merged pull request has no GitHub merge left, yet the parent-side merge still follows the parent's mode.
	return prLanding{atOnce: decision.MergeAtOnce}, nil
}

// recordedBeamLanding decides the landing of a beam whose landing record holds a pull request.
// Where the decision says the landing goes through a pull request, the recorded one is handled as follows:
// an open one is merged on GitHub, squashed, with its title and body left as they are;
// a closed one halts for the operator to reopen it on GitHub.
// A merged one lands through the already-landed path when it merged at the task head or has every task commit;
// one that lacks a task commit is decided as with none recorded.
// Otherwise the landing is local.
// Every outcome but a halt marks the record to be dropped once the landing completed.
// A non-nil ending ends the call at once.
func (fz *Finalize) recordedBeamLanding(ctx context.Context, mode string, recorded BeamPullRequest, origin Origin, hasCommits bool, decision PullRequestDecision, rec pullrequest.Record, found bool) (prLanding, *ending) {
	pr := fz.deps.PullRequest
	if !decision.Open {
		return prLanding{dropBeamRecord: true}, nil
	}
	api, status, action, err := pullRequestStatus(ctx, pr, origin.URL, recorded.Number)
	if err != nil {
		return prLanding{}, fz.githubFailure(ctx, action, err)
	}
	switch status.State {
	case pullrequest.StateOpen:
		beamRecord := pullrequest.Record{Number: recorded.Number, URL: recorded.URL}
		return prLanding{merge: true, atOnce: true, dropBeamRecord: true, api: api, record: beamRecord}, nil
	case pullrequest.StateClosed:
		return prLanding{}, halt(ctx, finalizeName, LandingStopPrefix+fmt.Sprintf("the beam's pull request %s was closed without a merge; way forward: the operator reopens it on GitHub, then runs lym loom resume", recorded.URL))
	}

	head, err := fz.deps.TaskHead()
	if err != nil {
		return prLanding{}, fail(ctx, finalizeName, fmt.Errorf("landingshed: %s: read the task head: %w", finalizeName, err))
	}
	if status.HeadSHA != head {
		missing, err := pr.CommitsMissing(status.HeadSHA, head)
		if err != nil {
			return prLanding{}, fail(ctx, finalizeName, fmt.Errorf("landingshed: %s: list the task commits the merged pull request lacks: %w", finalizeName, err))
		}
		if len(missing) > 0 {
			again, err := resolveWithParentRule(ctx, finalizeName, fz.deps, mode, BeamWithoutPullRequest, origin, hasCommits, found)
			if err != nil {
				return prLanding{}, fail(ctx, finalizeName, err)
			}
			if again.Halt != "" {
				return prLanding{}, halt(ctx, finalizeName, LandingStopPrefix+again.Halt)
			}
			landing, end := fz.landingForDecision(ctx, again, origin, rec, found)
			landing.dropBeamRecord = true
			return landing, end
		}
	}
	return prLanding{atOnce: true, dropBeamRecord: true}, nil
}

// dropBeamRecord drops the spent pull request from the beam's landing record once the landing completed.
// A failed drop is logged only, since the landing has already happened.
func (fz *Finalize) dropBeamRecord() {
	if err := fz.deps.PullRequest.DropBeamRecord(); err != nil {
		logger.Warn("landingshed: the landed pull request could not be dropped from the beam's landing record", "row", finalizeName, "cause", err)
	}
}

// dropRecordedPullRequest ends the run's pull request when the landing no longer goes through one.
// It closes a recorded pull request that GitHub reports open, with reason as its closing comment;
// then it removes the record, and lands locally.
func (fz *Finalize) dropRecordedPullRequest(ctx context.Context, origin Origin, rec pullrequest.Record, reason string) *ending {
	if action, err := closeIfOpen(ctx, fz.deps.PullRequest, origin.URL, rec.Number, reason); err != nil {
		return fz.githubFailure(ctx, action, err)
	}
	if err := removePRRecord(fz.deps.PullRequest, finalizeName); err != nil {
		return fail(ctx, finalizeName, err)
	}
	return nil
}

// pullRequestCommitTitle is the commit title of a pull request's merge: the title followed by ` (#<number>)`.
// That is the form GitHub's own squash merge writes, so the landed commit names the pull request it came from.
func pullRequestCommitTitle(title string, number int) string {
	return fmt.Sprintf("%s (#%d)", title, number)
}

// mergeOnGitHub merges the run's pull request on GitHub, after the lock re-check and before the parent-side merge.
// It pushes the task branch first.
// It reopens the pull request when marked to reopen it, and finds or opens it when marked to open it.
// It merges with the pushed head as the expected head.
// The merge squashes when landing.yaml's squash is set;
// it always squashes for a parent that takes its pull request with no review.
// The commit title is the title with the pull request's number in parentheses;
// the commit message is the message passed in.
// A merge that GitHub refuses as pending is retried for up to `prMergeRetryWindow`, on `PullRequest.Now`'s clock.
// Any other refusal but a method one has GitHub's merge state read, waiting out a mergeability not yet computed;
// the merge is retried while that state reports the pull request can merge.
// A retry is scheduled only while the window has not passed.
// Each merge-state reading can wait up to a further window.
// A scheduled retry sleeps its backoff first.
// So Finalize halts within about two and a half windows.
// It then fetches the parent's upstream and fast-forwards the parent pair to it,
// so the parent-side merge that follows finds the parent holding the task's work.
// It returns nil when the landing goes on; each halt opens with LandingStopPrefix.
func (fz *Finalize) mergeOnGitHub(ctx context.Context, parentHandle parentMerger, landing prLanding, title, message string) *ending {
	pr := fz.deps.PullRequest
	if end := pushTask(ctx, fz.deps, finalizeName, LandingStopPrefix); end != nil {
		return end
	}
	head, err := fz.deps.TaskHead()
	if err != nil {
		return fail(ctx, finalizeName, fmt.Errorf("landingshed: %s: read the task head: %w", finalizeName, err))
	}
	if landing.reopen {
		if err := landing.api.Reopen(ctx, landing.record.Number); err != nil {
			return fz.githubFailure(ctx, "reopen the pull request", err)
		}
	}
	if landing.open {
		rec, end := fz.openAtOnce(ctx, landing.api, head, title, message)
		if end != nil {
			return end
		}
		landing.record = rec
	}

	squash := fz.deps.Config.Squash || landing.atOnce
	deadline := pr.Now().Add(prMergeRetryWindow)
	wait := prWaitFloor
	for {
		err := landing.api.Merge(ctx, landing.record.Number, head, squash, pullRequestCommitTitle(title, landing.record.Number), message)
		if err == nil {
			break
		}
		var refusal *pullrequest.MergeRefusal
		if !errors.As(err, &refusal) {
			return fz.githubFailure(ctx, "merge the pull request", err)
		}
		retry := refusal.Kind == pullrequest.RefusalPending
		var state *pullrequest.MergeState
		if refusal.Kind == pullrequest.RefusalOther {
			// GitHub refuses a merge as not mergeable while it has not yet computed mergeability,
			// so its own reading of the pull request decides whether the refusal lasts.
			read, err := pullrequest.ReadMergeState(ctx, landing.api, landing.record.Number, head, fz.deps.ParentBranch, pr.Now, pr.Sleep)
			if err != nil {
				return fz.githubFailure(ctx, "read whether the pull request can merge", err)
			}
			state = &read
			retry = read.MergeableNow
		}
		if retry && pr.Now().Before(deadline) {
			if err := pr.Sleep(ctx, wait); err != nil {
				return fail(ctx, finalizeName, fmt.Errorf("landingshed: %s: sleep before retrying the merge: %w", finalizeName, err))
			}
			wait = nextPRWait(wait, false)
			continue
		}
		return halt(ctx, finalizeName, LandingStopPrefix+fz.mergeRefusalReason(landing, refusal, state))
	}

	if _, err := fz.deps.ReadParentTips(true); err != nil {
		return halt(ctx, finalizeName, LandingStopPrefix+fz.mergedButNotPulledReason(landing.record, fmt.Sprintf("the parent's upstream could not be fetched: %v", err)))
	}
	result, err := parentHandle.FastForwardPair(fabricengine.SyncOptions{SkipPush: fz.deps.PushSkipped})
	if err != nil {
		return halt(ctx, finalizeName, LandingStopPrefix+fz.mergedButNotPulledReason(landing.record, fmt.Sprintf("the parent pair could not be fast-forwarded: %v", err)))
	}
	if result.Refusal != "" {
		return halt(ctx, finalizeName, LandingStopPrefix+fz.mergedButNotPulledReason(landing.record, fmt.Sprintf("the parent pair refused the fast-forward: %s", result.Refusal)))
	}
	return nil
}

// openAtOnce finds the open pull request from the task branch into the parent, or opens one with the title and body.
// It sets the title and body of a pull request it finds, unless PullRequest.BeamRecord is set.
// It is for a parent that takes its pull request with no review.
// It records the pull request with head as its gated head and the review ended,
// so a re-entry merges the recorded one and reopens it when it was closed.
// A non-nil ending ends the call at once: a GitHub failure is either a transient error or a halt.
func (fz *Finalize) openAtOnce(ctx context.Context, api pullrequest.API, head, title, body string) (pullrequest.Record, *ending) {
	ref, found, action, err := findOrOpenPullRequest(ctx, api, fz.deps.TaskBranch, fz.deps.ParentBranch, prText{title: title, body: body})
	if err != nil {
		return pullrequest.Record{}, fz.githubFailure(ctx, action, err)
	}
	if found && fz.deps.PullRequest.BeamRecord == nil {
		if err := api.Edit(ctx, ref.Number, title, body); err != nil {
			return pullrequest.Record{}, fz.githubFailure(ctx, "set the pull request's title and body", err)
		}
	}

	rec := pullrequest.Record{
		Number:      ref.Number,
		URL:         ref.URL,
		Base:        fz.deps.ParentBranch,
		Head:        fz.deps.TaskBranch,
		GatedHead:   head,
		ReviewEnded: true,
	}
	pr := fz.deps.PullRequest
	if err := pullrequest.WriteRecord(pr.RecordPath, pr.RecordLockPath, rec); err != nil {
		return pullrequest.Record{}, fail(ctx, finalizeName, fmt.Errorf("landingshed: %s: record the pull request: %w", finalizeName, err))
	}
	if err := commitPRRecord(pr, finalizeName); err != nil {
		return pullrequest.Record{}, fail(ctx, finalizeName, err)
	}
	return rec, nil
}

// mergeRefusalReason is the Stuck reason for a merge GitHub refused for good.
// It also covers a merge GitHub kept refusing as pending past the retry window.
// A method refusal names the squash setting, or the repository's settings for a parent that always squashes.
// A beam's refusal never names a cancel.
// It names GitHub's message, the pull request's status verb and the resume.
// The exception is a refusal outlasting the window while GitHub reports the pull request can merge.
// That one names the resume alone.
// state is GitHub's merge state read after the refusal, nil when none was read;
// each cause it names follows GitHub's message.
// A state that can merge names the resume alone;
// a state whose only cause is a mergeability GitHub has not computed names waiting for it, with no approval or cancel.
func (fz *Finalize) mergeRefusalReason(landing prLanding, refusal *pullrequest.MergeRefusal, state *pullrequest.MergeState) string {
	rec := landing.record
	reported := ""
	if state != nil {
		if state.MergeableNow {
			return fmt.Sprintf("GitHub refused to merge pull request %s: %s, yet GitHub now reports it can merge; way forward: run lym loom resume, which retries the merge", rec.URL, refusal.Message)
		}
		reported = "; GitHub reports: " + strings.Join(state.Causes, ", ")
		if state.MergeabilityUncomputed() {
			wait := "wait until GitHub computes whether the pull request can merge"
			if fz.deps.PullRequest.BeamRecord != nil {
				wait += fmt.Sprintf(", which lym beam pr status %s shows", fz.deps.Slug)
			}
			return fmt.Sprintf("GitHub refused to merge pull request %s: %s%s; way forward: %s, then run lym loom resume", rec.URL, refusal.Message, reported, wait)
		}
	}
	if fz.deps.PullRequest.BeamRecord != nil {
		check := fmt.Sprintf("check the pull request with lym beam pr status %s, then run lym loom resume", fz.deps.Slug)
		if refusal.Kind == pullrequest.RefusalMethod {
			return fmt.Sprintf("GitHub refused the squash merge of pull request %s: %s; way forward: allow squash merging in the repository's settings on GitHub, since a beam always lands by squash, then %s", rec.URL, refusal.Message, check)
		}
		return fmt.Sprintf("GitHub refused to merge pull request %s: %s%s; way forward: settle on GitHub what the message names, then %s", rec.URL, refusal.Message, reported, check)
	}
	if refusal.Kind == pullrequest.RefusalMethod && landing.atOnce {
		return fmt.Sprintf("GitHub refused the squash merge of pull request %s: %s; way forward: allow squash merging in the repository's settings on GitHub, since a landing whose beam.yaml pull_request is merge always squashes, then run lym loom resume", rec.URL, refusal.Message)
	}
	if refusal.Kind == pullrequest.RefusalMethod {
		return fmt.Sprintf("GitHub refused the merge method for pull request %s: %s; way forward: set landing.yaml's squash to the method the repository allows with lym config landing --set squash=<true|false> in the prime worktree, or allow the method in the repository's settings, then run lym loom resume", rec.URL, refusal.Message)
	}
	return fmt.Sprintf("GitHub refused to merge pull request %s: %s%s; way forward: a second person approves the pull request again or settles the required check or conflict on GitHub, then runs lym loom resume; or the operator runs lym batten cancel %s", rec.URL, refusal.Message, reported, fz.deps.Slug)
}

// mergedButNotPulledReason is the Stuck reason for a pull request GitHub merged that the parent pair could not take in.
// The re-entry finds the pull request merged and lands through the already-landed path.
func (fz *Finalize) mergedButNotPulledReason(rec pullrequest.Record, cause string) string {
	return fmt.Sprintf("pull request %s was merged on GitHub, but %s; way forward: fix that, then run lym loom resume, which finds the pull request merged and lands through the parent-side merge", rec.URL, cause)
}
