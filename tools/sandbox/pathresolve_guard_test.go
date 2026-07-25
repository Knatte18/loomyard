// pathresolve_guard_test.go enforces the Dev/Prod Binary Separation Invariant: no
// non-test *.go file in the tools/sandbox package, other than resolve.go, may perform
// a bare-PATH "lyx" lookup or spawn -- lookPath("lyx"), exec.Command("lyx", or
// exec.CommandContext("lyx". resolve.go's resolveLyx is the single allowlisted
// resolution site; every other call site must route through it instead, so the
// dev/prod distinction can never silently regress to a bare PATH fallback. See
// CONSTRAINTS.md's Dev/Prod Binary Separation Invariant.

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

// bannedLyxTokens are the raw substrings a non-test *.go file in tools/sandbox may
// not contain outside pathResolveAllowlistFile.
var bannedLyxTokens = []string{
	`lookPath("lyx")`,
	`exec.Command("lyx"`,
	`exec.CommandContext("lyx"`,
}

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
		content := string(data)

		for _, token := range bannedLyxTokens {
			if strings.Contains(content, token) {
				failures = append(failures, fmt.Sprintf(
					"%s: contains banned bare-PATH lyx literal %q -- route through resolveLyx (resolve.go) instead",
					entry.Name(), token,
				))
				break
			}
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
