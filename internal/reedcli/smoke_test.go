//go:build smoke

// smoke_test.go is the shared smoke-test harness: the helpers (binary
// discovery, live-tmux process/pane probes, transcript watching, fixture
// wiring) common to the smoke test files in this package
// (smoke_lifecycle_test.go, smoke_teardown_test.go, smoke_resume_test.go,
// smoke_attach_test.go). Those files drive the composed live-tmux behaviors
// through RunCLI against a real server — the basic up -> add -> status ->
// down round-trip, crash recovery, layout survival under stacked
// below-parent adds, add-after-remove-last, down's synchronous server teardown,
// cross-worktree scope, the interactive attach handover, and native claude
// --resume codeword recall. These paths are exactly where hermetic tests
// prove nothing — tmux's real semantics (positional select-layout, silent
// split failures, corpse panes, async kill-server) and claude's real
// transcript persistence only show up live. Excluded from the default `go
// test ./internal/reedcli/...`; runs under `go test -tags smoke`.

package reedcli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/reedengine"
)

// smokePwshPath is the default PowerShell 7 binary the smoke helpers shell
// out to for Windows process-table and PEB probes. Explicit absolute path,
// never a bare "pwsh": the WindowsApps execution alias is a 0-byte ConPTY
// stub. Callers should resolve via pwshBinaryPath(t), not this constant
// directly, so a machine without pwsh (e.g. Linux) skips fast instead of
// discovering the absence only after a doomed tmux boot + poll-timeout.
const smokePwshPath = `C:\Code\tools\powershell7\pwsh.exe`

