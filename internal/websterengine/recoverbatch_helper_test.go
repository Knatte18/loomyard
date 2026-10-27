// recoverbatch_helper_test.go holds the RecoverBatch fixture the tier0 and tier1 recover-batch tests share:
// a scripted clock, the fully-wired RecoverDeps over a temp worktree and the driver that composes the recovery phases.
// The fixtures are package-local: the internal and external test packages deliberately do not share a test-helper package.

package websterengine_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/batcher"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/shuttlefake"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// recoverFakeClock is a package-local, scriptable clock double.
// Now starts at a fixed base and only advances when Sleep is called or a test directly mutates Now, so a test controls exactly how much virtual time elapses without ever blocking for real.
// That includes simulating the wall-clock gap between two separate RecoverBatch calls, which a real process boundary would otherwise supply for free.
type recoverFakeClock struct {
	now time.Time
}

func (c *recoverFakeClock) Now() time.Time        { return c.now }
func (c *recoverFakeClock) Sleep(d time.Duration) { c.now = c.now.Add(d) }

var _ websterengine.Clock = (*recoverFakeClock)(nil)

// namedEngines registers engine under name, which is also the set's default.
func namedEngines(name string, engine shuttleengine.Engine) shuttleengine.EngineSet {
	return shuttleengine.NewEngineSet(name, map[string]shuttleengine.Engine{name: engine})
}

// recoverFixture is a fully-wired set of RecoverBatch dependencies.
// A temp directory holds base.txt as WorktreeRoot over a fakeGit at one commit, with a one-batch plan backed by a seeded plan dir and its corresponding execution-batch list.
// A real *shuttleengine.Runner over shuttlefake.Reed and shuttlefake.Engine is the Starter, and webster's two roles are pre-resolved.
type recoverFixture struct {
	Deps       websterengine.RecoverDeps
	Reed       *shuttlefake.Reed
	Engine     *shuttlefake.Engine
	Git        *fakeGit
	Worktree   string
	ReportsDir string
	// Index is the fake index behind Deps.Geom.Index, nil over a real repository.
	Index *fakeIndex
	// Head returns the worktree's current HEAD commit.
	Head func(t *testing.T) string
}

func newRecoverFixture(t *testing.T) *recoverFixture {
	t.Helper()
	worktree := t.TempDir()
	writeWorktreeFile(t, worktree, "base.txt", "base")
	git := newFakeGit()
	fx := newRecoverFixtureOver(t, worktree, git)
	fx.Git = git
	fx.Head = func(*testing.T) string { return git.head }
	return fx
}

// newRecoverFixtureOver builds the fixture over worktree, answering git questions from git; a nil git means the real repository at worktree.
func newRecoverFixtureOver(t *testing.T, worktree string, git websterengine.Git) *recoverFixture {
	t.Helper()

	fakeIdx, index := indexOver(git)

	plan := &planparser.Plan{}
	batches := []batcher.Batch{
		{Cards: []planparser.Card{{Number: 1, Slug: "json-flag", Title: "json-flag", Intent: "add the --json flag"}}},
	}

	reed := &shuttlefake.Reed{}
	engine := &shuttlefake.Engine{}
	hubPath := filepath.Dir(worktree)
	// webster's prompts are read from disk at call time, so the fixture's hub must carry them before RecoverBatch reaches RenderRecoveryPrompt.
	seedHubStencils(t, hubPath)
	shuttleCfg := shuttleengine.Config{RunDir: t.TempDir(), RunTimeoutMin: 60, StartupTimeoutS: 30}
	runner := shuttleengine.NewRunner(reed, shuttlefake.EngineSet(engine), worktree, worktree, shuttleCfg)

	roles := map[websterengine.Role]shuttleengine.ModelChoice{
		websterengine.RoleMaster:   {Model: "master-model"},
		websterengine.RoleRecovery: {Model: "recovery-model", Effort: "high"},
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
		Gate:       testGateConfig(),
		Engines:    shuttlefake.EngineSet(engine),
		Reed:       reed,
		Stopper:    runner,
		ShuttleCfg: shuttleCfg,
		Geom: websterengine.Geometry{
			AnchorRoot:   worktree,
			WorktreeRoot: worktree,
			WebsterDir:   websterDir,
			ReportsDir:   reportsDir,
			StencilsDir:  fabricengine.StencilsDir(hubPath),
			SpecsDir:     fabricengine.SpecsDir(hubPath),
			// A real (empty) plan directory: the terminal recovery path runs the same post-batch mechanical pass record-batch does, which re-baselines the plan fingerprint over this directory.
			// No card in this fixture declares a handle, so nothing is ever written into it.
			PlanDir: planDir,
			Git:     git,
			Index:   index,
		},
	}

	return &recoverFixture{Deps: deps, Reed: reed, Engine: engine, Worktree: worktree, ReportsDir: reportsDir, Index: fakeIdx}
}

// writeRecoverReport seeds fx's reportsDir with a batch-report YAML file for batch 1 at its plan-format-pinned filename.
func writeRecoverReport(t *testing.T, reportsDir, content string) {
	t.Helper()
	path := filepath.Join(reportsDir, websterengine.ReportFileName(1, "json-flag"))
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write batch report: %v", err)
	}
}

// appendClaudeHook appends one recorded Claude hook, its stamp line at at and its payload line, to batch 1's recovery events file.
func appendClaudeHook(t *testing.T, fx *recoverFixture, hook string, at time.Time, payload map[string]any) {
	t.Helper()
	payload["hook_event_name"] = hook
	line, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal %s payload: %v", hook, err)
	}
	path := fx.Deps.State.Batches[1].EventsPath
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("open events file %s: %v", path, err)
	}
	defer f.Close()
	if _, err := fmt.Fprintf(f, "{\"lym_stamp\":%q,\"lym_at\":%q}\n%s\n", hook, at.UTC().Format(time.RFC3339), line); err != nil {
		t.Fatalf("append %s hook to %s: %v", hook, path, err)
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
	head := fx.Head(t)
	writeRecoverReport(t, fx.ReportsDir, "status: OK\nhead_sha: "+head+"\n")
	return head
}

// recoverDriveResult mirrors the composed per-call outcome the CLI verb assembles from the three lease-scoped phases, so each test keeps asserting one call's whole effect.
type recoverDriveResult struct {
	Digest   *websterengine.Digest
	Running  bool
	Spawned  bool
	ElapsedS int
	Warnings []string
}

// driveRecoverBatch composes RecoverSpawnOrAttach, RecoverAwait and PersistRecoveryTerminal against deps' in-memory state.
// That is exactly the sequence webstercli's recover-batch verb drives, minus the lease and the SaveState and fabric steps the CLI owns, so every re-entrancy and classification test exercises the composition production runs.
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
