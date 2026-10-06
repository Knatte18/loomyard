// bootstrapverb_test.go covers BootstrapVerb: that it is exactly "start", and that it equals the
// Use field of the cobra command startCmd builds -- the assertion that catches a rename of the verb
// that leaves the constant behind.

package loomcli

import "testing"

// TestBootstrapVerb asserts BootstrapVerb is exactly "start" and equals the Use field of the cobra
// command startCmd builds, so a rename of the verb cannot leave the constant behind.
//
//testtiming:keep pins BootstrapVerb being exactly start and equal to the Use of the command startCmd builds, so a rename cannot leave the constant behind; the covering test never reads the constant
func TestBootstrapVerb(t *testing.T) {
	if BootstrapVerb != "start" {
		t.Errorf("BootstrapVerb = %q; want %q", BootstrapVerb, "start")
	}
	cmd := newLoomCLI().startCmd()
	if cmd.Use != BootstrapVerb {
		t.Errorf("startCmd().Use = %q; want it to equal BootstrapVerb = %q", cmd.Use, BootstrapVerb)
	}
}
