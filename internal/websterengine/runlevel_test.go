//go:build integration

// runlevel_test.go exercises Run end to end (Tier 2 — see
// docs/benchmarks/running-tests.md): a real scratch git repo backs
// WorktreeRoot and a real on-disk plan directory backs PlanDir (so
// ParsePlan/Validate/the batchifier/fingerprint all run for real, against
// the real identity batchifier — newRunFixture injects it into
// RunDeps.Batcher directly, since Run itself does no config I/O and needs no
// _lyx/ tree on WorktreeRoot), while the Master spawn itself is a local,
// fully-scripted fake (MasterStarter/
// MasterHandle) whose Wait method can carry an onWait
// side effect that writes outcome.yaml/summary.md the instant before it
// returns — modeling the real ordering (Master writes its two contract
// files DURING the run Wait blocks on). FindRun's cross-process resolution
// is satisfied by hand-seeding a run.json under the fixture's own shuttle
// run-dir root, mirroring what a real *shuttleengine.Runner.Start would
// have produced. This package's testmain_test.go already wires
// gitkit.HermeticGitEnv() for the whole test binary — package-local (the
// internal and external test packages deliberately do not share a
// test-helper package, mirroring recoverbatch_test.go/recordbatch_test.go's
// own precedent), except for the shared newScratchRepo/commitFile/
// seedPlanDir/mustFingerprint helpers already defined in beginbatch_test.go.

package websterengine_test

import (
	"encoding/json"
	"errors"
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
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// runFakeReed is a hermetic shuttleengine.ReedOps double: RemoveStrand records
// every call and retires the guid from the scripted Status result; AddStrand
// is never reached by Run's own path (Run never registers a strand itself —
// StartMaster's real implementation would, but the fake Starter below skips
// straight to a scripted handle) and errors loud if ever called, so a stray
// call surfaces immediately rather than silently no-opping.
type runFakeReed struct {
	mu             sync.Mutex
	status         reedengine.StatusResult
	removedStrands []string
}

func (m *runFakeReed) AddStrand(spec reedengine.AddSpec) (reedengine.Strand, error) {
	return reedengine.Strand{}, fmt.Errorf("run fake reed: AddStrand is not used by Run's own path")
}

func (m *runFakeReed) RemoveStrand(guid string, recursive bool) (reedengine.Removed, error) {
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

func (m *runFakeReed) Status() (reedengine.StatusResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.status, nil
}

func (m *runFakeReed) SendText(guid, text string, submit bool) error { return nil }
func (m *runFakeReed) SendKey(guid, key string) error                { return nil }
func (m *runFakeReed) CapturePane(guid string) (string, error)       { return "", nil }

var _ shuttleengine.ReedOps = (*runFakeReed)(nil)

// runFakeEngine is a hermetic shuttleengine.Engine double: every method
// returns a fixed, inert value. Run's own path never reaches any of these —
// the whole-session fork audit Run reads (Result.ForkAudit) is scripted
// directly on the fake handle's Result, not produced by calling the engine —
// so this fake exists only to satisfy RunDeps.Engine's type.
type runFakeEngine struct{}

func (e *runFakeEngine) Prepare(runDir string, spec shuttleengine.Spec, cfg shuttleengine.Config) (shuttleengine.Launch, error) {
	return shuttleengine.Launch{}, nil
}
func (e *runFakeEngine) ParseEvents(data []byte) ([]shuttleengine.Event, error) { return nil, nil }
func (e *runFakeEngine) Startup(capture string) shuttleengine.StartupState {
	return shuttleengine.StartupReady
}
func (e *runFakeEngine) InterruptSequence() []shuttleengine.PaneInput    { return nil }
func (e *runFakeEngine) TrustDismissSequence() []shuttleengine.PaneInput { return nil }
func (e *runFakeEngine) ComposeSend(text string) []shuttleengine.PaneInput {
	return nil
}
func (e *runFakeEngine) AuditForks(sessionID, workdir string) (shuttleengine.ForkAudit, error) {
	return shuttleengine.ForkAudit{}, nil
}
func (e *runFakeEngine) AuditForksIncremental(sessionID, workdir string, seenTranscripts map[string]bool) (shuttleengine.ForkAudit, error) {
	return shuttleengine.ForkAudit{}, nil
}
func (e *runFakeEngine) ModelSwitchSequence(model string) []shuttleengine.PaneInput {
	return nil
}

var _ shuttleengine.Engine = (*runFakeEngine)(nil)

// runFakeHandle is a hermetic websterengine.MasterHandle double: Wait runs
// the caller-scripted onWait side effect (if any) — modeling Master writing
// its contract files DURING the call Wait blocks on — then returns the
// scripted Result/error.
type runFakeHandle struct {
	strandGUID string
	result     shuttleengine.Result
	waitErr    error
	onWait     func()
}

func (h *runFakeHandle) StrandGUID() string { return h.strandGUID }

func (h *runFakeHandle) Wait() (shuttleengine.Result, error) {
	if h.onWait != nil {
		h.onWait()
	}
	return h.result, h.waitErr
}

var _ websterengine.MasterHandle = (*runFakeHandle)(nil)

// runFakeStarter is a hermetic websterengine.MasterStarter double: it
// records every spec StartMaster was called with and hands back the
// caller-scripted handle, or startErr when the caller wants to prove a step
// never reaches the spawn at all.
type runFakeStarter struct {
	mu         sync.Mutex
	startCalls []shuttleengine.Spec
	handle     websterengine.MasterHandle
	startErr   error
}

func (s *runFakeStarter) StartMaster(spec shuttleengine.Spec) (websterengine.MasterHandle, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.startCalls = append(s.startCalls, spec)
	if s.startErr != nil {
		return nil, s.startErr
	}
	return s.handle, nil
}

func (s *runFakeStarter) callCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.startCalls)
}

var _ websterengine.MasterStarter = (*runFakeStarter)(nil)

// seedRunPlanDir writes a syntactically complete, validation-clean
// plan-format plan with numCards cards into a fresh temp plan directory:
// each card carries a format-4 Create group whose single target is the card's own
// new-file path (so path-missing never fires — a Create group's targets stay
// exempt from on-disk existence checking exactly as Creates: entries were).
// The overview carries NO plan-level "## verify:" section — deliberately,
// so ShouldRunIntegration(plan) is false and the integration stage
// (runlevel.go's runIntegrationStage) stays a no-op for every fixture built
// on this helper; the dedicated integration-stage tests (integration_test.go)
// instead call appendIntegrationVerify against an already-seeded plan dir.
// numCards == 0 yields a "## Card Index" section with no entries at all,
// which ParsePlan's own parseCardIndex refuses loud ("no card index entries
// found") — the vehicle for the zero-batch refusal test, which under the
// flat model is actually the batchifier-derived-zero-batches refusal (an
// empty Cards list, if it ever parsed, would batchify to zero batches too).
func seedRunPlanDir(t *testing.T, numCards int) string {
	t.Helper()
	dir := t.TempDir()

	var index strings.Builder
	files := map[string]string{}
	for i := 1; i <= numCards; i++ {
		slug := fmt.Sprintf("batch%d", i)
		file := fmt.Sprintf("%02d-%s.md", i, slug)
		index.WriteString(fmt.Sprintf("%d — %s — placeholder card %d\n", i, slug, i))

		creates := fmt.Sprintf("internal/%s/new.go", slug)
		body := fmt.Sprintf(
			"# Card %d — %s\n\n**Create:**\n- `%s`\n\n**Intent:** placeholder card.\n",
			i, slug, creates,
		)
		files[file] = body
	}
	files["00-overview.md"] = "---\nformat: 4\napproved: true\n---\n\n# Plan\n\nFraming.\n\n## Card Index\n\n" +
		index.String()

	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatalf("write plan fixture %s: %v", name, err)
		}
	}
	return dir
}

