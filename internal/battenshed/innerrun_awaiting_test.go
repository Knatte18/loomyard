// innerrun_awaiting_test.go covers the inner-run watch's awaiting hand-off, the resume once per approval, the halted-state remedies, and the done arm's wait for the driver strand.

package battenshed

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/state"
)

func awaitingStatus() []statusResult {
	return []statusResult{{
		status: shedengine.Status{State: shedengine.StateAwaiting, Error: "waiting on review", CurrentProducer: "Publish"},
		found:  true,
	}}
}

func doneStatus() []statusResult {
	return []statusResult{{status: shedengine.Status{State: shedengine.StateDone}, found: true}}
}

func TestInnerRun_AwaitingWithoutApprovalWaitsExempt(t *testing.T) {
	clock := &fakeClock{}
	_, spawnCalls, deps := newInnerRunDeps(nil, nil, awaitingStatus(), clock)

	producer := NewInnerRun("innerrun", "myslug", deps, time.Millisecond, t.TempDir(), testGrace)
	outcome, ptr, err := producer.Call(context.Background())
	if err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}
	if outcome != shedengine.Stuck || !ptr.BudgetExempt {
		t.Errorf("Call() = %v exempt=%v; want an exempt Stuck", outcome, ptr.BudgetExempt)
	}
	for _, want := range []string{"waiting on review", "lyx loom approve"} {
		if !strings.Contains(ptr.Reason, want) {
			t.Errorf("Reason = %q; want substring %q", ptr.Reason, want)
		}
	}
	if clock.sleepCalls != 1 {
		t.Errorf("Sleep calls = %d; want 1", clock.sleepCalls)
	}
	if *spawnCalls != 0 {
		t.Errorf("Spawn calls = %d; want 0 without an approval", *spawnCalls)
	}
}

func TestInnerRun_AwaitingResumesOncePerApproval(t *testing.T) {
	scratchDir := t.TempDir()
	clock := &fakeClock{}
	_, spawnCalls, deps := newInnerRunDeps(nil, nil, awaitingStatus(), clock)
	approval := ChildDecision{Kind: DecisionApprove, At: "2026-01-01T10:00:00Z", HeadSHA: "abc"}
	deps.ReadDecision = func() (ChildDecision, bool, error) { return approval, true, nil }
	producer := NewInnerRun("innerrun", "myslug", deps, time.Millisecond, scratchDir, testGrace)

	outcome, ptr, err := producer.Call(context.Background())
	if err != nil || outcome != shedengine.Stuck || !ptr.BudgetExempt {
		t.Fatalf("first Call() = %v %+v %v; want an exempt Stuck", outcome, ptr, err)
	}
	if *spawnCalls != 1 {
		t.Fatalf("Spawn calls = %d; want 1", *spawnCalls)
	}
	if !strings.Contains(ptr.Reason, "resumed") {
		t.Errorf("Reason = %q; want it to say the child was resumed", ptr.Reason)
	}
	if _, err := os.Stat(decisionActedFile(scratchDir, "innerrun")); err != nil {
		t.Errorf("decision-acted marker: %v; want it written", err)
	}

	outcome, ptr, err = producer.Call(context.Background())
	if err != nil || outcome != shedengine.Stuck || !ptr.BudgetExempt {
		t.Fatalf("second Call() = %v %+v %v; want an exempt Stuck", outcome, ptr, err)
	}
	if *spawnCalls != 1 {
		t.Errorf("Spawn calls = %d after the same approval; want still 1", *spawnCalls)
	}
	for _, want := range []string{"already acted on", approval.At, "lyx loom approve"} {
		if !strings.Contains(ptr.Reason, want) {
			t.Errorf("Reason = %q; want substring %q", ptr.Reason, want)
		}
	}

	approval = ChildDecision{Kind: DecisionApprove, At: "2026-01-01T11:00:00Z", HeadSHA: "def"}
	if _, _, err := producer.Call(context.Background()); err != nil {
		t.Fatalf("third Call() error = %v", err)
	}
	if *spawnCalls != 2 {
		t.Errorf("Spawn calls = %d after a new approval; want 2", *spawnCalls)
	}
}

