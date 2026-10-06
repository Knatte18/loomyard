// strand_test.go covers StrandLive against a shuttlefake.Reed (present/live, present/not-live, absent), TurnEnded against a shuttlefake.Engine (stop event, no stop event, missing events file, a ParseEvents error), and removeStrandIfLive's three cases (live, not-live, a failed removal of a live strand).
// Tier 1: no git, only local fakes.

package websterengine

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/shuttlefake"
)

// TestStrandLive pins StrandLive's live, not-live, absent and probe-error results.
//
//testtiming:keep pins StrandLive's own live/not-live/absent results and wrapped probe error, which the removeStrandIfLive test only observes as whether a removal happened
func TestStrandLive(t *testing.T) {
	t.Parallel()

	t.Run("guid present and live", func(t *testing.T) {
		reed := &shuttlefake.Reed{Strands: []reedengine.StrandStatus{
			{GUID: "other", Live: false},
			{GUID: "target", Live: true},
		}}
		live, err := StrandLive(reed, "target")
		if err != nil {
			t.Fatalf("StrandLive() error = %v; want nil", err)
		}
		if !live {
			t.Errorf("StrandLive() = false; want true")
		}
	})

	t.Run("guid present and not live", func(t *testing.T) {
		reed := &shuttlefake.Reed{Strands: []reedengine.StrandStatus{{GUID: "target", Live: false}}}
		live, err := StrandLive(reed, "target")
		if err != nil {
			t.Fatalf("StrandLive() error = %v; want nil", err)
		}
		if live {
			t.Errorf("StrandLive() = true; want false")
		}
	})

	t.Run("guid absent from Status is false, nil", func(t *testing.T) {
		reed := &shuttlefake.Reed{Strands: []reedengine.StrandStatus{{GUID: "someone-else", Live: true}}}
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
		_, err := StrandLive(&shuttlefake.Reed{StatusErr: wantErr}, "target")
		if err == nil {
			t.Fatalf("StrandLive() error = nil; want a wrapped error")
		}
		if !errors.Is(err, wantErr) {
			t.Errorf("StrandLive() error = %v; want it to wrap %v", err, wantErr)
		}
	})
}

func TestTurnEnded(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	eventsPath := dir + "/events.jsonl"

	t.Run("missing events file is false, nil", func(t *testing.T) {
		ended, err := TurnEnded(eventsPath, &shuttlefake.Engine{})
		if err != nil {
			t.Fatalf("TurnEnded() error = %v; want nil", err)
		}
		if ended {
			t.Errorf("TurnEnded() = true for a missing events file; want false")
		}
	})

	if err := os.WriteFile(eventsPath, []byte("irrelevant bytes; the engine ignores them"), 0o644); err != nil {
		t.Fatalf("write events file %s: %v", eventsPath, err)
	}

	t.Run("no Stop event is false", func(t *testing.T) {
		ended, err := TurnEnded(eventsPath, &shuttlefake.Engine{Events: []shuttleengine.Event{{Kind: shuttleengine.EventAsk, Message: "still working"}}})
		if err != nil {
			t.Fatalf("TurnEnded() error = %v; want nil", err)
		}
		if ended {
			t.Errorf("TurnEnded() = true with only an EventAsk; want false")
		}
	})

	t.Run("a Stop event anywhere in the batch is true", func(t *testing.T) {
		ended, err := TurnEnded(eventsPath, &shuttlefake.Engine{Events: []shuttleengine.Event{
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
		_, err := TurnEnded(eventsPath, &shuttlefake.Engine{EventsErr: wantErr})
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
		reed := &shuttlefake.Reed{Strands: []reedengine.StrandStatus{{GUID: "target", Live: true}}}
		if err := removeStrandIfLive(reed, "target"); err != nil {
			t.Fatalf("removeStrandIfLive() error = %v; want nil", err)
		}
		if len(reed.RemovedGUIDs) != 1 || reed.RemovedGUIDs[0] != "target" {
			t.Errorf("RemovedGUIDs = %v; want [target]", reed.RemovedGUIDs)
		}
	})

	t.Run("not-live strand is a no-op", func(t *testing.T) {
		reed := &shuttlefake.Reed{Strands: []reedengine.StrandStatus{{GUID: "target", Live: false}}}
		if err := removeStrandIfLive(reed, "target"); err != nil {
			t.Fatalf("removeStrandIfLive() error = %v; want nil", err)
		}
		if len(reed.RemovedGUIDs) != 0 {
			t.Errorf("RemovedGUIDs = %v; want none for a not-live strand", reed.RemovedGUIDs)
		}
	})

	t.Run("absent guid (StrandLive false) is a no-op", func(t *testing.T) {
		reed := &shuttlefake.Reed{}
		if err := removeStrandIfLive(reed, "target"); err != nil {
			t.Fatalf("removeStrandIfLive() error = %v; want nil", err)
		}
		if len(reed.RemovedGUIDs) != 0 {
			t.Errorf("RemovedGUIDs = %v; want none for an absent strand", reed.RemovedGUIDs)
		}
	})

	// This case previously pinned the opposite behaviour — a failed probe swallowed as not-live.
	// The round-4 review (R4-37) overturned it: a probe that could not answer has not established
	// that the strand is dead, so swallowing it left a leftover agent running while its replacement
	// was spawned beside it, which is the exact double-agent this reclaim exists to prevent. A run
	// that refuses to start over an unanswerable reed probe is diagnosable; two Masters committing
	// to one branch is not.
	t.Run("a StrandLive error fails the reclaim rather than guessing the strand is dead", func(t *testing.T) {
		reed := &shuttlefake.Reed{StatusErr: errors.New("reed unreachable")}
		err := removeStrandIfLive(reed, "target")
		if err == nil {
			t.Fatal("removeStrandIfLive() error = nil; want the probe failure surfaced")
		}
		if !strings.Contains(err.Error(), "reed unreachable") {
			t.Errorf("removeStrandIfLive() error = %v; want it to carry the underlying probe failure", err)
		}
		if len(reed.RemovedGUIDs) != 0 {
			t.Errorf("RemovedGUIDs = %v; want none when StrandLive itself errored", reed.RemovedGUIDs)
		}
	})

	t.Run("a failed removal of a live strand propagates", func(t *testing.T) {
		wantErr := errors.New("remove failed")
		reed := &shuttlefake.Reed{
			Strands:   []reedengine.StrandStatus{{GUID: "target", Live: true}},
			RemoveErr: wantErr,
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
