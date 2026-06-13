package worktree_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/mhgo/internal/worktree"
)

// TestRunCLI_List tests the "list" subcommand.
func TestRunCLI_List(t *testing.T) {
	// Create a test repository
	hub := newTestRepo(t)

	// Change to the hub directory
	t.Chdir(hub)

	// Create _mhgo directory and worktree.yaml config
	mhgoDir := filepath.Join(hub, "_mhgo")
	if err := os.MkdirAll(mhgoDir, 0755); err != nil {
		t.Fatalf("failed to create _mhgo directory: %v", err)
	}

	configFile := filepath.Join(mhgoDir, "worktree.yaml")
	if err := os.WriteFile(configFile, []byte("branch_prefix: wt-\n"), 0644); err != nil {
		t.Fatalf("failed to write worktree.yaml: %v", err)
	}

	// Run the list subcommand
	var buf bytes.Buffer
	exitCode := worktree.RunCLI(&buf, []string{"list"})

	// Check exit code is 0
	if exitCode != 0 {
		t.Errorf("expected exit code 0, got %d", exitCode)
	}

	// Parse the JSON output
	var result map[string]any
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse JSON output: %v\nOutput: %s", err, buf.String())
	}

	// Verify ok:true
	ok, ok_exists := result["ok"].(bool)
	if !ok_exists || !ok {
		t.Errorf("expected ok:true in output, got %v", result)
	}

	// Verify worktrees array exists and has length 1 (the hub itself)
	worktrees, worktrees_exists := result["worktrees"].([]any)
	if !worktrees_exists {
		t.Errorf("expected worktrees array in output, got %v", result)
	}
	if len(worktrees) != 1 {
		t.Errorf("expected worktrees array of length 1, got %d", len(worktrees))
	}
}

// TestRunCLI_UnknownSubcommand tests the unknown subcommand error handling.
func TestRunCLI_UnknownSubcommand(t *testing.T) {
	// Create a test repository
	hub := newTestRepo(t)

	// Change to the hub directory
	t.Chdir(hub)

	// Create _mhgo directory and worktree.yaml config
	mhgoDir := filepath.Join(hub, "_mhgo")
	if err := os.MkdirAll(mhgoDir, 0755); err != nil {
		t.Fatalf("failed to create _mhgo directory: %v", err)
	}

	configFile := filepath.Join(mhgoDir, "worktree.yaml")
	if err := os.WriteFile(configFile, []byte("branch_prefix: wt-\n"), 0644); err != nil {
		t.Fatalf("failed to write worktree.yaml: %v", err)
	}

	// Run with unknown subcommand
	var buf bytes.Buffer
	exitCode := worktree.RunCLI(&buf, []string{"bogus"})

	// Check exit code is 1
	if exitCode != 1 {
		t.Errorf("expected exit code 1, got %d", exitCode)
	}

	// Parse the JSON output
	var result map[string]any
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse JSON output: %v\nOutput: %s", err, buf.String())
	}

	// Verify ok:false
	ok, ok_exists := result["ok"].(bool)
	if !ok_exists || ok {
		t.Errorf("expected ok:false in output, got %v", result)
	}
}

// TestRunCLI_RemoveWithForceFlag tests the remove subcommand with --force flag parsing.
func TestRunCLI_RemoveWithForceFlag(t *testing.T) {
	// Create a test repository
	hub := newTestRepo(t)

	// Change to the hub directory
	t.Chdir(hub)

	// Create _mhgo directory and worktree.yaml config
	mhgoDir := filepath.Join(hub, "_mhgo")
	if err := os.MkdirAll(mhgoDir, 0755); err != nil {
		t.Fatalf("failed to create _mhgo directory: %v", err)
	}

	configFile := filepath.Join(mhgoDir, "worktree.yaml")
	if err := os.WriteFile(configFile, []byte("branch_prefix: wt-\n"), 0644); err != nil {
		t.Fatalf("failed to write worktree.yaml: %v", err)
	}

	// Add a remote
	bare := addRemote(t, hub)

	// Create a second worktree using git worktree add
	slug := "test-wt"
	branch := "wt-" + slug
	target := filepath.Join(filepath.Dir(hub), slug)

	mustRun(t, hub, "git", "worktree", "add", "-b", branch, target)

	// Run the remove subcommand with --force flag
	var buf bytes.Buffer
	exitCode := worktree.RunCLI(&buf, []string{"remove", "--force", slug})

	// Check exit code is 0
	if exitCode != 0 {
		t.Errorf("expected exit code 0, got %d\nOutput: %s", exitCode, buf.String())
	}

	// Parse the JSON output
	var result map[string]any
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse JSON output: %v\nOutput: %s", err, buf.String())
	}

	// Verify ok:true
	ok, ok_exists := result["ok"].(bool)
	if !ok_exists || !ok {
		t.Errorf("expected ok:true in output, got %v", result)
	}

	// Verify that the target worktree directory was removed
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Errorf("expected worktree directory %q to be removed, but it still exists", target)
	}

	_ = bare // silence unused warning
}
