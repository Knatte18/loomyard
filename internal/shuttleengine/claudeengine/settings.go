// settings.go composes the Claude Code settings.json document Prepare
// writes for each run: a Stop hook that appends every turn-end event to the
// run's events.jsonl (the only channel ParseEvents reads), and the
// PreToolUse guardrails that keep a run's work visible in its own pane —
// denying the in-process Agent tool (or, in a fork-mode run, allowing only
// unnamed fork calls through it), refusing `lyx webster` verbs from inside a
// fork in a fork-mode run (the fork-context deadlock guard), denying
// AskUserQuestion in autonomous runs (where there is no operator present to
// answer it), and recording — never denying — a live AskUserQuestion call in
// interactive runs so the run loop can classify it as a real-time asking
// signal instead of waiting for the timeout.

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

// steerWebsterForkDeny is the PreToolUse(Bash) deny reason for the
// fork-context webster-verb guard (see the Bash hook built in buildSettings
// below). webster's Master session forks one implementer per batch in-session,
// and every fork inherits Master's whole prompt — including the master
// template's await-batch poll loop. A fork that starts driving that loop
// itself (polling await-batch for the report it is itself meant to write)
// livelocks the run, so this guard refuses a `lyx webster` command whenever
// the hook fires inside a fork (the payload carries a top-level agent_id),
// steering the fork back to its own implementer job. It must contain no
// single/double quote or backslash, for the same nested-quoting reason as the
// other steer constants (checked at init).
const steerWebsterForkDeny = "lyx webster verbs belong to the Master session, never a fork. You are an implementer fork: do your batch work and write your report, and do NOT run any lyx webster command (not await-batch, not anything) — polling for the report you must write only deadlocks the run. This call is refused."

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
//
// forkSubagents also adds a second, independent PreToolUse(Bash) hook — the
// fork-context webster-verb guard (steerWebsterForkDeny) — regardless of
// cfg.ClaudeDenyAgentTool: it deterministically closes webster's fork-loop
// deadlock by refusing a `lyx webster` command whenever the hook fires inside
// a fork (its payload carries a top-level agent_id). See the inline comment
// on that hook below for the full rationale and the live-verified detection
// signal.
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
	if forkSubagents {
		// The fork-context webster-verb guard: a deterministic close for the
		// fork-loop deadlock (a fork inheriting Master's await-batch loop can
		// drive `lyx webster` verbs itself and livelock the run). The hook
		// reads its stdin payload once and denies the Bash call only when BOTH
		// (a) it fired inside a fork — the payload carries a top-level
		// `agent_id`, present only for a subagent, never a top-level Master
		// call (confirmed live on Claude Code 2.1.205; the fork's
		// transcript_path is NOT distinguishable, it equals the parent's) —
		// AND (b) the command is a `lyx webster` invocation. A fork's ordinary
		// Bash (git, verify, edit) is left untouched; only Master's own verbs
		// are policed. Ending with `; true` guarantees exit 0 (a non-matching
		// grep would otherwise exit non-zero) so a non-webster or non-fork call
		// is allowed, never a spurious hook error. Gated on forkSubagents alone
		// because only a fork-authorized (webster Master) run has forks that
		// could reach this state; a recovery strand is a separate,
		// non-fork-authorized session and never sees this hook. This is a
		// steering guard, not a security boundary — the same class as the
		// Agent-tool deny above — so the grep patterns are substring/whitespace
		// matches, not a full JSON parse. `lyx webster` is a webster-family
		// string rather than a Claude marker; it lives here (not in webster)
		// because hook-schema composition is claudeengine's own seam.
		webForkCmd := "in=$(cat); { printf '%s' \"$in\" | grep -q '\"agent_id\"'; } && { printf '%s' \"$in\" | grep -Eq 'lyx[[:space:]]+webster'; } && echo '" + denyJSON(steerWebsterForkDeny) + "'; true"
		doc.Hooks.PreToolUse = append(doc.Hooks.PreToolUse, hookEntry{
			Matcher: "Bash",
			Hooks:   []hookCommand{{Type: "command", Command: webForkCmd}},
		})
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
	for _, steer := range []string{steerAgentDeny, steerAskUserQuestionDeny, steerAgentNonForkDeny, steerWebsterForkDeny} {
		if strings.ContainsAny(steer, steerTextForbiddenChars) {
			panic(fmt.Sprintf("claudeengine: steer text contains a forbidden character (one of %q), which would break the JSON payload or the echo hook command: %q", steerTextForbiddenChars, steer))
		}
	}
}
