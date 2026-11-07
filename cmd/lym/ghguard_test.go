// ghguard_test.go enforces the GitHub Auth Invariant's shell-out half:
// no production (non-test) package outside internal/githubclient may shell out to the `gh` CLI.
// internal/githubclient owns the one bounded, timeout-guarded `gh auth token` shell-out (token.go);
// every other package must go through it rather than growing its own credential path.
// See `PATTERN-github-auth`.

package main

import (
	"fmt"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// ghGuardAllowlist names the single package directory permitted to shell out to `gh`.
// It is the one place the GitHub Auth Invariant designates as owning token resolution.
var ghGuardAllowlist = []scankit.Entry{
	{Key: "internal/githubclient/", Why: "owns the one bounded `gh auth token` shell-out"},
}

// ghExecSpawnTokens are the substrings that identify an exec.Command or exec.CommandContext call.
// Each token is matched only on the same line as the quoted "gh" argument, never as a whole-file substring.
// exec.CommandContext takes its context.Context argument first,
// so the literal `exec.CommandContext("gh"` never appears in compilable Go, and a whole-file scan for it can never match.
var ghExecSpawnTokens = []string{"exec.Command", "exec.CommandContext"}

// ghLookPathLiteral is the banned token matched as a whole-line substring.
// A direct bare LookPath("gh") call has no argument-order ambiguity, so the plain substring is enough.
const ghLookPathLiteral = `LookPath("gh")`

// TestGHGuard_NoShellOutOutsideGithubclient walks every non-test *.go file under the module root.
// It fails if any file outside the ghGuardAllowlist directory contains a banned `gh` shell-out token.
// A bare "gh" substring cannot be a banned token: it matches words such as "through", "right" and "highlight".
// Both banned forms therefore carry the quoted binary name, so no English word can match.
//
//lym:guard
func TestGHGuard_NoShellOutOutsideGithubclient(t *testing.T) {
	tiergate.Require(t, 0)
	allow := scankit.NewAllowlist(ghGuardAllowlist)
	var failures []string

	scanned := scankit.Walk(t, scankit.Options{}, func(f *scankit.File) {
		if allow.Allowed(f.Rel) {
			return
		}
		if token, bad := firstBannedGHToken(string(f.Data)); bad {
			failures = append(failures, fmt.Sprintf(
				"%s: contains banned gh shell-out token %q -- route through internal/githubclient instead",
				f.Rel, token,
			))
		}
	})

	scankit.RequireFloor(t, scanned, 20, "gh guard")
	allow.RequireNoStale(t)

	if len(failures) > 0 {
		t.Errorf("`PATTERN-github-auth` violated:\n%s", strings.Join(failures, "\n"))
	}
}

// firstBannedGHToken reports the first banned gh shell-out token found in content.
func firstBannedGHToken(content string) (token string, bad bool) {
	for _, line := range strings.Split(content, "\n") {
		if strings.Contains(line, ghLookPathLiteral) {
			return ghLookPathLiteral, true
		}
		if token, bad := lineHasBannedGHSpawn(line); bad {
			return token, true
		}
	}
	return "", false
}

// lineHasBannedGHSpawn reports whether line contains both a spawn token and "gh" on the same line.
func lineHasBannedGHSpawn(line string) (token string, bad bool) {
	if !strings.Contains(line, `"gh"`) {
		return "", false
	}
	for _, spawnToken := range ghExecSpawnTokens {
		if strings.Contains(line, spawnToken) {
			return spawnToken + ` ... "gh"`, true
		}
	}
	return "", false
}

// pullRequestMutationCalls are go-github's calls that open, edit, merge or close a pull request.
var pullRequestMutationCalls = []string{".PullRequests.Create(", ".PullRequests.Edit(", ".PullRequests.Merge("}

// issueEditCall is go-github's call that closes an issue;
// since a pull request is an issue, the call also closes a pull request.
const issueEditCall = ".Issues.Edit("

// pullRequestCallAllowlist names the one package directory permitted to make go-github's pull request mutating calls.
var pullRequestCallAllowlist = []scankit.Entry{
	{Key: "internal/pullrequest/", Why: "owns the run's pull request calls"},
}

// issueEditAllowlist names the package directories permitted to close an issue through go-github, each with its reason.
var issueEditAllowlist = []scankit.Entry{
	{Key: "internal/pullrequest/", Why: "closes a pull request through its issue"},
	{Key: "internal/hubissues/", Why: "closes a taken-in hub issue"},
}

// pullRequestImportAllowlist names the package directories permitted to import internal/pullrequest.
var pullRequestImportAllowlist = []scankit.Entry{
	{Key: "internal/landingshed/", Why: "PR-Review and Finalize hold the API"},
	{Key: "internal/pairteardown/", Why: "a teardown closes the run's open pull request"},
	{Key: "internal/loomcli/", Why: "wires the producers' seams and owns the pr-continue verb"},
	{Key: "internal/beamcli/", Why: "`lym beam pr` opens, edits and reopens a beam's pull request and never merges one"},
	{Key: "internal/testkit/envkit/", Why: "its LandingDeps stub names pullrequest.API in the API seam's func literal"},
	{Key: "internal/testkit/prkit/", Why: "the shared fake of pullrequest.API"},
}

// pullRequestImportLine is the import line of internal/pullrequest.
const pullRequestImportLine = `"github.com/Knatte18/loomyard/internal/pullrequest"`

// TestPullRequestGuard_OnlyLandingAndTeardown enforces the pull request clause of `PATTERN-github-auth`.
// It walks every non-test *.go file under the module root.
// Only internal/pullrequest calls go-github's mutating pull request calls.
// Only it and internal/hubissues call the issue edit.
// Only the allowlisted directories import internal/pullrequest.
// So only they can open, merge or close a pull request through it.
// The read-only `.PullRequests.List(` calls open, merge and close nothing and stay allowed.
//
//lym:guard
func TestPullRequestGuard_OnlyLandingAndTeardown(t *testing.T) {
	tiergate.Require(t, 0)
	calls := scankit.NewAllowlist(pullRequestCallAllowlist)
	edits := scankit.NewAllowlist(issueEditAllowlist)
	imports := scankit.NewAllowlist(pullRequestImportAllowlist)
	var failures []string

	scanned := scankit.Walk(t, scankit.Options{}, func(f *scankit.File) {
		for _, line := range strings.Split(string(f.Data), "\n") {
			if strings.HasPrefix(strings.TrimSpace(line), "//") {
				continue
			}
			for _, call := range pullRequestMutationCalls {
				if strings.Contains(line, call) && !calls.Allowed(f.Rel) {
					failures = append(failures, fmt.Sprintf("%s: calls %s -- route it through internal/pullrequest instead", f.Rel, call))
				}
			}
			if strings.Contains(line, issueEditCall) && !edits.Allowed(f.Rel) {
				failures = append(failures, fmt.Sprintf("%s: calls %s -- a pull request closes through its issue, so route it through internal/pullrequest instead", f.Rel, issueEditCall))
			}
			if strings.Contains(line, pullRequestImportLine) && !imports.Allowed(f.Rel) {
				failures = append(failures, fmt.Sprintf("%s: imports internal/pullrequest -- only PR-Review, Finalize and pair teardown, with the wiring that serves them, may open, merge or close a pull request", f.Rel))
			}
		}
	})

	scankit.RequireFloor(t, scanned, 20, "pull request guard")
	calls.RequireNoStale(t)
	// edits is left out of the stale check:
	// internal/pullrequest holds an entry for a close through the issue that its client does not make.
	imports.RequireNoStale(t)

	if len(failures) > 0 {
		t.Errorf("`PATTERN-github-auth` violated:\n%s", strings.Join(failures, "\n"))
	}
}
