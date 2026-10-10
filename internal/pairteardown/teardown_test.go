// teardown_test.go drives the teardown sequence through fakes, so it spawns nothing.

package pairteardown

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/shedverbs"
)

// fakeSubstrates records the order of substrate calls and answers from scripted values.
type fakeSubstrates struct {
	calls      []string
	quiets     []quietState
	refusalErr error
	endErr     error
	seizeErr   error
	awaitErr   error
	sleeps     int

	// removeErr is returned by the removal.
	removeErr error
	// childLanded is what the landed read answers.
	childLanded bool
	// battenSeeded and battenState are what batten's run reads answer.
	battenSeeded bool
	battenState  string
	// claim is the board entry's status when claimFound; claimReadErr fails the read.
	claim        *string
	claimFound   bool
	claimReadErr error
}

func (f *fakeSubstrates) teardown() *Teardown {
	return &Teardown{
		prime:    &lyxcwd.Location{HubPath: "/hub", AnchorRel: "."},
		interval: time.Second,
		gone:     func(string) (bool, error) { return false, nil },
		quiet: func(string) (quietState, error) {
			f.calls = append(f.calls, "quiet")
			st := f.quiets[0]
			if len(f.quiets) > 1 {
				f.quiets = f.quiets[1:]
			}
			return st, nil
		},
		refusal: func(string, bool) error {
			f.calls = append(f.calls, "refusal")
			return f.refusalErr
		},
		endSession: func(string, bool) (bool, string, error) {
			f.calls = append(f.calls, "end")
			return f.endErr == nil, "", f.endErr
		},
		remove: func(Request) (fabricengine.RemoveResult, error) {
			f.calls = append(f.calls, "remove")
			return fabricengine.RemoveResult{}, f.removeErr
		},
		landed: func(string) (bool, error) {
			f.calls = append(f.calls, "landed")
			return f.childLanded, nil
		},
		battenRun: func(string) (bool, string, error) {
			f.calls = append(f.calls, "battenRun")
			return f.battenSeeded, f.battenState, nil
		},
		readClaim: func(string) (*string, bool, error) {
			f.calls = append(f.calls, "readClaim")
			return f.claim, f.claimFound, f.claimReadErr
		},
		writeClaim: func(_ string, status *string) error {
			if status == nil {
				f.calls = append(f.calls, "write=clear")
			} else {
				f.calls = append(f.calls, "write="+*status)
			}
			return nil
		},
		sleep: func(context.Context, time.Duration) error {
			f.sleeps++
			return nil
		},
		killLoop: func(string) (bool, error) {
			f.calls = append(f.calls, "kill")
			return true, nil
		},
		seizeLoop: func(_ context.Context, _ string, kill func() (bool, error)) (func(bool) error, error) {
			if _, err := kill(); err != nil {
				return nil, err
			}
			f.calls = append(f.calls, "seize")
			if f.seizeErr != nil {
				return nil, f.seizeErr
			}
			return func(sessionEnded bool) error {
				f.calls = append(f.calls, fmt.Sprintf("release(ended=%t)", sessionEnded))
				return nil
			}, nil
		},
		awaitRunLock: func(context.Context, string) error {
			f.calls = append(f.calls, "await")
			return f.awaitErr
		},
	}
}

var busy = quietState{driver: "tst:slug:driver"}

