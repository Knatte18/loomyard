// config_test.go — tests for weft configuration.

package weft

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Pathspec != "_lyx" {
		t.Errorf("DefaultConfig().Pathspec = %q; want %q", cfg.Pathspec, "_lyx")
	}
}

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

func TestLoadConfig(t *testing.T) {
	tests := []struct {
		name           string
		writeYAML      bool
		mkLyx          bool
		wantPathspec   string
		wantErrSubstr  string
	}{
		{
			name:          "TestLoadConfig_DefaultWhenNoYAML",
			writeYAML:     false,
			mkLyx:         true,
			wantPathspec:  "_lyx",
			wantErrSubstr: "",
		},
		{
			name:          "TestLoadConfig_OverrideFromYAML",
			writeYAML:     true,
			mkLyx:         true,
			wantPathspec:  "_lyx _codeguide",
			wantErrSubstr: "",
		},
		{
			name:          "TestLoadConfig_MissingLyx",
			writeYAML:     false,
			mkLyx:         false,
			wantPathspec:  "",
			wantErrSubstr: "weft worktree or its _lyx is missing",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()

			if tt.mkLyx {
				lyxDir := filepath.Join(tmpDir, "_lyx")
				configDir := filepath.Join(lyxDir, "config")
				if err := os.MkdirAll(configDir, 0o755); err != nil {
					t.Fatalf("MkdirAll: %v", err)
				}

				if tt.writeYAML {
					weftYAML := filepath.Join(configDir, "weft.yaml")
					yamlContent := "pathspec: _lyx _codeguide\n"
					if err := os.WriteFile(weftYAML, []byte(yamlContent), 0o644); err != nil {
						t.Fatalf("WriteFile: %v", err)
					}
				}
			}

			cfg, err := LoadConfig(tmpDir)

			if tt.wantErrSubstr != "" {
				if err == nil {
					t.Fatalf("LoadConfig: expected error but got nil")
				}
				if !strings.Contains(err.Error(), tt.wantErrSubstr) {
					t.Errorf("error message = %q; want substring %q", err.Error(), tt.wantErrSubstr)
				}
				if cfg.Pathspec != "" {
					t.Errorf("Config should be zero-valued on error; got Pathspec=%q", cfg.Pathspec)
				}
			} else {
				if err != nil {
					t.Fatalf("LoadConfig: %v", err)
				}
				if cfg.Pathspec != tt.wantPathspec {
					t.Errorf("Pathspec = %q; want %q", cfg.Pathspec, tt.wantPathspec)
				}
			}
		})
	}
}
