// config_test.go — unit tests for the generic config loader (config.go).
//
// Tests cover: uninitialized dir, defaults passthrough, YAML override, .lyx/ ignored,
// required env (set and unset), optional env (set, unset, with prefix), .env loading,
// and literal ? character.

package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/config"
)

// TestLoad_UninitializedDir tests that an error is returned when _lyx/
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

// TestLoad_Defaults tests that defaults are returned when _lyx/ exists
// but the YAML file is absent.
func TestLoad_Defaults(t *testing.T) {
	tmpDir := t.TempDir()

	// Create _lyx/ directory
	lyxDir := filepath.Join(tmpDir, "_lyx")
	if err := os.Mkdir(lyxDir, 0755); err != nil {
		t.Fatalf("failed to create _lyx: %v", err)
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

	// Create _lyx/ and _lyx/config/ directories
	lyxDir := filepath.Join(tmpDir, "_lyx")
	if err := os.Mkdir(lyxDir, 0755); err != nil {
		t.Fatalf("failed to create _lyx: %v", err)
	}
	configDir := filepath.Join(lyxDir, "config")
	if err := os.Mkdir(configDir, 0755); err != nil {
		t.Fatalf("failed to create _lyx/config: %v", err)
	}

	// Write board.yaml
	yamlFile := filepath.Join(configDir, "board.yaml")
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

// TestLoad_DotLyxIgnored tests that .lyx/ files are ignored (only _lyx/ is used).
func TestLoad_DotLyxIgnored(t *testing.T) {
	tmpDir := t.TempDir()

	// Create _lyx/ and _lyx/config/ directories with board.yaml
	lyxDir := filepath.Join(tmpDir, "_lyx")
	if err := os.Mkdir(lyxDir, 0755); err != nil {
		t.Fatalf("failed to create _lyx: %v", err)
	}
	configDir := filepath.Join(lyxDir, "config")
	if err := os.Mkdir(configDir, 0755); err != nil {
		t.Fatalf("failed to create _lyx/config: %v", err)
	}

	yamlFile := filepath.Join(configDir, "board.yaml")
	if err := os.WriteFile(yamlFile, []byte("path: correct\n"), 0644); err != nil {
		t.Fatalf("failed to write _lyx/config/board.yaml: %v", err)
	}

	// Create .lyx/ directory with board.yaml (should be ignored)
	dotLyxDir := filepath.Join(tmpDir, ".lyx")
	if err := os.Mkdir(dotLyxDir, 0755); err != nil {
		t.Fatalf("failed to create .lyx: %v", err)
	}

	dotYamlFile := filepath.Join(dotLyxDir, "board.yaml")
	if err := os.WriteFile(dotYamlFile, []byte("path: wrong\n"), 0644); err != nil {
		t.Fatalf("failed to write .lyx/board.yaml: %v", err)
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

	// Create _lyx/ and _lyx/config/ directories
	lyxDir := filepath.Join(tmpDir, "_lyx")
	if err := os.Mkdir(lyxDir, 0755); err != nil {
		t.Fatalf("failed to create _lyx: %v", err)
	}
	configDir := filepath.Join(lyxDir, "config")
	if err := os.Mkdir(configDir, 0755); err != nil {
		t.Fatalf("failed to create _lyx/config: %v", err)
	}

	// Set environment variable
	t.Setenv("TEST_EXTRACT_REQ_VAR", "expanded")

	// Write board.yaml with env variable
	yamlFile := filepath.Join(configDir, "board.yaml")
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

	// Create _lyx/ and _lyx/config/ directories
	lyxDir := filepath.Join(tmpDir, "_lyx")
	if err := os.Mkdir(lyxDir, 0755); err != nil {
		t.Fatalf("failed to create _lyx: %v", err)
	}
	configDir := filepath.Join(lyxDir, "config")
	if err := os.Mkdir(configDir, 0755); err != nil {
		t.Fatalf("failed to create _lyx/config: %v", err)
	}

	// Ensure variable is unset by not setting it at all
	// (t.Setenv with empty string leaves the var set; we need it completely absent)

	// Write board.yaml with unset env variable
	yamlFile := filepath.Join(configDir, "board.yaml")
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

	// Create _lyx/ and _lyx/config/ directories
	lyxDir := filepath.Join(tmpDir, "_lyx")
	if err := os.Mkdir(lyxDir, 0755); err != nil {
		t.Fatalf("failed to create _lyx: %v", err)
	}
	configDir := filepath.Join(lyxDir, "config")
	if err := os.Mkdir(configDir, 0755); err != nil {
		t.Fatalf("failed to create _lyx/config: %v", err)
	}

	// Set environment variable
	t.Setenv("TEST_EXTRACT_OPT_VAR", "set_value")

	// Write board.yaml with optional env variable
	yamlFile := filepath.Join(configDir, "board.yaml")
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

	// Create _lyx/ and _lyx/config/ directories
	lyxDir := filepath.Join(tmpDir, "_lyx")
	if err := os.Mkdir(lyxDir, 0755); err != nil {
		t.Fatalf("failed to create _lyx: %v", err)
	}
	configDir := filepath.Join(lyxDir, "config")
	if err := os.Mkdir(configDir, 0755); err != nil {
		t.Fatalf("failed to create _lyx/config: %v", err)
	}

	// Ensure variable is unset by not setting it at all
	// (t.Setenv with empty string leaves the var set; we need it completely absent)

	// Write board.yaml with optional env variable
	yamlFile := filepath.Join(configDir, "board.yaml")
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

	// Create _lyx/ and _lyx/config/ directories
	lyxDir := filepath.Join(tmpDir, "_lyx")
	if err := os.Mkdir(lyxDir, 0755); err != nil {
		t.Fatalf("failed to create _lyx: %v", err)
	}
	configDir := filepath.Join(lyxDir, "config")
	if err := os.Mkdir(configDir, 0755); err != nil {
		t.Fatalf("failed to create _lyx/config: %v", err)
	}

	// Ensure variable is unset by not setting it at all
	// (t.Setenv with empty string leaves the var set; we need it completely absent)

	// Write board.yaml with optional env variable with prefix
	yamlFile := filepath.Join(configDir, "board.yaml")
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

	// Create _lyx/ and _lyx/config/ directories
	lyxDir := filepath.Join(tmpDir, "_lyx")
	if err := os.Mkdir(lyxDir, 0755); err != nil {
		t.Fatalf("failed to create _lyx: %v", err)
	}
	configDir := filepath.Join(lyxDir, "config")
	if err := os.Mkdir(configDir, 0755); err != nil {
		t.Fatalf("failed to create _lyx/config: %v", err)
	}

	// Create .env file
	envFile := filepath.Join(tmpDir, ".env")
	if err := os.WriteFile(envFile, []byte("TEST_EXTRACT_DOTENV_KEY=from_dotenv\n"), 0644); err != nil {
		t.Fatalf("failed to write .env: %v", err)
	}

	// Ensure OS env is unset by not setting it at all
	// (t.Setenv with empty string leaves the var set; we need it completely absent)

	// Write board.yaml with env variable
	yamlFile := filepath.Join(configDir, "board.yaml")
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

	// Create _lyx/ and _lyx/config/ directories
	lyxDir := filepath.Join(tmpDir, "_lyx")
	if err := os.Mkdir(lyxDir, 0755); err != nil {
		t.Fatalf("failed to create _lyx: %v", err)
	}
	configDir := filepath.Join(lyxDir, "config")
	if err := os.Mkdir(configDir, 0755); err != nil {
		t.Fatalf("failed to create _lyx/config: %v", err)
	}

	// Create .env file
	envFile := filepath.Join(tmpDir, ".env")
	if err := os.WriteFile(envFile, []byte("TEST_EXTRACT_OS_WINS=dotenv_val\n"), 0644); err != nil {
		t.Fatalf("failed to write .env: %v", err)
	}

	// Set OS environment
	t.Setenv("TEST_EXTRACT_OS_WINS", "os_val")

	// Write board.yaml with env variable
	yamlFile := filepath.Join(configDir, "board.yaml")
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

	// Create _lyx/ and _lyx/config/ directories
	lyxDir := filepath.Join(tmpDir, "_lyx")
	if err := os.Mkdir(lyxDir, 0755); err != nil {
		t.Fatalf("failed to create _lyx: %v", err)
	}
	configDir := filepath.Join(lyxDir, "config")
	if err := os.Mkdir(configDir, 0755); err != nil {
		t.Fatalf("failed to create _lyx/config: %v", err)
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

	// Create _lyx/ and _lyx/config/ directories
	lyxDir := filepath.Join(tmpDir, "_lyx")
	if err := os.Mkdir(lyxDir, 0755); err != nil {
		t.Fatalf("failed to create _lyx: %v", err)
	}
	configDir := filepath.Join(lyxDir, "config")
	if err := os.Mkdir(configDir, 0755); err != nil {
		t.Fatalf("failed to create _lyx/config: %v", err)
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

	// Create _lyx/ directory (no .env file)
	lyxDir := filepath.Join(tmpDir, "_lyx")
	if err := os.Mkdir(lyxDir, 0755); err != nil {
		t.Fatalf("failed to create _lyx: %v", err)
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

	// Create _lyx/ and _lyx/config/ directories
	lyxDir := filepath.Join(tmpDir, "_lyx")
	if err := os.Mkdir(lyxDir, 0755); err != nil {
		t.Fatalf("failed to create _lyx: %v", err)
	}
	configDir := filepath.Join(lyxDir, "config")
	if err := os.Mkdir(configDir, 0755); err != nil {
		t.Fatalf("failed to create _lyx/config: %v", err)
	}

	// Write board.yaml with literal question mark
	yamlFile := filepath.Join(configDir, "board.yaml")
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

// TestFindBaseDir_Present tests that FindBaseDir returns the cwd when _lyx/ exists.
func TestFindBaseDir_Present(t *testing.T) {
	tmpDir := t.TempDir()

	// Create _lyx/ directory
	lyxDir := filepath.Join(tmpDir, "_lyx")
	if err := os.Mkdir(lyxDir, 0755); err != nil {
		t.Fatalf("failed to create _lyx: %v", err)
	}

	result, err := config.FindBaseDir(tmpDir)
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

	result, err := config.FindBaseDir(tmpDir)
	if err == nil {
		t.Fatalf("expected error, got nil; result: %v", result)
	}

	if result != "" {
		t.Errorf("expected empty string, got %q", result)
	}

	if !stringContains(err.Error(), "not initialized") {
		t.Errorf("expected error containing 'not initialized', got: %v", err)
	}
}

// TestLoad_OldFlatPathNotPickedUp ensures that the old flat path (_lyx/board.yaml)
// is NOT picked up after migration to _lyx/config/. This is a regression guard to
// verify the hard-cut migration does not accidentally fall back to the old path.
func TestLoad_OldFlatPathNotPickedUp(t *testing.T) {
	tmpDir := t.TempDir()

	// Create _lyx/ directory
	lyxDir := filepath.Join(tmpDir, "_lyx")
	if err := os.Mkdir(lyxDir, 0755); err != nil {
		t.Fatalf("failed to create _lyx: %v", err)
	}

	// Write board.yaml in the OLD flat location (_lyx/board.yaml)
	oldPath := filepath.Join(lyxDir, "board.yaml")
	oldContent := "path: old_flat_path\nhome: OldHome.md\n"
	if err := os.WriteFile(oldPath, []byte(oldContent), 0644); err != nil {
		t.Fatalf("failed to write _lyx/board.yaml: %v", err)
	}

	// Do NOT create _lyx/config/ or _lyx/config/board.yaml
	// Load should return defaults, not the old flat-path values
	defaults := map[string]string{
		"path": "default_path",
		"home": "Home.md",
	}

	result, err := config.Load(tmpDir, "board", defaults)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Assert that the values came from defaults, not from the old flat path.
	// If the old path was picked up, path would be "old_flat_path" and home would be "OldHome.md".
	if result["path"] != "default_path" {
		t.Errorf("expected path %q (from defaults), got %q; old flat path may have been picked up", "default_path", result["path"])
	}
	if result["home"] != "Home.md" {
		t.Errorf("expected home %q (from defaults), got %q; old flat path may have been picked up", "Home.md", result["home"])
	}
}

// stringContains is a helper to check if a substring exists in a string.
func stringContains(s, substr string) bool {
	return strings.Contains(s, substr)
}
