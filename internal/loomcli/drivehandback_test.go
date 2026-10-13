// drivehandback_test.go — untagged Tier-1 tests for the Go driver's hand-back: the parent messages, the stop report, the commit and the park marker, over fake seams in a temp worktree.

package loomcli

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/loomengine"
)

// TestDriveNotices pins what a hand-back tells its parent under each notify rule: the notice verbatim, the question line, the friction-failed done line and the run-id line, each ending in the tail.
func TestDriveNotices(t *testing.T) {
	t.Parallel()

	const tail = " Answer briefly, then continue your task."
	awaiting := driveStop{OK: true, State: "awaiting", LoopState: "awaiting"}
	done := driveStop{OK: true, State: "done", LoopState: "done"}
	generic := "Run run-1 stopped; stop report /r.md." + tail
	question := "Run run-1 stopped with a question; stop report /r.md. Question: widen the task?" + tail
	friction := "Run run-1 is done but reported friction: failed; stop report /r.md." + tail

	tests := []struct {
		name    string
		h       driveHandBack
		watched bool
		want    []string
	}{
		{name: "watched relays the parent notice verbatim", h: driveHandBack{Stop: awaiting, Relay: "review is ready"}, watched: true, want: []string{"review is ready" + tail}},
		{name: "unwatched relays the notice in place of the generic line", h: driveHandBack{Stop: awaiting, Relay: "review is ready"}, want: []string{"review is ready" + tail}},
		{name: "watched question names run-id, report and question", h: driveHandBack{Stop: awaiting, Question: "widen the task?"}, watched: true, want: []string{question}},
		{name: "unwatched question names run-id, report and question", h: driveHandBack{Stop: awaiting, Question: "widen the task?"}, want: []string{question}},
		{name: "watched friction-failed done notifies", h: driveHandBack{Stop: done, DoneFrictionFailed: true}, watched: true, want: []string{friction}},
		{name: "unwatched friction-failed done notifies", h: driveHandBack{Stop: done, DoneFrictionFailed: true}, want: []string{friction}},
		{name: "watched awaiting without a notice sends nothing", h: driveHandBack{Stop: awaiting}, watched: true},
		{name: "unwatched awaiting without a notice sends the run-id line", h: driveHandBack{Stop: awaiting}, want: []string{generic}},
		{name: "unwatched done without friction sends nothing", h: driveHandBack{Stop: done}},
		{name: "watched verify halt sends the unwatched line", h: driveHandBack{Stop: driveStop{OK: true, State: "blocked", LoopState: "blocked"}, WatchedVerifyLine: true}, watched: true, want: []string{generic}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tt.h.RunID = "run-1"
			tt.h.ReportPath = "/r.md"

			got := driveNotices(tt.h, tt.watched)

			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("driveNotices() = %q; want %q", got, tt.want)
			}
			for _, notice := range got {
				if !strings.HasSuffix(notice, driveNoticeTail) {
					t.Errorf("notice %q does not end with the tail", notice)
				}
			}
		})
	}
}

