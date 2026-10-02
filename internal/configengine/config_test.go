// config_test.go — unit tests for the strict and degrading config loaders.
//
// Tests cover: the strict Load contract using yamlengine + envsource, missing-key detection,
// absent-file errors, env variable resolution via templates, nested-key handling, the
// not-initialized error path, the ConfigDir/ConfigFile/ LyxDirName path constructors configengine
// singly declares, and the degrading LoadOrTemplate contract -- its two proven-absence fallback
// triggers, the strict-when-present boundary it shares with Load, and the absence-only
// discrimination that keeps a stat failure from being mistaken for absence.

package configengine_test

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
	"gopkg.in/yaml.v3"
)

// TestLoad_HappyPath tests that all template keys present in file round-trip correctly.
func TestLoad_HappyPath(t *testing.T) {
	tmpDir := t.TempDir()

	// Create _lyx/config/ directories
	lyxDir := filepath.Join(tmpDir, lyxdirs.LyxDirName)
	if err := os.Mkdir(lyxDir, 0755); err != nil {
		t.Fatalf("failed to create _lyx: %v", err)
	}
	configDir := configengine.ConfigDir(tmpDir)
	if err := os.Mkdir(configDir, 0755); err != nil {
		t.Fatalf("failed to create _lyx/config: %v", err)
	}

	// Simple template with two keys
	template := []byte("path: _board\nhome: Home.md\n")

	// Write config file matching template
	yamlFile := configengine.ConfigFile(tmpDir, "board")
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

// captureLog redirects the logger's stderr half into a buffer at Info verbosity for the test and
// restores the defaults in t.Cleanup.
func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	logger.SetOutput(&buf)
	logger.SetVerbosity(1)
	t.Cleanup(func() {
		logger.SetOutput(os.Stderr)
		logger.SetVerbosity(0)
	})
	return &buf
}

// writeConfig creates _lyx/config/ under a fresh temp dir, writes content as module's config file
// and returns the base dir and the file path.
func writeConfig(t *testing.T, module, content string) (baseDir, path string) {
	t.Helper()
	baseDir = t.TempDir()
	if err := os.MkdirAll(configengine.ConfigDir(baseDir), 0755); err != nil {
		t.Fatalf("failed to create _lyx/config: %v", err)
	}
	path = configengine.ConfigFile(baseDir, module)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write %s.yaml: %v", module, err)
	}
	return baseDir, path
}

// assertFileUnchanged fails when the file at path no longer holds want.
func assertFileUnchanged(t *testing.T, path, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to re-read %s: %v", path, err)
	}
	if string(got) != want {
		t.Errorf("config file was rewritten: got %q, want %q", got, want)
	}
}

// assertOneFillLine fails unless the captured log holds exactly one fill line naming the module and
// the key-path.
func assertOneFillLine(t *testing.T, buf *bytes.Buffer, module, keyPath string) {
	t.Helper()
	log := buf.String()
	if n := strings.Count(log, "filled missing keys from template"); n != 1 {
		t.Fatalf("expected exactly one fill line, got %d in log: %s", n, log)
	}
	if !strings.Contains(log, "module="+module) {
		t.Errorf("fill line does not name module %q: %s", module, log)
	}
	if !strings.Contains(log, keyPath) {
		t.Errorf("fill line does not name key-path %q: %s", keyPath, log)
	}
}

