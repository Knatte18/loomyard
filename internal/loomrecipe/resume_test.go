package loomrecipe

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/seatengine"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/state"
	"github.com/Knatte18/loomyard/internal/testkit/shedfake"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// liveSeatLead is the non-nil seatengine.Handle the fake seat runner returns as a live lead.
// The producer never calls it.
var liveSeatLead seatengine.Handle = (*shuttleengine.Run)(nil)

// countingProducer is a shedengine.ShedProducer fake that always reports Done and counts every call.
// It proves the crash-recovery re-call is unconditional:
// a producer whose output already exists must still be called again, never skipped.
type countingProducer struct {
	calls int
}

func (p *countingProducer) Call(context.Context) (shedengine.Outcome, shedengine.OutputPointer, error) {
	p.calls++
	return shedengine.Done, shedengine.OutputPointer{}, nil
}

// resetCurrentProducer patches the status file at statusPath and statusLockPath.
// It sets CurrentProducer to currentProducer, State to running, and PauseRequested to pauseRequested.
// It writes through state.UpdateJSON, never hand-written JSON.
// The result simulates the on-disk snapshot a crash, a bounce, or a pending pause leaves behind.
func resetCurrentProducer(t *testing.T, statusPath, statusLockPath, currentProducer string, pauseRequested bool) {
	t.Helper()
	err := state.UpdateJSON(statusPath, statusLockPath, func(cur shedengine.Status, found bool) (shedengine.Status, error) {
		cur.CurrentProducer = currentProducer
		cur.State = shedengine.StateRunning
		cur.PauseRequested = pauseRequested
		return cur, nil
	})
	if err != nil {
		t.Fatalf("resetCurrentProducer: state.UpdateJSON: %v", err)
	}
}

// TestResume_DoesNotRestartAtRowOne drives a run to shedengine.RunBlocked at Webster.
// The real Webster producer genuinely fails on a malformed batcher config, so its OnStuck is empty.
// No substituted fake row stands in for it.
// The test fixes that on-disk cause.
// It then asserts that a freshly constructed Shed over the same status file re-calls Webster directly.
// It does not restart the whole list at Preflight.
// The fixed second run blocks again at Merge-In, where the fixture's verdict is Stuck.
// Resuming past Webster, not reaching Done, is this test's own point.
func TestResume_DoesNotRestartAtRowOne(t *testing.T) {
	tiergate.Require(t, 0)
	_, env, paths := buildSequenceFixture(t)
	writeBatcherConfig(t, env.AnchorPath, "active: [not valid yaml\n")

	shed1, err := New(shedrun.RecipeTapestry, env, paths)
	if err != nil {
		t.Fatalf("New() error = %v; want nil", err)
	}
	shed1.Producers[0].Producer = fakeAlwaysDoneProducer{}
	result1, err := shed1.Run(context.Background())
	if err != nil {
		t.Fatalf("first Run() error = %v; want nil", err)
	}
	if result1.Outcome != shedengine.RunBlocked {
		t.Fatalf("first Run() outcome = %q; want %q (reason: %s)", result1.Outcome, shedengine.RunBlocked, result1.Reason)
	}
	if result1.HaltedProducer != loomshed.NameWebster {
		t.Fatalf("first Run() HaltedProducer = %q; want %q", result1.HaltedProducer, loomshed.NameWebster)
	}

	writeBatcherConfig(t, env.AnchorPath, `active: "identity"`+"\n")

	shed2, err := New(shedrun.RecipeTapestry, env, paths)
	if err != nil {
		t.Fatalf("New() error = %v; want nil", err)
	}
	shed2.Producers[0].Producer = fakeAlwaysDoneProducer{}
	result2, err := shed2.Run(context.Background())
	if err != nil {
		t.Fatalf("second Run() error = %v; want nil", err)
	}
	if result2.Outcome != shedengine.RunBlocked {
		t.Fatalf("second Run() outcome = %q; want %q (reason: %s)", result2.Outcome, shedengine.RunBlocked, result2.Reason)
	}
	if result2.HaltedProducer != loomshed.NameParentAlign {
		t.Fatalf("second Run() HaltedProducer = %q; want %q", result2.HaltedProducer, loomshed.NameParentAlign)
	}

	// result2.History is the full persisted history: it includes the first run's own entries.
	// So the proof is Preflight appearing exactly once across both runs, not History[0] naming Webster.
	preflightCount := 0
	for _, e := range result2.History {
		if e.Producer == loomshed.NamePreflight {
			preflightCount++
		}
	}
	if preflightCount != 1 {
		t.Errorf("Preflight appears %d time(s) across both runs' History; want 1 -- a restart at row 1 would call it again", preflightCount)
	}
}

