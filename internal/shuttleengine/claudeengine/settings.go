// settings.go composes the Claude Code settings.json document Prepare writes for each run:
// a Stop hook that appends every turn-end event to the run's events.jsonl (the only channel ParseEvents reads).
// The UserPromptSubmit, StopFailure, Notification and SessionEnd hooks append their payloads to the same file for ParseSessionSignals,
// and so does the SessionStart hook of a run whose spec sets a context-after-compaction command, which then runs that command so its output joins the session's context.
// Every recording hook writes a stamp line with the hook-side time before its payload, which ParseEvents skips.
// The document also carries the PreToolUse guardrails that keep a run's work visible in its own pane —
// denying the in-process Agent tool (or, in a fork-mode run, letting fork subagents through it while still denying every other subagent type; a run with Spec.AllowAgentTool set installs no Agent deny at all),
// refusing `lyx webster` verbs from inside a fork in a fork-mode run (the fork-context deadlock guard),
// denying AskUserQuestion in autonomous runs (where there is no operator present to answer it),
// and recording — never denying — a live AskUserQuestion call in interactive runs so the run loop holds it like any turn end without output.
// The Bash tool's default stdin is set by the env file Prepare writes (see command.go), not by a hook.
// buildDenyNotice, built from the same standingDenies table so the two cannot drift, is the one-line system-prompt notice announcing each installed deny to the session.
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

// steerPythonDeny refuses a Bash python invocation; it must contain no single/double quote or backslash (checked at init).
const steerPythonDeny = "lyx agents never run Python; edit files with Edit or Write, search and read with grep, awk or cat"

// noticePythonDeny announces the python deny in the same wording as its steer.
const noticePythonDeny = steerPythonDeny + "."

// pythonCommandPattern is the grep -E pattern the python deny matches against the raw PreToolUse payload JSON.
// It matches python, python3 or python3.<minor>, optionally path-prefixed, as a whole word in command position:
// right after the "command" key's opening quote, after a JSON-escaped newline, after ; & | ( or a backtick (which also covers $( ),
// or after one of env, exec, xargs, sudo, time.
// The shell single quotes around it forbid a single quote in the pattern.
const pythonCommandPattern = `("command"[[:space:]]*:[[:space:]]*"|\\n|[;&|(` + "`" + `]|(^|[^A-Za-z0-9_-])(env|exec|xargs|sudo|time)[[:space:]]+)[[:space:]]*([^[:space:]"\\;&|()` + "`" + `]*/)?python(3(\.[0-9]+)?)?([^A-Za-z0-9_./-]|$)`

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
// Every event but PreToolUse is a recording hook installed for every session.
// PreToolUse is omitted when an autonomous run has every deny off; interactive runs always carry at least the AskUserQuestion marker.
type settingsHooks struct {
	Stop             []hookEntry `json:"Stop"`
	PreToolUse       []hookEntry `json:"PreToolUse,omitempty"`
	UserPromptSubmit []hookEntry `json:"UserPromptSubmit"`
	StopFailure      []hookEntry `json:"StopFailure"`
	Notification     []hookEntry `json:"Notification"`
	SessionEnd       []hookEntry `json:"SessionEnd"`
	SessionStart     []hookEntry `json:"SessionStart,omitempty"`
}

// compactionMatcher is the SessionStart matcher for a session restarting after a compaction.
const compactionMatcher = "compact"

