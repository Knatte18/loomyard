// config_test.go — unit tests for fabricengine.LoadConfig.
//
// Covers: happy-path with template keys present, branch_prefix/pathspec parsing, environment
// variable resolution, and not-initialized error path.

package fabricengine_test

import (
	"os"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
)

// TestLoadConfig covers LoadConfig over a hand-written fabric config:
// a valid config with all template keys parses both fields and Dirs();
// an empty branch_prefix stays the empty string;
// an environment variable in branch_prefix is resolved;
// and a missing _lyx/ returns the fabric-specific not-initialized error, naming the reconcile verb
// that fixes it.
//
//testtiming:keep LoadConfig parsing both fields and Dirs(), keeping an empty prefix, resolving env references and naming the reconcile verb when uninitialized; coverage of its blocks by other tests does not show an assertion of this
func TestLoadConfig(t *testing.T) {
	tests := []struct {
		name           string
		env            map[string]string
		content        string
		wantErrContain []string
		wantBranch     string
		wantPathspec   string
		wantDirs       []string
	}{
		{
			name:         "happy_path",
			content:      "branch_prefix: hanf/\npathspec: _lyx _extra\n",
			wantBranch:   "hanf/",
			wantPathspec: "_lyx _extra",
			wantDirs:     []string{"_lyx", "_extra"},
		},
		{
			name:         "empty_branch_prefix",
			content:      "branch_prefix: \"\"\npathspec: _lyx\n",
			wantBranch:   "",
			wantPathspec: "_lyx",
			wantDirs:     []string{"_lyx"},
		},
		{
			name:         "env_resolution",
			env:          map[string]string{"TEST_FABRIC_BRANCH_PREFIX": "feature/"},
			content:      "branch_prefix: ${env:TEST_FABRIC_BRANCH_PREFIX}\npathspec: _lyx\n",
			wantBranch:   "feature/",
			wantPathspec: "_lyx",
			wantDirs:     []string{"_lyx"},
		},
		{
			name:           "not_initialized",
			wantErrContain: []string{"not initialized", "lyx fabric reconcile"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for key, value := range tt.env {
				t.Setenv(key, value)
			}
			tmpDir := t.TempDir()

			if tt.content != "" {
				if err := os.MkdirAll(configengine.ConfigDir(tmpDir), 0755); err != nil {
					t.Fatalf("failed to create _lyx/config: %v", err)
				}
				if err := os.WriteFile(configengine.ConfigFile(tmpDir, "fabric"), []byte(tt.content), 0644); err != nil {
					t.Fatalf("failed to write config: %v", err)
				}
			}

			cfg, err := fabricengine.LoadConfig(tmpDir)
			if len(tt.wantErrContain) > 0 {
				if err == nil {
					t.Fatalf("expected error for not initialized, got nil; config: %+v", cfg)
				}
				for _, want := range tt.wantErrContain {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("expected error containing %q, got: %v", want, err)
					}
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if cfg.BranchPrefix != tt.wantBranch {
				t.Errorf("BranchPrefix = %q; want %q", cfg.BranchPrefix, tt.wantBranch)
			}
			if cfg.Pathspec != tt.wantPathspec {
				t.Errorf("Pathspec = %q; want %q", cfg.Pathspec, tt.wantPathspec)
			}
			gotDirs := cfg.Dirs()
			if len(gotDirs) != len(tt.wantDirs) {
				t.Fatalf("Dirs() = %v; want %v", gotDirs, tt.wantDirs)
			}
			for i := range tt.wantDirs {
				if gotDirs[i] != tt.wantDirs[i] {
					t.Errorf("Dirs()[%d] = %q; want %q", i, gotDirs[i], tt.wantDirs[i])
				}
			}
		})
	}
}
