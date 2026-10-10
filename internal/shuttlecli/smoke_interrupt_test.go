//go:build llm

// smoke_interrupt_test.go proves the run loop's core interrupt use case
// against a REAL claude in a REAL tmux pane: Runner.Start's returned *Run
// stays usable while its Wait call blocks in a goroutine, so an operator
// (or another process, via the CLI's interrupt/send verbs) can stop the
// agent's in-progress turn and hand it a one-line replacement instruction
// without killing its pane or session. This test drives
// shuttleengine.NewRunner directly rather than shuttlecli.RunCLI, because it
// needs the *Run handle while Wait blocks elsewhere — the CLI's `run` verb
// blocks on Runner.Run (Start+Wait combined) and never hands back a handle
// to interrupt.
//
// Determinism notes (round opus-r2): live-driving against real claude
// (2.1.200) established two things a prior round's version of this test did
// not account for. First, an "open-ended, never-completing" counting prompt
// is NOT actually open-ended against a real model: it self-bounds its own
// output (observed stopping near ~200 lines with "I'll pause here — I can't
// produce output without bound in a single response") and ends its turn on
// its own, so the run can reach a terminal outcome before the mid-turn poll
// ever catches it — this is retried (see maxMidTurnAttempts) rather than
// treated as a hard failure. Second, and more fundamentally: the provider's
// Stop hook fires on ANY turn end, including one ended by Interrupt itself.
// Wait holds that Stop, since the output file is not yet written, and keeps polling the same agent, so the redirect turn Send starts is the one that finishes the run.
// The test asserts that the redirect actually reaches the still-live pane and the agent rewrites the output file — proven by polling the file directly — and that Wait then returns done.
//
// Determinism notes (round fable-r6): the provider TUI renders NO streamed
// response text while a turn is in progress — the whole response flushes to
// the pane in ONE frame at turn end (proven by capturing this exact counting
// run every 250ms). Mid-turn detection therefore keys on the pane capture
// CHANGING between polls (the in-turn spinner repaints at least once per
// second) rather than on any visible content shape; see
// midTurnActivityThreshold for the live calibration.

package shuttlecli

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/reedcli"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine/claudeengine"
	"github.com/Knatte18/loomyard/internal/testkit/llmkit"
	"github.com/Knatte18/loomyard/internal/testkit/tmuxkit"
)

// midTurnActivityThreshold is how many pane-capture changes (across polls of
// an input-ready pane) startMidTurnCountingRun requires before declaring the
// agent genuinely mid-turn. Calibrated live (round fable-r6) by capturing
// this exact counting run every 250ms: the provider TUI showed only its
// spinner line — whose elapsed-seconds counter repaints at least once per
// second — for the whole ~24-second turn, flushed the entire response in ONE
// frame as the turn ended, and then stayed completely static (68+ seconds
// without a single changed capture). A changing capture is therefore the
// only mid-turn signal the pane gives: a heuristic matching the counting
// CONTENT (the previous "3+ numeric lines" shape) could only ever match
// AFTER the turn was over, which is why it watched a genuinely mid-turn
// agent for 210 straight seconds without firing. Three changes at the
// 1-second poll cadence mean roughly three seconds of sustained turn
// activity — comfortably inside even the shortest observed counting turn
// (~20s), unreachable by an idle pane (zero changes), and insulated from
// launch noise because only captures the engine classifies StartupReady
// (the provider TUI actually on screen — the same gate Interrupt/Send
// enforce) participate at all.
const midTurnActivityThreshold = 3

// waitOutcome carries a completed run.Wait() call's result off the
// background goroutine TestSmokeInterruptSendContinues drives it in, so the
// test can select on it without blocking the main goroutine.
type waitOutcome struct {
	result shuttleengine.Result
	err    error
}

// maxMidTurnAttempts bounds how many fresh runs TestSmokeInterruptSendContinues
// starts while trying to catch one genuinely mid-turn (real claude sometimes
// ends the counting turn on its own before the poll below observes it — see
// the file-level doc comment). Each attempt is cheap relative to the
// determinism this buys: a single flaky "didn't catch it in time" no longer
// fails the whole test.
const maxMidTurnAttempts = 4

