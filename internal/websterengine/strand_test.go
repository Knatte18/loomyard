// strand_test.go covers StrandLive against a fake shuttleengine.ReedOps
// double (present/live, present/not-live, absent), TurnEnded against a fake
// shuttleengine.Engine double (stop event, no stop event, missing events
// file, a ParseEvents error), and removeStrandIfLive's three cases (live,
// not-live, a failed removal of a live strand). Tier 1: no git, only local
// fakes.

package websterengine

import (
	"errors"
	"os"
	"testing"

	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// fakeReed is a minimal shuttleengine.ReedOps double: Status and RemoveStrand
// are scripted; every other method is unreached by this file's tests.
type fakeReed struct {
	status       reedengine.StatusResult
	statusErr    error
	removeErr    error
	removeCalls  int
	removedGUIDs []string
}

func (m *fakeReed) AddStrand(spec reedengine.AddSpec) (reedengine.Strand, error) {
	return reedengine.Strand{}, nil
}
func (m *fakeReed) RemoveStrand(guid string, recursive bool) (reedengine.Removed, error) {
	m.removeCalls++
	m.removedGUIDs = append(m.removedGUIDs, guid)
	if m.removeErr != nil {
		return reedengine.Removed{}, m.removeErr
	}
	return reedengine.Removed{}, nil
}
func (m *fakeReed) Status() (reedengine.StatusResult, error) {
	if m.statusErr != nil {
		return reedengine.StatusResult{}, m.statusErr
	}
	return m.status, nil
}
func (m *fakeReed) SendText(guid, text string, submit bool) error { return nil }
func (m *fakeReed) SendKey(guid, key string) error                { return nil }
func (m *fakeReed) CapturePane(guid string) (string, error)       { return "", nil }

var _ shuttleengine.ReedOps = (*fakeReed)(nil)

func TestStrandLive(t *testing.T) {
	t.Parallel()

	t.Run("guid present and live", func(t *testing.T) {
		reed := &fakeReed{status: reedengine.StatusResult{Strands: []reedengine.StrandStatus{
			{GUID: "other", Live: false},
			{GUID: "target", Live: true},
		}}}
		live, err := StrandLive(reed, "target")
		if err != nil {
			t.Fatalf("StrandLive() error = %v; want nil", err)
		}
		if !live {
			t.Errorf("StrandLive() = false; want true")
		}
	})

	t.Run("guid present and not live", func(t *testing.T) {
		reed := &fakeReed{status: reedengine.StatusResult{Strands: []reedengine.StrandStatus{{GUID: "target", Live: false}}}}
		live, err := StrandLive(reed, "target")
		if err != nil {
			t.Fatalf("StrandLive() error = %v; want nil", err)
		}
		if live {
			t.Errorf("StrandLive() = true; want false")
		}
	})

	t.Run("guid absent from Status is false, nil", func(t *testing.T) {
		reed := &fakeReed{status: reedengine.StatusResult{Strands: []reedengine.StrandStatus{{GUID: "someone-else", Live: true}}}}
		live, err := StrandLive(reed, "target")
		if err != nil {
			t.Fatalf("StrandLive() error = %v; want nil", err)
		}
		if live {
			t.Errorf("StrandLive() = true for an absent guid; want false")
		}
	})

	t.Run("reed Status error propagates", func(t *testing.T) {
		wantErr := errors.New("reed unreachable")
		_, err := StrandLive(&fakeReed{statusErr: wantErr}, "target")
		if err == nil {
			t.Fatalf("StrandLive() error = nil; want a wrapped error")
		}
		if !errors.Is(err, wantErr) {
			t.Errorf("StrandLive() error = %v; want it to wrap %v", err, wantErr)
		}
	})
}

// fakeEngine is a minimal shuttleengine.Engine double for TurnEnded: only
// ParseEvents is scripted, since TurnEnded never calls any other method.
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
func (e *fakeEngine) AuditForks(sessionID, workdir string) (shuttleengine.ForkAudit, error) {
	return shuttleengine.ForkAudit{}, nil
}
func (e *fakeEngine) AuditForksIncremental(sessionID, workdir string, seenTranscripts map[string]bool) (shuttleengine.ForkAudit, error) {
	return shuttleengine.ForkAudit{}, nil
}
func (e *fakeEngine) ModelSwitchSequence(model string) []shuttleengine.PaneInput {
	return nil
}

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

func TestRemoveStrandIfLive(t *testing.T) {
	t.Parallel()

	t.Run("live strand is removed", func(t *testing.T) {
		reed := &fakeReed{status: reedengine.StatusResult{Strands: []reedengine.StrandStatus{{GUID: "target", Live: true}}}}
		if err := removeStrandIfLive(reed, "target"); err != nil {
			t.Fatalf("removeStrandIfLive() error = %v; want nil", err)
		}
		if reed.removeCalls != 1 {
			t.Errorf("RemoveStrand called %d time(s); want exactly 1", reed.removeCalls)
		}
		if len(reed.removedGUIDs) != 1 || reed.removedGUIDs[0] != "target" {
			t.Errorf("removedGUIDs = %v; want [target]", reed.removedGUIDs)
		}
	})

	t.Run("not-live strand is a no-op", func(t *testing.T) {
		reed := &fakeReed{status: reedengine.StatusResult{Strands: []reedengine.StrandStatus{{GUID: "target", Live: false}}}}
		if err := removeStrandIfLive(reed, "target"); err != nil {
			t.Fatalf("removeStrandIfLive() error = %v; want nil", err)
		}
		if reed.removeCalls != 0 {
			t.Errorf("RemoveStrand called %d time(s); want 0 for a not-live strand", reed.removeCalls)
		}
	})

	t.Run("absent guid (StrandLive false) is a no-op", func(t *testing.T) {
		reed := &fakeReed{status: reedengine.StatusResult{}}
		if err := removeStrandIfLive(reed, "target"); err != nil {
			t.Fatalf("removeStrandIfLive() error = %v; want nil", err)
		}
		if reed.removeCalls != 0 {
			t.Errorf("RemoveStrand called %d time(s); want 0 for an absent strand", reed.removeCalls)
		}
	})

	t.Run("a StrandLive error is treated as not-live", func(t *testing.T) {
		reed := &fakeReed{statusErr: errors.New("reed unreachable")}
		if err := removeStrandIfLive(reed, "target"); err != nil {
			t.Fatalf("removeStrandIfLive() error = %v; want nil (a StrandLive error is swallowed as not-live)", err)
		}
		if reed.removeCalls != 0 {
			t.Errorf("RemoveStrand called %d time(s); want 0 when StrandLive itself errored", reed.removeCalls)
		}
	})

	t.Run("a failed removal of a live strand propagates", func(t *testing.T) {
		wantErr := errors.New("remove failed")
		reed := &fakeReed{
			status:    reedengine.StatusResult{Strands: []reedengine.StrandStatus{{GUID: "target", Live: true}}},
			removeErr: wantErr,
		}
		err := removeStrandIfLive(reed, "target")
		if err == nil {
			t.Fatalf("removeStrandIfLive() error = nil; want a propagated error")
		}
		if !errors.Is(err, wantErr) {
			t.Errorf("removeStrandIfLive() error = %v; want it to wrap %v", err, wantErr)
		}
	})
}
