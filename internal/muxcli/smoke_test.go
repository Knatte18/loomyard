//go:build smoke

// smoke_test.go is the shared smoke-test harness: the helpers (binary
// discovery, live-psmux process/pane probes, transcript watching, fixture
// wiring) common to the smoke test files in this package
// (smoke_lifecycle_test.go, smoke_teardown_test.go, smoke_resume_test.go,
// smoke_attach_test.go). Those files drive the composed live-psmux behaviors
// through RunCLI against a real server — the basic up -> add -> status ->
// down round-trip, crash recovery, layout survival under mixed top/stack
// adds, add-after-remove-last, down's synchronous server teardown,
// cross-worktree scope, the interactive attach handover, and native claude
// --resume codeword recall. These paths are exactly where hermetic tests
// prove nothing — psmux's real semantics (positional select-layout, silent
// split failures, corpse panes, async kill-server) and claude's real
// transcript persistence only show up live. Excluded from the default `go
// test ./internal/muxcli/...`; runs under `go test -tags smoke`.

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

// smokePwshPath is the PowerShell 7 binary the smoke helpers shell out to
// for Windows process-table and PEB probes. Explicit absolute path, never a
// bare "pwsh": the WindowsApps execution alias is a 0-byte ConPTY stub.
const smokePwshPath = `C:\Code\tools\powershell7\pwsh.exe`

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

// waitServerGone blocks until `psmux -L socket has-session -t session` exits
// non-zero (the server/session is gone), or fails the test after a timeout.
// psmux's kill-server is asynchronous — it returns before the socket is
// released — so a test that simulates a crash must wait for the server to
// actually die before exercising recovery, or it races the teardown. The
// deadline is saturation-sized: the teardown is ~1s quiet, but concurrent
// suites pegging the CPU have starved fixed 5s waits of this shape.
func waitServerGone(t *testing.T, psmuxPath, socket, session string) {
	t.Helper()
	const timeout = 30 * time.Second
	deadline := time.Now().Add(timeout)
	for {
		if err := exec.Command(psmuxPath, "-L", socket, "has-session", "-t", session).Run(); err != nil {
			return // non-zero exit: server/session gone
		}
		if time.Now().After(deadline) {
			t.Fatalf("psmux server still up %s after kill-server (socket %s)", timeout, socket)
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

// paneProcessTree returns the OS pids of the session's pane child processes
// AND their full descendant subtrees. #{pane_pid} names only the pane's
// immediate launcher; on Windows the process actually holding the worktree
// directory is a deeper descendant, so the reap-correctness assertion must
// track the whole subtree, computed here with the same Win32_Process closure
// the engine uses.
func paneProcessTree(t *testing.T, psmuxPath, pwshPath, socket, session string) []int {
	t.Helper()
	out, err := exec.Command(psmuxPath, "-L", socket, "list-panes", "-t", session, "-F", "#{pane_pid}").Output()
	if err != nil {
		t.Fatalf("list-panes #{pane_pid}: %v", err)
	}
	var roots []string
	for _, l := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			if _, perr := strconv.Atoi(l); perr != nil {
				t.Fatalf("parse pane pid %q: %v", l, perr)
			}
			roots = append(roots, l)
		}
	}
	if len(roots) == 0 {
		return nil
	}
	script := fmt.Sprintf(`$roots=@(%s)
$all=Get-CimInstance Win32_Process | Select-Object ProcessId,ParentProcessId
$acc=New-Object System.Collections.Generic.HashSet[int]
foreach($r in $roots){[void]$acc.Add([int]$r)}
$changed=$true
while($changed){$changed=$false;foreach($p in $all){if($acc.Contains([int]$p.ParentProcessId) -and -not $acc.Contains([int]$p.ProcessId)){[void]$acc.Add([int]$p.ProcessId);$changed=$true}}}
$acc`, strings.Join(roots, ","))
	treeOut, err := exec.Command(pwshPath, "-NoProfile", "-NonInteractive", "-Command", script).Output()
	if err != nil {
		t.Fatalf("compute pane process tree: %v", err)
	}
	var pids []int
	for _, l := range strings.Split(strings.TrimSpace(string(treeOut)), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			pid, perr := strconv.Atoi(l)
			if perr != nil {
				t.Fatalf("parse subtree pid %q: %v", l, perr)
			}
			pids = append(pids, pid)
		}
	}
	return pids
}

