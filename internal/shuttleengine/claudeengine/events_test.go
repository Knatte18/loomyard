// events_test.go tests ParseEvents on a fixture JSONL of Stop, non-Stop, garbage, and blank lines.
// It pins the lenient skip of unrecognized lines, that Raw keeps each event's exact line, and the live-ask signal.

package claudeengine

import (
	"bytes"
	"testing"

	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestParseEvents_LenientFixture(t *testing.T) {
	tiergate.Require(t, 0)
	c := New()
	events, err := c.ParseEvents([]byte(fixtureJSONL))
	if err != nil {
		t.Fatalf("ParseEvents() error: %v", err)
	}

	if len(events) != 2 {
		t.Fatalf("ParseEvents() returned %d events; want 2 (fixture events: %+v)", len(events), events)
	}

	if events[0].Kind != shuttleengine.EventStop {
		t.Errorf("events[0].Kind = %v; want EventStop", events[0].Kind)
	}
	if events[0].Message != "first message" {
		t.Errorf("events[0].Message = %q; want %q", events[0].Message, "first message")
	}
	wantRaw0 := `{"hook_event_name":"Stop","last_assistant_message":"first message","session_id":"s1"}`
	if !bytes.Equal(events[0].Raw, []byte(wantRaw0)) {
		t.Errorf("events[0].Raw = %q; want %q", events[0].Raw, wantRaw0)
	}

	if events[1].Kind != shuttleengine.EventStop {
		t.Errorf("events[1].Kind = %v; want EventStop", events[1].Kind)
	}
	if events[1].Message != "second message" {
		t.Errorf("events[1].Message = %q; want %q", events[1].Message, "second message")
	}
	wantRaw1 := `{"hook_event_name":"Stop","last_assistant_message":"second message","session_id":"s1"}`
	if !bytes.Equal(events[1].Raw, []byte(wantRaw1)) {
		t.Errorf("events[1].Raw = %q; want %q", events[1].Raw, wantRaw1)
	}
}

// TestParseEvents_LiveAsk verifies a PreToolUse+AskUserQuestion line becomes an EventAsk.
// Its Message joins every tool_input.questions[].question string with a newline between them.
func TestParseEvents_LiveAsk(t *testing.T) {
	tiergate.Require(t, 0)
	c := New()
	const line = `{"hook_event_name":"PreToolUse","tool_name":"AskUserQuestion","tool_input":{"questions":[{"question":"Which approach?"},{"question":"Proceed now?"}]}}`
	events, err := c.ParseEvents([]byte(line))
	if err != nil {
		t.Fatalf("ParseEvents() error: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("ParseEvents() returned %d events; want 1", len(events))
	}
	if events[0].Kind != shuttleengine.EventAsk {
		t.Errorf("events[0].Kind = %v; want EventAsk", events[0].Kind)
	}
	wantMessage := "Which approach?\nProceed now?"
	if events[0].Message != wantMessage {
		t.Errorf("events[0].Message = %q; want %q", events[0].Message, wantMessage)
	}
	if !bytes.Equal(events[0].Raw, []byte(line)) {
		t.Errorf("events[0].Raw = %q; want %q", events[0].Raw, line)
	}
}

// TestParseEvents_LiveAsk_UnexpectedShape verifies a PreToolUse+AskUserQuestion line with an unexpected tool_input.
// It still classifies as EventAsk, since the hook_event_name and tool_name pair matches.
// Its Message is empty, and ParseEvents returns no error.
func TestParseEvents_LiveAsk_UnexpectedShape(t *testing.T) {
	tiergate.Require(t, 0)
	c := New()
	const line = `{"hook_event_name":"PreToolUse","tool_name":"AskUserQuestion","tool_input":{"unexpected":true}}`
	events, err := c.ParseEvents([]byte(line))
	if err != nil {
		t.Fatalf("ParseEvents() error: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("ParseEvents() returned %d events; want 1", len(events))
	}
	if events[0].Kind != shuttleengine.EventAsk {
		t.Errorf("events[0].Kind = %v; want EventAsk", events[0].Kind)
	}
	if events[0].Message != "" {
		t.Errorf("events[0].Message = %q; want empty (unexpected tool_input shape)", events[0].Message)
	}
}

// TestParseEvents_PreToolUse_OtherToolSkipped verifies a PreToolUse line for a tool other than AskUserQuestion.
// ParseEvents skips such a line the way it skips any other unrecognized line.
func TestParseEvents_PreToolUse_OtherToolSkipped(t *testing.T) {
	tiergate.Require(t, 0)
	c := New()
	const line = `{"hook_event_name":"PreToolUse","tool_name":"Agent","tool_input":{}}`
	events, err := c.ParseEvents([]byte(line))
	if err != nil {
		t.Fatalf("ParseEvents() error: %v", err)
	}
	if len(events) != 0 {
		t.Errorf("ParseEvents() returned %d events; want 0 (non-AskUserQuestion PreToolUse skipped)", len(events))
	}
}

// TestParseEvents_RawPreservesSurroundingWhitespace verifies that Raw carries the exact line bytes:
// incidental leading and trailing whitespace stays in Raw, while ParseEvents trims a copy.
// The trimmed copy serves the blank-line check and the JSON parse.
func TestParseEvents_RawPreservesSurroundingWhitespace(t *testing.T) {
	tiergate.Require(t, 0)
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
	tiergate.Require(t, 0)
	c := New()
	events, err := c.ParseEvents([]byte(""))
	if err != nil {
		t.Fatalf("ParseEvents() error: %v", err)
	}
	if len(events) != 0 {
		t.Errorf("ParseEvents(\"\") = %v; want empty", events)
	}
}

// fixtureJSONL mirrors a Stop-hook events.jsonl that holds two valid Stop events.
// A garbage line simulates a partial or truncated append;
// a non-Stop event and a blank line sit between the two Stop events.
const fixtureJSONL = `{"hook_event_name":"Stop","last_assistant_message":"first message","session_id":"s1"}
{this is not valid json
{"hook_event_name":"SessionStart","session_id":"s1"}

{"hook_event_name":"Stop","last_assistant_message":"second message","session_id":"s1"}
`
