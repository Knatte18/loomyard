// config_test.go verifies webster.yaml's template parses, defaults resolve
// through LoadConfig, overrides round-trip, and a malformed role model-spec
// fails loud naming the offending key — the builderengine config_test.go
// pattern, but seeded via plain os.MkdirAll/os.WriteFile against a
// t.TempDir() rather than lyxtest.CopyWeft/SeedConfig: configengine.Load
// only requires a filesystem _lyx/config/<module>.yaml, no git repository,
// so this test stays untagged and spawn-free (Test Tier Purity Invariant).

package websterengine_test

import (
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/hubgeometry"
	"github.com/Knatte18/loomyard/internal/websterengine"
	"gopkg.in/yaml.v3"
)

// seedConfig writes module's content to <baseDir>/_lyx/config/<module>.yaml,
// creating the config directory (and its _lyx parent) as needed. It is a
// plain-filesystem stand-in for lyxtest.SeedConfig, deliberately avoiding
// that helper's git spawn since configengine.Load never needs a repository.
func seedConfig(t *testing.T, baseDir, module, content string) {
	t.Helper()

	configDir := hubgeometry.ConfigDir(baseDir)
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	configPath := hubgeometry.ConfigFile(baseDir, module)
	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write config file: %v", err)
	}
}

// TestConfigTemplate_ParsesAsYAML asserts the embedded template is
// well-formed YAML on its own, independent of LoadConfig's strict-decode
// path.
func TestConfigTemplate_ParsesAsYAML(t *testing.T) {
	var out map[string]any
	if err := yaml.Unmarshal([]byte(websterengine.ConfigTemplate()), &out); err != nil {
		t.Fatalf("ConfigTemplate() does not parse as YAML: %v", err)
	}
}

// TestConfigTemplate_RoundTripsThroughLoadConfig seeds the template
// verbatim and asserts LoadConfig resolves it into the documented defaults.
func TestConfigTemplate_RoundTripsThroughLoadConfig(t *testing.T) {
	baseDir := t.TempDir()
	seedConfig(t, baseDir, "webster", websterengine.ConfigTemplate())

	cfg, err := websterengine.LoadConfig(baseDir, "webster")
	if err != nil {
		t.Fatalf("LoadConfig(template) = _, %v; want nil error", err)
	}

	want := websterengine.Config{
		Master:                "sonnet",
		MasterOversized:       "opus",
		Recovery:              "opus[effort=high]",
		SelfFixCap:            2,
		MasterTimeoutMin:      480,
		RecoveryTimeoutMin:    60,
		PollWaitS:             480,
		BatchContextCapTokens: 100000,
		BatchCardCap:          10,
	}
	if cfg != want {
		t.Errorf("LoadConfig(template) = %+v; want %+v", cfg, want)
	}
}

// TestConfigTemplate_ContainsEveryConfigYAMLTag walks Config's fields via
// reflection and asserts every yaml tag appears in the template text — so a
// struct field added without a matching template line is caught
// mechanically rather than relying on review to notice the gap.
func TestConfigTemplate_ContainsEveryConfigYAMLTag(t *testing.T) {
	text := websterengine.ConfigTemplate()

	typ := reflect.TypeOf(websterengine.Config{})
	for i := 0; i < typ.NumField(); i++ {
		tag := typ.Field(i).Tag.Get("yaml")
		if tag == "" {
			t.Fatalf("Config field %q has no yaml tag", typ.Field(i).Name)
		}
		if !containsKey(text, tag) {
			t.Errorf("ConfigTemplate() does not contain key %q for field %q", tag, typ.Field(i).Name)
		}
	}
}

// containsKey reports whether text contains a "<key>:" line-start token,
// the shape every one of this template's keys takes.
func containsKey(text, key string) bool {
	needle := key + ":"
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), needle) {
			return true
		}
	}
	return false
}

func TestLoadConfig_OverridesRoundTrip(t *testing.T) {
	baseDir := t.TempDir()
	override := `master: opus[effort=high]
master_oversized: opus[effort=max]
recovery: opus[effort=max]
self_fix_cap: 5
master_timeout_min: 120
recovery_timeout_min: 30
poll_wait_s: 60
batch_context_cap_tokens: 50000
batch_card_cap: 6
`
	seedConfig(t, baseDir, "webster", override)

	cfg, err := websterengine.LoadConfig(baseDir, "webster")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Master != "opus[effort=high]" {
		t.Errorf("Master = %q, want %q", cfg.Master, "opus[effort=high]")
	}
	if cfg.MasterOversized != "opus[effort=max]" {
		t.Errorf("MasterOversized = %q, want %q", cfg.MasterOversized, "opus[effort=max]")
	}
	if cfg.SelfFixCap != 5 {
		t.Errorf("SelfFixCap = %d, want %d", cfg.SelfFixCap, 5)
	}
	if cfg.BatchCardCap != 6 {
		t.Errorf("BatchCardCap = %d, want %d", cfg.BatchCardCap, 6)
	}
}

func TestLoadConfig_BadRoleGrammarNamesTheKey(t *testing.T) {
	baseDir := t.TempDir()
	// "sonnet " has a trailing space — Parse rejects whitespace anywhere in
	// a spec string.
	badRole := `master: sonnet
master_oversized: "opus "
recovery: opus[effort=high]
self_fix_cap: 2
master_timeout_min: 480
recovery_timeout_min: 60
poll_wait_s: 480
batch_context_cap_tokens: 100000
batch_card_cap: 10
`
	seedConfig(t, baseDir, "webster", badRole)

	_, err := websterengine.LoadConfig(baseDir, "webster")
	if err == nil {
		t.Fatal("LoadConfig() = nil error; want error naming the offending key")
	}
	if !strings.Contains(err.Error(), "master_oversized") {
		t.Errorf("LoadConfig() error = %q; want it to name the offending key %q", err.Error(), "master_oversized")
	}
}

func TestLoadConfig_NotInitialized(t *testing.T) {
	tmpDir := t.TempDir()
	// Do NOT create _lyx/

	cfg, err := websterengine.LoadConfig(tmpDir, "webster")
	if err == nil {
		t.Fatalf("expected error for not initialized, got nil; config: %+v", cfg)
	}

	errMsg := err.Error()
	if !strings.Contains(errMsg, "not initialized") {
		t.Errorf("expected error containing 'not initialized', got: %v", err)
	}
	if !strings.Contains(errMsg, "lyx init") {
		t.Errorf("expected error containing 'lyx init', got: %v", err)
	}
}
