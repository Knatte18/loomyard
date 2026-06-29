//go:build integration

// configcli_integration_test.go — e2e integration tests for configcli.
// Tests real weft.RunCLI over CopyPaired fixtures.

package configcli

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/configreg"
	"github.com/Knatte18/loomyard/internal/lyxtest"
	"github.com/Knatte18/loomyard/internal/paths"
	"github.com/Knatte18/loomyard/internal/warp"
	"github.com/Knatte18/loomyard/internal/weft"
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

	// FIRST: Create the host worktree via warp.New().Add() (which is dormant).
	// Then wire the host _lyx junction via WireJunctions.
	// Without this the host worktree has no _lyx, so configengine.Edit→FindBaseDir would error.
	w := warp.New(warp.Config{})
	_, err := w.Add(f.Layout, slug, warp.AddOptions{SkipPush: true})
	if err != nil {
		t.Fatalf("worktree.Add(%q): %v", slug, err)
	}

	// Wire junctions for the new host worktree.
	if err := warp.WireJunctions(f.Layout, slug); err != nil {
		t.Fatalf("WireJunctions(%q): %v", slug, err)
	}

	// Resolve layout for the new host worktree.
	hostWorktreePath := f.Layout.WorktreePath(slug)
	hostLayout, err := paths.Resolve(hostWorktreePath)
	if err != nil {
		t.Fatalf("paths.Resolve(%q): %v", hostWorktreePath, err)
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
		return weft.RunCLI(w, []string{"commit"})
	}

	// Run dispatch with the fake editor and injected sync.
	var out bytes.Buffer
	code := dispatch(hostLayout, os.Stdin, &out, []string{"warp"}, fakeEdit, injectedSync, false)

	// Assert dispatch succeeded.
	if code != 0 {
		t.Errorf("dispatch() = %d; want 0; output: %s", code, out.String())
	}

	// Assert _lyx/config/warp.yaml is tracked/committed in the weft worktree.
	weftWorktreePath := f.Layout.WeftWorktreePath(slug)
	configRelPath := paths.ConfigFile(".", "warp")
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
}
