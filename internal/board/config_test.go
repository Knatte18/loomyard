// config_test.go — unit tests for the Config system (config.go).
//
// Covers: defaults, error on uninitialized, layered merging, environment variable
// expansion, path resolution (relative vs absolute), and malformed YAML.

package board_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/mhgo/internal/board"
)

// TestDefaultsReturned tests that defaults are returned when _mhgo/ exists
// but board.yaml is absent.
func TestDefaultsReturned(t *testing.T) {
	baseDir := t.TempDir()

	// Create _mhgo/ directory (empty, no board.yaml)
	mhgoDir := filepath.Join(baseDir, "_mhgo")
	if err := os.Mkdir(mhgoDir, 0755); err != nil {
		t.Fatalf("failed to create _mhgo directory: %v", err)
	}

	cfg, err := board.LoadConfig(baseDir, "board")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Note: Path is resolved relative to baseDir, so check for expected suffix
	expectedPathSuffix := "_board"
	if !stringContains(cfg.Path, expectedPathSuffix) {
		t.Errorf("expected Path to contain %q, got %q", expectedPathSuffix, cfg.Path)
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

// TestErrorNotInitialized tests that an error containing "not initialized"
// is returned when _mhgo/ directory does not exist.
func TestErrorNotInitialized(t *testing.T) {
	baseDir := t.TempDir()

	// Do not create _mhgo/ directory

	cfg, err := board.LoadConfig(baseDir, "board")
	if err == nil {
		t.Fatalf("expected error, got nil; config: %+v", cfg)
	}

	errMsg := err.Error()
	if !stringContains(errMsg, "not initialized") {
		t.Errorf("expected error message to contain 'not initialized', got: %s", errMsg)
	}
}

// TestDeepMergeMultipleLayers tests that keys from lower layers are overridden
// by higher layers: default < _mhgo/board.yaml < .mhgo/board.yaml.
func TestDeepMergeMultipleLayers(t *testing.T) {
	baseDir := t.TempDir()

	// Create directory structure
	mhgoDir := filepath.Join(baseDir, "_mhgo")
	dotMhgoDir := filepath.Join(baseDir, ".mhgo")
	if err := os.Mkdir(mhgoDir, 0755); err != nil {
		t.Fatalf("failed to create _mhgo: %v", err)
	}
	if err := os.Mkdir(dotMhgoDir, 0755); err != nil {
		t.Fatalf("failed to create .mhgo: %v", err)
	}

	// Write _mhgo/board.yaml: override path and home
	mhgoFile := filepath.Join(mhgoDir, "board.yaml")
	if err := os.WriteFile(mhgoFile, []byte("path: ../_custom\nhome: Custom.md\n"), 0644); err != nil {
		t.Fatalf("failed to write _mhgo/board.yaml: %v", err)
	}

	// Write .mhgo/board.yaml: override only sidebar
	dotMhgoFile := filepath.Join(dotMhgoDir, "board.yaml")
	if err := os.WriteFile(dotMhgoFile, []byte("sidebar: _Custom.md\n"), 0644); err != nil {
		t.Fatalf("failed to write .mhgo/board.yaml: %v", err)
	}

	cfg, err := board.LoadConfig(baseDir, "board")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// path from _mhgo should be overridden
	if !stringContains(cfg.Path, "_custom") {
		t.Errorf("expected path to contain '_custom', got %q", cfg.Path)
	}

	// home from _mhgo should persist (not in .mhgo)
	if cfg.Home != "Custom.md" {
		t.Errorf("expected Home 'Custom.md', got %q", cfg.Home)
	}

	// sidebar from .mhgo should override
	if cfg.Sidebar != "_Custom.md" {
		t.Errorf("expected Sidebar '_Custom.md', got %q", cfg.Sidebar)
	}

	// proposal_prefix from default should persist (not in any layer)
	if cfg.ProposalPrefix != board.DefaultConfig().ProposalPrefix {
		t.Errorf("expected ProposalPrefix %q, got %q", board.DefaultConfig().ProposalPrefix, cfg.ProposalPrefix)
	}
}

// TestEnvExpansionWholeValue tests that $env:NAME is expanded when it is
// the entire value.
func TestEnvExpansionWholeValue(t *testing.T) {
	baseDir := t.TempDir()

	// Create _mhgo/ directory
	mhgoDir := filepath.Join(baseDir, "_mhgo")
	if err := os.Mkdir(mhgoDir, 0755); err != nil {
		t.Fatalf("failed to create _mhgo: %v", err)
	}

	// Set an environment variable (use an absolute path that will work on Windows and Unix)
	customBoardPath := filepath.Join(baseDir, "custom_board")
	t.Setenv("MHGO_BOARD_PATH", customBoardPath)

	// Write _mhgo/board.yaml with env variable reference
	mhgoFile := filepath.Join(mhgoDir, "board.yaml")
	if err := os.WriteFile(mhgoFile, []byte("path: $env:MHGO_BOARD_PATH\n"), 0644); err != nil {
		t.Fatalf("failed to write _mhgo/board.yaml: %v", err)
	}

	cfg, err := board.LoadConfig(baseDir, "board")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Path != customBoardPath {
		t.Errorf("expected path %q, got %q", customBoardPath, cfg.Path)
	}
}

// TestEnvExpansionEmbedded tests that $env:NAME is expanded within a path
// like $env:MHGO_BASE/boards/main.
func TestEnvExpansionEmbedded(t *testing.T) {
	baseDir := t.TempDir()

	// Create _mhgo/ directory
	mhgoDir := filepath.Join(baseDir, "_mhgo")
	if err := os.Mkdir(mhgoDir, 0755); err != nil {
		t.Fatalf("failed to create _mhgo: %v", err)
	}

	// Set environment variables
	basePath := filepath.Join(baseDir, "base")
	t.Setenv("MHGO_BASE", basePath)

	// Write _mhgo/board.yaml with embedded env variable
	mhgoFile := filepath.Join(mhgoDir, "board.yaml")
	// Use forward slashes in the YAML since they work cross-platform
	if err := os.WriteFile(mhgoFile, []byte("path: $env:MHGO_BASE/boards/main\n"), 0644); err != nil {
		t.Fatalf("failed to write _mhgo/board.yaml: %v", err)
	}

	cfg, err := board.LoadConfig(baseDir, "board")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Check that the path contains the base and the sub-path elements
	// (Don't use filepath.Join here since the YAML might have mixed separators)
	if !stringContains(cfg.Path, "base") || !stringContains(cfg.Path, "boards") || !stringContains(cfg.Path, "main") {
		t.Errorf("expected path to contain base, boards, and main; got %q", cfg.Path)
	}
}

// TestEnvExpansionUnsetError tests that an error is returned when a referenced
// environment variable is not set.
func TestEnvExpansionUnsetError(t *testing.T) {
	baseDir := t.TempDir()

	// Create _mhgo/ directory
	mhgoDir := filepath.Join(baseDir, "_mhgo")
	if err := os.Mkdir(mhgoDir, 0755); err != nil {
		t.Fatalf("failed to create _mhgo: %v", err)
	}

	// Ensure the variable is NOT set
	t.Setenv("NONEXISTENT_VAR", "")
	os.Unsetenv("NONEXISTENT_VAR")

	// Write _mhgo/board.yaml with unset env variable
	mhgoFile := filepath.Join(mhgoDir, "board.yaml")
	if err := os.WriteFile(mhgoFile, []byte("path: $env:NONEXISTENT_VAR\n"), 0644); err != nil {
		t.Fatalf("failed to write _mhgo/board.yaml: %v", err)
	}

	cfg, err := board.LoadConfig(baseDir, "board")
	if err == nil {
		t.Fatalf("expected error for unset environment variable, got nil; config: %+v", cfg)
	}

	errMsg := err.Error()
	if !stringContains(errMsg, "referenced env var") || !stringContains(errMsg, "NONEXISTENT_VAR") {
		t.Errorf("expected error message to reference unset variable, got: %s", errMsg)
	}
}

// TestRelativePathResolution tests that a relative path is resolved against baseDir.
func TestRelativePathResolution(t *testing.T) {
	baseDir := t.TempDir()

	// Create _mhgo/ directory
	mhgoDir := filepath.Join(baseDir, "_mhgo")
	if err := os.Mkdir(mhgoDir, 0755); err != nil {
		t.Fatalf("failed to create _mhgo: %v", err)
	}

	// Write _mhgo/board.yaml with relative path
	mhgoFile := filepath.Join(mhgoDir, "board.yaml")
	if err := os.WriteFile(mhgoFile, []byte("path: _custom_board\n"), 0644); err != nil {
		t.Fatalf("failed to write _mhgo/board.yaml: %v", err)
	}

	cfg, err := board.LoadConfig(baseDir, "board")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := filepath.Join(baseDir, "_custom_board")
	if cfg.Path != expected {
		t.Errorf("expected path %q, got %q", expected, cfg.Path)
	}
}

// TestAbsolutePathPassthrough tests that an absolute path is used as-is.
func TestAbsolutePathPassthrough(t *testing.T) {
	baseDir := t.TempDir()

	// Create _mhgo/ directory
	mhgoDir := filepath.Join(baseDir, "_mhgo")
	if err := os.Mkdir(mhgoDir, 0755); err != nil {
		t.Fatalf("failed to create _mhgo: %v", err)
	}

	// Create an absolute path by using TempDir
	absBoard := t.TempDir()

	// Write _mhgo/board.yaml with absolute path
	mhgoFile := filepath.Join(mhgoDir, "board.yaml")
	if err := os.WriteFile(mhgoFile, []byte("path: "+absBoard+"\n"), 0644); err != nil {
		t.Fatalf("failed to write _mhgo/board.yaml: %v", err)
	}

	cfg, err := board.LoadConfig(baseDir, "board")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Path != absBoard {
		t.Errorf("expected path %q, got %q", absBoard, cfg.Path)
	}
}

// TestMalformedYAMLError tests that malformed YAML surfaces an error.
func TestMalformedYAMLError(t *testing.T) {
	baseDir := t.TempDir()

	// Create _mhgo/ directory
	mhgoDir := filepath.Join(baseDir, "_mhgo")
	if err := os.Mkdir(mhgoDir, 0755); err != nil {
		t.Fatalf("failed to create _mhgo: %v", err)
	}

	// Write malformed YAML
	mhgoFile := filepath.Join(mhgoDir, "board.yaml")
	if err := os.WriteFile(mhgoFile, []byte("path: value\n  invalid indentation: [ unclosed"), 0644); err != nil {
		t.Fatalf("failed to write _mhgo/board.yaml: %v", err)
	}

	cfg, err := board.LoadConfig(baseDir, "board")
	if err == nil {
		t.Fatalf("expected error for malformed YAML, got nil; config: %+v", cfg)
	}

	errMsg := err.Error()
	if !stringContains(errMsg, "parsing YAML") && !stringContains(errMsg, "error") {
		t.Errorf("expected error message about YAML parsing, got: %s", errMsg)
	}
}

// TestOutputsFromConfig tests that (Config).Outputs() returns an Outputs struct
// with the correct values.
func TestOutputsFromConfig(t *testing.T) {
	cfg := board.Config{
		Path:           "/some/path",
		Home:           "Home.md",
		Sidebar:        "_Sidebar.md",
		ProposalPrefix: "proposal-",
	}

	out := cfg.Outputs()

	if out.Home != "Home.md" {
		t.Errorf("expected Home 'Home.md', got %q", out.Home)
	}
	if out.Sidebar != "_Sidebar.md" {
		t.Errorf("expected Sidebar '_Sidebar.md', got %q", out.Sidebar)
	}
	if out.ProposalPrefix != "proposal-" {
		t.Errorf("expected ProposalPrefix 'proposal-', got %q", out.ProposalPrefix)
	}
}

// TestDefaultOutputs tests that DefaultOutputs() matches DefaultConfig().Outputs().
func TestDefaultOutputs(t *testing.T) {
	defaultOut := board.DefaultOutputs()
	configOut := board.DefaultConfig().Outputs()

	if defaultOut.Home != configOut.Home {
		t.Errorf("DefaultOutputs Home mismatch: %q vs %q", defaultOut.Home, configOut.Home)
	}
	if defaultOut.Sidebar != configOut.Sidebar {
		t.Errorf("DefaultOutputs Sidebar mismatch: %q vs %q", defaultOut.Sidebar, configOut.Sidebar)
	}
	if defaultOut.ProposalPrefix != configOut.ProposalPrefix {
		t.Errorf("DefaultOutputs ProposalPrefix mismatch: %q vs %q", defaultOut.ProposalPrefix, configOut.ProposalPrefix)
	}
}
