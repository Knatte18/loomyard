// panebin.go owns the pane-binary seam.
// The seam resolves `lym` to the binary that spawned each strand pane or detached window.
// Two chokepoints reach it, launchStrandLocked for strand panes and OpenWindow for detached windows,
// so every pane reed creates for a `lym` command resolves `lym` to its spawning binary by construction.
// It also composes the session-switch and follow-jump commands.
// The key and mouse bindings run them, built from the same executable-path lookup.
// The launch line exports LYM_BIN, LYM_STRAND_NAME and LYM_PARENT when told.
// It also exports the .NET build-server switches and the hub's PaneEnv variables, ahead of the strand's command.
// The file also owns the per-strand launch script the line is written to,
// so the pane types a short source statement instead of the full line.

package reedengine

import (
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/selfexec"
	"github.com/Knatte18/loomyard/internal/shell"
)

// paneBinPrelude returns the pure, injectable pane-binary composition for exe on sh's dialect.
// It is sh's PrependPathEntry of exe's parent directory, then sh's ExportEnv of agentname.LymBinEnv to exe,
// joined by sh.Chain.
// The prepend is unconditional, with no guard against the directory already being on the pane's PATH:
// Nothing accumulates across relaunches or resumes, because every strand pane is a fresh pane with a fresh shell.
// The one nesting case is a pane whose command itself spawns another strand pane.
// That produces a duplicate PATH entry, which resolves identically.
func paneBinPrelude(sh shell.Shell, exe string) string {
	return sh.Chain(sh.PrependPathEntry(filepath.Dir(exe)), sh.ExportEnv(agentname.LymBinEnv, exe))
}

// nameExports returns the statements exporting the strand's full name on sh's dialect.
// It also exports the worktree's parent when parent is non-empty.
// An empty name exports nothing, so a strand with no formed name never sees an empty LYM_STRAND_NAME.
func nameExports(sh shell.Shell, name, parent string) string {
	var parts []string
	if name != "" {
		parts = append(parts, sh.ExportEnv(agentname.StrandNameEnv, name))
	}
	if parent != "" {
		parts = append(parts, sh.ExportEnv(agentname.ParentEnv, parent))
	}
	return sh.Chain(parts...)
}

// envExports returns the statements exporting vars in name order on sh's dialect, empty when vars holds none.
func envExports(sh shell.Shell, vars map[string]string) string {
	parts := make([]string, 0, len(vars))
	for _, name := range slices.Sorted(maps.Keys(vars)) {
		parts = append(parts, sh.ExportEnv(name, vars[name]))
	}
	return sh.Chain(parts...)
}

// paneEnvExports returns the statements exporting the variables from the geometry's PaneEnv, on sh's dialect.
// It returns empty when PaneEnv is nil or returns none.
// A failed read logs a named logger.Warn and exports nothing, so the strand still launches.
// A variable named by proc.BuildServerOffEnv is left out.
// The launch line exports those switches itself, and a hub value must not undo them.
func (e *Engine) paneEnvExports(sh shell.Shell, strandGUID string) string {
	if e.geom.PaneEnv == nil {
		return ""
	}
	vars, err := e.geom.PaneEnv()
	if err != nil {
		logger.Warn("reed: could not read the hub's env variables, launching strand pane without them", "strand", strandGUID, "err", err)
		return ""
	}
	vars = maps.Clone(vars)
	for _, v := range proc.BuildServerOffEnv {
		delete(vars, v.Name)
	}
	return envExports(sh, vars)
}

// buildServerExports returns the statements exporting proc.BuildServerOffEnv on sh's dialect.
// They keep any .NET build the strand's process runs from leaving MSBuild servers behind.
func buildServerExports(sh shell.Shell) string {
	parts := make([]string, 0, len(proc.BuildServerOffEnv))
	for _, v := range proc.BuildServerOffEnv {
		parts = append(parts, sh.ExportEnv(v.Name, v.Value))
	}
	return sh.Chain(parts...)
}

