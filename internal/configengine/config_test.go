// config_test.go — unit tests for the strict config loader.
//
// Tests cover: the strict Load contract using yamlengine + envsource,
// missing-key detection, absent-file errors, env variable resolution via templates,
// nested-key handling, and the not-initialized error path.

package configengine_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/hubgeometry"
	"gopkg.in/yaml.v3"
)

// TestLoad_HappyPath tests that all template keys present in file round-trip correctly.
func TestLoad_HappyPath(t *testing.T) {
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

	// Simple template with two keys
	template := []byte("path: _board\nhome: Home.md\n")

	// Write config file matching template
	yamlFile := hubgeometry.ConfigFile(tmpDir, "board")
	if err := os.WriteFile(yamlFile, []byte("path: custom_path\nhome: Index.md\n"), 0644); err != nil {
		t.Fatalf("failed to write board.yaml: %v", err)
	}

	resolved, err := configengine.Load(tmpDir, "board", template)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Unmarshal resolved bytes and verify values
	var result map[string]string
	if err := yaml.Unmarshal(resolved, &result); err != nil {
		t.Fatalf("failed to unmarshal resolved config: %v", err)
	}

	if result["path"] != "custom_path" {
		t.Errorf("expected path %q, got %q", "custom_path", result["path"])
	}
	if result["home"] != "Index.md" {
		t.Errorf("expected home %q, got %q", "Index.md", result["home"])
	}
}

// TestLoad_MissingKey tests that missing template key in file returns an error.
func TestLoad_MissingKey(t *testing.T) {
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

	// Template with two keys
	template := []byte("path: _board\nhome: Home.md\n")

	// Config file missing "home" key
	yamlFile := hubgeometry.ConfigFile(tmpDir, "board")
	if err := os.WriteFile(yamlFile, []byte("path: custom_path\n"), 0644); err != nil {
		t.Fatalf("failed to write board.yaml: %v", err)
	}

	_, err := configengine.Load(tmpDir, "board", template)
	if err == nil {
		t.Fatalf("expected error for missing key, got nil")
	}

	errMsg := err.Error()
	if !strings.Contains(errMsg, "missing keys") {
		t.Errorf("expected error containing 'missing keys', got: %v", err)
	}
	if !strings.Contains(errMsg, "home") {
		t.Errorf("expected error containing 'home', got: %v", err)
	}
	if !strings.Contains(errMsg, "lyx config reconcile") {
		t.Errorf("expected error containing 'lyx config reconcile', got: %v", err)
	}
}

// TestLoad_AbsentFile tests that missing config file returns an error.
func TestLoad_AbsentFile(t *testing.T) {
	tmpDir := t.TempDir()

	// Create _lyx/config/ directories but NOT board.yaml
	lyxDir := filepath.Join(tmpDir, hubgeometry.LyxDirName)
	if err := os.Mkdir(lyxDir, 0755); err != nil {
		t.Fatalf("failed to create _lyx: %v", err)
	}
	configDir := hubgeometry.ConfigDir(tmpDir)
	if err := os.Mkdir(configDir, 0755); err != nil {
		t.Fatalf("failed to create _lyx/config: %v", err)
	}

	template := []byte("path: _board\n")

	_, err := configengine.Load(tmpDir, "board", template)
	if err == nil {
		t.Fatalf("expected error for absent file, got nil")
	}

	errMsg := err.Error()
	if !strings.Contains(errMsg, "not found") {
		t.Errorf("expected error containing 'not found', got: %v", err)
	}
	if !strings.Contains(errMsg, "lyx config reconcile") {
		t.Errorf("expected error containing 'lyx config reconcile', got: %v", err)
	}
}

// TestLoad_EnvResolution tests that ${env:NAME} values are resolved correctly.
func TestLoad_EnvResolution(t *testing.T) {
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

	// Set environment variable
	t.Setenv("TEST_CONFIG_VAR", "resolved_value")

	// Template with an env marker
	template := []byte("path: ${env:TEST_CONFIG_VAR}\n")

	// Config file with the same env marker
	yamlFile := hubgeometry.ConfigFile(tmpDir, "board")
	if err := os.WriteFile(yamlFile, []byte("path: ${env:TEST_CONFIG_VAR}\n"), 0644); err != nil {
		t.Fatalf("failed to write board.yaml: %v", err)
	}

	resolved, err := configengine.Load(tmpDir, "board", template)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Unmarshal and verify the env var was expanded
	var result map[string]string
	if err := yaml.Unmarshal(resolved, &result); err != nil {
		t.Fatalf("failed to unmarshal resolved config: %v", err)
	}

	if result["path"] != "resolved_value" {
		t.Errorf("expected path %q (from env), got %q", "resolved_value", result["path"])
	}
}

