// strand_test.go covers StrandLive, TurnEndedAfter and LastTurnMessage, each against its shuttlefake fake.
// It pins StrandLive's live, not-live and absent results, and the results of the other two over an events file.

package gatefixer

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/shuttlefake"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestStrandLive pins StrandLive's live, not-live, absent and probe-error results.
func TestStrandLive(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	t.Run("guid present and live", func(t *testing.T) {
		t.Parallel()
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
		t.Parallel()
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
		t.Parallel()
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
		t.Parallel()
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

// TestLastTurnMessage pins LastTurnMessage's trimmed message of the newest stop after the offset.
// An offset past the file's end parses no bytes.
// A missing file, no stop and a message-less newest stop all give "".
func TestLastTurnMessage(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	const earlier = "earlier-stop\n"
	tests := []struct {
		name string
		// content is the events file's bytes; nil leaves the file missing.
		content []byte
		offset  int64
		events  func(data []byte) ([]shuttleengine.Event, error)
		want    string
	}{
		{
			name:    "the newest stop's message is trimmed",
			content: []byte("bytes"),
			events: func([]byte) ([]shuttleengine.Event, error) {
				return []shuttleengine.Event{
					{Kind: shuttleengine.EventStop, Message: "older message"},
					{Kind: shuttleengine.EventAsk, Message: "a probe"},
					{Kind: shuttleengine.EventStop, Message: "  newest message \n"},
				}, nil
			},
			want: "newest message",
		},
		{
			name:    "only the bytes from the offset on are parsed",
			content: []byte(earlier + "later-stop\n"),
			offset:  int64(len(earlier)),
			events: func(data []byte) ([]shuttleengine.Event, error) {
				if strings.Contains(string(data), "earlier-stop") {
					return []shuttleengine.Event{{Kind: shuttleengine.EventStop, Message: "from the earlier turn"}}, nil
				}
				if strings.Contains(string(data), "later-stop") {
					return []shuttleengine.Event{{Kind: shuttleengine.EventStop, Message: "later"}}, nil
				}
				return nil, nil
			},
			want: "later",
		},
		{
			name:    "an offset past the file's end parses no bytes",
			content: []byte("bytes"),
			offset:  100,
			events: func(data []byte) ([]shuttleengine.Event, error) {
				if len(data) == 0 {
					return []shuttleengine.Event{{Kind: shuttleengine.EventStop, Message: "from no bytes"}}, nil
				}
				return nil, nil
			},
			want: "from no bytes",
		},
		{
			name:    "no stop event gives an empty message",
			content: []byte("bytes"),
			events: func([]byte) ([]shuttleengine.Event, error) {
				return []shuttleengine.Event{{Kind: shuttleengine.EventAsk, Message: "still working"}}, nil
			},
			want: "",
		},
		{
			name:    "a newest stop with no message gives an empty message",
			content: []byte("bytes"),
			events: func([]byte) ([]shuttleengine.Event, error) {
				return []shuttleengine.Event{
					{Kind: shuttleengine.EventStop, Message: "older message"},
					{Kind: shuttleengine.EventStop, Message: " \n"},
				}, nil
			},
			want: "",
		},
		{
			name:   "a missing events file gives an empty message",
			events: func([]byte) ([]shuttleengine.Event, error) { return nil, errors.New("must not parse") },
			want:   "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), "events.jsonl")
			if tt.content != nil {
				if err := os.WriteFile(path, tt.content, 0o644); err != nil {
					t.Fatalf("write events file %s: %v", path, err)
				}
			}
			got, err := LastTurnMessage(path, tt.offset, &shuttlefake.Engine{ParseEventsFn: tt.events})
			if err != nil {
				t.Fatalf("LastTurnMessage() error = %v; want nil", err)
			}
			if got != tt.want {
				t.Errorf("LastTurnMessage() = %q; want %q", got, tt.want)
			}
		})
	}
}

// TestTurnEndedAfter pins TurnEndedAfter over an engine without the SessionSignalParser capability:
// the newest ParseEvents event decides, and the offset skips the load turns.
func TestTurnEndedAfter(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	dir := t.TempDir()
	eventsPath := filepath.Join(dir, "events.jsonl")
	turnEnded := func(path string, engine shuttleengine.Engine) (bool, error) {
		return TurnEndedAfter(path, 0, engine, TurnEndRead{})
	}

	t.Run("missing events file is false, nil", func(t *testing.T) {
		t.Parallel()
		ended, err := turnEnded(filepath.Join(dir, "absent.jsonl"), &shuttlefake.Engine{})
		if err != nil {
			t.Fatalf("TurnEndedAfter() error = %v; want nil", err)
		}
		if ended {
			t.Errorf("TurnEndedAfter() = true for a missing events file; want false")
		}
	})

	if err := os.WriteFile(eventsPath, []byte("irrelevant bytes; the engine ignores them"), 0o644); err != nil {
		t.Fatalf("write events file %s: %v", eventsPath, err)
	}

	t.Run("no Stop event is false", func(t *testing.T) {
		t.Parallel()
		ended, err := turnEnded(eventsPath, &shuttlefake.Engine{Events: []shuttleengine.Event{{Kind: shuttleengine.EventAsk, Message: "still working"}}})
		if err != nil {
			t.Fatalf("TurnEndedAfter() error = %v; want nil", err)
		}
		if ended {
			t.Errorf("TurnEndedAfter() = true with only an EventAsk; want false")
		}
	})

	t.Run("a newest Stop event is true", func(t *testing.T) {
		t.Parallel()
		ended, err := turnEnded(eventsPath, &shuttlefake.Engine{Events: []shuttleengine.Event{
			{Kind: shuttleengine.EventAsk, Message: "mid-turn probe"},
			{Kind: shuttleengine.EventStop, Message: "final message"},
		}})
		if err != nil {
			t.Fatalf("TurnEndedAfter() error = %v; want nil", err)
		}
		if !ended {
			t.Errorf("TurnEndedAfter() = false with a Stop event present; want true")
		}
	})

	t.Run("a ParseEvents error propagates", func(t *testing.T) {
		t.Parallel()
		wantErr := errors.New("boom")
		_, err := turnEnded(eventsPath, &shuttlefake.Engine{EventsErr: wantErr})
		if err == nil {
			t.Fatalf("TurnEndedAfter() error = nil; want a wrapped error")
		}
		if !errors.Is(err, wantErr) {
			t.Errorf("TurnEndedAfter() error = %v; want it to wrap %v", err, wantErr)
		}
	})

	t.Run("a skill-load turn end before the offset is not counted", func(t *testing.T) {
		t.Parallel()
		loadTurn := "load-turn-stop\n"
		path := filepath.Join(dir, "offset-events.jsonl")
		if err := os.WriteFile(path, []byte(loadTurn+"prompt-turn-working\n"), 0o644); err != nil {
			t.Fatalf("write events file %s: %v", path, err)
		}
		engine := &shuttlefake.Engine{ParseEventsFn: func(data []byte) ([]shuttleengine.Event, error) {
			if strings.Contains(string(data), "load-turn-stop") {
				return []shuttleengine.Event{{Kind: shuttleengine.EventStop, Message: "skills loaded"}}, nil
			}
			return nil, nil
		}}
		if ended, err := TurnEndedAfter(path, 0, engine, TurnEndRead{}); err != nil || !ended {
			t.Fatalf("TurnEndedAfter(offset 0) = %v, %v; want true, nil (the load turn's Stop is in range)", ended, err)
		}
		if ended, err := TurnEndedAfter(path, int64(len(loadTurn)), engine, TurnEndRead{}); err != nil || ended {
			t.Errorf("TurnEndedAfter(past the load turn) = %v, %v; want false, nil", ended, err)
		}
	})
}
