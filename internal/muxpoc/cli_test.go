package muxpoc

import (
	"bytes"
	"testing"
)

// These exercise the dispatch/parse error paths only — they never reach a
// subcommand that shells out to psmux, so they are safe to run without it.

func TestRunCLIErrors(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"TestRunCLINoSubcommandFails", nil},
		{"TestRunCLIUnknownSubcommandFails", []string{"bogus"}},
		{"TestRunCLIUnknownFlagFails", []string{"--no-such-flag", "status"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			code := RunCLI(&out, tt.args)

			// All error cases return exit code 1
			if code != 1 {
				t.Errorf("RunCLI(%v) = %d, want 1", tt.args, code)
			}

			// All error cases write nothing to stdout
			if out.Len() != 0 {
				t.Errorf("error case wrote to stdout: %q", out.String())
			}
		})
	}
}
