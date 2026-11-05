// finalize_pr.go holds Finalize's landing through the run's pull request: the decision it makes under the parent lock, the merge on GitHub before the parent-side merge, and the halt texts a failed beam.yaml read, push or GitHub call share with PR-Review.

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
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// prMergeRetryWindow is how long a merge GitHub refuses as pending is retried under the lock.
const prMergeRetryWindow = pullrequest.MergeRetryWindow

// prLanding is what Finalize's decision about the run's pull request leaves for the landing.
// The zero value is a landing with no GitHub merge, which lands through the parent-side merge alone.
type prLanding struct {
	// merge marks a landing that merges the run's pull request on GitHub before the parent-side merge, opening it first when open is set.
	merge bool
	// open marks a landing whose pull request Finalize finds or opens and records itself, just before it merges it.
	open bool
	// atOnce marks a parent that takes its pull request with no review, which always squash-merges.
	atOnce bool
	// reopen marks a landing whose closed recorded pull request mergeOnGitHub reopens just before it merges.
	reopen bool
	// dropBeamRecord marks the landing of a beam whose recorded pull request is spent: Finalize drops it from the beam's landing record once the parent-side landing completed.
	dropBeamRecord bool
	// api calls GitHub for the repository the origin names, set when merge is.
	api pullrequest.API
	// record is the run's pull request record, set when merge is and open is not.
	record pullrequest.Record
}

// beamModeUnreadableReason is the Stuck reason of a receiving branch's beam.yaml that could not be read in worktree.
func beamModeUnreadableReason(worktree string, err error) string {
	return fmt.Sprintf("the receiving branch's beam.yaml could not be read in worktree %s: %v; way forward: fix it with lym config beam --set pull_request=<on|off|required|merge> in that worktree, then run lym loom resume", worktree, err)
}

// pushHaltReason is the Stuck reason of a push of the task branch that failed for a reason other than a transient one.
func pushHaltReason(worktreeRoot string, err error) string {
	return fmt.Sprintf("pushing the task branch failed: %v; way forward: run lym fabric push in the task worktree %s, then run lym loom resume", err, worktreeRoot)
}

// tokenFailure reports whether err is a GitHub token that could not be resolved or that GitHub rejected.
// The HTTP client wraps both in an error the transient classifier reads as transient, so callers check this first.
func tokenFailure(err error) bool {
	return errors.Is(err, githubclient.ErrTokenUnresolvable) || errors.Is(err, githubclient.ErrTokenRejected)
}

// githubHaltReason is the Stuck reason of a failed GitHub call named by action that is not transient.
// Only an authentication failure names the token: an unresolvable token names gh auth login, and a rejected token or a 401 or 403 response names the token and repository access.
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

// creditBlock reads the engines the task's sessions ran under and returns the credit block Config composes from them.
func (d Deps) creditBlock() (string, error) {
	var enginesRun []string
	if d.EnginesRun != nil {
		var err error
		if enginesRun, err = d.EnginesRun(); err != nil {
			return "", fmt.Errorf("read the engines the task ran under: %w", err)
		}
	}
	return d.Config.CreditBlock(enginesRun), nil
}

// noDriftVerdict stands in a prompt for the regression check when none ran for the task.
const noDriftVerdict = "No regression check ran for this task."

// driftVerdict reads the task's regression check section, "" when none ran or no seam is told.
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

// withSection returns body with section after a blank line, and body unchanged when section is empty.
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

// bodyWithCredit composes a pull request's body and its merge's commit message alike: the change description's trimmed body, then a blank line and the credit block when the block is not empty.
// The title travels separately.
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

// githubFailure judges a failed GitHub call named by action, which is the whole of Finalize's disposition of one.
// A token failure halts first, since the HTTP client wraps it in a type the transient classifier reads as transient; then a transient failure is an error the driver re-steps; any other failure halts.
// Each halt opens with LandingStopPrefix.
func (fz *Finalize) githubFailure(ctx context.Context, action string, err error) (shedengine.Outcome, shedengine.OutputPointer, error, bool) {
	if !tokenFailure(err) {
		if terr := transientFailure(ctx, finalizeName, action, err); terr != nil {
			return "", shedengine.OutputPointer{}, terr, true
		}
	}
	return fz.halt(ctx, LandingStopPrefix+githubHaltReason(action, err))
}