// composePaneLaunchLine returns the launch script's content:
// the pane-binary prelude, the build-server exports, the name exports, then launchCmd, on sh's dialect.
// It reads the running process's own path via executablePath.
// When the path does not resolve, or names the running test binary, it logs a named logger.Warn and drops the prelude,
// so the pane launches with no prelude rather than failing the strand launch.
// The build-server and name exports do not depend on that lookup and stay either way.
// Because Chain drops empty parts, an empty launchCmd yields the statements alone.
// The result has no trailing separator and no empty command fragment.
// It also returns the executable path the prelude exports, empty when the lookup failed and the prelude was dropped.
func composePaneLaunchLine(sh shell.Shell, executablePath func() (string, error), launchCmd, strandGUID, name, parent string) (line, exe string) {
	prelude := ""
	exe, err := selfexec.ResolveLymBinary(executablePath)
	if err != nil {
		exe = ""
		logger.Warn("reed: could not resolve this binary, launching strand pane with no lym-bin prelude", "strand", strandGUID, "err", err)
	} else {
		prelude = paneBinPrelude(sh, exe)
	}
	return sh.Chain(prelude, buildServerExports(sh), nameExports(sh, name, parent), launchCmd), exe
}

// composeWindowCommand returns the shell line a detached reed window runs:
// the pane-binary prelude, then `lym` invoked with lymArgs, each quoted, on sh's dialect.
// No strand name exports, since a window is not a strand.
// An executable path that does not resolve, or names the running test binary, logs a named logger.Warn.
// The window then opens with no prelude.
func composeWindowCommand(sh shell.Shell, executablePath func() (string, error), lymArgs []string) string {
	prelude := ""
	exe, err := selfexec.ResolveLymBinary(executablePath)
	if err != nil {
		logger.Warn("reed: could not resolve this binary, opening window with no lym-bin prelude", "err", err)
	} else {
		prelude = paneBinPrelude(sh, exe)
	}
	words := make([]string, 0, len(lymArgs)+1)
	words = append(words, sh.Invoke("lym"))
	for _, arg := range lymArgs {
		words = append(words, sh.Quote(arg))
	}
	return sh.Chain(prelude, strings.Join(words, " "))
}

// composeSwitchCommand returns the shell line a key binding's run-shell runs to move the client one session along.
// The line starts with the absolute path of this binary.
// It then runs `reed switch` with the told socket and tmux paths and the key-press client, on sh's dialect.
// It reads the running process's own path via executablePath,
// so the binding names the `lym` that pinned it, never the one PATH resolves.
// Every pinned token is quoted once and then has each # doubled.
// The doubling keeps run-shell's format expansion at key press from touching the pinned tokens.
// #{q:client_name} is the one value left for that expansion, as its own unquoted word.
// An executable path that does not resolve, or names the running test binary, logs a named logger.Warn.
// It then returns false, so the caller pins no switch binding.
func composeSwitchCommand(sh shell.Shell, executablePath func() (string, error), next bool, socketPath, tmuxPath string) (string, bool) {
	exe, err := selfexec.ResolveLymBinary(executablePath)
	if err != nil {
		logger.Warn("reed: could not resolve this binary, pinning no session-switch binding", "err", err)
		return "", false
	}
	direction := "--prev"
	if next {
		direction = "--next"
	}
	pinned := func(token string) string { return strings.ReplaceAll(token, "#", "##") }
	words := []string{
		pinned(sh.Invoke(exe)), "reed", "switch", direction,
		"--socket", pinned(sh.Quote(socketPath)),
		"--client", "#{q:client_name}",
		"--tmux", pinned(sh.Quote(tmuxPath)),
	}
	return strings.Join(words, " "), true
}

// composeFollowCommand returns the shell line the follow button's run-shell runs to jump back to the run.
// The line starts with the absolute path of this binary.
// It then runs `reed follow` with the told socket and tmux paths and the clicking client's session, on sh's dialect.
// It reads the running process's own path via executablePath,
// so the button names the `lym` that pinned it, never the one PATH resolves.
// Every pinned token is quoted once and then has each # doubled.
// The line is format-expanded when the button is clicked,
// and the doubling keeps that expansion from touching the pinned tokens.
// #{q:session_name} is the one value left for that expansion, as its own unquoted word,
// so the session reaches the verb as one quoted argument.
// An executable path that does not resolve, or names the running test binary, logs a named logger.Warn.
// It then returns false, so the caller pins no follow jump.
func composeFollowCommand(sh shell.Shell, executablePath func() (string, error), socketPath, tmuxPath string) (string, bool) {
	exe, err := selfexec.ResolveLymBinary(executablePath)
	if err != nil {
		logger.Warn("reed: could not resolve this binary, pinning no follow jump", "err", err)
		return "", false
	}
	pinned := func(token string) string { return strings.ReplaceAll(token, "#", "##") }
	words := []string{
		pinned(sh.Invoke(exe)), "reed", "follow",
		"--socket", pinned(sh.Quote(socketPath)),
		"--session", "#{q:session_name}",
		"--tmux", pinned(sh.Quote(tmuxPath)),
	}
	return strings.Join(words, " "), true
}

