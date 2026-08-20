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
	if cfg.Conflict != "opus[effort=high]" {
		t.Errorf("cfg.Conflict = %q; want %q", cfg.Conflict, "opus[effort=high]")
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
conflict: "opus[effort"
conflict_timeout_min: 60
`)

	_, err := LoadConfig(baseDir, "landing")
	if err == nil {
		t.Fatal("LoadConfig() = _, nil; want non-nil error for malformed conflict spec")
	}
	if !strings.Contains(err.Error(), "conflict") {
		t.Errorf("LoadConfig() error = %q; want it to name the %q key", err.Error(), "conflict")
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
