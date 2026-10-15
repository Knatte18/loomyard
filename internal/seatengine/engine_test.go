// engine_test.go covers Engine.Run: the start order, how the chair's result and failures end the step, advisor failures, the notices sent to the chair and the stop rule.

package seatengine

import (
	"errors"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/shuttlefake"
)

func TestEngine_Run_ChairEndStopsEverySeat(t *testing.T) {
	t.Parallel()
	waitErr := errors.New("wait failed")
	tests := []struct {
		name    string
		result  shuttleengine.Result
		waitErr error
	}{
		{name: "done", result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}},
		{name: "failing gate", result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone, Gate: &shuttleengine.GateOutcome{Passed: false}}},
		{name: "died", result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDied}},
		{name: "timeout", result: shuttleengine.Result{Outcome: shuttleengine.OutcomeTimeout}},
		{name: "a wait error", waitErr: waitErr},
	}
	for _, advisors := range []int{2, 0} {
		for _, tt := range tests {
			t.Run(tt.name+"/"+string(rune('0'+advisors))+" advisors", func(t *testing.T) {
				t.Parallel()
				shuttle := newFakeShuttle(t)
				engine, geom := engineFixture(t, shuttle)
				table := engineTable(geom.WorktreeRoot, advisors)
				advisorGate := shuttleengine.GateSpec{{Name: "advisor-gate", Attempts: 2}}
				if advisors > 1 {
					table.Seats[2].Gate = advisorGate
				}

				ended := runAsync(engine, table)
				wantStarts := []string{role(""), role(advisorName(1)), role(advisorName(2))}[:advisors+1]
				if got := shuttle.awaitStarts(advisors + 1); !slices.Equal(got, wantStarts) {
					t.Fatalf("start order = %v, want %v", got, wantStarts)
				}
				chair := shuttle.handle(role(""))
				for n := 1; n <= advisors; n++ {
					if line, want := receive(t, chair), advisorUpNotice("ly:task:multi-advisor-"+string(rune('0'+n))); line != want {
						t.Errorf("notice %d = %q, want %q", n, line, want)
					}
				}
				chair.release(tt.result, tt.waitErr)
				end := await(t, ended)

				if tt.waitErr == nil && end.err != nil {
					t.Fatalf("Run() error = %v, want nil", end.err)
				}
				if tt.waitErr != nil && !errors.Is(end.err, tt.waitErr) {
					t.Fatalf("Run() error = %v, want it to wrap the wait error", end.err)
				}
				if end.result.Chair.Outcome != tt.result.Outcome || end.result.Chair.Gate != tt.result.Gate {
					t.Errorf("chair result = %+v, want the outcome and gate shuttle reported", end.result.Chair)
				}
				if want := table.Chair().Outputs; !slices.Equal(end.result.ChairOutputs, want) {
					t.Errorf("ChairOutputs = %v, want %v", end.result.ChairOutputs, want)
				}
				if len(end.result.Advisors) != advisors {
					t.Fatalf("advisor results = %d, want %d", len(end.result.Advisors), advisors)
				}
				for n := 1; n <= advisors; n++ {
					if stops := shuttle.handle(role(advisorName(n))).stopCount(); stops != 1 {
						t.Errorf("advisor %d stopped %d times, want once", n, stops)
					}
					if end.result.Advisors[n-1].Notified {
						t.Errorf("advisor %d: notified the chair, want nothing sent for a stop", n)
					}
				}
				chairStopped := tt.waitErr != nil || tt.result.Outcome == shuttleengine.OutcomeTimeout
				if chairStops, want := shuttle.handle(role("")).stopCount(), map[bool]int{true: 1, false: 0}[chairStopped]; chairStops != want {
					t.Errorf("chair stopped %d times, want %d", chairStops, want)
				}
				if attempts := chair.attemptCount(); attempts != advisors {
					t.Errorf("chair took %d send attempts, want one up line per advisor (%d)", attempts, advisors)
				}

				if got := len(shuttle.gates[role("")]); got != 1 {
					t.Errorf("chair gate has %d entries, want the table's one", got)
				}
				if advisors > 0 && len(shuttle.gates[role(advisorName(1))]) != 0 {
					t.Error("an ungated advisor started gated, want ungated")
				}
				if advisors > 1 && !reflect.DeepEqual(shuttle.gates[role(advisorName(2))], advisorGate) {
					t.Errorf("gated advisor started with %+v, want exactly its seat's gate %+v", shuttle.gates[role(advisorName(2))], advisorGate)
				}
				prompt := shuttle.spec(role("")).Prompt
				if advisors == 0 && !strings.Contains(prompt, "Your advisors, by strand name: (none)") {
					t.Errorf("chair prompt = %q, want no advisors named", prompt)
				}
			})
		}
	}
}

