// cancel_test.go pins cancelRun's decisions and order over fake substrates, and the pure readings it builds on.

package battencli

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/battenrecipe"
	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/pairteardown"
	"github.com/Knatte18/loomyard/internal/runfsm"
	"github.com/Knatte18/loomyard/internal/runproc"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

const cancelSlug = "some-slug"

// destructiveCancelCalls are the seams whose call means the cancel ended, removed, cleared or lifted something.
var destructiveCancelCalls = []string{"WriteIntent", "CommitRecord", "EndForCancel", "StopTackler", "RemovePair", "DiscardTaskBranch", "FinishCancel", "RemoveRecords", "ClearClaim", "LiftPauses", "SettleTeardown", "EndBeam"}

// cancelFake is a set of fake substrates for cancelRun that records every call and answers from its knobs.
// Its zero value has no status, no board entry and no records, and nothing alive.
type cancelFake struct {
	calls []string

	hubPaused       bool
	worktreePresent bool
	report          fabriclayout.OriginReport
	reconcileErr    error
	pullErr         error
	machine, owner  string
	subject         string
	status          shedengine.Status
	statusFound     bool
	boardStatus     *string
	boardFound      bool
	recordsPresent  bool
	reached         bool
	marker          cancelMarkerReading
	takeMarkerErr   error
	goos            string
	recordLists     [][]runproc.Record
	createdAt       map[int]time.Time
	parts           fabriclayout.PairParts
	sessionLive     bool
	endResult       pairteardown.CancelResult
	endErr          error
	lockAnswers     []string
	tackler         string
	tacklerErr      error
	removeResult    fabriclayout.RemoveResult
	removeErr       error
	onOrigin        bool
	discardResult   fabriclayout.DiscardResult
	discardErr      error
	teardownPending bool
	settleErr       error
	recordsErr      error
	recordsCommit   bool
	claimCleared    bool
	claimErr        error
	lifted          []string
	notifyErr       error
	pushOwed        bool
	pushErr         error

	// verdict answers the act rule; commitResult is whether the record commit made a commit.
	// intentErr, commitErr and finishErr fail the intent write, the record commit and the finish.
	// intentRemove and finishRemove record the remove flag each was written with.
	verdict      runfsm.Verdict
	verdictErr   error
	commitResult bool
	intentErr    error
	commitErr    error
	finishErr    error
	intentRemove bool
	finishRemove bool

	// The fields from beamProject through stopBeamErr answer the beam seams;
	// their zero values make the slug a task.
	beamProject     bool
	hubReport       fabriclayout.OriginReport
	hubReconcileErr error
	landRefusal     error
	landingHeld     bool
	landBlockers    BeamLandBlockers
	stopBeamErr     error

	endRequest      pairteardown.CancelRequest
	removeRequest   pairteardown.Request
	markerReleases  int
	locksHeld       int
	lockTakenHeld   bool
	listCalls       int
	notices         []string
	gateRestored    bool
	callsAtRestore  int
	lockAnswerIndex int
}

// newCancelFake returns the fake of a blocked run with no pair that a cancel ends in full.
func newCancelFake() *cancelFake {
	running := "running · Worktree-Create"
	return &cancelFake{
		status:         shedengine.Status{CurrentProducer: battenrecipe.NameWorktreeCreate, State: shedengine.StateBlocked},
		statusFound:    true,
		boardStatus:    &running,
		boardFound:     true,
		recordsPresent: true,
		machine:        "this-machine",
		claimCleared:   true,
		verdict:        runfsm.Act(runfsm.Inputs{State: runfsm.Running}),
		commitResult:   true,
		recordsCommit:  true,
	}
}

func (f *cancelFake) call(name string) { f.calls = append(f.calls, name) }

func (f *cancelFake) called(name string) bool { return slices.Contains(f.calls, name) }

// deps wires every seam of cancelDeps to the fake.
func (f *cancelFake) deps() cancelDeps {
	return cancelDeps{
		HubPaused: func() (bool, error) { f.call("HubPaused"); return f.hubPaused, nil },
		ExemptGate: func() (func(), error) {
			f.call("ExemptGate")
			return func() { f.gateRestored, f.callsAtRestore = true, len(f.calls) }, nil
		},
		TaskWorktreePresent: func() (bool, error) { f.call("TaskWorktreePresent"); return f.worktreePresent, nil },
		Reconcile: func() (fabriclayout.OriginReport, error) {
			f.call("Reconcile")
			return f.report, f.reconcileErr
		},
		PullBolt: func() error { f.call("PullBolt"); return f.pullErr },
		Owner: func() (string, string, bool, error) {
			f.call("Owner")
			return f.owner, f.subject, f.owner != "" && f.owner != f.machine, nil
		},
		ReadStatus: func() (shedengine.Status, bool, error) {
			f.call("ReadStatus")
			return f.status, f.statusFound, nil
		},
		BoardStatus:    func() (*string, bool, error) { f.call("BoardStatus"); return f.boardStatus, f.boardFound, nil },
		RecordsPresent: func() (bool, error) { f.call("RecordsPresent"); return f.recordsPresent, nil },
		ChildReachedParent: func(shedengine.Status, bool) (bool, error) {
			f.call("ChildReachedParent")
			return f.reached, nil
		},
		ReadMarker: func() (cancelMarkerReading, error) { f.call("ReadMarker"); return f.marker, nil },
		TakeMarker: func() (func() error, error) {
			f.call("TakeMarker")
			if f.takeMarkerErr != nil {
				return nil, f.takeMarkerErr
			}
			return func() error { f.markerReleases++; return nil }, nil
		},
		GOOS: f.goos,
		LiveRecords: func() ([]runproc.Record, error) {
			f.call("LiveRecords")
			f.listCalls++
			if len(f.recordLists) == 0 {
				return nil, nil
			}
			return f.recordLists[min(f.listCalls-1, len(f.recordLists)-1)], nil
		},
		CreatedAt: func(pid int) (time.Time, bool) {
			created, ok := f.createdAt[pid]
			return created, ok
		},
		PairParts:   func() (fabriclayout.PairParts, error) { f.call("PairParts"); return f.parts, nil },
		SessionLive: func() (bool, error) { f.call("SessionLive"); return f.sessionLive, nil },
		EndForCancel: func(_ context.Context, req pairteardown.CancelRequest) (pairteardown.CancelResult, error) {
			f.call("EndForCancel")
			f.endRequest = req
			if req.AfterEnd != nil {
				busy, err := req.AfterEnd()
				if err != nil {
					return f.endResult, err
				}
				if busy != "" {
					if busy, err = req.AfterEnd(); err != nil || busy != "" {
						return f.endResult, errors.Join(err, fmt.Errorf("still busy at %s", busy))
					}
				}
			}
			return f.endResult, f.endErr
		},
		TakeBattenLocks: func() (func(), string, error) {
			f.call("TakeBattenLocks")
			if f.locksHeld != 0 {
				f.lockTakenHeld = true
			}
			if f.lockAnswerIndex < len(f.lockAnswers) {
				busy := f.lockAnswers[f.lockAnswerIndex]
				f.lockAnswerIndex++
				if busy != "" {
					return nil, busy, nil
				}
			}
			f.locksHeld++
			return func() { f.locksHeld-- }, "", nil
		},
		StopTackler: func() (string, error) { f.call("StopTackler"); return f.tackler, f.tacklerErr },
		RemovePair: func(req pairteardown.Request) (fabriclayout.RemoveResult, error) {
			f.call("RemovePair")
			f.removeRequest = req
			return f.removeResult, f.removeErr
		},
		TaskBranchOnOrigin: func() (bool, error) { f.call("TaskBranchOnOrigin"); return f.onOrigin, nil },
		DiscardTaskBranch: func() (fabriclayout.DiscardResult, error) {
			f.call("DiscardTaskBranch")
			return f.discardResult, f.discardErr
		},
		TeardownPending: func() (bool, error) { f.call("TeardownPending"); return f.teardownPending, nil },
		SettleTeardown:  func() error { f.call("SettleTeardown"); return f.settleErr },
		Verdict:         func() (runfsm.Verdict, error) { f.call("Verdict"); return f.verdict, f.verdictErr },
		WriteIntent: func(remove bool) (bool, error) {
			f.call("WriteIntent")
			f.intentRemove = remove
			return f.statusFound && f.intentErr == nil, f.intentErr
		},
		CommitRecord: func() (bool, error) { f.call("CommitRecord"); return f.commitResult, f.commitErr },
		FinishCancel: func(remove bool) (bool, error) {
			f.call("FinishCancel")
			f.finishRemove = remove
			return f.statusFound && f.finishErr == nil, f.finishErr
		},
		RemoveRecords: func() (bool, error) { f.call("RemoveRecords"); return f.recordsCommit, f.recordsErr },
		ClearClaim:    func() (bool, bool, error) { f.call("ClearClaim"); return f.claimCleared, f.boardFound, f.claimErr },
		LiftPauses:    func() ([]string, error) { f.call("LiftPauses"); return f.lifted, nil },
		PushRecords:   func() (bool, error) { f.call("PushRecords"); return f.pushOwed, f.pushErr },
		Notify: func(line string) error {
			f.call("Notify")
			f.notices = append(f.notices, line)
			return f.notifyErr
		},
		BeamProject: func() (bool, error) { f.call("BeamProject"); return f.beamProject, nil },
		ReconcileHub: func() (fabriclayout.OriginReport, error) {
			f.call("ReconcileHub")
			return f.hubReport, f.hubReconcileErr
		},
		LandRefusal:  func() error { f.call("LandRefusal"); return f.landRefusal },
		LandingHeld:  func() (bool, error) { f.call("LandingHeld"); return f.landingHeld, nil },
		LandBlockers: func() (BeamLandBlockers, error) { f.call("LandBlockers"); return f.landBlockers, nil },
		StopBeam:     func() error { f.call("StopBeam"); return f.stopBeamErr },
		// EndBeam only records the call; what it does to the board is pinned over a real hub.
		EndBeam: func() (bool, error) { f.call("EndBeam"); return false, nil },
	}
}

