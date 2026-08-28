// tierpurity_test.go enforces the Test Tier Purity Invariant: untagged *_test.go files (the ones
// that run in every plain `go test`, without `-tags integration`/`smoke`) perform no
// expensive spawns — no gitexec.Run, no exec.Command/CommandContext, no gitkit.Copy* fixture-tree
// copy, and no hubforge.NewHub real-hub fixture build.
// This is the repo-wide grep-guard that keeps the offline Tier 1 loop's premise from rotting
// silently again, machine-enforcing what was previously review discipline only.
// See CONSTRAINTS.md's Test Tier Purity Invariant.
// It also flags an untagged file containing a long literal time.Sleep(...) (see
// cmd/lyx/tiersleep_test.go).

package main

import (
	"fmt"
	"go/token"
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
	"internal/proc":                           "process control is the package's subject — its tests must spawn",
	"cmd/lyx/tierpurity_test.go":              "contains the banned token strings as its own test data",
	"cmd/lyx/hermeticenv_test.go":             "contains the banned token strings as its own test data (Hermetic Git Test Environment Invariant guard)",
	"tools/sandbox/pathresolve_guard_test.go": "contains the banned `exec.Command`/`exec.CommandContext` token strings as its own scan data (Dev/Prod Binary Separation guard)",
	"cmd/lyx/ghguard_test.go":                 "contains the banned `exec.Command`/`exec.CommandContext` token strings as its own scan data (GitHub Auth Invariant guard)",
	"cmd/lyx/gitrepoboundary_test.go":         "resolves its scan root via `go env GOMOD` (contains `exec.Command`) and names `gitexec.RunGit` in its own doc comment (gitrepo Client Boundary Invariant guard)",
	"cmd/lyx/boardguard_test.go":              "contains `exec.Command` to resolve the module root via `go env GOMOD` (mirrors ghguard_test.go/gitrepoboundary_test.go's identical pattern, both already allowlisted here) — the Fabric Git Invariant board-guard",
	"cmd/lyx/rawgitmutation_test.go":          "contains the banned `gitexec.Run`/`exec.Command` token strings as its own scan data (Fabric Git Invariant raw-git-mutation guard)",
	"cmd/lyx/destructiveguard_test.go":        "resolves its scan root via `go env GOMOD` (contains `exec.Command`) and carries its own banned destructive tokens as scan data (Fabric Destruction Chokepoint Invariant guard)",
	"cmd/lyx/uncontainedwrite_test.go":        "resolves its scan root via `go env GOMOD` (contains `exec.Command`) and carries its own banned raw-write tokens as scan data (Fabric Write-Side Containment Invariant guard)",
	"cmd/lyx/checkedcall_test.go":             "contains the banned `gitexec.RunGit`/`exec.Command` token strings as its own scan data and resolves its scan root via `go env GOMOD` (gitexec Checked-Call Invariant guard)",
	"cmd/lyx/cwdmutation_test.go":             "resolves its scan root via `go env GOMOD` (contains `exec.Command`) and carries its own banned t.Chdir(/os.Chdir( tokens as scan data (Cwd Resolution Invariant chdir-mutation guard)",
	"cmd/lyx/configstrictness_test.go":        "resolves its scan root via `go env GOMOD` (contains `exec.Command`) (Config Strictness Invariant guard)",
	"cmd/lyx/spawnobservability_test.go":      "contains the banned `exec.Command`/`exec.CommandContext` token strings as its own scan data and resolves its scan root via `go env GOMOD` (Live-Substrate Spawn Observability guard)",
}

// knownTierTags are the `//go:build` constraint substrings that mark a *_test.go file
// as tagged (i.e. excluded from a plain `go test` run) for Test Tier Purity purposes.
// isTierTagged matches on any entry, so adding a new tier tag here is the single place
// that both the purity guard and its doc comments need to stay in sync with.
var knownTierTags = []string{"integration", "smoke"}