func TestEngine_Run_StartFailureEndingTheRunStopsEveryStartedSeat(t *testing.T) {
	t.Parallel()
	chairRole := role("")
	lastAdvisorRole := role(advisorName(2))
	tests := []struct {
		name            string
		shuttle         func(*fakeShuttle)
		wantIs          error
		wantText        string
		chairStop       int
		advisorsStarted bool
	}{
		{name: "the provider never came up", shuttle: func(f *fakeShuttle) { f.startErrs = map[string]error{chairRole: shuttleengine.ErrNotStarted} }, wantIs: shuttleengine.ErrNotStarted},
		{name: "another start error", shuttle: func(f *fakeShuttle) { f.startErrs = map[string]error{chairRole: errors.New("no pane")} }, wantText: "no pane"},
		{name: "a held strand name", shuttle: func(f *fakeShuttle) { f.strandNames = map[string]string{chairRole: "ly:task:multi-2"} }, wantText: `lyx reed remove --name ly:task:multi"`, chairStop: 1},
		{name: "a held strand name and a failed stop", shuttle: func(f *fakeShuttle) {
			f.strandNames = map[string]string{chairRole: "ly:task:multi-2"}
			f.stopErrs = map[string]error{chairRole: errors.New("pane gone")}
		}, wantIs: ErrSeatNotStopped, wantText: `lyx reed remove guid-multi"`, chairStop: 1},
		{name: "an advisor's held strand name and a failed stop", shuttle: func(f *fakeShuttle) {
			f.strandNames = map[string]string{lastAdvisorRole: "ly:task:multi-advisor-2-2"}
			f.stopErrs = map[string]error{lastAdvisorRole: errors.New("pane gone")}
		}, wantIs: ErrSeatNotStopped, wantText: `lyx reed remove guid-multi-advisor-2"`, chairStop: 1, advisorsStarted: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			shuttle := newFakeShuttle(t)
			tt.shuttle(shuttle)
			engine, geom := engineFixture(t, shuttle)

			ended := runAsync(engine, engineTable(geom.WorktreeRoot, 2))
			end := await(t, ended)

			if end.err == nil {
				t.Fatal("Run() error = nil, want the start failure")
			}
			if tt.wantIs != nil && !errors.Is(end.err, tt.wantIs) {
				t.Errorf("Run() error = %v, want it to wrap %v", end.err, tt.wantIs)
			}
			if tt.wantText != "" && !strings.Contains(end.err.Error(), tt.wantText) {
				t.Errorf("Run() error = %v, want it to contain %q", end.err, tt.wantText)
			}
			for n := 1; n <= 2; n++ {
				advisor, started := shuttle.handles[role(advisorName(n))]
				if started != tt.advisorsStarted {
					t.Fatalf("advisor %d started = %v, want %v", n, started, tt.advisorsStarted)
				}
				if started && advisor.stopCount() != 1 {
					t.Errorf("advisor %d stopped %d times, want once", n, advisor.stopCount())
				}
			}
			if _, started := shuttle.handles[chairRole]; started != (tt.chairStop > 0) {
				t.Errorf("chair handle started = %v, want %v", started, tt.chairStop > 0)
			}
			if tt.chairStop > 0 {
				if stops := shuttle.handle(chairRole).stopCount(); stops != tt.chairStop {
					t.Errorf("chair stopped %d times, want %d", stops, tt.chairStop)
				}
			}
		})
	}

	forkWake := []shuttleengine.Requirement{shuttleengine.RequirementBackgroundWake}
	unmet := []struct {
		name     string
		advisors int
		forks    bool
		requires []shuttleengine.Requirement
		key      shuttleengine.ModelKey
		wantText string
	}{
		{name: "the discussion chair", forks: true, requires: forkWake, key: shuttleengine.ModelKey{Module: "loom", Key: "discussion"}, wantText: "set discussion in loom.yaml to an alias whose engine meets them"},
		{name: "the plan chair", forks: true, requires: forkWake, key: shuttleengine.ModelKey{Module: "loom", Key: "plan"}, wantText: "set plan in loom.yaml to an alias whose engine meets them"},
		{name: "the cluster-fan reviewer", forks: true, key: shuttleengine.ModelKey{Module: "loom", Key: "fan_review"}, wantText: "set fan_review in loom.yaml to an alias whose engine meets them"},
		{name: "a recipe seat of a two-seat table", advisors: 1, wantText: "set the seat's model to an alias whose engine meets them"},
	}
	for _, tt := range unmet {
		t.Run("an unmet requirement refuses "+tt.name, func(t *testing.T) {
			t.Parallel()
			engine, geom := engineFixture(t, newFakeShuttle(t))
			runner, reed := realRunnerShuttle(t, geom.WorktreeRoot)
			engine.shuttle = runner
			table := engineTable(geom.WorktreeRoot, tt.advisors)
			table.Seats[0].ForkSubagents = tt.forks
			table.Seats[0].Requires = tt.requires
			table.Seats[0].ModelKey = tt.key

			end := await(t, runAsync(engine, table))

			if !errors.Is(end.err, shuttleengine.ErrRequirementsUnmet) {
				t.Fatalf("Run() error = %v, want it to wrap ErrRequirementsUnmet", end.err)
			}
			for _, want := range []string{`engine "fake"`, tt.wantText} {
				if !strings.Contains(end.err.Error(), want) {
					t.Errorf("Run() error = %v, want it to contain %q", end.err, want)
				}
			}
			if len(reed.AddedSpecs) != 0 {
				t.Errorf("reed AddStrand calls = %d, want none before any pane", len(reed.AddedSpecs))
			}
		})
	}
}

