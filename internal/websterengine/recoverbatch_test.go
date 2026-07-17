//go:build integration

// recoverbatch_test.go exercises RecoverBatch end to end (Tier 2 — see
// docs/benchmarks/running-tests.md): a real scratch git repo backs
// WorktreeRoot for the genuine HeadSHA/ChangedFiles/Dirty calls, a real
// *shuttleengine.Runner wired over local fake shuttleengine.MuxOps/
// shuttleengine.Engine doubles is the Starter (builderengine's own
// established fake-starter approach — spawn_test.go's spawnFixture
// pattern), and a fake Clock replays the whole bounded-wait sequence with
// no real sleeps, mirroring builderengine/poll_test.go's fakeClock. The
// re-entrancy contract (spawn-once, attach-thereafter, elapsed-across-
// calls) is this file's test centre, per the batch's own "Batch Tests"
// note. This package's testmain_test.go already wires
// lyxtest.HermeticGitEnv() for the whole test binary.

package websterengine_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/builderengine"
	"github.com/Knatte18/loomyard/internal/hubgeometry"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/muxengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// recoverFakeMux is a hermetic shuttleengine.MuxOps double: AddStrand mints
// a distinct GUID per call and registers it live in the scripted Status
// result (a spawned strand is live until explicitly removed or the test
// overrides Status directly), RemoveStrand records every call and retires
// the guid from Status, and the send/capture methods stay inert since
// RecoverBatch's own path never exercises them.
type recoverFakeMux struct {
	mu             sync.Mutex
	counter        int
	status         muxengine.StatusResult
	statusErr      error
	removedStrands []string
}

func (m *recoverFakeMux) AddStrand(spec muxengine.AddSpec) (muxengine.Strand, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.counter++
	guid := fmt.Sprintf("recover-test-strand-%d", m.counter)
	m.status.Strands = append(m.status.Strands, muxengine.StrandStatus{GUID: guid, Live: true})
	return muxengine.Strand{GUID: guid}, nil
}

func (m *recoverFakeMux) RemoveStrand(guid string, recursive bool) (muxengine.Removed, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.removedStrands = append(m.removedStrands, guid)
	for i, s := range m.status.Strands {
		if s.GUID == guid {
			m.status.Strands = append(m.status.Strands[:i], m.status.Strands[i+1:]...)
			break
		}
	}
	return muxengine.Removed{}, nil
}

func (m *recoverFakeMux) Status() (muxengine.StatusResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.statusErr != nil {
		return muxengine.StatusResult{}, m.statusErr
	}
	return m.status, nil
}

func (m *recoverFakeMux) SendText(guid, text string, submit bool) error { return nil }
func (m *recoverFakeMux) SendKey(guid, key string) error                { return nil }
func (m *recoverFakeMux) CapturePane(guid string) (string, error)       { return "", nil }

var _ shuttleengine.MuxOps = (*recoverFakeMux)(nil)

// recoverFakeEngine is a hermetic shuttleengine.Engine double: Prepare
// counts every call (so a test can prove an ATTACH call never re-spawns)
// without writing any real provider artifacts; ParseEvents is scripted per
// test (a canned Events slice, defaulting to none — no Stop event, i.e.
// TurnEnded reports false) since it is the only method RecoverBatch's own
// TurnEnded call reaches. Every other method returns a fixed, inert value.
type recoverFakeEngine struct {
	mu           sync.Mutex
	prepareCalls int
	events       []shuttleengine.Event
	eventsErr    error
}

func (e *recoverFakeEngine) Prepare(runDir string, spec shuttleengine.Spec, cfg shuttleengine.Config) (shuttleengine.Launch, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.prepareCalls++
	return shuttleengine.Launch{Cmd: "fake-launch-cmd", SessionID: "fake-session"}, nil
}

func (e *recoverFakeEngine) ParseEvents(data []byte) ([]shuttleengine.Event, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.eventsErr != nil {
		return nil, e.eventsErr
	}
	return e.events, nil
}

