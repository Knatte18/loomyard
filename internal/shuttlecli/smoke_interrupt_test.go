// smoke_interrupt_test.go proves the run loop's interrupt use case against a real claude in a real tmux pane.
// The *Run that Runner.Start returns stays usable while its Wait call blocks in a goroutine,
// so an operator or another caller can stop the agent's turn and send it a one-line replacement instruction
// without killing the pane or the session.
// The test drives shuttleengine.NewRunner directly rather than shuttlecli.RunCLI.
// It needs the *Run handle while Wait blocks elsewhere;
// the CLI's `run` verb blocks on Runner.Run, which returns no handle to interrupt.
//
// The real provider shapes the test in these ways.
// A counting prompt meant to run without end still bounds its own output and ends its turn,
// so the run can finish before the mid-turn check sees it; the attempt is retried (see maxMidTurnAttempts).
// The provider's Stop hook fires on any turn end, including one that Interrupt ends;
// so Wait holds that Stop and keeps polling the same agent while the output file is unwritten,
// and the redirect turn that Send starts is the one that finishes the run.
//
// The provider TUI shows no streamed response text during a turn; the whole response appears in one frame at turn end.
// Mid-turn detection therefore keys on the pane capture changing between polls, not on its visible content.
// The in-turn spinner repaints at least once per second; see midTurnActivityThreshold for the calibration.

package shuttlecli

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/reedcli"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine/claudeengine"
	"github.com/Knatte18/loomyard/internal/shuttleengines"
	"github.com/Knatte18/loomyard/internal/testkit/llmkit"
	"github.com/Knatte18/loomyard/internal/testkit/shuttlefake"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
	"github.com/Knatte18/loomyard/internal/testkit/tmuxkit"
)

// midTurnActivityThreshold is how many pane-capture changes an input-ready pane must show across polls;
// the check requires that many before it declares the agent mid-turn.
// A changing capture is the only mid-turn signal the pane gives.
// content could only match after the turn ended, so it cannot signal a turn in progress.
// Three changes at the one-second poll cadence mean about three seconds of sustained turn activity.
// That is shorter than a counting turn.
// An idle pane makes no changes, so it never reaches the threshold.
// Only captures the engine classifies StartupReady take part, so launch noise cannot count.
// That state means the provider TUI is on screen.
const midTurnActivityThreshold = 3

// waitOutcome carries the result of a completed run.Wait() call from the goroutine that made it.
// Passing it over a channel lets the caller select on the outcome without blocking.
type waitOutcome struct {
	result shuttleengine.Result
	err    error
}

// maxMidTurnAttempts bounds how many fresh runs the interrupt smoke test starts while it tries to catch one mid-turn.
// The real claude process sometimes ends the counting turn on its own before the poll observes it.
// A retry is cheap, and it keeps one missed window from failing the whole test.
const maxMidTurnAttempts = 4

