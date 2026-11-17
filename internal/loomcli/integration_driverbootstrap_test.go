// integration_driverbootstrap_test.go covers a property no unit test in this package can reach:
// a real end-to-end llm driver launch returns before the driver session has finished.
// The launch runs through the production driverStarter seam, backed by a real *shuttleengine.Runner;
// it starts a stubbed provider process in a real reed pane over a fixture hub built through hubforge.
// The unit tests drive the seam through a fake driverStarter that returns immediately by construction.
// None of them can catch a regression that makes the real seam wait on the driver;
// that failure would deadlock every parent run that spawns an llm-driven child.
//
// This file runs a stubbed provider binary, never a real `claude`;
// a real run would spawn a live, billed Claude session.
//
// The package's testmain_test.go is untagged, so it arms gitkit.HermeticGitEnv() for the whole test binary.
// That covers this file's integration tag too, so this file adds no TestMain of its own.
//
// This drives startLLMDriverArm directly over a *loomCLI wired through wire().
// wire() is the production wiring `loom start` uses.
// It skips the full cobra RunE, which on a go-driven run spawns the status watcher pane via os.Executable();
// under go test that resolves to the test binary and would re-run the whole suite inside the pane.
// wire() spawns no process and resolves no cwd, so it carries none of that hazard.
// startLLMDriverArm never reaches the verb's success-envelope tail; that is by construction, not by a flag.
package loomcli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine/claudeengine"
	"github.com/Knatte18/loomyard/internal/testkit/stencilkit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
	"github.com/Knatte18/loomyard/internal/testkit/tmuxkit"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// integrationWriteStubDriverScript writes a POSIX shell script standing in for the claude binary.
// The driver spec names no skill, so the claude engine's launch line passes the prompt pointer as an argument.
// The script takes the prompt.md path from that argument;
// it extracts the drive report path that driverPrompt quoted into the file.
// It records a session start through stubRecordSessionStart.
// It prints claudeengine's idle-input-box fixture at start and again after the one line it reads.
// Shuttle's startup step blocks Start until it sees a ready marker, and the color step waits for a ready pane;
// a script without the fixture would make every call time out instead of returning fast.
// The one typed line is the `/color` command shuttle types for the driver's colored segment.
// It then holds its driver session open until releasePath exists,
// so the session cannot finish before the caller releases it.
// After that it writes a one-line report and exits.
func integrationWriteStubDriverScript(t *testing.T, releasePath string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "stub-claude.sh")
	script := fmt.Sprintf(`#!/bin/sh
%s
prompt_file=
while [ $# -gt 0 ]; do
  case $1 in *prompt.md*) prompt_file=$(printf '%%s' "$1" | grep -o '[^ "]*prompt\.md' | head -1) ;; esac
  shift
done
box() { printf '%%s\n' '%s'; }
box
IFS= read -r line
box
report=$(grep -o '"[^"]*drive-report[^"]*"' "$prompt_file" | head -1 | tr -d '"')
while [ ! -e '%s' ]; do sleep 0.1; done
if [ -n "$report" ]; then
  mkdir -p "$(dirname "$report")"
  printf 'stub driver report\n' > "$report"
fi
exit 0
`, stubRecordSessionStart, claudeengine.IdleInputBoxFixture, releasePath)
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write stub driver script: %v", err)
	}
	return path
}

// integrationDriverShuttleConfig returns shuttle's shipped config template with startup probed on every poll.
// The shipped template probes every tenth poll; this sets liveness_every_n_polls: 1.
// Shuttle's startup step probes once immediately, before the stub prints its ready marker;
// it then sleeps a whole probe interval,
// so at the shipped cadence Start waits ten polls however fast the provider is ready.
func integrationDriverShuttleConfig() string {
	cfg := shuttleengine.ConfigTemplate()
	return strings.Replace(cfg, "liveness_every_n_polls: 10 ", "liveness_every_n_polls: 1 ", 1)
}

// waitForDriveReport polls dir for a single "drive-report-*.md" file and returns its path.
// It fails the test once timeout elapses.
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

