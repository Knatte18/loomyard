// events_test.go exercises ParseEvents against a fixture JSONL containing
// two Stop events, an interleaved garbage line, a non-Stop event, and a
// blank line — asserting the lenient skip behaviour and that Raw round
// trips the exact line it was parsed from.

package claudeengine

import (
	"bytes"
	"testing"
)

func TestParseEvents_LenientFixture(t *testing.T) {
	c := New()
	events, err := c.ParseEvents([]byte(fixtureJSONL))
	if err != nil {
		t.Fatalf("ParseEvents() error: %v", err)
	}

	if len(events) != 2 {
		t.Fatalf("ParseEvents() returned %d events; want 2 (fixture events: %+v)", len(events), events)
	}

	if events[0].LastAssistantMessage != "first message" {
		t.Errorf("events[0].LastAssistantMessage = %q; want %q", events[0].LastAssistantMessage, "first message")
	}
	wantRaw0 := `{"hook_event_name":"Stop","last_assistant_message":"first message","session_id":"s1"}`
	if !bytes.Equal(events[0].Raw, []byte(wantRaw0)) {
		t.Errorf("events[0].Raw = %q; want %q", events[0].Raw, wantRaw0)
	}

	if events[1].LastAssistantMessage != "second message" {
		t.Errorf("events[1].LastAssistantMessage = %q; want %q", events[1].LastAssistantMessage, "second message")
	}
	wantRaw1 := `{"hook_event_name":"Stop","last_assistant_message":"second message","session_id":"s1"}`
	if !bytes.Equal(events[1].Raw, []byte(wantRaw1)) {
		t.Errorf("events[1].Raw = %q; want %q", events[1].Raw, wantRaw1)
	}
}

// TestParseEvents_RawPreservesSurroundingWhitespace verifies that Raw carries
// the exact original line bytes -- including incidental leading/trailing
// whitespace -- rather than the trimmed copy used internally for the
// blank-line check and JSON parse.
func TestParseEvents_RawPreservesSurroundingWhitespace(t *testing.T) {
	c := New()
	const line = `  {"hook_event_name":"Stop","last_assistant_message":"padded"}  `
	events, err := c.ParseEvents([]byte(line))
	if err != nil {
		t.Fatalf("ParseEvents() error: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("ParseEvents() returned %d events; want 1", len(events))
	}
	if !bytes.Equal(events[0].Raw, []byte(line)) {
		t.Errorf("events[0].Raw = %q; want the untrimmed original line %q", events[0].Raw, line)
	}
}

func TestParseEvents_EmptyInput(t *testing.T) {
	c := New()
	events, err := c.ParseEvents([]byte(""))
	if err != nil {
		t.Fatalf("ParseEvents() error: %v", err)
	}
	if len(events) != 0 {
		t.Errorf("ParseEvents(\"\") = %v; want empty", events)
	}
}

// fixtureJSONL mirrors a Stop-hook events.jsonl containing: a valid Stop
// event, a garbage (non-JSON) line simulating a partial/truncated append, a
// non-Stop event, a blank line, and a second valid Stop event.
const fixtureJSONL = `{"hook_event_name":"Stop","last_assistant_message":"first message","session_id":"s1"}
{this is not valid json
{"hook_event_name":"SessionStart","session_id":"s1"}

{"hook_event_name":"Stop","last_assistant_message":"second message","session_id":"s1"}
`
