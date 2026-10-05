// background.go lists the background tasks a Stop payload describes as still outstanding, so
// ParseEvents can surface a non-empty list as EventWaiting instead of EventStop.
// Two signals feed the list, deduplicated by task id:
// a background_tasks[] entry whose status is "running" (type "subagent" is a fork, any other type,
// such as a shell, is a shell), and a background launch in the transcript at transcript_path that
// no later <task-notification> user message names by the task id the launch's tool result returned
// (Agent and Task launches are forks, Bash and Monitor launches are shells).
// Both signals are always read, since a running shell entry and an unmatched fork launch can be
// outstanding together.
// An unreadable or missing transcript counts as nothing outstanding from that signal: it must
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
// running at the turn end, payload entries first.
func outstandingBackgroundTasks(fields map[string]any) []shuttleengine.BackgroundTask {
	var tasks []shuttleengine.BackgroundTask
	seen := map[string]bool{}
	add := func(t shuttleengine.BackgroundTask) {
		if seen[t.ID] {
			return
		}
		seen[t.ID] = true
		tasks = append(tasks, t)
	}

	if entries, ok := fields["background_tasks"].([]any); ok {
		for _, e := range entries {
			entry, ok := e.(map[string]any)
			if !ok {
				continue
			}
			if status, _ := entry["status"].(string); status != "running" {
				continue
			}
			add(payloadTask(entry))
		}
	}

	path, _ := fields["transcript_path"].(string)
	if path == "" {
		return tasks
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return tasks
	}
	for _, t := range transcriptHasUnmatchedLaunch(data) {
		add(t)
	}
	return tasks
}

// payloadTask converts one running background_tasks[] entry: type "subagent" is a fork, any other
// type is a shell labelled by its command, else its description, else its id.
func payloadTask(entry map[string]any) shuttleengine.BackgroundTask {
	id, _ := entry["id"].(string)
	if typ, _ := entry["type"].(string); typ == "subagent" {
		return shuttleengine.BackgroundTask{Kind: shuttleengine.BackgroundFork, ID: id}
	}
	label, _ := entry["command"].(string)
	if label == "" {
		label, _ = entry["description"].(string)
	}
	if label == "" {
		label = id
	}
	return shuttleengine.BackgroundTask{Kind: shuttleengine.BackgroundShell, ID: id, Label: label}
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
// id is never named by a later task-notification user message, and returns them as tasks.
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
			unmatched = append(unmatched, shuttleengine.BackgroundTask{Kind: l.kind, ID: l.ids[0], Label: l.label})
		}
	}
	return unmatched
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
