//go:build integration

// configcli_integration_test.go — e2e integration tests for configcli.
// Tests real fabriccli.RunCLI over a hubforge.NewHub fixture, plus the --set/reconcile
// chain that spawns gitexec.RunGit(["init"], …).

package configcli

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/fabriccli"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

// TestE2ESyncIntegration is an e2e test using a real hub: creates a new worktree with dispatch,
// edits a config, and verifies the file is tracked in the fabric repo while the code side stays pristine.
func TestE2ESyncIntegration(t *testing.T) {
	const slug = "config-e2e-test"

	// Build a real hub. fabriccli.CloneAndWire has already materialized every registered module's
	// config plus the repo-wide fabric.yaml at BoardDir, and the records-side primary already sits on its
	// WeftBranchName-suffixed branch -- everything the old fixture's SeedConfig, seedRepoWideFabricConfig
	// and manual records-branch checkout hand-rolled is arriving for real now, so none of it is needed.
	h := hubforge.NewHub(t, ".")

	// Create the worktree via Topology.Add, which -- per batch 5's eager wiring -- already wires the
	// new pair's junctions itself, reading the wired name-set from the real repo-wide fabric.yaml.
	// Without this the worktree has no _lyx, so configengine.Edit→FindBaseDir would error.
	_, err := h.Topology.Add(h.Location, slug, fabricengine.AddOptions{SkipPush: true})
	if err != nil {
		t.Fatalf("Topology.Add(%q): %v", slug, err)
	}

	// Resolve layout for the new worktree.
	codeWorktreePath := fabricengine.WorktreePath(h.Location, slug)
	codeLayout, err := lyxcwd.Resolve(codeWorktreePath)
	if err != nil {
		t.Fatalf("lyxcwd.Resolve(%q): %v", codeWorktreePath, err)
	}

	// NOTE: This test must NOT call t.Parallel(): it calls t.Setenv("WEFT_SKIP_GIT", …) and
	// t.Setenv("WEFT_SKIP_PUSH", …) below, which panic under t.Parallel() exactly as t.Chdir did.

	// Explicitly clear WEFT_SKIP_GIT and WEFT_SKIP_PUSH so the commit is not a silent no-op.
	t.Setenv("WEFT_SKIP_GIT", "")
	t.Setenv("WEFT_SKIP_PUSH", "")

	// Create a fake editor that writes valid YAML.
	// loom is per-worktree, so this exercises the fabric-sync path;
	// a hub-wide module commits in _board instead (see TestConfigHubWideCommitsInBoard).
	validYAML := "discussion_timeout_min: 60\n"
	fakeEdit := func(path string) error {
		return os.WriteFile(path, []byte(validYAML), 0o644)
	}

	// Create an injected sync function that calls fabriccli.RunCLIIn with "commit" instead of "sync".
	// (sync calls a detached spawnPush that cannot run in-process, so we use commit.) The cwd is not
	// a configcli dependence at all: dispatch is already given an explicit layout above, so this is
	// the one seam the injectedSync closure needs its own cwd for.
	injectedSync := func(w io.Writer) int {
		return fabriccli.RunCLIIn(codeWorktreePath, w, []string{"commit"})
	}

	// Run dispatch with the fake editor and injected sync.
	var out bytes.Buffer
	code := dispatch(codeLayout, &out, []string{"loom"}, fakeEdit, injectedSync, nil, false, nil)

	// Assert dispatch succeeded.
	if code != 0 {
		t.Errorf("dispatch() = %d; want 0; output: %s", code, out.String())
	}

	// Assert _lyx/config/loom.yaml is tracked/committed in the fabric worktree.
	fabricWorktreePath := fabricengine.WeftWorktreePath(h.Location, slug)
	configRelPath := configengine.ConfigFile(".", "loom")
	configPath := filepath.Join(fabricWorktreePath, configRelPath)
	// For git commands, use forward slashes (git always uses forward slashes).
	configRelPathForGit := strings.ReplaceAll(configRelPath, "\\", "/")

	// Verify the file exists in the fabric worktree filesystem.
	configContent, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("failed to read config file from fabric worktree at %s: %v", configPath, err)
	}

	// Verify the content matches what we wrote.
	if string(configContent) != validYAML {
		t.Errorf("loom config content mismatch; got %q, want %q", string(configContent), validYAML)
	}

	// Verify it's tracked in git (git ls-files should list it).
	cmd := exec.Command("git", "ls-files", configRelPathForGit)
	cmd.Dir = fabricWorktreePath
	lsFilesOut, err := cmd.Output()
	if err != nil {
		t.Fatalf("git ls-files failed: %v", err)
	}
	if !strings.Contains(string(lsFilesOut), configRelPathForGit) {
		t.Errorf("config file not tracked in fabric worktree; git ls-files output: %q", string(lsFilesOut))
	}

	// Verify the code side's git does NOT list the config file (it should be excluded).
	cmd = exec.Command("git", "ls-files")
	cmd.Dir = codeWorktreePath
	allFilesOut, err := cmd.Output()
	if err != nil {
		t.Fatalf("code-side git ls-files failed: %v", err)
	}
	if strings.Contains(string(allFilesOut), "_lyx") {
		t.Errorf("_lyx should be excluded from code-side git tracking; git ls-files output: %q", string(allFilesOut))
	}

	// Assert output contains success message.
	outStr := out.String()
	if !strings.Contains(outStr, "edited and synced") {
		t.Errorf("dispatch output missing success message; got %q", outStr)
	}

	// Assert the output is a JSON envelope with ok:true and module:"loom",
	// the same shape Card 7/8 introduced, exercised here end-to-end through
	// the real dispatch/fabriccli.RunCLI("commit") path rather than a fake sync.
	var env map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(outStr)), &env); err != nil {
		t.Fatalf("dispatch output is not valid JSON: %v; got %q", err, outStr)
	}
	if ok, _ := env["ok"].(bool); !ok {
		t.Errorf("dispatch output envelope ok = %v; want true; got %q", env["ok"], outStr)
	}
	if module, _ := env["module"].(string); module != "loom" {
		t.Errorf("dispatch output envelope module = %q; want \"loom\"; got %q", module, outStr)
	}
}

