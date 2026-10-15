// tackler_test.go — untagged Tier-1 tests for the Go driver's tackler call, over the arbiter's fake shuttle, a fixed clock and a fake pause reader.

package loomcli

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/sheddriver"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// tacklerFixture is one temp run directory with the tackler call's seams over it.
type tacklerFixture struct {
	t    *testing.T
	root string
	now  time.Time

	status     shedengine.Status
	settings   loomengine.TacklerSettings
	fake       *arbiterFake
	snapshot   sheddriver.WorktreeSnapshot
	paused     time.Duration
	closed     func(spawned time.Time) bool
	producers  []string
	envelope   string
	reportPath string
}

const tacklerTestRun = "operator-surface"

func newTacklerFixture(t *testing.T) *tacklerFixture {
	t.Helper()
	root := t.TempDir()
	return &tacklerFixture{
		t:    t,
		root: root,
		now:  time.Date(2026, 10, 12, 12, 0, 0, 0, time.UTC),
		status: shedengine.Status{
			CurrentProducer: "Plan-Bouncer",
			State:           shedengine.StateFailed,
			History: []shedengine.HistoryEntry{
				{Producer: "Plan-Draft", Outcome: shedengine.Done},
				{Producer: "Plan-Write", Outcome: shedengine.Done},
			},
		},
		settings:   loomengine.TacklerSettings{Model: "claude-resolved", Effort: "low", Version: "v1", Timeout: 10 * time.Minute, MaxSpawns: 2},
		fake:       &arbiterFake{runGUID: "guid-1"},
		snapshot:   sheddriver.WorktreeSnapshot{Head: "abc123", Branch: "task", Dirty: map[string]string{"a.go": "h1"}},
		producers:  []string{"Plan-Draft", "Plan-Write", "Plan-Bouncer", "PR-Gate"},
		envelope:   filepath.Join(root, "stop-envelope.json"),
		reportPath: filepath.Join(root, "report.md"),
	}
}

func (f *tacklerFixture) tacklerDir() string { return filepath.Join(f.root, "tackler") }
func (f *tacklerFixture) markerPath() string { return filepath.Join(f.root, "scratch", "tackler.json") }
func (f *tacklerFixture) attemptDir(n int) string {
	return filepath.Join(f.tacklerDir(), "attempt-"+string(rune('0'+n)))
}

func (f *tacklerFixture) deps() tacklerDeps {
	producers := make([]shedengine.ProducerDef, len(f.producers))
	for i, name := range f.producers {
		producers[i] = shedengine.ProducerDef{Name: name}
	}
	return tacklerDeps{
		Closed: func(_ sheddriver.StopKey, spawned time.Time) (bool, error) {
			return f.closed != nil && f.closed(spawned), nil
		},
		ReadStatus:   func() (shedengine.Status, bool, error) { return f.status, true, nil },
		Settings:     f.settings,
		TacklerDir:   f.tacklerDir(),
		AttemptDir:   f.attemptDir,
		MarkerPath:   f.markerPath(),
		Producers:    producers,
		Snapshot:     func() (sheddriver.WorktreeSnapshot, error) { return f.snapshot, nil },
		PausedWithin: func(_, _ time.Time) (time.Duration, error) { return f.paused, nil },
		Prompt: tacklerPromptInput{
			Slug:      "operator-surface-task",
			Worktree:  f.root,
			TraceDir:  filepath.Join(f.root, "traces"),
			DriverLog: filepath.Join(f.root, "driver.log"),
		},
		StencilsDir: seededStencils(f.t),
		Shuttle:     f.fake,
		Now:         func() time.Time { return f.now },
	}
}

