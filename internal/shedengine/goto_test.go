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

//testtiming:keep pins that goto clears Error, Transient and PauseRequested and records the goto history entry and activity, which its covering tests do not
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
	req := gotoRequest(shed, "A")
	req.RunID = "some-slug"
	if _, err := Goto(req); err == nil || !strings.Contains(err.Error(), `"lyx shed pause some-slug"`) || !strings.Contains(err.Error(), `"lyx shed status some-slug"`) {
		t.Errorf("Goto(...) told RunID error = %v; want the pause and status verbs addressing some-slug", err)
	}
	if after := readStatus(t, shed.StatusPath, shed.StatusLockPath); !reflect.DeepEqual(before, after) {
		t.Errorf("status file changed on a busy refusal: %+v -> %+v", before, after)
	}
}

// TestGoto_RefusalNamesWayForward pins that a refused goto names its way forward and leaves the status file untouched,
// and that taking the way forward (where one exists) moves the run.
func TestGoto_RefusalNamesWayForward(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		state   State
		target  string
		wantMsg string
		// recoverTo is the target a goto takes after the refusal; empty when the refusal is terminal.
		recoverTo string
	}{
		{"unknown target lists valid names in list order", StateBlocked, "Z", "way forward: re-run goto with --to naming one of: A, B", "A"},
		{"done run names seeding a new run", StateDone, "A", "seed a new run", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			shed, _, _ := gotoShed(t)
			seed := commonSeed("B")
			seed.State = tt.state
			seedStatus(t, shed.StatusPath, shed.StatusLockPath, seed)
			before := readStatus(t, shed.StatusPath, shed.StatusLockPath)

			_, err := Goto(gotoRequest(shed, tt.target))
			if err == nil {
				t.Fatal("Goto(...) = nil error; want a refusal")
			}
			if !strings.Contains(err.Error(), tt.wantMsg) {
				t.Errorf("error %q does not contain %q", err, tt.wantMsg)
			}
			if after := readStatus(t, shed.StatusPath, shed.StatusLockPath); !reflect.DeepEqual(before, after) {
				t.Errorf("status file changed on a refusal")
			}
			if tt.recoverTo != "" {
				if _, err := Goto(gotoRequest(shed, tt.recoverTo)); err != nil {
					t.Errorf("Goto to a listed name = %v; want nil", err)
				}
			}
		})
	}
}

// TestGoto_MissingStatusFileWayForward pins that goto over a missing status file names seeding the run
// (a told clause replaces the generic seed advice), after which goto moves the seeded run.
func TestGoto_MissingStatusFileWayForward(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		told       string
		wantMsg    string
		wantAbsent string
	}{
		{"generic seed advice", "", missingStatusWayForward, ""},
		{"told clause replaces the seed advice", "way forward: told clause", "way forward: told clause", "lyx shed seed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			shed, _, _ := gotoShed(t)
			req := gotoRequest(shed, "A")
			req.MissingStatusWayForward = tt.told

			_, err := Goto(req)
			if err == nil {
				t.Fatal("Goto(...) over a missing status file = nil error; want a refusal")
			}
			if !strings.Contains(err.Error(), tt.wantMsg) {
				t.Errorf("error %q does not name %q", err, tt.wantMsg)
			}
			if tt.wantAbsent != "" && strings.Contains(err.Error(), tt.wantAbsent) {
				t.Errorf("error %q names %q; want the told clause instead", err, tt.wantAbsent)
			}

			seed := commonSeed("B")
			seed.State = StateBlocked
			seedStatus(t, shed.StatusPath, shed.StatusLockPath, seed)
			if _, err := Goto(gotoRequest(shed, "A")); err != nil {
				t.Errorf("Goto(...) once the run is seeded = %v; want nil", err)
			}
		})
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
	told := gotoRequest(shed, "Finalize")
	told.RunID = "some-slug"
	if _, err := Goto(told); err == nil || !strings.Contains(err.Error(), `"lyx shed status some-slug"`) {
		t.Errorf("Goto(...) told RunID error = %v; want lyx shed status addressing some-slug", err)
	}

	got, err := Goto(gotoRequest(shed, "Publish"))
	if err != nil {
		t.Fatalf("Goto --to Publish = _, %v; want nil", err)
	}
	if got.CurrentProducer != "Publish" || got.State != StatePaused {
		t.Errorf("CurrentProducer, State = %q, %q; want Publish, paused", got.CurrentProducer, got.State)
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

// TestGoto_BlockedRunAdmitsOnlyEarlierRows pins which rows a blocked run's goto refuses and admits,
// relative to the row it is blocked at: forward rows are refused, the row itself, an earlier row and an offshoot's partner are admitted.
// A row removed from the list falls back to the latest history entry's successor.
func TestGoto_BlockedRunAdmitsOnlyEarlierRows(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		current string
		history []HistoryEntry
		refuse  []string
		admit   []string
	}{
		{"blocked at a bouncer refuses forward rows", "Webster-Bouncer", nil, []string{"Describe", "Publish"}, []string{"Webster-Bouncer", "Webster"}},
		{"blocked at an offshoot admits the partner, not the successor", "PR-Rework", nil, []string{"Finalize"}, []string{"PR-Gate"}},
		{"removed current row uses the latest history entry", "Removed", []HistoryEntry{{Producer: "Webster", Outcome: Done}}, []string{"Describe"}, []string{"Webster-Bouncer"}},
		{"removed current row with an empty history admits only the first row", "Removed", nil, []string{"Webster-Bouncer"}, []string{"Webster"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			shed := tailShed(t, tt.current, StateBlocked, tt.history)
			for _, target := range tt.refuse {
				assertGotoRefused(t, shed, target)
			}
			for _, target := range tt.admit {
				if _, err := Goto(gotoRequest(shed, target)); err != nil {
					t.Errorf("Goto --to %s = %v; want nil", target, err)
				}
			}
		})
	}
}
