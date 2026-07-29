// pathresolve_guard_test.go enforces the Dev/Prod Binary Separation Invariant: no
// non-test *.go file in the tools/sandbox package, other than resolve.go, may perform
// a bare-PATH "lyx" lookup or spawn -- lookPath("lyx"), or an exec.Command/
// exec.CommandContext call whose line also names "lyx". The exec forms are matched
// line-based rather than as a whole-file substring, because exec.CommandContext takes
// its context.Context argument first, so the literal exec.CommandContext("lyx" never
// appears in compilable Go -- a substring scan for it can never match. resolve.go's
// resolveLyx is the single allowlisted resolution site; every other call site must
// route through it instead, so the dev/prod distinction can never silently regress to
// a bare PATH fallback. See CONSTRAINTS.md's Dev/Prod Binary Separation Invariant.

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// pathResolveAllowlistFile is the single file permitted to contain a banned
// bare-PATH lyx literal: it is resolve.go itself, the sole resolution site the
// Dev/Prod Binary Separation Invariant designates.
const pathResolveAllowlistFile = "resolve.go"

// bareLyxLookupLiteral is the one banned token still matched as a whole-file
// substring: a direct bare-PATH lookPath call, which has no argument-order
// ambiguity to worry about.
const bareLyxLookupLiteral = `lookPath("lyx")`

// execSpawnTokens are the substrings identifying an exec.Command or
// exec.CommandContext call. They are matched on the SAME LINE as the quoted
// "lyx" argument (see lineHasBannedLyxSpawn) rather than as a whole-file
// substring together with the argument: exec.CommandContext's first
// parameter is a context.Context, not the binary name, so the literal
// `exec.CommandContext("lyx"` can never appear in compilable Go — a
// substring scan for it can never match, silently exempting the exact call
// shape the Dev/Prod Binary Separation Invariant most needs to catch.
// Line-based matching finds both spellings regardless of how many arguments
// precede "lyx".
var execSpawnTokens = []string{"exec.Command", "exec.CommandContext"}

// TestPathResolveGuard_NoBarePathLyxOutsideResolve walks every non-test *.go file in
// the tools/sandbox package directory and fails if any file other than
// pathResolveAllowlistFile contains a banned bare-PATH lyx literal.
func TestPathResolveGuard_NoBarePathLyxOutsideResolve(t *testing.T) {
	dir := sandboxSourceDir(t)

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read sandbox source dir %s: %v", dir, err)
	}

	var scanned int
	var failures []string
	for _, entry := range entries {
		// Only non-test *.go files are in scope: this guard's own source is a
		// *_test.go file and is excluded by the same filter, with no special-case
		// needed.
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		scanned++

		if entry.Name() == pathResolveAllowlistFile {
			// resolve.go is the designated resolution site; its own bare-PATH
			// lookup is the invariant's implementation, not a violation of it.
			continue
		}

		data, readErr := os.ReadFile(filepath.Join(dir, entry.Name()))
		if readErr != nil {
			t.Fatalf("read %s: %v", entry.Name(), readErr)
		}

		if token, bad := firstBannedLyxToken(string(data)); bad {
			failures = append(failures, fmt.Sprintf(
				"%s: contains banned bare-PATH lyx literal %q -- route through resolveLyx (resolve.go) instead",
				entry.Name(), token,
			))
		}
	}

	// Vacuous-scan protection: tools/sandbox has 4 non-test .go files today
	// (main.go, report.go, resolve.go, suite.go); fewer than 3 found means the
	// directory read is misconfigured (wrong dir, files missing) rather than the
	// package having genuinely shrunk below the resolution split it was built for.
	if scanned < 3 {
		t.Fatalf("pathresolve guard: only scanned %d non-test .go file(s) in %s; expected at least 3 -- the directory read may be misconfigured", scanned, dir)
	}

	if len(failures) > 0 {
		t.Errorf("Dev/Prod Binary Separation Invariant violated (see CONSTRAINTS.md):\n%s", strings.Join(failures, "\n"))
	}
}

// firstBannedLyxToken reports the first banned bare-PATH lyx token found in content,
// scanning line by line so the exec.Command/exec.CommandContext check can require the
// spawn token and the quoted "lyx" argument to co-occur on one line rather than
// merely anywhere in the file. The lookPath("lyx") literal needs no such pairing --
// it has no argument-order ambiguity -- so it is checked directly per line too, for a
// single unified scan pass.
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

// lineHasBannedLyxSpawn reports whether line contains both an exec.Command/
// exec.CommandContext spawn token and the quoted "lyx" argument -- the pairing that
// identifies a bare-PATH lyx spawn regardless of how many arguments (e.g. a leading
// context.Context) separate the two tokens on the line.
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

// sandboxSourceDir returns the tools/sandbox package directory, derived from this
// test file's own location via runtime.Caller rather than a hardcoded absolute
// path, so the guard works from any checkout or worktree.
func sandboxSourceDir(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate pathresolve_guard_test.go source file")
	}
	return filepath.Dir(thisFile)
}
