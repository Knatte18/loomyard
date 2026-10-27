//go:build tier0

// pathresolve_guard_test.go enforces the Dev/Prod Binary Separation Invariant: no non-test *.go file in the tools/sandbox package, other than resolve.go, may perform a bare-PATH "lym" lookup or spawn -- lookPath("lym"), or an exec.Command/ exec.CommandContext call whose line also names "lym".
// The exec forms are matched line-based rather than as a whole-file substring, because exec.CommandContext takes its context.Context argument first, so the literal exec.CommandContext("lym" never appears in compilable Go -- a substring scan for it can never match.
// resolve.go's resolveLym is the single allowlisted resolution site;
// every other call site must route through it instead, so the dev/prod distinction can never silently regress to a bare PATH fallback.
// The rule is stated in the command doc, doc.go.

package main

import (
	"fmt"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
)

// pathResolveAllowlist is the single file permitted to contain a banned bare-PATH lym literal.
var pathResolveAllowlist = []scankit.Entry{
	{Key: "tools/sandbox/resolve.go", Why: "the sole resolution site"},
}

// pathResolveMinFiles is the vacuous-scan floor for this guard's single-directory walk.
const pathResolveMinFiles = 3

// bareLymLookupLiteral is the one banned token matched as a whole-file substring.
const bareLymLookupLiteral = `lookPath("lym")`

// execSpawnTokens are the substrings identifying exec.Command or
// exec.CommandContext calls, matched line-based with the "lym" argument.
var execSpawnTokens = []string{"exec.Command", "exec.CommandContext"}

// TestPathResolveGuard_NoBarePathLymOutsideResolve fails if any non-test *.go file contains a
// banned bare-PATH lym literal.
func TestPathResolveGuard_NoBarePathLymOutsideResolve(t *testing.T) {
	allow := scankit.NewAllowlist(pathResolveAllowlist)

	var failures []string
	scanned := scankit.Walk(t, scankit.Options{Roots: []string{"tools/sandbox"}, Shallow: true}, func(f *scankit.File) {
		if allow.Allowed(f.Rel) {
			return
		}
		if token, bad := firstBannedLymToken(string(f.Data)); bad {
			failures = append(failures, fmt.Sprintf(
				"%s: contains banned bare-PATH lym literal %q -- route through resolveLym (resolve.go) instead",
				f.Rel, token,
			))
		}
	})

	scankit.RequireFloor(t, scanned, pathResolveMinFiles, "pathresolve guard")
	allow.RequireNoStale(t)

	if len(failures) > 0 {
		t.Errorf("dev/prod binary separation violated: resolve lym through resolveLym:\n%s", strings.Join(failures, "\n"))
	}
}

// firstBannedLymToken reports the first banned bare-PATH lym token found in
// content, scanning line by line.
func firstBannedLymToken(content string) (token string, bad bool) {
	for _, line := range strings.Split(content, "\n") {
		if strings.Contains(line, bareLymLookupLiteral) {
			return bareLymLookupLiteral, true
		}
		if token, bad := lineHasBannedLymSpawn(line); bad {
			return token, true
		}
	}
	return "", false
}

// lineHasBannedLymSpawn reports whether line contains both exec.Command/
// exec.CommandContext and the "lym" argument.
func lineHasBannedLymSpawn(line string) (token string, bad bool) {
	if !strings.Contains(line, `"lym"`) {
		return "", false
	}
	for _, spawnToken := range execSpawnTokens {
		if strings.Contains(line, spawnToken) {
			return spawnToken + ` ... "lym"`, true
		}
	}
	return "", false
}
