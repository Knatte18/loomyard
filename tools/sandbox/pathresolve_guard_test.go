// pathresolve_guard_test.go enforces the Dev/Prod Binary Separation Invariant: no non-test *.go
// file in the tools/sandbox package, other than resolve.go, may perform a bare-PATH "lyx" lookup or
// spawn -- lookPath("lyx"), or an exec.Command/ exec.CommandContext call whose line also names
// "lyx".
// The exec forms are matched line-based rather than as a whole-file substring, because
// exec.CommandContext takes its context.Context argument first, so the literal
// exec.CommandContext("lyx" never appears in compilable Go -- a substring scan for it can never
// match.
// resolve.go's resolveLyx is the single allowlisted resolution site;
// every other call site must route through it instead, so the dev/prod distinction can never
// silently regress to a bare PATH fallback.
// See `PATTERN-dev-prod-binary-separation`.

package main

import (
	"fmt"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
)

// pathResolveAllowlist is the single file permitted to contain a banned bare-PATH lyx literal.
var pathResolveAllowlist = []scankit.Entry{
	{Key: "tools/sandbox/resolve.go", Why: "the sole resolution site"},
}

// pathResolveMinFiles is the vacuous-scan floor for this guard's single-directory walk.
const pathResolveMinFiles = 3

// bareLyxLookupLiteral is the one banned token matched as a whole-file substring.
const bareLyxLookupLiteral = `lookPath("lyx")`

// execSpawnTokens are the substrings identifying exec.Command or
// exec.CommandContext calls, matched line-based with the "lyx" argument.
var execSpawnTokens = []string{"exec.Command", "exec.CommandContext"}

// TestPathResolveGuard_NoBarePathLyxOutsideResolve fails if any non-test *.go file contains a
// banned bare-PATH lyx literal.
func TestPathResolveGuard_NoBarePathLyxOutsideResolve(t *testing.T) {
	allow := scankit.NewAllowlist(pathResolveAllowlist)

	var failures []string
	scanned := scankit.Walk(t, scankit.Options{Roots: []string{"tools/sandbox"}, Shallow: true}, func(f *scankit.File) {
		if allow.Allowed(f.Rel) {
			return
		}
		if token, bad := firstBannedLyxToken(string(f.Data)); bad {
			failures = append(failures, fmt.Sprintf(
				"%s: contains banned bare-PATH lyx literal %q -- route through resolveLyx (resolve.go) instead",
				f.Rel, token,
			))
		}
	})

	scankit.RequireFloor(t, scanned, pathResolveMinFiles, "pathresolve guard")
	allow.RequireNoStale(t)

	if len(failures) > 0 {
		t.Errorf("`PATTERN-dev-prod-binary-separation` violated:\n%s", strings.Join(failures, "\n"))
	}
}

// firstBannedLyxToken reports the first banned bare-PATH lyx token found in
// content, scanning line by line.
func firstBannedLyxToken(content string) (token string, bad bool) {
	for _, line := range strings.Split(content, "\n") {
		if strings.Contains(line, bareLyxLookupLiteral) {
			return bareLyxLookupLiteral, true
		}
		if token, bad := lineHasBannedLyxSpawn(line); bad {
			return token, true
		}
	}
	return "", false
}

// lineHasBannedLyxSpawn reports whether line contains both exec.Command/
// exec.CommandContext and the "lyx" argument.
func lineHasBannedLyxSpawn(line string) (token string, bad bool) {
	if !strings.Contains(line, `"lyx"`) {
		return "", false
	}
	for _, spawnToken := range execSpawnTokens {
		if strings.Contains(line, spawnToken) {
			return spawnToken + ` ... "lyx"`, true
		}
	}
	return "", false
}
