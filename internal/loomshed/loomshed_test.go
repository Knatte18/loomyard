package loomshed

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/state"
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
}

var wantProducerTable = []wantProducerRow{
	{NamePreflight, ""},
	{NameDiscussionWrite, ""},
	{NameDiscussionValidate, NameDiscussionWrite},
	{NameDiscussionReview, NameDiscussionWrite},
	{NamePlanSweep, ""},
	{NamePlanWrite, ""},
	{NamePlanValidate, NamePlanWrite},
	{NamePlanReview, NamePlanWrite},
	{NameBatchifier, ""},
	{NameWebster, ""},
	{NameWebsterReview, NameWebster},
	{NameFinalize, ""},
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

	seed := shedengine.Status{
		CurrentProducer: NamePreflight,
		State:           shedengine.StateRunning,
		History:         []shedengine.HistoryEntry{},
	}
	if err := state.WriteJSON(deps.StatusPath, deps.StatusLockPath, seed); err != nil {
		t.Fatalf("seed status file: %v", err)
	}

	// Drive Run to exercise (*Shed).validate() indirectly, since it is unexported: a validation
	// error (a typo'd OnStuck, a duplicate name, two lock paths naming one file) surfaces as Run
	// returning a non-nil error before it ever reads the status file. The discussion-record and
	// support-log paths in deps do not exist on disk, so Discussion-Validate bounces back to
	// Discussion-Write until the bounce budget (3) is exhausted and the run blocks -- that is an
	// ordinary Stuck/blocked outcome, not a validation failure, and is what this test expects.
	result, err := shed.Run(context.Background())
	if err != nil {
		t.Fatalf("Run() error = %v; want nil (no shedengine.validate() failure)", err)
	}
	if result.Outcome != shedengine.RunBlocked {
		t.Errorf("Run() outcome = %q; want %q (bounce budget exhausted between Discussion-Write and Discussion-Validate)", result.Outcome, shedengine.RunBlocked)
	}
}