// seedShuttleRunState hand-seeds a run.json under runDirRoot naming
// strandGUID/sessionID, satisfying shuttleengine.FindRun's cross-process
// scan the way a real *shuttleengine.Runner.Start would — without needing to
// actually drive one (Run's own fake MasterStarter never touches the real
// run-dir machinery).
func seedShuttleRunState(t *testing.T, runDirRoot, strandGUID, sessionID string) {
	t.Helper()
	runDir := filepath.Join(runDirRoot, "fake-run-"+strandGUID)
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatalf("mkdir run dir: %v", err)
	}
	rs := shuttleengine.RunState{
		RunID:      "fake-run-" + strandGUID,
		StrandGUID: strandGUID,
		SessionID:  sessionID,
		CreatedAt:  time.Now().UTC().Format(time.RFC3339),
	}
	data, err := json.MarshalIndent(rs, "", "  ")
	if err != nil {
		t.Fatalf("marshal run state: %v", err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "run.json"), data, 0o644); err != nil {
		t.Fatalf("write run.json: %v", err)
	}
}

// runFixture is a fully-wired set of Run dependencies: a real scratch git
// repo as WorktreeRoot, a real on-disk plan directory, a fake reed/engine,
// and a fake Starter a test scripts per case.
type runFixture struct {
	Deps           websterengine.RunDeps
	Reed           *runFakeReed
	Starter        *runFakeStarter
	Worktree       string
	PlanDir        string
	ShuttleRunRoot string
}

func newRunFixture(t *testing.T, numCards int) *runFixture {
	t.Helper()

	planDir := seedRunPlanDir(t, numCards)
	worktree := newScratchRepo(t)
	commitFile(t, worktree, "base.txt", "base", "base commit")

	reed := &runFakeReed{}
	starter := &runFakeStarter{}
	hubPath := filepath.Dir(worktree)
	// webster's prompts are read from disk at call time now, so the fixture's
	// hub must carry them before Run reaches RenderMasterPrompt.
	seedHubStencils(t, hubPath)
	shuttleRunRoot := t.TempDir()
	shuttleCfg := shuttleengine.Config{RunDir: shuttleRunRoot, RunTimeoutMin: 60, StartupTimeoutS: 30}

	roles := map[websterengine.Role]modelspec.Resolved{
		websterengine.RoleMaster:   {Engine: "claude", Model: "master-model", Params: map[string]string{}},
		websterengine.RoleRecovery: {Engine: "claude", Model: "recovery-model", Params: map[string]string{"effort": "high"}},
	}

	// Select("") is the right call here rather than Active: the fixture's
	// WorktreeRoot is a bare scratch git repo with no _lyx/ tree, and the
	// point of the runlevel-call-site decision is that Run needs no config
	// tree.
	activeBatcher, err := batcher.Select("")
	if err != nil {
		t.Fatalf("batcher.Select(\"\") error = %v", err)
	}

	bisectRepo := gitrepo.New(worktree)
	deps := websterengine.RunDeps{
		Starter:    starter,
		Reed:       reed,
		Engine:     &runFakeEngine{},
		ShuttleCfg: shuttleCfg,
		Roles:      roles,
		Batcher:    activeBatcher,
		Config: websterengine.Config{
			SelfFixCap:       2,
			MasterTimeoutMin: 480,
			PollWaitS:        480,
		},
		Geom: websterengine.Geometry{
			AnchorRoot:   worktree,
			WorktreeRoot: worktree,
			WebsterDir:   t.TempDir(),
			ScratchDir:   t.TempDir(),
			ReportsDir:   t.TempDir(),
			PromptsDir:   t.TempDir(),
			StencilsDir:  fabricengine.StencilsDir(hubPath),
			PlanDir:      planDir,
		},
		RefMatcher:   websterengine.NeverMatches{},
		OpenBisector: func() (websterengine.FabricBisector, error) { return bisectRepo, nil },
	}

	return &runFixture{Deps: deps, Reed: reed, Starter: starter, Worktree: worktree, PlanDir: planDir, ShuttleRunRoot: shuttleRunRoot}
}

// addCardUses rewrites an already-seeded card file under planDir — one of seedRunPlanDir's own
// "%02d-batch%d.md" files — to carry a "**Uses:**" field naming ref, modelled on
// integration_test.go's own appendIntegrationVerify: read the file, splice the field in, write it
// back. Inserted ahead of the card's own "**Intent:**" line, which every seedRunPlanDir card
// carries.
// A path-shaped ref this points at another card's Create target is already satisfied by
// planparser.Validate's own createTargetsUnion check within the same plan, but a caller that wants
// the ref to also resolve on disk (mirroring a real cross-card file dependency) must still create it
// under the fixture's own worktree — this helper only edits the plan-format text.
func addCardUses(t *testing.T, planDir string, cardNumber int, ref string) {
	t.Helper()
	slug := fmt.Sprintf("batch%d", cardNumber)
	path := filepath.Join(planDir, fmt.Sprintf("%02d-%s.md", cardNumber, slug))
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read card fixture %s: %v", path, err)
	}
	const marker = "\n**Intent:**"
	body := string(data)
	idx := strings.Index(body, marker)
	if idx == -1 {
		t.Fatalf("card fixture %s carries no **Intent:** marker to splice **Uses:** ahead of", path)
	}
	usesBlock := fmt.Sprintf("\n**Uses:**\n- `%s`\n", ref)
	newBody := body[:idx] + usesBlock + body[idx:]
	if err := os.WriteFile(path, []byte(newBody), 0o644); err != nil {
		t.Fatalf("write card fixture with Uses: %v", err)
	}
}

// seedMatchingState saves st into fx's webster dir after stamping its
// PlanFingerprint to match fx's own on-disk plan directory (and defaulting
// its Batches map when nil), so Run's own fingerprint gate passes and the
// pre-seeded state survives into the run unmodified.
func seedMatchingState(t *testing.T, fx *runFixture, st *websterengine.State) {
	t.Helper()
	st.PlanFingerprint = mustFingerprint(t, fx.PlanDir)
	if st.Batches == nil {
		st.Batches = map[int]*websterengine.BatchState{}
	}
	if err := websterengine.SaveState(fx.Deps.Geom.WebsterDir, fx.Deps.Geom.ScratchDir, st); err != nil {
		t.Fatalf("seed matching state: %v", err)
	}
}

// TestRun_ErrRunBusy proves Run's fail-fast refusal when another invocation already holds
// scratchDir's run.lock: the loser touches nothing (the Starter is never reached) and the error
// satisfies errors.Is(err, ErrRunBusy).
func TestRun_ErrRunBusy(t *testing.T) {
	fx := newRunFixture(t, 1)

	if err := os.MkdirAll(fx.Deps.Geom.ScratchDir, 0o755); err != nil {
		t.Fatalf("mkdir webster scratch dir: %v", err)
	}
	held, err := lock.AcquireWriteLock(filepath.Join(fx.Deps.Geom.ScratchDir, "run.lock"))
	if err != nil {
		t.Fatalf("acquire run.lock: %v", err)
	}
	defer held.Release()

	_, err = websterengine.Run(fx.Deps, websterengine.RunOptions{})
	if !errors.Is(err, websterengine.ErrRunBusy) {
		t.Fatalf("Run() error = %v; want errors.Is(err, ErrRunBusy)", err)
	}
	if fx.Starter.callCount() != 0 {
		t.Errorf("Starter was reached (%d calls) while run.lock was held; want zero", fx.Starter.callCount())
	}
}

