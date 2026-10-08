// innerrun_awaiting_test.go covers the inner-run watch's awaiting hand-off, the resume once per decision, and the done arm's wait for the driver strand.

package battenshed

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/state"
	"github.com/Knatte18/loomyard/internal/testkit/shedfake"
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

// TestInnerRun_AwaitingWithoutApprovalWaitsExempt asserts an awaiting child with no decision on disk is an exempt wait that names the review and the approve command, checks once before the test's pause ends it and spawns nothing.
//
//testtiming:keep pins the no-decision wait -- its reason, one check and no spawn -- which the decision-bearing awaiting tests never reach
func TestInnerRun_AwaitingWithoutApprovalWaitsExempt(t *testing.T) {
	scratchDir := t.TempDir()
	clock := &fakeClock{}
	_, spawnCalls, deps := newInnerRunDeps(t, nil, nil, awaitingStatus(), clock)
	clock.watchReason(scratchDir)

	producer := NewInnerRun("innerrun", "myslug", deps, time.Millisecond, scratchDir, testGrace)
	outcome, ptr := shedfake.CallOK(t, producer)
	if outcome != shedengine.Stuck || !ptr.BudgetExempt {
		t.Errorf("Call() = %v exempt=%v; want an exempt Stuck", outcome, ptr.BudgetExempt)
	}
	for _, want := range []string{"waiting on review", "lyx loom approve", "lyx loom resume"} {
		if !strings.Contains(clock.lastReason(), want) {
			t.Errorf("wait reason = %q; want substring %q", clock.lastReason(), want)
		}
	}
	if clock.sleepCalls != 1 {
		t.Errorf("Sleep calls = %d; want 1", clock.sleepCalls)
	}
	if *spawnCalls != 0 {
		t.Errorf("Spawn calls = %d; want 0 without an approval", *spawnCalls)
	}
}

// TestInnerRun_AwaitingRevivesADeadDriverAndKeepsItsHints asserts an awaiting child's dead driver strand is revived once, and that the hand-off hint keeps naming both hand-offs.
func TestInnerRun_AwaitingRevivesADeadDriverAndKeepsItsHints(t *testing.T) {
	t.Parallel()

	scratchDir := t.TempDir()
	clock := &fakeClock{}
	_, spawnCalls, deps := newInnerRunDeps(t, nil, nil, awaitingStatus(), clock)
	clock.watchReason(scratchDir)
	revives := 0
	deps.DriverStrand = driverStrandScript(ChildDriverDead, ChildDriverLive)
	deps.ReviveStrands = func(context.Context) error {
		revives++
		return nil
	}
	producer := NewInnerRun("innerrun", "myslug", deps, time.Millisecond, scratchDir, testGrace)

	for i := 0; i < 2; i++ {
		shedfake.CallOK(t, producer)
	}

	if revives != 1 {
		t.Errorf("revives across two Calls = %d; want 1", revives)
	}
	if *spawnCalls != 0 {
		t.Errorf("Spawn calls = %d; want 0 without a decision", *spawnCalls)
	}
	if reason := clock.lastReason(); !strings.Contains(reason, "lyx loom approve") || !strings.Contains(reason, "lyx loom resume") {
		t.Errorf("wait reason = %q; want the approve and the circling-then-resume hand-offs", reason)
	}
}

// awaitingStatusWithHistory is an awaiting child whose history holds n entries.
func awaitingStatusWithHistory(n int) statusResult {
	return statusResult{
		status: shedengine.Status{
			State:           shedengine.StateAwaiting,
			Error:           "waiting on review",
			CurrentProducer: "Publish",
			History:         make([]shedengine.HistoryEntry, n),
		},
		found: true,
	}
}

