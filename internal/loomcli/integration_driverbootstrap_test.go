//go:build integration

// integration_driverbootstrap_test.go covers the one property no Tier 1 test in this task can reach:
// that a real end-to-end llm driver launch -- the production driverStarter seam, backed by a real
// *shuttleengine.Runner starting a real (stubbed) provider process in a real reed pane over a fixture
// hub built through hubforge -- returns before the driver session itself has finished, rather than
// waiting on it. Every Tier 1 test of this same branch drives it through a fake driverStarter that
// returns immediately BY CONSTRUCTION, so none of them can catch a regression that made the real seam
// start waiting on the driver -- the failure mode that would deadlock every parent run that spawns an
// llm-driven child.
//
// Like the batch's own scope note, this exercises a stubbed provider binary, never a real `claude`:
// exercising the real thing spawns a live, billed Claude session bounded only by the task at hand.
//
// This package's own testmain_test.go is untagged, so it already arms the hermetic git test
// environment (gitkit.HermeticGitEnv()) for the whole test binary, this file's own integration tag
// included -- exactly as wiring_commitstatus_integration_test.go already relies on, so this file adds
// no TestMain of its own.
//
// This drives the llm arm's own launch machinery (startLLMDriverArm) directly over a *loomCLI wired
// through wire() -- the same production wiring "loom start" itself uses -- rather than through the
// full cobra RunE: "loom start" also spawns the status strand's own watcher pane via os.Executable(),
// which under go test resolves to this very test binary and would recursively re-run the whole suite
// inside that pane, exactly the hazard the Live-Substrate Spawn Observability invariant's "never
// re-exec os.Executable() under go test" clause and this package's own smoke suite (see its header)
// both exist to avoid. wire() itself spawns no process and resolves no cwd, so it carries none of that
// hazard, and startLLMDriverArm never reaches the terminal-handover code at all -- suppressed by
// construction, not by a flag.
package loomcli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine/claudeengine"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// integrationWriteStubDriverScript writes a POSIX shell script standing in for the claude binary this
// file's one spawn launches. The claude engine's own launch line expands the whole composed prompt
// into ONE ARGUMENT via shell.ReadFile's "$(cat ...)" idiom rather than piping it over stdin (see
// internal/shell/posix.go's own ReadFile), so the script reads the prompt from its first positional
// argument, extracts the drive report path driverPrompt quoted into that text, prints
// claudeengine's own ready-marker fixture -- shuttle's own startup step now blocks Start until this
// (or the window closes), so a script that skipped it would make every call here time out at
// startup_timeout_s instead of returning fast -- then sleeps settleDelay, giving the caller a window
// to observe the run in flight, past readiness, before the report exists -- writes a one-line report
// there, and exits.
func integrationWriteStubDriverScript(t *testing.T, settleDelay time.Duration) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "stub-claude.sh")
	script := fmt.Sprintf(`#!/bin/sh
report=$(printf '%%s' "$1" | grep -o '"[^"]*drive-report[^"]*"' | head -1 | tr -d '"')
echo '%s'
sleep %s
if [ -n "$report" ]; then
  mkdir -p "$(dirname "$report")"
  printf 'stub driver report\n' > "$report"
fi
exit 0
`, claudeengine.ReadyFooterFixture, settleDelay)
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write stub driver script: %v", err)
	}
	return path
}

// integrationDriverShuttleConfig returns shuttle's shipped config template with the claude key
// pointed at stubPath -- this file's whole point is proving a real production spawn returns without
// waiting, so the provider path must resolve to a real (if stubbed) executable.
func integrationDriverShuttleConfig(stubPath string) string {
	cfg := shuttleengine.ConfigTemplate()
	return strings.Replace(cfg, "claude: ${env:LYX_SHUTTLE_CLAUDE:-}", "claude: "+stubPath, 1)
}

// waitForDriveReport polls dir for a single "drive-report-*.md" file to appear, returning its path,
// or fails the test after timeout.
func waitForDriveReport(t *testing.T, dir string, timeout time.Duration) string {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		matches, err := filepath.Glob(filepath.Join(dir, "drive-report-*.md"))
		if err == nil && len(matches) == 1 {
			return matches[0]
		}
		if time.Now().After(deadline) {
			t.Fatalf("no single drive-report-*.md file appeared under %s within %s (matches=%v err=%v)", dir, timeout, matches, err)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// TestIntegrationDriverBootstrap_ReturnsWithoutWaitingOnTheDriver drives one end-to-end llm driver
// launch over a fixture hub built through hubforge, seeded with the llm driver, and asserts three
// things: the launch call returns once the stub's ready marker lands and well before its settle
// delay elapses, proving shuttle's own blocking Start returns past the provider's startup gates
// rather than waiting on the whole driver session to finish; the started run's persisted state file
// exists under the run directory the handle reports; and the drive report the stubbed driver writes
// and exits lands under the run's ephemeral scratch directory, never its durable one.
func TestIntegrationDriverBootstrap_ReturnsWithoutWaitingOnTheDriver(t *testing.T) {
	const stubSettleDelay = 2 * time.Second
	stubPath := integrationWriteStubDriverScript(t, stubSettleDelay)

	h := hubforge.NewHub(t, ".")
	hubforge.SeedConfig(t, h, map[string]string{
		"reed":    reedengine.ConfigTemplate(),
		"shuttle": integrationDriverShuttleConfig(stubPath),
		"webster": websterengine.ConfigTemplate(),
	})
	const slug = "loom-integration-driver-task"
	hubforge.AddPair(t, h, slug)
	worktree := h.PairWarpWorktree(slug)

	loc, err := lyxcwd.Resolve(worktree)
	if err != nil {
		t.Fatalf("lyxcwd.Resolve(%s): %v", worktree, err)
	}

	// Seed the fixture worktree's own run with the llm driver directly, per shedrun's own
	// sole-writer contract -- this test drives the launch machinery below the CLI's own bootstrap
	// verb, so it needs no committed origin record and no "parent" param to agree with.
	if err := shedrun.WriteSeed(loc, shedrun.SelfRunID, shedrun.Seed{Recipe: shedrun.RecipeLoom, Driver: shedrun.DriverLLM}); err != nil {
		t.Fatalf("seed the llm driver: %v", err)
	}

	c := &loomCLI{runID: shedrun.SelfRunID}
	if err := c.wire(loc, worktree); err != nil {
		t.Fatalf("wire: %v", err)
	}
	t.Cleanup(func() { _, _ = c.reed.Down() })
	if _, err := c.reed.Up(); err != nil {
		t.Fatalf("reed.Up: %v", err)
	}

	start := time.Now()
	run, err := c.startLLMDriverArm(driverStrandNone, "", c.runID)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("startLLMDriverArm: %v", err)
	}

	if elapsed >= stubSettleDelay {
		t.Errorf("startLLMDriverArm took %s; want well under the stub's own %s settle delay -- Start must return once the provider is ready, not wait on the whole driver session", elapsed, stubSettleDelay)
	}

	runStatePath := filepath.Join(run.RunDir(), "run.json")
	if _, err := os.Stat(runStatePath); err != nil {
		t.Errorf("run state file %s: %v; want it persisted under the run directory the handle reports", runStatePath, err)
	}

	scratchDir := shedrun.ScratchDir(loc, c.runID)
	reportPath := waitForDriveReport(t, scratchDir, 10*time.Second)
	if !strings.HasPrefix(reportPath, scratchDir+string(filepath.Separator)) {
		t.Errorf("drive report path = %q; want it under the run's ephemeral scratch directory %q, not its durable one", reportPath, scratchDir)
	}
}
