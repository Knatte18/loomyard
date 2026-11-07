// smoke_judge_test.go is treadleengine's opt-in live smoke test for the circling-check judge.
// It runs one real judge call through a real claude in a real tmux pane, and checks the verdict and handoff files.
// It is an external test package, since an in-package test importing internal/hubforge would close a compile cycle.

package treadleengine_test

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/contracts/stencils"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/reedcli"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine/claudeengine"
	"github.com/Knatte18/loomyard/internal/shuttleengines"
	"github.com/Knatte18/loomyard/internal/stencilstore"
	"github.com/Knatte18/loomyard/internal/testkit/llmkit"
	"github.com/Knatte18/loomyard/internal/testkit/shuttlefake"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
	"github.com/Knatte18/loomyard/internal/testkit/tmuxkit"
	"github.com/Knatte18/loomyard/internal/treadleengine"
)

// smokePwshPath is the PowerShell 7 binary for the orphaned-conhost teardown probe.
const smokePwshPath = `C:\Code\tools\powershell7\pwsh.exe`

// seedHubStencils fills the stencils directory of hub with every shipped stencil and returns that directory.
// It runs the same stencilstore.Reconcile pass as the lym root pre-run.
// Seeding is needed because hubforge.NewHub builds a fabric, not a seeded board.
// Treadle reads judge templates from disk at call time,
// so an unseeded hub makes the judge fall back to its fail-safe default.
func seedHubStencils(t *testing.T, hub string) string {
	t.Helper()
	baseDir := fabricengine.StencilsDir(hub)
	if _, err := stencilstore.Reconcile(baseDir, stencils.Registry(), stencilstore.ModeProduction, stencilstore.Source{}); err != nil {
		t.Fatalf("stencilstore.Reconcile(%q) = %v; want nil error", baseDir, err)
	}
	return baseDir
}

// hubHolder is one process holding the fixture hub as its cwd.
type hubHolder struct {
	pid  int
	name string
}

// hubHolders returns every process whose current working directory is inside dir.
// It reads each process's cwd from RTL_USER_PROCESS_PARAMETERS.CurrentDirectory in its PEB.
// Win32_Process exposes no cwd column, so the probe reads the PEB instead.
// It returns nil when nothing holds dir or the probe fails, and callers then keep waiting.
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

