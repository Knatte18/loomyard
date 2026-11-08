// channelmap_test.go covers the channel map file and the forward-only cursors in its entries.

package slackengine_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/Knatte18/loomyard/internal/slackengine"
	"github.com/Knatte18/loomyard/internal/state"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// timestampAt renders t as a Slack timestamp.
func timestampAt(t time.Time) string {
	return fmt.Sprintf("%d.%06d", t.Unix(), t.Nanosecond()/1000)
}

func TestChannelMap(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	mapPath := func(t *testing.T) string {
		t.Helper()
		return filepath.Join(t.TempDir(), "slack", slackengine.ChannelMapFileName)
	}

	t.Run("MissingFileReadsEmpty", func(t *testing.T) {
		t.Parallel()
		channelMap, err := slackengine.ReadChannelMap(mapPath(t))
		if err != nil || len(channelMap.Channels) != 0 {
			t.Errorf("ReadChannelMap(missing) = %+v, %v; want an empty map", channelMap, err)
		}
	})

	t.Run("EntryReadsBackAndAnUnknownKeySurvivesAnUpdate", func(t *testing.T) {
		t.Parallel()
		path := mapPath(t)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("MkdirAll = %v", err)
		}
		written := `{"channels":{"ly:orch":{"id":"C1","name":"ly-neve-prime","future_entry_key":"kept"}},"future_top_key":7}`
		if err := os.WriteFile(path, []byte(written), 0o644); err != nil {
			t.Fatalf("WriteFile = %v", err)
		}
		err := slackengine.UpdateChannelMap(path, func(channelMap *slackengine.ChannelMap) error {
			channelMap.Channels["ly:payments:orch"] = slackengine.ChannelEntry{ID: "C2", Name: "ly-neve-payments", Operator: "U1"}
			return nil
		})
		if err != nil {
			t.Fatalf("UpdateChannelMap = %v", err)
		}
		channelMap, err := slackengine.ReadChannelMap(path)
		if err != nil {
			t.Fatalf("ReadChannelMap = %v", err)
		}
		want := slackengine.ChannelMap{Channels: map[string]slackengine.ChannelEntry{
			"ly:orch":          {ID: "C1", Name: "ly-neve-prime"},
			"ly:payments:orch": {ID: "C2", Name: "ly-neve-payments", Operator: "U1"},
		}}
		if diff := cmp.Diff(want, channelMap); diff != "" {
			t.Errorf("ReadChannelMap() mismatch (-want +got):\n%s", diff)
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("ReadFile = %v", err)
		}
		for _, key := range []string{"future_entry_key", "future_top_key"} {
			if !strings.Contains(string(raw), key) {
				t.Errorf("channel map file lost the key %s: %s", key, raw)
			}
		}
		if orch, ok := channelMap.OrchOf("C2"); !ok || orch != "ly:payments:orch" {
			t.Errorf("OrchOf(C2) = %q, %v; want ly:payments:orch", orch, ok)
		}
		if orch, ok := channelMap.OrchOf("C9"); ok {
			t.Errorf("OrchOf(C9) = %q, true; want not found", orch)
		}
	})

	t.Run("ConcurrentAdvancesLeaveTheNewestCursor", func(t *testing.T) {
		t.Parallel()
		path := mapPath(t)
		const writers = 20
		var wg sync.WaitGroup
		errs := make(chan error, writers)
		for i := range writers {
			wg.Add(1)
			go func() {
				defer wg.Done()
				errs <- slackengine.UpdateChannelMap(path, func(channelMap *slackengine.ChannelMap) error {
					entry := channelMap.Channels["ly:orch"]
					entry.Advance(fmt.Sprintf("1700000000.%06d", i+1))
					channelMap.Channels["ly:orch"] = entry
					return nil
				})
			}()
		}
		wg.Wait()
		close(errs)
		for err := range errs {
			if err != nil {
				t.Fatalf("UpdateChannelMap = %v", err)
			}
		}
		channelMap, err := slackengine.ReadChannelMap(path)
		if err != nil {
			t.Fatalf("ReadChannelMap = %v", err)
		}
		if got, want := channelMap.Channels["ly:orch"].Cursor, fmt.Sprintf("1700000000.%06d", writers); got != want {
			t.Errorf("cursor = %q; want the newest, %q", got, want)
		}
	})

	t.Run("ThreadCursorsMoveForwardOnly", func(t *testing.T) {
		t.Parallel()
		var entry slackengine.ChannelEntry
		entry.AdvanceThread("100.000000", "200.000000")
		entry.AdvanceThread("100.000000", "150.000000")
		if got := entry.Threads["100.000000"]; got != "200.000000" {
			t.Errorf("thread cursor after an older AdvanceThread = %q; want 200.000000", got)
		}
		entry.Advance("300.000000")
		entry.Advance("250.000000")
		if entry.Cursor != "300.000000" {
			t.Errorf("cursor after an older Advance = %q; want 300.000000", entry.Cursor)
		}
	})

	t.Run("PruneDropsOldThreadsAndRaisesTheFloor", func(t *testing.T) {
		t.Parallel()
		now := time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)
		old := timestampAt(now.Add(-25 * time.Hour))
		recent := timestampAt(now.Add(-time.Hour))
		entry := slackengine.ChannelEntry{Threads: map[string]string{"1.000000": old, "2.000000": recent}}
		entry.PruneThreads(now)
		if want := map[string]string{"2.000000": recent}; !cmp.Equal(entry.Threads, want) {
			t.Errorf("Threads after the prune = %v; want %v", entry.Threads, want)
		}
		if want := timestampAt(now.Add(-24 * time.Hour)); entry.ThreadFloor != want {
			t.Errorf("ThreadFloor after the prune = %q; want %q", entry.ThreadFloor, want)
		}
	})

	t.Run("ThreadStartIsTheCursorOrTheLaterOfTheThreadAndTheFloor", func(t *testing.T) {
		t.Parallel()
		entry := slackengine.ChannelEntry{ThreadFloor: "500.000000", Threads: map[string]string{"100.000000": "900.000000"}}
		tests := []struct {
			name   string
			thread string
			want   string
		}{
			{"CursorWins", "100.000000", "900.000000"},
			{"FloorWinsOverAnOlderThread", "200.000000", "500.000000"},
			{"ThreadWinsOverAnOlderFloor", "700.000000", "700.000000"},
		}
		for _, tt := range tests {
			if got := entry.ThreadStart(tt.thread); got != tt.want {
				t.Errorf("%s: ThreadStart(%s) = %q; want %q", tt.name, tt.thread, got, tt.want)
			}
		}
	})

	t.Run("UndecodableFileIsAnErrorNamingItAndMovingItAside", func(t *testing.T) {
		t.Parallel()
		path := mapPath(t)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("MkdirAll = %v", err)
		}
		if err := os.WriteFile(path, []byte("{"), 0o644); err != nil {
			t.Fatalf("WriteFile = %v", err)
		}
		_, err := slackengine.ReadChannelMap(path)
		if err == nil || !errors.Is(err, state.ErrDecode) || !strings.Contains(err.Error(), path) ||
			!strings.Contains(err.Error(), "move the file aside") {
			t.Errorf("ReadChannelMap({) error = %v; want one wrapping state.ErrDecode that names the path and moving it aside", err)
		}
	})
}
