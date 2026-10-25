//go:build tier0

// archivedrecords_test.go tests the decoders of archived parent-review round files and alignment attempt files.

package reportengine

import (
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

func TestParentReviewRoundNumber(t *testing.T) {
	t.Parallel()
	tests := []struct {
		dirName string
		want    int
		wantOK  bool
	}{
		{"round-1", 1, true},
		{"round-12", 12, true},
		{"round-0", 0, false},
		{"round-", 0, false},
		{"round-1a", 0, false},
		{"round--1", 0, false},
		{"attempt-1", 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.dirName, func(t *testing.T) {
			t.Parallel()
			got, ok := parentReviewRoundNumber(tt.dirName)
			if got != tt.want || ok != tt.wantOK {
				t.Errorf("parentReviewRoundNumber(%q) = %d, %v; want %d, %v", tt.dirName, got, ok, tt.want, tt.wantOK)
			}
		})
	}
}

func TestParseParentReviewRound(t *testing.T) {
	t.Parallel()
	opened := time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC)
	delivered := opened.Add(time.Hour)
	lastPrompt := delivered.Add(time.Minute)
	lastPing := delivered.Add(2 * time.Minute)
	recorded := delivered.Add(time.Hour)
	spawned := delivered.Add(3 * time.Minute)

	t.Run("EveryFileDecodes", func(t *testing.T) {
		t.Parallel()
		files := map[string][]byte{
			parentReviewRequestFile:   []byte(`{"slug":"s","round":2,"reviewer":"orch","opened_at":"2026-03-01T10:00:00Z","state":"open","cap":3,"unknown":true}`),
			parentReviewDeliveryFile:  []byte(`{"delivered_at":"2026-03-01T11:00:00Z","failed_reason":"gone","prompts":2,"last_prompt_at":"2026-03-01T11:01:00Z","waiting_notifies":1,"cap_warned":true,"pings":4,"last_ping_at":"2026-03-01T11:02:00Z"}`),
			parentReviewVerdictFile:   []byte(`{"kind":"approve","recorded_at":"2026-03-01T12:00:00Z","consumed":true,"superseding":true}`),
			parentReviewAlignmentFile: []byte(`{"attempt":"attempt-1","strand":"x:s:align","spawned_at":"2026-03-01T11:03:00Z","state":"applied","report":"r.md","reason":"ok"}`),
		}
		want := parentReviewRound{
			Number:  2,
			Request: &parentReviewRequest{Slug: "s", Round: 2, Reviewer: "orch", OpenedAt: opened, State: "open", Cap: 3},
			Delivery: parentReviewDelivery{
				DeliveredAt: delivered, FailedReason: "gone", Prompts: 2, LastPromptAt: lastPrompt,
				WaitingNotifys: 1, CapWarned: true, Pings: 4, LastPingAt: lastPing,
			},
			Verdict:   &parentReviewVerdict{Kind: "approve", RecordedAt: recorded, Consumed: true, Superseding: true},
			Alignment: &parentReviewAlignment{Attempt: "attempt-1", Strand: "x:s:align", SpawnedAt: spawned, State: "applied", Report: "r.md", Reason: "ok"},
		}
		got, err := parseParentReviewRound(2, files)
		if err != nil {
			t.Fatalf("parseParentReviewRound() error = %v; want nil", err)
		}
		if diff := cmp.Diff(want, got); diff != "" {
			t.Errorf("parseParentReviewRound() mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("AbsentFilesLeaveNilViews", func(t *testing.T) {
		t.Parallel()
		got, err := parseParentReviewRound(1, map[string][]byte{})
		if err != nil {
			t.Fatalf("parseParentReviewRound() error = %v; want nil", err)
		}
		if diff := cmp.Diff(parentReviewRound{Number: 1}, got); diff != "" {
			t.Errorf("parseParentReviewRound() mismatch (-want +got):\n%s", diff)
		}
	})

	for _, file := range []string{parentReviewRequestFile, parentReviewDeliveryFile, parentReviewVerdictFile, parentReviewAlignmentFile} {
		t.Run("UndecodableFileNamed_"+file, func(t *testing.T) {
			t.Parallel()
			got, err := parseParentReviewRound(1, map[string][]byte{file: []byte("{")})
			if err == nil || !strings.Contains(err.Error(), file) {
				t.Errorf("parseParentReviewRound() error = %v; want one naming %s", err, file)
			}
			if diff := cmp.Diff(parentReviewRound{}, got); diff != "" {
				t.Errorf("parseParentReviewRound() on error mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestParseAlignmentAttempt(t *testing.T) {
	t.Parallel()
	spawned := time.Date(2026, 3, 1, 11, 3, 0, 0, time.UTC)
	deadline := spawned.Add(10 * time.Minute)
	attemptJSON := []byte(`{"number":2,"key":"k","point":"parent-review","token":"tok","spawned_at":"2026-03-01T11:03:00Z","deadline":"2026-03-01T11:13:00Z","strand_guid":"g","snapshot":"abc","closed":true,"closed_reason":"done","applied":"applied","notified":true}`)
	wantAttempt := alignmentAttempt{
		Number: 2, Key: "k", Point: "parent-review", Token: "tok", SpawnedAt: spawned, Deadline: deadline,
		StrandGUID: "g", Snapshot: "abc", Closed: true, ClosedReason: "done", Applied: "applied", Notified: true,
	}

	t.Run("AttemptWithVerdict", func(t *testing.T) {
		t.Parallel()
		files := map[string][]byte{
			alignmentAttemptFile: attemptJSON,
			"verdict-tok.yaml":   []byte("verdict: approve\nreason: fine\nquestion: why\nsuperseded:\n  - a\n  - b\n"),
			"verdict-other.yaml": []byte("verdict: reject\n"),
		}
		attempt, verdict, err := parseAlignmentAttempt(files)
		if err != nil {
			t.Fatalf("parseAlignmentAttempt() error = %v; want nil", err)
		}
		if diff := cmp.Diff(wantAttempt, attempt); diff != "" {
			t.Errorf("parseAlignmentAttempt() attempt mismatch (-want +got):\n%s", diff)
		}
		wantVerdict := &alignmentVerdict{Verdict: alignmentVerdictApprove, Reason: "fine", Question: "why", Superseded: []string{"a", "b"}}
		if diff := cmp.Diff(wantVerdict, verdict); diff != "" {
			t.Errorf("parseAlignmentAttempt() verdict mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("AttemptWithoutVerdictFile", func(t *testing.T) {
		t.Parallel()
		attempt, verdict, err := parseAlignmentAttempt(map[string][]byte{alignmentAttemptFile: attemptJSON})
		if err != nil || verdict != nil {
			t.Fatalf("parseAlignmentAttempt() verdict, error = %v, %v; want nil, nil", verdict, err)
		}
		if diff := cmp.Diff(wantAttempt, attempt); diff != "" {
			t.Errorf("parseAlignmentAttempt() attempt mismatch (-want +got):\n%s", diff)
		}
	})

	errorTests := []struct {
		name    string
		files   map[string][]byte
		wantErr string
	}{
		{"AttemptFileAbsent", map[string][]byte{}, alignmentAttemptFile},
		{"AttemptFileUndecodable", map[string][]byte{alignmentAttemptFile: []byte("{")}, alignmentAttemptFile},
		{"VerdictFileUndecodable", map[string][]byte{alignmentAttemptFile: attemptJSON, "verdict-tok.yaml": []byte("verdict: [")}, "verdict-tok.yaml"},
	}
	for _, tt := range errorTests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, _, err := parseAlignmentAttempt(tt.files)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("parseAlignmentAttempt() error = %v; want one naming %s", err, tt.wantErr)
			}
		})
	}
}