// TestRun_NilBatcherRefuses proves Run refuses with ErrNilBatcher when the caller never populated
// RunDeps.Batcher — webstercli always populates the field, so this test is the sentinel's only
// evidence.
func TestRun_NilBatcherRefuses(t *testing.T) {
	fx := newRunFixture(t, 1)
	fx.Deps.Batcher = nil

	_, err := websterengine.Run(fx.Deps, websterengine.RunOptions{})
	if !errors.Is(err, websterengine.ErrNilBatcher) {
		t.Fatalf("Run() error = %v; want errors.Is(err, ErrNilBatcher)", err)
	}
}

// TestRun_ZeroBatchPlanRefusedLoud proves a plan that parses to zero cards (and so batchifies to
// zero execution batches) is refused loud before any spawn — nothing-to-build is a malformed plan,
// never a vacuous outcome: done.
func TestRun_ZeroBatchPlanRefusedLoud(t *testing.T) {
	fx := newRunFixture(t, 0)

	_, err := websterengine.Run(fx.Deps, websterengine.RunOptions{})
	if err == nil {
		t.Fatalf("Run() error = nil; want an error refusing a zero-batch plan")
	}
	if fx.Starter.callCount() != 0 {
		t.Errorf("Starter was reached (%d calls) for a zero-batch plan; want zero", fx.Starter.callCount())
	}
}

// TestRun_FingerprintMismatchWithoutFreshLeavesPauseIntact proves a stale on-disk state.json (a
// fingerprint that no longer matches the plan directory) refuses loud without --fresh, never
// reaching the spawn, and that a pending pause request is left untouched by the refusal (only a run
// that passes every gate clears it).
func TestRun_FingerprintMismatchWithoutFreshLeavesPauseIntact(t *testing.T) {
	fx := newRunFixture(t, 1)

	st := &websterengine.State{PlanFingerprint: "stale-fingerprint", Batches: map[int]*websterengine.BatchState{}}
	if err := websterengine.SaveState(fx.Deps.Geom.WebsterDir, fx.Deps.Geom.ScratchDir, st); err != nil {
		t.Fatalf("seed stale state: %v", err)
	}
	if err := websterengine.RequestPause(fx.Deps.Geom.ScratchDir); err != nil {
		t.Fatalf("RequestPause() error = %v", err)
	}

	_, err := websterengine.Run(fx.Deps, websterengine.RunOptions{})
	if !errors.Is(err, websterengine.ErrFingerprintMismatch) {
		t.Fatalf("Run() error = %v; want errors.Is(err, ErrFingerprintMismatch)", err)
	}
	if !websterengine.PauseRequested(fx.Deps.Geom.ScratchDir) {
		t.Error("pause flag cleared on a refused run; want it left intact")
	}
	if fx.Starter.callCount() != 0 {
		t.Errorf("Starter was reached (%d calls) on a fingerprint mismatch; want zero", fx.Starter.callCount())
	}
}

// TestRun_FreshArchivesStateReportsAndClearsPrompts proves --fresh archives the stale state.json
// and reports dir under timestamped names (never deleting them), recreates an empty reports dir,
// and clears the re-renderable prompts dir outright (never archived).
func TestRun_FreshArchivesStateReportsAndClearsPrompts(t *testing.T) {
	fx := newRunFixture(t, 1)

	staleState := &websterengine.State{PlanFingerprint: "stale", Batches: map[int]*websterengine.BatchState{}}
	if err := websterengine.SaveState(fx.Deps.Geom.WebsterDir, fx.Deps.Geom.ScratchDir, staleState); err != nil {
		t.Fatalf("seed stale state: %v", err)
	}

	if err := os.MkdirAll(fx.Deps.Geom.ReportsDir, 0o755); err != nil {
		t.Fatalf("mkdir reports dir: %v", err)
	}
	reportPath := filepath.Join(fx.Deps.Geom.ReportsDir, "01-batch1.yaml")
	if err := os.WriteFile(reportPath, []byte("status: OK\nhead_sha: deadbeef\n"), 0o644); err != nil {
		t.Fatalf("seed stale report: %v", err)
	}

	if err := os.MkdirAll(fx.Deps.Geom.PromptsDir, 0o755); err != nil {
		t.Fatalf("mkdir prompts dir: %v", err)
	}
	promptPath := filepath.Join(fx.Deps.Geom.PromptsDir, "01-batch1.md")
	if err := os.WriteFile(promptPath, []byte("stale prompt\n"), 0o644); err != nil {
		t.Fatalf("seed stale prompt: %v", err)
	}

	fx.Starter.startErr = fmt.Errorf("stop before spawn")

	_, err := websterengine.Run(fx.Deps, websterengine.RunOptions{Fresh: true})
	if err == nil {
		t.Fatalf("Run() error = nil; want the scripted starter error")
	}

	// The stale state.json is archived (renamed, content preserved) rather
	// than deleted; the live path is then reinitialized fresh by the same
	// --fresh sequence, so a state.json legitimately exists there again by
	// the time Run returns — it must no longer carry the stale fingerprint.
	stateFile := filepath.Join(fx.Deps.Geom.WebsterDir, "state.json")
	archived, globErr := filepath.Glob(filepath.Join(fx.Deps.Geom.WebsterDir, "state-*.json"))
	if globErr != nil || len(archived) != 1 {
		t.Fatalf("archived state glob = %v, %v; want exactly 1", archived, globErr)
	}
	archivedData, err := os.ReadFile(archived[0])
	if err != nil {
		t.Fatalf("read archived state %s: %v", archived[0], err)
	}
	if !strings.Contains(string(archivedData), `"stale"`) {
		t.Errorf("archived state content = %q; want it to still carry the stale fingerprint", archivedData)
	}
	liveData, err := os.ReadFile(stateFile)
	if err != nil {
		t.Fatalf("read live state.json %s: %v", stateFile, err)
	}
	if strings.Contains(string(liveData), `"stale"`) {
		t.Errorf("live state.json still carries the stale fingerprint; want it reinitialized fresh")
	}

	if _, statErr := os.Stat(reportPath); !os.IsNotExist(statErr) {
		t.Errorf("stale report still present at its original path; want the reports dir archived away wholesale")
	}
	archivedReportsDirs, globErr := filepath.Glob(fx.Deps.Geom.ReportsDir + "-*")
	if globErr != nil || len(archivedReportsDirs) != 1 {
		t.Fatalf("archived reports dir glob = %v, %v; want exactly 1", archivedReportsDirs, globErr)
	}
	if _, statErr := os.Stat(filepath.Join(archivedReportsDirs[0], "01-batch1.yaml")); statErr != nil {
		t.Errorf("archived reports dir missing the stale report: %v", statErr)
	}
	if info, statErr := os.Stat(fx.Deps.Geom.ReportsDir); statErr != nil || !info.IsDir() {
		t.Errorf("ReportsDir not recreated after --fresh archiving: %v", statErr)
	}

	if _, statErr := os.Stat(promptPath); !os.IsNotExist(statErr) {
		t.Errorf("rendered prompt file still present; want the prompts dir cleared (re-renderable, never archived)")
	}
}

