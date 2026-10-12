// session_test.go — untagged Tier-1 tests for the agent's lifecycle over a fake shuttle and an injected clock.

package alignment

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/stencilkit"
)

// fakeRun is a started or probed agent whose Wait blocks until the test finishes it.
type fakeRun struct {
	guid    string
	release chan struct{}
	result  shuttleengine.Result
	err     error
}

func newFakeRun(guid string) *fakeRun {
	return &fakeRun{guid: guid, release: make(chan struct{})}
}

func (r *fakeRun) Wait() (shuttleengine.Result, error) {
	<-r.release
	return r.result, r.err
}

func (r *fakeRun) StrandGUID() string { return r.guid }

// finish releases Wait with result and err.
func (r *fakeRun) finish(result shuttleengine.Result, err error) {
	r.result, r.err = result, err
	close(r.release)
}

// fakeShuttle records starts, probes and stops, and hands out runs the test finishes.
type fakeShuttle struct {
	mu       sync.Mutex
	starts   []shuttleengine.Spec
	probes   []shuttleengine.Spec
	stops    []string
	runs     []*fakeRun
	startErr error
	stopErr  error
	// probeRun is the run Probe finds; nil means none is live.
	probeRun *fakeRun
	// startHook runs inside Start, after the spec is recorded.
	startHook func(spec shuttleengine.Spec)
}

func (f *fakeShuttle) Start(spec shuttleengine.Spec) (Run, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.starts = append(f.starts, spec)
	if f.startErr != nil {
		return nil, f.startErr
	}
	if f.startHook != nil {
		f.startHook(spec)
	}
	run := newFakeRun("guid-" + string(rune('0'+len(f.starts))))
	f.runs = append(f.runs, run)
	return run, nil
}

func (f *fakeShuttle) Probe(spec shuttleengine.Spec) (Run, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.probes = append(f.probes, spec)
	if f.probeRun == nil {
		return nil, false, nil
	}
	return f.probeRun, true, nil
}

func (f *fakeShuttle) StopStrand(guid string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.stops = append(f.stops, guid)
	return f.stopErr
}

// sessionHarness is a Session over a fake shuttle, a seeded stencils directory, a fixed token and a movable clock.
type sessionHarness struct {
	root    string
	stencil string
	fake    *fakeShuttle
	session *Session
	now     time.Time
}

func newSessionHarness(t *testing.T) *sessionHarness {
	t.Helper()
	h := &sessionHarness{
		root:    filepath.Join(t.TempDir(), "alignment", "parent-review"),
		stencil: t.TempDir(),
		fake:    &fakeShuttle{},
		now:     time.Date(2026, 3, 3, 12, 0, 0, 0, time.UTC),
	}
	stencilkit.SeedInto(t, h.stencil)
	h.session = h.newSession()
	return h
}

// newSession returns a fresh Session over the harness's root and fake, standing in for a driver restart.
func (h *sessionHarness) newSession() *Session {
	return NewSession(SessionConfig{
		Root:        h.root,
		StencilsDir: h.stencil,
		Shuttle:     h.fake,
		Now:         func() time.Time { return h.now },
		Token:       func() string { return "tok" },
	})
}

func (h *sessionHarness) spawnInput(key string) SpawnInput {
	return SpawnInput{
		Key: key,
		Prompt: PromptInput{
			Point:              PointParentReview,
			Slug:               "task-a",
			Worktree:           "/hub/task-a",
			DecisionRecordPath: "/hub/task-a/_lyx/discussion/decision-record.md",
			SupportLogPath:     "/hub/task-a/_lyx/discussion/support-log.md",
			PlanDir:            "/hub/task-a/_lyx/plan",
			ParentBranch:       "main",
		},
		Settings: Settings{Model: "claude-opus", Timeout: 20 * time.Minute},
		Snapshot: "snap",
	}
}

// begin spawns key and fails the test on an error.
func (h *sessionHarness) begin(t *testing.T, key string) Attempt {
	t.Helper()
	attempt, err := h.session.Begin(h.spawnInput(key))
	if err != nil {
		t.Fatalf("Begin(%q) error = %v; want nil", key, err)
	}
	return attempt
}

