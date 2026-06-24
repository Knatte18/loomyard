// config_test.go — unit tests for worktree.LoadConfig.
//
// Covers: happy-path with template keys present, branch_prefix parsing,
// environment variable resolution, and not-initialized error path.

package worktree_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/paths"
	"github.com/Knatte18/loomyard/internal/worktree"
)

// TestLoadConfig_HappyPath tests that LoadConfig loads a valid config
// with all template keys present and branch_prefix is parsed correctly.
func TestLoadConfig_HappyPath(t *testing.T) {
	tmpDir := t.TempDir()

	// Create _lyx/config/ directories
	lyxDir := filepath.Join(tmpDir, "_lyx")
	if err := os.Mkdir(lyxDir, 0755); err != nil {
		t.Fatalf("failed to create _lyx: %v", err)
	}
	configDir := filepath.Join(lyxDir, "config")
	if err := os.Mkdir(configDir, 0755); err != nil {
		t.Fatalf("failed to create _lyx/config: %v", err)
	}

	// Write a config file with branch_prefix
	configFile := paths.ConfigFile(tmpDir, "worktree")
	content := `branch_prefix: hanf/
`
	if err := os.WriteFile(configFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	cfg, err := worktree.LoadConfig(tmpDir, "worktree")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.BranchPrefix != "hanf/" {
		t.Errorf("expected BranchPrefix %q, got %q", "hanf/", cfg.BranchPrefix)
	}
}

// TestLoadConfig_EmptyBranchPrefix tests that branch_prefix defaults to empty string.
func TestLoadConfig_EmptyBranchPrefix(t *testing.T) {
	tmpDir := t.TempDir()

	// Create _lyx/config/ directories
	lyxDir := filepath.Join(tmpDir, "_lyx")
	if err := os.Mkdir(lyxDir, 0755); err != nil {
		t.Fatalf("failed to create _lyx: %v", err)
	}
	configDir := filepath.Join(lyxDir, "config")
	if err := os.Mkdir(configDir, 0755); err != nil {
		t.Fatalf("failed to create _lyx/config: %v", err)
	}

	// Write a config file with empty branch_prefix
	configFile := paths.ConfigFile(tmpDir, "worktree")
	content := `branch_prefix: ""
`
	if err := os.WriteFile(configFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	cfg, err := worktree.LoadConfig(tmpDir, "worktree")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.BranchPrefix != "" {
		t.Errorf("expected empty BranchPrefix, got %q", cfg.BranchPrefix)
	}
}

// TestLoadConfig_EnvResolution tests that environment variables in config
// are resolved correctly.
func TestLoadConfig_EnvResolution(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("TEST_BRANCH_PREFIX", "feature/")

	// Create _lyx/config/ directories
	lyxDir := filepath.Join(tmpDir, "_lyx")
	if err := os.Mkdir(lyxDir, 0755); err != nil {
		t.Fatalf("failed to create _lyx: %v", err)
	}
	configDir := filepath.Join(lyxDir, "config")
	if err := os.Mkdir(configDir, 0755); err != nil {
		t.Fatalf("failed to create _lyx/config: %v", err)
	}

	// Write config with env variable
	configFile := paths.ConfigFile(tmpDir, "worktree")
	content := `branch_prefix: ${env:TEST_BRANCH_PREFIX}
`
	if err := os.WriteFile(configFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	cfg, err := worktree.LoadConfig(tmpDir, "worktree")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.BranchPrefix != "feature/" {
		t.Errorf("expected BranchPrefix %q (from env), got %q", "feature/", cfg.BranchPrefix)
	}
}

// TestLoadConfig_NotInitialized tests that missing _lyx/ returns the
// worktree-specific not-initialized error.
func TestLoadConfig_NotInitialized(t *testing.T) {
	tmpDir := t.TempDir()
	// Do NOT create _lyx/

	cfg, err := worktree.LoadConfig(tmpDir, "worktree")
	if err == nil {
		t.Fatalf("expected error for not initialized, got nil; config: %+v", cfg)
	}

	errMsg := err.Error()
	if !strings.Contains(errMsg, "not initialized") {
		t.Errorf("expected error containing 'not initialized', got: %v", err)
	}
	if !strings.Contains(errMsg, "lyx init") {
		t.Errorf("expected error containing 'lyx init', got: %v", err)
	}
}