// TestResume_CrashRecoveryRecallsUnconditionally proves the re-call is unconditional.
// After a run reaches its terminal row, it resets CurrentProducer to Preflight.
// That mimics Preflight's output already existing from the first pass.
// A fresh Shed still calls Preflight again rather than skipping it.
//
// Both runs end at shedengine.RunBlocked at Merge-In, where the fixture's verdict is Stuck.
// After resetCurrentProducer(..., loomshed.NamePreflight, false), the second run re-calls row 1;
// it then runs the later rows over the first run's history.
// This test's own point is the re-call count, not the terminal state.
//
// It holds one counting := &countingProducer{} across both builds and asserts counting.calls == 2 at the end:
// substituting a fresh &countingProducer{} at the second site would leave the count at 1.
// That would quietly invert what the test measures.
func TestResume_CrashRecoveryRecallsUnconditionally(t *testing.T) {
	tiergate.Require(t, 0)
	_, env, paths := buildSequenceFixture(t)
	counting := &countingProducer{}

	shed1, err := New(shedrun.RecipeTapestry, env, paths)
	if err != nil {
		t.Fatalf("New() error = %v; want nil", err)
	}
	shed1.Producers[0].Producer = counting
	result1, err := shed1.Run(context.Background())
	if err != nil {
		t.Fatalf("first Run() error = %v; want nil", err)
	}
	if result1.Outcome != shedengine.RunBlocked {
		t.Fatalf("first Run() outcome = %q; want %q", result1.Outcome, shedengine.RunBlocked)
	}
	if counting.calls != 1 {
		t.Fatalf("counting.calls after first Run() = %d; want 1", counting.calls)
	}

	resetCurrentProducer(t, paths.StatusPath, paths.StatusLockPath, loomshed.NamePreflight, false)

	shed2, err := New(shedrun.RecipeTapestry, env, paths)
	if err != nil {
		t.Fatalf("New() error = %v; want nil", err)
	}
	shed2.Producers[0].Producer = counting
	result2, err := shed2.Run(context.Background())
	if err != nil {
		t.Fatalf("second Run() error = %v; want nil", err)
	}
	if result2.Outcome != shedengine.RunBlocked {
		t.Fatalf("second Run() outcome = %q; want %q", result2.Outcome, shedengine.RunBlocked)
	}
	if counting.calls != 2 {
		t.Errorf("counting.calls after second Run() = %d; want 2 -- Preflight must be re-called even though its output already exists from the first pass", counting.calls)
	}
}

