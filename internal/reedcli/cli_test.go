// cli_test.go covers the reedcli cobra seam through RunCLI.
// It covers watchdog's and switch's flag refusals and the not-a-git-repo error surface.
// It also covers the refusal of attach and up in fabric's own checkouts, and that down is not refused there.
// No test in this file needs a live tmux session;
// the real up/add/status/down round-trip lives in smoke_test.go behind //go:build tmux || llm.
// Config resolution against a real fixture hub is tested in cli_integration_test.go.
// Every test that calls t.Chdir, which is process-global state, skips t.Parallel.

package reedcli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lymcwd"
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

// TestRunCLI_AttachAndUp_RefuseFabricCheckouts pins the refusal of attach and up in fabric's own checkouts.
// Those are the hub's Bolt checkout and a records sibling.
// Each refusal exits 1 and names the verb, fabric's reason and the hub to run it from.
// The checkouts hold no config, so a refusal proves it comes before the config read and any tmux call.
func TestRunCLI_AttachAndUp_RefuseFabricCheckouts(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	hub := filepath.Join(t.TempDir(), "repo-LYMHUB")
	recordsSibling := filepath.Base(fabricengine.RecordsWorktree(&lymcwd.Location{HubPath: hub, WorktreeName: "prime"}))
	for _, checkout := range []string{lymcwd.BoltDirName, recordsSibling} {
		writeBareGitDir(t, filepath.Join(hub, checkout))
	}
	resolvedHub, err := filepath.EvalSymlinks(hub)
	if err != nil {
		t.Fatalf("resolve the hub path: %v", err)
	}

	tests := []struct {
		name     string
		verb     string
		checkout string
	}{
		{name: "AttachInBolt", verb: "attach", checkout: lymcwd.BoltDirName},
		{name: "UpInBolt", verb: "up", checkout: lymcwd.BoltDirName},
		{name: "AttachInRecordsSibling", verb: "attach", checkout: recordsSibling},
		{name: "UpInRecordsSibling", verb: "up", checkout: recordsSibling},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var out bytes.Buffer
			exitCode := RunCLIIn(filepath.Join(hub, tt.checkout), &out, []string{tt.verb})

			if exitCode != 1 {
				t.Errorf("RunCLIIn(%s) in %s = %d; want 1", tt.verb, tt.checkout, exitCode)
			}
			errMsg := envelope.Decode(t, out.String()).Error
			fabricRefusal := fabricengine.RequireDrivableWorktree(&lymcwd.Location{HubPath: resolvedHub, WorktreeName: tt.checkout})
			if fabricRefusal == nil {
				t.Fatalf("fabric accepts %s as a drivable worktree; want a refusal", tt.checkout)
			}
			for _, want := range []string{
				"reed: lym reed " + tt.verb + " " + fabricRefusalText,
				fabricRefusal.Error(),
				"way forward: run it from the hub's prime or a task worktree under " + resolvedHub,
			} {
				if !strings.Contains(errMsg, want) {
					t.Errorf("RunCLIIn(%s) in %s error = %q; want it to contain %q", tt.verb, tt.checkout, errMsg, want)
				}
			}
		})
	}
}

// TestRunCLI_Down_IsNotRefusedInBolt pins that down still runs in the Bolt checkout, where attach and up are refused.
// It goes on to fail for want of config or print nothing, so the assertion is only that the refusal text is absent.
func TestRunCLI_Down_IsNotRefusedInBolt(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	hub := filepath.Join(t.TempDir(), "repo-LYMHUB")
	writeBareGitDir(t, filepath.Join(hub, lymcwd.BoltDirName))

	var out bytes.Buffer
	RunCLIIn(filepath.Join(hub, lymcwd.BoltDirName), &out, []string{"down"})

	if strings.Contains(out.String(), fabricRefusalText) {
		t.Errorf("RunCLIIn(down) in Bolt output = %q; want no refusal %q", out.String(), fabricRefusalText)
	}
}

// fabricRefusalText is the part of reed's refusal of attach and up that every refused checkout shares.
const fabricRefusalText = "boots a session only in a worktree of the hub"

// writeBareGitDir makes dir a git worktree root with the least .git directory a repository read accepts, running no git.
func writeBareGitDir(t *testing.T, dir string) {
	t.Helper()
	gitDir := filepath.Join(dir, ".git")
	for _, sub := range []string{"objects", "refs"} {
		if err := os.MkdirAll(filepath.Join(gitDir, sub), 0o755); err != nil {
			t.Fatalf("create %s: %v", sub, err)
		}
	}
	if err := os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte("ref: refs/heads/main\n"), 0o644); err != nil {
		t.Fatalf("write HEAD: %v", err)
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
