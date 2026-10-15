// reroute_test.go covers the reroute edge and the halting Stuck: a reroute's routing and episode effect, the three-reroute cap, the wiring fault and OnReroute validation.

package shedengine

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

// rerouteOutcome is one scripted call of a reroute test's producer.
type rerouteOutcome int

const (
	callReroute rerouteOutcome = iota
	callDone
)

// rerouteSpans is the gate span every scripted reroute pointer carries, which each history entry of a reroute verdict must keep.
var rerouteSpans = []GateSpan{{Gate: "parent-review", Kind: GateSpanKindHeld, Start: "2026-08-15T09:00:00Z", End: "2026-08-15T09:00:05Z"}}

// scriptedRerouteProducer returns the scripted outcomes in order, repeating the last one.
func scriptedRerouteProducer(script []rerouteOutcome) *funcProducer {
	i := 0
	return &funcProducer{fn: func(context.Context) (Outcome, OutputPointer, error) {
		call := script[min(i, len(script)-1)]
		i++
		if call == callDone {
			return Done, OutputPointer{}, nil
		}
		return Stuck, OutputPointer{Reroute: true, Reason: "parent moved", GateSpans: rerouteSpans}, nil
	}}
}

// rerouteShed wires row "A" (rerouting to "B" as scripted) and a row "B" that is Done and routes back to "A".
func rerouteShed(t *testing.T, script []rerouteOutcome) (*Shed, *funcProducer) {
	t.Helper()
	shed, statusPath, _, statusLockPath := newTestShed(t)
	a := scriptedRerouteProducer(script)
	shed.Producers = []ProducerDef{
		{Name: "A", Producer: a, OnReroute: "B", OnDone: "B"},
		{Name: "B", Producer: fixedOutcomeProducer(Done, ""), OnDone: "A"},
	}
	seedStatus(t, statusPath, statusLockPath, commonSeed("A"))
	return shed, a
}

func countOutcome(history []HistoryEntry, outcome Outcome) int {
	count := 0
	for _, entry := range history {
		if entry.Outcome == outcome {
			count++
		}
	}
	return count
}

func TestStep_RerouteMovesToTargetAndEndsTargetSegmentEpisode(t *testing.T) {
	shed, statusPath, _, statusLockPath := newTestShed(t)
	shed.Producers = []ProducerDef{
		{Name: "A", Producer: scriptedRerouteProducer([]rerouteOutcome{callReroute}), OnReroute: "B"},
		{Name: "B", Segment: "seg", Producer: fixedOutcomeProducer(Done, ""), OnStuck: "C"},
		{Name: "C", Segment: "seg", Producer: fixedOutcomeProducer(Done, "")},
	}
	seed := commonSeed("A")
	seed.History = []HistoryEntry{{Producer: "B", Outcome: Stuck}, {Producer: "C", Outcome: Stuck}}
	seedStatus(t, statusPath, statusLockPath, seed)

	res, err := shed.Step(context.Background())
	if err != nil {
		t.Fatalf("Step = _, %v; want nil", err)
	}

	got := readStatus(t, statusPath, statusLockPath)
	if res.Next != "B" || res.State != StateRunning || got.CurrentProducer != "B" || got.State != StateRunning {
		t.Fatalf("Step Next/State = %q/%q, file %q/%q; want B/running", res.Next, res.State, got.CurrentProducer, got.State)
	}
	last := got.History[len(got.History)-1]
	if last.Producer != "A" || last.Outcome != OutcomeReroute || last.RoutedTo != "B" || last.BudgetExempt {
		t.Errorf("last history entry = %+v; want A's reroute entry routed to B, spending no budget", last)
	}
	assertRFC3339UTC(t, last.At)
	if !reflect.DeepEqual(last.GateSpans, rerouteSpans) {
		t.Errorf("reroute entry GateSpans = %+v; want %+v", last.GateSpans, rerouteSpans)
	}
	if got.Activity.Last != "A → rerouted to B" {
		t.Errorf("Activity.Last = %q; want %q", got.Activity.Last, "A → rerouted to B")
	}
	for _, def := range shed.Producers[1:] {
		if n := episodeStuckCount(got.History, def, shed.Producers); n != 0 {
			t.Errorf("episodeStuckCount(%s) = %d; want 0 after a reroute into its segment", def.Name, n)
		}
	}
}

