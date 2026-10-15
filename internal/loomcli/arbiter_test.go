package loomcli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/arbiterstop"
	"github.com/Knatte18/loomyard/internal/battenshed"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/shedadapters"
	"github.com/Knatte18/loomyard/internal/sheddriver"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedverbs"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// arbiterFake is a scripted arbiterShuttle: it records every call and runs the scripted hooks inside Wait and AttachIfLive, where the real arbiter would write its verdict.
type arbiterFake struct {
	starts       []shuttleengine.Spec
	attaches     []shuttleengine.Spec
	stoppedGUIDs []string
	runStops     int

	startErr   error
	runGUID    string
	onWait     func(spec shuttleengine.Spec)
	waitResult shuttleengine.Result
	waitErr    error

	attachFound  bool
	attachErr    error
	attachResult shuttleengine.Result
	onAttach     func(spec shuttleengine.Spec)
}

func (f *arbiterFake) Start(spec shuttleengine.Spec) (arbiterRun, error) {
	if f.startErr != nil {
		return nil, f.startErr
	}
	f.starts = append(f.starts, spec)
	return &arbiterFakeRun{fake: f, spec: spec}, nil
}

func (f *arbiterFake) AttachIfLive(spec shuttleengine.Spec) (shuttleengine.Result, bool, error) {
	f.attaches = append(f.attaches, spec)
	if f.onAttach != nil {
		f.onAttach(spec)
	}
	return f.attachResult, f.attachFound, f.attachErr
}

func (f *arbiterFake) StopStrand(guid string) error {
	f.stoppedGUIDs = append(f.stoppedGUIDs, guid)
	return nil
}

type arbiterFakeRun struct {
	fake *arbiterFake
	spec shuttleengine.Spec
}

func (r *arbiterFakeRun) Wait() (shuttleengine.Result, error) {
	if r.fake.onWait != nil {
		r.fake.onWait(r.spec)
	}
	result := r.fake.waitResult
	if result.Outcome == "" {
		result.Outcome = shuttleengine.OutcomeDone
	}
	return result, r.fake.waitErr
}

func (r *arbiterFakeRun) Stop() error {
	r.fake.runStops++
	return nil
}

func (r *arbiterFakeRun) StrandGUID() string { return r.fake.runGUID }

// arbiterFixture is one temp run directory with the arbiter verb's seams over it.
type arbiterFixture struct {
	t    *testing.T
	root string
	now  time.Time

	status           shedengine.Status
	statusMissing    bool
	statusErr        error
	stopEnvelope     string
	escalationBrief  string
	escalationOpen   bool
	escalationCause  shedadapters.EscalationCause
	escalationErr    error
	circlingPending  bool
	verifyRecord     *battenshed.ChildPublishFailure
	head             string
	settings         loomengine.ArbiterSettings
	fake             *arbiterFake
	envelopePath     string
	removeEnvelope   bool
	reportPath       string
	caseBriefPath    string
	interruptPolicy  map[string]string
	producerRowNames []string
}

const arbiterTestRun = "operator-surface"

func newArbiterFixture(t *testing.T, producer string, state shedengine.State) *arbiterFixture {
	t.Helper()
	root := t.TempDir()
	f := &arbiterFixture{
		t:    t,
		root: root,
		now:  time.Date(2026, 10, 12, 12, 0, 0, 0, time.UTC),
		status: shedengine.Status{
			CurrentProducer: producer,
			State:           state,
			History:         []shedengine.HistoryEntry{{Producer: "Plan-Write", Outcome: shedengine.Done}, {Producer: "Plan-Bouncer", Outcome: shedengine.Done}},
		},
		stopEnvelope:     `{"ok":false,"kind":"","transient":""}`,
		escalationCause:  shedadapters.EscalationCircling,
		settings:         loomengine.ArbiterSettings{Model: "claude-resolved", Effort: "high", Version: "v1", Timeout: 30 * time.Minute, MaxSpawns: 3},
		fake:             &arbiterFake{runGUID: "guid-1"},
		interruptPolicy:  map[string]string{},
		producerRowNames: []string{"Plan-Write", "Plan-Bouncer", loomshed.NamePublish, "PR-Gate"},
	}
	f.envelopePath = filepath.Join(root, "stop-envelope.json")
	f.reportPath = filepath.Join(root, "report.md")
	return f
}

// haltAtVerifyRow stops the run at row with a Stuck entry at 11:30 after an entry at 11:00, and leaves a verify record naming a target at HEAD, written at writtenAt.
func (f *arbiterFixture) haltAtVerifyRow(row, writtenAt string) {
	f.head = "head-1"
	f.status.History = []shedengine.HistoryEntry{
		{Producer: "Plan-Bouncer", Outcome: shedengine.Done, At: "2026-10-12T11:00:00Z"},
		{Producer: row, Outcome: shedengine.Stuck, At: "2026-10-12T11:30:00Z"},
	}
	f.verifyRecord = &battenshed.ChildPublishFailure{Row: row, Head: f.head, Target: "Webster-Burler-Review", WrittenAt: writtenAt}
}

