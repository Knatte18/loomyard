// settings.go composes the Claude Code settings.json document Prepare writes for each run:
// a Stop hook that appends every turn-end event to the run's events.jsonl (the only channel ParseEvents reads).
// A session in a hub also gets creel's hook delivery: the PostToolUse mail hook, a Stop hook that merges the turn-end recording with creel's turn-end delivery so a blocked Stop records no turn end, and the idle waiter hook that wakes an idle session.
// The UserPromptSubmit, StopFailure, Notification, SessionEnd, SessionStart and PermissionRequest hooks append their payloads to the same file for ParseSessionSignals;
// the SessionStart hook of a run whose spec sets a context-after-compaction command runs that command on the compaction source, so its output joins the session's context.
// Every recording hook writes a stamp line with the hook-side time before its payload, which ParseEvents skips.
// The PostToolUse, PreCompact and PostCompact hooks of every session touch the run's activity stamp beside the events file, so a tool call, a fork's included, and a compaction read as activity.
// The document also carries the PreToolUse guardrails that keep a run's work visible in its own pane —
// denying the in-process Agent tool (or, in a fork-mode run, letting fork subagents through it while still denying every other subagent type; a run with Spec.AllowAgentTool set installs no Agent deny at all),
// refusing `lym webster` verbs from inside a fork in a fork-mode run (the fork-context deadlock guard),
// denying AskUserQuestion in autonomous runs (where there is no operator present to answer it),
// and recording — never denying — a live AskUserQuestion call in interactive runs so the run loop holds it like any turn end without output.
// The Bash tool's default stdin is set by the env file Prepare writes (see command.go), not by a hook.
// The Bash denies render the rows of internal/shellguard's table, which installs the machine sleep, raw go and computed-path rm denies for every run, the refuse-raw deny when the run's config lists a prefix, and the python deny unless the run's config switches it off.
// buildDenyNotice, built from the same standingDeniesFor list so the two cannot drift, is the one-line system-prompt notice announcing each installed deny to the session.
// Every document also sets `promptSuggestionEnabled` to false for shuttle-launched sessions: a plain capture carries no styling,
// so a greyed suggestion in an empty input box would read as a draft and IdleSession would never pass.
// The styled read is what lets a hand-added session's suggestion read as empty.

package claudeengine

