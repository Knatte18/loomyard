//go:build tmux

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
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/burlerengine"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/seatengine"
	"github.com/Knatte18/loomyard/internal/shedadapters"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/stencilstore"
	"github.com/Knatte18/loomyard/internal/testkit/shuttlefake"
	"github.com/Knatte18/loomyard/internal/testkit/stencilkit"
)

// shellLaunchEngine is a shuttleengine.Engine that launches a plain shell script instead of a provider session.
// Prepare and ParseEvents carry behaviour;
// every other method is the embedded shuttlefake.Engine's inert answer, because the attach path this suite exercises -- Start's run.json persistence, reed's liveness answer, and Wait's events-plus-output-files poll -- never reaches any of them.
// It is used through a pointer, since the embedded fake holds a mutex.
//
// The script it writes reproduces a real run's completion shape in the order shuttle's Wait requires
// it: stay quiet for a moment (so the round is in flight when the probe runs), write every declared
// output file, THEN append a turn-end line to events.jsonl, then keep running. Wait classifies done
// only on an event whose tick also finds every output file present, so writing the event first would
// leave the run held instead.
type shellLaunchEngine struct {
	shuttlefake.Engine
	// quietSeconds is how long the script waits before writing anything, so the run is genuinely
	// in flight rather than already complete when the producer's probe runs.
	quietSeconds int
}

