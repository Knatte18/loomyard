//go:build tier0

// cancel_test.go pins the cancel phase: its call order over the fake substrates, the records write lock hold, and the live-strand pick.

package pairteardown

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/runproc"
	"github.com/Knatte18/loomyard/internal/testkit/locationkit"
)

// hookAnswer is one call's answer of the cancel request's AfterEnd hook.
type hookAnswer struct {
	busy string
	err  error
}

// TestEndForCancel drives the cancel phase through the fake substrates and pins which substrate calls it makes, in what order, and what it returns.
// A refusal row's calls end at the refusing step, so nothing after it ran.
func TestEndForCancel(t *testing.T) {
	t.Parallel()

	survivors := []runproc.Ended{{PID: 4242, Role: runproc.RoleLoop}}
	const presentHead = "holdWriteLocks,endOwned,release,driverLock,afterEnd"
	const refusalTail = "lyx batten cancel slug"
	tests := []struct {
		name string
		gone bool
		keep bool
		// hooks answers each AfterEnd call in turn; a request with no hooks has a nil AfterEnd.
		hooks                 []hookAnswer
		writeLockBusy         bool
		endOwnedErr           error
		driverLockHeldByProbe []bool
		driverLockHeld        bool
		awaitErr              error
		killCaptureErr        error
		strands               []string
		wantCalls             string
		wantErrIs             error
		wantAnyError          bool
		wantErrContains       []string
		wantResult            CancelResult
	}{
		{
			name:       "PresentPairKeptWithdrawsTheTeardownMark",
			keep:       true,
			hooks:      []hookAnswer{{}},
			strands:    []string{"tst:slug:driver"},
			wantCalls:  presentHead + ",killCapture,stopStrands,kill,seize,await,end,release(ended=false)",
			wantResult: CancelResult{StrandsStopped: []string{"tst:slug:driver"}, SessionEnded: true},
		},
		{
			name:       "PresentPairRemovedKeepsTheTeardownMark",
			hooks:      []hookAnswer{{}},
			wantCalls:  presentHead + ",killCapture,stopStrands,kill,seize,await,end,release(ended=true)",
			wantResult: CancelResult{SessionEnded: true},
		},
		{
			name:       "NilHookTakesNothing",
			wantCalls:  "holdWriteLocks,endOwned,release,driverLock,killCapture,stopStrands,kill,seize,await,end,release(ended=true)",
			wantResult: CancelResult{SessionEnded: true},
		},
		{
			name:       "GonePairEndsOwnedProcessesAndTheSessionByNameOnly",
			gone:       true,
			hooks:      []hookAnswer{{}},
			wantCalls:  "holdWriteLocks,endOwned,release,afterEnd,killCapture,end",
			wantResult: CancelResult{SessionEnded: true, TaskWorktreeGone: true},
		},
		{
			name:       "HookBusyOnceEndsAndProbesAgainThenGoesOn",
			hooks:      []hookAnswer{{busy: "/hub/run.lock"}, {}},
			wantCalls:  presentHead + ",holdWriteLocks,endOwned,release,driverLock,afterEnd,killCapture,stopStrands,kill,seize,await,end,release(ended=true)",
			wantResult: CancelResult{SessionEnded: true},
		},
		{
			name:            "BusyWriteLockRefusesNamingItsPathAndTheWayForward",
			hooks:           []hookAnswer{{}},
			writeLockBusy:   true,
			wantCalls:       "holdWriteLocks",
			wantErrIs:       ErrDriverBusy,
			wantErrContains: []string{"/hub/records/lock", "lyx tender status", "ps", "Task Manager", refusalTail},
		},
		{
			name:            "SurvivorsRefuseNamingEachPidAndTheWayForward",
			hooks:           []hookAnswer{{}},
			endOwnedErr:     &runproc.SurvivorsError{Survivors: survivors},
			wantCalls:       "holdWriteLocks,endOwned,release",
			wantErrIs:       ErrDriverBusy,
			wantErrContains: []string{"4242", runproc.RoleLoop, "lyx tender status", "ps", refusalTail},
		},
		{
			name:            "OwnedEndErrorThatIsNotSurvivorsIsReturned",
			hooks:           []hookAnswer{{}},
			endOwnedErr:     errors.New("record directory unreadable"),
			wantCalls:       "holdWriteLocks,endOwned,release",
			wantErrContains: []string{"record directory unreadable"},
		},
		{
			name:            "DriverLockHeldAtTheFirstProbeRefuses",
			hooks:           []hookAnswer{{}},
			driverLockHeld:  true,
			wantCalls:       "holdWriteLocks,endOwned,release,driverLock",
			wantErrIs:       ErrDriverBusy,
			wantErrContains: []string{refusalTail},
		},
		{
			name:                  "DriverLockHeldAtTheSecondProbeAfterABusyHookRefuses",
			hooks:                 []hookAnswer{{busy: "/hub/run.lock"}, {}},
			driverLockHeldByProbe: []bool{false, true},
			wantCalls:             presentHead + ",holdWriteLocks,endOwned,release,driverLock",
			wantErrIs:             ErrDriverBusy,
			wantErrContains:       []string{refusalTail},
		},
		{
			name:            "HookStillBusyAfterTheRetryRefusesNamingItsPath",
			hooks:           []hookAnswer{{busy: "/hub/run.lock"}, {busy: "/hub/run.lock"}},
			wantCalls:       presentHead + ",holdWriteLocks,endOwned,release,driverLock,afterEnd",
			wantErrIs:       ErrDriverBusy,
			wantErrContains: []string{"/hub/run.lock", "lyx tender status", "ps", "Task Manager", "lyx batten cancel"},
		},
		{
			name:            "HookErrorStopsThePhase",
			hooks:           []hookAnswer{{err: errors.New("cancel marker unwritable")}},
			wantCalls:       presentHead,
			wantErrContains: []string{"cancel marker unwritable"},
		},
		{
			name:            "SpentRunLockWaitRefusesNamingTheCancelAndPutsTheMarkBack",
			hooks:           []hookAnswer{{}},
			awaitErr:        errors.New("the run lock is still held by pid 42; retry the removal once the holder ends"),
			wantCalls:       presentHead + ",killCapture,stopStrands,kill,seize,await,release(ended=false)",
			wantErrContains: []string{"pid 42", refusalTail},
		},
		{
			name:            "CaptureJobTerminationFailureRefusesNamingTheCancelBeforeAnyStrandIsStopped",
			hooks:           []hookAnswer{{}},
			killCaptureErr:  errors.New("job object unreachable"),
			wantCalls:       presentHead + ",killCapture",
			wantErrContains: []string{"end the pair's drift capture", "job object unreachable", refusalTail},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := &fakeSubstrates{
				quiets: []quietState{{quiet: true}}, gone: tt.gone, writeLockBusy: tt.writeLockBusy, endOwnedErr: tt.endOwnedErr,
				driverLockHeld: tt.driverLockHeld, driverLockHeldByProbe: tt.driverLockHeldByProbe, awaitErr: tt.awaitErr, strands: tt.strands,
				killCaptureErr: tt.killCaptureErr,
			}
			td := f.teardown()
			request := CancelRequest{Slug: "slug", Keep: tt.keep}
			if len(tt.hooks) > 0 {
				hooks := tt.hooks
				request.AfterEnd = func() (string, error) {
					f.calls = append(f.calls, "afterEnd")
					answer := hooks[0]
					hooks = hooks[1:]
					return answer.busy, answer.err
				}
			}

			result, err := td.EndForCancel(context.Background(), request)

			switch {
			case tt.wantErrIs != nil:
				if !errors.Is(err, tt.wantErrIs) {
					t.Fatalf("EndForCancel error = %v, want one wrapping %v", err, tt.wantErrIs)
				}
			case tt.wantAnyError || len(tt.wantErrContains) > 0:
				if err == nil {
					t.Fatal("EndForCancel error = nil, want one")
				}
			case err != nil:
				t.Fatalf("EndForCancel error = %v, want nil", err)
			}
			for _, want := range tt.wantErrContains {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("EndForCancel error %q lacks %q", err.Error(), want)
				}
			}
			if got := strings.Join(f.calls, ","); got != tt.wantCalls {
				t.Errorf("calls = %s, want %s", got, tt.wantCalls)
			}
			if err == nil {
				if result.SessionEnded != tt.wantResult.SessionEnded || result.TaskWorktreeGone != tt.wantResult.TaskWorktreeGone || !slices.Equal(result.StrandsStopped, tt.wantResult.StrandsStopped) {
					t.Errorf("EndForCancel result = %+v, want %+v", result, tt.wantResult)
				}
			}
		})
	}
}