// callsInOrder reports whether names occur in calls in that order, not necessarily adjacent.
func callsInOrder(calls, names []string) bool {
	next := 0
	for _, call := range calls {
		if next < len(names) && call == names[next] {
			next++
		}
	}
	return next == len(names)
}

func runningStatus(producer string) shedengine.Status {
	return shedengine.Status{CurrentProducer: producer, State: shedengine.StateRunning}
}

// TestCancelRun drives cancelRun through the fakes;
// a refusal row asserts none of the destructive seams ran, bar the rows that name the seam that refused.
func TestCancelRun(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	liveRecord := runproc.Record{PID: 4242, Role: "batten", RegisteredAt: time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)}
	entry := func(state fabriclayout.EntryState) fabriclayout.OriginReport {
		return fabriclayout.OriginReport{Entries: []fabriclayout.EntryOutcome{{Slug: cancelSlug, State: state, Owner: "other-machine", AdoptSubject: "some-beam", How: "done", WayForward: "fix it"}}}
	}
	pullUnreachable := fmt.Errorf("pull: %w", boardengine.ErrOwnerOriginUnreachable)

	type row struct {
		name  string
		setup func(*cancelFake)
		// remove is the --remove flag.
		remove bool
		// wantErr is a substring of the refusal, empty for none.
		wantErr string
		// wantErrAlso lists further substrings the refusal carries, and errAbsent substrings it never carries.
		wantErrAlso []string
		errAbsent   []string
		// nothingLeft is whether the refusal matches ErrNothingLeft through errors.Is.
		nothingLeft bool
		// judging is "endonly" when cancelJudgingEndOnly over a fresh fake of the row must return the origin-unreachable error.
		// It is "full" when that call must return nil, and empty when no judging check runs.
		judging string
		// wantCalls is an ordered subsequence of the calls made.
		wantCalls []string
		// noCalls are seams that must not have been called.
		noCalls []string
		// marker is "none" when the cancel's lock was never taken.
		// Any other value says it was taken and released exactly once:
		// "left" for a cancel that stopped early, "removed" for one that ran to its end.
		marker string
		check  func(*testing.T, cancelReport, *cancelFake)
	}

	keptCancelStatus := shedengine.Status{
		CurrentProducer: battenrecipe.NameRunShed,
		State:           shedengine.StateCancelled,
		Cancel:          &shedengine.CancelIntent{Finish: &shedengine.CancelFinish{}},
	}
	rows := []row{
		{
			name: "LandedSlugWithNoBattenRecordRefusesWithTheTerminalWording",
			setup: func(f *cancelFake) {
				f.statusFound = false
				f.verdict = runfsm.Act(runfsm.Inputs{State: runfsm.Landed})
			},
			wantErr: "the slug has landed",
			noCalls: append([]string{"TakeMarker"}, destructiveCancelCalls...),
			marker:  "none",
		},
		{
			name: "HandOverVerdictRefuses",
			setup: func(f *cancelFake) {
				f.verdict = runfsm.Act(runfsm.Inputs{OwnedElsewhere: true, Owner: "other-machine", AdoptSubject: "some-beam"})
			},
			wantErr: "machine other-machine owns the run",
			noCalls: append([]string{"TakeMarker"}, destructiveCancelCalls...),
			marker:  "none",
		},
		{
			name:    "UnreadableRunStateRefusesNamingTheRerun",
			setup:   func(f *cancelFake) { f.verdictErr = errors.New("board is gone") },
			wantErr: `could not read the run's state of some-slug: board is gone; way forward: fix the cause, then run "lym batten cancel some-slug" again`,
			noCalls: append([]string{"TakeMarker"}, destructiveCancelCalls...),
			marker:  "none",
		},
		{
			name:    "FinishedCancelThatKeptItsPairRefusesWithoutRemove",
			setup:   func(f *cancelFake) { f.status = keptCancelStatus },
			wantErr: `some-slug is already cancelled with its pair kept; way forward: run "lym batten cancel some-slug --remove"`,
			noCalls: append([]string{"TakeMarker"}, destructiveCancelCalls...),
			marker:  "none",
		},
		{
			name:      "FinishedCancelThatKeptItsPairIsFinishedByRemove",
			remove:    true,
			setup:     func(f *cancelFake) { f.status = keptCancelStatus },
			wantCalls: []string{"TakeMarker", "WriteIntent", "CommitRecord", "EndForCancel", "RemovePair", "RemoveRecords"},
			noCalls:   []string{"FinishCancel"},
			marker:    "removed",
		},
		{
			name:      "IntentIsWrittenAndCommittedBeforeAnythingEnds",
			remove:    true,
			wantCalls: []string{"TakeMarker", "WriteIntent", "CommitRecord", "EndForCancel"},
			marker:    "removed",
			check: func(t *testing.T, _ cancelReport, f *cancelFake) {
				if !f.intentRemove {
					t.Error("intent written with remove = false; want the cancel's --remove flag")
				}
			},
		},
		{
			name:      "FinishIsWrittenKeepingThePairWithoutRemove",
			wantCalls: []string{"EndForCancel", "FinishCancel", "CommitRecord"},
			marker:    "removed",
			check: func(t *testing.T, _ cancelReport, f *cancelFake) {
				if f.finishRemove || f.intentRemove {
					t.Errorf("finish remove, intent remove = %v, %v; want both false for a cancel that keeps the pair", f.finishRemove, f.intentRemove)
				}
			},
		},
		{
			name:      "SlugWithNoRecordWritesNoIntentAndKeepsNothing",
			setup:     func(f *cancelFake) { f.statusFound = false },
			wantCalls: []string{"WriteIntent", "EndForCancel", "FinishCancel", "RemoveRecords"},
			noCalls:   []string{"CommitRecord"},
			marker:    "removed",
			check: func(t *testing.T, r cancelReport, _ *cancelFake) {
				if r.RecordKept || r.RecordsRemoved {
					t.Errorf("report = %+v; want no record kept and none removed for a slug with no record", r)
				}
			},
		},
		{
			name:    "FailedIntentWriteRefusesWithNothingEnded",
			setup:   func(f *cancelFake) { f.intentErr = errors.New("disk full") },
			wantErr: `could not record the cancel of some-slug: disk full; way forward: fix the cause, then run "lym batten cancel some-slug" again`,
			noCalls: append([]string{"CommitRecord"}, destructiveCancelCalls[2:]...),
			marker:  "removed",
		},
		{
			name: "FailedIntentCommitRefusesWithNothingEnded",
			setup: func(f *cancelFake) {
				f.commitErr = errors.New("a merge is in progress on the prime's records side; conclude or abort that merge there")
			},
			wantErr: `could not commit the cancel of some-slug: a merge is in progress on the prime's records side; conclude or abort that merge there; way forward: fix the cause it names, then run "lym batten cancel some-slug" again`,
			noCalls: destructiveCancelCalls[2:],
			marker:  "removed",
		},
		{
			name:    "FailedFinishStopsBeforeTheClaim",
			setup:   func(f *cancelFake) { f.finishErr = errors.New("no intent recorded") },
			wantErr: `could not finish the cancel of some-slug: no intent recorded; way forward: fix the cause, then run "lym batten cancel some-slug" again`,
			noCalls: []string{"ClearClaim", "LiftPauses"},
			marker:  "left",
		},
		{
			name:      "BlockedRunWithNoPair",
			judging:   "full",
			wantCalls: []string{"HubPaused", "ExemptGate", "TaskWorktreePresent", "PullBolt", "Owner", "ReadStatus", "Verdict", "TakeMarker", "WriteIntent", "CommitRecord", "EndForCancel", "StopTackler", "FinishCancel", "CommitRecord", "ClearClaim", "Notify"},
			noCalls:   []string{"Reconcile", "RemovePair", "DiscardTaskBranch", "RemoveRecords"},
			marker:    "removed",
			setup:     func(f *cancelFake) { f.tackler = "guid-1"; f.lifted = []string{"some-slug"} },
			check: func(t *testing.T, r cancelReport, f *cancelFake) {
				if r.TacklerStopped != "guid-1" || r.RecordsRemoved || !r.RecordKept || !r.ClaimCleared || !r.PushSpawned || !r.NoticePosted || r.EndOnly {
					t.Errorf("report = %+v; want tackler, kept record, claim, push and notice done and no records removed", r)
				}
				if r.State != "blocked" || r.Row != battenrecipe.NameWorktreeCreate {
					t.Errorf("report state, row = %q, %q; want blocked at Worktree-Create", r.State, r.Row)
				}
				if !slices.Equal(r.Lifted, []string{"some-slug"}) {
					t.Errorf("report.Lifted = %v; want the lifted scope", r.Lifted)
				}
				if !f.gateRestored || f.callsAtRestore != len(f.calls) {
					t.Errorf("gate restored = %v at call %d of %d; want restored last", f.gateRestored, f.callsAtRestore, len(f.calls))
				}
				if f.locksHeld != 0 {
					t.Errorf("batten locks held after the cancel = %d; want 0", f.locksHeld)
				}
			},
		},
		{
			name:      "OwedRecordsPushAfterTheLiftsNamesTheRerun",
			setup:     func(f *cancelFake) { f.pushOwed = true },
			wantCalls: []string{"FinishCancel", "CommitRecord", "ClearClaim", "LiftPauses", "PushRecords", "Notify"},
			marker:    "removed",
			check: func(t *testing.T, r cancelReport, _ *cancelFake) {
				if !r.RecordKept || r.PushSpawned {
					t.Errorf("report = %+v; want the record kept and no push counted as landed", r)
				}
				if !strings.Contains(r.WayForward, `run "lym batten cancel some-slug" again`) {
					t.Errorf("report way forward = %q; want it to name the cancel to run again", r.WayForward)
				}
			},
		},
		{
			name: "LiveRunWithPairKeepsIt",
			setup: func(f *cancelFake) {
				f.worktreePresent = true
				f.status = runningStatus(battenrecipe.NameRunShed)
				f.parts = fabriclayout.PairParts{TaskWorktree: true, RecordsWorktree: true, RecordsBranch: true, TaskBranch: true}
				f.endResult = pairteardown.CancelResult{Ended: []runproc.Ended{{PID: 7, Role: "batten"}}, StrandsStopped: []string{"agent-1"}}
			},
			wantCalls: []string{"Reconcile", "EndForCancel", "StopTackler", "FinishCancel", "CommitRecord", "ClearClaim"},
			noCalls:   []string{"PullBolt", "RemovePair", "DiscardTaskBranch", "RemoveRecords"},
			marker:    "removed",
			check: func(t *testing.T, r cancelReport, f *cancelFake) {
				if !f.endRequest.Keep || f.endRequest.Slug != cancelSlug {
					t.Errorf("EndForCancel request = %+v; want Keep for the slug", f.endRequest)
				}
				wantWay := `run "lym batten cancel some-slug --remove" before rerunning it`
				if r.WayForward != wantWay || r.PairRemoved {
					t.Errorf("report way forward, pair removed = %q, %v; want %q, false", r.WayForward, r.PairRemoved, wantWay)
				}
				if len(r.Ended) != 1 || len(r.StrandsStopped) != 1 {
					t.Errorf("report ended, strands = %v, %v; want the phase's lists", r.Ended, r.StrandsStopped)
				}
				if !strings.Contains(f.notices[0], "pair was kept") {
					t.Errorf("notice = %q; want it to name the kept pair", f.notices[0])
				}
			},
		},
		{
			name:   "RemoveOnLiveRunRemovesPairAndBranch",
			remove: true,
			setup: func(f *cancelFake) {
				f.worktreePresent = true
				f.status = runningStatus(battenrecipe.NameRunShed)
				f.parts = fabriclayout.PairParts{TaskWorktree: true, TaskBranch: true}
				f.onOrigin = true
				f.removeResult = fabriclayout.RemoveResult{SnapshotTag: "snapshot/some-slug/1"}
				f.discardResult = fabriclayout.DiscardResult{ArchiveTags: []string{"archive/some-slug/abc"}}
			},
			wantCalls: []string{"WriteIntent", "CommitRecord", "EndForCancel", "RemovePair", "DiscardTaskBranch", "RemoveRecords", "ClearClaim"},
			noCalls:   []string{"FinishCancel"},
			marker:    "removed",
			check: func(t *testing.T, r cancelReport, f *cancelFake) {
				want := pairteardown.Request{Slug: cancelSlug, Force: true, Snapshot: true, Remote: true, LeaveClaim: true, ClearStray: true, Rerun: "lym batten cancel " + cancelSlug + " --remove"}
				if f.removeRequest.Slug != want.Slug || !f.removeRequest.Force || !f.removeRequest.Snapshot || !f.removeRequest.Remote || !f.removeRequest.LeaveClaim || !f.removeRequest.ClearStray || f.removeRequest.Rerun != want.Rerun {
					t.Errorf("RemovePair request = %+v; want %+v", f.removeRequest, want)
				}
				if f.endRequest.Keep {
					t.Error("EndForCancel request kept the pair; want it released for the removal")
				}
				if !r.PairRemoved || r.SnapshotTag != "snapshot/some-slug/1" || !slices.Equal(r.ArchiveTags, []string{"archive/some-slug/abc"}) || !r.ClaimCleared || !r.RecordsRemoved || r.RecordKept {
					t.Errorf("report = %+v; want the pair removed with its tags and the claim cleared", r)
				}
			},
		},
		{
			name: "PausedRun",
			setup: func(f *cancelFake) {
				f.status = shedengine.Status{CurrentProducer: battenrecipe.NameRunShed, State: shedengine.StatePaused}
			},
			check: func(t *testing.T, r cancelReport, _ *cancelFake) {
				if r.State != "paused" || r.Row != battenrecipe.NameRunShed {
					t.Errorf("report state, row = %q, %q; want paused at Run-Shed", r.State, r.Row)
				}
			},
			marker: "removed",
		},
		{
			name: "AwaitingRun",
			setup: func(f *cancelFake) {
				f.status = shedengine.Status{CurrentProducer: battenrecipe.NameRunShed, State: shedengine.StateAwaiting}
			},
			marker: "removed",
		},
		{
			name: "FailedRun",
			setup: func(f *cancelFake) {
				f.status = shedengine.Status{CurrentProducer: battenrecipe.NameRunShed, State: shedengine.StateFailed}
			},
			marker: "removed",
		},
		{
			name:   "SeedWithNoStatus",
			setup:  func(f *cancelFake) { f.statusFound = false },
			marker: "removed",
			check: func(t *testing.T, r cancelReport, f *cancelFake) {
				if r.State != "" || !strings.Contains(f.notices[0], "state no longer readable") {
					t.Errorf("state, notice = %q, %q; want an empty state and the unreadable wording", r.State, f.notices[0])
				}
			},
		},
		{
			name:   "ClaimWithNoRecords",
			setup:  func(f *cancelFake) { f.statusFound, f.recordsPresent = false, false },
			marker: "removed",
			check: func(t *testing.T, r cancelReport, _ *cancelFake) {
				if !r.ClaimCleared {
					t.Error("claim was not cleared; want the claim alone to be left to cancel")
				}
			},
		},
		{
			name:   "ReRunAfterRecordsWereRemoved",
			setup:  func(f *cancelFake) { f.statusFound, f.recordsPresent = false, false; f.sessionLive = true },
			marker: "removed",
			check: func(t *testing.T, _ cancelReport, f *cancelFake) {
				if !strings.Contains(f.notices[0], "state no longer readable") {
					t.Errorf("notice = %q; want the unreadable wording", f.notices[0])
				}
			},
		},
		{
			name:    "HubPauseRefuses",
			setup:   func(f *cancelFake) { f.hubPaused = true },
			wantErr: `a hub pause covers the prime; way forward: run "lym tender resume", then "lym batten cancel some-slug" again`,
			noCalls: append([]string{"ExemptGate", "TaskWorktreePresent", "TakeMarker"}, destructiveCancelCalls...),
			marker:  "none",
		},
		{
			name:    "ReachedParentRefuses",
			setup:   func(f *cancelFake) { f.reached = true },
			wantErr: `way forward: run "lym batten run some-slug" to finish the landing, then "lym batten cleanup --apply"`,
			noCalls: append([]string{"TakeMarker"}, destructiveCancelCalls...),
			marker:  "none",
		},
		{
			name:    "EndedDoneWithRecordsRefuses",
			setup:   func(f *cancelFake) { f.worktreePresent = true; f.report = entry(fabriclayout.EntryEnded) },
			wantErr: `some-slug was landed by another machine; way forward: run "lym cleanup --apply" from the prime`,
			noCalls: append([]string{"TakeMarker"}, destructiveCancelCalls...),
			marker:  "none",
		},
		{
			name:   "EndedWithNoRecordsHereGoesOn",
			remove: true,
			setup: func(f *cancelFake) {
				f.worktreePresent, f.report, f.recordsPresent = true, entry(fabriclayout.EntryEnded), false
				f.parts = fabriclayout.PairParts{TaskWorktree: true}
			},
			wantCalls: []string{"Reconcile", "RemovePair", "RemoveRecords"},
			marker:    "removed",
		},
		{
			name:   "EntryAbsentFromBoardHasNoClaimToClear",
			setup:  func(f *cancelFake) { f.boardStatus, f.boardFound, f.claimCleared = nil, false, false },
			marker: "removed",
			check: func(t *testing.T, r cancelReport, _ *cancelFake) {
				if r.ClaimCleared {
					t.Error("claim cleared for an entry absent from the board; want false")
				}
			},
		},
		{
			name:    "OwnedElsewherePresentRefuses",
			setup:   func(f *cancelFake) { f.worktreePresent = true; f.report = entry(fabriclayout.EntryOwnedElsewhere) },
			wantErr: `some-slug is owned by machine other-machine; way forward: run "lym adopt some-beam" on this machine first, or "lym batten cancel some-slug" on other-machine`,
			noCalls: append([]string{"TakeMarker"}, destructiveCancelCalls...),
			marker:  "none",
		},
		{
			name:    "OwnedElsewhereAbsentRefuses",
			setup:   func(f *cancelFake) { f.owner, f.subject = "other-machine", "some-beam" },
			wantErr: `some-slug is owned by machine other-machine; way forward: run "lym adopt some-beam" on this machine first`,
			noCalls: append([]string{"TakeMarker"}, destructiveCancelCalls...),
			marker:  "none",
		},
		{
			name:    "OwnedElsewhereAbsentRootTaskNamesRoot",
			setup:   func(f *cancelFake) { f.owner, f.subject = "other-machine", "root" },
			wantErr: `some-slug is owned by machine other-machine; way forward: run "lym adopt root" on this machine first`,
			noCalls: append([]string{"TakeMarker"}, destructiveCancelCalls...),
			marker:  "none",
		},
		{
			name: "OwnedElsewhereWithOriginUnreachableNamesTheBeam",
			setup: func(f *cancelFake) {
				f.worktreePresent, f.report = true, fabriclayout.OriginReport{Unreachable: true}
				f.owner, f.subject = "other-machine", "some-beam"
			},
			wantErr: `some-slug is owned by machine other-machine; way forward: run "lym adopt some-beam" on this machine first`,
			noCalls: append([]string{"TakeMarker"}, destructiveCancelCalls...),
			marker:  "none",
		},
		{
			name: "StoppedBranchNamesTheCancel",
			setup: func(f *cancelFake) {
				f.worktreePresent = true
				f.report = entry(fabriclayout.EntryStopped)
				f.report.Branches = []fabriclayout.BranchOutcome{{Branch: "some-slug-records", Repo: "records", Slug: cancelSlug, State: fabriclayout.BranchDiverged, WayForward: "reset it"}}
			},
			wantErr: `cannot cancel "some-slug": branch some-slug-records is diverged against origin; reset it; then run "lym batten cancel some-slug" again`,
			noCalls: append([]string{"TakeMarker"}, destructiveCancelCalls...),
			marker:  "none",
		},
		{
			name: "StoppedBranchNamingTheRemovalAtLandEndsInTheLandWayForwardNotTheCancel",
			setup: func(f *cancelFake) {
				f.worktreePresent = true
				f.status = runningStatus(battenrecipe.NameLand)
				f.report = entry(fabriclayout.EntryStopped)
				f.report.Branches = []fabriclayout.BranchOutcome{{Branch: "some-slug-records", Repo: "records", Slug: cancelSlug, State: fabriclayout.BranchDiverged, WayForward: "run `lym fabric remove some-slug` to clear it"}}
			},
			wantErr:   "cannot cancel \"some-slug\": branch some-slug-records is diverged against origin; run `lym fabric remove some-slug` to clear it; " + pairteardown.BattenFollowUp(cancelSlug, runningStatus(battenrecipe.NameLand), true),
			errAbsent: []string{`then run "lym batten cancel some-slug" again`},
			noCalls:   append([]string{"TakeMarker"}, destructiveCancelCalls...),
			marker:    "none",
		},
		{
			name: "AnchorStoppedBranchLetsTheCancelThrough",
			setup: func(f *cancelFake) {
				f.worktreePresent = true
				f.report = entry(fabriclayout.EntryStopped)
				f.report.Branches = []fabriclayout.BranchOutcome{{Branch: "some-slug-records", Repo: "records", Slug: cancelSlug, State: fabriclayout.BranchAnchorStopped, WayForward: "run the set"}}
			},
			marker: "removed",
		},
		{
			name: "AnchorStoppedWithADivergedBranchStillRefuses",
			setup: func(f *cancelFake) {
				f.worktreePresent = true
				f.report = entry(fabriclayout.EntryStopped)
				f.report.Branches = []fabriclayout.BranchOutcome{
					{Branch: "some-slug", Repo: "code", Slug: cancelSlug, State: fabriclayout.BranchDiverged, WayForward: "reset it"},
					{Branch: "some-slug-records", Repo: "records", Slug: cancelSlug, State: fabriclayout.BranchAnchorStopped, WayForward: "run the set"},
				}
			},
			wantErr: `cannot cancel "some-slug": branch some-slug is diverged against origin; reset it; then run "lym batten cancel some-slug" again`,
			noCalls: append([]string{"TakeMarker"}, destructiveCancelCalls...),
			marker:  "none",
		},
		{
			name:    "ReconcileErrorRefuses",
			setup:   func(f *cancelFake) { f.worktreePresent = true; f.reconcileErr = errors.New("git is gone") },
			wantErr: `git is gone; way forward: fix the cause it names, then run "lym batten cancel some-slug" again`,
			noCalls: append([]string{"TakeMarker"}, destructiveCancelCalls...),
			marker:  "none",
		},
		{
			name:    "PullErrorRefuses",
			setup:   func(f *cancelFake) { f.pullErr = errors.New("board is dirty") },
			wantErr: `board is dirty; way forward: fix the cause it names, then run "lym batten cancel some-slug" again`,
			noCalls: append([]string{"TakeMarker"}, destructiveCancelCalls...),
			marker:  "none",
		},
		{
			name:    "WindowsLiveRecordAtFirstListRefusesAndReleasesTheLock",
			setup:   func(f *cancelFake) { f.goos = "windows"; f.recordLists = [][]runproc.Record{nil, {liveRecord}} },
			wantErr: `pid 4242 (batten): taskkill /PID 4242 /T /F; way forward: end each with its taskkill line, then run "lym batten cancel some-slug" again`,
			noCalls: destructiveCancelCalls,
			marker:  "removed",
		},
		{
			name: "WindowsLiveRecordAtFirstListOverAnInterruptedCancelReleasesTheLock",
			setup: func(f *cancelFake) {
				f.goos = "windows"
				f.marker = cancelMarkerReading{Present: true}
				f.recordLists = [][]runproc.Record{nil, {liveRecord}}
			},
			wantErr: "taskkill /PID 4242 /T /F",
			noCalls: destructiveCancelCalls,
			marker:  "left",
		},
		{
			name:    "WindowsLiveRecordAtTheReListReleasesLocksAndLeavesTheMarker",
			setup:   func(f *cancelFake) { f.goos = "windows"; f.recordLists = [][]runproc.Record{nil, nil, {liveRecord}} },
			wantErr: "taskkill /PID 4242 /T /F",
			noCalls: []string{"StopTackler", "RemovePair", "RemoveRecords", "ClearClaim", "LiftPauses"},
			marker:  "left",
			check: func(t *testing.T, _ cancelReport, f *cancelFake) {
				if f.locksHeld != 0 {
					t.Errorf("batten locks held after the refusal = %d; want 0", f.locksHeld)
				}
			},
		},
		{
			name: "WindowsRecordCreatedAfterItsRegistrationIsDropped",
			setup: func(f *cancelFake) {
				f.goos = "windows"
				f.recordLists = [][]runproc.Record{{liveRecord}}
				f.createdAt = map[int]time.Time{4242: liveRecord.RegisteredAt.Add(time.Hour)}
			},
			wantCalls: []string{"EndForCancel", "FinishCancel"},
			marker:    "removed",
		},
		{
			name: "NothingLeftRefuses",
			setup: func(f *cancelFake) {
				f.recordsPresent, f.boardStatus, f.boardFound, f.statusFound = false, nil, false, false
			},
			wantErr:     `nothing of some-slug is left to cancel; way forward: check it with "lym board list"`,
			nothingLeft: true,
			noCalls:     append([]string{"TakeMarker"}, destructiveCancelCalls...),
			marker:      "none",
		},
		{
			name: "TaskBranchAloneRefusesWithoutRemove",
			setup: func(f *cancelFake) {
				f.recordsPresent, f.boardStatus, f.boardFound = false, nil, false
				f.parts = fabriclayout.PairParts{TaskBranch: true}
			},
			wantErr: `run "lym batten cancel some-slug --remove", which archives and deletes its task branch`,
			noCalls: append([]string{"TakeMarker"}, destructiveCancelCalls...),
			marker:  "none",
		},
		{
			name:        "NothingLeftWithRemoveReadsOrigin",
			remove:      true,
			setup:       func(f *cancelFake) { f.recordsPresent, f.boardStatus, f.boardFound = false, nil, false },
			wantErr:     `nothing of some-slug is left to cancel`,
			nothingLeft: true,
			wantCalls:   []string{"TaskBranchOnOrigin"},
			noCalls:     append([]string{"TakeMarker"}, destructiveCancelCalls...),
			marker:      "none",
		},
		{
			name:   "NothingLeftWithRemoveAndUnreachableOriginNamesTheRerun",
			remove: true,
			setup: func(f *cancelFake) {
				f.recordsPresent, f.boardStatus, f.boardFound = false, nil, false
				f.pullErr = pullUnreachable
			},
			wantErr: `origin was unreachable, so the task branch of some-slug on origin could not be read; way forward: run "lym batten cancel some-slug --remove" again once origin answers`,
			noCalls: append([]string{"TakeMarker", "TaskBranchOnOrigin"}, destructiveCancelCalls...),
			marker:  "none",
		},
		{
			name:   "TaskBranchOnOriginAloneIsLeftToDiscardWithRemove",
			remove: true,
			setup: func(f *cancelFake) {
				f.recordsPresent, f.boardStatus, f.boardFound = false, nil, false
				f.onOrigin = true
			},
			wantCalls: []string{"RemovePair", "DiscardTaskBranch", "RemoveRecords"},
			marker:    "removed",
		},
		{
			name:   "UnfinishedTeardownAloneIsLeftToFinishWithRemove",
			remove: true,
			setup: func(f *cancelFake) {
				f.recordsPresent, f.boardStatus, f.boardFound, f.statusFound = false, nil, false, false
				f.teardownPending = true
			},
			wantCalls: []string{"TeardownPending", "RemovePair"},
			noCalls:   []string{"SettleTeardown"},
			marker:    "removed",
		},
		{
			name:   "FailedSettleOfAGonePairNamesTheRerun",
			remove: true,
			setup: func(f *cancelFake) {
				f.removeErr = fmt.Errorf("remove: %w", fabricengine.ErrPairNotFound)
				f.settleErr = errors.New("archive tag push refused")
			},
			wantErr: `settling the unfinished teardown of some-slug failed: archive tag push refused; way forward: fix the cause it names, then run "lym batten cancel some-slug --remove" again`,
			noCalls: []string{"RemoveRecords", "ClearClaim", "LiftPauses"},
			marker:  "left",
		},
		{
			name: "LiveCancelRefuses",
			setup: func(f *cancelFake) {
				f.marker = cancelMarkerReading{Present: true, WriterAlive: true, Intent: shedengine.CancelIntent{StartedAt: "2026-10-23T08:00:00Z"}}
			},
			wantErr: "a cancel of some-slug is under way on this machine since 2026-10-23T08:00:00Z",
			noCalls: append([]string{"TakeMarker"}, destructiveCancelCalls...),
			marker:  "none",
		},
		{
			name:      "InterruptedCancelIsTakenOver",
			setup:     func(f *cancelFake) { f.marker = cancelMarkerReading{Present: true} },
			wantCalls: []string{"ReadMarker", "TakeMarker", "EndForCancel", "FinishCancel"},
			marker:    "removed",
		},
		{
			name:    "EndForCancelRefusalLeavesTheMarker",
			setup:   func(f *cancelFake) { f.endErr = errors.New("driver busy; run \"lym batten cancel some-slug\" again") },
			wantErr: "driver busy",
			noCalls: []string{"StopTackler", "RemovePair", "RemoveRecords", "ClearClaim", "LiftPauses"},
			marker:  "left",
		},
		{
			name:   "RefusedRemovePairNamesTheCancelWithRemove",
			remove: true,
			setup: func(f *cancelFake) {
				f.parts = fabriclayout.PairParts{TaskWorktree: true}
				f.removeErr = errors.New("dirty beyond force")
			},
			wantErr: `dirty beyond force; way forward: fix the cause it names, then run "lym batten cancel some-slug --remove" again`,
			noCalls: []string{"DiscardTaskBranch", "RemoveRecords", "ClearClaim", "LiftPauses"},
			marker:  "left",
		},
		{
			name:   "FailedDiscardNamesTheCancelWithRemove",
			remove: true,
			setup: func(f *cancelFake) {
				f.parts = fabriclayout.PairParts{TaskBranch: true}
				f.discardErr = errors.New("archive tag push refused")
			},
			wantErr: `archive tag push refused; way forward: fix the cause it names, then run "lym batten cancel some-slug --remove" again`,
			noCalls: []string{"RemoveRecords", "ClearClaim", "LiftPauses"},
			marker:  "left",
		},
		{
			name: "StrayDirectoryAloneRefusesWithoutRemove",
			setup: func(f *cancelFake) {
				f.recordsPresent, f.boardStatus, f.boardFound = false, nil, false
				f.parts = fabriclayout.PairParts{StrayPath: true}
			},
			wantErr: `a directory that is no worktree is all that is left of some-slug, at its task worktree path; way forward: run "lym batten cancel some-slug --remove", which deletes it`,
			noCalls: append([]string{"TakeMarker"}, destructiveCancelCalls...),
			marker:  "none",
		},
		{
			name:   "StrayDirectoryAloneWithRemoveDeletesIt",
			remove: true,
			setup: func(f *cancelFake) {
				f.recordsPresent, f.boardStatus, f.boardFound, f.statusFound = false, nil, false, false
				f.parts = fabriclayout.PairParts{StrayPath: true}
				f.removeErr = fmt.Errorf("remove: %w", fabricengine.ErrPairNotFound)
				f.removeResult = fabriclayout.RemoveResult{StrayPathDeleted: true}
			},
			wantCalls: []string{"TakeMarker", "EndForCancel", "RemovePair", "RemoveRecords", "ClearClaim"},
			noCalls:   []string{"DiscardTaskBranch"},
			marker:    "removed",
			check: func(t *testing.T, r cancelReport, f *cancelFake) {
				if !f.removeRequest.ClearStray || !r.PairRemoved || !r.StrayDirectoryDeleted {
					t.Errorf("RemovePair request ClearStray, report pair removed, stray directory deleted = %v, %v, %v; want true, true, true", f.removeRequest.ClearStray, r.PairRemoved, r.StrayDirectoryDeleted)
				}
			},
		},
		{
			name:   "RemoveWithNoLocalPartLeftReadsPairNotFoundAsDone",
			remove: true,
			setup: func(f *cancelFake) {
				f.removeErr = fmt.Errorf("remove: %w", fabricengine.ErrPairNotFound)
			},
			wantCalls: []string{"RemovePair", "SettleTeardown", "RemoveRecords", "ClearClaim"},
			noCalls:   []string{"DiscardTaskBranch"},
			marker:    "removed",
			check: func(t *testing.T, r cancelReport, _ *cancelFake) {
				if !r.PairRemoved {
					t.Error("PairRemoved = false; want a pair with nothing left read as removed")
				}
				if r.StrayDirectoryDeleted {
					t.Error("StrayDirectoryDeleted = true; want false when the removal deleted no directory")
				}
			},
		},
		{
			name: "TakeBattenLocksBusyOnceThenFree",
			setup: func(f *cancelFake) {
				f.lockAnswers = []string{"/hub/run.lock", ""}
			},
			wantCalls: []string{"TakeBattenLocks", "TakeBattenLocks", "FinishCancel"},
			marker:    "removed",
			check: func(t *testing.T, _ cancelReport, f *cancelFake) {
				if f.lockTakenHeld {
					t.Error("TakeBattenLocks was called while an earlier call's locks were held; want a busy report to hold none")
				}
			},
		},
		{
			name:   "FailingNoticeOnlyWarns",
			setup:  func(f *cancelFake) { f.notifyErr = errors.New("orch is away") },
			marker: "removed",
			check: func(t *testing.T, r cancelReport, _ *cancelFake) {
				if r.NoticePosted || !r.RecordKept {
					t.Errorf("report = %+v; want the cancel done with the notice unposted", r)
				}
			},
		},
		{
			name:    "RecordsRemovalFailureWithRemoveStopsBeforeTheClaim",
			remove:  true,
			setup:   func(f *cancelFake) { f.recordsErr = errors.New("commit failed") },
			wantErr: `could not remove the records of some-slug: commit failed; way forward: fix the cause, then run "lym batten cancel some-slug --remove" again`,
			noCalls: []string{"ClearClaim", "LiftPauses"},
			marker:  "left",
		},
		{
			name:      "TaskNeverReachesTheBeamSeams",
			wantCalls: []string{"BeamProject", "EndForCancel", "FinishCancel"},
			noCalls:   []string{"ReconcileHub", "LandRefusal", "LandingHeld", "LandBlockers", "StopBeam"},
			marker:    "removed",
		},
		{
			name: "BeamWithMembersRefusesNamingThemWithNothingTakenOrEnded",
			setup: func(f *cancelFake) {
				f.beamProject = true
				f.landRefusal = errors.New("battencli: the beam some-slug cannot land while work remains under it; way forward: the open entry m is a member of the beam: land it with `lym batten run m`, or end it with `lym batten cancel m --remove`")
			},
			wantErr:     "the open entry m is a member of the beam",
			wantErrAlso: []string{"lym batten run m", `then run "lym batten cancel some-slug" again`},
			wantCalls:   []string{"BeamProject", "ReconcileHub", "LandRefusal", "LandingHeld"},
			noCalls:     append([]string{"TakeMarker", "LandBlockers", "StopBeam"}, destructiveCancelCalls...),
			marker:      "none",
		},
		{
			name: "BeamHeldByItsPawlRunNamesTheMembersCancelAndTheBoardMoveNeverItsRun",
			setup: func(f *cancelFake) {
				f.beamProject = true
				f.landingHeld = true
				f.landRefusal = errors.New("battencli: the beam some-slug cannot land while work remains under it; way forward: land it with `lym batten run m`")
				f.landBlockers = BeamLandBlockers{
					Members: []BeamLandMember{
						{Slug: "m", HasPair: true},
						{Slug: "k", Running: true},
						{Slug: "n"},
					},
					ChildPairs: []string{"p"},
				}
			},
			wantErr: "the open entry m is a member of the beam: end it with `lym batten cancel m --remove`, run in the beam's parent worktree, then move its `project` off the beam with `lym board upsert`",
			wantErrAlso: []string{
				"the open entry k is a member of the beam: end it with `lym batten cancel k --remove`",
				"the open entry n is a member of the beam with no run status: move its `project` off the beam with `lym board upsert`",
				"the pair p was forked from the beam: end it with `lym batten cancel p --remove`",
				`then run "lym batten cancel some-slug" again`,
			},
			errAbsent: []string{"lym batten run"},
			wantCalls: []string{"BeamProject", "ReconcileHub", "LandRefusal", "LandingHeld", "LandBlockers"},
			noCalls:   append([]string{"TakeMarker", "StopBeam"}, destructiveCancelCalls...),
			marker:    "none",
		},
		{
			name:        "BeamWithOriginUnreachableRefusesNamingTheReRun",
			setup:       func(f *cancelFake) { f.beamProject = true; f.hubReport = fabriclayout.OriginReport{Unreachable: true} },
			wantErr:     "the members of the beam some-slug could not be read against it",
			wantErrAlso: []string{`run "lym batten cancel some-slug" once origin answers`},
			noCalls:     append([]string{"TakeMarker", "LandRefusal", "StopBeam"}, destructiveCancelCalls...),
			marker:      "none",
		},
		{
			name:        "BeamWhoseHubReconcileFailsRefusesNamingTheReRun",
			setup:       func(f *cancelFake) { f.beamProject = true; f.hubReconcileErr = errors.New("git failed") },
			wantErr:     "the origin reconcile of the hub failed: git failed",
			wantErrAlso: []string{`then run "lym batten cancel some-slug" again`},
			noCalls:     append([]string{"TakeMarker", "LandRefusal", "StopBeam"}, destructiveCancelCalls...),
			marker:      "none",
		},
		{
			name:      "BeamWithNoMembersEndsAndStopsItsOrchAfterTheEndKeepingItsFlag",
			setup:     func(f *cancelFake) { f.beamProject = true },
			wantCalls: []string{"BeamProject", "ReconcileHub", "LandRefusal", "TakeMarker", "EndForCancel", "StopTackler", "StopBeam", "FinishCancel", "ClearClaim"},
			noCalls:   []string{"LandBlockers", "EndBeam"},
			marker:    "removed",
			check: func(t *testing.T, r cancelReport, _ *cancelFake) {
				if r.BeamEnded {
					t.Error("BeamEnded = true; want the flag kept by a cancel that keeps the pair")
				}
			},
		},
		{
			name:      "RemoveOfABeamClearsItsFlagAfterTheClaim",
			remove:    true,
			setup:     func(f *cancelFake) { f.beamProject = true },
			wantCalls: []string{"StopBeam", "RemovePair", "RemoveRecords", "ClearClaim", "LiftPauses", "PushRecords", "EndBeam", "Notify"},
			marker:    "removed",
			check: func(t *testing.T, r cancelReport, _ *cancelFake) {
				if !r.PairRemoved || !r.ClaimCleared {
					t.Errorf("report = %+v; want the pair removed and the claim cleared", r)
				}
				notice := cancelNotice(cancelReport{Slug: cancelSlug, PairRemoved: true, BeamEnded: true})
				if !strings.Contains(notice, "its beam flag cleared, leaving a plain project") {
					t.Errorf("notice = %q; want it to name the cleared beam flag", notice)
				}
			},
		},
		{
			name:   "BeamFlagAloneIsLeftToClearWithRemove",
			remove: true,
			setup: func(f *cancelFake) {
				f.beamProject = true
				f.recordsPresent, f.boardStatus, f.boardFound, f.statusFound = false, nil, false, false
			},
			wantCalls: []string{"TakeMarker", "EndForCancel", "RemovePair", "ClearClaim", "EndBeam"},
			noCalls:   []string{"TeardownPending"},
			marker:    "removed",
		},
		{
			name: "BeamFlagAloneRefusesWithoutRemove",
			setup: func(f *cancelFake) {
				f.beamProject = true
				f.recordsPresent, f.boardStatus, f.boardFound, f.statusFound = false, nil, false, false
			},
			wantErr:     `nothing of some-slug is left to cancel`,
			nothingLeft: true,
			noCalls:     append([]string{"TakeMarker"}, destructiveCancelCalls...),
			marker:      "none",
		},
		{
			name:   "RemoveOfABeamWithOnlyIdleMembersEndsItAndItsFlag",
			remove: true,
			setup: func(f *cancelFake) {
				f.beamProject = true
				f.landRefusal = errors.New("battencli: the beam some-slug cannot land")
				f.landBlockers = BeamLandBlockers{Members: []BeamLandMember{{Slug: "n"}}}
			},
			wantCalls: []string{"LandRefusal", "LandBlockers", "TakeMarker", "EndForCancel", "EndBeam"},
			marker:    "removed",
		},
		{
			name:   "RemoveOfAHeldBeamWithOnlyIdleMembersEndsIt",
			remove: true,
			setup: func(f *cancelFake) {
				f.beamProject, f.landingHeld = true, true
				f.landRefusal = errors.New("battencli: the beam some-slug cannot land")
				f.landBlockers = BeamLandBlockers{Members: []BeamLandMember{{Slug: "n"}}}
			},
			wantCalls: []string{"LandingHeld", "LandBlockers", "EndForCancel", "EndBeam"},
			marker:    "removed",
		},
		{
			name:   "RemoveOfABeamRefusesOnlyWhatStillWorksUnderIt",
			remove: true,
			setup: func(f *cancelFake) {
				f.beamProject = true
				f.landRefusal = errors.New("battencli: the beam some-slug cannot land")
				f.landBlockers = BeamLandBlockers{Members: []BeamLandMember{{Slug: "m", HasPair: true}, {Slug: "n"}, {Slug: "s", Beam: true}}}
			},
			wantErr:     "the open entry m is a member of the beam",
			wantErrAlso: []string{"the open entry s is a member of the beam", `then run "lym batten cancel some-slug --remove" again`},
			errAbsent:   []string{"the open entry n "},
			noCalls:     append([]string{"TakeMarker", "StopBeam"}, destructiveCancelCalls...),
			marker:      "none",
		},
		{
			name:   "RemoveOfAHeldBeamNamesNoMoveForAMemberItCancels",
			remove: true,
			setup: func(f *cancelFake) {
				f.beamProject, f.landingHeld = true, true
				f.landRefusal = errors.New("battencli: the beam some-slug cannot land")
				f.landBlockers = BeamLandBlockers{Members: []BeamLandMember{{Slug: "k", Running: true}, {Slug: "n"}}}
			},
			wantErr:   "the open entry k is a member of the beam: end it with `lym batten cancel k --remove`",
			errAbsent: []string{"the open entry n ", "move its `project`", "lym batten run"},
			noCalls:   append([]string{"TakeMarker", "StopBeam"}, destructiveCancelCalls...),
			marker:    "none",
		},
		{
			name: "FailingStopBeamWarnsAndTheCancelSucceeds",
			setup: func(f *cancelFake) {
				f.beamProject = true
				f.stopBeamErr = errors.New("orch is busy")
			},
			marker: "removed",
			check: func(t *testing.T, r cancelReport, _ *cancelFake) {
				if len(r.Warnings) != 1 || !strings.Contains(r.Warnings[0], "orch is busy") || !r.RecordKept || !r.ClaimCleared {
					t.Errorf("report = %+v; want the cancel done with one warning naming the stop failure", r)
				}
			},
		},
	}

	for _, unreachable := range []struct {
		name  string
		setup func(*cancelFake)
	}{
		{"ReconcileUnreachable", func(f *cancelFake) { f.worktreePresent = true; f.report = fabriclayout.OriginReport{Unreachable: true} }},
		{"PullUnreachable", func(f *cancelFake) { f.pullErr = pullUnreachable }},
	} {
		for _, remove := range []bool{false, true} {
			name := fmt.Sprintf("%s_Remove%v_EndsOnly", unreachable.name, remove)
			rows = append(rows, row{
				name:      name,
				remove:    remove,
				judging:   "endonly",
				setup:     func(f *cancelFake) { unreachable.setup(f); f.parts = fabriclayout.PairParts{TaskBranch: true} },
				wantCalls: []string{"Owner", "WriteIntent", "CommitRecord", "EndForCancel", "StopTackler", "Notify"},
				noCalls:   []string{"RemovePair", "DiscardTaskBranch", "FinishCancel", "RemoveRecords", "ClearClaim", "LiftPauses", "PushRecords"},
				marker:    "left",
				check: func(t *testing.T, r cancelReport, f *cancelFake) {
					if !r.EndOnly || r.RecordsRemoved || r.ClaimCleared {
						t.Errorf("report = %+v; want end-only with nothing removed or cleared", r)
					}
					if !strings.Contains(f.notices[0], `run "lym batten cancel some-slug`) || !strings.Contains(f.notices[0], "once origin answers") {
						t.Errorf("notice = %q; want it to name the cancel to run once origin answers", f.notices[0])
					}
					if f.locksHeld != 0 {
						t.Errorf("batten locks held after the end-only cancel = %d; want 0", f.locksHeld)
					}
				},
			})
		}
		rows = append(rows, row{
			name:    unreachable.name + "_OwnedElsewhereRefuses",
			setup:   func(f *cancelFake) { unreachable.setup(f); f.owner = "other-machine" },
			wantErr: "is owned by machine other-machine",
			noCalls: append([]string{"TakeMarker"}, destructiveCancelCalls...),
			marker:  "none",
		})
	}

	for _, tt := range rows {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			fake := newCancelFake()
			if tt.setup != nil {
				tt.setup(fake)
			}

			report, err := cancelRun(context.Background(), cancelSlug, tt.remove, fake.deps())

			switch {
			case tt.wantErr == "" && err != nil:
				t.Fatalf("cancelRun() error = %v; want nil", err)
			case tt.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErr)):
				t.Fatalf("cancelRun() error = %v; want it to contain %q", err, tt.wantErr)
			}
			for _, want := range tt.wantErrAlso {
				if err == nil || !strings.Contains(err.Error(), want) {
					t.Errorf("cancelRun() error = %v; want it to contain %q", err, want)
				}
			}
			for _, absent := range tt.errAbsent {
				if err != nil && strings.Contains(err.Error(), absent) {
					t.Errorf("cancelRun() error = %q; want it to never carry %q", err.Error(), absent)
				}
			}
			if err != nil {
				if got := errors.Is(err, ErrNothingLeft); got != tt.nothingLeft {
					t.Errorf("errors.Is(%q, ErrNothingLeft) = %v; want %v", err, got, tt.nothingLeft)
				}
			}
			if tt.judging != "" {
				judged := newCancelFake()
				tt.setup(judged)
				judgedErr := cancelJudgingEndOnly(context.Background(), cancelSlug, tt.remove, judged.deps())
				switch {
				case tt.judging == "full" && judgedErr != nil:
					t.Errorf("cancelJudgingEndOnly() = %v; want nil for a full cancel", judgedErr)
				case tt.judging == "endonly":
					if judgedErr == nil || !strings.Contains(judgedErr.Error(), "origin was unreachable") || !strings.Contains(judgedErr.Error(), cancelCommand(cancelSlug, tt.remove)+" again") || errors.Is(judgedErr, ErrNothingLeft) {
						t.Errorf("cancelJudgingEndOnly() = %v; want an origin-unreachable error naming %q that does not match ErrNothingLeft", judgedErr, cancelCommand(cancelSlug, tt.remove))
					}
					if judged.called("ClearClaim") || judged.called("RemoveRecords") {
						t.Errorf("calls = %v; want the end-only cancel to leave the run status and the run record", judged.calls)
					}
				}
			}
			if !callsInOrder(fake.calls, tt.wantCalls) {
				t.Errorf("calls = %v; want %v in order", fake.calls, tt.wantCalls)
			}
			for _, name := range tt.noCalls {
				if fake.called(name) {
					t.Errorf("calls = %v; want no %s call", fake.calls, name)
				}
			}
			switch tt.marker {
			case "none":
				if fake.markerReleases != 0 || fake.called("TakeMarker") {
					t.Errorf("lock taken, released = %v, %d; want it never taken", fake.called("TakeMarker"), fake.markerReleases)
				}
			case "left", "removed":
				if fake.markerReleases != 1 {
					t.Errorf("lock releases = %d; want exactly one", fake.markerReleases)
				}
			}
			if tt.check != nil {
				tt.check(t, report, fake)
			}
		})
	}
}