// startMidTurnCountingRun starts one shuttle run with a prompt that directs an open-ended counting task.
// It polls the pane's captured content until the capture keeps changing while the provider TUI is on screen.
// That change shows the turn is underway.
// Matching the counting content cannot show it, because the provider TUI renders no streamed text mid-turn.
// It returns the run handle, the wait channel, and true once mid-turn is observed.
// It returns false when the run reaches a terminal outcome first.
// It also returns false when the window elapses without a terminal outcome or turn activity.
// In both cases it tears the strand down so the caller can retry with a fresh run.
func startMidTurnCountingRun(t *testing.T, runner *shuttleengine.Runner, engine shuttleengine.Engine, reedEngine *reedengine.Engine, shuttleCfg shuttleengine.Config, outputPath string) (run *shuttleengine.Run, waitCh chan waitOutcome, ok bool) {
	t.Helper()

	prompt := fmt.Sprintf(
		"Count out loud starting from 1, one number per line, going up forever with no "+
			"upper bound and no pause to ask anything — do not write any file and do not stop "+
			"counting until you are told otherwise. (The file %s only matters if you are later "+
			"told to write it.)",
		outputPath,
	)

	run, err := runner.Start(shuttleengine.Spec{
		Prompt:      prompt,
		OutputFiles: []string{outputPath},
		ModelChoice: shuttleengine.ModelChoice{Model: smokeClaudeModel},
		Timeout:     5 * time.Minute,
	})
	if err != nil {
		t.Fatalf("runner.Start: %v", err)
	}

	waitCh = make(chan waitOutcome, 1)
	go func() {
		result, waitErr := run.Wait()
		waitCh <- waitOutcome{result, waitErr}
	}()

	guid := run.StrandGUID()

	// abandonAttempt tears this run's strand down and signals the caller to retry with a fresh one.
	// Both non-success exits funnel through it.
	// An attempt that ends before a usable mid-turn window is a transient miss against a live model, not a test failure,
	// so it is retried rather than hard-failed.
	// The strand is removed so a run that died, timed out, or was still mid-turn does not leak into the next attempt.
	// A RemoveStrand error is logged and does not fail the test.
	abandonAttempt := func(reason string) (*shuttleengine.Run, chan waitOutcome, bool) {
		t.Logf("%s; retrying with a fresh run", reason)
		if _, rerr := reedEngine.RemoveStrand(guid, false); rerr != nil {
			t.Logf("cleanup: remove strand %s after abandoned attempt (non-fatal): %v", guid, rerr)
		}
		return nil, nil, false
	}

	// The activity window is generous relative to startup, so a slow launch alone is not mistaken for a hang.
	// If the window elapses with neither a terminal outcome nor turn activity, the attempt is abandoned and retried.
	deadline := time.Now().Add(time.Duration(shuttleCfg.StartupTimeoutS)*time.Second + 120*time.Second)

	// previousCapture holds the last capture the engine classified StartupReady.
	// observedChanges counts the later ready captures that differed from their predecessor.
	// Boot frames, where the provider TUI is not on screen yet, neither set the baseline nor count.
	// The shell echoing the launch line changes the pane too, but that is not turn activity.
	previousCapture := ""
	haveBaseline := false
	observedChanges := 0
	for {
		select {
		case res := <-waitCh:
			// The run reached a terminal outcome before mid-turn was observed.
			// The model sometimes ends the counting turn on its own; that is not a test failure, only a missed window.
			return abandonAttempt(fmt.Sprintf("attempt reached a terminal outcome (%s, err=%v) before mid-turn was observed", res.result.Outcome, res.err))
		default:
		}
		if time.Now().After(deadline) {
			// The window elapsed with neither a terminal outcome nor turn activity.
			// A live turn repaints its spinner every second, so a silent pane means the provider TUI never came up or froze.
			// The attempt is abandoned and retried.
			// instead of hard-failing on the first miss; a genuine persistent hang exhausts maxMidTurnAttempts,
			// and the caller then fails the test.
			return abandonAttempt(fmt.Sprintf("timed out waiting for the pane to show turn activity (guid %s)", guid))
		}

		capture, err := reedEngine.CapturePane(guid)
		if err != nil {
			t.Logf("CapturePane during mid-turn poll (retrying): %v", err)
			time.Sleep(time.Second)
			continue
		}
		// Only input-ready frames participate, the same StartupReady gate Interrupt/Send enforce.
		// A detection here implies the interrupt that follows will find the pane state it requires.
		if engine.Startup(capture) == shuttleengine.StartupReady {
			if haveBaseline && capture != previousCapture {
				observedChanges++
				if observedChanges >= midTurnActivityThreshold {
					return run, waitCh, true
				}
			}
			previousCapture, haveBaseline = capture, true
		}
		time.Sleep(time.Second)
	}
}

// pollFileContentEquals polls path until its trimmed content equals want or deadline passes.
// It returns the last content read and whether that content matched want.
// The test observes the file rather than the run's wait outcome.
// A redirected agent keeps writing after the wait has already returned,
// so the file itself is the property to observe.
func pollFileContentEquals(path, want string, deadline time.Time) (last string, matched bool) {
	for {
		if data, err := os.ReadFile(path); err == nil {
			last = strings.TrimSpace(string(data))
			if last == want {
				return last, true
			}
		}
		if time.Now().After(deadline) {
			return last, false
		}
		time.Sleep(time.Second)
	}
}