// pullRequestLanding decides, under the parent lock, how the landing relates to the run's pull request, following ResolvePullRequest, asked with GitHub's rules for the parent branch, and never the record alone.
// A no closes a recorded pull request GitHub reports open, removes the record and lands locally.
// A yes needs the recorded pull request: an absent record, and a closed pull request the operator did not end the review with, are halts.
// The exception is a yes marked MergeAtOnce, which has no review to end: with no record it is marked for mergeOnGitHub to open the pull request itself, and a closed recorded pull request is marked for mergeOnGitHub to reopen, so nothing is opened or reopened on GitHub before the owner checks.
// A yes never lands by a local merge pushed to the parent: a merged pull request lands through the parent-side merge's already-landed path, an open one is marked to merge on GitHub, and a closed one whose record carries the operator's ending is reopened and marked to merge, since GitHub merges only an open pull request.
// A Finalize with no ParentMode seam skips the step and lands locally.
// A beam's landing, whose PullRequest.BeamRecord is set, never reads the entry's field and reads the beam's landing record beside the run's own pull request record.
// A pull request the run's own record holds, which GitHub reports open, with none in the beam's is merged as it stands whatever the resolver says; a closed or merged one is decided as any other record is.
// stop is true when Call returns the other values at once.
func (fz *Finalize) pullRequestLanding(ctx context.Context) (prLanding, shedengine.Outcome, shedengine.OutputPointer, error, bool) {
	pr := fz.deps.PullRequest
	if pr.ParentMode == nil {
		return prLanding{}, "", shedengine.OutputPointer{}, nil, false
	}
	ended := func(outcome shedengine.Outcome, out shedengine.OutputPointer, err error, _ bool) (prLanding, shedengine.Outcome, shedengine.OutputPointer, error, bool) {
		return prLanding{}, outcome, out, err, true
	}
	halt := func(reason string) (prLanding, shedengine.Outcome, shedengine.OutputPointer, error, bool) {
		return ended(fz.halt(ctx, LandingStopPrefix+reason))
	}

	mode, worktree, err := pr.ParentMode()
	if err != nil {
		return halt(beamModeUnreadableReason(worktree, err))
	}
	field := ""
	if pr.BeamRecord == nil {
		if field, err = pr.EntryField(); err != nil {
			return ended(fz.errorStop(ctx, fmt.Errorf("landingshed: %s: read the landing entry's pull_request: %w", finalizeName, err)))
		}
	}
	origin, err := pr.Origin()
	if err != nil {
		return ended(fz.errorStop(ctx, fmt.Errorf("landingshed: %s: read the origin: %w", finalizeName, err)))
	}
	hasCommits, err := pr.TaskHasCommits()
	if err != nil {
		return ended(fz.errorStop(ctx, fmt.Errorf("landingshed: %s: read whether the task branch has commits beyond its parent: %w", finalizeName, err)))
	}
	rec, found, err := pullrequest.ReadRecord(pr.RecordPath, pr.RecordLockPath)
	if err != nil {
		return ended(fz.errorStop(ctx, fmt.Errorf("landingshed: %s: read the pull request record: %w", finalizeName, err)))
	}
	beam := NotABeam
	var beamPullRequest BeamPullRequest
	if pr.BeamRecord != nil {
		recorded := false
		if beamPullRequest, recorded, err = pr.BeamRecord(); err != nil {
			return ended(fz.errorStop(ctx, fmt.Errorf("landingshed: %s: read the beam landing record: %w", finalizeName, err)))
		}
		beam = BeamWithoutPullRequest
		if recorded {
			beam = BeamWithPullRequest
		}
	}

	if beam == BeamWithoutPullRequest && found {
		// A pull request Finalize opened before a halt, or one an older PR-Review left, is merged as it stands whatever the resolver says, while GitHub reports it open.
		// A closed or merged one is decided below as any other record is.
		api, err := pr.API(origin.URL)
		if err != nil {
			return prStopped(fz.githubFailure(ctx, "open the GitHub client", err))
		}
		status, err := api.Status(ctx, rec.Number)
		if err != nil {
			return prStopped(fz.githubFailure(ctx, "read the pull request's status", err))
		}
		if status.State == pullrequest.StateOpen {
			return prLanding{merge: true, atOnce: true, api: api, record: rec}, "", shedengine.OutputPointer{}, nil, false
		}
	}
	decision, err := resolveWithParentRule(ctx, finalizeName, fz.deps, mode, field, beam, origin, hasCommits, found)
	if err != nil {
		return ended(fz.errorStop(ctx, err))
	}
	if decision.Halt != "" {
		return halt(decision.Halt)
	}
	if beam == BeamWithPullRequest {
		return fz.recordedBeamLanding(ctx, mode, beamPullRequest, origin, hasCommits, decision, rec, found)
	}
	return fz.landingForDecision(ctx, decision, origin, rec, found)
}

