//go:build integration

// configcli_integration_test.go — e2e integration tests for configcli.
// Tests real weft.RunCLI over CopyPaired fixtures, plus the --set/reconcile
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

	"github.com/Knatte18/loomyard/internal/configreg"
	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/hubgeometry"
	"github.com/Knatte18/loomyard/internal/lyxtest"
	"github.com/Knatte18/loomyard/internal/warpengine"
	"github.com/Knatte18/loomyard/internal/weftcli"
)

// TestE2ESyncIntegration is an e2e test using CopyPaired: creates a host worktree with
// dispatch, edits a config, and verifies the file is tracked in the weft repo while the
// host stays pristine.
func TestE2ESyncIntegration(t *testing.T) {
	const slug = "config-e2e-test"

	// Build paired fixture (host + weft).
	f := lyxtest.CopyPaired(t)

	// Seed the weft-prime fixture with real config templates that weft.RunCLI will need.
	seeds := make(map[string]string)
	for _, m := range configreg.Modules() {
		seeds[m.Name] = m.Template()
	}
	lyxtest.SeedConfig(t, f.WeftPrime, seeds)

	// FIRST: Create the host worktree via warpengine.New().Add() (which is dormant).
	// Then wire the host _lyx junction via WireJunctions.
	// Without this the host worktree has no _lyx, so configengine.Edit→FindBaseDir would error.
	w := warpengine.New(warpengine.Config{})
	_, err := w.Add(f.Layout, slug, warpengine.AddOptions{SkipPush: true})
	if err != nil {
		t.Fatalf("worktree.Add(%q): %v", slug, err)
	}

	// Wire junctions for the new host worktree.
	if err := warpengine.WireJunctions(f.Layout, slug); err != nil {
		t.Fatalf("WireJunctions(%q): %v", slug, err)
	}

	// Resolve layout for the new host worktree.
	hostWorktreePath := f.Layout.WorktreePath(slug)
	hostLayout, err := hubgeometry.Resolve(hostWorktreePath)
	if err != nil {
		t.Fatalf("hubgeometry.Resolve(%q): %v", hostWorktreePath, err)
	}

	// Chdir into the host worktree so weft.RunCLI's cwd resolution lands on the fixture.
	// NOTE: This test must NOT call t.Parallel() due to t.Chdir.
	t.Chdir(hostWorktreePath)

	// Explicitly clear WEFT_SKIP_GIT and WEFT_SKIP_PUSH so the commit is not a silent no-op.
	t.Setenv("WEFT_SKIP_GIT", "")
	t.Setenv("WEFT_SKIP_PUSH", "")

	// Create a fake editor that writes valid YAML.
	validYAML := "branch_prefix: test-prefix\n"
	fakeEdit := func(path string) error {
		return os.WriteFile(path, []byte(validYAML), 0o644)
	}

	// Create an injected sync function that calls weft.RunCLI with "commit" instead of "sync".
	// (sync calls a detached spawnPush that cannot run in-process, so we use commit.)
	injectedSync := func(w io.Writer) int {
		return weftcli.RunCLI(w, []string{"commit"})
	}

	// Run dispatch with the fake editor and injected sync.
	var out bytes.Buffer
	code := dispatch(hostLayout, os.Stdin, &out, []string{"warp"}, fakeEdit, injectedSync, false, nil)

	// Assert dispatch succeeded.
	if code != 0 {
		t.Errorf("dispatch() = %d; want 0; output: %s", code, out.String())
	}

	// Assert _lyx/config/warpengine.yaml is tracked/committed in the weft worktree.
	weftWorktreePath := f.Layout.WeftWorktreePath(slug)
	configRelPath := hubgeometry.ConfigFile(".", "warp")
	configPath := filepath.Join(weftWorktreePath, configRelPath)
	// For git commands, use forward slashes (git always uses forward slashes).
	configRelPathForGit := strings.ReplaceAll(configRelPath, "\\", "/")

	// Verify the file exists in the weft worktree filesystem.
	configContent, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("failed to read config file from weft worktree at %s: %v", configPath, err)
	}

	// Verify the content matches what we wrote.
	if string(configContent) != validYAML {
		t.Errorf("weft config content mismatch; got %q, want %q", string(configContent), validYAML)
	}

	// Verify it's tracked in git (git ls-files should list it).
	cmd := exec.Command("git", "ls-files", configRelPathForGit)
	cmd.Dir = weftWorktreePath
	lsFilesOut, err := cmd.Output()
	if err != nil {
		t.Fatalf("git ls-files failed: %v", err)
	}
	if !strings.Contains(string(lsFilesOut), configRelPathForGit) {
		t.Errorf("config file not tracked in weft worktree; git ls-files output: %q", string(lsFilesOut))
	}

	// Verify the host worktree's git does NOT list the config file (it should be excluded).
	cmd = exec.Command("git", "ls-files")
	cmd.Dir = hostWorktreePath
	allFilesOut, err := cmd.Output()
	if err != nil {
		t.Fatalf("host git ls-files failed: %v", err)
	}
	if strings.Contains(string(allFilesOut), "_lyx") {
		t.Errorf("_lyx should be excluded from host git tracking; git ls-files output: %q", string(allFilesOut))
	}

	// Assert output contains success message.
	outStr := out.String()
	if !strings.Contains(outStr, "edited and synced") {
		t.Errorf("dispatch output missing success message; got %q", outStr)
	}

	// Assert the output is a JSON envelope with ok:true and module:"warp",
	// the same shape Card 7/8 introduced, exercised here end-to-end through
	// the real dispatch/weftcli.RunCLI("commit") path rather than a fake sync.
	var env map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(outStr)), &env); err != nil {
		t.Fatalf("dispatch output is not valid JSON: %v; got %q", err, outStr)
	}
	if ok, _ := env["ok"].(bool); !ok {
		t.Errorf("dispatch output envelope ok = %v; want true; got %q", env["ok"], outStr)
	}
	if module, _ := env["module"].(string); module != "warp" {
		t.Errorf("dispatch output envelope module = %q; want \"warp\"; got %q", module, outStr)
	}
}