// writeOutput writes the attempt's report and then its verdict.
func writeOutput(t *testing.T, a Attempt, verdict string) {
	t.Helper()
	writeFile(t, a.ReportPath(), "# Report\n")
	writeFile(t, a.VerdictPath(), verdict)
}

func (h *sessionHarness) poll(t *testing.T, a Attempt) Outcome {
	t.Helper()
	outcome, err := h.session.Poll(a)
	if err != nil {
		t.Fatalf("Poll() error = %v; want nil", err)
	}
	return outcome
}

// waitWatch waits until attempt number's run has ended and its result is stored.
func (h *sessionHarness) waitWatch(t *testing.T, number int) {
	t.Helper()
	select {
	case <-h.session.watchDone(number):
	case <-time.After(10 * time.Second):
		t.Fatalf("the watch of attempt %d never ended", number)
	}
}

// TestSession verifies the agent's lifecycle: one spawn per key, a non-blocking poll that reads the agent's verdict or classifies how it ended, a close that stops the strand, a restart that probes once, and the failure paths of a spawn.
func TestSession(t *testing.T) {
	t.Parallel()
	const approve = "verdict: approve\nreason: in scope\n"

	t.Run("the first Begin writes its records, starts one spec and polls pending until the files exist", func(t *testing.T) {
		t.Parallel()
		h := newSessionHarness(t)
		attempt := h.begin(t, "round-1")

		stored, err := ReadAttempt(attempt.Dir)
		if err != nil || stored.Number != 1 || stored.Key != "round-1" || stored.Token != "tok" || stored.StrandGUID != "guid-1" || stored.Snapshot != "snap" || !stored.Deadline.Equal(h.now.Add(20*time.Minute)) {
			t.Fatalf("stored attempt = %+v, %v; want number 1, key round-1, token tok, strand guid-1, the snapshot and a deadline 20 minutes out", stored, err)
		}
		prompt, err := os.ReadFile(attempt.PromptPath())
		if err != nil || !strings.Contains(string(prompt), attempt.VerdictPath()) || !strings.Contains(string(prompt), attempt.ReportPath()) {
			t.Errorf("prompt.md = %q, %v; want it to name the verdict and report paths", prompt, err)
		}
		if len(h.fake.starts) != 1 || h.fake.starts[0].Role != Role || strings.Join(h.fake.starts[0].OutputFiles, "|") != attempt.VerdictPath()+"|"+attempt.ReportPath() {
			t.Fatalf("starts = %+v; want one spec with the alignment role and the verdict and report as output files", h.fake.starts)
		}

		if got := h.poll(t, attempt); got.Done {
			t.Errorf("Poll() before any file = %+v; want pending", got)
		}
		writeOutput(t, attempt, approve)
		if got := h.poll(t, attempt); !got.Done || got.Cause != "" || got.Verdict.Verdict != VerdictApprove {
			t.Errorf("Poll() after the files = %+v; want done with the approve", got)
		}
	})

	t.Run("a second Begin with the same key starts nothing, closed or open", func(t *testing.T) {
		t.Parallel()
		h := newSessionHarness(t)
		first := h.begin(t, "round-1")
		if again := h.begin(t, "round-1"); again.Number != first.Number || len(h.fake.starts) != 1 {
			t.Fatalf("second Begin = attempt %d with %d starts; want attempt %d and 1 start", again.Number, len(h.fake.starts), first.Number)
		}
		if err := h.session.Close(first, "fell back"); err != nil {
			t.Fatalf("Close() error = %v; want nil", err)
		}
		if again := h.begin(t, "round-1"); again.Number != first.Number || len(h.fake.starts) != 1 {
			t.Errorf("Begin after Close = attempt %d with %d starts; want attempt %d and 1 start", again.Number, len(h.fake.starts), first.Number)
		}
	})

	t.Run("a file planted at the next attempt's verdict path is gone after Begin", func(t *testing.T) {
		t.Parallel()
		h := newSessionHarness(t)
		h.begin(t, "round-1")
		planted := Attempt{Dir: filepath.Join(h.root, "attempt-2"), Token: "tok"}
		writeFile(t, planted.VerdictPath(), approve)

		second := h.begin(t, "round-2")
		if _, err := os.Stat(second.VerdictPath()); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("planted verdict stat error = %v; want it gone", err)
		}
		if got := h.poll(t, second); got.Done {
			t.Errorf("Poll() = %+v; want pending", got)
		}
	})

	t.Run("the deadline stops the strand and gives a timeout", func(t *testing.T) {
		t.Parallel()
		h := newSessionHarness(t)
		attempt := h.begin(t, "round-1")

		h.now = h.now.Add(20 * time.Minute)
		got := h.poll(t, attempt)
		if !got.Done || got.Cause != CauseTimeout || len(h.fake.stops) != 1 || h.fake.stops[0] != "guid-1" {
			t.Errorf("Poll() at the deadline = %+v with stops %v; want a timeout and one stop of guid-1", got, h.fake.stops)
		}
	})

	t.Run("a run that ended without a verdict is classified through the watch", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name   string
			result shuttleengine.Result
			err    error
			want   string
		}{
			{"died", shuttleengine.Result{Outcome: shuttleengine.OutcomeDied}, nil, CauseDied},
			{"done with no files", shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}, nil, CauseMissing},
			{"a shuttle timeout before the deadline", shuttleengine.Result{Outcome: shuttleengine.OutcomeTimeout}, nil, CauseTimeout},
			{"a wait error", shuttleengine.Result{}, errors.New("reed is gone"), CauseDied},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				h := newSessionHarness(t)
				attempt := h.begin(t, "round-1")
				h.fake.runs[0].finish(tt.result, tt.err)
				h.waitWatch(t, attempt.Number)

				if got := h.poll(t, attempt); !got.Done || got.Cause != tt.want {
					t.Errorf("Poll() = %+v; want done with cause %q", got, tt.want)
				}
			})
		}
	})

	t.Run("an escalate with no question gives an invalid verdict", func(t *testing.T) {
		t.Parallel()
		h := newSessionHarness(t)
		attempt := h.begin(t, "round-1")
		writeOutput(t, attempt, "verdict: escalate\nreason: unsure\n")

		if got := h.poll(t, attempt); !got.Done || got.Cause != CauseInvalid || !strings.Contains(got.Detail, "needs a question") {
			t.Errorf("Poll() = %+v; want done with cause invalid naming the missing question", got)
		}
	})

	t.Run("Close stops the strand once and a verdict written afterwards polls as closed", func(t *testing.T) {
		t.Parallel()
		h := newSessionHarness(t)
		attempt := h.begin(t, "round-1")
		if err := h.session.Close(attempt, "the parent gave a verdict"); err != nil {
			t.Fatalf("Close() error = %v; want nil", err)
		}
		if len(h.fake.stops) != 1 || h.fake.stops[0] != "guid-1" {
			t.Fatalf("stops = %v; want guid-1 once", h.fake.stops)
		}

		writeOutput(t, attempt, approve)
		if got := h.poll(t, attempt); !got.Done || got.Cause != CauseClosed || got.Detail != "the parent gave a verdict" {
			t.Errorf("Poll() = %+v; want done with cause closed", got)
		}

		if err := h.session.Close(attempt, "another reason"); err != nil {
			t.Fatalf("second Close() error = %v; want nil", err)
		}
		stored, err := ReadAttempt(attempt.Dir)
		if err != nil || stored.ClosedReason != "the parent gave a verdict" || len(h.fake.stops) != 1 {
			t.Errorf("after a second Close: reason %q, stops %v, error %v; want the first reason and no further stop", stored.ClosedReason, h.fake.stops, err)
		}
	})

	t.Run("a Close whose stop fails returns the error and leaves the attempt open", func(t *testing.T) {
		t.Parallel()
		h := newSessionHarness(t)
		attempt := h.begin(t, "round-1")
		h.fake.stopErr = errors.New("reed refused")

		if err := h.session.Close(attempt, "fell back"); err == nil || !strings.Contains(err.Error(), "reed refused") {
			t.Fatalf("Close() error = %v; want the stop error", err)
		}
		if stored, err := ReadAttempt(attempt.Dir); err != nil || stored.Closed {
			t.Errorf("stored attempt = %+v, %v; want it not closed", stored, err)
		}
	})

	t.Run("a fresh Session over the same root probes once, and a found run is pending", func(t *testing.T) {
		t.Parallel()
		h := newSessionHarness(t)
		attempt := h.begin(t, "round-1")
		restarted := h.newSession()
		h.fake.probeRun = newFakeRun("guid-1")

		for range 2 {
			if got, err := restarted.Poll(attempt); err != nil || got.Done {
				t.Fatalf("Poll() after a restart = %+v, %v; want pending", got, err)
			}
		}
		if len(h.fake.probes) != 1 {
			t.Errorf("probes = %d; want exactly 1", len(h.fake.probes))
		}
	})

	t.Run("a fresh Session that finds no live run gives died", func(t *testing.T) {
		t.Parallel()
		h := newSessionHarness(t)
		attempt := h.begin(t, "round-1")
		restarted := h.newSession()

		got, err := restarted.Poll(attempt)
		if err != nil || !got.Done || got.Cause != CauseDied {
			t.Errorf("Poll() after a restart with no live run = %+v, %v; want done with cause died", got, err)
		}
	})

	t.Run("a Start that fails closes the attempt with its error and a second Begin starts nothing", func(t *testing.T) {
		t.Parallel()
		h := newSessionHarness(t)
		h.fake.startErr = errors.New("provider not ready")

		if _, err := h.session.Begin(h.spawnInput("round-1")); err == nil || !strings.Contains(err.Error(), "provider not ready") {
			t.Fatalf("Begin() error = %v; want the start error", err)
		}
		attempts, err := h.session.Attempts()
		if err != nil || len(attempts) != 1 || !attempts[0].Closed || !strings.Contains(attempts[0].ClosedReason, "provider not ready") {
			t.Fatalf("attempts = %+v, %v; want one closed attempt with the start error as its reason", attempts, err)
		}
		if again := h.begin(t, "round-1"); again.Number != 1 || len(h.fake.starts) != 1 {
			t.Errorf("second Begin = attempt %d with %d starts; want attempt 1 and no further start", again.Number, len(h.fake.starts))
		}
	})

	t.Run("a failed strand record stops the strand once and returns the error", func(t *testing.T) {
		t.Parallel()
		h := newSessionHarness(t)
		h.fake.startHook = func(spec shuttleengine.Spec) {
			dir := filepath.Dir(spec.OutputFiles[0])
			if err := os.RemoveAll(dir); err != nil {
				t.Errorf("RemoveAll(%q) = %v; want nil", dir, err)
			}
			if err := os.WriteFile(dir, []byte("not a directory"), 0o644); err != nil {
				t.Errorf("WriteFile(%q) = %v; want nil", dir, err)
			}
		}

		if _, err := h.session.Begin(h.spawnInput("round-1")); err == nil {
			t.Fatal("Begin() error = nil; want the record error")
		}
		if len(h.fake.stops) != 1 || h.fake.stops[0] != "guid-1" {
			t.Errorf("stops = %v; want guid-1 once", h.fake.stops)
		}
	})

	t.Run("MarkApplied round-trips through Attempts and keeps a key a newer binary wrote", func(t *testing.T) {
		t.Parallel()
		h := newSessionHarness(t)
		attempt := h.begin(t, "round-1")
		path := filepath.Join(attempt.Dir, attemptFileName)
		written, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("ReadFile(%q) = %v; want nil", path, err)
		}
		withExtra := strings.Replace(string(written), "{", `{"newer_key": "kept",`, 1)
		if err := os.WriteFile(path, []byte(withExtra), 0o644); err != nil {
			t.Fatalf("WriteFile(%q) = %v; want nil", path, err)
		}

		if err := h.session.MarkApplied(attempt, VerdictApprove, true); err != nil {
			t.Fatalf("MarkApplied() error = %v; want nil", err)
		}
		attempts, err := h.session.Attempts()
		if err != nil || len(attempts) != 1 || attempts[0].Applied != VerdictApprove || !attempts[0].Notified {
			t.Errorf("Attempts() = %+v, %v; want one attempt applied as approve and notified", attempts, err)
		}
		rewritten, err := os.ReadFile(path)
		if err != nil || !strings.Contains(string(rewritten), `"newer_key": "kept"`) {
			t.Errorf("attempt.json after MarkApplied = %s, %v; want the newer_key kept", rewritten, err)
		}
	})
}
