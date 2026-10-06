// set_test.go contains table-driven tests for SetValues, covering unknown-key rejection,
// byte-for-byte round-tripping of tricky values, comment/order preservation, orphan-key
// preservation, list keys and open maps, and the partial-existing regression case that motivated
// Card 1's always-mutate-the-template-tree design.

package yamlengine

import (
	"slices"
	"strings"
	"testing"
)

const (
	openMapSetTemplate = "name: tmpl\nlabels: {}\n"
	preservedMarker    = "# preserved (not in current template)"
	listKeyTemplate    = "require_pr_to_base: [\"main\"] # bases\nsquash: true\n"
)

// TestSetValues pins what a successful SetValues merges: the requested values land byte-for-byte,
// every other key keeps its existing value or its template default, template comments and key order
// survive, an orphan top-level key is preserved whole and reported, list keys and open maps are set
// whole, and a second call on the merged output reproduces it.
func TestSetValues(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name          string
		template      string
		existing      string
		pairs         []KV
		openMaps      []string
		wantValues    map[string]string
		wantPreserved []string
		contains      []string
		notContains   []string
		inOrder       []string
	}{
		{
			name:       "value with equals signs round-trips",
			template:   "key1: default\n",
			pairs:      []KV{{Key: "key1", Value: "a=b=c"}},
			wantValues: map[string]string{"key1": "a=b=c"},
		},
		{
			name:       "value with spaces round-trips",
			template:   "key1: default\n",
			pairs:      []KV{{Key: "key1", Value: "hello there world"}},
			wantValues: map[string]string{"key1": "hello there world"},
		},
		{
			name:       "multiple pairs are all applied",
			template:   "key1: default1\nkey2: default2\nkey3: default3\n",
			pairs:      []KV{{Key: "key1", Value: "set1"}, {Key: "key3", Value: "set3"}},
			wantValues: map[string]string{"key1": "set1", "key3": "set3", "key2": "default2"},
		},
		{
			name:       "empty existing behaves like the template",
			template:   "key1: default1\nkey2: default2\n",
			pairs:      []KV{{Key: "key1", Value: "set1"}},
			wantValues: map[string]string{"key1": "set1", "key2": "default2"},
		},
		{
			name:       "template comments and order survive, only the requested key changes",
			template:   "# Key 1 comment\nkey1: template_val1\n# Key 2 comment\nkey2: template_val2\n",
			existing:   "key2: user_val2\nkey1: user_val1\n",
			pairs:      []KV{{Key: "key1", Value: "new_val1"}},
			wantValues: map[string]string{"key1": "new_val1", "key2": "user_val2"},
			contains:   []string{"# Key 1 comment", "# Key 2 comment"},
			inOrder:    []string{"key1: ", "key2: "},
		},
		{
			// The plan-review round-1 regression: a key in the template but absent from a non-empty,
			// partial existing must still be applied rather than silently dropped.
			name:       "partial existing does not suppress the set",
			template:   "key1: default1\nkey2: default2\nkey3: default3\n",
			existing:   "key1: user_val1\n",
			pairs:      []KV{{Key: "key2", Value: "newly_set"}},
			wantValues: map[string]string{"key1": "user_val1", "key2": "newly_set", "key3": "default3"},
		},
		{
			name:          "unknown existing key is preserved after every template key",
			template:      "key1: default1\nkey2: default2\n",
			existing:      "key1: user_val1\nkey2: user_val2\npath: ../_board\n",
			pairs:         []KV{{Key: "key1", Value: "new_val1"}},
			wantValues:    map[string]string{"path": "../_board"},
			wantPreserved: []string{"path"},
			inOrder:       []string{"key1: ", "key2: ", "path: "},
		},
		{
			name:          "unknown existing keys are preserved and reported sorted",
			template:      "key1: default1\n",
			existing:      "key1: user_val1\nzebra: z_val\napple: a_val\nmango: m_val\n",
			wantValues:    map[string]string{"apple": "a_val", "mango": "m_val", "zebra": "z_val"},
			wantPreserved: []string{"apple", "mango", "zebra"},
		},
		{
			name:          "non-flat orphan is preserved whole under its top-level key",
			template:      "key1: default1\n",
			existing:      "key1: user_val1\nextra:\n  nested: value\n",
			wantPreserved: []string{"extra"},
			contains:      []string{"extra:", "nested: value"},
		},
		{
			name:     "emptied list survives an unrelated set",
			template: listKeyTemplate,
			existing: "require_pr_to_base: []\nsquash: true\n",
			pairs:    []KV{{Key: "squash", Value: "false"}},
			contains: []string{"require_pr_to_base: []"},
		},
		{
			name:     "lengthened list survives an unrelated set",
			template: listKeyTemplate,
			existing: "require_pr_to_base: [a, b]\nsquash: true\n",
			pairs:    []KV{{Key: "squash", Value: "false"}},
			contains: []string{"require_pr_to_base: [a, b]"},
		},
		{
			name:     "list key can be set to the empty list",
			template: listKeyTemplate,
			pairs:    []KV{{Key: "require_pr_to_base", Value: "[]"}},
			contains: []string{"require_pr_to_base: []"},
		},
		{
			name:     "list key can be set to two elements",
			template: listKeyTemplate,
			pairs:    []KV{{Key: "require_pr_to_base", Value: "[main, develop]"}},
			contains: []string{"require_pr_to_base: [main, develop]"},
		},
		{
			name:     "open map adds an entry and rewrites another",
			template: openMapSetTemplate,
			existing: "name: mine\nlabels:\n  a: old\n",
			pairs:    []KV{{Key: "labels.x", Value: "new x"}, {Key: "labels.a", Value: "new a"}},
			openMaps: []string{"labels"},
			contains: []string{"a: new a", "x: new x", "name: mine"},
		},
		{
			name:        "open map on an empty template writes a block mapping",
			template:    openMapSetTemplate,
			pairs:       []KV{{Key: "labels.x", Value: "new x"}},
			openMaps:    []string{"labels"},
			contains:    []string{"  x: new x"},
			notContains: []string{"{"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result, err := SetValues([]byte(tt.template), []byte(tt.existing), tt.pairs, tt.openMaps...)
			if err != nil {
				t.Fatalf("SetValues() unexpected error: %v", err)
			}
			if len(result.Unknown) != 0 {
				t.Fatalf("SetValues() Unknown = %v; want none", result.Unknown)
			}
			merged := string(result.Merged)
			for key, want := range tt.wantValues {
				assertMergedKeyValue(t, result, key, want)
			}
			if !slices.Equal(result.Preserved, tt.wantPreserved) {
				t.Errorf("SetValues() Preserved = %v; want %v", result.Preserved, tt.wantPreserved)
			}
			if hasMarker := strings.Contains(merged, preservedMarker); hasMarker != (len(tt.wantPreserved) > 0) {
				t.Errorf("SetValues() merged has preserved marker = %v with Preserved = %v; merged = %q", hasMarker, result.Preserved, merged)
			}
			for _, want := range tt.contains {
				if !strings.Contains(merged, want) {
					t.Errorf("SetValues() merged = %q; want it to contain %q", merged, want)
				}
			}
			for _, unwanted := range tt.notContains {
				if strings.Contains(merged, unwanted) {
					t.Errorf("SetValues() merged = %q; want it not to contain %q", merged, unwanted)
				}
			}
			previous := -1
			for _, want := range tt.inOrder {
				at := strings.Index(merged, want)
				if at < previous {
					t.Errorf("SetValues() merged = %q; want %q after the entry before it", merged, want)
				}
				previous = at
			}

			again, err := SetValues([]byte(tt.template), result.Merged, tt.pairs, tt.openMaps...)
			if err != nil {
				t.Fatalf("SetValues() second call unexpected error: %v", err)
			}
			if string(again.Merged) != merged {
				t.Errorf("SetValues() second call Merged = %q; want identical to first call Merged %q", again.Merged, merged)
			}
			if !slices.Equal(again.Preserved, result.Preserved) {
				t.Errorf("SetValues() second call Preserved = %v; want %v", again.Preserved, result.Preserved)
			}
		})
	}
}