// TestDispatchSet_PreservedKeyDetectedByReconcile is the end-to-end test that
// closes the loop on the task's second symptom: reconcile "not detecting
// drift". It chains --set into reconcile so that a preserved orphan key
// planted by --set is then correctly reported by reconcile's own
// drift-detection, proving reconcile never gets a chance to look once --set
// stops silently destroying the key first.
func TestDispatchSet_PreservedKeyDetectedByReconcile(t *testing.T) {
	tmpDir := t.TempDir()

	// Initialize a minimal git repo so hubgeometry.Resolve works for the
	// reconcile call below (RunCLI resolves its layout from cwd).
	_, _, exitCode, err := gitexec.RunGit([]string{"init"}, tmpDir)
	if err != nil || exitCode != 0 {
		t.Fatalf("git init failed: %v (exit code %d)", err, exitCode)
	}

	seedModuleConfig(t, tmpDir, "warp", "branch_prefix: old-\nlegacy_key: keepme\n")

	// Run --set via dispatch, exactly as
	// TestDispatchSet_PreservesUnrecognizedKeyReportsWarning does, using an
	// explicit *hubgeometry.Layout (dispatch takes one directly, unlike
	// RunCLI which resolves it from cwd).
	var setOut bytes.Buffer
	setCode := dispatch(makeLayoutAt(tmpDir), nil, &setOut, []string{"warp"}, makeNeverCalledEditor(t), (&fakeSyncTracker{exitCode: 0}).syncFunc(), false, []string{"branch_prefix=new-"})
	if setCode != 0 {
		t.Fatalf("dispatch(--set) = %d; want 0; output: %q", setCode, setOut.String())
	}

	// Chdir into the temp repo so hubgeometry.Getwd inside RunCLI resolves
	// there, then run reconcile.
	oldCwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer os.Chdir(oldCwd) //nolint:errcheck

	var reconcileOut bytes.Buffer
	reconcileCode := RunCLI(&reconcileOut, []string{"reconcile"})
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
	var warpMod map[string]any
	for _, m := range modules {
		mod, ok := m.(map[string]any)
		if !ok {
			continue
		}
		if mod["module"] == "warp" {
			warpMod = mod
			break
		}
	}
	if warpMod == nil {
		t.Fatalf("no modules entry for \"warp\"; got %v", modules)
	}
	removed, ok := warpMod["removed"].([]any)
	if !ok {
		t.Fatalf("warp module entry missing \"removed\" field or wrong type; got %v", warpMod)
	}
	found := false
	for _, r := range removed {
		if r == "legacy_key" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("warp module's removed = %v; want it to contain \"legacy_key\"", removed)
	}
}
