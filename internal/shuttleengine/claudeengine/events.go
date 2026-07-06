// events.go implements ParseEvents, the lenient reader over a run's
// events.jsonl: the Stop hook (settings.go) appends one JSON line per turn
// end, and the live-ask marker hook (settings.go) appends one JSON line the
// instant an AskUserQuestion tool call opens; this file turns that raw byte
// stream into the shuttleengine.Events the run loop classifies outcomes
// from. All Claude payload-shape knowledge (hook_event_name, tool_name,
// tool_input, the literal AskUserQuestion tool name) lives only in this
// file, per the provider-seam containment decision.
package claudeengine

import (
	"encoding/json"
	"strings"

	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// ParseEvents parses data (a run's events.jsonl contents) into Events. It is
// deliberately lenient: a run's events file is read while the run may still
// be in progress, so a line can be truncated mid-append, and claude versions
// differ on which fields a payload carries — neither case is fatal. Blank
// lines are skipped. A line that fails to parse as JSON is skipped (a
// partial append in progress). A line whose hook_event_name is "Stop"
// becomes an EventStop, with Message set to its last_assistant_message field
// ("" when absent or not a string). A line whose hook_event_name is
// "PreToolUse" and whose tool_name is "AskUserQuestion" becomes an EventAsk,
// with Message set to every tool_input.questions[].question string
// newline-joined ("" when the shape is unexpected — stay lenient, do not
// error). Any other line — a different hook_event_name, a PreToolUse for a
// different tool, or no hook_event_name at all — is skipped, since it
// cannot be confirmed as either signal this reader surfaces.
func (c *Claude) ParseEvents(data []byte) ([]shuttleengine.Event, error) {
	var events []shuttleengine.Event

	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		var fields map[string]any
		if err := json.Unmarshal([]byte(trimmed), &fields); err != nil {
			// A malformed or partially-written line: skip it rather than
			// aborting the whole parse, since the file may still be growing.
			continue
		}

		eventName, ok := fields["hook_event_name"].(string)
		if !ok {
			continue
		}

		switch eventName {
		case "Stop":
			lastMessage, _ := fields["last_assistant_message"].(string)
			events = append(events, shuttleengine.Event{
				Kind:    shuttleengine.EventStop,
				Message: lastMessage,
				// Raw preserves the original line bytes (not the trimmed copy
				// used above for the blank check and JSON parse) so a
				// byte-exact round-trip is possible if a caller ever needs it.
				Raw: []byte(line),
			})
		case "PreToolUse":
			toolName, _ := fields["tool_name"].(string)
			if toolName != "AskUserQuestion" {
				continue
			}
			events = append(events, shuttleengine.Event{
				Kind:    shuttleengine.EventAsk,
				Message: askQuestionText(fields),
				Raw:     []byte(line),
			})
		}
	}

	return events, nil
}

// askQuestionText extracts the newline-joined question text from a
// PreToolUse(AskUserQuestion) payload's tool_input.questions[].question
// entries. It stays lenient with an unexpected shape (a missing
// tool_input/questions field, or a non-string question) — returning "" for
// what it cannot confirm rather than erroring, since a live-ask line is read
// mid-run just like a Stop line.
func askQuestionText(fields map[string]any) string {
	toolInput, ok := fields["tool_input"].(map[string]any)
	if !ok {
		return ""
	}
	questions, ok := toolInput["questions"].([]any)
	if !ok {
		return ""
	}

	var texts []string
	for _, q := range questions {
		questionFields, ok := q.(map[string]any)
		if !ok {
			continue
		}
		text, ok := questionFields["question"].(string)
		if !ok {
			continue
		}
		texts = append(texts, text)
	}
	return strings.Join(texts, "\n")
}
