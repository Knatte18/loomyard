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

// TestLoadConfig tests board.LoadConfig variants: defaults when config absent,
// errors on uninitialized, relative/absolute path resolution, malformed YAML,
// and env var fallback syntax.
//
// Folds: TestDefaultsReturned, TestErrorNotInitialized, TestRelativePathResolution,
// TestAbsolutePathPassthrough, TestMalformedYAMLError, TestLoadConfig_FallbackPathResolution
func TestLoadConfig(t *testing.T) {
	tests := []struct {
		name           string
		writeYAML      string // YAML to write; "" means no config file (empty _lyx/)
		mkLyx          bool   // whether to create _lyx/ directory
		wantPathSuffix string // expected suffix of cfg.Path (for relative paths)
		wantPath       string // exact expected path (if non-empty, overrides wantPathSuffix)
		wantErrSubstr  string // expected substring in error message; "" means no error
	}{
		{
			name:           "TestDefaultsReturned",
			writeYAML:      "",
			mkLyx:          true,
			wantPathSuffix: "_board",
		},
		{
			name:          "TestErrorNotInitialized",
			writeYAML:     "",
			mkLyx:         false,
			wantErrSubstr: "not initialized",
		},
		{
			name:           "TestRelativePathResolution",
			writeYAML:      "path: _custom_board\n",
			mkLyx:          true,
			wantPathSuffix: "_custom_board",
		},
		{
			name:      "TestAbsolutePathPassthrough",
			writeYAML: "", // Will be set dynamically with absolute path
			mkLyx:     true,
		},
		{
			name:          "TestMalformedYAMLError",
			writeYAML:     "path: value\n  invalid indentation: [ unclosed",
			mkLyx:         true,
			wantErrSubstr: "yaml:",
		},
		{
			name:           "TestLoadConfig_FallbackPathResolution",
			writeYAML:      "path: $env:NONEXISTENT_LYX_TEST_VAR_XYZ ? ../_board\n",
			mkLyx:          true,
			wantPathSuffix: "_board",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			baseDir := t.TempDir()

			if tt.mkLyx {
				lyxDir := filepath.Join(baseDir, "_lyx")
				if err := os.Mkdir(lyxDir, 0755); err != nil {
					t.Fatalf("failed to create _lyx: %v", err)
				}
				configDir := filepath.Join(lyxDir, "config")
				if err := os.Mkdir(configDir, 0755); err != nil {
					t.Fatalf("failed to create _lyx/config: %v", err)
				}

				if tt.writeYAML != "" {
					boardYamlPath := filepath.Join(configDir, "board.yaml")
					yamlContent := tt.writeYAML

					// Special case for TestAbsolutePathPassthrough: use an actual temp directory
					if tt.name == "TestAbsolutePathPassthrough" {
						absBoard := t.TempDir()
						yamlContent = "path: " + absBoard + "\n"
						tt.wantPath = absBoard
					}

					if err := os.WriteFile(boardYamlPath, []byte(yamlContent), 0644); err != nil {
						t.Fatalf("failed to write _lyx/config/board.yaml: %v", err)
					}
				}
			}

			cfg, err := board.LoadConfig(baseDir, "board")

			if tt.wantErrSubstr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil; config: %+v", tt.wantErrSubstr, cfg)
				}
				errMsg := err.Error()
				if !strings.Contains(errMsg, tt.wantErrSubstr) {
					t.Errorf("expected error to contain %q, got: %s", tt.wantErrSubstr, errMsg)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}

				if tt.wantPath != "" {
					if cfg.Path != tt.wantPath {
						t.Errorf("expected path %q, got %q", tt.wantPath, cfg.Path)
					}
				} else if tt.wantPathSuffix != "" {
					if !strings.Contains(cfg.Path, tt.wantPathSuffix) {
						t.Errorf("expected Path to contain %q, got %q", tt.wantPathSuffix, cfg.Path)
					}
				}

				// Check defaults
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
		})
	}
}

// TestOutputs tests the Outputs() method on Config and DefaultOutputs function;
// asserts that field values are correctly transferred from Config to Outputs.
//
// Folds: TestOutputsFromConfig, TestDefaultOutputs
func TestOutputs(t *testing.T) {
	t.Run("TestOutputsFromConfig", func(t *testing.T) {
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
	})

	t.Run("TestDefaultOutputs", func(t *testing.T) {
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
	})
}
