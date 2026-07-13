//go:build smoke

// smoke_test.go walks the two live-composed builder behaviors round fable-r2
// found broken against a REAL psmux server (no fake mux): poll's terminal
// strand release (a done batch must not leak its live pane — nobody else
// ever holds the shuttle Run handle) and spawn-batch's in-flight guard (a
// live strand behind a non-terminal cursor refuses the spawn). The pane runs
// a plain pwsh, never a real agent — the behaviors under test are builder's
// own substrate bookkeeping, not implementer quality. Both tests self-skip
// when the configured psmux binary is absent, mirroring muxengine's
// contract_integration_test.go discipline, and tear their scratch server
// down via t.Cleanup.

package buildercli

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/builderengine"
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/hubgeometry"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/muxengine"
	"github.com/Knatte18/loomyard/internal/muxengine/render"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// bootRealMux builds a scratch hub (git repo + plan fixture + mux config),
// boots a REAL psmux server/session on the hub's own derived socket, and
// registers teardown. Skips the calling test when the configured psmux
// binary is not on this box.
func bootRealMux(t *testing.T) (*muxengine.Engine, *hubgeometry.Layout, string) {
	t.Helper()

	hub := newScratchRepo(t)
	commitFile(t, hub, "base.txt", "base", "base commit")
	seedPlanFixture(t, hub, builderengineTestdataDir("plan-valid"))

	configDir := hubgeometry.ConfigDir(hub)
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	if err := os.WriteFile(hubgeometry.ConfigFile(hub, "mux"), []byte(muxengine.ConfigTemplate()), 0o644); err != nil {
		t.Fatalf("write mux config: %v", err)
	}

	cfg, err := muxengine.LoadConfig(hub, "mux")
	if err != nil {
		t.Fatalf("muxengine.LoadConfig: %v", err)
	}
	if _, err := exec.LookPath(cfg.Psmux); err != nil {
		t.Skipf("configured psmux binary %q not found: %v", cfg.Psmux, err)
	}

	layout := &hubgeometry.Layout{Hub: hub, WorktreeRoot: hub, Cwd: hub, RelPath: "."}
	eng := muxengine.New(cfg, layout)
	if _, err := eng.Up(); err != nil {
		t.Fatalf("mux Up: %v", err)
	}
	t.Cleanup(func() {
		if _, err := eng.Down(); err != nil {
			t.Errorf("mux Down: %v", err)
		}
	})

	return eng, layout, hub
}

