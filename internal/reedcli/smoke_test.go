// smoke_test.go is the shared smoke-test harness for the smoke test files in this package.
// It holds the helpers: binary discovery, live-tmux process and pane probes, transcript watching, and fixture wiring.
// Those files drive the live-tmux behaviors through RunCLIIn against a real server.
// Hermetic tests prove nothing on these paths:
// tmux's real semantics show up only live: positional select-layout, silent split failures, corpse panes, and async kill-server.
// claude's real transcript persistence does too.
// Excluded from the default `go test ./internal/reedcli/...`; runs under `go test -tags tmux`, or `-tags llm` for the claude resume test.
// The file carries `tmux || llm` because both tiers' files use its helpers.

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
	"github.com/Knatte18/loomyard/internal/testkit/tmuxkit"
)

// smokePwshPath is the default PowerShell 7 binary the smoke helpers use for Windows process-table and PEB probes.
// It is an explicit absolute path, never a bare "pwsh": the WindowsApps execution alias is a 0-byte ConPTY stub.
// Callers should resolve it via pwshBinaryPath(t), not this constant directly.
// That way a machine without pwsh (e.g. Linux) skips fast.
// Otherwise the absence surfaces only after a doomed tmux boot and poll timeout.
const smokePwshPath = `C:\Code\tools\powershell7\pwsh.exe`