// TestTeardown_CallSequence drives EndSession and Run through the fake substrates and pins which substrate calls each makes, in what order, and what it returns.
// The board-claim rows pin the settlement rule through Run: which reads precede the removal and the write, and what is written.
func TestTeardown_CallSequence(t *testing.T) {
	t.Parallel()

	refusal := errors.New("worktree has uncommitted changes")
	text := func(s string) *string { return &s }
	runClaim := text(boardengine.RunStatus("running", "Webster-Burler"))
	const sessionEnd = "quiet,refusal,kill,seize,await,end,release(ended=true)"
	tests := []struct {
		name string
		// run selects Teardown.Run over Teardown.EndSession.
		run        bool
		quiets     []quietState
		refusalErr error
		endErr     error
		seizeErr   error
		awaitErr   error
		// removeErr fails the removal.
		removeErr error
		// childLanded, battenSeeded, battenState, claim, claimFound and claimReadErr script the board-claim reads.
		childLanded  bool
		battenSeeded bool
		battenState  string
		claim        *string
		claimFound   bool
		claimReadErr error
		// gone makes the task worktree read as gone.
		gone    bool
		request Request
		// wantErrIs is a sentinel the error must wrap; wantErrContains are substrings it must hold.
		wantErrIs       error
		wantErrContains []string
		// wantAnyError requires an error without naming it.
		wantAnyError bool
		// wantCalls is the comma-joined substrate call order.
		wantCalls  string
		wantSleeps int
		// wantSession, when set, is the Ended and DriverWasLive of the returned session result.
		wantSession *SessionResult
		// wantSawGone requires the session end to have been told the task worktree is gone.
		wantSawGone bool
	}{
		{
			name:            "BusyDriverAtBoundRefusesWithoutTouchingAnything",
			quiets:          []quietState{busy},
			request:         Request{Slug: "slug", QuietWait: 3 * time.Second, RefuseWhenBusy: true},
			wantErrIs:       ErrDriverBusy,
			wantErrContains: []string{"tst:slug:driver", "lyx reed attach", "lyx reed down"},
			wantCalls:       "quiet,quiet,quiet,quiet",
			wantSleeps:      3,
		},
		{
			name:        "DriverTurningRetiringProceedsAfterCountedSleeps",
			quiets:      []quietState{busy, busy, {quiet: true}},
			request:     Request{Slug: "slug", QuietWait: time.Minute, RefuseWhenBusy: true},
			wantCalls:   "quiet,quiet,quiet,refusal,kill,seize,await,end,release(ended=true)",
			wantSleeps:  2,
			wantSession: &SessionResult{Ended: true, DriverWasLive: false},
		},
		{
			name:        "ZeroWaitWithoutRefuseProceedsAndReportsDriverWasLive",
			quiets:      []quietState{busy},
			request:     Request{Slug: "slug"},
			wantCalls:   "quiet,refusal,kill,seize,await,end,release(ended=true)",
			wantSession: &SessionResult{Ended: true, DriverWasLive: true},
		},
		{
			name:       "RefusalProbeErrorNeverEndsTheSession",
			quiets:     []quietState{{quiet: true}},
			refusalErr: refusal,
			request:    Request{Slug: "slug"},
			wantErrIs:  refusal,
			wantCalls:  "quiet,refusal",
		},
		{
			name:         "SessionEndErrorNeverRemoves",
			run:          true,
			quiets:       []quietState{{quiet: true}},
			endErr:       errors.New("tmux down"),
			request:      Request{Slug: "slug"},
			wantAnyError: true,
			wantCalls:    "quiet,refusal,kill,seize,await,end,release(ended=false)",
		},
		{
			name:      "CallsQuietProbeLoopKillEndAndRemoveInOrderThenReadsTheClaim",
			run:       true,
			quiets:    []quietState{{quiet: true}},
			request:   Request{Slug: "slug"},
			wantCalls: sessionEnd + ",landed,remove,readClaim",
		},
		{
			name:       "DoneEntryIsKept",
			run:        true,
			quiets:     []quietState{{quiet: true}},
			request:    Request{Slug: "slug"},
			claim:      text("done"),
			claimFound: true,
			wantCalls:  sessionEnd + ",landed,remove,readClaim",
		},
		{
			name:       "EntryWithoutTheRunFormIsKept",
			run:        true,
			quiets:     []quietState{{quiet: true}},
			request:    Request{Slug: "slug"},
			claim:      text("wip"),
			claimFound: true,
			wantCalls:  sessionEnd + ",landed,remove,readClaim",
		},
		{
			name:       "LandedFromTheRequestMarksDoneWithoutReadingTheChild",
			run:        true,
			quiets:     []quietState{{quiet: true}},
			request:    Request{Slug: "slug", Landed: true},
			claim:      runClaim,
			claimFound: true,
			wantCalls:  sessionEnd + ",remove,readClaim,write=done",
		},
		{
			name:        "LandedFromTheChildReadMarksDone",
			run:         true,
			quiets:      []quietState{{quiet: true}},
			request:     Request{Slug: "slug"},
			childLanded: true,
			claim:       runClaim,
			claimFound:  true,
			wantCalls:   sessionEnd + ",landed,remove,readClaim,write=done",
		},
		{
			name:       "UnseededRunIsCleared",
			run:        true,
			quiets:     []quietState{{quiet: true}},
			request:    Request{Slug: "slug"},
			claim:      runClaim,
			claimFound: true,
			wantCalls:  sessionEnd + ",landed,remove,readClaim,battenRun,write=clear",
		},
		{
			name:         "PausedBattenRunIsCleared",
			run:          true,
			quiets:       []quietState{{quiet: true}},
			request:      Request{Slug: "slug"},
			battenSeeded: true,
			battenState:  "paused",
			claim:        runClaim,
			claimFound:   true,
			wantCalls:    sessionEnd + ",landed,remove,readClaim,battenRun,write=clear",
		},
		{
			name:         "BlockedBattenRunIsCleared",
			run:          true,
			quiets:       []quietState{{quiet: true}},
			request:      Request{Slug: "slug"},
			battenSeeded: true,
			battenState:  "blocked",
			claim:        runClaim,
			claimFound:   true,
			wantCalls:    sessionEnd + ",landed,remove,readClaim,battenRun,write=clear",
		},
		{
			name:         "FailedBattenRunIsCleared",
			run:          true,
			quiets:       []quietState{{quiet: true}},
			request:      Request{Slug: "slug"},
			battenSeeded: true,
			battenState:  "failed",
			claim:        runClaim,
			claimFound:   true,
			wantCalls:    sessionEnd + ",landed,remove,readClaim,battenRun,write=clear",
		},
		{
			name:         "AwaitingBattenRunIsCleared",
			run:          true,
			quiets:       []quietState{{quiet: true}},
			request:      Request{Slug: "slug"},
			battenSeeded: true,
			battenState:  "awaiting",
			claim:        runClaim,
			claimFound:   true,
			wantCalls:    sessionEnd + ",landed,remove,readClaim,battenRun,write=clear",
		},
		{
			name:         "DoneBattenRunMarksDone",
			run:          true,
			quiets:       []quietState{{quiet: true}},
			request:      Request{Slug: "slug"},
			battenSeeded: true,
			battenState:  "done",
			claim:        runClaim,
			claimFound:   true,
			wantCalls:    sessionEnd + ",landed,remove,readClaim,battenRun,write=done",
		},
		{
			name:         "RunningBattenRunIsHeldWithNoWrite",
			run:          true,
			quiets:       []quietState{{quiet: true}},
			request:      Request{Slug: "slug"},
			battenSeeded: true,
			battenState:  "running",
			claim:        runClaim,
			claimFound:   true,
			wantCalls:    sessionEnd + ",landed,remove,readClaim,battenRun",
		},
		{
			name:         "ClaimReadFailureWritesNothing",
			run:          true,
			quiets:       []quietState{{quiet: true}},
			request:      Request{Slug: "slug"},
			claimReadErr: errors.New("board unreadable"),
			wantCalls:    sessionEnd + ",landed,remove,readClaim",
		},
		{
			name:        "GoneTaskWorktreeOverAnUnseededRunClearsWithoutReadingLanded",
			run:         true,
			quiets:      []quietState{busy},
			gone:        true,
			request:     Request{Slug: "slug", RefuseWhenBusy: true},
			claim:       runClaim,
			claimFound:  true,
			wantCalls:   "refusal,remove,readClaim,battenRun,write=clear",
			wantSawGone: true,
		},
		{
			name:         "FailedRemovalRunsNoClaimSubstrateAfterIt",
			run:          true,
			quiets:       []quietState{{quiet: true}},
			request:      Request{Slug: "slug"},
			removeErr:    errors.New("removal refused"),
			claim:        runClaim,
			claimFound:   true,
			wantAnyError: true,
			wantCalls:    sessionEnd + ",landed,remove",
		},
		{
			name:            "HeldLoopLockReadsAsBusyAndNamesTheLoop",
			quiets:          []quietState{{loop: true}},
			request:         Request{Slug: "slug", RefuseWhenBusy: true},
			wantErrIs:       ErrDriverBusy,
			wantErrContains: []string{"the loop is live", "lyx shed pause slug"},
			wantCalls:       "quiet",
		},
		{
			name:         "LoopLockSeizeFailureNeverEndsTheSession",
			quiets:       []quietState{{quiet: true}},
			seizeErr:     errors.New("the loop lock is still held"),
			request:      Request{Slug: "slug"},
			wantAnyError: true,
			wantCalls:    "quiet,refusal,kill,seize",
		},
		{
			name:         "RunLockHeldPastTheBoundNeverEndsTheSessionAndRestoresThePidFile",
			quiets:       []quietState{{quiet: true}},
			awaitErr:     errors.New("the run lock is still held by pid 42"),
			request:      Request{Slug: "slug"},
			wantAnyError: true,
			wantCalls:    "quiet,refusal,kill,seize,await,release(ended=false)",
		},
		{
			name:        "GonePairSkipsQuietWaitAndEndsByName",
			quiets:      []quietState{busy},
			gone:        true,
			request:     Request{Slug: "slug", RefuseWhenBusy: true},
			wantCalls:   "refusal",
			wantSawGone: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := &fakeSubstrates{
				quiets: tt.quiets, refusalErr: tt.refusalErr, endErr: tt.endErr, seizeErr: tt.seizeErr, awaitErr: tt.awaitErr,
				removeErr: tt.removeErr, childLanded: tt.childLanded, battenSeeded: tt.battenSeeded, battenState: tt.battenState,
				claim: tt.claim, claimFound: tt.claimFound, claimReadErr: tt.claimReadErr,
			}
			td := f.teardown()
			var sawGone bool
			if tt.gone {
				td.gone = func(string) (bool, error) { return true, nil }
				td.endSession = func(_ string, gone bool) (bool, string, error) {
					sawGone = gone
					return true, "", nil
				}
			}

			var session SessionResult
			var err error
			if tt.run {
				_, err = td.Run(context.Background(), tt.request)
			} else {
				session, err = td.EndSession(context.Background(), tt.request)
			}

			switch {
			case tt.wantErrIs != nil:
				if !errors.Is(err, tt.wantErrIs) {
					t.Fatalf("error = %v, want one wrapping %v", err, tt.wantErrIs)
				}
			case tt.wantAnyError:
				if err == nil {
					t.Fatal("error = nil, want one")
				}
			case err != nil:
				t.Fatalf("error = %v, want nil", err)
			}
			for _, want := range tt.wantErrContains {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q lacks %q", err.Error(), want)
				}
			}
			if got := strings.Join(f.calls, ","); got != tt.wantCalls {
				t.Errorf("calls = %s, want %s", got, tt.wantCalls)
			}
			if f.sleeps != tt.wantSleeps {
				t.Errorf("sleeps = %d, want %d", f.sleeps, tt.wantSleeps)
			}
			if tt.wantSession != nil && (session.Ended != tt.wantSession.Ended || session.DriverWasLive != tt.wantSession.DriverWasLive) {
				t.Errorf("session = %+v, want Ended %v and DriverWasLive %v", session, tt.wantSession.Ended, tt.wantSession.DriverWasLive)
			}
			if sawGone != tt.wantSawGone {
				t.Errorf("session end told the task worktree is gone = %v, want %v", sawGone, tt.wantSawGone)
			}
		})
	}
}