func (e *shellLaunchEngine) Prepare(runDir string, spec shuttleengine.Spec, _ shuttleengine.Config) (shuttleengine.Launch, error) {
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
func (e *shellLaunchEngine) ParseEvents(data []byte) ([]shuttleengine.Event, error) {
	if len(data) == 0 {
		return nil, nil
	}
	return []shuttleengine.Event{{Kind: shuttleengine.EventStop, Raw: data}}, nil
}

// probingHalfRunner is a shedadapters.BurlerHalfRunner whose ProbeReview and ProbeFix probe the half's output file through the real shuttleengine.Runner, as burlerengine.Engine does, and whose ResumeReview and ResumeFix wait on the live half.
// Review and Fix fail the test: they stand in for the burlerengine half this producer must NOT start while an equivalent one is still alive.
type probingHalfRunner struct {
	t       *testing.T
	runner  *shuttleengine.Runner
	timeout time.Duration
}

func (r probingHalfRunner) Review(_ burlerengine.Profile, _ burlerengine.RunOpts) (burlerengine.ReviewResult, error) {
	r.t.Error("burlerengine review was started while an equivalent live run existed; want the producer to resume it instead")
	return burlerengine.ReviewResult{}, nil
}

func (r probingHalfRunner) ProbeReview(p burlerengine.Profile, _ burlerengine.RunOpts) (burlerengine.LiveHalf, error) {
	return r.probe(p.ReviewPath)
}

func (r probingHalfRunner) ResumeReview(_ burlerengine.Profile, _ burlerengine.RunOpts, live burlerengine.LiveHalf) (burlerengine.ReviewResult, error) {
	result, err := live.Handle.Wait()
	if err != nil || result.Outcome != shuttleengine.OutcomeDone {
		return burlerengine.ReviewResult{Outcome: result.Outcome}, err
	}
	return burlerengine.ReviewResult{Outcome: shuttleengine.OutcomeDone, Digest: "smoke-digest"}, nil
}

func (r probingHalfRunner) Fix(_ burlerengine.Profile, _ burlerengine.RunOpts, _ string) (burlerengine.FixResult, error) {
	r.t.Error("burlerengine fix was started while an equivalent live run existed; want the producer to resume it instead")
	return burlerengine.FixResult{}, nil
}

func (r probingHalfRunner) ProbeFix(p burlerengine.Profile, _ burlerengine.RunOpts) (burlerengine.LiveHalf, error) {
	return r.probe(p.FixerReportPath)
}

func (r probingHalfRunner) ResumeFix(_ burlerengine.Profile, _ burlerengine.RunOpts, _ string, live burlerengine.LiveHalf) (burlerengine.FixResult, error) {
	result, err := live.Handle.Wait()
	if err != nil || result.Outcome != shuttleengine.OutcomeDone {
		return burlerengine.FixResult{Outcome: result.Outcome}, err
	}
	return burlerengine.FixResult{Outcome: shuttleengine.OutcomeDone}, nil
}

// probe reports the live run declaring outputPath, or a half that is gone.
func (r probingHalfRunner) probe(outputPath string) (burlerengine.LiveHalf, error) {
	run, found, err := r.runner.ProbeGated(shuttleengine.Spec{OutputFiles: []string{outputPath}, Timeout: r.timeout}, nil)
	if err != nil || !found {
		return burlerengine.LiveHalf{}, err
	}
	return burlerengine.LiveHalf{State: burlerengine.HalfLive, Handle: run}, nil
}

// TestSmokeBurlerReview_AttachesToALiveReviewerInsteadOfRespawning starts one real, live shuttle run declaring a round's review.
// It then calls BurlerReviewProducer.Call against the same run directory and asserts it resumed that run rather than starting a second reviewer.
// It also asserts the live run's own artifact was left where it wrote it and that the accepted review was recorded.
func TestSmokeBurlerReview_AttachesToALiveReviewerInsteadOfRespawning(t *testing.T) {
	tmuxBinaryPath(t)
	_, loc, worktree, _ := newWiredPairFixture(t)
	registerBootstrapTeardown(t, loc, worktree)

	reedEngine := probeReedEngine(t, loc)
	if _, err := reedEngine.Up(); err != nil {
		t.Fatalf("reed up: %v", err)
	}

	runDir := filepath.Join(loomengine.LoomReviewsDir(loc), "attach-probe-review")
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%s): %v", runDir, err)
	}
	// The round-artifact convention is a durable two-sided contract (see internal/shedadapters'
	// package documentation), so naming the files literally here is pinning that contract, not
	// duplicating a derivation.
	reviewPath := filepath.Join(runDir, "round-1-review.md")

	// The stand-in run's quiet period is load-bearing:
	// a review already on disk would be a finished review, and the producer would hand it on without probing anything.
	shuttleCfg, err := shuttleengine.LoadConfig(configengine.Dirs{Anchor: loc.AnchorPath()}, "shuttle")
	if err != nil {
		t.Fatalf("load shuttle config: %v", err)
	}
	reedGeom, err := hubgeom.ReedGeometry(loc)
	if err != nil {
		t.Fatalf("reed geometry: %v", err)
	}
	runner := shuttleengine.NewRunner(reedEngine, shuttlefake.EngineSet(&shellLaunchEngine{quietSeconds: 3}), reedGeom.AnchorPath, reedGeom.WorktreeRoot, shuttleCfg)

	live, err := runner.Start(shuttleengine.Spec{
		Prompt:      "smoke: stand in for a live burler reviewer",
		OutputFiles: []string{reviewPath},
		Role:        "burler-review",
		Round:       "1",
		Timeout:     2 * time.Minute,
	})
	if err != nil {
		t.Fatalf("start the live reviewer stand-in run: %v", err)
	}
	t.Cleanup(func() { _, _ = reedEngine.RemoveStrand(live.StrandGUID(), false) })

	producer, err := shedadapters.NewBurlerReviewProducer(
		"Webster-Burler-Review",
		shedadapters.BurlerHalfDeps{Runner: probingHalfRunner{t: t, runner: runner, timeout: 2 * time.Minute}},
		burlerengine.Profile{Rubric: "smoke rubric", FixScope: burlerengine.FixScopeOverlay},
		burlerengine.RunOpts{Timeout: 2 * time.Minute},
		runDir,
		nil,
	)
	if err != nil {
		t.Fatalf("NewBurlerReviewProducer: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	outcome, ptr, err := producer.Call(ctx)
	if err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}
	if outcome != shedengine.Stuck {
		t.Errorf("Call() outcome = %q; want %q (an accepted review hands off to the fix row)", outcome, shedengine.Stuck)
	}
	if ptr.Path != reviewPath {
		t.Errorf("Call() pointer = %q; want %q", ptr.Path, reviewPath)
	}

	// The live run's own bytes must still be at the canonical path: archiving would have renamed
	// them to a stamped sibling, which is precisely what breaks an attached run's file contract.
	content, readErr := os.ReadFile(reviewPath)
	if readErr != nil {
		t.Fatalf("os.ReadFile(%s) = %v; want the live run's own file left in place", reviewPath, readErr)
	}
	if string(content) != "live" {
		t.Errorf("%s content = %q; want %q -- the attached run's artifact must be the one that survives", reviewPath, content, "live")
	}
	if _, err := os.Stat(filepath.Join(runDir, "round-1-acceptance.md")); err != nil {
		t.Errorf("os.Stat(acceptance record) = %v; want the resumed review recorded as accepted", err)
	}

	status, err := reedEngine.Status()
	if err != nil {
		t.Fatalf("reed status: %v", err)
	}
	for _, s := range status.Strands {
		if s.Live {
			t.Errorf("live strand %q (%s) survived the attached review; want none -- a leftover pane here is a second agent nothing owns", s.Name, s.GUID)
		}
	}
}