// tmuxBinaryPath returns the tmux binary path from the environment or resolved via PATH.
// It skips the calling test when the binary is absent.
// A -tags=tmux run then never hard-fails on a machine without the tool.
func tmuxBinaryPath(t *testing.T) string {
	t.Helper()
	if path := os.Getenv("LYM_REED_TMUX"); path != "" {
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
	t.Skipf("tmux not found in PATH or LYM_REED_TMUX; checked: %s, %s", binName, altName)
	return ""
}

// pwshBinaryPath returns the PowerShell 7 binary path, from the LYM_REED_PWSH override or the smokePwshPath default.
// It skips the calling test immediately when the binary is absent.
// Callers must reach this only from a runtime.GOOS == "windows" branch.
// The probes it backs read WMI process trees and PEB cwd via P/Invoke into ntdll.dll and kernel32.dll.
// They are Windows-only by construction.
// On Linux the same probes read /proc (see smoke_proctree_test.go) and need no pwsh.
// So on Linux this function is never called, rather than skipped.
func pwshBinaryPath(t *testing.T) string {
	t.Helper()
	path := smokePwshPath
	if override := os.Getenv("LYM_REED_PWSH"); override != "" {
		path = override
	}
	if _, err := os.Stat(path); err != nil {
		t.Skipf("pwsh not found at %s (set LYM_REED_PWSH to override): %v", path, err)
	}
	return path
}

// statusStrand returns the tracked strand with the given guid from a `status` JSON envelope.
// It also reports whether the strand was found.
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

// waitServerGone blocks until `tmux -L socket has-session -t session` exits non-zero.
// A non-zero exit means the server or session is gone; the test fails after a timeout.
// tmux's kill-server is asynchronous: it returns before the socket is released.
// A test that simulates a crash must wait for the server to die before it exercises recovery, or it races the teardown.
// The deadline is sized for a saturated CPU, because concurrent suites have starved short fixed waits of this shape.
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

// listPaneLines returns the list-panes rows of every window of the session as
// "<pane_id> <pane_dead> <pane_top> <pane_height>" strings.
func listPaneLines(t *testing.T, tmuxPath, socket, session string) []string {
	t.Helper()
	out, err := exec.Command(tmuxPath, "-L", socket, "list-panes", "-s", "-t", session,
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

// socketAndSessionIn returns the socket and session names that `status` reports, run from cwd through RunCLIIn.
// It also registers a cleanup that kills the tmux server on that socket.
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
	// This call does nothing for a test that registered the hub's key before its first boot.
	// Such a registration makes the server hermetic; this call covers a test that did not register the key.
	tmuxkit.KillOnCleanup(t, tmuxBinaryPath(t), socket)
	return socket, session
}

// smokeReapLaunchCmd returns the OS-appropriate long-running command line that TestSmokeTeardown's reap steps type.
// On Windows it is a long-lived pwsh host; on POSIX it is `sleep 300`.
// reed types the command into the pane's shell, so #{pane_pid} is that shell (bash on POSIX), not the command.
// The command must actually run, so the reap assertions have a real child of that shell to find and track;
// otherwise the reap would pass trivially, since tmux would kill only the idle shell.
func smokeReapLaunchCmd() string {
	if runtime.GOOS == "windows" {
		return "pwsh -NoExit -Command Write-Host ready"
	}
	return "sleep 300"
}

// smokeMarkerLaunchCmd returns the OS-appropriate command line that prints marker into the pane and then stays alive,
// so a later capture or attach can find it.
// On POSIX, `exec` replaces the inner bash with sleep, leaving one long-lived process, as pwsh -NoExit does.
func smokeMarkerLaunchCmd(marker string) string {
	if runtime.GOOS == "windows" {
		return fmt.Sprintf("pwsh -NoExit -Command Write-Host %s", marker)
	}
	return fmt.Sprintf("bash -c 'echo %s; exec sleep 300'", marker)
}

// harnessShellBinaryPath returns the interactive shell that the TestSmokeLifecycle harness session runs in.
// It is pwsh on Windows, and on POSIX it is bash, taken from the LYM_REED_SHELL override or a PATH lookup.
// The test is skipped if bash is absent.
// It hosts the nested attach handover, so it must be a real interactive shell.
func harnessShellBinaryPath(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		return pwshBinaryPath(t)
	}
	if override := os.Getenv("LYM_REED_SHELL"); override != "" {
		return override
	}
	path, err := exec.LookPath("bash")
	if err != nil {
		t.Skipf("no bash found on PATH for the harness pane shell (set LYM_REED_SHELL to override): %v", err)
	}
	return path
}

// smokeAttachInvokeLine returns the OS-appropriate line that the harness pane types to run lymExe's attach handover.
// The line first unsets TMUX_SESSION, because tmux refuses to nest a client into a session it runs inside;
// then it echoes the exit code.
// It uses pwsh syntax on Windows and POSIX syntax elsewhere.
func smokeAttachInvokeLine(lymExe string) string {
	if runtime.GOOS == "windows" {
		return fmt.Sprintf(`$env:TMUX_SESSION=$null; & '%s' reed attach; Write-Host ATTACH-EXIT:$LASTEXITCODE`, lymExe)
	}
	return fmt.Sprintf(`unset TMUX_SESSION; '%s' reed attach; echo ATTACH-EXIT:$?`, lymExe)
}

// smokeInvokeLine returns the OS-appropriate line that runs bin with args, typed literally into the pane's own shell.
// On Windows it uses pwsh's call operator, `& 'bin' 'arg' ...`.
// On POSIX it invokes directly, `'bin' 'arg' ...`.
// A leading `&` is bash's background-job operator, so it is a syntax error there.
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

// addStrandIn runs `add` with the given extra flags against cwd and returns the new guid.
// cwd becomes the working directory of the reed run rather than the process working directory,
// so a test can drive reed for a worktree it is not standing in.
// An empty cwd leaves the run on the process working directory.
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

// paneProcessTree returns the OS pids of the session's pane child processes and their full descendant subtrees.
func paneProcessTree(t *testing.T, tmuxPath, socket, session string) []int {
	t.Helper()
	out, err := exec.Command(tmuxPath, "-L", socket, "list-panes", "-s", "-t", session, "-F", "#{pane_pid}").Output()
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

// panePaneSubtree returns the OS pids of a single pane's child process and its full descendant subtree.
func panePaneSubtree(t *testing.T, tmuxPath, socket, session, paneID string) []int {
	t.Helper()
	out, err := exec.Command(tmuxPath, "-L", socket, "list-panes", "-s", "-t", session,
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

// hubHolder is one process still holding the fixture hub as its current working directory.
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

// deferHubRelease registers a cleanup that makes the fixture hub directory releasable.
// The cleanup runs before the framework's TempDir RemoveAll.
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

// tmuxSocketPids returns the OS pids of every tmux process whose command line names the given -L socket.
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

// reapHarnessServer tears down the test's private harness tmux server and waits for its process subtree to exit.
// A watchdog daemon an attach spawned in the subtree is killed rather than waited for.
// It detaches into its own session and exits only on its own production idle schedule.
func reapHarnessServer(t *testing.T, tmuxPath, socket string) {
	t.Helper()
	subtree := pidClosure(t, tmuxSocketPids(t, tmuxPath, socket))
	_ = exec.Command(tmuxPath, "-L", socket, "kill-server").Run()
	deadline := time.Now().Add(20 * time.Second)
	for _, pid := range subtree {
		if argv, ok := tmuxkit.ProcArgv(pid); runtime.GOOS != "windows" && ok && tmuxkit.IsWatchdog(argv) {
			if p, err := os.FindProcess(pid); err == nil {
				_ = p.Kill()
			}
			continue
		}
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

// materializeSibling clones h's code bare origin into a second worktree under the hub directory h.Path.
// The clone sits directly under h.Path, not under the hub's parent,
// so it resolves to the same hub as the prime worktree and shares its per-hub tmux socket.
// The clone is a plain repo rather than a hub, so it is seeded through gitkit.SeedConfig.
func materializeSibling(t *testing.T, h *hubforge.Hub, name string) string {
	t.Helper()
	sibling := filepath.Join(h.Path, name)
	gitkit.MustRun(t, h.Path, "git", "clone", h.CodeBare, sibling)
	gitkit.MustRun(t, sibling, "git", "config", "user.email", "test@test.com")
	gitkit.MustRun(t, sibling, "git", "config", "user.name", "Test")
	gitkit.SeedConfig(t, sibling, map[string]string{
		"reed": reedengine.ConfigTemplate(),
	})
	return sibling
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
	out, err := exec.Command(tmuxPath, "-L", socket, "list-panes", "-s", "-t", session,
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

// paneIDForStrandIn runs `status` with cwd as the working directory and returns the pane id of the tracked strand guid.
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
	out, err := exec.Command(tmuxPath, "-L", socket, "list-panes", "-s", "-t", session, "-F", "#{pane_id}").Output()
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

// assertSiblingStaysLive polls for dur and fails if the sibling loses its session, pane, server pid, or agent process.
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
