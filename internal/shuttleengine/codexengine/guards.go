// guards.go renders the PreToolUse denies a Codex launch carries: the question-tool deny and the shell guard table.
// The same denies also render as the deny notice that opens the Codex developer instructions.
// Each installed row is one Bash entry that prints Codex's deny decision when the payload matches all its patterns.

package codexengine

import (
	"fmt"
	"strings"

	"github.com/Knatte18/loomyard/internal/shell"
	"github.com/Knatte18/loomyard/internal/shellguard"
)

// foregroundPhrase is Codex's phrase for running a long command in the foreground.
const foregroundPhrase = "with the shell command timeout set to 3600000 milliseconds"

// toolMatcherBash is the tool name that Codex's PreToolUse payload carries for a shell command.
// Every guard entry matches on this name.
const toolMatcherBash = "Bash"

// guardInputs are the launch inputs that decide which shell guard rows a launch installs.
type guardInputs struct {
	// denyPython follows the `DenyPython` field of the shuttle config.
	denyPython bool
	// denyPublish follows the `DenyPublish` field of the launch spec.
	denyPublish bool
}

// switchInstalled maps each shellguard.Switch to whether a launch with the given inputs installs the rows hanging on it.
var switchInstalled = map[shellguard.Switch]func(guardInputs) bool{
	shellguard.SwitchNone:        func(guardInputs) bool { return true },
	shellguard.SwitchDenyPython:  func(in guardInputs) bool { return in.denyPython },
	shellguard.SwitchDenyPublish: func(in guardInputs) bool { return in.denyPublish },
}

// installedGuards returns the shell guard rows a launch with in installs over the hub's refuse list, in table order.
// It panics on a row hanging on a switch the engine does not know, which would otherwise stay uninstalled unseen.
func installedGuards(refuseRaw []string, in guardInputs) []shellguard.Row {
	var rows []shellguard.Row
	for _, row := range shellguard.Rows(foregroundPhrase, refuseRaw) {
		installed, known := switchInstalled[row.Switch]
		if !known {
			panic(fmt.Sprintf("codexengine: shellguard row %q hangs on switch %q, which the engine does not know, so the row would stay uninstalled", row.Name, row.Switch))
		}
		if installed(in) {
			rows = append(rows, row)
		}
	}
	return rows
}

// denyDecision returns the PreToolUse output that blocks a command with steer as the reason.
// The output has the shape Codex honours.
// steer must hold no quote or backslash, which shellguard checks at init.
func denyDecision(steer string) string {
	return `{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny","permissionDecisionReason":"` + steer + `"}}`
}

// guardCommand returns the hook command of row.
// It reads the payload once and greps it for every pattern.
// It prints the row's Codex deny when all match and the empty hook output otherwise, exiting 0 either way.
// Each pattern is shell-quoted, since a pattern may hold a single quote.
func guardCommand(row shellguard.Row) string {
	posix := shell.Posix()
	greps := make([]string, len(row.Patterns))
	for i, pattern := range row.Patterns {
		greps[i] = `{ printf '%s' "$in" | grep -Eq ` + posix.Quote(pattern) + `; }`
	}
	return `in=$(cat); ` + strings.Join(greps, " && ") + " && echo " + posix.Quote(denyDecision(row.SteerFor(engineName))) + " || " + emptyHookOutput
}

// guardEntries returns one Bash PreToolUse entry per row, in order.
func guardEntries(rows []shellguard.Row) []string {
	entries := make([]string, len(rows))
	for i, row := range rows {
		entries[i] = hookEntry(toolMatcherBash, guardCommand(row))
	}
	return entries
}

// questionToolPattern matches a payload whose tool name ends in request_user_input or request_user_input_async,
// so a name Codex prefixes with its namespace, as it prefixes spawn_agent, still matches.
const questionToolPattern = `"tool_name":[[:space:]]*"[^"]*request_user_input(_async)?"`

// steerQuestionDeny is the reason the question-tool deny gives the model: an autonomous run has no operator to answer.
// It holds no quote or backslash, since it rides a shell-quoted JSON constant.
const steerQuestionDeny = "you cannot ask the operator a question here. If you are blocked or need operator input, state the question as your final message and end your turn WITHOUT writing the result file. The run then waits for the answer, which arrives as your next turn."

// noticeQuestionDeny announces the question-tool deny in the developer instructions.
const noticeQuestionDeny = "request_user_input is unavailable: when blocked or needing operator input, state the question as your final message and end your turn without writing the result file; the run then waits for the answer, which arrives as your next turn."

// questionDenyCommand returns the hook command of the question-tool deny.
// It greps the payload for a question tool's name.
// It prints Codex's deny when the name matches and the empty hook output otherwise, exiting 0 either way.
func questionDenyCommand() string {
	posix := shell.Posix()
	return `grep -Eq ` + posix.Quote(questionToolPattern) + ` && echo ` + posix.Quote(denyDecision(steerQuestionDeny)) + ` || ` + emptyHookOutput
}

// denyNotice returns the sentences announcing a launch's denies to a Codex session, space-separated, or "" for none:
// the question-tool deny's when questionDeny is set, then each of rows'.
func denyNotice(questionDeny bool, rows []shellguard.Row) string {
	var notices []string
	if questionDeny {
		notices = append(notices, noticeQuestionDeny)
	}
	for _, row := range rows {
		notices = append(notices, row.NoticeFor(engineName))
	}
	return strings.Join(notices, " ")
}