// TestLoad_OptionalEnv tests that ${env:NAME:-default} uses default when var is unset.
func TestLoad_OptionalEnv(t *testing.T) {
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

	// Do NOT set TEST_OPTIONAL_VAR; the default should be used

	// Template with optional env
	template := []byte("path: ${env:TEST_OPTIONAL_VAR:-default_path}\n")

	// Config file with optional env
	yamlFile := hubgeometry.ConfigFile(tmpDir, "board")
	if err := os.WriteFile(yamlFile, []byte("path: ${env:TEST_OPTIONAL_VAR:-default_path}\n"), 0644); err != nil {
		t.Fatalf("failed to write board.yaml: %v", err)
	}

	resolved, err := configengine.Load(tmpDir, "board", template)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Unmarshal and verify the default was used
	var result map[string]string
	if err := yaml.Unmarshal(resolved, &result); err != nil {
		t.Fatalf("failed to unmarshal resolved config: %v", err)
	}

	if result["path"] != "default_path" {
		t.Errorf("expected path %q (from default), got %q", "default_path", result["path"])
	}
}

// TestLoad_ExtraKeyTolerated tests that extra keys in the file are tolerated (no error).
func TestLoad_ExtraKeyTolerated(t *testing.T) {
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

	// Template with one key
	template := []byte("path: _board\n")

	// Config file with extra key
	yamlFile := hubgeometry.ConfigFile(tmpDir, "board")
	if err := os.WriteFile(yamlFile, []byte("path: custom_path\nextra_key: extra_value\n"), 0644); err != nil {
		t.Fatalf("failed to write board.yaml: %v", err)
	}

	resolved, err := configengine.Load(tmpDir, "board", template)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should succeed without error
	if resolved == nil {
		t.Fatalf("expected resolved bytes, got nil")
	}
}

// TestLoad_NotInitialized tests that _lyx/ absent returns the not-initialized error.
func TestLoad_NotInitialized(t *testing.T) {
	tmpDir := t.TempDir()
	// Do NOT create _lyx/

	template := []byte("path: _board\n")

	_, err := configengine.Load(tmpDir, "board", template)
	if err == nil {
		t.Fatalf("expected error for not initialized, got nil")
	}

	errMsg := err.Error()
	if !strings.Contains(errMsg, "not initialized") {
		t.Errorf("expected error containing 'not initialized', got: %v", err)
	}
}

// TestLoad_NestedKeyTemplate tests that nested keys round-trip correctly.
func TestLoad_NestedKeyTemplate(t *testing.T) {
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

	// Template with nested structure
	template := []byte("server:\n  host: localhost\n  port: '8080'\n")

	// Config file with nested values
	yamlFile := hubgeometry.ConfigFile(tmpDir, "test")
	if err := os.WriteFile(yamlFile, []byte("server:\n  host: example.com\n  port: '9090'\n"), 0644); err != nil {
		t.Fatalf("failed to write test.yaml: %v", err)
	}

	resolved, err := configengine.Load(tmpDir, "test", template)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Unmarshal nested config
	var result map[string]interface{}
	if err := yaml.Unmarshal(resolved, &result); err != nil {
		t.Fatalf("failed to unmarshal resolved config: %v", err)
	}

	server, ok := result["server"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected server key to be a map, got %T", result["server"])
	}

	if server["host"] != "example.com" {
		t.Errorf("expected server.host %q, got %q", "example.com", server["host"])
	}
	if server["port"] != "9090" {
		t.Errorf("expected server.port %q, got %q", "9090", server["port"])
	}
}

// TestFindBaseDir_Present tests that FindBaseDir returns the cwd when _lyx/ exists.
func TestFindBaseDir_Present(t *testing.T) {
	tmpDir := t.TempDir()

	// Create _lyx/ directory
	lyxDir := filepath.Join(tmpDir, hubgeometry.LyxDirName)
	if err := os.Mkdir(lyxDir, 0755); err != nil {
		t.Fatalf("failed to create _lyx: %v", err)
	}

	result, err := configengine.FindBaseDir(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result != tmpDir {
		t.Errorf("expected %q, got %q", tmpDir, result)
	}
}

// TestFindBaseDir_Absent tests that FindBaseDir returns an error when _lyx/ does not exist.
func TestFindBaseDir_Absent(t *testing.T) {
	tmpDir := t.TempDir()

	result, err := configengine.FindBaseDir(tmpDir)
	if err == nil {
		t.Fatalf("expected error, got nil; result: %v", result)
	}

	if result != "" {
		t.Errorf("expected empty string, got %q", result)
	}

	if !strings.Contains(err.Error(), "not initialized") {
		t.Errorf("expected error containing 'not initialized', got: %v", err)
	}
}
