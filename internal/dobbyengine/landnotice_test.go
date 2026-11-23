// landnotice_test.go pins the landing notice over a pass harness.
// It covers which runs get a line under each reading of land_notice and the line's text.
// It covers a member landing under a recorded pull request, whose line carries no follow-up.
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
	landedEmpty := RunView{Slug: "task", State: runStateDone, Producer: "Worktree-Teardown", PastLand: true, Landed: true, LandingCommit: commit, EmptyLanding: true}
	const landedLine = "[dobby notice] task: landed as 0123456789ab"
	const emptyLandingLine = "[dobby notice] task: landed with an empty diff: no file of its parent branch changed, which stays at 0123456789ab"
	const unknownCommitLine = "[dobby notice] task: landed, commit unknown"
	const unlandedLine = "[dobby notice] task: ended without landing; lym batten status task shows the run"

	tests := []struct {
		name string
		// requested is what the land_notice lookup reads for the run while it is ahead of Land.
		requested bool
		// requestedByTick, when set, replaces requested with the lookup's reading at each tick.
		requestedByTick []bool
		// failLookupOnTick is the one-based tick whose land_notice lookup fails; zero fails none.
		failLookupOnTick int
		// failLookupThroughTick is the one-based tick through which every land_notice lookup fails; zero fails none.
		failLookupThroughTick int
		// seed is the memory the state record holds for the run before the first tick, nil leaving none.
		seed *LandNoticeState
		// rebuildBeforeTick is the one-based tick that a new daemon over the same state record runs; zero rebuilds none.
		rebuildBeforeTick int
		// beam makes the dobby the one of beam b, with the run's board entry a member of b while the run is ahead of Land.
		beam bool
		// pullRequest is what b's landing record reads as holding.
		pullRequest bool
		// ticks are the run listings of successive ticks.
		ticks [][]RunView
		// failPosts is how many posts fail before the Notice seam succeeds.
		failPosts int
		want      []string
		// wantPosts counts the calls of the Notice seam, failed ones included, when it differs from len(want).
		wantPosts int
	}{
		{name: "LandedNamesTheCommitOnce", requested: true, ticks: [][]RunView{{running}, {landed}, {landed}}, want: []string{landedLine}},
		{name: "TeardownRowCountsAsPastLand", requested: true, ticks: [][]RunView{{running}, {tearingDown}, {landed}}, want: []string{landedLine}},
		{name: "UnlandedNamesTheOutcome", requested: true, ticks: [][]RunView{{running}, {unlanded}}, want: []string{unlandedLine}},
		{name: "LandedWithNoCommitRecorded", requested: true, ticks: [][]RunView{{running}, {landedNoCommit}}, want: []string{unknownCommitLine}},
		{name: "EmptyLandingIsFlagged", requested: true, ticks: [][]RunView{{running}, {landedEmpty}}, want: []string{emptyLandingLine}},
		{name: "FirstSeenPastLandSendsNothing", requested: true, ticks: [][]RunView{{landed}, {landed}}},
		{name: "ALookupReadingOffSendsNothing", ticks: [][]RunView{{running}, {landed}}},
		{name: "AFailedPostIsOwedUntilItGoesOut", requested: true, ticks: [][]RunView{{running}, {landed}, {landed}, {landed}, {landed}, {landed}}, failPosts: 4, want: []string{landedLine}, wantPosts: 5},
		{name: "AnOwedLineOutlivesTheRunsListing", requested: true, ticks: [][]RunView{{running}, {landed}, {}}, failPosts: 1, want: []string{landedLine}, wantPosts: 2},
		{name: "ARunNoLongerListedIsForgotten", requested: true, ticks: [][]RunView{{running}, {}, {landed}}},
		{name: "AnOwedLineIsPostedOnceByARebuiltDaemon", requested: true, failPosts: 1, rebuildBeforeTick: 3, ticks: [][]RunView{{running}, {landed}, {landed}}, want: []string{landedLine}, wantPosts: 2},
		{name: "TheLookupFollowsARequestWithdrawnBetweenTicks", requestedByTick: []bool{true, false}, ticks: [][]RunView{{running}, {running}, {landed}}},
		{name: "TheLookupFollowsARequestRaisedBetweenTicks", requestedByTick: []bool{false, true}, ticks: [][]RunView{{running}, {running}, {landed}}, want: []string{landedLine}},
		{name: "AFailedLookupKeepsTheLastReading", requestedByTick: []bool{true, false}, failLookupOnTick: 2, ticks: [][]RunView{{running}, {running}, {landed}}, want: []string{landedLine}},
		{name: "AMemberWhoseLookupReadsOffGetsNoLineUnderAPullRequest", beam: true, pullRequest: true, ticks: [][]RunView{{running}, {landed}}},
		{name: "AMemberLandingUnderAPullRequestCarriesNoFollowUp", beam: true, pullRequest: true, requested: true, ticks: [][]RunView{{running}, {landed}}, want: []string{landedLine}},
		{name: "ARunNeverReadAheadOfLandWaitsPastLandForARead", requested: true, failLookupThroughTick: 3, ticks: [][]RunView{{running}, {running}, {landed}, {landed}, {landed}}, want: []string{landedLine}},
		{name: "AnOlderMemoryHoldingARequestIsNotReadAgainPastLand", seed: &LandNoticeState{Requested: true}, ticks: [][]RunView{{landed}, {landed}}, want: []string{landedLine}},
		{name: "AClearOnlyMemoryOfAnOlderBinaryGetsNoSecondNotice", requested: true, seed: &LandNoticeState{Requested: true, ClearOwed: true}, ticks: [][]RunView{{landed}, {landed}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := newPassHarness(t)
			var tickNumber int
			var requested bool
			h.d.deps.LandNotice = func(slug string) (bool, error) {
				if tickNumber == tt.failLookupOnTick || tickNumber <= tt.failLookupThroughTick {
					return false, errors.New("a beam.yaml of the chain does not read")
				}
				return requested, nil
			}
			var posts int
			var sent []string
			h.d.deps.Notice = func(_ context.Context, line string) error {
				posts++
				if posts <= tt.failPosts {
					return errors.New("no orch strand is recorded")
				}
				sent = append(sent, line)
				return nil
			}
			if tt.beam {
				h.d.geom.Slug = "b"
				h.d.deps.PullRequestRecorded = func() (bool, error) { return tt.pullRequest, nil }
			}
			if tt.seed != nil {
				err := UpdateState(h.d.geom.StatePath, func(s *State) error {
					s.LandNotices = map[string]LandNoticeState{"task": *tt.seed}
					return nil
				})
				if err != nil {
					t.Fatalf("UpdateState(seed) = %v; want nil", err)
				}
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
				if tt.beam && slices.ContainsFunc(runs, func(run RunView) bool { return !run.PastLand }) {
					status := "running · Run-Shed"
					h.fake.tasks = []TaskView{{Slug: "task", Kind: "task", Status: &status, Project: "b"}}
				}
				requested = tt.requested
				if i < len(tt.requestedByTick) {
					requested = tt.requestedByTick[i]
				}
				h.tick(false)
			}

			if !slices.Equal(sent, tt.want) {
				t.Errorf("notices = %q; want %q", sent, tt.want)
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
