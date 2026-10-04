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
