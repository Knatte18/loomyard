//go:build integration

// recoverbatch_test.go exercises RecoverBatch end to end (Tier 2 — see
// docs/benchmarks/running-tests.md): a real scratch git repo backs
// WorktreeRoot for the genuine HeadSHA/ChangedFiles/Dirty calls, a real
// *shuttleengine.Runner wired over local fake shuttleengine.ReedOps/
// shuttleengine.Engine doubles is the Starter, webster's own
// established fake-starter approach, and a fake Clock replays the whole
// bounded-wait sequence with no real sleeps, webster's own fakeClock. The
// re-entrancy contract (spawn-once, attach-thereafter, elapsed-across-
// calls) is this file's test centre, per the batch's own "Batch Tests"
// note. This package's testmain_test.go already wires
// gitkit.HermeticGitEnv() for the whole test binary.

package websterengine_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/batcher"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// recoverFakeReed is a hermetic shuttleengine.ReedOps double: AddStrand mints
// a distinct GUID per call and registers it live in the scripted Status
// result (a spawned strand is live until explicitly removed or the test
// overrides Status directly), RemoveStrand records every call and retires
// the guid from Status, and the send/capture methods stay inert since
// RecoverBatch's own path never exercises them.
type recoverFakeReed struct {
	mu             sync.Mutex
	counter        int
	status         reedengine.StatusResult
	statusErr      error
	removedStrands []string
}

func (m *recoverFakeReed) AddStrand(spec reedengine.AddSpec) (reedengine.Strand, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.counter++
	guid := fmt.Sprintf("recover-test-strand-%d", m.counter)
	m.status.Strands = append(m.status.Strands, reedengine.StrandStatus{GUID: guid, Live: true})
	return reedengine.Strand{GUID: guid}, nil
}

func (m *recoverFakeReed) RemoveStrand(guid string, recursive bool) (reedengine.Removed, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.removedStrands = append(m.removedStrands, guid)
	for i, s := range m.status.Strands {
		if s.GUID == guid {
			m.status.Strands = append(m.status.Strands[:i], m.status.Strands[i+1:]...)
			break
		}
	}
	return reedengine.Removed{}, nil
}

func (m *recoverFakeReed) Status() (reedengine.StatusResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.statusErr != nil {
		return reedengine.StatusResult{}, m.statusErr
	}
	return m.status, nil
}

func (m *recoverFakeReed) SendText(guid, text string, submit bool) error { return nil }
func (m *recoverFakeReed) SendKey(guid, key string) error                { return nil }
func (m *recoverFakeReed) CapturePane(guid string) (string, error)       { return "", nil }

var _ shuttleengine.ReedOps = (*recoverFakeReed)(nil)

// recoverFakeEngine is a hermetic shuttleengine.Engine double: Prepare
// counts every call (so a test can prove an ATTACH call never re-spawns)
// without writing any real provider artifacts; ParseEvents is scripted per
// test (a canned Events slice, defaulting to none — no Stop event, i.e.
// TurnEnded reports false) since it is the only method RecoverBatch's own
// TurnEnded call reaches. Every other method returns a fixed, inert value.
type recoverFakeEngine struct {
	mu           sync.Mutex
	prepareCalls int
	lastPrompt   string
	events       []shuttleengine.Event
	eventsErr    error
}

