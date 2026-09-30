package shedengine

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/lock"
)

func gotoRequest(shed *Shed, target string) GotoRequest {
	return GotoRequest{
		StatusPath:     shed.StatusPath,
		LockPath:       shed.LockPath,
		StatusLockPath: shed.StatusLockPath,
		Producers:      shed.Producers,
		Target:         target,
	}
}

func gotoShed(t *testing.T) (*Shed, *funcProducer, *funcProducer) {
	t.Helper()
	shed, _, _, _ := newTestShed(t)
	a := fixedOutcomeProducer(Done, "")
	b := fixedOutcomeProducer(Done, "")
	shed.Producers = []ProducerDef{{Name: "A", Producer: a}, {Name: "B", Producer: b}}
	return shed, a, b
}

func TestGoto_MovesBlockedRunToTarget(t *testing.T) {
	shed, _, _ := gotoShed(t)
	seed := commonSeed("A")
	seed.State = StateBlocked
	seed.Error = "stuck on A"
	seed.Transient = "network"
	seed.PauseRequested = true
	seedStatus(t, shed.StatusPath, shed.StatusLockPath, seed)

	returned, err := Goto(gotoRequest(shed, "B"))
	if err != nil {
		t.Fatalf("Goto(...) = _, %v; want nil error", err)
	}

	got := readStatus(t, shed.StatusPath, shed.StatusLockPath)
	if !reflect.DeepEqual(returned, got) {
		t.Errorf("returned Status = %+v; want the written file %+v", returned, got)
	}
	if got.CurrentProducer != "B" || got.State != StatePaused {
		t.Errorf("CurrentProducer, State = %q, %q; want B, paused", got.CurrentProducer, got.State)
	}
	if got.Error != "" || got.Transient != "" || got.PauseRequested {
		t.Errorf("Error, Transient, PauseRequested = %q, %q, %v; want all cleared", got.Error, got.Transient, got.PauseRequested)
	}
	if len(got.History) != 1 || got.History[0].Producer != "B" || got.History[0].Outcome != OutcomeGoto {
		t.Fatalf("History = %+v; want one goto entry for B", got.History)
	}
	assertRFC3339UTC(t, got.History[0].At)
	if got.Activity.Now != "B" || got.Activity.Last != "B → goto" {
		t.Errorf("Activity = %+v; want Now B, Last %q", got.Activity, "B → goto")
	}
}

func TestGoto_RunLockHeldRefusesAndLeavesFile(t *testing.T) {
	shed, _, _ := gotoShed(t)
	seed := commonSeed("A")
	seed.State = StateBlocked
	seedStatus(t, shed.StatusPath, shed.StatusLockPath, seed)
	before := readStatus(t, shed.StatusPath, shed.StatusLockPath)

	// The lock parent is normally made by Goto itself, so make it here before holding the lock.
	if _, err := Goto(gotoRequest(shed, "nope")); err == nil {
		t.Fatal("Goto with an unknown target succeeded; want error")
	}
	held, locked, err := lock.TryAcquireWriteLock(shed.LockPath)
	if err != nil || !locked {
		t.Fatalf("TryAcquireWriteLock = _, %v, %v; want acquired", locked, err)
	}
	defer held.Release()

	_, err = Goto(gotoRequest(shed, "B"))
	if !errors.Is(err, ErrShedBusy) {
		t.Fatalf("Goto(...) error = %v; want ErrShedBusy", err)
	}
	for _, want := range []string{"lyx shed pause", "lyx shed status"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not name %q", err, want)
		}
	}
	if after := readStatus(t, shed.StatusPath, shed.StatusLockPath); !reflect.DeepEqual(before, after) {
		t.Errorf("status file changed on a busy refusal: %+v -> %+v", before, after)
	}
}

func TestGoto_UnknownTargetListsValidNames(t *testing.T) {
	shed, _, _ := gotoShed(t)
	seed := commonSeed("A")
	seed.State = StateBlocked
	seedStatus(t, shed.StatusPath, shed.StatusLockPath, seed)
	before := readStatus(t, shed.StatusPath, shed.StatusLockPath)

	_, err := Goto(gotoRequest(shed, "Z"))
	if err == nil {
		t.Fatal("Goto(...) = nil error; want a refusal")
	}
	if !strings.Contains(err.Error(), "way forward: re-run goto with --to naming one of: A, B") {
		t.Errorf("error %q does not end in a way forward listing the valid names in list order", err)
	}
	if after := readStatus(t, shed.StatusPath, shed.StatusLockPath); !reflect.DeepEqual(before, after) {
		t.Errorf("status file changed on an unknown-target refusal")
	}

	// Taking the way forward: a name from the list moves the run.
	if _, err := Goto(gotoRequest(shed, "B")); err != nil {
		t.Errorf("Goto to a listed name = %v; want nil", err)
	}
}