// settle runs one call over the fixture's seams.
func (f *tacklerFixture) settle() (sheddriver.TacklerEnvelope, error) {
	f.t.Helper()
	call, err := newTacklerCall(f.deps(), tacklerInput{
		RunID:        tacklerTestRun,
		EnvelopePath: f.envelope,
		ReportPath:   f.reportPath,
		TraceCopy:    filepath.Join(f.root, "step.trace.log"),
		StderrPath:   filepath.Join(f.root, "step.stderr.log"),
	})
	if err != nil {
		f.t.Fatalf("newTacklerCall() error = %v; want nil", err)
	}
	return call.settle()
}

// priorAttempt writes an attempt for the fixture's current stop, with its deadline offset from now.
func (f *tacklerFixture) priorAttempt(number int, deadlineIn time.Duration) sheddriver.TacklerAttempt {
	f.t.Helper()
	attempt := sheddriver.TacklerAttempt{
		Number:     number,
		StopKey:    sheddriver.StopKeyOf(f.status),
		SpawnedAt:  f.now.Add(-5 * time.Minute),
		Deadline:   f.now.Add(deadlineIn),
		StrandGUID: "old-guid",
	}
	if err := sheddriver.WriteTacklerAttempt(f.attemptDir(number), attempt); err != nil {
		f.t.Fatal(err)
	}
	attempt.Dir = f.attemptDir(number)
	return attempt
}

