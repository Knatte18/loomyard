//go:build tier0

// cancelcmd_test.go pins the cancel verb's pre-resolution refusals and the helpers that wire its seams: the pause readings, the claim clear, the tackler stop and batten's two locks.

package battencli

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/sheddriver"
	"github.com/Knatte18/loomyard/internal/tenderengine"
	"github.com/Knatte18/loomyard/internal/testkit/envelope"
)

// TestTakeBattenLocks pins that both locks are held until release, and that a busy lock names its own path with nothing held.
func TestTakeBattenLocks(t *testing.T) {
	t.Parallel()

	const wait = 20 * time.Millisecond
	// tryTake reports whether a fresh handle takes the lock at path, releasing it at once.
	tryTake := func(t *testing.T, path string) bool {
		t.Helper()
		held, acquired, err := lock.TryAcquireWriteLock(path)
		if err != nil {
			t.Fatalf("TryAcquireWriteLock(%s): %v", path, err)
		}
		if acquired {
			if err := held.Release(); err != nil {
				t.Fatalf("Release(%s): %v", path, err)
			}
		}
		return acquired
	}
	// hold takes the lock at path on its own handle for the test's length.
	hold := func(t *testing.T, path string) {
		t.Helper()
		held, err := lock.AcquireWriteLock(path)
		if err != nil {
			t.Fatalf("AcquireWriteLock(%s): %v", path, err)
		}
		t.Cleanup(func() { _ = held.Release() })
	}

	t.Run("BothFreeAreHeldUntilRelease", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		driverLock, runLock := filepath.Join(dir, "driver.lock"), filepath.Join(dir, "run.lock")

		release, busy, err := takeBattenLocks(driverLock, runLock, wait)

		if err != nil || busy != "" {
			t.Fatalf("takeBattenLocks(free) = busy %q, err %v; want both free", busy, err)
		}
		if tryTake(t, driverLock) || tryTake(t, runLock) {
			t.Error("a fresh handle took a lock the cancel holds; want both held")
		}
		release()
		if !tryTake(t, driverLock) || !tryTake(t, runLock) {
			t.Error("a fresh handle could not take a lock after release; want both free")
		}
	})

	t.Run("RunLockHeldElsewhereIsBusyAndReleasesTheDriverLock", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		driverLock, runLock := filepath.Join(dir, "driver.lock"), filepath.Join(dir, "run.lock")
		hold(t, runLock)

		release, busy, err := takeBattenLocks(driverLock, runLock, wait)

		if err != nil || busy != runLock || release != nil {
			t.Fatalf("takeBattenLocks(run lock held) = busy %q, release nil %v, err %v; want busy %q", busy, release == nil, err, runLock)
		}
		if !tryTake(t, driverLock) {
			t.Error("the driver lock is still held after a busy report; want a busy report to hold nothing")
		}
	})

	t.Run("DriverLockHeldElsewhereIsBusyAndTakesNoRunLock", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		driverLock, runLock := filepath.Join(dir, "driver.lock"), filepath.Join(dir, "run.lock")
		hold(t, driverLock)

		release, busy, err := takeBattenLocks(driverLock, runLock, wait)

		if err != nil || busy != driverLock || release != nil {
			t.Fatalf("takeBattenLocks(driver lock held) = busy %q, release nil %v, err %v; want busy %q", busy, release == nil, err, driverLock)
		}
		if !tryTake(t, runLock) {
			t.Error("the run lock is held after a busy driver lock; want it never taken")
		}
	})
}

// TestCancelPauseEntries pins which pause entries refuse a cancel and which of them the cancel lifts.
func TestCancelPauseEntries(t *testing.T) {
	t.Parallel()

	epoch := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	hubEntry := pauserecord.Entry{Scope: pauserecord.ScopeHub, Caller: "orch", PausedAt: epoch}
	ownEntry := pauserecord.Entry{Scope: cancelSlug, Caller: "orch", PausedAt: epoch.Add(time.Minute)}
	otherEntry := pauserecord.Entry{Scope: "other-slug", Caller: "orch", PausedAt: epoch.Add(2 * time.Minute)}
	liftedHub := pauserecord.Entry{Scope: pauserecord.ScopeHub, Caller: "orch", PausedAt: epoch, ResumedAt: epoch.Add(time.Hour)}

	tests := []struct {
		name       string
		record     pauserecord.Record
		wantHub    bool
		wantLifted []tenderengine.EntryRef
	}{
		{"LiveHubEntryRefusesAndIsNotLifted", pauserecord.Record{Live: []pauserecord.Entry{hubEntry}}, true, nil},
		{"OwnSlugEntryIsLiftedAndNeverRefuses", pauserecord.Record{Live: []pauserecord.Entry{ownEntry}}, false, []tenderengine.EntryRef{{Scope: cancelSlug, Caller: "orch", PausedAt: ownEntry.PausedAt}}},
		{"OtherSlugEntryIsNeitherLiftedNorRefuses", pauserecord.Record{Live: []pauserecord.Entry{otherEntry}}, false, nil},
		{"LiftedHubEntryDoesNotRefuse", pauserecord.Record{Resumed: []pauserecord.Entry{liftedHub}}, false, nil},
		{"OwnEntryAmongOthersIsTheOnlyOneLifted", pauserecord.Record{Live: []pauserecord.Entry{otherEntry, ownEntry, hubEntry}}, true, []tenderengine.EntryRef{{Scope: cancelSlug, Caller: "orch", PausedAt: ownEntry.PausedAt}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := hubPauseLive(tt.record); got != tt.wantHub {
				t.Errorf("hubPauseLive() = %v; want %v", got, tt.wantHub)
			}
			if got := slugPauseRefs(tt.record, cancelSlug); !slices.Equal(got, tt.wantLifted) {
				t.Errorf("slugPauseRefs() = %v; want %v", got, tt.wantLifted)
			}
		})
	}
}

