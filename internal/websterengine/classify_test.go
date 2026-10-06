// classify_test.go covers Classify's pinned decision order: a present Report short-circuits to
// terminal regardless of every other field, with its head SHA and deviations carried through and a
// large deviation list never changing the mapped status (the deviation list is informational),
// and absent a report the dead-reason precedence runs TurnEnded -> timeout -> strand-died ->
// running, in that order.
// Tier 1: no git, inputs constructed directly.

package websterengine

import (
	"fmt"
	"slices"
	"testing"
	"time"
)

func TestClassify(t *testing.T) {
	t.Parallel()

	var many []string
	for i := 0; i < 500; i++ {
		many = append(many, fmt.Sprintf("internal/pkg%d/file.go", i))
	}

	tests := []struct {
		name         string
		in           ClassifyInputs
		wantTerminal bool
		wantStatus   string
		wantDead     string
		wantBatch    string
		wantHeadSHA  string
		wantElapsedS int
	}{
		{
			name:         "a report's deviations carry through and never change an OK status",
			in:           ClassifyInputs{BatchNumber: 1, BatchSlug: "x", Report: &Report{Status: ReportStatusOK, HeadSHA: "abc", Deviations: many}},
			wantTerminal: true,
			wantStatus:   DigestStatusDone,
			wantHeadSHA:  "abc",
		},
		{
			name:         "a report's deviations never change a FAILED status",
			in:           ClassifyInputs{BatchNumber: 1, BatchSlug: "x", Report: &Report{Status: ReportStatusFailed, HeadSHA: "def", Deviations: many}},
			wantTerminal: true,
			wantStatus:   DigestStatusStuck,
			wantHeadSHA:  "def",
		},
		{
			// Every other field is set to values that would otherwise classify as
			// dead-timeout, to prove Report wins outright regardless.
			name: "report present short-circuits to terminal done",
			in: ClassifyInputs{
				BatchNumber:  4,
				BatchSlug:    "some-batch",
				Report:       &Report{Status: ReportStatusOK, HeadSHA: "abc123"},
				TurnEnded:    true,
				StrandLive:   false,
				Elapsed:      10 * time.Hour,
				BatchTimeout: time.Minute,
			},
			wantTerminal: true,
			wantStatus:   DigestStatusDone,
			wantBatch:    "04-some-batch",
			wantHeadSHA:  "abc123",
		},
		{
			name: "report present FAILED is stuck",
			in: ClassifyInputs{
				BatchNumber: 1,
				BatchSlug:   "x",
				Report:      &Report{Status: ReportStatusFailed, HeadSHA: "def"},
			},
			wantTerminal: true,
			wantStatus:   DigestStatusStuck,
			wantHeadSHA:  "def",
		},
		{
			name: "no report and turn ended is dead asking",
			in: ClassifyInputs{
				BatchNumber: 2,
				BatchSlug:   "y",
				TurnEnded:   true,
				// StrandLive must not matter: TurnEnded is checked first.
				StrandLive:   true,
				Elapsed:      time.Second,
				BatchTimeout: time.Hour,
			},
			wantTerminal: true,
			wantStatus:   DigestStatusDead,
			wantDead:     DeadReasonAsking,
		},
		{
			name: "no report past the timeout is dead timeout",
			in: ClassifyInputs{
				BatchNumber: 3,
				BatchSlug:   "z",
				TurnEnded:   false,
				// StrandLive must not matter: timeout is checked before strand liveness.
				StrandLive:   true,
				Elapsed:      2 * time.Hour,
				BatchTimeout: time.Hour,
			},
			wantTerminal: true,
			wantStatus:   DigestStatusDead,
			wantDead:     DeadReasonTimeout,
		},
		{
			name: "no report and a dead strand is dead died",
			in: ClassifyInputs{
				BatchNumber:  5,
				BatchSlug:    "w",
				TurnEnded:    false,
				StrandLive:   false,
				Elapsed:      time.Minute,
				BatchTimeout: time.Hour,
			},
			wantTerminal: true,
			wantStatus:   DigestStatusDead,
			wantDead:     DeadReasonDied,
		},
		{
			name: "no report and a live strand is still running",
			in: ClassifyInputs{
				BatchNumber:  6,
				BatchSlug:    "v",
				TurnEnded:    false,
				StrandLive:   true,
				Elapsed:      42 * time.Second,
				BatchTimeout: time.Hour,
			},
			wantTerminal: false,
			wantStatus:   DigestStatusRunning,
			wantBatch:    "06-v",
			wantElapsedS: 42,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, terminal := Classify(tt.in)

			if terminal != tt.wantTerminal {
				t.Fatalf("Classify() terminal = %v; want %v", terminal, tt.wantTerminal)
			}
			if got.Status != tt.wantStatus {
				t.Errorf("Classify().Status = %q; want %q", got.Status, tt.wantStatus)
			}
			if got.DeadReason != tt.wantDead {
				t.Errorf("Classify().DeadReason = %q; want %q", got.DeadReason, tt.wantDead)
			}
			if tt.wantBatch != "" && got.Batch != tt.wantBatch {
				t.Errorf("Classify().Batch = %q; want %q", got.Batch, tt.wantBatch)
			}
			if got.HeadSHA != tt.wantHeadSHA {
				t.Errorf("Classify().HeadSHA = %q; want %q", got.HeadSHA, tt.wantHeadSHA)
			}
			if tt.wantElapsedS != 0 && got.ElapsedS != tt.wantElapsedS {
				t.Errorf("Classify().ElapsedS = %d; want %d", got.ElapsedS, tt.wantElapsedS)
			}
			if tt.in.Report != nil && !slices.Equal(got.Deviations, tt.in.Report.Deviations) {
				t.Errorf("Classify().Deviations has %d entries; want the report's %d, in order", len(got.Deviations), len(tt.in.Report.Deviations))
			}
		})
	}
}
