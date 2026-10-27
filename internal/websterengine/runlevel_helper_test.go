// runlevel_helper_test.go holds the Run fixture the tier0 and tier1 run-level tests share:
// the scripted Master starter and handle, the seeded plan directory and the fully-wired RunDeps over a temp worktree.
// The fixtures are package-local: the internal and external test packages deliberately do not share a test-helper package.

package websterengine_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/batcher"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/plankit"
	"github.com/Knatte18/loomyard/internal/testkit/shuttlefake"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// recordingStopper is a websterengine.StrandStopper that records every guid it is asked to stop, in order, and answers Err.
type recordingStopper struct {
	Stopped []string
	Err     error
}

func (s *recordingStopper) StopStrand(guid string) error {
	s.Stopped = append(s.Stopped, guid)
	return s.Err
}

// runFakeHandle is a hermetic websterengine.MasterHandle double.
// Wait runs the caller-scripted onWait side effect, if any, modeling Master writing its contract files during the call Wait blocks on, then returns the scripted Result and error.
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

// runFakeStarter is a hermetic websterengine.MasterStarter double.
// It records every spec and gate StartMaster was called with and hands back the caller-scripted handle, or startErr when the caller wants to prove a step never reaches the spawn at all.
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

// seedRunPlanDir writes a syntactically complete, validation-clean plan with numCards cards into a fresh temp plan directory.
// Each card carries a format-4 Create group whose single target is the card's own new-file path, so path-missing never fires: a Create group's targets stay exempt from on-disk existence checking.
// The overview carries no plan-level "## verify:" section, deliberately, so the verify gate passes without running anything for every fixture built on this helper.
// A test that needs one calls appendIntegrationVerify against an already-seeded plan dir.
// numCards == 0 yields a "## Card Index" section with no entries at all, which ParsePlan's own parseCardIndex refuses loud ("no card index entries found"), the vehicle for the zero-batch refusal test.
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

// seedShuttleRunState hand-seeds a run.json under runDirRoot naming strandGUID and sessionID.
// That satisfies shuttleengine.FindRun's cross-process scan the way a real *shuttleengine.Runner.Start would, without driving one: Run's own fake MasterStarter never touches the real run-dir machinery.
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
		Engine:     shuttlefake.EngineName,
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

// runFixture is a fully-wired set of Run dependencies.
// A temp directory holds base.txt as WorktreeRoot over a fakeGit at one commit, beside a real on-disk plan directory, a fake reed and engine, and a fake Starter a test scripts per case.
type runFixture struct {
	Deps           websterengine.RunDeps
	Stopper        *recordingStopper
	Starter        *runFakeStarter
	Git            *fakeGit
	Worktree       string
	PlanDir        string
	ShuttleRunRoot string
}

func newRunFixture(t *testing.T, numCards int) *runFixture {
	t.Helper()
	worktree := t.TempDir()
	writeWorktreeFile(t, worktree, "base.txt", "base")
	git := newFakeGit()
	fx := newRunFixtureOver(t, numCards, worktree, git)
	fx.Git = git
	return fx
}

// newRunFixtureOver builds the fixture over worktree, answering git questions from git.
// A nil git means the real repository at worktree.
func newRunFixtureOver(t *testing.T, numCards int, worktree string, git websterengine.Git) *runFixture {
	t.Helper()

	_, index := indexOver(git)

	planDir := seedRunPlanDir(t, numCards)

	stopper := &recordingStopper{}
	starter := &runFakeStarter{}
	hubPath := filepath.Dir(worktree)
	// webster's prompts are read from disk at call time, so the fixture's hub must carry them before Run reaches RenderMasterPrompt.
	seedHubStencils(t, hubPath)
	shuttleRunRoot := t.TempDir()
	shuttleCfg := shuttleengine.Config{RunDir: shuttleRunRoot, RunTimeoutMin: 60, StartupTimeoutS: 30}

	roles := map[websterengine.Role]shuttleengine.ModelChoice{
		websterengine.RoleMaster:   {Model: "master-model"},
		websterengine.RoleRecovery: {Model: "recovery-model", Effort: "high"},
	}

	// The fixture's WorktreeRoot is a bare scratch git repo with no _lym/ tree, and Run needs no config tree, so the identity batcher is injected directly.
	activeBatcher := batcher.Identity()

	deps := websterengine.RunDeps{
		Starter:    starter,
		Stopper:    stopper,
		Engines:    shuttlefake.EngineSet(&shuttlefake.Engine{}),
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
			GateConfig:   func() (gateslot.Config, error) { return testGateConfig(), nil },
			Git:          git,
			Index:        index,
		},
		RefMatcher: websterengine.NeverMatches{},
	}

	return &runFixture{Deps: deps, Stopper: stopper, Starter: starter, Worktree: worktree, PlanDir: planDir, ShuttleRunRoot: shuttleRunRoot}
}

// writeContractFiles writes the two files Merriam's last action writes: outcome.yaml carrying outcome and summary.md carrying summary.
func writeContractFiles(t *testing.T, fx *runFixture, outcome, summary string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(fx.Deps.Geom.WebsterDir, "outcome.yaml"), []byte(outcome), 0o644); err != nil {
		t.Fatalf("write outcome.yaml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(fx.Deps.Geom.WebsterDir, "summary.md"), []byte(summary), 0o644); err != nil {
		t.Fatalf("write summary.md: %v", err)
	}
}

// writeDoneContract writes the two files Merriam's last action writes, outcome done.
func writeDoneContract(t *testing.T, fx *runFixture) {
	t.Helper()
	writeContractFiles(t, fx, "outcome: done\nstuck_reason: null\nbatches_done: 1\n", "# Shipped\n\nAll good.\n")
}

// seedMatchingState saves st into fx's webster dir after stamping its PlanFingerprint to match fx's own on-disk plan directory, and defaulting its Batches map when nil.
// Run's own fingerprint gate then passes and the pre-seeded state survives into the run unmodified.
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
