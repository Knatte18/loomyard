// config_test.go — untagged Tier-1 unit tests for landingshed.LoadConfig.
//
// Seeds a bare t.TempDir() with just a _lyx/config/landing.yaml file (no real hub, no SeedConfig,
// no git spawn), modelled on internal/loomengine's own config_test.go.

package landingshed

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
)

// seedLandingConfig creates <baseDir>/_lyx/config/landing.yaml with the given contents.
func seedLandingConfig(t *testing.T, baseDir, contents string) {
	t.Helper()
	configDir := filepath.Join(baseDir, "_lyx", "config")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) = %v; want nil", configDir, err)
	}
	cfgPath := filepath.Join(configDir, "landing.yaml")
	if err := os.WriteFile(cfgPath, []byte(contents), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) = %v; want nil", cfgPath, err)
	}
}

// TestLoadConfig_TemplateDefaults verifies the template's default values round-trip, including the single-entry base-branch list, the squash default and the three Describe-related keys.
func TestLoadConfig_TemplateDefaults(t *testing.T) {
	t.Parallel()

	baseDir := t.TempDir()
	seedLandingConfig(t, baseDir, ConfigTemplate())

	cfg, err := LoadConfig(configengine.Dirs{Board: baseDir}, "landing")
	if err != nil {
		t.Fatalf("LoadConfig(%q, \"landing\") = _, %v; want nil error", baseDir, err)
	}
	if len(cfg.RequirePRToBase) != 1 || cfg.RequirePRToBase[0] != "main" {
		t.Errorf("cfg.RequirePRToBase = %v; want [\"main\"]", cfg.RequirePRToBase)
	}
	if !cfg.Squash {
		t.Error("cfg.Squash = false; want true")
	}
	if cfg.Conflict != "opus[high]" {
		t.Errorf("cfg.Conflict = %q; want %q", cfg.Conflict, "opus[high]")
	}
	if cfg.ConflictTimeoutMin != 60 {
		t.Errorf("cfg.ConflictTimeoutMin = %d; want %d", cfg.ConflictTimeoutMin, 60)
	}
	if cfg.Describe != "sonnet[medium]" {
		t.Errorf("cfg.Describe = %q; want %q", cfg.Describe, "sonnet[medium]")
	}
	if cfg.DescribeTimeoutMin != 30 {
		t.Errorf("cfg.DescribeTimeoutMin = %d; want 30", cfg.DescribeTimeoutMin)
	}
	if cfg.CoAuthoredBy != "Claude <noreply@anthropic.com>" {
		t.Errorf("cfg.CoAuthoredBy = %q; want %q", cfg.CoAuthoredBy, "Claude <noreply@anthropic.com>")
	}
}