// tmuxBinaryPath returns the tmux binary path from the environment or
// resolved via PATH, skipping the calling test when it is absent so a
// -tags=smoke run never hard-fails on a machine without the tool.
func tmuxBinaryPath(t *testing.T) string {
	t.Helper()
	if path := os.Getenv("LYX_REED_TMUX"); path != "" {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	binName := "tmux"
	if _, err := os.Stat(`C:\Windows\System32\cmd.exe`); err == nil {
		binName = "tmux.exe"
	}
	if path, err := exec.LookPath(binName); err == nil {
		return path
	}
	altName := "tmux"
	if binName == "tmux" {
		altName = "tmux.exe"
	}
	if path, err := exec.LookPath(altName); err == nil {
		return path
	}
	t.Skipf("tmux not found in PATH or LYX_REED_TMUX; checked: %s, %s", binName, altName)
	return ""
}

// pwshBinaryPath returns the PowerShell 7 binary path (LYX_REED_PWSH override
// or the smokePwshPath default), skipping the calling test immediately when
// it is absent. Callers must only reach this from a runtime.GOOS == "windows"
// branch: the probes it backs (WMI process trees, PEB cwd reads via P/Invoke
// into ntdll.dll/kernel32.dll) are Windows-only by construction, and on
// Linux the same probes are answered natively via /proc (see
// smoke_proctree.go) without any pwsh dependency at all — so on Linux this
// function is simply never called, not skipped.
func pwshBinaryPath(t *testing.T) string {
	t.Helper()
	path := smokePwshPath
	if override := os.Getenv("LYX_REED_PWSH"); override != "" {
		path = override
	}
	if _, err := os.Stat(path); err != nil {
		t.Skipf("pwsh not found at %s (set LYX_REED_PWSH to override): %v", path, err)
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

// waitServerGone blocks until `tmux -L socket has-session -t session` exits
// non-zero (the server/session is gone), or fails the test after a timeout.
// tmux's kill-server is asynchronous — it returns before the socket is
// released — so a test that simulates a crash must wait for the server to
// actually die before exercising recovery, or it races the teardown. The
// deadline is saturation-sized: the teardown is ~1s quiet, but concurrent
// suites pegging the CPU have starved fixed 5s waits of this shape.
func waitServerGone(t *testing.T, tmuxPath, socket, session string) {
	t.Helper()
	const timeout = 30 * time.Second
	deadline := time.Now().Add(timeout)
	for {
		if err := exec.Command(tmuxPath, "-L", socket, "has-session", "-t", session).Run(); err != nil {
			return // non-zero exit: server/session gone
		}
		if time.Now().After(deadline) {
			t.Fatalf("tmux server still up %s after kill-server (socket %s)", timeout, socket)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// listPaneLines returns the session's list-panes rows as
// "<pane_id> <pane_dead> <pane_top> <pane_height>" strings.
func listPaneLines(t *testing.T, tmuxPath, socket, session string) []string {
	t.Helper()
	out, err := exec.Command(tmuxPath, "-L", socket, "list-panes", "-t", session,
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

// socketAndSession reads the socket and session names from the current status.
func socketAndSession(t *testing.T) (socket, session string) {
	t.Helper()
	return socketAndSessionIn(t, "")
}

// socketAndSessionIn is socketAndSession driven through the RunCLIIn seam; see addStrandIn for what
// the cwd argument means.
func socketAndSessionIn(t *testing.T, cwd string) (socket, session string) {
	t.Helper()
	var out bytes.Buffer
	if code := RunCLIIn(cwd, &out, []string{"status"}); code != 0 {
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

// smokeClaudeModel is the model every real `claude` process this package spawns must run on.
// The suite's Claude-adjacent assertions are about reed (env hygiene on the server spawn, opaque
// resumeCmd replay), never about model capability, so the cheapest model is always the right one —
// and leaving it unpinned silently bills the operator's default model on every sweep.
const smokeClaudeModel = "haiku"

// smokeReapLaunchCmd returns the OS-appropriate long-running command line
// the pane-child-reap fixtures (TestSmokeDownReapsPaneChildProcesses,
// TestSmokeDownLeavesNoTmuxOnSocket, TestSmokeRemoveReapsRemovedPaneChildProcesses,
// TestSmokeDownInOneWorktreeLeavesSiblingSessionAlive) type into a pane: a
// long-lived pwsh host on Windows, `sleep 300` on POSIX. reed types cmdStr
// literally into the pane's own shell (send-keys -l, never exec'd directly —
// see spawn.go's launchStrandLocked), so #{pane_pid} is always that shell
// (bash on POSIX per the config template), not cmdStr's own process; a
// command that actually runs gives the reap assertions a REAL child of that
// shell to find and track, meaningfully exercising "reap the whole subtree,
// not just #{pane_pid}" rather than trivially passing because the shell
// itself (with nothing running under it) was the only thing tmux ever had
// to kill.
func smokeReapLaunchCmd() string {
	if runtime.GOOS == "windows" {
		return "pwsh -NoExit -Command Write-Host ready"
	}
	return "sleep 300"
}

// smokeMarkerLaunchCmd returns the OS-appropriate long-running command line
// that prints marker into the pane and then stays alive, so a later capture
// (or a nested attach, per TestSmokeAttachRendersInsideHarnessPane) can find
// it. `exec` on the POSIX branch replaces the inner bash with sleep rather
// than leaving a bash-parent-of-sleep pair, mirroring pwsh -NoExit's single
// long-lived process shape.
func smokeMarkerLaunchCmd(marker string) string {
	if runtime.GOOS == "windows" {
		return fmt.Sprintf("pwsh -NoExit -Command Write-Host %s", marker)
	}
	return fmt.Sprintf("bash -c 'echo %s; exec sleep 300'", marker)
}

// harnessShellBinaryPath returns the interactive pane-shell binary
// TestSmokeAttachRendersInsideHarnessPane boots its private harness session
// with: pwsh on Windows (via pwshBinaryPath), bash on POSIX (LYX_REED_SHELL
// override or PATH lookup, skipping the test if absent). This is a real,
// generically-available interactive shell to host the nested attach
// handover — not a pwsh-specific probe — so unlike pwshBinaryPath it has a
// meaningful POSIX branch instead of being Windows-only.
func harnessShellBinaryPath(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		return pwshBinaryPath(t)
	}
	if override := os.Getenv("LYX_REED_SHELL"); override != "" {
		return override
	}
	path, err := exec.LookPath("bash")
	if err != nil {
		t.Skipf("no bash found on PATH for the harness pane shell (set LYX_REED_SHELL to override): %v", err)
	}
	return path
}

// smokeAttachInvokeLine returns the OS-appropriate command line the harness
// pane types to unset TMUX_SESSION (tmux refuses to nest a client into a
// session it is itself running inside otherwise), run lyxExe's attach
// handover, and echo its exit code — pwsh syntax on Windows,
// posix syntax elsewhere.
func smokeAttachInvokeLine(lyxExe string) string {
	if runtime.GOOS == "windows" {
		return fmt.Sprintf(`$env:TMUX_SESSION=$null; & '%s' reed attach; Write-Host ATTACH-EXIT:$LASTEXITCODE`, lyxExe)
	}
	return fmt.Sprintf(`unset TMUX_SESSION; '%s' reed attach; echo ATTACH-EXIT:$?`, lyxExe)
}

// smokeInvokeLine returns the OS-appropriate command line that runs bin with
// args typed literally into the pane's own shell: pwsh's call operator (`&
// 'bin' 'arg' ...`) on Windows, direct invocation (`'bin' 'arg' ...`) on
// POSIX — `&` there is bash's BACKGROUND-job operator, not a call operator,
// so a bare leading `&` is a hard syntax error (verified:
// `bash -c "& 'echo' 'hi'"` → "syntax error near unexpected token `&'"),
// which is why TestSmokeClaudeResumeRecallsCodeword's claude launch/resume
// command lines never actually ran claude at all on Linux before this fix —
// the pane just showed a syntax error, and the test then waited its full
// timeout for a transcript a process that never started could never write.
func smokeInvokeLine(bin string, args ...string) string {
	quoted := make([]string, 0, len(args)+1)
	quoted = append(quoted, "'"+bin+"'")
	for _, a := range args {
		quoted = append(quoted, "'"+a+"'")
	}
	if runtime.GOOS == "windows" {
		return "& " + strings.Join(quoted, " ")
	}
	return strings.Join(quoted, " ")
}

// addStrand runs `add` with the given extra flags and returns the new guid.
func addStrand(t *testing.T, cmdStr string, extra ...string) string {
	t.Helper()
	return addStrandIn(t, "", cmdStr, extra...)
}

// addStrandIn is addStrand driven through the RunCLIIn seam: cwd is seeded into the execution
// context rather than into the process, so a test can drive reed for a worktree it is not standing
// in.
// An empty cwd means "read the process cwd", exactly as RunCLIIn itself documents.
func addStrandIn(t *testing.T, cwd, cmdStr string, extra ...string) string {
	t.Helper()
	var out bytes.Buffer
	args := append([]string{"add", "--cmd", cmdStr}, extra...)
	if code := RunCLIIn(cwd, &out, args); code != 0 {
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

// serverPID asks tmux for the server's OS pid via the #{pid} format variable.
func serverPID(t *testing.T, tmuxPath, socket, session string) int {
	t.Helper()
	out, err := exec.Command(tmuxPath, "-L", socket, "display-message", "-p", "-t", session, "#{pid}").Output()
	if err != nil {
		t.Fatalf("display-message #{pid}: %v", err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil {
		t.Fatalf("parse server pid %q: %v", out, err)
	}
	return pid
}

// processGone reports whether pid no longer names a running process.
func processGone(pid int) bool {
	if runtime.GOOS != "windows" {
		return !posixProcessAlive(pid)
	}
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
// and their full descendant subtrees.
func paneProcessTree(t *testing.T, tmuxPath, socket, session string) []int {
	t.Helper()
	out, err := exec.Command(tmuxPath, "-L", socket, "list-panes", "-t", session, "-F", "#{pane_pid}").Output()
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
	if runtime.GOOS != "windows" {
		intRoots := make([]int, 0, len(roots))
		for _, r := range roots {
			pid, _ := strconv.Atoi(r)
			intRoots = append(intRoots, pid)
		}
		return linuxDescendantClosure(intRoots)
	}
	pwshPath := pwshBinaryPath(t)
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

// panePaneSubtree returns the OS pids of a single pane's child process and
// its full descendant subtree.
func panePaneSubtree(t *testing.T, tmuxPath, socket, session, paneID string) []int {
	t.Helper()
	out, err := exec.Command(tmuxPath, "-L", socket, "list-panes", "-t", session,
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
	rootPID, perr := strconv.Atoi(root)
	if perr != nil {
		t.Fatalf("parse pane pid %q: %v", root, perr)
	}
	if runtime.GOOS != "windows" {
		return linuxDescendantClosure([]int{rootPID})
	}
	pwshPath := pwshBinaryPath(t)
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

// hubHolders returns every process whose current working directory is inside dir.
func hubHolders(t *testing.T, dir string) []hubHolder {
	t.Helper()
	if runtime.GOOS != "windows" {
		return linuxHubHolders(dir)
	}
	pwshPath := pwshBinaryPath(t)
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
// releasable before the framework's TempDir RemoveAll.
func deferHubRelease(t *testing.T, hub string) {
	t.Helper()
	t.Cleanup(func() {
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

		if !waitReleased(10 * time.Second) {
			deadline := time.Now().Add(90 * time.Second)
			for {
				for _, h := range hubHolders(t, hub) {
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

		if prev != "" && !strings.HasPrefix(strings.ToLower(prev), strings.ToLower(hub)) {
			_ = os.Chdir(prev)
		}
	})
}

// tmuxSocketPids returns the OS pids of every tmux process whose command
// line names the given -L socket.
func tmuxSocketPids(t *testing.T, tmuxPath, socket string) []int {
	t.Helper()
	if runtime.GOOS != "windows" {
		return linuxTmuxSocketPids(tmuxPath, socket)
	}
	pwshPath := pwshBinaryPath(t)
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

// pidClosure expands roots to roots-plus-their-transitive-descendant pids.
func pidClosure(t *testing.T, roots []int) []int {
	t.Helper()
	if len(roots) == 0 {
		return nil
	}
	if runtime.GOOS != "windows" {
		return linuxDescendantClosure(roots)
	}
	pwshPath := pwshBinaryPath(t)
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

// reapHarnessServer tears down the test's private harness tmux server and
// waits for its process subtree to exit.
func reapHarnessServer(t *testing.T, tmuxPath, socket string) {
	t.Helper()
	subtree := pidClosure(t, tmuxSocketPids(t, tmuxPath, socket))
	_ = exec.Command(tmuxPath, "-L", socket, "kill-server").Run()
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

// capturePane returns the rendered content of the target pane on socket.
func capturePane(t *testing.T, tmuxPath, socket, target string) string {
	t.Helper()
	out, err := exec.Command(tmuxPath, "-L", socket, "capture-pane", "-p", "-t", target).Output()
	if err != nil {
		t.Fatalf("capture-pane -t %s: %v", target, err)
	}
	return string(out)
}

// capturePaneScrollback returns the target pane's full scrollback (via -S -), not merely its
// visible viewport.
// This is deliberately a separate helper from capturePane rather than an edit to it: capturePane
// passes no -S and captures the visible viewport only, which is what its existing callers assert
// against, whereas the header-noise assertions need the full scrollback that -S - reaches.
func capturePaneScrollback(t *testing.T, tmuxPath, socket, target string) string {
	t.Helper()
	out, err := exec.Command(tmuxPath, "-L", socket, "capture-pane", "-p", "-S", "-", "-t", target).Output()
	if err != nil {
		t.Fatalf("capture-pane -S - -t %s: %v", target, err)
	}
	return string(out)
}

// sendKeysLine types text literally into the target pane and submits it with Enter.
func sendKeysLine(t *testing.T, tmuxPath, socket, target, text string) {
	t.Helper()
	if err := exec.Command(tmuxPath, "-L", socket, "send-keys", "-t", target, "-l", text).Run(); err != nil {
		t.Fatalf("send-keys -l %q: %v", text, err)
	}
	if err := exec.Command(tmuxPath, "-L", socket, "send-keys", "-t", target, "Enter").Run(); err != nil {
		t.Fatalf("send-keys Enter: %v", err)
	}
}

// pollPaneContains polls capture-pane until the target pane contains want.
func pollPaneContains(t *testing.T, tmuxPath, socket, target, want string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	last := ""
	for {
		last = capturePane(t, tmuxPath, socket, target)
		if strings.Contains(last, want) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("pane %s never showed %q within %s; last capture:\n%s", target, want, timeout, last)
		}
		time.Sleep(500 * time.Millisecond)
	}
}

var _, smokeTestFile, _, _ = runtime.Caller(0)

// buildLyxBinary compiles cmd/lyx into a temp dir and returns its path.
func buildLyxBinary(t *testing.T) string {
	t.Helper()
	return buildLyxBinaryWithLDFlags(t, "")
}

// buildLyxBinaryWithLDFlags compiles cmd/lyx into a temp dir with the given -ldflags value (omitted
// from the build argv entirely when ldflags is empty) and returns its path.
// The dev channel stamp -X github.com/Knatte18/loomyard/internal/buildinfo.Channel=dev is what makes
// stencilstore.ModeFor(buildinfo.IsDev()) return ModeDev: buildinfo.Channel is "" for a plain `go
// build`, so an unstamped binary is production mode and never emits the dev-refusal warn.
func buildLyxBinaryWithLDFlags(t *testing.T, ldflags string) string {
	t.Helper()
	repoRoot, err := filepath.Abs(filepath.Join(filepath.Dir(smokeTestFile), "..", ".."))
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	lyxExe := filepath.Join(t.TempDir(), "lyx.exe")
	args := []string{"build", "-o", lyxExe}
	if ldflags != "" {
		args = append(args, "-ldflags", ldflags)
	}
	args = append(args, "./cmd/lyx")
	cmd := exec.Command("go", args...)
	cmd.Dir = repoRoot
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build ./cmd/lyx: %v\n%s", err, out)
	}
	return lyxExe
}

// paneEventuallyContains reports whether the target pane comes to contain want.
func paneEventuallyContains(t *testing.T, tmuxPath, socket, target, want string, timeout time.Duration) bool {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		if strings.Contains(capturePane(t, tmuxPath, socket, target), want) {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(1 * time.Second)
	}
}

// claudeProjectDir returns the ~/.claude/projects/<encoded-cwd> directory for cwd dir.
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

// claudeTranscriptFiles returns the set of every *.jsonl transcript path under projectDir.
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

// waitTranscriptStable blocks until a new transcript appears in projectDir and stops growing.
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

// claudeBinaryPath returns the claude CLI's path from the environment or PATH.
func claudeBinaryPath(t *testing.T) string {
	t.Helper()
	if path := os.Getenv("LYX_REED_CLAUDE"); path != "" {
		return path
	}
	path, err := exec.LookPath("claude")
	if err != nil {
		t.Skip("claude not found on PATH")
	}
	return path
}

// materializeSibling clones h's warp bare origin into a second worktree inside the primary hub
// directory and seeds reed config into it.
// It clones to filepath.Join(h.Path, name), a direct child of the hub directory, matching
// h.PrimeWorktree()'s own parentage — not filepath.Join(h.Container, name), which is the hub
// directory's own parent and would give lyxcwd.Resolve a different HubPath for the sibling than
// for the prime worktree, breaking the shared-per-hub-socket invariant the "sibling" name promises.
// The clone is a plain repo, not a hub, so it stays on gitkit.SeedConfig rather than
// hubforge.SeedConfig — the third, ad-hoc "sibling" resolution of the SeedConfig triage.
func materializeSibling(t *testing.T, h *hubforge.Hub, name string) string {
	t.Helper()
	sibling := filepath.Join(h.Path, name)
	gitkit.MustRun(t, h.Path, "git", "clone", h.WarpBare, sibling)
	gitkit.MustRun(t, sibling, "git", "config", "user.email", "test@test.com")
	gitkit.MustRun(t, sibling, "git", "config", "user.name", "Test")
	gitkit.SeedConfig(t, sibling, map[string]string{
		"reed": reedengine.ConfigTemplate(),
	})
	return sibling
}

// mustChdir changes the process working directory or fails the test.
func mustChdir(t *testing.T, dir string) {
	t.Helper()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir %s: %v", dir, err)
	}
}

// sessionAlive reports whether the named session currently exists on the socket.
func sessionAlive(tmuxPath, socket, session string) bool {
	return exec.Command(tmuxPath, "-L", socket, "has-session", "-t", session).Run() == nil
}

// waitSessionUp blocks until the named session answers has-session on the socket.
func waitSessionUp(t *testing.T, tmuxPath, socket, session string) {
	t.Helper()
	const timeout = 60 * time.Second
	deadline := time.Now().Add(timeout)
	for {
		if sessionAlive(tmuxPath, socket, session) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("session %s never came up on socket %s within %s", session, socket, timeout)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// paneLiveOnSession reports whether paneID is live on the session.
func paneLiveOnSession(lines []string, paneID string) bool {
	for _, l := range lines {
		fields := strings.Fields(l)
		if len(fields) >= 2 && fields[0] == paneID && fields[1] == "0" {
			return true
		}
	}
	return false
}

// paneRootPID returns a pane's root process id on the socket.
func paneRootPID(t *testing.T, tmuxPath, socket, session, paneID string) int {
	t.Helper()
	out, err := exec.Command(tmuxPath, "-L", socket, "list-panes", "-t", session,
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

// paneIDForStrand runs status and returns the tracked strand's live pane id.
func paneIDForStrand(t *testing.T, guid string) string {
	t.Helper()
	return paneIDForStrandIn(t, "", guid)
}

// paneIDForStrandIn is paneIDForStrand driven through the RunCLIIn seam; see addStrandIn for what
// the cwd argument means.
func paneIDForStrandIn(t *testing.T, cwd, guid string) string {
	t.Helper()
	var out bytes.Buffer
	if code := RunCLIIn(cwd, &out, []string{"status"}); code != 0 {
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

// paneCurrentPath asks tmux for a pane's own current working directory.
func paneCurrentPath(t *testing.T, tmuxPath, socket, paneID string) string {
	t.Helper()
	out, err := exec.Command(tmuxPath, "-L", socket, "display-message", "-p", "-t", paneID, "#{pane_current_path}").Output()
	if err != nil {
		t.Fatalf("display-message #{pane_current_path} for %s: %v", paneID, err)
	}
	return strings.TrimSpace(string(out))
}

// harnessOnlyPaneID returns the sole pane id of a freshly-booted harness session.
func harnessOnlyPaneID(t *testing.T, tmuxPath, socket, session string) string {
	t.Helper()
	out, err := exec.Command(tmuxPath, "-L", socket, "list-panes", "-t", session, "-F", "#{pane_id}").Output()
	if err != nil {
		t.Fatalf("list-panes #{pane_id}: %v", err)
	}
	lines := strings.Fields(strings.TrimSpace(string(out)))
	if len(lines) != 1 {
		t.Fatalf("harness session %s has %d panes; want exactly 1 (output: %q)", session, len(lines), out)
	}
	return lines[0]
}

// serverProcCountForSession counts the server processes backing a session on the socket.
func serverProcCountForSession(t *testing.T, tmuxPath, socket, session string) int {
	t.Helper()
	if runtime.GOOS != "windows" {
		if exec.Command(tmuxPath, "-L", socket, "has-session", "-t", session).Run() != nil {
			return 0
		}
		return len(linuxTmuxSocketPids(tmuxPath, socket))
	}
	pwshPath := pwshBinaryPath(t)
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

// waitServerProcCountForSession polls serverProcCountForSession until it equals want.
func waitServerProcCountForSession(t *testing.T, tmuxPath, socket, session string, want int) {
	t.Helper()
	const timeout = 60 * time.Second
	deadline := time.Now().Add(timeout)
	for {
		if got := serverProcCountForSession(t, tmuxPath, socket, session); got == want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("session %s backing-server count never reached %d within %s (got %d)", session, want, timeout, serverProcCountForSession(t, tmuxPath, socket, session))
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// assertSiblingStaysLive polls for dur, failing if the sibling's session, pane,
// server pid, or agent root process drop.
func assertSiblingStaysLive(t *testing.T, tmuxPath, socket, session, paneID string, wantServerPID, agentPID int, dur time.Duration) {
	t.Helper()
	deadline := time.Now().Add(dur)
	for {
		if !sessionAlive(tmuxPath, socket, session) {
			t.Fatalf("sibling session %s died after down in the other worktree — down killed the shared-socket server set", session)
		}
		if lines := listPaneLines(t, tmuxPath, socket, session); !paneLiveOnSession(lines, paneID) {
			t.Fatalf("sibling pane %s not live after down in the other worktree; panes=%v", paneID, lines)
		}
		if pid := serverPID(t, tmuxPath, socket, session); pid != wantServerPID {
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

// waitSocketFreeOfTmux polls until no tmux process names the socket.
func waitSocketFreeOfTmux(t *testing.T, tmuxPath, socket string) {
	t.Helper()
	const timeout = 30 * time.Second
	deadline := time.Now().Add(timeout)
	for {
		if pids := tmuxSocketPids(t, tmuxPath, socket); len(pids) == 0 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("tmux still on socket %s after %s: pids=%v", socket, timeout, tmuxSocketPids(t, tmuxPath, socket))
		}
		time.Sleep(100 * time.Millisecond)
	}
}
