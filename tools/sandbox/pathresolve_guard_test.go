// pathresolve_guard_test.go enforces the dev/prod binary separation in tools/sandbox.
// No non-test *.go file there may look up "lym" on PATH or spawn it by exec, except resolve.go.
// Code that needs lym routes the lookup through resolveLym, so a dev run never silently falls back to PATH.

package main

import (
	"fmt"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// pathResolveAllowlist is the single file permitted to contain a banned bare-PATH lym literal.
var pathResolveAllowlist = []scankit.Entry{
	{Key: "tools/sandbox/resolve.go", Why: "the sole resolution site"},
}

// pathResolveMinFiles is the minimum number of files the walk must scan, so an empty walk cannot pass vacuously.
const pathResolveMinFiles = 3

// bareLymLookupLiteral is the one banned token matched as a whole-file substring.
const bareLymLookupLiteral = `lookPath("lym")`

// execSpawnTokens are the substrings that mark an exec.Command or exec.CommandContext call.
// The guard matches them line by line, together with the "lym" argument.
var execSpawnTokens = []string{"exec.Command", "exec.CommandContext"}

// TestPathResolveGuard_NoBarePathLymOutsideResolve fails when a non-test *.go file holds a banned bare-PATH lym literal.
func TestPathResolveGuard_NoBarePathLymOutsideResolve(t *testing.T) {
	tiergate.Require(t, 0)
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

// firstBannedLymToken reports the first banned bare-PATH lym token in content, scanning line by line.
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

// lineHasBannedLymSpawn reports whether line calls exec.Command or exec.CommandContext with the "lym" argument.
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
