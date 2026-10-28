//go:build tier0

// carryover_test.go covers the carry-over of operator instructions into a re-spawned run: the sent-texts record a verified send keeps, and the predecessor a run start reads instructions from.

package shuttleengine

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/testkit/logcapture"
)

// instructionEngine is a fakeEngine whose session signals read each "PROMPT:<text>" line as a turn start carrying <text> as its instruction.
type instructionEngine struct {
	*fakeEngine
}

func (e instructionEngine) ParseSessionSignals(data []byte) ([]SessionSignal, int) {
	var signals []SessionSignal
	for _, line := range strings.Split(string(data), "\n") {
		if text, ok := strings.CutPrefix(line, "PROMPT:"); ok {
			signals = append(signals, SessionSignal{Kind: SessionSignalTurnStart, Instruction: text})
		}
	}
	return signals, len(data)
}

// predecessorEvents is the events file of a seeded predecessor: a skill-load turn start before the prompt offset, the prompt, an operator instruction, a text a send typed and a repeat of the instruction.
const predecessorEvents = "PROMPT:skill load\nPROMPT:the prompt\nPROMPT:also fix the README\nPROMPT:a parent send\nPROMPT:also fix the README\n"

// seedPredecessor writes the run record of an earlier run into the run root of fx, with events as its events file and its prompt offset past the skill-load turn start.
// The run's directory is named after its RunID, and the strand of its record after the same ID.
// The sent text "a parent send" is recorded as a send into it.
func seedPredecessor(t *testing.T, fx *fixture, rs RunState, events string) {
	t.Helper()
	dir := filepath.Join(fx.RunRoot, rs.RunID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	rs.StrandGUID = "strand-" + rs.RunID
	rs.EventsPath = filepath.Join(dir, eventsFileName)
	rs.PromptOffset = int64(len("PROMPT:skill load\n"))
	if err := saveRunState(dir, rs); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(rs.EventsPath, []byte(events), 0o644); err != nil {
		t.Fatal(err)
	}
	recordSentText(dir, "a parent send")
}

// TestStart_CarriesOverOperatorInstructions pins which earlier run of a role a start carries instructions over from, and what it carries.
// The predecessor's prompt turn start is no operator instruction, and neither is a text its sends typed.
func TestStart_CarriesOverOperatorInstructions(t *testing.T) {
	t.Parallel()
	earlier := time.Date(2026, 10, 28, 8, 0, 0, 0, time.UTC).Format(time.RFC3339)
	later := time.Date(2026, 10, 28, 9, 0, 0, 0, time.UTC).Format(time.RFC3339)
	tests := []struct {
		name string
		// predecessors are the earlier runs' records, each with predecessorEvents unless events overrides it.
		predecessors []RunState
		events       string
		role         string
		want         []string
	}{
		{
			name:         "a stopped run of the role carries its own carried and operator instructions",
			predecessors: []RunState{{RunID: "run-old", CreatedAt: earlier, Outcome: runOutcomeStopped, RoleKey: "darn", CarriedInstructions: []string{"from an earlier session"}}},
			role:         "darn",
			want:         []string{"from an earlier session", "also fix the README"},
		},
		{
			name:         "a died run counts as unfinished",
			predecessors: []RunState{{RunID: "run-old", CreatedAt: earlier, Outcome: string(OutcomeDied), RoleKey: "darn"}},
			role:         "darn",
			want:         []string{"also fix the README"},
		},
		{
			name:         "a timed-out run counts as unfinished",
			predecessors: []RunState{{RunID: "run-old", CreatedAt: earlier, Outcome: string(OutcomeTimeout), RoleKey: "darn"}},
			role:         "darn",
			want:         []string{"also fix the README"},
		},
		{
			name:         "a running run whose waiter is gone counts as unfinished",
			predecessors: []RunState{{RunID: "run-old", CreatedAt: earlier, Outcome: runOutcomeRunning, PID: -1, RoleKey: "darn"}},
			role:         "darn",
			want:         []string{"also fix the README"},
		},
		{
			name:         "a done run carries nothing",
			predecessors: []RunState{{RunID: "run-old", CreatedAt: earlier, Outcome: string(OutcomeDone), RoleKey: "darn"}},
			role:         "darn",
		},
		{
			name:         "a running run with a live waiter carries nothing",
			predecessors: []RunState{{RunID: "run-old", CreatedAt: earlier, Outcome: runOutcomeRunning, PID: os.Getpid(), RoleKey: "darn"}},
			role:         "darn",
		},
		{
			name:         "a run of another role carries nothing",
			predecessors: []RunState{{RunID: "run-old", CreatedAt: earlier, Outcome: runOutcomeStopped, RoleKey: "review"}},
			role:         "darn",
		},
		{
			name:         "a spec with no role carries nothing",
			predecessors: []RunState{{RunID: "run-old", CreatedAt: earlier, Outcome: runOutcomeStopped}},
			role:         "",
		},
		{
			name: "a done run newer than an unfinished one hides it",
			predecessors: []RunState{
				{RunID: "run-old", CreatedAt: earlier, Outcome: runOutcomeStopped, RoleKey: "darn"},
				{RunID: "run-new", CreatedAt: later, Outcome: string(OutcomeDone), RoleKey: "darn"},
			},
			role: "darn",
		},
		{
			name: "an unfinished run newer than a done one is the one carried from",
			predecessors: []RunState{
				{RunID: "run-old", CreatedAt: earlier, Outcome: string(OutcomeDone), RoleKey: "darn"},
				{RunID: "run-new", CreatedAt: later, Outcome: runOutcomeStopped, RoleKey: "darn"},
			},
			role: "darn",
			want: []string{"also fix the README"},
		},
		{
			name:         "a run that died before its deferred prompt landed keeps its first instruction",
			predecessors: []RunState{{RunID: "run-old", CreatedAt: earlier, Outcome: string(OutcomeDied), RoleKey: "darn", PromptLine: "read the prompt file"}},
			events:       "PROMPT:skill load\nPROMPT:fix the typo\n",
			role:         "darn",
			want:         []string{"fix the typo"},
		},
		{
			name:         "a delivered deferred prompt is no operator instruction",
			predecessors: []RunState{{RunID: "run-old", CreatedAt: earlier, Outcome: runOutcomeStopped, RoleKey: "darn", PromptLine: "read the prompt file"}},
			events:       "PROMPT:skill load\nPROMPT:read the prompt file\nPROMPT:fix the typo\n",
			role:         "darn",
			want:         []string{"fix the typo"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			reed := &fakeReed{AddStrandResult: reedengine.Strand{GUID: "strand-1", Name: "hub:task:darn"}}
			engine := &fakeEngine{PrepareLaunch: Launch{Cmd: "cmd", SessionID: "session-1"}}
			readyStart(reed, engine)
			fx := newFixture(t, reed, instructionEngine{engine}, withConfig(fastConfig))
			var notices []string
			fx.Runner.SetNotifier(func(line string) error {
				notices = append(notices, line)
				return nil
			})
			events := predecessorEvents
			if tt.events != "" {
				events = tt.events
			}
			for _, predecessor := range tt.predecessors {
				seedPredecessor(t, fx, predecessor, events)
			}

			outputFile := filepath.Join(t.TempDir(), "out.md")
			run, err := fx.Runner.Start(Spec{Prompt: "do the task", OutputFiles: []string{outputFile}, Role: tt.role})
			if err != nil {
				t.Fatalf("Start() error = %v; want nil", err)
			}

			wantPrompt := "do the task"
			if len(tt.want) > 0 {
				wantPrompt += carryOverBlock(tt.want)
			}
			if len(engine.PrepareCalls) != 1 || engine.PrepareCalls[0].Spec.Prompt != wantPrompt {
				t.Errorf("prepared prompts = %+v; want one prompt %q", engine.PrepareCalls, wantPrompt)
			}
			record, _, err := loadRunState(run.RunDir())
			if err != nil {
				t.Fatal(err)
			}
			if record.RoleKey != tt.role || !reflect.DeepEqual(record.CarriedInstructions, tt.want) {
				t.Errorf("new record RoleKey, CarriedInstructions = %q, %q; want %q, %q", record.RoleKey, record.CarriedInstructions, tt.role, tt.want)
			}
			wantNotices := 0
			if len(tt.want) > 0 {
				wantNotices = 1
			}
			if len(notices) != wantNotices {
				t.Fatalf("notices = %q; want %d", notices, wantNotices)
			}
			if wantNotices == 1 && !strings.Contains(notices[0], strings.TrimSpace(tt.want[len(tt.want)-1])) {
				t.Errorf("notice = %q; want it to quote the carried instruction %q", notices[0], tt.want[len(tt.want)-1])
			}
		})
	}
}

// TestStart_CarryOverSurvivesAStartRetry pins that a start whose first launch died carries the instructions into the prompt of the launch that succeeds, sends one notice after that success, and logs the carry-over once.
// It captures the process-global logger, so it runs serially.
func TestStart_CarryOverSurvivesAStartRetry(t *testing.T) {
	buf := logcapture.CaptureVerbose(t)
	logger.SetVerbosity(2)
	live := reedengine.StatusResult{Strands: []reedengine.StrandStatus{{GUID: "strand-1", PaneID: "%1", Live: true}}}
	dead := reedengine.StatusResult{Strands: []reedengine.StrandStatus{{GUID: "strand-1", PaneID: "%1", Live: false}}}
	reed := &fakeReed{
		StatusQueue:  []reedengine.StatusResult{live, dead, live},
		CaptureQueue: []string{"$ claude ...", "❯ "},
	}
	engine := &fakeEngine{StartupScript: []StartupState{StartupPending, StartupReady}}
	seedStartupFakes(reed, engine)
	cfg := fastConfig
	cfg.StartRetries = 1
	clock := &awaitClock{fakeClock: newFakeClock(time.Now())}
	fx := newFixture(t, reed, instructionEngine{engine}, withConfig(cfg), withClock(clock))
	var notices []string
	fx.Runner.SetNotifier(func(line string) error {
		notices = append(notices, line)
		return nil
	})
	seedPredecessor(t, fx, RunState{RunID: "run-old", CreatedAt: time.Date(2026, 10, 28, 8, 0, 0, 0, time.UTC).Format(time.RFC3339), Outcome: runOutcomeStopped, RoleKey: "darn"}, predecessorEvents)

	run, err := fx.Runner.Start(Spec{Prompt: "do the task", OutputFiles: []string{filepath.Join(t.TempDir(), "out.md")}, Role: "darn"})

	if err != nil || run == nil {
		t.Fatalf("Start() = %v, %v; want the ready run", run, err)
	}
	wantPrompt := "do the task" + carryOverBlock([]string{"also fix the README"})
	if len(engine.PrepareCalls) != 2 {
		t.Fatalf("prepared launches = %d; want 2, the died one and the retry", len(engine.PrepareCalls))
	}
	for i, call := range engine.PrepareCalls {
		if call.Spec.Prompt != wantPrompt {
			t.Errorf("prompt of launch %d = %q; want %q", i+1, call.Spec.Prompt, wantPrompt)
		}
	}
	if len(notices) != 1 {
		t.Errorf("notices = %q; want one, after the launch that succeeded", notices)
	}
	if got := countLevelLines(buf.String(), "WARN", "operator instructions carried over into a re-spawned run"); got != 1 {
		t.Errorf("WARN carry-over lines = %d; want 1; log: %s", got, buf.String())
	}
}

// TestRun_Send_RecordsTheSentText pins that a verified send into a run with a directory appends its text to the run's sent-texts file.
func TestRun_Send_RecordsTheSentText(t *testing.T) {
	t.Parallel()
	reed := &fakeReed{StatusQueue: liveStrandStatus(true), CaptureQueue: []string{"❯ ", "❯ ", "❯ updated instructions"}}
	clock := newFakeClock(time.Date(2026, 10, 28, 8, 0, 0, 0, time.UTC))
	runDir := t.TempDir()
	run := newFixture(t, reed, readyAgentEngine(), withConfig(Config{})).newRun(Spec{}, withRunDir(runDir), withRunClock(clock, clock.Now().Add(time.Hour)))

	if err := run.Send("updated instructions"); err != nil {
		t.Fatalf("Send() error = %v; want nil", err)
	}

	got, err := os.ReadFile(filepath.Join(runDir, sentTextsFileName))
	if err != nil {
		t.Fatal(err)
	}
	if want := "\"updated instructions\"\n"; string(got) != want {
		t.Errorf("sent-texts file = %q; want %q", got, want)
	}
}

// TestRun_Send_SucceedsWhenTheSentTextCannotBeRecorded pins that a run directory the sent-texts file cannot be written in does not fail the send, and that the failure is logged once.
// It captures the process-global logger, so it runs serially.
func TestRun_Send_SucceedsWhenTheSentTextCannotBeRecorded(t *testing.T) {
	buf := logcapture.CaptureVerbose(t)
	logger.SetVerbosity(2)
	reed := &fakeReed{StatusQueue: liveStrandStatus(true), CaptureQueue: []string{"❯ ", "❯ ", "❯ updated instructions"}}
	clock := newFakeClock(time.Date(2026, 10, 28, 8, 0, 0, 0, time.UTC))
	runDir := filepath.Join(t.TempDir(), "missing")
	run := newFixture(t, reed, readyAgentEngine(), withConfig(Config{})).newRun(Spec{}, withRunDir(runDir), withRunClock(clock, clock.Now().Add(time.Hour)))

	if err := run.Send("updated instructions"); err != nil {
		t.Fatalf("Send() error = %v; want nil", err)
	}

	if _, err := os.Stat(filepath.Join(runDir, sentTextsFileName)); !os.IsNotExist(err) {
		t.Errorf("sent-texts file stat error = %v; want it absent", err)
	}
	if got := countLevelLines(buf.String(), "WARN", "sent text not recorded"); got != 1 {
		t.Errorf("WARN lines of %q = %d; want 1; log: %s", "sent text not recorded", got, buf.String())
	}
}
