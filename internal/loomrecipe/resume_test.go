package loomrecipe

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/state"
)

// countingProducer is a shedengine.ShedProducer fake that always reports Done and counts every
// call, used to prove crash-recovery's unconditional re-call: a producer whose output already
// exists must still be called again, never skipped.
type countingProducer struct {
	calls int
}

func (p *countingProducer) Call(context.Context) (shedengine.Outcome, shedengine.OutputPointer, error) {
	p.calls++
	return shedengine.Done, shedengine.OutputPointer{}, nil
}

// resetCurrentProducer patches the status file at statusPath/statusLockPath to name
// currentProducer, state running, and pauseRequested, via state.UpdateJSON -- never by hand-writing
// JSON -- simulating the on-disk snapshot a crash, a bounce, or a pending pause leaves behind.
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

// TestResume_DoesNotRestartAtRowOne drives a run to shedengine.RunBlocked at Batchifier (whose
// OnStuck is empty, per the real Batchifier gate genuinely failing on a malformed config -- never a
// substituted fake row), fixes the on-disk cause, and asserts that a freshly constructed Shed over
// the same status file re-calls Batchifier directly rather than restarting the whole list at
// Preflight. The fixed second run blocks again, at Publish (see sequence_test.go's
// wantSequenceOrder doc comment for why) -- resuming past Batchifier, not reaching Done, is this
// test's own point.
func TestResume_DoesNotRestartAtRowOne(t *testing.T) {
	_, env, paths := buildSequenceFixture(t)
	writeBatcherConfig(t, env.AnchorPath, "active: [not valid yaml\n")

	shed1, err := New(env, paths)
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
	if result1.HaltedProducer != loomshed.NameBatchifier {
		t.Fatalf("first Run() HaltedProducer = %q; want %q", result1.HaltedProducer, loomshed.NameBatchifier)
	}

	writeBatcherConfig(t, env.AnchorPath, `active: "identity"`+"\n")

	shed2, err := New(env, paths)
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
	if result2.HaltedProducer != loomshed.NamePublish {
		t.Fatalf("second Run() HaltedProducer = %q; want %q", result2.HaltedProducer, loomshed.NamePublish)
	}

	// result2.History is the full persisted history, including the first run's own appended
	// entries (Result.History docs this explicitly) -- so "did not restart at row 1" is proven by
	// Preflight appearing exactly once across both runs, not by History[0] naming Batchifier.
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

// TestResume_CrashRecoveryRecallsUnconditionally proves the re-call is unconditional: after a run
// that reaches its terminal row, resetting current_producer back to Preflight -- as if Preflight's
// output already exists from the first pass -- still makes a fresh Shed call it again, rather than
// skip it.
//
// The two runs do not end at the same halted row: the first run ends shedengine.RunBlocked at
// Publish (see sequence_test.go's wantSequenceOrder doc comment for why). After
// resetCurrentProducer(..., loomshed.NamePreflight, false), the second run re-calls row 1, Shed
// advances to row 2, and row 2 finds a history carrying the first run's later producers -- exactly
// the half-finished shape the fresh-start rule rejects -- so the second run blocks at
// Loom-Preflight instead. This test's own point is the re-call count, not the terminal state.
//
// It holds one counting := &countingProducer{} across both builds and asserts counting.calls == 2
// at the end: substituting a fresh &countingProducer{} at the second site would leave the count at
// 1 and quietly invert what the test measures.
func TestResume_CrashRecoveryRecallsUnconditionally(t *testing.T) {
	_, env, paths := buildSequenceFixture(t)
	counting := &countingProducer{}

	shed1, err := New(env, paths)
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

	shed2, err := New(env, paths)
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

// TestResume_PauseStopsAtBoundaryAndClearsFlag sets pause_requested on a status file already
// mid-list, asserts the run stops at that boundary with shedengine.RunPaused without calling the
// halted producer, that the flag is cleared in the same persist, and that a subsequent run resumes
// rather than re-pausing on the flag it is resuming from.
func TestResume_PauseStopsAtBoundaryAndClearsFlag(t *testing.T) {
	_, env, paths := buildSequenceFixture(t)
	resetCurrentProducer(t, paths.StatusPath, paths.StatusLockPath, loomshed.NameBatchifier, true)

	shed1, err := New(env, paths)
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
	if result1.HaltedProducer != loomshed.NameBatchifier {
		t.Fatalf("first Run() HaltedProducer = %q; want %q", result1.HaltedProducer, loomshed.NameBatchifier)
	}
	if len(result1.History) != 0 {
		t.Errorf("first Run() History = %+v; want empty -- pause is checked before the halted producer is ever called", result1.History)
	}

	got, found, err := state.ReadJSONStrict[shedengine.Status](paths.StatusPath, paths.StatusLockPath)
	if err != nil {
		t.Fatalf("ReadJSONStrict() error = %v; want nil", err)
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
	if got.CurrentProducer != loomshed.NameBatchifier {
		t.Errorf("persisted CurrentProducer = %q; want %q", got.CurrentProducer, loomshed.NameBatchifier)
	}

	shed2, err := New(env, paths)
	if err != nil {
		t.Fatalf("New() error = %v; want nil", err)
	}
	shed2.Producers[0].Producer = fakeAlwaysDoneProducer{}
	result2, err := shed2.Run(context.Background())
	if err != nil {
		t.Fatalf("second Run() error = %v; want nil", err)
	}
	// A subsequent run must resume rather than re-pause on the flag it is resuming from -- it
	// blocks at Publish (see sequence_test.go's wantSequenceOrder doc comment for why), never
	// shedengine.RunPaused again.
	if result2.Outcome != shedengine.RunBlocked {
		t.Fatalf("second Run() outcome = %q; want %q -- a subsequent run must resume rather than re-pause on the flag it is resuming from", result2.Outcome, shedengine.RunBlocked)
	}
	if result2.HaltedProducer != loomshed.NamePublish {
		t.Errorf("second Run() HaltedProducer = %q; want %q", result2.HaltedProducer, loomshed.NamePublish)
	}
}

// TestBounceRouting_StuckContinuesAtDeclaredTarget drives Discussion-Bouncer (a real producer)
// genuinely Stuck by scripting its judge round's verdict as non-approving, and asserts the run
// continues at its declared OnStuck target, Discussion-Burler, immediately afterward.
//
// Discussion-Validate is gone, and with it the standalone-validator vehicle this test used to drive:
// a real, repeatably bounceable producer pair is what this test needs, and the Discussion-Review
// segment's mutual on_stuck pair (Discussion-Bouncer/Discussion-Burler) is the one this task's new
// writer-row gates have no bearing on -- fakeLoomBurler never reads burlerengine.RunOpts.Gate at
// all -- so it is what remains, matching the same assertion shape over a pair that still exists.
// This test asserts only that routing continues at the declared target after one Stuck -- the
// budget-exhaustion property belongs to TestBounceRouting_BudgetExhaustionBlocks below.
func TestBounceRouting_StuckContinuesAtDeclaredTarget(t *testing.T) {
	_, env, paths := buildSequenceFixture(t)
	env.Shuttle.(*fakeLoomShuttle).bouncerVerdict = "BLOCKING"

	shed, err := New(env, paths)
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
		t.Fatalf("Run() History ends at the Stuck entry; want a following entry naming the bounce target %q", loomshed.NameDiscussionBurler)
	}
	if got := result.History[stuckIdx+1].Producer; got != loomshed.NameDiscussionBurler {
		t.Errorf("History[%d].Producer (following the Stuck entry) = %q; want the declared bounce target %q", stuckIdx+1, got, loomshed.NameDiscussionBurler)
	}
}

// TestBounceRouting_EmptyTargetBlocksInstead drives Batchifier (a real producer, OnStuck: "")
// genuinely Stuck via a malformed on-disk config, and asserts the run ends shedengine.RunBlocked
// rather than bouncing anywhere.
func TestBounceRouting_EmptyTargetBlocksInstead(t *testing.T) {
	_, env, paths := buildSequenceFixture(t)
	writeBatcherConfig(t, env.AnchorPath, "active: [not valid yaml\n")

	shed, err := New(env, paths)
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
	if result.HaltedProducer != loomshed.NameBatchifier {
		t.Errorf("Run() HaltedProducer = %q; want %q", result.HaltedProducer, loomshed.NameBatchifier)
	}
	if result.Reason != "stuck with no OnStuck target" {
		t.Errorf("Run() Reason = %q; want %q", result.Reason, "stuck with no OnStuck target")
	}
}

// TestBounceRouting_BudgetExhaustionBlocks drives Discussion-Bouncer (a real producer) genuinely
// and repeatedly Stuck by scripting its judge round's verdict as BLOCKING every time, and asserts
// Discussion-Bouncer's own bounce budget is consumed and exhausting it blocks -- MaxBounces+1 Stuck
// entries authored by Discussion-Bouncer, then shedengine.RunBlocked.
//
// Discussion-Validate is no longer this test's vehicle. Discussion-Write now carries its own "gate:
// discussion" Config key (resolveGateSpec, internal/shedrecipe/entries_gate.go), which calls the
// exact same discussionparser.Validate function Discussion-Validate's own Call does -- so a
// Discussion-Write whose redo never fixes its artifact fails its OWN gate on every bounce, and
// Discussion-Write carries no on_stuck, so that failure blocks the whole run on the very first
// bounce rather than letting Discussion-Validate cycle it repeatedly. Discussion-Bouncer's mutual
// on_stuck with Discussion-Burler (segment: Discussion-Review) has no such dependency on this
// task's new gates -- fakeLoomBurler, this fixture's shedadapters.BurlerRunner fake, never reads
// burlerengine.RunOpts.Gate at all -- so it is what remains a genuinely, repeatably bounceable real
// producer pair under the new design, matching the property this test exists to prove (a real
// producer's per-episode bounce budget is consumed and exhausted, never a fake standing in for a
// generic engine mechanism the recipe's own graph never exercises).
//
// The budget here is per-producer and episode-scoped, counted from the persisted history[] -- never
// a run-wide counter. f.bouncerVerdict = "BLOCKING" makes the judge round reject on every pass, so
// Discussion-Bouncer never returns Done and its episode (the run of its own history entries since
// its last Done) is therefore the whole run: every Stuck entry it authors counts. Discussion-Burler,
// the producer it bounces to, consumes none of Discussion-Bouncer's own budget -- each producer's
// episode count is its own.
//
// Discussion-Bouncer's max_bounces is declared directly on its own recipe row
// (contracts/recipes/loom-recipe.yaml), not inherited from ShedPaths.MaxBounces the way
// Discussion-Validate's budget once was, so this test drives discussionBouncerMaxBounces+1 round
// trips rather than parameterizing a small paths.MaxBounces.
//
// The Bouncer is what this test binds its assertion to, never the Burler, even though both segment
// rows declare their own max_bounces of 5: Discussion-Bouncer's own Stuck sequence runs one ahead of
// the round producer's count -- its seed call authors the segment's very first Stuck before
// Discussion-Burler ever runs once -- so with equal budgets Discussion-Bouncer exhausts first,
// within the segment's first generation, and Discussion-Burler's own budget is never spent at all.
func TestBounceRouting_BudgetExhaustionBlocks(t *testing.T) {
	// discussionBouncerMaxBounces mirrors the "max_bounces: 5" this task's Card 25 left untouched
	// on the Discussion-Bouncer row of contracts/recipes/loom-recipe.yaml.
	const discussionBouncerMaxBounces = 5

	_, env, paths := buildSequenceFixture(t)
	env.Shuttle.(*fakeLoomShuttle).bouncerVerdict = "BLOCKING"

	shed, err := New(env, paths)
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
	if result.Reason != "bounce budget exhausted" {
		t.Errorf("Run() Reason = %q; want %q", result.Reason, "bounce budget exhausted")
	}

	// The blocking Stuck entry is itself appended to history before the inner switch decides
	// whether to bounce or block, so it counts toward the total even though it is the one that
	// triggers the block rather than one the budget check let through -- MaxBounces+1 total.
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

// TestResume_DiscussionWriteRespawnsRatherThanReportDoneOffFileExistence is one half of the
// The "interactive-mode trap" regression pair, retargeted onto the row it was
// always really about now that Discussion-Validate, the bounce that used to reach it, is gone:
// current_producer is planted directly at Discussion-Write with both discussion artifacts already
// present and complete on disk -- the identical on-disk shape a crash mid-interview leaves -- and
// the fake shuttle's Attach reports not-found (no live agent matches), so Discussion-Write must
// respawn rather than report Done off bare file existence.
//
// current_producer is planted directly at Discussion-Write via resetCurrentProducer, per the
// resume tests above, rather than driven through the whole sequence from row 1 -- the point under
// test is what Discussion-Write does when re-entered, not how the run got there.
//
// The fake's default writeOutputs=true means the respawned run rewrites both output files with
// valid content, so Discussion-Write's own gate passes on the respawned run's very next call: this
// is what proves the respawn resolves to a genuine Done rather than reporting Done straight off the
// pre-existing files without ever calling Run again.
func TestResume_DiscussionWriteRespawnsRatherThanReportDoneOffFileExistence(t *testing.T) {
	_, env, paths := buildSequenceFixture(t)

	loomShuttle := env.Shuttle.(*fakeLoomShuttle)

	resetCurrentProducer(t, paths.StatusPath, paths.StatusLockPath, loomshed.NameDiscussionWrite, false)

	shed, err := New(env, paths)
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

	if loomShuttle.attachCalls == 0 {
		t.Errorf("fakeLoomShuttle.attachCalls = 0; want > 0 -- the probe must run before any archive-and-respawn decision")
	}
	if loomShuttle.discussionRunCalls == 0 {
		t.Errorf("fakeLoomShuttle.discussionRunCalls = 0; want > 0 -- with Attach reporting not-found, Discussion-Write must respawn a fresh agent")
	}
}

// TestResume_LiveMatchingRunAttachesInsteadOfRespawning is the other half of the regression pair:
// the identical on-disk shape as the bounce case above -- both discussion artifacts present -- but
// this time the fake shuttle's Attach reports a still-live matching run (Outcome: OutcomeDone), the
// crash-mid-interview case where the agent already finished before the driver noticed. Discussion-Write
// must attach to that result rather than archive the artifacts and spawn a second agent on top of
// one that (as far as this run's own state on disk is concerned) may still be working.
//
// current_producer is planted directly at Discussion-Write, since this test's whole point is what
// that row does when re-entered with a live match, not how the run got there.
func TestResume_LiveMatchingRunAttachesInsteadOfRespawning(t *testing.T) {
	_, env, paths := buildSequenceFixture(t)

	loomShuttle := env.Shuttle.(*fakeLoomShuttle)
	loomShuttle.attachFound = true
	loomShuttle.attachRole = "discussion"
	loomShuttle.attachResult = shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}

	resetCurrentProducer(t, paths.StatusPath, paths.StatusLockPath, loomshed.NameDiscussionWrite, false)

	shed, err := New(env, paths)
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
		t.Errorf("Discussion-Write outcome = %q; want %q -- attaching to a live matching run whose Outcome is OutcomeDone must report Done", got, shedengine.Done)
	}

	if loomShuttle.discussionRunCalls != 0 {
		t.Errorf("fakeLoomShuttle.discussionRunCalls = %d; want 0 -- an attached run must not respawn a second agent", loomShuttle.discussionRunCalls)
	}
	if loomShuttle.attachCalls == 0 {
		t.Errorf("fakeLoomShuttle.attachCalls = 0; want > 0 -- the probe must have run to find the live match")
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
