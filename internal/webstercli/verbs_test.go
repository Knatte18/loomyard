//go:build integration

// verbs_test.go covers webstercli's git-backed/spawn-backed verbs (begin-batch, record-batch, recover-batch, run) through the RunCLI seam:
// a real scratch git repo backs WorktreeRoot, a real *shuttleengine.Runner wired over local fake shuttleengine.ReedOps/shuttleengine.Engine doubles is the starter/injector seam, webster's own fixture pattern — a fake struct alone cannot satisfy these interfaces, since a genuine *shuttleengine.Run's StrandGUID is only ever minted by a real Runner.Start), and run's own Master spawn is a local fake MasterStarter (mirroring websterengine's own runlevel_test.go runFakeStarter).
// Most tests build a *websterCLI literal directly (bypassing Command()'s PersistentPreRunE) and drive one verb's cobra.Command through clihelp.Execute, webster's own package-local injection point for these tests; seedPersistentPreRunFixture and its tests are the deliberate exception, driving Command()'s real PersistentPreRunE through RunCLIIn.
// WEFT_SKIP_GIT=1 is set on every test that reaches a fabricSync call, so no real records sibling worktree is needed; the one test that must PROVE fabricSync was never reached (ErrRunBusy) instead leaves WEFT_SKIP_GIT unset and asserts the envelope carries no fabric-sync or fabricengine error text -- the failure a reached fabricSync would stamp in this records-less geometry.

package webstercli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/batcher"
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/plankit"
	"github.com/Knatte18/loomyard/internal/testkit/shuttlefake"
	"github.com/Knatte18/loomyard/internal/testkit/stencilkit"
	"github.com/Knatte18/loomyard/internal/websterengine"
	"github.com/spf13/cobra"
)

// seedHubStencils populates hub's real fabricengine.StencilsDir(hub) with every shipped stencil --
// webster's prompts are read from disk at call time,
// so a fixture hub that is never seeded fails every verb that renders one.
func seedHubStencils(t *testing.T, hub string) {
	t.Helper()
	stencilkit.SeedInto(t, fabricengine.StencilsDir(hub))
}

func newScratchRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	gitkit.Git(t, dir, "init")
	gitkit.Git(t, dir, "config", "user.name", "Test User")
	gitkit.Git(t, dir, "config", "user.email", "test@example.com")
	return dir
}

// seedAnchoredGitLink makes anchorPath (a plain, .git-less subdirectory of worktree) openable as a
// git checkout by go-git's PlainOpenWithOptions, which requires a literal ".git" entry at the exact
// path it is given rather than discovering one in a parent directory. It writes a ".git" gitlink
// file at anchorPath pointing straight at worktree's own ".git" directory, so reads through it (e.g.
// gitrepo.Repo.CurrentSHA) see the SAME live refs worktree's own commits update -- unlike
// `git worktree add`, which would pin anchorPath to a detached SHA at creation time and go stale the
// moment a later commit lands in worktree.
func seedAnchoredGitLink(t *testing.T, anchorPath, worktree string) {
	t.Helper()
	if err := os.MkdirAll(anchorPath, 0o755); err != nil {
		t.Fatalf("mkdir anchor path %s: %v", anchorPath, err)
	}
	gitdir := filepath.Join(worktree, ".git")
	content := fmt.Sprintf("gitdir: %s\n", gitdir)
	if err := os.WriteFile(filepath.Join(anchorPath, ".git"), []byte(content), 0o644); err != nil {
		t.Fatalf("write anchored .git link at %s: %v", anchorPath, err)
	}
}

// verbsFakeMasterStarter is a hermetic websterengine.MasterStarter double
// that records whether it was ever called and errors loud if it is — used
// only by tests proving a refusal path never reaches Master's own spawn.
type verbsFakeMasterStarter struct {
	called bool
}

func (s *verbsFakeMasterStarter) StartMaster(spec shuttleengine.Spec, gate shuttleengine.GateSpec) (websterengine.MasterHandle, error) {
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
	Reed     *shuttlefake.Reed
	Engine   *shuttlefake.Engine
	Runner   *shuttleengine.Runner
	Worktree string
}

