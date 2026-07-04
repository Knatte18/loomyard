//go:build smoke

// smoke_test.go drives the composed live-psmux behaviors through RunCLI
// against a real server: the basic up -> add -> status -> down round-trip,
// crash recovery (kill-server -> up -> resume), layout survival under mixed
// top/stack adds, add-after-remove-last (corpse panes are never adopted),
// down's synchronous server teardown, the interactive attach handover
// (driven inside a harness psmux pane), and native claude --resume codeword
// recall (skipped when claude is absent). These paths are exactly where
// hermetic tests prove nothing — psmux's real semantics (positional
// select-layout, silent split failures, corpse panes, async kill-server)
// and claude's real transcript persistence only show up live. Excluded from
// the default `go test ./internal/muxcli/...`; runs under `go test -tags
// smoke`.

package muxcli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
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

// listPaneLines returns the session's list-panes rows as
// "<pane_id> <pane_dead> <pane_top> <pane_height>" strings. Uses psmux
// directly (the same controlled exception the sandbox suite grants) so a
// smoke test can assert on the real pane set rather than trusting mux's own
// reporting.
func listPaneLines(t *testing.T, psmuxPath, socket, session string) []string {
	t.Helper()
	out, err := exec.Command(psmuxPath, "-L", socket, "list-panes", "-t", session,
		"-F", "#{pane_id} #{pane_dead} #{pane_top} #{pane_height}").Output()
	if err != nil {
		t.Fatalf("list-panes: %v", err)
	}
	var lines []string
	for _, l := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			lines = append(lines, l)
		}
	}
	return lines
}

// socketAndSession reads the socket and session names from a fresh `status`.
func socketAndSession(t *testing.T) (socket, session string) {
	t.Helper()
	var out bytes.Buffer
	if code := RunCLI(&out, []string{"status"}); code != 0 {
		t.Fatalf("status = %d; want 0, output: %s", code, out.String())
	}
	var result map[string]any
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatalf("parse status result: %v", err)
	}
	socket, _ = result["socket"].(string)
	session, _ = result["session"].(string)
	if socket == "" || session == "" {
		t.Fatalf("status result missing socket/session: %v", result)
	}
	return socket, session
}

// addStrand runs `add` with the given extra flags and returns the new guid.
func addStrand(t *testing.T, cmdStr string, extra ...string) string {
	t.Helper()
	var out bytes.Buffer
	args := append([]string{"add", "--cmd", cmdStr}, extra...)
	if code := RunCLI(&out, args); code != 0 {
		t.Fatalf("add %v = %d; want 0, output: %s", extra, code, out.String())
	}
	var result map[string]any
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatalf("parse add result: %v", err)
	}
	guid, _ := result["guid"].(string)
	if guid == "" {
		t.Fatalf("add result missing guid: %v", result)
	}
	return guid
}

// TestSmokeTopBandsThenStackAddsKeepEverySessionPane pins the composed
// split-path defect this round fixed: with two top-anchored strands laid
// out (a 1-row band + a stretched band), psmux parks the active pane on the
// tiny band, and a session-target split-window then fails SILENTLY (exit 0,
// no new pane, prints an existing pane's id) — mux would bind the new
// strand to an existing pane, and the next select-layout's duplicate pane
// number made psmux destroy every pane in the session. The fix splits the
// tallest alive pane explicitly and hard-errors on a non-new reported id,
// so this sequence must now yield one live pane per visible strand.
func TestSmokeTopBandsThenStackAddsKeepEverySessionPane(t *testing.T) {
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

	var out bytes.Buffer
	if code := RunCLI(&out, []string{"up"}); code != 0 {
		t.Fatalf("up = %d; want 0, output: %s", code, out.String())
	}

	launch := "pwsh -NoExit -Command Write-Host ready"
	guids := []string{
		addStrand(t, launch, "--anchor", "top", "--name", "band1"),
		addStrand(t, launch, "--anchor", "top", "--name", "band2"),
		addStrand(t, launch, "--name", "stack1"),
		addStrand(t, launch, "--name", "stack2"),
	}

	socket, session := socketAndSession(t)
	panes := listPaneLines(t, psmuxPath, socket, session)
	if len(panes) != len(guids) {
		t.Fatalf("session holds %d panes %v; want %d (one per visible strand — a shortfall means a silent split failure destroyed panes)", len(panes), panes, len(guids))
	}

	out.Reset()
	if code := RunCLI(&out, []string{"status"}); code != 0 {
		t.Fatalf("status = %d; want 0, output: %s", code, out.String())
	}
	for _, guid := range guids {
		strand, found := statusStrand(t, out.Bytes(), guid)
		if !found {
			t.Fatalf("status missing strand %s; output: %s", guid, out.String())
		}
		if live, _ := strand["live"].(bool); !live {
			t.Errorf("strand %s (%v) live = false; want true", guid, strand["name"])
		}
	}
}

