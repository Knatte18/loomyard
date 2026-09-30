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
	seed := commonSeed("B")
	seed.State = StateBlocked
	seed.Error = "stuck on B"
	seed.Transient = "network"
	seed.PauseRequested = true
	seedStatus(t, shed.StatusPath, shed.StatusLockPath, seed)

	returned, err := Goto(gotoRequest(shed, "A"))
	if err != nil {
		t.Fatalf("Goto(...) = _, %v; want nil error", err)
	}

	got := readStatus(t, shed.StatusPath, shed.StatusLockPath)
	if !reflect.DeepEqual(returned, got) {
		t.Errorf("returned Status = %+v; want the written file %+v", returned, got)
	}
	if got.CurrentProducer != "A" || got.State != StatePaused {
		t.Errorf("CurrentProducer, State = %q, %q; want A, paused", got.CurrentProducer, got.State)
	}
	if got.Error != "" || got.Transient != "" || got.PauseRequested {
		t.Errorf("Error, Transient, PauseRequested = %q, %q, %v; want all cleared", got.Error, got.Transient, got.PauseRequested)
	}
	if len(got.History) != 1 || got.History[0].Producer != "A" || got.History[0].Outcome != OutcomeGoto {
		t.Fatalf("History = %+v; want one goto entry for A", got.History)
	}
	assertRFC3339UTC(t, got.History[0].At)
	if got.Activity.Now != "A" || got.Activity.Last != "A → goto" {
		t.Errorf("Activity = %+v; want Now A, Last %q", got.Activity, "A → goto")
	}
}

func TestGoto_RunLockHeldRefusesAndLeavesFile(t *testing.T) {
	shed, _, _ := gotoShed(t)
	seed := commonSeed("B")
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

	_, err = Goto(gotoRequest(shed, "A"))
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
	seed := commonSeed("B")
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
	if _, err := Goto(gotoRequest(shed, "A")); err != nil {
		t.Errorf("Goto to a listed name = %v; want nil", err)
	}
}

