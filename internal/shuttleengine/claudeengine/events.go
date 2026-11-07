// events.go parses a run's events.jsonl into the shuttleengine events the run loop classifies outcomes from.
// It keeps the Stop and AskUserQuestion lines and skips every other line, the stamp lines included.
package claudeengine

import (
	"encoding/json"
	"strings"

	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// ParseEvents parses events.jsonl into Events.
// A Stop line becomes EventStop, or EventWaiting when background work is outstanding.
// A PreToolUse line for AskUserQuestion becomes EventAsk.
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
			kind := shuttleengine.EventStop
			outstanding := outstandingBackgroundTasks(fields)
			if len(outstanding) > 0 {
				kind = shuttleengine.EventWaiting
			}
			events = append(events, shuttleengine.Event{
				Kind:        kind,
				Message:     lastMessage,
				Raw:         []byte(line),
				Outstanding: outstanding,
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