// TestSmokeRemoveLastStrandThenAddRunsTheNewCommand pins the corpse-pane
// adoption defect this round fixed: kill-pane on a session's SOLE pane does
// not remove it — under remain-on-exit psmux corpses it as pane_dead=1 with
// exit 0 — and the old adopt path then bound the next added strand to that
// corpse, silently swallowing its send-keys (the command never ran, and the
// next verb's reconcile stripped the binding again). The fix never adopts a
// dead pane, so the post-remove add must yield a strand that is live and
// STAYS live across the next reconciling verb.
func TestSmokeRemoveLastStrandThenAddRunsTheNewCommand(t *testing.T) {
	psmuxBinaryPath(t)

	fixture := lyxtest.CopyPaired(t)
	lyxtest.SeedConfig(t, fixture.Hub, map[string]string{
		"mux": muxengine.ConfigTemplate(),
	})
	t.Chdir(fixture.Hub)
	t.Cleanup(func() {
		var buf bytes.Buffer
		RunCLI(&buf, []string{"down"})
	})

	var out bytes.Buffer
	if code := RunCLI(&out, []string{"up"}); code != 0 {
		t.Fatalf("up = %d; want 0, output: %s", code, out.String())
	}

	launch := "pwsh -NoExit -Command Write-Host ready"
	first := addStrand(t, launch, "--name", "first")
	out.Reset()
	if code := RunCLI(&out, []string{"remove", first}); code != 0 {
		t.Fatalf("remove = %d; want 0, output: %s", code, out.String())
	}

	second := addStrand(t, launch, "--name", "second")

	// The reconciling verb is the trap: with the old corpse adoption the
	// strand read live immediately after add (its binding named the corpse,
	// still present), and only the next reconcile exposed the lie by
	// clearing the binding. up reconciles; the strand must still be live.
	out.Reset()
	if code := RunCLI(&out, []string{"up"}); code != 0 {
		t.Fatalf("post-add up = %d; want 0, output: %s", code, out.String())
	}
	out.Reset()
	if code := RunCLI(&out, []string{"status"}); code != 0 {
		t.Fatalf("status = %d; want 0, output: %s", code, out.String())
	}
	strand, found := statusStrand(t, out.Bytes(), second)
	if !found {
		t.Fatalf("status missing strand %s; output: %s", second, out.String())
	}
	if live, _ := strand["live"].(bool); !live {
		t.Errorf("strand added after remove-last: live = false; want true (adopted a dead corpse pane?); status: %s", out.String())
	}
}

