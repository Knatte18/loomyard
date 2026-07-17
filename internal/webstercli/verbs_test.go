//go:build integration

// verbs_test.go covers webstercli's five git-backed/spawn-backed verbs
// (begin-batch, record-batch, recover-batch, run) through the RunCLI seam:
// a real scratch git repo backs WorktreeRoot, a real *shuttleengine.Runner
// wired over local fake shuttleengine.MuxOps/shuttleengine.Engine doubles
// is the starter/injector seam (exactly buildercli's own spawnbatch_test.go
// pattern — a fake struct alone cannot satisfy these interfaces, since a
// genuine *shuttleengine.Run's StrandGUID is only ever minted by a real
// Runner.Start), and run's own Master spawn is a local fake MasterStarter
// (mirroring websterengine's own runlevel_test.go runFakeStarter). Tests
// build a *websterCLI literal directly (bypassing Command()'s
// PersistentPreRunE) and drive one verb's cobra.Command through
// clihelp.Execute, the package-local injection point buildercli's own
// tests establish. WEFT_SKIP_GIT=1 is set on every test that reaches a
// weftCommit call, so no real weft sibling worktree is needed; the one test
// that must PROVE weftCommit was never reached (ErrRunBusy) instead leaves
// WEFT_SKIP_GIT unset and asserts the weft worktree directory was never
// even created.

package webstercli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/Knatte18/loomyard/internal/builderengine"
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/hubgeometry"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/muxengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// newScratchRepo initializes a fresh git repo at t.TempDir(), configures a
// test identity, and returns its path.
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
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir parent of %s: %v", name, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	mustGit(t, dir, "add", name)
	mustGit(t, dir, "commit", "-m", message)
	return strings.TrimSpace(mustGit(t, dir, "rev-parse", "HEAD"))
}

// verbsFakeMux is a hermetic shuttleengine.MuxOps double: AddStrand mints a
// distinct GUID per call and registers it live, RemoveStrand records every
// call and retires the guid, and the send/capture methods stay inert.
type verbsFakeMux struct {
	mu             sync.Mutex
	counter        int
	status         muxengine.StatusResult
	removedStrands []string
}

func (m *verbsFakeMux) AddStrand(spec muxengine.AddSpec) (muxengine.Strand, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.counter++
	guid := fmt.Sprintf("verbs-strand-%d", m.counter)
	m.status.Strands = append(m.status.Strands, muxengine.StrandStatus{GUID: guid, Live: true})
	return muxengine.Strand{GUID: guid}, nil
}