func (f *arbiterFixture) arbiterDir() string { return filepath.Join(f.root, "arbiter") }
func (f *arbiterFixture) markerPath() string { return filepath.Join(f.root, "scratch", "arbiter.json") }
func (f *arbiterFixture) attemptDir(n int) string {
	return filepath.Join(f.arbiterDir(), "attempt-"+string(rune('0'+n)))
}

func (f *arbiterFixture) deps() arbiterDeps {
	producers := make([]shedengine.ProducerDef, len(f.producerRowNames))
	for i, name := range f.producerRowNames {
		producers[i] = shedengine.ProducerDef{Name: name}
	}
	return arbiterDeps{
		ReadStatus: func() (shedengine.Status, bool, error) {
			if f.statusErr != nil {
				return shedengine.Status{}, false, f.statusErr
			}
			return f.status, !f.statusMissing, nil
		},
		Settings:   f.settings,
		ArbiterDir: f.arbiterDir(),
		AttemptDir: f.attemptDir,
		MarkerPath: f.markerPath(),
		Producers:  producers,
		Stop: arbiterstop.Readers{
			PendingEscalation: func(string) (shedadapters.EscalationCause, string, bool, error) {
				if f.escalationErr != nil {
					return "", "", false, f.escalationErr
				}
				return f.escalationCause, f.escalationBrief, f.escalationOpen, nil
			},
			VerifyRecord: func() (battenshed.ChildPublishFailure, bool, error) {
				if f.verifyRecord == nil {
					return battenshed.ChildPublishFailure{}, false, nil
				}
				return *f.verifyRecord, true, nil
			},
			Head:            func() (string, error) { return f.head, nil },
			InterruptPolicy: func(producer string) string { return f.interruptPolicy[producer] },
		},
		PendingCirclingDecision: func(string) (bool, error) { return f.circlingPending, nil },
		Prompt: arbiterPromptInput{
			Slug:               "operator-surface-task",
			Worktree:           f.root,
			DecisionRecordPath: filepath.Join(f.root, "decision-record.md"),
			SupportLogPath:     filepath.Join(f.root, "support-log.md"),
		},
		StencilsDir:   seededStencils(f.t),
		Shuttle:       f.fake,
		Now:           func() time.Time { return f.now },
		RelaunchRunID: arbiterTestRun,
	}
}

// run writes the stop envelope and runs the verb, returning its exit code and decoded output.
func (f *arbiterFixture) run() (int, map[string]any) {
	f.t.Helper()
	if !f.removeEnvelope {
		if err := os.WriteFile(f.envelopePath, []byte(f.stopEnvelope), 0o644); err != nil {
			f.t.Fatal(err)
		}
	}
	var out bytes.Buffer
	code := arbiterVerb(&out, f.deps(), arbiterInput{RunID: arbiterTestRun, EnvelopePath: f.envelopePath, ReportPath: f.reportPath, CaseBriefPath: f.caseBriefPath})
	var fields map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(out.Bytes()), &fields); err != nil {
		f.t.Fatalf("arbiterVerb output %q is not one JSON object: %v", out.String(), err)
	}
	return code, fields
}

// priorAttempt writes an attempt for the fixture's current stop, with its deadline offset from now.
func (f *arbiterFixture) priorAttempt(number int, deadlineIn time.Duration) loomengine.ArbiterAttempt {
	f.t.Helper()
	attempt := loomengine.ArbiterAttempt{
		Number:     number,
		StopKey:    sheddriver.StopKeyOf(f.status),
		SpawnedAt:  f.now.Add(-10 * time.Minute),
		Deadline:   f.now.Add(deadlineIn),
		StrandGUID: "old-guid",
	}
	if err := loomengine.WriteArbiterAttempt(f.attemptDir(number), attempt); err != nil {
		f.t.Fatal(err)
	}
	attempt.Dir = f.attemptDir(number)
	return attempt
}

