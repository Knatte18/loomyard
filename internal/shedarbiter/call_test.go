package shedarbiter

import (
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/arbiterstop"
	"github.com/Knatte18/loomyard/internal/landingshed"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/shedadapters"
	"github.com/Knatte18/loomyard/internal/sheddriver"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedverbs"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/stencilkit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// fakeShuttle is a scripted sheddriver.AgentShuttle: it records every call.
// It runs the scripted hooks inside Wait and AttachIfLive, where the real arbiter would write its verdict.
type fakeShuttle struct {
	starts       []shuttleengine.Spec
	attaches     []shuttleengine.Spec
	gates        []shuttleengine.GateSpec
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

// EngineName names the choice's own engine, and claude for a choice naming none.
func (f *fakeShuttle) EngineName(choice shuttleengine.ModelChoice) string {
	if choice.Engine != "" {
		return choice.Engine
	}
	return "claude"
}

// wantEngines returns the engine the fixture's settings resolve to.
// It also returns the engine whose edit directive copy a prompt must lack.
func (f *callFixture) wantEngines() (engine, otherEngine string) {
	if f.settings.ModelChoice.Engine != "" {
		return f.settings.ModelChoice.Engine, "claude"
	}
	return "claude", "codex"
}

func (f *fakeShuttle) Start(spec shuttleengine.Spec, gate shuttleengine.GateSpec) (sheddriver.AgentRun, error) {
	if f.startErr != nil {
		return nil, f.startErr
	}
	f.starts = append(f.starts, spec)
	f.gates = append(f.gates, gate)
	return &fakeRun{fake: f, spec: spec}, nil
}

func (f *fakeShuttle) AttachIfLive(spec shuttleengine.Spec, gate shuttleengine.GateSpec) (shuttleengine.Result, bool, error) {
	f.attaches = append(f.attaches, spec)
	f.gates = append(f.gates, gate)
	if f.onAttach != nil {
		f.onAttach(spec)
	}
	return f.attachResult, f.attachFound, f.attachErr
}

func (f *fakeShuttle) StopStrand(guid string) error {
	f.stoppedGUIDs = append(f.stoppedGUIDs, guid)
	return nil
}

type fakeRun struct {
	fake *fakeShuttle
	spec shuttleengine.Spec
}

func (r *fakeRun) Wait() (shuttleengine.Result, error) {
	if r.fake.onWait != nil {
		r.fake.onWait(r.spec)
	}
	result := r.fake.waitResult
	if result.Outcome == "" {
		result.Outcome = shuttleengine.OutcomeDone
	}
	return result, r.fake.waitErr
}

func (r *fakeRun) Stop() error {
	r.fake.runStops++
	return nil
}

func (r *fakeRun) StrandGUID() string { return r.fake.runGUID }

// callFixture is one temp run directory with the call's seams over it.
type callFixture struct {
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
	settings         sheddriver.ArbiterSettings
	fake             *fakeShuttle
	envelopePath     string
	removeEnvelope   bool
	reportPath       string
	caseBriefPath    string
	interruptPolicy  map[string]string
	producerRowNames []string
	gotoGuard        shedengine.GotoGuard
	amendmentTarget  func() string
	wayForward       string
	// worktree, when set, is the worktree seam the call reads; nil leaves Deps.Worktree unset.
	worktree *fakeWorktree
}

// fakeWorktree is a scripted worktree seam over one current snapshot and changed-path list.
// A test swaps them while the arbiter runs.
// Each read has its own error, so a test fails the spawn read or one read after the arbiter.
type fakeWorktree struct {
	current     sheddriver.WorktreeSnapshot
	changes     map[string]string
	snapshotErr error
	changesErr  error
	ancestorErr error
	logErr      error
	descends    bool
	commits     []string
	// ancestorArgs and logArgs are the (sha, ref) and (from, to) pairs the last IsAncestor and CommitLog reads received.
	ancestorArgs, logArgs [2]string
}

func (w *fakeWorktree) seam() *WorktreeSeam {
	return &WorktreeSeam{
		Snapshot: func() (sheddriver.WorktreeSnapshot, error) { return w.current, w.snapshotErr },
		Changes:  func() (map[string]string, error) { return w.changes, w.changesErr },
		IsAncestor: func(sha, ref string) (bool, error) {
			w.ancestorArgs = [2]string{sha, ref}
			return w.descends, w.ancestorErr
		},
		CommitLog: func(from, to string) ([]string, error) {
			w.logArgs = [2]string{from, to}
			return w.commits, w.logErr
		},
	}
}

const testRun = "operator-surface"

func newCallFixture(t *testing.T, producer string, state shedengine.State) *callFixture {
	t.Helper()
	root := t.TempDir()
	f := &callFixture{
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
		settings:         sheddriver.ArbiterSettings{ModelChoice: shuttleengine.ModelChoice{Model: "claude-resolved", Effort: "high", Version: "v1"}, Timeout: 30 * time.Minute, MaxSpawns: 3},
		fake:             &fakeShuttle{runGUID: "guid-1"},
		interruptPolicy:  map[string]string{},
		producerRowNames: []string{"Plan-Write", "Plan-Bouncer", loomshed.NameParentAlign, loomshed.NameFinalize},
	}
	f.envelopePath = filepath.Join(root, "stop-envelope.json")
	f.reportPath = filepath.Join(root, "report.md")
	return f
}

// haltAtVerifyRow halts the run at row, with a Stuck history entry for it at 11:30.
// The history starts with a Done entry at 11:00, and the status error is reason.
func (f *callFixture) haltAtVerifyRow(row, reason string) {
	f.status.Error = reason
	f.status.History = []shedengine.HistoryEntry{
		{Producer: "Plan-Bouncer", Outcome: shedengine.Done, At: "2026-10-12T11:00:00Z"},
		{Producer: row, Outcome: shedengine.Stuck, At: "2026-10-12T11:30:00Z"},
	}
}

func (f *callFixture) arbiterDir() string { return filepath.Join(f.root, "arbiter") }
func (f *callFixture) markerPath() string { return filepath.Join(f.root, "scratch", "arbiter.json") }
func (f *callFixture) attemptDir(n int) string {
	return filepath.Join(f.arbiterDir(), "attempt-"+string(rune('0'+n)))
}

func (f *callFixture) deps() Deps {
	producers := make([]shedengine.ProducerDef, len(f.producerRowNames))
	for i, name := range f.producerRowNames {
		producers[i] = shedengine.ProducerDef{Name: name}
	}
	amendmentTarget := f.amendmentTarget
	if amendmentTarget == nil {
		amendmentTarget = func() string { return AbsentPath }
	}
	var worktree *WorktreeSeam
	if f.worktree != nil {
		worktree = f.worktree.seam()
	}
	return Deps{
		Worktree: worktree,
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
		GotoGuard:  f.gotoGuard,
		Stop: arbiterstop.Readers{
			PendingEscalation: func(string) (shedadapters.EscalationCause, string, bool, error) {
				if f.escalationErr != nil {
					return "", "", false, f.escalationErr
				}
				return f.escalationCause, f.escalationBrief, f.escalationOpen, nil
			},
			InterruptPolicy: func(producer string) string { return f.interruptPolicy[producer] },
		},
		PendingCirclingDecision: func(string) (bool, error) { return f.circlingPending, nil },
		Prompt: PromptInput{
			Slug:               "operator-surface-task",
			Worktree:           f.root,
			DecisionRecordPath: filepath.Join(f.root, "decision-record.md"),
			SupportLogPath:     filepath.Join(f.root, "support-log.md"),
		},
		StencilsDir:        seededStencils(f.t),
		AmendmentTarget:    amendmentTarget,
		Shuttle:            f.fake,
		Now:                func() time.Time { return f.now },
		RelaunchRunID:      testRun,
		HandBackWayForward: f.wayForward,
	}
}

// run writes the stop envelope and settles the stop, returning the verdict envelope and the call's error.
func (f *callFixture) run() (loomengine.ArbiterEnvelope, error) {
	f.t.Helper()
	if !f.removeEnvelope {
		if err := os.WriteFile(f.envelopePath, []byte(f.stopEnvelope), 0o644); err != nil {
			f.t.Fatal(err)
		}
	}
	call, err := NewCall(f.deps(), Input{RunID: testRun, EnvelopePath: f.envelopePath, ReportPath: f.reportPath, CaseBriefPath: f.caseBriefPath})
	if err != nil {
		return loomengine.ArbiterEnvelope{}, err
	}
	return call.Settle()
}

// priorAttempt writes an attempt for the fixture's current stop, with its deadline offset from now.
func (f *callFixture) priorAttempt(number int, deadlineIn time.Duration) loomengine.ArbiterAttempt {
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

func writeVerdictFile(t *testing.T, dir, text string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(loomengine.ArbiterVerdictPath(dir), []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

const (
	resumeVerdict = "verdict: resume\nreason: the repair is safe\nverbs:\n  - lym loom circling accept operator-surface-task\n"
	handOnVerdict = "verdict: hand-on\nreason: the design is wrong\nquestion: should the parent widen the task?\n"
)

// TestCall_SpawnPath covers the call for a stop no attempt has arbitrated yet, including the eligibility list.
// An eligible stop spawns exactly one run whose spec, attempt record, prompt and marker carry the expected values;
// a refused stop spawns nothing and writes no attempt directory.
func TestCall_SpawnPath(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name        string
		producer    string
		state       shedengine.State
		envelope    string
		setup       func(f *callFixture)
		wantBrief   bool
		wantRefusal string
		// wantSuffix is the clause the refusal must end with when non-empty.
		wantSuffix  string
		wantNoSpawn bool
		// wantPromptHas is a fragment the spawned prompt must carry.
		wantPromptHas string
		// keepsArbiterDir is set for the row that seeds the attempt records it is refused over.
		keepsArbiterDir bool
	}{
		{name: "producer error", producer: "Plan-Write", state: shedengine.StateFailed, envelope: `{"kind":"producer"}`},
		{name: "bootstrap error", producer: "Plan-Write", state: shedengine.StateFailed, envelope: `{"kind":"bootstrap"}`},
		{
			name: "interrupted under reinvoke", producer: "Plan-Write", state: shedengine.StateFailed, envelope: `{"kind":"interrupted"}`,
			setup: func(f *callFixture) { f.interruptPolicy["Plan-Write"] = shedverbs.InterruptPolicyReinvoke },
		},
		{name: "transient failure that failed twice", producer: "Plan-Write", state: shedengine.StateFailed, envelope: `{"kind":"busy","transient":"network"}`},
		{name: "blocked stop", producer: "Plan-Write", state: shedengine.StateBlocked},
		{
			name: "a codex model choice spawns for the codex engine", producer: "Plan-Write", state: shedengine.StateBlocked,
			setup: func(f *callFixture) { f.settings.ModelChoice.Engine = "codex" },
		},
		{
			// The target answers only once the attempt record exists, so a prompt naming it pins the read at the spawn.
			// settleFresh writes that record first.
			name: "amendment target read at the spawn", producer: "Plan-Write", state: shedengine.StateBlocked,
			setup: func(f *callFixture) {
				f.amendmentTarget = func() string {
					if _, err := loomengine.ReadArbiterAttempt(f.attemptDir(1)); err != nil {
						return AbsentPath
					}
					return "Plan-Burler-Review"
				}
			},
			wantPromptHas: "Plan-Burler-Review",
		},
		{
			name: "blocked stop carrying a tackler case brief", producer: "Plan-Write", state: shedengine.StateBlocked,
			setup: func(f *callFixture) {
				f.caseBriefPath = filepath.Join(f.root, "tackler", "attempt-1", "case-brief.md")
			},
		},
		{
			name: "Parent-Align halt", producer: loomshed.NameParentAlign, state: shedengine.StateBlocked,
			setup: func(f *callFixture) { f.haltAtVerifyRow(loomshed.NameParentAlign, "") },
		},
		{
			name: "landing halt at Finalize refused", producer: loomshed.NameFinalize, state: shedengine.StateBlocked,
			setup: func(f *callFixture) {
				f.haltAtVerifyRow(loomshed.NameFinalize, landingshed.LandingStopPrefix+"the fixer escalated; way forward: resolve it, then `lym loom resume`")
			},
			wantRefusal: "a landing halt", wantNoSpawn: true,
		},
		{
			name: "awaiting at a Bouncer row with a pending escalation", producer: "Plan-Bouncer", state: shedengine.StateAwaiting, wantBrief: true,
			setup: func(f *callFixture) {
				f.escalationOpen = true
				f.escalationBrief = filepath.Join(f.root, "round-2-escalation.md")
			},
		},
		{
			name: "awaiting at a hard-cap escalation", producer: "Plan-Bouncer", state: shedengine.StateAwaiting, wantBrief: true,
			setup: func(f *callFixture) {
				f.escalationOpen = true
				f.escalationCause = shedadapters.EscalationHardCap
				f.escalationBrief = filepath.Join(f.root, "round-2-escalation.md")
			},
		},
		{
			name: "awaiting at a budget escalation refused", producer: "Plan-Bouncer", state: shedengine.StateAwaiting,
			setup: func(f *callFixture) {
				f.escalationOpen = true
				f.escalationCause = shedadapters.EscalationBudget
			},
			wantRefusal: "has cause \"budget\"", wantNoSpawn: true,
		},
		{
			name: "awaiting with an unreadable escalation record refused", producer: "Plan-Bouncer", state: shedengine.StateAwaiting,
			setup: func(f *callFixture) {
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
			setup:       func(f *callFixture) { f.interruptPolicy["Plan-Write"] = shedverbs.InterruptPolicyHandback },
			wantRefusal: "whose policy hands back", wantNoSpawn: true,
		},
		{name: "awaiting with a parent notice but no escalation refused", producer: loomshed.NameFinalize, state: shedengine.StateAwaiting, wantRefusal: "no pending review escalation", wantNoSpawn: true},
		{name: "paused refused", producer: "Plan-Write", state: shedengine.StatePaused, wantRefusal: "a paused run is not a halted stop", wantNoSpawn: true},
		{name: "done refused", producer: "Plan-Write", state: shedengine.StateDone, wantRefusal: "a done run is not a halted stop", wantNoSpawn: true},
		{name: "running refused", producer: "Plan-Write", state: shedengine.StateRunning, wantRefusal: "a running run is not a halted stop", wantNoSpawn: true},
		{name: "invalid state refused", producer: "Plan-Write", state: "bogus", wantRefusal: "not a halted stop", wantNoSpawn: true},
		{
			name: "missing status file refused", producer: "Plan-Write", state: shedengine.StateBlocked,
			setup:       func(f *callFixture) { f.statusMissing = true },
			wantRefusal: "status or envelope unreadable", wantNoSpawn: true,
		},
		{
			name: "status that does not parse refused", producer: "Plan-Write", state: shedengine.StateBlocked,
			setup:       func(f *callFixture) { f.statusErr = errors.New("invalid character") },
			wantRefusal: "status or envelope unreadable", wantNoSpawn: true,
		},
		{
			name: "unreadable envelope path refused", producer: "Plan-Write", state: shedengine.StateBlocked,
			setup:       func(f *callFixture) { f.removeEnvelope = true },
			wantRefusal: "status or envelope unreadable", wantNoSpawn: true,
		},
		{
			name: "budget of 0 refused", producer: "Plan-Write", state: shedengine.StateBlocked,
			setup:       func(f *callFixture) { f.settings.MaxSpawns = 0 },
			wantRefusal: "arbiter: off", wantNoSpawn: true,
		},
		{
			name: "a refusal ends with the deps' way forward", producer: "Plan-Write", state: shedengine.StateBlocked,
			setup: func(f *callFixture) {
				f.settings.MaxSpawns = 0
				f.wayForward = "way forward: read the stop with `lym batten status`"
			},
			wantRefusal: "arbiter: off", wantSuffix: "way forward: read the stop with `lym batten status`", wantNoSpawn: true,
		},
		{
			name: "spent budget refused, counted from the attempt records on disk", producer: "Plan-Write", state: shedengine.StateBlocked,
			setup: func(f *callFixture) {
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
			f := newCallFixture(t, tt.producer, tt.state)
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
				writeVerdictFile(t, f.attemptDir(1), handOnVerdict)
			}

			_, err := f.run()

			if tt.wantNoSpawn {
				if err == nil {
					t.Fatalf("Settle() error = nil; want a refusal containing %q", tt.wantRefusal)
				}
				message := err.Error()
				if !strings.Contains(message, tt.wantRefusal) || !strings.Contains(message, "way forward:") {
					t.Errorf("refusal = %q; want it to contain %q and a way forward", message, tt.wantRefusal)
				}
				if tt.wantSuffix != "" && !strings.HasSuffix(message, tt.wantSuffix) {
					t.Errorf("refusal = %q; want it to end with %q", message, tt.wantSuffix)
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

			if err != nil {
				t.Fatalf("Settle() error = %v; want nil", err)
			}
			if len(f.fake.starts) != 1 {
				t.Fatalf("starts = %d; want exactly one", len(f.fake.starts))
			}
			spec := f.fake.starts[0]
			verdictPath := loomengine.ArbiterVerdictPath(f.attemptDir(1))
			if spec.Role != loomengine.ArbiterRole || !reflect.DeepEqual(spec.OutputFiles, []string{verdictPath}) || spec.Model != "claude-resolved" || spec.Timeout != 30*time.Minute {
				t.Errorf("spec = role %q, outputs %v, model %q, timeout %v; want the arbiter role over %q, the resolved model and 30m", spec.Role, spec.OutputFiles, spec.Model, spec.Timeout, verdictPath)
			}
			wantEngine, otherEngine := f.wantEngines()
			if spec.PromptEngine != wantEngine {
				t.Errorf("spec.PromptEngine = %q; want %q, the engine the model choice resolves to", spec.PromptEngine, wantEngine)
			}
			stencilkit.RequireEditDirectiveOf(t, seededStencils(t), spec.Prompt, wantEngine, otherEngine)
			if captured.prompt != spec.Prompt || !strings.Contains(spec.Prompt, verdictPath) || !strings.Contains(spec.Prompt, f.envelopePath) {
				t.Errorf("prompt.md does not equal the spec prompt, or the prompt lacks the verdict and envelope paths")
			}
			if !strings.Contains(spec.Prompt, tt.wantPromptHas) {
				t.Errorf("prompt lacks %q", tt.wantPromptHas)
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
				t.Error("marker still present after the call finished; want it removed")
			}
		})
	}
}

// TestCall_Rerun covers the call for a stop an attempt already arbitrated:
// it spawns nothing and returns or awaits that attempt's verdict.
func TestCall_Rerun(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name            string
		setup           func(f *callFixture, attempt loomengine.ArbiterAttempt)
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
			setup: func(f *callFixture, attempt loomengine.ArbiterAttempt) {
				marker := loomengine.ArbiterMarker{Attempt: 1, StrandGUID: "old-guid", Deadline: attempt.Deadline}
				if err := loomengine.WriteArbiterMarker(f.markerPath(), marker); err != nil {
					t.Fatal(err)
				}
				f.fake.attachFound = true
				f.fake.onAttach = func(shuttleengine.Spec) { writeVerdictFile(t, attempt.Dir, handOnVerdict) }
			},
			wantAttaches: 1, wantAttachAfter: 20 * time.Minute, wantVerdict: loomengine.ArbiterVerdictHandOn,
		},
		{
			name: "a marker past its deadline stops the strand without attaching", deadlineIn: -time.Minute,
			setup: func(f *callFixture, attempt loomengine.ArbiterAttempt) {
				marker := loomengine.ArbiterMarker{Attempt: 1, StrandGUID: "old-guid", Deadline: attempt.Deadline}
				if err := loomengine.WriteArbiterMarker(f.markerPath(), marker); err != nil {
					t.Fatal(err)
				}
			},
			wantStopped: []string{"old-guid"}, wantVerdict: loomengine.ArbiterVerdictHandOn, wantCause: sheddriver.CauseTimeout,
		},
		{
			name: "an attach error stops the strand and reads died", deadlineIn: 20 * time.Minute,
			setup: func(f *callFixture, attempt loomengine.ArbiterAttempt) {
				marker := loomengine.ArbiterMarker{Attempt: 1, StrandGUID: "old-guid", Deadline: attempt.Deadline}
				if err := loomengine.WriteArbiterMarker(f.markerPath(), marker); err != nil {
					t.Fatal(err)
				}
				f.fake.attachErr = errors.New("reed unreachable")
			},
			wantAttaches: 1, wantAttachAfter: 20 * time.Minute, wantStopped: []string{"old-guid"}, wantVerdict: loomengine.ArbiterVerdictHandOn, wantCause: sheddriver.CauseDied,
		},
		{
			name: "a marked attempt with no live strand reads died", deadlineIn: 20 * time.Minute,
			setup: func(f *callFixture, attempt loomengine.ArbiterAttempt) {
				marker := loomengine.ArbiterMarker{Attempt: 1, StrandGUID: "old-guid", Deadline: attempt.Deadline}
				if err := loomengine.WriteArbiterMarker(f.markerPath(), marker); err != nil {
					t.Fatal(err)
				}
			},
			wantAttaches: 1, wantAttachAfter: 20 * time.Minute, wantVerdict: loomengine.ArbiterVerdictHandOn, wantCause: sheddriver.CauseDied,
		},
		{
			name: "an attempt with no marker whose strand is live attaches and waits", deadlineIn: 5 * time.Minute,
			setup: func(f *callFixture, attempt loomengine.ArbiterAttempt) {
				f.fake.attachFound = true
				f.fake.onAttach = func(shuttleengine.Spec) { writeVerdictFile(t, attempt.Dir, handOnVerdict) }
			},
			wantAttaches: 1, wantAttachAfter: 5 * time.Minute, wantVerdict: loomengine.ArbiterVerdictHandOn,
		},
		{
			name: "a live attempt is attached under the codex engine", deadlineIn: 5 * time.Minute,
			setup: func(f *callFixture, attempt loomengine.ArbiterAttempt) {
				f.settings.ModelChoice.Engine = "codex"
				f.fake.attachFound = true
				f.fake.onAttach = func(shuttleengine.Spec) { writeVerdictFile(t, attempt.Dir, handOnVerdict) }
			},
			wantAttaches: 1, wantAttachAfter: 5 * time.Minute, wantVerdict: loomengine.ArbiterVerdictHandOn,
		},
		{
			name: "an attempt with no marker, no live strand and no verdict reads missing", deadlineIn: 5 * time.Minute,
			setup:        func(f *callFixture, attempt loomengine.ArbiterAttempt) {},
			wantAttaches: 1, wantAttachAfter: 5 * time.Minute, wantVerdict: loomengine.ArbiterVerdictHandOn, wantCause: sheddriver.CauseMissing,
		},
		{
			name: "an attempt past its deadline with no marker reads missing without attaching", deadlineIn: -time.Minute,
			setup:       func(f *callFixture, attempt loomengine.ArbiterAttempt) {},
			wantVerdict: loomengine.ArbiterVerdictHandOn, wantCause: sheddriver.CauseMissing,
		},
		{
			name: "a finished unapplied verdict returns without starting", deadlineIn: -time.Minute,
			setup: func(f *callFixture, attempt loomengine.ArbiterAttempt) {
				writeVerdictFile(t, attempt.Dir, handOnVerdict)
			},
			wantVerdict: loomengine.ArbiterVerdictHandOn,
		},
		{
			name: "an applied resume is refused", deadlineIn: -time.Minute,
			setup: func(f *callFixture, attempt loomengine.ArbiterAttempt) {
				writeVerdictFile(t, attempt.Dir, resumeVerdict)
				if err := loomengine.MarkArbiterApplied(attempt.Dir); err != nil {
					t.Fatal(err)
				}
			},
			wantRefusal: "an applied resume left the stop key unchanged",
		},
		{
			name: "an attempt for another stop key does not match", deadlineIn: 20 * time.Minute,
			setup: func(f *callFixture, attempt loomengine.ArbiterAttempt) {
				attempt.StopKey.HistoryLength++
				if err := loomengine.WriteArbiterAttempt(attempt.Dir, attempt); err != nil {
					t.Fatal(err)
				}
				f.fake.onWait = func(shuttleengine.Spec) { writeVerdictFile(t, f.attemptDir(2), handOnVerdict) }
			},
			wantStarts: 1, wantVerdict: loomengine.ArbiterVerdictHandOn,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newCallFixture(t, "Plan-Write", shedengine.StateBlocked)
			attempt := f.priorAttempt(1, tt.deadlineIn)
			tt.setup(f, attempt)

			envelope, err := f.run()

			if tt.wantRefusal != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantRefusal) || !strings.Contains(err.Error(), "way forward:") {
					t.Fatalf("Settle() error = %v; want a refusal containing %q and a way forward", err, tt.wantRefusal)
				}
			} else {
				if err != nil {
					t.Fatalf("Settle() error = %v; want nil", err)
				}
				if envelope.Verdict != tt.wantVerdict || envelope.Cause != tt.wantCause {
					t.Errorf("verdict, cause = %q, %q; want %q, %q", envelope.Verdict, envelope.Cause, tt.wantVerdict, tt.wantCause)
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
			if wantEngine, _ := f.wantEngines(); tt.wantAttaches > 0 && f.fake.attaches[0].PromptEngine != wantEngine {
				t.Errorf("attach PromptEngine = %q; want %q, the engine the model choice resolves to", f.fake.attaches[0].PromptEngine, wantEngine)
			}
			if !reflect.DeepEqual(f.fake.stoppedGUIDs, tt.wantStopped) {
				t.Errorf("stopped strands = %v; want %v", f.fake.stoppedGUIDs, tt.wantStopped)
			}
		})
	}
}

// TestVerdictGate covers the gate the arbiter's started and attached runs carry:
// it fails a verdict file that does not read, such as a verbs item whose ": " makes it a map;
// it passes an absent file and one written in block scalars.
func TestVerdictGate(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	const title = `--title "D6 refined: card 9's scope"`
	unquotedVerbs := "verdict: resume\nreason: the accept is recorded\nverbs:\n  - lym loom decision add operator-surface-task " + title + "\n"
	blockVerbs := "verdict: resume\nreason: |-\n  the accept is recorded: card 9 stands\nverbs:\n  - |-\n    lym loom decision add operator-surface-task " + title + "\n"

	paths := []struct {
		name  string
		setup func(f *callFixture)
	}{
		{name: "Start", setup: func(f *callFixture) {}},
		{name: "Attach", setup: func(f *callFixture) {
			f.priorAttempt(1, 20*time.Minute)
			f.fake.attachFound = true
		}},
	}
	files := []struct {
		name         string
		verdict      string
		wantPassed   bool
		wantFindings []string
	}{
		{name: "absent verdict passes", wantPassed: true},
		{name: "unquoted verbs item with a colon fails", verdict: unquotedVerbs, wantFindings: []string{"cannot unmarshal !!map into string", "block scalar"}},
		{name: "block scalar verdict passes", verdict: blockVerbs, wantPassed: true},
	}
	for _, path := range paths {
		t.Run(path.name, func(t *testing.T) {
			t.Parallel()
			f := newCallFixture(t, "Plan-Write", shedengine.StateBlocked)
			path.setup(f)

			_, _ = f.run()

			if len(f.fake.gates) != 1 || len(f.fake.gates[0]) != 1 {
				t.Fatalf("gates = %v; want one run carrying one entry", f.fake.gates)
			}
			entry := f.fake.gates[0][0]
			if entry.Attempts == 0 || !entry.PassOnCap {
				t.Errorf("gate entry attempts, pass-on-cap = %d, %v; want a re-prompt budget that lets the run through once spent", entry.Attempts, entry.PassOnCap)
			}
			verdictPath := loomengine.ArbiterVerdictPath(f.attemptDir(1))
			for _, file := range files {
				if err := os.Remove(verdictPath); err != nil && !errors.Is(err, os.ErrNotExist) {
					t.Fatal(err)
				}
				if file.verdict != "" {
					writeVerdictFile(t, f.attemptDir(1), file.verdict)
				}
				result, err := entry.Gate()
				if err != nil {
					t.Fatalf("%s: gate error = %v; want nil", file.name, err)
				}
				if result.Passed != file.wantPassed {
					t.Errorf("%s: gate passed = %v, findings %q; want %v", file.name, result.Passed, result.Findings, file.wantPassed)
				}
				for _, want := range append(file.wantFindings, verdictPath) {
					if !file.wantPassed && !strings.Contains(result.Findings, want) {
						t.Errorf("%s: findings = %q; want them to contain %q", file.name, result.Findings, want)
					}
				}
			}
		})
	}
}

// TestCall_Edits covers an arbiter that may edit.
// The call snapshots the worktree at spawn and reads the commits the arbiter made since.
// It hands on a resume over a worktree the run may not go on from.
// With no worktree seam the arbiter is told it edits nothing and its attempt records no snapshot.
func TestCall_Edits(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	const commit = "bbb1234 fix the gate"
	const gotoVerdict = "verdict: resume\nreason: fixed the gate\ngoto: Plan-Write\n"
	spawn := sheddriver.WorktreeSnapshot{Head: "aaa", Branch: "task"}
	moved := sheddriver.WorktreeSnapshot{Head: "bbb", Branch: "task"}

	tests := []struct {
		name string
		// noSeam leaves Deps.Worktree unset.
		noSeam bool
		// snapshotErr is what the seam's spawn read fails with.
		snapshotErr error
		// spawnChanges is the uncommitted paths and their content hashes at spawn; afterChanges is those the arbiter leaves.
		spawnChanges, afterChanges map[string]string
		// spawnChangesErr is what the seam's spawn read of the uncommitted paths fails with.
		spawnChangesErr error
		// after is the worktree the arbiter leaves; descends and commits answer the commit reads.
		after      sheddriver.WorktreeSnapshot
		descends   bool
		commits    []string
		verdict    string
		waitResult shuttleengine.Result
		// afterSnapshotErr, afterChangesErr, ancestorErr and logErr are what each read fails with after the arbiter.
		afterSnapshotErr, afterChangesErr, ancestorErr, logErr error
		// viaSeam also settles the stop through Seam's Judge, whose outcome must carry the envelope's commits.
		viaSeam bool

		wantRefusal  string
		wantVerdict  string
		wantCause    string
		wantCommits  []string
		wantReason   string
		wantQuestion []string
		wantPrompt   string
	}{
		{
			name: "a resume with no commit carries none", after: spawn, verdict: resumeVerdict,
			wantVerdict: loomengine.ArbiterVerdictResume, wantPrompt: "## Your one fix",
		},
		{
			name: "a resume after commits with a goto lists them", after: moved, descends: true, commits: []string{commit}, verdict: gotoVerdict,
			viaSeam:     true,
			wantVerdict: loomengine.ArbiterVerdictResume, wantCommits: []string{commit},
		},
		{
			name: "a resume after commits with no goto hands on naming them", after: moved, descends: true, commits: []string{commit}, verdict: resumeVerdict,
			wantVerdict: loomengine.ArbiterVerdictHandOn, wantCause: sheddriver.CausePrecondition, wantCommits: []string{commit},
			wantReason: "needs a goto target", wantQuestion: []string{commit, "keep or revert"},
		},
		{
			name: "an agent's hand-on after commits keeps its question and names them", after: moved, descends: true, commits: []string{commit}, verdict: handOnVerdict,
			wantVerdict: loomengine.ArbiterVerdictHandOn, wantCommits: []string{commit},
			wantReason: "the design is wrong", wantQuestion: []string{"should the parent widen the task?", commit},
		},
		{
			name: "a resume over uncommitted changes hands on", after: sheddriver.WorktreeSnapshot{Head: "aaa", Branch: "task", Dirty: map[string]string{"/wt/a.go": "hash"}}, verdict: gotoVerdict,
			wantVerdict: loomengine.ArbiterVerdictHandOn, wantCause: sheddriver.CausePrecondition, wantReason: "uncommitted changes",
		},
		{
			name: "a resume over a new untracked file hands on", after: spawn, afterChanges: map[string]string{"new.go": "h1"}, verdict: gotoVerdict,
			wantVerdict: loomengine.ArbiterVerdictHandOn, wantCause: sheddriver.CausePrecondition, wantReason: "uncommitted changes",
		},
		{
			name: "a resume leaving an untracked file it found untouched goes on", after: spawn, verdict: resumeVerdict,
			spawnChanges: map[string]string{"b.go": "h2", "a.go": "h1"}, afterChanges: map[string]string{"a.go": "h1", "b.go": "h2"},
			wantVerdict: loomengine.ArbiterVerdictResume,
		},
		{
			name: "a resume over an untracked file it found and edited hands on", after: spawn, verdict: gotoVerdict,
			spawnChanges: map[string]string{"a.go": "h1"}, afterChanges: map[string]string{"a.go": "h2"},
			wantVerdict: loomengine.ArbiterVerdictHandOn, wantCause: sheddriver.CausePrecondition, wantReason: "uncommitted changes",
		},
		{
			name: "a resume over uncommitted changes after commits hands on and lists the commits", descends: true, commits: []string{commit},
			after:   sheddriver.WorktreeSnapshot{Head: "bbb", Branch: "task", Dirty: map[string]string{"/wt/a.go": "hash"}},
			verdict: gotoVerdict, wantVerdict: loomengine.ArbiterVerdictHandOn, wantCause: sheddriver.CausePrecondition,
			wantCommits: []string{commit}, wantReason: "uncommitted changes", wantQuestion: []string{commit},
		},
		{
			name:  "an agent's hand-on over uncommitted changes gains the worktree note",
			after: sheddriver.WorktreeSnapshot{Head: "aaa", Branch: "task", Dirty: map[string]string{"/wt/a.go": "hash"}}, verdict: handOnVerdict,
			wantVerdict: loomengine.ArbiterVerdictHandOn, wantReason: "the design is wrong",
			wantQuestion: []string{"should the parent widen the task?", "Check the worktree first", "uncommitted changes"},
		},
		{
			name: "a resume after a worktree that cannot be read hands on", after: spawn, afterSnapshotErr: errors.New("HEAD is detached"), verdict: gotoVerdict,
			wantVerdict: loomengine.ArbiterVerdictHandOn, wantCause: sheddriver.CausePrecondition, wantReason: "could not be read after the arbiter",
		},
		{
			name: "a resume after uncommitted paths that cannot be read hands on", after: spawn, afterChangesErr: errors.New("index locked"), verdict: gotoVerdict,
			wantVerdict: loomengine.ArbiterVerdictHandOn, wantCause: sheddriver.CausePrecondition, wantReason: "uncommitted paths could not be read",
		},
		{
			name: "a resume after a failed ancestry read hands on naming the move", after: moved, ancestorErr: errors.New("bad object"), verdict: gotoVerdict,
			wantVerdict: loomengine.ArbiterVerdictHandOn, wantCause: sheddriver.CausePrecondition,
			wantReason: "HEAD moved from aaa to bbb and the arbiter's commits could not be read",
		},
		{
			name: "a resume after a failed commit log read hands on naming the move", after: moved, descends: true, logErr: errors.New("bad object"), verdict: gotoVerdict,
			wantVerdict: loomengine.ArbiterVerdictHandOn, wantCause: sheddriver.CausePrecondition,
			wantReason: "HEAD moved from aaa to bbb and the arbiter's commits could not be read", wantQuestion: []string{"HEAD moved from aaa to bbb"},
		},
		{
			name: "a resume after a branch switch hands on", after: sheddriver.WorktreeSnapshot{Head: "aaa", Branch: "other"}, verdict: gotoVerdict,
			wantVerdict: loomengine.ArbiterVerdictHandOn, wantCause: sheddriver.CausePrecondition, wantReason: "left branch",
		},
		{
			name: "a resume over rewritten history hands on", after: moved, descends: false, verdict: gotoVerdict,
			wantVerdict: loomengine.ArbiterVerdictHandOn, wantCause: sheddriver.CausePrecondition, wantReason: "rewrote the branch's history",
		},
		{
			name: "a timeout after commits hands on naming them", after: moved, descends: true, commits: []string{commit},
			waitResult:  shuttleengine.Result{Outcome: shuttleengine.OutcomeTimeout},
			wantVerdict: loomengine.ArbiterVerdictHandOn, wantCause: sheddriver.CauseTimeout, wantCommits: []string{commit}, wantQuestion: []string{commit},
		},
		{
			name: "a worktree that cannot be read at spawn refuses and spawns nothing", snapshotErr: errors.New("index locked"),
			wantRefusal: "the worktree could not be read before the spawn",
		},
		{
			name: "uncommitted paths that cannot be read at spawn refuse and spawn nothing", spawnChangesErr: errors.New("index locked"),
			wantRefusal: "the worktree's uncommitted paths could not be read before the spawn",
		},
		{
			name: "no worktree seam tells the arbiter it edits nothing", noSeam: true, verdict: resumeVerdict,
			wantVerdict: loomengine.ArbiterVerdictResume, wantPrompt: "## No fix of your own",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newCallFixture(t, loomshed.NameParentAlign, shedengine.StateFailed)
			f.stopEnvelope = `{"kind":"producer"}`
			worktree := &fakeWorktree{
				current: spawn, changes: tt.spawnChanges, changesErr: tt.spawnChangesErr, snapshotErr: tt.snapshotErr,
				descends: tt.descends, commits: tt.commits, ancestorErr: tt.ancestorErr, logErr: tt.logErr,
			}
			if !tt.noSeam {
				f.worktree = worktree
			}
			f.fake.waitResult = tt.waitResult
			f.fake.onWait = func(shuttleengine.Spec) {
				worktree.current = tt.after
				worktree.changes = tt.afterChanges
				worktree.snapshotErr = tt.afterSnapshotErr
				worktree.changesErr = tt.afterChangesErr
				if tt.verdict != "" {
					writeVerdictFile(t, f.attemptDir(1), tt.verdict)
				}
			}

			envelope, err := f.run()

			if tt.wantRefusal != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantRefusal) || !strings.Contains(err.Error(), "way forward:") {
					t.Fatalf("Settle() error = %v; want a refusal containing %q and a way forward", err, tt.wantRefusal)
				}
				if len(f.fake.starts) != 0 {
					t.Errorf("starts = %d; want none", len(f.fake.starts))
				}
				return
			}
			if err != nil {
				t.Fatalf("Settle() error = %v; want nil", err)
			}
			if envelope.Verdict != tt.wantVerdict || envelope.Cause != tt.wantCause {
				t.Errorf("verdict, cause = %q, %q; want %q, %q", envelope.Verdict, envelope.Cause, tt.wantVerdict, tt.wantCause)
			}
			if !slices.Equal(envelope.Commits, tt.wantCommits) {
				t.Errorf("commits = %q; want %q", envelope.Commits, tt.wantCommits)
			}
			if !strings.Contains(envelope.Reason, tt.wantReason) {
				t.Errorf("reason = %q; want it to contain %q", envelope.Reason, tt.wantReason)
			}
			for _, want := range tt.wantQuestion {
				if !strings.Contains(envelope.Question, want) {
					t.Errorf("question = %q; want it to contain %q", envelope.Question, want)
				}
			}
			if len(f.fake.starts) != 1 || !strings.Contains(f.fake.starts[0].Prompt, tt.wantPrompt) {
				t.Errorf("the spawned prompt lacks %q", tt.wantPrompt)
			}
			saved, _, err := loomengine.ReadArbiterEnvelope(loomengine.ArbiterEnvelopePath(f.attemptDir(1)))
			if err != nil || !slices.Equal(saved.Commits, tt.wantCommits) {
				t.Errorf("envelope.json commits = %q, err %v; want %q", saved.Commits, err, tt.wantCommits)
			}
			if tt.viaSeam {
				// The attempt is settled, so the second call returns its envelope without spawning.
				outcome, err := Seam(f.deps(), testRun, func(string) bool { return false }).Judge(f.envelopePath, f.reportPath, f.caseBriefPath)
				if err != nil || !slices.Equal(outcome.Commits, tt.wantCommits) {
					t.Errorf("Judge commits = %q, err %v; want %q", outcome.Commits, err, tt.wantCommits)
				}
			}
			attempt, err := loomengine.ReadArbiterAttempt(f.attemptDir(1))
			if err != nil {
				t.Fatalf("ReadArbiterAttempt: %v", err)
			}
			switch {
			case tt.noSeam && attempt.Snapshot != nil:
				t.Errorf("attempt snapshot = %+v; want none with no worktree seam", attempt.Snapshot)
			case !tt.noSeam && (attempt.Snapshot == nil || !attempt.Snapshot.Equal(spawn)):
				t.Errorf("attempt snapshot = %+v; want the worktree at spawn, %+v", attempt.Snapshot, spawn)
			}
			if !maps.Equal(attempt.Changes, tt.spawnChanges) {
				t.Errorf("attempt changes = %q; want the uncommitted paths at spawn, %q", attempt.Changes, tt.spawnChanges)
			}
			if tt.after.Head == moved.Head {
				wantPair := [2]string{spawn.Head, moved.Head}
				if worktree.ancestorArgs != wantPair {
					t.Errorf("IsAncestor(sha, ref) = %q; want the spawn HEAD and the current HEAD, %q", worktree.ancestorArgs, wantPair)
				}
				if tt.descends && tt.ancestorErr == nil && worktree.logArgs != wantPair {
					t.Errorf("CommitLog(from, to) = %q; want the spawn HEAD and the current HEAD, %q", worktree.logArgs, wantPair)
				}
			}
		})
	}
}

