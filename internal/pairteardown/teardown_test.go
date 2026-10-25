//go:build tier0

// teardown_test.go drives the teardown sequence through fakes, so it spawns nothing.

package pairteardown

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/runproc"
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
	// snapshotErr fails the snapshot step.
	snapshotErr error
	// childLanded is what the landed read answers.
	childLanded bool
	// battenSeeded and battenState are what batten's run reads answer.
	battenSeeded bool
	battenState  string
	// claim is the board entry's status when claimFound; claimReadErr fails the read.
	claim        *string
	claimFound   bool
	claimReadErr error

	// endedProcs and endOwnedErr are what the owner-record end answers; spared is the roles its last call spared.
	endedProcs  []runproc.Ended
	endOwnedErr error
	spared      []string
	// parent is the slug's parent, empty for the prime; cancelMark is whether a cancel marker stands.
	parent     string
	cancelMark bool
	// ownedUnder answers the live owned processes of the owners it names, ahead of the owned list.
	ownedUnder map[string][]runproc.Record
	// driverLockHeld is whether the driver lock reads held after the owned end.
	driverLockHeld bool
	// battenAlive is whether a registered batten process reads alive.
	battenAlive bool
	// driverPID is the pid of the registered driver, when driverFound.
	driverPID   int
	driverFound bool

	// paused and hubEntry are the pause readings; pausedErr fails the read.
	paused    bool
	hubEntry  bool
	pausedErr error
	// awaitOrPassErr is what the pre-end run-lock wait answers; awaitSeen is what it was last called with.
	awaitOrPassErr error
	awaitSeen      awaitCall
	// owned and ownedErr are the slug's live owned processes a busy refusal names.
	owned    []runproc.Record
	ownedErr error

	// gone is what the gone substrate answers.
	gone bool
	// strands is what the strand stop answers as the stopped names.
	strands []string
	// writeLockBusy makes the records write lock hold refuse.
	writeLockBusy bool
	// driverLockHeldByProbe answers each driver-lock probe in turn, ahead of the driverLockHeld flag.
	driverLockHeldByProbe []bool
}

// awaitCall is one call of the pre-end run-lock wait.
type awaitCall struct {
	paused, hubEntry bool
	retry            string
}

