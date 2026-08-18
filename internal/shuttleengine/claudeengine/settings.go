// settings.go composes the Claude Code settings.json document Prepare writes for each run: a Stop
// hook that appends every turn-end event to the run's events.jsonl (the only channel ParseEvents
// reads),
// and the PreToolUse guardrails that keep a run's work visible in its own pane — denying the
// in-process Agent tool (or, in a fork-mode run, letting fork subagents through it while still
// denying every other subagent type),
// refusing `lyx webster` verbs from inside a fork in a fork-mode run (the fork-context deadlock
// guard), denying AskUserQuestion in autonomous runs (where there is no operator present to answer
// it), and recording — never denying — a live AskUserQuestion call in interactive runs so the run
// loop can classify it as a real-time asking signal instead of waiting for the timeout.

package claudeengine

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// steerAgentDeny redirects the model back into this pane; shuttle's design is that every agent runs in a separate visible tmux pane, not Claude Code's in-process Agent tool.
const steerAgentDeny = "do the work in this session; nested agents are not available here — all work must stay visible in this pane"

// steerAgentNonForkDeny is the PreToolUse(Agent) deny reason for fork-mode runs, where fork subagents are allowed and every other subagent type is refused.
const steerAgentNonForkDeny = "only fork subagents may be spawned here; other agents are unavailable — do the work in this session or in your forks"

// steerAskUserQuestionDeny denies AskUserQuestion in autonomous runs, where no operator is present to answer.
const steerAskUserQuestionDeny = "you cannot open an interactive dialog here. If you are blocked or need operator input, state the question as your final message and end your turn WITHOUT writing the result file."

// steerWebsterForkDeny guards against fork-context deadlock: a fork inherits Master's await-batch loop and polling it would livelock the run.
// It refuses `lyx webster` commands inside forks (detected by top-level agent_id in the payload). Must contain no single/double quote or backslash (checked at init).
const steerWebsterForkDeny = "lyx webster verbs belong to the Master session, never a fork. You are an implementer fork: do your batch work and write your report, and do NOT run any lyx webster command (not await-batch, not anything) — polling for the report you must write only deadlocks the run. This call is refused."

// hookCommand is one Claude Code hook invocation, run under git-bash on Windows.
type hookCommand struct {
	Type    string `json:"type"`
	Command string `json:"command"`
}

// hookEntry is one matcher/hooks pair in a settings.json hook event list.
// Matcher is omitted for events that carry no tool-name matcher (like Stop).
type hookEntry struct {
	Matcher string        `json:"matcher,omitempty"`
	Hooks   []hookCommand `json:"hooks"`
}

// settingsHooks is the "hooks" object of a Claude Code settings.json document.
// PreToolUse is omitted when an autonomous run has both denies off; interactive runs always carry at least the AskUserQuestion marker.
type settingsHooks struct {
	Stop       []hookEntry `json:"Stop"`
	PreToolUse []hookEntry `json:"PreToolUse,omitempty"`
}

// settingsDoc is the Claude Code settings.json document Prepare writes.
type settingsDoc struct {
	Hooks settingsHooks `json:"hooks"`
}

// shQuote wraps s in POSIX shell single quotes, escaping embedded quotes with the standard sh idiom (close, emit escaped quote, reopen).
// This prevents paths containing apostrophes from breaking out of the quoted argument.
func shQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// denyJSON builds the `echo`-able deny-and-steer JSON payload for PreToolUse hooks.
// steer must contain no single quotes, as it rides inside a single-quoted `echo` argument under git-bash.
func denyJSON(steer string) string {
	return fmt.Sprintf(
		`{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny","permissionDecisionReason":"%s"}}`,
		steer,
	)
}

