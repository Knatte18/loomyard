// pullrequestmode.go holds ResolvePullRequest, which decides whether a landing goes through a pull request.
// It also holds resolveWithParentRule, which decides again with GitHub's rules for the parent branch when needed.

package landingshed

import (
	"context"
	"fmt"
	"strings"

	"github.com/Knatte18/loomyard/internal/beamsetting"
	"github.com/Knatte18/loomyard/internal/githubclient"
	"github.com/Knatte18/loomyard/internal/logger"
)

// githubHost is the one host a landing opens a pull request on.
const githubHost = "github.com"

// Origin holds the facts about the origin a pull request needs.
type Origin struct {
	// URL is the task pair's origin remote URL, empty when there is none.
	URL string
	// Upstream reports whether the parent branch has an upstream.
	Upstream bool
	// PushSkipped reports whether the hub's push skip is set.
	PushSkipped bool
}

// PullRequestDecision is the answer to whether a landing goes through a pull request, with the reason for it.
type PullRequestDecision struct {
	// Open reports whether the landing goes through a pull request.
	Open bool
	// MergeAtOnce reports whether the landing opens a pull request and squash-merges it on GitHub at once, with no review.
	// A landing whose beam.pull_request is trail asks for it, and so does every beam.
	MergeAtOnce bool
	// TrueMerge reports whether the landing keeps the task branch's history.
	// It lands as a fast-forward or a merge commit, never a squash,
	// so a merge of the parent's own parent stays an ancestor of the parent.
	TrueMerge bool
	// Reason names the values the decision read.
	Reason string
	// Halt is a Stuck reason with its way forward, non-empty only when the landing must stop.
	Halt string
}

// BeamRecord is what a beam's landing record says about the beam's pull request.
// The landing decision reads it when the landing is a beam's.
type BeamRecord int

const (
	// NotABeam marks a landing that is no beam's, which is decided from beam.pull_request alone.
	NotABeam BeamRecord = iota
	// BeamWithoutPullRequest marks a beam whose landing record holds no pull request.
	BeamWithoutPullRequest BeamRecord = 1
	// BeamWithPullRequest marks a beam whose landing record holds a pull request.
	BeamWithPullRequest BeamRecord = 2
)

// BeamPullRequest is the pull request a beam's landing record holds.
type BeamPullRequest struct {
	// Number is the pull request's number.
	Number int
	// URL is the pull request's web address.
	URL string
}

// ResolvePullRequest decides whether a landing goes through a pull request.
// mode is beam.pull_request for this landing, found by the per-task lookup:
// one of review, off, trail or merge-in.
// A merge-in mode on a landing that is no beam's wants no pull request, whatever GitHub's rules say, and marks TrueMerge:
// a pull request squashes away the merged commit, and the merge is reviewed again in the beam's own pull request.
// On a beam, merge-in reads as off.
// parentRuleRequired reports whether GitHub's rules for the parent branch require a pull request.
// A landing without a pull request ends in a direct push, and GitHub refuses that push.
// A pull request is required when the mode is trail or GitHub's rules require one.
// A pull request is wanted when the beam's landing record holds one or when it is required.
// It is also wanted when the mode is review and the landing is no beam's.
// Under review or off, a beam whose record holds none wants none.
// A pull request is marked MergeAtOnce when the mode is trail or the landing is a beam's.
// A beam always lands by squash on GitHub with no review.
// A wanted pull request needs, in this order:
// an origin on github.com, an origin whose URL names owner and repository, an upstream for the parent and no push skip.
// A missing fact is a Halt when a pull request is required or the beam's record holds one.
// An origin on github.com whose URL names no owner and repository is always a Halt.
// Any other missing fact gives no pull request, and the reason names that fact.
// A task branch with no commit beyond its parent gets no pull request, even when one is wanted.
// That is decided before any origin fact is judged, since GitHub opens no pull request on an empty branch.
// It reads no file: the caller reads these and passes their values in.
// These are beam.pull_request, the beam's landing record, the parent's GitHub rules and the task branch.
func ResolvePullRequest(mode string, beam BeamRecord, parentRuleRequired bool, origin Origin, taskHasCommits bool) PullRequestDecision {
	isBeam := beam != NotABeam
	modeClause := fmt.Sprintf("%s for this landing is %s", beamPullRequestName(), mode)
	if !isBeam && mode == beamsetting.PullRequestMergeIn {
		return PullRequestDecision{TrueMerge: true, Reason: fmt.Sprintf("no pull request: %s, so the task lands by a true merge whatever GitHub's rules say", modeClause)}
	}
	mergeAtOnce := mode == beamsetting.PullRequestTrail || isBeam
	required := mode == beamsetting.PullRequestTrail || parentRuleRequired

	beamClause := ""
	switch beam {
	case BeamWithPullRequest:
		beamClause = ", and the beam's landing record holds a pull request"
	case BeamWithoutPullRequest:
		beamClause = ", and the beam's landing record holds no pull request"
	}
	wanted := false
	demand := ""
	switch {
	case beam == BeamWithPullRequest:
		wanted, demand = true, "the beam's landing record holds a pull request"
	case mode == beamsetting.PullRequestTrail:
		wanted, demand = true, modeClause
	case parentRuleRequired:
		wanted, demand = true, "GitHub's rules for the parent branch require a pull request"
	case mode == beamsetting.PullRequestReview && !isBeam:
		wanted, demand = true, modeClause
	}
	if !wanted {
		return PullRequestDecision{Reason: fmt.Sprintf("pull request off: %s%s", modeClause, beamClause)}
	}
	if !taskHasCommits {
		return PullRequestDecision{Reason: fmt.Sprintf("no pull request: %s, but the task branch has no commits beyond its parent, so the landing is empty", demand)}
	}

	owner, repo, missing, alwaysHalt := checkOrigin(origin)
	if missing == "" {
		rule := ""
		if parentRuleRequired {
			rule = ", GitHub's rules for the parent branch require one"
		}
		return PullRequestDecision{Open: true, MergeAtOnce: mergeAtOnce, Reason: fmt.Sprintf("pull request on: %s%s%s, and the origin is %s/%s", modeClause, rule, beamClause, owner, repo)}
	}

	demanded := required || beam == BeamWithPullRequest
	if !demanded && !alwaysHalt {
		return PullRequestDecision{Reason: fmt.Sprintf("no pull request: %s and %s", modeClause, missing)}
	}
	return PullRequestDecision{Halt: fmt.Sprintf("a pull request is required here (%s) but %s; way forward: give the hub a GitHub origin, an upstream for the parent branch and no push skip, then run lym loom resume", demand, missing)}
}