// TestDriveHandBack covers handBackRun over fake notify, orch-lookup and commit seams in a temp worktree:
// what the stop report holds, how a failed send and a missing orch strand read, that the commit runs and the park marker holds the report path, that a failed commit still writes the marker, and that a friction-failed done reports without parking.
func TestDriveHandBack(t *testing.T) {
	t.Parallel()

	baseHandBack := driveHandBack{
		RunID:    "run-1",
		Stop:     driveStop{OK: true, State: "awaiting", LoopState: "awaiting"},
		Question: "widen the task?",
	}
	frictionDone := driveHandBack{RunID: "run-1", Stop: driveStop{OK: true, State: "done", LoopState: "done"}, DoneFrictionFailed: true}
	tests := []struct {
		name         string
		h            driveHandBack
		orchRecorded bool
		notifyErr    error
		commitErr    error
		// wantReport lists the substrings the written report holds.
		wantReport []string
		wantSent   int
		// wantParked is true when the commit runs and the park marker is written.
		wantParked bool
	}{
		{
			name: "report holds the run, reason, rules, attempts, question and directories", h: baseHandBack, orchRecorded: true,
			wantReport: []string{
				"run-1", "Reason: the review needs a decision", "an awaiting stop: planned hand-back", "tackler attempt 1: verdict hand-back, cause timeout, verdict file /t/verdict.yaml",
				"widen the task?", "Friction directory: /friction", "Repair directory: /repair", "history length 4", "sent: Run run-1 stopped with a question",
			},
			wantSent: 1, wantParked: true,
		},
		{
			name: "failed send appears in the report", h: baseHandBack, orchRecorded: true, notifyErr: errors.New("queue full"),
			wantReport: []string{"send failed (queue full): Run run-1 stopped with a question"}, wantSent: 1, wantParked: true,
		},
		{
			name: "no orch strand queues nothing and the report carries the line", h: baseHandBack,
			wantReport: []string{"not queued, no orch strand recorded: Run run-1 stopped with a question"}, wantParked: true,
		},
		{
			name: "failing commit still writes the park marker", h: baseHandBack, orchRecorded: true, commitErr: errors.New("disk full"),
			wantReport: []string{"sent: Run run-1 stopped with a question"}, wantSent: 1, wantParked: true,
		},
		{
			name: "friction-failed done writes the report with its send line and parks nothing", h: frictionDone, orchRecorded: true,
			wantReport: []string{"sent: Run run-1 is done but reported friction: failed"}, wantSent: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			var sent []string
			var commits int
			deps := driveParkDeps{
				ReportPath: filepath.Join(root, "drive-reports", "report.md"),
				Commit: commitStatusDeps{
					MergeActive: func() (bool, error) { return false, nil },
					Commit:      func(string) error { commits++; return tt.commitErr },
					Push:        func() error { return nil },
				},
				MarkerPath: filepath.Join(root, "scratch", "driver-parked"),
				Notify: func(line string) error {
					sent = append(sent, line)
					return tt.notifyErr
				},
				OrchRecorded: func() (bool, error) { return tt.orchRecorded, nil },
				Now:          func() time.Time { return time.Date(2026, 10, 12, 12, 0, 0, 0, time.UTC) },
			}
			h := tt.h
			h.ReportPath = deps.ReportPath
			report := driveReport{
				RunID:         "run-1",
				HistoryLength: 4,
				Progress:      "step 2 of 5",
				Reason:        "the review needs a decision",
				Rules:         []string{"an awaiting stop: planned hand-back"},
				Attempts:      []driveAttemptNote{{Role: loomengine.TacklerRole, Number: 1, Verdict: loomengine.TacklerVerdictHandBack, Cause: "timeout", VerdictPath: "/t/verdict.yaml"}},
				Question:      h.Question,
				FrictionDir:   "/friction",
				RepairDir:     "/repair",
			}

			err := handBackRun(deps, h, report, true)

			if err != nil {
				t.Fatalf("handBackRun() error = %v; want nil", err)
			}
			data, readErr := os.ReadFile(deps.ReportPath)
			if readErr != nil {
				t.Fatalf("read the stop report: %v", readErr)
			}
			for _, want := range tt.wantReport {
				if !strings.Contains(string(data), want) {
					t.Errorf("stop report does not hold %q:\n%s", want, data)
				}
			}
			if len(sent) != tt.wantSent {
				t.Errorf("notices queued = %d; want %d", len(sent), tt.wantSent)
			}
			marker, markerErr := os.ReadFile(deps.MarkerPath)
			if tt.wantParked {
				if commits != 1 {
					t.Errorf("commits = %d; want 1", commits)
				}
				if markerErr != nil || string(marker) != deps.ReportPath {
					t.Errorf("park marker = %q, error %v; want the report path %q", marker, markerErr, deps.ReportPath)
				}
				return
			}
			if commits != 0 || !errors.Is(markerErr, os.ErrNotExist) {
				t.Errorf("commits = %d, marker error = %v; want no commit and no park marker", commits, markerErr)
			}
		})
	}
}