// TestLoadConfig_InvalidKeys verifies a hand-edited landing.yaml with an ungrammatical model-spec or an empty co_authored_by fails loud at load time, naming the key, rather than being silently carried into the spawn site that reads it.
func TestLoadConfig_InvalidKeys(t *testing.T) {
	t.Parallel()

	const wellFormed = "require_pr_to_base: [\"main\"]\nsquash: true\nconflict: opus[high]\nconflict_timeout_min: 60\n" +
		"describe: sonnet[medium]\ndescribe_timeout_min: 30\nco_authored_by: Claude <noreply@anthropic.com>\n"
	cases := []struct {
		name, valid, invalid, key string
	}{
		{"malformed conflict", "conflict: opus[high]", "conflict: \"opus[high\"", "conflict"},
		{"malformed describe", "describe: sonnet[medium]", "describe: \"sonnet[medium\"", "describe"},
		{"empty co_authored_by", "co_authored_by: Claude <noreply@anthropic.com>", "co_authored_by: \"  \"", "co_authored_by"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			baseDir := t.TempDir()
			seedLandingConfig(t, baseDir, strings.Replace(wellFormed, tc.valid, tc.invalid, 1))
			_, err := LoadConfig(configengine.Dirs{Board: baseDir}, "landing")
			if err == nil {
				t.Fatal("LoadConfig() = _, nil; want non-nil error")
			}
			if !strings.Contains(err.Error(), tc.key) {
				t.Errorf("LoadConfig() error = %q; want it to name %q", err.Error(), tc.key)
			}
		})
	}

	t.Run("a local conflict wins over the hub file's", func(t *testing.T) {
		t.Parallel()

		board, anchor := t.TempDir(), t.TempDir()
		seedLandingConfig(t, board, ConfigTemplate())
		writeLocalLandingConfig(t, anchor, "conflict: sonnet[high]\n")

		cfg, err := LoadConfig(configengine.Dirs{Anchor: anchor, Board: board}, "landing")
		if err != nil {
			t.Fatalf("LoadConfig() = _, %v; want nil error", err)
		}
		if cfg.Conflict != "sonnet[high]" {
			t.Errorf("cfg.Conflict = %q; want %q", cfg.Conflict, "sonnet[high]")
		}
	})

	t.Run("a local publish_verify is refused naming the local file and the key", func(t *testing.T) {
		t.Parallel()

		board, anchor := t.TempDir(), t.TempDir()
		seedLandingConfig(t, board, ConfigTemplate())
		localPath := writeLocalLandingConfig(t, anchor, "publish_verify: rm -rf .\n")

		_, err := LoadConfig(configengine.Dirs{Anchor: anchor, Board: board}, "landing")
		if !errors.Is(err, configengine.ErrInvalid) {
			t.Fatalf("LoadConfig() error = %v; want ErrInvalid", err)
		}
		for _, want := range []string{localPath, "publish_verify"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("LoadConfig() error = %q; want it to contain %q", err.Error(), want)
			}
		}
	})
}

// writeLocalLandingConfig writes contents as anchor's local landing override and returns its path.
// anchor gets an empty _lyx/config/ so the layered load sees an initialised worktree.
func writeLocalLandingConfig(t *testing.T, anchor, contents string) string {
	t.Helper()
	for _, dir := range []string{configengine.ConfigDir(anchor), configengine.ScratchDir(anchor)} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("MkdirAll(%q) = %v; want nil", dir, err)
		}
	}
	localPath := configengine.LocalFile(anchor, "landing")
	if err := os.WriteFile(localPath, []byte(contents), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) = %v; want nil", localPath, err)
	}
	return localPath
}

// TestLoadConfig_NotInitialized verifies uninitialized baseDir yields recovery hint -- an absent
// config file is an error rather than a degrading default, which is the whole point of adopting the
// strict loader.
func TestLoadConfig_NotInitialized(t *testing.T) {
	t.Parallel()

	baseDir := t.TempDir()

	_, err := LoadConfig(configengine.Dirs{Board: baseDir}, "landing")
	if err == nil {
		t.Fatal("LoadConfig() = _, nil; want non-nil error for uninitialized baseDir")
	}
	want := `not initialized here; run "lyx fabric reconcile"; a session lyx refuses the verb from reports status: FAILED and the orch runs it`
	if err.Error() != want {
		t.Errorf("LoadConfig() error = %q; want %q", err.Error(), want)
	}
}

// TestLoadConfig_PublishVerify verifies the publish_verify key loads as written, and that a landing.yaml without it loads the template's empty value.
func TestLoadConfig_PublishVerify(t *testing.T) {
	t.Parallel()

	const withoutKey = "require_pr_to_base: [\"main\"]\nsquash: true\nconflict: opus[high]\nconflict_timeout_min: 60\n" +
		"describe: sonnet[medium]\ndescribe_timeout_min: 30\nco_authored_by: Claude <noreply@anthropic.com>\n"
	cases := []struct {
		name, contents, want string
	}{
		{"key set", withoutKey + "publish_verify: go test -tags tmux ./...\n", "go test -tags tmux ./..."},
		{"key absent", withoutKey, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			baseDir := t.TempDir()
			seedLandingConfig(t, baseDir, tc.contents)
			cfg, err := LoadConfig(configengine.Dirs{Board: baseDir}, "landing")
			if err != nil {
				t.Fatalf("LoadConfig() = _, %v; want nil error", err)
			}
			if cfg.PublishVerify != tc.want {
				t.Errorf("cfg.PublishVerify = %q; want %q", cfg.PublishVerify, tc.want)
			}
		})
	}
}
