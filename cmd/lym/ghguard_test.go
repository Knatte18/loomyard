//go:build tier0

// ghguard_test.go enforces the GitHub Auth Invariant's shell-out half: no production (non-test)
// package outside internal/githubclient may shell out to the `gh` CLI.
// internal/githubclient owns the one bounded, timeout-guarded `gh auth token` shell-out (token.go);
// every other package must go through it rather than growing its own credential path.
// See `PATTERN-github-auth`.

package main

import (
	"fmt"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
)

// ghGuardAllowlist names the single package directory permitted to shell out to
// `gh`: the one place the GitHub Auth Invariant designates as owning token
// resolution.
var ghGuardAllowlist = []scankit.Entry{
	{Key: "internal/githubclient/", Why: "owns the one bounded `gh auth token` shell-out"},
}

// ghExecSpawnTokens are the substrings identifying an exec.Command or
// exec.CommandContext call, matched on the SAME LINE as the quoted "gh" argument
// (see lineHasBannedGHSpawn) rather than as a whole-file substring together with
// the argument. exec.CommandContext takes its context.Context argument first, so
// the literal exec.CommandContext("gh" never appears in compilable Go -- exactly
// the call shape internal/githubclient/token.go itself uses -- and a whole-file
// substring scan for it can never match.
var ghExecSpawnTokens = []string{"exec.Command", "exec.CommandContext"}

// ghLookPathLiteral is the one banned token still matched as a whole-line
// substring: a direct bare LookPath("gh") call, which has no argument-order
// ambiguity to worry about.
const ghLookPathLiteral = `LookPath("gh")`

// TestGHGuard_NoShellOutOutsideGithubclient walks every non-test *.go file under the module root
// and fails if any file outside the ghGuardAllowlist directory contains a banned `gh` shell-out token.
// A bare "gh" substring is unusable as a banned token -- it matches "through", "right", "highlight"
// and hundreds of other words repo-wide -- so, following tools/sandbox/pathresolve_guard_test.go's
// precedent, both banned forms carry the quoted binary name so no English word can match.
//
//lym:guard
func TestGHGuard_NoShellOutOutsideGithubclient(t *testing.T) {
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

// issueEditCall is go-github's call that closes an issue, and so a pull request, since a pull request is an issue.
const issueEditCall = ".Issues.Edit("

// pullRequestCallAllowlist names the one package directory permitted to open, edit, merge or close a pull request through go-github.
var pullRequestCallAllowlist = []scankit.Entry{
	{Key: "internal/pullrequest/", Why: "owns the run's pull request calls"},
}

// issueEditAllowlist names the package directories permitted to close an issue through go-github: the pull request client, and the hub's issue intake, which closes issues only.
var issueEditAllowlist = []scankit.Entry{
	{Key: "internal/pullrequest/", Why: "closes a pull request through its issue"},
	{Key: "internal/hubissues/", Why: "closes a taken-in hub issue"},
}

// pullRequestImportAllowlist names the package directories permitted to import internal/pullrequest: the two producers that hold its API, the CLI that wires their seams and the `pr-continue` verb, and the kit whose landing stub names the API in a func literal.
var pullRequestImportAllowlist = []scankit.Entry{
	{Key: "internal/landingshed/", Why: "PR-Review and Finalize hold the API"},
	{Key: "internal/pairteardown/", Why: "a teardown closes the run's open pull request"},
	{Key: "internal/loomcli/", Why: "wires the producers' seams and owns the pr-continue verb"},
	{Key: "internal/testkit/envkit/", Why: "its LandingDeps stub names pullrequest.API in the API seam's func literal"},
}

// pullRequestImportLine is the import line of internal/pullrequest.
const pullRequestImportLine = `"github.com/Knatte18/loomyard/internal/pullrequest"`

// TestPullRequestGuard_OnlyLandingAndTeardown walks every non-test *.go file under the module root and enforces the pull request clause of `PATTERN-github-auth`.
// Only internal/pullrequest calls go-github's mutating pull request calls, only it and internal/hubissues call the issue edit, and only the allowlisted directories import internal/pullrequest, so only PR-Review, Finalize and pair teardown open, merge or close one.
// The read-only `.PullRequests.List(` calls open, merge and close nothing and stay allowed.
//
//lym:guard
func TestPullRequestGuard_OnlyLandingAndTeardown(t *testing.T) {
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
	// edits is left out of the stale check: internal/pullrequest holds an entry for a close through the issue that its client does not make today.
	imports.RequireNoStale(t)

	if len(failures) > 0 {
		t.Errorf("`PATTERN-github-auth` violated:\n%s", strings.Join(failures, "\n"))
	}
}