// TestHoldRecordsWriteLocks pins which records write locks the hold takes and that a lock held elsewhere refuses naming its path and leaves nothing held.
func TestHoldRecordsWriteLocks(t *testing.T) {
	t.Parallel()

	// A wait of zero reports busy before its first try, so the wait is a few milliseconds.
	const contendedWait = 20 * time.Millisecond
	newHub := func(t *testing.T, withPair bool) (*Teardown, string, string) {
		t.Helper()
		hub := t.TempDir()
		prime := locationkit.Location(hub, "prime", ".")
		primeRecords := fabricengine.RecordsWorktree(prime)
		pairRecords := fabricengine.RecordsWorktreePath(prime, "slug")
		dirs := []string{primeRecords}
		if withPair {
			dirs = append(dirs, pairRecords)
		}
		for _, dir := range dirs {
			// An empty .git directory is the geometry the lock directory's exclude seeding needs.
			if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
				t.Fatal(err)
			}
		}
		return &Teardown{prime: prime, writeLockWait: contendedWait}, primeRecords, pairRecords
	}
	held := func(t *testing.T, worktree string) bool {
		t.Helper()
		release, ok, err := fabricengine.HoldRecordsWriteLock(worktree, contendedWait)
		if err != nil {
			t.Fatalf("HoldRecordsWriteLock(%s): %v", worktree, err)
		}
		if ok {
			if err := release(); err != nil {
				t.Fatal(err)
			}
		}
		return !ok
	}

	t.Run("NoPairRecordsWorktreeTakesOnlyThePrimesLock", func(t *testing.T) {
		t.Parallel()
		td, primeRecords, _ := newHub(t, false)
		release, err := td.holdRecordsWriteLocks("slug")
		if err != nil {
			t.Fatalf("holdRecordsWriteLocks: %v", err)
		}
		if !held(t, primeRecords) {
			t.Error("the prime's records write lock is free while held")
		}
		if err := release(); err != nil {
			t.Fatal(err)
		}
		if held(t, primeRecords) {
			t.Error("the prime's records write lock is still held after the release")
		}
	})

	t.Run("PairRecordsWorktreePresentTakesBothLocks", func(t *testing.T) {
		t.Parallel()
		td, primeRecords, pairRecords := newHub(t, true)
		release, err := td.holdRecordsWriteLocks("slug")
		if err != nil {
			t.Fatalf("holdRecordsWriteLocks: %v", err)
		}
		if !held(t, primeRecords) || !held(t, pairRecords) {
			t.Error("a records write lock is free while held")
		}
		if err := release(); err != nil {
			t.Fatal(err)
		}
		if held(t, primeRecords) || held(t, pairRecords) {
			t.Error("a records write lock is still held after the release")
		}
	})

	t.Run("PairLockHeldElsewhereRefusesNamingItAndHoldsNeither", func(t *testing.T) {
		t.Parallel()
		td, primeRecords, pairRecords := newHub(t, true)
		releaseOther, ok, err := fabricengine.HoldRecordsWriteLock(pairRecords, contendedWait)
		if err != nil || !ok {
			t.Fatalf("HoldRecordsWriteLock(pair) = held %v, err %v; want held", ok, err)
		}
		t.Cleanup(func() { _ = releaseOther() })

		_, err = td.holdRecordsWriteLocks("slug")
		if !errors.Is(err, ErrDriverBusy) {
			t.Fatalf("holdRecordsWriteLocks error = %v, want one wrapping ErrDriverBusy", err)
		}
		if !strings.Contains(err.Error(), fabricengine.RecordsLockDirPath(pairRecords)) {
			t.Errorf("holdRecordsWriteLocks error %q lacks the busy lock's path %q", err.Error(), fabricengine.RecordsLockDirPath(pairRecords))
		}
		if held(t, primeRecords) {
			t.Error("the prime's records write lock is still held after the refusal")
		}
	})
}

