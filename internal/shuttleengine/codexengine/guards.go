// guards.go renders the PreToolUse denies a Codex launch carries: the question-tool deny and the shell guard table.
// The same denies also render as the deny notice that opens the Codex developer instructions.
// Each installed row is one Bash entry that prints Codex's deny decision when the payload matches all its patterns.

package codexengine

import (
	"fmt"
	"strings"

	"github.com/Knatte18/loomyard/internal/shell"
	"github.com/Knatte18/loomyard/internal/shellguard"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// foregroundPhrase is Codex's phrase for running a long command in the foreground.
const foregroundPhrase = "with the shell command timeout set to 3600000 milliseconds"

// toolMatcherBash is the tool name that Codex's PreToolUse payload carries for a shell command.
// Every guard entry matches on this name.
const toolMatcherBash = "Bash"

// guardInputs are the launch inputs that decide which shell guard rows a launch installs and announces.
type guardInputs struct {
	// denyPython follows the `DenyPython` field of the shuttle config; it decides only whether the python rows are announced.
	denyPython bool
	// denyPublish follows the `DenyPublish` field of the launch spec.
	denyPublish bool
	// allowRawTmux follows the `AllowRawTmux` field of the launch spec; it leaves out the tmux and script row.
	allowRawTmux bool
}

// pythonAllowedCheck returns the command the python row's hook runs on a matching call.
// It asks the lym binary at lymPath whether the shuttle config the launch read allows Python now.
// It runs from the hook's working directory, the session's own.
// The verb finds that config from the session's worktree.
// When no hub holds the worktree, it reads the standalone target's state directory.
// The verb prints nothing, so the hook's output stays the deny alone.
// A missing or failing lym exits non-zero, so the hook keeps denying.
func pythonAllowedCheck(lymPath string) string {
	return shell.Posix().Quote(lymPath) + " shuttle " + shuttleengine.PythonAllowedHookVerb + " >/dev/null 2>&1"
}

// switchInstalled maps each shellguard.Switch to whether a launch with the given inputs installs the rows hanging on it.
// The python rows are installed in every launch, since their hook asks the hub on each matching call.
var switchInstalled = map[shellguard.Switch]func(guardInputs) bool{
	shellguard.SwitchNone:        func(guardInputs) bool { return true },
	shellguard.SwitchDenyPython:  func(guardInputs) bool { return true },
	shellguard.SwitchDenyPublish: func(in guardInputs) bool { return in.denyPublish },
	shellguard.SwitchDenyRawTmux: func(in guardInputs) bool { return !in.allowRawTmux },
}

// switchAnnounced maps each shellguard.Switch to whether a launch with the given inputs announces its rows.
// The announcement is the row's sentence in the developer instructions.
// The python rows are announced as the shuttle config stands at launch.
var switchAnnounced = map[shellguard.Switch]func(guardInputs) bool{
	shellguard.SwitchNone:        func(guardInputs) bool { return true },
	shellguard.SwitchDenyPython:  func(in guardInputs) bool { return in.denyPython },
	shellguard.SwitchDenyPublish: func(in guardInputs) bool { return in.denyPublish },
	shellguard.SwitchDenyRawTmux: func(in guardInputs) bool { return !in.allowRawTmux },
}

// switchLiveCheck maps a shellguard.Switch whose hook decides on each matching call to the command it asks.
// The command is rendered over the path of the lym binary that runs it.
// The hook denies a matching call unless that command exits 0.
var switchLiveCheck = map[shellguard.Switch]func(lymPath string) string{
	shellguard.SwitchDenyPython: pythonAllowedCheck,
}

// installedGuards returns the shell guard rows a launch with in installs over the hub's refuse list, in table order.
// It panics on a row hanging on a switch the engine does not know.
// Such a row would otherwise stay uninstalled or unannounced unseen.
func installedGuards(refuseRaw []string, in guardInputs) []shellguard.Row {
	var rows []shellguard.Row
	for _, row := range shellguard.Rows(foregroundPhrase, refuseRaw) {
		installed, known := switchInstalled[row.Switch]
		_, announcedKnown := switchAnnounced[row.Switch]
		if !known || !announcedKnown {
			panic(fmt.Sprintf("codexengine: shellguard row %q hangs on switch %q, which the engine does not know, so the row would stay uninstalled or unannounced", row.Name, row.Switch))
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
// A row whose switch has a live check prints the deny on a match only when that check fails.
// The check runs through the lym binary at lymPath.
// Each pattern is shell-quoted, since a pattern may hold a single quote.
func guardCommand(row shellguard.Row, lymPath string) string {
	posix := shell.Posix()
	greps := make([]string, len(row.Patterns))
	for i, pattern := range row.Patterns {
		greps[i] = `{ printf '%s' "$in" | grep -Eq ` + posix.Quote(pattern) + `; }`
	}
	deny := "echo " + posix.Quote(denyDecision(row.SteerFor(engineName)))
	if check, live := switchLiveCheck[row.Switch]; live {
		deny = "! " + check(lymPath) + " && " + deny
	}
	return `in=$(cat); ` + strings.Join(greps, " && ") + " && " + deny + " || " + emptyHookOutput
}

// guardEntries returns one Bash PreToolUse entry per row, in order, over the lym binary at lymPath.
func guardEntries(rows []shellguard.Row, lymPath string) []string {
	entries := make([]string, len(rows))
	for i, row := range rows {
		entries[i] = hookEntry(toolMatcherBash, guardCommand(row, lymPath))
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
// the question-tool deny's when questionDeny is set, then each of rows' that a launch with in announces.
func denyNotice(questionDeny bool, rows []shellguard.Row, in guardInputs) string {
	var notices []string
	if questionDeny {
		notices = append(notices, noticeQuestionDeny)
	}
	for _, row := range rows {
		if switchAnnounced[row.Switch](in) {
			notices = append(notices, row.NoticeFor(engineName))
		}
	}
	return strings.Join(notices, " ")
}
