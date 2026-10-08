// innerrun_halted_test.go covers the inner-run watch's halted arm: a blocked, paused or failed child is a budget-exempt wait that never spawns or resumes it, Warns once per halt episode and revives a dead driver strand once per episode.
// The tests that capture log output replace the process-global logger output,
// so they do not run in parallel.

package battenshed

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strconv"
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
		scratchDir := t.TempDir()
		clock := &fakeClock{}
		_, spawnCalls, deps := newInnerRunDeps(t, nil, nil, []statusResult{haltedStatus(state, 2)}, clock)
		clock.watchReason(scratchDir)

		producer := NewInnerRun("innerrun", "myslug", deps, time.Millisecond, scratchDir, testGrace)
		outcome, ptr, err := producer.Call(context.Background())
		if err != nil || outcome != shedengine.Stuck || !ptr.BudgetExempt {
			t.Fatalf("state %q: Call() = %v %+v %v; want an exempt Stuck", state, outcome, ptr, err)
		}
		for _, want := range []string{string(state), "loom session halted", "loom-side-producer", "lyx loom resume", "lyx loom start"} {
			if !strings.Contains(clock.lastReason(), want) {
				t.Errorf("state %q: wait reason = %q; want substring %q", state, clock.lastReason(), want)
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

// TestInnerRun_HaltWarnsOncePerEpisode asserts the checks of one wait at one unchanged status Warn once, and that a longer history in the same state, written while the wait runs, starts a new episode.
func TestInnerRun_HaltWarnsOncePerEpisode(t *testing.T) {
	var buf bytes.Buffer
	logger.SetOutput(&buf)
	t.Cleanup(func() { logger.SetOutput(os.Stderr) })

	const warnText = "inner shed run halted"
	scratchDir := t.TempDir()
	clock := &fakeClock{}
	statuses := []statusResult{haltedStatus(shedengine.StatePaused, 3), haltedStatus(shedengine.StatePaused, 5)}
	_, _, deps := newInnerRunDeps(t, nil, nil, statuses, clock)
	clock.pauseAtSleep = 5
	var warnsBeforeRegrowth int
	clock.onSleep = func(call int) {
		if call == 4 {
			warnsBeforeRegrowth = strings.Count(buf.String(), warnText)
			clock.bumpStatus()
		}
	}
	producer := NewInnerRun("innerrun", "myslug", deps, time.Millisecond, scratchDir, testGrace)

	if _, _, err := producer.Call(context.Background()); err != nil {
		t.Fatalf("Call() error = %v", err)
	}
	if warnsBeforeRegrowth != 1 {
		t.Fatalf("Warn count over three checks at one history length = %d; want 1\nlog: %s", warnsBeforeRegrowth, buf.String())
	}
	if got := strings.Count(buf.String(), warnText); got != 2 {
		t.Errorf("Warn count after the history grew = %d; want 2", got)
	}
	raw, err := os.ReadFile(haltWarnedFile(scratchDir, "innerrun"))
	if err != nil {
		t.Fatalf("halt-warned marker: %v; want it written", err)
	}
	if got := strings.TrimSpace(string(raw)); got != "5" {
		t.Errorf("halt-warned marker = %q; want %q", got, "5")
	}
}

func TestInnerRun_ReHaltAfterObservedResumeWarnsAgainAtTheSameHistoryLength(t *testing.T) {
	var buf bytes.Buffer
	logger.SetOutput(&buf)
	t.Cleanup(func() { logger.SetOutput(os.Stderr) })

	// A failed child resumed and failing again on the same producer appends no history entry, so both halts read length 4.
	failed := haltedStatus(shedengine.StateFailed, 4).status
	running := shedengine.Status{State: shedengine.StateRunning, History: make([]shedengine.HistoryEntry, 4)}
	current := failed
	clock := &fakeClock{}
	_, _, deps := newInnerRunDeps(t, nil, nil, []statusResult{{status: failed, found: true}}, clock)
	deps.ReadStatus = func(string, string) (shedengine.Status, bool, error) { return current, true, nil }
	scratchDir := t.TempDir()
	// The operator's resume ran the child's own driver, so a spawn is already confirmed.
	if err := os.WriteFile(SpawnConfirmedFile(scratchDir, "innerrun"), pidMarker(os.Getpid()), 0o644); err != nil {
		t.Fatal(err)
	}
	// The first Call sees the resume, the second the re-halt, and the third begins in the new episode.
	clock.pauseAtSleep = 3
	clock.onSleep = func(call int) {
		switch call {
		case 1:
			current = running
			clock.bumpStatus()
		case 2:
			current = failed
			clock.bumpStatus()
		}
	}
	producer := NewInnerRun("innerrun", "myslug", deps, time.Millisecond, scratchDir, testGrace)

	for i := 0; i < 3; i++ {
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
	paused := haltedStatus(shedengine.StatePaused, 2).status
	current := paused
	_, spawnCalls, deps := newInnerRunDeps(t, nil, nil, []statusResult{{status: paused, found: true}}, clock)
	deps.ReadStatus = func(string, string) (shedengine.Status, bool, error) { return current, true, nil }
	clock.pauseAtSleep = 0
	clock.onSleep = func(call int) {
		switch call {
		case 1:
			current = shedengine.Status{State: shedengine.StateRunning}
		case 2:
			current = shedengine.Status{State: shedengine.StateDone}
		}
		clock.bumpStatus()
	}
	scratchDir := t.TempDir()
	// The operator's resume ran the child's own driver, so a spawn is already confirmed.
	if err := os.WriteFile(SpawnConfirmedFile(scratchDir, "innerrun"), pidMarker(os.Getpid()), 0o644); err != nil {
		t.Fatal(err)
	}
	producer := NewInnerRun("innerrun", "myslug", deps, time.Millisecond, scratchDir, testGrace)

	outcome, ptr, err := producer.Call(context.Background())
	if err != nil || outcome != shedengine.Stuck || !ptr.BudgetExempt || ptr.Path != "child paused → running" {
		t.Fatalf("paused Call() = %v %+v %v; want an exempt Stuck for the resume", outcome, ptr, err)
	}
	outcome, ptr, err = producer.Call(context.Background())
	if err != nil || outcome != shedengine.Stuck || !ptr.BudgetExempt || ptr.Path != "child running → done" {
		t.Fatalf("running Call() = %v %+v %v; want an exempt Stuck for the finish", outcome, ptr, err)
	}
	outcome, _, err = producer.Call(context.Background())
	if err != nil || outcome != shedengine.Done {
		t.Fatalf("done Call() = %v %v; want Done", outcome, err)
	}
	if *spawnCalls != 0 {
		t.Errorf("Spawn calls = %d; want 0", *spawnCalls)
	}
}

// driverStrandScript answers DriverStrand with the next entry of strands on each call, holding on the last.
func driverStrandScript(strands ...ChildDriverStrand) func(context.Context) (ChildDriverStrand, error) {
	calls := 0
	return func(context.Context) (ChildDriverStrand, error) {
		i := calls
		calls++
		if i >= len(strands) {
			i = len(strands) - 1
		}
		return strands[i], nil
	}
}

// reviveFailedWarn is the log text of a revive that left the driver strand not live.
const reviveFailedWarn = "the revive left the child's driver strand not live"

// TestInnerRun_HaltedRevivesADeadDriverOncePerEpisode asserts a halted child's dead driver strand is revived once per episode per process, that the child is never spawned or resumed,
// that a retiring, live or absent strand is not revived, and that the reason names `lyx loom resume` and, only when no driver can be woken, `lyx loom start` too.
func TestInnerRun_HaltedRevivesADeadDriverOncePerEpisode(t *testing.T) {
	tests := []struct {
		name         string
		strands      []ChildDriverStrand
		reviveErr    error
		marker       string
		warned       string
		wantRevives  int
		wantWarns    int
		wantStart    bool
		wantMarkerAs string
	}{
		{name: "DeadStrandRevivedOnce", strands: []ChildDriverStrand{ChildDriverDead, ChildDriverLive}, wantRevives: 1, wantMarkerAs: strconv.Itoa(os.Getpid()) + " ok"},
		{name: "FailedReviveWarnsOnceAndNamesStart", strands: []ChildDriverStrand{ChildDriverDead}, reviveErr: errors.New("tmux gone"), wantRevives: 1, wantWarns: 1, wantStart: true, wantMarkerAs: strconv.Itoa(os.Getpid()) + " failed"},
		{name: "ReviveLeavingTheStrandDeadWarnsOnceAndNamesStart", strands: []ChildDriverStrand{ChildDriverDead}, wantRevives: 1, wantWarns: 1, wantStart: true, wantMarkerAs: strconv.Itoa(os.Getpid()) + " failed"},
		{name: "AReHaltAtANewHistoryLengthRevivesAgain", strands: []ChildDriverStrand{ChildDriverDead, ChildDriverLive}, marker: strconv.Itoa(os.Getpid()) + " ok\n", warned: "1\n", wantRevives: 1, wantMarkerAs: strconv.Itoa(os.Getpid()) + " ok"},
		{name: "AnEarlierProcessMarkerRevivesAgain", strands: []ChildDriverStrand{ChildDriverDead, ChildDriverLive}, marker: strconv.Itoa(os.Getpid()+1) + " failed\n", wantRevives: 1, wantMarkerAs: strconv.Itoa(os.Getpid()) + " ok"},
		{name: "RetiringStrandIsLeftToStart", strands: []ChildDriverStrand{ChildDriverRetiring}},
		{name: "LiveStrandNeedsNothing", strands: []ChildDriverStrand{ChildDriverLive}},
		{name: "NoStrandHasNothingToReviveAndNamesStart", strands: []ChildDriverStrand{ChildDriverNone}, wantStart: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			logger.SetOutput(&buf)
			t.Cleanup(func() { logger.SetOutput(os.Stderr) })

			scratchDir := t.TempDir()
			if tt.marker != "" {
				if err := os.WriteFile(revivedFile(scratchDir, "innerrun"), []byte(tt.marker), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if tt.warned != "" {
				if err := os.WriteFile(haltWarnedFile(scratchDir, "innerrun"), []byte(tt.warned), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			clock := &fakeClock{}
			_, spawnCalls, deps := newInnerRunDeps(t, nil, nil, []statusResult{haltedStatus(shedengine.StateBlocked, 2)}, clock)
			clock.watchReason(scratchDir)
			revives := 0
			deps.DriverStrand = driverStrandScript(tt.strands...)
			deps.ReviveStrands = func(context.Context) error {
				revives++
				return tt.reviveErr
			}
			producer := NewInnerRun("innerrun", "myslug", deps, time.Millisecond, scratchDir, testGrace)

			for i := 0; i < 2; i++ {
				outcome, got, err := producer.Call(context.Background())
				if err != nil || outcome != shedengine.Stuck || !got.BudgetExempt {
					t.Fatalf("Call() %d = %v %+v %v; want an exempt Stuck", i, outcome, got, err)
				}
			}
			reason := clock.lastReason()

			if revives != tt.wantRevives {
				t.Errorf("revives across two Calls in one episode = %d; want %d", revives, tt.wantRevives)
			}
			if *spawnCalls != 0 {
				t.Errorf("Spawn calls = %d; want 0: the child is never resumed", *spawnCalls)
			}
			if got := strings.Count(buf.String(), reviveFailedWarn); got != tt.wantWarns {
				t.Errorf("revive warn count = %d; want %d\nlog: %s", got, tt.wantWarns, buf.String())
			}
			if !strings.Contains(reason, `"lyx loom resume"`) {
				t.Errorf("wait reason = %q; want it to name lyx loom resume", reason)
			}
			if got := strings.Contains(reason, `"lyx loom start"`); got != tt.wantStart {
				t.Errorf("wait reason = %q; names lyx loom start = %v, want %v", reason, got, tt.wantStart)
			}
			raw, err := os.ReadFile(revivedFile(scratchDir, "innerrun"))
			if tt.wantMarkerAs == "" {
				if err == nil && tt.marker == "" {
					t.Errorf("revived marker %q written; want none", raw)
				}
				return
			}
			if err != nil || strings.TrimSpace(string(raw)) != tt.wantMarkerAs {
				t.Errorf("revived marker = %q, %v; want %q", raw, err, tt.wantMarkerAs)
			}
		})
	}
}

// TestInnerRun_StrandReadErrorSkipsTheReviveWithAWarn asserts a DriverStrand read error is warned about and never fails the row or revives.
func TestInnerRun_StrandReadErrorSkipsTheReviveWithAWarn(t *testing.T) {
	var buf bytes.Buffer
	logger.SetOutput(&buf)
	t.Cleanup(func() { logger.SetOutput(os.Stderr) })

	_, _, deps := newInnerRunDeps(t, nil, nil, []statusResult{haltedStatus(shedengine.StatePaused, 2)}, &fakeClock{})
	deps.DriverStrand = func(context.Context) (ChildDriverStrand, error) {
		return ChildDriverNone, errors.New("directory unreadable")
	}
	deps.ReviveStrands = func(context.Context) error {
		t.Error("revive called after a failed strand read")
		return nil
	}

	outcome, ptr, err := NewInnerRun("innerrun", "myslug", deps, time.Millisecond, t.TempDir(), testGrace).Call(context.Background())

	if err != nil || outcome != shedengine.Stuck || !ptr.BudgetExempt {
		t.Fatalf("Call() = %v %+v %v; want an exempt Stuck", outcome, ptr, err)
	}
	if !strings.Contains(buf.String(), "could not read the child's driver strand") {
		t.Errorf("log lacks the strand-read warn: %s", buf.String())
	}
}

// TestInnerRun_ARunningChildEndsTheReviveEpisode asserts the revived marker is removed once the child is seen running,
// so the next halt revives afresh.
func TestInnerRun_ARunningChildEndsTheReviveEpisode(t *testing.T) {
	t.Parallel()

	scratchDir := t.TempDir()
	statuses := []statusResult{
		haltedStatus(shedengine.StateBlocked, 2),
		{status: shedengine.Status{State: shedengine.StateRunning}, found: true},
	}
	_, _, deps := newInnerRunDeps(t, nil, nil, statuses, &fakeClock{})
	deps.DriverStrand = driverStrandScript(ChildDriverDead, ChildDriverLive)
	deps.ReviveStrands = func(context.Context) error { return nil }
	if err := os.WriteFile(SpawnConfirmedFile(scratchDir, "innerrun"), pidMarker(os.Getpid()), 0o644); err != nil {
		t.Fatal(err)
	}
	producer := NewInnerRun("innerrun", "myslug", deps, time.Millisecond, scratchDir, testGrace)

	if _, _, err := producer.Call(context.Background()); err != nil {
		t.Fatalf("halted Call() error = %v", err)
	}
	if _, err := os.Stat(revivedFile(scratchDir, "innerrun")); err != nil {
		t.Fatalf("revived marker after the halted Call: %v; want it written", err)
	}
	if _, _, err := producer.Call(context.Background()); err != nil {
		t.Fatalf("running Call() error = %v", err)
	}
	if _, err := os.Stat(revivedFile(scratchDir, "innerrun")); !os.IsNotExist(err) {
		t.Errorf("revived marker after the running Call: stat err %v; want it removed", err)
	}
}
