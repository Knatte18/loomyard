// bootstrapverb_test.go covers BootstrapVerb: it asserts batten's capability declaration explicitly
// against the empty string, so a future edit filling the constant in must break this test and force a
// human to confirm batten really did grow a bootstrap verb.

package battencli

import "testing"

// TestBootstrapVerb_IsEmpty asserts BootstrapVerb -- the capability declaration under test -- is
// exactly the empty string.
func TestBootstrapVerb_IsEmpty(t *testing.T) {
	if BootstrapVerb != "" {
		t.Errorf("BootstrapVerb = %q; want %q", BootstrapVerb, "")
	}
}