// TestCall_VerdictOutcomes covers how a spawned arbiter's end becomes the settled envelope.
func TestCall_VerdictOutcomes(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name         string
		producer     string
		state        shedengine.State
		verdict      string
		waitResult   shuttleengine.Result
		setup        func(f *callFixture)
		wantVerdict  string
		wantCause    string
		wantGoto     string
		wantRelaunch string
		wantRunStops int
		// wantReason is a substring of the settled reason, and wantQuestion of its question, each asserted when non-empty.
		wantReason   string
		wantQuestion string
		// wantError is set for a run the call refuses after the arbiter started.
		wantError bool
	}{
		{
			name: "a marker that cannot be written stops the started arbiter and refuses", producer: "Plan-Write", state: shedengine.StateBlocked,
			setup: func(f *callFixture) {
				if err := os.WriteFile(filepath.Dir(f.markerPath()), []byte("not a directory"), 0o644); err != nil {
					t.Fatal(err)
				}
			},
			wantError: true, wantRunStops: 1,
		},
		{
			name: "a valid resume carries its relaunch line", producer: "Plan-Write", state: shedengine.StateFailed,
			verdict: resumeVerdict, setup: func(f *callFixture) { f.stopEnvelope = `{"kind":"producer"}` },
			wantVerdict:  loomengine.ArbiterVerdictResume,
			wantRelaunch: "lym shed step " + testRun + " --until-stop --arbiter-attempt 1",
		},
		{
			name: "a resume at a blocked stop with a goto target", producer: loomshed.NameParentAlign, state: shedengine.StateBlocked,
			verdict:     "verdict: resume\nreason: go back\ngoto: Plan-Write\n",
			wantVerdict: loomengine.ArbiterVerdictResume, wantGoto: "Plan-Write",
			wantRelaunch: "lym shed step " + testRun + " --until-stop --arbiter-attempt 1",
		},
		{
			name: "a resume at an escalation with its decision recorded", producer: "Plan-Bouncer", state: shedengine.StateAwaiting,
			verdict: resumeVerdict,
			setup: func(f *callFixture) {
				f.escalationOpen = true
				f.circlingPending = true
			},
			wantVerdict:  loomengine.ArbiterVerdictResume,
			wantRelaunch: "lym shed step " + testRun + " --until-stop --arbiter-attempt 1",
		},
		{
			name: "a hand-on carries its question", producer: "Plan-Write", state: shedengine.StateBlocked,
			verdict: handOnVerdict, wantVerdict: loomengine.ArbiterVerdictHandOn,
		},
		{
			name: "no verdict file reads missing", producer: "Plan-Write", state: shedengine.StateBlocked,
			wantVerdict: loomengine.ArbiterVerdictHandOn, wantCause: sheddriver.CauseMissing,
		},
		{
			name: "an unparseable verdict reads unparseable", producer: "Plan-Write", state: shedengine.StateBlocked,
			verdict: "verdict: [", wantVerdict: loomengine.ArbiterVerdictHandOn, wantCause: sheddriver.CauseUnparseable,
		},
		{
			name: "a resume at an escalation with no pending decision", producer: "Plan-Bouncer", state: shedengine.StateAwaiting,
			verdict: resumeVerdict, setup: func(f *callFixture) { f.escalationOpen = true },
			wantVerdict: loomengine.ArbiterVerdictHandOn, wantCause: sheddriver.CausePrecondition,
		},
		{
			name: "a resume at a blocked stop with no goto target", producer: "Plan-Write", state: shedengine.StateBlocked,
			verdict: resumeVerdict, wantVerdict: loomengine.ArbiterVerdictHandOn, wantCause: sheddriver.CausePrecondition,
		},
		{
			name: "a resume naming a goto target past the current row", producer: "Plan-Write", state: shedengine.StateBlocked,
			verdict:     "verdict: resume\nreason: skip ahead\ngoto: Finalize\n",
			wantVerdict: loomengine.ArbiterVerdictHandOn, wantCause: sheddriver.CausePrecondition,
		},
		{
			name: "a resume naming a goto target the guard refuses", producer: loomshed.NameParentAlign, state: shedengine.StateBlocked,
			verdict: "verdict: resume\nreason: go back\ngoto: Plan-Write\n",
			setup: func(f *callFixture) {
				f.gotoGuard = func(string) (string, error) { return "the Plan-Review verdict is settled", nil }
			},
			wantVerdict: loomengine.ArbiterVerdictHandOn, wantCause: sheddriver.CausePrecondition,
			wantReason: "the Plan-Review verdict is settled",
		},
		{
			name: "a timeout stops the run and reads hand-on", producer: "Plan-Write", state: shedengine.StateBlocked,
			waitResult:  shuttleengine.Result{Outcome: shuttleengine.OutcomeTimeout},
			wantVerdict: loomengine.ArbiterVerdictHandOn, wantCause: sheddriver.CauseTimeout, wantRunStops: 1,
		},
		{
			name: "a died run stops and reads hand-on", producer: "Plan-Write", state: shedengine.StateBlocked,
			waitResult:  shuttleengine.Result{Outcome: shuttleengine.OutcomeDied},
			wantVerdict: loomengine.ArbiterVerdictHandOn, wantCause: sheddriver.CauseDied, wantRunStops: 1,
		},
		{
			name: "a run ended on an API error stops and reads hand-on naming the error", producer: "Plan-Write", state: shedengine.StateBlocked,
			waitResult:  shuttleengine.Result{Outcome: shuttleengine.OutcomeAPIError, APIError: "server_error"},
			wantVerdict: loomengine.ArbiterVerdictHandOn, wantCause: sheddriver.CauseAPIError, wantRunStops: 1,
			wantReason: "server_error", wantQuestion: "server_error",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newCallFixture(t, tt.producer, tt.state)
			if tt.setup != nil {
				tt.setup(f)
			}
			f.fake.waitResult = tt.waitResult
			f.fake.onWait = func(shuttleengine.Spec) {
				if tt.verdict != "" {
					writeVerdictFile(t, f.attemptDir(1), tt.verdict)
				}
			}

			envelope, err := f.run()

			if tt.wantError {
				if err == nil {
					t.Fatalf("Settle() error = nil, envelope %+v; want an error", envelope)
				}
				if f.fake.runStops != tt.wantRunStops {
					t.Errorf("run stops = %d; want %d", f.fake.runStops, tt.wantRunStops)
				}
				return
			}
			if err != nil {
				t.Fatalf("Settle() error = %v; want nil", err)
			}
			if envelope.Verdict != tt.wantVerdict || envelope.Cause != tt.wantCause || envelope.Goto != tt.wantGoto {
				t.Errorf("verdict, cause, goto = %q, %q, %q; want %q, %q, %q", envelope.Verdict, envelope.Cause, envelope.Goto, tt.wantVerdict, tt.wantCause, tt.wantGoto)
			}
			if !strings.Contains(envelope.Reason, tt.wantReason) {
				t.Errorf("reason = %q; want it to contain %q", envelope.Reason, tt.wantReason)
			}
			if !strings.Contains(envelope.Question, tt.wantQuestion) {
				t.Errorf("question = %q; want it to contain %q", envelope.Question, tt.wantQuestion)
			}
			if envelope.Relaunch != tt.wantRelaunch {
				t.Errorf("relaunch = %q; want %q", envelope.Relaunch, tt.wantRelaunch)
			}
			if envelope.Attempt != 1 || envelope.AttemptDir != f.attemptDir(1) || envelope.VerdictPath != loomengine.ArbiterVerdictPath(f.attemptDir(1)) {
				t.Errorf("attempt, attempt_dir, verdict_path = %v, %v, %v; want 1 and the attempt's paths", envelope.Attempt, envelope.AttemptDir, envelope.VerdictPath)
			}
			if tt.wantVerdict == loomengine.ArbiterVerdictHandOn && envelope.Question == "" {
				t.Error("hand-on question is empty; want the question for the parent")
			}
			if f.fake.runStops != tt.wantRunStops {
				t.Errorf("run stops = %d; want %d", f.fake.runStops, tt.wantRunStops)
			}
			saved, found, err := loomengine.ReadArbiterEnvelope(loomengine.ArbiterEnvelopePath(f.attemptDir(1)))
			if err != nil || !found || saved.Verdict != tt.wantVerdict || saved.Cause != tt.wantCause || saved.Relaunch != tt.wantRelaunch {
				t.Errorf("envelope.json = %+v, found %v, err %v; want the settled verdict, cause and relaunch", saved, found, err)
			}
		})
	}
}