func writeTacklerFile(t *testing.T, path, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

const (
	tacklerResumeVerdict   = "verdict: resume\nreason: the failure was transient\n"
	tacklerHandBackVerdict = "verdict: hand-back\nreason: the design is wrong\nquestion: should the parent widen the task?\n"
)

// TestTacklerCall_SpawnPath covers the call for a stop no open attempt judged: the budget refusals, what a fresh spawn records before and while the tackler runs, and the three ways a started tackler can end without a verdict.
func TestTacklerCall_SpawnPath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		setup func(f *tacklerFixture)
		// wantErr is the sentinel the call returns; empty rows expect an envelope.
		wantErr     error
		wantErrText string
		wantCause   string
		wantVerdict string
		wantStarts  int
		wantStops   int
		wantAttach  int
		// keepsTacklerDir is set for the row that seeds the attempt records it is refused over.
		keepsTacklerDir bool
	}{
		{
			name:    "tackler_max_spawns 0 starts nothing",
			setup:   func(f *tacklerFixture) { f.settings.MaxSpawns = 0 },
			wantErr: errTacklerUnavailable, wantErrText: "tackler_max_spawns: 0",
		},
		{
			name: "spent budget starts nothing",
			setup: func(f *tacklerFixture) {
				f.settings.MaxSpawns = 1
				other := sheddriver.TacklerAttempt{Number: 1, StopKey: sheddriver.StopKey{CurrentProducer: "PR-Gate", HistoryLength: 1, State: "awaiting"}}
				if err := sheddriver.WriteTacklerAttempt(f.attemptDir(1), other); err != nil {
					t.Fatal(err)
				}
			},
			wantErr: errTacklerUnavailable, wantErrText: "spawn budget spent", keepsTacklerDir: true,
		},
		{
			name:        "fresh stop records the attempt before the start and concludes the verdict",
			setup:       func(f *tacklerFixture) {},
			wantVerdict: sheddriver.TacklerVerdictResume, wantStarts: 1,
		},
		{
			name:      "start error reads hand-back died",
			setup:     func(f *tacklerFixture) { f.fake.startErr = errors.New("no pane") },
			wantCause: arbiterCauseDied, wantVerdict: sheddriver.TacklerVerdictHandBack,
		},
		{
			name: "timeout with no paused time stops the strand",
			setup: func(f *tacklerFixture) {
				f.fake.waitResult = shuttleengine.Result{Outcome: shuttleengine.OutcomeTimeout}
			},
			wantCause: arbiterCauseTimeout, wantVerdict: sheddriver.TacklerVerdictHandBack, wantStarts: 1, wantStops: 1,
		},
		{
			name: "timeout over a paused window re-attaches with the deadline moved out",
			setup: func(f *tacklerFixture) {
				f.paused = 5 * time.Minute
				f.fake.waitResult = shuttleengine.Result{Outcome: shuttleengine.OutcomeTimeout}
				f.fake.attachFound = true
			},
			wantVerdict: sheddriver.TacklerVerdictResume, wantStarts: 1, wantAttach: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newTacklerFixture(t)
			tt.setup(f)

			type captured struct {
				attempt sheddriver.TacklerAttempt
				marker  sheddriver.TacklerMarker
				prompt  string
			}
			var atWait captured
			var attachSpec shuttleengine.Spec
			writeVerdict := func() {
				writeTacklerFile(t, sheddriver.TacklerVerdictPath(f.attemptDir(1)), tacklerResumeVerdict)
			}
			f.fake.onWait = func(shuttleengine.Spec) {
				atWait.attempt, _ = sheddriver.ReadTacklerAttempt(f.attemptDir(1))
				atWait.marker, _, _ = sheddriver.ReadTacklerMarker(f.markerPath())
				data, _ := os.ReadFile(sheddriver.TacklerPromptPath(f.attemptDir(1)))
				atWait.prompt = string(data)
				if f.fake.waitResult.Outcome == "" {
					writeVerdict()
				}
			}
			f.fake.onAttach = func(spec shuttleengine.Spec) {
				attachSpec = spec
				writeVerdict()
			}

			envelope, err := f.settle()

			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) || !strings.Contains(err.Error(), tt.wantErrText) {
					t.Fatalf("settle() error = %v; want %v naming %q", err, tt.wantErr, tt.wantErrText)
				}
				if len(f.fake.starts) != 0 {
					t.Errorf("starts = %d; want none", len(f.fake.starts))
				}
				if _, statErr := os.Stat(f.attemptDir(1)); !tt.keepsTacklerDir && !errors.Is(statErr, os.ErrNotExist) {
					t.Errorf("attempt directory exists after a refusal (stat error %v); want none", statErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("settle() error = %v; want nil", err)
			}
			if envelope.Verdict != tt.wantVerdict || envelope.Cause != tt.wantCause {
				t.Errorf("envelope verdict, cause = %q, %q; want %q, %q", envelope.Verdict, envelope.Cause, tt.wantVerdict, tt.wantCause)
			}
			if len(f.fake.starts) != tt.wantStarts || f.fake.runStops != tt.wantStops || len(f.fake.attaches) != tt.wantAttach {
				t.Errorf("starts, run stops, attaches = %d, %d, %d; want %d, %d, %d", len(f.fake.starts), f.fake.runStops, len(f.fake.attaches), tt.wantStarts, tt.wantStops, tt.wantAttach)
			}
			if _, found, _ := sheddriver.ReadTacklerMarker(f.markerPath()); found {
				t.Error("marker still present after the call finished; want it removed")
			}
			if stored, found, err := sheddriver.ReadTacklerEnvelope(sheddriver.TacklerEnvelopePath(f.attemptDir(1))); err != nil || !found || stored != envelope {
				t.Errorf("envelope.json = %+v, found %v, error %v; want the returned envelope %+v", stored, found, err, envelope)
			}
			if tt.wantStarts == 0 {
				return
			}

			spec := f.fake.starts[0]
			verdictPath := sheddriver.TacklerVerdictPath(f.attemptDir(1))
			if spec.Role != sheddriver.TacklerRole || !reflect.DeepEqual(spec.OutputFiles, []string{verdictPath}) || spec.Model != "claude-resolved" || spec.Timeout != 10*time.Minute {
				t.Errorf("spec = role %q, outputs %v, model %q, timeout %v; want the tackler role over %q, the resolved model and 10m", spec.Role, spec.OutputFiles, spec.Model, spec.Timeout, verdictPath)
			}
			wantAttempt := sheddriver.TacklerAttempt{
				Number: 1, StopKey: sheddriver.StopKeyOf(f.status), Snapshot: f.snapshot, SpawnedAt: f.now, Deadline: f.now.Add(10 * time.Minute),
				StrandGUID: "guid-1", EnvelopePath: f.envelope, ReportPath: f.reportPath, Dir: f.attemptDir(1),
			}
			if !reflect.DeepEqual(atWait.attempt, wantAttempt) {
				t.Errorf("attempt.json at the wait = %+v; want %+v", atWait.attempt, wantAttempt)
			}
			if want := (sheddriver.TacklerMarker{Attempt: 1, StrandGUID: "guid-1", Deadline: f.now.Add(10 * time.Minute)}); atWait.marker != want {
				t.Errorf("marker at the wait = %+v; want %+v", atWait.marker, want)
			}
			if atWait.prompt != spec.Prompt || !strings.Contains(spec.Prompt, verdictPath) || !strings.Contains(spec.Prompt, f.envelope) {
				t.Error("prompt.md does not equal the spec prompt, or the prompt lacks the verdict and envelope paths")
			}
			if tt.wantAttach == 1 {
				wantDeadline := f.now.Add(10*time.Minute + f.paused)
				if attachSpec.Timeout != wantDeadline.Sub(f.now) {
					t.Errorf("re-attach timeout = %v; want %v, the deadline moved out by the paused time", attachSpec.Timeout, wantDeadline.Sub(f.now))
				}
				if moved, _ := sheddriver.ReadTacklerAttempt(f.attemptDir(1)); !moved.Deadline.Equal(wantDeadline) {
					t.Errorf("attempt.json deadline = %v; want %v", moved.Deadline, wantDeadline)
				}
			}
		})
	}
}

