// shape_test.go carries the shape-and-identity assertions over New's built list: the literal
// seventeen-row producer table, the real Publish/Finalize swap, order stability, told-field
// threading, a missing-Landing-closure construction failure, and the routing-graph guard. It does
// not assert the recipe's own structure or parsing -- recipe_test.go owns that.

package loomrecipe

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/landingshed"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/preflightshed"
	"github.com/Knatte18/loomyard/internal/shedadapters"
	"github.com/Knatte18/loomyard/internal/shedcheck"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrecipe"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// wantProducerRow is one row of the literal producer-table this file's tests assert New's output
// against. producerType is the expected concrete Producer type, read via reflect.TypeOf -- consumed
// by TestNew_ProducerTable (which ignores it) and by recipe_test.go's shape assertion (which is the
// only reader of this column).
type wantProducerRow struct {
	name         string
	onStuck      string
	onDone       string
	segment      string
	maxBounces   int
	producerType reflect.Type
}

var wantProducerTable = []wantProducerRow{
	{loomshed.NamePreflight, "", loomshed.NameLoomPreflight, "", 0, reflect.TypeOf(preflightshed.NewPreflight("", ""))},
	{loomshed.NameLoomPreflight, "", loomshed.NameDiscussionWrite, "", 0, reflect.TypeOf(loomshed.NewLoomPreflight("", "", ""))},
	{loomshed.NameDiscussionWrite, "", loomshed.NameDiscussionValidate, "", 0, reflect.TypeOf(loomshed.NewDiscussionWrite("", nil, nil))},
	{loomshed.NameDiscussionValidate, loomshed.NameDiscussionWrite, loomshed.NameDiscussionBouncer, "", 0, reflect.TypeOf(loomshed.NewDiscussionValidate("", "", ""))},
	{loomshed.NameDiscussionBouncer, loomshed.NameDiscussionBurler, loomshed.NamePlanWrite, "Discussion-Review", 5, reflect.TypeOf(&shedadapters.Bouncer{})},
	{loomshed.NameDiscussionBurler, loomshed.NameDiscussionBouncer, loomshed.NameDiscussionBouncer, "Discussion-Review", 5, reflect.TypeOf(&shedadapters.BurlerProducer{})},
	{loomshed.NamePlanWrite, "", loomshed.NamePlanValidate, "", 0, reflect.TypeOf(loomshed.NewPlanWrite("", nil, nil))},
	{loomshed.NamePlanValidate, loomshed.NamePlanWrite, loomshed.NamePlanBouncer, "", 0, reflect.TypeOf(loomshed.NewPlanValidate("", "", ""))},
	{loomshed.NamePlanBouncer, loomshed.NamePlanBurler, loomshed.NamePlanRevalidate, "Plan-Review", 5, reflect.TypeOf(&shedadapters.Bouncer{})},
	{loomshed.NamePlanBurler, loomshed.NamePlanBouncer, loomshed.NamePlanBouncer, "Plan-Review", 5, reflect.TypeOf(&shedadapters.BurlerProducer{})},
	{loomshed.NamePlanRevalidate, loomshed.NamePlanWrite, loomshed.NameBatchifier, "", 0, reflect.TypeOf(loomshed.NewPlanValidate("", "", ""))},
	{loomshed.NameBatchifier, "", loomshed.NameWebster, "", 0, reflect.TypeOf(loomshed.NewBatchifier("", ""))},
	{loomshed.NameWebster, "", loomshed.NameWebsterBouncer, "", 0, reflect.TypeOf(loomshed.NewWebsterProducer("", "", nil, websterengine.RunDeps{}))},
	{loomshed.NameWebsterBouncer, loomshed.NameWebsterBurler, loomshed.NamePublish, "Webster-Review", 5, reflect.TypeOf(&shedadapters.Bouncer{})},
	{loomshed.NameWebsterBurler, loomshed.NameWebsterBouncer, loomshed.NameWebsterBouncer, "Webster-Review", 5, reflect.TypeOf(&shedadapters.BurlerProducer{})},
	{loomshed.NamePublish, "", loomshed.NameFinalize, "", 0, reflect.TypeOf(&landingshed.Publish{})},
	{loomshed.NameFinalize, "", "", "", 0, reflect.TypeOf(&landingshed.Finalize{})},
}