// panePaneSubtree returns the OS pids of a SINGLE pane's child process AND
// its full descendant subtree, resolved with the same Win32_Process closure
// the engine uses — the per-pane analogue of paneProcessTree, so the remove
// reap assertion tracks exactly the removed pane's subtree and not the
// surviving keeper's.
func panePaneSubtree(t *testing.T, psmuxPath, pwshPath, socket, session, paneID string) []int {
	t.Helper()
	out, err := exec.Command(psmuxPath, "-L", socket, "list-panes", "-t", session,
		"-F", "#{pane_id} #{pane_pid}").Output()
	if err != nil {
		t.Fatalf("list-panes #{pane_id} #{pane_pid}: %v", err)
	}
	root := ""
	for _, l := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		fields := strings.Fields(strings.TrimSpace(l))
		if len(fields) == 2 && fields[0] == paneID {
			root = fields[1]
			break
		}
	}
	if root == "" {
		t.Fatalf("pane %s not found in list-panes output %q", paneID, out)
	}
	if _, perr := strconv.Atoi(root); perr != nil {
		t.Fatalf("parse pane pid %q: %v", root, perr)
	}
	script := fmt.Sprintf(`$roots=@(%s)
$all=Get-CimInstance Win32_Process | Select-Object ProcessId,ParentProcessId
$acc=New-Object System.Collections.Generic.HashSet[int]
foreach($r in $roots){[void]$acc.Add([int]$r)}
$changed=$true
while($changed){$changed=$false;foreach($p in $all){if($acc.Contains([int]$p.ParentProcessId) -and -not $acc.Contains([int]$p.ProcessId)){[void]$acc.Add([int]$p.ProcessId);$changed=$true}}}
$acc`, root)
	treeOut, err := exec.Command(pwshPath, "-NoProfile", "-NonInteractive", "-Command", script).Output()
	if err != nil {
		t.Fatalf("compute pane subtree: %v", err)
	}
	var pids []int
	for _, l := range strings.Split(strings.TrimSpace(string(treeOut)), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			pid, perr := strconv.Atoi(l)
			if perr != nil {
				t.Fatalf("parse subtree pid %q: %v", l, perr)
			}
			pids = append(pids, pid)
		}
	}
	return pids
}

// hubHolder is one process still holding the fixture hub as its current
// working directory, as reported by hubHolders.
type hubHolder struct {
	pid  int
	name string
}

