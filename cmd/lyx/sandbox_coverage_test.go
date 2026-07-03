// sandbox_coverage_test.go enforces the "Sandbox Suite Coverage" invariant: every
// registered lyx module must either be exercised by a scenario in one of the
// tools/sandbox/*SANDBOX-SUITE.md suite files (declared via an explicit **Covers:**
// tag) or be named on this test's exclusion allowlist with a documented reason. This
// is the sandbox-suite analogue of registration_test.go's "exists => registered" guard.

package main

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"testing"
)

// coversLinePattern matches a "**Covers:** <module>[, <module>...]" line in any
// tools/sandbox/*SANDBOX-SUITE.md suite file, capturing the comma/whitespace-separated
// module list.
var coversLinePattern = regexp.MustCompile(`^\*\*Covers:\*\*\s*(.+)$`)

// excludedModules is the Sandbox Suite Coverage allowlist: modules that are
// intentionally never exercised by a sandbox scenario, each with a one-line
// reason. Coverage is module-level (see CONSTRAINTS.md's Sandbox Suite Coverage
// invariant), so each entry excludes the whole module, not individual subcommands.
var excludedModules = map[string]string{
	"ide":        "side-effect heavy: spawn opens a real VS Code window, menu is an interactive stdin picker",
	"selfreport": "create files a real GitHub issue",
}

// TestSandboxCoverage_AllModulesCoveredOrExcluded discovers every module
// registered in the live cobra root and every module declared covered by a
// **Covers:** tag across all tools/sandbox/*SANDBOX-SUITE.md suite files, then
// asserts that every registered module is either covered or on the
// excludedModules allowlist, and that every covered/excluded module name
// actually corresponds to a live registered module (catching typos and stale
// tags/allowlist entries left behind by a module rename or removal).
func TestSandboxCoverage_AllModulesCoveredOrExcluded(t *testing.T) {
	// Build the live cobra root and collect every registered module name, skipping
	// cobra's own infrastructure subtrees — mirrors longlist_test.go's skip pattern
	// so the module set here never drifts from what that guard already uses.
	root := newRoot()
	registered := make(map[string]bool)
	for _, child := range root.Commands() {
		name := child.Name()
		if name == "help" || name == "completion" {
			continue
		}
		registered[name] = true
	}

	covered := parseCoveredModules(t)

	// Sanity sub-test: both sets must be non-empty, so a silently-broken cobra-root
	// walk or doc parse (wrong directory, all lines skipped) cannot produce a
	// vacuous all-pass result — mirrors registration_test.go's discovered_non_empty.
	t.Run("discovered_non_empty", func(t *testing.T) {
		if len(registered) == 0 {
			t.Error("sandbox coverage guard: no registered modules found via newRoot().Commands(); the cobra root may be misconfigured")
		}
		if len(covered) == 0 {
			t.Error("sandbox coverage guard: no **Covers:** tags found across tools/sandbox/*SANDBOX-SUITE.md; the doc parse may be misconfigured")
		}
	})

	// Assert 1 (coverage): every registered module must be covered by a scenario
	// or explicitly excluded with a reason.
	for m := range registered {
		if len(covered[m]) > 0 {
			continue
		}
		if _, ok := excludedModules[m]; ok {
			continue
		}
		t.Errorf(
			"module %q is registered in newRoot() but has no **Covers:** tag in any tools/sandbox/*SANDBOX-SUITE.md file and is not on the excludedModules allowlist in cmd/lyx/sandbox_coverage_test.go; add a scenario tag or an allowlist entry with a reason",
			m,
		)
	}

	// Assert 2 (drift guard): every covered/excluded token must name a module that
	// is actually registered today, catching stale tags or allowlist entries left
	// behind by a rename or removal. Name the offending suite file(s) so the fix
	// is a one-line diff away, not a grep exercise.
	for m, files := range covered {
		if !registered[m] {
			t.Errorf(
				"%v tag %q via **Covers:** but no such module is registered in newRoot(); fix the typo or remove the stale tag",
				files, m,
			)
		}
	}
	for m := range excludedModules {
		if !registered[m] {
			t.Errorf(
				"excludedModules in cmd/lyx/sandbox_coverage_test.go names %q but no such module is registered in newRoot(); remove the stale allowlist entry",
				m,
			)
		}
	}
}

// parseCoveredModules scans every tools/sandbox/*SANDBOX-SUITE.md suite file on disk
// (resolving the repo root from this test file's own on-disk location, exactly as
// registration_test.go does) and returns a map from module token to the sorted list
// of suite-file basenames that declare it via a **Covers:** line — so Assert-2's
// stale-tag error can name the offending file(s) instead of just the token.
func parseCoveredModules(t *testing.T) map[string][]string {
	t.Helper()

	// This file lives at cmd/lyx/sandbox_coverage_test.go: three filepath.Dir
	// walk-ups reach the repo root (cmd/lyx -> cmd -> repo root), matching the
	// code (not the stale "two" comment) at registration_test.go:71.
	_, testFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not determine test file location via runtime.Caller")
	}
	repoRoot := filepath.Dir(filepath.Dir(filepath.Dir(testFile)))

	suitePattern := filepath.Join(repoRoot, "tools", "sandbox", "*SANDBOX-SUITE.md")
	suitePaths, err := filepath.Glob(suitePattern)
	if err != nil {
		t.Fatalf("could not glob tools/sandbox/*SANDBOX-SUITE.md: %v", err)
	}
	// Vacuous-glob guard: the repo ships at least SANDBOX-SUITE.md and
	// MUX-SANDBOX-SUITE.md, so fewer than two matches means the pattern or
	// directory resolved wrong rather than the suite set having genuinely shrunk.
	if len(suitePaths) < 2 {
		t.Fatalf(
			"tools/sandbox/*SANDBOX-SUITE.md glob matched %d file(s) (%v); expected at least 2 (the repo ships SANDBOX-SUITE.md and MUX-SANDBOX-SUITE.md) — the pattern or directory is likely wrong",
			len(suitePaths), suitePaths,
		)
	}

	covered := make(map[string][]string)
	for _, suitePath := range suitePaths {
		data, err := os.ReadFile(suitePath)
		if err != nil {
			t.Fatalf("could not read %s: %v", suitePath, err)
		}
		base := filepath.Base(suitePath)
		for _, line := range strings.Split(string(data), "\n") {
			match := coversLinePattern.FindStringSubmatch(strings.TrimSpace(line))
			if match == nil {
				continue
			}
			// Scenarios without a Covers line are skipped, so every token that does
			// appear is expected to be a bare registered-module name — no
			// parenthesized-token stripping is needed here.
			for _, token := range strings.Split(match[1], ",") {
				token = strings.TrimSpace(token)
				if token != "" {
					covered[token] = append(covered[token], base)
				}
			}
		}
	}
	for m := range covered {
		sort.Strings(covered[m])
	}
	return covered
}