// testEnv builds a shedrecipe.Env/ShedPaths pair whose every path field is an absolute path derived
// from a single t.TempDir(), fills Landing via testLandingDeps, fills WebsterRun and the four
// WebsterDeps seams the way buildSequenceFixture does, and fills Shuttle, DiscussionSpec,
// CommitDiscussion, PlanSpec, and CommitPlan with the non-writing fakeLoomShuttle variant so the
// discussion and plan paths this builder points at stay absent on disk.
func testEnv(t *testing.T) (shedrecipe.Env, ShedPaths) {
	t.Helper()
	dir := t.TempDir()

	cwd := filepath.Join(dir, "cwd")
	if err := os.MkdirAll(cwd, 0o755); err != nil {
		t.Fatalf("mkdir cwd: %v", err)
	}

	statusPath := filepath.Join(dir, "status.json")
	statusLockPath := filepath.Join(dir, "status.json.lock")
	decisionRecordPath := filepath.Join(dir, "discussion", "decision-record.md")
	supportLogPath := filepath.Join(dir, "discussion", "support-log.md")
	planOverviewPath := filepath.Join(dir, "plan", "00-overview.md")

	runRoot := filepath.Join(dir, "reviews")
	if err := os.MkdirAll(runRoot, 0o755); err != nil {
		t.Fatalf("mkdir run root: %v", err)
	}
	stencilsDir := filepath.Join(dir, "stencils")
	seedBouncerStencils(t, stencilsDir)

	env := shedrecipe.Env{
		Cwd:                cwd,
		AnchorPath:         dir,
		WorktreeRoot:       dir,
		StatusPath:         statusPath,
		StatusLockPath:     statusLockPath,
		DecisionRecordPath: decisionRecordPath,
		SupportLogPath:     supportLogPath,
		WebsterRun:         (&fakeWebsterRun{}).run,
		WebsterDeps: websterengine.RunDeps{
			Starter:    fakeMasterStarter{},
			Reed:       fakeReedOps{},
			Engine:     fakeShuttleEngine{},
			RefMatcher: fakeRefMatcher{},
		},
		Landing:     testLandingDeps(dir),
		Shuttle:     &fakeLoomShuttle{writeOutputs: false},
		RunRoot:     runRoot,
		StencilsDir: stencilsDir,
		Burler:      &fakeLoomBurler{},
		Now:         func() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) },
		DiscussionSpec: func() (shuttleengine.Spec, error) {
			return shuttleengine.Spec{
				Prompt:      "discussion prompt",
				OutputFiles: []string{decisionRecordPath, supportLogPath},
				Interactive: false,
				Role:        "discussion",
			}, nil
		},
		CommitDiscussion: func() error { return nil },
		PlanSpec: func() (shuttleengine.Spec, error) {
			return shuttleengine.Spec{
				Prompt:      "plan prompt",
				OutputFiles: []string{planOverviewPath},
				Interactive: false,
				Role:        "plan",
			}, nil
		},
		CommitPlan: func() error { return nil },
	}

	paths := ShedPaths{
		StatusPath:     statusPath,
		LockPath:       filepath.Join(dir, "run.lock"),
		StatusLockPath: statusLockPath,
		MaxBounces:     3,
	}

	return env, paths
}

