//go:build smoke

// smoke_judge_test.go is treadleengine's opt-in live-integration smoke
// test: TestSmokeJudgeCirclingToyFixture drives one real per-round
// circling-check progress judge call — runCircling — against a REAL claude
// in a REAL tmux pane, over two tiny fixture review files the test writes
// itself. This is the caller wiring the real substrate (reedengine +
// claudeengine + shuttleengine.Runner) directly, mirroring the Shuttle
// Provider-Seam Invariant burlerengine's own smoke_round_test.go exercises:
// treadleengine itself never imports claudeengine, but the test that
// exercises it as a caller may. The assertion is deliberately narrow (the
// verdict file parses) — this proves the judge spawn machinery, the file
// contract, and the verdict parse against a real engine, never judge
// quality.
//
// This file is a package treadleengine_test file, not an in-package test,
// because internal/treadleengine sits inside internal/fabriccli's dependency
// set: an in-package test importing internal/hubforge (which imports
// fabriccli) would close a compile cycle. runCircling and judgeInputs are
// unexported — the same package-local Shuttle-seam surface a future
// round-runner adapter drives — so this file drives
// them through treadleengine/export_test.go's RunCirclingForTest and
// JudgeInputsForTest shims instead. Follows the
// internal/burlerengine/smoke_round_test.go conventions otherwise: opt-in
// via -tags smoke, skipped when no claude binary resolves, poll-with-deadline
// waits only (via shuttleengine.Runner.Run itself), and the orphaned-conhost
// teardown guard against the fixture hub. The helpers here are reproduced
// (not imported) from burlerengine's smoke file, per the
// smoke-files-are-self-contained convention.

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
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/reedcli"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine/claudeengine"
	"github.com/Knatte18/loomyard/internal/stencilstore"
	"github.com/Knatte18/loomyard/internal/treadleengine"
)

// smokePwshPath is the PowerShell 7 binary for the orphaned-conhost teardown probe.
const smokePwshPath = `C:\Code\tools\powershell7\pwsh.exe`

// seedHubStencils populates hub's real fabricengine.StencilsDir(hub) with every shipped stencil,
// through the same stencilstore.Reconcile pass cmd/lyx's root pre-run runs, and returns that
// directory for the caller to pass as judgeInputs.StencilsDir. hubforge.NewHub builds a fabric, not
// a seeded board, and treadle reads its judge templates from disk at call time via
// stencilstore.Read — so an unseeded fixture hub degrades the judge to its fail-safe default, which
// is exactly what this test exists to catch.
func seedHubStencils(t *testing.T, hub string) string {
	t.Helper()
	baseDir := fabricengine.StencilsDir(hub)
	if _, err := stencilstore.Reconcile(baseDir, stencils.Registry(), stencilstore.ModeProduction, ""); err != nil {
		t.Fatalf("stencilstore.Reconcile(%q) = %v; want nil error", baseDir, err)
	}
	return baseDir
}

// claudeBinaryPath returns the claude CLI's path, skipping the test if absent.
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

// hubHolder is one process holding the fixture hub as its cwd.
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
// releasable before RemoveAll. Kills any orphaned conhost holding the hub cwd.
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

// TestSmokeJudgeCirclingToyFixture drives one real per-round circling-check progress judge call
// against a real claude, proving the machinery works.
func TestSmokeJudgeCirclingToyFixture(t *testing.T) {
	claudeBinaryPath(t)

	// shuttleengine.ConfigTemplate() and reedengine.ConfigTemplate() are each module's own plain
	// registered config: fabriccli.CloneAndWire already reconciled default config for every
	// registered module when NewHub built h, so seeding them again here would be a no-op duplicate
	// (outcome 1 of the SeedConfig triage).
	h := hubforge.NewHub(t, ".")
	deferHubRelease(t, h.PrimeWorktree())
	stencilsDir := seedHubStencils(t, h.Location.HubPath)
	t.Chdir(h.PrimeWorktree())
	t.Cleanup(func() {
		var buf bytes.Buffer
		reedcli.RunCLI(&buf, []string{"down"})
	})

	// up: boots the substrate. A strand must exist in an up'd session before
	// shuttle's AddStrand can bind it to a pane — runCircling drives exactly
	// one shuttle run under the hood.
	var reedOut bytes.Buffer
	if code := reedcli.RunCLI(&reedOut, []string{"up"}); code != 0 {
		t.Fatalf("reed up = %d; want 0, output: %s", code, reedOut.String())
	}

	// Write two tiny fixture review files: the same BLOCKING finding
	// (unambiguously worded so a real judge reads it as recurring, not the
	// test's own convergence quality) recurs unchanged from round 1 to
	// round 2.
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

	// Wire the real stack directly: treadleengine never imports claudeengine
	// itself, but this test is the caller and may.
	reedCfg, err := reedengine.LoadConfig(h.Location.AnchorPath(), "reed")
	if err != nil {
		t.Fatalf("load reed config: %v", err)
	}
	shuttleCfg, err := shuttleengine.LoadConfig(h.Location.AnchorPath(), "shuttle")
	if err != nil {
		t.Fatalf("load shuttle config: %v", err)
	}
	reedGeom := hubgeom.ReedGeometry(h.Location)
	reedEngine := reedengine.New(reedCfg, reedGeom)
	runner := shuttleengine.NewRunner(reedEngine, claudeengine.New(), reedGeom.AnchorPath, reedGeom.WorktreeRoot, shuttleCfg)

	// HandoffPath is REQUIRED input: the same judge call must write its
	// maintained handoff alongside the verdict (the handoff-on-disk shared
	// decision), and the template's handoff_path marker is a mandatory
	// top-level stencil marker — leaving it empty fails the fill before any
	// spawn, which is exactly the silent fail-safe degrade this test exists
	// to catch.
	verdict, rationale, ok := treadleengine.RunCirclingForTest(runner, "gate", treadleengine.JudgeInputsForTest{
		Round:        2,
		PriorReviews: []string{round1Path, round2Path},
		VerdictPath:  verdictPath,
		HandoffPath:  handoffPath,
		Model:        "haiku",
		StencilsDir:  stencilsDir,
	})

	// runCircling never errors — a spawn/parse failure would silently
	// degrade to (JudgeProgressing, "", false). Assert ok is true (a real
	// verdict was parsed, not the fail-safe fallback) and that the verdict
	// file was actually written and parses, so this test catches a silent
	// fail-safe degrade (which would otherwise look identical to a judge
	// that genuinely read the case as progressing) rather than asserting a
	// specific verdict, which a real LLM call cannot guarantee.
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

	// The same call must also have produced a well-formed handoff file —
	// this is the live half of the handoff file contract (a fake-shuttle
	// test can only prove the loop's handling, never that a real agent
	// actually writes a parseable handoff from the template's instructions).
	handoffContent, err := os.ReadFile(handoffPath)
	if err != nil {
		t.Fatalf("read judge handoff file: %v (the judge call must write the handoff alongside the verdict)", err)
	}
	if _, err := treadleengine.ParseHandoff(handoffContent); err != nil {
		t.Fatalf("judge handoff file failed to parse: %v; content:\n%s", err, handoffContent)
	}
}
