// cancelmarker_test.go pins the reading of a cancel under way: the intent in batten's record and the cancel's lock.
// It also pins the refusal words a reading earns.

package battencli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/state"
	"github.com/Knatte18/loomyard/internal/testkit/locationkit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestCancelMarker reads one slug's cancel over a temporary location, once per state of the record and the lock.
func TestCancelMarker(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	const slug = "slug"
	const takeWait = 200 * time.Millisecond
	intent := shedengine.CancelIntent{StartedAt: "2026-11-15T09:00:00Z", Machine: "other-machine"}
	writeRecord := func(t *testing.T, location *lymcwd.Location, status shedengine.Status) {
		t.Helper()
		if err := state.WriteJSON(shedrun.StatusFile(location, slug), shedrun.StatusLock(location, slug), status); err != nil {
			t.Fatalf("write the record: %v", err)
		}
	}
	holdLock := func(t *testing.T, location *lymcwd.Location) {
		t.Helper()
		held, err := takeCancelMarker(location, slug, takeWait)
		if err != nil {
			t.Fatalf("takeCancelMarker: %v", err)
		}
		t.Cleanup(func() { _ = held.release() })
	}
	withIntent := func(change func(*shedengine.CancelIntent)) shedengine.Status {
		withChange := intent
		if change != nil {
			change(&withChange)
		}
		return shedengine.Status{State: shedengine.StateRunning, Cancel: &withChange}
	}

	tests := []struct {
		name        string
		setup       func(t *testing.T, location *lymcwd.Location)
		wantPresent bool
		wantAlive   bool
		wantIntent  shedengine.CancelIntent
		// wantRefusal is the refusal's whole text when the reading is present.
		wantRefusal string
	}{
		{
			name:  "AbsentRecordAndLockReadsAbsent",
			setup: func(*testing.T, *lymcwd.Location) {},
		},
		{
			name: "IntentWithAFreeLockReadsInterruptedNamingItsMachine",
			setup: func(t *testing.T, location *lymcwd.Location) {
				writeRecord(t, location, withIntent(nil))
			},
			wantPresent: true,
			wantIntent:  intent,
			wantRefusal: `a cancel of slug was interrupted; way forward: run "lym batten cancel slug" from the prime of machine other-machine to finish it`,
		},
		{
			name: "IntentWithRemoveSetNamesTheRemoveFlag",
			setup: func(t *testing.T, location *lymcwd.Location) {
				writeRecord(t, location, withIntent(func(i *shedengine.CancelIntent) { i.Remove = true }))
			},
			wantPresent: true,
			wantIntent:  shedengine.CancelIntent{StartedAt: intent.StartedAt, Machine: intent.Machine, Remove: true},
			wantRefusal: `a cancel of slug was interrupted; way forward: run "lym batten cancel slug --remove" from the prime of machine other-machine to finish it`,
		},
		{
			name: "IntentWithAHeldLockReadsLiveSinceItsStart",
			setup: func(t *testing.T, location *lymcwd.Location) {
				writeRecord(t, location, withIntent(nil))
				holdLock(t, location)
			},
			wantPresent: true,
			wantAlive:   true,
			wantIntent:  intent,
			wantRefusal: "a cancel of slug is under way on this machine since 2026-11-15T09:00:00Z; way forward: wait for it to end, then run the verb again",
		},
		{
			name:        "HeldLockWithNoIntentYetReadsLiveWithNoStartTime",
			setup:       func(t *testing.T, location *lymcwd.Location) { holdLock(t, location) },
			wantPresent: true,
			wantAlive:   true,
			wantRefusal: "a cancel of slug is under way on this machine; way forward: wait for it to end, then run the verb again",
		},
		{
			name: "FinishedIntentReadsAbsent",
			setup: func(t *testing.T, location *lymcwd.Location) {
				status := withIntent(nil)
				status.Cancel.Finish = &shedengine.CancelFinish{EndedAt: "2026-11-15T09:05:00Z"}
				writeRecord(t, location, status)
			},
		},
		{
			name: "UndecodableRecordReadsPresentWithAnEmptyIntent",
			setup: func(t *testing.T, location *lymcwd.Location) {
				path := shedrun.StatusFile(location, slug)
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(`{"state":`), 0o644); err != nil {
					t.Fatal(err)
				}
			},
			wantPresent: true,
			wantRefusal: "a cancel of slug may have been interrupted, but its batten record does not decode, so a cancel cannot read it; way forward: repair the record by hand, then run the verb again",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			location := locationkit.Location(t.TempDir(), "prime", ".")
			tt.setup(t, location)

			reading, err := readCancelMarker(location, slug)

			if err != nil || reading.Present != tt.wantPresent || reading.WriterAlive != tt.wantAlive || reading.Intent != tt.wantIntent {
				t.Fatalf("readCancelMarker() = %+v, %v; want present %v, alive %v, intent %+v", reading, err, tt.wantPresent, tt.wantAlive, tt.wantIntent)
			}
			if !tt.wantPresent {
				return
			}
			if got := cancelMarkerRefusal(slug, reading).Error(); got != tt.wantRefusal {
				t.Errorf("cancelMarkerRefusal() = %q; want %q", got, tt.wantRefusal)
			}
			if tt.wantAlive {
				if _, err := takeCancelMarker(location, slug, 0); err == nil || !strings.Contains(err.Error(), "way forward: wait for it to end") {
					t.Errorf("takeCancelMarker(held, no wait) error = %v; want the live-cancel refusal", err)
				}
			}
		})
	}
}

