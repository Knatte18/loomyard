// innerrun_awaiting_test.go covers the inner-run watch's handling of a child halted in
// StateAwaiting: a hard error naming the hand-off remedy, never "unrecognized status state".

package battenshed

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/shedengine"
)

func TestInnerRun_AwaitingChildIsHandOffError(t *testing.T) {
	clock := &fakeClock{}
	statuses := []statusResult{{
		status: shedengine.Status{State: shedengine.StateAwaiting, Error: "waiting on review", CurrentProducer: "Publish"},
		found:  true,
	}}
	_, spawnCalls, deps := newInnerRunDeps(nil, nil, statuses, clock)

	producer := NewInnerRun("innerrun", "myslug", deps, time.Millisecond, t.TempDir())
	_, _, err := producer.Call(context.Background())
	if err == nil {
		t.Fatal("Call() error = nil; want a hard error for an awaiting child")
	}
	msg := err.Error()
	if strings.Contains(msg, "unrecognized status state") {
		t.Errorf("Call() error = %q; want the awaiting state recognised", msg)
	}
	for _, want := range []string{`"awaiting"`, "waiting on review", "Publish", "lyx loom approve", "lyx loom start"} {
		if !strings.Contains(msg, want) {
			t.Errorf("Call() error = %q; want substring %q", msg, want)
		}
	}
	if strings.Contains(msg, haltedChildRemedy) {
		t.Errorf("Call() error = %q; want the hand-off remedy, not haltedChildRemedy", msg)
	}
	if *spawnCalls != 0 {
		t.Errorf("Spawn calls = %d; want 0 against a halted child", *spawnCalls)
	}
}

func TestInnerRun_OtherHaltedStatesKeepHaltedChildRemedy(t *testing.T) {
	for _, state := range []shedengine.State{shedengine.StateBlocked, shedengine.StatePaused, shedengine.StateFailed} {
		clock := &fakeClock{}
		statuses := []statusResult{{status: shedengine.Status{State: state}, found: true}}
		_, _, deps := newInnerRunDeps(nil, nil, statuses, clock)

		producer := NewInnerRun("innerrun", "myslug", deps, time.Millisecond, t.TempDir())
		_, _, err := producer.Call(context.Background())
		if err == nil || !strings.Contains(err.Error(), haltedChildRemedy) {
			t.Errorf("state %q: error = %v; want it to carry haltedChildRemedy", state, err)
		}
	}
}