// addLivePane launches one real pane strand (an empty Cmd leaves the pane's
// own pwsh idling — a live pane, no agent) and returns its guid, waiting
// until the live mux Status reports it live (pane realization is
// asynchronous from AddStrand's return).
func addLivePane(t *testing.T, eng *muxengine.Engine, role, round string) string {
	t.Helper()

	strand, err := eng.AddStrand(muxengine.AddSpec{
		Role:    role,
		Round:   round,
		Cmd:     "",
		Display: render.Display{Anchor: render.AnchorBelowParent},
	})
	if err != nil {
		t.Fatalf("AddStrand: %v", err)
	}

	deadline := time.Now().Add(15 * time.Second)
	for {
		live, err := builderengine.StrandLive(eng, strand.GUID)
		if err != nil {
			t.Fatalf("StrandLive: %v", err)
		}
		if live {
			return strand.GUID
		}
		if time.Now().After(deadline) {
			t.Fatalf("strand %s never became live within 15s", strand.GUID)
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// TestSmoke_PollDoneReleasesStrand proves against a real psmux server that a
// done classification releases the batch's strand: the pane is gone from the
// live mux Status after poll returns (the F1 leak, observed live as panes
// accumulating across runs).
func TestSmoke_PollDoneReleasesStrand(t *testing.T) {
	t.Setenv("WEFT_SKIP_GIT", "1")
	eng, layout, hub := bootRealMux(t)
	guid := addLivePane(t, eng, "implementer", "01-json-flag")

	c := &builderCLI{
		engine:     &pollFakeEngine{},
		mux:        eng,
		layout:     layout,
		cfg:        builderengine.Config{BatchTimeoutMin: 60, PollWaitS: 5},
		planDir:    hubgeometry.PlanDir(hub),
		builderDir: hubgeometry.BuilderDir(hub),
		reportsDir: hubgeometry.BuilderReportsDir(hub),
	}

	startSHA := strings.TrimSpace(mustGit(t, hub, "rev-parse", "HEAD"))
	st := &builderengine.State{
		CurrentBatch: 1,
		Batches: map[int]*builderengine.BatchState{
			1: {
				Slug:          "json-flag",
				StartSHA:      startSHA,
				Role:          "implementer",
				StrandGUID:    guid,
				ShuttleRunDir: t.TempDir(),
				EventsPath:    filepath.Join(t.TempDir(), "events.jsonl"),
				SpawnedAt:     time.Now().UTC().Format(time.RFC3339),
			},
		},
	}
	if err := builderengine.SaveState(c.builderDir, st); err != nil {
		t.Fatalf("SaveState: %v", err)
	}

	if err := os.MkdirAll(c.reportsDir, 0o755); err != nil {
		t.Fatalf("mkdir reports dir: %v", err)
	}
	reportPath := filepath.Join(c.reportsDir, "01-json-flag.yaml")
	if err := os.WriteFile(reportPath, []byte("batch: 01-json-flag\nstatus: done\ntests: green\nstuck_reason: null\n"), 0o644); err != nil {
		t.Fatalf("write report: %v", err)
	}

	var out bytes.Buffer
	if exitCode := clihelp.Execute(c.pollCmd(), &out, nil); exitCode != 0 {
		t.Fatalf("poll = %d; want 0, output: %s", exitCode, out.String())
	}
	if !strings.Contains(out.String(), `"status":"done"`) {
		t.Fatalf("poll output = %q; want status:done", out.String())
	}

	// RemoveStrand waits for the destroyed pane's process subtree, so the
	// release is observable immediately — but poll on a deadline anyway:
	// substrate state transitions are asynchronous by contract.
	deadline := time.Now().Add(15 * time.Second)
	for {
		live, err := builderengine.StrandLive(eng, guid)
		if err != nil {
			t.Fatalf("StrandLive after poll: %v", err)
		}
		if !live {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("strand %s still live 15s after a done classification; want it released", guid)
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// smokeFailStarter fails the test if SpawnBatch ever reaches the Starter —
// the in-flight guard under test must refuse first.
type smokeFailStarter struct{ t *testing.T }

func (s smokeFailStarter) Start(spec shuttleengine.Spec) (*shuttleengine.Run, error) {
	s.t.Error("Starter reached despite a live in-flight batch")
	return nil, errors.New("starter must not be reached")
}

// TestSmoke_SpawnRefusedWhileStrandLive proves against a real psmux server
// that spawn-batch's in-flight guard refuses while a non-terminal batch's
// strand is genuinely live (the F4 double-spawn, observed live as two
// implementer panes for the same batch).
func TestSmoke_SpawnRefusedWhileStrandLive(t *testing.T) {
	eng, layout, hub := bootRealMux(t)
	guid := addLivePane(t, eng, "implementer", "02-list-tests")

	planDir := hubgeometry.PlanDir(hub)
	plan, err := builderengine.ParsePlan(planDir)
	if err != nil {
		t.Fatalf("ParsePlan: %v", err)
	}
	fingerprint, err := builderengine.Fingerprint(planDir)
	if err != nil {
		t.Fatalf("Fingerprint: %v", err)
	}

	deps := builderengine.SpawnDeps{
		Starter: smokeFailStarter{t},
		Plan:    plan,
		State: &builderengine.State{
			PlanFingerprint: fingerprint,
			CurrentBatch:    2,
			Batches: map[int]*builderengine.BatchState{
				2: {Slug: "list-tests", StrandGUID: guid},
			},
		},
		Roles: map[builderengine.Role]modelspec.Resolved{
			builderengine.RoleImplementer: {Engine: "claude", Model: "implementer-model"},
		},
		Config:       builderengine.Config{SelfFixCap: 2, BatchTimeoutMin: 45},
		WorktreeRoot: hub,
		BuilderDir:   hubgeometry.BuilderDir(hub),
		ReportsDir:   hubgeometry.BuilderReportsDir(hub),
		Layout:       layout,
		Mux:          eng,
	}

	_, err = builderengine.SpawnBatch(deps, builderengine.SpawnBatchOptions{BatchNumber: 1})
	if !errors.Is(err, builderengine.ErrBatchInFlight) {
		t.Fatalf("SpawnBatch() error = %v; want errors.Is(err, ErrBatchInFlight)", err)
	}
}

// TestSmoke_RunEntryReclaimsOrphanedOrchestrator proves against a real psmux
// server that Run's entry-time reclaim stops a recorded orchestrator strand
// the mux still reports live (the fable-r4 double-orchestrator: a killed
// `run` process, or a timed-out orchestrator whose kept pane kept working)
// before the fresh orchestrator spawn ever starts.
func TestSmoke_RunEntryReclaimsOrphanedOrchestrator(t *testing.T) {
	eng, _, hub := bootRealMux(t)
	orphanGUID := addLivePane(t, eng, "orchestrator", "")

	planDir := hubgeometry.PlanDir(hub)
	fingerprint, err := builderengine.Fingerprint(planDir)
	if err != nil {
		t.Fatalf("Fingerprint: %v", err)
	}
	builderDir := hubgeometry.BuilderDir(hub)
	seeded := &builderengine.State{
		RunGUID:            "smoke-orphan-run",
		PlanFingerprint:    fingerprint,
		OrchestratorStrand: orphanGUID,
		Batches:            map[int]*builderengine.BatchState{},
		ChainStartSHAs:     map[int]string{},
	}
	if err := builderengine.SaveState(builderDir, seeded); err != nil {
		t.Fatalf("SaveState: %v", err)
	}

	starter := &fakeOrchestratorStarter{
		Result:       shuttleengine.Result{Outcome: shuttleengine.OutcomeDone},
		WriteOutcome: "outcome: done\nstuck_reason: null\nbatches_done: 0\n",
	}
	deps := builderengine.RunDeps{
		Runner: starter,
		Mux:    eng,
		Roles: map[builderengine.Role]modelspec.Resolved{
			builderengine.RoleOrchestrator: {Engine: "claude", Model: "orchestrator-model"},
		},
		Config: builderengine.Config{
			SelfFixCap: 2, PollWaitS: 5, BatchTimeoutMin: 45,
			OrchestratorTimeoutMin: 5, BatchContextCapTokens: 100000, BatchCardCap: 10,
		},
		PlanDir:      planDir,
		BuilderDir:   builderDir,
		ReportsDir:   hubgeometry.BuilderReportsDir(hub),
		WorktreeRoot: hub,
	}

	if _, err := builderengine.Run(deps, builderengine.RunOptions{}); err != nil {
		t.Fatalf("Run() error = %v; want nil", err)
	}

	// RemoveStrand waits for the destroyed pane's subtree, so the reclaim is
	// observable immediately -- but poll on a deadline anyway: substrate
	// state transitions are asynchronous by contract.
	deadline := time.Now().Add(15 * time.Second)
	for {
		live, err := builderengine.StrandLive(eng, orphanGUID)
		if err != nil {
			t.Fatalf("StrandLive after Run: %v", err)
		}
		if !live {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("orphaned orchestrator strand %s still live 15s after Run returned; want it reclaimed at run entry", orphanGUID)
		}
		time.Sleep(200 * time.Millisecond)
	}
}
