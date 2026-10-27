//go:build tier0

// moverecord_test.go covers the move record: its read and removal, and the way forward MoveInProgress names for a standing record.
//
// Package fabricengine_test.

package fabricengine_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/proc"
)

func TestMoveRecord_ReadAndRemove(t *testing.T) {
	t.Parallel()
	weft := filepath.Join(t.TempDir(), "beam-weft")

	if _, found, err := fabricengine.ReadMoveRecord(weft); err != nil || found {
		t.Fatalf("ReadMoveRecord(no record) = (found %v, %v); want not found", found, err)
	}
	if _, err := os.Stat(fabricengine.RecordsLockDirPath(weft)); !os.IsNotExist(err) {
		t.Errorf("reading an absent record created the lock directory (stat error %v); want nothing created", err)
	}

	want := fabricengine.MoveRecord{A: "a", B: "b", Kind: fabricengine.MoveKindIntegrate, U: "abc123", PID: 42}
	writeMoveRecord(t, weft, want)
	got, found, err := fabricengine.ReadMoveRecord(weft)
	if err != nil || !found || got != want {
		t.Errorf("ReadMoveRecord = (%+v, %v, %v); want (%+v, true, nil)", got, found, err, want)
	}

	if err := fabricengine.RemoveMoveRecord(weft); err != nil {
		t.Fatalf("RemoveMoveRecord: %v", err)
	}
	if _, found, err := fabricengine.ReadMoveRecord(weft); err != nil || found {
		t.Errorf("ReadMoveRecord after removal = (found %v, %v); want not found", found, err)
	}
	if err := fabricengine.RemoveMoveRecord(weft); err != nil {
		t.Errorf("RemoveMoveRecord of an absent record = %v; want nil", err)
	}
}

func TestMoveInProgress(t *testing.T) {
	t.Parallel()
	const slug = "beam"
	staleRecord := fabricengine.MoveRecord{A: "a", B: "b", Kind: fabricengine.MoveKindMove, PID: 4242}
	ownStartTime, _ := proc.StartTime(os.Getpid())
	liveRecord := fabricengine.MoveRecord{A: "a", B: "b", Kind: fabricengine.MoveKindMove, PID: os.Getpid(), StartTime: ownStartTime}

	tests := []struct {
		name         string
		record       *fabricengine.MoveRecord
		holdLock     bool
		wantStanding bool
		want         []string
	}{
		{"NoRecord", nil, false, false, nil},
		{"LockFree", &staleRecord, false, true, []string{"interrupted", "`lym fabric anchor beam --set b`"}},
		{"LockHeldByLiveRecordProcess", &liveRecord, true, true, []string{"in progress", fmt.Sprint(os.Getpid()), "wait for it", "`lym fabric anchor beam --set b`"}},
		{"LockHeldOverStaleRecord", &staleRecord, true, true, []string{"starting in another process", "wait for it", "run the verb again"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			weft := filepath.Join(t.TempDir(), slug+"-weft")
			if tt.record != nil {
				writeMoveRecord(t, weft, *tt.record)
			}
			if tt.holdLock {
				holdMoveLock(t, weft)
			}

			wayForward, standing, err := fabricengine.MoveInProgress(slug, weft)
			if err != nil || standing != tt.wantStanding {
				t.Fatalf("MoveInProgress = (%q, %v, %v); want standing %v", wayForward, standing, err, tt.wantStanding)
			}
			if !tt.wantStanding && wayForward != "" {
				t.Errorf("MoveInProgress way forward = %q; want none without a record", wayForward)
			}
			for _, want := range tt.want {
				if !strings.Contains(wayForward, want) {
					t.Errorf("MoveInProgress way forward = %q; want it to contain %q", wayForward, want)
				}
			}
		})
	}
}
