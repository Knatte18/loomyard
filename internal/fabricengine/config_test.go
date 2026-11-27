// config_test.go — unit tests for fabricengine.LoadConfig.
//
// Covers: happy-path with template keys present, branch_prefix/pathspec parsing, and environment variable resolution;
// the not-initialized error path; the secret-file matcher a hub's secret_paths yields; and the push timing.

package fabricengine_test

import (
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestLoadConfig covers LoadConfig over a hand-written fabric config:
// a valid config with all template keys parses both fields and Dirs();
// an empty branch_prefix stays the empty string;
// an environment variable in branch_prefix is resolved;
// a missing `_lym/` returns the fabric-specific not-initialized error, naming the reconcile verb that fixes it;
// a machine override's branch_prefix wins over the hub file's;
// and a local pathspec or warp exclude list is refused.
//
//testtiming:keep LoadConfig parsing both fields and Dirs(), keeping an empty prefix, resolving env references and naming the reconcile verb when uninitialized; coverage of its blocks by other tests does not show an assertion of this
func TestLoadConfig(t *testing.T) {
	tiergate.Require(t, 0)
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
			content:      "branch_prefix: hanf/\npathspec: _lym _extra\n",
			wantBranch:   "hanf/",
			wantPathspec: "_lym _extra",
			wantDirs:     []string{"_lym", "_extra"},
		},
		{
			name:         "empty_branch_prefix",
			content:      "branch_prefix: \"\"\npathspec: _lym\n",
			wantBranch:   "",
			wantPathspec: "_lym",
			wantDirs:     []string{"_lym"},
		},
		{
			name:         "env_resolution",
			env:          map[string]string{"TEST_FABRIC_BRANCH_PREFIX": "feature/"},
			content:      "branch_prefix: ${env:TEST_FABRIC_BRANCH_PREFIX}\npathspec: _lym\n",
			wantBranch:   "feature/",
			wantPathspec: "_lym",
			wantDirs:     []string{"_lym"},
		},
		{
			name:           "not_initialized",
			wantErrContain: []string{"not initialized", "lym fabric reconcile", "a session lym refuses the verb from reports status: FAILED and the orch runs it"},
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
					t.Fatalf("failed to create _lym/config: %v", err)
				}
				if err := os.WriteFile(configengine.ConfigFile(tmpDir, "fabric"), []byte(tt.content), 0644); err != nil {
					t.Fatalf("failed to write config: %v", err)
				}
			}

			cfg, err := fabricengine.LoadConfig(configengine.Dirs{Board: tmpDir})
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

	t.Run("a machine override's branch_prefix wins over the hub file's", func(t *testing.T) {
		board, anchor := t.TempDir(), t.TempDir()
		for _, dir := range []string{configengine.ConfigDir(board), configengine.ScratchDir(board)} {
			if err := os.MkdirAll(dir, 0755); err != nil {
				t.Fatalf("failed to create %s: %v", dir, err)
			}
		}
		if err := os.WriteFile(configengine.ConfigFile(board, "fabric"), []byte("branch_prefix: hub/\npathspec: _lym\n"), 0644); err != nil {
			t.Fatalf("failed to write config: %v", err)
		}
		if err := os.WriteFile(configengine.MachineFile(board, "fabric"), []byte("branch_prefix: machine/\n"), 0644); err != nil {
			t.Fatalf("failed to write machine config: %v", err)
		}

		cfg, err := fabricengine.LoadConfig(configengine.Dirs{Anchor: anchor, Board: board})
		if err != nil {
			t.Fatalf("LoadConfig() error = %v; want the machine override read", err)
		}
		if cfg.BranchPrefix != "machine/" || cfg.Pathspec != "_lym" {
			t.Errorf("LoadConfig() = {BranchPrefix: %q, Pathspec: %q}; want {\"machine/\", \"_lym\"}", cfg.BranchPrefix, cfg.Pathspec)
		}
	})

	for _, tt := range []struct{ key, line string }{
		{"pathspec", "pathspec: docs\n"},
		{"warp_exclude", "warp_exclude: x\n"},
		{"secret_paths", "secret_paths: x\n"},
	} {
		t.Run("a local "+tt.key+" is refused naming the local file and the key", func(t *testing.T) {
			board, anchor := t.TempDir(), t.TempDir()
			for _, dir := range []string{configengine.ConfigDir(board), configengine.ConfigDir(anchor), configengine.ScratchDir(anchor)} {
				if err := os.MkdirAll(dir, 0755); err != nil {
					t.Fatalf("failed to create %s: %v", dir, err)
				}
			}
			if err := os.WriteFile(configengine.ConfigFile(board, "fabric"), []byte(fabricengine.ConfigTemplate()), 0644); err != nil {
				t.Fatalf("failed to write config: %v", err)
			}
			localPath := configengine.LocalFile(anchor, "fabric")
			if err := os.WriteFile(localPath, []byte(tt.line), 0644); err != nil {
				t.Fatalf("failed to write local config: %v", err)
			}

			_, err := fabricengine.LoadConfig(configengine.Dirs{Anchor: anchor, Board: board})
			if !errors.Is(err, configengine.ErrInvalid) {
				t.Fatalf("LoadConfig() error = %v; want ErrInvalid", err)
			}
			for _, want := range []string{localPath, tt.key} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("LoadConfig() error = %q; want it to contain %q", err.Error(), want)
				}
			}
		})
	}
}

