//go:build integration

// cli_integration_test.go holds the reedcli tests that build a real fixture hub (hubforge.NewHub)
// with reed config resolution against a real fixture hub, so this file is integration-tagged per
// the Test Tier Purity Invariant.

package reedcli

import (
	"bytes"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/testkit/envelope"
)

// TestRunCLI_ResolvesLayoutAndConfig builds a real fixture hub and verifies reed config resolution
// succeeds — reed's registered ConfigTemplate() arrives already materialized via
// fabriccli.CloneAndWire, so no explicit seeding is needed.
func TestRunCLI_ResolvesLayoutAndConfig(t *testing.T) {
	t.Parallel()

	h := hubforge.NewHub(t, ".")

	var out bytes.Buffer
	exitCode := RunCLIIn(h.PrimeWorktree(), &out, []string{"status"})

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
}

// coldAddLaunchCmd returns an OS-appropriate long-running --cmd for the cold-add scenario, so the
// strand's pane stays alive long enough for the follow-up `status` to report it live rather than
// racing a command that exits at once.
func coldAddLaunchCmd() string {
	if runtime.GOOS == "windows" {
		return "pwsh -NoExit -Command Write-Host ready"
	}
	return "sleep 300"
}

// skipWithoutMultiplexer skips the calling test when the configured multiplexer binary is absent,
// the same self-skip reedengine's own integration fixtures apply, so a -tags=integration run on a
// machine without the tool never hard-fails on a scenario that has to boot a real session.
func skipWithoutMultiplexer(t *testing.T, h *hubforge.Hub) {
	t.Helper()
	cfg, err := reedengine.LoadConfig(h.Location.AnchorPath(), "reed")
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if _, err := exec.LookPath(cfg.Tmux); err != nil {
		t.Skipf("configured multiplexer binary %q not found: %v", cfg.Tmux, err)
	}
}

