// reconcile_integration_test.go holds the reconcile scenario that seeds a real git repo with gitexec.RunGit:
// the dry-run and --apply round trips and the --set-then-reconcile drift check.
// This file is integration-tagged per the Test Tier Purity Invariant;
// the spawn-free not-a-git-repo assertion lives in TestCommand_FromNonGitDirectory.

package configcli

import (
	"bytes"
	"encoding/json"
	"os"
	"slices"
	"testing"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// runReconcileCLI runs `lym config reconcile <args>` from dir and returns the exit code and the decoded envelope.
func runReconcileCLI(t *testing.T, dir string, args ...string) (int, map[string]any) {
	t.Helper()
	var out bytes.Buffer
	code := clihelp.ExecuteIn(Command(), dir, &out, append([]string{"reconcile"}, args...))
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

// TestConfigReconcileInGitRepo runs `lym config reconcile` over one git repository.
// A dry run reports without writing, and a --set-planted orphan key is reported by reconcile instead of being lost.
// --apply prunes the sparse worktree layer without creating an absent file.
// Its steps run in order in that one repository, so the scenario spawns git once.
func TestConfigReconcileInGitRepo(t *testing.T) {
	tiergate.Require(t, 1)
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
		seedModuleConfig(t, tmpDir, "lenses", "lenses:\n  hub-lens: |\n    Hub emphasis.\nfans:\n  standard: [generic, generic, correctness, error-handling, test-gaps]\n")

		code, result := runReconcileCLI(t, tmpDir)

		if code != 0 {
			t.Errorf("Execute(reconcile) = %d; want 0, result: %v", code, result)
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
		keptExtra, _ := reconcileModule(t, result, "lenses")["keptExtra"].([]any)
		if !slices.Equal(keptExtra, []any{"lenses.hub-lens"}) {
			t.Errorf("lenses module's keptExtra = %v; want [lenses.hub-lens]", keptExtra)
		}
		if _, has := reconcileModule(t, result, "loom")["keptExtra"]; has {
			t.Error("loom module entry carries keptExtra; want it omitted when nothing is kept")
		}
		// A sparse layer's template-equal entry can override a tuned value below it, so reconcile keeps it.
		if dropped, has := reconcileModule(t, result, "lenses")["dropped"]; has {
			t.Errorf("lenses module's dropped = %v on the sparse worktree layer; want it omitted", dropped)
		}
		modules, _ := result["modules"].([]any)
		if len(modules) == 0 {
			t.Fatal("modules array is empty; want non-empty")
		}
		mod, _ := modules[0].(map[string]any)
		for _, field := range []string{"module", "added", "removed", "applied", "layer"} {
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
		setCode := dispatch(makeLayoutAt(tmpDir), &setOut, []string{"loom"}, makeNeverCalledEditor(t), (&fakeSyncTracker{exitCode: 0}).syncFunc(), nil, false, []string{"discussion_timeout_min=60"}, "", nil, true)
		if setCode != 0 {
			t.Fatalf("dispatch(--set) = %d; want 0; output: %q", setCode, setOut.String())
		}

		code, result := runReconcileCLI(t, tmpDir)

		if code != 0 {
			t.Fatalf("Execute(reconcile) = %d; want 0; result: %v", code, result)
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

	t.Run("apply prunes the sparse worktree layer", func(t *testing.T) {
		code, result := runReconcileCLI(t, tmpDir, "--apply")

		if code != 0 {
			t.Errorf("Execute(reconcile --apply) = %d; want 0, result: %v", code, result)
		}
		if ok, _ := result["ok"].(bool); !ok {
			t.Error("ok flag is not true")
		}
		if applied, _ := result["applied"].(bool); !applied {
			t.Error("applied is false; want true")
		}
		// In a hub the worktree file is a sparse layer: apply prunes unknown keys but never creates an absent module's file.
		if _, err := os.Stat(reedPath); !os.IsNotExist(err) {
			t.Errorf("reed.yaml created by --apply (stat err = %v); want it left absent", err)
		}
		if content, err := os.ReadFile(loomPath); err != nil || bytes.Contains(content, []byte("legacy_key")) {
			t.Errorf("loom.yaml = %q, %v; want its unknown legacy_key pruned", content, err)
		}
		if layer, _ := reconcileModule(t, result, "loom")["layer"].(string); layer != "worktree" {
			t.Errorf("loom module layer = %q; want worktree", layer)
		}
	})
}