func TestInnerRun_AwaitingResumesOnARejectionOnceAndAgainOnASameSecondApproval(t *testing.T) {
	scratchDir := t.TempDir()
	clock := &fakeClock{}
	_, spawnCalls, deps := newInnerRunDeps(nil, nil, awaitingStatus(), clock)
	decision := ChildDecision{Kind: DecisionReject, At: "2026-01-01T10:00:00Z", HeadSHA: "abc"}
	deps.ReadDecision = func() (ChildDecision, bool, error) { return decision, true, nil }
	producer := NewInnerRun("innerrun", "myslug", deps, time.Millisecond, scratchDir, testGrace)

	if outcome, ptr, err := producer.Call(context.Background()); err != nil || outcome != shedengine.Stuck || !ptr.BudgetExempt {
		t.Fatalf("first Call() = %v %+v %v; want an exempt Stuck", outcome, ptr, err)
	}
	if *spawnCalls != 1 {
		t.Fatalf("Spawn calls = %d after a rejection; want 1", *spawnCalls)
	}
	_, ptr, err := producer.Call(context.Background())
	if err != nil {
		t.Fatalf("second Call() error = %v", err)
	}
	if *spawnCalls != 1 {
		t.Errorf("Spawn calls = %d after the same rejection; want still 1", *spawnCalls)
	}
	for _, want := range []string{"already acted on", "reject", decision.At} {
		if !strings.Contains(ptr.Reason, want) {
			t.Errorf("Reason = %q; want substring %q", ptr.Reason, want)
		}
	}

	decision.Kind = DecisionApprove
	if _, _, err := producer.Call(context.Background()); err != nil {
		t.Fatalf("third Call() error = %v", err)
	}
	if *spawnCalls != 2 {
		t.Errorf("Spawn calls = %d after an approval at the same head and second; want 2", *spawnCalls)
	}
}

func TestInnerRun_AwaitingSpawnErrorWritesNoMarkerAndRetries(t *testing.T) {
	scratchDir := t.TempDir()
	clock := &fakeClock{}
	spawnErr := errors.New("bootstrap exited 1")
	_, spawnCalls, deps := newInnerRunDeps(nil, nil, awaitingStatus(), clock)
	deps.ReadDecision = func() (ChildDecision, bool, error) {
		return ChildDecision{Kind: DecisionApprove, At: "2026-01-01T10:00:00Z", HeadSHA: "abc"}, true, nil
	}
	failing := deps
	failing.Spawn = func(ctx context.Context) error {
		*spawnCalls++
		return spawnErr
	}

	_, _, err := NewInnerRun("innerrun", "myslug", failing, time.Millisecond, scratchDir, testGrace).Call(context.Background())
	if !errors.Is(err, spawnErr) {
		t.Fatalf("first Call() error = %v; want it to wrap %v", err, spawnErr)
	}
	if _, statErr := os.Stat(decisionActedFile(scratchDir, "innerrun")); !os.IsNotExist(statErr) {
		t.Errorf("decision-acted marker stat = %v; want absent after a failed spawn", statErr)
	}
	outcome, _, err := NewInnerRun("innerrun", "myslug", deps, time.Millisecond, scratchDir, testGrace).Call(context.Background())
	if err != nil || outcome != shedengine.Stuck {
		t.Fatalf("second Call() = %v %v; want an exempt Stuck", outcome, err)
	}
	if *spawnCalls != 2 {
		t.Errorf("Spawn calls = %d; want 2 -- the next Call retries the spawn", *spawnCalls)
	}
}

