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

	"github.com/Knatte18/loomyard/internal/lyxtest"
	"github.com/Knatte18/loomyard/internal/paths"
	"github.com/Knatte18/loomyard/internal/weft"
	"github.com/Knatte18/loomyard/internal/worktree"
)

// TestE2ESyncIntegration is an e2e test using CopyPaired: creates a host worktree with
// dispatch, edits a config, and verifies the file is tracked in the weft repo while the
// host stays pristine.
func TestE2ESyncIntegration(t *testing.T) {
	const slug = "config-e2e-test"

	// Build paired fixture (host + weft).
	f := lyxtest.CopyPaired(t)

	// FIRST: Seed the host _lyx junction by running worktree.New().Add().
	// Without this the host worktree has no _lyx, so config.Edit→FindBaseDir would error.
	w := worktree.New(worktree.Config{})
	_, err := w.Add(f.Layout, slug, worktree.AddOptions{SkipPush: true})
	if err != nil {
		t.Fatalf("worktree.Add(%q): %v", slug, err)
	}

	// Resolve layout for the new host worktree.
	hostWorktreePath := f.Layout.WorktreePath(slug)
	hostLayout, err := paths.Resolve(hostWorktreePath)
	if err != nil {
		t.Fatalf("paths.Resolve(%q): %v", hostWorktreePath, err)
	}

	// Chdir into the host worktree so weft.RunCLI's cwd resolution lands on the fixture.
	// NOTE: This test must NOT call t.Parallel() due to t.Chdir.
	oldCwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	if err := os.Chdir(hostWorktreePath); err != nil {
		t.Fatalf("Chdir(%s): %v", hostWorktreePath, err)
	}
	defer os.Chdir(oldCwd) // Restore cwd on exit.

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
	baseDir := filepath.Join(hostLayout.WorktreeRoot, hostLayout.RelPath)
	code := editOne(baseDir, &out, "worktree", fakeEdit, injectedSync)

	// Assert dispatch succeeded.
	if code != 0 {
		t.Errorf("editOne() = %d; want 0; output: %s", code, out.String())
	}

	// Assert _lyx/config/worktree.yaml is tracked/committed in the weft worktree.
	weftWorktreePath := f.Layout.WeftWorktreePath(slug)
	configRelPath := filepath.Join("_lyx", "config", "worktree.yaml")
	configPath := filepath.Join(weftWorktreePath, configRelPath)

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
	cmd := exec.Command("git", "ls-files", configRelPath)
	cmd.Dir = weftWorktreePath
	lsFilesOut, err := cmd.Output()
	if err != nil {
		t.Fatalf("git ls-files failed: %v", err)
	}
	if strings.TrimSpace(string(lsFilesOut)) != configRelPath {
		t.Errorf("config file not tracked in weft worktree; git ls-files output: %q", string(lsFilesOut))
	}

	// Assert the host worktree's _lyx/config is pristine (excluded via .git/info/exclude).
	hostConfigPath := filepath.Join(hostWorktreePath, "_lyx", "config", "worktree.yaml")
	if _, err := os.Stat(hostConfigPath); !os.IsNotExist(err) {
		t.Errorf("host worktree should not have config file (via exclusion), but found at %s", hostConfigPath)
	}

	// Verify the host worktree's git does NOT list the config file.
	cmd = exec.Command("git", "ls-files", configRelPath)
	cmd.Dir = hostWorktreePath
	hostLsFilesOut, err := cmd.Output()
	if err != nil {
		t.Fatalf("host git ls-files failed: %v", err)
	}
	if strings.TrimSpace(string(hostLsFilesOut)) != "" {
		t.Errorf("config file should not be tracked in host worktree; git ls-files output: %q", string(hostLsFilesOut))
	}

	// Assert output contains success message.
	outStr := out.String()
	if !strings.Contains(outStr, "edited and synced") {
		t.Errorf("dispatch output missing success message; got %q", outStr)
	}
}
