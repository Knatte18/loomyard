// engine_test.go covers Engine.Run: the start order, how the chair's result and failures end the step, advisor failures, the notices sent to the chair and the stop rule.

package seatengine

import (
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/shuttleengine"
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
				wantStarts := []string{role(RoleChair), role(AdvisorName(1)), role(AdvisorName(2))}[:advisors+1]
				if got := shuttle.awaitStarts(advisors + 1); !slices.Equal(got, wantStarts) {
					t.Fatalf("start order = %v, want %v", got, wantStarts)
				}
				chair := shuttle.handle(role(RoleChair))
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
					if stops := shuttle.handle(role(AdvisorName(n))).stopCount(); stops != 1 {
						t.Errorf("advisor %d stopped %d times, want once", n, stops)
					}
					if end.result.Advisors[n-1].Notified {
						t.Errorf("advisor %d: notified the chair, want nothing sent for a stop", n)
					}
				}
				if chairStops, want := shuttle.handle(role(RoleChair)).stopCount(), map[bool]int{true: 1, false: 0}[tt.waitErr != nil]; chairStops != want {
					t.Errorf("chair stopped %d times, want %d", chairStops, want)
				}
				if attempts := chair.attemptCount(); attempts != advisors {
					t.Errorf("chair took %d send attempts, want one up line per advisor (%d)", attempts, advisors)
				}

				if got := len(shuttle.gates[role(RoleChair)]); got != 1 {
					t.Errorf("chair gate has %d entries, want the table's one", got)
				}
				if advisors > 0 && len(shuttle.gates[role(AdvisorName(1))]) != 0 {
					t.Error("an ungated advisor started gated, want ungated")
				}
				if advisors > 1 && !reflect.DeepEqual(shuttle.gates[role(AdvisorName(2))], advisorGate) {
					t.Errorf("gated advisor started with %+v, want exactly its seat's gate %+v", shuttle.gates[role(AdvisorName(2))], advisorGate)
				}
				prompt := shuttle.spec(role(RoleChair)).Prompt
				if advisors == 0 && !strings.Contains(prompt, "Your advisors, by strand name: (none)") {
					t.Errorf("chair prompt = %q, want no advisors named", prompt)
				}
			})
		}
	}
}

func TestEngine_Run_StartFailureEndingTheRunStopsEveryStartedSeat(t *testing.T) {
	t.Parallel()
	chairRole := role(RoleChair)
	lastAdvisorRole := role(AdvisorName(2))
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
		{name: "a held strand name", shuttle: func(f *fakeShuttle) { f.strandNames = map[string]string{chairRole: "ly:task:multi-chair-2"} }, wantText: `lyx reed remove --name ly:task:multi-chair"`, chairStop: 1},
		{name: "a held strand name and a failed stop", shuttle: func(f *fakeShuttle) {
			f.strandNames = map[string]string{chairRole: "ly:task:multi-chair-2"}
			f.stopErrs = map[string]error{chairRole: errors.New("pane gone")}
		}, wantIs: ErrSeatNotStopped, wantText: `lyx reed remove guid-multi-chair"`, chairStop: 1},
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
				advisor, started := shuttle.handles[role(AdvisorName(n))]
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
}

func TestEngine_Run_AdvisorStartFailureIsRecordedAndNamedToTheChair(t *testing.T) {
	t.Parallel()
	advisorRole := role(AdvisorName(1))
	tests := []struct {
		name        string
		shuttle     func(*fakeShuttle)
		wantError   string
		wantStopped int
	}{
		{name: "a start error", shuttle: func(f *fakeShuttle) { f.startErrs = map[string]error{advisorRole: errors.New("no pane")} }, wantError: "no pane"},
		{name: "a held strand name", shuttle: func(f *fakeShuttle) { f.strandNames = map[string]string{advisorRole: "ly:task:multi-advisor-1-2"} }, wantError: `lyx reed remove --name ly:task:multi-advisor-1"`, wantStopped: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			shuttle := newFakeShuttle(t)
			tt.shuttle(shuttle)
			engine, geom := engineFixture(t, shuttle)

			ended := runAsync(engine, engineTable(geom.WorktreeRoot, 2))
			shuttle.awaitStarts(3)
			chair := shuttle.handle(role(RoleChair))
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
			if got := end.result.Advisors[1]; got.StartError != "" || got.StrandGUID == "" || got.Notified {
				t.Errorf("second advisor result = %+v, want it started and no ending notice", got)
			}
			if stops := shuttle.handle(role(AdvisorName(2))).stopCount(); stops != 1 {
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
			chairRole := role(RoleChair)
			shuttle.sendErrs = map[string][]error{chairRole: tt.chairBusy}
			shuttle.alwaysBusy = map[string]bool{chairRole: tt.alwaysBusy}
			engine, geom := engineFixture(t, shuttle)

			ended := runAsync(engine, engineTable(geom.WorktreeRoot, 1))
			shuttle.awaitStarts(2)
			chair := shuttle.handle(chairRole)
			shuttle.handle(role(AdvisorName(1))).release(shuttleengine.Result{Outcome: shuttleengine.OutcomeDied}, nil)

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
	chairRole := role(RoleChair)
	died := shuttleengine.Result{Outcome: shuttleengine.OutcomeDied}

	t.Run("two advisors ending together are typed as two whole lines", func(t *testing.T) {
		t.Parallel()
		shuttle := newFakeShuttle(t)
		engine, geom := engineFixture(t, shuttle)
		ended := runAsync(engine, engineTable(geom.WorktreeRoot, 2))
		shuttle.awaitStarts(3)
		shuttle.handle(role(AdvisorName(1))).release(died, nil)
		shuttle.handle(role(AdvisorName(2))).release(died, nil)

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
		shuttle.handle(role(AdvisorName(1))).release(died, nil)

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
	chair := shuttle.handle(role(RoleChair))
	receive(t, chair)
	advisor := shuttle.handle(role(AdvisorName(1)))
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
	shuttle.stopErrs = map[string]error{role(AdvisorName(1)): errors.New("pane gone")}
	engine, geom := engineFixture(t, shuttle)
	ended := runAsync(engine, engineTable(geom.WorktreeRoot, 1))
	shuttle.awaitStarts(2)
	shuttle.handle(role(RoleChair)).release(shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}, nil)
	end := await(t, ended)

	if !errors.Is(end.err, ErrSeatNotStopped) || !strings.Contains(end.err.Error(), `lyx reed remove guid-multi-advisor-1"`) {
		t.Errorf("Run() error = %v, want ErrSeatNotStopped naming the strand to remove", end.err)
	}
	if stops := shuttle.handle(role(AdvisorName(1))).stopCount(); stops != 1 {
		t.Errorf("advisor stop attempts = %d, want one, unretried", stops)
	}
	if end.result.Chair.Outcome != shuttleengine.OutcomeDone {
		t.Errorf("chair outcome = %q, want the result still returned", end.result.Chair.Outcome)
	}
}
