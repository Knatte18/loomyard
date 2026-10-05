// config_test.go — unit tests for boardengine.LoadConfig.
//
// Covers: happy-path with template keys present, missing-key error, absolute and relative path
// resolution, environment variable resolution, and not-initialized error path.

package boardengine_test

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
)

// TestLoadConfig_HappyPath tests that LoadConfig loads a valid config with all template keys
// present and resolves environment variables.
// LoadConfig no longer sets Config.Path;
// the caller does that via fabricengine.BoardDir.
func TestLoadConfig_HappyPath(t *testing.T) {
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

	// Write a config file with all template keys (path: is not a template key)
	configFile := configengine.ConfigFile(tmpDir, "board")
	content := `readme: Home.md
design_prefix: proposal-
`
	if err := os.WriteFile(configFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	cfg, err := boardengine.LoadConfig(tmpDir, "board")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Path is never set by LoadConfig; the caller sets it via fabricengine.BoardDir.
	if cfg.Path != "" {
		t.Errorf("expected Path to be empty after LoadConfig; got %q", cfg.Path)
	}
	if cfg.Readme != "Home.md" {
		t.Errorf("expected Readme %q, got %q", "Home.md", cfg.Readme)
	}
	if cfg.DesignPrefix != "proposal-" {
		t.Errorf("expected DesignPrefix %q, got %q", "proposal-", cfg.DesignPrefix)
	}
}

// TestLoadConfig_AbsolutePathResolution verifies that a path: key in the config file is ignored by
// LoadConfig because Config.Path has yaml:"-".
// The board data dir is geometry owned by fabricengine.BoardDir;
// the config key is a no-op.
func TestLoadConfig_AbsolutePathResolution(t *testing.T) {
	tmpDir := t.TempDir()
	absBoard := t.TempDir()

	// Create _lyx/config/ directories
	lyxDir := filepath.Join(tmpDir, lyxdirs.LyxDirName)
	if err := os.Mkdir(lyxDir, 0755); err != nil {
		t.Fatalf("failed to create _lyx: %v", err)
	}
	configDir := configengine.ConfigDir(tmpDir)
	if err := os.Mkdir(configDir, 0755); err != nil {
		t.Fatalf("failed to create _lyx/config: %v", err)
	}

	// Write config with an absolute path: key that should be ignored.
	configFile := configengine.ConfigFile(tmpDir, "board")
	content := `path: ` + absBoard + `
readme: Home.md
design_prefix: proposal-
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

// TestLoadConfig_RelativePathResolution verifies that a relative path: key in the config file is
// ignored by LoadConfig because Config.Path has yaml:"-".
// LoadConfig no longer performs any relative-path resolution;
// the board data dir is geometry owned by fabricengine.BoardDir.
func TestLoadConfig_RelativePathResolution(t *testing.T) {
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

	// Write config with a relative path: key that should be ignored.
	configFile := configengine.ConfigFile(tmpDir, "board")
	content := `path: ../custom_board
readme: Home.md
design_prefix: proposal-
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

// TestLoadConfig_EnvResolution verifies that a path: key using ${env:...} syntax in the config file
// is ignored by LoadConfig because Config.Path has yaml:"-".
// The env-override mechanism for the board data dir has been removed;
// the data dir is now geometry owned by fabricengine.BoardDir and is not env-overridable.
func TestLoadConfig_EnvResolution(t *testing.T) {
	tmpDir := t.TempDir()
	absBoard := t.TempDir()
	t.Setenv("TEST_BOARD_PATH", absBoard)

	// Create _lyx/config/ directories
	lyxDir := filepath.Join(tmpDir, lyxdirs.LyxDirName)
	if err := os.Mkdir(lyxDir, 0755); err != nil {
		t.Fatalf("failed to create _lyx: %v", err)
	}
	configDir := configengine.ConfigDir(tmpDir)
	if err := os.Mkdir(configDir, 0755); err != nil {
		t.Fatalf("failed to create _lyx/config: %v", err)
	}

	// Write config with an env-variable path: key that should be ignored.
	configFile := configengine.ConfigFile(tmpDir, "board")
	content := `path: ${env:TEST_BOARD_PATH}
readme: Home.md
design_prefix: proposal-
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

// TestLoadConfig_NotInitialized tests that missing _lyx/ returns the board-specific not-initialized
// error.
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
	if !strings.Contains(errMsg, "lyx fabric reconcile") {
		t.Errorf("expected error containing 'lyx fabric reconcile', got: %v", err)
	}
}

// writeBoardConfig writes content as the board config file under a fresh base dir and returns it.
func writeBoardConfig(t *testing.T, content string) string {
	t.Helper()
	tmpDir := t.TempDir()
	if err := os.Mkdir(filepath.Join(tmpDir, lyxdirs.LyxDirName), 0755); err != nil {
		t.Fatalf("failed to create _lyx: %v", err)
	}
	if err := os.Mkdir(configengine.ConfigDir(tmpDir), 0755); err != nil {
		t.Fatalf("failed to create _lyx/config: %v", err)
	}
	if err := os.WriteFile(configengine.ConfigFile(tmpDir, "board"), []byte(content), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}
	return tmpDir
}

// TestLoadConfig_LabelMaps asserts both label maps resolve in file order with their descriptions.
func TestLoadConfig_LabelMaps(t *testing.T) {
	dir := writeBoardConfig(t, `readme: Home.md
design_prefix: proposal-
types:
  idea: A thing to try
  bug: Broken
labels:
  urgent: Do first
  later:
`)

	cfg, err := boardengine.LoadConfig(dir, "board")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	wantTypes := []boardengine.Label{{Name: "idea", Description: "A thing to try"}, {Name: "bug", Description: "Broken"}}
	if !reflect.DeepEqual(cfg.Types, wantTypes) {
		t.Errorf("Types = %v; want %v", cfg.Types, wantTypes)
	}
	wantLabels := []boardengine.Label{{Name: "urgent", Description: "Do first"}, {Name: "later"}}
	if !reflect.DeepEqual(cfg.Labels, wantLabels) {
		t.Errorf("Labels = %v; want %v", cfg.Labels, wantLabels)
	}
}

// TestLoadConfig_LabelLists asserts the read-only list shape loads with empty descriptions.
func TestLoadConfig_LabelLists(t *testing.T) {
	dir := writeBoardConfig(t, `readme: Home.md
design_prefix: proposal-
types: [bug, idea]
labels: [urgent, later]
`)

	cfg, err := boardengine.LoadConfig(dir, "board")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	wantTypes := []boardengine.Label{{Name: "bug"}, {Name: "idea"}}
	if !reflect.DeepEqual(cfg.Types, wantTypes) {
		t.Errorf("Types = %v; want %v", cfg.Types, wantTypes)
	}
	wantLabels := []boardengine.Label{{Name: "urgent"}, {Name: "later"}}
	if !reflect.DeepEqual(cfg.Labels, wantLabels) {
		t.Errorf("Labels = %v; want %v", cfg.Labels, wantLabels)
	}
}

// TestLoadConfig_LabelsRefused asserts a scalar key and a non-scalar description are refused naming board.yaml and the key.
func TestLoadConfig_LabelsRefused(t *testing.T) {
	tests := []struct {
		name, content, key string
	}{
		{"scalar labels", "labels: urgent\n", "labels"},
		{"mapping description", "types:\n  bug:\n    nested: x\n", "types"},
		{"sequence description", "labels:\n  area: [a, b]\n", "labels"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := writeBoardConfig(t, "readme: Home.md\ndesign_prefix: proposal-\n"+tt.content)
			_, err := boardengine.LoadConfig(dir, "board")
			if err == nil {
				t.Fatal("expected an error")
			}
			if !strings.Contains(err.Error(), "board.yaml") || !strings.Contains(err.Error(), tt.key) {
				t.Errorf("error %q must name board.yaml and %q", err, tt.key)
			}
		})
	}
}

// TestLoadConfig_LabelsDefault asserts absent keys resolve to the template defaults.
func TestLoadConfig_LabelsDefault(t *testing.T) {
	dir := writeBoardConfig(t, `readme: Home.md
design_prefix: proposal-
`)

	cfg, err := boardengine.LoadConfig(dir, "board")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := cfg.Outputs().Types; !reflect.DeepEqual(got, []string{"bug", "enhancement"}) {
		t.Errorf("type names = %v; want [bug enhancement]", got)
	}
	for _, l := range cfg.Types {
		if l.Description == "" {
			t.Errorf("template type %q has no description", l.Name)
		}
	}
	if len(cfg.Labels) != 0 {
		t.Errorf("Labels = %v; want none", cfg.Labels)
	}
}

// TestOutputs_TypesFollowFileOrder asserts Outputs().Types lists the type names in the map's file order.
func TestOutputs_TypesFollowFileOrder(t *testing.T) {
	cfg := boardengine.Config{Types: []boardengine.Label{{Name: "idea"}, {Name: "bug"}, {Name: "enhancement"}}}
	if got := cfg.Outputs().Types; !reflect.DeepEqual(got, []string{"idea", "bug", "enhancement"}) {
		t.Errorf("Types = %v; want [idea bug enhancement]", got)
	}
}

// TestVocabulary asserts IsType and Known for a type, a plain label, an unknown label and the
// empty vocabulary of a path-only Config.
func TestVocabulary(t *testing.T) {
	v := boardengine.Config{Types: []boardengine.Label{{Name: "bug"}}, Labels: []boardengine.Label{{Name: "undecided"}}}.Vocabulary()
	tests := []struct {
		label         string
		isType, known bool
	}{
		{"bug", true, true},
		{"undecided", false, true},
		{"other", false, false},
	}
	for _, tt := range tests {
		if got := v.IsType(tt.label); got != tt.isType {
			t.Errorf("IsType(%q) = %v; want %v", tt.label, got, tt.isType)
		}
		if got := v.Known(tt.label); got != tt.known {
			t.Errorf("Known(%q) = %v; want %v", tt.label, got, tt.known)
		}
	}

	empty := boardengine.Config{Path: "/some/path"}.Vocabulary()
	if empty.IsType("bug") || empty.Known("bug") {
		t.Errorf("empty vocabulary must answer false to IsType and Known")
	}
}

// TestOutputs tests the Outputs() method on Config.
func TestOutputs(t *testing.T) {
	cfg := boardengine.Config{
		Path:         "/some/path",
		Readme:       "Home.md",
		DesignPrefix: "proposal-",
		Types:        []boardengine.Label{{Name: "bug"}, {Name: "enhancement"}},
	}

	out := cfg.Outputs()

	if !reflect.DeepEqual(out.Types, []string{"bug", "enhancement"}) {
		t.Errorf("expected Types [bug enhancement], got %v", out.Types)
	}

	if out.Readme != "Home.md" {
		t.Errorf("expected Readme %q, got %q", "Home.md", out.Readme)
	}
	if out.DesignPrefix != "proposal-" {
		t.Errorf("expected DesignPrefix %q, got %q", "proposal-", out.DesignPrefix)
	}
}
