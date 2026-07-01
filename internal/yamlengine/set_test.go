// set_test.go contains table-driven and individual tests for SetValues, covering
// unknown-key rejection, byte-for-byte round-tripping of tricky values, comment/order
// preservation, and the partial-existing regression case that motivated Card 1's
// always-mutate-the-template-tree design.

package yamlengine

import (
	"strings"
	"testing"
)

// TestSetValues_UnknownKeyRejectsWholeCall verifies that when any key among
// multiple pairs is unknown, SetValues returns a non-empty Unknown and a nil
// Merged — no partial mutation is observable, even though the other keys in
// the same call are valid.
func TestSetValues_UnknownKeyRejectsWholeCall(t *testing.T) {
	template := []byte("key1: default1\nkey2: default2\n")

	result, err := SetValues(template, nil, []KV{
		{Key: "key1", Value: "new1"},
		{Key: "bogus", Value: "irrelevant"},
	})
	if err != nil {
		t.Fatalf("SetValues() unexpected error: %v", err)
	}

	if len(result.Unknown) != 1 || result.Unknown[0] != "bogus" {
		t.Errorf("SetValues() Unknown = %v; want [\"bogus\"]", result.Unknown)
	}
	if result.Merged != nil {
		t.Errorf("SetValues() Merged = %q; want nil (no partial mutation)", result.Merged)
	}
}

// TestSetValues_ValueWithEqualsRoundTrips verifies that a value containing an
// '=' character is preserved byte-for-byte in Merged.
func TestSetValues_ValueWithEqualsRoundTrips(t *testing.T) {
	template := []byte("key1: default\n")
	const want = "a=b=c"

	result, err := SetValues(template, nil, []KV{{Key: "key1", Value: want}})
	if err != nil {
		t.Fatalf("SetValues() unexpected error: %v", err)
	}
	assertMergedKeyValue(t, result, "key1", want)
}

// TestSetValues_ValueWithSpacesRoundTrips verifies that a value containing
// spaces is preserved byte-for-byte in Merged.
func TestSetValues_ValueWithSpacesRoundTrips(t *testing.T) {
	template := []byte("key1: default\n")
	const want = "hello there world"

	result, err := SetValues(template, nil, []KV{{Key: "key1", Value: want}})
	if err != nil {
		t.Fatalf("SetValues() unexpected error: %v", err)
	}
	assertMergedKeyValue(t, result, "key1", want)
}

// TestSetValues_MultiplePairsAllApplied verifies that multiple valid pairs in
// one call are all reflected in Merged.
func TestSetValues_MultiplePairsAllApplied(t *testing.T) {
	template := []byte("key1: default1\nkey2: default2\nkey3: default3\n")

	result, err := SetValues(template, nil, []KV{
		{Key: "key1", Value: "set1"},
		{Key: "key3", Value: "set3"},
	})
	if err != nil {
		t.Fatalf("SetValues() unexpected error: %v", err)
	}
	assertMergedKeyValue(t, result, "key1", "set1")
	assertMergedKeyValue(t, result, "key3", "set3")
	assertMergedKeyValue(t, result, "key2", "default2")
}

// TestSetValues_CommentsAndOrderPreserved mirrors the idempotency-style
// assertions in TestReconcile_TemplateCommentsAndOrder: template comments and
// key order survive in Merged, and only the requested key's value changes.
func TestSetValues_CommentsAndOrderPreserved(t *testing.T) {
	template := []byte("# Key 1 comment\nkey1: template_val1\n# Key 2 comment\nkey2: template_val2\n")
	existing := []byte("key2: user_val2\nkey1: user_val1\n")

	result, err := SetValues(template, existing, []KV{{Key: "key1", Value: "new_val1"}})
	if err != nil {
		t.Fatalf("SetValues() unexpected error: %v", err)
	}
	if result.Unknown != nil {
		t.Fatalf("SetValues() Unknown = %v; want none", result.Unknown)
	}

	merged := string(result.Merged)
	if !strings.Contains(merged, "# Key 1 comment") || !strings.Contains(merged, "# Key 2 comment") {
		t.Errorf("SetValues() merged does not preserve template comments; got %q", merged)
	}
	// key1 was explicitly set to new_val1, overriding existing's user_val1.
	assertMergedKeyValue(t, result, "key1", "new_val1")
	// key2 was untouched by pairs, so existing's user_val2 must survive.
	assertMergedKeyValue(t, result, "key2", "user_val2")

	idx1 := strings.Index(merged, "key1")
	idx2 := strings.Index(merged, "key2")
	if idx1 > idx2 {
		t.Errorf("SetValues() merged does not preserve template key order")
	}
}

// TestSetValues_EmptyExistingBehavesLikeTemplate verifies that an empty
// existing behaves like Reconcile's empty-existing case: Merged is equivalent
// to the template with the requested keys set.
func TestSetValues_EmptyExistingBehavesLikeTemplate(t *testing.T) {
	template := []byte("key1: default1\nkey2: default2\n")

	result, err := SetValues(template, nil, []KV{{Key: "key1", Value: "set1"}})
	if err != nil {
		t.Fatalf("SetValues() unexpected error: %v", err)
	}
	assertMergedKeyValue(t, result, "key1", "set1")
	assertMergedKeyValue(t, result, "key2", "default2")
}

// TestSetValues_PartialExistingDoesNotSuppressSet is the plan-review round-1
// regression case: a pairs[i].Key present in template (so it passes Known
// validation) but absent from a non-empty, partial existing (which only has
// one of the template's three keys) must still be applied in Merged rather
// than silently dropped, because the working tree is always templateNode —
// never a bare parse of existing — so every template leaf has a real node
// regardless of what existing does or doesn't contain.
func TestSetValues_PartialExistingDoesNotSuppressSet(t *testing.T) {
	template := []byte("key1: default1\nkey2: default2\nkey3: default3\n")
	// existing has only key1; key2 and key3 have no corresponding node here.
	existing := []byte("key1: user_val1\n")

	result, err := SetValues(template, existing, []KV{{Key: "key2", Value: "newly_set"}})
	if err != nil {
		t.Fatalf("SetValues() unexpected error: %v", err)
	}
	if result.Unknown != nil {
		t.Fatalf("SetValues() Unknown = %v; want none", result.Unknown)
	}
	// key1's existing override must survive.
	assertMergedKeyValue(t, result, "key1", "user_val1")
	// key2 must be set, not silently dropped because it had no node in existing.
	assertMergedKeyValue(t, result, "key2", "newly_set")
	// key3 must remain the template default (untouched by both existing and pairs).
	assertMergedKeyValue(t, result, "key3", "default3")
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