// TestRun_EntryTimeReclaimStopsLiveMasterAndRecoveryStrandsButNotAbsent proves the entry-time
// reclaim stops a recorded, still-live Master strand and a recorded, non-terminal, still-live
// recovery-batch strand, but never touches a recorded strand the reed no longer reports at all
// (cleanly-absent — already gone, nothing to stop).
func TestRun_EntryTimeReclaimStopsLiveMasterAndRecoveryStrandsButNotAbsent(t *testing.T) {
	fx := newRunFixture(t, 1)

	st := &websterengine.State{
		MasterStrand: "prior-master-strand",
		Batches: map[int]*websterengine.BatchState{
			1: {Slug: "batch1", Kind: "recovery", Terminal: false, StrandGUID: "prior-recovery-strand"},
			2: {Slug: "batch2", Kind: "recovery", Terminal: false, StrandGUID: "absent-recovery-strand"},
		},
	}
	seedMatchingState(t, fx, st)

	fx.Reed.status = reedengine.StatusResult{Strands: []reedengine.StrandStatus{
		{GUID: "prior-master-strand", Live: true},
		{GUID: "prior-recovery-strand", Live: true},
		// "absent-recovery-strand" is deliberately absent from Status at all.
	}}

	fx.Starter.startErr = fmt.Errorf("stop before spawn")

	_, err := websterengine.Run(fx.Deps, websterengine.RunOptions{})
	if err == nil {
		t.Fatalf("Run() error = nil; want the scripted starter error")
	}

	wantRemoved := map[string]bool{"prior-master-strand": true, "prior-recovery-strand": true}
	for _, guid := range fx.Reed.removedStrands {
		if guid == "absent-recovery-strand" {
			t.Errorf("RemoveStrand called for a cleanly-absent strand %q; want it left untouched", guid)
		}
		delete(wantRemoved, guid)
	}
	if len(wantRemoved) != 0 {
		t.Errorf("RemoveStrand calls = %v; missing %v", fx.Reed.removedStrands, wantRemoved)
	}
}

// TestRun_StaleOutcomeAndSummaryArchivedBeforeSpawn proves both stale outcome.yaml and stale
// summary.md are archived (renamed with a timestamp suffix, never deleted) before Master ever
// spawns.
func TestRun_StaleOutcomeAndSummaryArchivedBeforeSpawn(t *testing.T) {
	fx := newRunFixture(t, 1)

	if err := os.MkdirAll(fx.Deps.Geom.WebsterDir, 0o755); err != nil {
		t.Fatalf("mkdir webster dir: %v", err)
	}
	outcomePath := filepath.Join(fx.Deps.Geom.WebsterDir, "outcome.yaml")
	summaryPath := filepath.Join(fx.Deps.Geom.WebsterDir, "summary.md")
	if err := os.WriteFile(outcomePath, []byte("outcome: stuck\nstuck_reason: \"prior run\"\nbatches_done: 0\n"), 0o644); err != nil {
		t.Fatalf("seed stale outcome: %v", err)
	}
	if err := os.WriteFile(summaryPath, []byte("# Prior run\n\nStale.\n"), 0o644); err != nil {
		t.Fatalf("seed stale summary: %v", err)
	}

	fx.Starter.startErr = fmt.Errorf("stop before spawn")

	_, err := websterengine.Run(fx.Deps, websterengine.RunOptions{})
	if err == nil {
		t.Fatalf("Run() error = nil; want the scripted starter error")
	}

	if _, statErr := os.Stat(outcomePath); !os.IsNotExist(statErr) {
		t.Errorf("stale outcome.yaml still present at its original path; want it archived away")
	}
	if _, statErr := os.Stat(summaryPath); !os.IsNotExist(statErr) {
		t.Errorf("stale summary.md still present at its original path; want it archived away")
	}
	if archived, globErr := filepath.Glob(filepath.Join(fx.Deps.Geom.WebsterDir, "outcome-*.yaml")); globErr != nil || len(archived) != 1 {
		t.Errorf("archived outcome glob = %v, %v; want exactly 1", archived, globErr)
	}
	if archived, globErr := filepath.Glob(filepath.Join(fx.Deps.Geom.WebsterDir, "summary-*.md")); globErr != nil || len(archived) != 1 {
		t.Errorf("archived summary glob = %v, %v; want exactly 1", archived, globErr)
	}
}

// TestRun_AssertedModelInitializedToMasterRoleModel proves the Master spawn persists
// State.AssertedModel to the launch model (RoleMaster's resolved model) BEFORE ever blocking on
// Wait — the idempotent-assertion baseline begin-batch's own per-batch check consults from batch 1
// onward — along with the strand and session identities.
func TestRun_AssertedModelInitializedToMasterRoleModel(t *testing.T) {
	fx := newRunFixture(t, 1)

	handle := &runFakeHandle{strandGUID: "master-strand-x", waitErr: fmt.Errorf("stop after spawn")}
	fx.Starter.handle = handle
	seedShuttleRunState(t, fx.ShuttleRunRoot, "master-strand-x", "master-session-x")

	_, err := websterengine.Run(fx.Deps, websterengine.RunOptions{})
	if err == nil {
		t.Fatalf("Run() error = nil; want the scripted wait error")
	}

	st, loadErr := websterengine.LoadState(fx.Deps.Geom.WebsterDir, fx.Deps.Geom.ScratchDir)
	if loadErr != nil {
		t.Fatalf("LoadState() error = %v", loadErr)
	}
	if st.AssertedModel != "master-model" {
		t.Errorf("State.AssertedModel = %q; want %q (the launch model)", st.AssertedModel, "master-model")
	}
	if st.MasterStrand != "master-strand-x" {
		t.Errorf("State.MasterStrand = %q; want %q", st.MasterStrand, "master-strand-x")
	}
	if st.MasterSessionID != "master-session-x" {
		t.Errorf("State.MasterSessionID = %q; want %q", st.MasterSessionID, "master-session-x")
	}
}

// TestRun_MasterStrandPersistedBeforeFindRun proves F14's orphan-window narrowing: when FindRun
// fails AFTER Master's pane is live (no shuttle run state seeded, so the session-ID resolve
// errors), Run still errors — but state.json has already recorded MasterStrand, so the next run's
// entry-time reclaim can find and stop the orphaned live pane.
// Without the pre-resolve save, the live pane would be invisible to every future reclaim.
func TestRun_MasterStrandPersistedBeforeFindRun(t *testing.T) {
	fx := newRunFixture(t, 1)

	fx.Starter.handle = &runFakeHandle{strandGUID: "master-strand-orphan"}
	// Deliberately DO NOT seed shuttle run state — FindRun then fails.

	_, err := websterengine.Run(fx.Deps, websterengine.RunOptions{})
	if err == nil {
		t.Fatal("Run() = nil error; want the FindRun resolve failure")
	}

	st, loadErr := websterengine.LoadState(fx.Deps.Geom.WebsterDir, fx.Deps.Geom.ScratchDir)
	if loadErr != nil || st == nil {
		t.Fatalf("LoadState() = %v, %v; want the pre-resolve state persisted", st, loadErr)
	}
	if st.MasterStrand != "master-strand-orphan" {
		t.Errorf("State.MasterStrand = %q; want it persisted BEFORE the FindRun failure so the reclaim can find the orphan", st.MasterStrand)
	}
}