// TestLoad_MissingKey tests that a template key the file lacks loads at its template default, the
// file stays byte-identical and one fill line is logged.
func TestLoad_MissingKey(t *testing.T) {
	buf := captureLog(t)
	content := "path: custom_path\n"
	tmpDir, yamlFile := writeConfig(t, "board", content)

	resolved, err := configengine.Load(tmpDir, "board", []byte("path: _board\nhome: Home.md\n"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result map[string]string
	if err := yaml.Unmarshal(resolved, &result); err != nil {
		t.Fatalf("failed to unmarshal resolved config: %v", err)
	}
	if result["path"] != "custom_path" {
		t.Errorf("expected path %q, got %q", "custom_path", result["path"])
	}
	if result["home"] != "Home.md" {
		t.Errorf("expected home %q (template default), got %q", "Home.md", result["home"])
	}
	assertFileUnchanged(t, yamlFile, content)
	assertOneFillLine(t, buf, "board", "home")
}

// TestLoad_CompleteFileLogsNoFill tests that a file holding every template key logs no fill line.
func TestLoad_CompleteFileLogsNoFill(t *testing.T) {
	buf := captureLog(t)
	tmpDir, _ := writeConfig(t, "board", "path: a\nhome: b\n")

	if _, err := configengine.Load(tmpDir, "board", []byte("path: _board\nhome: Home.md\n")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(buf.String(), "filled missing keys") {
		t.Errorf("unexpected fill line for a complete file: %s", buf.String())
	}
}

// TestLoad_FillKeepsExtraAndEmptyValues tests that an extra file key survives beside a filled one
// and that a present empty string and an emptied list are kept rather than refilled.
func TestLoad_FillKeepsExtraAndEmptyValues(t *testing.T) {
	buf := captureLog(t)
	template := []byte("name: tpl\nlabel: tpl\nrequire_pr_to_base:\n  - main\nadded: yes\n")
	content := "extra_key: extra\nname: \"\"\nlabel: x\nrequire_pr_to_base: []\n"
	tmpDir, yamlFile := writeConfig(t, "board", content)

	resolved, err := configengine.Load(tmpDir, "board", template)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result map[string]interface{}
	if err := yaml.Unmarshal(resolved, &result); err != nil {
		t.Fatalf("failed to unmarshal resolved config: %v", err)
	}
	if result["extra_key"] != "extra" {
		t.Errorf("extra_key = %v; want it kept", result["extra_key"])
	}
	if result["name"] != "" {
		t.Errorf("name = %v; want the present empty string kept", result["name"])
	}
	if list, ok := result["require_pr_to_base"].([]interface{}); !ok || len(list) != 0 {
		t.Errorf("require_pr_to_base = %v; want the emptied list kept", result["require_pr_to_base"])
	}
	if result["added"] == nil {
		t.Errorf("added was not filled from the template")
	}
	assertFileUnchanged(t, yamlFile, content)
	assertOneFillLine(t, buf, "board", "added")
}

// TestLoad_FilledEnvMarkerUnsetRefuses tests that a filled ${env:NAME} marker whose variable is
// unset still refuses.
func TestLoad_FilledEnvMarkerUnsetRefuses(t *testing.T) {
	tmpDir, _ := writeConfig(t, "board", "path: custom\n")

	_, err := configengine.Load(tmpDir, "board", []byte("path: _board\ntoken: ${env:TEST_FILL_UNSET_VAR}\n"))
	if err == nil {
		t.Fatalf("expected error for a filled marker with an unset variable, got nil")
	}
	if !strings.Contains(err.Error(), "TEST_FILL_UNSET_VAR") {
		t.Errorf("expected error naming the variable, got: %v", err)
	}
}

// TestLoad_FillRefusals tests that a shape mismatch, a key missing inside a list element and
// unparseable YAML each refuse and name the key-path or the file.
func TestLoad_FillRefusals(t *testing.T) {
	tests := []struct {
		name     string
		template string
		content  string
		wantErr  []string
	}{
		{
			name:     "null where the template holds a mapping",
			template: "server:\n  host: localhost\n",
			content:  "server:\n",
			wantErr:  []string{"server"},
		},
		{
			name:     "mapping where the template holds a scalar",
			template: "server: localhost\n",
			content:  "server:\n  host: x\n",
			wantErr:  []string{"server"},
		},
		{
			name:     "key missing inside a present list element",
			template: "items:\n  - name: a\n    size: 1\n",
			content:  "items:\n  - name: a\n",
			wantErr:  []string{"missing keys", "items"},
		},
		{
			name:     "unparseable file",
			template: "path: _board\n",
			content:  "path: [unclosed\n",
			wantErr:  []string{"board.yaml"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tmpDir, _ := writeConfig(t, "board", tc.content)

			_, err := configengine.Load(tmpDir, "board", []byte(tc.template))
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			for _, want := range tc.wantErr {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("expected error containing %q, got: %v", want, err)
				}
			}
			if strings.Contains(err.Error(), "lyx config reconcile") {
				t.Errorf("a refusal reconcile cannot fix must not hint at it, got: %v", err)
			}
		})
	}
}

// TestLoad_AbsentFile tests that missing config file returns an error.
func TestLoad_AbsentFile(t *testing.T) {
	tmpDir := t.TempDir()

	// Create _lyx/config/ directories but NOT board.yaml
	lyxDir := filepath.Join(tmpDir, lyxdirs.LyxDirName)
	if err := os.Mkdir(lyxDir, 0755); err != nil {
		t.Fatalf("failed to create _lyx: %v", err)
	}
	configDir := configengine.ConfigDir(tmpDir)
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
	lyxDir := filepath.Join(tmpDir, lyxdirs.LyxDirName)
	if err := os.Mkdir(lyxDir, 0755); err != nil {
		t.Fatalf("failed to create _lyx: %v", err)
	}
	configDir := configengine.ConfigDir(tmpDir)
	if err := os.Mkdir(configDir, 0755); err != nil {
		t.Fatalf("failed to create _lyx/config: %v", err)
	}

	// Set environment variable
	t.Setenv("TEST_CONFIG_VAR", "resolved_value")

	// Template with an env marker
	template := []byte("path: ${env:TEST_CONFIG_VAR}\n")

	// Config file with the same env marker
	yamlFile := configengine.ConfigFile(tmpDir, "board")
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
	lyxDir := filepath.Join(tmpDir, lyxdirs.LyxDirName)
	if err := os.Mkdir(lyxDir, 0755); err != nil {
		t.Fatalf("failed to create _lyx: %v", err)
	}
	configDir := configengine.ConfigDir(tmpDir)
	if err := os.Mkdir(configDir, 0755); err != nil {
		t.Fatalf("failed to create _lyx/config: %v", err)
	}

	// Do NOT set TEST_OPTIONAL_VAR; the default should be used

	// Template with optional env
	template := []byte("path: ${env:TEST_OPTIONAL_VAR:-default_path}\n")

	// Config file with optional env
	yamlFile := configengine.ConfigFile(tmpDir, "board")
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
	lyxDir := filepath.Join(tmpDir, lyxdirs.LyxDirName)
	if err := os.Mkdir(lyxDir, 0755); err != nil {
		t.Fatalf("failed to create _lyx: %v", err)
	}
	configDir := configengine.ConfigDir(tmpDir)
	if err := os.Mkdir(configDir, 0755); err != nil {
		t.Fatalf("failed to create _lyx/config: %v", err)
	}

	// Template with one key
	template := []byte("path: _board\n")

	// Config file with extra key
	yamlFile := configengine.ConfigFile(tmpDir, "board")
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
	lyxDir := filepath.Join(tmpDir, lyxdirs.LyxDirName)
	if err := os.Mkdir(lyxDir, 0755); err != nil {
		t.Fatalf("failed to create _lyx: %v", err)
	}
	configDir := configengine.ConfigDir(tmpDir)
	if err := os.Mkdir(configDir, 0755); err != nil {
		t.Fatalf("failed to create _lyx/config: %v", err)
	}

	// Template with nested structure
	template := []byte("server:\n  host: localhost\n  port: '8080'\n")

	// Config file with nested values
	yamlFile := configengine.ConfigFile(tmpDir, "test")
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
	lyxDir := filepath.Join(tmpDir, lyxdirs.LyxDirName)
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

// TestFindBaseDir_Absent_SatisfiesErrNotInitialized tests that an absent _lyx/ directory's error
// satisfies errors.Is(err, configengine.ErrNotInitialized) -- the sentinel is wrapped, not returned
// bare -- while the rendered message still contains "not initialized", since four strict callers
// outside this task depend on that text.
func TestFindBaseDir_Absent_SatisfiesErrNotInitialized(t *testing.T) {
	tmpDir := t.TempDir()
	// Do NOT create _lyx/

	_, err := configengine.FindBaseDir(tmpDir)
	if err == nil {
		t.Fatalf("expected error for absent _lyx/, got nil")
	}

	if !errors.Is(err, configengine.ErrNotInitialized) {
		t.Errorf("expected errors.Is(err, ErrNotInitialized) to hold, got: %v", err)
	}
	if !strings.Contains(err.Error(), "not initialized") {
		t.Errorf("expected error containing 'not initialized', got: %v", err)
	}
}

// TestErrNotInitialized_NotMatchedByUnrelatedError tests that a hand-constructed, unrelated error
// does not satisfy errors.Is(err, configengine.ErrNotInitialized).
func TestErrNotInitialized_NotMatchedByUnrelatedError(t *testing.T) {
	unrelated := errors.New("not initialized")

	if errors.Is(unrelated, configengine.ErrNotInitialized) {
		t.Errorf("expected a hand-constructed error with the same text to NOT satisfy errors.Is against the sentinel")
	}
}

// TestLoadOrTemplate_AbsentLyxDir tests that an absent _lyx/ directory resolves the caller-supplied
// template instead of erroring.
func TestLoadOrTemplate_AbsentLyxDir(t *testing.T) {
	tmpDir := t.TempDir()
	// Do NOT create _lyx/

	template := []byte("path: _board\nhome: Home.md\n")

	resolved, err := configengine.LoadOrTemplate(tmpDir, "board", template)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result map[string]string
	if err := yaml.Unmarshal(resolved, &result); err != nil {
		t.Fatalf("failed to unmarshal resolved config: %v", err)
	}
	if result["path"] != "_board" {
		t.Errorf("expected path %q, got %q", "_board", result["path"])
	}
	if result["home"] != "Home.md" {
		t.Errorf("expected home %q, got %q", "Home.md", result["home"])
	}
}

// TestLoadOrTemplate_AbsentConfigFile tests that _lyx/ present but the module's config file absent
// resolves the caller-supplied template instead of erroring.
func TestLoadOrTemplate_AbsentConfigFile(t *testing.T) {
	tmpDir := t.TempDir()

	lyxDir := filepath.Join(tmpDir, lyxdirs.LyxDirName)
	if err := os.Mkdir(lyxDir, 0755); err != nil {
		t.Fatalf("failed to create _lyx: %v", err)
	}
	configDir := configengine.ConfigDir(tmpDir)
	if err := os.Mkdir(configDir, 0755); err != nil {
		t.Fatalf("failed to create _lyx/config: %v", err)
	}
	// Do NOT write board.yaml

	template := []byte("path: _board\n")

	resolved, err := configengine.LoadOrTemplate(tmpDir, "board", template)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result map[string]string
	if err := yaml.Unmarshal(resolved, &result); err != nil {
		t.Fatalf("failed to unmarshal resolved config: %v", err)
	}
	if result["path"] != "_board" {
		t.Errorf("expected path %q, got %q", "_board", result["path"])
	}
}

// TestLoadOrTemplate_BothPresent_MatchesLoad tests that _lyx/ and the config file both present
// returns a result identical to configengine.Load on the same inputs, proving the fallback never
// shadows a real file.
func TestLoadOrTemplate_BothPresent_MatchesLoad(t *testing.T) {
	tmpDir := t.TempDir()

	lyxDir := filepath.Join(tmpDir, lyxdirs.LyxDirName)
	if err := os.Mkdir(lyxDir, 0755); err != nil {
		t.Fatalf("failed to create _lyx: %v", err)
	}
	configDir := configengine.ConfigDir(tmpDir)
	if err := os.Mkdir(configDir, 0755); err != nil {
		t.Fatalf("failed to create _lyx/config: %v", err)
	}

	template := []byte("path: _board\nhome: Home.md\n")
	yamlFile := configengine.ConfigFile(tmpDir, "board")
	if err := os.WriteFile(yamlFile, []byte("path: custom_path\nhome: Index.md\n"), 0644); err != nil {
		t.Fatalf("failed to write board.yaml: %v", err)
	}

	wantResolved, wantErr := configengine.Load(tmpDir, "board", template)
	if wantErr != nil {
		t.Fatalf("unexpected error from Load: %v", wantErr)
	}

	gotResolved, gotErr := configengine.LoadOrTemplate(tmpDir, "board", template)
	if gotErr != nil {
		t.Fatalf("unexpected error from LoadOrTemplate: %v", gotErr)
	}

	if string(gotResolved) != string(wantResolved) {
		t.Errorf("LoadOrTemplate result = %q; want %q (Load result)", gotResolved, wantResolved)
	}
}

// TestLoadOrTemplate_PresentMissingKey tests that a config file present but missing a template key
// loads that key at its template default, leaves the file byte-identical and logs one fill line.
func TestLoadOrTemplate_PresentMissingKey(t *testing.T) {
	buf := captureLog(t)
	content := "path: custom_path\n"
	tmpDir, yamlFile := writeConfig(t, "board", content)

	resolved, err := configengine.LoadOrTemplate(tmpDir, "board", []byte("path: _board\nhome: Home.md\n"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result map[string]string
	if err := yaml.Unmarshal(resolved, &result); err != nil {
		t.Fatalf("failed to unmarshal resolved config: %v", err)
	}
	if result["path"] != "custom_path" || result["home"] != "Home.md" {
		t.Errorf("resolved = %v; want path custom_path and home at its template default", result)
	}
	assertFileUnchanged(t, yamlFile, content)
	assertOneFillLine(t, buf, "board", "home")
}

// TestLoadOrTemplate_PresentEmpty tests that a present but empty config file loads as the template,
// is left untouched and logs one fill line.
func TestLoadOrTemplate_PresentEmpty(t *testing.T) {
	buf := captureLog(t)
	tmpDir, yamlFile := writeConfig(t, "board", "")

	resolved, err := configengine.LoadOrTemplate(tmpDir, "board", []byte("path: _board\n"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result map[string]string
	if err := yaml.Unmarshal(resolved, &result); err != nil {
		t.Fatalf("failed to unmarshal resolved config: %v", err)
	}
	if result["path"] != "_board" {
		t.Errorf("expected path %q (template default), got %q", "_board", result["path"])
	}
	assertFileUnchanged(t, yamlFile, "")
	assertOneFillLine(t, buf, "board", "path")
}

// TestLoadOrTemplate_PresentCommentsOnly tests that a present comments-only config file loads as the
// template, is left untouched and logs one fill line.
func TestLoadOrTemplate_PresentCommentsOnly(t *testing.T) {
	buf := captureLog(t)
	content := "# just a comment\n"
	tmpDir, yamlFile := writeConfig(t, "board", content)

	resolved, err := configengine.LoadOrTemplate(tmpDir, "board", []byte("path: _board\n"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result map[string]string
	if err := yaml.Unmarshal(resolved, &result); err != nil {
		t.Fatalf("failed to unmarshal resolved config: %v", err)
	}
	if result["path"] != "_board" {
		t.Errorf("expected path %q (template default), got %q", "_board", result["path"])
	}
	assertFileUnchanged(t, yamlFile, content)
	assertOneFillLine(t, buf, "board", "path")
}

// TestLoadOrTemplate_EnvOverride tests that the fallback path honours an env override: a variable
// referenced by an ${env:NAME:-default} marker in the template, set via t.Setenv, lands in the
// returned bytes with no _lyx/ anywhere on disk.
func TestLoadOrTemplate_EnvOverride(t *testing.T) {
	tmpDir := t.TempDir()
	// Do NOT create _lyx/

	t.Setenv("TEST_FALLBACK_VAR", "overridden_value")

	template := []byte("path: ${env:TEST_FALLBACK_VAR:-default_path}\n")

	resolved, err := configengine.LoadOrTemplate(tmpDir, "board", template)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result map[string]string
	if err := yaml.Unmarshal(resolved, &result); err != nil {
		t.Fatalf("failed to unmarshal resolved config: %v", err)
	}
	if result["path"] != "overridden_value" {
		t.Errorf("expected path %q (from env override), got %q", "overridden_value", result["path"])
	}
}

// TestLoadOrTemplate_AbsentBaseDirAndEnv tests that the fallback path with an absent .env and an
// absent baseDir returns no error.
func TestLoadOrTemplate_AbsentBaseDirAndEnv(t *testing.T) {
	// baseDir itself does not exist on disk -- not even as an empty directory -- so _lyx/ and
	// .env are both provably absent.
	baseDir := filepath.Join(t.TempDir(), "does-not-exist")

	template := []byte("path: _board\n")

	resolved, err := configengine.LoadOrTemplate(baseDir, "board", template)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resolved == nil {
		t.Fatalf("expected resolved bytes, got nil")
	}
}

// TestLoadOrTemplate_UnsetRequiredEnv_WrapsAsConfigTemplate exercises the fallback tail's own error
// wrap: with no _lyx/ on disk, a synthetic template containing a required ${env:NAME} marker whose
// variable is unset must return a non-nil error whose message contains "config template:" and the
// module name, and must NOT contain "config file" -- pinning the "%s config template: %w" wrap keyed
// on module and pinning that a fallback-path error never names a config-file path that does not
// exist.
func TestLoadOrTemplate_UnsetRequiredEnv_WrapsAsConfigTemplate(t *testing.T) {
	tmpDir := t.TempDir()
	// Do NOT create _lyx/

	template := []byte("path: ${env:TEST_UNSET_REQUIRED_VAR}\n")

	_, err := configengine.LoadOrTemplate(tmpDir, "board", template)
	if err == nil {
		t.Fatalf("expected error for unset required env var, got nil")
	}
	if !strings.Contains(err.Error(), "config template:") {
		t.Errorf("expected error containing 'config template:', got: %v", err)
	}
	if !strings.Contains(err.Error(), "board") {
		t.Errorf("expected error containing module name 'board', got: %v", err)
	}
	if strings.Contains(err.Error(), "config file") {
		t.Errorf("expected error to NOT contain 'config file', got: %v", err)
	}
}

// TestLoadOrTemplate_LyxDirStatFailure_DoesNotFallback tests absence-only discrimination: an _lyx/
// that exists but cannot be stat'd makes LoadOrTemplate return an error rather than falling back to
// the template.
func TestLoadOrTemplate_LyxDirStatFailure_DoesNotFallback(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod-based permission denial is not meaningful on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory mode bits")
	}

	tmpDir := t.TempDir()
	t.Cleanup(func() {
		if err := os.Chmod(tmpDir, 0755); err != nil {
			t.Errorf("failed to restore tmpDir mode: %v", err)
		}
	})

	lyxDir := filepath.Join(tmpDir, lyxdirs.LyxDirName)
	if err := os.Mkdir(lyxDir, 0755); err != nil {
		t.Fatalf("failed to create _lyx: %v", err)
	}

	if err := os.Chmod(tmpDir, 0o000); err != nil {
		t.Fatalf("failed to chmod tmpDir: %v", err)
	}

	template := []byte("path: _board\n")

	resolved, err := configengine.LoadOrTemplate(tmpDir, "board", template)
	if err == nil {
		t.Fatalf("expected error for unstattable _lyx/, got nil")
	}
	if errors.Is(err, configengine.ErrNotInitialized) {
		t.Errorf("expected a stat failure to NOT satisfy errors.Is(err, ErrNotInitialized), got: %v", err)
	}
	if resolved != nil {
		t.Errorf("expected nil resolved bytes, got: %v", resolved)
	}
}

// TestConfigDir verifies that ConfigDir joins baseDir with LyxDirName and the "config" subdirectory
// — moved here from lyxcwd's own unit test now that configengine is the single declarer of the
// "_lyx/config" path shape.
func TestConfigDir(t *testing.T) {
	t.Parallel()

	baseDir := "/home/user/project"
	got := configengine.ConfigDir(baseDir)
	want := filepath.Join(baseDir, lyxdirs.LyxDirName, "config")

	if got != want {
		t.Errorf("ConfigDir(%q) = %q; want %q", baseDir, got, want)
	}
}

// TestConfigFile verifies that ConfigFile joins ConfigDir with the module's ".yaml" filename —
// moved here from lyxcwd's own unit test alongside TestConfigDir.
func TestConfigFile(t *testing.T) {
	t.Parallel()

	baseDir := "/home/user/project"
	module := "myapp"
	got := configengine.ConfigFile(baseDir, module)
	want := filepath.Join(baseDir, lyxdirs.LyxDirName, "config", "myapp.yaml")

	if got != want {
		t.Errorf("ConfigFile(%q, %q) = %q; want %q", baseDir, module, got, want)
	}
}

// TestConfigFileRel verifies that ConfigFileRel joins LyxDirName, "config", and the module's
// ".yaml" filename into an anchor-relative path, that the result is never absolute, and that it
// stays in lockstep with ConfigFile so the two accessors can never drift apart.
func TestConfigFileRel(t *testing.T) {
	t.Parallel()

	for _, module := range []string{"loom", "board"} {
		got := configengine.ConfigFileRel(module)
		want := filepath.Join(lyxdirs.LyxDirName, "config", module+".yaml")

		if got != want {
			t.Errorf("ConfigFileRel(%q) = %q; want %q", module, got, want)
		}
		if filepath.IsAbs(got) {
			t.Errorf("ConfigFileRel(%q) = %q; want a relative path", module, got)
		}
	}

	base := "/home/user/project"
	module := "myapp"
	got := configengine.ConfigFile(base, module)
	want := filepath.Join(base, configengine.ConfigFileRel(module))
	if got != want {
		t.Errorf("ConfigFile(%q, %q) = %q; want filepath.Join(base, ConfigFileRel(module)) = %q", base, module, got, want)
	}
}

// TestLyxDirNameConstant verifies that LyxDirName is exported and has the expected value — moved
// here from lyxcwd's own unit test now that internal/lyxdirs is the sole declarer of the "_lyx"
// token, per the Lyxdirs Single-Declarer Invariant.
func TestLyxDirNameConstant(t *testing.T) {
	t.Parallel()

	if lyxdirs.LyxDirName != "_lyx" {
		t.Errorf("LyxDirName = %q; want %q", lyxdirs.LyxDirName, "_lyx")
	}
}
