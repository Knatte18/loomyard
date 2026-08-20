// config_test.go verifies batcher.yaml's template parses and Active resolves the configured
// batchifier, including its two degrading-absence cases (absent _lyx/, absent batcher.yaml) and its
// unknown-name error path, seeded via plain os.MkdirAll/os.WriteFile against a t.TempDir() rather
// than gitkit's weft/config fixture-copy helpers: configengine.LoadOrTemplate only requires a
// filesystem _lyx/config/<module>.yaml, no git repository, so this test stays untagged and
// spawn-free.
// Tier-1 (pure logic, no git, no TestMain), per the go-test-tiers-and-hermetic-git Shared Decision.

package batcher_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/batcher"
	"github.com/Knatte18/loomyard/internal/configengine"
	"gopkg.in/yaml.v3"
)

// seedConfig writes content to <baseDir>/_lyx/config/<module>.yaml,
// creating the config directory (and its _lyx parent) as needed. It is a
// plain-filesystem stand-in for gitkit's own git-spawning config-seeding
// helper, deliberately avoiding that helper's git spawn since
// configengine.LoadOrTemplate never needs a repository.
func seedConfig(t *testing.T, baseDir, module, content string) {
	t.Helper()

	configDir := configengine.ConfigDir(baseDir)
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	configPath := configengine.ConfigFile(baseDir, module)
	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write config file: %v", err)
	}
}

// TestConfigTemplate_ParsesAsYAML asserts the embedded template is well-formed YAML on its own,
// independent of Active's strict-decode path.
func TestConfigTemplate_ParsesAsYAML(t *testing.T) {
	var out map[string]any
	if err := yaml.Unmarshal([]byte(batcher.ConfigTemplate()), &out); err != nil {
		t.Fatalf("ConfigTemplate() does not parse as YAML: %v", err)
	}
}

// TestActive_TemplateDefaultResolvesIdentity seeds the template verbatim and asserts Active resolves
// the documented empty default to the identity batchifier.
func TestActive_TemplateDefaultResolvesIdentity(t *testing.T) {
	baseDir := t.TempDir()
	seedConfig(t, baseDir, "batcher", batcher.ConfigTemplate())

	got, err := batcher.Active(baseDir)
	if err != nil {
		t.Fatalf("Active(template) = _, %v; want nil error", err)
	}
	if got.Name() != batcher.DefaultName {
		t.Errorf("Active(template).Name() = %q; want %q", got.Name(), batcher.DefaultName)
	}
}

// TestActive_ExplicitNameResolves asserts an explicit active: value resolves to the matching
// registered batchifier.
func TestActive_ExplicitNameResolves(t *testing.T) {
	baseDir := t.TempDir()
	seedConfig(t, baseDir, "batcher", `active: "identity"`+"\n")

	got, err := batcher.Active(baseDir)
	if err != nil {
		t.Fatalf("Active(explicit identity) = _, %v; want nil error", err)
	}
	if got.Name() != "identity" {
		t.Errorf("Active(explicit identity).Name() = %q; want %q", got.Name(), "identity")
	}
}

// TestActive_UnknownNameErrors asserts an unregistered active: value surfaces Select's own
// unknown-batcher error unchanged.
func TestActive_UnknownNameErrors(t *testing.T) {
	baseDir := t.TempDir()
	seedConfig(t, baseDir, "batcher", `active: "does-not-exist"`+"\n")

	_, err := batcher.Active(baseDir)
	if err == nil {
		t.Fatal("Active(unknown name) = nil error; want error naming the unknown batcher")
	}
	if !strings.Contains(err.Error(), "unknown batcher") {
		t.Errorf("Active(unknown name) error = %q; want it to contain %q", err.Error(), "unknown batcher")
	}
	if !strings.Contains(err.Error(), "does-not-exist") {
		t.Errorf("Active(unknown name) error = %q; want it to contain %q", err.Error(), "does-not-exist")
	}
}

// TestActive_AbsentConfigResolvesTemplate asserts that when _lyx/ exists but batcher.yaml does not,
// Active degrades to the embedded template and resolves the identity batchifier ConfigTemplate()
// selects, rather than erroring.
func TestActive_AbsentConfigResolvesTemplate(t *testing.T) {
	baseDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(baseDir, "_lyx"), 0o755); err != nil {
		t.Fatalf("mkdir _lyx: %v", err)
	}
	// Do NOT write _lyx/config/batcher.yaml.

	got, err := batcher.Active(baseDir)
	if err != nil {
		t.Fatalf("Active(no batcher.yaml) = _, %v; want nil error", err)
	}
	if got.Name() != batcher.DefaultName {
		t.Errorf("Active(no batcher.yaml).Name() = %q; want %q", got.Name(), batcher.DefaultName)
	}
}

// TestActive_AbsentLyxDirResolvesTemplate asserts that on a bare tree with no _lyx/ directory at
// all, Active degrades to the embedded template and resolves the identity batchifier
// ConfigTemplate() selects, rather than the old strict "not initialized here" error.
func TestActive_AbsentLyxDirResolvesTemplate(t *testing.T) {
	baseDir := t.TempDir()
	// Do NOT create _lyx/ at all.

	got, err := batcher.Active(baseDir)
	if err != nil {
		t.Fatalf("Active(no _lyx/) = _, %v; want nil error", err)
	}
	if got.Name() != batcher.DefaultName {
		t.Errorf("Active(no _lyx/).Name() = %q; want %q", got.Name(), batcher.DefaultName)
	}
}
