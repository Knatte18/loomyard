// config_test.go — unit tests for the generic config loader (config.go).
//
// Tests cover: uninitialized dir, defaults passthrough, YAML override, .mhgo/ ignored,
// required env (set and unset), optional env (set, unset, with prefix), .env loading,
// and literal ? character.

package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/mhgo/internal/config"
)

// TestLoad_UninitializedDir tests that an error is returned when _mhgo/
// directory does not exist.
func TestLoad_UninitializedDir(t *testing.T) {
	tmpDir := t.TempDir()

	result, err := config.Load(tmpDir, "board", map[string]string{"path": "_board"})
	if err == nil {
		t.Fatalf("expected error, got nil; result: %v", result)
	}

	if !stringContains(err.Error(), "not initialized") {
		t.Errorf("expected error containing 'not initialized', got: %v", err)
	}
}

// TestLoad_Defaults tests that defaults are returned when _mhgo/ exists
// but the YAML file is absent.
func TestLoad_Defaults(t *testing.T) {
	tmpDir := t.TempDir()

	// Create _mhgo/ directory
	mhgoDir := filepath.Join(tmpDir, "_mhgo")
	if err := os.Mkdir(mhgoDir, 0755); err != nil {
		t.Fatalf("failed to create _mhgo: %v", err)
	}

	result, err := config.Load(tmpDir, "board", map[string]string{"path": "_board"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result["path"] != "_board" {
		t.Errorf("expected path %q, got %q", "_board", result["path"])
	}
}

// TestLoad_YAMLOverride tests that YAML values override defaults.
func TestLoad_YAMLOverride(t *testing.T) {
	tmpDir := t.TempDir()

	// Create _mhgo/ directory
	mhgoDir := filepath.Join(tmpDir, "_mhgo")
	if err := os.Mkdir(mhgoDir, 0755); err != nil {
		t.Fatalf("failed to create _mhgo: %v", err)
	}

	// Write board.yaml
	yamlFile := filepath.Join(mhgoDir, "board.yaml")
	if err := os.WriteFile(yamlFile, []byte("path: custom_path\n"), 0644); err != nil {
		t.Fatalf("failed to write board.yaml: %v", err)
	}

	result, err := config.Load(tmpDir, "board",
		map[string]string{"path": "default_path", "home": "Home.md"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result["path"] != "custom_path" {
		t.Errorf("expected path %q, got %q", "custom_path", result["path"])
	}
	if result["home"] != "Home.md" {
		t.Errorf("expected home %q, got %q", "Home.md", result["home"])
	}
}

// TestLoad_DotMhgoIgnored tests that .mhgo/ files are ignored (only _mhgo/ is used).
func TestLoad_DotMhgoIgnored(t *testing.T) {
	tmpDir := t.TempDir()

	// Create _mhgo/ directory with board.yaml
	mhgoDir := filepath.Join(tmpDir, "_mhgo")
	if err := os.Mkdir(mhgoDir, 0755); err != nil {
		t.Fatalf("failed to create _mhgo: %v", err)
	}

	yamlFile := filepath.Join(mhgoDir, "board.yaml")
	if err := os.WriteFile(yamlFile, []byte("path: correct\n"), 0644); err != nil {
		t.Fatalf("failed to write _mhgo/board.yaml: %v", err)
	}

	// Create .mhgo/ directory with board.yaml (should be ignored)
	dotMhgoDir := filepath.Join(tmpDir, ".mhgo")
	if err := os.Mkdir(dotMhgoDir, 0755); err != nil {
		t.Fatalf("failed to create .mhgo: %v", err)
	}

	dotYamlFile := filepath.Join(dotMhgoDir, "board.yaml")
	if err := os.WriteFile(dotYamlFile, []byte("path: wrong\n"), 0644); err != nil {
		t.Fatalf("failed to write .mhgo/board.yaml: %v", err)
	}

	result, err := config.Load(tmpDir, "board", map[string]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result["path"] != "correct" {
		t.Errorf("expected path %q, got %q", "correct", result["path"])
	}
}

// TestLoad_EnvRequired_Set tests that $env:NAME is expanded when set.
func TestLoad_EnvRequired_Set(t *testing.T) {
	tmpDir := t.TempDir()

	// Create _mhgo/ directory
	mhgoDir := filepath.Join(tmpDir, "_mhgo")
	if err := os.Mkdir(mhgoDir, 0755); err != nil {
		t.Fatalf("failed to create _mhgo: %v", err)
	}

	// Set environment variable
	t.Setenv("TEST_EXTRACT_REQ_VAR", "expanded")

	// Write board.yaml with env variable
	yamlFile := filepath.Join(mhgoDir, "board.yaml")
	if err := os.WriteFile(yamlFile, []byte("path: $env:TEST_EXTRACT_REQ_VAR\n"), 0644); err != nil {
		t.Fatalf("failed to write board.yaml: %v", err)
	}

	result, err := config.Load(tmpDir, "board", map[string]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result["path"] != "expanded" {
		t.Errorf("expected path %q, got %q", "expanded", result["path"])
	}
}

// TestLoad_EnvRequired_Unset tests that an error is returned when a required
// env variable is unset.
func TestLoad_EnvRequired_Unset(t *testing.T) {
	tmpDir := t.TempDir()

	// Create _mhgo/ directory
	mhgoDir := filepath.Join(tmpDir, "_mhgo")
	if err := os.Mkdir(mhgoDir, 0755); err != nil {
		t.Fatalf("failed to create _mhgo: %v", err)
	}

	// Ensure variable is unset
	t.Setenv("TEST_EXTRACT_MISSING_VAR_XYZ123", "")
	os.Unsetenv("TEST_EXTRACT_MISSING_VAR_XYZ123")

	// Write board.yaml with unset env variable
	yamlFile := filepath.Join(mhgoDir, "board.yaml")
	if err := os.WriteFile(yamlFile, []byte("path: $env:TEST_EXTRACT_MISSING_VAR_XYZ123\n"), 0644); err != nil {
		t.Fatalf("failed to write board.yaml: %v", err)
	}

	result, err := config.Load(tmpDir, "board", map[string]string{})
	if err == nil {
		t.Fatalf("expected error, got nil; result: %v", result)
	}

	errMsg := err.Error()
	if !stringContains(errMsg, "TEST_EXTRACT_MISSING_VAR_XYZ123") {
		t.Errorf("expected error containing variable name, got: %v", err)
	}
}

// TestLoad_EnvOptional_Set tests that $env:NAME ? fallback uses the env value when set.
func TestLoad_EnvOptional_Set(t *testing.T) {
	tmpDir := t.TempDir()

	// Create _mhgo/ directory
	mhgoDir := filepath.Join(tmpDir, "_mhgo")
	if err := os.Mkdir(mhgoDir, 0755); err != nil {
		t.Fatalf("failed to create _mhgo: %v", err)
	}

	// Set environment variable
	t.Setenv("TEST_EXTRACT_OPT_VAR", "set_value")

	// Write board.yaml with optional env variable
	yamlFile := filepath.Join(mhgoDir, "board.yaml")
	if err := os.WriteFile(yamlFile, []byte("path: $env:TEST_EXTRACT_OPT_VAR ? fallback\n"), 0644); err != nil {
		t.Fatalf("failed to write board.yaml: %v", err)
	}

	result, err := config.Load(tmpDir, "board", map[string]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result["path"] != "set_value" {
		t.Errorf("expected path %q, got %q", "set_value", result["path"])
	}
}

// TestLoad_EnvOptional_Unset tests that $env:NAME ? fallback uses the fallback when unset.
func TestLoad_EnvOptional_Unset(t *testing.T) {
	tmpDir := t.TempDir()

	// Create _mhgo/ directory
	mhgoDir := filepath.Join(tmpDir, "_mhgo")
	if err := os.Mkdir(mhgoDir, 0755); err != nil {
		t.Fatalf("failed to create _mhgo: %v", err)
	}

	// Ensure variable is unset
	t.Setenv("TEST_EXTRACT_OPT_ABSENT", "")
	os.Unsetenv("TEST_EXTRACT_OPT_ABSENT")

	// Write board.yaml with optional env variable
	yamlFile := filepath.Join(mhgoDir, "board.yaml")
	if err := os.WriteFile(yamlFile, []byte("path: $env:TEST_EXTRACT_OPT_ABSENT ? my_fallback\n"), 0644); err != nil {
		t.Fatalf("failed to write board.yaml: %v", err)
	}

	result, err := config.Load(tmpDir, "board", map[string]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result["path"] != "my_fallback" {
		t.Errorf("expected path %q, got %q", "my_fallback", result["path"])
	}
}

// TestLoad_EnvOptional_WithPrefix tests that optional env variables work with prefixes.
func TestLoad_EnvOptional_WithPrefix(t *testing.T) {
	tmpDir := t.TempDir()

	// Create _mhgo/ directory
	mhgoDir := filepath.Join(tmpDir, "_mhgo")
	if err := os.Mkdir(mhgoDir, 0755); err != nil {
		t.Fatalf("failed to create _mhgo: %v", err)
	}

	// Ensure variable is unset
	t.Setenv("TEST_EXTRACT_PREFIX_VAR", "")
	os.Unsetenv("TEST_EXTRACT_PREFIX_VAR")

	// Write board.yaml with optional env variable with prefix
	yamlFile := filepath.Join(mhgoDir, "board.yaml")
	if err := os.WriteFile(yamlFile, []byte("path: prefix/$env:TEST_EXTRACT_PREFIX_VAR ? default_name\n"), 0644); err != nil {
		t.Fatalf("failed to write board.yaml: %v", err)
	}

	result, err := config.Load(tmpDir, "board", map[string]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result["path"] != "prefix/default_name" {
		t.Errorf("expected path %q, got %q", "prefix/default_name", result["path"])
	}
}

// TestLoad_DotEnv_FillsUnset tests that .env file is used when OS env is not set.
func TestLoad_DotEnv_FillsUnset(t *testing.T) {
	tmpDir := t.TempDir()

	// Create _mhgo/ directory
	mhgoDir := filepath.Join(tmpDir, "_mhgo")
	if err := os.Mkdir(mhgoDir, 0755); err != nil {
		t.Fatalf("failed to create _mhgo: %v", err)
	}

	// Create .env file
	envFile := filepath.Join(tmpDir, ".env")
	if err := os.WriteFile(envFile, []byte("TEST_EXTRACT_DOTENV_KEY=from_dotenv\n"), 0644); err != nil {
		t.Fatalf("failed to write .env: %v", err)
	}

	// Ensure OS env is unset
	t.Setenv("TEST_EXTRACT_DOTENV_KEY", "")
	os.Unsetenv("TEST_EXTRACT_DOTENV_KEY")

	// Write board.yaml with env variable
	yamlFile := filepath.Join(mhgoDir, "board.yaml")
	if err := os.WriteFile(yamlFile, []byte("path: $env:TEST_EXTRACT_DOTENV_KEY\n"), 0644); err != nil {
		t.Fatalf("failed to write board.yaml: %v", err)
	}

	result, err := config.Load(tmpDir, "board", map[string]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result["path"] != "from_dotenv" {
		t.Errorf("expected path %q, got %q", "from_dotenv", result["path"])
	}
}

// TestLoad_DotEnv_OSEnvWins tests that OS environment takes precedence over .env.
func TestLoad_DotEnv_OSEnvWins(t *testing.T) {
	tmpDir := t.TempDir()

	// Create _mhgo/ directory
	mhgoDir := filepath.Join(tmpDir, "_mhgo")
	if err := os.Mkdir(mhgoDir, 0755); err != nil {
		t.Fatalf("failed to create _mhgo: %v", err)
	}

	// Create .env file
	envFile := filepath.Join(tmpDir, ".env")
	if err := os.WriteFile(envFile, []byte("TEST_EXTRACT_OS_WINS=dotenv_val\n"), 0644); err != nil {
		t.Fatalf("failed to write .env: %v", err)
	}

	// Set OS environment
	t.Setenv("TEST_EXTRACT_OS_WINS", "os_val")

	// Write board.yaml with env variable
	yamlFile := filepath.Join(mhgoDir, "board.yaml")
	if err := os.WriteFile(yamlFile, []byte("path: $env:TEST_EXTRACT_OS_WINS\n"), 0644); err != nil {
		t.Fatalf("failed to write board.yaml: %v", err)
	}

	result, err := config.Load(tmpDir, "board", map[string]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result["path"] != "os_val" {
		t.Errorf("expected path %q, got %q", "os_val", result["path"])
	}
}

// TestLoad_DotEnv_MalformedLine tests that malformed .env lines are silently skipped.
func TestLoad_DotEnv_MalformedLine(t *testing.T) {
	tmpDir := t.TempDir()

	// Create _mhgo/ directory
	mhgoDir := filepath.Join(tmpDir, "_mhgo")
	if err := os.Mkdir(mhgoDir, 0755); err != nil {
		t.Fatalf("failed to create _mhgo: %v", err)
	}

	// Create .env file with good and malformed lines
	envFile := filepath.Join(tmpDir, ".env")
	if err := os.WriteFile(envFile, []byte("GOOD_KEY=val\nMALFORMED_NO_EQUALS\n"), 0644); err != nil {
		t.Fatalf("failed to write .env: %v", err)
	}

	result, err := config.Load(tmpDir, "board", map[string]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should succeed even though .env has malformed line
	if result == nil {
		t.Fatalf("expected result, got nil")
	}
}

// TestLoad_DotEnv_Comment tests that comment lines in .env are skipped.
func TestLoad_DotEnv_Comment(t *testing.T) {
	tmpDir := t.TempDir()

	// Create _mhgo/ directory
	mhgoDir := filepath.Join(tmpDir, "_mhgo")
	if err := os.Mkdir(mhgoDir, 0755); err != nil {
		t.Fatalf("failed to create _mhgo: %v", err)
	}

	// Create .env file with comment
	envFile := filepath.Join(tmpDir, ".env")
	if err := os.WriteFile(envFile, []byte("# this is a comment\nREAL_KEY=real_val\n"), 0644); err != nil {
		t.Fatalf("failed to write .env: %v", err)
	}

	result, err := config.Load(tmpDir, "board", map[string]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result == nil {
		t.Fatalf("expected result, got nil")
	}
}

// TestLoad_DotEnv_Absent tests that a missing .env file does not cause an error.
func TestLoad_DotEnv_Absent(t *testing.T) {
	tmpDir := t.TempDir()

	// Create _mhgo/ directory (no .env file)
	mhgoDir := filepath.Join(tmpDir, "_mhgo")
	if err := os.Mkdir(mhgoDir, 0755); err != nil {
		t.Fatalf("failed to create _mhgo: %v", err)
	}

	result, err := config.Load(tmpDir, "board", map[string]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result == nil {
		t.Fatalf("expected result, got nil")
	}
}

// TestLoad_LiteralQuestionMark tests that literal ? characters are preserved.
func TestLoad_LiteralQuestionMark(t *testing.T) {
	tmpDir := t.TempDir()

	// Create _mhgo/ directory
	mhgoDir := filepath.Join(tmpDir, "_mhgo")
	if err := os.Mkdir(mhgoDir, 0755); err != nil {
		t.Fatalf("failed to create _mhgo: %v", err)
	}

	// Write board.yaml with literal question mark
	yamlFile := filepath.Join(mhgoDir, "board.yaml")
	if err := os.WriteFile(yamlFile, []byte("url: \"http://host?q=1\"\n"), 0644); err != nil {
		t.Fatalf("failed to write board.yaml: %v", err)
	}

	result, err := config.Load(tmpDir, "board", map[string]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result["url"] != "http://host?q=1" {
		t.Errorf("expected url %q, got %q", "http://host?q=1", result["url"])
	}
}

// stringContains is a helper to check if a substring exists in a string.
func stringContains(s, substr string) bool {
	return strings.Contains(s, substr)
}
