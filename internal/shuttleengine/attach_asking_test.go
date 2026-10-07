// attach_asking_test.go covers how Attach treats a run that halted at a turn end: a legacy record an
// older binary left at outcome asking is attached when its strand is live, and a held run, whose
// record stays running, is attached without a second notice for a turn end the parent already saw.

package shuttleengine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/reedengine"
)

const askingEventsOld = "ASK:old\n"

// offsetOf returns a pointer to the byte length of events, a recorded AskingOffset just past them.
func offsetOf(events string) *int64 {
	offset := int64(len(events))
	return &offset
}

//testtiming:keep pins the unit-level verdicts for a legacy asking record, with and without a recorded offset, and the unchanged verdicts for a dead pane and an untracked strand, which the end-to-end attach test never reaches
func TestDispositionCandidate_AskingReentry(t *testing.T) {
	live := []reedengine.StrandStatus{{GUID: "strand-1", PaneID: "%1", Live: true}}
	dead := []reedengine.StrandStatus{{GUID: "strand-1", PaneID: "%1", Live: false}}
	spec := Spec{OutputFiles: []string{filepath.Join(t.TempDir(), "out.md")}}
	tests := []struct {
		name    string
		offset  *int64
		strands []reedengine.StrandStatus
		want    attachVerdict
	}{
		{"live_with_offset_attaches", offsetOf(askingEventsOld), live, verdictAttachable},
		{"live_without_offset_attaches", nil, live, verdictAttachable},
		{"dead_pane_respawns", offsetOf(askingEventsOld), dead, verdictRespawnEligible},
		{"untracked_respawns", offsetOf(askingEventsOld), nil, verdictRespawnEligible},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := attachCandidate{
				dirMtime: time.Now(),
				state:    RunState{StrandGUID: "strand-1", Outcome: legacyAskingOutcome, AskingOffset: tt.offset},
			}
			if got := dispositionCandidate(c, tt.strands, spec, time.Hour, time.Now()); got != tt.want {
				t.Errorf("dispositionCandidate() = %v; want %v", got, tt.want)
			}
		})
	}
}

// TestAttach_LegacyAskingAndHeldRuns drives Attach over a run.json with a recording notifier and asserts
// the run's final outcome and which held turn ends reach the parent.
func TestAttach_LegacyAskingAndHeldRuns(t *testing.T) {
	const strandName = "lyx:slug:driver"
	shell := []BackgroundTask{{Kind: BackgroundShell, ID: "sh-1", Label: "sleep 9999"}}
	tests := []struct {
		name           string
		outcome        string
		events         string
		askingOffset   *int64
		notifiedOffset int64
		outputsExist   bool
		outstanding    []BackgroundTask
		shellJump      time.Duration
		wantOutcome    Outcome
		// wantNoticeEnds holds the ending of each expected notice, in order.
		wantNoticeEnds []string
	}{
		{
			name:    "legacy ask with offset: a later stop with outputs finalizes done and notifies nothing",
			outcome: legacyAskingOutcome, events: askingEventsOld + "STOP:done\n", askingOffset: offsetOf(askingEventsOld),
			outputsExist: true, wantOutcome: OutcomeDone,
		},
		{
			name:    "legacy ask with offset: the old ask is not notified, a later held stop is",
			outcome: legacyAskingOutcome, events: askingEventsOld + "STOP:new\n", askingOffset: offsetOf(askingEventsOld),
			wantOutcome: OutcomeTimeout, wantNoticeEnds: []string{"«new»"},
		},
		{
			name:    "legacy ask with offset and no events since: attached, holds to the deadline unnotified",
			outcome: legacyAskingOutcome, events: askingEventsOld, askingOffset: offsetOf(askingEventsOld),
			wantOutcome: OutcomeTimeout,
		},
		{
			name:    "legacy ask without offset: several held turn ends notify only the last, once",
			outcome: legacyAskingOutcome, events: "STOP:one\nSTOP:two\nSTOP:three\n",
			wantOutcome: OutcomeTimeout, wantNoticeEnds: []string{"«three»"},
		},
		{
			name:    "running record whose last held stop was notified: attached without a new notice",
			outcome: runOutcomeRunning, events: "STOP:held\n", notifiedOffset: int64(len("STOP:held\n")),
			wantOutcome: OutcomeTimeout,
		},
		{
			name:    "running record whose waiting line was notified: no new notice once the shell bound elapsed",
			outcome: runOutcomeRunning, events: "WAIT:background work\n", notifiedOffset: int64(len("WAIT:background work\n")),
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
			seedPresentReedState(t, fx.DotLyx)

			outputFile := filepath.Join(fx.RunRoot, "out.md")
			if tt.outputsExist {
				touchOutputFile(t, outputFile)
			}
			runDir := seedAttachRun(t, fx.RunRoot, "run-1", seedAttachRunOpts{
				strandGUID: "strand-1", sessionID: "session-1", outputFiles: []string{outputFile}, outcome: tt.outcome, includeOutcome: true, started: true,
			})
			rs, found, err := loadRunState(runDir)
			if err != nil || !found {
				t.Fatalf("loadRunState: found=%v err=%v", found, err)
			}
			rs.AskingOffset = tt.askingOffset
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
				if !strings.Contains(notices[i], "SendMessage to "+strandName+",") {
					t.Errorf("notice %d = %q; want it to name the strand reed reports for the guid", i, notices[i])
				}
			}
		})
	}
}
