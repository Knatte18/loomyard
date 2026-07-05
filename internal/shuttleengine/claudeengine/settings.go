// settings.go composes the Claude Code settings.json document Prepare
// writes for each run: a Stop hook that appends every turn-end event to the
// run's events.jsonl (the only channel ParseEvents reads), and the
// PreToolUse guardrails that keep a run's work visible in its own pane —
// denying the in-process Agent tool always, and denying AskUserQuestion in
// autonomous runs, where there is no operator present to answer it.

package claudeengine

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// steerAgentDeny is the PreToolUse(Agent) deny reason: it always redirects
// the model back into this pane, since shuttle's whole design point is that
// every agent is a separate, visible psmux pane — never Claude Code's
// in-process Agent tool (mux-hooks-exploration.md §B).
const steerAgentDeny = "do the work in this session; nested agents are not available here — all work must stay visible in this pane"

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
// when neither guardrail is configured on, so a run with both denies off
// emits no PreToolUse key at all.
type settingsHooks struct {
	Stop       []hookEntry `json:"Stop"`
	PreToolUse []hookEntry `json:"PreToolUse,omitempty"`
}

// settingsDoc is the Claude Code settings.json document Prepare writes.
type settingsDoc struct {
	Hooks settingsHooks `json:"hooks"`
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
// silently misinterpreted as an escape sequence.
//
// The Agent-tool deny is included whenever cfg.ClaudeDenyAgentTool is set,
// in both interactive and autonomous runs — Claude Code's in-process Agent
// tool must never be allowed to run work invisibly, regardless of who is
// watching the pane. The AskUserQuestion deny is included only when
// cfg.ClaudeDenyAskUserQuestion is set AND the run is autonomous
// (!interactive): an interactive run has an operator who can actually
// answer the dialog, so the deny would only get in the way there (Shared
// Decision "Interactive bool encodes the discussion's Autonomous default
// true").
func buildSettings(eventsPathPosix string, interactive bool, cfg shuttleengine.Config) ([]byte, error) {
	stopCmd := fmt.Sprintf("cat >> '%s' && printf '\\n' >> '%s'", eventsPathPosix, eventsPathPosix)

	doc := settingsDoc{
		Hooks: settingsHooks{
			Stop: []hookEntry{
				{Hooks: []hookCommand{{Type: "command", Command: stopCmd}}},
			},
		},
	}

	if cfg.ClaudeDenyAgentTool {
		doc.Hooks.PreToolUse = append(doc.Hooks.PreToolUse, hookEntry{
			Matcher: "Agent",
			Hooks:   []hookCommand{{Type: "command", Command: "echo '" + denyJSON(steerAgentDeny) + "'"}},
		})
	}
	if cfg.ClaudeDenyAskUserQuestion && !interactive {
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

// mustContainNoSingleQuote is a compile-time-adjacent guard: both steer
// constants ride inside a single-quoted echo argument under git-bash, so
// either one containing a `'` would corrupt the hook command. Panicking at
// package init turns a future edit that reintroduces a quote into an
// immediate, unmissable failure rather than a subtle hook-command bug
// discovered only via a live smoke test.
func init() {
	for _, steer := range []string{steerAgentDeny, steerAskUserQuestionDeny} {
		if strings.Contains(steer, "'") {
			panic(fmt.Sprintf("claudeengine: steer text contains a single quote, which would break the echo hook command: %q", steer))
		}
	}
}