// newLeadSeatProducer returns a MultiLLMProducer over a one-seat table whose lead declares outputFile under the role prefix "discussion" and round "1", run by a seat engine over runner.
// The lead's stencil is a one-line stencil written into a stencilkit-seeded directory.
func newLeadSeatProducer(t *testing.T, reedGeom reedengine.Geometry, runner *shuttleengine.Runner, outputFile string) *shedadapters.MultiLLMProducer {
	t.Helper()
	stencilsDir := t.TempDir()
	stencilkit.SeedInto(t, stencilsDir)
	const leadStencil = "smoke-lead"
	leadPath := stencilstore.Path(stencilsDir, leadStencil)
	if err := os.MkdirAll(filepath.Dir(leadPath), 0o755); err != nil {
		t.Fatalf("mkdir stencil parent %s: %v", filepath.Dir(leadPath), err)
	}
	if err := os.WriteFile(leadPath, []byte("smoke: stand in for a lead seat.\n"), 0o644); err != nil {
		t.Fatalf("write stencil %s: %v", leadPath, err)
	}

	table := seatengine.Table{
		RolePrefix: "discussion",
		Round:      "1",
		Timeout:    2 * time.Minute,
		Seats:      []seatengine.Seat{{Stencil: leadStencil, Outputs: []string{outputFile}}},
	}
	seats := seatengine.New(seatengine.RunnerShuttle(runner), seatengine.Geometry{
		WorktreeRoot: reedGeom.WorktreeRoot,
		AnchorPath:   reedGeom.AnchorPath,
		StencilsDir:  stencilsDir,
		Shortname:    reedGeom.NameShortname,
		Slug:         reedGeom.NameSlug,
	})
	return shedadapters.NewMultiLLMProducerSourced("Discussion-Write", func() (seatengine.Table, error) { return table, nil }, seats, nil, nil)
}