func (e *recoverFakeEngine) Startup(capture string) shuttleengine.StartupState {
	return shuttleengine.StartupReady
}
func (e *recoverFakeEngine) InterruptSequence() []shuttleengine.PaneInput    { return nil }
func (e *recoverFakeEngine) TrustDismissSequence() []shuttleengine.PaneInput { return nil }
func (e *recoverFakeEngine) ComposeSend(text string) []shuttleengine.PaneInput {
	return nil
}
func (e *recoverFakeEngine) AuditForks(sessionID, workdir string) (shuttleengine.ForkAudit, error) {
	return shuttleengine.ForkAudit{}, nil
}
func (e *recoverFakeEngine) AuditForksIncremental(sessionID, workdir string, seenTranscripts map[string]bool) (shuttleengine.ForkAudit, error) {
	return shuttleengine.ForkAudit{}, nil
}
func (e *recoverFakeEngine) ModelSwitchSequence(model string) []shuttleengine.PaneInput {
	return nil
}

var _ shuttleengine.Engine = (*recoverFakeEngine)(nil)

// prepareCallCount reports how many times e.Prepare has been called so far.
func (e *recoverFakeEngine) prepareCallCount() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.prepareCalls
}

// recoverFakeClock is a package-local, scriptable clock double: Now starts
// at a fixed base and only advances when Sleep is called or a test directly
// mutates Now, so a test controls exactly how much virtual time elapses —
// including simulating the wall-clock gap BETWEEN two separate
// RecoverBatch calls, which a real process boundary would otherwise supply
// for free — without ever blocking for real.
type recoverFakeClock struct {
	now time.Time
}

func (c *recoverFakeClock) Now() time.Time        { return c.now }
func (c *recoverFakeClock) Sleep(d time.Duration) { c.now = c.now.Add(d) }

var _ websterengine.Clock = (*recoverFakeClock)(nil)

// recoverFixture is a fully-wired set of RecoverBatch dependencies: a real
// scratch git repo (one base commit) as WorktreeRoot, a literal one-batch
// plan backed by a seeded plan dir, a real *shuttleengine.Runner over
// recoverFakeMux/recoverFakeEngine as the Starter, and webster's three
// roles pre-resolved.
type recoverFixture struct {
	Deps       websterengine.RecoverDeps
	Mux        *recoverFakeMux
	Engine     *recoverFakeEngine
	Worktree   string
	ReportsDir string
}

func newRecoverFixture(t *testing.T) *recoverFixture {
	t.Helper()

	planDir := seedPlanDir(t)
	plan := &builderengine.Plan{
		Dir: planDir,
		Batches: []builderengine.PlanBatch{
			{Number: 1, Slug: "json-flag", File: "01-json-flag.md", Scope: []string{"internal/foo"}},
		},
	}

	worktree := newScratchRepo(t)
	commitFile(t, worktree, "base.txt", "base", "base commit")

	mux := &recoverFakeMux{}
	engine := &recoverFakeEngine{}
	layout := &hubgeometry.Layout{WorktreeRoot: worktree, Cwd: worktree}
	shuttleCfg := shuttleengine.Config{RunDir: t.TempDir(), RunTimeoutMin: 60, StartupTimeoutS: 30}
	runner := shuttleengine.NewRunner(mux, engine, layout, shuttleCfg)

	roles := map[websterengine.Role]modelspec.Resolved{
		websterengine.RoleMaster:          {Engine: "claude", Model: "master-model", Params: map[string]string{}},
		websterengine.RoleMasterOversized: {Engine: "claude", Model: "oversized-model", Params: map[string]string{}},
		websterengine.RoleRecovery:        {Engine: "claude", Model: "recovery-model", Params: map[string]string{"effort": "high"}},
	}

	reportsDir := t.TempDir()

	deps := websterengine.RecoverDeps{
		Starter:      runner,
		Plan:         plan,
		State:        &websterengine.State{Batches: map[int]*websterengine.BatchState{}},
		Roles:        roles,
		Config:       websterengine.Config{SelfFixCap: 2, RecoveryTimeoutMin: 30},
		Engine:       engine,
		Mux:          mux,
		ShuttleCfg:   shuttleCfg,
		Layout:       layout,
		WorktreeRoot: worktree,
		WebsterDir:   t.TempDir(),
		ReportsDir:   reportsDir,
	}

	return &recoverFixture{Deps: deps, Mux: mux, Engine: engine, Worktree: worktree, ReportsDir: reportsDir}
}

// writeRecoverReport seeds fx's reportsDir with a batch-report YAML file for
// batch 1 at its plan-format-pinned filename.
func writeRecoverReport(t *testing.T, reportsDir, content string) {
	t.Helper()
	path := filepath.Join(reportsDir, builderengine.BatchReportFileName(1, "json-flag"))
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write batch report: %v", err)
	}
}

