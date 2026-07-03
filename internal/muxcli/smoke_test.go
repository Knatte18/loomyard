//go:build smoke

// smoke_test.go drives a real up -> add -> status -> down round-trip through
// RunCLI against a live psmux server. It is excluded from the default
// `go test ./internal/muxcli/...` (this batch's hermetic verify) and only
// runs under `go test -tags smoke ./internal/muxcli/...`.

package muxcli

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/lyxtest"
	"github.com/Knatte18/loomyard/internal/muxengine"
)

// psmuxBinaryPath returns the psmux binary path from the environment or the
// default install location, skipping the calling test when it is absent so a
// -tags=smoke run never hard-fails on a machine without the tool.
func psmuxBinaryPath(t *testing.T) string {
	t.Helper()
	path := os.Getenv("LYX_MUX_PSMUX")
	if path == "" {
		path = `C:\Code\tools\bin\psmux.exe`
	}
	if _, err := os.Stat(path); err != nil {
		t.Skipf("psmux not found at %s", path)
	}
	return path
}

// TestSmokeUpAddStatusDown boots the substrate, adds one strand with a cheap
// placeholder command, verifies status reports it tracked and live, then
// tears the substrate back down. Skipped when psmux is not found at the
// configured/default path so a -tags=smoke run never hard-fails on a
// machine without the tool installed.
func TestSmokeUpAddStatusDown(t *testing.T) {
	psmuxBinaryPath(t)

	fixture := lyxtest.CopyPaired(t)
	lyxtest.SeedConfig(t, fixture.Hub, map[string]string{
		"mux": muxengine.ConfigTemplate(),
	})
	t.Chdir(fixture.Hub)

	// Always attempt to tear the server down, even if an assertion below
	// fails partway through, so a failed run does not leak a live server.
	t.Cleanup(func() {
		var buf bytes.Buffer
		RunCLI(&buf, []string{"down"})
	})

	// up: boots the substrate (server + session), no strand command runs yet.
	var out bytes.Buffer
	if code := RunCLI(&out, []string{"up"}); code != 0 {
		t.Fatalf("up = %d; want 0, output: %s", code, out.String())
	}

	// add: a cheap placeholder command instead of a real Claude session.
	out.Reset()
	if code := RunCLI(&out, []string{"add", "--cmd", "pwsh -NoExit -Command Write-Host ready"}); code != 0 {
		t.Fatalf("add = %d; want 0, output: %s", code, out.String())
	}
	var addResult map[string]any
	if err := json.Unmarshal(out.Bytes(), &addResult); err != nil {
		t.Fatalf("parse add result: %v", err)
	}
	guid, _ := addResult["guid"].(string)
	if guid == "" {
		t.Fatalf("add result missing guid: %v", addResult)
	}

	// status: the added strand must be tracked and reported live.
	out.Reset()
	if code := RunCLI(&out, []string{"status"}); code != 0 {
		t.Fatalf("status = %d; want 0, output: %s", code, out.String())
	}
	var statusResult map[string]any
	if err := json.Unmarshal(out.Bytes(), &statusResult); err != nil {
		t.Fatalf("parse status result: %v", err)
	}
	strands, _ := statusResult["strands"].([]any)
	found := false
	for _, s := range strands {
		strand, _ := s.(map[string]any)
		if strand["guid"] != guid {
			continue
		}
		found = true
		if live, _ := strand["live"].(bool); !live {
			t.Errorf("status strand %s live = false; want true", guid)
		}
	}
	if !found {
		t.Errorf("status strands missing guid %s; got: %v", guid, strands)
	}

	// down: tears the server down and clears state.
	out.Reset()
	if code := RunCLI(&out, []string{"down"}); code != 0 {
		t.Fatalf("down = %d; want 0, output: %s", code, out.String())
	}
}

// statusStrand returns the tracked strand with the given guid from a `status`
// JSON envelope, and whether it was found.
func statusStrand(t *testing.T, statusJSON []byte, guid string) (map[string]any, bool) {
	t.Helper()
	var result map[string]any
	if err := json.Unmarshal(statusJSON, &result); err != nil {
		t.Fatalf("parse status result: %v", err)
	}
	strands, _ := result["strands"].([]any)
	for _, s := range strands {
		strand, _ := s.(map[string]any)
		if strand["guid"] == guid {
			return strand, true
		}
	}
	return nil, false
}