// deferHubRelease registers a cleanup that makes the fixture hub directory releasable before RemoveAll.
// The cleanup kills orphaned conhost processes that hold the hub as their cwd.
func deferHubRelease(t *testing.T, hub string) {
	t.Helper()
	t.Cleanup(func() {
		// A process cannot rename its own cwd, so step out of hub while probing.
		// The original cwd is restored at the end of this cleanup.
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

		// Grace phase: the healthy path, in which the ConPTY host exits on its own shortly after its pane dies.
		if !waitReleased(10 * time.Second) {
			// Escalation phase: find the holders and kill the orphaned conhosts.
			// Holders are rescanned each round, since one can appear late while the OS teardown is starved.
			// Any other holder is a real leak, which the kill must not paper over.
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

		// Restore the original cwd only when it is outside hub, which is the normal case.
		// t.Chdir's own restore runs before this cleanup.
		// Never chdir back into a hub that is about to be removed.
		if prev != "" && !strings.HasPrefix(strings.ToLower(prev), strings.ToLower(hub)) {
			_ = os.Chdir(prev)
		}
	})
}

// TestSmokeJudgeCirclingToyFixture runs one real per-round circling-check judge call against a real claude.
// It checks that the judge machinery works against a real engine.
func TestSmokeJudgeCirclingToyFixture(t *testing.T) {
	tiergate.Require(t, 3)
	llmkit.Claude(t, "LYM_REED_CLAUDE")

	// fabriccli.CloneAndWire reconciles the default config of every registered module when the hub is built.
	// Seeding shuttleengine's and reedengine's ConfigTemplate again would be a no-op.
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	deferHubRelease(t, h.PrimeWorktree())
	stencilsDir := seedHubStencils(t, h.Location.HubPath)
	reedCfg, err := reedengine.LoadConfig(configengine.Dirs{Anchor: h.Location.AnchorPath()}, "reed")
	if err != nil {
		t.Fatalf("load reed config: %v", err)
	}
	// Registered before the first boot, so reed's boot finds the kit's hermetic server on the hub's key.
	tmuxkit.KillOnCleanup(t, reedCfg.Tmux, reedengine.ServerName(h.Path))
	t.Chdir(h.PrimeWorktree())
	t.Cleanup(func() {
		var buf bytes.Buffer
		reedcli.RunCLI(&buf, []string{"down"})
	})

	// Run reed up, which boots the substrate.
	// A strand must exist in an up session before shuttle's AddStrand can bind it to a pane.
	// runCircling drives exactly one shuttle run under the hood.
	var reedOut bytes.Buffer
	if code := reedcli.RunCLI(&reedOut, []string{"up"}); code != 0 {
		t.Fatalf("reed up = %d; want 0, output: %s", code, reedOut.String())
	}

	// The two fixture review files carry the same BLOCKING finding, unchanged from round 1 to round 2.
	// It is worded so a real judge reads it as recurring, not as a verdict on the test's convergence.
	round1Path := filepath.Join(h.PrimeWorktree(), "round-1-review.md")
	round2Path := filepath.Join(h.PrimeWorktree(), "round-2-review.md")
	recurring := `---
verdict: BLOCKING
findings:
  - id: b-1
    severity: BLOCKING
    location: chair-table.txt:1
    summary: the chair's color does not match the table's color
---

The chair is red and the table is blue; they must match.
`
	if err := os.WriteFile(round1Path, []byte(recurring), 0o644); err != nil {
		t.Fatalf("write round-1 fixture review: %v", err)
	}
	if err := os.WriteFile(round2Path, []byte(recurring), 0o644); err != nil {
		t.Fatalf("write round-2 fixture review: %v", err)
	}

	verdictPath := filepath.Join(h.PrimeWorktree(), "round-2-judge.md")
	handoffPath := filepath.Join(h.PrimeWorktree(), "round-2-handoff.md")

	// Wire the real stack directly.
	// treadleengine does not import claudeengine, but a caller may, and this test is one.
	shuttleCfg, err := shuttleengine.LoadConfig(configengine.Dirs{Anchor: h.Location.AnchorPath()}, "shuttle")
	if err != nil {
		t.Fatalf("load shuttle config: %v", err)
	}
	reedGeom, err := hubgeom.ReedGeometry(h.Location)
	if err != nil {
		t.Fatalf("reed geometry: %v", err)
	}
	reedEngine := reedengine.New(reedCfg, reedGeom, shuttleengines.EnvScrub())
	runner := shuttleengine.NewRunner(reedEngine, shuttlefake.EngineSet(claudeengine.NewFromConfig(shuttleCfg, fabricengine.ConfigDirs(h.Location))), reedGeom.AnchorPath, reedGeom.WorktreeRoot, shuttleCfg)

	// HandoffPath is required: the judge call writes its maintained handoff next to the verdict.
	// The template's handoff_path marker is mandatory, so an empty HandoffPath fails the fill before any spawn.
	// The judge then falls back to its fail-safe default silently, which is the failure this test exists to catch.
	verdict, rationale, ok := treadleengine.RunCirclingForTest(runner, "gate", treadleengine.JudgeInputsForTest{
		Round:        2,
		PriorReviews: []string{round1Path, round2Path},
		VerdictPath:  verdictPath,
		HandoffPath:  handoffPath,
		Choice:       shuttleengine.ModelChoice{Model: "haiku"},
		StencilsDir:  stencilsDir,
	})

	// runCircling never errors; a spawn or parse failure silently degrades to (JudgeProgressing, "", false).
	// The test therefore asserts that ok is true, which shows a real verdict was parsed, not the fail-safe fallback.
	// It also checks that the verdict file was written and parses.
	// It asserts no specific verdict, since a real LLM call cannot guarantee one.
	if !ok {
		t.Fatalf("runCircling() ok = false; want true — a real judge call should not fall back to the fail-safe default (verdict=%q rationale=%q)", verdict, rationale)
	}
	content, err := os.ReadFile(verdictPath)
	if err != nil {
		t.Fatalf("read judge verdict file: %v (runCircling returned verdict=%q rationale=%q)", err, verdict, rationale)
	}
	if _, _, err := treadleengine.ParseJudgeVerdict(content, treadleengine.FramingCirclingForTest); err != nil {
		t.Fatalf("judge verdict file failed to parse: %v; content:\n%s", err, content)
	}
	if strings.TrimSpace(rationale) == "" {
		t.Error("runCircling() rationale is empty; want the real judge's non-empty rationale")
	}

	// The same call must also write a well-formed handoff file.
	// This is the live half of the handoff contract.
	// A fake-shuttle test can only prove the loop's handling.
	// Only a real agent shows that it writes a parseable handoff from the template's instructions.
	handoffContent, err := os.ReadFile(handoffPath)
	if err != nil {
		t.Fatalf("read judge handoff file: %v (the judge call must write the handoff alongside the verdict)", err)
	}
	if _, err := treadleengine.ParseHandoff(handoffContent); err != nil {
		t.Fatalf("judge handoff file failed to parse: %v; content:\n%s", err, handoffContent)
	}
}
