//go:build smoke

// smoke_attachprobe_test.go is the live-substrate regression guard for the two ways a producer's
// attach probe has failed a crashed run for real.
//
// The duplicate-agent defect a crucible round reproduced twice: a driver crash inside a review
// segment left the round's agent alive, and the next producer Call spawned a second one over it --
// two sessions writing the same review and fixer-report files, and on a fix-scope: source row, two
// sessions committing to the same branch.
//
// And its mirror image, crucible round opus5-high-r7's F1: a driver crash AFTER its agent finished,
// with reed's own strand table then gone, made Attach refuse outright rather than harvest the
// finished work -- failing the whole Shed step over an already-completed, already-paid-for LLM turn.
// The first defect is about not starting a second agent; the second is about not throwing away the
// first one's result. Both are answered by the same probe, which is why both live here.
//
// It drives the REAL substrate -- a real hub, a real reed session, a real tmux pane, a real
// shuttleengine.Runner writing a real run.json -- and spawns ZERO provider subprocesses. The one
// substitution is shuttleengine's own provider seam: a stub Engine whose Prepare returns a plain
// shell command instead of a provider launch line. That substitution is the point rather than a
// shortcut. What the probe matches on is a persisted run.json's OutputFiles set plus reed's own
// liveness answer for its strand, and both are produced identically whatever the pane happens to be
// running, so a shell pane exercises the probe end to end at no token cost.
//
// It lives in this package rather than in internal/shedadapters because everything it needs already
// exists here: the hub fixture, the tmux skip, the reed engine probe, and the hermetic git TestMain.

package loomcli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/burlerengine"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
	"github.com/Knatte18/loomyard/internal/shedadapters"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// shellLaunchEngine is a shuttleengine.Engine that launches a plain shell script instead of a
// provider session. Prepare and ParseEvents carry behaviour; every other method answers the zero
// value, because the attach path this suite exercises -- Start's run.json persistence, reed's
// liveness answer, and Wait's events-plus-output-files poll -- never reaches any of them.
//
// The script it writes reproduces a real run's completion shape in the order shuttle's Wait requires
// it: stay quiet for a moment (so the round is in flight when the probe runs), write every declared
// output file, THEN append a turn-end line to events.jsonl, then keep running. Wait classifies done
// only on an event whose tick also finds every output file present, so writing the event first would
// classify the run asking instead.
type shellLaunchEngine struct {
	// quietSeconds is how long the script waits before writing anything, so the run is genuinely
	// in flight rather than already complete when the producer's probe runs.
	quietSeconds int
}

func (e shellLaunchEngine) Prepare(runDir string, spec shuttleengine.Spec, _ shuttleengine.Config) (shuttleengine.Launch, error) {
	eventsPath := filepath.Join(runDir, "events.jsonl")

	script := fmt.Sprintf("#!/bin/sh\nsleep %d\n", e.quietSeconds)
	for _, out := range spec.OutputFiles {
		script += fmt.Sprintf("printf live > '%s'\n", out)
	}
	script += fmt.Sprintf("printf 'turn-end\\n' >> '%s'\n", eventsPath)
	script += "sleep 600\n"

	scriptPath := filepath.Join(runDir, "smoke-launch.sh")
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		return shuttleengine.Launch{}, fmt.Errorf("write smoke launch script: %w", err)
	}
	return shuttleengine.Launch{Cmd: "sh " + scriptPath, SessionID: "smoke-attach-probe"}, nil
}

// ParseEvents maps any non-empty events.jsonl content onto a single turn-end event, which is the one
// provider fact Wait's completion test needs from this seam.
func (e shellLaunchEngine) ParseEvents(data []byte) ([]shuttleengine.Event, error) {
	if len(data) == 0 {
		return nil, nil
	}
	return []shuttleengine.Event{{Kind: shuttleengine.EventStop, Raw: data}}, nil
}

func (e shellLaunchEngine) Startup(_ string) shuttleengine.StartupState {
	return shuttleengine.StartupReady
}

func (e shellLaunchEngine) InterruptSequence() []shuttleengine.PaneInput          { return nil }
func (e shellLaunchEngine) TrustDismissSequence(string) []shuttleengine.PaneInput { return nil }
func (e shellLaunchEngine) ComposeSend(_ string) []shuttleengine.PaneInput        { return nil }
func (e shellLaunchEngine) ModelSwitchSequence(_ string) []shuttleengine.PaneInput {
	return nil
}

func (e shellLaunchEngine) AuditForks(_, _ string) (shuttleengine.ForkAudit, error) {
	return shuttleengine.ForkAudit{}, nil
}

func (e shellLaunchEngine) AuditForksIncremental(_, _ string, _ map[string]bool) (shuttleengine.ForkAudit, error) {
	return shuttleengine.ForkAudit{}, nil
}

// refusingBurlerRunner fails the test if a round is ever run through it. It stands in for the
// burlerengine round this producer must NOT start while an equivalent one is still alive.
type refusingBurlerRunner struct {
	t *testing.T
}

