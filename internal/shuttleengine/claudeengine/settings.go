// settings.go composes the Claude Code settings.json document Prepare writes for each run: a Stop
// hook that appends every turn-end event to the run's events.jsonl (the only channel ParseEvents
// reads),
// and the PreToolUse guardrails that keep a run's work visible in its own pane — denying the
// in-process Agent tool (or, in a fork-mode run, letting fork subagents through it while still
// denying every other subagent type; a run with Spec.AllowAgentTool set installs no Agent deny at all),
// refusing `lyx webster` verbs from inside a fork in a fork-mode run (the fork-context deadlock
// guard), denying AskUserQuestion in autonomous runs (where there is no operator present to answer
// it), and recording — never denying — a live AskUserQuestion call in interactive runs so the run loop holds it like any turn end without output.
// The Bash tool's default stdin is set by the env file Prepare writes (see command.go), not by a hook.
// buildDenyNotice, built beside those hooks so the two cannot drift, is the one-line system-prompt notice announcing each installed deny to the session.
// Every document also sets `promptSuggestionEnabled` to false: a capture carries no styling,
// so a greyed suggestion in an empty input box would read as a draft and IdleSession would never pass.

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
const steerAskUserQuestionDeny = "you cannot open an interactive dialog here. If you are blocked or need operator input, state the question as your final message and end your turn WITHOUT writing the result file. The run then waits for the answer, which arrives as your next turn."

// steerWebsterForkDeny guards against fork-context deadlock: a fork inherits Master's await-batch loop and polling it would livelock the run.
// It refuses `lyx webster` commands inside forks (detected by top-level agent_id in the payload). Must contain no single/double quote or backslash (checked at init).
const steerWebsterForkDeny = "lyx webster verbs belong to the Master session, never a fork. You are an implementer fork: do your batch work and write your report, and do NOT run any lyx webster command (not await-batch, not anything) — polling for the report you must write only deadlocks the run. This call is refused."

// noticeAgentDeny announces the Agent deny in a non-fork run;
// it must hold for every session that receives it.
const noticeAgentDeny = "The Agent tool is unavailable in this session: do all exploration and work in this session, with no subagents."

// noticeAgentForkDeny announces the Agent deny in a fork run, where forks inherit this system prompt too.
const noticeAgentForkDeny = "The Agent tool accepts only fork subagents and refuses every other subagent type; a fork does its own work and never spawns further subagents."

// noticeAskUserQuestionDeny announces the AskUserQuestion deny in an autonomous run.
const noticeAskUserQuestionDeny = "AskUserQuestion is unavailable: when blocked or needing operator input, state the question as your final message and end your turn without writing the result file; the run then waits for the answer, which arrives as your next turn."

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
// PromptSuggestionEnabled has no omitempty and buildSettings leaves it false, so every document carries `"promptSuggestionEnabled": false`.
type settingsDoc struct {
	Hooks                   settingsHooks `json:"hooks"`
	PromptSuggestionEnabled bool          `json:"promptSuggestionEnabled"`
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

// denyInstalls reports which standing denies a run installs: the Agent deny, and the AskUserQuestion deny.
// buildSettings and buildDenyNotice both read it, so the hooks and their announcement cannot drift.
// An interactive run's AskUserQuestion hook only records, never denies, so it reports no AskUserQuestion deny.
// allowAgentTool is the per-run override (Spec.AllowAgentTool): it removes the Agent deny whatever cfg.ClaudeDenyAgentTool says.
func denyInstalls(interactive bool, cfg shuttleengine.Config, allowAgentTool bool) (agentDeny, askUserDeny bool) {
	return cfg.ClaudeDenyAgentTool && !allowAgentTool, !interactive && cfg.ClaudeDenyAskUserQuestion
}

// buildDenyNotice returns the one-line system-prompt notice announcing each deny buildSettings installs under the same inputs, or "" when it installs none.
func buildDenyNotice(interactive bool, cfg shuttleengine.Config, forkSubagents, allowAgentTool bool) string {
	agentDeny, askUserDeny := denyInstalls(interactive, cfg, allowAgentTool)
	var sentences []string
	if agentDeny {
		if forkSubagents {
			sentences = append(sentences, noticeAgentForkDeny)
		} else {
			sentences = append(sentences, noticeAgentDeny)
		}
	}
	if askUserDeny {
		sentences = append(sentences, noticeAskUserQuestionDeny)
	}
	return strings.Join(sentences, " ")
}

// buildSettings marshals settings.json: a Stop hook appending turn-end events to eventsPathPosix, and PreToolUse guardrails per cfg and interactive.
// eventsPathPosix must be a git-bash POSIX path (from shuttleengine.PosixPath); it's embedded via shQuote to escape any apostrophes.
// Agent-tool and AskUserQuestion denies are controlled by cfg; forkSubagents narrows the Agent deny to non-fork subagent types and adds a webster-verb guard,
// and allowAgentTool drops the Agent deny entirely while leaving the webster-verb guard keyed on forkSubagents alone.
func buildSettings(eventsPathPosix string, interactive bool, cfg shuttleengine.Config, forkSubagents, allowAgentTool bool) ([]byte, error) {
	quotedEventsPath := shQuote(eventsPathPosix)
	stopCmd := fmt.Sprintf("cat >> %s && printf '\\n' >> %s", quotedEventsPath, quotedEventsPath)

	doc := settingsDoc{
		Hooks: settingsHooks{
			Stop: []hookEntry{
				{Hooks: []hookCommand{{Type: "command", Command: stopCmd}}},
			},
		},
	}

	agentDeny, askUserDeny := denyInstalls(interactive, cfg, allowAgentTool)
	if agentDeny {
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
	} else if askUserDeny {
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

// noticeTextForbiddenChars extends steerTextForbiddenChars with line breaks, since the assembled notice rides one command-line argument that reed types with tmux send-keys.
const noticeTextForbiddenChars = steerTextForbiddenChars + "\n\r"

// init panics if any steer constant contains a forbidden character (checked at package load).
func init() {
	for _, steer := range []string{steerAgentDeny, steerAskUserQuestionDeny, steerAgentNonForkDeny, steerWebsterForkDeny} {
		if strings.ContainsAny(steer, steerTextForbiddenChars) {
			panic(fmt.Sprintf("claudeengine: steer text contains a forbidden character (one of %q), which would break the JSON payload or the echo hook command: %q", steerTextForbiddenChars, steer))
		}
	}
	for _, notice := range []string{noticeAgentDeny, noticeAgentForkDeny, noticeAskUserQuestionDeny} {
		if strings.ContainsAny(notice, noticeTextForbiddenChars) {
			panic(fmt.Sprintf("claudeengine: notice text contains a forbidden character (one of %q), which would break the one-line launch argument: %q", noticeTextForbiddenChars, notice))
		}
	}
}
