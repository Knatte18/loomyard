// config_test.go — untagged Tier-1 unit tests for landingshed.LoadConfig.
//
// Seeds a bare t.TempDir() with just a _lyx/config/landing.yaml file (no real hub, no SeedConfig,
// no git spawn), modelled on internal/loomengine's own config_test.go.

package landingshed

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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

// TestLoadConfig_WellFormed verifies the template's default values round-trip, including the
// single-entry base-branch list and the squash default.
func TestLoadConfig_WellFormed(t *testing.T) {
	baseDir := t.TempDir()
	seedLandingConfig(t, baseDir, ConfigTemplate())

	cfg, err := LoadConfig(baseDir, "landing")
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
}

// TestLoadConfig_MalformedConflictSpec verifies a hand-edited landing.yaml with an ungrammatical
// conflict model-spec fails loud at load time, naming the "conflict" key, rather than being
// silently carried into the conflict-resolution session's spawn site.
func TestLoadConfig_MalformedConflictSpec(t *testing.T) {
	baseDir := t.TempDir()
	seedLandingConfig(t, baseDir, `require_pr_to_base: ["main"]
squash: true
conflict: "opus[high"
conflict_timeout_min: 60
describe: sonnet[medium]
describe_timeout_min: 30
co_authored_by: Claude <noreply@anthropic.com>
`)

	_, err := LoadConfig(baseDir, "landing")
	if err == nil {
		t.Fatal("LoadConfig() = _, nil; want non-nil error for malformed conflict spec")
	}
	if !strings.Contains(err.Error(), "conflict") {
		t.Errorf("LoadConfig() error = %q; want it to name the %q key", err.Error(), "conflict")
	}
}

// TestLoadConfig_DescribeDefaults verifies the template carries the three Describe-related keys'
// defaults.
func TestLoadConfig_DescribeDefaults(t *testing.T) {
	baseDir := t.TempDir()
	seedLandingConfig(t, baseDir, ConfigTemplate())

	cfg, err := LoadConfig(baseDir, "landing")
	if err != nil {
		t.Fatalf("LoadConfig() = _, %v; want nil error", err)
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

// TestLoadConfig_InvalidDescribeKeys verifies a malformed describe spec and an empty co_authored_by
// each fail at load, naming their key.
func TestLoadConfig_InvalidDescribeKeys(t *testing.T) {
	const head = "require_pr_to_base: [\"main\"]\nsquash: true\nconflict: opus[high]\nconflict_timeout_min: 60\n"
	cases := []struct {
		name, tail, key string
	}{
		{"malformed describe", "describe: \"sonnet[medium\"\ndescribe_timeout_min: 30\nco_authored_by: Claude <noreply@anthropic.com>\n", "describe"},
		{"empty co_authored_by", "describe: sonnet[medium]\ndescribe_timeout_min: 30\nco_authored_by: \"  \"\n", "co_authored_by"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			baseDir := t.TempDir()
			seedLandingConfig(t, baseDir, head+tc.tail)
			_, err := LoadConfig(baseDir, "landing")
			if err == nil {
				t.Fatal("LoadConfig() = _, nil; want non-nil error")
			}
			if !strings.Contains(err.Error(), tc.key) {
				t.Errorf("LoadConfig() error = %q; want it to name %q", err.Error(), tc.key)
			}
		})
	}
}

// TestLoadConfig_NotInitialized verifies uninitialized baseDir yields recovery hint -- an absent
// config file is an error rather than a degrading default, which is the whole point of adopting the
// strict loader.
func TestLoadConfig_NotInitialized(t *testing.T) {
	baseDir := t.TempDir()

	_, err := LoadConfig(baseDir, "landing")
	if err == nil {
		t.Fatal("LoadConfig() = _, nil; want non-nil error for uninitialized baseDir")
	}
	want := `not initialized here; run "lyx fabric reconcile"`
	if err.Error() != want {
		t.Errorf("LoadConfig() error = %q; want %q", err.Error(), want)
	}
}
