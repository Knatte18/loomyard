// loop_catchup_test.go pins when the dobby runs the catch-up from origin and what it holds while one is owed.
// It also pins how the dobby keeps its runs' bolt-hold entries in step with the hold.

package dobbyengine

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/runfsm"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// fakeCatchUpDeps returns catch-up seams over no runs whose Bolt pull is pull.
// Its other seams succeed and do nothing.
func fakeCatchUpDeps(pull func() error) CatchUpDeps {
	return CatchUpDeps{
		PullBolt:          pull,
		Register:          func() error { return nil },
		Runs:              func() ([]string, error) { return nil, nil },
		FetchRun:          func(string) error { return nil },
		ReadRun:           func(string) (CatchUpRun, error) { return CatchUpRun{}, nil },
		PushOwed:          func(string) error { return nil },
		HandOver:          func(string, string) error { return nil },
		ApplyPauseIntents: func([]string) error { return nil },
	}
}

// tickAfter advances the harness's clock by gap and runs one tick that is no probe tick.
func (h *passHarness) tickAfter(gap time.Duration) {
	h.t.Helper()
	h.clock.sleep(context.Background(), gap)
	h.d.probeDue = false
	if err := h.d.tick(context.Background(), h.clock.now()); err != nil {
		h.t.Fatalf("tick() = %v; want nil", err)
	}
}

// handOverRun is a running run whose verdict hands it over to another machine, its pair present or not.
func handOverRun(slug string, pairPresent bool) RunView {
	run := runningRun(slug)
	run.Verdict = runfsm.Verdict{Answer: runfsm.AnswerHandOver, Reason: runfsm.ReasonOwnedElsewhere}
	run.PairPresent = pairPresent
	return run
}