// TestCancelMarker_ReadsNothingFromAPrimeWithNoScratchRoot pins the read of a fresh prime.
// It creates no lock file or scratch root.
//
//testtiming:keep pins that a read of a prime with no scratch root creates no lock file or scratch directory
func TestCancelMarker_ReadsNothingFromAPrimeWithNoScratchRoot(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	location := locationkit.Location(t.TempDir(), "prime", ".")

	reading, err := readCancelMarker(location, "slug")

	if err != nil || reading.Present {
		t.Fatalf("readCancelMarker() = %+v, %v; want absent with no error", reading, err)
	}
	for _, path := range []string{shedrun.CancelMarkerLock(location, "slug"), shedrun.ScratchRoot(location)} {
		if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
			t.Errorf("stat of %s after the read = %v; want not exist", path, statErr)
		}
	}
}

// TestJudgeCancelMarker pins each reading of the cancel's probes.
// The cases include the intent being finished between the first read and the shared lock,
// and a lock held before the intent is written.
//
//testtiming:keep pins the race rows of the three probes, the intent finished between the read and the shared lock and a lock held before the intent is written
func TestJudgeCancelMarker(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name                                    string
		intentBefore, lockFree, intentUnderLock bool
		wantPresent, wantWriterAlive            bool
	}{
		{"NoIntentWithTheLockFreeReadsNotPresent", false, true, false, false, false},
		{"NoIntentWithTheLockHeldElsewhereReadsWriterAlive", false, false, false, true, true},
		{"IntentWithTheLockHeldElsewhereReadsWriterAlive", true, false, false, true, true},
		{"IntentWithTheLockFreeAndStillThereReadsWriterGone", true, true, true, true, false},
		{"FinishedBetweenTheReadAndTheSharedLockReadsNotPresent", true, true, false, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			present, writerAlive := judgeCancelMarker(tt.intentBefore, tt.lockFree, tt.intentUnderLock)
			if present != tt.wantPresent || writerAlive != tt.wantWriterAlive {
				t.Errorf("judgeCancelMarker(%v, %v, %v) = %v, %v; want %v, %v",
					tt.intentBefore, tt.lockFree, tt.intentUnderLock, present, writerAlive, tt.wantPresent, tt.wantWriterAlive)
			}
		})
	}
}
