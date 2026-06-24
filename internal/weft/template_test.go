// template_test.go — tests for the weft ConfigTemplate generator.
//
// Covers: ConfigTemplate returns valid YAML with correct schema and resolves to
// the correct literal value (no env-marker substitution) when the environment is empty.

package weft

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

// TestConfigTemplate_HasPathspecKey asserts that the template contains
// the pathspec key.
func TestConfigTemplate_HasPathspecKey(t *testing.T) {
	got := ConfigTemplate()
	var result map[string]any
	if err := yaml.Unmarshal([]byte(got), &result); err != nil {
		t.Fatalf("ConfigTemplate() is not valid YAML: %v", err)
	}

	if _, ok := result["pathspec"]; !ok {
		t.Errorf("ConfigTemplate() missing expected key: pathspec")
	}
}

// TestConfigTemplate_ResolvesToLiteralValue asserts that resolving the template
// against an empty environment yields the literal _lyx value (no substitution).
func TestConfigTemplate_ResolvesToLiteralValue(t *testing.T) {
	got := ConfigTemplate()
	resolved, err := yamlengine.Resolve([]byte(got), nil)
	if err != nil {
		t.Fatalf("Resolve() failed: %v", err)
	}

	var result map[string]any
	if err := yaml.Unmarshal(resolved, &result); err != nil {
		t.Fatalf("resolved YAML is not valid: %v", err)
	}

	pathspec, ok := result["pathspec"]
	if !ok {
		t.Errorf("resolved template missing key pathspec")
		return
	}
	if pathspec != "_lyx" {
		t.Errorf("resolved[pathspec] = %q; want %q", pathspec, "_lyx")
	}
}
