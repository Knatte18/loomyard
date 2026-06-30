// config_test.go — unit tests for boardengine.LoadConfig.
//
// Covers: happy-path with template keys present, missing-key error,
// absolute and relative path resolution, environment variable resolution,
// and not-initialized error path.

package boardengine_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/paths"
)

// TestLoadConfig_HappyPath tests that LoadConfig loads a valid config
// with all template keys present and resolves environment variables.
// LoadConfig no longer sets Config.Path; the caller does that via paths.BoardDir.
func TestLoadConfig_HappyPath(t *testing.T) {
	tmpDir := t.TempDir()

	// Create _lyx/config/ directories
	lyxDir := filepath.Join(tmpDir, paths.LyxDirName)
	if err := os.Mkdir(lyxDir, 0755); err != nil {
		t.Fatalf("failed to create _lyx: %v", err)
	}
	configDir := paths.ConfigDir(tmpDir)
	if err := os.Mkdir(configDir, 0755); err != nil {
		t.Fatalf("failed to create _lyx/config: %v", err)
	}

	// Write a config file with all template keys (path: is not a template key)
	configFile := paths.ConfigFile(tmpDir, "board")
	content := `home: Home.md
sidebar: _Sidebar.md
proposal_prefix: proposal-
`
	if err := os.WriteFile(configFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	cfg, err := boardengine.LoadConfig(tmpDir, "board")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Path is never set by LoadConfig; the caller sets it via paths.BoardDir.
	if cfg.Path != "" {
		t.Errorf("expected Path to be empty after LoadConfig; got %q", cfg.Path)
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

// TestLoadConfig_AbsolutePathResolution verifies that a path: key in the config
// file is ignored by LoadConfig because Config.Path has yaml:"-".
// The board data dir is geometry owned by paths.BoardDir; the config key is a no-op.
func TestLoadConfig_AbsolutePathResolution(t *testing.T) {
	tmpDir := t.TempDir()
	absBoard := t.TempDir()

	// Create _lyx/config/ directories
	lyxDir := filepath.Join(tmpDir, paths.LyxDirName)
	if err := os.Mkdir(lyxDir, 0755); err != nil {
		t.Fatalf("failed to create _lyx: %v", err)
	}
	configDir := paths.ConfigDir(tmpDir)
	if err := os.Mkdir(configDir, 0755); err != nil {
		t.Fatalf("failed to create _lyx/config: %v", err)
	}

	// Write config with an absolute path: key that should be ignored.
	configFile := paths.ConfigFile(tmpDir, "board")
	content := `path: ` + absBoard + `
home: Home.md
sidebar: _Sidebar.md
proposal_prefix: proposal-
`
	if err := os.WriteFile(configFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	cfg, err := boardengine.LoadConfig(tmpDir, "board")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// yaml:"-" means the path: key in the file is never mapped to Config.Path.
	if cfg.Path != "" {
		t.Errorf("expected Path to be empty (yaml:\"-\" ignores config key); got %q", cfg.Path)
	}
}

// TestLoadConfig_RelativePathResolution verifies that a relative path: key in the
// config file is ignored by LoadConfig because Config.Path has yaml:"-".
// LoadConfig no longer performs any relative-path resolution; the board data dir
// is geometry owned by paths.BoardDir.
func TestLoadConfig_RelativePathResolution(t *testing.T) {
	tmpDir := t.TempDir()

	// Create _lyx/config/ directories
	lyxDir := filepath.Join(tmpDir, paths.LyxDirName)
	if err := os.Mkdir(lyxDir, 0755); err != nil {
		t.Fatalf("failed to create _lyx: %v", err)
	}
	configDir := paths.ConfigDir(tmpDir)
	if err := os.Mkdir(configDir, 0755); err != nil {
		t.Fatalf("failed to create _lyx/config: %v", err)
	}

	// Write config with a relative path: key that should be ignored.
	configFile := paths.ConfigFile(tmpDir, "board")
	content := `path: ../custom_board
home: Home.md
sidebar: _Sidebar.md
proposal_prefix: proposal-
`
	if err := os.WriteFile(configFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	cfg, err := boardengine.LoadConfig(tmpDir, "board")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// yaml:"-" means the path: key in the file is never mapped to Config.Path;
	// no relative-path resolution is performed.
	if cfg.Path != "" {
		t.Errorf("expected Path to be empty (yaml:\"-\" ignores config key); got %q", cfg.Path)
	}
}

// TestLoadConfig_EnvResolution verifies that a path: key using ${env:...} syntax
// in the config file is ignored by LoadConfig because Config.Path has yaml:"-".
// The env-override mechanism for the board data dir has been removed; the data
// dir is now geometry owned by paths.BoardDir and is not env-overridable.
func TestLoadConfig_EnvResolution(t *testing.T) {
	tmpDir := t.TempDir()
	absBoard := t.TempDir()
	t.Setenv("TEST_BOARD_PATH", absBoard)

	// Create _lyx/config/ directories
	lyxDir := filepath.Join(tmpDir, paths.LyxDirName)
	if err := os.Mkdir(lyxDir, 0755); err != nil {
		t.Fatalf("failed to create _lyx: %v", err)
	}
	configDir := paths.ConfigDir(tmpDir)
	if err := os.Mkdir(configDir, 0755); err != nil {
		t.Fatalf("failed to create _lyx/config: %v", err)
	}

	// Write config with an env-variable path: key that should be ignored.
	configFile := paths.ConfigFile(tmpDir, "board")
	content := `path: ${env:TEST_BOARD_PATH}
home: Home.md
sidebar: _Sidebar.md
proposal_prefix: proposal-
`
	if err := os.WriteFile(configFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	cfg, err := boardengine.LoadConfig(tmpDir, "board")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// yaml:"-" means Config.Path is never populated from the config file, even
	// after env-variable resolution expands the value.
	if cfg.Path != "" {
		t.Errorf("expected Path to be empty (yaml:\"-\" ignores config key); got %q", cfg.Path)
	}
}

// TestLoadConfig_NotInitialized tests that missing _lyx/ returns the
// board-specific not-initialized error.
func TestLoadConfig_NotInitialized(t *testing.T) {
	tmpDir := t.TempDir()
	// Do NOT create _lyx/

	cfg, err := boardengine.LoadConfig(tmpDir, "board")
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
	cfg := boardengine.Config{
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