func TestInnerRun_AwaitingNotParkedRetriesWithoutMarkerThenResumes(t *testing.T) {
	scratchDir := t.TempDir()
	clock := &fakeClock{}
	_, _, deps := newInnerRunDeps(nil, nil, awaitingStatus(), clock)
	deps.ReadDecision = func() (ChildDecision, bool, error) {
		return ChildDecision{Kind: DecisionApprove, At: "2026-01-01T10:00:00Z", HeadSHA: "abc"}, true, nil
	}
	// The child's driver parks after the second spawn attempt: the fake refuses until then.
	spawnCalls := 0
	deps.Spawn = func(ctx context.Context) error {
		spawnCalls++
		if spawnCalls <= 2 {
			return fmt.Errorf("%w: exit status 1", ErrChildNotParked)
		}
		return nil
	}
	producer := NewInnerRun("innerrun", "myslug", deps, time.Millisecond, scratchDir, testGrace)

	for i := 1; i <= 2; i++ {
		outcome, ptr, err := producer.Call(context.Background())
		if err != nil || outcome != shedengine.Stuck || !ptr.BudgetExempt {
			t.Fatalf("Call() %d = %v %+v %v; want an exempt Stuck", i, outcome, ptr, err)
		}
		if !strings.Contains(ptr.Reason, "not parked") {
			t.Errorf("Call() %d Reason = %q; want it to name the unparked driver", i, ptr.Reason)
		}
		if _, statErr := os.Stat(decisionActedFile(scratchDir, "innerrun")); !os.IsNotExist(statErr) {
			t.Fatalf("Call() %d: decision-acted marker stat = %v; want absent while the driver has not parked", i, statErr)
		}
	}

	outcome, ptr, err := producer.Call(context.Background())
	if err != nil || outcome != shedengine.Stuck || !strings.Contains(ptr.Reason, "resumed") {
		t.Fatalf("third Call() = %v %+v %v; want the resume", outcome, ptr, err)
	}
	if spawnCalls != 3 {
		t.Errorf("Spawn calls = %d; want 3", spawnCalls)
	}
	if _, statErr := os.Stat(decisionActedFile(scratchDir, "innerrun")); statErr != nil {
		t.Errorf("decision-acted marker: %v; want it written once the resume succeeded", statErr)
	}
}

func TestInnerRun_ReadDecisionErrorIsHardError(t *testing.T) {
	clock := &fakeClock{}
	readErr := errors.New("decision unreadable")
	_, _, deps := newInnerRunDeps(nil, nil, awaitingStatus(), clock)
	deps.ReadDecision = func() (ChildDecision, bool, error) { return ChildDecision{}, false, readErr }

	_, _, err := NewInnerRun("innerrun", "myslug", deps, time.Millisecond, t.TempDir(), testGrace).Call(context.Background())
	if !errors.Is(err, readErr) {
		t.Errorf("Call() error = %v; want it to wrap %v", err, readErr)
	}
}

func TestInnerRun_OtherHaltedStatesKeepHaltedChildRemedy(t *testing.T) {
	for _, state := range []shedengine.State{shedengine.StateBlocked, shedengine.StatePaused, shedengine.StateFailed} {
		clock := &fakeClock{}
		statuses := []statusResult{{status: shedengine.Status{State: state}, found: true}}
		_, _, deps := newInnerRunDeps(nil, nil, statuses, clock)

		producer := NewInnerRun("innerrun", "myslug", deps, time.Millisecond, t.TempDir(), testGrace)
		_, _, err := producer.Call(context.Background())
		if err == nil || !strings.Contains(err.Error(), haltedChildRemedy) {
			t.Errorf("state %q: error = %v; want it to carry haltedChildRemedy", state, err)
		}
	}
}

func TestInnerRun_DoneWaitsForLiveDriverThenFinishes(t *testing.T) {
	scratchDir := t.TempDir()
	clock := &fakeClock{}
	_, _, deps := newInnerRunDeps(nil, nil, doneStatus(), clock)
	alive := true
	deps.DriverAlive = func(ctx context.Context) (bool, error) { return alive, nil }
	producer := NewInnerRun("innerrun", "myslug", deps, time.Millisecond, scratchDir, testGrace)

	outcome, ptr, err := producer.Call(context.Background())
	if err != nil || outcome != shedengine.Stuck || !ptr.BudgetExempt {
		t.Fatalf("Call() = %v %+v %v; want an exempt Stuck while the driver is live", outcome, ptr, err)
	}
	if !strings.Contains(ptr.Reason, "stop report") {
		t.Errorf("Reason = %q; want it to name the wait for the driver's stop report", ptr.Reason)
	}
	if _, err := os.Stat(doneSeenFile(scratchDir, "innerrun")); err != nil {
		t.Errorf("done-seen marker: %v; want it written on first sight", err)
	}

	alive = false
	outcome, _, err = producer.Call(context.Background())
	if err != nil || outcome != shedengine.Done {
		t.Fatalf("Call() = %v %v; want Done once the driver is gone", outcome, err)
	}
	if _, err := os.Stat(doneSeenFile(scratchDir, "innerrun")); !os.IsNotExist(err) {
		t.Errorf("done-seen marker stat = %v; want it removed on Done", err)
	}
}