// prStopped adapts a Finalize stop, which carries no landing, to the stop return of pullRequestLanding.
func prStopped(outcome shedengine.Outcome, out shedengine.OutputPointer, err error, _ bool) (prLanding, shedengine.Outcome, shedengine.OutputPointer, error, bool) {
	return prLanding{}, outcome, out, err, true
}

// landingForDecision turns a yes or no of the resolver into the landing, against the run's own pull request record rec, which found reports present.
// A no closes a recorded pull request GitHub reports open and lands locally; a yes needs the recorded pull request, bar a yes marked MergeAtOnce, which opens or reopens the pull request itself.
// stop is true when the other values are what Call returns.
func (fz *Finalize) landingForDecision(ctx context.Context, decision PullRequestDecision, origin Origin, rec pullrequest.Record, found bool) (prLanding, shedengine.Outcome, shedengine.OutputPointer, error, bool) {
	pr := fz.deps.PullRequest
	if !decision.Open {
		if !found {
			return prLanding{}, "", shedengine.OutputPointer{}, nil, false
		}
		return fz.dropRecordedPullRequest(ctx, origin, rec, decision.Reason)
	}
	if !found && !decision.MergeAtOnce {
		return prStopped(fz.halt(ctx, LandingStopPrefix+fmt.Sprintf("this landing goes through a pull request (%s) but the run has no pull request recorded; way forward: lym loom goto PR-Review, which opens it", decision.Reason)))
	}

	api, err := pr.API(origin.URL)
	if err != nil {
		return prStopped(fz.githubFailure(ctx, "open the GitHub client", err))
	}
	if !found {
		return prLanding{merge: true, open: true, atOnce: true, api: api}, "", shedengine.OutputPointer{}, nil, false
	}
	status, err := api.Status(ctx, rec.Number)
	if err != nil {
		return prStopped(fz.githubFailure(ctx, "read the pull request's status", err))
	}
	switch status.State {
	case pullrequest.StateOpen:
		return prLanding{merge: true, atOnce: decision.MergeAtOnce, api: api, record: rec}, "", shedengine.OutputPointer{}, nil, false
	case pullrequest.StateClosed:
		if !rec.ReviewEnded && !decision.MergeAtOnce {
			return prStopped(fz.halt(ctx, LandingStopPrefix+fmt.Sprintf("the run's pull request %s was closed without a merge; way forward: reopen it on GitHub, then lym loom goto PR-Review, or lym batten cancel %s", rec.URL, fz.deps.Slug)))
		}
		if decision.MergeAtOnce {
			return prLanding{merge: true, atOnce: true, reopen: true, api: api, record: rec}, "", shedengine.OutputPointer{}, nil, false
		}
		if err := api.Reopen(ctx, rec.Number); err != nil {
			return prStopped(fz.githubFailure(ctx, "reopen the pull request", err))
		}
		return prLanding{merge: true, api: api, record: rec}, "", shedengine.OutputPointer{}, nil, false
	}
	// A merged pull request has no GitHub merge left, yet the parent-side merge still follows the parent's mode.
	return prLanding{atOnce: decision.MergeAtOnce}, "", shedengine.OutputPointer{}, nil, false
}

