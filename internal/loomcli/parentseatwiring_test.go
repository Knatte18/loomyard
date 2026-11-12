// parentseatwiring_test.go covers the Discussion-Review parent seat's wiring:
// the ask setting, the decision record's write time and the seat's placement on the discussion segment alone.

package loomcli

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/shedadapters"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestAskParentSetting_ReadsTheBooleanSetting proves askParentSetting asks for loom's discussion_review_ask_parent.
// It answers true only for a boolean true; an error or a non-boolean reads as false.
func TestAskParentSetting_ReadsTheBooleanSetting(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name  string
		value any
		err   error
		want  bool
	}{
		{name: "True", value: true, want: true},
		{name: "False", value: false, want: false},
		{name: "LookupError", err: errors.New("unreadable"), want: false},
		{name: "NonBoolean", value: "true", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var gotModule, gotKey string
			lookup := func(module, key string) (any, error) {
				gotModule, gotKey = module, key
				return tt.value, tt.err
			}

			got := askParentSetting(lookup)

			if got != tt.want {
				t.Errorf("askParentSetting() = %v; want %v", got, tt.want)
			}
			if gotModule != "loom" || gotKey != "discussion_review_ask_parent" {
				t.Errorf("lookup called with (%q, %q); want (%q, %q)", gotModule, gotKey, "loom", "discussion_review_ask_parent")
			}
		})
	}
}

// TestLatestCallAt_ReadsTheLatestProducerStamp proves latestCallAt answers the zero time for an absent status file.
// Otherwise it answers the latest At or LastAt among the producer's entries.
// It errors on a stamp that is not a time.
func TestLatestCallAt_ReadsTheLatestProducerStamp(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	const producer = loomshed.NameDiscussionWrite
	tests := []struct {
		name    string
		history []shedengine.HistoryEntry
		absent  bool
		want    time.Time
		wantErr bool
	}{
		{name: "AbsentStatusFile", absent: true},
		{
			name: "LatestAtOfTheProducersEntries",
			history: []shedengine.HistoryEntry{
				{Producer: producer, At: "2026-11-01T10:00:00Z"},
				{Producer: producer, At: "2026-11-01T12:00:00Z"},
			},
			want: time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC),
		},
		{
			name: "LastAtLaterThanAtWins",
			history: []shedengine.HistoryEntry{
				{Producer: producer, At: "2026-11-01T10:00:00Z", LastAt: "2026-11-01T14:00:00Z"},
			},
			want: time.Date(2026, 11, 1, 14, 0, 0, 0, time.UTC),
		},
		{
			name: "OtherProducersIgnored",
			history: []shedengine.HistoryEntry{
				{Producer: producer, At: "2026-11-01T10:00:00Z"},
				{Producer: loomshed.NameDarnWrite, At: "2026-11-02T10:00:00Z", LastAt: "2026-11-03T10:00:00Z"},
			},
			want: time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC),
		},
		{
			name:    "NoEntryOfTheProducer",
			history: []shedengine.HistoryEntry{{Producer: loomshed.NameDarnWrite, At: "2026-11-02T10:00:00Z"}},
		},
		{
			name:    "UnparseableTime",
			history: []shedengine.HistoryEntry{{Producer: producer, At: "yesterday"}},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			statusPath := filepath.Join(dir, "status.json")
			lockPath := filepath.Join(dir, "status.lock")
			if !tt.absent {
				writeStatusFixture(t, statusPath, lockPath, shedengine.Status{History: tt.history})
			}

			got, err := latestCallAt(statusPath, lockPath, producer)

			if (err != nil) != tt.wantErr {
				t.Fatalf("latestCallAt() error = %v; want error %v", err, tt.wantErr)
			}
			if !got.Equal(tt.want) {
				t.Errorf("latestCallAt() = %v; want %v", got, tt.want)
			}
		})
	}
}

// TestLoomSegmentSettings_ParentSeatLandsOnTheDiscussionSegmentOnly proves where the seat passed in lands.
// It is the Discussion-Review segment's Parent, and no other segment carries one.
func TestLoomSegmentSettings_ParentSeatLandsOnTheDiscussionSegmentOnly(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	seat := &shedadapters.ParentReviewSeat{Slug: "a-slug"}

	settings := loomSegmentSettings(loomengine.Config{}, loomengine.ReviewSettings{}, loomengine.JudgeSettings{}, nil, seat)

	if len(settings) != len(loomSegmentPerches) {
		t.Fatalf("loomSegmentSettings() holds %d segments; want %d", len(settings), len(loomSegmentPerches))
	}
	for segment, got := range settings {
		want := (*shedadapters.ParentReviewSeat)(nil)
		if segment == "Discussion-Review" {
			want = seat
		}
		if got.Parent != want {
			t.Errorf("segment %q Parent = %p; want %p", segment, got.Parent, want)
		}
	}
	if _, ok := settings["Discussion-Review"]; !ok {
		t.Error(`loomSegmentSettings() holds no "Discussion-Review" segment`)
	}
}
