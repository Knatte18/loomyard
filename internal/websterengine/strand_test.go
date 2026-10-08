// strand_test.go covers StrandLive against a shuttlefake.Reed (present/live, present/not-live, absent) and TurnEnded against a shuttlefake.Engine (stop event, no stop event, missing events file, a ParseEvents error).
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
//testtiming:keep pins StrandLive's own live/not-live/absent results and wrapped probe error, which the recover-batch tests only observe as a classification
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

	t.Run("a skill-load turn end before the offset is not counted", func(t *testing.T) {
		loadTurn := "load-turn-stop\n"
		path := dir + "/offset-events.jsonl"
		if err := os.WriteFile(path, []byte(loadTurn+"prompt-turn-working\n"), 0o644); err != nil {
			t.Fatalf("write events file %s: %v", path, err)
		}
		engine := &shuttlefake.Engine{ParseEventsFn: func(data []byte) ([]shuttleengine.Event, error) {
			if strings.Contains(string(data), "load-turn-stop") {
				return []shuttleengine.Event{{Kind: shuttleengine.EventStop, Message: "skills loaded"}}, nil
			}
			return nil, nil
		}}
		if ended, err := TurnEndedAfter(path, 0, engine); err != nil || !ended {
			t.Fatalf("TurnEndedAfter(offset 0) = %v, %v; want true, nil (the load turn's Stop is in range)", ended, err)
		}
		if ended, err := TurnEndedAfter(path, int64(len(loadTurn)), engine); err != nil || ended {
			t.Errorf("TurnEndedAfter(past the load turn) = %v, %v; want false, nil", ended, err)
		}
	})
}
