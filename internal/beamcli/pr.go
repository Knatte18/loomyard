// pr.go holds the "lym beam pr open", "update", "status" and "close" sequences.
// They are thin verbs over the pull request from a beam's branch into its parent.
// The beam's orch runs them so it never runs gh by hand.

package beamcli

import (
	"context"
	"errors"
	"fmt"

	"github.com/Knatte18/loomyard/internal/githubclient"
	"github.com/Knatte18/loomyard/internal/landingshed"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/pawlengine"
	"github.com/Knatte18/loomyard/internal/pullrequest"
	"github.com/Knatte18/loomyard/internal/shedtransient"
)

// The pull_request modes a beam's parent reports its follow-up by.
const (
	parentModeRequired = "required"
	parentModeMerge    = "merge"
)

// The comments left on a pull request when a beam closes it.
const (
	closeOnlyComment              = "Closed by the beam's orch; the beam's work stays on its branch."
	landWithoutPullRequestComment = "Closed because the operator chose to land the beam without a pull request."
)

// beamPullRequestSide holds what a pull request verb needs of the beam's pair.
// API is nil when Origin lacks a fact a pull request needs, or when the API could not be opened;
// APIErr holds the cause in that case. A verb that calls API checks the origin first.
// Branch is the beam pair's branch and ParentBranch the branch it lands on.
type beamPullRequestSide struct {
	API          pullrequest.API
	APIErr       error
	Origin       landingshed.Origin
	Branch       string
	ParentBranch string
}