// TestSetValues_UnknownKeys pins that an unknown key rejects the whole call: Unknown names it,
// Merged is nil so no partial mutation is observable, and Known lists what was settable.
func TestSetValues_UnknownKeys(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		template  string
		pairs     []KV
		openMaps  []string
		wantKnown string
	}{
		{
			name:     "valid pair next to an unknown one",
			template: "key1: default1\nkey2: default2\n",
			pairs:    []KV{{Key: "key1", Value: "new1"}, {Key: "bogus", Value: "irrelevant"}},
		},
		{
			name:      "undeclared key next to an open map",
			template:  openMapSetTemplate,
			pairs:     []KV{{Key: "bogus", Value: "v"}},
			openMaps:  []string{"labels"},
			wantKnown: "labels.<name>",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result, err := SetValues([]byte(tt.template), nil, tt.pairs, tt.openMaps...)
			if err != nil {
				t.Fatalf("SetValues() unexpected error: %v", err)
			}
			if !slices.Equal(result.Unknown, []string{"bogus"}) {
				t.Errorf("SetValues() Unknown = %v; want [bogus]", result.Unknown)
			}
			if result.Merged != nil {
				t.Errorf("SetValues() Merged = %q; want nil (no partial mutation)", result.Merged)
			}
			if tt.wantKnown != "" && !slices.Contains(result.Known, tt.wantKnown) {
				t.Errorf("SetValues() Known = %v; want it to contain %q", result.Known, tt.wantKnown)
			}
		})
	}
}