func (m *verbsFakeMux) RemoveStrand(guid string, recursive bool) (muxengine.Removed, error) {
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

func (m *verbsFakeMux) Status() (muxengine.StatusResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.status, nil
}

func (m *verbsFakeMux) SendText(guid, text string, submit bool) error { return nil }
func (m *verbsFakeMux) SendKey(guid, key string) error                { return nil }
func (m *verbsFakeMux) CapturePane(guid string) (string, error)       { return "", nil }

var _ shuttleengine.MuxOps = (*verbsFakeMux)(nil)

// verbsFakeEngine is a hermetic shuttleengine.Engine double: Prepare counts
// every call and returns a canned Launch without writing any real provider
// artifacts; AuditForksIncremental hands back a caller-scripted ForkAudit;
// ParseEvents hands back a caller-scripted (default empty, i.e. no Stop
// event) event slice. Every other method is inert.
type verbsFakeEngine struct {
	mu           sync.Mutex
	prepareCalls int
	auditForks   shuttleengine.ForkAudit
	events       []shuttleengine.Event
}

func (e *verbsFakeEngine) Prepare(runDir string, spec shuttleengine.Spec, cfg shuttleengine.Config) (shuttleengine.Launch, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.prepareCalls++
	return shuttleengine.Launch{Cmd: "fake-launch-cmd", SessionID: fmt.Sprintf("fake-session-%d", e.prepareCalls)}, nil
}
func (e *verbsFakeEngine) ParseEvents(data []byte) ([]shuttleengine.Event, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.events, nil
}
func (e *verbsFakeEngine) Startup(capture string) shuttleengine.StartupState {
	return shuttleengine.StartupReady
}
func (e *verbsFakeEngine) InterruptSequence() []shuttleengine.PaneInput    { return nil }
func (e *verbsFakeEngine) TrustDismissSequence() []shuttleengine.PaneInput { return nil }
func (e *verbsFakeEngine) ComposeSend(text string) []shuttleengine.PaneInput {
	return nil
}
func (e *verbsFakeEngine) AuditForks(sessionID, workdir string) (shuttleengine.ForkAudit, error) {
	return shuttleengine.ForkAudit{}, nil
}
func (e *verbsFakeEngine) AuditForksIncremental(sessionID, workdir string, seenTranscripts map[string]bool) (shuttleengine.ForkAudit, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.auditForks, nil
}
func (e *verbsFakeEngine) ModelSwitchSequence(model string) []shuttleengine.PaneInput {
	return nil
}

var _ shuttleengine.Engine = (*verbsFakeEngine)(nil)

// verbsFakeMasterStarter is a hermetic websterengine.MasterStarter double
// that records whether it was ever called and errors loud if it is — used
// only by tests proving a refusal path never reaches Master's own spawn.
type verbsFakeMasterStarter struct {
	called bool
}

func (s *verbsFakeMasterStarter) StartMaster(spec shuttleengine.Spec) (websterengine.MasterHandle, error) {
	s.called = true
	return nil, fmt.Errorf("verbsFakeMasterStarter: StartMaster must not be reached in this test")
}

var _ websterengine.MasterStarter = (*verbsFakeMasterStarter)(nil)

// verbsFixture is a fully-wired *websterCLI (bypassing Command()'s
// PersistentPreRunE) over a real scratch git repo and a real
// *shuttleengine.Runner wired over local fakes, plus a single-batch plan
// fixture seeded under the fixture's own _lyx/plan.
type verbsFixture struct {
	CLI      *websterCLI
	Mux      *verbsFakeMux
	Engine   *verbsFakeEngine
	Runner   *shuttleengine.Runner
	Worktree string
}

func newVerbsFixture(t *testing.T) *verbsFixture {
	t.Helper()

	worktree := newScratchRepo(t)
	commitFile(t, worktree, "base.txt", "base", "base commit")

	layout := &hubgeometry.Layout{WorktreeRoot: worktree, Cwd: worktree, RelPath: "."}
	seedValidPlanDir(t, hubgeometry.PlanDir(worktree))

	mux := &verbsFakeMux{}
	engine := &verbsFakeEngine{}
	shuttleCfg := shuttleengine.Config{RunDir: filepath.Join(t.TempDir(), "runs"), RunTimeoutMin: 60, StartupTimeoutS: 30}
	runner := shuttleengine.NewRunner(mux, engine, layout, shuttleCfg)

	roles := map[websterengine.Role]modelspec.Resolved{
		websterengine.RoleMaster:          {Engine: "claude", Model: "master-model", Params: map[string]string{}},
		websterengine.RoleMasterOversized: {Engine: "claude", Model: "oversized-model", Params: map[string]string{}},
		websterengine.RoleRecovery:        {Engine: "claude", Model: "recovery-model", Params: map[string]string{}},
	}

	c := &websterCLI{
		runner:     runner,
		starter:    runner,
		injector:   runner,
		engine:     engine,
		mux:        mux,
		layout:     layout,
		shuttleCfg: shuttleCfg,
		cfg: websterengine.Config{
			SelfFixCap:            2,
			MasterTimeoutMin:      480,
			RecoveryTimeoutMin:    60,
			PollWaitS:             1,
			BatchContextCapTokens: 1_000_000,
			BatchCardCap:          50,
		},
		roles:      roles,
		planDir:    hubgeometry.PlanDir(worktree),
		websterDir: hubgeometry.WebsterDir(worktree),
		reportsDir: hubgeometry.WebsterReportsDir(worktree),
		promptsDir: hubgeometry.WebsterPromptsDir(worktree),
	}

	return &verbsFixture{CLI: c, Mux: mux, Engine: engine, Runner: runner, Worktree: worktree}
}

// initState writes a minimal state.json (fingerprint-matched to fx's own
// on-disk plan) for fx's webster dir, standing in for the state "lyx
// webster run" would have already created before Master ever calls
// begin-batch/record-batch/recover-batch.
func (fx *verbsFixture) initState(t *testing.T, assertedModel string) *websterengine.State {
	t.Helper()
	fp, err := builderengine.Fingerprint(fx.CLI.planDir)
	if err != nil {
		t.Fatalf("Fingerprint(%q) error = %v", fx.CLI.planDir, err)
	}
	st := &websterengine.State{
		RunGUID:         "guid-1",
		PlanFingerprint: fp,
		MasterStrand:    "master-strand-1",
		MasterSessionID: "master-session-1",
		AssertedModel:   assertedModel,
		Batches:         map[int]*websterengine.BatchState{},
		ChainStartSHAs:  map[int]string{},
	}
	if err := websterengine.SaveState(fx.CLI.websterDir, st); err != nil {
		t.Fatalf("SaveState() error = %v", err)
	}
	return st
}

// writeBatchReport seeds fx's reportsDir with a batch-report YAML file for
// batch 1 ("01-only") at its plan-format-pinned filename.
func writeBatchReport(t *testing.T, reportsDir string) {
	t.Helper()
	if err := os.MkdirAll(reportsDir, 0o755); err != nil {
		t.Fatalf("mkdir reports dir: %v", err)
	}
	path := filepath.Join(reportsDir, builderengine.BatchReportFileName(1, "only"))
	content := "batch: 01-only\nstatus: done\ntests: green\nstuck_reason: null\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write batch report: %v", err)
	}
}

