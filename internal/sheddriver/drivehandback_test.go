// drivehandback_test.go — untagged Tier-1 tests for the shed driver's hand-back: the parent messages, the stop report and the park, over fake seams in a temp worktree.

package sheddriver

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// TestDriveNotices pins what a hand-back tells its parent under each notify rule: the notice verbatim, the question line, the friction-failed done line and the run-id line, each ending in the tail.
func TestDriveNotices(t *testing.T) {
	t.Parallel()

	const tail = " Answer briefly, then continue your task."
	awaiting := Stop{OK: true, State: "awaiting", LoopState: "awaiting"}
	done := Stop{OK: true, State: "done", LoopState: "done"}
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
		{name: "watched blocked stop sends nothing", h: driveHandBack{Stop: Stop{OK: true, State: "blocked", LoopState: "blocked"}}, watched: true},
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

// TestDriveHandBack covers handBackRun over fake notify, orch-lookup and park seams in a temp worktree:
// what the stop report holds, how a declined send, a failed send and a missing orch strand read, that the park runs after the report is written and its error is the hand-back's, and that a friction-failed done reports without parking.
func TestDriveHandBack(t *testing.T) {
	t.Parallel()

	baseHandBack := driveHandBack{
		RunID:    "run-1",
		Stop:     Stop{OK: true, State: "awaiting", LoopState: "awaiting"},
		Question: "widen the task?",
	}
	frictionDone := driveHandBack{RunID: "run-1", Stop: Stop{OK: true, State: "done", LoopState: "done"}, DoneFrictionFailed: true}
	tests := []struct {
		name         string
		h            driveHandBack
		orchRecorded bool
		notifyErr    error
		// notifyDeclines makes Notify report sent false with no error.
		notifyDeclines bool
		parkErr        error
		// wantReport lists the substrings the written report holds.
		wantReport []string
		wantSent   int
		// wantParked is true when Park is called with the report path.
		wantParked bool
		// wantErr is the text the hand-back's error holds; empty rows expect none.
		wantErr string
		// noFrictionDir leaves the report's friction directory empty.
		noFrictionDir bool
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
			name: "an empty friction directory reads none", h: baseHandBack, orchRecorded: true, noFrictionDir: true,
			wantReport: []string{"Friction directory: none"}, wantSent: 1, wantParked: true,
		},
		{
			name: "a notice the recipe declines reads not sent", h: baseHandBack, orchRecorded: true, notifyDeclines: true,
			wantReport: []string{"not sent, the recipe sends none at this stop: Run run-1 stopped with a question"}, wantSent: 1, wantParked: true,
		},
		{
			name: "a park that fails is the hand-back's error after the report is written", h: baseHandBack, orchRecorded: true, parkErr: errors.New("marker unwritable"),
			wantReport: []string{"sent: Run run-1 stopped with a question"}, wantSent: 1, wantParked: true, wantErr: "marker unwritable",
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
			var parked []string
			deps := HandBackDeps{
				ReportPath: filepath.Join(root, "drive-reports", "report.md"),
				Notify: func(_ Stop, line string) (bool, error) {
					sent = append(sent, line)
					return !tt.notifyDeclines, tt.notifyErr
				},
				OrchRecorded: func() (bool, error) { return tt.orchRecorded, nil },
				Park: func(reportPath string) error {
					if _, err := os.Stat(reportPath); err != nil {
						t.Errorf("Park ran before the stop report was written: %v", err)
					}
					parked = append(parked, reportPath)
					return tt.parkErr
				},
				Now: func() time.Time { return time.Date(2026, 10, 12, 12, 0, 0, 0, time.UTC) },
			}
			h := tt.h
			h.ReportPath = deps.ReportPath
			report := driveReport{
				RunID:         "run-1",
				HistoryLength: 4,
				Progress:      "step 2 of 5",
				Reason:        "the review needs a decision",
				Rules:         []string{"an awaiting stop: planned hand-back"},
				Attempts:      []driveAttemptNote{{Role: TacklerRole, Number: 1, Verdict: TacklerVerdictHandBack, Cause: "timeout", VerdictPath: "/t/verdict.yaml"}},
				Question:      h.Question,
				FrictionDir:   "/friction",
				RepairDir:     "/repair",
			}

			if tt.noFrictionDir {
				report.FrictionDir = ""
			}

			err := handBackRun(deps, h, report, true)

			if tt.wantErr == "" && err != nil {
				t.Fatalf("handBackRun() error = %v; want nil", err)
			}
			if tt.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErr)) {
				t.Fatalf("handBackRun() error = %v; want it to hold %q", err, tt.wantErr)
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
			wantParks := 0
			if tt.wantParked {
				wantParks = 1
			}
			if len(parked) != wantParks || (wantParks == 1 && parked[0] != deps.ReportPath) {
				t.Errorf("Park calls = %q; want %d, with the report path %q", parked, wantParks, deps.ReportPath)
			}
		})
	}
}
