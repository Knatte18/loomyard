// smoke_run_test.go holds the live round-trip test for `lym shuttle run`, TestSmokeShuttleRunWritesOutputAndCleans.
// It also holds the helpers the smoke files share, such as deferHubRelease for the orphaned-conhost hazard.
// The test skips unless tiergate selects tier 3 and a claude binary resolves.

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

	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/reedcli"
	"github.com/Knatte18/loomyard/internal/testkit/llmkit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// smokePwshPath is the PowerShell 7 binary the smoke helpers shell out to for the orphaned-conhost teardown probe.
// It is an explicit absolute path, never a bare "pwsh": the WindowsApps execution alias is a 0-byte ConPTY stub.
const smokePwshPath = `C:\Code\tools\powershell7\pwsh.exe`

// smokeClaudeModel is the model every real `claude` process this package spawns must run on.
// Every test checks something mechanical: a file's content, an envelope field, or whether a strand is still tracked.
// A stronger model buys none of that.
// Leaving the model unpinned only spent the account default's tokens on each real spawn.
// A test that needs model-specific behaviour must say so explicitly rather than silently omit the pin.
const smokeClaudeModel = "haiku"

// hubHolder is one process still holding the fixture hub as its current working directory.
type hubHolder struct {
	pid  int
	name string
}

// hubHolders returns every process whose current working directory is inside dir.
// It reads the directory from each PEB (RTL_USER_PROCESS_PARAMETERS.CurrentDirectory) via NtQueryInformationProcess.
// Win32_Process exposes no cwd column, so this PEB read is the only way to find the conhost.exe holders.
// It returns nil when nothing holds dir or the probe fails, and callers degrade to waiting.
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

// deferHubRelease registers a cleanup that makes the fixture hub directory releasable.
// The framework's TempDir RemoveAll runs after it, so RemoveAll never fails with a worktree-dir-in-use error.
// The holder in question is the conhost.exe the OS parents to psmux to host each pane's pseudo-console.
// reed never spawns it, and it is not a #{pane_pid} descendant.
// On a quiet machine it exits on its own a beat after its pane dies.
// Under CPU saturation it can be ORPHANED, and then it holds the hub cwd indefinitely.
// No fixed wait can ever out-last it, so the cleanup confirms rather than waits.
// It first allows a short grace for the self-exit path.
// Then it kills any conhost whose PEB cwd is inside the hub, which is safe because its console app is already gone.
// It keeps confirming until the hub actually renames.
// A non-conhost holder is a genuine leak: the test fails loudly instead of masking it.
// Register it before t.Chdir and before any later t.Cleanup, so that it runs after them.
// By then the cwd is already restored out of the hub.
func deferHubRelease(t *testing.T, hub string) {
	t.Helper()
	t.Cleanup(func() {
		// A process cannot rename its own cwd, so make sure ours is not in hub while probing.
		// Then restore it, so a later test's cwd-relative work is not corrupted.
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

		// Grace phase: the healthy path, where the ConPTY host exits on its own moments after its pane died.
		if !waitReleased(10 * time.Second) {
			// Escalation phase: identify the actual holders.
			// Orphaned conhosts are killed; a conhost can appear late while the OS teardown is starved,
			// so the holders are re-scanned each round.
			// Anything else holding the hub is a real leak the kill must not paper over.
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

		// Restore the original cwd only when it is outside hub, which is the normal case:
		// t.Chdir's own restore has already run when this cleanup does.
		// Never chdir back into a hub that is about to be removed.
		if prev != "" && !strings.HasPrefix(strings.ToLower(prev), strings.ToLower(hub)) {
			_ = os.Chdir(prev)
		}
	})
}

// reedStatusStrand runs `lym reed status` in the current worktree and returns the tracked strand record for guid.
// The second result is false when status does not list guid.
// Smoke tests use it to confirm a run's post-outcome reed state.
func reedStatusStrand(t *testing.T, guid string) (map[string]any, bool) {
	t.Helper()
	var out bytes.Buffer
	if code := reedcli.RunCLI(&out, []string{"status"}); code != 0 {
		t.Fatalf("reed status = %d; want 0, output: %s", code, out.String())
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

// TestSmokeShuttleRunWritesOutputAndCleans drives one full `lym shuttle run` against a real claude in a real tmux pane.
// The run blocks until the agent writes the requested output file and the run loop classifies it "done".
// It asserts the file-contract output and the envelope's outcome,
// and that both the strand and the run directory are cleaned up afterward.
func TestSmokeShuttleRunWritesOutputAndCleans(t *testing.T) {
	tiergate.Require(t, 3)
	llmkit.Claude(t, "LYM_REED_CLAUDE")

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	deferHubRelease(t, h.Path)
	registerReedServer(t, h)
	t.Chdir(h.PrimeWorktree())
	t.Cleanup(func() {
		var buf bytes.Buffer
		reedcli.RunCLI(&buf, []string{"down"})
	})

	// `up` boots the substrate first.
	// A strand must exist in an up'd session before `AddStrand` can bind a pane to it.
	var reedOut bytes.Buffer
	if code := reedcli.RunCLI(&reedOut, []string{"up"}); code != 0 {
		t.Fatalf("reed up = %d; want 0, output: %s", code, reedOut.String())
	}

	outputPath := filepath.Join(h.PrimeWorktree(), "smoke-run-output.txt")
	prompt := fmt.Sprintf("write exactly DONE to %s then stop", outputPath)

	var out bytes.Buffer
	code := RunCLI(&out, []string{
		"run",
		"--prompt", prompt,
		"--output-file", outputPath,
		"--model", smokeClaudeModel,
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
	if strand, found := reedStatusStrand(t, guid); found {
		t.Errorf("reed status still lists strand %s after a \"done\" outcome; want it removed; strand: %v", guid, strand)
	}

	runDir, _ := result["runDir"].(string)
	if runDir == "" {
		t.Fatalf("run result missing runDir: %v", result)
	}
	if _, err := os.Stat(runDir); !os.IsNotExist(err) {
		t.Errorf("run dir %s still exists after a \"done\" outcome (stat err=%v); want it removed", runDir, err)
	}
}