// SessionStartContext renders the additional-context JSON a SessionStart hook prints to add text to the session's context.
func SessionStartContext(text string) ([]byte, error) {
	type specificOutput struct {
		HookEventName     string `json:"hookEventName"`
		AdditionalContext string `json:"additionalContext"`
	}
	data, err := json.Marshal(struct {
		HookSpecificOutput specificOutput `json:"hookSpecificOutput"`
	}{specificOutput{HookEventName: hookEventSessionStart, AdditionalContext: text}})
	if err != nil {
		return nil, fmt.Errorf("marshal session start context: %w", err)
	}
	return data, nil
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

// denyInputs are the run inputs that decide which standing denies a run installs.
// allowAgentTool is the per-run override (Spec.AllowAgentTool): it removes the Agent deny whatever cfg.ClaudeDenyAgentTool says.
type denyInputs struct {
	interactive    bool
	cfg            shuttleengine.Config
	forkSubagents  bool
	allowAgentTool bool
}

// standingDeny is one PreToolUse deny lyx installs, with the system-prompt sentence announcing it.
// buildSettings and buildDenyNotice both iterate standingDenies, so the hooks and their announcement cannot drift.
type standingDeny struct {
	// matcher is the tool name the PreToolUse entry matches.
	matcher string
	// command is the hook command, which carries steer in its deny JSON.
	command string
	// steer is the deny reason shown to the model.
	steer string
	// notice is the system-prompt sentence announcing the deny, or "" for a deny with none.
	notice string
	// installed reports whether a run with the given inputs installs this deny.
	installed func(denyInputs) bool
}

// standingDenies lists lyx's denies in the order their PreToolUse entries appear.
// An interactive run's AskUserQuestion hook only records, never denies, so it stays outside the table.
var standingDenies = []standingDeny{
	{
		// Grep the payload for a fork subagent_type; a match exits 0 allowing the call, no
		// match echoes the deny JSON. Whether the fork carried a name is deliberately NOT
		// part of this test — a named fork is a defect signal the AUDIT records as
		// ForkAudit.NamedSpawns for the caller's policy to interpret, never something this
		// hook refuses mid-run.
		matcher:   "Agent",
		command:   fmt.Sprintf(`grep -q '"subagent_type":"fork"' || echo '%s'`, denyJSON(steerAgentNonForkDeny)),
		steer:     steerAgentNonForkDeny,
		notice:    noticeAgentForkDeny,
		installed: func(in denyInputs) bool { return in.cfg.ClaudeDenyAgentTool && !in.allowAgentTool && in.forkSubagents },
	},
	{
		matcher:   "Agent",
		command:   "echo '" + denyJSON(steerAgentDeny) + "'",
		steer:     steerAgentDeny,
		notice:    noticeAgentDeny,
		installed: func(in denyInputs) bool { return in.cfg.ClaudeDenyAgentTool && !in.allowAgentTool && !in.forkSubagents },
	},
	{
		// Fork-context webster-verb guard: deny `lyx webster` inside forks (detected by top-level agent_id in payload).
		// Ending with `; true` guarantees exit 0 so non-webster or non-fork calls are allowed.
		matcher:   "Bash",
		command:   "in=$(cat); { printf '%s' \"$in\" | grep -q '\"agent_id\"'; } && { printf '%s' \"$in\" | grep -Eq 'lyx[[:space:]]+webster'; } && echo '" + denyJSON(steerWebsterForkDeny) + "'; true",
		steer:     steerWebsterForkDeny,
		installed: func(in denyInputs) bool { return in.forkSubagents },
	},
	{
		matcher:   "AskUserQuestion",
		command:   "echo '" + denyJSON(steerAskUserQuestionDeny) + "'",
		steer:     steerAskUserQuestionDeny,
		notice:    noticeAskUserQuestionDeny,
		installed: func(in denyInputs) bool { return !in.interactive && in.cfg.ClaudeDenyAskUserQuestion },
	},
	{
		// A guardrail, not a barrier.
		// The pattern lets through python as an argument, behind bash -c, sh -c or eval, after a prefix it does not list (env FOO=1, nohup, command, timeout), and through a launcher such as py or uv run.
		// It falsely denies python after a listed separator inside a quoted argument, a heredoc body or another payload field such as description.
		// Ending with `; true` guarantees exit 0 so a non-match allows the call.
		matcher:   "Bash",
		command:   "grep -Eq '" + pythonCommandPattern + "' && echo '" + denyJSON(steerPythonDeny) + "'; true",
		steer:     steerPythonDeny,
		notice:    noticePythonDeny,
		installed: func(in denyInputs) bool { return in.cfg.ClaudeDenyPython },
	},
}

// buildDenyNotice returns the one-line system-prompt notice announcing each deny buildSettings installs under the same inputs, or "" when it installs none.
func buildDenyNotice(interactive bool, cfg shuttleengine.Config, forkSubagents, allowAgentTool bool) string {
	in := denyInputs{interactive: interactive, cfg: cfg, forkSubagents: forkSubagents, allowAgentTool: allowAgentTool}
	var sentences []string
	for _, deny := range standingDenies {
		if deny.notice != "" && deny.installed(in) {
			sentences = append(sentences, deny.notice)
		}
	}
	return strings.Join(sentences, " ")
}

// recordingCommand returns the hook command that appends one hook event to the events file at quotedEventsPath:
// a stamp line naming hookEventName with the hook-side time, then the payload on standard input followed by a newline.
// The two are joined by `;` so a failed stamp never blocks the payload, and the command's exit status is the payload append's.
// A failed `date` leaves the stamp's time empty, which the parser reads as no time.
func recordingCommand(hookEventName, quotedEventsPath string) string {
	stamp := `printf '{"` + stampHookKey + `":"` + hookEventName + `","` + stampAtKey + `":"%s"}\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" >> ` + quotedEventsPath
	payload := "cat >> " + quotedEventsPath + " && printf '\\n' >> " + quotedEventsPath
	return stamp + "; " + payload
}

// buildSettings marshals settings.json: a Stop hook and the other recording hooks appending their events to eventsPathPosix, and PreToolUse guardrails per cfg and interactive.
// eventsPathPosix must be a git-bash POSIX path (from shuttleengine.PosixPath); it's embedded via shQuote to escape any apostrophes.
// Agent-tool and AskUserQuestion denies are controlled by cfg; forkSubagents narrows the Agent deny to non-fork subagent types and adds a webster-verb guard,
// and allowAgentTool drops the Agent deny entirely while leaving the webster-verb guard keyed on forkSubagents alone.
// A non-empty contextAfterCompaction adds a SessionStart hook for the compaction matcher that records the event and then runs that command verbatim, so its standard output reaches the session's context; empty adds none.
func buildSettings(eventsPathPosix string, interactive bool, cfg shuttleengine.Config, forkSubagents, allowAgentTool bool, contextAfterCompaction string) ([]byte, error) {
	quotedEventsPath := shQuote(eventsPathPosix)
	recordingHook := func(hookEventName string, alwaysSucceeds bool) []hookEntry {
		command := recordingCommand(hookEventName, quotedEventsPath)
		if alwaysSucceeds {
			command += "; true"
		}
		return []hookEntry{{Hooks: []hookCommand{{Type: "command", Command: command}}}}
	}

	doc := settingsDoc{
		Hooks: settingsHooks{
			Stop:             recordingHook(hookEventStop, false),
			UserPromptSubmit: recordingHook(hookEventUserPromptSubmit, true),
			StopFailure:      recordingHook(hookEventStopFailure, true),
			Notification:     recordingHook(hookEventNotification, true),
			SessionEnd:       recordingHook(hookEventSessionEnd, true),
		},
	}

	if contextAfterCompaction != "" {
		recording := recordingHook(hookEventSessionStart, true)[0].Hooks[0]
		doc.Hooks.SessionStart = []hookEntry{{
			Matcher: compactionMatcher,
			Hooks:   []hookCommand{recording, {Type: "command", Command: contextAfterCompaction}},
		}}
	}

	in := denyInputs{interactive: interactive, cfg: cfg, forkSubagents: forkSubagents, allowAgentTool: allowAgentTool}
	for _, deny := range standingDenies {
		if deny.installed(in) {
			doc.Hooks.PreToolUse = append(doc.Hooks.PreToolUse, hookEntry{
				Matcher: deny.matcher,
				Hooks:   []hookCommand{{Type: "command", Command: deny.command}},
			})
		}
	}
	if interactive {
		// Record the live ask like a turn end, allowing the tool call to proceed unhindered.
		doc.Hooks.PreToolUse = append(doc.Hooks.PreToolUse, hookEntry{
			Matcher: "AskUserQuestion",
			Hooks:   recordingHook(hookEventPreToolUse, false)[0].Hooks,
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
	for _, deny := range standingDenies {
		if strings.ContainsAny(deny.steer, steerTextForbiddenChars) {
			panic(fmt.Sprintf("claudeengine: steer text contains a forbidden character (one of %q), which would break the JSON payload or the echo hook command: %q", steerTextForbiddenChars, deny.steer))
		}
		if strings.ContainsAny(deny.notice, noticeTextForbiddenChars) {
			panic(fmt.Sprintf("claudeengine: notice text contains a forbidden character (one of %q), which would break the one-line launch argument: %q", noticeTextForbiddenChars, deny.notice))
		}
	}
}
