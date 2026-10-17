// config_test.go — untagged Tier-1 unit tests for landingshed.LoadConfig.
//
// Seeds a bare t.TempDir() with just a _lyx/config/landing.yaml file (no real hub, no SeedConfig,
// no git spawn), modelled on internal/loomengine's own config_test.go.

package landingshed

import (
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/configengine"
	"gopkg.in/yaml.v3"
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
	if cfg.TrailerText != "" {
		t.Errorf("cfg.TrailerText = %q; want empty", cfg.TrailerText)
	}
	if want := map[string]string{"claude": "Co-Authored-By: Claude <noreply@anthropic.com>"}; !maps.Equal(cfg.EngineTrailers, want) {
		t.Errorf("cfg.EngineTrailers = %v; want %v", cfg.EngineTrailers, want)
	}
	if cfg.Deploy != "" {
		t.Errorf("cfg.Deploy = %q; want empty", cfg.Deploy)
	}
	if cfg.ParentLockIdleReleaseMin != 10 {
		t.Errorf("cfg.ParentLockIdleReleaseMin = %d; want 10", cfg.ParentLockIdleReleaseMin)
	}
}

// TestConfig_IdleRelease verifies the idle bound is the configured minutes, and that a value below 1 reads as the default.
func TestConfig_IdleRelease(t *testing.T) {
	t.Parallel()

	tests := []struct {
		minutes int
		want    time.Duration
	}{
		{10, 10 * time.Minute},
		{3, 3 * time.Minute},
		{0, 10 * time.Minute},
		{-1, 10 * time.Minute},
	}
	for _, tt := range tests {
		if got := (Config{ParentLockIdleReleaseMin: tt.minutes}).IdleRelease(); got != tt.want {
			t.Errorf("Config{ParentLockIdleReleaseMin: %d}.IdleRelease() = %v; want %v", tt.minutes, got, tt.want)
		}
	}
}

// TestConfig_TrailerBlock verifies the trailer block holds the fixed text, then the text of each engine that ran in name order, and drops an empty or whitespace-only text and an engine with no entry.
func TestConfig_TrailerBlock(t *testing.T) {
	t.Parallel()

	engineTrailers := map[string]string{"alpha": "Co-Authored-By: Alpha <a@x>\n", "beta": "Co-Authored-By: Beta <b@x>", "blank": "  "}
	tests := []struct {
		name       string
		cfg        Config
		enginesRun []string
		want       string
	}{
		{"FixedThenEnginesInNameOrder", Config{TrailerText: "Created using Loomyard\nSecond line\n", EngineTrailers: engineTrailers}, []string{"beta", "alpha"}, "Created using Loomyard\nSecond line\nCo-Authored-By: Alpha <a@x>\nCo-Authored-By: Beta <b@x>"},
		{"EngineThatDidNotRunAddsNothing", Config{EngineTrailers: engineTrailers}, []string{"beta"}, "Co-Authored-By: Beta <b@x>"},
		{"EngineWithoutEntryOrWithBlankTextAddsNothing", Config{TrailerText: "Fixed", EngineTrailers: engineTrailers}, []string{"gamma", "blank"}, "Fixed"},
		{"EngineNamedTwiceAddsOneLine", Config{EngineTrailers: engineTrailers}, []string{"alpha", "alpha"}, "Co-Authored-By: Alpha <a@x>"},
		{"BothEmpty", Config{TrailerText: " \n"}, []string{"alpha"}, ""},
		{"NoEngineRan", Config{EngineTrailers: engineTrailers}, nil, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.cfg.TrailerBlock(tt.enginesRun); got != tt.want {
				t.Errorf("TrailerBlock(%v) = %q; want %q", tt.enginesRun, got, tt.want)
			}
		})
	}
}

