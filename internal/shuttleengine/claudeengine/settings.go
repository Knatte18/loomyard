// settings.go composes the Claude Code settings.json document Prepare
// writes for each run: a Stop hook that appends every turn-end event to the
// run's events.jsonl (the only channel ParseEvents reads), and the
// PreToolUse guardrails that keep a run's work visible in its own pane —
// denying the in-process Agent tool (or, in a fork-mode run, allowing only
// unnamed fork calls through it), denying AskUserQuestion in autonomous runs
// (where there is no operator present to answer it), and recording — never
// denying — a live AskUserQuestion call in interactive runs so the run loop
// can classify it as a real-time asking signal instead of waiting for the
// timeout.

package claudeengine

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// steerAgentDeny is the PreToolUse(Agent) deny reason: it always redirects
// the model back into this pane, since shuttle's whole design point is that
// every agent is a separate, visible tmux pane — never Claude Code's
// in-process Agent tool (mux-hooks-exploration.md §B).
const steerAgentDeny = "do the work in this session; nested agents are not available here — all work must stay visible in this pane"

// steerAgentNonForkDeny is the PreToolUse(Agent) deny reason used in
// fork-mode runs in place of steerAgentDeny: an unnamed fork call is allowed
// through (see the conditional hook built in buildSettings below), so the
// steer text narrows to "other agents", not "nested agents" generally.
const steerAgentNonForkDeny = "only fork subagents may be spawned here; other agents are unavailable — do the work in this session or in your forks"

// steerAskUserQuestionDeny is the PreToolUse(AskUserQuestion) deny reason
// used only for autonomous runs: there is no operator present to answer an
// interactive dialog, so the model is steered to end its turn with the
// question as its final message instead.
const steerAskUserQuestionDeny = "you cannot open an interactive dialog here. If you are blocked or need operator input, state the question as your final message and end your turn WITHOUT writing the result file."

// hookCommand is one Claude Code hook invocation: a shell command run under
// git-bash on Windows.
type hookCommand struct {
	Type    string `json:"type"`
	Command string `json:"command"`
}

// hookEntry is one matcher/hooks pair inside a settings.json hook event
// list. Matcher is omitted entirely for events (like Stop) that carry no
// tool-name matcher.
type hookEntry struct {
	Matcher string        `json:"matcher,omitempty"`
	Hooks   []hookCommand `json:"hooks"`
}

// settingsHooks is the "hooks" object of a Claude Code settings.json
// document. PreToolUse is omitted entirely (via omitempty on a nil slice)
// when an autonomous run has both denies off, so that case emits no
// PreToolUse key at all; an interactive run always carries at least the
// non-denying AskUserQuestion marker entry.
type settingsHooks struct {
	Stop       []hookEntry `json:"Stop"`
	PreToolUse []hookEntry `json:"PreToolUse,omitempty"`
}

// settingsDoc is the Claude Code settings.json document Prepare writes.
type settingsDoc struct {
	Hooks settingsHooks `json:"hooks"`
}

// shQuote wraps s in POSIX shell single quotes for embedding in a git-bash
// hook command, escaping any embedded literal single quote with the
// standard sh idiom (close the quote, emit an escaped quote, reopen the
// quote) so a run directory path containing an apostrophe cannot break out
// of the quoted argument and have its remainder interpreted as shell syntax.
func shQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// denyJSON builds the literal `echo`-able deny-and-steer JSON payload a
// PreToolUse hook command prints on stdout to deny a tool call. steer must
// contain no single quotes: the payload rides inside a single-quoted `echo`
// argument under git-bash, so an embedded `'` would prematurely close that
// argument.
func denyJSON(steer string) string {
	return fmt.Sprintf(
		`{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny","permissionDecisionReason":"%s"}}`,
		steer,
	)
}

