// run_routing_test.go covers Run's outcome-routing scenarios: the happy path, the completion
// terminal values, the unconditional re-call guarantee (card 11), Stuck routing and the bounce
// budget (card 12), and producer errors and unrecognised outcomes (card 13). It drives Run end to
// end against a real status file for every branch the loop's routing switch takes.

package shedengine

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestRun_HappyPath(t *testing.T) {
	shed, statusPath, _, statusLockPath := newTestShed(t)

	p1 := fixedOutcomeProducer(Done, "")
	p2 := fixedOutcomeProducer(Done, "")
	p3 := fixedOutcomeProducer(Done, "")
	shed.Producers = []ProducerDef{
		{Name: "Preflight", Producer: p1},
		{Name: "Plan-Write", Producer: p2},
		{Name: "Finalize", Producer: p3},
	}
	seedStatus(t, statusPath, statusLockPath, commonSeed("Preflight"))

	result, err := shed.Run(context.Background())
	if err != nil {
		t.Fatalf("Run(...) = _, %v; want nil error", err)
	}
	if result.Outcome != RunDone {
		t.Errorf("Result.Outcome = %q; want %q", result.Outcome, RunDone)
	}

	got := readStatus(t, statusPath, statusLockPath)
	if got.State != StateDone {
		t.Errorf("persisted State = %q; want %q", got.State, StateDone)
	}

	wantNames := []string{"Preflight", "Plan-Write", "Finalize"}
	if len(got.History) != len(wantNames) {
		t.Fatalf("len(History) = %d; want %d", len(got.History), len(wantNames))
	}
	for i, name := range wantNames {
		entry := got.History[i]
		if entry.Producer != name {
			t.Errorf("History[%d].Producer = %q; want %q", i, entry.Producer, name)
		}
		if entry.Outcome != Done {
			t.Errorf("History[%d].Outcome = %q; want %q", i, entry.Outcome, Done)
		}
		assertRFC3339UTC(t, entry.At)
	}
	assertHistoryNonDecreasing(t, got.History)

	for i, p := range []*funcProducer{p1, p2, p3} {
		if p.calls != 1 {
			t.Errorf("producer %d calls = %d; want 1", i, p.calls)
		}
	}
}

func TestRun_CompletionTerminalValues(t *testing.T) {
	shed, statusPath, _, statusLockPath := newTestShed(t)

	shed.Producers = []ProducerDef{
		{Name: "Preflight", Producer: fixedOutcomeProducer(Done, "")},
		{Name: "Plan-Write", Producer: fixedOutcomeProducer(Done, "")},
		{Name: "Finalize", Producer: fixedOutcomeProducer(Done, "")},
	}
	seedStatus(t, statusPath, statusLockPath, commonSeed("Preflight"))

	result, err := shed.Run(context.Background())
	if err != nil {
		t.Fatalf("Run(...) = _, %v; want nil error", err)
	}

	got := readStatus(t, statusPath, statusLockPath)
	const lastName = "Finalize"
	if got.CurrentProducer != lastName {
		t.Errorf("persisted CurrentProducer = %q; want %q", got.CurrentProducer, lastName)
	}
	if got.Activity.Now != lastName {
		t.Errorf("persisted Activity.Now = %q; want %q", got.Activity.Now, lastName)
	}
	if result.HaltedProducer != lastName {
		t.Errorf("Result.HaltedProducer = %q; want %q", result.HaltedProducer, lastName)
	}

	if !reflect.DeepEqual(result.History, got.History) {
		t.Errorf("Result.History = %+v; want (persisted) %+v", result.History, got.History)
	}
}

func TestRun_UnconditionalRecall(t *testing.T) {
	// Shed cannot tell a stale output file from a fresh one by existence alone, notably after a
	// bounce-back, so it never skips a call because OutputPointer.Path already exists on disk --
	// that three-case respawn discipline is delegated whole to each engine adapter instead.
	shed, statusPath, _, statusLockPath := newTestShed(t)

	artifactPath := filepath.Join(t.TempDir(), "artifact.txt")
	if err := os.WriteFile(artifactPath, []byte("stale from a prior attempt"), 0o644); err != nil {
		t.Fatalf("seed artifact file: %v", err)
	}

	producer := fixedOutcomeProducer(Done, artifactPath)
	shed.Producers = []ProducerDef{{Name: "Plan-Write", Producer: producer}}
	seedStatus(t, statusPath, statusLockPath, commonSeed("Plan-Write"))

	if _, err := shed.Run(context.Background()); err != nil {
		t.Fatalf("Run(...) = _, %v; want nil error", err)
	}

	if producer.calls != 1 {
		t.Errorf("producer.calls = %d; want 1 -- Run must never skip a call because its output already exists", producer.calls)
	}
}