// hubHolders returns every process whose current working directory is inside
// dir, read from each process's PEB (RTL_USER_PROCESS_PARAMETERS.
// CurrentDirectory via NtQueryInformationProcess) — the only way to find the
// conhost.exe holders, since Win32_Process exposes no cwd column. Returns nil
// when nothing holds dir or the probe fails (callers degrade to waiting).
func hubHolders(t *testing.T, pwshPath, dir string) []hubHolder {
	t.Helper()
	script := fmt.Sprintf(`
Add-Type -TypeDefinition @'
using System;
using System.Runtime.InteropServices;
public static class PebReader {
    [StructLayout(LayoutKind.Sequential)]
    public struct PBI { public IntPtr r1; public IntPtr Peb; public IntPtr r2; public IntPtr r3; public IntPtr Pid; public IntPtr r4; }
    [DllImport("ntdll.dll")] public static extern int NtQueryInformationProcess(IntPtr h, int c, ref PBI p, int l, out int r);
    [DllImport("kernel32.dll")] public static extern IntPtr OpenProcess(uint a, bool i, int pid);
    [DllImport("kernel32.dll")] public static extern bool ReadProcessMemory(IntPtr h, IntPtr a, byte[] b, int s, out IntPtr r);
    [DllImport("kernel32.dll")] public static extern bool CloseHandle(IntPtr h);
    public static string GetCwd(int pid) {
        IntPtr h = OpenProcess(0x0410, false, pid); // QUERY_INFORMATION | VM_READ
        if (h == IntPtr.Zero) return null;
        try {
            var pbi = new PBI(); int rl;
            if (NtQueryInformationProcess(h, 0, ref pbi, Marshal.SizeOf(pbi), out rl) != 0) return null;
            byte[] p = new byte[8]; IntPtr rd;
            if (!ReadProcessMemory(h, (IntPtr)((long)pbi.Peb + 0x20), p, 8, out rd)) return null; // PEB.ProcessParameters
            long pp = BitConverter.ToInt64(p, 0); if (pp == 0) return null;
            byte[] us = new byte[16];
            if (!ReadProcessMemory(h, (IntPtr)(pp + 0x38), us, 16, out rd)) return null; // CurrentDirectory.DosPath
            ushort len = BitConverter.ToUInt16(us, 0); long sp = BitConverter.ToInt64(us, 8);
            if (len == 0 || sp == 0) return null;
            byte[] ch = new byte[len];
            if (!ReadProcessMemory(h, (IntPtr)sp, ch, len, out rd)) return null;
            return System.Text.Encoding.Unicode.GetString(ch);
        } finally { CloseHandle(h); }
    }
}
'@
$needle = '%s'
Get-Process | ForEach-Object {
    $cwd = [PebReader]::GetCwd($_.Id)
    if ($cwd -and $cwd.StartsWith($needle, [System.StringComparison]::OrdinalIgnoreCase)) {
        "{0} {1}" -f $_.Id, $_.ProcessName
    }
}`, strings.ReplaceAll(dir, "'", "''"))
	out, err := exec.Command(pwshPath, "-NoProfile", "-NonInteractive", "-Command", script).Output()
	if err != nil {
		return nil
	}
	var holders []hubHolder
	for _, l := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		fields := strings.Fields(strings.TrimSpace(l))
		if len(fields) != 2 {
			continue
		}
		pid, perr := strconv.Atoi(fields[0])
		if perr != nil || pid <= 0 {
			continue
		}
		holders = append(holders, hubHolder{pid: pid, name: fields[1]})
	}
	return holders
}

// deferHubRelease registers a cleanup that makes the fixture hub directory
// releasable before the framework's TempDir RemoveAll — which runs AFTER this
// cleanup — so RemoveAll never fails with a worktree-dir-in-use error. The
// holder in question is the conhost.exe the OS parents to psmux to host each
// pane's pseudo-console: mux never spawns it, it is not a #{pane_pid}
// descendant, and on a quiet machine it exits on its own a beat after its
// pane dies — but under CPU saturation it can be ORPHANED and then holds the
// hub cwd indefinitely (observed: conhosts from failed runs still pinning
// their fixture hubs hours later), so no fixed wait can ever out-last it.
// The cleanup therefore confirms rather than waits: a short grace for the
// self-exit path, then it kills any conhost whose PEB cwd is inside the hub
// (safe — its console app is already gone; killing an orphaned host leaks
// nothing) and keeps confirming until the hub actually renames. A NON-conhost
// holder is a genuine leak (a pane child or psmux the product reap missed)
// and fails the test loudly instead of being masked. Registered before
// t.Chdir and the down cleanup so it runs AFTER them (cwd already restored
// out of hub) but BEFORE RemoveAll.
func deferHubRelease(t *testing.T, hub string) {
	t.Helper()
	t.Cleanup(func() {
		// A process cannot rename its own cwd; make sure ours is not in hub
		// while probing, then restore it so a later test's cwd-relative work
		// (e.g. buildLyxBinary resolving the module root) is not corrupted.
		prev, _ := os.Getwd()
		_ = os.Chdir(os.TempDir())

		released := func() bool {
			probe := hub + ".relprobe"
			if err := os.Rename(hub, probe); err != nil {
				return false
			}
			_ = os.Rename(probe, hub)
			return true
		}
		waitReleased := func(timeout time.Duration) bool {
			deadline := time.Now().Add(timeout)
			for {
				if released() {
					return true
				}
				if time.Now().After(deadline) {
					return false
				}
				time.Sleep(200 * time.Millisecond)
			}
		}

		// Grace phase: the healthy path, where the ConPTY host exits on its
		// own moments after its pane died.
		if !waitReleased(10 * time.Second) {
			// Escalation phase: identify the actual holders. Orphaned
			// conhosts are killed (re-scanned each round — one can appear
			// late while the OS teardown is starved); anything else holding
			// the hub is a real leak the kill must not paper over.
			deadline := time.Now().Add(90 * time.Second)
			for {
				for _, h := range hubHolders(t, smokePwshPath, hub) {
					if strings.EqualFold(h.name, "conhost") {
						if p, err := os.FindProcess(h.pid); err == nil {
							_ = p.Kill()
						}
						continue
					}
					t.Errorf("non-conhost process %d (%s) still holds fixture hub %s after teardown — a real stray-state leak, not an OS ConPTY artifact", h.pid, h.name, hub)
				}
				if waitReleased(5 * time.Second) {
					break
				}
				if time.Now().After(deadline) {
					break // let RemoveAll surface the residual error
				}
			}
		}

		// Restore the original cwd only when it is outside hub (the normal
		// case, since t.Chdir's own restore has already run by now); never
		// chdir back into a hub that is about to be removed.
		if prev != "" && !strings.HasPrefix(strings.ToLower(prev), strings.ToLower(hub)) {
			_ = os.Chdir(prev)
		}
	})
}

