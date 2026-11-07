// settings.go composes the Claude Code settings.json document that Prepare writes for each run.
// It also composes the creel-only document that OpenLaunch writes for a session opened in a hub.
// Each recording hook appends to the run's events.jsonl, the only channel ParseEvents reads.
// Each recording hook writes a stamp line with the hook-side time before its payload, which ParseEvents skips.
// A document holds only what differs per session or role: hooks naming the session's own paths, its denies and its scope.
// A setting every session of a worktree shares comes from the worktree's `.claude/settings.local.json` instead.
// Examples are `attribution` and `promptSuggestionEnabled`.
// No hook denies the Agent tool, since a subagent or fork runs inside the session and shows in its own pane.

package claudeengine

import (
	"encoding/json"
	"fmt"
	"path"
	"strings"

	"github.com/Knatte18/loomyard/internal/shellguard"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// steerAskUserQuestionDeny denies AskUserQuestion in autonomous runs, where no operator is present to answer.
const steerAskUserQuestionDeny = "you cannot open an interactive dialog here. If you are blocked or need operator input, state the question as your final message and end your turn WITHOUT writing the result file. The run then waits for the answer, which arrives as your next turn."

// steerWebsterForkDeny guards against fork-context deadlock:
// a fork inherits Master's await-batch loop and polling it would livelock the run.
// It refuses `lym webster` commands inside forks (detected by top-level agent_id in the payload).
// Its text must contain no single/double quote or backslash (checked at init).
const steerWebsterForkDeny = "lym webster verbs belong to the Master session, never a fork. You are an implementer fork: do your batch work and write your report, and do NOT run any lym webster command (not await-batch, not anything) — polling for the report you must write only deadlocks the run. This call is refused."

// noticeAskUserQuestionDeny announces the AskUserQuestion deny in an autonomous run.
const noticeAskUserQuestionDeny = "AskUserQuestion is unavailable: when blocked or needing operator input, state the question as your final message and end your turn without writing the result file; the run then waits for the answer, which arrives as your next turn."

// hookCommand is one Claude Code hook invocation, run under git-bash on Windows.
// AsyncRewake runs the command in the background and wakes the idle session when it exits 2.
// Timeout bounds it in seconds.
// RewakeMessage and RewakeSummary label that wake for the model and the operator's notification row.
// Every one of them is omitted when unset.
type hookCommand struct {
	Type          string `json:"type"`
	Command       string `json:"command"`
	AsyncRewake   bool   `json:"asyncRewake,omitempty"`
	Timeout       int    `json:"timeout,omitempty"`
	RewakeMessage string `json:"rewakeMessage,omitempty"`
	RewakeSummary string `json:"rewakeSummary,omitempty"`
}

// hookEntry is one matcher/hooks pair in a settings.json hook event list.
// Matcher is omitted for events that carry no tool-name matcher (like Stop).
type hookEntry struct {
	Matcher string        `json:"matcher,omitempty"`
	Hooks   []hookCommand `json:"hooks"`
}

// settingsHooks is the "hooks" object of a Claude Code settings.json document.
// Every event but PreToolUse and PostToolUse is a recording hook installed for every session.
// PostToolUse, PreCompact and PostCompact touch the activity stamp.
// PostToolUse also carries the batching hook while it is on and the creel mail hook in a hub.
// In a hub, Stop carries two entries: the recording merged with creel's turn-end delivery, and creel's idle waiter;
// in no hub it carries the recording alone.
// PreToolUse is omitted when an autonomous run has every deny off;
// interactive runs always carry at least the AskUserQuestion marker.
type settingsHooks struct {
	Stop              []hookEntry `json:"Stop"`
	PreToolUse        []hookEntry `json:"PreToolUse,omitempty"`
	UserPromptSubmit  []hookEntry `json:"UserPromptSubmit"`
	StopFailure       []hookEntry `json:"StopFailure"`
	Notification      []hookEntry `json:"Notification"`
	SessionEnd        []hookEntry `json:"SessionEnd"`
	SessionStart      []hookEntry `json:"SessionStart"`
	PermissionRequest []hookEntry `json:"PermissionRequest"`
	PostToolUse       []hookEntry `json:"PostToolUse"`
	PreCompact        []hookEntry `json:"PreCompact"`
	PostCompact       []hookEntry `json:"PostCompact"`
}

// compactionMatcher is the SessionStart matcher for a session restarting after a compaction.
const compactionMatcher = "compact"

// nonCompactionMatcher is the SessionStart matcher for every source but compaction.
const nonCompactionMatcher = "startup|resume|clear"

// SessionStartContext renders the JSON that a SessionStart hook prints to add text to the session's context.
func SessionStartContext(text string) ([]byte, error) {
	return hookContext(hookEventSessionStart, text)
}

// hookContext renders the additional-context JSON a hook of the named event prints to add text to the session's context.
func hookContext(hookEventName, text string) ([]byte, error) {
	type specificOutput struct {
		HookEventName     string `json:"hookEventName"`
		AdditionalContext string `json:"additionalContext"`
	}
	data, err := json.Marshal(struct {
		HookSpecificOutput specificOutput `json:"hookSpecificOutput"`
	}{specificOutput{HookEventName: hookEventName, AdditionalContext: text}})
	if err != nil {
		return nil, fmt.Errorf("marshal %s context: %w", hookEventName, err)
	}
	return data, nil
}

// creelMailCommand is the command the PostToolUse hook runs when the strand's mail flag stands.
// It is creel's claim verb, given this engine's registry name.
// It renders the claimed mail as this engine's hook output.
const creelMailCommand = "lym creel hook-read claude"

// creelMailHookCommand returns the PostToolUse hook command that hands the strand its creel mail at each tool boundary.
// It forms the flag's file name from the strand's name by shell parameter expansion alone, turning every ":" into ".".
// It exits 0 at once unless that flag stands in the directory at quotedFlagDir,
// so a tool call with no mail starts no process.
// An empty or unset strand name names no regular file and exits the same way.
// When the flag stands, it changes to the session's project directory,
// so a session that moved out of the hub still reaches it.
// It then runs creel's claim verb, whose output is the hook's.
func creelMailHookCommand(quotedFlagDir string) string {
	return shuttleengine.StrandFlagNameShell() + `; [ -f ` + quotedFlagDir + `/"$f$n" ] || exit 0; cd "${CLAUDE_PROJECT_DIR:-.}" 2>/dev/null; ` + creelMailCommand
}

// creelStopHookCommand returns the Stop hook command that merges the turn-end recording with creel's turn-end delivery,
// so the record is written only when no mail was handed to the session.
// Claude Code runs an event's hooks in parallel,
// so a separate creel hook could not keep the recording from logging a turn end the mail then continues.
// The command reads the hook input once.
// When the strand's flag stands in the directory at quotedFlagDir, it pipes that input to creel's claim verb.
// That runs from the session's project directory, and its output is the hook's.
// An exit 0 of that step means mail was handed to the session, which continues the turn,
// and the command then exits 0 having appended nothing to the events file at quotedEventsPath.
// Otherwise, or when the flag is absent, the command appends the Stop stamp and the input as the recording command does.
// It exits with that append's status, and what the delivery step printed stays the hook's output.
// An echo printed with exit 1 relies on that output to reach the operator.
func creelStopHookCommand(quotedFlagDir, quotedEventsPath string) string {
	record := `printf '%s' "$in" | { ` + recordingCommand(hookEventStop, quotedEventsPath) + `; }`
	return creelStopDelivery(quotedFlagDir) + `; ` + record
}

// creelOpenStopHookCommand returns the Stop hook command of a session with no events file:
// creel's turn-end delivery alone.
// It reads the hook input once.
// When the strand's flag stands in the directory at quotedFlagDir, it pipes the input to creel's mail command.
// The mail command runs from the session's project directory, and its output is the hook's.
// It always exits 0;
// whatever the delivery printed, an echo with exit 1 included, reaches Claude Code as the hook's output.
func creelOpenStopHookCommand(quotedFlagDir string) string {
	return creelStopDelivery(quotedFlagDir) + `; true`
}

// creelStopDelivery returns the opening of a creel Stop hook command:
// it reads the hook input into $in and forms the strand's flag name.
// When that flag stands in the directory at quotedFlagDir, it pipes the input to creel's mail command.
// The command runs from the session's project directory, and the shell exits 0 when that hands out mail.
func creelStopDelivery(quotedFlagDir string) string {
	deliver := `if [ -f ` + quotedFlagDir + `/"$f$n" ]; then cd "${CLAUDE_PROJECT_DIR:-.}" 2>/dev/null; printf '%s' "$in" | ` + creelMailCommand + ` && exit 0; fi`
	return `in=$(cat); ` + shuttleengine.StrandFlagNameShell() + `; ` + deliver
}

// creelWaitCommand is the command the idle waiter hook runs.
// It runs creel's waiter verb with this engine's registry name, so the wake renders as this engine's.
const creelWaitCommand = "lym creel hook-wait claude"

// creelWaitTimeout is the idle waiter hook's timeout in seconds, a day.
// It is well under the 2^31-1 ms ceiling of the timers Claude Code arms, and long enough to outlive any idle spell.
const creelWaitTimeout = 86400

// creelRewakeMessage is the label the idle waiter's wake carries for the model.
// It replaces Claude Code's default label, which names the hook's command.
// Claude Code's hook schema marks the rewake message and summary keys `@internal`.
// The message opens with `[creel]` so the wake's first line matches the creel trust sentence.
const creelRewakeMessage = "[creel] Mail that arrived while you were idle:"

// creelRewakeSummary is the row the operator's terminal draws for the idle waiter's wake.
// Claude Code takes a per-wake summary only from a hook of an Anthropic-marketplace plugin,
// so the summary is fixed for the session.
const creelRewakeSummary = "creel mail"

// creelWaitHookCommand returns the Stop hook command of the idle waiter:
// it changes to the session's project directory and runs creel's waiter verb.
// The change lets a session that moved out of the hub still reach the waiter.
// It runs whether or not the strand's mail flag stands, since the waiter wakes an idle session for later mail.
func creelWaitHookCommand() string {
	return `cd "${CLAUDE_PROJECT_DIR:-.}" 2>/dev/null; ` + creelWaitCommand
}

// creelWaitHookEntry returns the Stop hook entry of the idle waiter:
// it runs the waiter in the background for up to the waiter's timeout.
// When the waiter exits 2, it wakes the idle session under creel's labels.
func creelWaitHookEntry() hookEntry {
	return hookEntry{Hooks: []hookCommand{{Type: "command", Command: creelWaitHookCommand(), AsyncRewake: true, Timeout: creelWaitTimeout, RewakeMessage: creelRewakeMessage, RewakeSummary: creelRewakeSummary}}}
}

// openSettingsHooks is the "hooks" object of an opened session's settings document: creel's hook delivery and nothing else.
type openSettingsHooks struct {
	Stop        []hookEntry `json:"Stop"`
	PostToolUse []hookEntry `json:"PostToolUse"`
}

// openSettingsDoc is the settings document written for a session opened in a hub.
// It carries hooks and the session's scope alone,
// so every other setting of the session stays the worktree's own.
type openSettingsDoc struct {
	Hooks openSettingsHooks `json:"hooks"`
	sessionScope
}

// buildOpenSettings marshals an opened session's settings document over the creel flag directory creelFlagDirPosix.
// The path is a git-bash POSIX path, and the document holds the PostToolUse creel mail hook and the scope.
// On Stop, it runs creel's turn-end delivery followed by the idle waiter.
// It records nothing, since an opened session has no events file.
func buildOpenSettings(creelFlagDirPosix string, scope sessionScope) ([]byte, error) {
	quotedFlagDir := shQuote(creelFlagDirPosix)
	doc := openSettingsDoc{Hooks: openSettingsHooks{
		Stop: []hookEntry{
			{Hooks: []hookCommand{{Type: "command", Command: creelOpenStopHookCommand(quotedFlagDir)}}},
			creelWaitHookEntry(),
		},
		PostToolUse: []hookEntry{{Hooks: []hookCommand{{Type: "command", Command: creelMailHookCommand(quotedFlagDir)}}}},
	}, sessionScope: scope}
	data, err := json.Marshal(doc)
	if err != nil {
		return nil, fmt.Errorf("marshal open settings: %w", err)
	}
	return data, nil
}

// settingsDoc is the Claude Code settings.json document written for a session:
// its hooks, with the session's scope following as the last keys.
type settingsDoc struct {
	Hooks settingsHooks `json:"hooks"`
	sessionScope
}

// shQuote wraps s in POSIX shell single quotes, escaping embedded quotes with the standard sh idiom.
// It keeps a path that contains an apostrophe from breaking out of the quoted argument.
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
type denyInputs struct {
	interactive   bool
	cfg           shuttleengine.Config
	claude        Config
	forkSubagents bool
	denyPublish   bool
}

// standingDeny is one PreToolUse deny lym installs, with the system-prompt sentence announcing it.
// The hook entries and the announcement are rendered from one list of denies, so they cannot drift.
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

// claudeDenies returns the denies keyed on Claude's own tools and payload fields, in the order their PreToolUse entries appear.
func claudeDenies() []standingDeny {
	return []standingDeny{
		{
			// Denies `lym webster` inside a fork, detected by a top-level agent_id in the payload.
			// The command ends with `; true`, so it exits 0 and every other call is allowed.
			matcher:   "Bash",
			command:   "in=$(cat); { printf '%s' \"$in\" | grep -q '\"agent_id\"'; } && { printf '%s' \"$in\" | grep -Eq 'lym[[:space:]]+webster'; } && echo '" + denyJSON(steerWebsterForkDeny) + "'; true",
			steer:     steerWebsterForkDeny,
			installed: func(in denyInputs) bool { return in.forkSubagents },
		},
		{
			matcher:   "AskUserQuestion",
			command:   "echo '" + denyJSON(steerAskUserQuestionDeny) + "'",
			steer:     steerAskUserQuestionDeny,
			notice:    noticeAskUserQuestionDeny,
			installed: func(in denyInputs) bool { return !in.interactive && in.claude.DenyAskUserQuestion },
		},
	}
}

// standingDeniesFor lists lym's denies in the order their PreToolUse entries appear:
// the Claude denies, then every shell guard row, the refuse-raw row among them when refuseRaw is non-empty.
// An interactive run's AskUserQuestion hook only records, never denies, so it stays outside the list.
func standingDeniesFor(refuseRaw []string) []standingDeny {
	return append(claudeDenies(), shellGuardDenies(refuseRaw)...)
}

// bashForegroundPhrase is Claude's phrase for running a long command in the foreground.
// The `raw-go` row's steer and notice carry it.
var bashForegroundPhrase = "with the Bash tool timeout set to " + bashMaxTimeoutMS

// shellSwitchInstalled maps each shell guard switch to whether a run with the given inputs installs its rows.
var shellSwitchInstalled = map[shellguard.Switch]func(denyInputs) bool{
	shellguard.SwitchNone:        func(denyInputs) bool { return true },
	shellguard.SwitchDenyPython:  func(in denyInputs) bool { return in.cfg.DenyPython },
	shellguard.SwitchDenyPublish: func(in denyInputs) bool { return in.denyPublish },
}

// shellGuardInstalled reports whether a run with the given inputs installs the shell guard rows hanging on sw;
// a switch it does not know is not installed.
func shellGuardInstalled(sw shellguard.Switch, in denyInputs) bool {
	installed, known := shellSwitchInstalled[sw]
	return known && installed(in)
}

// shellGuardDenies renders every shellguard row for the told refuse list as a standing deny, in table order.
func shellGuardDenies(refuseRaw []string) []standingDeny {
	var denies []standingDeny
	for _, row := range shellguard.Rows(bashForegroundPhrase, refuseRaw) {
		denies = append(denies, renderShellGuard(row))
	}
	return denies
}

// renderShellGuard wraps a shell guard row as a PreToolUse(Bash) hook entry.
// A one-pattern row greps the payload once;
// a row of several patterns greps the payload once per pattern and denies only when all match.
// Each pattern is shell-quoted, since a pattern may hold a single quote.
// The command ends with `; true`, so a non-match exits 0 and allows the call.
func renderShellGuard(row shellguard.Row) standingDeny {
	var command string
	if len(row.Patterns) == 1 {
		command = "grep -Eq " + shQuote(row.Patterns[0]) + " && echo '" + denyJSON(row.Steer) + "'; true"
	} else {
		greps := make([]string, len(row.Patterns))
		for i, pattern := range row.Patterns {
			greps[i] = "{ printf '%s' \"$in\" | grep -Eq " + shQuote(pattern) + "; }"
		}
		command = "in=$(cat); " + strings.Join(greps, " && ") + " && echo '" + denyJSON(row.Steer) + "'; true"
	}
	return standingDeny{
		matcher:   "Bash",
		command:   command,
		steer:     row.Steer,
		notice:    row.Notice,
		installed: func(in denyInputs) bool { return shellGuardInstalled(row.Switch, in) },
	}
}

// buildDenyNotice returns the one-line system-prompt notice that announces each deny the given inputs install.
// The notice is "" when they install none.
func buildDenyNotice(interactive bool, cfg shuttleengine.Config, claude Config, forkSubagents, denyPublish bool) string {
	in := denyInputs{interactive: interactive, cfg: cfg, claude: claude, forkSubagents: forkSubagents, denyPublish: denyPublish}
	var sentences []string
	for _, deny := range standingDeniesFor(cfg.RefuseRaw) {
		if deny.notice != "" && deny.installed(in) {
			sentences = append(sentences, deny.notice)
		}
	}
	return strings.Join(sentences, " ")
}

// recordingCommand returns the hook command that appends one hook event to the events file at quotedEventsPath:
// a stamp line naming hookEventName with the hook-side time, then the payload on standard input followed by a newline.
// The two are joined by `;` so a failed stamp never blocks the payload,
// and the command's exit status is the payload append's.
// A failed `date` leaves the stamp's time empty, which the parser reads as no time.
func recordingCommand(hookEventName, quotedEventsPath string) string {
	stamp := `printf '{"` + stampHookKey + `":"` + hookEventName + `","` + stampAtKey + `":"%s"}\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" >> ` + quotedEventsPath
	payload := "cat >> " + quotedEventsPath + " && printf '\\n' >> " + quotedEventsPath
	return stamp + "; " + payload
}

// activityStampHook returns the hook entry that touches the activity stamp beside the events file at eventsPathPosix,
// and the entry always exits 0, so a failed touch never blocks the session.
func activityStampHook(eventsPathPosix string) []hookEntry {
	stampPath := path.Join(path.Dir(eventsPathPosix), shuttleengine.ActivityStampFileName)
	return []hookEntry{{Hooks: []hookCommand{{Type: "command", Command: "touch " + shQuote(stampPath) + "; true"}}}}
}

// buildSettings marshals settings.json:
// a Stop hook and the other recording hooks append their events to eventsPathPosix.
// That path must be a git-bash POSIX path, and it is shell-quoted, so an apostrophe in it cannot break the command.
// The PreToolUse guardrails follow from interactive, cfg, claude and forkSubagents.
// denyPublish adds a deny of git push and of gh calls beyond reads, comments, and title and body edits.
// Every SessionStart source is recorded exactly once.
// A non-empty contextAfterCompaction splits the SessionStart hook in two:
// one entry records every source but compaction,
// and the compaction entry records the event and then runs that command verbatim,
// so its standard output reaches the session's context.
// An empty contextAfterCompaction gives a single recording entry with no matcher.
// The recordings print nothing and exit 0, so the PermissionRequest recording answers no request,
// and the SessionStart recording adds nothing to the session's context.
// claude's batching nudge, when on, adds a PostToolUse entry matched to the tools a breach is made of,
// and it names the state file beside the events file.
// A non-empty creelFlagDirPosix is a git-bash POSIX path like eventsPathPosix.
// It adds the creel mail hook to PostToolUse,
// and the hook tests the strand's flag in that directory.
// It also replaces the Stop recording with the command that merges it with creel's turn-end delivery.
// Creel's idle waiter is added as a second Stop entry.
// An empty creelFlagDirPosix adds none of them and keeps the plain Stop recording.
// scope is the session's scope, carried as the document's last keys; the zero scope adds none.
func buildSettings(eventsPathPosix string, interactive bool, cfg shuttleengine.Config, claude Config, forkSubagents, denyPublish bool, contextAfterCompaction, creelFlagDirPosix string, scope sessionScope) ([]byte, error) {
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
			Stop:              recordingHook(hookEventStop, false),
			UserPromptSubmit:  recordingHook(hookEventUserPromptSubmit, true),
			StopFailure:       recordingHook(hookEventStopFailure, true),
			Notification:      recordingHook(hookEventNotification, true),
			SessionEnd:        recordingHook(hookEventSessionEnd, true),
			PermissionRequest: recordingHook(hookEventPermissionRequest, true),
			PostToolUse:       activityStampHook(eventsPathPosix),
			PreCompact:        activityStampHook(eventsPathPosix),
			PostCompact:       activityStampHook(eventsPathPosix),
		},
		sessionScope: scope,
	}

	if contextAfterCompaction == "" {
		doc.Hooks.SessionStart = recordingHook(hookEventSessionStart, true)
	} else {
		recording := recordingHook(hookEventSessionStart, true)[0].Hooks[0]
		doc.Hooks.SessionStart = []hookEntry{
			{Matcher: nonCompactionMatcher, Hooks: []hookCommand{recording}},
			{Matcher: compactionMatcher, Hooks: []hookCommand{recording, {Type: "command", Command: contextAfterCompaction}}},
		}
	}

	in := denyInputs{interactive: interactive, cfg: cfg, claude: claude, forkSubagents: forkSubagents, denyPublish: denyPublish}
	for _, deny := range standingDeniesFor(cfg.RefuseRaw) {
		if deny.installed(in) {
			doc.Hooks.PreToolUse = append(doc.Hooks.PreToolUse, hookEntry{
				Matcher: deny.matcher,
				Hooks:   []hookCommand{{Type: "command", Command: deny.command}},
			})
		}
	}
	if interactive {
		// The hook records the live ask like a turn end and lets the tool call proceed.
		doc.Hooks.PreToolUse = append(doc.Hooks.PreToolUse, hookEntry{
			Matcher: "AskUserQuestion",
			Hooks:   recordingHook(hookEventPreToolUse, false)[0].Hooks,
		})
	}
	if claude.BatchingNudge.Enabled {
		statePath := path.Join(path.Dir(eventsPathPosix), batchingNudgeStateFileName)
		doc.Hooks.PostToolUse = append(doc.Hooks.PostToolUse, hookEntry{
			Matcher: batchingNudgeMatcher,
			Hooks:   []hookCommand{{Type: "command", Command: batchingNudgeHookCommand(shQuote(statePath))}},
		})
	}
	if creelFlagDirPosix != "" {
		quotedFlagDir := shQuote(creelFlagDirPosix)
		doc.Hooks.PostToolUse = append(doc.Hooks.PostToolUse, hookEntry{Hooks: []hookCommand{{Type: "command", Command: creelMailHookCommand(quotedFlagDir)}}})
		doc.Hooks.Stop = []hookEntry{
			{Hooks: []hookCommand{{Type: "command", Command: creelStopHookCommand(quotedFlagDir, quotedEventsPath)}}},
			creelWaitHookEntry(),
		}
	}

	data, err := json.Marshal(doc)
	if err != nil {
		return nil, fmt.Errorf("marshal claude settings: %w", err)
	}
	return data, nil
}

