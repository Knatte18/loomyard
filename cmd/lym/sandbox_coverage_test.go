// sandbox_coverage_test.go enforces the "Sandbox Suite Coverage" invariant:
// every registered lym module needs a scenario in a tools/sandbox/*SUITE.md file, declared by a **Covers:** tag.
// A module without one is named on this test's exclusion allowlist, with a documented reason.

package main

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/shortcutcli"
	"github.com/Knatte18/loomyard/internal/testkit/scankit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// coversLinePattern matches a **Covers:** line in a tools/sandbox/*SUITE.md suite file, capturing its module list.
var coversLinePattern = regexp.MustCompile(`^\*\*Covers:\*\*\s*(.+)$`)

// excludedModules is the allowlist of modules that no sandbox scenario exercises;
// each entry carries a written reason and excludes the whole module, since coverage is module-level.
var excludedModules = []scankit.Entry{
	{
		Key: "ide",
		Why: "side-effect heavy: open and spawn start a real VS Code window, seed writes into a task worktree a scenario would have to create a pair for, menu is an interactive stdin picker",
	},
	{
		Key: "selfreport",
		Why: "create files a real GitHub issue",
	},
	{
		Key: "shed",
		Why: "armed re-exposure of loom's and batten's own verbs; covered by those two modules' own scenarios",
	},
	{
		Key: "orch",
		Why: "spawns a long-lived interactive Claude session and a detached watcher in the hub prime; exercised by internal/orchcli's smoke test instead",
	},
	{
		Key: "hibernate",
		Why: "its daemon hibernates the host machine, which no sandbox scenario may do",
	},
	{
		Key: "drift",
		Why: "needs a hub whose drift.yaml declares a capture and a compare command and a run at a Drift row, which no sandbox scenario builds; its verbs are covered by internal/driftcli's own tests",
	},
	{
		Key: "adopt",
		Why: "takes over runs from another machine, which needs a second clone of the hub that no sandbox scenario builds",
	},
	{
		Key: "beam",
		Why: "makes, promotes and lands beams over a board project entry and a pair that a scenario would have to build; exercised by internal/beamcli's own hubforge tests instead",
	},
	{
		Key: "heddle",
		Why: "set and remove start the board's detached Bolt sync and render writes into a worktree root; exercised by internal/heddlecli's own hubforge tests instead",
	},
	{
		Key: "slack",
		Why: "its relay calls the live Slack Web API with the machine's bot token, which no sandbox scenario may reach; exercised by internal/slackcli's and internal/slackengine's own tests instead",
	},
}

// TestSandboxCoverage_AllModulesCoveredOrExcluded asserts every module is covered or excluded.
// A narrowed gate runs it only when a suite file, the root command or a module's CLI package changes;
// a whole gate always runs it.
//
//testtiming:keep pins that every registered module is covered by the sandbox suite or excluded with a reason, a guard no covering test asserts
//lym:guard
//lym:watch tools/sandbox/*SUITE.md cmd/lym/main.go internal/*cli/*.go internal/shortcutcli/*
//lym:watch internal/testkit/scankit/*.go
func TestSandboxCoverage_AllModulesCoveredOrExcluded(t *testing.T) {
	tiergate.Require(t, 0)
	// Collect the module names registered on the live cobra root.
	// The completion subtree is skipped, since it is cobra's and not a lym module.
	root := newRoot()
	registered := make(map[string]bool)
	for _, child := range root.Commands() {
		name := child.Name()
		if name == "completion" {
			continue
		}
		// A shortcut runs other modules' commands and has nothing of its own for a sandbox scenario to exercise.
		if child.GroupID == shortcutcli.GroupID {
			continue
		}
		registered[name] = true
	}

	covered := parseCoveredModules(t)

	// An empty set would let the asserts below pass vacuously.
	t.Run("discovered_non_empty", func(t *testing.T) {
		if len(registered) == 0 {
			t.Error("sandbox coverage guard: no registered modules found via newRoot().Commands(); the cobra root may be misconfigured")
		}
		if len(covered) == 0 {
			t.Error("sandbox coverage guard: no **Covers:** tags found across tools/sandbox/*SUITE.md; the doc parse may be misconfigured")
		}
	})

	// A module without a covering tag must be on the allowlist, or the scenario coverage has a gap.
	excluded := scankit.NewAllowlist(excludedModules)
	for m := range registered {
		if len(covered[m]) > 0 {
			continue
		}
		if excluded.Allowed(m) {
			continue
		}
		t.Errorf(
			"module %q is registered in newRoot() but has no **Covers:** tag in any tools/sandbox/*SUITE.md file and is not on the excludedModules allowlist in cmd/lym/sandbox_coverage_test.go; add a scenario tag or an allowlist entry with a reason",
			m,
		)
	}

	// A covered or excluded name must be a registered module, so a stale or misspelled name fails the guard.
	for m, files := range covered {
		if !registered[m] {
			t.Errorf(
				"%v tag %q via **Covers:** but no such module is registered in newRoot(); fix the typo or remove the stale tag",
				files, m,
			)
		}
	}
	for _, e := range excludedModules {
		if !registered[e.Key] {
			t.Errorf(
				"excludedModules in cmd/lym/sandbox_coverage_test.go names %q but no such module is registered in newRoot(); remove the stale allowlist entry",
				e.Key,
			)
		}
	}
}

// parseCoveredModules scans tools/sandbox/*SUITE.md files and returns modules covered via **Covers:** tags.
func parseCoveredModules(t *testing.T) map[string][]string {
	t.Helper()

	suitePattern := filepath.Join(scankit.Root(t), "tools", "sandbox", "*SUITE.md")
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
			// Each token is trimmed and an empty one is skipped; a token that names no module fails the guard.
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