func TestInnerRun_DoneLivenessErrorIsTreatedAsLive(t *testing.T) {
	clock := &fakeClock{}
	_, _, deps := newInnerRunDeps(nil, nil, doneStatus(), clock)
	deps.DriverAlive = func(ctx context.Context) (bool, error) { return false, errors.New("reed unreadable") }

	outcome, _, err := NewInnerRun("innerrun", "myslug", deps, time.Millisecond, t.TempDir(), testGrace).Call(context.Background())
	if err != nil || outcome != shedengine.Stuck {
		t.Errorf("Call() = %v %v; want an exempt Stuck, the error read as live", outcome, err)
	}
}

func TestInnerRun_DoneFinishesPastGraceWithLiveDriverAndWarns(t *testing.T) {
	var buf bytes.Buffer
	logger.SetOutput(&buf)
	t.Cleanup(func() { logger.SetOutput(os.Stderr) })

	scratchDir := t.TempDir()
	clock := &fakeClock{}
	_, _, deps := newInnerRunDeps(nil, nil, doneStatus(), clock)
	deps.DriverAlive = func(ctx context.Context) (bool, error) { return true, nil }
	producer := NewInnerRun("innerrun", "myslug", deps, time.Millisecond, scratchDir, testGrace)

	if outcome, _, _ := producer.Call(context.Background()); outcome != shedengine.Stuck {
		t.Fatalf("first Call() outcome = %v; want Stuck", outcome)
	}
	clock.advance(testGrace)
	outcome, _, err := producer.Call(context.Background())
	if err != nil || outcome != shedengine.Done {
		t.Fatalf("Call() = %v %v; want Done past the grace", outcome, err)
	}
	if !strings.Contains(buf.String(), "myslug") || !strings.Contains(buf.String(), "exit grace") {
		t.Errorf("log = %q; want a warning naming the slug and the grace", buf.String())
	}
}