// prOpen opens the pull request from the beam slug's branch into its parent.
// When the landing record already names one, it brings that one to an open state.
// It returns what it did.
// A recorded pull request GitHub reports open changes nothing but commits the landing directory.
// A closed one is reopened, and a merged one is left for Finalize.
// With none recorded, it pushes the beam branch and finds an open pull request from it.
// It creates one with the description's text when there is none, and records the pull request with the pushed head.
func prOpen(ctx context.Context, deps landDeps, slug string) (map[string]any, error) {
	const verb = "lym beam pr open"
	if err := judgePullRequestVerb(deps, verb, slug); err != nil {
		return nil, err
	}
	record, found, err := deps.Landing(slug)
	if err != nil {
		return nil, rerunRefusal(verb, slug, "read the landing record", err)
	}
	if !found || record.DescribedAt.IsZero() {
		return nil, fmt.Errorf("%s: no change description is written for the beam %s; way forward: run `lym beam describe %s`, then `%s %s`", verb, slug, slug, verb, slug)
	}
	hasCommits, err := deps.BeamHasCommits(slug)
	if err != nil {
		return nil, rerunRefusal(verb, slug, "read the beam's commits", err)
	}
	if !hasCommits {
		return nil, fmt.Errorf("%s: the beam %s has no commits beyond its parent, so GitHub opens no pull request; way forward: land a member into the beam first, or land without a pull request with `lym beam ready %s`", verb, slug, slug)
	}
	side, err := deps.PullRequestSide(slug)
	if err != nil {
		return nil, rerunRefusal(verb, slug, "read the beam's origin", err)
	}
	if err := requireOrigin(verb, slug, side); err != nil {
		return nil, err
	}
	if record.PullRequest != nil {
		return openRecorded(ctx, deps, side, slug, *record.PullRequest)
	}

	head, err := deps.PushBeam(slug)
	if err != nil {
		return nil, rerunRefusal(verb, slug, "push the beam branch", err)
	}
	ref, open, err := side.API.FindOpen(ctx, side.Branch, side.ParentBranch)
	if err != nil {
		return nil, rerunRefusal(verb, slug, "look for an open pull request", err)
	}
	if open {
		if err := markReadyForReview(ctx, side, ref.Number); err != nil {
			return nil, rerunRefusal(verb, slug, "mark the pull request ready for review", err)
		}
	} else {
		title, body, err := deps.Description(slug)
		if err != nil {
			return nil, rerunRefusal(verb, slug, "read the change description", err)
		}
		if ref, err = side.API.Create(ctx, side.Branch, side.ParentBranch, title, body); err != nil {
			return nil, rerunRefusal(verb, slug, "open the pull request", err)
		}
	}
	err = deps.WriteLanding(slug, func(rec *pawlengine.LandingRecord) error {
		rec.PullRequest = &pawlengine.LandingPullRequest{Number: ref.Number, URL: ref.URL, PushedHead: head, AnnounceHead: head}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("%s: the pull request %s stands but recording it failed: %w; way forward: fix the cause it names, then run `%s %s` again, which finds the pull request open and records it", verb, ref.URL, err, verb, slug)
	}
	return map[string]any{
		"slug":        slug,
		"number":      ref.Number,
		"url":         ref.URL,
		"head":        head,
		"created":     !open,
		"way_forward": pullRequestFollowUp(slug),
	}, nil
}

// openRecorded settles the pull request the landing record names according to where GitHub reports it.
func openRecorded(ctx context.Context, deps landDeps, side beamPullRequestSide, slug string, recorded pawlengine.LandingPullRequest) (map[string]any, error) {
	const verb = "lym beam pr open"
	status, err := side.API.Status(ctx, recorded.Number)
	if err != nil {
		return nil, rerunRefusal(verb, slug, fmt.Sprintf("read pull request #%d", recorded.Number), err)
	}
	result := map[string]any{"slug": slug, "number": recorded.Number, "url": recorded.URL, "way_forward": pullRequestFollowUp(slug)}
	switch status.State {
	case pullrequest.StateOpen:
		// A record a failed commit left uncommitted is committed here.
		err := deps.WriteLanding(slug, func(*pawlengine.LandingRecord) error { return nil })
		if err != nil {
			return nil, rerunRefusal(verb, slug, "commit the landing record", err)
		}
		result["already"] = true
	case pullrequest.StateClosed:
		if err := side.API.Reopen(ctx, recorded.Number); err != nil {
			return nil, rerunRefusal(verb, slug, fmt.Sprintf("reopen pull request #%d", recorded.Number), err)
		}
		result["reopened"] = true
	case pullrequest.StateMerged:
		result["merged"] = true
		result["way_forward"] = fmt.Sprintf("pull request #%d is merged; run `lym beam ready %s`, after which Finalize lands the beam", recorded.Number, slug)
	default:
		return nil, fmt.Errorf("%s: GitHub reports pull request #%d in the state %q, which this verb does not know; way forward: look at %s, then run `%s %s` again", verb, recorded.Number, status.State, recorded.URL, verb, slug)
	}
	return result, nil
}

// prUpdate pushes the beam branch and sets the recorded pull request's title and body from the change description.
// It marks a draft ready for review and records the pushed head.
func prUpdate(ctx context.Context, deps landDeps, slug string) (map[string]any, error) {
	const verb = "lym beam pr update"
	if err := judgePullRequestVerb(deps, verb, slug); err != nil {
		return nil, err
	}
	record, found, err := deps.Landing(slug)
	if err != nil {
		return nil, rerunRefusal(verb, slug, "read the landing record", err)
	}
	if !found || record.PullRequest == nil {
		return nil, fmt.Errorf("%s: the beam %s has no pull request recorded; way forward: run `lym beam pr open %s`", verb, slug, slug)
	}
	number := record.PullRequest.Number
	side, err := deps.PullRequestSide(slug)
	if err != nil {
		return nil, rerunRefusal(verb, slug, "read the beam's origin", err)
	}
	if err := requireOrigin(verb, slug, side); err != nil {
		return nil, err
	}

	head, err := deps.PushBeam(slug)
	if err != nil {
		return nil, rerunRefusal(verb, slug, "push the beam branch", err)
	}
	title, body, err := deps.Description(slug)
	if err != nil {
		return nil, rerunRefusal(verb, slug, "read the change description", err)
	}
	if err := side.API.Edit(ctx, number, title, body); err != nil {
		return nil, rerunRefusal(verb, slug, fmt.Sprintf("edit pull request #%d", number), err)
	}
	if err := markReadyForReview(ctx, side, number); err != nil {
		return nil, rerunRefusal(verb, slug, "mark the pull request ready for review", err)
	}
	err = deps.WriteLanding(slug, func(rec *pawlengine.LandingRecord) error {
		if rec.PullRequest != nil {
			rec.PullRequest.PushedHead = head
		}
		return nil
	})
	if err != nil {
		return nil, rerunRefusal(verb, slug, "record the pushed head", err)
	}
	return map[string]any{
		"slug":        slug,
		"number":      number,
		"url":         record.PullRequest.URL,
		"head":        head,
		"way_forward": pullRequestFollowUp(slug),
	}, nil
}

// judgePullRequestVerb runs the refusals every pull request verb shares, in order.
// It refuses unless the entry is an open beam with its pair here;
// no other machine owns the beam or its parent;
// and the beam's run is at Beam-Await, and its ready record does not stand.
func judgePullRequestVerb(deps landDeps, verb, slug string) error {
	here, err := deps.Machine()
	if err != nil {
		return fmt.Errorf("%s: read this machine's name: %w", verb, err)
	}
	owners, err := deps.ReadOwners()
	if err != nil {
		return fmt.Errorf("%s: read the board's owners: %w", verb, err)
	}
	if err := judgeLandEntry(deps, owners, verb, slug); err != nil {
		return err
	}
	if err := judgeLandOwners(deps, owners, here, verb, slug); err != nil {
		return err
	}
	if err := judgeAwaitingRun(deps, verb, slug); err != nil {
		return err
	}
	return judgeNotReady(deps, verb, slug)
}

// requireOrigin refuses a side whose origin lacks a fact a pull request needs, and names the fact.
// A side whose API could not be opened is refused with its cause named.
func requireOrigin(verb, slug string, side beamPullRequestSide) error {
	if missing := landingshed.OriginMissing(side.Origin); missing != "" {
		return fmt.Errorf("%s: no pull request is possible for the beam %s: %s; way forward: give the hub a GitHub origin, an upstream for the parent branch and no push skip, then run `%s %s` again, or land without a pull request with `lym beam ready %s`", verb, slug, missing, verb, slug, slug)
	}
	if side.API == nil {
		return rerunRefusal(verb, slug, "open the pull request API", side.APIErr)
	}
	return nil
}

// markReadyForReview turns the pull request number from a draft into one ready for review.
// It leaves a pull request that is not a draft as it is.
func markReadyForReview(ctx context.Context, side beamPullRequestSide, number int) error {
	status, err := side.API.Status(ctx, number)
	if err != nil {
		return err
	}
	if !status.Draft {
		return nil
	}
	return side.API.SetDraft(ctx, number, false)
}

// rerunRefusal words the refusal of a failed step of verb.
// The message names the step and its cause, then the way forward: fix the cause and run the same verb for slug again.
func rerunRefusal(verb, slug, step string, err error) error {
	return fmt.Errorf("%s: %s: %w; way forward: fix the cause it names, then run `%s %s` again", verb, step, err, verb, slug)
}

// pullRequestFollowUp names the next steps once the pull request stands.
// The operator reviews it.
// After a change, the orch revises the description and records it, which updates the pull request;
// when the operator says so, it is marked ready.
func pullRequestFollowUp(slug string) string {
	return fmt.Sprintf("review the pull request with the operator; after a change revise the description and run `lym beam describe %s`, which updates the pull request, and run `lym beam ready %s` when the operator says so", slug, slug)
}

// prStatus reads the pull request the beam slug's landing record names.
// It reports whether GitHub would merge it now and what stops it, and it changes nothing.
// It answers at any row up to Beam-Down.
// With none recorded, it says so and names what decides the landing's pull request at the row the run stands at.
func prStatus(ctx context.Context, deps landDeps, slug string) (map[string]any, error) {
	const verb = "lym beam pr status"
	owners, err := deps.ReadOwners()
	if err != nil {
		return nil, fmt.Errorf("%s: read the board's owners: %w", verb, err)
	}
	if err := judgeLandEntry(deps, owners, verb, slug); err != nil {
		return nil, err
	}
	record, found, err := deps.Landing(slug)
	if err != nil {
		return nil, rerunRefusal(verb, slug, "read the landing record", err)
	}
	if !found || record.PullRequest == nil {
		status, runFound, err := deps.ChildStatus(slug)
		if err != nil {
			return nil, rerunRefusal(verb, slug, "read the pawl run", err)
		}
		wayForward := fmt.Sprintf("Finalize decides the landing's pull request by the parent's rules; run `lym batten status %s` to follow the landing", slug)
		if runFound && status.CurrentProducer == pawlengine.NameBeamAwait {
			wayForward = fmt.Sprintf("no pull request is recorded; run `lym beam pr open %s` when the operator wants one", slug)
		}
		return map[string]any{"slug": slug, "recorded": false, "way_forward": wayForward}, nil
	}

	recorded := *record.PullRequest
	side, err := deps.PullRequestSide(slug)
	if err != nil {
		return nil, rerunRefusal(verb, slug, "read the beam's origin", err)
	}
	if err := requireOrigin(verb, slug, side); err != nil {
		return nil, err
	}
	head, err := deps.BeamHead(slug)
	if err != nil {
		return nil, rerunRefusal(verb, slug, "read the beam's head", err)
	}
	merge, err := pullrequest.ReadMergeState(ctx, side.API, recorded.Number, head, side.ParentBranch, deps.Now, deps.Sleep)
	if err != nil {
		return nil, rerunRefusal(verb, slug, fmt.Sprintf("read the merge state of pull request #%d", recorded.Number), err)
	}
	checks := make([]map[string]string, 0, len(merge.Checks))
	for _, check := range merge.Checks {
		checks = append(checks, map[string]string{"name": check.Name, "conclusion": check.Conclusion})
	}
	result := map[string]any{
		"slug":            slug,
		"recorded":        true,
		"number":          recorded.Number,
		"url":             recorded.URL,
		"state":           string(merge.State),
		"draft":           merge.Draft,
		"head":            merge.Head,
		"mergeable_state": merge.MergeableState,
		"approved":        merge.Approved,
		"checks":          checks,
		"mergeable_now":   merge.MergeableNow,
	}
	if merge.MergeableNow {
		result["way_forward"] = fmt.Sprintf("the pull request can merge now; run `lym beam ready %s` when the operator says so", slug)
		return result, nil
	}
	result["causes"] = merge.Causes
	result["way_forward"] = fmt.Sprintf("fix each cause named, then run `lym beam pr status %s` again", slug)
	return result, nil
}

// prClose closes the pull request the beam slug's landing record names, with one comment.
// It then drops the pull request from the record, so the beam lands without it.
// An open pull request is closed; one that GitHub reports as closed or merged is only dropped.
// The landing decision for a beam with no pull request chooses the comment.
// When it is no, the comment says the operator chose to land without a pull request;
// otherwise it is the close-only comment.
// A failed read of the parent's mode or of the beam's commits still closes and drops, with the close-only comment.
// The read's error is named in the way forward.
func prClose(ctx context.Context, deps landDeps, slug string) (map[string]any, error) {
	const verb = "lym beam pr close"
	if err := judgePullRequestVerb(deps, verb, slug); err != nil {
		return nil, err
	}
	record, found, err := deps.Landing(slug)
	if err != nil {
		return nil, rerunRefusal(verb, slug, "read the landing record", err)
	}
	if !found || record.PullRequest == nil {
		return nil, fmt.Errorf("%s: the beam %s has no pull request recorded; way forward: run `lym beam pr open %s` to record one, or `lym beam ready %s` to land without one", verb, slug, slug, slug)
	}
	recorded := *record.PullRequest
	side, err := deps.PullRequestSide(slug)
	if err != nil {
		return nil, rerunRefusal(verb, slug, "read the beam's origin", err)
	}
	if err := requireOrigin(verb, slug, side); err != nil {
		return nil, err
	}
	status, err := side.API.Status(ctx, recorded.Number)
	if err != nil {
		return nil, rerunRefusal(verb, slug, fmt.Sprintf("read pull request #%d", recorded.Number), err)
	}

	mode, rulesRequired, readErr := deps.PullRequestMode(slug)
	hasCommits := false
	if readErr == nil {
		hasCommits, readErr = deps.BeamHasCommits(slug)
	}
	comment := closeOnlyComment
	wayForward := ""
	if readErr == nil {
		decision := landingshed.ResolvePullRequest(slug, mode, "", landingshed.BeamWithoutPullRequest, rulesRequired, side.Origin, hasCommits)
		if !decision.Open {
			comment = landWithoutPullRequestComment
		}
		wayForward = closeFollowUp(slug, mode, decision.Open)
	} else {
		wayForward = fmt.Sprintf("the parent's pull request mode or the beam's commits could not be read: %v; fix the cause, then run `lym beam ready %s`, which refuses what a pull request is still owed", readErr, slug)
	}

	closed := status.State == pullrequest.StateOpen
	if closed {
		if err := side.API.Close(ctx, recorded.Number, comment); err != nil {
			return nil, rerunRefusal(verb, slug, fmt.Sprintf("close pull request #%d", recorded.Number), err)
		}
	}
	err = deps.WriteLanding(slug, func(rec *pawlengine.LandingRecord) error {
		rec.PullRequest = nil
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("%s: pull request #%d is closed but the landing record still names it: %w; way forward: fix the cause it names, then run `%s %s` again, which only drops the closed pull request", verb, recorded.Number, err, verb, slug)
	}
	return map[string]any{
		"slug":        slug,
		"number":      recorded.Number,
		"url":         recorded.URL,
		"closed":      closed,
		"dropped":     true,
		"way_forward": wayForward,
	}, nil
}

// closeFollowUp names what follows a dropped pull request.
// The caller passes whether the beam's landing decision, with no pull request recorded, still wants one.
// It also passes the parent's pull request mode.
// A merge parent's Finalize opens its own pull request.
// Any other wanted pull request comes from a required parent or GitHub's rules,
// and `lym beam ready` refuses until one is recorded again.
func closeFollowUp(slug, mode string, wantsPullRequest bool) string {
	switch {
	case !wantsPullRequest:
		return fmt.Sprintf("the pull request is dropped from the landing record; run `lym beam ready %s` when the operator says so, after which Finalize lands the beam without a pull request", slug)
	case mode == parentModeMerge:
		return fmt.Sprintf("the pull request is dropped, but the parent's pull_request is merge, so Finalize still lands through a pull request it finds or opens; run `lym beam ready %s` when the operator says so", slug)
	default:
		demand := "GitHub's rules for the parent branch require one"
		if mode == parentModeRequired {
			demand = "the parent's pull_request is required"
		}
		return fmt.Sprintf("the pull request is dropped, but %s, so `lym beam ready %s` refuses until one is recorded again; run `lym beam pr open %s`", demand, slug, slug)
	}
}

// parentRuleRequired reports whether GitHub's rules for parentBranch require a pull request.
// It reads the rules through the API that open returns for the origin's URL.
// An origin missing a fact that a pull request needs reads as no rule, with no call made.
// A cancelled context and a transient failure are returned.
// An unusable token and any other failure read as no rule, with one warning,
// so every parent mode is then decided by beam.yaml alone.
func parentRuleRequired(ctx context.Context, origin landingshed.Origin, parentBranch string, open func(originURL string) (pullrequest.API, error)) (bool, error) {
	if landingshed.OriginMissing(origin) != "" {
		return false, nil
	}
	api, err := open(origin.URL)
	if err == nil {
		var required bool
		if required, err = api.RequiresPullRequest(ctx, parentBranch); err == nil {
			return required, nil
		}
	}
	tokenFailure := errors.Is(err, githubclient.ErrTokenUnresolvable) || errors.Is(err, githubclient.ErrTokenRejected)
	if !tokenFailure && (ctx.Err() != nil || shedtransient.Class(err) != "") {
		return false, err
	}
	logger.Warn("beamcli: GitHub's rules for the parent branch are unreadable, so the parent's beam.yaml decides alone", "branch", parentBranch, "cause", err)
	return false, nil
}