func TestEngine_Run_AdvisorStartFailureIsRecordedAndNamedToTheChair(t *testing.T) {
	t.Parallel()
	advisorRole := role(advisorName(1))
	tests := []struct {
		name        string
		shuttle     func(*fakeShuttle)
		unmetKey    bool
		wantError   string
		wantStopped int
	}{
		{name: "a start error", shuttle: func(f *fakeShuttle) { f.startErrs = map[string]error{advisorRole: errors.New("no pane")} }, wantError: "no pane"},
		{name: "a held strand name", shuttle: func(f *fakeShuttle) { f.strandNames = map[string]string{advisorRole: "ly:task:multi-advisor-1-2"} }, wantError: `lyx reed remove --name ly:task:multi-advisor-1"`, wantStopped: 1},
		{name: "an unmet requirement names the advisor's model key", shuttle: func(*fakeShuttle) {}, unmetKey: true, wantError: `engine "fake"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			shuttle := newFakeShuttle(t)
			tt.shuttle(shuttle)
			engine, geom := engineFixture(t, shuttle)
			table := engineTable(geom.WorktreeRoot, 2)
			var reed *shuttlefake.Reed
			if tt.unmetKey {
				// The chair of a two-seat table requires peer messaging itself, so no chair could start on the fake engine's set: only this advisor runs on it.
				var runner Shuttle
				runner, reed = realRunnerShuttle(t, geom.WorktreeRoot)
				shuttle.startedBy = map[string]Shuttle{advisorRole: runner}
				table.Seats[1].ModelKey = shuttleengine.ModelKey{Module: "loom", Key: "discussion_advisors", Entry: advisorName(1)}
			}

			ended := runAsync(engine, table)
			shuttle.awaitStarts(3)
			chair := shuttle.handle(role(""))
			lines := []string{receive(t, chair), receive(t, chair)}
			want := []string{advisorNotStartedNotice("ly:task:multi-advisor-1"), advisorUpNotice("ly:task:multi-advisor-2")}
			if !slices.Equal(lines, want) {
				t.Errorf("notices = %q, want %q", lines, want)
			}
			chair.release(shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}, nil)
			end := await(t, ended)

			if end.err != nil {
				t.Fatalf("Run() error = %v, want nil", end.err)
			}
			if got := end.result.Advisors[0]; !strings.Contains(got.StartError, tt.wantError) || got.StrandGUID != "" || !got.Notified {
				t.Errorf("failed advisor result = %+v, want the start error %q, no strand and Notified", got, tt.wantError)
			}
			if tt.unmetKey {
				if got, want := end.result.Advisors[0].StartError, "the model of "+advisorName(1)+" in discussion_advisors"; !strings.Contains(got, want) {
					t.Errorf("failed advisor start error = %q, want it to contain %q", got, want)
				}
				if len(reed.AddedSpecs) != 0 {
					t.Errorf("reed AddStrand calls = %d, want none before any pane", len(reed.AddedSpecs))
				}
			}
			if got := end.result.Advisors[1]; got.StartError != "" || got.StrandGUID == "" || got.Notified {
				t.Errorf("second advisor result = %+v, want it started and no ending notice", got)
			}
			if stops := shuttle.handle(role(advisorName(2))).stopCount(); stops != 1 {
				t.Errorf("second advisor stopped %d times, want once", stops)
			}
			if stopped := shuttle.handles[advisorRole]; tt.wantStopped > 0 && stopped.stopCount() != tt.wantStopped {
				t.Errorf("misnamed advisor stopped %d times, want %d", stopped.stopCount(), tt.wantStopped)
			}
		})
	}
}

func TestEngine_Run_AdvisorDeathSendsOneNoticeOnceTheChairTakesIt(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		chairBusy    []error
		alwaysBusy   bool
		wantAttempts int
	}{
		{name: "an idle chair", wantAttempts: 2},
		{name: "a busy chair delays the lines", chairBusy: []error{shuttleengine.ErrSessionBusy, shuttleengine.ErrSubmissionNotLanded}, wantAttempts: 4},
		{name: "a chair that stays busy does not block the join", alwaysBusy: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			shuttle := newFakeShuttle(t)
			chairRole := role("")
			shuttle.sendErrs = map[string][]error{chairRole: tt.chairBusy}
			shuttle.alwaysBusy = map[string]bool{chairRole: tt.alwaysBusy}
			engine, geom := engineFixture(t, shuttle)

			ended := runAsync(engine, engineTable(geom.WorktreeRoot, 1))
			shuttle.awaitStarts(2)
			chair := shuttle.handle(chairRole)
			shuttle.handle(role(advisorName(1))).release(shuttleengine.Result{Outcome: shuttleengine.OutcomeDied}, nil)

			if tt.alwaysBusy {
				for chair.attemptCount() < 3 {
					time.Sleep(time.Millisecond)
				}
			} else {
				if line, want := receive(t, chair), advisorUpNotice("ly:task:multi-advisor-1"); line != want {
					t.Errorf("first notice = %q, want %q", line, want)
				}
				line := receive(t, chair)
				for _, want := range []string{"Advisor ly:task:multi-advisor-1 ended (died)", "no longer answers", geom.WorktreeRoot} {
					if !strings.Contains(line, want) {
						t.Errorf("notice = %q, want it to contain %q", line, want)
					}
				}
			}
			chair.release(shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}, nil)
			end := await(t, ended)

			if end.err != nil {
				t.Fatalf("Run() error = %v, want nil", end.err)
			}
			if got := end.result.Advisors[0]; got.Outcome != shuttleengine.OutcomeDied || got.Notified == tt.alwaysBusy {
				t.Errorf("advisor result = %+v, want died and Notified = %v", got, !tt.alwaysBusy)
			}
			if tt.wantAttempts > 0 && chair.attemptCount() != tt.wantAttempts {
				t.Errorf("send attempts = %d, want %d", chair.attemptCount(), tt.wantAttempts)
			}
			if len(chair.sent) != 0 {
				t.Errorf("a second line was typed into the chair: %q", <-chair.sent)
			}
		})
	}
}

func TestEngine_Run_AdvisorsEndingTogetherOrAsTheyStart(t *testing.T) {
	t.Parallel()
	chairRole := role("")
	died := shuttleengine.Result{Outcome: shuttleengine.OutcomeDied}

	t.Run("two advisors ending together are typed as two whole lines", func(t *testing.T) {
		t.Parallel()
		shuttle := newFakeShuttle(t)
		engine, geom := engineFixture(t, shuttle)
		ended := runAsync(engine, engineTable(geom.WorktreeRoot, 2))
		shuttle.awaitStarts(3)
		shuttle.handle(role(advisorName(1))).release(died, nil)
		shuttle.handle(role(advisorName(2))).release(died, nil)

		chair := shuttle.handle(chairRole)
		var lines []string
		for range 4 {
			if line := receive(t, chair); !strings.HasSuffix(line, "is up and takes your questions.") {
				lines = append(lines, line)
			}
		}
		slices.Sort(lines)
		if len(lines) != 2 {
			t.Fatalf("ending lines = %q, want two beside the two up lines", lines)
		}
		for n, line := range lines {
			if want := "Advisor ly:task:multi-advisor-" + string(rune('1'+n)) + " ended (died)"; strings.Count(line, "\n") != 0 || !strings.HasPrefix(line, want) {
				t.Errorf("line %d = %q, want one whole line starting %q", n, line, want)
			}
		}
		chair.release(shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}, nil)
		await(t, ended)
	})

	t.Run("an advisor ending as it starts is told after its up line", func(t *testing.T) {
		t.Parallel()
		shuttle := newFakeShuttle(t)
		engine, geom := engineFixture(t, shuttle)
		ended := runAsync(engine, engineTable(geom.WorktreeRoot, 1))
		shuttle.awaitStarts(2)
		shuttle.handle(role(advisorName(1))).release(died, nil)

		chair := shuttle.handle(chairRole)
		if line, want := receive(t, chair), advisorUpNotice("ly:task:multi-advisor-1"); line != want {
			t.Errorf("first notice = %q, want %q", line, want)
		}
		if line := receive(t, chair); !strings.HasPrefix(line, "Advisor ly:task:multi-advisor-1 ended (died)") {
			t.Errorf("second notice = %q, want the advisor's ending", line)
		}
		chair.release(shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}, nil)
		if end := await(t, ended); !end.result.Advisors[0].Notified {
			t.Error("advisor result Notified = false, want true")
		}
	})
}

func TestEngine_Run_DoneAdvisorSendsNothingAndIsKeptUntilTheChairEnds(t *testing.T) {
	t.Parallel()
	shuttle := newFakeShuttle(t)
	engine, geom := engineFixture(t, shuttle)
	ended := runAsync(engine, engineTable(geom.WorktreeRoot, 1))
	shuttle.awaitStarts(2)
	chair := shuttle.handle(role(""))
	receive(t, chair)
	advisor := shuttle.handle(role(advisorName(1)))
	advisor.release(shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}, nil)

	select {
	case end := <-ended:
		t.Fatalf("the run ended with the chair still running: %+v", end)
	case <-time.After(20 * time.Millisecond):
	}
	if advisor.stopCount() != 0 {
		t.Error("a done advisor was stopped before the chair ended, want it kept")
	}
	chair.release(shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}, nil)
	end := await(t, ended)

	if end.err != nil || advisor.stopCount() != 1 {
		t.Errorf("Run() error = %v, advisor stops = %d, want nil and one stop", end.err, advisor.stopCount())
	}
	if got := end.result.Advisors[0]; got.Outcome != shuttleengine.OutcomeDone || got.Notified {
		t.Errorf("advisor result = %+v, want done and no notice", got)
	}
	if attempts := chair.attemptCount(); attempts != 1 {
		t.Errorf("chair took %d send attempts, want the up line alone for a done advisor", attempts)
	}
}

func TestEngine_Run_FailedStopReturnsTheWayForwardUnretried(t *testing.T) {
	t.Parallel()
	shuttle := newFakeShuttle(t)
	shuttle.stopErrs = map[string]error{role(advisorName(1)): errors.New("pane gone")}
	engine, geom := engineFixture(t, shuttle)
	ended := runAsync(engine, engineTable(geom.WorktreeRoot, 1))
	shuttle.awaitStarts(2)
	shuttle.handle(role("")).release(shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}, nil)
	end := await(t, ended)

	if !errors.Is(end.err, ErrSeatNotStopped) || !strings.Contains(end.err.Error(), `lyx reed remove guid-multi-advisor-1"`) {
		t.Errorf("Run() error = %v, want ErrSeatNotStopped naming the strand to remove", end.err)
	}
	if stops := shuttle.handle(role(advisorName(1))).stopCount(); stops != 1 {
		t.Errorf("advisor stop attempts = %d, want one, unretried", stops)
	}
	if end.result.Chair.Outcome != shuttleengine.OutcomeDone {
		t.Errorf("chair outcome = %q, want the result still returned", end.result.Chair.Outcome)
	}
}

func TestEngine_Run_RequiredAdvisorEndsTheStep(t *testing.T) {
	t.Parallel()
	waitErr, acceptErr, startErr := errors.New("wait failed"), errors.New("output rejected"), errors.New("no pane")
	died := shuttleengine.Result{Outcome: shuttleengine.OutcomeDied}
	failedGate := shuttleengine.Result{Outcome: shuttleengine.OutcomeDone, Gate: &shuttleengine.GateOutcome{Passed: false}}
	done := shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}
	tests := []struct {
		name        string
		advisor     shuttleengine.Result
		waitErr     error
		acceptErr   error
		startErr    error
		resume      bool
		stopErr     error
		wantAccepts int
		wantNoStop  bool
		// acceptedThenChairEnds releases the advisor done and accepted, and ends the chair afterwards.
		acceptedThenChairEnds bool
	}{
		{name: "the advisor is done and accepted, then the chair ends", advisor: done, wantAccepts: 1, acceptedThenChairEnds: true},
		{name: "the advisor died", advisor: died},
		{name: "the advisor timed out", advisor: shuttleengine.Result{Outcome: shuttleengine.OutcomeTimeout}},
		{name: "the advisor's wait errored", advisor: shuttleengine.Result{}, waitErr: waitErr},
		{name: "the advisor is done with its gate failed", advisor: failedGate},
		{name: "the accept hook errored", advisor: done, acceptErr: acceptErr, wantAccepts: 1},
		{name: "the advisor failed to start", startErr: startErr, wantNoStop: true},
		{name: "the resume did not find the advisor", resume: true, wantNoStop: true},
		{name: "the advisor cannot be stopped", advisor: died, stopErr: errors.New("pane gone")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			shuttle := newFakeShuttle(t)
			if tt.startErr != nil {
				shuttle.startErrs = map[string]error{role(advisorName(1)): tt.startErr}
			}
			if tt.stopErr != nil {
				shuttle.stopErrs = map[string]error{role(advisorName(1)): tt.stopErr}
			}
			engine, geom := engineFixture(t, shuttle)
			table := engineTable(geom.WorktreeRoot, 1)
			var mu sync.Mutex
			var accepted []shuttleengine.Result
			table.Seats[1].Required = true
			acceptCalled := make(chan struct{}, 2)
			table.Seats[1].Accept = func(result shuttleengine.Result) error {
				mu.Lock()
				defer mu.Unlock()
				accepted = append(accepted, result)
				acceptCalled <- struct{}{}
				return tt.acceptErr
			}

			var chair *fakeHandle
			var ended <-chan engineEnd
			if tt.resume {
				chair = shuttle.newHandle(role(""))
				channel := make(chan engineEnd, 1)
				go func() {
					result, err := engine.Resume(table, LiveTable{Chair: chair})
					channel <- engineEnd{result, err}
				}()
				ended = channel
			} else {
				ended = runAsync(engine, table)
				shuttle.awaitStarts(2)
				chair = shuttle.handle(role(""))
				if tt.startErr == nil {
					shuttle.handle(role(advisorName(1))).release(tt.advisor, tt.waitErr)
				}
			}
			if tt.acceptedThenChairEnds {
				select {
				case <-acceptCalled:
				case <-time.After(fakeWait):
					t.Fatal("Accept was not called")
				}
				advisor := shuttle.handle(role(advisorName(1)))
				if stops := advisor.stopCount(); stops != 0 {
					t.Errorf("advisor stopped %d times before the chair ended, want none", stops)
				}
				chair.release(done, nil)
				end := await(t, ended)
				mu.Lock()
				defer mu.Unlock()
				if end.err != nil || end.result.Advisors[0].EndedStep || advisor.stopCount() != 1 || len(accepted) != 1 || !reflect.DeepEqual(accepted[0], done) {
					t.Errorf("Run() error = %v, EndedStep = %v, advisor stops = %d, Accept calls = %v; want nil, false, 1 and one call with the done result", end.err, end.result.Advisors[0].EndedStep, advisor.stopCount(), accepted)
				}
				return
			}
			end := await(t, ended)

			if tt.stopErr != nil {
				if !errors.Is(end.err, ErrSeatNotStopped) || !strings.Contains(end.err.Error(), "way forward") {
					t.Fatalf("error = %v, want ErrSeatNotStopped with its way forward", end.err)
				}
			} else if end.err != nil {
				t.Fatalf("error = %v, want nil", end.err)
			}
			if chair.stopCount() != 1 {
				t.Errorf("chair stopped %d times, want once", chair.stopCount())
			}
			if !tt.wantNoStop && shuttle.handle(role(advisorName(1))).stopCount() != 1 {
				t.Errorf("advisor stopped %d times, want once", shuttle.handle(role(advisorName(1))).stopCount())
			}
			got := end.result.Advisors[0]
			if !got.EndedStep || got.Notified || (got.StartError != "") != (tt.startErr != nil) {
				t.Errorf("advisor result = %+v, want EndedStep, no notice, and a start error only for a failed start", got)
			}
			if got.Result.Outcome != tt.advisor.Outcome || got.Result.Gate != tt.advisor.Gate || !errors.Is(got.WaitErr, tt.waitErr) || !errors.Is(got.AcceptErr, tt.acceptErr) {
				t.Errorf("advisor result = %+v, want the result %+v, wait error %v and accept error %v", got, tt.advisor, tt.waitErr, tt.acceptErr)
			}
			mu.Lock()
			defer mu.Unlock()
			if len(accepted) != tt.wantAccepts {
				t.Errorf("accept calls = %d, want %d", len(accepted), tt.wantAccepts)
			}
			for len(chair.sent) > 0 {
				if line := <-chair.sent; line != advisorUpNotice("ly:task:"+role(advisorName(1))) {
					t.Errorf("a line beyond the up line reached the chair: %q", line)
				}
			}
		})
	}
}

