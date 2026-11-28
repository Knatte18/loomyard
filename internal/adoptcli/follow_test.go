// follow_test.go drives the follow of an adopt record over scripted records, a fake clock and a fake sleep.

package adoptcli

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/adoptrecord"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// followRead is one scripted read of the adopt record.
type followRead struct {
	record adoptrecord.Record
	found  bool
}

func TestFollow(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	call := adoptrecord.Request{Slugs: []string{"alpha"}}
	since := time.Unix(1000, 0)
	const worker = 100

	record := func(pid int, state string, started time.Time, steps ...string) followRead {
		built := adoptrecord.Record{Request: call, Subjects: []string{"alpha"}, PID: pid, Start: started, State: state, InFlight: "owner"}
		for _, step := range steps {
			built.Steps = append(built.Steps, adoptrecord.StepRecord{Name: step})
		}
		return followRead{record: built, found: true}
	}
	running := func(pid int, steps ...string) followRead {
		return record(pid, adoptrecord.StateRunning, since.Add(time.Minute), steps...)
	}
	ended := func(pid int, state string, steps ...string) followRead {
		return record(pid, state, since.Add(time.Minute), steps...)
	}
	repeat := func(read followRead, times int) []followRead {
		return slices.Repeat([]followRead{read}, times)
	}
	closedChannel := func() <-chan struct{} {
		closed := make(chan struct{})
		close(closed)
		return closed
	}
	openChannel := func() <-chan struct{} { return make(chan struct{}) }
	seconds := func(counts ...int) []time.Duration {
		var durations []time.Duration
		for _, count := range counts {
			durations = append(durations, time.Duration(count)*time.Second)
		}
		return durations
	}

	tests := []struct {
		name   string
		reads  []followRead
		exited func() <-chan struct{}
		dead   bool
		cap    time.Duration
		// wantErr lists the texts the error holds; none means the follow ends without an error.
		wantErr   []string
		wantPID   int
		wantState string
		wantOut   string
		// wantReads and wantSleeps are asserted when set.
		wantReads  int
		wantSleeps []time.Duration
	}{
		{
			name:   "StepsPrintAsTheyEndAndDoneReturnsTheRecord",
			reads:  []followRead{running(worker, "reconcile"), running(worker, "reconcile", "pairs"), ended(worker, adoptrecord.StateDone, "reconcile", "pairs", "pause")},
			exited: openChannel, wantPID: worker, wantState: adoptrecord.StateDone,
			wantOut: "adopt: reconcile done\nadopt: pairs done\nadopt: pause done\n",
		},
		{
			name:   "StoppedReturnsTheRecordWithoutAnError",
			reads:  []followRead{ended(worker, adoptrecord.StateStopped, "reconcile")},
			exited: openChannel, wantPID: worker, wantState: adoptrecord.StateStopped, wantOut: "adopt: reconcile done\n",
		},
		{
			name:   "InterruptedNamesTheRerunAndDiscard",
			reads:  []followRead{running(worker)},
			exited: openChannel, dead: true,
			wantErr: []string{"lym adopt alpha", "lym adopt --discard"},
		},
		{
			name:   "WorkerThatLostTheLockToALaterWorkerOfTheSameSubjectsFollowsIt",
			reads:  []followRead{running(200), ended(200, adoptrecord.StateDone, "reconcile")},
			exited: closedChannel, wantPID: 200, wantState: adoptrecord.StateDone, wantOut: "adopt: reconcile done\n",
		},
		{
			name: "WorkerThatLostTheLockToALaterWorkerOfOtherSubjectsIsRefusedNamingTheRunningAdopt",
			reads: []followRead{{found: true, record: adoptrecord.Record{
				Request: adoptrecord.Request{Slugs: []string{"beta"}}, Subjects: []string{"beta"}, PID: 200, Start: since.Add(time.Minute), State: adoptrecord.StateRunning,
			}}},
			exited:  closedChannel,
			wantErr: []string{"beta", "lym beam status"},
		},
		{
			name:    "RecordOfAnotherPidStartedBeforeTheCallerIsNotFollowed",
			reads:   repeat(record(300, adoptrecord.StateDone, since.Add(-time.Minute)), 5),
			exited:  closedChannel,
			wantErr: []string{"lym adopt alpha", "lym beam status", workerOutName}, wantReads: 2,
		},
		{
			name:    "FollowingWithNoExitedChannelEndsWhenTheRecordTurnsToAnotherPid",
			reads:   []followRead{running(worker), running(200)},
			wantErr: []string{"another started", "lym beam status", "lym adopt alpha"}, wantReads: 2,
		},
		{
			name:    "FollowingWithNoExitedChannelEndsWhenTheRecordIsRemoved",
			reads:   []followRead{running(worker), {}},
			wantErr: []string{"lym beam status", workerOutName}, wantReads: 2,
		},
		{
			name:    "ExitedWorkerWithNoRecordReadsOnceMoreThenNamesTheRerunStatusAndOutputFile",
			reads:   []followRead{{}, {}, {}},
			exited:  closedChannel,
			wantErr: []string{"lym adopt alpha", "lym beam status", workerOutName}, wantReads: 2,
		},
		{
			name:    "AtTheCapTheFollowReturnsStillRunningNamingBeamStatus",
			reads:   []followRead{running(worker)},
			exited:  openChannel,
			cap:     5 * time.Second,
			wantErr: []string{"still running", "lym beam status", "lym adopt alpha"}, wantSleeps: seconds(1, 2, 4),
		},
		{
			name:   "ACapOfZeroWaitsUntilTheWorkerEnds",
			reads:  append(repeat(running(worker), 30), ended(worker, adoptrecord.StateDone)),
			exited: openChannel, wantPID: worker, wantState: adoptrecord.StateDone, wantReads: 31,
		},
		{
			name:   "AnUnchangedRecordDoublesTheWakeToTheCeilingAndAChangeReturnsItToTheFloor",
			reads:  append(append(repeat(running(worker, "reconcile"), 5), running(worker, "reconcile", "pairs")), ended(worker, adoptrecord.StateDone, "reconcile", "pairs")),
			exited: openChannel, wantPID: worker, wantState: adoptrecord.StateDone,
			wantOut: "adopt: reconcile done\nadopt: pairs done\n", wantSleeps: seconds(1, 2, 4, 8, 8, 1),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			now := since.Add(time.Second)
			reads := 0
			var sleeps []time.Duration
			var steps strings.Builder
			deps := followDeps{
				ReadRecord: func() (adoptrecord.Record, bool, error) {
					read := tt.reads[min(reads, len(tt.reads)-1)]
					reads++
					return read.record, read.found, nil
				},
				Alive: func(int, string) bool { return !tt.dead },
				Now:   func() time.Time { return now },
				Sleep: func(wait time.Duration) {
					sleeps = append(sleeps, wait)
					now = now.Add(wait)
				},
				Cap:   tt.cap,
				Steps: &steps,
			}
			if tt.exited != nil {
				deps.Exited = tt.exited()
			}

			got, err := follow(deps, call, worker, since)

			if len(tt.wantErr) > 0 {
				if err == nil {
					t.Fatalf("follow error = nil, want one containing %q", tt.wantErr)
				}
				for _, text := range tt.wantErr {
					if !strings.Contains(err.Error(), text) {
						t.Errorf("follow error = %q, want it to contain %q", err, text)
					}
				}
			} else if err != nil || got.PID != tt.wantPID || got.State != tt.wantState {
				t.Errorf("follow = pid %d, state %q, %v; want pid %d, state %q and no error", got.PID, got.State, err, tt.wantPID, tt.wantState)
			}
			if steps.String() != tt.wantOut {
				t.Errorf("step lines = %q, want %q", steps.String(), tt.wantOut)
			}
			if tt.wantReads != 0 && reads != tt.wantReads {
				t.Errorf("reads = %d, want %d", reads, tt.wantReads)
			}
			if tt.wantSleeps != nil && !slices.Equal(sleeps, tt.wantSleeps) {
				t.Errorf("sleeps = %v, want %v", sleeps, tt.wantSleeps)
			}
		})
	}
}
