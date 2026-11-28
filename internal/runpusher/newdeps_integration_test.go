// newdeps_integration_test.go drives the wait NewDeps fills.

package runpusher

import (
	"context"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/pushledger"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestNewDeps_WaitWakesOnAPushNowMark(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	deps := NewDeps(pairTargetIn(t), fabricengine.PushTiming{})
	write := func(entry pushledger.Entry) {
		t.Helper()
		err := pushledger.Update(deps.HubPath, deps.Key, time.Now(), func(pushledger.Entry, bool) (pushledger.Entry, bool, error) {
			return entry, true, nil
		})
		if err != nil {
			t.Fatalf("write the ledger entry: %v", err)
		}
	}
	write(pushledger.Entry{FirstOwed: time.Now()})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- deps.Wait(ctx, time.Hour, nil) }()

	write(pushledger.Entry{FirstOwed: time.Now(), IntervalS: 120})
	// Longer than the wake floor, so a wake on the unmarked write would have ended the wait by now.
	select {
	case err := <-done:
		t.Fatalf("Wait() = %v after a write with no mark; want it still waiting", err)
	case <-time.After(markWakeFloor + markWakeFloor/2):
	}

	write(pushledger.Entry{FirstOwed: time.Now(), PushNow: true})
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Wait() = %v after the push-now mark; want nil", err)
		}
	case <-time.After(time.Minute):
		t.Fatal("Wait() did not return after the push-now mark")
	}
}
