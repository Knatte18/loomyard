// hermeticenv_test.go enforces the Hermetic Git Test Environment Invariant.
// Every test package that spawns git, directly or through the gitkit and hubforge fixtures, must be hermetic.
// Such a package runs under gitkit.HermeticGitEnv() via a TestMain, or is named on an allowlist with a reason.
// It is the repo-wide grep guard for that invariant,
// and it machine-enforces what the two-layer hermetic mechanism otherwise relies on every new package remembering to do.
// See `PATTERN-test-isolation`.

package main

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// allowedNonHermetic is the Hermetic Git Test Environment Invariant allowlist, keyed two ways.
// A key ending in "/" names a directory.
// Its entry exempts the whole package and everything below it from the "git-spawning ⇒ hermetic" requirement.
// Those tests spawn non-git processes, for which a git-hermetic TestMain would be meaningless.
// A key ending in "_test.go" names one file.
// Its entry keeps that file's own tokens out of every package-level determination this guard makes.
// The file carries the guard's tokens as its own test data, not as real evidence.
// A file entry does not exempt its package.
var allowedNonHermetic = []scankit.Entry{
	{Key: "internal/proc/", Why: "spawns generic non-git processes — process control is the package's subject"},
	{Key: "cmd/lym/hermeticenv_test.go", Why: "this guard file itself; carries the tokens as its own test data"},
	{Key: "tools/sandbox/pathresolve_guard_test.go", Why: "contains the banned `exec.Command`/`exec.CommandContext` token strings as its own scan data (Dev/Prod Binary Separation guard)"},
	{Key: "internal/testkit/tmuxkit/", Why: "its tests spawn the tmux binary against the kit's own sockets, never git"},
	{Key: "internal/reedengine/attachgeometry_integration_test.go", Why: "spawns a real tmux/pty client process via exec.Command to prove the attach handover — a real non-git process, not a git spawn"},
	{Key: "internal/reedengine/testmain_guard_test.go", Why: "re-executes the test binary itself with a positional argument to prove TestMain's guard refuses it — a non-git process, not a git spawn"},
	{Key: "internal/termwindow/termwindow_integration_test.go", Why: "spawns bash over the window's shell line with a stub executable standing in for lym — a real non-git process, not a git spawn"},
}

// gitSpawnTokens are the raw substrings that mark a *_test.go file as git-spawning for the hermetic invariant.
// A gitkit spawn is not a token here; the gitkit definition in gitkitspawn_test.go matches it instead.
// hubforge.NewHub joins the set because it drives a real fabriccli.CloneAndWire clone internally.
// hubforge.SeedConfig and hubforge.SeedFabricConfig take a *Hub that only NewHub can produce,
// so the hubforge.NewHub token already covers every package that can reach them.
// hubforge.CopyHub and hubforge.SharedHub join it because each builds a real hub on first use.
var gitSpawnTokens = []string{
	"gitexec.Run",
	"exec.Command",
	"hubforge.NewHub",
	"hubforge.CopyHub",
	"hubforge.SharedHub",
}

// hermeticPresenceToken is the raw substring that proves a package runs under the hermetic git test environment.
// It is the bare helper name.
// That matches the qualified gitkit.HermeticGitEnv call and the unqualified HermeticGitEnv() call in gitkit's own tests.
// It proves presence only;
// the semantic half, a real TestMain that calls the helper before m.Run(), is a review obligation.
const hermeticPresenceToken = "HermeticGitEnv"

// pkgHermeticStatus holds, per package directory, the evidence the guard's walk collects across its *_test.go files.
// It records whether any file marks the package git-spawning, and which file and token did so for the failure message.
// It also records whether any file proves the hermetic presence token is there.
type pkgHermeticStatus struct {
	spawningFile  string
	spawningToken string
	hermetic      bool
}

// TestHermeticGitEnv_GitSpawningPackagesHaveTestMain walks every *_test.go file under the module root.
// It fails for each package that spawns git and lacks the hermetic presence token.
// A git-spawn token counts directly or via the gitkit fixture helpers.
// A package on the allowedNonHermetic allowlist is exempt.
// The walk reads every test file, whatever its tier or build constraint.
// The git-spawning set is almost exactly the tier1 set.
// So skipping the tiered files would make this guard vacuous.
//
//lym:guard
func TestHermeticGitEnv_GitSpawningPackagesHaveTestMain(t *testing.T) {
	tiergate.Require(t, 0)
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
			"%s: contains git-spawning token %q (in %s) but no test file in the package contains %s — add a testmain_test.go calling gitkit.HermeticGitEnv(), or add an allowedNonHermetic entry in cmd/lym/hermeticenv_test.go with a reason",
			dir, status.spawningToken, status.spawningFile, hermeticPresenceToken,
		))
	}
	// Map iteration order is random; sort the failures so the message is stable.
	sort.Strings(failures)

	// Vacuous-scan protection: finding no git-spawning package means the walk is misconfigured.
	if gitSpawningCount == 0 {
		t.Fatal("hermetic git env guard: found zero git-spawning packages — the walk may be misconfigured")
	}
	excludedFiles.RequireNoStale(t)
	exemptPackages.RequireNoStale(t)

	if len(failures) > 0 {
		t.Errorf("`PATTERN-test-isolation` violated:\n%s", strings.Join(failures, "\n"))
	}
}

// firstSpawnToken returns the first git-spawn token found as a raw substring of content, and whether one was found.
// Plain tokens are checked in declared order before the gitkit fixture spawn reference.
func firstSpawnToken(content string) (string, bool) {
	for _, token := range gitSpawnTokens {
		if strings.Contains(content, token) {
			return token, true
		}
	}
	return gitkitSpawnReference(content)
}
