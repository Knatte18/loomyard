// landnotice_test.go pins the landing notice over a pass harness: which runs get a line under each land_notice value, the line's text, the request's clearing, the retry of a failed post and the memory's upkeep.

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

	tests := []struct {
		name string
		mode string
		// requested is the board entry's notify_on_land while the run is ahead of Land; the entry is pruned once the run is past it.
		requested bool
		// requestedByTick, when set, replaces requested with the entry's notify_on_land at each tick.
		requestedByTick []bool
		// failBoardOnTick is the one-based tick whose board read fails; zero fails none.
		failBoardOnTick int
		// ticks are the run listings of successive ticks.
		ticks [][]RunView
		// failPosts is how many posts fail before the Notice seam succeeds.
		failPosts  int
		want       []string
		wantClears []string
		// wantPosts counts the calls of the Notice seam, failed ones included, when it differs from len(want).
		wantPosts int
	}{
		{name: "AllLandedNamesTheCommitOnce", mode: LandNoticeAll, ticks: [][]RunView{{running}, {landed}, {landed}}, want: []string{landedLine}},
		{name: "AllTeardownRowCountsAsPastLand", mode: LandNoticeAll, ticks: [][]RunView{{running}, {tearingDown}, {landed}}, want: []string{landedLine}},
		{name: "AllUnlandedNamesTheOutcome", mode: LandNoticeAll, ticks: [][]RunView{{running}, {unlanded}}, want: []string{"[dobby notice] task: ended without landing; lym batten status task shows the run"}},
		{name: "AllLandedWithNoCommitRecorded", mode: LandNoticeAll, ticks: [][]RunView{{running}, {landedNoCommit}}, want: []string{"[dobby notice] task: landed, commit unknown"}},
		{name: "AllFirstSeenPastLandSendsNothing", mode: LandNoticeAll, ticks: [][]RunView{{landed}, {landed}}},
		{name: "PerTaskRequestedSendsAndClearsAfterThePrune", mode: LandNoticePerTask, requested: true, ticks: [][]RunView{{running}, {landed}}, want: []string{landedLine}, wantClears: []string{"task"}},
		{name: "PerTaskUnrequestedSendsNothing", mode: LandNoticePerTask, ticks: [][]RunView{{running}, {landed}}},
		{name: "OffSendsNothingAndKeepsTheRequest", mode: LandNoticeOff, requested: true, ticks: [][]RunView{{running}, {landed}}},
		{name: "AllRequestedClearsTheRequest", mode: LandNoticeAll, requested: true, ticks: [][]RunView{{running}, {landed}}, want: []string{landedLine}, wantClears: []string{"task"}},
		{name: "AFailedPostIsRetriedOnTheNextTick", mode: LandNoticeAll, ticks: [][]RunView{{running}, {landed}, {landed}}, failPosts: 1, want: []string{landedLine}, wantPosts: 2},
		{name: "APostIsDroppedAfterItsLastAttempt", mode: LandNoticeAll, ticks: [][]RunView{{running}, {landed}, {landed}, {landed}, {landed}}, failPosts: 9, wantPosts: landNoticeMaxAttempts},
		{name: "PerTaskFollowsARequestWithdrawnBetweenTicks", mode: LandNoticePerTask, requestedByTick: []bool{true, false}, ticks: [][]RunView{{running}, {running}, {landed}}},
		{name: "PerTaskFollowsARequestRaisedBetweenTicks", mode: LandNoticePerTask, requestedByTick: []bool{false, true}, ticks: [][]RunView{{running}, {running}, {landed}}, want: []string{landedLine}, wantClears: []string{"task"}},
		{name: "AFailedBoardReadKeepsTheFirstReading", mode: LandNoticePerTask, requestedByTick: []bool{true, false}, failBoardOnTick: 2, ticks: [][]RunView{{running}, {running}, {landed}}, want: []string{landedLine}, wantClears: []string{"task"}},
		{name: "ARunNoLongerListedIsForgotten", mode: LandNoticeAll, ticks: [][]RunView{{running}, {}, {landed}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := newPassHarness(t)
			h.d.deps.LoadConfig = func() (Config, error) { return Config{MaxRuns: 3, LandNotice: tt.mode}, nil }
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
			h.d.deps.ClearNotifyOnLand = func(slug string) error {
				clears = append(clears, slug)
				return nil
			}
			status := "running · Run-Shed"
			var tickNumber int
			h.d.deps.Tasks = func() ([]TaskView, error) {
				if tickNumber == tt.failBoardOnTick {
					return nil, errors.New("the board is unreadable")
				}
				return append([]TaskView(nil), h.fake.tasks...), nil
			}
			for i, runs := range tt.ticks {
				tickNumber = i + 1
				h.fake.runs = runs
				h.fake.tasks = nil
				requested := tt.requested
				if i < len(tt.requestedByTick) {
					requested = tt.requestedByTick[i]
				}
				if slices.ContainsFunc(runs, func(run RunView) bool { return !run.PastLand }) {
					h.fake.tasks = []TaskView{{Slug: "task", Kind: "task", Status: &status, NotifyOnLand: requested}}
				}
				if tickNumber == tt.failBoardOnTick {
					// The tick's own start step reads the board after the notice step, so a failed read fails the tick; the notice step has run by then.
					h.clock.sleep(context.Background(), 10*time.Second)
					if err := h.d.tick(context.Background(), h.clock.now()); err == nil {
						t.Fatalf("tick() = nil on a failed board read; want the start step's error")
					}
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