func newVerbsFixture(t *testing.T) *verbsFixture {
	t.Helper()

	worktree := newScratchRepo(t)
	gitkit.CommitFile(t, worktree, "base.txt", "base", "base commit")

	layout := &lyxcwd.Location{HubPath: filepath.Dir(worktree), WorktreeName: filepath.Base(worktree), AnchorRel: "backend"}
	seedValidPlanDir(t, planparser.PlanDir(layout.AnchorPath()))
	seedHubStencils(t, layout.HubPath)
	// begin-batch/record-batch/recover-batch read HEAD at layout.AnchorPath() (RunDeps.WorktreeRoot,
	// unchanged by this batch), which requires a literal .git at that path -- link the anchored
	// directory back onto worktree's real .git so it shares the same live ref/commit history rather
	// than needing its own separate repository.
	seedAnchoredGitLink(t, layout.AnchorPath(), worktree)

	reed := &shuttlefake.Reed{}
	engine := &shuttlefake.Engine{}
	engine.PrepareFn = func(string, shuttleengine.Spec, shuttleengine.Config) (shuttleengine.Launch, error) {
		return shuttleengine.Launch{Cmd: "fake-launch-cmd", SessionID: fmt.Sprintf("fake-session-%d", engine.PrepareCalls)}, nil
	}
	shuttleCfg := shuttleengine.Config{RunDir: filepath.Join(t.TempDir(), "runs"), RunTimeoutMin: 60, StartupTimeoutS: 30}
	runner := shuttleengine.NewRunner(reed, engine, layout.AnchorPath(), layout.WorktreePath(), shuttleCfg)

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
		anchorRel:  layout.AnchorRel,
		shuttleCfg: shuttleCfg,
		geom:       hubgeom.WebsterGeometry(layout),
		refMatcher: fabricengine.NewRefScanner(layout),
		openFabric: func() (*fabricengine.Fabric, error) { return fabricengine.Open(layout) },
		cfg: websterengine.Config{
			SelfFixCap:         2,
			MasterTimeoutMin:   480,
			RecoveryTimeoutMin: 60,
		},
		roles:   roles,
		batcher: activeBatcher,
	}

	return &verbsFixture{CLI: c, Reed: reed, Engine: engine, Runner: runner, Worktree: worktree}
}

// TestPersistentPreRun_OpenFabricWiredButUninvoked proves the laziness argument cli.go's own
// resolvePersistentPreRun doc comment makes: the fabric-handle opener is built as a closure and
// stored on c, but is never itself called during pre-run wiring.
// The fixture carries a hub-level board directory (so preflight.HubPresent selects hub mode) but no
// records sibling worktree at all -- fabricengine.Open stat-checks that sibling, so an eager call here
// would stat-fail and surface as a non-nil error from the "status" verb below (one of the three
// healthy-but-unwired locations the doc comment names). Reaching exit 0 is therefore itself the
// behavioural half of the proof; c.openFabric != nil is the structural half.
func TestPersistentPreRun_OpenFabricWiredButUninvoked(t *testing.T) {
	container := t.TempDir()
	worktree := filepath.Join(container, "pair")
	if err := os.MkdirAll(worktree, 0o755); err != nil {
		t.Fatalf("mkdir worktree: %v", err)
	}
	gitkit.Git(t, worktree, "init")
	gitkit.Git(t, worktree, "config", "user.name", "Test User")
	gitkit.Git(t, worktree, "config", "user.email", "test@example.com")
	gitkit.CommitFile(t, worktree, "base.txt", "base", "base commit")
	// A hub-level board directory -- with no records sibling anywhere near it -- is what selects hub
	// mode here without wiring a real, fully-paired fabric hub: preflight.HubPresent only stats
	// <hub>/_board/_lyx.
	if err := os.MkdirAll(filepath.Join(fabricengine.BoardDir(container), "_lyx"), 0o755); err != nil {
		t.Fatalf("mkdir hub board dir: %v", err)
	}

	c := &websterCLI{}
	parent := &cobra.Command{
		Use:               "webster",
		PersistentPreRunE: c.resolvePersistentPreRun,
	}
	parent.AddCommand(c.statusCmd())

	var out strings.Builder
	exitCode := clihelp.ExecuteIn(parent, worktree, &out, []string{"status"})

	if exitCode != 0 {
		t.Fatalf("status = %d; want 0 (an eager fabricengine.Open would stat-fail on the records-less fixture worktree), output: %s", exitCode, out.String())
	}
	if c.openFabric == nil {
		t.Fatal("c.openFabric = nil after PersistentPreRunE; want a wired opener closure")
	}
}

