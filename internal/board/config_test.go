// config_test.go — unit tests for board.LoadConfig.
//
// Covers: happy-path with template keys present, missing-key error,
// absolute and relative path resolution, environment variable resolution,
// and not-initialized error path.

package board_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/board"
	"github.com/Knatte18/loomyard/internal/paths"
)

// TestLoadConfig_HappyPath tests that LoadConfig loads a valid config
// with all template keys present and resolves environment variables.
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

	// Write a config file with all template keys
	configFile := paths.ConfigFile(tmpDir, "board")
	content := `path: _custom_board
home: Home.md
sidebar: _Sidebar.md
proposal_prefix: proposal-
`
	if err := os.WriteFile(configFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	cfg, err := board.LoadConfig(tmpDir, "board")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify relative path was resolved
	if !strings.HasSuffix(cfg.Path, "_custom_board") {
		t.Errorf("expected path to end with %q, got %q", "_custom_board", cfg.Path)
	}
	if cfg.Home != "Home.md" {
		t.Errorf("expected Home %q, got %q", "Home.md", cfg.Home)
	}
	if cfg.Sidebar != "_Sidebar.md" {
		t.Errorf("expected Sidebar %q, got %q", "_Sidebar.md", cfg.Sidebar)
	}
	if cfg.ProposalPrefix != "proposal-" {
		t.Errorf("expected ProposalPrefix %q, got %q", "proposal-", cfg.ProposalPrefix)
	}
}

// TestLoadConfig_AbsolutePathResolution tests that absolute paths in config
// are passed through unchanged.
func TestLoadConfig_AbsolutePathResolution(t *testing.T) {
	tmpDir := t.TempDir()
	absBoard := t.TempDir()

	// Create _lyx/config/ directories
	lyxDir := filepath.Join(tmpDir, "_lyx")
	if err := os.Mkdir(lyxDir, 0755); err != nil {
		t.Fatalf("failed to create _lyx: %v", err)
	}
	configDir := filepath.Join(lyxDir, "config")
	if err := os.Mkdir(configDir, 0755); err != nil {
		t.Fatalf("failed to create _lyx/config: %v", err)
	}

	// Write config with absolute path
	configFile := paths.ConfigFile(tmpDir, "board")
	content := `path: ` + absBoard + `
home: Home.md
sidebar: _Sidebar.md
proposal_prefix: proposal-
`
	if err := os.WriteFile(configFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	cfg, err := board.LoadConfig(tmpDir, "board")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Path != absBoard {
		t.Errorf("expected path %q, got %q", absBoard, cfg.Path)
	}
}

// TestLoadConfig_RelativePathResolution tests that relative paths are resolved
// relative to baseDir.
func TestLoadConfig_RelativePathResolution(t *testing.T) {
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

	// Write config with relative path
	configFile := paths.ConfigFile(tmpDir, "board")
	content := `path: ../custom_board
home: Home.md
sidebar: _Sidebar.md
proposal_prefix: proposal-
`
	if err := os.WriteFile(configFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	cfg, err := board.LoadConfig(tmpDir, "board")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := filepath.Join(tmpDir, "../custom_board")
	if cfg.Path != expected {
		t.Errorf("expected path %q, got %q", expected, cfg.Path)
	}
}

// TestLoadConfig_EnvResolution tests that environment variables in config
// are resolved correctly.
func TestLoadConfig_EnvResolution(t *testing.T) {
	tmpDir := t.TempDir()
	absBoard := t.TempDir()
	t.Setenv("TEST_BOARD_PATH", absBoard)

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
	configFile := paths.ConfigFile(tmpDir, "board")
	content := `path: ${env:TEST_BOARD_PATH}
home: Home.md
sidebar: _Sidebar.md
proposal_prefix: proposal-
`
	if err := os.WriteFile(configFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	cfg, err := board.LoadConfig(tmpDir, "board")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Path != absBoard {
		t.Errorf("expected path %q (from env), got %q", absBoard, cfg.Path)
	}
}

// TestLoadConfig_NotInitialized tests that missing _lyx/ returns the
// board-specific not-initialized error.
func TestLoadConfig_NotInitialized(t *testing.T) {
	tmpDir := t.TempDir()
	// Do NOT create _lyx/

	cfg, err := board.LoadConfig(tmpDir, "board")
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

// TestOutputs tests the Outputs() method on Config.
func TestOutputs(t *testing.T) {
	cfg := board.Config{
		Path:           "/some/path",
		Home:           "Home.md",
		Sidebar:        "_Sidebar.md",
		ProposalPrefix: "proposal-",
	}

	out := cfg.Outputs()

	if out.Home != "Home.md" {
		t.Errorf("expected Home %q, got %q", "Home.md", out.Home)
	}
	if out.Sidebar != "_Sidebar.md" {
		t.Errorf("expected Sidebar %q, got %q", "_Sidebar.md", out.Sidebar)
	}
	if out.ProposalPrefix != "proposal-" {
		t.Errorf("expected ProposalPrefix %q, got %q", "proposal-", out.ProposalPrefix)
	}
}
