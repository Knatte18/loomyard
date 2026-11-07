// bootstrapverb_test.go pins BootstrapVerb, batten's capability declaration, to the empty string.
// A future edit that fills it in breaks this test, so a human confirms batten grew a bootstrap verb.

package battencli

import "testing"

import "github.com/Knatte18/loomyard/internal/testkit/tiergate"

// TestBootstrapVerb_IsEmpty asserts that BootstrapVerb is exactly the empty string.
//
//testtiming:keep a guard that fires when BootstrapVerb is filled in, which no covering test asserts
func TestBootstrapVerb_IsEmpty(t *testing.T) {
	tiergate.Require(t, 0)
	if BootstrapVerb != "" {
		t.Errorf("BootstrapVerb = %q; want %q", BootstrapVerb, "")
	}
}