// TestInnerRun_AwaitingResumesOncePerApproval walks an approval through the wait: it is acted on within one check of appearing, the call goes on waiting until the child leaves awaiting, and the same approval is never acted on twice while a new one is.
func TestInnerRun_AwaitingResumesOncePerApproval(t *testing.T) {
	scratchDir := t.TempDir()
	clock := &fakeClock{}
	current := awaitingStatusWithHistory(4).status
	_, spawnCalls, deps := newInnerRunDeps(t, nil, nil, []statusResult{awaitingStatusWithHistory(4)}, clock)
	deps.ReadStatus = func(string, string) (shedengine.Status, bool, error) { return current, true, nil }
	clock.watchReason(scratchDir)
	approval := ChildDecision{Kind: DecisionApprove, At: "2026-01-01T10:00:00Z", HeadSHA: "abc"}
	decided := false
	deps.ReadDecision = func() (ChildDecision, bool, error) { return approval, decided, nil }
	producer := NewInnerRun("innerrun", "myslug", deps, time.Millisecond, scratchDir, testGrace)

	// The approval appears before check 1, is acted on at check 1, and the child leaves awaiting at check 3.
	var spawnsBeforeCheck2 int
	clock.pauseAtSleep = 0
	clock.onSleep = func(call int) {
		switch call {
		case 1:
			decided = true
		case 2:
			spawnsBeforeCheck2 = *spawnCalls
		case 3:
			current = shedengine.Status{State: shedengine.StateRunning, CurrentProducer: "Publish"}
			clock.bumpStatus()
		}
	}
	outcome, ptr, err := producer.Call(context.Background())
	if err != nil || outcome != shedengine.Stuck || !ptr.BudgetExempt {
		t.Fatalf("first Call() = %v %+v %v; want an exempt Stuck", outcome, ptr, err)
	}
	if spawnsBeforeCheck2 != 1 || *spawnCalls != 1 {
		t.Fatalf("Spawn calls = %d after check 1 and %d at the end; want the approval acted on once, within one check", spawnsBeforeCheck2, *spawnCalls)
	}
	if want := "child awaiting → running at Publish"; ptr.Path != want {
		t.Errorf("Path = %q; want the call to return on the child leaving awaiting: %q", ptr.Path, want)
	}
	if !strings.Contains(clock.reasons[1], "resume was delivered") {
		t.Errorf("wait reason after the resume = %q; want it to say the resume was delivered", clock.reasons[1])
	}
	raw, err := os.ReadFile(decisionActedFile(scratchDir, "innerrun"))
	if err != nil {
		t.Fatalf("decision-acted marker: %v; want it written", err)
	}
	if want := decisionIdentity(approval) + "4\n"; string(raw) != want {
		t.Errorf("decision-acted marker = %q; want %q", raw, want)
	}

	// The child is awaiting again with the history unchanged since the resume.
	current = awaitingStatusWithHistory(4).status
	clock.onSleep = nil
	clock.pauseAtSleep = clock.sleepCalls + 1
	if _, _, err := producer.Call(context.Background()); err != nil {
		t.Fatalf("second Call() error = %v", err)
	}
	if *spawnCalls != 1 {
		t.Errorf("Spawn calls = %d after the same approval; want still 1", *spawnCalls)
	}
	reason := clock.lastReason()
	for _, want := range []string{"has not re-stepped yet", approval.At, "lyx loom start", "pane"} {
		if !strings.Contains(reason, want) {
			t.Errorf("wait reason = %q; want substring %q", reason, want)
		}
	}
	if strings.Contains(reason, "lyx loom approve") {
		t.Errorf("wait reason = %q; want no re-approve hint while the resume is pending", reason)
	}

	approval = ChildDecision{Kind: DecisionApprove, At: "2026-01-01T11:00:00Z", HeadSHA: "def"}
	clock.pauseAtSleep = clock.sleepCalls + 1
	if _, _, err := producer.Call(context.Background()); err != nil {
		t.Fatalf("third Call() error = %v", err)
	}
	if *spawnCalls != 2 {
		t.Errorf("Spawn calls = %d after a new approval; want 2 even with the recorded history length unchanged", *spawnCalls)
	}
}