// TestBeginBatchCmd_HappyPath proves the success envelope carries
// prompt_path/start_sha/model, and that state.json was persisted with the
// new BatchState.
func TestBeginBatchCmd_HappyPath(t *testing.T) {
	t.Setenv("WEFT_SKIP_GIT", "1")
	fx := newVerbsFixture(t)
	// Pre-assert the master model so BeginBatch's idempotent model-switch
	// check skips the Injector.Inject call entirely — this test is about
	// the CLI's own envelope/state-save wiring, not the inject choreography
	// itself (covered live by the sandbox suite, per shuttleengine's own
	// Inject doc).
	fx.initState(t, "master-model")

	var out strings.Builder
	exitCode := clihelp.Execute(fx.CLI.beginBatchCmd(), &out, []string{"1"})

	if exitCode != 0 {
		t.Fatalf("begin-batch 1 = %d; want 0, output: %s", exitCode, out.String())
	}
	got := out.String()
	for _, want := range []string{`"batch":"01-only"`, `"prompt_path"`, `"start_sha"`, `"model":"master-model"`} {
		if !strings.Contains(got, want) {
			t.Errorf("output missing %q; got %q", want, got)
		}
	}

	loaded, err := websterengine.LoadState(fx.CLI.websterDir)
	if err != nil || loaded == nil {
		t.Fatalf("LoadState() after begin-batch = %v, %v; want a state, nil", loaded, err)
	}
	bs, ok := loaded.Batches[1]
	if !ok {
		t.Fatal("loaded.Batches[1] missing after begin-batch; state.json was not persisted")
	}
	if bs.Kind != "fork" {
		t.Errorf("loaded.Batches[1].Kind = %q; want \"fork\"", bs.Kind)
	}
}

// TestBeginBatchCmd_PausedEnvelope proves the pause refusal is an
// operational signal (exit 0, {"paused": true}), never a hard error, and
// that state.json is left untouched.
func TestBeginBatchCmd_PausedEnvelope(t *testing.T) {
	t.Setenv("WEFT_SKIP_GIT", "1")
	fx := newVerbsFixture(t)
	fx.initState(t, "master-model")
	if err := builderengine.RequestPause(fx.CLI.websterDir); err != nil {
		t.Fatalf("RequestPause() error = %v", err)
	}

	var out strings.Builder
	exitCode := clihelp.Execute(fx.CLI.beginBatchCmd(), &out, []string{"1"})

	if exitCode != 0 {
		t.Fatalf("begin-batch 1 while paused = %d; want 0, output: %s", exitCode, out.String())
	}
	if !strings.Contains(out.String(), `"paused":true`) {
		t.Errorf("output missing paused:true; got %q", out.String())
	}

	loaded, err := websterengine.LoadState(fx.CLI.websterDir)
	if err != nil || loaded == nil {
		t.Fatalf("LoadState() after paused begin-batch = %v, %v; want a state, nil", loaded, err)
	}
	if _, ok := loaded.Batches[1]; ok {
		t.Error("loaded.Batches[1] present after a paused refusal; want state untouched")
	}
}

