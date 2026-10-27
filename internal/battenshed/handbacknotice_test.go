//go:build tier0

// handbacknotice_test.go pins HandBackNotice's and RowStopNotice's one-notice-per-stop rule against the hand-back marker.

package battenshed

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// TestHandBackNotice asserts a hand-back notice is queued once per stop, and skipped when an earlier hand-back of the same stop did.
// It is queued when the marker only holds an earlier stop, held back without a recorded orch strand so a later call can still send, and never sent without a Notify.
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
		{name: "a hand-back marker of an earlier stop still queues the hand-back", marker: "handback 2\n", recorded: true, calls: 1, wantNotifies: 1, wantMarker: []string{"handback 5"}},
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

// TestRowStopNotice asserts the row-stop notice names the row, the one-line reason, the report and the way forward, is posted once per stop, again for a later stop, and never without a Notify.
func TestRowStopNotice(t *testing.T) {
	t.Parallel()

	const reportPath = "/reports/stop.md"
	newDeps := func(lines *[]string) RowNoticeDeps {
		return RowNoticeDeps{
			Notify: func(_ context.Context, line string) (bool, error) {
				*lines = append(*lines, line)
				return true, nil
			},
			OrchStrandRecorded: func() (bool, error) { return true, nil },
		}
	}

	t.Run("a first hand-back posts the line with the report path", func(t *testing.T) {
		t.Parallel()

		var lines []string
		if !RowStopNotice(context.Background(), newDeps(&lines), "a-slug", t.TempDir(), "Land", "pull failed:\nboom", reportPath, 5) {
			t.Fatal("RowStopNotice() = false; want it posted")
		}
		want := noticePrefix + " a-slug: Land failed: pull failed: boom; report " + reportPath + "; way forward: fix the cause, then run \"lym batten run a-slug\""
		if !slices.Equal(lines, []string{want}) {
			t.Errorf("notices = %q; want %q", lines, []string{want})
		}
	})

	t.Run("a second call for the same stop posts nothing and a later stop posts again", func(t *testing.T) {
		t.Parallel()

		var lines []string
		deps, scratchDir := newDeps(&lines), t.TempDir()
		RowStopNotice(context.Background(), deps, "a-slug", scratchDir, "Land", "boom", reportPath, 5)
		if RowStopNotice(context.Background(), deps, "a-slug", scratchDir, "Land", "boom", reportPath, 5) || len(lines) != 1 {
			t.Errorf("a repeated stop posted again; notices = %q", lines)
		}
		if !RowStopNotice(context.Background(), deps, "a-slug", scratchDir, "Land", "boom", reportPath, 7) || len(lines) != 2 {
			t.Errorf("a later stop was not posted; notices = %q", lines)
		}
	})

	t.Run("a nil Notify posts nothing", func(t *testing.T) {
		t.Parallel()

		if RowStopNotice(context.Background(), RowNoticeDeps{}, "a-slug", t.TempDir(), "Land", "boom", reportPath, 5) {
			t.Error("RowStopNotice() = true with a nil Notify; want false")
		}
	})
}