// bannedTokens are the raw substrings an untagged *_test.go file may not contain.
// Matching is deliberately raw-substring, not whole-token or AST: exec.Command also
// matches exec.CommandContext, and gitkit.Copy prefix-matches gitkit.CopyRepo and any future
// Copy* fixture. Comment or string-literal mentions trip the guard too — that is
// accepted (rename the mention or tag the file).
// hubforge.NewHub is banned by the same rule: it drives a real fabriccli.CloneAndWire clone, so an
// untagged test calling it is exactly the expensive-spawn violation this guard exists to catch.
// hubforge.SeedConfig and hubforge.SeedFabricConfig need no separate entries — both take a *Hub that
// only NewHub can produce, so this token already covers every package that can reach them.
var bannedTokens = []string{
	"gitexec.Run",
	"exec.Command",
	"gitkit.Copy",
	"hubforge.NewHub",
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

// TestTierPurity_UntaggedTestsSpawnNothing walks every *_test.go file under the module root and
// fails if any untagged file — one whose first non-empty line is not a `//go:build` constraint
// mentioning any of knownTierTags — contains a banned spawn token as a raw substring, unless the
// file (or its containing directory) is on the allowedSpawners allowlist.
// Platform-only constraints (e.g. `//go:build windows`) count as untagged: they still run in Tier 1
// on that platform.
func TestTierPurity_UntaggedTestsSpawnNothing(t *testing.T) {
	// Skip cleanly rather than fail when the go toolchain is not on PATH, mirroring
	// crosscompile_test.go so this gate never blocks a minimal environment.
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
		// Normalize to slash-separated form before any comparison.
		relPath = filepath.ToSlash(relPath)
		scanned++

		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		if isTierTagged(data) {
			return nil
		}

		bannedTok, bad := firstBannedToken(data)
		if bad && !pathAllowlisted(relPath, allowedSpawners) {
			failures = append(failures, fmt.Sprintf(
				"%s: contains banned token %q in an untagged test file — move it behind one of knownTierTags' `//go:build` constraints (integration or smoke), or add an allowedSpawners entry in cmd/lyx/tierpurity_test.go with a reason",
				relPath, bannedTok,
			))
		}

		// Sleep guard is an independent check and must still run for every untagged file.
		if !pathAllowlisted(relPath, allowedLongSleepers) {
			if evidence, found := findLongLiteralSleep(token.NewFileSet(), path, data); found {
				failures = append(failures, fmt.Sprintf(
					"%s: contains a literal time.Sleep(...) of >= 1s in an untagged test file (%s) — move it behind a build tag, shrink the duration, or add an allowedLongSleepers entry in cmd/lyx/tiersleep_test.go with a reason",
					relPath, evidence,
				))
			}
		}

		return nil
	})
	if walkErr != nil {
		t.Fatalf("failed to walk module tree: %v", walkErr)
	}

	// Vacuous-scan protection: fewer than 20 found means misconfiguration.
	if scanned < 20 {
		t.Fatalf("tier purity guard: only scanned %d *_test.go file(s) under %s; expected at least 20 — the walk may be misconfigured", scanned, moduleRoot)
	}

	if len(failures) > 0 {
		t.Errorf("Test Tier Purity Invariant violated (see CONSTRAINTS.md):\n%s", strings.Join(failures, "\n"))
	}
}

// isTierTagged reports whether data's first line is a `//go:build` constraint with a known tier tag.
func isTierTagged(data []byte) bool {
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if !strings.HasPrefix(trimmed, "//go:build") {
			return false
		}
		for _, tag := range knownTierTags {
			if strings.Contains(trimmed, tag) {
				return true
			}
		}
		return false
	}
	return false
}

// TestIsTierTagged_RecognizesKnownTagsList verifies isTierTagged recognizes all known tier tags.
func TestIsTierTagged_RecognizesKnownTagsList(t *testing.T) {
	tests := []struct {
		name string
		line string
		want bool
	}{
		{"integration", "//go:build integration", true},
		{"smoke", "//go:build smoke", true},
		{"platform_only_untagged", "//go:build windows", false},
		{"empty", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isTierTagged([]byte(tt.line))
			if got != tt.want {
				t.Errorf("isTierTagged(%q) = %v; want %v", tt.line, got, tt.want)
			}
		})
	}
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

// pathAllowlisted reports whether relPath is covered by an entry in allowlist.
func pathAllowlisted(relPath string, allowlist map[string]string) bool {
	for prefix := range allowlist {
		if relPath == prefix || strings.HasPrefix(relPath, prefix+"/") {
			return true
		}
	}
	return false
}