// TestRecoverBatch_FirstCallSpawnsArchivesStaleReportAndStopsLiveStrand
// proves the first call for a batch with no live recovery record spawns a
// fresh recovery strand: a stale report at the batch's own report path is
// archived (renamed with a timestamp suffix, never deleted), a prior
// recorded strand still reported live by the mux is stopped, the fresh
// BatchState's strand fields are recorded, and — with no report landing
// inside the wait window — the call returns Running with Spawned: true.
func TestRecoverBatch_FirstCallSpawnsArchivesStaleReportAndStopsLiveStrand(t *testing.T) {
	fx := newRecoverFixture(t)

	stalePath := filepath.Join(fx.ReportsDir, "01-json-flag.yaml")
	if err := os.WriteFile(stalePath, []byte("batch: 01-json-flag\nstatus: stuck\ntests: red\nstuck_reason: \"blocked\"\n"), 0o644); err != nil {
		t.Fatalf("seed stale report: %v", err)
	}

	fx.Deps.State.Batches[1] = &websterengine.BatchState{
		Slug: "json-flag", Kind: "recovery", Terminal: true, Status: "dead", StrandGUID: "orphan-1",
	}
	fx.Mux.status = muxengine.StatusResult{Strands: []muxengine.StrandStatus{{GUID: "orphan-1", Live: true}}}

	clk := &recoverFakeClock{now: time.Unix(0, 0)}
	result, err := websterengine.RecoverBatch(fx.Deps, 1, 3*time.Second, clk)
	if err != nil {
		t.Fatalf("RecoverBatch() error = %v; want nil", err)
	}
	if !result.Spawned {
		t.Error("RecoverResult.Spawned = false; want true (first call spawns)")
	}
	if !result.Running {
		t.Errorf("RecoverResult.Running = false; want true (no report landed inside the wait window)")
	}
	if result.Digest != nil {
		t.Errorf("RecoverResult.Digest = %+v; want nil for a running result", result.Digest)
	}

	// The stale report was archived (renamed with a timestamp suffix), never
	// deleted, and the live path is free for the fresh recovery's own report.
	if _, statErr := os.Stat(stalePath); !os.IsNotExist(statErr) {
		t.Errorf("stat(%s) = %v; want the live report path freed (archived away)", stalePath, statErr)
	}
	archived, globErr := filepath.Glob(filepath.Join(fx.ReportsDir, "01-json-flag-*.yaml"))
	if globErr != nil || len(archived) != 1 {
		t.Fatalf("archived report glob = %v, %v; want exactly 1 archive", archived, globErr)
	}
	data, err := os.ReadFile(archived[0])
	if err != nil {
		t.Fatalf("read archived report %s: %v", archived[0], err)
	}
	if !strings.Contains(string(data), "status: stuck") {
		t.Errorf("archived report content = %q; want the prior report preserved verbatim", string(data))
	}

	// The prior orphan's live strand was stopped before the fresh spawn.
	found := false
	for _, guid := range fx.Mux.removedStrands {
		if guid == "orphan-1" {
			found = true
		}
	}
	if !found {
		t.Errorf("RemoveStrand calls = %v; want the prior live strand %q stopped", fx.Mux.removedStrands, "orphan-1")
	}

	// The fresh BatchState's strand fields are recorded.
	bs := fx.Deps.State.Batches[1]
	if bs.Kind != "recovery" {
		t.Errorf("BatchState.Kind = %q; want %q", bs.Kind, "recovery")
	}
	if bs.Terminal {
		t.Error("BatchState.Terminal = true after a running result; want false")
	}
	if bs.StrandGUID == "" || bs.StrandGUID == "orphan-1" {
		t.Errorf("BatchState.StrandGUID = %q; want a freshly minted guid distinct from the stopped orphan", bs.StrandGUID)
	}
	if bs.ShuttleRunDir == "" {
		t.Error("BatchState.ShuttleRunDir is empty; want the resolved run directory")
	}
	if bs.EventsPath == "" {
		t.Error("BatchState.EventsPath is empty; want the resolved events.jsonl path")
	}
	if _, parseErr := time.Parse(time.RFC3339, bs.SpawnedAt); parseErr != nil {
		t.Errorf("BatchState.SpawnedAt = %q: %v; want a valid RFC3339 timestamp", bs.SpawnedAt, parseErr)
	}
	wantHead, err := builderengine.HeadSHA(fx.Worktree)
	if err != nil {
		t.Fatalf("HeadSHA() error = %v", err)
	}
	if bs.StartSHA != wantHead {
		t.Errorf("BatchState.StartSHA = %q; want the fresh HeadSHA %q", bs.StartSHA, wantHead)
	}

	if fx.Engine.prepareCallCount() != 1 {
		t.Errorf("Engine.prepareCalls = %d; want exactly 1", fx.Engine.prepareCallCount())
	}
}

