// poll_test.go covers Classify's decision table (all five classification
// outcomes: report-present terminal, dead/asking, dead/timeout, dead/died,
// and non-terminal running), TurnEnded against a fake shuttleengine.Engine
// double, StrandLive against a fake shuttleengine.MuxOps double, and
// PollUntilTerminal's long-poll loop against a fake clock — a terminal
// mid-wait result short-circuits, a deadline returns the last running
// digest, and a gather error propagates. This file lives in package
// builderengine (not builderengine_test) because the clock/realClock seam
// is unexported (TurnEnded and StrandLive are themselves exported, so
// buildercli's own `poll` verb calls them directly).

package builderengine

import (
	"errors"
	"os"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/muxengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

func TestClassify_DecisionTable(t *testing.T) {
	t.Parallel()

	doneReport := &Report{Batch: "03-x", Status: ReportStatusDone, Tests: ReportTestsGreen}
	stuckReport := &Report{Batch: "03-x", Status: ReportStatusStuck, Tests: ReportTestsRed, StuckReason: "compile error persists"}

	tests := []struct {
		name         string
		in           ClassifyInputs
		wantStatus   string
		wantDead     string
		wantTerminal bool
	}{
		{
			name:         "report present done wins regardless of other fields",
			in:           ClassifyInputs{BatchNumber: 3, BatchSlug: "x", Report: doneReport, TurnEnded: true, StrandLive: false, Elapsed: time.Hour, BatchTimeout: time.Minute},
			wantStatus:   DigestStatusDone,
			wantTerminal: true,
		},
		{
			name:         "report present stuck",
			in:           ClassifyInputs{BatchNumber: 3, BatchSlug: "x", Report: stuckReport},
			wantStatus:   DigestStatusStuck,
			wantTerminal: true,
		},
		{
			name:         "no report, turn ended -> dead asking",
			in:           ClassifyInputs{BatchNumber: 4, BatchSlug: "y", TurnEnded: true, StrandLive: true, Elapsed: time.Minute, BatchTimeout: time.Hour},
			wantStatus:   DigestStatusDead,
			wantDead:     DeadReasonAsking,
			wantTerminal: true,
		},
		{
			name:         "no report, elapsed past timeout -> dead timeout",
			in:           ClassifyInputs{BatchNumber: 5, BatchSlug: "z", TurnEnded: false, StrandLive: true, Elapsed: 2 * time.Hour, BatchTimeout: time.Hour},
			wantStatus:   DigestStatusDead,
			wantDead:     DeadReasonTimeout,
			wantTerminal: true,
		},
		{
			name:         "no report, turn in progress, strand gone -> dead died",
			in:           ClassifyInputs{BatchNumber: 6, BatchSlug: "w", TurnEnded: false, StrandLive: false, Elapsed: time.Minute, BatchTimeout: time.Hour},
			wantStatus:   DigestStatusDead,
			wantDead:     DeadReasonDied,
			wantTerminal: true,
		},
		{
			name:         "no report, turn in progress, strand live -> running",
			in:           ClassifyInputs{BatchNumber: 7, BatchSlug: "v", TurnEnded: false, StrandLive: true, Elapsed: 42 * time.Second, BatchTimeout: time.Hour},
			wantStatus:   DigestStatusRunning,
			wantTerminal: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			digest, terminal := Classify(tt.in)
			if digest.Status != tt.wantStatus {
				t.Errorf("Classify().Status = %q; want %q", digest.Status, tt.wantStatus)
			}
			if digest.DeadReason != tt.wantDead {
				t.Errorf("Classify().DeadReason = %q; want %q", digest.DeadReason, tt.wantDead)
			}
			if terminal != tt.wantTerminal {
				t.Errorf("Classify() terminal = %v; want %v", terminal, tt.wantTerminal)
			}
		})
	}
}

func TestClassify_RunningSnapshotCarriesOnlyBatchStatusElapsed(t *testing.T) {
	t.Parallel()

	digest, terminal := Classify(ClassifyInputs{
		BatchNumber: 9, BatchSlug: "fresh", TurnEnded: false, StrandLive: true,
		Elapsed: 5 * time.Second, BatchTimeout: time.Hour,
	})
	if terminal {
		t.Fatalf("Classify() terminal = true; want false")
	}
	if digest.Batch != "09-fresh" {
		t.Errorf("Batch = %q; want %q", digest.Batch, "09-fresh")
	}
	if digest.ElapsedS != 5 {
		t.Errorf("ElapsedS = %d; want 5", digest.ElapsedS)
	}
	if digest.Tests != "" || digest.StuckReason != "" || digest.FilesChanged != 0 || digest.Dirty || digest.OutOfScope != nil || digest.DriftUnreported != nil {
		t.Errorf("running snapshot carries an unexpected populated field: %+v", digest)
	}
}