// TestRun_DoneOutcomeWithValidSummaryAndCleanAuditPopulatesResult proves the full success path: a
// done outcome.yaml with valid batches_done, a valid summary.md, and a clean whole-session audit
// whose fork-transcript count meets the begun fork-batch count together populate RunResult, and the
// terminal (non-paused) outcome clears any pause flag.
func TestRun_DoneOutcomeWithValidSummaryAndCleanAuditPopulatesResult(t *testing.T) {
	fx := newRunFixture(t, 1)

	seedMatchingState(t, fx, &websterengine.State{
		Batches: map[int]*websterengine.BatchState{
			1: {Slug: "batch1", Kind: "fork", Terminal: true, Status: "done"},
		},
	})

	handle := &runFakeHandle{
		strandGUID: "master-strand-done",
		result: shuttleengine.Result{
			Outcome:   shuttleengine.OutcomeDone,
			SessionID: "master-session-done",
			RunDir:    "/run/dir/done",
			ForkAudit: &shuttleengine.ForkAudit{
				Forks: []shuttleengine.ForkReport{
					{TranscriptPath: "/transcripts/fork1.jsonl", ReportReturned: true},
				},
			},
		},
		onWait: func() {
			if err := os.WriteFile(filepath.Join(fx.Deps.Geom.WebsterDir, "outcome.yaml"), []byte("outcome: done\nstuck_reason: null\nbatches_done: 1\n"), 0o644); err != nil {
				t.Fatalf("write outcome.yaml: %v", err)
			}
			if err := os.WriteFile(filepath.Join(fx.Deps.Geom.WebsterDir, "summary.md"), []byte("# Shipped batch1\n\nAll good.\n"), 0o644); err != nil {
				t.Fatalf("write summary.md: %v", err)
			}
		},
	}
	fx.Starter.handle = handle
	seedShuttleRunState(t, fx.ShuttleRunRoot, "master-strand-done", "master-session-done")

	result, err := websterengine.Run(fx.Deps, websterengine.RunOptions{})
	if err != nil {
		t.Fatalf("Run() error = %v; want nil", err)
	}
	if result.Outcome != "done" {
		t.Errorf("RunResult.Outcome = %q; want %q", result.Outcome, "done")
	}
	if result.BatchesDone != 1 {
		t.Errorf("RunResult.BatchesDone = %d; want 1", result.BatchesDone)
	}
	if result.SummaryTitle != "Shipped batch1" {
		t.Errorf("RunResult.SummaryTitle = %q; want %q", result.SummaryTitle, "Shipped batch1")
	}
	if websterengine.PauseRequested(fx.Deps.Geom.ScratchDir) {
		t.Error("pause flag present after a done outcome; want it cleared")
	}
}

// TestRun_ResumedDoneRunCountsOnlyCurrentSessionForkBatches proves the run-exit audit cross-check
// is session-scoped: a crash-resumed run whose prior session already completed fork batches must
// NOT count them against the fresh session's whole-session audit (which by construction covers only
// the fresh session's own subagents dir).
// Before this scoping, a legitimately completed resume hard-errored with "audited < begun" (round
// fable-r1's F5).
// The current session's own shortfall still fails.
func TestRun_ResumedDoneRunCountsOnlyCurrentSessionForkBatches(t *testing.T) {
	newHandle := func(fx *runFixture, forks []shuttleengine.ForkReport) *runFakeHandle {
		return &runFakeHandle{
			strandGUID: "master-strand-resume",
			result: shuttleengine.Result{
				Outcome:   shuttleengine.OutcomeDone,
				SessionID: "master-session-resume",
				RunDir:    "/run/dir/resume",
				ForkAudit: &shuttleengine.ForkAudit{Forks: forks},
			},
			onWait: func() {
				if err := os.WriteFile(filepath.Join(fx.Deps.Geom.WebsterDir, "outcome.yaml"), []byte("outcome: done\nstuck_reason: null\nbatches_done: 2\n"), 0o644); err != nil {
					t.Fatalf("write outcome.yaml: %v", err)
				}
				if err := os.WriteFile(filepath.Join(fx.Deps.Geom.WebsterDir, "summary.md"), []byte("# Resumed and finished\n\nBoth batches done.\n"), 0o644); err != nil {
					t.Fatalf("write summary.md: %v", err)
				}
			},
		}
	}

	t.Run("PriorSessionBatchesExcluded_ResumePasses", func(t *testing.T) {
		fx := newRunFixture(t, 2)
		// Batch 1 was forked and recorded by the CRASHED prior session; only
		// batch 2 belongs to the fresh session the audit covers.
		seedMatchingState(t, fx, &websterengine.State{
			Batches: map[int]*websterengine.BatchState{
				1: {Slug: "batch1", Kind: "fork", Terminal: true, Status: "done", SessionID: "master-session-crashed"},
				2: {Slug: "batch2", Kind: "fork", Terminal: true, Status: "done", SessionID: "master-session-resume"},
			},
		})
		fx.Starter.handle = newHandle(fx, []shuttleengine.ForkReport{
			{TranscriptPath: "/transcripts/fork2.jsonl", ReportReturned: true},
		})
		seedShuttleRunState(t, fx.ShuttleRunRoot, "master-strand-resume", "master-session-resume")

		if _, err := websterengine.Run(fx.Deps, websterengine.RunOptions{}); err != nil {
			t.Fatalf("Run() on a completed resume = %v; want nil (prior session's batches are outside this session's audit)", err)
		}
	})

	t.Run("CurrentSessionShortfallStillFails", func(t *testing.T) {
		fx := newRunFixture(t, 2)
		seedMatchingState(t, fx, &websterengine.State{
			Batches: map[int]*websterengine.BatchState{
				1: {Slug: "batch1", Kind: "fork", Terminal: true, Status: "done", SessionID: "master-session-resume"},
				2: {Slug: "batch2", Kind: "fork", Terminal: true, Status: "done", SessionID: "master-session-resume"},
			},
		})
		fx.Starter.handle = newHandle(fx, []shuttleengine.ForkReport{
			{TranscriptPath: "/transcripts/fork2.jsonl", ReportReturned: true},
		})
		seedShuttleRunState(t, fx.ShuttleRunRoot, "master-strand-resume", "master-session-resume")

		_, err := websterengine.Run(fx.Deps, websterengine.RunOptions{})
		if err == nil {
			t.Fatal("Run() = nil error; want the audited-fewer-than-begun cross-check failure for the current session")
		}
		if !strings.Contains(err.Error(), "fewer than") {
			t.Errorf("Run() error = %q; want the cross-check shortfall message", err.Error())
		}
	})
}

// TestRun_DoneWithMissingSummaryIsHardError proves a done outcome.yaml with no summary.md at all is
// a hard error — required content-validity on outcome: done, never guessed.
func TestRun_DoneWithMissingSummaryIsHardError(t *testing.T) {
	fx := newRunFixture(t, 1)

	handle := &runFakeHandle{
		strandGUID: "master-strand-nosum",
		result: shuttleengine.Result{
			Outcome:   shuttleengine.OutcomeDone,
			SessionID: "master-session-nosum",
			RunDir:    "/run/dir/nosum",
			ForkAudit: &shuttleengine.ForkAudit{},
		},
		onWait: func() {
			if err := os.WriteFile(filepath.Join(fx.Deps.Geom.WebsterDir, "outcome.yaml"), []byte("outcome: done\nstuck_reason: null\nbatches_done: 1\n"), 0o644); err != nil {
				t.Fatalf("write outcome.yaml: %v", err)
			}
			// summary.md deliberately never written.
		},
	}
	fx.Starter.handle = handle
	seedShuttleRunState(t, fx.ShuttleRunRoot, "master-strand-nosum", "master-session-nosum")

	_, err := websterengine.Run(fx.Deps, websterengine.RunOptions{})
	if err == nil {
		t.Fatalf("Run() error = nil; want a hard error for a done outcome with a missing summary.md")
	}
	if !strings.Contains(err.Error(), "summary") {
		t.Errorf("Run() error = %q; want it to name the missing summary", err.Error())
	}
}

