// config_test.go — tier0 unit tests for landingshed.LoadConfig.
//
// Seeds a bare t.TempDir() with just a _lym/config/landing.yaml file (no real hub, no SeedConfig,
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
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
	"gopkg.in/yaml.v3"
)

// seedLandingConfig creates <baseDir>/_lym/config/landing.yaml with the given contents.
func seedLandingConfig(t *testing.T, baseDir, contents string) {
	t.Helper()
	configDir := filepath.Join(baseDir, "_lym", "config")
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
	tiergate.Require(t, 0)
	t.Parallel()

	baseDir := t.TempDir()
	seedLandingConfig(t, baseDir, ConfigTemplate())

	cfg, err := LoadConfig(configengine.Dirs{Board: baseDir}, "landing")
	if err != nil {
		t.Fatalf("LoadConfig(%q, \"landing\") = _, %v; want nil error", baseDir, err)
	}
	if len(cfg.DeployBranches) != 1 || cfg.DeployBranches[0] != "main" {
		t.Errorf("cfg.DeployBranches = %v; want [\"main\"]", cfg.DeployBranches)
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
	if cfg.PRReviewer != "opus[high]" {
		t.Errorf("cfg.PRReviewer = %q; want %q", cfg.PRReviewer, "opus[high]")
	}
	if !cfg.Credit {
		t.Error("cfg.Credit = false; want true")
	}
	if cfg.CreditHeading != "Created using" {
		t.Errorf("cfg.CreditHeading = %q; want %q", cfg.CreditHeading, "Created using")
	}
	if want := "LoomYard https://github.com/Knatte18/loomyard"; cfg.CreditLoomYard != want {
		t.Errorf("cfg.CreditLoomYard = %q; want %q", cfg.CreditLoomYard, want)
	}
	if want := map[string]string{"claude": "Claude <noreply@anthropic.com>"}; !maps.Equal(cfg.CreditEngines, want) {
		t.Errorf("cfg.CreditEngines = %v; want %v", cfg.CreditEngines, want)
	}
	if cfg.Deploy != "" {
		t.Errorf("cfg.Deploy = %q; want empty", cfg.Deploy)
	}
	if cfg.ParentLockIdleReleaseMin != 10 {
		t.Errorf("cfg.ParentLockIdleReleaseMin = %d; want 10", cfg.ParentLockIdleReleaseMin)
	}
	if cfg.Fixer != "opus[high]" {
		t.Errorf("cfg.Fixer = %q; want %q", cfg.Fixer, "opus[high]")
	}
	if cfg.FixerTimeoutMin != 60 {
		t.Errorf("cfg.FixerTimeoutMin = %d; want 60", cfg.FixerTimeoutMin)
	}
	if cfg.ParentAlignFixAttempts != 2 {
		t.Errorf("cfg.ParentAlignFixAttempts = %d; want 2", cfg.ParentAlignFixAttempts)
	}
	if cfg.FinalizeFixAttempts != 2 {
		t.Errorf("cfg.FinalizeFixAttempts = %d; want 2", cfg.FinalizeFixAttempts)
	}
	if cfg.FinalizeFixMaxLines != 40 {
		t.Errorf("cfg.FinalizeFixMaxLines = %d; want 40", cfg.FinalizeFixMaxLines)
	}
	retiredKeys := []string{"publish_verify", "co_authored_by", "trailer_text", "engine_trailers"}
	for _, key := range retiredKeys {
		if strings.Contains(ConfigTemplate(), key) {
			t.Errorf("ConfigTemplate() holds %s; want the retired key gone from the template", key)
		}
	}

	retiredDir := t.TempDir()
	seedLandingConfig(t, retiredDir, ConfigTemplate()+"publish_verify: go test -tags tmux ./...\nco_authored_by: Someone <s@x>\ntrailer_text: Fixed\nengine_trailers:\n  claude: \"Co-Authored-By: Someone <s@x>\"\n")
	retired, err := LoadConfig(configengine.Dirs{Board: retiredDir}, "landing")
	if err != nil {
		t.Fatalf("LoadConfig() over a file holding the retired keys %v = _, %v; want nil error", retiredKeys, err)
	}
	if retired.Conflict != cfg.Conflict {
		t.Errorf("retired-key file cfg.Conflict = %q; want %q", retired.Conflict, cfg.Conflict)
	}
	if !maps.Equal(retired.CreditEngines, cfg.CreditEngines) {
		t.Errorf("retired-key file cfg.CreditEngines = %v; want the template's %v", retired.CreditEngines, cfg.CreditEngines)
	}
}

// TestConfig_IdleRelease verifies the idle bound is the configured minutes, and that a value below 1 reads as the default.
func TestConfig_IdleRelease(t *testing.T) {
	tiergate.Require(t, 0)
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

// TestConfig_CreditBlock verifies the credit block is the heading, then the LoomYard item, then the item of each engine that ran in name order, each once, and drops an empty or whitespace-only text, an engine with no entry, and the whole block when credit is off or no item remains.
func TestConfig_CreditBlock(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	engines := map[string]string{"alpha": "Alpha <a@x>\n", "beta": "Beta <b@x>", "blank": "  "}
	full := Config{Credit: true, CreditHeading: "Created using\n", CreditLoomYard: "LoomYard https://x/loomyard", CreditEngines: engines}
	withoutLoomYard := full
	withoutLoomYard.CreditLoomYard = " "
	withoutHeading := full
	withoutHeading.CreditHeading = ""
	off := full
	off.Credit = false
	tests := []struct {
		name       string
		cfg        Config
		enginesRun []string
		want       string
	}{
		{"HeadingLoomYardThenEnginesInNameOrder", full, []string{"beta", "alpha"}, "Created using\n - LoomYard https://x/loomyard\n - Alpha <a@x>\n - Beta <b@x>"},
		{"EngineThatDidNotRunAddsNothing", full, []string{"beta"}, "Created using\n - LoomYard https://x/loomyard\n - Beta <b@x>"},
		{"EngineWithoutEntryOrWithBlankTextAddsNothing", full, []string{"gamma", "blank"}, "Created using\n - LoomYard https://x/loomyard"},
		{"EngineNamedTwiceAddsOneLine", full, []string{"alpha", "alpha"}, "Created using\n - LoomYard https://x/loomyard\n - Alpha <a@x>"},
		{"NoEngineRanKeepsLoomYard", full, nil, "Created using\n - LoomYard https://x/loomyard"},
		{"EmptyLoomYardAddsNoLine", withoutLoomYard, []string{"alpha"}, "Created using\n - Alpha <a@x>"},
		{"EmptyHeadingLeavesItemsAlone", withoutHeading, []string{"alpha"}, " - LoomYard https://x/loomyard\n - Alpha <a@x>"},
		{"NoItemLineIsEmpty", withoutLoomYard, nil, ""},
		{"CreditOffIsEmpty", off, []string{"alpha"}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.cfg.CreditBlock(tt.enginesRun); got != tt.want {
				t.Errorf("CreditBlock(%v) = %q; want %q", tt.enginesRun, got, tt.want)
			}
		})
	}
}

// TestMigrateConfig verifies a retired require_pr_to_base moves to deploy_branches and a retired merge_fix_attempts to parent_align_fix_attempts keeping their values, the retired co-author and trailer keys are left for reconcile's prune with no value carried into credit_engines, and a migrated document passes unchanged.
func TestMigrateConfig(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name      string
		existing  string
		wantLines int
		// wantDeploy is deploy_branches as fmt.Sprint renders the decoded value; empty skips the check.
		wantDeploy string
		// wantAttempts is parent_align_fix_attempts as fmt.Sprint renders the decoded value; empty skips the check.
		wantAttempts string
	}{
		{"RetiredCoAuthorAndTrailerKeysLeftToReconcilesPrune", "squash: true\nco_authored_by: Someone <s@x>\ntrailer_text: Fixed\nengine_trailers:\n  claude: \"Co-Authored-By: Kept <k@x>\"\n", 0, "", ""},
		{"RetiredPublishVerifyLeftToReconcilesPrune", "publish_verify: go test -tags tmux ./...\n", 0, "", ""},
		{"RequirePRToBaseListMovesToDeployBranches", "require_pr_to_base: [main, develop]\nsquash: true\n", 1, "[main develop]", ""},
		{"RequirePRToBaseEmptyListStaysEmpty", "require_pr_to_base: []\nsquash: true\n", 1, "[]", ""},
		{"ExistingDeployBranchesWins", "require_pr_to_base: [old]\ndeploy_branches: [main]\n", 1, "[main]", ""},
		{"CoAuthoredByLeftAndRequirePRToBaseMoves", "co_authored_by: Someone <s@x>\nrequire_pr_to_base: [main]\n", 1, "[main]", ""},
		{"MergeFixAttemptsMovesToParentAlignFixAttempts", "merge_fix_attempts: 5\nsquash: true\n", 1, "", "5"},
		{"ExistingParentAlignFixAttemptsWins", "merge_fix_attempts: 5\nparent_align_fix_attempts: 3\n", 1, "", "3"},
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
			if tt.wantLines == 0 && string(migrated) != tt.existing {
				t.Errorf("MigrateConfig() = %q; want the document unchanged", migrated)
			}
			var got map[string]any
			if err := yaml.Unmarshal(migrated, &got); err != nil {
				t.Fatalf("unmarshal migrated document: %v", err)
			}
			if _, ok := got[creditEnginesKey]; ok {
				t.Errorf("migrated document %q holds %s; want no value carried into it", migrated, creditEnginesKey)
			}
			if _, ok := got["require_pr_to_base"]; ok {
				t.Errorf("migrated document %q still holds require_pr_to_base", migrated)
			}
			if _, ok := got["merge_fix_attempts"]; ok {
				t.Errorf("migrated document %q still holds merge_fix_attempts", migrated)
			}
			if tt.wantAttempts != "" {
				if gotAttempts := fmt.Sprint(got["parent_align_fix_attempts"]); gotAttempts != tt.wantAttempts {
					t.Errorf("migrated parent_align_fix_attempts = %s; want %s", gotAttempts, tt.wantAttempts)
				}
			}
			if tt.wantDeploy != "" {
				if gotDeploy := fmt.Sprint(got["deploy_branches"]); gotDeploy != tt.wantDeploy {
					t.Errorf("migrated deploy_branches = %s; want %s", gotDeploy, tt.wantDeploy)
				}
			}
			again, againLines, err := MigrateConfig(migrated)
			if err != nil || len(againLines) != 0 || string(again) != string(migrated) {
				t.Errorf("second MigrateConfig() = %q, %q, %v; want the document unchanged with no lines", again, againLines, err)
			}
		})
	}
	t.Run("MergeFixAttemptsRenameLine", func(t *testing.T) {
		t.Parallel()
		_, lines, err := MigrateConfig([]byte("merge_fix_attempts: 5\n"))
		const want = "merge_fix_attempts: renamed to parent_align_fix_attempts"
		if err != nil || len(lines) != 1 || lines[0] != want {
			t.Errorf("MigrateConfig() lines = %q, %v; want [%q]", lines, err, want)
		}
	})
}

