// landnotice_test.go pins the landing notice over a pass harness.
// It covers which runs and done entries get a line under each land_notice value.
// It covers the line's text and the request's clearing.
// It covers a beam with a recorded pull request flagging its members and adding its follow-up to their landed line.
// It covers the owed line's posts until one succeeds, and the memory's upkeep.

package dobbyengine

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestLandNotices(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	const commit = "0123456789abcdef0123456789abcdef01234567"
	running := RunView{Slug: "task", State: runStateRunning, Producer: producerRunShed}
	landed := RunView{Slug: "task", State: runStateDone, Producer: "Worktree-Teardown", PastLand: true, Landed: true, LandingCommit: commit}
	tearingDown := RunView{Slug: "task", State: runStateRunning, Producer: "Worktree-Teardown", PastLand: true, Landed: true, LandingCommit: commit}
	unlanded := RunView{Slug: "task", State: runStateDone, Producer: "Worktree-Teardown", PastLand: true}
	landedNoCommit := RunView{Slug: "task", State: runStateDone, Producer: "Worktree-Teardown", PastLand: true, Landed: true}
	const landedLine = "[dobby notice] task: landed as 0123456789ab"
	const unknownCommitLine = "[dobby notice] task: landed, commit unknown"
	const memberFollowUp = "; add it to the change description, then run `lym beam describe b`, which updates the pull request"
	const memberLandedLine = landedLine + memberFollowUp
	const unlandedLine = "[dobby notice] task: ended without landing; lym batten status task shows the run"

	tests := []struct {
		name string
		mode string
		// requested is the board entry's notify_on_land while the run is ahead of Land, and the entry is pruned after.
		requested bool
		// requestedByTick, when set, replaces requested with the entry's notify_on_land at each tick.
		requestedByTick []bool
		// failBoardOnTick is the one-based tick whose board read fails; zero fails none.
		failBoardOnTick int
		// failConfigOnTick is the one-based tick whose beam.yaml read fails; zero fails none.
		failConfigOnTick int
		// rebuildBeforeTick is the one-based tick that a new daemon over the same state record runs; zero rebuilds none.
		rebuildBeforeTick int
		// doneEntryParent, when set, makes each tick with no run ahead of Land put a done entry of the slug on the board.
		// The entry carries notify_on_land and sits under this parent worktree until the request is cleared.
		doneEntryParent string
		// doneEntryOwnedElsewhere marks that done entry as owned by another machine.
		doneEntryOwnedElsewhere bool
		// beam makes the dobby the one of beam b, with the run's entry a member of b.
		beam bool
		// pullRequest is what b's landing record reads as holding, and failPullRequestOnTick the one-based tick whose read fails.
		pullRequest           bool
		failPullRequestOnTick int
		// pullRequestFromTick, when set, makes b's landing record hold a pull request from that one-based tick on.
		pullRequestFromTick int
		// entryProject, when set, replaces b as the project of the run's entry.
		entryProject string
		// entryDone makes the run's entry read done while the run is ahead of Land.
		entryDone bool
		// entryStaysOpen keeps the run's entry open on the board once the run is past Land, flagged until its request is cleared.
		entryStaysOpen bool
		// failFlags is how many SetNotifyOnLand calls fail before the seam succeeds.
		failFlags int
		// ticks are the run listings of successive ticks.
		ticks [][]RunView
		// failPosts is how many posts fail before the Notice seam succeeds.
		failPosts int
		// failClears is how many clears fail before the ClearNotifyOnLand seam succeeds.
		failClears int
		want       []string
		wantClears []string
		// wantFlags lists the slugs the SetNotifyOnLand seam was called for.
		wantFlags []string
		// wantPosts counts the calls of the Notice seam, failed ones included, when it differs from len(want).
		wantPosts int
	}{
		{name: "AllLandedNamesTheCommitOnce", mode: LandNoticeAll, ticks: [][]RunView{{running}, {landed}, {landed}}, want: []string{landedLine}},
		{name: "AllTeardownRowCountsAsPastLand", mode: LandNoticeAll, ticks: [][]RunView{{running}, {tearingDown}, {landed}}, want: []string{landedLine}},
		{name: "AllUnlandedNamesTheOutcome", mode: LandNoticeAll, ticks: [][]RunView{{running}, {unlanded}}, want: []string{unlandedLine}},
		{name: "AllLandedWithNoCommitRecorded", mode: LandNoticeAll, ticks: [][]RunView{{running}, {landedNoCommit}}, want: []string{unknownCommitLine}},
		{name: "AllFirstSeenPastLandSendsNothing", mode: LandNoticeAll, ticks: [][]RunView{{landed}, {landed}}},
		{name: "PerTaskRequestedSendsAndClearsAfterThePrune", mode: LandNoticePerTask, requested: true, ticks: [][]RunView{{running}, {landed}}, want: []string{landedLine}, wantClears: []string{"task"}},
		{name: "PerTaskUnrequestedSendsNothing", mode: LandNoticePerTask, ticks: [][]RunView{{running}, {landed}}},
		{name: "OffSendsNothingAndClearsTheRequest", mode: LandNoticeOff, requested: true, ticks: [][]RunView{{running}, {landed}}, wantClears: []string{"task"}},
		{name: "AllRequestedClearsTheRequest", mode: LandNoticeAll, requested: true, ticks: [][]RunView{{running}, {landed}}, want: []string{landedLine}, wantClears: []string{"task"}},
		{name: "AFailedPostIsOwedUntilItGoesOut", mode: LandNoticeAll, ticks: [][]RunView{{running}, {landed}, {landed}, {landed}, {landed}, {landed}}, failPosts: 4, want: []string{landedLine}, wantPosts: 5},
		{name: "AnOwedLineOutlivesTheRunsListing", mode: LandNoticeAll, ticks: [][]RunView{{running}, {landed}, {}}, failPosts: 1, want: []string{landedLine}, wantPosts: 2},
		{name: "PerTaskFollowsARequestWithdrawnBetweenTicks", mode: LandNoticePerTask, requestedByTick: []bool{true, false}, ticks: [][]RunView{{running}, {running}, {landed}}},
		{name: "PerTaskFollowsARequestRaisedBetweenTicks", mode: LandNoticePerTask, requestedByTick: []bool{false, true}, ticks: [][]RunView{{running}, {running}, {landed}}, want: []string{landedLine}, wantClears: []string{"task"}},
		{name: "AFailedBoardReadKeepsTheFirstReading", mode: LandNoticePerTask, requestedByTick: []bool{true, false}, failBoardOnTick: 2, ticks: [][]RunView{{running}, {running}, {landed}}, want: []string{landedLine}, wantClears: []string{"task"}},
		{name: "ARunNoLongerListedIsForgotten", mode: LandNoticeAll, ticks: [][]RunView{{running}, {}, {landed}}},
		{name: "ADoneEntryWithNoRunSeenIsNoticedAndCleared", mode: LandNoticePerTask, doneEntryParent: "prime", ticks: [][]RunView{{}, {}}, want: []string{unknownCommitLine}, wantClears: []string{"task"}},
		{name: "ADoneEntryWithItsRunFirstSeenPastLandNamesTheCommit", mode: LandNoticePerTask, doneEntryParent: "prime", ticks: [][]RunView{{landed}, {landed}}, want: []string{landedLine}, wantClears: []string{"task"}},
		{name: "ADoneEntryUnderOffIsClearedWithNoLine", mode: LandNoticeOff, doneEntryParent: "prime", ticks: [][]RunView{{}}, wantClears: []string{"task"}},
		{name: "ADoneEntryOfAnotherWorktreeIsLeftToItsDobby", mode: LandNoticeAll, doneEntryParent: "beam-x", ticks: [][]RunView{{}}},
		{name: "ARequestRaisedAsTheRunLandsIsSentOnce", mode: LandNoticePerTask, doneEntryParent: "prime", ticks: [][]RunView{{running}, {landed}, {landed}}, want: []string{landedLine}, wantClears: []string{"task"}},
		{name: "AFailedPostOfADoneEntryIsOwedUntilItGoesOut", mode: LandNoticePerTask, doneEntryParent: "prime", ticks: [][]RunView{{}, {}, {}}, failPosts: 2, want: []string{unknownCommitLine}, wantClears: []string{"task"}, wantPosts: 3},
		{name: "ADoneEntryOwnedByAnotherMachineIsLeftToItsDobby", mode: LandNoticeAll, doneEntryParent: "prime", doneEntryOwnedElsewhere: true, ticks: [][]RunView{{}, {}}},
		{name: "AFailedClearIsRetriedWithoutAnotherPost", mode: LandNoticePerTask, requested: true, failClears: 1, ticks: [][]RunView{{running}, {landed}, {landed}}, want: []string{landedLine}, wantClears: []string{"task", "task"}},
		{name: "AFailedModeReadDecidesNothingUntilItReads", mode: LandNoticePerTask, requested: true, failConfigOnTick: 2, ticks: [][]RunView{{running}, {landed}, {landed}}, want: []string{landedLine}, wantClears: []string{"task"}},
		{name: "ADoneEntryIsNotTakenUpWhileTheModeReadFails", mode: LandNoticePerTask, doneEntryParent: "prime", failConfigOnTick: 1, ticks: [][]RunView{{}, {}}, want: []string{unknownCommitLine}, wantClears: []string{"task"}},
		{name: "AnOwedLineIsPostedOnceByARebuiltDaemon", mode: LandNoticeAll, failPosts: 1, rebuildBeforeTick: 3, ticks: [][]RunView{{running}, {landed}, {landed}}, want: []string{landedLine}, wantPosts: 2},
		{name: "APullRequestFlagsAMemberAndItsLineCarriesTheFollowUp", mode: LandNoticePerTask, beam: true, pullRequest: true, ticks: [][]RunView{{running}, {landed}}, want: []string{memberLandedLine}, wantClears: []string{"task"}, wantFlags: []string{"task"}},
		{name: "AMemberFlaggedByHandGetsTheSameOneLine", mode: LandNoticePerTask, beam: true, pullRequest: true, requested: true, ticks: [][]RunView{{running}, {landed}}, want: []string{memberLandedLine}, wantClears: []string{"task"}},
		{name: "ABeamWithNoPullRequestFlagsNothing", mode: LandNoticePerTask, beam: true, ticks: [][]RunView{{running}, {landed}}},
		{name: "AnUnlandedMemberLineCarriesNoFollowUp", mode: LandNoticePerTask, beam: true, pullRequest: true, ticks: [][]RunView{{running}, {unlanded}}, want: []string{unlandedLine}, wantClears: []string{"task"}, wantFlags: []string{"task"}},
		{name: "AFailedLandingRecordReadDecidesNothingUntilItReads", mode: LandNoticePerTask, beam: true, pullRequest: true, requested: true, failPullRequestOnTick: 2, ticks: [][]RunView{{running}, {landed}, {landed}}, want: []string{memberLandedLine}, wantClears: []string{"task"}},
		{name: "AMemberWhoseEntryStaysOpenPastLandIsNotFlaggedAgain", mode: LandNoticePerTask, beam: true, pullRequest: true, entryStaysOpen: true, ticks: [][]RunView{{running}, {landed}, {landed}, {}}, want: []string{memberLandedLine}, wantClears: []string{"task"}, wantFlags: []string{"task"}},
		{name: "AMemberLandingInTheTickThePullRequestIsRecordedGetsTheLine", mode: LandNoticePerTask, beam: true, pullRequestFromTick: 2, ticks: [][]RunView{{running}, {landed}}, want: []string{memberLandedLine}},
		{name: "ANonMemberEntryIsNotFlagged", mode: LandNoticePerTask, beam: true, pullRequest: true, entryProject: "other", ticks: [][]RunView{{running}, {landed}}},
		{name: "ADoneMemberEntryIsNotFlagged", mode: LandNoticePerTask, beam: true, pullRequest: true, entryDone: true, ticks: [][]RunView{{running}, {landed}}, want: []string{memberLandedLine}},
		{name: "AFailedFlagWriteIsRetriedOnTheNextTick", mode: LandNoticePerTask, beam: true, pullRequest: true, failFlags: 1, ticks: [][]RunView{{running}, {running}, {landed}}, want: []string{memberLandedLine}, wantClears: []string{"task"}, wantFlags: []string{"task", "task"}},
		{name: "AMemberLandedWithNoCommitCarriesTheFollowUp", mode: LandNoticePerTask, beam: true, pullRequest: true, ticks: [][]RunView{{running}, {landedNoCommit}}, want: []string{unknownCommitLine + memberFollowUp}, wantClears: []string{"task"}, wantFlags: []string{"task"}},
		{name: "ADoneEntryTakenUpUnderAPullRequestCarriesTheFollowUp", mode: LandNoticePerTask, beam: true, pullRequest: true, doneEntryParent: "prime", ticks: [][]RunView{{}, {}}, want: []string{unknownCommitLine + memberFollowUp}, wantClears: []string{"task"}},
		{name: "OffWithAPullRequestFlagsTheMemberAndSendsNoLine", mode: LandNoticeOff, beam: true, pullRequest: true, ticks: [][]RunView{{running}, {landed}}, wantClears: []string{"task"}, wantFlags: []string{"task"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := newPassHarness(t)
			var tickNumber int
			h.d.deps.LoadConfig = func() (Config, error) {
				if tt.failConfigOnTick != 0 && tickNumber == tt.failConfigOnTick {
					return Config{}, errors.New("beam.yaml does not load")
				}
				return Config{MaxRuns: 3, LandNotice: tt.mode}, nil
			}
			var posts int
			var sent, clears []string
			h.d.deps.Notice = func(_ context.Context, line string) error {
				posts++
				if posts <= tt.failPosts {
					return errors.New("no orch strand is recorded")
				}
				sent = append(sent, line)
				return nil
			}
			cleared := false
			h.d.deps.ClearNotifyOnLand = func(slug string) error {
				clears = append(clears, slug)
				if len(clears) <= tt.failClears {
					return errors.New("the board is locked")
				}
				cleared = true
				return nil
			}
			project := ""
			if tt.beam {
				project = "b"
				h.d.geom.Slug = project
				h.d.deps.PullRequestRecorded = func() (bool, error) {
					if tickNumber == tt.failPullRequestOnTick {
						return false, errors.New("the landing record is unreadable")
					}
					return tt.pullRequest || (tt.pullRequestFromTick != 0 && tickNumber >= tt.pullRequestFromTick), nil
				}
			}
			var flags []string
			flagged := false
			h.d.deps.SetNotifyOnLand = func(slug string) error {
				flags = append(flags, slug)
				if len(flags) <= tt.failFlags {
					return errors.New("the board is locked")
				}
				flagged = true
				return nil
			}
			status, done := "running · Run-Shed", "done"
			h.d.deps.Tasks = func() ([]TaskView, error) {
				if tickNumber == tt.failBoardOnTick {
					return nil, errors.New("the board is unreadable")
				}
				return append([]TaskView(nil), h.fake.tasks...), nil
			}
			for i, runs := range tt.ticks {
				tickNumber = i + 1
				if tickNumber == tt.rebuildBeforeTick {
					h.d = &daemon{
						geom: h.d.geom, deps: h.d.deps, probe: h.d.probe,
						flights: map[string]flight{}, childSeen: map[string]time.Time{}, wasCovered: map[string]bool{}, liftPending: map[string]bool{},
					}
				}
				h.fake.runs = runs
				h.fake.tasks = nil
				requested := tt.requested
				if i < len(tt.requestedByTick) {
					requested = tt.requestedByTick[i]
				}
				switch {
				case slices.ContainsFunc(runs, func(run RunView) bool { return !run.PastLand }):
					entryStatus, entryProject := &status, project
					if tt.entryDone {
						entryStatus = &done
					}
					if tt.entryProject != "" {
						entryProject = tt.entryProject
					}
					h.fake.tasks = []TaskView{{Slug: "task", Kind: "task", Status: entryStatus, NotifyOnLand: requested || flagged, Project: entryProject}}
				case tt.entryStaysOpen:
					h.fake.tasks = []TaskView{{Slug: "task", Kind: "task", Status: &status, NotifyOnLand: flagged && !cleared, Project: project}}
				case tt.doneEntryParent != "" && !cleared:
					h.fake.tasks = []TaskView{{Slug: "task", Kind: "task", Status: &done, NotifyOnLand: true, Parent: tt.doneEntryParent, OwnedElsewhere: tt.doneEntryOwnedElsewhere}}
				}
				if tickNumber == tt.failBoardOnTick {
					// The tick's own start step reads the board after the notice step, so a failed read fails the tick;
					// the notice step has run by then.
					h.clock.sleep(context.Background(), 10*time.Second)
					if err := h.d.tick(context.Background(), h.clock.now()); err == nil {
						t.Fatalf("tick() = nil on a failed board read; want the start step's error")
					}
					continue
				}
				if tickNumber == tt.failConfigOnTick {
					// The tick may stop on the unreadable beam.yaml before or after its notice step; either way that step decides nothing.
					h.clock.sleep(context.Background(), 10*time.Second)
					_ = h.d.tick(context.Background(), h.clock.now())
					continue
				}
				h.tick(false)
			}

			if !slices.Equal(sent, tt.want) {
				t.Errorf("notices = %q; want %q", sent, tt.want)
			}
			if !slices.Equal(clears, tt.wantClears) {
				t.Errorf("cleared = %q; want %q", clears, tt.wantClears)
			}
			if !slices.Equal(flags, tt.wantFlags) {
				t.Errorf("flagged = %q; want %q", flags, tt.wantFlags)
			}
			wantPosts := tt.wantPosts
			if wantPosts == 0 {
				wantPosts = len(tt.want)
			}
			if posts != wantPosts {
				t.Errorf("Notice calls = %d; want %d", posts, wantPosts)
			}
			if memory := h.stateRecord().LandNotices; len(memory) != 0 {
				t.Errorf("state LandNotices = %+v; want none once the notice is settled", memory)
			}
		})
	}
}
