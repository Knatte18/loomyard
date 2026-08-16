//go:build integration

// verbs_test.go covers webstercli's five git-backed/spawn-backed verbs
// (begin-batch, record-batch, recover-batch, run) through the RunCLI seam:
// a real scratch git repo backs WorktreeRoot, a real *shuttleengine.Runner
// wired over local fake shuttleengine.ReedOps/shuttleengine.Engine doubles
// is the starter/injector seam, webster's own fixture pattern — a fake
// struct alone cannot satisfy these interfaces, since a
// genuine *shuttleengine.Run's StrandGUID is only ever minted by a real
// Runner.Start), and run's own Master spawn is a local fake MasterStarter
// (mirroring websterengine's own runlevel_test.go runFakeStarter). Tests
// build a *websterCLI literal directly (bypassing Command()'s
// PersistentPreRunE) and drive one verb's cobra.Command through
// clihelp.Execute, webster's own package-local injection point for these
// tests. WEFT_SKIP_GIT=1 is set on every test that reaches a
// fabricSync call, so no real weft sibling worktree is needed; the one test
// that must PROVE fabricSync was never reached (ErrRunBusy) instead leaves
// WEFT_SKIP_GIT unset and asserts the envelope carries no fabric-sync or
// fabricengine error text -- the failure a reached fabricSync would stamp
// in this weft-less geometry.

package webstercli

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/Knatte18/loomyard/contracts/stencils"
	"github.com/Knatte18/loomyard/internal/batcher"
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/stencilstore"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// seedHubStencils populates hub's real fabricengine.StencilsDir(hub) with every shipped stencil,
// through the same stencilstore.Reconcile pass cmd/lyx's root pre-run runs -- webster's prompts are
// read from disk at call time now, so a fixture hub that is never seeded fails every verb that
// renders one.
func seedHubStencils(t *testing.T, hub string) {
	t.Helper()
	baseDir := fabricengine.StencilsDir(hub)
	if _, err := stencilstore.Reconcile(baseDir, stencils.Registry(), stencilstore.ModeProduction, ""); err != nil {
		t.Fatalf("stencilstore.Reconcile(%q) = %v; want nil error", baseDir, err)
	}
}

// newScratchRepo initializes a fresh git repo at t.TempDir() and returns its path.
func newScratchRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	mustGit(t, dir, "init")
	mustGit(t, dir, "config", "user.name", "Test User")
	mustGit(t, dir, "config", "user.email", "test@example.com")
	return dir
}

// mustGit runs a git command in dir via gitexec.RunGit.
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

// commitFile writes name/content in dir and commits it, returning the new HEAD SHA.
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

// verbsFakeReed is a hermetic shuttleengine.ReedOps double.
type verbsFakeReed struct {
	mu             sync.Mutex
	counter        int
	status         reedengine.StatusResult
	removedStrands []string
}

func (m *verbsFakeReed) AddStrand(spec reedengine.AddSpec) (reedengine.Strand, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.counter++
	guid := fmt.Sprintf("verbs-strand-%d", m.counter)
	m.status.Strands = append(m.status.Strands, reedengine.StrandStatus{GUID: guid, Live: true})
	return reedengine.Strand{GUID: guid}, nil
}

func (m *verbsFakeReed) RemoveStrand(guid string, recursive bool) (reedengine.Removed, error) {
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

func (m *verbsFakeReed) Status() (reedengine.StatusResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.status, nil
}

func (m *verbsFakeReed) SendText(guid, text string, submit bool) error { return nil }
func (m *verbsFakeReed) SendKey(guid, key string) error                { return nil }
func (m *verbsFakeReed) CapturePane(guid string) (string, error)       { return "", nil }

var _ shuttleengine.ReedOps = (*verbsFakeReed)(nil)

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
	Reed     *verbsFakeReed
	Engine   *verbsFakeEngine
	Runner   *shuttleengine.Runner
	Worktree string
}