// serverPID asks psmux for the server's OS pid via the #{pid} format
// variable (the only server-liveness signal psmux exposes: list-sessions
// and kill-server both exit 0 whether or not a server holds the socket).
func serverPID(t *testing.T, psmuxPath, socket, session string) int {
	t.Helper()
	out, err := exec.Command(psmuxPath, "-L", socket, "display-message", "-p", "-t", session, "#{pid}").Output()
	if err != nil {
		t.Fatalf("display-message #{pid}: %v", err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil {
		t.Fatalf("parse server pid %q: %v", out, err)
	}
	return pid
}

// processGone reports whether pid no longer names a running process,
// tolerating a just-released process object: a live process blocks in Wait,
// so a Wait that returns within the short grace window means exited.
func processGone(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return true
	}
	done := make(chan struct{})
	go func() {
		_, _ = proc.Wait()
		close(done)
	}()
	select {
	case <-done:
		return true
	case <-time.After(100 * time.Millisecond):
		return false
	}
}

// TestSmokeDownReleasesServerBeforeReturning pins the down->up churn race
// this round fixed: psmux's kill-server is asynchronous, and a Down that
// returned while the old server still held the socket let an immediate up
// spawn a duplicate server process that lingered forever as an unreachable
// stray. Down now waits on the server PROCESS itself (psmux's CLI cannot
// report server absence — every probe exits 0), so the moment it returns
// the server must be gone — and an immediate up+add cycle must work. Three
// back-to-back cycles with no sleeps.
func TestSmokeDownReleasesServerBeforeReturning(t *testing.T) {
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

	var out bytes.Buffer
	if code := RunCLI(&out, []string{"up"}); code != 0 {
		t.Fatalf("up = %d; want 0, output: %s", code, out.String())
	}
	socket, session := socketAndSession(t)

	launch := "pwsh -NoExit -Command Write-Host ready"
	for cycle := 0; cycle < 3; cycle++ {
		pid := serverPID(t, psmuxPath, socket, session)
		out.Reset()
		if code := RunCLI(&out, []string{"down"}); code != 0 {
			t.Fatalf("cycle %d down = %d; want 0, output: %s", cycle, code, out.String())
		}
		// No sleep: the server process must already be gone when down
		// returns.
		if !processGone(pid) {
			t.Fatalf("cycle %d: psmux server (pid %d) still running immediately after down returned", cycle, pid)
		}
		out.Reset()
		if code := RunCLI(&out, []string{"up"}); code != 0 {
			t.Fatalf("cycle %d up = %d; want 0, output: %s", cycle, code, out.String())
		}
		addStrand(t, launch, "--name", "churn")
	}
}

// capturePane returns the rendered content of the target pane on socket via
// capture-pane -p (a controlled psmux exception, like listPaneLines).
func capturePane(t *testing.T, psmuxPath, socket, target string) string {
	t.Helper()
	out, err := exec.Command(psmuxPath, "-L", socket, "capture-pane", "-p", "-t", target).Output()
	if err != nil {
		t.Fatalf("capture-pane -t %s: %v", target, err)
	}
	return string(out)
}

// sendKeysLine types text literally into the target pane (send-keys -l, so
// psmux never reinterprets it) and submits it with a separate Enter.
func sendKeysLine(t *testing.T, psmuxPath, socket, target, text string) {
	t.Helper()
	if err := exec.Command(psmuxPath, "-L", socket, "send-keys", "-t", target, "-l", text).Run(); err != nil {
		t.Fatalf("send-keys -l %q: %v", text, err)
	}
	if err := exec.Command(psmuxPath, "-L", socket, "send-keys", "-t", target, "Enter").Run(); err != nil {
		t.Fatalf("send-keys Enter: %v", err)
	}
}

// pollPaneContains polls capture-pane until the target pane's rendered
// content contains want, failing the test after timeout with the last
// capture attached for diagnosis.
func pollPaneContains(t *testing.T, psmuxPath, socket, target, want string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	last := ""
	for {
		last = capturePane(t, psmuxPath, socket, target)
		if strings.Contains(last, want) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("pane %s never showed %q within %s; last capture:\n%s", target, want, timeout, last)
		}
		time.Sleep(500 * time.Millisecond)
	}
}

// buildLyxBinary compiles the working tree's cmd/lyx into a temp dir and
// returns its path. The attach test must exec a REAL lyx process (the
// terminal handover cannot run in-process through RunCLI), and building
// from source guarantees the process under test is never a stale deployed
// snapshot. Must be called BEFORE t.Chdir moves the test off the repo.
func buildLyxBinary(t *testing.T) string {
	t.Helper()
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	lyxExe := filepath.Join(t.TempDir(), "lyx.exe")
	cmd := exec.Command("go", "build", "-o", lyxExe, "./cmd/lyx")
	cmd.Dir = repoRoot
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build ./cmd/lyx: %v\n%s", err, out)
	}
	return lyxExe
}