// writeInflight writes a step's in-flight record into stepsDir, and its envelope beside it when finished.
func writeInflight(t *testing.T, stepsDir, traceID string, pid int, started time.Time, finished bool) {
	t.Helper()
	record := fmt.Sprintf(`{"trace_id":%q,"pid":%d,"started_at":%q}`, traceID, pid, started.Format(time.RFC3339))
	if err := os.WriteFile(filepath.Join(stepsDir, traceID+".inflight.json"), []byte(record), 0o644); err != nil {
		t.Fatalf("write the in-flight record: %v", err)
	}
	if finished {
		if err := os.WriteFile(filepath.Join(stepsDir, traceID+".json"), []byte("{}"), 0o644); err != nil {
			t.Fatalf("write the envelope: %v", err)
		}
	}
}

// TestLoopTeardown_LockMarkAndRunLockWait drives the loop-ending helpers over real lock and pid files in a temporary steps directory, with a fake kill and sleep.
func TestLoopTeardown_LockMarkAndRunLockWait(t *testing.T) {
	t.Parallel()

	prior := shedverbs.LoopPIDRecord{
		LoopID: "loop-1",
		Loop:   proc.TreeRecord{PID: 100, StartTime: "1"},
		Child:  proc.TreeRecord{PID: 101, PGID: 101, StartTime: "2"},
	}
	// setup returns a steps directory with its pid file and lock paths, holding the prior record when withPrior is set.
	setup := func(t *testing.T, withPrior bool) (pidPath, lockPath string) {
		t.Helper()
		dir := t.TempDir()
		pidPath, lockPath = filepath.Join(dir, "loop.pid"), filepath.Join(dir, "loop.lock")
		if withPrior {
			if err := shedverbs.WriteLoopPIDRecord(pidPath, prior); err != nil {
				t.Fatalf("write the prior record: %v", err)
			}
		}
		return pidPath, lockPath
	}
	lockFree := mustRunLockFree
	readRecord := func(t *testing.T, pidPath string) (shedverbs.LoopPIDRecord, bool) {
		t.Helper()
		rec, found, err := shedverbs.ReadLoopPIDRecord(pidPath)
		if err != nil {
			t.Fatalf("ReadLoopPIDRecord: %v", err)
		}
		return rec, found
	}
	var sleeps int
	noSleep := func(context.Context, time.Duration) error { return nil }
	countingSleep := func(context.Context, time.Duration) error { sleeps++; return nil }
	noKill := func() (bool, error) { return false, nil }

	t.Run("MarkReplacesAKilledLoopRecordAndOutlivesTheSessionEnd", func(t *testing.T) {
		pidPath, lockPath := setup(t, true)
		release, err := holdLoopLock(context.Background(), noKill, pidPath, lockPath, 0, noSleep)
		if err != nil {
			t.Fatalf("holdLoopLock: %v", err)
		}
		if lockFree(t, lockPath) {
			t.Error("the loop lock is free while the teardown holds it")
		}
		if rec, _ := readRecord(t, pidPath); rec.LoopID != shedverbs.TeardownLoopID || rec.Child != (proc.TreeRecord{}) || rec.Loop != (proc.TreeRecord{}) {
			t.Errorf("pid file = %+v; want the teardown's mark naming no loop process and no child", rec)
		}
		if err := release(true); err != nil {
			t.Fatalf("release(true): %v", err)
		}
		if rec, _ := readRecord(t, pidPath); rec.LoopID != shedverbs.TeardownLoopID {
			t.Errorf("pid file after the session end = %+v; want the mark kept", rec)
		}
		if !lockFree(t, lockPath) {
			t.Error("the loop lock is still held after release")
		}
	})

	t.Run("MarkIsWrittenWhenNoRecordWasThere", func(t *testing.T) {
		pidPath, lockPath := setup(t, false)
		release, err := holdLoopLock(context.Background(), noKill, pidPath, lockPath, 0, noSleep)
		if err != nil {
			t.Fatalf("holdLoopLock: %v", err)
		}
		if err := release(true); err != nil {
			t.Fatalf("release(true): %v", err)
		}
		if rec, found := readRecord(t, pidPath); !found || rec.LoopID != shedverbs.TeardownLoopID || rec.Child != (proc.TreeRecord{}) {
			t.Errorf("pid file = %+v, found %v; want the mark with no child", rec, found)
		}
	})

	t.Run("FailedSessionEndPutsBackThePriorRecordOrRemovesTheMark", func(t *testing.T) {
		pidPath, lockPath := setup(t, true)
		release, err := holdLoopLock(context.Background(), noKill, pidPath, lockPath, 0, noSleep)
		if err != nil {
			t.Fatalf("holdLoopLock: %v", err)
		}
		if err := release(false); err != nil {
			t.Fatalf("release(false): %v", err)
		}
		if rec, _ := readRecord(t, pidPath); rec != prior {
			t.Errorf("pid file = %+v; want the prior record %+v back", rec, prior)
		}

		pidPath, lockPath = setup(t, false)
		release, err = holdLoopLock(context.Background(), noKill, pidPath, lockPath, 0, noSleep)
		if err != nil {
			t.Fatalf("holdLoopLock: %v", err)
		}
		if err := release(false); err != nil {
			t.Fatalf("release(false): %v", err)
		}
		if _, found := readRecord(t, pidPath); found {
			t.Error("the mark is still in place after a failed session end with no prior record")
		}
		if !lockFree(t, lockPath) {
			t.Error("the loop lock is still held after release")
		}
	})

	t.Run("LoopRespawnedAfterTheFirstKillIsKilledAgainBeforeTheSeizeSucceeds", func(t *testing.T) {
		pidPath, lockPath := setup(t, true)
		blocker := mustHoldLock(t, lockPath)
		kills, before := 0, sleeps
		kill := func() (bool, error) {
			kills++
			if kills == 2 {
				if err := blocker.Release(); err != nil {
					t.Errorf("release the respawned loop's lock: %v", err)
				}
			}
			return true, nil
		}
		release, err := holdLoopLock(context.Background(), kill, pidPath, lockPath, 5, countingSleep)
		if err != nil {
			t.Fatalf("holdLoopLock: %v", err)
		}
		if err := release(true); err != nil {
			t.Fatalf("release(true): %v", err)
		}
		if kills != 2 || sleeps-before != 1 {
			t.Errorf("kills = %d, sleeps = %d; want the loop killed twice around one sleep", kills, sleeps-before)
		}
	})

	t.Run("LoopLockHeldPastTheBoundFailsNamingItAndLeavesThePidFile", func(t *testing.T) {
		pidPath, lockPath := setup(t, true)
		blocker := mustHoldLock(t, lockPath)
		defer blocker.Release()
		kills := 0
		kill := func() (bool, error) { kills++; return true, nil }
		_, err := holdLoopLock(context.Background(), kill, pidPath, lockPath, 2, noSleep)
		if err == nil || !strings.Contains(err.Error(), lockPath) {
			t.Fatalf("holdLoopLock error = %v; want one naming the loop lock %s", err, lockPath)
		}
		if kills != 3 {
			t.Errorf("kills = %d; want one per attempt, 3", kills)
		}
		if rec, _ := readRecord(t, pidPath); rec != prior {
			t.Errorf("pid file = %+v; want the prior record untouched", rec)
		}
	})

	t.Run("AbsentPidFileAndTheMarkKillNothing", func(t *testing.T) {
		pidPath, lockPath := setup(t, false)
		if killed, err := killLoop(pidPath, lockPath, "job"); killed || err != nil {
			t.Errorf("killLoop with no pid file = %v, %v; want false, nil", killed, err)
		}
		if err := shedverbs.WriteLoopPIDRecord(pidPath, shedverbs.LoopPIDRecord{LoopID: shedverbs.TeardownLoopID}); err != nil {
			t.Fatalf("write the mark: %v", err)
		}
		if killed, err := killLoop(pidPath, lockPath, "job"); killed || err != nil {
			t.Errorf("killLoop over the mark = %v, %v; want false, nil", killed, err)
		}
	})

	t.Run("RunLockHeldPastTheBoundNamesTheNewestUnfinishedStep", func(t *testing.T) {
		stepsDir := t.TempDir()
		lockPath := filepath.Join(stepsDir, "run.lock")
		holder := mustHoldLock(t, lockPath)
		defer holder.Release()

		err := awaitRunLockNamingHolder(context.Background(), lockPath, stepsDir, "slug", 1, noSleep)
		if !errors.Is(err, errRunLockHeld) || !strings.Contains(err.Error(), "an unknown process") || !strings.Contains(err.Error(), "retry the removal") {
			t.Errorf("error with no step record = %v; want the held sentinel naming an unknown holder and the way forward", err)
		}

		base := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
		writeInflight(t, stepsDir, "older", 7, base, false)
		writeInflight(t, stepsDir, "newer", 8, base.Add(time.Minute), false)
		writeInflight(t, stepsDir, "finished", 9, base.Add(time.Hour), true)
		err = awaitRunLockNamingHolder(context.Background(), lockPath, stepsDir, "slug", 1, noSleep)
		if !errors.Is(err, errRunLockHeld) || !strings.Contains(err.Error(), "pid 8") {
			t.Errorf("error with step records = %v; want the held sentinel naming pid 8", err)
		}
	})

	t.Run("RunLockFreeAtOnceNeverSleeps", func(t *testing.T) {
		stepsDir := t.TempDir()
		before := sleeps
		if err := awaitRunLockFree(context.Background(), filepath.Join(stepsDir, "run.lock"), 3, countingSleep); err != nil {
			t.Fatalf("awaitRunLockFree: %v", err)
		}
		if sleeps != before {
			t.Errorf("sleeps = %d; want none for a free lock", sleeps-before)
		}
	})
}

// mustHoldLock takes the exclusive lock at lockPath and fails the test when it cannot.
func mustHoldLock(t *testing.T, lockPath string) *lock.FileLock {
	t.Helper()
	held, ok, err := lock.TryAcquireWriteLock(lockPath)
	if err != nil || !ok {
		t.Fatalf("TryAcquireWriteLock(%s) = %v, %v; want the lock", lockPath, ok, err)
	}
	return held
}

// mustRunLockFree reports whether the lock at lockPath is free.
func mustRunLockFree(t *testing.T, lockPath string) bool {
	t.Helper()
	free, err := runLockFree(lockPath)
	if err != nil {
		t.Fatalf("runLockFree(%s): %v", lockPath, err)
	}
	return free
}