func TestEngine_Run_ChairHoldDeferral(t *testing.T) {
	t.Parallel()
	midTurn := shuttleengine.RunSessionState{State: shuttleengine.SessionState{Name: shuttleengine.SessionBusy, Cause: shuttleengine.SessionCauseTurn}}
	tests := []struct {
		name string
		// upLineStuck keeps the chair's session busy, so no up line lands.
		upLineStuck bool
		// act scripts the advisor once it is counted, after its up line landed unless upLineStuck.
		act        func(advisor *fakeHandle)
		wantDefers bool
		resume     bool
	}{
		{name: "an advisor mid-turn defers", act: func(a *fakeHandle) { a.setSession(midTurn, nil) }, wantDefers: true},
		{name: "an advisor whose up line has not landed defers", upLineStuck: true, act: func(a *fakeHandle) { a.setSession(shuttleengine.RunSessionState{}, nil) }, wantDefers: true},
		{name: "an asking advisor whose up line has not landed does not defer", upLineStuck: true, act: heldAdvisor(shuttleengine.SessionAsking)},
		{name: "an idle-stalled advisor whose up line has not landed does not defer", upLineStuck: true, act: heldAdvisor(shuttleengine.SessionIdleStalled)},
		{name: "an idle-done advisor whose up line has not landed does not defer", upLineStuck: true, act: heldAdvisor(shuttleengine.SessionIdleDone)},
		{name: "a dead advisor whose up line has not landed does not defer", upLineStuck: true, act: heldAdvisor(shuttleengine.SessionDead)},
		{name: "a dead advisor whose up line landed does not defer", act: heldAdvisor(shuttleengine.SessionDead)},
		{name: "an unreadable session with the up line not landed still defers", upLineStuck: true, act: func(a *fakeHandle) { a.setSession(midTurn, errors.New("unreadable")) }, wantDefers: true},
		{name: "idle advisors with landed up lines do not defer", act: func(a *fakeHandle) {}},
		{name: "an advisor that ended done never defers", act: func(a *fakeHandle) {
			a.setSession(midTurn, nil)
			a.release(shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}, nil)
		}},
		{name: "an advisor that timed out never defers", act: func(a *fakeHandle) {
			a.setSession(midTurn, nil)
			a.release(shuttleengine.Result{Outcome: shuttleengine.OutcomeTimeout}, nil)
		}},
		{name: "a session-state error does not defer", act: func(a *fakeHandle) { a.setSession(midTurn, errors.New("unreadable")) }},
		{name: "an attached chair defers on the advisors the probe found", resume: true, act: func(a *fakeHandle) { a.setSession(midTurn, nil) }, wantDefers: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			shuttle := newFakeShuttle(t)
			if tt.upLineStuck {
				shuttle.alwaysBusy = map[string]bool{role(""): true}
			}
			engine, geom := engineFixture(t, shuttle)
			table := engineTable(geom.WorktreeRoot, 1)

			var chair, advisor *fakeHandle
			var predicate func() bool
			var ended <-chan engineEnd
			if tt.resume {
				chair, advisor = shuttle.newHandle(role("")), shuttle.newHandle(role(advisorName(1)))
				shuttle.live = map[string]*fakeHandle{role(""): chair, role(advisorName(1)): advisor}
				found, err := engine.Probe(table)
				if err != nil {
					t.Fatalf("Probe() = %v", err)
				}
				if probed := shuttle.probeSpec(role(advisorName(1))); probed.HoldDeferral != nil {
					t.Error("the probed advisor spec carries a hold deferral, want none")
				}
				predicate = shuttle.probeSpec(role("")).HoldDeferral
				channel := make(chan engineEnd, 1)
				go func() {
					result, err := engine.Resume(table, found)
					channel <- engineEnd{result, err}
				}()
				ended = channel
			} else {
				ended = runAsync(engine, table)
				shuttle.awaitStarts(2)
				chair, advisor = shuttle.handle(role("")), shuttle.handle(role(advisorName(1)))
				if started := shuttle.spec(role(advisorName(1))); started.HoldDeferral != nil {
					t.Error("the advisor spec carries a hold deferral, want none")
				}
				predicate = shuttle.spec(role("")).HoldDeferral
			}
			if predicate == nil {
				t.Fatal("the chair spec carries no hold deferral")
			}
			// With no advisor counted the predicate is false, so it holding with the advisor mid-turn proves the advisor is counted.
			advisor.setSession(midTurn, nil)
			awaitCondition(t, "the advisor to be counted", predicate)
			if !tt.upLineStuck {
				receive(t, chair)
				advisor.setSession(shuttleengine.RunSessionState{}, nil)
				awaitCondition(t, "the up line to land", func() bool { return !predicate() })
			}
			if tt.act != nil {
				tt.act(advisor)
			}
			if !tt.wantDefers {
				awaitCondition(t, "the deferral to end", func() bool { return !predicate() })
			}
			if got := predicate(); got != tt.wantDefers {
				t.Errorf("hold deferral = %v, want %v", got, tt.wantDefers)
			}
			chair.release(shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}, nil)
			await(t, ended)
		})
	}

	t.Run("a one-seat table's lead never defers and is sent nothing", func(t *testing.T) {
		t.Parallel()
		shuttle := newFakeShuttle(t)
		engine, geom := engineFixture(t, shuttle)
		table := engineTable(geom.WorktreeRoot, 0)
		table.Amendment = "\nFresh start.\n"
		ended := runAsync(engine, table)
		shuttle.awaitStarts(1)
		lead := shuttle.handle(role(""))
		if prompt := shuttle.spec(role("")).Prompt; !strings.HasSuffix(prompt, table.Amendment) {
			t.Errorf("lead prompt = %q, want it to end with the table's Amendment", prompt)
		}
		predicate := shuttle.spec(role("")).HoldDeferral
		if predicate == nil {
			t.Fatal("the lead spec carries no hold deferral")
		}
		if predicate() {
			t.Error("hold deferral = true before the lead reads busy, want false")
		}
		lead.setSession(midTurn, nil)
		if predicate() {
			t.Error("hold deferral = true with the lead mid-turn, want false")
		}
		lead.release(shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}, nil)
		await(t, ended)
		if attempts := lead.attemptCount(); attempts != 0 {
			t.Errorf("lead took %d send attempts, want none", attempts)
		}
	})
}

// heldAdvisor scripts an advisor's session to read as name.
func heldAdvisor(name shuttleengine.SessionStateName) func(*fakeHandle) {
	return func(a *fakeHandle) {
		a.setSession(shuttleengine.RunSessionState{State: shuttleengine.SessionState{Name: name}}, nil)
	}
}

// awaitCondition polls cond until it holds, failing the test when it does not within fakeWait.
func awaitCondition(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(fakeWait)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(time.Millisecond)
	}
}