// TestSmokeAttachRendersInsideHarnessPane drives the one verb no headless
// test could previously reach: the interactive terminal handover of
// `lyx mux attach`. A pane inside a separate harness psmux server has a
// real ConPTY terminal, so running lyx mux attach THERE (with PSMUX_SESSION
// unset — psmux refuses nesting otherwise) exercises the full handover:
// pre-flight, stdio inheritance, psmux attach, and actual rendering. The
// harness pane's capture must show the mux session's strand content and
// status bar, and after a C-b d detach the attach process must exit 0.
func TestSmokeAttachRendersInsideHarnessPane(t *testing.T) {
	psmuxPath := psmuxBinaryPath(t)
	lyxExe := buildLyxBinary(t)

	fixture := lyxtest.CopyPaired(t)
	lyxtest.SeedConfig(t, fixture.Hub, map[string]string{
		"mux": muxengine.ConfigTemplate(),
	})
	t.Chdir(fixture.Hub)
	t.Cleanup(func() {
		var buf bytes.Buffer
		RunCLI(&buf, []string{"down"})
	})

	var out bytes.Buffer
	if code := RunCLI(&out, []string{"up"}); code != 0 {
		t.Fatalf("up = %d; want 0, output: %s", code, out.String())
	}
	addStrand(t, "pwsh -NoExit -Command Write-Host ATTACH-MARKER-ALPHA", "--name", "amarker")
	muxSocket, session := socketAndSession(t)

	// Harness server on its own socket, spawned with cwd = the fixture hub
	// so the lyx process typed into its pane resolves the right geometry.
	harness := fmt.Sprintf("lyx-attach-harness-%d", os.Getpid())
	if err := exec.Command(psmuxPath, "-L", harness, "new-session", "-d", "-s", "h", "-x", "140", "-y", "42",
		`C:\Code\tools\powershell7\pwsh.exe`).Run(); err != nil {
		t.Fatalf("boot harness server: %v", err)
	}
	t.Cleanup(func() {
		_ = exec.Command(psmuxPath, "-L", harness, "kill-server").Run()
	})
	deadline := time.Now().Add(10 * time.Second)
	for exec.Command(psmuxPath, "-L", harness, "has-session", "-t", "h").Run() != nil {
		if time.Now().After(deadline) {
			t.Fatal("harness session did not come up within 10s")
		}
		time.Sleep(100 * time.Millisecond)
	}

	// The handover under test: attach to the mux session from inside the
	// harness pane. PSMUX_SESSION must be unset or psmux refuses to nest.
	sendKeysLine(t, psmuxPath, harness, "%1",
		fmt.Sprintf(`$env:PSMUX_SESSION=$null; & '%s' mux attach; Write-Host ATTACH-EXIT:$LASTEXITCODE`, lyxExe))

	// The harness pane now renders the INNER session: the strand's marker
	// only ever existed inside the mux session, so seeing it here proves
	// the attach handover rendered for real.
	pollPaneContains(t, psmuxPath, harness, "%1", "ATTACH-MARKER-ALPHA", 20*time.Second)

	// Detach (prefix C-b, then d) and confirm the attach process exited 0.
	if err := exec.Command(psmuxPath, "-L", harness, "send-keys", "-t", "%1", "C-b", "d").Run(); err != nil {
		t.Fatalf("send detach keys: %v", err)
	}
	pollPaneContains(t, psmuxPath, harness, "%1", "ATTACH-EXIT:0", 15*time.Second)

	// The mux session itself must have survived the client detaching.
	if err := exec.Command(psmuxPath, "-L", muxSocket, "has-session", "-t", session).Run(); err != nil {
		t.Errorf("mux session %s gone after detach: %v", session, err)
	}
}

// paneEventuallyContains reports whether the target pane's rendered content
// comes to contain want within timeout — the non-fatal sibling of
// pollPaneContains, for a branch that has a fallback path when it does not.
func paneEventuallyContains(t *testing.T, psmuxPath, socket, target, want string, timeout time.Duration) bool {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		if strings.Contains(capturePane(t, psmuxPath, socket, target), want) {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(1 * time.Second)
	}
}

