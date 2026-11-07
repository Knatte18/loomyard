// cli_test.go covers the reedcli cobra seam through RunCLI:
// watchdog's and switch's flag refusals and the not-a-git-repo error surface.
// No test in this file needs a live tmux session;
// the real up/add/status/down round-trip lives in smoke_test.go behind //go:build tmux || llm.
// Config resolution against a real fixture hub is tested in cli_integration_test.go.
// Every test here calls t.Chdir, which is process-global state, so none calls t.Parallel.

package reedcli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/envelope"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestRunCLI_Watchdog_RefusesBadHubPath verifies that watchdog refuses an absent or relative --hub-path.
// It refuses on the envelope, before the verb blocks.
// The non-git row also checks that the verb runs with no git repository present.
// PersistentPreRunE returns early for watchdog.
// The verb's own RunE then reports the missing --hub-path flag, not lymcwd.Resolve's not-a-git-repository error.
func TestRunCLI_Watchdog_RefusesBadHubPath(t *testing.T) {
	tiergate.Require(t, 0)
	tests := []struct {
		name string
		args []string
		// nonGitDir runs the verb from a directory outside any git repository.
		nonGitDir bool
	}{
		{name: "absent", args: []string{"watchdog", "--tmux", "/usr/bin/tmux"}},
		{name: "relative", args: []string{"watchdog", "--hub-path", "relative/path", "--tmux", "/usr/bin/tmux"}},
		{name: "absent from a non-git directory", args: []string{"watchdog"}, nonGitDir: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.nonGitDir {
				t.Chdir(t.TempDir())
			}

			var out bytes.Buffer
			exitCode := RunCLI(&out, tt.args)

			if exitCode == 0 {
				t.Fatalf("RunCLI(%v) = 0; want non-zero", tt.args)
			}
			errMsg := envelope.Decode(t, out.String()).Error
			if errMsg == "not a git repository" {
				t.Errorf("RunCLI(%v) error = %q; want the --hub-path validation error, not lymcwd.Resolve's not-a-git-repository error", tt.args, errMsg)
			}
			if !strings.Contains(errMsg, "--hub-path") {
				t.Errorf("RunCLI(%v) error = %q; want it to name --hub-path", tt.args, errMsg)
			}
		})
	}
}

// TestRunCLI_NotAGitRepo verifies that a real verb run from a non-git directory exits 1.
// Its JSON error reads "not a git repository".
func TestRunCLI_NotAGitRepo(t *testing.T) {
	tiergate.Require(t, 0)
	t.Chdir(t.TempDir())

	var out bytes.Buffer
	exitCode := RunCLI(&out, []string{"status"})

	if exitCode != 1 {
		t.Errorf("RunCLI(status) in non-git dir = %d; want 1", exitCode)
	}

	if errMsg := envelope.Decode(t, out.String()).Error; errMsg != "not a git repository" {
		t.Errorf("RunCLI(status) error = %q; want exactly \"not a git repository\"", errMsg)
	}
}

// TestRunCLI_Switch_RefusesBadFlagsAndNeedsNoRepository verifies the flag checks of switch, follow and button.
// Switch refuses a missing or doubled direction and a missing --client.
// Follow refuses a missing --socket, --session or --tmux.
// Button refuses a missing --socket, --tmux, --session, --range or --client.
// It also refuses --confirmed or --answered without --hash.
// Each refusal is a JSON error with a non-zero exit.
// Button with every flag and a socket that does not exist exits 0 with no output.
// It runs outside a git repository, so a refusal naming its flag proves the verb skips the root pre-run's resolution.
func TestRunCLI_Switch_RefusesBadFlagsAndNeedsNoRepository(t *testing.T) {
	tiergate.Require(t, 0)
	tests := []struct {
		name    string
		args    []string
		wantErr string
		// wantOK marks a call that passes its flag checks: it exits 0 and prints nothing.
		wantOK bool
	}{
		{name: "neither direction", args: []string{"switch", "--socket", "/tmp/s", "--client", "c", "--tmux", "tmux"}, wantErr: "--next and --prev"},
		{name: "both directions", args: []string{"switch", "--next", "--prev", "--socket", "/tmp/s", "--client", "c", "--tmux", "tmux"}, wantErr: "--next and --prev"},
		{name: "missing client", args: []string{"switch", "--next", "--socket", "/tmp/s", "--tmux", "tmux"}, wantErr: "--client"},
		{name: "follow missing session", args: []string{"follow", "--socket", "/tmp/s", "--tmux", "tmux"}, wantErr: "--session"},
		{name: "follow missing socket", args: []string{"follow", "--session", "s", "--tmux", "tmux"}, wantErr: "--socket"},
		{name: "follow missing tmux", args: []string{"follow", "--socket", "/tmp/s", "--session", "s"}, wantErr: "--tmux"},
		{name: "button missing socket", args: []string{"button", "--tmux", "tmux", "--session", "s", "--range", "lymbtn0", "--client", "c"}, wantErr: "--socket"},
		{name: "button missing tmux", args: []string{"button", "--socket", "/tmp/s", "--session", "s", "--range", "lymbtn0", "--client", "c"}, wantErr: "--tmux"},
		{name: "button missing session", args: []string{"button", "--socket", "/tmp/s", "--tmux", "tmux", "--range", "lymbtn0", "--client", "c"}, wantErr: "--session"},
		{name: "button missing range", args: []string{"button", "--socket", "/tmp/s", "--tmux", "tmux", "--session", "s", "--client", "c"}, wantErr: "--range"},
		{name: "button missing client", args: []string{"button", "--socket", "/tmp/s", "--tmux", "tmux", "--session", "s", "--range", "lymbtn0"}, wantErr: "--client"},
		{name: "button confirmed without hash", args: []string{"button", "--socket", "/tmp/s", "--tmux", "tmux", "--session", "s", "--range", "lymbtn0", "--client", "c", "--confirmed"}, wantErr: "--hash"},
		{name: "button answered without hash", args: []string{"button", "--socket", "/tmp/s", "--tmux", "tmux", "--session", "s", "--range", "lymbtn0", "--client", "c", "--answered"}, wantErr: "--hash"},
		{name: "button with every flag needs no repository", args: []string{"button", "--socket", "/nonexistent/lym-socket", "--tmux", "/nonexistent/lym-tmux", "--session", "s", "--range", "lymbtn0", "--client", "c"}, wantOK: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Chdir(t.TempDir())

			var out bytes.Buffer
			exitCode := RunCLI(&out, tt.args)

			if tt.wantOK {
				if exitCode != 0 || out.Len() != 0 {
					t.Errorf("RunCLI(%v) = %d with output %q; want exit 0 and no output", tt.args, exitCode, out.String())
				}
				return
			}
			if exitCode == 0 {
				t.Fatalf("RunCLI(%v) = 0; want non-zero", tt.args)
			}
			if errMsg := envelope.Decode(t, out.String()).Error; !strings.Contains(errMsg, tt.wantErr) {
				t.Errorf("RunCLI(%v) error = %q; want it to contain %q", tt.args, errMsg, tt.wantErr)
			}
		})
	}
}