func newVerbsFixture(t *testing.T) *verbsFixture {
	t.Helper()

	worktree := newScratchRepo(t)
	commitFile(t, worktree, "base.txt", "base", "base commit")

	layout := &lyxcwd.Location{HubPath: filepath.Dir(worktree), WorktreeName: filepath.Base(worktree), AnchorRel: "."}
	seedValidPlanDir(t, loomengine.PlanDir(layout))
	seedHubStencils(t, layout.HubPath)

	reed := &verbsFakeReed{}
	engine := &verbsFakeEngine{}
	shuttleCfg := shuttleengine.Config{RunDir: filepath.Join(t.TempDir(), "runs"), RunTimeoutMin: 60, StartupTimeoutS: 30}
	runner := shuttleengine.NewRunner(reed, engine, layout, shuttleCfg)

	roles := map[websterengine.Role]modelspec.Resolved{
		websterengine.RoleMaster:   {Engine: "claude", Model: "master-model", Params: map[string]string{}},
		websterengine.RoleRecovery: {Engine: "claude", Model: "recovery-model", Params: map[string]string{}},
	}

	// The default (empty) batcher name resolves to the identity batchifier
	// -- exactly what PersistentPreRunE would have resolved via Active and
	// stored on c.batcher, bypassed here along with the rest of
	// PersistentPreRunE.
	activeBatcher, err := batcher.Select("")
	if err != nil {
		t.Fatalf("batcher.Select(\"\") error = %v", err)
	}

	c := &websterCLI{
		runner:     runner,
		starter:    runner,
		injector:   runner,
		engine:     engine,
		reed:       reed,
		layout:     layout,
		shuttleCfg: shuttleCfg,
		cfg: websterengine.Config{
			SelfFixCap:         2,
			MasterTimeoutMin:   480,
			RecoveryTimeoutMin: 60,
			PollWaitS:          1,
		},
		roles:             roles,
		batcher:           activeBatcher,
		planDir:           loomengine.PlanDir(layout),
		websterDir:        websterengine.Dir(layout),
		websterScratchDir: websterengine.ScratchDir(layout),
		reportsDir:        websterengine.ReportsDir(layout),
		promptsDir:        websterengine.PromptsDir(layout),
	}

	return &verbsFixture{CLI: c, Reed: reed, Engine: engine, Runner: runner, Worktree: worktree}
}

