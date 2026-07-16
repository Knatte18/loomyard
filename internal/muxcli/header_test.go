// header_test.go covers the `header` verb's pure command construction: Use,
// Short, and the --blocking flag registration. It never runs RunE/PreRunE
// and never invokes the --blocking path, since that path blocks forever by
// design; the enveloped default's end-to-end PreRunE -> HeaderText round
// trip is covered by the mux smoke suite (batch 4), not here.

package muxcli

import "testing"

func TestHeaderCmd_UseAndShort(t *testing.T) {
	c := &muxCLI{}
	cmd := c.headerCmd()

	if cmd.Use != "header" {
		t.Errorf("headerCmd().Use = %q; want %q", cmd.Use, "header")
	}
	if cmd.Short == "" {
		t.Error("headerCmd().Short is empty; want a non-empty short description")
	}
}

func TestHeaderCmd_BlockingFlagRegistered(t *testing.T) {
	c := &muxCLI{}
	cmd := c.headerCmd()

	flag := cmd.Flags().Lookup("blocking")
	if flag == nil {
		t.Fatal("headerCmd() did not register a --blocking flag")
	}
	if flag.Value.Type() != "bool" {
		t.Errorf("--blocking flag type = %q; want %q", flag.Value.Type(), "bool")
	}
	if flag.DefValue != "false" {
		t.Errorf("--blocking flag default = %q; want %q", flag.DefValue, "false")
	}
}