// TestMigrateConfig verifies a retired co_authored_by moves into engine_trailers as the claude line, an engine_trailers already set wins over it, and a migrated document passes unchanged.
func TestMigrateConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		existing     string
		wantTrailers map[string]string
		wantLines    int
	}{
		{"MovesToClaudeLine", "squash: true\nco_authored_by: Someone <s@x>\n", map[string]string{"claude": "Co-Authored-By: Someone <s@x>"}, 1},
		{"ExistingEngineTrailersWins", "co_authored_by: Someone <s@x>\nengine_trailers: {}\n", map[string]string{}, 1},
		{"ExistingEntriesKept", "co_authored_by: Someone <s@x>\nengine_trailers:\n  claude: \"Co-Authored-By: Kept <k@x>\"\n", map[string]string{"claude": "Co-Authored-By: Kept <k@x>"}, 1},
		{"EmptyValueDroppedWithoutEntry", "squash: true\nco_authored_by: \"  \"\n", nil, 1},
		{"NonScalarValueDroppedWithoutEntry", "co_authored_by: [a, b]\n", nil, 1},
		{"NoRetiredKeyUnchanged", "engine_trailers:\n  claude: \"Co-Authored-By: Kept <k@x>\"\n", map[string]string{"claude": "Co-Authored-By: Kept <k@x>"}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			migrated, lines, err := MigrateConfig([]byte(tt.existing))
			if err != nil {
				t.Fatalf("MigrateConfig() error = %v; want nil", err)
			}
			if len(lines) != tt.wantLines {
				t.Errorf("MigrateConfig() lines = %q; want %d line(s)", lines, tt.wantLines)
			}
			var got map[string]any
			if err := yaml.Unmarshal(migrated, &got); err != nil {
				t.Fatalf("unmarshal migrated document: %v", err)
			}
			if _, ok := got["co_authored_by"]; ok {
				t.Errorf("migrated document %q still holds co_authored_by", migrated)
			}
			var trailers struct {
				EngineTrailers map[string]string `yaml:"engine_trailers"`
			}
			if err := yaml.Unmarshal(migrated, &trailers); err != nil {
				t.Fatalf("unmarshal migrated engine_trailers: %v", err)
			}
			if !maps.Equal(trailers.EngineTrailers, tt.wantTrailers) {
				t.Errorf("migrated engine_trailers = %v; want %v", trailers.EngineTrailers, tt.wantTrailers)
			}
			again, againLines, err := MigrateConfig(migrated)
			if err != nil || len(againLines) != 0 || string(again) != string(migrated) {
				t.Errorf("second MigrateConfig() = %q, %q, %v; want the document unchanged with no lines", again, againLines, err)
			}
		})
	}
}

// TestMigrateConfig_InvalidDocument verifies an unparsable document and a non-mapping root return an error marked configengine.ErrInvalid.
func TestMigrateConfig_InvalidDocument(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		existing string
	}{
		{"Unparsable", "co_authored_by: [unclosed\n"},
		{"NonMappingRoot", "- co_authored_by\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if _, _, err := MigrateConfig([]byte(tt.existing)); !errors.Is(err, configengine.ErrInvalid) {
				t.Errorf("MigrateConfig(%q) error = %v; want one marked configengine.ErrInvalid", tt.existing, err)
			}
		})
	}
}

// TestLoadConfig_InvalidKeys verifies a hand-edited landing.yaml with an ungrammatical model-spec or an engine_trailers key naming no engine fails loud at load time, naming the key, rather than being silently carried into the site that reads it.
func TestLoadConfig_InvalidKeys(t *testing.T) {
	t.Parallel()

	const wellFormed = "require_pr_to_base: [\"main\"]\nsquash: true\nconflict: opus[high]\nconflict_timeout_min: 60\n" +
		"describe: sonnet[medium]\ndescribe_timeout_min: 30\nengine_trailers:\n  claude: \"Co-Authored-By: Claude <noreply@anthropic.com>\"\n"
	cases := []struct {
		name, valid, invalid, key string
	}{
		{"malformed conflict", "conflict: opus[high]", "conflict: \"opus[high\"", "conflict"},
		{"malformed describe", "describe: sonnet[medium]", "describe: \"sonnet[medium\"", "describe"},
		{"engine_trailers key naming no engine", "  claude:", "  claud:", "engine_trailers.claud"},
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

// TestLoadConfig_OptionalStringKeys verifies each optional string landing key loads as written, and that a landing.yaml without it loads the template's empty value.
func TestLoadConfig_OptionalStringKeys(t *testing.T) {
	t.Parallel()

	const withoutKeys = "require_pr_to_base: [\"main\"]\nsquash: true\nconflict: opus[high]\nconflict_timeout_min: 60\n" +
		"describe: sonnet[medium]\ndescribe_timeout_min: 30\n"
	cases := []struct {
		name, contents, want string
		field                func(Config) string
	}{
		{"trailer_text set", withoutKeys + "trailer_text: |\n  Created using Loomyard\n  Second line\n", "Created using Loomyard\nSecond line\n", func(c Config) string { return c.TrailerText }},
		{"trailer_text absent", withoutKeys, "", func(c Config) string { return c.TrailerText }},
		{"engine_trailers empty names no engine", withoutKeys + "engine_trailers: {}\n", "map[]", func(c Config) string { return fmt.Sprint(c.EngineTrailers) }},
		{"publish_verify set", withoutKeys + "publish_verify: go test -tags tmux ./...\n", "go test -tags tmux ./...", func(c Config) string { return c.PublishVerify }},
		{"publish_verify absent", withoutKeys, "", func(c Config) string { return c.PublishVerify }},
		{"deploy set", withoutKeys + "deploy: ./update-plugins.sh\n", "./update-plugins.sh", func(c Config) string { return c.Deploy }},
		{"deploy absent", withoutKeys, "", func(c Config) string { return c.Deploy }},
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
			if got := tc.field(cfg); got != tc.want {
				t.Errorf("%s = %q; want %q", tc.name, got, tc.want)
			}
		})
	}
}
