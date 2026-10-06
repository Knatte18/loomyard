// set_test.go — unit tests for the non-interactive Set entry point (set.go).
//
// Tests cover: scaffold-then-set when the config file is missing, rollback of a freshly-scaffolded file on an unknown key, byte-for-byte preservation of a pre-existing file on a refused key, preservation of untouched keys when setting one key on an existing multi-key file, open-map entries, and end-to-end reporting of Set's returned preserved-keys list for an orphaned key.

package configengine_test

import (
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/yamlengine"
)

// TestSet pins what Set writes: a missing file is scaffolded from the template and the pairs applied in one call, other keys keep their values, an orphaned top-level key survives and is reported, and a refused pair removes a file Set scaffolded or leaves a pre-existing one byte for byte unchanged.
func TestSet(t *testing.T) {
	t.Parallel()
	const labelsTemplate = "name: x\nlabels:\n  bug: a bug\n"

	tests := []struct {
		name     string
		state    baseState
		existing string
		template string
		pairs    []yamlengine.KV
		openMaps []string
		// wantErr is a substring of the refusal; empty means Set must succeed.
		wantErr       string
		wantPreserved []string
		contains      []string
	}{
		{
			name:     "missing file is scaffolded then set",
			state:    baseLyx,
			template: "key1: value1\nkey2: value2\n",
			pairs:    []yamlengine.KV{{Key: "key1", Value: "set1"}},
			contains: []string{"key1: set1", "key2: value2"},
		},
		{
			name:     "other keys keep their values",
			state:    baseWithFile,
			existing: "key1: original_value1\nkey2: original_value2\n",
			template: "key1: default1\nkey2: default2\n",
			pairs:    []yamlengine.KV{{Key: "key1", Value: "new_value1"}},
			contains: []string{"key1: new_value1", "key2: original_value2"},
		},
		{
			name:          "key absent from the template is preserved and reported",
			state:         baseWithFile,
			existing:      "key1: original_value1\nlegacy: keepme\n",
			template:      "key1: default1\n",
			pairs:         []yamlengine.KV{{Key: "key1", Value: "new_value1"}},
			wantPreserved: []string{"legacy"},
			contains:      []string{"key1: new_value1", "legacy: keepme"},
		},
		{
			name:     "declared open map gains an entry beside the existing ones",
			state:    baseWithFile,
			existing: labelsTemplate,
			template: labelsTemplate,
			pairs:    []yamlengine.KV{{Key: "labels.x", Value: "an x"}},
			openMaps: []string{"labels"},
			contains: []string{"x: an x", "bug: a bug"},
		},
		{
			name:     "unknown key removes the scaffolded file",
			state:    baseLyx,
			template: "key1: value1\n",
			pairs:    []yamlengine.KV{{Key: "bogus", Value: "x"}},
			wantErr:  "bogus",
		},
		{
			name:     "unknown key leaves the existing file unchanged",
			state:    baseWithFile,
			existing: "key1: original_value\n",
			template: "key1: default1\n",
			pairs:    []yamlengine.KV{{Key: "bogus", Value: "x"}},
			wantErr:  "unknown config key(s)",
		},
		{
			name:     "undeclared key beside an open map is still refused",
			state:    baseWithFile,
			existing: labelsTemplate,
			template: labelsTemplate,
			pairs:    []yamlengine.KV{{Key: "bogus", Value: "x"}},
			openMaps: []string{"labels"},
			wantErr:  "unknown config key(s)",
		},
		{
			name:     "open-map entry on a list-shaped file is refused",
			state:    baseWithFile,
			existing: "name: x\nlabels:\n  - bug\n",
			template: labelsTemplate,
			pairs:    []yamlengine.KV{{Key: "labels.x", Value: "an x"}},
			openMaps: []string{"labels"},
			wantErr:  "map of name to description",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			baseDir, path := newBase(t, tt.state, "testmod", tt.existing)

			preserved, err := configengine.Set(baseDir, "testmod", tt.template, tt.pairs, tt.openMaps...)

			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("Set() = %v; want an error containing %q", err, tt.wantErr)
				}
				if tt.state == baseLyx {
					if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
						t.Errorf("config file still exists after the refusal; want the scaffold removed")
					}
					return
				}
				assertFileUnchanged(t, path, tt.existing)
				return
			}
			if err != nil {
				t.Fatalf("Set() = %v; want nil", err)
			}
			if !slices.Equal(preserved, tt.wantPreserved) {
				t.Errorf("Set() preserved = %v; want %v", preserved, tt.wantPreserved)
			}
			data, readErr := os.ReadFile(path)
			if readErr != nil {
				t.Fatalf("config file not found at %s: %v", path, readErr)
			}
			for _, want := range tt.contains {
				if !strings.Contains(string(data), want) {
					t.Errorf("Set() file = %q; want it to contain %q", data, want)
				}
			}
		})
	}
}