// TestTacklerCall_Rerun covers the call for a stop an attempt already judged: it returns or awaits that attempt's verdict, refuses an applied one and skips one already handed to the parent.
func TestTacklerCall_Rerun(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		setup func(f *tacklerFixture, attempt sheddriver.TacklerAttempt)
		// wantErr is the sentinel the call returns; empty rows expect an envelope.
		wantErr error
		// deadlinePassed gives the prior attempt a deadline a minute behind the clock.
		deadlinePassed bool
		wantCause      string
		wantAttempt    int
		wantStarts     int
		wantAttaches   int
		wantStopped    []string
		wantTimeout    time.Duration
	}{
		{
			name: "verdict on disk is returned with no start",
			setup: func(f *tacklerFixture, attempt sheddriver.TacklerAttempt) {
				writeTacklerFile(t, sheddriver.TacklerVerdictPath(attempt.Dir), tacklerResumeVerdict)
			},
			wantAttempt: 1,
		},
		{
			name: "live attempt within its deadline is attached for the remaining time",
			setup: func(f *tacklerFixture, attempt sheddriver.TacklerAttempt) {
				f.fake.attachFound = true
				f.fake.onAttach = func(shuttleengine.Spec) {
					writeTacklerFile(t, sheddriver.TacklerVerdictPath(attempt.Dir), tacklerResumeVerdict)
				}
			},
			wantAttempt: 1, wantAttaches: 1, wantTimeout: 5 * time.Minute,
		},
		{
			name: "attempt with no live run and no verdict reads hand-back missing",
			setup: func(f *tacklerFixture, attempt sheddriver.TacklerAttempt) {
				f.fake.attachFound = false
			},
			wantAttempt: 1, wantAttaches: 1, wantTimeout: 5 * time.Minute, wantCause: arbiterCauseMissing,
		},
		{
			name: "marker past its deadline is stopped and reads hand-back timeout",
			setup: func(f *tacklerFixture, attempt sheddriver.TacklerAttempt) {
				marker := sheddriver.TacklerMarker{Attempt: attempt.Number, StrandGUID: "marker-guid", Deadline: f.now.Add(-time.Minute)}
				if err := sheddriver.WriteTacklerMarker(f.markerPath(), marker); err != nil {
					t.Fatal(err)
				}
			},
			deadlinePassed: true,
			wantAttempt:    1, wantCause: arbiterCauseTimeout, wantStopped: []string{"marker-guid"},
		},
		{
			name: "applied attempt returns errTacklerResumeReturned",
			setup: func(f *tacklerFixture, attempt sheddriver.TacklerAttempt) {
				writeTacklerFile(t, sheddriver.TacklerVerdictPath(attempt.Dir), tacklerResumeVerdict)
				if err := sheddriver.MarkTacklerApplied(attempt.Dir); err != nil {
					t.Fatal(err)
				}
			},
			wantErr: errTacklerResumeReturned,
		},
		{
			name: "attempt already handed to the parent is skipped for a fresh one",
			setup: func(f *tacklerFixture, attempt sheddriver.TacklerAttempt) {
				writeTacklerFile(t, sheddriver.TacklerVerdictPath(attempt.Dir), tacklerHandBackVerdict)
				f.closed = func(time.Time) bool { return true }
				f.fake.onWait = func(shuttleengine.Spec) {
					writeTacklerFile(t, sheddriver.TacklerVerdictPath(f.attemptDir(2)), tacklerResumeVerdict)
				}
			},
			wantAttempt: 2, wantStarts: 1,
		},
		{
			name: "attempt already handed to the parent still counts toward the budget",
			setup: func(f *tacklerFixture, attempt sheddriver.TacklerAttempt) {
				f.settings.MaxSpawns = 1
				f.closed = func(time.Time) bool { return true }
			},
			wantErr: errTacklerUnavailable,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newTacklerFixture(t)
			deadlineIn := 5 * time.Minute
			if tt.deadlinePassed {
				deadlineIn = -time.Minute
			}
			tt.setup(f, f.priorAttempt(1, deadlineIn))

			envelope, err := f.settle()

			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("settle() error = %v; want %v", err, tt.wantErr)
				}
				if len(f.fake.starts) != 0 {
					t.Errorf("starts = %d; want none", len(f.fake.starts))
				}
				return
			}
			if err != nil {
				t.Fatalf("settle() error = %v; want nil", err)
			}
			if envelope.Attempt != tt.wantAttempt || envelope.Cause != tt.wantCause {
				t.Errorf("envelope attempt, cause = %d, %q; want %d, %q", envelope.Attempt, envelope.Cause, tt.wantAttempt, tt.wantCause)
			}
			if len(f.fake.starts) != tt.wantStarts || len(f.fake.attaches) != tt.wantAttaches {
				t.Errorf("starts, attaches = %d, %d; want %d, %d", len(f.fake.starts), len(f.fake.attaches), tt.wantStarts, tt.wantAttaches)
			}
			if !reflect.DeepEqual(f.fake.stoppedGUIDs, tt.wantStopped) {
				t.Errorf("stopped strands = %v; want %v", f.fake.stoppedGUIDs, tt.wantStopped)
			}
			if tt.wantAttaches == 1 && f.fake.attaches[0].Timeout != tt.wantTimeout {
				t.Errorf("attach timeout = %v; want %v", f.fake.attaches[0].Timeout, tt.wantTimeout)
			}
		})
	}
}

