// config_test.go — unit tests for fabricengine.LoadConfig.
//
// Covers: happy-path with template keys present, branch_prefix/pathspec parsing,
// environment variable resolution, and not-initialized error path. Mirrors
// warpengine's config_test.go.

package fabricengine_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubgeometry"
)

// TestLoadConfig_HappyPath tests that LoadConfig loads a valid config
// with all template keys present and both fields are parsed correctly.
func TestLoadConfig_HappyPath(t *testing.T) {
	tmpDir := t.TempDir()

	// Create _lyx/config/ directories
	lyxDir := filepath.Join(tmpDir, hubgeometry.LyxDirName)
	if err := os.Mkdir(lyxDir, 0755); err != nil {
		t.Fatalf("failed to create _lyx: %v", err)
	}
	configDir := hubgeometry.ConfigDir(tmpDir)
	if err := os.Mkdir(configDir, 0755); err != nil {
		t.Fatalf("failed to create _lyx/config: %v", err)
	}

	// Write a config file with both keys
	configFile := hubgeometry.ConfigFile(tmpDir, "fabric")
	content := "branch_prefix: hanf/\npathspec: _lyx _raddle\n"
	if err := os.WriteFile(configFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	cfg, err := fabricengine.LoadConfig(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.BranchPrefix != "hanf/" {
		t.Errorf("BranchPrefix = %q; want %q", cfg.BranchPrefix, "hanf/")
	}
	if cfg.Pathspec != "_lyx _raddle" {
		t.Errorf("Pathspec = %q; want %q", cfg.Pathspec, "_lyx _raddle")
	}
	wantDirs := []string{"_lyx", "_raddle"}
	gotDirs := cfg.Dirs()
	if len(gotDirs) != len(wantDirs) {
		t.Fatalf("Dirs() = %v; want %v", gotDirs, wantDirs)
	}
	for i := range wantDirs {
		if gotDirs[i] != wantDirs[i] {
			t.Errorf("Dirs()[%d] = %q; want %q", i, gotDirs[i], wantDirs[i])
		}
	}
}

// TestLoadConfig_EmptyBranchPrefix tests that branch_prefix defaults to empty string.
func TestLoadConfig_EmptyBranchPrefix(t *testing.T) {
	tmpDir := t.TempDir()

	lyxDir := filepath.Join(tmpDir, hubgeometry.LyxDirName)
	if err := os.Mkdir(lyxDir, 0755); err != nil {
		t.Fatalf("failed to create _lyx: %v", err)
	}
	configDir := hubgeometry.ConfigDir(tmpDir)
	if err := os.Mkdir(configDir, 0755); err != nil {
		t.Fatalf("failed to create _lyx/config: %v", err)
	}

	configFile := hubgeometry.ConfigFile(tmpDir, "fabric")
	content := "branch_prefix: \"\"\npathspec: _lyx\n"
	if err := os.WriteFile(configFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	cfg, err := fabricengine.LoadConfig(tmpDir)
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
	t.Setenv("TEST_FABRIC_BRANCH_PREFIX", "feature/")

	lyxDir := filepath.Join(tmpDir, hubgeometry.LyxDirName)
	if err := os.Mkdir(lyxDir, 0755); err != nil {
		t.Fatalf("failed to create _lyx: %v", err)
	}
	configDir := hubgeometry.ConfigDir(tmpDir)
	if err := os.Mkdir(configDir, 0755); err != nil {
		t.Fatalf("failed to create _lyx/config: %v", err)
	}

	configFile := hubgeometry.ConfigFile(tmpDir, "fabric")
	content := "branch_prefix: ${env:TEST_FABRIC_BRANCH_PREFIX}\npathspec: _lyx\n"
	if err := os.WriteFile(configFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	cfg, err := fabricengine.LoadConfig(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.BranchPrefix != "feature/" {
		t.Errorf("expected BranchPrefix %q (from env), got %q", "feature/", cfg.BranchPrefix)
	}
}

// TestLoadConfig_NotInitialized tests that missing _lyx/ returns the
// fabric-specific not-initialized error.
func TestLoadConfig_NotInitialized(t *testing.T) {
	tmpDir := t.TempDir()
	// Do NOT create _lyx/

	cfg, err := fabricengine.LoadConfig(tmpDir)
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