// resolveWithParentRule decides whether a landing goes through a pull request.
// A no answer that would push a non-empty task directly to an origin a pull request could open on:
// the decision is made again with GitHub's rules for the parent branch.
// The rules are read only for that answer,
// so a landing already sent through a pull request makes no extra call.
// A no marked TrueMerge, which no rule changes, and one that never reaches GitHub make none either.
// A cancelled context and a transient failure of the read are errors the driver re-steps.
// beam is what the beam's landing record holds, NotABeam for a landing that is no beam's.
// recordFound reports whether the run already holds a pull request record.
// Such a record shows the rule applied when the pull request was opened.
// When recordFound is set, any other failure of the read except an unusable token is a Halt.
// Reading it as no rule would close the recorded pull request and land by a direct push that GitHub refuses.
// Otherwise any failure of the read, an unusable token included, logs one warning and reads as no rule,
// so the landing goes as beam.yaml says.
func resolveWithParentRule(ctx context.Context, name string, deps Deps, mode string, beam BeamRecord, origin Origin, taskHasCommits, recordFound bool) (PullRequestDecision, error) {
	decision := ResolvePullRequest(mode, beam, false, origin, taskHasCommits)
	if decision.Open || decision.TrueMerge || decision.Halt != "" || !taskHasCommits {
		return decision, nil
	}
	if _, _, missing, _ := checkOrigin(origin); missing != "" {
		return decision, nil
	}
	const action = "read GitHub's rules for the parent branch"
	required, err := readParentRule(ctx, deps, origin.URL)
	if err != nil {
		if !tokenFailure(err) {
			if terr := transientFailure(ctx, name, action, err); terr != nil {
				return PullRequestDecision{}, terr
			}
			if recordFound {
				return PullRequestDecision{Halt: githubHaltReason(action, err)}, nil
			}
		}
		logger.Warn("landingshed: GitHub's rules for the parent branch are unreadable, so the landing goes as beam.yaml says", "row", name, "branch", deps.ParentBranch, "cause", err)
		return decision, nil
	}
	if !required {
		return decision, nil
	}
	return ResolvePullRequest(mode, beam, true, origin, taskHasCommits), nil
}

// readParentRule reports whether GitHub's rules for deps.ParentBranch require a pull request.
// The rules are read from the repository at originURL.
func readParentRule(ctx context.Context, deps Deps, originURL string) (bool, error) {
	api, err := deps.PullRequest.API(originURL)
	if err != nil {
		return false, err
	}
	return api.RequiresPullRequest(ctx, deps.ParentBranch)
}

// OriginMissing returns the first fact an origin lacks for a pull request, or "" when none is missing.
// The facts are checked in the order a pull request needs them.
// A caller that opens a pull request itself refuses an origin for which OriginMissing returns a non-empty string.
func OriginMissing(origin Origin) string {
	_, _, missing, _ := checkOrigin(origin)
	return missing
}

// checkOrigin returns the origin's owner and repository, or the first fact the origin lacks.
// It checks the facts in the order a pull request needs them, and missing is "" when all are present.
// alwaysHalt is true for an origin on github.com whose URL names no repository that lym reads.
func checkOrigin(origin Origin) (owner, repo, missing string, alwaysHalt bool) {
	if !strings.EqualFold(originHost(origin.URL), githubHost) {
		if origin.URL == "" {
			return "", "", "the task pair has no origin remote", false
		}
		return "", "", fmt.Sprintf("the origin %s is not on %s", origin.URL, githubHost), false
	}
	owner, repo, err := githubclient.ParseOwnerRepo(origin.URL)
	if err != nil {
		return "", "", fmt.Sprintf("the origin %s is on %s but names no repository lym reads: %v", origin.URL, githubHost, err), true
	}
	switch {
	case !origin.Upstream:
		return "", "", "the parent branch has no upstream", false
	case origin.PushSkipped:
		return "", "", "the hub's push skip is set", false
	}
	return owner, repo, "", false
}

// originHost returns the host of a remote URL: the part after any scheme:// and any user-info@, up to the first : or /.
// The user@host:path scp form gives the part between @ and :, and a local path gives "".
func originHost(remoteURL string) string {
	rest := remoteURL
	if i := strings.Index(rest, "://"); i >= 0 {
		rest = rest[i+len("://"):]
	}
	authority, _, _ := strings.Cut(rest, "/")
	if i := strings.LastIndex(authority, "@"); i >= 0 {
		authority = authority[i+1:]
	}
	host, _, _ := strings.Cut(authority, ":")
	return host
}
