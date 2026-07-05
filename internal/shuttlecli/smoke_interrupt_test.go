//go:build smoke

// smoke_interrupt_test.go proves the run loop's core interrupt use case
// against a REAL claude in a REAL psmux pane: Runner.Start's returned *Run
// stays usable while its Wait call blocks in a goroutine, so an operator
// (or another process, via the CLI's interrupt/send verbs) can stop the
// agent's in-progress turn and hand it a one-line replacement instruction
// without killing its pane or session. This test drives
// shuttleengine.NewRunner directly rather than shuttlecli.RunCLI, because it
// needs the *Run handle while Wait blocks elsewhere — the CLI's `run` verb
// blocks on Runner.Run (Start+Wait combined) and never hands back a handle
// to interrupt.

package shuttlecli

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/hubgeometry"
	"github.com/Knatte18/loomyard/internal/lyxtest"
	"github.com/Knatte18/loomyard/internal/muxcli"
	"github.com/Knatte18/loomyard/internal/muxengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine/claudeengine"
)

// waitOutcome carries a completed run.Wait() call's result off the
// background goroutine TestSmokeInterruptSendContinues drives it in, so the
// test can select on it after sending the interrupt-and-redirect sequence.
type waitOutcome struct {
	result shuttleengine.Result
	err    error
}

// TestSmokeInterruptSendContinues starts a run whose prompt directs a slow
// multi-step task before writing its output file, then — after a startup
// grace window, so the pane is past claude's one-time trust dialog and
// ready for input — calls run.Interrupt() followed by run.Send() with a
// one-line replacement instruction. Asserts Wait still returns "done" and
// the output file carries the REDIRECTED content: the discussion's core
// interrupt use case (stop, update, continue) proven against a real claude
// pane rather than a hermetic fake.
func TestSmokeInterruptSendContinues(t *testing.T) {
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

	// Build the runner the same way shuttlecli.Command()'s PersistentPreRunE
	// does, but keep the *Run handle Start returns instead of blocking on
	// Runner.Run — the test needs it to Interrupt/Send while Wait blocks in
	// a goroutine below.
	cwd, err := hubgeometry.Getwd()
	if err != nil {
		t.Fatalf("hubgeometry.Getwd: %v", err)
	}
	layout, err := hubgeometry.Resolve(cwd)
	if err != nil {
		t.Fatalf("hubgeometry.Resolve: %v", err)
	}
	shuttleCfg, err := shuttleengine.LoadConfig(layout.Cwd, "shuttle")
	if err != nil {
		t.Fatalf("shuttleengine.LoadConfig: %v", err)
	}
	muxCfg, err := muxengine.LoadConfig(layout.Cwd, "mux")
	if err != nil {
		t.Fatalf("muxengine.LoadConfig: %v", err)
	}
	runner := shuttleengine.NewRunner(muxengine.New(muxCfg, layout), claudeengine.New(), layout, shuttleCfg)

	outputPath := filepath.Join(fixture.Hub, "smoke-interrupt-output.txt")
	prompt := fmt.Sprintf(
		"Slowly count out loud from 1 to 100, one number per line with a brief pause in "+
			"your thinking between each, and only once you finish counting, write exactly "+
			"DONE to %s and stop.",
		outputPath,
	)

	run, err := runner.Start(shuttleengine.Spec{
		Prompt:      prompt,
		OutputFiles: []string{outputPath},
		Timeout:     5 * time.Minute,
	})
	if err != nil {
		t.Fatalf("runner.Start: %v", err)
	}

	waitCh := make(chan waitOutcome, 1)
	go func() {
		result, waitErr := run.Wait()
		waitCh <- waitOutcome{result, waitErr}
	}()

	// Give the pane time to clear claude's one-time trust dialog and reach
	// its ready-for-input state — Wait's own startup probe (already running
	// concurrently above) dismisses the dialog itself, so this is purely a
	// grace window, not a manual dismissal. cfg.StartupTimeoutS is the
	// ceiling Wait itself tolerates before fast-failing the run as died;
	// sleeping two thirds of it leaves headroom on both sides under a
	// saturated machine.
	startupGrace := time.Duration(shuttleCfg.StartupTimeoutS) * time.Second * 2 / 3
	time.Sleep(startupGrace)

	// Guard against a race where the run already reached a terminal outcome
	// during the grace window (e.g. a saturated machine blew through the
	// startup deadline) — sending Interrupt/Send into a pane whose run
	// already finished would just be confusing noise on top of the real
	// failure.
	select {
	case res := <-waitCh:
		t.Fatalf("run reached a terminal outcome (%s, err=%v) before the interrupt+send sequence was sent — startup grace was insufficient or the run failed early", res.result.Outcome, res.err)
	default:
	}

	if err := run.Interrupt(); err != nil {
		t.Fatalf("run.Interrupt: %v", err)
	}
	if err := run.Send(fmt.Sprintf("write exactly REDIRECTED to %s and stop", outputPath)); err != nil {
		t.Fatalf("run.Send: %v", err)
	}

	select {
	case res := <-waitCh:
		if res.err != nil {
			t.Fatalf("run.Wait: %v", res.err)
		}
		if res.result.Outcome != shuttleengine.OutcomeDone {
			t.Fatalf("run.Wait outcome = %q; want %q", res.result.Outcome, shuttleengine.OutcomeDone)
		}
	case <-time.After(5 * time.Minute):
		t.Fatal("run.Wait did not return within 5m after the interrupt+send sequence")
	}

	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read output file: %v", err)
	}
	if got := strings.TrimSpace(string(data)); got != "REDIRECTED" {
		t.Errorf("output file content = %q; want %q (the interrupt+send sequence must have redirected the run)", got, "REDIRECTED")
	}
}
