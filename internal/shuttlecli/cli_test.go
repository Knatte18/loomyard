// cli_test.go covers the shuttlecli cobra seam through RunCLI: bare-group
// listing, the unknown-subcommand JSON envelope, and run's flag-shape
// validation. No live psmux/claude session is required by any test in this
// file; the full run/interrupt/send round-trip against a live agent lives in
// smoke tests (batch 6) and the sandbox suite.

package shuttlecli

import (
	"bytes"
	"strings"
	"testing"
)

// TestRunCLI_NoArgs verifies that "lyx shuttle" with no subcommand lists the
// run verb and exits 0 — no git repo is needed, since the PersistentPreRunE
// guard skips layout/config/engine resolution for the group command itself.
func TestRunCLI_NoArgs(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	exitCode := RunCLI(&out, nil)

	if exitCode != 0 {
		t.Errorf("RunCLI(nil) = %d; want 0", exitCode)
	}

	got := out.String()
	wantSubs := []string{"run"}
	for _, sub := range wantSubs {
		if !strings.Contains(got, sub) {
			t.Errorf("RunCLI(nil) no-arg listing missing subcommand %q; got:\n%s", sub, got)
		}
	}
}

// TestRunCLI_UnknownSubcommand verifies that an unknown subcommand exits 1
// and emits a JSON error envelope with ok=false, without needing a git repo
// (the PersistentPreRunE guard for cmd.Name() == "shuttle" fires before
// layout resolution).
func TestRunCLI_UnknownSubcommand(t *testing.T) {
	t.Chdir(t.TempDir())

	var out bytes.Buffer
	exitCode := RunCLI(&out, []string{"bogus"})

	if exitCode != 1 {
		t.Errorf("RunCLI(bogus) = %d; want 1", exitCode)
	}

	got := out.String()
	if !strings.Contains(got, `"ok":false`) {
		t.Errorf("RunCLI(bogus) output missing ok:false envelope; got: %q", got)
	}
	if !strings.Contains(got, "unknown") {
		t.Errorf("RunCLI(bogus) output missing \"unknown\"; got: %q", got)
	}
}

// TestRunCLI_Run_FlagValidation exercises run's flag-shape validation
// (missing --output-file, both --prompt and --prompt-file, neither) against
// an uninitialized (non-git) directory. Config resolution aborts first in
// that directory, but run's RunE validates flag shape before ever touching
// c.runner, so each case's flag-specific error still surfaces in the
// captured output alongside the PersistentPreRunE abort's own error line.
func TestRunCLI_Run_FlagValidation(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{
			name:    "MissingOutputFile",
			args:    []string{"run", "--prompt", "do the thing"},
			wantErr: "--output-file",
		},
		{
			name:    "BothPromptAndPromptFile",
			args:    []string{"run", "--prompt", "do the thing", "--prompt-file", "task.md", "--output-file", "out.md"},
			wantErr: "mutually exclusive",
		},
		{
			name:    "NeitherPromptNorPromptFile",
			args:    []string{"run", "--output-file", "out.md"},
			wantErr: "exactly one of --prompt or --prompt-file",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Chdir(t.TempDir())

			var out bytes.Buffer
			exitCode := RunCLI(&out, tt.args)

			if exitCode != 1 {
				t.Errorf("RunCLI(%v) = %d; want 1", tt.args, exitCode)
			}
			if !strings.Contains(out.String(), tt.wantErr) {
				t.Errorf("RunCLI(%v) output = %q; want substring %q", tt.args, out.String(), tt.wantErr)
			}
		})
	}
}