func (r refusingBurlerRunner) Run(_ burlerengine.Profile, _ burlerengine.RunOpts) (burlerengine.Result, error) {
	r.t.Error("burlerengine round was started while an equivalent live run existed; want the producer to attach to it instead")
	return burlerengine.Result{}, nil
}

// TestSmokeBurlerRound_AttachesToALiveRoundInsteadOfRespawning starts a real, live shuttle run whose
// declared output files are a burler round's own review/fixer-report pair, then calls
// BurlerProducer.Call against the same run directory and asserts it attached to that run rather than
// starting a second one -- and that the live run's own artifacts were left where it wrote them.
func TestSmokeBurlerRound_AttachesToALiveRoundInsteadOfRespawning(t *testing.T) {
	tmuxBinaryPath(t)
	_, loc, worktree, _ := newWiredPairFixture(t)
	registerBootstrapTeardown(t, loc, worktree)

	reedEngine := probeReedEngine(t, loc)
	if _, err := reedEngine.Up(); err != nil {
		t.Fatalf("reed up: %v", err)
	}

	runDir := filepath.Join(loomengine.LoomReviewsDir(loc), "attach-probe")
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%s): %v", runDir, err)
	}
	// The round-artifact convention is a durable two-sided contract (see internal/shedadapters'
	// package documentation), so naming the two files literally here is pinning that contract, not
	// duplicating a derivation.
	reviewPath := filepath.Join(runDir, "round-1-review.md")
	fixerReportPath := filepath.Join(runDir, "round-1-fixer-report.md")

	// The stand-in run's own quiet period is load-bearing rather than cosmetic: a round whose
	// artifacts are already on disk is a COMPLETE round, so the producer would resolve the NEXT
	// round number and correctly find nothing live for it. The pause is what makes this an
	// in-flight round 1, which is the state a driver crash actually leaves behind. It is not a
	// synchronisation point either way -- Wait polls against its own deadline rather than assuming
	// any timing.
	shuttleCfg, err := shuttleengine.LoadConfig(loc.AnchorPath(), "shuttle")
	if err != nil {
		t.Fatalf("load shuttle config: %v", err)
	}
	reedGeom := hubgeom.ReedGeometry(loc)
	runner := shuttleengine.NewRunner(reedEngine, shellLaunchEngine{quietSeconds: 3}, reedGeom.AnchorPath, reedGeom.WorktreeRoot, shuttleCfg)

	liveSpec := shuttleengine.Spec{
		Prompt:      "smoke: stand in for a live burler round",
		OutputFiles: []string{reviewPath, fixerReportPath},
		Role:        "burler",
		Round:       "1",
		Timeout:     2 * time.Minute,
	}
	live, err := runner.Start(liveSpec)
	if err != nil {
		t.Fatalf("start the live stand-in run: %v", err)
	}
	t.Cleanup(func() { _, _ = reedEngine.RemoveStrand(live.StrandGUID(), false) })

	producer, err := shedadapters.NewBurlerProducer(
		"Webster-Burler",
		refusingBurlerRunner{t: t},
		runner,
		burlerengine.Profile{Rubric: "smoke rubric", FixScope: burlerengine.FixScopeOverlay},
		burlerengine.RunOpts{Timeout: 2 * time.Minute},
		runDir,
		nil,
	)
	if err != nil {
		t.Fatalf("NewBurlerProducer: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	outcome, ptr, err := producer.Call(ctx)
	if err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}
	if outcome != shedengine.Stuck {
		t.Errorf("Call() outcome = %q; want %q (a completed round hands off to its Bouncer)", outcome, shedengine.Stuck)
	}
	if ptr.Path != reviewPath {
		t.Errorf("Call() pointer = %q; want %q", ptr.Path, reviewPath)
	}

	// The live run's own bytes must still be at the canonical paths: archiving would have renamed
	// them to stamped siblings, which is precisely what breaks an attached run's file contract.
	for _, path := range []string{reviewPath, fixerReportPath} {
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Errorf("os.ReadFile(%s) = %v; want the live run's own file left in place", path, readErr)
			continue
		}
		if string(content) != "live" {
			t.Errorf("%s content = %q; want %q -- the attached run's artifact must be the one that survives", path, content, "live")
		}
	}

	// No agent pane left behind. The attached run's own strand is reaped by the wait loop's finalize
	// step once it completes, and a respawn -- had one happened -- would have left its own pane
	// running beside it, since nothing in this test ever tears a second one down.
	status, err := reedEngine.Status()
	if err != nil {
		t.Fatalf("reed status: %v", err)
	}
	for _, s := range status.Strands {
		if s.Live {
			t.Errorf("live strand %q (%s) survived the attached round; want none -- a leftover pane here is a second agent nothing owns", s.Name, s.GUID)
		}
	}
}

