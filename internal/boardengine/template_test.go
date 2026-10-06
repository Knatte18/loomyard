// template_test.go — tests for the boardengine ConfigTemplate generator.
//
// Covers: ConfigTemplate parses as YAML and resolves to the correct defaults, every required key present, when the environment is empty.

package boardengine

import (
	"reflect"
	"testing"

	"github.com/Knatte18/loomyard/internal/yamlengine"
	"gopkg.in/yaml.v3"
)

// TestConfigTemplate_ResolvesToDefaults asserts the template resolves to correct defaults with
// empty environment.
func TestConfigTemplate_ResolvesToDefaults(t *testing.T) {
	t.Parallel()
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
		{"readme", "README.md"},
		{"design_prefix", "design-"},
		{"types", map[string]any{
			"bug":         "Something is broken or behaves wrongly",
			"enhancement": "A new capability or an improvement to an existing one",
		}},
		{"labels", map[string]any{}},
	}

	for _, tt := range tests {
		got, ok := result[tt.key]
		if !ok {
			t.Errorf("resolved template missing key %q", tt.key)
			continue
		}
		if !reflect.DeepEqual(got, tt.wantVal) {
			t.Errorf("resolved[%q] = %v; want %v", tt.key, got, tt.wantVal)
		}
	}
}
