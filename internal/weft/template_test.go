// template_test.go — tests for the weft ConfigTemplate generator.
//
// Covers: ConfigTemplate is non-empty and parses as valid YAML yielding _lyx.

package weft

import (
	"testing"

	"gopkg.in/yaml.v3"
)

// TestConfigTemplate asserts ConfigTemplate is non-empty and parses as valid YAML
// yielding _lyx when uncommented.
func TestConfigTemplate(t *testing.T) {
	template := ConfigTemplate()

	// Assert non-empty
	if template == "" {
		t.Error("ConfigTemplate() returned empty string")
	}

	// Uncomment the line by removing the leading "# "
	uncommented := ""
	if len(template) > 2 && template[0] == '#' && template[1] == ' ' {
		uncommented = template[2:]
	} else {
		t.Fatalf("ConfigTemplate() does not start with '# ': %q", template)
	}

	// Parse as YAML
	var parsed map[string]string
	if err := yaml.Unmarshal([]byte(uncommented), &parsed); err != nil {
		t.Fatalf("failed to parse uncommented template as YAML: %v; template: %q", err, uncommented)
	}

	// Assert pathspec key exists and equals _lyx
	pathspec, exists := parsed["pathspec"]
	if !exists {
		t.Errorf("parsed YAML missing 'pathspec' key; got: %v", parsed)
	}
	if pathspec != "_lyx" {
		t.Errorf("pathspec = %q; want %q", pathspec, "_lyx")
	}
}