// composeButtonCommand returns the shell line the click binding runs for a button range, on sh's dialect.
// It changes into the session's run directory, then runs the absolute path of this binary with `reed button`.
// The line also carries the told socket and tmux paths, and the clicked session, range and client.
// The run directory rides in the line because run-shell format-expands its command but not its start directory;
// it is read from the session option #{@lym_button_dir}, which the render pins already quoted in sh's dialect,
// so the word is left for the click's expansion unquoted.
// Every pinned token is quoted once and then has each # doubled.
// The line is format-expanded when the button is clicked,
// and the doubling keeps that expansion from touching the pinned tokens.
// #{q:session_name}, #{q:mouse_status_range} and #{q:client_name} are left for that expansion, each as its own word.
// The output of the directory change and of the verb is discarded,
// and the line ends with status zero, so tmux opens no view mode whatever either does, a gone run directory included.
// An executable path that does not resolve, or names the running test binary, logs a named logger.Warn.
// It then returns false, so the caller pins no button click.
func composeButtonCommand(sh shell.Shell, executablePath func() (string, error), socketPath, tmuxPath string) (string, bool) {
	exe, err := selfexec.ResolveLymBinary(executablePath)
	if err != nil {
		logger.Warn("reed: could not resolve this binary, pinning no button click", "err", err)
		return "", false
	}
	pinned := func(token string) string { return strings.ReplaceAll(token, "#", "##") }
	words := []string{
		pinned(sh.Invoke(exe)), "reed", "button",
		"--socket", pinned(sh.Quote(socketPath)),
		"--tmux", pinned(sh.Quote(tmuxPath)),
		"--session", "#{q:session_name}",
		"--range", "#{q:mouse_status_range}",
		"--client", "#{q:client_name}",
	}
	return sh.Chain(sh.DiscardOutput(sh.ChangeDirToWord("#{"+buttonDirOption+"}")), sh.DiscardOutput(strings.Join(words, " ")), sh.ExitSuccess()), true
}

// composeSlackToggleCommand returns the shell line the click binding runs for a Slack toggle, on sh's dialect.
// It changes into the session's run directory, as composeButtonCommand's line does.
// Then it runs the absolute path of this binary with `slack toggle` and the clicked range, slack or away.
// The line also carries the told socket and tmux paths and the clicking client, so the verb can redraw its bar.
// Every pinned token is quoted once and then has each # doubled, since the click format-expands the line.
// #{q:mouse_status_range} and #{q:client_name} are left for that expansion, each as its own word.
// The output of both steps is discarded and the line ends with status zero, so tmux opens no view mode.
// An executable path that does not resolve, or names the running test binary, logs a named logger.Warn.
// It then returns false, so the caller pins no toggle click.
func composeSlackToggleCommand(sh shell.Shell, executablePath func() (string, error), socketPath, tmuxPath string) (string, bool) {
	exe, err := selfexec.ResolveLymBinary(executablePath)
	if err != nil {
		logger.Warn("reed: could not resolve this binary, pinning no Slack toggle click", "err", err)
		return "", false
	}
	pinned := func(token string) string { return strings.ReplaceAll(token, "#", "##") }
	words := []string{
		pinned(sh.Invoke(exe)), "slack", "toggle", "#{q:mouse_status_range}",
		"--socket", pinned(sh.Quote(socketPath)),
		"--tmux", pinned(sh.Quote(tmuxPath)),
		"--client", "#{q:client_name}",
	}
	return sh.Chain(sh.DiscardOutput(sh.ChangeDirToWord("#{"+buttonDirOption+"}")), sh.DiscardOutput(strings.Join(words, " ")), sh.ExitSuccess()), true
}

// composeSlackStateCommand returns the shell line the Slack toggles' state job runs, on sh's dialect:
// the absolute path of this binary with `slack bar`, which prints the machine's Slack state.
// The line rides the option the job's format reads, and an option's value is not expanded again, so no # is doubled.
// An executable path that does not resolve, or names the running test binary, logs a named logger.Warn.
// It then returns false, so both toggles stay unlit.
func composeSlackStateCommand(sh shell.Shell, executablePath func() (string, error)) (string, bool) {
	exe, err := selfexec.ResolveLymBinary(executablePath)
	if err != nil {
		logger.Warn("reed: could not resolve this binary, pinning no Slack state line", "err", err)
		return "", false
	}
	return sh.Invoke(exe) + " slack bar", true
}