// startMidTurnCountingRun starts one shuttle run whose prompt directs an
// open-ended counting task, then polls the pane's captured content for proof
// the turn is genuinely underway: the capture keeps CHANGING while the
// provider TUI is on screen (see midTurnActivityThreshold for why matching
// the counting content itself cannot work — the TUI renders no streamed text
// mid-turn). Returns the run handle, its wait-outcome channel, and true once
// mid-turn is observed. If the run reaches a terminal outcome first (real
// claude sometimes self-ends the counting turn — see the file-level doc
// comment), OR the window elapses with neither a terminal outcome nor
// observed turn activity (a launch that never brought the provider TUI up),
// it tears the strand down and returns ok=false so the caller can retry with
// a fresh run. The caller bounds total attempts via maxMidTurnAttempts.
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
		Model:       smokeClaudeModel,
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

	// abandonAttempt tears this run's strand down and signals the caller to
	// retry with a fresh one. Both non-success exits below funnel through it:
	// an attempt that ends before yielding a usable mid-turn window is a
	// transient miss against a live model, not a test failure, so it must be
	// retried rather than hard-failed. The strand is removed because an
	// died/timeout run (or one still mid-turn when we give up) would
	// otherwise survive per the run-loop cleanup rules and leak into the next
	// attempt; RemoveStrand's error is non-fatal cleanup noise.
	abandonAttempt := func(reason string) (*shuttleengine.Run, chan waitOutcome, bool) {
		t.Logf("%s; retrying with a fresh run", reason)
		if _, rerr := reedEngine.RemoveStrand(guid, false); rerr != nil {
			t.Logf("cleanup: remove strand %s after abandoned attempt (non-fatal): %v", guid, rerr)
		}
		return nil, nil, false
	}

	// The activity window is generous relative to startup: slow startup alone
	// must not be mistaken for a hang. If it still elapses with neither a
	// terminal outcome nor observed turn activity, we abandon and retry
	// rather than hard-fail (the caller bounds total attempts via
	// maxMidTurnAttempts).
	deadline := time.Now().Add(time.Duration(shuttleCfg.StartupTimeoutS)*time.Second + 120*time.Second)

	// Turn-activity detection state: previousCapture is the last capture the
	// engine classified StartupReady, and observedChanges counts how many
	// later ready captures differed from their predecessor. Boot frames (no
	// provider TUI on screen yet) neither baseline nor count — the shell
	// echoing the launch line changes the pane too, but is not turn activity.
	previousCapture := ""
	haveBaseline := false
	observedChanges := 0
	for {
		select {
		case res := <-waitCh:
			// Real claude self-bounded the counting turn (or otherwise ended
			// it) before we ever observed it mid-turn: not a test failure,
			// just an attempt that didn't get a usable window.
			return abandonAttempt(fmt.Sprintf("attempt reached a terminal outcome (%s, err=%v) before mid-turn was observed", res.result.Outcome, res.err))
		default:
		}
		if time.Now().After(deadline) {
			// The window elapsed with neither a terminal outcome nor observed
			// turn activity — the provider TUI never came up (or froze), since
			// a live turn repaints its spinner every second. Abandon and retry
			// instead of hard-failing on the first miss; a genuine persistent
			// hang exhausts maxMidTurnAttempts and the caller then fails the
			// test.
			return abandonAttempt(fmt.Sprintf("timed out waiting for the pane to show turn activity (guid %s)", guid))
		}

		capture, err := reedEngine.CapturePane(guid)
		if err != nil {
			t.Logf("CapturePane during mid-turn poll (retrying): %v", err)
			time.Sleep(time.Second)
			continue
		}
		// Only input-ready frames participate — the same StartupReady gate
		// Interrupt/Send enforce, so a detection here implies the interrupt
		// that follows will find the pane state it requires.
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

// pollFileContentEquals polls path until its trimmed content equals want or
// deadline passes, returning the last content read and whether it matched.
// This is the test's deterministic substitute for trusting Wait's outcome
// (see the file-level doc comment): the redirected agent keeps running and
// writing independently of whatever Wait already returned, so the file
// itself — not the run loop's classification — is the property to observe.
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

// TestSmokeInterruptSendContinues starts a run whose prompt directs an open-ended counting task,
// retries with a fresh run if real claude self-ends the turn before a mid-turn window is observed
// (see startMidTurnCountingRun), then calls run.Interrupt() followed by run.Send() with a one-line
// replacement instruction.
// It asserts that the output file eventually carries the REDIRECTED content, proven by polling the file directly.
// It asserts too that Wait then drains with the done outcome:
// the interrupted turn's own Stop is a hold, so only the redirect turn's completion ends the run.
func TestSmokeInterruptSendContinues(t *testing.T) {
	llmkit.Claude(t, "LYX_REED_CLAUDE")

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	deferHubRelease(t, h.Path)
	registerReedServer(t, h)
	t.Chdir(h.PrimeWorktree())
	t.Cleanup(func() {
		var buf bytes.Buffer
		reedcli.RunCLI(&buf, []string{"down"})
	})

	// up: boots the substrate. A strand must exist in an up'd session
	// before shuttle's AddStrand can bind it to a pane.
	var reedOut bytes.Buffer
	if code := reedcli.RunCLI(&reedOut, []string{"up"}); code != 0 {
		t.Fatalf("reed up = %d; want 0, output: %s", code, reedOut.String())
	}

	// Keep the *Run handle Start returns instead of blocking on Runner.Run —
	// the test needs it to Interrupt/Send while Wait blocks in a goroutine below.
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

	// The redirected instruction reaches the still-live pane and the agent
	// eventually rewrites the output file.
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

// newSmokeRunner builds a shuttleengine.Runner against the hub the test is chdir'd into, the same way shuttlecli.Command()'s PersistentPreRunE does, and returns it with the engine, reed engine and config it was built from.
// Tests that need the *Run handle Start returns use it instead of RunCLI.
func newSmokeRunner(t *testing.T) (*shuttleengine.Runner, shuttleengine.Engine, *reedengine.Engine, shuttleengine.Config) {
	t.Helper()

	cwd, err := lyxcwd.Getwd()
	if err != nil {
		t.Fatalf("lyxcwd.Getwd: %v", err)
	}
	layout, err := lyxcwd.Resolve(cwd)
	if err != nil {
		t.Fatalf("lyxcwd.Resolve: %v", err)
	}
	shuttleCfg, err := shuttleengine.LoadConfig(layout.AnchorPath(), "shuttle")
	if err != nil {
		t.Fatalf("shuttleengine.LoadConfig: %v", err)
	}
	reedCfg, err := reedengine.LoadConfig(layout.AnchorPath(), "reed")
	if err != nil {
		t.Fatalf("reedengine.LoadConfig: %v", err)
	}
	reedGeom, err := hubgeom.ReedGeometry(layout)
	if err != nil {
		t.Fatalf("reed geometry: %v", err)
	}
	reedEngine := reedengine.New(reedCfg, reedGeom)
	engine := claudeengine.New()
	runner := shuttleengine.NewRunner(reedEngine, engine, reedGeom.AnchorPath, reedGeom.WorktreeRoot, shuttleCfg)
	return runner, engine, reedEngine, shuttleCfg
}

// registerReedServer registers the hub's reed socket key with tmuxkit before the test's first reed boot, so the boot finds the kit's hermetic server.
func registerReedServer(t *testing.T, h *hubforge.Hub) {
	t.Helper()
	reedCfg, err := reedengine.LoadConfig(h.Location.AnchorPath(), "reed")
	if err != nil {
		t.Fatalf("reedengine.LoadConfig: %v", err)
	}
	tmuxkit.KillOnCleanup(t, reedCfg.Tmux, reedengine.ServerName(h.Path))
}
