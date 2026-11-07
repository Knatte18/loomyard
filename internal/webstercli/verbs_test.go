// verbs_test.go covers webstercli's git-backed and spawn-backed verbs through the RunCLI seam.
// The verbs are begin-batch, record-batch, recover-batch, and run.
// A real scratch git repo backs WorktreeRoot.
// A real *shuttleengine.Runner wraps local fake shuttleengine.ReedOps and shuttleengine.Engine doubles:
// only a real Runner.Start mints a *shuttleengine.Run's StrandGUID, so these interfaces need more than a fake struct.
// The run verb's Master spawn is a local fake MasterStarter.
// Most tests build a *websterCLI literal directly, bypassing Command()'s PersistentPreRunE.
// They drive one verb's cobra.Command through clihelp.Execute, the package's injection point for these tests.
// seedPersistentPreRunFixture and its tests are the exception.
// They drive Command()'s real PersistentPreRunE through RunCLIIn.
// FABRIC_SKIP_GIT=1 is set on every test that reaches a fabricSync call, so no real records sibling worktree is needed.
// The one test proving fabricSync was never reached (ErrRunBusy) leaves FABRIC_SKIP_GIT unset.
// It asserts the envelope carries no fabric-sync or fabricengine error text:
// that is the failure a reached fabricSync would stamp in this records-less geometry.

package webstercli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/batcher"
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/plankit"
	"github.com/Knatte18/loomyard/internal/testkit/shuttlefake"
	"github.com/Knatte18/loomyard/internal/testkit/stencilkit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
	"github.com/Knatte18/loomyard/internal/websterengine"
	"github.com/spf13/cobra"
)

// seedHubStencils populates hub's fabricengine.StencilsDir(hub) with every shipped stencil.
// webster's prompts are read from disk at call time.
// A fixture hub that is never seeded therefore fails every verb that renders one.
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

// verbsFakeMasterStarter is a hermetic websterengine.MasterStarter double.
// It records whether it was called and errors loud if it was.
// Only tests proving a refusal path never reaches Master's own spawn use it.
type verbsFakeMasterStarter struct {
	called bool
}

func (s *verbsFakeMasterStarter) StartMaster(spec shuttleengine.Spec, gate shuttleengine.GateSpec) (websterengine.MasterHandle, error) {
	s.called = true
	return nil, fmt.Errorf("verbsFakeMasterStarter: StartMaster must not be reached in this test")
}

var _ websterengine.MasterStarter = (*verbsFakeMasterStarter)(nil)

