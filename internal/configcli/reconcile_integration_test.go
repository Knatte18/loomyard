//go:build integration

// reconcile_integration_test.go holds the reconcile scenario that spawns
// gitexec.RunGit(["init"], …) to seed a real git repo: the dry-run and --apply round trips and the
// --set-then-reconcile drift check. This file is integration-tagged per the Test Tier
// Purity Invariant; the spawn-free not-a-git-repo assertion lives in TestRunCLIIn_FromNonGitDirectory.

package configcli

import (
	"bytes"
	"encoding/json"
	"os"
	"slices"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

// runReconcileCLI runs `lyx config reconcile <args>` from dir and returns the exit code and the decoded envelope.
func runReconcileCLI(t *testing.T, dir string, args ...string) (int, map[string]any) {
	t.Helper()
	var out bytes.Buffer
	code := RunCLIIn(dir, &out, append([]string{"reconcile"}, args...))
	var result map[string]any
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatalf("parse JSON: %v, output: %s", err, out.String())
	}
	return code, result
}

// reconcileModule returns the envelope's modules entry for module, failing the test when it is absent.
func reconcileModule(t *testing.T, result map[string]any, module string) map[string]any {
	t.Helper()
	modules, ok := result["modules"].([]any)
	if !ok {
		t.Fatalf("modules is not an array; got %v", result)
	}
	for _, m := range modules {
		if mod, ok := m.(map[string]any); ok && mod["module"] == module {
			return mod
		}
	}
	t.Fatalf("no modules entry for %q; got %v", module, modules)
	return nil
}

// TestConfigReconcileInGitRepo runs `lyx config reconcile` over one git repository: a dry run
// reports without writing, a --set-planted orphan key is reported by reconcile instead of being lost,
// and --apply writes the missing module files and retires a per-worktree hub-wide copy the hub file
// subsumes.
// Its steps run in this order in one repository; the apply step relies on the dry-run step having
// left reed.yaml unwritten.
// The scenario runs in one repository so it spawns git once.
func TestConfigReconcileInGitRepo(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	if _, _, exitCode, err := gitexec.RunGit([]string{"init"}, tmpDir); err != nil || exitCode != 0 {
		t.Fatalf("git init failed: %v (exit code %d)", err, exitCode)
	}
	loomPath := configengine.ConfigFile(tmpDir, "loom")
	reedPath := configengine.ConfigFile(tmpDir, "reed")

	if !t.Run("dry run writes nothing and reports every module's shape", func(t *testing.T) {
		const originalContent = "discussion_timeout_min: 480\nstale_key: old_value\n"
		seedModuleConfig(t, tmpDir, "loom", originalContent)

		code, result := runReconcileCLI(t, tmpDir)

		if code != 0 {
			t.Errorf("RunCLI(reconcile) = %d; want 0, result: %v", code, result)
		}
		if ok, _ := result["ok"].(bool); !ok {
			t.Error("ok flag is not true")
		}
		if applied, _ := result["applied"].(bool); applied {
			t.Error("applied is true; want false (dry-run)")
		}
		if content, err := os.ReadFile(loomPath); err != nil || string(content) != originalContent {
			t.Errorf("loom.yaml = %q, %v; want it unchanged by a dry run", content, err)
		}
		if _, err := os.Stat(reedPath); !os.IsNotExist(err) {
			t.Errorf("reed.yaml written by a dry run (stat err = %v)", err)
		}
		modules, _ := result["modules"].([]any)
		if len(modules) == 0 {
			t.Fatal("modules array is empty; want non-empty")
		}
		mod, _ := modules[0].(map[string]any)
		for _, field := range []string{"module", "added", "removed", "applied"} {
			if _, has := mod[field]; !has {
				t.Errorf("module entry missing %q field", field)
			}
		}
	}) {
		return
	}

	if !t.Run("an orphan key planted by --set is reported by reconcile", func(t *testing.T) {
		seedModuleConfig(t, tmpDir, "loom", "discussion_timeout_min: 480\nlegacy_key: keepme\n")

		var setOut bytes.Buffer
		setCode := dispatch(makeLayoutAt(tmpDir), &setOut, []string{"loom"}, makeNeverCalledEditor(t), (&fakeSyncTracker{exitCode: 0}).syncFunc(), nil, false, []string{"discussion_timeout_min=60"})
		if setCode != 0 {
			t.Fatalf("dispatch(--set) = %d; want 0; output: %q", setCode, setOut.String())
		}

		code, result := runReconcileCLI(t, tmpDir)

		if code != 0 {
			t.Fatalf("RunCLI(reconcile) = %d; want 0; result: %v", code, result)
		}
		removed, ok := reconcileModule(t, result, "loom")["removed"].([]any)
		if !ok {
			t.Fatalf("loom module entry missing \"removed\" field or wrong type; got %v", result)
		}
		if !slices.Contains(removed, any("legacy_key")) {
			t.Errorf("loom module's removed = %v; want it to contain \"legacy_key\"", removed)
		}
	}) {
		return
	}

	t.Run("apply writes the missing module files and retires the subsumed board copy", func(t *testing.T) {
		const subsumedBoard = "labels:\n  area-x: Area X\n"
		boardPath := configengine.ConfigFile(tmpDir, "board")
		seedModuleConfig(t, tmpDir, "board", subsumedBoard)
		location, err := lyxcwd.Resolve(tmpDir)
		if err != nil {
			t.Fatalf("resolve: %v", err)
		}
		seedModuleConfig(t, fabricengine.BoardDir(location.HubPath), "board", "labels:\n  area-x: Area X\n  area-y: Area Y\n")

		code, result := runReconcileCLI(t, tmpDir, "--apply")

		if code != 0 {
			t.Errorf("RunCLI(reconcile --apply) = %d; want 0, result: %v", code, result)
		}
		if ok, _ := result["ok"].(bool); !ok {
			t.Error("ok flag is not true")
		}
		if applied, _ := result["applied"].(bool); !applied {
			t.Error("applied is false; want true")
		}
		// Hub-wide modules are never written per-worktree, so "reed" is the generic module this
		// reconcile-writes-to-disk assertion exercises.
		if _, err := os.Stat(reedPath); err != nil {
			t.Errorf("reed.yaml not created: %v", err)
		}
		if retired, _ := reconcileModule(t, result, "board")["retired"].(bool); !retired {
			t.Errorf("board module not reported retired; result = %v", result)
		}
		if _, err := os.Stat(boardPath); !os.IsNotExist(err) {
			t.Errorf("per-worktree board.yaml still present after --apply (stat err = %v)", err)
		}
	})
}
