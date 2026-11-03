// events.go reads a run's events file, where the launch's hooks append a stamp line and a payload line per hook event.
// ParseEvents yields the turn ends the run loop classifies outcomes from, and ParseSessionSignals the session signals a session state folds.
// All Codex payload-shape knowledge the events file needs lives in this file, per PATTERN-shuttle-provider-seam.

package codexengine

import (
	"bytes"
	"encoding/json"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

var _ shuttleengine.SessionSignalParser = (*Codex)(nil)

// ParseEvents parses the events file into Events: each Stop payload becomes an EventStop carrying its last_assistant_message.
// Codex fires no Stop for an interrupted turn, so an interrupt yields no event.
// It is lenient: stamp lines, malformed lines and other hooks yield nothing, since the file may still be growing.
func (c *Codex) ParseEvents(data []byte) ([]shuttleengine.Event, error) {
	var events []shuttleengine.Event
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		var payload struct {
			HookEventName        string `json:"hook_event_name"`
			LastAssistantMessage string `json:"last_assistant_message"`
		}
		if err := json.Unmarshal([]byte(trimmed), &payload); err != nil || payload.HookEventName != hookEventStop {
			continue
		}
		events = append(events, shuttleengine.Event{
			Kind:    shuttleengine.EventStop,
			Message: payload.LastAssistantMessage,
			Raw:     []byte(line),
		})
	}
	return events, nil
}

// pendingStamp is a stamp line not yet taken by a payload line.
type pendingStamp struct {
	hook string
	at   time.Time
}

// ParseSessionSignals parses events file bytes into session signals, in file order.
// SessionStart reads as a session start, UserPromptSubmit as a turn start, Stop as a turn end and PermissionRequest as an ask.
// A payload line takes its time from the nearest preceding untaken stamp naming its hook, and reads a zero time when there is none or the stamp's time is empty or malformed.
// consumed ends at the last newline-terminated line, except that it stops before the earliest line of the trailing run of stamp lines, which have no payload line after them yet.
// It is lenient: malformed lines, stamp lines and unknown hooks yield no signal.
func (c *Codex) ParseSessionSignals(data []byte) ([]shuttleengine.SessionSignal, int) {
	var signals []shuttleengine.SessionSignal
	var pending []pendingStamp
	holdFrom := -1
	offset := 0

	for offset < len(data) {
		newline := bytes.IndexByte(data[offset:], '\n')
		if newline < 0 {
			break
		}
		lineStart := offset
		line := data[offset : offset+newline]
		offset += newline + 1

		trimmed := bytes.TrimSpace(line)
		if len(trimmed) == 0 {
			continue
		}
		var fields map[string]any
		if err := json.Unmarshal(trimmed, &fields); err != nil {
			holdFrom = -1
			continue
		}
		if hook, ok := fields[stampHookKey].(string); ok {
			pending = append(pending, pendingStamp{hook: hook, at: parseStampTime(fields)})
			if holdFrom < 0 {
				holdFrom = lineStart
			}
			continue
		}
		holdFrom = -1

		hookName, ok := fields["hook_event_name"].(string)
		if !ok {
			continue
		}
		var at time.Time
		for i := len(pending) - 1; i >= 0; i-- {
			if pending[i].hook == hookName {
				at = pending[i].at
				pending = append(pending[:i], pending[i+1:]...)
				break
			}
		}
		if signal, ok := signalFromPayload(hookName, fields); ok {
			signal.At = at
			signal.SessionID, _ = fields["session_id"].(string)
			signal.Raw = append([]byte(nil), line...)
			signals = append(signals, signal)
		}
	}

	if holdFrom >= 0 {
		return signals, holdFrom
	}
	return signals, offset
}

// parseStampTime reads a stamp line's RFC 3339 time, or the zero time when it is empty or malformed.
func parseStampTime(fields map[string]any) time.Time {
	text, _ := fields[stampAtKey].(string)
	at, err := time.Parse(time.RFC3339, text)
	if err != nil {
		return time.Time{}
	}
	return at.UTC()
}

// signalFromPayload reads the signal a payload line of the named hook carries, or false when the hook yields none.
func signalFromPayload(hookName string, fields map[string]any) (shuttleengine.SessionSignal, bool) {
	switch hookName {
	case hookEventStop:
		return shuttleengine.SessionSignal{Kind: shuttleengine.SessionSignalTurnEnd, Event: true}, true
	case hookEventUserPromptSubmit:
		prompt, _ := fields["prompt"].(string)
		return shuttleengine.SessionSignal{Kind: shuttleengine.SessionSignalTurnStart, Instruction: typedInstruction(prompt)}, true
	case hookEventPermissionRequest:
		return shuttleengine.SessionSignal{Kind: shuttleengine.SessionSignalAsk}, true
	case hookEventSessionStart:
		source, _ := fields["source"].(string)
		return shuttleengine.SessionSignal{Kind: shuttleengine.SessionSignalSessionStart, Source: sessionStartSource(source)}, true
	}
	return shuttleengine.SessionSignal{}, false
}

// typedInstruction returns a UserPromptSubmit prompt as a typed instruction, or "" for a slash command, which is a command to Codex's own interface.
// A prompt is a slash command when its first word starts with "/" and holds no further "/", so one that opens with a multi-segment path is an instruction.
func typedInstruction(prompt string) string {
	trimmed := strings.TrimSpace(prompt)
	if words := strings.Fields(trimmed); len(words) > 0 && strings.HasPrefix(words[0], "/") && !strings.Contains(words[0][1:], "/") {
		return ""
	}
	return trimmed
}

// sessionStartSource maps a SessionStart payload's source to the provider-neutral source: startup and resume map to their own, and any other source, a fork's included, to the empty source.
func sessionStartSource(source string) shuttleengine.SessionStartSource {
	switch source {
	case "startup":
		return shuttleengine.SessionStartStartup
	case "resume":
		return shuttleengine.SessionStartResume
	}
	return ""
}
