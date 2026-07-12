//go:build integration

// spawn_test.go exercises SpawnBatch end to end against a real scratch git
// repo (Tier 2 — see docs/benchmarks/running-tests.md, mirroring chain_test.go
// and gitquery_test.go): HeadSHA capture is genuine git, and the spawn itself
// runs through a real *shuttleengine.Runner wired to local fake
// shuttleengine.MuxOps/shuttleengine.Engine doubles (the shuttleengine
// fakes_test.go pattern — builderengine's own fakes are test-file-local, per
// the discussion's test-conventions decision), so Start produces a genuine
// *shuttleengine.Run whose run.json shuttleengine.FindRun can resolve. No
// real agent spawns anywhere in this file.

package builderengine_test

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/builderengine"
	"github.com/Knatte18/loomyard/internal/hubgeometry"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/muxengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// spawnFakeMux is a hermetic shuttleengine.MuxOps double: AddStrand mints a
// distinct GUID per call so multiple spawns in one test never collide;
// Status and RemoveStrand are scriptable/recorded so the in-flight guard and
// dead-respawn cleanup tests can drive and observe them; the send/capture
// methods stay inert, since SpawnBatch's path never exercises them.
type spawnFakeMux struct {
	mu             sync.Mutex
	counter        int
	status         muxengine.StatusResult
	statusErr      error
	removedStrands []string
}

func (m *spawnFakeMux) AddStrand(spec muxengine.AddSpec) (muxengine.Strand, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.counter++
	return muxengine.Strand{GUID: "spawn-test-strand-" + strconv.Itoa(m.counter)}, nil
}

func (m *spawnFakeMux) RemoveStrand(guid string, recursive bool) (muxengine.Removed, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.removedStrands = append(m.removedStrands, guid)
	return muxengine.Removed{}, nil
}

func (m *spawnFakeMux) Status() (muxengine.StatusResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.statusErr != nil {
		return muxengine.StatusResult{}, m.statusErr
	}
	return m.status, nil
}

func (m *spawnFakeMux) SendText(guid, text string, submit bool) error { return nil }
func (m *spawnFakeMux) SendKey(guid, key string) error                { return nil }
func (m *spawnFakeMux) CapturePane(guid string) (string, error)       { return "", nil }

var _ shuttleengine.MuxOps = (*spawnFakeMux)(nil)

// prepareCall records one spawnFakeEngine.Prepare invocation: the run
// directory it was handed and the exact Spec it received (already
// path-resolved and defaulted by Spec.validate, since Prepare runs after
// that), the source spawn_test.go's spec-mapping tests inspect.
type prepareCall struct {
	RunDir string
	Spec   shuttleengine.Spec
}

// spawnFakeEngine is a hermetic shuttleengine.Engine double: Prepare records
// every call and returns a canned Launch without writing any real provider
// artifacts; every other method returns a fixed, inspectable value, since
// SpawnBatch's own path through Runner.Start never reaches
// Interrupt/Send/Startup machinery (that lives in poll's classification,
// out of this batch's scope).
type spawnFakeEngine struct {
	mu           sync.Mutex
	PrepareCalls []prepareCall
}

func (e *spawnFakeEngine) Prepare(runDir string, spec shuttleengine.Spec, cfg shuttleengine.Config) (shuttleengine.Launch, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.PrepareCalls = append(e.PrepareCalls, prepareCall{RunDir: runDir, Spec: spec})
	return shuttleengine.Launch{Cmd: "fake-launch-cmd", SessionID: "fake-session"}, nil
}

func (e *spawnFakeEngine) ParseEvents(data []byte) ([]shuttleengine.Event, error) { return nil, nil }
func (e *spawnFakeEngine) Startup(capture string) shuttleengine.StartupState {
	return shuttleengine.StartupReady
}
func (e *spawnFakeEngine) InterruptSequence() []shuttleengine.PaneInput    { return nil }
func (e *spawnFakeEngine) TrustDismissSequence() []shuttleengine.PaneInput { return nil }
func (e *spawnFakeEngine) ComposeSend(text string) []shuttleengine.PaneInput {
	return nil
}

