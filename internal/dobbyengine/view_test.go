//go:build tier0

// view_test.go pins BuildView: the runs a dobby drives and the reasons each released task waits.

package dobbyengine

import (
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

func TestBuildView(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)
	exited := now.Add(-5 * time.Minute)
	draft := "draft"

	tests := []struct {
		name   string
		mutate func(in *ViewInput)
		want   View
	}{
		{
			name: "AReleasedTaskWithNothingHoldingItIsReady",
			mutate: func(in *ViewInput) {
				in.Tasks = []TaskView{readyTask("a")}
			},
			want: View{Alive: true, MaxRuns: 2, Runs: []ViewRun{}, Waiting: []WaitingTask{{Slug: "a"}}},
		},
		{
			name: "EachEntryReasonIsListed",
			mutate: func(in *ViewInput) {
				deps, owned, paused := readyTask("deps"), readyTask("owned"), readyTask("paused")
				deps.PendingDependencies = []string{"x", "y"}
				owned.OwnedElsewhere = true
				in.Tasks = []TaskView{deps, owned, paused}
				in.Covers = func(slug string) bool { return slug == "paused" }
			},
			want: View{Alive: true, MaxRuns: 2, Runs: []ViewRun{}, Waiting: []WaitingTask{
				{Slug: "deps", Dependencies: []string{"x", "y"}},
				{Slug: "owned", OwnedElsewhere: true},
				{Slug: "paused", Paused: true},
			}},
		},
		{
			name: "ARefusedStartShowsItsTextAndTheNextTryUntilTheBackoffRunsOut",
			mutate: func(in *ViewInput) {
				in.Tasks = []TaskView{readyTask("backing"), readyTask("due"), readyTask("changed")}
				changed := in.Tasks[2]
				changed.Fingerprint = "changed-v2"
				in.Tasks[2] = changed
				in.Record.Starts = map[string]StartState{
					"backing": {Refusal: "pre-run refused", NextTry: now.Add(4 * time.Minute), EntryFingerprint: "backing-v1"},
					"due":     {Refusal: "pre-run refused", NextTry: now.Add(-time.Minute), EntryFingerprint: "due-v1"},
					"changed": {Refusal: "pre-run refused", NextTry: now.Add(4 * time.Minute), EntryFingerprint: "changed-v1"},
				}
			},
			want: View{Alive: true, MaxRuns: 2, Runs: []ViewRun{}, Waiting: []WaitingTask{
				{Slug: "backing", Refusal: "pre-run refused", NextTry: now.Add(4 * time.Minute)},
				{Slug: "due", Refusal: "pre-run refused"},
				{Slug: "changed"},
			}},
		},
		{
			name: "MaxRunsHoldsBackTheTasksPastTheFreeSlotsInBoardOrder",
			mutate: func(in *ViewInput) {
				in.Runs = []RunView{runningRun("r")}
				held := readyTask("held")
				held.PendingDependencies = []string{"x"}
				in.Tasks = []TaskView{held, readyTask("first"), readyTask("second")}
			},
			want: View{Alive: true, MaxRuns: 2, Runs: []ViewRun{{Slug: "r", State: runStateRunning, Producer: producerRunShed}}, Waiting: []WaitingTask{
				{Slug: "held", Dependencies: []string{"x"}},
				{Slug: "first"},
				{Slug: "second", AtMaxRuns: true},
			}},
		},
		{
			name: "MaxRunsZeroHoldsBackEveryTask",
			mutate: func(in *ViewInput) {
				in.Config.MaxRuns = 0
				in.Tasks = []TaskView{readyTask("a")}
			},
			want: View{Alive: true, Runs: []ViewRun{}, Waiting: []WaitingTask{{Slug: "a", AtMaxRuns: true}}},
		},
		{
			name: "RunsShowTheirRecordAndDoneOrOwnedElsewhereRunsTakeNoSlot",
			mutate: func(in *ViewInput) {
				done, flying, away := runningRun("done"), runningRun("flying"), runningRun("away")
				done.State = runStateDone
				flying.DriverLockHeld, flying.Producer = true, "Land"
				in.Runs = []RunView{flying, done, away}
				in.Record.Runs = map[string]RunState{
					"flying": {LastPassExit: exited, FailingPassError: "boom", FailingPassCount: 2},
					"away":   {OwnedElsewhere: true, FullPassOwed: true},
				}
				in.Tasks = []TaskView{readyTask("a")}
			},
			want: View{Alive: true, MaxRuns: 2, Runs: []ViewRun{
				{Slug: "away", State: runStateRunning, Producer: producerRunShed, OwnedElsewhere: true, FullPassOwed: true},
				{Slug: "flying", State: runStateRunning, Producer: "Land", PassInFlight: true, LastPassExit: exited, FailingPassError: "boom", FailingPassCount: 2},
			}, Waiting: []WaitingTask{{Slug: "a"}}},
		},
		{
			name: "ADoneRunHoldingItsDriverLockStillTakesASlot",
			mutate: func(in *ViewInput) {
				finishing := runningRun("finishing")
				finishing.State, finishing.DriverLockHeld = runStateDone, true
				in.Runs = []RunView{runningRun("r"), finishing}
				in.Tasks = []TaskView{readyTask("a")}
			},
			want: View{Alive: true, MaxRuns: 2, Runs: []ViewRun{{Slug: "r", State: runStateRunning, Producer: producerRunShed}}, Waiting: []WaitingTask{{Slug: "a", AtMaxRuns: true}}},
		},
		{
			name: "APawlRunWithoutItsHoldTakesNoSlotAndWithItTakesOne",
			mutate: func(in *ViewInput) {
				waiting, holding := runningRun("waiting"), runningRun("holding")
				waiting.NoSlot = true
				in.Runs = []RunView{waiting, holding}
				in.Tasks = []TaskView{readyTask("first"), readyTask("second")}
			},
			want: View{Alive: true, MaxRuns: 2, Runs: []ViewRun{
				{Slug: "holding", State: runStateRunning, Producer: producerRunShed},
				{Slug: "waiting", State: runStateRunning, Producer: producerRunShed},
			}, Waiting: []WaitingTask{{Slug: "first"}, {Slug: "second", AtMaxRuns: true}}},
		},
		{
			name: "OnlyTasksReleasedToThisWorktreeWait",
			mutate: func(in *ViewInput) {
				other, drafted, project := readyTask("other"), readyTask("drafted"), readyTask("project")
				other.Parent = "beam"
				drafted.Status = &draft
				project.Kind = "project"
				in.Tasks = []TaskView{other, drafted, project}
			},
			want: View{Alive: true, MaxRuns: 2, Runs: []ViewRun{}, Waiting: []WaitingTask{}},
		},
		{
			name:   "ADeadDobbyReadsNotAlive",
			mutate: func(in *ViewInput) { in.Alive = false },
			want:   View{MaxRuns: 2, Runs: []ViewRun{}, Waiting: []WaitingTask{}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			in := ViewInput{
				Worktree: "prime", Alive: true, Config: Config{MaxRuns: 2},
				Covers: func(string) bool { return false }, Now: now,
			}
			tt.mutate(&in)
			if diff := cmp.Diff(tt.want, BuildView(in)); diff != "" {
				t.Errorf("BuildView() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
