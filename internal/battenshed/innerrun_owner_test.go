// innerrun_owner_test.go covers the inner-run watch's owner check over a fake clock and fake seams:
// when the check runs, what an owned-elsewhere verdict does to the parent lock, the pause and the notice, and how a spawn refused as owned elsewhere is held.

package battenshed

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/testkit/shedfake"
)

// ownerNotices counts the notices that name the adopt command, the one an owned-elsewhere run sends.
func (h *lockHarness) ownerNotices() int {
	count := 0
	for _, line := range h.notices {
		if strings.Contains(line, "lyx adopt") {
			count++
		}
	}
	return count
}

// elsewhereVerdict reads every check as the run owned by machine m2.
func elsewhereVerdict(int) (OwnerVerdict, error) {
	return OwnerVerdict{Owner: "m2", Elsewhere: true}, nil
}

// TestInnerRun_OwnerCheck asserts the owner check runs after a sleep of a minute or more and after a pause hold ends, and that an owned-elsewhere verdict releases the child's parent lock record, pauses with the owner and sends one notice.
// It asserts a Finalize past its parent-side merge keeps the record, an unreachable verdict or a check error is retried at the next check, a done child waits out an owner hold, and a spawn refused as owned elsewhere pauses, writes no marker and spawns nothing more while the owner stands.
func TestInnerRun_OwnerCheck(t *testing.T) {
	t.Parallel()
	running := shedengine.Status{State: shedengine.StateRunning, CurrentProducer: "Webster"}
	finalize := func(state shedengine.State, transient string) shedengine.Status {
		return shedengine.Status{State: state, CurrentProducer: finalizeRow, Transient: transient}
	}
	ownerHeld := func(int) pauserecord.Coverage {
		return pauserecord.Coverage{Covered: true, OwnerHeld: true, Entry: pauserecord.Entry{Scope: "myslug", Caller: pauserecord.OwnerCaller}}
	}
	tests := []struct {
		name     string
		status   shedengine.Status
		sleepFor time.Duration
		// configure sets the harness's seams before the Call.
		configure    func(h *lockHarness)
		wantChecks   int
		wantReleased bool
		// wantPauses counts the attempts of the owner pause.
		wantPauses  int
		wantNotices int
	}{
		{name: "a sleep of a minute runs the check once", status: running, sleepFor: 2 * time.Minute, wantChecks: 1},
		{name: "a shorter sleep does not run it", status: running, sleepFor: 30 * time.Second},
		{name: "a pause hold ending runs it", status: running, configure: func(h *lockHarness) { h.coveredThrough = 3 }, wantChecks: 1},
		{
			name: "an owned-elsewhere verdict releases the record, pauses with the owner and sends one notice", status: running, sleepFor: 2 * time.Minute,
			configure:  func(h *lockHarness) { h.verdict = elsewhereVerdict },
			wantChecks: 1, wantReleased: true, wantPauses: 1, wantNotices: 1,
		},
		{
			name: "an unreachable verdict retries at the next check and pauses nothing", status: running, sleepFor: 2 * time.Minute,
			configure: func(h *lockHarness) {
				h.verdict = func(n int) (OwnerVerdict, error) { return OwnerVerdict{Unreachable: n <= 2}, nil }
			},
			wantChecks: 3,
		},
		{
			name: "a check error retries at the next check", status: running, sleepFor: 2 * time.Minute,
			configure: func(h *lockHarness) {
				h.verdict = func(n int) (OwnerVerdict, error) {
					if n == 1 {
						return OwnerVerdict{}, errors.New("origin unreadable")
					}
					return OwnerVerdict{}, nil
				}
			},
			wantChecks: 2,
		},
		{
			name: "a failed owner pause is retried at the next probe", status: running, sleepFor: 2 * time.Minute,
			configure: func(h *lockHarness) {
				h.verdict = elsewhereVerdict
				h.pauseErr = func(n int) error {
					if n == 1 {
						return errors.New("tender unavailable")
					}
					return nil
				}
			},
			wantChecks: 2, wantReleased: true, wantPauses: 2, wantNotices: 1,
		},
		{
			name: "an owner-held coverage releases the record with no check", status: running,
			configure:    func(h *lockHarness) { h.coverage = ownerHeld },
			wantReleased: true,
		},
		{
			name: "a running Finalize with the parent holding the work keeps the record", status: finalize(shedengine.StateRunning, ""), sleepFor: 2 * time.Minute,
			configure:  func(h *lockHarness) { h.verdict, h.parentHolds = elsewhereVerdict, true },
			wantChecks: 1, wantPauses: 1, wantNotices: 1,
		},
		{
			name: "a failed transient Finalize with no re-step keeps the record", status: finalize(shedengine.StateFailed, "git-transport"), sleepFor: 2 * time.Minute,
			configure:  func(h *lockHarness) { h.verdict, h.parentHolds = elsewhereVerdict, true },
			wantChecks: 1, wantPauses: 1, wantNotices: 1,
		},
		{
			name: "a failed Finalize with a re-step recorded releases the record", status: finalize(shedengine.StateFailed, "git-transport"), sleepFor: 2 * time.Minute,
			configure: func(h *lockHarness) {
				h.verdict, h.parentHolds, h.restepped = elsewhereVerdict, true, true
			},
			wantChecks: 1, wantReleased: true, wantPauses: 1, wantNotices: 1,
		},
		{
			name: "a blocked Finalize with the parent holding the work releases the record", status: finalize(shedengine.StateBlocked, ""), sleepFor: 2 * time.Minute,
			configure:  func(h *lockHarness) { h.verdict, h.parentHolds = elsewhereVerdict, true },
			wantChecks: 1, wantReleased: true, wantPauses: 1, wantNotices: 1,
		},
		{
			name: "a running Finalize whose parent lacks the work releases the record", status: finalize(shedengine.StateRunning, ""), sleepFor: 2 * time.Minute,
			configure:  func(h *lockHarness) { h.verdict = elsewhereVerdict },
			wantChecks: 1, wantReleased: true, wantPauses: 1, wantNotices: 1,
		},
		{
			name: "an unreadable parent read at a running Finalize keeps the record", status: finalize(shedengine.StateRunning, ""), sleepFor: 2 * time.Minute,
			configure: func(h *lockHarness) {
				h.verdict, h.parentHoldsErr = elsewhereVerdict, errors.New("branch unreadable")
			},
			wantChecks: 1, wantPauses: 1, wantNotices: 1,
		},
		{
			name: "an unreadable re-step read at a failed Finalize keeps the record", status: finalize(shedengine.StateFailed, "git-transport"), sleepFor: 2 * time.Minute,
			configure: func(h *lockHarness) {
				h.verdict, h.restepErr = elsewhereVerdict, errors.New("settle records unreadable")
			},
			wantChecks: 1, wantPauses: 1, wantNotices: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := newLockHarness(t, tt.status)
			h.record = loopRecord()
			// A confirmed spawn keeps a running child out of the spawn hold, so the checks themselves meet the coverage.
			recordSpawnConfirmed("innerrun", "myslug", h.scratchDir, SpawnConfirmedFile(h.scratchDir, "innerrun"))
			if tt.sleepFor > 0 {
				h.clock.onSleep = func(call int) {
					if call == 2 {
						h.sleep(tt.sleepFor)
					}
				}
			}
			if tt.configure != nil {
				tt.configure(h)
			}

			h.runChecks(6)

			if h.ownerCalls != tt.wantChecks {
				t.Errorf("owner checks = %d; want %d", h.ownerCalls, tt.wantChecks)
			}
			if got := len(h.released) == 1; got != tt.wantReleased || (h.record == nil) != tt.wantReleased {
				t.Errorf("released = %v (record left: %v); want released %v", h.released, h.record != nil, tt.wantReleased)
			}
			if len(h.ownerPauses) != tt.wantPauses {
				t.Errorf("owner pauses = %q; want %d", h.ownerPauses, tt.wantPauses)
			}
			for _, owner := range h.ownerPauses {
				if owner != "m2" {
					t.Errorf("owner pause named %q; want m2", owner)
				}
			}
			if got := h.ownerNotices(); got != tt.wantNotices {
				t.Errorf("owner notices = %q; want %d", h.notices, tt.wantNotices)
			}
		})
	}

	t.Run("a running child with no driver is noticed once the owner pause lifts and an owned-here verdict follows, and not while the pause covers it", func(t *testing.T) {
		t.Parallel()
		h := newLockHarness(t, running)
		h.record = loopRecord()
		recordSpawnConfirmed("innerrun", "myslug", h.scratchDir, SpawnConfirmedFile(h.scratchDir, "innerrun"))
		h.verdict = func(n int) (OwnerVerdict, error) {
			if n == 1 {
				return OwnerVerdict{Owner: "m2", Elsewhere: true}, nil
			}
			return OwnerVerdict{}, nil
		}
		h.coverage = func(call int) pauserecord.Coverage {
			return pauserecord.Coverage{Covered: call < 5, OwnerHeld: true, Entry: pauserecord.Entry{Scope: "myslug", Caller: pauserecord.OwnerCaller}}
		}
		driverDeadNotices := func() int {
			count := 0
			for _, line := range h.notices {
				if strings.Contains(line, "driver strand is dead") {
					count++
				}
			}
			return count
		}
		coveredNotices := -1
		h.clock.onSleep = func(call int) {
			switch call {
			case 2:
				h.sleep(2 * time.Minute)
			case 4:
				coveredNotices = driverDeadNotices()
			}
		}

		h.runChecks(10)

		if coveredNotices != 0 || driverDeadNotices() != 1 {
			t.Errorf("driver-dead notices = %d while covered, %d in the end; want 0 and 1 (notices %q)", coveredNotices, driverDeadNotices(), h.notices)
		}
	})

	t.Run("a done child under an owner-held pause is held until the coverage clears", func(t *testing.T) {
		t.Parallel()
		h := newLockHarness(t, shedengine.Status{State: shedengine.StateDone})
		h.coverage = func(call int) pauserecord.Coverage {
			coverage := ownerHeld(call)
			coverage.Covered = call < 5
			return coverage
		}
		h.clock.pauseAtSleep = 50

		outcome, _ := shedfake.CallOK(t, h.producer)

		if outcome != shedengine.Done || h.clock.sleepCalls != 5 {
			t.Errorf("Call() = %q after %d checks; want Done once the coverage cleared at check 5", outcome, h.clock.sleepCalls)
		}
	})

	for _, pauseFails := range []bool{false, true} {
		name := "an awaiting child whose resume spawn is refused as owned elsewhere spawns no more"
		if pauseFails {
			name += " when the owner pause fails too"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			h := newLockHarness(t, awaitingAt(lockGateRow))
			h.decision = &ChildDecision{Kind: DecisionApprove, At: "2026-01-01T10:00:00Z", HeadSHA: "abc"}
			h.spawnErr = &ChildOwnedElsewhereError{Owner: "m2"}
			h.verdict = elsewhereVerdict
			if pauseFails {
				h.pauseErr = func(int) error { return errors.New("tender unavailable") }
			}

			h.runChecks(6)

			if *h.spawns != 1 {
				t.Errorf("Spawn calls = %d; want 1", *h.spawns)
			}
			if len(h.ownerPauses) == 0 {
				t.Error("owner pauses = none; want the refusal to pause the run")
			}
			if got := h.ownerNotices(); got != 1 {
				t.Errorf("owner notices = %q; want 1", h.notices)
			}
			if _, err := os.Stat(decisionActedFile(h.scratchDir, "innerrun")); !os.IsNotExist(err) {
				t.Errorf("decision-acted marker stat err = %v; want it absent", err)
			}
		})
	}

	t.Run("a respawn refused as owned elsewhere is an exempt Stuck and spawns nothing until the check reads owned here", func(t *testing.T) {
		t.Parallel()
		clock := &fakeClock{}
		_, spawns, deps := newInnerRunDeps(t, nil, nil, []statusResult{{status: shedengine.Status{State: shedengine.StateRunning}, found: true}}, clock)
		deps.NoticeProbe = time.Second
		deps.Spawn = func(context.Context) error {
			*spawns++
			if *spawns == 1 {
				return &ChildOwnedElsewhereError{Owner: "m2"}
			}
			return nil
		}
		checks := 0
		deps.OwnerCheck = func(context.Context) (OwnerVerdict, error) {
			checks++
			if checks < 3 {
				if *spawns != 1 {
					t.Errorf("Spawn calls = %d at owner check %d; want 1: no spawn while the owner stands", *spawns, checks)
				}
				return OwnerVerdict{Owner: "m2", Elsewhere: true}, nil
			}
			return OwnerVerdict{}, nil
		}
		producer := NewInnerRun("innerrun", "myslug", deps, time.Second, t.TempDir(), testGrace)

		outcome, pointer, err := producer.Call(context.Background())
		if err != nil || outcome != shedengine.Stuck || !pointer.BudgetExempt || !strings.Contains(pointer.Path, "m2") {
			t.Fatalf("first Call() = %q, %+v, %v; want a budget-exempt Stuck naming m2", outcome, pointer, err)
		}

		clock.pauseAtSleep = 50
		if _, _, err := producer.Call(context.Background()); err != nil {
			t.Fatalf("second Call() error = %v", err)
		}
		if *spawns != 2 || checks != 3 {
			t.Errorf("Spawn calls = %d, owner checks = %d; want 2 and 3: the spawn waits for the owned-here reading", *spawns, checks)
		}
	})

	t.Run("a binary re-step refused as owned elsewhere pauses, writes no marker and does not spawn again", func(t *testing.T) {
		t.Parallel()
		h := newRestepHarness(t)
		h.spawnErrs = []error{&ChildOwnedElsewhereError{Owner: "m2"}}

		h.call(h.producer(), 6)

		if h.spawns != 1 || h.marker() != "" {
			t.Errorf("spawns = %d, marker = %q; want 1 and none", h.spawns, h.marker())
		}
	})

	t.Run("a Publish retry resume refused as owned elsewhere pauses, writes no marker change and does not spawn again", func(t *testing.T) {
		t.Parallel()
		h := newRestepHarness(t)
		h.status.State = shedengine.StatePaused
		h.status.CurrentProducer = "target-row"
		h.retryTaking = true
		h.spawnErrs = []error{&ChildOwnedElsewhereError{Owner: "m2"}}

		h.call(h.producer(), 6)

		raw, err := os.ReadFile(publishRetryFile(h.scratchDir, "innerrun"))
		if h.spawns != 1 || err != nil || !strings.HasPrefix(string(raw), publishRetryStarted+" ") {
			t.Errorf("spawns = %d, publish-retry marker = %q (%v); want 1 and the marker still started", h.spawns, raw, err)
		}
	})
}