func TestRun_CatchUp(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	t.Run("a fresh start runs the catch-up before the first pass and a re-exec runs none", func(t *testing.T) {
		t.Parallel()
		for _, tt := range []struct {
			name         string
			reexec       bool
			wantFirst    string
			wantCatchUps int
		}{
			{"FreshStart", false, "catch-up", 1},
			{"ReExec", true, "pass r1 continue", 0},
		} {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				h := newLoopHarness(t, 2)
				if tt.reexec {
					if err := UpdateState(h.geom.StatePath, func(s *State) error { s.PID, s.StartTicks = os.Getpid(), loopStartTicks; return nil }); err != nil {
						t.Fatalf("UpdateState() = %v; want nil", err)
					}
				}
				var events []string
				h.deps.CatchUp = fakeCatchUpDeps(func() error { events = append(events, "catch-up"); return nil })
				h.deps.Runs = func() ([]RunView, error) { return []RunView{runningRun("r1")}, nil }
				h.deps.SpawnPass = func(slug string, continuePass, _ bool) (<-chan PassExit, error) {
					kind := "full"
					if continuePass {
						kind = "continue"
					}
					events = append(events, "pass "+slug+" "+kind)
					exit := make(chan PassExit, 1)
					exit <- PassExit{}
					return exit, nil
				}

				if err := h.run(t); err != nil {
					t.Fatalf("Run() = %v; want nil", err)
				}

				if len(events) == 0 || events[0] != tt.wantFirst {
					t.Errorf("events = %q; want %q first", events, tt.wantFirst)
				}
				if got := strings.Count(strings.Join(events, ";"), "catch-up"); got != tt.wantCatchUps {
					t.Errorf("catch-ups = %d; want %d", got, tt.wantCatchUps)
				}
			})
		}
	})

	t.Run("a sleep the watch sees runs the catch-up before the next pass, which is a full pass", func(t *testing.T) {
		t.Parallel()
		h := newLoopHarness(t, 4)
		if err := UpdateState(h.geom.StatePath, func(s *State) error { s.PID, s.StartTicks = os.Getpid(), loopStartTicks; return nil }); err != nil {
			t.Fatalf("UpdateState() = %v; want nil", err)
		}
		var events []string
		h.deps.CatchUp = fakeCatchUpDeps(func() error { events = append(events, "catch-up"); return nil })
		h.deps.Runs = func() ([]RunView, error) { return []RunView{runningRun("r1")}, nil }
		h.deps.SpawnPass = func(slug string, continuePass, _ bool) (<-chan PassExit, error) {
			kind := "full"
			if continuePass {
				kind = "continue"
			}
			events = append(events, "pass "+slug+" "+kind)
			exit := make(chan PassExit, 1)
			exit <- PassExit{}
			return exit, nil
		}
		h.perTick = func(n int) error {
			if n == 1 {
				h.clock.suspend(5 * time.Minute)
			}
			return nil
		}

		if err := h.run(t); err != nil {
			t.Fatalf("Run() = %v; want nil", err)
		}

		at := slices.Index(events, "catch-up")
		if at < 0 || strings.Count(strings.Join(events, ";"), "catch-up") != 1 || at+1 >= len(events) || events[at+1] != "pass r1 full" {
			t.Errorf("events = %q; want one catch-up followed by a full pass", events)
		}
	})

	t.Run("an unreachable origin spawns no pass, records the origin wait and retries after the delay", func(t *testing.T) {
		t.Parallel()
		h := newPassHarness(t, runningRun("r1"))
		pulls := 0
		h.d.deps.CatchUp = fakeCatchUpDeps(func() error {
			pulls++
			if pulls == 1 {
				return fmt.Errorf("pull: %w", ErrOriginUnreachable)
			}
			return nil
		})
		h.d.catchUpOwed = true

		h.tick(false)

		wait := h.stateRecord().CatchUp
		if wait == nil || wait.Reason != CatchUpWaitOrigin || !strings.Contains(wait.Detail, "origin is unreachable") {
			t.Errorf("recorded wait = %+v; want the origin wait", wait)
		}
		if len(h.fake.spawned) != 0 || len(h.fake.notices) != 0 {
			t.Errorf("passes, notices = %d, %d; want none while the catch-up is owed", len(h.fake.spawned), len(h.fake.notices))
		}

		h.tickAfter(500 * time.Millisecond)
		if pulls != 1 {
			t.Errorf("tries before the delay ran out = %d; want 1", pulls)
		}
		h.tickAfter(time.Second)
		if pulls != 2 || h.stateRecord().CatchUp != nil {
			t.Errorf("tries, wait after the delay = %d, %+v; want a second try that clears the wait", pulls, h.stateRecord().CatchUp)
		}
		if got := h.fake.spawned; len(got) != 1 || got[0].continuePass {
			t.Errorf("passes after the catch-up = %+v; want one full pass", got)
		}
	})

	t.Run("a refusal records refused, posts one notice and spawns nothing", func(t *testing.T) {
		t.Parallel()
		h := newPassHarness(t, runningRun("r1"))
		deps := fakeCatchUpDeps(func() error { return nil })
		deps.Register = func() error {
			return &boardengine.RegistrationError{Reason: "the config directory does not resolve", WayForward: "set XDG_CONFIG_HOME"}
		}
		h.d.deps.CatchUp = deps
		h.d.catchUpOwed = true

		h.tick(false)
		h.tick(false)

		wait := h.stateRecord().CatchUp
		if wait == nil || wait.Reason != CatchUpWaitRefused || !strings.Contains(wait.Detail, "set XDG_CONFIG_HOME") {
			t.Errorf("recorded wait = %+v; want the refused wait carrying the refusal's way forward", wait)
		}
		if len(h.fake.spawned) != 0 || len(h.fake.notices) != 1 {
			t.Fatalf("passes, notices = %d, %d; want no pass and one notice", len(h.fake.spawned), len(h.fake.notices))
		}
		for _, want := range []string{"set XDG_CONFIG_HOME", "lym beam stop", "lym beam start"} {
			if !strings.Contains(h.fake.notices[0], want) {
				t.Errorf("notice = %q; want it to carry %q", h.fake.notices[0], want)
			}
		}
	})

	t.Run("any other failure records failed with its detail, posts one notice and a later success clears the wait", func(t *testing.T) {
		t.Parallel()
		h := newPassHarness(t, runningRun("r1"))
		pulls := 0
		h.d.deps.CatchUp = fakeCatchUpDeps(func() error {
			pulls++
			if pulls <= 2 {
				return errors.New("the board diverged")
			}
			return nil
		})
		h.d.catchUpOwed = true

		h.tick(false)
		h.tick(false)

		wait := h.stateRecord().CatchUp
		if wait == nil || wait.Reason != CatchUpWaitFailed || !strings.Contains(wait.Detail, "the board diverged") {
			t.Errorf("recorded wait = %+v; want the failed wait with its detail", wait)
		}
		if len(h.fake.spawned) != 0 || len(h.fake.notices) != 1 || !strings.Contains(h.fake.notices[0], "the board diverged") {
			t.Errorf("passes, notices = %d, %q; want no pass and one notice naming the detail", len(h.fake.spawned), h.fake.notices)
		}

		h.tick(false)

		if h.stateRecord().CatchUp != nil {
			t.Errorf("recorded wait after the success = %+v; want it cleared", h.stateRecord().CatchUp)
		}
		if got := h.fake.spawned; len(got) != 1 || got[0].continuePass {
			t.Errorf("passes after the success = %+v; want a full pass", got)
		}
	})

	t.Run("a listed run handing over with its pair present owes the catch-up, gets no pass and is handed over", func(t *testing.T) {
		t.Parallel()
		h := newPassHarness(t, handOverRun("r1", true))
		var handOvers []string
		deps := fakeCatchUpDeps(func() error { return nil })
		deps.Runs = func() ([]string, error) { return []string{"r1"}, nil }
		deps.ReadRun = func(string) (CatchUpRun, error) { return CatchUpRun{Owner: "m2", OwnedElsewhere: true}, nil }
		deps.HandOver = func(slug, owner string) error { handOvers = append(handOvers, slug+" "+owner); return nil }
		h.d.deps.CatchUp = deps

		h.tick(true)
		h.tick(true)

		if !slices.Equal(handOvers, []string{"r1 m2"}) {
			t.Errorf("hand-overs = %q; want r1 handed over to m2", handOvers)
		}
		if len(h.fake.spawned) != 0 {
			t.Errorf("passes = %+v; want none for a run another machine owns", h.fake.spawned)
		}
	})

	t.Run("a listed run handing over with no pair present owes no catch-up", func(t *testing.T) {
		t.Parallel()
		h := newPassHarness(t, handOverRun("r1", false))
		catchUps := 0
		h.d.deps.CatchUp = fakeCatchUpDeps(func() error { catchUps++; return nil })

		for range 4 {
			h.tick(true)
		}

		if catchUps != 0 {
			t.Errorf("catch-ups = %d; want none, since the run has no pair here", catchUps)
		}
	})

	t.Run("a listed run handing over whose pair stays present after a successful catch-up owes it once", func(t *testing.T) {
		t.Parallel()
		h := newPassHarness(t, handOverRun("r1", true))
		catchUps := 0
		h.d.deps.CatchUp = fakeCatchUpDeps(func() error { catchUps++; return nil })

		for range 5 {
			h.tick(true)
		}
		if catchUps != 1 {
			t.Errorf("catch-ups while the run lists as handed over = %d; want 1", catchUps)
		}

		h.fake.runs[0] = runningRun("r1")
		h.tick(true)
		h.fake.runs[0] = handOverRun("r1", true)
		h.tick(true)
		h.tick(true)
		if catchUps != 2 {
			t.Errorf("catch-ups after the run stopped answering hand over and answered again = %d; want 2", catchUps)
		}
	})

	t.Run("a failing first read keeps the catch-up owed and spawns no pass, and a succeeding one runs once", func(t *testing.T) {
		t.Parallel()
		h := newPassHarness(t, runningRun("r1"))
		pulls, reads := 0, 0
		h.d.deps.CatchUp = fakeCatchUpDeps(func() error { pulls++; return nil })
		h.d.deps.Upgrade = fakeUpgradeDeps(t, func() ([]pauserecord.Entry, error) {
			reads++
			if reads == 1 {
				return nil, errors.New("pause record locked")
			}
			return nil, nil
		})
		h.d.catchUpOwed = true

		h.tick(false)

		wait := h.stateRecord().CatchUp
		if wait == nil || wait.Reason != CatchUpWaitFailed || !strings.Contains(wait.Detail, "pause record locked") {
			t.Errorf("recorded wait = %+v; want the failed wait carrying the first read's error", wait)
		}
		if pulls != 0 || len(h.fake.spawned) != 0 {
			t.Errorf("catch-ups, passes = %d, %d; want none while the first read fails", pulls, len(h.fake.spawned))
		}

		h.tickAfter(time.Second)
		if reads != 2 || pulls != 1 || h.stateRecord().CatchUp != nil {
			t.Errorf("reads, catch-ups, wait = %d, %d, %+v; want the first read retried once, the catch-up after it and the wait cleared", reads, pulls, h.stateRecord().CatchUp)
		}
		if got := h.fake.spawned; len(got) != 1 || got[0].continuePass {
			t.Errorf("passes after the first read = %+v; want one full pass", got)
		}

		h.d.catchUpOwed = true
		h.tick(false)
		if reads != 2 || pulls != 2 {
			t.Errorf("reads, catch-ups after a later absence = %d, %d; want no second first read and a second catch-up", reads, pulls)
		}
	})

	t.Run("a zero CatchUp runs passes as before", func(t *testing.T) {
		t.Parallel()
		h := newPassHarness(t, handOverRun("away", true), runningRun("r1"))

		h.tick(true)

		if got := h.fake.spawnsOf("r1"); len(got) != 1 {
			t.Errorf("passes of r1 = %+v; want one with no catch-up configured", got)
		}
		if h.d.catchUpOwed {
			t.Errorf("catchUpOwed = true; want a dobby with no catch-up seams to owe none")
		}
	})
}

