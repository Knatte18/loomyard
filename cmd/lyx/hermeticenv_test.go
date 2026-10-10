// hermeticenv_test.go enforces the Hermetic Git Test Environment Invariant: every test package
// whose tests spawn git — directly or via the fixture helpers now living in gitkit and hubforge —
// must run under gitkit.HermeticGitEnv(), wired via a TestMain, or be named on an allowlist with a
// reason.
// This is the repo-wide grep-guard companion to tierpurity_test.go, machine-enforcing what the
// two-layer hermetic mechanism otherwise relies on every new package remembering to do.
// See `PATTERN-test-isolation`.

package main

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
)

// allowedNonHermetic is the Hermetic Git Test Environment Invariant allowlist, with
// two distinct entry kinds distinguished by whether the key names a directory
// (a path ending in "/", matching the package and everything below it) or a single *_test.go file:
//
//   - A directory entry (trailing "/") exempts the whole package from the
//     "git-spawning ⇒ hermetic" requirement — the package's tests genuinely spawn
//     non-git processes for which a git-hermetic TestMain would be meaningless.
//   - A file entry (exact match, ending in "_test.go") is a per-file scan
//     exclusion: it keeps that one file's own token content out of every
//     package-level determination this guard makes (both "is this package
//     git-spawning" and "does this package already contain the hermetic
//     presence token"), because the file carries the guard's tokens as its own
//     test data rather than as real evidence. It is NOT a package-level
//     exemption — see hermeticenv_test.go's own entry below.
var allowedNonHermetic = []scankit.Entry{
	{Key: "internal/proc/", Why: "spawns generic non-git processes — process control is the package's subject"},
	{Key: "cmd/lyx/hermeticenv_test.go", Why: "this guard file itself; carries the tokens as its own test data"},
	{Key: "tools/sandbox/pathresolve_guard_test.go", Why: "contains the banned `exec.Command`/`exec.CommandContext` token strings as its own scan data (Dev/Prod Binary Separation guard)"},
	{Key: "internal/testkit/tmuxkit/", Why: "its tests spawn the tmux binary against the kit's own sockets, never git"},
	{Key: "internal/reedengine/attachgeometry_integration_test.go", Why: "spawns a real tmux/pty client process via exec.Command to prove the attach handover — a real non-git process, not a git spawn"},
}

// gitSpawnTokens are the raw substrings that mark a *_test.go file as git-spawning for the Hermetic Git Test Environment Invariant.
// A gitkit spawn is not a token here:
// gitkitSpawnReference (cmd/lyx/gitkitspawn_test.go) defines it once, shared with tierpurity_test.go,
// and firstSpawnToken consults it after these tokens.
// hubforge.NewHub joins the set because it drives a real fabriccli.CloneAndWire clone internally,
// and hubforge.SeedConfig/SeedFabricConfig both take a *Hub only NewHub can produce,
// so this one token already covers every package that can reach any of the three.
// hubforge.CopyHub and hubforge.SharedHub join it because each builds a real hub on first use.
var gitSpawnTokens = []string{
	"gitexec.Run",
	"exec.Command",
	"hubforge.NewHub",
	"hubforge.CopyHub",
	"hubforge.SharedHub",
}

// hermeticPresenceToken is the raw substring proving a package runs under the
// hermetic git test environment: the bare, unqualified helper name. A bare-name
// match (rather than the qualified "gitkit.HermeticGitEnv" form) is deliberate —
// it matches both the qualified call form used by other packages and the
// unqualified HermeticGitEnv() form gitkit's own package-gitkit tests use (see
// the helper-name-HermeticGitEnv Shared Decision). This proves presence only — the
// mechanical half of the check. The semantic half (a real TestMain that calls the
// helper before m.Run()) is a review obligation, exactly like the repo's other
// grep-guards (`PATTERN-shell-mechanics-seam` and `PATTERN-shuttle-provider-seam`).
const hermeticPresenceToken = "HermeticGitEnv"

