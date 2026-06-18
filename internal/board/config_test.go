// config_test.go — unit tests for the Config system (config.go).
//
// Covers: defaults, error on uninitialized, layered merging, environment variable
// expansion, path resolution (relative vs absolute), and malformed YAML.

package board_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/board"
)

// TestDefaultsReturned tests that defaults are returned when _lyx/ exists
// but board.yaml is absent.
func TestDefaultsReturned(t *testing.T) {
	baseDir := t.TempDir()

	// Create _lyx/ directory (empty, no board.yaml)
	lyxDir := filepath.Join(baseDir, "_lyx")
	if err := os.Mkdir(lyxDir, 0755); err != nil {
		t.Fatalf("failed to create _lyx directory: %v", err)
	}

	cfg, err := board.LoadConfig(baseDir, "board")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Note: Path is resolved relative to baseDir, so check for expected suffix
	expectedPathSuffix := "_board"
	if !strings.Contains(cfg.Path, expectedPathSuffix) {
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
// is returned when _lyx/ directory does not exist.
func TestErrorNotInitialized(t *testing.T) {
	baseDir := t.TempDir()

	// Do not create _lyx/ directory

	cfg, err := board.LoadConfig(baseDir, "board")
	if err == nil {
		t.Fatalf("expected error, got nil; config: %+v", cfg)
	}

	errMsg := err.Error()
	if !strings.Contains(errMsg, "not initialized") {
		t.Errorf("expected error message to contain 'not initialized', got: %s", errMsg)
	}
}

// TestRelativePathResolution tests that a relative path is resolved against baseDir.
func TestRelativePathResolution(t *testing.T) {
	baseDir := t.TempDir()

	// Create _lyx/ and _lyx/config/ directories
	lyxDir := filepath.Join(baseDir, "_lyx")
	if err := os.Mkdir(lyxDir, 0755); err != nil {
		t.Fatalf("failed to create _lyx: %v", err)
	}
	configDir := filepath.Join(lyxDir, "config")
	if err := os.Mkdir(configDir, 0755); err != nil {
		t.Fatalf("failed to create _lyx/config: %v", err)
	}

	// Write _lyx/config/board.yaml with relative path
	lyxFile := filepath.Join(configDir, "board.yaml")
	if err := os.WriteFile(lyxFile, []byte("path: _custom_board\n"), 0644); err != nil {
		t.Fatalf("failed to write _lyx/config/board.yaml: %v", err)
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

	// Create _lyx/ and _lyx/config/ directories
	lyxDir := filepath.Join(baseDir, "_lyx")
	if err := os.Mkdir(lyxDir, 0755); err != nil {
		t.Fatalf("failed to create _lyx: %v", err)
	}
	configDir := filepath.Join(lyxDir, "config")
	if err := os.Mkdir(configDir, 0755); err != nil {
		t.Fatalf("failed to create _lyx/config: %v", err)
	}

	// Create an absolute path by using TempDir
	absBoard := t.TempDir()

	// Write _lyx/config/board.yaml with absolute path
	lyxFile := filepath.Join(configDir, "board.yaml")
	if err := os.WriteFile(lyxFile, []byte("path: "+absBoard+"\n"), 0644); err != nil {
		t.Fatalf("failed to write _lyx/config/board.yaml: %v", err)
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

	// Create _lyx/ and _lyx/config/ directories
	lyxDir := filepath.Join(baseDir, "_lyx")
	if err := os.Mkdir(lyxDir, 0755); err != nil {
		t.Fatalf("failed to create _lyx: %v", err)
	}
	configDir := filepath.Join(lyxDir, "config")
	if err := os.Mkdir(configDir, 0755); err != nil {
		t.Fatalf("failed to create _lyx/config: %v", err)
	}

	// Write malformed YAML
	lyxFile := filepath.Join(configDir, "board.yaml")
	if err := os.WriteFile(lyxFile, []byte("path: value\n  invalid indentation: [ unclosed"), 0644); err != nil {
		t.Fatalf("failed to write _lyx/config/board.yaml: %v", err)
	}

	cfg, err := board.LoadConfig(baseDir, "board")
	if err == nil {
		t.Fatalf("expected error for malformed YAML, got nil; config: %+v", cfg)
	}

	errMsg := err.Error()
	if !strings.Contains(errMsg, "yaml:") {
		t.Errorf("expected error message about YAML parsing (starting with 'yaml:'), got: %s", errMsg)
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

// TestLoadConfig_FallbackPathResolution tests that optional env var syntax with fallback works.
func TestLoadConfig_FallbackPathResolution(t *testing.T) {
	tmpDir := t.TempDir()

	// Create _lyx/ and _lyx/config/ directories
	lyxDir := filepath.Join(tmpDir, "_lyx")
	if err := os.Mkdir(lyxDir, 0755); err != nil {
		t.Fatalf("failed to create _lyx: %v", err)
	}
	configDir := filepath.Join(lyxDir, "config")
	if err := os.Mkdir(configDir, 0755); err != nil {
		t.Fatalf("failed to create _lyx/config: %v", err)
	}

	// Write _lyx/config/board.yaml with fallback syntax for unset var
	boardYamlPath := filepath.Join(configDir, "board.yaml")
	if err := os.WriteFile(boardYamlPath, []byte("path: $env:NONEXISTENT_LYX_TEST_VAR_XYZ ? ../_board\n"), 0644); err != nil {
		t.Fatalf("failed to write _lyx/config/board.yaml: %v", err)
	}

	cfg, err := board.LoadConfig(tmpDir, "board")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// The fallback path "../_board" should be resolved relative to tmpDir
	expected := filepath.Join(tmpDir, "../_board")
	// filepath.Join will clean it to a sibling directory
	expected = filepath.Clean(expected)
	if cfg.Path != expected {
		t.Errorf("expected path %q, got %q", expected, cfg.Path)
	}
}