func TestNew_ProducerTable(t *testing.T) {
	env, paths := testEnv(t)
	shed, err := New(env, paths)
	if err != nil {
		t.Fatalf("New() error = %v; want nil", err)
	}

	if len(shed.Producers) != len(wantProducerTable) {
		t.Fatalf("New() produced %d rows; want %d", len(shed.Producers), len(wantProducerTable))
	}
	for i, want := range wantProducerTable {
		got := shed.Producers[i]
		if got.Name != want.name {
			t.Errorf("row %d name = %q; want %q", i, got.Name, want.name)
		}
		if got.OnStuck != want.onStuck {
			t.Errorf("row %d (%s) OnStuck = %q; want %q", i, got.Name, got.OnStuck, want.onStuck)
		}
		if got.OnDone != want.onDone {
			t.Errorf("row %d (%s) OnDone = %q; want %q", i, got.Name, got.OnDone, want.onDone)
		}
		if got.Segment != want.segment {
			t.Errorf("row %d (%s) Segment = %q; want %q", i, got.Name, got.Segment, want.segment)
		}
		if got.MaxBounces != want.maxBounces {
			t.Errorf("row %d (%s) MaxBounces = %d; want %d", i, got.Name, got.MaxBounces, want.maxBounces)
		}
		if got.Producer == nil {
			t.Errorf("row %d (%s) Producer = nil; want non-nil", i, got.Name)
		}
	}
}

func TestNew_ToldShedFields(t *testing.T) {
	env, paths := testEnv(t)
	shed, err := New(env, paths)
	if err != nil {
		t.Fatalf("New() error = %v; want nil", err)
	}

	if shed.StatusPath != paths.StatusPath {
		t.Errorf("shed.StatusPath = %q; want %q", shed.StatusPath, paths.StatusPath)
	}
	if shed.LockPath != paths.LockPath {
		t.Errorf("shed.LockPath = %q; want %q", shed.LockPath, paths.LockPath)
	}
	if shed.StatusLockPath != paths.StatusLockPath {
		t.Errorf("shed.StatusLockPath = %q; want %q", shed.StatusLockPath, paths.StatusLockPath)
	}
	if shed.MaxBounces != paths.MaxBounces {
		t.Errorf("shed.MaxBounces = %d; want %d", shed.MaxBounces, paths.MaxBounces)
	}
}

// TestNew_PublishAndFinalizeAreRealProducers asserts the row named Publish and the row named
// Finalize are each backed by the real landingshed producer type, not the stub type, and both keep
// their escalate-on-stuck setting.
func TestNew_PublishAndFinalizeAreRealProducers(t *testing.T) {
	env, paths := testEnv(t)
	shed, err := New(env, paths)
	if err != nil {
		t.Fatalf("New() error = %v; want nil", err)
	}

	var publishRow, finalizeRow *shedengine.ProducerDef
	for i := range shed.Producers {
		switch shed.Producers[i].Name {
		case loomshed.NamePublish:
			publishRow = &shed.Producers[i]
		case loomshed.NameFinalize:
			finalizeRow = &shed.Producers[i]
		}
	}
	if publishRow == nil {
		t.Fatalf("New() producer list has no row named %q", loomshed.NamePublish)
	}
	if finalizeRow == nil {
		t.Fatalf("New() producer list has no row named %q", loomshed.NameFinalize)
	}

	if _, ok := publishRow.Producer.(*landingshed.Publish); !ok {
		t.Errorf("row %q Producer = %T; want *landingshed.Publish, not the stub", loomshed.NamePublish, publishRow.Producer)
	}
	if _, ok := finalizeRow.Producer.(*landingshed.Finalize); !ok {
		t.Errorf("row %q Producer = %T; want *landingshed.Finalize, not the stub", loomshed.NameFinalize, finalizeRow.Producer)
	}
	if publishRow.OnStuck != "" {
		t.Errorf("row %q OnStuck = %q; want \"\" (escalate, never bounce)", loomshed.NamePublish, publishRow.OnStuck)
	}
	if finalizeRow.OnStuck != "" {
		t.Errorf("row %q OnStuck = %q; want \"\" (escalate, never bounce)", loomshed.NameFinalize, finalizeRow.OnStuck)
	}
}

// TestNew_ProducerTableOrderUnchangedByWiring re-asserts TestNew_ProducerTable's own table-order and
// name coverage, now that the list's order is the recipe's own list order rather than a Go literal's:
// the seventeen rows stay in their existing table order with their existing names, regardless of
// what backs rows 16 and 17.
func TestNew_ProducerTableOrderUnchangedByWiring(t *testing.T) {
	env, paths := testEnv(t)
	shed, err := New(env, paths)
	if err != nil {
		t.Fatalf("New() error = %v; want nil", err)
	}
	if len(shed.Producers) != len(wantProducerTable) {
		t.Fatalf("New() produced %d rows; want %d", len(shed.Producers), len(wantProducerTable))
	}
	for i, want := range wantProducerTable {
		if got := shed.Producers[i].Name; got != want.name {
			t.Errorf("row %d name = %q; want %q", i, got, want.name)
		}
	}
}

