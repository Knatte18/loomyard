//go:build integration

// spawnbatch_test.go covers the spawn-batch verb's flag validation, the
// plan-validation gate's shared findingsEnvelope, the ErrPaused envelope,
// and the success envelope's field shape and weft commit -- stubbing the
// Starter seam with a real *shuttleengine.Runner wired over local fake
// MuxOps/Engine doubles, exactly mirroring how builderengine's own
// spawn_test.go fakes the same seam (a fake struct alone cannot satisfy
// Starter, since a genuine *shuttleengine.Run's StrandGUID is only ever
// minted by a real Runner.Start). Tests build a *builderCLI literal
// directly (bypassing Command()'s PersistentPreRunE) and drive one verb's
// cobra.Command through clihelp.Execute, the package-local injection point
// this batch's card calls for.

package buildercli

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/Knatte18/loomyard/internal/builderengine"
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/hubgeometry"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/muxengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// spawnFakeMux is a hermetic shuttleengine.MuxOps double: AddStrand mints a
// distinct GUID per call; every other method returns an inert zero value,
// mirroring builderengine's own spawn_test.go spawnFakeMux.
type spawnFakeMux struct {
	mu      sync.Mutex
	counter int
}

func (m *spawnFakeMux) AddStrand(spec muxengine.AddSpec) (muxengine.Strand, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.counter++
	return muxengine.Strand{GUID: "buildercli-spawn-strand-" + strconv.Itoa(m.counter)}, nil
}
func (m *spawnFakeMux) RemoveStrand(guid string, recursive bool) (muxengine.Removed, error) {
	return muxengine.Removed{}, nil
}
func (m *spawnFakeMux) Status() (muxengine.StatusResult, error)       { return muxengine.StatusResult{}, nil }
func (m *spawnFakeMux) SendText(guid, text string, submit bool) error { return nil }
func (m *spawnFakeMux) SendKey(guid, key string) error                { return nil }
func (m *spawnFakeMux) CapturePane(guid string) (string, error)       { return "", nil }

var _ shuttleengine.MuxOps = (*spawnFakeMux)(nil)

// spawnFakeEngine is a hermetic shuttleengine.Engine double: Prepare
// records every call and returns a canned Launch without writing any real
// provider artifacts, mirroring builderengine's own spawn_test.go
// spawnFakeEngine.
type spawnFakeEngine struct {
	mu           sync.Mutex
	PrepareCalls int
}

func (e *spawnFakeEngine) Prepare(runDir string, spec shuttleengine.Spec, cfg shuttleengine.Config) (shuttleengine.Launch, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.PrepareCalls++
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

// newScratchRepo initializes a fresh git repo at t.TempDir(), configures a
// test identity, and returns its path -- the same minimal recipe
// builderengine's own gitquery_test.go uses, reimplemented here since it is
// package-private there.
func newScratchRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	mustGit(t, dir, "init")
	mustGit(t, dir, "config", "user.name", "Test User")
	mustGit(t, dir, "config", "user.email", "test@example.com")
	return dir
}

// mustGit runs a git command in dir via gitexec.RunGit, failing the test on
// any spawn error or non-zero exit.
func mustGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	stdout, stderr, exitCode, err := gitexec.RunGit(args, dir)
	if err != nil {
		t.Fatalf("git %v in %s: %v", args, dir, err)
	}
	if exitCode != 0 {
		t.Fatalf("git %v in %s exited %d: %s", args, dir, exitCode, stderr)
	}
	return stdout
}

// commitFile writes name/content in dir and commits it, returning the new
// HEAD SHA.
func commitFile(t *testing.T, dir, name, content, message string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	mustGit(t, dir, "add", name)
	mustGit(t, dir, "commit", "-m", message)
	return strings.TrimSpace(mustGit(t, dir, "rev-parse", "HEAD"))
}

// spawnBatchFixture is a fully-wired *builderCLI (bypassing Command()'s
// PersistentPreRunE) plus the fakes its RunE bodies exercise, and a fresh
// copy of the plan-valid plan fixture under this fixture's own worktree's
// _lyx/plan.
type spawnBatchFixture struct {
	CLI    *builderCLI
	Engine *spawnFakeEngine
	Hub    string
}

