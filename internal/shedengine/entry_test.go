// entry_test.go covers the entry gate: a gated row's evaluation when the run enters it, the skip of a row whose gate does not hold and the chain of consecutive skips.

package shedengine

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// fakeGate is an EntryGate whose answer a test changes between steps, counting its evaluations.
type fakeGate struct {
	holds  bool
	reason string
	err    error
	calls  int
}

func (g *fakeGate) evaluate() (bool, string, error) {
	g.calls++
	return g.holds, g.reason, g.err
}

// outcomeSequence returns a producer that reports outcomes in order and repeats the last one.
func outcomeSequence(outcomes ...Outcome) *funcProducer {
	next := 0
	return &funcProducer{
		fn: func(context.Context) (Outcome, OutputPointer, error) {
			outcome := outcomes[min(next, len(outcomes)-1)]
			next++
			return outcome, OutputPointer{Reason: "stopped"}, nil
		},
	}
}

// blockShapedList returns a list shaped like an expanded block: the block row "Blk" gated by outer, whose Done enters "In1" and whose skip routes to "Exit";
// the inlined rows "In1" and "In2" in block "B"; the inner block row "Inner" in block "B" gated by inner, whose skip routes to "Exit"; and the awaiting row "Exit".
func blockShapedList(outer, inner *fakeGate) (producers []ProducerDef, in1, in2, exit *funcProducer) {
	in1, in2, exit = fixedOutcomeProducer(Done, ""), fixedOutcomeProducer(Done, ""), outcomeSequence(Awaiting)
	producers = []ProducerDef{
		{Name: "Blk", Producer: fixedOutcomeProducer(Done, ""), OnDone: "In1", SkipTo: "Exit", EntryGate: outer.evaluate},
		{Name: "In1", Producer: in1, OnDone: "Inner", Block: "B"},
		{Name: "Inner", Producer: fixedOutcomeProducer(Done, ""), OnDone: "In2", SkipTo: "Exit", Block: "B", EntryGate: inner.evaluate},
		{Name: "In2", Producer: in2, OnDone: "Exit", Block: "B/Inner"},
		{Name: "Exit", Producer: exit},
	}
	return producers, in1, in2, exit
}