// claudeTranscriptFiles returns the set of every *.jsonl transcript path
// currently under ~/.claude/projects. Claude persists one JSONL per
// conversation, keyed by a project directory derived from the session's
// cwd; the snapshot-diff against this set is how the test spots the ONE new
// transcript its own claude launch produced, without having to reproduce
// claude's exact cwd-to-key encoding.
func claudeTranscriptFiles(t *testing.T) map[string]bool {
	t.Helper()
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("resolve home dir: %v", err)
	}
	root := filepath.Join(home, ".claude", "projects")
	found := map[string]bool{}
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasSuffix(path, ".jsonl") {
			found[path] = true
		}
		return nil
	})
	return found
}

// waitTranscriptStable blocks until a transcript that did NOT exist in
// `before` (the snapshot taken just before this test launched its claude)
// appears and stops growing — the direct, TUI-independent proof that claude
// persisted a conversation. Excluding `before` keeps a concurrently-growing
// transcript from another session (e.g. the outer Claude Code session that
// may be running this suite) from ever being mistaken for the one under
// test. It dismisses the trust gate on every poll (a fresh dir re-triggers
// it). "Stable" means the same non-zero size across two consecutive polls,
// so an in-progress write is never mistaken for a finished one. Returns the
// new transcript's path.
func waitTranscriptStable(t *testing.T, before map[string]bool, dismissTrust func(paneID string), paneID string, timeout time.Duration) string {
	t.Helper()
	deadline := time.Now().Add(timeout)
	sizes := map[string]int64{}
	for {
		dismissTrust(paneID)

		for path := range claudeTranscriptFiles(t) {
			if before[path] {
				continue // pre-existing — not this test's transcript
			}
			info, err := os.Stat(path)
			if err != nil {
				continue
			}
			prev, seen := sizes[path]
			if seen && prev > 0 && info.Size() == prev {
				return path
			}
			sizes[path] = info.Size()
		}

		if time.Now().After(deadline) {
			t.Fatalf("no new claude transcript persisted+stabilized within %s (env hygiene may be broken — claude in a nested Claude Code session stops writing transcripts)", timeout)
		}
		time.Sleep(2 * time.Second)
	}
}

// claudeBinaryPath returns the claude CLI's path from the environment or
// PATH, skipping the calling test when it is absent so a -tags=smoke run
// never hard-fails on a machine without a configured claude.
func claudeBinaryPath(t *testing.T) string {
	t.Helper()
	if path := os.Getenv("LYX_MUX_CLAUDE"); path != "" {
		return path
	}
	path, err := exec.LookPath("claude")
	if err != nil {
		t.Skip("claude not found on PATH")
	}
	return path
}

