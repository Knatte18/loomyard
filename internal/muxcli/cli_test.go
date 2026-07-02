// cli_test.go covers the muxcli cobra seam through RunCLI: bare-group
// listing, the unknown-subcommand JSON envelope, config resolution against
// a real fixture hub, and the built attach invocation. No live psmux
// session is required by any test in this file; the real up/add/status/down
// round-trip lives in smoke_test.go behind //go:build smoke.

package muxcli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/lyxtest"
	"github.com/Knatte18/loomyard/internal/muxengine"
)

// TestRunCLI_NoArgs verifies that "lyx mux" with no subcommand lists all
// seven registered verbs and exits 0 — no git repo is needed, since the
// PersistentPreRunE guard skips layout/config resolution for the group
// command itself.
func TestRunCLI_NoArgs(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	exitCode := RunCLI(&out, nil)

	if exitCode != 0 {
		t.Errorf("RunCLI(nil) = %d; want 0", exitCode)
	}

	got := out.String()
	wantSubs := []string{"up", "down", "add", "remove", "status", "resume", "attach"}
	for _, sub := range wantSubs {
		if !strings.Contains(got, sub) {
			t.Errorf("RunCLI(nil) no-arg listing missing subcommand %q; got:\n%s", sub, got)
		}
	}
}

// TestRunCLI_UnknownSubcommand verifies that an unknown subcommand exits 1
// and emits a JSON error envelope with ok=false, without needing a git repo
// (the PersistentPreRunE guard for cmd.Name() == "mux" fires before layout
// resolution).
func TestRunCLI_UnknownSubcommand(t *testing.T) {
	t.Chdir(t.TempDir())

	var out bytes.Buffer
	exitCode := RunCLI(&out, []string{"bogus"})

	if exitCode != 1 {
		t.Errorf("RunCLI(bogus) = %d; want 1", exitCode)
	}

	var env map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out.String())), &env); err != nil {
		t.Fatalf("RunCLI(bogus) output is not valid JSON: %v; got: %q", err, out.String())
	}
	if ok, _ := env["ok"].(bool); ok {
		t.Errorf("RunCLI(bogus) ok = true; want false")
	}
	if errMsg, _ := env["error"].(string); !strings.Contains(errMsg, "unknown") {
		t.Errorf("RunCLI(bogus) error = %q; want \"unknown\" substring", errMsg)
	}
}

// TestRunCLI_NotAGitRepo verifies that a real verb (not the bare group)
// invoked from a non-git directory surfaces hubgeometry's bare
// ErrNotAGitRepo sentinel via the PersistentPreRunE abort path.
func TestRunCLI_NotAGitRepo(t *testing.T) {
	t.Chdir(t.TempDir())

	var out bytes.Buffer
	exitCode := RunCLI(&out, []string{"status"})

	if exitCode != 1 {
		t.Errorf("RunCLI(status) in non-git dir = %d; want 1", exitCode)
	}

	var env map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out.String())), &env); err != nil {
		t.Fatalf("RunCLI(status) output is not valid JSON: %v; got: %q", err, out.String())
	}
	if errMsg, _ := env["error"].(string); errMsg != "not a git repository" {
		t.Errorf("RunCLI(status) error = %q; want exactly \"not a git repository\"", errMsg)
	}
}

// TestRunCLI_ResolvesLayoutAndConfig seeds a real mux.yaml into a fixture
// hub's _lyx/config/ (mux config is anchored at layout.Cwd, unlike weft's
// weft-sibling-anchored config) and verifies PersistentPreRunE reaches the
// engine call rather than aborting on Getwd/Resolve/LoadConfig. Status then
// fails because no psmux server is running under this fixture's socket name
// — that failure is the point: it proves config resolution itself
// succeeded, exercising a domain-error path distinct from the no-git-repo
// path TestRunCLI_NotAGitRepo covers.
func TestRunCLI_ResolvesLayoutAndConfig(t *testing.T) {
	fixture := lyxtest.CopyPaired(t)
	lyxtest.SeedConfig(t, fixture.Hub, map[string]string{
		"mux": muxengine.ConfigTemplate(),
	})

	t.Chdir(fixture.Hub)

	var out bytes.Buffer
	exitCode := RunCLI(&out, []string{"status"})

	if exitCode != 1 {
		t.Errorf("RunCLI(status) = %d; want 1 (no live psmux session)", exitCode)
	}

	var env map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out.String())), &env); err != nil {
		t.Fatalf("RunCLI(status) output is not valid JSON: %v; got: %q", err, out.String())
	}
	if ok, _ := env["ok"].(bool); ok {
		t.Errorf("RunCLI(status) ok = true; want false (no psmux session up)")
	}

	// Guard against the wrong failure: this must NOT be a config-resolution
	// error (those paths are covered by TestRunCLI_NotAGitRepo and the
	// "not initialized" case) — this test's whole point is that config
	// resolution succeeded and the engine's own psmux check is what failed.
	errMsg, _ := env["error"].(string)
	if strings.Contains(errMsg, "not initialized") || strings.Contains(errMsg, "not a git repository") {
		t.Errorf("RunCLI(status) error = %q; want a psmux/session error, not a config-resolution error", errMsg)
	}
}

// TestAttachArgv verifies the attach invocation targets the worktree
// session: "-L <socket> attach-session -t <session>". This is the built
// attach invocation's one assertable seam — the argv build, not a JSON
// round-trip, since a real attach hands stdio to psmux (the documented
// JSON-envelope exception).
func TestAttachArgv(t *testing.T) {
	got := attachArgv("hub-abc123", "my-worktree")
	want := []string{"-L", "hub-abc123", "attach-session", "-t", "my-worktree"}

	if len(got) != len(want) {
		t.Fatalf("attachArgv() = %v; want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("attachArgv()[%d] = %q; want %q", i, got[i], want[i])
		}
	}
}