// buildSettings marshals the Claude Code settings.json document for one
// run: a Stop hook that appends each turn-end event (plus a trailing
// newline, guaranteeing JSONL line separation) to eventsPathPosix, and the
// PreToolUse guardrails cfg and interactive select. eventsPathPosix must
// already be a git-bash POSIX path (shuttleengine.PosixPath) — hook
// commands run under git-bash on Windows, where a bare backslash path is
// silently misinterpreted as an escape sequence. eventsPathPosix is embedded
// via shQuote rather than a bare `'%s'`, so a run directory containing a
// literal apostrophe (an unusual but legal Windows path character) cannot
// break out of the quoted shell argument.
//
// The Agent-tool deny is included whenever cfg.ClaudeDenyAgentTool is set,
// in both interactive and autonomous runs — Claude Code's in-process Agent
// tool must never be allowed to run work invisibly, regardless of who is
// watching the pane. AskUserQuestion's PreToolUse entry is mutually
// exclusive on the interactive/autonomous split: an interactive run gets a
// non-denying marker hook — reusing the SAME append command as the Stop
// hook — so the live tool call is recorded into events.jsonl (and thus
// classifiable by ParseEvents/pollEventsTick) while the tool call itself
// proceeds unhindered, since an operator is present to actually answer it;
// an autonomous run instead gets the existing deny, gated on
// cfg.ClaudeDenyAskUserQuestion, since there is no operator to answer a
// dialog there (Shared Decision "Interactive bool encodes the discussion's
// Autonomous default true", and the live-ask-signal decision).
//
// forkSubagents changes the Agent-tool deny's shape when
// cfg.ClaudeDenyAgentTool is set: false keeps today's blanket deny
// (steerAgentDeny) unchanged; true replaces it with a conditional hook that
// greps the hook's stdin payload (the tool call's compact-JSON tool_input)
// for the substring `"subagent_type":"fork"` — a match exits 0 printing
// nothing, allowing an unnamed fork call through, while no match falls
// through to echoing the steerAgentNonForkDeny deny JSON. This is
// deliberately a steering guard, not a security boundary: the `name`
// parameter a caller could still pass to fork() is NOT hook-checked, since a
// `"name"` substring is indistinguishable from ordinary prompt-string
// content by grep — unnamed-ness is verified post-hoc by AuditForks reading
// the parent transcript instead. The hook applies session-wide, so it also
// polices any Agent call attempted from inside a fork's own pane, not just
// the parent's. When cfg.ClaudeDenyAgentTool is false, fork mode emits no
// Agent hook at all, exactly as before this parameter existed.
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
			// grep reads the hook's stdin payload; a match (an unnamed-fork
			// call's compact-JSON tool_input) exits 0 printing nothing, which
			// allows the call; no match falls through to echoing the deny
			// JSON. See buildSettings' doc comment above for why this is a
			// steering guard, not a security boundary.
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
	if interactive {
		// Record the live ask instead of denying it: the marker hook reuses
		// the Stop hook's append command verbatim (same events.jsonl, same
		// JSONL contract) and emits no deny JSON, so the tool call proceeds
		// normally while ParseEvents gets a classifiable line the instant it
		// opens. This marker is always on for interactive runs — there is no
		// config key to disable it, since recording a live ask is never
		// harmful to an operator who is present to answer it.
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

// steerTextForbiddenChars are the characters a steer constant must never
// contain, checked at package init (see the init func below). Each rides
// inside TWO nested quoting layers: denyJSON substitutes it raw into a JSON
// string literal (so a literal `"` closes that string early and a literal
// `\` starts a JSON escape sequence, either corrupting the payload), and the
// whole JSON payload then rides inside a single-quoted echo argument under
// git-bash (so a literal `'` closes that shell argument early). All three
// must stay absent, not just the `'` a narrower guard would catch.
const steerTextForbiddenChars = `'"\`

// init panics at package load if either steer constant contains a character
// from steerTextForbiddenChars — turning a future edit that reintroduces one
// into an immediate, unmissable failure rather than a subtle hook-command or
// JSON-payload corruption discovered only via a live smoke test.
func init() {
	for _, steer := range []string{steerAgentDeny, steerAskUserQuestionDeny, steerAgentNonForkDeny} {
		if strings.ContainsAny(steer, steerTextForbiddenChars) {
			panic(fmt.Sprintf("claudeengine: steer text contains a forbidden character (one of %q), which would break the JSON payload or the echo hook command: %q", steerTextForbiddenChars, steer))
		}
	}
}
