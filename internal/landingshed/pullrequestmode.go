// pullrequestmode.go holds ResolvePullRequest, the one decision of whether a landing goes through a pull request.

package landingshed

import (
	"fmt"
	"strings"

	"github.com/Knatte18/loomyard/internal/githubclient"
	"github.com/Knatte18/loomyard/internal/logger"
)

// githubHost is the one host a landing opens a pull request on.
const githubHost = "github.com"

// Pull-request modes of the receiving branch's beam.yaml and the landing entry's field.
const (
	pullRequestOn       = "on"
	pullRequestOff      = "off"
	pullRequestRequired = "required"
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
	// Reason names the values the decision read.
	Reason string
	// Halt is a Stuck reason with its way forward, non-empty only when the landing must stop.
	Halt string
}

// ResolvePullRequest decides whether a landing goes through a pull request.
// parentMode is the receiving branch's beam.yaml pull_request (on, off or required) and entryField the landing entry's pull_request (empty, on or off); slug is the landing run's slug.
// A required parent wants one whatever the field says, an on parent wants one unless the field is off, and an off parent wants one only when the field is on.
// A field off under a required parent logs one Warn and still wants one.
// A wanted pull request needs, in this order, an origin on github.com, an origin ParseOwnerRepo accepts, an upstream for the parent and no push skip.
// A missing fact is a Halt when the parent is required or the field is on, and an origin on github.com that ParseOwnerRepo refuses is always a Halt; any other missing fact under an on parent alone is no pull request, naming the fact.
// It reads no file: the caller reads the parent's beam.yaml and the entry and tells the values.
func ResolvePullRequest(slug, parentMode, entryField string, origin Origin) PullRequestDecision {
	if parentMode == pullRequestRequired && entryField == pullRequestOff {
		logger.Warn("landingshed: the entry's pull_request is off but the parent's beam.yaml pull_request is required, so a pull request is still required", "slug", slug)
	}

	field := entryField
	if field == "" {
		field = "empty"
	}
	wanted := false
	demand := ""
	switch {
	case parentMode == pullRequestRequired:
		wanted, demand = true, "the parent's beam.yaml pull_request is required"
	case entryField == pullRequestOn:
		wanted, demand = true, "the entry's pull_request is on"
	case parentMode == pullRequestOn && entryField != pullRequestOff:
		wanted, demand = true, "the parent's beam.yaml pull_request is on"
	}
	if !wanted {
		return PullRequestDecision{Reason: fmt.Sprintf("pull request off: the parent's beam.yaml pull_request is %s and the entry's pull_request is %s", parentMode, field)}
	}

	owner, repo, missing, alwaysHalt := checkOrigin(origin)
	if missing == "" {
		return PullRequestDecision{Open: true, Reason: fmt.Sprintf("pull request on: the parent's beam.yaml pull_request is %s, the entry's pull_request is %s and the origin is %s/%s", parentMode, field, owner, repo)}
	}

	demanded := parentMode == pullRequestRequired || entryField == pullRequestOn
	if !demanded && !alwaysHalt {
		return PullRequestDecision{Reason: fmt.Sprintf("no pull request: the parent's beam.yaml pull_request is on and %s", missing)}
	}
	halt := fmt.Sprintf("a pull request is required here (%s) but %s; way forward: give the hub a GitHub origin, an upstream for the parent branch and no push skip, then run lym loom resume", demand, missing)
	if entryField == pullRequestOn && parentMode != pullRequestRequired {
		halt += fmt.Sprintf(", or run lym batten cancel %s and start a new run with the entry's pull_request changed", slug)
	}
	return PullRequestDecision{Halt: halt}
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