// TestClearRunClaim pins that only a run-form status is cleared, and that a failed read or write is returned.
func TestClearRunClaim(t *testing.T) {
	t.Parallel()

	running, done, backlog := "running · Run-Shed", "done", "backlog"
	errRead, errWrite := errors.New("read failed"), errors.New("write failed")
	tests := []struct {
		name        string
		status      *string
		found       bool
		readErr     error
		writeErr    error
		wantWrites  int
		wantCleared bool
		wantFound   bool
		wantErr     error
	}{
		{name: "RunFormStatusIsCleared", status: &running, found: true, wantWrites: 1, wantCleared: true, wantFound: true},
		{name: "DoneStatusIsLeftAlone", status: &done, found: true, wantFound: true},
		{name: "NonRunStatusIsLeftAlone", status: &backlog, found: true, wantFound: true},
		{name: "EntryWithNoStatusIsLeftAlone", found: true, wantFound: true},
		{name: "AbsentEntryIsLeftAlone"},
		{name: "ReadErrorIsReturned", readErr: errRead, wantErr: errRead},
		{name: "WriteErrorIsReturned", status: &running, found: true, writeErr: errWrite, wantWrites: 1, wantFound: true, wantErr: errWrite},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var written []*string
			cleared, found, err := clearRunClaim(
				func() (*string, bool, error) { return tt.status, tt.found, tt.readErr },
				func(status *string) error { written = append(written, status); return tt.writeErr },
			)

			if !errors.Is(err, tt.wantErr) || cleared != tt.wantCleared || found != tt.wantFound {
				t.Errorf("clearRunClaim() = cleared %v, found %v, err %v; want %v, %v, %v", cleared, found, err, tt.wantCleared, tt.wantFound, tt.wantErr)
			}
			if len(written) != tt.wantWrites {
				t.Fatalf("clearRunClaim() wrote %d statuses; want %d", len(written), tt.wantWrites)
			}
			if tt.wantWrites == 1 && written[0] != nil {
				t.Errorf("clearRunClaim() wrote %q; want nil, which clears the status", *written[0])
			}
		})
	}
}

// TestStopRecordedTackler pins that the strand a tackler marker names is stopped and its guid returned.
func TestStopRecordedTackler(t *testing.T) {
	t.Parallel()

	errStop := errors.New("stop failed")
	tests := []struct {
		name      string
		marker    *sheddriver.TacklerMarker
		stopErr   error
		wantGUID  string
		wantStops []string
		wantErr   error
	}{
		{name: "NoMarkerStopsNothing"},
		{name: "MarkerStopsItsStrand", marker: &sheddriver.TacklerMarker{StrandGUID: "guid-1"}, wantGUID: "guid-1", wantStops: []string{"guid-1"}},
		{name: "StopErrorIsReturned", marker: &sheddriver.TacklerMarker{StrandGUID: "guid-2"}, stopErr: errStop, wantStops: []string{"guid-2"}, wantErr: errStop},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			markerPath := filepath.Join(t.TempDir(), "scratch", "tackler-marker.json")
			if tt.marker != nil {
				if err := sheddriver.WriteTacklerMarker(markerPath, *tt.marker); err != nil {
					t.Fatalf("WriteTacklerMarker: %v", err)
				}
			}
			var stops []string
			guid, err := stopRecordedTackler(markerPath, func(guid string) error { stops = append(stops, guid); return tt.stopErr })

			if guid != tt.wantGUID || !errors.Is(err, tt.wantErr) || !slices.Equal(stops, tt.wantStops) {
				t.Errorf("stopRecordedTackler() = %q, %v with stops %v; want %q, %v with stops %v", guid, err, stops, tt.wantGUID, tt.wantErr, tt.wantStops)
			}
		})
	}
}

// TestCancelCmd_RefusesBeforeAnything pins that an omitted or reserved slug refuses before any path is resolved, and a named slug outside a repository refuses with the location's resolve error.
// The prime refusal is RefuseUnlessPrime's, pinned by TestRefuseNonPrime.
func TestCancelCmd_RefusesBeforeAnything(t *testing.T) {
	t.Parallel()

	outside := t.TempDir()
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{"OmittedSlugRefusesByName", []string{"cancel"}, "no slug given"},
		{"ReservedSlugRefusesAsReserved", []string{"cancel", "self"}, "reserved"},
		{"NamedSlugOutsideARepositoryRefusesWithTheResolveError", []string{"cancel", cancelSlug}, "not a git repository"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var out bytes.Buffer

			if code := RunCLIIn(outside, &out, tt.args); code != 1 {
				t.Errorf("RunCLIIn(%v) = %d; want 1; output: %s", tt.args, code, out.String())
			}

			envelope.RequireErr(t, out.String(), tt.wantErr)
			entries, err := os.ReadDir(outside)
			if err != nil || len(entries) != 0 {
				t.Errorf("the working directory holds %v, %v after the refusal; want it untouched", entries, err)
			}
		})
	}
}