// TestGoto_MissingStatusFileNamesSeeding pins goto's missing-status refusal to step's way forward:
// Shed never seeds a status file, so the run is seeded, after which goto moves it.
func TestGoto_MissingStatusFileNamesSeeding(t *testing.T) {
	shed, _, _ := gotoShed(t)

	_, err := Goto(gotoRequest(shed, "B"))
	if err == nil {
		t.Fatal("Goto(...) over a missing status file = nil error; want a refusal")
	}
	if !strings.Contains(err.Error(), missingStatusWayForward) {
		t.Errorf("error %q does not name seeding the run as its way forward", err)
	}

	seed := commonSeed("A")
	seed.State = StateBlocked
	seedStatus(t, shed.StatusPath, shed.StatusLockPath, seed)
	if _, err := Goto(gotoRequest(shed, "B")); err != nil {
		t.Errorf("Goto(...) once the run is seeded = %v; want nil", err)
	}
}

func TestGoto_DoneRunRefusedNamingNewSeed(t *testing.T) {
	shed, _, _ := gotoShed(t)
	seed := commonSeed("B")
	seed.State = StateDone
	seedStatus(t, shed.StatusPath, shed.StatusLockPath, seed)
	before := readStatus(t, shed.StatusPath, shed.StatusLockPath)

	_, err := Goto(gotoRequest(shed, "A"))
	if err == nil {
		t.Fatal("Goto(...) = nil error; want a refusal")
	}
	if !strings.Contains(err.Error(), "seed a new run") {
		t.Errorf("error %q does not name seeding a new run", err)
	}
	if after := readStatus(t, shed.StatusPath, shed.StatusLockPath); !reflect.DeepEqual(before, after) {
		t.Errorf("status file changed on a done refusal")
	}
}

func TestGoto_StepResumesAtTarget(t *testing.T) {
	shed, a, b := gotoShed(t)
	seed := commonSeed("A")
	seed.State = StateBlocked
	seed.Error = "stuck on A"
	seedStatus(t, shed.StatusPath, shed.StatusLockPath, seed)

	if _, err := Goto(gotoRequest(shed, "B")); err != nil {
		t.Fatalf("Goto(...) = _, %v; want nil error", err)
	}
	res, err := shed.Step(context.Background())
	if err != nil {
		t.Fatalf("Step(...) = _, %v; want nil error", err)
	}
	if a.calls != 0 || b.calls != 1 {
		t.Errorf("calls A, B = %d, %d; want 0, 1 -- the step must resume at the goto target", a.calls, b.calls)
	}
	if res.State != StateDone {
		t.Errorf("Step State = %q; want done", res.State)
	}
}

func TestStep_UnknownCurrentProducerNamesGoto(t *testing.T) {
	shed, a, b := gotoShed(t)
	seed := commonSeed("Renamed")
	seed.State = StateBlocked
	seedStatus(t, shed.StatusPath, shed.StatusLockPath, seed)
	before := readStatus(t, shed.StatusPath, shed.StatusLockPath)

	_, err := shed.Step(context.Background())
	if err == nil {
		t.Fatal("Step(...) = nil error; want a refusal")
	}
	for _, want := range []string{"lyx shed goto", "A, B"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not contain %q", err, want)
		}
	}
	if after := readStatus(t, shed.StatusPath, shed.StatusLockPath); !reflect.DeepEqual(before, after) {
		t.Errorf("status file changed on a lookup refusal")
	}

	got, err := Goto(gotoRequest(shed, "B"))
	if err != nil {
		t.Fatalf("Goto(...) = _, %v; want nil error", err)
	}
	if got.CurrentProducer != "B" || got.State != StatePaused {
		t.Errorf("CurrentProducer, State = %q, %q; want B, paused", got.CurrentProducer, got.State)
	}
	if _, err := shed.Step(context.Background()); err != nil {
		t.Fatalf("Step after goto = _, %v; want nil error", err)
	}
	if a.calls != 0 || b.calls != 1 {
		t.Errorf("calls A, B = %d, %d; want 0, 1", a.calls, b.calls)
	}
}

func TestStep_BudgetExhaustionNamesGotoAndGotoRestoresBudget(t *testing.T) {
	shed, statusPath, statusLockPath := scriptedStuckShed(t, 1, []bool{false})

	if res, err := shed.Step(context.Background()); err != nil || res.State != StateRunning {
		t.Fatalf("first Step = %q, %v; want running bounce", res.State, err)
	}
	res, err := shed.Step(context.Background())
	if err != nil {
		t.Fatalf("second Step = _, %v; want nil", err)
	}
	if res.State != StateBlocked {
		t.Fatalf("second Step State = %q; want blocked", res.State)
	}
	if got := readStatus(t, statusPath, statusLockPath); !strings.Contains(got.Error, "lyx shed goto") || !strings.HasPrefix(got.Error, ReasonBounceBudgetExhausted) {
		t.Errorf("persisted Error = %q; want the budget prefix and a lyx shed goto way forward", got.Error)
	}

	if _, err := Goto(gotoRequest(shed, "Wait")); err != nil {
		t.Fatalf("Goto(...) = _, %v; want nil error", err)
	}
	res, err = shed.Step(context.Background())
	if err != nil {
		t.Fatalf("Step after goto = _, %v; want nil", err)
	}
	if res.State != StateRunning || res.Next != "Wait" {
		t.Errorf("Step after goto State/Next = %q/%q; want a bounce, running/Wait", res.State, res.Next)
	}
}