func (f *fakeSubstrates) teardown() *Teardown {
	return &Teardown{
		prime:        &lyxcwd.Location{HubPath: "/hub", AnchorRel: "."},
		interval:     time.Second,
		goos:         "linux",
		parentOf:     func(string) (string, error) { return f.parent, nil },
		cancelMarked: func(string) bool { return f.cancelMark },
		endOwned: func(_ string, spare ...string) ([]runproc.Ended, error) {
			f.calls = append(f.calls, "endOwned")
			f.spared = spare
			return f.endedProcs, f.endOwnedErr
		},
		driverLockHeld: func(string) (bool, error) {
			f.calls = append(f.calls, "driverLock")
			if len(f.driverLockHeldByProbe) > 0 {
				held := f.driverLockHeldByProbe[0]
				f.driverLockHeldByProbe = f.driverLockHeldByProbe[1:]
				return held, nil
			}
			return f.driverLockHeld, nil
		},
		holdWriteLocks: func(slug string) (func() error, error) {
			f.calls = append(f.calls, "holdWriteLocks")
			if f.writeLockBusy {
				return nil, writeLockBusyError(slug, "/hub/records/lock")
			}
			return func() error {
				f.calls = append(f.calls, "release")
				return nil
			}, nil
		},
		stopStrands: func(string) ([]string, error) {
			f.calls = append(f.calls, "stopStrands")
			return f.strands, nil
		},
		battenAlive: func(string) (bool, error) { return f.battenAlive, nil },
		driverPID:   func(string) (int, bool) { return f.driverPID, f.driverFound },
		paused:      func(string) (bool, bool, error) { return f.paused, f.hubEntry, f.pausedErr },
		awaitOrPass: func(_ context.Context, _ string, paused, hubEntry bool, retry string) error {
			f.calls = append(f.calls, "awaitOrPass")
			f.awaitSeen = awaitCall{paused: paused, hubEntry: hubEntry, retry: retry}
			return f.awaitOrPassErr
		},
		ownedProcesses: func(owner string) ([]runproc.Record, error) {
			if records, ok := f.ownedUnder[owner]; ok {
				return records, f.ownedErr
			}
			return f.owned, f.ownedErr
		},
		gone: func(string) (bool, error) { return f.gone, nil },
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
		archive: func(string) { f.calls = append(f.calls, "archive") },
		remove: func(req Request) (fabricengine.RemoveResult, error) {
			if req.Local {
				f.calls = append(f.calls, "remove-local")
			} else {
				f.calls = append(f.calls, "remove")
			}
			return fabricengine.RemoveResult{}, f.removeErr
		},
		snapshot: func(*fabricengine.Mutations, string) (string, []string, error) {
			f.calls = append(f.calls, "snapshot")
			if f.snapshotErr != nil {
				return "", nil, f.snapshotErr
			}
			return "set-aside/slug/abc", []string{"task/a.txt"}, nil
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
	// reads are the skip readings an unpaused, unended pair with no done entry and no landed run makes before its quiet wait.
	const reads = "readClaim,landed,"
	const sessionEnd = reads + "quiet,refusal,endOwned,driverLock,kill,seize,await,end,release(ended=true)"
	const skippedEnd = "awaitOrPass,refusal,endOwned,driverLock,kill,seize,await,end,release(ended=true)"
	survivors := []runproc.Ended{{PID: 4242, Role: runproc.RoleLoop}}
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
		// snapshotErr fails the snapshot step; afterSnapshotErr fails the request's AfterSnapshot, which is set whenever either is wanted.
		snapshotErr       error
		afterSnapshotErr  error
		withAfterSnapshot bool
		// wantSnapshotTag is the SnapshotTag the removal result must carry.
		wantSnapshotTag string
		// childLanded, battenSeeded, battenState, claim, claimFound and claimReadErr script the board-claim reads.
		childLanded  bool
		battenSeeded bool
		battenState  string
		claim        *string
		claimFound   bool
		claimReadErr error
		// endedProcs and endOwnedErr script the owner-record end; driverLockHeld, goos, driverPID and driverFound script the driver-lock re-check; battenAlive scripts the registered batten read.
		endedProcs     []runproc.Ended
		endOwnedErr    error
		driverLockHeld bool
		goos           string
		driverPID      int
		driverFound    bool
		battenAlive    bool
		// paused, hubEntry and pausedErr script the pause reading; awaitOrPassErr scripts the pre-end run-lock wait; owned scripts the processes a busy refusal names.
		paused         bool
		hubEntry       bool
		pausedErr      error
		awaitOrPassErr error
		owned          []runproc.Record
		// wantAwait, when set, is what the pre-end run-lock wait must have been called with.
		wantAwait *awaitCall
		// wantEnded and wantSurvivors are the EndedProcesses and Survivors of the returned session result.
		wantEnded     []runproc.Ended
		wantSurvivors []runproc.Ended
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
			owned:           []runproc.Record{{PID: 4100, Role: runproc.RoleDriver}, {PID: 4101, Role: runproc.RolePush}},
			wantErrContains: []string{"tst:slug:driver", "lyx reed attach", "lyx tender pause slug", "4100 (driver)", "4101 (push)"},
			wantCalls:       reads + "quiet,quiet,quiet,quiet",
			wantSleeps:      3,
		},
		{
			name:        "DriverTurningRetiringProceedsAfterCountedSleeps",
			quiets:      []quietState{busy, busy, {quiet: true}},
			request:     Request{Slug: "slug", QuietWait: time.Minute, RefuseWhenBusy: true},
			wantCalls:   reads + "quiet,quiet,quiet,refusal,endOwned,driverLock,kill,seize,await,end,release(ended=true)",
			wantSleeps:  2,
			wantSession: &SessionResult{Ended: true, DriverWasLive: false},
		},
		{
			name:        "ZeroWaitWithoutRefuseProceedsAndReportsDriverWasLive",
			quiets:      []quietState{busy},
			request:     Request{Slug: "slug"},
			wantCalls:   sessionEnd,
			wantSession: &SessionResult{Ended: true, DriverWasLive: true},
		},
		{
			name:       "RefusalProbeErrorNeverEndsTheSession",
			quiets:     []quietState{{quiet: true}},
			refusalErr: refusal,
			request:    Request{Slug: "slug"},
			wantErrIs:  refusal,
			wantCalls:  reads + "quiet,refusal",
		},
		{
			name:         "SessionEndErrorNeverRemoves",
			run:          true,
			quiets:       []quietState{{quiet: true}},
			endErr:       errors.New("tmux down"),
			request:      Request{Slug: "slug"},
			wantAnyError: true,
			wantCalls:    reads + "quiet,refusal,endOwned,driverLock,kill,seize,await,end,release(ended=false)",
		},
		{
			name:      "CallsQuietProbeLoopKillEndAndRemoveInOrderThenReadsTheClaim",
			run:       true,
			quiets:    []quietState{{quiet: true}},
			request:   Request{Slug: "slug"},
			wantCalls: sessionEnd + ",landed,archive,remove,readClaim",
		},
		{
			name:        "DoneEntryIsKeptAndSkipsTheDriverChecksWhateverBattenReads",
			run:         true,
			quiets:      []quietState{{quiet: true}},
			request:     Request{Slug: "slug"},
			claim:       text("done"),
			claimFound:  true,
			battenAlive: true,
			wantCalls:   "readClaim," + skippedEnd + ",landed,archive,remove,readClaim",
		},
		{
			name:       "EntryWithoutTheRunFormIsKept",
			run:        true,
			quiets:     []quietState{{quiet: true}},
			request:    Request{Slug: "slug"},
			claim:      text("wip"),
			claimFound: true,
			wantCalls:  sessionEnd + ",landed,archive,remove,readClaim",
		},
		{
			name:       "LandedFromTheRequestMarksDoneWithoutReadingTheChild",
			run:        true,
			quiets:     []quietState{{quiet: true}},
			request:    Request{Slug: "slug", Landed: true},
			claim:      runClaim,
			claimFound: true,
			wantCalls:  sessionEnd + ",archive,remove,readClaim,write=done",
		},
		{
			name:        "LandedFromTheChildReadMarksDone",
			run:         true,
			quiets:      []quietState{{quiet: true}},
			request:     Request{Slug: "slug"},
			childLanded: true,
			battenAlive: true,
			claim:       runClaim,
			claimFound:  true,
			wantCalls:   sessionEnd + ",landed,archive,remove,readClaim,write=done",
		},
		{
			name:       "UnseededRunIsCleared",
			run:        true,
			quiets:     []quietState{{quiet: true}},
			request:    Request{Slug: "slug"},
			claim:      runClaim,
			claimFound: true,
			wantCalls:  sessionEnd + ",landed,archive,remove,readClaim,battenRun,write=draft",
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
			wantCalls:    sessionEnd + ",landed,archive,remove,readClaim,battenRun,write=draft",
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
			wantCalls:    sessionEnd + ",landed,archive,remove,readClaim,battenRun,write=draft",
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
			wantCalls:    sessionEnd + ",landed,archive,remove,readClaim,battenRun,write=draft",
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
			wantCalls:    sessionEnd + ",landed,archive,remove,readClaim,battenRun,write=draft",
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
			wantCalls:    sessionEnd + ",landed,archive,remove,readClaim,battenRun,write=done",
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
			wantCalls:    sessionEnd + ",landed,archive,remove,readClaim,battenRun",
		},
		{
			name:         "ClaimReadFailureAfterTheRemovalWritesNothing",
			run:          true,
			quiets:       []quietState{{quiet: true}},
			request:      Request{Slug: "slug", Verdict: fabricengine.EntryEnded},
			claimReadErr: errors.New("board unreadable"),
			wantCalls:    skippedEnd + ",landed,archive,remove,readClaim",
		},
		{
			name:         "ClaimReadFailureInTheSkipReadingIsReturnedWithNothingEnded",
			run:          true,
			quiets:       []quietState{{quiet: true}},
			request:      Request{Slug: "slug"},
			claimReadErr: errors.New("board unreadable"),
			wantAnyError: true,
			wantCalls:    "readClaim",
		},
		{
			name:        "GoneTaskWorktreeOverAnUnseededRunClearsWithoutReadingLanded",
			run:         true,
			quiets:      []quietState{busy},
			gone:        true,
			request:     Request{Slug: "slug", RefuseWhenBusy: true},
			claim:       runClaim,
			claimFound:  true,
			wantCalls:   "refusal,endOwned,end,archive,remove,readClaim,battenRun,write=draft",
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
			wantCalls:    sessionEnd + ",landed,archive,remove",
		},
		{
			name:            "HeldLoopLockReadsAsBusyAndNamesTheLoop",
			quiets:          []quietState{{loop: true}},
			request:         Request{Slug: "slug", RefuseWhenBusy: true},
			wantErrIs:       ErrDriverBusy,
			wantErrContains: []string{"the loop is live", "lyx shed pause slug --after <current_producer>"},
			wantCalls:       reads + "quiet",
		},
		{
			name:         "LoopLockSeizeFailureNeverEndsTheSession",
			quiets:       []quietState{{quiet: true}},
			seizeErr:     errors.New("the loop lock is still held"),
			request:      Request{Slug: "slug"},
			wantAnyError: true,
			wantCalls:    reads + "quiet,refusal,endOwned,driverLock,kill,seize",
		},
		{
			name:         "RunLockHeldPastTheBoundNeverEndsTheSessionAndRestoresThePidFile",
			quiets:       []quietState{{quiet: true}},
			awaitErr:     errors.New("the run lock is still held by pid 42"),
			request:      Request{Slug: "slug"},
			wantAnyError: true,
			wantCalls:    reads + "quiet,refusal,endOwned,driverLock,kill,seize,await,release(ended=false)",
		},
		{
			name:        "GonePairSkipsQuietWaitAndEndsByName",
			quiets:      []quietState{busy},
			gone:        true,
			request:     Request{Slug: "slug", RefuseWhenBusy: true},
			wantCalls:   "refusal,endOwned,end",
			wantSawGone: true,
		},
		{
			name:            "SurvivorsUnderRefuseWhenBusyRefuseNamingThePidWithNoLoopEndAndNoSessionEnd",
			quiets:          []quietState{{quiet: true}},
			endedProcs:      []runproc.Ended{{PID: 4100, Role: runproc.RolePush}},
			endOwnedErr:     &runproc.SurvivorsError{Survivors: survivors},
			request:         Request{Slug: "slug", RefuseWhenBusy: true},
			wantErrIs:       ErrDriverBusy,
			wantErrContains: []string{"4242", runproc.RoleLoop, "retry the removal"},
			wantCalls:       reads + "quiet,refusal,endOwned",
			wantSession:     &SessionResult{},
			wantEnded:       []runproc.Ended{{PID: 4100, Role: runproc.RolePush}},
			wantSurvivors:   survivors,
		},
		{
			name:          "SurvivorsWithoutRefuseWhenBusyGoOnToTheSessionEndAndAreReported",
			quiets:        []quietState{{quiet: true}},
			endOwnedErr:   &runproc.SurvivorsError{Survivors: survivors},
			request:       Request{Slug: "slug"},
			wantCalls:     sessionEnd,
			wantSession:   &SessionResult{Ended: true},
			wantSurvivors: survivors,
		},
		{
			name:         "OwnedEndErrorThatIsNotSurvivorsIsReturnedForEveryCaller",
			quiets:       []quietState{{quiet: true}},
			endOwnedErr:  errors.New("record directory unreadable"),
			request:      Request{Slug: "slug"},
			wantAnyError: true,
			wantCalls:    reads + "quiet,refusal,endOwned",
		},
		{
			name:        "EndedProcessesAreCarriedOnTheSessionResult",
			quiets:      []quietState{{quiet: true}},
			endedProcs:  []runproc.Ended{{PID: 4100, Role: runproc.RoleDriver}},
			request:     Request{Slug: "slug"},
			wantCalls:   sessionEnd,
			wantSession: &SessionResult{Ended: true},
			wantEnded:   []runproc.Ended{{PID: 4100, Role: runproc.RoleDriver}},
		},
		{
			name:            "DriverLockStillHeldUnderRefuseWhenBusyRefusesOnLinuxNamingTenderStatus",
			quiets:          []quietState{{quiet: true}},
			driverLockHeld:  true,
			request:         Request{Slug: "slug", RefuseWhenBusy: true},
			wantErrIs:       ErrDriverBusy,
			wantErrContains: []string{"lyx tender status"},
			wantCalls:       reads + "quiet,refusal,endOwned,driverLock",
		},
		{
			name:            "DriverLockStillHeldOnWindowsNamesTaskkillWithTheRecordedDriverPid",
			quiets:          []quietState{{quiet: true}},
			driverLockHeld:  true,
			goos:            "windows",
			driverPID:       777,
			driverFound:     true,
			request:         Request{Slug: "slug", RefuseWhenBusy: true},
			wantErrIs:       ErrDriverBusy,
			wantErrContains: []string{"taskkill /PID 777 /T /F"},
			wantCalls:       reads + "quiet,refusal,endOwned,driverLock",
		},
		{
			name:            "DriverLockStillHeldOnWindowsWithNoDriverRecordNamesTheLockPath",
			quiets:          []quietState{{quiet: true}},
			driverLockHeld:  true,
			goos:            "windows",
			request:         Request{Slug: "slug", RefuseWhenBusy: true},
			wantErrIs:       ErrDriverBusy,
			wantErrContains: []string{"Task Manager", "driver.lock"},
			wantCalls:       reads + "quiet,refusal,endOwned,driverLock",
		},
		{
			name:           "DriverLockStillHeldWithoutRefuseWhenBusyGoesOnAndReportsDriverWasLive",
			quiets:         []quietState{{quiet: true}},
			driverLockHeld: true,
			request:        Request{Slug: "slug"},
			wantCalls:      sessionEnd,
			wantSession:    &SessionResult{Ended: true, DriverWasLive: true},
		},
		{
			name:       "LocalModeRemovesLocallyAndNeverTouchesTheBoard",
			run:        true,
			quiets:     []quietState{{quiet: true}},
			request:    Request{Slug: "slug", Local: true, RefuseWhenBusy: true},
			claim:      runClaim,
			claimFound: true,
			wantCalls:  sessionEnd + ",archive,remove-local",
		},
		{
			name:              "LocalSnapshotRunsSnapshotThenAfterSnapshotThenRemove",
			run:               true,
			quiets:            []quietState{{quiet: true}},
			request:           Request{Slug: "slug", Local: true, Snapshot: true},
			withAfterSnapshot: true,
			wantCalls:         sessionEnd + ",snapshot,after,archive,remove-local",
			wantSnapshotTag:   "set-aside/slug/abc",
		},
		{
			name:              "OriginSnapshotRunsBeforeArchiveAndRemoveAndCarriesTheTag",
			run:               true,
			quiets:            []quietState{{quiet: true}},
			request:           Request{Slug: "slug", Snapshot: true},
			withAfterSnapshot: true,
			wantCalls:         sessionEnd + ",snapshot,after,landed,archive,remove,readClaim",
			wantSnapshotTag:   "set-aside/slug/abc",
		},
		{
			name:        "OriginSnapshotRefusedRemovesNothing",
			run:         true,
			quiets:      []quietState{{quiet: true}},
			request:     Request{Slug: "slug", Snapshot: true},
			snapshotErr: fabricengine.ErrPairBusy,
			wantErrIs:   fabricengine.ErrPairBusy,
			wantCalls:   sessionEnd + ",snapshot",
		},
		{
			name:       "LeaveClaimReadsNoLandedAnswerAndSettlesNoClaim",
			run:        true,
			quiets:     []quietState{{quiet: true}},
			request:    Request{Slug: "slug", Verdict: fabricengine.EntryEnded, LeaveClaim: true},
			claim:      runClaim,
			claimFound: true,
			wantCalls:  skippedEnd + ",archive,remove",
		},
		{
			name:              "SnapshotRefusedBusyStopsBeforeAfterSnapshotAndRemove",
			run:               true,
			quiets:            []quietState{{quiet: true}},
			request:           Request{Slug: "slug", Local: true, Snapshot: true},
			withAfterSnapshot: true,
			snapshotErr:       fabricengine.ErrPairBusy,
			wantErrIs:         fabricengine.ErrPairBusy,
			wantCalls:         sessionEnd + ",snapshot",
		},
		{
			name:              "AfterSnapshotErrorStopsBeforeRemoveAndKeepsTheTag",
			run:               true,
			quiets:            []quietState{{quiet: true}},
			request:           Request{Slug: "slug", Local: true, Snapshot: true},
			withAfterSnapshot: true,
			afterSnapshotErr:  errors.New("lift refused"),
			wantAnyError:      true,
			wantCalls:         sessionEnd + ",snapshot,after",
			wantSnapshotTag:   "set-aside/slug/abc",
		},
		{
			name:      "LocalModeBusyDriverWithRefuseWhenBusyEndsNothing",
			run:       true,
			quiets:    []quietState{busy},
			request:   Request{Slug: "slug", Local: true, RefuseWhenBusy: true},
			wantErrIs: ErrDriverBusy,
			wantCalls: reads + "quiet",
		},
		{
			name:       "LocalRemoveBusySurfacesWithTheBoardUntouched",
			run:        true,
			quiets:     []quietState{{quiet: true}},
			request:    Request{Slug: "slug", Local: true},
			removeErr:  fabricengine.ErrPairBusy,
			claim:      runClaim,
			claimFound: true,
			wantErrIs:  fabricengine.ErrPairBusy,
			wantCalls:  sessionEnd + ",archive,remove-local",
		},
		{
			name:       "ForceCaseOverAMergeInProgressRefusalEndsNothingAndSnapshotsNothing",
			run:        true,
			quiets:     []quietState{{quiet: true}},
			refusalErr: refusal,
			request:    Request{Slug: "slug", Local: true, Snapshot: true, Force: true},
			wantErrIs:  refusal,
			wantCalls:  reads + "quiet,refusal",
		},
		{
			name:      "EndedVerdictSkipsTheQuietWaitAndWaitsForTheRunLockFirst",
			quiets:    []quietState{busy},
			request:   Request{Slug: "slug", Verdict: fabricengine.EntryEnded, RefuseWhenBusy: true},
			wantCalls: skippedEnd,
			wantAwait: &awaitCall{retry: "the removal"},
		},
		{
			name:      "OwnedElsewhereVerdictSkipsTheQuietWait",
			quiets:    []quietState{busy},
			request:   Request{Slug: "slug", Verdict: fabricengine.EntryOwnedElsewhere, RefuseWhenBusy: true},
			wantCalls: skippedEnd,
			wantAwait: &awaitCall{retry: "the removal"},
		},
		{
			name:      "SlugPauseSkipsTheQuietWaitAndPassesNoHubEntry",
			quiets:    []quietState{busy},
			paused:    true,
			request:   Request{Slug: "slug", RefuseWhenBusy: true},
			wantCalls: skippedEnd,
			wantAwait: &awaitCall{paused: true, retry: "the removal"},
		},
		{
			name:      "HubPauseSkipsTheQuietWaitAndPassesTheHubEntry",
			quiets:    []quietState{busy},
			paused:    true,
			hubEntry:  true,
			request:   Request{Slug: "slug", RefuseWhenBusy: true},
			wantCalls: skippedEnd,
			wantAwait: &awaitCall{paused: true, hubEntry: true, retry: "the removal"},
		},
		{
			name:        "FinalizeDoneWithNoLiveBattenSkipsTheQuietWait",
			quiets:      []quietState{busy},
			childLanded: true,
			request:     Request{Slug: "slug", RefuseWhenBusy: true},
			wantCalls:   "readClaim,landed," + skippedEnd,
		},
		{
			name:           "RunLockRefusalFromTheWaitIsReturnedWithNothingEnded",
			quiets:         []quietState{busy},
			awaitOrPassErr: fmt.Errorf("%w: the run lock is held", ErrDriverBusy),
			request:        Request{Slug: "slug", Verdict: fabricengine.EntryEnded, RefuseWhenBusy: true},
			wantErrIs:      ErrDriverBusy,
			wantCalls:      "awaitOrPass",
		},
		{
			name:       "RefusalProbeInTheSkippedOrderEndsNoOwnedProcess",
			quiets:     []quietState{busy},
			refusalErr: refusal,
			request:    Request{Slug: "slug", Verdict: fabricengine.EntryEnded},
			wantErrIs:  refusal,
			wantCalls:  "awaitOrPass,refusal",
		},
		{
			name:         "SkipReadingErrorIsReturnedWithNothingEnded",
			quiets:       []quietState{busy},
			pausedErr:    errors.New("pause record unreadable"),
			request:      Request{Slug: "slug", RefuseWhenBusy: true},
			wantAnyError: true,
			wantCalls:    "",
		},
		{
			name:        "GoneTaskWorktreeMakesNoSkipReading",
			quiets:      []quietState{busy},
			gone:        true,
			pausedErr:   errors.New("pause record unreadable"),
			request:     Request{Slug: "slug", Verdict: fabricengine.EntryEnded, RefuseWhenBusy: true},
			wantCalls:   "refusal,endOwned,end",
			wantSawGone: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := &fakeSubstrates{
				quiets: tt.quiets, refusalErr: tt.refusalErr, endErr: tt.endErr, seizeErr: tt.seizeErr, awaitErr: tt.awaitErr,
				removeErr: tt.removeErr, childLanded: tt.childLanded, battenSeeded: tt.battenSeeded, battenState: tt.battenState,
				claim: tt.claim, claimFound: tt.claimFound, claimReadErr: tt.claimReadErr, snapshotErr: tt.snapshotErr,
				endedProcs: tt.endedProcs, endOwnedErr: tt.endOwnedErr, driverLockHeld: tt.driverLockHeld,
				driverPID: tt.driverPID, driverFound: tt.driverFound, battenAlive: tt.battenAlive,
				paused: tt.paused, hubEntry: tt.hubEntry, pausedErr: tt.pausedErr, awaitOrPassErr: tt.awaitOrPassErr, owned: tt.owned,
			}
			td := f.teardown()
			if tt.goos != "" {
				td.goos = tt.goos
			}
			if tt.withAfterSnapshot {
				tt.request.AfterSnapshot = func() error {
					f.calls = append(f.calls, "after")
					return tt.afterSnapshotErr
				}
			}
			var sawGone bool
			if tt.gone {
				td.gone = func(string) (bool, error) { return true, nil }
				td.endSession = func(_ string, gone bool) (bool, string, error) {
					f.calls = append(f.calls, "end")
					sawGone = gone
					return true, "", nil
				}
			}

			var session SessionResult
			var err error
			var removal fabricengine.RemoveResult
			if tt.run {
				var result Result
				result, err = td.Run(context.Background(), tt.request)
				removal = result.Removal
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
			if removal.SnapshotTag != tt.wantSnapshotTag {
				t.Errorf("SnapshotTag = %q, want %q", removal.SnapshotTag, tt.wantSnapshotTag)
			}
			if f.sleeps != tt.wantSleeps {
				t.Errorf("sleeps = %d, want %d", f.sleeps, tt.wantSleeps)
			}
			if tt.wantSession != nil && (session.Ended != tt.wantSession.Ended || session.DriverWasLive != tt.wantSession.DriverWasLive) {
				t.Errorf("session = %+v, want Ended %v and DriverWasLive %v", session, tt.wantSession.Ended, tt.wantSession.DriverWasLive)
			}
			if tt.wantSession != nil {
				if !slices.Equal(session.EndedProcesses, tt.wantEnded) {
					t.Errorf("EndedProcesses = %v, want %v", session.EndedProcesses, tt.wantEnded)
				}
				if !slices.Equal(session.Survivors, tt.wantSurvivors) {
					t.Errorf("Survivors = %v, want %v", session.Survivors, tt.wantSurvivors)
				}
			}
			if tt.wantAwait != nil && f.awaitSeen != *tt.wantAwait {
				t.Errorf("awaitOrPass called with %+v, want %+v", f.awaitSeen, *tt.wantAwait)
			}
			if sawGone != tt.wantSawGone {
				t.Errorf("session end told the task worktree is gone = %v, want %v", sawGone, tt.wantSawGone)
			}
		})
	}
}