func newSpawnBatchFixture(t *testing.T) *spawnBatchFixture {
	t.Helper()

	hub := newScratchRepo(t)
	commitFile(t, hub, "base.txt", "base", "base commit")

	seedPlanFixture(t, hub, builderengineTestdataDir("plan-valid"))

	layout := &hubgeometry.Layout{WorktreeRoot: hub, Cwd: hub, RelPath: "."}
	shuttleCfg := shuttleengine.Config{RunDir: filepath.Join(t.TempDir(), "runs"), RunTimeoutMin: 60, StartupTimeoutS: 30}
	mux := &spawnFakeMux{}
	engine := &spawnFakeEngine{}
	runner := shuttleengine.NewRunner(mux, engine, layout, shuttleCfg)

	roles := map[builderengine.Role]modelspec.Resolved{
		builderengine.RoleOrchestrator:         {Engine: "claude", Model: "orchestrator-model"},
		builderengine.RoleImplementer:          {Engine: "claude", Model: "implementer-model"},
		builderengine.RoleImplementerOversized: {Engine: "claude", Model: "implementer-oversized-model"},
		builderengine.RoleRecovery:             {Engine: "claude", Model: "recovery-model"},
	}

	c := &builderCLI{
		runner:     runner,
		starter:    runner,
		engine:     engine,
		mux:        mux,
		layout:     layout,
		shuttleCfg: shuttleCfg,
		cfg: builderengine.Config{
			SelfFixCap:            2,
			BatchTimeoutMin:       45,
			BatchContextCapTokens: 100000,
			BatchCardCap:          10,
		},
		roles:      roles,
		planDir:    hubgeometry.PlanDir(hub),
		builderDir: hubgeometry.BuilderDir(hub),
		reportsDir: hubgeometry.BuilderReportsDir(hub),
	}

	return &spawnBatchFixture{CLI: c, Engine: engine, Hub: hub}
}

// initState writes a minimal state.json for fx's builder dir, standing in
// for the state "lyx builder run" would have already created before the
// orchestrator ever calls spawn-batch.
func (fx *spawnBatchFixture) initState(t *testing.T) {
	t.Helper()
	// Record the real plan fingerprint, exactly as run's init would: SpawnBatch
	// refuses a state whose fingerprint no longer matches the on-disk plan.
	fingerprint, err := builderengine.Fingerprint(fx.CLI.planDir)
	if err != nil {
		t.Fatalf("Fingerprint(%q) error = %v", fx.CLI.planDir, err)
	}
	st := &builderengine.State{RunGUID: "guid-1", PlanFingerprint: fingerprint, Batches: map[int]*builderengine.BatchState{}}
	if err := builderengine.SaveState(fx.CLI.builderDir, st); err != nil {
		t.Fatalf("SaveState() error = %v", err)
	}
}

func TestSpawnBatchCmd_InvalidRoleRejectedBeforeEngine(t *testing.T) {
	t.Setenv("WEFT_SKIP_GIT", "1")
	fx := newSpawnBatchFixture(t)
	fx.initState(t)

	var out bytes.Buffer
	exitCode := clihelp.Execute(fx.CLI.spawnBatchCmd(), &out, []string{"1", "--role", "bogus"})

	if exitCode != 1 {
		t.Fatalf("spawn-batch --role bogus = %d; want 1, output: %s", exitCode, out.String())
	}
	if !strings.Contains(out.String(), "invalid") {
		t.Errorf("output missing invalid-role message; got %q", out.String())
	}
	if fx.Engine.PrepareCalls != 0 {
		t.Errorf("engine was reached (%d Prepare calls) for a rejected role; want zero", fx.Engine.PrepareCalls)
	}
}

func TestSpawnBatchCmd_ValidationRefusalCarriesFindings(t *testing.T) {
	t.Setenv("WEFT_SKIP_GIT", "1")
	fx := newSpawnBatchFixture(t)
	fx.initState(t)
	// Overwrite the seeded plan-valid fixture's overview with an unapproved
	// one, so the automatic gate refuses before the Starter is ever reached.
	seedPlanFixture(t, fx.Hub, builderengineTestdataDir("plan-unapproved"))

	var out bytes.Buffer
	exitCode := clihelp.Execute(fx.CLI.spawnBatchCmd(), &out, []string{"1"})

	if exitCode != 1 {
		t.Fatalf("spawn-batch on an unapproved plan = %d; want 1, output: %s", exitCode, out.String())
	}
	if !strings.Contains(out.String(), `"findings"`) {
		t.Errorf("output missing findings array; got %q", out.String())
	}
	if fx.Engine.PrepareCalls != 0 {
		t.Errorf("engine was reached (%d Prepare calls) for a refused plan; want zero", fx.Engine.PrepareCalls)
	}
}

