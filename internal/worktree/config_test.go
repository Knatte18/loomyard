package worktree_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/worktree"
)

// TestLoadConfig covers worktree config resolution: defaults when worktree.yaml
// is absent, branch_prefix parsed from YAML, and the not-initialized error.
func TestLoadConfig(t *testing.T) {
	tests := []struct {
		name string
		// initMhgo controls whether the _lyx/ marker dir is created;
		// LoadConfig rejects a base dir without it.
		initMhgo bool
		// yaml, when non-empty, is written to _lyx/worktree.yaml.
		yaml            string
		wantPrefix      string
		wantErrContains string
	}{
		{name: "DefaultsWhenYAMLAbsent", initMhgo: true, wantPrefix: ""},
		{name: "BranchPrefixFromYAML", initMhgo: true, yaml: "branch_prefix: hanf/\n", wantPrefix: "hanf/"},
		{name: "ErrorWhenNotInitialized", initMhgo: false, wantErrContains: `run "lyx init"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			baseDir := t.TempDir()

			// Create the _lyx/ marker (and optional config) only for the
			// initialized scenarios; the error case leaves it absent so
			// LoadConfig takes the not-initialized branch.
			if tt.initMhgo {
				lyxDir := filepath.Join(baseDir, "_lyx")
				if err := os.Mkdir(lyxDir, 0755); err != nil {
					t.Fatalf("create _lyx: %v", err)
				}
				if tt.yaml != "" {
					yamlPath := filepath.Join(lyxDir, "worktree.yaml")
					if err := os.WriteFile(yamlPath, []byte(tt.yaml), 0644); err != nil {
						t.Fatalf("write worktree.yaml: %v", err)
					}
				}
			}

			cfg, err := worktree.LoadConfig(baseDir, "worktree")

			if tt.wantErrContains != "" {
				if err == nil {
					t.Fatalf("LoadConfig() error = nil; want error containing %q", tt.wantErrContains)
				}
				if !strings.Contains(err.Error(), tt.wantErrContains) {
					t.Errorf("LoadConfig() error = %q; want substring %q", err.Error(), tt.wantErrContains)
				}
				return
			}

			if err != nil {
				t.Fatalf("LoadConfig() error = %v; want nil", err)
			}
			if cfg.BranchPrefix != tt.wantPrefix {
				t.Errorf("LoadConfig().BranchPrefix = %q; want %q", cfg.BranchPrefix, tt.wantPrefix)
			}
		})
	}
}