// TestSettleBoardClaim_RunningBattenRecordWayForward pins which way forward the kept claim of a running batten record warns with:
// batten's own pause while a registered batten is alive, and batten's cancel once that process is gone.
// It captures the process-global logger, so it is not parallel.
func TestSettleBoardClaim_RunningBattenRecordWayForward(t *testing.T) {
	tests := []struct {
		name        string
		battenAlive bool
		want        string
		notWant     string
	}{
		{"LiveRegisteredBattenKeepsThePauseWayForward", true, "lyx batten pause slug", "lyx batten cancel"},
		{"NoLiveRegisteredBattenNamesTheCancelVerb", false, "lyx batten cancel slug", "lyx board set-status"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf strings.Builder
			logger.SetOutput(&buf)
			t.Cleanup(func() { logger.SetOutput(logger.DefaultOutput()) })

			running := boardengine.RunStatus("running", "Webster-Burler")
			f := &fakeSubstrates{battenSeeded: true, battenState: "running", claim: &running, claimFound: true, battenAlive: tt.battenAlive}
			f.teardown().settleBoardClaim("slug", false)

			if !strings.Contains(buf.String(), tt.want) {
				t.Errorf("log = %q, want it to name %q", buf.String(), tt.want)
			}
			if strings.Contains(buf.String(), tt.notWant) {
				t.Errorf("log = %q, want it not to name %q", buf.String(), tt.notWant)
			}
			if slices.Contains(f.calls, "write=clear") || slices.Contains(f.calls, "write=draft") || slices.Contains(f.calls, "write=done") {
				t.Errorf("calls = %v, want the claim kept", f.calls)
			}
		})
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

// TestRegisteredBattenAlive_ReadsADobbyDrivenRun pins that the gap between two batten passes reads as a live lifecycle owner only while a dobby of the run's parent stands and batten's status reads running with no cancel marker.
func TestRegisteredBattenAlive_ReadsADobbyDrivenRun(t *testing.T) {
	t.Parallel()

	dobby := []runproc.Record{{PID: 7, Role: runproc.RoleDobby}}
	tests := []struct {
		name string
		f    fakeSubstrates
		want bool
	}{
		{"RunningStatusWithADobbyUnderTheParentIsAlive", fakeSubstrates{battenSeeded: true, battenState: "running", parent: "beam", ownedUnder: map[string][]runproc.Record{"beam": dobby}}, true},
		{"RunningStatusWithADobbyUnderTheHubIsAlive", fakeSubstrates{battenSeeded: true, battenState: "running", ownedUnder: map[string][]runproc.Record{runproc.HubOwner: dobby}}, true},
		{"HaltedStatusIsNotAlive", fakeSubstrates{battenSeeded: true, battenState: "blocked", parent: "beam", ownedUnder: map[string][]runproc.Record{"beam": dobby}}, false},
		{"CancelMarkerIsNotAlive", fakeSubstrates{battenSeeded: true, battenState: "running", parent: "beam", cancelMark: true, ownedUnder: map[string][]runproc.Record{"beam": dobby}}, false},
		{"NoDobbyRecordIsNotAlive", fakeSubstrates{battenSeeded: true, battenState: "running", parent: "beam", ownedUnder: map[string][]runproc.Record{"beam": nil}}, false},
		{"RegisteredBattenIsAlive", fakeSubstrates{ownedUnder: map[string][]runproc.Record{"slug": {{PID: 8, Role: runproc.RoleBatten}}}}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := tt.f
			got, err := f.teardown().registeredBattenAlive("slug")
			if err != nil {
				t.Fatalf("registeredBattenAlive() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("registeredBattenAlive() = %t; want %t", got, tt.want)
			}
		})
	}
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
