// template_test.go — tests for the boardengine ConfigTemplate generator.
//
// Covers: ConfigTemplate returns valid YAML with correct schema and resolves to
// the correct defaults when the environment is empty.

package boardengine

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/yamlengine"
	"gopkg.in/yaml.v3"
)

// TestConfigTemplate_ValidYAML asserts that ConfigTemplate returns valid YAML
// that can be parsed without error.
func TestConfigTemplate_ValidYAML(t *testing.T) {
	got := ConfigTemplate()
	var result map[string]any
	if err := yaml.Unmarshal([]byte(got), &result); err != nil {
		t.Errorf("ConfigTemplate() is not valid YAML: %v", err)
	}
}

// TestConfigTemplate_HasRequiredKeys asserts that the template contains
// all expected configuration keys (home, sidebar, proposal_prefix).
// The geometry key path is intentionally absent — board data dir is now
// owned by hubgeometry.BoardDir, not the config file.
func TestConfigTemplate_HasRequiredKeys(t *testing.T) {
	got := ConfigTemplate()
	var result map[string]any
	if err := yaml.Unmarshal([]byte(got), &result); err != nil {
		t.Fatalf("ConfigTemplate() is not valid YAML: %v", err)
	}

	expectedKeys := []string{"home", "sidebar", "proposal_prefix"}
	for _, key := range expectedKeys {
		if _, ok := result[key]; !ok {
			t.Errorf("ConfigTemplate() missing expected key: %s", key)
		}
	}
}

// TestConfigTemplate_ResolvesToDefaults asserts that resolving the template
// against an empty environment yields the correct default values.
func TestConfigTemplate_ResolvesToDefaults(t *testing.T) {
	got := ConfigTemplate()
	resolved, err := yamlengine.Resolve([]byte(got), nil)
	if err != nil {
		t.Fatalf("Resolve() failed: %v", err)
	}

	var result map[string]any
	if err := yaml.Unmarshal(resolved, &result); err != nil {
		t.Fatalf("resolved YAML is not valid: %v", err)
	}

	tests := []struct {
		key     string
		wantVal any
	}{
		{"home", "Home.md"},
		{"sidebar", "_Sidebar.md"},
		{"proposal_prefix", "proposal-"},
	}

	for _, tt := range tests {
		got, ok := result[tt.key]
		if !ok {
			t.Errorf("resolved template missing key %q", tt.key)
			continue
		}
		if got != tt.wantVal {
			t.Errorf("resolved[%q] = %q; want %q", tt.key, got, tt.wantVal)
		}
	}
}
