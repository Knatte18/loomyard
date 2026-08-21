// sandbox_coverage_test.go enforces the "Sandbox Suite Coverage" invariant: every registered lyx
// module must either be exercised by a scenario in one of the tools/sandbox/*SUITE.md suite files
// (declared via an explicit **Covers:** tag) or be named on this test's exclusion allowlist with a
// documented reason.
// This is the sandbox-suite analogue of registration_test.go's "exists => registered" guard.

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
// tools/sandbox/*SUITE.md suite file, capturing the comma/whitespace-separated
// module list.
var coversLinePattern = regexp.MustCompile(`^\*\*Covers:\*\*\s*(.+)$`)

// excludedModules is the Sandbox Suite Coverage allowlist: modules that are
// intentionally never exercised by a sandbox scenario, each with a one-line
// reason. Coverage is module-level (see CONSTRAINTS.md's Sandbox Suite Coverage
// invariant), so each entry excludes the whole module, not individual subcommands.
var excludedModules = map[string]string{
	"ide":        "side-effect heavy: spawn opens a real VS Code window, menu is an interactive stdin picker",
	"selfreport": "create files a real GitHub issue",
	"run":        "alias of loom's own bootstrap verb; covered by the loom module's scenario",
}

// TestSandboxCoverage_AllModulesCoveredOrExcluded asserts every module is covered or excluded.
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

	// Sanity sub-test: both sets must be non-empty.
	t.Run("discovered_non_empty", func(t *testing.T) {
		if len(registered) == 0 {
			t.Error("sandbox coverage guard: no registered modules found via newRoot().Commands(); the cobra root may be misconfigured")
		}
		if len(covered) == 0 {
			t.Error("sandbox coverage guard: no **Covers:** tags found across tools/sandbox/*SUITE.md; the doc parse may be misconfigured")
		}
	})

	// Assert 1: every registered module must be covered or excluded.
	for m := range registered {
		if len(covered[m]) > 0 {
			continue
		}
		if _, ok := excludedModules[m]; ok {
			continue
		}
		t.Errorf(
			"module %q is registered in newRoot() but has no **Covers:** tag in any tools/sandbox/*SUITE.md file and is not on the excludedModules allowlist in cmd/lyx/sandbox_coverage_test.go; add a scenario tag or an allowlist entry with a reason",
			m,
		)
	}

	// Assert 2: every covered/excluded token must name a registered module (drift guard).
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

// parseCoveredModules scans tools/sandbox/*SUITE.md files and returns modules covered via **Covers:** tags.
func parseCoveredModules(t *testing.T) map[string][]string {
	t.Helper()

	_, testFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not determine test file location via runtime.Caller")
	}
	repoRoot := filepath.Dir(filepath.Dir(filepath.Dir(testFile)))

	suitePattern := filepath.Join(repoRoot, "tools", "sandbox", "*SUITE.md")
	suitePaths, err := filepath.Glob(suitePattern)
	if err != nil {
		t.Fatalf("could not glob tools/sandbox/*SUITE.md: %v", err)
	}
	// Vacuous-glob guard: fewer than two suite files means misconfiguration.
	if len(suitePaths) < 2 {
		t.Fatalf(
			"tools/sandbox/*SUITE.md glob matched %d file(s) (%v); expected at least 2 (the repo ships SANDBOX-CORE-SUITE.md and SANDBOX-REED-SUITE.md) — the pattern or directory is likely wrong",
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