// TestRecoverBatch_SecondCallAttachesAndPersistsDoneDigest proves a
// re-entrant second call for the same batch, while the recovery strand is
// still recorded and non-terminal, ATTACHES rather than re-spawning (the
// fake Starter's Prepare call count stays at 1), and once the batch's own
// report has landed, returns the terminal digest with state persisted and
// the done-classified substrate released (strand removed, run dir removed).
func TestRecoverBatch_SecondCallAttachesAndPersistsDoneDigest(t *testing.T) {
	fx := newRecoverFixture(t)
	clk := &recoverFakeClock{now: time.Unix(0, 0)}

	// First call spawns; no report yet.
	first, err := websterengine.RecoverBatch(fx.Deps, 1, 2*time.Second, clk)
	if err != nil {
		t.Fatalf("RecoverBatch() first call error = %v; want nil", err)
	}
	if !first.Spawned || !first.Running {
		t.Fatalf("first call = %+v; want Spawned=true Running=true", first)
	}
	runDir := fx.Deps.State.Batches[1].ShuttleRunDir
	strandGUID := fx.Deps.State.Batches[1].StrandGUID

	// The re-fork's report has now landed.
	writeRecoverReport(t, fx.ReportsDir, "batch: 01-json-flag\nstatus: done\ntests: green\nstuck_reason: null\n")

	second, err := websterengine.RecoverBatch(fx.Deps, 1, 2*time.Second, clk)
	if err != nil {
		t.Fatalf("RecoverBatch() second call error = %v; want nil", err)
	}
	if second.Spawned {
		t.Error("second call Spawned = true; want false (ATTACH, not a re-spawn)")
	}
	if second.Running {
		t.Error("second call Running = true; want false (terminal once the report landed)")
	}
	if second.Digest == nil || second.Digest.Status != builderengine.DigestStatusDone {
		t.Fatalf("second call Digest = %+v; want a done digest", second.Digest)
	}
	if len(second.Warnings) != 0 {
		t.Errorf("second call Warnings = %v; want none", second.Warnings)
	}
	if fx.Engine.prepareCallCount() != 1 {
		t.Errorf("Engine.prepareCalls = %d; want exactly 1 (no second spawn on ATTACH)", fx.Engine.prepareCallCount())
	}

	bs := fx.Deps.State.Batches[1]
	if !bs.Terminal {
		t.Error("BatchState.Terminal = false; want true")
	}
	if bs.Status != builderengine.DigestStatusDone {
		t.Errorf("BatchState.Status = %q; want %q", bs.Status, builderengine.DigestStatusDone)
	}
	if bs.Digest == nil {
		t.Error("BatchState.Digest = nil; want the persisted digest")
	}
	if fx.Deps.State.CurrentBatch != 0 {
		t.Errorf("State.CurrentBatch = %d; want 0 (cleared)", fx.Deps.State.CurrentBatch)
	}

	// done-substrate release: strand removed, run dir removed.
	foundRemoved := false
	for _, guid := range fx.Mux.removedStrands {
		if guid == strandGUID {
			foundRemoved = true
		}
	}
	if !foundRemoved {
		t.Errorf("RemoveStrand calls = %v; want the done strand %q removed", fx.Mux.removedStrands, strandGUID)
	}
	if _, statErr := os.Stat(runDir); !os.IsNotExist(statErr) {
		t.Errorf("stat(%s) = %v; want the done run dir removed", runDir, statErr)
	}
}

