// shape_test.go carries the shape-and-identity assertions over New's built list: the literal
// producer table, the real Publish/Finalize swap, order stability, told-field
// threading, a missing-Landing-closure construction failure, and the routing-graph guard. It does
// not assert the recipe's own structure or parsing -- recipe_test.go owns that.

package loomrecipe

import (
	"context"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/landingshed"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/preflightshed"
	"github.com/Knatte18/loomyard/internal/shedadapters"
	"github.com/Knatte18/loomyard/internal/shedbuild"
	"github.com/Knatte18/loomyard/internal/shedcheck"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrecipe"
	"github.com/Knatte18/loomyard/internal/testkit/envkit"
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

// frictionReflectProducerType returns the dynamic type of the producer loomshed.NewFrictionReflect
// builds, for the shape table's Friction-Reflect row.
func frictionReflectProducerType() reflect.Type {
	p, err := loomshed.NewFrictionReflect("", func() string { return "" })
	if err != nil {
		panic(err)
	}
	return reflect.TypeOf(p)
}

var wantProducerTable = []wantProducerRow{
	{loomshed.NamePreflight, "", loomshed.NameLoomPreflight, "", 0, reflect.TypeOf(preflightshed.NewPreflight("", ""))},
	{loomshed.NameLoomPreflight, "", loomshed.NameDiscussionWrite, "", 0, reflect.TypeOf(loomshed.NewLoomPreflight("", "", ""))},
	{loomshed.NameDiscussionWrite, "", loomshed.NameDiscussionBouncer, "", 0, reflect.TypeOf(loomshed.NewDiscussionWrite("", nil, nil))},
	{loomshed.NameDiscussionBouncer, loomshed.NameDiscussionBurler, loomshed.NamePlanWrite, "Discussion-Review", 5, reflect.TypeOf(&shedadapters.Bouncer{})},
	{loomshed.NameDiscussionBurler, loomshed.NameDiscussionBouncer, loomshed.NameDiscussionBouncer, "Discussion-Review", 5, reflect.TypeOf(&shedadapters.BurlerProducer{})},
	{loomshed.NamePlanWrite, "", loomshed.NamePlanBouncer, "", 0, reflect.TypeOf(loomshed.NewPlanWrite("", nil, nil))},
	{loomshed.NamePlanBouncer, loomshed.NamePlanBurler, loomshed.NameBatchifier, "Plan-Review", 5, reflect.TypeOf(&shedadapters.Bouncer{})},
	{loomshed.NamePlanBurler, loomshed.NamePlanBouncer, loomshed.NamePlanBouncer, "Plan-Review", 5, reflect.TypeOf(&shedadapters.BurlerProducer{})},
	{loomshed.NameBatchifier, "", loomshed.NameWebster, "", 0, reflect.TypeOf(loomshed.NewBatchifier("", ""))},
	{loomshed.NameWebster, "", loomshed.NameWebsterBouncer, "", 0, reflect.TypeOf(loomshed.NewWebsterProducer("", "", nil, websterengine.RunDeps{}, nil))},
	{loomshed.NameWebsterBouncer, loomshed.NameWebsterBurler, loomshed.NameDescribe, "Webster-Review", 5, reflect.TypeOf(&shedadapters.Bouncer{})},
	{loomshed.NameWebsterBurler, loomshed.NameWebsterBouncer, loomshed.NameWebsterBouncer, "Webster-Review", 5, reflect.TypeOf(&shedadapters.BurlerProducer{})},
	{loomshed.NameDescribe, "", loomshed.NamePublish, "", 0, reflect.TypeOf(loomshed.NewDiscussionWrite("", nil, nil))},
	{loomshed.NamePublish, "", loomshed.NamePRGate, "", 0, reflect.TypeOf(&landingshed.Publish{})},
	{loomshed.NamePRGate, loomshed.NamePRRework, loomshed.NameFinalize, "PR-Review", 5, reflect.TypeOf(&landingshed.PRGate{})},
	{loomshed.NamePRRework, "", loomshed.NamePlanBouncer, "PR-Review", 0, reflect.TypeOf(loomshed.NewPRRework("", func(loomshed.ReworkTold) shedengine.ShedProducer { return nil }, loomshed.PRReworkDeps{}))},
	{loomshed.NameFinalize, "", loomshed.NameFrictionReflect, "", 0, reflect.TypeOf(&landingshed.Finalize{})},
	{loomshed.NameFrictionReflect, "", "", "", 0, frictionReflectProducerType()},
}

// testEnv builds a shedrecipe.Env/shedbuild.ShedPaths pair from envkit.FullEnv, whose path fields are absolute paths under one t.TempDir().
// It seeds the bouncer stencils and swaps in the webster run fake, the non-writing loom shuttle and the loom burler, so the discussion and plan paths this builder points at stay absent on disk.
// ApprovePlan stays FullEnv's non-nil no-op: this file's subject is the constructed producer table and its routing graph, never a driven run, so nothing here reads the plan's approval flag.
func testEnv(t *testing.T) (shedrecipe.Env, shedbuild.ShedPaths) {
	t.Helper()

	env := envkit.FullEnv(t)
	dir := filepath.Dir(env.StatusPath)
	seedBouncerStencils(t, env.StencilsDir)
	env.ParentReview = testParentReviewConfig(dir, env.DecisionRecordPath, env.SupportLogPath)
	env.WebsterRun = (&fakeWebsterRun{}).run
	env.Shuttle = newLoomShuttle("", false)
	env.Burler = newLoomBurler(t)
	env.Now = func() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) }

	paths := shedbuild.ShedPaths{
		StatusPath:     env.StatusPath,
		LockPath:       filepath.Join(dir, "run.lock"),
		StatusLockPath: env.StatusLockPath,
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
// every row stays in its existing table order with its existing name, regardless of what backs the
// Publish and Finalize rows.
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
	// deliberately writes nothing (env.Shuttle is a non-writing newLoomShuttle), so
	// Discussion-Write's own gate -- which reads exactly the two files the fake did not write --
	// fails on its very first (and, per the "every test fake evaluates the gate once" decision,
	// only) evaluation. Discussion-Write carries no on_stuck, so that single failed gate blocks the
	// whole run immediately, with no bounce budget in play at all. That is an ordinary
	// Stuck/blocked outcome, not a validation failure, and is what this test expects.
	result, err := shed.Run(context.Background())
	if err != nil {
		t.Fatalf("Run() error = %v; want nil (no shedengine.validate() failure)", err)
	}
	if result.Outcome != shedengine.RunBlocked {
		t.Errorf("Run() outcome = %q; want %q (Discussion-Write's own gate failed with no on_stuck to bounce to)", result.Outcome, shedengine.RunBlocked)
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

	findings := shedcheck.Check(shed.Producers, loomshed.NamePreflight, []string{loomshed.NameFrictionReflect})
	for _, f := range findings {
		t.Errorf("%s", f.String())
	}
}
