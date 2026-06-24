// reconcile_test.go contains table-driven tests for Reconcile and MissingKeys.

package yamlengine

import (
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestReconcile_AddMissingKey(t *testing.T) {
	// Template has a key; existing does not. The key should appear in added.
	template := []byte(`
key1: default_value
`)
	existing := []byte(``)

	merged, added, removed, err := Reconcile(template, existing)
	if err != nil {
		t.Fatalf("Reconcile() unexpected error: %v", err)
	}

	// added should contain "key1" since it's in template but not existing
	if len(added) != 1 || added[0] != "key1" {
		t.Errorf("Reconcile() added = %v; want [\"key1\"]", added)
	}

	if len(removed) != 0 {
		t.Errorf("Reconcile() removed = %v; want []", removed)
	}

	// merged should contain the template default
	if !strings.Contains(string(merged), "default_value") {
		t.Errorf("Reconcile() merged does not contain template default value")
	}
}

func TestReconcile_RemoveStaleKey(t *testing.T) {
	// Existing has a key; template does not. The key should appear in removed.
	template := []byte(`
key1: default
`)
	existing := []byte(`
key1: user_value
key2: stale_value
`)

	merged, added, removed, err := Reconcile(template, existing)
	if err != nil {
		t.Fatalf("Reconcile() unexpected error: %v", err)
	}

	// removed should contain "key2" since it's in existing but not template
	if len(removed) != 1 || removed[0] != "key2" {
		t.Errorf("Reconcile() removed = %v; want [\"key2\"]", removed)
	}

	if len(added) != 0 {
		t.Errorf("Reconcile() added = %v; want []", added)
	}

	// merged should not contain the stale key2
	if strings.Contains(string(merged), "stale_value") {
		t.Errorf("Reconcile() merged contains stale value")
	}
}

func TestReconcile_PreserveUserValue(t *testing.T) {
	// Existing has a different value than template. User value should be preserved.
	template := []byte(`
key1: template_default
`)
	existing := []byte(`
key1: user_custom_value
`)

	merged, added, removed, err := Reconcile(template, existing)
	if err != nil {
		t.Fatalf("Reconcile() unexpected error: %v", err)
	}

	if len(added) != 0 {
		t.Errorf("Reconcile() added = %v; want []", added)
	}

	if len(removed) != 0 {
		t.Errorf("Reconcile() removed = %v; want []", removed)
	}

	// merged should contain the user's value, not the template default
	if !strings.Contains(string(merged), "user_custom_value") {
		t.Errorf("Reconcile() merged does not preserve user value")
	}
	if strings.Contains(string(merged), "template_default") && !strings.Contains(string(merged), "user_custom_value") {
		t.Errorf("Reconcile() merged contains template default instead of user value")
	}
}

func TestReconcile_NestedAddRemovePreserve(t *testing.T) {
	// Test nested keys at depth >= 2
	template := []byte(`
level1:
  level2:
    kept_key: template_val1
    added_key: template_val2
`)
	existing := []byte(`
level1:
  level2:
    kept_key: user_val1
    extra_key: extra_val
`)

	merged, added, removed, err := Reconcile(template, existing)
	if err != nil {
		t.Fatalf("Reconcile() unexpected error: %v", err)
	}

	// added should contain "level1.level2.added_key"
	if !stringSliceContains(added, "level1.level2.added_key") {
		t.Errorf("Reconcile() added = %v; should contain level1.level2.added_key", added)
	}

	// removed should contain "level1.level2.extra_key"
	if !stringSliceContains(removed, "level1.level2.extra_key") {
		t.Errorf("Reconcile() removed = %v; should contain level1.level2.extra_key", removed)
	}

	// merged should preserve user value for kept_key
	if !strings.Contains(string(merged), "user_val1") {
		t.Errorf("Reconcile() merged does not preserve nested user value")
	}
}

func TestReconcile_EmptyExisting(t *testing.T) {
	// Empty existing should yield all template keys as added.
	template := []byte(`
key1: val1
key2: val2
`)
	existing := []byte(``)

	merged, added, removed, err := Reconcile(template, existing)
	if err != nil {
		t.Fatalf("Reconcile() unexpected error: %v", err)
	}

	// added should contain all keys from template
	if len(added) != 2 {
		t.Errorf("Reconcile() added = %v; want 2 keys", added)
	}

	if len(removed) != 0 {
		t.Errorf("Reconcile() removed = %v; want []", removed)
	}

	// merged should be equivalent to template
	mergedStr := string(merged)
	if !strings.Contains(mergedStr, "val1") || !strings.Contains(mergedStr, "val2") {
		t.Errorf("Reconcile() merged does not contain template values")
	}
}

func TestReconcile_CommentsOnlyExisting(t *testing.T) {
	// Existing with only comments should be treated as empty.
	template := []byte(`
key1: val1
`)
	existing := []byte(`
# This is just a comment
# No actual config
`)

	_, added, removed, err := Reconcile(template, existing)
	if err != nil {
		t.Fatalf("Reconcile() unexpected error: %v", err)
	}

	// added should contain "key1" (treat comments-only as empty)
	if len(added) != 1 || added[0] != "key1" {
		t.Errorf("Reconcile() added = %v; want [\"key1\"]", added)
	}

	if len(removed) != 0 {
		t.Errorf("Reconcile() removed = %v; want []", removed)
	}
}

func TestReconcile_Idempotence(t *testing.T) {
	// Reconcile(t, Reconcile(t, e)) should produce the same merged and empty deltas.
	template := []byte(`
key1: template_val1
key2: template_val2
`)
	existing := []byte(`
key1: user_val1
key3: extra_val
`)

	// First reconciliation
	merged1, _, _, err := Reconcile(template, existing)
	if err != nil {
		t.Fatalf("Reconcile() first call unexpected error: %v", err)
	}

	// Second reconciliation using merged1 as the new existing
	merged2, added2, removed2, err := Reconcile(template, merged1)
	if err != nil {
		t.Fatalf("Reconcile() second call unexpected error: %v", err)
	}

	// Merged results should be identical
	if !strings.EqualFold(strings.TrimSpace(string(merged1)), strings.TrimSpace(string(merged2))) {
		t.Errorf("Reconcile() idempotence failed: merged1 != merged2")
	}

	// added2 and removed2 should be empty (idempotent)
	if len(added2) != 0 {
		t.Errorf("Reconcile() idempotence: added2 = %v; want []", added2)
	}

	if len(removed2) != 0 {
		t.Errorf("Reconcile() idempotence: removed2 = %v; want []", removed2)
	}
}

func TestReconcile_TemplateCommentsAndOrder(t *testing.T) {
	// Template comments and key order should be preserved in merged output.
	template := []byte(`
# Key 1 comment
key1: template_val1
# Key 2 comment
key2: template_val2
`)
	existing := []byte(`
key2: user_val2
key1: user_val1
`)

	merged, _, _, err := Reconcile(template, existing)
	if err != nil {
		t.Fatalf("Reconcile() unexpected error: %v", err)
	}

	mergedStr := string(merged)

	// Check that template comments are preserved
	if !strings.Contains(mergedStr, "# Key 1 comment") {
		t.Errorf("Reconcile() merged does not preserve template comments")
	}

	// Check that user values are preserved
	if !strings.Contains(mergedStr, "user_val1") || !strings.Contains(mergedStr, "user_val2") {
		t.Errorf("Reconcile() merged does not preserve user values")
	}

	// Rough check that key1 comes before key2 (order from template)
	idx1 := strings.Index(mergedStr, "key1")
	idx2 := strings.Index(mergedStr, "key2")
	if idx1 > idx2 {
		t.Errorf("Reconcile() merged does not preserve template key order")
	}
}

func TestMissingKeys_TemplateOnly(t *testing.T) {
	// Template has keys; existing is empty. MissingKeys should return all template keys.
	template := []byte(`
key1: val1
key2: val2
`)
	existing := []byte(``)

	missing, err := MissingKeys(template, existing)
	if err != nil {
		t.Fatalf("MissingKeys() unexpected error: %v", err)
	}

	if len(missing) != 2 {
		t.Errorf("MissingKeys() = %v; want 2 keys", missing)
	}

	if !stringSliceContains(missing, "key1") || !stringSliceContains(missing, "key2") {
		t.Errorf("MissingKeys() = %v; want keys key1 and key2", missing)
	}
}

func TestMissingKeys_AllPresent(t *testing.T) {
	// All template keys present in existing. MissingKeys should return empty.
	template := []byte(`
key1: val1
key2: val2
`)
	existing := []byte(`
key1: user_val1
key2: user_val2
`)

	missing, err := MissingKeys(template, existing)
	if err != nil {
		t.Fatalf("MissingKeys() unexpected error: %v", err)
	}

	if len(missing) != 0 {
		t.Errorf("MissingKeys() = %v; want []", missing)
	}
}

func TestMissingKeys_EmptyValueCountsAsPresent(t *testing.T) {
	// A key with an empty value should NOT be reported as missing.
	template := []byte(`
key1: template_val
key2: default_val
`)
	existing := []byte(`
key1: ""
key2: user_val
`)

	missing, err := MissingKeys(template, existing)
	if err != nil {
		t.Fatalf("MissingKeys() unexpected error: %v", err)
	}

	// Neither key1 nor key2 should be missing
	if len(missing) != 0 {
		t.Errorf("MissingKeys() = %v; want [] (empty value counts as present)", missing)
	}
}

func TestMissingKeys_Nested(t *testing.T) {
	// Nested keys at depth >= 2
	template := []byte(`
level1:
  level2:
    key_a: val_a
    key_b: val_b
`)
	existing := []byte(`
level1:
  level2:
    key_a: user_val_a
`)

	missing, err := MissingKeys(template, existing)
	if err != nil {
		t.Fatalf("MissingKeys() unexpected error: %v", err)
	}

	if len(missing) != 1 || missing[0] != "level1.level2.key_b" {
		t.Errorf("MissingKeys() = %v; want [\"level1.level2.key_b\"]", missing)
	}
}

// Helper function to check if a string slice contains a specific string.
func stringSliceContains(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}

// TestReconcile_YAMLNodePreservation verifies that YAML node structure is preserved.
func TestReconcile_YAMLNodePreservation(t *testing.T) {
	template := []byte(`
key1: value1
key2: value2
`)
	existing := []byte(`
key1: custom_value
`)

	merged, _, _, err := Reconcile(template, existing)
	if err != nil {
		t.Fatalf("Reconcile() unexpected error: %v", err)
	}

	// Verify that merged can be unmarshalled into a valid YAML structure
	var result map[string]interface{}
	if err := yaml.Unmarshal(merged, &result); err != nil {
		t.Fatalf("Cannot unmarshal merged YAML: %v", err)
	}

	if result["key1"] != "custom_value" {
		t.Errorf("Merged YAML key1 = %v; want \"custom_value\"", result["key1"])
	}

	if result["key2"] != "value2" {
		t.Errorf("Merged YAML key2 = %v; want \"value2\"", result["key2"])
	}
}

// TestMissingKeys_Integration tests MissingKeys end-to-end.
func TestMissingKeys_Integration(t *testing.T) {
	tests := []struct {
		name     string
		template []byte
		existing []byte
		want     []string
	}{
		{
			name:     "empty_existing",
			template: []byte("key1: v1\nkey2: v2"),
			existing: []byte(""),
			want:     []string{"key1", "key2"},
		},
		{
			name:     "partial_overlap",
			template: []byte("a: 1\nb: 2\nc: 3"),
			existing: []byte("b: 20"),
			want:     []string{"a", "c"},
		},
		{
			name:     "all_present",
			template: []byte("a: 1\nb: 2"),
			existing: []byte("a: 10\nb: 20"),
			want:     []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := MissingKeys(tt.template, tt.existing)
			if err != nil {
				t.Fatalf("MissingKeys() unexpected error: %v", err)
			}

			// Sort both slices for comparison
			sort.Strings(got)
			sort.Strings(tt.want)

			if len(got) != len(tt.want) {
				t.Errorf("MissingKeys() returned %d items; want %d", len(got), len(tt.want))
				return
			}

			for i, v := range got {
				if v != tt.want[i] {
					t.Errorf("MissingKeys() got %v; want %v", got, tt.want)
					return
				}
			}
		})
	}
}