func TestInnerRun_UnparseableDoneSeenMarkerRestartsTheGrace(t *testing.T) {
	scratchDir := t.TempDir()
	clock := &fakeClock{}
	_, _, deps := newInnerRunDeps(nil, nil, doneStatus(), clock)
	deps.DriverAlive = func(ctx context.Context) (bool, error) { return true, nil }
	markerPath := doneSeenFile(scratchDir, "innerrun")
	if err := os.WriteFile(markerPath, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	producer := NewInnerRun("innerrun", "myslug", deps, time.Millisecond, scratchDir, testGrace)

	outcome, _, err := producer.Call(context.Background())
	if err != nil || outcome != shedengine.Stuck {
		t.Fatalf("Call() over an empty marker = %v %v; want an exempt Stuck, not a hard error", outcome, err)
	}
	raw, err := os.ReadFile(markerPath)
	if err != nil {
		t.Fatalf("read done-seen marker: %v", err)
	}
	if got, want := strings.TrimSpace(string(raw)), clock.Now().Format(time.RFC3339); got != want {
		t.Errorf("done-seen marker = %q; want it rewritten with the current time %q", got, want)
	}

	clock.advance(testGrace)
	if outcome, _, err := producer.Call(context.Background()); err != nil || outcome != shedengine.Done {
		t.Errorf("Call() a full grace later = %v %v; want Done", outcome, err)
	}
}

func TestInnerRun_StaleDoneSeenMarkerIsClearedWhileRunning(t *testing.T) {
	scratchDir := t.TempDir()
	clock := &fakeClock{}
	statuses := []statusResult{{status: shedengine.Status{State: shedengine.StateRunning}, found: true}}
	_, _, deps := newInnerRunDeps(nil, nil, statuses, clock)
	if err := os.WriteFile(SpawnConfirmedFile(scratchDir, "innerrun"), []byte("spawned\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	stale := clock.Now().Add(-2 * testGrace).Format(time.RFC3339)
	if err := os.WriteFile(doneSeenFile(scratchDir, "innerrun"), []byte(stale+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, _, err := NewInnerRun("innerrun", "myslug", deps, time.Millisecond, scratchDir, testGrace).Call(context.Background()); err != nil {
		t.Fatalf("Call() error = %v", err)
	}
	if _, err := os.Stat(doneSeenFile(scratchDir, "innerrun")); !os.IsNotExist(err) {
		t.Fatalf("done-seen marker stat = %v; want it removed by a running Call", err)
	}

	// A later done Call waits a full grace from its own first sight.
	_, _, doneDeps := newInnerRunDeps(nil, nil, doneStatus(), clock)
	doneDeps.DriverAlive = func(ctx context.Context) (bool, error) { return true, nil }
	outcome, _, err := NewInnerRun("innerrun", "myslug", doneDeps, time.Millisecond, scratchDir, testGrace).Call(context.Background())
	if err != nil || outcome != shedengine.Stuck {
		t.Errorf("done Call() = %v %v; want Stuck, the stale marker not shortening the wait", outcome, err)
	}
}

// newRunShed drives producer as a self-routing "Run-Shed" row through a real shedengine.Shed whose persisted history already carries counted Stuck entries, with the same bounce budget, so the row's budget is fully spent before the first Step.
func newRunShed(t *testing.T, producer shedengine.ShedProducer, counted int) *shedengine.Shed {
	t.Helper()
	root := t.TempDir()
	statusPath := filepath.Join(root, "durable", "status.json")
	statusLockPath := filepath.Join(root, "ephemeral", "status.json.lock")
	if err := os.MkdirAll(filepath.Dir(statusPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(statusLockPath), 0o755); err != nil {
		t.Fatal(err)
	}
	history := make([]shedengine.HistoryEntry, counted)
	for i := range history {
		history[i] = shedengine.HistoryEntry{Producer: "Run-Shed", Outcome: shedengine.Stuck, At: "2026-01-01T00:00:00Z"}
	}
	seed := shedengine.Status{CurrentProducer: "Run-Shed", State: shedengine.StateRunning, History: history}
	if err := state.WriteJSON(statusPath, statusLockPath, seed); err != nil {
		t.Fatal(err)
	}
	return &shedengine.Shed{
		Producers: []shedengine.ProducerDef{{
			Name:       "Run-Shed",
			Producer:   producer,
			OnStuck:    "Run-Shed",
			MaxBounces: counted,
		}},
		StatusPath:     statusPath,
		LockPath:       filepath.Join(root, "ephemeral", "run.lock"),
		StatusLockPath: statusLockPath,
	}
}

func TestInnerRun_DoneWaitIsNeitherCountedNorBlockedByTheRunningBudget(t *testing.T) {
	const counted = 3
	clock := &fakeClock{}
	_, _, deps := newInnerRunDeps(nil, nil, doneStatus(), clock)
	deps.Sleep = func(ctx context.Context, d time.Duration) { clock.advance(d) }
	deps.DriverAlive = func(ctx context.Context) (bool, error) { return true, nil }
	producer := NewInnerRun("innerrun", "myslug", deps, time.Minute, t.TempDir(), 5*time.Minute)
	shed := newRunShed(t, producer, counted)

	var last shedengine.StepResult
	for i := 0; i < 20; i++ {
		res, err := shed.Step(context.Background())
		if err != nil {
			t.Fatalf("Step %d error = %v", i, err)
		}
		if res.State == shedengine.StateBlocked {
			t.Fatalf("Step %d blocked on the running budget: %+v", i, res)
		}
		last = res
		if res.Outcome == shedengine.Done {
			break
		}
	}
	if last.Outcome != shedengine.Done {
		t.Fatalf("last outcome = %q; want Done after the full grace wait", last.Outcome)
	}
	if clock.now.Sub(time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)) < 5*time.Minute {
		t.Errorf("clock advanced %s; want the full grace wait", clock.now)
	}
}

func TestInnerRun_AwaitingWaitIsNotBlockedByTheRunningBudget(t *testing.T) {
	const counted = 3
	clock := &fakeClock{}
	_, _, deps := newInnerRunDeps(nil, nil, awaitingStatus(), clock)
	producer := NewInnerRun("innerrun", "myslug", deps, time.Minute, t.TempDir(), testGrace)
	shed := newRunShed(t, producer, counted)

	for i := 0; i < counted+2; i++ {
		res, err := shed.Step(context.Background())
		if err != nil {
			t.Fatalf("Step %d error = %v", i, err)
		}
		if res.State != shedengine.StateRunning || res.Next != "Run-Shed" {
			t.Fatalf("Step %d = state %q next %q; want running routed back to Run-Shed", i, res.State, res.Next)
		}
	}
}