// TestIntegrationDriverBootstrap_ReturnsWithoutWaitingOnTheDriver launches one llm driver end to end.
// The fixture hub is built through hubforge and seeded with the llm driver.
// It asserts three things.
// The launch call returns while the stub holds its driver session open.
// That shows shuttle's own blocking Start returns past the provider's startup gates
// without waiting on the whole driver session to finish.
// The started run's persisted state file exists under the run directory the handle reports.
// The drive report the stubbed driver writes once released lands under the run's durable drive-reports directory.
// It never lands in the ephemeral scratch directory.
func TestIntegrationDriverBootstrap_ReturnsWithoutWaitingOnTheDriver(t *testing.T) {
	tiergate.Require(t, 2)
	releasePath := filepath.Join(t.TempDir(), "release-driver")
	stubPath := integrationWriteStubDriverScript(t, releasePath)

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	hubforge.SeedConfig(t, h, map[string]string{
		"claude":  claudeConfigWithBinary(stubPath),
		"reed":    reedengine.ConfigTemplate(),
		"shuttle": integrationDriverShuttleConfig(),
		"webster": websterengine.ConfigTemplate(),
	})
	const slug = "loom-integration-driver-task"
	hubforge.AddPair(t, h, slug)
	worktree := h.PairCodeWorktree(slug)
	tmuxkit.KillOnCleanup(t, tmuxBinaryPath(t), reedengine.ServerName(h.Path))

	loc, err := lymcwd.Resolve(worktree)
	if err != nil {
		t.Fatalf("lymcwd.Resolve(%s): %v", worktree, err)
	}

	// Seed the fixture worktree's own run with the llm driver directly, as shedrun's sole-writer contract requires.
	// This test drives the launch machinery below the CLI's bootstrap verb,
	// so it needs no committed origin record and no "parent" param to agree with.
	if err := shedrun.WriteSeed(loc, shedrun.SelfRunID, shedrun.Seed{Recipe: shedrun.RecipeTapestry, Driver: shedrun.DriverLLM}); err != nil {
		t.Fatalf("seed the llm driver: %v", err)
	}

	c := &loomCLI{runID: shedrun.SelfRunID}
	if err := c.wire(loc, worktree); err != nil {
		t.Fatalf("wire: %v", err)
	}
	// The driver prompt is rendered from the shipped stencils, which a fixture hub does not seed.
	stencilkit.SeedInto(t, c.runDeps.Geom.StencilsDir)
	t.Cleanup(func() { _, _ = c.reed.Down() })
	if _, err := c.reed.Up(); err != nil {
		t.Fatalf("reed.Up: %v", err)
	}

	type launchResult struct {
		run driverHandle
		err error
	}
	launched := make(chan launchResult, 1)
	go func() {
		run, err := c.startLLMDriverArm(driverStrandNone, "", c.runID)
		launched <- launchResult{run: run, err: err}
	}()
	// The stub's session cannot finish before the release file exists,
	// so a launch that waits on the whole session never returns before this deadline.
	const launchDeadline = 60 * time.Second
	var launch launchResult
	select {
	case launch = <-launched:
	case <-time.After(launchDeadline):
		t.Fatalf("startLLMDriverArm did not return within %s while the stub held its driver session open -- Start must return once the provider is ready, not wait on the whole driver session", launchDeadline)
	}
	if launch.err != nil {
		t.Fatalf("startLLMDriverArm: %v", launch.err)
	}
	if err := os.WriteFile(releasePath, nil, 0o644); err != nil {
		t.Fatalf("release the stub driver: %v", err)
	}

	runStatePath := filepath.Join(launch.run.RunDir(), "run.json")
	if _, err := os.Stat(runStatePath); err != nil {
		t.Errorf("run state file %s: %v; want it persisted under the run directory the handle reports", runStatePath, err)
	}

	reportsDir := shedrun.DriveReportsDir(loc, c.runID)
	reportPath := waitForDriveReport(t, reportsDir, 10*time.Second)
	if !strings.HasPrefix(reportPath, reportsDir+string(filepath.Separator)) {
		t.Errorf("drive report path = %q; want it under the run's durable drive-reports directory %q, not its ephemeral scratch one", reportPath, reportsDir)
	}
}
