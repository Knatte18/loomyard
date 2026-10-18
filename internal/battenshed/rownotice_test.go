// rownotice_test.go covers the row notifier's delivery bound and the per-run marker reset.

package battenshed

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestRowNotifierSend_DeliveryBound asserts send skips Notify when no orch strand is recorded, retries an unqueued line with the retry spacing up to noticeMaxAttempts and then drops it, and stops retrying once the line is queued.
func TestRowNotifierSend_DeliveryBound(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		recorded     bool
		queuedOnCall int
		wantQueued   bool
		wantNotifies int
		wantSleeps   int
	}{
		{name: "NoStrandRecordedSkipsNotify", recorded: false, wantQueued: false, wantNotifies: 0, wantSleeps: 0},
		{name: "QueuedFirstTry", recorded: true, queuedOnCall: 1, wantQueued: true, wantNotifies: 1, wantSleeps: 0},
		{name: "QueuedOnLastAttempt", recorded: true, queuedOnCall: noticeMaxAttempts, wantQueued: true, wantNotifies: noticeMaxAttempts, wantSleeps: noticeMaxAttempts - 1},
		{name: "NeverQueuedIsDropped", recorded: true, queuedOnCall: 0, wantQueued: false, wantNotifies: noticeMaxAttempts, wantSleeps: noticeMaxAttempts - 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			notifies := 0
			var slept []time.Duration
			notifier := newRowNotifier("Land", "a-slug", t.TempDir(), RowNoticeDeps{
				Notify: func(context.Context, string) (bool, error) {
					notifies++
					if notifies == tt.queuedOnCall {
						return true, nil
					}
					return false, errors.New("queue refused")
				},
				OrchStrandRecorded: func() (bool, error) { return tt.recorded, nil },
				Sleep:              func(_ context.Context, d time.Duration) { slept = append(slept, d) },
			})

			if got := notifier.send(context.Background(), "line"); got != tt.wantQueued {
				t.Errorf("send() = %v; want %v", got, tt.wantQueued)
			}
			if notifies != tt.wantNotifies {
				t.Errorf("Notify calls = %d; want %d", notifies, tt.wantNotifies)
			}
			if len(slept) != tt.wantSleeps {
				t.Fatalf("sleeps = %v; want %d of them", slept, tt.wantSleeps)
			}
			for _, d := range slept {
				if d != rowNoticeRetrySpacing {
					t.Errorf("sleep = %v; want %v", d, rowNoticeRetrySpacing)
				}
			}
		})
	}
}

// TestClearRunMarkers_RemovesEveryRunMarkerSuffixOnly asserts ClearRunMarkers removes a marker for every suffix in runMarkerSuffixes, leaves any other file, and accepts an absent directory.
func TestClearRunMarkers_RemovesEveryRunMarkerSuffixOnly(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	for _, suffix := range runMarkerSuffixes {
		if err := os.WriteFile(filepath.Join(dir, "Row"+suffix), []byte("x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	other := filepath.Join(dir, "Row-spawned")
	if err := os.WriteFile(other, []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := ClearRunMarkers(dir); err != nil {
		t.Fatalf("ClearRunMarkers() = %v; want nil", err)
	}
	for _, suffix := range runMarkerSuffixes {
		if _, err := os.Stat(filepath.Join(dir, "Row"+suffix)); !os.IsNotExist(err) {
			t.Errorf("marker for suffix %q stat error = %v; want it removed", suffix, err)
		}
	}
	if _, err := os.Stat(other); err != nil {
		t.Errorf("unrelated file stat error = %v; want it left alone", err)
	}
	if err := ClearRunMarkers(filepath.Join(dir, "absent")); err != nil {
		t.Errorf("ClearRunMarkers(absent dir) = %v; want nil", err)
	}
}
