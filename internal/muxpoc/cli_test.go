// cli_test.go exercises the RunCLI dispatch and parse-error paths for muxpoc.
//
// These tests never reach a subcommand that shells out to psmux, so they are
// safe to run without psmux installed. The no-arg, unknown-subcommand, and
// bad-flag cases are tested separately because post-cobra they produce
// distinct exit codes and output content.
package muxpoc

import (
	"bytes"
	"encoding/json"
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

// TestRunCLIUnknownSubcommandFails verifies that an unknown subcommand exits 1
// and emits a JSON error envelope with ok=false.
func TestRunCLIUnknownSubcommandFails(t *testing.T) {
	var out bytes.Buffer
	code := RunCLI(&out, []string{"bogus"})

	if code != 1 {
		t.Errorf("RunCLI(bogus) = %d; want 1", code)
	}

	// GroupRunE wraps the error in a JSON envelope; parse and assert ok=false.
	var env map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out.String())), &env); err != nil {
		t.Fatalf("RunCLI(bogus) output is not valid JSON: %v; got: %q", err, out.String())
	}
	if ok, _ := env["ok"].(bool); ok {
		t.Errorf("RunCLI(bogus) ok = true; want false")
	}
	// The error text contains "unknown" (GroupRunE produces "unknown subcommand").
	if errMsg, _ := env["error"].(string); !strings.Contains(errMsg, "unknown") {
		t.Errorf("RunCLI(bogus) error = %q; want \"unknown\" substring", errMsg)
	}
}

// TestRunCLIUnknownFlagFails verifies that a bad flag produces the "unknown flag"
// cobra message wrapped in a JSON error envelope and exits 1. This is distinct from
// an unknown subcommand — cobra emits different messages for the two cases and tests
// must not conflate them.
func TestRunCLIUnknownFlagFails(t *testing.T) {
	var out bytes.Buffer
	code := RunCLI(&out, []string{"--no-such-flag", "status"})

	if code != 1 {
		t.Errorf("RunCLI(--no-such-flag status) = %d; want 1", code)
	}

	// RunRoot wraps the cobra flag-parse error in a JSON envelope; parse and assert ok=false.
	var env map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out.String())), &env); err != nil {
		t.Fatalf("RunCLI(--no-such-flag status) output is not valid JSON: %v; got: %q", err, out.String())
	}
	if ok, _ := env["ok"].(bool); ok {
		t.Errorf("RunCLI(--no-such-flag status) ok = true; want false")
	}
	// The error field contains "unknown flag" (the cobra message for an unrecognised flag).
	if errMsg, _ := env["error"].(string); !strings.Contains(errMsg, "unknown flag") {
		t.Errorf("RunCLI(--no-such-flag status) error = %q; want \"unknown flag\" substring", errMsg)
	}
}
