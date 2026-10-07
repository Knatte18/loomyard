// background.go lists the background tasks a Stop payload describes as still outstanding, so
// ParseEvents can surface a non-empty list as EventWaiting instead of EventStop.
// The Stop payload's background_tasks key is authoritative:
// when it carries a list, even an empty one, its entries whose status is "running" are the whole
// outstanding list (type "subagent" is a fork, any other type, such as a shell, is a shell) and the
// transcript is not read.
// A payload that omits a running task, lists it under another status, or changes an entry's shape
// under the same key counts as no outstanding work, and the transcript does not correct it.
// Only when the key is absent or its value is not a list does the transcript at transcript_path
// decide: a background launch that no later completion notification names by the task id the
// launch's tool result returned is outstanding (Agent and Task launches are forks, Bash and Monitor
// launches are shells).
// A completion notification is a <task-notification> user message, a queue-operation line whose
// content names the task id, or a queued_command attachment whose prompt names it.
// An unreadable or missing transcript counts as nothing outstanding: it must
// never keep a run waiting on a fault.
// All Claude payload-shape knowledge stays in this package, per the provider-seam containment decision.
package claudeengine

import (
	"encoding/json"
	"os"
	"regexp"
	"strings"

	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// taskNotificationMarker opens the user message Claude Code injects when a background task finishes.
const taskNotificationMarker = "<task-notification>"

// resultIDKeys are the toolUseResult fields that carry a launched task's id.
var resultIDKeys = []string{"agentId", "backgroundTaskId", "bash_id", "shellId", "taskId", "task_id"}

// resultTextID extracts a task id from a tool result's text ("... with ID: bx12ab34", "task abc123").
var resultTextID = regexp.MustCompile(`(?i)\b(?:id|task)[:\s]+([A-Za-z0-9_-]{6,})`)

// outstandingBackgroundTasks lists the background tasks the Stop payload fields describe as still
// running at the turn end.
// A background_tasks list is the whole answer; the transcript is the fallback for a payload without one.
func outstandingBackgroundTasks(fields map[string]any) []shuttleengine.BackgroundTask {
	if entries, ok := fields["background_tasks"].([]any); ok {
		var tasks []shuttleengine.BackgroundTask
		seen := map[string]bool{}
		for _, e := range entries {
			entry, ok := e.(map[string]any)
			if !ok {
				continue
			}
			if status, _ := entry["status"].(string); status != "running" {
				continue
			}
			task := payloadTask(entry)
			if seen[task.ID] {
				continue
			}
			seen[task.ID] = true
			tasks = append(tasks, task)
		}
		return tasks
	}

	path, _ := fields["transcript_path"].(string)
	if path == "" {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	return transcriptHasUnmatchedLaunch(data)
}

// payloadTask converts one running background_tasks[] entry: type "subagent" is a fork labelled by
// its description when present, any other type is a shell labelled by its command, else its
// description, else its id.
func payloadTask(entry map[string]any) shuttleengine.BackgroundTask {
	id, _ := entry["id"].(string)
	if typ, _ := entry["type"].(string); typ == "subagent" {
		description, _ := entry["description"].(string)
		return shuttleengine.BackgroundTask{Kind: shuttleengine.BackgroundFork, ID: id, Label: description, Signal: shuttleengine.SignalPayload}
	}
	label, _ := entry["command"].(string)
	if label == "" {
		label, _ = entry["description"].(string)
	}
	if label == "" {
		label = id
	}
	return shuttleengine.BackgroundTask{Kind: shuttleengine.BackgroundShell, ID: id, Label: label, Signal: shuttleengine.SignalPayload}
}

// transcriptLaunch is one background launch found in the transcript.
type transcriptLaunch struct {
	toolUseID string
	kind      shuttleengine.BackgroundKind
	label     string
	ids       []string // Task ids the launch's tool result returned.
	resultAt  int      // Line index of the tool result; notifications must come after it.
}

// transcriptHasUnmatchedLaunch scans transcript JSONL for background launches whose returned task
// id is never named by a later completion notification, and returns them as tasks.
// A notification is a task-notification user message, a queue-operation line whose content names
// the id, or a queued_command attachment whose prompt names it.
// A launch whose result carries no recoverable id cannot be matched and is not counted.
func transcriptHasUnmatchedLaunch(data []byte) []shuttleengine.BackgroundTask {
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

		if text, ok := notificationText(entry, trimmed); ok {
			notifications = append(notifications, struct {
				at   int
				text string
			}{i, text})
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
				l.kind, l.label = launchKindAndLabel(b)
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

	var unmatched []shuttleengine.BackgroundTask
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
			unmatched = append(unmatched, shuttleengine.BackgroundTask{Kind: l.kind, ID: l.ids[0], Label: l.label, Signal: shuttleengine.SignalTranscript})
		}
	}
	return unmatched
}

// notificationText returns the text of a transcript line that reports a background task's
// completion, and whether the line is one.
// The shapes are a task-notification user message, a queue-operation line (its content) and a
// queued_command attachment (its prompt).
func notificationText(entry map[string]any, line string) (string, bool) {
	switch entry["type"] {
	case "user":
		return line, strings.Contains(line, taskNotificationMarker)
	case "queue-operation":
		content, _ := entry["content"].(string)
		return content, content != ""
	}
	if attachment, ok := entry["attachment"].(map[string]any); ok && attachment["type"] == "queued_command" {
		prompt, _ := attachment["prompt"].(string)
		return prompt, prompt != ""
	}
	return "", false
}

// launchKindAndLabel classifies a background launch: Agent and Task are forks with no label, Bash
// and Monitor are shells labelled by the input's command, else its description.
func launchKindAndLabel(block map[string]any) (shuttleengine.BackgroundKind, string) {
	name, _ := block["name"].(string)
	if name == "Agent" || name == "Task" {
		return shuttleengine.BackgroundFork, ""
	}
	input, _ := block["input"].(map[string]any)
	label, _ := input["command"].(string)
	if label == "" {
		label, _ = input["description"].(string)
	}
	return shuttleengine.BackgroundShell, label
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