// TestSmokeClaudeResumeRecallsCodeword is the end-to-end proof of mux's one
// Claude-adjacent responsibility: env hygiene on the server spawn (without
// it, a claude launched from inside a Claude Code session treats itself as
// a nested child and silently stops persisting its transcript) plus the
// opaque resumeCmd replay. It launches a real claude in a strand with a
// codeword prompt, kills the whole psmux server out from under it, resumes
// via the stored `claude --continue`, and asserts the codeword comes back —
// which is only possible if the transcript was persisted and found again.
// Needs a logged-in claude CLI; runs a real subscription session (~1-3 min).
func TestSmokeClaudeResumeRecallsCodeword(t *testing.T) {
	psmuxPath := psmuxBinaryPath(t)
	claudePath := claudeBinaryPath(t)

	fixture := lyxtest.CopyPaired(t)
	lyxtest.SeedConfig(t, fixture.Hub, map[string]string{
		"mux": muxengine.ConfigTemplate(),
	})
	t.Chdir(fixture.Hub)
	t.Cleanup(func() {
		var buf bytes.Buffer
		RunCLI(&buf, []string{"down"})
	})

	var out bytes.Buffer
	if code := RunCLI(&out, []string{"up"}); code != 0 {
		t.Fatalf("up = %d; want 0, output: %s", code, out.String())
	}

	codeword := fmt.Sprintf("zebra-%d", time.Now().UnixNano()%1000000)
	launch := fmt.Sprintf(`& '%s' 'Remember the codeword %s. Reply with exactly: STORED %s'`, claudePath, codeword, codeword)
	resume := fmt.Sprintf(`& '%s' --continue`, claudePath)

	// Snapshot existing transcripts BEFORE the launch, so phase 1 can spot
	// the one new transcript this test's claude produces (see
	// waitTranscriptStable) without reproducing claude's cwd-to-key encoding.
	transcriptsBefore := claudeTranscriptFiles(t)
	guid := addStrand(t, launch, "--resume-cmd", resume, "--name", "agent")
	socket, session := socketAndSession(t)

	readPane := func() string {
		t.Helper()
		var buf bytes.Buffer
		if code := RunCLI(&buf, []string{"status"}); code != 0 {
			t.Fatalf("status = %d; want 0, output: %s", code, buf.String())
		}
		strand, ok := statusStrand(t, buf.Bytes(), guid)
		if !ok {
			t.Fatalf("status missing strand %s: %s", guid, buf.String())
		}
		paneID, _ := strand["paneId"].(string)
		if paneID == "" {
			t.Fatalf("strand %s has no pane: %s", guid, buf.String())
		}
		return paneID
	}

	// dismissTrust answers claude's one-time "do you trust this folder?"
	// gate (Enter = its default "yes") whenever that screen is visible. A
	// fresh fixture dir triggers it; it is operator setup, not the contract
	// under test. Called on every poll iteration (not once) because a single
	// early Enter can land before the prompt is interactive and be dropped.
	dismissTrust := func(paneID string) {
		content := capturePane(t, psmuxPath, socket, paneID)
		if strings.Contains(content, "trust") && strings.Contains(content, "folder") {
			_ = exec.Command(psmuxPath, "-L", socket, "send-keys", "-t", paneID, "Enter").Run()
		}
	}

	// Phase 1: let claude receive the codeword and PERSIST a transcript
	// before the crash. The persistence gate is the transcript file itself,
	// not a TUI idle marker: claude's "? for shortcuts" hint is on screen
	// even while it is still starting/thinking, so a marker-based wait can
	// fire before the first transcript flush and the crash then truncates
	// before anything reaches disk (which is exactly what a "No conversation
	// found" resume looks like — a test artifact, not a mux defect). Waiting
	// for the .jsonl to appear and stop growing is the direct proof that env
	// hygiene let claude persist — the whole point of this test.
	paneID := readPane()
	transcript := waitTranscriptStable(t, transcriptsBefore, dismissTrust, paneID, 180*time.Second)
	t.Logf("phase 1 transcript persisted: %s", transcript)

	// Phase 2: crash the whole server, then resume. The stored resumeCmd is
	// `claude --continue`, which reopens the most recent conversation for
	// this directory — it only finds one because the transcript above
	// persisted.
	if err := exec.Command(psmuxPath, "-L", socket, "kill-server").Run(); err != nil {
		t.Fatalf("kill-server: %v", err)
	}
	waitServerGone(t, psmuxPath, socket, session)

	out.Reset()
	if code := RunCLI(&out, []string{"resume"}); code != 0 {
		t.Fatalf("resume = %d; want 0, output: %s", code, out.String())
	}
	var resumeResult map[string]any
	if err := json.Unmarshal(out.Bytes(), &resumeResult); err != nil {
		t.Fatalf("parse resume result: %v", err)
	}
	if resumed, _ := resumeResult["resumed"].(float64); resumed != 1 {
		t.Fatalf("resumed = %v; want 1", resumeResult["resumed"])
	}

	// Phase 3: the codeword must come back in the RESUMED pane. The resume
	// command line is `claude --continue` — it carries NO codeword, so the
	// codeword appearing here can only come from the persisted transcript
	// being reloaded (the whole point). Match the codeword token alone: it
	// has no internal spaces, so it survives capture-pane's space-stripping
	// of claude's rendered response boxes. `--continue` re-renders the prior
	// turn, so the codeword typically returns on its own; if a future TUI
	// hides history, ask for it explicitly (the question carries no codeword,
	// so it cannot false-match).
	resumedPane := readPane()
	dismissTrust(resumedPane)
	if paneEventuallyContains(t, psmuxPath, socket, resumedPane, codeword, 30*time.Second) {
		return
	}
	sendKeysLine(t, psmuxPath, socket, resumedPane, "What was the codeword I gave you? Reply with only that word.")
	pollPaneContains(t, psmuxPath, socket, resumedPane, codeword, 120*time.Second)
}