// TestResume_PauseStopsAtBoundaryAndClearsFlag sets pause_requested on a status file already mid-list.
// It asserts the run stops at that boundary with shedengine.RunPaused without calling the halted producer.
// It also asserts that the flag is cleared in the same persist.
// A subsequent run must resume rather than re-pause on it.
func TestResume_PauseStopsAtBoundaryAndClearsFlag(t *testing.T) {
	tiergate.Require(t, 0)
	_, env, paths := buildSequenceFixture(t)
	resetCurrentProducer(t, paths.StatusPath, paths.StatusLockPath, loomshed.NameWebster, true)

	shed1, err := New(shedrun.RecipeTapestry, env, paths)
	if err != nil {
		t.Fatalf("New() error = %v; want nil", err)
	}
	shed1.Producers[0].Producer = fakeAlwaysDoneProducer{}
	result1, err := shed1.Run(context.Background())
	if err != nil {
		t.Fatalf("first Run() error = %v; want nil", err)
	}
	if result1.Outcome != shedengine.RunPaused {
		t.Fatalf("first Run() outcome = %q; want %q", result1.Outcome, shedengine.RunPaused)
	}
	if result1.HaltedProducer != loomshed.NameWebster {
		t.Fatalf("first Run() HaltedProducer = %q; want %q", result1.HaltedProducer, loomshed.NameWebster)
	}
	if len(result1.History) != 0 {
		t.Errorf("first Run() History = %+v; want empty -- pause is checked before the halted producer is ever called", result1.History)
	}

	got, found, err := state.ReadJSON[shedengine.Status](paths.StatusPath, paths.StatusLockPath)
	if err != nil {
		t.Fatalf("ReadJSON() error = %v; want nil", err)
	}
	if !found {
		t.Fatalf("status file not found after pause")
	}
	if got.State != shedengine.StatePaused {
		t.Errorf("persisted State = %q; want %q", got.State, shedengine.StatePaused)
	}
	if got.PauseRequested {
		t.Errorf("persisted PauseRequested = true; want false -- the flag must clear in the same persist that records the paused state")
	}
	if got.CurrentProducer != loomshed.NameWebster {
		t.Errorf("persisted CurrentProducer = %q; want %q", got.CurrentProducer, loomshed.NameWebster)
	}

	shed2, err := New(shedrun.RecipeTapestry, env, paths)
	if err != nil {
		t.Fatalf("New() error = %v; want nil", err)
	}
	shed2.Producers[0].Producer = fakeAlwaysDoneProducer{}
	result2, err := shed2.Run(context.Background())
	if err != nil {
		t.Fatalf("second Run() error = %v; want nil", err)
	}
	// A subsequent run must resume rather than re-pause on the flag it is resuming from.
	// It blocks at Merge-In, where the fixture's verdict is Stuck, and never returns shedengine.RunPaused again.
	if result2.Outcome != shedengine.RunBlocked {
		t.Fatalf("second Run() outcome = %q; want %q -- a subsequent run must resume rather than re-pause on the flag it is resuming from", result2.Outcome, shedengine.RunBlocked)
	}
	if result2.HaltedProducer != loomshed.NameParentAlign {
		t.Errorf("second Run() HaltedProducer = %q; want %q", result2.HaltedProducer, loomshed.NameParentAlign)
	}
}

// TestBounceRouting_StuckContinuesAtDeclaredTarget drives Discussion-Bouncer (a real producer)
// genuinely Stuck by scripting its judge round's verdict as non-approving, and asserts the run
// continues at its declared OnStuck target, Discussion-Burler-Review, immediately afterward.
//
// TestBounceRouting_StuckContinuesAtDeclaredTarget checks that a Stuck bounce continues at the declared target.
// It drives Discussion-Bouncer, a real producer, through the Discussion-Review segment's on_stuck ring.
// The loom burler never reads burlerengine.RunOpts.Gate, so the ring does not depend on the writer-row gates.
// It asserts only that routing continues at the target after one Stuck.
// The budget-exhaustion property is covered by TestBounceRouting_BudgetExhaustionBlocks.
func TestBounceRouting_StuckContinuesAtDeclaredTarget(t *testing.T) {
	tiergate.Require(t, 0)
	_, env, paths := buildSequenceFixture(t)
	blockBouncerJudge(env)

	shed, err := New(shedrun.RecipeTapestry, env, paths)
	if err != nil {
		t.Fatalf("New() error = %v; want nil", err)
	}
	shed.Producers[0].Producer = fakeAlwaysDoneProducer{}
	result, err := shed.Run(context.Background())
	if err != nil {
		t.Fatalf("Run() error = %v; want nil", err)
	}

	stuckIdx := -1
	for i, e := range result.History {
		if e.Producer == loomshed.NameDiscussionBouncer && e.Outcome == shedengine.Stuck {
			stuckIdx = i
			break
		}
	}
	if stuckIdx == -1 {
		t.Fatalf("Run() History has no %s Stuck entry: %+v", loomshed.NameDiscussionBouncer, result.History)
	}
	if stuckIdx+1 >= len(result.History) {
		t.Fatalf("Run() History ends at the Stuck entry; want a following entry naming the bounce target %q", loomshed.NameDiscussionBurlerReview)
	}
	if got := result.History[stuckIdx+1].Producer; got != loomshed.NameDiscussionBurlerReview {
		t.Errorf("History[%d].Producer (following the Stuck entry) = %q; want the declared bounce target %q", stuckIdx+1, got, loomshed.NameDiscussionBurlerReview)
	}
}

