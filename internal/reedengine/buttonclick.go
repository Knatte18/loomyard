// buttonclick.go implements the engine-less click of a status-bar button: it finds the clicked entry in the session's spec option, confirms when the entry asks, runs it, logs it and shows a failure on the clicking client.
// It reads no config and no reed state, only the session's @lym_button_spec, through the told anchor reader the current anchor of the worktree holding its run directory, and, for {strand}, the clicking client's active pane.

package reedengine

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/shell"
)

// ButtonRequest is one click of a status-bar button, as the click verb is told it.
type ButtonRequest struct {
	// TmuxPath is the tmux binary and SocketPath the server's socket the click addresses.
	TmuxPath   string
	SocketPath string
	// Session is the tmux session whose spec the click reads.
	Session string
	// Range is the clicked status range, buttonRangePrefix followed by the entry's index.
	Range string
	// Client is the clicking client, which prompts and messages go to.
	Client string
	// Confirmed reports that the operator already answered the entry's prompt, so Hash and Strand are the confirmed line's values.
	Confirmed bool
	// Hash is the buttonRunHash of the run text the prompt was raised for; it is read only when Confirmed.
	Hash string
	// Strand is the strand name the prompt was raised for, set when the entry names {strand}.
	Strand string
}

// buttonDeps are the process-level calls runButtonVia makes, replaceable by a test.
type buttonDeps struct {
	// run starts shellPath with flag and script in dir, waits for it, and returns its combined output and exit code.
	run func(shellPath, flag, script, dir string) (output string, exitCode int, err error)
	// executablePath resolves the running binary, which the confirmed line invokes.
	executablePath func() (string, error)
	// anchorOf returns the anchored directory, as recorded now, of the worktree holding dir.
	anchorOf func(dir string) (string, error)
	// sh is the dialect the run and confirm text and the confirmed line are quoted in.
	sh shell.Shell
}

// RunButton handles one click of a status-bar button on the told server.
// It builds its tmux command from the told paths and reads nothing else of reed's.
// anchorOf returns the anchored directory, as recorded now, of the worktree holding a directory;
// the entry runs there rather than in the rendered run directory, which goes stale when an anchor marker changes after the render.
// It returns nothing: every failure is shown to the clicking client or logged, so the click verb always exits 0.
func RunButton(req ButtonRequest, anchorOf func(dir string) (string, error)) {
	runButtonVia(newTmuxCmdForSocketPath(req.TmuxPath, req.SocketPath), req, buttonDeps{
		run:            runButtonScript,
		executablePath: os.Executable,
		anchorOf:       anchorOf,
		sh:             shell.ForGOOS(),
	})
}

// runButtonVia is RunButton's implementation over an already-built tmux command and its process-level calls, so a test can drive it through the execHook seam.
// A clicked range with no entry, an unrendered spec, a gone run directory, a run directory whose anchor does not resolve, an anchor directory that is gone and a failed run each show one message on the clicking client and run nothing further.
// An entry with a confirm text raises a prompt whose answer re-enters through a confirmed request, which runs the entry only when its run text still hashes to the prompt's hash.
func runButtonVia(cmd TmuxCmd, req ButtonRequest, deps buttonDeps) {
	say := func(text string) {
		// display-message expands formats and strftime fields, so every # and % of the text is doubled to show literally.
		shown := strings.NewReplacer("#", "##", "%", "%%").Replace(text)
		if err := cmd.run("display-message", "-c", req.Client, shown); err != nil {
			logger.Debug("reed: could not show a button message", "client", req.Client, "err", err)
		}
	}

	spec, ok := readButtonSpec(cmd, req.Session)
	if !ok {
		say("lym: the buttons are not rendered for this session; reattach it with lym reed attach")
		return
	}
	entry, ok := findButtonEntry(spec, req.Range)
	if !ok {
		say("lym: the buttons changed; click again")
		return
	}

	values := map[string]string{}
	if entry.Strand {
		strand, problem := buttonStrand(cmd, req, entry)
		if problem != "" {
			say(problem)
			return
		}
		values["strand"] = strand
	}
	run := fillRunPlaceholders(deps.sh, entry.Run, values)
	confirm := fillConfirmPlaceholders(entry.Confirm, values)

	if info, err := os.Stat(spec.Dir); err != nil || !info.IsDir() {
		say("lym: " + entry.Label + ": " + spec.Dir + " is gone; the row re-renders at the session's next attach")
		return
	}
	// The rendered directory is frozen at the last attach while an anchor marker can change after it,
	// and a lym verb refuses anywhere but its worktree's anchor, so the entry runs in the anchor read now.
	runDir, err := deps.anchorOf(spec.Dir)
	if err != nil {
		logger.Warn("reed: could not read the anchor of a button's run directory", "label", entry.Label, "dir", spec.Dir, "err", err)
		say("lym: " + entry.Label + ": the anchor of " + spec.Dir + " does not resolve: " + err.Error())
		return
	}
	if info, err := os.Stat(runDir); err != nil || !info.IsDir() {
		say("lym: " + entry.Label + ": the anchor directory " + runDir + " is gone; point the worktree's anchor at an existing directory")
		return
	}
	if runDir != spec.Dir {
		logger.Info("reed: a button runs in its worktree's current anchor, which differs from the rendered run directory", "label", entry.Label, "rendered", spec.Dir, "anchor", runDir)
	}

	hash := buttonRunHash(entry.Run)
	if req.Confirmed && req.Hash != hash {
		say("lym: the buttons changed since " + entry.Label + " asked; click again")
		return
	}
	if entry.Confirm != "" && !req.Confirmed {
		exe, err := deps.executablePath()
		if err != nil {
			logger.Warn("reed: could not resolve this binary, raising no button prompt", "label", entry.Label, "err", err)
			say("lym: " + entry.Label + " could not raise its prompt; see the lym log in " + runDir + "/.lym/logs")
			return
		}
		line := composeConfirmedButtonLine(deps.sh, exe, req, runDir, hash, values["strand"])
		if err := cmd.run("confirm-before", "-t", req.Client, "-p", confirm+" (y/n)", "run-shell -b "+tmuxQuote(line)); err != nil {
			logger.Warn("reed: could not raise a button prompt", "label", entry.Label, "client", req.Client, "err", err)
		}
		return
	}

	output, exitCode, err := deps.run(spec.Shell, spec.Flag, run, runDir)
	switch {
	case err != nil:
		say("lym: " + entry.Label + " failed (exit " + strconv.Itoa(exitCode) + "): " + err.Error())
	case exitCode != 0:
		firstLine, _, _ := strings.Cut(strings.TrimSpace(output), "\n")
		say("lym: " + entry.Label + " failed (exit " + strconv.Itoa(exitCode) + "): " + strings.TrimSpace(firstLine))
	}
}

