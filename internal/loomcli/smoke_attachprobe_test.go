//go:build smoke

// smoke_attachprobe_test.go is the live-substrate regression guard for the duplicate-agent defect a
// crucible round reproduced twice: a driver crash inside a review segment left the round's agent
// alive, and the next producer Call spawned a second one over it -- two sessions writing the same
// review and fixer-report files, and on a fix-scope: source row, two sessions committing to the same
// branch.
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

func (e shellLaunchEngine) InterruptSequence() []shuttleengine.PaneInput    { return nil }
func (e shellLaunchEngine) TrustDismissSequence() []shuttleengine.PaneInput { return nil }
func (e shellLaunchEngine) ComposeSend(_ string) []shuttleengine.PaneInput  { return nil }
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