// TestBounceRouting_EmptyTargetBlocksInstead checks that a Stuck producer with no OnStuck target blocks the run.
// It makes Webster, a real producer, Stuck with a malformed on-disk batcher config.
// The run then ends in shedengine.RunBlocked rather than bouncing anywhere.
func TestBounceRouting_EmptyTargetBlocksInstead(t *testing.T) {
	tiergate.Require(t, 0)
	_, env, paths := buildSequenceFixture(t)
	writeBatcherConfig(t, env.AnchorPath, "active: [not valid yaml\n")

	shed, err := New(shedrun.RecipeTapestry, env, paths)
	if err != nil {
		t.Fatalf("New() error = %v; want nil", err)
	}
	shed.Producers[0].Producer = fakeAlwaysDoneProducer{}
	result, err := shed.Run(context.Background())
	if err != nil {
		t.Fatalf("Run() error = %v; want nil", err)
	}
	if result.Outcome != shedengine.RunBlocked {
		t.Fatalf("Run() outcome = %q; want %q", result.Outcome, shedengine.RunBlocked)
	}
	if result.HaltedProducer != loomshed.NameWebster {
		t.Errorf("Run() HaltedProducer = %q; want %q", result.HaltedProducer, loomshed.NameWebster)
	}
	// The block reason carries the batcher fault's own cause, not a generic missing-OnStuck message.
	const wantPrefix = "active batchifier did not resolve: "
	if !strings.HasPrefix(result.Reason, wantPrefix) {
		t.Errorf("Run() Reason = %q; want prefix %q", result.Reason, wantPrefix)
	}
}

