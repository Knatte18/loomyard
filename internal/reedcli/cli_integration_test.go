// cli_integration_test.go holds the reedcli scenario that builds a real fixture hub and resolves reed config against it.
// The file is integration-tagged per the Test Tier Purity Invariant.

package reedcli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/testkit/envelope"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
	"github.com/Knatte18/loomyard/internal/testkit/tmuxkit"
)

// coldAddLaunchCmd returns an OS-appropriate long-running --cmd for the cold-add scenario.
// The command keeps the strand's pane alive long enough for the follow-up `status` to report it live,
// so the scenario does not race a command that exits at once.
func coldAddLaunchCmd() string {
	if runtime.GOOS == "windows" {
		return "pwsh -NoExit -Command Write-Host ready"
	}
	return "sleep 300"
}

// skipWithoutMultiplexer skips the calling test when the configured multiplexer binary is absent.
// It keeps an integration run on a machine without the tool from hard-failing on a scenario that boots a real session.
func skipWithoutMultiplexer(t *testing.T, h *hubforge.Hub) {
	t.Helper()
	cfg, err := reedengine.LoadConfig(configengine.Dirs{Anchor: h.Location.AnchorPath()}, "reed")
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if _, err := exec.LookPath(cfg.Tmux); err != nil {
		t.Skipf("configured multiplexer binary %q not found: %v", cfg.Tmux, err)
	}
	// Registered here, before the calling test's own teardown cleanups, so it runs after them:
	// whichever server the test boots for this hub is killed and its socket file removed.
	tmuxkit.KillOnCleanup(t, cfg.Tmux, reedengine.ServerName(h.Path))
}