// TestRecoverBatch_TimeoutAcrossCallsClassifiesDead proves
// RecoveryTimeoutMin is measured from the recorded SpawnedAt ACROSS
// re-entrant calls, not reset per call: virtual time advanced well past the
// configured timeout between two calls classifies dead/timeout on the
// second call even though neither call's own wait budget alone would ever
// cross it, and the dead classification keeps BOTH the strand and the run
// directory (diagnosis material), never removing either.
func TestRecoverBatch_TimeoutAcrossCallsClassifiesDead(t *testing.T) {
	fx := newRecoverFixture(t)
	fx.Deps.Config.RecoveryTimeoutMin = 1 // 1 minute
	clk := &recoverFakeClock{now: time.Unix(0, 0)}

	first, err := websterengine.RecoverBatch(fx.Deps, 1, 2*time.Second, clk)
	if err != nil {
		t.Fatalf("RecoverBatch() first call error = %v; want nil", err)
	}
	if !first.Running {
		t.Fatalf("first call = %+v; want Running=true", first)
	}
	runDir := fx.Deps.State.Batches[1].ShuttleRunDir
	strandGUID := fx.Deps.State.Batches[1].StrandGUID

	// Simulate the wall-clock gap between two separate CLI invocations: two
	// minutes pass, crossing the one-minute RecoveryTimeoutMin, with no
	// report ever landing.
	clk.now = clk.now.Add(2 * time.Minute)

	second, err := websterengine.RecoverBatch(fx.Deps, 1, 2*time.Second, clk)
	if err != nil {
		t.Fatalf("RecoverBatch() second call error = %v; want nil", err)
	}
	if second.Running {
		t.Error("second call Running = true; want false (terminal dead/timeout)")
	}
	if second.Digest == nil || second.Digest.Status != builderengine.DigestStatusDead {
		t.Fatalf("second call Digest = %+v; want a dead digest", second.Digest)
	}
	if second.Digest.DeadReason != builderengine.DeadReasonTimeout {
		t.Errorf("second call Digest.DeadReason = %q; want %q", second.Digest.DeadReason, builderengine.DeadReasonTimeout)
	}
	if second.ElapsedS < 120 {
		t.Errorf("second call ElapsedS = %d; want >= 120 (measured since the original spawn)", second.ElapsedS)
	}

	// dead classification keeps both the strand and the run dir.
	for _, guid := range fx.Mux.removedStrands {
		if guid == strandGUID {
			t.Errorf("RemoveStrand calls = %v; want the dead-classified strand %q kept", fx.Mux.removedStrands, strandGUID)
		}
	}
	if _, statErr := os.Stat(runDir); statErr != nil {
		t.Errorf("stat(%s) = %v; want the dead-classified run dir kept", runDir, statErr)
	}
}

// TestRecoverBatch_UnrecordedOrTerminalBatchSpawnsFresh proves the
// spawn-or-attach decision spawns fresh — never attaches — both when the
// batch has no recorded BatchState at all and when a prior recovery attempt
// already reached a terminal classification, in contrast to a recorded,
// non-terminal recovery BatchState, which attaches.
func TestRecoverBatch_UnrecordedOrTerminalBatchSpawnsFresh(t *testing.T) {
	tests := []struct {
		name   string
		prior  *websterengine.BatchState
		spawns bool
	}{
		{name: "no recorded BatchState spawns fresh", prior: nil, spawns: true},
		{name: "terminal prior recovery attempt spawns fresh", prior: &websterengine.BatchState{
			Slug: "json-flag", Kind: "recovery", Terminal: true, Status: "dead", StrandGUID: "prior-dead-1",
		}, spawns: true},
		{name: "non-terminal recorded recovery attaches", prior: &websterengine.BatchState{
			Slug: "json-flag", Kind: "recovery", Terminal: false, StrandGUID: "still-live-1",
			SpawnedAt: time.Unix(0, 0).UTC().Format(time.RFC3339),
		}, spawns: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fx := newRecoverFixture(t)
			if tt.prior != nil {
				fx.Deps.State.Batches[1] = tt.prior
				if tt.prior.StrandGUID != "" {
					fx.Mux.status = muxengine.StatusResult{Strands: []muxengine.StrandStatus{{GUID: tt.prior.StrandGUID, Live: true}}}
				}
			}

			clk := &recoverFakeClock{now: time.Unix(0, 0)}
			_, err := websterengine.RecoverBatch(fx.Deps, 1, 1*time.Second, clk)
			if err != nil {
				t.Fatalf("RecoverBatch() error = %v; want nil", err)
			}

			gotSpawn := fx.Engine.prepareCallCount() == 1
			if gotSpawn != tt.spawns {
				t.Errorf("Engine.prepareCalls = %d (spawned=%v); want spawned=%v", fx.Engine.prepareCallCount(), gotSpawn, tt.spawns)
			}
		})
	}
}