// TestSetValues_Refusals pins the errors SetValues returns for a value that does not fit its key.
func TestSetValues_Refusals(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		template string
		existing string
		pair     KV
		openMaps []string
		want     []string
	}{
		{
			name:     "scalar for a list key",
			template: "require_pr_to_base: [\"main\"]\n",
			pair:     KV{Key: "require_pr_to_base", Value: "main"},
			want:     []string{"list key require_pr_to_base"},
		},
		{
			name:     "entry into an open map that holds a list",
			template: openMapSetTemplate,
			existing: "labels:\n  - a\n",
			pair:     KV{Key: "labels.x", Value: "v"},
			openMaps: []string{"labels"},
			want:     []string{"labels", "map of name to description"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := SetValues([]byte(tt.template), []byte(tt.existing), []KV{tt.pair}, tt.openMaps...)
			if err == nil {
				t.Fatal("SetValues() = nil error; want a refusal")
			}
			for _, want := range tt.want {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error = %q; want it to contain %q", err, want)
				}
			}
		})
	}
}

// assertMergedKeyValue is a test helper that unmarshals result.Merged into a
// map and asserts the given top-level key holds want.
func assertMergedKeyValue(t *testing.T, result SetResult, key, want string) {
	t.Helper()
	got := extractYAMLValue(t, result.Merged, key)
	if got != want {
		t.Errorf("SetValues() merged[%q] = %q; want %q", key, got, want)
	}
}

// extractYAMLValue is a minimal top-level-key extractor for asserting a
// single scalar value out of merged YAML bytes without pulling in a full
// YAML-to-map round-trip (which would normalize quoting and defeat the
// byte-for-byte assertions this test file makes).
func extractYAMLValue(t *testing.T, data []byte, key string) string {
	t.Helper()
	prefix := key + ": "
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, prefix) {
			return strings.TrimPrefix(line, prefix)
		}
	}
	t.Fatalf("key %q not found in merged YAML: %q", key, string(data))
	return ""
}
