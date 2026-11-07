// bootstrapverb_test.go covers BootstrapVerb.
// It asserts the constant is exactly "start" and equals the Use field of the cobra command startCmd builds.
// That equality catches a rename of the verb that would leave the constant behind.

package loomcli

import "testing"

import "github.com/Knatte18/loomyard/internal/testkit/tiergate"

// TestBootstrapVerb asserts BootstrapVerb is exactly "start".
// It also asserts it equals the Use field of the cobra command startCmd builds.
// That keeps a rename of the verb from leaving the constant behind.
//
//testtiming:keep pins BootstrapVerb being exactly start and equal to the Use of the command startCmd builds, so a rename cannot leave the constant behind; the covering test never reads the constant
func TestBootstrapVerb(t *testing.T) {
	tiergate.Require(t, 0)
	if BootstrapVerb != "start" {
		t.Errorf("BootstrapVerb = %q; want %q", BootstrapVerb, "start")
	}
	cmd := newLoomCLI().startCmd()
	if cmd.Use != BootstrapVerb {
		t.Errorf("startCmd().Use = %q; want it to equal BootstrapVerb = %q", cmd.Use, BootstrapVerb)
	}
}
