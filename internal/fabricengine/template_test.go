// template_test.go — tests for the fabric ConfigTemplate generator.
//
// Covers: ConfigTemplate returns valid YAML with both expected keys and resolves
// to the correct defaults when the environment is empty.

package fabricengine

import (
	"strings"
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

// TestConfigTemplate_HasBothKeys asserts that the template contains both the
// branch_prefix and pathspec keys.
func TestConfigTemplate_HasBothKeys(t *testing.T) {
	got := ConfigTemplate()
	var result map[string]any
	if err := yaml.Unmarshal([]byte(got), &result); err != nil {
		t.Fatalf("ConfigTemplate() is not valid YAML: %v", err)
	}

	if _, ok := result["branch_prefix"]; !ok {
		t.Errorf("ConfigTemplate() missing expected key: branch_prefix")
	}
	if _, ok := result["pathspec"]; !ok {
		t.Errorf("ConfigTemplate() missing expected key: pathspec")
	}
}

// TestConfigTemplate_ResolvesToEmptyBranchPrefix asserts that resolving the
// template against an empty environment yields an empty string for
// branch_prefix.
func TestConfigTemplate_ResolvesToEmptyBranchPrefix(t *testing.T) {
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
		t.Fatalf("resolved template missing key branch_prefix")
	}
	if branchPrefix != "" {
		t.Errorf("resolved[branch_prefix] = %q; want %q", branchPrefix, "")
	}
}

// TestConfigTemplate_PathspecResolvesToLyxAndPattern asserts that the
// template's pathspec default resolves to "_lyx" and "_pattern", in that
// order, regardless of environment. The resolved value is whitespace-split
// (mirroring Config.Dirs, the consumer that actually splits it) rather than
// compared as one whole string, since the value is whitespace-split at the
// consumer -- a splitting bug there would otherwise be silent and would
// simply drop "_pattern".
func TestConfigTemplate_PathspecResolvesToLyxAndPattern(t *testing.T) {
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
		t.Fatalf("resolved template missing key pathspec")
	}
	pathspecStr, ok := pathspec.(string)
	if !ok {
		t.Fatalf("resolved[pathspec] = %#v; want a string", pathspec)
	}
	got2 := strings.Fields(pathspecStr)
	want := []string{"_lyx", "_pattern"}
	if len(got2) != len(want) {
		t.Fatalf("resolved[pathspec] whitespace-split = %v; want %v", got2, want)
	}
	for i := range want {
		if got2[i] != want[i] {
			t.Errorf("resolved[pathspec] whitespace-split = %v; want %v", got2, want)
			break
		}
	}
}