func TestRun_RerouteCapHaltsOnFourthRerouteAndResumeGrantsThreeMore(t *testing.T) {
	for _, tc := range []struct {
		name       string
		wayForward string
		wantClause string
	}{
		{"told way forward", "run \"lyx batten land\" again", "way forward: run \"lyx batten land\" again"},
		{"fallback way forward", "", "way forward: \"lyx shed run\" resumes the row"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			shed, a := rerouteShed(t, []rerouteOutcome{callReroute})
			shed.RerouteCapWayForward = tc.wayForward

			for halt := 1; halt <= 2; halt++ {
				res, err := shed.Run(context.Background())
				if err != nil {
					t.Fatalf("Run %d = _, %v; want nil", halt, err)
				}
				if res.Outcome != RunBlocked || res.HaltedProducer != "A" {
					t.Fatalf("Run %d = %q at %q; want blocked at A", halt, res.Outcome, res.HaltedProducer)
				}
				if !strings.Contains(res.Reason, "reroute cap reached: A rerouted to B 3 times since its last done; parent moved") || !strings.Contains(res.Reason, tc.wantClause) {
					t.Fatalf("Run %d reason = %q; want the cap reason with %q", halt, res.Reason, tc.wantClause)
				}
				last := res.History[len(res.History)-1]
				if last.Outcome != Stuck || !last.RerouteCapped || !last.BudgetExempt {
					t.Fatalf("Run %d last entry = %+v; want a budget-exempt capped Stuck", halt, last)
				}
				if !reflect.DeepEqual(last.GateSpans, rerouteSpans) {
					t.Errorf("Run %d capped Stuck GateSpans = %+v; want %+v", halt, last.GateSpans, rerouteSpans)
				}
			}
			if a.calls != 8 {
				t.Errorf("row A calls = %d; want 8, four per run (three reroutes, then the halt)", a.calls)
			}
		})
	}
}

func TestRun_DoneOfRowResetsRerouteCount(t *testing.T) {
	script := []rerouteOutcome{callReroute, callReroute, callDone, callReroute}
	shed, a := rerouteShed(t, script)

	res, err := shed.Run(context.Background())
	if err != nil {
		t.Fatalf("Run = _, %v; want nil", err)
	}

	if res.Outcome != RunBlocked || a.calls != 7 || countOutcome(res.History, OutcomeReroute) != 5 {
		t.Fatalf("Run = %q after %d calls with %d reroutes; want blocked after 7 calls and 5 reroutes (two, a done, then three more before the cap)", res.Outcome, a.calls, countOutcome(res.History, OutcomeReroute))
	}
}

func TestStep_RerouteWithoutOnRerouteFailsRun(t *testing.T) {
	shed, statusPath, _, statusLockPath := newTestShed(t)
	shed.Producers = []ProducerDef{{Name: "A", Producer: scriptedRerouteProducer([]rerouteOutcome{callReroute})}}
	seedStatus(t, statusPath, statusLockPath, commonSeed("A"))

	_, err := shed.Step(context.Background())

	if err == nil || !strings.Contains(err.Error(), `"A"`) || !strings.Contains(err.Error(), "OnReroute") {
		t.Fatalf("Step error = %v; want a wiring fault naming row A and OnReroute", err)
	}
	got := readStatus(t, statusPath, statusLockPath)
	if got.State != StateFailed || got.CurrentProducer != "A" {
		t.Errorf("status = %q at %q; want failed at A", got.State, got.CurrentProducer)
	}
	if last := got.History[len(got.History)-1]; !reflect.DeepEqual(last.GateSpans, rerouteSpans) {
		t.Errorf("persisted Stuck GateSpans = %+v; want %+v", last.GateSpans, rerouteSpans)
	}
}

func TestStep_HaltStuckBlocksAtRowDespiteOnStuckAndSpendsNoBudget(t *testing.T) {
	shed, statusPath, _, statusLockPath := newTestShed(t)
	shed.Producers = []ProducerDef{{
		Name:       "Gate",
		OnStuck:    "Gate",
		MaxBounces: 1,
		Producer: &funcProducer{fn: func(context.Context) (Outcome, OutputPointer, error) {
			return Stuck, OutputPointer{Halt: true, Reason: "lock wait paused"}, nil
		}},
	}}
	seedStatus(t, statusPath, statusLockPath, commonSeed("Gate"))

	for i := range 2 {
		res, err := shed.Step(context.Background())
		if err != nil {
			t.Fatalf("Step %d = _, %v; want nil", i, err)
		}
		if res.State != StateBlocked || res.Next != "Gate" || res.Reason != "lock wait paused" {
			t.Fatalf("Step %d = %q at %q reason %q; want blocked at Gate with the producer's reason", i, res.State, res.Next, res.Reason)
		}
	}
	got := readStatus(t, statusPath, statusLockPath)
	if n := episodeStuckCount(got.History, shed.Producers[0], shed.Producers); n != 0 || !got.History[0].BudgetExempt {
		t.Errorf("episode Stuck count = %d, first entry %+v; want 0 and a budget-exempt entry", n, got.History[0])
	}
}

func TestStep_OnRerouteValidation(t *testing.T) {
	for _, tc := range []struct {
		name      string
		onReroute string
		segment   string
		wantErr   string
	}{
		{"unknown target", "Nowhere", "", "OnReroute \"Nowhere\" naming no producer"},
		{"self", "A", "", "OnReroute naming itself"},
		{"cross-segment target", "B", "seg", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			shed, statusPath, _, statusLockPath := newTestShed(t)
			shed.Producers = []ProducerDef{
				{Name: "A", Segment: tc.segment, Producer: fixedOutcomeProducer(Done, ""), OnReroute: tc.onReroute},
				{Name: "B", Producer: fixedOutcomeProducer(Done, "")},
			}
			seedStatus(t, statusPath, statusLockPath, commonSeed("A"))

			_, err := shed.Step(context.Background())

			switch {
			case tc.wantErr == "" && err != nil:
				t.Fatalf("Step error = %v; want it accepted", err)
			case tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr)):
				t.Fatalf("Step error = %v; want it to contain %q", err, tc.wantErr)
			}
		})
	}
}
