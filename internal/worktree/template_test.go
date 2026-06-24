// template_test.go — tests for the worktree ConfigTemplate generator.
//
// Covers: ConfigTemplate returns valid YAML with correct schema and resolves to
// the correct defaults when the environment is empty.

package worktree

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

// TestConfigTemplate_HasBranchPrefixKey asserts that the template contains
// the branch_prefix key.
func TestConfigTemplate_HasBranchPrefixKey(t *testing.T) {
	got := ConfigTemplate()
	var result map[string]any
	if err := yaml.Unmarshal([]byte(got), &result); err != nil {
		t.Fatalf("ConfigTemplate() is not valid YAML: %v", err)
	}

	if _, ok := result["branch_prefix"]; !ok {
		t.Errorf("ConfigTemplate() missing expected key: branch_prefix")
	}
}

// TestConfigTemplate_ResolvesToEmptyDefault asserts that resolving the template
// against an empty environment yields an empty string for branch_prefix.
func TestConfigTemplate_ResolvesToEmptyDefault(t *testing.T) {
	got := ConfigTemplate()
	resolved, err := yamlengine.Resolve([]byte(got), nil)
	if err != nil {
		t.Fatalf("Resolve() failed: %v", err)
	}

	var result map[string]any
	if err := yaml.Unmarshal(resolved, &result); err != nil {
		t.Fatalf("resolved YAML is not valid: %v", err)
	}

	branchPrefix, ok := result["branch_prefix"]
	if !ok {
		t.Errorf("resolved template missing key branch_prefix")
		return
	}
	if branchPrefix != "" {
		t.Errorf("resolved[branch_prefix] = %q; want %q", branchPrefix, "")
	}
}
