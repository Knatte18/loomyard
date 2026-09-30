// background.go decides whether a Stop payload describes a turn that ended with background work
// still outstanding, so ParseEvents can surface it as EventWaiting instead of EventStop.
// Two signals, either of which suffices:
// a background_tasks[] entry whose status is "running", or, when that finds nothing running,
// a background launch in the transcript at transcript_path that no later <task-notification>
// user message names by the task id the launch's tool result returned.
// The research on Stop payloads only observed type "subagent" entries in background_tasks[], so the
// transcript signal carries the Monitor and backgrounded-Bash cases.
// An unreadable or missing transcript counts as nothing outstanding: the transcript signal must
// never keep a run waiting on a fault.
// All Claude payload-shape knowledge stays in this package, per the provider-seam containment decision.
package claudeengine

import (
	"encoding/json"
	"os"
	"regexp"
	"strings"
)

// taskNotificationMarker opens the user message Claude Code injects when a background task finishes.
const taskNotificationMarker = "<task-notification>"

// resultIDKeys are the toolUseResult fields that carry a launched task's id.
var resultIDKeys = []string{"agentId", "backgroundTaskId", "bash_id", "shellId", "taskId", "task_id"}

// resultTextID extracts a task id from a tool result's text ("... with ID: bx12ab34", "task abc123").
var resultTextID = regexp.MustCompile(`(?i)\b(?:id|task)[:\s]+([A-Za-z0-9_-]{6,})`)

// hasOutstandingBackgroundWork reports whether the Stop payload fields describe a turn end with
// background work still running.
func hasOutstandingBackgroundWork(fields map[string]any) bool {
	if tasks, ok := fields["background_tasks"].([]any); ok {
		for _, t := range tasks {
			task, ok := t.(map[string]any)
			if !ok {
				continue
			}
			if status, _ := task["status"].(string); status == "running" {
				return true
			}
		}
	}

	path, _ := fields["transcript_path"].(string)
	if path == "" {
		return false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	return transcriptHasUnmatchedLaunch(data)
}

// transcriptLaunch is one background launch found in the transcript.
type transcriptLaunch struct {
	toolUseID string
	ids       []string // Task ids the launch's tool result returned.
	resultAt  int      // Line index of the tool result; notifications must come after it.
}

// transcriptHasUnmatchedLaunch scans transcript JSONL for background launches whose returned task
// id is never named by a later task-notification user message.
// A launch whose result carries no recoverable id cannot be matched and is not counted.
func transcriptHasUnmatchedLaunch(data []byte) bool {
	lines := strings.Split(string(data), "\n")

	var launches []*transcriptLaunch
	byToolUse := map[string]*transcriptLaunch{}
	var notifications []struct {
		at   int
		text string
	}

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		var entry map[string]any
		if err := json.Unmarshal([]byte(trimmed), &entry); err != nil {
			continue
		}
		message, _ := entry["message"].(map[string]any)
		content, _ := message["content"].([]any)

		if strings.Contains(trimmed, taskNotificationMarker) && entry["type"] == "user" {
			notifications = append(notifications, struct {
				at   int
				text string
			}{i, trimmed})
		}

		for _, block := range content {
			b, ok := block.(map[string]any)
			if !ok {
				continue
			}
			switch b["type"] {
			case "tool_use":
				if !isBackgroundLaunch(b) {
					continue
				}
				id, _ := b["id"].(string)
				if id == "" {
					continue
				}
				l := &transcriptLaunch{toolUseID: id, resultAt: -1}
				launches = append(launches, l)
				byToolUse[id] = l
			case "tool_result":
				id, _ := b["tool_use_id"].(string)
				l := byToolUse[id]
				if l == nil {
					continue
				}
				l.resultAt = i
				l.ids = resultTaskIDs(entry, b)
			}
		}
	}

	for _, l := range launches {
		if len(l.ids) == 0 {
			continue
		}
		matched := false
		for _, n := range notifications {
			if n.at <= l.resultAt {
				continue
			}
			for _, id := range l.ids {
				if strings.Contains(n.text, id) {
					matched = true
					break
				}
			}
			if matched {
				break
			}
		}
		if !matched {
			return true
		}
	}
	return false
}

// isBackgroundLaunch reports whether a tool_use block launches background work: a Bash call with
// run_in_background, a Monitor call, or an Agent (or Task) call launched asynchronously.
func isBackgroundLaunch(block map[string]any) bool {
	name, _ := block["name"].(string)
	input, _ := block["input"].(map[string]any)
	background, _ := input["run_in_background"].(bool)
	switch name {
	case "Monitor":
		return true
	case "Bash", "Agent", "Task":
		return background
	}
	return false
}

// resultTaskIDs collects the task ids a launch's tool result returned, from the structured
// toolUseResult first and from the result text as a fallback.
func resultTaskIDs(entry, result map[string]any) []string {
	var ids []string
	if structured, ok := entry["toolUseResult"].(map[string]any); ok {
		for _, key := range resultIDKeys {
			if id, _ := structured[key].(string); id != "" {
				ids = append(ids, id)
			}
		}
	}
	if len(ids) > 0 {
		return ids
	}
	for _, m := range resultTextID.FindAllStringSubmatch(resultText(result["content"]), -1) {
		ids = append(ids, m[1])
	}
	return ids
}

// resultText flattens a tool_result content value, a string or a list of text blocks.
func resultText(content any) string {
	switch v := content.(type) {
	case string:
		return v
	case []any:
		var parts []string
		for _, item := range v {
			if m, ok := item.(map[string]any); ok {
				if text, _ := m["text"].(string); text != "" {
					parts = append(parts, text)
				}
			}
		}
		return strings.Join(parts, "\n")
	}
	return ""
}
