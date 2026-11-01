// bootstrapverb_test.go covers BootstrapVerb: it asserts batten's capability declaration explicitly
// against the empty string, so a future edit filling the constant in must break this test and force a
// human to confirm batten really did grow a bootstrap verb.

package battencli

import "testing"

import "github.com/Knatte18/loomyard/internal/testkit/tiergate"

// TestBootstrapVerb_IsEmpty asserts BootstrapVerb -- the capability declaration under test -- is exactly the empty string.
//
//testtiming:keep a guard that fires when BootstrapVerb is filled in, which no covering test asserts
func TestBootstrapVerb_IsEmpty(t *testing.T) {
	tiergate.Require(t, 0)
	if BootstrapVerb != "" {
		t.Errorf("BootstrapVerb = %q; want %q", BootstrapVerb, "")
	}
}