// TestRun_DoneWithUnrecordedBatchIsHardError proves the every-batch-done gate: a Master that writes
// outcome: done while a plan batch has no terminal done record (begun-but-never-recorded — a fork
// that slipped past record-batch) is a hard error naming the offending batch, even when the
// outcome/summary files are well-formed and the whole-session audit is clean (round fable-r1's
// F11).
func TestRun_DoneWithUnrecordedBatchIsHardError(t *testing.T) {
	fx := newRunFixture(t, 2)

	// Batch 1 recorded done; batch 2 was begun but never recorded terminal.
	seedMatchingState(t, fx, &websterengine.State{
		Batches: map[int]*websterengine.BatchState{
			1: {Slug: "batch1", Kind: "fork", Terminal: true, Status: "done", SessionID: "master-session-partial"},
			2: {Slug: "batch2", Kind: "fork", Terminal: false, SessionID: "master-session-partial"},
		},
	})

	handle := &runFakeHandle{
		strandGUID: "master-strand-partial",
		result: shuttleengine.Result{
			Outcome:   shuttleengine.OutcomeDone,
			SessionID: "master-session-partial",
			RunDir:    "/run/dir/partial",
			ForkAudit: &shuttleengine.ForkAudit{
				Forks: []shuttleengine.ForkReport{
					{TranscriptPath: "/transcripts/fork1.jsonl", ReportReturned: true},
					{TranscriptPath: "/transcripts/fork2.jsonl", ReportReturned: true},
				},
			},
		},
		onWait: func() {
			if err := os.WriteFile(filepath.Join(fx.Deps.Geom.WebsterDir, "outcome.yaml"), []byte("outcome: done\nstuck_reason: null\nbatches_done: 2\n"), 0o644); err != nil {
				t.Fatalf("write outcome.yaml: %v", err)
			}
			if err := os.WriteFile(filepath.Join(fx.Deps.Geom.WebsterDir, "summary.md"), []byte("# Claimed done\n\nPremature.\n"), 0o644); err != nil {
				t.Fatalf("write summary.md: %v", err)
			}
		},
	}
	fx.Starter.handle = handle
	seedShuttleRunState(t, fx.ShuttleRunRoot, "master-strand-partial", "master-session-partial")

	_, err := websterengine.Run(fx.Deps, websterengine.RunOptions{})
	if err == nil {
		t.Fatal("Run() = nil error; want a hard error for a done outcome with a batch lacking a terminal done record")
	}
	if !strings.Contains(err.Error(), "terminal done record") {
		t.Errorf("Run() error = %q; want the every-batch-done gate message", err.Error())
	}
}

// TestRun_DoneWithParentWriteViolationIsHardError proves the run-exit audit cross-check's
// CheckParent pass fires on a done outcome: a parent write outside the two contract files is a hard
// error carried on the run's own error, even though the outcome/summary files themselves are
// well-formed.
func TestRun_DoneWithParentWriteViolationIsHardError(t *testing.T) {
	fx := newRunFixture(t, 1)

	// Seed batch 1 as terminal done (recorded under the run's own Master
	// session) so the run reaches the whole-session audit cross-check rather
	// than tripping the every-batch-done gate first.
	seedMatchingState(t, fx, &websterengine.State{
		Batches: map[int]*websterengine.BatchState{
			1: {Slug: "batch1", Kind: "fork", Terminal: true, Status: "done", SessionID: "master-session-violation"},
		},
	})

	handle := &runFakeHandle{
		strandGUID: "master-strand-violation",
		result: shuttleengine.Result{
			Outcome:   shuttleengine.OutcomeDone,
			SessionID: "master-session-violation",
			RunDir:    "/run/dir/violation",
			ForkAudit: &shuttleengine.ForkAudit{
				ParentWrites: []string{"/somewhere/else/hand-written-report.yaml"},
				Forks: []shuttleengine.ForkReport{
					{TranscriptPath: "/transcripts/fork1.jsonl", ReportReturned: true},
				},
			},
		},
		onWait: func() {
			if err := os.WriteFile(filepath.Join(fx.Deps.Geom.WebsterDir, "outcome.yaml"), []byte("outcome: done\nstuck_reason: null\nbatches_done: 1\n"), 0o644); err != nil {
				t.Fatalf("write outcome.yaml: %v", err)
			}
			if err := os.WriteFile(filepath.Join(fx.Deps.Geom.WebsterDir, "summary.md"), []byte("# Shipped batch1\n"), 0o644); err != nil {
				t.Fatalf("write summary.md: %v", err)
			}
		},
	}
	fx.Starter.handle = handle
	seedShuttleRunState(t, fx.ShuttleRunRoot, "master-strand-violation", "master-session-violation")

	_, err := websterengine.Run(fx.Deps, websterengine.RunOptions{})
	if err == nil {
		t.Fatalf("Run() error = nil; want a hard error for a parent-write violation")
	}
	if !strings.Contains(err.Error(), "parent-write") {
		t.Errorf("Run() error = %q; want it to name the parent-write violation", err.Error())
	}
}

