// rownotice_test.go covers the notice log append, the row notifier's delivery bound and the per-run marker reset.

package battenshed

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/testkit/logcapture"
)

// noticeLogClock is the fixed time every test notice log stamps its lines with.
var noticeLogClock = time.Date(2026, 3, 4, 5, 6, 7, 0, time.FixedZone("plus-two", 2*60*60))

// fixedNoticeLog returns a NoticeLog over path whose lines carry noticeLogClock and the attach command `attach-it`.
func fixedNoticeLog(path string) NoticeLog {
	return NoticeLog{Path: path, AttachCommand: "attach-it", Now: func() time.Time { return noticeLogClock }}
}

// readNoticeLogLines returns the lines of the notice log at path, none when it does not exist.
func readNoticeLogLines(t *testing.T, path string) []string {
	t.Helper()
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
}

// unwritableNoticeLogPath returns a log path below a regular file, which can be neither opened nor created.
func unwritableNoticeLogPath(t *testing.T) string {
	t.Helper()
	file := filepath.Join(t.TempDir(), "plain-file")
	if err := os.WriteFile(file, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	return filepath.Join(file, "notices.log")
}

// TestAppendNoticeLog asserts a line carries the log's UTC time, the notice text and the attach command, tab-separated and newline-ended;
// concurrent appends leave whole lines, an unwritable path warns and writes nothing, and an empty path switches the log off.
// It stays serial: logcapture.Capture replaces the process-global logger output.
func TestAppendNoticeLog(t *testing.T) {
	t.Run("a line holds the UTC time, the text and the attach command", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "notices.log")
		appendNoticeLog(fixedNoticeLog(path), "batten: task blocked")

		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if want := "2026-03-04T03:06:07Z\tbatten: task blocked\tattach-it\n"; string(data) != want {
			t.Errorf("notice log = %q; want %q", data, want)
		}
	})

	t.Run("two goroutines appending at once leave two whole lines", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "notices.log")
		var wg sync.WaitGroup
		for _, text := range []string{strings.Repeat("a", 5000), strings.Repeat("b", 5000)} {
			wg.Add(1)
			go func() {
				defer wg.Done()
				appendNoticeLog(fixedNoticeLog(path), text)
			}()
		}
		wg.Wait()

		lines := readNoticeLogLines(t, path)
		slices.Sort(lines)
		want := []string{
			"2026-03-04T03:06:07Z\t" + strings.Repeat("a", 5000) + "\tattach-it",
			"2026-03-04T03:06:07Z\t" + strings.Repeat("b", 5000) + "\tattach-it",
		}
		if !slices.Equal(lines, want) {
			t.Errorf("notice log holds %d lines, not the two whole ones written", len(lines))
		}
	})

	t.Run("an unwritable path warns, writes nothing and returns", func(t *testing.T) {
		logs := logcapture.Capture(t)
		path := unwritableNoticeLogPath(t)
		appendNoticeLog(fixedNoticeLog(path), "batten: task blocked")

		if !strings.Contains(logs.String(), path) {
			t.Errorf("log %q does not name the unwritable path %q", logs.String(), path)
		}
	})

	t.Run("an empty path writes nothing", func(t *testing.T) {
		dir := t.TempDir()
		t.Chdir(dir)
		appendNoticeLog(fixedNoticeLog(""), "batten: task blocked")

		if entries, err := os.ReadDir(dir); err != nil || len(entries) != 0 {
			t.Errorf("working directory holds %v (read error %v); want nothing written", entries, err)
		}
	})
}

// TestRowNotifierSend_DeliveryBound asserts send skips Notify when no orch strand is recorded, retries an unqueued line with the retry spacing up to noticeMaxAttempts and then drops it, and stops retrying once the line is queued.
// Every send appends exactly one line to the notice log, whatever its delivery, and a log that cannot be written changes nothing about the delivery.
func TestRowNotifierSend_DeliveryBound(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		recorded      bool
		queuedOnCall  int
		unwritableLog bool
		wantQueued    bool
		wantNotifies  int
		wantSleeps    int
	}{
		{name: "NoStrandRecordedSkipsNotify", recorded: false, wantQueued: false, wantNotifies: 0, wantSleeps: 0},
		{name: "QueuedFirstTry", recorded: true, queuedOnCall: 1, wantQueued: true, wantNotifies: 1, wantSleeps: 0},
		{name: "QueuedOnLastAttempt", recorded: true, queuedOnCall: noticeMaxAttempts, wantQueued: true, wantNotifies: noticeMaxAttempts, wantSleeps: noticeMaxAttempts - 1},
		{name: "NeverQueuedIsDropped", recorded: true, queuedOnCall: 0, wantQueued: false, wantNotifies: noticeMaxAttempts, wantSleeps: noticeMaxAttempts - 1},
		{name: "UnwritableLogStillQueues", recorded: true, queuedOnCall: 1, unwritableLog: true, wantQueued: true, wantNotifies: 1, wantSleeps: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			logPath := filepath.Join(t.TempDir(), "notices.log")
			if tt.unwritableLog {
				logPath = unwritableNoticeLogPath(t)
			}
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
				NoticeLog:          fixedNoticeLog(logPath),
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
			if !tt.unwritableLog {
				wantLines := []string{"2026-03-04T03:06:07Z\tline\tattach-it"}
				if lines := readNoticeLogLines(t, logPath); !slices.Equal(lines, wantLines) {
					t.Errorf("notice log lines = %q; want %q", lines, wantLines)
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
