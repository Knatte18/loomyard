package muxpoc

import (
	"bytes"
	"testing"
)

// These exercise the dispatch/parse error paths only — they never reach a
// subcommand that shells out to psmux, so they are safe to run without it.

func TestRunCLINoSubcommandFails(t *testing.T) {
	var out bytes.Buffer
	if code := RunCLI(&out, nil); code != 1 {
		t.Errorf("RunCLI(nil) = %d, want 1", code)
	}
	if out.Len() != 0 {
		t.Errorf("usage error wrote to stdout: %q", out.String())
	}
}

func TestRunCLIUnknownSubcommandFails(t *testing.T) {
	var out bytes.Buffer
	if code := RunCLI(&out, []string{"bogus"}); code != 1 {
		t.Errorf("RunCLI(bogus) = %d, want 1", code)
	}
	if out.Len() != 0 {
		t.Errorf("unknown subcommand wrote to stdout: %q", out.String())
	}
}

func TestRunCLIUnknownFlagFails(t *testing.T) {
	var out bytes.Buffer
	if code := RunCLI(&out, []string{"--no-such-flag", "status"}); code != 1 {
		t.Errorf("RunCLI(--no-such-flag) = %d, want 1", code)
	}
}