// TestCancelVerdict pins which runs the act rule refuses a cancel: only a landed run and one another machine owns.
// A cancel resolves a shed pause, a pause intent, a covering pause, a cancel under way, a hold and an owed push itself.
func TestCancelVerdict(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	started := func(status shedengine.Status) runfsm.Records {
		return runfsm.Records{Batten: status, BattenFound: true, BattenStarted: true}
	}
	running := runningStatus(battenrecipe.NameRunShed)
	tests := []struct {
		name       string
		records    runfsm.Records
		inputs     runfsm.Inputs
		wantAnswer runfsm.Answer
		wantReason runfsm.Reason
	}{
		{name: "RunningRunActs", records: started(running), wantAnswer: runfsm.AnswerAct},
		{name: "ShedPausedRunActs", records: started(shedengine.Status{CurrentProducer: battenrecipe.NameRunShed, State: shedengine.StatePaused}), wantAnswer: runfsm.AnswerAct},
		{
			name:       "PauseIntentAndCoveringPauseAct",
			records:    started(shedengine.Status{CurrentProducer: battenrecipe.NameRunShed, State: shedengine.StateRunning, PauseIntent: &shedengine.PauseIntent{}}),
			inputs:     runfsm.Inputs{PauseCovers: true},
			wantAnswer: runfsm.AnswerAct,
		},
		{
			name:       "CancelUnderWayActs",
			records:    started(shedengine.Status{CurrentProducer: battenrecipe.NameRunShed, State: shedengine.StateRunning, Cancel: &shedengine.CancelIntent{}}),
			wantAnswer: runfsm.AnswerAct,
		},
		{name: "HeldRunWithAnOwedPushActs", records: started(running), inputs: runfsm.Inputs{Held: true, PushOwed: true}, wantAnswer: runfsm.AnswerAct},
		{name: "LandedSlugWaitsAsTerminal", records: runfsm.Records{BoardDone: true}, wantAnswer: runfsm.AnswerWait, wantReason: runfsm.ReasonTerminal},
		{name: "OwnedElsewhereIsHandedOver", records: started(running), inputs: runfsm.Inputs{OwnedElsewhere: true}, wantAnswer: runfsm.AnswerHandOver, wantReason: runfsm.ReasonOwnedElsewhere},
		{name: "MissingCodeTipWaits", records: started(running), inputs: runfsm.Inputs{CodeTipMissing: true}, wantAnswer: runfsm.AnswerWait, wantReason: runfsm.ReasonCodeTip},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := cancelVerdict(tt.records, tt.inputs)

			if got.Answer != tt.wantAnswer || got.Reason != tt.wantReason {
				t.Errorf("cancelVerdict() answer, reason = %q, %q; want %q, %q", got.Answer, got.Reason, tt.wantAnswer, tt.wantReason)
			}
		})
	}
}

