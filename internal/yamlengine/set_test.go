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

// TestSetValues_PreservesUnknownExistingKey verifies that a top-level key in
// existing with no counterpart in the template survives verbatim in Merged,
// is reported in SetResult.Preserved, is marked with the preserved marker
// comment, and is appended after every template key.
func TestSetValues_PreservesUnknownExistingKey(t *testing.T) {
	template := []byte("key1: default1\nkey2: default2\n")
	existing := []byte("key1: user_val1\nkey2: user_val2\npath: ../_board\n")

	result, err := SetValues(template, existing, []KV{{Key: "key1", Value: "new_val1"}})
	if err != nil {
		t.Fatalf("SetValues() unexpected error: %v", err)
	}
	if result.Unknown != nil {
		t.Fatalf("SetValues() Unknown = %v; want none", result.Unknown)
	}

	// The orphaned key's original value must survive, unmodified.
	assertMergedKeyValue(t, result, "path", "../_board")

	if len(result.Preserved) != 1 || result.Preserved[0] != "path" {
		t.Errorf("SetValues() Preserved = %v; want [\"path\"]", result.Preserved)
	}

	merged := string(result.Merged)
	if !strings.Contains(merged, "# preserved (not in current template)") {
		t.Errorf("SetValues() merged missing preserved marker comment; got %q", merged)
	}

	// The preserved key must be appended after every template key.
	idxKey1 := strings.Index(merged, "key1")
	idxKey2 := strings.Index(merged, "key2")
	idxPath := strings.Index(merged, "path")
	if idxPath < idxKey1 || idxPath < idxKey2 {
		t.Errorf("SetValues() preserved key does not appear after every template key; merged = %q", merged)
	}
}

// TestSetValues_PreservesMultipleUnknownKeysSorted verifies that when existing
// has multiple top-level orphan keys given in non-alphabetical order,
// SetResult.Preserved is sorted alphabetically and every orphan survives.
func TestSetValues_PreservesMultipleUnknownKeysSorted(t *testing.T) {
	template := []byte("key1: default1\n")
	existing := []byte("key1: user_val1\nzebra: z_val\napple: a_val\nmango: m_val\n")

	result, err := SetValues(template, existing, nil)
	if err != nil {
		t.Fatalf("SetValues() unexpected error: %v", err)
	}

	want := []string{"apple", "mango", "zebra"}
	if len(result.Preserved) != len(want) {
		t.Fatalf("SetValues() Preserved = %v; want %v", result.Preserved, want)
	}
	for i, key := range want {
		if result.Preserved[i] != key {
			t.Errorf("SetValues() Preserved[%d] = %q; want %q", i, result.Preserved[i], key)
		}
	}

	assertMergedKeyValue(t, result, "apple", "a_val")
	assertMergedKeyValue(t, result, "mango", "m_val")
	assertMergedKeyValue(t, result, "zebra", "z_val")
}

// TestSetValues_NoPreservedWhenAllKeysKnown is an explicit regression guard
// for the new Preserved field on the ordinary, no-orphan path: when every
// key in existing is already present in the template, nothing is grafted.
func TestSetValues_NoPreservedWhenAllKeysKnown(t *testing.T) {
	template := []byte("key1: default1\nkey2: default2\n")
	existing := []byte("key1: user_val1\nkey2: user_val2\n")

	result, err := SetValues(template, existing, []KV{{Key: "key1", Value: "new_val1"}})
	if err != nil {
		t.Fatalf("SetValues() unexpected error: %v", err)
	}

	if len(result.Preserved) != 0 {
		t.Errorf("SetValues() Preserved = %v; want none", result.Preserved)
	}
	if strings.Contains(string(result.Merged), "# preserved (not in current template)") {
		t.Errorf("SetValues() merged unexpectedly contains preserved marker comment; got %q", result.Merged)
	}
}

// TestSetValues_PreservedKeyIdempotent proves that the marker-comment-set-
// not-appended rule makes a preserving --set idempotent: calling SetValues
// again with existing set to the first call's Merged must reproduce the
// same Merged bytes and the same Preserved list, with no comment growth.
func TestSetValues_PreservedKeyIdempotent(t *testing.T) {
	template := []byte("key1: default1\n")
	existing := []byte("key1: user_val1\npath: ../_board\n")
	pairs := []KV{{Key: "key1", Value: "new_val1"}}

	first, err := SetValues(template, existing, pairs)
	if err != nil {
		t.Fatalf("SetValues() first call unexpected error: %v", err)
	}

	second, err := SetValues(template, first.Merged, pairs)
	if err != nil {
		t.Fatalf("SetValues() second call unexpected error: %v", err)
	}

	if string(second.Merged) != string(first.Merged) {
		t.Errorf("SetValues() second call Merged = %q; want identical to first call Merged %q", second.Merged, first.Merged)
	}

	if len(second.Preserved) != len(first.Preserved) {
		t.Fatalf("SetValues() second call Preserved = %v; want identical to first call Preserved %v", second.Preserved, first.Preserved)
	}
	for i := range first.Preserved {
		if second.Preserved[i] != first.Preserved[i] {
			t.Errorf("SetValues() second call Preserved[%d] = %q; want %q", i, second.Preserved[i], first.Preserved[i])
		}
	}
}

// TestSetValues_PreservesNonFlatOrphanWhole verifies root-key-granularity
// preservation handles a non-flat orphan (a nested mapping under a top-level
// key absent from the template) without any special-case logic: the whole
// subtree survives verbatim, and Preserved records only the top-level key
// name, not a flattened dotted path into the nested structure.
func TestSetValues_PreservesNonFlatOrphanWhole(t *testing.T) {
	template := []byte("key1: default1\n")
	existing := []byte("key1: user_val1\nextra:\n  nested: value\n")

	result, err := SetValues(template, existing, nil)
	if err != nil {
		t.Fatalf("SetValues() unexpected error: %v", err)
	}

	if len(result.Preserved) != 1 || result.Preserved[0] != "extra" {
		t.Errorf("SetValues() Preserved = %v; want [\"extra\"]", result.Preserved)
	}

	merged := string(result.Merged)
	if !strings.Contains(merged, "extra:") || !strings.Contains(merged, "nested: value") {
		t.Errorf("SetValues() merged does not preserve nested orphan structure verbatim; got %q", merged)
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
