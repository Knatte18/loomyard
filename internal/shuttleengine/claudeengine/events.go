// events.go implements ParseEvents, the lenient reader over a run's
// events.jsonl: the Stop hook (settings.go) appends one JSON line per turn
// end, and this file turns that raw byte stream into the StopEvents the run
// loop classifies outcomes from.

package claudeengine

import (
	"encoding/json"
	"strings"

	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// ParseEvents parses data (a run's events.jsonl contents) into StopEvents.
// It is deliberately lenient: a run's events file is read while the run may
// still be in progress, so a line can be truncated mid-append, and claude
// versions differ on which fields a Stop payload carries — neither case is
// fatal. Blank lines are skipped. A line that fails to parse as JSON is
// skipped (a partial append in progress). A line whose hook_event_name is
// present and not "Stop" is skipped (this reader only surfaces turn-end
// events); a line with no hook_event_name at all is also skipped, since it
// cannot be confirmed as a Stop event. A matched line's
// last_assistant_message field becomes LastAssistantMessage ("" when
// absent or not a string), and Raw carries the exact line bytes it was
// parsed from.
func (c *Claude) ParseEvents(data []byte) ([]shuttleengine.StopEvent, error) {
	var events []shuttleengine.StopEvent

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
		if !ok || eventName != "Stop" {
			continue
		}

		lastMessage, _ := fields["last_assistant_message"].(string)
		events = append(events, shuttleengine.StopEvent{
			LastAssistantMessage: lastMessage,
			// Raw preserves the original line bytes (not the trimmed copy used
			// above for the blank check and JSON parse) so a byte-exact
			// round-trip is possible if a caller ever needs it.
			Raw: []byte(line),
		})
	}

	return events, nil
}
