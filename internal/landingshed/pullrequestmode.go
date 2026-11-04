// pullrequestmode.go holds ResolvePullRequest, the one decision of whether a landing goes through a pull request, and the read of GitHub's rules for the parent branch both landing rows ask it with.

package landingshed

import (
	"context"
	"fmt"
	"strings"

	"github.com/Knatte18/loomyard/internal/githubclient"
	"github.com/Knatte18/loomyard/internal/logger"
)

// githubHost is the one host a landing opens a pull request on.
const githubHost = "github.com"

// Pull-request modes of the receiving branch's beam.yaml and the landing entry's field; merge is beam.yaml's alone.
const (
	pullRequestOn       = "on"
	pullRequestOff      = "off"
	pullRequestRequired = "required"
	pullRequestMerge    = "merge"
)

// Origin holds the facts about the origin a pull request needs.
type Origin struct {
	// URL is the task pair's origin remote URL, empty when there is none.
	URL string
	// Upstream reports whether the parent branch has an upstream.
	Upstream bool
	// PushSkipped reports whether the hub's push skip is set.
	PushSkipped bool
}

// PullRequestDecision is the outcome of ResolvePullRequest.
type PullRequestDecision struct {
	// Open reports whether the landing goes through a pull request.
	Open bool
	// MergeAtOnce reports whether Finalize opens the pull request itself and squash-merges it on GitHub at once, with no review, as a parent whose beam.yaml pull_request is merge asks.
	MergeAtOnce bool
	// Reason names the values the decision read.
	Reason string
	// Halt is a Stuck reason with its way forward, non-empty only when the landing must stop.
	Halt string
}

// ResolvePullRequest decides whether a landing goes through a pull request.
// parentMode is the receiving branch's beam.yaml pull_request (on, off, required or merge) and entryField the landing entry's pull_request (empty, on or off); slug is the landing run's slug.
// parentRuleRequired reports whether GitHub's rules for the parent branch require a pull request, and counts as a required parent whatever beam.yaml says, since GitHub refuses the direct push a landing with no pull request ends in.
// A required or merge parent wants one whatever the field says, an on parent wants one unless the field is off, and an off parent wants one only when the field is on.
// A merge parent's yes is marked MergeAtOnce.
// A field off under a required or merge parent logs one Warn and still wants one.
// A wanted pull request needs, in this order, an origin on github.com, an origin ParseOwnerRepo accepts, an upstream for the parent and no push skip.
// A missing fact is a Halt when the parent is required or the field is on, and an origin on github.com that ParseOwnerRepo refuses is always a Halt; any other missing fact under an on parent alone is no pull request, naming the fact.
// A wanted pull request on a task branch with no commit beyond its parent, taskHasCommits false, is no pull request before any origin fact is judged, since GitHub opens none on an empty branch.
// It reads no file: the caller reads the parent's beam.yaml, the entry, the parent branch's rules and the task branch and tells the values.
func ResolvePullRequest(slug, parentMode, entryField string, parentRuleRequired bool, origin Origin, taskHasCommits bool) PullRequestDecision {
	mergeAtOnce := parentMode == pullRequestMerge
	required := parentMode == pullRequestRequired || mergeAtOnce || parentRuleRequired
	if required && entryField == pullRequestOff {
		logger.Warn("landingshed: the entry's pull_request is off but the parent's beam.yaml pull_request is required or merge, or GitHub's rules for the parent branch require a pull request, so a pull request is still required", "slug", slug)
	}

	field := entryField
	if field == "" {
		field = "empty"
	}
	wanted := false
	demand := ""
	switch {
	case parentMode == pullRequestRequired, mergeAtOnce:
		wanted, demand = true, fmt.Sprintf("the parent's beam.yaml pull_request is %s", parentMode)
	case parentRuleRequired:
		wanted, demand = true, "GitHub's rules for the parent branch require a pull request"
	case entryField == pullRequestOn:
		wanted, demand = true, "the entry's pull_request is on"
	case parentMode == pullRequestOn && entryField != pullRequestOff:
		wanted, demand = true, "the parent's beam.yaml pull_request is on"
	}
	if !wanted {
		return PullRequestDecision{Reason: fmt.Sprintf("pull request off: the parent's beam.yaml pull_request is %s and the entry's pull_request is %s", parentMode, field)}
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
		return PullRequestDecision{Open: true, MergeAtOnce: mergeAtOnce, Reason: fmt.Sprintf("pull request on: the parent's beam.yaml pull_request is %s%s, the entry's pull_request is %s and the origin is %s/%s", parentMode, rule, field, owner, repo)}
	}

	demanded := required || entryField == pullRequestOn
	if !demanded && !alwaysHalt {
		return PullRequestDecision{Reason: fmt.Sprintf("no pull request: the parent's beam.yaml pull_request is on and %s", missing)}
	}
	halt := fmt.Sprintf("a pull request is required here (%s) but %s; way forward: give the hub a GitHub origin, an upstream for the parent branch and no push skip, then run lym loom resume", demand, missing)
	if entryField == pullRequestOn && !required {
		halt += fmt.Sprintf(", or run lym batten cancel %s and start a new run with the entry's pull_request changed", slug)
	}
	return PullRequestDecision{Halt: halt}
}

// resolveWithParentRule asks ResolvePullRequest for the run deps tells, and asks it again with GitHub's rules for the parent branch when its answer is a no that would land a non-empty task by direct push to an origin a pull request could be opened on.
// The rules are read only then, so a landing beam.yaml or the entry already sends through a pull request, and one that never reaches GitHub, makes no extra call.
// A cancelled context and a transient failure of the read are errors the driver re-steps.
// recordFound reports whether the run already holds a pull request record, which shows the rule applied when the pull request was opened.
// Then any other failure of the read except an unusable token is a Halt, since reading it as no rule would close the recorded pull request and land by a direct push GitHub refuses.
// Otherwise any failure of the read, an unusable token included, logs one Warn and reads as no rule, so the landing goes as beam.yaml and the entry say.
func resolveWithParentRule(ctx context.Context, name string, deps Deps, parentMode, entryField string, origin Origin, taskHasCommits, recordFound bool) (PullRequestDecision, error) {
	decision := ResolvePullRequest(deps.Slug, parentMode, entryField, false, origin, taskHasCommits)
	if decision.Open || decision.Halt != "" || !taskHasCommits {
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
		logger.Warn("landingshed: GitHub's rules for the parent branch are unreadable, so the landing goes as beam.yaml and the entry say", "row", name, "branch", deps.ParentBranch, "cause", err)
		return decision, nil
	}
	if !required {
		return decision, nil
	}
	return ResolvePullRequest(deps.Slug, parentMode, entryField, true, origin, taskHasCommits), nil
}

// readParentRule reports whether GitHub's rules for deps.ParentBranch, on the repository originURL names, require a pull request.
func readParentRule(ctx context.Context, deps Deps, originURL string) (bool, error) {
	api, err := deps.PullRequest.API(originURL)
	if err != nil {
		return false, err
	}
	return api.RequiresPullRequest(ctx, deps.ParentBranch)
}

// checkOrigin returns the origin's owner and repository, or the first missing fact in the order a pull request needs them, or "" when none is missing.
// alwaysHalt is true for an origin on github.com that ParseOwnerRepo refuses.
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