// TestConfigHubWideCommitsInBoard runs `lyx config <hub-wide module> --set` from a pair over a real hub
// and verifies the hub file changed and was committed in _board, with no per-worktree copy written.
func TestConfigHubWideCommitsInBoard(t *testing.T) {
	const slug = "config-hubwide-test"

	h := hubforge.NewHub(t, ".")
	if _, err := h.Topology.Add(h.Location, slug, fabricengine.AddOptions{SkipPush: true}); err != nil {
		t.Fatalf("Topology.Add(%q): %v", slug, err)
	}
	pairPath := fabricengine.WorktreePath(h.Location, slug)
	pairRecordsPath := fabricengine.WeftWorktreePath(h.Location, slug)
	boardDir := fabricengine.BoardDir(h.Location.HubPath)

	// NOTE: no t.Parallel(): t.Setenv panics under it.
	t.Setenv("WEFT_SKIP_GIT", "")
	t.Setenv("WEFT_SKIP_PUSH", "")

	rows := []struct {
		module string
		set    string
		want   string
	}{
		{module: "board", set: "labels.x=desc", want: "x: desc"},
		{module: "fabric", set: "branch_prefix=x", want: "branch_prefix: x"},
	}
	for _, row := range rows {
		t.Run(row.module, func(t *testing.T) {
			var out bytes.Buffer
			if code := RunCLIIn(pairPath, &out, []string{row.module, "--set", row.set}); code != 0 {
				t.Fatalf("lyx config %s --set %s = %d; output: %s", row.module, row.set, code, out.String())
			}

			hubFile := configengine.ConfigFile(boardDir, row.module)
			data, err := os.ReadFile(hubFile)
			if err != nil {
				t.Fatalf("read hub %s.yaml: %v", row.module, err)
			}
			if !strings.Contains(string(data), row.want) {
				t.Errorf("hub %s.yaml lacks %q; got %q", row.module, row.want, data)
			}

			status := exec.Command("git", "status", "--porcelain")
			status.Dir = boardDir
			statusOut, err := status.Output()
			if err != nil {
				t.Fatalf("git status in _board: %v", err)
			}
			if strings.TrimSpace(string(statusOut)) != "" {
				t.Errorf("_board is dirty after the edit: %q", statusOut)
			}

			show := exec.Command("git", "show", "--name-only", "--pretty=format:", "HEAD")
			show.Dir = boardDir
			showOut, err := show.Output()
			if err != nil {
				t.Fatalf("git show in _board: %v", err)
			}
			wantRel := strings.ReplaceAll(configengine.ConfigFileRel(row.module), "\\", "/")
			if strings.TrimSpace(string(showOut)) != wantRel {
				t.Errorf("HEAD in _board touches %q; want only %q", showOut, wantRel)
			}

			if _, err := os.Stat(configengine.ConfigFile(pairRecordsPath, row.module)); !os.IsNotExist(err) {
				t.Errorf("a per-worktree %s.yaml exists; stat err = %v", row.module, err)
			}
		})
	}
}