// pkgHermeticStatus accumulates, per package directory, the evidence the guard's
// walk collects across that directory's *_test.go files: whether any file marks
// the package git-spawning (and which file/token triggered it, for the failure
// message), and whether any file in the package proves the hermetic presence
// token is there.
type pkgHermeticStatus struct {
	spawningFile  string
	spawningToken string
	hermetic      bool
}

// TestHermeticGitEnv_GitSpawningPackagesHaveTestMain walks every *_test.go file under the module
// root and fails if any package whose test files contain a git-spawn token (directly or via the
// gitkit fixture helpers) lacks the hermetic presence token anywhere in its test files, unless the
// package is on the allowedNonHermetic allowlist.
// Unlike TestTierPurity_UntaggedTestsSpawnNothing, which only scans untagged files (its subject is
// Tier 1's offline guarantee), this guard scans every *_test.go file regardless of build
// constraint: the git-spawning set is almost exactly the integration-tagged set, so skipping tagged
// files the way tierpurity does would make this guard vacuous.
//
//lyx:guard
func TestHermeticGitEnv_GitSpawningPackagesHaveTestMain(t *testing.T) {
	var fileEntries, dirEntries []scankit.Entry
	for _, e := range allowedNonHermetic {
		if strings.HasSuffix(e.Key, "_test.go") {
			fileEntries = append(fileEntries, e)
		} else {
			dirEntries = append(dirEntries, e)
		}
	}
	excludedFiles := scankit.NewAllowlist(fileEntries)
	exemptPackages := scankit.NewAllowlist(dirEntries)
	packages := map[string]*pkgHermeticStatus{}

	scanned := scankit.Walk(t, scankit.Options{Filter: scankit.Test}, func(f *scankit.File) {
		// A file-level allowlist entry is excluded from content-based evidence entirely.
		if excludedFiles.Allowed(f.Rel) {
			return
		}

		content := string(f.Data)

		dir := filepath.ToSlash(filepath.Dir(f.Rel))
		status := packages[dir]
		if status == nil {
			status = &pkgHermeticStatus{}
			packages[dir] = status
		}

		if status.spawningFile == "" {
			if token, bad := firstSpawnToken(content); bad {
				status.spawningFile = f.Rel
				status.spawningToken = token
			}
		}
		if strings.Contains(content, hermeticPresenceToken) {
			status.hermetic = true
		}
	})
	scankit.RequireFloor(t, scanned, 1, "hermetic git env guard")

	var gitSpawningCount int
	var failures []string
	for dir, status := range packages {
		if status.spawningFile == "" {
			continue
		}
		gitSpawningCount++
		if status.hermetic {
			continue
		}
		if exemptPackages.Allowed(dir + "/") {
			continue
		}
		failures = append(failures, fmt.Sprintf(
			"%s: contains git-spawning token %q (in %s) but no test file in the package contains %s — add a testmain_test.go calling gitkit.HermeticGitEnv(), or add an allowedNonHermetic entry in cmd/lyx/hermeticenv_test.go with a reason",
			dir, status.spawningToken, status.spawningFile, hermeticPresenceToken,
		))
	}
	// Ensure deterministic failure message ordering.
	sort.Strings(failures)

	// Vacuous-scan protection: fewer than zero git-spawning packages means misconfiguration.
	if gitSpawningCount == 0 {
		t.Fatal("hermetic git env guard: found zero git-spawning packages — the walk may be misconfigured")
	}
	excludedFiles.RequireNoStale(t)
	exemptPackages.RequireNoStale(t)

	if len(failures) > 0 {
		t.Errorf("`PATTERN-test-isolation` violated:\n%s", strings.Join(failures, "\n"))
	}
}

// firstSpawnToken returns the first entry of gitSpawnTokens (in declared order) that appears as a raw substring of content, else the first gitkit spawn reference, and whether any was found.
func firstSpawnToken(content string) (string, bool) {
	for _, token := range gitSpawnTokens {
		if strings.Contains(content, token) {
			return token, true
		}
	}
	return gitkitSpawnReference(content)
}