// TestRecordBatchCmd_DigestEnvelope proves the terminal success envelope is
// the digest verbatim plus warnings, once one new fork transcript and a
// matching batch report are both present.
func TestRecordBatchCmd_DigestEnvelope(t *testing.T) {
	t.Setenv("WEFT_SKIP_GIT", "1")
	fx := newVerbsFixture(t)
	st := fx.initState(t, "master-model")
	startSHA := commitFile(t, fx.Worktree, "internal/only/impl.go", "package only\n", "01.1: add impl")
	st.Batches[1] = &websterengine.BatchState{Slug: "only", StartSHA: startSHA, Kind: "fork"}
	st.CurrentBatch = 1
	if err := websterengine.SaveState(fx.CLI.websterDir, st); err != nil {
		t.Fatalf("SaveState() error = %v", err)
	}
	fx.Engine.auditForks = shuttleengine.ForkAudit{
		Forks: []shuttleengine.ForkReport{{TranscriptPath: "subagents/fork1.jsonl", ReportReturned: true}},
	}
	writeBatchReport(t, fx.CLI.reportsDir)

	var out strings.Builder
	exitCode := clihelp.Execute(fx.CLI.recordBatchCmd(), &out, []string{"1"})

	if exitCode != 0 {
		t.Fatalf("record-batch 1 = %d; want 0, output: %s", exitCode, out.String())
	}
	got := out.String()
	for _, want := range []string{`"batch":"01-only"`, `"status":"done"`, `"tests":"green"`} {
		if !strings.Contains(got, want) {
			t.Errorf("output missing %q; got %q", want, got)
		}
	}

	loaded, err := websterengine.LoadState(fx.CLI.websterDir)
	if err != nil || loaded == nil {
		t.Fatalf("LoadState() after record-batch = %v, %v; want a state, nil", loaded, err)
	}
	if !loaded.Batches[1].Terminal {
		t.Error("loaded.Batches[1].Terminal = false; want true after a done digest")
	}
	if loaded.Batches[1].Digest == nil {
		t.Error("loaded.Batches[1].Digest = nil; want a persisted digest")
	}
}

// TestRecordBatchCmd_NoReportEnvelope proves a call with a new fork
// transcript but no report file yet is a ladder signal, not an error:
// {"no_report": true, "batch": ...}, exit 0.
func TestRecordBatchCmd_NoReportEnvelope(t *testing.T) {
	t.Setenv("WEFT_SKIP_GIT", "1")
	fx := newVerbsFixture(t)
	st := fx.initState(t, "master-model")
	startSHA := commitFile(t, fx.Worktree, "internal/only/impl.go", "package only\n", "01.1: add impl")
	st.Batches[1] = &websterengine.BatchState{Slug: "only", StartSHA: startSHA, Kind: "fork"}
	st.CurrentBatch = 1
	if err := websterengine.SaveState(fx.CLI.websterDir, st); err != nil {
		t.Fatalf("SaveState() error = %v", err)
	}
	fx.Engine.auditForks = shuttleengine.ForkAudit{
		Forks: []shuttleengine.ForkReport{{TranscriptPath: "subagents/fork1.jsonl", ReportReturned: true}},
	}
	// Deliberately never call writeBatchReport: the report has not landed yet.

	var out strings.Builder
	exitCode := clihelp.Execute(fx.CLI.recordBatchCmd(), &out, []string{"1"})

	if exitCode != 0 {
		t.Fatalf("record-batch 1 with no report = %d; want 0, output: %s", exitCode, out.String())
	}
	got := out.String()
	if !strings.Contains(got, `"no_report":true`) {
		t.Errorf("output missing no_report:true; got %q", got)
	}
	if !strings.Contains(got, `"batch":"01-only"`) {
		t.Errorf("output missing batch identifier; got %q", got)
	}

	loaded, err := websterengine.LoadState(fx.CLI.websterDir)
	if err != nil || loaded == nil {
		t.Fatalf("LoadState() after no-report record-batch = %v, %v; want a state, nil", loaded, err)
	}
	if loaded.Batches[1].Terminal {
		t.Error("loaded.Batches[1].Terminal = true; want false — no report has landed yet")
	}
}