// initState writes a minimal state.json (fingerprint-matched to fx's own
// on-disk plan) for fx's webster dir, standing in for the state "lyx
// webster run" would have already created before Master ever calls
// begin-batch/record-batch/recover-batch.
func (fx *verbsFixture) initState(t *testing.T, assertedModel string) *websterengine.State {
	t.Helper()
	return seedRunState(t, fx.CLI, assertedModel)
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

	loaded, err := websterengine.LoadState(fx.CLI.geom.WebsterDir, fx.CLI.geom.ScratchDir)
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

// TestBeginBatchCmd_DeleteTargetAlreadyGone proves begin-batch dispatches a card whose Delete target an earlier recorded batch already removed, and names the target as already deleted in the envelope's advisories.
func TestBeginBatchCmd_DeleteTargetAlreadyGone(t *testing.T) {
	t.Setenv("WEFT_SKIP_GIT", "1")
	fx := newVerbsFixture(t)
	const target = "internal/gone/x.txt"
	gitkit.CommitFile(t, fx.CLI.geom.WorktreeRoot, target, "x", "add the target")
	// The fixture's own single-card plan is replaced, so its card file goes too.
	if err := os.Remove(filepath.Join(fx.CLI.geom.PlanDir, "01-only.md")); err != nil {
		t.Fatalf("remove the fixture's card file: %v", err)
	}
	plankit.Write(t, fx.CLI.geom.PlanDir, plankit.Plan{
		Approved: true,
		Framing:  "Framing.",
		Cards: []plankit.Card{
			{Number: 1, Slug: "first", Summary: "removes the target", Groups: []plankit.Group{{Label: "Delete", Targets: []string{target}}}, Intent: "remove it.", ImpactSummary: "Removes the target."},
			{Number: 2, Slug: "second", Summary: "also lists the target", Groups: []plankit.Group{{Label: "Delete", Targets: []string{target}}}, Intent: "remove it too.", ImpactSummary: "Removes the target."},
		},
	})
	st := fx.initState(t, "master-model")
	if err := os.Remove(filepath.Join(fx.CLI.geom.WorktreeRoot, target)); err != nil {
		t.Fatalf("remove the target batch 1 deleted: %v", err)
	}
	st.Batches[1] = &websterengine.BatchState{Slug: "first", Kind: "fork", Terminal: true, Status: "done"}
	if err := websterengine.SaveState(fx.CLI.geom.WebsterDir, fx.CLI.geom.ScratchDir, st); err != nil {
		t.Fatalf("SaveState() error = %v", err)
	}

	var out strings.Builder
	exitCode := clihelp.Execute(fx.CLI.beginBatchCmd(), &out, []string{"2"})

	if exitCode != 0 {
		t.Fatalf("begin-batch 2 = %d; want 0 -- a Delete target batch 1 already removed must not refuse the dispatch, output: %s", exitCode, out.String())
	}
	got := out.String()
	for _, want := range []string{`"batch":"02-second"`, `"advisories"`, "delete-target-gone", target, "already deleted"} {
		if !strings.Contains(got, want) {
			t.Errorf("output missing %q; got %q", want, got)
		}
	}
}

// TestBeginBatchCmd_PausedEnvelope proves the pause refusal is an operational signal (exit 0,
// {"paused": true}), never a hard error, and that state.json is left untouched.
func TestBeginBatchCmd_PausedEnvelope(t *testing.T) {
	t.Setenv("WEFT_SKIP_GIT", "1")
	fx := newVerbsFixture(t)
	fx.initState(t, "master-model")
	if err := websterengine.RequestPause(fx.CLI.geom.ScratchDir); err != nil {
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

	loaded, err := websterengine.LoadState(fx.CLI.geom.WebsterDir, fx.CLI.geom.ScratchDir)
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
	exitCode := clihelp.Execute(fx.CLI.pauseCmd(), &pauseOut, []string{})
	if exitCode != 0 {
		t.Fatalf("pause() = %d; want 0, output: %s", exitCode, pauseOut.String())
	}
	if !strings.Contains(pauseOut.String(), `"paused":true`) {
		t.Errorf("pause() output missing paused:true; got %q", pauseOut.String())
	}

	if !websterengine.PauseRequested(fx.CLI.geom.ScratchDir) {
		t.Error("PauseRequested(fx.CLI.geom.ScratchDir) = false after pause(); want true")
	}
	if websterengine.PauseRequested(fx.CLI.geom.WebsterDir) {
		t.Error("PauseRequested(fx.CLI.geom.WebsterDir) = true; want the pause flag only under the scratch dir, never the durable dir")
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
			// DoneChecks resolves a card's Create target against
			// deps.Geom.WorktreeRoot, which hubgeom.WebsterGeometry fills
			// with layout.AnchorPath() (fx.Worktree + anchorRel), not
			// fx.Worktree itself -- quarry.Resolve is a live filesystem
			// walk rooted there, so the file must physically exist under
			// that anchored directory, not merely under fx.Worktree.
			// commitFile therefore runs with the anchored path itself as
			// its git working directory (seedAnchoredGitLink's gitlink
			// makes that a valid, shared-history checkout) rather than
			// fx.Worktree plus a path prefix: git treats a nested ".git"
			// file as an embedded-repository boundary, so `git add`
			// invoked from fx.Worktree silently refuses to descend into
			// it and stages nothing.
			startSHA := gitkit.CommitFile(t, fx.CLI.geom.WorktreeRoot, "internal/only/new.go", "package only\n", "01.1: add impl")
			st.Batches[1] = &websterengine.BatchState{Slug: "only", StartSHA: startSHA, Kind: "fork"}
			st.CurrentBatch = 1
			if err := websterengine.SaveState(fx.CLI.geom.WebsterDir, fx.CLI.geom.ScratchDir, st); err != nil {
				t.Fatalf("SaveState() error = %v", err)
			}
			fx.Engine.Audit = shuttleengine.ForkAudit{
				Forks: []shuttleengine.ForkReport{{TranscriptPath: "subagents/fork1.jsonl", ReportReturned: true}},
			}
			if tt.writeReport {
				writeBatchReport(t, fx.CLI.geom.ReportsDir, startSHA)
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

			loaded, err := websterengine.LoadState(fx.CLI.geom.WebsterDir, fx.CLI.geom.ScratchDir)
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

// TestRecordBatchCmd_FailedBatchEnvelope proves a fork writing a Master contract file exits non-zero with batch_failed, names recover-batch, and leaves the batch terminal failed in state.json.
func TestRecordBatchCmd_FailedBatchEnvelope(t *testing.T) {
	t.Setenv("WEFT_SKIP_GIT", "1")
	fx := newVerbsFixture(t)
	st := fx.initState(t, "master-model")
	startSHA := gitkit.CommitFile(t, fx.CLI.geom.WorktreeRoot, "internal/only/new.go", "package only\n", "01.1: add impl")
	st.Batches[1] = &websterengine.BatchState{Slug: "only", StartSHA: startSHA, Kind: "fork"}
	st.CurrentBatch = 1
	if err := websterengine.SaveState(fx.CLI.geom.WebsterDir, fx.CLI.geom.ScratchDir, st); err != nil {
		t.Fatalf("SaveState() error = %v", err)
	}
	fx.Engine.Audit = shuttleengine.ForkAudit{
		Forks: []shuttleengine.ForkReport{{
			TranscriptPath: "subagents/fork1.jsonl",
			ReportReturned: true,
			WritePaths:     []string{websterengine.OutcomePath(fx.CLI.geom.WebsterDir)},
		}},
	}
	writeBatchReport(t, fx.CLI.geom.ReportsDir, startSHA)

	var out strings.Builder
	exitCode := clihelp.Execute(fx.CLI.recordBatchCmd(), &out, []string{"1"})

	if exitCode == 0 {
		t.Fatalf("record-batch 1 = 0; want non-zero, output: %s", out.String())
	}
	got := out.String()
	for _, want := range []string{`"batch_failed":true`, `"batch":"01-only"`, `lyx webster recover-batch`} {
		if !strings.Contains(got, want) {
			t.Errorf("output missing %q; got %q", want, got)
		}
	}

	loaded, err := websterengine.LoadState(fx.CLI.geom.WebsterDir, fx.CLI.geom.ScratchDir)
	if err != nil || loaded == nil {
		t.Fatalf("LoadState() after record-batch = %v, %v; want a state, nil", loaded, err)
	}
	bs := loaded.Batches[1]
	if !bs.Terminal || bs.Digest == nil || bs.Digest.Status != websterengine.DigestStatusFailed {
		t.Errorf("loaded.Batches[1] = %+v; want terminal with a failed digest", bs)
	}
}

// TestRecordBatchCmd_ReportArchivedEnvelope proves a report with no begin record is archived, the call exits non-zero with report_archived, and the report is gone from its live path.
func TestRecordBatchCmd_ReportArchivedEnvelope(t *testing.T) {
	t.Setenv("WEFT_SKIP_GIT", "1")
	fx := newVerbsFixture(t)
	fx.initState(t, "master-model")
	startSHA := gitkit.CommitFile(t, fx.CLI.geom.WorktreeRoot, "internal/only/new.go", "package only\n", "01.1: add impl")
	writeBatchReport(t, fx.CLI.geom.ReportsDir, startSHA)

	var out strings.Builder
	exitCode := clihelp.Execute(fx.CLI.recordBatchCmd(), &out, []string{"1"})

	if exitCode == 0 {
		t.Fatalf("record-batch 1 = 0; want non-zero, output: %s", out.String())
	}
	got := out.String()
	for _, want := range []string{`"report_archived":true`, `"batch":"01-only"`, `lyx webster begin-batch`} {
		if !strings.Contains(got, want) {
			t.Errorf("output missing %q; got %q", want, got)
		}
	}
	live := filepath.Join(fx.CLI.geom.ReportsDir, websterengine.ReportFileName(1, "only"))
	if _, err := os.Stat(live); !os.IsNotExist(err) {
		t.Errorf("stat %s = %v; want the report gone from the live path", live, err)
	}
}

// TestRecoverBatchCmd_NeedsFreshEnvelope proves recover-batch over a batch failed on an uncheckable finding exits non-zero with needs_fresh, names run --fresh, and spawns nothing.
func TestRecoverBatchCmd_NeedsFreshEnvelope(t *testing.T) {
	t.Setenv("WEFT_SKIP_GIT", "1")
	fx := newVerbsFixture(t)
	st := fx.initState(t, "master-model")
	st.Batches[1] = &websterengine.BatchState{
		Slug: "only", Kind: "fork", Terminal: true, Status: websterengine.DigestStatusFailed,
		Digest:      &websterengine.Digest{Batch: "01-only", Status: websterengine.DigestStatusFailed},
		Uncheckable: []string{"fabric-reference: cat FABRICREF/webster/state.json"},
	}
	if err := websterengine.SaveState(fx.CLI.geom.WebsterDir, fx.CLI.geom.ScratchDir, st); err != nil {
		t.Fatalf("SaveState() error = %v", err)
	}

	var out strings.Builder
	if code := clihelp.Execute(fx.CLI.recoverBatchCmd(), &out, []string{"1", "--wait", "1ns"}); code == 0 {
		t.Fatalf("recover-batch 1 = 0; want non-zero, output: %s", out.String())
	}
	got := out.String()
	for _, want := range []string{`"needs_fresh":true`, `lyx webster run --fresh`} {
		if !strings.Contains(got, want) {
			t.Errorf("output missing %q; got %q", want, got)
		}
	}
	loaded, err := websterengine.LoadState(fx.CLI.geom.WebsterDir, fx.CLI.geom.ScratchDir)
	if err != nil || loaded == nil {
		t.Fatalf("LoadState() = %v, %v; want a state, nil", loaded, err)
	}
	if bs := loaded.Batches[1]; !bs.Terminal || bs.Status != websterengine.DigestStatusFailed || bs.StrandGUID != "" {
		t.Errorf("loaded.Batches[1] = %+v; want the failed record unchanged", bs)
	}
}

// TestRecoverBatchCmd_RunningThenTerminal drives recover-batch across two calls against the same
// batch: the first call performs the spawn and returns a running snapshot (the strand has no report
// yet), proving the running envelope touches neither status nor digest fields;
// the second call ATTACHES to the already-spawned strand and, once the report has landed in
// between, classifies terminal, proving the digest envelope and that state.json/the report were
// both committed to the records side by then.
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
	if fx.Engine.PrepareCalls != 1 {
		t.Fatalf("Engine.prepareCalls after first call = %d; want exactly 1 (the spawn)", fx.Engine.PrepareCalls)
	}

	loaded, err := websterengine.LoadState(fx.CLI.geom.WebsterDir, fx.CLI.geom.ScratchDir)
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
	// "Finishes" means its card's own Create target actually lands and is committed: the terminal
	// recovery path now runs the same mechanical post-batch pass record-batch does, so a recovery
	// reporting done over a card whose target never appeared is refused rather than marked terminal.
	head := gitkit.CommitFile(t, fx.CLI.geom.WorktreeRoot, "internal/only/new.go", "package only\n", "01.1: land the card's Create target")
	writeBatchReport(t, fx.CLI.geom.ReportsDir, head)

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
	if fx.Engine.PrepareCalls != 1 {
		t.Errorf("Engine.prepareCalls after attach call = %d; want still exactly 1 (no re-spawn)", fx.Engine.PrepareCalls)
	}

	loaded, err = websterengine.LoadState(fx.CLI.geom.WebsterDir, fx.CLI.geom.ScratchDir)
	if err != nil || loaded == nil {
		t.Fatalf("LoadState() after terminal attach = %v, %v; want a state, nil", loaded, err)
	}
	if !loaded.Batches[1].Terminal {
		t.Error("loaded.Batches[1].Terminal = false; want true after a done digest")
	}
}

// TestRunCmd_ErrRunBusySkipsRecordsBackstop proves the ErrRunBusy refusal never reaches Master's own
// spawn and never runs the exit-time fabric backstop -- WEFT_SKIP_GIT is deliberately left UNSET here
// so that an accidental fabricSync call would fail loudly: with no records sibling on disk,
// fabricengine.Open's stat validation errors and run's envelope would carry "fabric sync failed" plus
// fabricengine's missing-path text, both asserted absent below. (The pre-cutover evidence --
// the retired records engine creating its lock dir on disk -- no longer exists: fabric creates nothing before
// validation, so output text is the reachable-fabricSync signal now.)
func TestRunCmd_ErrRunBusySkipsRecordsBackstop(t *testing.T) {
	fx := newVerbsFixture(t)
	starter := &verbsFakeMasterStarter{}
	fx.CLI.masterStarter = starter

	if err := os.MkdirAll(fx.CLI.geom.ScratchDir, 0o755); err != nil {
		t.Fatalf("mkdir webster scratch dir: %v", err)
	}
	held, err := lock.AcquireWriteLock(filepath.Join(fx.CLI.geom.ScratchDir, "run.lock"))
	if err != nil {
		t.Fatalf("acquire run.lock: %v", err)
	}
	defer held.Release()

	var out strings.Builder
	exitCode := clihelp.Execute(fx.CLI.runCmd(), &out, []string{})

	if exitCode != 1 {
		t.Fatalf("run while run.lock is held = %d; want 1, output: %s", exitCode, out.String())
	}
	if !strings.Contains(out.String(), "already in progress") {
		t.Errorf("output missing the run-busy message; got %q", out.String())
	}
	if starter.called {
		t.Error("MasterStarter.StartMaster was reached while run.lock was held; want zero calls")
	}
	// A reached fabricSync in this records-less geometry fails at
	// fabricengine.Open and stamps both strings below into the envelope --
	// their absence is the post-cutover proof the backstop never ran. (The
	// old proof, the retired records engine's on-disk lock-dir creation, no longer exists:
	// fabric creates nothing before its path validation.)
	if strings.Contains(out.String(), "fabric sync failed") {
		t.Errorf("output mentions a fabric sync failure; ErrRunBusy must skip the fabric backstop entirely: %q", out.String())
	}
	if strings.Contains(out.String(), "fabricengine:") {
		t.Errorf("output carries a fabricengine error; ErrRunBusy must return before any fabric call: %q", out.String())
	}
}

// seedPersistentPreRunFixture returns a fresh real hub, built at anchor ("." or "backend"), with
// shuttle/reed/webster/batcher config seeded (batcher.yaml's raw content is caller-supplied, so a
// test can override its active: key) -- unlike every other test in this file, this one drives
// Command()'s real PersistentPreRunE (never bypassing it with a hand-built *websterCLI literal),
// since load-time batcher selection is wired there (PersistentPreRunE, now via batcher.Active).
// Callers pass h.PrimeWorktree() (unanchored) or h.Location.AnchorPath() (anchored) to RunCLIIn
// explicitly rather than relying on a chdir'd process cwd.
func seedPersistentPreRunFixture(t *testing.T, anchor, batcherConfig string) *hubforge.Hub {
	t.Helper()
	h := hubforge.NewHub(t, anchor)
	seedPersistentPreRunConfig(t, h, batcherConfig)
	return h
}

// seedPersistentPreRunConfig writes the shuttle/reed/webster/batcher config into h, replacing any earlier batcher.yaml.
func seedPersistentPreRunConfig(t *testing.T, h *hubforge.Hub, batcherConfig string) {
	t.Helper()
	hubforge.SeedConfig(t, h, map[string]string{
		"shuttle": shuttleengine.ConfigTemplate(),
		"reed":    reedengine.ConfigTemplate(),
		"webster": websterengine.ConfigTemplate(),
		"batcher": batcherConfig,
	})
}

// TestPersistentPreRunE_BatcherSelection proves the load-time batcher selection (batcher.Active(baseDir), wired into PersistentPreRunE) through the `status` verb, which never itself touches the batcher, over one hub whose batcher.yaml each step rewrites:
// an unknown active: name is a true fail-fast gate that aborts before any verb's RunE ever runs, with an output.Err envelope naming the bad batcher key, and the default (empty) active: key resolves to the identity batchifier, so the command proceeds normally through the rest of PersistentPreRunE and into the verb's own RunE.
// The steps share one hub and each rewrites its own config, so none relies on another's result.
// The scenario calls t.Parallel as a whole and no step does, since the steps share the one hub.
func TestPersistentPreRunE_BatcherSelection(t *testing.T) {
	t.Parallel()

	h := hubforge.NewHub(t, ".")

	if !t.Run("unknown batcher fails fast", func(t *testing.T) {
		seedPersistentPreRunConfig(t, h, strings.Replace(batcher.ConfigTemplate(), `active: ""`, `active: "bogus"`, 1))

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
	}) {
		return
	}

	t.Run("default batcher resolves", func(t *testing.T) {
		seedPersistentPreRunConfig(t, h, batcher.ConfigTemplate())

		var out strings.Builder
		exitCode := RunCLIIn(h.PrimeWorktree(), &out, []string{"status"})

		if exitCode != 0 {
			t.Fatalf("status with the default batcher = %d; want 0, output: %s", exitCode, out.String())
		}
		if !strings.Contains(out.String(), `"initialized":false`) {
			t.Errorf("output missing initialized:false; got %q", out.String())
		}
	})
}

// TestPersistentPreRunE_PlanDirAnchoredAtSubpath is the one case that covers wiring.go's production
// plan-dir resolution -- geom := hubgeom.WebsterGeometry(loc) inside wireHub, whose geom.PlanDir feeds
// c.geom.PlanDir -- at a nested anchor. Neither newVerbsFixture's AnchorRel flip nor cmd/lyx's anchoring-table guard rows
// carry that proof: both build their expectations from layout.AnchorPath() themselves, so a
// production call site that regressed to layout.WorktreePath() would stay self-consistent and pass
// at either of those. This test drives the real PersistentPreRunE through RunCLIIn and asserts on
// planparser's own error text, which only a wrong-root c.planDir can produce.
// This file stays serial: no t.Parallel() is added here, matching every other test in this file.
func TestPersistentPreRunE_PlanDirAnchoredAtSubpath(t *testing.T) {
	h := seedPersistentPreRunFixture(t, "backend", batcher.ConfigTemplate())

	seedValidPlanDir(t, planparser.PlanDir(h.Location.AnchorPath()))

	// lyxcwd.Resolve gates cwd against the anchored directory exactly, so at a "backend" hub the
	// anchor directory -- not h.PrimeWorktree(), the unanchored worktree root -- is what RunCLIIn
	// must be given.
	var out strings.Builder
	exitCode := RunCLIIn(h.Location.AnchorPath(), &out, []string{"validate"})

	if exitCode != 0 {
		t.Fatalf("validate at a subpath-anchored hub = %d; want 0, output: %s", exitCode, out.String())
	}
	got := out.String()
	if !strings.Contains(got, `"valid":true`) {
		t.Errorf("output missing valid:true; got %q", got)
	}
	if strings.Contains(got, "plan overview not found") {
		t.Errorf("output contains \"plan overview not found\"; got %q -- a WorktreePath()-based resolution at cli.go's c.planDir assignment would look under the un-anchored worktree root and produce exactly that error", got)
	}
}

// TestFabricSyncWayForward_NextSyncCommitsSavedState takes the fabric-sync refusals' way forward against a real hub.
// Each refusal leaves its state saved under `_lyx` with nothing committed, and the next bracket verb's own sync is this same fabricSync call over the scoped `_lyx` pathspec,
// so a sync after the failed one commits the state that the failure left behind.
// The per-verb tests below reach each refusal through a failing opener, which needs no hub.
func TestFabricSyncWayForward_NextSyncCommitsSavedState(t *testing.T) {
	t.Setenv("WEFT_SKIP_GIT", "")
	h := hubforge.NewHub(t, ".")
	geom := hubgeom.WebsterGeometry(h.Location)
	st := &websterengine.State{PlanFingerprint: "fp", Batches: map[int]*websterengine.BatchState{1: {Slug: "only", Kind: "fork"}}}
	if err := websterengine.SaveState(geom.WebsterDir, geom.ScratchDir, st); err != nil {
		t.Fatalf("SaveState() error = %v", err)
	}

	if _, err := fabricSync(failingFabricOpen, h.Location.AnchorRel, "begin-batch 01-only"); err == nil {
		t.Fatal("fabricSync with a failing opener = nil error; want the refusal's cause")
	}

	open := func() (*fabricengine.Fabric, error) { return fabricengine.Open(h.Location) }
	committed, err := fabricSync(open, h.Location.AnchorRel, "record-batch 01-only done")
	if err != nil || !committed {
		t.Fatalf("fabricSync after the failure = %v, %v; want the saved state committed", committed, err)
	}
	names, err := gitexec.Run([]string{"log", "-1", "--name-only", "--format="}, filepath.Join(h.PrimeWorktree(), h.Location.AnchorRel, "_lyx"))
	if err != nil {
		t.Fatalf("git log in the _lyx repository: %v", err)
	}
	if !strings.Contains(names, "webster/state.json") {
		t.Errorf("the _lyx repository HEAD commits %q; want it to carry the saved webster/state.json", names)
	}
}

// TestBeginBatchCmd_FabricSyncFailureWayForward reaches begin-batch's fabric-sync refusal and checks the state was saved locally anyway.
// It then re-runs the verb with no fabric opener, which skips the sync, to show the verb proceeds past the saved state;
// TestFabricSyncWayForward_NextSyncCommitsSavedState is the proof that a working sync commits it.
func TestBeginBatchCmd_FabricSyncFailureWayForward(t *testing.T) {
	t.Setenv("WEFT_SKIP_GIT", "")
	fx := newVerbsFixture(t)
	fx.initState(t, "master-model")
	fx.CLI.openFabric = failingFabricOpen

	var out strings.Builder
	if code := clihelp.Execute(fx.CLI.beginBatchCmd(), &out, []string{"1"}); code == 0 {
		t.Fatalf("begin-batch 1 = 0; want non-zero, output: %s", out.String())
	}
	wantWayForward(t, out.String(), "lyx fabric commit")
	loaded, err := websterengine.LoadState(fx.CLI.geom.WebsterDir, fx.CLI.geom.ScratchDir)
	if err != nil || loaded == nil || loaded.Batches[1] == nil {
		t.Fatalf("LoadState() = %v, %v; want the saved batch record", loaded, err)
	}

	fx.CLI.openFabric = nil
	out.Reset()
	if code := clihelp.Execute(fx.CLI.beginBatchCmd(), &out, []string{"1"}); code != 0 {
		t.Fatalf("begin-batch 1 after the way forward = %d; want 0, output: %s", code, out.String())
	}
}

// TestRecordBatchCmd_FabricSyncFailureWayForward is the record-batch twin of the begin-batch test:
// the batch is terminal on disk despite the sync failure, which is what the way forward commits.
func TestRecordBatchCmd_FabricSyncFailureWayForward(t *testing.T) {
	t.Setenv("WEFT_SKIP_GIT", "")
	fx := newVerbsFixture(t)
	st := fx.initState(t, "master-model")
	startSHA := gitkit.CommitFile(t, fx.CLI.geom.WorktreeRoot, "internal/only/new.go", "package only\n", "01.1: add impl")
	st.Batches[1] = &websterengine.BatchState{Slug: "only", StartSHA: startSHA, Kind: "fork"}
	st.CurrentBatch = 1
	if err := websterengine.SaveState(fx.CLI.geom.WebsterDir, fx.CLI.geom.ScratchDir, st); err != nil {
		t.Fatalf("SaveState() error = %v", err)
	}
	fx.Engine.Audit = shuttleengine.ForkAudit{
		Forks: []shuttleengine.ForkReport{{TranscriptPath: "subagents/fork1.jsonl", ReportReturned: true}},
	}
	writeBatchReport(t, fx.CLI.geom.ReportsDir, startSHA)
	fx.CLI.openFabric = failingFabricOpen

	var out strings.Builder
	if code := clihelp.Execute(fx.CLI.recordBatchCmd(), &out, []string{"1"}); code == 0 {
		t.Fatalf("record-batch 1 = 0; want non-zero, output: %s", out.String())
	}
	wantWayForward(t, out.String(), "lyx fabric commit")
	loaded, err := websterengine.LoadState(fx.CLI.geom.WebsterDir, fx.CLI.geom.ScratchDir)
	if err != nil || loaded == nil || !loaded.Batches[1].Terminal {
		t.Fatalf("LoadState() = %v, %v; want batch 1 terminal on disk despite the sync failure", loaded, err)
	}
}

// TestRecoverBatchCmd_FabricSyncAndReedBootWayForward reaches recover-batch's reed-boot refusal and its spawn-time fabric-sync refusal, checking each names its way forward.
// The final re-run has no fabric opener, which skips the sync, so it shows the verb proceeds;
// TestFabricSyncWayForward_NextSyncCommitsSavedState is the proof that a working sync commits the saved state.
func TestRecoverBatchCmd_FabricSyncAndReedBootWayForward(t *testing.T) {
	t.Setenv("WEFT_SKIP_GIT", "")
	fx := newVerbsFixture(t)
	fx.initState(t, "master-model")

	fx.CLI.reedUp = func(context.Context, bool) error { return fmt.Errorf("tmux not ready (injected)") }
	var out strings.Builder
	if code := clihelp.Execute(fx.CLI.recoverBatchCmd(), &out, []string{"1", "--wait", "1ns"}); code == 0 {
		t.Fatalf("recover-batch 1 with a failing reed boot = 0; want non-zero, output: %s", out.String())
	}
	// The named command must be one the verb parses: a bare batch number, never NN-<slug>.
	wantWayForward(t, out.String(), "re-run `lyx webster recover-batch 01`")

	fx.CLI.reedUp = nil
	fx.CLI.openFabric = failingFabricOpen
	out.Reset()
	if code := clihelp.Execute(fx.CLI.recoverBatchCmd(), &out, []string{"1", "--wait", "1ns"}); code == 0 {
		t.Fatalf("recover-batch 1 with a failing sync = 0; want non-zero, output: %s", out.String())
	}
	wantWayForward(t, out.String(), "lyx fabric commit")

	fx.CLI.openFabric = nil
	out.Reset()
	if code := clihelp.Execute(fx.CLI.recoverBatchCmd(), &out, []string{"1", "--wait", "1ns"}); code != 0 {
		t.Fatalf("recover-batch 1 after the way forward = %d; want 0, output: %s", code, out.String())
	}
}

// TestBracketVerbs_NoRunInProgressWayForward reaches the "no run in progress" refusal on each bracket verb, then takes its way forward:
// once the run's state exists the same verb proceeds.
func TestBracketVerbs_NoRunInProgressWayForward(t *testing.T) {
	t.Setenv("WEFT_SKIP_GIT", "1")
	fx := newVerbsFixture(t)
	verbs := map[string]*cobra.Command{
		"begin-batch":   fx.CLI.beginBatchCmd(),
		"record-batch":  fx.CLI.recordBatchCmd(),
		"recover-batch": fx.CLI.recoverBatchCmd(),
	}
	for name, cmd := range verbs {
		var out strings.Builder
		args := []string{"1"}
		if name == "recover-batch" {
			args = append(args, "--wait", "1ns")
		}
		if code := clihelp.Execute(cmd, &out, args); code == 0 {
			t.Fatalf("%s before any run = 0; want non-zero, output: %s", name, out.String())
		}
		if !strings.Contains(out.String(), "no run in progress") || !strings.Contains(out.String(), "first") {
			t.Errorf("%s refusal missing its way forward; got %q", name, out.String())
		}
	}

	fx.initState(t, "master-model")
	var out strings.Builder
	if code := clihelp.Execute(fx.CLI.beginBatchCmd(), &out, []string{"1"}); code != 0 {
		t.Fatalf("begin-batch 1 once the run exists = %d; want 0, output: %s", code, out.String())
	}
}
