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
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/batcher"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitexec"
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
func (e *recoverFakeEngine) InterruptSequence() []shuttleengine.PaneInput          { return nil }
func (e *recoverFakeEngine) TrustDismissSequence(string) []shuttleengine.PaneInput { return nil }
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

	// The terminal recovery path refuses a plan that differs from the recorded fingerprint, so the state records this one.
	planDir := seedPlanDir(t)
	websterDir := t.TempDir()
	state := &websterengine.State{Batches: map[int]*websterengine.BatchState{}}
	if err := websterengine.RestampPlanBaseline(state, planDir, websterDir); err != nil {
		t.Fatalf("RestampPlanBaseline() error = %v", err)
	}

	deps := websterengine.RecoverDeps{
		Starter:    runner,
		Plan:       plan,
		Batches:    batches,
		State:      state,
		Roles:      roles,
		Config:     websterengine.Config{SelfFixCap: 2, RecoveryTimeoutMin: 30},
		Engine:     engine,
		Reed:       reed,
		ShuttleCfg: shuttleCfg,
		Geom: websterengine.Geometry{
			AnchorRoot:   worktree,
			WorktreeRoot: worktree,
			WebsterDir:   websterDir,
			ReportsDir:   reportsDir,
			StencilsDir:  fabricengine.StencilsDir(hubPath),
			SpecsDir:     fabricengine.SpecsDir(hubPath),
			// A real (empty) plan directory: the terminal recovery path now runs the same
			// post-batch mechanical pass record-batch does, which re-baselines the plan
			// fingerprint over this directory. No card in this fixture declares a handle, so
			// nothing is ever written into it.
			PlanDir: planDir,
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
		postWarnings, perr := websterengine.PersistRecoveryTerminal(deps, deps.State, batchNumber, result.Digest)
		result.Warnings = append(result.Warnings, postWarnings...)
		if perr != nil {
			return nil, perr
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
// TestPersistRecoveryTerminal_RefusesPlanEditedSinceSpawn proves a card edited after the recovery spawned is refused with ErrFingerprintMismatch before the post-batch pass restamps over it, and the record stays non-terminal.
func TestPersistRecoveryTerminal_RefusesPlanEditedSinceSpawn(t *testing.T) {
	fx := newRecoverFixture(t)
	clk := &recoverFakeClock{now: time.Unix(0, 0)}

	if _, err := driveRecoverBatch(fx.Deps, 1, 2*time.Second, clk); err != nil {
		t.Fatalf("first call error = %v; want nil", err)
	}
	realHead, err := gitrepo.New(fx.Worktree).CurrentSHA()
	if err != nil {
		t.Fatalf("CurrentSHA() error = %v", err)
	}
	writeRecoverReport(t, fx.ReportsDir, "status: OK\nhead_sha: "+realHead+"\n")

	cardPath := filepath.Join(fx.Deps.Geom.PlanDir, "01-json-flag.md")
	if err := os.WriteFile(cardPath, []byte("# Card 1 — json-flag\n\n**Intent:** edited after the spawn.\n"), 0o644); err != nil {
		t.Fatalf("edit card: %v", err)
	}

	_, err = driveRecoverBatch(fx.Deps, 1, 2*time.Second, clk)
	if !errors.Is(err, websterengine.ErrFingerprintMismatch) {
		t.Fatalf("second call error = %v; want errors.Is(err, ErrFingerprintMismatch)", err)
	}
	if bs := fx.Deps.State.Batches[1]; bs.Terminal {
		t.Errorf("BatchState.Terminal = true; want the record left non-terminal")
	}
}

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

// recoverAtReportHead spawns the recovery strand, then seeds a done report at the worktree's current HEAD and returns that head.
func recoverAtReportHead(t *testing.T, fx *recoverFixture, clk *recoverFakeClock) string {
	t.Helper()
	first, err := driveRecoverBatch(fx.Deps, 1, time.Second, clk)
	if err != nil {
		t.Fatalf("RecoverBatch() first call error = %v; want nil", err)
	}
	if !first.Running {
		t.Fatalf("first call = %+v; want Running=true", first)
	}
	head := strings.TrimSpace(mustGit(t, fx.Worktree, "rev-parse", "HEAD"))
	writeRecoverReport(t, fx.ReportsDir, "status: OK\nhead_sha: "+head+"\n")
	return head
}

// TestRecoverBatch_ParentMergeAfterReportHead proves recover-batch follows record-batch's merge-only head rule:
// a --no-ff parent merge landing after the recovery report's head is accepted, the batch is recorded at the report's head,
// and the moved-HEAD notice names the merge.
func TestRecoverBatch_ParentMergeAfterReportHead(t *testing.T) {
	fx := newRecoverFixture(t)
	clk := &recoverFakeClock{now: time.Unix(0, 0)}
	head := recoverAtReportHead(t, fx, clk)
	fx.Deps.ParentBranch = func() (string, error) { return "parent1", nil }

	mustGit(t, fx.Worktree, "checkout", "-b", "parent1", head)
	commitFile(t, fx.Worktree, "parent1.txt", "p1", "parent1 commit")
	mustGit(t, fx.Worktree, "checkout", "-")
	mustGit(t, fx.Worktree, "merge", "--no-ff", "-m", "merge parent1", "parent1")
	merge := strings.TrimSpace(mustGit(t, fx.Worktree, "rev-parse", "HEAD"))

	result, err := driveRecoverBatch(fx.Deps, 1, time.Second, clk)
	if err != nil {
		t.Fatalf("RecoverBatch() error = %v; want nil", err)
	}
	if result.Digest == nil || result.Digest.Status != websterengine.DigestStatusDone {
		t.Fatalf("Digest = %+v; want a done digest", result.Digest)
	}
	bs := fx.Deps.State.Batches[1]
	if !bs.Terminal {
		t.Error("BatchState.Terminal = false; want true")
	}
	if len(bs.CardSHAs) != 1 || bs.CardSHAs[0] != head {
		t.Errorf("BatchState.CardSHAs = %v; want [%s]", bs.CardSHAs, head)
	}
	found := false
	for _, w := range result.Warnings {
		if strings.Contains(w, "only merge commits") && strings.Contains(w, merge) {
			found = true
		}
	}
	if !found {
		t.Errorf("Warnings = %v; want the moved-HEAD notice naming merge %s", result.Warnings, merge)
	}
}

// TestRecoverBatch_NonMergeCommitAfterReportHeadRefused proves a plain commit on top of the report's head is refused with both SHAs and the merge-only rule, leaving the batch non-terminal.
func TestRecoverBatch_NonMergeCommitAfterReportHeadRefused(t *testing.T) {
	fx := newRecoverFixture(t)
	clk := &recoverFakeClock{now: time.Unix(0, 0)}
	head := recoverAtReportHead(t, fx, clk)

	commitFile(t, fx.Worktree, "extra.txt", "x", "extra commit")
	newHead := strings.TrimSpace(mustGit(t, fx.Worktree, "rev-parse", "HEAD"))

	_, err := driveRecoverBatch(fx.Deps, 1, time.Second, clk)
	if err == nil {
		t.Fatal("RecoverBatch() error = nil; want a refusal")
	}
	for _, want := range []string{head, newHead, "only merge commits"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q missing %q", err.Error(), want)
		}
	}
	if fx.Deps.State.Batches[1].Terminal {
		t.Error("BatchState.Terminal = true; want false after a refusal")
	}
}

// TestRecoverBatch_MergeInProgressRefused proves a conflicting merge left in progress refuses recover-batch with the merge --continue/--abort pointer, leaving the batch non-terminal.
func TestRecoverBatch_MergeInProgressRefused(t *testing.T) {
	fx := newRecoverFixture(t)
	clk := &recoverFakeClock{now: time.Unix(0, 0)}
	head := recoverAtReportHead(t, fx, clk)

	base := strings.TrimSpace(mustGit(t, fx.Worktree, "rev-parse", "--abbrev-ref", "HEAD"))
	mustGit(t, fx.Worktree, "checkout", "-b", "parent1", head)
	commitFile(t, fx.Worktree, "base.txt", "parent side", "parent side edit")
	mustGit(t, fx.Worktree, "checkout", base)
	commitFile(t, fx.Worktree, "base.txt", "our side", "our side edit")
	if _, _, exitCode, err := gitexec.RunGit([]string{"merge", "--no-ff", "-m", "merge parent1", "parent1"}, fx.Worktree); err != nil || exitCode == 0 {
		t.Fatalf("conflicting merge: exit=%d err=%v; want a conflict", exitCode, err)
	}

	_, err := driveRecoverBatch(fx.Deps, 1, time.Second, clk)
	if err == nil {
		t.Fatal("RecoverBatch() error = nil; want a refusal while a merge is in progress")
	}
	if !strings.Contains(err.Error(), "merge --continue") || !strings.Contains(err.Error(), "merge --abort") {
		t.Errorf("error %q; want the merge --continue/--abort pointer", err.Error())
	}
	if fx.Deps.State.Batches[1].Terminal {
		t.Error("BatchState.Terminal = true; want false after a refusal")
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

// TestRecoverBatch_TerminalRunsTheSamePostBatchChecksAsRecordBatch is the regression test for the
// round-4 review's R4-03. A recovery batch reaching status done used to be marked terminal on the
// recovery strand's self-reported status plus the head-SHA cross-check alone — no done-checks, no
// handle binding, no scope guard, no drift detection and no plan-staleness re-baseline. Master's own
// failure ladder treats a terminal recovery digest as "move on to the next batch", so a card
// recovered that way never bound its plan: handles and every later card kept referencing an unbound
// handle for the rest of the plan's life.
//
// Here the recovery reports done over a Create card whose target never appeared in the tree.
// The mechanical pass must refuse it and take the batch terminal failed with its report archived, exactly as record-batch does, so the next recover-batch spawns a fresh strand.
func TestRecoverBatch_TerminalRunsTheSamePostBatchChecksAsRecordBatch(t *testing.T) {
	fx := newRecoverFixture(t)
	clk := &recoverFakeClock{now: time.Unix(0, 0)}

	// The batch's card declares a Create target that the recovery never lands.
	card := planparser.Card{
		Number: 1, Slug: "json-flag", Title: "json-flag", Intent: "add the --json flag",
		Targets:      []string{"internal/never/there.go#"},
		TargetGroups: []planparser.TargetGroup{{Type: planparser.CardTypeCreate, Refs: []string{"internal/never/there.go#"}}},
	}
	fx.Deps.Plan.Cards = []planparser.Card{card}
	fx.Deps.Batches[0].Cards = []planparser.Card{card}

	first, err := driveRecoverBatch(fx.Deps, 1, 2*time.Second, clk)
	if err != nil {
		t.Fatalf("RecoverBatch() first call error = %v; want nil", err)
	}
	if !first.Spawned || !first.Running {
		t.Fatalf("first call = %+v; want Spawned=true Running=true", first)
	}

	realHead, err := gitrepo.New(fx.Worktree).CurrentSHA()
	if err != nil {
		t.Fatalf("CurrentSHA() error = %v", err)
	}
	writeRecoverReport(t, fx.ReportsDir, "status: OK\nhead_sha: "+realHead+"\n")

	_, err = driveRecoverBatch(fx.Deps, 1, 2*time.Second, clk)
	if !errors.Is(err, websterengine.ErrBatchFailed) {
		t.Fatalf("RecoverBatch() second call error = %v; want errors.Is(err, ErrBatchFailed) — a recovery reporting done over an unlanded Create target must be failed", err)
	}
	bs := fx.Deps.State.Batches[1]
	if !bs.Terminal || bs.Status != websterengine.DigestStatusFailed {
		t.Errorf("BatchState = terminal %v status %q; want terminal failed", bs.Terminal, bs.Status)
	}
	if bs.Digest == nil || len(bs.Digest.Reasons) == 0 {
		t.Errorf("failed digest = %+v; want reasons naming the done-check finding", bs.Digest)
	}
	if _, statErr := os.Stat(filepath.Join(fx.ReportsDir, websterengine.ReportFileName(1, "json-flag"))); !os.IsNotExist(statErr) {
		t.Errorf("stat(live report) = %v; want the report archived away", statErr)
	}
	if fx.Deps.State.CurrentBatch != 0 {
		t.Errorf("State.CurrentBatch = %d; want 0", fx.Deps.State.CurrentBatch)
	}

	// The next recover-batch spawns a fresh strand rather than re-attaching to the finished one.
	again, err := driveRecoverBatch(fx.Deps, 1, 1*time.Second, clk)
	if err != nil {
		t.Fatalf("RecoverBatch() third call error = %v; want nil", err)
	}
	if !again.Spawned {
		t.Errorf("third call = %+v; want Spawned=true after a failed recovery", again)
	}
}

// failedRecord builds the record RecordBatch leaves behind for a batch it failed on its merits:
// terminal, status failed, reasons ending with the suspect paths.
func failedRecord(reasons ...string) *websterengine.BatchState {
	return &websterengine.BatchState{
		Slug: "json-flag", Kind: "fork", Terminal: true, Status: websterengine.DigestStatusFailed,
		Digest: &websterengine.Digest{Batch: "01-json-flag", Status: websterengine.DigestStatusFailed, Reasons: reasons},
	}
}

// TestRecoverSpawnOrAttach_FailedBatchSpawnsWithFailureDigest proves recover-batch works from a failed batch for every reason class:
// no refusal, and the rendered prompt carries every reason and suspect path.
func TestRecoverSpawnOrAttach_FailedBatchSpawnsWithFailureDigest(t *testing.T) {
	tests := []struct {
		name    string
		reasons []string
	}{
		{name: "fork contract write", reasons: []string{"fork wrote the report contract file", "suspect path: _lyx/webster/state.json"}},
		{name: "fabric reference with failing verify", reasons: []string{"fabric-reference in parent transcript", "card verify failed: go test ./x", "suspect path: internal/x/x.go"}},
		{name: "done-check finding", reasons: []string{"Create target internal/never/there.go# does not resolve"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fx := newRecoverFixture(t)
			fx.Deps.State.Batches[1] = failedRecord(tt.reasons...)
			clk := &recoverFakeClock{now: time.Unix(0, 0)}

			_, spawned, err := websterengine.RecoverSpawnOrAttach(fx.Deps, 1, clk)
			if err != nil {
				t.Fatalf("RecoverSpawnOrAttach() error = %v; want nil", err)
			}
			if !spawned {
				t.Fatal("spawned = false; want a fresh recovery strand")
			}
			prompt := fx.Engine.lastPromptText()
			for _, r := range tt.reasons {
				if !strings.Contains(prompt, r) {
					t.Errorf("recovery prompt lacks reason %q", r)
				}
			}
		})
	}
}

// TestRecoverSpawnOrAttach_FailedBatchArchivesLateReport proves an OK report a still-running fork writes after the batch failed is archived rather than refused, so no refusal ring re-forms.
func TestRecoverSpawnOrAttach_FailedBatchArchivesLateReport(t *testing.T) {
	fx := newRecoverFixture(t)
	fx.Deps.State.Batches[1] = failedRecord("fork wrote the report contract file")
	writeRecoverReport(t, fx.ReportsDir, "status: OK\nhead_sha: abc\n")
	clk := &recoverFakeClock{now: time.Unix(0, 0)}

	if _, spawned, err := websterengine.RecoverSpawnOrAttach(fx.Deps, 1, clk); err != nil || !spawned {
		t.Fatalf("RecoverSpawnOrAttach() = spawned %v, err %v; want spawned without a refusal", spawned, err)
	}
	if _, statErr := os.Stat(filepath.Join(fx.ReportsDir, websterengine.ReportFileName(1, "json-flag"))); !os.IsNotExist(statErr) {
		t.Errorf("stat(live report) = %v; want the late report archived away", statErr)
	}
	archived, _ := filepath.Glob(filepath.Join(fx.ReportsDir, "01-json-flag-*.yaml"))
	if len(archived) != 1 {
		t.Errorf("archived reports = %v; want exactly 1", archived)
	}
}

// TestRecoverSpawn_InheritsTheStuckForksStartSHA proves a recovery record carries the ORIGINAL
// bracket's base commit rather than re-capturing the head at recovery-spawn time. The recovery's
// terminal path computes its post-batch delta from this SHA, and a fork frequently commits part of
// its work before getting stuck — a start SHA captured at recovery time excludes exactly that part,
// so a symbol the stuck fork already created reads as never created and its card's handle reports
// bind-count-mismatch against work that did land.
func TestRecoverSpawn_InheritsTheStuckForksStartSHA(t *testing.T) {
	fx := newRecoverFixture(t)
	clk := &recoverFakeClock{now: time.Unix(0, 0)}

	bracketStart, err := gitrepo.New(fx.Worktree).CurrentSHA()
	if err != nil {
		t.Fatalf("CurrentSHA() error = %v", err)
	}
	fx.Deps.State.Batches[1] = &websterengine.BatchState{Slug: "json-flag", StartSHA: bracketStart, Kind: "fork"}
	// The stuck fork committed part of its work before reporting stuck.
	commitFile(t, fx.Worktree, "internal/partial/impl.go", "package partial\n", "01.1: partial work")

	if _, _, err := websterengine.RecoverSpawnOrAttach(fx.Deps, 1, clk); err != nil {
		t.Fatalf("RecoverSpawnOrAttach() error = %v; want nil", err)
	}

	got := fx.Deps.State.Batches[1]
	if got.Kind != "recovery" {
		t.Fatalf("BatchState.Kind = %q; want %q", got.Kind, "recovery")
	}
	if got.StartSHA != bracketStart {
		t.Errorf("recovery BatchState.StartSHA = %q; want the stuck fork's own %q — the post-batch delta must span the whole bracket, not just the recovery's own share of it", got.StartSHA, bracketStart)
	}
}

// TestRecoverSpawn_RecordsCardSet proves the spawned recovery record carries the batch's card set as NN-<slug> entries.
func TestRecoverSpawn_RecordsCardSet(t *testing.T) {
	fx := newRecoverFixture(t)
	clk := &recoverFakeClock{now: time.Unix(0, 0)}

	if _, _, err := websterengine.RecoverSpawnOrAttach(fx.Deps, 1, clk); err != nil {
		t.Fatalf("RecoverSpawnOrAttach() error = %v; want nil", err)
	}
	got := fx.Deps.State.Batches[1].Cards
	if want := []string{"01-json-flag"}; !slices.Equal(got, want) {
		t.Errorf("recovery BatchState.Cards = %v; want %v", got, want)
	}
	if hashes := fx.Deps.State.Batches[1].CardHashes; len(hashes) != 1 || hashes["01-json-flag"] == "" {
		t.Errorf("recovery BatchState.CardHashes = %v; want one hash for 01-json-flag", hashes)
	}
}

// erroringStarter is a websterengine.Starter double whose Start always fails wrapping
// shuttleengine.ErrNotStarted — the not-ready-start error shuttle now returns from Start itself
// (per the shuttle-start-guarantees-readiness discussion), which recoverFixture's real
// *shuttleengine.Runner over recoverFakeEngine/recoverFakeReed never reaches on its own, since
// recoverFakeEngine.Startup always reports StartupReady.
type erroringStarter struct{}

func (erroringStarter) Start(spec shuttleengine.Spec) (*shuttleengine.Run, error) {
	return nil, fmt.Errorf("webster test: recovery strand never became ready: %w", shuttleengine.ErrNotStarted)
}

var _ websterengine.Starter = erroringStarter{}

// TestRecoverSpawnOrAttach_NotReadyStartSurfacesAndRecordsNothing proves a not-ready recovery
// start (shuttle's Start returning ErrNotStarted after tearing its own strand down) surfaces
// unchanged from RecoverSpawnOrAttach and records no batch state — the strand shuttle already
// tore down must never be persisted as this batch's recovery record.
func TestRecoverSpawnOrAttach_NotReadyStartSurfacesAndRecordsNothing(t *testing.T) {
	fx := newRecoverFixture(t)
	fx.Deps.Starter = erroringStarter{}
	clk := &recoverFakeClock{now: time.Unix(0, 0)}

	bs, spawned, err := websterengine.RecoverSpawnOrAttach(fx.Deps, 1, clk)

	if !errors.Is(err, shuttleengine.ErrNotStarted) {
		t.Errorf("RecoverSpawnOrAttach() error = %v; want it to wrap shuttleengine.ErrNotStarted", err)
	}
	if spawned {
		t.Error("RecoverSpawnOrAttach() spawned = true; want false on a not-ready start")
	}
	if bs != nil {
		t.Errorf("RecoverSpawnOrAttach() BatchState = %+v; want nil on a not-ready start", bs)
	}
	if fx.Deps.State.Batches[1] != nil {
		t.Errorf("State.Batches[1] = %+v; want nil — a strand shuttle already tore down must record no guid", fx.Deps.State.Batches[1])
	}
}

// TestRecoverSpawnOrAttach_WayForward_StartFailureIsTransient proves a failed recovery-strand start names the re-run,
// and that re-running the verb once the provider answers spawns the strand.
func TestRecoverSpawnOrAttach_WayForward_StartFailureIsTransient(t *testing.T) {
	fx := newRecoverFixture(t)
	realStarter := fx.Deps.Starter
	fx.Deps.Starter = erroringStarter{}
	clk := &recoverFakeClock{now: time.Unix(0, 0)}

	_, _, err := websterengine.RecoverSpawnOrAttach(fx.Deps, 1, clk)
	if err == nil || !strings.Contains(err.Error(), "way forward: transient, re-run `lyx webster recover-batch 1`") {
		t.Fatalf("RecoverSpawnOrAttach() error = %v; want the transient re-run way forward", err)
	}

	fx.Deps.Starter = realStarter
	_, spawned, err := websterengine.RecoverSpawnOrAttach(fx.Deps, 1, clk)
	if err != nil || !spawned {
		t.Fatalf("RecoverSpawnOrAttach() after the retry = spawned %v, error %v; want a spawned strand", spawned, err)
	}
}

// TestPersistRecoveryTerminal_WayForward_NoRecordedState proves a batch whose record vanished underneath the recovery wait names the recover-batch re-run,
// and that re-running spawns afresh.
func TestPersistRecoveryTerminal_WayForward_NoRecordedState(t *testing.T) {
	fx := newRecoverFixture(t)
	clk := &recoverFakeClock{now: time.Unix(0, 0)}

	_, err := websterengine.PersistRecoveryTerminal(fx.Deps, fx.Deps.State, 1, &websterengine.Digest{Batch: "01-json-flag", Status: websterengine.DigestStatusDone})
	if err == nil || !strings.Contains(err.Error(), "way forward: re-run `lyx webster recover-batch 1`") {
		t.Fatalf("PersistRecoveryTerminal() error = %v; want the recover-batch re-run way forward", err)
	}

	if _, spawned, err := websterengine.RecoverSpawnOrAttach(fx.Deps, 1, clk); err != nil || !spawned {
		t.Fatalf("RecoverSpawnOrAttach() after the way forward = spawned %v, error %v; want a spawned strand", spawned, err)
	}
}

// TestRecoverSpawnOrAttach_WayForward_MalformedReport proves the way forward record-batch names for a malformed report:
// recover-batch archives it and spawns a recovery strand.
func TestRecoverSpawnOrAttach_WayForward_MalformedReport(t *testing.T) {
	fx := newRecoverFixture(t)
	writeRecoverReport(t, fx.ReportsDir, "status: bogus\nhead_sha: deadbeef\n")
	clk := &recoverFakeClock{now: time.Unix(0, 0)}

	_, spawned, err := websterengine.RecoverSpawnOrAttach(fx.Deps, 1, clk)
	if err != nil || !spawned {
		t.Fatalf("RecoverSpawnOrAttach() over a malformed report = spawned %v, error %v; want it archived and a strand spawned", spawned, err)
	}
	if _, statErr := os.Stat(filepath.Join(fx.ReportsDir, "01-json-flag.yaml")); !os.IsNotExist(statErr) {
		t.Errorf("stat(malformed report) = %v; want it archived out of the report path", statErr)
	}
}

// TestRecoverBatch_WayForward_DoneReportRecordsInstead proves the OK-report refusal names record-batch and that the report it leaves in place is exactly what record-batch consumes:
// once the operator takes the way forward by removing the prior record's obstruction (here a terminal dead prior, the state after which the report is late), recover-batch proceeds and archives it.
func TestRecoverBatch_WayForward_DoneReportRecordsInstead(t *testing.T) {
	fx := newRecoverFixture(t)
	writeRecoverReport(t, fx.ReportsDir, "status: OK\nhead_sha: deadbeef\n")
	clk := &recoverFakeClock{now: time.Unix(0, 0)}

	_, err := driveRecoverBatch(fx.Deps, 1, time.Second, clk)
	if err == nil || !strings.Contains(err.Error(), "`lyx webster record-batch 1`") {
		t.Fatalf("RecoverBatch() error = %v; want the record-batch way forward", err)
	}
	if _, statErr := os.Stat(filepath.Join(fx.ReportsDir, "01-json-flag.yaml")); statErr != nil {
		t.Errorf("stat(done report) = %v; want it left for record-batch", statErr)
	}
}

// suspectRecovery seeds a failed batch 1 whose Master parent-write flagged internal/x.go holding "forged".
// internal/x.go is committed as "orig" (the batch's start commit) and the failed record carries the flagged blob.
// It returns the fixture, the start commit, and the flagged blob.
func suspectRecovery(t *testing.T) (*recoverFixture, string, string) {
	t.Helper()
	fx := newRecoverFixture(t)
	start := commitFile(t, fx.Worktree, "internal/x.go", "orig", "orig")
	if err := os.WriteFile(filepath.Join(fx.Worktree, "internal", "x.go"), []byte("forged"), 0o644); err != nil {
		t.Fatal(err)
	}
	blob := strings.TrimSpace(mustGit(t, fx.Worktree, "hash-object", "internal/x.go"))
	mustGit(t, fx.Worktree, "checkout", "--", "internal/x.go")
	fx.Deps.State.Batches[1] = &websterengine.BatchState{
		Slug: "json-flag", Kind: "fork", Terminal: true, Status: websterengine.DigestStatusFailed, StartSHA: start,
		SuspectPaths: []websterengine.SuspectPath{{Path: "internal/x.go", Blob: blob}},
	}
	return fx, start, blob
}

func recoverSuspect(t *testing.T, fx *recoverFixture) (*recoverDriveResult, error) {
	t.Helper()
	clk := &recoverFakeClock{now: time.Unix(0, 0)}
	recoverAtReportHead(t, fx, clk)
	return driveRecoverBatch(fx.Deps, 1, time.Second, clk)
}

func TestPersistRecoveryTerminal_FailsWhenSuspectContentSurvives(t *testing.T) {
	fx, start, _ := suspectRecovery(t)
	clk := &recoverFakeClock{now: time.Unix(0, 0)}
	if _, err := driveRecoverBatch(fx.Deps, 1, time.Second, clk); err != nil {
		t.Fatal(err)
	}
	commitFile(t, fx.Worktree, "internal/x.go", "forged", "strand keeps forged")
	head := strings.TrimSpace(mustGit(t, fx.Worktree, "rev-parse", "HEAD"))
	writeRecoverReport(t, fx.ReportsDir, "status: OK\nhead_sha: "+head+"\n")
	_, err := driveRecoverBatch(fx.Deps, 1, time.Second, clk)
	if !errors.Is(err, websterengine.ErrBatchFailed) {
		t.Fatalf("error = %v; want ErrBatchFailed", err)
	}
	if !strings.Contains(err.Error(), "internal/x.go") || !strings.Contains(err.Error(), "revert it to "+start) {
		t.Errorf("error = %q; want the path and the revert instruction", err.Error())
	}
	bs := fx.Deps.State.Batches[1]
	if !bs.Terminal || bs.Status != websterengine.DigestStatusFailed || len(bs.SuspectPaths) != 1 || bs.SuspectPaths[0].Blob == "" {
		t.Errorf("record = %+v; want terminal failed with SuspectPaths kept", bs)
	}
}

// TestPersistRecoveryTerminal_RefailKeepsFlaggedBlob proves a re-failed recovery keeps the blob the audit first flagged:
// a strand that deletes the committed flagged file without committing the delete fails, and a later strand that restores it is failed again.
func TestPersistRecoveryTerminal_RefailKeepsFlaggedBlob(t *testing.T) {
	fx, start, blob := suspectRecovery(t)
	clk := &recoverFakeClock{now: time.Unix(0, 0)}
	if _, err := driveRecoverBatch(fx.Deps, 1, time.Second, clk); err != nil {
		t.Fatal(err)
	}
	commitFile(t, fx.Worktree, "internal/x.go", "forged", "strand keeps forged")
	if err := os.Remove(filepath.Join(fx.Worktree, "internal", "x.go")); err != nil {
		t.Fatal(err)
	}
	head := strings.TrimSpace(mustGit(t, fx.Worktree, "rev-parse", "HEAD"))
	writeRecoverReport(t, fx.ReportsDir, "status: OK\nhead_sha: "+head+"\n")
	if _, err := driveRecoverBatch(fx.Deps, 1, time.Second, clk); !errors.Is(err, websterengine.ErrBatchFailed) {
		t.Fatalf("first recovery error = %v; want ErrBatchFailed", err)
	}
	if got := fx.Deps.State.Batches[1].SuspectPaths; len(got) != 1 || got[0].Blob != blob {
		t.Fatalf("SuspectPaths after re-fail = %+v; want the flagged blob %s kept", got, blob)
	}

	if _, err := driveRecoverBatch(fx.Deps, 1, time.Second, clk); err != nil {
		t.Fatal(err)
	}
	mustGit(t, fx.Worktree, "checkout", "--", "internal/x.go")
	commitFile(t, fx.Worktree, "other.txt", "o", "other work")
	head = strings.TrimSpace(mustGit(t, fx.Worktree, "rev-parse", "HEAD"))
	writeRecoverReport(t, fx.ReportsDir, "status: OK\nhead_sha: "+head+"\n")
	_, err := driveRecoverBatch(fx.Deps, 1, time.Second, clk)
	if !errors.Is(err, websterengine.ErrBatchFailed) || !strings.Contains(err.Error(), "revert it to "+start) {
		t.Fatalf("second recovery error = %v; want ErrBatchFailed naming the revert to %s", err, start)
	}
}

func TestPersistRecoveryTerminal_FailsOnUncommittedSuspectPath(t *testing.T) {
	fx, _, _ := suspectRecovery(t)
	if err := os.WriteFile(filepath.Join(fx.Worktree, "internal", "x.go"), []byte("forged"), 0o644); err != nil {
		t.Fatal(err)
	}
	commitFile(t, fx.Worktree, "other.txt", "o", "other work")
	head := strings.TrimSpace(mustGit(t, fx.Worktree, "rev-parse", "HEAD"))
	_, err := recoverSuspect(t, fx)
	if !errors.Is(err, websterengine.ErrBatchFailed) {
		t.Fatalf("error = %v; want ErrBatchFailed", err)
	}
	if !strings.Contains(err.Error(), "internal/x.go") || !strings.Contains(err.Error(), head) {
		t.Errorf("error = %q; want the path and the head %s", err.Error(), head)
	}
}

func TestPersistRecoveryTerminal_PassesReverted(t *testing.T) {
	fx, _, _ := suspectRecovery(t)
	commitFile(t, fx.Worktree, "other.txt", "o", "other work")
	result, err := recoverSuspect(t, fx)
	if err != nil {
		t.Fatalf("error = %v; want nil", err)
	}
	if result.Digest == nil || result.Digest.Status != websterengine.DigestStatusDone {
		t.Fatalf("Digest = %+v; want done", result.Digest)
	}
}

func TestPersistRecoveryTerminal_PassesRederived(t *testing.T) {
	fx, _, _ := suspectRecovery(t)
	commitFile(t, fx.Worktree, "internal/x.go", "derived", "strand re-derives")
	result, err := recoverSuspect(t, fx)
	if err != nil {
		t.Fatalf("error = %v; want nil", err)
	}
	if result.Digest == nil || result.Digest.Status != websterengine.DigestStatusDone {
		t.Fatalf("Digest = %+v; want done", result.Digest)
	}
}

func TestRecoverSpawn_CarriesSuspectPaths(t *testing.T) {
	fx, _, blob := suspectRecovery(t)
	clk := &recoverFakeClock{now: time.Unix(0, 0)}
	if _, err := driveRecoverBatch(fx.Deps, 1, time.Second, clk); err != nil {
		t.Fatal(err)
	}
	bs := fx.Deps.State.Batches[1]
	if bs.Kind != "recovery" || len(bs.SuspectPaths) != 1 || bs.SuspectPaths[0].Path != "internal/x.go" || bs.SuspectPaths[0].Blob != blob {
		t.Errorf("recovery record = %+v; want SuspectPaths carried", bs)
	}
}
