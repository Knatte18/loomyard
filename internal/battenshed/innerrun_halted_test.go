// innerrun_halted_test.go covers the inner-run watch's halted arm: a blocked, paused or failed child is a budget-exempt wait that never spawns and Warns once per halt episode.

package battenshed

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

func haltedStatus(state shedengine.State, historyLen int) statusResult {
	return statusResult{
		status: shedengine.Status{
			State:           state,
			Error:           "loom session halted",
			CurrentProducer: "loom-side-producer",
			History:         make([]shedengine.HistoryEntry, historyLen),
		},
		found: true,
	}
}

func TestInnerRun_HaltedStatesWaitExemptWithoutSpawning(t *testing.T) {
	for _, state := range []shedengine.State{shedengine.StatePaused, shedengine.StateBlocked, shedengine.StateFailed} {
		clock := &fakeClock{}
		_, spawnCalls, deps := newInnerRunDeps(nil, nil, []statusResult{haltedStatus(state, 2)}, clock)

		producer := NewInnerRun("innerrun", "myslug", deps, time.Millisecond, t.TempDir(), testGrace)
		outcome, ptr, err := producer.Call(context.Background())
		if err != nil || outcome != shedengine.Stuck || !ptr.BudgetExempt {
			t.Fatalf("state %q: Call() = %v %+v %v; want an exempt Stuck", state, outcome, ptr, err)
		}
		for _, want := range []string{string(state), "loom session halted", "loom-side-producer", "lyx loom start"} {
			if !strings.Contains(ptr.Reason, want) {
				t.Errorf("state %q: Reason = %q; want substring %q", state, ptr.Reason, want)
			}
		}
		if clock.sleepCalls != 1 {
			t.Errorf("state %q: Sleep calls = %d; want 1", state, clock.sleepCalls)
		}
		if *spawnCalls != 0 {
			t.Errorf("state %q: Spawn calls = %d; want 0 against a halted child", state, *spawnCalls)
		}
	}
}

func TestInnerRun_HaltWarnsOncePerEpisode(t *testing.T) {
	var buf bytes.Buffer
	logger.SetOutput(&buf)
	t.Cleanup(func() { logger.SetOutput(os.Stderr) })

	scratchDir := t.TempDir()
	clock := &fakeClock{}
	statuses := []statusResult{haltedStatus(shedengine.StatePaused, 3)}
	_, _, deps := newInnerRunDeps(nil, nil, statuses, clock)
	producer := NewInnerRun("innerrun", "myslug", deps, time.Millisecond, scratchDir, testGrace)

	for i := 0; i < 5; i++ {
		if _, _, err := producer.Call(context.Background()); err != nil {
			t.Fatalf("Call() %d error = %v", i, err)
		}
	}
	const warnText = "inner shed run halted"
	if got := strings.Count(buf.String(), warnText); got != 1 {
		t.Fatalf("Warn count after polls at one history length = %d; want 1\nlog: %s", got, buf.String())
	}
	raw, err := os.ReadFile(haltWarnedFile(scratchDir, "innerrun"))
	if err != nil {
		t.Fatalf("halt-warned marker: %v; want it written", err)
	}
	if got := strings.TrimSpace(string(raw)); got != "3" {
		t.Errorf("halt-warned marker = %q; want %q", got, "3")
	}

	// A resume and re-halt grows the history, which starts a new episode.
	_, _, regrown := newInnerRunDeps(nil, nil, []statusResult{haltedStatus(shedengine.StateBlocked, 5)}, clock)
	regrown.Sleep = deps.Sleep
	producer = NewInnerRun("innerrun", "myslug", regrown, time.Millisecond, scratchDir, testGrace)
	for i := 0; i < 3; i++ {
		if _, _, err := producer.Call(context.Background()); err != nil {
			t.Fatalf("Call() after re-halt %d error = %v", i, err)
		}
	}
	if got := strings.Count(buf.String(), warnText); got != 2 {
		t.Errorf("Warn count after a longer history = %d; want 2", got)
	}
}

func TestInnerRun_ReHaltAfterObservedResumeWarnsAgainAtTheSameHistoryLength(t *testing.T) {
	var buf bytes.Buffer
	logger.SetOutput(&buf)
	t.Cleanup(func() { logger.SetOutput(os.Stderr) })

	// A failed child resumed and failing again on the same producer appends no history entry, so both halts read length 4.
	statuses := []statusResult{
		haltedStatus(shedengine.StateFailed, 4),
		{status: shedengine.Status{State: shedengine.StateRunning, History: make([]shedengine.HistoryEntry, 4)}, found: true},
		haltedStatus(shedengine.StateFailed, 4),
	}
	clock := &fakeClock{}
	_, _, deps := newInnerRunDeps(nil, nil, statuses, clock)
	scratchDir := t.TempDir()
	// The operator's resume ran the child's own driver, so a spawn is already confirmed.
	if err := os.WriteFile(SpawnConfirmedFile(scratchDir, "innerrun"), []byte("spawned\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	producer := NewInnerRun("innerrun", "myslug", deps, time.Millisecond, scratchDir, testGrace)

	for i := 0; i < len(statuses); i++ {
		if _, _, err := producer.Call(context.Background()); err != nil {
			t.Fatalf("Call() %d error = %v", i, err)
		}
	}
	if got := strings.Count(buf.String(), "inner shed run halted"); got != 2 {
		t.Errorf("Warn count across two halt episodes at one history length = %d; want 2\nlog: %s", got, buf.String())
	}
}

func TestInnerRun_PausedThenRunningThenDoneReachesDone(t *testing.T) {
	clock := &fakeClock{}
	statuses := []statusResult{
		haltedStatus(shedengine.StatePaused, 2),
		{status: shedengine.Status{State: shedengine.StateRunning}, found: true},
		{status: shedengine.Status{State: shedengine.StateDone}, found: true},
	}
	_, spawnCalls, deps := newInnerRunDeps(nil, nil, statuses, clock)
	scratchDir := t.TempDir()
	// The operator's resume ran the child's own driver, so a spawn is already confirmed.
	if err := os.WriteFile(SpawnConfirmedFile(scratchDir, "innerrun"), []byte("spawned\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	producer := NewInnerRun("innerrun", "myslug", deps, time.Millisecond, scratchDir, testGrace)

	outcome, ptr, err := producer.Call(context.Background())
	if err != nil || outcome != shedengine.Stuck || !ptr.BudgetExempt {
		t.Fatalf("paused Call() = %v %+v %v; want an exempt Stuck", outcome, ptr, err)
	}
	outcome, ptr, err = producer.Call(context.Background())
	if err != nil || outcome != shedengine.Stuck || ptr.BudgetExempt {
		t.Fatalf("running Call() = %v %+v %v; want a counted Stuck", outcome, ptr, err)
	}
	outcome, _, err = producer.Call(context.Background())
	if err != nil || outcome != shedengine.Done {
		t.Fatalf("done Call() = %v %v; want Done", outcome, err)
	}
	if *spawnCalls != 0 {
		t.Errorf("Spawn calls = %d; want 0", *spawnCalls)
	}
}