// TestNew_MissingLandingClosureReturnsError asserts that an Env whose landing passthrough is
// missing a required closure makes New return an error rather than a list that panics at call
// time: New surfaces landingshed.NewPublish's and landingshed.NewFinalize's own construction
// failure instead of discarding it.
func TestNew_MissingLandingClosureReturnsError(t *testing.T) {
	env, paths := testEnv(t)
	env.Landing.OpenFabric = nil

	shed, err := New(env, paths)
	if err == nil {
		t.Fatalf("New() error = nil; want non-nil error for an Env.Landing missing a required closure")
	}
	if shed != nil {
		t.Errorf("New() shed = %+v; want nil alongside a non-nil error", shed)
	}
	if !strings.Contains(err.Error(), "Publish") {
		t.Errorf("New() error = %q; want it to name the offending row %q", err.Error(), "Publish")
	}
}

func TestNew_PassesShedValidation(t *testing.T) {
	env, paths := testEnv(t)
	shed, err := New(env, paths)
	if err != nil {
		t.Fatalf("New() error = %v; want nil", err)
	}
	shed.Producers[0].Producer = fakeAlwaysDoneProducer{}

	if err := loomshed.Seed(paths.StatusPath, paths.StatusLockPath, "validation-slug", "validation-parent"); err != nil {
		t.Fatalf("Seed(): %v", err)
	}

	// Drive Run to exercise (*Shed).validate() indirectly, since it is unexported: a validation
	// error (a typo'd OnStuck, a duplicate name, two lock paths naming one file) surfaces as Run
	// returning a non-nil error before it ever reads the status file. Row 3's fake shuttle
	// deliberately writes nothing (env.Shuttle is a fakeLoomShuttle{writeOutputs: false}), so
	// each bounce re-runs a real producer that leaves the record absent, and Discussion-Validate
	// bounces back to Discussion-Write repeatedly; Discussion-Validate never returns Done, so its
	// own budget -- inherited from paths.MaxBounces (3), since neither producer sets a MaxBounces
	// of its own -- is spent entirely on this one producer's episode, and the run blocks once it is
	// exhausted. That is an ordinary Stuck/blocked outcome, not a validation failure, and is what
	// this test expects.
	result, err := shed.Run(context.Background())
	if err != nil {
		t.Fatalf("Run() error = %v; want nil (no shedengine.validate() failure)", err)
	}
	if result.Outcome != shedengine.RunBlocked {
		t.Errorf("Run() outcome = %q; want %q (Discussion-Validate's own bounce budget exhausted)", result.Outcome, shedengine.RunBlocked)
	}
}

// TestNew_RoutingGraphIsClean is the guard that fires when one of the upcoming "loom: real LLM
// producers" tasks mis-wires a Bouncer/Burler pair.
//
// It catches a Burler left with OnDone: "" (reported as unexpected-terminal), a Bouncer whose
// OnDone never exits its segment (reported as unreachable downstream), and a Bouncer whose OnStuck
// never routes back to it (reported as blind-gate).
//
// It does NOT catch a Burler handing back via OnDone instead of OnStuck: both wirings produce the
// identical routing graph, and the difference is a verdict returned inside Call, which
// shedcheck.Check never inspects. A comment claiming unqualified perch coverage would be false.
func TestNew_RoutingGraphIsClean(t *testing.T) {
	env, paths := testEnv(t)
	shed, err := New(env, paths)
	if err != nil {
		t.Fatalf("New() error = %v; want nil", err)
	}

	findings := shedcheck.Check(shed.Producers, loomshed.NamePreflight, []string{loomshed.NameFinalize})
	for _, f := range findings {
		t.Errorf("%s", f.String())
	}
}
