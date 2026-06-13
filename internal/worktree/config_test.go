// config_test.go — unit tests for the Config system (config.go).
//
// Covers: defaults, error on uninitialized, and branch_prefix parsing from YAML.

package worktree_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/mhgo/internal/worktree"
)

// TestDefaultsReturned tests that defaults are returned when _mhgo/ exists
// but worktree.yaml is absent.
func TestDefaultsReturned(t *testing.T) {
	baseDir := t.TempDir()

	// Create _mhgo/ directory (empty, no worktree.yaml)
	mhgoDir := filepath.Join(baseDir, "_mhgo")
	if err := os.Mkdir(mhgoDir, 0755); err != nil {
		t.Fatalf("failed to create _mhgo directory: %v", err)
	}

	cfg, err := worktree.LoadConfig(baseDir, "worktree")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.BranchPrefix != "" {
		t.Errorf("expected empty BranchPrefix, got %q", cfg.BranchPrefix)
	}
}

// TestBranchPrefixFromYAML tests that branch_prefix is parsed from worktree.yaml.
func TestBranchPrefixFromYAML(t *testing.T) {
	baseDir := t.TempDir()

	// Create _mhgo/ directory
	mhgoDir := filepath.Join(baseDir, "_mhgo")
	if err := os.Mkdir(mhgoDir, 0755); err != nil {
		t.Fatalf("failed to create _mhgo: %v", err)
	}

	// Write _mhgo/worktree.yaml with branch_prefix
	mhgoFile := filepath.Join(mhgoDir, "worktree.yaml")
	if err := os.WriteFile(mhgoFile, []byte("branch_prefix: hanf/\n"), 0644); err != nil {
		t.Fatalf("failed to write _mhgo/worktree.yaml: %v", err)
	}

	cfg, err := worktree.LoadConfig(baseDir, "worktree")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.BranchPrefix != "hanf/" {
		t.Errorf("expected BranchPrefix %q, got %q", "hanf/", cfg.BranchPrefix)
	}
}

// TestErrorNotInitialized tests that an error containing "not initialized"
// is returned when _mhgo/ directory does not exist.
func TestErrorNotInitialized(t *testing.T) {
	baseDir := t.TempDir()

	// Do not create _mhgo/ directory

	cfg, err := worktree.LoadConfig(baseDir, "worktree")
	if err == nil {
		t.Fatalf("expected error, got nil; config: %+v", cfg)
	}

	errMsg := err.Error()
	if !strings.Contains(errMsg, "run \"mhgo init\"") {
		t.Errorf("expected error message to contain 'run \"mhgo init\"', got: %s", errMsg)
	}
}
