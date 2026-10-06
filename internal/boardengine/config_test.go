// config_test.go — unit tests for boardengine.LoadConfig.
//
// Covers: scalar keys with template defaults, ignored path: keys, label maps and lists, label
// refusals, the not-initialized error path, Outputs and Vocabulary.

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

// TestLoadConfig_ScalarKeysAndTemplateDefaults asserts LoadConfig reads the scalar keys and resolves absent types and labels keys to the template defaults.
// A path: key of any shape is ignored because Config.Path has yaml:"-":
// the board data dir is geometry owned by fabricengine.BoardDir, with no relative-path resolution and no env override.
// It sets an environment variable, so it runs serially.
func TestLoadConfig_ScalarKeysAndTemplateDefaults(t *testing.T) {
	t.Setenv("TEST_BOARD_PATH", t.TempDir())
	tests := []struct {
		name    string
		pathKey string
	}{
		{"no path key", ""},
		{"absolute path key", "path: " + t.TempDir() + "\n"},
		{"relative path key", "path: ../custom_board\n"},
		{"env path key", "path: ${env:TEST_BOARD_PATH}\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := writeBoardConfig(t, tt.pathKey+"readme: Home.md\ndesign_prefix: proposal-\n")

			cfg, err := boardengine.LoadConfig(dir, "board")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if cfg.Path != "" {
				t.Errorf("expected Path to be empty; got %q", cfg.Path)
			}
			if cfg.Readme != "Home.md" {
				t.Errorf("expected Readme %q, got %q", "Home.md", cfg.Readme)
			}
			if cfg.DesignPrefix != "proposal-" {
				t.Errorf("expected DesignPrefix %q, got %q", "proposal-", cfg.DesignPrefix)
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
		})
	}
}

// TestLoadConfig_NotInitialized tests that an absent _lyx/ and an absent board.yaml under a present
// _lyx/config/ both return the board-specific not-initialized error, naming the file and the way forward.
func TestLoadConfig_NotInitialized(t *testing.T) {
	tests := []struct {
		name      string
		createDir bool
	}{
		{name: "absent _lyx", createDir: false},
		{name: "absent board.yaml under present _lyx/config", createDir: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			if tt.createDir {
				if err := os.MkdirAll(configengine.ConfigDir(tmpDir), 0755); err != nil {
					t.Fatalf("failed to create _lyx/config: %v", err)
				}
			}

			cfg, err := boardengine.LoadConfig(tmpDir, "board")
			if err == nil {
				t.Fatalf("expected error for not initialized, got nil; config: %+v", cfg)
			}

			errMsg := err.Error()
			if !strings.Contains(errMsg, "not initialized") {
				t.Errorf("expected error containing 'not initialized', got: %v", err)
			}
			if !strings.Contains(errMsg, configengine.ConfigFile(tmpDir, "board")) {
				t.Errorf("expected error naming the board.yaml path, got: %v", err)
			}
			if !strings.Contains(errMsg, "lyx fabric reconcile") {
				t.Errorf("expected error containing 'lyx fabric reconcile', got: %v", err)
			}
		})
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

// TestVocabulary asserts IsType and Known for a type, a plain label, an unknown label and the
// empty vocabulary of a path-only Config.
//
//testtiming:keep pins the IsType and Known answers for a type, a plain label, an unknown label and the empty vocabulary, which its covering test never asserts
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

// TestOutputs asserts Outputs carries the readme name, the design prefix and the type names in the map's file order.
//
//testtiming:keep pins the readme name, design prefix and type order that Outputs carries, which its covering test does not assert
func TestOutputs(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		cfg  boardengine.Config
		want boardengine.Outputs
	}{
		{
			name: "all fields",
			cfg: boardengine.Config{
				Path:         "/some/path",
				Readme:       "Home.md",
				DesignPrefix: "proposal-",
				Types:        []boardengine.Label{{Name: "bug"}, {Name: "enhancement"}},
			},
			want: boardengine.Outputs{Readme: "Home.md", DesignPrefix: "proposal-", Types: []string{"bug", "enhancement"}},
		},
		{
			name: "types follow file order",
			cfg:  boardengine.Config{Types: []boardengine.Label{{Name: "idea"}, {Name: "bug"}, {Name: "enhancement"}}},
			want: boardengine.Outputs{Types: []string{"idea", "bug", "enhancement"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.cfg.Outputs(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Outputs() = %+v; want %+v", got, tt.want)
			}
		})
	}
}
