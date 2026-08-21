package loomshed

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/landingshed"
	"github.com/Knatte18/loomyard/internal/shedcheck"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// fakeAlwaysDoneProducer is a minimal shedengine.ShedProducer fake that always reports Done -- used
// to inject a fake Preflight without spawning git, per the Deps.Preflight Tier-1 carve-out.
type fakeAlwaysDoneProducer struct{}

func (fakeAlwaysDoneProducer) Call(context.Context) (shedengine.Outcome, shedengine.OutputPointer, error) {
	return shedengine.Done, shedengine.OutputPointer{}, nil
}

// wantProducerRow is one row of the literal producer-table this test asserts New's output against.
type wantProducerRow struct {
	name    string
	onStuck string
	onDone  string
}

var wantProducerTable = []wantProducerRow{
	{NamePreflight, "", NameLoomPreflight},
	{NameLoomPreflight, "", NameDiscussionWrite},
	{NameDiscussionWrite, "", NameDiscussionValidate},
	{NameDiscussionValidate, NameDiscussionWrite, NameDiscussionReview},
	{NameDiscussionReview, NameDiscussionWrite, NamePlanWrite},
	{NamePlanWrite, "", NamePlanValidate},
	{NamePlanValidate, NamePlanWrite, NamePlanReview},
	{NamePlanReview, NamePlanWrite, NameBatchifier},
	{NameBatchifier, "", NameWebster},
	{NameWebster, "", NameWebsterReview},
	{NameWebsterReview, NameWebster, NamePublish},
	{NamePublish, "", NameFinalize},
	{NameFinalize, "", ""},
}

func testDeps(t *testing.T) Deps {
	t.Helper()
	dir := t.TempDir()
	return Deps{
		StatusPath:         filepath.Join(dir, "status.json"),
		LockPath:           filepath.Join(dir, "run.lock"),
		StatusLockPath:     filepath.Join(dir, "status.json.lock"),
		MaxBounces:         3,
		AnchorPath:         dir,
		WorktreeRoot:       dir,
		DecisionRecordPath: filepath.Join(dir, "discussion", "decision-record.md"),
		SupportLogPath:     filepath.Join(dir, "discussion", "support-log.md"),
		Preflight:          fakeAlwaysDoneProducer{},
		WebsterRun:         (&fakeWebsterRun{}).run,
		Landing:            testLandingDeps(dir),
	}
}

func TestNew_ProducerTable(t *testing.T) {
	shed, err := New(testDeps(t))
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
		if got.Segment != "" {
			t.Errorf("row %d (%s) Segment = %q; want \"\" -- no row in this migration gains a non-empty Segment", i, got.Name, got.Segment)
		}
		if got.MaxBounces != 0 {
			t.Errorf("row %d (%s) MaxBounces = %d; want 0 -- no row in this migration gains a non-zero MaxBounces", i, got.Name, got.MaxBounces)
		}
		if got.Producer == nil {
			t.Errorf("row %d (%s) Producer = nil; want non-nil", i, got.Name)
		}
	}
}

func TestNew_ToldShedFields(t *testing.T) {
	deps := testDeps(t)
	shed, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v; want nil", err)
	}

	if shed.StatusPath != deps.StatusPath {
		t.Errorf("shed.StatusPath = %q; want %q", shed.StatusPath, deps.StatusPath)
	}
	if shed.LockPath != deps.LockPath {
		t.Errorf("shed.LockPath = %q; want %q", shed.LockPath, deps.LockPath)
	}
	if shed.StatusLockPath != deps.StatusLockPath {
		t.Errorf("shed.StatusLockPath = %q; want %q", shed.StatusLockPath, deps.StatusLockPath)
	}
	if shed.MaxBounces != deps.MaxBounces {
		t.Errorf("shed.MaxBounces = %d; want %d", shed.MaxBounces, deps.MaxBounces)
	}
}

// TestNew_PublishAndFinalizeAreRealProducers asserts the swap card 31 makes: the row named Publish
// and the row named Finalize are each backed by the real landingshed producer type, not the stub
// type, and both keep their escalate-on-stuck setting.
func TestNew_PublishAndFinalizeAreRealProducers(t *testing.T) {
	shed, err := New(testDeps(t))
	if err != nil {
		t.Fatalf("New() error = %v; want nil", err)
	}

	var publishRow, finalizeRow *shedengine.ProducerDef
	for i := range shed.Producers {
		switch shed.Producers[i].Name {
		case NamePublish:
			publishRow = &shed.Producers[i]
		case NameFinalize:
			finalizeRow = &shed.Producers[i]
		}
	}
	if publishRow == nil {
		t.Fatalf("New() producer list has no row named %q", NamePublish)
	}
	if finalizeRow == nil {
		t.Fatalf("New() producer list has no row named %q", NameFinalize)
	}

	if _, ok := publishRow.Producer.(*landingshed.Publish); !ok {
		t.Errorf("row %q Producer = %T; want *landingshed.Publish, not the stub", NamePublish, publishRow.Producer)
	}
	if _, ok := finalizeRow.Producer.(*landingshed.Finalize); !ok {
		t.Errorf("row %q Producer = %T; want *landingshed.Finalize, not the stub", NameFinalize, finalizeRow.Producer)
	}
	if publishRow.OnStuck != "" {
		t.Errorf("row %q OnStuck = %q; want \"\" (escalate, never bounce)", NamePublish, publishRow.OnStuck)
	}
	if finalizeRow.OnStuck != "" {
		t.Errorf("row %q OnStuck = %q; want \"\" (escalate, never bounce)", NameFinalize, finalizeRow.OnStuck)
	}
}

