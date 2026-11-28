// lockedcheck_test.go covers the two hooks Run and Step call under the run lock before any producer:
// Shed.LockedCheck, whose refusal writes nothing, and Shed.Restore, whose refusal blocks the run.

package shedengine

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/state"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestLockedCheck_RefusesUnderTheRunLock covers Run and Step alike.
// A check error is returned matching both ErrLockedCheckRefused and the check's own error.
// It calls no producer, leaves the status bytes unchanged, and the check sees the run lock held.
// A restore runs once before the producer under that lock.
// A restore error blocks the run, and a done run calls no restore.
// A nil check and a nil restore drive the producer.
func TestLockedCheck_RefusesUnderTheRunLock(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	errCheck := errors.New("a cancel is ending this run; way forward: wait for it to finish")

	drives := []struct {
		name  string
		drive func(shed *Shed) error
	}{
		{"Run", func(shed *Shed) error { _, err := shed.Run(context.Background()); return err }},
		{"Step", func(shed *Shed) error { _, err := shed.Step(context.Background()); return err }},
	}
	for _, drive := range drives {
		t.Run(drive.name, func(t *testing.T) {
			t.Parallel()

			t.Run("RefusingCheck", func(t *testing.T) {
				t.Parallel()
				shed, statusPath, lockPath, statusLockPath := newTestShed(t)
				producer := fixedOutcomeProducer(Done, "")
				shed.Producers = []ProducerDef{{Name: "A", Producer: producer}}
				seedStatus(t, statusPath, statusLockPath, commonSeed("A"))
				before, err := os.ReadFile(statusPath)
				if err != nil {
					t.Fatalf("read status file before the drive: %v", err)
				}

				var lockHeldInCheck bool
				shed.LockedCheck = func() error {
					other, acquired, err := lock.TryAcquireWriteLock(lockPath)
					if err != nil {
						t.Errorf("TryAcquireWriteLock(...) = _, _, %v", err)
					}
					if acquired {
						_ = other.Release()
					}
					lockHeldInCheck = !acquired
					return errCheck
				}

				err = drive.drive(shed)
				if !errors.Is(err, ErrLockedCheckRefused) || !errors.Is(err, errCheck) {
					t.Fatalf("%s(...) error = %v; want it to match ErrLockedCheckRefused and the check's error", drive.name, err)
				}
				if !lockHeldInCheck {
					t.Errorf("the check saw the run lock free; want it held")
				}
				if producer.calls != 0 {
					t.Errorf("producer calls = %d; want 0", producer.calls)
				}
				after, err := os.ReadFile(statusPath)
				if err != nil {
					t.Fatalf("read status file after the drive: %v", err)
				}
				if !bytes.Equal(before, after) {
					t.Errorf("status file changed by a refused drive:\nbefore: %s\nafter:  %s", before, after)
				}
				if released, acquired, err := lock.TryAcquireWriteLock(lockPath); err != nil || !acquired {
					t.Errorf("run lock after the refusal: acquired = %v, err = %v; want it released", acquired, err)
				} else {
					_ = released.Release()
				}
			})

			restores := []struct {
				name      string
				state     State
				err       error
				wantState State
				wantCalls int
				wantRuns  int
			}{
				{"RestoreRunsBeforeTheProducer", StateRunning, nil, StateDone, 1, 1},
				{"RefusingRestoreBlocksTheRun", StateRunning, errors.New("the worktree is detached; way forward: check it out"), StateBlocked, 0, 1},
				{"DoneRunIsNotRestored", StateDone, nil, StateDone, 0, 0},
			}
			for _, tt := range restores {
				t.Run(tt.name, func(t *testing.T) {
					t.Parallel()
					shed, statusPath, lockPath, statusLockPath := newTestShed(t)
					producer := fixedOutcomeProducer(Done, "")
					shed.Producers = []ProducerDef{{Name: "A", Producer: producer}}
					seed := commonSeed("A")
					seed.State = tt.state
					seedStatus(t, statusPath, statusLockPath, seed)
					runs := 0
					shed.Restore = func() error {
						runs++
						other, acquired, err := lock.TryAcquireWriteLock(lockPath)
						if err != nil {
							t.Errorf("TryAcquireWriteLock(...) = _, _, %v", err)
						}
						if acquired {
							_ = other.Release()
							t.Error("the restore saw the run lock free; want it held")
						}
						if producer.calls != 0 {
							t.Errorf("producer calls before the restore = %d; want 0", producer.calls)
						}
						return tt.err
					}

					if err := drive.drive(shed); err != nil {
						t.Fatalf("%s(...) = %v; want nil", drive.name, err)
					}

					if runs != tt.wantRuns || producer.calls != tt.wantCalls {
						t.Errorf("restores = %d, producer calls = %d; want %d and %d", runs, producer.calls, tt.wantRuns, tt.wantCalls)
					}
					got, _, err := state.ReadJSON[Status](statusPath, statusLockPath)
					if err != nil {
						t.Fatalf("read status file after the drive: %v", err)
					}
					if got.State != tt.wantState || got.CurrentProducer != "A" {
						t.Errorf("status = %s at %q; want %s at A", got.State, got.CurrentProducer, tt.wantState)
					}
					if tt.err == nil {
						return
					}
					if !strings.HasPrefix(got.Error, ReasonPreconditionUnmet) || !strings.HasSuffix(got.Error, "way forward: check it out") {
						t.Errorf("status error = %q; want %q first and the way forward last", got.Error, ReasonPreconditionUnmet)
					}
				})
			}

			t.Run("NilCheck", func(t *testing.T) {
				t.Parallel()
				shed, statusPath, _, statusLockPath := newTestShed(t)
				producer := fixedOutcomeProducer(Done, "")
				shed.Producers = []ProducerDef{{Name: "A", Producer: producer}}
				seedStatus(t, statusPath, statusLockPath, commonSeed("A"))

				if err := drive.drive(shed); err != nil {
					t.Fatalf("%s(...) = %v; want nil with no check", drive.name, err)
				}
				if producer.calls != 1 {
					t.Errorf("producer calls = %d; want 1", producer.calls)
				}
			})
		})
	}
}