var _ shuttleengine.Engine = (*spawnFakeEngine)(nil)

// spawnFixture is a fully-wired, mutation-safe-to-share-across-subtests set
// of SpawnBatch dependencies: a real scratch git repo (one committed base
// file) as WorktreeRoot, fresh builder/reports temp dirs, a real
// *shuttleengine.Runner over spawnFakeMux/spawnFakeEngine, and every one of
// builderengine's four roles pre-resolved with distinct
// Model/Effort/Version values so a spec-mapping test can tell them apart.
type spawnFixture struct {
	Deps       builderengine.SpawnDeps
	Engine     *spawnFakeEngine
	Mux        *spawnFakeMux
	Worktree   string
	ReportsDir string
}

// newSpawnFixture builds a fresh spawnFixture: a new scratch git repo, new
// temp builder/reports dirs, and a fresh *shuttleengine.Runner, so tests
// that spawn more than once in sequence (e.g. the chain-anchor test) share
// one fixture deliberately, while table-driven subtests that must not leak
// Starter-call side effects across cases each get their own.
func newSpawnFixture(t *testing.T) *spawnFixture {
	t.Helper()

	dir := filepath.Join("testdata", "plan-valid")
	plan, err := builderengine.ParsePlan(dir)
	if err != nil {
		t.Fatalf("ParsePlan(%q) error = %v; want nil", dir, err)
	}

	// SpawnBatch refuses when the recorded fingerprint no longer matches the
	// on-disk plan, so the fixture's State must record the real fingerprint —
	// the same value Run would have recorded at first init.
	fingerprint, err := builderengine.Fingerprint(dir)
	if err != nil {
		t.Fatalf("Fingerprint(%q) error = %v; want nil", dir, err)
	}

	worktree := newScratchRepo(t)
	commitFile(t, worktree, "base.txt", "base", "base commit")

	builderDir := t.TempDir()
	reportsDir := t.TempDir()
	runRoot := t.TempDir()

	mux := &spawnFakeMux{}
	engine := &spawnFakeEngine{}
	layout := &hubgeometry.Layout{WorktreeRoot: worktree, Cwd: worktree}
	shuttleCfg := shuttleengine.Config{RunDir: runRoot, RunTimeoutMin: 60, StartupTimeoutS: 30}
	runner := shuttleengine.NewRunner(mux, engine, layout, shuttleCfg)

	roles := map[builderengine.Role]modelspec.Resolved{
		builderengine.RoleOrchestrator: {
			Engine: "claude", Model: "orchestrator-model", Params: map[string]string{},
		},
		builderengine.RoleImplementer: {
			Engine: "claude", Model: "implementer-model",
			Params: map[string]string{"effort": "medium", "version": "v1"},
		},
		builderengine.RoleImplementerOversized: {
			Engine: "claude", Model: "implementer-oversized-model",
			Params: map[string]string{"effort": "high"},
		},
		builderengine.RoleRecovery: {
			Engine: "claude", Model: "recovery-model",
			Params: map[string]string{"effort": "high", "version": "v2"},
		},
	}

	cfg := builderengine.Config{
		SelfFixCap:      2,
		BatchTimeoutMin: 45,
	}

	deps := builderengine.SpawnDeps{
		Starter:      runner,
		Plan:         plan,
		State:        &builderengine.State{PlanFingerprint: fingerprint},
		Roles:        roles,
		Config:       cfg,
		WorktreeRoot: worktree,
		BuilderDir:   builderDir,
		ReportsDir:   reportsDir,
		ShuttleCfg:   shuttleCfg,
		Layout:       layout,
		Mux:          mux,
	}

	return &spawnFixture{Deps: deps, Engine: engine, Mux: mux, Worktree: worktree, ReportsDir: reportsDir}
}