// TestCommitCancelRecord asserts the cancel's commit of batten's record:
// it commits and pushes, and pushes nothing when nothing was committed.
// It refuses while the records side is mid-merge.
// It returns a failed merge-state read, commit or push that is not owed.
// A push that is owed returns nil with the commit made, so the cancel goes on and its push stays owed.
func TestCommitCancelRecord(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	boom := errors.New("boom")
	owed := &fabricengine.PushFailedError{Record: fabriclayout.FailedPush{Class: fabriclayout.PushFailureTransport}, Err: boom}
	tests := []struct {
		name          string
		mergeActive   bool
		mergeErr      error
		committed     bool
		commitErr     error
		pushErr       error
		wantCommitted bool
		wantCommits   int
		wantPushes    int
		wantErr       string
	}{
		{name: "CommitsAndPushes", committed: true, wantCommitted: true, wantCommits: 1, wantPushes: 1},
		{name: "NothingCommittedPushesNothing", wantCommits: 1},
		{name: "MidMergeRefusesAndCommitsNothing", mergeActive: true, wantErr: "a merge is in progress on the prime's records side"},
		{name: "FailedMergeStateReadIsReturned", mergeErr: boom, wantErr: "boom"},
		{name: "FailedCommitIsReturned", commitErr: boom, wantCommits: 1, wantErr: "boom"},
		{name: "OwedPushKeepsTheCommitAndReturnsNil", committed: true, pushErr: owed, wantCommitted: true, wantCommits: 1, wantPushes: 1},
		{name: "FailedPushIsReturned", committed: true, pushErr: boom, wantCommitted: true, wantCommits: 1, wantPushes: 1, wantErr: "boom"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var commits, pushes int
			committed, err := commitCancelRecord(commitStatusDeps{
				MergeActive: func() (bool, error) { return tt.mergeActive, tt.mergeErr },
				Commit:      func(string) (bool, error) { commits++; return tt.committed, tt.commitErr },
				Push:        func() error { pushes++; return tt.pushErr },
			}, "a-slug")

			if (err == nil) != (tt.wantErr == "") || (err != nil && !strings.Contains(err.Error(), tt.wantErr)) {
				t.Errorf("commitCancelRecord() error = %v; want it to contain %q", err, tt.wantErr)
			}
			if committed != tt.wantCommitted || commits != tt.wantCommits || pushes != tt.wantPushes {
				t.Errorf("committed, commits, pushes = %v, %d, %d; want %v, %d, %d", committed, commits, pushes, tt.wantCommitted, tt.wantCommits, tt.wantPushes)
			}
		})
	}
}