func writeArbiterVerdictFile(t *testing.T, dir, text string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(loomengine.ArbiterVerdictPath(dir), []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

const (
	arbiterResumeVerdict = "verdict: resume\nreason: the repair is safe\nverbs:\n  - lyx loom circling accept operator-surface-task\n"
	arbiterHandOnVerdict = "verdict: hand-on\nreason: the design is wrong\nquestion: should the parent widen the task?\n"
)

// TestArbiterVerb_SpawnPath covers the verb for a stop no attempt has arbitrated yet, including card 9's eligibility list.
// Each eligible stop spawns exactly one run whose spec, attempt record, prompt and marker carry the expected values; every refused stop spawns nothing and writes no attempt directory.
func TestArbiterVerb_SpawnPath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		producer    string
		state       shedengine.State
		envelope    string
		setup       func(f *arbiterFixture)
		wantBrief   bool
		wantRefusal string
		wantNoSpawn bool
		// keepsArbiterDir is set for the row that seeds the attempt records it is refused over.
		keepsArbiterDir bool
	}{
		{name: "producer error", producer: "Plan-Write", state: shedengine.StateFailed, envelope: `{"kind":"producer"}`},
		{name: "bootstrap error", producer: "Plan-Write", state: shedengine.StateFailed, envelope: `{"kind":"bootstrap"}`},
		{
			name: "interrupted under reinvoke", producer: "Plan-Write", state: shedengine.StateFailed, envelope: `{"kind":"interrupted"}`,
			setup: func(f *arbiterFixture) { f.interruptPolicy["Plan-Write"] = shedverbs.InterruptPolicyReinvoke },
		},
		{name: "transient failure that failed twice", producer: "Plan-Write", state: shedengine.StateFailed, envelope: `{"kind":"busy","transient":"network"}`},
		{name: "blocked stop", producer: "Plan-Write", state: shedengine.StateBlocked},
		{
			name: "blocked stop carrying a tackler case brief", producer: "Plan-Write", state: shedengine.StateBlocked,
			setup: func(f *arbiterFixture) {
				f.caseBriefPath = filepath.Join(f.root, "tackler", "attempt-1", "case-brief.md")
			},
		},
		{
			name: "Publish halt naming no target", producer: loomshed.NamePublish, state: shedengine.StateBlocked,
		},
		{
			name: "awaiting at a Bouncer row with a pending escalation", producer: "Plan-Bouncer", state: shedengine.StateAwaiting, wantBrief: true,
			setup: func(f *arbiterFixture) {
				f.escalationOpen = true
				f.escalationBrief = filepath.Join(f.root, "round-2-escalation.md")
			},
		},
		{
			name: "awaiting at a hard-cap escalation", producer: "Plan-Bouncer", state: shedengine.StateAwaiting, wantBrief: true,
			setup: func(f *arbiterFixture) {
				f.escalationOpen = true
				f.escalationCause = shedadapters.EscalationHardCap
				f.escalationBrief = filepath.Join(f.root, "round-2-escalation.md")
			},
		},
		{
			name: "awaiting at a budget escalation refused", producer: "Plan-Bouncer", state: shedengine.StateAwaiting,
			setup: func(f *arbiterFixture) {
				f.escalationOpen = true
				f.escalationCause = shedadapters.EscalationBudget
			},
			wantRefusal: "has cause \"budget\"", wantNoSpawn: true,
		},
		{
			name: "awaiting with an unreadable escalation record refused", producer: "Plan-Bouncer", state: shedengine.StateAwaiting,
			setup: func(f *arbiterFixture) {
				f.escalationErr = fmt.Errorf("record: %w", shedadapters.ErrEscalationMalformed)
			},
			wantRefusal: "unreadable", wantNoSpawn: true,
		},
		{name: "busy refused", producer: "Plan-Write", state: shedengine.StateFailed, envelope: `{"kind":"busy"}`, wantRefusal: "an error of kind \"busy\"", wantNoSpawn: true},
		{name: "ownership refused", producer: "Plan-Write", state: shedengine.StateFailed, envelope: `{"kind":"ownership"}`, wantRefusal: "an error of kind \"ownership\"", wantNoSpawn: true},
		{name: "unseeded refused", producer: "Plan-Write", state: shedengine.StateFailed, envelope: `{"kind":"unseeded"}`, wantRefusal: "an error of kind \"unseeded\"", wantNoSpawn: true},
		{name: "error envelope with no kind refused", producer: "Plan-Write", state: shedengine.StateFailed, wantRefusal: "carries no kind", wantNoSpawn: true},
		{
			name: "interrupted under handback refused", producer: "Plan-Write", state: shedengine.StateFailed, envelope: `{"kind":"interrupted"}`,
			setup:       func(f *arbiterFixture) { f.interruptPolicy["Plan-Write"] = shedverbs.InterruptPolicyHandback },
			wantRefusal: "whose policy hands back", wantNoSpawn: true,
		},
		{
			name: "Publish halt naming a target refused", producer: loomshed.NamePublish, state: shedengine.StateBlocked,
			setup:       func(f *arbiterFixture) { f.haltAtVerifyRow(loomshed.NamePublish, "2026-10-12T11:15:00Z") },
			wantRefusal: "verify halt whose failure record names a target", wantNoSpawn: true,
		},
		{
			name: "Merge-In verify halt naming a target refused", producer: loomshed.NameMergeIn, state: shedengine.StateBlocked,
			setup:       func(f *arbiterFixture) { f.haltAtVerifyRow(loomshed.NameMergeIn, "2026-10-12T11:15:00Z") },
			wantRefusal: "verify halt whose failure record names a target", wantNoSpawn: true,
		},
		{
			name: "Merge-In stop of another cause with an earlier verify's record stays eligible", producer: loomshed.NameMergeIn, state: shedengine.StateBlocked,
			setup: func(f *arbiterFixture) { f.haltAtVerifyRow(loomshed.NameMergeIn, "2026-10-12T11:00:00Z") },
		},
		{
			name: "Publish stop of another cause with an earlier verify's record is eligible", producer: loomshed.NamePublish, state: shedengine.StateBlocked,
			setup: func(f *arbiterFixture) { f.haltAtVerifyRow(loomshed.NamePublish, "2026-10-12T11:00:00Z") },
		},
		{
			name: "blocked stop at another row with a record naming a target stays eligible", producer: "Plan-Write", state: shedengine.StateBlocked,
			setup: func(f *arbiterFixture) { f.haltAtVerifyRow("Plan-Write", "2026-10-12T11:15:00Z") },
		},
		{name: "awaiting with a parent notice but no escalation refused", producer: "PR-Gate", state: shedengine.StateAwaiting, wantRefusal: "no pending review escalation", wantNoSpawn: true},
		{name: "paused refused", producer: "Plan-Write", state: shedengine.StatePaused, wantRefusal: "a paused run is not a halted stop", wantNoSpawn: true},
		{name: "done refused", producer: "Plan-Write", state: shedengine.StateDone, wantRefusal: "a done run is not a halted stop", wantNoSpawn: true},
		{name: "running refused", producer: "Plan-Write", state: shedengine.StateRunning, wantRefusal: "a running run is not a halted stop", wantNoSpawn: true},
		{name: "invalid state refused", producer: "Plan-Write", state: "bogus", wantRefusal: "not a halted stop", wantNoSpawn: true},
		{
			name: "missing status file refused", producer: "Plan-Write", state: shedengine.StateBlocked,
			setup:       func(f *arbiterFixture) { f.statusMissing = true },
			wantRefusal: "status or envelope unreadable", wantNoSpawn: true,
		},
		{
			name: "status that does not parse refused", producer: "Plan-Write", state: shedengine.StateBlocked,
			setup:       func(f *arbiterFixture) { f.statusErr = errors.New("invalid character") },
			wantRefusal: "status or envelope unreadable", wantNoSpawn: true,
		},
		{
			name: "unreadable envelope path refused", producer: "Plan-Write", state: shedengine.StateBlocked,
			setup:       func(f *arbiterFixture) { f.removeEnvelope = true },
			wantRefusal: "status or envelope unreadable", wantNoSpawn: true,
		},
		{
			name: "budget of 0 refused", producer: "Plan-Write", state: shedengine.StateBlocked,
			setup:       func(f *arbiterFixture) { f.settings.MaxSpawns = 0 },
			wantRefusal: "arbiter: off", wantNoSpawn: true,
		},
		{
			name: "spent budget refused, counted from the attempt records on disk", producer: "Plan-Write", state: shedengine.StateBlocked,
			setup: func(f *arbiterFixture) {
				f.settings.MaxSpawns = 1
				other := loomengine.ArbiterAttempt{Number: 1, StopKey: sheddriver.StopKey{CurrentProducer: "Plan-Bouncer", HistoryLength: 1, State: "awaiting"}}
				if err := loomengine.WriteArbiterAttempt(f.attemptDir(1), other); err != nil {
					t.Fatal(err)
				}
			},
			wantRefusal: "spawn budget spent", wantNoSpawn: true, keepsArbiterDir: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newArbiterFixture(t, tt.producer, tt.state)
			if tt.envelope != "" {
				f.stopEnvelope = tt.envelope
			}
			if tt.setup != nil {
				tt.setup(f)
			}
			var captured struct {
				marker  loomengine.ArbiterMarker
				attempt loomengine.ArbiterAttempt
				prompt  string
			}
			f.fake.onWait = func(spec shuttleengine.Spec) {
				captured.marker, _, _ = loomengine.ReadArbiterMarker(f.markerPath())
				captured.attempt, _ = loomengine.ReadArbiterAttempt(f.attemptDir(1))
				data, _ := os.ReadFile(loomengine.ArbiterPromptPath(f.attemptDir(1)))
				captured.prompt = string(data)
				writeArbiterVerdictFile(t, f.attemptDir(1), arbiterHandOnVerdict)
			}

			code, fields := f.run()

			if tt.wantNoSpawn {
				if code == 0 {
					t.Fatalf("arbiterVerb() exit = 0, output %v; want a refusal containing %q", fields, tt.wantRefusal)
				}
				message, _ := fields["error"].(string)
				if !strings.Contains(message, tt.wantRefusal) || !strings.Contains(message, "way forward:") {
					t.Errorf("refusal = %q; want it to contain %q and a way forward", message, tt.wantRefusal)
				}
				if len(f.fake.starts) != 0 {
					t.Errorf("starts = %d; want none", len(f.fake.starts))
				}
				if _, err := os.Stat(f.attemptDir(1)); !tt.keepsArbiterDir && !errors.Is(err, os.ErrNotExist) {
					t.Errorf("attempt directory exists after a refusal (stat error %v); want none", err)
				}
				if _, err := os.Stat(f.attemptDir(2)); !errors.Is(err, os.ErrNotExist) {
					t.Errorf("a second attempt directory exists after a refusal (stat error %v); want none", err)
				}
				return
			}

			if code != 0 {
				t.Fatalf("arbiterVerb() exit = %d, output %v; want 0", code, fields)
			}
			if len(f.fake.starts) != 1 {
				t.Fatalf("starts = %d; want exactly one", len(f.fake.starts))
			}
			spec := f.fake.starts[0]
			verdictPath := loomengine.ArbiterVerdictPath(f.attemptDir(1))
			if spec.Role != loomengine.ArbiterRole || !reflect.DeepEqual(spec.OutputFiles, []string{verdictPath}) || spec.Model != "claude-resolved" || spec.Timeout != 30*time.Minute {
				t.Errorf("spec = role %q, outputs %v, model %q, timeout %v; want the arbiter role over %q, the resolved model and 30m", spec.Role, spec.OutputFiles, spec.Model, spec.Timeout, verdictPath)
			}
			if captured.prompt != spec.Prompt || !strings.Contains(spec.Prompt, verdictPath) || !strings.Contains(spec.Prompt, f.envelopePath) {
				t.Errorf("prompt.md does not equal the spec prompt, or the prompt lacks the verdict and envelope paths")
			}
			if tt.wantBrief && !strings.Contains(spec.Prompt, f.escalationBrief) {
				t.Errorf("prompt lacks the escalation brief %q", f.escalationBrief)
			}
			if !tt.wantBrief && !strings.Contains(spec.Prompt, "escalation brief `none`") {
				t.Errorf("prompt reads a brief at a stop with no escalation; want none")
			}
			if f.caseBriefPath != "" && !strings.Contains(spec.Prompt, f.caseBriefPath) {
				t.Errorf("prompt lacks the case brief %q", f.caseBriefPath)
			}
			if f.caseBriefPath == "" && !strings.Contains(spec.Prompt, "case brief `none`") {
				t.Errorf("prompt reads a case brief at a stop given none; want none")
			}
			wantAttempt := loomengine.ArbiterAttempt{
				Number: 1, StopKey: sheddriver.StopKeyOf(f.status), SpawnedAt: f.now, Deadline: f.now.Add(30 * time.Minute),
				StrandGUID: "guid-1", EnvelopePath: f.envelopePath, ReportPath: f.reportPath, CaseBriefPath: f.caseBriefPath, Dir: f.attemptDir(1),
			}
			if !reflect.DeepEqual(captured.attempt, wantAttempt) {
				t.Errorf("attempt.json = %+v; want %+v", captured.attempt, wantAttempt)
			}
			if want := (loomengine.ArbiterMarker{Attempt: 1, StrandGUID: "guid-1", Deadline: f.now.Add(30 * time.Minute)}); captured.marker != want {
				t.Errorf("marker = %+v; want %+v", captured.marker, want)
			}
			if _, found, _ := loomengine.ReadArbiterMarker(f.markerPath()); found {
				t.Error("marker still present after the verb finished; want it removed")
			}
		})
	}
}

