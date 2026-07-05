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
	"regexp"
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

// countingLine matches a pane line that is just a number — the mid-turn
// signal TestSmokeInterruptSendContinues waits on: several of these mean the
// agent is actively counting (its turn is underway), so an interrupt has
// something to interrupt. Polling for this instead of sleeping a fixed grace
// is what makes the test deterministic — a fast model that would have blown
// through a fixed sleep is still caught mid-turn because the task never
// self-completes.
var countingLine = regexp.MustCompile(`(?m)^\s*\d+\s*$`)

// waitOutcome carries a completed run.Wait() call's result off the
// background goroutine TestSmokeInterruptSendContinues drives it in, so the
// test can select on it after sending the interrupt-and-redirect sequence.
type waitOutcome struct {
	result shuttleengine.Result
	err    error
}

// TestSmokeInterruptSendContinues starts a run whose prompt directs an
// open-ended, never-self-completing task, polls the pane's captured content
// until the task is genuinely underway (deterministic mid-turn detection —
// see countingLine), then calls run.Interrupt() followed by run.Send() with a
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
	muxEngine := muxengine.New(muxCfg, layout)
	runner := shuttleengine.NewRunner(muxEngine, claudeengine.New(), layout, shuttleCfg)

	outputPath := filepath.Join(fixture.Hub, "smoke-interrupt-output.txt")
	// Deliberately open-ended: the task has no natural completion point
	// before the interrupt+send sequence redirects it. A fixed-duration task
	// (e.g. "count to 100") flakes on a fast model that finishes inside
	// whatever grace window the test sleeps — the interrupt+send path never
	// gets exercised, and the test still passes on the resulting "done"
	// outcome (a false positive for the exact behavior it means to prove;
	// see the round-1 review finding this replaced). Counting upward forever
	// guarantees the run is never done on its own, so reaching the poll
	// below always means the agent is genuinely mid-turn.
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

	// Deterministic mid-turn detection: poll the pane's own captured content
	// for the counting task actually underway (at least a few numeric
	// lines), rather than sleeping a fixed grace window. This survives both
	// a slow machine (keeps polling past the old fixed sleep) and a fast one
	// (the open-ended task above guarantees there is always a mid-turn state
	// to observe — it never self-completes).
	guid := run.StrandGUID()
	deadline := time.Now().Add(time.Duration(shuttleCfg.StartupTimeoutS)*time.Second + 60*time.Second)
	for {
		select {
		case res := <-waitCh:
			t.Fatalf("run reached a terminal outcome (%s, err=%v) before the interrupt+send sequence was sent — the agent never reached a countable mid-turn state", res.result.Outcome, res.err)
		default:
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for the pane to show the counting task underway (guid %s)", guid)
		}

		capture, err := muxEngine.CapturePane(guid)
		if err != nil {
			t.Logf("CapturePane during mid-turn poll (retrying): %v", err)
			time.Sleep(time.Second)
			continue
		}
		if len(countingLine.FindAllString(capture, -1)) >= 3 {
			break
		}
		time.Sleep(time.Second)
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
