// config_test.go — untagged Tier-1 unit tests for loomengine.LoadConfig.
//
// Seeds a bare t.TempDir() with just a _lyx/config/loom.yaml file (no
// CopyWeft, no SeedConfig, no git spawn) since configengine.Load's
// env-source build tolerates a missing .env — a live weft fixture would be
// integration-tagged (like builderengine/config_test.go), which is out of
// scope for this package's plain "go test ./internal/loomengine/" run.

package loomengine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// seedLoomConfig creates <baseDir>/_lyx/config/loom.yaml with the given
// contents, mirroring the on-disk layout hubgeometry.ConfigFile expects
// (_lyx/config/<module>.yaml) without touching git or any other fixture
// machinery.
func seedLoomConfig(t *testing.T, baseDir, contents string) {
	t.Helper()
	configDir := filepath.Join(baseDir, "_lyx", "config")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) = %v; want nil", configDir, err)
	}
	cfgPath := filepath.Join(configDir, "loom.yaml")
	if err := os.WriteFile(cfgPath, []byte(contents), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) = %v; want nil", cfgPath, err)
	}
}

// TestLoadConfig_WellFormed verifies the seeded template's default values
// round-trip through LoadConfig unchanged.
func TestLoadConfig_WellFormed(t *testing.T) {
	baseDir := t.TempDir()
	seedLoomConfig(t, baseDir, ConfigTemplate())

	cfg, err := LoadConfig(baseDir, "loom")
	if err != nil {
		t.Fatalf("LoadConfig(%q, \"loom\") = _, %v; want nil error", baseDir, err)
	}
	if cfg.Discussion != "opus[effort=high]" {
		t.Errorf("cfg.Discussion = %q; want %q", cfg.Discussion, "opus[effort=high]")
	}
	if cfg.DiscussionTimeoutMin != 480 {
		t.Errorf("cfg.DiscussionTimeoutMin = %d; want %d", cfg.DiscussionTimeoutMin, 480)
	}
}

// TestLoadConfig_MalformedDiscussionSpec verifies a hand-edited loom.yaml
// with an ungrammatical discussion model-spec fails loud at load time,
// naming the "discussion" key, rather than being silently carried into the
// discussion producer's spawn site.
func TestLoadConfig_MalformedDiscussionSpec(t *testing.T) {
	baseDir := t.TempDir()
	// "opus[effort" has an unclosed bracket, written in place of the
	// template's well-formed discussion spec.
	seedLoomConfig(t, baseDir, `discussion: "opus[effort"
discussion_timeout_min: 480
`)

	_, err := LoadConfig(baseDir, "loom")
	if err == nil {
		t.Fatal("LoadConfig() = _, nil; want non-nil error for malformed discussion spec")
	}
	if !strings.Contains(err.Error(), "discussion") {
		t.Errorf("LoadConfig() error = %q; want it to name the %q key", err.Error(), "discussion")
	}
}

// TestLoadConfig_NotInitialized verifies a bare temp baseDir with no _lyx/
// directory yields the standard "not initialized" recovery hint.
func TestLoadConfig_NotInitialized(t *testing.T) {
	baseDir := t.TempDir()

	_, err := LoadConfig(baseDir, "loom")
	if err == nil {
		t.Fatal("LoadConfig() = _, nil; want non-nil error for uninitialized baseDir")
	}
	want := `not initialized here; run "lyx init"`
	if err.Error() != want {
		t.Errorf("LoadConfig() error = %q; want %q", err.Error(), want)
	}
}
