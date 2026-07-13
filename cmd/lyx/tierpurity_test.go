// tierpurity_test.go enforces the Test Tier Purity Invariant: untagged *_test.go files
// (the ones that run in every plain `go test`, without `-tags integration`/`smoke`)
// perform no expensive spawns — no gitexec.RunGit, no exec.Command/CommandContext, and
// no lyxtest.Copy* fixture-tree copy. This is the repo-wide grep-guard that keeps the
// offline Tier 1 loop's premise from rotting silently again, machine-enforcing what was
// previously review discipline only. See CONSTRAINTS.md's Test Tier Purity Invariant.

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

// allowedSpawners is the Test Tier Purity Invariant allowlist: module-relative,
// slash-separated file paths or directory-path prefixes that are permitted to contain
// a banned spawn token in an untagged test file, each with a one-line reason —
// mirroring sandbox_coverage_test.go's excludedModules style.
var allowedSpawners = map[string]string{
	"internal/proc":               "process control is the package's subject — its tests must spawn",
	"cmd/lyx/tierpurity_test.go":  "contains the banned token strings as its own test data",
	"cmd/lyx/hermeticenv_test.go": "contains the banned token strings as its own test data (Hermetic Git Test Environment Invariant guard)",
}

// bannedTokens are the raw substrings an untagged *_test.go file may not contain.
// Matching is deliberately raw-substring, not whole-token or AST: exec.Command also
// matches exec.CommandContext, and lyxtest.Copy prefix-matches lyxtest.CopyPaired,
// lyxtest.CopyPairedLocal, lyxtest.CopyHostHub, lyxtest.CopyWeft, and any future
// Copy* fixture. Comment or string-literal mentions trip the guard too — that is
// accepted (rename the mention or tag the file).
var bannedTokens = []string{
	"gitexec.RunGit",
	"exec.Command",
	"lyxtest.Copy",
}

// tierPuritySkipDirs names directories the walk never descends into: version control
// and the mill/wiki/scratch overlay trees, none of which are part of the Go module's
// test surface.
var tierPuritySkipDirs = map[string]bool{
	".git":     true,
	"_lyx":     true,
	"_mill":    true,
	".scratch": true,
	".wiki":    true,
	"_raddle":  true,
}

// TestTierPurity_UntaggedTestsSpawnNothing walks every *_test.go file under the module
// root and fails if any untagged file — one whose first non-empty line is not a
// `//go:build` constraint mentioning "integration" or "smoke" — contains a banned spawn
// token as a raw substring, unless the file (or its containing directory) is on the
// allowedSpawners allowlist. Platform-only constraints (e.g. `//go:build windows`)
// count as untagged: they still run in Tier 1 on that platform.
func TestTierPurity_UntaggedTestsSpawnNothing(t *testing.T) {
	// Skip cleanly rather than fail when the go toolchain is not on PATH, mirroring
	// crosscompile_test.go so this gate never blocks a minimal environment.
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not on PATH")
	}

	// Resolve the module root via `go env GOMOD` rather than assuming the test's
	// working directory, exactly as crosscompile_test.go does, so the walk is
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

	var scanned int
	var failures []string

	walkErr := filepath.WalkDir(moduleRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
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
		// Normalize to slash-separated form before any comparison: filepath.WalkDir
		// yields backslash paths on Windows (the primary dev OS), and un-normalized
		// matching would silently miss the slash-separated allowedSpawners prefixes.
		relPath = filepath.ToSlash(relPath)
		scanned++

		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		if isTierTagged(data) {
			return nil
		}

		token, bad := firstBannedToken(data)
		if !bad {
			return nil
		}
		if spawnerAllowed(relPath) {
			return nil
		}

		failures = append(failures, fmt.Sprintf(
			"%s: contains banned token %q in an untagged test file — move it behind `//go:build integration` (or `smoke`), or add an allowedSpawners entry in cmd/lyx/tierpurity_test.go with a reason",
			relPath, token,
		))
		return nil
	})
	if walkErr != nil {
		t.Fatalf("failed to walk module tree: %v", walkErr)
	}

	// Vacuous-scan protection: the repo has ~60 *_test.go files; fewer than 20 found
	// means the walk is misconfigured (wrong root, all files skipped) rather than the
	// suite having genuinely shrunk.
	if scanned < 20 {
		t.Fatalf("tier purity guard: only scanned %d *_test.go file(s) under %s; expected at least 20 — the walk may be misconfigured", scanned, moduleRoot)
	}

	if len(failures) > 0 {
		t.Errorf("Test Tier Purity Invariant violated (see CONSTRAINTS.md):\n%s", strings.Join(failures, "\n"))
	}
}

// isTierTagged reports whether data's first non-empty line is a `//go:build`
// constraint mentioning "integration" or "smoke". A platform-only constraint (e.g.
// `//go:build windows`) is NOT tagged — it still runs in Tier 1 on that platform, so
// its spawns still count.
func isTierTagged(data []byte) bool {
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if !strings.HasPrefix(trimmed, "//go:build") {
			return false
		}
		return strings.Contains(trimmed, "integration") || strings.Contains(trimmed, "smoke")
	}
	return false
}

// firstBannedToken returns the first entry of bannedTokens (in declared order) that
// appears as a raw substring of data, and whether any was found.
func firstBannedToken(data []byte) (string, bool) {
	content := string(data)
	for _, token := range bannedTokens {
		if strings.Contains(content, token) {
			return token, true
		}
	}
	return "", false
}

// spawnerAllowed reports whether relPath (module-relative, slash-separated) is covered
// by an allowedSpawners entry: an exact file match, or a match under a directory-prefix
// entry.
func spawnerAllowed(relPath string) bool {
	for prefix := range allowedSpawners {
		if relPath == prefix || strings.HasPrefix(relPath, prefix+"/") {
			return true
		}
	}
	return false
}