// TestSpawnBatch_RoleSelectionMatrix proves the discussion's role-selection
// decision mechanically: a plain batch spawns implementer, an oversized
// batch spawns implementer_oversized, a recovery override always wins
// regardless of oversized, and any other override is rejected before the
// Starter is ever reached.
func TestSpawnBatch_RoleSelectionMatrix(t *testing.T) {
	tests := []struct {
		name        string
		batchNumber int
		override    builderengine.Role
		wantRole    builderengine.Role
		wantErr     bool
	}{
		{name: "plain batch selects implementer", batchNumber: 1, wantRole: builderengine.RoleImplementer},
		{name: "oversized batch selects implementer_oversized", batchNumber: 5, wantRole: builderengine.RoleImplementerOversized},
		{name: "recovery override always wins", batchNumber: 1, override: builderengine.RoleRecovery, wantRole: builderengine.RoleRecovery},
		{name: "invalid override is rejected", batchNumber: 1, override: builderengine.Role("bogus"), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fx := newSpawnFixture(t)

			result, err := builderengine.SpawnBatch(fx.Deps, builderengine.SpawnBatchOptions{
				BatchNumber:  tt.batchNumber,
				RoleOverride: tt.override,
			})

			if tt.wantErr {
				if err == nil {
					t.Fatalf("SpawnBatch() error = nil; want an error for override %q", tt.override)
				}
				if len(fx.Engine.PrepareCalls) != 0 {
					t.Errorf("Starter was reached (%d Prepare calls) for a rejected override; want zero", len(fx.Engine.PrepareCalls))
				}
				return
			}

			if err != nil {
				t.Fatalf("SpawnBatch() error = %v; want nil", err)
			}
			if result.Role != tt.wantRole {
				t.Errorf("SpawnResult.Role = %q; want %q", result.Role, tt.wantRole)
			}

			bs, ok := fx.Deps.State.Batches[tt.batchNumber]
			if !ok {
				t.Fatalf("State.Batches[%d] missing after spawn", tt.batchNumber)
			}
			if bs.Role != string(tt.wantRole) {
				t.Errorf("BatchState.Role = %q; want %q", bs.Role, tt.wantRole)
			}
		})
	}
}

// TestSpawnBatch_PauseSentinel proves the pause gate fires before anything
// else — including before the Starter is ever reached — and that the
// returned error satisfies errors.Is(err, builderengine.ErrPaused).
func TestSpawnBatch_PauseSentinel(t *testing.T) {
	fx := newSpawnFixture(t)

	if err := builderengine.RequestPause(fx.Deps.BuilderDir); err != nil {
		t.Fatalf("RequestPause() error = %v; want nil", err)
	}

	_, err := builderengine.SpawnBatch(fx.Deps, builderengine.SpawnBatchOptions{BatchNumber: 1})
	if !errors.Is(err, builderengine.ErrPaused) {
		t.Fatalf("SpawnBatch() error = %v; want errors.Is(err, ErrPaused)", err)
	}
	if len(fx.Engine.PrepareCalls) != 0 {
		t.Errorf("Starter was reached (%d Prepare calls) while paused; want zero", len(fx.Engine.PrepareCalls))
	}
}

// TestSpawnBatch_StaleReportRefusal proves a pre-existing batch-report file
// is refused, as builder's own named error, before the Starter is ever
// reached — the discussion's "surface it as builder's own named error
// first" decision.
func TestSpawnBatch_StaleReportRefusal(t *testing.T) {
	fx := newSpawnFixture(t)

	stalePath := filepath.Join(fx.ReportsDir, "01-json-flag.yaml")
	if err := os.WriteFile(stalePath, []byte("batch: 01-json-flag\nstatus: done\ntests: green\n"), 0o644); err != nil {
		t.Fatalf("seed stale report: %v", err)
	}

	_, err := builderengine.SpawnBatch(fx.Deps, builderengine.SpawnBatchOptions{BatchNumber: 1})
	if err == nil {
		t.Fatalf("SpawnBatch() error = nil; want an error for a pre-existing report")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("SpawnBatch() error = %q; want it to mention the pre-existing report", err.Error())
	}
	if len(fx.Engine.PrepareCalls) != 0 {
		t.Errorf("Starter was reached (%d Prepare calls) for a stale report; want zero", len(fx.Engine.PrepareCalls))
	}
}