// TestRun_MasterNonDoneOutcomesMapToTypedErrors proves each of the asking/died/timeout shuttle
// outcomes for Master's own spawn maps to its own distinct *Master*Error type, carrying SessionID
// and the kept RunDir, and matches its own sentinel via errors.Is — never attempting to parse a
// (non-existent) outcome.yaml.
func TestRun_MasterNonDoneOutcomesMapToTypedErrors(t *testing.T) {
	tests := []struct {
		name    string
		outcome shuttleengine.Outcome
		check   func(t *testing.T, err error, wantSessionID, wantRunDir string)
	}{
		{
			name:    "asking",
			outcome: shuttleengine.OutcomeAsking,
			check: func(t *testing.T, err error, wantSessionID, wantRunDir string) {
				var target *websterengine.MasterAskingError
				if !errors.As(err, &target) {
					t.Fatalf("Run() error = %v; want a *MasterAskingError", err)
				}
				if target.SessionID != wantSessionID || target.RunDir != wantRunDir {
					t.Errorf("MasterAskingError = %+v; want session %q, run dir %q", target, wantSessionID, wantRunDir)
				}
				if !errors.Is(err, websterengine.ErrMasterAsking) {
					t.Error("errors.Is(err, ErrMasterAsking) = false; want true")
				}
			},
		},
		{
			name:    "died",
			outcome: shuttleengine.OutcomeDied,
			check: func(t *testing.T, err error, wantSessionID, wantRunDir string) {
				var target *websterengine.MasterDiedError
				if !errors.As(err, &target) {
					t.Fatalf("Run() error = %v; want a *MasterDiedError", err)
				}
				if target.SessionID != wantSessionID || target.RunDir != wantRunDir {
					t.Errorf("MasterDiedError = %+v; want session %q, run dir %q", target, wantSessionID, wantRunDir)
				}
				if !errors.Is(err, websterengine.ErrMasterDied) {
					t.Error("errors.Is(err, ErrMasterDied) = false; want true")
				}
			},
		},
		{
			name:    "timeout",
			outcome: shuttleengine.OutcomeTimeout,
			check: func(t *testing.T, err error, wantSessionID, wantRunDir string) {
				var target *websterengine.MasterTimeoutError
				if !errors.As(err, &target) {
					t.Fatalf("Run() error = %v; want a *MasterTimeoutError", err)
				}
				if target.SessionID != wantSessionID || target.RunDir != wantRunDir {
					t.Errorf("MasterTimeoutError = %+v; want session %q, run dir %q", target, wantSessionID, wantRunDir)
				}
				if !errors.Is(err, websterengine.ErrMasterTimeout) {
					t.Error("errors.Is(err, ErrMasterTimeout) = false; want true")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fx := newRunFixture(t, 1)

			wantSessionID := "master-session-" + tt.name
			wantRunDir := "/run/dir/" + tt.name
			handle := &runFakeHandle{
				strandGUID: "master-strand-" + tt.name,
				result: shuttleengine.Result{
					Outcome:              tt.outcome,
					SessionID:            wantSessionID,
					RunDir:               wantRunDir,
					LastAssistantMessage: "why do you ask?",
				},
			}
			fx.Starter.handle = handle
			seedShuttleRunState(t, fx.ShuttleRunRoot, "master-strand-"+tt.name, wantSessionID)

			_, err := websterengine.Run(fx.Deps, websterengine.RunOptions{})
			tt.check(t, err, wantSessionID, wantRunDir)
		})
	}
}

// TestRun_PausedOutcomeLeavesPauseFlagIntact proves a genuinely mid-run pause request (one
// requested WHILE Master is working, i.e.
// present again by the time Master's own "outcome: paused" final action lands — Run's own pre-spawn
// commitment-point clear already ran before Master ever started) is left intact by the post-run
// mapping: the operator's own record that a pause is still pending, never silently cleared out from
// under them.
func TestRun_PausedOutcomeLeavesPauseFlagIntact(t *testing.T) {
	fx := newRunFixture(t, 1)

	handle := &runFakeHandle{
		strandGUID: "master-strand-paused",
		result: shuttleengine.Result{
			Outcome:   shuttleengine.OutcomeDone,
			SessionID: "master-session-paused",
			RunDir:    "/run/dir/paused",
		},
		onWait: func() {
			// Simulate an operator pausing DURING Master's own run: the
			// pre-spawn ClearPause already ran before Master started, so
			// this is a genuinely new request Master's own paused final
			// action is responding to.
			if err := websterengine.RequestPause(fx.Deps.Geom.ScratchDir); err != nil {
				t.Fatalf("RequestPause() error = %v", err)
			}
			if err := os.WriteFile(filepath.Join(fx.Deps.Geom.WebsterDir, "outcome.yaml"), []byte("outcome: paused\nstuck_reason: null\nbatches_done: 0\n"), 0o644); err != nil {
				t.Fatalf("write outcome.yaml: %v", err)
			}
			if err := os.WriteFile(filepath.Join(fx.Deps.Geom.WebsterDir, "summary.md"), []byte("# Paused mid-run\n"), 0o644); err != nil {
				t.Fatalf("write summary.md: %v", err)
			}
		},
	}
	fx.Starter.handle = handle
	seedShuttleRunState(t, fx.ShuttleRunRoot, "master-strand-paused", "master-session-paused")

	result, err := websterengine.Run(fx.Deps, websterengine.RunOptions{})
	if err != nil {
		t.Fatalf("Run() error = %v; want nil", err)
	}
	if result.Outcome != "paused" {
		t.Errorf("RunResult.Outcome = %q; want %q", result.Outcome, "paused")
	}
	if !websterengine.PauseRequested(fx.Deps.Geom.ScratchDir) {
		t.Error("pause flag cleared on a paused outcome; want it left intact as the operator's own record")
	}
}

// TestRun_NilOpenBisectorRecordsUnlocalizedIntegrationFailure pins the nil-bisector bypass: a
// hub-shaped fixture always supplies a non-nil OpenBisector, so this is the one case a hub-only
// fixture cannot exercise at all.
// Two batches, each contributing its own CardSHA, accumulate at least two card SHAs — the minimum
// that matters, since zero or one SHA both take bisect's own early-return shapes and would pass even
// under a broken implementation that pushed the nil check down into bisect/BisectAndEscalate instead
// of bypassing them at the call site; two or more is what would reach the branch call that
// nil-pointer panics on a nil bisector if the bypass were ever removed.
// Asserts the call does not panic, state.json and summary.md both carry the failure under
// "unknown"/"unknown" for the offending SHA and card, and RunResult.Warnings carries the
// standalone-mode explanation.
func TestRun_NilOpenBisectorRecordsUnlocalizedIntegrationFailure(t *testing.T) {
	fx := newRunFixture(t, 2)
	appendIntegrationVerify(t, fx.PlanDir, "true")
	fx.Deps.OpenBisector = nil

	sha1 := commitFile(t, fx.Worktree, "card1.txt", "one", "card1")
	sha2 := commitFile(t, fx.Worktree, "card2.txt", "two", "card2")

	seedMatchingState(t, fx, &websterengine.State{
		Batches: map[int]*websterengine.BatchState{
			1: {Slug: "batch1", Kind: "fork", Terminal: true, Status: "done", CardSHAs: []string{sha1}},
			2: {Slug: "batch2", Kind: "fork", Terminal: true, Status: "done", CardSHAs: []string{sha2}},
		},
	})

	handle := &runFakeHandle{
		strandGUID: "master-strand-nilbisector",
		result: shuttleengine.Result{
			Outcome:   shuttleengine.OutcomeDone,
			SessionID: "master-session-nilbisector",
			RunDir:    "/run/dir/nilbisector",
			ForkAudit: &shuttleengine.ForkAudit{
				Forks: []shuttleengine.ForkReport{
					{TranscriptPath: "/transcripts/fork1.jsonl", ReportReturned: true},
					{TranscriptPath: "/transcripts/fork2.jsonl", ReportReturned: true},
				},
			},
		},
		onWait: func() {
			reportPath := websterengine.IntegrationReportPath(fx.Deps.Geom.ReportsDir)
			if err := os.WriteFile(reportPath, []byte("status: FAILED\nhead_sha: "+sha2+"\ndeviations: []\n"), 0o644); err != nil {
				t.Fatalf("write integration report: %v", err)
			}
			if err := os.WriteFile(filepath.Join(fx.Deps.Geom.WebsterDir, "outcome.yaml"), []byte("outcome: stuck\nstuck_reason: \"integration suite failed\"\nbatches_done: 2\n"), 0o644); err != nil {
				t.Fatalf("write outcome.yaml: %v", err)
			}
			if err := os.WriteFile(filepath.Join(fx.Deps.Geom.WebsterDir, "summary.md"), []byte("# Batches shipped\n\nBoth batches landed; integration failed.\n"), 0o644); err != nil {
				t.Fatalf("write summary.md: %v", err)
			}
		},
	}
	fx.Starter.handle = handle
	seedShuttleRunState(t, fx.ShuttleRunRoot, "master-strand-nilbisector", "master-session-nilbisector")

	result, err := websterengine.Run(fx.Deps, websterengine.RunOptions{})
	if err != nil {
		t.Fatalf("Run() error = %v; want nil (a FAILED integration report under a nil OpenBisector is escalated as unlocalized, not a Run() error)", err)
	}

	st, loadErr := websterengine.LoadState(fx.Deps.Geom.WebsterDir, fx.Deps.Geom.ScratchDir)
	if loadErr != nil {
		t.Fatalf("LoadState() error = %v", loadErr)
	}
	escalated, ok := st.Batches[-1]
	if !ok || escalated == nil {
		t.Fatalf("state.json carries no integration escalation record; want one at the reserved key")
	}
	if escalated.Slug != "unknown" {
		t.Errorf("escalated record Slug = %q; want %q (no fabric repo to localize with)", escalated.Slug, "unknown")
	}
	if escalated.Digest == nil || escalated.Digest.HeadSHA != "unknown" {
		t.Errorf("escalated record digest = %+v; want head_sha %q", escalated.Digest, "unknown")
	}

	summaryData, err := os.ReadFile(filepath.Join(fx.Deps.Geom.WebsterDir, "summary.md"))
	if err != nil {
		t.Fatalf("read summary.md: %v", err)
	}
	if !strings.Contains(string(summaryData), "unknown") {
		t.Errorf("summary.md does not name the unlocalized \"unknown\" card; got:\n%s", summaryData)
	}

	found := false
	for _, w := range result.Warnings {
		if strings.Contains(w, "no fabric repo to bisect against") {
			found = true
		}
	}
	if !found {
		t.Errorf("RunResult.Warnings = %v; want one explaining the unlocalized failure (no fabric repo to bisect against)", result.Warnings)
	}
}

// runToDone drives fx's Run to a done outcome with the given ForkAudit forks, scripting
// outcome.yaml/summary.md the same way TestRun_DoneOutcomeWithValidSummaryAndCleanAuditPopulatesResult
// does, and returns the RunResult.
func runToDone(t *testing.T, fx *runFixture, strandGUID, sessionID string, forks []shuttleengine.ForkReport, batchesDone int) websterengine.RunResult {
	t.Helper()

	handle := &runFakeHandle{
		strandGUID: strandGUID,
		result: shuttleengine.Result{
			Outcome:   shuttleengine.OutcomeDone,
			SessionID: sessionID,
			RunDir:    "/run/dir/" + sessionID,
			ForkAudit: &shuttleengine.ForkAudit{Forks: forks},
		},
		onWait: func() {
			outcome := fmt.Sprintf("outcome: done\nstuck_reason: null\nbatches_done: %d\n", batchesDone)
			if err := os.WriteFile(filepath.Join(fx.Deps.Geom.WebsterDir, "outcome.yaml"), []byte(outcome), 0o644); err != nil {
				t.Fatalf("write outcome.yaml: %v", err)
			}
			if err := os.WriteFile(filepath.Join(fx.Deps.Geom.WebsterDir, "summary.md"), []byte("# Shipped\n\nAll good.\n"), 0o644); err != nil {
				t.Fatalf("write summary.md: %v", err)
			}
		},
	}
	fx.Starter.handle = handle
	seedShuttleRunState(t, fx.ShuttleRunRoot, strandGUID, sessionID)

	result, err := websterengine.Run(fx.Deps, websterengine.RunOptions{})
	if err != nil {
		t.Fatalf("Run() error = %v; want nil", err)
	}
	return result
}

// TestRun_ReorderingIsObservableInMasterPrompt proves card 4's sequencing is load-bearing on the
// prompt Run hands Master: a two-card fixture where card 1 Uses card 2's own Create target makes
// batch 2 the execution predecessor of batch 1, so the rendered Master prompt's {{.batch_index}}
// region lists batch 02 above batch 01.
func TestRun_ReorderingIsObservableInMasterPrompt(t *testing.T) {
	fx := newRunFixture(t, 2)

	// Card 1 Uses card 2's own Create target, so batch 2 must run before
	// batch 1. The path is a Create target of card 2 within the same plan
	// (satisfied by planparser's own createTargetsUnion check), but this also
	// creates the file for real under the worktree, mirroring a genuine
	// cross-card file dependency.
	addCardUses(t, fx.PlanDir, 1, "internal/batch2/new.go")
	if err := os.MkdirAll(filepath.Join(fx.Worktree, "internal", "batch2"), 0o755); err != nil {
		t.Fatalf("mkdir internal/batch2: %v", err)
	}
	if err := os.WriteFile(filepath.Join(fx.Worktree, "internal", "batch2", "new.go"), []byte("package batch2\n"), 0o644); err != nil {
		t.Fatalf("seed internal/batch2/new.go: %v", err)
	}

	seedMatchingState(t, fx, &websterengine.State{
		Batches: map[int]*websterengine.BatchState{
			1: {Slug: "batch1", Kind: "fork", Terminal: true, Status: "done"},
			2: {Slug: "batch2", Kind: "fork", Terminal: true, Status: "done"},
		},
	})

	runToDone(t, fx, "master-strand-reorder", "master-session-reorder", []shuttleengine.ForkReport{
		{TranscriptPath: "/transcripts/fork1.jsonl", ReportReturned: true},
		{TranscriptPath: "/transcripts/fork2.jsonl", ReportReturned: true},
	}, 2)

	if fx.Starter.callCount() != 1 {
		t.Fatalf("Starter.callCount() = %d; want 1", fx.Starter.callCount())
	}
	prompt := fx.Starter.startCalls[0].Prompt
	idx02 := strings.Index(prompt, "02 — batch2")
	idx01 := strings.Index(prompt, "01 — batch1")
	if idx02 == -1 || idx01 == -1 || idx02 >= idx01 {
		t.Errorf("rendered Master prompt does not list batch 02 above batch 01: idx02=%d idx01=%d\n%s", idx02, idx01, prompt)
	}
}

// TestRun_CycleReportingSurfacesOnRunResultButNeverFails proves a dependency cycle between two
// batches is condensed, reported on RunResult.Cycles, and surfaced as a warning — while the run
// still reaches its ordinary done outcome, since a cycle is never fatal and never changes the exit
// path.
func TestRun_CycleReportingSurfacesOnRunResultButNeverFails(t *testing.T) {
	fx := newRunFixture(t, 2)

	// Card 1 Uses card 2's target and card 2 Uses card 1's target: a mutual
	// dependency SequenceBatches condenses into one cycle.
	addCardUses(t, fx.PlanDir, 1, "internal/batch2/new.go")
	addCardUses(t, fx.PlanDir, 2, "internal/batch1/new.go")
	for _, slug := range []string{"batch1", "batch2"} {
		dir := filepath.Join(fx.Worktree, "internal", slug)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir internal/%s: %v", slug, err)
		}
		if err := os.WriteFile(filepath.Join(dir, "new.go"), []byte("package "+slug+"\n"), 0o644); err != nil {
			t.Fatalf("seed internal/%s/new.go: %v", slug, err)
		}
	}

	seedMatchingState(t, fx, &websterengine.State{
		Batches: map[int]*websterengine.BatchState{
			1: {Slug: "batch1", Kind: "fork", Terminal: true, Status: "done"},
			2: {Slug: "batch2", Kind: "fork", Terminal: true, Status: "done"},
		},
	})

	result := runToDone(t, fx, "master-strand-cycle", "master-session-cycle", []shuttleengine.ForkReport{
		{TranscriptPath: "/transcripts/fork1.jsonl", ReportReturned: true},
		{TranscriptPath: "/transcripts/fork2.jsonl", ReportReturned: true},
	}, 2)

	if result.Outcome != "done" {
		t.Errorf("RunResult.Outcome = %q; want %q (a cycle is never fatal)", result.Outcome, "done")
	}
	if len(result.Cycles) != 1 {
		t.Fatalf("RunResult.Cycles = %v; want exactly 1 cycle", result.Cycles)
	}
	if got := result.Cycles[0].Batches; len(got) != 2 || got[0] != 1 || got[1] != 2 {
		t.Errorf("RunResult.Cycles[0].Batches = %v; want [1 2]", got)
	}

	wantWarning := result.Cycles[0].Warning()
	found := false
	for _, w := range result.Warnings {
		if w == wantWarning {
			found = true
		}
	}
	if !found {
		t.Errorf("RunResult.Warnings = %v; want the cycle's own Warning() line %q", result.Warnings, wantWarning)
	}
}