// psmuxSocketPids returns the OS pids of every psmux.exe process whose command
// line names the given -L socket (the server plus its __warm__ helper),
// enumerated through the Windows process table — the same reliable signal
// muxengine.serverProcessesOnSocket uses, reproduced here so the harness reap
// can find its private server without a mux engine handle.
func psmuxSocketPids(t *testing.T, pwshPath, socket string) []int {
	t.Helper()
	script := fmt.Sprintf(
		`(Get-CimInstance Win32_Process -Filter "Name='psmux.exe'" | Where-Object { $_.CommandLine -match [regex]::Escape('-L %s') }).ProcessId`,
		socket)
	out, err := exec.Command(pwshPath, "-NoProfile", "-NonInteractive", "-Command", script).Output()
	if err != nil {
		return nil
	}
	var pids []int
	for _, l := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if p, perr := strconv.Atoi(strings.TrimSpace(l)); perr == nil && p > 0 {
			pids = append(pids, p)
		}
	}
	return pids
}

// pidClosure expands roots to roots-plus-their-transitive-descendant pids in
// one Win32_Process pass — the same descendant-closure the engine's reap uses,
// so a harness reap can cover the pane shells nested below its server.
func pidClosure(t *testing.T, pwshPath string, roots []int) []int {
	t.Helper()
	if len(roots) == 0 {
		return nil
	}
	lits := make([]string, len(roots))
	for i, p := range roots {
		lits[i] = strconv.Itoa(p)
	}
	script := fmt.Sprintf(`$roots=@(%s)
$all=Get-CimInstance Win32_Process | Select-Object ProcessId,ParentProcessId
$acc=New-Object System.Collections.Generic.HashSet[int]
foreach($r in $roots){[void]$acc.Add([int]$r)}
$changed=$true
while($changed){$changed=$false;foreach($p in $all){if($acc.Contains([int]$p.ParentProcessId) -and -not $acc.Contains([int]$p.ProcessId)){[void]$acc.Add([int]$p.ProcessId);$changed=$true}}}
$acc`, strings.Join(lits, ","))
	out, err := exec.Command(pwshPath, "-NoProfile", "-NonInteractive", "-Command", script).Output()
	if err != nil {
		return roots
	}
	var pids []int
	for _, l := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if p, perr := strconv.Atoi(strings.TrimSpace(l)); perr == nil && p > 0 {
			pids = append(pids, p)
		}
	}
	if len(pids) == 0 {
		return roots
	}
	return pids
}

