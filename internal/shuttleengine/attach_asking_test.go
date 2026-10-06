// attach_asking_test.go covers the asking re-entry half of Attach: a live strand whose run.json says
// asking but whose events file grew past the recorded AskingOffset kept working and is attached to,
// while one with no growth or no recorded offset is still respawn-eligible.

package shuttleengine

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/reedengine"
)

const askingEventsOld = "ASK:old\n"

// askingCandidate builds an asking candidate over the given strand, with eventsSize the events
// file's current size and offset the recorded AskingOffset (nil for an older binary's record).
func askingCandidate(offset *int64, eventsSize int64) attachCandidate {
	return attachCandidate{
		dirMtime:   time.Now(),
		state:      RunState{StrandGUID: "strand-1", Outcome: string(OutcomeAsking), AskingOffset: offset},
		eventsSize: eventsSize,
	}
}

//testtiming:keep pins the three asking re-entry verdicts at the unit level, including no events since the recorded offset and no recorded offset, which the end-to-end attach test never reaches
func TestDispositionCandidate_AskingReentry(t *testing.T) {
	ten := int64(10)
	strands := []reedengine.StrandStatus{{GUID: "strand-1", PaneID: "%1", Live: true}}
	spec := Spec{OutputFiles: []string{filepath.Join(t.TempDir(), "out.md")}}
	tests := []struct {
		name string
		c    attachCandidate
		want attachVerdict
	}{
		{"events_grew_past_offset_attaches", askingCandidate(&ten, 11), verdictAttachable},
		{"no_events_since_respawns", askingCandidate(&ten, 10), verdictRespawnEligible},
		{"no_recorded_offset_respawns", askingCandidate(nil, 500), verdictRespawnEligible},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := dispositionCandidate(tt.c, strands, spec, time.Hour, time.Now()); got != tt.want {
				t.Errorf("dispositionCandidate() = %v; want %v", got, tt.want)
			}
		})
	}
}

//testtiming:keep pins that the candidate scan records each events file's size, which the end-to-end attach test observes only through its one growth case
func TestCollectAttachCandidates_RecordsEventsSize(t *testing.T) {
	runRoot := t.TempDir()
	outputFile := filepath.Join(runRoot, "out.md")
	runDir := seedAttachRun(t, runRoot, "run-1", seedAttachRunOpts{
		strandGUID: "strand-1", outputFiles: []string{outputFile}, outcome: "asking", includeOutcome: true,
	})
	if err := os.WriteFile(filepath.Join(runDir, eventsFileName), []byte(askingEventsOld), 0o644); err != nil {
		t.Fatalf("seed events: %v", err)
	}
	candidates, err := collectAttachCandidates(runRoot, []string{outputFile})
	if err != nil || len(candidates) != 1 {
		t.Fatalf("collectAttachCandidates() = %d candidates, err %v; want 1, nil", len(candidates), err)
	}
	if got, want := candidates[0].eventsSize, int64(len(askingEventsOld)); got != want {
		t.Errorf("eventsSize = %d; want %d", got, want)
	}
}

// TestAttach_AskingReentryStartsAtRecordedOffset attaches to an asking run whose events grew, and
// checks the reconstructed run read only the new events: the old ask is not re-classified.
func TestAttach_AskingReentryStartsAtRecordedOffset(t *testing.T) {
	reed := &fakeReed{StatusQueue: []reedengine.StatusResult{liveStatus("strand-1", "%1")}}
	fx := newFixture(t, reed, &fakeEngine{}, withConfig(fastConfig), withSeparateRunDir())
	runner, dotLyxDir, runRoot := fx.Runner, fx.DotLyx, fx.RunRoot
	seedPresentReedState(t, dotLyxDir)

	outputFile := filepath.Join(runRoot, "out.md")
	runDir := seedAttachRun(t, runRoot, "run-1", seedAttachRunOpts{
		strandGUID: "strand-1", sessionID: "session-1", outputFiles: []string{outputFile}, outcome: "asking", includeOutcome: true,
	})
	// Record the offset by rewriting the fixture through the real RunState codec.
	rs, found, err := loadRunState(runDir)
	if err != nil || !found {
		t.Fatalf("loadRunState: found=%v err=%v", found, err)
	}
	offset := int64(len(askingEventsOld))
	rs.AskingOffset = &offset
	if err := saveRunState(runDir, rs); err != nil {
		t.Fatalf("saveRunState: %v", err)
	}
	if err := os.WriteFile(filepath.Join(runDir, eventsFileName), []byte(askingEventsOld+"STOP:new\n"), 0o644); err != nil {
		t.Fatalf("seed events: %v", err)
	}

	result, found, err := runner.Attach(Spec{OutputFiles: []string{outputFile}, Timeout: time.Minute})
	if err != nil {
		t.Fatalf("Attach() error = %v; want nil", err)
	}
	if !found {
		t.Fatalf("found = false; want true (asking run that kept working attaches)")
	}
	if result.Outcome != OutcomeAsking || result.LastAssistantMessage != "new" {
		t.Errorf("result = {%s %q}; want asking with the post-offset message %q", result.Outcome, result.LastAssistantMessage, "new")
	}
	after, _, err := loadRunState(runDir)
	if err != nil {
		t.Fatalf("loadRunState after: %v", err)
	}
	if after.AskingOffset == nil || *after.AskingOffset != int64(len(askingEventsOld+"STOP:new\n")) {
		t.Errorf("AskingOffset after = %v; want the new asking's consumed offset", after.AskingOffset)
	}
}