// TestMigrateConfig_InvalidDocument verifies an unparsable document and a non-mapping root return an error marked configengine.ErrInvalid.
func TestMigrateConfig_InvalidDocument(t *testing.T) {
	tiergate.Require(t, 0)
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

// TestLoadConfig_InvalidKeys verifies a hand-edited landing.yaml with an ungrammatical model-spec or a credit_engines key naming no engine fails loud at load time, naming the key, rather than being silently carried into the site that reads it.
func TestLoadConfig_InvalidKeys(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	const wellFormed = "deploy_branches: [\"main\"]\nsquash: true\nconflict: opus[high]\nconflict_timeout_min: 60\n" +
		"describe: sonnet[medium]\ndescribe_timeout_min: 30\npr_reviewer: opus[high]\nfixer: opus[high]\nfixer_timeout_min: 60\nparent_align_fix_attempts: 2\n" +
		"finalize_fix_attempts: 2\nfinalize_fix_max_lines: 40\nfinalize_fix_review_rounds: 3\ncredit_engines:\n  claude: \"Claude <noreply@anthropic.com>\"\n"
	cases := []struct {
		name, valid, invalid, key string
	}{
		{"malformed conflict", "conflict: opus[high]", "conflict: \"opus[high\"", "conflict"},
		{"malformed describe", "describe: sonnet[medium]", "describe: \"sonnet[medium\"", "describe"},
		{"malformed pr_reviewer", "pr_reviewer: opus[high]", "pr_reviewer: not-a-spec[", "pr_reviewer"},
		{"credit_engines key naming no engine", "  claude:", "  claud:", "credit_engines.claud"},
		{"malformed fixer", "fixer: opus[high]", "fixer: \"opus[high\"", "fixer"},
		{"fixer timeout below 1", "fixer_timeout_min: 60", "fixer_timeout_min: 0", "fixer_timeout_min"},
		{"parent align fix attempts below 1", "parent_align_fix_attempts: 2", "parent_align_fix_attempts: 0", "parent_align_fix_attempts"},
		{"finalize fix attempts below 1", "finalize_fix_attempts: 2", "finalize_fix_attempts: 0", "finalize_fix_attempts"},
		{"finalize fix max lines below 1", "finalize_fix_max_lines: 40", "finalize_fix_max_lines: 0", "finalize_fix_max_lines"},
		{"finalize fix review rounds below 1", "finalize_fix_review_rounds: 3", "finalize_fix_review_rounds: 0", "finalize_fix_review_rounds"},
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
// anchor gets an empty _lym/config/ so the layered load sees an initialised worktree.
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
	tiergate.Require(t, 0)
	t.Parallel()

	baseDir := t.TempDir()

	_, err := LoadConfig(configengine.Dirs{Board: baseDir}, "landing")
	if err == nil {
		t.Fatal("LoadConfig() = _, nil; want non-nil error for uninitialized baseDir")
	}
	want := `not initialized here; run "lym fabric reconcile"; a session lym refuses the verb from reports status: FAILED and the orch runs it`
	if err.Error() != want {
		t.Errorf("LoadConfig() error = %q; want %q", err.Error(), want)
	}
}

// TestLoadConfig_OptionalStringKeys verifies each optional string landing key loads as written, and that a landing.yaml without it loads the template's empty value.
func TestLoadConfig_OptionalStringKeys(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	const withoutKeys = "deploy_branches: [\"main\"]\nsquash: true\nconflict: opus[high]\nconflict_timeout_min: 60\n" +
		"describe: sonnet[medium]\ndescribe_timeout_min: 30\n"
	cases := []struct {
		name, contents, want string
		field                func(Config) string
	}{
		{"credit_heading set", withoutKeys + "credit_heading: Made with\n", "Made with", func(c Config) string { return c.CreditHeading }},
		{"credit_engines empty names no engine", withoutKeys + "credit_engines: {}\n", "map[]", func(c Config) string { return fmt.Sprint(c.CreditEngines) }},
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
