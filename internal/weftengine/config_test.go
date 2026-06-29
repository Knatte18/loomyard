// config_test.go — tests for weft configuration.

package weftengine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/paths"
)

// TestConfigDirs tests the Dirs() method on Config.
func TestConfigDirs(t *testing.T) {
	tests := []struct {
		name     string
		pathspec string
		want     []string
	}{
		{"single", "_lyx", []string{"_lyx"}},
		{"multiple", "_lyx _codeguide", []string{"_lyx", "_codeguide"}},
		{"trailing_space", "_lyx ", []string{"_lyx"}},
		{"leading_space", " _lyx", []string{"_lyx"}},
		{"many_spaces", "_lyx  _codeguide", []string{"_lyx", "_codeguide"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Config{Pathspec: tt.pathspec}
			got := cfg.Dirs()
			if len(got) != len(tt.want) {
				t.Errorf("Dirs() returned %d items; want %d", len(got), len(tt.want))
				return
			}
			for i, w := range tt.want {
				if got[i] != w {
					t.Errorf("Dirs()[%d] = %q; want %q", i, got[i], w)
				}
			}
		})
	}
}

// TestLoadConfig_HappyPath tests that LoadConfig loads a valid config
// with all template keys present and pathspec is parsed correctly.
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

	// Write a config file with pathspec
	configFile := paths.ConfigFile(tmpDir, "weft")
	content := `pathspec: _lyx _codeguide
`
	if err := os.WriteFile(configFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	cfg, err := LoadConfig(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Pathspec != "_lyx _codeguide" {
		t.Errorf("expected Pathspec %q, got %q", "_lyx _codeguide", cfg.Pathspec)
	}

	// Verify Dirs() works
	dirs := cfg.Dirs()
	if len(dirs) != 2 || dirs[0] != "_lyx" || dirs[1] != "_codeguide" {
		t.Errorf("expected Dirs() to split into [_lyx, _codeguide], got %v", dirs)
	}
}

// TestLoadConfig_NotInitialized tests that missing _lyx/ returns the
// weft-specific error message.
func TestLoadConfig_NotInitialized(t *testing.T) {
	tmpDir := t.TempDir()
	// Do NOT create _lyx/

	cfg, err := LoadConfig(tmpDir)
	if err == nil {
		t.Fatalf("expected error for not initialized, got nil; config: %+v", cfg)
	}

	errMsg := err.Error()
	if !strings.Contains(errMsg, "weft worktree or its _lyx is missing") {
		t.Errorf("expected error containing 'weft worktree or its _lyx is missing', got: %v", err)
	}
	if !strings.Contains(errMsg, tmpDir) {
		t.Errorf("expected error to contain weftBaseDir path, got: %v", err)
	}
}