// TestRunCLI_ColdHub drives the verbs against one fixture hub whose prime worktree starts cold.
// reed's registered ConfigTemplate() is already materialized via fabriccli.CloneAndWire,
// so no explicit seeding is needed.
// The steps run serially in a fixed order.
// The refusal steps run first, while no session or state exists.
// The state-file step persists a state file and removes it again; the boot step runs last and boots a session.
// A later step relies on the earlier steps leaving the hub cold.
// The scenario calls t.Parallel but no step does, because every step shares the one hub.
func TestRunCLI_ColdHub(t *testing.T) {
	tiergate.Require(t, 2)
	t.Parallel()
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	worktree := h.PrimeWorktree()

	// ResolvesLayoutAndConfig verifies reed config resolution succeeds against the fixture hub.
	if !t.Run("ResolvesLayoutAndConfig", func(t *testing.T) {
		var out bytes.Buffer
		exitCode := RunCLIIn(worktree, &out, []string{"status"})

		if exitCode != 1 {
			t.Errorf("RunCLI(status) = %d; want 1 (no live tmux session)", exitCode)
		}

		env := envelope.Decode(t, out.String())
		if env.OK {
			t.Errorf("RunCLI(status) ok = true; want false (no tmux session up)")
		}

		errMsg := env.Error
		if strings.Contains(errMsg, "not initialized") || strings.Contains(errMsg, "not a git repository") {
			t.Errorf("RunCLI(status) error = %q; want a tmux/session error, not a config-resolution error", errMsg)
		}
	}) {
		return
	}

	// AddIfAbsentNoName_RejectsBeforeSessionCheck verifies that `add --if-absent` without --name is rejected.
	// The rejection must come before the session pre-flight.
	// That pre-flight would otherwise self-heal the cold worktree by booting a session.
	// With no session up, the error must name --name, never the "no reed session" message.
	// A rejected call must boot no session.
	if !t.Run("AddIfAbsentNoName_RejectsBeforeSessionCheck", func(t *testing.T) {
		var out bytes.Buffer
		exitCode := RunCLIIn(worktree, &out, []string{"add", "--if-absent", "--cmd", "pwsh -NoExit -Command Write-Host ready"})

		if exitCode != 1 {
			t.Errorf("RunCLI(add --if-absent, no --name) = %d; want 1", exitCode)
		}

		errMsg := envelope.Decode(t, out.String()).Error
		if !strings.Contains(errMsg, "--name") {
			t.Errorf("RunCLI(add --if-absent, no --name) error = %q; want it to name the --name requirement", errMsg)
		}
		noSessionMsg := `no reed session; run "lym reed up"`
		if errMsg == noSessionMsg {
			t.Errorf("RunCLI(add --if-absent, no --name) error = %q; want the --name rejection to precede the session check, not the no-session message", errMsg)
		}

		// Status still refuses, so the rejection fired before the self-heal pre-flight and booted no session.
		out.Reset()
		if code := RunCLIIn(worktree, &out, []string{"status"}); code != 1 {
			t.Errorf("RunCLI(status) after the rejected add = %d; want 1 (the --name rejection must precede the session pre-flight, booting nothing), output: %s", code, out.String())
		}
	}) {
		return
	}

	// AddIfAbsentNoCmd_StillRequiresCmd verifies that --if-absent leaves --cmd required.
	// `add --if-absent --name claude` without --cmd fails with cobra's missing-required-flag error.
	if !t.Run("AddIfAbsentNoCmd_StillRequiresCmd", func(t *testing.T) {
		var out bytes.Buffer
		exitCode := RunCLIIn(worktree, &out, []string{"add", "--if-absent", "--name", "claude"})

		if exitCode != 1 {
			t.Errorf("RunCLI(add --if-absent, no --cmd) = %d; want 1", exitCode)
		}

		errMsg := envelope.Decode(t, out.String()).Error
		if !strings.Contains(errMsg, `"cmd"`) {
			t.Errorf("RunCLI(add --if-absent, no --cmd) error = %q; want it to name the missing required --cmd flag", errMsg)
		}
	}) {
		return
	}

	// RemoveNotUp_FriendlyError verifies that running `remove` before `up` surfaces the friendly "no reed session" error.
	if !t.Run("RemoveNotUp_FriendlyError", func(t *testing.T) {
		var out bytes.Buffer
		exitCode := RunCLIIn(worktree, &out, []string{"remove", "does-not-exist"})

		if exitCode != 1 {
			t.Errorf("RunCLI(remove) before up = %d; want 1 (no live tmux session)", exitCode)
		}

		wantErr := `no reed session; run "lym reed up"`
		if errMsg := envelope.Decode(t, out.String()).Error; errMsg != wantErr {
			t.Errorf("RunCLI(remove) before up error = %q; want %q", errMsg, wantErr)
		}
	}) {
		return
	}

	// StatusNotUp_EnrichedResumeHint verifies that `status` before `up`, with persisted strands, names `lym reed resume`.
	// It removes the state file it persists, so the next step finds the worktree cold again.
	if !t.Run("StatusNotUp_EnrichedResumeHint", func(t *testing.T) {
		stateDir := filepath.Join(h.Location.WorktreePath(), ".lym")
		st := &reedengine.ReedState{
			Socket:  "test-socket",
			Session: "test-session",
			Strands: []reedengine.Strand{
				{GUID: "strand-one", Name: "one", Worktree: h.Location.WorktreePath(), Cmd: "true"},
				{GUID: "strand-two", Name: "two", Worktree: h.Location.WorktreePath(), Cmd: "true"},
			},
		}
		if err := reedengine.SaveState(stateDir, st); err != nil {
			t.Fatalf("SaveState: %v", err)
		}
		t.Cleanup(func() {
			if err := os.Remove(filepath.Join(stateDir, "reed.json")); err != nil {
				t.Errorf("remove the persisted state: %v", err)
			}
		})

		var out bytes.Buffer
		exitCode := RunCLIIn(worktree, &out, []string{"status"})

		if exitCode != 1 {
			t.Errorf("RunCLI(status) before up = %d; want 1 (no live tmux session)", exitCode)
		}

		wantErr := `no reed session (2 strands persisted); run "lym reed resume" to rebuild, or "lym reed up" for a bare substrate`
		if errMsg := envelope.Decode(t, out.String()).Error; errMsg != wantErr {
			t.Errorf("RunCLI(status) before up error = %q; want %q", errMsg, wantErr)
		}
	}) {
		return
	}

	// AddNotUp_SelfHealsAndSucceeds verifies that `add` before `up` boots the session itself and succeeds.
	// The add exits zero with the ordinary guid-and-name envelope.
	// The `status` verb still refuses on a cold worktree, then succeeds against the session add deposited.
	// That status names this worktree's own socket and session and lists the new strand as live.
	// The test runs in-process through RunCLIIn and skips when no multiplexer is available.
	t.Run("AddNotUp_SelfHealsAndSucceeds", func(t *testing.T) {
		skipWithoutMultiplexer(t, h)
		t.Cleanup(func() {
			// Best-effort: a session that add booted must not outlive the test.
			// When add failed before booting anything, down has nothing to tear down.
			var buf bytes.Buffer
			RunCLIIn(worktree, &buf, []string{"down"})
		})

		const strandName = "cold-add"
		var out bytes.Buffer
		exitCode := RunCLIIn(worktree, &out, []string{"add", "--name", strandName, "--cmd", coldAddLaunchCmd()})

		if exitCode != 0 {
			t.Fatalf("RunCLI(add) before up = %d; want 0 (add self-heals a cold worktree), output: %s", exitCode, out.String())
		}
		env := envelope.RequireOK(t, out.String())
		guid, _ := env.Raw["guid"].(string)
		if guid == "" {
			t.Errorf("RunCLI(add) before up envelope = %s; want a non-empty guid", out.String())
		}
		if name, _ := env.Raw["name"].(string); name != "tst:"+strandName {
			t.Errorf("RunCLI(add) before up name = %q; want %q", name, "tst:"+strandName)
		}

		out.Reset()
		if code := RunCLIIn(worktree, &out, []string{"status"}); code != 0 {
			t.Fatalf("RunCLI(status) after a cold add = %d; want 0 (the session add booted must now exist), output: %s", code, out.String())
		}
		status := envelope.Decode(t, out.String())
		if session, _ := status.Raw["session"].(string); session != reedengine.SessionName(worktree) {
			t.Errorf("RunCLI(status) after a cold add session = %q; want %q (this worktree's own session)", session, reedengine.SessionName(worktree))
		}
		if socket, _ := status.Raw["socket"].(string); socket != reedengine.ServerName(h.Path) {
			t.Errorf("RunCLI(status) after a cold add socket = %q; want %q (this hub's own socket)", socket, reedengine.ServerName(h.Path))
		}

		strands, _ := status.Raw["strands"].([]any)
		found := false
		for _, s := range strands {
			strand, _ := s.(map[string]any)
			if strand["guid"] != guid {
				continue
			}
			found = true
			if live, _ := strand["live"].(bool); !live {
				t.Errorf("RunCLI(status) strand %s live = false; want true (the cold add launched it into the session it booted)", guid)
			}
		}
		if !found {
			t.Errorf("RunCLI(status) strands = %v; want the cold add's strand %s listed", strands, guid)
		}

		// The session is up, so the follow verb reaches it when told the socket path, the session and the tmux binary.
		t.Run("FollowVerbReachesTheToldSession", func(t *testing.T) {
			cfg, err := reedengine.LoadConfig(configengine.Dirs{Anchor: h.Location.AnchorPath()}, "reed")
			if err != nil {
				t.Fatalf("LoadConfig: %v", err)
			}
			tmuxPath, err := exec.LookPath(cfg.Tmux)
			if err != nil {
				t.Fatalf("resolve the tmux binary %q: %v", cfg.Tmux, err)
			}
			session := reedengine.SessionName(worktree)
			socketOut, err := exec.Command(tmuxPath, "-L", reedengine.ServerName(h.Path), "display-message", "-p", "-t", "="+session+":", "#{socket_path}").Output()
			if err != nil {
				t.Fatalf("read the server's socket path: %v", err)
			}
			socketPath := strings.TrimSpace(string(socketOut))

			var followOut bytes.Buffer
			if code := RunCLIIn(worktree, &followOut, []string{"follow", "--socket", socketPath, "--session", session, "--tmux", tmuxPath}); code != 0 || followOut.Len() != 0 {
				t.Errorf("RunCLI(follow) = %d with output %q; want 0 and no output", code, followOut.String())
			}

			followOut.Reset()
			code := RunCLIIn(worktree, &followOut, []string{"follow", "--socket", socketPath, "--session", "no-such-session", "--tmux", tmuxPath})
			if code != 1 {
				t.Errorf("RunCLI(follow, unknown session) = %d; want 1", code)
			}
			if errMsg := envelope.Decode(t, followOut.String()).Error; !strings.Contains(errMsg, "no-such-session") {
				t.Errorf("RunCLI(follow, unknown session) error = %q; want it to name the session", errMsg)
			}
		})
	})
}