// fakeEngine is a minimal shuttleengine.Engine double for TurnEnded: only
// ParseEvents is scripted (a canned Events slice or a canned error), since
// TurnEnded never calls any other method.
type fakeEngine struct {
	events []shuttleengine.Event
	err    error
}

func (e *fakeEngine) Prepare(runDir string, spec shuttleengine.Spec, cfg shuttleengine.Config) (shuttleengine.Launch, error) {
	return shuttleengine.Launch{}, nil
}
func (e *fakeEngine) ParseEvents(data []byte) ([]shuttleengine.Event, error) {
	if e.err != nil {
		return nil, e.err
	}
	return e.events, nil
}
func (e *fakeEngine) Startup(capture string) shuttleengine.StartupState {
	return shuttleengine.StartupPending
}
func (e *fakeEngine) InterruptSequence() []shuttleengine.PaneInput      { return nil }
func (e *fakeEngine) TrustDismissSequence() []shuttleengine.PaneInput   { return nil }
func (e *fakeEngine) ComposeSend(text string) []shuttleengine.PaneInput { return nil }

var _ shuttleengine.Engine = (*fakeEngine)(nil)

func TestTurnEnded(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	eventsPath := dir + "/events.jsonl"

	t.Run("missing events file is false, nil", func(t *testing.T) {
		ended, err := TurnEnded(eventsPath, &fakeEngine{})
		if err != nil {
			t.Fatalf("TurnEnded() error = %v; want nil", err)
		}
		if ended {
			t.Errorf("TurnEnded() = true for a missing events file; want false")
		}
	})

	if err := os.WriteFile(eventsPath, []byte("irrelevant bytes; fakeEngine ignores them"), 0o644); err != nil {
		t.Fatalf("write events file %s: %v", eventsPath, err)
	}

	t.Run("no Stop event is false", func(t *testing.T) {
		ended, err := TurnEnded(eventsPath, &fakeEngine{events: []shuttleengine.Event{{Kind: shuttleengine.EventAsk, Message: "still working"}}})
		if err != nil {
			t.Fatalf("TurnEnded() error = %v; want nil", err)
		}
		if ended {
			t.Errorf("TurnEnded() = true with only an EventAsk; want false")
		}
	})

	t.Run("a Stop event anywhere in the batch is true", func(t *testing.T) {
		ended, err := TurnEnded(eventsPath, &fakeEngine{events: []shuttleengine.Event{
			{Kind: shuttleengine.EventAsk, Message: "mid-turn probe"},
			{Kind: shuttleengine.EventStop, Message: "final message"},
		}})
		if err != nil {
			t.Fatalf("TurnEnded() error = %v; want nil", err)
		}
		if !ended {
			t.Errorf("TurnEnded() = false with a Stop event present; want true")
		}
	})

	t.Run("a ParseEvents error propagates", func(t *testing.T) {
		wantErr := errors.New("boom")
		_, err := TurnEnded(eventsPath, &fakeEngine{err: wantErr})
		if err == nil {
			t.Fatalf("TurnEnded() error = nil; want a wrapped error")
		}
		if !errors.Is(err, wantErr) {
			t.Errorf("TurnEnded() error = %v; want it to wrap %v", err, wantErr)
		}
	})
}

// fakeMux is a minimal shuttleengine.MuxOps double for StrandLive: only
// Status is scripted (a canned StatusResult or a canned error), since
// StrandLive never calls any other method.
type fakeMux struct {
	status muxengine.StatusResult
	err    error
}

func (m *fakeMux) AddStrand(spec muxengine.AddSpec) (muxengine.Strand, error) {
	return muxengine.Strand{}, nil
}
func (m *fakeMux) RemoveStrand(guid string, recursive bool) (muxengine.Removed, error) {
	return muxengine.Removed{}, nil
}
func (m *fakeMux) Status() (muxengine.StatusResult, error) {
	if m.err != nil {
		return muxengine.StatusResult{}, m.err
	}
	return m.status, nil
}
func (m *fakeMux) SendText(guid, text string, submit bool) error { return nil }
func (m *fakeMux) SendKey(guid, key string) error                { return nil }
func (m *fakeMux) CapturePane(guid string) (string, error)       { return "", nil }

var _ shuttleengine.MuxOps = (*fakeMux)(nil)

