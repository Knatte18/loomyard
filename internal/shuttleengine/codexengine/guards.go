// guards.go renders the shell guard table as the PreToolUse hooks a Codex launch carries and as the deny notice its prompt file opens with.
// Each installed row is one Bash entry whose command greps the payload for the row's patterns and prints Codex's deny decision when all of them match.

package codexengine

import (
	"fmt"
	"strings"

	"github.com/Knatte18/loomyard/internal/shell"
	"github.com/Knatte18/loomyard/internal/shellguard"
)

// foregroundPhrase is Codex's phrase for running a long command in the foreground, which the gate rule of the raw go and refuse-raw rows carries.
const foregroundPhrase = "with the shell command timeout set to 3600000 milliseconds"

// toolMatcherBash is the tool name Codex's PreToolUse payload carries for a shell command, which every guard entry matches.
const toolMatcherBash = "Bash"

// guardInputs are the launch inputs that decide which shell guard rows a launch installs.
type guardInputs struct {
	// denyPython follows the shuttle config's python switch.
	denyPython bool
	// denyPublish follows the launch spec's publish guard.
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

// denyDecision returns the PreToolUse output that blocks a command with steer as the reason, the shape the spike saw Codex honour.
// steer holds no quote or backslash, which shellguard checks at init.
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

// denyNotice returns the sentences announcing each of rows to a Codex session, space-separated, or "" for no rows.
func denyNotice(rows []shellguard.Row) string {
	notices := make([]string, len(rows))
	for i, row := range rows {
		notices[i] = row.NoticeFor(engineName)
	}
	return strings.Join(notices, " ")
}
