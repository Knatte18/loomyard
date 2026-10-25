//go:build tier0

// yield_test.go pins the Yield outcome: a step and a run over a yielding producer leave the run as it was.

package shedengine

import (
	"bytes"
	"context"
	"os"
	"testing"
)

// TestStep_YieldPersistsNothing pins that a producer returning Yield leaves the status file's bytes and modification time unchanged, appends no history and calls no CommitStatus.
// Step reports Yield with the run still running at the same row, and Run ends on RunYielded after the producer's one call.
func TestStep_YieldPersistsNothing(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// drive runs the shed once and fails the test unless the call reports a yield.
		drive func(t *testing.T, shed *Shed)
	}{
		{"Step", func(t *testing.T, shed *Shed) {
			res, err := shed.Step(context.Background())
			if err != nil {
				t.Fatalf("Step() error = %v; want nil", err)
			}
			if res.Outcome != Yield || res.State != StateRunning || res.Next != "A" || res.Producer != "A" {
				t.Errorf("Step() = %+v; want a Yield outcome, state running, producer and next A", res)
			}
		}},
		{"Run", func(t *testing.T, shed *Shed) {
			res, err := shed.Run(context.Background())
			if err != nil {
				t.Fatalf("Run() error = %v; want nil", err)
			}
			if res.Outcome != RunYielded || res.HaltedProducer != "A" {
				t.Errorf("Run() = %+v; want RunYielded halted at A", res)
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			shed, statusPath, _, statusLockPath := newTestShed(t)
			yielding := fixedOutcomeProducer(Yield, "")
			var commitCalls []commitStatusCall
			shed.CommitStatus = recordingCommitStatus(&commitCalls)
			shed.Producers = []ProducerDef{
				{Name: "A", Producer: yielding, OnStuck: "A", OnDone: "B"},
				{Name: "B", Producer: fixedOutcomeProducer(Done, "")},
			}
			seedStatus(t, statusPath, statusLockPath, commonSeed("A"))
			before, err := os.ReadFile(statusPath)
			if err != nil {
				t.Fatal(err)
			}
			beforeInfo, err := os.Stat(statusPath)
			if err != nil {
				t.Fatal(err)
			}

			tt.drive(t, shed)

			after, err := os.ReadFile(statusPath)
			if err != nil {
				t.Fatal(err)
			}
			afterInfo, err := os.Stat(statusPath)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(before, after) || !afterInfo.ModTime().Equal(beforeInfo.ModTime()) {
				t.Errorf("status file changed across a yield: bytes equal %v, mtime before %v after %v", bytes.Equal(before, after), beforeInfo.ModTime(), afterInfo.ModTime())
			}
			if got := readStatus(t, statusPath, statusLockPath); len(got.History) != 0 {
				t.Errorf("History = %+v; want none appended", got.History)
			}
			if len(commitCalls) != 0 {
				t.Errorf("CommitStatus calls = %v; want none", commitCalls)
			}
			if yielding.calls != 1 {
				t.Errorf("producer calls = %d; want 1", yielding.calls)
			}
		})
	}
}