// TestSmokeInterruptSendContinues starts a run whose prompt directs an open-ended counting task.
// It retries with a fresh run if real claude self-ends the turn before a mid-turn window is observed.
// It then calls run.Interrupt() followed by run.Send() with a one-line replacement instruction.
// It asserts that the output file eventually carries the REDIRECTED content, proven by polling the file directly.
// It also asserts that Wait then drains with the done outcome.
// The interrupted turn's own Stop is a hold, so only the redirect turn's completion ends the run.
func TestSmokeInterruptSendContinues(t *testing.T) {
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

	// A strand must exist in an up'd session before shuttle's AddStrand can bind it to a pane.
	var reedOut bytes.Buffer
	if code := reedcli.RunCLI(&reedOut, []string{"up"}); code != 0 {
		t.Fatalf("reed up = %d; want 0, output: %s", code, reedOut.String())
	}

	// Keep the *Run handle Start returns instead of blocking on Runner.Run:
	// the test needs it to Interrupt/Send while Wait blocks in a goroutine.
	runner, engine, reedEngine, shuttleCfg := newSmokeRunner(t)

	outputPath := filepath.Join(h.PrimeWorktree(), "smoke-interrupt-output.txt")

	var run *shuttleengine.Run
	var waitCh chan waitOutcome
	for attempt := 1; attempt <= maxMidTurnAttempts; attempt++ {
		r, ch, ok := startMidTurnCountingRun(t, runner, engine, reedEngine, shuttleCfg, outputPath)
		if ok {
			run, waitCh = r, ch
			break
		}
		t.Logf("mid-turn attempt %d/%d did not catch the run mid-turn", attempt, maxMidTurnAttempts)
	}
	if run == nil {
		t.Fatalf("never observed the counting task genuinely underway after %d attempts", maxMidTurnAttempts)
	}

	if err := run.Interrupt(); err != nil {
		t.Fatalf("run.Interrupt: %v", err)
	}
	if err := run.Send(fmt.Sprintf("write exactly REDIRECTED to %s and stop", outputPath)); err != nil {
		t.Fatalf("run.Send: %v", err)
	}

	// The redirected instruction reaches the still-live pane and the agent eventually rewrites the output file.
	fileDeadline := time.Now().Add(3 * time.Minute)
	if last, matched := pollFileContentEquals(outputPath, "REDIRECTED", fileDeadline); !matched {
		t.Fatalf("output file content = %q after 3m; want %q (the interrupt+send sequence must have redirected the run)", last, "REDIRECTED")
	}

	// Drain Wait so the goroutine and reed/run-dir state settle before teardown.
	// The interrupted turn's Stop is a hold, so only the redirect turn's completion ends the run:
	// any error or outcome but OutcomeDone indicates a genuine mechanism failure.
	select {
	case res := <-waitCh:
		t.Logf("run.Wait outcome=%s err=%v", res.result.Outcome, res.err)
		if res.err != nil {
			t.Fatalf("run.Wait: %v", res.err)
		}
		if res.result.Outcome != shuttleengine.OutcomeDone {
			t.Fatalf("run.Wait outcome = %q; want %q (any other outcome after a successful redirect indicates a real mechanism failure)", res.result.Outcome, shuttleengine.OutcomeDone)
		}
	case <-time.After(5 * time.Minute):
		t.Fatal("run.Wait did not return within 5m after the interrupt+send sequence")
	}
}

// newSmokeRunner builds a shuttleengine.Runner against the hub the test is chdir'd into.
// It wires the runner the way a real CLI run does, so the smoke test exercises the same setup.
// It returns the runner with the engine, reed engine and config it was built from.
// Tests that need the *Run handle Start returns use this runner, because a blocking run never hands that handle back.
func newSmokeRunner(t *testing.T) (*shuttleengine.Runner, shuttleengine.Engine, *reedengine.Engine, shuttleengine.Config) {
	t.Helper()

	cwd, err := lymcwd.Getwd()
	if err != nil {
		t.Fatalf("lymcwd.Getwd: %v", err)
	}
	layout, err := lymcwd.Resolve(cwd)
	if err != nil {
		t.Fatalf("lymcwd.Resolve: %v", err)
	}
	shuttleCfg, err := shuttleengine.LoadConfig(configengine.Dirs{Anchor: layout.AnchorPath()}, "shuttle")
	if err != nil {
		t.Fatalf("shuttleengine.LoadConfig: %v", err)
	}
	reedCfg, err := reedengine.LoadConfig(configengine.Dirs{Anchor: layout.AnchorPath()}, "reed")
	if err != nil {
		t.Fatalf("reedengine.LoadConfig: %v", err)
	}
	reedGeom, err := hubgeom.ReedGeometry(layout)
	if err != nil {
		t.Fatalf("reed geometry: %v", err)
	}
	reedEngine := reedengine.New(reedCfg, reedGeom, shuttleengines.EnvScrub())
	engine := claudeengine.NewFromConfig(shuttleCfg, fabricengine.ConfigDirs(layout))
	runner := shuttleengine.NewRunner(reedEngine, shuttlefake.EngineSet(engine), reedGeom.AnchorPath, reedGeom.WorktreeRoot, shuttleCfg)
	return runner, engine, reedEngine, shuttleCfg
}

// registerReedServer registers the hub's reed socket key with tmuxkit before the test's first reed boot.
// This lets the boot find tmuxkit's hermetic server.
func registerReedServer(t *testing.T, h *hubforge.Hub) {
	t.Helper()
	reedCfg, err := reedengine.LoadConfig(configengine.Dirs{Anchor: h.Location.AnchorPath()}, "reed")
	if err != nil {
		t.Fatalf("reedengine.LoadConfig: %v", err)
	}
	tmuxkit.KillOnCleanup(t, reedCfg.Tmux, reedengine.ServerName(h.Path))
}