// TestInnerRun_AwaitingAgainAfterResumeGetsReapproveHint asserts a child that awaits again after a resume, seen by the wait as a longer history in the same state, gets the decide-again hint and no second resume.
func TestInnerRun_AwaitingAgainAfterResumeGetsReapproveHint(t *testing.T) {
	scratchDir := t.TempDir()
	clock := &fakeClock{}
	statuses := []statusResult{awaitingStatusWithHistory(4), awaitingStatusWithHistory(5)}
	_, spawnCalls, deps := newInnerRunDeps(t, nil, nil, statuses, clock)
	clock.watchReason(scratchDir)
	clock.pauseAtSleep = 2
	clock.onSleep = func(call int) {
		if call == 1 {
			clock.bumpStatus()
		}
	}
	approval := ChildDecision{Kind: DecisionApprove, At: "2026-01-01T10:00:00Z", HeadSHA: "abc"}
	deps.ReadDecision = func() (ChildDecision, bool, error) { return approval, true, nil }
	producer := NewInnerRun("innerrun", "myslug", deps, time.Millisecond, scratchDir, testGrace)

	outcome, ptr, err := producer.Call(context.Background())
	if err != nil || outcome != shedengine.Stuck || !ptr.BudgetExempt {
		t.Fatalf("Call() = %v %+v %v; want an exempt Stuck", outcome, ptr, err)
	}
	if *spawnCalls != 1 {
		t.Errorf("Spawn calls = %d; want still 1", *spawnCalls)
	}
	for _, want := range []string{"already acted on", approval.At, "lyx loom approve", "lyx loom resume"} {
		if !strings.Contains(clock.reasons[0], want) {
			t.Errorf("wait reason at the check that saw the longer history = %q; want substring %q", clock.reasons[0], want)
		}
	}
}

func TestInnerRun_AwaitingOldLayoutMarkerGetsReapproveHint(t *testing.T) {
	scratchDir := t.TempDir()
	clock := &fakeClock{}
	_, spawnCalls, deps := newInnerRunDeps(t, nil, nil, []statusResult{awaitingStatusWithHistory(4)}, clock)
	clock.watchReason(scratchDir)
	approval := ChildDecision{Kind: DecisionApprove, At: "2026-01-01T10:00:00Z", HeadSHA: "abc"}
	deps.ReadDecision = func() (ChildDecision, bool, error) { return approval, true, nil }
	if err := os.WriteFile(decisionActedFile(scratchDir, "innerrun"), []byte(decisionIdentity(approval)), 0o644); err != nil {
		t.Fatal(err)
	}

	outcome, ptr, err := NewInnerRun("innerrun", "myslug", deps, time.Millisecond, scratchDir, testGrace).Call(context.Background())
	if err != nil || outcome != shedengine.Stuck || !ptr.BudgetExempt {
		t.Fatalf("Call() = %v %+v %v; want an exempt Stuck", outcome, ptr, err)
	}
	if *spawnCalls != 0 {
		t.Errorf("Spawn calls = %d; want 0", *spawnCalls)
	}
	if reason := clock.lastReason(); !strings.Contains(reason, "lyx loom approve") || !strings.Contains(reason, "lyx loom resume") {
		t.Errorf("wait reason = %q; want the decide-again hint naming both hand-offs", reason)
	}
}

func TestInnerRun_AwaitingResumesOnARejectionOnceAndAgainOnASameSecondApproval(t *testing.T) {
	scratchDir := t.TempDir()
	clock := &fakeClock{}
	statuses := []statusResult{awaitingStatusWithHistory(4), awaitingStatusWithHistory(5)}
	_, spawnCalls, deps := newInnerRunDeps(t, nil, nil, statuses, clock)
	clock.watchReason(scratchDir)
	decision := ChildDecision{Kind: DecisionReject, At: "2026-01-01T10:00:00Z", HeadSHA: "abc"}
	deps.ReadDecision = func() (ChildDecision, bool, error) { return decision, true, nil }
	producer := NewInnerRun("innerrun", "myslug", deps, time.Millisecond, scratchDir, testGrace)

	// The rejection is acted on at entry; the child awaits again at check 1; an approval at the same head and second is then a new decision at check 2.
	clock.pauseAtSleep = 2
	clock.onSleep = func(call int) {
		switch call {
		case 1:
			clock.bumpStatus()
		case 2:
			decision.Kind = DecisionApprove
		}
	}
	if outcome, ptr, err := producer.Call(context.Background()); err != nil || outcome != shedengine.Stuck || !ptr.BudgetExempt {
		t.Fatalf("Call() = %v %+v %v; want an exempt Stuck", outcome, ptr, err)
	}
	for _, want := range []string{"already acted on", "reject", decision.At, "lyx loom resume"} {
		if !strings.Contains(clock.reasons[0], want) {
			t.Errorf("wait reason after the same rejection = %q; want substring %q", clock.reasons[0], want)
		}
	}
	if *spawnCalls != 2 {
		t.Errorf("Spawn calls = %d; want 1 for the rejection and 1 more for the approval at the same head and second", *spawnCalls)
	}
}