// verbsFixture is a *websterCLI wired without Command()'s PersistentPreRunE.
// It runs over a real scratch git repo and a real *shuttleengine.Runner built on local fakes.
// Its single-batch plan is seeded under the fixture's own _lym/plan, and the plan's root: is the anchor directory.
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

	layout := &lymcwd.Location{HubPath: filepath.Dir(worktree), WorktreeName: filepath.Base(worktree), AnchorRel: "backend"}
	// The plan's root: names the anchor directory.
	// A card path resolved against the anchor instead of the worktree root doubles the prefix and reads as missing.
	// The Edit target exists only under the anchor directory of the worktree root,
	// so a dispatch-time lookup against the anchor reports it missing.
	gitkit.CommitFile(t, worktree, layout.AnchorRel+"/README.md", "# backend\n", "add the anchor's README")
	plan := onlyCreatePlan("", "internal/only/new.go")
	plan.Root = layout.AnchorRel
	plan.Cards[0].Groups = append(plan.Cards[0].Groups, plankit.Group{Label: "Edit", Targets: []string{"README.md"}})
	plan.Cards[0].ImpactSummary = "Edits the README."
	plankit.Write(t, planparser.PlanDir(layout.AnchorPath()), plan)
	seedHubStencils(t, layout.HubPath)
	seedHubGateConfig(t, layout.HubPath)
	// The anchor holds no .git of its own.
	// Every verb that reads git therefore opens the repository at the worktree root.

	reed := &shuttlefake.Reed{}
	engine := &shuttlefake.Engine{}
	engine.PrepareFn = func(string, shuttleengine.Spec, shuttleengine.Config) (shuttleengine.Launch, error) {
		return shuttleengine.Launch{Cmd: "fake-launch-cmd", SessionID: fmt.Sprintf("fake-session-%d", engine.PrepareCalls)}, nil
	}
	shuttleCfg := shuttleengine.Config{RunDir: filepath.Join(t.TempDir(), "runs"), RunTimeoutMin: 60, StartupTimeoutS: 30, Agents: shuttlefake.Agents(t)}
	runner := shuttleengine.NewRunner(reed, shuttlefake.EngineSet(engine), layout.AnchorPath(), layout.WorktreePath(), shuttleCfg)

	roles := map[websterengine.Role]shuttleengine.ModelChoice{
		websterengine.RoleMaster:   {Model: "master-model"},
		websterengine.RoleRecovery: {Model: "recovery-model"},
	}

	// The default (empty) batcher name resolves to the identity batchifier.
	// PersistentPreRunE would resolve the same batcher via Active and store it on c.batcher.
	// This fixture bypasses that step along with the rest of PersistentPreRunE.
	activeBatcher := batcher.Identity()

	c := &websterCLI{
		runner:     runner,
		starter:    runner,
		engines:    shuttlefake.EngineSet(engine),
		reed:       reed,
		anchorRel:  layout.AnchorRel,
		shuttleCfg: shuttleCfg,
		geom:       hubGeometryWithIndex(layout),
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

// TestPersistentPreRun_OpenFabricWiredButUninvoked proves that pre-run wiring builds the fabric-handle opener.
// The opener is a closure stored on c, and wiring never calls it.
// The fixture carries a hub-level board directory, so preflight.HubPresent selects hub mode.
// It has no records sibling worktree, and fabricengine.Open stat-checks that sibling,
// so an eager call would fail the "status" verb below with a non-nil error.
// Reaching exit 0 is therefore the behavioural half of the proof, and c.openFabric != nil is the structural half.
func TestPersistentPreRun_OpenFabricWiredButUninvoked(t *testing.T) {
	tiergate.Require(t, 1)
	container := t.TempDir()
	worktree := filepath.Join(container, "pair")
	if err := os.MkdirAll(worktree, 0o755); err != nil {
		t.Fatalf("mkdir worktree: %v", err)
	}
	gitkit.Git(t, worktree, "init")
	gitkit.Git(t, worktree, "config", "user.name", "Test User")
	gitkit.Git(t, worktree, "config", "user.email", "test@example.com")
	gitkit.CommitFile(t, worktree, "base.txt", "base", "base commit")
	// A hub-level board directory with no records sibling near it selects hub mode here.
	// It does so without wiring a real, paired fabric hub: preflight.HubPresent only stats the Bolt checkout's _lym.
	if err := os.MkdirAll(filepath.Join(fabricengine.BoardDir(container), "_lym"), 0o755); err != nil {
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

// initState writes a minimal state.json for fx's webster dir, fingerprint-matched to fx's own on-disk plan.
// It stands in for the state "lym webster run" creates before Master calls begin-batch, record-batch or recover-batch.
func (fx *verbsFixture) initState(t *testing.T) *websterengine.State {
	t.Helper()
	return seedRunState(t, fx.CLI)
}

// sizeProbeBatcher is a batcher.Batcher double.
// While batching, it asks its size source about one path and records the answer.
// Then it batches like the identity batchifier.
type sizeProbeBatcher struct {
	path  string
	found bool
}

func (b *sizeProbeBatcher) Batch(plan *planparser.Plan, cards []planparser.Card, sizes batcher.SizeSource, before int, base batcher.StartBase) ([]batcher.Batch, error) {
	_, found, err := sizes.Lines(b.path)
	if err != nil {
		return nil, err
	}
	b.found = found
	return batcher.Identity().Batch(plan, cards, sizes, before, base)
}

func (b *sizeProbeBatcher) Name() string { return "size-probe" }

// TestExecutionBatches_SizesReadFromWorktreeRoot proves the size source is rooted at the git worktree root.
// A repository-relative path under the anchor directory is found.
// A source rooted at the pane's cwd would not find it.
func TestExecutionBatches_SizesReadFromWorktreeRoot(t *testing.T) {
	tiergate.Require(t, 1)
	fx := newVerbsFixture(t)
	probe := &sizeProbeBatcher{path: "backend/README.md"}
	fx.CLI.batcher = probe
	plan, err := planparser.ParsePlan(fx.CLI.geom.PlanDir)
	if err != nil {
		t.Fatalf("ParsePlan() error = %v", err)
	}

	if _, err := fx.CLI.executionBatches(plan, nil); err != nil {
		t.Fatalf("executionBatches() error = %v", err)
	}

	if !probe.found {
		t.Errorf("size source found %q = false; want true, the fixture commits it under the worktree root", probe.path)
	}
}

// TestRebaselineCmd_SizesReadFromWorktreeRoot proves rebaseline's size source is rooted at the git worktree root.
// It applies when rebaseline re-forms the unbegun tail of a recorded partition.
func TestRebaselineCmd_SizesReadFromWorktreeRoot(t *testing.T) {
	tiergate.Require(t, 1)
	t.Setenv("FABRIC_SKIP_GIT", "1")
	fx := newVerbsFixture(t)
	plan := onlyCreatePlan("", "internal/only/new.go")
	plan.Root = fx.CLI.anchorRel
	plan.Cards = append(plan.Cards, plankit.Card{Number: 2, Slug: "second", Summary: "edits the README", Groups: []plankit.Group{{Label: "Edit", Targets: []string{"README.md"}}}, Intent: "edit it.", ImpactSummary: "Edits the README."})
	plankit.Write(t, fx.CLI.geom.PlanDir, plan)
	st := fx.initState(t)
	st.Partition = []websterengine.PartitionBatch{{Cards: []string{"01-only"}}}
	st.Batches[1] = &websterengine.BatchState{Slug: "only", Cards: []string{"01-only"}, StartSHA: "abc123", Kind: "fork"}
	if err := websterengine.SaveState(fx.CLI.geom.WebsterDir, fx.CLI.geom.ScratchDir, st); err != nil {
		t.Fatalf("SaveState() error = %v", err)
	}
	probe := &sizeProbeBatcher{path: "backend/README.md"}
	fx.CLI.batcher = probe

	var out strings.Builder
	if exitCode := clihelp.Execute(fx.CLI.rebaselineCmd(), &out, nil); exitCode != 0 {
		t.Fatalf("rebaseline = %d; want 0, output: %s", exitCode, out.String())
	}

	if !probe.found {
		t.Errorf("size source found %q = false; want true, the fixture commits it under the worktree root", probe.path)
	}
}

// TestBeginBatchCmd_HappyPath proves the success envelope carries prompt_path/start_sha and no model key.
// It also proves that begin-batch types nothing into Master's pane and persists state.json with the new BatchState.
func TestBeginBatchCmd_HappyPath(t *testing.T) {
	tiergate.Require(t, 1)
	t.Setenv("FABRIC_SKIP_GIT", "1")
	fx := newVerbsFixture(t)
	fx.initState(t)

	var out strings.Builder
	exitCode := clihelp.Execute(fx.CLI.beginBatchCmd(), &out, []string{"1"})

	if exitCode != 0 {
		t.Fatalf("begin-batch 1 = %d; want 0, output: %s", exitCode, out.String())
	}
	got := out.String()
	for _, want := range []string{`"batch":"01-only"`, `"prompt_path"`, `"start_sha"`} {
		if !strings.Contains(got, want) {
			t.Errorf("output missing %q; got %q", want, got)
		}
	}
	if strings.Contains(got, `"model"`) {
		t.Errorf("output = %q; want no model key", got)
	}
	if len(fx.Reed.SendTextCalls) != 0 || len(fx.Reed.SendKeyCalls) != 0 {
		t.Errorf("reed received %d text and %d key inputs; want none, begin-batch types nothing into a pane", len(fx.Reed.SendTextCalls), len(fx.Reed.SendKeyCalls))
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

// TestBeginBatchCmd_ReportOnDisk proves begin-batch archives a report with no begin-batch record.
// It names the archive as archived_report.
// A report over a recorded batch is refused, with the record's state and its one remedy named.
func TestBeginBatchCmd_ReportOnDisk(t *testing.T) {
	tiergate.Require(t, 1)
	tests := []struct {
		name         string
		record       *websterengine.BatchState
		wantExit     int
		wantText     []string
		wantArchived bool
		// wantExhausted expects the recovery_exhausted envelope flag, and its absence otherwise.
		wantExhausted bool
	}{
		{name: "no begin-batch record is archived", wantExit: 0, wantText: []string{`"archived_report"`, `"batch":"01-only"`}, wantArchived: true},
		{
			name:     "a begun non-terminal record is refused",
			record:   &websterengine.BatchState{Slug: "only", Kind: "fork"},
			wantExit: 1,
			wantText: []string{"begun and not terminal", "record-batch 1"},
		},
		{
			name:     "a terminal record is refused",
			record:   &websterengine.BatchState{Slug: "only", Kind: "fork", Terminal: true, Status: "done"},
			wantExit: 1,
			wantText: []string{"terminal with status done", "begin the next batch"},
		},
		{
			name:          "a stuck record at the cap carries recovery_exhausted",
			record:        &websterengine.BatchState{Slug: "only", Kind: "fork", Terminal: true, Status: "stuck", Recoveries: 2},
			wantExit:      1,
			wantText:      []string{"terminal with status stuck", "end the run stuck naming the batch"},
			wantExhausted: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("FABRIC_SKIP_GIT", "1")
			fx := newVerbsFixture(t)
			st := fx.initState(t)
			if tt.record != nil {
				st.Batches[1] = tt.record
				if err := websterengine.SaveState(fx.CLI.geom.WebsterDir, fx.CLI.geom.ScratchDir, st); err != nil {
					t.Fatalf("SaveState() error = %v", err)
				}
			}
			if err := os.MkdirAll(fx.CLI.geom.ReportsDir, 0o755); err != nil {
				t.Fatalf("mkdir reports dir: %v", err)
			}
			reportPath := filepath.Join(fx.CLI.geom.ReportsDir, websterengine.ReportFileName(1, "only"))
			if err := os.WriteFile(reportPath, []byte("status: OK\nhead_sha: deadbeef\n"), 0o644); err != nil {
				t.Fatalf("seed report: %v", err)
			}

			var out strings.Builder
			exitCode := clihelp.Execute(fx.CLI.beginBatchCmd(), &out, []string{"1"})

			if exitCode != tt.wantExit {
				t.Fatalf("begin-batch 1 = %d; want %d, output: %s", exitCode, tt.wantExit, out.String())
			}
			for _, want := range tt.wantText {
				if !strings.Contains(out.String(), want) {
					t.Errorf("output missing %q; got %q", want, out.String())
				}
			}
			if got := strings.Contains(out.String(), `"recovery_exhausted":true`); got != tt.wantExhausted {
				t.Errorf("recovery_exhausted flag present = %v; want %v, output: %s", got, tt.wantExhausted, out.String())
			}
			_, statErr := os.Stat(reportPath)
			if tt.wantArchived != os.IsNotExist(statErr) {
				t.Errorf("stat(report) = %v; want the report moved away = %v", statErr, tt.wantArchived)
			}
		})
	}
}

// TestBeginBatchCmd_DeleteTargetAlreadyGone proves begin-batch dispatches a card whose target is already deleted.
// An earlier recorded batch deleted the target, and the envelope's advisories name it as already deleted.
func TestBeginBatchCmd_DeleteTargetAlreadyGone(t *testing.T) {
	tiergate.Require(t, 1)
	t.Setenv("FABRIC_SKIP_GIT", "1")
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
	st := fx.initState(t)
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

// TestBeginBatchCmd_PlanConflict proves begin-batch's blocking refusal carries the plan_conflict class and its cause.
// It names the rebaseline way forward, never carries plan_drifted, and leaves a refusal note holding the class.
// The tree_moved cause is an engine decision that this command only copies into the envelope.
// The engine's own test covers it.
func TestBeginBatchCmd_PlanConflict(t *testing.T) {
	tiergate.Require(t, 1)
	// Every row starts with a stale conflict record and a routed file in the scratch directory.
	// A row with a blocked path occupies it with a non-empty directory instead.
	const (
		blockRecord = "record"
		blockRouted = "routed"
	)
	conflictPlan := func(t *testing.T, fx *verbsFixture) {
		gitkit.CommitFile(t, fx.CLI.geom.WorktreeRoot, "internal/only/new.go", "package only\n", "add the Create target")
		plan := onlyCreatePlan("", "internal/only/new.go")
		plan.Cards[0].Uses = []string{"sub.Foo"}
		plankit.Write(t, fx.CLI.geom.PlanDir, plan)
	}
	tests := []struct {
		name string
		args []string
		// shapePlan shapes the fixture's plan before the run state is seeded.
		shapePlan func(t *testing.T, fx *verbsFixture)
		// afterState changes the fixture after the run state is seeded.
		afterState func(t *testing.T, fx *verbsFixture)
		blocked    string
		wantExit   int
		// wantCause is the plan_conflict class the refusal carries, empty for a row that must carry none.
		wantCause string
		wantText  []string
		// wantRecord is the substrings the record holds afterwards; a row with none leaves no record.
		wantRecord []string
	}{
		{
			name:       "a Create target that already exists and a bare-symbol Uses, with no batch begun, is a plan_defect",
			args:       []string{"1"},
			shapePlan:  conflictPlan,
			wantExit:   1,
			wantCause:  "plan_defect",
			wantText:   []string{"create-already-exists", "bare-symbol-target", "lym webster rebaseline --card NN"},
			wantRecord: []string{"# Plan conflict at begin-batch 01", "cause: plan_defect", "- create-already-exists", "- bare-symbol-target"},
		},
		{
			name:      "a record path that cannot be removed or written still refuses with the class",
			args:      []string{"1"},
			shapePlan: conflictPlan,
			blocked:   blockRecord,
			wantExit:  1,
			wantCause: "plan_defect",
			wantText:  []string{"create-already-exists", "lym webster rebaseline --card NN"},
		},
		{
			name:     "an invalid batch number leaves no record",
			args:     []string{"x"},
			wantExit: 1,
			wantText: []string{"is not a valid batch number"},
		},
		{
			name: "a plan that fails to parse leaves no record",
			args: []string{"1"},
			afterState: func(t *testing.T, fx *verbsFixture) {
				if err := os.Remove(filepath.Join(fx.CLI.geom.PlanDir, "00-overview.md")); err != nil {
					t.Fatalf("remove the overview: %v", err)
				}
			},
			wantExit: 1,
		},
		{
			name: "a card edited after the state was seeded leaves no record",
			args: []string{"1"},
			afterState: func(t *testing.T, fx *verbsFixture) {
				plankit.Write(t, fx.CLI.geom.PlanDir, onlyCreatePlan("", "internal/edited/new.go"))
			},
			wantExit: 1,
			wantText: []string{"fingerprint"},
		},
		{
			name: "a worktree quarry cannot open leaves no record",
			args: []string{"1"},
			afterState: func(t *testing.T, fx *verbsFixture) {
				fx.CLI.geom.WorktreeRoot = filepath.Join(t.TempDir(), "missing")
			},
			wantExit: 1,
		},
		{
			name:     "a begun batch removes the stale record and the routed file",
			args:     []string{"1"},
			wantExit: 0,
		},
		{
			name:     "a routed file that cannot be removed does not fail a begun batch",
			args:     []string{"1"},
			blocked:  blockRouted,
			wantExit: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("FABRIC_SKIP_GIT", "1")
			fx := newVerbsFixture(t)
			fx.CLI.frictionDir = t.TempDir()
			if tt.shapePlan != nil {
				tt.shapePlan(t, fx)
			}
			fx.initState(t)
			scratch := fx.CLI.geom.ScratchDir
			if err := os.MkdirAll(scratch, 0o755); err != nil {
				t.Fatalf("mkdir scratch dir: %v", err)
			}
			stale := &websterengine.PlanConflictError{Batch: 9, Cause: websterengine.ConflictCauseTreeMoved, Findings: []string{"stale-finding"}}
			recordPath, routedPath := websterengine.ConflictRecordPath(scratch), websterengine.RoutedConflictPath(scratch)
			if tt.blocked == blockRecord {
				if err := os.MkdirAll(filepath.Join(recordPath, "occupant"), 0o755); err != nil {
					t.Fatalf("occupy the record path: %v", err)
				}
			} else if err := websterengine.WriteConflictRecord(scratch, stale); err != nil {
				t.Fatalf("seed the stale record: %v", err)
			}
			if tt.blocked == blockRouted {
				if err := os.MkdirAll(filepath.Join(routedPath, "occupant"), 0o755); err != nil {
					t.Fatalf("occupy the routed path: %v", err)
				}
			} else if err := os.WriteFile(routedPath, []byte("- stale-finding\n"), 0o644); err != nil {
				t.Fatalf("seed the routed file: %v", err)
			}
			if tt.afterState != nil {
				tt.afterState(t, fx)
			}

			var out strings.Builder
			exitCode := clihelp.Execute(fx.CLI.noteRefusals(fx.CLI.beginBatchCmd()), &out, tt.args)

			got := out.String()
			if exitCode != tt.wantExit {
				t.Fatalf("begin-batch %v = %d; want %d, output: %s", tt.args, exitCode, tt.wantExit, got)
			}
			for _, want := range tt.wantText {
				if !strings.Contains(got, want) {
					t.Errorf("output missing %q; got %q", want, got)
				}
			}
			if tt.wantCause != "" && !strings.Contains(got, `"plan_conflict":"`+tt.wantCause+`"`) {
				t.Errorf("output = %q; want plan_conflict class %q", got, tt.wantCause)
			}
			if tt.wantCause == "" && strings.Contains(got, "plan_conflict") {
				t.Errorf("output = %q; want no plan_conflict class", got)
			}
			if strings.Contains(got, "plan_drifted") {
				t.Errorf("output = %q; want no plan_drifted", got)
			}
			if tt.wantCause != "" {
				note, ok := refusalNotes(t, fx.CLI.frictionDir)["webster-refusal-begin-batch.md"]
				if !ok || !strings.Contains(note, "plan_conflict: "+tt.wantCause) {
					t.Errorf("refusal note = %q, %v; want webster-refusal-begin-batch.md holding plan_conflict: %s", note, ok, tt.wantCause)
				}
			}

			switch {
			case tt.blocked == blockRecord:
				if info, err := os.Stat(recordPath); err != nil || !info.IsDir() {
					t.Errorf("stat record path = %v, %v; want the occupying directory in place", info, err)
				}
			case tt.wantRecord != nil:
				record, err := os.ReadFile(recordPath)
				if err != nil {
					t.Fatalf("read the conflict record: %v", err)
				}
				for _, want := range tt.wantRecord {
					if !strings.Contains(string(record), want) {
						t.Errorf("record missing %q; got %q", want, record)
					}
				}
				if strings.Contains(string(record), "stale-finding") {
					t.Errorf("record = %q; want the stale record replaced", record)
				}
			default:
				if _, err := os.Stat(recordPath); !os.IsNotExist(err) {
					t.Errorf("stat record path = %v; want no record", err)
				}
			}
			if tt.blocked == blockRouted {
				if info, err := os.Stat(routedPath); err != nil || !info.IsDir() {
					t.Errorf("stat routed path = %v, %v; want the occupying directory in place", info, err)
				}
			} else if tt.wantExit == 0 {
				if _, err := os.Stat(routedPath); !os.IsNotExist(err) {
					t.Errorf("stat routed path = %v; want the routed file removed once a batch begins", err)
				}
			}
		})
	}
}

// TestFingerprintRefusals_CarryPlanDrifted proves that editing a card past its fingerprint makes two verbs refuse.
// The first is record-batch over a begun, non-terminal batch.
// The second is the terminal pass of recover-batch over a landed recovery.
// Each refusal carries the plan_drifted flag.
func TestFingerprintRefusals_CarryPlanDrifted(t *testing.T) {
	tiergate.Require(t, 1)
	tests := []struct {
		name string
		// seed brings the fixture to the state the verb refuses from, before the card file is edited.
		seed func(t *testing.T, fx *verbsFixture, st *websterengine.State)
		verb func(fx *verbsFixture) *cobra.Command
		args []string
	}{
		{
			name: "record-batch over a begun batch",
			seed: func(t *testing.T, fx *verbsFixture, st *websterengine.State) {
				startSHA := gitkit.CommitFile(t, fx.CLI.geom.WorktreeRoot, "backend/internal/only/new.go", "package only\n", "01.1: add impl")
				st.Batches[1] = &websterengine.BatchState{Slug: "only", StartSHA: startSHA, Kind: "fork"}
				st.CurrentBatch = 1
				if err := websterengine.SaveState(fx.CLI.geom.WebsterDir, fx.CLI.geom.ScratchDir, st); err != nil {
					t.Fatalf("SaveState() error = %v", err)
				}
				fx.Engine.Audit = shuttleengine.ForkAudit{
					Forks: []shuttleengine.ForkReport{{TranscriptPath: "subagents/fork1.jsonl", ReportReturned: true}},
				}
				writeBatchReport(t, fx.CLI.geom.ReportsDir, startSHA)
			},
			verb: func(fx *verbsFixture) *cobra.Command { return fx.CLI.recordBatchCmd() },
			args: []string{"1"},
		},
		{
			name: "recover-batch attaching to a recovery whose report landed",
			seed: func(t *testing.T, fx *verbsFixture, _ *websterengine.State) {
				var spawn strings.Builder
				if code := clihelp.Execute(fx.CLI.recoverBatchCmd(), &spawn, []string{"1", "--wait", "1ns"}); code != 0 {
					t.Fatalf("recover-batch 1 (spawn) = %d; want 0, output: %s", code, spawn.String())
				}
				head := gitkit.CommitFile(t, fx.CLI.geom.WorktreeRoot, "backend/internal/only/new.go", "package only\n", "01.1: land the card's Create target")
				writeBatchReport(t, fx.CLI.geom.ReportsDir, head)
			},
			verb: func(fx *verbsFixture) *cobra.Command { return fx.CLI.recoverBatchCmd() },
			args: []string{"1", "--wait", "1ns"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("FABRIC_SKIP_GIT", "1")
			fx := newVerbsFixture(t)
			tt.seed(t, fx, fx.initState(t))
			plankit.Write(t, fx.CLI.geom.PlanDir, onlyCreatePlan("", "internal/edited/new.go"))

			var out strings.Builder
			exitCode := clihelp.Execute(tt.verb(fx), &out, tt.args)

			if exitCode != 1 || !strings.Contains(out.String(), `"plan_drifted":true`) {
				t.Errorf("%s 1 = %d, output %q; want exit 1 carrying plan_drifted", tt.name, exitCode, out.String())
			}
		})
	}
}

// TestRecordBatchCmd_Envelope proves record-batch's two envelopes over an identical fixture.
// The fixture already holds one new fork transcript.
// The terminal success envelope carries the digest verbatim plus warnings.
// It is returned once a matching batch report has also landed.
// Until then, it returns the {"no_report": true} ladder signal, which is not an error.
func TestRecordBatchCmd_Envelope(t *testing.T) {
	tiergate.Require(t, 1)
	tests := []struct {
		name        string
		writeReport bool
		// bashCommand, when set, is the one command the fork's audit records.
		bashCommand   string
		wantSubstrs   []string
		wantTerminal  bool
		wantDigestSet bool
	}{
		{
			name:          "ReadOnlyFabricReferenceEnvelope",
			writeReport:   true,
			bashCommand:   "lym fabric list",
			wantSubstrs:   []string{`"batch":"01-only"`, `"status":"done"`, `audit warning (fabric-reference)`},
			wantTerminal:  true,
			wantDigestSet: true,
		},
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
			t.Setenv("FABRIC_SKIP_GIT", "1")
			fx := newVerbsFixture(t)
			// record-batch runs on a recorded state and prices no partition, so a lister that fails proves it is never called.
			fx.CLI.instructionFiles = failingInstructionFiles
			st := fx.initState(t)
			// DoneChecks resolves a card's Create target against the git worktree root.
			// The plan's root prefixes that target, so the file is committed there under the anchor directory.
			startSHA := gitkit.CommitFile(t, fx.CLI.geom.WorktreeRoot, "backend/internal/only/new.go", "package only\n", "01.1: add impl")
			st.Batches[1] = &websterengine.BatchState{Slug: "only", StartSHA: startSHA, Kind: "fork"}
			st.CurrentBatch = 1
			if err := websterengine.SaveState(fx.CLI.geom.WebsterDir, fx.CLI.geom.ScratchDir, st); err != nil {
				t.Fatalf("SaveState() error = %v", err)
			}
			fx.Engine.Audit = shuttleengine.ForkAudit{
				Forks: []shuttleengine.ForkReport{{TranscriptPath: "subagents/fork1.jsonl", ReportReturned: true}},
			}
			if tt.bashCommand != "" {
				fx.Engine.Audit.Forks[0].BashCommands = []string{tt.bashCommand}
			}
			if tt.writeReport {
				writeBatchReport(t, fx.CLI.geom.ReportsDir, startSHA)
			}
			// Else: never call writeBatchReport, because the no-report scenario proves the report-not-landed-yet ladder.

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
			// Only the digest-envelope row's report carries a real head_sha to cross-check.
			// The no-report row never reaches that code path.
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

// TestRecordBatchCmd_FailedBatchEnvelope proves that a fork writing a Master contract file fails the batch.
// The call exits non-zero with batch_failed and names recover-batch; state.json leaves the batch terminal and failed.
func TestRecordBatchCmd_FailedBatchEnvelope(t *testing.T) {
	tiergate.Require(t, 1)
	t.Setenv("FABRIC_SKIP_GIT", "1")
	fx := newVerbsFixture(t)
	st := fx.initState(t)
	startSHA := gitkit.CommitFile(t, fx.CLI.geom.WorktreeRoot, "backend/internal/only/new.go", "package only\n", "01.1: add impl")
	st.Batches[1] = &websterengine.BatchState{Slug: "only", StartSHA: startSHA, Kind: "fork", AmendedCards: []websterengine.AmendedCard{{Card: "01-only"}}}
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
	for _, want := range []string{`"batch_failed":true`, `"card_amended":true`, `"batch":"01-only"`, `lym webster recover-batch`} {
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

// TestRecordBatchCmd_DeleteReferencedByLaterCard proves a batch fails when a later card references its Delete target.
// record-batch fails with batch_failed, naming that card and the plan edit.
// recover-batch over the same batch then refuses before spawning, with the same flag and way forward.
func TestRecordBatchCmd_DeleteReferencedByLaterCard(t *testing.T) {
	tiergate.Require(t, 1)
	t.Setenv("FABRIC_SKIP_GIT", "1")
	fx := newVerbsFixture(t)
	root := fx.CLI.geom.WorktreeRoot
	// The fixture's own single-card plan is replaced, so its card file goes too.
	if err := os.Remove(filepath.Join(fx.CLI.geom.PlanDir, "01-only.md")); err != nil {
		t.Fatalf("remove the fixture's card file: %v", err)
	}
	plankit.Write(t, fx.CLI.geom.PlanDir, plankit.Plan{
		Approved: true,
		Language: "go",
		Framing:  "Framing.",
		Cards: []plankit.Card{
			{Number: 1, Slug: "first", Summary: "deletes Gone", Groups: []plankit.Group{{Label: "Delete", Targets: []string{"sub#Gone"}}}, Intent: "delete it.", ImpactSummary: "Removes sub#Gone."},
			{Number: 2, Slug: "second", Summary: "edits the user of Gone", Groups: []plankit.Group{{Label: "Edit", Targets: []string{"sub/user.go"}}}, Intent: "edit it.", ImpactSummary: "Edits the user."},
		},
	})
	st := fx.initState(t)
	startSHA := gitkit.CommitFile(t, root, "sub/a.go", "package sub\n\nfunc Gone() {}\n", "01.1: add the target")
	headSHA := gitkit.CommitFile(t, root, "sub/user.go", "package sub\n\nfunc user() {\n\tGone()\n}\n", "01.2: add the user")
	st.Batches[1] = &websterengine.BatchState{Slug: "first", StartSHA: startSHA, Kind: "fork"}
	st.CurrentBatch = 1
	if err := websterengine.SaveState(fx.CLI.geom.WebsterDir, fx.CLI.geom.ScratchDir, st); err != nil {
		t.Fatalf("SaveState() error = %v", err)
	}
	fx.Engine.Audit = shuttleengine.ForkAudit{Forks: []shuttleengine.ForkReport{{TranscriptPath: "subagents/fork1.jsonl", ReportReturned: true}}}
	if err := os.MkdirAll(fx.CLI.geom.ReportsDir, 0o755); err != nil {
		t.Fatalf("mkdir reports dir: %v", err)
	}
	report := &websterengine.Report{Status: websterengine.ReportStatusOK, HeadSHA: headSHA}
	if err := websterengine.WriteReport(filepath.Join(fx.CLI.geom.ReportsDir, websterengine.ReportFileName(1, "first")), report); err != nil {
		t.Fatalf("write batch report: %v", err)
	}
	wantInOutput := []string{`"batch_failed":true`, "2-second", "sub/user.go:4", "way forward: move the delete to a card after", "lym webster rebaseline --card NN"}

	var out strings.Builder
	if code := clihelp.Execute(fx.CLI.recordBatchCmd(), &out, []string{"1"}); code == 0 {
		t.Fatalf("record-batch 1 = 0; want non-zero, output: %s", out.String())
	}
	for _, want := range wantInOutput {
		if !strings.Contains(out.String(), want) {
			t.Errorf("record-batch output missing %q; got %q", want, out.String())
		}
	}
	if strings.Contains(out.String(), "card_amended") {
		t.Errorf("record-batch output carries card_amended without an amendment: %q", out.String())
	}

	out.Reset()
	if code := clihelp.Execute(fx.CLI.recoverBatchCmd(), &out, []string{"1", "--wait", "1ns"}); code == 0 {
		t.Fatalf("recover-batch 1 = 0; want non-zero, output: %s", out.String())
	}
	for _, want := range wantInOutput {
		if !strings.Contains(out.String(), want) {
			t.Errorf("recover-batch output missing %q; got %q", want, out.String())
		}
	}
	if fx.Engine.PrepareCalls != 0 {
		t.Errorf("Engine.PrepareCalls = %d; want no recovery strand started", fx.Engine.PrepareCalls)
	}
}

// TestRecordBatchCmd_LaterTargetRemovedEnvelope proves a batch that removes a later card's symbol is rejected.
// The call exits non-zero with later_target_removed, naming the symbol and card.
// The open batch is saved marked handed back.
func TestRecordBatchCmd_LaterTargetRemovedEnvelope(t *testing.T) {
	tiergate.Require(t, 1)
	t.Setenv("FABRIC_SKIP_GIT", "1")
	fx := newVerbsFixture(t)
	root := fx.CLI.geom.WorktreeRoot
	// The fixture's own single-card plan is replaced, so its card file goes too.
	if err := os.Remove(filepath.Join(fx.CLI.geom.PlanDir, "01-only.md")); err != nil {
		t.Fatalf("remove the fixture's card file: %v", err)
	}
	plankit.Write(t, fx.CLI.geom.PlanDir, plankit.Plan{
		Approved: true,
		Language: "go",
		Framing:  "Framing.",
		Cards: []plankit.Card{
			{Number: 1, Slug: "first", Summary: "edits a.go", Groups: []plankit.Group{{Label: "Edit", Targets: []string{"sub/a.go"}}}, Intent: "edit it.", ImpactSummary: "Edits a.go."},
			{Number: 2, Slug: "second", Summary: "edits Gone", Groups: []plankit.Group{{Label: "Edit", Targets: []string{"sub#Gone"}}}, Intent: "edit it.", ImpactSummary: "Edits Gone."},
		},
	})
	st := fx.initState(t)
	startSHA := gitkit.CommitFile(t, root, "sub/a.go", "package sub\n\nfunc Gone() {}\n", "01.1: add the target")
	headSHA := gitkit.CommitFile(t, root, "sub/a.go", "package sub\n", "01.2: remove the target")
	st.Batches[1] = &websterengine.BatchState{Slug: "first", StartSHA: startSHA, Kind: "fork"}
	st.CurrentBatch = 1
	if err := websterengine.SaveState(fx.CLI.geom.WebsterDir, fx.CLI.geom.ScratchDir, st); err != nil {
		t.Fatalf("SaveState() error = %v", err)
	}
	fx.Engine.Audit = shuttleengine.ForkAudit{Forks: []shuttleengine.ForkReport{{TranscriptPath: "subagents/fork1.jsonl", ReportReturned: true}}}
	if err := os.MkdirAll(fx.CLI.geom.ReportsDir, 0o755); err != nil {
		t.Fatalf("mkdir reports dir: %v", err)
	}
	report := &websterengine.Report{Status: websterengine.ReportStatusOK, HeadSHA: headSHA}
	if err := websterengine.WriteReport(filepath.Join(fx.CLI.geom.ReportsDir, websterengine.ReportFileName(1, "first")), report); err != nil {
		t.Fatalf("write batch report: %v", err)
	}

	var out strings.Builder
	if code := clihelp.Execute(fx.CLI.recordBatchCmd(), &out, []string{"1"}); code == 0 {
		t.Fatalf("record-batch 1 = 0; want non-zero, output: %s", out.String())
	}
	for _, want := range []string{`"later_target_removed":true`, `"batch":"01-first"`, "2-second", "sub#Gone", "way forward:"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("record-batch output missing %q; got %q", want, out.String())
		}
	}
	loaded, err := websterengine.LoadState(fx.CLI.geom.WebsterDir, fx.CLI.geom.ScratchDir)
	if err != nil {
		t.Fatalf("LoadState() error = %v", err)
	}
	if bs := loaded.Batches[1]; bs.Terminal || !bs.LaterTargetsReturned {
		t.Errorf("saved batch record: terminal %v, LaterTargetsReturned %v; want an open batch marked handed back", bs.Terminal, bs.LaterTargetsReturned)
	}
}

// TestRecordBatchCmd_ReportArchivedEnvelope proves a report with no begin record is archived.
// The call exits non-zero with report_archived, and the report is gone from its live path.
func TestRecordBatchCmd_ReportArchivedEnvelope(t *testing.T) {
	tiergate.Require(t, 1)
	t.Setenv("FABRIC_SKIP_GIT", "1")
	fx := newVerbsFixture(t)
	fx.initState(t)
	startSHA := gitkit.CommitFile(t, fx.CLI.geom.WorktreeRoot, "backend/internal/only/new.go", "package only\n", "01.1: add impl")
	writeBatchReport(t, fx.CLI.geom.ReportsDir, startSHA)

	var out strings.Builder
	exitCode := clihelp.Execute(fx.CLI.recordBatchCmd(), &out, []string{"1"})

	if exitCode == 0 {
		t.Fatalf("record-batch 1 = 0; want non-zero, output: %s", out.String())
	}
	got := out.String()
	for _, want := range []string{`"report_archived":true`, `"batch":"01-only"`, `lym webster begin-batch`} {
		if !strings.Contains(got, want) {
			t.Errorf("output missing %q; got %q", want, got)
		}
	}
	live := filepath.Join(fx.CLI.geom.ReportsDir, websterengine.ReportFileName(1, "only"))
	if _, err := os.Stat(live); !os.IsNotExist(err) {
		t.Errorf("stat %s = %v; want the report gone from the live path", live, err)
	}
}

// TestRecoverBatchCmd_NeedsFreshEnvelope proves recover-batch exits non-zero with needs_fresh.
// The batch failed on an uncheckable finding.
// The envelope names the reset-to-start route, and recover-batch spawns nothing.
func TestRecoverBatchCmd_NeedsFreshEnvelope(t *testing.T) {
	tiergate.Require(t, 1)
	t.Setenv("FABRIC_SKIP_GIT", "1")
	fx := newVerbsFixture(t)
	st := fx.initState(t)
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
	for _, want := range []string{`"needs_fresh":true`, `1) lym webster reset --to start; 2) lym webster run`} {
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

	// Read-only fabric references recover in line.
	// A batch that recorded no start commit refuses with the evidence text and its own flag.
	loaded.Batches[1].Uncheckable = []string{`fabric-reference: ran a fabric-referencing command ("lym fabric list") that can rewrite run state`}
	if err := websterengine.SaveState(fx.CLI.geom.WebsterDir, fx.CLI.geom.ScratchDir, loaded); err != nil {
		t.Fatalf("SaveState() error = %v", err)
	}
	out.Reset()
	if code := clihelp.Execute(fx.CLI.recoverBatchCmd(), &out, []string{"1", "--wait", "1ns"}); code == 0 {
		t.Fatalf("recover-batch 1 over a read-only fabric reference with no start commit = 0; want non-zero, output: %s", out.String())
	}
	for _, want := range []string{`"audit_not_acceptable":true`, "recorded no start commit"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output missing %q; got %q", want, out.String())
		}
	}
}

// TestRenameToolMissingEnvelope proves begin-batch and recover-batch exit non-zero with rename_tool_missing.
// The batch holds a Rename pair for a symbol, and no rename tool is listed.
// Both verbs name the way forward and spawn nothing.
func TestRenameToolMissingEnvelope(t *testing.T) {
	tiergate.Require(t, 1)
	t.Setenv("FABRIC_SKIP_GIT", "1")
	for _, verb := range []string{"begin-batch", "recover-batch"} {
		t.Run(verb, func(t *testing.T) {
			fx := newVerbsFixture(t)
			plan := onlyCreatePlan("", "internal/only/new.go")
			plan.Cards[0].Groups = []plankit.Group{{Label: "Rename", Targets: []string{"`sub#Foo` -> `plan:sub#Bar`"}}}
			plankit.Write(t, fx.CLI.geom.PlanDir, plan)
			st := fx.initState(t)
			st.Batches[1] = &websterengine.BatchState{
				Slug: "only", Kind: "fork", Terminal: true, Status: websterengine.DigestStatusFailed,
				Digest: &websterengine.Digest{Batch: "01-only", Status: websterengine.DigestStatusFailed},
			}
			if err := websterengine.SaveState(fx.CLI.geom.WebsterDir, fx.CLI.geom.ScratchDir, st); err != nil {
				t.Fatalf("SaveState() error = %v", err)
			}

			cmd, args := fx.CLI.beginBatchCmd(), []string{"1"}
			if verb == "recover-batch" {
				cmd, args = fx.CLI.recoverBatchCmd(), []string{"1", "--wait", "1ns"}
			}
			var out strings.Builder
			if code := clihelp.Execute(cmd, &out, args); code == 0 {
				t.Fatalf("%s 1 = 0; want non-zero, output: %s", verb, out.String())
			}
			for _, want := range []string{`"rename_tool_missing":true`, "lym webster " + verb + " 1"} {
				if !strings.Contains(out.String(), want) {
					t.Errorf("output missing %q; got %q", want, out.String())
				}
			}
			if fx.Engine.PrepareCalls != 0 {
				t.Errorf("engine PrepareCalls = %d; want 0, the refusal comes before any spawn", fx.Engine.PrepareCalls)
			}
		})
	}
}

// TestRebaselineCmd_EditedCardOfFailedBatchThenRecover proves a one-card fix needs no reset.
// `rebaseline --card 1` over a failed batch's edited card exits 0 and keeps the batch's start SHA.
// `recover-batch 1` then spawns with the edited card's gate in its prompt.
func TestRebaselineCmd_EditedCardOfFailedBatchThenRecover(t *testing.T) {
	tiergate.Require(t, 1)
	t.Setenv("FABRIC_SKIP_GIT", "1")
	fx := newVerbsFixture(t)
	fx.initState(t)

	var out strings.Builder
	if code := clihelp.Execute(fx.CLI.beginBatchCmd(), &out, []string{"1"}); code != 0 {
		t.Fatalf("begin-batch 1 = %d; want 0, output: %s", code, out.String())
	}
	st, err := websterengine.LoadState(fx.CLI.geom.WebsterDir, fx.CLI.geom.ScratchDir)
	if err != nil || st == nil {
		t.Fatalf("LoadState() = %v, %v; want a state, nil", st, err)
	}
	failed := st.Batches[1]
	failed.Terminal = true
	failed.Status = websterengine.DigestStatusFailed
	failed.Digest = &websterengine.Digest{Batch: "01-only", Status: websterengine.DigestStatusFailed, Reasons: []string{"declared work missing"}}
	if err := websterengine.SaveState(fx.CLI.geom.WebsterDir, fx.CLI.geom.ScratchDir, st); err != nil {
		t.Fatalf("SaveState() error = %v", err)
	}
	startSHA := failed.StartSHA

	// The edit keeps the plan's root, so only the card changes and the overview stays as recorded.
	edited := onlyCreatePlan("", "internal/edited/new.go")
	edited.Root = fx.CLI.anchorRel
	plankit.Write(t, fx.CLI.geom.PlanDir, edited)

	out.Reset()
	if code := clihelp.Execute(fx.CLI.rebaselineCmd(), &out, []string{"--card", "1"}); code != 0 {
		t.Fatalf("rebaseline --card 1 = %d; want 0, output: %s", code, out.String())
	}
	loaded, err := websterengine.LoadState(fx.CLI.geom.WebsterDir, fx.CLI.geom.ScratchDir)
	if err != nil || loaded == nil {
		t.Fatalf("LoadState() after rebaseline = %v, %v; want a state, nil", loaded, err)
	}
	if bs := loaded.Batches[1]; bs.StartSHA != startSHA || !bs.Terminal || bs.Status != websterengine.DigestStatusFailed {
		t.Errorf("loaded.Batches[1] = %+v; want the failed record with start SHA %q kept", bs, startSHA)
	}

	var prompt string
	fx.Engine.PrepareFn = func(_ string, spec shuttleengine.Spec, _ shuttleengine.Config) (shuttleengine.Launch, error) {
		prompt = spec.Prompt
		return shuttleengine.Launch{Cmd: "fake-launch-cmd", SessionID: "fake-session-recovery"}, nil
	}
	out.Reset()
	if code := clihelp.Execute(fx.CLI.recoverBatchCmd(), &out, []string{"1", "--wait", "1ns"}); code != 0 {
		t.Fatalf("recover-batch 1 = %d; want 0, output: %s", code, out.String())
	}
	if want := "  - `lym gate test --gate card --card 1-only`\n"; !strings.Contains(prompt, want) {
		t.Errorf("recovery prompt lacks the edited card's gate call %q; got %q", want, prompt)
	}

	// A further edit of a card whose recovery strand is in flight is accepted and reported as amended.
	edited = onlyCreatePlan("", "internal/edited-again/new.go")
	edited.Root = fx.CLI.anchorRel
	plankit.Write(t, fx.CLI.geom.PlanDir, edited)
	out.Reset()
	if code := clihelp.Execute(fx.CLI.rebaselineCmd(), &out, []string{"--card", "1"}); code != 0 {
		t.Fatalf("rebaseline --card 1 over the in-flight recovery = %d; want 0, output: %s", code, out.String())
	}
	if want := `"cards_amended":["01-only"]`; !strings.Contains(out.String(), want) {
		t.Errorf("in-flight rebaseline output missing %s; got %q", want, out.String())
	}
}

// TestRecoverBatchCmd_RunningThenTerminal drives recover-batch across two calls against the same batch.
// The first call performs the spawn and returns a running snapshot, since the strand has no report yet.
// That proves the running envelope touches neither status nor digest fields.
// The second call attaches to the spawned strand and, once the report has landed, classifies terminal.
// That proves the digest envelope, and that state.json and the report were both committed by then.
// A last stretch drives the recovery count.
// A dead recovery that committed offers its retry; a third counted spawn is refused, and a new amendment spawns anyway.
func TestRecoverBatchCmd_RunningThenTerminal(t *testing.T) {
	tiergate.Require(t, 1)
	t.Setenv("FABRIC_SKIP_GIT", "1")
	fx := newVerbsFixture(t)
	// recover-batch runs on a recorded state and prices no partition, so a lister that fails proves it is never called.
	fx.CLI.instructionFiles = failingInstructionFiles
	fx.initState(t)

	// First call: no prior record exists for batch 1, so RecoverBatch spawns a fresh recovery strand.
	// The bounded, near-zero wait then elapses with no report on disk yet, so the result is Running.
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
	if bs.Recoveries != 1 || bs.RecoveryStartSHA == "" {
		t.Fatalf("loaded.Batches[1] Recoveries, RecoveryStartSHA = %d, %q; want 1 and the HEAD at the spawn", bs.Recoveries, bs.RecoveryStartSHA)
	}
	spawnStart := bs.RecoveryStartSHA

	// Between the two calls, the recovery implementer finishes.
	// Its report lands on disk, self-reporting the worktree's real HEAD.
	// The recovery path cross-checks head_sha against the worktree, as record-batch does.
	// Finishing means the card's own Create target actually lands and is committed.
	// The terminal recovery path runs the same mechanical post-batch pass as record-batch,
	// so a recovery reporting done over a card whose target never appeared is refused rather than marked terminal.
	head := gitkit.CommitFile(t, fx.CLI.geom.WorktreeRoot, "backend/internal/only/new.go", "package only\n", "01.1: land the card's Create target")
	writeBatchReport(t, fx.CLI.geom.ReportsDir, head)

	// A card of the batch is amended while that recovery runs, which forces its done report failed.
	// The envelope carries card_amended, and the call after it spawns again with the entry rendered.
	loaded.Batches[1].AmendedCards = []websterengine.AmendedCard{{Card: "01-only"}}
	if err := websterengine.SaveState(fx.CLI.geom.WebsterDir, fx.CLI.geom.ScratchDir, loaded); err != nil {
		t.Fatalf("SaveState() error = %v", err)
	}
	var outForced strings.Builder
	if code := clihelp.Execute(fx.CLI.recoverBatchCmd(), &outForced, []string{"1", "--wait", "1ns"}); code == 0 {
		t.Fatalf("recover-batch 1 over an amended card = 0; want non-zero, output: %s", outForced.String())
	}
	for _, want := range []string{`"batch_failed":true`, `"card_amended":true`, `"batch":"01-only"`} {
		if !strings.Contains(outForced.String(), want) {
			t.Errorf("forced-failure output missing %q; got %q", want, outForced.String())
		}
	}
	var outRespawn strings.Builder
	if code := clihelp.Execute(fx.CLI.recoverBatchCmd(), &outRespawn, []string{"1", "--wait", "1ns"}); code != 0 || !strings.Contains(outRespawn.String(), `"status":"running"`) {
		t.Fatalf("recover-batch 1 after the forced failure = %d, output: %s; want a running respawn", code, outRespawn.String())
	}
	writeBatchReport(t, fx.CLI.geom.ReportsDir, head)

	// The last call attaches: Kind is recovery, the strand is non-terminal, and StrandGUID is set.
	// recoverSpawn and archiveStaleReport never run again, so the report just written survives,
	// and the very first gather sees it as terminal.
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
	if strings.Contains(got2, "card_amended") {
		t.Errorf("done call output carries card_amended: %q", got2)
	}
	if fx.Engine.PrepareCalls != 2 {
		t.Errorf("Engine.prepareCalls after attach call = %d; want still exactly 2 (no re-spawn)", fx.Engine.PrepareCalls)
	}

	loaded, err = websterengine.LoadState(fx.CLI.geom.WebsterDir, fx.CLI.geom.ScratchDir)
	if err != nil || loaded == nil {
		t.Fatalf("LoadState() after terminal attach = %v, %v; want a state, nil", loaded, err)
	}
	if !loaded.Batches[1].Terminal {
		t.Error("loaded.Batches[1].Terminal = false; want true after a done digest")
	}

	// The recovery strand dies without a report, after committing past the HEAD it was spawned at:
	// the terminal envelope counts the spawn and offers the one retry.
	rec := loaded.Batches[1]
	rec.Terminal, rec.Status, rec.Digest, rec.AmendedCards = false, "", nil, nil
	rec.Recoveries, rec.RecoveryStartSHA = 1, spawnStart
	if err := os.Remove(filepath.Join(fx.CLI.geom.ReportsDir, websterengine.ReportFileName(1, "only"))); err != nil && !os.IsNotExist(err) {
		t.Fatalf("remove the recovery's report: %v", err)
	}
	if err := websterengine.SaveState(fx.CLI.geom.WebsterDir, fx.CLI.geom.ScratchDir, loaded); err != nil {
		t.Fatalf("SaveState() error = %v", err)
	}
	var outDead strings.Builder
	if code := clihelp.Execute(fx.CLI.recoverBatchCmd(), &outDead, []string{"1", "--wait", "1ns"}); code == 0 {
		t.Fatalf("recover-batch 1 over a dead recovery = 0; want non-zero, output: %s", outDead.String())
	}
	for _, want := range []string{`"ok":false`, `"status":"dead"`, `"recovery_retry":true`, `"recoveries":1`, "lym webster recover-batch 1"} {
		if !strings.Contains(outDead.String(), want) {
			t.Errorf("dead recovery output missing %q; got %q", want, outDead.String())
		}
	}

	// A third counted spawn is refused with its own flag; an amendment no spawn rendered still spawns at two recoveries.
	loaded, err = websterengine.LoadState(fx.CLI.geom.WebsterDir, fx.CLI.geom.ScratchDir)
	if err != nil || loaded == nil {
		t.Fatalf("LoadState() after the dead recovery = %v, %v; want a state, nil", loaded, err)
	}
	loaded.Batches[1].Recoveries = 2
	if err := websterengine.SaveState(fx.CLI.geom.WebsterDir, fx.CLI.geom.ScratchDir, loaded); err != nil {
		t.Fatalf("SaveState() error = %v", err)
	}
	var outExhausted strings.Builder
	if code := clihelp.Execute(fx.CLI.recoverBatchCmd(), &outExhausted, []string{"1", "--wait", "1ns"}); code == 0 {
		t.Fatalf("recover-batch 1 at two recoveries = 0; want non-zero, output: %s", outExhausted.String())
	}
	if want := `"recovery_exhausted":true`; !strings.Contains(outExhausted.String(), want) {
		t.Errorf("exhausted output missing %s; got %q", want, outExhausted.String())
	}
	if fx.Engine.PrepareCalls != 2 {
		t.Errorf("Engine.prepareCalls after the exhausted refusal = %d; want still 2", fx.Engine.PrepareCalls)
	}
	loaded.Batches[1].AmendedCards = []websterengine.AmendedCard{{Card: "01-only"}}
	if err := websterengine.SaveState(fx.CLI.geom.WebsterDir, fx.CLI.geom.ScratchDir, loaded); err != nil {
		t.Fatalf("SaveState() error = %v", err)
	}
	var outAmended strings.Builder
	if code := clihelp.Execute(fx.CLI.recoverBatchCmd(), &outAmended, []string{"1", "--wait", "1ns"}); code != 0 || !strings.Contains(outAmended.String(), `"status":"running"`) {
		t.Fatalf("recover-batch 1 over a new amendment at two recoveries = %d, output: %s; want a running respawn", code, outAmended.String())
	}
	if fx.Engine.PrepareCalls != 3 {
		t.Errorf("Engine.prepareCalls after the amendment respawn = %d; want 3", fx.Engine.PrepareCalls)
	}

	// The respawned recovery reports FAILED: a stuck digest exits non-zero.
	// With two recoveries spent, the way forward ends the run stuck.
	stuckReport := &websterengine.Report{Status: websterengine.ReportStatusFailed, HeadSHA: head}
	if err := websterengine.WriteReport(filepath.Join(fx.CLI.geom.ReportsDir, websterengine.ReportFileName(1, "only")), stuckReport); err != nil {
		t.Fatalf("write the stuck report: %v", err)
	}
	var outStuck strings.Builder
	if code := clihelp.Execute(fx.CLI.recoverBatchCmd(), &outStuck, []string{"1", "--wait", "1ns"}); code == 0 {
		t.Fatalf("recover-batch 1 over a stuck recovery = 0; want non-zero, output: %s", outStuck.String())
	}
	loaded, err = websterengine.LoadState(fx.CLI.geom.WebsterDir, fx.CLI.geom.ScratchDir)
	if err != nil || loaded == nil {
		t.Fatalf("LoadState() after the stuck recovery = %v, %v; want a state, nil", loaded, err)
	}
	sessions := loaded.Batches[1].RecoverySessions
	if len(sessions) == 0 {
		t.Fatalf("loaded.Batches[1].RecoverySessions is empty; want the spawned sessions")
	}
	stuckSession := sessions[len(sessions)-1]
	for _, want := range []string{`"ok":false`, `"status":"stuck"`, `"recovery_retry":false`, `"recoveries":2`, `"batch":"01-only"`, "end the run stuck, naming batch 01-only", `"recovery_session":"` + stuckSession + `"`, "(session " + stuckSession + ")"} {
		if !strings.Contains(outStuck.String(), want) {
			t.Errorf("stuck recovery output missing %q; got %q", want, outStuck.String())
		}
	}

	// A recovery that reports NEEDS_PARENT leaves the batch waiting, exits 0 and spends nothing;
	// the next call resumes it with a fresh strand, uncounted even at two recoveries.
	loaded.Batches[1].AmendedCards = []websterengine.AmendedCard{{Card: "01-only"}}
	if err := websterengine.SaveState(fx.CLI.geom.WebsterDir, fx.CLI.geom.ScratchDir, loaded); err != nil {
		t.Fatalf("SaveState() error = %v", err)
	}
	var outWaitSpawn strings.Builder
	if code := clihelp.Execute(fx.CLI.recoverBatchCmd(), &outWaitSpawn, []string{"1", "--wait", "1ns"}); code != 0 || !strings.Contains(outWaitSpawn.String(), `"status":"running"`) {
		t.Fatalf("recover-batch 1 over another new amendment = %d, output: %s; want a running respawn", code, outWaitSpawn.String())
	}
	reportPath := filepath.Join(fx.CLI.geom.ReportsDir, websterengine.ReportFileName(1, "only"))
	need := "raise the tier 2 timeout in the hub gate.yaml"
	if err := websterengine.WriteReport(reportPath, &websterengine.Report{Status: websterengine.ReportStatusNeedsParent, HeadSHA: head, Need: need}); err != nil {
		t.Fatalf("write the waiting report: %v", err)
	}
	var outWaiting strings.Builder
	if code := clihelp.Execute(fx.CLI.recoverBatchCmd(), &outWaiting, []string{"1", "--wait", "1ns"}); code != 0 {
		t.Fatalf("recover-batch 1 over a NEEDS_PARENT report = %d; want 0, output: %s", code, outWaiting.String())
	}
	for _, want := range []string{`"ok":true`, `"status":"waiting"`, `"need":"` + need + `"`, `"recoveries":2`} {
		if !strings.Contains(outWaiting.String(), want) {
			t.Errorf("waiting recovery output missing %q; got %q", want, outWaiting.String())
		}
	}
	loaded, err = websterengine.LoadState(fx.CLI.geom.WebsterDir, fx.CLI.geom.ScratchDir)
	if err != nil || loaded == nil {
		t.Fatalf("LoadState() after the waiting recovery = %v, %v; want a state, nil", loaded, err)
	}
	if bs := loaded.Batches[1]; bs.Terminal || bs.Status != websterengine.DigestStatusWaiting || bs.Recoveries != 2 {
		t.Errorf("waiting record Terminal, Status, Recoveries = %v, %q, %d; want false, %q, 2", bs.Terminal, bs.Status, bs.Recoveries, websterengine.DigestStatusWaiting)
	}
	if _, err := os.Stat(reportPath); !os.IsNotExist(err) {
		t.Errorf("stat of the waiting report = %v; want it archived", err)
	}
	var outResume strings.Builder
	if code := clihelp.Execute(fx.CLI.recoverBatchCmd(), &outResume, []string{"1", "--wait", "1ns"}); code != 0 || !strings.Contains(outResume.String(), `"status":"running"`) {
		t.Fatalf("recover-batch 1 over a waiting batch at two recoveries = %d, output: %s; want a running respawn", code, outResume.String())
	}
	loaded, err = websterengine.LoadState(fx.CLI.geom.WebsterDir, fx.CLI.geom.ScratchDir)
	if err != nil || loaded == nil {
		t.Fatalf("LoadState() after the resume = %v, %v; want a state, nil", loaded, err)
	}
	if got := loaded.Batches[1].Recoveries; got != 2 {
		t.Errorf("Recoveries after the resume spawn = %d; want 2, since a resume is not counted", got)
	}
}

// TestRunCmd_ErrRunBusySkipsRecordsBackstop proves the ErrRunBusy refusal never reaches Master's own spawn.
// It also proves the refusal never runs the exit-time fabric backstop.
// FABRIC_SKIP_GIT stays unset, so an accidental fabricSync call fails loudly.
// With no records sibling on disk, the stat validation in fabricengine.Open fails,
// and the run envelope would carry "fabric sync failed" plus fabricengine's missing-path text.
// The output text is the signal that fabricSync was reached, so both strings are asserted absent below.
func TestRunCmd_ErrRunBusySkipsRecordsBackstop(t *testing.T) {
	tiergate.Require(t, 1)
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
	// A reached fabricSync fails at fabricengine.Open in this records-less geometry.
	// It stamps both strings below into the envelope, so their absence proves the backstop never ran.
	if strings.Contains(out.String(), "fabric sync failed") {
		t.Errorf("output mentions a fabric sync failure; ErrRunBusy must skip the fabric backstop entirely: %q", out.String())
	}
	if strings.Contains(out.String(), "fabricengine:") {
		t.Errorf("output carries a fabricengine error; ErrRunBusy must return before any fabric call: %q", out.String())
	}
}

// verbsDiedMaster is a websterengine.MasterStarter double whose Master dies with one background shell outstanding.
type verbsDiedMaster struct {
	strandGUID string
	sessionID  string
}

func (m *verbsDiedMaster) StartMaster(shuttleengine.Spec, shuttleengine.GateSpec) (websterengine.MasterHandle, error) {
	return m, nil
}

func (m *verbsDiedMaster) StrandGUID() string { return m.strandGUID }

func (m *verbsDiedMaster) Wait() (shuttleengine.Result, error) {
	return shuttleengine.Result{Outcome: shuttleengine.OutcomeDied, SessionID: m.sessionID, EndedShells: []shuttleengine.EndedShell{{Label: "sleep 9999", ID: "sh-1", Signal: shuttleengine.SignalTranscript, Outstanding: 90 * time.Second}}}, nil
}

var (
	_ websterengine.MasterStarter = (*verbsDiedMaster)(nil)
	_ websterengine.MasterHandle  = (*verbsDiedMaster)(nil)
)

// TestRunCmd_DiedMasterNotesExpiredShellOutcome drives `run` through its cobra command.
// The Master dies with one background shell outstanding.
// The test asserts that the friction note on disk states the error outcome and that the next run reclaims the strand.
func TestRunCmd_DiedMasterNotesExpiredShellOutcome(t *testing.T) {
	tiergate.Require(t, 1)
	t.Setenv("FABRIC_SKIP_GIT", "1")
	fx := newVerbsFixture(t)
	fx.CLI.frictionDir = t.TempDir()
	fx.CLI.cfg.VerifyGateAttempts = 3
	master := &verbsDiedMaster{strandGUID: "master-strand-died", sessionID: "master-session-died"}
	fx.CLI.masterStarter = master

	runDir := filepath.Join(fx.CLI.shuttleCfg.RunDir, "fake-run-"+master.strandGUID)
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatalf("mkdir run dir: %v", err)
	}
	runState, err := json.Marshal(shuttleengine.RunState{RunID: "fake-run-" + master.strandGUID, StrandGUID: master.strandGUID, SessionID: master.sessionID, CreatedAt: time.Now().UTC().Format(time.RFC3339)})
	if err != nil {
		t.Fatalf("marshal run state: %v", err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "run.json"), runState, 0o644); err != nil {
		t.Fatalf("write run.json: %v", err)
	}

	var out strings.Builder
	exitCode := clihelp.Execute(fx.CLI.runCmd(), &out, []string{})

	if exitCode != 1 {
		t.Fatalf("run with a died Master = %d; want 1, output: %s", exitCode, out.String())
	}
	note, err := os.ReadFile(filepath.Join(fx.CLI.frictionDir, "webster-background-shell.md"))
	if err != nil {
		t.Fatalf("read friction note: %v", err)
	}
	for _, want := range []string{
		"`sleep 9999`",
		"outstanding for 1m30s",
		"the run never read the shell's result",
		"lym did not stop the shell",
		"the next `lym webster run` reclaims it at entry",
		"the run's final outcome: error (",
	} {
		if !strings.Contains(string(note), want) {
			t.Errorf("friction note = %q; want it to contain %q", note, want)
		}
	}
}

// seedPersistentPreRunFixture returns a fresh real hub, built at anchor ("." or "backend").
// It seeds shuttle, reed, webster and batcher config.
// The batcher.yaml content is caller-supplied, so a test can override its active: key.
// Tests that use it drive Command()'s real PersistentPreRunE, never a hand-built *websterCLI literal.
// Load-time batcher selection is wired in PersistentPreRunE.
// Callers pass h.PrimeWorktree() (unanchored) or h.Location.AnchorPath() (anchored) to RunCLIIn.
// The process cwd is never relied on.
func seedPersistentPreRunFixture(t *testing.T, anchor, batcherConfig string) *hubforge.Hub {
	t.Helper()
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: anchor})
	seedPersistentPreRunConfig(t, h, batcherConfig)
	// validate with no run state computes Merriam's start base, which reads the Master stencil.
	seedHubStencils(t, h.Path)
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

// TestPersistentPreRunE_BatcherSelection proves the load-time batcher selection.
// PersistentPreRunE wires it through batcher.Active(baseDir).
// It runs through the `status` verb, which never touches the batcher itself.
// The test uses one hub whose batcher.yaml each step rewrites.
// An active: key naming no profile is a fail-fast gate: it aborts before any verb's RunE runs,
// and the output.Err envelope names batcher.yaml and the bad key.
// A rebaseline under a profile with a negative coefficient is refused the same way,
// and it leaves state.json byte-identical.
// The template's default active: key resolves its wide profile,
// so the command proceeds through the rest of PersistentPreRunE into the verb's own RunE.
// A last step drives the config-load refusals of batcher.yaml and webster.yaml.
// Broken content, including a retired key whose way forward is the reconcile, carries `config_invalid: true`.
// An unreadable file and a broken shuttle.yaml carry none, and state.json stays byte-identical.
// The steps share one hub and each rewrites its own config, so none relies on another's result.
// The scenario calls t.Parallel as a whole and no step does, since the steps share the one hub.
func TestPersistentPreRunE_BatcherSelection(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})

	if !t.Run("unknown batcher fails fast", func(t *testing.T) {
		seedPersistentPreRunConfig(t, h, strings.Replace(batcher.ConfigTemplate(), `active: "wide"`, `active: "bogus"`, 1))

		var out strings.Builder
		exitCode := RunCLIIn(h.PrimeWorktree(), &out, []string{"status"})

		if exitCode != 1 {
			t.Fatalf("status with an unknown batcher = %d; want 1, output: %s", exitCode, out.String())
		}
		got := out.String()
		if !strings.Contains(got, `"ok":false`) {
			t.Errorf("output missing ok:false; got %q", got)
		}
		// The message is JSON-encoded, so its literal quotes appear as \".
		// Match the substrings separately, not the raw Go-quoted form.
		for _, want := range []string{"batcher.yaml did not load", "bogus", "way forward: fix batcher.yaml under _lym/config"} {
			if !strings.Contains(got, want) {
				t.Errorf("output missing %q; got %q", want, got)
			}
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

	t.Run("rebaseline with a bad profile leaves state untouched", func(t *testing.T) {
		seedPersistentPreRunConfig(t, h, strings.Replace(batcher.ConfigTemplate(), "fork_messages: 4", "fork_messages: -1", 1))
		geom := hubgeom.WebsterGeometry(h.Location)
		st := &websterengine.State{PlanFingerprint: "fp", Batches: map[int]*websterengine.BatchState{1: {Slug: "only", Kind: "fork"}}}
		if err := websterengine.SaveState(geom.WebsterDir, geom.ScratchDir, st); err != nil {
			t.Fatalf("SaveState() error = %v", err)
		}
		statePath := filepath.Join(geom.WebsterDir, "state.json")
		before, err := os.ReadFile(statePath)
		if err != nil {
			t.Fatalf("read state.json: %v", err)
		}

		var out strings.Builder
		exitCode := RunCLIIn(h.PrimeWorktree(), &out, []string{"rebaseline"})

		if exitCode != 1 {
			t.Fatalf("rebaseline with a negative coefficient = %d; want 1, output: %s", exitCode, out.String())
		}
		if !strings.Contains(out.String(), "batcher.yaml") || !strings.Contains(out.String(), "fork_messages") {
			t.Errorf("output does not name batcher.yaml and the coefficient; got %q", out.String())
		}
		after, err := os.ReadFile(statePath)
		if err != nil {
			t.Fatalf("read state.json after: %v", err)
		}
		if !bytes.Equal(before, after) {
			t.Errorf("state.json changed under a refused rebaseline:\nbefore %s\nafter  %s", before, after)
		}
	})

	t.Run("config load refusals carry config_invalid only for broken content", func(t *testing.T) {
		geom := hubgeom.WebsterGeometry(h.Location)
		st := &websterengine.State{PlanFingerprint: "fp", Batches: map[int]*websterengine.BatchState{1: {Slug: "only", Kind: "fork"}}}
		if err := websterengine.SaveState(geom.WebsterDir, geom.ScratchDir, st); err != nil {
			t.Fatalf("SaveState() error = %v", err)
		}
		statePath := filepath.Join(geom.WebsterDir, "state.json")

		badRole := "master: sonnet\nrecovery: \"opus \"\nself_fix_cap: 2\nmaster_timeout_min: 480\nrecovery_timeout_min: 60\nverify_gate_attempts: 3\n"
		tests := []struct {
			name string
			// seed rewrites the hub's config for the case; it may leave one file unreadable.
			seed            func(t *testing.T)
			wantConfig      bool
			wantContains    []string
			wantNotContains []string
		}{
			{
				name: "a malformed batcher profile",
				seed: func(t *testing.T) {
					seedPersistentPreRunConfig(t, h, strings.Replace(batcher.ConfigTemplate(), "fork_messages: 4", "fork_messages: -1", 1))
				},
				wantConfig:   true,
				wantContains: []string{"batcher.yaml did not load", "fork_messages", "way forward: fix batcher.yaml under _lym/config, then re-run the verb"},
			},
			{
				name: "a batcher profile carrying master_base",
				seed: func(t *testing.T) {
					seedPersistentPreRunConfig(t, h, strings.Replace(batcher.ConfigTemplate(), "orientation: 31400", "master_base: 52000", 1))
				},
				wantConfig:   true,
				wantContains: []string{"batcher.yaml did not load", "master_base", `way forward: run \"lym config reconcile --apply\", then re-run the verb`},
			},
			{
				name: "an unreadable batcher.yaml",
				seed: func(t *testing.T) {
					seedPersistentPreRunConfig(t, h, batcher.ConfigTemplate())
					makeConfigUnreadable(t, h, "batcher")
				},
				wantContains:    []string{"batcher.yaml could not be read", "way forward: transient, re-run the verb"},
				wantNotContains: []string{"did not load"},
			},
			{
				name: "a webster.yaml with a bad role spec",
				seed: func(t *testing.T) {
					seedPersistentPreRunConfig(t, h, batcher.ConfigTemplate())
					hubforge.SeedConfig(t, h, map[string]string{"webster": badRole})
				},
				wantConfig:   true,
				wantContains: []string{"webster.yaml did not load", "recovery", "way forward: fix webster.yaml under _lym/config, then re-run the verb"},
			},
			{
				name: "an unreadable webster.yaml",
				seed: func(t *testing.T) {
					seedPersistentPreRunConfig(t, h, batcher.ConfigTemplate())
					makeConfigUnreadable(t, h, "webster")
				},
				wantContains:    []string{"webster.yaml could not be read", "way forward: transient, re-run the verb"},
				wantNotContains: []string{"did not load"},
			},
			{
				name: "a malformed shuttle.yaml keeps its plain refusal",
				seed: func(t *testing.T) {
					seedPersistentPreRunConfig(t, h, batcher.ConfigTemplate())
					hubforge.SeedConfig(t, h, map[string]string{"shuttle": "not: [valid"})
				},
				wantNotContains: []string{"did not load", "could not be read"},
			},
		}
		for _, tt := range tests {
			tt.seed(t)
			before, err := os.ReadFile(statePath)
			if err != nil {
				t.Fatalf("%s: read state.json: %v", tt.name, err)
			}

			var out strings.Builder
			exitCode := RunCLIIn(h.PrimeWorktree(), &out, []string{"status"})

			got := out.String()
			if exitCode != 1 {
				t.Errorf("%s: status = %d; want 1, output: %s", tt.name, exitCode, got)
			}
			if hasFlag := strings.Contains(got, `"config_invalid":true`); hasFlag != tt.wantConfig {
				t.Errorf("%s: output has config_invalid:true = %v; want %v, output: %s", tt.name, hasFlag, tt.wantConfig, got)
			}
			for _, want := range tt.wantContains {
				if !strings.Contains(got, want) {
					t.Errorf("%s: output missing %q; got %q", tt.name, want, got)
				}
			}
			for _, unwanted := range tt.wantNotContains {
				if strings.Contains(got, unwanted) {
					t.Errorf("%s: output contains %q; got %q", tt.name, unwanted, got)
				}
			}
			after, err := os.ReadFile(statePath)
			if err != nil {
				t.Fatalf("%s: read state.json after: %v", tt.name, err)
			}
			if !bytes.Equal(before, after) {
				t.Errorf("%s: state.json changed under a refused verb:\nbefore %s\nafter  %s", tt.name, before, after)
			}
			unmakeConfigUnreadable(t, h, "batcher")
			unmakeConfigUnreadable(t, h, "webster")
		}
	})
}

// makeConfigUnreadable replaces h's config file for module with a directory of the same name.
// The file still exists, but reading it fails.
func makeConfigUnreadable(t *testing.T, h *hubforge.Hub, module string) {
	t.Helper()

	path := configengine.ConfigFile(h.Location.AnchorPath(), module)
	if err := os.Remove(path); err != nil {
		t.Fatalf("remove %s: %v", path, err)
	}
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
}

// unmakeConfigUnreadable removes the directory that makeConfigUnreadable left at h's config file for module, if any,
// so the next seed can write the file again.
func unmakeConfigUnreadable(t *testing.T, h *hubforge.Hub, module string) {
	t.Helper()

	path := configengine.ConfigFile(h.Location.AnchorPath(), module)
	if info, err := os.Stat(path); err == nil && info.IsDir() {
		if err := os.Remove(path); err != nil {
			t.Fatalf("remove %s: %v", path, err)
		}
	}
}

// TestPersistentPreRunE_PlanDirAnchoredAtSubpath covers the production plan-dir resolution at a nested anchor.
// The resolution is wireHub's geom.PlanDir, which feeds c.geom.PlanDir.
// Expectations built from layout.AnchorPath() cannot catch a call site that regressed to layout.WorktreePath().
// This test drives the real PersistentPreRunE through RunCLIIn.
// It asserts on planparser's own error text, which only a wrong-root c.planDir can produce.
// The plan's `root:` names the anchor's own directory and its Edit target is a file there,
// so a card path checked against the anchor instead of the worktree root doubles the prefix and reads as missing.
// The test does not call t.Parallel().
func TestPersistentPreRunE_PlanDirAnchoredAtSubpath(t *testing.T) {
	tiergate.Require(t, 1)
	h := seedPersistentPreRunFixture(t, "backend", batcher.ConfigTemplate())

	if err := os.WriteFile(filepath.Join(h.PrimeWorktree(), "backend", "README.md"), []byte("# backend\n"), 0o644); err != nil {
		t.Fatalf("write README.md under the anchor: %v", err)
	}
	plankit.Write(t, planparser.PlanDir(h.Location.AnchorPath()), plankit.Plan{
		Approved: true,
		Root:     "backend",
		Framing:  "Framing.",
		Cards: []plankit.Card{{
			Number:        1,
			Slug:          "only",
			Summary:       "edits the anchor's README",
			Groups:        []plankit.Group{{Label: "Edit", Targets: []string{"README.md"}}},
			Intent:        "placeholder card.",
			ImpactSummary: "none.",
		}},
	})

	// lymcwd.Resolve gates cwd against the anchored directory exactly.
	// So at a "backend" hub, RunCLIIn must get the anchor directory, not h.PrimeWorktree(), the unanchored root.
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
	for _, finding := range []string{"path-missing", "glyph-not-found"} {
		if strings.Contains(got, finding) {
			t.Errorf("output contains %q; got %q -- the root:-joined card path was looked up under the anchor instead of the worktree root", finding, got)
		}
	}
}

// TestFabricSyncWayForward_NextSyncCommitsSavedState takes the fabric-sync refusals' way forward against a real hub.
// Each refusal leaves its state saved under `_lym` with nothing committed.
// The next bracket verb's own sync is this same fabricSync call over the scoped `_lym` pathspec,
// so a sync after the failed one commits the state that the failure left behind.
// The per-verb tests below reach each refusal through a failing opener, which needs no hub.
func TestFabricSyncWayForward_NextSyncCommitsSavedState(t *testing.T) {
	tiergate.Require(t, 1)
	t.Setenv("FABRIC_SKIP_GIT", "")
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
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
	names, err := gitexec.Run([]string{"log", "-1", "--name-only", "--format="}, filepath.Join(h.PrimeWorktree(), h.Location.AnchorRel, "_lym"))
	if err != nil {
		t.Fatalf("git log in the _lym repository: %v", err)
	}
	if !strings.Contains(names, "webster/state.json") {
		t.Errorf("the _lym repository HEAD commits %q; want it to carry the saved webster/state.json", names)
	}
}

// TestBeginBatchCmd_FabricSyncFailureWayForward reaches begin-batch's fabric-sync refusal.
// It checks the state was saved locally anyway.
// It then re-runs the verb with no fabric opener, which skips the sync, to show the verb proceeds past the saved state.
// TestFabricSyncWayForward_NextSyncCommitsSavedState is the proof that a working sync commits it.
func TestBeginBatchCmd_FabricSyncFailureWayForward(t *testing.T) {
	tiergate.Require(t, 1)
	t.Setenv("FABRIC_SKIP_GIT", "")
	fx := newVerbsFixture(t)
	fx.initState(t)
	fx.CLI.openFabric = failingFabricOpen

	var out strings.Builder
	if code := clihelp.Execute(fx.CLI.beginBatchCmd(), &out, []string{"1"}); code == 0 {
		t.Fatalf("begin-batch 1 = 0; want non-zero, output: %s", out.String())
	}
	wantFabricSyncWayForward(t, out.String())
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

// TestRecordBatchCmd_FabricSyncFailureWayForward reaches record-batch's fabric-sync refusal.
// The batch is terminal on disk despite the sync failure, which is the state the way forward commits.
func TestRecordBatchCmd_FabricSyncFailureWayForward(t *testing.T) {
	tiergate.Require(t, 1)
	t.Setenv("FABRIC_SKIP_GIT", "")
	fx := newVerbsFixture(t)
	st := fx.initState(t)
	startSHA := gitkit.CommitFile(t, fx.CLI.geom.WorktreeRoot, "backend/internal/only/new.go", "package only\n", "01.1: add impl")
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
	wantFabricSyncWayForward(t, out.String())
	loaded, err := websterengine.LoadState(fx.CLI.geom.WebsterDir, fx.CLI.geom.ScratchDir)
	if err != nil || loaded == nil || !loaded.Batches[1].Terminal {
		t.Fatalf("LoadState() = %v, %v; want batch 1 terminal on disk despite the sync failure", loaded, err)
	}
}

// TestRecoverBatchCmd_FabricSyncAndReedBootWayForward reaches recover-batch's reed-boot refusal,
// and it reaches the spawn-time fabric-sync refusal; it checks that each names its way forward.
// The final re-run has no fabric opener, which skips the sync, so it shows the verb proceeds.
// TestFabricSyncWayForward_NextSyncCommitsSavedState is the proof that a working sync commits the saved state.
func TestRecoverBatchCmd_FabricSyncAndReedBootWayForward(t *testing.T) {
	tiergate.Require(t, 1)
	t.Setenv("FABRIC_SKIP_GIT", "")
	fx := newVerbsFixture(t)
	fx.initState(t)

	fx.CLI.reedUp = func() error { return fmt.Errorf("tmux not ready (injected)") }
	var out strings.Builder
	if code := clihelp.Execute(fx.CLI.recoverBatchCmd(), &out, []string{"1", "--wait", "1ns"}); code == 0 {
		t.Fatalf("recover-batch 1 with a failing reed boot = 0; want non-zero, output: %s", out.String())
	}
	// The named command must be one the verb parses: a bare batch number, never NN-<slug>.
	wantWayForward(t, out.String(), "re-run `lym webster recover-batch 01`")

	fx.CLI.reedUp = nil
	fx.CLI.openFabric = failingFabricOpen
	out.Reset()
	if code := clihelp.Execute(fx.CLI.recoverBatchCmd(), &out, []string{"1", "--wait", "1ns"}); code == 0 {
		t.Fatalf("recover-batch 1 with a failing sync = 0; want non-zero, output: %s", out.String())
	}
	wantFabricSyncWayForward(t, out.String())

	fx.CLI.openFabric = nil
	out.Reset()
	if code := clihelp.Execute(fx.CLI.recoverBatchCmd(), &out, []string{"1", "--wait", "1ns"}); code != 0 {
		t.Fatalf("recover-batch 1 after the way forward = %d; want 0, output: %s", code, out.String())
	}
}

// TestBracketVerbs_NoRunInProgressWayForward reaches the "no run in progress" refusal on each bracket verb.
// Once the run's state exists, the same verb proceeds.
func TestBracketVerbs_NoRunInProgressWayForward(t *testing.T) {
	tiergate.Require(t, 1)
	t.Setenv("FABRIC_SKIP_GIT", "1")
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

	fx.initState(t)
	var out strings.Builder
	if code := clihelp.Execute(fx.CLI.beginBatchCmd(), &out, []string{"1"}); code != 0 {
		t.Fatalf("begin-batch 1 once the run exists = %d; want 0, output: %s", code, out.String())
	}
}