// TestSmokeCrashRecovery covers the discussion's "server dead (reboot)"
// recovery state end-to-end against a live psmux server: after the server is
// killed out from under mux, `up` must reboot the substrate and reconcile the
// strand to not-live (its stale pane binding cleared, not mistaken for the
// reborn session's reused initial pane id), and `resume` must then rebuild the
// strand into a fresh live pane. This is the path the pane-id-collision fix
// (clearAllPaneBindings on a booted session) exists for; the single-pane
// TestSmokeUpAddStatusDown above never reaches it.
func TestSmokeCrashRecovery(t *testing.T) {
	psmuxPath := psmuxBinaryPath(t)

	fixture := lyxtest.CopyPaired(t)
	lyxtest.SeedConfig(t, fixture.Hub, map[string]string{
		"mux": muxengine.ConfigTemplate(),
	})
	t.Chdir(fixture.Hub)
	t.Cleanup(func() {
		var buf bytes.Buffer
		RunCLI(&buf, []string{"down"})
	})

	// up + add one strand.
	var out bytes.Buffer
	if code := RunCLI(&out, []string{"up"}); code != 0 {
		t.Fatalf("up = %d; want 0, output: %s", code, out.String())
	}
	out.Reset()
	if code := RunCLI(&out, []string{"add", "--cmd", "pwsh -NoExit -Command Write-Host ready"}); code != 0 {
		t.Fatalf("add = %d; want 0, output: %s", code, out.String())
	}
	var addResult map[string]any
	if err := json.Unmarshal(out.Bytes(), &addResult); err != nil {
		t.Fatalf("parse add result: %v", err)
	}
	guid, _ := addResult["guid"].(string)
	if guid == "" {
		t.Fatalf("add result missing guid: %v", addResult)
	}

	// Read the socket so we can kill the server directly (simulating a crash).
	out.Reset()
	if code := RunCLI(&out, []string{"status"}); code != 0 {
		t.Fatalf("status = %d; want 0, output: %s", code, out.String())
	}
	var statusResult map[string]any
	if err := json.Unmarshal(out.Bytes(), &statusResult); err != nil {
		t.Fatalf("parse status result: %v", err)
	}
	socket, _ := statusResult["socket"].(string)
	session, _ := statusResult["session"].(string)
	if socket == "" || session == "" {
		t.Fatalf("status result missing socket/session: %v", statusResult)
	}

	// readStrand runs `status` fresh and returns this test's strand record
	// plus the raw JSON, so a failing assertion can print what status saw.
	readStrand := func() (map[string]any, []byte, bool) {
		var buf bytes.Buffer
		if code := RunCLI(&buf, []string{"status"}); code != 0 {
			t.Fatalf("status = %d; want 0, output: %s", code, buf.String())
		}
		raw := append([]byte(nil), buf.Bytes()...)
		strand, ok := statusStrand(t, raw, guid)
		return strand, raw, ok
	}

	// Simulate a psmux crash: kill the whole server out from under mux.
	if err := exec.Command(psmuxPath, "-L", socket, "kill-server").Run(); err != nil {
		t.Fatalf("kill-server: %v", err)
	}
	// kill-server returns before the server has fully released its socket. If
	// we called up while the dying server still answered has-session, mux
	// would treat the session as still up (booted=false), skip the stale-
	// binding clear, and the reused pane id would read falsely live — a race
	// that only surfaces on a loaded machine. A real crash is a dead process,
	// so wait until the server is genuinely gone before simulating recovery.
	waitServerGone(t, psmuxPath, socket, session)

	// up after the crash: reboots the substrate and clears the stale binding
	// (the reborn session's initial pane reuses the old pane id, so without
	// the booted-session binding-clear the strand would look falsely live).
	out.Reset()
	if code := RunCLI(&out, []string{"up"}); code != 0 {
		t.Fatalf("post-crash up = %d; want 0, output: %s", code, out.String())
	}
	strand, statusRaw, found := readStrand()
	if !found {
		t.Fatalf("strand %s missing after post-crash up; status: %s", guid, statusRaw)
	}
	if live, _ := strand["live"].(bool); live {
		t.Errorf("post-crash up: strand %s live = true; want false (stale binding must be cleared); status: %s", guid, statusRaw)
	}

	// resume: rebuilds the strand into a fresh live pane.
	out.Reset()
	if code := RunCLI(&out, []string{"resume"}); code != 0 {
		t.Fatalf("resume = %d; want 0, output: %s", code, out.String())
	}
	var resumeResult map[string]any
	if err := json.Unmarshal(out.Bytes(), &resumeResult); err != nil {
		t.Fatalf("parse resume result: %v", err)
	}
	if resumed, _ := resumeResult["resumed"].(float64); resumed < 1 {
		t.Errorf("resume resumed = %v; want >= 1 (the crashed strand must be rebuilt)", resumeResult["resumed"])
	}
	strand, statusRaw, found = readStrand()
	if !found {
		t.Fatalf("strand %s missing after resume; status: %s", guid, statusRaw)
	}
	if live, _ := strand["live"].(bool); !live {
		t.Errorf("after resume: strand %s live = false; want true; status: %s", guid, statusRaw)
	}
}

// waitServerGone blocks until `psmux -L socket has-session -t session` exits
// non-zero (the server/session is gone), or fails the test after a timeout.
// psmux's kill-server is asynchronous — it returns before the socket is
// released — so a test that simulates a crash must wait for the server to
// actually die before exercising recovery, or it races the teardown.
func waitServerGone(t *testing.T, psmuxPath, socket, session string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		if err := exec.Command(psmuxPath, "-L", socket, "has-session", "-t", session).Run(); err != nil {
			return // non-zero exit: server/session gone
		}
		if time.Now().After(deadline) {
			t.Fatalf("psmux server still up 5s after kill-server (socket %s)", socket)
		}
		time.Sleep(50 * time.Millisecond)
	}
}
