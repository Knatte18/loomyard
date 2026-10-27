//go:build tier0

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
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/pairteardown"
	"github.com/Knatte18/loomyard/internal/runproc"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

const cancelSlug = "some-slug"

// destructiveCancelCalls are the seams whose call means the cancel ended, removed, cleared or lifted something.
var destructiveCancelCalls = []string{"EndForCancel", "StopTackler", "RemovePair", "DiscardTaskBranch", "RemoveRecords", "ClearClaim", "LiftPauses"}

// cancelFake is a set of fake substrates for cancelRun that records every call and answers from its knobs.
// Its zero value is a blocked run at Worktree-Create with no pair, a run claim and records on the local board and disk, and nothing alive.
type cancelFake struct {
	calls []string

	hubPaused       bool
	worktreePresent bool
	report          fabricengine.OriginReport
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
	markerFresh     bool
	goos            string
	recordLists     [][]runproc.Record
	createdAt       map[int]time.Time
	parts           fabricengine.PairParts
	sessionLive     bool
	endResult       pairteardown.CancelResult
	endErr          error
	lockAnswers     []string
	tackler         string
	tacklerErr      error
	removeResult    fabricengine.RemoveResult
	removeErr       error
	onOrigin        bool
	discardResult   fabricengine.DiscardResult
	discardErr      error
	recordsErr      error
	recordsCommit   bool
	claimCleared    bool
	claimErr        error
	lifted          []string
	notifyErr       error

	// beamProject, hubReport, hubReconcileErr, landRefusal, landingHeld, landBlockers and stopBeamErr answer the beam seams; the zero value is a task.
	beamProject     bool
	hubReport       fabricengine.OriginReport
	hubReconcileErr error
	landRefusal     error
	landingHeld     bool
	landBlockers    BeamLandBlockers
	stopBeamErr     error

	endRequest      pairteardown.CancelRequest
	removeRequest   pairteardown.Request
	markerReleases  []bool
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
		markerFresh:    true,
		machine:        "this-machine",
		claimCleared:   true,
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
		Reconcile: func() (fabricengine.OriginReport, error) {
			f.call("Reconcile")
			return f.report, f.reconcileErr
		},
		PullBolt: func() error { f.call("PullBolt"); return f.pullErr },
		Owner:    func() (string, string, string, error) { f.call("Owner"); return f.machine, f.owner, f.subject, nil },
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
		TakeMarker: func() (func(bool) error, bool, error) {
			f.call("TakeMarker")
			if f.takeMarkerErr != nil {
				return nil, false, f.takeMarkerErr
			}
			return func(remove bool) error { f.markerReleases = append(f.markerReleases, remove); return nil }, f.markerFresh, nil
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
		PairParts:   func() (fabricengine.PairParts, error) { f.call("PairParts"); return f.parts, nil },
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
		RemovePair: func(req pairteardown.Request) (fabricengine.RemoveResult, error) {
			f.call("RemovePair")
			f.removeRequest = req
			return f.removeResult, f.removeErr
		},
		TaskBranchOnOrigin: func() (bool, error) { f.call("TaskBranchOnOrigin"); return f.onOrigin, nil },
		DiscardTaskBranch: func() (fabricengine.DiscardResult, error) {
			f.call("DiscardTaskBranch")
			return f.discardResult, f.discardErr
		},
		RemoveRecords: func() (bool, error) { f.call("RemoveRecords"); return f.recordsCommit, f.recordsErr },
		ClearClaim:    func() (bool, bool, error) { f.call("ClearClaim"); return f.claimCleared, f.boardFound, f.claimErr },
		LiftPauses:    func() ([]string, error) { f.call("LiftPauses"); return f.lifted, nil },
		SpawnPush:     func() error { f.call("SpawnPush"); return nil },
		Notify: func(line string) error {
			f.call("Notify")
			f.notices = append(f.notices, line)
			return f.notifyErr
		},
		BeamProject: func() (bool, error) { f.call("BeamProject"); return f.beamProject, nil },
		ReconcileHub: func() (fabricengine.OriginReport, error) {
			f.call("ReconcileHub")
			return f.hubReport, f.hubReconcileErr
		},
		LandRefusal:  func() error { f.call("LandRefusal"); return f.landRefusal },
		LandingHeld:  func() (bool, error) { f.call("LandingHeld"); return f.landingHeld, nil },
		LandBlockers: func() (BeamLandBlockers, error) { f.call("LandBlockers"); return f.landBlockers, nil },
		StopBeam:     func() error { f.call("StopBeam"); return f.stopBeamErr },
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

// TestCancelRun drives cancelRun through the fakes; a refusal row asserts none of the destructive seams ran, bar the rows that name the seam that refused.
func TestCancelRun(t *testing.T) {
	t.Parallel()

	liveRecord := runproc.Record{PID: 4242, Role: "batten", RegisteredAt: time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)}
	entry := func(state fabricengine.EntryState) fabricengine.OriginReport {
		return fabricengine.OriginReport{Entries: []fabricengine.EntryOutcome{{Slug: cancelSlug, State: state, Owner: "other-machine", AdoptSubject: "some-beam", How: "done", WayForward: "fix it"}}}
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
		// wantCalls is an ordered subsequence of the calls made.
		wantCalls []string
		// noCalls are seams that must not have been called.
		noCalls []string
		// marker is "none" when no marker was taken, "left" when its lock was released and the file kept, and "removed" when removed.
		marker string
		check  func(*testing.T, cancelReport, *cancelFake)
	}

	rows := []row{
		{
			name:      "BlockedRunWithNoPair",
			wantCalls: []string{"HubPaused", "ExemptGate", "TaskWorktreePresent", "PullBolt", "Owner", "ReadStatus", "TakeMarker", "EndForCancel", "StopTackler", "RemoveRecords", "ClearClaim", "SpawnPush", "Notify"},
			noCalls:   []string{"Reconcile", "RemovePair", "DiscardTaskBranch"},
			marker:    "removed",
			setup:     func(f *cancelFake) { f.tackler = "guid-1"; f.lifted = []string{"some-slug"} },
			check: func(t *testing.T, r cancelReport, f *cancelFake) {
				if r.TacklerStopped != "guid-1" || !r.RecordsRemoved || !r.ClaimCleared || !r.PushSpawned || !r.NoticePosted || r.EndOnly {
					t.Errorf("report = %+v; want tackler, records, claim, push and notice done", r)
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
			name: "LiveRunWithPairKeepsIt",
			setup: func(f *cancelFake) {
				f.worktreePresent = true
				f.status = runningStatus(battenrecipe.NameRunShed)
				f.parts = fabricengine.PairParts{TaskWorktree: true, RecordsWorktree: true, RecordsBranch: true, TaskBranch: true}
				f.endResult = pairteardown.CancelResult{Ended: []runproc.Ended{{PID: 7, Role: "batten"}}, StrandsStopped: []string{"agent-1"}}
			},
			wantCalls: []string{"Reconcile", "EndForCancel", "StopTackler", "RemoveRecords", "ClearClaim"},
			noCalls:   []string{"PullBolt", "RemovePair", "DiscardTaskBranch"},
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
				f.parts = fabricengine.PairParts{TaskWorktree: true, TaskBranch: true}
				f.onOrigin = true
				f.removeResult = fabricengine.RemoveResult{SnapshotTag: "snapshot/some-slug/1"}
				f.discardResult = fabricengine.DiscardResult{ArchiveTags: []string{"archive/some-slug/abc"}}
			},
			wantCalls: []string{"EndForCancel", "RemovePair", "DiscardTaskBranch", "RemoveRecords", "ClearClaim"},
			marker:    "removed",
			check: func(t *testing.T, r cancelReport, f *cancelFake) {
				want := pairteardown.Request{Slug: cancelSlug, Force: true, Snapshot: true, Remote: true, LeaveClaim: true}
				if f.removeRequest.Slug != want.Slug || !f.removeRequest.Force || !f.removeRequest.Snapshot || !f.removeRequest.Remote || !f.removeRequest.LeaveClaim {
					t.Errorf("RemovePair request = %+v; want %+v", f.removeRequest, want)
				}
				if f.endRequest.Keep {
					t.Error("EndForCancel request kept the pair; want it released for the removal")
				}
				if !r.PairRemoved || r.SnapshotTag != "snapshot/some-slug/1" || !slices.Equal(r.ArchiveTags, []string{"archive/some-slug/abc"}) || !r.ClaimCleared {
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
			setup:   func(f *cancelFake) { f.worktreePresent = true; f.report = entry(fabricengine.EntryEnded) },
			wantErr: `some-slug was landed by another machine; way forward: run "lym cleanup --apply" from the prime`,
			noCalls: append([]string{"TakeMarker"}, destructiveCancelCalls...),
			marker:  "none",
		},
		{
			name:   "EndedWithNoRecordsHereGoesOn",
			remove: true,
			setup: func(f *cancelFake) {
				f.worktreePresent, f.report, f.recordsPresent = true, entry(fabricengine.EntryEnded), false
				f.parts = fabricengine.PairParts{TaskWorktree: true}
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
			setup:   func(f *cancelFake) { f.worktreePresent = true; f.report = entry(fabricengine.EntryOwnedElsewhere) },
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
				f.worktreePresent, f.report = true, fabricengine.OriginReport{Unreachable: true}
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
				f.report = entry(fabricengine.EntryStopped)
				f.report.Branches = []fabricengine.BranchOutcome{{Branch: "some-slug-records", Repo: "records", Slug: cancelSlug, State: fabricengine.BranchDiverged, WayForward: "reset it"}}
			},
			wantErr: `cannot cancel "some-slug": branch some-slug-records is diverged against origin; reset it; then run "lym batten cancel some-slug" again`,
			noCalls: append([]string{"TakeMarker"}, destructiveCancelCalls...),
			marker:  "none",
		},
		{
			name: "AnchorStoppedBranchLetsTheCancelThrough",
			setup: func(f *cancelFake) {
				f.worktreePresent = true
				f.report = entry(fabricengine.EntryStopped)
				f.report.Branches = []fabricengine.BranchOutcome{{Branch: "some-slug-records", Repo: "records", Slug: cancelSlug, State: fabricengine.BranchAnchorStopped, WayForward: "run the set"}}
			},
			marker: "removed",
		},
		{
			name: "AnchorStoppedWithADivergedBranchStillRefuses",
			setup: func(f *cancelFake) {
				f.worktreePresent = true
				f.report = entry(fabricengine.EntryStopped)
				f.report.Branches = []fabricengine.BranchOutcome{
					{Branch: "some-slug", Repo: "code", Slug: cancelSlug, State: fabricengine.BranchDiverged, WayForward: "reset it"},
					{Branch: "some-slug-records", Repo: "records", Slug: cancelSlug, State: fabricengine.BranchAnchorStopped, WayForward: "run the set"},
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
			name:    "WindowsLiveRecordAtFirstListRefusesAndRemovesAFreshMarker",
			setup:   func(f *cancelFake) { f.goos = "windows"; f.recordLists = [][]runproc.Record{nil, {liveRecord}} },
			wantErr: `pid 4242 (batten): taskkill /PID 4242 /T /F; way forward: end each with its taskkill line, then run "lym batten cancel some-slug" again`,
			noCalls: destructiveCancelCalls,
			marker:  "removed",
		},
		{
			name: "WindowsLiveRecordAtFirstListLeavesATakenOverMarker",
			setup: func(f *cancelFake) {
				f.goos, f.markerFresh = "windows", false
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
			wantCalls: []string{"EndForCancel", "RemoveRecords"},
			marker:    "removed",
		},
		{
			name: "NothingLeftRefuses",
			setup: func(f *cancelFake) {
				f.recordsPresent, f.boardStatus, f.boardFound, f.statusFound = false, nil, false, false
			},
			wantErr: `nothing of some-slug is left to cancel; way forward: check it with "lym board list"`,
			noCalls: append([]string{"TakeMarker"}, destructiveCancelCalls...),
			marker:  "none",
		},
		{
			name: "TaskBranchAloneRefusesWithoutRemove",
			setup: func(f *cancelFake) {
				f.recordsPresent, f.boardStatus, f.boardFound = false, nil, false
				f.parts = fabricengine.PairParts{TaskBranch: true}
			},
			wantErr: `run "lym batten cancel some-slug --remove", which archives and deletes its task branch`,
			noCalls: append([]string{"TakeMarker"}, destructiveCancelCalls...),
			marker:  "none",
		},
		{
			name:      "NothingLeftWithRemoveReadsOrigin",
			remove:    true,
			setup:     func(f *cancelFake) { f.recordsPresent, f.boardStatus, f.boardFound = false, nil, false },
			wantErr:   `nothing of some-slug is left to cancel`,
			wantCalls: []string{"TaskBranchOnOrigin"},
			noCalls:   append([]string{"TakeMarker"}, destructiveCancelCalls...),
			marker:    "none",
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
			name: "LiveMarkerWriterRefuses",
			setup: func(f *cancelFake) {
				f.marker = cancelMarkerReading{Present: true, WriterAlive: true, Marker: cancelMarker{PID: 99}}
			},
			wantErr: "a cancel of some-slug is under way (pid 99",
			noCalls: append([]string{"TakeMarker"}, destructiveCancelCalls...),
			marker:  "none",
		},
		{
			name:      "DeadMarkerWriterIsTakenOver",
			setup:     func(f *cancelFake) { f.marker = cancelMarkerReading{Present: true}; f.markerFresh = false },
			wantCalls: []string{"ReadMarker", "TakeMarker", "EndForCancel", "RemoveRecords"},
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
				f.parts = fabricengine.PairParts{TaskWorktree: true}
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
				f.parts = fabricengine.PairParts{TaskBranch: true}
				f.discardErr = errors.New("archive tag push refused")
			},
			wantErr: `archive tag push refused; way forward: fix the cause it names, then run "lym batten cancel some-slug --remove" again`,
			noCalls: []string{"RemoveRecords", "ClearClaim", "LiftPauses"},
			marker:  "left",
		},
		{
			name:   "RemoveWithNoLocalPartLeftReadsPairNotFoundAsDone",
			remove: true,
			setup: func(f *cancelFake) {
				f.removeErr = fmt.Errorf("remove: %w", fabricengine.ErrPairNotFound)
			},
			wantCalls: []string{"RemovePair", "RemoveRecords", "ClearClaim"},
			noCalls:   []string{"DiscardTaskBranch"},
			marker:    "removed",
			check: func(t *testing.T, r cancelReport, _ *cancelFake) {
				if !r.PairRemoved {
					t.Error("PairRemoved = false; want a pair with nothing left read as removed")
				}
			},
		},
		{
			name: "TakeBattenLocksBusyOnceThenFree",
			setup: func(f *cancelFake) {
				f.lockAnswers = []string{"/hub/run.lock", ""}
			},
			wantCalls: []string{"TakeBattenLocks", "TakeBattenLocks", "RemoveRecords"},
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
				if r.NoticePosted || !r.RecordsRemoved {
					t.Errorf("report = %+v; want the cancel done with the notice unposted", r)
				}
			},
		},
		{
			name:    "RecordsRemovalFailureStopsBeforeTheClaim",
			setup:   func(f *cancelFake) { f.recordsErr = errors.New("commit failed") },
			wantErr: `could not remove the records of some-slug: commit failed; way forward: fix the cause, then run "lym batten cancel some-slug" again`,
			noCalls: []string{"ClearClaim", "LiftPauses"},
			marker:  "left",
		},
		{
			name:      "TaskNeverReachesTheBeamSeams",
			wantCalls: []string{"BeamProject", "EndForCancel", "RemoveRecords"},
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
			setup:       func(f *cancelFake) { f.beamProject = true; f.hubReport = fabricengine.OriginReport{Unreachable: true} },
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
			name:      "BeamWithNoMembersEndsAndStopsItsOrchAfterTheEnd",
			setup:     func(f *cancelFake) { f.beamProject = true },
			wantCalls: []string{"BeamProject", "ReconcileHub", "LandRefusal", "TakeMarker", "EndForCancel", "StopTackler", "StopBeam", "RemoveRecords", "ClearClaim"},
			noCalls:   []string{"LandBlockers"},
			marker:    "removed",
		},
		{
			name: "FailingStopBeamWarnsAndTheCancelSucceeds",
			setup: func(f *cancelFake) {
				f.beamProject = true
				f.stopBeamErr = errors.New("orch is busy")
			},
			marker: "removed",
			check: func(t *testing.T, r cancelReport, _ *cancelFake) {
				if len(r.Warnings) != 1 || !strings.Contains(r.Warnings[0], "orch is busy") || !r.RecordsRemoved || !r.ClaimCleared {
					t.Errorf("report = %+v; want the cancel done with one warning naming the stop failure", r)
				}
			},
		},
	}

	for _, unreachable := range []struct {
		name  string
		setup func(*cancelFake)
	}{
		{"ReconcileUnreachable", func(f *cancelFake) { f.worktreePresent = true; f.report = fabricengine.OriginReport{Unreachable: true} }},
		{"PullUnreachable", func(f *cancelFake) { f.pullErr = pullUnreachable }},
	} {
		for _, remove := range []bool{false, true} {
			name := fmt.Sprintf("%s_Remove%v_EndsOnly", unreachable.name, remove)
			rows = append(rows, row{
				name:      name,
				remove:    remove,
				setup:     func(f *cancelFake) { unreachable.setup(f); f.parts = fabricengine.PairParts{TaskBranch: true} },
				wantCalls: []string{"Owner", "EndForCancel", "StopTackler", "Notify"},
				noCalls:   []string{"RemovePair", "DiscardTaskBranch", "RemoveRecords", "ClearClaim", "LiftPauses", "SpawnPush"},
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
				if len(fake.markerReleases) != 0 || fake.called("TakeMarker") {
					t.Errorf("marker taken, released = %v, %v; want it never taken", fake.called("TakeMarker"), fake.markerReleases)
				}
			case "left":
				if !slices.Equal(fake.markerReleases, []bool{false}) {
					t.Errorf("marker releases = %v; want one release that keeps the marker", fake.markerReleases)
				}
			case "removed":
				if !slices.Equal(fake.markerReleases, []bool{true}) {
					t.Errorf("marker releases = %v; want one release that removes the marker", fake.markerReleases)
				}
			}
			if tt.check != nil {
				tt.check(t, report, fake)
			}
		})
	}
}

// TestChildReachedParent pins which readings of batten's and the child's status say the work reached the parent.
func TestChildReachedParent(t *testing.T) {
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
