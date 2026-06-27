// cli_test.go exercises the RunCLI dispatch and parse-error paths for muxpoc.
//
// These tests never reach a subcommand that shells out to psmux, so they are
// safe to run without psmux installed. The no-arg, unknown-subcommand, and
// bad-flag cases are tested separately because post-cobra they produce
// distinct exit codes and output content.
package muxpoc

import (
	"bytes"
	"strings"
	"testing"
)

// TestRunCLINoArg verifies that "lyx muxpoc" with no subcommand exits 0 and
// writes the subcommand listing to stdout. Under cobra, no-arg on a parent
// with no Run/RunE prints help and exits successfully.
func TestRunCLINoArg(t *testing.T) {
	var out bytes.Buffer
	code := RunCLI(&out, nil)

	if code != 0 {
		t.Errorf("RunCLI(nil) = %d; want 0", code)
	}
	if out.Len() == 0 {
		t.Errorf("RunCLI(nil) wrote nothing to stdout; want subcommand listing")
	}
}

// TestRunCLIUnknownSubcommandFails verifies that an unknown subcommand produces
// the "unknown command" cobra message and exits 1.
func TestRunCLIUnknownSubcommandFails(t *testing.T) {
	var out bytes.Buffer
	code := RunCLI(&out, []string{"bogus"})

	if code != 1 {
		t.Errorf("RunCLI(bogus) = %d; want 1", code)
	}
	if got := out.String(); !strings.Contains(got, "unknown command") {
		t.Errorf("RunCLI(bogus) output = %q; want \"unknown command\" substring", got)
	}
}

// TestRunCLIUnknownFlagFails verifies that a bad flag produces the "unknown flag"
// cobra message and exits 1. This is distinct from an unknown subcommand — cobra
// emits different messages for the two cases and tests must not conflate them.
func TestRunCLIUnknownFlagFails(t *testing.T) {
	var out bytes.Buffer
	code := RunCLI(&out, []string{"--no-such-flag", "status"})

	if code != 1 {
		t.Errorf("RunCLI(--no-such-flag status) = %d; want 1", code)
	}
	if got := out.String(); !strings.Contains(got, "unknown flag") {
		t.Errorf("RunCLI(--no-such-flag status) output = %q; want \"unknown flag\" substring", got)
	}
}