// TestChildReachedParent pins which readings of batten's and the child's status say the work reached the parent.
func TestChildReachedParent(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	finalizeDone := shedengine.Status{State: shedengine.StateRunning, History: []shedengine.HistoryEntry{{Producer: loomshed.NameFinalize, Outcome: shedengine.Done}}}
	childDone := shedengine.Status{State: shedengine.StateDone}
	childRunning := shedengine.Status{State: shedengine.StateRunning}
	tests := []struct {
		name        string
		batten      shedengine.Status
		battenFound bool
		child       shedengine.Status
		childFound  bool
		pastMerge   bool
		want        bool
	}{
		{"ChildFinalizePastItsMerge", runningStatus(battenrecipe.NameRunShed), true, childRunning, true, true, true},
		{"ChildFinalizeDone", runningStatus(battenrecipe.NameRunShed), true, finalizeDone, true, false, true},
		{"BattenPastRunShedWithChildDone", runningStatus(battenrecipe.NameLand), true, childDone, true, false, true},
		{"BattenAtWorktreeTeardownWithChildNotFound", runningStatus(battenrecipe.NameWorktreeTeardown), true, shedengine.Status{}, false, false, true},
		{"BattenAtRunShedWithChildRunning", runningStatus(battenrecipe.NameRunShed), true, childRunning, true, false, false},
		{"BattenAtRunShedWithChildDone", runningStatus(battenrecipe.NameRunShed), true, childDone, true, false, false},
		{"BattenBlockedAtWorktreeCreateWithNoChild", runningStatus(battenrecipe.NameWorktreeCreate), true, shedengine.Status{}, false, false, false},
		{"NoStatusAtAll", shedengine.Status{}, false, shedengine.Status{}, false, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := childReachedParent(tt.batten, tt.battenFound, tt.child, tt.childFound, tt.pastMerge); got != tt.want {
				t.Errorf("childReachedParent() = %v; want %v", got, tt.want)
			}
		})
	}
}
