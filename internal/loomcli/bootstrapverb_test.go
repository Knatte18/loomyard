// bootstrapverb_test.go covers BootstrapVerb: that it is exactly "start", and that it equals the
// Use field of the cobra command startCmd builds -- the assertion that catches a rename of the verb
// that leaves the constant behind.

package loomcli

import "testing"

// TestBootstrapVerb_IsExactlyStart asserts BootstrapVerb is exactly "start".
func TestBootstrapVerb_IsExactlyStart(t *testing.T) {
	if BootstrapVerb != "start" {
		t.Errorf("BootstrapVerb = %q; want %q", BootstrapVerb, "start")
	}
}

// TestBootstrapVerb_MatchesStartCmdUse asserts BootstrapVerb equals the Use field of the cobra
// command startCmd builds, so a rename of the verb cannot leave the constant behind.
func TestBootstrapVerb_MatchesStartCmdUse(t *testing.T) {
	c := newLoomCLI()
	cmd := c.startCmd()
	if cmd.Use != BootstrapVerb {
		t.Errorf("startCmd().Use = %q; want it to equal BootstrapVerb = %q", cmd.Use, BootstrapVerb)
	}
}
