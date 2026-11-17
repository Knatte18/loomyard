// rollout.go reads the rollout that records a Codex session's turns, which every hook payload names in transcript_path.
// It answers whether a turn was interrupted, which ends a turn with no Stop hook.
// It answers how many tokens the session's context holds.

package codexengine

import (
	"encoding/json"
	"os"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

var _ shuttleengine.SessionProber = (*Codex)(nil)

// Rollout event types this file reads.
const (
	rolloutEventMessage   = "event_msg"
	eventTaskComplete     = "task_complete"
	eventTurnAborted      = "turn_aborted"
	eventTokenCount       = "token_count"
	payloadTranscriptPath = "transcript_path"
)

// rolloutEvent is one event_msg line of a rollout.
type rolloutEvent struct {
	at      time.Time
	kind    string
	turnID  string
	context int
	// hasContext is true for a token_count event that carries the latest model call's usage.
	hasContext bool
	total      rolloutTotal
	// hasTotal is true for a token_count event that carries the session's running total usage.
	hasTotal bool
}

// rolloutTotal is the total_token_usage of a token_count event: the session's running token counts.
// CacheWrite is zero when the event carries no cache_write_input_tokens.
type rolloutTotal struct {
	Input      int64 `json:"input_tokens"`
	CachedIn   int64 `json:"cached_input_tokens"`
	Output     int64 `json:"output_tokens"`
	CacheWrite int64 `json:"cache_write_input_tokens"`
}

// readRolloutEvents reads the event_msg lines of the rollout at path, in file order, skipping lines it cannot parse.
func readRolloutEvents(path string) ([]rolloutEvent, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var events []rolloutEvent
	for _, line := range strings.Split(string(data), "\n") {
		var parsed struct {
			Timestamp string `json:"timestamp"`
			Type      string `json:"type"`
			Payload   struct {
				Type   string `json:"type"`
				TurnID string `json:"turn_id"`
				Info   *struct {
					TotalTokenUsage *rolloutTotal `json:"total_token_usage"`
					LastTokenUsage  struct {
						TotalTokens int `json:"total_tokens"`
					} `json:"last_token_usage"`
				} `json:"info"`
			} `json:"payload"`
		}
		if json.Unmarshal([]byte(strings.TrimSpace(line)), &parsed) != nil || parsed.Type != rolloutEventMessage {
			continue
		}
		event := rolloutEvent{kind: parsed.Payload.Type, turnID: parsed.Payload.TurnID}
		if at, err := time.Parse(time.RFC3339Nano, parsed.Timestamp); err == nil {
			event.at = at.UTC()
		}
		if parsed.Payload.Info != nil {
			event.context = parsed.Payload.Info.LastTokenUsage.TotalTokens
			event.hasContext = true
			if parsed.Payload.Info.TotalTokenUsage != nil {
				event.total = *parsed.Payload.Info.TotalTokenUsage
				event.hasTotal = true
			}
		}
		events = append(events, event)
	}
	return events, nil
}

// payloadField returns the string at key in the hook payload raw, or "" when raw does not parse or key is not a string.
func payloadField(raw []byte, key string) string {
	var fields map[string]any
	if json.Unmarshal(raw, &fields) != nil {
		return ""
	}
	value, _ := fields[key].(string)
	return value
}

// ProcessLiveness reads every session unproven: Codex keeps no registry of its live sessions,
// so a Codex exit is caught by the exit record and the dead pane the launch line leaves.
func (c *Codex) ProcessLiveness(string) shuttleengine.Liveness {
	return shuttleengine.LivenessUnproven
}

// TurnStartInterrupt reports whether the turn that turnStart began was interrupted, with the interrupt's own time.
// It reads the rollout that turnStart's payload names:
// the turn is interrupted when the newest of its turn-ending events is turn_aborted.
// An event belongs to the turn when both carry a turn id and the ids are equal.
// When either lacks a turn id, an event belongs to the turn when it is not older than turnStart.
// Any failure to read reads as not interrupted.
func (c *Codex) TurnStartInterrupt(turnStart shuttleengine.SessionSignal) (time.Time, bool) {
	events, err := readRolloutEvents(payloadField(turnStart.Raw, payloadTranscriptPath))
	if err != nil {
		return time.Time{}, false
	}
	turnID := payloadField(turnStart.Raw, "turn_id")
	var last *rolloutEvent
	for i := range events {
		event := &events[i]
		if event.kind != eventTaskComplete && event.kind != eventTurnAborted {
			continue
		}
		if turnID != "" && event.turnID != "" {
			if event.turnID != turnID {
				continue
			}
		} else if !turnStart.At.IsZero() && !event.at.IsZero() && event.at.Before(turnStart.At) {
			continue
		}
		last = event
	}
	if last == nil || last.kind != eventTurnAborted {
		return time.Time{}, false
	}
	return last.at, true
}

// ContextTokens returns the context size the newest token_count event of the rollout turnEnd's payload names records:
// the latest model call's total tokens.
// A rollout that cannot be read or holds no such event reads unknown.
func (c *Codex) ContextTokens(turnEnd shuttleengine.Event) shuttleengine.ContextReading {
	events, err := readRolloutEvents(payloadField(turnEnd.Raw, payloadTranscriptPath))
	if err != nil {
		return shuttleengine.ContextReading{}
	}
	for i := len(events) - 1; i >= 0; i-- {
		if events[i].kind == eventTokenCount && events[i].hasContext {
			return shuttleengine.ContextReading{Tokens: events[i].context, Known: true}
		}
	}
	return shuttleengine.ContextReading{}
}

// CompactedSince finds no compaction boundary: this package reads no Codex compaction from a rollout.
func (c *Codex) CompactedSince(shuttleengine.Event, time.Time) (shuttleengine.CompactionBoundary, bool) {
	return shuttleengine.CompactionBoundary{}, false
}
