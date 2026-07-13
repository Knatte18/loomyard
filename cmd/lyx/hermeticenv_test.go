// hermeticenv_test.go enforces the Hermetic Git Test Environment Invariant: every
// test package whose tests spawn git — directly or via the lyxtest fixture helpers —
// must run under lyxtest.HermeticGitEnv(), wired via a TestMain, or be named on an
// allowlist with a reason. This is the repo-wide grep-guard companion to
// tierpurity_test.go, machine-enforcing what the two-layer hermetic mechanism
// otherwise relies on every new package remembering to do. See CONSTRAINTS.md's
// Hermetic Git Test Environment Invariant.

package main

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// allowedNonHermetic is the Hermetic Git Test Environment Invariant allowlist, with
// two distinct entry kinds distinguished by whether the key names a directory
// (path or path-prefix) or a single *_test.go file:
//
//   - A directory entry (no trailing ".go") exempts the whole package from the
//     "git-spawning ⇒ hermetic" requirement — the package's tests genuinely spawn
//     non-git processes for which a git-hermetic TestMain would be meaningless.
//   - A file entry (exact match, ending in "_test.go") is a per-file scan
//     exclusion: it keeps that one file's own token content out of every
//     package-level determination this guard makes (both "is this package
//     git-spawning" and "does this package already contain the hermetic
//     presence token"), because the file carries the guard's tokens as its own
//     test data rather than as real evidence. It is NOT a package-level
//     exemption — see hermeticenv_test.go's own entry below.
var allowedNonHermetic = map[string]string{
	"internal/proc":               "spawns generic non-git processes — process control is the package's subject",
	"cmd/lyx/hermeticenv_test.go": "this guard file itself; carries the tokens as its own test data",
}

// gitSpawnTokens are the raw substrings that mark a *_test.go file as git-spawning
// for the Hermetic Git Test Environment Invariant. This set is broader than
// tierpurity_test.go's bannedTokens: it adds lyxtest.MustRun and lyxtest.SeedConfig,
// which spawn git internally inside lyxtest itself — without them, a package whose
// only git spawn goes through those helpers would carry no matching token and
// silently skip the hermetic requirement.
var gitSpawnTokens = []string{
	"gitexec.RunGit",
	"exec.Command",
	"lyxtest.Copy",
	"lyxtest.MustRun",
	"lyxtest.SeedConfig",
}

// hermeticPresenceToken is the raw substring proving a package runs under the
// hermetic git test environment: the bare, unqualified helper name. A bare-name
// match (rather than the qualified "lyxtest.HermeticGitEnv" form) is deliberate —
// it matches both the qualified call form used by other packages and the
// unqualified HermeticGitEnv() form lyxtest's own package-lyxtest tests use (see
// the helper-name-HermeticGitEnv Shared Decision). This proves presence only — the
// mechanical half of the check. The semantic half (a real TestMain that calls the
// helper before m.Run()) is a review obligation, exactly like the repo's other
// grep-guards (the Shell Mechanics Seam and Provider-Seam entries in
// CONSTRAINTS.md).
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