// readButtonSpec reads and decodes the session's buttonSpecOption.
// A failed read, an empty value and a value that is not JSON each log a Warn and report false.
func readButtonSpec(cmd TmuxCmd, session string) (buttonSpec, bool) {
	raw, err := cmd.output("show-options", "-qv", "-t", exactSessionWindowTarget(session), buttonSpecOption)
	if err != nil {
		logger.Warn("reed: could not read the button spec", "session", session, "err", err)
		return buttonSpec{}, false
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		logger.Warn("reed: the session holds no button spec", "session", session)
		return buttonSpec{}, false
	}
	var spec buttonSpec
	if err := json.Unmarshal([]byte(raw), &spec); err != nil {
		logger.Warn("reed: the button spec is not valid JSON", "session", session, "err", err)
		return buttonSpec{}, false
	}
	return spec, true
}

// findButtonEntry returns the entry of spec whose index the clicked range names.
// A range whose suffix is no number, and an index no entry holds, report false.
func findButtonEntry(spec buttonSpec, rangeName string) (buttonSpecEntry, bool) {
	suffix, ok := strings.CutPrefix(rangeName, buttonRangePrefix)
	if !ok {
		return buttonSpecEntry{}, false
	}
	index, err := strconv.Atoi(suffix)
	if err != nil {
		return buttonSpecEntry{}, false
	}
	for _, entry := range spec.Entries {
		if entry.Index == index {
			return entry, true
		}
	}
	return buttonSpecEntry{}, false
}

// buttonStrand returns the strand name that fills {strand} for the click, or the message to show instead.
// A confirmed click carries the name the prompt was raised for, which must parse as a strand name.
// Any other click reads it from the clicking client's active pane.
func buttonStrand(cmd TmuxCmd, req ButtonRequest, entry buttonSpecEntry) (strand, problem string) {
	if req.Confirmed {
		if _, err := agentname.Parse(req.Strand); err != nil {
			return "", "lym: " + entry.Label + ": the strand name " + req.Strand + " is not a strand name; click the button again from the agent pane"
		}
		return req.Strand, ""
	}
	name, err := cmd.output("display-message", "-p", "-c", req.Client, "#{"+strandNamePaneOption+"}")
	name = strings.TrimSpace(name)
	if err != nil || name == "" {
		return "", "lym: " + entry.Label + " needs the agent's name, which this pane lacks; reattach the session with lym reed attach and click again"
	}
	return name, ""
}

// runButtonScript starts shellPath with flag and script in dir, waits for it with no timeout, and returns its combined output and exit code.
// The spawn and the end, with the exit code and the output, are logged at Info.
// A process that ran and exited non-zero is no error; an error means it could not be started, with exit code -1.
// It sets no caller identity: the process inherits the tmux server's environment, which reed stripped of the strand name variables at spawn.
func runButtonScript(shellPath, flag, script, dir string) (output string, exitCode int, err error) {
	logger.Info("reed: running a status-bar button", "shell", shellPath, "dir", dir, "script", script)
	command := exec.Command(shellPath, flag, script)
	command.Dir = dir
	combined, runErr := command.CombinedOutput()
	exitCode = 0
	var exitErr *exec.ExitError
	switch {
	case errors.As(runErr, &exitErr):
		exitCode = exitErr.ExitCode()
	case runErr != nil:
		exitCode = -1
		err = runErr
	}
	logger.Info("reed: a status-bar button ended", "exit", exitCode, "output", string(combined), "err", err)
	return string(combined), exitCode, err
}

// buttonRunHash returns the hex SHA-256 of run, which a confirmed click carries instead of the run text.
func buttonRunHash(run string) string {
	sum := sha256.Sum256([]byte(run))
	return hex.EncodeToString(sum[:])
}

// tmuxQuote wraps s in double quotes for tmux's command parser, escaping backslash, double quote and dollar sign with a backslash and doubling every #, since run-shell format-expands its command.
func tmuxQuote(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`, `$`, `\$`, `#`, `##`).Replace(s) + `"`
}