// TestTacklerCall_VerdictOutcomes covers the validation of the verdict a tackler wrote: which verdicts pass, and the cause each failing one reads as.
func TestTacklerCall_VerdictOutcomes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		verdict     string
		caseBrief   string
		state       shedengine.State
		wantVerdict string
		wantCause   string
		wantGoto    string
		wantBrief   bool
	}{
		{name: "resume at an error stop", verdict: tacklerResumeVerdict, wantVerdict: sheddriver.TacklerVerdictResume},
		{
			name: "resume with a goto to the current row", verdict: "verdict: resume\nreason: r\ngoto: Plan-Bouncer\n",
			wantVerdict: sheddriver.TacklerVerdictResume, wantGoto: "Plan-Bouncer",
		},
		{
			name: "resume with a goto to the last history entry's producer", verdict: "verdict: resume\nreason: r\ngoto: Plan-Write\n",
			wantVerdict: sheddriver.TacklerVerdictResume, wantGoto: "Plan-Write",
		},
		{
			name: "resume with a goto to another earlier row", verdict: "verdict: resume\nreason: r\ngoto: Plan-Draft\n",
			wantVerdict: sheddriver.TacklerVerdictHandBack, wantCause: arbiterCausePrecondition,
		},
		{
			name: "resume with a goto the shed refuses", verdict: "verdict: resume\nreason: r\ngoto: Plan-Bouncer\n", state: shedengine.StateAwaiting,
			wantVerdict: sheddriver.TacklerVerdictHandBack, wantCause: arbiterCausePrecondition,
		},
		{
			name: "resume at a blocked stop without a goto", verdict: tacklerResumeVerdict, state: shedengine.StateBlocked,
			wantVerdict: sheddriver.TacklerVerdictHandBack, wantCause: arbiterCausePrecondition,
		},
		{name: "hand-back with a question", verdict: tacklerHandBackVerdict, wantVerdict: sheddriver.TacklerVerdictHandBack},
		{
			name: "hand-back without a question", verdict: "verdict: hand-back\nreason: r\n",
			wantVerdict: sheddriver.TacklerVerdictHandBack, wantCause: tacklerCauseQuestion,
		},
		{
			name: "escalate with an empty case brief", verdict: "verdict: escalate\nreason: r\n", caseBrief: "",
			wantVerdict: sheddriver.TacklerVerdictHandBack, wantCause: tacklerCauseCaseBrief,
		},
		{
			name: "escalate with no case brief file", verdict: "verdict: escalate\nreason: r\n", caseBrief: "<absent>",
			wantVerdict: sheddriver.TacklerVerdictHandBack, wantCause: tacklerCauseCaseBrief,
		},
		{
			name: "escalate with a case brief", verdict: "verdict: escalate\nreason: r\n", caseBrief: "# Case\nhypothesis: the build is stale\n",
			wantVerdict: sheddriver.TacklerVerdictEscalate, wantBrief: true,
		},
		{
			name: "undecodable verdict", verdict: "verdict: [unclosed\n",
			wantVerdict: sheddriver.TacklerVerdictHandBack, wantCause: arbiterCauseUnparseable,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newTacklerFixture(t)
			if tt.state != "" {
				f.status.State = tt.state
			}
			f.fake.onWait = func(shuttleengine.Spec) {
				dir := f.attemptDir(1)
				writeTacklerFile(t, sheddriver.TacklerVerdictPath(dir), tt.verdict)
				if tt.caseBrief != "<absent>" && strings.HasPrefix(tt.verdict, "verdict: escalate") {
					writeTacklerFile(t, sheddriver.TacklerCaseBriefPath(dir), tt.caseBrief)
				}
			}

			envelope, err := f.settle()

			if err != nil {
				t.Fatalf("settle() error = %v; want nil", err)
			}
			if envelope.Verdict != tt.wantVerdict || envelope.Cause != tt.wantCause || envelope.Goto != tt.wantGoto {
				t.Errorf("envelope verdict, cause, goto = %q, %q, %q; want %q, %q, %q", envelope.Verdict, envelope.Cause, envelope.Goto, tt.wantVerdict, tt.wantCause, tt.wantGoto)
			}
			wantBriefPath := ""
			if tt.wantBrief {
				wantBriefPath = sheddriver.TacklerCaseBriefPath(f.attemptDir(1))
			}
			if envelope.CaseBriefPath != wantBriefPath {
				t.Errorf("envelope case brief path = %q; want %q", envelope.CaseBriefPath, wantBriefPath)
			}
			if tt.wantCause != "" && envelope.Question == "" {
				t.Error("a hand-back the call made carries no question for the run's parent")
			}
		})
	}
}
