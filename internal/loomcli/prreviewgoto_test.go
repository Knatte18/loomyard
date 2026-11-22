// prreviewgoto_test.go — tier 0 tests for the goto hook that clears a PR-Review review's recorded autonomy and session.

package loomcli

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/pullrequest"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestPRReviewEntryReset(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	errReset := errors.New("reset failed")
	tests := []struct {
		name       string
		target     string
		resetErr   error
		wantResets int
		wantErr    error
	}{
		{name: "PR-Review runs the reset once", target: loomshed.NamePRReview, wantResets: 1},
		{name: "PR-Review returns the reset's error", target: loomshed.NamePRReview, resetErr: errReset, wantResets: 1, wantErr: errReset},
		{name: "another target runs no reset", target: loomshed.NameFinalize},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			resets := 0
			hook := prReviewEntryReset(func() error { resets++; return tt.resetErr })
			if err := hook(tt.target); !errors.Is(err, tt.wantErr) {
				t.Errorf("hook(%q) = %v; want %v", tt.target, err, tt.wantErr)
			}
			if resets != tt.wantResets {
				t.Errorf("reset ran %d times; want %d", resets, tt.wantResets)
			}
		})
	}
}

func TestClearPRReviewOnGoto(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	autonomous := true
	full := pullrequest.Record{Number: 7, GatedHead: "h1", Autonomy: &autonomous, StrandGUID: "guid-1", StrandName: "strand", SessionID: "session", Engine: "claude"}
	errStop := errors.New("stop failed")
	errCommit := errors.New("commit failed")
	tests := []struct {
		name       string
		record     *pullrequest.Record
		stopErr    error
		commitErr  error
		wantErr    error
		wantStops  []string
		wantCommit int
		// wantRecord is the record expected afterwards.
		wantRecord *pullrequest.Record
	}{
		{name: "a record with fields loses them and keeps the gated head", record: &full, wantStops: []string{"guid-1"}, wantCommit: 1,
			wantRecord: &pullrequest.Record{Number: 7, GatedHead: "h1"}},
		{name: "a record naming no strand runs no stop", record: &pullrequest.Record{Number: 7, Autonomy: &autonomous}, wantCommit: 1,
			wantRecord: &pullrequest.Record{Number: 7}},
		{name: "a stop error clears nothing and commits nothing", record: &full, stopErr: errStop, wantErr: errStop, wantStops: []string{"guid-1"}, wantRecord: &full},
		{name: "a missing record commits nothing"},
		{name: "a commit error is returned", record: &full, commitErr: errCommit, wantErr: errCommit, wantStops: []string{"guid-1"}, wantCommit: 1,
			wantRecord: &pullrequest.Record{Number: 7, GatedHead: "h1"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			record, lock := filepath.Join(dir, "pull-request.json"), filepath.Join(dir, "pull-request.json.lock")
			if tt.record != nil {
				if err := pullrequest.WriteRecord(record, lock, *tt.record); err != nil {
					t.Fatalf("WriteRecord() error: %v", err)
				}
			}
			var stops []string
			commits := 0
			err := clearPRReviewOnGoto(record, lock,
				func(guid string) error { stops = append(stops, guid); return tt.stopErr },
				func() error { commits++; return tt.commitErr })
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("clearPRReviewOnGoto() = %v; want %v", err, tt.wantErr)
			}
			if tt.wantErr != nil && !strings.Contains(err.Error(), "lym loom goto --to "+loomshed.NamePRReview) {
				t.Errorf("clearPRReviewOnGoto() = %v; want it to name the goto that repeats the clear", err)
			}
			if strings.Join(stops, ",") != strings.Join(tt.wantStops, ",") {
				t.Errorf("stopped strands = %v; want %v", stops, tt.wantStops)
			}
			if commits != tt.wantCommit {
				t.Errorf("commit ran %d times; want %d", commits, tt.wantCommit)
			}
			got, found, readErr := pullrequest.ReadRecord(record, lock)
			if readErr != nil {
				t.Fatalf("ReadRecord() error: %v", readErr)
			}
			if tt.wantRecord == nil {
				if found {
					t.Errorf("record = %+v; want none", got)
				}
				return
			}
			if !found || got.GatedHead != tt.wantRecord.GatedHead || got.StrandGUID != tt.wantRecord.StrandGUID || got.Autonomy != nil != (tt.wantRecord.Autonomy != nil) {
				t.Errorf("record = %+v; want %+v", got, *tt.wantRecord)
			}
		})
	}
}
