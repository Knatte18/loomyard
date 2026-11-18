// attachrelaunch_test.go covers how an attach settles a run record whose relaunching mark outlived its waiter.

package shuttleengine

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestAttach_RelaunchingRecordPastItsBound pins how an attached Wait treats a record marked relaunching.
// Within its bound the mark holds the run live; past it, Wait settles the run by what the strand shows.
func TestAttach_RelaunchingRecordPastItsBound(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	const (
		promptLine = "Read the task prompt and carry it out."
		limitedEnd = "START\nSTOP:limit\n"
	)
	// The bound is twice fastConfig's startup timeout.
	bound := relaunchBound(fastConfig)
	heldPaneless := reedengine.StrandStatus{GUID: "strand-1", Held: true}
	live := reedengine.StrandStatus{GUID: "strand-1", PaneID: "%1", Live: true}

	tests := []struct {
		name string
		// markedAgo is how long before Attach the record's mark was written.
		markedAgo time.Duration
		strand    reedengine.StrandStatus
		// events is the events file; promptOffset and promptLine seed the record's new launch.
		events       string
		promptOffset int64
		promptLine   string
		outputExists bool
		timeout      time.Duration
		wantOutcome  Outcome
		wantRemoved  bool
		wantTyped    []string
		// wantElapsed is the least run-clock time Attach takes.
		wantElapsed time.Duration
		// wantMarkCleared is whether the record's mark is gone once Attach returns; it is read only when set.
		wantMarkCleared bool
	}{
		{
			name:      "AMarkBeforeTheRecordWriteOfAHeldStrandPastItsBoundEndsDiedWithTheStrandRemoved",
			markedAgo: 5 * time.Minute, strand: heldPaneless, timeout: time.Minute,
			wantOutcome: OutcomeDied, wantRemoved: true,
		},
		{
			name:      "AMarkWithinItsBoundAttachesAsLive",
			markedAgo: 10 * time.Second, strand: heldPaneless, timeout: 20 * time.Second,
			wantOutcome: OutcomeTimeout,
		},
		{
			name:      "AMarkPastItsBoundOverALiveStrandClearsAndTypesThePromptLineOnce",
			markedAgo: 5 * time.Minute, strand: live, events: limitedEnd, promptOffset: int64(len(limitedEnd)), promptLine: promptLine,
			timeout: 20 * time.Second, wantOutcome: OutcomeTimeout, wantTyped: []string{promptLine}, wantMarkCleared: true,
		},
		{
			name:      "ARecordWhoseOutputFilesAllExistEndsDoneNotDied",
			markedAgo: 5 * time.Minute, strand: heldPaneless, outputExists: true, timeout: time.Minute,
			wantOutcome: OutcomeDone, wantRemoved: true,
		},
		{
			name:      "AWaitBesideAMarkWithinItsBoundKeepsWaitingThroughStatusRetriesThenEndsDiedOnceItsBoundPasses",
			markedAgo: 0, strand: heldPaneless, timeout: 10 * time.Minute,
			wantOutcome: OutcomeDied, wantRemoved: true, wantElapsed: bound,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			reed := &echoReed{fakeReed: &fakeReed{StatusQueue: []reedengine.StatusResult{{Strands: []reedengine.StrandStatus{tt.strand}}}}, pane: "❯ "}
			engine := &sessionFakeEngine{waitingEngine: waitingEngine{fakeEngine: fakeEngine{StartupScript: []StartupState{StartupReady}}}, liveness: LivenessAlive, usageLimitLine: "STOP:limit"}
			clock := newFakeClock(time.Now())
			fx := newFixture(t, reed, engine, withConfig(fastConfig), withSeparateRunDir(), withClock(clock))
			var notices []string
			fx.Runner.SetNotifier(func(line string) error {
				notices = append(notices, line)
				return nil
			})
			seedPresentReedState(t, fx.DotLym)
			outputFile := filepath.Join(fx.RunRoot, "out.md")
			if tt.outputExists {
				touchOutputFile(t, outputFile)
			}
			runDir := seedAttachRun(t, fx.RunRoot, "run-1", seedAttachRunOpts{
				strandGUID: "strand-1", sessionID: "session-1", outputFiles: []string{outputFile}, outcome: runOutcomeRunning, includeOutcome: true,
				started: true, promptLine: tt.promptLine, promptOffset: tt.promptOffset,
				relaunchingAt: clock.Now().Add(-tt.markedAgo).UTC().Format(time.RFC3339),
			})
			if err := os.WriteFile(filepath.Join(runDir, eventsFileName), []byte(tt.events), 0o644); err != nil {
				t.Fatalf("seed events: %v", err)
			}

			start := clock.Now()
			result, found, err := fx.Runner.Attach(Spec{OutputFiles: []string{outputFile}, Timeout: tt.timeout})
			if err != nil || !found {
				t.Fatalf("Attach() = (_, %v, %v); want found and no error", found, err)
			}

			if result.Outcome != tt.wantOutcome {
				t.Errorf("Attach() outcome = %q; want %q", result.Outcome, tt.wantOutcome)
			}
			if removed := len(reed.RemoveStrandCalls) > 0; removed != tt.wantRemoved {
				t.Errorf("strand removed = %v; want %v", removed, tt.wantRemoved)
			}
			if !slices.Equal(reed.typed, tt.wantTyped) {
				t.Errorf("Attach() typed %q; want %q", reed.typed, tt.wantTyped)
			}
			if elapsed := clock.Now().Sub(start); elapsed < tt.wantElapsed {
				t.Errorf("Attach() took %v of run-clock time; want at least %v", elapsed, tt.wantElapsed)
			}
			if len(notices) != 0 {
				t.Errorf("notices = %q; want no hold notice for the limited turn end the record's new launch lies past", notices)
			}
			if tt.wantMarkCleared {
				record, found, err := loadRunState(runDir)
				if err != nil || !found || record.RelaunchingAt != "" || record.SessionID != "session-1" || record.PromptOffset != tt.promptOffset {
					t.Errorf("record = %+v, found %v, error %v; want the mark cleared and the new launch kept", record, found, err)
				}
			}
		})
	}
}