// TestSpawnBatch_RecoveryArchivesStaleReport proves a --role recovery respawn
// of a stuck batch archives the pre-existing report (rather than being refused
// by the pre-existing-report guard) and reaches the Starter — the exact
// stuck -> recovery escalation the orchestrator drives, which is unreachable if
// the stale report is not cleared first (shuttle's own Spec.validate refuses a
// pre-existing OutputFiles entry too). The stale report is archived, never
// deleted, so the prior stuck judgment stays auditable.
func TestSpawnBatch_RecoveryArchivesStaleReport(t *testing.T) {
	fx := newSpawnFixture(t)

	// The state a stuck non-chain batch leaves behind: its report is on disk
	// (poll classified it stuck and weft-committed it), and CurrentBatch has
	// reset. Batch 1 is a plain, chainless batch, so recovery is --role
	// recovery, never --restart-chain.
	stalePath := filepath.Join(fx.ReportsDir, "01-json-flag.yaml")
	if err := os.WriteFile(stalePath, []byte("batch: 01-json-flag\nstatus: stuck\ntests: red\nstuck_reason: \"blocked\"\n"), 0o644); err != nil {
		t.Fatalf("seed stale report: %v", err)
	}

	result, err := builderengine.SpawnBatch(fx.Deps, builderengine.SpawnBatchOptions{
		BatchNumber:  1,
		RoleOverride: builderengine.RoleRecovery,
	})
	if err != nil {
		t.Fatalf("SpawnBatch(--role recovery) with a pre-existing stuck report error = %v; want nil", err)
	}
	if result.Role != builderengine.RoleRecovery {
		t.Errorf("SpawnResult.Role = %q; want %q", result.Role, builderengine.RoleRecovery)
	}
	if len(fx.Engine.PrepareCalls) != 1 {
		t.Errorf("Engine.PrepareCalls = %d; want exactly 1 (the recovery spawn was reached)", len(fx.Engine.PrepareCalls))
	}

	// The live report path is now free (the recovery session's own fresh
	// report will land there), and the prior report survives under an
	// archived name rather than having been deleted.
	if _, statErr := os.Stat(stalePath); !os.IsNotExist(statErr) {
		t.Errorf("stat(%s) after recovery spawn = %v; want the live report path to be freed (archived away)", stalePath, statErr)
	}
	archived, err := filepath.Glob(filepath.Join(fx.ReportsDir, "01-json-flag-*.yaml"))
	if err != nil {
		t.Fatalf("glob archived reports: %v", err)
	}
	if len(archived) != 1 {
		t.Fatalf("archived report count = %d (%v); want exactly 1", len(archived), archived)
	}
	data, err := os.ReadFile(archived[0])
	if err != nil {
		t.Fatalf("read archived report %s: %v", archived[0], err)
	}
	if !strings.Contains(string(data), "status: stuck") {
		t.Errorf("archived report content = %q; want the prior stuck report preserved verbatim", string(data))
	}
}

// TestSpawnBatch_FingerprintMismatchRefused proves a plan edited after run
// init is refused at spawn-batch entry — before the Starter is ever
// reached — with the same ErrFingerprintMismatch sentinel Run uses, so a
// mid-run plan mutation can never be driven against the stale state.json
// (found live in round fable-r2: a mutated plan spawned silently).
func TestSpawnBatch_FingerprintMismatchRefused(t *testing.T) {
	fx := newSpawnFixture(t)
	fx.Deps.State.PlanFingerprint = "0000000000000000000000000000000000000000000000000000000000000000"

	_, err := builderengine.SpawnBatch(fx.Deps, builderengine.SpawnBatchOptions{BatchNumber: 1})
	if !errors.Is(err, builderengine.ErrFingerprintMismatch) {
		t.Fatalf("SpawnBatch() error = %v; want errors.Is(err, ErrFingerprintMismatch)", err)
	}
	if !strings.Contains(err.Error(), "--fresh") {
		t.Errorf("SpawnBatch() error = %q; want it to point at run --fresh", err.Error())
	}
	if len(fx.Engine.PrepareCalls) != 0 {
		t.Errorf("Starter was reached (%d Prepare calls) on a fingerprint mismatch; want zero", len(fx.Engine.PrepareCalls))
	}
}