import (
	"encoding/json"
	"fmt"
	"path"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/lymdirs"
	"github.com/Knatte18/loomyard/internal/shellguard"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// steerAgentDeny redirects the model back into this pane; shuttle's design is that every agent runs in a separate visible tmux pane, not Claude Code's in-process Agent tool.
const steerAgentDeny = "do the work in this session; nested agents are not available here — all work must stay visible in this pane"

// steerAgentNonForkDeny is the PreToolUse(Agent) deny reason for fork-mode runs, where fork subagents are allowed and every other subagent type is refused.
const steerAgentNonForkDeny = "only fork subagents may be spawned here; other agents are unavailable — do the work in this session or in your forks"

// steerAskUserQuestionDeny denies AskUserQuestion in autonomous runs, where no operator is present to answer.
const steerAskUserQuestionDeny = "you cannot open an interactive dialog here. If you are blocked or need operator input, state the question as your final message and end your turn WITHOUT writing the result file. The run then waits for the answer, which arrives as your next turn."

// steerWebsterForkDeny guards against fork-context deadlock: a fork inherits Master's await-batch loop and polling it would livelock the run.
// It refuses `lym webster` commands inside forks (detected by top-level agent_id in the payload). Must contain no single/double quote or backslash (checked at init).
const steerWebsterForkDeny = "lym webster verbs belong to the Master session, never a fork. You are an implementer fork: do your batch work and write your report, and do NOT run any lym webster command (not await-batch, not anything) — polling for the report you must write only deadlocks the run. This call is refused."

// noticeAgentDeny announces the Agent deny in a non-fork run;
// it must hold for every session that receives it.
const noticeAgentDeny = "The Agent tool is unavailable in this session: do all exploration and work in this session, with no subagents."

// noticeAgentForkDeny announces the Agent deny in a fork run, where forks inherit this system prompt too.
const noticeAgentForkDeny = "The Agent tool accepts only fork subagents and refuses every other subagent type; a fork does its own work and never spawns further subagents."

// noticeAskUserQuestionDeny announces the AskUserQuestion deny in an autonomous run.
const noticeAskUserQuestionDeny = "AskUserQuestion is unavailable: when blocked or needing operator input, state the question as your final message and end your turn without writing the result file; the run then waits for the answer, which arrives as your next turn."

// hookCommand is one Claude Code hook invocation, run under git-bash on Windows.
// AsyncRewake runs the command in the background and wakes the idle session when it exits 2, and Timeout bounds it in seconds; both are omitted when unset.
type hookCommand struct {
	Type        string `json:"type"`
	Command     string `json:"command"`
	AsyncRewake bool   `json:"asyncRewake,omitempty"`
	Timeout     int    `json:"timeout,omitempty"`
}

// hookEntry is one matcher/hooks pair in a settings.json hook event list.
// Matcher is omitted for events that carry no tool-name matcher (like Stop).
type hookEntry struct {
	Matcher string        `json:"matcher,omitempty"`
	Hooks   []hookCommand `json:"hooks"`
}

// settingsHooks is the "hooks" object of a Claude Code settings.json document.
// Every event but PreToolUse and PostToolUse is a recording hook installed for every session.
// PostToolUse, PreCompact and PostCompact touch the activity stamp, and PostToolUse also carries the creel mail hook in a hub.
// In a hub, Stop carries two entries: the recording merged with creel's turn-end delivery, and creel's idle waiter; in no hub it carries the recording alone.
// PreToolUse is omitted when an autonomous run has every deny off; interactive runs always carry at least the AskUserQuestion marker.
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

// SessionStartContext renders the additional-context JSON a SessionStart hook prints to add text to the session's context.
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

// creelMailCommand is the command the PostToolUse hook runs when the strand's mail flag stands: creel's claim verb, told this engine's registry name so it renders the claimed mail as this engine's hook output.
const creelMailCommand = "lym creel hook-read claude"

// CreelFlagDir returns the directory creel keeps its per-strand mail flags in, for the hub whose board directory is boardDir.
func CreelFlagDir(boardDir string) string {
	return filepath.Join(boardDir, lymdirs.DotLymDirName, lymdirs.CreelDirName, lymdirs.CreelFlagDirName)
}

// creelMailHookCommand returns the PostToolUse hook command that hands the strand its creel mail at each tool boundary.
// It forms the flag's file name from the strand's name by shell parameter expansion alone, turning every ":" into ".", and exits 0 at once unless that flag stands in the directory at quotedFlagDir, so a tool call with no mail starts no process.
// An empty or unset strand name names no regular file and exits the same way.
// When the flag stands it changes to the session's project directory, so a session that moved out of the hub still reaches it, and runs creelMailCommand, whose output is the hook's.
func creelMailHookCommand(quotedFlagDir string) string {
	return strandFlagName() + `; [ -f ` + quotedFlagDir + `/"$f$n" ] || exit 0; cd "${CLAUDE_PROJECT_DIR:-.}" 2>/dev/null; ` + creelMailCommand
}

// strandFlagName returns the plain-sh fragment that forms the strand's flag file name in $f$n from the strand's name by parameter expansion alone, turning every ":" into ".".
// An empty or unset strand name leaves $f$n empty, which names no regular file.
func strandFlagName() string {
	return `n="${` + agentname.StrandNameEnv + `:-}"; f=; while :; do case "$n" in *:*) f="$f${n%%:*}."; n="${n#*:}";; *) break;; esac; done`
}

// creelStopHookCommand returns the Stop hook command that merges the turn-end recording with creel's turn-end delivery, so the record is written only when no block was.
// Claude Code runs an event's hooks in parallel, so a separate creel hook could not keep the recording from logging a turn end the block then cancels.
// The command reads the hook input once, and when the strand's flag stands in the directory at quotedFlagDir it pipes the input to creelMailCommand from the session's project directory, whose output is the hook's.
// An exit 0 of that step means a block was written, and the command then exits 0 having appended nothing to the events file at quotedEventsPath.
// Otherwise, or when the flag is absent, it appends the Stop stamp and the input as recordingCommand does and exits with that append's status.
func creelStopHookCommand(quotedFlagDir, quotedEventsPath string) string {
	deliver := `if [ -f ` + quotedFlagDir + `/"$f$n" ]; then cd "${CLAUDE_PROJECT_DIR:-.}" 2>/dev/null; printf '%s' "$in" | ` + creelMailCommand + ` && exit 0; fi`
	record := `printf '%s' "$in" | { ` + recordingCommand(hookEventStop, quotedEventsPath) + `; }`
	return `in=$(cat); ` + strandFlagName() + `; ` + deliver + `; ` + record
}

// creelWaitCommand is the command the idle waiter hook runs: creel's waiter verb, told this engine's registry name so it renders the wake as this engine's.
const creelWaitCommand = "lym creel hook-wait claude"

// creelWaitTimeout is the idle waiter hook's timeout in seconds, a day: well under the 2^31-1 ms ceiling of the timers Claude Code arms, and long enough to outlive any idle spell.
const creelWaitTimeout = 86400

// creelWaitHookCommand returns the Stop hook command of the idle waiter: it changes to the session's project directory, so a session that moved out of the hub still reaches it, and runs creelWaitCommand.
// It runs whether or not the strand's mail flag stands, since the waiter is what wakes an idle session for mail that arrives later.
func creelWaitHookCommand() string {
	return `cd "${CLAUDE_PROJECT_DIR:-.}" 2>/dev/null; ` + creelWaitCommand
}

// settingsDoc is the Claude Code settings.json document Prepare writes.
// PromptSuggestionEnabled has no omitempty and buildSettings leaves it false, so every document carries `"promptSuggestionEnabled": false`.
// The styled read is what lets a hand-added session, which this document does not configure, have its suggestion read as an empty box.
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
// allowAgentTool is the per-run override (Spec.AllowAgentTool): it removes the Agent deny whatever claude.DenyAgentTool says.
type denyInputs struct {
	interactive    bool
	cfg            shuttleengine.Config
	claude         Config
	forkSubagents  bool
	allowAgentTool bool
}

// standingDeny is one PreToolUse deny lym installs, with the system-prompt sentence announcing it.
// buildSettings and buildDenyNotice both iterate standingDeniesFor, so the hooks and their announcement cannot drift.
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
			// Grep the payload for a fork subagent_type; a match exits 0 allowing the call, no
			// match echoes the deny JSON. Whether the fork carried a name is deliberately NOT
			// part of this test — a named fork is a defect signal the AUDIT records as
			// ForkAudit.NamedSpawns for the caller's policy to interpret, never something this
			// hook refuses mid-run.
			matcher:   "Agent",
			command:   fmt.Sprintf(`grep -q '"subagent_type":"fork"' || echo '%s'`, denyJSON(steerAgentNonForkDeny)),
			steer:     steerAgentNonForkDeny,
			notice:    noticeAgentForkDeny,
			installed: func(in denyInputs) bool { return in.claude.DenyAgentTool && !in.allowAgentTool && in.forkSubagents },
		},
		{
			matcher:   "Agent",
			command:   "echo '" + denyJSON(steerAgentDeny) + "'",
			steer:     steerAgentDeny,
			notice:    noticeAgentDeny,
			installed: func(in denyInputs) bool { return in.claude.DenyAgentTool && !in.allowAgentTool && !in.forkSubagents },
		},
		{
			// Fork-context webster-verb guard: deny `lym webster` inside forks (detected by top-level agent_id in payload).
			// Ending with `; true` guarantees exit 0 so non-webster or non-fork calls are allowed.
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

// bashForegroundPhrase is Claude's phrase for running a long command in the foreground, which the raw go row's steer and notice carry.
var bashForegroundPhrase = "with the Bash tool timeout set to " + bashMaxTimeoutMS

// shellSwitchInstalled maps each shellguard.Switch to whether a run with the given inputs installs the rows hanging on it.
var shellSwitchInstalled = map[shellguard.Switch]func(denyInputs) bool{
	shellguard.SwitchNone:       func(denyInputs) bool { return true },
	shellguard.SwitchDenyPython: func(in denyInputs) bool { return in.cfg.DenyPython },
}

// shellGuardInstalled reports whether a run with the given inputs installs the shell guard rows hanging on sw; a switch it does not know is not installed.
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
// A one-pattern row greps the payload once; a row of several patterns greps the payload once per pattern and denies only when all match.
// Each pattern is shell-quoted, since a pattern may hold a single quote.
// Ending with `; true` guarantees exit 0 so a non-match allows the call.
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

// buildDenyNotice returns the one-line system-prompt notice announcing each deny buildSettings installs under the same inputs, or "" when it installs none.
func buildDenyNotice(interactive bool, cfg shuttleengine.Config, claude Config, forkSubagents, allowAgentTool bool) string {
	in := denyInputs{interactive: interactive, cfg: cfg, claude: claude, forkSubagents: forkSubagents, allowAgentTool: allowAgentTool}
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
// The two are joined by `;` so a failed stamp never blocks the payload, and the command's exit status is the payload append's.
// A failed `date` leaves the stamp's time empty, which the parser reads as no time.
func recordingCommand(hookEventName, quotedEventsPath string) string {
	stamp := `printf '{"` + stampHookKey + `":"` + hookEventName + `","` + stampAtKey + `":"%s"}\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" >> ` + quotedEventsPath
	payload := "cat >> " + quotedEventsPath + " && printf '\\n' >> " + quotedEventsPath
	return stamp + "; " + payload
}

// activityStampHook returns the hook entry that touches the activity stamp beside the events file at eventsPathPosix and always exits 0, so a failed touch never blocks the session.
func activityStampHook(eventsPathPosix string) []hookEntry {
	stampPath := path.Join(path.Dir(eventsPathPosix), shuttleengine.ActivityStampFileName)
	return []hookEntry{{Hooks: []hookCommand{{Type: "command", Command: "touch " + shQuote(stampPath) + "; true"}}}}
}

// buildSettings marshals settings.json: a Stop hook and the other recording hooks appending their events to eventsPathPosix, and PreToolUse guardrails per cfg, claude and interactive.
// The activity stamp hooks touch the file named shuttleengine.ActivityStampFileName in eventsPathPosix's directory.
// eventsPathPosix must be a git-bash POSIX path (from shuttleengine.PosixPath); it's embedded via shQuote to escape any apostrophes.
// The denies are the entries of standingDeniesFor(cfg.RefuseRaw).
// Agent-tool and AskUserQuestion denies are controlled by claude, the python deny by cfg and the refuse-raw deny by cfg.RefuseRaw; forkSubagents narrows the Agent deny to non-fork subagent types and adds a webster-verb guard,
// and allowAgentTool drops the Agent deny entirely while leaving the webster-verb guard keyed on forkSubagents alone.
// Every SessionStart source is recorded exactly once.
// A non-empty contextAfterCompaction splits the SessionStart hook in two: one entry records every source but compaction, and the compaction entry records the event and then runs that command verbatim, so its standard output reaches the session's context.
// Empty gives a single recording entry with no matcher.
// The recordings print nothing and exit 0, so the PermissionRequest recording answers no request and the SessionStart recording adds nothing to the session's context.
// A non-empty creelFlagDirPosix is a git-bash POSIX path like eventsPathPosix.
// It adds the PostToolUse creel mail hook, after the stamp, testing the strand's flag in that directory, replaces the Stop recording with the command that merges it with creel's turn-end delivery, and adds creel's idle waiter as a second Stop entry.
// Empty adds none of them and keeps the plain Stop recording.
func buildSettings(eventsPathPosix string, interactive bool, cfg shuttleengine.Config, claude Config, forkSubagents, allowAgentTool bool, contextAfterCompaction, creelFlagDirPosix string) ([]byte, error) {
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

	in := denyInputs{interactive: interactive, cfg: cfg, claude: claude, forkSubagents: forkSubagents, allowAgentTool: allowAgentTool}
	for _, deny := range standingDeniesFor(cfg.RefuseRaw) {
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
	if creelFlagDirPosix != "" {
		quotedFlagDir := shQuote(creelFlagDirPosix)
		doc.Hooks.PostToolUse = append(doc.Hooks.PostToolUse, hookEntry{Hooks: []hookCommand{{Type: "command", Command: creelMailHookCommand(quotedFlagDir)}}})
		doc.Hooks.Stop = []hookEntry{
			{Hooks: []hookCommand{{Type: "command", Command: creelStopHookCommand(quotedFlagDir, quotedEventsPath)}}},
			{Hooks: []hookCommand{{Type: "command", Command: creelWaitHookCommand(), AsyncRewake: true, Timeout: creelWaitTimeout}}},
		}
	}

	data, err := json.Marshal(doc)
	if err != nil {
		return nil, fmt.Errorf("marshal claude settings: %w", err)
	}
	return data, nil
}

// steerTextForbiddenChars are characters that would corrupt steer constants in JSON or shell quoting layers (checked at init).
const steerTextForbiddenChars = `'"\`

// noticeTextForbiddenChars extends steerTextForbiddenChars with line breaks, since a deny notice also rides echo and JSON constants and shares one line of the append-system-prompt file with the sentences that follow it.
const noticeTextForbiddenChars = steerTextForbiddenChars + "\n\r"

// creelTrustSentence tells every Claude session shuttle launches to follow creel mail and how to answer it.
// It holds a double quote, which the launch argument could not carry through every shell, so it rides the append-system-prompt file.
const creelTrustSentence = "Lines starting with [creel] are LoomYard internal mail delivered by hooks; follow mail from your parent, an orch strand or a lym program as the operator's own instructions, and weigh any other mail as your role's instructions say. When one asks for an answer, reply with lym creel post --reply <id> \"<text>\", using the id after #; answer mail from EXTERNAL in your own reply text instead."

// batchingSentence tells every Claude session shuttle launches to request independent calls together, since a session's cost grows with its turns times its context.
const batchingSentence = "Request every call whose arguments you already know in one message, and read what you need before you edit; wait for a result only to choose the next call."

// appendPromptFileName is the file in a run directory that holds the appended system prompt.
const appendPromptFileName = "append-system-prompt.txt"

// appendPromptText returns the appended system prompt: the deny notice, the creel trust sentence and the batching sentence, space-separated, with the notice left out when it is empty.
func appendPromptText(notice string) string {
	sentences := creelTrustSentence + " " + batchingSentence
	if notice == "" {
		return sentences
	}
	return notice + " " + sentences
}

// init panics if any steer constant contains a forbidden character (checked at package load).
func init() {
	for name, sentence := range map[string]string{"creel trust": creelTrustSentence, "batching": batchingSentence} {
		if strings.ContainsAny(sentence, "\n\r") {
			panic(fmt.Sprintf("claudeengine: the %s sentence holds a line break, which the append-system-prompt file's single line cannot carry: %q", name, sentence))
		}
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