// steerTextForbiddenChars are characters that would corrupt steer constants in JSON or shell quoting.
// The package checks them at load.
const steerTextForbiddenChars = `'"\`

// noticeTextForbiddenChars are the characters a deny notice must not hold:
// single and double quotes, a backslash, and line breaks.
// A notice rides echo and JSON constants,
// and it shares one line of the append-system-prompt file with the sentences that follow it.
const noticeTextForbiddenChars = steerTextForbiddenChars + "\n\r"

// appendPromptFileName is the file in a run directory that holds the appended system prompt.
const appendPromptFileName = "append-system-prompt.txt"

// appendPromptText returns the appended system prompt:
// the deny notice and the creel trust sentence, space-separated, with the notice left out when it is empty.
// Each passage follows after a blank line, in order.
func appendPromptText(notice string, passages []string) string {
	text := shuttleengine.CreelTrustSentence
	if notice != "" {
		text = notice + " " + text
	}
	for _, passage := range passages {
		text += "\n\n" + passage
	}
	return text
}

// init panics if any steer constant contains a forbidden character (checked at package load).
func init() {
	if strings.ContainsAny(shuttleengine.CreelTrustSentence, "\n\r") {
		panic(fmt.Sprintf("claudeengine: the creel trust sentence holds a line break, which the append-system-prompt file's single line cannot carry: %q", shuttleengine.CreelTrustSentence))
	}
	sampleRefuseRaw := []string{"go test"}
	for _, row := range shellguard.Rows(bashForegroundPhrase, sampleRefuseRaw) {
		if _, known := shellSwitchInstalled[row.Switch]; !known {
			panic(fmt.Sprintf("claudeengine: shellguard row %q hangs on switch %q, which shellGuardInstalled does not know, so the row would stay uninstalled", row.Name, row.Switch))
		}
	}
	for _, deny := range standingDeniesFor(sampleRefuseRaw) {
		if strings.ContainsAny(deny.steer, steerTextForbiddenChars) {
			panic(fmt.Sprintf("claudeengine: steer text contains a forbidden character (one of %q), which would break the JSON payload or the echo hook command: %q", steerTextForbiddenChars, deny.steer))
		}
		if strings.ContainsAny(deny.notice, noticeTextForbiddenChars) {
			panic(fmt.Sprintf("claudeengine: notice text contains a forbidden character (one of %q), which would break the one-line launch argument: %q", noticeTextForbiddenChars, deny.notice))
		}
	}
}
