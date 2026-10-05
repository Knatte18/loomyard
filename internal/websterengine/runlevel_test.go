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
	"bytes"
	"encoding/json"
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
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/planglyph"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/plankit"
	"github.com/Knatte18/loomyard/internal/testkit/shuttlefake"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

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
// records every spec and gate StartMaster was called with and hands back the
// caller-scripted handle, or startErr when the caller wants to prove a step
// never reaches the spawn at all.
type runFakeStarter struct {
	mu         sync.Mutex
	startCalls []shuttleengine.Spec
	gateCalls  []shuttleengine.GateSpec
	handle     websterengine.MasterHandle
	startErr   error
}

func (s *runFakeStarter) StartMaster(spec shuttleengine.Spec, gate shuttleengine.GateSpec) (websterengine.MasterHandle, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.startCalls = append(s.startCalls, spec)
	s.gateCalls = append(s.gateCalls, gate)
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
// so the verify gate passes without running anything for every fixture built on this helper;
// a test that needs one calls appendIntegrationVerify against an already-seeded plan dir.
// numCards == 0 yields a "## Card Index" section with no entries at all,
// which ParsePlan's own parseCardIndex refuses loud ("no card index entries
// found") — the vehicle for the zero-batch refusal test, which under the
// flat model is actually the batchifier-derived-zero-batches refusal (an
// empty Cards list, if it ever parsed, would batchify to zero batches too).
func seedRunPlanDir(t *testing.T, numCards int) string {
	t.Helper()
	dir := t.TempDir()
	plankit.Write(t, dir, runPlan(1, numCards))
	return dir
}

// runPlan returns a validation-clean plan of numCards cards numbered from first, each a Create of its own new file.
func runPlan(first, numCards int) plankit.Plan {
	p := plankit.Plan{Approved: true, Framing: "Framing."}
	if first != 1 {
		p.FirstCard = first
	}
	for n := first; n < first+numCards; n++ {
		slug := fmt.Sprintf("batch%d", n)
		p.Cards = append(p.Cards, plankit.Card{
			Number:  n,
			Slug:    slug,
			Summary: fmt.Sprintf("placeholder card %d", n),
			Groups:  []plankit.Group{{Label: "Create", Targets: []string{fmt.Sprintf("internal/%s/new.go", slug)}}},
			Intent:  "placeholder card.",
		})
	}
	return p
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
	Reed           *shuttlefake.Reed
	Starter        *runFakeStarter
	Worktree       string
	PlanDir        string
	ShuttleRunRoot string
}

func newRunFixture(t *testing.T, numCards int) *runFixture {
	t.Helper()

	planDir := seedRunPlanDir(t, numCards)
	worktree := newScratchRepo(t)
	gitkit.CommitFile(t, worktree, "base.txt", "base", "base commit")

	// Run never registers a strand itself, so a stray AddStrand fails loud.
	reed := &shuttlefake.Reed{AddErr: errors.New("AddStrand is not used by Run's own path")}
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

	deps := websterengine.RunDeps{
		Starter:    starter,
		Reed:       reed,
		Engine:     &shuttlefake.Engine{},
		ShuttleCfg: shuttleCfg,
		Roles:      roles,
		Batcher:    activeBatcher,
		Config: websterengine.Config{
			SelfFixCap:         2,
			MasterTimeoutMin:   480,
			VerifyGateAttempts: 3,
		},
		Geom: websterengine.Geometry{
			AnchorRoot:   worktree,
			WorktreeRoot: worktree,
			VerifyDir:    t.TempDir(),
			WebsterDir:   t.TempDir(),
			ScratchDir:   t.TempDir(),
			ReportsDir:   t.TempDir(),
			PromptsDir:   t.TempDir(),
			StencilsDir:  fabricengine.StencilsDir(hubPath),
			PlanDir:      planDir,
		},
		RefMatcher: websterengine.NeverMatches{},
	}

	return &runFixture{Deps: deps, Reed: reed, Starter: starter, Worktree: worktree, PlanDir: planDir, ShuttleRunRoot: shuttleRunRoot}
}

// appendIntegrationVerify appends a plan-level "## verify:" section to the overview of the already-seeded plan dir at planDir,
// so a fixture built without one can exercise the plan-level verify gate.
func appendIntegrationVerify(t *testing.T, planDir, verify string) {
	t.Helper()
	path := filepath.Join(planDir, "00-overview.md")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read overview fixture: %v", err)
	}
	data = append(data, []byte("\n## verify:\n\n"+verify+"\n")...)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write overview fixture with verify: %v", err)
	}
}

// addCardUses rewrites an already-seeded card file under planDir — one of seedRunPlanDir's own
// "%02d-batch%d.md" files — to carry a "**Uses:**" field naming ref, modelled on
// appendIntegrationVerify: read the file, splice the field in, write it
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

// addCardCreateTarget rewrites an already-seeded card file under planDir to add target as an
// additional bullet under the card's existing "**Create:**" group, ahead of its own
// seedRunPlanDir-written path target -- so a test can add a glyph-shaped Create target alongside
// the plain-path one every seedRunPlanDir card already carries, without disturbing it.
func addCardCreateTarget(t *testing.T, planDir string, cardNumber int, target string) {
	t.Helper()
	slug := fmt.Sprintf("batch%d", cardNumber)
	path := filepath.Join(planDir, fmt.Sprintf("%02d-%s.md", cardNumber, slug))
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read card fixture %s: %v", path, err)
	}
	const marker = "**Create:**\n"
	body := string(data)
	idx := strings.Index(body, marker)
	if idx == -1 {
		t.Fatalf("card fixture %s carries no **Create:** marker to splice an extra target under", path)
	}
	insertAt := idx + len(marker)
	newBody := body[:insertAt] + fmt.Sprintf("- `%s`\n", target) + body[insertAt:]
	if err := os.WriteFile(path, []byte(newBody), 0o644); err != nil {
		t.Fatalf("write card fixture with an extra Create target: %v", err)
	}
}

