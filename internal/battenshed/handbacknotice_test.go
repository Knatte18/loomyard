// handbacknotice_test.go pins HandBackNotice's one-notice-per-stop-episode rule against the markers the rows leave.

package battenshed

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// TestHandBackNotice asserts a hand-back notice is queued once per stop, and skipped when the row already announced the same stop or an earlier hand-back did.
// It is queued when the marker only holds an earlier episode, held back without a recorded orch strand so a later call can still send, and never sent without a Notify.
func TestHandBackNotice(t *testing.T) {
	t.Parallel()
	const historyLen = 5

	tests := []struct {
		name         string
		marker       string
		recorded     bool
		nilNotify    bool
		calls        int
		wantNotifies int
		wantMarker   []string
	}{
		{name: "a stop with no marker queues once and records the hand-back", recorded: true, calls: 2, wantNotifies: 1, wantMarker: []string{"handback 5"}},
		{name: "the row's own notice of this stop queues nothing", marker: "failure 4\n", recorded: true, calls: 1, wantNotifies: 0, wantMarker: []string{"failure 4"}},
		{name: "a row notice of an earlier episode still queues the hand-back", marker: "failure 2\n", recorded: true, calls: 1, wantNotifies: 1, wantMarker: []string{"failure 2", "handback 5"}},
		{name: "no recorded orch strand queues nothing and writes no hand-back line", recorded: false, calls: 1, wantNotifies: 0, wantMarker: nil},
		{name: "a nil Notify queues nothing", recorded: true, nilNotify: true, calls: 1, wantNotifies: 0, wantMarker: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			scratchDir := t.TempDir()
			markerPath := filepath.Join(scratchDir, "Run-Shed"+rowNoticeFileSuffix)
			if tt.marker != "" {
				if err := os.WriteFile(markerPath, []byte(tt.marker), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			notifies := 0
			deps := RowNoticeDeps{
				Notify: func(context.Context, string) (bool, error) {
					notifies++
					return true, nil
				},
				OrchStrandRecorded: func() (bool, error) { return tt.recorded, nil },
			}
			if tt.nilNotify {
				deps.Notify = nil
			}

			for range tt.calls {
				HandBackNotice(context.Background(), deps, "a-slug", scratchDir, "Run-Shed", historyLen, "line")
			}
			if notifies != tt.wantNotifies {
				t.Errorf("Notify calls = %d; want %d", notifies, tt.wantNotifies)
			}
			var gotMarker []string
			if data, err := os.ReadFile(markerPath); err == nil {
				gotMarker = strings.Split(strings.TrimSpace(string(data)), "\n")
			}
			if !slices.Equal(gotMarker, tt.wantMarker) {
				t.Errorf("marker lines = %q; want %q", gotMarker, tt.wantMarker)
			}
		})
	}

	t.Run("a call after the orch strand is recorded still sends", func(t *testing.T) {
		t.Parallel()

		recorded := false
		notifies := 0
		deps := RowNoticeDeps{
			Notify: func(context.Context, string) (bool, error) {
				notifies++
				return true, nil
			},
			OrchStrandRecorded: func() (bool, error) { return recorded, nil },
		}
		scratchDir := t.TempDir()
		if HandBackNotice(context.Background(), deps, "a-slug", scratchDir, "Run-Shed", historyLen, "line") {
			t.Fatal("HandBackNotice() = true with no orch strand recorded; want false")
		}
		recorded = true
		if !HandBackNotice(context.Background(), deps, "a-slug", scratchDir, "Run-Shed", historyLen, "line") || notifies != 1 {
			t.Errorf("HandBackNotice() after the strand was recorded queued %d lines; want it to send once", notifies)
		}
	})
}