// TestSmokeLeadSeat_HarvestsAFinishedRunWithReedStateGone is the live-substrate guard for crucible round opus5-high-r7's F1:
// Attach's reed-state gates must consult the run's file contract before refusing,
// so a producer whose agent already finished advances instead of hard-failing the step.
//
// It exists as a smoke test on top of internal/shuttleengine's own hermetic coverage because the defect's cost is only visible at the composed layer.
// Attach returning an error is not obviously wrong in isolation, since an unreadable strand table genuinely is a problem worth reporting.
// But the seat producer turns that error into a hard Shed step failure over work that was already finished and sitting on disk, and nothing then removes the run directory, so every later resume re-refuses identically.
// The defect was reproduced in exactly this shape against the real built binary before the fix.
// The stand-in run is started by runner.Start, as a run started before the seat producer existed was,
// so the producer must attach to it instead of respawning.
//
// The substitution is the same one this file's other test makes and no more: a stub Engine launching a shell script instead of a provider.
// Everything Attach reads is genuine: a real run.json written by a real Start, real output files, and reed's own real state file.
// ZERO provider subprocesses are spawned.
func TestSmokeLeadSeat_HarvestsAFinishedRunWithReedStateGone(t *testing.T) {
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

	shuttleCfg, err := shuttleengine.LoadConfig(configengine.Dirs{Anchor: loc.AnchorPath()}, "shuttle")
	if err != nil {
		t.Fatalf("load shuttle config: %v", err)
	}
	reedGeom, err := hubgeom.ReedGeometry(loc)
	if err != nil {
		t.Fatalf("reed geometry: %v", err)
	}
	runner := shuttleengine.NewRunner(reedEngine, shuttlefake.EngineSet(&shellLaunchEngine{quietSeconds: 1}), reedGeom.AnchorPath, reedGeom.WorktreeRoot, shuttleCfg)

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

	producer := newLeadSeatProducer(t, reedGeom, runner, outputFile)

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

// TestSmokeLeadSeat_RerunRemovesTheSupersededStrand is the live-substrate guard for re-running a producer over a halted run:
// the earlier run's record sits at a terminal outcome with its shell strand still live,
// and the producer's fresh run must not start beside it.
// A live strand with other output files, standing in for another producer's agent, survives.
// That strand takes the role "other", since the seat engine refuses a lead whose strand name another live strand holds.
//
// The stub Engine and the real Runner, reed session and tmux panes are the ones this file's other tests use.
func TestSmokeLeadSeat_RerunRemovesTheSupersededStrand(t *testing.T) {
	tmuxBinaryPath(t)
	_, loc, worktree, _ := newWiredPairFixture(t)
	registerBootstrapTeardown(t, loc, worktree)

	reedEngine := probeReedEngine(t, loc)
	if _, err := reedEngine.Up(); err != nil {
		t.Fatalf("reed up: %v", err)
	}

	outputDir := filepath.Join(loomengine.LoomReviewsDir(loc), "rerun-supersedes")
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%s): %v", outputDir, err)
	}
	outputFile := filepath.Join(outputDir, "rerun.md")
	otherOutputFile := filepath.Join(outputDir, "other.md")

	shuttleCfg, err := shuttleengine.LoadConfig(configengine.Dirs{Anchor: loc.AnchorPath()}, "shuttle")
	if err != nil {
		t.Fatalf("load shuttle config: %v", err)
	}
	reedGeom, err := hubgeom.ReedGeometry(loc)
	if err != nil {
		t.Fatalf("reed geometry: %v", err)
	}
	// The first two runs stay quiet and never write their outputs;
	// the fresh run the producer starts writes its output after a second.
	launchEngine := &shellLaunchEngine{quietSeconds: 600}
	runner := shuttleengine.NewRunner(reedEngine, shuttlefake.EngineSet(launchEngine), reedGeom.AnchorPath, reedGeom.WorktreeRoot, shuttleCfg)

	spec := shuttleengine.Spec{
		Prompt:      "smoke: stand in for a producer re-run over a halted session",
		OutputFiles: []string{outputFile},
		Role:        "discussion",
		Round:       "1",
		Timeout:     2 * time.Minute,
	}
	halted, err := runner.Start(spec)
	if err != nil {
		t.Fatalf("start the halted stand-in run: %v", err)
	}
	t.Cleanup(func() { _, _ = reedEngine.RemoveStrand(halted.StrandGUID(), false) })

	otherSpec := spec
	otherSpec.OutputFiles = []string{otherOutputFile}
	otherSpec.Role = "other"
	other, err := runner.Start(otherSpec)
	if err != nil {
		t.Fatalf("start the other-outputs stand-in run: %v", err)
	}
	t.Cleanup(func() { _, _ = reedEngine.RemoveStrand(other.StrandGUID(), false) })

	// Leave the first run's record at a terminal outcome, as a halted run does, with its pane still alive.
	runRoot := filepath.Join(reedGeom.AnchorPath, lyxdirs.DotLyxDirName, "shuttle")
	if shuttleCfg.RunDir != "" {
		runRoot = shuttleCfg.RunDir
		if !filepath.IsAbs(runRoot) {
			runRoot = filepath.Join(reedGeom.AnchorPath, runRoot)
		}
	}
	markRunTerminal(t, runRoot, halted.StrandGUID(), "timeout")

	launchEngine.quietSeconds = 1
	producer := newLeadSeatProducer(t, reedGeom, runner, outputFile)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	outcome, _, err := producer.Call(ctx)
	if err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}
	if outcome != shedengine.Done {
		t.Errorf("Call() outcome = %q; want %q", outcome, shedengine.Done)
	}

	status, err := reedEngine.Status()
	if err != nil {
		t.Fatalf("reed status: %v", err)
	}
	var otherAlive bool
	for _, s := range status.Strands {
		switch s.GUID {
		case halted.StrandGUID():
			t.Errorf("superseded strand %q still in reed status; want it removed before the fresh run", s.GUID)
		case other.StrandGUID():
			otherAlive = s.Live
		}
	}
	if !otherAlive {
		t.Errorf("the strand with other outputs is not live; want it left alone")
	}
}

// markRunTerminal rewrites the run.json of the run owning strandGUID under runRoot so its outcome reads outcome, leaving every other field as Start wrote it.
func markRunTerminal(t *testing.T, runRoot, strandGUID, outcome string) {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(runRoot, "*", "run.json"))
	if err != nil {
		t.Fatalf("glob run records: %v", err)
	}
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		var fields map[string]any
		if err := json.Unmarshal(data, &fields); err != nil {
			t.Fatalf("decode %s: %v", path, err)
		}
		if fields["strandGuid"] != strandGUID {
			continue
		}
		fields["outcome"] = outcome
		updated, err := json.MarshalIndent(fields, "", "  ")
		if err != nil {
			t.Fatalf("encode %s: %v", path, err)
		}
		if err := os.WriteFile(path, updated, 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
		return
	}
	t.Fatalf("no run record under %s names strand %s", runRoot, strandGUID)
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