// TestSpawnBatch_InFlightGuardMatrix proves the ErrBatchInFlight guard
// refuses a spawn exactly when a recorded non-terminal batch's strand is
// still live, and never on the intended respawn ladders (terminal batch,
// dead strand, no cursor) nor when the mux status query itself fails (a
// downed mux hosts no live strand; Start surfaces real substrate errors).
func TestSpawnBatch_InFlightGuardMatrix(t *testing.T) {
	tests := []struct {
		name         string
		currentBatch int
		terminal     bool
		strandLive   bool
		statusErr    error
		wantRefused  bool
	}{
		{name: "non-terminal live strand refuses", currentBatch: 2, strandLive: true, wantRefused: true},
		{name: "non-terminal dead strand proceeds", currentBatch: 2, strandLive: false},
		{name: "terminal batch proceeds (respawn ladder)", currentBatch: 2, terminal: true, strandLive: true},
		{name: "no cursor proceeds", currentBatch: 0, strandLive: true},
		{name: "mux status error proceeds", currentBatch: 2, strandLive: true, statusErr: errors.New("no mux session")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fx := newSpawnFixture(t)
			fx.Mux.statusErr = tt.statusErr
			if tt.strandLive {
				fx.Mux.status = muxengine.StatusResult{Strands: []muxengine.StrandStatus{{GUID: "in-flight-strand", Live: true}}}
			}
			if tt.currentBatch != 0 {
				fx.Deps.State.CurrentBatch = tt.currentBatch
				fx.Deps.State.Batches = map[int]*builderengine.BatchState{
					tt.currentBatch: {Slug: "list-tests", StrandGUID: "in-flight-strand", Terminal: tt.terminal},
				}
			}

			_, err := builderengine.SpawnBatch(fx.Deps, builderengine.SpawnBatchOptions{BatchNumber: 1})

			if tt.wantRefused {
				if !errors.Is(err, builderengine.ErrBatchInFlight) {
					t.Fatalf("SpawnBatch() error = %v; want errors.Is(err, ErrBatchInFlight)", err)
				}
				if len(fx.Engine.PrepareCalls) != 0 {
					t.Errorf("Starter was reached (%d Prepare calls) despite a live in-flight batch; want zero", len(fx.Engine.PrepareCalls))
				}
				return
			}
			if err != nil {
				t.Fatalf("SpawnBatch() error = %v; want nil", err)
			}
			if len(fx.Engine.PrepareCalls) != 1 {
				t.Errorf("Engine.PrepareCalls = %d; want exactly 1", len(fx.Engine.PrepareCalls))
			}
		})
	}
}

// TestSpawnBatch_ChainAnchorRecordedOnce proves the chain-start SHA is
// recorded at whichever chain member spawns first and never overwritten by
// a later member's own spawn, per the discussion's chain-anchor decision.
func TestSpawnBatch_ChainAnchorRecordedOnce(t *testing.T) {
	fx := newSpawnFixture(t)

	if _, err := builderengine.SpawnBatch(fx.Deps, builderengine.SpawnBatchOptions{BatchNumber: 3}); err != nil {
		t.Fatalf("SpawnBatch(batch 3) error = %v; want nil", err)
	}
	anchor, ok := fx.Deps.State.ChainStartSHAs[4]
	if !ok || anchor == "" {
		t.Fatalf("ChainStartSHAs[4] not recorded after spawning chain member batch 3")
	}

	// Advance the host repo's HEAD before spawning the chain's other member,
	// so a wrongly-overwritten anchor would visibly differ from the first
	// one recorded above.
	commitFile(t, fx.Worktree, "extra.txt", "extra", "extra commit")

	if _, err := builderengine.SpawnBatch(fx.Deps, builderengine.SpawnBatchOptions{BatchNumber: 4}); err != nil {
		t.Fatalf("SpawnBatch(batch 4) error = %v; want nil", err)
	}
	if got := fx.Deps.State.ChainStartSHAs[4]; got != anchor {
		t.Errorf("ChainStartSHAs[4] = %q after spawning batch 4; want unchanged anchor %q", got, anchor)
	}
}