// TestRun_AcyclicPlanReportsNoCycles proves the common case: an unmodified newRunFixture plan,
// whose cards reference nothing of each other's, produces an empty RunResult.Cycles and adds no
// sequencing warning.
func TestRun_AcyclicPlanReportsNoCycles(t *testing.T) {
	fx := newRunFixture(t, 2)

	seedMatchingState(t, fx, &websterengine.State{
		Batches: map[int]*websterengine.BatchState{
			1: {Slug: "batch1", Kind: "fork", Terminal: true, Status: "done"},
			2: {Slug: "batch2", Kind: "fork", Terminal: true, Status: "done"},
		},
	})

	result := runToDone(t, fx, "master-strand-acyclic", "master-session-acyclic", []shuttleengine.ForkReport{
		{TranscriptPath: "/transcripts/fork1.jsonl", ReportReturned: true},
		{TranscriptPath: "/transcripts/fork2.jsonl", ReportReturned: true},
	}, 2)

	if len(result.Cycles) != 0 {
		t.Errorf("RunResult.Cycles = %v; want empty for an acyclic plan", result.Cycles)
	}
	for _, w := range result.Warnings {
		if strings.Contains(w, "dependency cycle") {
			t.Errorf("RunResult.Warnings = %v; want no sequencing-cycle warning for an acyclic plan", result.Warnings)
		}
	}
}
