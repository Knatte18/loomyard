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
// bare-PATH lyx literal: resolve.go, the sole resolution site.
const pathResolveAllowlistFile = "resolve.go"

// bareLyxLookupLiteral is the one banned token matched as a whole-file substring.
const bareLyxLookupLiteral = `lookPath("lyx")`

// execSpawnTokens are the substrings identifying exec.Command or
// exec.CommandContext calls, matched line-based with the "lyx" argument.
var execSpawnTokens = []string{"exec.Command", "exec.CommandContext"}

// TestPathResolveGuard_NoBarePathLyxOutsideResolve fails if any non-test *.go
// file contains a banned bare-PATH lyx literal.
func TestPathResolveGuard_NoBarePathLyxOutsideResolve(t *testing.T) {
	dir := sandboxSourceDir(t)

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read sandbox source dir %s: %v", dir, err)
	}

	var scanned int
	var failures []string
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		scanned++

		if entry.Name() == pathResolveAllowlistFile {
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

	if scanned < 3 {
		t.Fatalf("pathresolve guard: only scanned %d non-test .go file(s) in %s; expected at least 3 -- the directory read may be misconfigured", scanned, dir)
	}

	if len(failures) > 0 {
		t.Errorf("Dev/Prod Binary Separation Invariant violated (see CONSTRAINTS.md):\n%s", strings.Join(failures, "\n"))
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

// sandboxSourceDir returns the tools/sandbox package directory, derived from
// this test file's location.
func sandboxSourceDir(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate pathresolve_guard_test.go source file")
	}
	return filepath.Dir(thisFile)
}
