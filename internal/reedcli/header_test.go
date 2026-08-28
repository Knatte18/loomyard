// header_test.go covers the `header` verb's pure command construction: Use, Short, and the
// --blocking flag registration, plus headerBlockingPayload's clear-sequence bytes.
// It never runs RunE/PreRunE and never invokes the --blocking path, since that path blocks forever
// by design — the payload it writes is pinned instead by driving the pure helper directly.
// It also never drives the enveloped default through RunCLI: that reaches reed's PersistentPreRunE
// and therefore lyxcwd.Resolve, which spawns "git rev-parse", banned in the untagged suite by the
// Test Tier Purity Invariant.
// The enveloped default's end-to-end PreRunE -> HeaderText round trip is covered by the reed smoke
// suite (batch 4), not here.

package reedcli

import "testing"

func TestHeaderCmd_UseAndShort(t *testing.T) {
	c := &reedCLI{}
	cmd := c.headerCmd()

	if cmd.Use != "header" {
		t.Errorf("headerCmd().Use = %q; want %q", cmd.Use, "header")
	}
	if cmd.Short == "" {
		t.Error("headerCmd().Short is empty; want a non-empty short description")
	}
}

func TestHeaderCmd_BlockingFlagRegistered(t *testing.T) {
	c := &reedCLI{}
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

func TestHeaderBlockingPayload(t *testing.T) {
	const clearSeq = "\x1b[2J\x1b[3J\x1b[H"

	tests := []struct {
		name string
		text string
		want string
	}{
		{
			name: "TrailingCRLFTrimmed",
			text: "hub: /some/path\r\n",
			want: clearSeq + "hub: /some/path",
		},
		{
			name: "NoTrailingNewlineUnchanged",
			text: "hub: /some/path",
			want: clearSeq + "hub: /some/path",
		},
		{
			name: "InteriorNewlinesPreserved",
			text: "line one\nline two\n\n",
			want: clearSeq + "line one\nline two",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := headerBlockingPayload(tt.text)
			if got != tt.want {
				t.Errorf("headerBlockingPayload(%q) = %q; want %q", tt.text, got, tt.want)
			}
		})
	}
}