func TestInnerRun_AwaitingSpawnErrorWritesNoMarkerAndRetries(t *testing.T) {
	scratchDir := t.TempDir()
	clock := &fakeClock{}
	spawnErr := errors.New("bootstrap exited 1")
	_, spawnCalls, deps := newInnerRunDeps(t, nil, nil, awaitingStatus(), clock)
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

// TestInnerRun_AwaitingNotParkedRetriesWithoutMarkerThenResumes asserts a resume the child's driver refuses as not parked yet leaves the decision unacted, is retried inside the wait at most once per notice probe, and is acted on once the driver parks.
func TestInnerRun_AwaitingNotParkedRetriesWithoutMarkerThenResumes(t *testing.T) {
	scratchDir := t.TempDir()
	clock := &fakeClock{}
	_, _, deps := newInnerRunDeps(t, nil, nil, awaitingStatus(), clock)
	clock.watchReason(scratchDir)
	deps.NoticeProbe = 30 * time.Second
	deps.ReadDecision = func() (ChildDecision, bool, error) {
		return ChildDecision{Kind: DecisionApprove, At: "2026-01-01T10:00:00Z", HeadSHA: "abc"}, true, nil
	}
	// The child's driver parks after the second attempt: the fake refuses until then.
	var attempts []time.Duration
	start := clock.Now()
	deps.Spawn = func(ctx context.Context) error {
		attempts = append(attempts, clock.Now().Sub(start))
		if len(attempts) <= 2 {
			return fmt.Errorf("%w: exit status 1", ErrChildNotParked)
		}
		return nil
	}
	producer := NewInnerRun("innerrun", "myslug", deps, 10*time.Second, scratchDir, testGrace)

	// A check every 10s; the decision is retried at 30s and at 60s, and the call ends at check 8.
	clock.pauseAtSleep = 8
	actedBeforeCheck6 := true
	clock.onSleep = func(call int) {
		if call == 6 {
			_, statErr := os.Stat(decisionActedFile(scratchDir, "innerrun"))
			actedBeforeCheck6 = statErr == nil
		}
	}
	outcome, ptr, err := producer.Call(context.Background())
	if err != nil || outcome != shedengine.Stuck || !ptr.BudgetExempt {
		t.Fatalf("Call() = %v %+v %v; want an exempt Stuck", outcome, ptr, err)
	}
	if want := []time.Duration{0, 30 * time.Second, 60 * time.Second}; !slices.Equal(attempts, want) {
		t.Errorf("resume attempts at %v; want %v, one per notice probe", attempts, want)
	}
	if actedBeforeCheck6 {
		t.Error("decision-acted marker written before the driver parked; want it absent while the resume is refused")
	}
	if !strings.Contains(clock.reasons[0], "not parked") {
		t.Errorf("wait reason while refused = %q; want it to name the unparked driver", clock.reasons[0])
	}
	if !strings.Contains(clock.lastReason(), "resume was delivered") {
		t.Errorf("wait reason after the resume = %q; want it to say the resume was delivered", clock.lastReason())
	}
	if _, statErr := os.Stat(decisionActedFile(scratchDir, "innerrun")); statErr != nil {
		t.Errorf("decision-acted marker: %v; want it written once the resume succeeded", statErr)
	}
}

func TestInnerRun_ReadDecisionErrorIsHardError(t *testing.T) {
	clock := &fakeClock{}
	readErr := errors.New("decision unreadable")
	_, _, deps := newInnerRunDeps(t, nil, nil, awaitingStatus(), clock)
	deps.ReadDecision = func() (ChildDecision, bool, error) { return ChildDecision{}, false, readErr }

	_, _, err := NewInnerRun("innerrun", "myslug", deps, time.Millisecond, t.TempDir(), testGrace).Call(context.Background())
	if !errors.Is(err, readErr) {
		t.Errorf("Call() error = %v; want it to wrap %v", err, readErr)
	}
}

// TestInnerRun_DoneWaitsForLiveDriverThenFinishes asserts a done child with a live driver is waited on inside the call, with the done-seen marker written on first sight, until the driver is gone.
func TestInnerRun_DoneWaitsForLiveDriverThenFinishes(t *testing.T) {
	scratchDir := t.TempDir()
	clock := &fakeClock{}
	_, _, deps := newInnerRunDeps(t, nil, nil, doneStatus(), clock)
	clock.watchReason(scratchDir)
	clock.pauseAtSleep = 0
	deps.NoticeProbe = 0
	alive := true
	reads := 0
	deps.DriverAlive = func(ctx context.Context) (bool, error) { reads++; return alive, nil }
	clock.onSleep = func(call int) {
		if call == 2 {
			alive = false
		}
	}
	producer := NewInnerRun("innerrun", "myslug", deps, time.Millisecond, scratchDir, testGrace)

	outcome, _, err := producer.Call(context.Background())
	if err != nil || outcome != shedengine.Done {
		t.Fatalf("Call() = %v %v; want Done once the driver is gone", outcome, err)
	}
	if clock.sleepCalls != 2 || reads != 3 {
		t.Errorf("Sleep calls = %d and driver reads = %d; want a wait of two checks with a read at entry and at each check", clock.sleepCalls, reads)
	}
	if len(clock.reasons) != 1 || !strings.Contains(clock.reasons[0], "stop report") {
		t.Errorf("wait reasons = %q; want one naming the wait for the driver's stop report", clock.reasons)
	}
	if _, err := os.Stat(doneSeenFile(scratchDir, "innerrun")); !os.IsNotExist(err) {
		t.Errorf("done-seen marker stat = %v; want it removed on Done", err)
	}
}

func TestInnerRun_DoneLivenessErrorIsTreatedAsLive(t *testing.T) {
	clock := &fakeClock{}
	_, _, deps := newInnerRunDeps(t, nil, nil, doneStatus(), clock)
	deps.DriverAlive = func(ctx context.Context) (bool, error) { return false, errors.New("reed unreadable") }

	outcome, _, err := NewInnerRun("innerrun", "myslug", deps, time.Millisecond, t.TempDir(), testGrace).Call(context.Background())
	if err != nil || outcome != shedengine.Stuck {
		t.Errorf("Call() = %v %v; want an exempt Stuck, the error read as live", outcome, err)
	}
}

// TestInnerRun_DoneFinishesPastGraceWithLiveDriverAndWarns asserts a live driver is waited on for the exit grace and no longer, that its strand is read no more than once per notice probe, and that finishing past the grace warns.
func TestInnerRun_DoneFinishesPastGraceWithLiveDriverAndWarns(t *testing.T) {
	var buf bytes.Buffer
	logger.SetOutput(&buf)
	t.Cleanup(func() { logger.SetOutput(os.Stderr) })

	scratchDir := t.TempDir()
	clock := &fakeClock{}
	_, _, deps := newInnerRunDeps(t, nil, nil, doneStatus(), clock)
	clock.pauseAtSleep = 0
	const probe = time.Minute
	deps.NoticeProbe = probe
	reads := 0
	deps.DriverAlive = func(ctx context.Context) (bool, error) { reads++; return true, nil }
	producer := NewInnerRun("innerrun", "myslug", deps, 10*time.Second, scratchDir, testGrace)

	outcome, _, err := producer.Call(context.Background())
	if err != nil || outcome != shedengine.Done {
		t.Fatalf("Call() = %v %v; want Done past the grace", outcome, err)
	}
	if want := int(testGrace/probe) + 1; reads != want {
		t.Errorf("driver reads = %d over the grace; want %d, one at entry and one per notice probe", reads, want)
	}
	if !strings.Contains(buf.String(), "myslug") || !strings.Contains(buf.String(), "exit grace") {
		t.Errorf("log = %q; want a warning naming the slug and the grace", buf.String())
	}
}

func TestInnerRun_UnparseableDoneSeenMarkerRestartsTheGrace(t *testing.T) {
	scratchDir := t.TempDir()
	clock := &fakeClock{}
	_, _, deps := newInnerRunDeps(t, nil, nil, doneStatus(), clock)
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
	_, _, deps := newInnerRunDeps(t, nil, nil, statuses, clock)
	if err := os.WriteFile(SpawnConfirmedFile(scratchDir, "innerrun"), pidMarker(os.Getpid()), 0o644); err != nil {
		t.Fatal(err)
	}
	stale := clock.Now().Add(-2 * testGrace).Format(time.RFC3339)
	if err := os.WriteFile(doneSeenFile(scratchDir, "innerrun"), []byte(stale+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	shedfake.CallOK(t, NewInnerRun("innerrun", "myslug", deps, time.Millisecond, scratchDir, testGrace))
	if _, err := os.Stat(doneSeenFile(scratchDir, "innerrun")); !os.IsNotExist(err) {
		t.Fatalf("done-seen marker stat = %v; want it removed by a running Call", err)
	}

	// A later done Call waits a full grace from its own first sight.
	_, _, doneDeps := newInnerRunDeps(t, nil, nil, doneStatus(), clock)
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
	_, _, deps := newInnerRunDeps(t, nil, nil, doneStatus(), clock)
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

// TestInnerRun_EachStateChangeIsItsOwnHistoryEntry asserts a wait is not counted against the running budget:
// every return routes back to Run-Shed, still running, even with the budget fully spent,
// and that the returns for successive state changes are separate budget-exempt history entries, never folded across changes even when two of them read alike.
func TestInnerRun_EachStateChangeIsItsOwnHistoryEntry(t *testing.T) {
	const counted = 3
	const changes = 6
	clock := &fakeClock{}
	awaiting := awaitingStatus()[0].status
	running := shedengine.Status{State: shedengine.StateRunning, CurrentProducer: awaiting.CurrentProducer}
	current := awaiting
	_, _, deps := newInnerRunDeps(t, nil, nil, awaitingStatus(), clock)
	deps.ReadStatus = func(string, string) (shedengine.Status, bool, error) { return current, true, nil }
	clock.pauseAtSleep = 0
	clock.onSleep = func(int) {
		if current.State == shedengine.StateAwaiting {
			current = running
		} else {
			current = awaiting
		}
		clock.bumpStatus()
	}
	producer := NewInnerRun("innerrun", "myslug", deps, time.Minute, t.TempDir(), testGrace)
	shed := newRunShed(t, producer, counted)

	for i := 0; i < changes; i++ {
		res, err := shed.Step(context.Background())
		if err != nil {
			t.Fatalf("Step %d error = %v", i, err)
		}
		if res.State != shedengine.StateRunning || res.Next != "Run-Shed" {
			t.Fatalf("Step %d = state %q next %q; want running routed back to Run-Shed", i, res.State, res.Next)
		}
	}

	got, ok, err := state.ReadJSON[shedengine.Status](shed.StatusPath, shed.StatusLockPath)
	if err != nil || !ok {
		t.Fatalf("read status = %v found=%v", err, ok)
	}
	if len(got.History) != counted+changes {
		t.Fatalf("history length = %d; want %d seeded counted entries plus one entry per change", len(got.History), counted+changes)
	}
	for i, entry := range got.History[counted:] {
		wantPath := "child awaiting → running at Publish"
		if i%2 == 1 {
			wantPath = "child running → awaiting at Publish"
		}
		if entry.Producer != "Run-Shed" || entry.Outcome != shedengine.Stuck || !entry.BudgetExempt || entry.Output != wantPath || entry.Repeats != 0 {
			t.Errorf("entry %d = %+v; want an unfolded Run-Shed budget-exempt Stuck with output %q", i, entry, wantPath)
		}
	}
}