// TestLiveStrandGUIDs pins that every live row is picked whatever its name, in row order, and no dormant row is.
func TestLiveStrandGUIDs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		rows      []reedengine.DirectoryRow
		wantGUIDs []string
		wantNames []string
	}{
		{
			name: "LiveRowsOfEveryKindArePicked",
			rows: []reedengine.DirectoryRow{
				{Name: "tst:slug:driver", GUID: "g1", Live: true},
				{Name: "tst:slug:arbiter", GUID: "g2", Live: true},
				{Name: "tst:slug:producer", GUID: "g3", Live: true},
				{Name: "tst:slug:other", GUID: "g4", Live: true},
			},
			wantGUIDs: []string{"g1", "g2", "g3", "g4"},
			wantNames: []string{"tst:slug:driver", "tst:slug:arbiter", "tst:slug:producer", "tst:slug:other"},
		},
		{
			name: "DormantRowsAreNotPicked",
			rows: []reedengine.DirectoryRow{
				{Name: "tst:slug:driver", GUID: "g1"},
				{Name: "tst:slug:arbiter", GUID: "g2", Live: true},
			},
			wantGUIDs: []string{"g2"},
			wantNames: []string{"tst:slug:arbiter"},
		},
		{
			name: "NoRowsPickNothing",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			guids, names := liveStrandGUIDs(tt.rows)
			if !slices.Equal(guids, tt.wantGUIDs) || !slices.Equal(names, tt.wantNames) {
				t.Errorf("liveStrandGUIDs() = %v, %v; want %v, %v", guids, names, tt.wantGUIDs, tt.wantNames)
			}
		})
	}
}