// TestBounceRouting_BudgetExhaustionBlocks checks that exhausting a producer's bounce budget blocks the run.
// It drives Discussion-Bouncer, a real producer, Stuck on every round by scripting its judge to CONTINUE.
// It asserts MaxBounces+1 Stuck entries authored by Discussion-Bouncer, then shedengine.RunBlocked.
//
// The budget is per producer and episode-scoped, counted from the persisted history.
// blockBouncerJudge rejects every judge round, so Discussion-Bouncer never returns Done,
// and its episode is the whole run: every Stuck entry it authors counts.
// The Burler rows it bounces through spend none of its budget, since each producer's episode count is its own.
// The budget is the fixture's Env.Segments["Discussion-Review"].MaxBounces.
// New applies it to the row because the recipe declares none; it is not taken from ShedPaths.MaxBounces.
// So this test drives that budget+1 round trips rather than parameterizing a small paths.MaxBounces.
//
// The Bouncer is what this test binds its assertion to, never the Burler.
// Discussion-Burler-Review and Discussion-Burler-Fix hand-backs are budget-exempt.
// So Discussion-Bouncer's budget is the segment's one budget on judged rounds.
func TestBounceRouting_BudgetExhaustionBlocks(t *testing.T) {
	tiergate.Require(t, 0)
	_, env, paths := buildSequenceFixture(t)
	blockBouncerJudge(env)
	discussionBouncerMaxBounces := env.Segments["Discussion-Review"].MaxBounces

	shed, err := New(shedrun.RecipeTapestry, env, paths)
	if err != nil {
		t.Fatalf("New() error = %v; want nil", err)
	}
	shed.Producers[0].Producer = fakeAlwaysDoneProducer{}
	result, err := shed.Run(context.Background())
	if err != nil {
		t.Fatalf("Run() error = %v; want nil", err)
	}
	if result.Outcome != shedengine.RunBlocked {
		t.Fatalf("Run() outcome = %q; want %q", result.Outcome, shedengine.RunBlocked)
	}
	if result.HaltedProducer != loomshed.NameDiscussionBouncer {
		t.Errorf("Run() HaltedProducer = %q; want %q", result.HaltedProducer, loomshed.NameDiscussionBouncer)
	}
	if !strings.HasPrefix(result.Reason, shedengine.ReasonBounceBudgetExhausted) {
		t.Errorf("Run() Reason = %q; want prefix %q", result.Reason, shedengine.ReasonBounceBudgetExhausted)
	}

	// The blocking Stuck entry is appended to history before the inner switch decides whether to bounce or block,
	// so it counts toward the total: MaxBounces+1 in all.
	stuckCount := 0
	for _, e := range result.History {
		if e.Producer == loomshed.NameDiscussionBouncer && e.Outcome == shedengine.Stuck {
			stuckCount++
		}
	}
	if want := discussionBouncerMaxBounces + 1; stuckCount != want {
		t.Errorf("Discussion-Bouncer Stuck count = %d; want %d (MaxBounces+1)", stuckCount, want)
	}
}

// TestResume_DiscussionWriteRespawnsRatherThanReportDoneOffFileExistence checks the interactive-mode trap.
// current_producer is planted at Discussion-Write, with both discussion artifacts present and complete on disk.
// That is the on-disk shape a crash mid-interview leaves.
// The fake seat runner's Probe reports no live lead.
// So Discussion-Write must respawn rather than report Done off bare file existence.
//
// The test plants it through resetCurrentProducer.
// It targets what the row does when re-entered, not how the run reached it.
//
// The fake's default writeOutputs=true makes the respawned run rewrite both output files with valid content.
// The row's gate therefore passes on the respawned run's next call.
// That shows the Done came from a Run, not from the files already on disk.
func TestResume_DiscussionWriteRespawnsRatherThanReportDoneOffFileExistence(t *testing.T) {
	tiergate.Require(t, 0)
	_, env, paths := buildSequenceFixture(t)

	seats := env.Seats.(*shedfake.SeatRunner)

	resetCurrentProducer(t, paths.StatusPath, paths.StatusLockPath, loomshed.NameDiscussionWrite, false)

	shed, err := New(shedrun.RecipeTapestry, env, paths)
	if err != nil {
		t.Fatalf("New() error = %v; want nil", err)
	}
	shed.Producers[0].Producer = fakeAlwaysDoneProducer{}
	result, err := shed.Run(context.Background())
	if err != nil {
		t.Fatalf("Run() error = %v; want nil", err)
	}

	writeIdx := -1
	for i, e := range result.History {
		if e.Producer == loomshed.NameDiscussionWrite {
			writeIdx = i
			break
		}
	}
	if writeIdx == -1 {
		t.Fatalf("Run() History has no %s entry: %+v", loomshed.NameDiscussionWrite, result.History)
	}
	if got := result.History[writeIdx].Outcome; got != shedengine.Done {
		t.Errorf("Discussion-Write outcome = %q; want %q -- a respawned run must itself report Done, never report Done off bare pre-existing file existence", got, shedengine.Done)
	}

	if seats.ProbeCalls == 0 {
		t.Errorf("seat runner was never probed; want it probed -- the probe must run before any archive-and-respawn decision")
	}
	if countSeatRole(seats, "discussion") == 0 {
		t.Errorf("discussion tables run = 0; want > 0 -- with no live lead, Discussion-Write must respawn a fresh lead")
	}
}