func (e *recoverFakeEngine) Prepare(runDir string, spec shuttleengine.Spec, cfg shuttleengine.Config) (shuttleengine.Launch, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.prepareCalls++
	e.lastPrompt = spec.Prompt
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

// lastPromptText returns the Spec.Prompt text of the most recent Prepare call, the recovery
// prompt RecoverSpawnOrAttach rendered and handed to the (fake) provider — the only place this
// fixture ever sees that prompt's bytes, since the fake Prepare never writes prompt.md to disk.
func (e *recoverFakeEngine) lastPromptText() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.lastPrompt
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
// scratch git repo (one base commit) as WorktreeRoot, a one-batch plan backed
// by a seeded plan dir and its corresponding execution-batch list, a real
// *shuttleengine.Runner over recoverFakeReed/recoverFakeEngine as the Starter,
// and webster's two roles pre-resolved.
type recoverFixture struct {
	Deps       websterengine.RecoverDeps
	Reed       *recoverFakeReed
	Engine     *recoverFakeEngine
	Worktree   string
	ReportsDir string
}

func newRecoverFixture(t *testing.T) *recoverFixture {
	t.Helper()

	plan := &planparser.Plan{}
	batches := []batcher.Batch{
		{Cards: []planparser.Card{{Number: 1, Slug: "json-flag", Title: "json-flag", Intent: "add the --json flag"}}},
	}

	worktree := newScratchRepo(t)
	commitFile(t, worktree, "base.txt", "base", "base commit")

	reed := &recoverFakeReed{}
	engine := &recoverFakeEngine{}
	hubPath := filepath.Dir(worktree)
	// webster's prompts are read from disk at call time now, so the fixture's
	// hub must carry them before RecoverBatch reaches RenderRecoveryPrompt.
	seedHubStencils(t, hubPath)
	shuttleCfg := shuttleengine.Config{RunDir: t.TempDir(), RunTimeoutMin: 60, StartupTimeoutS: 30}
	runner := shuttleengine.NewRunner(reed, engine, worktree, worktree, shuttleCfg)

	roles := map[websterengine.Role]modelspec.Resolved{
		websterengine.RoleMaster:   {Engine: "claude", Model: "master-model", Params: map[string]string{}},
		websterengine.RoleRecovery: {Engine: "claude", Model: "recovery-model", Params: map[string]string{"effort": "high"}},
	}

	reportsDir := t.TempDir()

	deps := websterengine.RecoverDeps{
		Starter:    runner,
		Plan:       plan,
		Batches:    batches,
		State:      &websterengine.State{Batches: map[int]*websterengine.BatchState{}},
		Roles:      roles,
		Config:     websterengine.Config{SelfFixCap: 2, RecoveryTimeoutMin: 30},
		Engine:     engine,
		Reed:       reed,
		ShuttleCfg: shuttleCfg,
		Geom: websterengine.Geometry{
			AnchorRoot:   worktree,
			WorktreeRoot: worktree,
			WebsterDir:   t.TempDir(),
			ReportsDir:   reportsDir,
			StencilsDir:  fabricengine.StencilsDir(hubPath),
		},
	}

	return &recoverFixture{Deps: deps, Reed: reed, Engine: engine, Worktree: worktree, ReportsDir: reportsDir}
}

// writeRecoverReport seeds fx's reportsDir with a batch-report YAML file for
// batch 1 at its plan-format-pinned filename.
func writeRecoverReport(t *testing.T, reportsDir, content string) {
	t.Helper()
	path := filepath.Join(reportsDir, websterengine.ReportFileName(1, "json-flag"))
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write batch report: %v", err)
	}
}

// recoverDriveResult mirrors the composed per-call outcome the CLI verb
// assembles from the three lease-scoped phases, so each test keeps asserting
// one call's whole effect.
type recoverDriveResult struct {
	Digest   *websterengine.Digest
	Running  bool
	Spawned  bool
	ElapsedS int
	Warnings []string
}

// driveRecoverBatch composes RecoverSpawnOrAttach -> RecoverAwait ->
// PersistRecoveryTerminal against deps' in-memory state, exactly the
// sequence webstercli's recover-batch verb drives (minus the lease and
// SaveState/fabric steps, which the CLI owns) — so every re-entrancy and
// classification test below exercises the same composition production runs.
func driveRecoverBatch(deps websterengine.RecoverDeps, batchNumber int, wait time.Duration, clk websterengine.Clock) (*recoverDriveResult, error) {
	bs, spawned, err := websterengine.RecoverSpawnOrAttach(deps, batchNumber, clk)
	if err != nil {
		return nil, err
	}
	result, err := websterengine.RecoverAwait(deps, batchNumber, bs, wait, clk)
	if err != nil {
		return nil, err
	}
	if result.Digest != nil {
		if err := websterengine.PersistRecoveryTerminal(deps.State, batchNumber, result.Digest); err != nil {
			return nil, err
		}
	}
	return &recoverDriveResult{
		Digest:   result.Digest,
		Running:  result.Running,
		Spawned:  spawned,
		ElapsedS: result.ElapsedS,
		Warnings: result.Warnings,
	}, nil
}