// TestSpawnBatch_StatePersisted proves a successful spawn's BatchState and
// CurrentBatch survive a fresh LoadState round-trip from disk, not merely
// the in-memory deps.State the caller already holds.
func TestSpawnBatch_StatePersisted(t *testing.T) {
	fx := newSpawnFixture(t)

	result, err := builderengine.SpawnBatch(fx.Deps, builderengine.SpawnBatchOptions{BatchNumber: 1})
	if err != nil {
		t.Fatalf("SpawnBatch() error = %v; want nil", err)
	}

	loaded, err := builderengine.LoadState(fx.Deps.BuilderDir)
	if err != nil {
		t.Fatalf("LoadState() error = %v; want nil", err)
	}
	if loaded == nil {
		t.Fatal("LoadState() = nil; want the state SpawnBatch just saved")
	}
	if loaded.CurrentBatch != 1 {
		t.Errorf("loaded.CurrentBatch = %d; want 1", loaded.CurrentBatch)
	}
	bs, ok := loaded.Batches[1]
	if !ok {
		t.Fatal("loaded.Batches[1] missing after LoadState")
	}
	if bs.StartSHA != result.StartSHA {
		t.Errorf("loaded.Batches[1].StartSHA = %q; want %q", bs.StartSHA, result.StartSHA)
	}
	if bs.StrandGUID != result.StrandGUID {
		t.Errorf("loaded.Batches[1].StrandGUID = %q; want %q", bs.StrandGUID, result.StrandGUID)
	}
	if bs.Role != string(builderengine.RoleImplementer) {
		t.Errorf("loaded.Batches[1].Role = %q; want %q", bs.Role, builderengine.RoleImplementer)
	}
}

// TestSpawnBatch_SpecFieldsMapped proves the shuttleengine.Spec built for
// the spawn matches modelspec's documented consumer mapping and the
// discussion's remaining Spec fields exactly.
func TestSpawnBatch_SpecFieldsMapped(t *testing.T) {
	fx := newSpawnFixture(t)

	result, err := builderengine.SpawnBatch(fx.Deps, builderengine.SpawnBatchOptions{BatchNumber: 1})
	if err != nil {
		t.Fatalf("SpawnBatch() error = %v; want nil", err)
	}

	if len(fx.Engine.PrepareCalls) != 1 {
		t.Fatalf("Engine.PrepareCalls = %d; want exactly 1", len(fx.Engine.PrepareCalls))
	}
	spec := fx.Engine.PrepareCalls[0].Spec

	wantResolved := fx.Deps.Roles[builderengine.RoleImplementer]
	if spec.Model != wantResolved.Model {
		t.Errorf("spec.Model = %q; want %q", spec.Model, wantResolved.Model)
	}
	if spec.Effort != wantResolved.Params["effort"] {
		t.Errorf("spec.Effort = %q; want %q", spec.Effort, wantResolved.Params["effort"])
	}
	if spec.Version != wantResolved.Params["version"] {
		t.Errorf("spec.Version = %q; want %q", spec.Version, wantResolved.Params["version"])
	}
	if spec.Role != string(builderengine.RoleImplementer) {
		t.Errorf("spec.Role = %q; want %q", spec.Role, builderengine.RoleImplementer)
	}
	if spec.Round != "01-json-flag" {
		t.Errorf("spec.Round = %q; want %q", spec.Round, "01-json-flag")
	}
	if spec.Timeout != 45*time.Minute {
		t.Errorf("spec.Timeout = %v; want %v", spec.Timeout, 45*time.Minute)
	}
	if !spec.KeepPane {
		t.Errorf("spec.KeepPane = false; want true")
	}
	if len(spec.OutputFiles) != 1 || spec.OutputFiles[0] != result.ReportPath {
		t.Errorf("spec.OutputFiles = %v; want [%q]", spec.OutputFiles, result.ReportPath)
	}
	if strings.TrimSpace(spec.Prompt) == "" {
		t.Errorf("spec.Prompt is empty; want the filled implementer template")
	}
	if !strings.Contains(spec.Prompt, result.ReportPath) {
		t.Errorf("spec.Prompt does not mention the report path %q", result.ReportPath)
	}
}