// TestResume_LiveMatchingRunAttachesInsteadOfRespawning covers Discussion-Write re-entered with a live match.
// Both discussion artifacts are on disk, and the fake seat runner's Probe reports a live lead whose run ended done.
// This is the crash-mid-interview case where the agent finished before the driver noticed.
// Discussion-Write must resume that lead.
// It must not archive the artifacts and start a second lead beside one that may still be working.
//
// current_producer is planted directly at Discussion-Write.
// The test targets what that row does when re-entered with a live match, not how the run reached it.
func TestResume_LiveMatchingRunAttachesInsteadOfRespawning(t *testing.T) {
	tiergate.Require(t, 0)
	_, env, paths := buildSequenceFixture(t)

	// The live answer covers only the discussion prefix.
	// A table with any other prefix is not live, so later rows still run and write their outputs.
	seats := env.Seats.(*shedfake.SeatRunner)
	seats.ProbeFn = func(table seatengine.Table) (seatengine.LiveTable, error) {
		if table.RolePrefix == "discussion" {
			return seatengine.LiveTable{Chair: liveSeatLead}, nil
		}
		return seatengine.LiveTable{}, nil
	}
	seats.ResumeFn = func(table seatengine.Table, _ seatengine.LiveTable) (seatengine.Result, error) {
		return seatengine.Result{
			Chair:        shuttleengine.Result{Outcome: shuttleengine.OutcomeDone},
			ChairOutputs: []string{env.DecisionRecordPath, env.SupportLogPath},
		}, nil
	}

	resetCurrentProducer(t, paths.StatusPath, paths.StatusLockPath, loomshed.NameDiscussionWrite, false)

	shed, err := New(shedrun.RecipeTapestry, env, paths)
	if err != nil {
		t.Fatalf("New() error = %v; want nil", err)
	}
	shed.Producers[0].Producer = fakeAlwaysDoneProducer{}
	result, err := shed.Run(context.Background())
	if err != nil {
		t.Fatalf("Run() error = %v; want nil", err)
	}

	writeIdx := -1
	for i, e := range result.History {
		if e.Producer == loomshed.NameDiscussionWrite {
			writeIdx = i
			break
		}
	}
	if writeIdx == -1 {
		t.Fatalf("Run() History has no %s entry: %+v", loomshed.NameDiscussionWrite, result.History)
	}
	if got := result.History[writeIdx].Outcome; got != shedengine.Done {
		t.Errorf("Discussion-Write outcome = %q; want %q -- resuming a live matching lead that ended done must report Done", got, shedengine.Done)
	}

	if got := countSeatRole(seats, "discussion"); got != 0 {
		t.Errorf("discussion tables run = %d; want 0 -- a resumed lead must not start a second one", got)
	}
	if len(seats.GotResumeTables) != 1 || seats.GotResumeTables[0].RolePrefix != "discussion" {
		t.Errorf("seat runner resumed %d tables; want exactly the discussion table", len(seats.GotResumeTables))
	}

	if _, err := os.Stat(env.DecisionRecordPath); err != nil {
		t.Errorf("os.Stat(decisionRecordPath) after attach = %v; want the original file untouched, never archived to a timestamped sibling", err)
	}
	if _, err := os.Stat(env.SupportLogPath); err != nil {
		t.Errorf("os.Stat(supportLogPath) after attach = %v; want the original file untouched, never archived to a timestamped sibling", err)
	}
	siblings, err := filepath.Glob(filepath.Join(filepath.Dir(env.DecisionRecordPath), "decision-record-*"))
	if err != nil {
		t.Fatalf("Glob() error = %v; want nil", err)
	}
	if len(siblings) != 0 {
		t.Errorf("archived siblings of decision-record.md = %v; want none -- an attached run must not archive the live agent's own output files", siblings)
	}
}
