// ghguard_test.go enforces the GitHub Auth Invariant's shell-out half: no production (non-test) package outside internal/githubclient may shell out to the `gh` CLI.
// internal/githubclient owns the one bounded, timeout-guarded `gh auth token` shell-out (token.go);
// every other package must go through it rather than growing its own credential path.
// See CONSTRAINTS.md's GitHub Auth Invariant.

package main

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// ghGuardAllowlistDir is the single package directory permitted to shell out to
// `gh`: the one place the GitHub Auth Invariant designates as owning token
// resolution.
const ghGuardAllowlistDir = "internal/githubclient"

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

// TestGHGuard_NoShellOutOutsideGithubclient walks every non-test *.go file under the module root and fails if any file outside ghGuardAllowlistDir contains a banned `gh` shell-out token.
// A bare "gh" substring is unusable as a banned token -- it matches "through", "right", "highlight" and hundreds of other words repo-wide -- so, following tools/sandbox/pathresolve_guard_test.go's precedent, both banned forms carry the quoted binary name so no English word can match.
func TestGHGuard_NoShellOutOutsideGithubclient(t *testing.T) {
	// Skip cleanly rather than fail when the go toolchain is not on PATH,
	// mirroring tierpurity_test.go and hermeticenv_test.go so this gate never
	// blocks a minimal environment.
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not on PATH")
	}

	// Resolve the module root via `go env GOMOD` rather than assuming the test's working directory.
	out, err := exec.Command("go", "env", "GOMOD").CombinedOutput()
	if err != nil {
		t.Fatalf("go env GOMOD failed: %v\n%s", err, out)
	}
	goMod := strings.TrimSpace(string(out))
	if goMod == "" || goMod == os.DevNull {
		t.Skip("no enclosing Go module (go env GOMOD is empty)")
	}
	moduleRoot := filepath.Dir(goMod)

	var scanned int
	var failures []string

	walkErr := filepath.WalkDir(moduleRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			// Skip overlay directories per tierPuritySkipDirs.
			if tierPuritySkipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		// Only non-test *.go files are in scope.
		if !strings.HasSuffix(d.Name(), ".go") || strings.HasSuffix(d.Name(), "_test.go") {
			return nil
		}

		relPath, relErr := filepath.Rel(moduleRoot, path)
		if relErr != nil {
			return relErr
		}
		// Normalize to slash-separated form before any comparison.
		relPath = filepath.ToSlash(relPath)
		scanned++

		if relPath == ghGuardAllowlistDir || strings.HasPrefix(relPath, ghGuardAllowlistDir+"/") {
			// internal/githubclient is the designated `gh` shell-out owner; skip it.
			return nil
		}

		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}

		if token, bad := firstBannedGHToken(string(data)); bad {
			failures = append(failures, fmt.Sprintf(
				"%s: contains banned gh shell-out token %q -- route through internal/githubclient instead",
				relPath, token,
			))
		}
		return nil
	})
	if walkErr != nil {
		t.Fatalf("failed to walk module tree: %v", walkErr)
	}

	// Vacuous-scan protection: fewer than minimum found means misconfiguration.
	if scanned < 20 {
		t.Fatalf("gh guard: only scanned %d non-test .go file(s) under %s; expected at least 20 -- the walk may be misconfigured", scanned, moduleRoot)
	}

	if len(failures) > 0 {
		t.Errorf("GitHub Auth Invariant violated (see CONSTRAINTS.md):\n%s", strings.Join(failures, "\n"))
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