func TestStep_EntryGate(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	step := func(t *testing.T, shed *Shed) StepResult {
		t.Helper()
		res, err := shed.Step(context.Background())
		if err != nil {
			t.Fatalf("Step(...) = _, %v; want nil error", err)
		}
		return res
	}

	t.Run("a false gate records skipped with its reason, follows on_done and ends the row's bounce episode", func(t *testing.T) {
		t.Parallel()
		shed, statusPath, _, statusLockPath := newTestShed(t)
		off := &fakeGate{reason: "review budget is 0"}
		a, b := fixedOutcomeProducer(Done, ""), outcomeSequence(Awaiting)
		shed.Producers = []ProducerDef{
			{Name: "A", Producer: a, OnDone: "B", EntryGate: off.evaluate},
			{Name: "B", Producer: b},
		}
		seed := commonSeed("A")
		seed.History = []HistoryEntry{{Producer: "A", Outcome: Stuck}, {Producer: "A", Outcome: Stuck}}
		seedStatus(t, statusPath, statusLockPath, seed)

		res := step(t, shed)
		if a.calls != 0 || b.calls != 1 || res.Producer != "B" || res.State != StateAwaiting {
			t.Fatalf("calls A, B = %d, %d, Producer %q, State %q; want 0, 1, B, awaiting", a.calls, b.calls, res.Producer, res.State)
		}
		skipped := res.History[2]
		if skipped.Producer != "A" || skipped.Outcome != OutcomeSkipped || skipped.Reason != "review budget is 0" {
			t.Errorf("History[2] = %+v; want A skipped with the gate's reason", skipped)
		}
		assertRFC3339UTC(t, skipped.At)
		if count := episodeStuckCount(res.History, shed.Producers[0], shed.Producers); count != 0 {
			t.Errorf("episodeStuckCount(A) = %d; want 0, since the skip ends the episode", count)
		}
		if got := readStatus(t, statusPath, statusLockPath); got.EnteredRow != "" || got.CurrentProducer != "B" {
			t.Errorf("persisted EnteredRow, CurrentProducer = %q, %q; want empty, B", got.EnteredRow, got.CurrentProducer)
		}
	})

	t.Run("a chain of two skips runs in one step", func(t *testing.T) {
		t.Parallel()
		shed, statusPath, _, statusLockPath := newTestShed(t)
		offA, offB := &fakeGate{reason: "a off"}, &fakeGate{reason: "b off"}
		c := outcomeSequence(Awaiting)
		shed.Producers = []ProducerDef{
			{Name: "A", Producer: fixedOutcomeProducer(Done, ""), OnDone: "B", EntryGate: offA.evaluate},
			{Name: "B", Producer: fixedOutcomeProducer(Done, ""), OnDone: "C", EntryGate: offB.evaluate},
			{Name: "C", Producer: c},
		}
		seedStatus(t, statusPath, statusLockPath, commonSeed("A"))

		res := step(t, shed)
		if c.calls != 1 || len(res.History) != 3 || res.History[0].Outcome != OutcomeSkipped || res.History[1].Outcome != OutcomeSkipped || res.History[2].Outcome != Awaiting {
			t.Fatalf("calls C = %d, History = %+v; want C called once after two skips", c.calls, res.History)
		}
		if got := readStatus(t, statusPath, statusLockPath); got.Activity.Last != "C → awaiting" {
			t.Errorf("Activity.Last = %q; want %q", got.Activity.Last, "C → awaiting")
		}
	})

	t.Run("a skip of a row with an empty on_done finishes the run done", func(t *testing.T) {
		t.Parallel()
		shed, statusPath, _, statusLockPath := newTestShed(t)
		off := &fakeGate{reason: "off"}
		shed.Producers = []ProducerDef{{Name: "A", Producer: fixedOutcomeProducer(Done, ""), EntryGate: off.evaluate}}
		seedStatus(t, statusPath, statusLockPath, commonSeed("A"))

		res := step(t, shed)
		if res.State != StateDone || res.Next != "A" || len(res.History) != 1 || res.History[0].Outcome != OutcomeSkipped {
			t.Errorf("State, Next, History = %q, %q, %+v; want done at A with one skipped entry", res.State, res.Next, res.History)
		}
		if got := readStatus(t, statusPath, statusLockPath); got.State != StateDone {
			t.Errorf("persisted State = %q; want done", got.State)
		}
	})

	t.Run("a chain longer than the list writes failed naming the rows", func(t *testing.T) {
		t.Parallel()
		shed, statusPath, _, statusLockPath := newTestShed(t)
		offA, offB := &fakeGate{reason: "a off"}, &fakeGate{reason: "b off"}
		shed.Producers = []ProducerDef{
			{Name: "A", Producer: fixedOutcomeProducer(Done, ""), OnDone: "B", EntryGate: offA.evaluate},
			{Name: "B", Producer: fixedOutcomeProducer(Done, ""), OnDone: "A", EntryGate: offB.evaluate},
		}
		seedStatus(t, statusPath, statusLockPath, commonSeed("A"))

		_, err := shed.Step(context.Background())
		if err == nil || !strings.Contains(err.Error(), "A, B") || !strings.Contains(err.Error(), "lym shed goto") {
			t.Fatalf("Step error = %v; want it to name the chain A, B and lym shed goto", err)
		}
		got := readStatus(t, statusPath, statusLockPath)
		if got.State != StateFailed || !strings.Contains(got.Error, "A, B") || got.EnteredRow != "" {
			t.Errorf("persisted State, Error, EnteredRow = %q, %q, %q; want failed naming the chain, nothing entered", got.State, got.Error, got.EnteredRow)
		}
	})

	t.Run("an evaluation error writes failed, leaves the row not entered and a resume evaluates again", func(t *testing.T) {
		t.Parallel()
		shed, statusPath, _, statusLockPath := newTestShed(t)
		broken := &fakeGate{err: errors.New("settings unreadable")}
		a := fixedOutcomeProducer(Done, "")
		shed.Producers = []ProducerDef{{Name: "A", Producer: a, EntryGate: broken.evaluate}}
		seedStatus(t, statusPath, statusLockPath, commonSeed("A"))

		_, err := shed.Step(context.Background())
		if err == nil || !errors.Is(err, broken.err) {
			t.Fatalf("Step error = %v; want it to wrap the gate's error", err)
		}
		got := readStatus(t, statusPath, statusLockPath)
		if got.State != StateFailed || !strings.Contains(got.Error, "settings unreadable") || got.EnteredRow != "" || a.calls != 0 {
			t.Errorf("persisted State, Error, EnteredRow = %q, %q, %q, calls A = %d; want failed with the gate's error, nothing entered, A uncalled", got.State, got.Error, got.EnteredRow, a.calls)
		}

		broken.err, broken.holds = nil, true
		step(t, shed)
		if broken.calls != 2 || a.calls != 1 {
			t.Errorf("gate calls, calls A = %d, %d; want 2, 1: the resume evaluates the gate again", broken.calls, a.calls)
		}
	})

	t.Run("a resume of an entered row re-calls it without evaluating", func(t *testing.T) {
		t.Parallel()
		shed, statusPath, _, statusLockPath := newTestShed(t)
		on := &fakeGate{holds: true}
		a := outcomeSequence(Awaiting, Done)
		shed.Producers = []ProducerDef{{Name: "A", Producer: a, EntryGate: on.evaluate}}
		seedStatus(t, statusPath, statusLockPath, commonSeed("A"))

		if res := step(t, shed); res.State != StateAwaiting {
			t.Fatalf("first Step State = %q; want awaiting", res.State)
		}
		if got := readStatus(t, statusPath, statusLockPath); got.EnteredRow != "A" {
			t.Fatalf("persisted EnteredRow = %q; want A while the row's verdict is a halt", got.EnteredRow)
		}
		on.holds = false
		if res := step(t, shed); res.State != StateDone {
			t.Errorf("second Step State = %q; want done", res.State)
		}
		if on.calls != 1 || a.calls != 2 {
			t.Errorf("gate calls, calls A = %d, %d; want 1, 2", on.calls, a.calls)
		}
	})

	t.Run("a false gate on a block row routes to its skip target and records no inlined row", func(t *testing.T) {
		t.Parallel()
		shed, statusPath, _, statusLockPath := newTestShed(t)
		outer := &fakeGate{reason: "block off"}
		var in1, exit *funcProducer
		shed.Producers, in1, _, exit = blockShapedList(outer, &fakeGate{holds: true})
		seedStatus(t, statusPath, statusLockPath, commonSeed("Blk"))

		res := step(t, shed)
		if in1.calls != 0 || exit.calls != 1 || len(res.History) != 2 {
			t.Fatalf("calls In1, Exit = %d, %d, History = %+v; want 0, 1 and the skip plus Exit's verdict", in1.calls, exit.calls, res.History)
		}
		if skipped := res.History[0]; skipped.Producer != "Blk" || skipped.Outcome != OutcomeSkipped || skipped.Reason != "block off" || skipped.Block != "" {
			t.Errorf("History[0] = %+v; want Blk skipped with the gate's reason and no block", skipped)
		}
		if res.History[1].Producer != "Exit" {
			t.Errorf("History[1].Producer = %q; want Exit, the skip target, not the first inlined row", res.History[1].Producer)
		}
	})

	t.Run("a holding block gate enters the block and the inlined row's entry carries the block path", func(t *testing.T) {
		t.Parallel()
		shed, statusPath, _, statusLockPath := newTestShed(t)
		shed.Producers, _, _, _ = blockShapedList(&fakeGate{holds: true}, &fakeGate{holds: true})
		seedStatus(t, statusPath, statusLockPath, commonSeed("Blk"))

		step(t, shed)
		res := step(t, shed)
		if len(res.History) != 2 || res.History[0].Producer != "Blk" || res.History[0].Block != "" || res.History[1].Producer != "In1" || res.History[1].Block != "B" {
			t.Errorf("History = %+v; want Blk's Done with no block, then In1's with block B", res.History)
		}
	})

	t.Run("a false gate on an inner block row routes to the exit and its skipped entry carries the enclosing block", func(t *testing.T) {
		t.Parallel()
		shed, statusPath, _, statusLockPath := newTestShed(t)
		var in2, exit *funcProducer
		shed.Producers, _, in2, exit = blockShapedList(&fakeGate{holds: true}, &fakeGate{reason: "inner off"})
		seedStatus(t, statusPath, statusLockPath, commonSeed("Blk"))

		step(t, shed)
		step(t, shed)
		res := step(t, shed)
		if in2.calls != 0 || exit.calls != 1 || len(res.History) != 4 {
			t.Fatalf("calls In2, Exit = %d, %d, History = %+v; want 0, 1 and four entries", in2.calls, exit.calls, res.History)
		}
		if skipped := res.History[2]; skipped.Producer != "Inner" || skipped.Outcome != OutcomeSkipped || skipped.Reason != "inner off" || skipped.Block != "B" {
			t.Errorf("History[2] = %+v; want Inner skipped with block B", skipped)
		}
	})

	routedBack := []struct {
		name string
		// producers builds the list over the gates of A and B.
		producers func(a, b *funcProducer, gateA, gateB *fakeGate) []ProducerDef
		// wantGateB is how many times B's gate is evaluated across both steps.
		wantGateB int
	}{
		{
			name: "a Stuck routed back to its own row through a skipped row evaluates that row's gate",
			producers: func(a, b *funcProducer, gateA, gateB *fakeGate) []ProducerDef {
				return []ProducerDef{
					{Name: "A", Producer: a, OnStuck: "B", Segment: "s", EntryGate: gateA.evaluate},
					{Name: "B", Producer: b, OnDone: "A", Segment: "s", EntryGate: gateB.evaluate},
				}
			},
			wantGateB: 1,
		},
		{
			name: "a gated row whose on_stuck names itself evaluates its gate again",
			producers: func(a, b *funcProducer, gateA, gateB *fakeGate) []ProducerDef {
				return []ProducerDef{{Name: "A", Producer: a, OnStuck: "A", Segment: "s", EntryGate: gateA.evaluate}}
			},
		},
	}
	for _, tt := range routedBack {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			shed, statusPath, _, statusLockPath := newTestShed(t)
			gateA, gateB := &fakeGate{holds: true}, &fakeGate{reason: "b off"}
			a, b := outcomeSequence(Stuck, Done), fixedOutcomeProducer(Done, "")
			shed.Producers = tt.producers(a, b, gateA, gateB)
			seedStatus(t, statusPath, statusLockPath, commonSeed("A"))

			step(t, shed)
			res := step(t, shed)
			if gateA.calls != 2 || gateB.calls != tt.wantGateB || a.calls != 2 || res.State != StateDone {
				t.Errorf("gate calls A, B = %d, %d, calls A = %d, State %q; want 2, %d, 2, done", gateA.calls, gateB.calls, a.calls, res.State, tt.wantGateB)
			}
		})
	}

	t.Run("a pause_before on a gated row halts before evaluation and a gate answer changed during the pause decides", func(t *testing.T) {
		t.Parallel()
		shed, statusPath, _, statusLockPath := newTestShed(t)
		gateA, gateB := &fakeGate{reason: "a off"}, &fakeGate{holds: true}
		b := fixedOutcomeProducer(Done, "")
		shed.Producers = []ProducerDef{
			{Name: "A", Producer: fixedOutcomeProducer(Done, ""), OnDone: "B", EntryGate: gateA.evaluate},
			{Name: "B", Producer: b, EntryGate: gateB.evaluate},
		}
		seed := commonSeed("A")
		seed.PauseBefore = "B"
		seedStatus(t, statusPath, statusLockPath, seed)

		res := step(t, shed)
		if res.State != StatePaused || res.Next != "B" || res.Reason != "paused before B, as requested" || gateB.calls != 0 {
			t.Fatalf("State, Next, Reason, gate B calls = %q, %q, %q, %d; want paused before B with B's gate unevaluated", res.State, res.Next, res.Reason, gateB.calls)
		}
		if len(res.History) != 1 || res.History[0].Outcome != OutcomeSkipped {
			t.Errorf("History = %+v; want A's skip recorded before the pause", res.History)
		}
		if got := readStatus(t, statusPath, statusLockPath); got.PauseBefore != "" || got.State != StatePaused {
			t.Errorf("persisted State, PauseBefore = %q, %q; want paused with the condition cleared", got.State, got.PauseBefore)
		}

		gateB.holds, gateB.reason = false, "b off now"
		res = step(t, shed)
		if gateB.calls != 1 || b.calls != 0 || res.State != StateDone || res.History[1].Outcome != OutcomeSkipped || res.History[1].Reason != "b off now" {
			t.Errorf("gate B calls, calls B, State, History = %d, %d, %q, %+v; want B's gate evaluated once after the pause and B skipped", gateB.calls, b.calls, res.State, res.History)
		}
	})

	t.Run("a pause_before on an ungated row after a skip halts before the row is called", func(t *testing.T) {
		t.Parallel()
		shed, statusPath, _, statusLockPath := newTestShed(t)
		off := &fakeGate{reason: "a off"}
		b := fixedOutcomeProducer(Done, "")
		shed.Producers = []ProducerDef{
			{Name: "A", Producer: fixedOutcomeProducer(Done, ""), OnDone: "B", EntryGate: off.evaluate},
			{Name: "B", Producer: b},
		}
		seed := commonSeed("A")
		seed.PauseBefore = "B"
		seedStatus(t, statusPath, statusLockPath, seed)

		res := step(t, shed)
		if res.State != StatePaused || res.Next != "B" || b.calls != 0 || len(res.History) != 1 || res.History[0].Outcome != OutcomeSkipped {
			t.Errorf("State, Next, calls B, History = %q, %q, %d, %+v; want paused before B, B uncalled, A's skip recorded", res.State, res.Next, b.calls, res.History)
		}
	})

	t.Run("a pause_after on a skipped row pauses at its on_done row", func(t *testing.T) {
		t.Parallel()
		shed, statusPath, _, statusLockPath := newTestShed(t)
		off := &fakeGate{reason: "off"}
		b := fixedOutcomeProducer(Done, "")
		shed.Producers = []ProducerDef{
			{Name: "A", Producer: fixedOutcomeProducer(Done, ""), OnDone: "B", EntryGate: off.evaluate},
			{Name: "B", Producer: b},
		}
		seed := commonSeed("A")
		seed.PauseAfter = "A"
		seed.PauseAfterBy = "orch"
		seedStatus(t, statusPath, statusLockPath, seed)

		res := step(t, shed)
		wantReason := "paused after A, as requested by orch"
		if res.State != StatePaused || res.Next != "B" || res.Reason != wantReason || b.calls != 0 {
			t.Fatalf("State, Next, Reason, calls B = %q, %q, %q, %d; want paused at B with %q", res.State, res.Next, res.Reason, b.calls, wantReason)
		}
		if got := readStatus(t, statusPath, statusLockPath); got.PauseAfter != "" || got.PauseAfterBy != "" || got.PauseReason != wantReason {
			t.Errorf("persisted PauseAfter, PauseAfterBy, PauseReason = %q, %q, %q; want both conditions cleared and %q", got.PauseAfter, got.PauseAfterBy, got.PauseReason, wantReason)
		}
	})

	t.Run("a run resumed from blocked whose first row is skipped puts the label on the next verdict's entry", func(t *testing.T) {
		t.Parallel()
		shed, statusPath, _, statusLockPath := newTestShed(t)
		shed.ResumedBy = "orch"
		off := &fakeGate{reason: "off"}
		shed.Producers = []ProducerDef{
			{Name: "A", Producer: fixedOutcomeProducer(Done, ""), OnDone: "B", EntryGate: off.evaluate},
			{Name: "B", Producer: fixedOutcomeProducer(Done, "")},
		}
		seed := commonSeed("A")
		seed.State = StateBlocked
		seed.Error = "stuck on A"
		seedStatus(t, statusPath, statusLockPath, seed)

		step(t, shed)
		got := readStatus(t, statusPath, statusLockPath)
		if len(got.History) != 2 || got.History[0].Outcome != OutcomeSkipped || got.History[0].By != "" || got.History[1].Producer != "B" || got.History[1].By != "orch" {
			t.Errorf("persisted History = %+v; want the skipped entry unlabelled and B's entry labelled orch", got.History)
		}
		if got.PendingBy != "" {
			t.Errorf("persisted PendingBy = %q; want it handed to B's entry", got.PendingBy)
		}
	})
}