// TestArbiterVerb_Rerun covers the verb for a stop an attempt already arbitrated: it spawns nothing and returns or awaits that attempt's verdict.
func TestArbiterVerb_Rerun(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		setup           func(f *arbiterFixture, attempt loomengine.ArbiterAttempt)
		deadlineIn      time.Duration
		wantStarts      int
		wantAttaches    int
		wantStopped     []string
		wantVerdict     string
		wantCause       string
		wantRefusal     string
		wantAttachAfter time.Duration
	}{
		{
			name: "a live attempt is attached without starting", deadlineIn: 20 * time.Minute,
			setup: func(f *arbiterFixture, attempt loomengine.ArbiterAttempt) {
				marker := loomengine.ArbiterMarker{Attempt: 1, StrandGUID: "old-guid", Deadline: attempt.Deadline}
				if err := loomengine.WriteArbiterMarker(f.markerPath(), marker); err != nil {
					t.Fatal(err)
				}
				f.fake.attachFound = true
				f.fake.onAttach = func(shuttleengine.Spec) { writeArbiterVerdictFile(t, attempt.Dir, arbiterHandOnVerdict) }
			},
			wantAttaches: 1, wantAttachAfter: 20 * time.Minute, wantVerdict: loomengine.ArbiterVerdictHandOn,
		},
		{
			name: "a marker past its deadline stops the strand without attaching", deadlineIn: -time.Minute,
			setup: func(f *arbiterFixture, attempt loomengine.ArbiterAttempt) {
				marker := loomengine.ArbiterMarker{Attempt: 1, StrandGUID: "old-guid", Deadline: attempt.Deadline}
				if err := loomengine.WriteArbiterMarker(f.markerPath(), marker); err != nil {
					t.Fatal(err)
				}
			},
			wantStopped: []string{"old-guid"}, wantVerdict: loomengine.ArbiterVerdictHandOn, wantCause: arbiterCauseTimeout,
		},
		{
			name: "an attach error stops the strand and reads died", deadlineIn: 20 * time.Minute,
			setup: func(f *arbiterFixture, attempt loomengine.ArbiterAttempt) {
				marker := loomengine.ArbiterMarker{Attempt: 1, StrandGUID: "old-guid", Deadline: attempt.Deadline}
				if err := loomengine.WriteArbiterMarker(f.markerPath(), marker); err != nil {
					t.Fatal(err)
				}
				f.fake.attachErr = errors.New("reed unreachable")
			},
			wantAttaches: 1, wantAttachAfter: 20 * time.Minute, wantStopped: []string{"old-guid"}, wantVerdict: loomengine.ArbiterVerdictHandOn, wantCause: arbiterCauseDied,
		},
		{
			name: "a marked attempt with no live strand reads died", deadlineIn: 20 * time.Minute,
			setup: func(f *arbiterFixture, attempt loomengine.ArbiterAttempt) {
				marker := loomengine.ArbiterMarker{Attempt: 1, StrandGUID: "old-guid", Deadline: attempt.Deadline}
				if err := loomengine.WriteArbiterMarker(f.markerPath(), marker); err != nil {
					t.Fatal(err)
				}
			},
			wantAttaches: 1, wantAttachAfter: 20 * time.Minute, wantVerdict: loomengine.ArbiterVerdictHandOn, wantCause: arbiterCauseDied,
		},
		{
			name: "an attempt with no marker whose strand is live attaches and waits", deadlineIn: 5 * time.Minute,
			setup: func(f *arbiterFixture, attempt loomengine.ArbiterAttempt) {
				f.fake.attachFound = true
				f.fake.onAttach = func(shuttleengine.Spec) { writeArbiterVerdictFile(t, attempt.Dir, arbiterHandOnVerdict) }
			},
			wantAttaches: 1, wantAttachAfter: 5 * time.Minute, wantVerdict: loomengine.ArbiterVerdictHandOn,
		},
		{
			name: "an attempt with no marker, no live strand and no verdict reads missing", deadlineIn: 5 * time.Minute,
			setup:        func(f *arbiterFixture, attempt loomengine.ArbiterAttempt) {},
			wantAttaches: 1, wantAttachAfter: 5 * time.Minute, wantVerdict: loomengine.ArbiterVerdictHandOn, wantCause: arbiterCauseMissing,
		},
		{
			name: "an attempt past its deadline with no marker reads missing without attaching", deadlineIn: -time.Minute,
			setup:       func(f *arbiterFixture, attempt loomengine.ArbiterAttempt) {},
			wantVerdict: loomengine.ArbiterVerdictHandOn, wantCause: arbiterCauseMissing,
		},
		{
			name: "a finished unapplied verdict returns without starting", deadlineIn: -time.Minute,
			setup: func(f *arbiterFixture, attempt loomengine.ArbiterAttempt) {
				writeArbiterVerdictFile(t, attempt.Dir, arbiterHandOnVerdict)
			},
			wantVerdict: loomengine.ArbiterVerdictHandOn,
		},
		{
			name: "an applied resume is refused", deadlineIn: -time.Minute,
			setup: func(f *arbiterFixture, attempt loomengine.ArbiterAttempt) {
				writeArbiterVerdictFile(t, attempt.Dir, arbiterResumeVerdict)
				if err := loomengine.MarkArbiterApplied(attempt.Dir); err != nil {
					t.Fatal(err)
				}
			},
			wantRefusal: "an applied resume left the stop key unchanged",
		},
		{
			name: "an attempt for another stop key does not match", deadlineIn: 20 * time.Minute,
			setup: func(f *arbiterFixture, attempt loomengine.ArbiterAttempt) {
				attempt.StopKey.HistoryLength++
				if err := loomengine.WriteArbiterAttempt(attempt.Dir, attempt); err != nil {
					t.Fatal(err)
				}
				f.fake.onWait = func(shuttleengine.Spec) { writeArbiterVerdictFile(t, f.attemptDir(2), arbiterHandOnVerdict) }
			},
			wantStarts: 1, wantVerdict: loomengine.ArbiterVerdictHandOn,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newArbiterFixture(t, "Plan-Write", shedengine.StateBlocked)
			attempt := f.priorAttempt(1, tt.deadlineIn)
			tt.setup(f, attempt)

			code, fields := f.run()

			if tt.wantRefusal != "" {
				message, _ := fields["error"].(string)
				if code == 0 || !strings.Contains(message, tt.wantRefusal) || !strings.Contains(message, "way forward:") {
					t.Fatalf("arbiterVerb() = exit %d, %v; want a refusal containing %q and a way forward", code, fields, tt.wantRefusal)
				}
			} else {
				if code != 0 {
					t.Fatalf("arbiterVerb() exit = %d, output %v; want 0", code, fields)
				}
				if fields["verdict"] != tt.wantVerdict || fields["cause"] != tt.wantCause {
					t.Errorf("verdict, cause = %v, %v; want %q, %q", fields["verdict"], fields["cause"], tt.wantVerdict, tt.wantCause)
				}
			}
			if len(f.fake.starts) != tt.wantStarts {
				t.Errorf("starts = %d; want %d", len(f.fake.starts), tt.wantStarts)
			}
			if len(f.fake.attaches) != tt.wantAttaches {
				t.Fatalf("attaches = %d; want %d", len(f.fake.attaches), tt.wantAttaches)
			}
			if tt.wantAttaches > 0 && f.fake.attaches[0].Timeout != tt.wantAttachAfter {
				t.Errorf("attach timeout = %v; want the time left to the deadline, %v", f.fake.attaches[0].Timeout, tt.wantAttachAfter)
			}
			if !reflect.DeepEqual(f.fake.stoppedGUIDs, tt.wantStopped) {
				t.Errorf("stopped strands = %v; want %v", f.fake.stoppedGUIDs, tt.wantStopped)
			}
		})
	}
}