// TestNew_ProducerTableOrderUnchangedByWiring re-asserts TestNew_ProducerTable's own table-order and
// name coverage after the swap: the thirteen rows stay in their existing table order with their
// existing names, regardless of what backs rows 12 and 13.
func TestNew_ProducerTableOrderUnchangedByWiring(t *testing.T) {
	shed, err := New(testDeps(t))
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

// TestNew_MissingLandingClosureReturnsError asserts that a Deps whose landing passthrough is
// missing a required closure makes New return an error rather than a list that panics at call
// time: New surfaces landingshed.NewPublish's and landingshed.NewFinalize's own construction
// failure instead of discarding it.
func TestNew_MissingLandingClosureReturnsError(t *testing.T) {
	deps := testDeps(t)
	deps.Landing.OpenFabric = nil

	shed, err := New(deps)
	if err == nil {
		t.Fatalf("New() error = nil; want non-nil error for a Deps.Landing missing a required closure")
	}
	if shed != nil {
		t.Errorf("New() shed = %+v; want nil alongside a non-nil error", shed)
	}
}

func TestNew_NilPreflightReturnsError(t *testing.T) {
	deps := testDeps(t)
	deps.Preflight = nil

	shed, err := New(deps)
	if err == nil {
		t.Fatalf("New() error = nil; want non-nil error for a nil Deps.Preflight")
	}
	if shed != nil {
		t.Errorf("New() shed = %+v; want nil alongside a non-nil error", shed)
	}
}

func TestNew_PassesShedValidation(t *testing.T) {
	deps := testDeps(t)
	shed, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v; want nil", err)
	}

	if err := Seed(deps.StatusPath, deps.StatusLockPath, "validation-slug", "validation-parent"); err != nil {
		t.Fatalf("Seed(): %v", err)
	}

	// Drive Run to exercise (*Shed).validate() indirectly, since it is unexported: a validation
	// error (a typo'd OnStuck, a duplicate name, two lock paths naming one file) surfaces as Run
	// returning a non-nil error before it ever reads the status file. The discussion-record and
	// support-log paths in deps do not exist on disk, so Discussion-Validate bounces back to
	// Discussion-Write repeatedly; Discussion-Validate never returns Done, so its own budget --
	// inherited from deps.MaxBounces (3), since neither producer sets a MaxBounces of its own --
	// is spent entirely on this one producer's episode, and the run blocks once it is exhausted.
	// That is an ordinary Stuck/blocked outcome, not a validation failure, and is what this test
	// expects.
	result, err := shed.Run(context.Background())
	if err != nil {
		t.Fatalf("Run() error = %v; want nil (no shedengine.validate() failure)", err)
	}
	if result.Outcome != shedengine.RunBlocked {
		t.Errorf("Run() outcome = %q; want %q (Discussion-Validate's own bounce budget exhausted)", result.Outcome, shedengine.RunBlocked)
	}
}

// TestNew_RoutingGraphIsClean is the guard that fires when one of the five upcoming "loom: real LLM
// producers" tasks mis-wires a Bouncer/Burler pair.
//
// It catches a Burler left with OnDone: "" (reported as unexpected-terminal), a Bouncer whose
// OnDone never exits its segment (reported as unreachable downstream), and a Bouncer whose OnStuck
// never routes back to it (reported as blind-gate).
//
// It does NOT catch a Burler handing back via OnDone instead of OnStuck: both wirings produce the
// identical routing graph, and the difference is a verdict returned inside Call, which
// shedcheck.Check never inspects. A comment claiming unqualified perch coverage would be false.
//
// manifest/roadmap.md sequences "loom: convert to a Shed recipe" before the three perch-wiring
// tasks this guard exists for, and that item replaces this file's Go literal in loomshed.go -- the
// very thing this test reads -- with a recipe file. This guard must move onto the recipe-assembled
// list at that point rather than being deleted alongside the literal it happens to be written
// against.
func TestNew_RoutingGraphIsClean(t *testing.T) {
	shed, err := New(testDeps(t))
	if err != nil {
		t.Fatalf("New() error = %v; want nil", err)
	}

	findings := shedcheck.Check(shed.Producers, NamePreflight, []string{NameFinalize})
	for _, f := range findings {
		t.Errorf("%s", f.String())
	}
}
