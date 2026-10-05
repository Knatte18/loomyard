// handoffvoucher_test.go is the untagged suite for the handoff voucher (crucible round 2,
// R2-F1): recordHandoffVoucher and consumeHandoffVoucher directly, and observeEntry's consumption of
// the voucher into EntryObservation.Vouched. Everything runs against t.TempDir() paths via
// the production state primitives -- no tmux, no git, no real run.

package loomcli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// handoffVoucherPaths returns a voucher path and lock path rooted in a fresh temp dir.
func handoffVoucherPaths(t *testing.T) (string, string) {
	t.Helper()
	dir := t.TempDir()
	return filepath.Join(dir, "handoff-voucher.json"), filepath.Join(dir, "handoff-voucher.json.lock")
}

func TestConsumeHandoffVoucher(t *testing.T) {
	tests := []struct {
		name            string
		record          bool
		recordedHistory int
		recordedState   shedengine.State
		observedHistory int
		observedState   shedengine.State
		want            bool
	}{
		{
			name:   "Match_Suppresses",
			record: true, recordedHistory: 3, recordedState: shedengine.StateRunning,
			observedHistory: 3, observedState: shedengine.StateRunning,
			want: true,
		},
		{
			// A driver that resumed from the handoff and appended history before dying must be
			// reported as a crash, so a grown history mismatches.
			name:   "HistoryGrew_NoMatch",
			record: true, recordedHistory: 3, recordedState: shedengine.StateRunning,
			observedHistory: 4, observedState: shedengine.StateRunning,
			want: false,
		},
		{
			name:   "StateDiffers_NoMatch",
			record: true, recordedHistory: 3, recordedState: shedengine.StateRunning,
			observedHistory: 3, observedState: shedengine.StateBlocked,
			want: false,
		},
		{
			name:            "AbsentMarker_NoMatch",
			record:          false,
			observedHistory: 3, observedState: shedengine.StateRunning,
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			voucherPath, lockPath := handoffVoucherPaths(t)
			if tt.record {
				recordHandoffVoucher(voucherPath, lockPath, tt.recordedHistory, tt.recordedState)
				if _, err := os.Stat(voucherPath); err != nil {
					t.Fatalf("recordHandoffVoucher left no voucher at %s: %v", voucherPath, err)
				}
			}

			got := consumeHandoffVoucher(voucherPath, lockPath, tt.observedHistory, tt.observedState)
			if got != tt.want {
				t.Errorf("consumeHandoffVoucher(recorded %v/%d/%s, observed %d/%s) = %v; want %v",
					tt.record, tt.recordedHistory, tt.recordedState, tt.observedHistory, tt.observedState, got, tt.want)
			}

			// The voucher is one-shot: consumed (deleted) on every read, match or not,
			// so a driver crash after a consumed handoff is never suppressed by a stale voucher.
			if tt.record {
				if _, err := os.Stat(voucherPath); !os.IsNotExist(err) {
					t.Errorf("voucher still present after consume (stat err=%v); want it deleted", err)
				}
			}
		})
	}
}

// TestObserveEntry_ConsumesHandoffVoucherIntoObservation asserts the wiring: a voucher matching the
// persisted status yields Vouched true, and -- the one-shot property -- a second identical
// observation yields false, because the first consumed the voucher.
func TestObserveEntry_ConsumesHandoffVoucherIntoObservation(t *testing.T) {
	dir := t.TempDir()
	statusPath := filepath.Join(dir, "status.json")
	statusLockPath := filepath.Join(dir, "status.json.lock")
	runLockPath := filepath.Join(dir, "run.lock")
	voucherPath := filepath.Join(dir, "handoff-voucher.json")
	voucherLockPath := filepath.Join(dir, "handoff-voucher.json.lock")

	writeStatusFixture(t, statusPath, statusLockPath, shedengine.Status{
		CurrentProducer: "Plan-Write",
		State:           shedengine.StateRunning,
		History: []shedengine.HistoryEntry{
			{Producer: "Preflight", Outcome: shedengine.Done, At: "2026-01-01T00:00:00Z"},
			{Producer: "Loom-Preflight", Outcome: shedengine.Done, At: "2026-01-01T00:00:01Z"},
		},
		Product: productJSON(t, loomengine.Status{Slug: "a-task", Parent: "main"}),
	})
	recordHandoffVoucher(voucherPath, voucherLockPath, 2, shedengine.StateRunning)

	first := observeEntry(true, runLockPath, statusPath, statusLockPath, voucherPath, voucherLockPath)
	if !first.Observed {
		t.Fatalf("observeEntry first call: Observed = false; want true")
	}
	if !first.Vouched {
		t.Errorf("observeEntry first call: Vouched = false; want true -- a matching voucher must suppress the crash signature")
	}

	second := observeEntry(true, runLockPath, statusPath, statusLockPath, voucherPath, voucherLockPath)
	if second.Vouched {
		t.Errorf("observeEntry second call: Vouched = true; want false -- the voucher is one-shot and the first call consumed it")
	}
}