// reapHarnessServer tears down the test's private harness psmux server and
// waits for its whole process subtree (the server, its __warm__ helper, and the
// pane shells whose cwd is the fixture hub) to actually exit before returning.
// The harness is the test's own scaffolding, not a mux-managed session, so
// mux's down reap never covers it; without this wait its async teardown can
// outlive the framework's TempDir cleanup and leave the fixture hub dir busy
// under load. It snapshots the subtree BEFORE kill-server (while the processes
// still exist to enumerate), kills the server, then polls each pid to genuine
// exit, force-killing any straggler that outlives a generous deadline.
func reapHarnessServer(t *testing.T, psmuxPath, pwshPath, socket string) {
	t.Helper()
	subtree := pidClosure(t, pwshPath, psmuxSocketPids(t, pwshPath, socket))
	_ = exec.Command(psmuxPath, "-L", socket, "kill-server").Run()
	deadline := time.Now().Add(20 * time.Second)
	for _, pid := range subtree {
		for !processGone(pid) {
			if time.Now().After(deadline) {
				if p, err := os.FindProcess(pid); err == nil {
					_ = p.Kill()
				}
				time.Sleep(500 * time.Millisecond)
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
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

// claudeProjectDir returns the ~/.claude/projects/<encoded-cwd> directory
// claude persists transcripts into for sessions whose cwd is dir. Claude
// encodes the cwd into the project directory name by replacing every
// non-alphanumeric character with '-' (verified against a live transcript:
// `C:\...\Temp\TestSmoke...\001\hub` -> `C--...-Temp-TestSmoke...-001-hub`).
// Scoping the transcript watch to THIS directory is what keeps a
// concurrently running sibling suite's brand-new transcript — which a global
// snapshot-diff over all of ~/.claude/projects wrongly matched — from being
// mistaken for the one under test.
func claudeProjectDir(t *testing.T, dir string) string {
	t.Helper()
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("resolve home dir: %v", err)
	}
	encoded := []byte(dir)
	for i, c := range encoded {
		isAlnum := (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
		if !isAlnum {
			encoded[i] = '-'
		}
	}
	return filepath.Join(home, ".claude", "projects", string(encoded))
}

// claudeTranscriptFiles returns the set of every *.jsonl transcript path
// currently under projectDir (a claudeProjectDir result). Claude persists
// one JSONL per conversation inside its session-cwd's project directory, so
// watching only this test's directory pins the observation to this test's
// own claude.
func claudeTranscriptFiles(t *testing.T, projectDir string) map[string]bool {
	t.Helper()
	found := map[string]bool{}
	_ = filepath.WalkDir(projectDir, func(path string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasSuffix(path, ".jsonl") {
			found[path] = true
		}
		return nil
	})
	return found
}

// waitTranscriptStable blocks until a transcript that did NOT exist in
// `before` (the snapshot taken just before this test launched its claude)
// appears under projectDir — this test's own claude project directory, so a
// concurrent sibling suite's transcript can never match — and stops growing:
// the direct, TUI-independent proof that claude persisted a conversation.
// It dismisses the trust gate on every poll (a fresh dir re-triggers
// it). "Stable" means the same non-zero size across two consecutive polls,
// so an in-progress write is never mistaken for a finished one. Returns the
// new transcript's path.
func waitTranscriptStable(t *testing.T, projectDir string, before map[string]bool, dismissTrust func(paneID string), paneID string, timeout time.Duration) string {
	t.Helper()
	deadline := time.Now().Add(timeout)
	sizes := map[string]int64{}
	for {
		dismissTrust(paneID)

		for path := range claudeTranscriptFiles(t, projectDir) {
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

// materializeSibling clones the paired fixture's bare origin into a second
// worktree directory alongside the primary hub, so both live directly under
// the same parent directory. Because the psmux server name/socket derives from
// the hub (the parent of the worktree root) while the session name is the
// worktree's own basename, the two clones resolve to the SAME per-hub socket
// but carry DISTINCT sessions — exactly the "two worktrees on one hub" fixture
// the cross-worktree scope invariant needs. It seeds mux config into the
// sibling and returns its absolute path. A clone (not a bare mkdir) is used so
// the sibling is a full git repo with a main worktree, which hubgeometry.Resolve
// requires.
func materializeSibling(t *testing.T, fixture lyxtest.PairedFixture, name string) string {
	t.Helper()
	sibling := filepath.Join(fixture.Container, name)
	lyxtest.MustRun(t, fixture.Container, "git", "clone", fixture.Bare, sibling)
	lyxtest.MustRun(t, sibling, "git", "config", "user.email", "test@test.com")
	lyxtest.MustRun(t, sibling, "git", "config", "user.name", "Test")
	lyxtest.SeedConfig(t, sibling, map[string]string{
		"mux": muxengine.ConfigTemplate(),
	})
	return sibling
}

// mustChdir changes the process working directory or fails the test. The
// cross-worktree test drives two worktrees through RunCLI (which resolves the
// hub/session from cwd), so it switches cwd between them repeatedly rather than
// using t.Chdir once; smoke tests never run in parallel within one binary, so a
// process-wide cwd switch is safe (parallelism comes from separate binaries).
func mustChdir(t *testing.T, dir string) {
	t.Helper()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir %s: %v", dir, err)
	}
}

// sessionAlive reports whether the named session currently exists on the
// socket (has-session exit 0), without failing the test — the non-fatal probe
// the stability loop polls on.
func sessionAlive(psmuxPath, socket, session string) bool {
	return exec.Command(psmuxPath, "-L", socket, "has-session", "-t", session).Run() == nil
}

// waitSessionUp blocks until the named session answers has-session on the
// socket, or fails after a saturation-sized deadline. psmux verbs are async, so
// a just-issued up may not have registered its session on the first probe.
func waitSessionUp(t *testing.T, psmuxPath, socket, session string) {
	t.Helper()
	const timeout = 60 * time.Second
	deadline := time.Now().Add(timeout)
	for {
		if sessionAlive(psmuxPath, socket, session) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("session %s never came up on socket %s within %s", session, socket, timeout)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// paneLiveOnSession reports whether paneID appears in a listPaneLines result
// (rows of "<pane_id> <pane_dead> <pane_top> <pane_height>") with pane_dead=0.
func paneLiveOnSession(lines []string, paneID string) bool {
	for _, l := range lines {
		fields := strings.Fields(l)
		if len(fields) >= 2 && fields[0] == paneID && fields[1] == "0" {
			return true
		}
	}
	return false
}

// paneRootPID returns a pane's root process id (#{pane_pid}) on the socket —
// the immediate process psmux launched in the pane. For this test's
// `pwsh -NoExit` placeholder that root IS the agent process, and it is stable
// (unlike the pane's transient descendants such as the OS ConPTY conhost),
// which makes it the reliable OS-level "the agent is alive" signal for the
// sibling-stability check.
func paneRootPID(t *testing.T, psmuxPath, socket, session, paneID string) int {
	t.Helper()
	out, err := exec.Command(psmuxPath, "-L", socket, "list-panes", "-t", session,
		"-F", "#{pane_id} #{pane_pid}").Output()
	if err != nil {
		t.Fatalf("list-panes #{pane_id} #{pane_pid}: %v", err)
	}
	for _, l := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		fields := strings.Fields(strings.TrimSpace(l))
		if len(fields) == 2 && fields[0] == paneID {
			pid, perr := strconv.Atoi(fields[1])
			if perr != nil {
				t.Fatalf("parse pane pid %q: %v", fields[1], perr)
			}
			return pid
		}
	}
	t.Fatalf("pane %s not found in list-panes output %q", paneID, out)
	return 0
}

// paneIDForStrand runs `status` in the current worktree and returns the tracked
// strand's live pane id, failing if the strand or its pane is missing.
func paneIDForStrand(t *testing.T, guid string) string {
	t.Helper()
	var out bytes.Buffer
	if code := RunCLI(&out, []string{"status"}); code != 0 {
		t.Fatalf("status = %d; want 0, output: %s", code, out.String())
	}
	strand, ok := statusStrand(t, out.Bytes(), guid)
	if !ok {
		t.Fatalf("status missing strand %s: %s", guid, out.String())
	}
	paneID, _ := strand["paneId"].(string)
	if paneID == "" {
		t.Fatalf("strand %s has no pane: %s", guid, out.String())
	}
	return paneID
}

// serverProcCountForSession counts the psmux.exe server processes backing a
// SPECIFIC session on the socket. This psmux port spawns one
// `psmux.exe server -s <session> -L <socket>` process per session (all sharing
// the -L socket namespace, alongside psmux's internal `__warm__` helper), so
// the per-hub "no duplicate server" guarantee is checked per session: exactly
// one backing process for a live session, zero for a killed one, and never two
// (a duplicate spawned by a down->up churn race). Returns -1 when the process-
// table query itself fails, distinct from a genuine zero.
func serverProcCountForSession(t *testing.T, pwshPath, socket, session string) int {
	t.Helper()
	needle := fmt.Sprintf("-s %s -L %s", session, socket)
	script := fmt.Sprintf(
		`(Get-CimInstance Win32_Process -Filter "Name='psmux.exe'" | Where-Object { $_.CommandLine -match [regex]::Escape('%s') }).ProcessId`,
		needle)
	out, err := exec.Command(pwshPath, "-NoProfile", "-NonInteractive", "-Command", script).Output()
	if err != nil {
		return -1
	}
	count := 0
	for _, l := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if p, perr := strconv.Atoi(strings.TrimSpace(l)); perr == nil && p > 0 {
			count++
		}
	}
	return count
}

// waitServerProcCountForSession polls serverProcCountForSession until it equals
// want, or fails after a saturation-sized deadline — psmux spawns and reaps a
// session's backing server process asynchronously, so both "it came up" and
// "it went away" must be polled, never read once.
func waitServerProcCountForSession(t *testing.T, pwshPath, socket, session string, want int) {
	t.Helper()
	const timeout = 60 * time.Second
	deadline := time.Now().Add(timeout)
	for {
		if got := serverProcCountForSession(t, pwshPath, socket, session); got == want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("session %s backing-server count never reached %d within %s (got %d)", session, want, timeout, serverProcCountForSession(t, pwshPath, socket, session))
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// assertSiblingStaysLive polls for dur, failing IMMEDIATELY if the sibling
// worktree's session, its strand pane, its backing-server pid, or its agent
// root process ever drop. This is the anti-false-green core of the
// cross-worktree test: a naive down that killed the shared-socket server set
// (rather than only its own session) would trip one of these checks on the
// first or an early iteration, and holding the invariant for the whole window
// proves down left the sibling genuinely usable — not merely that a stale
// has-session lingered. Every per-iteration probe here is a fast psmux call
// (has-session / list-panes / display-message) or an in-process pid wait; the
// expensive process-table count that guards against a *duplicate* backing
// server is checked ONCE by the caller after this returns, so the tight loop
// never spawns a pwsh-per-iteration and starves concurrent copies under load.
func assertSiblingStaysLive(t *testing.T, psmuxPath, socket, session, paneID string, wantServerPID, agentPID int, dur time.Duration) {
	t.Helper()
	deadline := time.Now().Add(dur)
	for {
		if !sessionAlive(psmuxPath, socket, session) {
			t.Fatalf("sibling session %s died after down in the other worktree — down killed the shared-socket server set", session)
		}
		if lines := listPaneLines(t, psmuxPath, socket, session); !paneLiveOnSession(lines, paneID) {
			t.Fatalf("sibling pane %s not live after down in the other worktree; panes=%v", paneID, lines)
		}
		if pid := serverPID(t, psmuxPath, socket, session); pid != wantServerPID {
			t.Fatalf("sibling backing-server pid changed to %d (was %d) after down in the other worktree — its server was killed or restarted", pid, wantServerPID)
		}
		if processGone(agentPID) {
			t.Fatalf("sibling agent process %d (pane %s root) died after down in the other worktree", agentPID, paneID)
		}
		if time.Now().After(deadline) {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// waitSocketFreeOfPsmux polls until no psmux process names the socket, or fails
// after a saturation-sized deadline. kill-server is async, so the final
// stray-server check must poll rather than read once.
func waitSocketFreeOfPsmux(t *testing.T, pwshPath, socket string) {
	t.Helper()
	const timeout = 30 * time.Second
	deadline := time.Now().Add(timeout)
	for {
		if pids := psmuxSocketPids(t, pwshPath, socket); len(pids) == 0 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("psmux still on socket %s after %s: pids=%v", socket, timeout, psmuxSocketPids(t, pwshPath, socket))
		}
		time.Sleep(100 * time.Millisecond)
	}
}
