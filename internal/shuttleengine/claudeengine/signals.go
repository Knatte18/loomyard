// signals.go implements ParseSessionSignals, the lenient reader over a run's events.jsonl that turns Claude's hook lines into provider-neutral session signals.
// Each recording hook writes a time-stamp line before its payload line; the parser pairs the two so a signal carries the hook-side time.
// All Claude payload-shape knowledge the signals need (hook names, notification types, session-end reasons, the stamp format) lives only in this file and events.go, per PATTERN-shuttle-provider-seam.

package claudeengine

import (
	"bytes"
	"encoding/json"
	"time"

	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// Stamp line keys: a stamp line is {"lyx_stamp":"<hook event name>","lyx_at":"<RFC 3339 UTC>"}, written by a recording hook before its payload.
const (
	stampHookKey = "lyx_stamp"
	stampAtKey   = "lyx_at"
)

// Hook event names a payload line carries in hook_event_name.
const (
	hookEventStop             = "Stop"
	hookEventStopFailure      = "StopFailure"
	hookEventUserPromptSubmit = "UserPromptSubmit"
	hookEventPreToolUse       = "PreToolUse"
	hookEventNotification     = "Notification"
	hookEventSessionEnd       = "SessionEnd"
)

// askUserQuestionToolName is the tool whose PreToolUse payload is an ask.
const askUserQuestionToolName = "AskUserQuestion"

// Notification types the parser reads: the two that wait on an answer, and the idle notice.
const (
	notificationPermissionPrompt  = "permission_prompt"
	notificationElicitationDialog = "elicitation_dialog"
	notificationIdlePrompt        = "idle_prompt"
)

// SessionEnd reasons that end Claude's process.
const (
	sessionEndReasonLogout         = "logout"
	sessionEndReasonPromptInputEnd = "prompt_input_exit"
	sessionEndReasonOther          = "other"
)

var _ shuttleengine.SessionSignalParser = (*Claude)(nil)

// pendingStamp is a stamp line not yet taken by a payload line.
type pendingStamp struct {
	hook string
	at   time.Time
}

// ParseSessionSignals parses events.jsonl bytes into session signals, in file order.
// A payload line takes its time from the nearest preceding untaken stamp naming its hook, and reads a zero time when there is none or the stamp's time is empty or malformed.
// consumed ends at the last newline-terminated line, except that it stops before the earliest line of the trailing run of stamp lines, which have no payload line after them yet.
// It is lenient: malformed lines, stamp lines and unknown hooks yield no signal.
func (c *Claude) ParseSessionSignals(data []byte) ([]shuttleengine.SessionSignal, int) {
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
		return shuttleengine.SessionSignal{
			Kind:        shuttleengine.SessionSignalTurnEnd,
			Outstanding: outstandingBackgroundTasks(fields),
			Event:       true,
		}, true
	case hookEventStopFailure:
		return shuttleengine.SessionSignal{
			Kind:      shuttleengine.SessionSignalAPIErrorTurnEnd,
			ErrorText: apiErrorText(fields),
		}, true
	case hookEventUserPromptSubmit:
		return shuttleengine.SessionSignal{Kind: shuttleengine.SessionSignalTurnStart}, true
	case hookEventPreToolUse:
		if toolName, _ := fields["tool_name"].(string); toolName != askUserQuestionToolName {
			return shuttleengine.SessionSignal{}, false
		}
		return shuttleengine.SessionSignal{Kind: shuttleengine.SessionSignalAsk, Event: true}, true
	case hookEventNotification:
		notificationType, _ := fields["notification_type"].(string)
		switch notificationType {
		case notificationPermissionPrompt, notificationElicitationDialog:
			return shuttleengine.SessionSignal{Kind: shuttleengine.SessionSignalAsk}, true
		case notificationIdlePrompt:
			return shuttleengine.SessionSignal{Kind: shuttleengine.SessionSignalIdleNotice}, true
		}
		return shuttleengine.SessionSignal{}, false
	case hookEventSessionEnd:
		reason, _ := fields["reason"].(string)
		return shuttleengine.SessionSignal{
			Kind:        shuttleengine.SessionSignalSessionEnd,
			Reason:      reason,
			EndsProcess: sessionEndReasonEndsProcess(reason),
		}, true
	}
	return shuttleengine.SessionSignal{}, false
}

// apiErrorText returns a StopFailure payload's text from error, else error_details, else last_assistant_message.
func apiErrorText(fields map[string]any) string {
	for _, key := range []string{"error", "error_details", "last_assistant_message"} {
		if text, _ := fields[key].(string); text != "" {
			return text
		}
	}
	return ""
}

// sessionEndReasonEndsProcess reports whether a SessionEnd reason ends Claude's process; clear, resume and any unknown reason leave it running.
func sessionEndReasonEndsProcess(reason string) bool {
	switch reason {
	case sessionEndReasonLogout, sessionEndReasonPromptInputEnd, sessionEndReasonOther:
		return true
	}
	return false
}