// recordedBeamLanding decides the landing of a beam whose landing record holds the pull request recorded, which decision, the resolver's answer for that record, wants merged.
// An open one merges on GitHub, squashed, with no edit of its title or body; a closed one halts for the operator to reopen it on GitHub.
// A merged one lands through the already-landed path when it merged at the task head or lacks no task commit; one that lacks a task commit is decided as with none recorded.
// Every outcome but a halt marks the record to be dropped once the landing completed.
// stop is true when the other values are what Call returns.
func (fz *Finalize) recordedBeamLanding(ctx context.Context, mode string, recorded BeamPullRequest, origin Origin, hasCommits bool, decision PullRequestDecision, rec pullrequest.Record, found bool) (prLanding, shedengine.Outcome, shedengine.OutputPointer, error, bool) {
	pr := fz.deps.PullRequest
	if !decision.Open {
		return prLanding{dropBeamRecord: true}, "", shedengine.OutputPointer{}, nil, false
	}
	api, err := pr.API(origin.URL)
	if err != nil {
		return prStopped(fz.githubFailure(ctx, "open the GitHub client", err))
	}
	status, err := api.Status(ctx, recorded.Number)
	if err != nil {
		return prStopped(fz.githubFailure(ctx, "read the pull request's status", err))
	}
	switch status.State {
	case pullrequest.StateOpen:
		beamRecord := pullrequest.Record{Number: recorded.Number, URL: recorded.URL}
		return prLanding{merge: true, atOnce: true, dropBeamRecord: true, api: api, record: beamRecord}, "", shedengine.OutputPointer{}, nil, false
	case pullrequest.StateClosed:
		return prStopped(fz.halt(ctx, LandingStopPrefix+fmt.Sprintf("the beam's pull request %s was closed without a merge; way forward: the operator reopens it on GitHub, then runs lym loom resume", recorded.URL)))
	}

	head, err := fz.deps.TaskHead()
	if err != nil {
		return prStopped(fz.errorStop(ctx, fmt.Errorf("landingshed: %s: read the task head: %w", finalizeName, err)))
	}
	if status.HeadSHA != head {
		missing, err := pr.CommitsMissing(status.HeadSHA, head)
		if err != nil {
			return prStopped(fz.errorStop(ctx, fmt.Errorf("landingshed: %s: list the task commits the merged pull request lacks: %w", finalizeName, err)))
		}
		if len(missing) > 0 {
			again, err := resolveWithParentRule(ctx, finalizeName, fz.deps, mode, "", BeamWithoutPullRequest, origin, hasCommits, found)
			if err != nil {
				return prStopped(fz.errorStop(ctx, err))
			}
			if again.Halt != "" {
				return prStopped(fz.halt(ctx, LandingStopPrefix+again.Halt))
			}
			landing, outcome, out, err, stop := fz.landingForDecision(ctx, again, origin, rec, found)
			landing.dropBeamRecord = true
			return landing, outcome, out, err, stop
		}
	}
	return prLanding{atOnce: true, dropBeamRecord: true}, "", shedengine.OutputPointer{}, nil, false
}

// dropBeamRecord drops the spent pull request from the beam's landing record once the landing completed; a failed drop is logged only, since the landing has happened.
func (fz *Finalize) dropBeamRecord() {
	if err := fz.deps.PullRequest.DropBeamRecord(); err != nil {
		logger.Warn("landingshed: the landed pull request could not be dropped from the beam's landing record", "row", finalizeName, "cause", err)
	}
}

// dropRecordedPullRequest ends the run's pull request when the landing no longer goes through one.
// It closes a recorded pull request GitHub reports open with reason in the comment, removes the record, and lands locally.
func (fz *Finalize) dropRecordedPullRequest(ctx context.Context, origin Origin, rec pullrequest.Record, reason string) (prLanding, shedengine.Outcome, shedengine.OutputPointer, error, bool) {
	pr := fz.deps.PullRequest
	ended := func(outcome shedengine.Outcome, out shedengine.OutputPointer, err error, _ bool) (prLanding, shedengine.Outcome, shedengine.OutputPointer, error, bool) {
		return prLanding{}, outcome, out, err, true
	}
	api, err := pr.API(origin.URL)
	if err != nil {
		return ended(fz.githubFailure(ctx, "open the GitHub client", err))
	}
	status, err := api.Status(ctx, rec.Number)
	if err != nil {
		return ended(fz.githubFailure(ctx, "read the pull request's status", err))
	}
	if status.State == pullrequest.StateOpen {
		if err := api.Close(ctx, rec.Number, "lym closed this pull request: "+reason); err != nil {
			return ended(fz.githubFailure(ctx, "close the pull request", err))
		}
	}
	if err := pullrequest.RemoveRecord(pr.RecordPath, pr.RecordLockPath); err != nil {
		return ended(fz.errorStop(ctx, fmt.Errorf("landingshed: %s: %w", finalizeName, err)))
	}
	return prLanding{}, "", shedengine.OutputPointer{}, nil, false
}