// TestRunCLI_AddNotUp_SelfHealsAndSucceeds verifies that running `add` before `up` no longer refuses
// with the friendly "no reed session" error: add boots this worktree's session itself, exits zero with
// the ordinary guid-and-name envelope, and the `status` verb -- which still refuses on a cold worktree
// (TestRunCLI_ResolvesLayoutAndConfig) -- then succeeds against the session add deposited, naming this
// worktree's own socket and session and listing the new strand live. It is the integration-tier twin
// of smoke_coldstart_test.go's TestSmokeColdAddBootsAndAddsInOneCall, written against this file's
// in-process RunCLIIn fixture rather than the smoke tier's multiplexer probes.
func TestRunCLI_AddNotUp_SelfHealsAndSucceeds(t *testing.T) {
	t.Parallel()

	h := hubforge.NewHub(t, ".")
	skipWithoutMultiplexer(t, h)
	worktree := h.PrimeWorktree()
	t.Cleanup(func() {
		// Best-effort: the session add booted must not outlive the test, and down has nothing to
		// tear down when add failed before booting anything.
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
}

// TestRunCLI_AddIfAbsentNoName_RejectsBeforeSessionCheck verifies that `add --if-absent` with no
// --name is rejected by the engine's --name requirement, and specifically BEFORE the session pre-flight
// that would otherwise self-heal the cold worktree (TestRunCLI_AddNotUp_SelfHealsAndSucceeds): against
// a fixture hub with no session up, the error must name the --name requirement, never the "no reed
// session" message, and a rejected call must never boot a session as residue over what is actually a
// missing --name.
func TestRunCLI_AddIfAbsentNoName_RejectsBeforeSessionCheck(t *testing.T) {
	t.Parallel()

	h := hubforge.NewHub(t, ".")

	var out bytes.Buffer
	exitCode := RunCLIIn(h.PrimeWorktree(), &out, []string{"add", "--if-absent", "--cmd", "pwsh -NoExit -Command Write-Host ready"})

	if exitCode != 1 {
		t.Errorf("RunCLI(add --if-absent, no --name) = %d; want 1", exitCode)
	}

	errMsg := envelope.Decode(t, out.String()).Error
	if !strings.Contains(errMsg, "--name") {
		t.Errorf("RunCLI(add --if-absent, no --name) error = %q; want it to name the --name requirement", errMsg)
	}
	noSessionMsg := `no reed session; run "lyx reed up"`
	if errMsg == noSessionMsg {
		t.Errorf("RunCLI(add --if-absent, no --name) error = %q; want the --name rejection to precede the session check, not the no-session message", errMsg)
	}

	// The rejection must have fired before the self-heal pre-flight: status still refuses, proving
	// no session was booted as residue.
	out.Reset()
	if code := RunCLIIn(h.PrimeWorktree(), &out, []string{"status"}); code != 1 {
		t.Errorf("RunCLI(status) after the rejected add = %d; want 1 (the --name rejection must precede the session pre-flight, booting nothing), output: %s", code, out.String())
	}
}

// TestRunCLI_AddIfAbsentNoCmd_StillRequiresCmd verifies that --if-absent relaxes nothing about --cmd:
// it stays required, so `add --if-absent --name claude` with no --cmd must still fail with cobra's
// missing-required-flag error.
func TestRunCLI_AddIfAbsentNoCmd_StillRequiresCmd(t *testing.T) {
	t.Parallel()

	h := hubforge.NewHub(t, ".")

	var out bytes.Buffer
	exitCode := RunCLIIn(h.PrimeWorktree(), &out, []string{"add", "--if-absent", "--name", "claude"})

	if exitCode != 1 {
		t.Errorf("RunCLI(add --if-absent, no --cmd) = %d; want 1", exitCode)
	}

	errMsg := envelope.Decode(t, out.String()).Error
	if !strings.Contains(errMsg, `"cmd"`) {
		t.Errorf("RunCLI(add --if-absent, no --cmd) error = %q; want it to name the missing required --cmd flag", errMsg)
	}
}

// TestRunCLI_RemoveNotUp_FriendlyError verifies that running `remove` before `up` surfaces the
// friendly "no reed session" error.
func TestRunCLI_RemoveNotUp_FriendlyError(t *testing.T) {
	t.Parallel()

	h := hubforge.NewHub(t, ".")

	var out bytes.Buffer
	exitCode := RunCLIIn(h.PrimeWorktree(), &out, []string{"remove", "does-not-exist"})

	if exitCode != 1 {
		t.Errorf("RunCLI(remove) before up = %d; want 1 (no live tmux session)", exitCode)
	}

	wantErr := `no reed session; run "lyx reed up"`
	if errMsg := envelope.Decode(t, out.String()).Error; errMsg != wantErr {
		t.Errorf("RunCLI(remove) before up error = %q; want %q", errMsg, wantErr)
	}
}

// TestRunCLI_StatusNotUp_EnrichedResumeHint verifies that running `status` before `up` with
// persisted strands surfaces the enriched "lyx reed resume" message.
func TestRunCLI_StatusNotUp_EnrichedResumeHint(t *testing.T) {
	t.Parallel()

	h := hubforge.NewHub(t, ".")

	st := &reedengine.ReedState{
		Socket:  "test-socket",
		Session: "test-session",
		Strands: []reedengine.Strand{
			{GUID: "strand-one", Name: "one", Worktree: h.Location.WorktreePath(), Cmd: "true"},
			{GUID: "strand-two", Name: "two", Worktree: h.Location.WorktreePath(), Cmd: "true"},
		},
	}
	if err := reedengine.SaveState(filepath.Join(h.Location.WorktreePath(), ".lyx"), st); err != nil {
		t.Fatalf("SaveState: %v", err)
	}

	var out bytes.Buffer
	exitCode := RunCLIIn(h.PrimeWorktree(), &out, []string{"status"})

	if exitCode != 1 {
		t.Errorf("RunCLI(status) before up = %d; want 1 (no live tmux session)", exitCode)
	}

	wantErr := `no reed session (2 strands persisted); run "lyx reed resume" to rebuild, or "lyx reed up" for a bare substrate`
	if errMsg := envelope.Decode(t, out.String()).Error; errMsg != wantErr {
		t.Errorf("RunCLI(status) before up error = %q; want %q", errMsg, wantErr)
	}
}
