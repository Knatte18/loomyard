// integration_test.go — integration test for configcli with real weft operations.
//
//go:build integration

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

// TestIntegrationEditAndSync tests e2e: edit a config module, sync to weft, verify tracking.
//
// Setup:
// 1. Create a CopyPaired fixture (host + weft repos)
// 2. Call worktree.New(worktree.Config{}).Add(...) to seed the host _lyx junction
// 3. Resolve layout for the new host worktree
// 4. t.Chdir into the host worktree
// 5. Clear WEFT_SKIP_* env so commits are not silently no-ops
//
// Test:
// 1. Call dispatch with a fake editor (writes valid YAML) and injected sync
// 2. Verify _lyx/config/<module>.yaml is tracked/committed in weft (git ls-files)
// 3. Verify host git ls-files does NOT list it (host stays pristine via .git/info/exclude)
func TestIntegrationEditAndSync(t *testing.T) {
	// Create paired fixture: host + weft repos
	f := lyxtest.CopyPaired(t)

	// Add a host worktree to seed the _lyx junction.
	// Using worktree.New(...).Add() with SkipPush: true.
	cfg := worktree.Config{}
	w := worktree.New(cfg)
	slug := "testslug"
	_, err := w.Add(f.Layout, slug, worktree.AddOptions{SkipPush: true})
	if err != nil {
		t.Fatalf("worktree.Add failed: %v", err)
	}

	// Resolve layout for the new host worktree
	hostWorktreePath := f.Layout.WorktreePath(slug)
	hostLayout, err := paths.Resolve(hostWorktreePath)
	if err != nil {
		t.Fatalf("paths.Resolve failed: %v", err)
	}

	// Chdir into the new host worktree so weft.RunCLI resolves correctly.
	// Note: this test cannot use t.Parallel() due to t.Chdir.
	oldCwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get current directory: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(oldCwd); err != nil {
			t.Logf("failed to restore directory: %v", err)
		}
	})

	if err := os.Chdir(hostWorktreePath); err != nil {
		t.Fatalf("failed to chdir to host worktree: %v", err)
	}

	// Clear WEFT_SKIP_* env so commits actually happen
	t.Setenv("WEFT_SKIP_GIT", "")
	t.Setenv("WEFT_SKIP_PUSH", "")

	// Prepare output buffer
	var out bytes.Buffer

	// Fake editor: writes valid YAML
	fakeEd := fakeEditor("branch_prefix: test-\n", nil)

	// Injected sync: use weft.RunCLI("commit") instead of "sync" to avoid push
	injectedSync := func(w io.Writer) int {
		return weft.RunCLI(w, []string{"commit"})
	}

	// Call dispatch with the "worktree" module to edit worktree.yaml
	code := dispatch(hostLayout, bytes.NewReader(nil), &out, []string{"worktree"}, fakeEd, injectedSync)
	if code != 0 {
		t.Fatalf("dispatch failed with exit code %d; output: %s", code, out.String())
	}

	// Verify the config file is tracked in the weft worktree
	weftWorktreePath := hostLayout.WeftWorktree()
	lsFilesOutput, err := exec.Command("git", "-C", weftWorktreePath, "ls-files").Output()
	if err != nil {
		t.Fatalf("git ls-files in weft failed: %v", err)
	}
	weftFiles := strings.TrimSpace(string(lsFilesOutput))
	if !strings.Contains(weftFiles, "_lyx/config/worktree.yaml") {
		t.Fatalf("expected _lyx/config/worktree.yaml to be tracked in weft, got files: %q", weftFiles)
	}

	// Verify the host git ls-files does NOT list _lyx/config/
	hostLsFilesOutput, err := exec.Command("git", "-C", hostWorktreePath, "ls-files").Output()
	if err != nil {
		t.Fatalf("git ls-files in host failed: %v", err)
	}
	hostFiles := strings.TrimSpace(string(hostLsFilesOutput))
	if strings.Contains(hostFiles, "_lyx/config/") {
		t.Fatalf("expected _lyx/config/ to NOT be tracked in host (pristine via exclude), got files: %q", hostFiles)
	}
}
