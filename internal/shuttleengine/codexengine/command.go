// command.go composes the pane-shell lines that start, open and resume a Codex session, and the hook commands.
// A launch line appends the exit record to the run's events file and exits the pane's shell,
// so a Codex exit reads as a process-ending session end and leaves a dead pane.

package codexengine

import (
	"path"
	"strconv"
	"strings"

	"github.com/Knatte18/loomyard/internal/shell"
	"github.com/Knatte18/loomyard/internal/shellguard"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// codexHomeEnvKey is the variable Codex reads its home directory from.
// It rides the pane line because reed's server environment is scrubbed of every CODEX_ key.
const codexHomeEnvKey = "CODEX_HOME"

// instructionFallbackFile is the file Codex loads as project instructions in a directory with no AGENTS.md.
const instructionFallbackFile = "CLAUDE.md"

// sandboxNetworkAccess is the `-c` setting that lets the workspace-write sandbox open sockets.
// Codex's Linux sandbox refuses every socket connect without it, a Unix socket's included,
// so a session could not reach the creel daemon.
const sandboxNetworkAccess = "sandbox_workspace_write.network_access=true"

// The exit record is the line the launch line appends to the events file once Codex exits.
// It reads as a process-ending session end with reason exitRecordReason; it has no stamp line, so it carries no time.
// Codex fires no hook on its own exit, which is why the launch line appends this record.
const (
	exitRecordEvent  = "LymExit"
	exitRecordReason = "codex-exit"
	exitRecordLine   = `{"hook_event_name":"` + exitRecordEvent + `"}`
)

// launchPointer returns the first turn of a session that loads no skills: a pointer to the prompt file at promptPath.
func launchPointer(promptPath string) string {
	return "Read " + promptPath + " in full first; it is your complete, authoritative instructions."
}

// launchLine holds the settings that a session's launch line is composed from.
type launchLine struct {
	// bin is the Codex executable.
	bin string
	// home is the lym Codex home the session runs under.
	home string
	// writableRoots are the folders outside the workspace the sandbox lets the session write.
	writableRoots []string
	// model is the model slug; empty takes Codex's default.
	model string
	// effort is the reasoning effort; empty takes Codex's default.
	effort string
	// hooks are the `-c` hook settings, each one `hooks.<Event>=[…]` value; none for an opened session.
	hooks []string
	// firstTurn is the session's first turn, typed as the positional prompt;
	// empty for an opened session, which waits at its composer.
	firstTurn string
	// eventsPath is the run's events file, which the launch line appends the exit record to;
	// unused by an opened session's command.
	eventsPath string
}

// buildLaunchCmd composes the pane-shell line that starts a Codex session.
// When Codex exits, the line appends the exit record to the events file and exits the pane's shell.
func buildLaunchCmd(sh shell.Shell, line launchLine) string {
	return sh.Chain(codexCommand(sh, line), sh.AppendLine(line.eventsPath, exitRecordLine), sh.Exit())
}

// codexCommand composes the command that starts a Codex session under the lym home.
// It runs `-a never -s workspace-write`, so no approval prompt shows.
// The sandbox bounds writes to the workspace, the writable roots and the temporary folders.
// It turns on the sandbox's network access, so the session's shell commands reach the creel daemon's socket.
// A hook given by `-c` otherwise waits on a review prompt,
// so the command passes `--dangerously-bypass-hook-trust` with the hook settings.
// It passes the first turn as the positional prompt when there is one,
// and it loads CLAUDE.md as the project instructions of a directory with no AGENTS.md.
// CODEX_HOME names the lym home as a command-scoped assignment.
func codexCommand(sh shell.Shell, line launchLine) string {
	cmd := sh.Invoke(line.bin) + " --no-alt-screen -a never -s workspace-write -c " + sh.Quote(sandboxNetworkAccess)
	if len(line.writableRoots) > 0 {
		cmd += " -c " + sh.Quote("sandbox_workspace_write.writable_roots="+tomlArray(line.writableRoots))
	}
	if line.model != "" {
		cmd += " -m " + sh.Quote(line.model)
	}
	if line.effort != "" {
		cmd += " -c " + sh.Quote("model_reasoning_effort="+tomlString(line.effort))
	}
	cmd += " -c " + sh.Quote("project_doc_fallback_filenames="+tomlArray([]string{instructionFallbackFile}))
	for _, hook := range line.hooks {
		cmd += " -c " + sh.Quote(hook)
	}
	if len(line.hooks) > 0 {
		cmd += " --dangerously-bypass-hook-trust"
	}
	if line.firstTurn != "" {
		cmd += " " + sh.Quote(line.firstTurn)
	}
	return sh.WithEnv(codexHomeEnvKey, line.home, cmd)
}

// buildResumeCmd composes the line reed replays to revive the strand: an exit of the pane's shell and no Codex.
// Codex mints its session id at the first turn, which a line written before the launch cannot name,
// so a revive ends the run as died instead of starting a second session.
func buildResumeCmd(sh shell.Shell) string {
	return sh.Exit()
}

// Stamp line keys: a stamp line is {"lym_stamp":"<hook event name>","lym_at":"<RFC 3339 UTC>"}.
// A recording hook writes it before the hook's payload.
const (
	stampHookKey = "lym_stamp"
	stampAtKey   = "lym_at"
)

// Hook event names, as Codex's hook settings key them and its payloads carry them in hook_event_name.
const (
	hookEventSessionStart      = "SessionStart"
	hookEventUserPromptSubmit  = "UserPromptSubmit"
	hookEventStop              = "Stop"
	hookEventPermissionRequest = "PermissionRequest"
	hookEventPreToolUse        = "PreToolUse"
	hookEventPostToolUse       = "PostToolUse"
)

// recordedHookEvents are the hooks whose payloads the events file records.
// Their settings ride the launch line in this order.
var recordedHookEvents = []string{hookEventSessionStart, hookEventUserPromptSubmit, hookEventStop, hookEventPermissionRequest}

// emptyHookOutput is what every hook prints: an empty decision, which leaves the session's behaviour unchanged.
const emptyHookOutput = "printf '{}\\n'"

// recordingCommand returns the hook command that appends one hook event to the events file at eventsPath.
// It then prints the empty hook output.
func recordingCommand(hookEventName, eventsPath string) string {
	return appendRecord(hookEventName, eventsPath) + "; " + emptyHookOutput
}

// appendRecord returns the command that appends one hook event to the events file at eventsPath and prints nothing:
// a stamp line naming hookEventName with the hook-side time, then the payload on standard input and a newline.
// Its exit status is the payload append's,
// and a failed `date` leaves the stamp's time empty, which the parser reads as no time.
func appendRecord(hookEventName, eventsPath string) string {
	quoted := shell.Posix().Quote(eventsPath)
	stamp := `printf '{"` + stampHookKey + `":"` + hookEventName + `","` + stampAtKey + `":"%s"}\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" >> ` + quoted
	payload := "cat >> " + quoted + " && printf '\\n' >> " + quoted
	return stamp + "; " + payload
}

// activityCommand returns the hook command that touches the activity stamp beside the events file at eventsPath.
// It prints the empty hook output, so a failed touch never blocks the session.
func activityCommand(eventsPath string) string {
	stampPath := path.Join(path.Dir(eventsPath), shuttleengine.ActivityStampFileName)
	return "touch " + shell.Posix().Quote(stampPath) + "; " + emptyHookOutput
}

// toolMatcherAll is the matcher a tool hook takes to run on every tool.
const toolMatcherAll = "*"

// creelDelivery is what a run's creel hooks are built from; its zero value installs none.
type creelDelivery struct {
	// flagDir is the hub's creel flag directory, which holds a flag per strand with unread mail; empty outside a hub.
	flagDir string
	// home is the lym Codex home the idle waiter queues a woken turn under.
	home string
	// bin is the Codex executable the idle waiter runs to queue that turn.
	bin string
}

// Creel's hook verbs, told this engine's registry name so they render its hook output and its wake.
const (
	creelMailCommand = "lym creel hook-read " + engineName
	creelWaitCommand = "lym creel hook-wait " + engineName
)

// creelWaitTimeoutSeconds bounds the idle waiter hook, a day: long enough to outlive any idle spell.
const creelWaitTimeoutSeconds = 86400

// codexBinaryEnvKey names the variable the idle waiter's command sets to the Codex executable.
// The idle waiter runs that executable to queue a woken turn.
// It is not a CODEX_ key, since Codex may read every one of those as its own.
const codexBinaryEnvKey = "LYM_CODEX_BINARY"

// creelFlagTest returns the condition that holds when the strand's mail flag stands in the directory at flagDir.
// The flag's file name is the one the command forms in `$f$n`.
func creelFlagTest(flagDir string) string {
	return `[ -f ` + shell.Posix().Quote(flagDir) + `/"$f$n" ]`
}

// creelMailHookCommand returns the PostToolUse hook command that hands the strand its creel mail after a tool call.
// It prints the empty hook output and starts no process unless the strand's flag stands in flagDir.
// When the flag stands, it runs creel's mail verb, whose output is the hook's.
func creelMailHookCommand(flagDir string) string {
	return shuttleengine.StrandFlagNameShell() + "; " + creelFlagTest(flagDir) + " || { " + emptyHookOutput + "; exit 0; }; " + creelMailCommand
}

// creelStopHookCommand returns the Stop hook command that records the turn end in the events file at eventsPath.
// It reads the hook input once.
// When the strand's mail flag stands in flagDir, it pipes the input to creel's mail verb,
// and a zero exit from that verb ends the hook without recording the turn end, since the mail continues the turn.
// Otherwise it appends the Stop record,
// and it prints the empty hook output only when the flag is absent,
// so a hook-read output stays the hook's one JSON document.
func creelStopHookCommand(flagDir, eventsPath string) string {
	deliver := `if ` + creelFlagTest(flagDir) + `; then printf '%s' "$in" | ` + creelMailCommand + ` && exit 0; else ` + emptyHookOutput + `; fi`
	return `in=$(cat); ` + shuttleengine.StrandFlagNameShell() + `; ` + deliver + `; printf '%s' "$in" | { ` + appendRecord(hookEventStop, eventsPath) + `; }`
}

// creelWaitHookCommand returns the idle waiter's command, with the Codex home and executable set in its environment.
// The waiter queues the woken turn through that executable.
// The home is set on the command as well as inherited,
// so the queued turn reaches this session's home even when Codex hands its hooks a trimmed environment.
func creelWaitHookCommand(creel creelDelivery) string {
	posix := shell.Posix()
	return codexHomeEnvKey + "=" + posix.Quote(creel.home) + " " + codexBinaryEnvKey + "=" + posix.Quote(creel.bin) + " " + creelWaitCommand
}

// hookSettings returns the `-c` hook settings of a run whose events file is eventsPath.
// Each recorded event gets a recording hook.
// When guards is non-empty, a PreToolUse setting holds one entry per guard row.
// The activity hook runs after every tool call.
// With a creel flag directory, the Stop setting runs the merged creel Stop command, then the async idle waiter.
// The PostToolUse setting also runs the creel mail hook, after the activity hook.
// Entries of one event share one setting, since a second `-c` of the same key replaces the first.
func hookSettings(eventsPath string, guards []shellguard.Row, creel creelDelivery) []string {
	settings := make([]string, 0, len(recordedHookEvents)+2)
	for _, event := range recordedHookEvents {
		if event == hookEventStop && creel.flagDir != "" {
			settings = append(settings, hookSetting(event, hookEntry("", creelStopHookCommand(creel.flagDir, eventsPath)), asyncHookEntry(creelWaitHookCommand(creel), creelWaitTimeoutSeconds)))
			continue
		}
		settings = append(settings, hookSetting(event, hookEntry("", recordingCommand(event, eventsPath))))
	}
	if len(guards) > 0 {
		settings = append(settings, hookSetting(hookEventPreToolUse, guardEntries(guards)...))
	}
	postToolUse := []string{hookEntry(toolMatcherAll, activityCommand(eventsPath))}
	if creel.flagDir != "" {
		postToolUse = append(postToolUse, hookEntry(toolMatcherAll, creelMailHookCommand(creel.flagDir)))
	}
	return append(settings, hookSetting(hookEventPostToolUse, postToolUse...))
}

func hookEntry(matcher, command string) string {
	entry := "{"
	if matcher != "" {
		entry += "matcher=" + tomlString(matcher) + ","
	}
	return entry + `hooks=[{type="command",command=` + tomlString(command) + "}]}"
}

// asyncHookEntry returns a hook entry with no matcher, whose command Codex runs in the background.
// The command runs for up to timeoutSeconds without holding the turn open.
func asyncHookEntry(command string, timeoutSeconds int) string {
	return `{hooks=[{type="command",command=` + tomlString(command) + `,async=true,timeout=` + strconv.Itoa(timeoutSeconds) + "}]}"
}

func hookSetting(event string, entries ...string) string {
	return "hooks." + event + "=[" + strings.Join(entries, ",") + "]"
}

// tomlString returns s as a TOML basic string, escaping the quote, the backslash and control characters.
func tomlString(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch {
		case r == '"':
			b.WriteString(`\"`)
		case r == '\\':
			b.WriteString(`\\`)
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\t':
			b.WriteString(`\t`)
		case r < 0x20 || r == 0x7f:
			b.WriteString(`\u00` + hexByte(byte(r)))
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

func hexByte(b byte) string {
	const digits = "0123456789ABCDEF"
	return string([]byte{digits[b>>4], digits[b&0x0f]})
}

func tomlArray(values []string) string {
	quoted := make([]string, len(values))
	for i, value := range values {
		quoted[i] = tomlString(value)
	}
	return "[" + strings.Join(quoted, ",") + "]"
}