func TestStrandLive(t *testing.T) {
	t.Parallel()

	t.Run("guid present and live", func(t *testing.T) {
		mux := &fakeMux{status: muxengine.StatusResult{Strands: []muxengine.StrandStatus{
			{GUID: "other", Live: false},
			{GUID: "target", Live: true},
		}}}
		live, err := StrandLive(mux, "target")
		if err != nil {
			t.Fatalf("StrandLive() error = %v; want nil", err)
		}
		if !live {
			t.Errorf("StrandLive() = false; want true")
		}
	})

	t.Run("guid present and not live", func(t *testing.T) {
		mux := &fakeMux{status: muxengine.StatusResult{Strands: []muxengine.StrandStatus{{GUID: "target", Live: false}}}}
		live, err := StrandLive(mux, "target")
		if err != nil {
			t.Fatalf("StrandLive() error = %v; want nil", err)
		}
		if live {
			t.Errorf("StrandLive() = true; want false")
		}
	})

	t.Run("guid absent from Status is false, nil", func(t *testing.T) {
		mux := &fakeMux{status: muxengine.StatusResult{Strands: []muxengine.StrandStatus{{GUID: "someone-else", Live: true}}}}
		live, err := StrandLive(mux, "target")
		if err != nil {
			t.Fatalf("StrandLive() error = %v; want nil", err)
		}
		if live {
			t.Errorf("StrandLive() = true for an absent guid; want false")
		}
	})

	t.Run("mux Status error propagates", func(t *testing.T) {
		wantErr := errors.New("mux unreachable")
		_, err := StrandLive(&fakeMux{err: wantErr}, "target")
		if err == nil {
			t.Fatalf("StrandLive() error = nil; want a wrapped error")
		}
		if !errors.Is(err, wantErr) {
			t.Errorf("StrandLive() error = %v; want it to wrap %v", err, wantErr)
		}
	})
}

// fakeClock is a package-local, scriptable clock double for
// PollUntilTerminal: Now starts at a fixed base and only advances when
// Sleep is called, so a test controls exactly how many ticks elapse before
// a fixed wait budget is exceeded, without ever blocking for real.
type fakeClock struct {
	now time.Time
}

func (c *fakeClock) Now() time.Time { return c.now }
func (c *fakeClock) Sleep(d time.Duration) {
	c.now = c.now.Add(d)
}

var _ clock = (*fakeClock)(nil)

func TestPollUntilTerminal_TerminalMidWaitReturnsEarly(t *testing.T) {
	t.Parallel()

	calls := 0
	gather := func() (Digest, bool, error) {
		calls++
		if calls < 3 {
			return Digest{Batch: "01-x", Status: DigestStatusRunning}, false, nil
		}
		return Digest{Batch: "01-x", Status: DigestStatusDone}, true, nil
	}

	digest, err := PollUntilTerminal(gather, time.Hour, &fakeClock{now: time.Unix(0, 0)})
	if err != nil {
		t.Fatalf("PollUntilTerminal() error = %v; want nil", err)
	}
	if digest.Status != DigestStatusDone {
		t.Errorf("PollUntilTerminal().Status = %q; want %q", digest.Status, DigestStatusDone)
	}
	if calls != 3 {
		t.Errorf("gather called %d times; want exactly 3 (short-circuit on terminal)", calls)
	}
}

func TestPollUntilTerminal_DeadlineReturnsRunning(t *testing.T) {
	t.Parallel()

	running := Digest{Batch: "01-x", Status: DigestStatusRunning, ElapsedS: 99}
	gather := func() (Digest, bool, error) {
		return running, false, nil
	}

	digest, err := PollUntilTerminal(gather, 3*time.Second, &fakeClock{now: time.Unix(0, 0)})
	if err != nil {
		t.Fatalf("PollUntilTerminal() error = %v; want nil", err)
	}
	if digest.Status != DigestStatusRunning {
		t.Errorf("PollUntilTerminal().Status = %q; want %q", digest.Status, DigestStatusRunning)
	}
	if digest.ElapsedS != 99 {
		t.Errorf("PollUntilTerminal().ElapsedS = %d; want 99", digest.ElapsedS)
	}
}

func TestPollUntilTerminal_GatherErrorPropagates(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("gather failed")
	gather := func() (Digest, bool, error) {
		return Digest{}, false, wantErr
	}

	_, err := PollUntilTerminal(gather, time.Hour, &fakeClock{now: time.Unix(0, 0)})
	if err == nil {
		t.Fatalf("PollUntilTerminal() error = nil; want a propagated error")
	}
	if !errors.Is(err, wantErr) {
		t.Errorf("PollUntilTerminal() error = %v; want it to wrap %v", err, wantErr)
	}
}
