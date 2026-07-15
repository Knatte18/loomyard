//go:build smoke

// smoke_run_test.go is shuttlecli's live-integration smoke suite entry
// point: the shared helpers (claude binary discovery, the orphaned-conhost
// teardown guard) used across this file, smoke_guardrail_test.go, and
// smoke_interrupt_test.go, plus TestSmokeShuttleRunWritesOutputAndCleans —
// the full round-trip proof that `lyx shuttle run` drives a REAL claude in a
// REAL tmux pane to a "done" outcome, writes the file-contract output, and
// cleans up the strand and run directory afterward. Follows the
// internal/muxcli/smoke_*.go conventions: opt-in via -tags smoke, skipped
// when no claude binary resolves, deferHubRelease against the
// orphaned-conhost hazard. The helpers here are reproduced (not imported)
// from muxcli's smoke files, per the smoke-files-are-self-contained
// convention.

package shuttlecli

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
	"github.com/Knatte18/loomyard/internal/muxcli"
	"github.com/Knatte18/loomyard/internal/muxengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// smokePwshPath is the PowerShell 7 binary the smoke helpers shell out to
// for the orphaned-conhost teardown probe. Explicit absolute path, never a
// bare "pwsh": the WindowsApps execution alias is a 0-byte ConPTY stub.
const smokePwshPath = `C:\Code\tools\powershell7\pwsh.exe`

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

// hubHolder is one process still holding the fixture hub as its current
// working directory, as reported by hubHolders.
type hubHolder struct {
	pid  int
	name string
}

// hubHolders returns every process whose current working directory is
// inside dir, read from each process's PEB (RTL_USER_PROCESS_PARAMETERS.
// CurrentDirectory via NtQueryInformationProcess) — the only way to find the
// conhost.exe holders, since Win32_Process exposes no cwd column. Returns
// nil when nothing holds dir or the probe fails (callers degrade to
// waiting).
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
// releasable before the framework's TempDir RemoveAll — which runs AFTER
// this cleanup — so RemoveAll never fails with a worktree-dir-in-use error.
// The holder in question is the conhost.exe the OS parents to psmux to host
// each pane's pseudo-console: mux never spawns it, it is not a #{pane_pid}
// descendant, and on a quiet machine it exits on its own a beat after its
// pane dies — but under CPU saturation it can be ORPHANED and then holds the
// hub cwd indefinitely, so no fixed wait can ever out-last it. The cleanup
// therefore confirms rather than waits: a short grace for the self-exit
// path, then it kills any conhost whose PEB cwd is inside the hub (safe —
// its console app is already gone) and keeps confirming until the hub
// actually renames. A NON-conhost holder is a genuine leak and fails the
// test loudly instead of being masked. Registered before t.Chdir and the
// down cleanup so it runs AFTER them (cwd already restored out of hub) but
// BEFORE RemoveAll.
func deferHubRelease(t *testing.T, hub string) {
	t.Helper()
	t.Cleanup(func() {
		// A process cannot rename its own cwd; make sure ours is not in hub
		// while probing, then restore it so a later test's cwd-relative work
		// is not corrupted.
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

// muxStatusStrand runs `lyx mux status` in the current worktree and returns
// the tracked strand record for guid, or ok=false if status does not list
// it — the shared assertion helper this file's and smoke_guardrail_test.go's
// tests use to confirm a run's post-outcome mux state.
func muxStatusStrand(t *testing.T, guid string) (map[string]any, bool) {
	t.Helper()
	var out bytes.Buffer
	if code := muxcli.RunCLI(&out, []string{"status"}); code != 0 {
		t.Fatalf("mux status = %d; want 0, output: %s", code, out.String())
	}
	var result map[string]any
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatalf("parse status result: %v; output: %s", err, out.String())
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

// TestSmokeShuttleRunWritesOutputAndCleans drives one full `lyx shuttle run`
// against a REAL claude in a REAL tmux pane: `up` boots the substrate first
// (a strand must exist in an up'd session before AddStrand can bind a pane
// to it), then `run` blocks until the agent writes the requested output file
// and the run loop classifies it "done". Asserts the file-contract output,
// the envelope's outcome, and that both the strand and run directory are
// cleaned up afterward — the same happy path the sandbox suite's S1
// scenario exercises manually.
func TestSmokeShuttleRunWritesOutputAndCleans(t *testing.T) {
	claudeBinaryPath(t)

	fixture := lyxtest.CopyPaired(t)
	lyxtest.SeedConfig(t, fixture.Hub, map[string]string{
		"shuttle": shuttleengine.ConfigTemplate(),
		"mux":     muxengine.ConfigTemplate(),
	})
	deferHubRelease(t, fixture.Hub)
	t.Chdir(fixture.Hub)
	t.Cleanup(func() {
		var buf bytes.Buffer
		muxcli.RunCLI(&buf, []string{"down"})
	})

	// up: boots the substrate. A strand must exist in an up'd session
	// before shuttle's AddStrand can bind it to a pane.
	var muxOut bytes.Buffer
	if code := muxcli.RunCLI(&muxOut, []string{"up"}); code != 0 {
		t.Fatalf("mux up = %d; want 0, output: %s", code, muxOut.String())
	}

	outputPath := filepath.Join(fixture.Hub, "smoke-run-output.txt")
	prompt := fmt.Sprintf("write exactly DONE to %s then stop", outputPath)

	var out bytes.Buffer
	code := RunCLI(&out, []string{
		"run",
		"--prompt", prompt,
		"--output-file", outputPath,
		"--timeout", "5m",
	})
	if code != 0 {
		t.Fatalf("shuttle run = %d; want 0, output: %s", code, out.String())
	}

	var result map[string]any
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatalf("parse run result: %v; output: %s", err, out.String())
	}
	if outcome, _ := result["outcome"].(string); outcome != "done" {
		t.Fatalf("run outcome = %q; want \"done\"; output: %s", outcome, out.String())
	}

	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read output file: %v", err)
	}
	if got := strings.TrimSpace(string(data)); got != "DONE" {
		t.Errorf("output file content = %q; want \"DONE\"", got)
	}

	guid, _ := result["guid"].(string)
	if guid == "" {
		t.Fatalf("run result missing guid: %v", result)
	}
	if strand, found := muxStatusStrand(t, guid); found {
		t.Errorf("mux status still lists strand %s after a \"done\" outcome; want it removed; strand: %v", guid, strand)
	}

	runDir, _ := result["runDir"].(string)
	if runDir == "" {
		t.Fatalf("run result missing runDir: %v", result)
	}
	if _, err := os.Stat(runDir); !os.IsNotExist(err) {
		t.Errorf("run dir %s still exists after a \"done\" outcome (stat err=%v); want it removed", runDir, err)
	}
}