// TestGoto_MissingStatusFileNamesSeeding pins goto's missing-status refusal to step's way forward:
// Shed never seeds a status file, so the run is seeded, after which goto moves it.
func TestGoto_MissingStatusFileNamesSeeding(t *testing.T) {
	shed, _, _ := gotoShed(t)

	_, err := Goto(gotoRequest(shed, "A"))
	if err == nil {
		t.Fatal("Goto(...) over a missing status file = nil error; want a refusal")
	}
	if !strings.Contains(err.Error(), missingStatusWayForward) {
		t.Errorf("error %q does not name seeding the run as its way forward", err)
	}

	seed := commonSeed("B")
	seed.State = StateBlocked
	seedStatus(t, shed.StatusPath, shed.StatusLockPath, seed)
	if _, err := Goto(gotoRequest(shed, "A")); err != nil {
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
	seed := commonSeed("B")
	seed.State = StateBlocked
	seed.Error = "stuck on A"
	seedStatus(t, shed.StatusPath, shed.StatusLockPath, seed)

	if _, err := Goto(gotoRequest(shed, "A")); err != nil {
		t.Fatalf("Goto(...) = _, %v; want nil error", err)
	}
	res, err := shed.Step(context.Background())
	if err != nil {
		t.Fatalf("Step(...) = _, %v; want nil error", err)
	}
	if a.calls != 1 || b.calls != 0 {
		t.Errorf("calls A, B = %d, %d; want 1, 0 -- the step must resume at the goto target", a.calls, b.calls)
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

	got, err := Goto(gotoRequest(shed, "A"))
	if err != nil {
		t.Fatalf("Goto(...) = _, %v; want nil error", err)
	}
	if got.CurrentProducer != "A" || got.State != StatePaused {
		t.Errorf("CurrentProducer, State = %q, %q; want A, paused", got.CurrentProducer, got.State)
	}
	if _, err := shed.Step(context.Background()); err != nil {
		t.Fatalf("Step after goto = _, %v; want nil error", err)
	}
	if a.calls != 1 || b.calls != 0 {
		t.Errorf("calls A, B = %d, %d; want 1, 0", a.calls, b.calls)
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

// tailProducers is shaped like the loom recipe's tail: a segment with an offshoot, then the approval rows.
func tailProducers() []ProducerDef {
	return []ProducerDef{
		{Name: "Webster", Segment: "Webster-Review", OnDone: "Webster-Bouncer"},
		{Name: "Webster-Bouncer", Segment: "Webster-Review", OnDone: "Describe", OnStuck: "Webster-Burler"},
		{Name: "Webster-Burler", Segment: "Webster-Review", OnDone: "Webster-Bouncer", OnStuck: "Webster-Bouncer"},
		{Name: "Describe"},
		{Name: "Publish"},
		{Name: "PR-Gate"},
		{Name: "PR-Rework"},
		{Name: "Finalize"},
	}
}

func tailShed(t *testing.T, current string, state State, history []HistoryEntry) *Shed {
	t.Helper()
	shed, _, _, _ := newTestShed(t)
	shed.Producers = tailProducers()
	seed := commonSeed(current)
	seed.State = state
	seed.History = history
	seedStatus(t, shed.StatusPath, shed.StatusLockPath, seed)
	return shed
}

func assertGotoRefused(t *testing.T, shed *Shed, target string) error {
	t.Helper()
	before := readStatus(t, shed.StatusPath, shed.StatusLockPath)
	_, err := Goto(gotoRequest(shed, target))
	if err == nil {
		t.Fatalf("Goto --to %s succeeded; want a refusal", target)
	}
	if after := readStatus(t, shed.StatusPath, shed.StatusLockPath); !reflect.DeepEqual(before, after) {
		t.Errorf("status file changed on a refusal of --to %s", target)
	}
	return err
}

func TestGoto_AwaitingRefusesFinalizeAndAdmitsEarlierRow(t *testing.T) {
	shed := tailShed(t, "PR-Gate", StateAwaiting, nil)

	err := assertGotoRefused(t, shed, "Finalize")
	msg := err.Error()
	if !strings.Contains(msg, "way forward: re-run goto with --to naming one of: Webster, Webster-Bouncer, Webster-Burler, Describe, Publish") {
		t.Errorf("error %q does not list Webster through Publish in list order", msg)
	}
	list := msg[strings.LastIndex(msg, "one of: "):]
	for _, bad := range []string{"PR-Gate", "Finalize"} {
		if strings.Contains(list, bad) {
			t.Errorf("admitted list %q names %s", list, bad)
		}
	}
	if !strings.Contains(msg, "lyx shed status") {
		t.Errorf("error %q does not name lyx shed status", msg)
	}

	got, err := Goto(gotoRequest(shed, "Publish"))
	if err != nil {
		t.Fatalf("Goto --to Publish = _, %v; want nil", err)
	}
	if got.CurrentProducer != "Publish" || got.State != StatePaused {
		t.Errorf("CurrentProducer, State = %q, %q; want Publish, paused", got.CurrentProducer, got.State)
	}
}

func TestGoto_BlockedAtBouncerRefusesForwardRows(t *testing.T) {
	shed := tailShed(t, "Webster-Bouncer", StateBlocked, nil)
	for _, target := range []string{"Describe", "Publish"} {
		assertGotoRefused(t, shed, target)
	}
	if _, err := Goto(gotoRequest(shed, "Webster-Bouncer")); err != nil {
		t.Errorf("Goto --to Webster-Bouncer = %v; want nil", err)
	}
	if _, err := Goto(gotoRequest(shed, "Webster")); err != nil {
		t.Errorf("Goto --to Webster = %v; want nil", err)
	}
}

func TestGoto_RunningRunRefusedUntilPaused(t *testing.T) {
	shed := tailShed(t, "Describe", StateRunning, nil)
	err := assertGotoRefused(t, shed, "Webster")
	for _, want := range []string{"lyx shed pause", "lyx shed step"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not name %q", err, want)
		}
	}

	cur := readStatus(t, shed.StatusPath, shed.StatusLockPath)
	cur.PauseRequested = true
	seedStatus(t, shed.StatusPath, shed.StatusLockPath, cur)
	for i := range shed.Producers {
		shed.Producers[i].Producer = fixedOutcomeProducer(Done, "")
	}
	if _, err := shed.Step(context.Background()); err != nil {
		t.Fatalf("Step = _, %v; want nil", err)
	}
	if got := readStatus(t, shed.StatusPath, shed.StatusLockPath); got.State != StatePaused {
		t.Fatalf("State after the step = %q; want paused", got.State)
	}
	if _, err := Goto(gotoRequest(shed, "Webster")); err != nil {
		t.Errorf("Goto --to Webster once paused = %v; want nil", err)
	}
}

func TestGoto_RemovedCurrentRowUsesLatestHistoryEntry(t *testing.T) {
	history := []HistoryEntry{{Producer: "Webster", Outcome: Done}}
	shed := tailShed(t, "Removed", StateBlocked, history)
	assertGotoRefused(t, shed, "Describe")
	if _, err := Goto(gotoRequest(shed, "Webster-Bouncer")); err != nil {
		t.Errorf("Goto --to Webster-Bouncer (Webster's OnDone) = %v; want nil", err)
	}

	empty := tailShed(t, "Removed", StateBlocked, nil)
	assertGotoRefused(t, empty, "Webster-Bouncer")
	if _, err := Goto(gotoRequest(empty, "Webster")); err != nil {
		t.Errorf("Goto --to Webster with an empty history = %v; want nil", err)
	}
}

func TestGoto_BlockedAtOffshootAdmitsPartnerNotSuccessor(t *testing.T) {
	shed := tailShed(t, "PR-Rework", StateBlocked, nil)
	assertGotoRefused(t, shed, "Finalize")
	if _, err := Goto(gotoRequest(shed, "PR-Gate")); err != nil {
		t.Errorf("Goto --to PR-Gate = %v; want nil", err)
	}
}