// TestArbiterVerb_VerdictOutcomes covers how a spawned arbiter's end becomes the printed envelope.
func TestArbiterVerb_VerdictOutcomes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		producer     string
		state        shedengine.State
		verdict      string
		waitResult   shuttleengine.Result
		setup        func(f *arbiterFixture)
		wantVerdict  string
		wantCause    string
		wantGoto     string
		wantRelaunch string
		wantRunStops int
		// wantError is set for a run the verb refuses after the arbiter started.
		wantError bool
	}{
		{
			name: "a marker that cannot be written stops the started arbiter and refuses", producer: "Plan-Write", state: shedengine.StateBlocked,
			setup: func(f *arbiterFixture) {
				if err := os.WriteFile(filepath.Dir(f.markerPath()), []byte("not a directory"), 0o644); err != nil {
					t.Fatal(err)
				}
			},
			wantError: true, wantRunStops: 1,
		},
		{
			name: "a valid resume carries its relaunch line", producer: "Plan-Write", state: shedengine.StateFailed,
			verdict: arbiterResumeVerdict, setup: func(f *arbiterFixture) { f.stopEnvelope = `{"kind":"producer"}` },
			wantVerdict:  loomengine.ArbiterVerdictResume,
			wantRelaunch: "lyx shed step " + arbiterTestRun + " --until-stop --arbiter-attempt 1",
		},
		{
			name: "a resume at a blocked stop with a goto target", producer: loomshed.NamePublish, state: shedengine.StateBlocked,
			verdict:     "verdict: resume\nreason: go back\ngoto: Plan-Write\n",
			wantVerdict: loomengine.ArbiterVerdictResume, wantGoto: "Plan-Write",
			wantRelaunch: "lyx shed step " + arbiterTestRun + " --until-stop --arbiter-attempt 1",
		},
		{
			name: "a resume at an escalation with its decision recorded", producer: "Plan-Bouncer", state: shedengine.StateAwaiting,
			verdict: arbiterResumeVerdict,
			setup: func(f *arbiterFixture) {
				f.escalationOpen = true
				f.circlingPending = true
			},
			wantVerdict:  loomengine.ArbiterVerdictResume,
			wantRelaunch: "lyx shed step " + arbiterTestRun + " --until-stop --arbiter-attempt 1",
		},
		{
			name: "a hand-on carries its question", producer: "Plan-Write", state: shedengine.StateBlocked,
			verdict: arbiterHandOnVerdict, wantVerdict: loomengine.ArbiterVerdictHandOn,
		},
		{
			name: "no verdict file reads missing", producer: "Plan-Write", state: shedengine.StateBlocked,
			wantVerdict: loomengine.ArbiterVerdictHandOn, wantCause: arbiterCauseMissing,
		},
		{
			name: "an unparseable verdict reads unparseable", producer: "Plan-Write", state: shedengine.StateBlocked,
			verdict: "verdict: [", wantVerdict: loomengine.ArbiterVerdictHandOn, wantCause: arbiterCauseUnparseable,
		},
		{
			name: "a resume at an escalation with no pending decision", producer: "Plan-Bouncer", state: shedengine.StateAwaiting,
			verdict: arbiterResumeVerdict, setup: func(f *arbiterFixture) { f.escalationOpen = true },
			wantVerdict: loomengine.ArbiterVerdictHandOn, wantCause: arbiterCausePrecondition,
		},
		{
			name: "a resume at a blocked stop with no goto target", producer: "Plan-Write", state: shedengine.StateBlocked,
			verdict: arbiterResumeVerdict, wantVerdict: loomengine.ArbiterVerdictHandOn, wantCause: arbiterCausePrecondition,
		},
		{
			name: "a resume naming a goto target past the current row", producer: "Plan-Write", state: shedengine.StateBlocked,
			verdict:     "verdict: resume\nreason: skip ahead\ngoto: PR-Gate\n",
			wantVerdict: loomengine.ArbiterVerdictHandOn, wantCause: arbiterCausePrecondition,
		},
		{
			name: "a timeout stops the run and reads hand-on", producer: "Plan-Write", state: shedengine.StateBlocked,
			waitResult:  shuttleengine.Result{Outcome: shuttleengine.OutcomeTimeout},
			wantVerdict: loomengine.ArbiterVerdictHandOn, wantCause: arbiterCauseTimeout, wantRunStops: 1,
		},
		{
			name: "a died run stops and reads hand-on", producer: "Plan-Write", state: shedengine.StateBlocked,
			waitResult:  shuttleengine.Result{Outcome: shuttleengine.OutcomeDied},
			wantVerdict: loomengine.ArbiterVerdictHandOn, wantCause: arbiterCauseDied, wantRunStops: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newArbiterFixture(t, tt.producer, tt.state)
			if tt.setup != nil {
				tt.setup(f)
			}
			f.fake.waitResult = tt.waitResult
			f.fake.onWait = func(shuttleengine.Spec) {
				if tt.verdict != "" {
					writeArbiterVerdictFile(t, f.attemptDir(1), tt.verdict)
				}
			}

			code, fields := f.run()

			if tt.wantError {
				if code == 0 {
					t.Fatalf("arbiterVerb() exit = 0, output %v; want an error", fields)
				}
				if f.fake.runStops != tt.wantRunStops {
					t.Errorf("run stops = %d; want %d", f.fake.runStops, tt.wantRunStops)
				}
				return
			}
			if code != 0 {
				t.Fatalf("arbiterVerb() exit = %d, output %v; want 0", code, fields)
			}
			if fields["verdict"] != tt.wantVerdict || fields["cause"] != tt.wantCause || fields["goto"] != tt.wantGoto {
				t.Errorf("verdict, cause, goto = %v, %v, %v; want %q, %q, %q", fields["verdict"], fields["cause"], fields["goto"], tt.wantVerdict, tt.wantCause, tt.wantGoto)
			}
			relaunch, hasRelaunch := fields["relaunch"]
			if tt.wantRelaunch == "" && hasRelaunch {
				t.Errorf("relaunch = %v on a hand-on; want the key absent", relaunch)
			}
			if tt.wantRelaunch != "" && relaunch != tt.wantRelaunch {
				t.Errorf("relaunch = %v; want %q", relaunch, tt.wantRelaunch)
			}
			if fields["attempt"] != float64(1) || fields["attempt_dir"] != f.attemptDir(1) || fields["verdict_path"] != loomengine.ArbiterVerdictPath(f.attemptDir(1)) {
				t.Errorf("attempt, attempt_dir, verdict_path = %v, %v, %v; want 1 and the attempt's paths", fields["attempt"], fields["attempt_dir"], fields["verdict_path"])
			}
			if tt.wantVerdict == loomengine.ArbiterVerdictHandOn && fields["question"] == "" {
				t.Error("hand-on question is empty; want the question for the parent")
			}
			if f.fake.runStops != tt.wantRunStops {
				t.Errorf("run stops = %d; want %d", f.fake.runStops, tt.wantRunStops)
			}
			saved, found, err := loomengine.ReadArbiterEnvelope(loomengine.ArbiterEnvelopePath(f.attemptDir(1)))
			if err != nil || !found || saved.Verdict != tt.wantVerdict || saved.Cause != tt.wantCause || saved.Relaunch != tt.wantRelaunch {
				t.Errorf("envelope.json = %+v, found %v, err %v; want the printed verdict, cause and relaunch", saved, found, err)
			}
		})
	}
}
