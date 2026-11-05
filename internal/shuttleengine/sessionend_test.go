// sessionend_test.go covers the session-end hook on the stop path, where a run's record alone reports the ended session.

package shuttleengine

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestRunner_SessionEnd covers the hook StopStrand calls for an unawaited record it moves to stopped:
// the record's session, engine, role, model and start with the stop time and the engine's reading, the unknown reading for an engine without the capability, and no call for a record already terminal.
func TestRunner_SessionEnd(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	reading := SessionUsage{Known: true, Fresh: 100, CacheRead: 900, SplitKnown: true, Input: 40, CacheWrite: 10, Output: 50}
	started := time.Date(2026, 11, 5, 9, 0, 0, 0, time.UTC)
	stopped := started.Add(12 * time.Minute)

	tests := []struct {
		name      string
		reader    bool
		recorded  string
		wantCalls int
		wantUsage SessionUsage
	}{
		{name: "RunningRecordOnAReaderEngine", reader: true, recorded: runOutcomeRunning, wantCalls: 1, wantUsage: reading},
		{name: "RunningRecordOnAnEngineWithoutTheCapability", recorded: runOutcomeRunning, wantCalls: 1},
		{name: "TerminalRecordIsNotReportedAgain", reader: true, recorded: string(OutcomeDied)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var engine Engine = &fakeEngine{}
			if tt.reader {
				engine = &usageEngine{reading: reading}
			}
			fake := &fakeReed{StatusQueue: []reedengine.StatusResult{{Strands: []reedengine.StrandStatus{{GUID: "strand-1", PaneID: "%1", Live: true}}}}}
			fx := newFixture(t, fake, engine, withConfig(fastConfig), withSeparateRunDir(), withClock(newFakeClock(stopped)))
			record := RunState{
				RunID: "run-1", StrandGUID: "strand-1", SessionID: "session-1", Engine: "fake", Role: "sampler", Model: "opus",
				CreatedAt: started.Format(time.RFC3339), Outcome: tt.recorded,
			}
			runDir := filepath.Join(fx.RunRoot, "run-1")
			if err := os.MkdirAll(runDir, 0o755); err != nil {
				t.Fatalf("mkdir run dir: %v", err)
			}
			if err := saveRunState(runDir, record); err != nil {
				t.Fatalf("saveRunState: %v", err)
			}
			var ends []SessionEnd
			fx.Runner.SetSessionEnd(func(end SessionEnd) { ends = append(ends, end) })

			if err := fx.Runner.StopStrand("strand-1"); err != nil {
				t.Fatalf("StopStrand() error = %v; want nil", err)
			}

			if len(ends) != tt.wantCalls {
				t.Fatalf("session end calls = %+v; want %d", ends, tt.wantCalls)
			}
			if tt.wantCalls == 0 {
				return
			}
			got := ends[0]
			if !got.StartedAt.Equal(started) || !got.EndedAt.Equal(stopped) {
				t.Errorf("session end times = %v to %v; want %v to %v", got.StartedAt, got.EndedAt, started, stopped)
			}
			got.StartedAt, got.EndedAt = time.Time{}, time.Time{}
			want := SessionEnd{SessionID: "session-1", Engine: "fake", Role: "sampler", Model: "opus", Worktree: fx.Runner.paneCwd, Outcome: Outcome(runOutcomeStopped), Usage: tt.wantUsage}
			if got != want {
				t.Errorf("session end = %+v; want %+v", got, want)
			}
		})
	}
}
