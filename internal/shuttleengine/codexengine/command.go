// command.go composes the pane-shell lines Prepare and OpenLaunch hand back as a Launch, and the `-c` hook settings the launch line carries.
// The launch line starts the Codex TUI under the lym home with its first turn as the positional argument, then appends the exit record to the run's events file and exits the pane's shell,
// so a Codex exit reads as a process-ending session end and leaves a dead pane.
// An opened session's line is the Codex command alone, with no hooks, no first turn, no exit record and no exit.
// The resume line exits the pane's shell alone and starts no Codex.
// Every hook is a plain POSIX sh command that appends a stamp line and its payload to the run's events file, touches the run's activity stamp, or guards a shell command.
// Argument quoting and statement chaining are pane-shell mechanics owned by internal/shell.

package codexengine

import (
	"path"
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
// Codex's Linux sandbox refuses every socket connect without it, a Unix socket's included, so a session could not reach the creel daemon.
const sandboxNetworkAccess = "sandbox_workspace_write.network_access=true"

// The exit record is the line the launch line appends to the events file once Codex exits, since Codex fires no hook on its own exit.
// It reads as a process-ending session end with reason exitRecordReason; it has no stamp line, so it carries no time.
const (
	exitRecordEvent  = "LymExit"
	exitRecordReason = "codex-exit"
	exitRecordLine   = `{"hook_event_name":"` + exitRecordEvent + `"}`
)

// launchPointer returns the first turn of a session that loads no skills: a pointer to the prompt file at promptPath.
func launchPointer(promptPath string) string {
	return "Read " + promptPath + " in full first; it is your complete, authoritative instructions."
}

// launchLine is what buildLaunchCmd composes a launch line from.
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
	// firstTurn is the session's first turn, typed as the positional prompt; empty for an opened session, which waits at its composer.
	firstTurn string
	// eventsPath is the run's events file, which the launch line appends the exit record to; unused by an opened session's command.
	eventsPath string
}

// buildLaunchCmd composes the pane-shell line that starts a Codex session and, when Codex exits, appends the exit record to the events file and exits the pane's shell.
func buildLaunchCmd(sh shell.Shell, line launchLine) string {
	return sh.Chain(codexCommand(sh, line), sh.AppendLine(line.eventsPath, exitRecordLine), sh.Exit())
}

// codexCommand composes the command that starts a Codex session under the lym home.
// It runs `-a never -s workspace-write`, so no approval prompt shows and the sandbox bounds writes to the workspace, the writable roots and the temporary folders.
// It turns on the sandbox's network access, so the session's shell commands reach the creel daemon's socket.
// It passes every hook setting with `--dangerously-bypass-hook-trust`, since a hook given by `-c` otherwise waits on a review prompt, and the first turn when there is one,
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
// Codex mints its session id at the first turn, which a line written before the launch cannot name, so a revive ends the run as died instead of starting a second session.
func buildResumeCmd(sh shell.Shell) string {
	return sh.Exit()
}

// Stamp line keys: a stamp line is {"lym_stamp":"<hook event name>","lym_at":"<RFC 3339 UTC>"}, written by a recording hook before its payload.
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

// recordedHookEvents are the hooks whose payloads the events file records, in the order their settings ride the launch line.
var recordedHookEvents = []string{hookEventSessionStart, hookEventUserPromptSubmit, hookEventStop, hookEventPermissionRequest}

// emptyHookOutput is what every hook prints: an empty decision, which leaves the session's behaviour unchanged.
const emptyHookOutput = "printf '{}\\n'"

// recordingCommand returns the hook command that appends one hook event to the events file at eventsPath:
// a stamp line naming hookEventName with the hook-side time, then the payload on standard input and a newline, then the empty hook output.
// A failed `date` leaves the stamp's time empty, which the parser reads as no time.
func recordingCommand(hookEventName, eventsPath string) string {
	quoted := shell.Posix().Quote(eventsPath)
	stamp := `printf '{"` + stampHookKey + `":"` + hookEventName + `","` + stampAtKey + `":"%s"}\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" >> ` + quoted
	payload := "cat >> " + quoted + " && printf '\\n' >> " + quoted
	return stamp + "; " + payload + "; " + emptyHookOutput
}

// activityCommand returns the hook command that touches the activity stamp beside the events file at eventsPath and prints the empty hook output, so a failed touch never blocks the session.
func activityCommand(eventsPath string) string {
	stampPath := path.Join(path.Dir(eventsPath), shuttleengine.ActivityStampFileName)
	return "touch " + shell.Posix().Quote(stampPath) + "; " + emptyHookOutput
}

// toolMatcherAll is the matcher a tool hook takes to run on every tool.
const toolMatcherAll = "*"

// hookSettings returns the `-c` hook settings of a run whose events file is eventsPath:
// a recording hook per recorded event, one PreToolUse setting holding an entry per guard row when guards is non-empty, then the activity hook after every tool call.
// The guard entries share one setting, since a second `-c` of the same key replaces the first.
func hookSettings(eventsPath string, guards []shellguard.Row) []string {
	settings := make([]string, 0, len(recordedHookEvents)+2)
	for _, event := range recordedHookEvents {
		settings = append(settings, hookSetting(event, hookEntry("", recordingCommand(event, eventsPath))))
	}
	if len(guards) > 0 {
		settings = append(settings, hookSetting(hookEventPreToolUse, guardEntries(guards)...))
	}
	return append(settings, hookSetting(hookEventPostToolUse, hookEntry(toolMatcherAll, activityCommand(eventsPath))))
}

// hookEntry returns one `{matcher=…,hooks=[…]}` hook entry running command, with matcher when non-empty.
func hookEntry(matcher, command string) string {
	entry := "{"
	if matcher != "" {
		entry += "matcher=" + tomlString(matcher) + ","
	}
	return entry + `hooks=[{type="command",command=` + tomlString(command) + "}]}"
}

// hookSetting returns one `hooks.<event>=[…]` setting holding entries.
func hookSetting(event string, entries ...string) string {
	return "hooks." + event + "=[" + strings.Join(entries, ",") + "]"
}

// tomlString returns s as a TOML basic string.
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

// hexByte returns b as two uppercase hex digits.
func hexByte(b byte) string {
	const digits = "0123456789ABCDEF"
	return string([]byte{digits[b>>4], digits[b&0x0f]})
}

// tomlArray returns values as a TOML array of basic strings.
func tomlArray(values []string) string {
	quoted := make([]string, len(values))
	for i, value := range values {
		quoted[i] = tomlString(value)
	}
	return "[" + strings.Join(quoted, ",") + "]"
}