// buildSettings marshals settings.json: a Stop hook appending turn-end events to eventsPathPosix, and PreToolUse guardrails per cfg and interactive.
// eventsPathPosix must be a git-bash POSIX path (from shuttleengine.PosixPath); it's embedded via shQuote to escape any apostrophes.
// Agent-tool and AskUserQuestion denies are controlled by cfg; forkSubagents narrows the Agent deny to non-fork subagent types and adds a webster-verb guard.
func buildSettings(eventsPathPosix string, interactive bool, cfg shuttleengine.Config, forkSubagents bool) ([]byte, error) {
	quotedEventsPath := shQuote(eventsPathPosix)
	stopCmd := fmt.Sprintf("cat >> %s && printf '\\n' >> %s", quotedEventsPath, quotedEventsPath)

	doc := settingsDoc{
		Hooks: settingsHooks{
			Stop: []hookEntry{
				{Hooks: []hookCommand{{Type: "command", Command: stopCmd}}},
			},
		},
	}

	if cfg.ClaudeDenyAgentTool {
		if forkSubagents {
			// Grep the payload for a fork subagent_type; a match exits 0 allowing the call, no
			// match echoes the deny JSON. Whether the fork carried a name is deliberately NOT
			// part of this test — a named fork is a defect signal the AUDIT records as
			// ForkAudit.NamedSpawns for the caller's policy to interpret, never something this
			// hook refuses mid-run.
			agentCmd := fmt.Sprintf(`grep -q '"subagent_type":"fork"' || echo '%s'`, denyJSON(steerAgentNonForkDeny))
			doc.Hooks.PreToolUse = append(doc.Hooks.PreToolUse, hookEntry{
				Matcher: "Agent",
				Hooks:   []hookCommand{{Type: "command", Command: agentCmd}},
			})
		} else {
			doc.Hooks.PreToolUse = append(doc.Hooks.PreToolUse, hookEntry{
				Matcher: "Agent",
				Hooks:   []hookCommand{{Type: "command", Command: "echo '" + denyJSON(steerAgentDeny) + "'"}},
			})
		}
	}
	if forkSubagents {
		// Fork-context webster-verb guard: deny `lyx webster` inside forks (detected by top-level agent_id in payload).
		// Ending with `; true` guarantees exit 0 so non-webster or non-fork calls are allowed.
		webForkCmd := "in=$(cat); { printf '%s' \"$in\" | grep -q '\"agent_id\"'; } && { printf '%s' \"$in\" | grep -Eq 'lyx[[:space:]]+webster'; } && echo '" + denyJSON(steerWebsterForkDeny) + "'; true"
		doc.Hooks.PreToolUse = append(doc.Hooks.PreToolUse, hookEntry{
			Matcher: "Bash",
			Hooks:   []hookCommand{{Type: "command", Command: webForkCmd}},
		})
	}
	if interactive {
		// Record the live ask via the Stop hook's append command, allowing the tool call to proceed unhindered.
		doc.Hooks.PreToolUse = append(doc.Hooks.PreToolUse, hookEntry{
			Matcher: "AskUserQuestion",
			Hooks:   []hookCommand{{Type: "command", Command: stopCmd}},
		})
	} else if cfg.ClaudeDenyAskUserQuestion {
		doc.Hooks.PreToolUse = append(doc.Hooks.PreToolUse, hookEntry{
			Matcher: "AskUserQuestion",
			Hooks:   []hookCommand{{Type: "command", Command: "echo '" + denyJSON(steerAskUserQuestionDeny) + "'"}},
		})
	}

	data, err := json.Marshal(doc)
	if err != nil {
		return nil, fmt.Errorf("marshal claude settings: %w", err)
	}
	return data, nil
}

// steerTextForbiddenChars are characters that would corrupt steer constants in JSON or shell quoting layers (checked at init).
const steerTextForbiddenChars = `'"\`

// init panics if any steer constant contains a forbidden character (checked at package load).
func init() {
	for _, steer := range []string{steerAgentDeny, steerAskUserQuestionDeny, steerAgentNonForkDeny, steerWebsterForkDeny} {
		if strings.ContainsAny(steer, steerTextForbiddenChars) {
			panic(fmt.Sprintf("claudeengine: steer text contains a forbidden character (one of %q), which would break the JSON payload or the echo hook command: %q", steerTextForbiddenChars, steer))
		}
	}
}