func TestRun_SyncsBoltHoldEntries(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	type syncCall struct{ scope, held []string }
	newHarness := func(t *testing.T, syncErr *error) (*passHarness, *[]syncCall) {
		t.Helper()
		held, owed, acting := runningRun("r-held"), runningRun("r-owed"), runningRun("r-act")
		held.Verdict, owed.Verdict = waitVerdict(runfsm.ReasonHeld), waitVerdict(runfsm.ReasonPushOwed)
		h := newPassHarness(t, held, owed, acting)
		var calls []syncCall
		h.d.deps.CatchUp = CatchUpDeps{Runs: func() ([]string, error) { return []string{"r-held", "r-owed", "r-act", "stray"}, nil }}
		h.d.deps.SyncBoltHold = func(_ context.Context, scope, heldSlugs []string) error {
			calls = append(calls, syncCall{slices.Clone(scope), slices.Clone(heldSlugs)})
			return *syncErr
		}
		return h, &calls
	}

	t.Run("the scope is every slug the catch-up covers and a run whose push is owed is left out of the held list", func(t *testing.T) {
		t.Parallel()
		var syncErr error
		h, calls := newHarness(t, &syncErr)

		h.tick(false)

		want := syncCall{scope: []string{"r-held", "r-owed", "r-act", "stray"}, held: []string{"r-held"}}
		if len(*calls) != 1 || !slices.Equal((*calls)[0].scope, want.scope) || !slices.Equal((*calls)[0].held, want.held) {
			t.Errorf("sync calls = %+v; want %+v", *calls, want)
		}
	})

	t.Run("the held list is empty once no run reads held", func(t *testing.T) {
		t.Parallel()
		var syncErr error
		h, calls := newHarness(t, &syncErr)
		h.tick(false)

		h.fake.runs[0].Verdict = actVerdict
		h.tick(false)

		if len(*calls) != 2 || len((*calls)[1].held) != 0 {
			t.Errorf("sync calls = %+v; want a second call with an empty held list", *calls)
		}
	})

	t.Run("a failed sync is logged once and the tick goes on", func(t *testing.T) {
		t.Parallel()
		syncErr := errors.New("pause record locked")
		h, calls := newHarness(t, &syncErr)
		lines := h.recordLog()

		for range 3 {
			h.tick(true)
		}

		failing := 0
		for _, line := range *lines {
			if strings.HasPrefix(line, "bolt hold sync failing: pause record locked") {
				failing++
			}
		}
		if failing != 1 || len(*calls) != 3 {
			t.Errorf("failing lines, sync calls = %d, %d; want 1 and 3", failing, len(*calls))
		}
		if got := h.fake.spawnsOf("r-act"); len(got) == 0 {
			t.Errorf("passes of r-act = none; want the tick to go on past the failed sync")
		}
	})
}