// TestRecoverBatch_FirstCallSpawnsArchivesStaleReportAndStopsLiveStrand proves the first call for a
// batch with no live recovery record spawns a fresh recovery strand: a stale report at the batch's
// own report path is archived (renamed with a timestamp suffix, never deleted), a prior recorded
// strand still reported live by the reed is stopped, the fresh BatchState's strand fields are
// recorded, and — with no report landing inside the wait window — the call returns Running with
// Spawned: true.
func TestRecoverBatch_FirstCallSpawnsArchivesStaleReportAndStopsLiveStrand(t *testing.T) {
	fx := newRecoverFixture(t)

	stalePath := filepath.Join(fx.ReportsDir, "01-json-flag.yaml")
	if err := os.WriteFile(stalePath, []byte("status: FAILED\nhead_sha: deadbeef\n"), 0o644); err != nil {
		t.Fatalf("seed stale report: %v", err)
	}

	fx.Deps.State.Batches[1] = &websterengine.BatchState{
		Slug: "json-flag", Kind: "recovery", Terminal: true, Status: "dead", StrandGUID: "orphan-1",
	}
	fx.Reed.status = reedengine.StatusResult{Strands: []reedengine.StrandStatus{{GUID: "orphan-1", Live: true}}}

	clk := &recoverFakeClock{now: time.Unix(0, 0)}
	result, err := driveRecoverBatch(fx.Deps, 1, 3*time.Second, clk)
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
	// The archive timestamp comes from the INJECTED clock (clk starts at
	// time.Unix(0,0)), not the wall clock — F16: recoverSpawn archives with
	// clk.Now so a test clock makes the archive name deterministic.
	if !strings.Contains(archived[0], "19700101T000000Z") {
		t.Errorf("archived report %q; want the injected-clock epoch stamp 19700101T000000Z", archived[0])
	}
	data, err := os.ReadFile(archived[0])
	if err != nil {
		t.Fatalf("read archived report %s: %v", archived[0], err)
	}
	if !strings.Contains(string(data), "status: FAILED") {
		t.Errorf("archived report content = %q; want the prior report preserved verbatim", string(data))
	}

	// The prior orphan's live strand was stopped before the fresh spawn.
	found := false
	for _, guid := range fx.Reed.removedStrands {
		if guid == "orphan-1" {
			found = true
		}
	}
	if !found {
		t.Errorf("RemoveStrand calls = %v; want the prior live strand %q stopped", fx.Reed.removedStrands, "orphan-1")
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
	wantHead, err := gitrepo.New(fx.Worktree).CurrentSHA()
	if err != nil {
		t.Fatalf("CurrentSHA() error = %v", err)
	}
	if bs.StartSHA != wantHead {
		t.Errorf("BatchState.StartSHA = %q; want the fresh HeadSHA %q", bs.StartSHA, wantHead)
	}

	if fx.Engine.prepareCallCount() != 1 {
		t.Errorf("Engine.prepareCalls = %d; want exactly 1", fx.Engine.prepareCallCount())
	}
}

// TestRecoverBatch_DoneReportRefusedUnlessPriorDead proves the finished-work guard: a batch whose
// on-disk report already parses to status: done is refused (recover-batch never archives finished
// work — record-batch is the consuming verb), EXCEPT when the batch's persisted state is terminal
// dead, webster's own dead-orphan late-report case, where the report is archived and the spawn
// proceeds.
func TestRecoverBatch_DoneReportRefusedUnlessPriorDead(t *testing.T) {
	doneReport := "status: OK\nhead_sha: deadbeef\n"

	t.Run("DoneReport_NoPriorRecord_Refused", func(t *testing.T) {
		fx := newRecoverFixture(t)
		if err := os.WriteFile(filepath.Join(fx.ReportsDir, "01-json-flag.yaml"), []byte(doneReport), 0o644); err != nil {
			t.Fatalf("seed done report: %v", err)
		}

		_, err := driveRecoverBatch(fx.Deps, 1, time.Second, &recoverFakeClock{now: time.Unix(0, 0)})
		if err == nil {
			t.Fatal("RecoverBatch() with a done report = nil error; want the finished-work refusal")
		}
		if !strings.Contains(err.Error(), "record-batch") {
			t.Errorf("error = %q; want it to name record-batch as the consuming verb", err.Error())
		}
		// The done report must be untouched — never archived by a refusal.
		if _, statErr := os.Stat(filepath.Join(fx.ReportsDir, "01-json-flag.yaml")); statErr != nil {
			t.Errorf("stat(done report) = %v; want the report left in place", statErr)
		}
		if fx.Engine.prepareCallCount() != 0 {
			t.Errorf("Engine.prepareCalls = %d; want 0 (no spawn on refusal)", fx.Engine.prepareCallCount())
		}
	})

	t.Run("DoneReport_PriorTerminalDead_ArchivedAndSpawned", func(t *testing.T) {
		fx := newRecoverFixture(t)
		if err := os.WriteFile(filepath.Join(fx.ReportsDir, "01-json-flag.yaml"), []byte(doneReport), 0o644); err != nil {
			t.Fatalf("seed late done report: %v", err)
		}
		fx.Deps.State.Batches[1] = &websterengine.BatchState{
			Slug: "json-flag", Kind: "recovery", Terminal: true, Status: "dead",
		}

		result, err := driveRecoverBatch(fx.Deps, 1, time.Second, &recoverFakeClock{now: time.Unix(0, 0)})
		if err != nil {
			t.Fatalf("RecoverBatch() for a dead batch with a late done report = %v; want the archive-and-respawn path", err)
		}
		if !result.Spawned {
			t.Error("RecoverResult.Spawned = false; want true (dead-orphan late report is archive-never-refuse)")
		}
	})
}

// TestRecoverBatch_SecondCallAttachesAndPersistsDoneDigest proves a re-entrant second call for the
// same batch, while the recovery strand is still recorded and non-terminal, ATTACHES rather than
// re-spawning (the fake Starter's Prepare call count stays at 1), and once the batch's own report
// has landed, returns the terminal digest with state persisted and the done-classified substrate
// released (strand removed, run dir removed).
func TestRecoverBatch_SecondCallAttachesAndPersistsDoneDigest(t *testing.T) {
	fx := newRecoverFixture(t)
	clk := &recoverFakeClock{now: time.Unix(0, 0)}

	// First call spawns; no report yet.
	first, err := driveRecoverBatch(fx.Deps, 1, 2*time.Second, clk)
	if err != nil {
		t.Fatalf("RecoverBatch() first call error = %v; want nil", err)
	}
	if !first.Spawned || !first.Running {
		t.Fatalf("first call = %+v; want Spawned=true Running=true", first)
	}
	runDir := fx.Deps.State.Batches[1].ShuttleRunDir
	strandGUID := fx.Deps.State.Batches[1].StrandGUID

	// The re-fork's report has now landed, self-reporting the worktree's real
	// HEAD (the head_sha cross-check refuses a report whose SHA disagrees
	// with the worktree it left behind — see the mismatch test below).
	realHead, err := gitrepo.New(fx.Worktree).CurrentSHA()
	if err != nil {
		t.Fatalf("CurrentSHA() error = %v", err)
	}
	writeRecoverReport(t, fx.ReportsDir, "status: OK\nhead_sha: "+realHead+"\n")

	second, err := driveRecoverBatch(fx.Deps, 1, 2*time.Second, clk)
	if err != nil {
		t.Fatalf("RecoverBatch() second call error = %v; want nil", err)
	}
	if second.Spawned {
		t.Error("second call Spawned = true; want false (ATTACH, not a re-spawn)")
	}
	if second.Running {
		t.Error("second call Running = true; want false (terminal once the report landed)")
	}
	if second.Digest == nil || second.Digest.Status != websterengine.DigestStatusDone {
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
	if bs.Status != websterengine.DigestStatusDone {
		t.Errorf("BatchState.Status = %q; want %q", bs.Status, websterengine.DigestStatusDone)
	}
	if bs.Digest == nil {
		t.Error("BatchState.Digest = nil; want the persisted digest")
	}
	// A recovery-completed batch must land in the accumulated CardSHAs trail
	// exactly like a fork batch, or the integration-suite bisect searches a
	// gapped trail and blames the wrong card (crucible round fable-r1's F3).
	if len(bs.CardSHAs) != 1 || bs.CardSHAs[0] != realHead {
		t.Errorf("BatchState.CardSHAs = %v; want [%s] persisted at recovery terminal", bs.CardSHAs, realHead)
	}
	if fx.Deps.State.CurrentBatch != 0 {
		t.Errorf("State.CurrentBatch = %d; want 0 (cleared)", fx.Deps.State.CurrentBatch)
	}

	// done-substrate release: strand removed, run dir removed.
	foundRemoved := false
	for _, guid := range fx.Reed.removedStrands {
		if guid == strandGUID {
			foundRemoved = true
		}
	}
	if !foundRemoved {
		t.Errorf("RemoveStrand calls = %v; want the done strand %q removed", fx.Reed.removedStrands, strandGUID)
	}
	if _, statErr := os.Stat(runDir); !os.IsNotExist(statErr) {
		t.Errorf("stat(%s) = %v; want the done run dir removed", runDir, statErr)
	}
}

// TestRecoverBatch_ReportHeadSHAMismatchIsHardError proves the recovery path applies RecordBatch's
// own head_sha cross-check: a recovery report whose self-reported head_sha disagrees with the
// worktree's actual HEAD is refused loud rather than persisted into the digest and the bisect trail
// (crucible round fable-r1's F4 — before this check, only the fork path verified the report against
// the repo it claimed to describe).
func TestRecoverBatch_ReportHeadSHAMismatchIsHardError(t *testing.T) {
	fx := newRecoverFixture(t)
	clk := &recoverFakeClock{now: time.Unix(0, 0)}

	first, err := driveRecoverBatch(fx.Deps, 1, time.Second, clk)
	if err != nil {
		t.Fatalf("RecoverBatch() first call error = %v; want nil", err)
	}
	if !first.Running {
		t.Fatalf("first call = %+v; want Running=true", first)
	}

	writeRecoverReport(t, fx.ReportsDir, "status: OK\nhead_sha: deadbeef\n")

	_, err = driveRecoverBatch(fx.Deps, 1, time.Second, clk)
	if err == nil {
		t.Fatal("RecoverBatch() with a mismatching head_sha = nil error; want the cross-check refusal")
	}
	if !strings.Contains(err.Error(), "does not match the worktree's actual HEAD") {
		t.Errorf("error = %q; want the head_sha mismatch refusal", err.Error())
	}
	// Nothing was persisted terminal: the batch record stays non-terminal for
	// a corrected report or an operator to resolve.
	if bs := fx.Deps.State.Batches[1]; bs.Terminal {
		t.Errorf("BatchState.Terminal = true; want false after a refused report")
	}
}

// TestRecoverBatch_TimeoutAcrossCallsClassifiesDead proves RecoveryTimeoutMin is measured from the
// recorded SpawnedAt ACROSS re-entrant calls, not reset per call: virtual time advanced well past
// the configured timeout between two calls classifies dead/timeout on the second call even though
// neither call's own wait budget alone would ever cross it, and the dead classification keeps BOTH
// the strand and the run directory (diagnosis material), never removing either.
func TestRecoverBatch_TimeoutAcrossCallsClassifiesDead(t *testing.T) {
	fx := newRecoverFixture(t)
	fx.Deps.Config.RecoveryTimeoutMin = 1 // 1 minute
	clk := &recoverFakeClock{now: time.Unix(0, 0)}

	first, err := driveRecoverBatch(fx.Deps, 1, 2*time.Second, clk)
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

	second, err := driveRecoverBatch(fx.Deps, 1, 2*time.Second, clk)
	if err != nil {
		t.Fatalf("RecoverBatch() second call error = %v; want nil", err)
	}
	if second.Running {
		t.Error("second call Running = true; want false (terminal dead/timeout)")
	}
	if second.Digest == nil || second.Digest.Status != websterengine.DigestStatusDead {
		t.Fatalf("second call Digest = %+v; want a dead digest", second.Digest)
	}
	if second.Digest.DeadReason != websterengine.DeadReasonTimeout {
		t.Errorf("second call Digest.DeadReason = %q; want %q", second.Digest.DeadReason, websterengine.DeadReasonTimeout)
	}
	if second.ElapsedS < 120 {
		t.Errorf("second call ElapsedS = %d; want >= 120 (measured since the original spawn)", second.ElapsedS)
	}

	// dead classification keeps both the strand and the run dir.
	for _, guid := range fx.Reed.removedStrands {
		if guid == strandGUID {
			t.Errorf("RemoveStrand calls = %v; want the dead-classified strand %q kept", fx.Reed.removedStrands, strandGUID)
		}
	}
	if _, statErr := os.Stat(runDir); statErr != nil {
		t.Errorf("stat(%s) = %v; want the dead-classified run dir kept", runDir, statErr)
	}
}

// TestRecoverSpawnOrAttach_PredecessorDigestFollowsExecutionOrder proves RecoverSpawnOrAttach's
// recovery prompt carries the execution predecessor's digest rather than the batchNumber-1
// batch, and that the batch sitting first in execution order renders the no-previous-digest
// sentinel regardless of its own number — the same execution-predecessor semantics card 5 proved
// for BeginBatch, mirrored here for the recovery path.
func TestRecoverSpawnOrAttach_PredecessorDigestFollowsExecutionOrder(t *testing.T) {
	t.Run("predecessor is the batch that actually ran before it", func(t *testing.T) {
		fx := newRecoverFixture(t)
		// Batch 2 (list-tests) runs before batch 1 (json-flag) in execution
		// order, even though batch 1's declared number is lower.
		fx.Deps.Batches = []batcher.Batch{
			{Cards: []planparser.Card{{Number: 2, Slug: "list-tests", Title: "list-tests", Intent: "list the tests"}}},
			{Cards: []planparser.Card{{Number: 1, Slug: "json-flag", Title: "json-flag", Intent: "add the --json flag"}}},
		}
		fx.Deps.State.Batches[2] = &websterengine.BatchState{
			Slug:     "list-tests",
			Terminal: true,
			Status:   "done",
			Digest: &websterengine.Digest{
				Batch:   "02-list-tests",
				Status:  websterengine.DigestStatusDone,
				HeadSHA: "cafef00d",
			},
		}

		clk := &recoverFakeClock{now: time.Unix(0, 0)}
		_, spawned, err := websterengine.RecoverSpawnOrAttach(fx.Deps, 1, clk)
		if err != nil {
			t.Fatalf("RecoverSpawnOrAttach() error = %v; want nil", err)
		}
		if !spawned {
			t.Fatal("RecoverSpawnOrAttach() spawned = false; want true")
		}

		prompt := fx.Engine.lastPromptText()
		for _, want := range []string{"02-list-tests", "head_sha=cafef00d"} {
			if !strings.Contains(prompt, want) {
				t.Errorf("recovery prompt does not contain %q; got:\n%s", want, prompt)
			}
		}
	})

	t.Run("the batch sitting first renders the sentinel regardless of its number", func(t *testing.T) {
		fx := newRecoverFixture(t)
		fx.Deps.Batches = []batcher.Batch{
			{Cards: []planparser.Card{{Number: 2, Slug: "list-tests", Title: "list-tests", Intent: "list the tests"}}},
			{Cards: []planparser.Card{{Number: 1, Slug: "json-flag", Title: "json-flag", Intent: "add the --json flag"}}},
		}

		clk := &recoverFakeClock{now: time.Unix(0, 0)}
		_, spawned, err := websterengine.RecoverSpawnOrAttach(fx.Deps, 2, clk)
		if err != nil {
			t.Fatalf("RecoverSpawnOrAttach() error = %v; want nil", err)
		}
		if !spawned {
			t.Fatal("RecoverSpawnOrAttach() spawned = false; want true")
		}

		prompt := fx.Engine.lastPromptText()
		if !strings.Contains(prompt, "none (first batch)") {
			t.Errorf("recovery prompt does not contain the first-batch sentinel; got:\n%s", prompt)
		}
	})
}

// TestRecoverBatch_UnrecordedOrTerminalBatchSpawnsFresh proves the spawn-or-attach decision spawns
// fresh — never attaches — both when the batch has no recorded BatchState at all and when a prior
// recovery attempt already reached a terminal classification, in contrast to a recorded,
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
					fx.Reed.status = reedengine.StatusResult{Strands: []reedengine.StrandStatus{{GUID: tt.prior.StrandGUID, Live: true}}}
				}
			}

			clk := &recoverFakeClock{now: time.Unix(0, 0)}
			_, err := driveRecoverBatch(fx.Deps, 1, 1*time.Second, clk)
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
