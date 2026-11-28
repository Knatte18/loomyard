// parentseatwiring_test.go covers the review segments' parent seat wiring:
// the ask setting, the decision record's write time and each seat's placement on its own segment.

package loomcli

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/shedadapters"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestAskParentSetting_ReadsTheBooleanSetting proves askParentSetting asks loom for the key it is told.
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

			got := askParentSetting(lookup, codeAskParentKey)

			if got != tt.want {
				t.Errorf("askParentSetting() = %v; want %v", got, tt.want)
			}
			if gotModule != "loom" || gotKey != "code_review_ask_parent" {
				t.Errorf("lookup called with (%q, %q); want (%q, %q)", gotModule, gotKey, "loom", "code_review_ask_parent")
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

// TestLoomSegmentSettings_ParentSeatsLandOnTheirSegments proves where the seats passed in land.
// The discussion seat is the Discussion-Review segment's Parent and the code seat the Code-Review segment's;
// no other segment carries one.
//
//testtiming:keep pins which seat lands on which segment, where the wire test checks only presence
func TestLoomSegmentSettings_ParentSeatsLandOnTheirSegments(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	discussion := &shedadapters.ParentReviewSeat{Slug: "a-slug", Tag: shedadapters.DiscussionParentAskLineTag}
	code := &shedadapters.ParentReviewSeat{Slug: "a-slug", Tag: shedadapters.CodeParentAskLineTag}
	seats := map[loomengine.Perch]*shedadapters.ParentReviewSeat{loomengine.PerchDiscussion: discussion, loomengine.PerchCode: code}
	wantSeats := map[string]*shedadapters.ParentReviewSeat{"Discussion-Review": discussion, "Code-Review": code}

	settings := loomSegmentSettings(loomengine.Config{}, loomengine.ReviewSettings{}, loomengine.JudgeSettings{}, nil, seats)

	if len(settings) != len(loomSegmentPerches) {
		t.Fatalf("loomSegmentSettings() holds %d segments; want %d", len(settings), len(loomSegmentPerches))
	}
	for segment := range wantSeats {
		if _, ok := settings[segment]; !ok {
			t.Errorf("loomSegmentSettings() holds no %q segment", segment)
		}
	}
	for segment, got := range settings {
		if want := wantSeats[segment]; got.Parent != want {
			t.Errorf("segment %q Parent = %p; want %p", segment, got.Parent, want)
		}
	}
}

// TestParentSeats_BuildEachSeatFromItsOwnPerch proves what each production seat is built with.
// Its tag, subject, whether it asks before the judge, whether it reads the history for a rewrite of its subject,
// and the setting its AskEnabled reads.
func TestParentSeats_BuildEachSeatFromItsOwnPerch(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	location := &lymcwd.Location{HubPath: filepath.Join(t.TempDir(), "hub"), WorktreeName: "pair", AnchorRel: "."}
	tests := []struct {
		name               string
		perch              loomengine.Perch
		tag                string
		subject            string
		askBeforeJudge     bool
		hasRecordWrittenAt bool
		key                string
		wantWait           time.Duration
	}{
		{
			name: "Discussion", perch: loomengine.PerchDiscussion, tag: shedadapters.DiscussionParentAskLineTag,
			subject:            "the decision record " + loomengine.DiscussionDecisionRecord(location),
			hasRecordWrittenAt: true, key: "discussion_review_ask_parent", wantWait: 30 * time.Minute,
		},
		{
			name: "Code", perch: loomengine.PerchCode, tag: shedadapters.CodeParentAskLineTag,
			subject:        "the task's code change in the task worktree " + location.WorktreePath(),
			askBeforeJudge: true, key: "code_review_ask_parent", wantWait: 23 * time.Minute,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var gotModule, gotKey string
			seats := parentSeats(parentSeatInputs{
				location:       location,
				cfg:            loomengine.Config{ReviewTimeoutMin: 240, DiscussionReviewParentTimeoutMin: 30, CodeReviewParentTimeoutMin: 30},
				runTimeout:     30 * time.Minute,
				startAllowance: time.Minute,
				awaitCap:       func(loomengine.Perch) time.Duration { return time.Minute },
				setting: func(module, key string) (any, error) {
					gotModule, gotKey = module, key
					return true, nil
				},
			})

			seat := seats[tt.perch]
			if seat == nil {
				t.Fatalf("parentSeats() holds no seat for the %s perch", tt.perch)
			}
			if seat.Tag != tt.tag || seat.Subject != tt.subject || seat.AskBeforeJudge != tt.askBeforeJudge || (seat.RecordWrittenAt != nil) != tt.hasRecordWrittenAt {
				t.Errorf("seat = tag %q, subject %q, AskBeforeJudge %v, RecordWrittenAt set %v; want %q, %q, %v, %v",
					seat.Tag, seat.Subject, seat.AskBeforeJudge, seat.RecordWrittenAt != nil, tt.tag, tt.subject, tt.askBeforeJudge, tt.hasRecordWrittenAt)
			}
			if !seat.AskEnabled() || gotModule != "loom" || gotKey != tt.key {
				t.Errorf("AskEnabled() read (%q, %q); want (%q, %q)", gotModule, gotKey, "loom", tt.key)
			}
			if wait, reason := seat.Wait(); wait != tt.wantWait || reason != "" {
				t.Errorf("Wait() = %s, %q; want %s with no reason", wait, reason, tt.wantWait)
			}
		})
	}
}