// The flags a button's prompt re-enters the click verb with.
// buttonStageAnswered is passed after the ask prompt, and buttonStageConfirmed after a yes to the confirm prompt.
const (
	buttonStageAnswered  = "--answered"
	buttonStageConfirmed = "--confirmed"
)

// composeButtonReentryLine returns the shell line a button's prompt runs once answered, on sh's dialect.
// The line changes into dir, then runs exe as `reed button` with req's socket, tmux, session, range and client.
// The line then adds the stage and `--hash <hash>`,
// and it adds `--strand` and `--answer` with their values when values holds them.
// Every value is quoted as one word.
// The line opens with the directory change because run-shell does not format-expand its start directory,
// so the re-entered verb starts in the run directory.
// The directory change and the verb discard their output.
// The line ends with status zero, so tmux opens no view mode on the target pane whatever either does.
func composeButtonReentryLine(sh shell.Shell, exe string, req ButtonRequest, dir, stage, hash string, values map[string]string) string {
	words := []string{
		sh.Invoke(exe), "reed", "button",
		"--socket", sh.Quote(req.SocketPath),
		"--tmux", sh.Quote(req.TmuxPath),
		"--session", sh.Quote(req.Session),
		"--range", sh.Quote(req.Range),
		"--client", sh.Quote(req.Client),
		stage, "--hash", sh.Quote(hash),
	}
	for _, name := range []string{buttonStrandKey, buttonAnswerKey} {
		if value, ok := values[name]; ok {
			words = append(words, "--"+name, sh.Quote(value))
		}
	}
	return sh.Chain(sh.DiscardOutput(sh.ChangeDir(dir)), sh.DiscardOutput(strings.Join(words, " ")), sh.ExitSuccess())
}

// The two segments, joined under the state dir, name the directory that holds reed's per-strand launch scripts.
const (
	launchScriptReedSegment   = "reed"
	launchScriptLaunchSegment = "launch"
)

// launchScriptDir returns the directory under stateDir that holds per-strand launch scripts.
func launchScriptDir(stateDir string) string {
	return filepath.Join(stateDir, launchScriptReedSegment, launchScriptLaunchSegment)
}

// launchScriptPath returns the launch script path for strandGUID on sh's dialect.
func launchScriptPath(sh shell.Shell, stateDir, strandGUID string) string {
	return filepath.Join(launchScriptDir(stateDir), strandGUID+sh.ScriptExt())
}

// stageLaunchScript writes composedLine plus a trailing newline to the strand's launch script.
// It returns sh's source statement for that script, the send-keys payload.
// On any write error it logs a named logger.Warn and returns composedLine unchanged,
// so a cosmetic feature never fails a strand launch.
func stageLaunchScript(sh shell.Shell, stateDir, strandGUID, composedLine string) string {
	path := launchScriptPath(sh, stateDir, strandGUID)
	if err := writeLaunchScript(path, composedLine+"\n"); err != nil {
		logger.Warn("reed: could not write launch script, sending the full launch line", "strand", strandGUID, "path", path, "err", err)
		return composedLine
	}
	return sh.Source(path)
}

// removeLaunchScripts deletes the launch script of every GUID in guids.
// Deletion is best-effort: a missing file is silent,
// and any other error is logged with logger.Warn and never returned,
// so a cosmetic file never fails the removal that triggered it.
func removeLaunchScripts(sh shell.Shell, stateDir string, guids []string) {
	for _, guid := range guids {
		path := launchScriptPath(sh, stateDir, guid)
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			logger.Warn("reed: could not delete launch script", "strand", guid, "path", path, "err", err)
		}
	}
}

// writeLaunchScript atomically writes content to path with mode 0o644.
// The file is sourced, never executed, so it carries no exec bit.
// The temp file that os.CreateTemp creates has mode 0o600, so the mode is set explicitly before the rename.
func writeLaunchScript(path, content string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	renamed := false
	defer func() {
		if !renamed {
			_ = os.Remove(tmpName)
		}
	}()
	if _, err := tmp.WriteString(content); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	renamed = true
	return nil
}
