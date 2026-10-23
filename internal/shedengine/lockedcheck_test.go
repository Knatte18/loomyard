// lockedcheck_test.go covers Shed.LockedCheck: a refusal under the run lock stops Run and Step before any producer call or status write.

package shedengine

import (
	"bytes"
	"context"
	"errors"
	"os"
	"testing"

	"github.com/Knatte18/loomyard/internal/lock"
)

// TestLockedCheck_RefusesUnderTheRunLock covers Run and Step alike: a check error is returned matching both ErrLockedCheckRefused and the check's own error, no producer is called, the status bytes are unchanged, the check sees the run lock held, and a nil check drives the producer.
func TestLockedCheck_RefusesUnderTheRunLock(t *testing.T) {
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
