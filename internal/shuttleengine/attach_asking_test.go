// attach_asking_test.go covers how Attach treats a run that halted at a turn end:
// a held run, whose record stays running, is attached without a second notice for a turn end the parent already saw.

package shuttleengine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestDispositionCandidate_RelaunchingRecord pins that a relaunching mark attaches over a tracked strand.
// It does so inside its bound and past it.
// An untracked strand is left to the age rule.
// The strand is tracked unless a row says otherwise.
// The output files are absent, so no file contract decides the verdict.
//
//testtiming:keep pins the verdictError of a relaunching mark over an untracked strand, which no covering test reaches
func TestDispositionCandidate_RelaunchingRecord(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	now := time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC)
	bound := 10 * time.Minute
	spec := Spec{OutputFiles: []string{filepath.Join(t.TempDir(), "out.md")}}
	heldPaneless := []reedengine.StrandStatus{{GUID: "strand-1", Held: true}}
	tests := []struct {
		name          string
		relaunchingAt time.Time
		strands       []reedengine.StrandStatus
		want          attachVerdict
	}{
		{"within_the_bound_over_a_held_paneless_strand_attaches", now.Add(-time.Minute), heldPaneless, verdictAttachable},
		{"past_the_bound_over_a_held_paneless_strand_attaches_so_wait_settles_it", now.Add(-time.Hour), heldPaneless, verdictAttachable},
		{"within_the_bound_over_an_untracked_strand_cannot_be_ruled_live", now.Add(-time.Minute), nil, verdictError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c := attachCandidate{
				dirMtime: now,
				state:    RunState{StrandGUID: "strand-1", Outcome: runOutcomeRunning, RelaunchingAt: tt.relaunchingAt.Format(time.RFC3339)},
			}
			if got := dispositionCandidate(c, tt.strands, spec, time.Hour, now, bound); got != tt.want {
				t.Errorf("dispositionCandidate() = %v; want %v", got, tt.want)
			}
		})
	}
}

// TestAttach_HeldRuns drives Attach over a run.json with a recording notifier.
// It asserts the run's final outcome and which held turn ends reach the parent.
func TestAttach_HeldRuns(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	const strandName = "lym:slug:driver"
	shell := []BackgroundTask{{Kind: BackgroundShell, ID: "sh-1", Label: "sleep 9999"}}
	tests := []struct {
		name           string
		events         string
		notifiedOffset int64
		outstanding    []BackgroundTask
		shellJump      time.Duration
		wantOutcome    Outcome
		// wantNoticeEnds holds the ending of each expected notice, in order.
		wantNoticeEnds []string
	}{
		{
			name:        "running record whose last held stop was notified: attached without a new notice",
			events:      "STOP:held\n",
			wantOutcome: OutcomeTimeout, notifiedOffset: int64(len("STOP:held\n")),
		},
		{
			name:   "running record whose waiting line was notified: no new notice once the shell bound elapsed",
			events: "WAIT:background work\n", notifiedOffset: int64(len("WAIT:background work\n")),
			outstanding: shell, shellJump: 6 * time.Minute, wantOutcome: OutcomeTimeout,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			status := reedengine.StatusResult{Strands: []reedengine.StrandStatus{{GUID: "strand-1", Name: strandName, PaneID: "%1", Live: true}}}
			timeout := heldTestTimeout
			var clk Clock = newFakeClock(time.Now())
			if tt.shellJump > 0 {
				timeout = time.Hour
				clk = &jumpClock{fakeClock: newFakeClock(time.Now()), jump: tt.shellJump}
			}
			fx := newFixture(t, &fakeReed{StatusQueue: []reedengine.StatusResult{status}}, &waitingEngine{outstanding: tt.outstanding},
				withConfig(fastConfig), withSeparateRunDir(), withClock(clk))
			seedPresentReedState(t, fx.DotLym)

			outputFile := filepath.Join(fx.RunRoot, "out.md")
			runDir := seedAttachRun(t, fx.RunRoot, "run-1", seedAttachRunOpts{
				strandGUID: "strand-1", sessionID: "session-1", outputFiles: []string{outputFile}, outcome: runOutcomeRunning, includeOutcome: true, started: true,
			})
			rs, found, err := loadRunState(runDir)
			if err != nil || !found {
				t.Fatalf("loadRunState: found=%v err=%v", found, err)
			}
			rs.NotifiedOffset = tt.notifiedOffset
			if err := saveRunState(runDir, rs); err != nil {
				t.Fatalf("saveRunState: %v", err)
			}
			if err := os.WriteFile(filepath.Join(runDir, eventsFileName), []byte(tt.events), 0o644); err != nil {
				t.Fatalf("seed events: %v", err)
			}

			var notices []string
			fx.Runner.SetNotifier(func(line string) error {
				notices = append(notices, line)
				return nil
			})
			result, found, err := fx.Runner.Attach(Spec{OutputFiles: []string{outputFile}, Timeout: timeout})
			if err != nil || !found {
				t.Fatalf("Attach() found=%v err=%v; want found with no error", found, err)
			}
			if result.Outcome != tt.wantOutcome {
				t.Errorf("Outcome = %q; want %q", result.Outcome, tt.wantOutcome)
			}
			if len(notices) != len(tt.wantNoticeEnds) {
				t.Fatalf("notices = %q; want %d", notices, len(tt.wantNoticeEnds))
			}
			for i, end := range tt.wantNoticeEnds {
				if !strings.HasSuffix(notices[i], end) {
					t.Errorf("notice %d = %q; want it to end with %q", i, notices[i], end)
				}
				if !strings.Contains(notices[i], "lym creel post "+strandName+" ") {
					t.Errorf("notice %d = %q; want it to name the strand reed reports for the guid", i, notices[i])
				}
			}
		})
	}
}