func TestSpawnBatchCmd_NoRunInProgress(t *testing.T) {
	t.Setenv("WEFT_SKIP_GIT", "1")
	fx := newSpawnBatchFixture(t)
	// Deliberately skip fx.initState: no state.json exists yet.

	var out bytes.Buffer
	exitCode := clihelp.Execute(fx.CLI.spawnBatchCmd(), &out, []string{"1"})

	if exitCode != 1 {
		t.Fatalf("spawn-batch with no run in progress = %d; want 1, output: %s", exitCode, out.String())
	}
	if !strings.Contains(out.String(), "no run in progress") {
		t.Errorf("output missing no-run-in-progress message; got %q", out.String())
	}
}

func TestSpawnBatchCmd_PausedEnvelope(t *testing.T) {
	t.Setenv("WEFT_SKIP_GIT", "1")
	fx := newSpawnBatchFixture(t)
	fx.initState(t)
	if err := builderengine.RequestPause(fx.CLI.builderDir); err != nil {
		t.Fatalf("RequestPause() error = %v", err)
	}

	var out bytes.Buffer
	exitCode := clihelp.Execute(fx.CLI.spawnBatchCmd(), &out, []string{"1"})

	if exitCode != 1 {
		t.Fatalf("spawn-batch while paused = %d; want 1, output: %s", exitCode, out.String())
	}
	if !strings.Contains(out.String(), `"paused":true`) {
		t.Errorf("output missing paused:true; got %q", out.String())
	}
	if fx.Engine.PrepareCalls != 0 {
		t.Errorf("engine was reached (%d Prepare calls) while paused; want zero", fx.Engine.PrepareCalls)
	}
}

func TestSpawnBatchCmd_SuccessEnvelopeAndWeftCommit(t *testing.T) {
	t.Setenv("WEFT_SKIP_GIT", "1")
	fx := newSpawnBatchFixture(t)
	fx.initState(t)

	var out bytes.Buffer
	exitCode := clihelp.Execute(fx.CLI.spawnBatchCmd(), &out, []string{"1"})

	if exitCode != 0 {
		t.Fatalf("spawn-batch 1 = %d; want 0, output: %s", exitCode, out.String())
	}
	got := out.String()
	for _, want := range []string{
		`"batch_name":"01-json-flag"`, `"role":"implementer"`, `"strand_guid"`,
		`"run_dir"`, `"report_path"`, `"start_sha"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("output missing %q; got %q", want, got)
		}
	}
	if fx.Engine.PrepareCalls != 1 {
		t.Errorf("Engine.PrepareCalls = %d; want exactly 1", fx.Engine.PrepareCalls)
	}

	loaded, err := builderengine.LoadState(fx.CLI.builderDir)
	if err != nil || loaded == nil {
		t.Fatalf("LoadState() after spawn = %v, %v; want a state, nil", loaded, err)
	}
	if _, ok := loaded.Batches[1]; !ok {
		t.Errorf("loaded.Batches[1] missing after spawn-batch; state.json was not persisted to disk")
	}
}

func TestSpawnBatchCmd_RecoveryRoleOverride(t *testing.T) {
	t.Setenv("WEFT_SKIP_GIT", "1")
	fx := newSpawnBatchFixture(t)
	fx.initState(t)

	var out bytes.Buffer
	exitCode := clihelp.Execute(fx.CLI.spawnBatchCmd(), &out, []string{"1", "--role", "recovery"})

	if exitCode != 0 {
		t.Fatalf("spawn-batch 1 --role recovery = %d; want 0, output: %s", exitCode, out.String())
	}
	if !strings.Contains(out.String(), `"role":"recovery"`) {
		t.Errorf(`output missing "role":"recovery"; got %q`, out.String())
	}
}

func TestSpawnBatchCmd_InvalidBatchNumberArg(t *testing.T) {
	t.Setenv("WEFT_SKIP_GIT", "1")
	fx := newSpawnBatchFixture(t)
	fx.initState(t)

	var out bytes.Buffer
	exitCode := clihelp.Execute(fx.CLI.spawnBatchCmd(), &out, []string{"not-a-number"})

	if exitCode != 1 {
		t.Fatalf("spawn-batch not-a-number = %d; want 1, output: %s", exitCode, out.String())
	}
	if !strings.Contains(out.String(), "not a valid batch number") {
		t.Errorf("output missing batch-number error; got %q", out.String())
	}
}