// seedMatchingState saves st into fx's webster dir after stamping its
// PlanFingerprint to match fx's own on-disk plan directory (and defaulting
// its Batches map when nil), so Run's own fingerprint gate passes and the
// pre-seeded state survives into the run unmodified.
func seedMatchingState(t *testing.T, fx *runFixture, st *websterengine.State) {
	t.Helper()
	st.PlanFingerprint = mustFingerprint(t, fx.PlanDir)
	if err := websterengine.RestampPlanBaseline(st, fx.PlanDir, fx.Deps.Geom.WebsterDir); err != nil {
		t.Fatalf("stamp plan file hashes: %v", err)
	}
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
	requireWayForward(t, err, "lyx webster status")

	// Taking the way forward: once the other run finishes and releases the lock, Run proceeds.
	held.Release()
	askingMaster(t, fx, "busy")
	_, err = websterengine.Run(fx.Deps, websterengine.RunOptions{})
	requireReachedMaster(t, fx, err)
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

// TestRun_BlockingGlyphFindingRefusesRun proves that a blocking planglyph finding -- glyph-not-found,
// from a Uses: entry naming a unit that exists but a member that does not -- refuses the run before
// ever spawning Master, exactly as the pre-existing planparser-only findings already did, matching
// the pre-flight gate this batch moves onto planglyph.
func TestRun_BlockingGlyphFindingRefusesRun(t *testing.T) {
	fx := newRunFixture(t, 1)
	gitkit.CommitFile(t, fx.Worktree, "sub/a.go", "package sub\n\nfunc Foo() {}\n", "add sub package")
	addCardUses(t, fx.PlanDir, 1, "sub#Missing")

	_, err := websterengine.Run(fx.Deps, websterengine.RunOptions{})
	if err == nil {
		t.Fatal("Run() error = nil; want a plan-validation refusal for the blocking glyph-not-found finding")
	}
	if !strings.Contains(err.Error(), "glyph-not-found") {
		t.Errorf("Run() error = %v; want it to name glyph-not-found", err)
	}
	if fx.Starter.callCount() != 0 {
		t.Errorf("Starter was reached (%d calls) for a blocking-findings plan; want zero", fx.Starter.callCount())
	}
}

// TestRun_InformationalFindingsDoNotRefuseRun proves that an informational-only findings set --
// create-new-unit, on a Create target introducing a brand-new package -- does not refuse the run:
// Run reaches the Master spawn exactly as it would for a plan carrying no findings at all, per the
// severity-decides-the-verdict rule this pre-flight gate shares with Plan-Write's and
// Plan-Burler's own gate and the validate-plan/validate CLI verbs.
func TestRun_InformationalFindingsDoNotRefuseRun(t *testing.T) {
	fx := newRunFixture(t, 1)
	gitkit.CommitFile(t, fx.Worktree, "sub/a.go", "package sub\n\nfunc Foo() {}\n", "add sub package")
	addCardCreateTarget(t, fx.PlanDir, 1, "newpkg#Bar")

	wantSessionID := "master-session-informational"
	wantRunDir := "/run/dir/informational"
	handle := &runFakeHandle{
		strandGUID: "master-strand-informational",
		result: shuttleengine.Result{
			Outcome:              shuttleengine.OutcomeAsking,
			SessionID:            wantSessionID,
			RunDir:               wantRunDir,
			LastAssistantMessage: "why do you ask?",
		},
	}
	fx.Starter.handle = handle
	seedShuttleRunState(t, fx.ShuttleRunRoot, "master-strand-informational", wantSessionID)

	_, err := websterengine.Run(fx.Deps, websterengine.RunOptions{})
	var target *websterengine.MasterAskingError
	if !errors.As(err, &target) {
		t.Fatalf("Run() error = %v; want a *MasterAskingError, proving the informational-only findings set never refused the run before the spawn", err)
	}
	if fx.Starter.callCount() != 1 {
		t.Errorf("Starter.callCount() = %d; want 1 -- an informational-only findings set must reach the Master spawn", fx.Starter.callCount())
	}
}

// TestRun_QuarryUnavailableRefusesRunNamingQuarry proves that a quarry-unavailable error -- an
// unopenable WorktreeRoot -- refuses the run before ever spawning Master, with an error message
// naming quarry rather than the plan, matching internal/loomshed's own plan gate (gates.go's NewPlanGate) producer-side
// disposition and internal/loomcli/validate.go and internal/webstercli/validate.go's CLI-side halves
// of this same parity.
func TestRun_QuarryUnavailableRefusesRunNamingQuarry(t *testing.T) {
	fx := newRunFixture(t, 1)
	fx.Deps.Geom.WorktreeRoot = filepath.Join(t.TempDir(), "does-not-exist")

	_, err := websterengine.Run(fx.Deps, websterengine.RunOptions{})
	if err == nil {
		t.Fatal("Run() error = nil; want a quarry-unavailable refusal")
	}
	if !strings.Contains(err.Error(), "quarry") {
		t.Errorf("Run() error = %v; want it to name quarry rather than the plan", err)
	}
	if fx.Starter.callCount() != 0 {
		t.Errorf("Starter was reached (%d calls) for a quarry-unavailable worktreeRoot; want zero", fx.Starter.callCount())
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
	requireWayForward(t, err, "lyx webster rebaseline", "lyx webster run --fresh")
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

	fx.Reed.Strands = []reedengine.StrandStatus{
		{GUID: "prior-master-strand", Live: true},
		{GUID: "prior-recovery-strand", Live: true},
		// "absent-recovery-strand" is deliberately absent from Status at all.
	}

	fx.Starter.startErr = fmt.Errorf("stop before spawn")

	_, err := websterengine.Run(fx.Deps, websterengine.RunOptions{})
	if err == nil {
		t.Fatalf("Run() error = nil; want the scripted starter error")
	}

	wantRemoved := map[string]bool{"prior-master-strand": true, "prior-recovery-strand": true}
	for _, guid := range fx.Reed.RemovedGUIDs {
		if guid == "absent-recovery-strand" {
			t.Errorf("RemoveStrand called for a cleanly-absent strand %q; want it left untouched", guid)
		}
		delete(wantRemoved, guid)
	}
	if len(wantRemoved) != 0 {
		t.Errorf("RemoveStrand calls = %v; missing %v", fx.Reed.RemovedGUIDs, wantRemoved)
	}
}

// TestRun_EntryTimeReclaimWithNoRecordedStrandRemovesNothing proves a state recording no strand removes nothing.
func TestRun_EntryTimeReclaimWithNoRecordedStrandRemovesNothing(t *testing.T) {
	fx := newRunFixture(t, 1)
	seedMatchingState(t, fx, &websterengine.State{})
	fx.Starter.startErr = fmt.Errorf("stop before spawn")

	if _, err := websterengine.Run(fx.Deps, websterengine.RunOptions{}); err == nil {
		t.Fatalf("Run() error = nil; want the scripted starter error")
	}

	if len(fx.Reed.RemovedGUIDs) != 0 {
		t.Errorf("RemoveStrand calls = %v; want none", fx.Reed.RemovedGUIDs)
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

// TestRun_MasterSpecCarriesWebsterStrandRole proves Merriam spawns under the strand role `webster`
// while the model still resolves from RoleMaster.
func TestRun_MasterSpecCarriesWebsterStrandRole(t *testing.T) {
	fx := newRunFixture(t, 1)

	fx.Starter.handle = &runFakeHandle{strandGUID: "master-strand-role", waitErr: fmt.Errorf("stop after spawn")}
	seedShuttleRunState(t, fx.ShuttleRunRoot, "master-strand-role", "master-session-role")

	if _, err := websterengine.Run(fx.Deps, websterengine.RunOptions{}); err == nil {
		t.Fatalf("Run() error = nil; want the scripted wait error")
	}

	if len(fx.Starter.startCalls) != 1 {
		t.Fatalf("StartMaster calls = %d; want 1", len(fx.Starter.startCalls))
	}
	spec := fx.Starter.startCalls[0]
	if spec.Role != websterengine.MerriamStrandRole || spec.Role == string(websterengine.RoleMaster) {
		t.Errorf("Spec.Role = %q; want %q, distinct from RoleMaster", spec.Role, websterengine.MerriamStrandRole)
	}
	if want := fx.Deps.Roles[websterengine.RoleMaster].Model; spec.Model != want {
		t.Errorf("Spec.Model = %q; want %q (RoleMaster's resolved model)", spec.Model, want)
	}
	if want := []string{"scribe:prose", "scribe:code-quality", "scribe:testing"}; !slices.Equal(spec.Skills, want) {
		t.Errorf("Spec.Skills = %v; want %v", spec.Skills, want)
	}
}

// expiredShellRun drives fx's Run to a done outcome whose Master result lists labels as expired shells.
func expiredShellRun(t *testing.T, fx *runFixture, labels []string) websterengine.RunResult {
	t.Helper()
	seedMatchingState(t, fx, &websterengine.State{
		Batches: map[int]*websterengine.BatchState{
			1: {Slug: "batch1", Kind: "fork", Terminal: true, Status: "done"},
		},
	})
	fx.Starter.handle = &runFakeHandle{
		strandGUID: "master-strand-shells",
		result: shuttleengine.Result{
			Outcome:       shuttleengine.OutcomeDone,
			SessionID:     "master-session-shells",
			RunDir:        "/run/dir/shells",
			ForkAudit:     &shuttleengine.ForkAudit{Forks: []shuttleengine.ForkReport{{TranscriptPath: "/transcripts/fork1.jsonl", ReportReturned: true}}},
			ExpiredShells: labels,
		},
		onWait: func() {
			writeDoneContract(t, fx)
		},
	}
	seedShuttleRunState(t, fx.ShuttleRunRoot, "master-strand-shells", "master-session-shells")

	result, err := websterengine.Run(fx.Deps, websterengine.RunOptions{})
	if err != nil {
		t.Fatalf("Run() error = %v; want nil", err)
	}
	return result
}

// TestRun_ExpiredShellYieldsWarningSummarySectionAndFrictionNote proves a waited-out shell on a done outcome warns, lands in summary.md and is named in a friction note.
func TestRun_ExpiredShellYieldsWarningSummarySectionAndFrictionNote(t *testing.T) {
	fx := newRunFixture(t, 1)
	fx.Deps.FrictionDir = t.TempDir()

	result := expiredShellRun(t, fx, []string{"sleep 9999"})

	want := "turn end counted after background shell `sleep 9999` ran past `background_shell_wait_min`; the shell may still be running in the session"
	if !slices.Contains(result.Warnings, want) {
		t.Errorf("Warnings = %v; want %q", result.Warnings, want)
	}
	summary, err := os.ReadFile(filepath.Join(fx.Deps.Geom.WebsterDir, "summary.md"))
	if err != nil {
		t.Fatalf("read summary.md: %v", err)
	}
	if !strings.Contains(string(summary), "## Background shells waited out") || !strings.Contains(string(summary), "- `sleep 9999`") {
		t.Errorf("summary.md = %q; want the waited-out section naming the shell", summary)
	}
	note, err := os.ReadFile(filepath.Join(fx.Deps.FrictionDir, "webster-background-shell.md"))
	if err != nil {
		t.Fatalf("read friction note: %v", err)
	}
	if !strings.Contains(string(note), "sleep 9999") || !strings.Contains(string(note), "background_shell_wait_min") {
		t.Errorf("friction note = %q; want the shell and the bound named", note)
	}
}

// TestRun_NoExpiredShellsWritesNothing proves an empty list adds no warning, summary section or friction note.
func TestRun_NoExpiredShellsWritesNothing(t *testing.T) {
	fx := newRunFixture(t, 1)
	fx.Deps.FrictionDir = t.TempDir()

	result := expiredShellRun(t, fx, nil)

	if warningsContain(result.Warnings, "background_shell_wait_min") {
		t.Errorf("Warnings = %v; want no expired-shell warning", result.Warnings)
	}
	summary, err := os.ReadFile(filepath.Join(fx.Deps.Geom.WebsterDir, "summary.md"))
	if err != nil {
		t.Fatalf("read summary.md: %v", err)
	}
	if strings.Contains(string(summary), "Background shells waited out") {
		t.Errorf("summary.md = %q; want no waited-out section", summary)
	}
	if _, err := os.Stat(filepath.Join(fx.Deps.FrictionDir, "webster-background-shell.md")); !os.IsNotExist(err) {
		t.Errorf("friction note stat error = %v; want not-exist", err)
	}
}

// TestRun_MasterSpecAwaitsRecoverBatchShell proves Master's spec declares the recover-batch shell as awaited.
func TestRun_MasterSpecAwaitsRecoverBatchShell(t *testing.T) {
	fx := newRunFixture(t, 1)

	fx.Starter.handle = &runFakeHandle{strandGUID: "master-strand-await", waitErr: fmt.Errorf("stop after spawn")}
	seedShuttleRunState(t, fx.ShuttleRunRoot, "master-strand-await", "master-session-await")

	if _, err := websterengine.Run(fx.Deps, websterengine.RunOptions{}); err == nil {
		t.Fatalf("Run() error = nil; want the scripted wait error")
	}

	got := fx.Starter.startCalls[0].AwaitedShellPrefixes
	if !slices.Equal(got, []string{"lyx webster recover-batch"}) {
		t.Errorf("Spec.AwaitedShellPrefixes = %q; want the recover-batch prefix", got)
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

// auditDoneHandle builds a done Master handle whose onWait writes outcome.yaml (batchesDone batches) and a valid summary.md, with audit as the whole-session fork audit.
func auditDoneHandle(t *testing.T, fx *runFixture, session string, batchesDone int, audit shuttleengine.ForkAudit, extra func()) *runFakeHandle {
	t.Helper()
	return &runFakeHandle{
		strandGUID: "master-strand-audit",
		result: shuttleengine.Result{
			Outcome:   shuttleengine.OutcomeDone,
			SessionID: session,
			RunDir:    "/run/dir/audit",
			ForkAudit: &audit,
		},
		onWait: func() {
			if extra != nil {
				extra()
			}
			outcome := fmt.Sprintf("outcome: done\nstuck_reason: null\nbatches_done: %d\n", batchesDone)
			if err := os.WriteFile(filepath.Join(fx.Deps.Geom.WebsterDir, "outcome.yaml"), []byte(outcome), 0o644); err != nil {
				t.Fatalf("write outcome.yaml: %v", err)
			}
			if err := os.WriteFile(filepath.Join(fx.Deps.Geom.WebsterDir, "summary.md"), []byte("# Shipped\n\nAll good.\n"), 0o644); err != nil {
				t.Fatalf("write summary.md: %v", err)
			}
		},
	}
}

// TestRun_DoneWithParentWriteToTrackedFileDemotesToStuck proves the run-exit audit demotes a done outcome to stuck on an undispositioned correctness finding — a Master write into a tracked file — naming the path and the git way forward,
// that the finding stays pending and refuses a bare re-run, and that once the file is restored with git and the finding accepted a re-run with a clean audit ends done.
func TestRun_DoneWithParentWriteToTrackedFileDemotesToStuck(t *testing.T) {
	fx := newRunFixture(t, 1)
	seedMatchingState(t, fx, &websterengine.State{
		Batches: map[int]*websterengine.BatchState{
			1: {Slug: "batch1", Kind: "fork", Terminal: true, Status: "done", SessionID: "master-session-violation",
				Digest: &websterengine.Digest{Status: "done", HeadSHA: gitkit.RevParse(t, fx.Worktree, "HEAD")}},
		},
	})
	tracked := filepath.Join(fx.Worktree, "base.txt")
	forks := []shuttleengine.ForkReport{{TranscriptPath: "/transcripts/fork1.jsonl", ReportReturned: true}}

	fx.Starter.handle = auditDoneHandle(t, fx, "master-session-violation", 1,
		shuttleengine.ForkAudit{ParentWrites: []string{tracked}, Forks: forks},
		func() {
			if err := os.WriteFile(tracked, []byte("hand-edited by master"), 0o644); err != nil {
				t.Fatalf("edit tracked file: %v", err)
			}
		})
	seedShuttleRunState(t, fx.ShuttleRunRoot, "master-strand-audit", "master-session-violation")

	result, err := websterengine.Run(fx.Deps, websterengine.RunOptions{})
	if err != nil {
		t.Fatalf("Run() error = %v; want nil (a correctness finding demotes, it is not an error)", err)
	}
	if result.Outcome != "stuck" {
		t.Fatalf("RunResult.Outcome = %q; want %q", result.Outcome, "stuck")
	}
	if !strings.Contains(result.StuckReason, tracked) || !strings.Contains(result.StuckReason, "git") {
		t.Errorf("StuckReason = %q; want it to name %s and the git way forward", result.StuckReason, tracked)
	}

	if !strings.Contains(result.StuckReason, "lyx webster accept-audit") {
		t.Errorf("StuckReason = %q; want it to name lyx webster accept-audit", result.StuckReason)
	}
	st, err := websterengine.LoadState(fx.Deps.Geom.WebsterDir, fx.Deps.Geom.ScratchDir)
	if err != nil {
		t.Fatalf("LoadState() error = %v", err)
	}
	if len(st.PendingAuditFindings) != 1 || len(st.PendingAuditFindings[0].Paths) != 1 || st.PendingAuditFindings[0].Paths[0] != tracked {
		t.Fatalf("PendingAuditFindings = %+v; want one finding naming %s", st.PendingAuditFindings, tracked)
	}

	// A bare re-step without accepting is refused and spawns no Master.
	before := fx.Starter.callCount()
	_, err = websterengine.Run(fx.Deps, websterengine.RunOptions{})
	if !errors.Is(err, websterengine.ErrPendingAuditFindings) {
		t.Fatalf("second Run() error = %v; want ErrPendingAuditFindings", err)
	}
	if !strings.Contains(err.Error(), tracked) || !strings.Contains(err.Error(), "lyx webster accept-audit") {
		t.Errorf("second Run() error = %q; want it to name %s and lyx webster accept-audit", err, tracked)
	}
	if got := fx.Starter.callCount(); got != before {
		t.Errorf("Starter calls = %d after the refused Run; want %d", got, before)
	}

	if _, _, err := websterengine.AcceptPendingAudit(nil, st, fx.Deps.Geom, nil); !errors.Is(err, websterengine.ErrAuditNotAcceptable) || !strings.Contains(err.Error(), tracked) {
		t.Fatalf("AcceptPendingAudit() before the revert error = %v; want ErrAuditNotAcceptable naming %s", err, tracked)
	}
	gitkit.Git(t, fx.Worktree, "checkout", "--", "base.txt")
	if _, _, err := websterengine.AcceptPendingAudit(nil, st, fx.Deps.Geom, nil); err != nil {
		t.Fatalf("AcceptPendingAudit() after the revert error = %v; want nil", err)
	}
	if err := websterengine.SaveState(fx.Deps.Geom.WebsterDir, fx.Deps.Geom.ScratchDir, st); err != nil {
		t.Fatalf("SaveState() error = %v", err)
	}
	fx.Starter.handle = auditDoneHandle(t, fx, "master-session-violation", 1, shuttleengine.ForkAudit{Forks: forks}, nil)
	result, err = websterengine.Run(fx.Deps, websterengine.RunOptions{})
	if err != nil {
		t.Fatalf("third Run() error = %v; want nil", err)
	}
	if result.Outcome != "done" {
		t.Errorf("third RunResult.Outcome = %q; want %q after the file was restored and the finding accepted", result.Outcome, "done")
	}
	st, err = websterengine.LoadState(fx.Deps.Geom.WebsterDir, fx.Deps.Geom.ScratchDir)
	if err != nil {
		t.Fatalf("LoadState() error = %v", err)
	}
	if len(st.PendingAuditFindings) != 0 {
		t.Errorf("PendingAuditFindings = %+v; want none", st.PendingAuditFindings)
	}
}

// TestRun_DoneWithNamedSpawnAlreadyDispositionedAddsNoWarning proves a finding an earlier record-batch already warned on is dropped by the run-exit audit:
// the run ends done, no second warning is recorded, and summary.md carries the batch-level warning exactly once under "## Audit warnings".
func TestRun_DoneWithNamedSpawnAlreadyDispositionedAddsNoWarning(t *testing.T) {
	const session = "master-session-spawn"
	fx := newRunFixture(t, 3)
	id := session + "/parent:named-spawn:1"
	seedMatchingState(t, fx, &websterengine.State{
		AuditDispositions: map[string]string{id: "warned"},
		Batches: map[int]*websterengine.BatchState{
			1: {Slug: "batch1", Kind: "fork", Terminal: true, Status: "done", SessionID: session},
			2: {Slug: "batch2", Kind: "fork", Terminal: true, Status: "done", SessionID: session,
				AuditWarnings: []websterengine.AuditWarning{{Identity: id, Class: "named-spawn", Detail: "master spawned a named agent"}}},
			3: {Slug: "batch3", Kind: "fork", Terminal: true, Status: "done", SessionID: session},
		},
	})
	forks := []shuttleengine.ForkReport{
		{TranscriptPath: "/transcripts/f1.jsonl", ReportReturned: true},
		{TranscriptPath: "/transcripts/f2.jsonl", ReportReturned: true},
		{TranscriptPath: "/transcripts/f3.jsonl", ReportReturned: true},
	}
	fx.Starter.handle = auditDoneHandle(t, fx, session, 3, shuttleengine.ForkAudit{NamedSpawns: 1, Forks: forks}, nil)
	seedShuttleRunState(t, fx.ShuttleRunRoot, "master-strand-audit", session)

	result, err := websterengine.Run(fx.Deps, websterengine.RunOptions{})
	if err != nil {
		t.Fatalf("Run() error = %v; want nil", err)
	}
	if result.Outcome != "done" {
		t.Fatalf("RunResult.Outcome = %q; want %q", result.Outcome, "done")
	}
	if warningsContain(result.Warnings, "named-spawn") {
		t.Errorf("Warnings = %v; want no second named-spawn warning", result.Warnings)
	}
	st, err := websterengine.LoadState(fx.Deps.Geom.WebsterDir, fx.Deps.Geom.ScratchDir)
	if err != nil {
		t.Fatalf("LoadState() error = %v", err)
	}
	if len(st.AuditWarnings) != 0 {
		t.Errorf("run-level AuditWarnings = %v; want none", st.AuditWarnings)
	}
	summary, err := os.ReadFile(filepath.Join(fx.Deps.Geom.WebsterDir, "summary.md"))
	if err != nil {
		t.Fatalf("read summary.md: %v", err)
	}
	if n := strings.Count(string(summary), "## Audit warnings"); n != 1 {
		t.Errorf("summary.md carries %d Audit warnings section(s); want 1:\n%s", n, summary)
	}
	if n := strings.Count(string(summary), "master spawned a named agent"); n != 1 {
		t.Errorf("summary.md carries the batch 2 warning %d times; want once:\n%s", n, summary)
	}
}

// TestRun_DoneWithNestedAgentInFixerForkWarns proves a policy finding in the fixer fork's transcript leaves the run done, records one run-level warning in state.json, returns it on RunResult.Warnings, and lists it in summary.md's "Audit warnings" section.
func TestRun_DoneWithNestedAgentInFixerForkWarns(t *testing.T) {
	const session = "master-session-nested"
	fx := newRunFixture(t, 1)
	appendIntegrationVerify(t, fx.PlanDir, "true")
	seedMatchingState(t, fx, &websterengine.State{
		Batches: map[int]*websterengine.BatchState{
			1: {Slug: "batch1", Kind: "fork", Terminal: true, Status: "done", SessionID: session, CardSHAs: []string{"deadbeef"}},
		},
	})
	forks := []shuttleengine.ForkReport{
		{TranscriptPath: "/transcripts/fork1.jsonl", ReportReturned: true},
		{TranscriptPath: "/transcripts/fixer.jsonl", ReportReturned: true, AgentCalls: 1},
	}
	fx.Starter.handle = auditDoneHandle(t, fx, session, 1, shuttleengine.ForkAudit{Forks: forks}, func() {})
	seedShuttleRunState(t, fx.ShuttleRunRoot, "master-strand-audit", session)

	result, err := websterengine.Run(fx.Deps, websterengine.RunOptions{})
	if err != nil {
		t.Fatalf("Run() error = %v; want nil", err)
	}
	if result.Outcome != "done" {
		t.Fatalf("RunResult.Outcome = %q; want %q", result.Outcome, "done")
	}
	if !warningsContain(result.Warnings, "audit warning (nested-agent)") {
		t.Errorf("Warnings = %v; want the nested-agent warning", result.Warnings)
	}
	st, err := websterengine.LoadState(fx.Deps.Geom.WebsterDir, fx.Deps.Geom.ScratchDir)
	if err != nil {
		t.Fatalf("LoadState() error = %v", err)
	}
	if len(st.AuditWarnings) != 1 {
		t.Fatalf("run-level AuditWarnings = %v; want exactly one", st.AuditWarnings)
	}
	summary, err := os.ReadFile(filepath.Join(fx.Deps.Geom.WebsterDir, "summary.md"))
	if err != nil {
		t.Fatalf("read summary.md: %v", err)
	}
	if !strings.Contains(string(summary), "## Audit warnings") || !strings.Contains(string(summary), "nested-agent") {
		t.Errorf("summary.md = %q; want an Audit warnings section naming the nested-agent finding", summary)
	}
}

// TestRun_ForkStateWriteAtRunExit proves the run-exit audit leaves a pending fork-state-write finding for a fork writing state.json,
// and none for a fork writing only its own batch's report.
func TestRun_ForkStateWriteAtRunExit(t *testing.T) {
	tests := []struct {
		name        string
		write       func(geom websterengine.Geometry) string
		wantPending bool
	}{
		{"state.json", func(g websterengine.Geometry) string { return filepath.Join(g.WebsterDir, "state.json") }, true},
		{"own report", func(g websterengine.Geometry) string {
			return filepath.Join(g.ReportsDir, websterengine.ReportFileName(1, "batch1"))
		}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			const session = "master-session-statewrite"
			fx := newRunFixture(t, 1)
			appendIntegrationVerify(t, fx.PlanDir, "true")
			seedMatchingState(t, fx, &websterengine.State{
				Batches: map[int]*websterengine.BatchState{
					1: {Slug: "batch1", Kind: "fork", Terminal: true, Status: "done", SessionID: session, CardSHAs: []string{"deadbeef"}, ForkTranscripts: []string{"/transcripts/fork1.jsonl"}},
				},
			})
			forks := []shuttleengine.ForkReport{
				{TranscriptPath: "/transcripts/fork1.jsonl", ReportReturned: true, WritePaths: []string{tt.write(fx.Deps.Geom)}},
				{TranscriptPath: "/transcripts/fixer.jsonl", ReportReturned: true},
			}
			fx.Starter.handle = auditDoneHandle(t, fx, session, 1, shuttleengine.ForkAudit{Forks: forks}, func() {})
			seedShuttleRunState(t, fx.ShuttleRunRoot, "master-strand-audit", session)

			result, err := websterengine.Run(fx.Deps, websterengine.RunOptions{})
			if err != nil {
				t.Fatalf("Run() error = %v; want nil", err)
			}
			st, err := websterengine.LoadState(fx.Deps.Geom.WebsterDir, fx.Deps.Geom.ScratchDir)
			if err != nil {
				t.Fatalf("LoadState() error = %v", err)
			}
			if tt.wantPending {
				if len(st.PendingAuditFindings) != 1 || st.PendingAuditFindings[0].Class != "fork-state-write" {
					t.Errorf("PendingAuditFindings = %+v; want one fork-state-write finding", st.PendingAuditFindings)
				}
				// state.json is under _lyx, which nothing the run recorded can check, so the way forward is the --fresh route and never a git restore.
				_, way, _ := strings.Cut(result.StuckReason, "way forward:")
				if !strings.Contains(way, "lyx webster run --fresh") || strings.Contains(way, "accept-audit") {
					t.Errorf("StuckReason = %q; want the --fresh route without accept-audit", result.StuckReason)
				}
			} else if len(st.PendingAuditFindings) != 0 {
				t.Errorf("PendingAuditFindings = %+v; want none", st.PendingAuditFindings)
			}
		})
	}
}

// TestRun_FixerForkPlanWriteIsFlagged proves the run-exit fork audit covers the verify-gate fixer fork:
// a fork outside every batch bracket that writes the plan directory leaves one pending fork-plan-write finding.
func TestRun_FixerForkPlanWriteIsFlagged(t *testing.T) {
	const session = "master-session-fixer"
	fx := newRunFixture(t, 1)
	seedMatchingState(t, fx, &websterengine.State{
		Batches: map[int]*websterengine.BatchState{
			1: {Slug: "batch1", Kind: "fork", Terminal: true, Status: "done", SessionID: session, CardSHAs: []string{"deadbeef"}, ForkTranscripts: []string{"/transcripts/fork1.jsonl"}},
		},
	})
	planFile := filepath.Join(fx.Deps.Geom.PlanDir, "00-overview.md")
	forks := []shuttleengine.ForkReport{
		{TranscriptPath: "/transcripts/fork1.jsonl", ReportReturned: true},
		{TranscriptPath: "/transcripts/fixer.jsonl", ReportReturned: true, WritePaths: []string{planFile}},
	}
	fx.Starter.handle = auditDoneHandle(t, fx, session, 1, shuttleengine.ForkAudit{Forks: forks}, func() {})
	seedShuttleRunState(t, fx.ShuttleRunRoot, "master-strand-audit", session)

	result, err := websterengine.Run(fx.Deps, websterengine.RunOptions{})
	if err != nil {
		t.Fatalf("Run() error = %v; want nil (a correctness finding demotes, it is not an error)", err)
	}
	if result.Outcome != "stuck" {
		t.Fatalf("RunResult.Outcome = %q; want %q", result.Outcome, "stuck")
	}
	st, err := websterengine.LoadState(fx.Deps.Geom.WebsterDir, fx.Deps.Geom.ScratchDir)
	if err != nil {
		t.Fatalf("LoadState() error = %v", err)
	}
	if len(st.PendingAuditFindings) != 1 || st.PendingAuditFindings[0].Class != "fork-plan-write" {
		t.Errorf("PendingAuditFindings = %+v; want one fork-plan-write finding", st.PendingAuditFindings)
	}
}

// TestRun_RendersVerifyFixPrompt proves Run writes the fixer prompt naming the gate report and Merriam's prompt names that file.
func TestRun_RendersVerifyFixPrompt(t *testing.T) {
	const session = "master-session-fixprompt"
	fx := newRunFixture(t, 1)
	seedMatchingState(t, fx, &websterengine.State{
		Batches: map[int]*websterengine.BatchState{
			1: {Slug: "batch1", Kind: "fork", Terminal: true, Status: "done", SessionID: session, CardSHAs: []string{"deadbeef"}},
		},
	})
	forks := []shuttleengine.ForkReport{{TranscriptPath: "/transcripts/fork1.jsonl", ReportReturned: true}}
	fx.Starter.handle = auditDoneHandle(t, fx, session, 1, shuttleengine.ForkAudit{Forks: forks}, func() {})
	seedShuttleRunState(t, fx.ShuttleRunRoot, "master-strand-audit", session)

	if _, err := websterengine.Run(fx.Deps, websterengine.RunOptions{}); err != nil {
		t.Fatalf("Run() error = %v; want nil", err)
	}
	prompt, err := os.ReadFile(filepath.Join(fx.Deps.Geom.PromptsDir, "verify-fix.md"))
	if err != nil {
		t.Fatalf("read verify-fix prompt: %v", err)
	}
	if !strings.Contains(string(prompt), websterengine.VerifyGateReportPath(fx.Deps.Geom.ReportsDir)) {
		t.Errorf("verify-fix prompt does not name the gate report path")
	}
	if _, err := os.Stat(filepath.Join(fx.Deps.Geom.PromptsDir, "integration.md")); err == nil {
		t.Errorf("integration.md exists; Run renders no integration prompt")
	}
}

// TestRun_FabricReferenceInFixerForkIsStuck proves a fabric reference in the fixer fork's transcript is correctness whatever its command:
// the run ends stuck, the stuck reason quotes the command, and state.json carries one pending finding with no path.
func TestRun_FabricReferenceInFixerForkIsStuck(t *testing.T) {
	const session = "master-session-fabric"
	const cmd = "cat FABRICREF/webster/state.json"
	fx := newRunFixture(t, 1)
	fx.Deps.RefMatcher = fabricMatcher{}
	appendIntegrationVerify(t, fx.PlanDir, "true")
	seedMatchingState(t, fx, &websterengine.State{
		Batches: map[int]*websterengine.BatchState{
			1: {Slug: "batch1", Kind: "fork", Terminal: true, Status: "done", SessionID: session, CardSHAs: []string{"deadbeef"}},
		},
	})
	forks := []shuttleengine.ForkReport{
		{TranscriptPath: "/transcripts/fork1.jsonl", ReportReturned: true},
		{TranscriptPath: "/transcripts/fixer.jsonl", ReportReturned: true, BashCommands: []string{cmd}},
	}
	fx.Starter.handle = auditDoneHandle(t, fx, session, 1, shuttleengine.ForkAudit{Forks: forks}, func() {})
	seedShuttleRunState(t, fx.ShuttleRunRoot, "master-strand-audit", session)

	result, err := websterengine.Run(fx.Deps, websterengine.RunOptions{})
	if err != nil {
		t.Fatalf("Run() error = %v; want nil (a correctness finding demotes, it is not an error)", err)
	}
	if result.Outcome != "stuck" {
		t.Fatalf("RunResult.Outcome = %q; want %q", result.Outcome, "stuck")
	}
	if !strings.Contains(result.StuckReason, cmd) {
		t.Errorf("StuckReason = %q; want it to quote %q", result.StuckReason, cmd)
	}
	st, err := websterengine.LoadState(fx.Deps.Geom.WebsterDir, fx.Deps.Geom.ScratchDir)
	if err != nil {
		t.Fatalf("LoadState() error = %v", err)
	}
	if len(st.PendingAuditFindings) != 1 || len(st.PendingAuditFindings[0].Paths) != 0 {
		t.Errorf("PendingAuditFindings = %+v; want one finding with no path", st.PendingAuditFindings)
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

// verifyGateFixture wires fx for a run whose plan carries verify, with one card commit touching internal/batch1 under a go.mod module, and returns the card commit.
// The worktree is clean afterwards, so the gate's own verify is the only thing that can fail.
func verifyGateFixture(t *testing.T, fx *runFixture, verify string) string {
	t.Helper()
	appendIntegrationVerify(t, fx.PlanDir, verify)
	gitkit.CommitFile(t, fx.Worktree, "go.mod", "module example.com/m\n", "go.mod")
	sha := gitkit.CommitFile(t, fx.Worktree, "internal/batch1/a.go", "package batch1\n", "card 1")
	seedMatchingState(t, fx, &websterengine.State{
		Batches: map[int]*websterengine.BatchState{
			1: {Slug: "batch1", Kind: "fork", Terminal: true, Status: "done", CardSHAs: []string{sha}},
		},
	})
	return sha
}

// verifyGateOf returns the verify entry Run handed StartMaster.
func verifyGateOf(t *testing.T, fx *runFixture) shuttleengine.GateEntry {
	t.Helper()
	for _, e := range fx.Starter.gateCalls[0] {
		if e.Name == "verify" {
			return e
		}
	}
	t.Fatalf("StartMaster's gate %+v has no verify entry", fx.Starter.gateCalls[0])
	return shuttleengine.GateEntry{}
}

// writeDoneContract writes the two files Merriam's last action writes, outcome done.
func writeDoneContract(t *testing.T, fx *runFixture) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(fx.Deps.Geom.WebsterDir, "outcome.yaml"), []byte("outcome: done\nstuck_reason: null\nbatches_done: 1\n"), 0o644); err != nil {
		t.Fatalf("write outcome.yaml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(fx.Deps.Geom.WebsterDir, "summary.md"), []byte("# Shipped\n\nAll good.\n"), 0o644); err != nil {
		t.Fatalf("write summary.md: %v", err)
	}
}

// TestRun_VerifyGateFailsThenPassesEndsDone proves a fake Merriam whose first done arrival fails verify and whose second passes ends done with one re-prompt,
// and that the findings name the failing identity and the card whose commit touched its package.
func TestRun_VerifyGateFailsThenPassesEndsDone(t *testing.T) {
	fx := newRunFixture(t, 1)
	okFile := filepath.Join(t.TempDir(), "ok")
	sha := verifyGateFixture(t, fx, "[ -f "+okFile+" ] || { printf 'FAIL\\texample.com/m/internal/batch1\\t0.01s\\n'; exit 1; }")

	var findings string
	var reprompts int
	fx.Starter.handle = &runFakeHandle{
		strandGUID: "master-strand-gatepass",
		result: shuttleengine.Result{
			Outcome:   shuttleengine.OutcomeDone,
			SessionID: "master-session-gatepass",
			RunDir:    "/run/dir/gatepass",
			ForkAudit: &shuttleengine.ForkAudit{Forks: []shuttleengine.ForkReport{{TranscriptPath: "/transcripts/fork1.jsonl", ReportReturned: true}}},
		},
		onWait: func() {
			writeDoneContract(t, fx)
			entry := verifyGateOf(t, fx)
			first, err := entry.Gate()
			if err != nil {
				t.Fatalf("first gate evaluation error = %v; want nil", err)
			}
			if first.Passed {
				t.Fatalf("first gate evaluation passed; want a verify failure")
			}
			reprompts++
			findings = first.Findings
			// Merriam's fixer makes its fix.
			if err := os.WriteFile(okFile, nil, 0o644); err != nil {
				t.Fatalf("write ok file: %v", err)
			}
			second, err := entry.Gate()
			if err != nil || !second.Passed {
				t.Fatalf("second gate evaluation = %+v, %v; want a pass", second, err)
			}
		},
	}
	seedShuttleRunState(t, fx.ShuttleRunRoot, "master-strand-gatepass", "master-session-gatepass")

	result, err := websterengine.Run(fx.Deps, websterengine.RunOptions{})
	if err != nil {
		t.Fatalf("Run() error = %v; want nil", err)
	}
	if result.Outcome != "done" {
		t.Errorf("Outcome = %q; want done", result.Outcome)
	}
	if reprompts != 1 {
		t.Errorf("re-prompts = %d; want 1", reprompts)
	}
	if !strings.Contains(findings, "example.com/m/internal/batch1") {
		t.Errorf("findings = %q; want the failing identity", findings)
	}
	if !strings.Contains(findings, "01-batch1") {
		t.Errorf("findings = %q; want the card whose commit %s touched the failing package", findings, sha)
	}
	if _, err := os.Stat(websterengine.VerifyGateReportPath(fx.Deps.Geom.ReportsDir)); err != nil {
		t.Errorf("verify-gate report: %v; want one written by the failed evaluation", err)
	}
}

// TestRun_VerifyGateExhaustedEndsStuck proves a gate that never passes ends the run stuck with a reason naming the failing identities.
func TestRun_VerifyGateExhaustedEndsStuck(t *testing.T) {
	fx := newRunFixture(t, 1)
	verifyGateFixture(t, fx, "printf 'FAIL\\texample.com/m/internal/batch1\\t0.01s\\n'; exit 1")

	fx.Starter.handle = &runFakeHandle{
		strandGUID: "master-strand-gatestuck",
		result: shuttleengine.Result{
			Outcome:   shuttleengine.OutcomeDone,
			SessionID: "master-session-gatestuck",
			RunDir:    "/run/dir/gatestuck",
			ForkAudit: &shuttleengine.ForkAudit{Forks: []shuttleengine.ForkReport{{TranscriptPath: "/transcripts/fork1.jsonl", ReportReturned: true}}},
			Gate:      &shuttleengine.GateOutcome{Passed: false, Attempts: 3},
		},
		onWait: func() {
			writeDoneContract(t, fx)
			if res, err := verifyGateOf(t, fx).Gate(); err != nil || res.Passed {
				t.Fatalf("gate evaluation = %+v, %v; want a verify failure", res, err)
			}
		},
	}
	seedShuttleRunState(t, fx.ShuttleRunRoot, "master-strand-gatestuck", "master-session-gatestuck")

	result, err := websterengine.Run(fx.Deps, websterengine.RunOptions{})
	if err != nil {
		t.Fatalf("Run() error = %v; want nil", err)
	}
	if result.Outcome != "stuck" {
		t.Errorf("Outcome = %q; want stuck", result.Outcome)
	}
	if !strings.Contains(result.StuckReason, "example.com/m/internal/batch1") {
		t.Errorf("StuckReason = %q; want the failing identity", result.StuckReason)
	}
}

// TestRun_FlakyVerifyKeepsDoneWithWarning proves a verify failure that passes on rerun keeps the run done and surfaces the flaky warning and summary section.
func TestRun_FlakyVerifyKeepsDoneWithWarning(t *testing.T) {
	fx := newRunFixture(t, 1)
	counter := filepath.Join(t.TempDir(), "runs")
	// The first run records itself and fails.
	// The rerun sees the record and passes.
	verifyGateFixture(t, fx, "if [ -f "+counter+" ]; then exit 0; fi; : > "+counter+"; printf 'FAIL\\texample.com/m/internal/batch1\\t0.01s\\n'; exit 1")

	fx.Starter.handle = &runFakeHandle{
		strandGUID: "master-strand-flaky",
		result: shuttleengine.Result{
			Outcome:   shuttleengine.OutcomeDone,
			SessionID: "master-session-flaky",
			RunDir:    "/run/dir/flaky",
			ForkAudit: &shuttleengine.ForkAudit{Forks: []shuttleengine.ForkReport{{TranscriptPath: "/transcripts/fork1.jsonl", ReportReturned: true}}},
			Gate:      &shuttleengine.GateOutcome{Passed: true},
		},
		onWait: func() {
			writeDoneContract(t, fx)
			if res, err := verifyGateOf(t, fx).Gate(); err != nil || !res.Passed {
				t.Fatalf("gate evaluation = %+v, %v; want a pass on rerun", res, err)
			}
		},
	}
	seedShuttleRunState(t, fx.ShuttleRunRoot, "master-strand-flaky", "master-session-flaky")

	result, err := websterengine.Run(fx.Deps, websterengine.RunOptions{})
	if err != nil {
		t.Fatalf("Run() error = %v; want nil", err)
	}
	if result.Outcome != "done" {
		t.Errorf("Outcome = %q; want done", result.Outcome)
	}
	if len(result.Warnings) != 1 || !strings.Contains(result.Warnings[0], "flaky") {
		t.Errorf("Warnings = %v; want exactly the flaky warning", result.Warnings)
	}
	summary, err := os.ReadFile(filepath.Join(fx.Deps.Geom.WebsterDir, "summary.md"))
	if err != nil {
		t.Fatalf("read summary.md: %v", err)
	}
	if !strings.Contains(string(summary), "example.com/m/internal/batch1") {
		t.Errorf("summary.md = %q; want the flaky identity in its triage section", summary)
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

	// Card 1 Uses card 2's own Create target, so batch 2 must run before batch 1. The path is a
	// Create target of card 2 within the same plan, satisfied by planparser's own
	// createTargetsUnion check -- and deliberately NOT also seeded for real under the worktree:
	// under this fixture's default language: go, a Create target's own self-glyph would then
	// resolve found against the real repo, tripping the blocking create-already-exists finding this
	// batch's move onto planglyph now checks for real, which is not what this test is about.
	addCardUses(t, fx.PlanDir, 1, "internal/batch2/new.go")

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

	// Card 1 Uses card 2's target and card 2 Uses card 1's target: a mutual dependency
	// SequenceBatches condenses into one cycle. Neither path is also seeded for real under the
	// worktree, for the same reason TestRun_ReorderingIsObservableInMasterPrompt's own doc comment
	// gives: doing so would trip the blocking create-already-exists finding this batch's move onto
	// planglyph now checks for real, over a Create target's own default-language self-glyph.
	addCardUses(t, fx.PlanDir, 1, "internal/batch2/new.go")
	addCardUses(t, fx.PlanDir, 2, "internal/batch1/new.go")

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

// TestRun_ResumeWithCompletedCreateCardIsNotRefused is F2's (round fable5-high-r3) regression
// test: a resumed run whose state records batch 1 terminal, and whose batch-1 Create target
// consequently exists on disk, must sail past the entry validation gate — against pre-fix source
// the gate re-validated the WHOLE plan and refused the resume on create-already-exists, the plan
// working exactly as designed, wedging the documented `lyx webster run` resume flow permanently
// (--fresh only fires on a fingerprint mismatch, so there was no way out).
func TestRun_ResumeWithCompletedCreateCardIsNotRefused(t *testing.T) {
	fx := newRunFixture(t, 2)
	// Batch 1's own Create target landed — exactly what a completed Create card leaves behind.
	gitkit.CommitFile(t, fx.Worktree, "internal/batch1/new.go", "package batch1\n\nfunc Landed() {}\n", "card 1 landed")

	seedMatchingState(t, fx, &websterengine.State{
		RunGUID: "resume-run",
		Batches: map[int]*websterengine.BatchState{
			1: {Slug: "batch1", Kind: "fork", Terminal: true, Status: "done"},
		},
	})

	wantSessionID := "master-session-resume"
	handle := &runFakeHandle{
		strandGUID: "master-strand-resume",
		result: shuttleengine.Result{
			Outcome:              shuttleengine.OutcomeAsking,
			SessionID:            wantSessionID,
			RunDir:               "/run/dir/resume",
			LastAssistantMessage: "resumed and asking",
		},
	}
	fx.Starter.handle = handle
	seedShuttleRunState(t, fx.ShuttleRunRoot, "master-strand-resume", wantSessionID)

	_, err := websterengine.Run(fx.Deps, websterengine.RunOptions{})
	var target *websterengine.MasterAskingError
	if !errors.As(err, &target) {
		t.Fatalf("Run() error = %v; want a *MasterAskingError — the resume must reach the Master spawn, never a create-already-exists refusal for its own completed card", err)
	}
	if fx.Starter.callCount() != 1 {
		t.Errorf("Starter.callCount() = %d; want 1 — the resumed run must spawn Master", fx.Starter.callCount())
	}
}

// TestRun_UnapprovedPlanRefused pins the approval gate the entry-time ValidateDispatch scoping
// deliberately does not carry (ValidateDispatch runs the format-only check set): an unapproved
// plan is refused before batching, state, or any spawn.
func TestRun_UnapprovedPlanRefused(t *testing.T) {
	fx := newRunFixture(t, 1)
	overview := filepath.Join(fx.PlanDir, "00-overview.md")
	data, err := os.ReadFile(overview)
	if err != nil {
		t.Fatalf("read overview: %v", err)
	}
	if err := os.WriteFile(overview, []byte(strings.Replace(string(data), "approved: true", "approved: false", 1)), 0o644); err != nil {
		t.Fatalf("write overview: %v", err)
	}

	_, err = websterengine.Run(fx.Deps, websterengine.RunOptions{})
	if err == nil || !strings.Contains(err.Error(), "not approved") {
		t.Fatalf("Run() error = %v; want the not-approved refusal", err)
	}
	if fx.Starter.callCount() != 0 {
		t.Errorf("Starter was reached (%d calls) for an unapproved plan; want zero", fx.Starter.callCount())
	}
}

// TestRun_ValidationErrorAndRebaselineSaveFailure_ReportsBoth pins R5-3: when ValidateDispatch
// returns an error AND persisting the plan-fingerprint re-baseline also fails, Run must report
// both rather than dropping the second.
//
// It matters because the resolve pass has by then already rewritten the plan on disk, so a
// state.json still holding the pre-rewrite fingerprint makes the NEXT run refuse this run's own
// edit as a foreign one with ErrFingerprintMismatch — whose advised recourse (--fresh) restarts
// into the same wall. Pre-fix the operator got no hint at all that this had happened.
//
// The validation error is forced by pointing WorktreeRoot at a path with no repository, so
// planglyph's own openRepo fails; the save failure is forced by making the webster dir read-only
// after the matching state has been seeded, which is the one directory SaveState writes into.
func TestRun_ValidationErrorAndRebaselineSaveFailure_ReportsBoth(t *testing.T) {
	fx := newRunFixture(t, 1)
	seedMatchingState(t, fx, &websterengine.State{})

	fx.Deps.Geom.WorktreeRoot = filepath.Join(t.TempDir(), "no-such-tree")

	websterDir := fx.Deps.Geom.WebsterDir
	if err := os.Chmod(websterDir, 0o555); err != nil {
		t.Fatalf("chmod webster dir read-only: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(websterDir, 0o755) })

	_, err := websterengine.Run(fx.Deps, websterengine.RunOptions{})
	if err == nil {
		t.Fatal("Run() error = nil; want both the validation failure and the re-baseline persist failure reported")
	}
	got := err.Error()
	if !strings.Contains(got, "re-baseline") {
		t.Errorf("Run() error = %q; want it to also name the dropped plan-fingerprint re-baseline persist failure", got)
	}
	if !errors.Is(err, planglyph.ErrQuarryUnavailable) {
		t.Errorf("Run() error = %v; want the primary validation failure still classifiable via errors.Is(err, planglyph.ErrQuarryUnavailable) — the re-baseline report must not mask it", err)
	}
}

// TestRun_GateReachesStartMaster proves RunDeps.Gate is what Run hands StartMaster beside the Spec:
// the gate is threaded, never rebuilt or dropped, so a caller that tells webster a validator gets
// the Master run held to it. The gate closure is never invoked here -- evaluating it is
// shuttleengine's own Wait's job, and this fixture's Starter is a fake that never reaches it.
func TestRun_GateReachesStartMaster(t *testing.T) {
	fx := newRunFixture(t, 1)

	var called bool
	fx.Deps.Gate = shuttleengine.GateSpec{{
		Gate: func() (shuttleengine.GateResult, error) {
			called = true
			return shuttleengine.GateResult{Passed: true}, nil
		},
		Attempts: 7,
	}}

	seedMatchingState(t, fx, &websterengine.State{
		Batches: map[int]*websterengine.BatchState{
			1: {Slug: "batch1", Kind: "fork", Terminal: true, Status: "done"},
		},
	})

	runToDone(t, fx, "master-strand-gate", "master-session-gate", nil, 1)

	if len(fx.Starter.gateCalls) != 1 {
		t.Fatalf("len(Starter.gateCalls) = %d; want 1", len(fx.Starter.gateCalls))
	}
	got := fx.Starter.gateCalls[0]
	if len(got) != 2 {
		t.Fatalf("StartMaster received %d gate entries; want the told one and the verify entry", len(got))
	}
	if got[0].Gate == nil {
		t.Error("StartMaster received a nil Gate; want the told closure")
	}
	if got[0].Attempts != 7 {
		t.Errorf("StartMaster received Attempts = %d; want 7", got[0].Attempts)
	}
	if got[1].Name != "verify" {
		t.Errorf("StartMaster's last gate entry = %q; want the verify entry Run adds", got[1].Name)
	}
	if called {
		t.Error("the gate closure was invoked by Run; want it spent only by shuttle's own Wait")
	}
}

// TestRun_GateNamingVerifyIsRefused proves a RunDeps.Gate that already names `verify` is refused, so a recipe row cannot add a second entry beside Run's own.
func TestRun_GateNamingVerifyIsRefused(t *testing.T) {
	fx := newRunFixture(t, 1)
	fx.Deps.Gate = shuttleengine.GateSpec{{
		Name:     "verify",
		Gate:     func() (shuttleengine.GateResult, error) { return shuttleengine.GateResult{Passed: true}, nil },
		Attempts: 1,
	}}

	_, err := websterengine.Run(fx.Deps, websterengine.RunOptions{})
	requireWayForward(t, err, "verify", "drop the")
	if fx.Starter.callCount() != 0 {
		t.Errorf("Starter.callCount() = %d; want 0, since the refusal precedes the spawn", fx.Starter.callCount())
	}
}

// TestRun_ZeroGateReachesStartMasterWithOnlyVerify proves a RunDeps that names no gate hands StartMaster exactly the verify entry Run adds, with the configured attempt budget.
func TestRun_ZeroGateReachesStartMasterWithOnlyVerify(t *testing.T) {
	fx := newRunFixture(t, 1)

	seedMatchingState(t, fx, &websterengine.State{
		Batches: map[int]*websterengine.BatchState{
			1: {Slug: "batch1", Kind: "fork", Terminal: true, Status: "done"},
		},
	})

	runToDone(t, fx, "master-strand-ungated", "master-session-ungated", nil, 1)

	if len(fx.Starter.gateCalls) != 1 {
		t.Fatalf("len(Starter.gateCalls) = %d; want 1", len(fx.Starter.gateCalls))
	}
	got := fx.Starter.gateCalls[0]
	if len(got) != 1 || got[0].Name != "verify" {
		t.Fatalf("StartMaster received gate %+v; want exactly the verify entry", got)
	}
	if got[0].Attempts != fx.Deps.Config.VerifyGateAttempts {
		t.Errorf("verify entry Attempts = %d; want Config.VerifyGateAttempts %d", got[0].Attempts, fx.Deps.Config.VerifyGateAttempts)
	}
	if got[0].PassOnCap || got[0].MayHold {
		t.Errorf("verify entry = %+v; want a plain must-pass entry", got[0])
	}
}

// TestRun_Regression20260930_BegunUnrecordedBatchResumes pins the 2026-09-30 wedge: state records batch 1 begun but not terminal, with its Create target already committed,
// and Run must pass the entry validation and reach the Master spawn with no create-already-exists refusal.
func TestRun_Regression20260930_BegunUnrecordedBatchResumes(t *testing.T) {
	fx := newRunFixture(t, 2)
	gitkit.CommitFile(t, fx.Worktree, "internal/batch1/new.go", "package batch1\n\nfunc Landed() {}\n", "card 1 landed")

	seedMatchingState(t, fx, &websterengine.State{
		RunGUID: "resume-run",
		Batches: map[int]*websterengine.BatchState{
			1: {Slug: "batch1", Kind: "fork", StartSHA: "0123456789abcdef0123456789abcdef01234567"},
		},
	})

	wantSessionID := "master-session-begun"
	fx.Starter.handle = &runFakeHandle{
		strandGUID: "master-strand-begun",
		result: shuttleengine.Result{
			Outcome:              shuttleengine.OutcomeAsking,
			SessionID:            wantSessionID,
			RunDir:               "/run/dir/begun",
			LastAssistantMessage: "resumed and asking",
		},
	}
	seedShuttleRunState(t, fx.ShuttleRunRoot, "master-strand-begun", wantSessionID)

	_, err := websterengine.Run(fx.Deps, websterengine.RunOptions{})
	var target *websterengine.MasterAskingError
	if !errors.As(err, &target) {
		t.Fatalf("Run() error = %v; want a *MasterAskingError — a begun, unrecorded batch must resume to the Master spawn", err)
	}
	if fx.Starter.callCount() != 1 {
		t.Errorf("Starter.callCount() = %d; want 1", fx.Starter.callCount())
	}
}

// TestRun_Regression329_ForthcomingCreateTargetPassesEntryValidation pins #329 at run entry: state records batch 1 begun but not terminal with nothing landed,
// and card 2 Uses card 1's Create target, which does not exist yet; Run must pass the entry validation and reach the Master spawn.
func TestRun_Regression329_ForthcomingCreateTargetPassesEntryValidation(t *testing.T) {
	fx := newRunFixture(t, 2)
	addCardUses(t, fx.PlanDir, 2, "internal/batch1/new.go")

	seedMatchingState(t, fx, &websterengine.State{
		RunGUID: "resume-run",
		Batches: map[int]*websterengine.BatchState{
			1: {Slug: "batch1", Kind: "fork", StartSHA: "0123456789abcdef0123456789abcdef01234567"},
		},
	})
	askingMaster(t, fx, "forthcoming")

	_, err := websterengine.Run(fx.Deps, websterengine.RunOptions{})
	requireReachedMaster(t, fx, err)
}

// requireWayForward fails unless err carries the trailing "way forward:" clause and every want fragment after it, so each reaching test matches the message the way the refusal table does.
func requireWayForward(t *testing.T, err error, wants ...string) {
	t.Helper()
	if err == nil {
		t.Fatal("error = nil; want a refusal carrying a way forward")
	}
	msg := err.Error()
	_, clause, found := strings.Cut(msg, "way forward:")
	if !found {
		t.Fatalf("error = %q; want a trailing way forward clause", msg)
	}
	for _, want := range wants {
		if !strings.Contains(clause, want) {
			t.Errorf("way forward clause = %q; want it to contain %q", clause, want)
		}
	}
}

// askingMaster scripts fx's Starter with a Master that ends its turn asking, the cheapest way for a re-run to prove it got past every refusal gate and reached the spawn.
func askingMaster(t *testing.T, fx *runFixture, label string) {
	t.Helper()
	fx.Starter.startErr = nil
	fx.Starter.handle = &runFakeHandle{
		strandGUID: "master-strand-" + label,
		result: shuttleengine.Result{
			Outcome:   shuttleengine.OutcomeAsking,
			SessionID: "master-session-" + label,
			RunDir:    "/run/dir/" + label,
		},
	}
	seedShuttleRunState(t, fx.ShuttleRunRoot, "master-strand-"+label, "master-session-"+label)
}

// requireReachedMaster asserts err is the Master-asking error of a run that got past every gate.
func requireReachedMaster(t *testing.T, fx *runFixture, err error) {
	t.Helper()
	if !errors.Is(err, websterengine.ErrMasterAsking) {
		t.Fatalf("Run() after taking the way forward error = %v; want it to reach the Master spawn", err)
	}
	if fx.Starter.callCount() == 0 {
		t.Error("Starter was never reached after taking the way forward")
	}
}

// rebaselineOnDisk is what the rebaseline verb does: parse the edited plan, re-derive its batches, restamp the recorded fingerprint and save.
func rebaselineOnDisk(t *testing.T, fx *runFixture, cards ...int) {
	t.Helper()
	plan, err := planparser.ParsePlan(fx.PlanDir)
	if err != nil {
		t.Fatalf("ParsePlan() error = %v", err)
	}
	st, err := websterengine.LoadState(fx.Deps.Geom.WebsterDir, fx.Deps.Geom.ScratchDir)
	if err != nil || st == nil {
		t.Fatalf("LoadState() = %v, %v; want recorded state", st, err)
	}
	batches, _ := websterengine.SequenceBatches(fx.Deps.Batcher.Batch(plan.Cards))
	if _, err := websterengine.Rebaseline(websterengine.RebaselineDeps{Plan: plan, Batches: batches, State: st, Cards: cards, Geom: fx.Deps.Geom}); err != nil {
		t.Fatalf("Rebaseline() error = %v", err)
	}
	if err := websterengine.SaveState(fx.Deps.Geom.WebsterDir, fx.Deps.Geom.ScratchDir, st); err != nil {
		t.Fatalf("SaveState() error = %v", err)
	}
}

// emptyBatcher is a batchifier that derives no execution batches from any plan.
type emptyBatcher struct{}

func (emptyBatcher) Batch([]planparser.Card) []batcher.Batch { return nil }
func (emptyBatcher) Name() string                            { return "empty" }

func TestRun_WayForward_UnapprovedPlan(t *testing.T) {
	fx := newRunFixture(t, 1)
	overview := filepath.Join(fx.PlanDir, "00-overview.md")
	data, err := os.ReadFile(overview)
	if err != nil {
		t.Fatalf("read overview: %v", err)
	}
	if err := os.WriteFile(overview, []byte(strings.Replace(string(data), "approved: true", "approved: false", 1)), 0o644); err != nil {
		t.Fatalf("write overview: %v", err)
	}

	_, err = websterengine.Run(fx.Deps, websterengine.RunOptions{})
	requireWayForward(t, err, "approve the plan", "lyx webster run")

	if err := os.WriteFile(overview, data, 0o644); err != nil {
		t.Fatalf("approve plan: %v", err)
	}
	askingMaster(t, fx, "approved")
	_, err = websterengine.Run(fx.Deps, websterengine.RunOptions{})
	requireReachedMaster(t, fx, err)
}

func TestRun_WayForward_ZeroBatches(t *testing.T) {
	fx := newRunFixture(t, 1)
	realBatcher := fx.Deps.Batcher
	fx.Deps.Batcher = emptyBatcher{}

	_, err := websterengine.Run(fx.Deps, websterengine.RunOptions{})
	requireWayForward(t, err, "fix the plan's cards", "lyx webster rebaseline --card", "lyx webster run")
	if fx.Starter.callCount() != 0 {
		t.Errorf("Starter was reached (%d calls) for a zero-batch plan; want zero", fx.Starter.callCount())
	}

	fx.Deps.Batcher = realBatcher
	askingMaster(t, fx, "zero")
	_, err = websterengine.Run(fx.Deps, websterengine.RunOptions{})
	requireReachedMaster(t, fx, err)
}

func TestRun_WayForward_ValidationRefusal(t *testing.T) {
	fx := newRunFixture(t, 1)
	gitkit.CommitFile(t, fx.Worktree, "sub/a.go", "package sub\n\nfunc Foo() {}\n", "add sub package")
	cardPath := filepath.Join(fx.PlanDir, "01-batch1.md")
	original, err := os.ReadFile(cardPath)
	if err != nil {
		t.Fatalf("read card: %v", err)
	}
	addCardUses(t, fx.PlanDir, 1, "sub#Missing")

	_, err = websterengine.Run(fx.Deps, websterengine.RunOptions{})
	requireWayForward(t, err, "fix the named cards", "lyx webster rebaseline --card", "lyx webster run")

	// The refused run already recorded the edited plan;
	// fixing the card is a further edit, so the next run refuses it as foreign until the operator rebaselines, which the message names.
	if err := os.WriteFile(cardPath, original, 0o644); err != nil {
		t.Fatalf("fix card: %v", err)
	}
	askingMaster(t, fx, "validated")
	_, err = websterengine.Run(fx.Deps, websterengine.RunOptions{})
	if !errors.Is(err, websterengine.ErrFingerprintMismatch) {
		t.Fatalf("Run() after the plan fix error = %v; want errors.Is(err, ErrFingerprintMismatch) until rebaselined", err)
	}
	rebaselineOnDisk(t, fx, 1)
	_, err = websterengine.Run(fx.Deps, websterengine.RunOptions{})
	requireReachedMaster(t, fx, err)
}

func TestRun_WayForward_QuarryUnavailable(t *testing.T) {
	fx := newRunFixture(t, 1)
	worktree := fx.Deps.Geom.WorktreeRoot
	fx.Deps.Geom.WorktreeRoot = filepath.Join(t.TempDir(), "does-not-exist")

	_, err := websterengine.Run(fx.Deps, websterengine.RunOptions{})
	requireWayForward(t, err, "transient", "lyx webster run", "quarry")

	fx.Deps.Geom.WorktreeRoot = worktree
	askingMaster(t, fx, "quarry")
	_, err = websterengine.Run(fx.Deps, websterengine.RunOptions{})
	requireReachedMaster(t, fx, err)
}

func TestRun_WayForward_StartMasterFailure(t *testing.T) {
	fx := newRunFixture(t, 1)
	fx.Starter.startErr = errors.New("provider did not come up")

	_, err := websterengine.Run(fx.Deps, websterengine.RunOptions{})
	requireWayForward(t, err, "transient", "lyx webster run")

	askingMaster(t, fx, "start")
	_, err = websterengine.Run(fx.Deps, websterengine.RunOptions{})
	requireReachedMaster(t, fx, err)
}

// TestRun_WayForward_MasterEndedEarly proves the asking, died and timeout errors name the re-run,
// and that a fresh Master resuming from state.json then finishes.
func TestRun_WayForward_MasterEndedEarly(t *testing.T) {
	for _, outcome := range []shuttleengine.Outcome{shuttleengine.OutcomeAsking, shuttleengine.OutcomeDied, shuttleengine.OutcomeTimeout} {
		t.Run(string(outcome), func(t *testing.T) {
			fx := newRunFixture(t, 1)
			seedMatchingState(t, fx, &websterengine.State{
				Batches: map[int]*websterengine.BatchState{
					1: {Slug: "batch1", Kind: "fork", Terminal: true, Status: "done", SessionID: "master-session-early"},
				},
			})
			fx.Starter.handle = &runFakeHandle{
				strandGUID: "master-strand-early",
				result:     shuttleengine.Result{Outcome: outcome, SessionID: "master-session-early", RunDir: "/run/dir/early"},
			}
			seedShuttleRunState(t, fx.ShuttleRunRoot, "master-strand-early", "master-session-early")

			_, err := websterengine.Run(fx.Deps, websterengine.RunOptions{})
			requireWayForward(t, err, "lyx webster run", "re-step the Webster row", "resumes from state.json")

			runToDone(t, fx, "master-strand-early", "master-session-early", []shuttleengine.ForkReport{
				{TranscriptPath: "/transcripts/fork1.jsonl", ReportReturned: true},
			}, 1)
		})
	}
}

// TestRun_WayForward_RunExitRefusals reaches each run-exit refusal over a done Master, asserts its way forward, then takes it (the state a re-driven batch leaves, a finished summary, an audit that completed) and proves the re-run ends done.
func TestRun_WayForward_RunExitRefusals(t *testing.T) {
	const session = "master-session-exit"
	const strand = "master-strand-exit"
	oneFork := []shuttleengine.ForkReport{{TranscriptPath: "/transcripts/fork1.jsonl", ReportReturned: true}}
	doneRecord := func() *websterengine.BatchState {
		return &websterengine.BatchState{Slug: "batch1", Kind: "fork", Terminal: true, Status: "done", SessionID: session}
	}

	tests := []struct {
		name  string
		state map[int]*websterengine.BatchState
		audit *shuttleengine.ForkAudit
		// outcome is outcome.yaml's content; "" writes none.
		outcome string
		summary bool
		// repair mutates the world the way the way forward describes, before the clean re-run.
		repair func(t *testing.T, fx *runFixture)
		want   string
	}{
		{
			name:    "malformed outcome",
			state:   map[int]*websterengine.BatchState{1: doneRecord()},
			audit:   &shuttleengine.ForkAudit{Forks: oneFork},
			outcome: "outcome: [not a mapping\n",
			summary: true,
			want:    "stale file is archived",
		},
		{
			name:    "missing summary",
			state:   map[int]*websterengine.BatchState{1: doneRecord()},
			audit:   &shuttleengine.ForkAudit{Forks: oneFork},
			outcome: "outcome: done\nstuck_reason: null\nbatches_done: 1\n",
			summary: false,
			want:    "re-drives every batch without a done record",
		},
		{
			name:    "batch without a done record",
			state:   map[int]*websterengine.BatchState{1: {Slug: "batch1", Kind: "fork", SessionID: session}},
			audit:   &shuttleengine.ForkAudit{Forks: oneFork},
			outcome: "outcome: done\nstuck_reason: null\nbatches_done: 1\n",
			summary: true,
			repair: func(t *testing.T, fx *runFixture) {
				st, err := websterengine.LoadState(fx.Deps.Geom.WebsterDir, fx.Deps.Geom.ScratchDir)
				if err != nil || st == nil {
					t.Fatalf("LoadState() = %v, %v", st, err)
				}
				st.Batches[1] = doneRecord()
				if err := websterengine.SaveState(fx.Deps.Geom.WebsterDir, fx.Deps.Geom.ScratchDir, st); err != nil {
					t.Fatalf("SaveState() error = %v", err)
				}
			},
			want: "re-drives every batch without a done record",
		},
		{
			name:    "audit never completed",
			state:   map[int]*websterengine.BatchState{1: doneRecord()},
			audit:   nil,
			outcome: "outcome: done\nstuck_reason: null\nbatches_done: 1\n",
			summary: true,
			want:    "re-drives every batch without a done record",
		},
		{
			name:    "audited fewer forks than begun",
			state:   map[int]*websterengine.BatchState{1: doneRecord()},
			audit:   &shuttleengine.ForkAudit{},
			outcome: "outcome: done\nstuck_reason: null\nbatches_done: 1\n",
			summary: true,
			want:    "re-drives every batch without a done record",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fx := newRunFixture(t, 1)
			seedMatchingState(t, fx, &websterengine.State{Batches: tt.state})
			fx.Starter.handle = &runFakeHandle{
				strandGUID: strand,
				result: shuttleengine.Result{
					Outcome:   shuttleengine.OutcomeDone,
					SessionID: session,
					RunDir:    "/run/dir/exit",
					ForkAudit: tt.audit,
				},
				onWait: func() {
					if tt.outcome != "" {
						if err := os.WriteFile(filepath.Join(fx.Deps.Geom.WebsterDir, "outcome.yaml"), []byte(tt.outcome), 0o644); err != nil {
							t.Fatalf("write outcome.yaml: %v", err)
						}
					}
					if tt.summary {
						if err := os.WriteFile(filepath.Join(fx.Deps.Geom.WebsterDir, "summary.md"), []byte("# Shipped\n\nAll good.\n"), 0o644); err != nil {
							t.Fatalf("write summary.md: %v", err)
						}
					}
				},
			}
			seedShuttleRunState(t, fx.ShuttleRunRoot, strand, session)

			_, err := websterengine.Run(fx.Deps, websterengine.RunOptions{})
			requireWayForward(t, err, "lyx webster run", tt.want)

			if tt.repair != nil {
				tt.repair(t, fx)
			}
			runToDone(t, fx, strand, session, oneFork, 1)
		})
	}
}

// TestRun_FreshRunOverNewGenerationAfterArchive proves a rework generation gets a fresh run:
// a finished two-card run is archived with ArchiveRunRecord, the plan is replaced by a generation whose first_card is 3,
// and Run starts over that plan with no ErrFingerprintMismatch, recording and telling Master only the new generation's batches.
func TestRun_FreshRunOverNewGenerationAfterArchive(t *testing.T) {
	fx := newRunFixture(t, 2)
	appendIntegrationVerify(t, fx.PlanDir, "go test ./...")
	geom := fx.Deps.Geom

	seedMatchingState(t, fx, &websterengine.State{
		Batches: map[int]*websterengine.BatchState{
			1: {Slug: "batch1", Kind: "fork", Terminal: true, Status: "done"},
			2: {Slug: "batch2", Kind: "fork", Terminal: true, Status: "done"},
		},
	})
	outcomePath := filepath.Join(geom.WebsterDir, "outcome.yaml")
	if err := os.WriteFile(outcomePath, []byte("outcome: done\nstuck_reason: null\nbatches_done: 2\n"), 0o644); err != nil {
		t.Fatalf("seed outcome: %v", err)
	}

	archiveDir := filepath.Join(t.TempDir(), "prior-generation", "webster")
	if err := websterengine.ArchiveRunRecord(geom, archiveDir); err != nil {
		t.Fatalf("ArchiveRunRecord() error = %v", err)
	}

	// Replace the plan with a whole new generation numbered on from the retired one.
	entries, err := os.ReadDir(fx.PlanDir)
	if err != nil {
		t.Fatalf("read plan dir: %v", err)
	}
	for _, e := range entries {
		if err := os.Remove(filepath.Join(fx.PlanDir, e.Name())); err != nil {
			t.Fatalf("remove %s: %v", e.Name(), err)
		}
	}
	plankit.Write(t, fx.PlanDir, runPlan(3, 2))
	appendIntegrationVerify(t, fx.PlanDir, "go test ./...")

	fx.Starter.handle = &runFakeHandle{strandGUID: "master-strand-generation", waitErr: fmt.Errorf("stop after spawn")}
	_, err = websterengine.Run(fx.Deps, websterengine.RunOptions{})
	if errors.Is(err, websterengine.ErrFingerprintMismatch) {
		t.Fatalf("Run() error = %v; want no ErrFingerprintMismatch over a fresh generation", err)
	}
	if fx.Starter.callCount() != 1 {
		t.Fatalf("Starter.callCount() = %d; want Master spawned once (Run error: %v)", fx.Starter.callCount(), err)
	}

	st, err := websterengine.LoadState(geom.WebsterDir, geom.ScratchDir)
	if err != nil || st == nil {
		t.Fatalf("LoadState() = %v, %v; want the fresh run's state", st, err)
	}
	for number := range st.Batches {
		if number != 3 && number != 4 {
			t.Errorf("state records batch %d; want only the new generation's batches 3 and 4", number)
		}
	}

	prompt := fx.Starter.startCalls[0].Prompt
	for _, want := range []string{"03 — batch3", "04 — batch4"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("Master prompt lacks %q", want)
		}
	}
	for _, stale := range []string{"batch1", "batch2"} {
		if strings.Contains(prompt, stale) {
			t.Errorf("Master prompt names retired batch %q; want only the new generation's", stale)
		}
	}
}

// TestRun_FirstInitRecordsPlanFileHashes proves the first-init state records a hash for every plan file, 00-overview.md included.
func TestRun_FirstInitRecordsPlanFileHashes(t *testing.T) {
	fx := newRunFixture(t, 2)
	askingMaster(t, fx, "first")
	_, err := websterengine.Run(fx.Deps, websterengine.RunOptions{})
	requireReachedMaster(t, fx, err)

	st, err := websterengine.LoadState(fx.Deps.Geom.WebsterDir, fx.Deps.Geom.ScratchDir)
	if err != nil || st == nil {
		t.Fatalf("LoadState() = %v, %v; want recorded state", st, err)
	}
	for _, name := range []string{"00-overview.md", "01-batch1.md", "02-batch2.md"} {
		if st.PlanFileHashes[name] == "" {
			t.Errorf("PlanFileHashes = %v; want a hash for %s", st.PlanFileHashes, name)
		}
	}
}

// TestRun_FingerprintMismatchWayForwardNamesTheEditedCards proves the mismatch refusal names the cards to pass to rebaseline, and never a card for 00-overview.md.
func TestRun_FingerprintMismatchWayForwardNamesTheEditedCards(t *testing.T) {
	t.Run("edited card", func(t *testing.T) {
		fx := newRunFixture(t, 2)
		seedMatchingState(t, fx, &websterengine.State{})
		addCardUses(t, fx.PlanDir, 2, "base.txt")

		_, err := websterengine.Run(fx.Deps, websterengine.RunOptions{})
		if !errors.Is(err, websterengine.ErrFingerprintMismatch) {
			t.Fatalf("Run() error = %v; want errors.Is(err, ErrFingerprintMismatch)", err)
		}
		requireWayForward(t, err, "lyx webster rebaseline --card 02", "lyx webster run --fresh")
	})
	t.Run("edited overview", func(t *testing.T) {
		fx := newRunFixture(t, 2)
		seedMatchingState(t, fx, &websterengine.State{})
		overview := filepath.Join(fx.PlanDir, "00-overview.md")
		data, err := os.ReadFile(overview)
		if err != nil {
			t.Fatalf("read overview: %v", err)
		}
		if err := os.WriteFile(overview, []byte(strings.Replace(string(data), "Framing.", "Framing, edited.", 1)), 0o644); err != nil {
			t.Fatalf("edit overview: %v", err)
		}

		_, err = websterengine.Run(fx.Deps, websterengine.RunOptions{})
		if !errors.Is(err, websterengine.ErrFingerprintMismatch) {
			t.Fatalf("Run() error = %v; want errors.Is(err, ErrFingerprintMismatch)", err)
		}
		requireWayForward(t, err, "--fresh")
		if strings.Contains(err.Error(), "--card") {
			t.Errorf("error %q; want no --card for an edited 00-overview.md", err.Error())
		}
	})
}

// seedFreshPendingState seeds a state with one recorded batch started at the fixture's first commit and one pending finding naming paths, and returns that start commit.
func seedFreshPendingState(t *testing.T, fx *runFixture, paths ...string) string {
	t.Helper()
	start := gitkit.RevParse(t, fx.Worktree, "HEAD")
	seedMatchingState(t, fx, &websterengine.State{
		RunGUID: "stale-run",
		Batches: map[int]*websterengine.BatchState{
			1: {Slug: "batch1", Kind: "fork", StartSHA: start},
		},
		PendingAuditFindings: []websterengine.PendingAuditFinding{{ID: "sess/parent:write:1", Class: "parent-write", Detail: "master wrote a tracked file", Paths: paths}},
	})
	return start
}

// TestRun_FreshRefusesWhileSuspectPathDiffers proves --fresh refuses, archiving nothing, while a pending finding's suspect path still differs from the run's start commit.
func TestRun_FreshRefusesWhileSuspectPathDiffers(t *testing.T) {
	fx := newRunFixture(t, 1)
	tracked := filepath.Join(fx.Worktree, "base.txt")
	start := seedFreshPendingState(t, fx, tracked)
	gitkit.CommitFile(t, fx.Worktree, "base.txt", "hand-edited by master", "suspect write")
	marker := filepath.Join(fx.Deps.Geom.ReportsDir, "marker.yaml")
	if err := os.WriteFile(marker, []byte("x"), 0o644); err != nil {
		t.Fatalf("write marker: %v", err)
	}

	_, err := websterengine.Run(fx.Deps, websterengine.RunOptions{Fresh: true})
	if !errors.Is(err, websterengine.ErrPendingAuditFindings) {
		t.Fatalf("Run() error = %v; want ErrPendingAuditFindings", err)
	}
	if !strings.Contains(err.Error(), tracked) || !strings.Contains(err.Error(), start) {
		t.Errorf("Run() error = %q; want it to name %s and the start commit %s", err, tracked, start)
	}
	if _, statErr := os.Stat(filepath.Join(fx.Deps.Geom.WebsterDir, "state.json")); statErr != nil {
		t.Errorf("state.json was archived: %v", statErr)
	}
	if _, statErr := os.Stat(marker); statErr != nil {
		t.Errorf("reports dir was archived: %v", statErr)
	}
	if got := fx.Starter.callCount(); got != 0 {
		t.Errorf("Starter calls = %d; want 0", got)
	}
}

// TestRun_FreshContractFileEvidence proves --fresh refuses a pending finding on a contract file no Master write cleared,
// naming the delete route and --fresh as the re-run, and drops it once the file is absent.
func TestRun_FreshContractFileEvidence(t *testing.T) {
	fx := newRunFixture(t, 1)
	contract := websterengine.OutcomePath(fx.Deps.Geom.WebsterDir)
	seedFreshPendingState(t, fx, contract)
	if err := os.WriteFile(contract, []byte("outcome: done\n"), 0o644); err != nil {
		t.Fatalf("write contract file: %v", err)
	}

	_, err := websterengine.Run(fx.Deps, websterengine.RunOptions{Fresh: true})
	if !errors.Is(err, websterengine.ErrPendingAuditFindings) {
		t.Fatalf("Run() error = %v; want ErrPendingAuditFindings", err)
	}
	requireWayForward(t, err, "rm "+contract, "lyx webster run --fresh")
	if got := fx.Starter.callCount(); got != 0 {
		t.Errorf("Starter calls = %d; want 0", got)
	}

	if err := os.Remove(contract); err != nil {
		t.Fatalf("remove contract file: %v", err)
	}
	askingMaster(t, fx, "contract absent")
	_, err = websterengine.Run(fx.Deps, websterengine.RunOptions{Fresh: true})
	requireReachedMaster(t, fx, err)
}

// TestRun_FreshDropsFindingsOnceReset proves --fresh drops the pending finding once the branch is reset to the start commit, spawns Master, and names the dropped finding in the warnings.
func TestRun_FreshDropsFindingsOnceReset(t *testing.T) {
	const session = "master-session-fresh"
	fx := newRunFixture(t, 1)
	tracked := filepath.Join(fx.Worktree, "base.txt")
	start := seedFreshPendingState(t, fx, tracked)
	gitkit.CommitFile(t, fx.Worktree, "base.txt", "hand-edited by master", "suspect write")
	gitkit.Git(t, fx.Worktree, "reset", "--hard", start)

	forks := []shuttleengine.ForkReport{{TranscriptPath: "/transcripts/fork1.jsonl", ReportReturned: true}}
	fx.Starter.handle = auditDoneHandle(t, fx, session, 1, shuttleengine.ForkAudit{Forks: forks}, func() {
		st, err := websterengine.LoadState(fx.Deps.Geom.WebsterDir, fx.Deps.Geom.ScratchDir)
		if err != nil || st == nil {
			t.Fatalf("LoadState() = %v, %v", st, err)
		}
		if len(st.PendingAuditFindings) != 0 {
			t.Errorf("PendingAuditFindings = %+v; want none in the fresh state", st.PendingAuditFindings)
		}
		st.Batches[1] = &websterengine.BatchState{Slug: "batch1", Kind: "fork", Terminal: true, Status: "done", SessionID: session}
		if err := websterengine.SaveState(fx.Deps.Geom.WebsterDir, fx.Deps.Geom.ScratchDir, st); err != nil {
			t.Fatalf("SaveState() error = %v", err)
		}
	})
	seedShuttleRunState(t, fx.ShuttleRunRoot, "master-strand-audit", session)

	result, err := websterengine.Run(fx.Deps, websterengine.RunOptions{Fresh: true})
	if err != nil {
		t.Fatalf("Run() error = %v; want nil", err)
	}
	if fx.Starter.callCount() == 0 {
		t.Error("Starter was never reached")
	}
	if !warningsContain(result.Warnings, "--fresh dropped pending audit finding sess/parent:write:1") {
		t.Errorf("Warnings = %v; want the dropped finding named", result.Warnings)
	}
	st, err := websterengine.LoadState(fx.Deps.Geom.WebsterDir, fx.Deps.Geom.ScratchDir)
	if err != nil {
		t.Fatalf("LoadState() error = %v", err)
	}
	if len(st.PendingAuditFindings) != 0 || st.RunGUID == "stale-run" {
		t.Errorf("state = %+v; want a re-initialised run with no pending finding", st)
	}
}

// TestRun_FreshDropsPathlessFinding proves a pathless pending finding on an unchanged plan is dropped by --fresh, which re-initialises and proceeds.
func TestRun_FreshDropsPathlessFinding(t *testing.T) {
	fx := newRunFixture(t, 1)
	seedFreshPendingState(t, fx)
	askingMaster(t, fx, "pathless")
	var logs bytes.Buffer
	logger.SetOutput(&logs)
	t.Cleanup(func() { logger.SetOutput(os.Stderr) })

	_, err := websterengine.Run(fx.Deps, websterengine.RunOptions{Fresh: true})
	requireReachedMaster(t, fx, err)
	// Master ends asking, so no RunResult carries the drop warning; the log does.
	if !strings.Contains(logs.String(), "--fresh dropped pending audit finding sess/parent:write:1") {
		t.Errorf("log = %q; want the drop warning naming the finding", logs.String())
	}
	st, err := websterengine.LoadState(fx.Deps.Geom.WebsterDir, fx.Deps.Geom.ScratchDir)
	if err != nil {
		t.Fatalf("LoadState() error = %v", err)
	}
	if len(st.PendingAuditFindings) != 0 || st.RunGUID == "stale-run" {
		t.Errorf("state = %+v; want a re-initialised run with no pending finding", st)
	}
}

// seedUncheckableState seeds a state with one batch failed on an uncheckable finding, started at the fixture's first commit, and returns that start commit.
func seedUncheckableState(t *testing.T, fx *runFixture) string {
	t.Helper()
	start := gitkit.RevParse(t, fx.Worktree, "HEAD")
	seedMatchingState(t, fx, &websterengine.State{
		RunGUID: "stale-run",
		Batches: map[int]*websterengine.BatchState{
			1: {
				Slug: "batch1", Kind: "fork", StartSHA: start, Terminal: true, Status: websterengine.DigestStatusFailed,
				Digest:      &websterengine.Digest{Batch: "01-batch1", Status: websterengine.DigestStatusFailed, HeadSHA: start},
				Uncheckable: []string{"fabric-reference: cat FABRICREF/webster/state.json"},
			},
		},
	})
	return start
}

// TestRun_FreshDropsUncheckableBatch proves --fresh on an unchanged plan with HEAD at the start commit drops a batch failed on an uncheckable finding:
// the run re-initialises and the drop warning names the batch and its entries.
func TestRun_FreshDropsUncheckableBatch(t *testing.T) {
	fx := newRunFixture(t, 1)
	seedUncheckableState(t, fx)
	askingMaster(t, fx, "uncheckable")
	var logs bytes.Buffer
	logger.SetOutput(&logs)
	t.Cleanup(func() { logger.SetOutput(os.Stderr) })

	_, err := websterengine.Run(fx.Deps, websterengine.RunOptions{Fresh: true})
	requireReachedMaster(t, fx, err)
	if !strings.Contains(logs.String(), "--fresh dropped batch 01's uncheckable findings: fabric-reference: cat FABRICREF/webster/state.json") {
		t.Errorf("log = %q; want the drop warning naming batch 01 and its entry", logs.String())
	}
	st, err := websterengine.LoadState(fx.Deps.Geom.WebsterDir, fx.Deps.Geom.ScratchDir)
	if err != nil {
		t.Fatalf("LoadState() error = %v", err)
	}
	if bs := st.Batches[1]; st.RunGUID == "stale-run" || (bs != nil && len(bs.Uncheckable) != 0) {
		t.Errorf("state = %+v; want a re-initialised run without the failed batch", st)
	}
}

// TestRun_FreshRefusesUncheckableBatchPastStart proves the HEAD rule applies to a batch failed on an uncheckable finding:
// with HEAD one commit past the start, --fresh refuses naming the start commit and archives nothing.
func TestRun_FreshRefusesUncheckableBatchPastStart(t *testing.T) {
	fx := newRunFixture(t, 1)
	start := seedUncheckableState(t, fx)
	gitkit.CommitFile(t, fx.Worktree, "base.txt", "hand-edited by master", "suspect write")

	_, err := websterengine.Run(fx.Deps, websterengine.RunOptions{Fresh: true})
	if !errors.Is(err, websterengine.ErrPendingAuditFindings) {
		t.Fatalf("Run() error = %v; want ErrPendingAuditFindings", err)
	}
	if !strings.Contains(err.Error(), "is not the run's start commit "+start) {
		t.Errorf("Run() error = %q; want it to name the start commit %s", err, start)
	}
	if _, statErr := os.Stat(filepath.Join(fx.Deps.Geom.WebsterDir, "state.json")); statErr != nil {
		t.Errorf("state.json was archived: %v", statErr)
	}
}

// TestRun_FreshDivergentStartsNeedHeadBeforeEvery proves --fresh over recorded starts that share no single oldest commit never lets HEAD stand in unchecked:
// it refuses naming git merge-base while HEAD is past one of them, and drops the findings once HEAD is an ancestor of every start.
func TestRun_FreshDivergentStartsNeedHeadBeforeEvery(t *testing.T) {
	fx := newRunFixture(t, 1)
	root := gitkit.RevParse(t, fx.Worktree, "HEAD")
	left := gitkit.CommitFile(t, fx.Worktree, "left.txt", "l", "left")
	gitkit.Git(t, fx.Worktree, "reset", "--hard", root)
	right := gitkit.CommitFile(t, fx.Worktree, "right.txt", "r", "right")
	seedMatchingState(t, fx, &websterengine.State{
		RunGUID: "stale-run",
		Batches: map[int]*websterengine.BatchState{
			1: {
				Slug: "batch1", Kind: "fork", StartSHA: left, Terminal: true, Status: websterengine.DigestStatusFailed,
				Uncheckable: []string{"fabric-reference: cat FABRICREF/webster/state.json"},
			},
			2: {Slug: "batch2", Kind: "fork", StartSHA: right},
		},
	})

	_, err := websterengine.Run(fx.Deps, websterengine.RunOptions{Fresh: true})
	if !errors.Is(err, websterengine.ErrPendingAuditFindings) {
		t.Fatalf("Run() with HEAD past a start error = %v; want ErrPendingAuditFindings", err)
	}
	requireWayForward(t, err, "1) lyx webster reset --to start", "2) lyx webster run --fresh")
	if strings.Contains(err.Error(), "merge-base --octopus") {
		t.Errorf("Run() error = %q; want the reset verb, not a git merge-base command", err)
	}
	if _, statErr := os.Stat(filepath.Join(fx.Deps.Geom.WebsterDir, "state.json")); statErr != nil {
		t.Errorf("state.json was archived: %v", statErr)
	}

	gitkit.Git(t, fx.Worktree, "reset", "--hard", root)
	askingMaster(t, fx, "divergent")
	_, err = websterengine.Run(fx.Deps, websterengine.RunOptions{Fresh: true})
	requireReachedMaster(t, fx, err)
	st, err := websterengine.LoadState(fx.Deps.Geom.WebsterDir, fx.Deps.Geom.ScratchDir)
	if err != nil {
		t.Fatalf("LoadState() error = %v", err)
	}
	if st.RunGUID == "stale-run" {
		t.Errorf("RunGUID = %q; want a re-initialised run", st.RunGUID)
	}
}

// TestRun_FreshOnUnchangedPlanWithoutFindingsResumes proves --fresh stays a no-op on an unchanged plan with nothing pending: the state is kept.
func TestRun_FreshOnUnchangedPlanWithoutFindingsResumes(t *testing.T) {
	fx := newRunFixture(t, 1)
	seedMatchingState(t, fx, &websterengine.State{RunGUID: "kept-run"})
	askingMaster(t, fx, "resume")

	_, err := websterengine.Run(fx.Deps, websterengine.RunOptions{Fresh: true})
	requireReachedMaster(t, fx, err)
	st, err := websterengine.LoadState(fx.Deps.Geom.WebsterDir, fx.Deps.Geom.ScratchDir)
	if err != nil {
		t.Fatalf("LoadState() error = %v", err)
	}
	if st.RunGUID != "kept-run" {
		t.Errorf("RunGUID = %q; want %q kept", st.RunGUID, "kept-run")
	}
}

// TestRun_FreshRefusesCommitPastStart proves --fresh refuses, archiving nothing, while the suspect file is restored in the worktree but HEAD carries a commit past the run's start commit.
func TestRun_FreshRefusesCommitPastStart(t *testing.T) {
	fx := newRunFixture(t, 1)
	tracked := filepath.Join(fx.Worktree, "base.txt")
	start := seedFreshPendingState(t, fx, tracked)
	gitkit.CommitFile(t, fx.Worktree, "base.txt", "hand-edited by master", "suspect write")
	gitkit.Git(t, fx.Worktree, "checkout", start, "--", "base.txt")
	marker := filepath.Join(fx.Deps.Geom.ReportsDir, "marker.yaml")
	if err := os.WriteFile(marker, []byte("x"), 0o644); err != nil {
		t.Fatalf("write marker: %v", err)
	}

	_, err := websterengine.Run(fx.Deps, websterengine.RunOptions{Fresh: true})
	if !errors.Is(err, websterengine.ErrPendingAuditFindings) {
		t.Fatalf("Run() error = %v; want ErrPendingAuditFindings", err)
	}
	if !strings.Contains(err.Error(), "is not the run's start commit "+start) {
		t.Errorf("Run() error = %q; want it to name the start commit %s", err, start)
	}
	requireWayForward(t, err, "1) lyx webster reset --to start", "2) lyx webster run --fresh")
	if _, statErr := os.Stat(filepath.Join(fx.Deps.Geom.WebsterDir, "state.json")); statErr != nil {
		t.Errorf("state.json was archived: %v", statErr)
	}
	if _, statErr := os.Stat(marker); statErr != nil {
		t.Errorf("reports dir was archived: %v", statErr)
	}
}

// TestRun_FreshRefusesDifferingPlanPath proves --fresh refuses over an edited pending plan path with the restore-plan way forward, and drops the finding once the plan is restored.
func TestRun_FreshRefusesDifferingPlanPath(t *testing.T) {
	fx := newRunFixture(t, 1)
	card := filepath.Join(fx.PlanDir, "01-batch1.md")
	seedFreshPendingState(t, fx, card)
	addCardUses(t, fx.PlanDir, 1, "base.txt")

	_, err := websterengine.Run(fx.Deps, websterengine.RunOptions{Fresh: true})
	if !errors.Is(err, websterengine.ErrPendingAuditFindings) {
		t.Fatalf("Run() error = %v; want ErrPendingAuditFindings", err)
	}
	if !strings.Contains(err.Error(), "restore-plan") || strings.Contains(err.Error(), "git checkout") {
		t.Errorf("Run() error = %q; want restore-plan and no git checkout", err)
	}
	if _, statErr := os.Stat(filepath.Join(fx.Deps.Geom.WebsterDir, "state.json")); statErr != nil {
		t.Errorf("state.json was archived: %v", statErr)
	}

	st, err := websterengine.LoadState(fx.Deps.Geom.WebsterDir, fx.Deps.Geom.ScratchDir)
	if err != nil || st == nil {
		t.Fatalf("LoadState() = %v, %v", st, err)
	}
	if _, err := websterengine.RestorePlan(st, fx.Deps.Geom); err != nil {
		t.Fatalf("RestorePlan() error = %v", err)
	}
	askingMaster(t, fx, "restored")
	_, err = websterengine.Run(fx.Deps, websterengine.RunOptions{Fresh: true})
	requireReachedMaster(t, fx, err)
	st, err = websterengine.LoadState(fx.Deps.Geom.WebsterDir, fx.Deps.Geom.ScratchDir)
	if err != nil {
		t.Fatalf("LoadState() error = %v", err)
	}
	if len(st.PendingAuditFindings) != 0 || st.RunGUID == "stale-run" {
		t.Errorf("state = %+v; want a re-initialised run with no pending finding", st)
	}
}

// TestRun_FreshDropsPlanPathWithoutCopy proves --fresh drops a differing plan path whose recorded copy is missing from the store,
// and the drop warning names the card.
func TestRun_FreshDropsPlanPathWithoutCopy(t *testing.T) {
	fx := newRunFixture(t, 1)
	card := filepath.Join(fx.PlanDir, "01-batch1.md")
	seedFreshPendingState(t, fx, card)
	addCardUses(t, fx.PlanDir, 1, "base.txt")
	if err := os.RemoveAll(filepath.Join(fx.Deps.Geom.WebsterDir, "plan-baseline")); err != nil {
		t.Fatalf("empty the plan baseline store: %v", err)
	}
	askingMaster(t, fx, "no copy")
	var logs bytes.Buffer
	logger.SetOutput(&logs)
	t.Cleanup(func() { logger.SetOutput(os.Stderr) })

	_, err := websterengine.Run(fx.Deps, websterengine.RunOptions{Fresh: true})
	requireReachedMaster(t, fx, err)
	if !strings.Contains(logs.String(), card) || !strings.Contains(logs.String(), "recorded copy is missing") {
		t.Errorf("log = %q; want the drop warning naming %s and the missing copy", logs.String(), card)
	}
	st, err := websterengine.LoadState(fx.Deps.Geom.WebsterDir, fx.Deps.Geom.ScratchDir)
	if err != nil {
		t.Fatalf("LoadState() error = %v", err)
	}
	if len(st.PendingAuditFindings) != 0 || st.RunGUID == "stale-run" {
		t.Errorf("state = %+v; want a re-initialised run with no pending finding", st)
	}
}

// TestRun_PendingPlanPathNamesRestorePlan proves run entry over a pending plan-path finding names restore-plan and rebaseline, never a git checkout.
func TestRun_PendingPlanPathNamesRestorePlan(t *testing.T) {
	fx := newRunFixture(t, 1)
	seedFreshPendingState(t, fx, filepath.Join(fx.PlanDir, "01-batch1.md"))

	_, err := websterengine.Run(fx.Deps, websterengine.RunOptions{})
	if !errors.Is(err, websterengine.ErrPendingAuditFindings) {
		t.Fatalf("Run() error = %v; want ErrPendingAuditFindings", err)
	}
	requireWayForward(t, err, "restore-plan", "rebaseline --card")
	if strings.Contains(err.Error(), "git checkout") || strings.Contains(err.Error(), "with git") {
		t.Errorf("Run() error = %q; want no git way forward for a plan path", err)
	}
}

// TestRun_MasterSpecPromptIsRenderedPromptWithoutMasterFile pins that Run hands the rendered Master prompt straight to the spawn (the provider engine writes its own prompt.md) and writes no master.md.
func TestRun_MasterSpecPromptIsRenderedPromptWithoutMasterFile(t *testing.T) {
	fx := newRunFixture(t, 1)
	seedMatchingState(t, fx, &websterengine.State{
		Batches: map[int]*websterengine.BatchState{
			1: {Slug: "batch1", Kind: "fork", Terminal: true, Status: "done"},
		},
	})

	runToDone(t, fx, "master-strand-prompt", "master-session-prompt", nil, 1)

	prompt := fx.Starter.startCalls[0].Prompt
	if !strings.Contains(prompt, "01 — batch1") {
		t.Errorf("Spec.Prompt = %q; want the rendered Master prompt listing batch 01", prompt)
	}
	if _, err := os.Stat(filepath.Join(fx.Deps.Geom.PromptsDir, "master.md")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("master.md stat err = %v; want it absent, since Run no longer writes it", err)
	}
}