// pullRequestCommitTitle is the title of the commit a merge of pull request number writes: title followed by ` (#<number>)`, the form GitHub's own squash merge writes, so the landed commit names the pull request it came from.
func pullRequestCommitTitle(title string, number int) string {
	return fmt.Sprintf("%s (#%d)", title, number)
}

// mergeOnGitHub merges the run's pull request on GitHub, which Finalize does after its lock re-check and before the parent-side merge.
// It pushes the task branch, opens the pull request through openAtOnce or reopens the closed recorded one when the landing is marked to, and merges with the pushed head as the expected head, squashing as Config.Squash says, or always for a parent that takes the pull request with no review, with title and the pull request's number as the commit title and message as the commit message.
// A merge GitHub refuses as pending is retried for at most a minute of the told clock.
// It then fetches the parent's upstream and fast-forwards the parent pair to it, so the parent-side merge that follows finds the parent holding the task's work.
// stop is true when Call returns the other values at once; each halt opens with LandingStopPrefix.
func (fz *Finalize) mergeOnGitHub(ctx context.Context, parentHandle parentMerger, landing prLanding, title, message string) (shedengine.Outcome, shedengine.OutputPointer, error, bool) {
	pr := fz.deps.PullRequest
	if err := pr.PushTask(); err != nil {
		if terr := transientFailure(ctx, finalizeName, "push the task branch", err); terr != nil {
			return fz.errorStop(ctx, terr)
		}
		return fz.halt(ctx, LandingStopPrefix+pushHaltReason(fz.deps.WorktreeRoot, err))
	}
	head, err := fz.deps.TaskHead()
	if err != nil {
		return fz.errorStop(ctx, fmt.Errorf("landingshed: %s: read the task head: %w", finalizeName, err))
	}
	if landing.reopen {
		if err := landing.api.Reopen(ctx, landing.record.Number); err != nil {
			return fz.githubFailure(ctx, "reopen the pull request", err)
		}
	}
	if landing.open {
		rec, outcome, out, err, stop := fz.openAtOnce(ctx, landing.api, head, title, message)
		if stop {
			return outcome, out, err, true
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
		if refusal.Kind == pullrequest.RefusalPending && pr.Now().Before(deadline) {
			if err := pr.Sleep(ctx, wait); err != nil {
				if cerr := cancelErr(ctx, finalizeName); cerr != nil {
					return "", shedengine.OutputPointer{}, cerr, true
				}
				return fz.errorStop(ctx, fmt.Errorf("landingshed: %s: sleep before retrying the merge: %w", finalizeName, err))
			}
			wait = nextPRWait(wait, false)
			continue
		}
		return fz.halt(ctx, LandingStopPrefix+fz.mergeRefusalReason(landing, refusal))
	}

	if _, err := fz.deps.ReadParentTips(true); err != nil {
		return fz.halt(ctx, LandingStopPrefix+fz.mergedButNotPulledReason(landing.record, fmt.Sprintf("the parent's upstream could not be fetched: %v", err)))
	}
	result, err := parentHandle.FastForwardPair(fabricengine.SyncOptions{SkipPush: fz.deps.PushSkipped})
	if err != nil {
		return fz.halt(ctx, LandingStopPrefix+fz.mergedButNotPulledReason(landing.record, fmt.Sprintf("the parent pair could not be fast-forwarded: %v", err)))
	}
	if result.Refusal != "" {
		return fz.halt(ctx, LandingStopPrefix+fz.mergedButNotPulledReason(landing.record, fmt.Sprintf("the parent pair refused the fast-forward: %s", result.Refusal)))
	}
	return "", shedengine.OutputPointer{}, nil, false
}

// openAtOnce finds the open pull request from the task branch into the parent and sets its title and body, else opens one with them, for a parent that takes its pull request with no review.
// A beam's landing, whose PullRequest.BeamRecord is set, leaves the text of a pull request it finds as the orch wrote it.
// It records the pull request with head as its gated head and the review ended, so a re-entry merges the recorded one and reopens it when it was closed.
// stop is true when Call returns the other values at once; a GitHub failure is judged as githubFailure judges one.
func (fz *Finalize) openAtOnce(ctx context.Context, api pullrequest.API, head, title, body string) (pullrequest.Record, shedengine.Outcome, shedengine.OutputPointer, error, bool) {
	failed := func(outcome shedengine.Outcome, out shedengine.OutputPointer, err error, _ bool) (pullrequest.Record, shedengine.Outcome, shedengine.OutputPointer, error, bool) {
		return pullrequest.Record{}, outcome, out, err, true
	}
	ref, open, err := api.FindOpen(ctx, fz.deps.TaskBranch, fz.deps.ParentBranch)
	if err != nil {
		return failed(fz.githubFailure(ctx, "look for an open pull request", err))
	}
	if !open {
		if ref, err = api.Create(ctx, fz.deps.TaskBranch, fz.deps.ParentBranch, title, body); err != nil {
			return failed(fz.githubFailure(ctx, "open the pull request", err))
		}
	} else if fz.deps.PullRequest.BeamRecord == nil {
		if err := api.Edit(ctx, ref.Number, title, body); err != nil {
			return failed(fz.githubFailure(ctx, "set the pull request's title and body", err))
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
		return failed(fz.errorStop(ctx, fmt.Errorf("landingshed: %s: record the pull request: %w", finalizeName, err)))
	}
	return rec, "", shedengine.OutputPointer{}, nil, false
}

// mergeRefusalReason is the Stuck reason of a merge GitHub refused for good, or kept refusing as pending past the retry window.
// A method refusal names the squash setting, or the repository's settings for a parent that always squashes.
// A beam's refusal never names a cancel: it names GitHub's message, the pull request's status verb and the resume, and a method refusal also names the repository's squash setting.
func (fz *Finalize) mergeRefusalReason(landing prLanding, refusal *pullrequest.MergeRefusal) string {
	rec := landing.record
	if fz.deps.PullRequest.BeamRecord != nil {
		check := fmt.Sprintf("check the pull request with lym beam pr status %s, then run lym loom resume", fz.deps.Slug)
		if refusal.Kind == pullrequest.RefusalMethod {
			return fmt.Sprintf("GitHub refused the squash merge of pull request %s: %s; way forward: allow squash merging in the repository's settings on GitHub, since a beam always lands by squash, then %s", rec.URL, refusal.Message, check)
		}
		return fmt.Sprintf("GitHub refused to merge pull request %s: %s; way forward: settle on GitHub what the message names, then %s", rec.URL, refusal.Message, check)
	}
	if refusal.Kind == pullrequest.RefusalMethod && landing.atOnce {
		return fmt.Sprintf("GitHub refused the squash merge of pull request %s: %s; way forward: allow squash merging in the repository's settings on GitHub, since a parent whose beam.yaml pull_request is merge always squashes, then run lym loom resume", rec.URL, refusal.Message)
	}
	if refusal.Kind == pullrequest.RefusalMethod {
		return fmt.Sprintf("GitHub refused the merge method for pull request %s: %s; way forward: set landing.yaml's squash to the method the repository allows with lym config landing --set squash=<true|false> in the prime worktree, or allow the method in the repository's settings, then run lym loom resume", rec.URL, refusal.Message)
	}
	return fmt.Sprintf("GitHub refused to merge pull request %s: %s; way forward: a second person approves the pull request again or settles the required check or conflict on GitHub, then runs lym loom resume; or the operator runs lym batten cancel %s", rec.URL, refusal.Message, fz.deps.Slug)
}

// mergedButNotPulledReason is the Stuck reason of a pull request merged on GitHub whose merge the parent pair could not take in.
// The re-entry finds the pull request merged and lands through the already-landed path.
func (fz *Finalize) mergedButNotPulledReason(rec pullrequest.Record, cause string) string {
	return fmt.Sprintf("pull request %s was merged on GitHub, but %s; way forward: fix that, then run lym loom resume, which finds the pull request merged and lands through the parent-side merge", rec.URL, cause)
}
