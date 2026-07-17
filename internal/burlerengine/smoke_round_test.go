//go:build smoke

// smoke_round_test.go is burlerengine's opt-in live-integration smoke test:
// TestSmokeBurlerRoundToyFixture drives one full burler round — A-review then
// B-fix — against a REAL claude in a REAL tmux pane, over a toy chair/table
// color-mismatch fixture. This is the caller wiring the real substrate
// (muxengine + claudeengine + shuttleengine.Runner) directly, in an external
// test package, per the Shuttle Provider-Seam Invariant: burlerengine itself
// never imports claudeengine, but the test that exercises it as a caller may.
// The assertions are deliberately trivial (the toy is unambiguous on
// purpose) — this proves the A->B machinery, the file contract, and the
// verdict parse against a real engine, never review quality. Follows the
// internal/shuttlecli/smoke_*.go conventions: opt-in via -tags smoke,
// skipped when no claude binary resolves, poll-with-deadline waits only, and
// the orphaned-conhost teardown guard against the fixture hub. The helpers
// here are reproduced (not imported) from shuttlecli's smoke files, per the
// smoke-files-are-self-contained convention.

package burlerengine_test

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

	"github.com/Knatte18/loomyard/internal/burlerengine"
	"github.com/Knatte18/loomyard/internal/lyxtest"
	"github.com/Knatte18/loomyard/internal/muxcli"
	"github.com/Knatte18/loomyard/internal/muxengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine/claudeengine"
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

// knownColors is the fixed vocabulary the toy fixture's chair/table colors
// are drawn from — enough to detect whether the fixed target still mentions
// two distinct colors, without parsing the fixer's exact wording.
var knownColors = []string{
	"red", "blue", "green", "yellow", "orange", "purple",
	"black", "white", "brown", "pink", "gray", "grey",
}

// distinctColorsMentioned returns the set of knownColors that appear
// (case-insensitively) anywhere in text, in knownColors order.
func distinctColorsMentioned(text string) []string {
	lower := strings.ToLower(text)
	var found []string
	for _, c := range knownColors {
		if strings.Contains(lower, c) {
			found = append(found, c)
		}
	}
	return found
}

// TestSmokeBurlerRoundToyFixture drives one full burler round against a REAL
// claude in a REAL tmux pane, over a toy fixture whose chair and table
// colors deliberately mismatch: the target is unambiguous on purpose so this
// test proves the A->B machinery + file contract + verdict parse against a
// real engine, never review quality. It constructs the real stack directly
// (muxengine + claudeengine + shuttleengine.Runner + burlerengine.Engine) —
// this test IS the caller the Shuttle Provider-Seam Invariant reserves that
// wiring for.
func TestSmokeBurlerRoundToyFixture(t *testing.T) {
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

	// up: boots the substrate. A strand must exist in an up'd session before
	// shuttle's AddStrand can bind it to a pane — burlerengine.Run drives
	// exactly one shuttle run under the hood.
	var muxOut bytes.Buffer
	if code := muxcli.RunCLI(&muxOut, []string{"up"}); code != 0 {
		t.Fatalf("mux up = %d; want 0, output: %s", code, muxOut.String())
	}

	// Write the toy target: an unambiguous chair/table color mismatch, the
	// only thing a BLOCKING finding can legitimately be about here.
	targetPath := filepath.Join(fixture.Hub, "chair-table.txt")
	original := "In this small room there is a chair and a table. The chair is painted a " +
		"bright red color. The table is painted a deep blue color. Nothing else in " +
		"the room is described.\n"
	if err := os.WriteFile(targetPath, []byte(original), 0o644); err != nil {
		t.Fatalf("write toy target file: %v", err)
	}

	reviewPath := filepath.Join(fixture.Hub, "burler-smoke-review.md")
	fixerReportPath := filepath.Join(fixture.Hub, "burler-smoke-fixer-report.md")

	profile := burlerengine.Profile{
		Target: burlerengine.FileSet{
			Paths: []string{targetPath},
		},
		Fasit: burlerengine.FileSet{
			// The rule IS the source of truth here — no separate fasit file
			// is needed for a toy this small.
			Instructions: "the chair's color must match the table's color",
		},
		Rubric: "BLOCKING: the chair's color and the table's color, as described in the " +
			"target text, do not match.\nAPPROVED: the chair's color and the table's " +
			"color match; note anything else as a non-blocking MEDIUM/LOW/NIT finding.",
		FixScope:        burlerengine.FixScopeOverlay,
		ToolUse:         false,
		ReviewPath:      reviewPath,
		FixerReportPath: fixerReportPath,
	}

	// Wire the real stack directly: burlerengine never imports claudeengine
	// itself, but this test is the caller and may.
	muxCfg, err := muxengine.LoadConfig(fixture.Layout.Cwd, "mux")
	if err != nil {
		t.Fatalf("load mux config: %v", err)
	}
	shuttleCfg, err := shuttleengine.LoadConfig(fixture.Layout.Cwd, "shuttle")
	if err != nil {
		t.Fatalf("load shuttle config: %v", err)
	}
	muxEngine := muxengine.New(muxCfg, fixture.Layout)
	runner := shuttleengine.NewRunner(muxEngine, claudeengine.New(), fixture.Layout, shuttleCfg)
	engine := burlerengine.New(runner, fixture.Layout, burlerengine.Config{})

	result, err := engine.Run(profile, burlerengine.RunOpts{Timeout: 5 * time.Minute})
	if err != nil {
		t.Fatalf("burler round: %v", err)
	}

	if result.Outcome != shuttleengine.OutcomeDone {
		t.Fatalf("round outcome = %q; want %q; lastAssistantMessage: %q", result.Outcome, shuttleengine.OutcomeDone, result.LastAssistantMessage)
	}
	if result.Verdict != burlerengine.VerdictBlocking {
		t.Fatalf("round verdict = %q; want %q (the toy fixture's color mismatch is unambiguous)", result.Verdict, burlerengine.VerdictBlocking)
	}
	if len(result.Findings) == 0 {
		t.Errorf("round verdict is BLOCKING but Findings is empty; want at least one recorded finding")
	}

	fixed, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("read fixed target file: %v", err)
	}
	if string(fixed) == original {
		t.Errorf("target file content is unchanged after a BLOCKING round; want the chair/table color mismatch fixed")
	}
	if colors := distinctColorsMentioned(string(fixed)); len(colors) >= 2 {
		t.Errorf("fixed target file still mentions distinct colors %v; want the chair's and table's colors to match", colors)
	}

	fixerReport, err := os.ReadFile(fixerReportPath)
	if err != nil {
		t.Fatalf("read fixer report: %v", err)
	}
	if strings.TrimSpace(string(fixerReport)) == "" {
		t.Errorf("fixer report is empty; want a non-empty account of what was fixed")
	}
}