// testPlanFingerprint recomputes the plan-identity hash websterengine's own
// unexported fingerprint (fingerprint.go) computes -- duplicated here since
// this test lives in webstercli, an external package, and that algorithm is
// deliberately not exported. Must stay in lock-step with websterengine's
// own implementation: a SHA-256 digest over every "*.md" file's sorted name
// and contents in planDir.
func testPlanFingerprint(t *testing.T, planDir string) string {
	t.Helper()
	entries, err := os.ReadDir(planDir)
	if err != nil {
		t.Fatalf("ReadDir(%q): %v", planDir, err)
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)

	h := sha256.New()
	for _, name := range names {
		data, err := os.ReadFile(filepath.Join(planDir, name))
		if err != nil {
			t.Fatalf("ReadFile(%q): %v", name, err)
		}
		h.Write([]byte(name))
		h.Write([]byte{0})
		h.Write(data)
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

// initState writes a minimal state.json (fingerprint-matched to fx's own
// on-disk plan) for fx's webster dir, standing in for the state "lyx
// webster run" would have already created before Master ever calls
// begin-batch/record-batch/recover-batch.
func (fx *verbsFixture) initState(t *testing.T, assertedModel string) *websterengine.State {
	t.Helper()
	fp := testPlanFingerprint(t, fx.CLI.planDir)
	st := &websterengine.State{
		RunGUID:         "guid-1",
		PlanFingerprint: fp,
		MasterStrand:    "master-strand-1",
		MasterSessionID: "master-session-1",
		AssertedModel:   assertedModel,
		Batches:         map[int]*websterengine.BatchState{},
	}
	if err := websterengine.SaveState(fx.CLI.websterDir, fx.CLI.websterScratchDir, st); err != nil {
		t.Fatalf("SaveState() error = %v", err)
	}
	return st
}

// writeBatchReport seeds fx's reportsDir with a batch-report YAML file for
// batch 1 ("01-only") at its plan-format-pinned filename, status OK, using
// websterengine's own WriteReport so the on-disk shape always matches
// ParseReport's contract exactly. headSHA must equal the worktree's actual
// current HEAD when the caller drives record-batch's own terminal path
// (RecordBatch cross-checks report.HeadSHA against the live worktree HEAD);
// any non-empty placeholder is fine for await-batch (presence-only) and
// recover-batch (no such cross-check).
func writeBatchReport(t *testing.T, reportsDir, headSHA string) {
	t.Helper()
	if err := os.MkdirAll(reportsDir, 0o755); err != nil {
		t.Fatalf("mkdir reports dir: %v", err)
	}
	path := filepath.Join(reportsDir, websterengine.ReportFileName(1, "only"))
	report := &websterengine.Report{Status: websterengine.ReportStatusOK, HeadSHA: headSHA}
	if err := websterengine.WriteReport(path, report); err != nil {
		t.Fatalf("write batch report: %v", err)
	}
}

// TestBeginBatchCmd_HappyPath proves the success envelope carries prompt_path/start_sha/model,
// and that state.json was persisted with the new BatchState.
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

	loaded, err := websterengine.LoadState(fx.CLI.websterDir, fx.CLI.websterScratchDir)
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

// TestBeginBatchCmd_PausedEnvelope proves the pause refusal is an operational signal (exit 0,
// {"paused": true}), never a hard error, and that state.json is left untouched.
func TestBeginBatchCmd_PausedEnvelope(t *testing.T) {
	t.Setenv("WEFT_SKIP_GIT", "1")
	fx := newVerbsFixture(t)
	fx.initState(t, "master-model")
	if err := websterengine.RequestPause(fx.CLI.websterScratchDir); err != nil {
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

	loaded, err := websterengine.LoadState(fx.CLI.websterDir, fx.CLI.websterScratchDir)
	if err != nil || loaded == nil {
		t.Fatalf("LoadState() after paused begin-batch = %v, %v; want a state, nil", loaded, err)
	}
	if _, ok := loaded.Batches[1]; ok {
		t.Error("loaded.Batches[1] present after a paused refusal; want state untouched")
	}
}

// TestPauseCmd_ResolvesSameFileAsBeginBatchGate proves the CLI pause verb and begin-batch's own
// pause gate resolve the exact same file -- through the CLI itself, never by calling the engine
// accessor twice, since a pause verb that still writes the durable dir while begin-batch reads the
// scratch dir would leave pause silently non-functional with no test on either side alone failing.
func TestPauseCmd_ResolvesSameFileAsBeginBatchGate(t *testing.T) {
	t.Setenv("WEFT_SKIP_GIT", "1")
	fx := newVerbsFixture(t)
	fx.initState(t, "master-model")

	var pauseOut strings.Builder
	exitCode := clihelp.Execute(fx.CLI.pauseCmd(), &pauseOut, nil)
	if exitCode != 0 {
		t.Fatalf("pause() = %d; want 0, output: %s", exitCode, pauseOut.String())
	}
	if !strings.Contains(pauseOut.String(), `"paused":true`) {
		t.Errorf("pause() output missing paused:true; got %q", pauseOut.String())
	}

	if !websterengine.PauseRequested(fx.CLI.websterScratchDir) {
		t.Error("PauseRequested(fx.CLI.websterScratchDir) = false after pause(); want true")
	}
	if websterengine.PauseRequested(fx.CLI.websterDir) {
		t.Error("PauseRequested(fx.CLI.websterDir) = true; want the pause flag only under the scratch dir, never the durable dir")
	}

	var beginOut strings.Builder
	exitCode = clihelp.Execute(fx.CLI.beginBatchCmd(), &beginOut, []string{"1"})
	if exitCode != 0 {
		t.Fatalf("begin-batch 1 after pause() = %d; want 0, output: %s", exitCode, beginOut.String())
	}
	if !strings.Contains(beginOut.String(), `"paused":true`) {
		t.Errorf("begin-batch's own gate did not see pause() written by the CLI verb; output missing paused:true, got %q", beginOut.String())
	}
}

// TestAwaitBatchCmd_ReportPresenceEnvelope proves await-batch's two envelopes: {"report": true} the
// moment the batch's report file exists,
// and {"report": false} once the bounded wait elapses with no report -- NoReport_WindowElapses
// passes --wait 1ns explicitly to keep the window near-instant, versus the production default
// (websterengine.DefaultAwaitWaitS, ~30s) used whenever --wait is omitted -- with no state.json
// ever read or written, since the verb is deliberately stateless.
func TestAwaitBatchCmd_ReportPresenceEnvelope(t *testing.T) {
	fx := newVerbsFixture(t)

	t.Run("NoReport_WindowElapses", func(t *testing.T) {
		var out strings.Builder
		exitCode := clihelp.Execute(fx.CLI.awaitBatchCmd(), &out, []string{"1", "--wait", "1ns"})
		if exitCode != 0 {
			t.Fatalf("await-batch 1 = %d; want 0, output: %s", exitCode, out.String())
		}
		if !strings.Contains(out.String(), `"report":false`) {
			t.Errorf("output missing report:false; got %q", out.String())
		}
	})

	t.Run("ReportPresent", func(t *testing.T) {
		writeBatchReport(t, fx.CLI.reportsDir, "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef")
		var out strings.Builder
		exitCode := clihelp.Execute(fx.CLI.awaitBatchCmd(), &out, []string{"1"})
		if exitCode != 0 {
			t.Fatalf("await-batch 1 = %d; want 0, output: %s", exitCode, out.String())
		}
		got := out.String()
		for _, want := range []string{`"batch":"01-only"`, `"report":true`} {
			if !strings.Contains(got, want) {
				t.Errorf("output missing %q; got %q", want, got)
			}
		}
	})

	// Statelessness: neither call may have created a state.json.
	loaded, err := websterengine.LoadState(fx.CLI.websterDir, fx.CLI.websterScratchDir)
	if err != nil {
		t.Fatalf("LoadState() error = %v", err)
	}
	if loaded != nil {
		t.Error("await-batch created or mutated state.json; the verb must be stateless")
	}
}

// TestRecordBatchCmd_Envelope proves record-batch's two envelopes over an identical fixture (one
// new fork transcript already present): the terminal success envelope -- the digest verbatim plus
// warnings -- once a matching batch report has also landed,
// and the {"no_report": true} ladder signal (not an error) when the report has not landed yet.
func TestRecordBatchCmd_Envelope(t *testing.T) {
	tests := []struct {
		name          string
		writeReport   bool
		wantSubstrs   []string
		wantTerminal  bool
		wantDigestSet bool
	}{
		{
			name:          "DigestEnvelope",
			writeReport:   true,
			wantSubstrs:   []string{`"batch":"01-only"`, `"status":"done"`},
			wantTerminal:  true,
			wantDigestSet: true,
		},
		{
			name:          "NoReportEnvelope",
			writeReport:   false,
			wantSubstrs:   []string{`"no_report":true`, `"batch":"01-only"`},
			wantTerminal:  false,
			wantDigestSet: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("WEFT_SKIP_GIT", "1")
			fx := newVerbsFixture(t)
			st := fx.initState(t, "master-model")
			startSHA := commitFile(t, fx.Worktree, "internal/only/impl.go", "package only\n", "01.1: add impl")
			st.Batches[1] = &websterengine.BatchState{Slug: "only", StartSHA: startSHA, Kind: "fork"}
			st.CurrentBatch = 1
			if err := websterengine.SaveState(fx.CLI.websterDir, fx.CLI.websterScratchDir, st); err != nil {
				t.Fatalf("SaveState() error = %v", err)
			}
			fx.Engine.auditForks = shuttleengine.ForkAudit{
				Forks: []shuttleengine.ForkReport{{TranscriptPath: "subagents/fork1.jsonl", ReportReturned: true}},
			}
			if tt.writeReport {
				writeBatchReport(t, fx.CLI.reportsDir, startSHA)
			}
			// Else: deliberately never call writeBatchReport, since the
			// no-report scenario proves the report-not-landed-yet ladder.

			var out strings.Builder
			exitCode := clihelp.Execute(fx.CLI.recordBatchCmd(), &out, []string{"1"})

			if exitCode != 0 {
				t.Fatalf("record-batch 1 = %d; want 0, output: %s", exitCode, out.String())
			}
			got := out.String()
			for _, want := range tt.wantSubstrs {
				if !strings.Contains(got, want) {
					t.Errorf("output missing %q; got %q", want, got)
				}
			}
			// Only the digest-envelope row's report carries a real head_sha
			// to cross-check; the no-report row never reaches that code path.
			if tt.writeReport {
				want := fmt.Sprintf(`"head_sha":%q`, startSHA)
				if !strings.Contains(got, want) {
					t.Errorf("output missing %q; got %q", want, got)
				}
			}

			loaded, err := websterengine.LoadState(fx.CLI.websterDir, fx.CLI.websterScratchDir)
			if err != nil || loaded == nil {
				t.Fatalf("LoadState() after record-batch = %v, %v; want a state, nil", loaded, err)
			}
			if loaded.Batches[1].Terminal != tt.wantTerminal {
				t.Errorf("loaded.Batches[1].Terminal = %v; want %v", loaded.Batches[1].Terminal, tt.wantTerminal)
			}
			gotDigestSet := loaded.Batches[1].Digest != nil
			if gotDigestSet != tt.wantDigestSet {
				t.Errorf("loaded.Batches[1].Digest set = %v; want %v", gotDigestSet, tt.wantDigestSet)
			}
		})
	}
}

// TestRecoverBatchCmd_RunningThenTerminal drives recover-batch across two calls against the same
// batch: the first call performs the spawn and returns a running snapshot (the strand has no report
// yet), proving the running envelope touches neither status nor digest fields;
// the second call ATTACHES to the already-spawned strand and, once the report has landed in
// between, classifies terminal, proving the digest envelope and that state.json/the report were
// both weft-committed by then.
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

	loaded, err := websterengine.LoadState(fx.CLI.websterDir, fx.CLI.websterScratchDir)
	if err != nil || loaded == nil {
		t.Fatalf("LoadState() after spawn = %v, %v; want a state, nil", loaded, err)
	}
	bs, ok := loaded.Batches[1]
	if !ok || bs.Kind != "recovery" || bs.StrandGUID == "" {
		t.Fatalf("loaded.Batches[1] = %+v; want a recorded recovery strand after the spawn call", bs)
	}

	// Between the two calls, the recovery implementer "finishes": its
	// report lands on disk, self-reporting the worktree's real HEAD (the
	// recovery path cross-checks head_sha against the worktree exactly like
	// record-batch does).
	writeBatchReport(t, fx.CLI.reportsDir, strings.TrimSpace(mustGit(t, fx.Worktree, "rev-parse", "HEAD")))

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

	loaded, err = websterengine.LoadState(fx.CLI.websterDir, fx.CLI.websterScratchDir)
	if err != nil || loaded == nil {
		t.Fatalf("LoadState() after terminal attach = %v, %v; want a state, nil", loaded, err)
	}
	if !loaded.Batches[1].Terminal {
		t.Error("loaded.Batches[1].Terminal = false; want true after a done digest")
	}
}

// TestRunCmd_ErrRunBusySkipsWeftBackstop proves the ErrRunBusy refusal never reaches Master's own
// spawn and never runs the exit-time fabric backstop -- WEFT_SKIP_GIT is deliberately left UNSET here
// so that an accidental fabricSync call would fail loudly: with no weft sibling on disk,
// fabricengine.Open's stat validation errors and run's envelope would carry "fabric sync failed" plus
// fabricengine's missing-path text, both asserted absent below. (The pre-cutover evidence --
// weftengine creating the weft lock dir on disk -- no longer exists: fabric creates nothing before
// validation, so output text is the reachable-fabricSync signal now.)
func TestRunCmd_ErrRunBusySkipsWeftBackstop(t *testing.T) {
	fx := newVerbsFixture(t)
	starter := &verbsFakeMasterStarter{}
	fx.CLI.masterStarter = starter

	if err := os.MkdirAll(fx.CLI.websterScratchDir, 0o755); err != nil {
		t.Fatalf("mkdir webster scratch dir: %v", err)
	}
	held, err := lock.AcquireWriteLock(filepath.Join(fx.CLI.websterScratchDir, "run.lock"))
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
	// A reached fabricSync in this weft-less geometry fails at
	// fabricengine.Open and stamps both strings below into the envelope --
	// their absence is the post-cutover proof the backstop never ran. (The
	// old proof, weftengine's on-disk lock-dir creation, no longer exists:
	// fabric creates nothing before its path validation.)
	if strings.Contains(out.String(), "fabric sync failed") {
		t.Errorf("output mentions a fabric sync failure; ErrRunBusy must skip the fabric backstop entirely: %q", out.String())
	}
	if strings.Contains(out.String(), "fabricengine:") {
		t.Errorf("output carries a fabricengine error; ErrRunBusy must return before any fabric call: %q", out.String())
	}
}

// seedPersistentPreRunFixture returns a fresh real hub with shuttle/reed/webster/batcher config
// seeded (batcher.yaml's raw content is caller-supplied, so a test can override its active: key) --
// unlike every other test in this file, this one drives Command()'s real PersistentPreRunE (never
// bypassing it with a hand-built *websterCLI literal), since load-time batcher selection is wired
// there (PersistentPreRunE, now via batcher.Active). Callers pass h.PrimeWorktree() to RunCLIIn
// explicitly rather than relying on a chdir'd process cwd.
func seedPersistentPreRunFixture(t *testing.T, batcherConfig string) *hubforge.Hub {
	t.Helper()
	h := hubforge.NewHub(t, ".")
	hubforge.SeedConfig(t, h, map[string]string{
		"shuttle": shuttleengine.ConfigTemplate(),
		"reed":    reedengine.ConfigTemplate(),
		"webster": websterengine.ConfigTemplate(),
		"batcher": batcherConfig,
	})
	return h
}

// TestPersistentPreRunE_UnknownBatcherFailsFast proves the load-time batcher selection
// (batcher.Active(baseDir), wired into PersistentPreRunE) is a true fail-fast gate: an unknown
// batcher.yaml active: name aborts before any verb's RunE ever runs, with an output.Err envelope
// naming the bad batcher key -- proven here via the `status` verb, which never itself touches the
// batcher.
// This file stays serial: no t.Parallel() is added here even though this test's own chdir is gone,
// because internal/webstercli/verbs_test.go is not one of the three files the Shared Decision grants
// t.Parallel() to, and this file's other tests already call t.Setenv("WEFT_SKIP_GIT", …), which
// panics under t.Parallel() exactly as t.Chdir did.
func TestPersistentPreRunE_UnknownBatcherFailsFast(t *testing.T) {
	batcherConfig := strings.Replace(batcher.ConfigTemplate(), `active: ""`, `active: "bogus"`, 1)
	h := seedPersistentPreRunFixture(t, batcherConfig)

	var out strings.Builder
	exitCode := RunCLIIn(h.PrimeWorktree(), &out, []string{"status"})

	if exitCode != 1 {
		t.Fatalf("status with an unknown batcher = %d; want 1, output: %s", exitCode, out.String())
	}
	got := out.String()
	if !strings.Contains(got, `"ok":false`) {
		t.Errorf("output missing ok:false; got %q", got)
	}
	// The message is JSON-encoded (its literal quotes become \"), so match
	// the two substrings separately rather than the raw Go-quoted form.
	if !strings.Contains(got, "unknown batcher") || !strings.Contains(got, "bogus") {
		t.Errorf("output missing the unknown-batcher message; got %q", got)
	}
}

// TestPersistentPreRunE_DefaultBatcherResolves proves the default (empty) batcher.yaml active: key
// resolves to the identity batchifier and the command proceeds normally through the rest of
// PersistentPreRunE and into the verb's own RunE.
// This file stays serial: no t.Parallel() is added here even though this test's own chdir is gone,
// because internal/webstercli/verbs_test.go is not one of the three files the Shared Decision grants
// t.Parallel() to, and this file's other tests already call t.Setenv("WEFT_SKIP_GIT", …), which
// panics under t.Parallel() exactly as t.Chdir did.
func TestPersistentPreRunE_DefaultBatcherResolves(t *testing.T) {
	h := seedPersistentPreRunFixture(t, batcher.ConfigTemplate())

	var out strings.Builder
	exitCode := RunCLIIn(h.PrimeWorktree(), &out, []string{"status"})

	if exitCode != 0 {
		t.Fatalf("status with the default batcher = %d; want 0, output: %s", exitCode, out.String())
	}
	if !strings.Contains(out.String(), `"initialized":false`) {
		t.Errorf("output missing initialized:false; got %q", out.String())
	}
}
