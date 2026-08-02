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

// ParseEvents parses events.jsonl into Events: Stop lines become EventStop, PreToolUse+AskUserQuestion lines become EventAsk.
// It is lenient: malformed or unrecognized lines are skipped, since the file may still be growing during the run.
func (c *Claude) ParseEvents(data []byte) ([]shuttleengine.Event, error) {
	var events []shuttleengine.Event

	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		var fields map[string]any
		if err := json.Unmarshal([]byte(trimmed), &fields); err != nil {
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
				Raw:     []byte(line),
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

// askQuestionText extracts the newline-joined question strings from tool_input.questions[].question.
// It returns "" for unexpected shapes, staying lenient.
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