// TestDispatchSet_PreservedKeyDetectedByReconcile is the end-to-end test that closes the loop on
// the task's second symptom: reconcile "not detecting drift".
// It chains --set into reconcile so that a preserved orphan key planted by --set is then correctly
// reported by reconcile's own drift-detection, proving reconcile never gets a chance to look once
// --set stops silently destroying the key first.
//
// Uses "loom" rather than a hub-wide module: ReconcileAll only reports a hub-wide module's
// per-worktree copy (see ReconcileAll's doc comment), so a per-worktree module is needed to
// exercise this drift-detection path;
// the scenario under test (a preserved orphan key surviving --set, then reported by reconcile)
// is not module-specific.
func TestDispatchSet_PreservedKeyDetectedByReconcile(t *testing.T) {
	tmpDir := t.TempDir()

	// Initialize a minimal git repo so lyxcwd.Resolve works for the
	// reconcile call below (RunCLI resolves its layout from cwd).
	_, _, exitCode, err := gitexec.RunGit([]string{"init"}, tmpDir)
	if err != nil || exitCode != 0 {
		t.Fatalf("git init failed: %v (exit code %d)", err, exitCode)
	}

	seedModuleConfig(t, tmpDir, "loom", "discussion_timeout_min: 480\nlegacy_key: keepme\n")

	// Run --set via dispatch, exactly as
	// TestDispatchSet_PreservesUnrecognizedKeyReportsWarning does, using an
	// explicit *lyxcwd.Location (dispatch takes one directly, unlike
	// RunCLI which resolves it from cwd).
	var setOut bytes.Buffer
	setCode := dispatch(makeLayoutAt(tmpDir), &setOut, []string{"loom"}, makeNeverCalledEditor(t), (&fakeSyncTracker{exitCode: 0}).syncFunc(), nil, false, []string{"discussion_timeout_min=60"})
	if setCode != 0 {
		t.Fatalf("dispatch(--set) = %d; want 0; output: %q", setCode, setOut.String())
	}

	var reconcileOut bytes.Buffer
	reconcileCode := RunCLIIn(tmpDir, &reconcileOut, []string{"reconcile"})
	if reconcileCode != 0 {
		t.Fatalf("RunCLI(reconcile) = %d; want 0; output: %q", reconcileCode, reconcileOut.String())
	}

	var result map[string]any
	if err := json.Unmarshal(reconcileOut.Bytes(), &result); err != nil {
		t.Fatalf("parse JSON: %v, output: %s", err, reconcileOut.String())
	}
	modules, ok := result["modules"].([]any)
	if !ok {
		t.Fatalf("modules is not an array; got %v", result)
	}
	var loomMod map[string]any
	for _, m := range modules {
		mod, ok := m.(map[string]any)
		if !ok {
			continue
		}
		if mod["module"] == "loom" {
			loomMod = mod
			break
		}
	}
	if loomMod == nil {
		t.Fatalf("no modules entry for \"loom\"; got %v", modules)
	}
	removed, ok := loomMod["removed"].([]any)
	if !ok {
		t.Fatalf("loom module entry missing \"removed\" field or wrong type; got %v", loomMod)
	}
	found := false
	for _, r := range removed {
		if r == "legacy_key" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("loom module's removed = %v; want it to contain \"legacy_key\"", removed)
	}
}