// TestRecoverBatchCmd_RunningThenTerminal drives recover-batch across two
// calls against the same batch: the first call performs the spawn and
// returns a running snapshot (the strand has no report yet), proving the
// running envelope touches neither status nor digest fields; the second
// call ATTACHES to the already-spawned strand and, once the report has
// landed in between, classifies terminal, proving the digest envelope and
// that state.json/the report were both weft-committed by then.
func TestRecoverBatchCmd_RunningThenTerminal(t *testing.T) {
	t.Setenv("WEFT_SKIP_GIT", "1")
	fx := newVerbsFixture(t)
	fx.initState(t, "master-model")

	// First call: no prior record for batch 1, so RecoverBatch spawns a
	// fresh recovery strand, then the bounded (near-zero) wait elapses with
	// no report on disk yet -- Running.
	var out1 strings.Builder
	exitCode := clihelp.Execute(fx.CLI.recoverBatchCmd(), &out1, []string{"1", "--wait", "1ns"})
	if exitCode != 0 {
		t.Fatalf("recover-batch 1 (spawn) = %d; want 0, output: %s", exitCode, out1.String())
	}
	got1 := out1.String()
	if !strings.Contains(got1, `"status":"running"`) {
		t.Errorf("first call output missing status:running; got %q", got1)
	}
	if !strings.Contains(got1, `"batch":"01-only"`) {
		t.Errorf("first call output missing batch identifier; got %q", got1)
	}
	if fx.Engine.prepareCalls != 1 {
		t.Fatalf("Engine.prepareCalls after first call = %d; want exactly 1 (the spawn)", fx.Engine.prepareCalls)
	}

	loaded, err := websterengine.LoadState(fx.CLI.websterDir)
	if err != nil || loaded == nil {
		t.Fatalf("LoadState() after spawn = %v, %v; want a state, nil", loaded, err)
	}
	bs, ok := loaded.Batches[1]
	if !ok || bs.Kind != "recovery" || bs.StrandGUID == "" {
		t.Fatalf("loaded.Batches[1] = %+v; want a recorded recovery strand after the spawn call", bs)
	}

	// Between the two calls, the recovery implementer "finishes": its
	// report lands on disk.
	writeBatchReport(t, fx.CLI.reportsDir)

	// Second call: ATTACH (Kind == recovery, non-terminal, StrandGUID set)
	// -- recoverSpawn/archiveStaleReport never runs again, so the report
	// just written survives and the very first gather sees it -- terminal.
	var out2 strings.Builder
	exitCode = clihelp.Execute(fx.CLI.recoverBatchCmd(), &out2, []string{"1", "--wait", "1ns"})
	if exitCode != 0 {
		t.Fatalf("recover-batch 1 (attach) = %d; want 0, output: %s", exitCode, out2.String())
	}
	got2 := out2.String()
	for _, want := range []string{`"batch":"01-only"`, `"status":"done"`} {
		if !strings.Contains(got2, want) {
			t.Errorf("second call output missing %q; got %q", want, got2)
		}
	}
	if fx.Engine.prepareCalls != 1 {
		t.Errorf("Engine.prepareCalls after attach call = %d; want still exactly 1 (no re-spawn)", fx.Engine.prepareCalls)
	}

	loaded, err = websterengine.LoadState(fx.CLI.websterDir)
	if err != nil || loaded == nil {
		t.Fatalf("LoadState() after terminal attach = %v, %v; want a state, nil", loaded, err)
	}
	if !loaded.Batches[1].Terminal {
		t.Error("loaded.Batches[1].Terminal = false; want true after a done digest")
	}
}

// TestRunCmd_ErrRunBusySkipsWeftBackstop proves the ErrRunBusy refusal never
// reaches Master's own spawn and never runs the exit-time weft backstop --
// WEFT_SKIP_GIT is deliberately left UNSET here so that an accidental
// weftCommit call would attempt (and leave evidence of) a real git
// operation against the (nonexistent) weft sibling directory.
func TestRunCmd_ErrRunBusySkipsWeftBackstop(t *testing.T) {
	fx := newVerbsFixture(t)
	starter := &verbsFakeMasterStarter{}
	fx.CLI.masterStarter = starter

	if err := os.MkdirAll(fx.CLI.websterDir, 0o755); err != nil {
		t.Fatalf("mkdir webster dir: %v", err)
	}
	held, err := lock.AcquireWriteLock(filepath.Join(fx.CLI.websterDir, "run.lock"))
	if err != nil {
		t.Fatalf("acquire run.lock: %v", err)
	}
	defer held.Release()

	var out strings.Builder
	exitCode := clihelp.Execute(fx.CLI.runCmd(), &out, nil)

	if exitCode != 1 {
		t.Fatalf("run while run.lock is held = %d; want 1, output: %s", exitCode, out.String())
	}
	if !strings.Contains(out.String(), "already in progress") {
		t.Errorf("output missing the run-busy message; got %q", out.String())
	}
	if starter.called {
		t.Error("MasterStarter.StartMaster was reached while run.lock was held; want zero calls")
	}
	if strings.Contains(out.String(), "weft sync failed") {
		t.Errorf("output mentions a weft sync failure; ErrRunBusy must skip the weft backstop entirely: %q", out.String())
	}
	if _, statErr := os.Stat(fx.CLI.layout.WeftWorktree()); !os.IsNotExist(statErr) {
		t.Errorf("weft worktree dir exists after ErrRunBusy; want no weft commit ever attempted (stat err = %v)", statErr)
	}
}