// TestConfig_PushTiming covers the push timing a fabric.yaml yields:
// the shipped template's defaults, a value below one second floored,
// and a ceiling below the interval raised to it, each raised value with one warning line naming its key.
func TestConfig_PushTiming(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		name    string
		old     string
		new     string
		want    fabricengine.PushTiming
		wantKey string
	}{
		{
			name: "TemplateDefaults",
			want: fabricengine.PushTiming{Interval: 60 * time.Second, Ceiling: 300 * time.Second, NoticeAfter: 30 * time.Minute},
		},
		{
			name:    "IntervalBelowFloor",
			old:     "push_interval_s: 60",
			new:     "push_interval_s: 0",
			want:    fabricengine.PushTiming{Interval: time.Second, Ceiling: 300 * time.Second, NoticeAfter: 30 * time.Minute},
			wantKey: "fabric.push_interval_s",
		},
		{
			name:    "CeilingBelowInterval",
			old:     "push_ceiling_s: 300",
			new:     "push_ceiling_s: 10",
			want:    fabricengine.PushTiming{Interval: 60 * time.Second, Ceiling: 60 * time.Second, NoticeAfter: 30 * time.Minute},
			wantKey: "fabric.push_ceiling_s",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			board := t.TempDir()
			if err := os.MkdirAll(configengine.ConfigDir(board), 0o755); err != nil {
				t.Fatalf("failed to create _lym/config: %v", err)
			}
			content := strings.Replace(fabricengine.ConfigTemplate(), tt.old, tt.new, 1)
			if err := os.WriteFile(configengine.ConfigFile(board, "fabric"), []byte(content), 0o644); err != nil {
				t.Fatalf("failed to write config: %v", err)
			}
			cfg, err := fabricengine.LoadConfig(configengine.Dirs{Board: board})
			if err != nil {
				t.Fatalf("LoadConfig() error = %v", err)
			}

			got, warnings := cfg.PushTiming()
			if got != tt.want {
				t.Errorf("PushTiming() = %+v; want %+v", got, tt.want)
			}
			if tt.wantKey == "" {
				if len(warnings) != 0 {
					t.Errorf("PushTiming() warnings = %q; want none", warnings)
				}
				return
			}
			if len(warnings) != 1 || !strings.HasPrefix(warnings[0], tt.wantKey+" ") {
				t.Errorf("PushTiming() warnings = %q; want one line naming %s", warnings, tt.wantKey)
			}
		})
	}
}

// TestLoadSecretPathMatcher covers the matcher a hub's fabric.yaml yields:
// each pattern secret_paths adds refuses its paths, the shipped patterns still apply beside them,
// and a malformed added pattern fails as invalid config naming the key.
func TestLoadSecretPathMatcher(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		name        string
		content     string
		path        string
		wantPattern string
		wantErr     bool
	}{
		{name: "AddedBaseNamePattern", content: "secret_paths: \"*.key vault/*\"\n", path: "tls/server.key", wantPattern: "*.key"},
		{name: "AddedSlashPattern", content: "secret_paths: \"*.key vault/*\"\n", path: "vault/token", wantPattern: "vault/*"},
		{name: "ShippedPatternStillApplies", content: "secret_paths: \"*.key\"\n", path: "config/.env", wantPattern: ".env"},
		{name: "EmptyKeyMatchesNoOrdinaryFile", content: "secret_paths: \"\"\n", path: "tls/server.key"},
		{name: "MalformedPattern", content: "secret_paths: \"[unclosed\"\n", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			board := t.TempDir()
			if err := os.MkdirAll(configengine.ConfigDir(board), 0o755); err != nil {
				t.Fatalf("failed to create _lym/config: %v", err)
			}
			if err := os.WriteFile(configengine.ConfigFile(board, "fabric"), []byte(tt.content), 0o644); err != nil {
				t.Fatalf("failed to write config: %v", err)
			}

			m, err := fabricengine.LoadSecretPathMatcher(configengine.Dirs{Board: board})
			if tt.wantErr {
				if !errors.Is(err, configengine.ErrInvalid) || !strings.Contains(err.Error(), fabricengine.SecretPathsKey) {
					t.Fatalf("LoadSecretPathMatcher() error = %v; want ErrInvalid naming %s", err, fabricengine.SecretPathsKey)
				}
				return
			}
			if err != nil {
				t.Fatalf("LoadSecretPathMatcher() error = %v", err)
			}
			pattern, matched := m.Match(tt.path)
			if pattern != tt.wantPattern || matched != (tt.wantPattern != "") {
				t.Errorf("Match(%q) = (%q, %v); want (%q, %v)", tt.path, pattern, matched, tt.wantPattern, tt.wantPattern != "")
			}
		})
	}
}