// TestHermeticGitEnv_GitSpawningPackagesHaveTestMain walks every *_test.go file
// under the module root and fails if any package whose test files contain a
// git-spawn token (directly or via the lyxtest fixture helpers) lacks the
// hermetic presence token anywhere in its test files, unless the package is on
// the allowedNonHermetic allowlist. Unlike TestTierPurity_UntaggedTestsSpawnNothing,
// which only scans untagged files (its subject is Tier 1's offline guarantee),
// this guard scans every *_test.go file regardless of build constraint: the
// git-spawning set is almost exactly the integration-tagged set, so skipping
// tagged files the way tierpurity does would make this guard vacuous.
func TestHermeticGitEnv_GitSpawningPackagesHaveTestMain(t *testing.T) {
	// Skip cleanly rather than fail when the go toolchain is not on PATH, mirroring
	// tierpurity_test.go and crosscompile_test.go so this gate never blocks a
	// minimal environment.
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not on PATH")
	}

	// Resolve the module root via `go env GOMOD` rather than assuming the test's
	// working directory, exactly as tierpurity_test.go does, so the walk is
	// cwd-independent.
	out, err := exec.Command("go", "env", "GOMOD").CombinedOutput()
	if err != nil {
		t.Fatalf("go env GOMOD failed: %v\n%s", err, out)
	}
	goMod := strings.TrimSpace(string(out))
	if goMod == "" || goMod == os.DevNull {
		t.Skip("no enclosing Go module (go env GOMOD is empty)")
	}
	moduleRoot := filepath.Dir(goMod)

	packages := map[string]*pkgHermeticStatus{}

	walkErr := filepath.WalkDir(moduleRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			// tierPuritySkipDirs (defined in tierpurity_test.go, same package) is
			// reused rather than redeclared: both guards walk the identical
			// module tree and must skip the identical non-Go-module overlay
			// directories.
			if tierPuritySkipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(d.Name(), "_test.go") {
			return nil
		}

		relPath, relErr := filepath.Rel(moduleRoot, path)
		if relErr != nil {
			return relErr
		}
		// Normalize to slash-separated form before any comparison, exactly as
		// tierpurity_test.go does: filepath.WalkDir yields backslash paths on
		// Windows (the primary dev OS).
		relPath = filepath.ToSlash(relPath)

		// This guard's own file is excluded from content-based evidence
		// entirely — not just from the spawn-token check. It documents this
		// check's tokens as literal test data, including the bare hermetic
		// presence token itself; leaving it in the scan would let its own doc
		// comment trivially satisfy cmd/lyx's requirement before cmd/lyx's real
		// TestMain lands, defeating the guard's TDD-first-fails intent.
		if fileLevelExcluded(relPath) {
			return nil
		}

		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		content := string(data)

		dir := filepath.ToSlash(filepath.Dir(relPath))
		status := packages[dir]
		if status == nil {
			status = &pkgHermeticStatus{}
			packages[dir] = status
		}

		if status.spawningFile == "" {
			if token, bad := firstSpawnToken(content); bad {
				status.spawningFile = relPath
				status.spawningToken = token
			}
		}
		if strings.Contains(content, hermeticPresenceToken) {
			status.hermetic = true
		}
		return nil
	})
	if walkErr != nil {
		t.Fatalf("failed to walk module tree: %v", walkErr)
	}

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
		if packageAllowed(dir) {
			continue
		}
		failures = append(failures, fmt.Sprintf(
			"%s: contains git-spawning token %q (in %s) but no test file in the package contains %s — add a testmain_test.go calling lyxtest.HermeticGitEnv(), or add an allowedNonHermetic entry in cmd/lyx/hermeticenv_test.go with a reason",
			dir, status.spawningToken, status.spawningFile, hermeticPresenceToken,
		))
	}
	// Deterministic ordering: map iteration order is randomized, and a
	// non-deterministic failure message ordering makes the guard's output
	// harder to diff between runs.
	sort.Strings(failures)

	// Vacuous-scan protection: a walk that finds zero git-spawning packages is
	// misconfigured (wrong root, every file mis-skipped) rather than the repo
	// having genuinely stopped spawning git in its tests.
	if gitSpawningCount == 0 {
		t.Fatalf("hermetic git env guard: found zero git-spawning packages under %s — the walk may be misconfigured", moduleRoot)
	}

	if len(failures) > 0 {
		t.Errorf("Hermetic Git Test Environment Invariant violated (see CONSTRAINTS.md):\n%s", strings.Join(failures, "\n"))
	}
}

// firstSpawnToken returns the first entry of gitSpawnTokens (in declared order)
// that appears as a raw substring of content, and whether any was found.
func firstSpawnToken(content string) (string, bool) {
	for _, token := range gitSpawnTokens {
		if strings.Contains(content, token) {
			return token, true
		}
	}
	return "", false
}

// fileLevelExcluded reports whether relPath is an exact-match allowedNonHermetic
// entry naming a single *_test.go file (the guard's own file) — as opposed to a
// directory-prefix entry exempting a whole package. See allowedNonHermetic's doc
// comment for the distinction.
func fileLevelExcluded(relPath string) bool {
	for key := range allowedNonHermetic {
		if strings.HasSuffix(key, "_test.go") && relPath == key {
			return true
		}
	}
	return false
}

// packageAllowed reports whether dir (module-relative, slash-separated package
// directory) is covered by an allowedNonHermetic directory-prefix entry: an exact
// directory match, or a match under a directory-prefix entry. File-level entries
// (see fileLevelExcluded) never count as a package-level exemption.
func packageAllowed(dir string) bool {
	for key := range allowedNonHermetic {
		if strings.HasSuffix(key, "_test.go") {
			continue
		}
		if dir == key || strings.HasPrefix(dir, key+"/") {
			return true
		}
	}
	return false
}