// TestSpawnBatch_RestartChainOnChainlessBatchErrors proves --restart-chain
// against a chainless batch is refused before the Starter is ever reached
// and before any HeadSHA capture — the discussion's "error if the batch is
// chainless" requirement.
func TestSpawnBatch_RestartChainOnChainlessBatchErrors(t *testing.T) {
	fx := newSpawnFixture(t)

	_, err := builderengine.SpawnBatch(fx.Deps, builderengine.SpawnBatchOptions{BatchNumber: 1, RestartChain: true})
	if err == nil {
		t.Fatalf("SpawnBatch(--restart-chain) on a chainless batch error = nil; want an error")
	}
	if len(fx.Engine.PrepareCalls) != 0 {
		t.Errorf("Starter was reached (%d Prepare calls) for a chainless --restart-chain; want zero", len(fx.Engine.PrepareCalls))
	}
}

// TestSpawnBatch_RestartChainClearsStaleReportBeforeRefusal proves
// --restart-chain's own reset reaches and deletes a chain member's stale
// report BEFORE SpawnBatch's pre-existing-report check ever runs — the exact
// real-world invocation ("re-spawn the batch whose stale report is still on
// disk") --restart-chain exists to recover. Reviewing this any other
// ordering (stale-report check before the reset) makes --restart-chain
// unreachable on every real call, since the report the caller is trying to
// clear is the same one that would trip the check first.
func TestSpawnBatch_RestartChainClearsStaleReportBeforeRefusal(t *testing.T) {
	fx := newSpawnFixture(t)

	// First spawn records the chain's start-SHA anchor, mirroring the real
	// sequence: a chain member must spawn once before --restart-chain has
	// any recorded anchor to reset to.
	if _, err := builderengine.SpawnBatch(fx.Deps, builderengine.SpawnBatchOptions{BatchNumber: 3}); err != nil {
		t.Fatalf("SpawnBatch(batch 3) error = %v; want nil", err)
	}

	// Simulate the implementer having written its batch report and gone
	// stuck: the report is now present on disk, exactly the state a
	// stuck-chain-member recovery finds.
	stalePath := filepath.Join(fx.ReportsDir, "03-refactor-a.yaml")
	if err := os.WriteFile(stalePath, []byte("batch: 03-refactor-a\nstatus: stuck\n"), 0o644); err != nil {
		t.Fatalf("seed stale report: %v", err)
	}

	result, err := builderengine.SpawnBatch(fx.Deps, builderengine.SpawnBatchOptions{BatchNumber: 3, RestartChain: true})
	if err != nil {
		t.Fatalf("SpawnBatch(--restart-chain) with a pre-existing report error = %v; want nil", err)
	}
	if result.BatchName != "03-refactor-a" {
		t.Errorf("SpawnResult.BatchName = %q; want %q", result.BatchName, "03-refactor-a")
	}
	if len(fx.Engine.PrepareCalls) != 2 {
		t.Errorf("Engine.PrepareCalls = %d; want exactly 2 (one per spawn)", len(fx.Engine.PrepareCalls))
	}
}