// TestSmokeSingleLLM_HarvestsAFinishedRunWithReedStateGone is the live-substrate guard for crucible
// round opus5-high-r7's F1: Attach's reed-state gates must consult the run's file contract before
// refusing, so a producer whose agent already finished advances instead of hard-failing the step.
//
// It exists as a smoke test on top of internal/shuttleengine's own hermetic coverage because the
// defect's cost is only visible at the composed layer. Attach returning an error is not obviously
// wrong in isolation -- an unreadable strand table genuinely is a problem worth reporting -- but
// SingleLLMProducer.Call turns that error into a hard Shed step failure over work that was already
// finished and sitting on disk, and nothing then removes the run directory, so every later resume
// re-refuses identically. Reproduced in exactly this shape against the real built binary before the
// fix.
//
// The substitution is the same one this file's other test makes and no more: a stub Engine launching
// a shell script instead of a provider. Everything Attach reads -- a real run.json written by a real
// Start, real output files, and reed's own real state file -- is genuine, and ZERO provider
// subprocesses are spawned.
func TestSmokeSingleLLM_HarvestsAFinishedRunWithReedStateGone(t *testing.T) {
	tmuxBinaryPath(t)
	_, loc, worktree, _ := newWiredPairFixture(t)
	registerBootstrapTeardown(t, loc, worktree)

	reedEngine := probeReedEngine(t, loc)
	if _, err := reedEngine.Up(); err != nil {
		t.Fatalf("reed up: %v", err)
	}

	outputDir := filepath.Join(loomengine.LoomReviewsDir(loc), "reedless-harvest")
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%s): %v", outputDir, err)
	}
	outputFile := filepath.Join(outputDir, "finished.md")

	shuttleCfg, err := shuttleengine.LoadConfig(loc.AnchorPath(), "shuttle")
	if err != nil {
		t.Fatalf("load shuttle config: %v", err)
	}
	reedGeom := hubgeom.ReedGeometry(loc)
	runner := shuttleengine.NewRunner(reedEngine, shellLaunchEngine{quietSeconds: 1}, reedGeom.AnchorPath, reedGeom.WorktreeRoot, shuttleCfg)

	spec := shuttleengine.Spec{
		Prompt:      "smoke: stand in for an agent that finishes and is then orphaned",
		OutputFiles: []string{outputFile},
		Role:        "discussion",
		Round:       "1",
		Timeout:     2 * time.Minute,
	}
	started, err := runner.Start(spec)
	if err != nil {
		t.Fatalf("start the stand-in run: %v", err)
	}
	t.Cleanup(func() { _, _ = reedEngine.RemoveStrand(started.StrandGUID(), false) })

	// Wait for the script to satisfy the file contract. Start persisted run.json at "running" and
	// nothing has classified the run since, so once the file lands this is exactly the state a driver
	// crash after its agent finished leaves behind.
	waitForOutputFile(t, outputFile, 30*time.Second)

	// Now take reed's strand table away under the still-"running" record -- the `git clean -xdf` of
	// `.lyx` the Durable-vs-Ephemeral State Invariant sanctions, or reed's own documented remedy for a
	// corrupt state file. Restored on cleanup so registerBootstrapTeardown's own Down() can still find
	// the session it has to tear down.
	dotLyxDir := filepath.Join(reedGeom.AnchorPath, lyxdirs.DotLyxDirName)
	reedStatePath := filepath.Join(dotLyxDir, "reed.json")
	savedReedState, err := os.ReadFile(reedStatePath)
	if err != nil {
		t.Fatalf("read reed state before removing it: %v", err)
	}
	t.Cleanup(func() { _ = os.WriteFile(reedStatePath, savedReedState, 0o600) })
	if err := os.Remove(reedStatePath); err != nil {
		t.Fatalf("remove reed state: %v", err)
	}

	producer := shedadapters.NewSingleLLMProducer(
		"Discussion-Write",
		func() (shuttleengine.Spec, error) { return spec, nil },
		runner,
		nil,
		nil,
	)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	outcome, ptr, err := producer.Call(ctx)
	if err != nil {
		t.Fatalf("Call() error = %v; want nil -- an absent strand table answers \"reed's bookkeeping went wrong\", never \"did this run finish\", and this run's declared output file is already on disk", err)
	}
	if outcome != shedengine.Done {
		t.Errorf("Call() outcome = %q; want %q -- the finished run must be harvested, not respawned over", outcome, shedengine.Done)
	}
	if ptr.Path != outputFile {
		t.Errorf("Call() pointer = %q; want %q", ptr.Path, outputFile)
	}

	// The agent's own bytes must still be at the canonical path: a respawn would have archived them to
	// a stamped sibling and re-run the whole (expensive) step, which is the rework this guard exists
	// to prevent.
	content, err := os.ReadFile(outputFile)
	if err != nil {
		t.Fatalf("os.ReadFile(%s) = %v; want the finished run's own file left in place", outputFile, err)
	}
	if string(content) != "live" {
		t.Errorf("%s content = %q; want %q -- the harvested run's artifact must be the one that survives", outputFile, content, "live")
	}
}

// waitForOutputFile blocks until path exists or timeout elapses, failing the test when it never
// appears. It polls rather than synchronising on the launched script, because the script is a real
// subprocess in a real pane and this suite never assumes any timing from one.
func waitForOutputFile(t *testing.T, path string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("output file %s never appeared within %s; the stand-in run never satisfied its file contract", path, timeout)
}
